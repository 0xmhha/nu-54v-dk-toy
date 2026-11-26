#!/usr/bin/env python3
"""Fail CI when a selected tool, downloaded executable or action differs from the lock."""

import argparse
import hashlib
import json
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
LOCK = json.loads((ROOT / ".github/inputs.json").read_text())
TOOLS = {
    "python": (["python3", "--version"], r"Python (\S+)"),
    "go": (["go", "env", "GOVERSION"], r"go(\S+)"),
    "node": (["node", "--version"], r"v(\S+)"),
    "pnpm": (["pnpm", "--version"], r"(\S+)"),
    "uv": (["uv", "--version"], r"uv (\S+)"),
    "foundry": (["forge", "--version"], r"forge Version: (\d+\.\d+\.\d+)(?:-v\S+)?"),
    "cast": (["cast", "--version"], r"cast Version: (\d+\.\d+\.\d+)(?:-v\S+)?"),
    "java": (["java", "-version"], r"build (\d+\.\d+\.\d+\+\d+)"),
    "ruby": (["ruby", "--version"], r"ruby (\S+)"),
    # Bundler 4 prints only the number; older ones print "Bundler version 2.x".
    "bundler": (["bundle", "--version"], r"(?:Bundler version )?(\d+\.\d+\.\d+)"),
    "xcode": (["xcodebuild", "-version"], r"Xcode (\S+)\s+Build version (\S+)"),
    "nrf-sdk-manager": (["nrfutil", "sdk-manager", "--version"], r"(?m)^nrfutil-sdk-manager (\d+\.\d+\.\d+)(?:\s|$)"),
}


def check(condition, message):
    if not condition:
        raise ValueError(message)


def check_tools(names):
    for name in names:
        cmd, pattern = TOOLS[name]
        observed = subprocess.check_output(cmd, stderr=subprocess.STDOUT, text=True)
        match = re.search(pattern, observed)
        # macOS runners have no prebuilt Python newer than 3.12.10, so they keep their own pin.
        key = "python-macos" if name == "python" and sys.platform == "darwin" else "foundry" if name == "cast" else name
        expected = LOCK["versions"][key]
        actual = " / ".join(match.groups()) if match else None
        check(actual == expected, f"{name}: expected {expected}; observed {observed.strip()}")
        print(f"{name}={expected}")


def check_file(name, file):
    with file.open("rb") as stream:
        check(hashlib.file_digest(stream, "sha256").hexdigest() == LOCK["sha256"][name], f"{name} digest mismatch")
    print(f"{name} sha256 verified")


def check_android():
    for name in ("p02-user-app", "p04-merchant-kiosk"):
        path = ROOT / "products" / name / "android/gradle/wrapper"
        check_file("gradle-wrapper", path / "gradle-wrapper.jar")
        lines = (path / "gradle-wrapper.properties").read_text().splitlines()
        expected = f"distributionUrl=https\\://services.gradle.org/distributions/gradle-{LOCK['versions']['gradle']}-bin.zip"
        check([line for line in lines if line.startswith("distributionUrl=")] == [expected], f"{name}: Gradle URL mismatch")
        sha = f"distributionSha256Sum={LOCK['sha256']['gradle']}"
        check([line for line in lines if line.startswith("distributionSha256Sum=")] == [sha], f"{name}: Gradle ZIP digest mismatch")


def check_actions(root=ROOT):
    for path in (root / ".github/workflows").glob("*.y*ml"):
        workflow = json.loads(subprocess.check_output(
            ["ruby", "-rjson", "-ryaml", "-e", "puts JSON.generate(YAML.safe_load(File.read(ARGV[0])))", path],
            text=True,
        ))
        for job in workflow["jobs"].values():
            for item in [job, *job.get("steps", [])]:
                if "uses" not in item:
                    continue
                reference = item["uses"]
                check(isinstance(reference, str), f"{path.name}: action reference must be text")
                if reference.startswith("./"):
                    check((root / reference).resolve().is_relative_to(root), f"{path.name}: unsafe local action")
                    continue
                action, separator, ref = reference.rpartition("@")
                expected = LOCK["actions"].get(action)
                check(separator and expected and expected == ref and re.fullmatch(r"[a-f0-9]{40}", ref),
                      f"{path.name}: unpinned Action {reference}")
                if action == "actions/setup-node":
                    actual = item.get("with", {}).get("node-version")
                    expected_node = LOCK["versions"]["node"]
                    check(actual == expected_node, f"{path.name}: setup-node expected {expected_node}; observed {actual}")
    print("GitHub Actions use locked full SHAs and setup-node input")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tools", nargs="*", metavar="TOOL")
    parser.add_argument("--file", nargs=2, action="append", metavar=("LOCK_KEY", "PATH"), default=[])
    parser.add_argument("--android", action="store_true")
    parser.add_argument("--actions", action="store_true")
    args = parser.parse_args()
    try:
        if args.actions:
            check_actions()
        if args.android:
            check_android()
        for name, file in args.file:
            check_file(name, Path(file))
        check_tools(args.tools)
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError) as error:
        parser.exit(1, f"FAIL {error}\n")


if __name__ == "__main__":
    main()

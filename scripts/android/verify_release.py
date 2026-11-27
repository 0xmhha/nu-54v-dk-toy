#!/usr/bin/env python3
"""Verify an Android release artifact against the source-controlled signer policy."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import xml.etree.ElementTree as ET


HERE = Path(__file__).resolve().parent
POLICY = HERE / "release-signers.json"
BUILD_TOOLS = "37.0.0"
APK_SIGNER = r"(?:V[1-4](?:\.\d+)? )?Signer(?: #\d+)?:?"
BUNDLETOOL_SHA256 = "a099cfa1543f55593bc2ed16a70a7c67fe54b1747bb7301f37fdfd6d91028e29"


def output(*command: str | Path) -> str:
    return subprocess.run(
        command, check=True, stdout=subprocess.PIPE, text=True,
        env={**os.environ, "LC_ALL": "C"},
    ).stdout.strip()


def verify_apk(artifact: Path) -> tuple[str, str]:
    sdk = os.environ.get("ANDROID_HOME") or os.environ.get("ANDROID_SDK_ROOT")
    if not sdk:
        raise ValueError("Set ANDROID_HOME to use Android SDK build tools")
    tools = Path(sdk) / "build-tools" / BUILD_TOOLS
    signing = output(tools / "apksigner", "verify", "--Werr", "--verbose", "--print-certs", artifact)
    signers = {
        signer.lower()
        for signer in re.findall(rf"^{APK_SIGNER} certificate SHA-256 digest: ([a-fA-F0-9]{{64}})$", signing, re.MULTILINE)
    }
    if len(signers) != 1 or not re.search(r"^Number of signers: 1$", signing, re.MULTILINE):
        raise ValueError("Expected one verified APK signer")
    if re.search(rf"^{APK_SIGNER} certificate DN: .*\bCN\s*=\s*\"?Android Debug", signing, re.IGNORECASE | re.MULTILINE):
        raise ValueError("Android debug certificate")
    badging = output(tools / "aapt2", "dump", "badging", artifact)
    if "application-debuggable" in badging.splitlines():
        raise ValueError("Debuggable APK")
    package = re.search(r"^package: name='([^']+)'", badging, re.MULTILINE)
    if not package:
        raise ValueError("Cannot determine APK package")
    return package[1], signers.pop()


def verify_aab(artifact: Path, bundletool: Path | None) -> tuple[str, str]:
    if not bundletool:
        raise ValueError("The AAB verifier requires --bundletool or BUNDLETOOL_JAR")
    with bundletool.open("rb") as source:
        if hashlib.file_digest(source, "sha256").hexdigest() != BUNDLETOOL_SHA256:
            raise ValueError("bundletool does not match the pinned 1.18.3 release")
    signer = output("java", HERE / "BundleSigner.java", artifact)
    manifest = ET.fromstring(output("java", "-jar", bundletool, "dump", "manifest", f"--bundle={artifact}"))
    application = manifest.find("application")
    if application is None or "package" not in manifest.attrib:
        raise ValueError("Missing AAB application manifest")
    debuggable = application.get("{http://schemas.android.com/apk/res/android}debuggable", "false")
    if debuggable != "false":
        raise ValueError("Debuggable AAB")
    return manifest.attrib["package"], signer


def verify_artifact(product: str, channel: str, artifact: Path, bundletool: Path | None) -> dict:
    policy = json.loads(POLICY.read_text(encoding="utf-8"))
    app = policy["apps"][product]
    official_pins = [pin for pins in policy["apps"].values() for pin in pins.values() if pin]
    if len(set(official_pins)) != len(official_pins) or any(
        not re.fullmatch(r"[0-9a-f]{64}", pin) or pin == policy["debugSha256"] for pin in official_pins
    ):
        raise ValueError("Android release policy requires distinct non-debug certificates per app and role")
    if channel != "custom":
        pin = app[f"{channel}Sha256"]
        if not pin or not re.fullmatch(r"[0-9a-f]{64}", pin):
            raise ValueError(f"{product} has no enrolled {channel} certificate")

    if artifact.suffix not in (".apk", ".aab") or (artifact.suffix == ".aab" and channel == "distribution"):
        raise ValueError(f"Invalid artifact type for the {channel} channel")
    with tempfile.TemporaryDirectory() as temp:
        snapshot = Path(temp) / artifact.name
        shutil.copyfile(artifact, snapshot)
        if artifact.suffix == ".apk":
            package, signer = verify_apk(snapshot)
        else:
            package, signer = verify_aab(snapshot, bundletool)
        with snapshot.open("rb") as source:
            artifact_digest = hashlib.file_digest(source, "sha256").hexdigest()

    expected_package = f"{product}.custom" if channel == "custom" else product
    if package != expected_package:
        raise ValueError(f"Expected Android package {expected_package}, found {package}")
    if signer == policy["debugSha256"]:
        raise ValueError("Android debug certificate")
    if channel == "custom":
        if signer in official_pins:
            raise ValueError("Official keys cannot sign custom Android builds")
    elif signer != pin:
        raise ValueError(f"Artifact signer does not match the enrolled {channel} certificate for {product}")
    return {
        "package": package,
        "channel": channel,
        "file": artifact.name,
        "artifactSha256": artifact_digest,
        "signerSha256": signer,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("product", choices=("com.nu54renter", "com.nu54kiosk"))
    parser.add_argument("channel", choices=("upload", "distribution", "custom"))
    parser.add_argument("artifact", type=Path)
    parser.add_argument("--bundletool", type=Path, default=os.environ.get("BUNDLETOOL_JAR"))
    args = parser.parse_args()
    try:
        print(json.dumps(verify_artifact(args.product, args.channel, args.artifact, args.bundletool), sort_keys=True))
    except (OSError, ValueError, ET.ParseError, subprocess.CalledProcessError) as error:
        print(f"Android release verification failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

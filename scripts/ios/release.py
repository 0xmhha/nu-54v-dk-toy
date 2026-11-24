#!/usr/bin/env python3
"""Build a native mobile release archive and verify its embedded signing identity."""

import argparse
from datetime import datetime, timezone
from fnmatch import fnmatchcase
import hashlib
import json
import os
from pathlib import Path
import plistlib
import re
import subprocess
import sys
import tempfile


HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]
PRODUCTS = {
    "p04": ("p04-merchant-kiosk", "NU54Kiosk", "release-signers.json", "NU54_IOS"),
    "p02": ("p02-user-app", "NU54Renter", "renter-release-signers.json", "NU54_P02_IOS"),
}


def workspace(product: str) -> tuple[Path, str]:
    directory, scheme, _, _ = PRODUCTS[product]
    return REPO / "products" / directory / "ios" / f"{scheme}.xcworkspace", scheme


def call(*command: str | Path) -> bytes:
    return subprocess.check_output(command)


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ValueError(message)


def sha256(path: Path) -> str:
    with path.open("rb") as source:
        return hashlib.file_digest(source, "sha256").hexdigest()


def identity(channel: str, product: str = "p04") -> dict:
    policy = json.loads((HERE / PRODUCTS[product][2]).read_text())
    if channel == "official":
        require(bool(re.fullmatch(r"[A-Z0-9]{10}", policy["team"] or "")), "Official iOS Team ID is not enrolled")
        for key in ("certificateSha256", "profileSha256"):
            require(bool(re.fullmatch(r"[a-f0-9]{64}", policy[key] or "")), f"Official iOS {key} is not enrolled")
    return policy


def verify_profile(channel: str, profile: dict, entitlements: dict, bundle: str, leaf: bytes) -> str:
    team = entitlements.get("com.apple.developer.team-identifier")
    require(isinstance(team, str) and team in profile.get("TeamIdentifier", []), "iOS Team ID is outside the profile")
    app_id = entitlements.get("application-identifier")
    allowed = profile.get("Entitlements", {})
    require(app_id == allowed.get("application-identifier") and app_id in {
        f"{prefix}.{bundle}" for prefix in profile.get("ApplicationIdentifierPrefix", [])
    }, "iOS app identifier does not match the profile and bundle")
    require(allowed.get("get-task-allow") is False and entitlements.get("get-task-allow", False) is False,
            "Debug entitlements are forbidden")
    require(leaf in profile.get("DeveloperCertificates", []), "iOS signing certificate is absent from the profile")
    expires = profile.get("ExpirationDate")
    require(isinstance(expires, datetime) and expires.replace(tzinfo=timezone.utc) > datetime.now(timezone.utc),
            "iOS provisioning profile is expired")
    require("iOS" in profile.get("Platform", []), "Provisioning profile is not for iOS")
    if channel == "official":
        require(not profile.get("ProvisionedDevices") and not profile.get("ProvisionsAllDevices"),
                "Official iOS builds need an App Store provisioning profile")
    allowed_names = {
        "application-identifier", "com.apple.developer.team-identifier", "get-task-allow",
        "keychain-access-groups", "beta-reports-active",
    }
    require(entitlements.keys() <= allowed_names, "Unexpected iOS signed entitlement")
    for key, value in entitlements.items():
        expected = allowed.get(key)
        if key == "keychain-access-groups":
            require(isinstance(value, list) and isinstance(expected, list) and all(
                group == app_id and any(fnmatchcase(group, pattern) for pattern in expected) for group in value
            ), "Unexpected iOS keychain access groups")
        else:
            require(value == expected, f"iOS entitlement does not match provisioning profile: {key}")
    return team


def verify_owner_registration_isolation(executable: Path) -> None:
    binary = executable.read_bytes()
    qa_markers = (
        b"nu54.owner.registration.w04qa.p256",
        b"com.nu54renter.w04qa",
        b"owner-registration-host",
        b"owner-registration-other",
        b"w04-host-port",
        b"w04-other-host-port",
    )
    require(not any(marker in binary for marker in qa_markers),
            "The W04 simulator registration adapter is present in the release executable")


def verify(channel: str, app: Path, policy: dict) -> dict:
    plist = plistlib.loads((app / "Info.plist").read_bytes())
    bundle = policy["bundle" if channel == "official" else "customBundle"]
    require(plist.get("CFBundleIdentifier") == bundle, f"Expected iOS bundle {bundle}")
    require(plist.get("DTPlatformName") == "iphoneos", "Expected a physical-device iOS build")
    if bundle == "com.nu54renter" or bundle.startswith("com.nu54renter."):
        require(plist.get("CFBundleExecutable") == "NU54Renter", "Unexpected renter iOS executable")
        verify_owner_registration_isolation(app / "NU54Renter")
    profile_path = app / "embedded.mobileprovision"
    profile_sha = sha256(profile_path)
    if channel == "official":
        require(profile_sha == policy["profileSha256"], "Unexpected official iOS provisioning profile")
    call("codesign", "--verify", "--strict", "--deep", "-R", f'=anchor apple generic and identifier "{bundle}"', app)
    entitlements = plistlib.loads(call("codesign", "--display", "--entitlements", "-", "--xml", app))
    profile = plistlib.loads(call("security", "cms", "-D", "-i", profile_path))
    with tempfile.TemporaryDirectory() as temp:
        prefix = Path(temp) / "certificate"
        call("codesign", "--display", f"--extract-certificates={prefix}", app)
        leaf = Path(f"{prefix}0").read_bytes()
    team = verify_profile(channel, profile, entitlements, bundle, leaf)
    certificate_sha = hashlib.sha256(leaf).hexdigest()
    if channel == "official":
        require(team == policy["team"] and certificate_sha == policy["certificateSha256"],
                "Unexpected official iOS Team ID or certificate")
    else:
        require(certificate_sha != policy["certificateSha256"], "Official iOS keys cannot sign custom builds")
    files = {str(path.relative_to(app)): sha256(path) for path in sorted(app.rglob("*")) if path.is_file()}
    return {
        "bundle": bundle, "channel": channel, "certificateSha256": certificate_sha, "team": team,
        "profileSha256": profile_sha, "profileUuid": profile["UUID"], "entitlements": entitlements, "files": files,
    }


def require_legal_notices(product: str) -> None:
    """The official app is given to the public, so its legal notices must be complete."""
    app = PRODUCTS[product][0]
    run = subprocess.run([sys.executable, "-B", REPO / "scripts/legal_notices.py", "--release", "--app", app],
                         stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    require(run.returncode == 0, f"External distribution is blocked.\n{run.stdout.strip()}")


def archive(channel: str, destination: Path, policy: dict, product: str = "p04") -> dict:
    project, scheme = workspace(product)
    require(not destination.exists() and destination.suffix == ".xcarchive", "Use a new .xcarchive path")
    prefix = f"{PRODUCTS[product][3]}_{channel.upper()}"
    profile = os.environ.get(f"{prefix}_PROFILE")
    team = policy["team"] if channel == "official" else os.environ.get(f"{prefix}_TEAM")
    require(bool(profile) and bool(re.fullmatch(r"[A-Z0-9]{10}", team or "")), f"Set {prefix} provisioning inputs")
    commit = call("git", "-C", REPO, "rev-parse", "HEAD").decode().strip()
    dirty = bool(call("git", "-C", REPO, "status", "--porcelain"))
    if channel == "official":
        require(not dirty, "Official iOS builds require a clean source tree")
        require_legal_notices(product)
    bundle = policy["bundle" if channel == "official" else "customBundle"]
    subprocess.run([
        "xcodebuild", "-workspace", project, "-scheme", scheme, "-configuration", "Release",
        "-destination", "generic/platform=iOS", "-archivePath", destination, "archive",
        f"NU54_IOS_BUNDLE={bundle}", f"NU54_IOS_TEAM={team}", f"NU54_IOS_PROFILE={profile}",
    ], stdout=sys.stderr, check=True)
    receipt = verify(channel, destination / "Products" / "Applications" / f"{scheme}.app", policy)
    return {**receipt, "sourceCommit": commit, "sourceDirty": dirty}


def build_simulator(destination: Path, policy: dict, product: str = "p04") -> dict:
    project, scheme = workspace(product)
    subprocess.run([
        "xcodebuild", "-workspace", project, "-scheme", scheme, "-configuration", "Release",
        "-destination", "generic/platform=iOS Simulator", "-derivedDataPath", destination,
        "CODE_SIGNING_ALLOWED=NO", "build",
    ], stdout=sys.stderr, check=True)
    app = destination / "Build" / "Products" / "Release-iphonesimulator" / f"{scheme}.app"
    info = plistlib.loads((app / "Info.plist").read_bytes())
    require(info.get("CFBundleIdentifier") == policy["customBundle"], "Unexpected simulator bundle identifier")
    require(info.get("DTPlatformName") == "iphonesimulator", "Expected an iOS simulator build")
    if product == "p02":
        verify_owner_registration_isolation(app / info["CFBundleExecutable"])
    return {
        "channel": "custom-simulator", "bundle": policy["customBundle"],
        "executableSha256": sha256(app / info["CFBundleExecutable"]),
        "javascriptSha256": sha256(app / "main.jsbundle"),
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("archive", "verify", "simulator"))
    parser.add_argument("channel", choices=("official", "custom"))
    parser.add_argument("path", type=Path)
    parser.add_argument("--product", choices=tuple(PRODUCTS), default="p04")
    args = parser.parse_args()
    try:
        policy = identity(args.channel, args.product)
        path = args.path.resolve()
        if args.action == "archive":
            receipt = archive(args.channel, path, policy, args.product)
        elif args.action == "verify":
            receipt = verify(args.channel, path, policy)
        else:
            require(args.channel == "custom", "Simulator builds use the custom channel")
            receipt = build_simulator(path, policy, args.product)
        print(json.dumps({**receipt, "product": args.product}, indent=2, sort_keys=True))
    except (OSError, ValueError, plistlib.InvalidFileException, subprocess.CalledProcessError) as error:
        print(f"iOS release failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

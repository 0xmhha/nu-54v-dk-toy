#!/usr/bin/env python3
"""Join the isolated Android and iOS UI fixtures for one P05 subscription preview."""
import argparse
import json
import os
from pathlib import Path
import time

from run import atomic_json, await_file


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("lab", type=Path, help="P05 settlement fixture")
    parser.add_argument("android", type=Path, help="Android preview fixture")
    parser.add_argument("ios", type=Path, help="single iOS installation preview fixture")
    args = parser.parse_args()
    os.umask(0o077)
    lab = args.lab.resolve()
    android = args.android.resolve()
    ios = args.ios.resolve()
    if len({lab, android, ios}) != 3:
        parser.error("use three separate private fixture directories")
    for folder in (lab, android, ios):
        folder.mkdir(mode=0o700, parents=True, exist_ok=True)
        if folder.stat().st_mode & 0o077 or any((folder / marker).exists()
                                               for marker in ("preview.json", "ui-completed", "ui-aborted")):
            parser.error("use fresh fixture directories with mode 0700")
    try:
        profiles = {}
        for role, folder in (("android", android), ("ios", ios)):
            keys = json.loads(await_file(folder / "public-keys.json", timeout=660))
            if set(keys) != {role}:
                raise ValueError(f"expected one {role} installation")
            profiles.update(keys)
        if len(set(profiles.values())) != 2:
            raise ValueError("the two native installations must have independent keys")
        atomic_json(lab / "public-keys.json", profiles)
        preview = json.loads(await_file(lab / "preview.json", timeout=660))
        for role, folder in (("android", android), ("ios", ios)):
            atomic_json(folder / "preview.json", {**preview, "profiles": {role: preview["profiles"][role]}})
        deadline = time.monotonic() + 175
        while time.monotonic() < deadline:
            if any((folder / "ui-aborted").exists() for folder in (android, ios, lab)):
                raise RuntimeError("one of the native UI checks failed")
            if all((folder / "ui-completed").exists() for folder in (android, ios)):
                break
            time.sleep(0.25)
        else:
            raise RuntimeError("mixed native preview did not complete")
        (lab / "ui-completed").touch()
        counts = json.loads(await_file(lab / "read-only.json"))
        if counts["enrolledNativeInstallations"] != 2 or counts["state"] != "preview" or any(
            counts[name] != 0 for name in ("claims", "quotes", "submissions", "payments")
        ):
            raise RuntimeError("the two native readbacks did not preserve one untouched preview")
        print("PASS Android and iOS retained the same untouched subscription preview")
    except Exception:
        for folder in (lab, android, ios):
            (folder / "ui-aborted").touch()
        raise
    finally:
        for folder in (android, ios):
            (folder / "preview.json").unlink(missing_ok=True)


if __name__ == "__main__":
    main()

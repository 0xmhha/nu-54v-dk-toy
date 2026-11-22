#!/usr/bin/env python3
"""Capture selected SDK materials in a disposable local clone without making commits."""
import argparse
import hashlib
import os
import subprocess
from pathlib import Path

from evidence import require

ROOT = Path(__file__).resolve().parents[3]
MATERIALS = (
    ".github/inputs.json", ".npmrc", "LICENSE", "package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml",
    "go.work", "go.work.sum", "pyproject.toml", "uv.lock",
    "scripts/sdk_provenance.py", "scripts/verify_ci_inputs.py",
    "docs/content/specifications/protocol", "packages/protocol", "packages/contracts-abi",
    "packages/device-sim/package.json", "products/p02-user-app/package.json",
    "products/p04-merchant-kiosk/package.json", "products/p05-operations-backoffice/web/package.json",
    "products/p05-operations-backoffice/go.mod", "products/p05-operations-backoffice/go.sum",
    "products/p07-indexer/go.mod", "products/p07-indexer/go.sum",
    "products/p01-device-firmware/core", "products/p01-device-firmware/third_party/keccak",
    "products/p01-device-firmware/third_party/secp256k1",
    "products/p06-stablenet-contracts/src", "products/p06-stablenet-contracts/test",
    "products/p06-stablenet-contracts/script", "products/p06-stablenet-contracts/foundry.toml",
    "products/p06-stablenet-contracts/remappings.txt", "products/p10-platform/release",
)


def capture(destination):
    require(not destination.exists() and not destination.is_relative_to(ROOT), "capture target must be a new external path")
    head = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
    subprocess.run(["git", "clone", "--quiet", "--shared", "--no-tags", str(ROOT), str(destination)], check=True)
    actual = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=destination, text=True).strip()
    require(head == actual, "source commit changed during capture")
    listed = subprocess.check_output(
        ["git", "ls-files", "-z", "-c", "-o", "--exclude-standard", "--", *MATERIALS], cwd=ROOT,
    )
    expected = {}
    for relative in sorted(set(filter(None, listed.split(b"\0")))):
        name = Path(os.fsdecode(relative))
        path = ROOT / name
        if not path.exists():
            # An intentionally removed material stays removed in the snapshot.
            target = destination / name
            if target.is_file():
                target.unlink()
            expected[name] = None
            continue
        require(path.is_file() and not path.is_symlink(), f"non-regular SDK material {name}")
        body = path.read_bytes()
        target = destination / name
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(body)
        target.chmod(path.stat().st_mode & 0o777)
        expected[name] = hashlib.sha256(body).hexdigest()
    for name, digest in expected.items():
        source = ROOT / name
        observed = hashlib.sha256(source.read_bytes()).hexdigest() if source.exists() else None
        require(observed == digest, f"SDK material changed while capturing: {name}")
    print(f"Captured {len(expected)} selected SDK material files at base {head}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("destination", type=Path)
    capture(parser.parse_args().destination.resolve())

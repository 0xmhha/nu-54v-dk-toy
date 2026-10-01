#!/usr/bin/env python3
"""Run every implementation's check against the shared EIP-712 vectors (WBS2-P10-02, [N21]).

Each check is a command that exits 0 only if that implementation reproduces the vectors.
A check is SKIP, not FAIL, when a tool it needs is not installed or when the implementation
it tests does not exist yet. The kiosk and firmware checks run once their teams add the
entry points described in README.md next to this file.

Usage:
    python3 products/p10-platform/harness/run_conformance.py            SKIP does not fail
    python3 products/p10-platform/harness/run_conformance.py --strict   SKIP fails too (gate use)
"""

from __future__ import annotations

import json
import shutil
import subprocess
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Callable

ROOT = Path(__file__).resolve().parents[3]
KIOSK = ROOT / "products/p04-merchant-kiosk"
FIRMWARE = ROOT / "products/p01-device-firmware"
FIRMWARE_TEST = FIRMWARE / "test/test_eip712.c"
FIRMWARE_BUILD = "out/conformance"


def kiosk_has_conformance_script() -> str | None:
    """None when ready, otherwise the reason to skip."""
    scripts = json.loads((KIOSK / "package.json").read_text()).get("scripts", {})
    return None if "test:conformance" in scripts else "kiosk has no test:conformance script yet"


def firmware_has_eip712_test() -> str | None:
    return None if FIRMWARE_TEST.exists() else "firmware has no test/test_eip712.c yet"


@dataclass
class Check:
    name: str
    cmd: list[str]
    cwd: str
    tools: list[str] = field(default_factory=list)
    ready: Callable[[], str | None] = lambda: None
    # Commands run before cmd in the same directory (for example a build step).
    before: list[list[str]] = field(default_factory=list)


CHECKS = [
    Check("vectors reproducible (cast)", ["python3", "docs/content/specifications/protocol/build_eip712_vectors.py", "--check"], ".", ["cast"]),
    # The generator formats Go output with gofmt; without it the Go file would look stale.
    Check("generated bindings match schema", ["python3", "schema-gen/generate.py", "--check"], "packages/protocol", ["gofmt"]),
    Check("protocol Go encodeType and vector signers by role", ["go", "test", "./..."], "packages/protocol/go", ["go"]),
    Check("protocol TS encodeType", ["pnpm", "test"], "packages/protocol/ts", ["pnpm"]),
    Check("protocol Python encodeType", ["uv", "run", "--quiet", "--package", "nu54-protocol", "pytest", "-q", "python/tests"], "packages/protocol", ["uv"]),
    Check("operations tool Go digests", ["go", "test", "./internal/core/..."], "products/p05-operations-backoffice", ["go"]),
    Check("CBOR message vectors round-trip", ["python3", "cbor-gen/generate.py", "--check"], "packages/protocol"),
    Check("BLE fragment vectors reassemble", ["python3", "frame-gen/generate.py", "--check"], "packages/protocol"),
    Check("contract Solidity typehashes", ["forge", "test", "--match-contract", "PaymentTypesTest"], "products/p06-stablenet-contracts", ["forge"]),
    Check("contract Solidity EIP-712 digest and signer", ["forge", "test", "--match-contract", "Eip712VectorsTest"], "products/p06-stablenet-contracts", ["forge"]),
    Check("kiosk TypeScript EIP-712, CBOR and fragments", ["pnpm", "run", "test:conformance"], "products/p04-merchant-kiosk", ["pnpm"], kiosk_has_conformance_script),
    Check(
        "firmware C EIP-712, signatures and fragments (host)",
        ["ctest", "--test-dir", FIRMWARE_BUILD, "-R", "eip712_vectors|frame_vectors", "--output-on-failure", "--no-tests=error"],
        "products/p01-device-firmware",
        ["cmake", "ctest"],
        firmware_has_eip712_test,
        [["cmake", "-S", "test", "-B", FIRMWARE_BUILD], ["cmake", "--build", FIRMWARE_BUILD]],
    ),
]


def run(check: Check) -> tuple[str, str]:
    """Returns (status, detail) with status PASS, FAIL or SKIP."""
    missing = [t for t in check.tools if shutil.which(t) is None]
    if missing:
        return "SKIP", f"tool not installed: {', '.join(missing)}"
    reason = check.ready()
    if reason:
        return "SKIP", reason
    for cmd in [*check.before, check.cmd]:
        proc = subprocess.run(cmd, cwd=ROOT / check.cwd, capture_output=True, text=True)
        if proc.returncode != 0:
            return "FAIL", (proc.stdout + proc.stderr)[-2000:]
    return "PASS", ""


def main(argv: list[str]) -> int:
    strict = "--strict" in argv
    counts = {"PASS": 0, "FAIL": 0, "SKIP": 0}
    for check in CHECKS:
        status, detail = run(check)
        counts[status] += 1
        print(f"{status}  {check.name}" + (f"  ({detail})" if status == "SKIP" else ""))
        if status == "FAIL":
            print(detail)
    print(f"{counts['PASS']} passed, {counts['FAIL']} failed, {counts['SKIP']} skipped of {len(CHECKS)}")
    if counts["FAIL"] or (strict and counts["SKIP"]):
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))

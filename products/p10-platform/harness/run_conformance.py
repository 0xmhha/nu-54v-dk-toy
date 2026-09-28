#!/usr/bin/env python3
"""Run every implementation's check against the shared EIP-712 vectors (WBS2-P10-02, [N21]).

Each entry is a command that exits 0 only if that implementation reproduces the
vectors. New implementations (firmware C, kiosk TS signer) are added here as the
WBS tasks land.

Usage:
    python3 products/p10-platform/harness/run_conformance.py
"""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]

CHECKS = [
    ("vectors reproducible (cast)", ["python3", "docs/content/specifications/protocol/build_eip712_vectors.py", "--check"], "."),
    ("generated bindings match schema", ["python3", "schema-gen/generate.py", "--check"], "packages/protocol"),
    ("protocol Go encodeType", ["go", "test", "./..."], "packages/protocol/go"),
    ("protocol TS encodeType", ["pnpm", "test"], "packages/protocol/ts"),
    ("protocol Python encodeType", ["uv", "run", "--quiet", "--package", "nu54-protocol", "pytest", "-q", "python/tests"], "packages/protocol"),
    ("P05 Go core digests", ["go", "test", "./internal/core/..."], "products/p05-operations-backoffice"),
    ("P06 Solidity typehashes", ["forge", "test", "--match-contract", "PaymentTypesTest"], "products/p06-stablenet-contracts"),
    ("P06 Solidity EIP-712 digest and signer", ["forge", "test", "--match-contract", "Eip712VectorsTest"], "products/p06-stablenet-contracts"),
]


def main() -> int:
    failed = 0
    for name, cmd, cwd in CHECKS:
        proc = subprocess.run(cmd, cwd=ROOT / cwd, capture_output=True, text=True)
        ok = proc.returncode == 0
        failed += not ok
        print(f"{'PASS' if ok else 'FAIL'}  {name}")
        if not ok:
            print((proc.stdout + proc.stderr)[-2000:])
    print(f"{len(CHECKS) - failed}/{len(CHECKS)} checks passed")
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())

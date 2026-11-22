#!/usr/bin/env python3
"""Compare compiled contract interfaces and storage with the pre-upgrade baseline."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
if __name__ == "__main__":
    sys.dont_write_bytecode = True

from integration_workspace import run_command, scoped_workspace

from install_contract_compiler import VERSION

ROOT = Path(__file__).resolve().parents[1]
BASELINE = ROOT / "products/p06-stablenet-contracts/compiler-compatibility.json"
TARGETS = {
    "p06": (
        ROOT / "products/p06-stablenet-contracts",
        ("MerchantRegistry", "PaymentSettlement", "SettlementV2", "TestUSDC"),
    ),
    "p08": (
        ROOT / "products/p08-market-services",
        ("Market", "LabToken"),
    ),
}
# solc assigns numeric AST IDs to declarations and embeds them in some type keys.
TYPE_ID = re.compile(r"(t_(?:contract|enum|struct|userDefinedValueType)\([^()]+\))\d+")


def normalize_storage(value):
    if isinstance(value, dict):
        return {
            TYPE_ID.sub(r"\1", key): normalize_storage(item)
            for key, item in value.items()
            if key != "astId"
        }
    if isinstance(value, list):
        return [normalize_storage(item) for item in value]
    if isinstance(value, str):
        return TYPE_ID.sub(r"\1", value)
    return value


def canonical_json(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"))


def digest(value):
    return hashlib.sha256(canonical_json(value).encode()).hexdigest()


def fingerprints(product, environment=None):
    project, names = TARGETS[product]
    if environment is None:
        environment = os.environ
    with scoped_workspace("nu54-solidity-compatibility-") as scratch:
        out = scratch / "out"
        run_command(
            [sys.executable, "-B", str(ROOT / "scripts/contract_forge.py"), product, "build", "--quiet", "--no-cache",
             "--skip", "test", "--skip", "script", "--extra-output", "storageLayout"],
            cwd=project, check=True, env={**environment, "FOUNDRY_OUT": str(out), "FOUNDRY_CACHE_PATH": str(scratch / "cache")},
        )
        result = {}
        for name in names:
            artifact = json.loads((out / f"{name}.sol" / f"{name}.json").read_text())
            metadata = artifact["metadata"]
            settings = metadata["settings"]
            if (
                metadata["compiler"]["version"] != VERSION
                or settings["evmVersion"] != "prague"
                or settings["optimizer"] != {"enabled": True, "runs": 200}
                or settings.get("viaIR", False)
            ):
                raise ValueError(f"{product}/{name}: unexpected compiler version or settings")
            result[name] = {
                "abiSha256": digest(sorted(artifact["abi"], key=canonical_json)),
                "storageLayoutSha256": digest(normalize_storage(artifact["storageLayout"])),
            }
        return result


def main(argv=None, environment=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("products", nargs="+", choices=TARGETS)
    args = parser.parse_args(argv)
    baseline = json.loads(BASELINE.read_text())
    differences = []
    try:
        for product in args.products:
            for contract, fields in fingerprints(product, environment).items():
                for field, observed in fields.items():
                    expected = baseline["products"][product][contract][field]
                    if observed != expected:
                        differences.append(f"{product}/{contract} {field}: {observed} != {expected}")
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        parser.exit(1, f"{error}\n")
    if differences:
        parser.exit(1, "\n".join(differences) + "\n")
    print(f"ABI and storage compatibility verified for {', '.join(args.products)}", flush=True)


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""Regenerate SDK outputs in check mode and record their inputs and hashes."""

import hashlib
import json
from pathlib import Path
import runpy
import subprocess
import sys


ROOT = Path(__file__).resolve().parents[1]
PROTOCOL = ROOT / "packages/protocol"
ABI = ROOT / "packages/contracts-abi"
VECTORS = ROOT / "docs/content/specifications/protocol"
CONTRACTS = ROOT / "products/p06-stablenet-contracts"
RELEASE = ROOT / "products/p10-platform/release"
FIRMWARE = ROOT / "products/p01-device-firmware"
CONSUMER_SOURCES = (
    "quote.go",
    "quote.mts",
    "quote.py",
    "android-protected/Probe.java",
    "ios-profile/SceneSmoke.swift",
)


def output(*command: str | Path) -> str:
    return subprocess.check_output(command, cwd=ROOT, text=True).strip()


def hashes(paths: list[Path]) -> dict[str, str]:
    return {str(path.relative_to(ROOT)): hashlib.sha256(path.read_bytes()).hexdigest() for path in sorted(set(paths))}


def main() -> int:
    schema, cbor, frame = (PROTOCOL / dir / "generate.py" for dir in ("schema-gen", "cbor-gen", "frame-gen"))
    abigen = ABI / "tools/generate.py"
    eip712 = VECTORS / "build_eip712_vectors.py"
    source_clean = not bool(output("git", "status", "--porcelain"))
    v2 = PROTOCOL / "v2"
    quote = v2 / "generate.py"
    service = v2 / "generate_services.py"
    for generator in (schema, eip712, cbor, frame, abigen, quote, service):
        subprocess.run([sys.executable, generator, "--check"], cwd=ROOT, stdout=sys.stderr, check=True)

    schema_def = runpy.run_path(str(schema))
    cbor_def = runpy.run_path(str(cbor))
    frame_def = runpy.run_path(str(frame))
    abi_def = runpy.run_path(str(abigen))
    abi_outputs = [ABI / "abi/manifest.json", ABI / "ts/src/index.ts"]
    for contract, package, identifier, _ in abi_def["TARGETS"]:
        abi_outputs.extend([ABI / f"abi/{contract}.json", ABI / f"go/{package}/{package}.go", ABI / f"ts/src/{identifier}.ts"])
    compiler_version = json.loads(output(
        sys.executable, ROOT / "scripts/contract_forge.py", "p06", "inspect", "SettlementV2", "metadata", "--json",
    ))["compiler"]["version"]
    result = {
        "sourceCommit": output("git", "rev-parse", "HEAD"),
        "sourceClean": source_clean,
        "tools": {
            "python": sys.version.split()[0],
            "go": output("go", "version"),
            "forge": output("forge", "--version").splitlines()[0],
            "cast": output("cast", "--version").splitlines()[0],
            "solc": compiler_version.split("+", 1)[0],
            "solcBuild": compiler_version,
            "abigen": abi_def["ABIGEN"],
            "abigenV2": abi_def["ABIGEN_V2"],
            "uv": output("uv", "--version"),
            "node": output("node", "--version"),
            "npm": output("npm", "--version"),
            "pnpm": output("pnpm", "--version"),
            "typescript": output("pnpm", "--dir", PROTOCOL / "ts", "exec", "tsc", "--version"),
            "cc": output("cc", "--version").splitlines()[0],
        },
        "inputs": hashes([
            schema, eip712, cbor, frame, abigen, quote, service, schema_def["SCHEMA"],
            *v2.glob("*.json"), v2 / "testdata/quote.vectors", *PROTOCOL.glob("c/include/*.h"),
            *PROTOCOL.glob("c/src/*.c"), *PROTOCOL.glob("ts/src/*.ts"),
            ROOT / "package.json", ROOT / "pnpm-lock.yaml", ROOT / "pnpm-workspace.yaml",
            ROOT / "go.work", ROOT / "go.work.sum", ROOT / "uv.lock", ROOT / "LICENSE",
            PROTOCOL / "go/go.mod", PROTOCOL / "go/go.sum", *PROTOCOL.glob("go/**/*.go"),
            PROTOCOL / "python/pyproject.toml", *PROTOCOL.glob("python/nu54_protocol/*.py"),
            *(RELEASE / "consumers" / path for path in CONSUMER_SOURCES), PROTOCOL / "v2/testdata/test-c.sh",
            PROTOCOL / "c/test/quote_v2_test.c", PROTOCOL / "c/test/test_transport_v2.c",
            FIRMWARE / "core/src/nu54_keccak.c", FIRMWARE / "core/src/nu54_sig.c",
            FIRMWARE / "core/include/nu54_keccak.h", FIRMWARE / "core/include/nu54_sig.h",
            *FIRMWARE.glob("third_party/keccak/*.c"), *FIRMWARE.glob("third_party/secp256k1/include/*.h"),
            *FIRMWARE.glob("third_party/secp256k1/src/**/*.h"),
            *(FIRMWARE / f"third_party/secp256k1/src/{name}.c"
              for name in ("secp256k1", "precomputed_ecmult", "precomputed_ecmult_gen")),
            ROOT / "scripts/sdk_provenance.py", ROOT / "scripts/verify_ci_inputs.py",
            ROOT / "scripts/install_contract_compiler.py", ROOT / "scripts/contract_forge.py",
            ROOT / ".github/inputs.json",
            ROOT / "products/p10-platform/release/build_sdk.py",
            ROOT / "products/p10-platform/release/evidence.py",
            PROTOCOL / "ts/package.json", PROTOCOL / "ts/tsconfig.json", *PROTOCOL.glob("ts/src/**/*.ts"),
            ABI / "go/go.mod", ABI / "go/go.sum", CONTRACTS / "foundry.toml", CONTRACTS / "remappings.txt",
            *CONTRACTS.glob("src/**/*.sol"),
        ]),
        "outputs": hashes([
            *(PROTOCOL / path for path in schema_def["TARGETS"]), VECTORS / "eip712-vectors.json", cbor_def["OUT"], frame_def["OUT"], *abi_outputs,
            *runpy.run_path(str(quote))["generate"](),
            *runpy.run_path(str(service))["CHOICES"],
        ]),
    }
    print(json.dumps(result, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

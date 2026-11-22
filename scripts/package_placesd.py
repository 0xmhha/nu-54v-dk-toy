#!/usr/bin/env python3
"""Build an unsigned placesd Linux candidate, input manifest and binary SBOM."""
import hashlib
import json
import os
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[1]
MODULE = ROOT / "products/p09-travel-ai"


def checksum(path: Path):
    with path.open("rb") as source:
        return hashlib.file_digest(source, "sha256").hexdigest()


def main():
    output = os.getenv("PLACES_OUT")
    arch = os.environ.get("PLACES_ARCH", "amd64")
    if not output or arch not in {"amd64", "arm64"}:
        raise SystemExit("Set PLACES_OUT to an empty path and PLACES_ARCH to amd64 or arm64")
    out = Path(output).resolve()
    out.mkdir(parents=True, exist_ok=True)
    if any(out.iterdir()):
        raise SystemExit("PLACES_OUT must be empty")
    env = {**os.environ, "GOWORK": "off", "GOTOOLCHAIN": "local", "CGO_ENABLED": "0",
           "GOOS": "linux", "GOARCH": arch, "GOFLAGS": "", "GOAMD64": "v1", "GOARM64": "v8.0"}
    subprocess.run(["python3", "scripts/verify_ci_inputs.py", "go"], cwd=ROOT, env=env, check=True)
    binary = out / "placesd"
    subprocess.run(["go", "build", "-mod=readonly", "-trimpath", "-buildvcs=false", "-o", binary, "./cmd/placesd"],
                   cwd=MODULE, env=env, check=True)
    modules = json.loads(subprocess.check_output(["go", "version", "-m", "-json", binary], cwd=MODULE, env=env))
    inputs = [MODULE / name for name in ("go.mod", "go.sum", "Dockerfile", "Dockerfile.dockerignore")]
    inputs += [path for directory in ("cmd", "internal", "migrations") for path in (MODULE / directory).rglob("*")
               if path.suffix in {".go", ".sql"} and not path.name.endswith("_test.go")]
    inputs += [ROOT / "scripts/package_placesd.py", ROOT / ".github/inputs.json"]
    manifest = {
        "format": "nu54-placesd-release-v1",
        "target": {"os": "linux", "arch": arch, "toolchain": modules["GoVersion"]},
        "sourceSHA256": {str(path.relative_to(ROOT)): checksum(path) for path in sorted(inputs)},
        "binarySHA256": checksum(binary),
        "migrations": "apply the independently authorized P09 migrations before running placesd",
    }
    deps = [{"type": "library", "name": "stdlib", "version": modules["GoVersion"]}]
    deps += [{"type": "library", "name": item["Path"], "version": item["Version"],
              "purl": f"pkg:golang/{item['Path']}@{item['Version']}"} for item in modules["Deps"]]
    sbom = {
        "bomFormat": "CycloneDX", "specVersion": "1.6", "version": 1,
        "metadata": {"component": {"type": "application", "name": "placesd",
                     "hashes": [{"alg": "SHA-256", "content": manifest["binarySHA256"]}]}},
        "components": deps,
    }
    for name, document in (("provenance.json", manifest), ("placesd.cdx.json", sbom)):
        (out / name).write_text(json.dumps(document, sort_keys=True, indent=2) + "\n")
    names = ("placesd", "placesd.cdx.json", "provenance.json")
    (out / "SHA256SUMS").write_text("".join(f"{checksum(out / name)}  {name}\n" for name in names))


if __name__ == "__main__":
    main()

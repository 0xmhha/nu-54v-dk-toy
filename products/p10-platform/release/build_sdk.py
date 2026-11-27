#!/usr/bin/env python3
"""Build reproducible, installable Protocol SDK and shared vector candidates."""

import argparse
import io
import json
import os
import re
import subprocess
import sys
import tarfile
import tempfile
import tomllib
import zipfile
from pathlib import Path

from evidence import authenticate, deterministic_tar, digest, key_sha256, openssl, require, verify

ROOT = Path(__file__).resolve().parents[3]
PROTOCOL = ROOT / "packages/protocol"
CONSUMERS = Path(__file__).with_name("consumers")


def run(args, cwd=ROOT, capture=False, env=None):
    result = subprocess.run(args, cwd=cwd, stdout=subprocess.PIPE if capture else sys.stderr,
                            env={**os.environ, **(env or {})}, check=True, text=True)
    return result.stdout


def write(path, object_):
    path.write_text(json.dumps(object_, indent=2, sort_keys=True) + "\n")


def schemas(prefix):
    root = PROTOCOL / "v2"
    published = [*root.glob("*.json"), root / "testdata/quote.vectors"]
    return {f"{prefix}/spec-v2/{path.relative_to(root).as_posix()}": path.read_bytes() for path in published}


def go_proxy_files(version):
    module = (PROTOCOL / "go/go.mod").read_text().splitlines()[0].removeprefix("module ")
    root = f"{module}@{version}"
    files = {f"{root}/LICENSE": (ROOT / "LICENSE").read_bytes(), **schemas(root)}
    source = PROTOCOL / "go"
    for path in sorted(source.rglob("*.go")):
        if not path.name.endswith("_test.go"):
            files[f"{root}/{path.relative_to(source).as_posix()}"] = path.read_bytes()
    for name in ("go.mod", "go.sum"):
        files[f"{root}/{name}"] = (source / name).read_bytes()
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w") as archive:
        for name, value in sorted(files.items()):
            entry = zipfile.ZipInfo(name, date_time=(1980, 1, 1, 0, 0, 0))
            entry.external_attr = 0o100644 << 16
            archive.writestr(entry, value, compress_type=zipfile.ZIP_DEFLATED)
    base = f"{module}/@v"
    return {
        f"{base}/list": f"{version}\n".encode(),
        f"{base}/{version}.mod": files[f"{root}/go.mod"],
        f"{base}/{version}.info": (json.dumps({"Version": version, "Time": "1980-01-01T00:00:00Z"}) + "\n").encode(),
        f"{base}/{version}.zip": output.getvalue(),
    }


def go_sdk(output, version):
    deterministic_tar(output, go_proxy_files(f"v{version}"))


def python_sdk(out, version):
    project = PROTOCOL / "python"
    metadata = tomllib.loads((project / "pyproject.toml").read_text())
    require(metadata["project"]["version"] == version, "Python SDK version disagrees")
    with tempfile.TemporaryDirectory(prefix="nu54-wheel-src-") as directory:
        stage = Path(directory)
        selected = {
            "pyproject.toml": (project / "pyproject.toml").read_bytes(),
            "LICENSE": (ROOT / "LICENSE").read_bytes(),
        }
        for path in sorted((project / "nu54_protocol").glob("*.py")):
            selected[f"nu54_protocol/{path.name}"] = path.read_bytes()
        for name, content in schemas("nu54_protocol").items():
            selected[name.replace("/spec-v2/", "/spec_v2/")] = content
        for name, content in selected.items():
            path = stage / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(content)
        run(["uv", "build", "--wheel", "--force-pep517", "--no-create-gitignore", "--out-dir", str(out), str(stage)],
            env={"SOURCE_DATE_EPOCH": "315532800"})
    wheel = out / f"nu54_protocol-{version}-py3-none-any.whl"
    require(wheel.is_file(), "expected wheel was not produced")
    return wheel


def node_sdk(output, metadata):
    with tempfile.TemporaryDirectory(prefix="nu54-sdk-ts-") as directory:
        out = Path(directory)
        run(
            ["pnpm", "exec", "tsc", "src/index.ts", "src/transport-v2-node.ts", "--strict", "--declaration", "--rewriteRelativeImportExtensions",
             "--module", "NodeNext", "--moduleResolution", "NodeNext", "--target", "ES2022", "--types", "node",
             "--skipLibCheck", "--rootDir", "src", "--outDir", str(out)],
            PROTOCOL / "ts",
        )
        members = {"package/LICENSE": (ROOT / "LICENSE").read_bytes(), **schemas("package")}
        for path in out.rglob("*"):
            if not path.is_file():
                continue
            content = path.read_bytes()
            if path.name.endswith(".d.ts"):
                # TS rewrites JS specifiers only; declaration imports also need the shipped .js names.
                content = re.sub(rb'(["\'])(\.{1,2}/[^"\']+)\.ts\1', rb'\1\2.js\1', content)
            members[f"package/dist/{path.relative_to(out).as_posix()}"] = content
        require("package/dist/index.js" in members and "package/dist/index.d.ts" in members, "SDK entry points absent")
        package = {key: metadata[key] for key in ("name", "version", "license", "type", "dependencies")}
        package.update(main="./dist/index.js", types="./dist/index.d.ts",
                       exports={
                           ".": {"types": "./dist/index.d.ts", "import": "./dist/index.js"},
                           "./v2": {"types": "./dist/v2.d.ts", "import": "./dist/v2.js"},
                           "./v2/node": {"types": "./dist/transport-v2-node.d.ts", "import": "./dist/transport-v2-node.js"},
                           "./spec-v2/*": "./spec-v2/*",
                       })
        members["package/package.json"] = (json.dumps(package, indent=2, sort_keys=True) + "\n").encode()
        deterministic_tar(output, members)


def c_sdk(output):
    members = {"nu54-protocol/LICENSE": (ROOT / "LICENSE").read_bytes(), **schemas("nu54-protocol")}
    for path in sorted((PROTOCOL / "c").glob("*/*")):
        if path.suffix in {".h", ".c"}:
            members[f"nu54-protocol/{path.relative_to(PROTOCOL / 'c').as_posix()}"] = path.read_bytes()
    require(len(members) > 1, "SDK headers absent")
    deterministic_tar(output, members)


def unpack(output, folder):
    folder.mkdir()
    with tarfile.open(output, "r:gz") as tar:
        for member in tar.getmembers():
            name = Path(member.name)
            require(member.isfile() and not name.is_absolute() and ".." not in name.parts,
                    "unexpected SDK archive entry")
            path = folder / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(tar.extractfile(member).read())


def sbom(name, version, components):
    return {
        "bomFormat": "CycloneDX", "specVersion": "1.6", "version": 1,
        "metadata": {"component": {"name": name, "type": "library", "version": version}},
        "components": components,
    }


def go_consumer(output, scratch, version, dependency_proxy=None, collect_inventory=True):
    folder = scratch / "go"
    folder.mkdir()
    proxy = scratch / "go-proxy"
    unpack(output, proxy)
    module = (PROTOCOL / "go/go.mod").read_text().splitlines()[0].removeprefix("module ")
    module_version = f"v{version}"
    (folder / "go.mod").write_text(f"module nu54-external-consumer\n\ngo 1.26.0\n\nrequire {module} {module_version}\n")
    (folder / "main.go").write_bytes((CONSUMERS / "quote.go").read_bytes())
    environment = {
        "GOWORK": "off", "GOFLAGS": "", "GOPRIVATE": "", "GONOPROXY": "none",
        "GOMODCACHE": str(scratch / "modcache"), "CGO_ENABLED": "0",
        "GONOSUMDB": module, "GOPROXY": f"{proxy.as_uri()},https://proxy.golang.org",
    }
    if dependency_proxy:
        environment.update(GOPROXY=f"{proxy.as_uri()},{dependency_proxy.as_uri()}",GOSUMDB="off",GOTOOLCHAIN="local")
    # Read from the local distribution first. Third-party modules keep the normal Go checksum DB.
    info = json.loads(run(["go", "mod", "download", "-json", f"{module}@{module_version}"],
                          folder, capture=True, env=environment))
    require(info["Version"] == module_version and info["Sum"].startswith("h1:") and
            info["GoModSum"].startswith("h1:"), "Go module checksums absent")
    local_zip = proxy / module / "@v" / f"{module_version}.zip"
    require(digest(Path(info["Zip"])) == digest(local_zip), "Go did not install the candidate ZIP")
    run(["go", "mod", "tidy"], folder, env=environment)
    run(["go", "build", "-trimpath", "-mod=readonly", "-o", "check", "."], folder, env=environment)
    output_text = run(["./check", f"{info['Dir']}/spec-v2/testdata/quote.vectors"], folder, capture=True).strip()
    sums = (folder / "go.sum").read_text()
    require(f"{module} {module_version} {info['Sum']}" in sums and
            f"{module} {module_version}/go.mod {info['GoModSum']}" in sums, "consumer go.sum mismatch")
    components=[]
    if collect_inventory:
        resolved = run(["go", "list", "-m", "-f", "{{.Path}} {{.Version}}", "all"],
                       folder, capture=True, env=environment).splitlines()
        components = [{"type": "library", "name": name, "version": v}
                      for name, v in (row.split() for row in resolved[1:])]
    return sbom("nu54-protocol-go", version, components), {
        "checks": [output_text, "GOWORK=off, Go proxy direct isolated import, and go.sum match"],
        "modulePath": module, "moduleSum": info["Sum"], "goModSum": info["GoModSum"],
    }, folder


def python_consumer(output, scratch, version, wheels=None):
    folder = scratch / "python"
    folder.mkdir()
    env = folder / "venv"
    offline={"UV_OFFLINE":"true","UV_CACHE_DIR":str(folder/"uv-cache")} if wheels else None
    run(["uv", "venv", "--python", sys.executable, str(env)], folder, env=offline)
    interpreter = env / "bin/python"
    requirements = folder / "requirements.lock"
    locked = run(["uv", "export", "--package", "nu54-protocol", "--extra", "quote-v2", "--no-dev",
                  "--no-emit-project", "--locked"], capture=True, env=offline)
    requirements.write_text(locked + f"\nnu54-protocol[quote-v2] @ {output.as_uri()} --hash=sha256:{digest(output)}\n")
    install=["uv","pip","install","--python",str(interpreter),"--require-hashes","-r",str(requirements)]
    if wheels:
        install.extend(("--no-index","--find-links",str(wheels)))
    run(install,folder,env=offline)
    (folder / "check.py").write_bytes((CONSUMERS / "quote.py").read_bytes())
    observed = json.loads(run([str(interpreter), "-I", "check.py"], folder, capture=True))
    installed = {name: v for name, v in observed["installed"].items() if name != "nu54-protocol"}
    (folder / "requirements.txt").write_text("".join(f"{name}=={v}\n" for name, v in sorted(installed.items())))
    components = [{"type": "library", "name": name, "version": v} for name, v in sorted(installed.items())]
    return sbom("nu54-protocol-python", version, components), {
        "checks": ["hash-locked venv install of wheel with quote-v2 extra", "v1 import", "real v2 signer recovery vectors"],
        **observed,
    }, folder


def ts_consumer(output, scratch):
    folder = scratch / "typescript"
    folder.mkdir()
    write(folder / "package.json", {"name": "nu54-external-consumer", "version": "1.0.0", "type": "module",
                                    "dependencies": {"@nu54/protocol": f"file:{output}"}})
    run(["npm", "install", "--ignore-scripts", "--no-audit", "--no-fund"], folder)
    (folder / "check.mts").write_bytes((CONSUMERS / "quote.mts").read_bytes())
    run(["node", "--experimental-strip-types", "check.mts"], folder)
    run(["pnpm", "--dir", str(PROTOCOL / "ts"), "exec", "tsc", "--noEmit", "--strict", "--module", "NodeNext",
         "--target", "ES2022", "--skipLibCheck", str(folder / "check.mts")])
    return json.loads(run(["npm", "sbom", "--sbom-format=cyclonedx", "--omit=dev"], folder, capture=True)), {
        "checks": ["npm install", "Node ESM and typecheck", "C7 approval and denial", "all packaged quote vectors with real recovery",
                   "reconnect resumes live request and denies clock rollback"],
    }, folder


def c_consumer(output, scratch):
    folder = scratch / "c"
    unpack(output, folder)
    path = folder / "nu54-protocol/spec-v2/testdata/quote.vectors"
    quote = bytes.fromhex(path.read_text().splitlines()[1].split("|")[3])
    quote_array = ",".join(f"0x{byte:02x}" for byte in quote)
    (folder / "check.c").write_text(
        '#include "nu54_protocol.h"\n#include "nu54_quote_v2.h"\n#include "nu54_transport_v2.h"\n#include <string.h>\n'
        '_Static_assert(NU54_FIELDS_PAYMENT_AUTHORIZATION == 9, "SDK ABI mismatch");\n'
        f'static const uint8_t expected[] = {{{quote_array}}};\n'
        'int main(void) {\n'
        '    const uint8_t widths[] = NU54_V2_WIDTHS;\n'
        '    nu54_v2_quote_field fields[NU54_V2_FIELD_COUNT];\n'
        '    uint8_t encoded[NU54_V2_UNSIGNED_SIZE];\n'
        '    for (size_t i=0; i<NU54_V2_FIELD_COUNT; i++) {\n'
        '        fields[i] = (nu54_v2_quote_field){expected + (i+1)*32 - widths[i], widths[i]};\n'
        '    }\n'
        '    if (NU54_PROTOCOL_VERSION != 1 || nu54_v2_peer_role(NU54_V2_PAYMENT, NU54_V2_TERMINAL) != NU54_V2_BOARD ||\n'
        '        !nu54_v2_quote_encode(fields, NU54_V2_FIELD_COUNT, encoded) || memcmp(encoded, expected, sizeof(expected))) return 1;\n'
        '    fields[0].length--;\n'
        '    return nu54_v2_quote_encode(fields, NU54_V2_FIELD_COUNT, encoded);\n'
        '}\n'
    )
    run(["cc", "-std=c11", "-Wall", "-Werror", "-I", "nu54-protocol/include", "check.c",
         "nu54-protocol/src/nu54_quote_v2.c", "nu54-protocol/src/nu54_transport_v2.c", "-o", "check"], folder)
    run(["./check"], folder)
    # The transport test hashes with OpenSSL, as packages/protocol/v2/testdata/test-transport.sh does.
    prefix = os.environ.get("OPENSSL_PREFIX")
    openssl = [f"-I{prefix}/include", f"-L{prefix}/lib"] if prefix else []
    run(["cc", "-std=c11", "-Wall", "-Werror", "-I", "nu54-protocol/include", *openssl,
         str(PROTOCOL / "c/test/test_transport_v2.c"),
         "nu54-protocol/src/nu54_transport_v2.c", "-lcrypto", "-o", "transport-test"], folder)
    run(["./transport-test", str(PROTOCOL / "v2/testdata/payment-review.vectors")], folder)
    run(["sh", PROTOCOL / "v2/testdata/test-c.sh", folder / "nu54-protocol", path], folder)
    return {
        "checks": ["C11 v1/v2 source install", "all packed quote vectors using P01 Keccak and real signature recovery",
                   "v2 fragmentation, reconnect, replay and clock rollback guards"],
    }, folder



def assert_materials_unchanged(provenance):
    for collection in ("inputs", "outputs"):
        for name, sha256 in provenance[collection].items():
            require(digest(ROOT / name) == sha256, f"SDK material changed during build: {name}")
    require(run(["git", "rev-parse", "HEAD"], capture=True).strip() == provenance["sourceCommit"],
            "source commit changed during build")
    if provenance["sourceClean"]:
        require(not run(["git", "status", "--porcelain"], capture=True).strip(), "source became dirty during build")


def scan_report(identifier, root, trivy_cache, go_db, govulncheck):
    if identifier == "protocol-go":
        scanned = run([
            str(govulncheck), "-db", go_db.as_uri(), "-mode", "binary", "-format", "json", str(root / "check"),
        ], capture=True)
        decoder = json.JSONDecoder()
        events = []
        while scanned.strip():
            event, end = decoder.raw_decode(scanned.lstrip())
            events.append(event)
            scanned = scanned.lstrip()[end:]
        require(any(event.get("config", {}).get("scan_mode") == "binary" for event in events), "Go scanner observed no binary")
        require(not any(event["finding"]["trace"][0].get("function") for event in events if "finding" in event),
                "Go SDK consumer has a reachable advisory")
        return {"scope": "compiled external Go consumer symbols", "events": events}
    raw = run([
        "trivy", "fs", "--cache-dir", str(trivy_cache), "--skip-db-update", "--skip-java-db-update", "--offline-scan",
        "--skip-version-check", "--scanners", "vuln", "--format", "json", str(root),
    ], capture=True)
    report = json.loads(raw)
    results = report.get("Results") or []
    expected_type = {"protocol-ts": "npm", "protocol-python": "pip"}.get(identifier)
    if expected_type:
        require(any(row.get("Type") == expected_type and row.get("Packages") for row in results),
                f"scanner did not inventory {identifier}")
    require(not any(row.get("Vulnerabilities") for row in results), f"SDK dependency scan failed: {identifier}")
    return {"scope": "external resolved dependencies and unpacked payload", "report": report,
            "database": json.loads((trivy_cache / "db/metadata.json").read_text())}


def ref(path):
    return {"file": path.name, "sha256": digest(path)}


def write_evidence(out, artifact, role, report, **fields):
    identifier = artifact["id"]
    path = out / f"{identifier}.{role}.json"
    write(path, {"format": f"nu54-{role}-v1", "artifactSha256": artifact["payload"]["sha256"],
                 "evidenceKind": "observed", "report": ref(report), **fields})
    artifact.setdefault("evidence", {})[role] = ref(path)


def sign_local(out, artifacts, provenance):
    with tempfile.TemporaryDirectory(prefix="nu54-private-proof-") as temp:
        private = Path(temp) / "secret.pem"
        openssl(["genpkey", "-algorithm", "Ed25519", "-out", private])
        public = out / "local-release.pub.pem"
        public.write_bytes(openssl(["pkey", "-in", private, "-pubout"]))
        fingerprint = key_sha256(public)
        for artifact in artifacts:
            payload = out / artifact["payload"]["file"]
            signature = out / f"{artifact['id']}.payload.sig"
            signature.write_bytes(openssl(["pkeyutl", "-sign", "-rawin", "-inkey", private, "-in", payload]))
            require(authenticate(payload.read_bytes(), signature, public.read_bytes()) == fingerprint,
                    "local artifact signature did not verify")
            report = out / f"{artifact['id']}.signature-check.json"
            write(report, {"format": "nu54-detached-ed25519-v1", "publicKey": ref(public), "signature": ref(signature),
                           "payload": artifact["payload"], "signer": fingerprint})
            write_evidence(out, artifact, "artifactSigning", report, passed=True,
                           signatureFormat="nu54-detached-ed25519-v1")
        manifest = out / "local-release.json"
        write(manifest, {"format": "nu54-release-evidence-v1", "channel": "custom", "artifacts": artifacts,
                         "sourceCommit": provenance["sourceCommit"], "sourceClean": provenance["sourceClean"]})
        signature = out / "local-release.sig"
        signature.write_bytes(openssl(["pkeyutl", "-sign", "-rawin", "-inkey", private, "-in", manifest]))
    policy = {"format": "nu54-release-trust-v1",
              "channels": {"official": {}, "custom": {fingerprint: [a["id"] for a in artifacts]}},
              "installers": {"official": {}, "custom": {}}}
    write(out / "local-trust.json", policy)
    accepted = verify(manifest, signature, public, policy, "custom")
    expected = {(item["id"], item["version"], item["target"]) for item in artifacts}
    require(set(accepted) == expected, "local release artifact set changed")
    restricted = {**policy, "channels": {"official": {}, "custom": {fingerprint: [artifacts[0]["id"]]}}}
    try:
        verify(manifest, signature, public, restricted, "custom")
    except ValueError as error:
        require("outside signer scope" in str(error), "unexpected scoped policy rejection")
    else:
        raise ValueError("restricted signer policy accepted extra artifacts")
    write(out / "local-verification.json", {
        "kind": "local-custom-test-key", "release": ref(manifest), "trustedPolicy": ref(out / "local-trust.json"),
        "restrictedScopeRejected": True, "verified": [
            {"id": identifier, "version": version, "target": target, "status": state}
            for (identifier, version, target), state in accepted.items()
        ],
    })


def build(out, smoke, trivy_cache=None, go_db=None, govulncheck=None):
    metadata = json.loads((PROTOCOL / "ts/package.json").read_bytes())
    version = metadata["version"]
    require(re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?", version), "invalid SDK version")
    require(not out.is_relative_to(ROOT), "use an output directory outside the source checkout")
    out.mkdir(parents=True, exist_ok=False)
    provenance = json.loads(run([sys.executable, "scripts/sdk_provenance.py"], capture=True))
    schema = json.loads((PROTOCOL / "v2/schema.json").read_bytes())
    require(version == schema["sdkVersion"], "SDK version disagrees with the contract")
    packages = {
        "protocol-ts": (out / f"nu54-protocol-{version}.tgz", "node-esm"),
        "protocol-c": (out / f"nu54-protocol-c-{version}.tar.gz", "portable-c11"),
        "protocol-go": (out / f"nu54-protocol-go-{version}-proxy.tar.gz", "go-module-proxy"),
        "protocol-python": (python_sdk(out, version), "python-wheel"),
        "protocol-vectors": (out / f"nu54-protocol-vectors-{version}.tar.gz", "spec-vectors"),
    }
    node_sdk(packages["protocol-ts"][0], metadata)
    c_sdk(packages["protocol-c"][0])
    go_sdk(packages["protocol-go"][0], version)
    deterministic_tar(packages["protocol-vectors"][0], schemas("nu54-protocol-vectors"))
    raw_provenance = out / "generator-report.json"
    write(raw_provenance, provenance)
    artifacts = [
        {"id": identifier, "version": version, "target": target, "payload": ref(path)}
        for identifier, (path, target) in packages.items()
    ]
    for artifact in artifacts:
        write_evidence(out, artifact, "provenance", raw_provenance, **provenance)

    if smoke:
        with tempfile.TemporaryDirectory(prefix="nu54-sdk-consumer-") as directory:
            scratch = Path(directory)
            c_checks, c_path = c_consumer(packages["protocol-c"][0], scratch)
            vector_path = scratch / "vectors"
            unpack(packages["protocol-vectors"][0], vector_path)
            expected = schemas("nu54-protocol-vectors")
            require(all((vector_path / rel).read_bytes() == data for rel, data in expected.items()),
                    "packaged vector bytes differ")
            observations = {
                "protocol-ts": ts_consumer(packages["protocol-ts"][0], scratch),
                "protocol-c": (sbom("nu54-protocol-c", version, [
                    {"type": "library", "name": "OpenSSL", "scope": "optional",
                     "description": "Optional host adapter; SDK archive does not distribute it"},
                ]), c_checks, c_path),
                "protocol-go": go_consumer(packages["protocol-go"][0], scratch, version),
                "protocol-python": python_consumer(packages["protocol-python"][0], scratch, version),
                "protocol-vectors": (sbom("nu54-protocol-vectors", version, [
                    {"type": "file", "name": rel, "hashes": [{"alg": "SHA-256", "content": digest(vector_path / rel)}]}
                    for rel in sorted(expected)
                ]), {"checks": ["separate fixture package matches the canonical SDK schema and vectors"]}, vector_path),
            }
            for artifact in artifacts:
                identifier = artifact["id"]
                inventory, checks, location = observations[identifier]
                raw_sbom = out / f"{identifier}.raw-sbom.json"
                write(raw_sbom, inventory)
                write_evidence(out, artifact, "sbom", raw_sbom)
                regression = out / f"{identifier}.consumer.json"
                write(regression, {**checks, "artifactSha256": artifact["payload"]["sha256"]})
                write_evidence(out, artifact, "regression", regression, passed=True)
                if trivy_cache:
                    scanner = out / f"{identifier}.scan.json"
                    write(scanner, scan_report(identifier, location, trivy_cache, go_db, govulncheck))
                    write_evidence(out, artifact, "rescan", scanner, passed=True)

    assert_materials_unchanged(provenance)
    if trivy_cache:
        sign_local(out, artifacts, provenance)
    write(out / "unsigned-candidate.json", {"format": "nu54-sdk-candidate-v1", "sourceCommit": provenance["sourceCommit"],
          "sourceClean": provenance["sourceClean"], "artifacts": artifacts, "externalConsumerChecks": smoke})


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", required=True, type=Path, help="new output directory")
    parser.add_argument("--smoke", action="store_true", help="install and verify four independent consumers")
    parser.add_argument("--scan-cache", type=Path, help="pinned Trivy DB cache for local custom proof")
    parser.add_argument("--go-db", type=Path, help="pinned local Go vulnerability database directory")
    parser.add_argument("--govulncheck", type=Path, help="pinned govulncheck executable")
    args = parser.parse_args()
    if any((args.scan_cache, args.go_db, args.govulncheck)):
        require(args.smoke and all((args.scan_cache, args.go_db, args.govulncheck)),
                "local proof requires --smoke plus all scanner inputs")
    build(args.out.resolve(), args.smoke, *(path.resolve() if path else None
                                           for path in (args.scan_cache, args.go_db, args.govulncheck)))


if __name__ == "__main__":
    main()

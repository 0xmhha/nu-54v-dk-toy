#!/usr/bin/env python3
"""Verify a release evidence bundle using an independently supplied channel key."""

import argparse
import gzip
import hashlib
import io
import json
import re
import subprocess
import tarfile
import tempfile
from pathlib import Path

SHA256 = re.compile(r"[0-9a-f]{64}")


def require(condition, message):
    if not condition:
        raise ValueError(message)


def digest(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def parse(data):
    def unique(pairs):
        result = {}
        for key, value in pairs:
            require(key not in result, f"duplicate JSON key: {key}")
            result[key] = value
        return result

    return json.loads(data, object_pairs_hook=unique)


def read(path):
    return parse(path.read_bytes())


def resolve(root, ref):
    require(isinstance(ref, dict) and set(ref) == {"file", "sha256"}, "invalid file reference")
    name = Path(ref["file"])
    require(not name.is_absolute() and ".." not in name.parts and bool(name.parts), "unsafe evidence path")
    path = (root / name).resolve()
    require(path.is_relative_to(root.resolve()) and path.is_file(), "invalid evidence path")
    require(SHA256.fullmatch(ref["sha256"]) and digest(path) == ref["sha256"], f"file digest mismatch: {name}")
    return path


def deterministic_tar(path, members):
    with path.open("wb") as stream, gzip.GzipFile(fileobj=stream, filename="", mtime=0, mode="wb") as archive:
        with tarfile.open(fileobj=archive, mode="w", format=tarfile.USTAR_FORMAT) as tar:
            for name, content in sorted(members.items()):
                entry = tarfile.TarInfo(name)
                entry.size = len(content)
                entry.mode = 0o644
                tar.addfile(entry, io.BytesIO(content))


def openssl(args):
    result = subprocess.run(["openssl", *map(str, args)], capture_output=True)
    require(result.returncode == 0, f"OpenSSL {args[0]} failed")
    return result.stdout


def key_sha256(public_key):
    return hashlib.sha256(openssl(["pkey", "-pubin", "-in", public_key, "-outform", "DER"])).hexdigest()


def authenticate(data, signature, public_key_bytes):
    with tempfile.TemporaryDirectory(prefix="nu54-signature-") as directory:
        payload = Path(directory) / "payload"
        public = Path(directory) / "public.pem"
        payload.write_bytes(data)
        public.write_bytes(public_key_bytes)
        openssl(["pkeyutl", "-verify", "-rawin", "-pubin", "-inkey", public, "-in", payload, "-sigfile", signature])
        return key_sha256(public)


def verify(manifest, signature, public_key, policy, channel, install=None):
    manifest_bytes = manifest.read_bytes()
    claim = parse(manifest_bytes)
    require(claim["format"] == "nu54-release-evidence-v1", "unknown release schema")
    require(channel in {"official", "custom"} and claim["channel"] == channel, "release channel mismatch")
    require(policy["format"] == "nu54-release-trust-v1", "unknown release trust policy")
    selected = policy["channels"][channel]
    opposite = policy["channels"]["custom" if channel == "official" else "official"]
    fingerprint = authenticate(manifest_bytes, signature, public_key.read_bytes())
    require(fingerprint in selected and fingerprint not in opposite, "untrusted release signer or channel key reuse")
    require(re.fullmatch(r"[0-9a-f]{40}", claim["sourceCommit"]), "invalid source commit")
    require(type(claim["sourceClean"]) is bool, "source cleanliness missing")
    if channel == "official":
        require(claim["sourceClean"], "official release requires clean source")

    artifacts = set()
    for artifact in claim["artifacts"]:
        identifier = artifact["id"]
        require(identifier in selected[fingerprint] and identifier not in artifacts, "artifact outside signer scope or duplicate")
        artifacts.add(identifier)
        require(all(isinstance(artifact[key], str) and artifact[key].strip() for key in ("version", "target")), "missing artifact version/target")
        file = artifact["payload"]
        resolve(manifest.parent, file)
        roles = artifact["evidence"]
        require(set(roles) == {"provenance", "sbom", "rescan", "regression", "artifactSigning"}, "missing evidence role")
        for role, ref in roles.items():
            path = resolve(manifest.parent, ref)
            data = path.read_bytes()
            require(hashlib.sha256(data).hexdigest() == ref["sha256"], f"{role} evidence changed during verification")
            body = parse(data)
            require(body["format"] == f"nu54-{role}-v1" and body["artifactSha256"] == file["sha256"],
                    f"{role} is not bound to {identifier}")
            require(body.get("evidenceKind") in {"observed", "synthetic"}, f"invalid {role} observation")
            if channel == "official":
                require(body.get("evidenceKind") == "observed", f"official {role} evidence must be observed")
            if role == "provenance":
                require(body["sourceCommit"] == claim["sourceCommit"] and body["sourceClean"] is claim["sourceClean"],
                        "provenance source mismatch")
            if role in {"rescan", "regression", "artifactSigning"}:
                require(body.get("passed") is True, f"{role} gate failed")
            # Optional raw scanner, SDK-generator, native verifier or test output is separately hashed.
            if "report" in body:
                report = resolve(manifest.parent, body["report"])
                if role == "artifactSigning" and body.get("signatureFormat") == "nu54-detached-ed25519-v1":
                    signing = parse(report.read_bytes())
                    require(signing["format"] == body["signatureFormat"] and signing["payload"] == file,
                            "payload signature names a different artifact")
                    key = resolve(manifest.parent, signing["publicKey"])
                    signature_file = resolve(manifest.parent, signing["signature"])
                    payload = resolve(manifest.parent, file)
                    require(authenticate(payload.read_bytes(), signature_file, key.read_bytes()) == fingerprint
                            and signing["signer"] == fingerprint, "payload signing key differs from release policy")
            elif channel == "official":
                require(False, f"official {role} requires a referenced report")

    require(bool(artifacts), "no release artifacts")
    states = {(item["id"], item["version"], item["target"]): "installation-unverified" for item in claim["artifacts"]}
    if install is None:
        return states
    record, install_sig, install_key = install
    install_bytes = record.read_bytes()
    install_claim = parse(install_bytes)
    require(install_claim["format"] == "nu54-installation-v1", "unknown installation schema")
    require(install_claim["channel"] == channel and install_claim["releaseSha256"] == hashlib.sha256(manifest_bytes).hexdigest(),
            "installation release mismatch")
    install_fp = authenticate(install_bytes, install_sig, install_key.read_bytes())
    install_policy = policy["installers"][channel]
    other_installers = policy["installers"]["custom" if channel == "official" else "official"]
    identifier = install_claim["artifactId"]
    require(install_fp in install_policy and identifier in install_policy[install_fp] and install_fp not in other_installers,
            "untrusted installation signer")
    require(install_fp not in selected and install_fp not in opposite, "installation signer must be independent")
    linked = next((item for item in claim["artifacts"] if item["id"] == identifier), None)
    require(linked is not None and all(install_claim[key] == linked[key] for key in ("version", "target"))
            and install_claim["artifactSha256"] == linked["payload"]["sha256"], "installed artifact mismatch")
    kind = install_claim["evidenceKind"]
    require(kind in {"observed", "synthetic"} and (channel == "custom" or kind == "observed"), "invalid installation evidence kind")
    resolve(record.parent, install_claim["report"])
    states[(identifier, linked["version"], linked["target"])] = f"installation-{kind}"
    return states


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for key in ("manifest", "signature", "public-key", "trust"):
        parser.add_argument(f"--{key}", type=Path, required=True)
    parser.add_argument("--channel", choices=["official", "custom"], required=True)
    parser.add_argument("--installation", nargs=3, type=Path, metavar=("RECORD", "SIGNATURE", "KEY"))
    args = parser.parse_args()
    try:
        states = verify(args.manifest, args.signature, args.public_key, read(args.trust), args.channel, args.installation)
        print(f"PASS {args.channel} release signature and artifact evidence")
        for (artifact, version, target), state in sorted(states.items()):
            print(f"{artifact} {version} {target}: {state}")
    except (ValueError, KeyError, TypeError, OSError) as error:
        parser.exit(1, f"FAIL {error}\n")


if __name__ == "__main__":
    main()

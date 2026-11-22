"""Reproducible SDK tarballs and release-evidence trust checks."""

import hashlib
import json
import tempfile
import unittest
from pathlib import Path

import evidence


class EvidenceTests(unittest.TestCase):
    def test_file_pointer_detects_tampering_and_parent_paths(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            file = root / "artifact"
            file.write_bytes(b"original")
            ref = {"file": "artifact", "sha256": hashlib.sha256(b"original").hexdigest()}
            self.assertEqual(evidence.resolve(root, ref), file.resolve())
            file.write_bytes(b"tampered")
            with self.assertRaisesRegex(ValueError, "digest"):
                evidence.resolve(root, ref)
            with self.assertRaisesRegex(ValueError, "path"):
                evidence.resolve(root, dict(ref, file="../artifact"))

    @staticmethod
    def file(root, name, body):
        path = root / name
        path.write_text(json.dumps(body))
        return {"file": name, "sha256": evidence.digest(path)}

    @staticmethod
    def key(root, name):
        private = root / f"{name}.pem"
        public = root / f"{name}.pub.pem"
        evidence.openssl(["genpkey", "-algorithm", "ED25519", "-out", private])
        evidence.openssl(["pkey", "-in", private, "-pubout", "-out", public])
        return private, public

    @staticmethod
    def sign(path, key):
        signature = path.with_suffix(".sig")
        evidence.openssl(["pkeyutl", "-sign", "-rawin", "-inkey", key, "-in", path, "-out", signature])
        return signature

    def test_unsigned_custom_and_mismatched_install_are_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            builder, public = self.key(root, "builder")
            installer, install_public = self.key(root, "installer")
            key_hash = evidence.key_sha256(public)
            policy = {"format": "nu54-release-trust-v1", "channels": {"official": {}, "custom": {key_hash: ["sdk", "c-sdk"]}},
                      "installers": {"custom": {evidence.key_sha256(install_public): ["sdk"]}, "official": {}}}
            payload = self.file(root, "sdk.tgz", {"synthetic": "artifact"})
            commit = "a" * 40
            proof = {}
            for role in ("provenance", "sbom", "rescan", "regression", "artifactSigning"):
                proof[role] = self.file(root, f"{role}.json",
                    {"format": f"nu54-{role}-v1", "artifactSha256": payload["sha256"], "evidenceKind": "synthetic",
                     "passed": True, "sourceCommit": commit, "sourceClean": True})
            second_payload = self.file(root, "c-sdk.tar.gz", {"synthetic": "second artifact"})
            second_proof = {}
            for role in proof:
                second_proof[role] = self.file(root, f"c-{role}.json",
                    {"format": f"nu54-{role}-v1", "artifactSha256": second_payload["sha256"], "evidenceKind": "synthetic",
                     "passed": True, "sourceCommit": commit, "sourceClean": True})
            manifest = root / "release.json"
            release = {"format": "nu54-release-evidence-v1", "channel": "custom", "sourceCommit": commit,
                       "sourceClean": True,
                       "artifacts": [{"id": "sdk", "version": "1.0.0", "target": "synthetic", "payload": payload, "evidence": proof},
                                     {"id": "c-sdk", "version": "2.0.0", "target": "synthetic-c",
                                      "payload": second_payload, "evidence": second_proof}]}
            self.file(root, manifest.name, release)
            signature = self.sign(manifest, builder)
            self.assertEqual(evidence.verify(manifest, signature, public, policy, "custom"),
                             {("sdk", "1.0.0", "synthetic"): "installation-unverified",
                              ("c-sdk", "2.0.0", "synthetic-c"): "installation-unverified"})
            with self.assertRaisesRegex(ValueError, "channel"):
                evidence.verify(manifest, signature, public, policy, "official")
            with self.assertRaisesRegex(ValueError, "OpenSSL"):
                evidence.verify(manifest, signature, install_public, policy, "custom")
            with self.assertRaisesRegex(ValueError, "OpenSSL"):
                evidence.verify(manifest, root / "missing.sig", public, policy, "custom")

            record = root / "install.json"
            report = self.file(root, "install-report.json", {"synthetic": "test"})
            claim = {"format": "nu54-installation-v1", "channel": "custom", "releaseSha256": evidence.digest(manifest),
                     "artifactId": "sdk", "version": "0.9.0", "target": "synthetic", "evidenceKind": "synthetic",
                     "artifactSha256": payload["sha256"], "report": report}
            self.file(root, record.name, claim)
            install_sig = self.sign(record, installer)
            with self.assertRaisesRegex(ValueError, "installed artifact"):
                evidence.verify(manifest, signature, public, policy, "custom", (record, install_sig, install_public))
            claim["version"] = "1.0.0"
            self.file(root, record.name, claim)
            install_sig = self.sign(record, installer)
            self.assertEqual(evidence.verify(manifest, signature, public, policy, "custom",
                                            (record, install_sig, install_public)),
                             {("sdk", "1.0.0", "synthetic"): "installation-synthetic",
                              ("c-sdk", "2.0.0", "synthetic-c"): "installation-unverified"})

            release["channel"] = "official"
            self.file(root, manifest.name, release)
            signature = self.sign(manifest, builder)
            with self.assertRaisesRegex(ValueError, "untrusted"):
                evidence.verify(manifest, signature, public, policy, "official")
            policy["channels"] = {"official": {key_hash: ["sdk"]}, "custom": {}}
            with self.assertRaisesRegex(ValueError, "must be observed"):
                evidence.verify(manifest, signature, public, policy, "official")

    def test_archive_is_reproducible(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            evidence.deterministic_tar(root / "a.tgz", {"package/b.txt": b"B", "package/a.txt": b"A"})
            evidence.deterministic_tar(root / "b.tgz", {"package/a.txt": b"A", "package/b.txt": b"B"})
            self.assertEqual((root / "a.tgz").read_bytes(), (root / "b.tgz").read_bytes())


if __name__ == "__main__":
    unittest.main()

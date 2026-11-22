import json
from pathlib import Path
import tempfile
import unittest

from reference_lock import validate_lock

HERE = Path(__file__).resolve().parent


class ReferencePinTest(unittest.TestCase):
    def test_rejects_reference_and_transitive_drift(self):
        source = HERE / "reference/package-lock.json"
        validate_lock(source)
        for name, field, replaced in [
            ("node_modules/@walletconnect/sign-client", "version", "2.18.0"),
            ("node_modules/@walletconnect/sign-client", "integrity", "sha512-bad"),
            ("node_modules/@walletconnect/sign-client", "license", "SEE LICENSE IN LICENSE.md"),
            ("node_modules/events", "resolved", "https://other.example/events.tgz"),
            ("node_modules/decode-uri-component", "version", "0.5.0"),
            ("node_modules/ws", "version", "8.20.1"),
            ("node_modules/@walletconnect/relay-auth", "version", "1.0.4"),
        ]:
            document = json.loads(source.read_text())
            document["packages"][name][field] = replaced
            with tempfile.TemporaryDirectory() as directory:
                path = Path(directory) / "package-lock.json"
                path.write_text(json.dumps(document))
                with self.assertRaises(RuntimeError):
                    validate_lock(path)


if __name__ == "__main__":
    unittest.main()

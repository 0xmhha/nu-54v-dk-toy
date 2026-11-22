"""Exercise the W12 simulator runner interface without claiming a device."""

import os
from pathlib import Path
import plistlib
import subprocess
import sys
import tempfile
import unittest


RUNNER = Path(__file__).resolve().parents[2] / "run-w12-installed.sh"


class W12RunnerTest(unittest.TestCase):
    def test_generated_spec_is_owned_and_xcode_output_stays_private(self):
        for status in (0, 19):
            with self.subTest(status=status), tempfile.TemporaryDirectory(prefix="nu54-w12-spec-") as temp:
                root = Path(temp)
                binary, scratch = root / "bin", root / "scratch"
                binary.mkdir()
                (binary / "python3").symlink_to(sys.executable)
                scratch.mkdir()
                spec = root / "original.xctestrun"
                spec.write_bytes(plistlib.dumps({"Bundle": {"TestBundlePath": "__TESTROOT__/app.xctest"}}))
                original = spec.read_bytes()
                for name, source in {
                    "xcodebuild": """set -eu
printf '%s\n' "$3" > "$CHECK_PATH"
python3 - "$3" <<'PY'
import os, plistlib, sys
p = plistlib.load(open(sys.argv[1],"rb"))["Bundle"]
assert p["TestBundlePath"] == "__TESTROOT__/app.xctest"
assert p["EnvironmentVariables"]["NU54_W12_CREDENTIAL"] == "1"
assert p["EnvironmentVariables"]["NU54_W12_LINKED_PAYMENTS"] == "1"
assert p["EnvironmentVariables"].get("NU54_W12_SLOT_OPEN") == os.environ.get("NU54_W12_SLOT_OPEN")
PY
echo synthetic-credential-secret
exit "$FAKE_TEST_EXIT"
""",
                    "xcrun": """echo '{"passedTests": 1, "totalTestCount": 1}'""",
                }.items():
                    tool = binary / name
                    tool.write_text("#!/bin/bash\n" + source)
                    tool.chmod(0o700)
                env = {**os.environ, "PATH": f"{binary}:{os.defpath}", "TMPDIR": str(scratch),
                       "NU54_W12_SIM": "owned-test-device", "NU54_W12_XCTESTRUN": str(spec),
                       "NU54_W12_APP": "org.nu54.test", "NU54_W12_TOKEN": "synthetic-token",
                       "NU54_W12_LINKED_PAYMENTS": "1", "NU54_W12_CREDENTIAL": "1",
                       "FAKE_TEST_EXIT": str(status), "CHECK_PATH": str(root / "observed-spec")}
                env.pop("NU54_W12_SLOT_OPEN", None)
                if status == 0:
                    env["NU54_W12_SLOT_OPEN"] = "1"
                result = subprocess.run(["bash", "-c", 'echo sentinel > "W12-$$.xctestrun"; exec bash "$1" "$2"',
                                         "--", RUNNER, "http://127.0.0.1:1"], env=env, cwd=root, capture_output=True,
                                        text=True, timeout=15)
                self.assertEqual(status, result.returncode, result.stderr)
                self.assertNotIn("synthetic-credential-secret", result.stdout + result.stderr)
                owned = Path((root / "observed-spec").read_text().strip())
                self.assertEqual(root.resolve(), owned.parent)
                self.assertEqual(".xctestrun", owned.suffix)
                self.assertFalse(owned.exists())
                self.assertEqual(original, spec.read_bytes())
                caller = next(root.glob("W12-*.xctestrun"))
                self.assertEqual("sentinel\n", caller.read_text())
                self.assertEqual([], list(scratch.iterdir()))


if __name__ == "__main__":
    unittest.main()

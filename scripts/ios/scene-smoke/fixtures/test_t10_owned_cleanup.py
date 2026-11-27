"""Exercise the maintained T10 ownership boundary with fake native tools."""

import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import tempfile
import time
import unittest


SCENE = Path(__file__).resolve().parents[1]
OWNED = "DAB218DA-2856-45E9-AA07-778FBF12E55B"
SHARED = "EE313EAA-3FA0-4639-A857-499A880AAF41"

XCRUN = r"""
import json, os, sys, time
from pathlib import Path
root = Path(os.environ["PROBE"])
mode = os.environ["MODE"]
_, _, action, *args = sys.argv
if action == "create":
    (root / "device").write_text(args[0])
    if mode == "creation":
        time.sleep(.5)
    if mode == "lost-id":
        sys.exit(17)
    print(os.environ["OWNED"])
elif action in ("boot", "bootstatus", "install", "uninstall"):
    assert args[0] == os.environ["OWNED"]
elif action in ("shutdown", "delete"):
    assert args[0] == os.environ["OWNED"]
    with (root / "sim-actions").open("a") as log:
        log.write(action + "\n")
    if action == "delete":
        (root / "device").unlink(missing_ok=True)
elif action == "list":
    assert args == ["devices", "--json"]
    if mode == "list-error":
        sys.exit(3)
    if mode == "malformed-list":
        print('{"devices":{"runtime":[{"name":"partial"}]}}')
        sys.exit(0)
    devices = [{"udid": os.environ["SHARED"], "name": "shared simulator"}]
    if (root / "device").exists():
        devices.append({"udid": os.environ["OWNED"], "name": (root / "device").read_text()})
    print(json.dumps({"devices": {"runtime": devices}}))
else:
    sys.exit(2)
"""

LOWER = r"""
import json, os, signal, sys, time
from pathlib import Path
root = Path(os.environ["PROBE"])
sim, derived, result, port, mode = sys.argv[1:]
assert (sim, port, mode) == (os.environ["OWNED"], "65176", "--my-trip")
derived, result = Path(derived), Path(result)
derived.mkdir()
result.mkdir()
(result / "raw.xcresult").mkdir()
(derived / "intermediate").write_text("private")
db = root / "nested-db"
db.mkdir()
def leave(status):
    db.rmdir()
    (root / "db-stopped").touch()
    sys.exit(status)
def shutdown(*_):
    (root / "db-stopping").touch()
    time.sleep(5.3)
    leave(143)
signal.signal(signal.SIGTERM, shutdown)
if os.environ["MODE"] == "acceptance":
    (root / "running").touch()
    while True:
        time.sleep(.05)
if os.environ["MODE"] == "failure":
    (result / "assertion-lines.txt").write_text("22\nnot an assertion\n")
    print("raw fixture marker must not be in the receipt", file=sys.stderr)
    leave(17)
(result / "test-summary.json").write_text(json.dumps({"passedTests": 1, "totalTestCount": 1}))
(result / "signed.sha256").write_text("a" * 64 + "\n" + "b" * 64 + "\n")
(result / "db-check.log").write_text("0|0|0|0|0|0\n")
leave(0)
"""

MKTEMP = r"""
import os, pathlib, sys, tempfile, time
pattern = pathlib.Path(sys.argv[-1])
actual = tempfile.mkdtemp(prefix=pattern.name.split("XXXXXXXX")[0], dir=pattern.parent)
pathlib.Path(os.environ["PROBE"], "allocated").write_text(actual)
time.sleep(.5)
print(actual)
"""


class T10Cleanup(unittest.TestCase):
    def setUp(self):
        storage = tempfile.TemporaryDirectory()
        self.addCleanup(storage.cleanup)
        self.root = Path(storage.name)
        self.tmp = self.root / "tmp"
        self.tmp.mkdir()
        self.shared = self.tmp / "shared-derived"
        self.shared.mkdir()
        self.sentinel = self.shared / "raw.xcresult"
        self.sentinel.write_bytes(b"caller-owned")
        self.original = hashlib.sha256(self.sentinel.read_bytes()).digest()
        ios = self.root / "ios"
        scene = ios / "scene-smoke"
        scene.mkdir(parents=True)
        self.runner = scene / "run-travel-t10-owned.sh"
        for source in (SCENE / self.runner.name, SCENE / "private-build.sh", SCENE.parent / "test-environment.sh"):
            shutil.copyfile(source, (ios if source.parent == SCENE.parent else scene) / source.name)
        (scene / "run-itinerary-conflict.sh").write_text('exec python3 -B "$PROBE/lower.py" "$@"\n')
        (self.root / "lower.py").write_text(LOWER)
        self.fake("xcrun", XCRUN)
        self.fake("uuidgen", 'print("314C02BC-5462-4DF3-88AB-09C0CEF3E052")')

    def fake(self, name, code):
        path = self.root / name
        path.write_text("#!/usr/bin/env python3\n" + code)
        path.chmod(0o700)

    def run_fixture(self, mode, interruption=None):
        env = {**os.environ, "PROBE": str(self.root), "TMPDIR": str(self.tmp),
               "OWNED": OWNED, "SHARED": SHARED, "MODE": mode,
               "PATH": str(self.root) + os.pathsep + os.environ["PATH"]}
        process = subprocess.Popen(["bash", self.runner, "65176"], env=env,
                                   start_new_session=True, stdout=subprocess.PIPE,
                                   stderr=subprocess.PIPE, text=True)

        def reap():
            if process.poll() is None:
                os.killpg(process.pid, signal.SIGKILL)
                process.communicate(timeout=5)

        self.addCleanup(reap)
        if interruption:
            marker, signum, whole_group = interruption
            self.wait_for(marker, process)
            if whole_group:
                os.killpg(process.pid, signum)
            else:
                process.send_signal(signum)
                self.wait_for("db-stopping", process)
                process.send_signal(signal.SIGHUP)
                self.assertTrue((self.root / "nested-db").is_dir())
        output, errors = process.communicate(timeout=18)
        self.assertEqual(list(self.tmp.iterdir()), [self.shared])
        self.assertEqual(hashlib.sha256(self.sentinel.read_bytes()).digest(), self.original)
        self.assertFalse((self.root / "nested-db").exists())
        return process.returncode, json.loads(output), errors

    def wait_for(self, marker, process):
        deadline = time.monotonic() + 4
        while not (self.root / marker).exists():
            self.assertIsNone(process.poll(), marker)
            self.assertLess(time.monotonic(), deadline, marker)
            time.sleep(.01)

    def test_low_level_interrupt_between_private_spawn_and_pid_publication(self):
        # Use the actual caller-owned runner. Inject TERM at the Bash PID
        # assignment after the fake private process has really started.
        ios = self.root / "source/scripts/ios"
        scene = ios / "scene-smoke"
        fixtures = scene / "fixtures"
        fixtures.mkdir(parents=True)
        shutil.copyfile(SCENE.parent / "test-environment.sh", ios / "test-environment.sh")
        shutil.copyfile(SCENE / "run-itinerary-conflict.sh", scene / "run-itinerary-conflict.sh")
        setup = self.root / "bash-env"
        setup.write_text(r"""
if [[ ${0##*/} == run-itinerary-conflict.sh ]]; then
  trap 'if [[ $BASH_COMMAND == "private=\$!" ]]; then
    trap - DEBUG
    for ((ready=0; ready<200; ready++)); do
      [[ ! -s "$PROBE/private-pid" ]] || break
      sleep .01
    done
    kill -TERM "$$"
  fi' DEBUG
fi
""")
        (fixtures / "p09_private_tls.py").write_text("""
import os, signal, time
from pathlib import Path
probe = Path(os.environ["PROBE"])
def stop(*_):
    (probe / "private-stopped").touch()
    raise SystemExit(0)
signal.signal(signal.SIGTERM, stop)
(probe / "private-pid").write_text(str(os.getpid()))
while True:
    time.sleep(.05)
""")
        (fixtures / "p09_booking_catalog_tls.py").write_text("import time\ntime.sleep(10)\n")
        self.fake("codesign", "pass")
        self.fake("xcodebuild", """
import sys
from pathlib import Path
app = Path(sys.argv[sys.argv.index("-derivedDataPath")+1]) / "Build/Products/Release-iphonesimulator/NU54Renter.app"
app.mkdir(parents=True, exist_ok=True)
for name in ("NU54Renter", "main.jsbundle"):
    (app / name).write_text("fixture")
""")
        env = {**os.environ, "BASH_ENV": str(setup), "PROBE": str(self.root), "TMPDIR": str(self.tmp),
               "OWNED": OWNED, "SHARED": SHARED, "MODE": "success",
               "PATH": str(self.root) + os.pathsep + os.environ["PATH"]}
        caller_result = self.root / "caller-result"
        with (self.root / "lower.log").open("w+b") as log:
            proc = subprocess.Popen(["bash", scene / "run-itinerary-conflict.sh", OWNED, self.shared,
                                     caller_result, "65176", "--my-trip"], env=env,
                                    stdout=log, stderr=log, start_new_session=True)
            try:
                self.assertEqual(proc.wait(timeout=12), 143)
                self.assertTrue((self.root / "private-stopped").is_file(),
                                "private child survived interruption during PID publication")
            finally:
                marker = self.root / "private-pid"
                if marker.is_file() and not (self.root / "private-stopped").is_file():
                    try:
                        os.kill(int(marker.read_text()), signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                if proc.poll() is None:
                    proc.kill()
                    proc.wait(timeout=4)
        self.assertTrue(caller_result.is_dir())
        self.assertEqual(hashlib.sha256(self.sentinel.read_bytes()).digest(), self.original)
        self.assertEqual(list(self.tmp.iterdir()), [self.shared])

    def test_success_keeps_only_compact_evidence(self):
        rc, report, _ = self.run_fixture("success")
        self.assertEqual(rc, 0)
        self.assertEqual(report["privateWrites"], [0] * 6)
        self.assertTrue(report["ownedSimulatorRemoved"])
        self.assertTrue((self.root / "db-stopped").is_file())

    def test_failure_preserves_exit_and_suppresses_raw_log(self):
        rc, report, errors = self.run_fixture("failure")
        self.assertEqual(rc, 17)
        self.assertEqual(report["assertionLines"], [22])
        self.assertTrue(report["ownedSimulatorRemoved"])
        self.assertNotIn("raw fixture marker", errors)

    def test_group_interruption_during_simulator_creation(self):
        rc, report, _ = self.run_fixture("creation", ("device", signal.SIGHUP, True))
        self.assertEqual(rc, 129)
        self.assertTrue(report["ownedSimulatorRemoved"])
        self.assertFalse((self.root / "device").exists())

    def test_group_interruption_during_workspace_creation(self):
        self.fake("mktemp", MKTEMP)
        rc, report, _ = self.run_fixture("allocation", ("allocated", signal.SIGTERM, True))
        self.assertEqual(rc, 143)
        self.assertEqual(report["phase"], "setup")
        self.assertFalse((self.root / "sim-actions").exists())

    def test_slow_teardown_waits_despite_repeated_signals(self):
        rc, report, _ = self.run_fixture("acceptance", ("running", signal.SIGTERM, False))
        self.assertEqual(rc, 143)
        self.assertTrue(report["ownedSimulatorRemoved"])
        self.assertTrue((self.root / "db-stopped").exists())

    def test_malformed_inventory_is_not_absence(self):
        rc, report, errors = self.run_fixture("malformed-list")
        self.assertEqual(rc, 1)
        self.assertFalse(report["ownedSimulatorRemoved"])
        self.assertIn(OWNED, errors)

    def test_failed_inventory_is_not_absence(self):
        rc, report, errors = self.run_fixture("list-error")
        self.assertEqual(rc, 1)
        self.assertFalse(report["ownedSimulatorRemoved"])
        self.assertIn(OWNED, errors)

    def test_failed_creation_recovers_id_by_unique_name(self):
        rc, report, _ = self.run_fixture("lost-id")
        self.assertEqual(rc, 17)
        self.assertTrue(report["ownedSimulatorRemoved"])
        self.assertFalse((self.root / "device").exists())


if __name__ == "__main__":
    unittest.main()

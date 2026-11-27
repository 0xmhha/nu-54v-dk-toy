"""Check registration XCTest scratch without a simulator or Xcode."""

import itertools
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
import unittest


HERE = Path(__file__).resolve().parent
HARNESS = r"""
set -euo pipefail
source "$1"
phase=$2
tool=$3
ios_test_env=(env)
finish() {
  local status=$?
  trap '' HUP INT TERM
  registration_test_cleanup || exit 1
  exit "$status"
}
trap finish EXIT
trap 'registration_test_interrupt 129' HUP
trap 'registration_test_interrupt 130' INT
trap 'registration_test_interrupt 143' TERM
mktemp() {
  local created
  created=$(command mktemp "$@")
  if [[ $phase == allocation ]]; then
    printf '%s\n' "$created" >"$PROBE/allocating"
    sleep 0.6
  fi
  printf '%s\n' "$created"
}
registration_test_prepare
for build in test-build board-test-build; do
  registration_test_run "$tool" "$phase" "$registration_test_work/$build" "$PROBE/log-$build"
done
"""
TOOL = """#!/usr/bin/env python3
import os
from pathlib import Path
import signal
import sys
import time
phase, cache, output = sys.argv[1:]
cache, output = Path(cache), Path(output)
cache.mkdir()
(cache / "intermediate").touch()
output.write_text(str(cache.parent))
if phase == "tool":
    def stop(*_):
        (output.parent / "stopping").touch()
        time.sleep(0.4)
        assert cache.exists()
        (output.parent / "stopped").touch()
        sys.exit(0)
    signal.signal(signal.SIGTERM, stop)
    (output.parent / "child-pid").write_text(str(os.getpid()))
    (output.parent / "child").touch()
    while True:
        time.sleep(0.1)
if phase == "failure":
    sys.exit(17)
"""


class RegistrationWorkspaceTest(unittest.TestCase):
    def test_release_failure_cleans_default_caches_only(self):
        caches = ("Build/Intermediates.noindex", "ModuleCache.noindex", "SDKStatCaches.noindex",
                  "SDKExplicitPrecompiledModules", "CompilationCache.noindex", "Index.noindex")
        for scenario, explicit in (("registration", False), ("backup", False), ("replacement", False),
                                   ("board", False), ("tools", False), ("tools", True)):
            with self.subTest(scenario=scenario, explicit=explicit), tempfile.TemporaryDirectory() as directory:
                probe = Path(directory).resolve()
                tools = probe / "bin"
                tools.mkdir()
                temporary = probe / "tmp"
                temporary.mkdir()
                evidence = probe / "evidence"
                derived = probe / "caller-build" if explicit else evidence / "build"
                for name, body in {
                    "cmake": "#!/bin/sh\nexit 0\n",
                    "xcrun": "#!/bin/sh\nexit 1\n",
                    "xcodebuild": (
                        "#!/usr/bin/env python3\n"
                        "from pathlib import Path\nimport sys\n"
                        "root = Path(sys.argv[sys.argv.index('-derivedDataPath')+1])\n"
                        f"for part in {caches + ('Build/Products', 'Logs')!r}:\n"
                        "    folder = root / part\n"
                        "    folder.mkdir(parents=True)\n"
                        "    (folder / 'sentinel').touch()\n"
                        "sys.exit(17)\n"
                    ),
                }.items():
                    tool = tools / name
                    tool.write_text(body)
                    tool.chmod(0o700)
                env = {**os.environ, "PATH": str(tools) + os.pathsep + os.environ["PATH"],
                       "TMPDIR": str(temporary), "NU54_OPENSSL_ROOT": str(tools),
                       "NU54_W04_DERIVED_DATA": str(derived) if explicit else ""}
                result = subprocess.run(["bash", HERE / "run-registration.sh", "0" * 36, evidence, scenario],
                                        env=env, timeout=15, capture_output=True, text=True)
                self.assertEqual(result.returncode, 17, result.stderr)
                for part in caches:
                    self.assertEqual((derived / part / "sentinel").exists(), explicit, part)
                for part in ("Build/Products", "Logs"):
                    self.assertTrue((derived / part / "sentinel").is_file())
                self.assertTrue((evidence / "app-build.log").is_file())
                self.assertEqual(list(temporary.iterdir()), [])

    def test_success_and_failure_preserve_evidence(self):
        for phase, status, results in (("normal", 0, 2), ("failure", 17, 1)):
            with self.subTest(phase=phase):
                self.run_case(phase, status, results)

    def test_signals_during_allocation_and_xctest(self):
        for phase, sig in itertools.product(("allocation", "tool"), (signal.SIGHUP, signal.SIGINT, signal.SIGTERM)):
            with self.subTest(phase=phase, signal=sig):
                self.run_case(phase, 128 + sig, int(phase == "tool"), sig)

    def run_case(self, phase, status, results, sig=None):
        with tempfile.TemporaryDirectory() as directory:
            probe = Path(directory).resolve()
            temporary = probe / "tmp"
            temporary.mkdir()
            external = probe / "caller-build"
            external.write_text("keep")
            tool = probe / "fake-xctest"
            tool.write_text(TOOL)
            tool.chmod(0o700)
            env = {**os.environ, "PROBE": str(probe), "TMPDIR": str(temporary),
                   "NU54_W04_DERIVED_DATA": str(external)}
            with subprocess.Popen(["bash", "-c", HARNESS, "registration-test", HERE / "registration-test-build.sh",
                                   phase, tool], env=env, start_new_session=True) as runner:
                try:
                    if sig is not None:
                        self.wait_for(probe / ("allocating" if phase == "allocation" else "child"), runner)
                        runner.send_signal(sig)
                        if phase == "tool":
                            self.wait_for(probe / "stopping", runner)
                            runner.send_signal(signal.SIGTERM if sig != signal.SIGTERM else signal.SIGHUP)
                    self.assertEqual(runner.wait(timeout=8), status)
                finally:
                    if runner.poll() is None:
                        runner.terminate()
                        try:
                            runner.wait(timeout=12)
                        except subprocess.TimeoutExpired:
                            runner.kill()
                            runner.wait()
                    child = probe / "child-pid"
                    if child.is_file() and not (probe / "stopped").exists():
                        try:
                            os.killpg(int(child.read_text()), signal.SIGKILL)
                        except ProcessLookupError:
                            pass
            if phase == "tool":
                self.assertTrue((probe / "stopped").exists())
            self.assertEqual(len(list(probe.glob("log-*"))), results)
            self.assertEqual(list(temporary.iterdir()), [])
            self.assertEqual(external.read_text(), "keep")

    @staticmethod
    def wait_for(marker, runner):
        until = time.monotonic() + 5
        while not marker.is_file():
            if runner.poll() is not None or time.monotonic() >= until:
                raise AssertionError(f"{marker.name} not produced, exit={runner.poll()}")
            time.sleep(0.02)


if __name__ == "__main__":
    unittest.main()

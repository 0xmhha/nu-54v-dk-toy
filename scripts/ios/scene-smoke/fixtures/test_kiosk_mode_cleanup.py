#!/usr/bin/env python3
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
import unittest

FIXTURE = Path(__file__).with_name("p05_kiosk_mode_native.sh").resolve()
REPO = FIXTURE.parents[4]
TOOLS = r'''#!/bin/bash
set -eu
case "${0##*/}" in
  mktemp)
    owned=$(/usr/bin/mktemp "$@")
    if [[ ${MODE-} == allocate-term ]]; then
      while [[ ! -s "$LAUNCHER_FILE" ]]; do sleep .01; done
      kill -TERM "$(cat "$LAUNCHER_FILE")"
    fi
    printf '%s\n' "$owned" ;;
  initdb) mkdir -p "$2"; touch "$2/PG_VERSION"; if [[ ${MODE-} == init-fail ]]; then exit 41; fi ;;
  pg_ctl)
    if [[ "$*" == *" status"* ]]; then
      if [[ -e "$2/live" ]]; then exit 0; fi
      exit 3
    elif [[ "$*" == *" start"* ]]; then
      touch "$2/live"
      if [[ ${MODE-} == start-fail ]]; then exit 42; fi
    elif [[ "$*" == *" stop"* ]]; then
      test ! -e "$2/../ops-active"
      test ! -e "$2/../control-active"
      if [[ ${MODE-} == retained ]]; then exit 1; fi
      rm -f "$2/live"
    fi ;;
  createdb|make|psql) true ;;
  openssl)
    if [[ "$1" == rand ]]; then printf 'fixture fixture\n'; fi
    while [[ $# -gt 0 ]]; do
      if [[ "$1" == -out || "$1" == -keyout ]]; then shift; printf 'fixture' > "$1"; fi
      shift
    done ;;
  go)
    cat > "$3" <<'DAEMON'
#!/bin/bash
touch "${0%/*}/ops-active"
trap 'rm -f "${0%/*}/ops-active"; exit 0' TERM
while :; do
  if [[ ${MODE-} == complete && -e "${0%/*}/../ready" ]]; then
    rm "${0%/*}/ops-active"
    exit 0
  fi
  sleep .1
done
DAEMON
    chmod +x "$3" ;;
  python3)
    if [[ "$1" == -B ]]; then exec "$REAL_PYTHON" "$@"; fi
    if [[ "$1" == - && $# == 4 ]]; then
      cat >/dev/null
    elif [[ "$1" == - ]]; then
      cat >/dev/null
      dir="${2%/*}"
      touch "$dir/control-active"
      trap 'rm -f "$dir/control-active"; exit 0' TERM
      while :; do sleep .1; done
    else
      dir="$2"
      touch "$dir/control-active"
      trap 'rm -f "$dir/control-active"; exit 0' TERM
      printf '{"ready":true}' > "$5"
      while :; do sleep .1; done
    fi ;;
  nc) [[ "$*" == "-z 127.0.0.1 18867" ]] ;;
  curl) true ;;
  shasum) printf '%064d\n' 0 ;;
esac
'''


class Cleanup(unittest.TestCase):
    def fixture(self, lab, mode="", offline=False):
        lab.mkdir()
        (lab / "caller.txt").write_text("preserve")
        (lab / "offline-ready.tmp").write_text("caller neighbor")
        for role in ("terminal", "operator"):
            (lab / (role + ".der")).write_bytes(bytes(91))
        shim = lab.parent / "bin"
        shim.mkdir(exist_ok=True)
        driver = shim / "driver"
        driver.write_text(TOOLS)
        driver.chmod(0o700)
        for command in ("mktemp", "initdb", "pg_ctl", "createdb", "make", "psql", "openssl", "go", "python3", "nc", "curl", "shasum"):
            target = shim / command
            if not target.exists():
                target.symlink_to(driver)
        environment = {**os.environ, "PATH": f"{shim}:{os.environ['PATH']}", "MODE": mode,
                       "REAL_PYTHON": sys.executable, "NU54_KIOSK_ORIGIN_PORT": "18867", "NU54_KIOSK_OFFLINE_FIXTURE": str(int(offline))}
        return ["bash", str(FIXTURE), str(lab), str(lab/"terminal.der"), str(lab/"operator.der")], environment

    def wait_ready(self, process, ready):
        for _ in range(100):
            if ready.is_file():
                return
            self.assertIsNone(process.poll())
            time.sleep(.05)
        self.fail("fixture did not publish readiness")

    def test_cleans_partial_initialization_and_start_failure(self):
        for mode, code in (("init-fail", 41), ("start-fail", 42)):
            with self.subTest(mode=mode), tempfile.TemporaryDirectory(prefix="kiosk-clean-", dir="/tmp") as path:
                lab = Path(path) / "fixture"
                args, env = self.fixture(lab, mode)
                existing = lab / "fixture.other-owner"
                existing.mkdir()
                (existing / "sentinel").write_text("other run")
                result = subprocess.run(args, env=env, cwd=REPO, capture_output=True, timeout=10)
                self.assertEqual(result.returncode, code, result.stderr)
                self.assertEqual(list(lab.glob("fixture.*")), [existing])
                self.assertEqual((existing / "sentinel").read_text(), "other run")

    def test_cleans_normal_and_offline_after_term(self):
        for offline, ready in ((False, "ready"), (True, "offline-ready.json")):
            with self.subTest(offline=offline), tempfile.TemporaryDirectory(prefix="kiosk-clean-", dir="/tmp") as path:
                lab = Path(path) / "fixture"
                args, env = self.fixture(lab, offline=offline)
                with subprocess.Popen(args, cwd=REPO, env=env, start_new_session=True,
                                      stdout=subprocess.DEVNULL, stderr=subprocess.PIPE) as process:
                    try:
                        self.wait_ready(process, lab / ready)
                    finally:
                        if process.poll() is None:
                            process.send_signal(signal.SIGTERM)
                    _, error = process.communicate(timeout=10)
                self.assertEqual(process.returncode, 0, error)
                self.assertEqual(list(lab.glob("fixture.*")), [])
                self.assertFalse((lab / ready).exists())
                self.assertEqual((lab/"offline-ready.tmp").read_text(), "caller neighbor")
                self.assertEqual((lab/"caller.txt").read_text(), "preserve")

    def test_natural_completion_removes_owned_files(self):
        with tempfile.TemporaryDirectory(prefix="kiosk-clean-", dir="/tmp") as path:
            lab = Path(path) / "fixture"
            args, env = self.fixture(lab, "complete")
            result = subprocess.run(args, cwd=REPO, env=env, capture_output=True, timeout=10)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(list(lab.glob("fixture.*")), [])
            self.assertFalse((lab / "ready").exists())
            self.assertEqual((lab / "caller.txt").read_text(), "preserve")

    def test_term_between_mktemp_creation_and_assignment(self):
        with tempfile.TemporaryDirectory(prefix="kiosk-clean-", dir="/tmp") as path:
            lab = Path(path) / "fixture"
            args, env = self.fixture(lab, "allocate-term")
            handoff = Path(path) / "pid"
            with subprocess.Popen(args, cwd=REPO, env={**env, "LAUNCHER_FILE": str(handoff)},
                                  stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL) as process:
                handoff.write_text(str(process.pid))
                self.assertEqual(process.wait(timeout=10), 0)
            self.assertEqual(list(lab.glob("fixture.*")), [])

    def test_term_between_child_launch_and_recording_pid(self):
        hook_code = r'''set -T
interrupt_launch() {
  if [[ ${BASH_SOURCE[1]-} == "$FIXTURE_FILE" && ${pid_field-} == "$INTERRUPT_FIELD" &&
        "$BASH_COMMAND" == 'printf -v "$pid_field" '\''%s'\'' "$!"' ]]; then
    trap - DEBUG
    for _ in {1..150}; do
      [[ ! -e "$dir/$INTERRUPT_MARKER" ]] || break
      sleep .01
    done
    if [[ -e "$dir/$INTERRUPT_MARKER" ]]; then
      printf 'child-active\n' > "$HIT_FILE"
      kill -TERM "$$"
    fi
  fi
}
trap interrupt_launch DEBUG
'''
        for offline, field, marker in ((False, "ops", "ops-active"), (False, "controller", "control-active"),
                                       (True, "controller", "control-active")):
            with self.subTest(offline=offline, field=field), tempfile.TemporaryDirectory(prefix="kiosk-clean-", dir="/tmp") as path:
                work = Path(path)
                lab = work / "fixture"
                args, env = self.fixture(lab, offline=offline)
                handoff = work / "hook.sh"
                handoff.write_text(hook_code)
                hit = work / "reached"
                env.update(BASH_ENV=str(handoff), FIXTURE_FILE=str(FIXTURE), INTERRUPT_FIELD=field,
                           INTERRUPT_MARKER=marker, HIT_FILE=str(hit))
                result = subprocess.run(args, cwd=REPO, env=env, capture_output=True, timeout=10)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(hit.read_text().strip(), "child-active")
                self.assertEqual(list(lab.glob("fixture.*")), [])

    def test_existing_readiness_directory_is_untouched(self):
        with tempfile.TemporaryDirectory(prefix="kiosk-clean-", dir="/tmp") as path:
            lab = Path(path) / "fixture"
            args, env = self.fixture(lab)
            caller_ready = lab / "ready"
            caller_ready.mkdir()
            result = subprocess.run(args, cwd=REPO, env=env, capture_output=True, timeout=10)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(list(caller_ready.iterdir()), [])
            self.assertEqual(list(lab.glob("fixture.*")), [])

    def test_keeps_live_database_and_existing_readiness(self):
        with tempfile.TemporaryDirectory(prefix="kiosk-clean-", dir="/tmp") as path:
            lab = Path(path) / "fixture"
            args, env = self.fixture(lab, mode="retained")
            (lab/"ready").write_text("other owner")
            result = subprocess.run(args, cwd=REPO, env=env, capture_output=True, timeout=10)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn(b"Could not confirm PostgreSQL shutdown", result.stderr)
            self.assertEqual((lab/"ready").read_text(), "other owner")
            owned, = lab.glob("fixture.*")
            self.assertTrue((owned/"pgdata/live").exists())


if __name__ == "__main__":
    unittest.main()

"""Lifecycle regressions for owned integration runner caches and PostgreSQL."""
import json
import os
from pathlib import Path
from types import SimpleNamespace
import signal
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import call, patch

sys.dont_write_bytecode = True
from integration_workspace import WorkspaceCleanupError, _signal_group, run_command, scoped_workspace


from test_pos_upgrade_fixture import POSUpgradeLifecycle
from test_contract_test_workspace import ContractTestWorkspaceTest
from test_foundry_failure_paths import FoundryFailurePathsTest
from test_host_test_workspace import HostTestWorkspaceTest

SCRIPTS = Path(__file__).resolve().parent
DRIVER = """
from pathlib import Path
import os, signal, sys, time
import integration_workspace as runner
from integration_workspace import integration_workspace, start_postgres, cleanup_stack, OwnedProcess
marker, pg, action, status = sys.argv[1:]
if action == "stub-ignored-hup-term":
    signal.signal(signal.SIGHUP, signal.SIG_IGN)
if action in ("stub-owner-race", "stub-owner-hup", "stub-ignored-hup-term"):
    mkdtemp = runner.tempfile.mkdtemp
    def interrupted_creation(*args, **kwargs):
        path = mkdtemp(*args, **kwargs)
        Path(marker).write_text(path)
        if action != "stub-owner-race":
            signal.raise_signal(signal.SIGHUP)
        if action != "stub-owner-hup":
            signal.raise_signal(signal.SIGTERM)
        return path
    runner.tempfile.mkdtemp = interrupted_creation
with integration_workspace("nu54-cleanup-test-", pg + "/pg_ctl", "pg", os.environ, term_status=int(status)) as (work, env):
    assert Path(env["GOCACHE"]).parent == work
    assert Path(env["GOTMPDIR"]).parent == work
    assert env["TMPDIR"] == env["GOTMPDIR"]
    Path(marker).write_text(str(work))
    data = work / "pg"
    if action.startswith("stub"):
        data.mkdir()
        (data / "PG_VERSION").write_text("15")
    else:
        from integration_workspace import run_command
        run_command([pg + "/initdb", "-D", data, "-A", "trust", "--no-locale", "--encoding=UTF8"], check=True, env=env)
    if action in ("stub-double", "stub-fallback", "stub-start-fail"):
        (data / action).touch()
    start_postgres(pg + "/pg_ctl", data, work / "postgres.log", f"-k {work} -p 5432 -c listen_addresses=''", env)
    with cleanup_stack() as stack:
        if action in ("real-term", "real-hup", "stub-transition"):
            worker = OwnedProcess()
            stack.callback(worker.stop)
            worker.start([sys.executable, "-c", "import signal,time; signal.signal(signal.SIGTERM, signal.SIG_IGN); time.sleep(120)"])
            (work / "worker.pid").write_text(str(worker.child.pid))
            if action == "real-hup":
                def while_cleaning():
                    (work / "cleaning").touch()
                    time.sleep(.5)
                stack.callback(while_cleaning)
            (work / "ready").touch()
            while True:
                time.sleep(1)
        if action == "real-fail":
            raise RuntimeError("fixture failed after PostgreSQL startup")
"""
PG_STUB = """#!/usr/bin/env python3
import os,sys,time
from pathlib import Path
args = sys.argv[1:]
data = Path(args[1])
work = data.parent
phase = args[-1]
live = data / "synthetic-live"
if phase == "start":
    (work / "starting").touch()
    time.sleep(.35)
    if (data / "stub-start-fail").exists():
        sys.exit(9)
    live.touch()
    sys.exit(0)
if phase == "status":
    sys.exit(0 if live.exists() else 3)
if phase == "stop":
    mode = args[args.index("-m")+1]
    with (work / "attempts").open("a") as output:
        output.write(mode + "\\n")
    if (data / "stub-double").exists() or ((data / "stub-fallback").exists() and mode=="fast"):
        sys.exit(1)
    live.unlink(missing_ok=True)
    sys.exit(0)
sys.exit(2)
"""


class OwnedGroupPermissionTest(unittest.TestCase):
    def test_darwin_group_disappears_after_permission_error(self):
        with patch("integration_workspace.sys.platform", "darwin"), patch(
            "integration_workspace.os.killpg", side_effect=[PermissionError(), PermissionError(), ProcessLookupError()]
        ) as signal_group:
            _signal_group(SimpleNamespace(pid=123, poll=lambda: None), signal.SIGKILL)
        self.assertEqual(signal_group.call_args_list, [call(123, signal.SIGKILL), call(123, 0), call(123, 0)])

    def test_darwin_persistent_permission_error_is_reported(self):
        with patch("integration_workspace.sys.platform", "darwin"), patch(
            "integration_workspace.os.killpg", side_effect=PermissionError()
        ), patch("integration_workspace.time.monotonic", side_effect=[0, 0, 4]), patch(
            "integration_workspace.time.sleep"
        ):
            with self.assertRaises(WorkspaceCleanupError):
                _signal_group(SimpleNamespace(pid=123, poll=lambda: None), signal.SIGKILL)


class CommercialModeTest(unittest.TestCase):
    def test_regular_suite_keeps_explicit_kiosk_modes_available_to_separate_runners(self):
        import test_commercial_integrations as suite

        modes = {"NU54_K10_INSTALLED_RUNNER": "/test/installed", "NU54_K10_LIVE_HOLD": "1"}
        with patch.dict(os.environ, modes):
            shared = suite.secure_env()
            self.assertEqual({key: shared.get(key) for key in modes}, modes)
            with patch.object(sys, "argv", ["commercial"]), patch.object(
                suite, "integration_workspace", side_effect=InterruptedError("stop before starting PostgreSQL")
            ) as workspace:
                with self.assertRaises(InterruptedError):
                    suite.main()
            regular = workspace.call_args.args[3]
            self.assertFalse(modes.keys() & regular.keys())
            self.assertEqual({key: os.environ[key] for key in modes}, modes)


class IntegrationWorkspaceTest(unittest.TestCase):
    def setUp(self):
        self.scratch = tempfile.TemporaryDirectory(prefix="nu54-cleanup-harness-", dir="/tmp")
        self.addCleanup(self.scratch.cleanup)
        self.path = Path(self.scratch.name)
        self.marker = self.path / "owned"
        self.external = self.path / "caller-cache"
        self.external.mkdir()
        self.sentinel = self.external / "retained"
        self.sentinel.touch()
        self.env = {**os.environ, "PYTHONPATH": str(SCRIPTS), "GOCACHE": str(self.external),
                    "TMPDIR": str(self.external), "PYTHONDONTWRITEBYTECODE": "1", "GOENV": "off"}
        for key in tuple(self.env):
            if key.startswith("PG"):
                self.env.pop(key)

    def stub(self):
        executable = self.path / "pg_ctl"
        executable.write_text(PG_STUB)
        executable.chmod(0o700)
        return str(self.path)

    def launch(self, pg, action, term_status=143):
        child = subprocess.Popen([sys.executable, "-B", "-c", DRIVER, self.marker, pg, action, str(term_status)],
                                 env=self.env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
        self.addCleanup(self.reap, child)
        return child

    def reap(self, child):
        if child.poll() is None:
            child.terminate()
            try:
                child.wait(timeout=15)
            except subprocess.TimeoutExpired:
                child.kill()
                child.wait()
        for stream in (child.stdout, child.stderr):
            stream.close()

    def wait_for(self, condition, child=None):
        until = time.monotonic() + 12
        while time.monotonic() < until:
            if condition():
                return
            if child is not None and child.poll() is not None:
                self.fail(child.communicate()[1].decode())
            time.sleep(.03)
        self.fail("fixture did not reach its expected stage")

    def absent(self, pid):
        result = subprocess.run(["ps", "-o", "stat=", "-p", str(pid)], capture_output=True, text=True, check=False)
        state = result.stdout.strip()
        return not state or state.startswith("Z")

    def finish(self, child, code, removes=True):
        _, err = child.communicate(timeout=20)
        self.assertEqual(child.returncode, code, err.decode())
        self.assertTrue(self.sentinel.exists())
        work = Path(self.marker.read_text())
        self.assertEqual(work.exists(), not removes)
        return work, err.decode()

    def test_timeout_reaps_child_group(self):
        pidfile = self.path / "child-pid"
        command = (
            "import pathlib,subprocess,sys,time; "
            "c=subprocess.Popen([sys.executable,'-c','import time;time.sleep(120)']); "
            f"pathlib.Path({str(pidfile)!r}).write_text(str(c.pid)); time.sleep(120)"
        )
        with self.assertRaises(subprocess.TimeoutExpired):
            run_command([sys.executable, "-c", command], timeout=.6)
        self.assertTrue(pidfile.exists())
        self.wait_for(lambda: self.absent(int(pidfile.read_text())))

    def test_termination_grace_allows_nested_workspace_cleanup(self):
        inner = """
from pathlib import Path
import sys, time
from integration_workspace import scoped_workspace
with scoped_workspace("nested-", directory=sys.argv[1]) as work:
    Path(sys.argv[2]).write_text(str(work))
    time.sleep(120)
"""
        outer = """
import sys
from integration_workspace import run_command, scoped_workspace
with scoped_workspace("outer-", directory=sys.argv[1]):
    run_command([sys.executable, "-B", "-c", sys.argv[3], *sys.argv[1:3]], termination_grace=3)
"""
        child = subprocess.Popen([sys.executable, "-B", "-c", outer, self.path, self.marker, inner],
                                 env=self.env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
        self.addCleanup(self.reap, child)
        self.wait_for(lambda: self.marker.exists() and self.marker.stat().st_size, child)
        child.terminate()
        self.finish(child, 143)
        self.assertEqual(list(self.path.glob("outer-*")), [])

    def test_interruption_during_startup_waits_for_detached_postmaster(self):
        for interruption, expected in ((signal.SIGTERM, 0), (signal.SIGHUP, 129)):
            with self.subTest(interruption=interruption):
                self.marker.unlink(missing_ok=True)
                child = self.launch(self.stub(), "stub-transition", 0)
                self.wait_for(self.marker.exists, child)
                work = Path(self.marker.read_text())
                self.wait_for(lambda: (work / "starting").exists(), child)
                child.send_signal(interruption)
                time.sleep(.05)
                child.send_signal(interruption)
                self.finish(child, expected)

    def test_product_runners_wait_for_postgres_startup(self):
        # Stop each entry point after pg_ctl starts but before its handshake returns.
        # The external marker survives workspace cleanup and records that ordering.
        root = SCRIPTS.parent
        stub = self.path / "pg_ctl"
        stub.write_text("""#!/usr/bin/env python3
import os, sys, time
from pathlib import Path
args = sys.argv[1:]
if args == ['--version']:
    print('pg_ctl (PostgreSQL) 15.9')
    sys.exit(0)
data = Path(args[args.index('-D') + 1])
if args[-1] == 'start':
    data.joinpath('starting').touch()
    Path(os.environ['TEST_WORKSPACE']).write_text(str(data.parent))
    time.sleep(.5)
    Path(os.environ['TEST_START_RETURNED']).touch()
sys.exit(3 if args[-1] == 'status' else 0)
""")
        stub.chmod(0o700)
        initdb = self.path / "initdb"
        initdb.write_text("""#!/usr/bin/env python3
import sys
from pathlib import Path
data = Path(sys.argv[sys.argv.index('-D') + 1])
data.mkdir()
data.joinpath('PG_VERSION').write_text('15')
""")
        initdb.chmod(0o700)
        # Skip unrelated compiler builds and version probes before the first database.
        driver = """
import os, runpy, subprocess, sys
import integration_workspace as runner
original = runner.run_command
def run(args, **options):
    if any(str(a).endswith(('scripts/contract_forge.py', 'scripts/verify_ci_inputs.py')) for a in args):
        return subprocess.CompletedProcess(args, 0)
    if list(args) == ['anvil', '--version']:
        return subprocess.CompletedProcess(args, 0, 'anvil Version: ' + os.environ['TEST_FOUNDRY'])
    return original(args, **options)
runner.run_command = run
sys.argv = sys.argv[1:]
runpy.run_path(sys.argv[0], run_name='__main__')
"""
        returned = self.path / "started"
        env = {**self.env, "PATH": str(self.path) + os.pathsep + self.env["PATH"],
               "POSTGRES_BIN": str(self.path), "TEST_WORKSPACE": str(self.marker),
               "TEST_START_RETURNED": str(returned),
               "TEST_FOUNDRY": json.loads((root / ".github/inputs.json").read_text())["versions"]["foundry"]}
        for script, args in (
            ("scripts/test_travel_booking_integration.py", []),
            ("scripts/test_commercial_integrations.py", ["--postgres-bin", str(self.path)]),
            ("scripts/pos_upgrade_fixture.py", []),
            ("products/p07-indexer/test/generation-v1.py", ["--pg-bin", str(self.path), "--solc", "solc"]),
        ):
            with self.subTest(script=script):
                self.marker.unlink(missing_ok=True)
                returned.unlink(missing_ok=True)
                child = subprocess.Popen([sys.executable, "-B", "-c", driver, script, *args], cwd=root,
                                         env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
                self.addCleanup(self.reap, child)
                self.wait_for(lambda: self.marker.exists() and self.marker.stat().st_size, child)
                child.terminate()
                self.finish(child, 143)
                self.assertTrue(returned.exists(), "pg_ctl was interrupted before its startup handshake completed")

    def test_term_between_workspace_creation_and_return(self):
        child = self.launch(self.stub(), "stub-owner-race")
        self.finish(child, 143)

    def test_hup_between_workspace_creation_and_return(self):
        child = self.launch(self.stub(), "stub-owner-hup")
        self.wait_for(lambda: self.marker.exists() and self.marker.stat().st_size)
        work = Path(self.marker.read_text())
        try:
            self.finish(child, 129)
        finally:
            if work.exists():
                self.remove_stub(work)

    def test_ignored_hup_cannot_consume_deferred_term(self):
        child = self.launch(self.stub(), "stub-ignored-hup-term", 46)
        self.finish(child, 46)

    def test_workspace_restores_handler_and_preserves_explicit_hup_ignore(self):
        observed = []

        def received(signum, _frame):
            observed.append(signum)

        previous = signal.signal(signal.SIGHUP, received)
        try:
            with scoped_workspace("normal-", directory=self.path) as work:
                self.assertTrue(work.is_dir())
            self.assertFalse(work.exists())
            signal.raise_signal(signal.SIGHUP)
            self.assertEqual(observed, [signal.SIGHUP])
            signal.signal(signal.SIGHUP, signal.SIG_IGN)
            with scoped_workspace("nohup-", directory=self.path) as work:
                signal.raise_signal(signal.SIGHUP)
                self.assertTrue(work.is_dir())
            self.assertFalse(work.exists())
            self.assertEqual(signal.getsignal(signal.SIGHUP), signal.SIG_IGN)
        finally:
            signal.signal(signal.SIGHUP, previous)

    def test_early_start_failure_removes_scratch(self):
        child = self.launch(self.stub(), "stub-start-fail")
        _, err = self.finish(child, 1)
        self.assertIn("non-zero exit status 9", err)

    def test_shutdown_escalates_when_fast_fails(self):
        child = self.launch(self.stub(), "stub-fallback")
        self.finish(child, 0)

    def test_double_shutdown_failure_retains_only_owned_workspace(self):
        child = self.launch(self.stub(), "stub-double")
        work, err = self.finish(child, 1, removes=False)
        self.addCleanup(self.remove_stub, work)
        self.assertIn(str(work), err)
        self.assertIn("fast stop=1", err)
        self.assertIn("immediate stop=1", err)
        self.assertEqual((work / "attempts").read_text().splitlines(), ["fast", "immediate"])
        self.assertTrue((work / "pg" / "synthetic-live").exists())

    @staticmethod
    def remove_stub(work):
        # This case uses a marker file, never an actual database process.
        import shutil
        shutil.rmtree(work)

    def real(self):
        pg = os.getenv("POSTGRES_BIN")
        if not pg:
            self.skipTest("POSTGRES_BIN is required for real PostgreSQL coverage")
        return pg

    def test_real_exit_and_failure_reclaim_postgres_and_caches(self):
        for action, expected in (("real-success", 0), ("real-fail", 1)):
            with self.subTest(action=action):
                child = self.launch(self.real(), action)
                self.finish(child, expected)

    def test_real_interruption_stops_postgres_and_worker(self):
        child = self.launch(self.real(), "real-term", 0)
        self.wait_for(self.marker.exists, child)
        work = Path(self.marker.read_text())
        self.wait_for(lambda: (work / "ready").exists(), child)
        pg_pid = int((work / "pg" / "postmaster.pid").read_text().splitlines()[0])
        worker_pid = int((work / "worker.pid").read_text())
        child.terminate()
        time.sleep(.1)
        if child.poll() is None:
            child.terminate()
        self.finish(child, 0)
        self.wait_for(lambda: self.absent(pg_pid) and self.absent(worker_pid))

    def test_real_hup_during_cleanup_preserves_other_invocation(self):
        with tempfile.TemporaryDirectory(dir=self.path) as separate:
            sibling_marker = Path(separate) / "sibling"
            command = """
from integration_workspace import scoped_workspace
import pathlib, sys, time
with scoped_workspace("sibling-", directory=sys.argv[1]) as work:
    pathlib.Path(sys.argv[2]).write_text(str(work))
    time.sleep(120)
"""
            sibling = subprocess.Popen([sys.executable, "-B", "-c", command, separate, sibling_marker],
                                       env=self.env, start_new_session=True)
            def stop_sibling():
                if sibling.poll() is None:
                    sibling.terminate()
                    sibling.wait(timeout=15)
            self.addCleanup(stop_sibling)
            self.wait_for(lambda: sibling_marker.exists() and sibling_marker.stat().st_size, sibling)
            sibling_work = Path(sibling_marker.read_text())

            child = self.launch(self.real(), "real-hup", 0)
            self.wait_for(self.marker.exists, child)
            work = Path(self.marker.read_text())
            self.wait_for(lambda: (work / "ready").exists(), child)
            pg_pid = int((work / "pg" / "postmaster.pid").read_text().splitlines()[0])
            worker_pid = int((work / "worker.pid").read_text())
            child.send_signal(signal.SIGHUP)
            self.wait_for(lambda: (work / "cleaning").exists(), child)
            child.send_signal(signal.SIGHUP)
            child.terminate()
            self.finish(child, 129)
            self.wait_for(lambda: self.absent(pg_pid) and self.absent(worker_pid))
            self.assertIsNone(sibling.poll())
            self.assertTrue(sibling_work.is_dir())
            sibling.terminate()
            self.assertEqual(sibling.wait(timeout=15), 143)
            self.assertFalse(sibling_work.exists())


if __name__ == "__main__":
    unittest.main()

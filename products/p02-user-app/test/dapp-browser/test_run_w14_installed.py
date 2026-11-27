import errno
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch
import uuid

import run_w14_installed as acceptance


class W14AcceptanceRunner(unittest.TestCase):
    def setUp(self):
        self.storage = tempfile.TemporaryDirectory()
        self.addCleanup(self.storage.cleanup)
        self.directory = Path(self.storage.name)
        self.handlers = {kind: signal.getsignal(kind) for kind in
                         (signal.SIGINT, signal.SIGTERM, signal.SIGHUP)}
        self.addCleanup(lambda: [signal.signal(kind, previous) for kind, previous in self.handlers.items()])

    def test_late_xcode_output_does_not_leave_owned_scratch(self):
        remove = acceptance.shutil.rmtree
        attempts = []

        def late_output(path):
            attempts.append(path)
            if len(attempts) == 1:
                (path / "late-xcode-output").mkdir()
                raise OSError(errno.ENOTEMPTY, "late Xcode output")
            return remove(path)

        with patch.object(acceptance.shutil,"rmtree",late_output), self.assertRaises(InterruptedError):
            with acceptance.private_scratch() as scratch:
                raise InterruptedError("test interrupt")
        self.assertEqual(attempts,[scratch,scratch])
        self.assertFalse(scratch.exists())

    def test_fixture_rejects_wrong_types_and_duplicate_keys_or_records(self):
        valid = json.dumps(acceptance.EXPECTED)
        self.assertEqual(json.loads(acceptance.parse_fixture("fixture running\n" + valid)), acceptance.EXPECTED)
        for field, value in (("samePeerHash", 1), ("submissions", True), ("boardApprovals", 1.0),
                             ("connectionViewRetiredReview", "true"), ("finalizedAllowance", 4)):
            with self.subTest(field=field, value=value), self.assertRaises(ValueError):
                acceptance.parse_fixture(json.dumps({**acceptance.EXPECTED, field: value}))
        for invalid in (valid + "\n  " + valid, valid[:-1] + ',"submissions":1}', "{}"):
            with self.assertRaises(ValueError):
                acceptance.parse_fixture(invalid)

    def test_keeps_prior_receipt_when_staged_write_fails(self):
        evidence = self.directory / "evidence"
        prior = acceptance.publish(evidence, "first", {"xctest.txt": "prior\n"})
        with self.assertRaises(TypeError):
            acceptance.publish(evidence, "failure", {"xctest.txt": "new\n", "relay-anvil.json": None})
        self.assertEqual(list(evidence.iterdir()), [prior])
        self.assertEqual((prior / "xctest.txt").read_text(), "prior\n")

    def test_term_during_port_acquisition_releases_lease_without_starting(self):
        fd = os.open(self.directory / "lease", os.O_CREAT | os.O_RDWR, 0o600)

        def acquire():
            os.kill(os.getpid(), signal.SIGTERM)
            return fd

        with patch.object(acceptance, "acquire_port", acquire), patch.object(acceptance.Runner, "run") as run:
            with self.assertRaises(InterruptedError):
                acceptance.main(self.directory / "evidence")
            run.assert_not_called()
        with self.assertRaises(OSError):
            os.fstat(fd)
        self.assertFalse((self.directory / "evidence").exists())

    def test_term_during_create_removes_registered_simulator_even_without_returned_identifier(self):
        owned = {}
        sim = str(uuid.uuid4()).upper()
        original = acceptance.Runner.create_simulator

        def command(runner, args, **_):
            command = args[2]
            if command == "create":
                owned[sim] = {"udid": sim, "name": runner.sim_name, "state": "Shutdown"}
                os.kill(os.getpid(), signal.SIGTERM)
                raise InterruptedError("simctl did not report a UUID")
            if command == "list":
                return json.dumps({"devices": {"fixture": list(owned.values())}})
            if command == "delete":
                del owned[args[3]]
                return ""
            self.fail(f"Unexpected simulated command {command}")

        fd = os.open(self.directory / "lease", os.O_CREAT | os.O_RDWR, 0o600)
        with patch.object(acceptance, "acquire_port", return_value=fd), \
                patch.object(acceptance.Runner, "run", original), patch.object(acceptance.Runner, "command", command):
            with self.assertRaises(InterruptedError):
                acceptance.main(self.directory / "evidence")
        self.assertFalse(owned)
        self.assertFalse((self.directory / "evidence").exists())

    def test_term_during_fixture_bind_cleans_started_child(self):
        import_code = subprocess.Popen
        scratch = self.directory / "fixture"
        scratch.mkdir()
        runner = acceptance.Runner(scratch, "synthetic")
        signal.signal(signal.SIGTERM, runner.interrupt)
        with socket.socket() as probe:
            probe.bind(("127.0.0.1", 0))
            port = probe.getsockname()[1]
        ready = scratch / "listening"

        def listening_child(*_args, **kwargs):
            program = ("from pathlib import Path; import socket, sys, time;"
                       "s=socket.socket();s.bind(('127.0.0.1',int(sys.argv[1])));s.listen();"
                       "Path(sys.argv[2]).touch();time.sleep(30)")
            child = import_code([sys.executable, "-c", program, str(port), ready], **kwargs)
            self.addCleanup(runner.stop, child, True)
            deadline = time.monotonic() + 4
            while not ready.exists() and time.monotonic() < deadline:
                time.sleep(.02)
            self.assertTrue(ready.exists())
            os.kill(os.getpid(), signal.SIGTERM)
            return child

        with patch.object(acceptance.subprocess, "Popen", listening_child):
            with self.assertRaises(InterruptedError):
                runner.start_fixture(sys.executable)
        with patch.object(acceptance, "PORT", port):
            runner.clean()
        self.assertIsNotNone(runner.fixture.poll())

    def test_existing_listener_stays_owned_by_its_client(self):
        lease = self.directory / "lease"
        with socket.socket() as listener:
            listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            listener.bind(("127.0.0.1", 0))
            listener.listen()
            port = listener.getsockname()[1]
            with self.assertRaisesRegex(RuntimeError, "loopback port .* is busy"):
                acceptance.acquire_port(lease, port)
            with socket.create_connection(("127.0.0.1", port), timeout=2):
                client, _ = listener.accept()
                client.close()
        os.close(acceptance.acquire_port(lease, port))

    def test_unremoved_simulator_is_a_cleanup_error(self):
        scratch = self.directory / "simulator"
        scratch.mkdir()
        runner = acceptance.Runner(scratch, "synthetic")
        runner.creation_attempted = True
        with patch.object(runner, "owned_simulators", return_value={"owned-id": "Shutdown"}), \
                patch.object(runner, "command") as command:
            with self.assertRaisesRegex(RuntimeError, "still present"):
                runner.clean()
            command.assert_called_once_with(["xcrun", "simctl", "delete", "owned-id"], cleaning=True, capture=True)

    def test_separate_processes_and_checkouts_share_one_port_lease(self):
        driver = ("import os,sys;sys.path.insert(0,sys.argv[1]);"
                  "import run_w14_installed as w;"
                  "fd=w.acquire_port(sys.argv[2],0);print('locked',flush=True);sys.stdin.read();os.close(fd)")
        lease = self.directory / "lease"
        checkout_a, checkout_b = (self.directory / name for name in ("a", "b"))
        checkout_a.mkdir()
        checkout_b.mkdir()
        command = [sys.executable, "-B", "-c", driver, str(acceptance.HERE), lease]
        with subprocess.Popen(command, cwd=checkout_a, stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True) as first:
            try:
                self.assertEqual(first.stdout.readline().strip(), "locked")
                other = subprocess.run(command, cwd=checkout_b, input="", capture_output=True, text=True, timeout=4)
                self.assertNotEqual(other.returncode, 0)
                self.assertIn("host lease is unavailable", other.stderr)
            finally:
                first.communicate("")
        restored = subprocess.run(command, cwd=checkout_b, input="", capture_output=True, text=True, timeout=4)
        self.assertEqual(restored.returncode, 0)

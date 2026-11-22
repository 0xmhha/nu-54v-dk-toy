"""Regression for interrupts and cleanup of the two disconnected Core runners."""

import itertools
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import tempfile
import time
import unittest

ROOT = Path(__file__).resolve().parents[1]
WATCH = ROOT / "products/p02-user-app/test/verify_watch_export.py"
DISPLAY = ROOT / "products/p01-device-firmware/test/run_bip48_display.py"

CORE = """#!/usr/bin/env python3
import os, pathlib, signal, sys, time
probe = pathlib.Path(os.environ["BITCOIN_WORKSPACE_PROBE"])
scratch = next(a.split("=", 1)[1] for a in sys.argv if a.startswith("-datadir="))
def stop(*_):
    (probe / "stopping").touch()
    time.sleep(0.35)
    (probe / "stopped").touch()
    sys.exit(0)
signal.signal(signal.SIGTERM, stop)
if os.environ.get("BITCOIN_WORKSPACE_READY"):
    datadir = pathlib.Path(scratch) / ("signet" if "-signet" in sys.argv else "regtest")
    datadir.mkdir()
    (datadir / ".cookie").write_text("fixture")
(probe / "scratch").write_text(scratch)
(probe / "started").write_text(str(os.getpid()))
while True:
    time.sleep(0.1)
"""

CLI = """#!/usr/bin/env python3
import os, pathlib, sys, time
if sys.argv[-1] == "getblockchaininfo":
    print("{}")
else:
    pathlib.Path(os.environ["BITCOIN_WORKSPACE_PROBE"], "rpc-running").touch()
    time.sleep(30)
"""


class BitcoinWorkspaceTest(unittest.TestCase):
    def test_w17_display_child_does_not_write_bytecode(self):
        with tempfile.TemporaryDirectory() as name:
            probe = Path(name)
            for source in (
                ROOT / "scripts/integration_workspace.py",
                ROOT / "scripts/bitcoin_core_workspace.py",
                DISPLAY,
                *(DISPLAY.parent / filename for filename in (
                    "test_bip48.py", "run_bip48_node.py", "chain_openssl_test.py",
                    "run_wallet_chain_nodes.py", "bip48_vector.json", "bip48_two_input_vector.json",
                )),
                *(DISPLAY.parents[1] / "scripts" / filename for filename in (
                    "wallet_chain_lab.py", "wallet_chain_review.py",
                )),
            ):
                destination = probe / source.relative_to(ROOT)
                destination.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(source, destination)
            for executable, script_text in (("bitcoind", CORE), ("bitcoin-cli", CLI)):
                binary = probe / executable
                binary.write_text(script_text)
                binary.chmod(0o700)
            env = {
                **os.environ, "TMPDIR": name,
                "BITCOIN_WORKSPACE_PROBE": name, "BITCOIN_WORKSPACE_READY": "1",
            }
            env.pop("PYTHONDONTWRITEBYTECODE", None)
            env.pop("PYTHONPYCACHEPREFIX", None)
            result = subprocess.run(
                [sys.executable, "-B", probe / DISPLAY.relative_to(ROOT), probe, probe / "unused-oracle"],
                env=env, capture_output=True, text=True, timeout=25,
            )
            # The real display imports the review module before attempting its
            # RPC. The stand-in Core deliberately has no HTTP listener.
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("in display", result.stderr)
            self.assertIn("Connection refused", result.stderr)
            self.assertTrue((probe / "stopped").is_file())
            self.assertFalse(Path((probe / "scratch").read_text()).exists())
            self.assertEqual(list(probe.rglob("__pycache__")), [])

    def test_signals_during_core_start_and_cleanup(self):
        signals = (signal.SIGHUP, signal.SIGTERM, signal.SIGINT)
        cases = [(script, signum, False) for script, signum in itertools.product((WATCH, DISPLAY), signals)]
        cases.append((WATCH, signal.SIGTERM, True))
        for script, signum, ready in cases:
            with self.subTest(script=script.name, signal=signum, ready=ready), tempfile.TemporaryDirectory() as name:
                probe = Path(name)
                source = probe / "caller-input"
                source.write_text("preserve caller input")
                for executable, script_text in [("bitcoind", CORE), ("bitcoin-cli", CLI)]:
                    binary = probe / executable
                    binary.write_text(script_text)
                    binary.chmod(0o700)
                env = {**os.environ, "TMPDIR": name, "BITCOIN_WORKSPACE_PROBE": name}
                if ready:
                    env["BITCOIN_WORKSPACE_READY"] = "1"
                with subprocess.Popen([sys.executable, "-B", script, probe, source],
                                      stdin=subprocess.PIPE, stdout=subprocess.DEVNULL,
                                      start_new_session=True,
                                      env=env) as runner:
                    runner.stdin.write(b"{}")
                    runner.stdin.close()
                    try:
                        self.wait_file(probe / "started", runner)
                        scratch = Path((probe / "scratch").read_text())
                        self.assertEqual(scratch.resolve().parent, probe.resolve())
                        core_pid = int((probe / "started").read_text())
                        if ready:
                            self.wait_file(probe / "rpc-running", runner)
                        runner.send_signal(signum)
                        self.wait_file(probe / "stopping", runner)
                        self.assertTrue(scratch.is_dir(), "scratch removed before Core stopped")
                        runner.send_signal(signal.SIGTERM if signum != signal.SIGTERM else signal.SIGHUP)
                        self.assertEqual(runner.wait(timeout=5), 128 + signum)
                        self.assertTrue((probe / "stopped").exists())
                        self.assertFalse(scratch.exists())
                        with self.assertRaises(ProcessLookupError):
                            os.kill(core_pid, 0)
                        self.assertEqual(source.read_text(), "preserve caller input")
                    finally:
                        if (probe / "started").is_file() and not (probe / "stopped").exists():
                            recorded = (probe / "started").read_text()
                            if recorded:
                                self.stop_process(int(recorded))
                        if runner.poll() is None:
                            runner.kill()

    @staticmethod
    def stop_process(pid):
        try:
            os.kill(pid, signal.SIGKILL)
        except ProcessLookupError:
            pass

    @staticmethod
    def wait_file(marker, runner):
        deadline = time.monotonic() + 5
        while not marker.is_file() or (marker.name == "started" and not marker.stat().st_size):
            if runner.poll() is not None or time.monotonic() >= deadline:
                raise AssertionError(f"missing {marker.name}; runner={runner.poll()}")
            time.sleep(0.025)


if __name__ == "__main__":
    unittest.main()

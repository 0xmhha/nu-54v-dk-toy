"""Regression checks for host and Python test workspace ownership."""

import json
import os
from pathlib import Path
import shlex
import shutil
import signal
import subprocess
import tempfile
import time
import unittest
from unittest.mock import patch

import host_test_workspace as host


class HostTestWorkspaceTest(unittest.TestCase):
    def test_default_host_path_is_owned(self):
        with patch.object(host, "run_environment", side_effect=lambda _work, env: dict(env)), patch.object(host, "run_command") as commands:
            host.run_firmware("make", None, {})
        args = commands.call_args.args[0]
        self.assertEqual(args[:2], ["make", "_host-test"])
        output = Path(args[2].removeprefix("HOST_BUILD="))
        self.assertTrue(output.is_absolute())
        self.assertFalse(output.parent.exists())
        self.assertEqual(commands.call_args.kwargs["env"]["PYTHONDONTWRITEBYTECODE"], "1")

    def test_explicit_build_and_python_bytecode_dirs_survive_term(self):
        with tempfile.TemporaryDirectory() as directory:
            original = Path(directory)
            sentinel = original / "keep"
            sentinel.write_text("caller")
            def interrupt(args, env, **_kwargs):
                self.assertEqual(args[-1], f"HOST_BUILD={original}")
                self.assertEqual(env["PYTHONPYCACHEPREFIX"], directory)
                self.assertNotIn("PYTHONDONTWRITEBYTECODE", env)
                signal.raise_signal(signal.SIGTERM)

            with patch.object(host, "run_environment", side_effect=lambda _work, env: dict(env)), patch.object(
                host, "run_command", side_effect=interrupt,
            ), self.assertRaises(SystemExit) as error:
                host.run_firmware("make", directory, {"PYTHONPYCACHEPREFIX": directory})
            self.assertEqual(error.exception.code, 143)
            self.assertEqual(sentinel.read_text(), "caller")

    def test_pytest_explicit_options_win_and_failed_scratch_is_reaped(self):
        observed = []

        def fail(args, env, **_kwargs):
            self.assertEqual(args[-3:], ["pytest", "-q", "python/tests"])
            options = shlex.split(env["PYTEST_ADDOPTS"])
            self.assertEqual(options[-2:], ["-o", "cache_dir=external"])
            self.assertIn("--basetemp", options)
            work = Path(options[1].split("=", 1)[1]).parent
            observed.append(work)
            self.assertEqual(env["TMPDIR"], str(work))
            self.assertTrue(work.is_dir())
            raise ValueError("injected")

        with patch.object(host, "run_command", side_effect=fail), self.assertRaisesRegex(ValueError, "injected"):
            host.run_protocol_python({"PYTEST_ADDOPTS": "-o cache_dir=external"})
        self.assertFalse(observed[0].exists())

    def test_make_owns_default_host_build_on_failure_and_term(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            scripts = root / "scripts"
            project = root / "products/p01-device-firmware"
            scripts.mkdir()
            project.mkdir(parents=True)
            for name in ("host_test_workspace.py", "integration_workspace.py"):
                shutil.copyfile(host.ROOT / "scripts" / name, scripts / name)
            shutil.copyfile(host.ROOT / "products/p01-device-firmware/Makefile", project / "Makefile")
            cmake = root / "cmake"
            cmake.write_text("""
#!/usr/bin/env python3
import json, os, pathlib, sys, time
output = pathlib.Path(sys.argv[sys.argv.index("-B") + 1]).resolve()
output.mkdir(parents=True, exist_ok=True)
mark = pathlib.Path(os.environ["TEST_CMAKE_MARKER"])
temporary = mark.with_suffix(".tmp")
temporary.write_text(json.dumps([str(output), os.getpid()]))
temporary.rename(mark)
if os.environ["TEST_CMAKE_MODE"] == "term":
    time.sleep(60)
sys.exit(17)
""".lstrip())
            cmake.chmod(0o700)
            borrowed = project / "caller artifacts"
            borrowed.mkdir()
            keep = borrowed / "retained"
            keep.touch()

            for mode, explicit in (("fail", False), ("term", False), ("fail", True)):
                with self.subTest(mode=mode, explicit=explicit):
                    marker = root / "fixture.json"
                    marker.unlink(missing_ok=True)
                    env = {**os.environ, "PATH": str(root) + os.pathsep + os.environ["PATH"],
                           "TEST_CMAKE_MODE": mode, "TEST_CMAKE_MARKER": str(marker), "TMPDIR": str(root)}
                    env.pop("HOST_BUILD", None)
                    args = ["make", "-C", str(project), "test"]
                    if explicit:
                        args.append("HOST_BUILD=caller artifacts")
                    child = subprocess.Popen(args, env=env, stdout=subprocess.DEVNULL,
                                             stderr=subprocess.DEVNULL, start_new_session=True)
                    try:
                        deadline = time.monotonic() + 10
                        while not marker.exists() and child.poll() is None and time.monotonic() < deadline:
                            time.sleep(.02)
                        self.assertTrue(marker.exists(), "normal test did not configure the host build")
                        output, pid = json.loads(marker.read_text())
                        if mode == "term":
                            child.terminate()
                        self.assertNotEqual(child.wait(timeout=10), 0)
                        deadline = time.monotonic() + 5
                        while list(root.glob("nu54-p01-host-test-*")) and time.monotonic() < deadline:
                            time.sleep(.02)
                        self.assertEqual(list(root.glob("nu54-p01-host-test-*")), [])
                        with self.assertRaises(ProcessLookupError):
                            os.kill(pid, 0)
                        self.assertEqual(Path(output).exists(), explicit)
                        if explicit:
                            self.assertEqual(Path(output), borrowed.resolve())
                        self.assertTrue(keep.is_file())
                    finally:
                        if child.poll() is None:
                            os.killpg(child.pid, signal.SIGTERM)
                            child.wait(timeout=5)

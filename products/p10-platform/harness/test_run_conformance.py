import json
import os
from pathlib import Path
import tempfile
from subprocess import CompletedProcess
import unittest
from unittest.mock import patch

import run_conformance as harness


class ConformanceIsolation(unittest.TestCase):
    def test_success_and_failure_delete_owned_command_paths(self):
        for exit_code in (0, 7):
            homes = []
            firmware = harness.CHECKS[-1]
            checks = [firmware, next(check for check in harness.CHECKS if check.cmd[0] == "uv")]

            def run_command(command, *, env, **_):
                scratch = Path(env["TMPDIR"]).parent
                homes.append(scratch)
                self.assertIn("nu54-conformance-", scratch.name)
                self.assertTrue(scratch.is_dir())
                if command[0] in ("cmake", "ctest"):
                    output = Path(command[command.index("-B") + 1] if "-B" in command else command[2])
                    self.assertEqual(output, scratch / "firmware")
                    output.mkdir(exist_ok=True)
                if command[0] == "uv":
                    self.assertEqual(Path(env["UV_CACHE_DIR"]).parent, scratch)
                    self.assertEqual(Path(env["UV_PROJECT_ENVIRONMENT"]).parent, scratch)
                    self.assertIn(str(scratch), env["PYTEST_ADDOPTS"])
                return CompletedProcess(command, exit_code if command[0] == "ctest" else 0, "", "failure")

            with self.subTest(exit_code=exit_code), patch.object(harness, "CHECKS", checks), \
                    patch.object(harness.shutil, "which", return_value="/tool"), \
                    patch.object(harness.subprocess, "run", side_effect=AssertionError("unowned subprocess")), \
                    patch.object(harness, "run_command", run_command, create=True):
                self.assertEqual(harness.main([]), int(bool(exit_code)))
            self.assertTrue(homes)
            self.assertTrue(all(not home.exists() for home in homes))

    def test_skip_and_interrupt_cleanup_preserve_result_semantics(self):
        command = harness.Check("interrupt", ["probe"], ".", ["probe"])
        with patch.object(harness, "CHECKS", [command]), patch.object(harness.shutil, "which", return_value=None):
            self.assertEqual(harness.main([]), 0)
            self.assertEqual(harness.main(["--strict"]), 1)

        observed = []

        def interrupted(_, *, env, **_ignored):
            observed.append(Path(env["TMPDIR"]).parent)
            raise SystemExit(143)

        with patch.object(harness, "CHECKS", [command]), patch.object(harness.shutil, "which", return_value="/probe"), \
                patch.object(harness.subprocess, "run", side_effect=AssertionError("unowned subprocess")), \
                patch.object(harness, "run_command", interrupted, create=True):
            with self.assertRaises(SystemExit) as interrupted_run:
                harness.main([])
        self.assertEqual(interrupted_run.exception.code, 143)
        self.assertTrue(observed and not observed[0].exists())

    def test_go_dependencies_use_existing_archives_read_only_and_compile_in_owned_cache(self):
        with tempfile.TemporaryDirectory() as outer:
            root = Path(outer)
            source = root / "existing-modules"
            (source / "cache" / "download").mkdir(parents=True)
            owned = root / "owned"
            defaults = {"GOMODCACHE": str(source), "GOPROXY": "off"}
            with patch.object(harness, "run_environment", side_effect=lambda _work, environment: dict(environment)), \
                    patch.object(harness, "run_command", return_value=CompletedProcess([], 0, json.dumps(defaults), "")):
                env = harness.go_environment(owned, {})
            self.assertEqual(env["GOMODCACHE"], str(owned / "modules"))
            self.assertEqual(env["GOPROXY"], (source / "cache" / "download").resolve().as_uri() + "|off")

    def test_forge_child_inherits_owned_outputs_and_failure_paths(self):
        forge = next(check for check in harness.CHECKS if "forge" in check.tools)
        paths = []

        def run_command(command, *, env, **_):
            self.assertEqual(command[:3], ["python3", "scripts/contract_forge.py", "p06"])
            work = Path(env["TMPDIR"]).parent
            for name in ("FOUNDRY_OUT", "FOUNDRY_CACHE_PATH", "FOUNDRY_TEST_FAILURES_FILE",
                         "FOUNDRY_FUZZ_FAILURE_PERSIST_DIR", "FOUNDRY_INVARIANT_FAILURE_PERSIST_DIR"):
                self.assertEqual(Path(env[name]).parent, work)
            paths.append(work)
            return CompletedProcess(command, 0, "", "")

        with patch.dict(os.environ, {"FOUNDRY_SOLC": "/provisioned/solc"}), \
                patch.object(harness, "CHECKS", [forge]), patch.object(harness.shutil, "which", return_value="/forge"), \
                patch.object(harness, "run_command", run_command):
            self.assertEqual(harness.main(["--strict"]), 0)
        self.assertTrue(paths and not paths[0].exists())

    def test_offline_python_borrows_only_a_lock_checked_environment(self):
        with tempfile.TemporaryDirectory() as outer:
            root = Path(outer)
            borrowed = root / "approved"
            (borrowed / "bin").mkdir(parents=True)
            python = borrowed / "bin/python"
            python.write_text("caller marker")
            work = root / "owns"
            env = {"UV_OFFLINE": "1", "UV_PROJECT_ENVIRONMENT": str(borrowed),
                   "UV_CACHE_DIR": str(root / "caller-cache"), "UV_FIND_LINKS": str(root / "wheelhouse"),
                   "PYTEST_ADDOPTS": "-o cache_dir=caller-pytest --basetemp caller-temp"}
            command = next(check.cmd for check in harness.CHECKS if check.cmd[0] == "uv")
            for code in (0, 1):
                with patch.object(harness, "run_command", return_value=CompletedProcess([], code, "", "")) as probe:
                    options, selection = harness.python_conformance([*command], work, root, env)
                self.assertIn("--locked", probe.call_args.args[0])
                self.assertIn("--check", probe.call_args.args[0])
                self.assertEqual(selection["UV_PROJECT_ENVIRONMENT"], str(borrowed if code == 0 else work / "venv"))
                self.assertEqual(selection["UV_FIND_LINKS"], env["UV_FIND_LINKS"])
                self.assertEqual(selection["UV_CACHE_DIR"], env["UV_CACHE_DIR"])
                self.assertEqual("--no-sync" in options, code == 0)
                self.assertTrue(selection["PYTEST_ADDOPTS"].endswith(env["PYTEST_ADDOPTS"]))
                self.assertEqual(python.read_text(), "caller marker")

    def test_all_fourteen_checks_still_registered(self):
        self.assertEqual(len(harness.CHECKS), 14)

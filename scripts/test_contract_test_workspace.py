"""Regression coverage for the ordinary contract Make test recipes."""

import os
from pathlib import Path
import signal
import tempfile
import unittest
from unittest.mock import patch

import contract_test_workspace as contracts


def fake_guarded(run):
    def guarded(product, args, _work, env):
        return run(["python", "-B", "contract_forge.py", product, *args], env=env, cwd=contracts.PROJECTS[product])
    return guarded


class ContractTestWorkspaceTest(unittest.TestCase):
    def test_normal_entries_share_only_owned_artifacts_and_reap_each_run(self):
        for product, expected in (
            ("p06", ["test", "snapshot"]),
            ("p08", ["build", "test", "go"]),
        ):
            with self.subTest(product=product):
                calls = []

                def run(command, env, **kwargs):
                    calls.append((command, env, kwargs["cwd"]))
                    if command[0] == "go":
                        self.assertTrue(Path(env["GOCACHE"]).is_dir())
                    self.assertTrue(Path(env["FOUNDRY_OUT"]).parent.is_dir())

                environment = {key: value for key, value in os.environ.items()
                               if key not in {"FOUNDRY_OUT", "FOUNDRY_CACHE_PATH", "GOCACHE", "GOTMPDIR"}}
                with patch.object(contracts, "check_compatibility") as compatibility, patch.object(
                    contracts, "run_command", side_effect=run,
                ):
                    with patch.object(contracts, "run_guarded_test", side_effect=fake_guarded(run)):
                        contracts.test_product(product, ["--no-match-test", "testFuzz"], environment)
                compatibility.assert_called_once()
                self.assertEqual([
                    "go" if args[0] == "go" else args[4]
                    for args, _, _ in calls
                ], expected)
                paths = {(env["FOUNDRY_OUT"], env["FOUNDRY_CACHE_PATH"]) for _, env, _ in calls}
                self.assertEqual(len(paths), 1)
                out, cache = paths.pop()
                self.assertFalse(Path(out).parent.exists())
                self.assertFalse(Path(cache).exists())
                self.assertTrue(all(path == contracts.PROJECTS[product] for _, _, path in calls))

    def test_failure_and_term_preserve_explicit_paths(self):
        with tempfile.TemporaryDirectory() as directory:
            explicit = Path(directory)
            for interrupted in (False, True):
                with self.subTest(interrupted=interrupted):
                    def run(_command, env, **_kwargs):
                        self.assertEqual(env["FOUNDRY_OUT"], str(explicit))
                        self.assertEqual(env["GOFLAGS"], "-count=1")
                        cache = Path(env["FOUNDRY_CACHE_PATH"])
                        cache.mkdir()
                        observed.append(cache)
                        if interrupted:
                            signal.raise_signal(signal.SIGTERM)
                        raise ValueError("injected")

                    observed = []
                    with patch.object(contracts, "check_compatibility"), patch.object(
                        contracts, "run_guarded_test", side_effect=fake_guarded(run),
                    ):
                        with self.assertRaises(SystemExit if interrupted else ValueError) as failure:
                            contracts.test_product("p06", [], {
                                "FOUNDRY_OUT": str(explicit), "GOFLAGS": "-count=1",
                            })
                    if interrupted:
                        self.assertEqual(failure.exception.code, 143)
                    self.assertTrue(explicit.is_dir())
                    self.assertFalse(observed[0].parent.exists())

    def test_relative_caller_paths_are_resolved_from_the_product_for_go(self):
        with patch.object(contracts, "check_compatibility"), patch.object(contracts, "run_command") as run, patch.object(
            contracts, "run_guarded_test", side_effect=lambda *args: None,
        ):
            contracts.test_product("p08", [], {
                **os.environ, "FOUNDRY_OUT": "custom-output", "FOUNDRY_CACHE_PATH": "/tmp/custom-cache",
            })
        go = run.call_args
        self.assertEqual(go.args[0], ["go", "test", "./..."])
        self.assertEqual(go.kwargs["env"]["FOUNDRY_OUT"], str(contracts.PROJECTS["p08"] / "custom-output"))
        self.assertEqual(go.kwargs["env"]["FOUNDRY_CACHE_PATH"], "/tmp/custom-cache")

    def test_two_make_runs_isolate_and_reap_real_child_groups(self):
        import json
        import shutil
        import subprocess
        import sys
        import time

        with tempfile.TemporaryDirectory() as directory:
            checkout = Path(directory)
            scripts = checkout / "scripts"
            scripts.mkdir()
            project = checkout / "products/p06-stablenet-contracts"
            project.mkdir(parents=True)
            for name in ("contract_test_workspace.py", "integration_workspace.py", "check_contract_compatibility.py",
                         "install_contract_compiler.py"):
                shutil.copyfile(contracts.ROOT / "scripts" / name, scripts / name)
            for name in ("Makefile", "compiler-compatibility.json"):
                shutil.copyfile(contracts.PROJECTS["p06"] / name, project / name)
            (scripts / "contract_forge.py").write_text("""
def run_guarded_test(*args):
    raise AssertionError("fixture failed before reaching guarded tests")

if __name__ == "__main__":
    import json, os, pathlib, sys, time
    out = pathlib.Path(os.environ["FOUNDRY_OUT"])
    cache = pathlib.Path(os.environ["FOUNDRY_CACHE_PATH"])
    out.mkdir(exist_ok=True)
    cache.mkdir(exist_ok=True)
    marker = pathlib.Path(os.environ["TEST_COMPILER_READY"])
    marker.write_text(json.dumps([str(out), str(cache), os.getpid()]))
    if os.environ["TEST_COMPILER_MODE"] == "stop":
        time.sleep(60)
    sys.exit(17)
""")
            original = project / "out"
            original.mkdir()
            sentinel = original / "existing"
            sentinel.write_text("caller")
            env = {**os.environ, "TMPDIR": str(checkout)}
            for key in ("FOUNDRY_OUT", "FOUNDRY_CACHE_PATH"):
                env.pop(key, None)

            def launch(name, mode):
                marker = checkout / f"{name}.json"
                child = subprocess.Popen(
                    ["make", "-C", str(project), "test"],
                    env={**env, "TEST_COMPILER_READY": str(marker), "TEST_COMPILER_MODE": mode},
                    stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True,
                )
                self.addCleanup(self.reap_make, child)
                deadline = time.monotonic() + 10
                while not marker.exists() and child.poll() is None and time.monotonic() < deadline:
                    time.sleep(.01)
                self.assertTrue(marker.exists(), "normal recipe never reached the compiler")
                deadline = time.monotonic() + 3
                while not marker.stat().st_size and time.monotonic() < deadline:
                    time.sleep(.01)
                out, cache, pid = json.loads(marker.read_text())
                return child, Path(out).parent, Path(cache).parent, pid

            first, first_out, first_cache, first_pid = launch("first", "stop")
            second, second_out, second_cache, _ = launch("second", "fail")
            self.assertNotEqual(first_out, second_out)
            self.assertNotEqual(first_cache, second_cache)
            self.assertNotEqual(second.wait(timeout=10), 0)
            self.assertFalse(second_out.exists())
            self.assertFalse(second_cache.exists())
            self.assertTrue(first_out.exists())
            first.terminate()
            self.assertNotEqual(first.wait(timeout=10), 0)
            deadline = time.monotonic() + 5
            while first_out.exists() and time.monotonic() < deadline:
                time.sleep(.01)
            self.assertFalse(first_out.exists())
            self.assertFalse(first_cache.exists())
            with self.assertRaises(ProcessLookupError):
                os.kill(first_pid, 0)
            self.assertEqual(sentinel.read_text(), "caller")
            self.assertEqual(list(checkout.glob("nu54-*")), [])
            self.assertFalse((scripts / "__pycache__").exists())

    @staticmethod
    def reap_make(child):
        import subprocess
        if child.poll() is None:
            os.killpg(child.pid, signal.SIGTERM)
            try:
                child.wait(timeout=5)
            except subprocess.TimeoutExpired:
                child.kill()
                child.wait(timeout=5)

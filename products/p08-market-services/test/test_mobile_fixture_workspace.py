"""Process and storage regression for the P08 mobile fixture entrypoint."""

import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
import unittest


FIXTURE = Path(__file__).resolve().with_name("mobile-fixture.sh")
sys.path.insert(0, str(FIXTURE.parents[3] / "scripts"))
from integration_workspace import OwnedProcess
COMMAND = r"""
import json, os, pathlib, shutil, sys, time
name = pathlib.Path(sys.argv[0]).name
args = sys.argv[1:]
binary = pathlib.Path(os.environ.get("P08_MOBILE_FIXTURE_MARKETD", "unused"))
if name == "anvil":
    binary.with_name("anvil.pid").write_text(str(os.getpid()))
    time.sleep(600)
elif name == "cast":
    if args[:1] == ["chain-id"]:
        if not binary.with_name("anvil.pid").is_file():
            raise SystemExit(1)
        print("8283")
    elif args[:2] in (["wallet","private-key"], ["wallet","address"]):
        print("0x" + "42"*32)
elif name == "curl":
    raise SystemExit(not binary.with_name("marketd.pid").is_file())
elif name == "go":
    if args[:1] == ["env"]:
        print(json.dumps({"GOENV": "off", "GOCACHEPROG": ""}))
    else:
        shutil.copyfile(pathlib.Path(__file__).with_name("fake-marketd"), args[args.index("-o")+1])
        pathlib.Path(args[args.index("-o")+1]).chmod(0o700)
        pathlib.Path(os.environ["GOCACHE"], "probe").touch()
        pathlib.Path(os.environ["GOTMPDIR"], "probe").touch()
elif name == "python3" and args[:1] == ["../../scripts/contract_forge.py"]:
    out = pathlib.Path(os.environ["FOUNDRY_OUT"], "Market.sol")
    out.mkdir(parents=True)
    (out/"Market.json").write_text("market artifact")
    cache = pathlib.Path(os.environ["FOUNDRY_CACHE_PATH"])
    cache.mkdir()
    (cache/"probe").touch()
elif name == "marketd":
    if args == ["deploy"]:
        if not pathlib.Path(os.environ["P08_ARTIFACT"]).is_file():
            raise SystemExit("artifact unavailable")
        print(json.dumps({"venue":{"address":"venue"},"quote":{"address":"asset"}}))
    elif args == ["serve"]:
        binary.with_name("marketd.pid").write_text(str(os.getpid()))
        time.sleep(600)
else:
    os.execv(sys.executable, [sys.executable,*args])
"""

CALLBACK = r"""
import json, os, pathlib, sys, time
binary = pathlib.Path(os.environ["P08_MOBILE_FIXTURE_MARKETD"])
keys = ("P08_MOBILE_FIXTURE_MARKETD","P08_ARTIFACT","FOUNDRY_OUT","FOUNDRY_CACHE_PATH","GOCACHE","GOTMPDIR","TMPDIR")
result = {key:os.environ[key] for key in keys}
result["cwd"] = os.getcwd()
result["args"] = sys.argv[2:]
result["pids"] = [int(binary.with_name(name+".pid").read_text()) for name in ("anvil","marketd")]
assert binary.is_file() and pathlib.Path(result["P08_ARTIFACT"]).is_file()
path = pathlib.Path(sys.argv[1], "callback.json")
part = path.with_suffix(".tmp")
part.write_text(json.dumps(result))
part.replace(path)
if sys.argv[2] == "wait":
    time.sleep(600)
raise SystemExit(int(sys.argv[2]))
"""


class MobileFixtureWorkspaceTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="nu54-p08-fixture-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        caller = self.root / "caller with spaces"
        caller.mkdir()
        self.caller = caller
        for name in ("python3", "go", "anvil", "cast", "curl", "fake-marketd"):
            file = self.bin / name
            file.write_text(f"#!{sys.executable}\n" + COMMAND)
            file.chmod(0o700)
        self.env = {key: value for key, value in os.environ.items() if key not in (
            "FOUNDRY_OUT", "FOUNDRY_CACHE_PATH", "P08_ARTIFACT", "GOCACHE", "GOTMPDIR", "GOCACHEPROG")}
        self.env.update(PATH=f"{self.bin}:{os.defpath}", TMPDIR=str(self.root))

    def start(self, output, mode, environment):
        child = subprocess.Popen(["bash", FIXTURE, output, sys.executable, "-c", CALLBACK, output, mode, "argument with spaces"],
                                 cwd=self.caller, env=environment, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        self.addCleanup(self.stop, child)
        return child

    @staticmethod
    def stop(child):
        if child.poll() is None:
            child.terminate()
        child.communicate(timeout=8)

    def finish(self, output, child, code):
        _, errors = child.communicate(timeout=12)
        self.assertEqual(code, child.returncode, errors)
        data = json.loads((output / "callback.json").read_text())
        self.assertEqual(self.caller.resolve(), Path(data["cwd"]).resolve())
        self.assertEqual("argument with spaces", data["args"][1])
        self.assertFalse(Path(data["P08_MOBILE_FIXTURE_MARKETD"]).parent.exists())
        for pid in data["pids"]:
            with self.assertRaises(ProcessLookupError):
                os.kill(pid, 0)
        self.assertTrue((output / "profile.json").is_file())
        self.assertTrue((output / "anvil.log").is_file())
        self.assertTrue((output / "marketd.log").is_file())
        self.assertFalse((output / "marketd").exists())
        return data

    def test_normal_failure_and_unique_workspace(self):
        first = self.caller / "--owned-workspace"
        second = self.root / "evidence-b"
        a = self.finish(first, self.start("--owned-workspace", "0", self.env), 0)
        b = self.finish(second, self.start(second, "17", self.env), 17)
        self.assertNotEqual(a["P08_MOBILE_FIXTURE_MARKETD"], b["P08_MOBILE_FIXTURE_MARKETD"])
        for run in (a, b):
            for key in ("P08_ARTIFACT", "FOUNDRY_OUT", "FOUNDRY_CACHE_PATH", "GOCACHE", "GOTMPDIR", "TMPDIR"):
                self.assertFalse(Path(run[key]).exists(), key)

    def test_caller_paths_are_preserved(self):
        output = self.root / "evidence"
        env = self.env.copy()
        for key in ("FOUNDRY_OUT", "FOUNDRY_CACHE_PATH", "GOCACHE", "GOTMPDIR", "P08_ARTIFACT"):
            env[key] = str(self.root / key)
        for key in ("GOCACHE", "GOTMPDIR"):
            Path(env[key]).mkdir()
        Path(env["P08_ARTIFACT"]).touch()
        data = self.finish(output, self.start(output, "0", env), 0)
        for key in ("FOUNDRY_OUT", "FOUNDRY_CACHE_PATH", "GOCACHE", "GOTMPDIR", "P08_ARTIFACT"):
            self.assertEqual(env[key], data[key])
            self.assertTrue(Path(env[key]).exists())

    def test_signals_stop_owned_children_before_reclaiming_workspace(self):
        for signum in (signal.SIGTERM, signal.SIGHUP):
            with self.subTest(signum=signum):
                output = self.root / str(signum)
                child = self.start(output, "wait", self.env)
                deadline = time.monotonic() + 8
                while not (output / "callback.json").exists() and child.poll() is None and time.monotonic() < deadline:
                    time.sleep(0.02)
                self.assertTrue((output / "callback.json").exists())
                child.send_signal(signum)
                self.finish(output, child, 128 + signum)


    def test_outer_owned_fixture_finishes_before_logger(self):
        output = self.root / "outer-evidence"
        logger, fixture = OwnedProcess(), OwnedProcess()
        self.addCleanup(logger.stop)
        self.addCleanup(fixture.stop)
        result_log = self.root / "result.log"
        tee = logger.start(["tee", result_log], stdin=subprocess.PIPE, stdout=subprocess.DEVNULL)
        with tee.stdin:
            child = fixture.start(["bash", FIXTURE, output, sys.executable, "-c", CALLBACK, output, "wait", "argument with spaces"],
                                  cwd=self.caller, env=self.env, stdout=tee.stdin, stderr=subprocess.PIPE, text=True)
        deadline = time.monotonic() + 8
        while not (output / "callback.json").exists() and child.poll() is None and time.monotonic() < deadline:
            time.sleep(0.02)
        self.assertTrue((output / "callback.json").exists())
        fixture.stop()
        self.assertEqual(0, tee.wait(timeout=8))
        self.assertIn("Isolated P08 ready", result_log.read_text())
        self.finish(output, child, 143)


if __name__ == "__main__":
    unittest.main()

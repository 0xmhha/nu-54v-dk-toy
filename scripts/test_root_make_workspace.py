import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import tempfile
import time
import unittest


ROOT = Path(__file__).resolve().parents[1]
PROJECTS = ("products/p01-device-firmware", "products/p02-user-app")


class RootMakeWorkspace(unittest.TestCase):
    def setUp(self):
        self.storage = tempfile.TemporaryDirectory()
        self.addCleanup(self.storage.cleanup)
        self.workspace = Path(self.storage.name)
        self.repo = self.workspace / "checkout"
        self.repo.mkdir()
        shutil.copy2(ROOT / "Makefile", self.repo / "Makefile")
        (self.repo / "scripts").symlink_to(ROOT / "scripts", target_is_directory=True)
        for project in PROJECTS:
            directory = self.repo / project
            directory.mkdir(parents=True)
            (directory / "Makefile").write_text("test:\n\t@python3 -B ../../record.py\n")
        (self.repo / "record.py").write_text(
            "import json, os, pathlib, sys, time\n"
            "project = pathlib.Path.cwd().name\n"
            "keys = ('GOCACHE','GOTMPDIR','TMPDIR','GOMODCACHE','COMPAT')\n"
            "state = {key: os.environ.get(key) for key in keys}\n"
            "state['pid'] = os.getpid()\n"
            "pathlib.Path(state['GOCACHE'], 'fixture').touch()\n"
            "with open(os.environ['REPORT'], 'a') as output:\n"
            "    print(json.dumps({'project': project, **state}), file=output)\n"
            "if os.environ.get('HOLD'):\n"
            "    while not pathlib.Path(os.environ['HOLD']).exists(): time.sleep(.02)\n"
            "if os.environ.get('FAIL') == project: sys.exit(7)\n"
        )
        self.environment = os.environ.copy()
        for key in ('MAKEFLAGS', 'GOCACHE', 'GOTMPDIR'):
            self.environment.pop(key, None)
        self.environment.update(TMPDIR=str(self.workspace), GOMODCACHE=str(self.workspace / "downloads"))
        self.environment["GOENV"] = "off"
        (self.workspace / "downloads").mkdir()

    def launch(self, report, *args, **env):
        cmd = ['make', '-j2', 'test', 'COMPAT=from-command-line', *args]
        child = subprocess.Popen(cmd, cwd=self.repo, start_new_session=True, stdout=subprocess.PIPE,
                                 stderr=subprocess.STDOUT, text=True, env={**self.environment, **env, 'REPORT': str(report)})
        self.addCleanup(self.stop, child)
        return child

    def stop(self, child):
        if child.poll() is None:
            os.killpg(child.pid, signal.SIGHUP)
        child.communicate(timeout=15)

    def wait_records(self, path, count=1):
        for _ in range(500):
            if path.exists():
                records = [json.loads(line) for line in path.read_text().splitlines()]
                if len(records) >= count:
                    return records
            time.sleep(.02)
        self.fail(f"Missing {count} nested make records")

    def assert_clean(self, records):
        for record in records:
            self.assertFalse(Path(record["TMPDIR"]).parent.exists())
            self.assertEqual(record["COMPAT"], "from-command-line")
            self.assertEqual(record["GOMODCACHE"], self.environment["GOMODCACHE"])

    def test_order_project_selector_failure_and_explicit_caches(self):
        report = self.workspace / "order"
        child = self.launch(report, "PROJECTS=" + " ".join(PROJECTS))
        output, _ = child.communicate(timeout=15)
        self.assertEqual(child.returncode, 0, output)
        self.assertNotIn("jobserver unavailable", output)
        recorded = self.wait_records(report, 2)
        self.assertEqual([record["project"] for record in recorded], [Path(project).name for project in PROJECTS])
        self.assert_clean(recorded)
        self.assertEqual(len({record["GOCACHE"] for record in recorded}), 1)

        report = self.workspace / "failure"
        child = self.launch(report, "PROJECTS=" + " ".join(PROJECTS), FAIL=Path(PROJECTS[0]).name)
        output, _ = child.communicate(timeout=15)
        self.assertNotEqual(child.returncode, 0, output)
        records = self.wait_records(report)
        self.assertEqual([record["project"] for record in records], [Path(PROJECTS[0]).name])
        self.assert_clean(records)

        cache, temporary = self.workspace / "caller-cache", self.workspace / "caller-temp"
        cache.mkdir()
        temporary.mkdir()
        report = self.workspace / "explicit"
        child = self.launch(report, "P=p02", f"GOCACHE={cache}", GOTMPDIR=str(temporary))
        output, _ = child.communicate(timeout=15)
        self.assertEqual(child.returncode, 0, output)
        records = self.wait_records(report)
        self.assert_clean(records)
        self.assertEqual(Path(records[0]["GOCACHE"]), cache)
        self.assertEqual(Path(records[0]["GOTMPDIR"]), temporary)
        self.assertEqual(records[0]["project"], Path(PROJECTS[1]).name)
        self.assertTrue((cache / "fixture").exists() and temporary.is_dir())

        if shutil.which("go"):
            config = self.workspace / "go-env"
            content = f"GOCACHE={cache}\nGOTMPDIR={temporary}\n"
            config.write_text(content)
            report = self.workspace / "go-config"
            child = self.launch(report, "P=p02", GOENV=str(config))
            output, _ = child.communicate(timeout=15)
            self.assertEqual(child.returncode, 0, output)
            records = self.wait_records(report)
            self.assert_clean(records)
            self.assertEqual(Path(records[0]["GOCACHE"]), cache)
            self.assertEqual(Path(records[0]["GOTMPDIR"]), temporary)
            self.assertEqual(config.read_text(), content)

    def test_concurrent_calls_and_hangup(self):
        reports = [self.workspace / f"parallel-{i}" for i in range(2)]
        hold = self.workspace / "ready"
        children = [self.launch(report, "P=p02", HOLD=str(hold)) for report in reports]
        recorded = [self.wait_records(report)[0] for report in reports]
        self.assertNotEqual(recorded[0]["GOCACHE"], recorded[1]["GOCACHE"])
        self.assertTrue(all(Path(record["GOCACHE"]).is_dir() for record in recorded))
        os.killpg(children[0].pid, signal.SIGHUP)
        children[0].communicate(timeout=15)
        for _ in range(500):
            if not Path(recorded[0]["TMPDIR"]).parent.exists():
                break
            time.sleep(.02)
        self.assertNotEqual(children[0].returncode, 0)
        self.assert_clean(recorded[:1])
        with self.assertRaises(ProcessLookupError):
            os.kill(recorded[0]["pid"], 0)
        self.assertTrue(Path(recorded[1]["GOCACHE"]).is_dir())
        hold.touch()
        output, _ = children[1].communicate(timeout=15)
        self.assertEqual(children[1].returncode, 0, output)
        self.assert_clean(recorded)


if __name__ == "__main__":
    unittest.main()

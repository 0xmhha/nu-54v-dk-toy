#!/usr/bin/env python3
"""Fault-injected lifetime checks for the iOS scene runners."""

import os
from pathlib import Path
import plistlib
import signal
import stat
import subprocess
import sys
import tempfile
import time
import unittest

HERE = Path(__file__).resolve().parent
STUB = r'''#!{python}
import json, os, pathlib, plistlib, shutil, signal, subprocess, sys, tempfile, time
name = pathlib.Path(sys.argv[0]).name
args = sys.argv[1:]
mock = pathlib.Path(os.environ["SCENE_MOCK"])
def at(flag):
    return pathlib.Path(args[args.index(flag) + 1])
if name == "ditto":
    shutil.copytree(*args)
elif name == "mktemp":
    path = tempfile.mkdtemp(prefix=pathlib.Path(args[-1]).name.split(".")[0], dir=mock)
    (mock / "scratch-path").write_text(path)
    print(path)
elif name == "mkdir":
    if any("xcode-environment" in arg for arg in args):
        sys.exit(74)
    os.execv("/bin/mkdir", ["/bin/mkdir", *args])
elif name == "python3":
    if args and args[0] == "-" and os.environ.get("SCENE_SPEC_ALLOCATION"):
        real_mkstemp = tempfile.mkstemp
        def delayed_spec(suffix=None, prefix=None, dir=None, text=False):
            runner = args[1] if len(args) > 1 else ""
            source = os.environ.get(f"NU54_{{runner}}_XCTESTRUN")
            if runner not in ("K10", "W12") or not source:
                sys.exit(75)
            caller = pathlib.Path(source).resolve().parent
            destination = pathlib.Path(dir).resolve() if dir is not None else pathlib.Path.cwd().resolve()
            if destination != caller or mock.resolve() not in caller.parents:
                sys.exit(75)
            created = real_mkstemp(suffix=suffix, prefix=prefix, dir=destination, text=text)
            (mock / "allocating-spec").write_text(str(os.getpid()))
            time.sleep(0.4)
            return created
        tempfile.mkstemp = delayed_spec
        sys.argv = args
        exec(compile(sys.stdin.read(), "<fake allocation>", "exec"))
        sys.exit(0)
    if args and args[0] == "-c":
        os.execv(sys.executable, [sys.executable, *args])
    sys.exit(75)
elif name == "uuidgen":
    sys.exit(76)
elif name == "rm":
    selected = os.environ.get("SCENE_REMOVE_SIGNAL", "")
    if selected and any(
        (pathlib.Path(arg).name.startswith(("K10-", "W12-")) and arg.endswith(".xctestrun")) if selected == "spec"
        else pathlib.Path(arg).name.startswith("nu54-scene.") for arg in args
    ):
        (mock / "removing").write_text(str(os.getpid()))
        time.sleep(0.4)
    fault = os.environ.get("SCENE_REMOVE_FAULT")
    if fault == "1" or fault == "spec" and any(arg.endswith(".xctestrun") for arg in args):
        sys.exit(91)
    os.execv("/bin/rm", ["/bin/rm", *args])
elif name == "ruby":
    pathlib.Path(args[1]).mkdir()
elif name == "curl":
    count = mock / "curl"
    print("pos|2|1|1|1|21|21" if count.exists() else "menu|1|0|0|0|0|21")
    count.touch()
elif name == "xcrun":
    if args[:2] == ["simctl", "uninstall"]:
        (mock / "uninstall").touch()
    if args[:2] == ["simctl", "get_app_container"]:
        sys.exit(0 if os.environ.get("SCENE_INSTALLED") else 1)
    if args[:3] == ["xcresulttool", "get", "test-results"]:
        total = 4 if at("--path").name == "SceneSmoke.xcresult" else 1
        print(json.dumps({{"passedTests": total, "totalTestCount": total}}))
elif name == "xcodebuild":
    temporary = pathlib.Path(os.environ["TMPDIR"]).resolve()
    cache = pathlib.Path(os.environ["XDG_CACHE_HOME"]).resolve()
    assert temporary.name == "tmp" and cache == temporary.parent / "cache"
    assert all(pathlib.Path(os.environ[key]).resolve() == temporary for key in ("TMP", "TEMP"))
    scratch = pathlib.Path(tempfile.mkdtemp(prefix="NU54RenterSpec"))
    assert scratch.parent.resolve() == temporary
    (cache / "build-cache").touch()
    (mock / "build-environment").write_text(str(temporary.parent))
    if os.environ.get("SCENE_FAULT") == "app-build":
        sys.exit(73)
    if args[0] == "build-for-testing":
        tools = at("-derivedDataPath") / "Build" / "Products"
        tools.mkdir(parents=True, exist_ok=True)
        (tools / "SceneSmoke_Mock.xctestrun").write_bytes(
            plistlib.dumps({{"TestConfigurations": [{{"TestTargets": [{{}}]}}]}})
        )
    elif args[0] == "build":
        products = next(pathlib.Path(arg.split("=", 1)[1]) for arg in args if arg.startswith("CONFIGURATION_BUILD_DIR="))
        assert products.is_dir()
        (products / "compiled-local").touch()
    else:
        if args[0] == "test":
            work = at("-derivedDataPath")
            work.mkdir(parents=True, exist_ok=True)
            (work / "compiled-local").touch()
        else:
            spec = at("-xctestrun")
            assert spec.is_file()
            contract = plistlib.loads(spec.read_bytes())
            if spec.name == "SubscriptionEnrolled.xctestrun":
                target = contract["TestConfigurations"][0]["TestTargets"][0]
                assert target["EnvironmentVariables"]["NU54_SUBSCRIPTION_SALE"] == "sale-fixture"
            if spec.name == "SubscriptionAlerts.xctestrun" and "NU54_SUBSCRIPTION_UI_FIXTURE" in os.environ:
                target = contract["TestConfigurations"][0]["TestTargets"][0]
                assert target["EnvironmentVariables"]["NU54_SUBSCRIPTION_UI_FIXTURE"]
            work = spec.parent
        result = at("-resultBundlePath")
        result.mkdir(parents=True)
        (result / "evidence").write_text("original result bundle")
        mode = os.environ.get("SCENE_FAULT", "")
        if mode == "fail":
            sys.exit(73)
        if mode == "signal":
            (mock / "child-group").write_text(str(os.getpgrp()))
            def stopped(signum, frame):
                (mock / "stopped").write_text(str(work.is_dir()))
                sys.exit(99)
            signal.signal(signal.SIGTERM, stopped)
            signal.signal(signal.SIGINT, stopped)
            subprocess.Popen([sys.executable, "-c", """
import pathlib, signal, sys, time
work, mock = map(pathlib.Path, sys.argv[1:])
def stop(signum, frame):
    time.sleep(0.2)
    (mock / "child-stopped").write_text(str(work.is_dir()))
    sys.exit(0)
signal.signal(signal.SIGTERM, stop)
(mock / "child-started").touch()
while True:
    time.sleep(0.05)
""", str(work), str(mock)])
            while not (mock / "child-started").exists():
                time.sleep(0.05)
            (mock / "entered").write_text(str(work))
            while True:
                time.sleep(0.05)
'''


class SceneCleanup(unittest.TestCase):
    def invoke(self, runner, fault="", interrupted=None, preexisting=False, remove_failure=False):
        with tempfile.TemporaryDirectory(prefix="nu54-scene-cleanup-") as tmp:
            root = Path(tmp)
            bins = root / "bin"
            bins.mkdir()
            private = root / "private"
            private.mkdir()
            (private / "shared").mkdir()
            (private / "shared" / "keep").write_text("common cache")
            output = root / "evidence"
            output.mkdir()
            (output / "operator-summary.txt").write_text("caller evidence")
            if preexisting:
                for directory in ("Build", "Products", "UI"):
                    (output / directory).mkdir()
                    (output / directory / "keep").write_text("preexisting build")
            app = root / "Signed Caller.app"
            app.mkdir()
            (app / "Info.plist").write_bytes(plistlib.dumps({
                "CFBundleIdentifier": "com.example.scene",
                "CFBundleExecutable": "Caller",
            }))
            (app / "Caller").write_bytes(b"original signed input")
            fixture = root / "registered fixture"
            fixture.mkdir()
            (fixture / "preview.json").write_text('{"invoice":{"saleRef":"sale-fixture"}}')
            for name in ["xcodebuild", "xcrun", "codesign", "ruby", "ditto", "curl", "rm"]:
                prog = bins / name
                prog.write_text(STUB.format(python=sys.executable))
                prog.chmod(0o700)
            args = {
                "run.sh": ["sim", app, app, output],
                "run-kiosk-mode.sh": ["operator", "terminal", output],
                "run-market.sh": ["sim", app, output, "entry"],
                "run-subscription-native.sh": ["sim", app, output, "testExact"],
                "run-subscription-enrolled.sh": ["sim", app, fixture, output],
            }[runner]
            env = {**os.environ, "PATH": f"{bins}:{os.environ['PATH']}", "TMPDIR": str(private),
                   "TMP": str(private), "TEMP": str(private), "XDG_CACHE_HOME": str(private / "shared"),
                   "SCENE_MOCK": str(root), "SCENE_FAULT": fault,
                   "SCENE_REMOVE_FAULT": "1" if remove_failure else ""}
            env.pop("NU54_SUBSCRIPTION_UI_FIXTURE", None)
            proc = subprocess.Popen(["bash", HERE / runner, *args], env=env, text=True,
                                    stdout=subprocess.PIPE, stderr=subprocess.STDOUT, start_new_session=True)
            try:
                if interrupted:
                    deadline = time.monotonic() + 10
                    while not (root / "entered").exists() and proc.poll() is None and time.monotonic() < deadline:
                        time.sleep(0.05)
                    self.assertTrue((root / "entered").exists())
                    work = Path((root / "entered").read_text()).resolve()
                    top = private / work.relative_to(private.resolve()).parts[0]
                    self.assertEqual(stat.S_IMODE(top.stat().st_mode), 0o700)
                    proc.send_signal(interrupted)
                output_log, _ = proc.communicate(timeout=15)
            finally:
                child_group = root / "child-group"
                if child_group.exists() and not (root / "child-stopped").exists():
                    try:
                        os.killpg(int(child_group.read_text()), signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                if proc.poll() is None:
                    os.killpg(proc.pid, signal.SIGTERM)
                    proc.communicate(timeout=5)
                if proc.stdout is not None:
                    proc.stdout.close()
            expected = 128 + interrupted if interrupted else 73 if fault else 91 if remove_failure else 0
            self.assertEqual(proc.returncode, expected, output_log)
            self.assertEqual((app / "Caller").read_bytes(), b"original signed input")
            self.assertEqual((private / "shared" / "keep").read_text(), "common cache")
            self.assertEqual((output / "operator-summary.txt").read_text(), "caller evidence")
            self.assertEqual((fixture / "preview.json").read_text(), '{"invoice":{"saleRef":"sale-fixture"}}')
            build_environment = Path((root / "build-environment").read_text())
            self.assertEqual(build_environment.exists(), remove_failure)
            self.assertTrue(list(output.rglob("*.xcresult/evidence")), output_log)
            if remove_failure:
                self.assertIn("could not remove private build directory", output_log)
                self.assertEqual(len(list(private.glob("nu54-scene.*"))), 1)
            else:
                self.assertEqual(sorted(p.name for p in private.iterdir()), ["shared"])
            leftovers = sorted(
                str(p.relative_to(output)) for p in output.rglob("*")
                if (p.is_dir() and p.name in {"Build", "Products", "UI", "NativeAlerts.xcodeproj"})
                or (p.is_file() and p.suffix == ".xctestrun")
            )
            self.assertEqual(leftovers, ["Build", "Products", "UI"] if preexisting else [])
            if preexisting:
                for directory in leftovers:
                    self.assertEqual((output / directory / "keep").read_text(), "preexisting build")
            if interrupted:
                self.assertEqual((root / "stopped").read_text(), "True")
                self.assertEqual((root / "child-stopped").read_text(), "True")
            if runner == "run-subscription-enrolled.sh":
                self.assertEqual((fixture / "ui-completed").exists(), not fault)

    def test_repeated_signals_during_exit_cleanup(self):
        cases = [("shared", "", 0), ("shared", "fail", 73),
                 ("spec", "", 0), ("spec", "fail", 73), ("scratch", "", 0)]
        for phase, fault, expected in cases:
            with self.subTest(phase=phase, fault=fault), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp).resolve()
                caller = root / "caller"
                caller.mkdir()
                source = caller / "Caller.xctestrun"
                source.write_bytes(plistlib.dumps({"SceneSmoke": {}}))
                original = source.read_bytes()
                concurrent = caller / "K10-concurrent.xctestrun"
                concurrent.write_text("another invocation")
                app = caller / "Caller.app"
                app.mkdir()
                (app / "signed").write_text("caller input")
                private = root / "private"
                private.mkdir()
                for name in ("rm", "xcrun", "xcodebuild"):
                    tool = root / name
                    tool.write_text(STUB.format(python=sys.executable))
                    tool.chmod(0o700)
                env = {**os.environ, "PATH": f"{root}:{os.environ['PATH']}", "TMPDIR": str(private),
                       "SCENE_MOCK": str(root), "SCENE_FAULT": fault,
                       "SCENE_REMOVE_SIGNAL": "scratch" if phase == "shared" else phase, "SCENE_REMOVE_FAULT": "",
                       "NU54_K10_XCTESTRUN": str(source), "NU54_K10_APP": str(app),
                       "NU54_K10_SIM": "sim", "NU54_K10_TOKEN": "local-fixture"}
                for flag in ("NU54_K10_SIGNED_REVIEW", "NU54_W10_INSTALLED_BOARD",
                             "NU54_W10_INSTALLED_SETTLEMENT", "NU54_W11_INSTALLED_PICKUP"):
                    env[flag] = "1" if phase == "scratch" else ""
                if phase == "shared":
                    command = ["bash", "-c", 'set -eu; source "$1"; scene_prepare; exit "$2"',
                               "scene", HERE / "private-build.sh", str(expected)]
                else:
                    command = ["bash", HERE / "run-k10-installed.sh", "http://127.0.0.1:1"]
                with subprocess.Popen(command, env=env, cwd=root, start_new_session=True,
                                      stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True) as proc:
                    try:
                        deadline = time.monotonic() + 7
                        while not (root / "removing").is_file() and proc.poll() is None and time.monotonic() < deadline:
                            time.sleep(0.02)
                        self.assertTrue((root / "removing").is_file())
                        for sig in (signal.SIGHUP, signal.SIGINT, signal.SIGTERM):
                            proc.send_signal(sig)
                            try:
                                os.kill(int((root / "removing").read_text()), sig)
                            except ProcessLookupError:
                                pass
                            time.sleep(0.03)
                        log, _ = proc.communicate(timeout=8)
                    finally:
                        if proc.poll() is None:
                            proc.kill()
                            proc.communicate(timeout=5)
                outcome = proc.returncode, sorted(p.name for p in private.iterdir()), sorted(p.name for p in caller.iterdir())
                self.assertEqual(outcome, (expected, [], ["Caller.app", "Caller.xctestrun", concurrent.name]), log)
                if phase == "scratch":
                    self.assertIn("W11 installed original nearby pickup receipt: one passing test", log)
                self.assertEqual(source.read_bytes(), original)
                self.assertEqual(concurrent.read_text(), "another invocation")
                self.assertEqual((app / "signed").read_text(), "caller input")

    def test_faulty_spec_allocation_cannot_escape_or_write_to_working_directory(self):
        with tempfile.TemporaryDirectory(prefix="nu54-scene-allocation-") as tmp:
            outside = Path(tmp).resolve()
            root = outside / "fixture"
            root.mkdir()
            caller = root / "caller"
            caller.mkdir()
            source = caller / "Caller.xctestrun"
            source.write_bytes(plistlib.dumps({"SceneSmoke": {}}))
            stub = root / "python3"
            stub.write_text(STUB.format(python=sys.executable))
            stub.chmod(0o700)
            env = {**os.environ, "SCENE_MOCK": str(root), "SCENE_SPEC_ALLOCATION": "1",
                   "NU54_W12_XCTESTRUN": str(source)}
            faulty = "import sys, tempfile; tempfile.mkstemp(dir=sys.argv[2], prefix='W12-', suffix='.xctestrun')"
            for destination in (outside, root):
                with self.subTest(destination=destination.name):
                    result = subprocess.run([stub, "-", "W12", destination], input=faulty, env=env, cwd=root,
                                            capture_output=True, text=True, timeout=5)
                    self.assertEqual(result.returncode, 75, result.stderr)
                    self.assertFalse(list(destination.glob("W12-*.xctestrun")))

    def test_installed_runners_own_spec_during_allocation_and_exit(self):
        cases = [(name, "allocate", sig, "", 128 + sig) for name in ("K10", "W12")
                 for sig in (signal.SIGHUP, signal.SIGINT, signal.SIGTERM)]
        cases += [("W12", "remove", signal.SIGHUP, "", 0), ("W12", "remove", signal.SIGHUP, "fail", 73),
                  ("W12", "remove-error", None, "", 91), ("W12", "remove-error", None, "fail", 73)]
        for name, phase, sig, fault, expected in cases:
            with self.subTest(runner=name, phase=phase, sig=sig, fault=fault), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp).resolve()
                private = root / "private"
                private.mkdir()
                caller = root / "caller"
                caller.mkdir()
                source = caller / "Caller.xctestrun"
                original = plistlib.dumps({"SceneSmoke": {}})
                source.write_bytes(original)
                peer = caller / f"{name}-other.xctestrun"
                peer.write_text("concurrent run")
                app = caller / "Caller.app"
                app.mkdir()
                (app / "signed").write_text("caller input")
                for command_name in ("rm", "xcrun", "xcodebuild", "python3"):
                    tool = root / command_name
                    tool.write_text(STUB.format(python=sys.executable))
                    tool.chmod(0o700)
                env = {**os.environ, "PATH": f"{root}:{os.environ['PATH']}", "TMPDIR": str(private),
                       "SCENE_MOCK": str(root), "SCENE_FAULT": fault,
                       "SCENE_SPEC_ALLOCATION": "1",
                       "SCENE_REMOVE_SIGNAL": "spec" if phase == "remove" else "",
                       "SCENE_REMOVE_FAULT": "spec" if phase == "remove-error" else "",
                       f"NU54_{name}_XCTESTRUN": str(source), f"NU54_{name}_APP": str(app),
                       f"NU54_{name}_SIM": "sim", f"NU54_{name}_TOKEN": "local-fixture"}
                for flag in ("NU54_K10_SIGNED_REVIEW", "NU54_W10_INSTALLED_BOARD",
                             "NU54_W10_INSTALLED_SETTLEMENT", "NU54_W11_INSTALLED_PICKUP"):
                    env.pop(flag, None)
                with subprocess.Popen(["bash", HERE / f"run-{name.lower()}-installed.sh", "http://127.0.0.1:1"],
                                      env=env, cwd=root, start_new_session=True, stdout=subprocess.PIPE,
                                      stderr=subprocess.STDOUT, text=True) as proc:
                    try:
                        if sig is not None:
                            marker = root / ("allocating-spec" if phase == "allocate" else "removing")
                            deadline = time.monotonic() + 7
                            while (not marker.is_file() or not marker.stat().st_size) and proc.poll() is None and time.monotonic() < deadline:
                                time.sleep(0.02)
                            self.assertTrue(marker.is_file())
                            child = int(marker.read_text())
                            proc.send_signal(sig)
                            try:
                                os.kill(child, sig)
                            except ProcessLookupError:
                                pass
                            time.sleep(0.04)
                            proc.send_signal(signal.SIGTERM)
                        log, _ = proc.communicate(timeout=8)
                    finally:
                        if proc.poll() is None:
                            proc.kill()
                            proc.communicate(timeout=5)
                copies = set(caller.glob(f"{name}-*.xctestrun")) - {peer}
                actual = proc.returncode, list(private.iterdir()), len(copies)
                self.assertEqual(actual, (expected, [], int(phase == "remove-error")), log)
                self.assertEqual(source.read_bytes(), original)
                self.assertEqual(peer.read_text(), "concurrent run")
                self.assertEqual((app / "signed").read_text(), "caller input")
                if phase == "allocate":
                    self.assertFalse((root / "build-environment").exists())

    def test_run_outputs_and_failures(self):
        for runner in ["run.sh", "run-kiosk-mode.sh", "run-market.sh",
                       "run-subscription-native.sh", "run-subscription-enrolled.sh"]:
            for fault in ["", "fail"]:
                with self.subTest(runner=runner, fault=fault):
                    self.invoke(runner, fault)

    def test_caller_owned_output_directories_remain(self):
        self.invoke("run.sh", preexisting=True)
        self.invoke("run-subscription-enrolled.sh", preexisting=True)

    def test_cleanup_failure_does_not_hide_run_status(self):
        self.invoke("run.sh", remove_failure=True)
        self.invoke("run.sh", "fail", remove_failure=True)
        self.invoke("run.sh", "signal", signal.SIGTERM, remove_failure=True)

    def test_signal_preserves_result_and_original_status(self):
        for runner in ["run.sh", "run-subscription-enrolled.sh"]:
            for interrupted in [signal.SIGHUP, signal.SIGINT, signal.SIGTERM]:
                with self.subTest(runner=runner, interrupted=interrupted):
                    self.invoke(runner, "signal", interrupted)

    def test_direct_app_build_failure_removes_build_environment(self):
        with tempfile.TemporaryDirectory(prefix="nu54-scene-cleanup-") as tmp:
            root = Path(tmp)
            derived = root / "caller-derived"
            derived.mkdir()
            (derived / "keep").touch()
            for name in ("xcodebuild", "xcrun"):
                tool = root / name
                tool.write_text(STUB.format(python=sys.executable))
                tool.chmod(0o700)
            env = {**os.environ, "PATH": f"{root}:{os.environ['PATH']}", "TMPDIR": tmp,
                   "TMP": tmp, "TEMP": tmp, "XDG_CACHE_HOME": tmp, "SCENE_MOCK": tmp,
                   "SCENE_FAULT": "app-build"}
            output = root / "result"
            completed = subprocess.run(
                ["bash", HERE / "run-travel-visits.sh", "sim", derived, output],
                env=env, capture_output=True, text=True, timeout=15,
            )
            self.assertTrue((output / "app-build.log").is_file(), completed.stderr)
            log = (output / "app-build.log").read_text()
            self.assertEqual(completed.returncode, 73, log)
            self.assertFalse(Path((root / "build-environment").read_text()).exists())
            self.assertEqual(list(derived.iterdir()), [derived / "keep"])

    def test_kiosk_early_failure_preserves_installed_app_and_cleans_scratch(self):
        for runner in ("run-kiosk-offline.sh", "run-kiosk-outcome.sh"):
            for installed, expected in [("1", 2), ("", 74)]:
                with self.subTest(runner=runner, installed=bool(installed)):
                    self.check_kiosk_early_failure(runner, installed, expected)

    def check_kiosk_early_failure(self, runner, installed, expected):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for name in ("mktemp", "xcrun", "mkdir", "python3"):
                tool = root / name
                tool.write_text(STUB.format(python=sys.executable))
                tool.chmod(0o700)
            env = {**os.environ, "PATH": f"{root}:{os.environ['PATH']}", "TMPDIR": tmp,
                   "SCENE_MOCK": tmp, "SCENE_INSTALLED": installed}
            output = root / "result"
            arguments = ["bash", HERE / runner, "a" * 36]
            app = root / "Caller.app"
            app.mkdir()
            (app / "Info.plist").write_bytes(plistlib.dumps({"CFBundleIdentifier": "com.nu54kiosk.outcomeqa"}))
            (app / "signed").write_text("caller input")
            if runner == "run-kiosk-outcome.sh":
                arguments.append(app)
            completed = subprocess.run(
                [*arguments, output], env=env, capture_output=True, text=True, timeout=15,
            )
            self.assertEqual(completed.returncode, expected, completed.stderr)
            self.assertFalse((root / "uninstall").exists())
            self.assertEqual((app / "signed").read_text(), "caller input")
            if installed:
                self.assertFalse((root / "scratch-path").exists())
            else:
                self.assertIn("could not write kiosk failure report", completed.stderr)
                self.assertFalse(Path((root / "scratch-path").read_text()).exists())

    def test_direct_runners_clean_scratch_on_initialization_failure(self):
        runners = ("run-itinerary-conflict.sh", "run-subscription-denied.sh", "run-travel-booking-seats.sh",
                   "run-travel-discovery.sh", "run-travel-erasure.sh", "run-travel-pickup.sh",
                   "run-travel-visits.sh", "run-kiosk-k07.sh")
        for runner in runners:
            for early in (False, True) if runner != "run-kiosk-k07.sh" else (False,):
                with self.subTest(runner=runner, early=early), tempfile.TemporaryDirectory() as tmp:
                    root = Path(tmp)
                    output = root / "result"
                    fixture = root / "fixture"
                    fixture.mkdir(mode=0o700)
                    (fixture / "caller").touch()
                    app = root / "Kiosk.app"
                    app.mkdir()
                    (app / "Info.plist").write_bytes(plistlib.dumps({"CFBundleIdentifier": "com.nu54kiosk.k07.mock"}))
                    derived = root / "caller-derived"
                    derived.mkdir()
                    (derived / "caller").touch()
                    mocked = ("mktemp", "mkdir", "xcrun", "uuidgen") if early else ("mktemp", "mkdir", "xcrun")
                    for name in mocked:
                        tool = root / name
                        tool.write_text(STUB.format(python=sys.executable))
                        tool.chmod(0o700)
                    args = {
                        "run-itinerary-conflict.sh": [derived, output, "20000"],
                        "run-subscription-denied.sh": [derived, fixture, output],
                        "run-travel-pickup.sh": [derived, derived, output],
                        "run-kiosk-k07.sh": [app, output],
                    }.get(runner, [derived, output])
                    env = {**os.environ, "PATH": f"{root}:{os.environ['PATH']}", "SCENE_MOCK": tmp}
                    for key in ("SCENE_INSTALLED", "NU54_BOOKING_CATALOG_PORT", "NU54_SUBSCRIPTION_PAIR_ANDROID"):
                        env.pop(key, None)
                    completed = subprocess.run(
                        ["bash", HERE / runner, "sim", *args], env=env, capture_output=True,
                        text=True, timeout=15,
                    )
                    self.assertEqual(completed.returncode, 76 if early else 74, completed.stderr)
                    self.assertFalse(Path((root / "scratch-path").read_text()).exists())
                    self.assertFalse((root / "uninstall").exists())
                    self.assertEqual(list(derived.iterdir()), [derived / "caller"])
                    self.assertEqual(list(fixture.iterdir()), [fixture / "caller"])


if __name__ == "__main__":
    unittest.main()

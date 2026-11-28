#!/usr/bin/env python3
"""Run the kiosk app on an Android emulator against a local store, with a cafe menu set on the operations page.

The server, database, chain and indexer are the ones of the order flow; the operations page is served by the same
server and driven by a browser; the kiosk's screens are real and only its native modules are stood in for.
See integration-workspace.md.
"""

import argparse
from contextlib import contextmanager
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import time
import urllib.request

if __name__ == "__main__":
    sys.dont_write_bytecode = True

import app_build_record as build_record
from integration_workspace import OwnedProcess, cleanup_stack, integration_workspace, run_command
import test_order_flow as flow
from simulate_user_app import REMEDY, STALE_EXIT, STALE_MARK, wait_for  # one wording and one exit code for both apps

SIMULATOR = flow.FLOW / "simulator"
KIOSK = flow.ROOT / "products/p04-merchant-kiosk"
WEB = flow.ROOT / flow.P05 / "web"
HOST, BUNDLER = 8790, 8786  # not 8081, where the apps of other projects look for a bundler
MINUTES = 45
PACKAGE = "com.nu54kiosk"
APK = "products/p04-merchant-kiosk/android/app/build/outputs/apk/debug/app-debug.apk"


def step(name):
    request = urllib.request.Request(f"http://127.0.0.1:{HOST}/step", json.dumps({"step": name}).encode())
    with urllib.request.urlopen(request, timeout=60) as reply:
        answer = json.load(reply)
    if "error" in answer:
        raise RuntimeError(f"{name}: {answer['error']}")
    return answer["value"]


class Screen:
    """The emulator, turned into a landscape tablet for the run and put back afterwards."""

    mark = ""  # STALE_MARK when a stale app was allowed: on every line that tells how the run went

    def __init__(self, serial):
        if not re.fullmatch(r"emulator-\d+", serial):
            raise SystemExit("only an emulator can run this")
        self.adb = shutil.which("adb") or str(Path.home() / "Library/Android/sdk/platform-tools/adb")
        self.serial = serial

    def run(self, *args, check=False, timeout=120):
        return subprocess.run([self.adb, "-s", self.serial, *args], capture_output=True, timeout=timeout, check=check)

    def shell(self, *args):
        return self.run("shell", *args).stdout.decode(errors="replace")

    def running(self):
        if self.shell("getprop", "sys.boot_completed").strip() != "1":
            raise SystemExit(f"{self.serial} is not running")

    def app(self, build=False, allow_stale=False, root=flow.ROOT):
        """Why the installed kiosk app is stale when a stale app was allowed, else "". See app_build_record.py.

        Runs before the screen is turned and before the app is touched: an old build passes or fails for reasons the
        current sources do not have. Only an emulator without the app gets the build that lies there, as before.
        """
        self.running()
        apk, inputs = root / APK, build_record.KIOSK_APP
        absent = bool(self.run("shell", "pm", "path", PACKAGE).returncode)
        if build:
            print("Building the debug app", flush=True)
            build_record.build(apk, root, inputs, ["./gradlew", ":app:assembleDebug"], root / "products/p04-merchant-kiosk/android")
        elif absent:
            if not apk.is_file():
                raise SystemExit("install the debug app first: run this again with --build")
            if not allow_stale:  # a build that the run would refuse is not installed either
                build_record.admit(build_record.stale(apk, root, inputs), False, REMEDY)
        if build or absent:
            installed = self.run("install", "-r", str(apk), timeout=600)
            if installed.returncode:
                raise SystemExit(f"the debug app did not install on {self.serial}\n{(installed.stdout + installed.stderr).decode(errors='replace')[-2000:]}")
        return build_record.admit(build_record.stale(apk, root, inputs, lambda: build_record.installed_digest(self.run, PACKAGE)), allow_stale, REMEDY)

    def __enter__(self):
        self.running()
        self.before = {name: self.shell("settings", "get", "system", name).strip() for name in ("accelerometer_rotation", "user_rotation")}
        self.shell("settings", "put", "system", "accelerometer_rotation", "0")
        self.shell("settings", "put", "system", "user_rotation", "1")
        self.shell("wm", "size", "800x1280")
        self.shell("wm", "density", "160")
        return self

    def __exit__(self, *_):
        self.shell("am", "force-stop", PACKAGE)
        self.shell("wm", "size", "reset")
        self.shell("wm", "density", "reset")
        for name, value in self.before.items():
            self.shell("settings", "put", "system", name, value if value != "null" else "0")

    def prepare(self):
        self.run("reverse", f"tcp:{HOST}", f"tcp:{HOST}")
        self.run("reverse", f"tcp:{BUNDLER}", f"tcp:{BUNDLER}")
        setting = f'<map><string name=\\"debug_http_host\\">localhost:{BUNDLER}</string></map>'
        self.shell("am", "force-stop", PACKAGE)
        if self.run("shell", f"run-as {PACKAGE} sh -c 'mkdir -p shared_prefs && echo \"{setting}\" > shared_prefs/{PACKAGE}_preferences.xml'").returncode:
            raise SystemExit("the installed kiosk app is not a debug build")

    def nodes(self):
        # A dump fails while the screen is busy; an old file must not be read as the new screen.
        for _ in range(10):
            self.shell("rm", "-f", "/sdcard/nu54-kiosk.xml")
            self.shell("uiautomator", "dump", "/sdcard/nu54-kiosk.xml")
            text = self.run("exec-out", "cat", "/sdcard/nu54-kiosk.xml").stdout.decode(errors="replace")
            if "<node " in text:
                break
            time.sleep(1)
        return [(re.search(r' content-desc="([^"]*)"', n)[1], re.search(r' text="([^"]*)"', n)[1], [int(x) for x in re.findall(r"\d+", re.search(r' bounds="([^"]*)"', n)[1])])
                for n in re.findall(r"<node [^>]*>", text)]

    def tap(self, label, seconds=90):
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            for desc, text, box in self.nodes():
                if label in (desc, text):
                    self.shell("input", "tap", str((box[0] + box[2]) // 2), str((box[1] + box[3]) // 2))
                    time.sleep(1.5)
                    return
            time.sleep(2)
        raise AssertionError(f"no {label} on the kiosk{self.mark}")

    def sees(self, word, seconds=60):
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            if any(word in desc or word in text for desc, text, _ in self.nodes()):
                return
            time.sleep(2)
        raise AssertionError(f"{word} not on the kiosk{self.mark}")

    def shot(self, directory, name):
        (directory / f"{name}.png").write_bytes(self.run("exec-out", "screencap", "-p").stdout)


@contextmanager
def world(args, screen, tools):
    """The store with its cafe menu and the bundler, and the kiosk app started against them. Gives the host's process and its log."""
    with integration_workspace("nu54-kiosk-", str(tools / "pg_ctl"), "data", flow.secure_env()) as (scratch, env):
        screen.prepare()
        shared = run_command(["go", "env", "GOCACHE"], env=flow.secure_env(), capture_output=True, text=True, check=True).stdout.strip()
        env = {**env, "GOCACHE": shared}
        log = scratch / "log"
        log.mkdir()
        print("Building and preparing the database", flush=True)
        run_command(["pnpm", "--filter", "@nu54/p05-backoffice-web", "build"], cwd=flow.ROOT, env=env, check=True, capture_output=True)
        dsn = flow.database(scratch, env, tools, log)
        built = flow.build(scratch, env, log)
        with cleanup_stack() as stack:
            def start(name, command, cwd, extra):
                output = (log / f"{name}.log").open("wb")
                stack.callback(output.close)
                process = OwnedProcess()
                stack.callback(process.stop)
                process.start(command, cwd=cwd, env={**env, **extra}, stdin=subprocess.DEVNULL, stdout=output, stderr=subprocess.STDOUT)
                return process
            bundler = start("bundler", ["npx", "react-native", "start", "--port", str(BUNDLER), "--config", str(SIMULATOR / "metro.kiosk.config.js")], KIOSK, {})
            host = start("host", [str(built["server"]), "-test.run", "^TestOrderFlow$", "-test.v", "-test.timeout", f"{MINUTES + 5}m"],
                         flow.ROOT / flow.P05 / "internal/settlement", {
                             "P05_SETTLEMENT_TEST_DATABASE_URL": dsn, "P05_SETTLEMENT_TEST_INDEXER": str(built["indexer"]),
                             "P05_SETTLEMENT_TEST_RPC": "http://127.0.0.1:1", "FOUNDRY_OUT": str(built["contracts"]),
                             "NU54_ORDER_FLOW_BOARD": str(built["board"]), "NU54_ORDER_FLOW_BOARD_WON": str(built["board_won"]), "NU54_ORDER_FLOW_BOARD_KRWC": str(built["board_krwc"]),
                             "NU54_ORDER_FLOW_TOKEN": args.token, "NU54_ORDER_FLOW_JEST": str(SIMULATOR / "kiosk.jest.config.cjs"),
                             "NU54_ORDER_FLOW_MINUTES": str(MINUTES), "NU54_ORDER_FLOW_OPS_WEB": str(WEB / "dist")})
            wait_for(bundler, log / "bundler.log", "Dev server ready", 60)
            wait_for(host, log / "host.log", "SIMULATOR-READY", 180)
            store = step("fixture")
            print(f"The store is running at {store['endpoint']}/ops/ (client certificate: the kitchen's)", flush=True)
            run_command(["node", str(SIMULATOR / "cafe-menu.cjs"), store["endpoint"], store["branch"], store["cert"], store["key"], str(args.evidence)],
                        env={**os.environ}, check=True)
            screen.shell("am", "start", "-n", f"{PACKAGE}/.MainActivity")
            print("The kiosk is starting; the first bundle takes a minute.", flush=True)
            yield host, log / "host.log"


def main(arguments=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("serial", help="the emulator, for example emulator-5554")
    parser.add_argument("evidence", type=Path, help="a new directory for the screenshots")
    parser.add_argument("--postgres-bin", type=Path, default=os.environ.get("POSTGRES_BIN"), help="directory with the PostgreSQL tools")
    parser.add_argument("--token", choices=("usd", "won", "krwc"), default="won", help="the store's token")
    parser.add_argument("--stay", action="store_true", help="keep the store running after the screenshots, until interrupted")
    parser.add_argument("--build", action="store_true", help="build the debug app from the current sources, record what it was built from and install it")
    parser.add_argument("--allow-stale-app", action="store_true", help="run although the installed app is not known to be built from the current sources; "
                        f"every result line then says STALE APP ALLOWED and a run without a failure ends with {STALE_EXIT} instead of 0")
    args = parser.parse_args(arguments)
    args.evidence = args.evidence.absolute()
    if args.evidence.exists():
        raise SystemExit(f"{args.evidence} is there already: the screenshots go into a new directory")
    tools = flow.postgres_tools(args.postgres_bin)
    screen = Screen(args.serial)
    stale = screen.app(args.build, args.allow_stale_app)
    mark = screen.mark = STALE_MARK if stale else ""
    args.evidence.mkdir(parents=True)  # only now, so that a run refused for a stale app can be repeated with the same directory
    if stale:
        (args.evidence / "STALE-APP.txt").write_text(stale + "\n")
    with screen, world(args, screen, tools) as (host, log):
        if args.stay:
            # The menu screen keeps changing while photos arrive, which uiautomator cannot read; it is driven by hand.
            print(f"STORE-READY: drive the kiosk with adb; interrupt to end.{mark}", flush=True)
            host.child.wait()
        else:
            try:
                screen.tap("새 고객 주문", 240)
                time.sleep(12)
                screen.shot(args.evidence, "kiosk-menu")
            finally:
                print("\n".join(line for line in log.read_text(errors="replace").splitlines() if "KIOSK" in line)[-3000:], flush=True)
            print(f"Screenshots in {args.evidence}{mark}", flush=True)
            step("stop")
    if stale:
        raise SystemExit(STALE_EXIT)


if __name__ == "__main__":
    main()

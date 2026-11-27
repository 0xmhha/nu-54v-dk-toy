#!/usr/bin/env python3
"""Run the user app on an Android emulator against a board, kiosk and store that answer as they should.

The app's screens, the firmware core, server, indexer and contracts are real; its native modules are stood in for.
Nothing here reaches a release: only the bundler configuration of this run replaces the modules.
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

SIMULATOR = flow.FLOW / "simulator"
APP = flow.ROOT / "products/p02-user-app"
PACKAGE = "com.nu54renter"
APK = "products/p02-user-app/android/app/build/outputs/apk/debug/app-debug.apk"
MANIFEST = "products/p02-user-app/android/app/src/main/AndroidManifest.xml"
REMEDY = "rebuild and install it with --build, or pass --allow-stale-app to run against the old build on purpose"
STALE_MARK = " (STALE APP ALLOWED: not a result for the current sources)"  # on every result line of a run let through
STALE_EXIT = 3  # how such a run ends when nothing failed, so that a script cannot take it for a pass; a failure ends with 1
HOST, BUNDLER = 8790, 8785  # not 8081, where the apps of other projects look for a bundler
MINUTES = 45


def step(name, **values):
    """One step of the owner, kiosk, store or operator; see the list in host.test.ts."""
    request = urllib.request.Request(f"http://127.0.0.1:{HOST}/step", json.dumps({**values, "step": name}).encode())
    with urllib.request.urlopen(request, timeout=90) as reply:
        answer = json.load(reply)
    if "error" in answer:
        raise RuntimeError(f"{name}: {answer['error']}")
    return answer["value"]


def emulator(serial, build=False, allow_stale=False, root=flow.ROOT):
    """The emulator's adb, and why its app is stale when a stale app was allowed. See app_build_record.py."""
    if not re.fullmatch(r"emulator-\d+", serial):
        raise SystemExit("only an emulator can run this: it turns screen protection and key storage off")
    adb = shutil.which("adb") or str(Path.home() / "Library/Android/sdk/platform-tools/adb")
    def run(*args, timeout=90):
        return subprocess.run([adb, "-s", serial, *args], capture_output=True, timeout=timeout)
    if run("shell", "getprop", "sys.boot_completed").stdout.strip() != b"1":
        raise SystemExit(f"{serial} is not running")
    apk = root / APK
    if build:
        print("Building the debug app", flush=True)
        build_record.build(apk, root, build_record.USER_APP, ["./gradlew", ":app:assembleDebug"], root / "products/p02-user-app/android")
        installed = run("install", "-r", str(apk), timeout=600)
        if installed.returncode:
            raise SystemExit(f"the debug app did not install on {serial}\n{(installed.stdout + installed.stderr).decode(errors='replace')[-2000:]}")
    if run("shell", "pm", "path", PACKAGE).returncode:
        raise SystemExit("install the debug app first: run this again with --build")
    # Before the app is touched: an old build passes or fails for reasons the current sources do not have.
    stale = build_record.admit(build_record.stale(apk, root, build_record.USER_APP, lambda: build_record.installed_digest(run, PACKAGE)), allow_stale, REMEDY)
    run("reverse", f"tcp:{HOST}", f"tcp:{HOST}")
    run("reverse", f"tcp:{BUNDLER}", f"tcp:{BUNDLER}")
    # The app's own development setting names the bundler.
    setting = f'<map><string name=\\"debug_http_host\\">localhost:{BUNDLER}</string></map>'
    run("shell", "am", "force-stop", PACKAGE)
    if run("shell", f"run-as {PACKAGE} sh -c 'mkdir -p shared_prefs && echo \"{setting}\" > shared_prefs/{PACKAGE}_preferences.xml'").returncode:
        raise SystemExit("the installed app is not a debug build")
    for permission in ("BLUETOOTH_CONNECT", "BLUETOOTH_SCAN", "POST_NOTIFICATIONS"):
        permission = "android.permission." + permission
        stale = build_record.admit(build_record.lacks(run("shell", "pm", "grant", PACKAGE, permission), root / MANIFEST, permission), allow_stale, REMEDY) or stale
    return run, stale


def wait_for(process, log, word, seconds):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        if word in log.read_text(errors="replace"):
            return
        if process.child.poll() is not None:
            break
        time.sleep(0.5)
    raise SystemExit(f"{word} never came\n{log.read_text(errors='replace')[-4000:]}")


@contextmanager
def world(args, adb):
    """The board, the kiosk and the store, the bundler, and the app started among them. Gives the host's process."""
    tools = flow.postgres_tools(args.postgres_bin)
    with integration_workspace("nu54-app-", str(tools / "pg_ctl"), "data", flow.secure_env()) as (scratch, env):
        shared = run_command(["go", "env", "GOCACHE"], env=flow.secure_env(), capture_output=True, text=True, check=True).stdout.strip()
        env = {**env, "GOCACHE": shared}
        log = scratch / "log"
        log.mkdir()
        print("Building and preparing the database", flush=True)
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
            bundler = start("bundler", ["npx", "react-native", "start", "--port", str(BUNDLER), "--config", str(SIMULATOR / "metro.config.js")], APP, {})
            host = start("host", [str(built["server"]), "-test.run", "^TestOrderFlow$", "-test.v", "-test.timeout", f"{MINUTES + 5}m"],
                         flow.ROOT / flow.P05 / "internal/settlement", {
                             "P05_SETTLEMENT_TEST_DATABASE_URL": dsn, "P05_SETTLEMENT_TEST_INDEXER": str(built["indexer"]),
                             "P05_SETTLEMENT_TEST_RPC": "http://127.0.0.1:1", "FOUNDRY_OUT": str(built["contracts"]),
                             "NU54_ORDER_FLOW_BOARD": str(built["board"]), "NU54_ORDER_FLOW_BOARD_WON": str(built["board_won"]), "NU54_ORDER_FLOW_BOARD_KRWC": str(built["board_krwc"]),
                             "NU54_ORDER_FLOW_TOKEN": args.token, "NU54_ORDER_FLOW_JEST": str(SIMULATOR / "jest.config.cjs"),
                             "NU54_ORDER_FLOW_MINUTES": str(MINUTES)})
            wait_for(bundler, log / "bundler.log", "Dev server ready", 60)
            wait_for(host, log / "host.log", "SIMULATOR-READY", 120)
            adb("shell", "am", "force-stop", PACKAGE)
            adb("shell", "am", "start", "-n", f"{PACKAGE}/.MainActivity")
            print(f"Ready for {MINUTES - 2} minutes. The app is starting on {args.serial}; the first bundle takes a minute.", flush=True)
            yield host


def main(arguments=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("serial", nargs="?", help="the emulator, for example emulator-5554")
    parser.add_argument("--walk", type=Path, metavar="DIRECTORY", help="walk through the flows and keep screenshots in this new directory")
    parser.add_argument("--step", nargs=2, metavar=("NAME", "JSON"), help="take one step in a running simulation, for example --step order '{\"quantity\":2}'")
    parser.add_argument("--postgres-bin", type=Path, default=os.environ.get("POSTGRES_BIN"), help="directory with the PostgreSQL tools")
    parser.add_argument("--token", choices=("usd", "won", "krwc"), default="usd", help="the store's token")
    parser.add_argument("--build", action="store_true", help="build the debug app from the current sources, record what it was built from and install it")
    parser.add_argument("--allow-stale-app", action="store_true", help="run although the installed app is not known to be built from the current sources; "
                        f"every result line then says STALE APP ALLOWED and a run without a failure ends with {STALE_EXIT} instead of 0")
    args = parser.parse_args(arguments)
    if args.step:
        print(json.dumps(step(args.step[0], **json.loads(args.step[1])), ensure_ascii=False, indent=1))
        return
    if not args.serial:
        parser.error("name the emulator")
    if args.walk:
        args.walk = args.walk.absolute()
        if args.walk.exists():
            raise SystemExit(f"{args.walk} is there already: --walk takes a new directory")
    adb, stale = emulator(args.serial, args.build, args.allow_stale_app)
    mark = STALE_MARK if stale else ""
    if args.walk:
        args.walk.mkdir(parents=True)  # only now, so that a run refused for a stale app can be repeated with the same directory
        if stale:
            (args.walk / "STALE-APP.txt").write_text(stale + "\n")
    with world(args, adb) as host:
        if args.walk:
            from simulate_user_app_walk import Walk
            if failed := Walk(adb, step, args.walk, args.token, mark).run():
                raise SystemExit(f"FAIL user app{mark}: {failed} flows failed; see {args.walk}")
            print(f"PASS user app{mark}", flush=True)
        else:
            print("Take steps from another terminal with --step. Interrupt to end.", flush=True)
            host.child.wait()
    if stale:
        raise SystemExit(STALE_EXIT)


if __name__ == "__main__":
    main()

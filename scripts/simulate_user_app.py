#!/usr/bin/env python3
"""Run the user app on an Android emulator against a board, kiosk and store that answer as they should.

The app's screens, the firmware core, server, indexer and contracts are real; its native modules are stood in for.
Nothing here reaches a release: only the bundler configuration of this run replaces the modules.
See integration-workspace.md.
"""

import argparse
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

from integration_workspace import OwnedProcess, cleanup_stack, integration_workspace, run_command
import test_order_flow as flow

SIMULATOR = flow.FLOW / "simulator"
APP = flow.ROOT / "products/p02-user-app"
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


def emulator(serial):
    if not re.fullmatch(r"emulator-\d+", serial):
        raise SystemExit("only an emulator can run this: it turns screen protection and key storage off")
    adb = shutil.which("adb") or str(Path.home() / "Library/Android/sdk/platform-tools/adb")
    def run(*args):
        return subprocess.run([adb, "-s", serial, *args], capture_output=True, timeout=90)
    if run("shell", "getprop", "sys.boot_completed").stdout.strip() != b"1":
        raise SystemExit(f"{serial} is not running")
    if run("shell", "pm", "path", "com.nu54renter").returncode:
        raise SystemExit("install the debug app first: cd products/p02-user-app/android && ./gradlew :app:installDebug")
    run("reverse", f"tcp:{HOST}", f"tcp:{HOST}")
    run("reverse", f"tcp:{BUNDLER}", f"tcp:{BUNDLER}")
    # The app's own development setting names the bundler.
    setting = f'<map><string name=\\"debug_http_host\\">localhost:{BUNDLER}</string></map>'
    run("shell", "am", "force-stop", "com.nu54renter")
    if run("shell", f"run-as com.nu54renter sh -c 'mkdir -p shared_prefs && echo \"{setting}\" > shared_prefs/com.nu54renter_preferences.xml'").returncode:
        raise SystemExit("the installed app is not a debug build")
    for permission in ("BLUETOOTH_CONNECT", "BLUETOOTH_SCAN", "POST_NOTIFICATIONS"):
        run("shell", "pm", "grant", "com.nu54renter", "android.permission." + permission)
    return run


def wait_for(process, log, word, seconds):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        if word in log.read_text(errors="replace"):
            return
        if process.child.poll() is not None:
            break
        time.sleep(0.5)
    raise SystemExit(f"{word} never came\n{log.read_text(errors='replace')[-4000:]}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("serial", nargs="?", help="the emulator, for example emulator-5554")
    parser.add_argument("--walk", type=Path, metavar="DIRECTORY", help="walk through the flows and keep screenshots in this new directory")
    parser.add_argument("--step", nargs=2, metavar=("NAME", "JSON"), help="take one step in a running simulation, for example --step order '{\"quantity\":2}'")
    parser.add_argument("--postgres-bin", type=Path, default=os.environ.get("POSTGRES_BIN"), help="directory with the PostgreSQL tools")
    parser.add_argument("--token", choices=("usd", "won", "krwc"), default="usd", help="the store's token")
    args = parser.parse_args()
    if args.step:
        print(json.dumps(step(args.step[0], **json.loads(args.step[1])), ensure_ascii=False, indent=1))
        return
    if not args.serial:
        parser.error("name the emulator")
    if args.walk:
        args.walk = args.walk.absolute()
        args.walk.mkdir(parents=True)
    adb = emulator(args.serial)
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
            adb("shell", "am", "force-stop", "com.nu54renter")
            adb("shell", "am", "start", "-n", "com.nu54renter/.MainActivity")
            print(f"Ready for {MINUTES - 2} minutes. The app is starting on {args.serial}; the first bundle takes a minute.", flush=True)
            if args.walk:
                from simulate_user_app_walk import Walk
                if failed := Walk(adb, step, args.walk, args.token).run():
                    raise SystemExit(f"FAIL user app: {failed} flows failed; see {args.walk}")
                print("PASS user app", flush=True)
            else:
                print("Take steps from another terminal with --step. Interrupt to end.", flush=True)
                host.child.wait()


if __name__ == "__main__":
    main()

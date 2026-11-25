#!/usr/bin/env python3
"""Run the whole order scenario across the product code: board firmware core, phone, kiosk, server, indexer and contracts.

Needs no board, phone or real chain. PostgreSQL, Anvil and every process belong to this run and end with it.
See integration-workspace.md for what is real and what is stood in for.
"""

import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time

if __name__ == "__main__":
    sys.dont_write_bytecode = True

from integration_workspace import OwnedProcess, cleanup_stack, integration_workspace, run_command, start_postgres

ROOT = Path(__file__).resolve().parents[1]
P05 = "products/p05-operations-backoffice"
FLOW = ROOT / P05 / "test/order-flow"
USER = "nu54_order_flow"
DATABASE = "nu54_settlement_order_flow"
FLOWS = 17
VECTORS = ROOT / "packages/protocol/v2/testdata"
WON_BOARD = ("-DNU54_TOKEN_NAME=NU54 Test KRW", f"-DNU54_ORDER_VECTOR={VECTORS / 'order-lock-won.vectors'}",
             f"-DNU54_RETURN_VECTOR={VECTORS / 'wallet-return-won.vectors'}", "-DNU54_RETURN_AMOUNT=3420000")
LIMIT = 600


def secure_env():
    env = {k: v for k, v in os.environ.items() if not k.startswith(("PG", "P05_", "P07_", "NU54_"))
           and k not in {"ANVIL", "FORGE", "CAST", "FOUNDRY_OUT", "FOUNDRY_CACHE_PATH"}}
    # initdb and the server refuse an unset or multi-byte locale on macOS.
    return {**env, "LC_ALL": "C", "FOUNDRY_DYNAMIC_TEST_LINKING": "false"}


def postgres_tools(directory):
    """The directory of working PostgreSQL tools: the argument, else a Homebrew keg, else PATH."""
    on_path = shutil.which("initdb")
    candidates = [directory] if directory else [Path(keg) / "opt/postgresql@15/bin" for keg in ("/opt/homebrew", "/usr/local")]
    if not directory and on_path:
        candidates.append(Path(on_path).parent)
    for candidate in map(Path, candidates):
        try:
            subprocess.run([candidate / "initdb", "--version"], check=True, capture_output=True)
            return candidate.resolve()
        except (OSError, subprocess.CalledProcessError):
            continue
    raise SystemExit("PostgreSQL tools not found; pass --postgres-bin")


def openssl_root():
    brew = shutil.which("brew")
    if os.environ.get("OPENSSL_ROOT_DIR") or sys.platform != "darwin" or not brew:
        return {}
    found = subprocess.run([brew, "--prefix", "openssl@3"], capture_output=True, text=True)
    return {"OPENSSL_ROOT_DIR": found.stdout.strip()} if found.returncode == 0 else {}


def build(scratch, env, log):
    """Builds side by side. A failed step shows its output and ends the others."""
    out = {"indexer": scratch / "indexer", "board": scratch / "board/order-board", "board_won": scratch / "board-won/order-board", "contracts": scratch / "contracts",
           "server": scratch / "settlement.test"}
    target = ROOT / P05 / "internal/settlement/order_flow_anvil_test.go"
    if target.exists():
        raise SystemExit(f"{target} exists, but the fixture belongs in {FLOW / 'testdata'}")
    overlay = scratch / "overlay.json"
    overlay.write_text(json.dumps({"Replace": {str(target): str(FLOW / "testdata/order_flow_anvil_test.go")}}))
    foundry = {**env, "FOUNDRY_OUT": str(out["contracts"]), "FOUNDRY_CACHE_PATH": str(scratch / "forge-cache")}
    cmake = {**env, **openssl_root()}
    steps = {
        "contracts": [(["python3", "-B", "scripts/contract_forge.py", "p06", "build", "--offline", "--quiet"], foundry)],
        "board firmware": [(["cmake", "-S", FLOW / "board", "-B", scratch / "board", "-DCMAKE_BUILD_TYPE=Debug"], cmake),
                           (["cmake", "--build", scratch / "board"], cmake),
                           (["ctest", "--test-dir", scratch / "board", "--output-on-failure"], cmake)],
        "board firmware for won": [(["cmake", "-S", FLOW / "board", "-B", scratch / "board-won", "-DCMAKE_BUILD_TYPE=Debug", *WON_BOARD], cmake),
                                   (["cmake", "--build", scratch / "board-won"], cmake),
                                   (["ctest", "--test-dir", scratch / "board-won", "--output-on-failure"], cmake)],
        "server and indexer": [(["go", "test", "-c", "-overlay", overlay, "-o", out["server"], f"./{P05}/internal/settlement"], env),
                               (["go", "build", "-o", out["indexer"], "./products/p07-indexer/cmd/indexer"], env)],
    }
    started, running = time.monotonic(), {}
    with cleanup_stack() as stack:
        for name in steps:
            process = OwnedProcess()
            stack.callback(process.stop)
            running[name] = (process, (log / f"{name.replace(' ', '-')}.log").open("ab"))
            stack.callback(running[name][1].close)
        def advance(name):
            process, output = running[name]
            command, step_env = steps[name].pop(0)
            process.start([str(part) for part in command], cwd=ROOT, env=step_env, stdout=output, stderr=subprocess.STDOUT)
        for name in steps:
            advance(name)
        while running:
            time.sleep(0.2)
            for name in list(running):
                process, output = running[name]
                if (status := process.child.poll()) is None:
                    continue
                if status:
                    print(Path(output.name).read_text(errors="replace")[-4000:], file=sys.stderr)
                    raise SystemExit(f"FAIL building {name}")
                if steps[name]:
                    advance(name)
                else:
                    del running[name]
                    print(f"   built {name} ({time.monotonic() - started:.0f}s)", flush=True)
    return out


def database(scratch, env, tools, log):
    data, socket = scratch / "data", scratch / "socket"
    socket.mkdir(mode=0o700)
    def step(args, step_env):
        done = run_command([str(arg) for arg in args], cwd=ROOT, env=step_env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        if done.returncode:
            raise SystemExit(f"FAIL {args[0]}\n{done.stdout[-4000:]}")
    step([tools / "initdb", "-D", data, "-A", "trust", "--no-locale", "--encoding=UTF8", "-U", USER], env)
    with (log / "postgres-start.log").open("wb") as output:
        start_postgres(str(tools / "pg_ctl"), data, log / "postgres.log",
                       f"-c listen_addresses='' -c unix_socket_directories={socket} -c unix_socket_permissions=0700 -c fsync=off",
                       env, cwd=ROOT, stdout=output, stderr=output)
    local = {**env, "PGHOST": str(socket), "PGUSER": USER, "PGPORT": "5432", "PGSSLMODE": "disable",
             "PATH": str(tools) + os.pathsep + env["PATH"]}
    step([tools / "createdb", DATABASE], local)
    for owner in ("p05", "p07"):
        step(["python3", "-B", "products/p10-platform/database/migrate.py", owner],
             {**local, f"{owner.upper()}_MIGRATION_DATABASE_URL": DATABASE})
    return f"host={socket} port=5432 user={USER} dbname={DATABASE} sslmode=disable"


def run_flows(scratch, env, built, dsn, log, verbose, only):
    """Prints the flow lines as they come and returns them with everything else the run said."""
    env = {**env, "P05_SETTLEMENT_TEST_DATABASE_URL": dsn, "P05_SETTLEMENT_TEST_INDEXER": str(built["indexer"]),
           "P05_SETTLEMENT_TEST_RPC": "http://127.0.0.1:1", "FOUNDRY_OUT": str(built["contracts"]),
           "NU54_ORDER_FLOW_BOARD": str(built["board"]), "NU54_ORDER_FLOW_JEST": str(FLOW / "app/jest.config.cjs")}
    if only:
        env["NU54_ORDER_FLOW_ONLY"] = only
    lines, rest = [], []
    with cleanup_stack() as stack:
        process = OwnedProcess()
        stack.callback(process.stop)
        child = process.start([str(built["server"]), "-test.run", "^TestOrderFlow$", "-test.v", "-test.timeout", f"{LIMIT}s"],
                              cwd=ROOT / P05 / "internal/settlement", env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                              text=True, errors="replace", bufsize=1)
        for line in child.stdout:
            rest.append(line)
            at = line.find("ORDER-FLOW ")
            if at >= 0:
                word = line[at:].strip().removeprefix("ORDER-FLOW ")
                if not word.startswith("DONE"):
                    lines.append(word)
                print(word, flush=True)
            elif verbose:
                print("    " + line, end="", flush=True)
        status = child.wait()
        child.stdout.close()
    (log / "flows.log").write_text("".join(rest))
    return status, lines, rest


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--postgres-bin", type=Path, default=os.environ.get("POSTGRES_BIN"), help="directory with the PostgreSQL tools")
    parser.add_argument("--verbose", action="store_true", help="show the output of the server, the indexer and the apps as it comes")
    parser.add_argument("--only", metavar="PATTERN", help="run the flows whose names match, for example 'enroll|deposit|duplicate'")
    args = parser.parse_args()
    tools, started = postgres_tools(args.postgres_bin), time.monotonic()
    with integration_workspace("nu54-flow-", str(tools / "pg_ctl"), "data", secure_env()) as (scratch, env):
        # Only Go's own build cache is shared with other work; it decides the build time.
        shared = run_command(["go", "env", "GOCACHE"], env=secure_env(), capture_output=True, text=True, check=True).stdout.strip()
        env = {**env, "GOCACHE": shared}
        log = scratch / "log"
        log.mkdir()
        print("Building and preparing the database", flush=True)
        dsn = database(scratch, env, tools, log)
        built = build(scratch, env, log)
        status, lines, rest = run_flows(scratch, env, built, dsn, log, args.verbose, args.only)
    passed = sum(line.startswith("PASS ") for line in lines)
    wanted = len(lines) if args.only and lines else FLOWS
    if status or passed != wanted or len(lines) != wanted:
        if not args.verbose:
            print("".join(rest)[-12000:], file=sys.stderr)
        raise SystemExit(f"FAIL order flow: {passed} of {wanted} flows passed ({time.monotonic() - started:.0f}s)")
    print(f"PASS order flow: {passed} of {wanted} flows ({time.monotonic() - started:.0f}s)", flush=True)


if __name__ == "__main__":
    main()

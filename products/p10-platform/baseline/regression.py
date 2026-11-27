#!/usr/bin/env python3
"""Rerun the v1 synthetic flow, refusal vectors and isolated indexer restart."""

import argparse
import json
import os
import shlex
import subprocess
import tempfile
from pathlib import Path

from verify import HERE, ROOT, load, require

RESTART_TEST = "TestPostgresIdempotentRestartAndDuplicate"
INDEXER = "products/p07-indexer"


def run(name, directory, args, *, env=None):
    print(f"=== {name} ===\n  {shlex.join(args)}", flush=True)
    subprocess.run(args, cwd=ROOT / directory, check=True, env=env)
    print(f"PASS  {name}", flush=True)


def flow():
    toolchain = load(HERE / "v1.json")["toolchain"]["sourceBuildInputs"]
    foundry = toolchain["foundry"]
    version = subprocess.check_output(["forge", "--version"], text=True).splitlines()[0]
    tag = version.split(maxsplit=2)[-1].split("-", 1)[0]
    require(tag == foundry, f"v1 requires Forge {foundry}; observed {version}")
    run("renter setup and phone confirmation", "products/p02-user-app", [
        "pnpm", "exec", "jest", "--runInBand", "--runTestsByPath", "test/setup.test.ts", "test/confirm.test.ts",
    ])
    run("kiosk session, approval and finalized event matching", "products/p04-merchant-kiosk", [
        "pnpm", "exec", "jest", "--runInBand", "--runTestsByPath",
        "test/session.test.ts", "test/secure.test.ts", "test/pay.test.ts", "test/submit.test.ts",
    ])
    run("operations provisioning, return order and refusal frames", "products/p05-operations-backoffice", [
        "go", "test", "-count=1", "-v", "./internal/core/ops", "-run", "^Test(Provision|Return|Refuse)",
    ])
    run("local Solidity ledger and refusal cases", "products/p06-stablenet-contracts", [
        "forge", "test", "--offline", "--chain-id", "8283", "-v",
    ], env={**os.environ, "FOUNDRY_SOLC": toolchain["solc"]})
    with tempfile.TemporaryDirectory(prefix="nu54-v1-host-") as build:
        firmware = "products/p01-device-firmware"
        run("firmware host configure", firmware, ["cmake", "-S", "test", "-B", build, "-DCMAKE_BUILD_TYPE=Debug"])
        run("firmware host build", firmware, ["cmake", "--build", build, "--target", "test_session", "test_links"])
        run("firmware host normal and refusal session vectors", firmware, [
            "ctest", "--test-dir", build, "-R", "^(session_vectors|links)$", "--output-on-failure", "--no-tests=error",
        ])


def restart_passed(stdout):
    events = [json.loads(line) for line in stdout.splitlines()]
    actions = [event["Action"] for event in events if event.get("Test") == RESTART_TEST]
    return actions.count("run") == 1 and actions.count("pass") == 1 and not {"skip", "fail"}.intersection(actions)


def restart(postgres_bin):
    # Never pass a caller-supplied DB URL to the test: it truncates tables.
    def postgres(tool):
        return str(postgres_bin.resolve() / tool) if postgres_bin else tool

    with tempfile.TemporaryDirectory(prefix="nu54-v1-pg-", dir="/tmp") as directory:
        root = Path(directory)
        data = root / "data"
        socket = root / "socket"
        socket.mkdir(mode=0o700)
        control = [postgres("pg_ctl"), "-D", str(data), "-w"]
        run("initialize scratch PostgreSQL", ".", [
            postgres("initdb"), "-D", str(data), "-A", "trust", "--no-locale", "-U", "nu54_baseline",
        ])
        try:
            run("start scratch PostgreSQL on a private Unix socket", ".", [
                *control, "-l", str(root / "postgres.log"), "-o",
                f"-c listen_addresses='' -c unix_socket_directories={socket} -c unix_socket_permissions=0700", "start",
            ])
            print(f"=== {RESTART_TEST} ===", flush=True)
            proc = subprocess.run(
                ["go", "test", "-json", "-count=1", "./internal/ingest", "-run", f"^{RESTART_TEST}$"],
                cwd=ROOT / INDEXER,
                env={**os.environ, "P07_TEST_DATABASE_URL": f"host={socket} user=nu54_baseline dbname=postgres sslmode=disable"},
                capture_output=True, text=True,
            )
            print(proc.stdout + proc.stderr, end="", flush=True)
            if proc.returncode or not restart_passed(proc.stdout):
                raise ValueError("named indexer restart test did not run and pass")
            print("PASS  indexer restart and duplicate receipt regression", flush=True)
        finally:
            if (data / "postmaster.pid").exists():
                run("stop scratch PostgreSQL", ".", [*control, "-m", "fast", "stop"])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("group", choices=["all", "flow", "restart"])
    parser.add_argument("--postgres-bin", type=Path, help="directory containing initdb and pg_ctl")
    args = parser.parse_args()
    try:
        if args.group in {"all", "flow"}:
            flow()
        if args.group in {"all", "restart"}:
            restart(args.postgres_bin)
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        parser.exit(1, f"FAIL  {error}\n")


if __name__ == "__main__":
    main()

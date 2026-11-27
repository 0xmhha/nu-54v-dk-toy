#!/usr/bin/env python3
"""Compile the pinned v1 contract and test P07 against local Anvil and PostgreSQL."""

import argparse
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import sys
import tempfile
import time

ROOT = Path(__file__).resolve().parents[3]
sys.dont_write_bytecode = True
sys.path.insert(0, str(ROOT / "scripts"))
from integration_workspace import OwnedProcess, cleanup_stack, integration_workspace, run_command, start_postgres


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def wait_rpc(daemon, port):
    for _ in range(80):
        if daemon.poll() is not None:
            break
        try:
            with socket.create_connection(("127.0.0.1", port), timeout=0.2):
                return
        except OSError:
            time.sleep(0.1)
    raise RuntimeError("isolated Anvil listener did not become ready")



def prepare_w12_credential_fixture(work, env):
    """Compile only the P01 host and a disposable opsd overlay."""
    source = ROOT / "products/p01-device-firmware/test"
    host = work / "w12-board"
    run_command(["cmake", "-S", source, "-B", host], cwd=ROOT, env=env, check=True)
    run_command(["cmake", "--build", host, "--target", "test_session", "--parallel", "2"],
                cwd=ROOT, env=env, check=True)
    p05 = ROOT / "products/p05-operations-backoffice"
    original = p05 / "cmd/opsd/commerce.go"
    decorated = original.read_text()
    authority = '\tif dsn := os.Getenv("P05_RENTAL_AUTHORITY_READ_DATABASE_URL"); dsn != "" {'
    register = '\t\treader.Register(mux)\n\t}'
    handler = '\tcommerce.HTTP{QuoteAdmission:'
    end = 'Signer: signer, Cipher: cipher}.Register(mux)'
    fixture_code = (
        'Signer: signer, Cipher: cipher}\n'
        '\tobserveW12, err := fixtureHTTP.ConfigureW12Fixture(ctx, fixtureAuthority, os.Getenv("P05_W12_TEST_FIXTURE"))\n'
        '\tif err != nil { return err }\n\tfixtureHTTP.Register(mux)'
    )
    inserts = (
        (authority, '\tvar fixtureAuthority *commerce.RentalAuthorityReader\n' + authority),
        (register, '\t\treader.Register(mux)\n\t\tfixtureAuthority = reader\n\t}'),
        (handler, '\tfixtureHTTP := commerce.HTTP{QuoteAdmission:'),
        (end, fixture_code),
        ("Handler: gate.Handler(mux)", "Handler: gate.Handler(observeW12(mux))"),
    )
    for before, after in inserts:
        if decorated.count(before) != 1:
            raise RuntimeError("W12 fixture overlay no longer matches opsd")
        decorated = decorated.replace(before, after)
    command = work / "w12-opsd-commerce.go"
    command.write_text(decorated)
    hook = ROOT / "scripts/ios/scene-smoke/fixtures/w12/commerce.go.fixture"
    hookfile = work / "w12-hook.go"
    hookfile.write_bytes(hook.read_bytes())
    overlay = work / "w12-overlay.json"
    overlay.write_text(json.dumps({"Replace": {str(original): str(command),
        str(p05 / "internal/commerce/w12_fixture_hook.go"): str(hookfile)}}))
    return overlay, host / "test_session"

def run_generation_anvil(work, env, executable, json_events=False, combined_p05=False, protected_close=False, supervised_close=False, protected_withdrawal="", rollover_close=""):
    env = {k: v for k, v in env.items() if k not in ("P05_O09_PROTECTED_CLOSE_EXECUTOR", "P05_O09_SUPERVISED_CLOSE", "P05_O09_PROTECTED_WITHDRAWAL", "P05_O09_ROLLOVER_CLOSE")}
    binary = work / "indexer"
    run_command(["go", "build", "-o", binary, "./products/p07-indexer/cmd/indexer"],
                cwd=ROOT, env=env, check=True)
    port = free_port()
    owned = {**env, "P07_GENERATION_TEST_RPC": f"http://127.0.0.1:{port}",
             "P07_GENERATION_TEST_BINARY": str(binary)}
    if combined_p05:
        p05 = work / "opsd"
        build = ["go", "build", "-o", p05]
        if env.get("P05_O09_W12_CREDENTIAL") == "1":
            overlay, board = prepare_w12_credential_fixture(work, env)
            build += ["-overlay", overlay]
            owned["P05_O09_W12_CREDENTIAL_BOARD"] = str(board)
        run_command([*build, "./products/p05-operations-backoffice/cmd/opsd"], cwd=ROOT, env=env, check=True)
        owned["P05_O09_COMBINED_OPSD"] = str(p05)
        if rollover_close:
            owned["P05_O09_ROLLOVER_CLOSE"] = rollover_close
        if supervised_close:
            owned["P05_O09_SUPERVISED_CLOSE"] = "1"
        if protected_withdrawal:
            owned["P05_O09_PROTECTED_WITHDRAWAL"] = protected_withdrawal
        if protected_close:
            executor = work / "p05-close-executor.test"
            run_command(["go", "test", "-c", "-o", executor, "./products/p05-operations-backoffice/internal/commerce"],
                        cwd=ROOT, env=env, check=True)
            owned["P05_O09_PROTECTED_CLOSE_EXECUTOR"] = str(executor)
    else:
        owned.pop("P05_O09_COMBINED_OPSD", None)
    with (work / "anvil.log").open("w") as log, cleanup_stack() as stack:
        process = OwnedProcess()
        stack.callback(process.stop)
        daemon = process.start(
            [executable, "--host", "127.0.0.1", "--port", str(port), "--chain-id", "8283",
             "--slots-in-an-epoch", "1", "--hardfork", "prague", "--silent"],
            env=owned, stdout=log, stderr=subprocess.STDOUT,
        )
        wait_rpc(daemon, port)
        args = ["go", "test", "-p", "2", "-count=1", "-json" if json_events else "-v",
                "./products/p07-indexer/internal/chainview", "-run",
                "^Test(SharedRJP1Projection|SharedRFR1Receipt|V1GenerationSameLimitsDistinctTypedNonces|GenerationReaderRealV1PostgresMutualTLS)$"]
        run_command(args, cwd=ROOT, env=owned, check=True)


def terminate(signum, _frame):
    raise SystemExit(128 + signum)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--pg-bin", type=Path)
    parser.add_argument("--solc", type=Path)
    parser.add_argument("--existing-postgres", action="store_true",
                        help="use the commercial gate’s disposable P07 database and scoped caches")
    parser.add_argument("--combined-p05", action="store_true",
                        help="replace the fixture authority with actual opsd and its separate PostgreSQL database")
    parser.add_argument("--protected-close-p05", action="store_true",
                        help="exercise the staged 075 executor with both production daemons")
    parser.add_argument("--supervised-close-p05", action="store_true",
                        help="verify the opt-in protected close through the real P05 operations API")
    parser.add_argument("--protected-withdrawal-p05", action="store_true",
                        help="verify the guarded withdrawal through the real P05 operations API")
    parser.add_argument("--zero-withdrawal-p05", action="store_true",
                        help="verify a genuine zero-payout close through the guarded withdrawal status")
    parser.add_argument("--recover-withdrawal-p05", action="store_true",
                        help="restart opsd during the first guarded withdrawal transmission")
    parser.add_argument("--rollover-close-p05", choices=("unchanged", "renewed"),
                        help="admit the original under 077, migrate to 078 and restart opsd")
    parser.add_argument("--json", action="store_true", help="emit Go test events for the strict gate")
    parser.add_argument("--anvil", default="anvil")
    parser.add_argument("--forge", default="forge")
    args = parser.parse_args()
    if sum((args.protected_close_p05, args.supervised_close_p05, args.protected_withdrawal_p05, args.zero_withdrawal_p05, args.recover_withdrawal_p05, bool(args.rollover_close_p05))) > 1:
        parser.error("choose one protected return fixture mode")
    withdrawal_mode = ("positive" if args.protected_withdrawal_p05 else "zero" if args.zero_withdrawal_p05
                       else "recover" if args.recover_withdrawal_p05 else "")
    args.supervised_close_p05 = args.supervised_close_p05 or bool(withdrawal_mode) or bool(args.rollover_close_p05)
    args.combined_p05 = args.combined_p05 or args.protected_close_p05 or args.supervised_close_p05
    if os.environ.get("P05_O09_W12_CREDENTIAL") == "1" and (
        not args.combined_p05 or os.environ.get("P05_O09_RENTER_V1_ORIGIN") != "1"
        or os.environ.get("P05_O09_RENTER_ASSESSMENT") != "mixed"
    ):
        parser.error("W12 credential fixture requires --combined-p05 and mixed v1 order origins")
    signal.signal(signal.SIGTERM, terminate)
    if args.existing_postgres:
        if args.rollover_close_p05:
            parser.error("rollover requires a fresh disposable database")
        for key in ("P07_GENERATION_TEST_DATABASE_URL", "P07_GENERATION_TEST_ARTIFACTS", "TMPDIR", "GOCACHE"):
            if not os.environ.get(key):
                parser.error(f"--existing-postgres requires {key}")
        if args.combined_p05 and not os.environ.get("P05_O09_COMBINED_DATABASE_URL"):
            parser.error("--combined-p05 with --existing-postgres requires a disposable migrated P05 database")
        with tempfile.TemporaryDirectory(prefix="p07-rental-") as directory:
            run_generation_anvil(Path(directory), os.environ, args.anvil, args.json, args.combined_p05, args.protected_close_p05, args.supervised_close_p05, withdrawal_mode, args.rollover_close_p05)
        return
    if not args.pg_bin or not args.solc:
        parser.error("--pg-bin and --solc are required for a standalone run")
    pg = args.pg_bin.resolve()
    base = {k: v for k, v in os.environ.items() if not k.startswith(("PG", "P07_", "FOUNDRY_"))}
    base.update(PATH=f"{pg}{os.pathsep}{base['PATH']}", GOMAXPROCS="2", RAYON_NUM_THREADS="2")

    with integration_workspace("nu54-o09-p07-", pg / "pg_ctl", "data", base) as (work, env):
        pg_port = free_port()
        database = "nu54_generation_o09"
        url = f"host={work} port={pg_port} dbname={database} sslmode=disable"
        env.update(
            FORGE=args.forge,
            FOUNDRY_SOLC=str(args.solc.resolve()),
            FOUNDRY_OUT=str(work / "forge"),
            FOUNDRY_CACHE_PATH=str(work / "forge-cache"),
            P07_MIGRATION_DATABASE_URL=url,
            P07_GENERATION_TEST_DATABASE_URL=url,
            P07_GENERATION_TEST_ARTIFACTS=str(work / "forge"),
        )

        def run(*cmd, **options):
            run_command([str(part) for part in cmd], cwd=ROOT, env=env, check=True, **options)

        with (work / "setup.log").open("w+") as log:
            options = {"stdout": log, "stderr": subprocess.STDOUT}
            try:
                run(sys.executable, "-B", "scripts/contract_forge.py", "p06", "build", "--offline",
                    "--skip", "test", "--skip", "script", **options)
                run(pg / "initdb", "--no-locale", "--encoding=UTF8", "-A", "trust", "-D", work / "data", **options)
                start_postgres(pg / "pg_ctl", work / "data", work / "postgres.log",
                               f"-k {work} -p {pg_port} -c listen_addresses=''", env, cwd=ROOT, **options)
                run(pg / "createdb", "-h", work, "-p", pg_port, database, **options)
                run(sys.executable, "-B", "products/p10-platform/database/migrate.py", "p07", **options)
                if args.combined_p05:
                    p05_database = "nu54_commerce_generation_combined"
                    p05_url = f"host={work} port={pg_port} dbname={p05_database} sslmode=disable"
                    run(pg / "createdb", "-h", work, "-p", pg_port, p05_database, **options)
                    env.update(P05_MIGRATION_DATABASE_URL=p05_url, P05_O09_COMBINED_DATABASE_URL=p05_url)
                    checkpoint = 0
                    if args.rollover_close_p05:
                        checkpoint = 77
                    elif env.get("P05_O09_RECEIPT_ATTRIBUTION_UPGRADE") == "1":
                        checkpoint = 81
                    elif env.get("P05_O09_RENTER_ATTESTATION_UPGRADE") == "1":
                        checkpoint = 83
                    elif env.get("P05_O09_RENTER_TYPED_UPGRADE") == "1":
                        checkpoint = 85
                    elif env.get("P05_O09_W12_SCHEMA_086") == "1":
                        checkpoint = 86
                    if checkpoint:
                        run(sys.executable, "-B", "-c", "import os,sys; sys.path.insert(0,'products/p10-platform/database'); "
                            "import migrate; migrate.apply('p05', os.environ['P05_MIGRATION_DATABASE_URL'], "
                            "migrate.migrations(migrate.OWNERS['p05'][1])[:int(sys.argv[1])])", checkpoint, **options)
                    else:
                        run(sys.executable, "-B", "products/p10-platform/database/migrate.py", "p05", **options)
            except subprocess.CalledProcessError:
                log.seek(0)
                print("".join(log.readlines()[-20:]), file=sys.stderr)
                raise

        run_generation_anvil(work, env, args.anvil, args.json, args.combined_p05, args.protected_close_p05, args.supervised_close_p05, withdrawal_mode, args.rollover_close_p05)
    if work.exists():
        raise RuntimeError("P07 integration workspace still exists")
    print("P07 Anvil, PostgreSQL, compiler artifacts and Go scratch removed")


if __name__ == "__main__":
    main()

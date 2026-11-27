#!/usr/bin/env python3
"""Verify the W11 archived owner read on a real pre-096 finalized order."""
import argparse
import os
from pathlib import Path
import subprocess
import sys

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parents[4]
sys.path.insert(0, str(ROOT / "scripts"))

from integration_workspace import integration_workspace, run_command, start_postgres
from test_commercial_integrations import secure_env, verified_test


def run_upgrade(pg, prior):
    with integration_workspace("nu54-w11-upgrade-", pg / "pg_ctl", "data", secure_env()) as (work, env):
        database = "nu54_settlement_w11_upgrade"
        user = "nu54_w11_upgrade"
        env |= {"PATH": f"{pg}{os.pathsep}{env['PATH']}", "PGHOST": str(work),
                "PGPORT": "5432", "PGUSER": user, "PGSSLMODE": "disable"}

        def run(command, extra=None):
            result = run_command(command, cwd=ROOT, env=env | (extra or {}), stdout=subprocess.PIPE,
                                 stderr=subprocess.STDOUT, text=True)
            if result.returncode:
                raise RuntimeError(result.stdout)

        run([pg / "initdb", "-D", work / "data", "-U", user, "-A", "trust", "--no-locale", "--encoding=UTF8"])
        start_postgres(pg / "pg_ctl", work / "data", work / "postgres.log",
                       f"-h '' -k {work} -p 5432", env)
        run([pg / "createdb", database])
        endpoint = f"host={work} port=5432 user={user} dbname={database} sslmode=disable"
        run([sys.executable, "-B", "products/p10-platform/database/migrate.py", "p07"],
            {"P07_MIGRATION_DATABASE_URL": endpoint})
        apply_predecessor = """
import os,sys
sys.path.insert(0, 'products/p10-platform/database')
import migrate
revisions = migrate.migrations(migrate.OWNERS['p05'][1])
migrate.apply('p05', os.environ['P05_MIGRATION_DATABASE_URL'], revisions[:int(sys.argv[1])])
"""
        run([sys.executable, "-B", "-c", apply_predecessor, str(prior)],
            {"P05_MIGRATION_DATABASE_URL": endpoint})
        print(f"W11 predecessor {prior} owned workspace {work}", flush=True)
        parent = "TestLoopbackCustomerOrderReceipt"
        children = ("KioskNearbyPaidSeparateOriginal", "W11NearbyClosedOriginal")
        verified_test(["sh", "products/p05-operations-backoffice/test/settlement/anvil.sh", "-json", "-run",
                       f"^{parent}$/^({'|'.join(children)})$"],
                      {f"{parent}/{child}" for child in children},
                      env | {"P05_SETTLEMENT_TEST_DATABASE_URL": endpoint, "NU54_W11_UNPAID_HOST": "1",
                             "NU54_W11_UNPAID_UPGRADE": str(prior)})
    if work.exists():
        raise RuntimeError("W11 upgrade scratch was not removed")
    print(f"W11 predecessor {prior} upgraded with owned scratch removed", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--postgres-bin", type=Path, default=os.environ.get("POSTGRES_BIN"))
    parser.add_argument("--predecessor", type=int, choices=(94, 95))
    args = parser.parse_args()
    if not args.postgres_bin:
        parser.error("set POSTGRES_BIN to the local PostgreSQL tool directory")
    for prior in (args.predecessor,) if args.predecessor else (94, 95):
        run_upgrade(args.postgres_bin.resolve(), prior)


if __name__ == "__main__":
    main()

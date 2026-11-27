#!/usr/bin/env python3
"""Own the PostgreSQL, Anvil and C fixtures for sponsored W10 settlement."""
import argparse
import os
from pathlib import Path
import sys

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(ROOT / "scripts"))
from integration_workspace import integration_workspace, run_command, start_postgres
from test_commercial_integrations import secure_env, verified_test


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--postgres-bin", type=Path, default=os.environ.get("POSTGRES_BIN"))
    modes = parser.add_mutually_exclusive_group()
    modes.add_argument("--fixture-only", action="store_true", help="check the deployment and host finance path without an app")
    modes.add_argument("--w11-pickup", action="store_true", help="verify the same sponsored order through merchant readiness")
    modes.add_argument("--w11-pending", action="store_true", help="check the original while finality is pending, then reopen its receipt")
    modes.add_argument("--w11-unpaid", action="store_true", help="verify finalized non-consumption of the original nearby quote")
    modes.add_argument("--w12-busy", action="store_true", help="observe the original board approval, then reread it after refusal")
    args = parser.parse_args()
    if not args.postgres_bin:
        parser.error("set POSTGRES_BIN to the local PostgreSQL tool directory")
    pg = args.postgres_bin.resolve()
    env = secure_env()
    if not args.fixture_only:
        for name in ("NU54_K10_SIM", "NU54_K10_APP", "NU54_K10_XCTESTRUN", "NU54_W10_IOS_HOST_PORT_FILE"):
            if not os.environ.get(name):
                parser.error(f"the signed, installed iOS QA app requires {name}")
            env[name] = os.environ[name]
        env["NU54_K10_INSTALLED_RUNNER"] = str(ROOT / "scripts/ios/scene-smoke/run-k10-installed.sh")
    for name, enabled in (("NU54_W11_INSTALLED_PICKUP", args.w11_pickup),
                          ("NU54_W11_INSTALLED_PENDING", args.w11_pending),
                          ("NU54_W11_INSTALLED_UNPAID", args.w11_unpaid)):
        env.pop(name, None)
        if enabled:
            env[name] = "1"
    with integration_workspace("nu54-w10-sponsored-", pg / "pg_ctl", "data", env) as (work, env):
        work = work.resolve()
        env |= {"PATH": f"{pg}{os.pathsep}{env['PATH']}", "PGHOST": str(work), "PGPORT": "5432", "PGUSER": "nu54_w10"}

        def run(command, *, active=env, **options):
            return run_command(command, cwd=ROOT, env=active, check=True, **options)

        run([str(pg / "initdb"), "-D", str(work / "data"), "-U", env["PGUSER"], "-A", "trust",
             "--no-locale", "--encoding=UTF8"], stdout=sys.stderr)
        start_postgres(pg / "pg_ctl", work / "data", work / "postgres.log",
                       f"-h '' -k {work} -p {env['PGPORT']}", env)
        name = "nu54_commerce_w12_busy" if args.w12_busy else "nu54_settlement_installed_w10"
        run([str(pg / "createdb"), name])
        for product in (("p05",) if args.w12_busy else ("p05", "p07")):
            run(["python3", "-B", "products/p10-platform/database/migrate.py", product],
                active=env | {f"{product.upper()}_MIGRATION_DATABASE_URL": name})
        board = work / "board"
        run(["cmake", "-S", "products/p01-device-firmware/test", "-B", str(board), "-DCMAKE_BUILD_TYPE=Debug"])
        run(["cmake", "--build", str(board), "--target", "owner_registration_host", "test_install"])
        if args.w12_busy:
            inputs = env | {
                "P05_COMMERCE_TEST_DATABASE_URL": f"host={work} port={env['PGPORT']} user={env['PGUSER']} dbname={name} sslmode=disable",
                "NU54_K10_SIGNED_REVIEW": "1",
                "NU54_W10_INSTALLED_BOARD": "1",
                "NU54_W12_INSTALLED_BUSY": "1",
                "NU54_W10_BOARD_VALIDATOR": str(board / "test_install"),
                "NU54_W10_BOARD_HOST": str(board / "owner_registration_host"),
            }
            required = {"TestPostgresOperatorCatalogRealTLS/InstalledK10VirtualNearby"}
            verified_test(["go", "-C", "products/p05-operations-backoffice", "test", "-count=1", "-run",
                           "^TestPostgresOperatorCatalogRealTLS$/^InstalledK10VirtualNearby$", "-json",
                           "./internal/commerce"], required, inputs)
            return
        inputs = env | {
            "P05_SETTLEMENT_TEST_DATABASE_URL": f"host={work} port={env['PGPORT']} user={env['PGUSER']} dbname={name} sslmode=disable",
            "NU54_P01_INSTALL_TEST": str(board / "test_install"),
            "NU54_W10_BOARD_VALIDATOR": str(board / "test_install"),
            "NU54_W10_BOARD_HOST": str(board / "owner_registration_host"),
            "NU54_W10_FIXTURE_ONLY": "1" if args.fixture_only else "0",
        }
        if not args.fixture_only:
            dist = work / "finance-dist"
            run(["pnpm", "--dir", "products/p05-operations-backoffice/web", "exec", "vite", "build", "--outDir", str(dist)])
            inputs["NU54_FINANCE_BROWSER_DIST"] = str(dist)
        required = {"TestLoopbackInstalledW10Fixture"}
        if args.fixture_only:
            required.update({"TestLoopbackSponsoredBoardPayment", "TestLoopbackW10InstalledFinalizer"})
        selected = "^(" + "|".join(sorted(required)) + ")$"
        verified_test(["sh", "products/p05-operations-backoffice/test/settlement/anvil.sh",
                       "-run", selected, "-json"], required, inputs)


if __name__ == "__main__":
    main()

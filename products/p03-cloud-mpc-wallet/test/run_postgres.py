#!/usr/bin/env python3
"""Run P03 workflow tests against a private scratch PostgreSQL."""
import os
from pathlib import Path
import sys

PRODUCT = Path(__file__).resolve().parents[1]
ROOT = PRODUCT.parents[1]
sys.dont_write_bytecode = True
sys.path.insert(0, str(ROOT / "scripts"))
from integration_workspace import integration_workspace, run_command, start_postgres


def postgres(tool):
    directory = os.getenv("POSTGRES_BIN")
    return str(Path(directory) / tool) if directory else tool


def run(args, *, cwd=PRODUCT, env=None, timeout=180):
    run_command(args, cwd=cwd, env=env, check=True, timeout=timeout)


with integration_workspace("nu54-p03-", postgres("pg_ctl"), "data", os.environ) as (scratch, env):
    data = scratch / "data"
    socket = scratch / "s"
    socket.mkdir(mode=0o700)
    run([postgres("initdb"), "-D", str(data), "-A", "trust", "-U", "p03_test", "--no-locale", "--encoding=UTF8"], env=env)
    start_postgres(postgres("pg_ctl"), data, scratch / "postgres.log",
                   f"-c listen_addresses='' -c unix_socket_directories={socket} -c unix_socket_permissions=0700", env, cwd=PRODUCT)
    env.update(P03_MIGRATION_DATABASE_URL="postgres",
               PGHOST=str(socket), PGUSER="p03_test", PGSSLMODE="disable",
               P03_TEST_DATABASE_URL=f"host={socket} dbname=postgres user=p03_test sslmode=disable")
    postgres_bin = os.getenv("POSTGRES_BIN")
    if postgres_bin:
        env["PATH"] = postgres_bin + os.pathsep + env["PATH"]
    run(["python3", "products/p10-platform/database/migrate.py", "p03"], cwd=ROOT, env=env)
    run(["python3", "test/check_upgrade.py"], env=env)
    run(["go", "test", "-v", "-count=1", "./internal/wallet", "-run", "^TestPostgres(Wallet|RecoveryRefusalAndReview)$"], env=env)
    recovery_browser = env.get("NU54_P03_BROWSER_RECOVERY") == "1" and bool(env.get("NU54_P03_BROWSER_LAB"))
    ceremony_timeout, subprocess_timeout = ("17m", 1040) if recovery_browser else ("10m", 610)
    run(["go", "test", "-v", "-count=1", "./internal/ceremony", "-run", "^TestRealCeremonies$",
         "-timeout", ceremony_timeout], env={**env, "NU54_P03_REAL_DKG": "1"}, timeout=subprocess_timeout)
    if not any(env.get(flag) for flag in ("NU54_P03_ANDROID_SERIAL", "NU54_P03_IOS_SIMULATOR", "NU54_P03_MULTI", "P03_TEST_MARKET_PROFILE")):
        run(["go", "test", "-v", "-count=1", "./internal/ceremony", "-run", "^TestRealCeremonies$", "-timeout", "10m"],
            env={**env, "NU54_P03_REAL_DKG": "1", "NU54_P03_MULTI": "1"}, timeout=610)

"""Run the K11 migration regression in an owned PostgreSQL workspace."""

import hashlib
import os
from pathlib import Path
import subprocess
import sys

if __name__ == "__main__":
    sys.dont_write_bytecode = True

from integration_workspace import (
    _defer_interruptions,
    cleanup_stack,
    integration_workspace,
    run_command,
    start_postgres,
)

ROOT = Path(__file__).resolve().parents[1]
P05 = ROOT / "products/p05-operations-backoffice"
SQL = P05 / "test/pos-upgrade"
DATABASE = "nu54_commerce_k11_upgrade"

# Run the production migrator in an owned subprocess group so terminating the
# fixture also terminates any psql child. Each checkpoint has its own journal.
MIGRATE = """
import runpy
import sys
module = runpy.run_path("products/p10-platform/database/migrate.py")
_, path = module["OWNERS"]["p05"]
through = int(sys.argv[2])
revisions = module["migrations"](path)[:through]
if len(revisions) != through:
    raise SystemExit("missing POS upgrade checkpoint")
module["apply"]("p05", sys.argv[1], revisions)
"""


def run(args, env, **kwargs):
    return run_command(args, cwd=ROOT, env=env, check=True, **kwargs)


def clean_postgres_env(environment):
    return {key: value for key, value in environment.items() if not key.startswith("PG")}


def run_upgrade(work, environment):
    socket = work / "socket"
    if environment.get("PGHOST") != str(socket):
        raise ValueError("POS upgrade requires the owned PostgreSQL socket")
    local = {**clean_postgres_env(environment), "PGHOST": str(socket),
             "PGPORT": environment["PGPORT"], "PGUSER": environment["PGUSER"], "PGSSLMODE": "disable"}
    psql = ["psql", "-w", "-X", "-q", "-v", "ON_ERROR_STOP=1", "-d", DATABASE]
    with cleanup_stack() as cleanup:
        with _defer_interruptions():
            run(["createdb", "--maintenance-db=postgres", DATABASE], local, timeout=15)
            cleanup.callback(run, ["dropdb", "--force", "--maintenance-db=postgres", DATABASE], local, timeout=15)
        run([sys.executable, "-B", "-c", MIGRATE, DATABASE, "75"], local, stdout=subprocess.DEVNULL)
        run([*psql, "-f", SQL / "seed.sql"], local)
        run([sys.executable, "-B", "-c", MIGRATE, DATABASE, "76"], local, stdout=subprocess.DEVNULL)
        migration = P05 / "internal/store/migrations/076_pos_checkout_selection.sql"
        digest = hashlib.sha256(migration.read_bytes()).hexdigest()
        run([*psql, "-c", f"""
DO $$
BEGIN
    IF (SELECT max(version) FROM nu54_p05_migrations.revision) IS DISTINCT FROM 76
       OR NOT EXISTS (
           SELECT FROM nu54_p05_migrations.revision
           WHERE version=76 AND name='{migration.name}' AND sha256='{digest}'
       ) THEN
        RAISE EXCEPTION 'missing or inconsistent POS upgrade migration journal';
    END IF;
END $$;
"""], local)
        run([*psql, "-f", SQL / "verify.sql"], local)
    print("PASS K11 PostgreSQL upgrade 075→076 (one SQL fixture, signed replay and held drafts)", flush=True)


def main():
    environment = clean_postgres_env(os.environ)
    version = run(["pg_ctl", "--version"], environment, capture_output=True, text=True)
    if not version.stdout.startswith("pg_ctl (PostgreSQL) 15."):
        raise SystemExit("PostgreSQL 15 tools are required")
    with integration_workspace("n54-pos.", "pg_ctl", "data", environment) as (work, env):
        data, socket = work / "data", work / "socket"
        socket.mkdir(mode=0o700)
        user = "nu54_pos_fixture"
        run(["initdb", "-D", data, "-A", "trust", "--no-locale", "--encoding=UTF8", "-U", user], env,
            stdout=subprocess.DEVNULL)
        start_postgres("pg_ctl", data, work / "postgres.log",
                       f"-c listen_addresses='' -c unix_socket_directories={socket} -c unix_socket_permissions=0700",
                       env, cwd=ROOT, stdout=subprocess.DEVNULL)
        run_upgrade(work, {**env, "PGHOST": str(socket), "PGPORT": "5432", "PGUSER": user})


if __name__ == "__main__":
    main()

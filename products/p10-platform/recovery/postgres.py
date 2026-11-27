"""Snapshot and read-only reconciliation of P05 and P07 in an isolated cluster."""

import hashlib
import json
import os
import re
import subprocess
from contextlib import ExitStack, contextmanager
from pathlib import PurePosixPath

PRODUCTS = ("p05", "p07")
TABLES = {
    "p05": (
        "ledger.sale", "ledger.quote_work", "ledger.quote_outbox", "ledger.payment",
        "ledger.refund", "ledger.refund_confirmation", "ledger.journal", "ledger.pickup",
        "ledger.fulfillment_outbox", "ledger.fulfillment_inbox", "ledger.consumer_receipt",
        "ledger.settlement_work", "ledger.relay_attempt", "ledger.print_attempt", "ledger.print_decision",
        "ledger.subscription_offer", "ledger.subscription", "ledger.invoice", "ledger.travel_booking", "ledger.travel_booking_leg",
        "ledger.payout_change", "ledger.payout_approval",
    ),
    "p07": (
        "chainview.stream", "chainview.deployment_profile", "chainview.generation",
        "chainview.block", "chainview.observed_event", "chainview.evidence",
        "chainview.read_target", "chainview.incident",
    ),
}


def db_environment(path=None):
    env = {k: v for k, v in os.environ.items() if not k.startswith("PG")}
    if path:
        env["PATH"] = str(path) + os.pathsep + env["PATH"]
    return env


def local_database(environment, socket, user, port=5432):
    return {**environment, "PGHOST": str(socket), "PGPORT": str(port),
            "PGUSER": user, "PGSSLMODE": "disable"}


def pg(env, args, **kwargs):
    try:
        return subprocess.run(args, env=env, check=True, timeout=3600, **kwargs)
    except subprocess.CalledProcessError as exc:
        # libpq DSNs may identify a private host. Never log argv or query contents.
        raise RuntimeError(f"PostgreSQL {PurePosixPath(args[0]).name} failed ({exc.returncode})") from None


def sql(env, database, statement, snapshot=None):
    transaction = "BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;\n"
    if snapshot is not None:
        if not re.fullmatch(r"[0-9A-Fa-f]+-[0-9A-Fa-f]+-[0-9]+", snapshot):
            raise ValueError("invalid exported snapshot identity")
        transaction += f"SET TRANSACTION SNAPSHOT '{snapshot}';\n"
    transaction += "SET LOCAL timezone = 'UTC';\nSET LOCAL bytea_output = 'hex';\n"
    return pg(env, ["psql", "-X", "-qAtw", "-v", "ON_ERROR_STOP=1", "-d", database, "-f", "-"],
              input=transaction + statement + "\nCOMMIT;\n", text=True, capture_output=True).stdout.strip()


def file_digest(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


@contextmanager
def exported_snapshot(env, database):
    args = ["psql", "-X", "-qAtw", "-v", "ON_ERROR_STOP=1", "-d", database]
    with subprocess.Popen(args, env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True) as session:
        try:
            session.stdin.write("BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;\n"
                                "SELECT pg_export_snapshot(), extract(epoch FROM transaction_timestamp())::text, "
                                "current_setting('server_version_num')::int / 10000;\n")
            session.stdin.flush()
            parts = session.stdout.readline().strip().split("|")
            if len(parts) != 3 or not parts[2].isdecimal() or int(parts[2]) < 15:
                raise ValueError("recovery requires a live PostgreSQL 15 or later source")
            yield parts[0], parts[1]
        finally:
            session.stdin.close()


def owner_inventory(env, database, owner, snapshot=None):
    counts = {}
    tables = TABLES[owner]
    if owner == "p05" and sql(env, database, "SELECT to_regclass('ledger.subscription_entitlement') IS NOT NULL;", snapshot) == "t":
        tables += ("ledger.subscription_entitlement",)
    # Backups made before the order escrow have none of its tables.
    if owner == "p05" and sql(env, database, "SELECT to_regclass('ledger.order_escrow') IS NOT NULL;", snapshot) == "t":
        tables += ("ledger.order_escrow",)
    if owner == "p05" and sql(env, database, "SELECT to_regclass('ledger.escrow_action') IS NOT NULL;", snapshot) == "t":
        tables += ("ledger.order_escrow_event", "ledger.escrow_journal", "ledger.escrow_action",
                   "ledger.escrow_claim_order", "ledger.escrow_decision")
    if owner == "p05" and sql(env, database, "SELECT to_regclass('ledger.wallet_return') IS NOT NULL;", snapshot) == "t":
        tables += ("ledger.relay_version", "ledger.payer_enrollment_request", "ledger.wallet_return")
    if owner == "p05" and sql(env, database, "SELECT to_regclass('ledger.faucet_claim') IS NOT NULL;", snapshot) == "t":
        tables += ("ledger.faucet_claim",)
    if owner == "p05" and sql(env, database, "SELECT to_regclass('ledger.wallet_send') IS NOT NULL;", snapshot) == "t":
        tables += ("ledger.wallet_send",)
    if owner == "p05" and sql(env, database, "SELECT to_regclass('ledger.terminal_device') IS NOT NULL;", snapshot) == "t":
        tables += ("ledger.terminal_device", "ledger.terminal_device_change")
    for table in tables:
        counts[table] = row_inventory(env, database, table, snapshot)
    return counts


def row_inventory(env, database, table, snapshot):
    # Stream only fixed-width row hashes. Full tables and their contents never
    # have to fit in PostgreSQL's aggregate-size limit or process memory.
    cmd = ["psql", "-X", "-qAtw", "-v", "ON_ERROR_STOP=1", "-d", database]
    transaction = "BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;\n"
    if snapshot is not None:
        if not re.fullmatch(r"[0-9A-Fa-f]+-[0-9A-Fa-f]+-[0-9]+", snapshot):
            raise ValueError("invalid exported snapshot identity")
        transaction += f"SET TRANSACTION SNAPSHOT '{snapshot}';\n"
    statement = transaction + f"""
SET LOCAL timezone='UTC';
SET LOCAL bytea_output='hex';
COPY (SELECT encode(sha256(convert_to(to_jsonb(t)::text,'UTF8')),'hex') COLLATE "C" AS row_hash
      FROM {table} t ORDER BY row_hash) TO STDOUT;
COMMIT;
"""
    hasher, count = hashlib.sha256(), 0
    with subprocess.Popen(cmd, env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                          stderr=subprocess.DEVNULL) as session:
        session.stdin.write(statement.encode())
        session.stdin.close()
        for line in session.stdout:
            if not re.fullmatch(rb"[0-9a-f]{64}\n", line):
                session.kill()
                raise ValueError("invalid database row inventory hash")
            hasher.update(line[:-1])
            count += 1
        if session.wait(timeout=5) != 0:
            raise ValueError(f"could not inventory {table}")
    return {"count": count, "digest": hasher.hexdigest()}


def backup(env, sources, folder):
    folder.mkdir(mode=0o700)
    result = {}
    snapshots = {}
    with ExitStack() as stack:
        for database in dict.fromkeys(sources.values()):
            snapshots[database] = stack.enter_context(exported_snapshot(env, database))
            snapshot, _ = snapshots[database]
            foreign = sql(env, database, """
                SELECT count(*) FROM pg_namespace WHERE nspname NOT LIKE 'pg_%'
                  AND nspname NOT IN ('information_schema','public','ledger','chainview',
                    'nu54_p05_migrations','nu54_p07_migrations');
            """, snapshot)
            unexpected_public = sql(env, database, """
                SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
                WHERE n.nspname='public' AND c.relkind IN ('r','p','v','m','f')
                  AND c.relname NOT IN ('merchants','rentals','attestations','audit_log','receipts','cursor');
            """, snapshot)
            if foreign != "0" or unexpected_public != "0":
                raise ValueError("backup source must contain only P05/P07-owned schemas and legacy relations")
        for owner in PRODUCTS:
            database = sources[owner]
            snapshot, snapshot_at = snapshots[database]
            archive = folder / (owner + ".pgdump")
            pg(env, ["pg_dump", "-w", "-Fc", "--snapshot", snapshot, "-d", database, "-f", str(archive)])
            archive.chmod(0o600)
            result[owner] = {"sha256": file_digest(archive), "snapshotAt": snapshot_at,
                             "inventory": owner_inventory(env, database, owner, snapshot)}
    return result


class Quarantine:
    """Fresh OS-restricted cluster. No application principals, TCP listener or workers."""

    def __init__(self, root, tools):
        self.root = root.resolve()
        self.data = self.root / "data"
        self.socket = self.root / "socket"
        self.environment = db_environment(tools)
        self.env = local_database(self.environment, self.socket, "recovery_operator")

    def start(self):
        if not re.fullmatch(r"[A-Za-z0-9_./-]+", str(self.socket)) or len(str(self.socket)) > 80:
            raise ValueError("quarantine requires a short, plain filesystem path")
        self.root.mkdir(mode=0o700)
        self.socket.mkdir(mode=0o700)
        self._initdb(["initdb", "-D", str(self.data), "-A", "peer", "--no-locale",
                      "-U", self.env["PGUSER"]])
        # Map only the OS account running this private recovery process.
        import pwd
        account = pwd.getpwuid(os.getuid()).pw_name
        if not re.fullmatch(r"[A-Za-z0-9_-]+", account):
            raise ValueError("unsupported quarantine OS account name")
        (self.data / "pg_ident.conf").write_text(f"nu54_recovery {account} {self.env['PGUSER']}\n")
        (self.data / "pg_hba.conf").write_text("local all recovery_operator peer map=nu54_recovery\n")
        self._start_server(f"-c listen_addresses='' -c unix_socket_directories={self.socket} "
                           "-c unix_socket_permissions=0700")

    def _initdb(self, args):
        pg(self.environment, args)

    def _start_server(self, options):
        pg(self.environment, ["pg_ctl", "-D", str(self.data), "-w", "-l", str(self.root / "postgres.log"),
                              "-o", options, "start"])

    def stop(self):
        pg(self.environment, ["pg_ctl", "-D", str(self.data), "-m", "fast", "-w", "stop"])

    def restore(self, folder, backup_record):
        for owner in PRODUCTS:
            archive = folder / (owner + ".pgdump")
            if file_digest(archive) != backup_record[owner]["sha256"]:
                raise ValueError(f"{owner} archive digest differs")
            pg(self.env, ["createdb", "-w", owner])
            pg(self.env, ["pg_restore", "-w", "--exit-on-error", "--single-transaction", "--no-owner", "--no-privileges",
                          "-d", owner, str(archive)])
            if sql(self.env, owner, "SHOW listen_addresses;"):
                raise ValueError("unexpected network accessibility in quarantine")

    def probe_ordinary_access(self):
        ordinary = {**self.env, "PGUSER": "ordinary_recovery_probe"}
        for owner in PRODUCTS:
            read = "SELECT count(*) FROM ledger.sale" if owner == "p05" else "SELECT count(*) FROM chainview.stream"
            write = "SELECT * FROM ledger.claim_settlement_work()" if owner == "p05" else "DELETE FROM chainview.read_target"
            for statement in (read, write):
                process = subprocess.run(
                    ["psql", "-X", "-w", "-d", owner, "-c", statement],
                    env=ordinary, capture_output=True, text=True, timeout=5, check=False,
                )
                if process.returncode == 0 or "pg_hba.conf" not in process.stderr:
                    return False
        return True

    def reconcile(self, reference):
        inventory_deltas = {}
        for owner in PRODUCTS:
            inventory = owner_inventory(self.env, owner, owner)
            inventory_deltas[owner] = {table: {"expected": reference[owner]["inventory"][table]["count"],
                                              "observed": inventory[table]["count"]}
                                       for table in inventory if inventory[table] != reference[owner]["inventory"][table]}
        return inventory_deltas


def sql_json(env, database, query):
    return json.loads(sql(env, database, f"SELECT COALESCE(jsonb_agg(to_jsonb(rows)),'[]'::jsonb) FROM ({query}) AS rows;"))

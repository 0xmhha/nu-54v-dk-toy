#!/usr/bin/env python3
"""Apply one owner's ordered PostgreSQL migrations, with a transactional journal."""

import argparse
import hashlib
import json
import os
import re
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from urllib.parse import parse_qsl, unquote, urlsplit

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[2]
OWNERS = {
    "p03": ("P03_DATABASE_URL", ROOT / "products/p03-cloud-mpc-wallet/migrations"),
    "p05": ("P05_DATABASE_URL", ROOT / "products/p05-operations-backoffice/internal/store/migrations"),
    "p07": ("P07_DATABASE_URL", ROOT / "products/p07-indexer/migrations"),
    "p08": ("P08_DATABASE_URL", ROOT / "products/p08-market-services/migrations"),
    "p09": ("P09_DATABASE_URL", ROOT / "products/p09-travel-ai/migrations"),
    "p10": ("P10_DATABASE_URL", ROOT / "products/p10-platform/database/migrations"),
}


@dataclass(frozen=True)
class Migration:
    version: int
    name: str
    digest: str
    sql: str


def migrations(path):
    result = []
    for file in sorted(path.glob("*.sql")):
        match = re.fullmatch(r"(\d{3})_[a-z0-9_]+\.sql", file.name)
        if not match or int(match.group(1)) != len(result) + 1:
            raise ValueError(f"migration versions must be contiguous: {file.name}")
        body = file.read_bytes()
        sql = body.decode()
        # Files are reviewed source, and this catches accidental standalone transactions
        # or psql directives copied from the design DDLs.
        if re.search(r"(?im)(?:^|;)[ \t]*(?:(?:BEGIN|COMMIT|ROLLBACK)\s*(?:;|TRANSACTION\b)|\\)", sql):
            raise ValueError(f"runner owns the transaction and psql directives: {file.name}")
        result.append(Migration(int(match.group(1)), file.name, hashlib.sha256(body).hexdigest(), sql))
    if not result:
        raise ValueError(f"no migrations in {path}")
    return result


def check_connection(url):
    # psql's connection-string argument is visible to process listings. Accept
    # connection endpoints and service names here, with secrets in libpq's passfile.
    secret_hint = "use PGSERVICE and PGPASSFILE for credentials"
    if url.startswith(("postgresql://", "postgres://")):
        try:
            parts = urlsplit(url)
            embedded = parts.password is not None or any(
                key.lower() in {"password", "sslpassword"} for key, _ in parse_qsl(parts.query)
            )
        except ValueError:
            raise ValueError("invalid PostgreSQL endpoint") from None
    else:
        decoded = unquote(url)
        embedded = bool(re.search(r"(?i)password\s*=|postgres(?:ql)?://", decoded))
    if embedded:
        raise ValueError(f"embedded migration credential rejected; {secret_hint}")


def field_policy_check(owner, version):
    policy = json.loads((HERE / "field-purpose.json").read_text())["rules"]
    schemas = {"p03": ("wallet_mpc",), "p05": ("ledger",), "p07": ("chainview",), "p08": ("market",), "p09": ("travel", "travel_private", "travel_erasure"), "p10": ("platform", "privacy_ops")}
    owner_rules = [rule for rule in policy if rule["owner"] == owner and rule["introducedVersion"] <= version]
    fields = []
    for rule in owner_rules:
        for key in ("purpose", "basisRef", "retentionTrigger", "periodRef", "disposal"):
            if not rule.get(key, "").strip():
                raise ValueError(f"missing {key} for {rule['table']}")
        fields.extend(f"{rule['table']}.{column}" for column in rule["columns"])

    def quoted(identifier):
        if not re.fullmatch(r"[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*){0,2}", identifier):
            raise ValueError("invalid field policy identifier")
        return f"'{identifier}'"

    values = ", ".join(map(quoted, fields))
    namespaces = ", ".join(map(quoted, schemas[owner]))
    return f"""
DO $field_policy$
DECLARE
    actual text[];
    expected text[] := ARRAY(SELECT unnest(ARRAY[{values}]::text[]) ORDER BY 1);
BEGIN
    SELECT COALESCE(array_agg(field ORDER BY field), ARRAY[]::text[]) INTO actual
    FROM (
        SELECT n.nspname||'.'||c.relname||'.'||a.attname AS field
        FROM pg_catalog.pg_class c
        JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
        JOIN pg_catalog.pg_attribute a ON a.attrelid=c.oid
        WHERE n.nspname IN ({namespaces}) AND c.relkind IN ('r','p')
        AND a.attnum > 0 AND NOT a.attisdropped
    ) AS columns;
    IF actual IS DISTINCT FROM expected THEN
        RAISE EXCEPTION 'field policy does not match {owner} schema'
          USING DETAIL = format('expected %s, observed %s', expected, actual);
    END IF;
END
$field_policy$;
"""


class MigrationRejected(ValueError):
    def __init__(self, owner, file, psql_stderr):
        super().__init__(f"{owner} {file} halted; transaction rolled back")
        self.psql_stderr = psql_stderr


def apply(owner, url, revisions):
    check_connection(url)
    journal = f"nu54_{owner}_migrations.revision"
    # The lock avoids a concurrent first writer racing schema creation, as well as two
    # attempts at one revision. Every version checks the complete earlier history.
    for item in revisions:
        policy_check = field_policy_check(owner, item.version)
        final_check = policy_check if item.version == len(revisions) else ""
        cmd = f"""
BEGIN;
SELECT pg_catalog.pg_advisory_xact_lock(pg_catalog.hashtext('nu54-{owner}-migration'));
-- Public is the legacy object namespace; pg_catalog is searched implicitly first.
SET LOCAL search_path = public, pg_temp;
DO $namespace$
BEGIN
    IF EXISTS (SELECT FROM pg_namespace AS n
        CROSS JOIN LATERAL aclexplode(n.nspacl) AS permission
        WHERE n.nspname='public' AND permission.grantee=0 AND permission.privilege_type='CREATE') THEN
        RAISE EXCEPTION 'revoke PUBLIC CREATE on public before applying legacy migrations';
    END IF;
END
$namespace$;
CREATE SCHEMA IF NOT EXISTS nu54_{owner}_migrations;
REVOKE ALL ON SCHEMA nu54_{owner}_migrations FROM PUBLIC;
CREATE TABLE IF NOT EXISTS {journal} (
  version integer PRIMARY KEY,
  name text NOT NULL,
  sha256 text NOT NULL,
  applied_at timestamptz NOT NULL DEFAULT now()
);
DO $journal$
BEGIN
    IF EXISTS (SELECT FROM {journal} WHERE version > {len(revisions)}) THEN
        RAISE EXCEPTION 'database has a newer {owner} migration';
    END IF;
    IF (SELECT count(*) FROM {journal} WHERE version < {item.version}) <> {item.version - 1} THEN
        RAISE EXCEPTION 'migration journal gap before {item.name}';
    END IF;
    IF EXISTS (SELECT FROM {journal} WHERE version = {item.version}
        AND (name <> '{item.name}' OR sha256 <> '{item.digest}')) THEN
        RAISE EXCEPTION 'migration digest mismatch for {item.name}';
    END IF;
END
$journal$;
SELECT NOT EXISTS (SELECT FROM {journal} WHERE version = {item.version}) AS needs_migration \\gset
\\if :needs_migration
{item.sql}
{policy_check}
INSERT INTO {journal}(version,name,sha256) VALUES ({item.version},'{item.name}','{item.digest}');
\\else
{final_check}
\\endif
COMMIT;
"""
        result = subprocess.run(["psql", "-X", "-q", "-w", "--dbname", url, "-v", "ON_ERROR_STOP=1", "-f", "-"],
                                input=cmd, text=True, stderr=subprocess.PIPE)
        if result.stderr:
            sys.stderr.write(result.stderr)
        if result.returncode:
            raise MigrationRejected(owner, item.name, result.stderr)
        print(f"{owner} {item.name} verified", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("owner", choices=OWNERS)
    args = parser.parse_args()
    name, path = OWNERS[args.owner]
    try:
        # Deliberate, owner-specific migration credentials. Never reuse runtime credentials implicitly.
        variable = name.replace("_DATABASE_URL", "_MIGRATION_DATABASE_URL")
        url = os.environ.get(variable)
        if not url:
            raise ValueError(f"{variable} is required")
        apply(args.owner, url, migrations(path))
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        parser.exit(1, f"{error}\n")


if __name__ == "__main__":
    main()

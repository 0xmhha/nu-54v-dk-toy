#!/usr/bin/env python3
"""Create authenticated P05/P07 snapshots and inspect quarantined recovery."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import sys
from decimal import Decimal, InvalidOperation, ROUND_CEILING
import time
from uuid import UUID

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parent / "release"))
sys.path.insert(0, str(HERE.parent / "database"))
from evidence import authenticate, openssl, parse, read
from migrate import check_connection
from postgres import PRODUCTS, Quarantine, backup, db_environment, file_digest
from reconcile import Chain, reconcile
from erasure import apply_and_recheck


def save(path, record):
    raw = json.dumps(record, sort_keys=True, indent=2, ensure_ascii=True).encode() + b"\n"
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "wb") as stream:
        stream.write(raw)
        stream.flush()
        os.fsync(stream.fileno())
    return hashlib.sha256(raw).hexdigest()


def write_backup(args):
    scope = str(UUID(args.scope))
    sources = {owner: getattr(args, owner) for owner in PRODUCTS}
    for source in sources.values():
        check_connection(source)
    env = db_environment(args.postgres_bin)
    for key in ("PGSERVICEFILE", "PGPASSFILE", "PGSSLROOTCERT", "PGSSLCERT", "PGSSLKEY",
                "PGHOST", "PGPORT", "PGUSER", "PGSSLMODE"):
        if key in os.environ:
            env[key] = os.environ[key]
    manifest = args.archive / "manifest.json"
    save(manifest, {"format": "nu54-recovery-snapshot-v1", "scope": scope,
                    "targets": backup(env, sources, args.archive)})
    signature = args.archive / "manifest.ed25519"
    signature.write_bytes(openssl(["pkeyutl", "-sign", "-rawin", "-inkey", args.sign_key, "-in", manifest]))
    signature.chmod(0o600)
    print(file_digest(manifest))


def read_backup(folder, public_key, scope):
    manifest = folder / "manifest.json"
    raw = manifest.read_bytes()
    authenticate(raw, folder / "manifest.ed25519", public_key.read_bytes())
    record = parse(raw)
    if (record["format"] != "nu54-recovery-snapshot-v1" or record["scope"] != str(UUID(scope)) or
            set(record["targets"]) != set(PRODUCTS) or manifest.read_bytes() != raw):
        raise ValueError("backup signature or scope differs")
    return record["targets"], hashlib.sha256(raw).hexdigest()


def inspect_restore(args, progress=lambda _: None):
    started = time.monotonic_ns()
    begun = Decimal(time.time_ns()) / Decimal(10**9)
    incident = Decimal(args.incident_epoch) if args.incident_epoch else begun
    if not args.incident_epoch and not args.synthetic:
        raise ValueError("operational recovery requires a recorded incident timestamp")
    if not incident.is_finite() or incident > begun or incident < 0:
        raise ValueError("incident time must be the recorded source disconnect time")
    manifest, manifest_id = read_backup(args.archive, args.backup_key, args.scope)
    snapshots = [Decimal(row["snapshotAt"]) for row in manifest.values()]
    if any(not snapshot.is_finite() or snapshot > incident for snapshot in snapshots):
        raise ValueError("signed snapshots must predate the recorded incident")
    rpo = int((incident - min(snapshots)).to_integral_value(rounding=ROUND_CEILING))
    rpc_deadline = started / 1e9 + args.rto - float(begun - incident)
    chain = Chain(args.rpc, args.synthetic, rpc_deadline)
    isolation = Quarantine(args.isolation_root, args.postgres_bin)
    report = {"format": "nu54-recovery-report-v1", "scope": str(UUID(args.scope)), "job": str(UUID(args.job)),
              "manifest": manifest_id, "status": "hold", "synthetic": args.synthetic,
              "incidentEpoch": str(incident),
              "rpoSeconds": rpo, "goals": {"rpoSeconds": args.rpo, "rtoSeconds": args.rto},
              "goalRef": str(UUID(args.goal_ref)), "ordinaryTrafficFenced": False,
              "erasure": None}
    try:
        progress("restore")
        isolation.start()
        isolation.restore(args.archive, manifest)
        progress("isolation")
        report["ordinaryTrafficFenced"] = isolation.probe_ordinary_access()
        erasure_required = args.erasure is not None
        progress("erasure")
        if erasure_required:
            report["erasure"] = apply_and_recheck(read(args.erasure))
        progress("replay")
        report["reconciliation"] = reconcile(isolation, manifest, chain)
        validation_ns = time.monotonic_ns() - started
        rehearsal_rto = int(((begun - incident) + Decimal(validation_ns) / Decimal(10**9)).to_integral_value(rounding=ROUND_CEILING))
        report["validationSeconds"] = rehearsal_rto
        if all((rpo <= args.rpo, rehearsal_rto <= args.rto, report["ordinaryTrafficFenced"],
                report["reconciliation"]["matched"], report["erasure"] is not None or args.synthetic)):
            report["status"] = "await-independent-verification"
    except (OSError, RuntimeError, ValueError, KeyError, TypeError) as exc:
        report["error"] = str(exc)
    finally:
        if "validationSeconds" not in report:
            elapsed = (begun - incident) + Decimal(time.monotonic_ns() - started) / Decimal(10**9)
            report["validationSeconds"] = int(elapsed.to_integral_value(rounding=ROUND_CEILING))
        if (isolation.data / "postmaster.pid").exists():
            isolation.stop()
        report_id = save(args.report, report)
        print(f'{report["status"]} {report_id}')
    return 0 if report["status"] == "await-independent-verification" else 1


def seconds(value):
    duration = int(value)
    if not 0 <= duration <= 7 * 24 * 3600:
        raise argparse.ArgumentTypeError("goal seconds outside supported range")
    return duration


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--postgres-bin", type=Path)
    commands = parser.add_subparsers(dest="command", required=True)
    create = commands.add_parser("backup")
    create.add_argument("--scope", required=True)
    create.add_argument("--p05", required=True)
    create.add_argument("--p07", required=True)
    create.add_argument("--archive", required=True, type=Path)
    create.add_argument("--sign-key", required=True, type=Path)
    inspect = commands.add_parser("inspect")
    inspect.add_argument("--scope", required=True)
    inspect.add_argument("--job", required=True)
    inspect.add_argument("--archive", required=True, type=Path)
    inspect.add_argument("--backup-key", required=True, type=Path)
    inspect.add_argument("--isolation-root", required=True, type=Path)
    inspect.add_argument("--rpc", required=True)
    inspect.add_argument("--erasure", type=Path)
    inspect.add_argument("--synthetic", action="store_true")
    inspect.add_argument("--incident-epoch")
    inspect.add_argument("--goal-ref", required=True)
    inspect.add_argument("--rpo", required=True, type=seconds)
    inspect.add_argument("--rto", required=True, type=seconds)
    inspect.add_argument("--report", required=True, type=Path)
    args = parser.parse_args()
    if args.command == "backup":
        write_backup(args)
        return 0
    return inspect_restore(args)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (InvalidOperation, ValueError, RuntimeError, KeyError, TypeError, OSError) as exc:
        sys.exit(str(exc))

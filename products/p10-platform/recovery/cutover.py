#!/usr/bin/env python3
"""Coordinate independently signed recovery decisions and target compare-and-swap receipts."""

import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import time
from contextlib import contextmanager
from decimal import Decimal
from uuid import UUID

from recover import read, parse, save, openssl, authenticate, check_connection
from postgres import pg

ROLES = ("requester", "verifier", "applier")
TARGETS = ("p05", "p07", "p09")
DOMAIN = "nu54-recovery-v1"
HASH = re.compile(r"[0-9a-f]{64}")
MAX_REVISION = 2**63 - 2


def require(condition, message):
    if not condition:
        raise ValueError(message)


def canonical(record):
    return json.dumps(record, sort_keys=True, separators=(",", ":"), ensure_ascii=True, allow_nan=False).encode()


def digest(record):
    return hashlib.sha256(canonical(record)).hexdigest()


def uuid4_text(value):
    require(str(UUID(value)) == value, "noncanonical recovery scope or work ID")
    return value


def text_hash(value):
    require(isinstance(value, str) and HASH.fullmatch(value), "invalid recovery digest")
    return value


def revision(value):
    require(type(value) is int and 0 <= value <= MAX_REVISION, "invalid expected recovery revision")
    return value


def key_identity(public):
    der = openssl(["pkey", "-pubin", "-in", public, "-outform", "DER"])
    require(len(der) == 44 and der.startswith(bytes.fromhex("302a300506032b6570032100")),
            "recovery requires Ed25519 keys")
    return hashlib.sha256(der).hexdigest()


def sign(payload, key):
    with tempfile.TemporaryDirectory(prefix="n54-dr-sign-") as root:
        path = Path(root) / "payload"
        path.write_bytes(canonical(payload))
        return base64.b64encode(openssl(["pkeyutl", "-sign", "-rawin", "-inkey", key, "-in", path])).decode()


def verify(payload, signature, public):
    fingerprint = key_identity(public)
    raw = base64.b64decode(signature, validate=True)
    require(len(raw) == 64, "invalid Ed25519 signature length")
    with tempfile.TemporaryDirectory(prefix="n54-dr-verify-") as root:
        path = Path(root) / "signature"
        path.write_bytes(raw)
        require(authenticate(canonical(payload), path, public.read_bytes()) == fingerprint, "recovery signer mismatch")
    return fingerprint


def vote_payload(plan, role, actor):
    return {"domain": DOMAIN, "kind": "vote", "role": role, "actor": actor, "planDigest": digest(plan)}


def validate_plan(plan, config, allow_expired=False):
    require(set(plan) == {"kind", "job", "scope", "priorJob", "manifest", "incumbent", "routeRevision",
                           "targets", "evidence", "goals", "goalRef", "issuedAt", "expiresAt"}, "invalid plan fields")
    uuid4_text(plan["job"])
    scope = uuid4_text(plan["scope"])
    require(plan["kind"] in ("cutover", "rollback"), "invalid recovery action")
    require((plan["kind"] == "rollback") == (plan["priorJob"] is not None), "missing or unexpected rollback parent")
    if plan["priorJob"] is not None:
        require(uuid4_text(plan["priorJob"]) != plan["job"], "rollback cannot replace itself")
    spec = config["scopes"][scope]
    require(set(plan["targets"]) == set(spec["targets"]) and {"p05", "p07"} <= set(plan["targets"]) <= set(TARGETS),
            "configured scope and target roster differ")
    require(plan["goalRef"] == spec["goalRef"] and plan["goals"] == spec["goals"], "recovery goals differ from enrollment")
    uuid4_text(plan["goalRef"])
    require(set(plan["goals"]) == {"rpoSeconds", "rtoSeconds"} and
            all(type(seconds) is int and 0 <= seconds <= 7 * 24 * 3600 for seconds in plan["goals"].values()),
            "invalid enrolled recovery goals")
    revise = revision(plan["routeRevision"])
    for expected in plan["targets"].values():
        revision(expected)
    for field in ("manifest", "incumbent", "evidence"):
        text_hash(plan[field])
    require(plan["manifest"] != plan["incumbent"] or plan["kind"] == "rollback", "new manifest matches incumbent")
    start, expiry = plan["issuedAt"], plan["expiresAt"]
    now = int(time.time())
    require(type(start) is int and type(expiry) is int and 0 < expiry - start <= spec["maxApprovalSeconds"]
            and start <= now and (now < expiry or allow_expired), "recovery approval is outside its valid window")
    return spec, revise


def validate_votes(bundle, config, allow_expired=False):
    require(set(bundle) == {"plan", "votes"}, "invalid signed recovery bundle")
    plan, votes = bundle["plan"], bundle["votes"]
    spec, _ = validate_plan(plan, config, allow_expired)
    require(set(votes) == set(ROLES), "all three independent approvals are required")
    people, identities, keys = set(), set(), set()
    for role in ROLES:
        vote = votes[role]
        require(set(vote) == {"actor", "signature"}, "invalid vote")
        actor = uuid4_text(vote["actor"])
        principal = config["principals"][actor]
        require(role in principal["roles"] and plan["scope"] in principal["scopes"], "actor role or scope denied")
        require(principal["person"] and principal["issuer"] and principal["subject"], "missing original IdP identity")
        people.add(principal["person"])
        identities.add((principal["issuer"], principal["subject"]))
        keys.add(verify(vote_payload(plan, role, actor), vote["signature"], Path(principal["publicKey"])))
    target_keys = {key_identity(Path(path)) for path in spec["targets"].values()}
    require(len(people) == len(identities) == len(keys) == len(ROLES) and
            len(target_keys) == len(spec["targets"]) and keys.isdisjoint(target_keys), "recovery principals must be independent")
    return plan, spec


def verify_evidence(plan, spec, report):
    raw = report.read_bytes()
    require(hashlib.sha256(raw).hexdigest() == plan["evidence"], "evidence digest differs")
    measurements = parse(raw)
    require(measurements["format"] == "nu54-recovery-report-v1" and measurements["scope"] == plan["scope"] and
            measurements["job"] == plan["job"] and
            measurements["manifest"] == plan["manifest"] and measurements["goalRef"] == plan["goalRef"] and
            measurements["goals"] == plan["goals"] and measurements["status"] == "await-independent-verification" and
            measurements["ordinaryTrafficFenced"] is True and measurements["reconciliation"]["matched"] is True and
            0 <= measurements["rpoSeconds"] <= plan["goals"]["rpoSeconds"] and
            0 <= measurements["validationSeconds"] <= plan["goals"]["rtoSeconds"], "restore evidence remains on hold")
    require(spec.get("synthetic") is measurements["synthetic"], "evaluation channel differs")
    if "p09" in plan["targets"]:
        erasure = measurements["erasure"]
        require(erasure and erasure["namespace"] == spec["erasureNamespace"] and
                erasure["highWater"] == erasure["appliedThrough"], "erasure replay is incomplete")
    else:
        require(measurements["synthetic"], "operational cutover requires P09 erasure evidence")
    incident = Decimal(measurements["incidentEpoch"])
    require(incident.is_finite() and 0 <= incident <= time.time(), "invalid incident clock")
    return measurements


def literal(value):
    return "'" + value.replace("'", "''") + "'"


def j(value):
    return literal(canonical(value).decode()) + "::jsonb"


def h(value):
    return f"decode({literal(text_hash(value))},'hex')"


def q(value):
    return literal(uuid4_text(value)) + "::uuid"


def db(dsn, statement):
    check_connection(dsn)
    return pg({**os.environ, "PGCONNECT_TIMEOUT": "5"}, ["psql", "-X", "-qAtw", "-v", "ON_ERROR_STOP=1", "-d", dsn, "-f", "-"],
              input="SET standard_conforming_strings=on;\n" + statement, text=True, capture_output=True).stdout.strip()


@contextmanager
def transaction_guard(dsn, setup, probe):
    check_connection(dsn)
    with subprocess.Popen(["psql", "-X", "-qAtw", "-v", "ON_ERROR_STOP=1", "-d", dsn],
                          stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True,
                          env={**os.environ, "PGCONNECT_TIMEOUT": "5"}) as session:
        try:
            session.stdin.write("BEGIN;\nSET LOCAL statement_timeout='45s';\n" + setup + "\n" + probe + "\n")
            session.stdin.flush()
            result = session.stdout.readline().strip()
            require(bool(result) and session.poll() is None, "recovery transaction guard unavailable")
            yield result
            require(session.poll() is None, "recovery transaction guard disconnected")
        finally:
            session.stdin.close()


@contextmanager
def ingress_guard(dsn, scope, probe):
    # The exclusive counterpart in cutover_sql drains shared route readers and
    # deployment actions before setting or releasing the fence.
    scope_sql = q(scope)
    setup = f"DO $lock$ BEGIN PERFORM pg_advisory_xact_lock_shared(hashtextextended({scope_sql}::text,440054)); END; $lock$;"
    with transaction_guard(dsn, setup, probe) as result:
        yield result


class OperatorDenied(ValueError):
    """The continuing authorization of a recovery HTTP operation ended."""


def operator_guard_sql(scope, authorization, *, lock=False):
    actor, permission, deadline = authorization
    return f"""(clock_timestamp()<to_timestamp({int(deadline)}) AND
       platform.recovery_operator_authorized({q(scope)},{q(actor)},{literal(permission)},{str(lock).lower()}))"""


def guarded_cutover(dsn, scope, lock_sql, work, authorize):
    check_connection(dsn)
    args = ["psql", "-X", "-qAtw", "-v", "ON_ERROR_STOP=1", "-d", dsn]
    with subprocess.Popen(args, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                          env={**os.environ,"PGCONNECT_TIMEOUT":"5"},text=True) as session:
        def statement(sql):
            try:
                session.stdin.write(sql + "\n")
                session.stdin.flush()
                result = session.stdout.readline()
            except BrokenPipeError:
                raise RuntimeError("recovery transaction aborted") from None
            if result == "":
                raise RuntimeError("recovery transaction aborted")
            return result.strip()

        def check():
            permitted = operator_guard_sql(scope,authorize(),lock=True)
            if statement(f"SELECT {permitted};") != "t":
                raise OperatorDenied("recovery operator lease ended")

        try:
            if statement(lock_sql + "\nSELECT 'locked';") != "locked":
                raise RuntimeError("recovery transaction not locked")
            check()
            if statement(work + "\nSELECT 'prepared';") != "prepared":
                raise RuntimeError("recovery transition not prepared")
            check()
            permitted = operator_guard_sql(scope,authorize(),lock=True)
            finish = f"""
              DO $final$ BEGIN IF NOT {permitted} THEN RAISE EXCEPTION 'operator lease ended'; END IF; END; $final$;
              COMMIT;
              SELECT 'committed';
            """
            try:
                marker = statement(finish)
            except RuntimeError as exc:
                raise OperatorDenied("recovery commit authorization unavailable") from exc
            if marker != "committed":
                raise OperatorDenied("recovery operator lease ended")
        finally:
            session.stdin.close()


def cutover_sql(dsn, plan, body, result, authorize=None):
    deadline = int(plan["expiresAt"])
    scope, job = q(plan["scope"]), q(plan["job"])
    lock_sql = f"""
BEGIN;
SET LOCAL lock_timeout='45s';
DO $lock$ BEGIN
    PERFORM pg_advisory_xact_lock(hashtextextended({scope}::text,440054));
    PERFORM 1 FROM platform.recovery_route WHERE scope_ref={scope} FOR UPDATE;
END; $lock$;
"""
    work = f"""
DO $work$
DECLARE route platform.recovery_route; run platform.recovery_run;
BEGIN
    IF clock_timestamp() >= to_timestamp({deadline}) THEN RAISE EXCEPTION 'approval expired'; END IF;
    SELECT * INTO STRICT route FROM platform.recovery_route WHERE scope_ref={scope} FOR UPDATE;
    SELECT * INTO run FROM platform.recovery_run WHERE job_id={job};
    {body}
END;
$work$;
{result}
"""
    if authorize is not None:
        return guarded_cutover(dsn,plan["scope"],lock_sql,work,authorize)
    return db(dsn,lock_sql + work + "\nCOMMIT;").strip()


def preparation_guard(dsn,plan,spec,report,config,bundle):
    from preparation import check_report

    verified=check_report(dsn,plan,spec,report,config,bundle)
    return f"""
    IF EXISTS (SELECT 1 FROM platform.recovery_rehearsal WHERE job_id={q(plan['job'])}) AND
       NOT ({str(verified).lower()} AND EXISTS (
         SELECT 1 FROM platform.recovery_rehearsal WHERE job_id={q(plan['job'])}
           AND scope_ref={q(plan['scope'])} AND state='ready' AND report_digest={h(plan['evidence'])}
       )) THEN RAISE EXCEPTION 'signed rehearsal not ready'; END IF;
    """


def open_fence(dsn, bundle, report, config, *, authorize=None):
    plan, spec = validate_votes(bundle, config)
    verify_evidence(plan, spec, report)
    prepared=preparation_guard(dsn,plan,spec,report,config,bundle)
    scope, job, old = q(plan["scope"]), q(plan["job"]), h(plan["incumbent"])
    previous = q(plan["priorJob"]) if plan["priorJob"] else "NULL::uuid"
    manifest, plan_digest = h(plan["manifest"]), h(digest(plan))
    cutover_sql(dsn, plan, f"""
    {prepared}
    IF run.job_id IS NOT NULL THEN
       IF run.signed_plan <> {j(bundle)} OR run.state='superseded' THEN RAISE EXCEPTION 'work already used'; END IF;
       RETURN;
    END IF;
    IF route.revision <> {plan['routeRevision']} OR route.current_manifest <> {old} THEN
       RAISE EXCEPTION 'ingress revision changed';
    END IF;
    IF {previous} IS NOT NULL THEN
       IF NOT EXISTS (SELECT FROM platform.recovery_run parent
           WHERE parent.job_id={previous} AND parent.scope_ref={scope}
             AND parent.signed_plan->'plan'->>'incumbent'={literal(plan["manifest"])}
             AND ((parent.state='fenced' AND route.active_job={previous})
               OR (parent.state='complete' AND route.active_job IS NULL AND route.current_manifest=parent.manifest
                  AND route.revision=parent.route_revision+1))) THEN
          RAISE EXCEPTION 'rollback parent or previous delivery generation differs';
       END IF;
       UPDATE platform.recovery_run SET state='superseded' WHERE job_id={previous} AND state='fenced';
    ELSIF route.active_job IS NOT NULL THEN RAISE EXCEPTION 'another restore owns the ingress fence';
    END IF;
    INSERT INTO platform.recovery_run(job_id,scope_ref,prior_job,route_revision,manifest,plan_digest,signed_plan,expires_at,state)
        VALUES({job},{scope},{previous},route.revision,{manifest},{plan_digest},{j(bundle)},to_timestamp({plan['expiresAt']}),'fenced');
    UPDATE platform.recovery_route SET active_job={job} WHERE scope_ref={scope};""", "", authorize)


def receipt_payload(plan, target):
    old = plan["targets"][target]
    return {"domain": DOMAIN, "kind": "target-receipt", "target": target,
            "scope": plan["scope"], "job": plan["job"], "planDigest": digest(plan),
            "manifest": plan["manifest"], "evidence": plan["evidence"],
            "previousRevision": old, "appliedRevision": old + 1}


def target_apply(dsn, control_dsn, bundle, target, private, config):
    plan, spec = validate_votes(bundle, config, allow_expired=True)
    require(target in plan["targets"] and target in TARGETS, "target outside signed scope")
    scope, job = q(plan["scope"]), q(plan["job"])
    existing = db(dsn, f"""SELECT signed_receipt FROM platform.recovery_target_ack
      WHERE scope_ref={scope} AND target_id={literal(target)} AND job_id={job};""")
    if existing:
        receipt = json.loads(existing)
        verify_receipt(receipt, plan, spec)
        return receipt
    validate_plan(plan, config)
    # An adapter must use the coordinator's authenticated control plane and
    # observe its active fence. A stale plan cannot bypass target CAS.
    probe = f"""
      SELECT COALESCE((SELECT run.signed_plan::text FROM platform.recovery_route route
      JOIN platform.recovery_run run ON route.active_job=run.job_id
      WHERE route.scope_ref={scope} AND run.job_id={job} AND run.state='fenced'
        AND run.expires_at>clock_timestamp()),'null');
    """
    with ingress_guard(control_dsn, plan["scope"], probe) as witness:
        require(json.loads(witness) == bundle, "target did not observe its authorized active ingress fence")
        return target_cas(dsn, plan, spec, target, private)


def target_cas(dsn, plan, spec, target, private):
    scope, job = q(plan["scope"]), q(plan["job"])
    payload = receipt_payload(plan, target)
    receipt = {**payload, "signature": sign(payload, private)}
    verify(payload, receipt["signature"], Path(spec["targets"][target]))
    name, plan_digest = literal(target), h(digest(plan))
    result = db(dsn, f"""
BEGIN;
DO $target$
DECLARE route platform.recovery_target_route; previous platform.recovery_target_ack;
BEGIN
    SELECT * INTO STRICT route FROM platform.recovery_target_route WHERE scope_ref={scope} AND target_id={name} FOR UPDATE;
    SELECT * INTO previous FROM platform.recovery_target_ack WHERE scope_ref={scope} AND target_id={name} AND job_id={job};
    IF previous.job_id IS NOT NULL THEN
        IF previous.plan_digest <> {plan_digest} THEN RAISE EXCEPTION 'target operation ID conflicts'; END IF;
        RETURN;
    END IF;
    IF clock_timestamp() >= to_timestamp({plan['expiresAt']}) THEN RAISE EXCEPTION 'approval expired at target'; END IF;
    IF route.revision <> {payload['previousRevision']} THEN RAISE EXCEPTION 'target revision changed'; END IF;
    UPDATE platform.recovery_target_route SET revision=revision+1,manifest={h(plan['manifest'])}
        WHERE scope_ref={scope} AND target_id={name};
    INSERT INTO platform.recovery_target_ack VALUES({scope},{name},{job},{plan_digest},{j(receipt)});
END;
$target$;
SELECT signed_receipt FROM platform.recovery_target_ack WHERE scope_ref={scope} AND target_id={name} AND job_id={job};
COMMIT;
""")
    return json.loads(result)


def verify_receipt(receipt, plan, spec):
    target = receipt["target"]
    require(target in plan["targets"], "target outside signed scope")
    payload = receipt_payload(plan, target)
    require({key: value for key, value in receipt.items() if key != "signature"} == payload, "target receipt binding differs")
    verify(payload, receipt["signature"], Path(spec["targets"][target]))
    return target


def collect(dsn, bundle, receipt, config, *, authorize=None):
    plan, spec = validate_votes(bundle, config)
    target = literal(verify_receipt(receipt, plan, spec))
    job = q(plan["job"])
    cutover_sql(dsn, plan, f"""
    IF run.signed_plan IS DISTINCT FROM {j(bundle)} THEN RAISE EXCEPTION 'inactive recovery approvals'; END IF;
    IF run.state='complete' AND EXISTS (SELECT FROM platform.recovery_target_receipt WHERE job_id={job}
       AND target_id={target} AND signed_receipt={j(receipt)}) THEN RETURN; END IF;
    IF run.state <> 'fenced' OR route.active_job <> {job} THEN RAISE EXCEPTION 'inactive recovery approvals'; END IF;
    INSERT INTO platform.recovery_target_receipt VALUES({job},{target},{j(receipt)}) ON CONFLICT DO NOTHING;
    IF NOT EXISTS (SELECT FROM platform.recovery_target_receipt WHERE job_id={job} AND target_id={target}
        AND signed_receipt={j(receipt)}) THEN RAISE EXCEPTION 'conflicting target receipt'; END IF;
    """, "", authorize)


def finish(dsn, bundle, config, report, *, authorize=None):
    plan, spec = validate_votes(bundle, config)
    evidence = verify_evidence(plan, spec, report)
    prepared=preparation_guard(dsn,plan,spec,report,config,bundle)
    if "p09" not in plan["targets"]:
        return finish_checked(dsn, plan, bundle, evidence, authorize, prepared)
    with erasure_guard(spec, evidence["erasure"]):
        return finish_checked(dsn, plan, bundle, evidence, authorize, prepared)


@contextmanager
def erasure_guard(spec, result):
    from erasure import cursor

    namespace = q(result["namespace"])
    checkpoint = cursor(result["highWater"])
    dsn = spec.get("erasureGuardDatabase")
    require(dsn, "erasure writer barrier is required")
    with transaction_guard(dsn, "", f"SELECT travel_erasure.guard_restore({namespace},{checkpoint});") as matched:
        require(matched == "t", "erasure journal advanced after inspection")
        yield


def finish_checked(dsn, plan, bundle, evidence, authorize=None, prepared=""):
    scope, job = q(plan["scope"]), q(plan["job"])
    incident = format(Decimal(evidence["incidentEpoch"]), "f")
    elapsed = f"ceil(extract(epoch FROM clock_timestamp())-{incident}::numeric)"
    expected = "ARRAY[" + ",".join(literal(target) for target in sorted(plan["targets"])) + "]::text[]"
    targets = f"(SELECT array_agg(target_id ORDER BY target_id) FROM platform.recovery_target_receipt WHERE job_id={job})"
    cutover_sql(dsn, plan, f"""
    {prepared}
    IF run.signed_plan IS DISTINCT FROM {j(bundle)} THEN RAISE EXCEPTION 'inactive recovery approvals'; END IF;
    IF run.state='complete' THEN RETURN; END IF;
    IF run.state <> 'fenced' OR route.active_job <> {job} OR route.revision <> {plan['routeRevision']}
       OR {targets} IS DISTINCT FROM {expected} THEN RAISE EXCEPTION 'target receipts incomplete or ingress fence changed'; END IF;
    IF {elapsed} > {plan['goals']['rtoSeconds']} THEN RAISE EXCEPTION 'cutover exceeds enrolled RTO'; END IF;
    UPDATE platform.recovery_run SET state='complete',rto_seconds={elapsed},result_digest=(
       SELECT sha256(convert_to(jsonb_agg(signed_receipt ORDER BY target_id)::text,'UTF8'))
       FROM platform.recovery_target_receipt WHERE job_id={job}) WHERE job_id={job};
    UPDATE platform.recovery_route SET active_job=NULL,revision=revision+1,current_manifest=run.manifest WHERE scope_ref={scope};
    """, "", authorize)


def status(dsn, scope, job):
    return json.loads(db(dsn, f"""
SELECT jsonb_build_object('scope', route.scope_ref, 'ingressRevision', route.revision,
   'fenced', route.active_job IS NOT NULL, 'currentManifest', encode(route.current_manifest,'hex'),
   'activeJob', route.active_job, 'job', run.job_id, 'state', run.state, 'expiresAt', run.expires_at,
   'rtoSeconds', run.rto_seconds, 'resultDigest', encode(run.result_digest,'hex'), 'plan', run.signed_plan->'plan',
   'targets', COALESCE((SELECT jsonb_agg(signed_receipt ORDER BY target_id)
       FROM platform.recovery_target_receipt WHERE job_id=run.job_id),'[]'))
FROM platform.recovery_route route
LEFT JOIN platform.recovery_run run ON run.scope_ref=route.scope_ref AND run.job_id={q(job)}
WHERE route.scope_ref={q(scope)};
"""))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--policy", required=True, type=Path)
    parser.add_argument("--database", default=os.environ.get("P10_RECOVERY_DATABASE_URL"))
    commands = parser.add_subparsers(dest="command", required=True)
    voter = commands.add_parser("vote")
    voter.add_argument("--plan", required=True, type=Path)
    voter.add_argument("--role", required=True, choices=ROLES)
    voter.add_argument("--actor", required=True)
    voter.add_argument("--key", required=True, type=Path)
    voter.add_argument("--out", required=True, type=Path)
    started = commands.add_parser("fence")
    started.add_argument("--bundle", required=True, type=Path)
    started.add_argument("--report", required=True, type=Path)
    target = commands.add_parser("target-apply")
    target.add_argument("--bundle", required=True, type=Path)
    target.add_argument("--control-database", required=True)
    target.add_argument("--target", required=True, choices=TARGETS)
    target.add_argument("--key", required=True, type=Path)
    target.add_argument("--out", required=True, type=Path)
    ack = commands.add_parser("collect")
    ack.add_argument("--bundle", required=True, type=Path)
    ack.add_argument("--receipt", required=True, type=Path)
    done = commands.add_parser("finish")
    done.add_argument("--bundle", required=True, type=Path)
    done.add_argument("--report", required=True, type=Path)
    current = commands.add_parser("status")
    current.add_argument("--scope", required=True)
    current.add_argument("--job", required=True)
    args = parser.parse_args()
    config = read(args.policy)
    if args.command == "vote":
        plan = read(args.plan)
        validate_plan(plan, config)
        actor = uuid4_text(args.actor)
        principal = config["principals"][actor]
        require(args.role in principal["roles"] and plan["scope"] in principal["scopes"], "actor role or scope denied")
        payload = vote_payload(plan, args.role, actor)
        signature = sign(payload, args.key)
        verify(payload, signature, Path(principal["publicKey"]))
        save(args.out, {"actor": actor, "signature": signature})
        return
    require(args.database, "recovery database is required")
    if args.command == "status":
        require(args.scope in config["scopes"], "scope not enrolled")
        print(json.dumps(status(args.database, args.scope, args.job), sort_keys=True, indent=2))
        return
    bundle = read(args.bundle)
    if args.command == "fence":
        open_fence(args.database, bundle, args.report, config)
    elif args.command == "target-apply":
        save(args.out, target_apply(args.database, args.control_database, bundle, args.target, args.key, config))
    elif args.command == "collect":
        collect(args.database, bundle, read(args.receipt), config)
    else:
        finish(args.database, bundle, config, args.report)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, RuntimeError, KeyError, OSError, subprocess.TimeoutExpired) as exc:
        sys.exit(str(exc))

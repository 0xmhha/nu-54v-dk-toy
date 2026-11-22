"""Durable D06 work preparation and isolated restore worker."""

import argparse
import json
import os
from pathlib import Path
import re
import tempfile
import time
from uuid import uuid4

from cutover import OperatorDenied, db, digest, h, key_identity, literal, q, sign, text_hash, uuid4_text, verify
from recover import inspect_restore, read, save
from postgres import file_digest


def configured(config, scope, profile):
    if not re.fullmatch(r"[a-z][a-z0-9-]{0,39}", profile):
        raise OperatorDenied("unknown restore profile")
    enrollment=config.get("scopes",{}).get(scope,{}).get(profile)
    if not enrollment:
        raise OperatorDenied("unknown restore profile")
    required={"archive","backupKey","snapshot","rpc","synthetic","goalRef","goals"}
    if not required<=enrollment.keys() or type(enrollment["synthetic"]) is not bool:
        raise OperatorDenied("incomplete restore profile")
    text_hash(enrollment["snapshot"])
    uuid4_text(enrollment["goalRef"])
    goals=enrollment["goals"]
    if set(goals)!={"rpoSeconds","rtoSeconds"} or any(type(value) is not int or not 0<=value<=604800 for value in goals.values()):
        raise OperatorDenied("invalid restore profile goals")
    if enrollment.get("erasure"):
        text_hash(enrollment["erasureConfigDigest"])
    return enrollment


def request(dsn, scope, job, profile, fingerprint, authorize, *, stop=False):
    actor, role, until=authorize()
    if role!="requester":
        raise OperatorDenied("restore preparer permission required")
    # Recheck certificate and policy after waiting on route serialization.
    from cutover import guarded_cutover

    lock=f"""BEGIN;
SET LOCAL lock_timeout='45s';
DO $lock$ BEGIN PERFORM pg_advisory_xact_lock(hashtextextended({q(scope)}::text,440054)); END; $lock$;
"""
    op=f"""DO $request$ BEGIN IF NOT
      platform.request_recovery_rehearsal({q(scope)},{q(actor)},{q(job)},{literal(profile)},
            {h(fingerprint)},to_timestamp({int(until)}),{str(stop).lower()})
      THEN RAISE EXCEPTION 'rehearsal request differs'; END IF; END; $request$;"""
    # The cutover helper only guards this transaction's operator lease. The
    # request itself has no privileges to change ingress.
    guarded_cutover(dsn,scope,lock,op,authorize)


def details(dsn, scope, job=None):
    lookup=f" AND job_id={q(job)}" if job else ""
    return json.loads(db(dsn,f"""SELECT COALESCE(jsonb_agg(x),'[]') FROM
      (SELECT job_id::text AS job,profile_ref AS profile,state,stage,created_at AS "createdAt",updated_at AS "updatedAt",
          encode(report_digest,'hex') AS evidence,report_signature AS signature
       FROM platform.recovery_rehearsal WHERE scope_ref={q(scope)}{lookup} ORDER BY created_at DESC LIMIT 25) x;"""))


def check_report(dsn, plan, spec, path, config, bundle):
    runs=details(dsn,plan["scope"],plan["job"])
    if not runs:
        if spec.get("requireManagedRehearsal"):
            raise ValueError("signed rehearsal is required")
        return False
    row=runs[0]
    if row["state"]!="ready" or row["evidence"]!=plan["evidence"]:
        raise ValueError("signed rehearsal remains on hold")
    public=Path(spec["rehearsalReportKey"])
    actors={key_identity(Path(config["principals"][vote["actor"]]["publicKey"])) for vote in bundle["votes"].values()}
    targets={key_identity(Path(path)) for path in spec["targets"].values()}
    if key_identity(public) in actors | targets:
        raise ValueError("rehearsal report signer must be independent")
    verify({"job":plan["job"],"scope":plan["scope"],"report":file_digest(path)},row["signature"],public)
    return True


def consume(dsn, config, root, reports, report_key, postgres_bin=None):
    attempt=str(uuid4())
    work=json.loads(db(dsn,f"""WITH queued AS (
      SELECT job_id FROM platform.recovery_rehearsal WHERE state='queued' OR
        (state='running' AND lease_until<clock_timestamp())
      ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED
    ), claimed AS (
      UPDATE platform.recovery_rehearsal r SET state='running',stage='restore',attempt={q(attempt)},
        lease_until=clock_timestamp()+interval '10 minutes',updated_at=clock_timestamp()
      FROM queued WHERE r.job_id=queued.job_id
      RETURNING r.job_id::text AS job,r.scope_ref::text AS scope,r.profile_ref AS profile,encode(r.profile_digest,'hex') AS id
    ) SELECT COALESCE(jsonb_agg(claimed),'[]') FROM claimed;"""))
    if not work:
        return False
    job=work[0]
    identifier=q(job["job"])
    condition=f"job_id={identifier} AND attempt={q(attempt)} AND state='running' AND lease_until>clock_timestamp()"

    def checkpoint(stage):
        if db(dsn,f"""UPDATE platform.recovery_rehearsal SET stage={literal(stage)},
          updated_at=clock_timestamp(),lease_until=clock_timestamp()+interval '10 minutes'
          WHERE {condition} RETURNING 1;""")!="1":
            raise ValueError("rehearsal stopped or worker lease ended")

    try:
        profile=configured(config,job["scope"],job["profile"])
        if digest(profile)!=job["id"]:
            raise ValueError("rehearsal profile configuration changed")
        if file_digest(Path(profile["archive"])/"manifest.json")!=profile["snapshot"]:
            raise ValueError("enrolled snapshot manifest changed")
        if profile.get("erasure") and file_digest(Path(profile["erasure"]))!=profile["erasureConfigDigest"]:
            raise ValueError("enrolled erasure configuration changed")
        with tempfile.TemporaryDirectory(prefix="n54dr-",dir=root) as workdir:
            workdir=Path(workdir)
            temporary=workdir/"report.json"
            args=argparse.Namespace(scope=job["scope"],job=job["job"],report=temporary,
                isolation_root=workdir/"q",postgres_bin=postgres_bin,archive=Path(profile["archive"]),
                backup_key=Path(profile["backupKey"]),rpc=profile["rpc"],synthetic=profile["synthetic"],
                incident_epoch=profile.get("incidentEpoch"),erasure=Path(profile["erasure"]) if profile.get("erasure") else None,
                goal_ref=profile["goalRef"],rpo=profile["goals"]["rpoSeconds"],rto=profile["goals"]["rtoSeconds"])
            ready=inspect_restore(args,progress=checkpoint)==0
            result=read(temporary)
            if result["manifest"]!=profile["snapshot"]:
                raise ValueError("restore used a different snapshot")
            if "error" in result:
                result["error"]="restore-inspection-held"
            checkpoint("report")
            with tempfile.TemporaryDirectory(prefix=".prepare-",dir=reports) as publishing:
                staged=Path(publishing)/"report.json"
                fingerprint=save(staged,result)
                signature=sign({"job":job["job"],"scope":job["scope"],"report":fingerprint},report_key)
                report=reports/(fingerprint+".json")
                try:
                    os.link(staged,report)
                except FileExistsError:
                    if file_digest(report)!=fingerprint:
                        raise ValueError("rehearsal report storage differs")
            db(dsn,f"""UPDATE platform.recovery_rehearsal SET state={literal("ready" if ready else "hold")},
              updated_at=clock_timestamp(),report_digest={h(fingerprint)},report_signature={literal(signature)}
              WHERE {condition};""")
    except (ValueError,KeyError,OSError,RuntimeError,TypeError):
        db(dsn,f"""UPDATE platform.recovery_rehearsal SET state='hold',updated_at=clock_timestamp()
          WHERE {condition};""")
    return True


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--database",required=True)
    for name in ("config","isolation-root","reports","report-key","postgres-bin"):
        parser.add_argument("--"+name,type=Path,required=name!="postgres-bin")
    parser.add_argument("--watch",action="store_true",help="wait for additional scoped requests")
    args=parser.parse_args()
    while True:
        processed=consume(args.database,read(args.config),args.isolation_root,args.reports,args.report_key,args.postgres_bin)
        if not args.watch:
            return
        if not processed:
            time.sleep(.5)


if __name__=="__main__":
    main()

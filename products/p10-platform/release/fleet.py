#!/usr/bin/env python3
"""Verify candidates and coordinate signed, scoped local release transitions."""

import argparse
import json
import os
from pathlib import Path
import sys
import time

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "recovery"))
from cutover import canonical, digest as body_digest, key_identity, sign, text_hash, uuid4_text, verify as signature
from evidence import digest, key_sha256, read, require, resolve, verify

ROLES = ("requester", "verifier", "publisher")
IOS_FIELDS = {"bundleId","bundleBuild","content","executable"}
ANDROID_FIELDS = {"package","versionCode","certificate","rollbackDataPolicy"}
DIMENSIONS = ("product", "os", "board", "chain", "wire", "sdk", "abi")
DOMAIN = "nu54-release-fleet-v1"
class Database:
    def __init__(self, dsn):
        try:
            import psycopg2
            from psycopg2.extras import RealDictCursor
        except ModuleNotFoundError as exc:
            raise RuntimeError("install the P10 release requirements to connect to PostgreSQL") from exc
        from recover import check_connection

        check_connection(dsn)
        # A plain database name uses the independently configured libpq socket.
        if "=" not in dsn and "://" not in dsn:
            self.connection = psycopg2.connect(dbname=dsn, connect_timeout=5, cursor_factory=RealDictCursor)
        else:
            self.connection = psycopg2.connect(dsn, connect_timeout=5, cursor_factory=RealDictCursor)
        self.connection.autocommit = True
        self.execute("SET search_path TO platform,pg_catalog")
        self.execute("SET statement_timeout TO '30s'")

    def execute(self, statement, params=()):
        cursor = self.connection.cursor()
        cursor.execute(statement, params)
        return cursor

    def close(self):
        self.connection.close()


def database(dsn):
    return Database(dsn)


def tx(db, action):
    db.connection.autocommit = False
    try:
        db.execute("SELECT pg_advisory_xact_lock(440060)")
        result = action()
        db.connection.commit()
        return result
    except BaseException:
        db.connection.rollback()
        raise
    finally:
        db.connection.autocommit = True


def packed(value):
    return canonical(value).decode()


def one(db, statement, params, message):
    row = db.execute(statement, params).fetchone()
    require(row is not None, message)
    return row


def candidate(db, reference):
    return one(db, "SELECT * FROM fleet_candidate WHERE id=%s", (text_hash(reference),), "unknown candidate")


def combinations(matrix, manifest, channel):
    entries = {}
    artifacts = {item["id"]: item for item in manifest["artifacts"]}
    for entry in matrix:
        fields = {"artifact", "dimensions", "mode", "counter"}
        if entry.get("mode") == "board-image":
            fields |= {"bootKey", "measurement"}
            text_hash(entry["bootKey"])
            text_hash(entry["measurement"])
        if entry.get("mode") == "ios-simulator":
            fields |= IOS_FIELDS
            if "profile" in entry:
                fields.add("profile")
                require(entry["profile"] in {"fresh-install-only","existing-profile"},
                        "unknown iOS profile support scope")
            if "profileIdentity" in entry:
                fields.add("profileIdentity")
                text_hash(entry["profileIdentity"])
            if "profileProof" in entry:
                fields.add("profileProof")
                require(entry.get("profile") == "existing-profile", "iOS continuity proof lacks existing-profile scope")
            text_hash(entry["content"])
            text_hash(entry["executable"])
            require(all(isinstance(entry[key],str) and 0 < len(entry[key]) <= 100 for key in ("bundleId","bundleBuild")),
                    "invalid iOS identity tuple")
        if entry.get("mode") == "android-emulator" and "profile" in entry:
            fields |= ANDROID_FIELDS | {"profile"}
            require(entry["profile"] in {"fresh-install-only","practice-profile","protected-profile-local","protected-wallet"},
                    "unknown Android profile support scope")
            require(isinstance(entry.get("package"),str) and type(entry.get("versionCode")) is int and entry["versionCode"]>0
                    and entry.get("rollbackDataPolicy") in {"retain","unverified"},"invalid Android APK compatibility tuple")
            text_hash(entry["certificate"])
            if "profileProof" in entry:
                fields.add("profileProof")
                require(entry["profile"] in {"practice-profile","protected-profile-local"},
                        "Android continuity proof lacks an eligible local profile scope")
        if "interop" in entry:
            from interop import expectation
            fields.add("interop")
            check=entry["interop"]
            require(set(check)=={"language","vectors"} and
                    {"c":"abi","typescript":"sdk","go":"abi","python":"sdk"}.get(check["language"])==entry["mode"],
                    "invalid signed SDK interoperability scope")
            expectation(check["language"],check["vectors"])
        require(set(entry) == fields, "unexpected compatibility fields")
        item, dims = entry["artifact"], entry["dimensions"]
        require(item in artifacts and set(dims) == set(DIMENSIONS)
                and all(type(value) in {str, int} and len(str(value)) <= 100 for value in dims.values()),
                "invalid compatibility combination")
        require(entry["mode"] in {"sdk", "abi", "service", "android-emulator", "ios-simulator", "board-image", "physical"},
                "unknown installation observation mode")
        require(channel != "official" or entry["mode"] in {"sdk", "abi", "service", "physical"},
                "official mobile and board targets require physical observations")
        if dims["product"] in {"P01", "P02", "P04"}:
            require(entry["mode"] in {"android-emulator", "ios-simulator", "board-image", "physical"},
                    "mobile or board combination requires matching installation evidence")
        require(type(entry["counter"]) is int and entry["counter"] >= 0, "invalid minimum counter")
        require(dims["wire"] in (1, 2) and (entry["mode"] != "board-image" or entry["counter"] > 0),
                "invalid wire or counted board combination")
        if item in entries:
            existing = entries[item]
            require(dims not in existing["combinations"] and (entry["mode"], entry["counter"]) ==
                    (existing["mode"], existing["counter"]) and all(entry.get(field)==existing.get(field)
                    for field in {"bootKey","measurement","profile","profileProof","profileIdentity","interop"} | IOS_FIELDS | ANDROID_FIELDS), "duplicate or conflicting artifact compatibility")
            existing["combinations"].append(dims)
        else:
            entries[item] = {**entry, "payload": artifacts[item]["payload"]["sha256"], "version": artifacts[item]["version"],
                             "target": artifacts[item]["target"], "evidence": artifacts[item]["evidence"],
                             "combinations": [dims]}
    require(set(entries) == set(artifacts), "every artifact requires an enrolled compatibility combination")
    return entries


def for_target(release, target):
    row = release["artifacts"][target["artifact"]]
    require(target["dimensions"] in row["combinations"], "release target compatibility differs")
    return {**row, "dimensions": target["dimensions"]}


def verification_summary(role, reference, proof, raw, payload):
    result={"wrapper":reference["sha256"],"report":proof["report"]["sha256"],
            "kind":proof["evidenceKind"],"payload":payload}
    if role=="sbom":
        result["components"]=len(raw["components"])
    else:
        result["passed"]=proof.get("passed") is True
    if role=="regression" and isinstance(raw.get("checks"),list):
        result["checks"]=len(raw["checks"])
    if role=="rescan" and isinstance(raw.get("report"),dict):
        inspected=raw["report"].get("Results",[])
        if isinstance(inspected,list):
            result["scanTargets"]=len(inspected)
            result["findings"]=sum(len(item.get("Vulnerabilities") or []) for item in inspected)
    if role=="artifactSigning" and proof.get("signatureFormat")=="nu54-detached-ed25519-v1":
        result.update(signature=raw["signature"]["sha256"],signer=raw["signer"])
    return result


def register(db, manifest, sig, pub, policy, matrix, matrix_sig):
    record = read(manifest)
    channel = record["channel"]
    verify(manifest, sig, pub, policy["releaseTrust"], channel)
    identifier, signer = digest(manifest), key_identity(pub)
    raw = matrix.read_bytes()
    from evidence import authenticate, parse
    require(authenticate(raw, matrix_sig, pub.read_bytes()) == signer, "compatibility signer differs")
    supplied = parse(raw)
    require(supplied["format"] == DOMAIN and supplied["manifest"] == identifier, "compatibility manifest differs")
    rows = combinations(supplied["combinations"], record, channel)
    # Persist only application-independent evidence references and aggregate SBOM
    # identities. The evidence bundle still needs its original trust check on install.
    dossier={}
    for row in rows.values():
        row["fixtureEvidence"] = False
        for role, ref in row["evidence"].items():
            proof = read(resolve(manifest.parent, ref))
            fixture = proof["evidenceKind"] == "synthetic"
            require(proof.get("report") and (not fixture or (channel == "custom" and policy.get("localFixtureEvidence") is True)),
                    f"fleet requires observed {role}")
            row["fixtureEvidence"] |= fixture
            if role in {"sbom","rescan","regression","artifactSigning"}:
                raw=read(resolve(manifest.parent,proof["report"]))
                dossier[(row["artifact"],role)]=verification_summary(role,ref,proof,raw,row["payload"])
        ref = read(resolve(manifest.parent, row["evidence"]["sbom"])).get("report")
        require(ref is not None, "candidate lacks actual SBOM")
        sbom = read(resolve(manifest.parent, ref))
        require(sbom.get("bomFormat") == "CycloneDX" and isinstance(sbom.get("components"), list), "invalid CycloneDX SBOM")
        components=set()
        for item in sbom["components"]:
            version=item.get("version")
            # Unversioned optional host adapters remain in the signed SBOM.
            # They cannot be matched as versions distributed by this artifact.
            if version is None and item.get("scope")=="optional":
                continue
            require(isinstance(version,str) and version,"versioned SBOM component lacks a version")
            components.add(((item.get("purl") or item["name"]).removesuffix("@"+version),version))
        row["components"]=sorted(components)
    products = {}
    for row in rows.values():
        product = row["dimensions"]["product"]
        require(product in {f"P{number:02}" for number in range(1, 11)}, "unknown product")
        row["buildRelease"] = body_digest({"manifest": identifier, "product": product})
        products.setdefault(product, []).append(row)
    metadata = packed({"channel": channel, "artifacts": rows, "source": record["sourceCommit"]})

    def put():
        for product, artifacts in products.items():
            reference = bytes.fromhex(artifacts[0]["buildRelease"])
            version = "/".join(sorted({row["version"] for row in artifacts}))
            refs = {kind: body_digest(sorted(row["evidence"][kind]["sha256"] for row in artifacts))
                    for kind in ("sbom", "provenance")}
            db.execute("""
              INSERT INTO build_release VALUES (%s,%s,%s,%s,%s,%s,%s,%s)
              ON CONFLICT (manifest_digest) DO NOTHING
            """, (reference, int(product[1:]), version, channel, bytes.fromhex(signer), bytes.fromhex(digest(matrix)),
                  bytes.fromhex(refs["sbom"]), bytes.fromhex(refs["provenance"])))
            for row in artifacts:
                for dims in row["combinations"]:
                    combination = bytes.fromhex(body_digest({"artifact": row["artifact"], "dimensions": dims}))
                    for kind in ("regression", "rescan"):
                        db.execute("""INSERT INTO test_evidence VALUES (%s,%s,%s,%s,true,clock_timestamp())
                            ON CONFLICT DO NOTHING""",
                            (reference, combination, kind, bytes.fromhex(row["evidence"][kind]["sha256"])))
        db.execute("INSERT INTO fleet_candidate VALUES (%s,%s,%s) ON CONFLICT DO NOTHING", (identifier, signer, metadata))
        require(candidate(db, identifier)["metadata"] == metadata, "candidate compatibility metadata changed")
        for (artifact,kind),evidence in dossier.items():
            db.execute("""INSERT INTO fleet_verification VALUES (%s,%s,%s,%s::jsonb)
                ON CONFLICT DO NOTHING""",(identifier,artifact,kind,packed(evidence)))
        return identifier
    return tx(db, put)


def revoked(db, scope, signer):
    return db.execute("SELECT 1 FROM fleet_revocation WHERE cohort=%s AND key=%s", (scope, signer)).fetchone() is not None


def active_candidate(db, scope, reference):
    row = candidate(db, reference)
    require(not revoked(db, scope, row["signer"]), "candidate release signer revoked in this cohort")
    return json.loads(row["metadata"])


def vote_payload(plan, role, actor):
    return {"domain": DOMAIN, "role": role, "actor": actor, "plan": body_digest(plan)}


def approvals(bundle, policy):
    plan = bundle["plan"]
    require(set(bundle) == {"plan", "votes"} and set(bundle["votes"]) == set(ROLES), "all release approvals required")
    fields = {"id", "action", "cohort", "revision", "candidate", "parent", "targets", "advisory", "public", "issued", "expires"}
    if plan.get("action") == "resume":
        fields |= {"supersedes"}
        require(uuid4_text(plan["supersedes"]) != plan["id"], "cannot renew the same release job")
    require(set(plan) == fields and plan["action"] in {"publish", "rollback", "revoke", "resume"}, "unknown release plan")
    uuid4_text(plan["id"])
    cohort = policy["cohorts"][uuid4_text(plan["cohort"])]
    require(cohort["channel"] in {"official", "custom"}, "unknown fleet channel")
    require(type(plan["public"]) is bool and (not plan["public"] or cohort.get("public") is True),
            "public release disclosure is outside cohort policy")
    now = int(time.time())
    require(type(plan["issued"]) is int and type(plan["expires"]) is int and
        plan["issued"] <= now < plan["expires"] <= plan["issued"] + cohort["approvalSeconds"],
        "release approval expired or outside policy")
    require(type(plan["revision"]) is int and 0 <= plan["revision"] < 2**63-1, "invalid fleet revision")
    text_hash(plan["candidate"])
    if plan["parent"] is not None:
        text_hash(plan["parent"])
    if plan["advisory"] is not None:
        require(set(plan["advisory"]) == {"id", "digest"}, "advisory approval is incomplete")
        uuid4_text(plan["advisory"]["id"])
        text_hash(plan["advisory"]["digest"])
    require(set(plan["targets"]) == (set() if plan["action"] == "revoke" else set(cohort["targets"]))
            and all(type(value) is int and 0 <= value < 2**63-1 for value in plan["targets"].values()),
            "target roster or revision differs")
    identities, owners, keys = set(), set(), set()
    for role, vote in bundle["votes"].items():
        require(set(vote) == {"actor", "signature"}, "invalid vote")
        actor = uuid4_text(vote["actor"])
        enrollment = policy["principals"][actor]
        require(role in enrollment["roles"] and plan["cohort"] in enrollment["cohorts"], "release voter is outside scope")
        require(all(enrollment[key] for key in ("issuer", "subject", "person")), "release voter lacks original identity")
        identities.add((enrollment["issuer"], enrollment["subject"]))
        owners.add(enrollment["person"])
        keys.add(signature(vote_payload(plan, role, actor), vote["signature"], Path(enrollment["key"])))
    target_keys = {key_identity(Path(target["key"])) for target in cohort["targets"].values()}
    require(len(identities) == len(owners) == len(keys) == len(ROLES)
            and len(target_keys) == len(cohort["targets"]) and keys.isdisjoint(target_keys), "release identities must be independent")
    return plan, cohort, keys


def advisory_delta(db, case):
    fields = {"id", "cohort", "component", "affected", "before", "candidate", "owner", "reviewBy"}
    require(set(case) in (fields, fields | {"reason"}), "invalid advisory fields")
    if "reason" in case:
        require(isinstance(case["reason"],str) and 0 < len(case["reason"].strip()) <= 512, "invalid advisory reason")
    uuid4_text(case["id"])
    uuid4_text(case["cohort"])
    prior, successor = candidate(db, case["before"]), candidate(db, case["candidate"])
    require(prior["id"] != successor["id"] and case["owner"] and type(case["reviewBy"]) is int, "invalid patch comparison")
    old, new = [json.loads(row["metadata"]) for row in (prior, successor)]
    require(old["channel"] == new["channel"], "patch crosses release channels")
    require(isinstance(case["affected"], list) and case["affected"], "missing affected versions")
    def get(items, bad_only):
        return {artifact: sorted({version for name, version in row["components"]
                if name == case["component"] and (not bad_only or version in case["affected"])})
                for artifact, row in items["artifacts"].items()}
    affected = {artifact: versions for artifact, versions in get(old, True).items() if versions}
    remaining = {artifact: versions for artifact, versions in get(new, True).items() if versions}
    versions = get(new, False)
    require(affected, "advisory does not match the incumbent SBOM")
    require(set(affected) <= set(new["artifacts"]), "patch omitted affected target artifacts")
    observed_rescans = all(new["artifacts"][artifact]["payload"] != old["artifacts"][artifact]["payload"] for artifact in affected)
    return {**case, "channel": old["channel"], "oldVersions": affected, "newVersions": versions, "remaining": remaining,
            "boardCandidate": any(row["mode"]=="board-image" for row in new["artifacts"].values()),
            "state": "review" if not remaining and observed_rescans and not case.get("reason") else "hold"}


def record_advisory(db, case, policy):
    def put():
        report = advisory_delta(db, case)
        scope = case["cohort"]
        require(policy["cohorts"][scope]["channel"] == report["channel"], "advisory cohort channel differs")
        db.execute("INSERT INTO fleet_cohort(id) VALUES (%s) ON CONFLICT DO NOTHING", (scope,))
        db.execute("INSERT INTO fleet_advisory(id,report,cohort) VALUES (%s,%s,%s) ON CONFLICT(id) DO NOTHING",
                   (case["id"], packed(report), scope))
        stored = one(db, "SELECT report FROM fleet_advisory WHERE id=%s", (case["id"],), "missing advisory")
        require(stored["report"] == packed(report), "advisory evidence is immutable")
        previous = json.loads(candidate(db, case["before"])["metadata"])["artifacts"]
        for artifact in report["oldVersions"]:
            db.execute("""INSERT INTO advisory_case
                (advisory_ref,manifest_digest,component_ref,owner_role,next_review_at,decision,evidence_digest)
                VALUES (%s,%s,%s,%s,to_timestamp(%s),%s,%s) ON CONFLICT DO NOTHING""",
                (case["id"],bytes.fromhex(previous[artifact]["buildRelease"]),case["component"],case["owner"],case["reviewBy"],
                "hold" if report["state"] == "hold" else "investigate",bytes.fromhex(body_digest(report))))
        return report
    return tx(db, put)


def require_live(db, plan):
    require(db.execute("SELECT clock_timestamp()<to_timestamp(%s) AS live",(plan["expires"],)).fetchone()["live"],
            "release approval expired")


def resume_predecessor(db, current, plan):
    require(current["pending"] == plan["supersedes"], "interrupted release is no longer pending")
    old = one(db,"SELECT bundle,result,superseded_by FROM fleet_operation WHERE id=%s AND cohort=%s",
              (current["pending"],plan["cohort"]),"missing interrupted release")
    require(old["result"] is None and old["superseded_by"] is None, "interrupted release already closed")
    previous = json.loads(old["bundle"])["plan"]
    entry = candidate(db,previous["candidate"])
    signer_revoked = revoked(db,plan["cohort"],entry["signer"])
    expired = not db.execute("SELECT clock_timestamp()<to_timestamp(%s) AS live",(previous["expires"],)).fetchone()["live"]
    require(expired or signer_revoked, "current pending approval remains available")
    require(signer_revoked or previous["candidate"] == plan["candidate"],
            "expired release may only renew the same candidate")
    if previous["advisory"] is not None:
        require(plan["advisory"] is not None and (signer_revoked or plan["advisory"] == previous["advisory"]),
                "interrupted advisory evidence must be renewed")
    require(previous["targets"] == plan["targets"], "cannot change interrupted target identities or revisions")
    return json.loads(entry["metadata"])


def board_transition(previous, installed):
    if previous["mode"] == "board-image":
        require(installed["mode"] == "board-image" and previous.get("bootKey") == installed.get("bootKey"),
                "board boot root would change without rotation evidence")
    require(installed["counter"] >= previous["counter"], "board security counter would decrease")


def installation_transition(previous, successor):
    if previous["mode"] == "ios-simulator":
        require(successor["mode"] == "ios-simulator" and previous.get("bundleId")==successor.get("bundleId"),
                "iOS simulator installation identity would change")
    if previous["mode"]=="android-emulator" and "package" in previous:
        require(successor["mode"]=="android-emulator" and all(previous.get(key)==successor.get(key)
                for key in ("package","certificate")),"Android package or signing identity would change")
    board_transition(previous,successor)


def profile_gate(db, plan, spec, policy, selected, keys, interrupted=None, policy_source=None):
    from ios_profile import check as ios_check
    from android_profile import check as android_check, check_protected

    if not any(row["mode"] in {"ios-simulator","android-emulator"} for row in selected["artifacts"].values()):
        return
    if policy_source is not None:
        policy = policy_source()
        latest_spec = policy.get("cohorts", {}).get(plan["cohort"])
        require(latest_spec == spec, "mobile profile cohort enrollment changed")
    if interrupted is None and plan["action"] == "resume":
        older = one(db, "SELECT bundle FROM fleet_operation WHERE id=%s AND cohort=%s AND superseded_by=%s",
                    (plan["supersedes"],plan["cohort"],plan["id"]),"missing bound interrupted release")
        previous_candidate = json.loads(older["bundle"])["plan"]["candidate"]
        interrupted = json.loads(candidate(db,previous_candidate)["metadata"])
    current = candidate(db, plan["parent"]) if plan["parent"] else None
    previous_release = json.loads(current["metadata"]) if current else None
    candidate_signer = candidate(db, plan["candidate"])["signer"]
    excluded = keys | {candidate_signer} | {key_identity(Path(target["key"])) for target in spec["targets"].values()}
    if current:
        excluded.add(current["signer"])
    now = int(db.execute("SELECT extract(epoch FROM clock_timestamp()) AS n").fetchone()["n"])
    deadlines = []
    def verify_proof(check, wrapped, source, target):
        expires, verifier = check(wrapped,source,target,policy,now,excluded)
        require(not revoked(db,plan["cohort"],verifier),"existing-profile hold: continuity verifier revoked")
        deadlines.append(expires)

    for target in spec["targets"].values():
        row = for_target(selected,target)
        mode=row["mode"]
        if mode not in {"ios-simulator","android-emulator"}:
            continue
        previous = for_target(previous_release,target) if previous_release else None
        pending = for_target(interrupted,target) if interrupted else None
        if mode=="android-emulator":
            profiles=(target.get("androidProfile","fresh-install-only"),row.get("profile","fresh-install-only"),
                      (previous or {}).get("profile","fresh-install-only"),(pending or {}).get("profile","fresh-install-only"))
            require(set(profiles) <= {"fresh-install-only","practice-profile","protected-profile-local","protected-wallet"},
                    "unknown Android profile enrollment")
            require("protected-wallet" not in profiles,"Android protected-wallet rollback on hold: security-state continuity unverified")
            scope="protected-profile-local" if "protected-profile-local" in profiles else "practice-profile"
            if scope in profiles:
                require(previous and row["dimensions"]["os"]=="android-api-34","Android profile requires enrolled API 34 source")
                check=check_protected if scope=="protected-profile-local" else android_check
                if plan["action"]=="rollback":
                    require(previous.get("profile")==scope,"Android rollback signed scope differs")
                    verify_proof(check,previous.get("profileProof"),row,previous)
                else:
                    require(row.get("profile")==scope,"Android existing-profile hold: signed scope absent")
                    verify_proof(check,row.get("profileProof"),previous,row)
                    if pending and pending["payload"]!=row["payload"]:
                        verify_proof(check,row.get("profileProof"),pending,row)
            continue
        profiles = (target.get("iosProfile","fresh-install-only"),row.get("profile","fresh-install-only"))
        require(set(profiles) <= {"fresh-install-only","existing-profile"}, "unknown iOS profile enrollment")
        if "existing-profile" not in (*profiles, (previous or {}).get("profile"), (pending or {}).get("profile")):
            continue
        require(previous is not None,"iOS existing-profile hold: source artifact missing")
        if plan["action"] == "rollback":
            verify_proof(ios_check,previous.get("profileProof"),row,previous)
        else:
            require(row.get("profile") == "existing-profile","iOS existing-profile hold: signed scope absent")
            verify_proof(ios_check,row.get("profileProof"),previous,row)
            if pending and pending["payload"] != row["payload"]:
                verify_proof(ios_check,row.get("profileProof"),pending,row)
    if deadlines:
        require(db.execute("SELECT clock_timestamp()<to_timestamp(%s) AS live",
                           (min(deadlines),)).fetchone()["live"],"existing-profile hold: continuity evidence stale")


def require_candidate_unheld(db, plan):
    held=db.execute("""SELECT 1 FROM fleet_advisory_hold h JOIN fleet_advisory a ON a.id=h.advisory
                       WHERE a.cohort=%s AND a.report::jsonb->>'candidate'=%s LIMIT 1""",
                    (plan["cohort"],plan["candidate"])).fetchone()
    require(held is None,"candidate placed on scoped review hold")


def begin(db, bundle, policy, *, policy_source=None):
    plan, spec, key_ids = approvals(bundle, policy)
    def apply():
        require_live(db,plan)
        db.execute("INSERT INTO fleet_cohort(id) VALUES (%s) ON CONFLICT DO NOTHING", (plan["cohort"],))
        current = one(db, "SELECT * FROM fleet_cohort WHERE id=%s", (plan["cohort"],), "unknown cohort")
        prior = db.execute("SELECT * FROM fleet_operation WHERE id=%s", (plan["id"],)).fetchone()
        if prior:
            require(prior["bundle"] == packed(bundle), "release operation replay differs")
            return
        require(current["revision"] == plan["revision"] and current["current"] == plan["parent"], "stale fleet revision")
        if plan["action"] == "revoke":
            require(plan["candidate"] not in key_ids, "release signer cannot approve its own revocation")
            issuers=(policy.get(platform,{}).get(plan["candidate"],{}) for platform in
                     ("iosProfileVerifiers","androidProfileVerifiers"))
            profile_key = spec["channel"] == "custom" and any(
                target.get("package") in issuer.get("packages", ())
                and target["dimensions"]["os"] in issuer.get("os", ())
                for issuer in issuers for target in spec["targets"].values())
            require(plan["candidate"] in policy["releaseTrust"]["channels"][spec["channel"]] or profile_key,
                    "revocation key was never trusted in this cohort")
            db.execute("INSERT INTO fleet_revocation VALUES (%s,%s,%s) ON CONFLICT DO NOTHING", (plan["cohort"], plan["candidate"], packed(bundle)))
            db.execute("INSERT INTO fleet_operation(id,cohort,bundle,result) VALUES (%s,%s,%s,%s)",
                       (plan["id"], plan["cohort"], packed(bundle), body_digest(bundle)))
            require_live(db,plan)
            return
        interrupted = None
        if plan["action"] == "resume":
            interrupted = resume_predecessor(db,current,plan)
        else:
            require(current["pending"] is None, "cohort already has a pending release")
        selected = active_candidate(db, plan["cohort"], plan["candidate"])
        require_candidate_unheld(db,plan)
        require(selected["channel"] == spec["channel"], "fleet channel differs")
        target_keys = {key_identity(Path(target["key"])) for target in spec["targets"].values()}
        require(candidate(db, plan["candidate"])["signer"] not in key_ids | target_keys,
                "release signer must be independent from approvers and target observers")
        if plan["action"] == "rollback":
            previous = db.execute("SELECT * FROM fleet_operation WHERE cohort=%s AND result IS NOT NULL", (plan["cohort"],))
            require(any(json.loads(row["bundle"])["plan"]["candidate"] == plan["candidate"] and
                        json.loads(row["bundle"])["plan"]["action"] in {"publish","resume"} for row in previous),
                    "rollback candidate was never published to this cohort")
        if plan["advisory"]:
            patch = json.loads(one(db, "SELECT report FROM fleet_advisory WHERE id=%s", (plan["advisory"]["id"],), "unknown advisory")["report"])
            require(body_digest(patch) == plan["advisory"]["digest"] and patch["cohort"] == plan["cohort"] and patch["candidate"] == plan["candidate"] and patch["before"] == plan["parent"] and
                    patch["reviewBy"] >= int(time.time()) and patch["state"] == "review", "patch still on hold")
        profile_gate(db,plan,spec,policy,selected,key_ids,interrupted,policy_source)
        ios_devices=set()
        android_devices=set()
        for name, expected in plan["targets"].items():
            db.execute("INSERT INTO fleet_target(cohort,name) VALUES (%s,%s) ON CONFLICT DO NOTHING", (plan["cohort"], name))
            target = one(db, "SELECT * FROM fleet_target WHERE cohort=%s AND name=%s", (plan["cohort"], name), "unknown target")
            require(target["revision"] == expected and target["current"] == plan["parent"], "stale target revision")
            artifact = spec["targets"][name]["artifact"]
            row = for_target(selected, spec["targets"][name])
            if row["mode"] == "board-image":
                key_path = Path(spec["targets"][name].get("boardKeys", {}).get(row["bootKey"], ""))
                require(key_path.is_file() and key_identity(key_path) == row["bootKey"], "board boot root is not independently enrolled")
            if row["mode"] == "ios-simulator":
                from observe import ios_device_digest

                target_spec=spec["targets"][name]
                identifier=ios_device_digest(target_spec.get("device",""))
                require(identifier not in ios_devices and target_spec.get("package")==row["bundleId"],
                        "iOS simulator targets must be distinct and match the signed bundle identifier")
                ios_devices.add(identifier)
            if row["mode"]=="android-emulator" and "package" in row:
                from observe import android_device_digest

                target_spec=spec["targets"][name]
                device=android_device_digest(target_spec.get("device",""))
                require(device not in android_devices and target_spec.get("package")==row["package"],
                        "Android targets must be distinct and match the signed APK package")
                android_devices.add(device)
            if current["current"]:
                previous = json.loads(candidate(db, current["current"])["metadata"])["artifacts"][artifact]
                installation_transition(previous,row)
            if interrupted is not None:
                # A partially installed board may have adopted the pending
                # counter even if the observation response was lost.
                installation_transition(for_target(interrupted,spec["targets"][name]),row)
        db.execute("INSERT INTO fleet_operation(id,cohort,bundle,result) VALUES (%s,%s,%s,NULL)", (plan["id"], plan["cohort"], packed(bundle)))
        if interrupted is not None:
            db.execute("UPDATE fleet_operation SET superseded_by=%s WHERE cohort=%s AND id=%s",
                       (plan["id"],plan["cohort"],plan["supersedes"]))
        db.execute("UPDATE fleet_cohort SET pending=%s WHERE id=%s", (plan["id"], plan["cohort"]))
        require_live(db,plan)
        profile_gate(db,plan,spec,policy,selected,key_ids,interrupted,policy_source)
    return tx(db, apply)


def receipt_payload(plan, name, row, observation):
    result = {"domain": DOMAIN, "plan": body_digest(plan), "target": name, "candidate": plan["candidate"],
            "artifact": row["artifact"], "payload": row["payload"], "version": row["version"],
            "dimensions": row["dimensions"], "previous": plan["targets"][name], "applied": plan["targets"][name]+1,
            "mode": row["mode"], "counter": row["counter"], "observation": text_hash(observation)}
    if "interop" in row:
        from interop import expectation
        entry=row["interop"]
        result["interop"]=expectation(entry["language"],entry["vectors"])
    if row["mode"] == "board-image":
        result.update(bootKey=row["bootKey"],measurement=row["measurement"])
    if row["mode"]=="android-emulator" and "package" in row:
        result.update({field:row[field] for field in ANDROID_FIELDS | {"profile"}})
    if row["mode"] == "ios-simulator":
        result.update({field:row[field] for field in IOS_FIELDS})
        result.update({field:row[field] for field in ("profile","profileIdentity") if field in row})
    return result


def validate_ios_observation(row, enrolled, report):
    from observe import ios_device_digest

    details = report.get("details", {})
    runtime = details.get("runtime", "")
    prefix = "com.apple.CoreSimulator.SimRuntime."
    require(isinstance(runtime, str) and runtime.startswith(prefix) and
            runtime.removeprefix(prefix).lower() + "-simulator" == row["dimensions"]["os"],
            "iOS installed simulator runtime differs")
    expected = {"package": row["bundleId"], "runtime": runtime, "build": row["bundleBuild"],
                "content": row["content"], "executable": row["executable"],
                "deviceDigest": ios_device_digest(enrolled.get("device", ""))}
    if "profileIdentity" in row:
        expected["profileIdentity"]=row["profileIdentity"]
    require(details == expected, "iOS installed application evidence differs")


def android_installation_context(db, plan, enrolled, row):
    if row["mode"]!="android-emulator" or "package" not in row:
        return enrolled
    parent=plan["parent"]
    previous=None
    if parent:
        previous=for_target(json.loads(candidate(db,parent)["metadata"]),enrolled)
    operation="install"
    profiles={"practice-profile","protected-profile-local"}
    if row["profile"] in profiles and plan["action"]!="rollback":
        operation="enable-rollback"
    if previous and previous.get("profile") in profiles and plan["action"]=="rollback":
        operation="os-rollback"
    require(operation=="install" or previous is not None,"Android approved source artifact missing")
    return {**enrolled,"androidOperation":operation,"androidSource":previous}


def validate_android_observation(db, plan, enrolled, row, report):
    from observe import android_device_digest

    context=android_installation_context(db,plan,enrolled,row)
    expected={"package":row["package"],"installed":row["payload"],"api":row["dimensions"]["os"].removeprefix("android-api-"),
              "certificate":row["certificate"],"versionCode":row["versionCode"],"rollbackDataPolicy":row["rollbackDataPolicy"],
              "operation":context["androidOperation"],"deviceDigest":android_device_digest(enrolled["device"])}
    require(report.get("details")==expected,"installed Android APK evidence differs")


def record_receipt(db, bundle, wrapped, policy, *, policy_source=None):
    plan, spec, key_ids = approvals(bundle, policy)
    def put():
        require_live(db,plan)
        target = wrapped["receipt"]["target"]
        enrolled = spec["targets"][target]
        selected = active_candidate(db, plan["cohort"], plan["candidate"])
        require_candidate_unheld(db,plan)
        profile_gate(db,plan,spec,policy,selected,key_ids,policy_source=policy_source)
        row = for_target(selected, enrolled)
        report = wrapped["observation"]
        require(report["input"] == row["payload"] and report["mode"] == row["mode"] and report["version"] == row["version"]
            and report["observed"] is True, "installation evidence does not match enrolled target")
        if row["mode"] == "board-image":
            require(report.get("details") == {"slot":"host-file","application":row["payload"],
                    "measurement":row["measurement"],"counter":row["counter"],"bootKey":row["bootKey"]},
                    "board installed-slot evidence differs")
        if row["mode"] == "ios-simulator":
            validate_ios_observation(row, enrolled, report)
        if row["mode"]=="android-emulator" and "package" in row:
            validate_android_observation(db,plan,enrolled,row,report)
        if "interop" in row:
            from interop import expectation
            item=row["interop"]
            require(report.get("details",{}).get("interop")==expectation(item["language"],item["vectors"]),
                    "installed SDK interoperability observation differs")
        payload = receipt_payload(plan, target, row, body_digest(report))
        require(payload == wrapped["receipt"], "installation receipt body differs")
        signature(payload, wrapped["signature"], Path(enrolled["key"]))
        state = one(db, "SELECT * FROM fleet_operation WHERE id=%s", (plan["id"],), "unstarted release")
        require(state["bundle"] == packed(bundle) and state["superseded_by"] is None, "release operation is stale or superseded")
        previous = db.execute("SELECT claim FROM fleet_receipt WHERE job=%s AND target=%s", (plan["id"], target)).fetchone()
        if previous:
            require(previous["claim"] == packed(wrapped), "conflicting installation receipt")
            return
        require(state["result"] is None, "release already finished")
        pending=one(db,"SELECT pending FROM fleet_cohort WHERE id=%s",(plan["cohort"],),"unknown cohort")
        require(pending["pending"]==plan["id"],"release operation is no longer pending")
        db.execute("INSERT INTO fleet_receipt VALUES (%s,%s,%s)", (plan["id"], target, packed(wrapped)))
        require_live(db,plan)
        profile_gate(db,plan,spec,policy,selected,key_ids,policy_source=policy_source)
    return tx(db, put)


def finish(db, bundle, policy, *, policy_source=None):
    plan, spec, key_ids = approvals(bundle, policy)
    def publish():
        require_live(db,plan)
        state = one(db, "SELECT * FROM fleet_operation WHERE id=%s", (plan["id"],), "unstarted release")
        require(state["bundle"] == packed(bundle) and state["superseded_by"] is None, "active release evidence differs")
        if state["result"]:
            return state["result"]
        release = active_candidate(db, plan["cohort"], plan["candidate"])
        require_candidate_unheld(db,plan)
        profile_gate(db,plan,spec,policy,release,key_ids,policy_source=policy_source)
        receipts = db.execute("SELECT target,claim FROM fleet_receipt WHERE job=%s ORDER BY target", (plan["id"],)).fetchall()
        require({row["target"] for row in receipts} == set(plan["targets"]), "installation receipts incomplete")
        ios_devices = set()
        android_devices = set()
        for item in receipts:
            enrolled = spec["targets"][item["target"]]
            row = for_target(release, enrolled)
            if row["mode"] == "ios-simulator":
                claim = json.loads(item["claim"])
                validate_ios_observation(row, enrolled, claim["observation"])
                device = claim["observation"]["details"]["deviceDigest"]
                require(device not in ios_devices, "iOS simulator targets must be distinct")
                ios_devices.add(device)
            if row["mode"]=="android-emulator" and "package" in row:
                observation=json.loads(item["claim"])["observation"]
                validate_android_observation(db,plan,enrolled,row,observation)
                device=observation["details"]["deviceDigest"]
                require(device not in android_devices,"Android emulator targets must be distinct")
                android_devices.add(device)
        action = db.execute("UPDATE fleet_cohort SET current=%s,revision=revision+1,pending=NULL,active_job=%s,catalog_public=%s WHERE id=%s AND revision=%s AND pending=%s",
                            (plan["candidate"], plan["id"], plan["public"], plan["cohort"], plan["revision"], plan["id"]))
        require(action.rowcount == 1, "release pointer changed")
        for name, rev in plan["targets"].items():
            update = db.execute("UPDATE fleet_target SET current=%s,revision=revision+1 WHERE cohort=%s AND name=%s AND revision=%s",
                                (plan["candidate"], plan["cohort"], name, rev))
            require(update.rowcount == 1, "installation target pointer changed")
        result = body_digest([json.loads(item["claim"]) for item in receipts])
        for item in receipts:
            signed = json.loads(item["claim"])
            installed = signed["receipt"]
            artifact = for_target(release, spec["targets"][item["target"]])
            build = bytes.fromhex(artifact["buildRelease"])
            combination = bytes.fromhex(body_digest({"cohort": plan["cohort"], "target": item["target"],
                                                     "dimensions": artifact["dimensions"]}))
            report = bytes.fromhex(body_digest(signed))
            test_kind = "physical" if artifact["mode"] == "physical" else "install"
            db.execute("""INSERT INTO test_evidence VALUES (%s,%s,%s,%s,true,clock_timestamp())
                ON CONFLICT DO NOTHING""", (build, combination, test_kind, report))
            db.execute("""INSERT INTO promotion VALUES (%s,%s,%s,%s,%s,%s,clock_timestamp())
                ON CONFLICT DO NOTHING""", (build, combination, bytes.fromhex(body_digest(artifact["evidence"])),
                bundle["votes"]["requester"]["actor"], bundle["votes"]["publisher"]["actor"], report))
        if plan["advisory"]:
            patch = json.loads(one(db, "SELECT report FROM fleet_advisory WHERE id=%s",
                                   (plan["advisory"]["id"],), "unknown advisory")["report"])
            db.execute("UPDATE fleet_advisory SET published_job=%s WHERE id=%s AND cohort=%s",
                       (plan["id"], patch["id"], plan["cohort"]))
            old = json.loads(candidate(db, patch["before"])["metadata"])["artifacts"]
            for name in patch["oldVersions"]:
                db.execute("""UPDATE advisory_case SET decision='fixed',fixed_manifest=%s,evidence_digest=%s
                    WHERE advisory_ref=%s AND manifest_digest=%s AND component_ref=%s""",
                    (bytes.fromhex(release["artifacts"][name]["buildRelease"]), bytes.fromhex(result),
                     patch["id"], bytes.fromhex(old[name]["buildRelease"]), patch["component"]))
        db.execute("UPDATE fleet_operation SET result=%s WHERE id=%s", (result, plan["id"]))
        require_live(db,plan)
        profile_gate(db,plan,spec,policy,release,key_ids,policy_source=policy_source)
        return result
    return tx(db, publish)


def pending_status(db, scope, pending):
    if pending is None:
        return None
    row=one(db,"SELECT bundle FROM fleet_operation WHERE cohort=%s AND id=%s",(scope,pending),"unknown pending release")
    plan=json.loads(row["bundle"])["plan"]
    signer=candidate(db,plan["candidate"])["signer"]
    live=db.execute("SELECT clock_timestamp()<to_timestamp(%s) AS live",(plan["expires"],)).fetchone()["live"]
    reason="revoked" if revoked(db,scope,signer) else "installing" if live else "expired"
    return {"job":pending,"state":reason,"candidate":plan["candidate"],"targets":sorted(plan["targets"])}


def matrix_status(db, policy, scope):
    spec = policy["cohorts"][uuid4_text(scope)]
    state = db.execute("SELECT * FROM fleet_cohort WHERE id=%s", (scope,)).fetchone()
    pending=pending_status(db,scope,state["pending"]) if state else None
    if state is None or state["current"] is None:
        return {"state": "pending" if pending else "unpublished", "pendingStatus":pending, "targets": []}
    entry = candidate(db, state["current"])
    meta = json.loads(entry["metadata"])
    blocked = revoked(db, scope, entry["signer"])
    receipts = {row["target"]: json.loads(row["claim"]) for row in
                db.execute("SELECT target,claim FROM fleet_receipt WHERE job=%s", (state["active_job"],))}
    rows = []
    for name, value in spec["targets"].items():
        row = for_target(meta, value)
        labels = {"sdk": "consumer-install", "abi": "abi-consumer", "service": "service-install", "physical": "physical"}
        support = labels.get(row["mode"], "local-observation")
        if row["fixtureEvidence"]:
            support = "fixture-evidence"
        stopped=pending and pending["state"] in {"expired","revoked"} and name in pending["targets"]
        visible_row={name:value for name,value in row.items() if name!="profileProof"}
        if visible_row["mode"] in {"ios-simulator","android-emulator"}:
            visible_row.setdefault("profile","fresh-install-only")
        rows.append({**visible_row, "name": name, "status": "hold" if stopped else "revoked" if blocked else support,
                     "installedReceipt": receipts.get(name)})
    return {"state": "revoked" if blocked else "published", "manifest": entry["id"], "channel": meta["channel"], "releaseSigner": entry["signer"],
            "revision": state["revision"], "pending": state["pending"], "pendingStatus":pending, "targets": rows}


def installation_receipt(db, bundle, target, private, manifest, signature_file, public, policy, *, policy_source=None):
    from observe import observe

    plan, spec, key_ids = approvals(bundle, policy)
    enrolled = spec["targets"][target]
    state = one(db, "SELECT * FROM fleet_cohort WHERE id=%s", (plan["cohort"],), "unstarted fleet")
    require(state["pending"] == plan["id"], "installation needs an active approved plan")
    release = active_candidate(db, plan["cohort"], plan["candidate"])
    require_candidate_unheld(db,plan)
    profile_gate(db,plan,spec,policy,release,key_ids,policy_source=policy_source)
    require(digest(manifest) == plan["candidate"], "installation candidate manifest differs")
    verify(manifest, signature_file, public, policy["releaseTrust"], release["channel"])
    signer = candidate(db, plan["candidate"])["signer"]
    require(key_identity(public) == signer, "installation release signer differs")
    row = for_target(release, enrolled)
    enrolled_key = Path(enrolled["key"])
    proof = {"domain": DOMAIN, "kind": "installer-key", "plan": body_digest(plan), "target": target}
    signature(proof, sign(proof, private), enrolled_key)
    artifact = next(item for item in read(manifest)["artifacts"] if item["id"] == row["artifact"])
    context=android_installation_context(db,plan,enrolled,row)
    observation = observe(resolve(manifest.parent, artifact["payload"]), row, context)
    require_live(db,plan)
    state = one(db, "SELECT pending FROM fleet_cohort WHERE id=%s", (plan["cohort"],), "unknown cohort")
    require(state["pending"] == plan["id"], "installation plan changed during observation")
    current_release=active_candidate(db,plan["cohort"],plan["candidate"])
    require_candidate_unheld(db,plan)
    profile_gate(db,plan,spec,policy,current_release,key_ids,policy_source=policy_source)
    receipt = receipt_payload(plan, target, row, body_digest(observation))
    return {"receipt": receipt, "observation": observation, "signature": sign(receipt, private)}


def main():
    from contextlib import closing
    from recover import save

    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--database", required=True)
    parser.add_argument("--policy", type=Path, required=True)
    commands = parser.add_subparsers(dest="action", required=True)
    def new(name, paths=(), flags=()):
        sub = commands.add_parser(name)
        for path in paths:
            sub.add_argument("--" + path, required=True, type=Path)
        for flag in flags:
            sub.add_argument("--" + flag, required=True)
        return sub
    new("register", ("manifest", "signature", "key", "matrix", "matrix-signature"))
    new("advisory", ("case",))
    vote = new("vote", ("plan", "key", "out"), ("actor", "role"))
    new("begin", ("bundle",))
    new("install", ("bundle", "key", "manifest", "signature", "release-key", "out"), ("target",))
    new("collect", ("bundle", "receipt"))
    new("finish", ("bundle",))
    new("status", flags=("cohort",))
    args = parser.parse_args()
    def current_policy():
        return read(args.policy)

    policy = current_policy()
    if args.action == "vote":
        plan = read(args.plan)
        require(args.role in ROLES, "unknown vote role")
        principal = policy["principals"][uuid4_text(args.actor)]
        require(args.role in principal["roles"] and plan["cohort"] in principal["cohorts"], "vote outside enrollment")
        payload = vote_payload(plan, args.role, args.actor)
        signed = sign(payload, args.key)
        signature(payload, signed, Path(principal["key"]))
        save(args.out, {"actor": args.actor, "signature": signed})
        return
    with closing(database(args.database)) as db:
        if args.action == "register":
            result = register(db, args.manifest, args.signature, args.key, policy, args.matrix, args.matrix_signature)
        elif args.action == "advisory":
            result = record_advisory(db, read(args.case), policy)
        elif args.action == "status":
            result = matrix_status(db, policy, args.cohort)
        else:
            bundle = read(args.bundle)
            if args.action == "install":
                save(args.out, installation_receipt(db, bundle, args.target, args.key, args.manifest,
                                                   args.signature, args.release_key, policy,policy_source=current_policy))
                return
            if args.action == "begin":
                result = begin(db, bundle, policy,policy_source=current_policy)
            elif args.action == "collect":
                result = record_receipt(db, bundle, read(args.receipt), policy,policy_source=current_policy)
            else:
                result = finish(db, bundle, policy,policy_source=current_policy)
        print(json.dumps(result, sort_keys=True))


if __name__ == "__main__":
    try:
        main()
    except (ValueError, RuntimeError, KeyError, OSError) as error:
        sys.exit(str(error))

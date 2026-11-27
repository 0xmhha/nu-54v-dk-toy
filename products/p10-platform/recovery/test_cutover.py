"""Exercise the recovery coordinator and isolated target CASs on real PostgreSQL."""

import argparse
import hashlib
from contextlib import ExitStack, contextmanager
from concurrent.futures import ThreadPoolExecutor
from copy import deepcopy
import json
import os
import socket
import ssl
from pathlib import Path
import subprocess
import sys
import tempfile
import time
from urllib.request import urlopen
from urllib.error import HTTPError
from uuid import uuid4

from cutover import (ROLES, collect, db, finish, h, open_fence, q, sign, status, target_apply, validate_votes, vote_payload)
from recover import save

HERE = Path(__file__).resolve().parent


def reject(function, *args):
    try:
        function(*args)
    except (ValueError, RuntimeError):
        return
    raise AssertionError(f"{function.__name__} accepted a forbidden transition")


def key(folder, name):
    private, public = folder / (name + ".pem"), folder / (name + ".pub")
    subprocess.run(["openssl", "genpkey", "-algorithm", "ED25519", "-out", str(private)], check=True)
    private.chmod(0o600)
    subprocess.run(["openssl", "pkey", "-in", str(private), "-pubout", "-out", str(public)], check=True)
    return private, str(public)


def signed(plan, signers):
    return {"plan": plan, "votes": {role: {"actor": actor, "signature": sign(vote_payload(plan, role, actor), key_file)}
        for role, (actor, key_file) in signers.items()}}


def new_work(folder, scope, incumbent, manifest, revision, targets, spec, parent=None):
    now = int(time.time())
    job = str(uuid4())
    report = folder / (job + ".json")
    evidence = save(report, {"format": "nu54-recovery-report-v1", "scope": scope, "job": job, "manifest": manifest,
                "status": "await-independent-verification", "goalRef": spec["goalRef"], "goals": spec["goals"],
                "rpoSeconds": 1, "validationSeconds": 1, "incidentEpoch": str(now),
                "ordinaryTrafficFenced": True, "synthetic": True, "reconciliation": {"matched": True}})
    return {"kind": "rollback" if parent else "cutover", "job": job, "priorJob": parent, "scope": scope,
        "manifest": manifest, "incumbent": incumbent, "routeRevision": revision, "targets": targets, "evidence": evidence,
        "goalRef": spec["goalRef"], "goals": spec["goals"], "issuedAt": now, "expiresAt": now + 120}, report


@contextmanager
def managed_opsd(binary, control, route_login, cohort, manifest):
    with socket.socket() as probe:
        probe.bind(("127.0.0.1", 0))
        port = probe.getsockname()[1]
    env = {k: v for k, v in os.environ.items() if not k.startswith("P05_")}
    dsn = f"dbname={control} host={env['PGHOST']} user={route_login} port={env.get('PGPORT', '5432')} sslmode=disable"
    process = subprocess.Popen([str(binary)], env={**env, "P05_HTTP_ADDR": f"127.0.0.1:{port}",
        "P05_RECOVERY_DATABASE_URL": dsn, "P05_RECOVERY_SCOPE": cohort, "P05_RECOVERY_MANIFEST": manifest},
        stdout=subprocess.DEVNULL)
    try:
        def query():
            try:
                with urlopen(f"http://127.0.0.1:{port}/healthz", timeout=3) as response:
                    return response.status
            except HTTPError as exc:
                return exc.code
        deadline = time.monotonic() + 9
        while True:
            try:
                query()
                break
            except OSError:
                if process.poll() is not None or time.monotonic() >= deadline:
                    raise
                time.sleep(0.1)
        yield query
    finally:
        process.terminate()
        process.wait(timeout=8)


def check_browser(cli, folder, profile, origin, path, code, content):
    session = "-s=n54dr-" + uuid4().hex[:12]
    config = folder / "browser.json"
    config.write_text(json.dumps({"browser": {"browserName": "chromium",
        "launchOptions": {"channel": os.environ.get("NU54_RECOVERY_BROWSER_CHANNEL", "chrome")},
        "contextOptions": {"ignoreHTTPSErrors": True, "viewport": {"width": 390, "height": 760},
            "clientCertificates": [{"origin": origin, "certPath": profile["cert"], "keyPath": profile["key"]}]}}}))

    def browser(*args, check=True):
        return subprocess.run([cli, session, *args], cwd=folder, check=check,
                              capture_output=True, text=True, timeout=25).stdout

    try:
        browser("open", "--config=" + str(config), origin + path)
        expected = "not_found" if code == 404 else next(label for label in
            ("독립 확인 대기", "서비스 입구 차단", "전환 완료") if label.encode() in content)
        script = f"""() => {{
            document.documentElement.style.fontSize = '200%';
            const fits = document.documentElement.scrollWidth <= innerWidth;
            return {{ status: document.body.innerText.includes({json.dumps(expected, ensure_ascii=False)}),
                       fits, width: document.documentElement.scrollWidth, viewport: innerWidth }};
        }}"""
        result = browser("eval", script)
        import re
        assert re.search(r'"?status"?\s*:\s*true', result), result
        assert re.search(r'"?fits"?\s*:\s*true', result), result
        print(f"Chrome recovery viewport 390px/200% PASS {path.split('/')[1]} status={code}", flush=True)
    finally:
        browser("close", check=False)


def blocked_request(admin, lock_sql, login, request, revoke):
    from fleet import database

    lock = database(admin)
    lock.connection.autocommit = False
    with ThreadPoolExecutor(max_workers=1) as callers:
        try:
            lock.execute(lock_sql)
            pending = callers.submit(request)
            until = time.monotonic()+6
            while db(admin,f"SELECT count(*) FROM pg_stat_activity WHERE usename='{login}' AND wait_event_type='Lock';") == "0":
                if pending.done() or time.monotonic()>until:
                    raise AssertionError("authorized request did not wait on the fixture database lock")
                time.sleep(.025)
            revoke()
        finally:
            lock.connection.rollback()
            lock.close()
        return pending.result(timeout=10)


@contextmanager
def operator_view(folder, control, config, actor, requester, scope):
    from test_erasure import get as authenticated_get, tls_fixtures

    browser_cli = os.environ.get("NU54_RECOVERY_BROWSER_CLI")
    tls, _, _ = tls_fixtures(folder, browser=bool(browser_cli))
    server, operator, outsider = tls["ordinary"], tls["restore"], tls["traveler"]
    writer = tls["mutation"]
    def fingerprint(profile):
        cert=ssl.PEM_cert_to_DER_cert(Path(profile["cert"]).read_text())
        return hashlib.sha256(cert).hexdigest()
    config["operatorCertificates"] = {fingerprint(operator):actor, fingerprint(writer):requester}
    for who in (actor,requester):
        config["principals"][who]["expiresAt"] = int(time.time())+300
    policy = folder / "policy.json"
    save(policy, config)
    reader_login, writer_login = ["n54operator_"+uuid4().hex[:12] for _ in range(2)]
    for login, role, who, perm in ((reader_login,"status",actor,"read"),(writer_login,"coordinator",requester,"requester")):
        db(control,f"""CREATE ROLE {login} LOGIN IN ROLE nu54_p10_recovery_{role};
            INSERT INTO platform.recovery_operator_enrollment VALUES ('{login}',{q(who)},{q(scope)},'{perm}',now()+interval '5 minutes');""")
    reader = f"dbname={control} user={reader_login}"
    writable = f"dbname={control} user={writer_login}"
    with socket.socket() as probe:
        probe.bind(("127.0.0.1", 0))
        port = probe.getsockname()[1]
    process = subprocess.Popen([sys.executable, str(HERE / "recovery_operator.py"), "--port", str(port),
        "--policy", str(policy), "--reports", str(folder), "--status-database", reader,"--control-database",writable,
        "--cert", server["cert"], "--key", server["key"], "--client-ca", server["ca"]])
    try:
        deadline = time.monotonic() + 8
        while True:
            try:
                with socket.create_connection(("127.0.0.1", port), timeout=.5):
                    break
            except OSError:
                if process.poll() is not None or time.monotonic() >= deadline:
                    raise
                time.sleep(.1)
        origin = f"https://127.0.0.1:{port}"
        def query(view, scope, job, denied=False, *, browser=True):
            path = f"/{view}/{scope}/{job}"
            profile = outsider if denied else operator
            code, content = authenticated_get(profile, origin, path)
            if browser and browser_cli and view in {"D06", "X05"}:
                check_browser(browser_cli, folder, profile, origin, path, code, content)
            return code, content

        def post(bundle, profile=None):
            plan=bundle["plan"]
            return authenticated_get(profile or writer,origin,f"/v1/recovery/{scope}/{plan['job']}/fence",{"bundle":bundle})

        yield query,post,policy,reader_login,writer_login
    finally:
        if process.poll() is None:
            process.terminate()
        process.wait(timeout=8)
        db(control,f"DROP ROLE {reader_login},{writer_login};")


def test(folder, control, target_dbs, route_login, programs):
    scope, other_scope = str(uuid4()), str(uuid4())
    original, candidate, invalid = "a" * 64, "b" * 64, "c" * 64
    targets = {target: key(folder, target) for target in target_dbs}
    actors = {role: (str(uuid4()), *key(folder, role)) for role in ROLES}
    signers = {role: (actor, private) for role, (actor, private, _) in actors.items()}
    spec = {"targets": {target: public for target, (_, public) in targets.items()},
            "goals": {"rtoSeconds": 600, "rpoSeconds": 60}, "goalRef": str(uuid4()),
            "synthetic": True, "maxApprovalSeconds": 180}
    config = {"scopes": {scope: spec, other_scope: spec},
              "principals": {actor: {"person": role, "issuer": "synthetic-idp", "subject": role,
                "publicKey": public, "roles": [role], "scopes": [scope, other_scope]}
                for role, (actor, _, public) in actors.items()}}
    for cohort in (scope, other_scope):
        db(control, f"INSERT INTO platform.recovery_route VALUES ({q(cohort)},0,NULL,{h(original)});")
        for target, database in target_dbs.items():
            db(database, f"INSERT INTO platform.recovery_target_route VALUES ({q(cohort)},'{target}',0,{h(original)});")
    executable = folder / "opsd"
    subprocess.run(["go", "build", "-o", str(executable), "./products/p05-operations-backoffice/cmd/opsd"],
                   cwd=HERE.parents[2], check=True)
    old_api = programs.enter_context(managed_opsd(executable, control, route_login, scope, original))
    candidate_api = programs.enter_context(managed_opsd(executable, control, route_login, scope, candidate))
    canary_control = programs.enter_context(managed_opsd(executable, control, route_login, other_scope, original))
    assert (old_api(), candidate_api(), canary_control()) == (200, 503, 200)
    plan, report = new_work(folder, scope, original, candidate, 0, {"p05": 0, "p07": 0}, spec)
    bundle = signed(plan, signers)
    spool = folder / scope
    spool.mkdir(mode=0o700)
    (spool / (plan["job"] + ".json")).write_bytes(report.read_bytes())
    (folder / (plan["evidence"] + ".json")).write_bytes(report.read_bytes())
    operator, http_fence, policy_path, reader_login, writer_login = programs.enter_context(
        operator_view(folder,control,config,actors["verifier"][0],actors["requester"][0],scope))
    code, content = operator("D06", scope, plan["job"])
    assert code == 200 and "독립 확인 대기".encode() in content
    assert operator("D06", scope, plan["job"], denied=True)[0] == 404
    tampered = deepcopy(bundle)
    tampered["plan"]["goals"] = {"rtoSeconds": 601, "rpoSeconds": 60}
    reject(open_fence, control, tampered, report, config)
    tampered = deepcopy(bundle)
    tampered["plan"]["targets"]["p07"] = 1
    reject(open_fence, control, tampered, report, config)
    coupled = deepcopy(config)
    coupled["principals"][actors["verifier"][0]]["subject"] = "requester"
    reject(validate_votes, bundle, coupled)
    coupled = deepcopy(config)
    coupled["principals"][actors["verifier"][0]]["person"] = "requester"
    reject(validate_votes, bundle, coupled)
    coupled = deepcopy(config)
    coupled["principals"][actors["verifier"][0]]["publicKey"] = actors["requester"][2]
    shared_key = signed(plan, {**signers, "verifier": (actors["verifier"][0], actors["requester"][1])})
    reject(validate_votes, shared_key, coupled)
    def lease(login,expiry):
        db(control,f"UPDATE platform.recovery_operator_enrollment SET expires_at={expiry} WHERE login_role='{login}';")

    def expire(login):
        lease(login,"now()+interval '150 milliseconds'")
        time.sleep(.2)

    code, hidden = blocked_request(control,"LOCK TABLE platform.recovery_run IN ACCESS EXCLUSIVE MODE",reader_login,
        lambda: operator("X05",scope,plan["job"],browser=False),lambda: expire(reader_login))
    assert code == 404 and b"await-independent-verification" not in hidden
    lease(reader_login,"now()+interval '5 minutes'")
    assert operator("X05",scope,plan["job"],browser=False)[0] == 200
    def revoke():
        dropped=deepcopy(config)
        dropped["operatorCertificates"].clear()
        policy_path.write_text(json.dumps(dropped))
    code, hidden = blocked_request(control,f"SELECT pg_advisory_xact_lock(hashtextextended({q(scope)}::text,440054))",
        writer_login,lambda: http_fence(bundle),revoke)
    assert code == 404 and b"accepted" not in hidden and status(control,scope,plan["job"])["job"] is None
    assert http_fence(bundle)[0] == 404
    policy_path.write_text(json.dumps(config))
    code, hidden = blocked_request(control,"LOCK TABLE platform.recovery_run IN ACCESS EXCLUSIVE MODE",
        writer_login,lambda: http_fence(bundle),revoke)
    assert code == 404 and b"accepted" not in hidden and status(control,scope,plan["job"])["job"] is None
    policy_path.write_text(json.dumps(config))
    code, hidden = blocked_request(control,f"SELECT pg_advisory_xact_lock(hashtextextended({q(scope)}::text,440054))",
        writer_login,lambda: http_fence(bundle),lambda: expire(writer_login))
    assert code == 404 and not status(control,scope,plan["job"])["fenced"]
    lease(writer_login,"now()+interval '5 minutes'")
    from test_erasure import short_client_chain

    short, leaf, expiry = short_client_chain(folder)
    short_fp = hashlib.sha256(ssl.PEM_cert_to_DER_cert(leaf.read_text())).hexdigest()
    temporary_operator = deepcopy(config)
    temporary_operator["operatorCertificates"][short_fp] = actors["requester"][0]
    policy_path.write_text(json.dumps(temporary_operator))
    code, hidden = blocked_request(control,f"SELECT pg_advisory_xact_lock(hashtextextended({q(scope)}::text,440054))",
        writer_login,lambda: http_fence(bundle,short),lambda: time.sleep(max(0,expiry-time.time()+.1)))
    assert code == 404 and not status(control,scope,plan["job"])["fenced"]
    policy_path.write_text(json.dumps(config))
    assert http_fence(bundle)[0] == 200
    print("PASS blocked D06/X05 DB and intermediate expiry, file revocation, zero transitions before renewed allow")
    open_fence(control, bundle, report, config)
    assert status(control, scope, plan["job"])["fenced"]
    assert not status(control, other_scope, plan["job"])["fenced"]
    assert (old_api(), candidate_api(), canary_control()) == (503, 503, 200)
    reject(finish, control, bundle, config, report)

    def apply(bundle, target):
        return target_apply(target_dbs[target], control, bundle, target, targets[target][0], config)

    ack = apply(bundle, "p05")
    assert ack == apply(bundle, "p05")
    invalid_receipt = {**ack, "appliedRevision": 19}
    reject(collect, control, bundle, invalid_receipt, config)
    collect(control, bundle, ack, config)
    collect(control, bundle, ack, config)
    reject(finish, control, bundle, config, report)
    assert status(control, scope, plan["job"])["fenced"]
    assert (old_api(), candidate_api()) == (503, 503)
    code, content = operator("v1/recovery", scope, plan["job"])
    assert code == 200 and len(json.loads(content)["route"]["targets"]) == 1
    assert operator("X05", scope, plan["job"])[0] == 200
    collect(control, bundle, apply(bundle, "p07"), config)
    finish(control, bundle, config, report)
    finish(control, bundle, config, report)
    assert apply(bundle, "p05") == ack
    collect(control, bundle, ack, config)
    current = status(control, scope, plan["job"])
    assert current["state"] == "complete" and not current["fenced"]
    assert current["ingressRevision"] == 1 and current["currentManifest"] == candidate and current["resultDigest"]
    assert (old_api(), candidate_api(), canary_control()) == (503, 200, 200)
    code, content = operator("D06", scope, plan["job"])
    assert code == 200 and "전환 완료".encode() in content

    delayed, delayed_report = new_work(folder, scope, original, invalid, 0, {"p05": 0, "p07": 0}, spec)
    stale = signed(delayed, signers)
    reject(open_fence, control, stale, delayed_report, config)
    over_age = {**delayed, "issuedAt": int(time.time()) - 150, "expiresAt": int(time.time()) - 1}
    reject(validate_votes, signed(over_age, signers), config)

    failed, failed_report = new_work(folder, scope, candidate, invalid, 1, {"p05": 1, "p07": 0}, spec)
    failing = signed(failed, signers)
    open_fence(control, failing, failed_report, config)
    collect(control, failing, apply(failing, "p05"), config)
    reject(apply, failing, "p07")
    reject(finish, control, failing, config, failed_report)
    rollback, rollback_report = new_work(folder, scope, candidate, candidate, 1, {"p05": 2, "p07": 1}, spec, failed["job"])
    undo = signed(rollback, signers)
    open_fence(control, undo, rollback_report, config)
    assert status(control, scope, failed["job"])["state"] == "superseded"
    reject(open_fence, control, failing, failed_report, config)
    for target in target_dbs:
        collect(control, undo, apply(undo, target), config)
    finish(control, undo, config, rollback_report)
    end = status(control, scope, rollback["job"])
    assert end["currentManifest"] == candidate and end["ingressRevision"] == 2 and not end["fenced"]
    assert (old_api(), candidate_api(), canary_control()) == (503, 200, 200)
    print(json.dumps({"control": current["state"], "cohortFence": "isolated", "partialAcks": "held",
                      "lostResponse": "same signed receipt", "staleApproval": "denied",
                      "rollback": end["state"], "measuredCutoverSeconds": current["rtoSeconds"]}, sort_keys=True))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--existing-postgres", action="store_true", required=True)
    parser.parse_args()
    # Inherit the strict runner's private PG socket, with random, dedicated databases.
    names = {key: f"n54dr_{key}_{uuid4().hex[:12]}" for key in ("control", "p05", "p07")}
    with tempfile.TemporaryDirectory(prefix="n54dr-ctl-") as directory:
        created = []
        route_login = "n54_gen_" + uuid4().hex[:12]
        provisioned = False
        try:
            for database in names.values():
                subprocess.run(["createdb", "-w", database], check=True)
                created.append(database)
                subprocess.run([sys.executable, str(HERE.parent / "database/migrate.py"), "p10"],
                               env={**os.environ, "P10_MIGRATION_DATABASE_URL": database}, check=True)
            db(names["control"], f"CREATE ROLE {route_login} LOGIN IN ROLE nu54_p10_recovery_route_only;")
            provisioned = True
            with ExitStack() as programs:
                test(Path(directory), names["control"], {key: names[key] for key in ("p05", "p07")}, route_login, programs)
        finally:
            for database in created:
                subprocess.run(["dropdb", "-w", database], check=True)
            if provisioned:
                db("postgres", f"DROP ROLE {route_login};")


if __name__ == "__main__":
    main()

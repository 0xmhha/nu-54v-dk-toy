"""Exercise independently signed local publication and real npm installations."""

from contextlib import closing, contextmanager
from copy import deepcopy
from pathlib import Path
import argparse
import hashlib
import json
import ssl
import os
import socket
import subprocess
import sys
import tempfile
import time
from urllib.request import urlopen
from urllib.error import HTTPError
from uuid import uuid4

from evidence import deterministic_tar, digest, openssl
from fleet import (DIMENSIONS, DOMAIN, ROLES, approvals, begin, body_digest, combinations, database, finish, installation_receipt,
                   key_identity, matrix_status, record_advisory, record_receipt, register, sign, vote_payload)
from fleet_portal import projection, render
from recover import save


def keys(root, name):
    private = root / (name + ".pem")
    public = root / (name + ".pub.pem")
    openssl(["genpkey", "-algorithm", "Ed25519", "-out", private])
    public.write_bytes(openssl(["pkey", "-in", private, "-pubout"]))
    return private, public


def detached(path, root):
    signature = path.with_suffix(".sig")
    signature.write_bytes(openssl(["pkeyutl", "-sign", "-rawin", "-inkey", root, "-in", path]))
    return signature


def reject(operation, *args):
    try:
        operation(*args)
    except (ValueError, KeyError):
        return
    raise AssertionError("forbidden fleet operation succeeded")


def bundle(root, name, key, version, dependency, dims):
    root = root / name
    root.mkdir()
    private, public = key
    pkg = root / "consumer.tgz"
    deterministic_tar(pkg, {"package/package.json":
        ('{"name":"@nu54/fixture","version":"' + version + '","type":"module","exports":"./index.js"}').encode(),
        "package/index.js": f"export const version = '{version}'\n".encode()})
    artifact = {"id": "fixture-sdk", "version": version, "target": "node", "payload": ref(pkg), "evidence": {}}
    proofs = {"sbom": {"bomFormat": "CycloneDX", "components":
        [{"name": "fixture", "purl": "pkg:npm/fixture@" + dependency, "version": dependency}]}}
    payload_signature = detached(pkg, private)
    pub_copy = root / "release.pub.pem"
    pub_copy.write_bytes(public.read_bytes())
    proofs["artifactSigning"] = {"format": "nu54-detached-ed25519-v1", "payload": ref(pkg), "publicKey": ref(pub_copy),
                                "signature": ref(payload_signature), "signer": key_identity(public)}
    source = "a" * 40
    for role in ("sbom", "provenance", "rescan", "regression", "artifactSigning"):
        report = root / (role + ".raw.json")
        save(report, proofs.get(role, {"testFixture": role}))
        record = root / (role + ".wrapped.json")
        claim = {"format": f"nu54-{role}-v1", "artifactSha256": digest(pkg), "evidenceKind": "synthetic", "report": ref(report)}
        if role == "provenance":
            claim |= {"sourceCommit": source, "sourceClean": True}
        if role in {"rescan", "regression", "artifactSigning"}:
            claim["passed"] = True
        if role == "artifactSigning":
            claim["signatureFormat"] = "nu54-detached-ed25519-v1"
        save(record, claim)
        artifact["evidence"][role] = ref(record)
    manifest = root / "release.json"
    save(manifest, {"format": "nu54-release-evidence-v1", "channel": "custom",
                    "sourceCommit": source, "sourceClean": True, "artifacts": [artifact]})
    matrix = root / "matrix.json"
    save(matrix, {"format": DOMAIN, "manifest": digest(manifest),
        "combinations": [{"artifact": artifact["id"], "dimensions": dims, "mode": "sdk", "counter": 0}]})
    return manifest, detached(manifest, private), public, matrix, detached(matrix, private)


def ref(path):
    return {"file": path.name, "sha256": digest(path)}


def exercise(root, db, service_dsn, catalog_dsn, review_dsn, admin_dsn):
    scope, other = (str(uuid4()) for _ in range(2))
    actors = {role: (str(uuid4()), *keys(root, role)) for role in ROLES}
    releases = {name: keys(root, name) for name in ("prior", "affected", "fixed")}
    installers = {name: keys(root, name) for name in ("consumer-a", "consumer-b", "consumer-c")}
    dims = dict(zip(DIMENSIONS, ("P10", "node", "host", "8283", 2, "typescript", "abi-2"), strict=True))
    cohort = {"channel": "custom", "public": True, "approvalSeconds": 600, "targets": {name: {
        "key": str(item[1]), "artifact": "fixture-sdk", "dimensions": dims, "package": "@nu54/fixture"}
        for name, item in installers.items()}}
    policy = {"localFixtureEvidence": True, "cohorts": {scope: cohort, other: cohort}, "principals": {
        actor: {"person": role, "issuer": "local-idp", "subject": role, "roles": [role], "cohorts": [scope], "key": str(pub)}
        for role, (actor, _, pub) in actors.items()}, "releaseTrust": {
        "format": "nu54-release-trust-v1",
        "channels": {"official": {}, "custom": {key_identity(pair[1]): ["fixture-sdk"] for pair in releases.values()}}}}
    inputs = {}
    for name, ver, dependency in (("prior", "1.0.0", "2"), ("affected", "1.0.1", "1"), ("fixed", "1.0.2", "2")):
        inputs[name] = bundle(root, name, releases[name], ver, dependency, dims)
        manifest, sig, pub, matrix, matrix_sig = inputs[name]
        assert register(db, manifest, sig, pub, policy, matrix, matrix_sig) == digest(manifest)
    manifest = {"artifacts": [{"id": "x", "version": "1", "target": "linux", "payload": {"sha256": "a" * 64}, "evidence": {}}]}
    row = {"artifact": "x", "dimensions": dims, "mode": "sdk", "counter": 0}
    alternative = {**row, "dimensions": {**dims, "os": "linux"}}
    assert len(combinations([row, alternative], manifest, "official")["x"]["combinations"]) == 2
    assert combinations([row], manifest, "official")["x"]["mode"] == "sdk"
    reject(combinations, [{**row, "mode": "android-emulator"}], manifest, "official")

    def plan(name, revision, parent=None, action="publish", advisory=None):
        now = int(time.time())
        digest_or_key = key_identity(releases[name][1]) if action == "revoke" else digest(inputs[name][0])
        payload = {"id": str(uuid4()), "action": action, "cohort": scope, "revision": revision, "candidate": digest_or_key,
            "parent": digest(inputs[parent][0]) if parent else None, "targets": {name: revision for name in installers} if action != "revoke" else {},
            "advisory": advisory, "public": True, "issued": now, "expires": now + 300}
        votes = {role: {"actor": actor, "signature": sign(vote_payload(payload, role, actor), private)}
                 for role, (actor, private, _) in actors.items()}
        return {"plan": payload, "votes": votes}

    def publish(name, signed):
        begin(db, signed, policy)
        first = None
        for target, (private, _) in installers.items():
            manifest, sig, pub, _, _ = inputs[name]
            receipt = installation_receipt(db, signed, target, private, manifest, sig, pub, policy)
            if not first:
                first = receipt
                tampered = deepcopy(receipt)
                tampered["observation"]["version"] = "9"
                reject(record_receipt, db, signed, tampered, policy)
                record_receipt(db, signed, receipt, policy)
                with closing(database(service_dsn)) as restarted:
                    record_receipt(restarted, signed, receipt, policy)
                    reject(finish, restarted, signed, policy)
            record_receipt(db, signed, receipt, policy)
        result = finish(db, signed, policy)
        assert result == finish(db, signed, policy)
        return result

    initial = plan("prior", 0)
    coupled = deepcopy(policy)
    coupled["principals"][actors["verifier"][0]]["subject"] = "requester"
    reject(approvals, initial, coupled)
    publish("prior", initial)
    publish("affected", plan("affected", 1, "prior"))
    case = {"id": str(uuid4()), "cohort": scope, "component": "pkg:npm/fixture", "affected": ["1"],
        "before": digest(inputs["affected"][0]), "candidate": digest(inputs["fixed"][0]),
        "owner": "synthetic-release-operator", "reviewBy": int(time.time()) + 300}
    patch = record_advisory(db, case, policy)
    own_hidden = {**case, "id": str(uuid4()), "owner": "private-unpublished-owner"}
    foreign = {**case, "id": str(uuid4()), "cohort": other, "owner": "private-other-cohort-owner"}
    record_advisory(db, own_hidden, policy)
    foreign_patch = record_advisory(db, foreign, policy)
    reject(begin, db, plan("fixed", 2, "affected",
           advisory={"id": foreign["id"], "digest": body_digest(foreign_patch)}), policy)
    assert patch["oldVersions"] == {"fixture-sdk": ["1"]} and patch["newVersions"] == {"fixture-sdk": ["2"]}
    assert patch["state"] == "review"
    publish("fixed", plan("fixed", 2, "affected", advisory={"id": case["id"], "digest": body_digest(patch)}))
    begin(db, plan("affected", 3, "fixed", action="revoke"), policy)
    reject(begin, db, plan("affected", 3, "fixed", action="rollback"), policy)
    assert matrix_status(db, policy, other)["state"] == "unpublished"
    status = matrix_status(db, policy, scope)
    assert all(row["status"] == "fixture-evidence" and row["installedReceipt"] for row in status["targets"])
    rollback = plan("prior", 3, "fixed", action="rollback")
    begin(db, rollback, policy)
    disclosure(root, db, policy, scope, other, case, own_hidden, foreign,
               rollback["plan"]["candidate"], catalog_dsn, review_dsn, admin_dsn)
    publish("prior", rollback)
    assert matrix_status(db, policy, scope)["revision"] == 4
    assert db.execute("SELECT count(*) AS n FROM build_release").fetchone()["n"] == 3
    assert db.execute("SELECT count(*) AS n FROM promotion").fetchone()["n"] == 9
    assert db.execute("SELECT count(*) AS n FROM advisory_case WHERE decision='fixed' AND fixed_manifest IS NOT NULL").fetchone()["n"] == 1
    print("PASS fleet PostgreSQL authority, actual SDK installs, independent receipts, patch, key revocation and rollback")
    return scope, other


@contextmanager
def portal_server(dsn, policy_file, flags=()):
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        port = listener.getsockname()[1]
    portal = Path(__file__).with_name("fleet_portal.py")
    process = subprocess.Popen([sys.executable, str(portal), "--database", dsn, "--policy", str(policy_file),
                                "--port", str(port), *map(str, flags)])
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
        yield f"{'https' if flags else 'http'}://127.0.0.1:{port}"
    finally:
        if process.poll() is None:
            process.terminate()
        process.wait(timeout=8)


def browser_layout(cli, folder, origin, scope, *, artifact="fixture-sdk", unsupported_wire="1", component="pkg:npm/fixture"):
    import re
    config = folder / "fleet-browser.json"
    config.write_text(json.dumps({"browser": {"browserName": "chromium",
        "launchOptions": {"channel": "chrome"},
        "contextOptions":{"viewport":{"width":390,"height":760}}}}))
    session = "-s=n54fleet-" + uuid4().hex[:12]

    def run(*args, check=True):
        return subprocess.run([cli,session,*args],cwd=folder,capture_output=True,text=True,check=check,timeout=25).stdout

    def snapshot():
        target = folder / "fleet-accessibility.yml"
        run("snapshot","--filename="+str(target))
        return target.read_text()

    def reference(tree,role,label):
        found = re.search(rf'{role} "{re.escape(label)}"[^\n]*?\[ref=([^]]+)\]',tree)
        assert found, (role,label,tree[:1500])
        return found[1]

    try:
        for i,page in enumerate(("D01","D02","D03","D04","D05")):
            url = f"{origin}/{page}?cohort={scope}"
            if i == 0:
                run("open","--config="+str(config),url)
            else:
                run("goto",url)
            measure = run("eval","() => { document.documentElement.style.fontSize='200%'; return { fits:document.documentElement.scrollWidth <= innerWidth, heading:document.querySelector('h1') !== null }; }")
            assert re.search(r'"?fits"?\s*:\s*true',measure) and re.search(r'"?heading"?\s*:\s*true',measure), measure
            print(f"Chrome fleet viewport 390px/200% PASS {page}")
        run("goto",f"{origin}/D01?cohort={scope}")
        tree=snapshot()
        assert "C1" in tree and "C9" in tree
        run("select",reference(tree,"combobox","계약"),"C2/")
        run("click",reference(snapshot(),"button","조회"))
        tree=snapshot()
        assert "pay.prepare" in tree and "C3/order.create" not in tree
        prepare=re.search(r'pay\.prepare[^\n]*?\[ref=([^]]+)\]',tree)
        if prepare:
            run("click",prepare[1])
            assert "payer" in snapshot()
        tree = snapshot()
        run("select", reference(tree, "combobox", "계약"), "C3/")
        run("select", reference(snapshot(), "combobox", "역할"), "terminal")
        run("click", reference(snapshot(), "button", "조회"))
        tree = snapshot()
        assert "C3/order.create" in tree and "C3/subscription.offers" not in tree
        for label in ("C3/order.create", "명령 멱등성과 결과 대사", "오류 코드와 복구"):
            detail = re.search(rf'{re.escape(label)}[^\n]*?\[ref=([^]]+)\]', snapshot())
            assert detail, label
            run("click", detail[1])
        tree = snapshot()
        assert all(item in tree for item in ("intentDigest", "REPLAY_CONFLICT", "UNKNOWN_RESULT",
                                             "reconcile-prior-receipt", "reconcile-before-new-authority"))
        run("click",reference(snapshot(),"link","SDK·ABI 호환표"))
        tree=snapshot()
        assert artifact in tree
        run("fill",reference(tree,"textbox","wire"),unsupported_wire)
        run("click",reference(snapshot(),"button","조합 조회"))
        tree=snapshot()
        assert "지원 상태는 보류" in tree and artifact not in tree
        run("click",reference(tree,"link","보안 공지·수정 릴리스"))
        tree=snapshot()
        assert component in tree and "공개 승인" in tree and "영향 버전" in tree and "수정 후보 버전" in tree
        assert "비공개 SBOM" not in tree and "private-other-cohort-owner" not in tree
        print("PASS Chrome D01 payment filter, D02 unsupported selection and D04 approved public advisory")
    finally:
        run("close",check=False)


def disclosure(root, db, policy, scope, other, case, hidden, foreign, pending, catalog_dsn, review_dsn, admin_dsn):
    from test_cutover import blocked_request
    from test_erasure import get, short_client_chain, tls_fixtures

    tls, _, _ = tls_fixtures(root)
    cert = ssl.PEM_cert_to_DER_cert(Path(tls["restore"]["cert"]).read_text())
    actor = str(uuid4())
    policy["operatorCertificates"] = {hashlib.sha256(cert).hexdigest(): actor}
    policy["principals"][actor] = {"roles": ["release-review"], "cohorts": [scope], "expiresAt": int(time.time())+300}
    policy_file = root / "fleet-policy.json"
    policy_file.write_text(json.dumps(policy))
    server = tls["ordinary"]
    review_flags = ["--review","--cert",server["cert"],"--key",server["key"],"--client-ca",server["ca"]]
    assert projection(db, "D01", "", {"family":"C3/"})["operations"]
    with closing(database(catalog_dsn)) as source:
        assert projection(source, "D04", scope, {})["advisories"] == [], "synthetic evidence escaped to public notice"
    # Emulate the observed-vs-fixture marking in the disposable seeded DB. This
    # only exercises disclosure projection, separate from artifact observation.
    with closing(database(admin_dsn)) as admin:
        admin.execute("INSERT INTO fleet_review_enrollment VALUES (%s,%s,now()+interval '5 minutes')",
                      (review_dsn.split("user=")[1],scope))
        admin.execute("""UPDATE fleet_candidate SET metadata=jsonb_set(metadata::jsonb,
            '{artifacts,fixture-sdk,fixtureEvidence}','false'::jsonb)::text WHERE id=%s""",(case["candidate"],))
    with closing(database(catalog_dsn)) as source:
        for table in ("fleet_candidate","fleet_cohort","fleet_receipt","fleet_advisory"):
            assert not source.execute("SELECT has_table_privilege(current_user,%s,'SELECT') AS ok",
                                      ("platform."+table,)).fetchone()["ok"]
        actual = projection(source, "D04", scope, {})["advisories"]
        assert actual == [{"id":case["id"],"component":case["component"],"affected":["1"],
                           "versions":{"fixture-sdk":["2"]},"manifest":case["candidate"],"state":"published"}]
        assert not projection(source, "D04", other, {})["advisories"]
        assert not projection(source, "D02", scope, {"wire":"1"})["targets"]
        result = projection(source, "D03", scope, {})["targets"]
        assert len(result) == 1 and result[0]["name"] == "fixture-sdk"
    with closing(database(review_dsn)) as reviewer:
        assert projection(reviewer,"D04",other,{},review=True) is None
    with portal_server(catalog_dsn, policy_file) as public, portal_server(review_dsn,policy_file,review_flags) as private:
        for page in ("D01","D02","D03","D04","D05"):
            with urlopen(f"{public}/{page}?cohort={scope}", timeout=4) as response:
                assert response.status == 200 and b"<html" in response.read()
        with urlopen(public+"/D01",timeout=4) as response:
            assert response.status == 200
        if cli := os.environ.get("NU54_FLEET_BROWSER_CLI"):
            browser_layout(cli,root,public,scope)
        url = f"/api/D04?cohort={scope}"
        with urlopen(public+url,timeout=4) as response:
            body = response.read()
            assert case["id"].encode() in body and b"owner" not in body and hidden["id"].encode() not in body
        code, body = get(tls["restore"], private, url)
        assert code == 200 and hidden["id"].encode() in body and foreign["id"].encode() not in body
        pending_url = f"/api/D03?cohort={scope}"
        with urlopen(public + pending_url, timeout=4) as response:
            assert pending.encode() not in response.read()
        code, body = get(tls["restore"], private, pending_url)
        assert code == 200 and json.loads(body)["pending"]["manifest"] == pending
        assert get(tls["restore"], private, f"/api/D04?cohort={other}")[0] == 404
        assert get(tls["traveler"], private, url)[0] == 404
        login = review_dsn.split("user=")[1]
        def expire_database():
            with closing(database(admin_dsn)) as admin:
                admin.execute("UPDATE fleet_review_enrollment SET expires_at=now()+interval '150ms' WHERE cohort=%s",(scope,))
            time.sleep(.2)
        query = lambda: get(tls["restore"], private, url)
        result = blocked_request(admin_dsn,"LOCK TABLE platform.fleet_advisory IN ACCESS EXCLUSIVE MODE",login,query,expire_database)
        assert result[0] == 404 and hidden["owner"].encode() not in result[1]
        with closing(database(admin_dsn)) as admin:
            admin.execute("UPDATE fleet_review_enrollment SET expires_at=now()+interval '5 minutes' WHERE cohort=%s",(scope,))
        assert query()[0] == 200
        def revoke_file():
            policy["operatorCertificates"].clear()
            policy_file.write_text(json.dumps(policy))
        result = blocked_request(admin_dsn,"LOCK TABLE platform.fleet_advisory IN ACCESS EXCLUSIVE MODE",login,query,revoke_file)
        assert result[0] == 404 and hidden["owner"].encode() not in result[1]
        assert query()[0] == 404
        policy["operatorCertificates"][hashlib.sha256(cert).hexdigest()] = actor
        policy_file.write_text(json.dumps(policy))
        short,leaf,expiry = short_client_chain(root)
        short_digest = hashlib.sha256(ssl.PEM_cert_to_DER_cert(leaf.read_text())).hexdigest()
        policy["operatorCertificates"][short_digest]=actor
        policy_file.write_text(json.dumps(policy))
        result = blocked_request(admin_dsn,"LOCK TABLE platform.fleet_advisory IN ACCESS EXCLUSIVE MODE",login,
            lambda: get(short,private,url),lambda: time.sleep(max(0,expiry-time.time()+.1)))
        assert result[0] == 404 and hidden["owner"].encode() not in result[1]
        assert query()[0] == 200
        print("PASS private D04 PostgreSQL wait crosses lease expiry, file revocation and intermediate expiry")
        policy["principals"][actor]["expiresAt"] = int(time.time())-1
        policy_file.write_text(json.dumps(policy))
        assert get(tls["restore"], private, url)[0] == 404
        policy["principals"][actor]["expiresAt"] = int(time.time())+300
        del policy["operatorCertificates"][hashlib.sha256(cert).hexdigest()]
        policy_file.write_text(json.dumps(policy))
        assert get(tls["restore"], private, url)[0] == 404
        policy["operatorCertificates"][hashlib.sha256(cert).hexdigest()] = actor
        policy_file.write_text(json.dumps(policy))
        with closing(database(admin_dsn)) as admin:
            admin.execute("UPDATE fleet_review_enrollment SET expires_at=now() WHERE cohort=%s",(scope,))
        assert get(tls["restore"], private, url)[0] == 404
    print("PASS D01 public and D04 minimal scoped disclosure, mTLS reviewer, expiry and revocation")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--existing-postgres", required=True, action="store_true")
    parser.parse_args()
    name = "n54fleet_" + uuid4().hex[:12]
    clone = name + "_clone"
    service, catalog, reviewer = name + "_service", name + "_catalog", name + "_review"
    subprocess.run(["createdb", "-w", name], check=True)
    roles, cloned = False, False
    try:
        migrate = Path(__file__).resolve().parents[1] / "database/migrate.py"
        subprocess.run([sys.executable, str(migrate), "p10"],
                       env={**os.environ, "P10_MIGRATION_DATABASE_URL": name}, check=True)
        with closing(database(name)) as admin:
            admin.execute(f"CREATE ROLE {service} LOGIN IN ROLE nu54_p10_fleet_service")
            admin.execute(f"CREATE ROLE {catalog} LOGIN IN ROLE nu54_p10_fleet_catalog")
            admin.execute(f"CREATE ROLE {reviewer} LOGIN IN ROLE nu54_p10_fleet_review")
            roles = True
        service_dsn, catalog_dsn = f"dbname={name} user={service}", f"dbname={name} user={catalog}"
        with tempfile.TemporaryDirectory(prefix="n54fleet-") as temporary:
            root = Path(temporary)
            with closing(database(service_dsn)) as connection:
                scope, other = exercise(root, connection, service_dsn, catalog_dsn, f"dbname={name} user={reviewer}", name)
            archive = root / "fleet-authority.pgdump"
            subprocess.run(["pg_dump", "-w", "-Fc", "-d", name, "-f", str(archive)], check=True)
            subprocess.run(["createdb", "-w", clone], check=True)
            cloned = True
            subprocess.run(["pg_restore", "-w", "--exit-on-error", "--single-transaction", "-d", clone, str(archive)], check=True)
            with closing(database(f"dbname={clone} user={service}")) as restored:
                assert restored.execute("SELECT count(*) AS n FROM promotion").fetchone()["n"] == 9
                assert restored.execute("SELECT count(*) AS n FROM fleet_receipt").fetchone()["n"] == 12
                assert restored.execute("SELECT count(*) AS n FROM fleet_revocation").fetchone()["n"] == 1
                state = restored.execute("SELECT revision,current,pending FROM fleet_cohort WHERE id=%s",(scope,)).fetchone()
                isolated = restored.execute("SELECT revision,current FROM fleet_cohort WHERE id=%s",(other,)).fetchone()
                assert isolated["revision"] == 0 and isolated["current"] is None
                assert state["revision"] == 4 and state["current"] and state["pending"] is None
            print("PASS PostgreSQL backup restore preserves promotions, signed receipts, revocation and rollback state")
    finally:
        if cloned:
            subprocess.run(["dropdb", "-w", clone], check=True)
        subprocess.run(["dropdb", "-w", name], check=True)
        if roles:
            with closing(database("postgres")) as admin:
                admin.execute(f"DROP ROLE {service}, {catalog}, {reviewer}")

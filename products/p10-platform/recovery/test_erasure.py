"""Rehearse pre-erasure P09 backup recovery across actual mTLS product processes."""

import argparse
from base64 import b64encode
from contextlib import ExitStack, contextmanager
from datetime import datetime, timedelta, timezone
import hashlib
import json
import os
from pathlib import Path
import pwd
import re
import socket
import ssl
import subprocess
import sys
import time
from urllib.error import HTTPError
from urllib.request import Request
from uuid import uuid4

from cutover import ROLES, collect, db, erasure_guard, finish, h, open_fence, q, status, target_apply
from recover import save
from erasure import apply_and_recheck
from postgres import Quarantine, pg
from reconcile import opener

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[2]
sys.path.insert(0, str(ROOT / "scripts"))
from integration_workspace import cleanup_stack, run_command, scoped_workspace, start_postgres, stop_postgres


CAPACITY_RETRY_DELAYS = (1, 2, 4, 8, 15)
SHM_CAPACITY_FAILURE = re.compile(
    r"FATAL:\s+could not create shared memory segment: No space left on device[^\S\n]*\n"
    r"[^\n]*DETAIL:\s+Failed system call was shmget\("
)


class ClusterStartupError(RuntimeError):
    def __init__(self, stage, diagnostic, returncode=1):
        super().__init__(f"Rehearsal PostgreSQL {stage} failed ({returncode})")
        self.diagnostic = diagnostic
        self.capacity_limited = (
            returncode > 0
            and len(re.findall(r"(?m)\b(?:FATAL|PANIC):", diagnostic)) == 1
            and SHM_CAPACITY_FAILURE.search(diagnostic) is not None
        )


class RehearsalQuarantine(Quarantine):
    def _initdb(self, args):
        # initdb and its bootstrap child run in an owned process group.
        self.environment["LC_ALL"] = "C"
        result = run_command(args, env=self.environment, capture_output=True,
                             text=True, errors="replace", timeout=60, termination_grace=3)
        if result.returncode:
            raise ClusterStartupError("initdb", result.stderr, result.returncode)
        print(result.stdout, end="", flush=True)
        if result.stderr:
            print(result.stderr, end="", file=sys.stderr, flush=True)

    def _start_server(self, options):
        log = self.root / "postgres.log"
        try:
            start_postgres("pg_ctl", self.data, log, options, self.environment)
        except subprocess.CalledProcessError as failure:
            diagnostic = log.read_text(errors="replace") if log.exists() else ""
            raise ClusterStartupError("pg_ctl", diagnostic, failure.returncode) from None


def run(args, **options):
    return subprocess.run(args, cwd=ROOT, check=True, **options)


def db_env(cluster, user):
    return {**cluster.env, "PGUSER": user}


def dsn(cluster, database, user):
    socket_dir = cluster.socket if cluster else os.environ["PGHOST"]
    return f"host={socket_dir} user={user} dbname={database} sslmode=disable"


def migrate(env, owner, database):
    run([sys.executable, str(HERE.parent / "database/migrate.py"), owner],
        env={**env, f"{owner.upper()}_MIGRATION_DATABASE_URL": database})


def connection(cluster, database, source, statement):
    env = db_env(cluster, source) if cluster else os.environ.copy()
    return pg(env, ["psql", "-X", "-qAtw", "-v", "ON_ERROR_STOP=1", "-d", database],
              input=statement, capture_output=True, text=True).stdout.strip()


def login(cluster, database, role, capability):
    connection(cluster, database, "recovery_operator", f"CREATE ROLE {role} LOGIN IN ROLE {capability};")
    os_user = pwd.getpwuid(os.getuid()).pw_name
    with (cluster.data / "pg_ident.conf").open("a") as config:
        config.write(f"nu54_recovery {os_user} {role}\n")
    with (cluster.data / "pg_hba.conf").open("a") as config:
        config.write(f"local {database} {role} peer map=nu54_recovery\n")
    pg(cluster.env, ["pg_ctl", "-D", str(cluster.data), "reload"])


@contextmanager
def isolated(root):
    for attempt, delay in enumerate((*CAPACITY_RETRY_DELAYS, None), 1):
        started = False
        try:
            with scoped_workspace(root.name + "-", directory=root.parent) as work, cleanup_stack() as cleanup:
                cluster = RehearsalQuarantine(work / "db", None)
                cleanup.callback(stop_postgres, "pg_ctl", cluster.data, cluster.environment)
                try:
                    cluster.start()
                except ClusterStartupError as failure:
                    print(f"{root.name} attempt {attempt}: {failure}", file=sys.stderr, flush=True)
                    if failure.diagnostic:
                        print(failure.diagnostic.rstrip(), file=sys.stderr, flush=True)
                    raise
                started = True
                yield cluster
                return
        except ClusterStartupError as failure:
            if started or not failure.capacity_limited or delay is None:
                raise
            print(f"{root.name}: PostgreSQL shmget capacity exhausted. Retrying in {delay}s.",
                  file=sys.stderr, flush=True)
            time.sleep(delay)


def tls_fixtures(folder, *, browser=False):
    def quiet(args):
        return run(["openssl", *map(str, args)], capture_output=True).stdout

    tls_algorithm = ["ec", "-pkeyopt", "ec_paramgen_curve:prime256v1"] if browser else ["ed25519"]
    ca, ca_key = folder / "ca.pem", folder / "ca-key"
    quiet(["req", "-x509", "-noenc", "-newkey", *tls_algorithm, "-days", "1", "-subj", "/CN=NU54 Rehearsal",
           "-keyout", ca_key, "-out", ca])
    identities = {}
    for index, label in enumerate(("journal", "workload", "ordinary", "restore", "traveler", "mutation"), 2):
        server = label in {"journal", "workload", "ordinary"}
        cert, key, request, ext = (folder / f"{label}{suffix}" for suffix in (".pem", ".key", ".csr", ".ext"))
        ext.write_text("extendedKeyUsage=serverAuth\nsubjectAltName=IP:127.0.0.1\n" if server else "extendedKeyUsage=clientAuth\n")
        quiet(["req", "-new", "-noenc", "-newkey", *tls_algorithm, "-subj", f"/CN={label}",
               "-keyout", key, "-out", request])
        quiet(["x509", "-req", "-in", request, "-CA", ca, "-CAkey", ca_key,
               "-set_serial", str(index), "-days", "1", "-extfile", ext, "-out", cert])
        identities[label] = {"cert": str(cert), "key": str(key), "ca": str(ca)}
    seed, verify_key = folder / "journal.seed", folder / "journal.pub"
    key_pem = folder / "journal-signing.pem"
    quiet(["genpkey", "-algorithm", "ED25519", "-out", key_pem])
    seed.write_bytes(b64encode(quiet(["pkey", "-in", key_pem, "-outform", "DER"])[-32:]))
    verify_key.write_bytes(b64encode(quiet(["pkey", "-in", key_pem, "-pubout", "-outform", "DER"])[-32:]))
    seed.chmod(0o600)
    return identities, seed, verify_key


def short_client_chain(folder):
    intermediate_key, intermediate_csr, intermediate = [folder / ("short-intermediate"+suffix) for suffix in (".key",".csr",".pem")]
    leaf_key, leaf_csr, leaf, chain = [folder / ("short-client"+suffix) for suffix in (".key",".csr",".pem",".chain")]
    for name, key, csr in (("recovery short intermediate",intermediate_key,intermediate_csr),("recovery client",leaf_key,leaf_csr)):
        run(["openssl","req","-new","-noenc","-newkey","ec","-pkeyopt","ec_paramgen_curve:prime256v1",
             "-subj",f"/CN={name}","-keyout",str(key),"-out",str(csr)],capture_output=True)
    (folder / "short-index").touch()
    (folder / "short-serial").write_text("70\n")
    config = folder / "short-ca.cnf"
    config.write_text(f"""[ca]
default_ca=signer
[signer]
database={folder / 'short-index'}
serial={folder / 'short-serial'}
new_certs_dir={folder}
certificate={folder / 'ca.pem'}
private_key={folder / 'ca-key'}
default_md=sha256
policy=names
x509_extensions=ca_ext
[names]
commonName=supplied
[ca_ext]
basicConstraints=critical,CA:true,pathlen:0
keyUsage=critical,keyCertSign,cRLSign
subjectKeyIdentifier=hash
authorityKeyIdentifier=keyid
""")
    now = datetime.now(timezone.utc)
    expiry = (now + timedelta(seconds=5)).replace(microsecond=0)
    stamp = "%Y%m%d%H%M%SZ"
    run(["openssl","ca","-config",str(config),"-batch","-notext","-startdate",
         (now-timedelta(minutes=1)).strftime(stamp),"-enddate",expiry.strftime(stamp),
         "-in",str(intermediate_csr),"-out",str(intermediate)],capture_output=True)
    ext = folder / "short-leaf-ext"
    ext.write_text("basicConstraints=CA:false\nextendedKeyUsage=clientAuth\n")
    run(["openssl","x509","-req","-in",str(leaf_csr),"-CA",str(intermediate),"-CAkey",str(intermediate_key),
         "-set_serial","99","-days","1","-extfile",str(ext),"-out",str(leaf)],capture_output=True)
    chain.write_bytes(leaf.read_bytes()+intermediate.read_bytes())
    return {"ca":str(folder/"ca.pem"),"cert":str(chain),"key":str(leaf_key)},leaf,expiry.timestamp()


def grants(folder, name, cert, subject, role):
    der = ssl.PEM_cert_to_DER_cert(Path(cert).read_text())
    path = folder / name
    path.write_text(json.dumps({hashlib.sha256(der).hexdigest(): {
        "role": role, "subject": subject, "expiresAt": int(time.time()) + 900}}))
    path.chmod(0o600)
    return path


def get(config, origin, path, payload=None):
    tls = ssl.create_default_context(cafile=config["ca"])
    tls.minimum_version = ssl.TLSVersion.TLSv1_3
    tls.load_cert_chain(config["cert"], config["key"])
    try:
        body = None if payload is None else json.dumps(payload).encode()
        with opener(tls).open(Request(origin + path,data=body,headers={"Content-Type":"application/json"}), timeout=10) as response:
            return response.status, response.read()
    except HTTPError as exc:
        return exc.code, exc.read()


def command(binary, identity, grant, env, port, args=()):
    clean = {k: v for k, v in os.environ.items() if not k.startswith("P09_")}
    return subprocess.Popen([str(binary), "-listen", f"127.0.0.1:{port}", "-cert", identity["cert"], "-key", identity["key"],
                             "-client-ca", identity["ca"], "-grants", str(grant), *map(str, args)],
                            cwd=ROOT, env={**clean, **env})


@contextmanager
def serve(binary, identity, grant, env, args=()):
    with socket.socket() as available:
        available.bind(("127.0.0.1", 0))
        port = available.getsockname()[1]
    proc = command(binary, identity, grant, env, port, args)
    try:
        deadline = time.monotonic() + 8
        while True:
            try:
                with socket.create_connection(("127.0.0.1", port), timeout=0.5):
                    break
            except OSError:
                if proc.poll() is not None or time.monotonic() > deadline:
                    raise
                time.sleep(.1)
        yield f"https://127.0.0.1:{port}"
    finally:
        if proc.poll() is None:
            proc.terminate()
        proc.wait(timeout=8)


def fixture_source(name, subject, root_record, child, surviving):
    db(name, f"""
INSERT INTO travel_private.policy VALUES
 ('voice',1,'fixture',gen_random_uuid(),3600,false,true),('transcription',1,'fixture',gen_random_uuid(),3600,false,true);
INSERT INTO travel_private.consent_decision(subject_ref,purpose,revision,policy_version,recipient,accepted,expires_at,request_key)
 SELECT {q(subject)},purpose,1,1,'fixture',true,now()+interval '15 minutes',gen_random_uuid()
 FROM travel_private.policy;
INSERT INTO travel_private.consent SELECT subject_ref,purpose,revision FROM travel_private.consent_decision;
INSERT INTO travel_private.record(subject_ref,id,purpose,revision,recipient,parent_id,delete_after) VALUES
 ({q(subject)},{q(root_record)},'voice',1,'fixture',NULL,now()+interval '15 minutes'),
 ({q(subject)},{q(child)},'transcription',1,'fixture',{q(root_record)},now()+interval '15 minutes'),
 ({q(subject)},{q(surviving)},'voice',1,'fixture',NULL,now()+interval '15 minutes');
INSERT INTO travel_private.record_object SELECT subject_ref,id,sha256(decode('abcd','hex')),
  decode(repeat('32',45),'hex') FROM travel_private.record;
""")


def publish_with_receipts(folder, control, scope, manifest, incumbent, evidence, guard_dsn, probe):
    from test_cutover import key, reject, signed

    targets = {target: key(folder, target + "-owner") for target in ("p05", "p07", "p09")}
    actors = {role: (str(uuid4()), *key(folder, role + "-operator")) for role in ROLES}
    spec = {"targets": {target: public for target, (_, public) in targets.items()},
        "goals": {"rtoSeconds": 600, "rpoSeconds": 60}, "goalRef": str(uuid4()), "synthetic": True,
        "maxApprovalSeconds": 180, "erasureNamespace": evidence["namespace"], "erasureGuardDatabase": guard_dsn}
    config = {"scopes": {scope: spec}, "principals": {actor: {"person": role, "issuer": "fixture-idp",
        "subject": role, "roles": [role], "scopes": [scope], "publicKey": public}
        for role, (actor, _, public) in actors.items()}}
    for target in targets:
        db(control, f"INSERT INTO platform.recovery_target_route VALUES ({q(scope)},'{target}',0,{h(incumbent)});")
    now, job = int(time.time()), str(uuid4())
    report = folder / "p09-publication-proof.json"
    report_id = save(report, {"format": "nu54-recovery-report-v1", "scope": scope, "job": job,
        "manifest": manifest, "status": "await-independent-verification", "goalRef": spec["goalRef"],
        "goals": spec["goals"], "rpoSeconds": 0, "validationSeconds": 0, "incidentEpoch": str(now),
        "ordinaryTrafficFenced": True, "synthetic": True, "erasure": evidence, "reconciliation": {"matched": True}})
    plan = {"kind": "cutover", "scope": scope, "job": job, "priorJob": None, "manifest": manifest,
        "incumbent": incumbent, "routeRevision": 0, "targets": {target: 0 for target in targets},
        "evidence": report_id, "goals": spec["goals"], "goalRef": spec["goalRef"], "issuedAt": now, "expiresAt": now + 120}
    bundle = signed(plan, {role: (actor, private) for role, (actor, private, _) in actors.items()})
    open_fence(control, bundle, report, config)
    for target in ("p05", "p07"):
        collect(control, bundle, target_apply(control, control, bundle, target, targets[target][0], config), config)
    reject(finish, control, bundle, config, report)
    assert probe()[0] == 503 and status(control, scope, job)["fenced"]
    collect(control, bundle, target_apply(control, control, bundle, "p09", targets["p09"][0], config), config)
    finish(control, bundle, config, report)
    assert len(status(control, scope, job)["targets"]) == 3


def test(folder, names, route_login):
    artifacts, seed, journal_key = tls_fixtures(folder)
    programs = {}
    for binary in ("recordsd", "erasured"):
        programs[binary] = folder / binary
        run(["go", "build", "-o", str(programs[binary]), f"./products/p09-travel-ai/cmd/{binary}"])
    with ExitStack() as resources:
        journal = resources.enter_context(isolated(folder / "j"))
        workload = resources.enter_context(isolated(folder / "q"))
        pg(journal.env, ["createdb", "-w", "erasure"])
        pg(workload.env, ["createdb", "-w", "workload"])
        migrate(journal.env, "p09", "erasure")
        migrate(workload.env, "p09", "workload")
        login(journal, "erasure", "restore_export", "nu54_p09_erasure_export")
        login(journal, "erasure", "restore_writer", "nu54_p09_erasure_append")
        login(journal, "erasure", "restore_barrier", "nu54_p09_erasure_guard")
        login(workload, "workload", "restore_apply", "nu54_p09_restore_apply")
        login(workload, "workload", "restore_reader", "nu54_p09_private_user")
        subject, deleted, dependent, surviving = [str(uuid4()) for _ in range(4)]
        namespace, scope, manifest = str(uuid4()), str(uuid4()), "a" * 64
        fixture_source(names["source"], subject, deleted, dependent, surviving)
        old_backup = folder / "pre-erasure.pgdump"
        run(["pg_dump", "-w", "-Fc", "--data-only", "--schema=travel_private",
             "-d", names["source"], "-f", str(old_backup)])
        connection(journal, "erasure", "recovery_operator", f"INSERT INTO travel_erasure.namespace(namespace) VALUES ({q(namespace)});")
        def append(item):
            return (f"SELECT travel_erasure.append_fence({q(subject)},'object',NULL,NULL,'record',{q(item)},"
                    "now(),now()+interval '91 days');")
        assert connection(journal, "erasure", "restore_writer", append(deleted)) == "1"
        pg(workload.env, ["pg_restore", "-w", "--exit-on-error", "--single-transaction",
                          "-d", "workload", str(old_backup)])
        assert connection(workload, "workload", "recovery_operator", "SELECT count(*) FROM travel_private.record_object;") == "3"
        restore = grants(folder, "restore-grants.json", artifacts["restore"]["cert"], str(uuid4()), "restore")
        traveler = grants(folder, "traveler-grants.json", artifacts["traveler"]["cert"], subject, "private-user")
        journal_origin = resources.enter_context(serve(programs["erasured"], artifacts["journal"], restore, {
            "P09_ERASURE_DATABASE_URL": dsn(journal, "erasure", "restore_export")}, ["-journal-signer", seed]))
        def restore_env(high):
            return {"P09_RESTORE_QUARANTINE": "true", "P09_RESTORE_NAMESPACE": namespace, "P09_RESTORE_HIGH_WATER": high,
                "P09_RESTORE_JOURNAL_KEY": str(journal_key), "P09_RESTORE_DATABASE_URL": dsn(workload, "workload", "restore_apply")}
        def journal_config(workload):
            return {"namespace": namespace, "journal": {**artifacts["restore"], "origin": journal_origin},
                    "workload": {**artifacts["restore"], "origin": workload}}

        ordinary_dsn = dsn(workload, "workload", "restore_reader")
        content_key = folder / "private-content.fixture-key"
        content_key.write_bytes(b64encode(os.urandom(32)))
        content_key.chmod(0o600)
        with serve(programs["recordsd"], artifacts["workload"], restore, restore_env("1")) as isolated_origin:
            assert get(artifacts["restore"], isolated_origin, "/v1/private/records")[0] == 404
            process = command(programs["recordsd"], artifacts["ordinary"], traveler,
                             {"P09_PRIVATE_DATABASE_URL": ordinary_dsn}, 0)
            try:
                assert process.wait(timeout=5) != 0, "ordinary P09 startup admitted a quarantined workload"
            finally:
                if process.poll() is None:
                    process.terminate()
                    process.wait(timeout=5)
            evidence = apply_and_recheck(journal_config(isolated_origin))
            assert evidence["highWater"] == evidence["appliedThrough"] == "1"
            assert apply_and_recheck(journal_config(isolated_origin)) == evidence

        assert connection(workload, "workload", "recovery_operator", "SELECT count(*) FROM travel_private.record_object;") == "1"
        incumbent = "b" * 64
        db(names["control"], f"INSERT INTO platform.recovery_route VALUES ({q(scope)},0,NULL,{h(incumbent)});")
        guard_dsn = dsn(journal, "erasure", "restore_barrier")
        configuration = {"P09_PRIVATE_DATABASE_URL": ordinary_dsn, "P09_CONTENT_KEY": str(content_key), "P09_RECOVERY_SCOPE": scope,
            "P09_RECOVERY_MANIFEST": manifest, "P09_RECOVERY_DATABASE_URL": dsn(None, names["control"], route_login),
            "P09_LIVE_ERASURE_GUARD_DATABASE_URL": guard_dsn}
        with serve(programs["recordsd"], artifacts["ordinary"], traveler, configuration) as current:
            def ordinary_read():
                return get(artifacts["traveler"], current, "/v1/private/records")
            assert ordinary_read()[0] == 503
            publish_with_receipts(folder, names["control"], scope, manifest, incumbent, evidence, guard_dsn, ordinary_read)
            seen, content = ordinary_read()
            assert seen == 200, f"ordinary restored workload returned {seen}"
            assert surviving.encode() in content and deleted.encode() not in content and dependent.encode() not in content
            # The publication supervisor holds this read-only guard through the
            # ingress CAS. An authenticated writer must wait for it to release.
            with erasure_guard({"erasureGuardDatabase": guard_dsn}, evidence):
                writing = subprocess.Popen(["psql", "-X", "-qAtw", "-v", "ON_ERROR_STOP=1", "-d", "erasure",
                                           "-c", append(surviving)], env=db_env(journal, "restore_writer"), stdout=subprocess.PIPE, text=True)
                time.sleep(.15)
                assert writing.poll() is None, "journal write passed a live publication barrier"
            written, _ = writing.communicate(timeout=5)
            assert writing.returncode == 0 and written.strip() == "2"
            blocked, content = get(artifacts["traveler"], current, "/v1/private/records")
            assert blocked == 503 and surviving.encode() not in content
            try:
                with erasure_guard({"erasureGuardDatabase": guard_dsn}, evidence):
                    raise AssertionError("old cursor passed publication guard")
            except ValueError:
                pass
            with serve(programs["recordsd"], artifacts["workload"], restore, restore_env("2")) as isolated_origin:
                evidence = apply_and_recheck(journal_config(isolated_origin))
            assert evidence["highWater"] == "2"
            assert connection(workload, "workload", "recovery_operator", "SELECT count(*) FROM travel_private.record_object;") == "0"
            code, content = get(artifacts["traveler"], current, "/v1/private/records")
            assert code == 200 and all(item.encode() not in content for item in (deleted, dependent, surviving))
        print(json.dumps({"preErasureBackupRows": 3, "afterFirstApplyRows": 1, "afterSecondApplyRows": 0,
            "physicalJournalCluster": True, "highWater": evidence["highWater"], "deletedContentReexposure": 0,
            "staleLiveRead": "blocked", "concurrentJournalAppend": "waited", "publicationTargetReceipts": 3,
            "mTLS": "recordsd and erasured"}, sort_keys=True))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--existing-postgres", action="store_true", required=True)
    parser.parse_args()
    names = {key: f"n54p09_{key}_{uuid4().hex[:12]}" for key in ("source", "control")}
    created = []
    route_login = "n54_gen_" + uuid4().hex[:12]
    provisioned = False
    try:
        with scoped_workspace("n54p09-", directory="/tmp") as directory:
            for name, owner in zip(names.values(), ("p09", "p10"), strict=True):
                run(["createdb", "-w", name])
                created.append(name)
                migrate(os.environ.copy(), owner, name)
            db(names["control"], f"CREATE ROLE {route_login} LOGIN IN ROLE nu54_p10_recovery_route_only;")
            provisioned = True
            test(directory, names, route_login)
    finally:
        for name in created:
            run(["dropdb", "-w", name])
        if provisioned:
            db("postgres", f"DROP ROLE {route_login};")


if __name__ == "__main__":
    main()

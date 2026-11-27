"""Disposable loopback-only P09/P05 visit journey authority for the iOS simulator."""

import argparse
import base64
from contextlib import ExitStack
import hashlib
from http.server import BaseHTTPRequestHandler, HTTPServer
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
from tempfile import TemporaryDirectory
import time

SCRIPTS = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(SCRIPTS))
from test_travel_booking_integration import free_loopback_port, run

SUBJECT = "a4aa517d-b258-4f88-bea1-00630c790cd5"
ISSUER = "synthetic-qa-p05"
PROOFS = [f"10000000-0000-4000-8000-{i:012d}" for i in range(1, 6)]
POLICIES = [
    ("voice", "p09-sync", False), ("review", "p09", True), ("challenge", "p09", False),
]


def b64(value):
    return base64.b64encode(value).decode()


def write_private(path, data):
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(descriptor, "w") as stream:
        stream.write(data)


def stop(process):
    if process.poll() is None:
        process.terminate()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()


def start(stack, log, args, env):
    stream = stack.enter_context(log.open("a"))
    process = subprocess.Popen(args, env=env, stdout=stream, stderr=stream)
    stack.callback(stop, process)
    return process


def stop_postgres(pg_ctl, data, env):
    if (data / "postmaster.pid").is_file():
        run(pg_ctl, "-D", data, "-m", "fast", "-w", "stop", env=env)


def fixture(work, ports, ready_file, pg, stack):
    database_port, records_port, benefit_port = ports
    env = {key: val for key, val in os.environ.items() if not key.startswith(("PG", "P05_", "P09_"))}
    env.update(PATH=str(pg) + os.pathsep + env["PATH"], PGHOST=str(work), PGPORT=str(database_port))
    data = work / "pg"
    run(pg / "initdb", "-D", data, "-A", "trust", "--no-locale", "--encoding=UTF8", env=env)
    stack.callback(stop_postgres, pg / "pg_ctl", data, env)
    run(pg / "pg_ctl", "-D", data, "-l", work / "postgres.log", "-o",
        f"-k {work} -p {database_port} -c listen_addresses=''", "-w", "start", env=env)

    def database(owner, name):
        dsn = f"host={work} port={database_port} dbname={name}"
        run(pg / "createdb", name, env=env)
        run("python3", SCRIPTS / "../products/p10-platform/database/migrate.py", owner,
            env={**env, f"{owner.upper()}_MIGRATION_DATABASE_URL": dsn})
        return dsn

    def sql(dsn, statement):
        run(pg / "psql", "-Xv", "ON_ERROR_STOP=1", dsn, "-c", statement, env=env)

    travel = database("p09", "nu54_p09_private_visit_qa")
    benefit = database("p05", "nu54_p05_benefit_visit_qa")
    policies = ", ".join(
        f"('{purpose}',1,'{recipient}',gen_random_uuid(),1200,{str(target).lower()},true)"
        for purpose, recipient, target in POLICIES
    )
    sql(travel, "CREATE ROLE visit_traveler LOGIN IN ROLE nu54_p09_private_user; "
        f"INSERT INTO travel_private.policy VALUES {policies};")
    example_visits = [("merchant-a", "live"), ("merchant-a", "live"), ("merchant-b", "live"),
                      ("merchant-c", "live"), ("merchant-d", "synthetic")]
    visits = ", ".join(
        f"('{ISSUER}','{proof}','{SUBJECT}','{place}','{mode}',1,'settled',now()-interval '{i} days')"
        for i, (proof, (place, mode)) in enumerate(zip(PROOFS, example_visits), start=1)
    )
    sql(travel, "INSERT INTO travel_private.visit_evidence"
        f"(issuer,proof,subject_ref,place_id,environment,source_revision,state,observed_at) VALUES {visits};")
    # Seeder uses the disposable owner. The service below can read only the scoped benefit summary.
    sql(benefit, f"""
      CREATE ROLE visit_benefit_reader LOGIN IN ROLE nu54_p05_benefit_read;
      INSERT INTO ledger.branch(id,tenant_ref,timezone,close_local)
        VALUES('10000000-1000-4000-8000-000000000001',gen_random_uuid(),'Asia/Seoul','23:00');
      INSERT INTO ledger.sale(id,branch_id,origin,origin_ref,state,asset_ref,amount)
        VALUES('20000000-1000-4000-8000-000000000001','10000000-1000-4000-8000-000000000001',
          'menu',gen_random_uuid(),'paid','fixture-asset',1);
      INSERT INTO ledger.visit_benefit_enrollment VALUES
        ('10000000-1000-4000-8000-000000000001','20000000-1000-4000-8000-000000000001',
          '30000000-1000-4000-8000-000000000001','{ISSUER}','{PROOFS[0]}','live');
      INSERT INTO ledger.quote_work(sale_id,actor_id,claim_digest,quote_id,order_id,nonce,expected_revision,profile_version,
        merchant_signer,payer,payer_generation,payer_role,issued_at,expires_at,fence,lease_until)
        VALUES('20000000-1000-4000-8000-000000000001',gen_random_uuid(),decode(repeat('11',32),'hex'),decode(repeat('12',32),'hex'),
        decode(repeat('13',32),'hex'),decode(repeat('14',32),'hex'),1,1,decode(repeat('15',20),'hex'),decode(repeat('16',20),'hex'),1,1,now(),now()+interval '5 min',1,now()+interval '1 min');
      INSERT INTO ledger.payment(sale_id,stream_id,merchant_key,evidence_id,tx_hash,log_index,inclusion_hash,included_at,asset_ref,payer,payer_generation,payment_mode,principal)
        VALUES('20000000-1000-4000-8000-000000000001',gen_random_uuid(),'0x'||repeat('15',32),gen_random_uuid(),
        '0x'||repeat('17',32),0,'0x'||repeat('18',32),now(),'fixture-asset','0x'||repeat('16',20),1,1,1);
      DO $check$ BEGIN
        IF ledger.apply_visit_benefit('{ISSUER}','{PROOFS[0]}','30000000-1000-4000-8000-000000000001',1,'eligible',now()+interval '40 seconds') IS NOT TRUE THEN RAISE EXCEPTION 'credit missing'; END IF;
        IF ledger.apply_visit_benefit('{ISSUER}','{PROOFS[0]}','30000000-1000-4000-8000-000000000001',1,'eligible',now()+interval '40 seconds') IS NOT FALSE THEN RAISE EXCEPTION 'duplicate effect'; END IF;
        IF (SELECT count(*) FROM ledger.visit_benefit_effect WHERE delta=1)<>1 THEN RAISE EXCEPTION 'wrong credit count'; END IF;
      END $check$;
    """)

    recordsd, benefit_reader = work / "recordsd", work / "benefit-reader"
    run("go", "build", "-o", recordsd, "./products/p09-travel-ai/cmd/recordsd", env=env)
    run("go", "build", "-o", benefit_reader, "./products/p05-operations-backoffice/cmd/visit-benefit-read", env=env)
    ca, ca_key = work / "ca.pem", work / "ca.key"
    run("openssl", "req", "-x509", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes",
        "-keyout", ca_key, "-out", ca, "-days", "1", "-subj", "/CN=Disposable travel fixture CA",
        "-addext", "basicConstraints=critical,CA:TRUE", "-addext", "keyUsage=critical,keyCertSign,cRLSign", env=env)

    def certificate(name, extension):
        key, csr, cert = (work / f"{name}.{suffix}" for suffix in ("key", "csr", "pem"))
        extensions = work / f"{name}.conf"
        extensions.write_text(extension)
        run("openssl", "req", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes",
            "-keyout", key, "-out", csr, "-subj", f"/CN={name}", env=env)
        run("openssl", "x509", "-req", "-in", csr, "-CA", ca, "-CAkey", ca_key,
            "-CAcreateserial", "-extfile", extensions, "-out", cert, "-days", "1", env=env)
        return cert, key

    server_cert, server_key = certificate("server", "subjectAltName=IP:127.0.0.1\nextendedKeyUsage=serverAuth\n")
    reader_cert, reader_key = certificate("benefit-reader", "extendedKeyUsage=clientAuth\n")
    ca_der = subprocess.check_output(["openssl", "x509", "-in", ca, "-outform", "DER"])
    server_pem = subprocess.check_output(["openssl", "x509", "-in", server_cert, "-pubkey", "-noout"])
    server_spki = subprocess.check_output(["openssl", "pkey", "-pubin", "-outform", "DER"], input=server_pem)
    reader_der = subprocess.check_output(["openssl", "x509", "-in", reader_cert, "-outform", "DER"])
    review_key, content_key = work / "review.key", work / "content.key"
    write_private(review_key, b64(os.urandom(32)))
    write_private(content_key, b64(os.urandom(32)))
    start(stack, work / "benefit.log",
          [benefit_reader, "-listen", f"127.0.0.1:{benefit_port}", "-cert", server_cert, "-key", server_key,
           "-client-ca", ca, "-reader-sha256", hashlib.sha256(reader_der).hexdigest()],
          {**env, "P05_BENEFIT_READ_DATABASE_URL": benefit + " user=visit_benefit_reader"})

    grants = work / "grants.json"

    def start_records():
        start(stack, work / "recordsd.log",
              [recordsd, "-listen", f"127.0.0.1:{records_port}", "-cert", server_cert, "-key", server_key,
               "-client-ca", ca, "-grants", grants],
              {**env, "P09_PRIVATE_DATABASE_URL": travel + " user=visit_traveler", "P09_REVIEW_KEY": str(review_key),
               "P09_CONTENT_KEY": str(content_key),
               "P09_BENEFIT_STATUS_ENDPOINT": f"https://127.0.0.1:{benefit_port}/v1/visit-benefits/status",
               "P09_BENEFIT_STATUS_CA": str(ca), "P09_BENEFIT_STATUS_CERT": str(reader_cert),
               "P09_BENEFIT_STATUS_KEY": str(reader_key)})

    class Enrollment(BaseHTTPRequestHandler):
        enrolled = False
        populated = False
        adjusted = False

        def respond(self, payload):
            body = json.dumps(payload, separators=(",", ":")).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def do_POST(self):
            if self.path == "/verify-one" and Enrollment.enrolled:
                sql(travel, f"""DO $check$ BEGIN
                    IF (SELECT count(*) FROM travel_private.visit_options('{SUBJECT}') WHERE challenge='verified')<>1
                      OR NOT EXISTS(SELECT FROM travel_private.visit_options('{SUBJECT}') WHERE proof='{PROOFS[0]}' AND challenge='verified')
                    THEN RAISE EXCEPTION 'visit proof not unique'; END IF;
                END $check$;""")
                sql(benefit, f"""DO $check$ BEGIN
                    IF (SELECT count(*) FROM ledger.visit_benefit_effect)<>1
                      OR NOT EXISTS(SELECT FROM ledger.visit_benefit_effect WHERE issuer='{ISSUER}' AND proof='{PROOFS[0]}' AND delta=1)
                    THEN RAISE EXCEPTION 'benefit effect not unique'; END IF;
                END $check$;""")
                self.respond({})
                return
            if self.path == "/adjust" and Enrollment.enrolled and not Enrollment.adjusted:
                sql(travel, f"UPDATE travel_private.visit_evidence SET source_revision=2,state='refunded' WHERE proof='{PROOFS[2]}'")
                sql(benefit, f"""DO $check$ BEGIN
                    IF ledger.apply_visit_benefit('{ISSUER}','{PROOFS[0]}','30000000-1000-4000-8000-000000000001',
                      2,'adjusted',now()+interval '40 seconds') IS NOT TRUE THEN RAISE EXCEPTION 'adjustment missing'; END IF;
                END $check$;""")
                Enrollment.adjusted = True
                self.respond({})
                return
            if self.path == "/populate" and Enrollment.enrolled and not Enrollment.populated:
                sql(travel, f"""
                    INSERT INTO travel_private.visit_evidence(issuer,proof,subject_ref,place_id,environment,source_revision,state,observed_at)
                    SELECT repeat('i',64),gen_random_uuid(),'{SUBJECT}',repeat('p',48),'live',1,'settled',now()-make_interval(hours=>n)
                      FROM generate_series(1,111) n;
                    INSERT INTO travel_private.visit_claim(issuer,proof,purpose,consent_revision)
                    SELECT issuer,proof,'challenge',1 FROM travel_private.visit_evidence WHERE issuer=repeat('i',64);
                """)
                Enrollment.populated = True
                self.respond({})
                return
            if self.path != "/enroll" or Enrollment.enrolled:
                self.send_error(404)
                return
            try:
                length = int(self.headers.get("Content-Length", "0"))
                if not 1 <= length <= 2048:
                    raise ValueError("invalid public key length")
                public = base64.b64decode(self.rfile.read(length), validate=True)
                if len(public) != 91:
                    raise ValueError("invalid public key shape")
                (work / "user.der").write_bytes(public)
                run("openssl", "pkey", "-pubin", "-inform", "DER", "-in", work / "user.der",
                    "-out", work / "user.pub", env=env)
                extensions = work / "user.conf"
                extensions.write_text("extendedKeyUsage=clientAuth\nkeyUsage=digitalSignature\n")
                cert = work / "user.pem"
                run("openssl", "x509", "-new", "-force_pubkey", work / "user.pub", "-subj", "/CN=Synthetic travel-only user",
                    "-CA", ca, "-CAkey", ca_key, "-CAcreateserial", "-days", "1", "-extfile", extensions, "-out", cert, env=env)
                client = subprocess.check_output(["openssl", "x509", "-in", cert, "-outform", "DER"])
            except (ValueError, subprocess.CalledProcessError):
                self.send_error(400)
                return
            expiry = int(time.time()) + 900
            update = work / "grants.next"
            write_private(update, json.dumps({hashlib.sha256(client).hexdigest(): {
                "subject": SUBJECT, "role": "private-user", "expiresAt": expiry,
            }}))
            update.replace(grants)
            start_records()
            Enrollment.enrolled = True
            self.respond({
                "endpoint": f"https://127.0.0.1:{records_port}/v1/private",
                "pin": hashlib.sha256(server_spki).hexdigest(), "ca": b64(ca_der), "certificates": [b64(client)],
                "scope": {"role": "private-user", "expiresAt": expiry},
                "policies": [{"purpose": purpose, "version": 1, "recipient": recipient, "maxLifetimeSeconds": 1200,
                              "targetRequired": target} for purpose, recipient, target in POLICIES],
            })

    with HTTPServer(("127.0.0.1", 0), Enrollment) as enrollment:
        ready_file.write_text(f"http://127.0.0.1:{enrollment.server_port}")
        print("PRIVATE VISIT ENROLLMENT READY", flush=True)
        enrollment.serve_forever()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ready-file", type=Path, required=True)
    args = parser.parse_args()
    pg = Path(os.environ.get("POSTGRES_BIN", "/opt/homebrew/opt/postgresql@15/bin"))
    ports = set()
    while len(ports) < 3:
        ports.add(free_loopback_port())
    os.umask(0o077)
    with TemporaryDirectory(prefix="nu54-ios-visit-", dir="/tmp") as directory, ExitStack() as stack:
        fixture(Path(directory), sorted(ports), args.ready_file, pg, stack)


if __name__ == "__main__":
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(0))
    main()

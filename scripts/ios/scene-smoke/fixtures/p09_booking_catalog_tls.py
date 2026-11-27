"""Publish the real, signed P09 catalog for a disposable native booking run."""
import argparse
import http.server
import json
import os
import re
from pathlib import Path
import signal
import ssl
import subprocess
import sys
import tempfile
import time
import threading
import urllib.error
import urllib.request
import uuid

sys.path.insert(0, str(Path.cwd() / "scripts"))
from test_travel_booking_integration import BRANCH, free_loopback_port, pg, published_fixture, run



def watch_private_schema(dsn, env, fixture):
    def sql(statement):
        return subprocess.check_output(
            [pg("psql"), "-XqAt", "-v", "ON_ERROR_STOP=1", dsn, "-c", statement],
            env=env, text=True).strip()

    names = sql("SELECT tablename FROM pg_tables WHERE schemaname='travel_private' ORDER BY 1").splitlines()
    required = {"consent", "consent_decision", "record", "record_object", "visit_evidence",
                "visit_claim", "review", "processing_job", "itinerary"}
    if len(names) != 17 or not required.issubset(names) or not all(re.fullmatch(r"[a-z_]+", name) for name in names):
        raise RuntimeError("The privacy fixture must observe 17 optional relations")
    privileges = sql("""SELECT has_schema_privilege('travelread','travel_private','USAGE') OR
        EXISTS (SELECT FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
        WHERE n.nspname='travel_private' AND c.relkind='r'
        AND has_table_privilege('travelread',c.oid,'SELECT, INSERT, UPDATE, DELETE'))""")
    if privileges != "f":
        raise RuntimeError("The public role has private schema authority")
    sql("""CREATE SCHEMA acceptance_probe;
        CREATE SEQUENCE acceptance_probe.writes;
        CREATE FUNCTION acceptance_probe.observe() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER
        SET search_path=pg_catalog AS $$ BEGIN
            PERFORM nextval('acceptance_probe.writes'::regclass);
            RETURN NULL;
        END $$""")
    for name in names:
        sql(f"""CREATE TRIGGER booking_watch_write AFTER INSERT OR UPDATE OR DELETE ON travel_private.{name}
                   FOR EACH ROW EXECUTE FUNCTION acceptance_probe.observe();
               CREATE TRIGGER booking_watch_truncate AFTER TRUNCATE ON travel_private.{name}
                   FOR EACH STATEMENT EXECUTE FUNCTION acceptance_probe.observe()""")

    def counts():
        return {name: int(sql("SELECT count(*) FROM travel_private." + name)) for name in names}

    before = counts()
    if before["consent"] or before["consent_decision"]:
        raise RuntimeError("The optional-travel fixture unexpectedly has a registered consent")
    public_requests = []
    authenticated_requests = []

    def finish():
        while not (fixture / "privacy-check").exists():
            time.sleep(0.2)
        after = counts()
        writes = int(sql("SELECT CASE WHEN is_called THEN last_value ELSE 0 END FROM acceptance_probe.writes"))
        proof = {"optionalRelations": len(names), "optionalWrites": writes, "before": before, "after": after,
                 "publicPrivatePrivileges": privileges != "f", "publicRequests": len(public_requests),
                 "authenticatedPublicRequests": len(authenticated_requests)}
        staged = fixture / "privacy-proof.tmp"
        staged.write_text(json.dumps(proof, sort_keys=True))
        staged.replace(fixture / "privacy-proof.json")

    threading.Thread(target=finish, daemon=True).start()
    return public_requests, authenticated_requests

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("fixture", type=Path)
    parser.add_argument("port", type=int)
    parser.add_argument("simulator")
    parser.add_argument("--itinerary", action="store_true")
    parser.add_argument("--pickup", action="store_true")
    parser.add_argument("--wait-pickup-branch", action="store_true")
    parser.add_argument("--pickup-dsn-file", type=Path)
    args = parser.parse_args()
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(0))
    os.umask(0o077)

    with tempfile.TemporaryDirectory(prefix="nu54-booking-catalog-", dir="/tmp") as path:
        work = Path(path)
        db_port = str(free_loopback_port())
        env = {k: v for k, v in os.environ.items() if not k.startswith("PG")}
        env.update(PGPORT=db_port, PGHOST=path)
        ctl = pg("pg_ctl")
        try:
            if args.pickup and args.pickup_dsn_file:
                deadline = time.monotonic() + 90
                while not args.pickup_dsn_file.is_file() and time.monotonic() < deadline:
                    time.sleep(0.2)
                dsn = args.pickup_dsn_file.read_text().strip()
                if not re.fullmatch(r"host=/tmp/nu54-native-pickup-[a-z0-9_]+ port=[1-9][0-9]{3,4} dbname=nu54_p09_native_pickup", dsn):
                    raise ValueError("Use only the pickup fixture database")
            else:
                run(pg("initdb"), "-D", work / "pg", "-A", "trust", "--no-locale", "--encoding=UTF8", env=env)
                run(ctl, "-D", work / "pg", "-l", work / "postgres.log", "-o",
                    f"-k {work} -p {db_port} -c listen_addresses=''", "-w", "start", env=env)
                db = "nu54_p09_native_booking"
                dsn = f"host={work} port={db_port} dbname={db}"
                run(pg("createdb"), db, env=env)
                run("python3", "products/p10-platform/database/migrate.py", "p09",
                    env={**env, "P09_MIGRATION_DATABASE_URL": dsn})
            run("openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes",
                "-keyout", work / "ca.key", "-out", work / "ca.pem",
                "-days", "1", "-subj", "/CN=NU54 Disposable Booking Catalog",
                "-addext", "basicConstraints=critical,CA:TRUE", "-addext", "keyUsage=critical,keyCertSign,cRLSign", env=env)
            run("openssl", "req", "-newkey", "rsa:2048", "-nodes", "-keyout", work / "server.key",
                "-out", work / "server.csr", "-subj", "/CN=127.0.0.1", env=env)
            (work / "server.conf").write_text(
                "subjectAltName=IP:127.0.0.1\nextendedKeyUsage=serverAuth\nkeyUsage=digitalSignature,keyEncipherment\n")
            run("openssl", "x509", "-req", "-in", work / "server.csr", "-CA", work / "ca.pem",
                "-CAkey", work / "ca.key", "-CAcreateserial", "-extfile", work / "server.conf",
                "-out", work / "server.pem", "-days", "1", env=env)
            expires = time.monotonic() + 260
            branch_path = args.fixture / "public-branch"
            branch = BRANCH
            if not args.itinerary and (not args.pickup or args.wait_pickup_branch):
                while not branch_path.is_file():
                    if (args.fixture / "ui-aborted").is_file() or time.monotonic() >= expires:
                        raise SystemExit("Local booking did not announce its public branch")
                    time.sleep(0.2)
                branch = str(uuid.UUID(branch_path.read_text()))

            with published_fixture(work, env, dsn, branch=branch, capabilities=("pickup",) if args.pickup else ("booking",)) as approved:
                watcher = watch_private_schema(dsn, env, args.fixture) if os.environ.get("NU54_TRAVEL_NATIVE_UC1") == "1" else None
                class PublicCatalog(http.server.BaseHTTPRequestHandler):
                    def log_message(self, fmt, *arguments):
                        pass

                    def do_GET(self):
                        if self.headers.get("Authorization") or self.headers.get("Cookie") or self.headers.get("X-NU54-Subject"):
                            if watcher:
                                watcher[1].append(1)
                            self.send_error(400)
                            return
                        try:
                            with urllib.request.urlopen(approved + self.path, timeout=3) as response:
                                data = response.read()
                                if watcher:
                                    watcher[0].append(1)
                                self.send_response(response.status)
                                self.send_header("Content-Type", response.headers["Content-Type"])
                                self.send_header("Content-Length", str(len(data)))
                                self.end_headers()
                                self.wfile.write(data)
                        except urllib.error.HTTPError as error:
                            self.send_error(error.code)

                with http.server.ThreadingHTTPServer(("127.0.0.1", args.port), PublicCatalog) as server:
                    tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
                    tls.minimum_version = ssl.TLSVersion.TLSv1_3
                    tls.load_cert_chain(work / "server.pem", work / "server.key")
                    server.socket = tls.wrap_socket(server.socket, server_side=True)
                    run("xcrun", "simctl", "keychain", args.simulator, "add-root-cert", work / "ca.pem", env=env)
                    (args.fixture / "catalog-ready").touch()
                    server.serve_forever()
        finally:
            if (work / "pg" / "postmaster.pid").is_file():
                subprocess.run([ctl, "-D", work / "pg", "-m", "fast", "-w", "stop"], env=env, check=False)


if __name__ == "__main__":
    main()

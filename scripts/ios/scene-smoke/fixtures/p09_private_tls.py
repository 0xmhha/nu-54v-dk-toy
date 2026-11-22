import argparse
import secrets
import ssl
from threading import Event, Thread
import urllib.request
import uuid
import base64, hashlib, http.server, json, os, sys, time
from pathlib import Path
sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parents[3]))
from integration_workspace import OwnedProcess, check_output, cleanup_stack, integration_workspace, start_postgres
from test_travel_booking_integration import run, free_loopback_port, published_fixture
base64str=lambda b:base64.b64encode(b).decode()
actor='a4aa517d-b258-4f88-bea1-00630c790cd5'
parser=argparse.ArgumentParser(description="Disposable P09 iOS enrollment service")
parser.add_argument("--records-port",type=int,default=65141)
parser.add_argument("--enrollment-port",type=int,default=65142)
parser.add_argument("--work-file",type=Path,default=Path("/tmp/nu54-p09-native-private-fixture.path"))
parser.add_argument("--conflict",action="store_true")
parser.add_argument("--recommendation",action="store_true")
args=parser.parse_args()
records_origin=f"https://127.0.0.1:{args.records_port}"
test_grant=os.environ.get("NU54_T13_TEST_GRANT","")
if (args.conflict or args.recommendation) and not test_grant: parser.error("test mode requires an isolated test grant")

base = {k: v for k, v in os.environ.items() if not k.startswith("PG")}
pg = os.environ.get("POSTGRES_BIN", "/opt/homebrew/opt/postgresql@15/bin")
base["PATH"] = pg + os.pathsep + base["PATH"]
with integration_workspace("nu54-private-ios-", pg + "/pg_ctl", "pg", base, term_status=0) as (work, env):
    port = str(free_loopback_port())
    env.update(PGPORT=port, PGHOST=str(work))
    run(pg + "/initdb", "-D", work / "pg", "-A", "trust", "--no-locale", "--encoding=UTF8", env=env)
    start_postgres(pg + "/pg_ctl", work / "pg", work / "postgres.log", f"-k {work} -p {port} -c listen_addresses=''", env)
    service = OwnedProcess()
    recommendation_lifetime = 604800 if args.recommendation else 172800
    with cleanup_stack() as stack:
        stack.callback(service.stop)
        db='nu54_p09_private_ios'
        dsn=f'host={work} port={port} dbname={db}'
        run(pg+'/createdb',db,env=env)
        run('python3','products/p10-platform/database/migrate.py','p09',env={**env,'P09_MIGRATION_DATABASE_URL':dsn})
        catalog_dsn=f'host={work} port={port} dbname=nu54_p09_private_catalog'
        run(pg+'/createdb','nu54_p09_private_catalog',env=env)
        run('python3','products/p10-platform/database/migrate.py','p09',env={**env,'P09_MIGRATION_DATABASE_URL':catalog_dsn})
        archive=work/'catalog'
        archive.mkdir(mode=0o700)
        stack.enter_context(published_fixture(archive,env,catalog_dsn))
        run(pg+'/psql','-Xv','ON_ERROR_STOP=1',dsn,'-c',
            "CREATE ROLE privateios LOGIN IN ROLE nu54_p09_private_user; INSERT INTO travel_private.policy VALUES "
            f"('voice',1,'p09-sync',gen_random_uuid(),1200,false,true),('transcription',1,'fixture-transcriber',gen_random_uuid(),1200,true,true),('recommendation',1,'p09',gen_random_uuid(),{recommendation_lifetime},false,true);",env=env)
        run('go','build','-o',work/'recordsd','./products/p09-travel-ai/cmd/recordsd',env=env)
        run('openssl','req','-x509','-newkey','ec','-pkeyopt','ec_paramgen_curve:P-256','-nodes','-keyout',work/'ca.key','-out',work/'ca.pem',
            '-days','1','-subj','/CN=Isolated P09 private iOS issuer','-addext','basicConstraints=critical,CA:TRUE',
            '-addext','keyUsage=critical,keyCertSign,cRLSign',env=env)
        run('openssl','req','-newkey','ec','-pkeyopt','ec_paramgen_curve:P-256','-nodes','-keyout',work/'server.key','-out',work/'server.csr',
            '-subj','/CN=127.0.0.1',env=env)
        (work/'server.conf').write_text('subjectAltName=IP:127.0.0.1\nextendedKeyUsage=serverAuth\nkeyUsage=digitalSignature\n')
        run('openssl','x509','-req','-in',work/'server.csr','-CA',work/'ca.pem','-CAkey',work/'ca.key',
            '-CAcreateserial','-extfile',work/'server.conf','-out',work/'server.pem','-days','1',env=env)
        caDER=check_output(['openssl','x509','-in',work/'ca.pem','-outform','DER'], env=env)
        serverPublic=check_output(['openssl','x509','-in',work/'server.pem','-pubkey','-noout'])
        serverSPKI=check_output(['openssl','pkey','-pubin','-outform','DER'],input=serverPublic, env=env)
        secret=work/'content'
        secret.write_text(base64str(os.urandom(32)));secret.chmod(0o600)
        rank_endpoint = "http://127.0.0.1:11434/api/generate"
        rank_model = "gemma3:12b"
        rank_started, release_rank = Event(), Event()
        rank_requests = []
        if args.recommendation:
            class Ranker(http.server.BaseHTTPRequestHandler):
                def do_POST(self):
                    try:
                        length = int(self.headers.get("Content-Length", "0"))
                        if self.path != "/api/generate" or length not in range(1, 8193):
                            raise ValueError("invalid ranker request")
                        envelope = json.loads(self.rfile.read(length))
                        _, public = envelope["prompt"].rsplit("Include every listed ID once. ", 1)
                        candidate_request = json.loads(public)
                        assert set(candidate_request) == {"publicCandidates", "categoryPreference"}
                        assert candidate_request["categoryPreference"] in {"all", "restaurant", "attraction"}
                        ids = []
                        for candidate in candidate_request["publicCandidates"]:
                            assert set(candidate) == {"id", "category"} and candidate["id"] in {"landmark", "merchant"}
                            ids.append(candidate["id"])
                        assert ids and envelope["model"] == "fixture-ranker"
                        rank_requests.append(candidate_request)
                        if len(rank_requests) == 1:
                            rank_started.set()
                            if not release_rank.wait(timeout=50):
                                raise ValueError("fixture cancellation did not release ranker")
                        body = json.dumps({"done": True, "response": json.dumps({"order": sorted(ids)})}).encode()
                        self.send_response(200)
                        self.send_header("Content-Length", str(len(body)))
                        self.end_headers()
                        self.wfile.write(body)
                    except (AssertionError, ValueError, KeyError):
                        self.send_error(400)

            ranker = stack.enter_context(http.server.HTTPServer(("127.0.0.1", 0), Ranker))
            thread = Thread(target=ranker.serve_forever, daemon=True)
            thread.start()
            stack.callback(ranker.shutdown)
            stack.callback(release_rank.set)
            rank_endpoint = f"http://127.0.0.1:{ranker.server_port}/api/generate"
            rank_model = "fixture-ranker"

        class Enroll(http.server.BaseHTTPRequestHandler):
            def do_POST(self):
                if (args.conflict or args.recommendation) and not secrets.compare_digest(self.headers.get("Authorization",""), "Bearer "+test_grant):
                    self.send_error(404);return
                if args.recommendation and self.path in {"/ranking-started", "/ranking-release", "/draft-proof"}:
                    if self.path == "/ranking-started" and not rank_started.wait(timeout=10):
                        self.send_error(409)
                        return
                    if self.path == "/ranking-release":
                        release_rank.set()
                    saved = check_output([pg+"/psql", "-XtA", dsn, "-c",
                        "SELECT count(*) FROM travel_private.itinerary"], env=env, text=True).strip()
                    body = json.dumps({"saved": saved, "ranked": str(len(rank_requests))}).encode()
                    self.send_response(200)
                    self.send_header("Content-Length", str(len(body)))
                    self.end_headers()
                    self.wfile.write(body)
                    return
                if args.conflict and self.path == "/advance":
                    try:
                        ctx=ssl.create_default_context(cafile=str(work/"ca.pem"))
                        ctx.minimum_version=ssl.TLSVersion.TLSv1_3
                        ctx.load_cert_chain(work/"peer.pem",work/"peer.key")

                        def call(path,body=None):
                            request=urllib.request.Request(
                                records_origin+"/v1/private"+path,
                                data=None if body is None else json.dumps(body).encode(),
                                headers={"Content-Type":"application/json"})
                            with urllib.request.urlopen(request,context=ctx,timeout=8) as response:
                                return json.load(response)

                        items=call("/records",{"projection":"saved-itineraries"})["itineraries"]
                        assert len(items)==1
                        saved=call("/itineraries/"+items[0]["id"])
                        assert saved["revision"]==1
                        keys=("id","consentRevision","catalogVersion","expiresAt","budgetUnit","stops")
                        changes={key:saved["plan"][key] for key in keys}
                        changes.update(expectedRevision=1,budgetLimit="17000")
                        preview=call("/itineraries/preview",changes)
                        updated=call("/itineraries/apply",{**changes,"requestKey":str(uuid.uuid4()),"confirmedPreview":preview["confirmation"]})
                        assert updated["revision"]==2
                        self.send_response(200);self.send_header("Content-Length","1");self.end_headers();self.wfile.write(b"2")
                    except Exception:
                        self.send_error(409)
                    return
                if self.path!='/enroll':self.send_error(404);return
                try:
                    encoded=self.rfile.read(min(int(self.headers.get('Content-Length',0)),2048))
                    public=base64.b64decode(encoded,validate=True)
                    if len(public)!=91: self.send_error(400);return
                    (work/'client.der').write_bytes(public)
                    run('openssl','pkey','-pubin','-inform','DER','-in',work/'client.der','-out',work/'client.pub',env=env)
                    (work/'client.conf').write_text('extendedKeyUsage=clientAuth\nkeyUsage=digitalSignature\n')
                    run('openssl','x509','-new','-force_pubkey',work/'client.pub','-subj','/CN=synthetic customer - no commerce',
                        '-CA',work/'ca.pem','-CAkey',work/'ca.key','-CAcreateserial','-days','1','-extfile',work/'client.conf','-out',work/'client.pem',env=env)
                    client=check_output(['openssl','x509','-in',work/'client.pem','-outform','DER'], env=env)
                    expiry=int(time.time())+600
                    grants={hashlib.sha256(client).hexdigest():{'subject':actor,'role':'private-user','expiresAt':expiry}}
                    if args.conflict:
                        run("openssl","req","-newkey","ec","-pkeyopt","ec_paramgen_curve:P-256","-nodes",
                            "-keyout",work/"peer.key","-out",work/"peer.csr","-subj","/CN=Synthetic second client",env=env)
                        run("openssl","x509","-req","-in",work/"peer.csr","-CA",work/"ca.pem","-CAkey",work/"ca.key",
                            "-CAcreateserial","-days","1","-extfile",work/"client.conf","-out",work/"peer.pem",env=env)
                        peer_der=check_output(["openssl","x509","-in",work/"peer.pem","-outform","DER"], env=env)
                        grants[hashlib.sha256(peer_der).hexdigest()]={"subject":actor,"role":"private-user","expiresAt":expiry}

                    (work/'grants.json').write_text(json.dumps(grants))
                    with (work/'recordsd.log').open('ab') as log:
                        service.start([work/'recordsd','-listen',f'127.0.0.1:{args.records_port}','-cert',work/'server.pem','-key',
                            work/'server.key','-client-ca',work/'ca.pem','-grants',work/'grants.json'],cwd=Path.cwd(),
                            env={**env,'P09_PRIVATE_DATABASE_URL':dsn+' user=privateios','P09_CONTENT_KEY':str(secret),
                              'P09_APPROVED_CATALOG_DATABASE_URL':catalog_dsn+' user=travelread',
                              'P09_APPROVED_CATALOG_TRUST_KEY':str(archive/'public.hex'),
                              'P09_LOCAL_AI_ENDPOINT':rank_endpoint,
                              'P09_LOCAL_AI_MODEL':rank_model},stdout=log,stderr=log)
                    profile={'endpoint':records_origin+'/v1/private','pin':hashlib.sha256(serverSPKI).hexdigest(),
                        'ca':base64str(caDER),'certificates':[base64str(client)],
                        'scope':{'role':'private-user','expiresAt':expiry},
                        'policies':[{'purpose':'voice','version':1,'recipient':'p09-sync','maxLifetimeSeconds':1200,'targetRequired':False},
                                    {'purpose':'transcription','version':1,'recipient':'fixture-transcriber','maxLifetimeSeconds':1200,'targetRequired':True}, {'purpose':'recommendation','version':1,'recipient':'p09','maxLifetimeSeconds':recommendation_lifetime,'targetRequired':False}]}
                    body=json.dumps(profile,separators=(',',':')).encode()
                    self.send_response(200);self.send_header('Content-Type','application/json')
                    self.send_header('Content-Length',str(len(body)));self.end_headers();self.wfile.write(body)
                except Exception:
                    self.send_error(500)
        args.work_file.write_text(str(work))
        print('PRIVATE ENROLLMENT READY',flush=True)
        http.server.HTTPServer(('127.0.0.1',args.enrollment_port),Enroll).serve_forever()

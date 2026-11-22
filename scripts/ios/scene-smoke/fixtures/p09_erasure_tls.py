import argparse
import base64, hashlib, http.server, json, os, sys, time
from pathlib import Path
sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parents[3]))
from integration_workspace import OwnedProcess, check_output, cleanup_stack, integration_workspace, start_postgres
from test_travel_booking_integration import run, free_loopback_port
base64str=lambda b:base64.b64encode(b).decode()
actor='a4aa517d-b258-4f88-bea1-00630c790cd5'
observer='f096ef18-bd37-49ab-aec8-d6b621369576'
parser=argparse.ArgumentParser(description='Disposable T12 PostgreSQL and iOS certificate fixture')
parser.add_argument('--work-file',type=Path,required=True)
args=parser.parse_args()
grants={}
base = {k: v for k, v in os.environ.items() if not k.startswith("PG")}
pg = os.environ.get("POSTGRES_BIN", "/opt/homebrew/opt/postgresql@15/bin")
base["PATH"] = pg + os.pathsep + base["PATH"]
with integration_workspace("nu54-private-ios-", pg + "/pg_ctl", "pg", base, term_status=0) as (work, env):
    port = str(free_loopback_port())
    env.update(PGPORT=port, PGHOST=str(work))
    run(pg + "/initdb", "-D", work / "pg", "-A", "trust", "--no-locale", "--encoding=UTF8", env=env)
    start_postgres(pg + "/pg_ctl", work / "pg", work / "postgres.log", f"-k {work} -p {port} -c listen_addresses=''", env)
    service = OwnedProcess()
    with cleanup_stack() as stack:
        stack.callback(service.stop)
        db='nu54_p09_private_ios'
        dsn=f'host={work} port={port} dbname={db}'
        run(pg+'/createdb',db,env=env)
        run('python3','products/p10-platform/database/migrate.py','p09',env={**env,'P09_MIGRATION_DATABASE_URL':dsn})
        catalog_dsn=f'host={work} port={port} dbname=nu54_p09_private_catalog'
        run(pg+'/createdb','nu54_p09_private_catalog',env=env)
        run('python3','products/p10-platform/database/migrate.py','p09',env={**env,'P09_MIGRATION_DATABASE_URL':catalog_dsn})
        run(pg+'/psql','-Xv','ON_ERROR_STOP=1',dsn,'-c',
            "CREATE ROLE privateios LOGIN IN ROLE nu54_p09_private_user; INSERT INTO travel_private.policy VALUES "
            "('voice',1,'p09-sync',gen_random_uuid(),1200,false,true),('transcription',1,'fixture-transcriber',gen_random_uuid(),1200,true,true),('recommendation',1,'p09',gen_random_uuid(),172800,false,true);",env=env)
        run(pg+'/psql','-Xv','ON_ERROR_STOP=1',dsn,'-c',"CREATE ROLE localeraseworker LOGIN IN ROLE nu54_p09_erasure_worker;\nCREATE ROLE localeraseappend LOGIN IN ROLE nu54_p09_erasure_append;\nSELECT travel_private.decide('a4aa517d-b258-4f88-bea1-00630c790cd5','voice',0,1,'p09-sync',NULL,true,now()+interval '1100 seconds',gen_random_uuid());\nSELECT travel_private.write_record('a4aa517d-b258-4f88-bea1-00630c790cd5','3b34aacb-c51d-4470-8204-2ba911aaa901','voice',1,'p09-sync',NULL,now()+interval '600 seconds',decode(repeat('00',32),'hex'),decode(repeat('01',35),'hex'));\nSELECT travel_private.write_record('a4aa517d-b258-4f88-bea1-00630c790cd5','3b34aacb-c51d-4470-8204-2ba911aaa902','voice',1,'p09-sync',NULL,now()+interval '1000 seconds',decode(repeat('00',32),'hex'),decode(repeat('02',35),'hex'));",env=env)
        run(pg+'/psql','-Xv','ON_ERROR_STOP=1',catalog_dsn,'-c',"INSERT INTO travel_erasure.namespace(namespace) VALUES(gen_random_uuid())",env=env)
        run('go','build','-o',work/'erase-worker','./products/p09-travel-ai/cmd/erase-worker',env=env)
        worker = OwnedProcess()
        stack.callback(worker.stop)
        with (work/'erase.log').open('ab') as workerlog:
            worker.start([work/'erase-worker'],env={**env,'P09_ERASURE_WORKLOAD_DATABASE_URL':dsn+' user=localeraseworker','P09_ERASURE_WRITE_DATABASE_URL':catalog_dsn+' user=localeraseappend'},stdout=workerlog,stderr=workerlog)
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
        class Enroll(http.server.BaseHTTPRequestHandler):
            def do_POST(self):
                if self.path not in ('/enroll','/enroll-peer','/enroll-observer'):
                    self.send_error(404);return
                subject=observer if self.path=='/enroll-observer' else actor
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
                    grants[hashlib.sha256(client).hexdigest()]={'subject':subject,'role':'private-user','expiresAt':expiry}
                    (work/'grants.tmp').write_text(json.dumps(grants))
                    (work/'grants.tmp').replace(work/'grants.json')
                    if service.child is None:
                        with (work/'recordsd.log').open('ab') as log:
                            service.start([work/'recordsd','-listen','127.0.0.1:64141','-cert',work/'server.pem','-key',
                            work/'server.key','-client-ca',work/'ca.pem','-grants',work/'grants.json'],cwd=Path.cwd(),
                            env={**env,'P09_PRIVATE_DATABASE_URL':dsn+' user=privateios','P09_CONTENT_KEY':str(secret)},stdout=log,stderr=log)
                    profile={'endpoint':'https://127.0.0.1:64141/v1/private','pin':hashlib.sha256(serverSPKI).hexdigest(),
                        'ca':base64str(caDER),'certificates':[base64str(client)],
                        'scope':{'role':'private-user','expiresAt':expiry},
                        'policies':[{'purpose':'voice','version':1,'recipient':'p09-sync','maxLifetimeSeconds':1200,'targetRequired':False},
                                    {'purpose':'transcription','version':1,'recipient':'fixture-transcriber','maxLifetimeSeconds':1200,'targetRequired':True}, {'purpose':'recommendation','version':1,'recipient':'p09','maxLifetimeSeconds':172800,'targetRequired':False}]}
                    body=json.dumps(profile,separators=(',',':')).encode()
                    self.send_response(200);self.send_header('Content-Type','application/json')
                    self.send_header('Content-Length',str(len(body)));self.end_headers();self.wfile.write(body)
                except Exception:
                    self.send_error(500)
        args.work_file.write_text(str(work))
        server = http.server.HTTPServer(('127.0.0.1',64142),Enroll)
        print('PRIVATE ENROLLMENT READY',flush=True)
        server.serve_forever()

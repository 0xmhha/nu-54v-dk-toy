import http.server, os, ssl, subprocess, sys, tempfile, urllib.request
from pathlib import Path
from urllib.error import HTTPError
sys.path.insert(0,str(Path.cwd()/'scripts'))
from test_travel_booking_integration import run, pg, free_loopback_port, published_fixture

with tempfile.TemporaryDirectory(prefix='nu54-travel-ios-fixture-',dir='/tmp') as local:
    work=Path(local)
    port=str(free_loopback_port())
    env={key:value for key,value in os.environ.items() if not key.startswith('PG')}
    env.update(PGPORT=port,PGHOST=local)
    pg_bin=os.environ.get('POSTGRES_BIN','/opt/homebrew/opt/postgresql@15/bin')
    env['PATH']=pg_bin+os.pathsep+env['PATH']
    ctl=str(Path(pg_bin)/'pg_ctl')
    run(str(Path(pg_bin)/'initdb'),'-D',work/'pg','-A','trust','--no-locale','--encoding=UTF8',env=env)
    run(ctl,'-D',work/'pg','-l',work/'postgres.log','-o',f"-k {work} -p {port} -c listen_addresses=''",'-w','start',env=env)
    try:
        database='nu54_p09_travel_ios'
        dsn=f'host={work} port={port} dbname={database}'
        run('createdb',database,env=env)
        run('python3','products/p10-platform/database/migrate.py','p09',env={**env,'P09_MIGRATION_DATABASE_URL':dsn})
        run('openssl','req','-x509','-newkey','rsa:2048','-nodes','-keyout',work/'ca.key','-out',work/'ca.pem',
            '-days','1','-subj','/CN=NU54 Disposable Travel Acceptance','-addext','basicConstraints=critical,CA:TRUE',
            '-addext','keyUsage=critical,keyCertSign,cRLSign',env=env)
        run('openssl','req','-newkey','rsa:2048','-nodes','-keyout',work/'server.key','-out',work/'server.csr',
            '-subj','/CN=127.0.0.1',env=env)
        (work/'san.conf').write_text('subjectAltName=IP:127.0.0.1\nextendedKeyUsage=serverAuth\nkeyUsage=digitalSignature,keyEncipherment\n')
        run('openssl','x509','-req','-in', work/'server.csr','-CA',work/'ca.pem','-CAkey',work/'ca.key',
            '-CAcreateserial','-extfile',work/'san.conf','-out',work/'server.pem','-days','1',env=env)
        with published_fixture(work,env,dsn) as public_url:
            class Proxy(http.server.BaseHTTPRequestHandler):
                def do_GET(self):
                    if self.headers.get('Authorization') or self.headers.get('Cookie'):
                        self.send_error(400)
                        return
                    if self.path.startswith('/v1/map/'):
                        self.send_error(404)
                        return
                    try:
                        with urllib.request.urlopen(public_url+self.path,timeout=3) as f:
                            payload=f.read()
                            self.send_response(f.status)
                            self.send_header('Content-Type',f.headers['Content-Type'])
                            self.send_header('Content-Length',str(len(payload)))
                            self.end_headers()
                            self.wfile.write(payload)
                    except HTTPError as error:
                        self.send_error(error.code)
            server=http.server.ThreadingHTTPServer(('127.0.0.1',65140),Proxy)
            tls=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
            tls.minimum_version=ssl.TLSVersion.TLSv1_3
            tls.load_cert_chain(work/'server.pem',work/'server.key')
            server.socket=tls.wrap_socket(server.socket,server_side=True)
            Path('/tmp/nu54-p09-native-fixture.path').write_text(local)
            print('ROUTE READY '+local,flush=True)
            server.serve_forever()
    finally:
        subprocess.run([ctl,'-D',work/'pg','-m','fast','-w','stop'],env=env)

#!/bin/bash
# Synthetic two-identity fixture for the installed K06/K12 simulator tests.
set -euo pipefail
exec 9>&2
if [[ $# != 3 ]]; then
  echo 'usage: p05_kiosk_mode_native.sh SCRATCH_ROOT TERMINAL_SPKI_DER OPERATOR_SPKI_DER' >&2
  exit 2
fi
mkdir -p "$1"
lab=$(cd -- "$1" && pwd)
[[ "$lab" = /private/tmp/* || "$lab" = /tmp/* ]]
offline="${NU54_KIOSK_OFFLINE_FIXTURE:-0}"
origin_port="${NU54_KIOSK_ORIGIN_PORT:-18866}"
terminal_spki="$2"
operator_spki="$3"
for key in "$terminal_spki" "$operator_spki"; do
  [[ "$(wc -c < "$key" | tr -d ' ')" = 91 ]]
done
# Inherited deployment settings must never become part of this fixture.
while IFS= read -r name; do
  case "$name" in P05_*|PG*) unset "$name";; esac
done < <(compgen -A variable)
# Record interruptions until the newly allocated path is known to the owner.
deferred=
defer_signal() { if [[ -z "$deferred" ]]; then deferred="$1"; fi; }
begin_deferred() {
  deferred=
  trap 'defer_signal 0' INT TERM
  trap 'defer_signal 129' HUP
}
end_deferred() {
  trap 'exit 0' INT TERM
  trap 'exit 129' HUP
  if [[ -n "$deferred" ]]; then exit "$deferred"; fi
}
begin_deferred
dir=$(mktemp -d "$lab/fixture.XXXXXX")
socket="$dir/pg"
db=nu54_commerce_k06
writer=k06_http_fixture
ops=
controller=
owned_ready=ready
if [[ "$offline" == 1 ]]; then owned_ready=offline-ready.json; fi
stop_child() {
  [[ -n "$1" ]] || return 0
  kill -TERM "$1" 2>/dev/null || :
  wait "$1" 2>/dev/null || :
}
stop_database() {
  local status=0 mode
  if [[ ! -e "$dir/pgdata/PG_VERSION" && ! -e "$dir/pgdata/postmaster.pid" ]]; then
    return 0
  fi
  pg_ctl -D "$dir/pgdata" status >/dev/null 2>&1 || status=$?
  if ((status == 3)); then return 0; fi
  for mode in fast immediate; do
    pg_ctl -D "$dir/pgdata" -m "$mode" -w -t 10 stop >/dev/null 2>&1 || :
    status=0
    pg_ctl -D "$dir/pgdata" status >/dev/null 2>&1 || status=$?
    if ((status == 3)); then return 0; fi
  done
  return 1
}
cleanup() {
  local status=$?
  trap - EXIT
  trap '' HUP INT TERM
  stop_child "$controller"
  stop_child "$ops"
  if [[ ! -L "$lab/$owned_ready" && "$lab/$owned_ready" -ef "$dir/$owned_ready" ]]; then
    rm -f -- "$lab/$owned_ready" || { if ((status == 0)); then status=1; fi; }
  fi
  if stop_database; then
    rm -rf -- "$dir" || { if ((status == 0)); then status=1; fi; }
  else
    echo "Could not confirm PostgreSQL shutdown. Retained fixture at $dir" >&9
    if ((status == 0)); then status=1; fi
  fi
  exit "$status"
}
trap cleanup EXIT
end_deferred
launch_child() {
  local pid_field=$1
  shift
  begin_deferred
  "$@" <&0 &
  printf -v "$pid_field" '%s' "$!"
  end_deferred
}
mkdir -p "$socket" "$dir/tmp" "$dir/go-cache" "$dir/go-tmp"
export PGUSER=k06_fixture PGHOST="$socket" PGPORT=55487 PGSSLMODE=disable
export TMPDIR="$dir/tmp" GOCACHE="$dir/go-cache" GOTMPDIR="$dir/go-tmp" PYTHONDONTWRITEBYTECODE=1
publish_ready() {
  # A hard link publishes the complete marker without replacing caller data.
  python3 -B - "$dir/$owned_ready" "$lab/$owned_ready" <<'PYLINK'
import os
import sys

os.link(sys.argv[1], sys.argv[2])
PYLINK
}
initdb -D "$dir/pgdata" -A trust --no-locale --encoding=UTF8 -U "$PGUSER" >/dev/null
pg_ctl -D "$dir/pgdata" -l "$dir/postgres.log" -o "-p $PGPORT -c listen_addresses='' -c unix_socket_directories=$socket -c unix_socket_permissions=0700" -w start
createdb "$db"
P05_MIGRATION_DATABASE_URL="$db" make -C products/p05-operations-backoffice migrate > "$dir/migrations.log"
psql -X "$db" -q -v ON_ERROR_STOP=1 <<'SQL'
CREATE ROLE k06_http_fixture LOGIN IN ROLE nu54_p05_commerce;
INSERT INTO ledger.branch VALUES ('5dc8d4be-c24d-4553-ab79-9c30f630e477','80d39491-fa9b-4f35-9c2a-f6a258dd45d6','Asia/Seoul','04:00');
INSERT INTO ledger.login_subject VALUES ('k06_iwa_operator','94001137-522a-44b9-b8b4-e3fceff2a920');
INSERT INTO ledger.staff_grant VALUES ('5dc8d4be-c24d-4553-ab79-9c30f630e477','94001137-522a-44b9-b8b4-e3fceff2a920','operator',now()-interval '1 min',NULL);
INSERT INTO ledger.commerce_grant VALUES ('5dc8d4be-c24d-4553-ab79-9c30f630e477','3ce37c55-4f32-433a-9807-2bef754496b5','terminal','menu',now()+interval '1 hour',1);
INSERT INTO ledger.command_rule VALUES ('5dc8d4be-c24d-4553-ab79-9c30f630e477','local K06','fixture','dispose',interval '15 minutes');
INSERT INTO ledger.menu_release VALUES ('5dc8d4be-c24d-4553-ab79-9c30f630e477',1,now()-interval '1 minute',now()+interval '40 minutes');
INSERT INTO ledger.finance_grant VALUES ('5dc8d4be-c24d-4553-ab79-9c30f630e477','5de83ac7-c8ab-4015-b2a7-b929a33dad2c','pickup_manage',now()+interval '1 hour');
WITH originals AS (
  INSERT INTO ledger.sale(id,branch_id,origin,origin_ref,asset_ref,amount)
    SELECT ('80000000-0000-4000-8000-'||lpad(number::text,12,'0'))::uuid,'5dc8d4be-c24d-4553-ab79-9c30f630e477',
      'menu',gen_random_uuid(),'0x2222222222222222222222222222222222222222',7000000
    FROM generate_series(1,21) AS ids(number) RETURNING id,branch_id
)
INSERT INTO ledger.order_checkout SELECT id,branch_id,'3ce37c55-4f32-433a-9807-2bef754496b5',1,now()+interval '40 minutes',sha256('K06 original') FROM originals;
SQL
if [[ "$offline" == 1 ]]; then
  psql -X "$db" -q -v ON_ERROR_STOP=1 <<'SQLMENU'
INSERT INTO ledger.operator_branch_label VALUES ('5dc8d4be-c24d-4553-ab79-9c30f630e477','여울 커피 성수점');
INSERT INTO ledger.stock VALUES ('5dc8d4be-c24d-4553-ab79-9c30f630e477','a-drink',2),
 ('5dc8d4be-c24d-4553-ab79-9c30f630e477','z-bread',0);
SQLMENU
fi

openssl req -x509 -new -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -noenc -days 1 -subj /CN=NU54-K06-Fixture -keyout "$dir/ca.key" -out "$dir/ca.pem" >/dev/null 2>&1
openssl req -new -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -noenc -subj /CN=NU54-K06-Server -keyout "$dir/server.key" -out "$dir/server.csr" >/dev/null 2>&1
printf 'extendedKeyUsage=serverAuth\nsubjectAltName=IP:127.0.0.1\n' > "$dir/server.ext"
printf 'extendedKeyUsage=clientAuth\n' > "$dir/client.ext"
openssl x509 -req -in "$dir/server.csr" -CA "$dir/ca.pem" -CAkey "$dir/ca.key" -CAcreateserial -days 1 -extfile "$dir/server.ext" -out "$dir/server.pem"
for role in terminal operator clerk; do
  key="$operator_spki"
  [[ "$role" != terminal ]] || key="$terminal_spki"
  openssl pkey -pubin -inform DER -in "$key" -out "$dir/$role.pub"
  openssl x509 -req -in "$dir/server.csr" -force_pubkey "$dir/$role.pub" -CA "$dir/ca.pem" -CAkey "$dir/ca.key" -CAserial "$dir/ca.srl" -days 1 -extfile "$dir/client.ext" -out "$dir/$role.pem"
done
python3 - "$dir" "$origin_port" "$offline" <<'PY'
from pathlib import Path
import base64, hashlib, json, ssl, subprocess, sys, time
d=Path(sys.argv[1])
branch='5dc8d4be-c24d-4553-ab79-9c30f630e477'
tenant='80d39491-fa9b-4f35-9c2a-f6a258dd45d6'
terminal='3ce37c55-4f32-433a-9807-2bef754496b5'
issuer='94001137-522a-44b9-b8b4-e3fceff2a920'
def der(name):
    return ssl.PEM_cert_to_DER_cert((d/(name+'.pem')).read_text())
pem=subprocess.check_output(['openssl','x509','-in',str(d/'server.pem'),'-pubkey','-noout'])
pin=hashlib.sha256(subprocess.check_output(['openssl','pkey','-pubin','-outform','DER'],input=pem)).hexdigest()
grants={}
for name, role, subject in [('terminal','terminal',terminal),('operator','operator',issuer),('clerk','operator','5de83ac7-c8ab-4015-b2a7-b929a33dad2c')]:
    scope=dict(tenantRef=tenant,branchRef=branch,role=role,subjectRef=subject,expiresAt=int(time.time())+2700)
    grants[hashlib.sha256(der(name)).hexdigest()]={k:v for k,v in scope.items() if k!='branchRef'}
    profile=dict(endpoint=f'https://127.0.0.1:{sys.argv[2]}/v2/commerce',pin=pin,ca=base64.b64encode(der('ca')).decode(),
        certificates=[base64.b64encode(der(name)).decode()],scope=scope)
    if name == 'terminal' and sys.argv[3] == '1':
        profile['payment'] = {'chain':8283,'contract':'0x'+'1'*40,'asset':'0x'+'2'*40,
            'beneficiary':'0x'+'3'*40,'signer':'0x'+'4'*40,'merchantId':'a'*64,'keyVersion':1,
            'beneficiaryVersion':1,'maxBaseUnits':'30000000'}
    expected={'clerk':['pickup'],'operator':['terminal_settings']}.get(name)
    (d/(name+'.json')).write_text(json.dumps(dict(profile=profile,terminal=terminal,staffPermissions=expected)))
asset='0x'+'2'*40
(d/'grants.json').write_text(json.dumps(grants))
(d/'profiles.json').write_text(json.dumps({branch:{'chainId':8283,'contract':'0x'+'1'*40,'asset':asset,'beneficiary':'0x'+'3'*40,'signer':'0x'+'4'*40,'merchantId':'a'*64,'keyVersion':1,'beneficiaryVersion':1}}))
PY
openssl rand -hex 32 > "$dir/commitment"
openssl rand -hex 32 > "$dir/storage"
go build -o "$dir/opsd" ./products/p05-operations-backoffice/cmd/opsd
dsn="host=$socket port=$PGPORT user=$PGUSER dbname=$db sslmode=disable"
export P05_HTTP_ADDR=127.0.0.1:$origin_port P05_COMMERCE_DATABASE_URL="${dsn/user=$PGUSER/user=$writer}"
export P05_COMMERCE_CERT="$dir/server.pem" P05_COMMERCE_KEY="$dir/server.key" P05_COMMERCE_CA="$dir/ca.pem"
export P05_COMMERCE_GRANTS="$dir/grants.json" P05_COMMERCE_PROFILES="$dir/profiles.json"
export P05_QUOTE_COMMITMENT_KEY_FILE="$dir/commitment" P05_QUOTE_STORAGE_KEY_FILE="$dir/storage"
export P05_QUOTE_SIGNER=/usr/bin/false P05_QUOTE_SIGNER_SHA256="$(shasum -a 256 /usr/bin/false | cut -d' ' -f1)"
if [[ "$offline" == 1 ]]; then
  launch_child controller python3 scripts/ios/scene-smoke/fixtures/p05_kiosk_offline.py "$dir" "$dsn" "$origin_port" "$dir/$owned_ready"
  for attempt in {1..300}; do
    [[ ! -s "$dir/$owned_ready" ]] || break
    if ! kill -0 "$controller" 2>/dev/null; then
      wait "$controller" || exit $?
      exit 1
    fi
    sleep 0.1
  done
  test -s "$dir/$owned_ready"
  publish_ready
  wait "$controller"
  exit
fi
launch_child ops "$dir/opsd" > "$dir/server.log" 2>&1
for attempt in {1..30}; do
  kill -0 "$ops"
  if nc -z 127.0.0.1 "$origin_port"; then break; fi
  sleep 0.15
done
kill -0 "$ops"
nc -z 127.0.0.1 "$origin_port"
printf '%s\n' "$dsn" > "$dir/db-dsn"
launch_child controller python3 - "$dir/db-dsn" <<'PYCONTROL' > "$dir/control.log" 2>&1
from pathlib import Path
import subprocess
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

dsn = Path(sys.argv[1]).read_text().strip()
query = """SELECT terminal_mode,mode_revision,
  (SELECT count(*) FROM ledger.terminal_transition),
  (SELECT count(*) FROM ledger.terminal_transition WHERE handoff_at IS NOT NULL),
  (SELECT count(*) FROM ledger.terminal_transition WHERE ack_at IS NOT NULL),
  (SELECT count(*) FROM ledger.terminal_handoff_order),
  (SELECT count(*) FROM ledger.sale)
FROM ledger.commerce_grant WHERE role_code='terminal'"""
class Control(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != '/state':
            self.send_error(404)
            return
        result = subprocess.check_output(['psql', '-XAt', dsn, '-c', query]).strip()
        self.send_response(200)
        self.end_headers()
        self.wfile.write(result)
HTTPServer(('127.0.0.1', 18966), Control).serve_forever()
PYCONTROL
for attempt in {1..30}; do
  kill -0 "$controller"
  if curl --silent --fail http://127.0.0.1:18966/state >/dev/null; then break; fi
  sleep 0.15
done
kill -0 "$controller"
curl --silent --fail http://127.0.0.1:18966/state >/dev/null
printf '%s\n' "$dir" > "$dir/$owned_ready"
publish_ready
wait "$ops"

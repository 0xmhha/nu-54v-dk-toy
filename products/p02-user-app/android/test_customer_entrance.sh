#!/usr/bin/env bash
set -euo pipefail

# The customer entrance transport on an isolated emulator: a local TLS 1.3 server that asks for a client
# certificate and reports whether it got one, a disposable CA, and the shipping CommerceHttp.customerEntrance.
cd "$(dirname "$0")"
adb_bin="${ANDROID_HOME:?Set ANDROID_HOME}/platform-tools/adb"
serial="${1:?Pass the serial of an isolated Android emulator}"
[[ "$serial" =~ ^emulator-[0-9]+$ ]]
scratch="$(mktemp -d)"
server=""
port=""
cleanup() {
  failure="$?"
  if [[ -n "$server" ]]; then kill "$server" 2>/dev/null || true; fi
  if [[ -n "$port" ]]; then "$adb_bin" -s "$serial" reverse --remove "tcp:$port" >/dev/null 2>&1 || true; fi
  if [[ "$failure" != 0 && -f "$scratch/server.log" ]]; then cat "$scratch/server.log" >&2; fi
  rm -rf "$scratch"
}
trap cleanup EXIT

runner=com.nu54renter.ble.CustomerEntranceLab
./gradlew --no-daemon -PreactNativeArchitectures=arm64-v8a -Pnu54LabRunner="$runner" :app:assembleDebug :app:assembleDebugAndroidTest
"$adb_bin" -s "$serial" install -r app/build/outputs/apk/debug/app-debug.apk
"$adb_bin" -s "$serial" install -r app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk
instrumentation="com.nu54renter.test/$runner"
"$adb_bin" -s "$serial" shell am instrument -w -e lab key "$instrumentation" | tee "$scratch/key.log"
sed -n 's/^INSTRUMENTATION_RESULT: commerce_spki=//p' "$scratch/key.log" | tr -d '\r\n' |
  openssl base64 -A -d | openssl pkey -inform DER -pubin >"$scratch/customer.pub"
for name in ca other; do
  openssl req -new -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -noenc -days 1 \
    -subj "/CN=NU54-Entrance-Lab-$name" -keyout "$scratch/$name.key" -out "$scratch/$name.pem" 2>/dev/null
done
openssl req -new -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -noenc -subj /CN=127.0.0.1 \
  -keyout "$scratch/point.key" -out "$scratch/point.csr" 2>/dev/null
printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=serverAuth\nsubjectAltName=IP:127.0.0.1\n' >"$scratch/server.ext"
printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=clientAuth\n' >"$scratch/customer.ext"
openssl x509 -req -in "$scratch/point.csr" -CA "$scratch/ca.pem" -CAkey "$scratch/ca.key" -CAcreateserial -days 1 \
  -extfile "$scratch/server.ext" -out "$scratch/point.pem" 2>/dev/null
openssl x509 -new -force_pubkey "$scratch/customer.pub" -subj /CN=NU54-Lab-Customer -CA "$scratch/ca.pem" \
  -CAkey "$scratch/ca.key" -CAcreateserial -days 1 -extfile "$scratch/customer.ext" -out "$scratch/customer.pem" 2>/dev/null
python3 - "$scratch" >"$scratch/server.log" 2>&1 <<'PY' &
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
import json, ssl, sys

files = Path(sys.argv[1])

class Entrance(BaseHTTPRequestHandler):
    def answer(self, status, body, location=None):
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        if location:
            self.send_header("Location", location)
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        sent = self.rfile.read(int(self.headers["Content-Length"]))
        if self.path == "/v2/customer/challenge" and sent == b'{"synthetic":true}':
            # The enrolled customer certificate would verify here; the entrance transport must not offer it.
            seen = {"clientCertificate": bool(self.connection.getpeercert()), "tls": self.connection.version()}
            self.answer(200, json.dumps(seen).encode())
        elif self.path == "/v2/customer/proof" and sent == b'{"busy":true}':
            self.answer(429, b'{"status":"error","error":"BUSY"}')
        elif self.path == "/v2/customer/proof" and sent == b'{"moved":true}':
            self.answer(307, b'{}', "/v2/customer/challenge")
        else:
            self.answer(404, b'{}')

context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
context.minimum_version = ssl.TLSVersion.TLSv1_3
context.verify_mode = ssl.CERT_OPTIONAL
context.load_verify_locations(files / "ca.pem")
context.load_cert_chain(files / "point.pem", files / "point.key")
class TLSHTTPServer(HTTPServer):
    def get_request(self):
        raw, remote = super().get_request()
        try:
            return context.wrap_socket(raw, server_side=True), remote
        except ssl.SSLError as error:
            print(f"TLS lab negotiation: {error}", file=sys.stderr, flush=True)
            raw.close()
            raise

server = TLSHTTPServer(("127.0.0.1", 0), Entrance)
(files / "port").write_text(str(server.server_port))
server.serve_forever()
PY
server=$!
for _ in {1..25}; do if [[ -s "$scratch/port" ]]; then break; fi; sleep 0.1; done
port="$(cat "$scratch/port")"
"$adb_bin" -s "$serial" reverse "tcp:$port" "tcp:$port"
pin="$(openssl x509 -in "$scratch/point.pem" -pubkey -noout | openssl pkey -pubin -outform DER | shasum -a 256 | cut -d' ' -f1)"
ca="$(openssl x509 -in "$scratch/ca.pem" -outform DER | openssl base64 -A)"
other="$(openssl x509 -in "$scratch/other.pem" -outform DER | openssl base64 -A)"
cert="$(openssl x509 -in "$scratch/customer.pem" -outform DER | openssl base64 -A)"
profile="$(python3 - "$port" "$pin" "$ca" "$cert" <<'PY'
import base64,json,sys,time
port,pin,ca,cert=sys.argv[1:]
# The enrolled commerce endpoint is another port of the same server; the entrance borrows only its CA and pin.
raw={"endpoint":"https://127.0.0.1:9/v2/commerce","pin":pin,"ca":ca,"certificates":[cert],
     "scope":{"role":"customer","expiresAt":int(time.time())+180}}
print(base64.b64encode(json.dumps(raw).encode()).decode())
PY
)"
"$adb_bin" -s "$serial" shell am instrument -w -e lab entrance -e port "$port" -e profile "$profile" -e otherCa "$other" \
  "$instrumentation" | tee "$scratch/receipt.log"
for line in customer_entrance_lab=pass entrance_client_certificate=none entrance_redirect=reject entrance_pin=reject \
  entrance_ca=reject entrance_host=reject entrance_scope=reject; do
  grep -q "^INSTRUMENTATION_RESULT: $line" "$scratch/receipt.log"
done

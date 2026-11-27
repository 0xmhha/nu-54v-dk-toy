#!/usr/bin/env bash
# Add TLS to a running disposable P08 mobile fixture. App trust is limited to
# the supplied QA CA. The proxy, certificate material and profile are temporary.
set -euo pipefail
: "${P08_PROFILE:?start mobile-fixture.sh first}"
marketd="${P08_MOBILE_FIXTURE_MARKETD:?start mobile-fixture.sh first}"
work=$(mktemp -d)
bridge=
cleanup() {
  if [[ -n "$bridge" ]]; then kill "$bridge" 2>/dev/null || true; wait "$bridge" || true; fi
  rm -rf "$work"
}
trap cleanup EXIT
umask 077
host=p08.localtest.me
port=9443
listen_port=$port
readiness="${NU54_MOBILE_QA_PORT_FILE:-$work/listen-port}"
if [[ -n "${NU54_MOBILE_QA_PORT_FILE:-}" ]]; then
  listen_port=0
fi
python3 - "$host" <<'PY'
import ipaddress, socket, sys
addresses = socket.getaddrinfo(sys.argv[1], None, type=socket.SOCK_STREAM)
assert addresses and all(ipaddress.ip_address(endpoint[4][0]).is_loopback for endpoint in addresses)
PY
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj '/CN=P08 Disposable Mobile CA' \
  -addext 'basicConstraints=critical,CA:TRUE' -keyout "$work/ca.key" -out "$work/ca.pem" >/dev/null 2>&1
openssl req -new -newkey rsa:2048 -nodes -subj "/CN=$host" -keyout "$work/leaf.key" -out "$work/leaf.csr" >/dev/null 2>&1
printf 'subjectAltName=DNS:%s\nextendedKeyUsage=serverAuth\nbasicConstraints=CA:FALSE\n' "$host" >"$work/leaf.ext"
openssl x509 -req -in "$work/leaf.csr" -CA "$work/ca.pem" -CAkey "$work/ca.key" -CAcreateserial \
  -days 1 -extfile "$work/leaf.ext" -out "$work/leaf.pem" >/dev/null 2>&1
openssl x509 -in "$work/ca.pem" -outform DER -out "$work/ca.der"
export NU54_MOBILE_QA_PROFILE NU54_MOBILE_QA_CA="$work/ca.der"
NU54_MOBILE_QA_PROFILE=$(P08_MOBILE_ORIGIN="https://$host:$port" P08_MOBILE_CA_DER="$work/ca.der" "$marketd" mobile-profile)
python3 - "$listen_port" "$work/leaf.pem" "$work/leaf.key" "$readiness" <<'PY' &
import http.client
import http.server
from pathlib import Path
import ssl
import sys
import tempfile

class Gateway(http.server.BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def relay(self):
        if not self.path.startswith("/v1/"):
            return self.send_error(404)
        size = int(self.headers.get("Content-Length", "0"))
        if not 0 <= size <= 8192:
            return self.send_error(413)
        upstream = http.client.HTTPConnection("127.0.0.1", 8284, timeout=10)
        try:
            body = self.rfile.read(size) if size else None
            upstream.request(self.command, self.path, body, {"Content-Type": "application/json"} if size else {})
            response = upstream.getresponse()
            data = response.read(32769)
            if len(data) > 32768:
                return self.send_error(502)
            self.send_response(response.status)
            self.send_header("Content-Type", response.getheader("Content-Type", "application/json"))
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)
        finally:
            upstream.close()

    do_GET = relay
    do_POST = relay

server = http.server.ThreadingHTTPServer(("127.0.0.1", int(sys.argv[1])), Gateway)
context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
context.minimum_version = ssl.TLSVersion.TLSv1_2
context.load_cert_chain(*sys.argv[2:4])
server.socket = context.wrap_socket(server.socket, server_side=True)
ready = Path(sys.argv[4])
with tempfile.NamedTemporaryFile(mode="w", dir=ready.parent, prefix=".nu54-port-", delete=False) as stream:
    pending = Path(stream.name)
    try:
        stream.write(str(server.server_port) + "\n")
        stream.close()
        pending.replace(ready)
    finally:
        pending.unlink(missing_ok=True)
server.serve_forever()
PY
bridge=$!
for _ in {1..12}; do
  if [[ -s "$readiness" ]] && read -r bridge_port <"$readiness" &&
    curl --noproxy '*' --connect-to "$host:$port:127.0.0.1:$bridge_port" --cacert "$work/ca.pem" \
      -fsS "https://$host:$port/v1/profile" >/dev/null 2>&1; then break; fi
  kill -0 "$bridge"
  sleep 1
done
read -r bridge_port <"$readiness"
curl --noproxy '*' --connect-to "$host:$port:127.0.0.1:$bridge_port" --cacert "$work/ca.pem" \
  -fsS "https://$host:$port/v1/profile" >/dev/null
"$@"

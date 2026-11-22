"""HTTPS test page for the actual platform WebView. Trust its certificate in the QA device only."""
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import ssl
import sys

CLIENT = """
async function record(message) {await fetch('/evidence',{method:'POST',body:message});}
async function run() {
  if(!window.ethereum?.isConnected()) {setTimeout(run,25);return;}
  const account=await window.ethereum.request({method:'eth_requestAccounts',params:[]});
  await record('account:'+account[0]);
  if(location.pathname==='/second') return;
  try {await window.ethereum.request({method:'eth_sign',params:[]});await record('FAIL:unsafe-approved');}
  catch(error) {await record('unsafe-denied:'+error.code);}
  const frame=document.querySelector('iframe');
  if(frame.contentWindow.ethereum) await record('FAIL:iframe-provider');
  else await record('iframe-no-provider');
  const opened=window.open('/popup','_blank');
  if(opened===null) await record('popup-blocked');
  // The certificate also contains localhost, which is deliberately another origin.
  location.href='https://localhost:18448/redirected';
}
run().catch(error=>record('FAIL:'+String(error)));
"""


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path in ("/popup", "/redirected"):
            print("FAIL:loaded"+self.path, flush=True)
        if self.path in ("/native", "/second"):
            markup = '<iframe title="Nested frame" src="/frame"></iframe><script src="/client.js"></script>'
        elif self.path == "/client.js":
            self.send_response(200)
            self.send_header("Content-Type","text/javascript")
            self.end_headers()
            self.wfile.write(CLIENT.encode())
            return
        else:
            markup = "<p>Nested or unknown page</p>"
        self.send_response(200)
        self.send_header("Content-Type","text/html")
        self.end_headers()
        self.wfile.write(markup.encode())

    def do_POST(self):
        length = int(self.headers["Content-Length"])
        if self.path != "/evidence" or not 0 < length <= 160:
            self.send_error(400)
            return
        print(self.rfile.read(length).decode(),flush=True)
        self.send_response(200)
        self.end_headers()

    def log_message(self, *_):
        pass


server = ThreadingHTTPServer(("127.0.0.1",18448),Handler)
tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
tls.load_cert_chain(sys.argv[1], sys.argv[2])
server.socket = tls.wrap_socket(server.socket,server_side=True)
server.serve_forever()

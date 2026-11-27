"""Private control channel for the K08 installed acceptance test."""
import http.server
import json
from pathlib import Path
import secrets
import sys

fixture, output = map(Path, sys.argv[1:])
access = secrets.token_urlsafe(32)


class Stages(http.server.BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def stage(self):
        if self.path != "/stage" or not secrets.compare_digest(self.headers.get("Authorization", ""), "Bearer " + access):
            self.send_error(404)
            return None
        try:
            return (fixture / "stage.json").read_bytes()
        except FileNotFoundError:
            self.send_response(202)
            self.end_headers()
            return None

    def do_GET(self):
        payload = self.stage()
        if payload is not None:
            self.send_response(200)
            self.send_header("Cache-Control", "no-store")
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)

    def do_POST(self):
        payload = self.stage()
        if payload is None:
            return
        name = json.loads(payload)["stage"]
        if self.headers.get("Content-Length") != str(len(name)) or self.rfile.read(len(name)) != name.encode():
            self.send_error(400)
            return
        (fixture / name).touch()
        self.send_response(204)
        self.end_headers()


with http.server.HTTPServer(("127.0.0.1", 0), Stages) as server:
    output.write_text(json.dumps({"origin": f"http://127.0.0.1:{server.server_port}", "token": access}))
    server.serve_forever()

"""Single-use synthetic C3 enrollment exchange for the disposable iOS acceptance."""
import argparse
import base64
import binascii
import http.server
import json
import secrets
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("fixture", type=Path)
    parser.add_argument("ready", type=Path)
    parser.add_argument("--travel", action="store_true")
    parser.add_argument("--single", action="store_true", help="pair this iOS installation with Android")
    args = parser.parse_args()
    roles = {"ios"} if args.single else {"ios", "ios-second"}
    access = secrets.token_urlsafe(32)
    keys = {}
    issued = set()

    class Enrollment(http.server.BaseHTTPRequestHandler):
        def log_message(self, fmt, *arguments):
            pass

        def role(self):
            role = self.path.removeprefix("/")
            if role not in roles or not secrets.compare_digest(self.headers.get("Authorization", ""), "Bearer " + access):
                self.send_response_only(404)
                self.end_headers()
                return None
            return role

        def travel_stage(self):
            if not args.travel or not secrets.compare_digest(
                self.headers.get("Authorization", ""), "Bearer " + access
            ):
                self.send_response_only(404)
                self.end_headers()
                return
            try:
                if not (args.fixture / "catalog-ready").is_file():
                    raise FileNotFoundError()
                stage = (args.fixture / "travel-stage").read_text()
            except FileNotFoundError:
                self.send_response_only(202)
                self.end_headers()
                return
            if stage not in {
                "t06-reserved", "t06-quote", "t06-board-approve", "t06-submitted", "t06-pending",
                "t06-paid", "t06-preparing", "t06-ready", "t06-restarted",
                "booking", "deposit-quote", "deposit-refused", "deposit-board-refuse", "deposit-board-approve",
                "deposit-approved", "deposit-checking", "deposit", "balance-quote",
                "balance-board-refuse", "balance-refused", "balance-board-approve", "balance-approved",
                "balance-checking", "both-paid", "partial", "refunded",
                "cancel-preview", "cancel-requested", "deposit-refunded",
            }:
                self.send_response_only(202)
                self.end_headers()
                return
            if self.command == "POST":
                if self.headers.get("Content-Length") != str(len(stage)) or self.rfile.read(len(stage)) != stage.encode():
                    self.send_response_only(400)
                    self.end_headers()
                    return
                (args.fixture / ("travel-" + stage)).touch()
                self.send_response_only(204)
                self.end_headers()
            else:
                payload = stage.encode()
                self.send_response(200)
                self.send_header("Cache-Control", "no-store")
                self.send_header("Content-Length", str(len(payload)))
                self.end_headers()
                self.wfile.write(payload)

        def do_POST(self):
            if self.path == "/travel-stage":
                self.travel_stage()
                return
            role = self.role()
            if role is None:
                return
            try:
                length = int(self.headers["Content-Length"])
                if not 1 <= length <= 160 or role in keys:
                    raise ValueError()
                public_key = self.rfile.read(length).decode("ascii")
                if len(base64.b64decode(public_key, validate=True)) != 91 or public_key in keys.values():
                    raise ValueError()
            except (KeyError, ValueError, binascii.Error):
                self.send_response_only(400)
                self.end_headers()
                return
            keys[role] = public_key
            if set(keys) == roles:
                staged = args.fixture / "public-keys.tmp"
                staged.write_text(json.dumps(keys))
                staged.replace(args.fixture / "public-keys.json")
            self.send_response_only(204)
            self.end_headers()

        def do_GET(self):
            if self.path == "/travel-stage":
                self.travel_stage()
                return
            if self.path == "/travel-cancellation-terms":
                if not args.travel or not secrets.compare_digest(
                    self.headers.get("Authorization", ""), "Bearer " + access
                ):
                    self.send_error(404)
                    return
                try:
                    payload = (args.fixture / "cancellation-terms.json").read_bytes()
                except FileNotFoundError:
                    self.send_error(404)
                    return
                self.send_response(200)
                self.send_header("Cache-Control", "no-store")
                self.send_header("Content-Length", str(len(payload)))
                self.end_headers()
                self.wfile.write(payload)
                return
            if self.path == "/travel-proof":
                if (not args.travel or
                    not secrets.compare_digest(self.headers.get("Authorization", ""), "Bearer " + access)):
                    self.send_error(404)
                    return
                try:
                    payload = (args.fixture / "original-receipt.json").read_bytes()
                except FileNotFoundError:
                    self.send_error(404)
                    return
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Cache-Control", "no-store")
                self.send_header("Content-Length", str(len(payload)))
                self.end_headers()
                self.wfile.write(payload)
                return
            role = self.role()
            if role is None:
                return
            if role not in keys or role in issued:
                self.send_response_only(404)
                self.end_headers()
                return
            try:
                fixture = json.loads((args.fixture / "preview.json").read_bytes())
                payload = json.dumps({
                    "registration": json.dumps(fixture["profiles"][role]),
                    "tlsPin": fixture["tlsPin"],
                    "presentationPin": fixture["presentationPin"],
                    "saleRef": fixture["invoice"]["saleRef"],
                    "bookingRef": fixture["invoice"].get("bookingRef"),
                    "travelRecipient": fixture["invoice"].get("travelRecipient"),
                }).encode()
            except FileNotFoundError:
                self.send_response_only(202)
                self.end_headers()
                return
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Cache-Control", "no-store")
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)
            issued.add(role)

    with http.server.HTTPServer(("127.0.0.1", 0), Enrollment) as server:
        staged = args.ready.with_suffix(".tmp")
        staged.write_text(json.dumps({"origin": f"http://127.0.0.1:{server.server_port}", "access": access}))
        staged.replace(args.ready)
        server.serve_forever()


if __name__ == "__main__":
    main()

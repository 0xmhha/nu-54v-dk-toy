"""Stop and restart the same isolated P05 service for K01 acceptance."""
import hmac
import json
from pathlib import Path
import secrets
import signal
import socket
import subprocess
import sys
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

root, dsn, origin_port, ready = sys.argv[1:]
root = Path(root)
ready = Path(ready)
token = secrets.token_hex(24)
branch = "5dc8d4be-c24d-4553-ab79-9c30f630e477"
terminal = "3ce37c55-4f32-433a-9807-2bef754496b5"
app = None
state = "online"


def sql(query):
    return subprocess.check_output(["psql", "-XqAt", "-v", "ON_ERROR_STOP=1", dsn, "-c", query], text=True).strip()


def publish(version, price):
    label = {"ko": "아이스 아메리카노", "en": "Iced americano"}
    rows = [
        {"sku": "a-drink", "option": "plain", "label": "Iced americano", "metadata": {"labels": label},
         "displayPrice": price, "price": price},
        {"sku": "z-bread", "option": "plain", "label": "Salt bread",
         "metadata": {"labels": {"ko": "소금빵", "en": "Salt bread"}},
         "displayPrice": "3000000", "price": "3000000"},
    ]
    rows += [
        {"sku": f"item-{number:02d}", "option": "plain", "label": "Today's menu",
         "displayPrice": "3000000", "price": "3000000"}
        for number in range(49)
    ]
    rows.append({"sku": "z-legacy", "option": "plain", "label": "한정 메뉴",
                 "displayPrice": "3000000", "price": "3000000"})
    categories = [
        {"key": "warm", "sortOrder": 1, "labels": {"ko": "음료", "en": "Drinks"}},
        {"key": "oven", "sortOrder": 2, "labels": {"ko": "베이커리", "en": "Bakery"}},
    ]
    bindings = [{"sku": "a-drink", "categoryKey": "warm"}, {"sku": "z-bread", "categoryKey": "oven"}]
    for row in rows:
        row.update(asset="0x" + "2"*40, tax="0", discount="0")
    def packed(value):
        return json.dumps(value, ensure_ascii=False).replace("'", "''")

    published = sql(f"""
      SELECT ledger.publish_operator_catalog_categories(
        '80d39491-fa9b-4f35-9c2a-f6a258dd45d6','{branch}','94001137-522a-44b9-b8b4-e3fceff2a920',
        gen_random_uuid(),{version},now()+interval '30 minutes','{packed(rows)}'::jsonb,NULL,
        '{packed(categories)}'::jsonb,'{packed(bindings)}'::jsonb)
    """)
    if not published:
        raise RuntimeError("The approved branch did not publish")


def start():
    global app
    with (root / "server.log").open("a") as log:
        app = subprocess.Popen([root / "opsd"], stdout=log, stderr=log)
    for _ in range(80):
        if app.poll() is not None:
            raise RuntimeError("P05 did not start")
        try:
            with socket.create_connection(("127.0.0.1", int(origin_port)), timeout=.2):
                return
        except OSError:
            time.sleep(.1)
    raise RuntimeError("P05 listener unavailable")


def stop():
    global app
    if app is not None:
        app.terminate()
        app.wait(timeout=10)
        app = None


def evidence():
    value = json.loads(sql(f"""
      SELECT json_build_object(
        'categoryCount',(SELECT count(*) FROM ledger.menu_category WHERE branch_id='{branch}'
                          AND version=(SELECT max(version) FROM ledger.menu_release WHERE branch_id='{branch}')),
        'publicRows',(SELECT count(*) FROM ledger.price WHERE branch_id='{branch}'
                        AND version=(SELECT max(version) FROM ledger.menu_release WHERE branch_id='{branch}')),
        'orders',(SELECT count(*) FROM ledger.sale s JOIN ledger.order_checkout c ON c.sale_id=s.id
                  WHERE c.creator='{terminal}' AND s.origin='menu' AND s.amount IN (3500000,4200000)),
        'original',(SELECT coalesce(max(s.id::text),'') FROM ledger.sale s JOIN ledger.order_checkout c ON c.sale_id=s.id
                    WHERE c.creator='{terminal}' AND s.origin='menu' AND s.amount=4200000),
        'available',(SELECT available FROM ledger.stock WHERE branch_id='{branch}' AND sku='a-drink'))
    """))
    return {**value, "service": state}


class Control(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_GET(self):
        if self.path != "/state" or not hmac.compare_digest(self.headers.get("Authorization", ""), "Bearer " + token):
            self.send_error(404)
            return
        payload = json.dumps(evidence()).encode()
        self.send_response(200)
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def do_POST(self):
        global state
        if not hmac.compare_digest(self.headers.get("Authorization", ""), "Bearer " + token):
            self.send_error(404)
            return
        if self.path == "/stop" and state == "online":
            stop()
            state = "stopped"
        elif self.path == "/restart" and state == "stopped":
            publish(2, "4200000")
            start()
            state = "restarted"
        else:
            self.send_error(409)
            return
        self.send_response(204)
        self.end_headers()


signal.signal(signal.SIGTERM, lambda *_: sys.exit(0))
try:
    publish(1, "3500000")
    start()
    with HTTPServer(("127.0.0.1", 0), Control) as server:
        config = {"directory": str(root), "origin": f"http://127.0.0.1:{server.server_port}", "token": token}
        ready.with_suffix(".tmp").write_text(json.dumps(config))
        ready.with_suffix(".tmp").replace(ready)
        server.serve_forever()
finally:
    stop()

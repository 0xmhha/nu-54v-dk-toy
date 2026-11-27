"""Loopback-only synthetic settlement and enrollment for the native pickup journey."""

import argparse
import base64
from decimal import Decimal
import hashlib
from http.server import BaseHTTPRequestHandler, HTTPServer
import json
import os
from pathlib import Path
import secrets
import subprocess
import sys
import time

sys.dont_write_bytecode = True
from p09_visit_journey import SCRIPTS, b64, write_private
from integration_workspace import OwnedProcess, check_output, cleanup_stack, integration_workspace, start_postgres
from test_travel_booking_integration import BRANCH, free_loopback_port, pg, run

TENANT = "80d39491-fa9b-4f35-9c2a-f6a258dd45d6"
SEAT_A = "dfb6bf5f-2445-418d-8966-9dfb03d76812"
SEAT_B = "6978011b-2888-4340-a04e-2587e13a1d55"
PEER = "2fd1308f-3785-43e3-aed5-4082b195c6b7"
CUSTOMER = "0fb9757b-d626-4fb8-b02e-68a9fc8930f0"
TERMINAL = "ea5b823b-9f93-4cad-8b1d-ebd37db4fb5c"
FINANCE = ("fde18be4-23a4-4467-b5f3-35ee0de6674a", "35b3760c-af7e-456c-9cf7-7af75b389db5",
           "9507e11b-6269-41a3-b329-14149d7b217f")
ASSET = "0x" + "a" * 40
MERCHANT = "0x" + "b" * 64


def fixture(work, env, ready, catalog_dsn_file, stack, booking_seats=False, k07_menu=False, pickup_changed=False):
    port = free_loopback_port()
    commerce_port = free_loopback_port()
    while port == commerce_port:
        commerce_port = free_loopback_port()
    env.update(PGHOST=str(work), PGPORT=str(port))
    data = work / "pg"
    run(pg("initdb"), "-D", data, "-A", "trust", "--no-locale", "--encoding=UTF8", env=env)
    start_postgres(pg("pg_ctl"), data, work / "postgres.log", f"-k {work} -p {port} -c listen_addresses=''", env)
    commerce = OwnedProcess()
    stack.callback(commerce.stop)
    database = "nu54_commerce_pickup_ios"
    dsn = f"host={work} port={port} dbname={database}"
    run(pg("createdb"), database, env=env)
    run("python3", SCRIPTS / "../products/p10-platform/database/migrate.py", "p05",
        env={**env, "P05_MIGRATION_DATABASE_URL": dsn})

    if catalog_dsn_file:
        catalog_name = "nu54_p09_native_pickup"
        run(pg("createdb"), catalog_name, env=env)
        catalog_dsn = f"host={work} port={port} dbname={catalog_name}"
        run("python3", SCRIPTS / "../products/p10-platform/database/migrate.py", "p09",
            env={**env, "P09_MIGRATION_DATABASE_URL": catalog_dsn})
        write_private(catalog_dsn_file, catalog_dsn)

    def sql(statement):
        return check_output([pg("psql"), "-AtXv", "ON_ERROR_STOP=1", dsn, "-c", statement], env=env, text=True).strip()

    sql(f"""
      CREATE ROLE pickup_native_http LOGIN IN ROLE nu54_p05_commerce;
      INSERT INTO ledger.branch VALUES('{BRANCH}','{TENANT}','Asia/Seoul','04:00');
      INSERT INTO ledger.command_rule VALUES('{BRANCH}','fixture menu','fixture purpose','discard fixture',interval '15 minutes');
      INSERT INTO ledger.commerce_grant VALUES
        ('{BRANCH}','{CUSTOMER}','customer',NULL,now()+interval '30 minutes'),
        ('{BRANCH}','{TERMINAL}','terminal','menu',now()+interval '30 minutes');
      INSERT INTO ledger.travel_pickup_terminal VALUES('{BRANCH}','{TERMINAL}');
      INSERT INTO ledger.finance_grant VALUES
        ('{BRANCH}','{FINANCE[0]}','refund_request',now()+interval '30 minutes'),
        ('{BRANCH}','{FINANCE[1]}','refund_approve',now()+interval '30 minutes'),
        ('{BRANCH}','{FINANCE[2]}','refund_submit',now()+interval '30 minutes');
      INSERT INTO ledger.menu_release VALUES('{BRANCH}',1,now()-interval '1 minute',now()+interval '30 minutes');
      INSERT INTO ledger.price VALUES('{BRANCH}',1,'drip','plain','시험 커피','{ASSET}',3500000);
      INSERT INTO ledger.stock VALUES('{BRANCH}','drip',6);
      INSERT INTO ledger.travel_pickup_window(branch_id,starts_at,ends_at,capacity)
        VALUES('{BRANCH}',date_trunc('minute',now())+interval '1 hour',
          date_trunc('minute',now())+interval '70 minutes',2);
    """)
    if k07_menu:
        publication = json.loads((SCRIPTS / "../products/p04-merchant-kiosk/test/fixtures/k07-publication.json").read_text())
        operator = "81940dd0-7f2c-42a3-97f7-b7b0850b481c"
        for row in publication["rows"]:
            amount = str(int(Decimal(row["displayPrice"]) * 1_000_000))
            row.update(asset=ASSET, displayPrice=amount, price=amount, tax="0", discount="0")
        encoded_rows = json.dumps(publication["rows"]).replace("'", "''")
        sql(f"""
          INSERT INTO ledger.login_subject VALUES ('k07_native_fixture','{operator}');
          INSERT INTO ledger.staff_grant VALUES ('{BRANCH}','{operator}','operator',now()-interval '1 minute',NULL);
          INSERT INTO ledger.operator_branch_label VALUES ('{BRANCH}','여울 커피 성수점');
          SELECT ledger.publish_operator_catalog_pricing('{TENANT}','{BRANCH}','{operator}',gen_random_uuid(),
            1,now()+interval '30 minutes','{encoded_rows}'::jsonb,NULL);
          UPDATE ledger.stock SET available={int(publication["stock"])}
            WHERE branch_id='{BRANCH}' AND sku IN
              (SELECT sku FROM ledger.price WHERE branch_id='{BRANCH}' AND version=2);
          INSERT INTO ledger.catalog_option_stock(branch_id,sku,option_key,available)
            SELECT branch_id,sku,option_key,{int(publication["stock"])}
            FROM ledger.price WHERE branch_id='{BRANCH}' AND version=2;
        """)
    if pickup_changed:
        change_operator = "81940dd0-7f2c-42a3-97f7-b7b0850b481c"
        sql(f"""
          INSERT INTO ledger.login_subject VALUES ('pickup_change_fixture','{change_operator}');
          INSERT INTO ledger.staff_grant VALUES ('{BRANCH}','{change_operator}','operator',now()-interval '1 minute',NULL);
        """)

    drop_file = work / "drop-committed-booking-response"
    if booking_seats:
        sql(f"""
          INSERT INTO ledger.commerce_grant VALUES
            ('{BRANCH}','{PEER}','customer',NULL,now()+interval '30 minutes');
          INSERT INTO ledger.travel_slot(branch_id,id,starts_at,capacity,terms_version,cancellation_terms,asset_ref,deposit,balance,
                                        price_version,no_show_terms,chain_id,cancellation_cutoff_seconds)
            VALUES ('{BRANCH}','{SEAT_A}',now()+interval '2 hours',2,1,'첫 시간 취소 조건','{ASSET}',10000000,20000000,1,'첫 시간 노쇼 조건',8283,3600),
                   ('{BRANCH}','{SEAT_B}',now()+interval '3 hours',4,1,'둘째 시간 이전 조건','{ASSET}',6000000,10000000,1,'둘째 시간 이전 노쇼 조건',8283,3600);
        """)
    native_server = work / "commerce"
    run("go", "build", "-o", native_server, "./products/p05-operations-backoffice/test/pickup-native", env=env)

    ca, ca_key = work / "ca.pem", work / "ca.key"
    server, server_key = work / "server.pem", work / "server.key"
    server_extension = work / "server.conf"
    server_extension.write_text("subjectAltName=IP:127.0.0.1\nextendedKeyUsage=serverAuth\n")
    run("openssl", "req", "-x509", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes",
        "-keyout", ca_key, "-out", ca, "-days", "1", "-subj", "/CN=Disposable Native Pickup CA",
        "-addext", "basicConstraints=critical,CA:TRUE", "-addext", "keyUsage=critical,keyCertSign,cRLSign", env=env)
    csr = work / "server.csr"
    run("openssl", "req", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes", "-keyout", server_key,
        "-out", csr, "-subj", "/CN=127.0.0.1", env=env)
    run("openssl", "x509", "-req", "-in", csr, "-CA", ca, "-CAkey", ca_key, "-CAcreateserial", "-days", "1",
        "-extfile", server_extension, "-out", server, env=env)

    def certificate(path):
        return check_output(["openssl", "x509", "-in", path, "-outform", "DER"], env=env)

    public = check_output(["openssl", "x509", "-in", server, "-pubkey", "-noout"], env=env)
    spki = check_output(["openssl", "pkey", "-pubin", "-outform", "DER"], env=env, input=public)
    pin = hashlib.sha256(spki).hexdigest()
    presentation_key = "11" * 32
    presentation_pin = hashlib.sha256(bytes.fromhex(presentation_key)).hexdigest()
    participants = {"customer": CUSTOMER}
    if booking_seats:
        participants["peer"] = PEER
    elif not pickup_changed:
        participants["terminal"] = TERMINAL
    profiles, grants = {}, {}
    token = secrets.token_urlsafe(30)

    def enroll(role, key):
        key_path = work / f"{role}.der"
        pem = work / f"{role}.pub"
        leaf = work / f"{role}.pem"
        extension = work / f"{role}.conf"
        extension.write_text("extendedKeyUsage=clientAuth\nkeyUsage=digitalSignature\n")
        key_path.write_bytes(key)
        run("openssl", "pkey", "-pubin", "-inform", "DER", "-in", key_path, "-out", pem, env=env)
        run("openssl", "x509", "-new", "-force_pubkey", pem, "-subj", f"/CN=Disposable Pickup {role}",
            "-CA", ca, "-CAkey", ca_key, "-CAcreateserial", "-extfile", extension, "-out", leaf, "-days", "1", env=env)
        der = certificate(leaf)
        expiry = int(time.time()) + 1200
        actor = participants[role]
        role_code = "customer" if role == "peer" else role
        grants[hashlib.sha256(der).hexdigest()] = {"tenantRef": TENANT, "subjectRef": actor, "role": role_code, "expiresAt": expiry}
        scope = {"role": role_code, "branchRef": BRANCH, "subjectRef": actor, "tenantRef": TENANT, "expiresAt": expiry}
        profile = {"endpoint": f"https://127.0.0.1:{commerce_port}/v2/commerce", "pin": pin,
                   "ca": b64(certificate(ca)), "certificates": [b64(der)], "scope": scope}
        if role_code == "customer" or k07_menu:
            profile["payment"] = {"chain": 8283, "contract": "0x" + "c" * 40, "asset": ASSET, "beneficiary": "0x" + "d" * 40,
                                  "signer": "0x" + "e" * 40, "merchantId": "b" * 64, "keyVersion": 1,
                                  "beneficiaryVersion": 1, "maxBaseUnits": "30000000", "presentationKey": presentation_key}
        else:
            profile["pickupKitchen"] = True
        profiles[role] = profile
        if len(profiles) == len(participants):
            path = work / "grants.json"
            write_private(path, json.dumps(grants))
            command = [native_server, "-port", str(commerce_port), "-cert", server,
                       "-key", server_key, "-client-ca", ca, "-grants", path]
            if booking_seats:
                command.extend(["-drop-hold-marker", drop_file])
            if pickup_changed:
                command.append("-customer-only")
            with (work / "commerce.log").open("a") as log:
                commerce.start(command, env={**env, "P05_PICKUP_NATIVE_DATABASE_URL": dsn + " user=pickup_native_http"},
                               stdout=log, stderr=log)
        return {"registration": json.dumps(profile, separators=(",", ":")), "pin": pin, "presentationPin": presentation_pin}

    if booking_seats:
        peer_key = work / "seat-peer-key.pem"
        run("openssl", "genpkey", "-algorithm", "EC", "-pkeyopt", "ec_paramgen_curve:P-256", "-out", peer_key, env=env)
        public_key = check_output(["openssl", "pkey", "-in", peer_key, "-pubout", "-outform", "DER"], env=env)
        enroll("peer", public_key)

    def latest_order(state):
        # The bridge can only synthesize evidence for the native fixture customer's real captured order.
        sale = sql(f"""SELECT s.id FROM ledger.travel_pickup_order o JOIN ledger.sale s ON s.id=o.sale_id
            JOIN ledger.order_checkout c ON c.sale_id=o.sale_id
          WHERE o.branch_id='{BRANCH}' AND c.creator='{CUSTOMER}' AND s.state='{state}'
          ORDER BY s.id LIMIT 1""")
        if not sale:
            raise ValueError("fixture order unavailable")
        return sale

    def pay():
        sale = latest_order("preview")
        sql(f"""
          INSERT INTO ledger.quote_work(sale_id,actor_id,claim_digest,quote_id,order_id,nonce,expected_revision,profile_version,
            merchant_signer,payer,payer_generation,payer_role,issued_at,expires_at,fence,lease_until,payload_cipher,signed_digest)
          VALUES('{sale}','{CUSTOMER}',decode(repeat('91',32),'hex'),sha256(convert_to('{sale}-quote','UTF8')),
            sha256(convert_to('{sale}-order','UTF8')),sha256(convert_to('{sale}-nonce','UTF8')),0,1,
            decode(repeat('11',20),'hex'),decode(repeat('22',20),'hex'),1,1,
            now()-interval '10 seconds',now()+interval '30 seconds',1,now()+interval '30 seconds',
            decode('00','hex'),sha256(convert_to('{sale}-claim','UTF8')));
          UPDATE ledger.sale SET state='issued' WHERE id='{sale}';
          SELECT ledger.confirm_payment(s.id,q.signed_digest,'{BRANCH}',gen_random_uuid(),
            '0x'||encode(sha256(convert_to(s.id::text||'-pay','UTF8')),'hex'),0,'0x'||repeat('b2',32),
            date_trunc('second',now()),s.asset_ref,s.amount,'0x'||repeat('22',20),1,1,'{MERCHANT}')
            FROM ledger.sale s JOIN ledger.quote_work q ON q.sale_id=s.id WHERE s.id='{sale}';
        """)
        return sale

    def acknowledge(sale):
        sql(f"""
          UPDATE ledger.fulfillment_outbox SET fence=1,state='sending',lease_until=now()+interval '2 minutes'
            WHERE sale_id='{sale}' AND kind='paid';
          SELECT ledger.prepare_fulfillment(event_id,1,effect_key,repeat('b3',32),decode(repeat('b4',32),'hex'),true)
            FROM ledger.fulfillment_outbox WHERE sale_id='{sale}' AND kind='paid';
          SELECT ledger.receive_fulfillment(event_id,effect_key,1,repeat('b3',32),decode(repeat('b4',32),'hex'),
            '{BRANCH}','{BRANCH}','{MERCHANT}',true) FROM ledger.fulfillment_outbox WHERE sale_id='{sale}' AND kind='paid';
        """)

    def refund_request(sale):
        # The staff terminal can only declare failure. Independently scoped roles handle money.
        state = sql(f"SELECT state FROM ledger.travel_pickup_progress WHERE sale_id='{sale}'")
        if state != "unfulfillable":
            raise ValueError("fixture pickup is not unfulfillable")
        result = sql(f"""
          SELECT ledger.reserve_console_refund('{TENANT}','{BRANCH}','{FINANCE[0]}',
            '{sale}',gen_random_uuid(),3500000,'merchant_cancel')::text;
        """)
        return result

    def refund_finalize(sale, refund):
        sql(f"""
          SELECT ledger.approve_refund('{TENANT}','{BRANCH}','{FINANCE[1]}','{sale}','{refund}',0);
          SELECT ledger.submit_console_refund('{TENANT}','{BRANCH}','{FINANCE[2]}','{sale}','{refund}','{MERCHANT}',3500000,0);
          SELECT ledger.mark_refund_inflight('{refund}');
          SELECT ledger.confirm_refund(r.id,'{BRANCH}',gen_random_uuid(),'0x'||repeat('c1',32),0,'0x'||repeat('c2',32),
            date_trunc('second',now()),'{ASSET}',3500000,'0x'||repeat('22',20),1,r.chain_refund_id)
            FROM ledger.refund r WHERE r.id='{refund}' AND r.sale_id='{sale}';
        """)

    class Control(BaseHTTPRequestHandler):
        sale = None
        refund = None
        acknowledged = set()

        def reply(self, payload):
            raw = json.dumps(payload).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(raw)))
            self.end_headers()
            self.wfile.write(raw)

        def do_POST(self):
            if not secrets.compare_digest(self.headers.get("Authorization", ""), "Bearer " + token):
                self.send_error(404)
                return
            role = self.path.removeprefix("/enroll/")
            try:
                if role in participants and role not in profiles:
                    length = int(self.headers.get("Content-Length", "0"))
                    if not 1 <= length <= 160:
                        raise ValueError("invalid enrollment key")
                    key = base64.b64decode(self.rfile.read(length), validate=True)
                    if len(key) != 91:
                        raise ValueError("invalid enrollment key")
                    self.reply(enroll(role, key))
                elif pickup_changed and self.path == "/publish-pickup-change":
                    changed = [{"sku": "drip", "option": "plain", "label": "시험 커피",
                                "asset": ASSET, "displayPrice": "4500000", "price": "4500000",
                                "tax": "0", "discount": "0",
                                "metadata": {"optionLabels": {"ko": "귀리 음료"}}}]
                    encoded = json.dumps(changed, ensure_ascii=False)
                    result = json.loads(sql(f"""SELECT ledger.publish_operator_catalog_pricing('{TENANT}','{BRANCH}',
                            '{change_operator}',gen_random_uuid(),1,now()+interval '30 minutes',
                            '{encoded}'::jsonb,NULL)"""))
                    price = sql(f"""SELECT unit_price FROM ledger.price WHERE branch_id='{BRANCH}'
                            AND version={(int(result["version"]))} AND sku='drip' AND option_key='plain'""")
                    self.reply({"version": str(result["version"]), "price": price})
                elif pickup_changed and self.path == "/pickup-change-proof":
                    row = sql(f"""
                      SELECT (SELECT max(version) FROM ledger.menu_release WHERE branch_id='{BRANCH}'),
                             (SELECT count(*) FROM ledger.sale WHERE branch_id='{BRANCH}'),
                             (SELECT coalesce(max(amount),0) FROM ledger.sale WHERE branch_id='{BRANCH}'),
                             (SELECT count(*) FROM ledger.travel_booking WHERE branch_id='{BRANCH}'),
                             (SELECT count(*) FROM ledger.quote_work),
                             (SELECT count(*) FROM ledger.payment),
                             (SELECT count(*) FROM ledger.relay_attempt)
                    """)
                    version, orders, amount, bookings, quotes, payments, relays = row.split("|")
                    self.reply(dict(version=version, orders=orders, amount=amount, bookings=bookings,
                                    quotes=quotes, payments=payments, relays=relays))
                elif k07_menu and self.path == "/k07-proof":
                    result = sql(f"""
                        SELECT json_build_object(
                          'orders',count(*),'orderRef',coalesce(min(s.id::text),''),
                          'lines',coalesce((
                            SELECT json_agg(json_build_object('sku',l.sku,'option',l.option_key,
                                'quantity',l.quantity,'unitPrice',l.price::text) ORDER BY l.sku,l.option_key)
                            FROM ledger.order_line l JOIN ledger.sale item ON item.id=l.sale_id
                            WHERE item.branch_id='{BRANCH}' AND item.origin='menu' AND l.menu_version=2
                          ),'[]'::json),
                          'stock',(SELECT json_object_agg(p.sku,p.available) FROM ledger.stock p
                            WHERE p.branch_id='{BRANCH}' AND EXISTS (SELECT FROM ledger.price m
                              WHERE m.branch_id=p.branch_id AND m.sku=p.sku AND m.version=2)),
                          'optionStock',(SELECT json_object_agg(p.sku||'/'||p.option_key,p.available)
                            FROM ledger.catalog_option_stock p WHERE p.branch_id='{BRANCH}')
                        )
                        FROM ledger.sale s JOIN ledger.order_checkout c ON c.sale_id=s.id
                        WHERE s.branch_id='{BRANCH}' AND s.origin='menu' AND c.menu_version=2
                    """)
                    self.reply(json.loads(result))
                elif booking_seats and self.path == "/last-seat":
                    sql(f"""SELECT ledger.travel_hold('{TENANT}','{BRANCH}','{PEER}',gen_random_uuid(),
                            '{SEAT_A}',2,1,'20000000','40000000','{ASSET}')""")
                    self.reply({})
                elif booking_seats and self.path == "/change-terms":
                    sql(f"""UPDATE ledger.travel_slot SET deposit=5000000,terms_version=3,price_version=2,
                            cancellation_terms='둘째 시간 최신 조건', no_show_terms='미방문 시 예약금 환불 불가' WHERE branch_id='{BRANCH}' AND id='{SEAT_B}'""")
                    self.reply({})
                elif booking_seats and self.path == "/drop-booking-response":
                    write_private(drop_file, "armed")
                    self.reply({})
                elif booking_seats and self.path == "/booking-proof":
                    row = sql(f"""SELECT count(*),coalesce(min(party_size),0),coalesce(min(terms_version),0),
                        coalesce(min(slot_id::text),'') FROM ledger.travel_booking WHERE branch_id='{BRANCH}' AND actor_id='{CUSTOMER}'""")
                    total, party, terms, slot = row.split("|")
                    self.reply({"count": total, "party": party, "terms": terms, "slot": slot})
                elif booking_seats and self.path == "/booking-payment-proof":
                    row = sql(f"""SELECT count(*),coalesce(min(l.sale_id::text),''),
                        count(q.sale_id) FROM ledger.travel_booking b
                        JOIN ledger.travel_booking_leg l ON (l.branch_id,l.booking_id)=(b.branch_id,b.id)
                        LEFT JOIN ledger.quote_work q ON q.sale_id=l.sale_id
                        WHERE b.branch_id='{BRANCH}' AND b.actor_id='{CUSTOMER}'""")
                    count, sale, quotes = row.split("|")
                    self.reply({"count": count, "order": sale, "quotes": quotes})
                elif not pickup_changed and self.path == "/paid" and len(profiles) == 2:
                    Control.sale = pay()
                    self.reply({"sale": Control.sale})
                elif self.path == "/accepted" and Control.sale and Control.sale not in Control.acknowledged:
                    acknowledge(Control.sale)
                    Control.acknowledged.add(Control.sale)
                    self.reply({})
                elif self.path == "/refund-request" and Control.sale and not Control.refund:
                    Control.refund = refund_request(Control.sale)
                    self.reply({})
                elif self.path == "/refund-final" and Control.sale and Control.refund:
                    refund_finalize(Control.sale, Control.refund)
                    self.reply({})
                else:
                    self.send_error(404)
            except (ValueError, subprocess.CalledProcessError):
                self.send_error(409)

    with HTTPServer(("127.0.0.1", 0), Control) as control:
        write_private(ready, json.dumps({"origin": f"http://127.0.0.1:{control.server_port}", "access": token}))
        print("PICKUP NATIVE FIXTURE READY", flush=True)
        control.serve_forever()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ready-file", type=Path, required=True)
    parser.add_argument("--catalog-dsn-file", type=Path)
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--booking-seats", action="store_true")
    mode.add_argument("--k07-menu", action="store_true", help="publish the K07 menu and enable its read-only ledger proof")
    mode.add_argument("--pickup-changed", action="store_true")
    args = parser.parse_args()
    os.umask(0o077)
    base = {k: v for k, v in os.environ.items() if not k.startswith(("PG", "P05_", "P09_"))}
    with integration_workspace("nu54-native-pickup-", pg("pg_ctl"), "pg", base, term_status=0) as (work, env):
        with cleanup_stack() as stack:
            fixture(work, env, args.ready_file, args.catalog_dsn_file, stack, args.booking_seats, args.k07_menu, args.pickup_changed)


if __name__ == "__main__":
    main()

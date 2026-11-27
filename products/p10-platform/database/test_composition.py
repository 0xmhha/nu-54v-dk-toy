#!/usr/bin/env python3
"""Apply initial production migrations to fresh and cloned v1 test databases."""

import argparse
import concurrent.futures
import json
import os
import subprocess
import uuid
from dataclasses import replace
from pathlib import Path

from migrate import MigrationRejected, OWNERS, ROOT, apply, migrations

HERE = Path(__file__).resolve().parent
P05_LEDGER_CHECK = "products/p05-operations-backoffice/test/migrations/ledger.sql"
W10_GAS_UPGRADE_RESULT = "PASS W10 PostgreSQL upgrade 089→090 (one SQL fixture, archive bytes and default NULL preserved)"
CAFE_PICKUP_UPGRADE_RESULT = "PASS K05 PostgreSQL upgrade 098→099 (original scoped pickup settlement preserved)"
CUSTOMER_CAFE_FORWARD_UPGRADE_RESULT = "PASS W10 PostgreSQL upgrade 099→100 (original customer receipt and submission preserved)"
CUSTOMER_CAFE_REVIEW_UPGRADE_RESULT = "PASS W10 PostgreSQL upgrade 100→101 (original cafe review scope preserved)"
RENTER_PAYMENT_ESCROW_UPGRADE_RESULT = "PASS W10 PostgreSQL upgrade 101→102 (rental handover binding and original admission preserved)"
RENTER_ESCROW_DEADLINE_UPGRADE_RESULT = "PASS W10 PostgreSQL upgrade 102→103 (payer deadline and original admission preserved)"
TRAVEL_PICKUP_UPGRADE_RESULT = "PASS T06 PostgreSQL upgrade 097→098 (original paid pickup and receipt preserved)"
RETURN_RECEIPT_UPGRADE_RESULT = "PASS O09 PostgreSQL upgrade 096→097 (historical envelope preserved and signer ambiguity held)"
RETURN_ISSUANCE_ARCHIVE_UPGRADE_RESULT = "PASS O09/W11 PostgreSQL upgrade 094→096 (one SQL fixture, two interrupted revisions recovered)"
RETURN_PICKUP_UPGRADE_RESULT = "PASS W12/W11 PostgreSQL upgrade 092→094 (one SQL fixture, two interrupted revisions recovered)"
LEGACY_DATA = """
INSERT INTO public.merchants(merchant,name,payout,status) VALUES ('merchant','synthetic','payout','active');
INSERT INTO public.rentals(device,withdraw_addr,state,last_anchor,deposit_tx,close_tx)
VALUES ('test-device','test-return','returning',100,'test-deposit','test-close');
INSERT INTO public.attestations(merchant,payout,valid_from,valid_until,signature)
VALUES ('merchant','payout',10,200,'synthetic-signature');
INSERT INTO public.audit_log(actor_role,action,subject) VALUES ('operator','return','synthetic-scope');
INSERT INTO public.receipts VALUES ('test-tx',0,'test-merchant','test-order','test-device',23,9,120,500);
INSERT INTO public.cursor(block_number) VALUES (120);
"""
LEGACY_SNAPSHOT = """
SELECT to_jsonb(r)::text FROM public.merchants AS r ORDER BY merchant;
SELECT to_jsonb(r)::text FROM public.rentals AS r ORDER BY device;
SELECT to_jsonb(r)::text FROM public.attestations AS r ORDER BY id;
SELECT to_jsonb(r)::text FROM public.audit_log AS r ORDER BY id;
SELECT to_jsonb(r)::text FROM public.receipts AS r ORDER BY tx_hash,log_index;
SELECT to_jsonb(r)::text FROM public.cursor AS r ORDER BY id;
"""


def psql(url, command=None, *, file=None, output=True):
    args = ["psql", "-X", "-A", "-t", "-q", "-w", "--dbname", url, "-v", "ON_ERROR_STOP=1"]
    if file is not None:
        args += ["-f", str(file)]
    proc = subprocess.run(args, input=command, text=True, capture_output=output, check=True, timeout=30)
    return proc.stdout.strip() if output else ""


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def exercise_interruption(url):
    p05 = migrations(OWNERS["p05"][1])
    interrupted = replace(p05[1], sql=p05[1].sql + "\nSELECT 1/0;\n")
    try:
        apply("p05", url, [p05[0], interrupted])
    except ValueError as error:
        require("002_ledger.sql halted" in str(error), "wrong revision failed")
    else:
        raise AssertionError("interruption was accepted")
    require(psql(url, "SELECT to_regnamespace('ledger') IS NULL;") == "t", "partial schema escaped rollback")
    require(psql(url, "SELECT version FROM nu54_p05_migrations.revision;") == "1", "partial journal commit")


def refuse_uncatalogued_field(url):
    p05 = migrations(OWNERS["p05"][1])
    candidate = replace(p05[1], sql=p05[1].sql + "\nALTER TABLE ledger.sale ADD COLUMN extra text;\n")
    try:
        apply("p05", url, [p05[0], candidate])
    except ValueError as error:
        require("002_ledger.sql halted" in str(error), "catalog error did not halt proposed revision")
    else:
        raise AssertionError("uncatalogued field was accepted")
    require(psql(url, "SELECT to_regnamespace('ledger') IS NULL;") == "t", "uncatalogued field escaped rollback")


def assert_roles_and_feature_scope(url):
    psql(url, file=ROOT / P05_LEDGER_CHECK)
    require(psql(url, """
        SELECT count(*) FROM pg_namespace WHERE nspname = 'kiosk';
    """) == "0", "a deferred product schema was deployed")
    require(psql(url, """
        SELECT count(*) FROM pg_constraint c
        JOIN pg_class child ON child.oid = c.conrelid
        JOIN pg_namespace child_ns ON child_ns.oid = child.relnamespace
        JOIN pg_class parent ON parent.oid = c.confrelid
        JOIN pg_namespace parent_ns ON parent_ns.oid = parent.relnamespace
        WHERE c.contype='f' AND child_ns.oid <> parent_ns.oid
        AND child_ns.nspname IN ('chainview','ledger','platform','privacy_ops','travel','travel_private','travel_erasure','wallet_mpc','market');
    """) == "0", "cross-owner foreign key")
    require(psql(url, """SELECT to_regclass('travel.place') IS NOT NULL
        AND to_regclass('travel.opening_interval') IS NOT NULL
        AND to_regclass('travel.catalog_revision') IS NOT NULL
        AND has_table_privilege('nu54_p09_public','travel.place','SELECT')
        AND NOT has_table_privilege('nu54_p09_public','travel.place','INSERT,UPDATE,DELETE')
        AND NOT has_schema_privilege('nu54_p09_public','ledger','USAGE')
        AND NOT has_schema_privilege('nu54_p05_commerce','travel','USAGE');""")=="t",
        "P09 owner schema or role boundary missing")
    require(psql(url, """SELECT to_regclass('wallet_mpc.wallet') IS NOT NULL
        AND to_regclass('wallet_mpc.key_epoch') IS NOT NULL
        AND to_regclass('wallet_mpc.signature_result') IS NOT NULL
        AND has_table_privilege('nu54_p03_service','wallet_mpc.operation','SELECT,INSERT,UPDATE')
        AND NOT has_schema_privilege('nu54_p03_service','ledger','USAGE')
        AND NOT has_schema_privilege('nu54_p03_service','travel','USAGE')
        AND NOT has_schema_privilege('nu54_p09_public','wallet_mpc','USAGE')
        AND NOT has_schema_privilege('nu54_p05_commerce','wallet_mpc','USAGE');""")=="t",
        "P03 owner schema or role boundary missing")
    require(psql(url, """SELECT to_regclass('market.venue') IS NOT NULL
        AND to_regclass('market.keeper_job') IS NOT NULL
        AND to_regclass('market.keeper_attempt') IS NOT NULL
        AND has_table_privilege('nu54_p08_keeper','market.keeper_attempt','SELECT,INSERT')
        AND NOT has_column_privilege('nu54_p08_keeper','market.keeper_attempt','raw_tx','UPDATE')
        AND has_column_privilege('nu54_p08_inspector','market.keeper_attempt','tx_hash','SELECT')
        AND NOT has_column_privilege('nu54_p08_inspector','market.keeper_attempt','raw_tx','SELECT')
        AND NOT has_column_privilege('nu54_p08_inspector','market.keeper_attempt','state','UPDATE')
        AND NOT has_schema_privilege('nu54_p08_keeper','ledger','USAGE')
        AND NOT has_schema_privilege('nu54_p08_keeper','chainview','USAGE')
        AND NOT has_schema_privilege('nu54_p05_commerce','market','USAGE');""")=="t",
        "P08 owner schema or role boundary missing")
    relationships = set(psql(url, """
        SELECT c.conrelid::regclass::text||' -> '||c.confrelid::regclass::text
        FROM pg_constraint c
        JOIN pg_class child ON child.oid=c.conrelid
        JOIN pg_namespace ns ON ns.oid=child.relnamespace
        WHERE c.contype='f' AND ns.nspname IN ('ledger','chainview','platform','privacy_ops','travel','travel_private','travel_erasure','wallet_mpc','market');
    """).splitlines())
    expected = {
        "market.keeper_job -> market.venue", "market.keeper_attempt -> market.keeper_job",
        "wallet_mpc.auth_link -> wallet_mpc.identity",
        "wallet_mpc.profile_binding -> wallet_mpc.identity",
        "wallet_mpc.participant_binding -> wallet_mpc.profile_binding",
        "wallet_mpc.authority_binding -> wallet_mpc.profile_binding",
        "wallet_mpc.wallet -> wallet_mpc.identity", "wallet_mpc.wallet -> wallet_mpc.key_epoch",
        "wallet_mpc.operation -> wallet_mpc.wallet", "wallet_mpc.key_epoch -> wallet_mpc.wallet",
        "wallet_mpc.key_epoch -> wallet_mpc.operation", "wallet_mpc.participant_ack -> wallet_mpc.operation",
        "wallet_mpc.signature_result -> wallet_mpc.operation", "wallet_mpc.audit_outbox -> wallet_mpc.wallet",
        "wallet_mpc.audit_outbox -> wallet_mpc.operation",
        "ledger.command_receipt -> ledger.branch", "ledger.command_receipt -> ledger.sale",
        "ledger.command_rule -> ledger.branch", "ledger.sale -> ledger.branch",
        "ledger.staff_grant -> ledger.branch", "ledger.staff_grant -> ledger.login_subject",
        "chainview.generation -> chainview.stream", "chainview.generation -> chainview.block",
        "chainview.block -> chainview.generation", "chainview.observed_event -> chainview.block",
        "chainview.evidence -> chainview.block", "chainview.evidence -> chainview.observed_event",
        "chainview.read_target -> chainview.stream", "chainview.read_target -> chainview.generation",
        "chainview.switch_request -> chainview.generation", "chainview.incident -> chainview.generation",
        "platform.test_evidence -> platform.build_release", "platform.advisory_case -> platform.build_release",
        "platform.promotion -> platform.build_release", "platform.restore_approval -> platform.restore_job",
        "platform.cutover_receipt -> platform.restore_approval",
        "travel.place -> travel.catalog_revision", "travel.opening_interval -> travel.place",
        "ledger.travel_slot -> ledger.branch", "ledger.travel_booking -> ledger.travel_slot",
        "ledger.travel_booking -> ledger.commerce_grant",
        "privacy_ops.access_record -> privacy_ops.retention_rule", "privacy_ops.rights_case -> privacy_ops.retention_rule",
    }
    require(expected <= relationships, f"missing ERD links: {expected - relationships}")
    require(psql(url, """
        SELECT count(*) FROM pg_constraint c
        WHERE c.contype='f' AND c.conrelid='ledger.command_receipt'::regclass
        AND c.confrelid='ledger.sale'::regclass AND array_length(c.conkey,1)=2;
    """) == "1", "missing composite relationship command_receipt → sale")



def assert_field_policy(url):
    rules = json.loads((HERE / "field-purpose.json").read_text())["rules"]
    covered = []
    for rule in rules:
        require(all(rule.get(key) for key in ("table", "columns", "purpose", "basisRef", "retentionTrigger", "periodRef", "disposal")),
                f"missing policy: {rule}")
        covered.extend(f"{rule['table']}.{column}" for column in rule["columns"])
    actual = psql(url, """
        SELECT c.table_schema||'.'||c.table_name||'.'||c.column_name
        FROM information_schema.columns c
        JOIN information_schema.tables t USING (table_schema, table_name)
        WHERE t.table_type='BASE TABLE' AND c.table_schema IN ('ledger','chainview','platform','privacy_ops','travel','travel_private','travel_erasure','wallet_mpc','market');
    """).splitlines()
    require(sorted(covered) == sorted(actual),
            f"missing or stale field policy: {set(covered).symmetric_difference(actual)}")


def concurrent_commands(url):
    role = f"plan_concurrent_{uuid.uuid4().hex}"
    branch, subject, key, origin = [str(uuid.uuid4()) for _ in range(4)]
    psql(url, f"""
        CREATE ROLE {role} LOGIN IN ROLE nu54_p05_staff;
        INSERT INTO ledger.branch VALUES ('{branch}','{uuid.uuid4()}','Asia/Seoul','04:00');
        INSERT INTO ledger.login_subject VALUES ('{role}','{subject}');
        INSERT INTO ledger.staff_grant VALUES ('{branch}','{subject}','operator',now(),NULL);
        INSERT INTO ledger.command_rule VALUES ('{branch}','synthetic-order','synthetic-contract','expire-fixture',interval '1 day');
    """)
    try:
        stmt = f"SET SESSION AUTHORIZATION {role}; SELECT ledger.create_sale('{branch}','{key}','pos','{origin}','asset',50);"
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as executor:
            results = list(executor.map(lambda _: psql(url, stmt), range(8)))
        require(len(set(results)) == 1 and results[0], "concurrent retries returned different results")
        require(psql(url, f"SELECT count(*) FROM ledger.sale WHERE branch_id='{branch}';") == "1", "duplicate sale")
        require(psql(url, f"SELECT count(*) FROM ledger.command_receipt WHERE branch_id='{branch}';") == "1", "duplicate receipt")
    finally:
        psql(url, f"DROP ROLE {role};")


def apply_all(url):
    for owner, (_, directory) in OWNERS.items():
        apply(owner, url, migrations(directory))


def exercise_merchant_gas_upgrade(url):
    p05 = migrations(OWNERS["p05"][1])
    apply("p05", url, [revision for revision in p05 if revision.version <= 89])
    signed = (ROOT / "packages/protocol/v2/testdata/quote.vectors").read_text().splitlines()[1].split("|")[1]
    archive = bytes.fromhex(signed).hex()
    sale = uuid.uuid4()
    branch = uuid.uuid4()
    psql(url, f"""
        INSERT INTO ledger.branch VALUES ('{branch}',gen_random_uuid(),'Asia/Seoul','04:00');
        INSERT INTO ledger.sale(id,branch_id,origin,origin_ref,state,asset_ref,amount)
        VALUES ('{sale}','{branch}','pos',gen_random_uuid(),'issued','0x'||repeat('1',40),6500000);
        INSERT INTO ledger.quote_work(
            sale_id,actor_id,claim_digest,quote_id,order_id,nonce,expected_revision,profile_version,
            merchant_signer,payer,payer_generation,payer_role,issued_at,expires_at,fence,lease_until,
            payload_cipher,signed_digest)
        VALUES ('{sale}',gen_random_uuid(),sha256('claim'),sha256('quote'),sha256('order'),sha256('nonce'),0,1,
            decode(repeat('1a',20),'hex'),decode(repeat('2a',20),'hex'),1,1,now(),now()+interval '2 minutes',1,now(),
            decode('{archive}','hex'),sha256(decode('{archive}','hex')));
    """)
    snapshot = f"""SELECT encode(payload_cipher,'hex'),encode(signed_digest,'hex')
        FROM ledger.quote_work WHERE sale_id='{sale}';"""
    before = psql(url, snapshot)
    apply("p05", url, [revision for revision in p05 if revision.version <= 90])
    require(before == psql(url, snapshot), "090 altered the archived signed quote bytes or digest")
    require(psql(url, f"SELECT gas_sponsor_digest IS NULL FROM ledger.quote_work WHERE sale_id='{sale}';") == "t",
            "090 fabricated a sponsorship commitment for an old quote")
    require(psql(url, """
        SELECT NOT has_column_privilege('nu54_p05_commerce','ledger.quote_work','gas_sponsor_digest','INSERT,UPDATE')
        AND NOT has_function_privilege('nu54_p05_commerce',
            'ledger.store_forward_internal(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea)','EXECUTE')
        AND has_function_privilege('nu54_p05_commerce',
            'ledger.pin_merchant_gas_sponsor(uuid,uuid,uuid,uuid,bytea,bytea,bytea)','EXECUTE');
    """) == "t", "090 granted direct sponsorship mutation or forwarded past the terminal gate")
    psql(url, f"""BEGIN;
        DELETE FROM ledger.quote_work WHERE sale_id='{sale}';
        DELETE FROM ledger.sale WHERE id='{sale}';
        DELETE FROM ledger.branch WHERE id='{branch}';
        COMMIT;""")
    print(W10_GAS_UPGRADE_RESULT, flush=True)


def exercise_return_pickup_upgrade(url):
    p05 = migrations(OWNERS["p05"][1])
    legacy = [r for r in p05 if r.version <= 92]
    successors = [r for r in p05 if 93 <= r.version <= 94]
    require(len(legacy) == 92 and [r.version for r in successors] == [93, 94],
            "W12/W11 historical migration inputs missing")
    apply("p05", url, legacy)
    branch, actor, device, sale, request = (uuid.uuid4() for _ in range(5))
    psql(url, f"""
        INSERT INTO ledger.branch VALUES ('{branch}',gen_random_uuid(),'Asia/Seoul','04:00');
        INSERT INTO ledger.login_subject VALUES ('audit_subject','{actor}');
        INSERT INTO ledger.rental_enrollment(branch_id,device_ref,generation,account_ref,withdraw_ref,revision,enrollment_digest,attested_until)
        VALUES ('{branch}','{device}',1,'0x'||repeat('1',40),'0x'||repeat('2',40),'qa',sha256('test-enrollment'),now()+interval '1 hour');
        INSERT INTO ledger.rental_operation(branch_id,device_ref,generation,action,request_key,actor_id,status,evidence)
        VALUES ('{branch}','{device}',1,'cleanup','{request}','{actor}','acknowledged','{{"status":"cleaned"}}');
        INSERT INTO ledger.sale(id,branch_id,origin,origin_ref,state,asset_ref,amount) VALUES ('{sale}','{branch}','menu',gen_random_uuid(),'paid','test-usdc',6500000);
        INSERT INTO ledger.quote_work(sale_id,actor_id,claim_digest,quote_id,order_id,nonce,expected_revision,profile_version,merchant_signer,payer,payer_generation,payer_role,issued_at,expires_at,fence,lease_until)
        VALUES ('{sale}','{actor}',sha256('claim'),sha256('quote'),sha256('order'),sha256('nonce'),0,1,decode(repeat('1a',20),'hex'),decode(repeat('2a',20),'hex'),1,1,now(),now()+interval '2 minutes',1,now());
        INSERT INTO ledger.payment(sale_id,stream_id,merchant_key,evidence_id,tx_hash,log_index,inclusion_hash,included_at,asset_ref,payer,payer_generation,payment_mode,principal)
        VALUES ('{sale}',gen_random_uuid(),'0x'||repeat('1',64),gen_random_uuid(),'0x'||repeat('2',64),0,'0x'||repeat('3',64),now(),'test-usdc','0x'||repeat('4',40),1,1,6500000);
        INSERT INTO ledger.pickup(sale_id,branch_id,business_day,number) VALUES ('{sale}','{branch}',current_date,1);
        INSERT INTO ledger.travel_pickup_progress(sale_id,state,revision,updated_at) VALUES ('{sale}','collected',2,now());
        INSERT INTO ledger.travel_pickup_transition VALUES ('{sale}','{actor}',gen_random_uuid(),1,'ready'),
               ('{sale}','{actor}',gen_random_uuid(),2,'collected');
    """)
    snapshot = """
        SELECT version,name,sha256,applied_at FROM nu54_p05_migrations.revision WHERE version<=92 ORDER BY version;
        SELECT to_jsonb(o) FROM ledger.rental_operation o;
        SELECT sale_id,actor_id,request_key,revision,target FROM ledger.travel_pickup_transition ORDER BY revision;
        SELECT to_jsonb(p) FROM ledger.travel_pickup_progress p;
    """
    historic = psql(url, snapshot)
    absence = {
        93: "SELECT to_regclass('ledger.rental_cleanup_intent') IS NULL",
        94: """SELECT NOT EXISTS(SELECT FROM information_schema.columns WHERE table_schema='ledger'
                      AND table_name='travel_pickup_transition' AND column_name='ready_at')""",
    }
    accepted = legacy[:]
    for revision in successors:
        interrupted = replace(revision, sql=revision.sql + "\nSELECT 1/0;\n")
        try:
            apply("p05", url, [*accepted, interrupted])
        except MigrationRejected as error:
            require("division by zero" in error.psql_stderr, f"unexpected {revision.version} failure")
        else:
            raise AssertionError(f"{revision.version} interruption committed")
        require(psql(url, "SELECT max(version) FROM nu54_p05_migrations.revision") == str(revision.version - 1),
                f"{revision.version} journal escaped rollback")
        require(psql(url, absence[revision.version]) == "t", f"{revision.version} DDL escaped rollback")
        accepted.append(revision)
        apply("p05", url, accepted)
        require(psql(url, snapshot) == historic, f"{revision.version} changed an original row")
    require(psql(url, "SELECT count(*)||'|'||count(ready_at) FROM ledger.travel_pickup_transition") == "2|0",
            "094 assigned times to legacy transitions")
    require(psql(url, "SELECT count(*) FROM ledger.rental_cleanup_intent") == "0",
            "093 promoted a historical cleanup")
    require(psql(url, """SELECT NOT (has_table_privilege('nu54_p05_commerce','ledger.rental_cleanup_intent','SELECT')
                 OR has_table_privilege('nu54_p05_commerce','ledger.rental_cleanup_intent','INSERT')
                 OR has_table_privilege('nu54_p05_commerce','ledger.rental_cleanup_intent','UPDATE')
                 OR has_table_privilege('nu54_p05_commerce','ledger.rental_cleanup_intent','DELETE'))""") == "t",
            "093 admits direct mutation or reads from commerce")
    journal = "SELECT version,name,sha256,applied_at FROM nu54_p05_migrations.revision ORDER BY version"
    complete = psql(url, journal)
    apply("p05", url, accepted)
    require(psql(url, snapshot) == historic, "093/094 repeat changed an original row")
    require(psql(url, journal) == complete, "093/094 repeat changed the completed journal")
    for status in ("dispatching", "unknown", "submitted", "acknowledged"):
        psql(url, f"UPDATE ledger.rental_operation SET status='{status}'")
        require(psql(url, f"SELECT ledger.rental_predecessor_has_unverified_cleanup('{branch}','{device}',2)") == "t",
                f"historical {status} granted successor eligibility")
        try:
            psql(url, f"""INSERT INTO ledger.rental_enrollment(
                branch_id,device_ref,generation,account_ref,withdraw_ref,revision,enrollment_digest,attested_until)
                VALUES ('{branch}','{device}',2,'0x'||repeat('3',40),'0x'||repeat('2',40),'qa',
                        sha256('successor'),now()+interval '1 hour');""")
        except subprocess.CalledProcessError as error:
            require("verified predecessor board cleanup required" in (error.stderr or ""),
                    f"historical {status} was rejected for an unrelated reason")
        else:
            raise AssertionError(f"historical {status} enabled a successor enrollment")



def exercise_return_issuance_archive_upgrade(url):
    p05 = migrations(OWNERS["p05"][1])
    accepted = [r for r in p05 if r.version <= 94]
    successors = [r for r in p05 if 95 <= r.version <= 96]
    require(len(accepted) == 94 and [r.version for r in successors] == [95, 96], "094 to 096 SQL inputs missing")
    apply("p05", url, accepted)
    branch, actor, device, proof, intent, sale = (uuid.uuid4() for _ in range(6))
    psql(url, f"""
        INSERT INTO ledger.branch VALUES ('{branch}',gen_random_uuid(),'Asia/Seoul','04:00');
        INSERT INTO ledger.login_subject VALUES ('migration_audit','{actor}');
        INSERT INTO ledger.rental_enrollment(branch_id,device_ref,generation,account_ref,withdraw_ref,revision,enrollment_digest,attested_until,attested_at)
        VALUES ('{branch}','{device}',1,'0x'||repeat('1',40),'0x'||repeat('2',40),'qa',sha256('enrollment'),now()+interval '1 hour',now());
        INSERT INTO ledger.rental_generation_source VALUES
        ('{branch}','{device}',1,sha256('enrollment'),'0x'||repeat('1',40),'0x'||repeat('2',40),'qa',sha256('measurement'),
         '0x'||repeat('3',40),'0x'||repeat('4',40),sha256('deployment'),gen_random_uuid(),gen_random_uuid(),
         sha256('challenge'),now()+interval '10 minutes',current_user,now()+interval '10 minutes');
        INSERT INTO ledger.rental_generation_authorized_proof(branch_id,device_ref,generation,request_key,request_digest,source_digest,
            enrollment_digest,challenge,freeze_digest,journal_root,first_nonce,next_nonce,payment_entries,limit_entries,burned_entries,
            stream_ref,stream_generation,finalized_height,finalized_hash,assessment_digest,frozen_at,finalized_at,recorded_at,fresh_until,
            pending_payments,pending_limits)
        SELECT branch_id,device_ref,generation,'{proof}',sha256('request'),ledger.rental_generation_source_digest(s),enrollment_digest,
            challenge,sha256('freeze'),sha256('root'),sha256('nonce'),sha256('nonce'),0,0,0,stream_ref,stream_generation,
            14,sha256('final'),sha256('review'),now(),now(),now(),now()+interval '5 minutes',0,0
        FROM ledger.rental_generation_source s;
        INSERT INTO ledger.rental_proof_submission VALUES ('{branch}','{device}',1,'{proof}',sha256('submitted'));
        INSERT INTO ledger.rental_close_candidate(branch_id,device_ref,generation,request_key,actor_id,proof_key,review_ref,authority_digest,
            account_ref,signer,nonce,tx_hash,state)
        VALUES ('{branch}','{device}',1,gen_random_uuid(),'{actor}','{proof}',repeat('a',64),sha256('scope'),
                '0x'||repeat('1',40),'0x'||repeat('b',40),0,sha256('close'),'confirmed');
        INSERT INTO ledger.rental_cleanup_intent VALUES ('{branch}','{device}',1,'{actor}','{intent}',sha256('scope'));
        INSERT INTO ledger.sale(id,branch_id,origin,origin_ref,state,asset_ref,amount) VALUES
            ('{sale}','{branch}','menu',gen_random_uuid(),'closed','test-usdc',6500000);
        INSERT INTO ledger.quote_work(sale_id,actor_id,claim_digest,quote_id,order_id,nonce,expected_revision,profile_version,merchant_signer,
            payer,payer_generation,payer_role,issued_at,expires_at,fence,lease_until)
        VALUES ('{sale}','{actor}',sha256('claim'),sha256('quote'),sha256('order'),sha256('nonce'),0,1,
            decode(repeat('1a',20),'hex'),decode(repeat('2a',20),'hex'),1,1,now(),now()+interval '2 minutes',1,now());
        INSERT INTO ledger.sale_resolution(sale_id,evidence_id) VALUES ('{sale}',gen_random_uuid());
        INSERT INTO ledger.menu_reservation_outcome(branch_id,actor_id,request_key,origin_ref,nearby_customer,intent_digest,sale_id)
        VALUES ('{branch}',gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),'{actor}',sha256('unsigned original'),'{sale}');
    """)
    snapshot = """
        SELECT version,name,sha256,applied_at FROM nu54_p05_migrations.revision WHERE version<=94 ORDER BY version;
        SELECT branch_id,device_ref,generation,actor_id,request_key,disposition FROM ledger.rental_cleanup_intent;
        SELECT to_jsonb(c) FROM ledger.rental_close_candidate c;
        SELECT to_jsonb(o) FROM ledger.menu_reservation_outcome o;
        SELECT to_jsonb(q) FROM ledger.quote_work q;
        SELECT to_jsonb(r) FROM ledger.sale_resolution r;
    """
    historic = psql(url, snapshot)
    absence = {
        95: "SELECT to_regclass('ledger.rental_cleanup_station') IS NULL",
        96: "SELECT to_regprocedure('ledger.owns_closed_nearby_order(uuid,uuid,uuid)') IS NULL",
    }
    for revision in successors:
        interrupted = replace(revision, sql=revision.sql + "\nSELECT 1/0;\n")
        try:
            apply("p05", url, [*accepted, interrupted])
        except MigrationRejected as error:
            require("division by zero" in error.psql_stderr, f"unexpected {revision.version} failure")
        else:
            raise AssertionError(f"{revision.version} interruption committed")
        require(psql(url, "SELECT max(version) FROM nu54_p05_migrations.revision") == str(revision.version - 1),
                f"{revision.version} journal escaped rollback")
        require(psql(url, absence[revision.version]) == "t", f"{revision.version} DDL escaped rollback")
        accepted.append(revision)
        apply("p05", url, accepted)
        require(psql(url, snapshot) == historic, f"{revision.version} changed an original row")
    require(psql(url, "SELECT count(*)||'|'||count(anchor_height)||'|'||count(anchor_hash)||'|'||count(signed_intent) FROM ledger.rental_cleanup_intent") == "1|0|0|0",
            "095 promoted a legacy intent")
    require(psql(url, "SELECT count(*) FROM ledger.rental_cleanup_station") == "0", "095 qualified a station by default")
    require(psql(url, f"SELECT ledger.owns_closed_nearby_order('{branch}','{actor}','{sale}')") == "f",
            "096 promoted an unsigned original to archive ownership")
    journal = "SELECT version,name,sha256,applied_at FROM nu54_p05_migrations.revision ORDER BY version"
    complete = psql(url, journal)
    apply("p05", url, accepted)
    require(psql(url, snapshot) == historic and psql(url, journal) == complete, "095/096 replay changed original rows or journal")

    return branch, actor, device, intent


def exercise_return_receipt_upgrade(url, branch, actor, device, intent):
    p05 = migrations(OWNERS["p05"][1])
    accepted = [r for r in p05 if r.version <= 96]
    current = [r for r in p05 if r.version == 97]
    require(len(accepted) == 96 and len(current) == 1, "096 to 097 SQL inputs missing")
    revision = current[0]
    tenant = psql(url, f"SELECT tenant_ref FROM ledger.branch WHERE id='{branch}'")
    psql(url, f"""
        UPDATE ledger.rental_cleanup_intent SET anchor_height=14,anchor_hash=sha256('final'),
            signed_intent=convert_to('RCI1','UTF8')||decode(repeat('5c',177),'hex') WHERE request_key='{intent}';
        INSERT INTO ledger.finance_grant VALUES ('{branch}','{actor}','rental_manage',now()+interval '1 hour');
        INSERT INTO ledger.rental_collector_registration(branch_id,device_ref,generation,operator_ref,collector_certificate,
            collector_peer,first_nonce,registration_proof,authority_source,valid_until)
        SELECT branch_id,device_ref,generation,'{actor}',sha256('station'),decode(repeat('05',7),'hex'),decode(repeat('00',32),'hex'),
            sha256('registered'),ledger.rental_generation_source_digest(s),now()+interval '1 hour'
        FROM ledger.rental_generation_source s WHERE branch_id='{branch}' AND device_ref='{device}';
        INSERT INTO ledger.rental_cleanup_station VALUES ('{branch}','{device}',1,sha256('station'),now()+interval '1 hour');
    """)
    snapshot = """
        SELECT version,name,sha256,applied_at FROM nu54_p05_migrations.revision WHERE version<=96 ORDER BY version;
        SELECT branch_id,device_ref,generation,actor_id,request_key,disposition,anchor_height,anchor_hash,signed_intent
            FROM ledger.rental_cleanup_intent;
        SELECT to_jsonb(c) FROM ledger.rental_close_candidate c;
        SELECT to_jsonb(p) FROM ledger.rental_generation_authorized_proof p;
    """
    historic = psql(url, snapshot)
    interrupted = replace(revision, sql=revision.sql + "\nSELECT 1/0;\n")
    try:
        apply("p05", url, [*accepted, interrupted])
    except MigrationRejected as error:
        require("division by zero" in error.psql_stderr, "unexpected 097 interruption")
    else:
        raise AssertionError("097 interruption committed")
    require(psql(url, "SELECT max(version) FROM nu54_p05_migrations.revision") == "96",
            "097 journal escaped rollback")
    require(psql(url, """SELECT count(*) FROM information_schema.columns WHERE table_schema='ledger'
            AND table_name='rental_cleanup_intent' AND column_name IN ('intent_board_signer','signed_receipt')""") == "0",
            "097 schema escaped rollback")
    apply("p05", url, [*accepted, revision])
    require(psql(url, snapshot) == historic, "097 retry changed original issuance or upstream evidence")
    authority = f"""SELECT encode(board,'hex') FROM ledger.rental_cleanup_receipt_authority(
        '{tenant}','{branch}','{device}',1,'{intent}',sha256('station'))"""
    psql(url, f"UPDATE ledger.rental_generation_source SET board_signer_ref='0x'||repeat('7',40) WHERE branch_id='{branch}'")
    try:
        psql(url, authority)
    except subprocess.CalledProcessError as error:
        require("original cleanup receipt authority unavailable" in (error.stderr or ""), "historical signer was held for another reason")
    else:
        raise AssertionError("replacement board was trusted for a legacy envelope")
    require(psql(url, "SELECT count(*)||'|'||count(intent_board_signer)||'|'||count(signed_receipt) FROM ledger.rental_cleanup_intent") == "1|0|0",
            "097 inferred a missing signer or receipt")
    psql(url, f"UPDATE ledger.rental_generation_source SET board_signer_ref='0x'||repeat('3',40) WHERE branch_id='{branch}'")
    require(psql(url, authority) == "3" * 40, "exact independently archived source did not resolve original signer")
    require(psql(url, f"SELECT ledger.rental_predecessor_has_unverified_cleanup('{branch}','{device}',2)") == "t",
            "legacy envelope released the successor")
    journal = "SELECT version,name,sha256,applied_at FROM nu54_p05_migrations.revision ORDER BY version"
    completed = psql(url, journal)
    apply("p05", url, [*accepted, revision])
    require(psql(url, snapshot) == historic and psql(url, journal) == completed, "097 replay falsified historical rows or journal")
    require(psql(url, "SELECT count(intent_board_signer)+count(signed_receipt) FROM ledger.rental_cleanup_intent") == "0",
            "an exact authority lookup inferred a signer or receipt")


def exercise_travel_pickup_upgrade(url):
    revisions = migrations(OWNERS["p05"][1])
    predecessor = [r for r in revisions if r.version <= 97]
    successor = next(r for r in revisions if r.version == 98)
    require(len(predecessor) == 97, "097 predecessor is incomplete")
    apply("p05", url, predecessor)
    tenant, branch, owner, stranger, window = (str(uuid.uuid4()) for _ in range(5))
    asset = "0x" + "a" * 40
    merchant = "0x" + "b" * 64
    psql(url, f"""
        INSERT INTO ledger.branch VALUES('{branch}','{tenant}','Asia/Seoul','04:00');
        INSERT INTO ledger.command_rule VALUES('{branch}','fixture menu','fixture purpose','discard fixture',interval '15 minutes');
        INSERT INTO ledger.commerce_grant VALUES
          ('{branch}','{owner}','customer',NULL,now()+interval '30 minutes'),
          ('{branch}','{stranger}','customer',NULL,now()+interval '30 minutes');
        INSERT INTO ledger.menu_release VALUES('{branch}',1,now()-interval '1 minute',now()+interval '30 minutes');
        INSERT INTO ledger.price VALUES('{branch}',1,'drip','plain','fixture coffee','{asset}',4500000);
        INSERT INTO ledger.stock VALUES('{branch}','drip',6);
        INSERT INTO ledger.travel_pickup_window(branch_id,id,starts_at,ends_at,capacity)
          VALUES('{branch}','{window}',now()+interval '1 hour',now()+interval '70 minutes',3);
    """)
    sales = []
    for quantity in (1, 2):
        sale = psql(url, f"""
          SELECT ledger.reserve_travel_pickup('{tenant}','{branch}','{owner}',gen_random_uuid(),gen_random_uuid(),
            1,'{asset}','[{{"sku":"drip","option_key":"plain","quantity":{quantity}}}]','{window}',starts_at)
          FROM ledger.travel_pickup_window WHERE id='{window}'
        """)
        sales.append(str(uuid.UUID(sale)))
        psql(url, f"""
          INSERT INTO ledger.quote_work(sale_id,actor_id,claim_digest,quote_id,order_id,nonce,expected_revision,profile_version,
            merchant_signer,payer,payer_generation,payer_role,issued_at,expires_at,fence,lease_until,payload_cipher,signed_digest)
          VALUES('{sale}','{owner}',sha256('claim'),sha256('{sale}-quote'),sha256('{sale}-order'),sha256('{sale}-nonce'),0,1,
            decode(repeat('11',20),'hex'),decode(repeat('22',20),'hex'),1,1,
            now()-interval '10 seconds',now()+interval '30 seconds',1,now()+interval '30 seconds',
            decode('00','hex'),sha256('{sale}-claim'));
          UPDATE ledger.sale SET state='issued' WHERE id='{sale}';
          SELECT ledger.confirm_payment(s.id,q.signed_digest,'{branch}',gen_random_uuid(),
            '0x'||encode(sha256(convert_to(s.id::text||'-pay','UTF8')),'hex'),0,'0x'||repeat('b2',32),
            date_trunc('second',now()),s.asset_ref,s.amount,'0x'||repeat('22',20),1,1,'{merchant}')
          FROM ledger.sale s JOIN ledger.quote_work q ON q.sale_id=s.id WHERE s.id='{sale}';
        """)
    acknowledged, waiting = sales
    psql(url, f"""
      UPDATE ledger.fulfillment_outbox SET fence=1,state='sending',lease_until=now()+interval '2 minutes'
        WHERE sale_id='{acknowledged}' AND kind='paid';
      SELECT ledger.prepare_fulfillment(event_id,1,effect_key,repeat('b3',32),decode(repeat('b4',32),'hex'),true)
        FROM ledger.fulfillment_outbox WHERE sale_id='{acknowledged}' AND kind='paid';
      SELECT ledger.receive_fulfillment(event_id,effect_key,1,repeat('b3',32),decode(repeat('b4',32),'hex'),
        '{branch}','{branch}','{merchant}',true) FROM ledger.fulfillment_outbox WHERE sale_id='{acknowledged}' AND kind='paid';
    """)
    snapshot = """
      SELECT version,name,sha256,applied_at FROM nu54_p05_migrations.revision WHERE version<=97 ORDER BY version;
      SELECT to_jsonb(x) FROM ledger.sale x ORDER BY id;
      SELECT to_jsonb(x) FROM ledger.travel_pickup_order x ORDER BY sale_id;
      SELECT to_jsonb(x) FROM ledger.payment x ORDER BY sale_id;
      SELECT to_jsonb(x) FROM ledger.journal x ORDER BY sale_id;
      SELECT to_jsonb(x) FROM ledger.pickup x ORDER BY sale_id;
      SELECT to_jsonb(x) FROM ledger.fulfillment_outbox x ORDER BY sale_id;
      SELECT to_jsonb(x) FROM ledger.consumer_receipt x ORDER BY event_id;
    """
    historic = psql(url, snapshot)
    original = {}
    for sale in sales:
        # Read through the restricted SQL entry as an authenticated customer would.
        original[sale] = psql(url, f"""
          SET ROLE nu54_p05_commerce;
          SELECT id,state,amount,pickup_no,receipt_ref
          FROM ledger.commerce_order_status('{tenant}','{branch}','{owner}','{sale}')
        """).splitlines()[-1].split("|")
        require(len(original[sale]) == 5 and all(original[sale][3:]), "097 paid keys missing")
    try:
        apply("p05", url, [*predecessor, replace(successor, sql=successor.sql + "\nSELECT 1/0;\n")])
    except MigrationRejected as error:
        require("division by zero" in error.psql_stderr, "unexpected 098 interruption")
    else:
        raise AssertionError("098 interruption committed")
    require(psql(url, "SELECT max(version) FROM nu54_p05_migrations.revision") == "97", "098 journal escaped rollback")
    require(psql(url, f"""SELECT pickup_no FROM ledger.commerce_order_status('{tenant}','{branch}','{owner}','{waiting}')""")
            == original[waiting][3], "098 projection escaped rollback")
    apply("p05", url, [*predecessor, successor])
    for sale in sales:
        result = psql(url, f"""
          SET ROLE nu54_p05_commerce;
          SELECT id,state,amount,pickup_no,receipt_ref,receipt_kind
          FROM ledger.commerce_order_status('{tenant}','{branch}','{owner}','{sale}')
        """).splitlines()[-1].split("|")
        expected = [*original[sale], ""]
        if sale == waiting:
            expected[3], expected[5] = "", "pickup-awaiting-acceptance"
        require(result == expected, "098 projection lost or incorrectly exposed an original paid receipt")
    require(psql(url, f"""
      SET ROLE nu54_p05_commerce;
      SELECT count(*) FROM ledger.commerce_order_status('{tenant}','{branch}','{stranger}','{waiting}')
    """).splitlines()[-1] == "0", "098 exposed another customer's order")
    journal = "SELECT version,name,sha256,applied_at FROM nu54_p05_migrations.revision ORDER BY version"
    completed = psql(url, journal)
    apply("p05", url, [*predecessor, successor])
    require(psql(url, snapshot) == historic and psql(url, journal) == completed,
            "098 replay changed historical orders, payment, ACK evidence or migration receipts")


def exercise_cafe_pickup_upgrade(url):
    revisions = migrations(OWNERS["p05"][1])
    predecessor = [revision for revision in revisions if revision.version <= 98]
    successor = next(revision for revision in revisions if revision.version == 99)
    require(len(predecessor) == 98, "098 predecessor is incomplete")
    # The 093 control predates durable ready timestamps. Its time must stay unknown.
    apply("p05", url, predecessor[:93])
    tenant, branch, other_branch, customer, nearby_customer, terminal, operator, outsider = (
        str(uuid.uuid4()) for _ in range(8)
    )
    asset = "0x" + "a" * 40
    merchant = "0x" + "b" * 64

    def commerce(sql):
        return psql(url, "SET ROLE nu54_p05_commerce;\n" + sql)

    psql(url, f"""
      INSERT INTO ledger.branch VALUES
        ('{branch}','{tenant}','Asia/Seoul','04:00'), ('{other_branch}','{tenant}','Asia/Seoul','04:00');
      INSERT INTO ledger.command_rule VALUES ('{branch}','fixture menu','fixture purpose','dispose',interval '15 minutes');
      INSERT INTO ledger.commerce_grant VALUES
        ('{branch}','{customer}','customer',NULL,now()+interval '30 minutes'),
        ('{branch}','{nearby_customer}','customer',NULL,now()+interval '30 minutes'),
        ('{branch}','{terminal}','terminal','menu',now()+interval '30 minutes');
      INSERT INTO ledger.travel_pickup_terminal VALUES ('{branch}','{terminal}');
      INSERT INTO ledger.finance_grant VALUES
        ('{branch}','{operator}','pickup_manage',now()+interval '30 minutes'),
        ('{other_branch}','{operator}','pickup_manage',now()+interval '30 minutes'),
        ('{other_branch}','{outsider}','pickup_manage',now()+interval '30 minutes');
      INSERT INTO ledger.payer_enrollment VALUES
        ('{branch}','{nearby_customer}',decode(repeat('22',20),'hex'),1,1,now()+interval '30 minutes');
      INSERT INTO ledger.menu_release VALUES ('{branch}',1,now()-interval '1 minute',now()+interval '30 minutes');
      INSERT INTO ledger.price VALUES ('{branch}',1,'drip','plain','fixture coffee','{asset}',4000000);
      INSERT INTO ledger.stock VALUES ('{branch}','drip',20);
    """)

    def paid_order(quantity, acknowledge=True, nearby=False, forward_key=None):
        creator, role = (terminal, "terminal") if nearby else (customer, "customer")
        payer = nearby_customer if nearby else customer
        sale = commerce(f"""
          SELECT ledger.reserve_order('{tenant}','{branch}','{creator}',gen_random_uuid(),'menu',gen_random_uuid(),
            1,'{asset}','[{{"sku":"drip","option_key":"plain","quantity":{quantity}}}]','{role}')
        """)
        sale = str(uuid.UUID(sale))
        if nearby:
            commerce(f"""
              SELECT ledger.request_checkout_pair('{tenant}','{branch}','{payer}','{sale}',gen_random_uuid(),'a1b2c3d4');
              SELECT ledger.confirm_checkout_pair('{tenant}','{branch}','{terminal}','{sale}','{payer}',
                'a1b2c3d4',gen_random_uuid(),sha256('nearby-intent'),sha256('{sale}-claim'));
            """)
        psql(url, f"""
          INSERT INTO ledger.quote_work(sale_id,actor_id,claim_digest,quote_id,order_id,nonce,expected_revision,profile_version,
            merchant_signer,payer,payer_generation,payer_role,issued_at,expires_at,fence,lease_until,payload_cipher,signed_digest)
          VALUES('{sale}','{payer}',sha256('{sale}-claim'),sha256('{sale}-quote'),sha256('{sale}-order'),sha256('{sale}-nonce'),0,1,
            decode(repeat('11',20),'hex'),decode(repeat('22',20),'hex'),1,1,
            now()-interval '10 seconds',now()+interval '30 seconds',1,now()+interval '30 seconds',
            decode('00','hex'),sha256('{sale}-signed'));
          UPDATE ledger.sale SET state='issued' WHERE id='{sale}';
        """)
        if forward_key:
            psql(url, f"""
              UPDATE ledger.quote_work SET gas_sponsor_digest=sha256('{sale}-sponsor') WHERE sale_id='{sale}';
            """)
            require(commerce(customer_cafe_forward_sql(tenant, branch, payer, sale, forward_key)) == sale,
                    "099 fixture did not accept the original customer submission")
        psql(url, f"""
          SELECT ledger.confirm_payment(s.id,q.signed_digest,'{branch}',gen_random_uuid(),
            '0x'||encode(sha256(convert_to(s.id::text||'-pay','UTF8')),'hex'),0,'0x'||repeat('b2',32),
            date_trunc('second',now()),s.asset_ref,s.amount,'0x'||repeat('22',20),1,1,'{merchant}')
          FROM ledger.sale s JOIN ledger.quote_work q ON q.sale_id=s.id WHERE s.id='{sale}';
        """)
        if acknowledge:
            claimed = psql(url, f"""
              SET ROLE nu54_p05_dispatch;
              SELECT event_id,fence,effect_key FROM ledger.claim_fulfillment() WHERE sale_id='{sale}'
            """)
            require(len(claimed.split("|")) == 3, "dispatcher did not claim original fulfillment")
            event, fence, effect = claimed.split("|")
            epoch = "b3" * 32
            psql(url, f"""
              SET ROLE nu54_p05_dispatch;
              SELECT ledger.prepare_fulfillment('{event}',{int(fence)},'{effect}','{epoch}',sha256('{sale}-payload'),true);
            """)
            confirmed = psql(url, f"""
              SET ROLE nu54_p05_receiver;
              SELECT ledger.receive_fulfillment('{event}','{effect}',{int(fence)},'{epoch}',sha256('{sale}-payload'),
                  '{branch}','{branch}','{merchant}',true);
            """)
            require(confirmed == "delivered", "receiver did not accept original fulfillment")
            delivery = psql(url, f"""
              SET ROLE nu54_p05_dispatch;
              SELECT ledger.finish_fenced_fulfillment('{event}',{int(fence)},'{effect}','{epoch}','delivered',sha256('{sale}-payload'));
            """)
            require(delivery == "t", "dispatcher did not complete original accepted fulfillment")
        return sale

    def advance(sale, *states):
        for revision, state in enumerate(states):
            step = commerce(f"""
              SELECT ledger.kiosk_pickup_change('{tenant}','{branch}','{operator}','{sale}',
                  gen_random_uuid(),{revision},'{state}','operator')
            """)
            require(step == str(revision + 1), "original pickup transition did not advance")

    legacy = paid_order(1)
    advance(legacy, "preparing", "ready")
    apply("p05", url, predecessor)
    ready = paid_order(1)
    advance(ready, "preparing", "ready")
    collected = paid_order(2, nearby=True)
    advance(collected, "preparing", "ready", "collected")
    preparing = paid_order(1)
    advance(preparing, "preparing")
    unacknowledged = paid_order(1, acknowledge=False)

    def detail(sale, *, actor=operator, site=branch, role="operator", include=True):
        sale_arg = f"'{sale}'" if sale else "NULL"
        return commerce(f"""
          SELECT COALESCE(jsonb_agg(row), '[]') FROM ledger.kiosk_pickup_queue(
            '{tenant}','{site}','{actor}',{sale_arg},'{role}',{str(include).lower()}) row
        """)

    def buyer_view(sale, payer):
        return commerce(f"""
          SELECT id,state,amount,pickup_no,receipt_ref
          FROM ledger.commerce_order_status('{tenant}','{branch}','{payer}','{sale}')
        """)

    sales = (legacy, ready, collected, preparing, unacknowledged)
    original = {sale: json.loads(detail(sale))[0] for sale in sales}
    require(all("settlement" not in row for row in original.values()), "098 unexpectedly exposes 099 settlement info")
    buyers = {sale: buyer_view(sale, nearby_customer if sale == collected else customer) for sale in sales}
    require(all("|paid|" in value for value in buyers.values()), "original customer receipt not visible")
    require(buyer_view(collected, customer) == "", "different customer reads nearby original")
    original_queue = json.loads(detail(None))
    require(psql(url, f"""SELECT count(*) FROM ledger.travel_pickup_transition
          WHERE sale_id='{legacy}' AND target='ready' AND ready_at IS NULL""") == "1",
          "legacy ready timestamp was fabricated upstream")
    expected = {}
    for sale in (ready, collected):
        refs = psql(url, f"""
          SELECT jsonb_build_object('event',e.event_id,'pickup',p.number,'receipt',p.receipt_ref,
            'settlement',jsonb_build_object('amount',j.amount::text,'asset',j.asset_ref,
              'finalizedAt',to_char(j.booked_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
              'pickupReadyAt',to_char(t.ready_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"')))
          FROM ledger.payment a JOIN ledger.journal j ON (a.sale_id,a.evidence_id)=(j.sale_id,j.evidence_id)
          JOIN ledger.pickup p ON p.sale_id=a.sale_id JOIN ledger.fulfillment_outbox e ON e.sale_id=a.sale_id
          JOIN ledger.consumer_receipt ack ON ack.event_id=e.event_id AND ack.effect_key=e.effect_key
          JOIN ledger.travel_pickup_transition t ON t.sale_id=a.sale_id AND t.target='ready'
          WHERE j.sale_id='{sale}' AND e.kind='paid' AND e.state='delivered'
        """)
        require(bool(refs), "fixture lacks the original delivered event and preserved ready transition")
        expected[sale] = json.loads(refs)
        require(expected[sale]["settlement"]["finalizedAt"] < expected[sale]["settlement"]["pickupReadyAt"],
                "fixture did not establish separate authentic timestamps")
        require(buyers[sale].split("|")[-1] == expected[sale]["receipt"], "buyer sees a different receipt")
    require(expected[ready]["event"] != expected[collected]["event"] and
            expected[ready]["pickup"] != expected[collected]["pickup"], "original pickup identities are not distinct")

    snapshot = """
      SELECT version,name,sha256,applied_at FROM nu54_p05_migrations.revision WHERE version<=98 ORDER BY version;
      SELECT to_jsonb(x) FROM ledger.sale x ORDER BY id;
      SELECT to_jsonb(x) FROM ledger.order_checkout x ORDER BY sale_id;
      SELECT to_jsonb(x) FROM ledger.checkout_claim x ORDER BY sale_id;
      SELECT to_jsonb(x) FROM ledger.payment x ORDER BY sale_id;
      SELECT to_jsonb(x) FROM ledger.journal x ORDER BY sale_id;
      SELECT to_jsonb(x) FROM ledger.pickup x ORDER BY sale_id;
      SELECT to_jsonb(x) FROM ledger.travel_pickup_progress x ORDER BY sale_id;
      SELECT to_jsonb(x) FROM ledger.travel_pickup_transition x ORDER BY sale_id,revision;
      SELECT to_jsonb(x) FROM ledger.fulfillment_outbox x ORDER BY sale_id;
      SELECT to_jsonb(x) FROM ledger.consumer_receipt x ORDER BY event_id;
      SELECT oid::regprocedure,proacl FROM pg_proc WHERE proname='kiosk_pickup_queue' ORDER BY oid::regprocedure::text;
    """
    historic = psql(url, snapshot)
    try:
        apply("p05", url, [*predecessor, replace(successor, sql=successor.sql + "\nSELECT 1/0;\n")])
    except MigrationRejected as error:
        require("division by zero" in error.psql_stderr, "unexpected 099 interruption")
    else:
        raise AssertionError("099 interruption committed")
    require(psql(url, "SELECT max(version) FROM nu54_p05_migrations.revision") == "98", "099 journal escaped rollback")
    require(json.loads(detail(ready))[0] == original[ready] and psql(url, snapshot) == historic,
            "099 projection or data escaped rollback")

    apply("p05", url, [*predecessor, successor])
    for sale in sales:
        current = json.loads(detail(sale))[0]
        if sale in expected:
            refs = expected[sale]
            require(current.pop("settlement") == refs["settlement"], "099 changed original price or timestamp")
            require(current["eventRef"] == refs["event"] and current["pickupNo"] == refs["pickup"],
                    "099 output does not bind the original fulfillment event")
        require(current == original[sale], "099 altered old kitchen detail or disclosed unverified evidence")
        payer = nearby_customer if sale == collected else customer
        require(buyer_view(sale, payer) == buyers[sale], "099 changed original customer order")
    require(json.loads(detail(None)) == original_queue, "099 changed the normal queue or leaked protected detail")
    require(detail(ready, include=False) == "[]" and detail(ready, actor=terminal, role="terminal") == "[]" and
            detail(ready, site=other_branch) == "[]", "099 exposed detail outside the explicit operator route")
    require(buyer_view(collected, customer) == "", "099 exposed another customer's nearby order")
    try:
        detail(ready, actor=outsider)
    except subprocess.CalledProcessError as error:
        require("pickup assignment unavailable" in error.stderr, "wrong denial for unassigned operator")
    else:
        raise AssertionError("099 allowed unassigned operator")
    journal = "SELECT version,name,sha256,applied_at FROM nu54_p05_migrations.revision ORDER BY version"
    completed = psql(url, journal)
    apply("p05", url, [*predecessor, successor])
    require(psql(url, snapshot) == historic and psql(url, journal) == completed,
            "099 changed historical data, original ACL or migration receipt")

    psql(url, f"""
      UPDATE ledger.fulfillment_outbox SET lease_until=now()+interval '10 minutes'
        WHERE sale_id='{unacknowledged}' AND state='pending';
    """)
    key = str(uuid.uuid4())
    original = paid_order(1, nearby=True, forward_key=key)
    return tenant, branch, nearby_customer, original, key


def customer_cafe_forward_sql(tenant, branch, buyer, sale, key):
    return f"""
      SELECT ledger.store_forward('{tenant}','{branch}','{buyer}','{sale}','{key}',
        sha256('{sale}-intent'),sha256('{sale}-signed'),sha256('{sale}-approval'),'{branch}',decode(repeat('aa',93),'hex'));
    """


def exercise_customer_cafe_forward_upgrade(url, tenant, branch, buyer, sale, key):
    revisions = migrations(OWNERS["p05"][1])
    predecessor = [revision for revision in revisions if revision.version <= 99]
    successor = next(revision for revision in revisions if revision.version == 100)
    require(len(predecessor) == 99, "099 predecessor is incomplete")
    journal = "SELECT version,name,sha256,applied_at FROM nu54_p05_migrations.revision ORDER BY version"
    view_sql = f"""
      SET ROLE nu54_p05_commerce;
      SELECT id,state,amount,pickup_no,receipt_ref
        FROM ledger.commerce_order_receipt('{tenant}','{branch}','{buyer}','{sale}');
    """
    context_sql = f"""
      SET ROLE nu54_p05_commerce;
      SELECT ledger.menu_checkout_context('{tenant}','{branch}','{buyer}','{sale}')->>'reviewKind';
    """
    original = psql(url, view_sql)
    fields = original.split("|")
    require(len(fields) == 5 and fields[:3] == [sale, "paid", "4000000"] and fields[3] and fields[4],
            "099 fixture lacks the original paid customer receipt")
    uuid.UUID(fields[4])
    snapshot = f"""
      SELECT to_jsonb(r) FROM ledger.command_receipt r
        WHERE result_ref='{sale}' AND operation='payment_forward' ORDER BY request_key;
      SELECT to_jsonb(s) FROM ledger.settlement_submission s WHERE sale_id='{sale}';
      SELECT gas_sponsor_digest FROM ledger.quote_work WHERE sale_id='{sale}';
    """
    original_rows = psql(url, snapshot)
    rows = original_rows.splitlines()
    require(len(rows) == 3 and json.loads(rows[0])["request_key"] == key and
            json.loads(rows[1])["sale_id"] == sale and bool(rows[2]),
            "099 fixture lacks one original forwarding receipt, submission or sponsorship commitment")
    require(psql(url, context_sql) == "", "099 unexpectedly projected the 100 review kind")
    before = psql(url, journal)
    try:
        apply("p05", url, [*predecessor, replace(successor, sql=successor.sql + "\nSELECT 1/0;\n")])
    except MigrationRejected as error:
        require("division by zero" in error.psql_stderr, "unexpected 100 interruption")
    else:
        raise AssertionError("100 interruption committed")
    require(psql(url, journal) == before and psql(url, context_sql) == "" and
            psql(url, view_sql) == original and psql(url, snapshot) == original_rows,
            "100 interruption changed the original customer receipt or admission")
    require(psql(url, "SELECT NOT EXISTS(SELECT FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace "
                      "WHERE n.nspname='ledger' AND p.proname='store_customer_forward')") == "t",
            "100 function escaped rollback")

    upgraded = [*predecessor, successor]
    apply("p05", url, upgraded)
    completed = psql(url, journal)
    require(len(completed.splitlines()) == 100 and completed.startswith(before + "\n"),
            "100 did not preserve the predecessor journal")
    require(psql(url, context_sql) == "cafe", "100 did not classify the original paid cafe order")
    require(psql(url, "SET ROLE nu54_p05_commerce;\n" +
                 customer_cafe_forward_sql(tenant, branch, buyer, sale, key)) == sale,
            "100 replay did not return the original sale")
    require(psql(url, view_sql) == original and psql(url, snapshot) == original_rows,
            "100 replay changed the original receipt, amount or admission count")
    apply("p05", url, upgraded)
    require(psql(url, journal) == completed and psql(url, view_sql) == original and
            psql(url, snapshot) == original_rows and psql(url, context_sql) == "cafe",
            "100 reapplication changed the original customer receipt or admission")


def exercise_customer_cafe_review_upgrade(url, tenant, branch, buyer, sale):
    revisions = migrations(OWNERS["p05"][1])
    original = [revision for revision in revisions if revision.version <= 100]
    successor = next(revision for revision in revisions if revision.version == 101)
    journal = "SELECT version,name,sha256,applied_at FROM nu54_p05_migrations.revision ORDER BY version"
    comparison, other_branch = (str(uuid.uuid4()) for _ in range(2))
    psql(url, f"""
      INSERT INTO ledger.branch VALUES ('{other_branch}','{tenant}','Asia/Seoul','04:00');
      INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,valid_until) VALUES
        ('{branch}','{comparison}','customer',now()+interval '10 minutes'),
        ('{other_branch}','{buyer}','customer',now()+interval '10 minutes');
      INSERT INTO ledger.checkout_claim
        SELECT sale_id,branch_id,'{comparison}',sha256('{sale}-comparison-101'),payer,payer_generation,payer_role,
            now()+interval '10 minutes'
        FROM ledger.checkout_claim WHERE sale_id='{sale}' AND actor_id='{buyer}';
    """)
    creator = psql(url, f"SELECT creator FROM ledger.order_checkout WHERE sale_id='{sale}'")
    stranger = str(uuid.uuid4())
    psql(url, f"INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,terminal_mode,valid_until) "
              f"VALUES ('{branch}','{stranger}','terminal','menu',now()+interval '10 minutes')")
    def review(actor, site=branch):
        raw = psql(url, f"""SET ROLE nu54_p05_commerce;
            SELECT ledger.menu_checkout_context('{tenant}','{site}','{actor}','{sale}');""")
        return json.loads(raw) if raw else None

    def client_creator_review():
        raw = psql(url, f"""BEGIN;
            UPDATE ledger.order_checkout SET creator='{comparison}' WHERE sale_id='{sale}';
            UPDATE ledger.checkout_claim SET expires_at=now()-interval '1 second'
              WHERE sale_id='{sale}' AND actor_id='{comparison}';
            SET LOCAL ROLE nu54_p05_commerce;
            SELECT ledger.menu_checkout_context('{tenant}','{branch}','{comparison}','{sale}');
            ROLLBACK;""")
        return json.loads(raw) if raw else None

    buyer_context = review(buyer)
    creator_context = review(creator)
    customer_creator_context = client_creator_review()
    require(buyer_context and set(buyer_context) == {"reviewKind", "branchLabel", "payerBound"} and
            buyer_context["reviewKind"] == "cafe", "100 cannot open the original paid buyer scope")
    require(creator_context and "reviewKind" not in creator_context and "branchLabel" in creator_context,
            "100 terminal creator lost the original checkout scope")
    require(customer_creator_context and customer_creator_context["reviewKind"] == "cafe" and
            customer_creator_context["payerBound"] is False, "100 paid customer creator lost open authority")
    require(review(comparison) is None and review(stranger) is None and review(buyer, site=other_branch) is None,
            "100 exposed cafe review outside the original order scope")
    history = psql(url, journal)
    apply("p05", url, [*original, successor])
    updated = psql(url, journal)
    require(updated.startswith(history + "\n") and len(updated.splitlines()) == 101,
            "101 did not preserve the 100 journal")
    require(review(buyer) == buyer_context and review(creator) == creator_context and
            client_creator_review() == customer_creator_context and review(comparison) is None and
            review(stranger) is None and review(buyer, site=other_branch) is None,
            "101 changed the original buyer, creator, comparison, terminal or branch scope")
    apply("p05", url, [*original, successor])
    require(psql(url, journal) == updated and review(buyer) == buyer_context, "101 repeat changed original scope")


def exercise_renter_payment_escrow_upgrade(url, tenant, branch):
    revisions = migrations(OWNERS["p05"][1])
    predecessor = [revision for revision in revisions if revision.version <= 101]
    successor = next(revision for revision in revisions if revision.version == 102)
    renter, manager, device, sale, request = (str(uuid.uuid4()) for _ in range(5))
    account = "0x" + "7a" * 20
    asset = "0x" + "a" * 40
    deployment = "8b" * 32

    def commerce(sql):
        return psql(url, "SET ROLE nu54_p05_commerce;" + chr(10) + sql)

    psql(url, f"""
      INSERT INTO ledger.login_subject VALUES ('escrow_fixture_manager','{manager}');
      INSERT INTO ledger.finance_grant VALUES ('{branch}','{manager}','rental_manage',now()+interval '1 hour');
      INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,valid_until)
        VALUES ('{branch}','{renter}','customer',now()+interval '1 hour');
      INSERT INTO ledger.payer_enrollment VALUES('{branch}','{renter}',decode(repeat('7a',20),'hex'),7,2,now()+interval '1 hour');
      INSERT INTO ledger.rental_enrollment(branch_id,device_ref,generation,account_ref,withdraw_ref,revision,
          enrollment_digest,attested_until,attested_at)
        VALUES('{branch}','{device}',7,'{account}','0x'||repeat('7b',20),'v2-fixture',sha256('{device}'),
          now()+interval '1 hour',now()-interval '1 minute');
      INSERT INTO ledger.sale(id,branch_id,origin,origin_ref,state,asset_ref,amount,revision)
        VALUES('{sale}','{branch}','menu',gen_random_uuid(),'issued','{asset}',4000000,1);
      INSERT INTO ledger.order_checkout(sale_id,branch_id,creator,expires_at,intent_digest)
        VALUES ('{sale}','{branch}','{renter}',now()+interval '15 minutes',sha256('{sale}-cart'));
      INSERT INTO ledger.order_line VALUES ('{sale}','{branch}',1,'drip','plain',1,4000000);
      INSERT INTO ledger.checkout_claim VALUES('{sale}','{branch}','{renter}',sha256('{sale}-claim'),
        decode(repeat('7a',20),'hex'),7,2,now()+interval '15 minutes');
      INSERT INTO ledger.quote_work(sale_id,actor_id,claim_digest,quote_id,order_id,nonce,expected_revision,profile_version,
        merchant_signer,payer,payer_generation,payer_role,issued_at,expires_at,fence,lease_until,payload_cipher,
        signed_digest,gas_sponsor_digest)
      VALUES('{sale}','{renter}',sha256('{sale}-claim'),sha256('{sale}-quote'),sha256('{sale}-order'),sha256('{sale}-nonce'),
        0,1,decode(repeat('11',20),'hex'),decode(repeat('7a',20),'hex'),7,2,now()-interval '1 second',
        now()+interval '5 minutes',1,now()+interval '5 minutes',decode('aa','hex'),sha256('{sale}-signed'),sha256('{sale}-sponsor'));
    """)
    offer_scope = f"'{tenant}','{branch}','{manager}','{device}',7,'{renter}','{account}',now()+interval '20 minutes'"
    legacy = commerce(f"SELECT ledger.offer_renter_return({offer_scope});")
    require(commerce(f"SELECT ledger.change_renter_return('{tenant}','{renter}','{legacy}','accept','{account}');") == "t",
            "legacy handover could not be accepted")
    view = f"SELECT ledger.read_renter_return('{tenant}','{renter}','{legacy}');"
    legacy_view = commerce(view)
    journal = "SELECT version,name,sha256,applied_at FROM nu54_p05_migrations.revision ORDER BY version"
    before = psql(url, journal)
    try:
        apply("p05", url, [*predecessor, replace(successor, sql=successor.sql + chr(10) + "SELECT 1/0;" + chr(10))])
    except MigrationRejected as error:
        require("division by zero" in error.psql_stderr, "unexpected 102 interruption")
    else:
        raise AssertionError("102 interruption committed")
    require(psql(url, journal) == before and commerce(view) == legacy_view, "102 interrupted upgrade changed old handover")
    applied = [*predecessor, successor]
    apply("p05", url, applied)
    require(commerce(view) == legacy_view and psql(url, f"""
        SELECT payment_deployment IS NULL FROM ledger.renter_return_assignment WHERE assignment_id='{legacy}';
    """) == "t", "102 changed 101 handover or inferred a deployment")

    def scope(digest):
        return commerce(f"""
          SELECT requires_v2, expires_at>clock_timestamp()
          FROM ledger.renter_payment_scope('{tenant}','{branch}','{renter}','{sale}',sha256('{sale}-signed'),
            decode('{digest}','hex'),false);
        """)

    require(scope(deployment) == "f|", "old handover unexpectedly implied payment authority")
    revoke = lambda assignment: commerce(f"SELECT ledger.change_renter_return('{tenant}','{manager}','{assignment}','revoke','');")
    require(revoke(legacy) == "t", "fixture could not end the legacy handover")
    assignment = commerce(f"SELECT ledger.offer_renter_handover({offer_scope},decode('{deployment}','hex'));")
    require(scope(deployment) == "t|", "unaccepted v2 handover already opened the escrow")
    require(commerce(f"SELECT ledger.change_renter_return('{tenant}','{renter}','{assignment}','accept','{account}');") == "t",
            "v2 handover could not be accepted")
    require(scope(deployment) == "t|t" and scope("9c" * 32) == "t|",
            "accepted handover did not bind solely to the v2 deployment")
    def forward():
        return commerce(f"""
          SELECT sale_ref,new_sponsored_admission FROM ledger.store_customer_escrow_forward(
            '{tenant}','{branch}','{renter}','{sale}','{request}',sha256('{sale}-intent'),sha256('{sale}-signed'),
            sha256('{sale}-approval'),'{branch}',decode(repeat('aa',93),'hex'),sha256('{sale}-sponsor'),
            now()+interval '20 seconds',decode('{deployment}','hex'),now()+interval '15 seconds');
        """)
    require(forward() == f"{sale}|t", "one current v2 rental admission failed")
    require(revoke(assignment) == "t" and scope(deployment) == "t|", "revoked handover still permits new proof")
    require(forward() == f"{sale}|f", "accepted original rental key did not survive handover revocation")
    require(psql(url, f"""SELECT
          (SELECT count(*) FROM ledger.settlement_submission WHERE sale_id='{sale}'),
          (SELECT count(*) FROM ledger.command_receipt WHERE result_ref='{sale}' AND operation='payment_forward');
        """) == "1|1", "rental recovery wrote another receipt or admission")
    after = psql(url, journal)
    require(len(after.splitlines()) == 102 and after.startswith(before + chr(10)), "102 changed predecessor journal")
    apply("p05", url, applied)
    require(psql(url, journal) == after and scope(deployment) == "t|", "102 reapply changed revoked handover")
    return tenant, branch, renter, manager, device, sale, request, account, deployment


def exercise_renter_escrow_deadline_upgrade(url, tenant, branch, renter, manager, device, sale, request, account, deployment):
    revisions = migrations(OWNERS["p05"][1])
    predecessor = [revision for revision in revisions if revision.version <= 102]
    successor = next(revision for revision in revisions if revision.version == 103)
    fresh, held, fresh_key = (str(uuid.uuid4()) for _ in range(3))

    def commerce(sql):
        return psql(url, "SET ROLE nu54_p05_commerce;" + chr(10) + sql)

    for original in (fresh, held):
        psql(url, f"""
          INSERT INTO ledger.sale(id,branch_id,origin,origin_ref,state,asset_ref,amount,revision)
            SELECT '{original}',branch_id,origin,gen_random_uuid(),'issued',asset_ref,amount,1
            FROM ledger.sale WHERE id='{sale}';
          INSERT INTO ledger.order_checkout(sale_id,branch_id,creator,expires_at,intent_digest)
            VALUES('{original}','{branch}','{renter}',now()+interval '15 minutes',sha256('{original}-cart'));
          INSERT INTO ledger.order_line
            SELECT '{original}',branch_id,menu_version,sku,option_key,quantity,price
            FROM ledger.order_line WHERE sale_id='{sale}';
          INSERT INTO ledger.checkout_claim
            SELECT '{original}',branch_id,actor_id,sha256('{original}-claim'),payer,payer_generation,payer_role,
              now()+interval '15 minutes'
            FROM ledger.checkout_claim WHERE sale_id='{sale}' AND actor_id='{renter}';
          INSERT INTO ledger.quote_work(sale_id,actor_id,claim_digest,quote_id,order_id,nonce,expected_revision,profile_version,
            merchant_signer,payer,payer_generation,payer_role,issued_at,expires_at,fence,lease_until,payload_cipher,
            signed_digest,gas_sponsor_digest)
            SELECT '{original}',actor_id,sha256('{original}-claim'),sha256('{original}-quote'),
              sha256('{original}-order'),sha256('{original}-nonce'),0,profile_version,merchant_signer,payer,
              payer_generation,payer_role,now()-interval '1 second',now()+interval '10 minutes',1,
              now()+interval '10 minutes',payload_cipher,sha256('{original}-signed'),sha256('{original}-sponsor')
            FROM ledger.quote_work WHERE sale_id='{sale}';
        """)
    inherited = commerce(f"""SELECT ledger.offer_renter_handover('{tenant}','{branch}','{manager}','{device}',7,
        '{renter}','{account}',now()+interval '20 minutes',decode('{deployment}','hex'));""")
    require(commerce(f"SELECT ledger.change_renter_return('{tenant}','{renter}','{inherited}','accept','{account}');") == "t",
            "102 funded handover not accepted")
    view = f"SELECT ledger.read_renter_return('{tenant}','{renter}','{inherited}');"
    original_view = commerce(view)
    payer_deadline = psql(url, f"""
      UPDATE ledger.payer_enrollment SET expires_at=date_trunc('second',now())+interval '6 minutes'
      WHERE branch_id='{branch}' AND actor_id='{renter}' RETURNING expires_at;
    """)

    def scope(original):
        return commerce(f"""SELECT requires_v2,expires_at
          FROM ledger.renter_payment_scope('{tenant}','{branch}','{renter}','{original}',sha256('{original}-signed'),
            decode('{deployment}','hex'),false);""")

    def forward(original, key, bounded):
        command = "store_customer_escrow_forward_bounded" if bounded else "store_customer_escrow_forward"
        return commerce(f"""SELECT * FROM ledger.{command}(
          '{tenant}','{branch}','{renter}','{original}','{key}',sha256('{original}-intent'),sha256('{original}-signed'),
          sha256('{original}-approval'),'{branch}',decode(repeat('aa',93),'hex'),sha256('{original}-sponsor'),
          now()+interval '8 minutes',decode('{deployment}','hex'),now()+interval '7 minutes');""")

    historical_scope = scope(fresh)
    require(historical_scope.startswith("t|") and historical_scope != f"t|{payer_deadline}",
            "102 fixture did not reproduce authorization beyond payer enrollment")
    journal = "SELECT version,name,sha256,applied_at FROM nu54_p05_migrations.revision ORDER BY version"
    rows = f"""SELECT to_jsonb(r) FROM ledger.command_receipt r
                  WHERE r.result_ref='{sale}' AND r.operation='payment_forward';
               SELECT to_jsonb(s) FROM ledger.settlement_submission s WHERE s.sale_id='{sale}';"""
    before, historical_rows = psql(url, journal), psql(url, rows)
    try:
        apply("p05", url, [*predecessor, replace(successor, sql=successor.sql + chr(10) + "SELECT 1/0;" + chr(10))])
    except MigrationRejected as error:
        require("division by zero" in error.psql_stderr, "unexpected 103 interruption")
    else:
        raise AssertionError("103 interruption committed")
    require(psql(url, journal) == before and psql(url, rows) == historical_rows and
            commerce(view) == original_view and scope(fresh) == historical_scope,
            "103 interruption changed old deadline, handover, submission or receipt")
    applied = [*predecessor, successor]
    apply("p05", url, applied)
    after = psql(url, journal)
    require(len(after.splitlines()) == 103 and after.startswith(before + chr(10)), "103 changed predecessor journal")
    require(scope(fresh) == f"t|{payer_deadline}" and commerce(view) == original_view,
            "103 proof deadline did not stop at the exact payer enrollment limit")
    require(forward(fresh, fresh_key, True) == f"{fresh}|t|{payer_deadline}",
            "103 new admission returned an expiry beyond the payer enrollment")
    psql(url, f"""UPDATE ledger.payer_enrollment SET expires_at=now()-interval '1 second'
                  WHERE branch_id='{branch}' AND actor_id='{renter}';""")
    require(scope(held) == "t|", "expired enrollment remained available for payment review")
    try:
        forward(held, str(uuid.uuid4()), True)
    except subprocess.CalledProcessError as error:
        require("current rental escrow unavailable" in error.stderr, "unexpected expired enrollment result")
    else:
        raise AssertionError("103 admitted a new payment after payer enrollment expired")
    require(forward(fresh, fresh_key, True) == f"{fresh}|f|" and forward(sale, request, False) == f"{sale}|f",
            "expired enrollment blocked retrieval of the accepted original")
    require(commerce(f"SELECT ledger.change_renter_return('{tenant}','{manager}','{inherited}','revoke','');") == "t",
            "fixture could not revoke 102 handover")
    require(scope(held) == "t|" and forward(sale, request, False) == f"{sale}|f" and psql(url, rows) == historical_rows,
            "revoked handover changed existing payment or opened new payment review")
    apply("p05", url, applied)
    require(psql(url, journal) == after and psql(url, rows) == historical_rows,
            "103 repeat changed original payment, command receipt or journal")


def verify_drift(url):
    p05 = migrations(OWNERS["p05"][1])
    changed = replace(p05[0], digest="0" * 64)
    try:
        apply("p05", url, [changed, *p05[1:]])
    except MigrationRejected as error:
        require("ERROR:  migration digest mismatch for 001_backoffice.sql" in error.psql_stderr,
                "expected applied SQL digest mismatch")
    else:
        raise AssertionError("applied SQL digest drift was accepted")

    try:
        apply("p05", url, p05[:-1])
    except MigrationRejected as error:
        require("ERROR:  database has a newer p05 migration" in error.psql_stderr,
                "expected rejection for newer database")
    else:
        raise AssertionError("older runner accepted a newer database")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--socket", type=Path, required=True, help="private local scratch PostgreSQL socket directory")
    parser.add_argument("--port", type=int, default=5432)
    parser.add_argument("--user", required=True)
    args = parser.parse_args()
    socket = args.socket.resolve()
    if not socket.is_relative_to(Path("/tmp").resolve()) or not socket.is_dir() or socket.stat().st_uid != os.getuid():
        parser.error("scratch socket must be in a user-owned temporary directory")
    if not args.user.isidentifier() or " " in str(socket):
        parser.error("invalid scratch connection")
    base = f"host={socket} port={args.port} user={args.user} sslmode=disable"
    admin = f"{base} dbname=postgres"
    for legacy in (False, True):
        name = f"nu54_migration_{uuid.uuid4().hex}"
        url = f"{base} dbname={name}"
        psql(admin, f"CREATE DATABASE {name};")
        try:
            print(f"=== {'legacy v1' if legacy else 'empty'} database ===", flush=True)
            if legacy:
                for owner in ("p05", "p07"):
                    psql(url, migrations(OWNERS[owner][1])[0].sql)
                psql(url, LEGACY_DATA)
                before = psql(url, LEGACY_SNAPSHOT)
                p09 = migrations(OWNERS["p09"][1])
                apply("p09", url, p09[:1])
                psql(url, """INSERT INTO travel.catalog_revision VALUES(
                    1,'legacy_fixture','synthetic','2026-11-01Z','public fixture',decode(repeat('ab',64),'hex'));""")
                p09_before = psql(url, "TABLE travel.catalog_revision;")
                exercise_interruption(url)
                refuse_uncatalogued_field(url)
                exercise_merchant_gas_upgrade(url)
            apply_all(url)
            assert_roles_and_feature_scope(url)
            assert_field_policy(url)
            if legacy:
                require(before == psql(url, LEGACY_SNAPSHOT), "v1 row changed")
                require(p09_before == psql(url, "TABLE travel.catalog_revision;"), "prior P09 revision changed")
                apply_all(url)
                verify_drift(url)
                require(before == psql(url, LEGACY_SNAPSHOT), "v1 repeat changed a row")
                concurrent_commands(url)
        except subprocess.CalledProcessError as error:
            print(error.stderr or "psql failed", flush=True)
            raise
        finally:
            psql(admin, f"DROP DATABASE {name};")
    name = f"nu54_migration_return_pickup_{uuid.uuid4().hex}"
    psql(admin, f"CREATE DATABASE {name};")
    try:
        exercise_return_pickup_upgrade(f"{base} dbname={name}")
    finally:
        psql(admin, f"DROP DATABASE {name};")
    print(RETURN_PICKUP_UPGRADE_RESULT, flush=True)
    name = f"nu54_migration_return_issuance_archive_{uuid.uuid4().hex}"
    psql(admin, f"CREATE DATABASE {name};")
    try:
        url = f"{base} dbname={name}"
        original = exercise_return_issuance_archive_upgrade(url)
        print(RETURN_ISSUANCE_ARCHIVE_UPGRADE_RESULT, flush=True)
        exercise_return_receipt_upgrade(url, *original)
        print(RETURN_RECEIPT_UPGRADE_RESULT, flush=True)
    finally:
        psql(admin, f"DROP DATABASE {name};")
    name = f"nu54_migration_travel_pickup_{uuid.uuid4().hex}"
    psql(admin, f"CREATE DATABASE {name};")
    try:
        exercise_travel_pickup_upgrade(f"{base} dbname={name}")
    finally:
        psql(admin, f"DROP DATABASE {name};")
    print(TRAVEL_PICKUP_UPGRADE_RESULT, flush=True)
    name = f"nu54_migration_cafe_pickup_{uuid.uuid4().hex}"
    psql(admin, f"CREATE DATABASE {name};")
    try:
        url = f"{base} dbname={name}"
        original = exercise_cafe_pickup_upgrade(url)
        print(CAFE_PICKUP_UPGRADE_RESULT, flush=True)
        exercise_customer_cafe_forward_upgrade(url, *original)
        print(CUSTOMER_CAFE_FORWARD_UPGRADE_RESULT, flush=True)
        exercise_customer_cafe_review_upgrade(url, *original[:-1])
        print(CUSTOMER_CAFE_REVIEW_UPGRADE_RESULT, flush=True)
        rental = exercise_renter_payment_escrow_upgrade(url, *original[:2])
        print(RENTER_PAYMENT_ESCROW_UPGRADE_RESULT, flush=True)
        exercise_renter_escrow_deadline_upgrade(url, *rental)
        print(RENTER_ESCROW_DEADLINE_UPGRADE_RESULT, flush=True)
    finally:
        psql(admin, f"DROP DATABASE {name};")
    print("PASS  empty and v1 schemas, role boundaries, idempotency, interruption, and drift")


if __name__ == "__main__":
    main()

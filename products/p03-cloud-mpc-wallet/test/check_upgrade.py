"""Verify the P03 per-wallet request migration preserves populated v1 rows."""
import os
from pathlib import Path
import subprocess
import sys

product = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(product.parent / "p10-platform/database"))
from migrate import apply, migrations  # noqa: E402

def sql(database, text):
    return subprocess.run(["psql", "-X", "-v", "ON_ERROR_STOP=1", "-At", "-d", database, "-c", text],
                          capture_output=True, text=True, check=True, timeout=30).stdout.strip()

if not os.environ["PGHOST"].startswith(("/tmp/", "/private/tmp/")):
    raise ValueError("migration upgrade verification requires a scratch PostgreSQL socket")

sql("postgres", "CREATE DATABASE p03_upgrade")
revisions = migrations(product / "migrations")
apply("p03", "p03_upgrade", revisions[:1])
subject = "97eed890-89be-4dde-99ba-11e812358d91"
wallet = "ae664968-e6a4-483b-8a55-b71d115f2f21"
key = "86402f2b-582e-4bb7-94d2-0471ebcd2677"
pending = "5580c515-f569-4efb-91c4-bfbeefb58e4f"
sql("p03_upgrade", f"""
INSERT INTO wallet_mpc.identity(subject_ref) VALUES ('{subject}');
INSERT INTO wallet_mpc.wallet(wallet_id,subject_ref,profile,state)
VALUES ('{wallet}','{subject}','sepolia-native-test-v1','creating');
INSERT INTO wallet_mpc.operation(operation_id,wallet_id,kind,epoch,session,fingerprint,state,
  approval_proof,not_before,expires_at)
VALUES ('{key}','{wallet}','dkg',1,'{{"evidence":"known epoch"}}',decode(repeat('ab',32),'hex'),
  'succeeded',NULL,now(),now()+interval '20 minutes'),
 ('{pending}','{wallet}','refresh',2,'{{"evidence":"outstanding round"}}',decode(repeat('ac',32),'hex'),
  'running',decode(repeat('01',64),'hex'),now(),now()+interval '20 minutes');
INSERT INTO wallet_mpc.key_epoch(wallet_id,epoch,operation_id,activated_at) VALUES ('{wallet}',1,'{key}',now());
UPDATE wallet_mpc.wallet SET active_epoch=1,public_key=decode('04'||repeat('ab',64),'hex'),state='blocked'
WHERE wallet_id='{wallet}';
INSERT INTO wallet_mpc.participant_ack(operation_id,participant_ref,evidence_kind,evidence_digest)
VALUES ('{pending}','legacy-b1','fenced',decode(repeat('10',32),'hex'));
""")
expected = sql("p03_upgrade", """
SELECT o.operation_id,o.session,o.state,encode(o.approval_proof,'hex'),w.active_epoch,encode(w.public_key,'hex'),
 (SELECT count(*) FROM wallet_mpc.participant_ack a WHERE a.operation_id=o.operation_id)
FROM wallet_mpc.operation o JOIN wallet_mpc.wallet w USING(wallet_id) ORDER BY o.operation_id
""")
apply("p03", "p03_upgrade", revisions[:5])
actual = sql("p03_upgrade", """
SELECT o.operation_id,o.session,o.state,encode(o.approval_proof,'hex'),w.active_epoch,encode(w.public_key,'hex'),
 (SELECT count(*) FROM wallet_mpc.participant_ack a WHERE a.operation_id=o.operation_id)
FROM wallet_mpc.operation o JOIN wallet_mpc.wallet w USING(wallet_id)
WHERE o.request_ref=o.operation_id ORDER BY o.operation_id
""")
if actual != expected:
    raise AssertionError("P03 upgrade changed legacy wallet, ceremony, or its receipt")
print("PASS P03 migration preserves legacy wallet, active epoch, pending ceremony, and partial ACK")

# A version 5 wallet may already have a running signature and completed
# evidence. Version 6 widens the domain without reclassifying either.
running = "652b9ead-046e-44ac-8948-1e032a8fd118"
signed = "d8a69bd7-e01a-4581-9909-66484211c401"
sql("p03_upgrade", f"""
INSERT INTO wallet_mpc.operation(operation_id,request_ref,wallet_id,kind,epoch,session,fingerprint,state,
  approval_proof,not_before,expires_at)
VALUES ('{running}','{running}','{wallet}','sign',1,'{{"evidence":"running sign"}}',decode(repeat('aa',32),'hex'),
  'running',decode(repeat('cd',64),'hex'),now(),now()+interval '20 minutes'),
 ('{signed}','{signed}','{wallet}','sign',1,'{{"evidence":"completed sign"}}',decode(repeat('ad',32),'hex'),
  'succeeded',NULL,now(),now()+interval '20 minutes');
INSERT INTO wallet_mpc.signature_result(operation_id,signature,recovery_id,expires_at)
VALUES ('{signed}',decode(repeat('02',70),'hex'),0,now()+interval '20 minutes');
""")
snapshot = """
SELECT operation_id,request_ref,state,encode(approval_proof,'hex'),encode(r.signature,'hex')
FROM wallet_mpc.operation LEFT JOIN wallet_mpc.signature_result r USING(operation_id)
ORDER BY operation_id
"""
expected = sql("p03_upgrade", snapshot)
apply("p03", "p03_upgrade", revisions)
if sql("p03_upgrade", snapshot) != expected:
    raise AssertionError("P03 W21 migration changed existing signing or recovery evidence")
print("PASS P03 v6 migration preserves legacy approval and signature receipts")

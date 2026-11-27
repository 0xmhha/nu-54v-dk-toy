-- Transactional core regression. Every row in this fixture is synthetic.
BEGIN;
INSERT INTO ledger.branch(id,tenant_ref,timezone,close_local)
VALUES ('838403f1-2888-41f2-b7bb-766a14110001','838403f1-2888-41f2-b7bb-766a14110002','Asia/Seoul','04:00');
INSERT INTO ledger.sale(id,branch_id,origin,origin_ref,state,asset_ref,amount)
VALUES ('838403f1-2888-41f2-b7bb-766a14110003','838403f1-2888-41f2-b7bb-766a14110001','pos',
 '838403f1-2888-41f2-b7bb-766a14110004','checking','0x1111111111111111111111111111111111111111',100);
INSERT INTO ledger.quote_work(sale_id,actor_id,claim_digest,quote_id,order_id,nonce,expected_revision,profile_version,
 merchant_signer,payer,payer_generation,payer_role,issued_at,expires_at,fence,lease_until,payload_cipher,signed_digest)
VALUES ('838403f1-2888-41f2-b7bb-766a14110003','838403f1-2888-41f2-b7bb-766a14110005',
 decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),decode(repeat('33',32),'hex'),decode(repeat('44',32),'hex'),
 0,1,decode(repeat('55',20),'hex'),decode(repeat('66',20),'hex'),1,2,'2026-11-20 01:00Z','2026-11-20 01:01Z',
 1,'2026-11-20 01:00Z',decode('aa','hex'),decode(repeat('77',32),'hex'));

DO $test$
DECLARE
    sale_ uuid := '838403f1-2888-41f2-b7bb-766a14110003';
    quote_ bytea := decode(repeat('77',32),'hex');
    stream_ uuid := gen_random_uuid();
    proof_ uuid := gen_random_uuid();
    tx_ text := '0x'||repeat('aa',32);
    block_ text := '0x'||repeat('bb',32);
BEGIN
    PERFORM ledger.confirm_payment(sale_,quote_,stream_,proof_,tx_,1,block_,
        '2026-11-20 01:00:45Z'::timestamptz,
        '0x1111111111111111111111111111111111111111',100,
        '0x6666666666666666666666666666666666666666',1,1,'0x'||repeat('cc',32));
    PERFORM ledger.confirm_payment(sale_,quote_,stream_,gen_random_uuid(),tx_,1,block_,
        '2026-11-20 01:00:45Z'::timestamptz,
        '0x1111111111111111111111111111111111111111',100,
        '0x6666666666666666666666666666666666666666',1,1,'0x'||repeat('cc',32));
    IF (SELECT count(*) FROM ledger.payment WHERE sale_id=sale_) <> 1
       OR (SELECT count(*) FROM ledger.journal WHERE sale_id=sale_) <> 1
       OR (SELECT count(*) FROM ledger.fulfillment_outbox WHERE sale_id=sale_) <> 1
       OR (SELECT count(*) FROM ledger.pickup WHERE sale_id=sale_) <> 1 THEN
        RAISE EXCEPTION 'payment effects were not exactly once';
    END IF;
END
$test$;
ROLLBACK;

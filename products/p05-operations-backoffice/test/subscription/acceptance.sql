-- Isolated fixture. The subscription commands are exercised as their runtime capabilities.
BEGIN;
INSERT INTO ledger.branch VALUES ('12000000-0000-4000-8000-000000000001','12000000-0000-4000-8000-000000000002','Asia/Seoul','04:00');
INSERT INTO ledger.commerce_grant VALUES
 ('12000000-0000-4000-8000-000000000001','12000000-0000-4000-8000-000000000003','customer',NULL,now()+interval '1 hour'),
 ('12000000-0000-4000-8000-000000000001','12000000-0000-4000-8000-000000000004','customer',NULL,now()+interval '1 hour');
INSERT INTO ledger.branch VALUES ('12000000-0000-4000-8000-000000000011','12000000-0000-4000-8000-000000000002','Asia/Seoul','04:00');
INSERT INTO ledger.commerce_grant VALUES
 ('12000000-0000-4000-8000-000000000011','12000000-0000-4000-8000-000000000004','customer',NULL,now()+interval '1 hour');

INSERT INTO ledger.subscription_offer(id,branch_id,label,terms_version,terms_digest,cadence,asset_ref,amount,fulfillment_kind)
VALUES ('12000000-0000-4000-8000-000000000005','12000000-0000-4000-8000-000000000001','Weekly test','v1',
  sha256('weekly fixture'::bytea),'weekly','0x1111111111111111111111111111111111111111',30,'physical');

INSERT INTO ledger.subscription_offer(id,branch_id,series_ref,label,terms_version,terms_digest,cadence,asset_ref,amount,fulfillment_kind)
SELECT '12000000-0000-4000-8000-000000000008',branch_id,series_ref,'Weekly revised','v2',
  sha256('revised weekly fixture'::bytea),cadence,asset_ref,40,'digital'
FROM ledger.subscription_offer WHERE id='12000000-0000-4000-8000-000000000005';

DO $test$
DECLARE
    tenant constant uuid := '12000000-0000-4000-8000-000000000002';
    branch constant uuid := '12000000-0000-4000-8000-000000000001';
    foreign_branch constant uuid := '12000000-0000-4000-8000-000000000011';
    actor constant uuid := '12000000-0000-4000-8000-000000000003';
    other constant uuid := '12000000-0000-4000-8000-000000000004';
    offer constant uuid := '12000000-0000-4000-8000-000000000005';
    request constant uuid := '12000000-0000-4000-8000-000000000006';
    digest constant bytea := sha256('weekly fixture'::bytea);
    revised constant uuid := '12000000-0000-4000-8000-000000000008';
    revision_digest constant bytea := sha256('revised weekly fixture'::bytea);
    skip_key constant uuid := '12000000-0000-4000-8000-000000000010';
    replacement uuid;
    schedule uuid;
    first uuid;
    physical_invoice uuid;
    physical_sale uuid;
    expired_sale uuid;
    resolution uuid := gen_random_uuid();
    quote_cutoff timestamptz := date_trunc('second',statement_timestamp()) - interval '1 minute';
    proof constant bytea := sha256('fixture merchant quote'::bytea);
BEGIN
    IF ledger.next_subscription_cycle('2024-01-31','monthly',31)<>'2024-02-29'
       OR ledger.next_subscription_cycle('2024-02-29','monthly',31)<>'2024-03-31'
       OR ('2024-03-01'::timestamp AT TIME ZONE 'America/New_York')<>'2024-03-01T05:00Z'
       OR ('2024-04-01'::timestamp AT TIME ZONE 'America/New_York')<>'2024-04-01T04:00Z' THEN
       RAISE EXCEPTION 'subscription date lost anchor, leap day or zone offset';
    END IF;
    schedule := ledger.accept_subscription(tenant,branch,actor,offer,digest,request);
    IF schedule IS DISTINCT FROM ledger.accept_subscription(tenant,branch,actor,offer,digest,request) THEN
        RAISE EXCEPTION 'same consent request changed identity';
    END IF;
    first := ledger.make_due_invoice();
    IF first IS NULL OR ledger.make_due_invoice() IS NOT NULL THEN
        RAISE EXCEPTION 'one due cycle must create exactly one invoice';
    END IF;
    IF EXISTS(SELECT FROM ledger.subscription_invoices(tenant,branch,other))
       OR (SELECT count(*) FROM ledger.subscription_invoices(tenant,branch,actor))<>1 THEN
        RAISE EXCEPTION 'subscription invoices do not preserve ownership';
    END IF;
    BEGIN
      PERFORM ledger.accept_subscription(tenant,branch,actor,revised,revision_digest,gen_random_uuid());
      RAISE EXCEPTION 'ordinary accept installed two versions without explicit reconsent';
    EXCEPTION WHEN SQLSTATE 'P0501' THEN NULL;
    END;
    BEGIN
      PERFORM ledger.skip_subscription_invoice(tenant,branch,other,first,skip_key);
      RAISE EXCEPTION 'foreign account skipped this invoice';
    EXCEPTION WHEN insufficient_privilege THEN NULL;
    END;
    IF ledger.skip_subscription_invoice(tenant,branch,actor,first,skip_key)<>first THEN
        RAISE EXCEPTION 'same-cycle skip changed invoice identity';
    END IF;
    IF ledger.skip_subscription_invoice(tenant,branch,actor,first,skip_key)<>first THEN
        RAISE EXCEPTION 'same-cycle skip replay changed invoice identity';
    END IF;
    BEGIN
      PERFORM ledger.skip_subscription_invoice(tenant,branch,actor,gen_random_uuid(),skip_key);
      RAISE EXCEPTION 'subscription skip key accepted a different invoice';
    EXCEPTION WHEN SQLSTATE 'P0501' THEN NULL;
    END;
    IF (SELECT state FROM ledger.sale WHERE id=(SELECT sale_id FROM ledger.invoice WHERE id=first))<>'closed' THEN
        RAISE EXCEPTION 'same-cycle skip did not close the unissued checkout';
    END IF;
    IF NOT EXISTS (SELECT FROM ledger.subscription_invoice_rows(tenant,branch,actor)
        WHERE id=first AND lifecycle->>'stage'='closed' AND NOT lifecycle ? 'quoteRef') THEN
        RAISE EXCEPTION 'unsigned skip was confused with a signed quote expiry';
    END IF;
    PERFORM ledger.change_subscription(tenant,branch,actor,schedule,0,'pause',gen_random_uuid());
    IF ledger.make_due_invoice() IS NOT NULL THEN RAISE EXCEPTION 'paused subscription created an invoice'; END IF;
    PERFORM ledger.change_subscription(tenant,branch,actor,schedule,1,'resume',gen_random_uuid());
    replacement := ledger.reconsent_subscription(tenant,branch,actor,schedule,2,revised,revision_digest,'12000000-0000-4000-8000-000000000009');
    IF replacement = schedule OR (SELECT amount FROM ledger.subscription WHERE id=replacement)<>40
      OR (SELECT amount FROM ledger.subscription WHERE id=schedule)<>30
      OR (SELECT next_cycle FROM ledger.subscription WHERE id=replacement)<=(SELECT cycle_key FROM ledger.invoice WHERE id=first)
      OR ledger.make_due_invoice() IS NOT NULL THEN
      RAISE EXCEPTION 'reconsent did not preserve old version and adopt exact new terms';
    END IF;
    PERFORM ledger.change_subscription(tenant,branch,actor,replacement,0,'cancel',gen_random_uuid());
    IF ledger.make_due_invoice() IS NOT NULL
       OR (SELECT state FROM ledger.sale WHERE id=(SELECT sale_id FROM ledger.invoice WHERE id=first))<>'closed' THEN
        RAISE EXCEPTION 'cancel did not close unissued order and prevent future cycles';
    END IF;
    PERFORM ledger.accept_subscription(tenant,branch,other,offer,digest,gen_random_uuid());
    physical_invoice := ledger.make_due_invoice();
    SELECT sale_id INTO STRICT physical_sale FROM ledger.invoice WHERE id=physical_invoice AND fulfillment_kind='physical';
    UPDATE ledger.sale SET state='checking' WHERE id=physical_sale;
    INSERT INTO ledger.quote_work(sale_id,actor_id,claim_digest,quote_id,order_id,nonce,expected_revision,profile_version,
        merchant_signer,payer,payer_generation,payer_role,issued_at,expires_at,fence,lease_until,payload_cipher,signed_digest)
      VALUES(physical_sale,other,sha256('claim'::bytea),sha256('quote id'::bytea),sha256('order'::bytea),sha256('nonce'::bytea),
        0,1,decode(repeat('55',20),'hex'),decode(repeat('66',20),'hex'),1,2,quote_cutoff-interval '1 minute',quote_cutoff,
        1,quote_cutoff-interval '1 minute',decode('aa','hex'),proof);
    IF NOT EXISTS (SELECT FROM ledger.commerce_order_status_with_invoice(tenant,branch,other,physical_sale)
        WHERE subscription->>'stage'='reconcile' AND subscription->>'termsVersion'='v1'
          AND (subscription->>'quoteExpiresAt')::timestamptz=quote_cutoff
          AND (subscription->>'dueAt')::timestamptz>quote_cutoff)
        OR EXISTS (SELECT FROM ledger.commerce_order_status_with_invoice(tenant,branch,actor,physical_sale))
        OR EXISTS (SELECT FROM ledger.subscription_invoice_rows(tenant,branch,actor) WHERE sale_id=physical_sale)
        OR EXISTS (SELECT FROM ledger.commerce_order_status_with_invoice(tenant,foreign_branch,other,physical_sale))
        OR EXISTS (SELECT FROM ledger.subscription_invoice_rows(tenant,foreign_branch,other)) THEN
        RAISE EXCEPTION 'late signed invoice lost its independent deadline, original terms or scope';
    END IF;
    PERFORM ledger.confirm_payment(physical_sale,proof,gen_random_uuid(),gen_random_uuid(),'0x'||repeat('aa',32),1,
       '0x'||repeat('bb',32),quote_cutoff-interval '15 seconds','0x'||repeat('11',20),30,'0x'||repeat('66',20),1,1,'0x'||repeat('cc',32));
    IF (SELECT count(*) FROM ledger.pickup WHERE sale_id=physical_sale)<>1
       OR (SELECT count(*) FROM ledger.fulfillment_outbox WHERE sale_id=physical_sale)<>1
       OR EXISTS(SELECT FROM ledger.subscription_entitlement WHERE invoice_id=physical_invoice) THEN
        RAISE EXCEPTION 'physical contract lost its pickup or incorrectly received a digital entitlement';
    END IF;
    IF NOT EXISTS (SELECT FROM ledger.subscription_invoice_rows(tenant,branch,other)
        WHERE sale_id=physical_sale AND lifecycle->>'stage'='paid') THEN
        RAISE EXCEPTION 'late finalized payment did not supersede reconcile';
    END IF;

    UPDATE ledger.subscription SET next_due_at=clock_timestamp()-interval '1 minute'
      WHERE customer_scope=other AND state='active';
    first := ledger.make_due_invoice();
    SELECT sale_id INTO STRICT expired_sale FROM ledger.invoice WHERE id=first;
    UPDATE ledger.sale SET state='issued' WHERE id=expired_sale;
    INSERT INTO ledger.quote_work(sale_id,actor_id,claim_digest,quote_id,order_id,nonce,expected_revision,profile_version,
        merchant_signer,payer,payer_generation,payer_role,issued_at,expires_at,fence,lease_until,payload_cipher,signed_digest)
      SELECT expired_sale,actor_id,claim_digest,sha256('expiry quote id'::bytea),
        sha256('expiry order'::bytea),sha256('expiry nonce'::bytea),expected_revision,profile_version,
        merchant_signer,payer,payer_generation,payer_role,issued_at,expires_at,fence,lease_until,payload_cipher,signed_digest
      FROM ledger.quote_work WHERE sale_id=physical_sale;
    IF NOT EXISTS (SELECT FROM ledger.subscription_invoice_rows(tenant,branch,other)
        WHERE sale_id=expired_sale AND lifecycle->>'stage'='reconcile') THEN
        RAISE EXCEPTION 'expired clock alone resolved a signed invoice';
    END IF;
    IF NOT ledger.close_unpaid(expired_sale,proof,resolution,quote_cutoff+interval '1 second') THEN
        RAISE EXCEPTION 'canonical unpaid resolution did not close the signed invoice';
    END IF;
    IF NOT EXISTS (SELECT FROM ledger.commerce_order_status_with_invoice(tenant,branch,other,expired_sale)
          WHERE subscription->>'stage'='expired' AND subscription ? 'quoteRef' AND receipt_ref IS NULL) THEN
        RAISE EXCEPTION 'canonical unpaid resolution did not resolve the signed invoice';
    END IF;
END
$test$;
ROLLBACK;

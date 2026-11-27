DO $$
DECLARE
    tenant_ constant uuid := '00000000-0000-4000-8000-000000000001';
    branch_ constant uuid := '77390000-0000-4000-8000-000000000001';
    terminal_ constant uuid := '77390000-0000-4000-8000-000000000002';
    customer_a constant uuid := '77390000-0000-4000-8000-000000000004';
    customer_b constant uuid := '77390000-0000-4000-8000-000000000005';
    profile_ constant bytea := decode(repeat('03',32),'hex');
    signer_ constant bytea := decode(repeat('09',20),'hex');
    future_nonce constant bytea := decode(repeat('2c',32),'hex');
    issued ledger.pos_order;
    held ledger.pos_order;
    q ledger.quote_work;
    replay record;
BEGIN
    SELECT * INTO STRICT issued FROM ledger.pos_order WHERE branch_id=branch_ AND external_ref='legacy-issued';
    SELECT * INTO STRICT held FROM ledger.pos_order WHERE branch_id=branch_ AND external_ref='legacy-held';
    SELECT * INTO STRICT q FROM ledger.quote_work WHERE sale_id=held.sale_id;
    -- Time spent replaying the migration journal varies by CI host. Check
    -- the previously prepared fence as live first, then explicitly expire it.
    UPDATE ledger.quote_work SET lease_until=LEAST(expires_at,clock_timestamp()+interval '30 seconds')
        WHERE sale_id=held.sale_id;
    SET LOCAL ROLE nu54_p05_commerce;
    IF ledger.pos_selection_held(tenant_,branch_,terminal_,issued.handoff_ref)
       OR NOT ledger.pos_selection_held(tenant_,branch_,terminal_,held.handoff_ref) THEN
       RAISE EXCEPTION 'committed and held legacy POS orders are not distinguished';
    END IF;
    SELECT * INTO STRICT replay FROM ledger.prepare_quote(tenant_,branch_,customer_a,issued.sale_id,1,gen_random_uuid(),
        decode(repeat('08',32),'hex'),decode(repeat('04',32),'hex'),profile_,signer_,
        decode(repeat('2a',32),'hex'),decode(repeat('2b',32),'hex'),future_nonce);
    IF replay.signed_digest IS DISTINCT FROM decode(repeat('0e',32),'hex') OR
       replay.payload_cipher IS DISTINCT FROM decode(repeat('0d',765),'hex') OR
       replay.nonce IS DISTINCT FROM decode(repeat('0c',32),'hex') THEN
       RAISE EXCEPTION 'signed original changed';
    END IF;
    BEGIN
        PERFORM FROM ledger.prepare_quote(tenant_,branch_,customer_b,issued.sale_id,1,gen_random_uuid(),
            decode(repeat('08',32),'hex'),decode(repeat('05',32),'hex'),profile_,signer_,
            decode(repeat('2a',32),'hex'),decode(repeat('2b',32),'hex'),future_nonce);
        RAISE EXCEPTION 'a different historical claimant rebound a committed quote';
    EXCEPTION WHEN SQLSTATE '42501' THEN NULL;
    END;
    BEGIN
        PERFORM ledger.finish_quote(tenant_,branch_,terminal_,held.sale_id,q.claim_digest,profile_,q.fence,
            decode(repeat('0d',765),'hex'),decode(repeat('0e',32),'hex'));
        RAISE EXCEPTION 'historical in-flight draft committed after upgrade';
    EXCEPTION WHEN SQLSTATE 'P0703' THEN NULL;
    END;
    BEGIN
        PERFORM FROM ledger.pos_nearby_checkout(tenant_,branch_,terminal_,held.sale_id,customer_a,gen_random_uuid(),'ab1234cd',false);
        RAISE EXCEPTION 'nearby returned a claim from a historical held order';
    EXCEPTION WHEN SQLSTATE '42501' THEN NULL;
    END;
    SELECT * INTO STRICT replay FROM ledger.pos_nearby_checkout(tenant_,branch_,terminal_,issued.sale_id,
        customer_a,gen_random_uuid(),'ab1234cd',false);
    IF replay.claim_digest IS DISTINCT FROM decode(repeat('04',32),'hex') THEN
        RAISE EXCEPTION 'nearby lost the existing signed claimant';
    END IF;
    BEGIN
        PERFORM FROM ledger.pos_nearby_checkout(tenant_,branch_,terminal_,issued.sale_id,customer_b,gen_random_uuid(),'ab1234cd',false);
        RAISE EXCEPTION 'nearby returned a different historical claim after one was signed';
    EXCEPTION WHEN SQLSTATE '42501' THEN NULL;
    END;
    RESET ROLE;
    -- Model recovery after the previous process' lease passed without changing
    -- the claims or installing a new ciphertext.
    UPDATE ledger.quote_work SET lease_until=clock_timestamp()-interval '1 second' WHERE sale_id=held.sale_id;
    SET LOCAL ROLE nu54_p05_commerce;
    BEGIN
        PERFORM ledger.finish_quote(tenant_,branch_,terminal_,held.sale_id,q.claim_digest,profile_,q.fence,
            decode(repeat('0d',765),'hex'),decode(repeat('0e',32),'hex'));
        RAISE EXCEPTION 'expired historical signing fence was accepted';
    EXCEPTION WHEN SQLSTATE 'P0702' THEN NULL;
    END;
    BEGIN
        PERFORM FROM ledger.prepare_quote(tenant_,branch_,customer_a,held.sale_id,0,gen_random_uuid(),
            decode(repeat('08',32),'hex'),decode(repeat('06',32),'hex'),profile_,signer_,
            decode(repeat('2a',32),'hex'),decode(repeat('2b',32),'hex'),future_nonce);
        RAISE EXCEPTION 'legacy claimant acquired another signing lease';
    EXCEPTION WHEN SQLSTATE 'P0703' THEN NULL;
    END;
    RESET ROLE;
    IF (SELECT count(*) FROM ledger.checkout_claim WHERE sale_id=held.sale_id)<>2
       OR (SELECT count(*) FROM ledger.checkout_claim WHERE sale_id=issued.sale_id)<>2
       OR (SELECT state FROM ledger.sale WHERE id=held.sale_id)<>'preview'
       OR EXISTS(SELECT FROM ledger.quote_outbox WHERE sale_id=held.sale_id)
       OR EXISTS(SELECT FROM ledger.quote_work WHERE sale_id=held.sale_id AND (signed_digest IS NOT NULL OR fence<>q.fence)) THEN
        RAISE EXCEPTION 'upgrade changed a legacy draft or its claims';
    END IF;
END $$;

-- Wallets and SettlementV2 reject a quote whose issue time is still in the future
-- (now < issuedAt). Signing the issue time two seconds before the database clock
-- lets a phone clock running slightly behind this server accept a fresh quote. The
-- lease and expiry are unchanged, so the quote does not stay valid any longer.
-- 076 renamed 035's prepare_quote to prepare_quote_internal; this keeps 035's body
-- and changes only the stored and signed issued_at.
CREATE OR REPLACE FUNCTION ledger.prepare_quote_internal(tenant uuid, branch uuid, actor uuid, sale_ uuid, expected bigint,
    request_key_ uuid, request_intent_ bytea,
    claim_ bytea, profile_ bytea, signer_ bytea, quote_id_ bytea, order_id_ bytea, nonce_ bytea)
RETURNS TABLE(quote_id bytea, order_id bytea, nonce bytea, payer bytea, payer_generation bigint, payer_role smallint,
    issued_at timestamptz,expires_at timestamptz,fence bigint,payload_cipher bytea,signed_digest bytea,intent_digest bytea,amount text,asset text,issued_signer bytea)
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
DECLARE
    g ledger.commerce_grant := ledger.commerce_allowed(tenant,branch,actor);
    s ledger.sale;
    c ledger.order_checkout;
    cl ledger.checkout_claim;
    q ledger.quote_work;
    p ledger.issuance_profile;
    receipt ledger.command_receipt;
    clock timestamptz;
    lease timestamptz;
BEGIN
    IF request_key_ IS NULL OR request_intent_ IS NULL OR octet_length(request_intent_)<>32 THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid quote intent';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||actor::text||'issue_quote'||request_key_::text,0));
    SELECT * INTO receipt FROM ledger.command_receipt r WHERE r.branch_id=branch AND r.scope_ref=actor
      AND r.operation='issue_quote' AND r.request_key=request_key_;
    IF FOUND AND (receipt.intent_digest<>request_intent_ OR receipt.result_ref<>sale_) THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='request conflict';
    END IF;
    SELECT * INTO s FROM ledger.sale WHERE id=sale_ AND branch_id=branch FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied'; END IF;
    clock := date_trunc('second',clock_timestamp());
    SELECT * INTO c FROM ledger.order_checkout WHERE sale_id=sale_;
    SELECT claim_row.* INTO cl FROM ledger.checkout_claim claim_row
      JOIN ledger.payer_enrollment e USING (branch_id,actor_id)
      JOIN ledger.commerce_grant customer_scope USING (branch_id,actor_id)
      WHERE claim_row.sale_id=sale_ AND claim_row.claim_digest=claim_ AND claim_row.expires_at>clock
        AND e.expires_at>clock AND customer_scope.valid_until>clock AND customer_scope.role_code='customer'
        AND (claim_row.payer,claim_row.payer_generation,claim_row.payer_role)=(e.payer,e.payer_generation,e.payer_role)
        AND (claim_row.actor_id=actor OR (g.role_code='terminal' AND c.creator=actor))
        FOR SHARE OF claim_row,e,customer_scope;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied'; END IF;
    SELECT * INTO q FROM ledger.quote_work WHERE sale_id=sale_ FOR UPDATE;
    IF FOUND AND (q.actor_id<>cl.actor_id OR q.claim_digest<>claim_) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied';
    END IF;
    IF q.sale_id IS NOT NULL AND (q.payer<>cl.payer OR q.payer_generation<>cl.payer_generation OR q.payer_role<>cl.payer_role) THEN
        RAISE EXCEPTION USING ERRCODE='P0702', MESSAGE='payer authority changed';
    END IF;
    IF q.signed_digest IS NOT NULL THEN
        IF q.consumed_at IS NOT NULL OR q.expires_at<=clock OR s.state<>'issued' OR
           expected NOT IN (q.expected_revision,s.revision) THEN
            RAISE EXCEPTION USING ERRCODE='P0702', MESSAGE='quote no longer available';
        END IF;
        IF receipt.request_key IS NULL THEN
            INSERT INTO ledger.command_receipt VALUES
              (branch,actor,'issue_quote',request_key_,request_intent_,sale_,clock,q.expires_at);
        END IF;
        RETURN QUERY SELECT q.quote_id,q.order_id,q.nonce,q.payer,q.payer_generation,q.payer_role,q.issued_at,q.expires_at,
            q.fence,q.payload_cipher,q.signed_digest,c.intent_digest,s.amount::text,s.asset_ref,q.merchant_signer;
        RETURN;
    END IF;
    PERFORM pg_advisory_xact_lock_shared(hashtextextended(branch::text||'operator-catalog',0));
    IF c.menu_version IS NOT NULL AND EXISTS (SELECT 1 FROM ledger.catalog_pricing cp WHERE cp.branch_id=branch) AND
       ( c.menu_version IS DISTINCT FROM (SELECT max(m.version) FROM ledger.menu_release m WHERE m.branch_id=branch)
         OR NOT ledger.catalog_is_open(branch,c.menu_version,clock_timestamp()) )
    THEN
        RAISE EXCEPTION USING ERRCODE='P0702', MESSAGE='stale or closed menu';
    END IF;
    SELECT * INTO p FROM ledger.issuance_profile WHERE branch_id=branch AND enabled
      AND authorized_digest=profile_ FOR SHARE;
    IF NOT FOUND OR c.sale_id IS NULL OR s.state<>'preview' OR s.revision<>expected
       OR c.expires_at<=clock OR (q.sale_id IS NOT NULL AND (p.version<>q.profile_version OR q.merchant_signer<>signer_ OR q.expires_at<=clock)) THEN
        RAISE EXCEPTION USING ERRCODE='P0702', MESSAGE='order no longer issuable';
    END IF;
    IF q.sale_id IS NOT NULL THEN
        IF q.lease_until>clock THEN RAISE EXCEPTION USING ERRCODE='P0703', MESSAGE='quote signing in progress'; END IF;
        UPDATE ledger.quote_work w SET fence=w.fence+1,lease_until=LEAST(clock+interval '5 seconds',w.expires_at)
          WHERE w.sale_id=sale_ RETURNING * INTO q;
    ELSE
        lease := LEAST(clock + make_interval(secs=>p.quote_lifetime_seconds), c.expires_at, cl.expires_at, g.valid_until);
        IF lease<=clock THEN RAISE EXCEPTION USING ERRCODE='P0702', MESSAGE='quote expired'; END IF;
        INSERT INTO ledger.quote_work
          (sale_id,actor_id,claim_digest,quote_id,order_id,nonce,expected_revision,profile_version,merchant_signer,payer,payer_generation,payer_role,issued_at,expires_at,fence,lease_until)
          VALUES (sale_,cl.actor_id,claim_,quote_id_,order_id_,nonce_,expected,p.version,signer_,cl.payer,cl.payer_generation,cl.payer_role,
            clock-interval '2 seconds',lease,1,LEAST(clock+interval '5 seconds',lease)) RETURNING * INTO q;
    END IF;
    IF receipt.request_key IS NULL THEN
        INSERT INTO ledger.command_receipt VALUES
          (branch,actor,'issue_quote',request_key_,request_intent_,sale_,clock,q.expires_at);
    END IF;
    RETURN QUERY SELECT q.quote_id,q.order_id,q.nonce,q.payer,q.payer_generation,q.payer_role,q.issued_at,q.expires_at,
        q.fence,q.payload_cipher,q.signed_digest,c.intent_digest,s.amount::text,s.asset_ref,q.merchant_signer;
END $$;

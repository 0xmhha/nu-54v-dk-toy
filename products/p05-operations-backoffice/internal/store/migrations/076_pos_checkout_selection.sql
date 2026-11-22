-- The registered POS handoff requires the original terminal and an exact
-- active candidate. Keep existing issued receipts and non-POS authorization.
ALTER FUNCTION ledger.authorize_checkout(uuid,uuid,uuid,uuid,uuid,uuid,bytea,bytea)
    RENAME TO authorize_checkout_internal;
REVOKE EXECUTE ON FUNCTION ledger.authorize_checkout_internal(uuid,uuid,uuid,uuid,uuid,uuid,bytea,bytea)
    FROM nu54_p05_commerce;

CREATE FUNCTION ledger.authorize_checkout(tenant uuid, branch uuid, actor uuid, sale_ uuid, customer uuid,
    request_key_ uuid, intent_ bytea, candidate bytea)
RETURNS TABLE(claim_digest bytea, expires_at timestamptz)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF EXISTS (SELECT FROM ledger.pos_order p WHERE p.branch_id=branch AND p.sale_id=sale_)
       AND NOT EXISTS (SELECT FROM ledger.command_receipt r WHERE r.branch_id=branch AND r.scope_ref=actor
           AND r.operation='authorize_checkout' AND r.request_key=request_key_ AND r.result_ref=sale_) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='registered POS requires an approved candidate';
    END IF;
    RETURN QUERY SELECT c.claim_digest,c.expires_at FROM ledger.authorize_checkout_internal(
        tenant,branch,actor,sale_,customer,request_key_,intent_,candidate) c;
END $$;
REVOKE ALL ON FUNCTION ledger.authorize_checkout(uuid,uuid,uuid,uuid,uuid,uuid,bytea,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.authorize_checkout(uuid,uuid,uuid,uuid,uuid,uuid,bytea,bytea) TO nu54_p05_commerce;

CREATE OR REPLACE FUNCTION ledger.confirm_checkout_pair(tenant uuid, branch uuid, actor uuid, sale_ uuid, customer uuid,
    code_ text, key_ uuid, intent bytea, random_claim bytea)
RETURNS TABLE(claim_digest bytea, expires_at timestamptz)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE grant_ ledger.commerce_grant := ledger.commerce_allowed(tenant,branch,actor);
        pos_ ledger.pos_order;
BEGIN
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||sale_::text||'checkout-pair',0));
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||actor::text||'authorize_checkout'||key_::text,0));
    SELECT * INTO pos_ FROM ledger.pos_order p WHERE p.branch_id=branch AND p.sale_id=sale_ FOR SHARE;
    IF grant_.role_code<>'terminal' OR (pos_.sale_id IS NOT NULL AND
       (pos_.terminal_id<>actor OR grant_.valid_until<=clock_timestamp()))
       OR NOT EXISTS(SELECT FROM ledger.checkout_pair p WHERE p.branch_id=branch AND p.sale_id=sale_
           AND p.customer_ref=customer AND p.comparison_code=code_ AND p.expires_at>clock_timestamp() FOR SHARE) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='checkout scope denied';
    END IF;
    PERFORM FROM ledger.sale s WHERE s.id=sale_ AND s.branch_id=branch FOR UPDATE;
    IF EXISTS(SELECT FROM ledger.checkout_claim c WHERE c.branch_id=branch AND c.sale_id=sale_ AND c.actor_id<>customer) THEN
        IF pos_.sale_id IS NOT NULL THEN
            RAISE EXCEPTION USING ERRCODE='P0761', MESSAGE='checkout already paired';
        END IF;
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='checkout already paired';
    END IF;
    RETURN QUERY SELECT c.claim_digest,c.expires_at FROM ledger.authorize_checkout_internal(
        tenant,branch,actor,sale_,customer,key_,intent,random_claim) c;
    -- Existing POS candidates are already short lived, limited to eight and
    -- cleaned by the same expiry sweep. An exact eligible losing request
    -- can receive conflict until then without disclosing the winner.
    IF pos_.sale_id IS NULL THEN
        DELETE FROM ledger.checkout_pair p WHERE p.branch_id=branch AND p.sale_id=sale_ AND p.customer_ref<>customer;
    END IF;
END $$;

CREATE FUNCTION ledger.pos_selected_marker(tenant_ uuid,branch_ uuid,actor_ uuid,handoff_ uuid)
RETURNS text LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT CASE WHEN count(*)=1 THEN min(p.comparison_code) END
    FROM ledger.lookup_pos_order(tenant_,branch_,actor_,handoff_) o
    JOIN ledger.sale s ON s.branch_id=branch_ AND s.id=o.sale_ref AND s.state IN ('preview','issued')
    JOIN ledger.checkout_claim c ON c.branch_id=branch_ AND c.sale_id=o.sale_ref AND c.expires_at>clock_timestamp()
    LEFT JOIN ledger.checkout_pair p ON p.branch_id=branch_ AND p.sale_id=o.sale_ref
      AND p.customer_ref=c.actor_id AND p.expires_at>clock_timestamp()
    WHERE o.payer_bound
$$;
REVOKE ALL ON FUNCTION ledger.pos_selected_marker(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.pos_selected_marker(uuid,uuid,uuid,uuid) TO nu54_p05_commerce;

CREATE OR REPLACE FUNCTION ledger.request_checkout_pair(tenant uuid, branch uuid, actor uuid, sale_ uuid, key_ uuid, code_ text)
RETURNS TABLE(code text, expiry timestamptz) LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    grant_ ledger.commerce_grant := ledger.commerce_allowed(tenant,branch,actor);
    prior ledger.checkout_pair;
    order_expiry timestamptz;
BEGIN
    IF grant_.role_code<>'customer' OR code_ !~ '^[0-9a-f]{8}$' THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='checkout scope denied';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||sale_::text||'checkout-pair',0));
    SELECT c.expires_at INTO order_expiry FROM ledger.order_checkout c
      JOIN ledger.sale s ON s.id=c.sale_id AND s.branch_id=c.branch_id
      JOIN ledger.commerce_grant creator ON creator.branch_id=c.branch_id AND creator.actor_id=c.creator
      WHERE c.branch_id=branch AND c.sale_id=sale_ AND c.expires_at>statement_timestamp()
        AND s.state IN ('preview','issued') AND creator.role_code='terminal' AND creator.valid_until>statement_timestamp()
      FOR SHARE OF c,s,creator;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='checkout scope denied'; END IF;
    DELETE FROM ledger.checkout_pair WHERE branch_id=branch AND sale_id=sale_ AND expires_at<=statement_timestamp();
    SELECT * INTO prior FROM ledger.checkout_pair p WHERE p.branch_id=branch AND p.sale_id=sale_ AND p.customer_ref=actor;
    IF FOUND THEN
        RETURN QUERY SELECT prior.comparison_code,prior.expires_at;
        RETURN;
    END IF;
    IF EXISTS(SELECT FROM ledger.pos_order p JOIN ledger.checkout_claim c
        ON c.branch_id=p.branch_id AND c.sale_id=p.sale_id
        WHERE p.branch_id=branch AND p.sale_id=sale_) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='checkout scope denied';
    END IF;
    IF (SELECT count(*) FROM ledger.checkout_pair p WHERE p.branch_id=branch AND p.sale_id=sale_)>=8 THEN
        RAISE EXCEPTION USING ERRCODE='P0703', MESSAGE='checkout comparison busy';
    END IF;
    order_expiry := LEAST(order_expiry,grant_.valid_until,statement_timestamp()+interval '35 seconds');
    INSERT INTO ledger.checkout_pair VALUES(branch,sale_,actor,key_,code_,order_expiry);
    RETURN QUERY SELECT code_,order_expiry;
END $$;

CREATE OR REPLACE FUNCTION ledger.pending_checkout_pairs(tenant uuid, branch uuid, actor uuid, sale_ uuid)
RETURNS TABLE(customer_ref uuid, code text, expiry timestamptz) LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE grant_ ledger.commerce_grant := ledger.commerce_allowed(tenant,branch,actor);
BEGIN
    IF grant_.role_code<>'terminal' OR NOT EXISTS(SELECT FROM ledger.order_checkout c
        WHERE c.branch_id=branch AND c.creator=actor AND c.sale_id=sale_ AND c.expires_at>statement_timestamp()) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='checkout scope denied';
    END IF;
    IF EXISTS(SELECT FROM ledger.pos_order p JOIN ledger.checkout_claim c
        ON c.branch_id=p.branch_id AND c.sale_id=p.sale_id
        WHERE p.branch_id=branch AND p.sale_id=sale_) THEN
        RETURN;
    END IF;
    RETURN QUERY SELECT p.customer_ref,p.comparison_code,p.expires_at FROM ledger.checkout_pair p
      WHERE p.branch_id=branch AND p.sale_id=sale_ AND p.expires_at>statement_timestamp()
      ORDER BY p.expires_at,p.customer_ref LIMIT 8;
END $$;

-- Old direct authorization could accumulate drafts for one registered sale.
-- Leave every prior row untouched; only an already committed signature can
-- resolve ambiguity without an operator. The original issuer still checks
-- its caller, claim, ciphertext and expiry on every replay.
CREATE FUNCTION ledger.pos_checkout_holds(branch_ uuid,sale_ uuid)
RETURNS boolean LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT EXISTS(SELECT FROM ledger.pos_order p WHERE p.branch_id=branch_ AND p.sale_id=sale_)
        AND (SELECT count(*) FROM ledger.checkout_claim c WHERE c.branch_id=branch_ AND c.sale_id=sale_)>1
        AND NOT EXISTS(SELECT FROM ledger.quote_work q WHERE q.sale_id=sale_ AND q.signed_digest IS NOT NULL)
$$;
REVOKE ALL ON FUNCTION ledger.pos_checkout_holds(uuid,uuid) FROM PUBLIC;

-- Enrolled terminals relay the authenticated nearby principal, never the
-- display mark. QR and BLE enroll in the same per-customer candidate row.
CREATE FUNCTION ledger.pos_nearby_checkout(tenant_ uuid,branch_ uuid,terminal_ uuid,sale_ uuid,
    customer_ uuid,key_ uuid,code_ text,register_ boolean)
RETURNS TABLE(code text, claim_digest bytea, expiry timestamptz)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE terminal_grant ledger.commerce_grant;
DECLARE customer_grant ledger.commerce_grant;
BEGIN
    terminal_grant := ledger.commerce_allowed(tenant_,branch_,terminal_);
    customer_grant := ledger.commerce_allowed(tenant_,branch_,customer_);
    PERFORM pg_advisory_xact_lock(hashtextextended(branch_::text||sale_::text||'checkout-pair',0));
    IF terminal_grant.role_code<>'terminal' OR customer_grant.role_code<>'customer'
       OR LEAST(terminal_grant.valid_until,customer_grant.valid_until)<=clock_timestamp()
       OR NOT EXISTS(SELECT FROM ledger.pos_order p WHERE p.branch_id=branch_ AND p.sale_id=sale_
           AND p.terminal_id=terminal_ FOR SHARE) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='checkout scope denied';
    END IF;
    RETURN QUERY SELECT NULL::text,c.claim_digest,c.expires_at
        FROM ledger.checkout_claim c JOIN ledger.payer_enrollment e USING (branch_id,actor_id)
        WHERE c.branch_id=branch_ AND c.sale_id=sale_ AND c.actor_id=customer_
            AND LEAST(c.expires_at,e.expires_at)>clock_timestamp()
            AND (c.payer,c.payer_generation,c.payer_role)=(e.payer,e.payer_generation,e.payer_role)
            AND NOT ledger.pos_checkout_holds(branch_,sale_)
            AND NOT EXISTS(SELECT FROM ledger.quote_work q WHERE q.sale_id=sale_
              AND q.signed_digest IS NOT NULL AND q.claim_digest<>c.claim_digest)
        FOR SHARE OF c,e;
    IF FOUND THEN RETURN; END IF;
    IF register_ AND NOT EXISTS(
        SELECT FROM ledger.checkout_claim c WHERE c.branch_id=branch_ AND c.sale_id=sale_
    ) THEN
        RETURN QUERY SELECT p.code,NULL::bytea,p.expiry
            FROM ledger.request_checkout_pair(tenant_,branch_,customer_,sale_,key_,code_) p;
    ELSE
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='checkout scope denied';
    END IF;
END $$;
REVOKE ALL ON FUNCTION ledger.pos_nearby_checkout(uuid,uuid,uuid,uuid,uuid,uuid,text,boolean) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.pos_nearby_checkout(uuid,uuid,uuid,uuid,uuid,uuid,text,boolean) TO nu54_p05_commerce;

ALTER FUNCTION ledger.prepare_quote(uuid,uuid,uuid,uuid,bigint,uuid,bytea,bytea,bytea,bytea,bytea,bytea,bytea)
    RENAME TO prepare_quote_internal;
REVOKE EXECUTE ON FUNCTION ledger.prepare_quote_internal(uuid,uuid,uuid,uuid,bigint,uuid,bytea,bytea,bytea,bytea,bytea,bytea,bytea)
    FROM nu54_p05_commerce;
CREATE FUNCTION ledger.prepare_quote(tenant uuid,branch uuid,actor uuid,sale_ uuid,expected bigint,
    request_key_ uuid,request_intent_ bytea,claim_ bytea,profile_ bytea,signer_ bytea,quote_id_ bytea,order_id_ bytea,nonce_ bytea)
RETURNS TABLE(quote_id bytea,order_id bytea,nonce bytea,payer bytea,payer_generation bigint,payer_role smallint,
    issued_at timestamptz,expires_at timestamptz,fence bigint,payload_cipher bytea,signed_digest bytea,intent_digest bytea,
    amount text,asset text,issued_signer bytea)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    -- Let the normal issuer establish exact caller scope before the held
    -- draft check; any reserved work rolls back in the same transaction.
    RETURN QUERY SELECT * FROM ledger.prepare_quote_internal(tenant,branch,actor,sale_,expected,
        request_key_,request_intent_,claim_,profile_,signer_,quote_id_,order_id_,nonce_);
    IF ledger.pos_checkout_holds(branch,sale_) THEN
        RAISE EXCEPTION USING ERRCODE='P0703', MESSAGE='legacy POS selection requires operator review';
    END IF;
END $$;
REVOKE ALL ON FUNCTION ledger.prepare_quote(uuid,uuid,uuid,uuid,bigint,uuid,bytea,bytea,bytea,bytea,bytea,bytea,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.prepare_quote(uuid,uuid,uuid,uuid,bigint,uuid,bytea,bytea,bytea,bytea,bytea,bytea,bytea)
    TO nu54_p05_commerce;

ALTER FUNCTION ledger.finish_quote(uuid,uuid,uuid,uuid,bytea,bytea,bigint,bytea,bytea)
    RENAME TO finish_quote_internal;
REVOKE EXECUTE ON FUNCTION ledger.finish_quote_internal(uuid,uuid,uuid,uuid,bytea,bytea,bigint,bytea,bytea)
    FROM nu54_p05_commerce;
CREATE FUNCTION ledger.finish_quote(tenant uuid,branch uuid,actor uuid,sale_ uuid,claim_ bytea,
    profile_ bytea,fence_ bigint,ciphertext bytea,digest_ bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE committed boolean;
        held boolean;
BEGIN
    held := ledger.pos_checkout_holds(branch,sale_);
    committed := ledger.finish_quote_internal(tenant,branch,actor,sale_,claim_,profile_,fence_,ciphertext,digest_);
    IF held THEN
        RAISE EXCEPTION USING ERRCODE='P0703', MESSAGE='legacy POS selection requires operator review';
    END IF;
    RETURN committed;
END $$;
REVOKE ALL ON FUNCTION ledger.finish_quote(uuid,uuid,uuid,uuid,bytea,bytea,bigint,bytea,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.finish_quote(uuid,uuid,uuid,uuid,bytea,bytea,bigint,bytea,bytea)
    TO nu54_p05_commerce;

CREATE FUNCTION ledger.pos_selection_held(tenant_ uuid,branch_ uuid,actor_ uuid,handoff_ uuid)
RETURNS boolean LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT coalesce((SELECT ledger.pos_checkout_holds(branch_,o.sale_ref)
        FROM ledger.lookup_pos_order(tenant_,branch_,actor_,handoff_) o),false)
$$;
REVOKE ALL ON FUNCTION ledger.pos_selection_held(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.pos_selection_held(uuid,uuid,uuid,uuid) TO nu54_p05_commerce;

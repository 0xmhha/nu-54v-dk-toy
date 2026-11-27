-- A live signed quote can acquire a gas commitment once. Store only the
-- authenticated grant digest on the existing quote lifetime.
ALTER TABLE ledger.quote_work ADD COLUMN gas_sponsor_digest bytea
    CHECK (gas_sponsor_digest IS NULL OR octet_length(gas_sponsor_digest)=32);

CREATE FUNCTION ledger.pin_merchant_gas_sponsor(tenant_ uuid, branch_ uuid, actor_ uuid,
    sale_ uuid, claim_ bytea, quote_ bytea, grant_ bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE work ledger.quote_work;
BEGIN
    IF grant_ IS NULL OR octet_length(grant_)<>32 OR NOT EXISTS (
        SELECT FROM ledger.private_quote_context(tenant_,branch_,actor_,sale_,claim_)
    ) THEN RETURN false; END IF;
    -- Match the existing sale-first lock ordering used by issuance and forwarding.
    PERFORM FROM ledger.sale WHERE id=sale_ AND branch_id=branch_ AND state='issued' FOR UPDATE;
    IF NOT FOUND THEN RETURN false; END IF;
    SELECT * INTO work FROM ledger.quote_work WHERE sale_id=sale_ FOR UPDATE;
    IF NOT FOUND OR work.signed_digest IS DISTINCT FROM quote_ OR work.claim_digest IS DISTINCT FROM claim_
       OR work.consumed_at IS NOT NULL OR work.expires_at<=clock_timestamp()
       OR NOT EXISTS (SELECT FROM ledger.private_quote_context(tenant_,branch_,actor_,sale_,claim_))
    THEN RETURN false; END IF;
    IF work.gas_sponsor_digest IS NULL THEN
        UPDATE ledger.quote_work SET gas_sponsor_digest=grant_ WHERE sale_id=sale_;
        RETURN true;
    END IF;
    RETURN work.gas_sponsor_digest=grant_;
END $$;
REVOKE ALL ON FUNCTION ledger.pin_merchant_gas_sponsor(uuid,uuid,uuid,uuid,bytea,bytea,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.pin_merchant_gas_sponsor(uuid,uuid,uuid,uuid,bytea,bytea,bytea) TO nu54_p05_commerce;

ALTER FUNCTION ledger.store_forward(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea)
    RENAME TO store_forward_internal;
REVOKE ALL ON FUNCTION ledger.store_forward_internal(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea)
    FROM PUBLIC, nu54_p05_commerce;

CREATE FUNCTION ledger.store_terminal_forward(tenant_ uuid, branch_ uuid, actor_ uuid, sale_ uuid,
    key_ uuid, intent_ bytea, quote_ bytea, approval_ bytea, stream_ uuid, encrypted_ bytea,
    sponsorship_required_ boolean, current_grant_ bytea, sponsorship_until_ timestamptz)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE pinned bytea;
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code IS DISTINCT FROM 'terminal'
       OR NOT EXISTS (SELECT FROM ledger.forward_quote(tenant_,branch_,actor_,sale_))
    THEN RAISE EXCEPTION 'payment forwarding scope denied' USING ERRCODE='42501'; END IF;
    PERFORM FROM ledger.sale WHERE id=sale_ AND branch_id=branch_ FOR UPDATE;
    SELECT gas_sponsor_digest INTO STRICT pinned FROM ledger.quote_work WHERE sale_id=sale_;
    IF coalesce(sponsorship_required_,true) OR pinned IS NOT NULL THEN
        IF EXISTS (SELECT FROM ledger.settlement_submission WHERE sale_id=sale_) THEN
            -- Only an exact accepted receipt may recover after revocation. The
            -- original implementation rechecks its quote, approval and stream.
            IF NOT EXISTS (SELECT FROM ledger.command_receipt r
                WHERE r.branch_id=branch_ AND r.scope_ref=actor_ AND r.operation='payment_forward'
                  AND r.request_key=key_ AND r.result_ref=sale_ AND r.intent_digest=intent_)
            THEN RAISE EXCEPTION 'accepted sponsored forward requires the original request' USING ERRCODE='P1001'; END IF;
        ELSIF pinned IS NULL OR pinned IS DISTINCT FROM current_grant_ OR sponsorship_until_ IS NULL
              OR sponsorship_until_<=clock_timestamp() THEN
            RAISE EXCEPTION 'merchant gas sponsorship unavailable' USING ERRCODE='P1050';
        END IF;
    END IF;
    RETURN ledger.store_forward_internal(tenant_,branch_,actor_,sale_,key_,intent_,quote_,approval_,stream_,encrypted_);
END $$;
REVOKE ALL ON FUNCTION ledger.store_terminal_forward(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea,boolean,bytea,timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.store_terminal_forward(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea,boolean,bytea,timestamptz) TO nu54_p05_commerce;


-- Keep the previous API available to customer and legacy callers. A terminal
-- that uses it without a live grant cannot bypass a pin made by upgraded opsd.
CREATE FUNCTION ledger.store_forward(tenant_ uuid, branch_ uuid, actor_ uuid, sale_ uuid,
    key_ uuid, intent_ bytea, quote_ bytea, approval_ bytea, stream_ uuid, encrypted_ bytea)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code='terminal' THEN
        RETURN ledger.store_terminal_forward(tenant_,branch_,actor_,sale_,key_,intent_,quote_,approval_,
            stream_,encrypted_,false,NULL,NULL);
    END IF;
    RETURN ledger.store_forward_internal(tenant_,branch_,actor_,sale_,key_,intent_,quote_,approval_,stream_,encrypted_);
END $$;
REVOKE ALL ON FUNCTION ledger.store_forward(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.store_forward(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea) TO nu54_p05_commerce;

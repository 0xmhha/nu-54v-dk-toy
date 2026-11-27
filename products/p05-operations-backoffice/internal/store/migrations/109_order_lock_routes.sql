-- One order lock may arrive over the kiosk and over the customer's phone. Both callers are authorized for
-- the order, so the same approval gets the same result; a different one still conflicts. A SettlementV2
-- approval keeps its earlier rule.
CREATE FUNCTION ledger.same_order_lock(sale_ uuid,approval_ bytea) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT EXISTS (SELECT FROM ledger.settlement_submission p
        WHERE p.sale_id=sale_ AND p.approval_digest=approval_ AND octet_length(p.approval_cipher)=113)
$$;
REVOKE ALL ON FUNCTION ledger.same_order_lock(uuid,bytea) FROM PUBLIC;

-- Unchanged except for that exception.
CREATE OR REPLACE FUNCTION ledger.store_customer_forward_unbound(tenant_ uuid, branch_ uuid, actor_ uuid, sale_ uuid,
    key_ uuid, intent_ bytea, quote_ bytea, approval_ bytea, stream_ uuid, encrypted_ bytea,
    current_grant_ bytea, sponsorship_until_ timestamptz)
RETURNS TABLE(sale_ref uuid, new_sponsored_admission boolean)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $customer$
DECLARE pinned bytea;
DECLARE protected_admission boolean := false;
BEGIN
    PERFORM FROM ledger.sale WHERE id=sale_ AND branch_id=branch_ FOR UPDATE;
    IF NOT FOUND OR NOT ledger.request_authority_live(tenant_,branch_,actor_,'customer',ARRAY[]::text[])
       OR NOT EXISTS (SELECT FROM ledger.quote_work WHERE sale_id=sale_ AND actor_id=actor_)
    THEN RAISE EXCEPTION 'customer payment scope denied' USING ERRCODE='42501'; END IF;
    SELECT gas_sponsor_digest INTO STRICT pinned FROM ledger.quote_work WHERE sale_id=sale_;
    IF pinned IS NOT NULL AND ledger.is_cafe_checkout(branch_,sale_) THEN
        IF EXISTS (SELECT FROM ledger.settlement_submission WHERE sale_id=sale_) THEN
            IF NOT ledger.same_order_lock(sale_,approval_) AND NOT EXISTS (SELECT FROM ledger.command_receipt r
                WHERE r.branch_id=branch_ AND r.scope_ref=actor_ AND r.operation='payment_forward'
                  AND r.request_key=key_ AND r.result_ref=sale_ AND r.intent_digest=intent_)
            THEN RAISE EXCEPTION 'accepted sponsored forward requires the original request' USING ERRCODE='P1001'; END IF;
        ELSE
            IF pinned IS DISTINCT FROM current_grant_ OR sponsorship_until_ IS NULL
                 OR sponsorship_until_<=clock_timestamp() THEN
                RAISE EXCEPTION 'merchant gas sponsorship unavailable' USING ERRCODE='P1050';
            END IF;
            protected_admission := true;
        END IF;
    END IF;
    RETURN QUERY SELECT ledger.store_forward_internal(tenant_,branch_,actor_,sale_,key_,intent_,quote_,approval_,stream_,encrypted_),
        protected_admission;
END
$customer$;

CREATE OR REPLACE FUNCTION ledger.store_terminal_forward_unbound(tenant_ uuid, branch_ uuid, actor_ uuid, sale_ uuid,
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
            IF NOT ledger.same_order_lock(sale_,approval_) AND NOT EXISTS (SELECT FROM ledger.command_receipt r
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

-- Classify the original commercial café sale using existing owner records.
-- This helper is private to the two scoped routines below.
CREATE FUNCTION ledger.is_cafe_checkout(branch_ uuid, sale_ uuid)
RETURNS boolean LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $cafe$
    SELECT EXISTS (SELECT FROM ledger.sale s WHERE s.branch_id=branch_ AND s.id=sale_
        AND ledger.kiosk_menu_pickup(s.id)
        AND NOT EXISTS (SELECT FROM ledger.travel_pickup_order p WHERE p.sale_id=s.id)
        AND NOT EXISTS (SELECT FROM ledger.travel_booking_leg leg WHERE leg.sale_id=s.id)
        AND NOT EXISTS (SELECT FROM ledger.invoice i WHERE i.sale_id=s.id))
$cafe$;
REVOKE ALL ON FUNCTION ledger.is_cafe_checkout(uuid,uuid) FROM PUBLIC;

-- Customer status can show the café distinction only for its authorized original.
-- The terminal result and its existing permissions retain the previous definition.
CREATE OR REPLACE FUNCTION ledger.menu_checkout_context(tenant uuid, branch uuid, actor uuid, sale_ uuid)
RETURNS jsonb LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT jsonb_strip_nulls(jsonb_build_object(
        'branchLabel', COALESCE(label.label, '지점 · ' || right(branch::text,8)),
        'payerBound', claims.total > 0,
        'customerRef', CASE WHEN claims.total=1 THEN claims.customer END,
        'finalOutcome', CASE
            WHEN s.state='paid' AND EXISTS (
                SELECT FROM ledger.payment p
                JOIN ledger.journal j ON j.sale_id=p.sale_id AND j.branch_id=s.branch_id
                    AND j.refund_id IS NULL AND j.evidence_id=p.evidence_id
                JOIN ledger.pickup pickup ON pickup.sale_id=p.sale_id AND pickup.branch_id=s.branch_id
                JOIN ledger.settlement_work work ON work.sale_id=p.sale_id AND work.refund_id IS NULL
                JOIN ledger.relay_attempt attempt ON attempt.work_id=work.id AND attempt.tx_hash=p.tx_hash
                WHERE p.sale_id=s.id AND p.payment_mode=1 AND p.principal=s.amount AND p.asset_ref=s.asset_ref
            ) THEN 'relay-paid'
            WHEN s.state='closed' AND EXISTS (
                SELECT FROM ledger.sale_resolution resolution
                JOIN ledger.quote_work quote ON quote.sale_id=resolution.sale_id AND quote.signed_digest IS NOT NULL
                WHERE resolution.sale_id=s.id
            ) THEN 'expired-unpaid'
        END
    ))
    FROM ledger.order_checkout c
    JOIN ledger.sale s ON s.branch_id=c.branch_id AND s.id=c.sale_id AND s.origin='menu'
    JOIN ledger.branch b ON b.id=c.branch_id AND b.tenant_ref=tenant
    JOIN ledger.commerce_grant g ON g.branch_id=c.branch_id AND g.actor_id=c.creator
        AND g.role_code='terminal' AND g.valid_until>statement_timestamp()
    LEFT JOIN ledger.operator_branch_label label ON label.branch_id=c.branch_id
    CROSS JOIN LATERAL (
        SELECT count(*) AS total, min(cl.actor_id::text) AS customer
        FROM ledger.checkout_claim cl
        WHERE cl.branch_id=c.branch_id AND cl.sale_id=c.sale_id
    ) claims
    WHERE c.branch_id=branch AND c.sale_id=sale_ AND c.creator=actor
    UNION ALL
    SELECT jsonb_build_object(
        'branchLabel',COALESCE(label.label,'지점 · ' || right(branch::text,8)),
        'payerBound',EXISTS(SELECT FROM ledger.checkout_claim claim
            WHERE claim.branch_id=branch AND claim.sale_id=sale_ AND claim.actor_id=actor
              AND claim.expires_at>clock_timestamp()),
        'reviewKind','cafe')
    FROM ledger.branch b
    LEFT JOIN ledger.operator_branch_label label ON label.branch_id=b.id
    WHERE b.id=branch AND b.tenant_ref=tenant
      AND (ledger.commerce_allowed(tenant,branch,actor)).role_code='customer'
      AND ledger.is_cafe_checkout(branch,sale_)
      AND EXISTS(SELECT FROM ledger.commerce_order_receipt(tenant,branch,actor,sale_));
$$;

-- Customer C3 admission preserves the first grant commitment and the existing
-- settlement scope. Exact accepted requests can be recovered after grant removal.
CREATE FUNCTION ledger.store_customer_forward(tenant_ uuid, branch_ uuid, actor_ uuid, sale_ uuid,
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
            IF NOT EXISTS (SELECT FROM ledger.command_receipt r
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
REVOKE ALL ON FUNCTION ledger.store_customer_forward(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea,bytea,timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.store_customer_forward(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea,bytea,timestamptz) TO nu54_p05_commerce;

-- Old servers and alternate service entry points cannot submit a new pinned
-- café approval without a current grant. Other direct callers keep their scope.
CREATE OR REPLACE FUNCTION ledger.store_forward(tenant_ uuid, branch_ uuid, actor_ uuid, sale_ uuid,
    key_ uuid, intent_ bytea, quote_ bytea, approval_ bytea, stream_ uuid, encrypted_ bytea)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $forward$
DECLARE role_ text := (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code;
BEGIN
    IF role_='terminal' THEN
        RETURN ledger.store_terminal_forward(tenant_,branch_,actor_,sale_,key_,intent_,quote_,approval_,
            stream_,encrypted_,false,NULL,NULL);
    END IF;
    IF role_='customer' THEN
        RETURN (SELECT x.sale_ref FROM ledger.store_customer_forward(tenant_,branch_,actor_,sale_,key_,intent_,quote_,approval_,
            stream_,encrypted_,NULL,NULL) x);
    END IF;
    RETURN ledger.store_forward_internal(tenant_,branch_,actor_,sale_,key_,intent_,quote_,approval_,stream_,encrypted_);
END
$forward$;

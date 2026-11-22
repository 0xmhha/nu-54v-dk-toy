CREATE INDEX customer_checkout_origin ON ledger.order_checkout(branch_id,creator,sale_id);
CREATE INDEX customer_checkout_claim ON ledger.checkout_claim(branch_id,actor_id,sale_id);
CREATE INDEX customer_checkout_quote ON ledger.quote_work(actor_id,sale_id) WHERE signed_digest IS NOT NULL;

CREATE FUNCTION ledger.customer_order_rows(tenant_ uuid,branch_ uuid,actor_ uuid,after_ uuid)
RETURNS TABLE(id uuid,state text,asset_ref text,amount text,access_ends_at timestamptz)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $rows$
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code <> 'customer' THEN
        RAISE EXCEPTION 'customer scope required' USING ERRCODE='42501';
    END IF;
    RETURN QUERY
    WITH candidates AS (
        SELECT c.sale_id FROM ledger.order_checkout c
        WHERE c.branch_id=branch_ AND c.creator=actor_
        UNION
        SELECT cl.sale_id FROM ledger.checkout_claim cl
        WHERE cl.branch_id=branch_ AND cl.actor_id=actor_ AND cl.expires_at>statement_timestamp()
        UNION
        SELECT q.sale_id FROM ledger.quote_work q
        JOIN ledger.sale s ON s.id=q.sale_id AND s.branch_id=branch_ AND s.state='paid'
        WHERE q.actor_id=actor_ AND q.signed_digest IS NOT NULL
    )
    SELECT v.id,v.state,v.asset_ref,v.amount::text,
        CASE WHEN oc.creator=actor_ OR (v.state='paid' AND v.receipt_ref IS NOT NULL AND q.actor_id=actor_)
             THEN NULL ELSE cl.expires_at END
    FROM candidates c
    JOIN ledger.sale s ON s.id=c.sale_id AND s.branch_id=branch_ AND s.origin IN ('menu','pos')
    JOIN ledger.order_checkout oc ON oc.sale_id=s.id AND oc.branch_id=branch_
    LEFT JOIN ledger.checkout_claim cl ON cl.sale_id=s.id AND cl.branch_id=branch_ AND cl.actor_id=actor_
    LEFT JOIN ledger.quote_work q ON q.sale_id=s.id AND q.signed_digest IS NOT NULL
    CROSS JOIN LATERAL ledger.commerce_order_view(tenant_,branch_,actor_,c.sale_id) v
    WHERE (after_=actor_ OR c.sale_id>after_)
      AND (v.state<>'paid' OR oc.creator=actor_ OR q.actor_id=actor_)
    ORDER BY c.sale_id LIMIT 50;
END
$rows$;
REVOKE ALL ON FUNCTION ledger.customer_order_rows(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.customer_order_rows(uuid,uuid,uuid,uuid) TO nu54_p05_commerce;

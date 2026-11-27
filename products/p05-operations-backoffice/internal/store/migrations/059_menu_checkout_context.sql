-- Only the creator of a terminal menu order may read its public checkout context.
CREATE FUNCTION ledger.menu_checkout_context(tenant uuid, branch uuid, actor uuid, sale_ uuid)
RETURNS jsonb LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT jsonb_strip_nulls(jsonb_build_object(
        'branchLabel', COALESCE(label.label, '지점 · ' || right(branch::text,8)),
        'payerBound', claims.total > 0,
        'customerRef', CASE WHEN claims.total=1 THEN claims.customer END
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
    WHERE c.branch_id=branch AND c.sale_id=sale_ AND c.creator=actor;
$$;

REVOKE ALL ON FUNCTION ledger.menu_checkout_context(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.menu_checkout_context(uuid,uuid,uuid,uuid) TO nu54_p05_commerce;

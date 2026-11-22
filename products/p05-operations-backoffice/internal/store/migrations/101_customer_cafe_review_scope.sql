-- Authorize direct menu context using the scoped order status and selected
-- payer. Receipt/status callers already materialize the distinct payment
-- projection and enforce its private access deadline at response release.
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
      AND EXISTS (
        SELECT FROM ledger.commerce_order_status_with_invoice(tenant,branch,actor,sale_) visible
        JOIN ledger.order_checkout checkout ON (checkout.branch_id,checkout.sale_id)=(branch,visible.id)
        WHERE visible.state<>'paid' OR checkout.creator=actor OR EXISTS (
            SELECT FROM ledger.quote_work payer WHERE payer.sale_id=visible.id
                AND payer.actor_id=actor AND payer.signed_digest IS NOT NULL
        )
      );
$$;


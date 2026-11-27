CREATE FUNCTION ledger.console_refund_queue(tenant uuid, branch uuid, actor uuid, after_ uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE capabilities text[];
DECLARE queue jsonb;
BEGIN
    SELECT array_agg(g.capability) INTO capabilities
      FROM ledger.finance_grant g JOIN ledger.branch b ON b.id=g.branch_id
      WHERE b.id=branch AND b.tenant_ref=tenant AND g.actor_id=actor AND g.valid_until>clock_timestamp()
        AND g.capability IN ('refund_request','refund_approve','refund_submit');
    IF capabilities IS NULL THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='refund scope denied';
    END IF;
    WITH page AS MATERIALIZED (
        SELECT r.id, s.id AS sale_id, r.state, r.amount, p.principal, s.asset_ref,
               r.requester, r.approver, r.reason_code, r.treasury_submitted_at,
               EXISTS(SELECT FROM ledger.settlement_work w WHERE w.refund_id=r.id AND NOT w.finished) AS queued
        FROM ledger.sale s JOIN ledger.payment p ON p.sale_id=s.id
        JOIN ledger.refund r ON r.sale_id=p.sale_id
        WHERE s.branch_id=branch AND (after_ IS NULL OR r.id>after_)
        ORDER BY r.id LIMIT 51
    ), numbered AS (
        SELECT page.*,row_number() OVER(ORDER BY id) AS position
        FROM page
    )
    SELECT jsonb_build_object(
        'items',coalesce(jsonb_agg(jsonb_build_object(
            'refundId',id,'saleId',sale_id,'state',state,'amount',amount::text,
            'principal',principal::text,'asset',asset_ref,'next',
            CASE
                WHEN state='requested' AND requester=actor THEN 'requester_cannot_decide'
                WHEN state='requested' AND 'refund_approve'=ANY(capabilities) THEN 'decide'
                WHEN state='requested' THEN 'approval_role_required'
                WHEN state='approved' AND (treasury_submitted_at IS NOT NULL OR queued) THEN 'await_chain'
                WHEN state='approved' AND reason_code IS NULL THEN 'await_prior'
                WHEN state='approved' AND (requester=actor OR approver=actor) THEN 'independent_treasury_required'
                WHEN state='approved' AND 'refund_submit'=ANY(capabilities) THEN 'review_funds'
                WHEN state='approved' THEN 'treasury_role_required'
                WHEN state='inflight' THEN 'await_chain'
                ELSE 'read_result'
            END) ORDER BY id) FILTER (WHERE position<=50),'[]'::jsonb),
        'nextCursor',CASE WHEN count(*)=51 THEN min(id::text) FILTER (WHERE position=50) END)
      INTO queue FROM numbered;
    RETURN queue;
END $$;
REVOKE ALL ON FUNCTION ledger.console_refund_queue(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.console_refund_queue(uuid,uuid,uuid,uuid) TO nu54_p05_finance;

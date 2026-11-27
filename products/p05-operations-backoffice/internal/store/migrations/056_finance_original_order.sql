CREATE FUNCTION ledger.finance_order_snapshot(tenant uuid,branch uuid,actor uuid,order_ uuid)
RETURNS TABLE (detail jsonb, chain_scope jsonb)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'report_read');
    RETURN QUERY SELECT jsonb_build_object(
        'saleId',s.id,'asset',s.asset_ref,'amount',s.amount::text,'state',s.state,'revision',s.revision,
        'quoteId','0x'||encode(q.quote_id,'hex'),
        -- The worker also marks an expired, never-broadcast relay as final.
        'paidAt',p.included_at,'transaction',COALESCE(p.tx_hash,CASE
            WHEN relay.state IN ('broadcast','observed','reverted') THEN relay.tx_hash END),
        'fundsStatus',CASE WHEN p.sale_id IS NOT NULL THEN 'confirmed'
            WHEN s.state='closed' THEN 'closed' ELSE 'held' END,
        'fulfillmentStatus',CASE WHEN p.sale_id IS NOT NULL THEN 'recorded'
            WHEN s.state='closed' THEN 'closed' ELSE 'held' END,
        'refunds',coalesce(p.refunded,0)::text,
        'reserved',coalesce(p.reserved,0)::text,'inFlight',coalesce(p.in_flight,0)::text,
        'events',coalesce((SELECT jsonb_agg(jsonb_build_object('eventId',o.event_id,'kind',o.kind,'state',o.state)
                    ORDER BY o.created_at,o.event_id) FROM ledger.fulfillment_outbox o WHERE o.sale_id=s.id),'[]'::jsonb),
        'refundDetails',coalesce((SELECT jsonb_agg(jsonb_build_object('refundId',r.id,'amount',r.amount::text,'state',r.state,
                    'confirmedAt',f.included_at,'transaction',f.tx_hash) ORDER BY r.requested_at,r.id)
                    FROM ledger.refund r LEFT JOIN ledger.refund_confirmation f ON f.refund_id=r.id WHERE r.sale_id=s.id),'[]'::jsonb)
    ), jsonb_build_object(
        'orderKey','0x'||encode(q.order_id,'hex'),'quoteDigest','0x'||encode(q.signed_digest,'hex'),
        'stream',coalesce(p.stream_id,sub.stream_id),'candidate',relay.tx_hash,
        'merchant',p.merchant_key,'block',p.inclusion_hash,'log',p.log_index
    ) FROM ledger.sale s
      LEFT JOIN ledger.payment p ON p.sale_id=s.id
      LEFT JOIN ledger.quote_outbox published ON published.sale_id=s.id
      LEFT JOIN ledger.quote_work q ON q.sale_id=s.id AND q.signed_digest=published.quote_digest
      LEFT JOIN ledger.settlement_submission sub ON sub.sale_id=q.sale_id AND sub.quote_digest=q.signed_digest
      LEFT JOIN ledger.settlement_work w ON w.sale_id=sub.sale_id AND w.refund_id IS NULL
      LEFT JOIN ledger.relay_attempt relay ON relay.work_id=w.id
      WHERE s.branch_id=branch AND s.id=order_;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied';
    END IF;
END $$;

REVOKE ALL ON FUNCTION ledger.finance_order_snapshot(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.finance_order_snapshot(uuid,uuid,uuid,uuid) TO nu54_p05_finance;

-- Preserve existing database callers and keep correlation values out of the public detail.
CREATE OR REPLACE FUNCTION ledger.finance_order(tenant uuid,branch uuid,actor uuid,order_ uuid)
RETURNS jsonb LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT snapshot.detail FROM ledger.finance_order_snapshot(tenant,branch,actor,order_) snapshot
$$;

-- Scope the additive original-payment details to acknowledged cafe detail reads.
-- The existing kitchen grant, legacy overload and queue response stay in place.
CREATE OR REPLACE FUNCTION ledger.kiosk_pickup_queue(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid,role_ text,include_cafe_ boolean)
RETURNS SETOF jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $kitchen$
BEGIN
    PERFORM ledger.kiosk_require_pickup(tenant_,branch_,actor_,role_);
    RETURN QUERY SELECT jsonb_strip_nulls(jsonb_build_object(
      'saleRef',s.id,'kind',CASE WHEN include_cafe_ THEN CASE WHEN o.sale_id IS NULL THEN 'cafe' ELSE 'travel' END END,
      'startsAt',coalesce(o.starts_at,paid.included_at,submission.received_at,c.expires_at),'timezone',b.timezone,
      'pickupNo',CASE WHEN ack.accepted THEN p.number END,
      'eventRef',CASE WHEN include_cafe_ AND ack.accepted THEN e.event_id END,
      'state',CASE WHEN ack.accepted THEN coalesce(k.state,'accepted') ELSE 'checking' END,
      'revision',coalesce(k.revision,0),
      'settlement',CASE WHEN detail.booked_at IS NOT NULL THEN jsonb_build_object(
        'amount',paid.principal::text,'asset',paid.asset_ref,
        'finalizedAt',to_char(detail.booked_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
        'pickupReadyAt',to_char(detail.ready_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) END,
      'items',CASE WHEN sale_ IS NOT NULL AND ack.accepted THEN (SELECT jsonb_agg(jsonb_build_object(
        'label',x.label,'option',l.option_key,'quantity',l.quantity) ORDER BY l.sku COLLATE "C",l.option_key COLLATE "C")
        FROM ledger.order_line l JOIN ledger.price x ON
          (x.branch_id,x.version,x.sku,x.option_key)=(l.branch_id,l.menu_version,l.sku,l.option_key)
        WHERE l.sale_id=s.id) END))
    FROM ledger.sale s JOIN ledger.order_checkout c ON (c.branch_id,c.sale_id)=(s.branch_id,s.id)
    JOIN ledger.branch b ON b.id=s.branch_id
    LEFT JOIN ledger.travel_pickup_order o ON o.sale_id=s.id
    LEFT JOIN ledger.travel_pickup_progress k ON k.sale_id=s.id
    LEFT JOIN ledger.payment paid ON paid.sale_id=s.id
    LEFT JOIN ledger.settlement_submission submission ON submission.sale_id=s.id
    LEFT JOIN ledger.pickup p ON p.sale_id=s.id
    LEFT JOIN ledger.fulfillment_outbox e ON e.sale_id=s.id AND e.kind='paid' AND e.source_ref=s.id
    LEFT JOIN ledger.consumer_receipt response ON response.event_id=e.event_id AND response.effect_key=e.effect_key
    CROSS JOIN LATERAL (SELECT s.state='paid' AND p.sale_id IS NOT NULL AND response.event_id IS NOT NULL AS accepted) ack
    LEFT JOIN LATERAL (
        SELECT booked.booked_at, first_ready.ready_at
          FROM ledger.journal booked
          CROSS JOIN LATERAL (
            SELECT transition.ready_at FROM ledger.travel_pickup_transition transition
             WHERE transition.sale_id=s.id AND transition.target='ready' AND transition.revision<=k.revision
             ORDER BY transition.revision LIMIT 1
          ) first_ready
         WHERE (booked.branch_id,booked.sale_id,booked.evidence_id)=(s.branch_id,s.id,paid.evidence_id)
           AND booked.refund_id IS NULL AND (booked.asset_ref,booked.amount)=(paid.asset_ref,paid.principal)
           AND isfinite(booked.booked_at) AND first_ready.ready_at>=booked.booked_at
    ) detail ON sale_ IS NOT NULL AND include_cafe_ AND role_='operator' AND o.sale_id IS NULL
          AND ack.accepted AND k.state IN ('ready','collected')
          AND (paid.principal,paid.asset_ref)=(s.amount,s.asset_ref)
          AND paid.reserved=0 AND paid.in_flight=0 AND paid.refunded=0
          AND e.state='delivered' AND e.payload_digest=response.payload_digest
    WHERE s.branch_id=branch_ AND (sale_ IS NULL OR s.id=sale_) AND (
       (o.sale_id IS NOT NULL AND ack.accepted) OR
       (o.sale_id IS NULL AND include_cafe_ AND role_='operator' AND s.state IN ('checking','paid') AND ledger.kiosk_menu_pickup(s.id))
    ) AND (sale_ IS NOT NULL OR coalesce(k.state,'accepted') NOT IN ('collected','unfulfillable'))
    ORDER BY ack.accepted DESC,coalesce(o.starts_at,paid.included_at,submission.received_at,c.expires_at),s.id LIMIT 32;
END
$kitchen$;

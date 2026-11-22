-- Keep the existing travel response and payment enums readable by deployed
-- clients. An updated operator explicitly opts in to the cafe projection.
CREATE FUNCTION ledger.kiosk_pickup_queue(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid,role_ text,include_cafe_ boolean)
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
      'items',CASE WHEN sale_ IS NOT NULL AND ack.accepted THEN (SELECT jsonb_agg(jsonb_build_object(
        'label',x.label,'option',l.option_key,'quantity',l.quantity) ORDER BY l.sku)
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
    WHERE s.branch_id=branch_ AND (sale_ IS NULL OR s.id=sale_) AND (
       (o.sale_id IS NOT NULL AND ack.accepted) OR
       (o.sale_id IS NULL AND include_cafe_ AND role_='operator' AND s.state IN ('checking','paid') AND ledger.kiosk_menu_pickup(s.id))
    ) AND (sale_ IS NOT NULL OR coalesce(k.state,'accepted') NOT IN ('collected','unfulfillable'))
    ORDER BY ack.accepted DESC,coalesce(o.starts_at,paid.included_at,submission.received_at,c.expires_at),s.id LIMIT 32;
END
$kitchen$;

CREATE OR REPLACE FUNCTION ledger.kiosk_pickup_queue(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid,role_ text)
RETURNS SETOF jsonb LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT * FROM ledger.kiosk_pickup_queue(tenant_,branch_,actor_,sale_,role_,false);
$$;
REVOKE ALL ON FUNCTION ledger.kiosk_pickup_queue(uuid,uuid,uuid,uuid,text,boolean) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.kiosk_pickup_queue(uuid,uuid,uuid,uuid,text,boolean) TO nu54_p05_commerce;

CREATE OR REPLACE FUNCTION ledger.commerce_order_receipt(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid)
RETURNS TABLE(id uuid,revision bigint,state text,asset_ref text,amount numeric,expires_at timestamptz,
    pickup_no bigint,receipt_ref uuid,quote_digest text,order_key text,receipt_kind text,subscription jsonb,
    payment jsonb,access_ends_at timestamptz)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $receipt$
    SELECT v.*,
        CASE WHEN evidence.stage IS NOT NULL THEN jsonb_strip_nulls(jsonb_build_object(
            'stage',evidence.stage,
            'chainId',8283,
            'relayTxHash',attempt.tx_hash,
            'txHash',paid.tx_hash,
            'includedAt',paid.included_at,
            'pickupStage',CASE WHEN v.state='paid' AND v.pickup_no IS NOT NULL THEN pickup.state END
        )) END,
        CASE WHEN grant_record.role_code='customer' AND checkout.creator<>actor_
                    AND (v.state<>'paid' OR v.receipt_ref IS NULL OR payer.sale_id IS NULL)
             THEN claim.expires_at END
    FROM ledger.commerce_order_status_with_invoice(tenant_,branch_,actor_,sale_) v
    CROSS JOIN LATERAL ledger.commerce_allowed(tenant_,branch_,actor_) grant_record
    JOIN ledger.order_checkout checkout ON (checkout.branch_id,checkout.sale_id)=(branch_,v.id)
    LEFT JOIN ledger.checkout_claim claim ON (claim.branch_id,claim.sale_id,claim.actor_id)=(branch_,v.id,actor_)
    LEFT JOIN ledger.quote_work payer ON payer.sale_id=v.id AND payer.signed_digest IS NOT NULL
        AND payer.actor_id=actor_ AND grant_record.role_code='customer'
    LEFT JOIN ledger.settlement_submission acceptance ON acceptance.sale_id=payer.sale_id
    LEFT JOIN ledger.settlement_work work ON work.sale_id=payer.sale_id AND work.refund_id IS NULL
    LEFT JOIN ledger.relay_attempt attempt ON attempt.work_id=work.id
    LEFT JOIN ledger.payment paid ON paid.sale_id=payer.sale_id AND v.state='paid'
    LEFT JOIN ledger.sale_resolution unpaid ON unpaid.sale_id=payer.sale_id AND v.state='closed'
    LEFT JOIN ledger.travel_pickup_progress pickup ON pickup.sale_id=paid.sale_id
    CROSS JOIN LATERAL (
        SELECT CASE
            WHEN paid.sale_id IS NOT NULL THEN 'finalized'
            WHEN unpaid.sale_id IS NOT NULL THEN 'unpaid'
            WHEN v.state='checking' AND attempt.state='signed' THEN 'relaySigned'
            WHEN v.state='checking' AND attempt.state='broadcast' THEN 'submitted'
            WHEN v.state='checking' AND attempt.state IN ('unknown','observed','reverted','final') THEN 'checking'
            WHEN v.state='checking' AND acceptance.sale_id IS NOT NULL THEN 'received'
        END AS stage
    ) evidence
    WHERE grant_record.role_code<>'customer' OR v.state<>'paid' OR payer.sale_id IS NOT NULL
          OR checkout.creator=actor_
$receipt$;

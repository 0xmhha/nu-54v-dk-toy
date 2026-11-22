-- Preserve receipt and payment finality; number visibility for travel pickup
-- follows the original paid outbox ACK used by the dedicated customer lookup.
-- Mask at the lowest commerce projection so existing grants cannot bypass it.
CREATE OR REPLACE FUNCTION ledger.commerce_order_view(tenant uuid, branch uuid, actor uuid, sale_ uuid)
RETURNS TABLE(id uuid, revision bigint, state text, asset_ref text, amount numeric, expires_at timestamptz, pickup_no bigint, receipt_ref uuid, quote_digest text, order_key text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE grant_ ledger.commerce_grant := ledger.commerce_allowed(tenant,branch,actor);
BEGIN
    RETURN QUERY
      SELECT o.id,o.revision,o.state,o.asset_ref,o.amount::numeric,c.expires_at,
        CASE WHEN grant_.role_code<>'customer' OR trip.sale_id IS NULL OR ledger.travel_pickup_accepted(o.id)
             THEN p.number END,
        COALESCE(p.receipt_ref,e.receipt_ref),
        CASE WHEN o.state='issued' THEN encode(q.signed_digest,'hex') END,
        CASE WHEN o.state='issued' THEN encode(q.order_id,'hex') END
      FROM ledger.sale o JOIN ledger.order_checkout c ON c.sale_id=o.id AND c.branch_id=o.branch_id
      LEFT JOIN ledger.pickup p ON p.branch_id=o.branch_id AND p.sale_id=o.id AND o.state='paid'
      LEFT JOIN ledger.travel_pickup_order trip ON trip.sale_id=o.id AND trip.branch_id=o.branch_id
      LEFT JOIN ledger.invoice i ON i.sale_id=o.id AND o.origin='subscription'
      LEFT JOIN ledger.subscription_entitlement e ON e.invoice_id=i.id AND o.state='paid'
      LEFT JOIN ledger.quote_work q ON q.sale_id=o.id AND q.signed_digest IS NOT NULL
      WHERE o.branch_id=branch AND o.id=sale_ AND (c.creator=actor OR
        EXISTS(SELECT FROM ledger.checkout_claim issued WHERE issued.branch_id=branch AND issued.sale_id=o.id
            AND issued.actor_id=actor AND issued.expires_at>statement_timestamp()) OR
        (o.state='paid' AND COALESCE(p.receipt_ref,e.receipt_ref) IS NOT NULL AND q.actor_id=actor) OR
        (grant_.role_code='customer' AND ledger.owns_closed_nearby_order(branch,actor,o.id)));
END $$;

CREATE OR REPLACE FUNCTION ledger.commerce_order_status(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid)
RETURNS TABLE(id uuid,revision bigint,state text,asset_ref text,amount numeric,expires_at timestamptz,
    pickup_no bigint,receipt_ref uuid,quote_digest text,order_key text,receipt_kind text)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $status$
    SELECT v.id,v.revision,v.state,v.asset_ref,v.amount,v.expires_at,v.pickup_no,
        coalesce(v.receipt_ref,j.id),v.quote_digest,v.order_key,
        CASE WHEN v.state='paid' AND i.fulfillment_kind='digital' THEN 'digital'
             WHEN v.state='paid' AND leg.sale_id IS NOT NULL THEN 'booking'
             WHEN v.state='paid' AND v.pickup_no IS NULL AND trip.sale_id IS NOT NULL
                 THEN 'pickup-awaiting-acceptance' END
    FROM ledger.commerce_order_view(tenant_,branch_,actor_,sale_) v
    JOIN ledger.sale s ON s.id=v.id AND s.branch_id=branch_
    LEFT JOIN ledger.travel_pickup_order trip ON trip.sale_id=s.id AND trip.branch_id=s.branch_id
    LEFT JOIN ledger.invoice i ON i.sale_id=s.id AND s.origin='subscription'
    LEFT JOIN ledger.travel_booking_leg leg ON leg.sale_id=s.id AND leg.branch_id=branch_ AND s.origin='travel'
    LEFT JOIN ledger.payment p ON p.sale_id=leg.sale_id AND v.state='paid'
    LEFT JOIN ledger.journal j ON j.sale_id=p.sale_id AND j.refund_id IS NULL
$status$;

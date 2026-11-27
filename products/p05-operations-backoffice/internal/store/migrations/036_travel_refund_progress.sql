CREATE FUNCTION ledger.travel_legs_with_refunds(tenant_ uuid,branch_ uuid,actor_ uuid,booking_ uuid)
RETURNS TABLE(kind text,sale_id uuid,state text,amount text,receipt_ref uuid,refunded text,
              refund_expected text,refund_state text)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $progress$
    SELECT leg.kind,leg.sale_id,leg.state,leg.amount,leg.receipt_ref,leg.refunded,expected.amount::text,
        CASE WHEN booking.cancel_requested_at IS NULL THEN 'not-requested'
             WHEN leg.state='closed' AND paid.sale_id IS NULL THEN 'not-due'
             WHEN expected.amount IS NULL THEN 'waiting-payment'
             WHEN expected.amount=0 THEN 'not-due'
             WHEN paid.refunded=expected.amount THEN 'settled'
             ELSE 'waiting-refund' END
    FROM ledger.travel_legs(tenant_,branch_,actor_,booking_) leg
    JOIN ledger.travel_booking booking ON booking.branch_id=branch_ AND booking.id=booking_ AND booking.actor_id=actor_
    LEFT JOIN ledger.payment paid ON paid.sale_id=leg.sale_id
    CROSS JOIN LATERAL (SELECT CASE WHEN booking.cancel_requested_at IS NOT NULL THEN
        trunc(paid.principal * CASE leg.kind WHEN 'deposit' THEN booking.deposit_refund_bps ELSE booking.balance_refund_bps END / 10000)
    END AS amount) expected
    ORDER BY CASE leg.kind WHEN 'deposit' THEN 0 ELSE 1 END
$progress$;

REVOKE ALL ON FUNCTION ledger.travel_legs_with_refunds(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.travel_legs_with_refunds(uuid,uuid,uuid,uuid) TO nu54_p05_commerce;

CREATE FUNCTION ledger.travel_cancellation_preview(tenant_ uuid,branch_ uuid,actor_ uuid,booking_ uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $preview$
DECLARE
    auth ledger.commerce_grant := ledger.commerce_allowed(tenant_,branch_,actor_);
    booking ledger.travel_booking;
    can_cancel boolean;
    legs jsonb;
BEGIN
    IF auth.role_code <> 'customer' THEN
        RAISE EXCEPTION USING ERRCODE='P0002', MESSAGE='booking unavailable';
    END IF;
    SELECT * INTO booking FROM ledger.travel_booking
      WHERE branch_id=branch_ AND actor_id=actor_ AND id=booking_;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE='P0002', MESSAGE='booking unavailable';
    END IF;
    can_cancel := booking.cancel_requested_at IS NULL AND booking.released_at IS NULL
        AND booking.cancellation_deadline > clock_timestamp();
    SELECT coalesce(jsonb_agg(jsonb_strip_nulls(jsonb_build_object(
        'kind',l.kind,'saleRef',l.sale_id,'paymentState',s.state,
        'refundExpected',CASE WHEN can_cancel AND s.state='paid' THEN
          trunc(p.principal * CASE l.kind WHEN 'deposit' THEN booking.deposit_refund_bps
             ELSE booking.balance_refund_bps END / 10000)::text END))
        ORDER BY CASE l.kind WHEN 'deposit' THEN 0 ELSE 1 END), '[]'::jsonb)
      INTO legs
      FROM ledger.travel_booking_leg l JOIN ledger.sale s ON (s.branch_id,s.id)=(l.branch_id,l.sale_id)
      LEFT JOIN ledger.payment p ON p.sale_id=l.sale_id
      WHERE (l.branch_id,l.booking_id)=(branch_,booking_);
    RETURN jsonb_build_object('canCancel',can_cancel,'cancellationDeadline',booking.cancellation_deadline,'legs',legs);
END
$preview$;

REVOKE ALL ON FUNCTION ledger.travel_cancellation_preview(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.travel_cancellation_preview(uuid,uuid,uuid,uuid) TO nu54_p05_commerce;

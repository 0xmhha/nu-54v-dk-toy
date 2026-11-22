ALTER TABLE ledger.refund ADD COLUMN reason_code text CHECK(reason_code IN ('partial_service','merchant_cancel','duplicate_charge'));

CREATE FUNCTION ledger.reserve_console_refund(tenant uuid,branch uuid,actor uuid,sale_ uuid,key_ uuid,amount_ numeric,reason_ text)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE refund_ uuid;
DECLARE prior_reason text;
BEGIN
    IF reason_ IS NULL OR reason_ NOT IN ('partial_service','merchant_cancel','duplicate_charge') THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid refund reason';
    END IF;
    -- reserve_refund retains the payment lock through this transaction.
    refund_:=ledger.reserve_refund(tenant,branch,actor,sale_,key_,amount_);
    SELECT f.reason_code INTO prior_reason FROM ledger.refund f WHERE f.id=refund_;
    IF prior_reason IS NULL THEN
        UPDATE ledger.refund f SET reason_code=reason_ WHERE f.id=refund_;
    ELSIF prior_reason<>reason_ THEN
        RAISE EXCEPTION USING ERRCODE='P1001', MESSAGE='refund reason differs';
    END IF;
    RETURN refund_;
END $$;

CREATE FUNCTION ledger.reject_console_refund(tenant uuid,branch uuid,actor uuid,sale_ uuid,refund_ uuid)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE item ledger.refund;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'refund_approve');
    PERFORM FROM ledger.payment p JOIN ledger.sale s ON s.id=p.sale_id
      WHERE p.sale_id=sale_ AND s.branch_id=branch FOR UPDATE OF p;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='refund scope denied'; END IF;
    SELECT * INTO item FROM ledger.refund r WHERE r.sale_id=sale_ AND r.id=refund_ FOR UPDATE;
    IF NOT FOUND OR item.requester=actor THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='independent decision required';
    END IF;
    IF item.state='cancelled' AND item.approver=actor THEN RETURN; END IF;
    IF item.state<>'requested' THEN
        RAISE EXCEPTION USING ERRCODE='P1001', MESSAGE='refund already decided';
    END IF;
    UPDATE ledger.refund r SET state='cancelled',approver=actor WHERE r.id=refund_;
    UPDATE ledger.payment p SET reserved=p.reserved-item.amount WHERE p.sale_id=sale_;
END $$;

CREATE FUNCTION ledger.console_refunds(tenant uuid,branch uuid,actor uuid,sale_ uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE capabilities text[];
DECLARE report jsonb;
BEGIN
    SELECT array_agg(g.capability) INTO capabilities FROM ledger.finance_grant g JOIN ledger.branch b ON b.id=g.branch_id
      WHERE b.id=branch AND b.tenant_ref=tenant AND g.actor_id=actor AND g.valid_until>clock_timestamp()
        AND g.capability IN ('refund_request','refund_approve');
    IF capabilities IS NULL THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='refund scope denied';
    END IF;
    SELECT jsonb_build_object('saleId',s.id,'asset',s.asset_ref,'principal',s.amount::text,
        'paid',p.sale_id IS NOT NULL,'payer',p.payer,'merchant',p.merchant_key,
        'reserved',coalesce(p.reserved,0)::text,'inFlight',coalesce(p.in_flight,0)::text,
        'refunded',coalesce(p.refunded,0)::text,
        'available',coalesce(p.principal-p.refunded-p.reserved-p.in_flight,0)::text,
        'canRequest','refund_request'=ANY(capabilities),
        'refunds',coalesce((SELECT jsonb_agg(jsonb_build_object(
             'id',r.id,'reason',r.reason_code,'amount',r.amount::text,'state',r.state,
             'canDecide','refund_approve'=ANY(capabilities) AND r.requester<>actor AND r.state='requested',
             'transaction',f.tx_hash,'confirmedAt',f.included_at) ORDER BY r.requested_at,r.id)
          FROM ledger.refund r LEFT JOIN ledger.refund_confirmation f ON f.refund_id=r.id WHERE r.sale_id=s.id),'[]'::jsonb))
      INTO report FROM ledger.sale s LEFT JOIN ledger.payment p ON p.sale_id=s.id
      WHERE s.branch_id=branch AND s.id=sale_;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='refund scope denied'; END IF;
    RETURN report;
END $$;
REVOKE ALL ON FUNCTION ledger.reserve_console_refund(uuid,uuid,uuid,uuid,uuid,numeric,text),
    ledger.reject_console_refund(uuid,uuid,uuid,uuid,uuid),ledger.console_refunds(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.reserve_console_refund(uuid,uuid,uuid,uuid,uuid,numeric,text),
    ledger.reject_console_refund(uuid,uuid,uuid,uuid,uuid),ledger.console_refunds(uuid,uuid,uuid,uuid) TO nu54_p05_finance;

ALTER TABLE ledger.refund ADD COLUMN rejection_code text
  CHECK(rejection_code IN ('route_review','amount_review','policy_review'));
ALTER TABLE ledger.refund ADD COLUMN rejected_at timestamptz;
ALTER TABLE ledger.refund ADD CONSTRAINT refund_review_evidence
  CHECK ((rejection_code IS NULL AND rejected_at IS NULL)
      OR (rejection_code IS NOT NULL AND rejected_at IS NOT NULL AND isfinite(rejected_at)
          AND state='cancelled' AND approver IS NOT NULL AND approver<>requester));

CREATE OR REPLACE FUNCTION ledger.reserve_refund(tenant uuid,branch uuid,actor uuid,sale_ uuid,key_ uuid,amount_ numeric)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $reserve$
DECLARE
    p ledger.payment;
    prior ledger.refund;
    result uuid;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'refund_request');
    SELECT payment.* INTO p FROM ledger.payment payment JOIN ledger.sale s ON s.id=payment.sale_id
      WHERE payment.sale_id=sale_ AND s.branch_id=branch FOR UPDATE OF payment;
    IF NOT FOUND THEN RAISE EXCEPTION 'payment scope denied' USING ERRCODE='42501'; END IF;
    SELECT * INTO prior FROM ledger.refund WHERE sale_id=sale_ AND request_key=key_;
    IF FOUND THEN
        IF prior.amount IS DISTINCT FROM amount_ OR prior.requester<>actor THEN
            RAISE EXCEPTION 'refund request key conflict' USING ERRCODE='P1001';
        END IF;
        RETURN prior.id;
    END IF;
    IF amount_ IS NULL OR amount_<=0 OR amount_<>trunc(amount_) OR p.reserved+p.in_flight+p.refunded+amount_>p.principal THEN
        RAISE EXCEPTION 'refund exceeds available principal' USING ERRCODE='P1030';
    END IF;
    UPDATE ledger.payment SET reserved=reserved+amount_ WHERE sale_id=sale_;
    INSERT INTO ledger.refund(sale_id,request_key,chain_refund_id,amount,requester,requested_at,state)
      VALUES(sale_,key_,sha256(convert_to(gen_random_uuid()::text||gen_random_uuid()::text,'UTF8')),
        amount_,actor,clock_timestamp(),'requested') RETURNING id INTO result;
    RETURN result;
END
$reserve$;

CREATE FUNCTION ledger.reject_console_refund(tenant uuid,branch uuid,actor uuid,sale_ uuid,refund_ uuid,rejection_ text)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE item ledger.refund;
BEGIN
    IF rejection_ IS NULL OR rejection_ NOT IN ('route_review','amount_review','policy_review') THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid refund rejection code';
    END IF;
    PERFORM ledger.require_finance(tenant,branch,actor,'refund_approve');
    PERFORM FROM ledger.payment p JOIN ledger.sale s ON s.id=p.sale_id
      WHERE p.sale_id=sale_ AND s.branch_id=branch FOR UPDATE OF p;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='refund scope denied'; END IF;
    SELECT * INTO item FROM ledger.refund r WHERE r.sale_id=sale_ AND r.id=refund_ FOR UPDATE;
    IF NOT FOUND OR item.requester=actor THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='independent decision required';
    END IF;
    IF item.state='cancelled' AND item.approver=actor AND item.rejection_code=rejection_ THEN RETURN; END IF;
    IF item.state<>'requested' THEN
        RAISE EXCEPTION USING ERRCODE='P1001', MESSAGE='refund already decided';
    END IF;
    UPDATE ledger.refund r SET state='cancelled',approver=actor,rejection_code=rejection_,rejected_at=clock_timestamp() WHERE r.id=refund_;
    UPDATE ledger.payment p SET reserved=p.reserved-item.amount WHERE p.sale_id=sale_;
END $$;

CREATE OR REPLACE FUNCTION ledger.console_refunds(tenant uuid,branch uuid,actor uuid,sale_ uuid)
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
             'rejectionCode',r.rejection_code,'rejectedAt',r.rejected_at,
             'transaction',f.tx_hash,'confirmedAt',f.included_at) ORDER BY r.requested_at,r.id)
          FROM ledger.refund r LEFT JOIN ledger.refund_confirmation f ON f.refund_id=r.id WHERE r.sale_id=s.id),'[]'::jsonb))
      INTO report FROM ledger.sale s LEFT JOIN ledger.payment p ON p.sale_id=s.id
      WHERE s.branch_id=branch AND s.id=sale_;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='refund scope denied'; END IF;
    RETURN report;
END $$;
REVOKE EXECUTE ON FUNCTION ledger.reject_console_refund(uuid,uuid,uuid,uuid,uuid) FROM nu54_p05_finance;
REVOKE ALL ON FUNCTION ledger.reject_console_refund(uuid,uuid,uuid,uuid,uuid,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.reject_console_refund(uuid,uuid,uuid,uuid,uuid,text) TO nu54_p05_finance;

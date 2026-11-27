-- Preserve existing financial liabilities and signed work without creating
-- a selectable legacy channel for new manual refunds.
ALTER TABLE ledger.refund ADD COLUMN decision_path text;
UPDATE ledger.refund SET decision_path=CASE WHEN reason_code IS NULL THEN 'prior' ELSE 'manual' END;
ALTER TABLE ledger.refund ALTER COLUMN decision_path SET DEFAULT 'manual';
ALTER TABLE ledger.refund ALTER COLUMN decision_path SET NOT NULL;
ALTER TABLE ledger.refund ADD CONSTRAINT refund_decision_path
  CHECK ((decision_path='prior' AND reason_code IS NULL) OR (decision_path='manual' AND reason_code IS NOT NULL));

CREATE OR REPLACE FUNCTION ledger.reserve_console_refund(tenant uuid,branch uuid,actor uuid,sale_ uuid,key_ uuid,amount_ numeric,reason_ text)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $reserve$
DECLARE
    p ledger.payment;
    prior ledger.refund;
    result uuid;
BEGIN
    IF reason_ IS NULL OR reason_ NOT IN ('partial_service','merchant_cancel','duplicate_charge') THEN
        RAISE EXCEPTION 'refund classification required' USING ERRCODE='23514';
    END IF;
    PERFORM ledger.require_finance(tenant,branch,actor,'refund_request');
    SELECT payment.* INTO p FROM ledger.payment payment JOIN ledger.sale s ON s.id=payment.sale_id
      WHERE payment.sale_id=sale_ AND s.branch_id=branch FOR UPDATE OF payment;
    IF NOT FOUND THEN RAISE EXCEPTION 'payment scope denied' USING ERRCODE='42501'; END IF;
    SELECT * INTO prior FROM ledger.refund WHERE sale_id=sale_ AND request_key=key_;
    IF FOUND THEN
        IF prior.amount IS DISTINCT FROM amount_ OR prior.requester<>actor OR prior.reason_code IS DISTINCT FROM reason_ THEN
            RAISE EXCEPTION 'refund request key conflict' USING ERRCODE='P1001';
        END IF;
        RETURN prior.id;
    END IF;
    IF amount_ IS NULL OR amount_<=0 OR amount_<>trunc(amount_) OR p.reserved+p.in_flight+p.refunded+amount_>p.principal THEN
        RAISE EXCEPTION 'refund exceeds available principal' USING ERRCODE='P1030';
    END IF;
    UPDATE ledger.payment SET reserved=reserved+amount_ WHERE sale_id=sale_;
    INSERT INTO ledger.refund(sale_id,request_key,chain_refund_id,amount,requester,requested_at,state,reason_code)
      VALUES(sale_,key_,sha256(convert_to(gen_random_uuid()::text||gen_random_uuid()::text,'UTF8')),
        amount_,actor,clock_timestamp(),'requested',reason_) RETURNING id INTO result;
    RETURN result;
END
$reserve$;

CREATE OR REPLACE FUNCTION ledger.enqueue_refund_work() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $queue$
BEGIN
    -- Earlier service-to-service reservations keep their existing worker path.
    IF NEW.decision_path='prior' THEN
        INSERT INTO ledger.settlement_work(id,sale_id,refund_id) VALUES(NEW.id,NEW.sale_id,NEW.id) ON CONFLICT DO NOTHING;
    END IF;
    RETURN NEW;
END
$queue$;

CREATE FUNCTION ledger.replay_staff_refund(tenant uuid,branch uuid,actor uuid,sale_ uuid,key_ uuid,amount_ numeric,reason_ text)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE prior ledger.refund;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'refund_request');
    PERFORM FROM ledger.payment p JOIN ledger.sale s ON s.id=p.sale_id
      WHERE p.sale_id=sale_ AND s.branch_id=branch;
    IF NOT FOUND THEN RAISE EXCEPTION 'payment scope denied' USING ERRCODE='42501'; END IF;
    SELECT * INTO prior FROM ledger.refund f WHERE f.sale_id=sale_ AND f.request_key=key_;
    IF NOT FOUND THEN RETURN NULL; END IF;
    IF (prior.amount,prior.requester,prior.reason_code) IS DISTINCT FROM (amount_,actor,reason_) THEN
        RAISE EXCEPTION 'refund request key conflict' USING ERRCODE='P1001';
    END IF;
    RETURN prior.id;
END $$;
REVOKE ALL ON FUNCTION ledger.replay_staff_refund(uuid,uuid,uuid,uuid,uuid,numeric,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.replay_staff_refund(uuid,uuid,uuid,uuid,uuid,numeric,text) TO nu54_p05_finance;

CREATE OR REPLACE FUNCTION ledger.reserve_refund(tenant uuid,branch uuid,actor uuid,sale_ uuid,key_ uuid,amount_ numeric)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE prior uuid;
BEGIN
    prior:=ledger.replay_staff_refund(tenant,branch,actor,sale_,key_,amount_,NULL);
    IF prior IS NULL THEN RAISE EXCEPTION 'prior refund reference required' USING ERRCODE='P1001'; END IF;
    RETURN prior;
END $$;

ALTER TABLE ledger.refund ADD COLUMN review_revision bigint NOT NULL DEFAULT 0 CHECK(review_revision>=0);
CREATE TABLE ledger.refund_failed_attempt (
    refund_id uuid NOT NULL REFERENCES ledger.refund,
    review_revision bigint NOT NULL CHECK(review_revision>=0),
    tx_hash text NOT NULL UNIQUE CHECK(tx_hash ~ '^0x[0-9a-f]{64}$'),
    sender text NOT NULL CHECK(sender ~ '^0x[0-9a-f]{40}$'),
    nonce numeric NOT NULL CHECK(nonce>=0 AND nonce=trunc(nonce) AND nonce<power(2::numeric,64)),
    block_hash text NOT NULL CHECK(block_hash ~ '^0x[0-9a-f]{64}$'),
    height bigint NOT NULL CHECK(height>=0),
    checker uuid NOT NULL,
    treasury uuid NOT NULL,
    recorded_at timestamptz NOT NULL CHECK(isfinite(recorded_at)),
    PRIMARY KEY(refund_id,review_revision),
    UNIQUE(sender,nonce)
);
REVOKE ALL ON ledger.refund_failed_attempt FROM PUBLIC;

CREATE FUNCTION ledger.reopen_finalized_refund(work_ uuid,fence_ bigint,hash_ text,block_ text,height_ bigint)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE job ledger.settlement_work := ledger.valid_settlement_lease(work_,fence_);
DECLARE item ledger.refund;
DECLARE relay ledger.relay_attempt;
BEGIN
    IF job.refund_id IS NULL THEN RETURN false; END IF;
    PERFORM FROM ledger.payment WHERE sale_id=job.sale_id FOR UPDATE;
    SELECT * INTO STRICT item FROM ledger.refund WHERE id=job.refund_id FOR UPDATE;
    SELECT * INTO relay FROM ledger.relay_attempt WHERE work_id=job.id FOR UPDATE;
    IF NOT FOUND OR relay.tx_hash IS DISTINCT FROM hash_ OR relay.state='final' OR
       item.decision_path<>'manual' OR item.state<>'inflight' OR item.approver IS NULL OR item.treasury_actor IS NULL OR
       EXISTS(SELECT FROM ledger.refund_confirmation c WHERE c.refund_id=item.id) THEN
        RETURN false;
    END IF;
    INSERT INTO ledger.refund_failed_attempt VALUES(item.id,item.review_revision,relay.tx_hash,relay.sender,
        relay.nonce,block_,height_,item.approver,item.treasury_actor,clock_timestamp());
    UPDATE ledger.payment SET in_flight=in_flight-item.amount,reserved=reserved+item.amount WHERE sale_id=job.sale_id;
    UPDATE ledger.refund SET state='requested',approver=NULL,treasury_actor=NULL,treasury_submitted_at=NULL,
        review_revision=review_revision+1 WHERE id=item.id;
    DELETE FROM ledger.relay_attempt WHERE work_id=job.id;
    UPDATE ledger.settlement_work SET finished=true,lease_until=NULL WHERE id=job.id;
    RETURN true;
END $$;
REVOKE ALL ON FUNCTION ledger.reopen_finalized_refund(uuid,bigint,text,text,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.reopen_finalized_refund(uuid,bigint,text,text,bigint) TO nu54_p05_settlement;

-- Retain the uniqueness of a nonce even after the signed attempt moves to history.
CREATE FUNCTION ledger.reject_archived_relay_nonce() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
BEGIN
    IF EXISTS(SELECT FROM ledger.refund_failed_attempt WHERE sender=NEW.sender AND nonce=NEW.nonce) THEN
        RAISE EXCEPTION 'archived relay nonce cannot be reused' USING ERRCODE='P1004';
    END IF;
    RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION ledger.reject_archived_relay_nonce() FROM PUBLIC;
CREATE TRIGGER relay_nonce_archive_guard BEFORE INSERT ON ledger.relay_attempt
FOR EACH ROW EXECUTE FUNCTION ledger.reject_archived_relay_nonce();

CREATE FUNCTION ledger.approve_refund(tenant uuid,branch uuid,actor uuid,sale_ uuid,refund_ uuid,revision_ bigint)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $approve$
DECLARE
    r ledger.refund;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'refund_approve');
    SELECT f.* INTO r FROM ledger.refund f JOIN ledger.sale s ON s.id=f.sale_id
        WHERE f.id=refund_ AND f.sale_id=sale_ AND s.branch_id=branch FOR UPDATE OF f;
    IF NOT FOUND OR r.requester=actor THEN RAISE EXCEPTION 'independent refund approval required' USING ERRCODE='42501'; END IF;
    IF r.review_revision IS DISTINCT FROM revision_ THEN
        RAISE EXCEPTION 'refund review changed' USING ERRCODE='P1001';
    END IF;
    IF r.approver=actor AND r.state<>'cancelled' THEN RETURN false; END IF;
    IF r.state<>'requested' THEN RAISE EXCEPTION 'refund is already decided' USING ERRCODE='P1001'; END IF;
    UPDATE ledger.refund SET approver=actor,state='approved' WHERE id=refund_;
    RETURN true;
END
$approve$;

CREATE FUNCTION ledger.reject_console_refund(tenant uuid,branch uuid,actor uuid,sale_ uuid,refund_ uuid,rejection_ text,revision_ bigint)
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
    IF item.review_revision IS DISTINCT FROM revision_ THEN
        RAISE EXCEPTION 'refund review changed' USING ERRCODE='P1001';
    END IF;
    IF item.state='cancelled' AND item.approver=actor AND item.rejection_code=rejection_ THEN RETURN; END IF;
    IF item.state<>'requested' THEN
        RAISE EXCEPTION USING ERRCODE='P1001', MESSAGE='refund already decided';
    END IF;
    UPDATE ledger.refund r SET state='cancelled',approver=actor,rejection_code=rejection_,rejected_at=clock_timestamp() WHERE r.id=refund_;
    UPDATE ledger.payment p SET reserved=p.reserved-item.amount WHERE p.sale_id=sale_;
END $$;

CREATE FUNCTION ledger.submit_console_refund(tenant uuid,branch uuid,actor uuid,sale_ uuid,refund_ uuid,
    merchant_ text,funds_ numeric,revision_ bigint)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $submit$
DECLARE item ledger.refund;
DECLARE payment ledger.payment;
DECLARE committed_refs numeric;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'refund_submit');
    SELECT p.* INTO payment FROM ledger.payment p JOIN ledger.sale s ON s.id=p.sale_id
      WHERE p.sale_id=sale_ AND s.branch_id=branch FOR UPDATE OF p;
    IF NOT FOUND OR payment.merchant_key<>merchant_ THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='refund submission scope denied';
    END IF;
    SELECT * INTO item FROM ledger.refund r WHERE r.sale_id=sale_ AND r.id=refund_ FOR UPDATE;
    IF NOT FOUND OR item.requester=actor OR item.approver=actor THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='separate treasury operator required';
    END IF;
    IF item.review_revision IS DISTINCT FROM revision_ THEN
        RAISE EXCEPTION 'refund review changed' USING ERRCODE='P1001';
    END IF;
    IF item.treasury_actor=actor THEN RETURN; END IF;
    IF item.state<>'approved' OR item.reason_code IS NULL OR item.treasury_actor IS NOT NULL
       OR EXISTS(SELECT FROM ledger.settlement_work w WHERE w.refund_id=item.id AND NOT w.finished) THEN
        RAISE EXCEPTION USING ERRCODE='P1001', MESSAGE='approved refund is already submitted or unavailable';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(payment.stream_id::text||payment.merchant_key,57));
    SELECT coalesce(sum(r.amount),0) INTO committed_refs FROM ledger.refund r
      JOIN ledger.payment p ON p.sale_id=r.sale_id
      WHERE p.stream_id=payment.stream_id AND p.merchant_key=payment.merchant_key
        AND r.treasury_actor IS NOT NULL AND r.state IN ('approved','inflight');
    IF funds_ IS NULL OR funds_<committed_refs+item.amount THEN
        RAISE EXCEPTION USING ERRCODE='P1032', MESSAGE='refund treasury reserve insufficient';
    END IF;
    UPDATE ledger.refund SET treasury_actor=actor,treasury_submitted_at=clock_timestamp() WHERE id=item.id;
    INSERT INTO ledger.settlement_work(id,sale_id,refund_id) VALUES(item.id,item.sale_id,item.id)
    ON CONFLICT(id) DO UPDATE SET finished=false,retry_at=clock_timestamp(),lease_until=NULL
    WHERE ledger.settlement_work.finished;
END
$submit$;
CREATE OR REPLACE FUNCTION ledger.console_refunds(tenant uuid,branch uuid,actor uuid,sale_ uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE capabilities text[];
DECLARE report jsonb;
BEGIN
    SELECT array_agg(g.capability) INTO capabilities FROM ledger.finance_grant g JOIN ledger.branch b ON b.id=g.branch_id
      WHERE b.id=branch AND b.tenant_ref=tenant AND g.actor_id=actor AND g.valid_until>clock_timestamp()
        AND g.capability IN ('refund_request','refund_approve','refund_submit');
    IF capabilities IS NULL THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='refund scope denied';
    END IF;
    SELECT jsonb_build_object('saleId',s.id,'asset',s.asset_ref,'principal',s.amount::text,
        'paid',p.sale_id IS NOT NULL,'payer',p.payer,'merchant',p.merchant_key,
        'payerGeneration',p.payer_generation::text,'rentalWithdrawal',rental.withdraw_ref,'rentalPayer',q.payer_role=2,
        'orderBusinessDay',(SELECT min(j.business_day) FROM ledger.journal j WHERE j.sale_id=s.id AND j.refund_id IS NULL),
        'canManageFunds','refund_submit'=ANY(capabilities),
        'fundsCommitted',(SELECT coalesce(sum(q.amount),0)::text FROM ledger.refund q
             JOIN ledger.payment original ON original.sale_id=q.sale_id
             WHERE original.stream_id=p.stream_id AND original.merchant_key=p.merchant_key
               AND q.treasury_actor IS NOT NULL AND q.state IN ('approved','inflight')),
        'reserved',coalesce(p.reserved,0)::text,'inFlight',coalesce(p.in_flight,0)::text,
        'refunded',coalesce(p.refunded,0)::text,
        'available',coalesce(p.principal-p.refunded-p.reserved-p.in_flight,0)::text,
        'canRequest','refund_request'=ANY(capabilities),
        'refunds',coalesce((SELECT jsonb_agg(jsonb_build_object(
             'id',r.id,'reason',r.reason_code,'amount',r.amount::text,'state',r.state,
             'canDecide','refund_approve'=ANY(capabilities) AND r.requester<>actor AND r.state='requested',
             'rejectionCode',r.rejection_code,'rejectedAt',r.rejected_at,
             'submitted',r.treasury_submitted_at IS NOT NULL OR a.work_id IS NOT NULL,
             'reviewRevision',r.review_revision,
             'previousFailure',(SELECT old.tx_hash FROM ledger.refund_failed_attempt old
                  WHERE old.refund_id=r.id ORDER BY old.review_revision DESC LIMIT 1),
             'canSubmit','refund_submit'=ANY(capabilities) AND r.requester<>actor AND r.approver<>actor
                 AND r.state='approved' AND r.reason_code IS NOT NULL AND r.treasury_submitted_at IS NULL
                 AND NOT EXISTS(SELECT FROM ledger.settlement_work w WHERE w.refund_id=r.id AND NOT w.finished),
             'pendingTransaction',CASE WHEN a.state NOT IN ('reserved','signed','final') THEN a.tx_hash END,
             'businessDay',(SELECT min(j.business_day) FROM ledger.journal j WHERE j.sale_id=s.id AND j.refund_id=r.id),
             'transaction',f.tx_hash,'confirmedAt',f.included_at) ORDER BY r.requested_at,r.id)
          FROM ledger.refund r LEFT JOIN ledger.refund_confirmation f ON f.refund_id=r.id
          LEFT JOIN ledger.relay_attempt a ON a.work_id=r.id WHERE r.sale_id=s.id),'[]'::jsonb))
      INTO report FROM ledger.sale s LEFT JOIN ledger.payment p ON p.sale_id=s.id
      LEFT JOIN ledger.quote_work q ON q.sale_id=p.sale_id
      LEFT JOIN ledger.rental_enrollment rental ON rental.branch_id=s.branch_id AND rental.account_ref=p.payer
      WHERE s.branch_id=branch AND s.id=sale_;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='refund scope denied'; END IF;
    RETURN report;
END $$;

-- Version-zero compatibility can only decide the first review.
CREATE OR REPLACE FUNCTION ledger.approve_refund(tenant uuid,branch uuid,actor uuid,sale_ uuid,refund_ uuid)
RETURNS boolean LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog AS $$
    SELECT ledger.approve_refund(tenant,branch,actor,sale_,refund_,0);
$$;
REVOKE EXECUTE ON FUNCTION ledger.reject_console_refund(uuid,uuid,uuid,uuid,uuid,text),
    ledger.submit_console_refund(uuid,uuid,uuid,uuid,uuid,text,numeric) FROM nu54_p05_finance;
REVOKE ALL ON FUNCTION ledger.approve_refund(uuid,uuid,uuid,uuid,uuid,bigint),
    ledger.reject_console_refund(uuid,uuid,uuid,uuid,uuid,text,bigint),
    ledger.submit_console_refund(uuid,uuid,uuid,uuid,uuid,text,numeric,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.approve_refund(uuid,uuid,uuid,uuid,uuid,bigint),
    ledger.reject_console_refund(uuid,uuid,uuid,uuid,uuid,text,bigint),
    ledger.submit_console_refund(uuid,uuid,uuid,uuid,uuid,text,numeric,bigint) TO nu54_p05_finance;

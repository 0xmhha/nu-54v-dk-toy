ALTER TABLE ledger.finance_grant DROP CONSTRAINT finance_grant_capability_check;
ALTER TABLE ledger.finance_grant ADD CONSTRAINT finance_grant_capability_check CHECK(capability IN (
    'refund_request','refund_approve','refund_submit','day_close','report_read','report_export','reconcile_read','reconcile_retry',
    'payout_propose','payout_approve','payout_notice','rental_manage','firmware_manage','firmware_station',
    'experience_read','experience_cancel','access_manage','audit_read','legacy_ledger','legacy_cashout','legacy_reconcile'
));

ALTER TABLE ledger.refund ADD COLUMN treasury_actor uuid;
ALTER TABLE ledger.refund ADD COLUMN treasury_submitted_at timestamptz;
ALTER TABLE ledger.refund ADD CONSTRAINT refund_treasury_evidence
  CHECK ((treasury_actor IS NULL AND treasury_submitted_at IS NULL)
      OR (treasury_actor IS NOT NULL AND treasury_submitted_at IS NOT NULL
          AND isfinite(treasury_submitted_at) AND treasury_actor<>requester
          AND approver IS NOT NULL AND treasury_actor<>approver AND state IN ('approved','inflight','final')));

CREATE OR REPLACE FUNCTION ledger.enqueue_refund_work() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $queue$
BEGIN
    -- Earlier service-to-service reservations keep their existing worker path.
    IF NEW.reason_code IS NULL THEN
        INSERT INTO ledger.settlement_work(id,sale_id,refund_id) VALUES(NEW.id,NEW.sale_id,NEW.id) ON CONFLICT DO NOTHING;
    END IF;
    RETURN NEW;
END
$queue$;

CREATE FUNCTION ledger.submit_console_refund(tenant uuid,branch uuid,actor uuid,sale_ uuid,refund_ uuid,
    merchant_ text,funds_ numeric)
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
    IF item.treasury_actor=actor THEN RETURN; END IF;
    IF item.state<>'approved' OR item.reason_code IS NULL OR item.treasury_actor IS NOT NULL
       OR EXISTS(SELECT FROM ledger.settlement_work w WHERE w.refund_id=item.id) THEN
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
    INSERT INTO ledger.settlement_work(id,sale_id,refund_id) VALUES(item.id,item.sale_id,item.id);
END
$submit$;
REVOKE ALL ON FUNCTION ledger.submit_console_refund(uuid,uuid,uuid,uuid,uuid,text,numeric) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.submit_console_refund(uuid,uuid,uuid,uuid,uuid,text,numeric) TO nu54_p05_finance;

CREATE OR REPLACE FUNCTION ledger.operator_scopes(tenant uuid,actor uuid,role_ text)
RETURNS jsonb LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT coalesce(jsonb_agg(jsonb_build_object('id',b.id,'label',coalesce(n.label,'등록 이름 없음 · '||right(b.id::text,8)),'timezone',b.timezone,
        'capabilities',g.capabilities) ORDER BY n.label,b.id),'[]'::jsonb)
    FROM ledger.branch b LEFT JOIN ledger.operator_branch_label n ON n.branch_id=b.id
    JOIN LATERAL (
       SELECT array_agg(DISTINCT permission ORDER BY permission) AS capabilities
       FROM (
         SELECT 'catalog'::text AS permission FROM ledger.staff_grant s WHERE role_='operator' AND s.branch_id=b.id
           AND s.subject_ref=actor AND s.role_code='operator' AND s.valid_from<=clock_timestamp() AND s.revoked_at IS NULL
         UNION ALL
         SELECT f.capability FROM ledger.finance_grant f WHERE f.branch_id=b.id AND f.actor_id=actor AND
           f.valid_until>clock_timestamp() AND
           (role_='finance' AND f.capability IN ('refund_request','refund_approve','refund_submit','day_close','report_read','report_export','reconcile_read','reconcile_retry')
            OR role_='operator' AND f.capability IN ('experience_read','experience_cancel','payout_propose','payout_approve','payout_notice',
                   'rental_manage','firmware_manage','firmware_station','access_manage','audit_read','legacy_ledger','legacy_cashout','legacy_reconcile'))
       ) authorized
    ) g ON cardinality(g.capabilities)>0
    WHERE b.tenant_ref=tenant;
$$;

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
             'canSubmit','refund_submit'=ANY(capabilities) AND r.requester<>actor AND r.approver<>actor
                 AND r.state='approved' AND r.reason_code IS NOT NULL AND r.treasury_submitted_at IS NULL
                 AND NOT EXISTS(SELECT FROM ledger.settlement_work w WHERE w.refund_id=r.id),
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

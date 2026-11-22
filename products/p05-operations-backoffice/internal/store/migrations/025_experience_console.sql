ALTER TABLE ledger.finance_grant DROP CONSTRAINT finance_grant_capability_check;
ALTER TABLE ledger.finance_grant ADD CONSTRAINT finance_grant_capability_check CHECK(capability IN (
    'refund_request','refund_approve','day_close','report_read','report_export','reconcile_read','reconcile_retry',
    'payout_propose','payout_approve','payout_notice','rental_manage','firmware_manage','firmware_station',
    'experience_read','experience_cancel'
));

CREATE TABLE ledger.operator_subscription_cancel (
    branch_id uuid NOT NULL,
    actor_id uuid NOT NULL REFERENCES ledger.login_subject(subject_ref),
    key uuid NOT NULL,
    subscription_id uuid NOT NULL,
    reviewed_revision bigint NOT NULL,
    cancelled_at timestamptz NOT NULL,
    PRIMARY KEY(branch_id,actor_id,key),
    FOREIGN KEY(branch_id,subscription_id) REFERENCES ledger.subscription(branch_id,id)
);
REVOKE ALL ON ledger.operator_subscription_cancel FROM PUBLIC;

CREATE FUNCTION ledger.operator_experience(tenant uuid,branch uuid,actor uuid,detail_ uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE bookings jsonb;
DECLARE schedules jsonb;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'experience_read');
    SELECT coalesce(jsonb_agg(q.view ORDER BY q.expires_at DESC,q.id),'[]'::jsonb) INTO bookings FROM (
       SELECT b.id,b.expires_at,jsonb_build_object('id',b.id,'visitAt',s.starts_at,'partySize',b.party_size,
         'expiresAt',b.expires_at,'issuedAt',b.issuance_started_at,'cancelledAt',b.cancel_requested_at,'releasedAt',b.released_at,
         'termsVersion',b.terms_version,'deposit',b.deposit::text,'balance',b.balance::text,
         'legs',coalesce((SELECT jsonb_agg(jsonb_build_object(
           'kind',l.kind,'order',sale.id,'state',sale.state,'amount',sale.amount::text,'expiresAt',q.expires_at,
           'businessDay',j.business_day,'refundable',coalesce(p.principal-p.refunded-p.in_flight-p.reserved,0)::text)
           ORDER BY l.kind DESC)
           FROM ledger.travel_booking_leg l JOIN ledger.sale sale ON (sale.branch_id,sale.id)=(l.branch_id,l.sale_id)
           LEFT JOIN ledger.quote_work q ON q.sale_id=sale.id LEFT JOIN ledger.payment p ON p.sale_id=sale.id
           LEFT JOIN ledger.journal j ON j.sale_id=sale.id AND j.refund_id IS NULL
           WHERE l.branch_id=branch AND l.booking_id=b.id),'[]'::jsonb)) AS view
       FROM ledger.travel_booking b JOIN ledger.travel_slot s ON (s.branch_id,s.id)=(b.branch_id,b.slot_id)
       WHERE b.branch_id=branch AND (detail_ IS NULL OR b.id=detail_)
       ORDER BY b.expires_at DESC,b.id LIMIT 50
    ) q;
    SELECT coalesce(jsonb_agg(q.view ORDER BY q.accepted_at DESC,q.id),'[]'::jsonb) INTO schedules FROM (
       SELECT s.id,s.accepted_at,jsonb_build_object('id',s.id,'state',s.state,'revision',s.revision,
         'nextCycle',s.next_cycle,'nextDue',s.next_due_at,'cancelledAt',s.cancelled_at,
         'canCancel',EXISTS(SELECT FROM ledger.finance_grant g WHERE g.branch_id=branch AND g.actor_id=actor
           AND g.capability='experience_cancel' AND g.valid_until>clock_timestamp()),
         'invoices',coalesce((SELECT jsonb_agg(i.view ORDER BY i.cycle_key DESC) FROM (
            SELECT p.cycle_key,jsonb_build_object('id',p.id,'cycle',p.cycle_key,'dueAt',p.due_at,'order',sale.id,
              'state',sale.state,'amount',sale.amount::text,'grantedAt',e.granted_at,'revokedAt',e.revoked_at) AS view
            FROM ledger.invoice p JOIN ledger.sale sale ON (sale.branch_id,sale.id)=(p.branch_id,p.sale_id)
            LEFT JOIN ledger.subscription_entitlement e ON e.invoice_id=p.id
            WHERE p.branch_id=branch AND p.schedule_ref=s.id ORDER BY p.cycle_key DESC LIMIT 25
         ) i),'[]'::jsonb)) AS view
       FROM ledger.subscription s WHERE s.branch_id=branch AND (detail_ IS NULL OR s.id=detail_)
       ORDER BY s.accepted_at DESC,s.id LIMIT 50
    ) q;
    IF detail_ IS NOT NULL AND jsonb_array_length(bookings)+jsonb_array_length(schedules)=0 THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='experience scope denied';
    END IF;
    RETURN jsonb_build_object('bookings',bookings,'subscriptions',schedules);
END $$;

CREATE FUNCTION ledger.operator_cancel_subscription(tenant uuid,branch uuid,actor uuid,subject uuid,key_ uuid,revision_ bigint)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE prior ledger.operator_subscription_cancel;
DECLARE schedule ledger.subscription;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'experience_cancel');
    PERFORM ledger.require_finance(tenant,branch,actor,'experience_read');
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||actor::text||key_::text,28));
    SELECT * INTO prior FROM ledger.operator_subscription_cancel c WHERE c.branch_id=branch AND c.actor_id=actor AND c.key=key_;
    IF FOUND THEN
        IF (prior.subscription_id,prior.reviewed_revision) IS DISTINCT FROM (subject,revision_) THEN
            RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='cancel request conflict';
        END IF;
        RETURN ledger.operator_experience(tenant,branch,actor,subject);
    END IF;
    SELECT * INTO schedule FROM ledger.subscription s WHERE s.branch_id=branch AND s.id=subject FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='experience scope denied'; END IF;
    IF schedule.revision<>revision_ OR schedule.state='cancelled' THEN
        RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='subscription changed';
    END IF;
    UPDATE ledger.subscription s SET state='cancelled',cancelled_at=clock_timestamp(),
        next_cycle=NULL,next_due_at=NULL,revision=s.revision+1 WHERE s.branch_id=branch AND s.id=subject;
    INSERT INTO ledger.operator_subscription_cancel VALUES(branch,actor,key_,subject,revision_,clock_timestamp());
    RETURN ledger.operator_experience(tenant,branch,actor,subject);
END $$;

REVOKE ALL ON FUNCTION ledger.operator_experience(uuid,uuid,uuid,uuid),
    ledger.operator_cancel_subscription(uuid,uuid,uuid,uuid,uuid,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.operator_experience(uuid,uuid,uuid,uuid),
    ledger.operator_cancel_subscription(uuid,uuid,uuid,uuid,uuid,bigint) TO nu54_p05_commerce;

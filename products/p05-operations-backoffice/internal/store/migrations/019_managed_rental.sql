ALTER TABLE ledger.finance_grant DROP CONSTRAINT finance_grant_capability_check;
ALTER TABLE ledger.finance_grant ADD CONSTRAINT finance_grant_capability_check CHECK(capability IN (
    'refund_request','refund_approve','day_close','report_read','report_export','reconcile_read','reconcile_retry',
    'payout_propose','payout_approve','payout_notice','rental_manage'
));

CREATE TABLE ledger.rental_enrollment(
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    device_ref uuid NOT NULL,
    generation bigint NOT NULL CHECK(generation>0),
    account_ref text NOT NULL CHECK(account_ref ~ '^0x[0-9a-f]{40}$'),
    withdraw_ref text NOT NULL CHECK(withdraw_ref ~ '^0x[0-9a-f]{40}$'),
    revision text NOT NULL CHECK(revision ~ '^[a-z0-9_-]{1,32}$'),
    enrollment_digest bytea NOT NULL CHECK(octet_length(enrollment_digest)=32),
    attested_until timestamptz NOT NULL CHECK(isfinite(attested_until)),
    no_pending_until timestamptz CHECK(isfinite(no_pending_until)),
    PRIMARY KEY(branch_id,device_ref,generation),
    UNIQUE(account_ref)
);
CREATE TABLE ledger.rental_operation(
    branch_id uuid NOT NULL,
    device_ref uuid NOT NULL,
    generation bigint NOT NULL,
    action text NOT NULL CHECK(action IN ('open','close','withdraw','cleanup')),
    request_key uuid NOT NULL,
    actor_id uuid NOT NULL REFERENCES ledger.login_subject(subject_ref),
    amount ledger.units,
    status text NOT NULL CHECK(status IN ('dispatching','submitted','unknown','acknowledged')),
    evidence jsonb,
    started_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(branch_id,device_ref,generation,action),
    UNIQUE(branch_id,actor_id,request_key),
    FOREIGN KEY(branch_id,device_ref,generation) REFERENCES ledger.rental_enrollment,
    CHECK((action='open')=(amount IS NOT NULL)),
    CHECK(amount IS NULL OR amount>0)
);
REVOKE ALL ON ledger.rental_enrollment,ledger.rental_operation FROM PUBLIC;

CREATE FUNCTION ledger.rental_member(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE subject ledger.rental_enrollment;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'rental_manage');
    SELECT * INTO subject FROM ledger.rental_enrollment e WHERE e.branch_id=branch AND e.device_ref=device
      AND e.generation=generation_ AND e.attested_until>clock_timestamp();
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='enrolled rental device required'; END IF;
    RETURN jsonb_build_object('account',subject.account_ref,'destination',subject.withdraw_ref,'revision',subject.revision,
       'generation',subject.generation,'hold',coalesce(subject.no_pending_until<=clock_timestamp(),true),
       'actions',coalesce((SELECT jsonb_agg(jsonb_build_object('action',o.action,'requestKey',o.request_key,
            'status',o.status,'evidence',o.evidence) ORDER BY o.started_at)
            FROM ledger.rental_operation o WHERE o.branch_id=branch AND o.device_ref=device AND o.generation=generation_),
           '[]'::jsonb));
END $$;

CREATE FUNCTION ledger.begin_rental_action(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,
    action_ text,key_ uuid,amount_ numeric)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE subject jsonb;
DECLARE prior ledger.rental_operation;
BEGIN
    subject:=ledger.rental_member(tenant,branch,actor,device,generation_);
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||device::text||generation_::text,25));
    SELECT * INTO prior FROM ledger.rental_operation o WHERE o.branch_id=branch AND o.device_ref=device
      AND o.generation=generation_ AND o.action=action_;
    IF FOUND THEN
        IF (prior.request_key,prior.actor_id,prior.amount) IS DISTINCT FROM (key_,actor,amount_) THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='same enrolled action already reserved';
        END IF;
        RETURN false;
    END IF;
    IF action_ IS DISTINCT FROM 'open' AND (subject->>'hold')::boolean THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='v1 pending payment evidence held';
    END IF;
    INSERT INTO ledger.rental_operation(branch_id,device_ref,generation,action,request_key,actor_id,amount,status)
      VALUES(branch,device,generation_,action_,key_,actor,amount_,'dispatching');
    RETURN true;
END $$;

CREATE FUNCTION ledger.record_rental_action(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,
    action_ text,key_ uuid,status_ text,evidence_ jsonb)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'rental_manage');
    IF status_ NOT IN ('submitted','unknown','acknowledged') THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid rental execution status';
    END IF;
    UPDATE ledger.rental_operation SET status=status_,evidence=evidence_
      WHERE branch_id=branch AND device_ref=device AND generation=generation_ AND action=action_
        AND request_key=key_ AND actor_id=actor AND status='dispatching';
    RETURN FOUND;
END $$;
REVOKE ALL ON FUNCTION ledger.rental_member(uuid,uuid,uuid,uuid,bigint),
    ledger.begin_rental_action(uuid,uuid,uuid,uuid,bigint,text,uuid,numeric),
    ledger.record_rental_action(uuid,uuid,uuid,uuid,bigint,text,uuid,text,jsonb) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.rental_member(uuid,uuid,uuid,uuid,bigint),
    ledger.begin_rental_action(uuid,uuid,uuid,uuid,bigint,text,uuid,numeric),
    ledger.record_rental_action(uuid,uuid,uuid,uuid,bigint,text,uuid,text,jsonb) TO nu54_p05_commerce;

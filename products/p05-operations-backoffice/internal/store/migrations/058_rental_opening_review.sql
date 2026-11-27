-- The enrolled station supplies this timestamp only for recorded verification.
-- Existing enrollment rows retain their original expiry and no invented time.
ALTER TABLE ledger.rental_enrollment ADD COLUMN attested_at timestamptz
    CHECK (attested_at IS NULL OR (isfinite(attested_at) AND attested_at<attested_until));

CREATE OR REPLACE FUNCTION ledger.rental_member(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE subject ledger.rental_enrollment;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'rental_manage');
    SELECT * INTO subject FROM ledger.rental_enrollment e WHERE e.branch_id=branch AND e.device_ref=device
      AND e.generation=generation_ FOR SHARE;
    IF NOT FOUND OR subject.attested_until<=clock_timestamp() OR subject.attested_at>clock_timestamp() THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='enrolled rental device required';
    END IF;
    PERFORM ledger.require_finance(tenant,branch,actor,'rental_manage');
    RETURN jsonb_build_object('account',subject.account_ref,'destination',subject.withdraw_ref,'revision',subject.revision,
       'generation',subject.generation,'hold',coalesce(subject.no_pending_until<=clock_timestamp(),true),
       'attestedAt',subject.attested_at,'attestedUntil',subject.attested_until,
       'reviewRef',encode(sha256(convert_to(jsonb_build_array(subject.branch_id,subject.device_ref,subject.generation,
            encode(subject.enrollment_digest,'hex'),subject.account_ref,subject.withdraw_ref,subject.revision,
            extract(epoch FROM subject.attested_at),extract(epoch FROM subject.attested_until))::text,'UTF8')),'hex'),
       'actions',coalesce((SELECT jsonb_agg(jsonb_build_object('action',o.action,'requestKey',o.request_key,
            'status',o.status,'amount',o.amount::text,'evidence',o.evidence) ORDER BY o.started_at)
            FROM ledger.rental_operation o WHERE o.branch_id=branch AND o.device_ref=device AND o.generation=generation_),
           '[]'::jsonb));
END $$;

CREATE FUNCTION ledger.begin_rental_action(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,
    action_ text,key_ uuid,amount_ numeric,review_ text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE subject jsonb;
DECLARE prior ledger.rental_operation;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'rental_manage');
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||device::text||generation_::text,25));
    subject:=ledger.rental_member(tenant,branch,actor,device,generation_);
    SELECT * INTO prior FROM ledger.rental_operation o WHERE o.branch_id=branch AND o.device_ref=device
      AND o.generation=generation_ AND o.action=action_;
    IF FOUND THEN
        IF (prior.request_key,prior.actor_id,prior.amount) IS DISTINCT FROM (key_,actor,amount_) THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='same enrolled action already reserved';
        END IF;
        RETURN false;
    END IF;
    IF review_ IS NOT NULL AND (action_ IS DISTINCT FROM 'open' OR review_ IS DISTINCT FROM subject->>'reviewRef') THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='rental opening evidence changed';
    END IF;
    IF action_ IS DISTINCT FROM 'open' AND (subject->>'hold')::boolean THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='v1 pending payment evidence held';
    END IF;
    INSERT INTO ledger.rental_operation(branch_id,device_ref,generation,action,request_key,actor_id,amount,status)
      VALUES(branch,device,generation_,action_,key_,actor,amount_,'dispatching');
    RETURN true;
END $$;

REVOKE ALL ON FUNCTION ledger.begin_rental_action(uuid,uuid,uuid,uuid,bigint,text,uuid,numeric,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.begin_rental_action(uuid,uuid,uuid,uuid,bigint,text,uuid,numeric,text) TO nu54_p05_commerce;

CREATE OR REPLACE FUNCTION ledger.begin_rental_action(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,
    action_ text,key_ uuid,amount_ numeric)
RETURNS boolean LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT ledger.begin_rental_action(tenant,branch,actor,device,generation_,action_,key_,amount_,NULL)
$$;

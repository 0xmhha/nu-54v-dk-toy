-- Read the registered reconciliation validity separately from the opening digest.
-- An expiry records the validity of the station's clearance, independent of any count.
-- Keep acknowledgement of an already reserved operation before the new-close checks.
CREATE FUNCTION ledger.rental_require_live_grant(tenant uuid,branch uuid,actor uuid)
RETURNS timestamptz LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $$
DECLARE expiry timestamptz;
BEGIN
    SELECT g.valid_until INTO expiry FROM ledger.finance_grant g JOIN ledger.branch b ON b.id=g.branch_id
      WHERE b.tenant_ref=tenant AND b.id=branch AND g.actor_id=actor
        AND g.capability='rental_manage' AND g.valid_until>clock_timestamp()
      FOR SHARE OF g;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='rental authority expired or revoked';
    END IF;
    RETURN expiry;
END $$;
REVOKE ALL ON FUNCTION ledger.rental_require_live_grant(uuid,uuid,uuid) FROM PUBLIC;

CREATE OR REPLACE FUNCTION ledger.rental_member(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE subject ledger.rental_enrollment;
DECLARE latest bigint;
DECLARE cleared boolean;
DECLARE opening_review text;
DECLARE authority_expiry timestamptz;
BEGIN
    PERFORM ledger.rental_require_live_grant(tenant,branch,actor);
    SELECT * INTO subject FROM ledger.rental_enrollment e WHERE e.branch_id=branch AND e.device_ref=device
      AND e.generation=generation_ FOR SHARE;
    IF NOT FOUND OR subject.attested_until<=clock_timestamp() OR subject.attested_at>clock_timestamp() THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='enrolled rental device required';
    END IF;
    authority_expiry:=ledger.rental_require_live_grant(tenant,branch,actor);
    SELECT max(e.generation) INTO latest FROM ledger.rental_enrollment e WHERE e.branch_id=branch AND e.device_ref=device;
    cleared:=coalesce(subject.no_pending_until>clock_timestamp(),false);
    opening_review:=encode(sha256(convert_to(jsonb_build_array(subject.branch_id,subject.device_ref,subject.generation,
            encode(subject.enrollment_digest,'hex'),subject.account_ref,subject.withdraw_ref,subject.revision,
            extract(epoch FROM subject.attested_at),extract(epoch FROM subject.attested_until))::text,'UTF8')),'hex');
    RETURN jsonb_build_object('account',subject.account_ref,'destination',subject.withdraw_ref,'revision',subject.revision,
       'generation',subject.generation,'hold',NOT cleared,
       'attestedAt',subject.attested_at,'attestedUntil',subject.attested_until,'reviewRef',opening_review,
       'pendingEvidenceUntil',subject.no_pending_until,
       'returnReviewRef', CASE WHEN latest=subject.generation AND cleared THEN
         encode(sha256(convert_to(jsonb_build_array('rental-return-review-v1',tenant,actor,opening_review,
            extract(epoch FROM authority_expiry),extract(epoch FROM subject.no_pending_until))::text,'UTF8')),'hex') END,
       'actions',coalesce((SELECT jsonb_agg(jsonb_build_object('action',o.action,'requestKey',o.request_key,
            'status',o.status,'amount',o.amount::text,'evidence',o.evidence) ORDER BY o.started_at)
            FROM ledger.rental_operation o WHERE o.branch_id=branch AND o.device_ref=device AND o.generation=generation_),
           '[]'::jsonb));
END $$;

CREATE OR REPLACE FUNCTION ledger.begin_rental_action(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,
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
    IF action_='close' AND (review_ IS NULL OR review_ IS DISTINCT FROM subject->>'returnReviewRef') THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='rental return review required or changed';
    END IF;
    IF action_<>'close' AND review_ IS NOT NULL AND (action_<>'open' OR review_ IS DISTINCT FROM subject->>'reviewRef') THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='rental opening evidence changed';
    END IF;
    IF action_ IS DISTINCT FROM 'open' AND (subject->>'hold')::boolean THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='v1 pending payment evidence held';
    END IF;
    INSERT INTO ledger.rental_operation(branch_id,device_ref,generation,action,request_key,actor_id,amount,status)
      VALUES(branch,device,generation_,action_,key_,actor,amount_,'dispatching');
    RETURN true;
END $$;


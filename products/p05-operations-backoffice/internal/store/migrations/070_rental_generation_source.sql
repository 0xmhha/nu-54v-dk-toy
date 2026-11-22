-- Provisioning the authority requires the protected P01 enrollment service.
-- This revision gives neither operators nor evidence writers that capability.
DO $role$
BEGIN
    IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='nu54_p05_rental_evidence') THEN
        CREATE ROLE nu54_p05_rental_evidence NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
    END IF;
END
$role$;
GRANT USAGE ON SCHEMA ledger TO nu54_p05_rental_evidence;

ALTER TABLE ledger.rental_enrollment ADD COLUMN generation_evidence_required boolean NOT NULL DEFAULT false;

CREATE TABLE ledger.rental_generation_source (
    branch_id uuid NOT NULL,
    device_ref uuid NOT NULL,
    generation bigint NOT NULL,
    enrollment_digest bytea NOT NULL CHECK(octet_length(enrollment_digest)=32),
    account_ref text NOT NULL CHECK(account_ref~'^0x[0-9a-f]{40}$'),
    withdraw_ref text NOT NULL CHECK(withdraw_ref~'^0x[0-9a-f]{40}$'),
    enrollment_revision text NOT NULL CHECK(enrollment_revision~'^[a-z0-9_-]{1,32}$'),
    board_measurement bytea NOT NULL CHECK(octet_length(board_measurement)=32),
    board_signer_ref text NOT NULL CHECK(board_signer_ref~'^0x[0-9a-f]{40}$' AND board_signer_ref<>account_ref),
    settlement_ref text NOT NULL CHECK(settlement_ref~'^0x[0-9a-f]{40}$'),
    deployment_digest bytea NOT NULL CHECK(octet_length(deployment_digest)=32),
    stream_ref uuid NOT NULL,
    stream_generation uuid NOT NULL,
    challenge bytea NOT NULL UNIQUE CHECK(octet_length(challenge)=32),
    challenge_until timestamptz NOT NULL CHECK(isfinite(challenge_until)),
    writer_login name NOT NULL,
    valid_until timestamptz NOT NULL CHECK(isfinite(valid_until) AND valid_until>=challenge_until),
    PRIMARY KEY(branch_id,device_ref,generation),
    FOREIGN KEY(branch_id,device_ref,generation) REFERENCES ledger.rental_enrollment
);

CREATE TABLE ledger.rental_generation_authorized_proof (
    branch_id uuid NOT NULL,
    device_ref uuid NOT NULL,
    generation bigint NOT NULL,
    request_key uuid NOT NULL,
    request_digest bytea NOT NULL CHECK(octet_length(request_digest)=32),
    source_digest bytea NOT NULL CHECK(octet_length(source_digest)=32),
    enrollment_digest bytea NOT NULL CHECK(octet_length(enrollment_digest)=32),
    challenge bytea NOT NULL UNIQUE CHECK(octet_length(challenge)=32),
    freeze_digest bytea NOT NULL UNIQUE CHECK(octet_length(freeze_digest)=32),
    journal_root bytea NOT NULL CHECK(octet_length(journal_root)=32),
    first_nonce bytea NOT NULL CHECK(octet_length(first_nonce)=32),
    next_nonce bytea NOT NULL CHECK(octet_length(next_nonce)=32 AND next_nonce>=first_nonce),
    payment_entries integer NOT NULL CHECK(payment_entries BETWEEN 0 AND 8),
    limit_entries integer NOT NULL CHECK(limit_entries BETWEEN 0 AND 8),
    burned_entries integer NOT NULL CHECK(burned_entries BETWEEN 0 AND 8),
    stream_ref uuid NOT NULL,
    stream_generation uuid NOT NULL,
    previous_height bigint CHECK(previous_height>0),
    previous_hash bytea CHECK(octet_length(previous_hash)=32),
    CHECK((previous_height IS NULL)=(previous_hash IS NULL)),
    finalized_height bigint NOT NULL CHECK(finalized_height>0),
    finalized_hash bytea NOT NULL CHECK(octet_length(finalized_hash)=32),
    assessment_digest bytea NOT NULL UNIQUE CHECK(octet_length(assessment_digest)=32),
    frozen_at timestamptz NOT NULL CHECK(isfinite(frozen_at)),
    finalized_at timestamptz NOT NULL CHECK(isfinite(finalized_at) AND finalized_at>=frozen_at),
    recorded_at timestamptz NOT NULL CHECK(isfinite(recorded_at) AND recorded_at>=finalized_at),
    fresh_until timestamptz NOT NULL CHECK(isfinite(fresh_until) AND fresh_until>recorded_at),
    pending_payments integer NOT NULL CHECK(pending_payments BETWEEN 0 AND 8),
    pending_limits integer NOT NULL CHECK(pending_limits BETWEEN 0 AND 8),
    CHECK(pending_payments+pending_limits<=8),
    PRIMARY KEY(branch_id,device_ref,generation,request_key),
    FOREIGN KEY(branch_id,device_ref,generation) REFERENCES ledger.rental_generation_source
);

REVOKE ALL ON ledger.rental_generation_source,ledger.rental_generation_authorized_proof
    FROM PUBLIC,nu54_p05_commerce,nu54_p05_rental_evidence;

CREATE FUNCTION ledger.lock_rental_evidence_device(branch uuid, device uuid)
RETURNS void LANGUAGE sql SET search_path=pg_catalog,pg_temp AS $$
    SELECT pg_advisory_xact_lock(hashtextextended(branch::text||device::text,26))
$$;
REVOKE ALL ON FUNCTION ledger.lock_rental_evidence_device(uuid,uuid) FROM PUBLIC;

CREATE FUNCTION ledger.lock_rental_enrollment_insert()
RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.lock_rental_evidence_device(NEW.branch_id,NEW.device_ref);
    RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION ledger.lock_rental_enrollment_insert() FROM PUBLIC;
CREATE TRIGGER rental_generation_registration_fence BEFORE INSERT ON ledger.rental_enrollment
    FOR EACH ROW EXECUTE FUNCTION ledger.lock_rental_enrollment_insert();

CREATE FUNCTION ledger.require_rental_generation_evidence()
RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.lock_rental_evidence_device(NEW.branch_id,NEW.device_ref);
    UPDATE ledger.rental_enrollment e SET generation_evidence_required=true
        WHERE (e.branch_id,e.device_ref,e.generation)=(NEW.branch_id,NEW.device_ref,NEW.generation)
          AND e.attested_at IS NOT NULL;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='qualified rental enrollment required'; END IF;
    RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION ledger.require_rental_generation_evidence() FROM PUBLIC;
CREATE TRIGGER rental_protected_mode BEFORE INSERT ON ledger.rental_generation_source
    FOR EACH ROW EXECUTE FUNCTION ledger.require_rental_generation_evidence();

CREATE FUNCTION ledger.reject_rental_evidence_downgrade()
RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF OLD.generation_evidence_required AND NOT NEW.generation_evidence_required THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='rental generation evidence mode cannot be downgraded';
    END IF;
    RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION ledger.reject_rental_evidence_downgrade() FROM PUBLIC;
CREATE TRIGGER rental_protected_mode_immutable BEFORE UPDATE OF generation_evidence_required ON ledger.rental_enrollment
    FOR EACH ROW EXECUTE FUNCTION ledger.reject_rental_evidence_downgrade();

CREATE FUNCTION ledger.rental_generation_source_digest(s ledger.rental_generation_source)
RETURNS bytea LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT sha256(convert_to(jsonb_build_array(s.branch_id,s.device_ref,s.generation,s.enrollment_digest,
          s.account_ref,s.withdraw_ref,s.enrollment_revision,s.board_measurement,s.board_signer_ref,
          s.settlement_ref,s.deployment_digest,s.stream_ref,s.stream_generation,s.challenge,s.writer_login,
          extract(epoch FROM s.challenge_until),extract(epoch FROM s.valid_until))::text,'UTF8'))
$$;
REVOKE ALL ON FUNCTION ledger.rental_generation_source_digest(ledger.rental_generation_source) FROM PUBLIC;


-- This procedure accepts commitments from the separately authenticated
-- collector only. The HTTP adapter verifies P01 and the pinned P07 response
-- before calling it. Incomplete/unknown assessments never enter this table.
CREATE FUNCTION ledger.record_rental_generation_proof(branch uuid, device uuid, generation_ bigint,
    request_ uuid, registration_ bytea, measurement_ bytea, signer_ text, settlement_ text, deployment_ bytea,
    stream_ uuid, stream_generation_ uuid, challenge_ bytea, freeze_ bytea, root_ bytea,
    assessment_ bytea, first_ bytea, next_ bytea, payment_ integer, limit_ integer, burned_ integer,
    pending_payment_ integer, pending_limit_ integer, height_ bigint, hash_ bytea,
    frozen_ timestamptz, freeze_until_ timestamptz, finalized_ timestamptz, fresh_ timestamptz,
    previous_height_ bigint, previous_hash_ bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE scope ledger.rental_generation_source;
DECLARE enrollment ledger.rental_enrollment;
DECLARE prior bytea;
DECLARE previous ledger.rental_generation_authorized_proof;
DECLARE request_digest_ bytea;
DECLARE checked_at timestamptz;
BEGIN
    PERFORM ledger.lock_rental_evidence_device(branch,device);
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||device::text||generation_::text,25));
    SELECT * INTO scope FROM ledger.rental_generation_source s
      WHERE (s.branch_id,s.device_ref,s.generation)=(branch,device,generation_) FOR SHARE;
    IF NOT FOUND OR scope.writer_login<>session_user THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='registered rental evidence authority required';
    END IF;
    request_digest_:=sha256(convert_to(jsonb_build_array(
      'P05-rental-generation-proof-v1',branch,device,generation_,request_,registration_,measurement_,signer_,settlement_,
      deployment_,stream_,stream_generation_,challenge_,freeze_,root_,assessment_,first_,next_,payment_,limit_,burned_,
      pending_payment_,pending_limit_,height_,hash_,extract(epoch FROM frozen_),extract(epoch FROM freeze_until_),
      extract(epoch FROM finalized_),extract(epoch FROM fresh_),previous_height_,previous_hash_)::text,'UTF8'));
    SELECT p.request_digest INTO prior FROM ledger.rental_generation_authorized_proof p
      WHERE (p.branch_id,p.device_ref,p.generation,p.request_key)=(branch,device,generation_,request_);
    IF FOUND THEN
        IF prior<>request_digest_ THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='rental evidence request binding changed';
        END IF;
        RETURN false;
    END IF;
    SELECT * INTO enrollment FROM ledger.rental_enrollment e
      WHERE (e.branch_id,e.device_ref,e.generation)=(branch,device,generation_) FOR SHARE;
    checked_at:=clock_timestamp();
    IF NOT FOUND OR EXISTS(SELECT FROM ledger.rental_enrollment e WHERE
         (e.branch_id,e.device_ref)=(branch,device) AND e.generation>generation_)
       OR (scope.enrollment_digest,scope.account_ref,scope.withdraw_ref,scope.enrollment_revision) IS DISTINCT FROM
          (enrollment.enrollment_digest,enrollment.account_ref,enrollment.withdraw_ref,enrollment.revision)
       OR (scope.enrollment_digest,scope.board_measurement,scope.board_signer_ref,scope.settlement_ref,scope.deployment_digest,
           scope.stream_ref,scope.stream_generation,scope.challenge) IS DISTINCT FROM
          (registration_,measurement_,signer_,settlement_,deployment_,stream_,stream_generation_,challenge_)
       OR scope.valid_until<=checked_at OR scope.challenge_until<=checked_at OR enrollment.attested_until<=checked_at
       OR enrollment.attested_at IS NULL OR enrollment.attested_at>checked_at
       OR frozen_ IS NULL OR frozen_<enrollment.attested_at OR frozen_>checked_at
       OR freeze_until_ IS NULL OR freeze_until_>scope.challenge_until OR freeze_until_>checked_at+interval '120 seconds'
       OR fresh_ IS NULL OR fresh_<=checked_at OR fresh_>freeze_until_
       OR finalized_ IS NULL OR finalized_<frozen_ OR finalized_>checked_at
       OR payment_ IS NULL OR limit_ IS NULL OR burned_ IS NULL
       OR payment_ NOT BETWEEN 0 AND 8 OR limit_ NOT BETWEEN 0 AND 8 OR burned_ NOT BETWEEN 0 AND 8
       OR payment_+limit_+burned_>8 OR first_ IS NULL OR next_ IS NULL OR octet_length(first_)<>32 OR octet_length(next_)<>32
       OR get_byte(first_,31)<>0 OR ledger.rental_nonce_number(next_)-ledger.rental_nonce_number(first_)<>payment_+limit_+burned_
       OR pending_payment_ IS NULL OR pending_limit_ IS NULL
       OR pending_payment_ NOT BETWEEN 0 AND payment_ OR pending_limit_ NOT BETWEEN 0 AND limit_ THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='current rental generation evidence did not match';
    END IF;
    SELECT * INTO previous FROM ledger.rental_generation_authorized_proof p
      WHERE (p.branch_id,p.device_ref,p.generation)=(branch,device,generation_)
      ORDER BY p.recorded_at,p.request_key LIMIT 1;
    IF FOUND AND (previous.journal_root,previous.first_nonce,previous.next_nonce,
        previous.payment_entries,previous.limit_entries,previous.burned_entries,previous.frozen_at) IS DISTINCT FROM
        (root_,first_,next_,payment_,limit_,burned_,frozen_) THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='frozen rental journal changed';
    END IF;
    SELECT * INTO previous FROM ledger.rental_generation_authorized_proof p
      WHERE (p.branch_id,p.device_ref,p.generation,p.stream_ref,p.stream_generation)=
        (branch,device,generation_,stream_,stream_generation_)
      ORDER BY p.finalized_height DESC,p.recorded_at DESC,p.request_key DESC LIMIT 1;
    IF (FOUND AND (previous_height_,previous_hash_) IS DISTINCT FROM (previous.finalized_height,previous.finalized_hash))
        OR (NOT FOUND AND (previous_height_ IS NOT NULL OR previous_hash_ IS NOT NULL)) THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='rental previous finalized anchor was not verified';
    END IF;
    IF FOUND AND (height_<previous.finalized_height OR finalized_<previous.finalized_at
        OR pending_payment_>previous.pending_payments OR pending_limit_>previous.pending_limits
        OR (height_=previous.finalized_height AND (hash_,finalized_,pending_payment_,pending_limit_) IS DISTINCT FROM
            (previous.finalized_hash,previous.finalized_at,previous.pending_payments,previous.pending_limits))) THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='rental finality or pending count regressed';
    END IF;
    INSERT INTO ledger.rental_generation_authorized_proof VALUES(
        branch,device,generation_,request_,request_digest_,ledger.rental_generation_source_digest(scope),registration_,challenge_,freeze_,root_,
        first_,next_,payment_,limit_,burned_,stream_,stream_generation_,previous_height_,previous_hash_,height_,hash_,
        assessment_,frozen_,finalized_,checked_at,fresh_,pending_payment_,pending_limit_);
    RETURN true;
EXCEPTION WHEN unique_violation THEN
    RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='rental evidence challenge or appraisal already used';
END $$;
REVOKE ALL ON FUNCTION ledger.record_rental_generation_proof(uuid,uuid,bigint,uuid,bytea,bytea,text,text,bytea,
    uuid,uuid,bytea,bytea,bytea,bytea,bytea,bytea,integer,integer,integer,integer,integer,bigint,bytea,
    timestamptz,timestamptz,timestamptz,timestamptz,bigint,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.record_rental_generation_proof(uuid,uuid,bigint,uuid,bytea,bytea,text,text,bytea,
    uuid,uuid,bytea,bytea,bytea,bytea,bytea,bytea,integer,integer,integer,integer,integer,bigint,bytea,
    timestamptz,timestamptz,timestamptz,timestamptz,bigint,bytea) TO nu54_p05_rental_evidence;

CREATE FUNCTION ledger.rental_generation_authorized_review(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE authority_expiry timestamptz;
DECLARE scope ledger.rental_generation_source;
DECLARE enrolled ledger.rental_enrollment;
DECLARE proof ledger.rental_generation_authorized_proof;
DECLARE observed_at timestamptz;
DECLARE pending jsonb:=jsonb_build_object('state','unknown');
DECLARE review text;
BEGIN
    authority_expiry:=ledger.rental_require_live_grant(tenant,branch,actor);
    SELECT * INTO scope FROM ledger.rental_generation_source s
      WHERE (s.branch_id,s.device_ref,s.generation)=(branch,device,generation_) FOR SHARE;
    IF NOT FOUND THEN
        IF EXISTS (SELECT FROM ledger.rental_enrollment e
            WHERE (e.branch_id,e.device_ref,e.generation)=(branch,device,generation_) AND e.generation_evidence_required) THEN
            RETURN jsonb_build_object('returnReviewRef',NULL,'hold',true,'pendingEvidenceUntil',NULL,'generationPending',pending);
        END IF;
        RETURN '{}'::jsonb;
    END IF;
    SELECT * INTO enrolled FROM ledger.rental_enrollment e
      WHERE (e.branch_id,e.device_ref,e.generation)=(branch,device,generation_) FOR SHARE;
    observed_at:=clock_timestamp();
    IF FOUND AND enrolled.attested_at<=observed_at AND enrolled.attested_until>observed_at
       AND scope.valid_until>observed_at AND scope.challenge_until>observed_at
       AND (scope.enrollment_digest,scope.account_ref,scope.withdraw_ref,scope.enrollment_revision)=
          (enrolled.enrollment_digest,enrolled.account_ref,enrolled.withdraw_ref,enrolled.revision)
       AND NOT EXISTS (SELECT FROM ledger.rental_enrollment e WHERE (e.branch_id,e.device_ref)=(branch,device)
                       AND e.generation>generation_) THEN
        SELECT * INTO proof FROM ledger.rental_generation_authorized_proof p
          WHERE (p.branch_id,p.device_ref,p.generation)=(branch,device,generation_)
            AND p.source_digest=ledger.rental_generation_source_digest(scope)
            AND p.frozen_at>=enrolled.attested_at AND p.recorded_at<=observed_at AND p.fresh_until>observed_at
          ORDER BY p.recorded_at DESC LIMIT 1;
        IF FOUND THEN
            pending:=jsonb_build_object('state',CASE WHEN proof.pending_payments+proof.pending_limits=0 THEN 'verified' ELSE 'pending' END,
              'pendingCount',proof.pending_payments+proof.pending_limits,
              'pendingPayments',proof.pending_payments,'pendingLimits',proof.pending_limits,'validUntil',proof.fresh_until);
            IF proof.pending_payments+proof.pending_limits=0 THEN
                review:=encode(sha256(convert_to(jsonb_build_array('rental-return-verified-v1',tenant,actor,
                   proof.source_digest,proof.assessment_digest,proof.finalized_hash,proof.stream_generation,
                   extract(epoch FROM proof.fresh_until),extract(epoch FROM authority_expiry))::text,'UTF8')),'hex');
            END IF;
        END IF;
    END IF;
    RETURN jsonb_build_object('returnReviewRef',review,'hold',review IS NULL,
        'pendingEvidenceUntil',CASE WHEN review IS NOT NULL THEN proof.fresh_until END,'generationPending',pending);
END $$;
REVOKE ALL ON FUNCTION ledger.rental_generation_authorized_review(uuid,uuid,uuid,uuid,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.rental_generation_authorized_review(uuid,uuid,uuid,uuid,bigint) TO nu54_p05_commerce;

ALTER FUNCTION ledger.begin_rental_action(uuid,uuid,uuid,uuid,bigint,text,uuid,numeric,text)
    RENAME TO begin_rental_action_legacy;
REVOKE EXECUTE ON FUNCTION ledger.begin_rental_action_legacy(uuid,uuid,uuid,uuid,bigint,text,uuid,numeric,text) FROM nu54_p05_commerce;

CREATE FUNCTION ledger.begin_rental_action(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,
    action_ text,key_ uuid,amount_ numeric,review_ text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE previous ledger.rental_operation;
DECLARE qualified jsonb;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'rental_manage');
    PERFORM ledger.lock_rental_evidence_device(branch,device);
    IF action_ IS DISTINCT FROM 'close' OR NOT EXISTS (SELECT FROM ledger.rental_enrollment e
        WHERE (e.branch_id,e.device_ref,e.generation)=(branch,device,generation_) AND e.generation_evidence_required) THEN
        RETURN ledger.begin_rental_action_legacy(tenant,branch,actor,device,generation_,action_,key_,amount_,review_);
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||device::text||generation_::text,25));
    -- Retain permission and enrollment checks from the existing reservation path.
    PERFORM ledger.rental_member(tenant,branch,actor,device,generation_);
    SELECT * INTO previous FROM ledger.rental_operation o WHERE (o.branch_id,o.device_ref,o.generation,o.action)=
        (branch,device,generation_,action_);
    IF FOUND THEN
        IF (previous.request_key,previous.actor_id,previous.amount) IS DISTINCT FROM (key_,actor,amount_) THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='same enrolled action already reserved';
        END IF;
        RETURN false;
    END IF;
    qualified:=ledger.rental_generation_authorized_review(tenant,branch,actor,device,generation_);
    IF (qualified->>'hold')::boolean IS DISTINCT FROM false OR review_ IS NULL OR
       review_ IS DISTINCT FROM qualified->>'returnReviewRef' THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='current verified generation return review required';
    END IF;
    INSERT INTO ledger.rental_operation(branch_id,device_ref,generation,action,request_key,actor_id,amount,status)
      VALUES(branch,device,generation_,action_,key_,actor,amount_,'dispatching');
    RETURN true;
END $$;
REVOKE ALL ON FUNCTION ledger.begin_rental_action(uuid,uuid,uuid,uuid,bigint,text,uuid,numeric,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.begin_rental_action(uuid,uuid,uuid,uuid,bigint,text,uuid,numeric,text) TO nu54_p05_commerce;

CREATE SCHEMA travel_private;
REVOKE ALL ON SCHEMA travel_private FROM PUBLIC;

CREATE TABLE travel_private.policy (
    purpose text NOT NULL CHECK (purpose IN ('location','voice','transcription','recommendation','review','challenge')),
    version integer NOT NULL CHECK (version > 0),
    recipient text NOT NULL CHECK (recipient ~ '^[a-z0-9][a-z0-9_-]{0,63}$'),
    evidence_ref uuid NOT NULL,
    max_lifetime_seconds integer NOT NULL CHECK (max_lifetime_seconds BETWEEN 1 AND 31536000),
    target_required boolean NOT NULL,
    active boolean NOT NULL,
    PRIMARY KEY(purpose,version,recipient),
    CHECK (max_lifetime_seconds <= CASE purpose WHEN 'location' THEN 2592000 WHEN 'voice' THEN 604800
        WHEN 'transcription' THEN 2592000 WHEN 'recommendation' THEN 1209600 ELSE 31536000 END)
);
CREATE UNIQUE INDEX policy_one_current ON travel_private.policy(purpose,recipient) WHERE active;

CREATE TABLE travel_private.consent_decision (
    subject_ref uuid NOT NULL,
    purpose text NOT NULL,
    revision bigint NOT NULL CHECK (revision > 0),
    policy_version integer NOT NULL,
    recipient text NOT NULL,
    selected_object uuid,
    accepted boolean NOT NULL,
    decided_at timestamptz NOT NULL DEFAULT statement_timestamp() CHECK (isfinite(decided_at)),
    expires_at timestamptz NOT NULL CHECK (isfinite(expires_at) AND expires_at>=decided_at),
    request_key uuid NOT NULL,
    receipt uuid NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    PRIMARY KEY (subject_ref,purpose,revision),
    UNIQUE (subject_ref,purpose,request_key),
    FOREIGN KEY(purpose,policy_version,recipient) REFERENCES travel_private.policy(purpose,version,recipient)
);
CREATE TABLE travel_private.consent (
    subject_ref uuid NOT NULL,
    purpose text NOT NULL,
    revision bigint NOT NULL,
    PRIMARY KEY(subject_ref,purpose),
    FOREIGN KEY(subject_ref,purpose,revision) REFERENCES travel_private.consent_decision
);

DO $roles$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p09_private_user') THEN
        CREATE ROLE nu54_p09_private_user NOLOGIN NOSUPERUSER NOCREATEROLE NOCREATEDB NOBYPASSRLS;
    END IF;
END
$roles$;

CREATE FUNCTION travel_private.decide(
    actor_ uuid, purpose_ text, expected_ bigint, policy_ integer, recipient_ text,
    object_ uuid, decision_ boolean, expiry_ timestamptz, request_ uuid
) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $decision$
DECLARE
    previous travel_private.consent_decision;
    policy travel_private.policy;
    actual_revision bigint;
    received_at timestamptz;
    result uuid;
BEGIN
    IF actor_ IS NULL OR purpose_ IS NULL OR expected_ IS NULL OR policy_ IS NULL OR decision_ IS NULL OR request_ IS NULL THEN
        RAISE EXCEPTION USING ERRCODE='22023', MESSAGE='invalid consent command';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(actor_::text||':'||purpose_, 900018));
    SELECT * INTO previous FROM travel_private.consent_decision
      WHERE subject_ref=actor_ AND purpose=purpose_ AND request_key=request_;
    IF FOUND THEN
        IF (previous.revision - 1,previous.policy_version,previous.recipient,previous.selected_object,previous.accepted,
            CASE WHEN previous.accepted THEN previous.expires_at ELSE NULL END)
           IS DISTINCT FROM (expected_,policy_,recipient_,object_,decision_,expiry_) THEN
            RAISE EXCEPTION USING ERRCODE='P1801', MESSAGE='consent conflict';
        END IF;
        RETURN previous.receipt;
    END IF;
    SELECT revision INTO actual_revision FROM travel_private.consent WHERE subject_ref=actor_ AND purpose=purpose_;
    IF coalesce(actual_revision,0)<>expected_ THEN
        RAISE EXCEPTION USING ERRCODE='P1801', MESSAGE='stale consent revision';
    END IF;
    received_at := clock_timestamp();
    IF decision_ THEN
        SELECT p.* INTO policy FROM travel_private.policy p
          WHERE p.purpose=purpose_ AND p.version=policy_ AND p.recipient=recipient_ AND p.active FOR SHARE;
        IF NOT FOUND OR policy.target_required <> (object_ IS NOT NULL)
            OR expiry_ IS NULL OR expiry_<=received_at
            OR expiry_>received_at+make_interval(secs=>policy.max_lifetime_seconds) THEN
            RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='consent scope unavailable';
        END IF;
    ELSE
        SELECT d.* INTO previous FROM travel_private.consent_decision d
          WHERE d.subject_ref=actor_ AND d.purpose=purpose_ AND d.revision=actual_revision;
        IF NOT FOUND OR NOT previous.accepted OR expiry_ IS NOT NULL OR
           (previous.policy_version,previous.recipient,previous.selected_object) IS DISTINCT FROM (policy_,recipient_,object_) THEN
            RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='consent scope unavailable';
        END IF;
    END IF;
    INSERT INTO travel_private.consent_decision(subject_ref,purpose,revision,policy_version,recipient,selected_object,
      accepted,decided_at,expires_at,request_key)
      VALUES(actor_,purpose_,expected_+1,policy_,recipient_,object_,decision_,received_at,coalesce(expiry_,received_at),request_)
      RETURNING receipt INTO result;
    INSERT INTO travel_private.consent VALUES(actor_,purpose_,expected_+1)
      ON CONFLICT(subject_ref,purpose) DO UPDATE SET revision=excluded.revision;
    RETURN result;
END
$decision$;

CREATE FUNCTION travel_private.current_decisions(actor_ uuid)
RETURNS TABLE(purpose text, revision bigint, policy_version integer, recipient text, selected_object uuid,
    accepted boolean, decided_at timestamptz, expires_at timestamptz, receipt uuid)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $view$
    SELECT d.purpose,d.revision,d.policy_version,d.recipient,d.selected_object,d.accepted,d.decided_at,d.expires_at,d.receipt
    FROM travel_private.consent c JOIN travel_private.consent_decision d USING(subject_ref,purpose,revision)
    WHERE c.subject_ref=actor_ ORDER BY d.purpose
$view$;

CREATE FUNCTION travel_private.require_consent(actor_ uuid,purpose_ text,rev_ bigint,recipient_ text,object_ uuid)
RETURNS timestamptz
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $guard$
DECLARE expiry timestamptz;
BEGIN
    IF actor_ IS NULL OR purpose_ IS NULL THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='consent scope unavailable';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(actor_::text||':'||purpose_,900018));
    SELECT d.expires_at INTO expiry
      FROM travel_private.consent c JOIN travel_private.consent_decision d USING(subject_ref,purpose,revision)
      JOIN travel_private.policy p ON (p.purpose,p.version,p.recipient)=(d.purpose,d.policy_version,d.recipient)
      WHERE c.subject_ref=actor_ AND c.purpose=purpose_ AND c.revision=rev_
        AND d.recipient=recipient_ AND (d.selected_object IS NULL OR d.selected_object=object_)
        AND d.expires_at>clock_timestamp() AND d.accepted AND p.active FOR SHARE OF p;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='consent scope unavailable';
    END IF;
    RETURN expiry;
END
$guard$;

REVOKE ALL ON FUNCTION travel_private.decide(uuid,text,bigint,integer,text,uuid,boolean,timestamptz,uuid),
    travel_private.current_decisions(uuid),travel_private.require_consent(uuid,text,bigint,text,uuid) FROM PUBLIC;
GRANT USAGE ON SCHEMA travel_private TO nu54_p09_private_user;
GRANT EXECUTE ON FUNCTION travel_private.decide(uuid,text,bigint,integer,text,uuid,boolean,timestamptz,uuid),
    travel_private.current_decisions(uuid) TO nu54_p09_private_user;

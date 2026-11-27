CREATE TABLE travel_private.visit_evidence (
    issuer text NOT NULL,
    proof uuid NOT NULL,
    subject_ref uuid NOT NULL,
    place_id text NOT NULL,
    environment text NOT NULL CHECK (environment IN ('synthetic','live')),
    source_revision bigint NOT NULL CHECK (source_revision>0),
    state text NOT NULL CHECK (state IN ('settled','refunded','reorg')),
    observed_at timestamptz NOT NULL CHECK (isfinite(observed_at)),
    PRIMARY KEY (issuer,proof)
);
CREATE TABLE travel_private.visit_claim (
    issuer text NOT NULL,
    proof uuid NOT NULL,
    purpose text NOT NULL CHECK (purpose IN ('review','challenge')),
    consent_revision bigint NOT NULL,
    claim_ref uuid NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    applied_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    PRIMARY KEY (issuer,proof,purpose),
    FOREIGN KEY (issuer,proof) REFERENCES travel_private.visit_evidence
);
DO $roles$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p09_visit_issuer') THEN
    CREATE ROLE nu54_p09_visit_issuer NOLOGIN NOSUPERUSER NOCREATEROLE NOCREATEDB NOBYPASSRLS;
  END IF;
END
$roles$;

CREATE FUNCTION travel_private.put_visit(
    issuer_ text,proof_ uuid,actor_ uuid,place_ text,environment_ text,version_ bigint,state_ text,observed_ timestamptz
) RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $visit$
DECLARE prior travel_private.visit_evidence;
BEGIN
  IF issuer_ IS NULL OR proof_ IS NULL OR actor_ IS NULL OR place_ IS NULL
    OR version_ IS NULL OR observed_ IS NULL OR observed_>clock_timestamp()
    OR version_<=0 OR state_ NOT IN ('settled','refunded','reorg') THEN
      RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='visit proof unavailable';
  END IF;
  PERFORM pg_advisory_xact_lock(hashtextextended(issuer_||':'||proof_::text,900019));
  SELECT * INTO prior FROM travel_private.visit_evidence WHERE issuer=issuer_ AND proof=proof_;
  IF FOUND THEN
    IF (prior.subject_ref,prior.place_id,prior.environment) IS DISTINCT FROM (actor_,place_,environment_) OR
       prior.source_revision>version_ OR prior.observed_at>observed_ THEN
      RAISE EXCEPTION USING ERRCODE='P1801', MESSAGE='visit version conflict';
    END IF;
    IF prior.source_revision=version_ THEN
      IF (prior.state,prior.observed_at) IS DISTINCT FROM (state_,observed_) THEN
         RAISE EXCEPTION USING ERRCODE='P1801', MESSAGE='visit version conflict';
      END IF;
      RETURN false;
    END IF;
  END IF;
  INSERT INTO travel_private.visit_evidence VALUES(issuer_,proof_,actor_,place_,environment_,version_,state_,observed_)
    ON CONFLICT(issuer,proof) DO UPDATE SET source_revision=excluded.source_revision,
    state=excluded.state,observed_at=excluded.observed_at;
  RETURN true;
END
$visit$;

CREATE FUNCTION travel_private.claim_visit(
    actor_ uuid,issuer_ text,proof_ uuid,purpose_ text,revision_ bigint,place_ text
) RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $claim$
DECLARE
    prior travel_private.visit_evidence;
    out_ uuid;
BEGIN
  IF purpose_ NOT IN ('review','challenge') THEN
      RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='visit scope unavailable';
  END IF;
  PERFORM travel_private.require_consent(actor_,purpose_,revision_,'p09',proof_);
  SELECT * INTO prior FROM travel_private.visit_evidence WHERE issuer=issuer_ AND proof=proof_ FOR SHARE;
  IF NOT FOUND OR (prior.subject_ref,prior.place_id,prior.state) IS DISTINCT FROM (actor_,place_,'settled') THEN
      RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='visit scope unavailable';
  END IF;
  INSERT INTO travel_private.visit_claim(issuer,proof,purpose,consent_revision)
    VALUES(issuer_,proof_,purpose_,revision_) ON CONFLICT(issuer,proof,purpose) DO NOTHING
    RETURNING claim_ref INTO out_;
  IF out_ IS NULL THEN
    SELECT claim_ref INTO out_ FROM travel_private.visit_claim WHERE issuer=issuer_ AND proof=proof_ AND purpose=purpose_;
  END IF;
  RETURN out_;
END
$claim$;

CREATE FUNCTION travel_private.my_visits(actor_ uuid)
RETURNS TABLE(issuer text,proof uuid,place_id text,purpose text,claim uuid,classification text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $scope$
 SELECT e.issuer,e.proof,e.place_id,c.purpose,c.claim_ref,
   CASE WHEN e.state<>'settled' THEN 'adjusted'
        WHEN e.environment='synthetic' THEN 'test-only'
        WHEN decision.revision IS NULL OR NOT decision.accepted OR decision.expires_at<=statement_timestamp() OR NOT policy.active
          THEN 'permission-held'
        ELSE 'verified' END
 FROM travel_private.visit_evidence e
 JOIN travel_private.visit_claim c USING(issuer,proof)
 LEFT JOIN travel_private.consent current ON (current.subject_ref,current.purpose,current.revision)=(e.subject_ref,c.purpose,c.consent_revision)
 LEFT JOIN travel_private.consent_decision decision ON (decision.subject_ref,decision.purpose,decision.revision)=(current.subject_ref,current.purpose,current.revision)
 LEFT JOIN travel_private.policy policy ON (policy.purpose,policy.version,policy.recipient)=(decision.purpose,decision.policy_version,decision.recipient)
 WHERE e.subject_ref=actor_ ORDER BY c.applied_at DESC LIMIT 100
$scope$;

REVOKE ALL ON FUNCTION travel_private.put_visit(text,uuid,uuid,text,text,bigint,text,timestamptz),
    travel_private.claim_visit(uuid,text,uuid,text,bigint,text),travel_private.my_visits(uuid) FROM PUBLIC;
GRANT USAGE ON SCHEMA travel_private TO nu54_p09_visit_issuer;
GRANT EXECUTE ON FUNCTION travel_private.put_visit(text,uuid,uuid,text,text,bigint,text,timestamptz) TO nu54_p09_visit_issuer;
GRANT EXECUTE ON FUNCTION travel_private.claim_visit(uuid,text,uuid,text,bigint,text),travel_private.my_visits(uuid) TO nu54_p09_private_user;

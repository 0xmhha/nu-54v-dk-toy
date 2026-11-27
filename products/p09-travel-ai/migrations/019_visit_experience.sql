ALTER TABLE travel_private.visit_evidence ADD COLUMN listed_at timestamptz;
UPDATE travel_private.visit_evidence SET listed_at=observed_at;
ALTER TABLE travel_private.visit_evidence ALTER COLUMN listed_at SET NOT NULL,
 ALTER COLUMN listed_at SET DEFAULT statement_timestamp();
CREATE INDEX visit_evidence_subject_listed ON travel_private.visit_evidence(subject_ref,listed_at DESC,issuer,proof);

CREATE TABLE travel_private.visit_erasure_fence (
  issuer text NOT NULL,
  proof uuid NOT NULL,
  purpose text NOT NULL CHECK (purpose IN ('review','challenge')),
  through_revision bigint NOT NULL CHECK (through_revision>0),
  PRIMARY KEY (issuer,proof,purpose),
  FOREIGN KEY (issuer,proof) REFERENCES travel_private.visit_evidence
);

CREATE FUNCTION travel_private.visit_options(actor_ uuid)
RETURNS TABLE(issuer text,proof uuid,place_id text,observed_at timestamptz,environment text,state text,
    challenge text,claim_id uuid,review_id uuid,review_state text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $options$
 WITH visits AS (
 SELECT e.issuer,e.proof,e.place_id,e.observed_at,e.environment,e.state,c.applied_at,c.claim_ref,
   CASE WHEN c.claim_ref IS NULL OR travel_private.fenced(actor_,'visit-claim',c.claim_ref,'challenge',c.consent_revision) THEN 'unsubmitted'
        WHEN e.state<>'settled' THEN 'adjusted'
        WHEN e.environment='synthetic' THEN 'test-only'
        WHEN d.revision IS NULL OR NOT d.accepted OR d.expires_at<=statement_timestamp()
          OR NOT coalesce(p.active,false)
          THEN 'permission-held'
        ELSE 'verified' END AS challenge, r.id AS review_id,
   CASE WHEN r.id IS NULL THEN NULL
        WHEN e.environment='synthetic' THEN 'test-only'
        WHEN NOT r.published THEN 'private'
        WHEN NOT EXISTS(SELECT FROM travel_private.consent rc
             JOIN travel_private.consent_decision rd USING(subject_ref,purpose,revision)
             JOIN travel_private.policy rp ON (rp.purpose,rp.version,rp.recipient)=(rd.purpose,rd.policy_version,rd.recipient)
             WHERE (rc.subject_ref,rc.purpose,rc.revision)=(actor_,'review',r.consent_revision)
               AND rd.accepted AND rp.active AND rd.expires_at>statement_timestamp())
          THEN 'private'
        WHEN e.state<>'settled' THEN 'adjusted'
        ELSE 'published' END AS review_state
 FROM travel_private.visit_evidence e
 LEFT JOIN travel_private.visit_claim c ON (c.issuer,c.proof,c.purpose)=(e.issuer,e.proof,'challenge')
 LEFT JOIN travel_private.consent consent ON (consent.subject_ref,consent.purpose,consent.revision)=(actor_,'challenge',c.consent_revision)
 LEFT JOIN travel_private.consent_decision d ON (d.subject_ref,d.purpose,d.revision)=(consent.subject_ref,consent.purpose,consent.revision)
 LEFT JOIN travel_private.policy p ON (p.purpose,p.version,p.recipient)=(d.purpose,d.policy_version,d.recipient)
 LEFT JOIN travel_private.review r ON (r.issuer,r.proof,r.subject_ref)=(e.issuer,e.proof,actor_) AND r.delete_after>statement_timestamp()
   AND NOT travel_private.fenced(actor_,'review',r.id,'review',r.consent_revision)
 WHERE e.subject_ref=actor_
 ), ranked AS (
  SELECT visits.*, count(*) FILTER (WHERE visits.challenge='verified') OVER (
    PARTITION BY visits.place_id ORDER BY visits.applied_at,visits.issuer,visits.proof ROWS UNBOUNDED PRECEDING) AS visit_order
  FROM visits
 )
 SELECT v.issuer,v.proof,v.place_id,v.observed_at,v.environment,v.state,
   CASE WHEN v.challenge='verified' AND v.visit_order>1 THEN 'duplicate' ELSE v.challenge END,CASE WHEN v.challenge<>'unsubmitted' THEN v.claim_ref END,v.review_id,v.review_state
 FROM ranked v ORDER BY v.observed_at DESC,v.issuer,v.proof
$options$;

CREATE FUNCTION travel_private.visit_option_page(actor_ uuid,place_ text,cursor_ text)
RETURNS TABLE(issuer text,proof uuid,place_id text,observed_at timestamptz,environment text,state text,
  challenge text,claim_id uuid,review_id uuid,review_state text,continuation text)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $page$
DECLARE anchor travel_private.visit_evidence;
BEGIN
  IF cursor_ IS NOT NULL THEN
    SELECT e.* INTO anchor FROM travel_private.visit_evidence e
      WHERE e.subject_ref=actor_ AND (place_ IS NULL OR e.place_id=place_)
        AND encode(sha256(convert_to(actor_::text||':'||coalesce(place_,'')||':'||e.issuer||':'||e.proof::text,'UTF8')),'hex')=cursor_;
    IF NOT FOUND THEN
      RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='visit scope unavailable';
    END IF;
  END IF;
  RETURN QUERY SELECT v.*,encode(sha256(convert_to(actor_::text||':'||coalesce(place_,'')||':'||v.issuer||':'||v.proof::text,'UTF8')),'hex')
   FROM travel_private.visit_options(actor_) v JOIN travel_private.visit_evidence e USING(issuer,proof)
   WHERE e.subject_ref=actor_ AND (place_ IS NULL OR e.place_id=place_)
      AND (cursor_ IS NULL OR e.listed_at < anchor.listed_at OR
           (e.listed_at=anchor.listed_at AND (e.issuer,e.proof)>(anchor.issuer,anchor.proof)))
   ORDER BY e.listed_at DESC,e.issuer,e.proof LIMIT 17;
END
$page$;

CREATE FUNCTION travel_private.challenge_count(actor_ uuid)
RETURNS bigint
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $count$
 SELECT count(*) FROM (SELECT DISTINCT v.place_id FROM travel_private.visit_options(actor_) v
   WHERE v.challenge='verified' LIMIT 3) places
$count$;

CREATE OR REPLACE FUNCTION travel_private.claim_visit(
    actor_ uuid,issuer_ text,proof_ uuid,purpose_ text,revision_ bigint,place_ text
) RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $claim$
DECLARE prior travel_private.visit_evidence; out_ uuid;
BEGIN
  IF purpose_ NOT IN ('review','challenge') THEN
      RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='visit scope unavailable';
  END IF;
  PERFORM travel_private.require_consent(actor_,purpose_,revision_,'p09',proof_);
  IF EXISTS(SELECT FROM travel_private.visit_erasure_fence f
      WHERE (f.issuer,f.proof,f.purpose)=(issuer_,proof_,purpose_) AND f.through_revision>=revision_)
     OR EXISTS(SELECT FROM travel_private.visit_claim c
      WHERE (c.issuer,c.proof,c.purpose)=(issuer_,proof_,purpose_)
        AND travel_private.fenced(actor_,'visit-claim',c.claim_ref,purpose_,c.consent_revision)) THEN
      RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='visit scope unavailable';
  END IF;
  SELECT * INTO prior FROM travel_private.visit_evidence WHERE issuer=issuer_ AND proof=proof_ FOR SHARE;
  IF NOT FOUND OR (prior.subject_ref,prior.place_id,prior.state) IS DISTINCT FROM (actor_,place_,'settled') THEN
      RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='visit scope unavailable';
  END IF;
  INSERT INTO travel_private.visit_claim(issuer,proof,purpose,consent_revision)
    VALUES(issuer_,proof_,purpose_,revision_) ON CONFLICT(issuer,proof,purpose)
    DO UPDATE SET consent_revision=excluded.consent_revision
      WHERE travel_private.visit_claim.consent_revision<>excluded.consent_revision
    RETURNING claim_ref INTO out_;
  IF out_ IS NULL THEN
    SELECT claim_ref INTO out_ FROM travel_private.visit_claim WHERE (issuer,proof,purpose)=(issuer_,proof_,purpose_);
  END IF;
  RETURN out_;
END
$claim$;

CREATE FUNCTION travel_private.review_intent(actor_ uuid,issuer_ text,proof_ uuid,place_ text,revision_ bigint)
RETURNS TABLE(id uuid, environment text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $intent$
BEGIN
  PERFORM travel_private.require_consent(actor_,'review',revision_,'p09',proof_);
  RETURN QUERY SELECT r.id,e.environment FROM travel_private.visit_evidence e
    LEFT JOIN travel_private.review r ON (r.issuer,r.proof,r.subject_ref)=(e.issuer,e.proof,actor_)
    WHERE (e.issuer,e.proof,e.subject_ref,e.place_id)=(issuer_,proof_,actor_,place_);
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='review scope unavailable';
  END IF;
END
$intent$;

CREATE OR REPLACE FUNCTION travel_private.write_review(actor_ uuid,id_ uuid,issuer_ text,proof_ uuid,place_ text,revision_ bigint,
    publish_ boolean, hash_ bytea,sealed_ bytea) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $review$
DECLARE evidence travel_private.visit_evidence; prior travel_private.review; expiry timestamptz;
BEGIN
    expiry:=travel_private.require_consent(actor_,'review',revision_,'p09',proof_);
    IF travel_private.fenced(actor_,'review',id_,'review',revision_) OR
       EXISTS(SELECT FROM travel_private.visit_erasure_fence f
          WHERE (f.issuer,f.proof,f.purpose)=(issuer_,proof_,'review')) THEN
      RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='review scope unavailable';
    END IF;
    SELECT * INTO evidence FROM travel_private.visit_evidence WHERE issuer=issuer_ AND proof=proof_ FOR SHARE;
    IF NOT FOUND OR (evidence.subject_ref,evidence.place_id) IS DISTINCT FROM (actor_,place_) THEN
      RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='review scope unavailable';
    END IF;
    IF publish_ AND evidence.environment<>'live' THEN
      RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='review publication unavailable';
    END IF;
    SELECT * INTO prior FROM travel_private.review WHERE issuer=issuer_ AND proof=proof_ FOR UPDATE;
    IF FOUND THEN
      IF (prior.id,prior.consent_revision,prior.published,prior.content_hash)
         IS DISTINCT FROM (id_,revision_,publish_,hash_) THEN
        RAISE EXCEPTION USING ERRCODE='P1801', MESSAGE='review revision conflict';
      END IF;
      RETURN prior.published;
    END IF;
    IF evidence.state<>'settled' THEN
      RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='review scope unavailable';
    END IF;
    IF EXISTS(SELECT FROM travel_private.review WHERE id=id_) THEN
      RAISE EXCEPTION USING ERRCODE='P1801', MESSAGE='review identifier conflict';
    END IF;
    INSERT INTO travel_private.review VALUES(issuer_,proof_,id_,actor_,revision_,publish_,hash_,sealed_,expiry);
    RETURN publish_;
END
$review$;

CREATE FUNCTION travel_private.own_review(actor_ uuid,issuer_ text,proof_ uuid,place_ text)
RETURNS TABLE(id uuid,body bytea)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $own$
 SELECT r.id,r.sealed FROM travel_private.review r JOIN travel_private.visit_evidence e USING(issuer,proof)
 WHERE (r.subject_ref,r.issuer,r.proof,e.place_id)=(actor_,issuer_,proof_,place_) AND r.delete_after>statement_timestamp()
   AND NOT travel_private.fenced(actor_,'review',r.id,'review',r.consent_revision)
$own$;

CREATE FUNCTION travel_private.public_review_details(place_ text)
RETURNS TABLE(review_id uuid,body bytea,badge text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $public$
  SELECT r.id,r.sealed,CASE WHEN e.state='settled' THEN 'verified-visit' ELSE 'adjusted' END
   FROM travel_private.review r
   JOIN travel_private.visit_evidence e USING(issuer,proof)
   JOIN travel_private.consent c ON c.subject_ref=r.subject_ref AND c.purpose='review' AND c.revision=r.consent_revision
   JOIN travel_private.consent_decision d ON (d.subject_ref,d.purpose,d.revision)=(c.subject_ref,c.purpose,c.revision)
   JOIN travel_private.policy p ON (p.purpose,p.version,p.recipient)=(d.purpose,d.policy_version,d.recipient)
   WHERE e.place_id=place_ AND e.environment='live' AND r.published
     AND d.accepted AND p.active AND d.expires_at>statement_timestamp() AND r.delete_after>statement_timestamp()
     AND NOT travel_private.fenced(r.subject_ref,'review',r.id,'review',r.consent_revision)
   ORDER BY r.id LIMIT 50
$public$;

CREATE OR REPLACE FUNCTION travel_private.assert_visit_benefit(issuer_ text,proof_ uuid)
RETURNS TABLE(revision bigint,classification text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $assert$
DECLARE evidence travel_private.visit_evidence; previous travel_private.visit_benefit_assertion; stage text;
BEGIN
    SELECT * INTO evidence FROM travel_private.visit_evidence WHERE (issuer,proof)=(issuer_,proof_);
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='benefit scope unavailable'; END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(evidence.subject_ref::text||':challenge',900018));
    SELECT * INTO evidence FROM travel_private.visit_evidence WHERE (issuer,proof)=(issuer_,proof_) FOR UPDATE;
    stage:=CASE WHEN evidence.environment='synthetic' THEN 'test-only'
                WHEN evidence.state<>'settled' THEN 'adjusted'
                WHEN EXISTS(SELECT FROM travel_private.visit_options(evidence.subject_ref) v
                    WHERE v.issuer=issuer_ AND v.proof=proof_ AND v.challenge='verified') THEN 'eligible'
                ELSE 'permission-held' END;
    SELECT a.* INTO previous FROM travel_private.visit_benefit_assertion a WHERE (a.issuer,a.proof)=(issuer_,proof_);
    IF NOT FOUND OR previous.classification<>stage THEN
      INSERT INTO travel_private.visit_benefit_assertion VALUES(issuer_,proof_,coalesce(previous.revision,0)+1,stage)
          ON CONFLICT(issuer,proof) DO UPDATE SET revision=excluded.revision,classification=excluded.classification
          RETURNING * INTO previous;
    END IF;
    RETURN QUERY SELECT previous.revision,previous.classification;
END
$assert$;
REVOKE ALL ON FUNCTION travel_private.visit_options(uuid),travel_private.visit_option_page(uuid,text,text),travel_private.challenge_count(uuid),
 travel_private.review_intent(uuid,text,uuid,text,bigint),travel_private.own_review(uuid,text,uuid,text),travel_private.public_review_details(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION travel_private.visit_option_page(uuid,text,text),travel_private.challenge_count(uuid),
 travel_private.review_intent(uuid,text,uuid,text,bigint),travel_private.own_review(uuid,text,uuid,text) TO nu54_p09_private_user;
GRANT EXECUTE ON FUNCTION travel_private.public_review_details(text) TO nu54_p09_review_read;

ALTER TABLE travel_private.erasure_target DROP CONSTRAINT erasure_target_kind_check,
 ADD CONSTRAINT erasure_target_kind_check CHECK(kind IN ('record','job','review','visit-claim'));

ALTER TABLE travel_erasure.fence DROP CONSTRAINT fence_check1,
 ADD CONSTRAINT fence_scope_check CHECK(
   (kind='purpose' AND purpose IN ('location','voice','transcription','recommendation','review','challenge')
     AND through_revision IS NOT NULL AND through_revision>0 AND object_kind IS NULL AND object_token IS NULL)
   OR (kind='object' AND purpose IS NULL AND through_revision IS NULL
     AND object_kind IN ('record','job','review','itinerary','visit-claim') AND object_token IS NOT NULL));
ALTER TABLE travel_private.restore_fence DROP CONSTRAINT fence_check1,
 ADD CONSTRAINT restore_fence_scope_check CHECK(
   (kind='purpose' AND purpose IN ('location','voice','transcription','recommendation','review','challenge')
     AND through_revision IS NOT NULL AND through_revision>0 AND object_kind IS NULL AND object_token IS NULL)
   OR (kind='object' AND purpose IS NULL AND through_revision IS NULL
     AND object_kind IN ('record','job','review','itinerary','visit-claim') AND object_token IS NOT NULL));


CREATE FUNCTION travel_private.record_visit_erasure(actor_ uuid,object_ uuid,kind_ text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $erase$
DECLARE entry record;
BEGIN
    IF kind_='review' THEN
      SELECT r.issuer,r.proof,'review'::text AS purpose,r.consent_revision INTO entry FROM travel_private.review r
       WHERE (r.subject_ref,r.id)=(actor_,object_);
    ELSIF kind_='visit-claim' THEN
      SELECT c.issuer,c.proof,c.purpose,c.consent_revision INTO entry FROM travel_private.visit_claim c
        JOIN travel_private.visit_evidence e USING(issuer,proof)
        WHERE e.subject_ref=actor_ AND c.claim_ref=object_ AND c.purpose='challenge';
    ELSE RETURN false;
    END IF;
    IF NOT FOUND THEN RETURN false; END IF;
    INSERT INTO travel_private.visit_erasure_fence VALUES(entry.issuer,entry.proof,entry.purpose,entry.consent_revision)
       ON CONFLICT(issuer,proof,purpose) DO UPDATE SET through_revision=greatest(
         travel_private.visit_erasure_fence.through_revision,excluded.through_revision);
    RETURN true;
END
$erase$;

CREATE FUNCTION travel_private.request_visit_erasure(actor_ uuid,key_ uuid,object_ uuid,kind_ text)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $erase$
DECLARE prior travel_private.erasure_request;
BEGIN
    IF kind_ NOT IN ('review','visit-claim') THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='erasure scope unavailable';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(actor_::text||':'||CASE WHEN kind_='review' THEN 'review' ELSE 'challenge' END,900018));
    SELECT r.* INTO prior FROM travel_private.erasure_request r WHERE (r.subject_ref,r.id)=(actor_,key_);
    IF FOUND AND (prior.source_id<>object_ OR NOT EXISTS(SELECT FROM travel_private.erasure_target t
         WHERE (t.subject_ref,t.request_id,t.kind,t.target)=(actor_,key_,kind_,object_))) THEN
        RAISE EXCEPTION USING ERRCODE='P1801', MESSAGE='erasure conflict';
    END IF;
    SELECT r.* INTO prior FROM travel_private.erasure_request r JOIN travel_private.erasure_target t
       ON (r.subject_ref,r.id)=(t.subject_ref,t.request_id)
       WHERE (r.subject_ref,t.kind,t.target)=(actor_,kind_,object_);
    IF FOUND THEN RETURN prior.id; END IF;
    IF NOT travel_private.record_visit_erasure(actor_,object_,kind_) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='erasure scope unavailable';
    END IF;
    INSERT INTO travel_private.erasure_request(subject_ref,id,source_id) VALUES(actor_,key_,object_);
    INSERT INTO travel_private.erasure_target VALUES(actor_,key_,kind_,object_);
    RETURN key_;
END
$erase$;

CREATE OR REPLACE FUNCTION travel_private.finish_selected_erasure(actor_ uuid,key_ uuid,namespace_ uuid,sequence_ bigint[],through_ bigint)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $finish$
DECLARE req travel_private.erasure_request; checkpoint travel_private.restore_checkpoint;
BEGIN
    SELECT * INTO req FROM travel_private.erasure_request WHERE subject_ref=actor_ AND id=key_ FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='erasure scope unavailable'; END IF;
    IF req.journaled_at IS NOT NULL THEN RETURN false; END IF;
    SELECT * INTO checkpoint FROM travel_private.restore_checkpoint FOR UPDATE;
    IF FOUND THEN
        IF checkpoint.namespace<>namespace_ OR checkpoint.applied_through>through_ OR
           through_-checkpoint.applied_through <> (
               SELECT count(DISTINCT indexed) FROM unnest(sequence_) indexed
                WHERE indexed>checkpoint.applied_through AND indexed<=through_) THEN
          RAISE EXCEPTION USING ERRCODE='P1801', MESSAGE='restore namespace unavailable';
        END IF;
        UPDATE travel_private.restore_checkpoint SET applied_through=through_;
    END IF;
    DELETE FROM travel_private.record r USING travel_private.erasure_target t
      WHERE (r.subject_ref,r.id)=(actor_,req.source_id)
        AND (t.subject_ref,t.request_id,t.kind,t.target)=(actor_,key_,'record',req.source_id);
    UPDATE travel_private.erasure_request SET journaled_at=clock_timestamp() WHERE subject_ref=actor_ AND id=key_;
    RETURN true;
END
$finish$;

CREATE FUNCTION travel_private.finish_visit_erasure() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $erase$
BEGIN
    DELETE FROM travel_private.review r USING travel_private.erasure_target t
     WHERE (r.subject_ref,r.id)=(t.subject_ref,t.target) AND (t.subject_ref,t.request_id,t.kind)=(NEW.subject_ref,NEW.id,'review');
    DELETE FROM travel_private.visit_claim c USING travel_private.erasure_target t,travel_private.visit_evidence e
     WHERE (t.subject_ref,t.request_id,t.kind,t.target)=(NEW.subject_ref,NEW.id,'visit-claim',c.claim_ref)
       AND (e.issuer,e.proof,e.subject_ref)=(c.issuer,c.proof,t.subject_ref);
    RETURN NEW;
END
$erase$;
CREATE TRIGGER finish_visit_erasure AFTER UPDATE OF journaled_at ON travel_private.erasure_request
 FOR EACH ROW WHEN (OLD.journaled_at IS NULL AND NEW.journaled_at IS NOT NULL) EXECUTE FUNCTION travel_private.finish_visit_erasure();

CREATE FUNCTION travel_private.capture_visit_erasure_fence() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $restore$
BEGIN
    IF NEW.kind='object' AND NEW.object_kind IN ('review','visit-claim') THEN
      PERFORM travel_private.record_visit_erasure(NEW.subject_ref,NEW.object_token,NEW.object_kind);
      IF NEW.object_kind='visit-claim' THEN
        DELETE FROM travel_private.visit_claim c USING travel_private.visit_evidence e
          WHERE (c.issuer,c.proof)=(e.issuer,e.proof) AND e.subject_ref=NEW.subject_ref
            AND c.purpose='challenge' AND c.claim_ref=NEW.object_token;
      END IF;
    END IF;
    RETURN NEW;
END
$restore$;
CREATE TRIGGER capture_visit_erasure_fence AFTER INSERT ON travel_private.restore_fence
 FOR EACH ROW EXECUTE FUNCTION travel_private.capture_visit_erasure_fence();

REVOKE ALL ON FUNCTION travel_private.request_visit_erasure(uuid,uuid,uuid,text),
 travel_private.record_visit_erasure(uuid,uuid,text), travel_private.finish_visit_erasure(),
 travel_private.capture_visit_erasure_fence() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION travel_private.request_visit_erasure(uuid,uuid,uuid,text) TO nu54_p09_private_user;

CREATE OR REPLACE FUNCTION travel_private.selected_erasure_status_v2(actor_ uuid,key_ uuid)
RETURNS TABLE(status text,local_device_ack boolean,provider_pending boolean,backup_expiry timestamptz,local_device_evidence text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $status$
    SELECT s.*,CASE WHEN EXISTS(SELECT FROM travel_private.erasure_target t
                         WHERE (t.subject_ref,t.request_id)=(actor_,key_) AND t.kind IN ('review','visit-claim')) THEN 'not-applicable'
                    WHEN NOT s.local_device_ack THEN 'none'
                    WHEN e.installation_proof_digest IS NULL THEN 'subject-reported'
                    ELSE 'installation-reported' END
    FROM travel_private.selected_erasure_status(actor_,key_) s
    JOIN travel_private.erasure_request e ON e.subject_ref=actor_ AND e.id=key_
$status$;

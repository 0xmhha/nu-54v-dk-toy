CREATE TABLE travel_private.review (
    issuer text NOT NULL,
    proof uuid NOT NULL,
    id uuid NOT NULL UNIQUE,
    subject_ref uuid NOT NULL,
    consent_revision bigint NOT NULL CHECK(consent_revision>0),
    published boolean NOT NULL,
    content_hash bytea NOT NULL CHECK(octet_length(content_hash)=32),
    sealed bytea NOT NULL CHECK(octet_length(sealed) BETWEEN 29 AND 8192),
    delete_after timestamptz NOT NULL CHECK(isfinite(delete_after)),
    PRIMARY KEY(issuer,proof),
    FOREIGN KEY(issuer,proof) REFERENCES travel_private.visit_evidence
);
DO $roles$
BEGIN
  IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='nu54_p09_review_read') THEN
    CREATE ROLE nu54_p09_review_read NOLOGIN NOSUPERUSER NOCREATEROLE NOCREATEDB NOBYPASSRLS;
  END IF;
END
$roles$;

CREATE FUNCTION travel_private.write_review(actor_ uuid,id_ uuid,issuer_ text,proof_ uuid,place_ text,revision_ bigint,
    publish_ boolean, hash_ bytea,sealed_ bytea) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $review$
DECLARE evidence travel_private.visit_evidence; prior travel_private.review; expiry timestamptz;
BEGIN
    expiry:=travel_private.require_consent(actor_,'review',revision_,'p09',proof_);
    IF travel_private.fenced(actor_,'review',id_,'review',revision_) THEN
      RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='review scope unavailable';
    END IF;
    SELECT * INTO evidence FROM travel_private.visit_evidence WHERE issuer=issuer_ AND proof=proof_ FOR SHARE;
    IF NOT FOUND OR (evidence.subject_ref,evidence.place_id,evidence.state) IS DISTINCT FROM (actor_,place_,'settled') THEN
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
    IF EXISTS(SELECT FROM travel_private.review WHERE id=id_) THEN
      RAISE EXCEPTION USING ERRCODE='P1801', MESSAGE='review identifier conflict';
    END IF;
    INSERT INTO travel_private.review VALUES(issuer_,proof_,id_,actor_,revision_,publish_,hash_,sealed_,expiry);
    RETURN publish_;
END
$review$;

CREATE FUNCTION travel_private.published_reviews(place_ text)
RETURNS TABLE(review_id uuid,body bytea)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $public$
  SELECT r.id,r.sealed FROM travel_private.review r
   JOIN travel_private.visit_evidence e USING(issuer,proof)
   JOIN travel_private.consent c ON c.subject_ref=r.subject_ref AND c.purpose='review' AND c.revision=r.consent_revision
   JOIN travel_private.consent_decision d ON (d.subject_ref,d.purpose,d.revision)=(c.subject_ref,c.purpose,c.revision)
   JOIN travel_private.policy p ON (p.purpose,p.version,p.recipient)=(d.purpose,d.policy_version,d.recipient)
   WHERE e.place_id=place_ AND e.environment='live' AND e.state='settled' AND r.published
     AND d.accepted AND p.active AND d.expires_at>statement_timestamp() AND r.delete_after>statement_timestamp()
     AND NOT travel_private.fenced(r.subject_ref,'review',r.id,'review',r.consent_revision)
   ORDER BY r.id LIMIT 50
$public$;

CREATE FUNCTION travel_private.restore_review_fence() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $restore$
BEGIN
    IF NEW.kind='purpose' AND NEW.purpose='review' THEN
        DELETE FROM travel_private.review WHERE subject_ref=NEW.subject_ref AND consent_revision<=NEW.through_revision;
    ELSIF NEW.kind='object' AND NEW.object_kind='review' THEN
        DELETE FROM travel_private.review WHERE subject_ref=NEW.subject_ref AND id=NEW.object_token;
    END IF;
    RETURN NEW;
END
$restore$;
CREATE TRIGGER restore_review_fence AFTER INSERT ON travel_private.restore_fence
FOR EACH ROW EXECUTE FUNCTION travel_private.restore_review_fence();

REVOKE ALL ON FUNCTION travel_private.write_review(uuid,uuid,text,uuid,text,bigint,boolean,bytea,bytea),
    travel_private.published_reviews(text),travel_private.restore_review_fence() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION travel_private.write_review(uuid,uuid,text,uuid,text,bigint,boolean,bytea,bytea) TO nu54_p09_private_user;
GRANT USAGE ON SCHEMA travel_private TO nu54_p09_review_read;
GRANT EXECUTE ON FUNCTION travel_private.published_reviews(text) TO nu54_p09_review_read;

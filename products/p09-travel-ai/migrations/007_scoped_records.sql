CREATE TABLE travel_private.record (
    subject_ref uuid NOT NULL,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    purpose text NOT NULL,
    revision bigint NOT NULL,
    recipient text NOT NULL,
    parent_id uuid,
    created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    delete_after timestamptz NOT NULL CHECK(isfinite(delete_after)),
    PRIMARY KEY(subject_ref,id),
    FOREIGN KEY(subject_ref,purpose,revision) REFERENCES travel_private.consent_decision,
    FOREIGN KEY(subject_ref,parent_id) REFERENCES travel_private.record ON DELETE CASCADE
);
CREATE TABLE travel_private.record_object (
    subject_ref uuid NOT NULL,
    record_id uuid NOT NULL,
    content_hash bytea NOT NULL CHECK(octet_length(content_hash)=32),
    sealed bytea NOT NULL CHECK(octet_length(sealed) BETWEEN 29 AND 1048604),
    PRIMARY KEY(subject_ref,record_id),
    FOREIGN KEY(subject_ref,record_id) REFERENCES travel_private.record ON DELETE CASCADE
);
CREATE FUNCTION travel_private.fenced(actor_ uuid,kind_ text,item_ uuid,purpose_ text,revision_ bigint)
RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $fence$
    SELECT EXISTS(SELECT FROM travel_private.restore_fence f WHERE f.subject_ref=actor_ AND
       ((f.kind='object' AND f.object_kind=kind_ AND f.object_token=item_) OR
        (f.kind='purpose' AND f.purpose=purpose_ AND f.through_revision>=revision_)))
$fence$;

CREATE FUNCTION travel_private.write_record(actor_ uuid, item_ uuid, purpose_ text, rev_ bigint,recipient_ text,
    parent_ uuid,delete_ timestamptz,digest_ bytea,sealed_ bytea) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $record$
DECLARE
    expiry timestamptz;
    own travel_private.record;
    parent travel_private.record;
BEGIN
    IF purpose_ NOT IN ('location','voice','transcription','recommendation') OR
       (purpose_='transcription') IS DISTINCT FROM (parent_ IS NOT NULL) OR item_ IS NULL OR
       delete_ IS NULL OR delete_<=clock_timestamp() OR sealed_ IS NULL THEN
      RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='record scope unavailable';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(actor_::text||':record:'||item_::text,900018));
    SELECT * INTO own FROM travel_private.record WHERE subject_ref=actor_ AND id=item_;
    IF FOUND THEN
      IF (own.purpose,own.revision,own.recipient,own.parent_id,own.delete_after) IS DISTINCT FROM
          (purpose_,rev_,recipient_,parent_,delete_) THEN
        RAISE EXCEPTION USING ERRCODE='P1801', MESSAGE='record scope conflict';
      END IF;
      IF NOT EXISTS (SELECT FROM travel_private.record_object WHERE subject_ref=actor_ AND record_id=item_ AND content_hash=digest_) THEN
        RAISE EXCEPTION USING ERRCODE='P1801', MESSAGE='record content conflict';
      END IF;
    END IF;
    -- Voice precedes its dependent transcription in the lock order.
    IF parent_ IS NOT NULL THEN
      SELECT * INTO parent FROM travel_private.record WHERE subject_ref=actor_ AND id=parent_ AND purpose='voice';
      IF NOT FOUND OR parent.delete_after<=clock_timestamp() OR travel_private.fenced(actor_,'record',parent.id,parent.purpose,parent.revision) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='record scope unavailable';
      END IF;
      PERFORM travel_private.require_consent(actor_,parent.purpose,parent.revision,parent.recipient,parent.id);
      PERFORM 1 FROM travel_private.record WHERE subject_ref=actor_ AND id=parent_ FOR SHARE;
      IF NOT FOUND OR parent.delete_after<delete_ THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='record scope unavailable';
      END IF;
    END IF;
    expiry:=travel_private.require_consent(actor_,purpose_,rev_,recipient_,coalesce(parent_,item_));
    IF delete_>expiry OR travel_private.fenced(actor_,'record',item_,purpose_,rev_) THEN
      RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='record scope unavailable';
    END IF;
    IF own.id IS NOT NULL THEN RETURN own.id; END IF;
    INSERT INTO travel_private.record(subject_ref,id,purpose,revision,recipient,parent_id,delete_after)
      VALUES(actor_,item_,purpose_,rev_,recipient_,parent_,delete_);
    INSERT INTO travel_private.record_object VALUES(actor_,item_,digest_,sealed_);
    RETURN item_;
END
$record$;

CREATE FUNCTION travel_private.my_records(actor_ uuid)
RETURNS TABLE(id uuid,purpose text,revision bigint,parent uuid,delete_after timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $records$
  SELECT r.id,r.purpose,r.revision,r.parent_id,r.delete_after FROM travel_private.record r
  JOIN travel_private.consent c USING(subject_ref,purpose,revision)
  JOIN travel_private.consent_decision d USING(subject_ref,purpose,revision)
  JOIN travel_private.policy p ON (p.purpose,p.version,p.recipient)=(d.purpose,d.policy_version,d.recipient)
  LEFT JOIN travel_private.record parent ON (parent.subject_ref,parent.id)=(r.subject_ref,r.parent_id)
  WHERE r.subject_ref=actor_ AND d.accepted AND p.active AND d.expires_at>statement_timestamp()
   AND r.delete_after>statement_timestamp() AND NOT travel_private.fenced(actor_,'record',r.id,r.purpose,r.revision)
   AND (r.parent_id IS NULL OR (parent.delete_after>statement_timestamp()
      AND NOT travel_private.fenced(actor_,'record',parent.id,parent.purpose,parent.revision)
      AND EXISTS (SELECT FROM travel_private.consent c2 JOIN travel_private.consent_decision d2 USING(subject_ref,purpose,revision)
        JOIN travel_private.policy p2 ON (p2.purpose,p2.version,p2.recipient)=(d2.purpose,d2.policy_version,d2.recipient)
        WHERE c2.subject_ref=actor_ AND c2.purpose=parent.purpose AND c2.revision=parent.revision AND d2.accepted
          AND d2.expires_at>statement_timestamp() AND p2.active)))
  ORDER BY r.created_at DESC LIMIT 100
$records$;

CREATE FUNCTION travel_private.restore_content_fence() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $restore$
BEGIN
  IF NEW.kind='purpose' THEN
    DELETE FROM travel_private.record WHERE subject_ref=NEW.subject_ref AND purpose=NEW.purpose
      AND revision<=NEW.through_revision;
  ELSIF NEW.object_kind='record' THEN
    DELETE FROM travel_private.record WHERE subject_ref=NEW.subject_ref AND id=NEW.object_token;
  END IF;
  RETURN NEW;
END
$restore$;
CREATE TRIGGER restore_content_fence AFTER INSERT ON travel_private.restore_fence
FOR EACH ROW EXECUTE FUNCTION travel_private.restore_content_fence();

REVOKE ALL ON FUNCTION travel_private.fenced(uuid,text,uuid,text,bigint),travel_private.restore_content_fence(),
    travel_private.write_record(uuid,uuid,text,bigint,text,uuid,timestamptz,bytea,bytea),travel_private.my_records(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION travel_private.write_record(uuid,uuid,text,bigint,text,uuid,timestamptz,bytea,bytea),
    travel_private.my_records(uuid) TO nu54_p09_private_user;

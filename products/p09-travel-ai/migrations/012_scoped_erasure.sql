CREATE TABLE travel_private.erasure_request (
    subject_ref uuid NOT NULL,
    id uuid NOT NULL,
    source_id uuid NOT NULL,
    requested_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    journaled_at timestamptz,
    this_device_erased_at timestamptz,
    PRIMARY KEY(subject_ref,id),
    UNIQUE(subject_ref,source_id)
);
CREATE TABLE travel_private.erasure_target (
    subject_ref uuid NOT NULL,
    request_id uuid NOT NULL,
    kind text NOT NULL CHECK(kind IN ('record','job')),
    target uuid NOT NULL,
    PRIMARY KEY(subject_ref,request_id,kind,target),
    FOREIGN KEY(subject_ref,request_id) REFERENCES travel_private.erasure_request
);
DO $roles$
BEGIN
    IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='nu54_p09_erasure_worker') THEN
        CREATE ROLE nu54_p09_erasure_worker NOLOGIN NOSUPERUSER NOCREATEROLE NOCREATEDB NOBYPASSRLS;
    END IF;
END
$roles$;

CREATE OR REPLACE FUNCTION travel_private.fenced(actor_ uuid,kind_ text,item_ uuid,purpose_ text,revision_ bigint)
RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $fence$
    SELECT EXISTS(SELECT FROM travel_private.restore_fence f WHERE f.subject_ref=actor_ AND
       ((f.kind='object' AND f.object_kind=kind_ AND f.object_token=item_) OR
        (f.kind='purpose' AND f.purpose=purpose_ AND f.through_revision>=revision_)))
      OR EXISTS(SELECT FROM travel_private.erasure_target task
        WHERE task.subject_ref=actor_ AND task.kind=kind_ AND task.target=item_)
$fence$;

CREATE FUNCTION travel_private.request_selected_erasure(actor_ uuid,key_ uuid,object_ uuid)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $request$
DECLARE previous travel_private.erasure_request;
BEGIN
    IF actor_ IS NULL OR key_ IS NULL OR object_ IS NULL THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='erasure scope unavailable';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(actor_::text||':voice',900018));
    PERFORM pg_advisory_xact_lock(hashtextextended(actor_::text||':transcription',900018));
    SELECT * INTO previous FROM travel_private.erasure_request WHERE subject_ref=actor_ AND id=key_;
    IF FOUND THEN
        IF previous.source_id<>object_ THEN RAISE EXCEPTION USING ERRCODE='P1801', MESSAGE='erasure conflict'; END IF;
        RETURN previous.id;
    END IF;
    SELECT * INTO previous FROM travel_private.erasure_request WHERE subject_ref=actor_ AND source_id=object_;
    IF FOUND THEN RETURN previous.id; END IF;
    IF NOT EXISTS (SELECT FROM travel_private.record WHERE subject_ref=actor_ AND id=object_) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='erasure scope unavailable';
    END IF;
    INSERT INTO travel_private.erasure_request(subject_ref,id,source_id) VALUES(actor_,key_,object_);
    WITH RECURSIVE selected(id) AS (
        SELECT object_ UNION ALL
        SELECT r.id FROM travel_private.record r JOIN selected ON selected.id=r.parent_id WHERE r.subject_ref=actor_
    )
    INSERT INTO travel_private.erasure_target
      SELECT actor_,key_,'record',selected.id FROM selected
      UNION ALL
      SELECT actor_,key_,'job',j.id FROM travel_private.processing_job j JOIN selected ON selected.id=j.source_token
          WHERE j.subject_ref=actor_;
    UPDATE travel_private.processing_job j SET state=CASE WHEN j.dispatched_at IS NULL THEN 'erased'
        WHEN j.ended_at IS NOT NULL THEN 'delete-pending' ELSE 'cancel-pending' END,retry_at=clock_timestamp()
      FROM travel_private.erasure_target task
      WHERE (j.subject_ref,j.id)=(task.subject_ref,task.target) AND task.kind='job' AND
          task.subject_ref=actor_ AND task.request_id=key_ AND j.state<>'erased';
    RETURN key_;
END
$request$;

CREATE FUNCTION travel_private.selected_erasure_status(actor_ uuid,key_ uuid)
RETURNS TABLE(status text,local_device_ack boolean,provider_pending boolean,backup_expiry timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $status$
    SELECT CASE WHEN e.journaled_at IS NULL THEN 'journal-pending'
                WHEN EXISTS(SELECT FROM travel_private.erasure_target task
                  JOIN travel_private.processing_job j ON (j.subject_ref,j.id)=(task.subject_ref,task.target)
                  WHERE task.subject_ref=e.subject_ref AND task.request_id=e.id AND task.kind='job' AND j.state<>'erased')
                THEN 'provider-pending'
                ELSE 'access-fenced' END,
           e.this_device_erased_at IS NOT NULL,
           EXISTS(SELECT FROM travel_private.erasure_target task
                  JOIN travel_private.processing_job j ON (j.subject_ref,j.id)=(task.subject_ref,task.target)
                  WHERE task.subject_ref=e.subject_ref AND task.request_id=e.id AND task.kind='job' AND j.state<>'erased'),
           e.requested_at+interval '90 days'
    FROM travel_private.erasure_request e WHERE e.subject_ref=actor_ AND e.id=key_
$status$;

CREATE FUNCTION travel_private.pending_erasure_targets()
RETURNS TABLE(subject_ref uuid,request_id uuid,kind text,target uuid,cutoff timestamptz)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $worker$
  SELECT task.subject_ref,task.request_id,task.kind,task.target,e.requested_at
    FROM travel_private.erasure_target task JOIN travel_private.erasure_request e
      ON (e.subject_ref,e.id)=(task.subject_ref,task.request_id)
    WHERE e.journaled_at IS NULL AND (task.subject_ref,task.request_id)=(
      SELECT oldest.subject_ref,oldest.id FROM travel_private.erasure_request oldest
        WHERE oldest.journaled_at IS NULL ORDER BY oldest.requested_at,oldest.id LIMIT 1)
    ORDER BY task.kind,task.target
$worker$;

CREATE FUNCTION travel_private.finish_selected_erasure(actor_ uuid,key_ uuid,namespace_ uuid,sequence_ bigint[],through_ bigint)
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
    DELETE FROM travel_private.record WHERE subject_ref=actor_ AND id=req.source_id;
    UPDATE travel_private.erasure_request SET journaled_at=clock_timestamp() WHERE subject_ref=actor_ AND id=key_;
    RETURN true;
END
$finish$;

CREATE FUNCTION travel_private.mark_device_erased(actor_ uuid,key_ uuid) RETURNS boolean
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $device$
  WITH changed AS (UPDATE travel_private.erasure_request SET this_device_erased_at=coalesce(this_device_erased_at,clock_timestamp())
    WHERE subject_ref=actor_ AND id=key_ RETURNING 1) SELECT EXISTS(SELECT FROM changed)
$device$;

REVOKE ALL ON FUNCTION travel_private.request_selected_erasure(uuid,uuid,uuid),
    travel_private.selected_erasure_status(uuid,uuid),travel_private.pending_erasure_targets(),
    travel_private.finish_selected_erasure(uuid,uuid,uuid,bigint[],bigint),travel_private.mark_device_erased(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION travel_private.request_selected_erasure(uuid,uuid,uuid),travel_private.selected_erasure_status(uuid,uuid),
    travel_private.mark_device_erased(uuid,uuid) TO nu54_p09_private_user;
GRANT USAGE ON SCHEMA travel_private TO nu54_p09_erasure_worker;
GRANT EXECUTE ON FUNCTION travel_private.pending_erasure_targets(),
    travel_private.finish_selected_erasure(uuid,uuid,uuid,bigint[],bigint) TO nu54_p09_erasure_worker;


CREATE FUNCTION travel_erasure.writer_cursor()
RETURNS TABLE(namespace uuid,high_water bigint)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $state$
    SELECT n.namespace,n.high_water FROM travel_erasure.namespace n
$state$;
REVOKE ALL ON FUNCTION travel_erasure.writer_cursor() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION travel_erasure.writer_cursor() TO nu54_p09_erasure_append;

CREATE FUNCTION travel_private.read_record(actor_ uuid,item_ uuid)
RETURNS TABLE(purpose text,sealed bytea)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $read$
  SELECT meta.purpose,content.sealed FROM travel_private.my_records(actor_) meta
   JOIN travel_private.record_object content ON content.subject_ref=actor_ AND content.record_id=meta.id
   WHERE meta.id=item_
$read$;
REVOKE ALL ON FUNCTION travel_private.read_record(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION travel_private.read_record(uuid,uuid) TO nu54_p09_private_user;

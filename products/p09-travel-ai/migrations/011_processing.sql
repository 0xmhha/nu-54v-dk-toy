CREATE TABLE travel_private.processing_job (
    subject_ref uuid NOT NULL,
    id uuid NOT NULL,
    parent_id uuid,
    source_token uuid NOT NULL,
    consent_revision bigint NOT NULL CHECK(consent_revision>0),
    recipient text NOT NULL,
    handle uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    state text NOT NULL CHECK(state IN ('queued','dispatching','processing','completed','cancel-pending','delete-pending','erased')),
    expires_at timestamptz NOT NULL CHECK(isfinite(expires_at)),
    retry_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    dispatched_at timestamptz CHECK(isfinite(dispatched_at)),
    ended_at timestamptz CHECK(isfinite(ended_at)),
    deleted_at timestamptz CHECK(isfinite(deleted_at)),
    PRIMARY KEY(subject_ref,id),
    FOREIGN KEY(subject_ref,parent_id) REFERENCES travel_private.record ON DELETE SET NULL(parent_id),
    CHECK(state<>'erased' OR dispatched_at IS NULL OR (ended_at IS NOT NULL AND deleted_at IS NOT NULL AND deleted_at>=ended_at))
);
CREATE INDEX processing_reconcile ON travel_private.processing_job(state,expires_at);

CREATE TABLE travel_private.processing_recipient (
    login_role name PRIMARY KEY,
    recipient text NOT NULL CHECK(recipient ~ '^[a-z0-9][a-z0-9_-]{0,63}$')
);
DO $roles$
BEGIN
    IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='nu54_p09_processor') THEN
        CREATE ROLE nu54_p09_processor NOLOGIN NOSUPERUSER NOCREATEROLE NOCREATEDB NOBYPASSRLS;
    END IF;
END
$roles$;

CREATE FUNCTION travel_private.queue_transcription(actor_ uuid,job_ uuid,source_ uuid,rev_ bigint,recipient_ text)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $queue$
DECLARE src travel_private.record; prior travel_private.processing_job; expiry timestamptz;
BEGIN
    SELECT * INTO src FROM travel_private.record WHERE subject_ref=actor_ AND id=source_ AND purpose='voice';
    IF NOT FOUND OR src.delete_after<=clock_timestamp() OR travel_private.fenced(actor_,'record',source_,src.purpose,src.revision) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='record scope unavailable';
    END IF;
    PERFORM travel_private.require_consent(actor_,src.purpose,src.revision,src.recipient,source_);
    expiry:=travel_private.require_consent(actor_,'transcription',rev_,recipient_,source_);
    PERFORM 1 FROM travel_private.record WHERE subject_ref=actor_ AND id=source_ FOR SHARE;
    IF NOT FOUND OR travel_private.fenced(actor_,'job',job_,'transcription',rev_) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='work scope unavailable';
    END IF;
    SELECT * INTO prior FROM travel_private.processing_job WHERE subject_ref=actor_ AND id=job_;
    IF FOUND THEN
        IF (prior.source_token,prior.consent_revision,prior.recipient) IS DISTINCT FROM (source_,rev_,recipient_) THEN
           RAISE EXCEPTION USING ERRCODE='P1801', MESSAGE='work conflict';
        END IF;
        RETURN prior.id;
    END IF;
    INSERT INTO travel_private.processing_job(subject_ref,id,parent_id,source_token,consent_revision,recipient,state,expires_at)
        VALUES(actor_,job_,source_,source_,rev_,recipient_,'queued',least(expiry,src.delete_after));
    RETURN job_;
END
$queue$;

CREATE FUNCTION travel_private.cancel_jobs_on_withdrawal() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $cancel$
BEGIN
    IF NEW.accepted OR NEW.purpose NOT IN ('voice','transcription') THEN RETURN NEW; END IF;
    UPDATE travel_private.processing_job j
      SET state=CASE WHEN j.dispatched_at IS NULL THEN 'erased'
                     WHEN j.ended_at IS NOT NULL THEN 'delete-pending'
                     ELSE 'cancel-pending' END
      WHERE j.subject_ref=NEW.subject_ref AND j.state<>'erased'
        AND (NEW.purpose='voice' OR j.consent_revision<NEW.revision);
    RETURN NEW;
END
$cancel$;
CREATE TRIGGER cancel_jobs_on_withdrawal AFTER INSERT ON travel_private.consent_decision
FOR EACH ROW EXECUTE FUNCTION travel_private.cancel_jobs_on_withdrawal();

CREATE FUNCTION travel_private.next_processor_work(recipient_ text)
RETURNS TABLE(subject_ref uuid,job_id uuid,source_token uuid,handle uuid,action text,source bytea,expires_at timestamptz)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $process$
DECLARE work travel_private.processing_job; src travel_private.record; body bytea;
BEGIN
    IF NOT EXISTS(SELECT FROM travel_private.processing_recipient a WHERE a.login_role=session_user AND a.recipient=recipient_) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='recipient scope unavailable';
    END IF;
    SELECT j.* INTO work FROM travel_private.processing_job j
      WHERE j.recipient=recipient_ AND j.state NOT IN ('erased','completed') AND j.retry_at<=clock_timestamp()
      ORDER BY CASE WHEN j.state IN ('cancel-pending','delete-pending') THEN 0 ELSE 1 END,j.expires_at,j.id
      LIMIT 1;
    IF NOT FOUND THEN RETURN; END IF;
    -- The normal provider daemon uses the same lock order as withdrawal.
    PERFORM pg_advisory_xact_lock(hashtextextended(work.subject_ref::text||':voice',900018));
    PERFORM pg_advisory_xact_lock(hashtextextended(work.subject_ref::text||':transcription',900018));
    SELECT j.* INTO work FROM travel_private.processing_job j
      WHERE (j.subject_ref,j.id)=(work.subject_ref,work.id) FOR UPDATE;
    IF NOT FOUND OR work.state IN ('erased','completed') THEN RETURN; END IF;
    IF work.state IN ('queued','dispatching','processing') THEN
      BEGIN
        IF work.expires_at<=clock_timestamp() OR travel_private.fenced(work.subject_ref,'job',work.id,'transcription',work.consent_revision) THEN
           RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='work held';
        END IF;
        SELECT r.* INTO src FROM travel_private.record r WHERE r.subject_ref=work.subject_ref AND r.id=work.parent_id AND r.purpose='voice' FOR SHARE;
        IF NOT FOUND OR src.delete_after<=clock_timestamp() OR travel_private.fenced(src.subject_ref,'record',src.id,src.purpose,src.revision) THEN
           RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='work held';
        END IF;
        PERFORM travel_private.require_consent(src.subject_ref,src.purpose,src.revision,src.recipient,src.id);
        PERFORM travel_private.require_consent(work.subject_ref,'transcription',work.consent_revision,work.recipient,work.parent_id);
      EXCEPTION WHEN insufficient_privilege THEN
        UPDATE travel_private.processing_job j SET state=CASE
          WHEN j.dispatched_at IS NULL THEN 'erased'
          WHEN j.ended_at IS NOT NULL THEN 'delete-pending' ELSE 'cancel-pending' END
          WHERE (j.subject_ref,j.id)=(work.subject_ref,work.id) RETURNING j.* INTO work;
      END;
    END IF;
    IF work.state='erased' THEN RETURN; END IF;
    UPDATE travel_private.processing_job j SET retry_at=clock_timestamp()+interval '1 second'
      WHERE (j.subject_ref,j.id)=(work.subject_ref,work.id);
    IF work.state='queued' THEN
        SELECT b.sealed INTO STRICT body FROM travel_private.record_object b
          WHERE (b.subject_ref,b.record_id)=(work.subject_ref,work.parent_id);
        UPDATE travel_private.processing_job j SET state='dispatching',dispatched_at=clock_timestamp()
          WHERE (j.subject_ref,j.id)=(work.subject_ref,work.id);
        RETURN QUERY SELECT work.subject_ref,work.id,work.source_token,work.handle,'send',body,work.expires_at;
    ELSE
        RETURN QUERY SELECT work.subject_ref,work.id,work.source_token,work.handle,
           CASE WHEN work.state='cancel-pending' THEN 'cancel' WHEN work.state='delete-pending' THEN 'delete' ELSE 'status' END,
           NULL::bytea,work.expires_at;
    END IF;
END
$process$;

CREATE FUNCTION travel_private.store_provider_result(handle_ uuid,recipient_ text,digest_ bytea,sealed_ bytea) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $save$
DECLARE work travel_private.processing_job;
BEGIN
    IF NOT EXISTS(SELECT FROM travel_private.processing_recipient a WHERE a.login_role=session_user AND a.recipient=recipient_) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='recipient scope unavailable';
    END IF;
    SELECT * INTO work FROM travel_private.processing_job j WHERE j.handle=handle_ AND j.recipient=recipient_;
    IF NOT FOUND OR work.state NOT IN ('dispatching','processing') OR work.parent_id IS NULL OR
        travel_private.fenced(work.subject_ref,'job',work.id,'transcription',work.consent_revision) THEN RETURN false;
    END IF;
    BEGIN
      PERFORM travel_private.write_record(work.subject_ref,work.id,'transcription',work.consent_revision,work.recipient,
        work.parent_id,work.expires_at,digest_,sealed_);
    EXCEPTION WHEN insufficient_privilege THEN
      RETURN false;
    END;
    RETURN true;
END
$save$;

CREATE FUNCTION travel_private.provider_ack(handle_ uuid,recipient_ text,event_ text,ended_ timestamptz,deleted_ timestamptz)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $ack$
DECLARE work travel_private.processing_job;
BEGIN
    IF NOT EXISTS(SELECT FROM travel_private.processing_recipient a WHERE a.login_role=session_user AND a.recipient=recipient_) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='recipient scope unavailable';
    END IF;
    SELECT * INTO work FROM travel_private.processing_job j WHERE j.handle=handle_ AND j.recipient=recipient_ FOR UPDATE;
    IF NOT FOUND OR work.dispatched_at IS NULL THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='work scope unavailable';
    END IF;
    IF event_='accepted' AND work.state='dispatching' THEN
        UPDATE travel_private.processing_job SET state='processing' WHERE handle=handle_;
    ELSIF event_='terminal' AND ended_ IS NOT NULL AND ended_>=work.dispatched_at AND ended_<=clock_timestamp()
        AND work.state IN ('cancel-pending','dispatching','processing') THEN
        UPDATE travel_private.processing_job SET state='delete-pending',ended_at=ended_ WHERE handle=handle_;
    ELSIF event_='erased' AND work.state='delete-pending' AND ended_=work.ended_at
        AND deleted_ IS NOT NULL AND deleted_>=work.ended_at AND deleted_<=clock_timestamp() THEN
        UPDATE travel_private.processing_job SET state='erased',deleted_at=deleted_ WHERE handle=handle_;
    ELSE
        RETURN false;
    END IF;
    RETURN true;
END
$ack$;

CREATE FUNCTION travel_private.restore_job_fence() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $restore$
BEGIN
    UPDATE travel_private.processing_job j SET state=CASE WHEN j.dispatched_at IS NULL THEN 'erased'
        WHEN j.ended_at IS NOT NULL THEN 'delete-pending' ELSE 'cancel-pending' END
      WHERE j.subject_ref=NEW.subject_ref AND j.state<>'erased' AND
        ((NEW.kind='object' AND ((NEW.object_kind='job' AND j.id=NEW.object_token)
          OR (NEW.object_kind='record' AND j.source_token=NEW.object_token))) OR
         (NEW.kind='purpose' AND ((NEW.purpose='transcription' AND j.consent_revision<=NEW.through_revision)
          OR (NEW.purpose='voice' AND j.parent_id IS NULL))));
    RETURN NEW;
END
$restore$;
CREATE TRIGGER zz_restore_job_fence AFTER INSERT ON travel_private.restore_fence
FOR EACH ROW EXECUTE FUNCTION travel_private.restore_job_fence();

REVOKE ALL ON FUNCTION travel_private.queue_transcription(uuid,uuid,uuid,bigint,text),
  travel_private.cancel_jobs_on_withdrawal(),travel_private.restore_job_fence(),
  travel_private.next_processor_work(text),travel_private.store_provider_result(uuid,text,bytea,bytea),travel_private.provider_ack(uuid,text,text,timestamptz,timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION travel_private.queue_transcription(uuid,uuid,uuid,bigint,text) TO nu54_p09_private_user;
GRANT USAGE ON SCHEMA travel_private TO nu54_p09_processor;
GRANT EXECUTE ON FUNCTION travel_private.next_processor_work(text),travel_private.store_provider_result(uuid,text,bytea,bytea),travel_private.provider_ack(uuid,text,text,timestamptz,timestamptz) TO nu54_p09_processor;

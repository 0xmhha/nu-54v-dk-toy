CREATE TABLE platform.recovery_rehearsal (
    job_id uuid PRIMARY KEY,
    scope_ref uuid NOT NULL REFERENCES platform.recovery_route,
    requested_by uuid NOT NULL,
    profile_ref text NOT NULL CHECK (profile_ref ~ '^[a-z][a-z0-9-]{0,39}$'),
    profile_digest platform.digest32 NOT NULL,
    state text NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','running','stopped','hold','ready')),
    stage text NOT NULL DEFAULT 'queued' CHECK (stage IN ('queued','restore','isolation','erasure','replay','report')),
    attempt uuid,
    lease_until timestamptz CHECK (isfinite(lease_until)),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    report_digest platform.digest32,
    report_signature text,
    CHECK ((report_digest IS NULL) = (report_signature IS NULL)),
    CHECK (state <> 'ready' OR report_digest IS NOT NULL)
);

DO $role$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p10_recovery_worker') THEN
        CREATE ROLE nu54_p10_recovery_worker NOLOGIN NOSUPERUSER NOBYPASSRLS;
    ELSIF EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p10_recovery_worker'
        AND (rolcanlogin OR rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication OR rolbypassrls)) THEN
        RAISE EXCEPTION 'unsafe rehearsal worker capability';
    END IF;
END
$role$;
GRANT USAGE ON SCHEMA platform TO nu54_p10_recovery_worker;
GRANT USAGE ON DOMAIN platform.digest32 TO nu54_p10_recovery_worker;
GRANT SELECT ON platform.recovery_rehearsal TO nu54_p10_recovery_worker;
GRANT UPDATE(state,stage,attempt,lease_until,updated_at,report_digest,report_signature)
    ON platform.recovery_rehearsal TO nu54_p10_recovery_worker;
GRANT SELECT ON platform.recovery_rehearsal TO nu54_p10_recovery_status,nu54_p10_recovery_coordinator;

CREATE FUNCTION platform.request_recovery_rehearsal(scope_ uuid, actor_ uuid, job_ uuid, profile_ text,
    profile_digest_ bytea, until_ timestamptz, stop_ boolean)
RETURNS boolean LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $job$
DECLARE found_job platform.recovery_rehearsal%ROWTYPE;
BEGIN
    PERFORM pg_advisory_xact_lock(hashtextextended(scope_::text,440054));
    IF NOT platform.recovery_operator_authorized(scope_,actor_,'requester',true) OR clock_timestamp()>=until_ THEN
        RETURN false;
    END IF;
    IF stop_ THEN
        UPDATE platform.recovery_rehearsal SET state='stopped',updated_at=clock_timestamp()
          WHERE (job_id,scope_ref,requested_by)=(job_,scope_,actor_) AND state IN ('queued','running');
        IF NOT FOUND THEN
            RETURN EXISTS (SELECT 1 FROM platform.recovery_rehearsal WHERE
                (job_id,scope_ref,requested_by,state)=(job_,scope_,actor_,'stopped'));
        END IF;
    ELSE
        IF EXISTS (SELECT FROM platform.recovery_run WHERE job_id=job_) AND
           NOT EXISTS (SELECT FROM platform.recovery_rehearsal WHERE job_id=job_) THEN
            RETURN false;
        END IF;
        INSERT INTO platform.recovery_rehearsal(job_id,scope_ref,requested_by,profile_ref,profile_digest)
            VALUES(job_,scope_,actor_,profile_,profile_digest_) ON CONFLICT(job_id) DO NOTHING;
        SELECT * INTO found_job FROM platform.recovery_rehearsal WHERE job_id=job_;
        IF (found_job.scope_ref,found_job.requested_by,found_job.profile_ref,found_job.profile_digest) IS DISTINCT FROM
           (scope_,actor_,profile_,profile_digest_) THEN
            RETURN false;
        END IF;
    END IF;
    IF NOT platform.recovery_operator_authorized(scope_,actor_,'requester',true) OR clock_timestamp()>=until_ THEN
        RAISE EXCEPTION 'rehearsal operator lease ended';
    END IF;
    RETURN true;
END
$job$;

REVOKE ALL ON FUNCTION platform.request_recovery_rehearsal(uuid,uuid,uuid,text,bytea,timestamptz,boolean) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.request_recovery_rehearsal(uuid,uuid,uuid,text,bytea,timestamptz,boolean)
    TO nu54_p10_recovery_coordinator;

-- Durable restore coordination. The coordinator validates enrolled IdP subjects,
-- signed approvals, and signed target receipts before invoking these operations.
CREATE TABLE platform.recovery_route (
    scope_ref uuid PRIMARY KEY,
    revision bigint NOT NULL CHECK (revision >= 0),
    active_job uuid,
    current_manifest platform.digest32 NOT NULL
);
CREATE TABLE platform.recovery_run (
    job_id uuid PRIMARY KEY,
    scope_ref uuid NOT NULL REFERENCES platform.recovery_route(scope_ref),
    prior_job uuid REFERENCES platform.recovery_run(job_id),
    route_revision bigint NOT NULL CHECK (route_revision >= 0),
    manifest platform.digest32 NOT NULL,
    plan_digest platform.digest32 NOT NULL,
    signed_plan jsonb NOT NULL,
    expires_at timestamptz NOT NULL CHECK (isfinite(expires_at)),
    state text NOT NULL CHECK (state IN ('fenced','complete','superseded')),
    rto_seconds integer CHECK (rto_seconds >= 0),
    result_digest platform.digest32
        CHECK ((result_digest IS NOT NULL AND rto_seconds IS NOT NULL) OR state <> 'complete'),
    UNIQUE (scope_ref, job_id)
);
ALTER TABLE platform.recovery_route ADD FOREIGN KEY(scope_ref,active_job)
    REFERENCES platform.recovery_run(scope_ref,job_id);
CREATE TABLE platform.recovery_target_receipt (
    job_id uuid NOT NULL REFERENCES platform.recovery_run(job_id),
    target_id text NOT NULL CHECK (target_id IN ('p05','p07','p09')),
    signed_receipt jsonb NOT NULL,
    PRIMARY KEY (job_id,target_id)
);

-- This pair is installed in each destination-control database. Target publisher
-- applies a CAS to its own scoped pointer and commits the signed receipt with it.
CREATE TABLE platform.recovery_target_route (
    scope_ref uuid NOT NULL,
    target_id text NOT NULL CHECK (target_id IN ('p05','p07','p09')),
    revision bigint NOT NULL CHECK (revision >= 0),
    manifest platform.digest32 NOT NULL,
    PRIMARY KEY (scope_ref,target_id)
);
CREATE TABLE platform.recovery_target_ack (
    scope_ref uuid NOT NULL,
    target_id text NOT NULL,
    job_id uuid NOT NULL,
    plan_digest platform.digest32 NOT NULL,
    signed_receipt jsonb NOT NULL,
    PRIMARY KEY (scope_ref,target_id,job_id),
    FOREIGN KEY (scope_ref,target_id) REFERENCES platform.recovery_target_route
);
DO $roles$
DECLARE role_name text;
BEGIN
    FOREACH role_name IN ARRAY ARRAY['nu54_p10_recovery_coordinator','nu54_p10_recovery_target','nu54_p10_recovery_status',
        'nu54_p10_recovery_route_only']
    LOOP
        IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname=role_name) THEN
            EXECUTE format('CREATE ROLE %I NOLOGIN NOSUPERUSER NOBYPASSRLS',role_name);
        ELSIF EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname=role_name
          AND (rolcanlogin OR rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication OR rolbypassrls)) THEN
            RAISE EXCEPTION 'unsafe recovery capability role';
        END IF;
    END LOOP;
END
$roles$;
GRANT USAGE ON SCHEMA platform TO nu54_p10_recovery_coordinator,nu54_p10_recovery_target,nu54_p10_recovery_status;
GRANT USAGE ON DOMAIN platform.digest32 TO nu54_p10_recovery_coordinator,nu54_p10_recovery_target;
GRANT SELECT ON platform.recovery_route,platform.recovery_run,platform.recovery_target_receipt TO nu54_p10_recovery_status;
GRANT SELECT,UPDATE ON platform.recovery_route TO nu54_p10_recovery_coordinator;
GRANT SELECT,INSERT,UPDATE ON platform.recovery_run TO nu54_p10_recovery_coordinator;
GRANT SELECT,INSERT ON platform.recovery_target_receipt TO nu54_p10_recovery_coordinator;
GRANT SELECT,UPDATE ON platform.recovery_target_route TO nu54_p10_recovery_target;
GRANT SELECT,INSERT ON platform.recovery_target_ack TO nu54_p10_recovery_target;

GRANT USAGE ON SCHEMA platform TO nu54_p10_recovery_route_only;
GRANT SELECT(scope_ref,active_job,current_manifest) ON platform.recovery_route TO nu54_p10_recovery_route_only;

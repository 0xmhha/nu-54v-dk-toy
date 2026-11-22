-- Fleet placement extends the P10 release authority with independently signed
-- work and installer receipts. Build and promotion records remain authoritative.
CREATE TABLE platform.fleet_candidate (
    id text PRIMARY KEY CHECK (id ~ '^[0-9a-f]{64}$'),
    signer text NOT NULL CHECK (signer ~ '^[0-9a-f]{64}$'),
    metadata text NOT NULL
);
CREATE TABLE platform.fleet_cohort (
    id uuid PRIMARY KEY,
    revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
    current text REFERENCES platform.fleet_candidate,
    pending uuid,
    active_job uuid
);
CREATE TABLE platform.fleet_operation (
    id uuid PRIMARY KEY,
    cohort uuid NOT NULL REFERENCES platform.fleet_cohort,
    bundle text NOT NULL,
    result text CHECK (result ~ '^[0-9a-f]{64}$'),
    UNIQUE (cohort,id)
);
ALTER TABLE platform.fleet_cohort ADD FOREIGN KEY (id,pending) REFERENCES platform.fleet_operation(cohort,id);
ALTER TABLE platform.fleet_cohort ADD FOREIGN KEY (id,active_job) REFERENCES platform.fleet_operation(cohort,id);
CREATE TABLE platform.fleet_receipt (
    job uuid NOT NULL REFERENCES platform.fleet_operation,
    target text NOT NULL,
    claim text NOT NULL,
    PRIMARY KEY(job,target)
);
CREATE TABLE platform.fleet_target (
    cohort uuid NOT NULL REFERENCES platform.fleet_cohort,
    name text NOT NULL,
    revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
    current text REFERENCES platform.fleet_candidate,
    PRIMARY KEY(cohort,name)
);
CREATE TABLE platform.fleet_revocation (
    cohort uuid NOT NULL REFERENCES platform.fleet_cohort,
    key text NOT NULL CHECK (key ~ '^[0-9a-f]{64}$'),
    approval text NOT NULL,
    PRIMARY KEY(cohort,key)
);
CREATE TABLE platform.fleet_advisory (
    id uuid PRIMARY KEY,
    report text NOT NULL
);
DO $roles$
DECLARE role_name text;
BEGIN
    FOREACH role_name IN ARRAY ARRAY['nu54_p10_fleet_service','nu54_p10_fleet_catalog']
    LOOP
        IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname=role_name) THEN
            EXECUTE format('CREATE ROLE %I NOLOGIN NOSUPERUSER NOBYPASSRLS',role_name);
        ELSIF EXISTS (SELECT FROM pg_roles WHERE rolname=role_name
            AND (rolcanlogin OR rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication OR rolbypassrls)) THEN
            RAISE EXCEPTION 'unsafe fleet capability role';
        END IF;
    END LOOP;
END
$roles$;
GRANT USAGE ON SCHEMA platform TO nu54_p10_fleet_service,nu54_p10_fleet_catalog;
GRANT USAGE ON DOMAIN platform.digest32 TO nu54_p10_fleet_service;
GRANT SELECT, INSERT ON platform.build_release,platform.test_evidence,platform.promotion TO nu54_p10_fleet_service;
GRANT SELECT, INSERT, UPDATE ON platform.advisory_case TO nu54_p10_fleet_service;
GRANT SELECT, INSERT ON platform.fleet_candidate,platform.fleet_receipt,platform.fleet_revocation TO nu54_p10_fleet_service;
GRANT SELECT, INSERT, UPDATE ON platform.fleet_cohort,platform.fleet_operation,platform.fleet_target,platform.fleet_advisory TO nu54_p10_fleet_service;
GRANT SELECT ON platform.fleet_candidate,platform.fleet_cohort,platform.fleet_receipt,platform.fleet_revocation,
    platform.fleet_advisory TO nu54_p10_fleet_catalog;

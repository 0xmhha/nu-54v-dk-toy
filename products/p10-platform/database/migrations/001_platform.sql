-- PostgreSQL 15 reference model for P10, designed for a new scratch DB.
-- API credentials and signing/identity keys live in separate services.
CREATE SCHEMA platform;
SET LOCAL search_path = platform, pg_catalog;
CREATE DOMAIN digest32 AS bytea CHECK (octet_length(VALUE) = 32);

-- One immutable manifest binds component digests, channel, hardware, ABI,
-- chain domain and evidence; addresses and serials are not search keys.
CREATE TABLE build_release (
    manifest_digest digest32 PRIMARY KEY,
    product smallint NOT NULL CHECK (product BETWEEN 1 AND 10),
    version text NOT NULL CHECK (length(version) BETWEEN 1 AND 80),
    channel text NOT NULL CHECK (channel IN ('official','custom')),
    trust_root digest32 NOT NULL,
    spec_digest digest32 NOT NULL,
    sbom_digest digest32 NOT NULL,
    provenance_digest digest32 NOT NULL,
    UNIQUE (product, version, channel, trust_root)
);
CREATE TABLE test_evidence (
    manifest_digest digest32 NOT NULL REFERENCES build_release,
    combination_digest digest32 NOT NULL,
    test_kind text NOT NULL CHECK (test_kind IN ('vector','physical','install','regression','rescan','rebuild')),
    report_digest digest32 NOT NULL,
    passed boolean NOT NULL,
    tested_at timestamptz NOT NULL,
    PRIMARY KEY (manifest_digest, combination_digest, test_kind, report_digest)
);
CREATE TABLE advisory_case (
    advisory_ref uuid NOT NULL, manifest_digest digest32 NOT NULL REFERENCES build_release,
    component_ref text NOT NULL, owner_role text NOT NULL, next_review_at timestamptz NOT NULL,
    decision text NOT NULL CHECK (decision IN ('investigate','hold','mitigate','fixed','unaffected')),
    evidence_digest digest32,
    fixed_manifest digest32 REFERENCES build_release,
    PRIMARY KEY (advisory_ref, manifest_digest, component_ref),
    CHECK (decision <> 'fixed' OR (fixed_manifest IS NOT NULL AND fixed_manifest <> manifest_digest)),
    CHECK (decision = 'investigate' OR evidence_digest IS NOT NULL)
);
CREATE TABLE promotion (
    manifest_digest digest32 NOT NULL REFERENCES build_release,
    combination_digest digest32 NOT NULL, evidence_set_digest digest32 NOT NULL,
    prepared_by uuid NOT NULL, approved_by uuid NOT NULL,
    published_receipt digest32 NOT NULL, published_at timestamptz NOT NULL,
    PRIMARY KEY (manifest_digest, combination_digest),
    CHECK (prepared_by <> approved_by)
);
-- Per scoped recovery, a signed verification bundle binds exact targets/checkpoints,
-- lease revision, agreed RPO/RTO, observed reconciliation and deletion watermark.
CREATE TABLE restore_job (
    job_id uuid PRIMARY KEY,
    target_scope_ref uuid NOT NULL, snapshot_ref uuid NOT NULL,
    expected_revision bigint NOT NULL CHECK (expected_revision >= 0),
    requested_by uuid NOT NULL, requested_at timestamptz NOT NULL,
    goal_rto_seconds integer NOT NULL CHECK (goal_rto_seconds >= 0),
    goal_rpo_seconds integer NOT NULL CHECK (goal_rpo_seconds >= 0),
    UNIQUE (job_id, requested_by)
);
CREATE TABLE restore_approval (
    job_id uuid PRIMARY KEY, requested_by uuid NOT NULL, verified_by uuid NOT NULL,
    proof_digest digest32 NOT NULL, approved_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL CHECK (expires_at > approved_at),
    measured_rto_seconds integer NOT NULL CHECK (measured_rto_seconds >= 0),
    measured_rpo_seconds integer NOT NULL CHECK (measured_rpo_seconds >= 0),
    FOREIGN KEY (job_id, requested_by) REFERENCES restore_job(job_id, requested_by),
    CHECK (requested_by <> verified_by)
);
CREATE TABLE cutover_receipt (
    job_id uuid PRIMARY KEY REFERENCES restore_approval,
    target_result_digest digest32 NOT NULL, applied_revision bigint NOT NULL CHECK (applied_revision > 0),
    applied_at timestamptz NOT NULL
);

REVOKE ALL ON SCHEMA platform FROM PUBLIC;
REVOKE ALL ON DOMAIN digest32 FROM PUBLIC;

-- Cluster-level capability roles. Deployments grant memberships to scoped logins.
DO $roles$
DECLARE role_name text;
BEGIN
    FOREACH role_name IN ARRAY ARRAY['nu54_p10_release_build', 'nu54_p10_release_approve', 'nu54_p10_restore_request', 'nu54_p10_restore_verify', 'nu54_p10_restore_apply']
    LOOP
        IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = role_name) THEN
            EXECUTE format('CREATE ROLE %I NOLOGIN', role_name);
        ELSIF EXISTS (
            SELECT FROM pg_catalog.pg_roles WHERE rolname = role_name
                AND (rolcanlogin OR rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication OR rolbypassrls)
        ) THEN
            RAISE EXCEPTION 'unsafe capability role %', role_name;
        END IF;
    END LOOP;
END
$roles$;

GRANT USAGE ON SCHEMA platform TO nu54_p10_release_build, nu54_p10_release_approve,
    nu54_p10_restore_request, nu54_p10_restore_verify, nu54_p10_restore_apply;
GRANT USAGE ON DOMAIN digest32 TO nu54_p10_release_build, nu54_p10_release_approve,
    nu54_p10_restore_request, nu54_p10_restore_verify, nu54_p10_restore_apply;
GRANT SELECT, INSERT ON build_release, test_evidence, advisory_case TO nu54_p10_release_build;
GRANT UPDATE (next_review_at, decision, evidence_digest, fixed_manifest) ON advisory_case TO nu54_p10_release_build;
GRANT SELECT ON build_release, test_evidence, advisory_case TO nu54_p10_release_approve;
GRANT SELECT, INSERT ON promotion TO nu54_p10_release_approve;
GRANT SELECT, INSERT ON restore_job TO nu54_p10_restore_request;
GRANT SELECT ON restore_job TO nu54_p10_restore_verify, nu54_p10_restore_apply;
GRANT SELECT, INSERT ON restore_approval TO nu54_p10_restore_verify;
GRANT SELECT ON restore_approval TO nu54_p10_restore_apply;
GRANT SELECT, INSERT ON cutover_receipt TO nu54_p10_restore_apply;

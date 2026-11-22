-- Design of the restricted audit plane. Each controller deploys its own scope.
-- Object storage/keys, authenticated writers and retention enforcement are separate.
CREATE SCHEMA privacy_ops;
CREATE TABLE privacy_ops.retention_rule (
    rule_id uuid PRIMARY KEY,
    class_code text NOT NULL CHECK (length(class_code) BETWEEN 1 AND 64),
    version integer NOT NULL CHECK (version > 0),
    purpose_code text NOT NULL, basis_ref text NOT NULL, owner_role text NOT NULL,
    trigger_event text NOT NULL, period_ref text NOT NULL, disposal_method text NOT NULL,
    effective_at timestamptz NOT NULL, UNIQUE(class_code, version)
);
CREATE TABLE privacy_ops.access_record (
    access_id uuid PRIMARY KEY, system_ref uuid NOT NULL, actor_ref uuid NOT NULL,
    occurred_at timestamptz NOT NULL, source_locator_cipher bytea NOT NULL,
    task_code text NOT NULL, outcome text NOT NULL CHECK (outcome IN ('allowed','denied','failed')),
    -- For operations affecting subjects this names a sealed, expiry-bound list.
    subject_scope_receipt uuid, affects_subject boolean NOT NULL,
    downloaded boolean NOT NULL, rule_id uuid NOT NULL REFERENCES privacy_ops.retention_rule,
    keep_until timestamptz NOT NULL CHECK (isfinite(occurred_at) AND isfinite(keep_until) AND keep_until > occurred_at),
    CHECK ((subject_scope_receipt IS NOT NULL) = affects_subject),
    CHECK (octet_length(source_locator_cipher) > 0)
);
CREATE INDEX access_time ON privacy_ops.access_record(system_ref, occurred_at);
-- Tenant/product-local rights handling. No universal subject identifier or raw credentials.
CREATE TABLE privacy_ops.rights_case (
    case_id uuid PRIMARY KEY, service_scope uuid NOT NULL, local_claim_ref uuid NOT NULL,
    request_type text NOT NULL CHECK (request_type IN ('read','correct','erase','stop','withdraw','transfer','appeal')),
    authorized_scope_digest bytea NOT NULL CHECK (octet_length(authorized_scope_digest)=32),
    received_at timestamptz NOT NULL, due_at timestamptz NOT NULL CHECK(due_at >= received_at),
    state text NOT NULL CHECK (state IN ('verifying','working','partial','refused','complete')),
    basis_ref text, result_receipt uuid, notice_at timestamptz,
    CHECK (state <> 'refused' OR basis_ref IS NOT NULL),
    CHECK (state NOT IN ('complete','refused') OR (result_receipt IS NOT NULL AND notice_at IS NOT NULL))
);

-- Rule labels identify reviewed policy, never free-form request or customer data.
ALTER TABLE privacy_ops.retention_rule
    ADD CHECK (btrim(class_code) <> '' AND btrim(purpose_code) <> '' AND btrim(basis_ref) <> ''
        AND btrim(owner_role) <> '' AND btrim(trigger_event) <> '' AND btrim(period_ref) <> ''
        AND btrim(disposal_method) <> '');
ALTER TABLE privacy_ops.rights_case
    ADD COLUMN rule_id uuid NOT NULL REFERENCES privacy_ops.retention_rule,
    ADD COLUMN keep_until timestamptz NOT NULL,
    ADD CHECK (isfinite(received_at) AND isfinite(keep_until) AND keep_until > received_at);

REVOKE ALL ON SCHEMA privacy_ops FROM PUBLIC;

-- Cluster-level capability roles. Deployments grant memberships to scoped logins.
DO $roles$
DECLARE role_name text;
BEGIN
    FOREACH role_name IN ARRAY ARRAY['nu54_p10_audit_writer', 'nu54_p10_audit_reader', 'nu54_p10_retention_admin', 'nu54_p10_rights']
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

GRANT USAGE ON SCHEMA privacy_ops TO nu54_p10_audit_writer, nu54_p10_audit_reader,
    nu54_p10_retention_admin, nu54_p10_rights;
GRANT INSERT ON privacy_ops.access_record TO nu54_p10_audit_writer;
GRANT SELECT ON privacy_ops.access_record TO nu54_p10_audit_reader;
GRANT SELECT ON privacy_ops.retention_rule TO nu54_p10_audit_reader, nu54_p10_rights;
GRANT SELECT, INSERT ON privacy_ops.retention_rule TO nu54_p10_retention_admin;
GRANT SELECT, INSERT ON privacy_ops.rights_case TO nu54_p10_rights;
GRANT UPDATE (state, basis_ref, result_receipt, notice_at) ON privacy_ops.rights_case TO nu54_p10_rights;

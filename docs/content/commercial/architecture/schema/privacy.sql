-- Design of the restricted audit plane. Each controller deploys its own scope.
-- Object storage/keys, authenticated writers and retention enforcement are separate.
BEGIN;
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
    keep_until timestamptz NOT NULL CHECK (keep_until > occurred_at),
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
COMMIT;

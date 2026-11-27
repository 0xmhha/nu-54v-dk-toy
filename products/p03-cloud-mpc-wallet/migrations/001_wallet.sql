CREATE SCHEMA wallet_mpc;
REVOKE ALL ON SCHEMA wallet_mpc FROM PUBLIC;

CREATE TABLE wallet_mpc.identity (
    subject_ref uuid PRIMARY KEY,
    enrolled_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE wallet_mpc.auth_link (
    subject_ref uuid NOT NULL REFERENCES wallet_mpc.identity,
    credential_ref uuid NOT NULL UNIQUE,
    linked_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    unlinked_at timestamptz,
    PRIMARY KEY (subject_ref, credential_ref)
);
CREATE TABLE wallet_mpc.wallet (
    wallet_id uuid PRIMARY KEY,
    subject_ref uuid NOT NULL UNIQUE REFERENCES wallet_mpc.identity,
    profile text NOT NULL CHECK (profile = 'sepolia-native-test-v1'),
    active_epoch bigint CHECK (active_epoch > 0),
    public_key bytea CHECK (octet_length(public_key) = 65),
    state text NOT NULL CHECK (state IN ('creating','active','blocked','failed','closed')),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CHECK ((active_epoch IS NULL) = (public_key IS NULL)),
    CHECK (state <> 'active' OR public_key IS NOT NULL),
    CHECK (state NOT IN ('creating','failed') OR public_key IS NULL)
);
CREATE TABLE wallet_mpc.operation (
    operation_id uuid PRIMARY KEY,
    wallet_id uuid NOT NULL REFERENCES wallet_mpc.wallet,
    kind text NOT NULL CHECK (kind IN ('dkg','sign','refresh')),
    epoch bigint NOT NULL CHECK (epoch > 0),
    session jsonb NOT NULL,
    fingerprint bytea NOT NULL UNIQUE CHECK (octet_length(fingerprint) = 32),
    state text NOT NULL CHECK (state IN ('reserved','running','succeeded','failed','held','expired')),
    intent jsonb,
    approval_proof bytea CHECK (octet_length(approval_proof) = 64),
    result_digest bytea CHECK (octet_length(result_digest) = 32),
    not_before timestamptz NOT NULL,
    expires_at timestamptz NOT NULL CHECK (expires_at > not_before),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CHECK (approval_proof IS NULL OR state = 'running')
);
CREATE UNIQUE INDEX wallet_pending_operation ON wallet_mpc.operation(wallet_id)
WHERE kind IN ('dkg','refresh') AND state IN ('reserved','running');

CREATE TABLE wallet_mpc.key_epoch (
    wallet_id uuid NOT NULL REFERENCES wallet_mpc.wallet,
    epoch bigint NOT NULL CHECK (epoch > 0),
    operation_id uuid NOT NULL UNIQUE REFERENCES wallet_mpc.operation,
    activated_at timestamptz NOT NULL,
    retired_at timestamptz,
    PRIMARY KEY (wallet_id,epoch),
    CHECK (retired_at IS NULL OR retired_at >= activated_at)
);
CREATE UNIQUE INDEX wallet_active_epoch ON wallet_mpc.key_epoch(wallet_id) WHERE retired_at IS NULL;
ALTER TABLE wallet_mpc.wallet ADD FOREIGN KEY (wallet_id,active_epoch) REFERENCES wallet_mpc.key_epoch;

CREATE TABLE wallet_mpc.participant_ack (
    operation_id uuid NOT NULL REFERENCES wallet_mpc.operation,
    participant_ref text NOT NULL CHECK (length(participant_ref) BETWEEN 2 AND 64),
    evidence_kind text NOT NULL CHECK (evidence_kind IN ('ready','retired','fenced','signed')),
    evidence_digest bytea NOT NULL CHECK (octet_length(evidence_digest) = 32),
    received_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(operation_id,participant_ref,evidence_kind)
);
CREATE TABLE wallet_mpc.signature_result (
    operation_id uuid PRIMARY KEY REFERENCES wallet_mpc.operation,
    signature bytea NOT NULL CHECK (octet_length(signature) BETWEEN 8 AND 72),
    recovery_id smallint NOT NULL CHECK (recovery_id IN (0,1)),
    expires_at timestamptz NOT NULL
);
CREATE TABLE wallet_mpc.audit_outbox (
    event_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    wallet_id uuid NOT NULL REFERENCES wallet_mpc.wallet,
    operation_id uuid REFERENCES wallet_mpc.operation,
    event_kind text NOT NULL CHECK (event_kind IN ('created','sign_finished','recovery_started',
        'epoch_activated','recovery_held','closed','incident_hold')),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    published_at timestamptz
);
CREATE INDEX wallet_audit_pending ON wallet_mpc.audit_outbox (created_at) WHERE published_at IS NULL;

DO $roles$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'nu54_p03_service') THEN
        CREATE ROLE nu54_p03_service NOLOGIN;
    END IF;
END
$roles$;
GRANT USAGE ON SCHEMA wallet_mpc TO nu54_p03_service;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA wallet_mpc TO nu54_p03_service;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA wallet_mpc TO nu54_p03_service;

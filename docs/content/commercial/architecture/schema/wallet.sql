-- P03 모델 검증용 PostgreSQL DDL. 배포 마이그레이션이 아니다.
-- P03 전용 IdP subject_ref, 프로필/정책 참조에는 원격 FK를 두지 않는다.
BEGIN;
CREATE SCHEMA wallet_mpc;
SET LOCAL search_path TO wallet_mpc, pg_catalog;

CREATE TABLE wallet (
    wallet_id uuid PRIMARY KEY,
    subject_ref uuid NOT NULL,
    create_request_id uuid NOT NULL,
    create_intent_hash bytea NOT NULL CHECK (octet_length(create_intent_hash) = 32),
    profile_ref text NOT NULL CHECK (length(profile_ref) BETWEEN 1 AND 128),
    policy_ref text NOT NULL CHECK (length(policy_ref) BETWEEN 1 AND 128),
    public_key bytea CHECK (octet_length(public_key) BETWEEN 1 AND 128),
    active_epoch bigint CHECK (active_epoch > 0),
    state text NOT NULL CHECK (state IN ('creating','active','blocked','failed','closed')),
    created_at timestamptz NOT NULL,
    UNIQUE (subject_ref, create_request_id),
    CHECK (state <> 'active' OR (public_key IS NOT NULL AND active_epoch IS NOT NULL)),
    CHECK (state NOT IN ('creating','failed') OR (public_key IS NULL AND active_epoch IS NULL))
);

CREATE TABLE key_epoch (
    wallet_id uuid NOT NULL REFERENCES wallet,
    epoch bigint NOT NULL CHECK (epoch > 0),
    state text NOT NULL CHECK (state IN ('preparing','active','retired','aborted')),
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL CHECK (expires_at > created_at),
    PRIMARY KEY (wallet_id, epoch)
);
CREATE UNIQUE INDEX one_active_epoch ON key_epoch (wallet_id) WHERE state = 'active';
CREATE UNIQUE INDEX one_preparing_epoch ON key_epoch (wallet_id) WHERE state = 'preparing';
ALTER TABLE wallet ADD FOREIGN KEY (wallet_id, active_epoch)
    REFERENCES key_epoch (wallet_id, epoch);

-- 접수 한 행은 대상 세대의 참여자 펜스와 은퇴로 완료 여부를 계산한다.
CREATE TABLE closure_receipt (
    wallet_id uuid PRIMARY KEY REFERENCES wallet,
    request_ref uuid NOT NULL,
    epoch bigint NOT NULL CHECK (epoch > 0),
    accepted_at timestamptz NOT NULL,
    FOREIGN KEY (wallet_id, epoch) REFERENCES key_epoch (wallet_id, epoch)
);

CREATE TABLE participant_ack (
    wallet_id uuid NOT NULL,
    epoch bigint NOT NULL,
    participant_role char(1) NOT NULL CHECK (participant_role IN ('A','B','C')),
    participant_ref uuid NOT NULL,
    activation_evidence_hash bytea CHECK (octet_length(activation_evidence_hash) = 32),
    ready_at timestamptz,
    ready_public_key_hash bytea CHECK (octet_length(ready_public_key_hash) = 32),
    PRIMARY KEY (wallet_id, epoch, participant_role),
    FOREIGN KEY (wallet_id, epoch) REFERENCES key_epoch (wallet_id, epoch),
    CHECK ((ready_at IS NULL) = (activation_evidence_hash IS NULL)),
    CHECK ((ready_at IS NULL) = (ready_public_key_hash IS NULL))
);

CREATE TABLE sign_request (
    wallet_id uuid NOT NULL,
    request_id uuid NOT NULL,
    epoch bigint NOT NULL,
    purpose text NOT NULL CHECK (length(purpose) BETWEEN 1 AND 32),
    intent_hash bytea NOT NULL CHECK (octet_length(intent_hash) = 32),
    approval_ref uuid NOT NULL UNIQUE,
    state text NOT NULL CHECK (state IN ('reserved','running','signed','failed','unknown','expired')),
    -- Canonical signature/transaction package capability in a restricted TTL store.
    result_ref uuid UNIQUE,
    result_digest bytea CHECK (octet_length(result_digest) = 32),
    result_expires_at timestamptz,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL CHECK (expires_at > created_at),
    PRIMARY KEY (wallet_id, request_id),
    FOREIGN KEY (wallet_id, epoch) REFERENCES key_epoch (wallet_id, epoch),
    CHECK ((state = 'signed') = (result_digest IS NOT NULL)),
    CHECK ((result_digest IS NULL) = (result_expires_at IS NULL)),
    CHECK (result_ref IS NULL OR state = 'signed'),
    CHECK (result_expires_at IS NULL OR result_expires_at > created_at)
);

CREATE TABLE recovery_request (
    wallet_id uuid NOT NULL,
    request_id uuid NOT NULL,
    from_epoch bigint NOT NULL,
    to_epoch bigint NOT NULL CHECK (to_epoch > from_epoch),
    intent_hash bytea NOT NULL CHECK (octet_length(intent_hash) = 32),
    approval_evidence_ref uuid UNIQUE,
    state text NOT NULL CHECK (state IN ('pending','approved','activating','completed','denied','expired')),
    created_at timestamptz NOT NULL,
    not_before timestamptz NOT NULL CHECK (not_before >= created_at),
    expires_at timestamptz NOT NULL CHECK (expires_at > not_before),
    PRIMARY KEY (wallet_id, request_id),
    UNIQUE (wallet_id, to_epoch),
    UNIQUE (wallet_id, request_id, from_epoch),
    FOREIGN KEY (wallet_id, from_epoch) REFERENCES key_epoch (wallet_id, epoch),
    FOREIGN KEY (wallet_id, to_epoch) REFERENCES key_epoch (wallet_id, epoch),
    CHECK (state NOT IN ('approved','activating','completed') OR approval_evidence_ref IS NOT NULL)
);
CREATE UNIQUE INDEX one_inflight_recovery ON recovery_request (wallet_id)
    WHERE state IN ('pending','approved','activating');

CREATE TABLE old_epoch_fence_ack (
    wallet_id uuid NOT NULL,
    recovery_request_id uuid NOT NULL,
    from_epoch bigint NOT NULL,
    participant_role char(1) NOT NULL CHECK (participant_role IN ('A','B','C')),
    fence_evidence_hash bytea NOT NULL CHECK (octet_length(fence_evidence_hash) = 32),
    fenced_at timestamptz NOT NULL,
    PRIMARY KEY (wallet_id, recovery_request_id, participant_role),
    FOREIGN KEY (wallet_id, recovery_request_id, from_epoch)
        REFERENCES recovery_request (wallet_id, request_id, from_epoch),
    FOREIGN KEY (wallet_id, from_epoch, participant_role)
        REFERENCES participant_ack (wallet_id, epoch, participant_role)
);

CREATE TABLE audit_outbox (
    event_id uuid PRIMARY KEY,
    wallet_id uuid NOT NULL REFERENCES wallet,
    event_kind text NOT NULL CHECK (event_kind IN
        ('created','sign_finished','recovery_started','epoch_activated','recovery_held','closed','incident_hold')),
    reason_code text CHECK (length(reason_code) BETWEEN 1 AND 64),
    created_at timestamptz NOT NULL,
    published_at timestamptz CHECK (published_at >= created_at)
);
CREATE INDEX audit_outbox_pending ON audit_outbox (created_at) WHERE published_at IS NULL;
COMMIT;

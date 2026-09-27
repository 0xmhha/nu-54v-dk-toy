-- P05 back-office store ([N20]). Designed for opsd; opsctl writes the audit table.
CREATE TABLE IF NOT EXISTS merchants (
    merchant        TEXT PRIMARY KEY,
    name            TEXT        NOT NULL,
    payout          TEXT        NOT NULL,
    status          TEXT        NOT NULL CHECK (status IN ('active', 'revoked')),
    registered_tx   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS rentals (
    device          TEXT PRIMARY KEY,
    withdraw_addr   TEXT        NOT NULL,
    state           TEXT        NOT NULL CHECK (state IN ('provisioning', 'active', 'returning', 'closed')),
    last_anchor     BIGINT,
    deposit_tx      TEXT,
    close_tx        TEXT,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS attestations (
    id              BIGSERIAL PRIMARY KEY,
    merchant        TEXT        NOT NULL REFERENCES merchants(merchant),
    payout          TEXT        NOT NULL,
    valid_from      BIGINT      NOT NULL,
    valid_until     BIGINT      NOT NULL,
    signature       TEXT        NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_log (
    id              BIGSERIAL PRIMARY KEY,
    at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor_role      TEXT        NOT NULL CHECK (actor_role IN ('operator', 'registry_admin', 'kiosk', 'test_merchant')),
    action          TEXT        NOT NULL,
    subject         TEXT        NOT NULL,
    tx_hash         TEXT,
    detail          JSONB       NOT NULL DEFAULT '{}'
);

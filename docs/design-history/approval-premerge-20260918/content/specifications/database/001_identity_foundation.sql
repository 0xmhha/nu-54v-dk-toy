BEGIN;
-- Reference design only. Apply only to an isolated empty review database.
CREATE SCHEMA wallet_platform;
SET search_path = wallet_platform, pg_catalog;
CREATE DOMAIN app_id AS text CHECK (VALUE ~ '^[A-Za-z0-9_-]{1,128}$');
CREATE DOMAIN evm_address AS text CHECK (VALUE ~ '^0x[0-9a-f]{40}$');
CREATE DOMAIN hash32 AS text CHECK (VALUE ~ '^0x[0-9a-f]{64}$');
CREATE DOMAIN uint256_amount AS numeric CHECK (VALUE >= 0 AND VALUE = trunc(VALUE) AND VALUE <= 115792089237316195423570985008687907853269984665640564039457584007913129639935);
CREATE DOMAIN atomic_total AS numeric CHECK (VALUE >= 0 AND VALUE = trunc(VALUE) AND VALUE < 'Infinity'::numeric);
CREATE TABLE schema_migrations(version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE accounts (
  id app_id PRIMARY KEY,
  status text NOT NULL CHECK (status IN ('active','deletion_pending','deactivated')),
  display_name_ref text,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


CREATE TABLE auth_identities (
  id app_id PRIMARY KEY,
  account_id app_id NOT NULL REFERENCES accounts(id),
  provider text NOT NULL CHECK (provider IN ('google','apple')),
  provider_subject text NOT NULL,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (provider, provider_subject)
);


CREATE TABLE auth_sessions (
  id app_id PRIMARY KEY,
  account_id app_id NOT NULL REFERENCES accounts(id),
  credential_hash text NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  auth_flow_ref text,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


CREATE TABLE stores (
  id app_id PRIMARY KEY,
  display_name text NOT NULL,
  timezone text NOT NULL,
  state text NOT NULL CHECK (state IN ('active','suspended','closed')),
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


CREATE TABLE memberships (
  id app_id PRIMARY KEY,
  store_id app_id NOT NULL REFERENCES stores(id),
  account_id app_id NOT NULL REFERENCES accounts(id),
  role_code text NOT NULL,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (store_id, account_id)
);


CREATE TABLE protocol_profiles (
  id app_id PRIMARY KEY,
  version text NOT NULL,
  kind text NOT NULL,
  schema_digest text NOT NULL,
  verifier_ref text NOT NULL,
  state text NOT NULL CHECK (state IN ('draft','enabled','revoked')),
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


CREATE TABLE assets (
  id app_id PRIMARY KEY,
  chain_id bigint NOT NULL CHECK (chain_id = 8283),
  kind text NOT NULL CHECK (kind IN ('native','erc20')),
  token_address evm_address,
  decimals smallint NOT NULL CHECK (decimals BETWEEN 0 AND 255),
  display_symbol text NOT NULL,
  is_test_asset boolean NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  CHECK ((kind = 'native' AND token_address IS NULL) OR (kind = 'erc20' AND token_address IS NOT NULL)),
  UNIQUE (id, chain_id)
);
CREATE UNIQUE INDEX assets_native_unique ON assets(chain_id) WHERE kind = 'native';
CREATE UNIQUE INDEX assets_token_unique ON assets(chain_id, token_address) WHERE kind = 'erc20';

CREATE TABLE operations (
  id app_id PRIMARY KEY,
  account_id app_id REFERENCES accounts(id),
  principal_scope text NOT NULL,
  kind text NOT NULL,
  state text NOT NULL CHECK (state IN ('queued','running','succeeded','failed','unknown','cancelled')),
  result_ref text,
  safe_error_code text,
  updated_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


CREATE TABLE idempotency_records (
  id app_id PRIMARY KEY,
  principal_scope text NOT NULL,
  action text NOT NULL,
  request_key text NOT NULL,
  normalized_digest text NOT NULL,
  operation_id app_id REFERENCES operations(id),
  result_ref text,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (principal_scope, action, request_key)
);


CREATE TABLE outbox (
  id app_id PRIMARY KEY,
  aggregate_type text NOT NULL,
  aggregate_id app_id NOT NULL,
  aggregate_revision bigint NOT NULL CHECK (aggregate_revision >= 0),
  event_type text NOT NULL,
  safe_payload jsonb NOT NULL,
  correlation_id app_id NOT NULL,
  published_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);
CREATE INDEX outbox_unpublished ON outbox(created_at,id) WHERE published_at IS NULL;

CREATE TABLE inbox (
  id app_id PRIMARY KEY,
  consumer_id text NOT NULL,
  event_id app_id NOT NULL,
  processed_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (consumer_id, event_id)
);


CREATE TABLE audit_events (
  id app_id PRIMARY KEY,
  actor_ref text NOT NULL,
  subject_type text NOT NULL,
  subject_id app_id NOT NULL,
  action text NOT NULL,
  safe_diff_ref text,
  reason text,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


INSERT INTO schema_migrations(version) VALUES ('001');
COMMIT;

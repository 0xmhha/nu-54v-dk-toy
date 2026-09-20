BEGIN;
SET search_path = wallet_platform, pg_catalog;
CREATE TABLE credential_metadata (
  id app_id PRIMARY KEY,
  account_id app_id NOT NULL REFERENCES accounts(id),
  issuer_ref text NOT NULL,
  subject_ref text NOT NULL,
  profile_id app_id NOT NULL REFERENCES protocol_profiles(id),
  private_content_ref text NOT NULL,
  status text NOT NULL CHECK (status IN ('active','expired','revoked','unknown')),
  expires_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


CREATE TABLE credential_status_history (
  id app_id PRIMARY KEY,
  credential_id app_id NOT NULL REFERENCES credential_metadata(id),
  status_version bigint NOT NULL CHECK (status_version >= 0),
  status text NOT NULL,
  evidence_ref text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (credential_id,status_version)
);


CREATE TABLE paid_resource_requests (
  id app_id PRIMARY KEY,
  account_id app_id NOT NULL REFERENCES accounts(id),
  wallet_id app_id NOT NULL REFERENCES wallets(id),
  resource_ref text NOT NULL,
  profile_id app_id NOT NULL REFERENCES protocol_profiles(id),
  price_snapshot jsonb NOT NULL,
  payment_state text NOT NULL,
  delivery_state text NOT NULL CHECK (delivery_state IN ('awaiting_payment','processing','delivered','failed')),
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


CREATE TABLE entitlements (
  id app_id PRIMARY KEY,
  request_id app_id NOT NULL UNIQUE REFERENCES paid_resource_requests(id),
  payment_evidence_ref text NOT NULL UNIQUE,
  resource_result_ref text,
  state text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


CREATE TABLE market_quotes (
  id app_id PRIMARY KEY,
  market_ref text NOT NULL,
  profile_id app_id NOT NULL REFERENCES protocol_profiles(id),
  action text NOT NULL,
  quote_snapshot jsonb NOT NULL,
  price_source_ref text NOT NULL,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


CREATE TABLE chain_projections (
  id app_id PRIMARY KEY,
  chain_id bigint NOT NULL CHECK (chain_id = 8283),
  profile_id app_id NOT NULL REFERENCES protocol_profiles(id),
  entity_type text NOT NULL,
  entity_key text NOT NULL,
  observed_block_hash hash32 NOT NULL,
  observed_block_number numeric NOT NULL CHECK (observed_block_number >= 0 AND observed_block_number = trunc(observed_block_number)),
  projection jsonb NOT NULL,
  validity text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (chain_id,profile_id,entity_type,entity_key)
);


CREATE TABLE keeper_jobs (
  id app_id PRIMARY KEY,
  market_ref text NOT NULL,
  action text NOT NULL,
  interval_key text NOT NULL,
  operation_id app_id NOT NULL REFERENCES operations(id),
  submission_id app_id REFERENCES transaction_submissions(id),
  state text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (market_ref,action,interval_key)
);


INSERT INTO schema_migrations(version) VALUES ('006');
COMMIT;

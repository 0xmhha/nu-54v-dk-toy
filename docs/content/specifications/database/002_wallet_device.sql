BEGIN;
SET search_path = wallet_platform, pg_catalog;
CREATE TABLE wallets (
  id app_id PRIMARY KEY,
  kind text NOT NULL CHECK (kind IN ('hardware_eoa','cloud_mpc_eoa','smart_account')),
  chain_id bigint NOT NULL CHECK (chain_id = 8283),
  address evm_address NOT NULL,
  signer_ref text,
  profile_id app_id REFERENCES protocol_profiles(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (id, chain_id)
);


CREATE TABLE wallet_bindings (
  id app_id PRIMARY KEY,
  wallet_id app_id NOT NULL REFERENCES wallets(id),
  account_id app_id REFERENCES accounts(id),
  store_id app_id REFERENCES stores(id),
  permission_profile text NOT NULL,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  CHECK ((account_id IS NOT NULL)::int + (store_id IS NOT NULL)::int = 1)
);
CREATE UNIQUE INDEX wallet_account_active ON wallet_bindings(wallet_id,account_id) WHERE account_id IS NOT NULL AND revoked_at IS NULL;
CREATE UNIQUE INDEX wallet_store_active ON wallet_bindings(wallet_id,store_id) WHERE store_id IS NOT NULL AND revoked_at IS NULL;

CREATE TABLE devices (
  id app_id PRIMARY KEY,
  model text NOT NULL,
  firmware_version text,
  identity_verifier_ref text,
  state text NOT NULL CHECK (state IN ('unprovisioned','assigned','active','return_pending','quarantined')),
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


CREATE TABLE rentals (
  id app_id PRIMARY KEY,
  device_id app_id NOT NULL REFERENCES devices(id),
  account_id app_id NOT NULL REFERENCES accounts(id),
  state text NOT NULL CHECK (state IN ('assigned','active','return_requested','return_pending','returned')),
  returned_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (id, device_id, account_id),
  CHECK ((state = 'returned') = (returned_at IS NOT NULL))
);
CREATE UNIQUE INDEX one_active_rental ON rentals(device_id) WHERE state <> 'returned';

CREATE TABLE device_bindings (
  id app_id PRIMARY KEY,
  device_id app_id NOT NULL REFERENCES devices(id),
  account_id app_id NOT NULL REFERENCES accounts(id),
  rental_id app_id,
  wallet_id app_id REFERENCES wallets(id),
  wallet_origin text NOT NULL CHECK (wallet_origin IN ('new_travel','imported')),
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  FOREIGN KEY (rental_id,device_id,account_id) REFERENCES rentals(id,device_id,account_id),
  UNIQUE (id,device_id,account_id),
  UNIQUE (id,rental_id,wallet_origin)
);
CREATE UNIQUE INDEX one_active_device_owner ON device_bindings(device_id) WHERE revoked_at IS NULL;

CREATE TABLE enrollments (
  id app_id PRIMARY KEY,
  device_id app_id NOT NULL REFERENCES devices(id),
  account_id app_id NOT NULL REFERENCES accounts(id),
  challenge_hash text NOT NULL,
  profile_id app_id NOT NULL REFERENCES protocol_profiles(id),
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


CREATE TABLE mpc_participants (
  id app_id PRIMARY KEY,
  wallet_id app_id NOT NULL REFERENCES wallets(id),
  participant_ref text NOT NULL,
  share_store_ref text NOT NULL,
  profile_id app_id NOT NULL REFERENCES protocol_profiles(id),
  state text NOT NULL CHECK (state IN ('pending','active','revoked')),
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (wallet_id,participant_ref)
);


CREATE TABLE signing_sessions (
  id app_id PRIMARY KEY,
  wallet_id app_id NOT NULL REFERENCES wallets(id),
  operation_id app_id NOT NULL REFERENCES operations(id),
  profile_id app_id NOT NULL REFERENCES protocol_profiles(id),
  payload_digest hash32 NOT NULL,
  state text NOT NULL CHECK (state IN ('pending','signing','completed','aborted','failed')),
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


INSERT INTO schema_migrations(version) VALUES ('002');
COMMIT;

BEGIN;
SET search_path = wallet_platform, pg_catalog;
CREATE TABLE return_checks (
  id app_id PRIMARY KEY,
  rental_id app_id NOT NULL REFERENCES rentals(id),
  binding_id app_id NOT NULL REFERENCES device_bindings(id),
  check_version bigint NOT NULL CHECK (check_version >= 0),
  wallet_origin text NOT NULL CHECK (wallet_origin IN ('new_travel','imported')),
  policy_id text NOT NULL,
  checks jsonb NOT NULL,
  state text NOT NULL CHECK (state IN ('pending','eligible_for_reset','reset_confirmed','completed')),
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (rental_id,check_version),
  FOREIGN KEY (binding_id,rental_id,wallet_origin) REFERENCES device_bindings(id,rental_id,wallet_origin)
);


CREATE TABLE reset_clearances (
  id app_id PRIMARY KEY,
  return_check_id app_id NOT NULL REFERENCES return_checks(id),
  challenge_hash text NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  owner_approval_ref text,
  consumed_at timestamptz,
  reset_evidence_ref text,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  CHECK (consumed_at IS NULL OR (owner_approval_ref IS NOT NULL AND reset_evidence_ref IS NOT NULL))
);


CREATE TABLE firmware_releases (
  id app_id PRIMARY KEY,
  model text NOT NULL,
  firmware_version text NOT NULL,
  package_hash hash32 NOT NULL,
  package_ref text NOT NULL,
  manifest jsonb NOT NULL,
  compatibility_profile app_id NOT NULL REFERENCES protocol_profiles(id),
  state text NOT NULL CHECK (state IN ('draft','published','withdrawn')),
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (model,firmware_version)
);


CREATE TABLE device_jobs (
  id app_id PRIMARY KEY,
  device_id app_id NOT NULL REFERENCES devices(id),
  release_id app_id REFERENCES firmware_releases(id),
  kind text NOT NULL CHECK (kind IN ('fota','reset','configuration')),
  state text NOT NULL,
  progress_ref text,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


INSERT INTO schema_migrations(version) VALUES ('004');
COMMIT;

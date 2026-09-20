BEGIN;
SET search_path = wallet_platform, pg_catalog;
CREATE TABLE consents (
  id app_id PRIMARY KEY,
  account_id app_id NOT NULL REFERENCES accounts(id),
  purpose text NOT NULL,
  version bigint NOT NULL CHECK (version >= 0),
  enabled boolean NOT NULL,
  superseded_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (account_id,purpose,version)
);
CREATE UNIQUE INDEX current_consent ON consents(account_id,purpose) WHERE superseded_at IS NULL;

CREATE TABLE recordings (
  id app_id PRIMARY KEY,
  account_id app_id NOT NULL REFERENCES accounts(id),
  format_profile app_id NOT NULL REFERENCES protocol_profiles(id),
  duration_seconds numeric NOT NULL CHECK (duration_seconds >= 0),
  completeness text NOT NULL CHECK (completeness IN ('complete','partial','failed')),
  cloud_object_ref text,
  access_blocked_at timestamptz,
  deleted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


CREATE TABLE processing_jobs (
  id app_id PRIMARY KEY,
  account_id app_id NOT NULL REFERENCES accounts(id),
  recording_id app_id REFERENCES recordings(id),
  operation_id app_id NOT NULL REFERENCES operations(id),
  consent_id app_id NOT NULL REFERENCES consents(id),
  kind text NOT NULL,
  state text NOT NULL CHECK (state IN ('queued','running','succeeded','failed','cancelled')),
  result_ref text,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


CREATE TABLE privacy_requests (
  id app_id PRIMARY KEY,
  account_id app_id NOT NULL REFERENCES accounts(id),
  kind text NOT NULL CHECK (kind IN ('export','delete','account_deletion')),
  scope jsonb NOT NULL,
  state text NOT NULL,
  operation_id app_id NOT NULL REFERENCES operations(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


CREATE TABLE data_lineage (
  id app_id PRIMARY KEY,
  owner_id app_id NOT NULL REFERENCES accounts(id),
  source_type text NOT NULL,
  source_id app_id NOT NULL,
  derived_type text NOT NULL,
  derived_id app_id NOT NULL,
  source_revision bigint NOT NULL CHECK (source_revision >= 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (source_type,source_id,derived_type,derived_id,source_revision)
);


INSERT INTO schema_migrations(version) VALUES ('005');
COMMIT;

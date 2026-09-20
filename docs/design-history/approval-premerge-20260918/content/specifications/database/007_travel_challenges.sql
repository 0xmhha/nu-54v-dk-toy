BEGIN;
SET search_path = wallet_platform, pg_catalog;
CREATE TABLE places (
  id app_id PRIMARY KEY,
  provider text NOT NULL,
  provider_place_id text NOT NULL,
  name text NOT NULL,
  latitude numeric CHECK (latitude BETWEEN -90 AND 90),
  longitude numeric CHECK (longitude BETWEEN -180 AND 180),
  source_observed_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (provider,provider_place_id)
);


CREATE TABLE store_place_links (
  id app_id PRIMARY KEY,
  store_id app_id NOT NULL REFERENCES stores(id),
  place_id app_id NOT NULL REFERENCES places(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (store_id,place_id)
);


CREATE TABLE reviews (
  id app_id PRIMARY KEY,
  account_id app_id NOT NULL REFERENCES accounts(id),
  place_id app_id NOT NULL REFERENCES places(id),
  allocation_id app_id REFERENCES payment_allocations(id),
  rating smallint NOT NULL CHECK (rating BETWEEN 1 AND 5),
  private_text_ref text NOT NULL,
  source_type text NOT NULL,
  visibility text NOT NULL,
  deleted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


CREATE TABLE trips (
  id app_id PRIMARY KEY,
  account_id app_id NOT NULL REFERENCES accounts(id),
  region_ref text NOT NULL,
  locale text NOT NULL,
  timezone text NOT NULL,
  state text NOT NULL,
  deleted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


CREATE TABLE travel_evidence (
  id app_id PRIMARY KEY,
  account_id app_id NOT NULL REFERENCES accounts(id),
  trip_id app_id NOT NULL REFERENCES trips(id),
  place_id app_id NOT NULL REFERENCES places(id),
  source_type text NOT NULL CHECK (source_type IN ('location_visit','testnet_payment','commercial_purchase','user_report')),
  source_ref text NOT NULL,
  validity text NOT NULL CHECK (validity IN ('pending','valid','invalidated','unknown')),
  consent_id app_id REFERENCES consents(id),
  observed_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


CREATE TABLE itineraries (
  id app_id PRIMARY KEY,
  account_id app_id NOT NULL REFERENCES accounts(id),
  trip_id app_id NOT NULL REFERENCES trips(id),
  constraints_snapshot jsonb NOT NULL,
  validated_stops jsonb NOT NULL,
  validation_state text NOT NULL,
  consent_id app_id NOT NULL REFERENCES consents(id),
  deleted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


CREATE TABLE participations (
  id app_id PRIMARY KEY,
  account_id app_id NOT NULL REFERENCES accounts(id),
  trip_id app_id NOT NULL REFERENCES trips(id),
  challenge_ref text NOT NULL,
  state text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (account_id,trip_id,challenge_ref)
);


CREATE TABLE challenge_entries (
  id app_id PRIMARY KEY,
  participation_id app_id NOT NULL REFERENCES participations(id),
  step_ref text NOT NULL,
  evidence_id app_id NOT NULL REFERENCES travel_evidence(id),
  evidence_revision bigint NOT NULL CHECK (evidence_revision >= 0),
  state text NOT NULL,
  reward_key text UNIQUE,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (participation_id,step_ref,evidence_id)
);


INSERT INTO schema_migrations(version) VALUES ('007');
COMMIT;

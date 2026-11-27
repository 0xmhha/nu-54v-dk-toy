-- P08 and P09 independently owned logical stores. PostgreSQL 14+.
-- Opaque cross-owner references are resolved and authorized by the issuing service.
BEGIN;
CREATE SCHEMA IF NOT EXISTS market;
CREATE SCHEMA IF NOT EXISTS travel;
-- Unconstrained numeric avoids numeric(p,0) silently rounding fractional inputs.
CREATE DOMAIN market.uint78 AS numeric CHECK (VALUE::text NOT IN ('NaN','Infinity','-Infinity') AND VALUE >= 0 AND VALUE < 1e78 AND trunc(VALUE) = VALUE);

CREATE TABLE market.venue (
  venue_id uuid PRIMARY KEY, chain_ref text NOT NULL CHECK (chain_ref = '8283'), contract_ref text NOT NULL,
  kind text NOT NULL CHECK (kind IN ('pool','fx','perpetual')),
  test_only boolean NOT NULL DEFAULT true CHECK (test_only),
  UNIQUE (chain_ref, contract_ref, kind)
);
CREATE TABLE market.price_version (
  price_id uuid PRIMARY KEY, venue_id uuid NOT NULL REFERENCES market.venue,
  source_ref text NOT NULL, base_asset_ref text NOT NULL, quote_asset_ref text NOT NULL,
  price_numerator market.uint78 NOT NULL CHECK (price_numerator > 0),
  price_denominator market.uint78 NOT NULL CHECK (price_denominator > 0),
  observed_at timestamptz NOT NULL, valid_until timestamptz NOT NULL,
  state text NOT NULL CHECK (state IN ('valid','stale','suspended')),
  UNIQUE (price_id, venue_id), CHECK (valid_until > observed_at)
);
CREATE TABLE market.quote (
  quote_id uuid PRIMARY KEY, venue_id uuid NOT NULL REFERENCES market.venue,
  price_id uuid NOT NULL,
  subject_ref text NOT NULL, operation text NOT NULL CHECK (operation IN ('swap','lp_add','lp_remove','fx','open','close')),
  input_asset_ref text NOT NULL, output_asset_ref text NOT NULL,
  input_units market.uint78 NOT NULL CHECK (input_units > 0), limit_units market.uint78 NOT NULL CHECK (limit_units >= 0),
  fee_units market.uint78 NOT NULL CHECK (fee_units >= 0), fee_asset_ref text NOT NULL,
  route_ref text, expires_at timestamptz NOT NULL, approved_at timestamptz,
  state text NOT NULL CHECK (state IN ('offered','approved','submitted','confirmed','rejected','expired')),
  created_at timestamptz NOT NULL DEFAULT now(), FOREIGN KEY (price_id, venue_id) REFERENCES market.price_version (price_id, venue_id), CHECK (input_asset_ref <> output_asset_ref)
);
-- Ordered asset legs encode both sides of LP operations and actual limits.
CREATE TABLE market.quote_leg (
  quote_id uuid NOT NULL REFERENCES market.quote ON DELETE CASCADE,
  leg_no smallint NOT NULL CHECK (leg_no BETWEEN 1 AND 4), asset_ref text NOT NULL,
  direction text NOT NULL CHECK (direction IN ('in','out')),
  quantity_units market.uint78 NOT NULL CHECK (quantity_units > 0),
  limit_units market.uint78 NOT NULL, PRIMARY KEY (quote_id, leg_no)
);
CREATE TABLE market.position_terms (
  quote_id uuid PRIMARY KEY REFERENCES market.quote ON DELETE CASCADE,
  direction text NOT NULL CHECK (direction IN ('long','short')),
  margin_asset_ref text NOT NULL, margin_units market.uint78 NOT NULL CHECK (margin_units > 0),
  position_units market.uint78 NOT NULL CHECK (position_units > 0),
  liquidation_policy_ref text NOT NULL, funding_policy_ref text NOT NULL
);
CREATE TABLE market.execution (
  execution_id uuid PRIMARY KEY, quote_id uuid NOT NULL UNIQUE REFERENCES market.quote,
  chain_ref text NOT NULL CHECK (chain_ref = '8283'), tx_hash text NOT NULL, state text NOT NULL CHECK (state IN ('prepared','submitted','unknown','confirmed','reorged','rejected')),
  result_ref text, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE (chain_ref, tx_hash)
);
CREATE TABLE market.position_cursor (
  venue_id uuid NOT NULL REFERENCES market.venue, position_ref text NOT NULL,
  revision bigint NOT NULL CHECK (revision >= 0), funding_cursor bigint NOT NULL CHECK (funding_cursor >= 0),
  state text NOT NULL CHECK (state IN ('open','closed','liquidated','unknown')),
  PRIMARY KEY (venue_id, position_ref)
);
CREATE TABLE market.keeper_job (
  job_id uuid PRIMARY KEY, venue_id uuid NOT NULL, position_ref text NOT NULL,
  kind text NOT NULL CHECK (kind IN ('funding','liquidation')), period_key text NOT NULL,
  expected_revision bigint NOT NULL, price_id uuid NOT NULL,
  chain_ref text NOT NULL CHECK (chain_ref = '8283'), tx_hash text NOT NULL,
  state text NOT NULL CHECK (state IN ('prepared','submitted','unknown','confirmed','reorged','rejected')),
  created_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (venue_id, position_ref) REFERENCES market.position_cursor,
  FOREIGN KEY (price_id, venue_id) REFERENCES market.price_version (price_id, venue_id),
  UNIQUE (venue_id, position_ref, kind, period_key), UNIQUE (chain_ref, tx_hash)
);

CREATE TABLE travel.place (
  place_id uuid PRIMARY KEY, kind text NOT NULL CHECK (kind IN ('restaurant','attraction')),
  name text NOT NULL, region text NOT NULL, address_text text, latitude_e6 integer, longitude_e6 integer,
  merchant_location_ref text, source_ref text NOT NULL, source_updated_at timestamptz NOT NULL,
  CHECK (latitude_e6 IS NULL OR latitude_e6 BETWEEN -90000000 AND 90000000),
  CHECK (longitude_e6 IS NULL OR longitude_e6 BETWEEN -180000000 AND 180000000)
);
CREATE TABLE travel.opening_interval (
  place_id uuid NOT NULL REFERENCES travel.place ON DELETE CASCADE, weekday smallint NOT NULL CHECK (weekday BETWEEN 0 AND 6),
  opens time NOT NULL, closes time NOT NULL, timezone text NOT NULL, source_updated_at timestamptz NOT NULL,
  PRIMARY KEY (place_id, weekday, opens), CHECK (closes > opens)
);
-- Consent is purpose-specific; version increment invalidates in-flight jobs.
CREATE TABLE travel.consent (
  subject_ref text NOT NULL, purpose text NOT NULL CHECK (purpose IN ('location','audio','transcription','recommendation','review_publication','challenge_evidence')),
  revision bigint NOT NULL CHECK (revision > 0), policy_version text NOT NULL, provider_ref text,
  granted boolean NOT NULL, decided_at timestamptz NOT NULL, expires_at timestamptz,
  PRIMARY KEY (subject_ref, purpose)
);
-- Append-only decision evidence; application role has INSERT/SELECT only.
CREATE TABLE travel.consent_decision (
  subject_ref text NOT NULL, purpose text NOT NULL, revision bigint NOT NULL CHECK (revision > 0),
  policy_version text NOT NULL, provider_ref text, granted boolean NOT NULL,
  decided_at timestamptz NOT NULL, expires_at timestamptz, evidence_ref text NOT NULL,
  PRIMARY KEY (subject_ref, purpose, revision),
  FOREIGN KEY (subject_ref, purpose) REFERENCES travel.consent
);
CREATE TABLE travel.record (
  record_id uuid PRIMARY KEY, subject_ref text NOT NULL, purpose text NOT NULL,
  consent_revision bigint NOT NULL, kind text NOT NULL CHECK (kind IN ('location','audio','transcript','summary','recommendation')),
  parent_record_id uuid, object_ref text NOT NULL, created_at timestamptz NOT NULL, delete_after timestamptz NOT NULL,
  state text NOT NULL CHECK (state IN ('active','deleting')),
  FOREIGN KEY (subject_ref, purpose, consent_revision) REFERENCES travel.consent_decision,
  UNIQUE (record_id, subject_ref), FOREIGN KEY (parent_record_id, subject_ref) REFERENCES travel.record (record_id, subject_ref),
  CHECK (delete_after > created_at), UNIQUE (object_ref),
  CHECK ((kind = 'location' AND purpose = 'location') OR (kind = 'audio' AND purpose = 'audio') OR
         (kind IN ('transcript','summary') AND purpose = 'transcription') OR
         (kind = 'recommendation' AND purpose = 'recommendation'))
);
CREATE TABLE travel.processing_job (
  job_id uuid PRIMARY KEY, subject_ref text NOT NULL, purpose text NOT NULL,
  consent_revision bigint NOT NULL, source_record_id uuid,
  state text NOT NULL CHECK (state IN ('queued','dispatching','in_flight','finished','cancel_pending','cancelled','failed')),
  created_at timestamptz NOT NULL, finished_at timestamptz,
  FOREIGN KEY (subject_ref, purpose, consent_revision) REFERENCES travel.consent_decision,
  FOREIGN KEY (source_record_id, subject_ref) REFERENCES travel.record (record_id, subject_ref)
);
-- Client-generated handle is durably stored before egress, including when receipt is lost.
CREATE TABLE travel.recipient_dispatch (
  job_id uuid NOT NULL REFERENCES travel.processing_job, recipient_ref text NOT NULL,
  recipient_handle text NOT NULL, fence_revision bigint NOT NULL,
  state text NOT NULL CHECK (state IN ('prepared','sent','unknown','finished','cancel_pending','delete_pending','deleted')),
  prepared_at timestamptz NOT NULL, finished_at timestamptz, deleted_at timestamptz,
  PRIMARY KEY (job_id, recipient_ref), UNIQUE (recipient_ref, recipient_handle)
);
-- Durable, content-free replay fence retained beyond backup restoration horizon.
CREATE TABLE travel.erasure_tombstone (
  subject_ref text NOT NULL, purpose text NOT NULL, through_revision bigint NOT NULL,
  erased_at timestamptz NOT NULL, retain_until timestamptz NOT NULL,
  PRIMARY KEY (subject_ref, purpose), CHECK (retain_until > erased_at)
);
-- Individual deletion survives restore without deleting siblings of the same decision.
CREATE TABLE travel.erased_object (
  object_kind text NOT NULL CHECK (object_kind IN ('record','review','itinerary','job')),
  object_token uuid NOT NULL, erased_at timestamptz NOT NULL,
  retain_until timestamptz NOT NULL CHECK (retain_until > erased_at),
  PRIMARY KEY (object_kind, object_token)
);
CREATE TABLE travel.delete_request (
  request_id uuid PRIMARY KEY, subject_ref text NOT NULL, purpose text NOT NULL,
  through_revision bigint NOT NULL CHECK (through_revision > 0),
  selection text NOT NULL CHECK (selection IN ('purpose','selected')),
  requested_at timestamptz NOT NULL, state text NOT NULL CHECK (state IN ('pending','complete','partial')),
  FOREIGN KEY (subject_ref, purpose) REFERENCES travel.consent
);
CREATE TABLE travel.delete_target (
  request_id uuid NOT NULL REFERENCES travel.delete_request ON DELETE CASCADE,
  target_kind text NOT NULL CHECK (target_kind IN ('device','server_original','server_derived','queue','provider','backup')),
  target_ref text NOT NULL, state text NOT NULL CHECK (state IN ('pending','complete','failed','expired')),
  attempted_at timestamptz, completed_at timestamptz, reason_code text,
  PRIMARY KEY (request_id, target_kind, target_ref)
);
CREATE TABLE travel.review (
  review_id uuid PRIMARY KEY, subject_ref text NOT NULL, place_id uuid NOT NULL REFERENCES travel.place,
  evidence_issuer_ref text NOT NULL, evidence_ref text NOT NULL, publication_consent_revision bigint NOT NULL, evidence_class text NOT NULL CHECK (evidence_class IN ('test','verified_real','unverified')),
  badge_state text NOT NULL CHECK (badge_state IN ('none','pending','verified','revoked')),
  publication_purpose text NOT NULL DEFAULT 'review_publication' CHECK (publication_purpose = 'review_publication'),
  publication_state text NOT NULL CHECK (publication_state IN ('draft','published','withdrawn')),
  body_object_ref text NOT NULL, created_at timestamptz NOT NULL, delete_after timestamptz NOT NULL,
  FOREIGN KEY (subject_ref, publication_purpose, publication_consent_revision) REFERENCES travel.consent_decision,
  UNIQUE (evidence_issuer_ref, evidence_ref), CHECK (delete_after > created_at),
  CHECK (badge_state <> 'verified' OR evidence_class = 'verified_real')
);
CREATE TABLE travel.itinerary (
  itinerary_id uuid PRIMARY KEY, subject_ref text NOT NULL, revision bigint NOT NULL CHECK (revision > 0),
  created_at timestamptz NOT NULL, delete_after timestamptz NOT NULL, CHECK (delete_after > created_at)
);
CREATE TABLE travel.itinerary_stop (
  itinerary_id uuid NOT NULL REFERENCES travel.itinerary ON DELETE CASCADE,
  ordinal integer NOT NULL CHECK (ordinal >= 0), place_id uuid NOT NULL REFERENCES travel.place,
  planned_at timestamptz, PRIMARY KEY (itinerary_id, ordinal)
);
CREATE TABLE travel.challenge_progress (
  challenge_ref text NOT NULL, subject_ref text NOT NULL, evidence_ref text NOT NULL,
  consent_purpose text NOT NULL DEFAULT 'challenge_evidence' CHECK (consent_purpose = 'challenge_evidence'),
  consent_revision bigint NOT NULL,
  state text NOT NULL CHECK (state IN ('pending','verified','revoked')),
  reward_ref text, updated_at timestamptz NOT NULL,
  FOREIGN KEY (subject_ref, consent_purpose, consent_revision) REFERENCES travel.consent_decision,
  PRIMARY KEY (challenge_ref, subject_ref, evidence_ref)
);
COMMIT;

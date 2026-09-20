BEGIN;
SET search_path = wallet_platform, pg_catalog;
CREATE TABLE recipient_configurations (
  id app_id PRIMARY KEY,
  store_id app_id NOT NULL REFERENCES stores(id),
  wallet_id app_id NOT NULL REFERENCES wallets(id),
  address evm_address NOT NULL,
  ownership_proof_ref text NOT NULL,
  retired_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (id, store_id)
);
CREATE UNIQUE INDEX current_store_recipient ON recipient_configurations(store_id) WHERE retired_at IS NULL;

CREATE TABLE menu_items (
  id app_id PRIMARY KEY,
  store_id app_id NOT NULL REFERENCES stores(id),
  name text NOT NULL,
  options_snapshot jsonb NOT NULL,
  price_minor atomic_total NOT NULL,
  currency text NOT NULL,
  fraction_digits smallint NOT NULL CHECK (fraction_digits BETWEEN 0 AND 18),
  availability text NOT NULL CHECK (availability IN ('available','sold_out','hidden')),
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (id,store_id)
);


CREATE TABLE orders (
  id app_id PRIMARY KEY,
  store_id app_id NOT NULL REFERENCES stores(id),
  account_id app_id REFERENCES accounts(id),
  state text NOT NULL CHECK (state IN ('awaiting_payment','paid','cancelled','expired','fulfilled')),
  recipient_config_id app_id NOT NULL,
  recipient_snapshot evm_address NOT NULL,
  price_minor atomic_total NOT NULL,
  currency text NOT NULL,
  fraction_digits smallint NOT NULL CHECK (fraction_digits BETWEEN 0 AND 18),
  capability_hash text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  FOREIGN KEY (recipient_config_id,store_id) REFERENCES recipient_configurations(id,store_id),
  UNIQUE (id,store_id)
);
CREATE INDEX orders_store_cursor ON orders(store_id,created_at,id);

CREATE TABLE order_lines (
  id app_id PRIMARY KEY,
  order_id app_id NOT NULL REFERENCES orders(id),
  line_no integer NOT NULL CHECK (line_no > 0),
  item_snapshot jsonb NOT NULL,
  quantity integer NOT NULL CHECK (quantity > 0),
  line_total atomic_total NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (order_id,line_no)
);


CREATE TABLE payment_attempts (
  id app_id PRIMARY KEY,
  order_id app_id NOT NULL REFERENCES orders(id),
  asset_id app_id NOT NULL REFERENCES assets(id),
  amount uint256_amount NOT NULL CHECK (amount > 0),
  recipient_snapshot evm_address NOT NULL,
  quote_expires_at timestamptz NOT NULL,
  state text NOT NULL,
  payer_address evm_address,
  session_ref text,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (id,order_id),
  UNIQUE (id,asset_id),
  UNIQUE (id,order_id,asset_id)
);


CREATE TABLE transaction_intents (
  id app_id PRIMARY KEY,
  attempt_id app_id REFERENCES payment_attempts(id),
  wallet_id app_id REFERENCES wallets(id),
  source_kind text NOT NULL,
  source_ref app_id NOT NULL,
  profile_id app_id NOT NULL REFERENCES protocol_profiles(id),
  chain_id bigint NOT NULL CHECK (chain_id = 8283),
  payload_digest hash32 NOT NULL,
  review_payload_ref text NOT NULL,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


CREATE TABLE transaction_submissions (
  id app_id PRIMARY KEY,
  intent_id app_id NOT NULL REFERENCES transaction_intents(id),
  chain_id bigint NOT NULL CHECK (chain_id = 8283),
  tx_hash hash32,
  userop_hash hash32,
  state text NOT NULL CHECK (state IN ('prepared','submitted','unknown','observed','failed')),
  signed_payload_ref text,
  submission_kind text NOT NULL CHECK (submission_kind IN ('eoa','user_operation')),
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  CHECK ((submission_kind = 'eoa' AND tx_hash IS NOT NULL AND userop_hash IS NULL) OR (submission_kind = 'user_operation' AND userop_hash IS NOT NULL))
);
CREATE UNIQUE INDEX submission_eoa_unique ON transaction_submissions(chain_id,tx_hash) WHERE submission_kind = 'eoa';
CREATE UNIQUE INDEX submission_userop_unique ON transaction_submissions(chain_id,userop_hash) WHERE submission_kind = 'user_operation';
CREATE INDEX submission_bundle_lookup ON transaction_submissions(chain_id,tx_hash) WHERE tx_hash IS NOT NULL;

CREATE TABLE payment_evidence (
  id app_id PRIMARY KEY,
  chain_id bigint NOT NULL CHECK (chain_id = 8283),
  tx_hash hash32 NOT NULL,
  kind text NOT NULL CHECK (kind IN ('erc20_log','native_transaction')),
  log_index integer,
  asset_id app_id NOT NULL,
  payer_address evm_address NOT NULL,
  recipient_address evm_address NOT NULL,
  amount uint256_amount NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  CHECK ((kind = 'erc20_log' AND log_index IS NOT NULL AND log_index >= 0) OR (kind = 'native_transaction' AND log_index IS NULL)),
  FOREIGN KEY (asset_id,chain_id) REFERENCES assets(id,chain_id),
  UNIQUE (id,asset_id),
  UNIQUE (id,chain_id,tx_hash)
);
CREATE UNIQUE INDEX evidence_log_unique ON payment_evidence(chain_id,tx_hash,log_index) WHERE kind = 'erc20_log';
CREATE UNIQUE INDEX evidence_native_unique ON payment_evidence(chain_id,tx_hash) WHERE kind = 'native_transaction';

CREATE TABLE chain_observations (
  id app_id PRIMARY KEY,
  evidence_id app_id REFERENCES payment_evidence(id),
  chain_id bigint NOT NULL CHECK (chain_id = 8283),
  tx_hash hash32 NOT NULL,
  block_hash hash32 NOT NULL,
  block_number numeric NOT NULL CHECK (block_number >= 0 AND block_number = trunc(block_number)),
  receipt_status text NOT NULL CHECK (receipt_status IN ('success','reverted')),
  canonicality text NOT NULL CHECK (canonicality IN ('canonical','orphaned','unknown')),
  policy_id text NOT NULL,
  policy_satisfied boolean NOT NULL,
  observed_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  CHECK (evidence_id IS NULL OR receipt_status = 'success'),
  FOREIGN KEY (evidence_id,chain_id,tx_hash) REFERENCES payment_evidence(id,chain_id,tx_hash),
  UNIQUE (id,evidence_id)
);


CREATE TABLE payment_allocations (
  id app_id PRIMARY KEY,
  evidence_id app_id NOT NULL UNIQUE REFERENCES payment_evidence(id),
  order_id app_id NOT NULL REFERENCES orders(id),
  attempt_id app_id NOT NULL,
  state text NOT NULL CHECK (state IN ('pending','accepted','exception','invalidated')),
  observation_id app_id NOT NULL REFERENCES chain_observations(id),
  asset_id app_id NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  FOREIGN KEY (attempt_id,order_id) REFERENCES payment_attempts(id,order_id),
  FOREIGN KEY (attempt_id,order_id,asset_id) REFERENCES payment_attempts(id,order_id,asset_id),
  FOREIGN KEY (evidence_id,asset_id) REFERENCES payment_evidence(id,asset_id),
  FOREIGN KEY (observation_id,evidence_id) REFERENCES chain_observations(id,evidence_id),
  UNIQUE (id,order_id,asset_id)
);


CREATE TABLE refund_balances (
  id app_id PRIMARY KEY,
  allocation_id app_id NOT NULL UNIQUE REFERENCES payment_allocations(id),
  asset_id app_id NOT NULL REFERENCES assets(id),
  policy_refundable atomic_total NOT NULL,
  reserved atomic_total NOT NULL DEFAULT 0,
  confirmed atomic_total NOT NULL DEFAULT 0,
  order_id app_id NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  CHECK (reserved + confirmed <= policy_refundable),
  UNIQUE (id,asset_id),
  FOREIGN KEY (allocation_id,order_id,asset_id) REFERENCES payment_allocations(id,order_id,asset_id),
  UNIQUE (id,asset_id,order_id)
);


CREATE TABLE refunds (
  id app_id PRIMARY KEY,
  order_id app_id NOT NULL REFERENCES orders(id),
  balance_id app_id NOT NULL,
  asset_id app_id NOT NULL,
  amount uint256_amount NOT NULL CHECK (amount > 0),
  destination evm_address NOT NULL,
  destination_proof_ref text NOT NULL,
  state text NOT NULL CHECK (state IN ('requested','authorized','signing','submitted','unknown','confirmed','rejected','failed')),
  intent_id app_id REFERENCES transaction_intents(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  FOREIGN KEY (balance_id,asset_id) REFERENCES refund_balances(id,asset_id),
  FOREIGN KEY (balance_id,asset_id,order_id) REFERENCES refund_balances(id,asset_id,order_id)
);


CREATE TABLE refund_reservations (
  id app_id PRIMARY KEY,
  refund_id app_id NOT NULL UNIQUE REFERENCES refunds(id),
  amount uint256_amount NOT NULL CHECK (amount > 0),
  state text NOT NULL CHECK (state IN ('active','consumed','released')),
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);


CREATE TABLE settlements (
  id app_id PRIMARY KEY,
  store_id app_id NOT NULL REFERENCES stores(id),
  period_start timestamptz NOT NULL,
  period_end timestamptz NOT NULL,
  asset_id app_id NOT NULL REFERENCES assets(id),
  state text NOT NULL CHECK (state IN ('open','closed','corrected')),
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  CHECK (period_start < period_end),
  UNIQUE (store_id,period_start,period_end,asset_id)
);


CREATE TABLE settlement_revisions (
  id app_id PRIMARY KEY,
  settlement_id app_id NOT NULL REFERENCES settlements(id),
  version bigint NOT NULL CHECK (version >= 0),
  totals_snapshot jsonb NOT NULL,
  source_revision bigint NOT NULL,
  reason text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (settlement_id,version)
);


CREATE TABLE benefit_entries (
  id app_id PRIMARY KEY,
  account_id app_id NOT NULL REFERENCES accounts(id),
  store_id app_id NOT NULL REFERENCES stores(id),
  allocation_id app_id REFERENCES payment_allocations(id),
  rule_version text NOT NULL,
  kind text NOT NULL CHECK (kind IN ('grant','consume','correction')),
  source_key text NOT NULL,
  units bigint NOT NULL CHECK (units <> 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
  UNIQUE (account_id,store_id,rule_version,kind,source_key)
);


CREATE TABLE benefit_redemptions (
  id app_id PRIMARY KEY,
  grant_entry_id app_id NOT NULL REFERENCES benefit_entries(id),
  order_id app_id NOT NULL REFERENCES orders(id),
  state text NOT NULL CHECK (state IN ('reserved','used','released')),
  created_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);
CREATE UNIQUE INDEX one_live_redemption ON benefit_redemptions(grant_entry_id) WHERE state IN ('reserved','used');

INSERT INTO schema_migrations(version) VALUES ('003');
COMMIT;

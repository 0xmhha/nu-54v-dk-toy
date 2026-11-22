CREATE SCHEMA market;
REVOKE ALL ON SCHEMA market FROM PUBLIC;

CREATE DOMAIN market.uint256 AS numeric
  CHECK (VALUE >= 0 AND VALUE < 2::numeric ^ 256 AND VALUE = trunc(VALUE));

CREATE TABLE market.venue (
  code_hash bytea NOT NULL CHECK (octet_length(code_hash) = 32),
  genesis bytea NOT NULL CHECK (octet_length(genesis) = 32),
  contract_address bytea NOT NULL CHECK (octet_length(contract_address) = 20),
  deployment_hash bytea NOT NULL CHECK (octet_length(deployment_hash) = 32),
  chain_id integer NOT NULL CHECK (chain_id = 8283),
  profile text NOT NULL CHECK (profile = 'p08-isolated-8283-v1'),
  PRIMARY KEY (genesis, contract_address, deployment_hash)
);

CREATE TABLE market.keeper_job (
  genesis bytea NOT NULL,
  contract_address bytea NOT NULL,
  deployment_hash bytea NOT NULL,
  position market.uint256 NOT NULL CHECK (position > 0),
  kind text NOT NULL CHECK (kind IN ('funding','liquidation')),
  period market.uint256 NOT NULL CHECK ((kind = 'funding') = (period > 0)),
  PRIMARY KEY (genesis, contract_address, deployment_hash, position, kind, period),
  FOREIGN KEY (genesis, contract_address, deployment_hash)
    REFERENCES market.venue (genesis, contract_address, deployment_hash)
);

CREATE TABLE market.keeper_attempt (
  genesis bytea NOT NULL,
  contract_address bytea NOT NULL,
  deployment_hash bytea NOT NULL,
  position market.uint256 NOT NULL,
  kind text NOT NULL,
  period market.uint256 NOT NULL,
  revision market.uint256 NOT NULL CHECK (revision > 0),
  price_version market.uint256 NOT NULL CHECK (price_version > 0),
  tx_hash bytea NOT NULL CHECK (octet_length(tx_hash) = 32),
  raw_tx bytea NOT NULL CHECK (octet_length(raw_tx) BETWEEN 1 AND 2048),
  state text NOT NULL DEFAULT 'prepared'
    CHECK (state IN ('prepared','submitted','unknown','confirmed','rejected','reorged')),
  block_height bigint CHECK (block_height > 0),
  block_hash bytea CHECK (octet_length(block_hash) = 32),
  created_at timestamptz NOT NULL DEFAULT now(),
  reviewed_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (genesis, contract_address, deployment_hash, position, kind, period, revision, price_version),
  UNIQUE (genesis, contract_address, deployment_hash, tx_hash),
  CHECK ((state IN ('confirmed','rejected')) = (block_height IS NOT NULL AND block_hash IS NOT NULL)),
  FOREIGN KEY (genesis, contract_address, deployment_hash, position, kind, period)
    REFERENCES market.keeper_job (genesis, contract_address, deployment_hash, position, kind, period)
);
CREATE UNIQUE INDEX market_one_unresolved
  ON market.keeper_attempt (genesis, contract_address, deployment_hash)
  WHERE state IN ('prepared','submitted','unknown','reorged');

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'nu54_p08_keeper') THEN
    CREATE ROLE nu54_p08_keeper NOLOGIN;
  END IF;
END $$;
GRANT USAGE ON SCHEMA market TO nu54_p08_keeper;
GRANT SELECT, INSERT ON market.venue, market.keeper_job TO nu54_p08_keeper;
GRANT SELECT, INSERT, UPDATE (state, block_height, block_hash, reviewed_at) ON market.keeper_attempt TO nu54_p08_keeper;

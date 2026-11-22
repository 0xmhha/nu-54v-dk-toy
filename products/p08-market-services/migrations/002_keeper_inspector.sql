DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'nu54_p08_inspector') THEN
    CREATE ROLE nu54_p08_inspector NOLOGIN;
  END IF;
END $$;

GRANT USAGE ON SCHEMA market TO nu54_p08_inspector;
GRANT SELECT (genesis, contract_address, deployment_hash, position, kind, period, revision, price_version,
    tx_hash, state, block_height, block_hash, created_at)
  ON market.keeper_attempt TO nu54_p08_inspector;

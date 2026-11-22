-- P07 owns chain evidence and read generations. Legacy rows remain in public.
CREATE SCHEMA chainview;
SET LOCAL search_path TO chainview, pg_catalog;
CREATE TABLE stream (
  id uuid PRIMARY KEY, chain_ref text NOT NULL, contract_ref text NOT NULL, abi_hash text NOT NULL,
  manifest_ref text NOT NULL, UNIQUE(chain_ref,contract_ref,abi_hash,manifest_ref)
);
CREATE TABLE generation (
  id uuid PRIMARY KEY, stream_id uuid NOT NULL REFERENCES stream,
  base_height bigint NOT NULL CHECK(base_height>=0), tip_height bigint, tip_hash text,
  finalized_height bigint, finalized_hash text, fence bigint NOT NULL CHECK(fence>=0),
  lease_until timestamptz CHECK (isfinite(lease_until)), state text NOT NULL CHECK(state IN ('building','ready','frozen')),
  CHECK((tip_height IS NULL)=(tip_hash IS NULL)), CHECK((finalized_height IS NULL)=(finalized_hash IS NULL)),
  CHECK(finalized_height IS NULL OR (tip_height IS NOT NULL AND finalized_height BETWEEN base_height AND tip_height)),
  CHECK (tip_height >= base_height),
  CHECK (state <> 'ready' OR finalized_height IS NOT NULL),
  UNIQUE(stream_id,id)
);
CREATE TABLE block (
  generation_id uuid NOT NULL REFERENCES generation, block_hash text NOT NULL,
  height bigint NOT NULL CHECK(height>=0), parent_hash text NOT NULL, block_time timestamptz NOT NULL,
  canonical boolean NOT NULL, PRIMARY KEY(generation_id,block_hash), UNIQUE(generation_id,height,block_hash)
);
CREATE UNIQUE INDEX one_canonical_height ON block(generation_id,height) WHERE canonical;
ALTER TABLE generation ADD FOREIGN KEY(id,tip_height,tip_hash) REFERENCES block(generation_id,height,block_hash);
ALTER TABLE generation ADD FOREIGN KEY(id,finalized_height,finalized_hash) REFERENCES block(generation_id,height,block_hash);
CREATE TABLE observed_event (
  generation_id uuid NOT NULL, block_hash text NOT NULL, tx_hash text NOT NULL, log_index integer NOT NULL CHECK(log_index>=0),
  kind text NOT NULL CHECK(kind IN ('paid','refunded','invalidated','cashout','rental')),
  receipt_success boolean NOT NULL CHECK(receipt_success), recipient_ref text, signer_nonce text,
  merchant_key text NOT NULL, order_key text NOT NULL, refund_key text, asset_ref text,
  amount numeric CHECK(amount>=0 AND amount=trunc(amount) AND amount<power(2::numeric,256)),
  PRIMARY KEY(generation_id,block_hash,tx_hash,log_index),
  FOREIGN KEY(generation_id,block_hash) REFERENCES block,
  CHECK((kind='refunded')=(refund_key IS NOT NULL)),
  CHECK(kind NOT IN ('paid','refunded') OR (asset_ref IS NOT NULL AND recipient_ref IS NOT NULL AND amount IS NOT NULL AND amount>0)),
  CHECK(kind<>'paid' OR signer_nonce IS NOT NULL)
);
CREATE INDEX event_lookup ON observed_event(generation_id,merchant_key,order_key);
CREATE TABLE evidence (
  id uuid PRIMARY KEY, generation_id uuid NOT NULL, merchant_key text NOT NULL, order_key text NOT NULL,
  kind text NOT NULL CHECK(kind IN ('paid','refunded','absent','invalidated')),
  anchor_height bigint NOT NULL, anchor_hash text NOT NULL, policy_ref text NOT NULL,
  expires_at timestamptz, coverage_start bigint, state_proof_ref text,
  log_block text, tx_hash text, log_index integer, created_at timestamptz NOT NULL,
  FOREIGN KEY(generation_id,anchor_height,anchor_hash) REFERENCES block(generation_id,height,block_hash),
  FOREIGN KEY(generation_id,log_block,tx_hash,log_index) REFERENCES observed_event,
  CHECK((kind='absent' AND expires_at IS NOT NULL AND coverage_start IS NOT NULL
    AND coverage_start BETWEEN 0 AND anchor_height AND state_proof_ref IS NOT NULL
    AND log_block IS NULL AND tx_hash IS NULL AND log_index IS NULL)
    OR (kind<>'absent' AND log_block IS NOT NULL AND tx_hash IS NOT NULL AND log_index IS NOT NULL))
);
CREATE TABLE read_target (
  stream_id uuid PRIMARY KEY REFERENCES stream, generation_id uuid NOT NULL, revision bigint NOT NULL CHECK (revision >= 0),
  FOREIGN KEY(stream_id,generation_id) REFERENCES generation(stream_id,id)
);
CREATE TABLE switch_request (
  id uuid PRIMARY KEY, stream_id uuid NOT NULL, target_id uuid NOT NULL, expected_revision bigint NOT NULL CHECK (expected_revision >= 0),
  operator_ref uuid NOT NULL, compare_ref uuid NOT NULL, requested_at timestamptz NOT NULL,
  applied_at timestamptz,
  CHECK (isfinite(requested_at) AND (applied_at IS NULL OR (isfinite(applied_at) AND applied_at >= requested_at))),
  FOREIGN KEY(stream_id,target_id) REFERENCES generation(stream_id,id)
);
CREATE TABLE incident (
  id uuid PRIMARY KEY, generation_id uuid NOT NULL REFERENCES generation, disputed_height bigint NOT NULL CHECK (disputed_height >= 0),
  old_hash text NOT NULL, new_hash text NOT NULL CHECK(new_hash<>old_hash), opened_at timestamptz NOT NULL,
  resolved_at timestamptz, notified_at timestamptz,
  CHECK (isfinite(opened_at) AND (resolved_at IS NULL OR (isfinite(resolved_at) AND resolved_at >= opened_at))
    AND (notified_at IS NULL OR (isfinite(notified_at) AND notified_at >= opened_at)))
);

-- Only current, finalized evidence reaches the downstream reconciliation role.
-- The service still verifies the evidence policy against the originating order.
CREATE VIEW active_evidence WITH (security_barrier = true) AS
SELECT e.*
FROM evidence AS e
JOIN read_target AS target ON target.generation_id = e.generation_id
JOIN generation AS g ON g.id = e.generation_id
JOIN block AS anchor ON (anchor.generation_id, anchor.block_hash) = (e.generation_id, e.anchor_hash)
LEFT JOIN observed_event AS log ON (log.generation_id, log.block_hash, log.tx_hash, log.log_index) =
    (e.generation_id, e.log_block, e.tx_hash, e.log_index)
LEFT JOIN block AS included ON (included.generation_id, included.block_hash) = (log.generation_id, log.block_hash)
WHERE g.state = 'ready' AND anchor.canonical AND e.anchor_height <= g.finalized_height
    AND ((e.kind = 'absent' AND e.expires_at > statement_timestamp())
        OR (log.kind = e.kind AND log.merchant_key = e.merchant_key AND log.order_key = e.order_key
            AND included.canonical AND included.height <= e.anchor_height));

REVOKE ALL ON SCHEMA chainview FROM PUBLIC;

-- Cluster-level capability roles. Deployments grant memberships to scoped logins.
DO $roles$
DECLARE role_name text;
BEGIN
    FOREACH role_name IN ARRAY ARRAY['nu54_p07_legacy', 'nu54_p07_ingest', 'nu54_p07_read', 'nu54_p07_switch']
    LOOP
        IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = role_name) THEN
            EXECUTE format('CREATE ROLE %I NOLOGIN', role_name);
        ELSIF EXISTS (
            SELECT FROM pg_catalog.pg_roles WHERE rolname = role_name
                AND (rolcanlogin OR rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication OR rolbypassrls)
        ) THEN
            RAISE EXCEPTION 'unsafe capability role %', role_name;
        END IF;
    END LOOP;
END
$roles$;

GRANT USAGE ON SCHEMA public TO nu54_p07_legacy;
GRANT SELECT, INSERT ON public.receipts TO nu54_p07_legacy;
GRANT SELECT, INSERT ON public.cursor TO nu54_p07_legacy;
GRANT UPDATE (block_number) ON public.cursor TO nu54_p07_legacy;

GRANT USAGE ON SCHEMA chainview TO nu54_p07_ingest, nu54_p07_read, nu54_p07_switch;
GRANT SELECT, INSERT ON stream, generation, block, observed_event, evidence, incident TO nu54_p07_ingest;
GRANT UPDATE (tip_height, tip_hash, finalized_height, finalized_hash, fence, lease_until, state)
    ON generation TO nu54_p07_ingest;
GRANT UPDATE (canonical) ON block TO nu54_p07_ingest;
GRANT UPDATE (resolved_at, notified_at) ON incident TO nu54_p07_ingest;
GRANT SELECT ON active_evidence TO nu54_p07_read;
GRANT SELECT ON stream, generation, read_target, switch_request TO nu54_p07_switch;

GRANT INSERT (id, stream_id, target_id, expected_revision, operator_ref, compare_ref, requested_at)
    ON switch_request TO nu54_p07_switch;

-- The switch service authenticates the operator and verifies the comparison
-- artifact before submitting a request. The database owns the pointer CAS.
CREATE FUNCTION chainview.apply_switch(request_id uuid) RETURNS timestamptz
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog
AS $function$
DECLARE
    pending chainview.switch_request%ROWTYPE;
    changed_revision bigint;
    applied timestamptz;
BEGIN
    SELECT * INTO STRICT pending FROM chainview.switch_request WHERE id = request_id FOR UPDATE;
    IF pending.applied_at IS NOT NULL THEN
        RETURN pending.applied_at;
    END IF;
    PERFORM 1 FROM chainview.generation AS g
        WHERE g.stream_id = pending.stream_id AND g.id = pending.target_id AND g.state = 'ready'
        FOR SHARE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'target generation is not ready' USING ERRCODE = '55000';
    END IF;

    UPDATE chainview.read_target AS target
        SET generation_id = pending.target_id, revision = target.revision + 1
        WHERE target.stream_id = pending.stream_id AND target.revision = pending.expected_revision
        RETURNING target.revision INTO changed_revision;
    IF changed_revision IS NULL AND pending.expected_revision = 0 THEN
        INSERT INTO chainview.read_target VALUES (pending.stream_id, pending.target_id, 1)
            ON CONFLICT (stream_id) DO NOTHING RETURNING revision INTO changed_revision;
    END IF;
    IF changed_revision IS NULL THEN
        RAISE EXCEPTION 'read target revision changed' USING ERRCODE = '55000';
    END IF;

    UPDATE chainview.switch_request SET applied_at = clock_timestamp()
        WHERE id = request_id RETURNING applied_at INTO applied;
    RETURN applied;
END
$function$;
REVOKE ALL ON FUNCTION chainview.apply_switch(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION chainview.apply_switch(uuid) TO nu54_p07_switch;

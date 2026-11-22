SET LOCAL search_path TO chainview, pg_catalog;

ALTER TABLE observed_event
    ADD COLUMN payer_ref text,
    ADD COLUMN quote_digest text,
    ADD COLUMN payment_mode smallint CHECK (payment_mode IN (1, 2)),
    ADD COLUMN payer_generation numeric CHECK (
        payer_generation > 0 AND payer_generation = trunc(payer_generation) AND payer_generation < power(2::numeric, 64)
    ),
    ADD CHECK ((quote_digest IS NOT NULL) = (payer_generation IS NOT NULL)),
    ADD CHECK (quote_digest IS NULL OR (kind = 'paid' AND payment_mode IS NOT NULL AND payer_ref IS NOT NULL));

CREATE TABLE deployment_profile (
    stream_id uuid PRIMARY KEY REFERENCES stream,
    version smallint NOT NULL CHECK (version IN (1, 2)),
    asset_ref text NOT NULL CHECK (asset_ref ~ '^0x[0-9a-f]{40}$'),
    code_hash text NOT NULL CHECK (code_hash ~ '^0x[0-9a-f]{64}$'),
    start_height bigint NOT NULL CHECK (start_height > 0),
    start_hash text NOT NULL CHECK (start_hash ~ '^0x[0-9a-f]{64}$')
);

CREATE TABLE receipt_scope (
    login_role name NOT NULL,
    stream_id uuid NOT NULL REFERENCES stream,
    merchant_key text NOT NULL,
    valid_until timestamptz NOT NULL CHECK (isfinite(valid_until)),
    PRIMARY KEY (login_role, stream_id, merchant_key)
);

CREATE UNIQUE INDEX evidence_one_observation
    ON evidence(generation_id, log_block, tx_hash, log_index) WHERE log_block IS NOT NULL;

CREATE OR REPLACE VIEW active_evidence WITH (security_barrier = true) AS
SELECT e.*
FROM evidence AS e
JOIN read_target AS target ON target.generation_id = e.generation_id
JOIN generation AS g ON g.id = e.generation_id
JOIN block AS anchor ON (anchor.generation_id, anchor.block_hash) = (e.generation_id, e.anchor_hash)
LEFT JOIN observed_event AS log ON (log.generation_id, log.block_hash, log.tx_hash, log.log_index) =
    (e.generation_id, e.log_block, e.tx_hash, e.log_index)
LEFT JOIN block AS included ON (included.generation_id, included.block_hash) = (log.generation_id, log.block_hash)
WHERE g.state = 'ready'
    AND NOT EXISTS (
        SELECT FROM incident i JOIN generation affected ON affected.id=i.generation_id
        WHERE affected.stream_id=target.stream_id AND i.resolved_at IS NULL
    )
    AND anchor.canonical AND e.anchor_height <= g.finalized_height
    AND ((e.kind = 'absent' AND e.expires_at > statement_timestamp())
        OR (log.kind = e.kind AND log.merchant_key = e.merchant_key AND log.order_key = e.order_key
            AND included.canonical AND included.height <= e.anchor_height));

CREATE VIEW finalized_receipt WITH (security_barrier = true) AS
SELECT e.id, target.stream_id, e.generation_id, s.chain_ref, s.contract_ref, s.abi_hash, e.kind,
    e.merchant_key, e.order_key, log.refund_key, log.asset_ref, log.amount, e.tx_hash, e.log_index,
    included.height AS inclusion_height, e.log_block AS inclusion_hash, included.block_time AS included_at,
    e.anchor_height, e.anchor_hash, e.policy_ref, e.created_at AS attested_at
FROM active_evidence AS e
JOIN read_target AS target ON target.generation_id = e.generation_id
JOIN stream AS s ON s.id = target.stream_id
JOIN observed_event AS log ON (log.generation_id, log.block_hash, log.tx_hash, log.log_index) =
    (e.generation_id, e.log_block, e.tx_hash, e.log_index)
JOIN block AS included ON (included.generation_id, included.block_hash) = (log.generation_id, log.block_hash);

CREATE VIEW settlement_receipt WITH (security_barrier = true) AS
SELECT r.*, log.payer_ref, log.recipient_ref, log.signer_nonce, log.quote_digest, log.payment_mode, log.payer_generation
FROM finalized_receipt AS r
JOIN active_evidence AS e ON e.id = r.id
JOIN observed_event AS log ON (log.generation_id, log.block_hash, log.tx_hash, log.log_index) =
    (e.generation_id, e.log_block, e.tx_hash, e.log_index);

CREATE VIEW pending_incident WITH (security_barrier = true) AS
SELECT i.id, g.stream_id, i.generation_id, i.disputed_height, i.old_hash, i.new_hash, i.opened_at
FROM incident i JOIN generation g ON g.id = i.generation_id
WHERE i.notified_at IS NULL;

DO $roles$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'nu54_p07_public') THEN
        CREATE ROLE nu54_p07_public NOLOGIN;
    ELSIF EXISTS (
        SELECT FROM pg_catalog.pg_roles WHERE rolname = 'nu54_p07_public'
            AND (rolcanlogin OR rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication OR rolbypassrls)
    ) THEN
        RAISE EXCEPTION 'unsafe public receipt role';
    END IF;
END
$roles$;

GRANT USAGE ON SCHEMA chainview TO nu54_p07_public;
GRANT SELECT ON finalized_receipt TO nu54_p07_public;
GRANT SELECT, INSERT ON deployment_profile TO nu54_p07_ingest;

DO $roles$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'nu54_p07_reconcile') THEN
        CREATE ROLE nu54_p07_reconcile NOLOGIN;
    ELSIF EXISTS (
        SELECT FROM pg_catalog.pg_roles WHERE rolname = 'nu54_p07_reconcile'
            AND (rolcanlogin OR rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication OR rolbypassrls)
    ) THEN
        RAISE EXCEPTION 'unsafe restricted receipt role';
    END IF;
END
$roles$;

CREATE FUNCTION chainview.reconciliation_receipt(p_stream uuid, p_merchant text, p_order text)
    RETURNS SETOF chainview.settlement_receipt
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog
AS $sql$
    SELECT receipt.*
    FROM chainview.settlement_receipt AS receipt
    JOIN chainview.receipt_scope AS scope ON scope.stream_id = receipt.stream_id AND scope.merchant_key = receipt.merchant_key
    WHERE scope.login_role = session_user AND scope.valid_until > statement_timestamp()
        AND receipt.stream_id = p_stream AND receipt.merchant_key = p_merchant AND receipt.order_key = p_order
$sql$;
REVOKE ALL ON FUNCTION chainview.reconciliation_receipt(uuid, text, text) FROM PUBLIC;
GRANT USAGE ON SCHEMA chainview TO nu54_p07_reconcile;
GRANT EXECUTE ON FUNCTION chainview.reconciliation_receipt(uuid, text, text) TO nu54_p07_reconcile;
DO $roles$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'nu54_p07_incident') THEN
        CREATE ROLE nu54_p07_incident NOLOGIN;
    ELSIF EXISTS (
        SELECT FROM pg_catalog.pg_roles WHERE rolname = 'nu54_p07_incident'
            AND (rolcanlogin OR rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication OR rolbypassrls)
    ) THEN
        RAISE EXCEPTION 'unsafe incident delivery role';
    END IF;
END
$roles$;
GRANT USAGE ON SCHEMA chainview TO nu54_p07_incident;
GRANT SELECT ON pending_incident TO nu54_p07_incident;
REVOKE UPDATE (resolved_at, notified_at) ON incident FROM nu54_p07_ingest;
GRANT SELECT ON read_target TO nu54_p07_ingest;

CREATE FUNCTION chainview.acknowledge_incident(p_id uuid) RETURNS boolean
    LANGUAGE sql SECURITY DEFINER SET search_path = pg_catalog
AS $sql$
    WITH updated AS (
        UPDATE chainview.incident SET notified_at = clock_timestamp()
        WHERE id = p_id AND notified_at IS NULL
        RETURNING id
    )
    SELECT EXISTS (SELECT FROM updated)
$sql$;
REVOKE ALL ON FUNCTION chainview.acknowledge_incident(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION chainview.acknowledge_incident(uuid) TO nu54_p07_incident;

CREATE FUNCTION chainview.lock_absence_anchor(p_stream uuid, p_generation uuid, p_height bigint, p_hash text)
    RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog
AS $sql$
DECLARE matched boolean;
BEGIN
    PERFORM pg_catalog.pg_advisory_xact_lock_shared(pg_catalog.hashtextextended(p_stream::text, 9));
    SELECT true INTO matched FROM chainview.read_target t
        JOIN chainview.generation g ON g.id = t.generation_id
    WHERE t.stream_id = p_stream AND g.id = p_generation AND g.state = 'ready'
        AND g.finalized_height = p_height AND g.finalized_hash = p_hash
        AND NOT EXISTS (
            SELECT FROM chainview.incident i JOIN chainview.generation affected ON affected.id=i.generation_id
            WHERE affected.stream_id=p_stream AND i.resolved_at IS NULL
        )
    FOR SHARE OF t, g;
    RETURN COALESCE(matched,false);
END
$sql$;
REVOKE ALL ON FUNCTION chainview.lock_absence_anchor(uuid, uuid, bigint, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION chainview.lock_absence_anchor(uuid, uuid, bigint, text) TO nu54_p07_ingest;

CREATE TABLE projection_comparison (
    id uuid PRIMARY KEY,
    stream_id uuid NOT NULL,
    candidate_id uuid NOT NULL,
    previous_id uuid,
    expected_revision bigint NOT NULL CHECK (expected_revision >= 0),
    anchor_height bigint NOT NULL,
    anchor_hash text NOT NULL,
    compared_at timestamptz NOT NULL,
    FOREIGN KEY(stream_id, candidate_id) REFERENCES generation(stream_id,id),
    FOREIGN KEY(stream_id, previous_id) REFERENCES generation(stream_id,id)
);

CREATE FUNCTION chainview.complete_prefix(p_generation uuid, p_height bigint) RETURNS boolean
    LANGUAGE sql STABLE SET search_path = pg_catalog
AS $sql$
    SELECT count(*) = p_height - g.base_height + 1 AND
        bool_and(CASE WHEN current.height=g.base_height THEN current.block_hash=p.start_hash
                      ELSE parent.block_hash IS NOT NULL END)
    FROM chainview.generation g JOIN chainview.deployment_profile p ON p.stream_id=g.stream_id
    JOIN chainview.block current ON current.generation_id=g.id AND current.canonical
    LEFT JOIN chainview.block parent ON parent.generation_id=g.id AND parent.canonical
        AND parent.block_hash=current.parent_hash AND parent.height=current.height-1
    WHERE g.id=p_generation AND current.height BETWEEN g.base_height AND p_height
    GROUP BY g.base_height
$sql$;
REVOKE ALL ON FUNCTION chainview.complete_prefix(uuid, bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION chainview.complete_prefix(uuid, bigint) TO nu54_p07_ingest;

CREATE FUNCTION chainview.record_comparison(p_stream uuid, p_candidate uuid, p_revision bigint) RETURNS uuid
    LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog
AS $comparison$
DECLARE
    target chainview.generation%ROWTYPE;
    previous_target chainview.read_target%ROWTYPE;
    previous_generation chainview.generation%ROWTYPE;
    proof_id uuid;
    differences boolean;
BEGIN
    PERFORM pg_catalog.pg_advisory_xact_lock_shared(pg_catalog.hashtextextended(p_stream::text, 9));
    IF EXISTS (
        SELECT FROM chainview.incident i JOIN chainview.generation affected ON affected.id=i.generation_id
        WHERE affected.stream_id=p_stream AND i.resolved_at IS NULL
    ) THEN
        RAISE EXCEPTION 'stream has an unresolved finalized incident' USING ERRCODE='55000';
    END IF;
    SELECT * INTO target FROM chainview.generation WHERE id=p_candidate AND stream_id=p_stream FOR SHARE;
    IF NOT FOUND OR target.state <> 'ready'
        OR NOT COALESCE(chainview.complete_prefix(p_candidate,target.finalized_height),false) THEN
        RAISE EXCEPTION 'candidate generation has incomplete canonical coverage' USING ERRCODE='55000';
    END IF;
    SELECT * INTO previous_target FROM chainview.read_target WHERE stream_id=p_stream FOR SHARE;
    IF p_revision IS DISTINCT FROM COALESCE(previous_target.revision,0) THEN
        RAISE EXCEPTION 'read target revision changed' USING ERRCODE='55000';
    END IF;
    IF previous_target.generation_id IS NOT NULL THEN
        SELECT * INTO STRICT previous_generation FROM chainview.generation
            WHERE id=previous_target.generation_id FOR SHARE;
        IF previous_generation.finalized_height IS DISTINCT FROM target.finalized_height
            OR previous_generation.finalized_hash IS DISTINCT FROM target.finalized_hash
            OR NOT COALESCE(chainview.complete_prefix(previous_generation.id,target.finalized_height),false) THEN
            RAISE EXCEPTION 'comparison must cover the same complete finalized chain' USING ERRCODE='55000';
        END IF;
        WITH facts AS (
            SELECT e.generation_id,e.block_hash,e.tx_hash,e.log_index,e.kind,e.merchant_key,
                e.order_key,e.refund_key,e.asset_ref,e.amount,e.payer_ref,e.recipient_ref,e.signer_nonce,
                e.quote_digest,e.payment_mode,e.payer_generation
            FROM chainview.observed_event e JOIN chainview.block b
                ON b.generation_id=e.generation_id AND b.block_hash=e.block_hash
            WHERE e.generation_id IN (previous_generation.id,p_candidate)
                AND b.canonical AND b.height<=target.finalized_height
        )
        SELECT EXISTS (
            SELECT FROM facts GROUP BY block_hash,tx_hash,log_index,kind,merchant_key,order_key,
                refund_key,asset_ref,amount,payer_ref,recipient_ref,signer_nonce,quote_digest,payment_mode,payer_generation
            HAVING count(DISTINCT generation_id)<>2
        ) INTO differences;
        IF previous_generation.id<>p_candidate AND differences THEN
            RAISE EXCEPTION 'finalized settlement projection differs' USING ERRCODE='55000';
        END IF;
    END IF;
    INSERT INTO chainview.projection_comparison VALUES (gen_random_uuid(),p_stream,p_candidate,
        previous_target.generation_id,p_revision,target.finalized_height,target.finalized_hash,clock_timestamp())
        RETURNING id INTO proof_id;
    RETURN proof_id;
END
$comparison$;
REVOKE ALL ON FUNCTION chainview.record_comparison(uuid,uuid,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION chainview.record_comparison(uuid,uuid,bigint) TO nu54_p07_switch;

ALTER FUNCTION chainview.apply_switch(uuid) RENAME TO apply_switch_trusted_v2;
REVOKE ALL ON FUNCTION chainview.apply_switch_trusted_v2(uuid) FROM nu54_p07_switch;

CREATE FUNCTION chainview.apply_switch(p_request uuid) RETURNS timestamptz
    LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog
AS $apply$
DECLARE
    pending chainview.switch_request%ROWTYPE;
    matched boolean;
BEGIN
    SELECT * INTO STRICT pending FROM chainview.switch_request WHERE id=p_request FOR UPDATE;
    IF pending.applied_at IS NOT NULL THEN
        RETURN pending.applied_at;
    END IF;
    PERFORM pg_catalog.pg_advisory_xact_lock_shared(pg_catalog.hashtextextended(pending.stream_id::text, 9));
    -- Retain the 002 contract for preexisting streams with no 003 deployment profile.
    IF EXISTS (SELECT FROM chainview.deployment_profile WHERE stream_id=pending.stream_id) THEN
        SELECT true INTO matched FROM chainview.projection_comparison c
            JOIN chainview.generation g ON g.id=c.candidate_id
            LEFT JOIN chainview.read_target t ON t.stream_id=c.stream_id
            WHERE c.id=pending.compare_ref AND c.stream_id=pending.stream_id AND c.candidate_id=pending.target_id
                AND c.expected_revision=pending.expected_revision
                AND c.previous_id IS NOT DISTINCT FROM t.generation_id
                AND COALESCE(t.revision,0)=c.expected_revision AND g.state='ready'
                AND NOT EXISTS (
                    SELECT FROM chainview.incident i JOIN chainview.generation affected ON affected.id=i.generation_id
                    WHERE affected.stream_id=c.stream_id AND i.resolved_at IS NULL
                )
                AND g.finalized_height=c.anchor_height AND g.finalized_hash=c.anchor_hash
                AND (t.generation_id IS NULL OR EXISTS (
                    SELECT FROM chainview.generation current WHERE current.id=t.generation_id
                        AND current.finalized_height=c.anchor_height AND current.finalized_hash=c.anchor_hash
                    FOR SHARE
                ))
            FOR SHARE OF g;
        IF matched IS DISTINCT FROM true THEN
            RAISE EXCEPTION 'missing or stale comparison for switch' USING ERRCODE='55000';
        END IF;
    END IF;
    RETURN chainview.apply_switch_trusted_v2(p_request);
END
$apply$;
REVOKE ALL ON FUNCTION chainview.apply_switch(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION chainview.apply_switch(uuid) TO nu54_p07_switch;

CREATE FUNCTION chainview.reconciliation_absence(p_stream uuid, p_merchant text, p_order text)
    RETURNS TABLE(evidence_id uuid, generation_id uuid, anchor_height bigint, anchor_hash text,
        anchored_at timestamptz, coverage_start bigint, policy_ref text, state_proof_ref text, expires_at timestamptz)
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog
AS $sql$
    SELECT e.id,e.generation_id,e.anchor_height,e.anchor_hash,anchor.block_time,e.coverage_start,
        e.policy_ref,e.state_proof_ref,e.expires_at
    FROM chainview.active_evidence e
    JOIN chainview.read_target t ON t.generation_id=e.generation_id
    JOIN chainview.block anchor ON anchor.generation_id=e.generation_id AND anchor.block_hash=e.anchor_hash
    JOIN chainview.receipt_scope scope ON scope.stream_id=t.stream_id AND scope.merchant_key=e.merchant_key
    WHERE e.kind='absent' AND t.stream_id=p_stream AND e.merchant_key=p_merchant AND e.order_key=p_order
         AND scope.login_role=session_user AND scope.valid_until>statement_timestamp()
    ORDER BY e.anchor_height DESC, e.created_at DESC
    LIMIT 1
$sql$;
REVOKE ALL ON FUNCTION chainview.reconciliation_absence(uuid,text,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION chainview.reconciliation_absence(uuid,text,text) TO nu54_p07_reconcile;

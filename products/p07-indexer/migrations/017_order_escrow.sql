-- OrderEscrow facts use the same blocks, observations, evidence, finality and holds as settlement receipts.
ALTER TABLE chainview.deployment_profile
    DROP CONSTRAINT deployment_profile_version_check,
    ADD CONSTRAINT deployment_profile_version_check CHECK (version IN (1, 2, 3));

ALTER TABLE chainview.observed_event
    ADD COLUMN confirmer_ref text,
    ADD COLUMN deliver_by bigint,
    ADD COLUMN confirm_window bigint,
    ADD COLUMN claimable_at bigint,
    ADD COLUMN resolve_by bigint,
    ADD COLUMN reason smallint,
    DROP CONSTRAINT observed_event_kind_check,
    ADD CONSTRAINT observed_event_kind_check CHECK (kind IN ('paid','refunded','invalidated','cashout','rental',
        'locked','delivered','disputed','released','cancelled','claim-skipped')),
    -- A lock carries a quote digest without a payer generation or a payment mode.
    DROP CONSTRAINT observed_event_check3,
    ADD CONSTRAINT observed_event_check3 CHECK (kind='locked' OR (quote_digest IS NOT NULL)=(payer_generation IS NOT NULL)),
    DROP CONSTRAINT observed_event_check4,
    ADD CONSTRAINT observed_event_check4 CHECK (quote_digest IS NULL OR kind='locked'
        OR (kind='paid' AND payment_mode IS NOT NULL AND payer_ref IS NOT NULL)),
    DROP CONSTRAINT quote_window_complete,
    ADD CONSTRAINT quote_window_complete CHECK (
        (quote_issued_at IS NULL AND quote_expires_at IS NULL)
        OR (kind IN ('paid','locked') AND quote_digest IS NOT NULL AND quote_issued_at IS NOT NULL
            AND quote_expires_at IS NOT NULL AND quote_issued_at<quote_expires_at)
    ),
    ADD CONSTRAINT escrow_event_complete CHECK (CASE kind
        WHEN 'locked' THEN num_nulls(payer_ref,recipient_ref,confirmer_ref,asset_ref,amount,quote_digest,deliver_by,confirm_window)=0
            AND amount>0 AND deliver_by>0 AND confirm_window>0
            AND num_nonnulls(signer_nonce,payment_mode,payer_generation,claimable_at,resolve_by,reason)=0
        WHEN 'delivered' THEN claimable_at IS NOT NULL AND claimable_at>0
            AND num_nonnulls(payer_ref,recipient_ref,confirmer_ref,asset_ref,amount,deliver_by,confirm_window,resolve_by,reason)=0
        WHEN 'disputed' THEN resolve_by IS NOT NULL AND resolve_by>0
            AND num_nonnulls(payer_ref,recipient_ref,confirmer_ref,asset_ref,amount,deliver_by,confirm_window,claimable_at,reason)=0
        WHEN 'released' THEN num_nulls(recipient_ref,asset_ref,amount,reason)=0 AND amount>0 AND reason BETWEEN 0 AND 3
            AND num_nonnulls(payer_ref,confirmer_ref,deliver_by,confirm_window,claimable_at,resolve_by)=0
        WHEN 'cancelled' THEN num_nulls(payer_ref,asset_ref,amount,reason)=0 AND amount>0 AND reason BETWEEN 0 AND 3
            AND num_nonnulls(recipient_ref,confirmer_ref,deliver_by,confirm_window,claimable_at,resolve_by)=0
        WHEN 'claim-skipped' THEN reason IS NOT NULL AND reason BETWEEN 1 AND 6
            AND num_nonnulls(payer_ref,recipient_ref,confirmer_ref,asset_ref,amount,deliver_by,confirm_window,claimable_at,resolve_by)=0
        ELSE num_nonnulls(confirmer_ref,deliver_by,confirm_window,claimable_at,resolve_by,reason)=0 END);

ALTER TABLE chainview.evidence
    DROP CONSTRAINT evidence_kind_check,
    ADD CONSTRAINT evidence_kind_check CHECK (kind IN ('paid','refunded','absent','invalidated',
        'locked','delivered','disputed','released','cancelled','claim-skipped'));

-- One definition decides what is current. The existing view keeps its earlier kinds, so the public,
-- explorer, finance and evidence readers never see an escrow fact.
CREATE VIEW chainview.current_evidence WITH (security_barrier=true) AS
SELECT e.*
FROM chainview.evidence AS e
JOIN chainview.read_target AS target ON target.generation_id = e.generation_id
JOIN chainview.generation AS g ON g.id = e.generation_id
JOIN chainview.block AS anchor ON (anchor.generation_id, anchor.block_hash) = (e.generation_id, e.anchor_hash)
LEFT JOIN chainview.observed_event AS log ON (log.generation_id, log.block_hash, log.tx_hash, log.log_index) =
    (e.generation_id, e.log_block, e.tx_hash, e.log_index)
LEFT JOIN chainview.block AS included ON (included.generation_id, included.block_hash) = (log.generation_id, log.block_hash)
WHERE g.state = 'ready'
    AND NOT EXISTS (
        SELECT FROM chainview.incident i JOIN chainview.generation affected ON affected.id=i.generation_id
        WHERE affected.stream_id=target.stream_id AND i.resolved_at IS NULL
    )
    AND anchor.canonical AND e.anchor_height <= g.finalized_height
    AND ((e.kind = 'absent' AND e.expires_at > statement_timestamp())
        OR (log.kind = e.kind AND log.merchant_key = e.merchant_key AND log.order_key = e.order_key
            AND included.canonical AND included.height <= e.anchor_height));

CREATE OR REPLACE VIEW chainview.active_evidence WITH (security_barrier=true) AS
SELECT * FROM chainview.current_evidence WHERE kind IN ('paid','refunded','absent','invalidated');

CREATE FUNCTION chainview.escrow_receipt(p_stream uuid, p_merchant text, p_order text)
    RETURNS TABLE(id uuid, kind text, chain_ref text, contract_ref text, abi_hash text, policy_ref text,
        payer_ref text, beneficiary_ref text, confirmer_ref text, amount numeric, quote_digest text,
        deliver_by bigint, confirm_window bigint, claimable_at bigint, resolve_by bigint, reason smallint,
        tx_hash text, inclusion_hash text, block_height bigint, log_index int, included_at timestamptz)
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $sql$
    SELECT e.id,e.kind,s.chain_ref,s.contract_ref,s.abi_hash,e.policy_ref,
        log.payer_ref,log.recipient_ref,log.confirmer_ref,log.amount,log.quote_digest,
        log.deliver_by,log.confirm_window,log.claimable_at,log.resolve_by,log.reason,
        e.tx_hash,e.log_block,included.height,e.log_index,included.block_time
    FROM chainview.current_evidence e
    JOIN chainview.read_target t ON t.generation_id=e.generation_id
    JOIN chainview.stream s ON s.id=t.stream_id
    JOIN chainview.receipt_scope scope ON scope.stream_id=t.stream_id AND scope.merchant_key=e.merchant_key
    JOIN chainview.observed_event log ON (log.generation_id,log.block_hash,log.tx_hash,log.log_index)
        =(e.generation_id,e.log_block,e.tx_hash,e.log_index)
    JOIN chainview.block included ON (included.generation_id,included.block_hash)=(log.generation_id,log.block_hash)
    WHERE scope.login_role=session_user AND scope.valid_until>statement_timestamp()
        AND t.stream_id=p_stream AND e.merchant_key=p_merchant AND e.order_key=p_order
        AND e.kind IN ('locked','delivered','disputed','released','cancelled','claim-skipped')
    ORDER BY included.height,e.log_index
$sql$;
REVOKE ALL ON FUNCTION chainview.escrow_receipt(uuid,text,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION chainview.escrow_receipt(uuid,text,text) TO nu54_p07_reconcile;

-- A replay must also reproduce the escrow fields. Their columns are empty for settlement streams.
CREATE OR REPLACE FUNCTION chainview.record_comparison(p_stream uuid, p_candidate uuid, p_revision bigint) RETURNS uuid
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
                e.quote_digest,e.payment_mode,e.payer_generation,
                e.confirmer_ref,e.deliver_by,e.confirm_window,e.claimable_at,e.resolve_by,e.reason
            FROM chainview.observed_event e JOIN chainview.block b
                ON b.generation_id=e.generation_id AND b.block_hash=e.block_hash
            WHERE e.generation_id IN (previous_generation.id,p_candidate)
                AND b.canonical AND b.height<=target.finalized_height
        )
        SELECT EXISTS (
            SELECT FROM facts GROUP BY block_hash,tx_hash,log_index,kind,merchant_key,order_key,
                refund_key,asset_ref,amount,payer_ref,recipient_ref,signer_nonce,quote_digest,payment_mode,payer_generation,
                confirmer_ref,deliver_by,confirm_window,claimable_at,resolve_by,reason
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

-- The totals count a lock as a payment and a cancellation as a refund.
CREATE OR REPLACE FUNCTION chainview.finish_replay(p_job uuid,p_diverged boolean)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE job chainview.replay_job;
DECLARE candidate chainview.generation;
DECLARE current_target chainview.read_target;
DECLARE current_generation chainview.generation;
DECLARE proof uuid;
DECLARE diff_reason text;
DECLARE logs_delta bigint;
DECLARE pays_delta numeric;
DECLARE refunds_delta numeric;
BEGIN
    SELECT * INTO job FROM chainview.replay_job WHERE id=p_job FOR UPDATE;
    IF NOT FOUND OR job.state<>'running' THEN RETURN; END IF;
    PERFORM pg_advisory_xact_lock_shared(hashtextextended(job.stream_id::text,9));
    IF p_diverged THEN
        UPDATE chainview.replay_job SET state='diff',reason='anchor_changed',finished_at=clock_timestamp() WHERE id=p_job;
        RETURN;
    END IF;
    SELECT * INTO candidate FROM chainview.generation WHERE id=p_job AND stream_id=job.stream_id FOR SHARE;
    IF NOT FOUND OR candidate.finalized_height IS NULL OR candidate.finalized_height<job.anchor_height THEN RETURN; END IF;
    SELECT * INTO current_target FROM chainview.read_target WHERE stream_id=job.stream_id FOR SHARE;
    SELECT * INTO current_generation FROM chainview.generation WHERE id=current_target.generation_id FOR SHARE;
    IF current_target.revision IS DISTINCT FROM job.expected_revision OR
       current_target.generation_id IS DISTINCT FROM job.previous_id OR
       current_generation.finalized_height IS DISTINCT FROM job.anchor_height OR
       current_generation.finalized_hash IS DISTINCT FROM job.anchor_hash OR
       candidate.finalized_height IS DISTINCT FROM job.anchor_height OR
       candidate.finalized_hash IS DISTINCT FROM job.anchor_hash OR
       NOT COALESCE(chainview.complete_prefix(candidate.id,job.anchor_height),false) OR
       NOT COALESCE(chainview.complete_prefix(current_generation.id,job.anchor_height),false) OR
       EXISTS(SELECT FROM chainview.incident i JOIN chainview.generation affected ON affected.id=i.generation_id
           WHERE affected.stream_id=job.stream_id AND i.resolved_at IS NULL) THEN
        diff_reason:='anchor_changed';
    ELSE
        BEGIN
            proof:=chainview.record_comparison(job.stream_id,p_job,job.expected_revision);
        EXCEPTION WHEN object_not_in_prerequisite_state THEN
            diff_reason:='projection_diff';
            WITH facts AS (
                SELECT e.generation_id,e.block_hash,e.tx_hash,e.log_index,e.kind,e.merchant_key,
                    e.order_key,e.refund_key,e.asset_ref,e.amount,e.payer_ref,e.recipient_ref,e.signer_nonce,
                    e.quote_digest,e.payment_mode,e.payer_generation,
                    e.confirmer_ref,e.deliver_by,e.confirm_window,e.claimable_at,e.resolve_by,e.reason
                FROM chainview.observed_event e JOIN chainview.block b USING(generation_id,block_hash)
                WHERE e.generation_id IN (job.previous_id,p_job) AND b.canonical AND b.height<=job.anchor_height
            ), compared AS (
                SELECT block_hash,tx_hash,log_index,kind,merchant_key,order_key,refund_key,asset_ref,
                    amount,payer_ref,recipient_ref,signer_nonce,quote_digest,payment_mode,payer_generation,
                    confirmer_ref,deliver_by,confirm_window,claimable_at,resolve_by,reason
                FROM facts GROUP BY block_hash,tx_hash,log_index,kind,merchant_key,order_key,refund_key,asset_ref,
                    amount,payer_ref,recipient_ref,signer_nonce,quote_digest,payment_mode,payer_generation,
                    confirmer_ref,deliver_by,confirm_window,claimable_at,resolve_by,reason
                HAVING count(DISTINCT generation_id)<>2
            )
            SELECT (SELECT count(*) FROM (SELECT DISTINCT block_hash,tx_hash,log_index FROM compared) differing),
                   COALESCE(sum(CASE WHEN generation_id=p_job THEN amount ELSE -amount END) FILTER(WHERE kind IN ('paid','locked')),0),
                   COALESCE(sum(CASE WHEN generation_id=p_job THEN amount ELSE -amount END) FILTER(WHERE kind IN ('refunded','cancelled')),0)
            INTO logs_delta,pays_delta,refunds_delta FROM facts;
        END;
    END IF;
    IF proof IS NOT NULL THEN logs_delta:=0; pays_delta:=0; refunds_delta:=0; END IF;
    UPDATE chainview.replay_job SET state=CASE WHEN proof IS NOT NULL THEN 'matched' ELSE 'diff' END,
           reason=diff_reason,comparison_id=proof,finished_at=clock_timestamp(),
           differing_logs=logs_delta,paid_delta=pays_delta,refund_delta=refunds_delta WHERE id=p_job;
END
$$;

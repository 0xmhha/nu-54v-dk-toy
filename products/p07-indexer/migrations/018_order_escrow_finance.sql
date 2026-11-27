-- Anyone can make the escrow contract report a skipped claim. Order reads must not scan them.
CREATE INDEX evidence_order_lookup ON chainview.evidence(generation_id,merchant_key,order_key)
    WHERE kind<>'claim-skipped';
CREATE INDEX evidence_skip_lookup ON chainview.evidence(generation_id,merchant_key,order_key,anchor_height,tx_hash,log_index)
    WHERE kind='claim-skipped';
CREATE INDEX event_money ON chainview.observed_event(generation_id,merchant_key)
    WHERE kind IN ('paid','refunded','locked','released','cancelled');

CREATE VIEW chainview.escrow_fact WITH (security_barrier=true) AS
SELECT e.id,e.kind,s.chain_ref,s.contract_ref,s.abi_hash,e.policy_ref,
    log.payer_ref,log.recipient_ref AS beneficiary_ref,log.confirmer_ref,log.amount,log.quote_digest,
    log.deliver_by,log.confirm_window,log.claimable_at,log.resolve_by,log.reason,
    e.tx_hash,e.log_block AS inclusion_hash,included.height AS block_height,e.log_index,included.block_time AS included_at,
    t.stream_id,e.merchant_key,e.order_key,e.anchor_height
FROM chainview.current_evidence e
JOIN chainview.read_target t ON t.generation_id=e.generation_id
JOIN chainview.stream s ON s.id=t.stream_id
JOIN chainview.observed_event log ON (log.generation_id,log.block_hash,log.tx_hash,log.log_index)
    =(e.generation_id,e.log_block,e.tx_hash,e.log_index)
JOIN chainview.block included ON (included.generation_id,included.block_hash)=(log.generation_id,log.block_hash);

-- The contract bounds the other facts of one order to a handful. Skipped claims are the most recently
-- confirmed 20; the result of one claim transaction is complete in escrow_transaction.
CREATE OR REPLACE FUNCTION chainview.escrow_receipt(p_stream uuid, p_merchant text, p_order text)
    RETURNS TABLE(id uuid, kind text, chain_ref text, contract_ref text, abi_hash text, policy_ref text,
        payer_ref text, beneficiary_ref text, confirmer_ref text, amount numeric, quote_digest text,
        deliver_by bigint, confirm_window bigint, claimable_at bigint, resolve_by bigint, reason smallint,
        tx_hash text, inclusion_hash text, block_height bigint, log_index int, included_at timestamptz)
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $sql$
    SELECT f.id,f.kind,f.chain_ref,f.contract_ref,f.abi_hash,f.policy_ref,f.payer_ref,f.beneficiary_ref,
        f.confirmer_ref,f.amount,f.quote_digest,f.deliver_by,f.confirm_window,f.claimable_at,f.resolve_by,
        f.reason,f.tx_hash,f.inclusion_hash,f.block_height,f.log_index,f.included_at
    FROM chainview.escrow_fact f
    WHERE f.id = ANY(ARRAY(
            SELECT e.id FROM chainview.evidence e
            WHERE e.generation_id=(SELECT t.generation_id FROM chainview.read_target t WHERE t.stream_id=p_stream)
              AND e.merchant_key=p_merchant AND e.order_key=p_order
              AND e.kind IN ('locked','delivered','disputed','released','cancelled'))
        || ARRAY(
            SELECT e.id FROM chainview.evidence e
            WHERE e.generation_id=(SELECT t.generation_id FROM chainview.read_target t WHERE t.stream_id=p_stream)
              AND e.merchant_key=p_merchant AND e.order_key=p_order AND e.kind='claim-skipped'
            ORDER BY e.anchor_height DESC,e.tx_hash DESC,e.log_index DESC LIMIT 20))
        AND f.stream_id=p_stream
        AND EXISTS (SELECT FROM chainview.receipt_scope scope WHERE scope.login_role=session_user
            AND scope.stream_id=p_stream AND scope.merchant_key=p_merchant AND scope.valid_until>statement_timestamp())
    ORDER BY f.block_height,f.log_index
$sql$;

-- Every order fact of the merchant in one transaction, so a batch claim is settled from its own receipt.
CREATE FUNCTION chainview.escrow_transaction(p_stream uuid, p_merchant text, p_tx text)
    RETURNS TABLE(order_ref text, id uuid, kind text, chain_ref text, contract_ref text, abi_hash text, policy_ref text,
        payer_ref text, beneficiary_ref text, confirmer_ref text, amount numeric, quote_digest text,
        deliver_by bigint, confirm_window bigint, claimable_at bigint, resolve_by bigint, reason smallint,
        tx_hash text, inclusion_hash text, block_height bigint, log_index int, included_at timestamptz)
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $sql$
    SELECT f.order_key,f.id,f.kind,f.chain_ref,f.contract_ref,f.abi_hash,f.policy_ref,f.payer_ref,f.beneficiary_ref,
        f.confirmer_ref,f.amount,f.quote_digest,f.deliver_by,f.confirm_window,f.claimable_at,f.resolve_by,
        f.reason,f.tx_hash,f.inclusion_hash,f.block_height,f.log_index,f.included_at
    FROM chainview.escrow_fact f
    WHERE f.stream_id=p_stream AND f.merchant_key=p_merchant AND f.tx_hash=p_tx
        AND f.kind IN ('locked','delivered','disputed','released','cancelled','claim-skipped')
        AND EXISTS (SELECT FROM chainview.receipt_scope scope WHERE scope.login_role=session_user
            AND scope.stream_id=p_stream AND scope.merchant_key=p_merchant AND scope.valid_until>statement_timestamp())
    ORDER BY f.block_height,f.log_index
$sql$;
REVOKE ALL ON FUNCTION chainview.escrow_transaction(uuid,text,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION chainview.escrow_transaction(uuid,text,text) TO nu54_p07_reconcile;

-- A settlement day keeps its exact output. An escrow day has its own fields, never 'gross' or 'refunds',
-- so a reader of settlement days cannot take it for a proof of no sales.
CREATE OR REPLACE FUNCTION chainview.finance_day_evidence(stream_ uuid,merchant_ text,asset_ text,from_ timestamptz,until_ timestamptz,
    chain_ text,contract_ text,abi_ text,manifest_ text)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    generation_ uuid;
    final_hash text;
    final_height bigint;
    proof jsonb;
    escrow boolean;
BEGIN
    IF NOT isfinite(from_) OR NOT isfinite(until_) OR until_<from_+interval '22 hours'
       OR until_>from_+interval '26 hours' OR asset_ !~ '^0x[0-9a-f]{40}$'
       OR NOT chainview.reconciliation_ready(stream_,merchant_) THEN
        RETURN NULL;
    END IF;
    SELECT g.id,g.finalized_hash,g.finalized_height,p.version=3 INTO generation_,final_hash,final_height,escrow
      FROM chainview.read_target t JOIN chainview.generation g ON g.id=t.generation_id
      JOIN chainview.stream s ON s.id=t.stream_id
      LEFT JOIN chainview.deployment_profile p ON p.stream_id=s.id
      JOIN chainview.block f ON f.generation_id=g.id AND f.block_hash=g.finalized_hash AND f.canonical
      JOIN chainview.block b ON b.generation_id=g.id AND b.height=g.base_height AND b.canonical
      WHERE t.stream_id=stream_ AND (s.chain_ref,s.contract_ref,s.abi_hash,s.manifest_ref)=(chain_,contract_,abi_,manifest_)
        AND b.block_time<=from_ AND f.block_time>=until_;
    IF NOT FOUND THEN RETURN NULL; END IF;
    IF escrow THEN
        SELECT jsonb_build_object('anchor',final_hash,'height',final_height,
            'locked',trim_scale(coalesce(sum(e.amount) FILTER (WHERE e.kind='locked'),0))::text,
            'lockedCount',count(*) FILTER (WHERE e.kind='locked'),
            'released',trim_scale(coalesce(sum(e.amount) FILTER (WHERE e.kind='released'),0))::text,
            'releasedCount',count(*) FILTER (WHERE e.kind='released'),
            'cancelled',trim_scale(coalesce(sum(e.amount) FILTER (WHERE e.kind='cancelled'),0))::text,
            'cancelledCount',count(*) FILTER (WHERE e.kind='cancelled'),
            'count',count(*),
            'digest',encode(sha256(convert_to(coalesce(jsonb_agg(jsonb_build_array(
                e.kind,e.tx_hash,e.log_index,e.block_hash,trim_scale(e.amount)::text)
                ORDER BY e.tx_hash COLLATE "C",e.log_index,e.block_hash COLLATE "C"),'[]'::jsonb)::text,'UTF8')),'hex'))
          INTO proof
          FROM chainview.observed_event e JOIN chainview.block b
            ON b.generation_id=e.generation_id AND b.block_hash=e.block_hash
          WHERE e.generation_id=generation_ AND b.canonical AND b.height<=final_height
            AND b.block_time>=from_ AND b.block_time<until_ AND e.receipt_success
            AND e.merchant_key=merchant_ AND e.asset_ref=asset_ AND e.kind IN ('locked','released','cancelled');
        RETURN proof;
    END IF;
    SELECT jsonb_build_object('anchor',final_hash,'height',final_height,
        'gross',trim_scale(coalesce(sum(e.amount) FILTER (WHERE e.kind='paid'),0))::text,
        'refunds',trim_scale(coalesce(sum(e.amount) FILTER (WHERE e.kind='refunded'),0))::text,
        'count',count(*),
        'digest',encode(sha256(convert_to(coalesce(jsonb_agg(jsonb_build_array(
            e.kind,e.tx_hash,e.log_index,e.block_hash,trim_scale(e.amount)::text)
            ORDER BY e.tx_hash COLLATE "C",e.log_index,e.block_hash COLLATE "C"),'[]'::jsonb)::text,'UTF8')),'hex'))
      INTO proof
      FROM chainview.observed_event e JOIN chainview.block b
        ON b.generation_id=e.generation_id AND b.block_hash=e.block_hash
      WHERE e.generation_id=generation_ AND b.canonical AND b.height<=final_height
        AND b.block_time>=from_ AND b.block_time<until_ AND e.receipt_success
        AND e.merchant_key=merchant_ AND e.asset_ref=asset_ AND e.kind IN ('paid','refunded');
    RETURN proof;
END $$;

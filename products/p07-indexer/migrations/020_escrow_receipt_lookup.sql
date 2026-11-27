-- Look up the order's evidence ids first, then read the facts by that one array. With the ids
-- computed inside the same statement, a CI PostgreSQL 16 server applied the id filter only above
-- the whole escrow_fact view, so every skipped claim of the stream was joined on each read.
-- The second query is planned on every call with the ids in hand: a plan kept from the first read,
-- made while the tables were nearly empty, scanned all evidence once they had grown.
CREATE OR REPLACE FUNCTION chainview.escrow_receipt(p_stream uuid, p_merchant text, p_order text)
    RETURNS TABLE(id uuid, kind text, chain_ref text, contract_ref text, abi_hash text, policy_ref text,
        payer_ref text, beneficiary_ref text, confirmer_ref text, amount numeric, quote_digest text,
        deliver_by bigint, confirm_window bigint, claimable_at bigint, resolve_by bigint, reason smallint,
        tx_hash text, inclusion_hash text, block_height bigint, log_index int, included_at timestamptz)
    LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $fn$
#variable_conflict use_column
DECLARE
    wanted uuid[];
BEGIN
    wanted := ARRAY(
            SELECT e.id FROM chainview.evidence e
            WHERE e.generation_id=(SELECT t.generation_id FROM chainview.read_target t WHERE t.stream_id=p_stream)
              AND e.merchant_key=p_merchant AND e.order_key=p_order
              AND e.kind IN ('locked','delivered','disputed','released','cancelled'))
        || ARRAY(
            SELECT e.id FROM chainview.evidence e
            WHERE e.generation_id=(SELECT t.generation_id FROM chainview.read_target t WHERE t.stream_id=p_stream)
              AND e.merchant_key=p_merchant AND e.order_key=p_order AND e.kind='claim-skipped'
            ORDER BY e.anchor_height DESC,e.tx_hash DESC,e.log_index DESC LIMIT 20);
    RETURN QUERY EXECUTE
    'SELECT f.id,f.kind,f.chain_ref,f.contract_ref,f.abi_hash,f.policy_ref,f.payer_ref,f.beneficiary_ref,
        f.confirmer_ref,f.amount,f.quote_digest,f.deliver_by,f.confirm_window,f.claimable_at,f.resolve_by,
        f.reason,f.tx_hash,f.inclusion_hash,f.block_height,f.log_index,f.included_at
    FROM chainview.escrow_fact f
    WHERE f.id = ANY($1)
        AND f.stream_id=$2
        AND EXISTS (SELECT FROM chainview.receipt_scope scope WHERE scope.login_role=session_user
            AND scope.stream_id=$2 AND scope.merchant_key=$3 AND scope.valid_until>statement_timestamp())
    ORDER BY f.block_height,f.log_index'
    USING wanted, p_stream, p_merchant;
END
$fn$;

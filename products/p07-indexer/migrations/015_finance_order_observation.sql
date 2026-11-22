-- Expose the exact order's canonical heights through the existing restricted reader role.
CREATE FUNCTION chainview.finance_order_observation(
    stream_ uuid, merchant_ text, order_ text, digest_ text, tx_ text,
    contract_ text, abi_ text, manifest_ text, asset_ text, amount_ numeric,
    paid_block_ text, paid_log_ integer)
RETURNS TABLE (observed_height bigint, confirmed_height bigint)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT b.height,r.anchor_height
    FROM chainview.receipt_scope scope
    JOIN chainview.stream s ON s.id=scope.stream_id
    JOIN chainview.read_target t ON t.stream_id=s.id
    JOIN chainview.generation g ON g.id=t.generation_id
    JOIN chainview.observed_event e ON e.generation_id=g.id
      AND e.merchant_key=scope.merchant_key AND e.order_key=order_ AND e.tx_hash=tx_
    JOIN chainview.block b ON b.generation_id=e.generation_id AND b.block_hash=e.block_hash
    LEFT JOIN chainview.finalized_receipt r ON r.generation_id=e.generation_id AND r.inclusion_hash=e.block_hash
      AND r.tx_hash=e.tx_hash AND r.log_index=e.log_index AND r.policy_ref=manifest_
      AND e.block_hash=paid_block_ AND e.log_index=paid_log_
    WHERE scope.login_role=session_user AND scope.valid_until>statement_timestamp()
      AND scope.stream_id=stream_ AND scope.merchant_key=merchant_
      AND s.chain_ref='8283' AND s.contract_ref=contract_ AND s.abi_hash=abi_ AND s.manifest_ref=manifest_
      AND g.state='ready' AND b.canonical AND b.height<=g.tip_height
      AND NOT EXISTS (SELECT FROM chainview.incident i JOIN chainview.generation affected ON affected.id=i.generation_id
        WHERE affected.stream_id=stream_ AND i.resolved_at IS NULL)
      AND e.kind='paid' AND e.receipt_success AND e.quote_digest=digest_ AND e.asset_ref=asset_ AND e.amount=amount_
      AND (paid_block_ IS NULL OR r.id IS NOT NULL)
    ORDER BY e.log_index LIMIT 1
$$;

REVOKE ALL ON FUNCTION chainview.finance_order_observation(uuid,text,text,text,text,text,text,text,text,numeric,text,integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION chainview.finance_order_observation(uuid,text,text,text,text,text,text,text,text,numeric,text,integer) TO nu54_p07_reconcile;

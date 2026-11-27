-- Expose a derived checkpoint of the active, uninterrupted canonical prefix.
-- The explorer receives neither generation IDs nor raw block/event access.
CREATE FUNCTION chainview.explorer_quote_coverage(p_stream uuid)
RETURNS TABLE(chain_ref text,contract_ref text,asset_ref text,manifest_ref text,abi_hash text,code_hash text,
              base_height bigint,anchor_height bigint,anchor_hash text,anchored_at timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT s.chain_ref,s.contract_ref,p.asset_ref,s.manifest_ref,s.abi_hash,p.code_hash,
           g.base_height,g.finalized_height,b.block_hash,b.block_time
      FROM chainview.stream s JOIN chainview.deployment_profile p ON p.stream_id=s.id
      JOIN chainview.read_target t ON t.stream_id=s.id
      JOIN chainview.generation g ON g.id=t.generation_id
      JOIN chainview.block b ON b.generation_id=g.id AND b.height=g.finalized_height
                            AND b.block_hash=g.finalized_hash AND b.canonical
      WHERE s.id=p_stream AND p.version=2 AND g.state='ready'
        AND g.base_height=p.start_height
        AND chainview.complete_prefix(g.id,g.finalized_height)
        AND NOT EXISTS (SELECT FROM chainview.incident i JOIN chainview.generation affected ON affected.id=i.generation_id
                         WHERE affected.stream_id=t.stream_id AND i.resolved_at IS NULL)
$$;
REVOKE ALL ON FUNCTION chainview.explorer_quote_coverage(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION chainview.explorer_quote_coverage(uuid) TO nu54_p07_explorer;

CREATE OR REPLACE VIEW chainview.explorer_boundary WITH (security_barrier=true) AS
SELECT t.stream_id,e.id,e.kind,e.merchant_key,e.order_key,e.tx_hash,e.log_index,
       r.inclusion_height,r.included_at,e.created_at,e.anchor_height,e.anchor_hash,e.expires_at,e.coverage_start,
       e.state_proof_ref,log.quote_issued_at,log.quote_expires_at,
       CASE WHEN log.quote_expires_at IS NOT NULL THEN log.quote_digest END AS quote_digest,
       e.policy_ref,anchor.block_time AS anchored_at,log.quote_digest AS event_quote_digest
FROM chainview.active_evidence e
JOIN chainview.read_target t ON t.generation_id=e.generation_id
JOIN chainview.block anchor ON anchor.generation_id=e.generation_id AND anchor.block_hash=e.anchor_hash
LEFT JOIN chainview.finalized_receipt r ON r.id=e.id
LEFT JOIN chainview.observed_event log ON (log.generation_id,log.block_hash,log.tx_hash,log.log_index)
  =(e.generation_id,e.log_block,e.tx_hash,e.log_index)
WHERE e.kind IN ('paid','absent','invalidated');

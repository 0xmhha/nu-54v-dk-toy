ALTER TABLE chainview.observed_event
ADD COLUMN quote_issued_at timestamptz,
ADD COLUMN quote_expires_at timestamptz,
ADD CONSTRAINT quote_window_complete CHECK (
    (quote_issued_at IS NULL AND quote_expires_at IS NULL)
    OR (kind='paid' AND quote_digest IS NOT NULL AND quote_issued_at IS NOT NULL
        AND quote_expires_at IS NOT NULL AND quote_issued_at<quote_expires_at)
);

CREATE OR REPLACE VIEW chainview.explorer_boundary WITH (security_barrier=true) AS
SELECT t.stream_id,e.id,e.kind,e.merchant_key,e.order_key,e.tx_hash,e.log_index,
       r.inclusion_height,r.included_at,e.created_at,e.anchor_height,e.anchor_hash,e.expires_at,e.coverage_start,
       e.state_proof_ref,log.quote_issued_at,log.quote_expires_at,
       CASE WHEN log.quote_expires_at IS NOT NULL THEN log.quote_digest END AS quote_digest
FROM chainview.active_evidence e
JOIN chainview.read_target t ON t.generation_id=e.generation_id
LEFT JOIN chainview.finalized_receipt r ON r.id=e.id
LEFT JOIN chainview.observed_event log ON (log.generation_id,log.block_hash,log.tx_hash,log.log_index)
  =(e.generation_id,e.log_block,e.tx_hash,e.log_index)
WHERE e.kind IN ('paid','absent','invalidated');

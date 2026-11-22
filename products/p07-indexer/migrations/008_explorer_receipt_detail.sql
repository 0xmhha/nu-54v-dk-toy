ALTER TABLE chainview.deployment_profile
ADD COLUMN asset_decimals smallint CHECK (asset_decimals BETWEEN 0 AND 255);
GRANT UPDATE (asset_decimals) ON chainview.deployment_profile TO nu54_p07_ingest;

-- The projection exposes a local-comparison digest for a v2 recipient,
-- rather than disclosing the recipient or payer address.
CREATE VIEW chainview.explorer_receipt_detail WITH (security_barrier=true) AS
SELECT r.id,r.stream_id,r.kind,r.merchant_key,r.order_key,r.refund_key,r.asset_ref,r.amount,
       r.tx_hash,r.log_index,r.inclusion_height,r.included_at,r.anchor_height,r.attested_at,
       CASE WHEN p.version=2 AND r.kind='paid'
            AND log.recipient_ref ~ '^0x[0-9a-f]{40}$'
       THEN encode(sha256(convert_to(concat_ws('|','nu54-explorer-recipient-v1',r.chain_ref,
           lower(r.contract_ref),lower(r.tx_hash),r.log_index::text,log.recipient_ref),'UTF8')),'hex')
       END AS recipient_commitment
FROM chainview.finalized_receipt r
JOIN chainview.deployment_profile p ON p.stream_id=r.stream_id
JOIN chainview.evidence e ON e.id=r.id
JOIN chainview.observed_event log ON (log.generation_id,log.block_hash,log.tx_hash,log.log_index)
  =(e.generation_id,e.log_block,e.tx_hash,e.log_index)
WHERE r.kind IN ('paid','refunded');

CREATE VIEW chainview.explorer_units WITH (security_barrier=true) AS
SELECT stream_id,asset_decimals FROM chainview.deployment_profile;
GRANT SELECT ON chainview.explorer_receipt_detail,chainview.explorer_units TO nu54_p07_public;
CREATE INDEX explorer_evidence_transaction ON chainview.evidence(tx_hash,generation_id,log_index)
WHERE tx_hash IS NOT NULL;

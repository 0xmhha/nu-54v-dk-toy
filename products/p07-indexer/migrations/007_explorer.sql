ALTER TABLE chainview.generation ADD COLUMN observed_at timestamptz;
ALTER TABLE chainview.generation ADD COLUMN observed_finalized_height bigint CHECK (observed_finalized_height>=0);
ALTER TABLE chainview.generation ADD COLUMN fork_ancestor bigint CHECK (fork_ancestor>=0);
ALTER TABLE chainview.generation ADD COLUMN fork_observed_at timestamptz;

CREATE VIEW chainview.explorer_deployment WITH (security_barrier=true) AS
SELECT s.id AS stream_id,s.chain_ref,s.contract_ref,p.version,p.asset_ref,
       g.tip_height,g.finalized_height,g.observed_finalized_height,g.observed_at,
       CASE WHEN g.state='frozen' OR EXISTS (
          SELECT FROM chainview.incident i JOIN chainview.generation affected ON affected.id=i.generation_id
          WHERE affected.stream_id=s.id AND i.resolved_at IS NULL
       ) THEN 'held'
       WHEN g.observed_at IS NULL OR g.observed_at<clock_timestamp()-interval '30 seconds'
            OR g.finalized_height IS NULL OR g.finalized_height<g.observed_finalized_height THEN 'catching_up'
       ELSE 'current' END AS status
FROM chainview.stream s JOIN chainview.deployment_profile p ON p.stream_id=s.id
LEFT JOIN chainview.read_target t ON t.stream_id=s.id
LEFT JOIN chainview.generation g ON g.id=t.generation_id;

CREATE VIEW chainview.explorer_monitor WITH (security_barrier=true) AS
SELECT d.*,g.base_height,g.tip_hash,g.finalized_hash,g.fork_ancestor,g.fork_observed_at,
       i.id AS incident_ref,i.disputed_height,i.old_hash,i.new_hash,i.opened_at
FROM chainview.explorer_deployment d
LEFT JOIN chainview.read_target t ON t.stream_id=d.stream_id
LEFT JOIN chainview.generation g ON g.id=t.generation_id
LEFT JOIN LATERAL (
    SELECT i.id,i.disputed_height,i.old_hash,i.new_hash,i.opened_at
    FROM chainview.incident i JOIN chainview.generation affected ON affected.id=i.generation_id
    WHERE affected.stream_id=d.stream_id AND i.resolved_at IS NULL
    ORDER BY i.opened_at DESC,i.id LIMIT 1
) i ON true;

CREATE VIEW chainview.explorer_boundary WITH (security_barrier=true) AS
SELECT t.stream_id,e.id,e.kind,e.merchant_key,e.order_key,e.tx_hash,e.log_index,
       r.inclusion_height,r.included_at,e.created_at,e.anchor_height,e.anchor_hash,e.expires_at,e.coverage_start,
       e.state_proof_ref
FROM chainview.active_evidence e
JOIN chainview.read_target t ON t.generation_id=e.generation_id
LEFT JOIN chainview.finalized_receipt r ON r.id=e.id
WHERE e.kind IN ('paid','absent','invalidated');

DO $roles$
BEGIN
    IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='nu54_p07_explorer') THEN
        CREATE ROLE nu54_p07_explorer NOLOGIN;
    END IF;
    IF EXISTS(SELECT FROM pg_roles WHERE rolname='nu54_p07_explorer'
        AND (rolcanlogin OR rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb OR rolreplication)) THEN
        RAISE EXCEPTION 'unsafe explorer role';
    END IF;
END
$roles$;
GRANT SELECT ON chainview.explorer_deployment TO nu54_p07_public;
GRANT USAGE ON SCHEMA chainview TO nu54_p07_explorer;
GRANT SELECT ON chainview.explorer_monitor,chainview.explorer_boundary TO nu54_p07_explorer;
GRANT UPDATE (observed_at,observed_finalized_height,fork_ancestor,fork_observed_at)
ON chainview.generation TO nu54_p07_ingest;

-- Publish only the revision of a deployment's active read target, not its
-- internal generation ID. The browser server rechecks it before release.
CREATE VIEW chainview.explorer_read_revision WITH (security_barrier=true) AS
SELECT d.stream_id,t.revision,d.status
FROM chainview.explorer_deployment d
LEFT JOIN chainview.read_target t ON t.stream_id=d.stream_id;
GRANT SELECT ON chainview.explorer_read_revision TO nu54_p07_public,nu54_p07_explorer;

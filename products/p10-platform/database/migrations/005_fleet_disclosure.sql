-- Older investigation records remain sealed until their owner assigns a cohort.
ALTER TABLE platform.fleet_advisory ADD COLUMN cohort uuid REFERENCES platform.fleet_cohort;
ALTER TABLE platform.fleet_advisory ADD COLUMN published_job uuid;
ALTER TABLE platform.fleet_advisory ADD FOREIGN KEY (cohort,published_job)
    REFERENCES platform.fleet_operation(cohort,id);
ALTER TABLE platform.fleet_cohort ADD COLUMN catalog_public boolean NOT NULL DEFAULT false;

CREATE VIEW platform.fleet_public_cohort WITH (security_barrier=true) AS
SELECT c.id, c.revision, c.current AS manifest, b.signer,
       b.metadata::jsonb->>'channel' AS channel
FROM platform.fleet_cohort c JOIN platform.fleet_candidate b ON b.id=c.current
WHERE c.catalog_public
AND NOT EXISTS (SELECT FROM platform.fleet_revocation r WHERE (r.cohort,r.key)=(c.id,b.signer))
AND NOT EXISTS (SELECT FROM jsonb_each(b.metadata::jsonb->'artifacts') item
                WHERE COALESCE((item.value->>'fixtureEvidence')::boolean,true));

CREATE VIEW platform.fleet_public_target WITH (security_barrier=true) AS
SELECT DISTINCT pub.id AS cohort,r.claim::jsonb->'receipt'->>'artifact' AS name,
       r.claim::jsonb->'receipt'->'dimensions' AS dimensions,
       r.claim::jsonb->'receipt'->>'version' AS version,r.claim::jsonb->'receipt'->>'mode' AS mode,
       r.claim::jsonb->'receipt'->>'payload' AS payload,r.claim::jsonb->'receipt'->'counter' AS counter,
       o.result AS observation
FROM platform.fleet_public_cohort pub JOIN platform.fleet_cohort c ON c.id=pub.id
JOIN platform.fleet_operation o ON (o.cohort,o.id)=(c.id,c.active_job)
JOIN platform.fleet_receipt r ON r.job=o.id
JOIN platform.fleet_candidate b ON b.id=c.current
WHERE COALESCE((b.metadata::jsonb->'artifacts'->(r.claim::jsonb->'receipt'->>'artifact')->>'fixtureEvidence')::boolean,true)=false;

CREATE VIEW platform.fleet_public_advisory WITH (security_barrier=true) AS
SELECT a.cohort,a.id,a.report::jsonb->>'component' AS component,
       a.report::jsonb->'affected' AS affected,a.report::jsonb->'newVersions' AS versions,
       a.report::jsonb->>'candidate' AS manifest
FROM platform.fleet_advisory a JOIN platform.fleet_public_cohort pub ON pub.id=a.cohort
JOIN platform.fleet_operation o ON (o.cohort,o.id)=(a.cohort,a.published_job)
WHERE o.result IS NOT NULL AND a.report::jsonb->>'candidate'=pub.manifest
AND EXISTS (SELECT FROM platform.fleet_public_target installed WHERE installed.cohort=a.cohort);

REVOKE SELECT ON platform.fleet_candidate,platform.fleet_cohort,platform.fleet_receipt,
    platform.fleet_revocation,platform.fleet_advisory FROM nu54_p10_fleet_catalog;
GRANT SELECT ON platform.fleet_public_cohort,platform.fleet_public_target,platform.fleet_public_advisory
    TO nu54_p10_fleet_catalog;
DO $roles$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p10_fleet_review') THEN
        CREATE ROLE nu54_p10_fleet_review NOLOGIN NOSUPERUSER NOBYPASSRLS;
    ELSIF EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p10_fleet_review'
        AND (rolcanlogin OR rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication OR rolbypassrls)) THEN
        RAISE EXCEPTION 'unsafe fleet reviewer capability role';
    END IF;
END
$roles$;
CREATE TABLE platform.fleet_review_enrollment (
    login_role name NOT NULL,
    cohort uuid NOT NULL REFERENCES platform.fleet_cohort,
    expires_at timestamptz NOT NULL CHECK (isfinite(expires_at)),
    PRIMARY KEY(login_role,cohort)
);
GRANT USAGE ON SCHEMA platform TO nu54_p10_fleet_review;
GRANT UPDATE(published_job) ON platform.fleet_advisory TO nu54_p10_fleet_service;
CREATE FUNCTION platform.fleet_review_snapshot(scope_ uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $review$
    SELECT jsonb_build_object('advisories',COALESCE((
      SELECT jsonb_agg(a.report::jsonb ORDER BY a.id) FROM platform.fleet_advisory a WHERE a.cohort=scope_
    ),'[]'::jsonb),'pending',(
      SELECT jsonb_build_object('manifest',b.id,'metadata',b.metadata::jsonb,'receipts',COALESCE((
        SELECT jsonb_agg(r.claim::jsonb ORDER BY r.target) FROM platform.fleet_receipt r WHERE r.job=o.id
      ),'[]'::jsonb))
      FROM platform.fleet_cohort c JOIN platform.fleet_operation o ON (o.cohort,o.id)=(c.id,c.pending)
      JOIN platform.fleet_candidate b ON b.id=o.bundle::jsonb->'plan'->>'candidate' WHERE c.id=scope_
    ))
    WHERE EXISTS (SELECT FROM platform.fleet_review_enrollment e
      WHERE e.login_role=session_user AND e.cohort=scope_ AND e.expires_at>statement_timestamp())
$review$;
REVOKE ALL ON FUNCTION platform.fleet_review_snapshot(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.fleet_review_snapshot(uuid) TO nu54_p10_fleet_review;

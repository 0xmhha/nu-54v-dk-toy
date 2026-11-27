ALTER TABLE platform.fleet_operation ADD COLUMN superseded_by uuid;
ALTER TABLE platform.fleet_operation ADD FOREIGN KEY(cohort,superseded_by)
    REFERENCES platform.fleet_operation(cohort,id);
ALTER TABLE platform.fleet_operation ADD CHECK(superseded_by IS NULL OR (superseded_by<>id AND result IS NULL));

CREATE OR REPLACE FUNCTION platform.fleet_review_snapshot(scope_ uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $review$
    SELECT jsonb_build_object('advisories',COALESCE((
      SELECT jsonb_agg(a.report::jsonb ORDER BY a.id) FROM platform.fleet_advisory a WHERE a.cohort=scope_
    ),'[]'::jsonb),'pending',(
      SELECT jsonb_build_object('manifest',b.id,'metadata',b.metadata::jsonb,
        'state',CASE WHEN EXISTS(SELECT FROM platform.fleet_revocation rv WHERE rv.cohort=c.id AND rv.key=b.signer) THEN 'revoked'
                     WHEN to_timestamp((o.bundle::jsonb->'plan'->>'expires')::bigint)<=clock_timestamp() THEN 'expired'
                     ELSE 'installing' END,
        'receipts',COALESCE((SELECT jsonb_agg(r.claim::jsonb ORDER BY r.target) FROM platform.fleet_receipt r WHERE r.job=o.id),'[]'::jsonb))
      FROM platform.fleet_cohort c JOIN platform.fleet_operation o ON (o.cohort,o.id)=(c.id,c.pending)
      JOIN platform.fleet_candidate b ON b.id=o.bundle::jsonb->'plan'->>'candidate' WHERE c.id=scope_
    ))
    WHERE EXISTS (SELECT FROM platform.fleet_review_enrollment e
      WHERE e.login_role=session_user AND e.cohort=scope_ AND e.expires_at>clock_timestamp())
$review$;

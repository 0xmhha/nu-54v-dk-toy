-- Disclose iOS profile scope from the current signed installation only.
CREATE OR REPLACE VIEW platform.fleet_public_target WITH (security_barrier=true) AS
SELECT DISTINCT pub.id AS cohort,r.claim::jsonb->'receipt'->>'artifact' AS name,
       r.claim::jsonb->'receipt'->'dimensions' AS dimensions,
       r.claim::jsonb->'receipt'->>'version' AS version,r.claim::jsonb->'receipt'->>'mode' AS mode,
       r.claim::jsonb->'receipt'->>'payload' AS payload,r.claim::jsonb->'receipt'->'counter' AS counter,
       o.result AS observation,
       r.claim::jsonb->'receipt'->>'bootKey' AS boot_key,
       r.claim::jsonb->'receipt'->>'measurement' AS measurement,
       CASE WHEN r.claim::jsonb->'receipt'->>'mode'='ios-simulator'
            THEN COALESCE(r.claim::jsonb->'receipt'->>'profile','fresh-install-only') END AS ios_profile
FROM platform.fleet_public_cohort pub JOIN platform.fleet_cohort c ON c.id=pub.id
JOIN platform.fleet_operation o ON (o.cohort,o.id)=(c.id,c.active_job)
JOIN platform.fleet_receipt r ON r.job=o.id
JOIN platform.fleet_candidate b ON b.id=c.current
WHERE COALESCE((b.metadata::jsonb->'artifacts'->(r.claim::jsonb->'receipt'->>'artifact')->>'fixtureEvidence')::boolean,true)=false;

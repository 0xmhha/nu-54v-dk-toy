DO $roles$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p05_benefit_read') THEN
        CREATE ROLE nu54_p05_benefit_read NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
    END IF;
END
$roles$;

CREATE FUNCTION ledger.visit_benefit_status(issuer_ text,proof_ uuid)
RETURNS TABLE(credited bigint,adjusted bigint,uncredited bigint)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp
AS $status$
 SELECT count(*) FILTER(WHERE s.credited),
        count(*) FILTER(WHERE NOT s.credited AND (s.classification='adjusted' OR EXISTS(
          SELECT FROM ledger.visit_benefit_effect f WHERE (f.issuer,f.proof,f.campaign)=(s.issuer,s.proof,s.campaign) AND f.delta=-1))),
        count(*) FILTER(WHERE NOT s.credited)
 FROM ledger.visit_benefit_state s
 JOIN ledger.visit_benefit_enrollment e USING(issuer,proof,campaign)
 WHERE (s.issuer,s.proof)=(issuer_,proof_) AND e.environment='live'
$status$;

REVOKE ALL ON FUNCTION ledger.visit_benefit_status(text,uuid) FROM PUBLIC;
GRANT USAGE ON SCHEMA ledger TO nu54_p05_benefit_read;
GRANT EXECUTE ON FUNCTION ledger.visit_benefit_status(text,uuid) TO nu54_p05_benefit_read;

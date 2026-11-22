CREATE TABLE travel_private.visit_benefit_assertion (
    issuer text NOT NULL,
    proof uuid NOT NULL,
    revision bigint NOT NULL CHECK(revision>0),
    classification text NOT NULL CHECK (classification IN ('eligible','test-only','permission-held','adjusted')),
    PRIMARY KEY(issuer,proof),
    FOREIGN KEY(issuer,proof) REFERENCES travel_private.visit_evidence
);
DO $roles$
BEGIN
    IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='nu54_p09_benefit_assert') THEN
        CREATE ROLE nu54_p09_benefit_assert NOLOGIN NOSUPERUSER NOCREATEROLE NOCREATEDB NOBYPASSRLS;
    END IF;
END
$roles$;

CREATE FUNCTION travel_private.assert_visit_benefit(issuer_ text,proof_ uuid)
RETURNS TABLE(revision bigint,classification text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $assert$
DECLARE evidence travel_private.visit_evidence; previous travel_private.visit_benefit_assertion; stage text;
BEGIN
    SELECT * INTO evidence FROM travel_private.visit_evidence WHERE (issuer,proof)=(issuer_,proof_);
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='benefit scope unavailable'; END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(evidence.subject_ref::text||':challenge',900018));
    SELECT * INTO evidence FROM travel_private.visit_evidence WHERE (issuer,proof)=(issuer_,proof_) FOR UPDATE;
    stage:=CASE WHEN evidence.environment='synthetic' THEN 'test-only'
                WHEN evidence.state<>'settled' THEN 'adjusted'
                WHEN EXISTS(SELECT FROM travel_private.my_visits(evidence.subject_ref) v
                    WHERE v.issuer=issuer_ AND v.proof=proof_ AND v.purpose='challenge' AND v.classification='verified') THEN 'eligible'
                ELSE 'permission-held' END;
    SELECT a.* INTO previous FROM travel_private.visit_benefit_assertion a WHERE (a.issuer,a.proof)=(issuer_,proof_);
    IF NOT FOUND OR previous.classification<>stage THEN
      INSERT INTO travel_private.visit_benefit_assertion VALUES(issuer_,proof_,coalesce(previous.revision,0)+1,stage)
          ON CONFLICT(issuer,proof) DO UPDATE SET revision=excluded.revision,classification=excluded.classification
          RETURNING * INTO previous;
    END IF;
    RETURN QUERY SELECT previous.revision,previous.classification;
END
$assert$;
REVOKE ALL ON FUNCTION travel_private.assert_visit_benefit(text,uuid) FROM PUBLIC;
GRANT USAGE ON SCHEMA travel_private TO nu54_p09_benefit_assert;
GRANT EXECUTE ON FUNCTION travel_private.assert_visit_benefit(text,uuid) TO nu54_p09_benefit_assert;

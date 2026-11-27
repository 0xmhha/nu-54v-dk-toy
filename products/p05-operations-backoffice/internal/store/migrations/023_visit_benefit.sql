CREATE TABLE ledger.visit_benefit_enrollment (
    branch_id uuid NOT NULL,
    sale_id uuid NOT NULL,
    campaign uuid NOT NULL,
    issuer text NOT NULL,
    proof uuid NOT NULL,
    environment text NOT NULL CHECK (environment IN ('synthetic','live')),
    PRIMARY KEY(issuer,proof,campaign),
    UNIQUE(branch_id,sale_id,campaign),
    FOREIGN KEY(branch_id,sale_id) REFERENCES ledger.sale(branch_id,id)
);

CREATE TABLE ledger.visit_benefit_state (
    issuer text NOT NULL,
    proof uuid NOT NULL,
    campaign uuid NOT NULL,
    assertion_revision bigint NOT NULL,
    classification text NOT NULL CHECK (classification IN ('eligible','test-only','permission-held','adjusted')),
    credited boolean NOT NULL,
    PRIMARY KEY(issuer,proof,campaign),
    FOREIGN KEY(issuer,proof,campaign) REFERENCES ledger.visit_benefit_enrollment
);

CREATE TABLE ledger.visit_benefit_effect (
    issuer text NOT NULL,
    proof uuid NOT NULL,
    campaign uuid NOT NULL,
    assertion_revision bigint NOT NULL,
    delta smallint NOT NULL CHECK(delta IN (-1,1)),
    PRIMARY KEY(issuer,proof,campaign,assertion_revision),
    FOREIGN KEY(issuer,proof,campaign) REFERENCES ledger.visit_benefit_enrollment
);

DO $roles$
BEGIN
    IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='nu54_p05_benefit_worker') THEN
      CREATE ROLE nu54_p05_benefit_worker NOLOGIN NOSUPERUSER NOCREATEROLE NOCREATEDB NOBYPASSRLS;
    END IF;
END
$roles$;

CREATE FUNCTION ledger.apply_visit_benefit(issuer_ text, proof_ uuid,campaign_ uuid,version_ bigint,condition_ text, expiry_ timestamptz)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $effect$
DECLARE enrollment ledger.visit_benefit_enrollment; previous ledger.visit_benefit_state; credit boolean;
BEGIN
    IF version_<=0 OR version_ IS NULL OR expiry_ IS NULL OR expiry_<=clock_timestamp() OR
       expiry_>clock_timestamp()+interval '1 minute' OR condition_ NOT IN ('eligible','test-only','permission-held','adjusted') THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='benefit assertion unavailable';
    END IF;
    SELECT * INTO enrollment FROM ledger.visit_benefit_enrollment
      WHERE (issuer,proof,campaign)=(issuer_,proof_,campaign_) FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='benefit scope unavailable'; END IF;
    SELECT * INTO previous FROM ledger.visit_benefit_state WHERE (issuer,proof,campaign)=(issuer_,proof_,campaign_);
    IF FOUND THEN
      IF previous.assertion_revision>version_ OR
        (previous.assertion_revision=version_ AND previous.classification<>condition_) THEN
          RAISE EXCEPTION USING ERRCODE='P1801', MESSAGE='benefit assertion version conflict';
      END IF;
      IF previous.assertion_revision=version_ THEN RETURN false; END IF;
    END IF;
    credit := condition_='eligible' AND enrollment.environment='live' AND EXISTS (
        SELECT FROM ledger.payment p JOIN ledger.sale s ON s.id=p.sale_id
        WHERE s.branch_id=enrollment.branch_id AND s.id=enrollment.sale_id AND s.state='paid' AND p.refunded=0
    );
    IF credit IS DISTINCT FROM coalesce(previous.credited,false) THEN
      INSERT INTO ledger.visit_benefit_effect VALUES(issuer_,proof_,campaign_,version_,CASE WHEN credit THEN 1 ELSE -1 END);
    END IF;
    INSERT INTO ledger.visit_benefit_state VALUES(issuer_,proof_,campaign_,version_,condition_,credit)
        ON CONFLICT(issuer,proof,campaign) DO UPDATE SET assertion_revision=excluded.assertion_revision,
            classification=excluded.classification, credited=excluded.credited;
    RETURN credit IS DISTINCT FROM coalesce(previous.credited,false);
END
$effect$;

REVOKE ALL ON FUNCTION ledger.apply_visit_benefit(text,uuid,uuid,bigint,text,timestamptz) FROM PUBLIC;
GRANT USAGE ON SCHEMA ledger TO nu54_p05_benefit_worker;
GRANT EXECUTE ON FUNCTION ledger.apply_visit_benefit(text,uuid,uuid,bigint,text,timestamptz) TO nu54_p05_benefit_worker;

-- P07 receives a scoped read projection with a distinct runtime database role.
DO $role$
BEGIN
    IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='nu54_p05_rental_authority_read') THEN
        CREATE ROLE nu54_p05_rental_authority_read NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
    END IF;
END
$role$;
GRANT USAGE ON SCHEMA ledger TO nu54_p05_rental_authority_read;

CREATE TABLE ledger.rental_authority_audience (
    branch_id uuid NOT NULL,
    device_ref uuid NOT NULL,
    generation bigint NOT NULL,
    reader_login name NOT NULL,
    audience_certificate bytea NOT NULL CHECK(octet_length(audience_certificate)=32),
    writer_certificate bytea NOT NULL CHECK(octet_length(writer_certificate)=32 AND writer_certificate<>audience_certificate),
    authority_source bytea NOT NULL CHECK(octet_length(authority_source)=32),
    valid_until timestamptz NOT NULL CHECK(isfinite(valid_until)),
    PRIMARY KEY(branch_id,device_ref,generation),
    FOREIGN KEY(branch_id,device_ref,generation) REFERENCES ledger.rental_generation_source ON DELETE CASCADE
);
REVOKE ALL ON ledger.rental_authority_audience FROM PUBLIC,nu54_p05_commerce,
    nu54_p05_rental_authority,nu54_p05_rental_evidence,nu54_p05_rental_authority_read;

CREATE FUNCTION ledger.read_rental_generation_authority(branch uuid,device uuid,generation_ bigint,
    stream_ uuid,stream_generation_ uuid,challenge_ bytea,audience_ bytea,writer_ bytea)
RETURNS TABLE(registration bytea,measurement bytea,deployment bytea,settlement text,account text,
    challenge bytea,registration_deadline timestamptz,identity_key bytea,board bytea,signer text,collector bytea,peer bytea,
    first_nonce bytea,confirmed_digest bytea,registered_at timestamptz,challenge_until timestamptz,valid_until timestamptz)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE authority record;
DECLARE source ledger.rental_generation_source;
DECLARE audience ledger.rental_authority_audience;
DECLARE tenant_ uuid;
DECLARE collector_ bytea;
DECLARE registered_ timestamptz;
BEGIN
    PERFORM ledger.lock_rental_evidence_device(branch,device);
    SELECT * INTO audience FROM ledger.rental_authority_audience a
      WHERE (a.branch_id,a.device_ref,a.generation)=(branch,device,generation_) FOR SHARE;
    IF NOT FOUND OR audience.reader_login<>session_user OR audience.audience_certificate IS DISTINCT FROM audience_
        OR audience.writer_certificate IS DISTINCT FROM writer_ OR audience.valid_until<=clock_timestamp() THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='rental evidence audience denied';
    END IF;
    SELECT * INTO source FROM ledger.rental_generation_source s
      WHERE (s.branch_id,s.device_ref,s.generation)=(branch,device,generation_) FOR SHARE;
    IF NOT FOUND OR (source.stream_ref,source.stream_generation,source.challenge) IS DISTINCT FROM (stream_,stream_generation_,challenge_)
        OR audience.authority_source IS DISTINCT FROM ledger.rental_generation_source_digest(source)
        OR NOT EXISTS (SELECT FROM pg_roles r WHERE r.rolname=source.writer_login AND r.rolcanlogin
          AND pg_has_role(r.oid,'nu54_p05_rental_evidence','USAGE')
          AND NOT pg_has_role(r.oid,'nu54_p05_commerce','MEMBER')
          AND NOT pg_has_role(r.oid,'nu54_p05_rental_authority','MEMBER')
          AND NOT pg_has_role(r.oid,'nu54_p05_rental_authority_read','MEMBER')
          AND NOT (r.rolsuper OR r.rolcreaterole OR r.rolcreatedb OR r.rolbypassrls)) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='rental authority source denied';
    END IF;
    SELECT b.tenant_ref INTO tenant_ FROM ledger.branch b WHERE b.id=branch FOR SHARE;
    SELECT p.collector_certificate INTO collector_ FROM ledger.rental_collector_registration p
      WHERE (p.branch_id,p.device_ref,p.generation)=(branch,device,generation_) FOR SHARE;
    IF NOT FOUND OR audience_ IS NOT DISTINCT FROM collector_ OR writer_ IS NOT DISTINCT FROM collector_ THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='separate rental credentials required';
    END IF;
    SELECT * INTO authority FROM ledger.rental_collector_authority(tenant_,branch,device,generation_,collector_);
    SELECT e.attested_at INTO registered_ FROM ledger.rental_enrollment e
      WHERE (e.branch_id,e.device_ref,e.generation)=(branch,device,generation_) FOR SHARE;
    RETURN QUERY SELECT authority.registration,authority.measurement,authority.deployment,authority.settlement,authority.account,
        authority.challenge,authority.deadline,authority.identity_key,authority.board,authority.signer,authority.collector,
        authority.peer,authority.first_nonce,authority.confirmed_digest,registered_,
        source.challenge_until,least(authority.deadline,audience.valid_until);
END $$;
REVOKE ALL ON FUNCTION ledger.read_rental_generation_authority(uuid,uuid,bigint,uuid,uuid,bytea,bytea,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.read_rental_generation_authority(uuid,uuid,bigint,uuid,uuid,bytea,bytea,bytea)
    TO nu54_p05_rental_authority_read;

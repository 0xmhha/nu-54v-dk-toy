-- Read-only current credential scope. This is independent of frozen settlement evidence.
CREATE FUNCTION ledger.renter_credential_scope(tenant uuid, customer uuid, assignment uuid,
    audience_ bytea, writer_ bytea)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE a ledger.renter_return_assignment;
DECLARE source ledger.rental_generation_source;
DECLARE audience ledger.rental_authority_audience;
DECLARE collector ledger.rental_collector_registration;
DECLARE authority record;
DECLARE current_return jsonb;
DECLARE deadline timestamptz;
BEGIN
    IF octet_length(audience_) IS DISTINCT FROM 32 OR octet_length(writer_) IS DISTINCT FROM 32
       OR audience_ IS NOT DISTINCT FROM writer_ THEN RETURN NULL; END IF;
    SELECT r.* INTO a FROM ledger.renter_return_assignment r JOIN ledger.branch b ON b.id=r.branch_id
        WHERE r.assignment_id=assignment AND r.customer_ref=customer AND b.tenant_ref=tenant;
    IF NOT FOUND THEN RETURN NULL; END IF;
    PERFORM ledger.lock_rental_evidence_device(a.branch_id,a.device_ref);
    current_return:=ledger.read_renter_return(tenant,customer,assignment);
    IF current_return IS NULL THEN RETURN NULL; END IF;
    SELECT s.* INTO source FROM ledger.rental_generation_source s WHERE
        (s.branch_id,s.device_ref,s.generation)=(a.branch_id,a.device_ref,a.generation) FOR SHARE;
    IF NOT FOUND OR (source.account_ref,source.enrollment_digest) IS DISTINCT FROM
        (a.account_ref,a.enrollment_digest) THEN RETURN NULL; END IF;
    SELECT p.* INTO audience FROM ledger.rental_authority_audience p WHERE
        (p.branch_id,p.device_ref,p.generation)=(a.branch_id,a.device_ref,a.generation) FOR SHARE;
    IF NOT FOUND OR audience.audience_certificate IS DISTINCT FROM audience_
       OR audience.writer_certificate IS DISTINCT FROM writer_
       OR audience.authority_source IS DISTINCT FROM ledger.rental_generation_source_digest(source)
       OR audience.valid_until<=clock_timestamp()
       OR NOT EXISTS (SELECT FROM pg_roles r WHERE r.rolname=audience.reader_login AND r.rolcanlogin
          AND pg_has_role(r.oid,'nu54_p05_rental_authority_read','USAGE')
          AND NOT pg_has_role(r.oid,'nu54_p05_commerce','MEMBER')
          AND NOT pg_has_role(r.oid,'nu54_p05_rental_authority','MEMBER')
          AND NOT pg_has_role(r.oid,'nu54_p05_rental_evidence','MEMBER')
          AND NOT (r.rolsuper OR r.rolcreaterole OR r.rolcreatedb OR r.rolbypassrls))
       OR NOT EXISTS (SELECT FROM pg_roles r WHERE r.rolname=source.writer_login AND r.rolcanlogin
          AND pg_has_role(r.oid,'nu54_p05_rental_evidence','USAGE')
          AND NOT pg_has_role(r.oid,'nu54_p05_commerce','MEMBER')
          AND NOT pg_has_role(r.oid,'nu54_p05_rental_authority','MEMBER')
          AND NOT pg_has_role(r.oid,'nu54_p05_rental_authority_read','MEMBER')
          AND NOT (r.rolsuper OR r.rolcreaterole OR r.rolcreatedb OR r.rolbypassrls))
    THEN RETURN NULL; END IF;
    SELECT p.* INTO collector FROM ledger.rental_collector_registration p WHERE
        (p.branch_id,p.device_ref,p.generation)=(a.branch_id,a.device_ref,a.generation) FOR SHARE;
    IF NOT FOUND OR collector.collector_certificate IN (audience_,writer_) THEN RETURN NULL; END IF;
    SELECT * INTO authority FROM ledger.rental_collector_authority(tenant,a.branch_id,a.device_ref,a.generation,
        collector.collector_certificate);
    IF NOT FOUND OR authority.deadline IS NULL THEN RETURN NULL; END IF;
    deadline:=least((current_return->>'assignmentUntil')::timestamptz,authority.deadline,audience.valid_until);
    IF deadline<=clock_timestamp() THEN RETURN NULL; END IF;
    RETURN jsonb_build_object('branch',a.branch_id,'board',a.device_ref,'generation',a.generation::text,
        'streamId',source.stream_ref,'streamGeneration',source.stream_generation,
        'challenge','0x'||encode(source.challenge,'hex'),'callerFingerprint',encode(writer_,'hex'),
        'validUntil',floor(extract(epoch FROM deadline))::bigint,
        'scopeDigest',encode(sha256(convert_to(jsonb_build_array('renter-credential-v1',current_return,a.accepted_at,
            audience.authority_source,audience.reader_login,audience.audience_certificate,audience.writer_certificate,
            collector.registration_proof,authority.identity_key,authority.signer,
            extract(epoch FROM authority.deadline),extract(epoch FROM audience.valid_until))::text,'UTF8')),'hex'));
EXCEPTION WHEN insufficient_privilege THEN
    RETURN NULL;
END $$;

REVOKE ALL ON FUNCTION ledger.renter_credential_scope(uuid,uuid,uuid,bytea,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.renter_credential_scope(uuid,uuid,uuid,bytea,bytea) TO nu54_p05_commerce;

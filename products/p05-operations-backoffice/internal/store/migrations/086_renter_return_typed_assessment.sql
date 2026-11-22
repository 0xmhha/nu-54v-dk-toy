CREATE OR REPLACE FUNCTION ledger.renter_return_assessment(tenant uuid,customer uuid,assignment uuid,writer_ bytea)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE a ledger.renter_return_assignment;
DECLARE source ledger.rental_generation_source;
DECLARE audience ledger.rental_authority_audience;
DECLARE collector ledger.rental_collector_registration;
DECLARE proof ledger.rental_generation_authorized_proof;
DECLARE authority record;
DECLARE public_view jsonb;
DECLARE deadline timestamptz;
DECLARE submitted bytea;
BEGIN
    SELECT r.* INTO a FROM ledger.renter_return_assignment r JOIN ledger.branch b ON b.id=r.branch_id
        WHERE r.assignment_id=assignment AND r.customer_ref=customer AND b.tenant_ref=tenant;
    IF NOT FOUND THEN RETURN NULL; END IF;
    PERFORM ledger.lock_rental_evidence_device(a.branch_id,a.device_ref);
    public_view:=ledger.read_renter_return(tenant,customer,assignment);
    IF public_view IS NULL THEN RETURN NULL; END IF;
    SELECT s.* INTO source FROM ledger.rental_generation_source s WHERE
        (s.branch_id,s.device_ref,s.generation)=(a.branch_id,a.device_ref,a.generation) FOR SHARE;
    IF NOT FOUND OR source.account_ref<>a.account_ref OR source.enrollment_digest<>a.enrollment_digest THEN RETURN NULL; END IF;
    SELECT ar.* INTO audience FROM ledger.rental_authority_audience ar WHERE
        (ar.branch_id,ar.device_ref,ar.generation)=(a.branch_id,a.device_ref,a.generation) FOR SHARE;
    IF NOT FOUND OR audience.writer_certificate IS DISTINCT FROM writer_
       OR audience.authority_source IS DISTINCT FROM ledger.rental_generation_source_digest(source)
       OR NOT EXISTS (SELECT FROM pg_roles r WHERE r.rolname=source.writer_login AND r.rolcanlogin
           AND pg_has_role(r.oid,'nu54_p05_rental_evidence','USAGE')
           AND NOT (r.rolsuper OR r.rolcreaterole OR r.rolcreatedb OR r.rolbypassrls)) THEN RETURN NULL; END IF;
    SELECT cr.* INTO collector FROM ledger.rental_collector_registration cr WHERE
        (cr.branch_id,cr.device_ref,cr.generation)=(a.branch_id,a.device_ref,a.generation) FOR SHARE;
    SELECT * INTO authority FROM ledger.rental_collector_authority(tenant,a.branch_id,a.device_ref,a.generation,
        collector.collector_certificate);
    IF NOT FOUND OR authority.deadline IS NULL THEN RETURN NULL; END IF;
    SELECT p.* INTO proof FROM ledger.rental_generation_authorized_proof p WHERE
        (p.branch_id,p.device_ref,p.generation,p.source_digest)=(a.branch_id,a.device_ref,a.generation,
          ledger.rental_generation_source_digest(source))
        ORDER BY p.recorded_at DESC,p.request_key DESC LIMIT 1 FOR SHARE;
    IF NOT FOUND OR proof.challenge<>source.challenge OR proof.enrollment_digest<>a.enrollment_digest
       OR (proof.stream_ref,proof.stream_generation)<>(source.stream_ref,source.stream_generation) THEN RETURN NULL; END IF;
    SELECT q.submitted_digest INTO submitted FROM ledger.rental_proof_submission q WHERE
        (q.branch_id,q.device_ref,q.generation,q.request_key)=(a.branch_id,a.device_ref,a.generation,proof.request_key) FOR SHARE;
    deadline:=least((public_view->>'assignmentUntil')::timestamptz,authority.deadline,audience.valid_until,proof.fresh_until);
    IF NOT FOUND OR deadline<=clock_timestamp() OR proof.finalized_at>clock_timestamp() THEN RETURN NULL; END IF;
    RETURN jsonb_build_object('branch',a.branch_id,'device',a.device_ref,'generation',a.generation,
       'account',source.account_ref,'registration','0x'||encode(source.enrollment_digest,'hex'),
       'measurement','0x'||encode(source.board_measurement,'hex'),'deployment','0x'||encode(source.deployment_digest,'hex'),
       'settlement',source.settlement_ref,'challenge','0x'||encode(source.challenge,'hex'),'stream',source.stream_ref,
       'streamGeneration',source.stream_generation,'assessment','sha256:'||encode(proof.assessment_digest,'hex'),
       'height',proof.finalized_height,'hash','0x'||encode(proof.finalized_hash,'hex'),
       'expiresAt',floor(extract(epoch FROM deadline))::bigint,'asOf',proof.finalized_at,
       'pendingCount',proof.pending_payments+proof.pending_limits,
       'pendingPayments',proof.pending_payments,'pendingLimits',proof.pending_limits,
       'scopeDigest',encode(sha256(convert_to(jsonb_build_array('renter-assessment-v2',public_view,proof.source_digest,proof.request_key,
           submitted,proof.assessment_digest,proof.finalized_hash,proof.pending_payments,proof.pending_limits,proof.finalized_at,
           collector.registration_proof,authority.identity_key,audience.audience_certificate,audience.writer_certificate,
           extract(epoch FROM audience.valid_until),extract(epoch FROM deadline))::text,'UTF8')),'hex'));
EXCEPTION WHEN insufficient_privilege THEN
    RETURN NULL;
END $$;
REVOKE ALL ON FUNCTION ledger.renter_return_assessment(uuid,uuid,uuid,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.renter_return_assessment(uuid,uuid,uuid,bytea) TO nu54_p05_commerce;

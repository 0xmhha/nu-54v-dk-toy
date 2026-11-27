-- Commissioning authority and evidence writer have different capabilities.
-- The collector registration is a policy pin supplied by the qualified installer.
DO $role$
BEGIN
    IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='nu54_p05_rental_authority') THEN
        CREATE ROLE nu54_p05_rental_authority NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
    END IF;
END
$role$;
GRANT USAGE ON SCHEMA ledger TO nu54_p05_rental_authority;

CREATE TABLE ledger.rental_collector_registration (
    branch_id uuid NOT NULL,
    device_ref uuid NOT NULL,
    generation bigint NOT NULL,
    operator_ref uuid NOT NULL REFERENCES ledger.login_subject(subject_ref),
    collector_certificate bytea NOT NULL CHECK(octet_length(collector_certificate)=32),
    collector_peer bytea NOT NULL CHECK(octet_length(collector_peer)=7),
    first_nonce bytea NOT NULL CHECK(octet_length(first_nonce)=32 AND get_byte(first_nonce,31)=0),
    registration_proof bytea UNIQUE CHECK(octet_length(registration_proof)=32),
    authority_source bytea NOT NULL CHECK(octet_length(authority_source)=32),
    valid_until timestamptz NOT NULL CHECK(isfinite(valid_until)),
    PRIMARY KEY (branch_id,device_ref,generation),
    FOREIGN KEY (branch_id,device_ref,generation) REFERENCES ledger.rental_generation_source
);
REVOKE ALL ON ledger.rental_collector_registration
FROM PUBLIC,nu54_p05_commerce,nu54_p05_rental_authority,nu54_p05_rental_evidence;

CREATE FUNCTION ledger.rental_installed_board(branch uuid,device uuid,revision_ text,measurement_ bytea)
RETURNS ledger.firmware_member LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $$
DECLARE installed ledger.firmware_member;
BEGIN
    SELECT * INTO installed FROM ledger.firmware_member f WHERE (f.branch_id,f.device_ref)=(branch,device) FOR SHARE;
    IF NOT FOUND OR installed.withdrawn OR installed.revision<>revision_ OR NOT EXISTS(
        SELECT FROM ledger.firmware_campaign c
        WHERE (c.branch_id,c.device_ref)=(branch,device) AND c.state='verified'
          AND c.version=installed.installed_version AND c.counter=installed.installed_counter
          AND c.measurement=measurement_ AND c.proof_digest IS NOT NULL
    ) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='verified installed board required';
    END IF;
    RETURN installed;
END $$;
REVOKE ALL ON FUNCTION ledger.rental_installed_board(uuid,uuid,text,bytea) FROM PUBLIC;

CREATE FUNCTION ledger.rental_collector_authority(tenant uuid,branch uuid,device uuid,generation_ bigint,certificate_ bytea)
RETURNS TABLE(registration bytea,measurement bytea,deployment bytea,settlement text,account text,
              challenge bytea,deadline timestamptz, identity_key bytea, board bytea,
              signer text, collector bytea, peer bytea, first_nonce bytea, confirmed_digest bytea)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE pin ledger.rental_collector_registration;
DECLARE source ledger.rental_generation_source;
DECLARE installed ledger.firmware_member;
DECLARE enrollment ledger.rental_enrollment;
DECLARE grant_until timestamptz;
BEGIN
    PERFORM ledger.lock_rental_evidence_device(branch,device);
    SELECT * INTO pin FROM ledger.rental_collector_registration p
        WHERE (p.branch_id,p.device_ref,p.generation)=(branch,device,generation_) FOR SHARE;
    IF NOT FOUND OR pin.registration_proof IS NULL OR pin.collector_certificate IS DISTINCT FROM certificate_ OR pin.valid_until<=clock_timestamp() THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='protected collector scope denied';
    END IF;
    grant_until:=ledger.rental_require_live_grant(tenant,branch,pin.operator_ref);
    SELECT * INTO source FROM ledger.rental_generation_source s
        WHERE (s.branch_id,s.device_ref,s.generation)=(branch,device,generation_) FOR SHARE;
    IF NOT FOUND OR pin.authority_source IS DISTINCT FROM ledger.rental_generation_source_digest(source)
        OR least(source.valid_until,source.challenge_until)<=clock_timestamp() THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='protected collector source expired';
    END IF;
    SELECT * INTO enrollment FROM ledger.rental_enrollment e
        WHERE (e.branch_id,e.device_ref,e.generation)=(branch,device,generation_) FOR SHARE;
    IF NOT FOUND OR enrollment.attested_at IS NULL OR enrollment.attested_at>clock_timestamp()
        OR enrollment.attested_until<=clock_timestamp() OR NOT enrollment.generation_evidence_required
        OR (enrollment.enrollment_digest,enrollment.account_ref,enrollment.withdraw_ref,enrollment.revision)
             IS DISTINCT FROM (source.enrollment_digest,source.account_ref,source.withdraw_ref,source.enrollment_revision)
        OR EXISTS(SELECT FROM ledger.rental_enrollment e
                  WHERE (e.branch_id,e.device_ref)=(branch,device) AND e.generation>generation_) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='protected collector enrollment no longer current';
    END IF;
    SELECT * INTO installed FROM ledger.rental_installed_board(branch,device,enrollment.revision,source.board_measurement);
    RETURN QUERY SELECT source.enrollment_digest,source.board_measurement,source.deployment_digest,
        source.settlement_ref,source.account_ref,source.challenge,
        least(source.valid_until,source.challenge_until,enrollment.attested_until,pin.valid_until,grant_until),
        installed.identity_p256,installed.board_id,source.board_signer_ref,pin.collector_certificate,
        pin.collector_peer,pin.first_nonce,pin.registration_proof;
END $$;
REVOKE ALL ON FUNCTION ledger.rental_collector_authority(uuid,uuid,uuid,bigint,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.rental_collector_authority(uuid,uuid,uuid,bigint,bytea) TO nu54_p05_rental_authority;

-- A staged registration only becomes usable after the distinct installed-board
-- key authenticates the complete current scope. The runtime login cannot
-- create or change the independently provisioned board and collector pins.
CREATE FUNCTION ledger.rental_collector_registration_scope(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint)
RETURNS TABLE(registration bytea,measurement bytea,deployment bytea,settlement text,account text,
              challenge bytea,deadline timestamptz, identity_key bytea, board bytea,
              signer text, collector bytea, peer bytea, first_nonce bytea, source_digest bytea)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE pin ledger.rental_collector_registration;
DECLARE source ledger.rental_generation_source;
DECLARE installed ledger.firmware_member;
DECLARE enrollment ledger.rental_enrollment;
DECLARE grant_until timestamptz;
BEGIN
    PERFORM ledger.lock_rental_evidence_device(branch,device);
    grant_until:=ledger.rental_require_live_grant(tenant,branch,actor);
    SELECT * INTO pin FROM ledger.rental_collector_registration p
        WHERE (p.branch_id,p.device_ref,p.generation)=(branch,device,generation_) FOR UPDATE;
    IF NOT FOUND OR pin.operator_ref IS DISTINCT FROM actor OR pin.valid_until<=clock_timestamp() THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='protected registration scope denied';
    END IF;
    SELECT * INTO source FROM ledger.rental_generation_source s
        WHERE (s.branch_id,s.device_ref,s.generation)=(branch,device,generation_) FOR SHARE;
    IF NOT FOUND OR pin.authority_source IS DISTINCT FROM ledger.rental_generation_source_digest(source)
        OR least(source.valid_until,source.challenge_until)<=clock_timestamp() THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='protected registration source expired';
    END IF;
    SELECT * INTO enrollment FROM ledger.rental_enrollment e
        WHERE (e.branch_id,e.device_ref,e.generation)=(branch,device,generation_) FOR SHARE;
    IF NOT FOUND OR enrollment.attested_at IS NULL OR enrollment.attested_at>clock_timestamp()
        OR enrollment.attested_until<=clock_timestamp() OR NOT enrollment.generation_evidence_required
        OR (enrollment.enrollment_digest,enrollment.account_ref,enrollment.withdraw_ref,enrollment.revision)
             IS DISTINCT FROM (source.enrollment_digest,source.account_ref,source.withdraw_ref,source.enrollment_revision)
        OR EXISTS(SELECT FROM ledger.rental_enrollment e
                  WHERE (e.branch_id,e.device_ref)=(branch,device) AND e.generation>generation_) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='protected registration enrollment no longer current';
    END IF;
    SELECT * INTO installed FROM ledger.rental_installed_board(branch,device,enrollment.revision,source.board_measurement);
    RETURN QUERY SELECT source.enrollment_digest,source.board_measurement,source.deployment_digest,
        source.settlement_ref,source.account_ref,source.challenge,
        least(source.valid_until,source.challenge_until,enrollment.attested_until,pin.valid_until,grant_until),
        installed.identity_p256,installed.board_id,source.board_signer_ref,pin.collector_certificate,
        pin.collector_peer,pin.first_nonce,pin.authority_source;
END $$;
REVOKE ALL ON FUNCTION ledger.rental_collector_registration_scope(uuid,uuid,uuid,uuid,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.rental_collector_registration_scope(uuid,uuid,uuid,uuid,bigint) TO nu54_p05_rental_authority;

CREATE FUNCTION ledger.record_rental_collector_registration(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,
    source_ bytea,proof_ bytea)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE verified_source bytea;
BEGIN
    SELECT a.source_digest INTO verified_source FROM ledger.rental_collector_registration_scope(
        tenant,branch,actor,device,generation_) a;
    IF verified_source IS DISTINCT FROM source_ OR proof_ IS NULL OR octet_length(proof_)<>32 THEN
       RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='protected registration proof scope differs';
    END IF;
    UPDATE ledger.rental_collector_registration p SET registration_proof=proof_
        WHERE (p.branch_id,p.device_ref,p.generation)=(branch,device,generation_)
          AND (p.registration_proof IS NULL OR p.registration_proof=proof_);
    IF NOT FOUND THEN
       RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='protected registration already confirmed';
    END IF;
END $$;
REVOKE ALL ON FUNCTION ledger.record_rental_collector_registration(uuid,uuid,uuid,uuid,bigint,bytea,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.record_rental_collector_registration(uuid,uuid,uuid,uuid,bigint,bytea,bytea) TO nu54_p05_rental_authority;

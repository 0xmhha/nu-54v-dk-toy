ALTER TABLE ledger.rental_cleanup_intent
    ADD COLUMN intent_board_signer bytea CHECK(intent_board_signer IS NULL OR
        (signed_intent IS NOT NULL AND octet_length(intent_board_signer)=20)),
    ADD COLUMN signed_receipt bytea CHECK(signed_receipt IS NULL OR
        (signed_intent IS NOT NULL AND octet_length(signed_receipt)=109
         AND substring(signed_receipt FROM 1 FOR 4)=convert_to('RCR1','UTF8')));

CREATE FUNCTION ledger.pin_rental_cleanup_issuance_with_signer(tenant uuid,branch uuid,device uuid,generation_ bigint,key_ uuid,cert_ bytea,
    writer_ bytea,scope_ text,close_ bytea,freeze_ bytea,disposition_ bytea,envelope_ bytea,expiry_ bigint,signer_ bytea)
RETURNS bytea LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE authority record;
DECLARE exact bytea;
BEGIN
    SELECT * INTO authority FROM ledger.rental_cleanup_issue_scope(tenant,branch,device,generation_,key_,cert_);
    IF signer_ IS DISTINCT FROM decode(substr(authority.board,3),'hex') THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='board evidence signer changed';
    END IF;
    exact:=ledger.pin_rental_cleanup_issuance(tenant,branch,device,generation_,key_,cert_,writer_,scope_,close_,freeze_,disposition_,envelope_,expiry_);
    IF authority.signed IS NULL THEN
        UPDATE ledger.rental_cleanup_intent i SET intent_board_signer=signer_
          WHERE (i.branch_id,i.device_ref,i.generation)=(branch,device,generation_);
    END IF;
    RETURN exact;
END $$;

-- Authentication after physical collection uses the original issuance
-- archive and current permissions. It does not ask the retired slot for proof.
CREATE FUNCTION ledger.rental_cleanup_receipt_authority(tenant uuid,branch uuid,device uuid,generation_ bigint,key_ uuid,cert_ bytea)
RETURNS TABLE(envelope bytea,board bytea,disposition bytea,height bigint,anchor bytea,
    enrollment bytea,measurement bytea,deployment bytea,settlement text,account text,deadline timestamptz,scope_digest bytea)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE original ledger.rental_cleanup_intent;
DECLARE station ledger.rental_cleanup_station;
DECLARE pin ledger.rental_collector_registration;
DECLARE source ledger.rental_generation_source;
DECLARE close_candidate ledger.rental_close_candidate;
DECLARE proof ledger.rental_generation_authorized_proof;
DECLARE actor_until timestamptz;
DECLARE registrar_until timestamptz;
BEGIN
    PERFORM ledger.lock_rental_evidence_device(branch,device);
    SELECT * INTO station FROM ledger.rental_cleanup_station s
        WHERE (s.branch_id,s.device_ref,s.generation)=(branch,device,generation_) FOR SHARE;
    SELECT * INTO pin FROM ledger.rental_collector_registration p
        WHERE (p.branch_id,p.device_ref,p.generation)=(branch,device,generation_) FOR SHARE;
    SELECT * INTO original FROM ledger.rental_cleanup_intent i
        WHERE (i.branch_id,i.device_ref,i.generation,i.request_key)=(branch,device,generation_,key_) FOR SHARE;
    IF NOT FOUND OR station.certificate IS DISTINCT FROM cert_ OR pin.collector_certificate IS DISTINCT FROM cert_
        OR greatest(station.valid_until,pin.valid_until) IS NULL
        OR least(station.valid_until,pin.valid_until)<=clock_timestamp() OR pin.registration_proof IS NULL THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='cleanup receipt scope denied';
    END IF;
    actor_until:=ledger.rental_require_live_grant(tenant,branch,original.actor_id);
    registrar_until:=ledger.rental_require_live_grant(tenant,branch,pin.operator_ref);
    SELECT * INTO close_candidate FROM ledger.rental_close_candidate c
        WHERE (c.branch_id,c.device_ref,c.generation)=(branch,device,generation_) FOR SHARE;
    SELECT * INTO proof FROM ledger.rental_generation_authorized_proof p
        WHERE (p.branch_id,p.device_ref,p.generation,p.request_key)=(branch,device,generation_,close_candidate.proof_key) FOR SHARE;
    SELECT * INTO source FROM ledger.rental_generation_source s
        WHERE (s.branch_id,s.device_ref,s.generation)=(branch,device,generation_) FOR SHARE;
    IF NOT FOUND OR original.signed_intent IS NULL OR original.anchor_hash IS NULL OR original.anchor_height IS NULL
       OR close_candidate.state IS DISTINCT FROM 'confirmed' OR proof.pending_payments IS DISTINCT FROM 0
       OR proof.pending_limits IS DISTINCT FROM 0 OR proof.enrollment_digest IS DISTINCT FROM source.enrollment_digest
       OR (original.intent_board_signer IS NULL AND proof.source_digest IS DISTINCT FROM ledger.rental_generation_source_digest(source))
       OR least(station.valid_until,pin.valid_until,actor_until,registrar_until)<=clock_timestamp() THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='original cleanup receipt authority unavailable';
    END IF;
    RETURN QUERY SELECT original.signed_intent,coalesce(original.intent_board_signer,decode(substr(source.board_signer_ref,3),'hex')),
        original.disposition,original.anchor_height,original.anchor_hash,source.enrollment_digest,source.board_measurement,
        source.deployment_digest,source.settlement_ref,source.account_ref,least(station.valid_until,pin.valid_until,actor_until,registrar_until),
        sha256(convert_to(jsonb_build_array(source.enrollment_digest,source.board_measurement,source.deployment_digest,
            source.settlement_ref,source.account_ref,original.disposition,original.anchor_height,original.anchor_hash)::text,'UTF8'));
END $$;

CREATE FUNCTION ledger.record_rental_cleanup_receipt(tenant uuid,branch uuid,device uuid,generation_ bigint,key_ uuid,cert_ bytea,
    envelope_ bytea,signer_ bytea,scope_ bytea,receipt_ bytea,lease_ bigint)
RETURNS TABLE(recorded bytea, current_until timestamptz) LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE authority record;
DECLARE prior bytea;
BEGIN
    SELECT * INTO authority FROM ledger.rental_cleanup_receipt_authority(tenant,branch,device,generation_,key_,cert_);
    IF authority.envelope IS DISTINCT FROM envelope_ OR authority.board IS DISTINCT FROM signer_ OR authority.scope_digest IS DISTINCT FROM scope_ OR
       lease_ IS NULL OR lease_<=extract(epoch FROM clock_timestamp()) THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='cleanup receipt authority changed';
    END IF;
    SELECT i.signed_receipt INTO prior FROM ledger.rental_cleanup_intent i
      WHERE (i.branch_id,i.device_ref,i.generation)=(branch,device,generation_);
    IF receipt_ IS NOT NULL THEN
        IF prior IS NULL THEN
            UPDATE ledger.rental_cleanup_intent i SET signed_receipt=receipt_
              WHERE (i.branch_id,i.device_ref,i.generation)=(branch,device,generation_);
            prior:=receipt_;
        ELSIF prior IS DISTINCT FROM receipt_ THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='cleanup receipt already pinned';
        END IF;
    END IF;
    RETURN QUERY SELECT prior,least(authority.deadline,to_timestamp(lease_));
END $$;

CREATE OR REPLACE FUNCTION ledger.rental_cleanup_pending(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE pending jsonb;
BEGIN
    PERFORM ledger.rental_require_live_grant(tenant,branch,actor);
    SELECT jsonb_build_object('state',CASE WHEN c.signed_receipt IS NULL THEN 'board-receipt-required'
        ELSE 'receipt-authenticated-handoff-required' END)
        || CASE WHEN c.actor_id=actor THEN jsonb_build_object('requestKey',c.request_key) ELSE '{}'::jsonb END
      INTO pending FROM ledger.rental_cleanup_intent c
      WHERE (c.branch_id,c.device_ref,c.generation)=(branch,device,generation_);
    RETURN coalesce(pending,'{"state":"settlement-proof-required"}'::jsonb);
END $$;

REVOKE ALL ON FUNCTION ledger.pin_rental_cleanup_issuance_with_signer(uuid,uuid,uuid,bigint,uuid,bytea,bytea,text,bytea,bytea,bytea,bytea,bigint,bytea),
    ledger.rental_cleanup_receipt_authority(uuid,uuid,uuid,bigint,uuid,bytea),
    ledger.record_rental_cleanup_receipt(uuid,uuid,uuid,bigint,uuid,bytea,bytea,bytea,bytea,bytea,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.pin_rental_cleanup_issuance_with_signer(uuid,uuid,uuid,bigint,uuid,bytea,bytea,text,bytea,bytea,bytea,bytea,bigint,bytea),
    ledger.rental_cleanup_receipt_authority(uuid,uuid,uuid,bigint,uuid,bytea),
    ledger.record_rental_cleanup_receipt(uuid,uuid,uuid,bigint,uuid,bytea,bytea,bytea,bytea,bytea,bigint) TO nu54_p05_rental_close;

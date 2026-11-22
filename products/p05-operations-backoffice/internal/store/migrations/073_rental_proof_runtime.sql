-- Retain an authenticated submission digest for retry without retaining the
-- signed receipt or its public journal. 070 remains the proof authority.
CREATE TABLE ledger.rental_proof_submission (
    branch_id uuid NOT NULL,
    device_ref uuid NOT NULL,
    generation bigint NOT NULL,
    request_key uuid NOT NULL,
    submitted_digest bytea NOT NULL CHECK(octet_length(submitted_digest)=32),
    PRIMARY KEY (branch_id,device_ref,generation,request_key),
    FOREIGN KEY (branch_id,device_ref,generation,request_key)
      REFERENCES ledger.rental_generation_authorized_proof ON DELETE CASCADE
);
REVOKE ALL ON ledger.rental_proof_submission FROM PUBLIC,nu54_p05_commerce,nu54_p05_rental_evidence;

CREATE FUNCTION ledger.rental_writer_scope(tenant uuid,branch uuid,device uuid,generation_ bigint,
    certificate_ bytea,request_ uuid,submitted_ bytea)
RETURNS TABLE(registration bytea,measurement bytea,deployment bytea,settlement text,account text,
    challenge bytea,deadline timestamptz,identity_key bytea,board bytea,signer text,collector bytea,peer bytea,
    first_nonce bytea,confirmed_digest bytea,stream_ref uuid,stream_generation uuid,
    previous_height bigint,previous_hash bytea,replayed boolean)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE authority record;
DECLARE source ledger.rental_generation_source;
DECLARE previous ledger.rental_generation_authorized_proof;
DECLARE existing bytea;
BEGIN
    SELECT * INTO authority FROM ledger.rental_collector_authority(tenant,branch,device,generation_,certificate_);
    SELECT * INTO source FROM ledger.rental_generation_source s
        WHERE (s.branch_id,s.device_ref,s.generation)=(branch,device,generation_) FOR SHARE;
    IF NOT FOUND OR source.writer_login<>session_user THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='dedicated rental writer required';
    END IF;
    SELECT p.submitted_digest INTO existing FROM ledger.rental_proof_submission p
      WHERE (p.branch_id,p.device_ref,p.generation,p.request_key)=(branch,device,generation_,request_);
    IF FOUND AND existing IS DISTINCT FROM submitted_ THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='rental collector request binding changed';
    END IF;
    SELECT * INTO previous FROM ledger.rental_generation_authorized_proof p
      WHERE (p.branch_id,p.device_ref,p.generation,p.stream_ref,p.stream_generation)=
        (branch,device,generation_,source.stream_ref,source.stream_generation)
      ORDER BY p.finalized_height DESC,p.recorded_at DESC,p.request_key DESC LIMIT 1;
    RETURN QUERY SELECT authority.registration,authority.measurement,authority.deployment,authority.settlement,
        authority.account,authority.challenge,authority.deadline,authority.identity_key,authority.board,
        authority.signer,authority.collector,authority.peer,authority.first_nonce,authority.confirmed_digest,
        source.stream_ref,source.stream_generation,previous.finalized_height,previous.finalized_hash,existing IS NOT NULL;
END $$;
REVOKE ALL ON FUNCTION ledger.rental_writer_scope(uuid,uuid,uuid,bigint,bytea,uuid,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.rental_writer_scope(uuid,uuid,uuid,bigint,bytea,uuid,bytea) TO nu54_p05_rental_evidence;

CREATE FUNCTION ledger.record_rental_proof_submission(branch uuid,device uuid,generation_ bigint,request_ uuid,submitted_ bytea)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF NOT EXISTS (SELECT FROM ledger.rental_generation_source s
      WHERE (s.branch_id,s.device_ref,s.generation)=(branch,device,generation_) AND s.writer_login=session_user) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='dedicated rental writer required';
    END IF;
    INSERT INTO ledger.rental_proof_submission VALUES(branch,device,generation_,request_,submitted_);
END $$;
REVOKE ALL ON FUNCTION ledger.record_rental_proof_submission(uuid,uuid,bigint,uuid,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.record_rental_proof_submission(uuid,uuid,bigint,uuid,bytea) TO nu54_p05_rental_evidence;

-- The runtime evidence path is prepared independently of live P07 close
-- admission. Once used, it cannot fall back to a cached zero on restart.
CREATE FUNCTION ledger.rental_runtime_close_hold(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.rental_require_live_grant(tenant,branch,actor);
    RETURN EXISTS (SELECT FROM ledger.rental_proof_submission p
       WHERE (p.branch_id,p.device_ref,p.generation)=(branch,device,generation_));
END $$;
REVOKE ALL ON FUNCTION ledger.rental_runtime_close_hold(uuid,uuid,uuid,uuid,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.rental_runtime_close_hold(uuid,uuid,uuid,uuid,bigint) TO nu54_p05_commerce;

ALTER FUNCTION ledger.begin_rental_action(uuid,uuid,uuid,uuid,bigint,text,uuid,numeric,text)
    RENAME TO begin_rental_action_with_cached_proof;
REVOKE EXECUTE ON FUNCTION ledger.begin_rental_action_with_cached_proof(uuid,uuid,uuid,uuid,bigint,text,uuid,numeric,text) FROM nu54_p05_commerce;

CREATE FUNCTION ledger.begin_rental_action(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,
    action_ text,key_ uuid,amount_ numeric,review_ text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF action_='close' THEN
        PERFORM ledger.lock_rental_evidence_device(branch,device);
        IF ledger.rental_runtime_close_hold(tenant,branch,actor,device,generation_)
           AND NOT EXISTS (SELECT FROM ledger.rental_operation o
             WHERE (o.branch_id,o.device_ref,o.generation,o.action)=(branch,device,generation_,'close')) THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='live rental chain authority required';
        END IF;
    END IF;
    RETURN ledger.begin_rental_action_with_cached_proof(tenant,branch,actor,device,generation_,action_,key_,amount_,review_);
END $$;
REVOKE ALL ON FUNCTION ledger.begin_rental_action(uuid,uuid,uuid,uuid,bigint,text,uuid,numeric,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.begin_rental_action(uuid,uuid,uuid,uuid,bigint,text,uuid,numeric,text) TO nu54_p05_commerce;

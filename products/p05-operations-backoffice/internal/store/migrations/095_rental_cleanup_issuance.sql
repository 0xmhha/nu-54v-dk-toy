ALTER TABLE ledger.rental_cleanup_intent
    ADD COLUMN anchor_height bigint CHECK(anchor_height>0),
    ADD COLUMN anchor_hash bytea CHECK(octet_length(anchor_hash)=32),
    ADD COLUMN signed_intent bytea CHECK(octet_length(signed_intent)=181),
    ADD CONSTRAINT cleanup_anchor_pair CHECK((anchor_height IS NULL)=(anchor_hash IS NULL));

-- Provisioned independently of the runtime roles; a freeze collector alone has
-- no authority to receive a signed retirement intent.
CREATE TABLE ledger.rental_cleanup_station (
    branch_id uuid NOT NULL,
    device_ref uuid NOT NULL,
    generation bigint NOT NULL,
    certificate bytea NOT NULL CHECK(octet_length(certificate)=32),
    valid_until timestamptz NOT NULL,
    PRIMARY KEY(branch_id,device_ref,generation),
    FOREIGN KEY(branch_id,device_ref,generation) REFERENCES ledger.rental_collector_registration
);
REVOKE ALL ON ledger.rental_cleanup_station FROM PUBLIC,nu54_p05_commerce,nu54_p05_rental_close,nu54_p05_rental_authority;

CREATE FUNCTION ledger.admit_anchored_rental_cleanup(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,
    key_ uuid,writer_ bytea,scope_ text,close_ bytea,freeze_ bytea,disposition_ bytea,lease_ bigint,height_ bigint,anchor_ bytea)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF height_ IS NULL OR height_<=0 OR anchor_ IS NULL OR octet_length(anchor_)<>32 OR anchor_=decode(repeat('00',32),'hex') THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='cleanup anchor required';
    END IF;
    PERFORM ledger.lock_rental_evidence_device(branch,device);
    IF EXISTS(SELECT FROM ledger.rental_cleanup_intent c WHERE (c.branch_id,c.device_ref,c.generation)=(branch,device,generation_)) THEN
        PERFORM ledger.admit_rental_cleanup(tenant,branch,actor,device,generation_,key_,writer_,scope_,close_,freeze_,disposition_,lease_);
        RETURN;
    END IF;
    PERFORM ledger.admit_rental_cleanup(tenant,branch,actor,device,generation_,key_,writer_,scope_,close_,freeze_,disposition_,lease_);
    UPDATE ledger.rental_cleanup_intent c SET anchor_height=height_,anchor_hash=anchor_
      WHERE (c.branch_id,c.device_ref,c.generation)=(branch,device,generation_);
END $$;

CREATE FUNCTION ledger.rental_cleanup_issue_scope(tenant uuid,branch uuid,device uuid,generation_ bigint,key_ uuid,cert_ bytea)
RETURNS TABLE(actor uuid,disposition bytea,height bigint,anchor bytea,signed bytea,board text,deadline timestamptz)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE request ledger.rental_cleanup_intent;
DECLARE station ledger.rental_cleanup_station;
DECLARE registration record;
DECLARE grant_until timestamptz;
BEGIN
    PERFORM ledger.lock_rental_evidence_device(branch,device);
    SELECT * INTO station FROM ledger.rental_cleanup_station s WHERE (s.branch_id,s.device_ref,s.generation)=(branch,device,generation_) FOR SHARE;
    IF NOT FOUND OR station.certificate IS DISTINCT FROM cert_ OR station.valid_until<=clock_timestamp() THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='cleanup station scope denied';
    END IF;
    SELECT * INTO request FROM ledger.rental_cleanup_intent c WHERE (c.branch_id,c.device_ref,c.generation,c.request_key)=(branch,device,generation_,key_) FOR SHARE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='cleanup request denied';
    END IF;
    grant_until:=ledger.rental_require_live_grant(tenant,branch,request.actor_id);
    SELECT * INTO registration FROM ledger.rental_collector_authority(tenant,branch,device,generation_,cert_);
    IF request.anchor_height IS NULL OR request.anchor_hash IS NULL THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='original cleanup anchor unavailable';
    END IF;
    RETURN QUERY SELECT request.actor_id,request.disposition,request.anchor_height,request.anchor_hash,
      request.signed_intent,registration.signer,least(station.valid_until,registration.deadline,grant_until);
END $$;

CREATE FUNCTION ledger.pin_rental_cleanup_issuance(tenant uuid,branch uuid,device uuid,generation_ bigint,key_ uuid,cert_ bytea,
    writer_ bytea,scope_ text,close_ bytea,freeze_ bytea,disposition_ bytea,envelope_ bytea,expiry_ bigint)
RETURNS bytea LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE current_scope record;
BEGIN
    SELECT * INTO current_scope FROM ledger.rental_cleanup_issue_scope(tenant,branch,device,generation_,key_,cert_);
    IF current_scope.disposition IS DISTINCT FROM disposition_ OR
       ledger.rental_cleanup_freeze(tenant,branch,current_scope.actor,device,generation_,writer_,scope_,close_) IS DISTINCT FROM freeze_ OR
       expiry_ IS NULL OR expiry_<=extract(epoch FROM clock_timestamp()) OR expiry_>extract(epoch FROM current_scope.deadline) THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='cleanup issuance authority changed';
    END IF;
    IF current_scope.signed IS NOT NULL THEN
        RETURN current_scope.signed;
    END IF;
    IF envelope_ IS NULL OR octet_length(envelope_)<>181 OR substring(envelope_ FROM 1 FOR 4)<>convert_to('RCI1','UTF8') OR
       substring(envelope_ FROM 37 FOR 32)<>disposition_ THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='invalid cleanup envelope';
    END IF;
    UPDATE ledger.rental_cleanup_intent c SET signed_intent=envelope_
      WHERE (c.branch_id,c.device_ref,c.generation)=(branch,device,generation_);
    RETURN envelope_;
END $$;

REVOKE ALL ON FUNCTION ledger.admit_anchored_rental_cleanup(uuid,uuid,uuid,uuid,bigint,uuid,bytea,text,bytea,bytea,bytea,bigint,bigint,bytea),
    ledger.rental_cleanup_issue_scope(uuid,uuid,uuid,bigint,uuid,bytea),
    ledger.pin_rental_cleanup_issuance(uuid,uuid,uuid,bigint,uuid,bytea,bytea,text,bytea,bytea,bytea,bytea,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.admit_anchored_rental_cleanup(uuid,uuid,uuid,uuid,bigint,uuid,bytea,text,bytea,bytea,bytea,bigint,bigint,bytea),
    ledger.rental_cleanup_issue_scope(uuid,uuid,uuid,bigint,uuid,bytea),
    ledger.pin_rental_cleanup_issuance(uuid,uuid,uuid,bigint,uuid,bytea,bytea,text,bytea,bytea,bytea,bytea,bigint) TO nu54_p05_rental_close;

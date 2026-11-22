-- A current handover can be explicitly issued for an independently verified
-- v2 payment deployment. Existing handovers keep their original scope.
ALTER TABLE ledger.renter_return_assignment ADD COLUMN payment_deployment bytea
    CHECK (payment_deployment IS NULL OR octet_length(payment_deployment)=32);

CREATE FUNCTION ledger.offer_renter_handover(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,
    customer uuid,account_ text,expires_ timestamptz,payment_deployment_ bytea)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE enrolled ledger.rental_enrollment;
DECLARE id uuid;
BEGIN
    IF payment_deployment_ IS NOT NULL AND octet_length(payment_deployment_)<>32 THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid payment deployment';
    END IF;
    PERFORM ledger.lock_rental_evidence_device(branch,device);
    PERFORM ledger.rental_require_live_grant(tenant,branch,actor);
    SELECT * INTO enrolled FROM ledger.rental_enrollment e
        WHERE (e.branch_id,e.device_ref,e.generation)=(branch,device,generation_) FOR SHARE;
    IF NOT FOUND OR enrolled.account_ref IS DISTINCT FROM account_ OR enrolled.attested_at IS NULL
       OR enrolled.attested_at>clock_timestamp() OR expires_<=clock_timestamp() OR expires_>enrolled.attested_until
       OR customer IS NULL OR customer=actor OR EXISTS (
          SELECT FROM ledger.rental_enrollment e WHERE (e.branch_id,e.device_ref)=(branch,device) AND e.generation>generation_) THEN
        RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='current rental handover required';
    END IF;
    UPDATE ledger.renter_return_assignment a SET revoked_at=clock_timestamp()
      WHERE (a.branch_id,a.device_ref,a.generation)=(branch,device,generation_)
        AND a.revoked_at IS NULL AND a.expires_at<=clock_timestamp();
    SELECT a.assignment_id INTO id FROM ledger.renter_return_assignment a
      WHERE (a.branch_id,a.device_ref,a.generation,a.customer_ref,a.offered_by,a.account_ref,a.enrollment_digest,a.expires_at)
          =(branch,device,generation_,customer,actor,account_,enrolled.enrollment_digest,expires_)
        AND a.revoked_at IS NULL AND a.payment_deployment IS NOT DISTINCT FROM payment_deployment_;
    IF FOUND THEN RETURN id; END IF;
    IF EXISTS(SELECT FROM ledger.renter_return_assignment a
        WHERE (a.branch_id,a.device_ref,a.generation)=(branch,device,generation_) AND a.revoked_at IS NULL) THEN
        RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='current rental handover must be revoked before replacement';
    END IF;
    INSERT INTO ledger.renter_return_assignment(branch_id,device_ref,generation,customer_ref,offered_by,account_ref,
        enrollment_digest,expires_at,payment_deployment)
        VALUES(branch,device,generation_,customer,actor,account_,enrolled.enrollment_digest,expires_,payment_deployment_)
        RETURNING assignment_id INTO id;
    RETURN id;
END $$;

REVOKE ALL ON FUNCTION ledger.offer_renter_handover(uuid,uuid,uuid,uuid,bigint,uuid,text,timestamptz,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.offer_renter_handover(uuid,uuid,uuid,uuid,bigint,uuid,text,timestamptz,bytea) TO nu54_p05_commerce;

CREATE OR REPLACE FUNCTION ledger.offer_renter_return(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,
    customer uuid,account_ text,expires_ timestamptz)
RETURNS uuid LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT ledger.offer_renter_handover(tenant,branch,actor,device,generation_,customer,account_,expires_,NULL)
$$;

-- Match the existing handover to an exact buyer quote. Once a generation
-- has been offered on v2, a withdrawn or replaced assignment cannot act
-- like an unrelated legacy enrollment for new forwarding decisions.
CREATE FUNCTION ledger.renter_payment_scope(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid,quote_ bytea,
    deployment_ bytea,lock_ boolean)
RETURNS TABLE(requires_v2 boolean, expires_at timestamptz)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE enrolled ledger.rental_enrollment;
DECLARE original ledger.renter_return_assignment;
DECLARE handover jsonb;
BEGIN
    requires_v2 := false;
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code IS DISTINCT FROM 'customer'
       OR NOT ledger.is_cafe_checkout(branch_,sale_) THEN
        RETURN NEXT;
        RETURN;
    END IF;
    SELECT e.* INTO enrolled
      FROM ledger.quote_work q JOIN ledger.rental_enrollment e
        ON e.account_ref='0x'||encode(q.payer,'hex') AND e.generation=q.payer_generation
      WHERE e.branch_id=branch_ AND q.sale_id=sale_ AND q.actor_id=actor_
        AND q.signed_digest=quote_ AND q.payer_role=2;
    IF NOT FOUND THEN RETURN NEXT; RETURN; END IF;
    IF lock_ THEN
        PERFORM ledger.lock_rental_evidence_device(enrolled.branch_id,enrolled.device_ref);
    END IF;
    SELECT EXISTS(SELECT FROM ledger.renter_return_assignment a
      WHERE (a.branch_id,a.device_ref,a.generation)=(enrolled.branch_id,enrolled.device_ref,enrolled.generation)
        AND a.payment_deployment IS NOT NULL) INTO requires_v2;
    IF requires_v2 AND deployment_ IS NOT NULL THEN
        SELECT a.* INTO original FROM ledger.renter_return_assignment a
          WHERE (a.branch_id,a.device_ref,a.generation)=(enrolled.branch_id,enrolled.device_ref,enrolled.generation)
            AND a.customer_ref=actor_ AND a.payment_deployment=deployment_
            AND a.revoked_at IS NULL AND a.accepted_at IS NOT NULL AND a.expires_at>clock_timestamp()
            AND EXISTS(SELECT FROM ledger.payer_enrollment pe
                WHERE pe.branch_id=branch_ AND pe.actor_id=actor_ AND pe.payer_role=2
                  AND '0x'||encode(pe.payer,'hex')=a.account_ref AND pe.payer_generation=a.generation
                  AND pe.expires_at>clock_timestamp());
        IF FOUND THEN
            handover := ledger.read_renter_return(tenant_,actor_,original.assignment_id);
            expires_at := (handover->>'assignmentUntil')::timestamptz;
        END IF;
    END IF;
    RETURN NEXT;
END
$$;
REVOKE ALL ON FUNCTION ledger.renter_payment_scope(uuid,uuid,uuid,uuid,bytea,bytea,boolean) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.renter_payment_scope(uuid,uuid,uuid,uuid,bytea,bytea,boolean) TO nu54_p05_commerce;

ALTER FUNCTION ledger.store_customer_forward(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea,bytea,timestamptz)
    RENAME TO store_customer_forward_unbound;
REVOKE ALL ON FUNCTION ledger.store_customer_forward_unbound(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea,bytea,timestamptz)
    FROM PUBLIC,nu54_p05_commerce;

CREATE FUNCTION ledger.store_customer_escrow_forward(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid,
    key_ uuid,intent_ bytea,quote_ bytea,approval_ bytea,stream_ uuid,encrypted_ bytea,
    current_grant_ bytea,sponsor_until_ timestamptz,deployment_ bytea,escrow_until_ timestamptz)
RETURNS TABLE(sale_ref uuid,new_sponsored_admission boolean)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE rental record;
BEGIN
    PERFORM FROM ledger.sale s WHERE s.id=sale_ AND s.branch_id=branch_ FOR UPDATE;
    IF NOT EXISTS(SELECT FROM ledger.settlement_submission p WHERE p.sale_id=sale_) THEN
        SELECT * INTO rental FROM ledger.renter_payment_scope(tenant_,branch_,actor_,sale_,quote_,deployment_,true);
        IF rental.requires_v2 AND
           (rental.expires_at IS NULL OR rental.expires_at<=clock_timestamp() OR
            escrow_until_ IS NULL OR escrow_until_<=clock_timestamp()
            OR NOT EXISTS(SELECT FROM ledger.quote_work q WHERE q.sale_id=sale_
                AND q.gas_sponsor_digest=current_grant_)) THEN
            RAISE EXCEPTION 'current rental escrow unavailable' USING ERRCODE='P1050';
        END IF;
    END IF;
    RETURN QUERY SELECT * FROM ledger.store_customer_forward_unbound(tenant_,branch_,actor_,sale_,key_,intent_,
        quote_,approval_,stream_,encrypted_,current_grant_,sponsor_until_);
END
$$;
REVOKE ALL ON FUNCTION ledger.store_customer_escrow_forward(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea,bytea,timestamptz,bytea,timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.store_customer_escrow_forward(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea,bytea,timestamptz,bytea,timestamptz)
    TO nu54_p05_commerce;

CREATE FUNCTION ledger.store_customer_forward(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid,
    key_ uuid,intent_ bytea,quote_ bytea,approval_ bytea,stream_ uuid,encrypted_ bytea,current_grant_ bytea,sponsor_until_ timestamptz)
RETURNS TABLE(sale_ref uuid,new_sponsored_admission boolean)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT * FROM ledger.store_customer_escrow_forward(tenant_,branch_,actor_,sale_,key_,intent_,quote_,approval_,
        stream_,encrypted_,current_grant_,sponsor_until_,NULL,NULL)
$$;
REVOKE ALL ON FUNCTION ledger.store_customer_forward(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea,bytea,timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.store_customer_forward(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea,bytea,timestamptz)
    TO nu54_p05_commerce;


-- A terminal using its earlier ABI can recover accepted results. The
-- customer-owned v2 rental handover requires the verified C3 admission path
-- for a new submission.
ALTER FUNCTION ledger.store_terminal_forward(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea,boolean,bytea,timestamptz)
    RENAME TO store_terminal_forward_unbound;
REVOKE ALL ON FUNCTION ledger.store_terminal_forward_unbound(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea,boolean,bytea,timestamptz)
    FROM PUBLIC,nu54_p05_commerce;

CREATE FUNCTION ledger.store_terminal_forward(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid,
    key_ uuid,intent_ bytea,quote_ bytea,approval_ bytea,stream_ uuid,encrypted_ bytea,
    sponsorship_required_ boolean,current_grant_ bytea,sponsorship_until_ timestamptz)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE payer uuid;
DECLARE restricted boolean;
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code='terminal' AND
       EXISTS(SELECT FROM ledger.forward_quote(tenant_,branch_,actor_,sale_)) THEN
        PERFORM FROM ledger.sale s WHERE s.id=sale_ AND s.branch_id=branch_ FOR UPDATE;
        IF NOT EXISTS(SELECT FROM ledger.settlement_submission p WHERE p.sale_id=sale_) THEN
            SELECT q.actor_id INTO payer FROM ledger.quote_work q WHERE q.sale_id=sale_ AND q.payer_role=2;
            IF FOUND THEN
                SELECT scope.requires_v2 INTO restricted
                FROM ledger.renter_payment_scope(tenant_,branch_,payer,sale_,quote_,NULL,true) scope;
                IF restricted THEN
                    RAISE EXCEPTION 'rental escrow requires customer forwarding' USING ERRCODE='P1050';
                END IF;
            END IF;
        END IF;
    END IF;
    RETURN ledger.store_terminal_forward_unbound(tenant_,branch_,actor_,sale_,key_,intent_,quote_,approval_,
        stream_,encrypted_,sponsorship_required_,current_grant_,sponsorship_until_);
END
$$;
REVOKE ALL ON FUNCTION ledger.store_terminal_forward(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea,boolean,bytea,timestamptz)
    FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.store_terminal_forward(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea,boolean,bytea,timestamptz)
    TO nu54_p05_commerce;

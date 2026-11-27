-- The exact checkout payer enrollment can expire before the handover or chain
-- evidence. Return the current authority limit without changing 102's ABI.
CREATE OR REPLACE FUNCTION ledger.renter_payment_scope(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid,quote_ bytea,
    deployment_ bytea,lock_ boolean)
RETURNS TABLE(requires_v2 boolean, expires_at timestamptz)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE enrolled ledger.rental_enrollment;
DECLARE original ledger.renter_return_assignment;
DECLARE handover jsonb;
DECLARE payer_until timestamptz;
DECLARE manager_until timestamptz;
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
            AND a.revoked_at IS NULL AND a.accepted_at IS NOT NULL AND a.expires_at>clock_timestamp();
        IF FOUND THEN
            -- Serialize enrollment changes with proof and admission reads.
            SELECT pe.expires_at INTO payer_until
              FROM ledger.payer_enrollment pe
              WHERE pe.branch_id=branch_ AND pe.actor_id=actor_ AND pe.payer_role=2
                AND '0x'||encode(pe.payer,'hex')=original.account_ref AND pe.payer_generation=original.generation
              FOR SHARE;
            IF payer_until>clock_timestamp() THEN
                handover := ledger.read_renter_return(tenant_,actor_,original.assignment_id);
                IF handover IS NOT NULL THEN
                    IF lock_ THEN
                        -- The manager's grant can be withdrawn independently from the handover.
                        SELECT g.valid_until INTO manager_until FROM ledger.finance_grant g
                          WHERE (g.branch_id,g.actor_id,g.capability)=(branch_,original.offered_by,'rental_manage')
                          FOR SHARE;
                    ELSE
                        manager_until := (handover->>'assignmentUntil')::timestamptz;
                    END IF;
                    IF manager_until>clock_timestamp() THEN
                        expires_at := least(payer_until,manager_until,(handover->>'assignmentUntil')::timestamptz);
                        IF expires_at<=clock_timestamp() THEN expires_at := NULL; END IF;
                    END IF;
                END IF;
            END IF;
        END IF;
    END IF;
    RETURN NEXT;
END
$$;

CREATE FUNCTION ledger.store_customer_escrow_forward_bounded(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid,
    key_ uuid,intent_ bytea,quote_ bytea,approval_ bytea,stream_ uuid,encrypted_ bytea,
    current_grant_ bytea,sponsor_until_ timestamptz,deployment_ bytea,escrow_until_ timestamptz)
RETURNS TABLE(sale_ref uuid,new_sponsored_admission boolean,admission_until timestamptz)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE rental record;
DECLARE deadline timestamptz;
DECLARE quote_until timestamptz;
BEGIN
    PERFORM FROM ledger.sale s WHERE s.id=sale_ AND s.branch_id=branch_ FOR UPDATE;
    IF NOT EXISTS(SELECT FROM ledger.settlement_submission p WHERE p.sale_id=sale_) THEN
        SELECT * INTO rental FROM ledger.renter_payment_scope(tenant_,branch_,actor_,sale_,quote_,deployment_,true);
        IF rental.requires_v2 THEN
            SELECT date_trunc('second',q.expires_at) INTO quote_until FROM ledger.quote_work q WHERE q.sale_id=sale_
              AND q.gas_sponsor_digest=current_grant_;
            IF rental.expires_at IS NULL OR escrow_until_ IS NULL OR quote_until IS NULL OR sponsor_until_ IS NULL THEN
                RAISE EXCEPTION 'current rental escrow unavailable' USING ERRCODE='P1050';
            END IF;
            deadline := least(rental.expires_at,escrow_until_,quote_until,sponsor_until_);
            IF deadline<=clock_timestamp() THEN
                RAISE EXCEPTION 'current rental escrow unavailable' USING ERRCODE='P1050';
            END IF;
        END IF;
    END IF;
    SELECT result.sale_ref,result.new_sponsored_admission INTO STRICT sale_ref,new_sponsored_admission
        FROM ledger.store_customer_forward_unbound(tenant_,branch_,actor_,sale_,key_,intent_,quote_,approval_,
            stream_,encrypted_,current_grant_,sponsor_until_) result;
    IF new_sponsored_admission AND deadline IS NOT NULL THEN
        IF deadline<=clock_timestamp() THEN
            RAISE EXCEPTION 'current rental escrow unavailable' USING ERRCODE='P1050';
        END IF;
        admission_until := deadline;
    END IF;
    RETURN NEXT;
END
$$;
REVOKE ALL ON FUNCTION ledger.store_customer_escrow_forward_bounded(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea,bytea,timestamptz,bytea,timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.store_customer_escrow_forward_bounded(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea,bytea,timestamptz,bytea,timestamptz)
    TO nu54_p05_commerce;

CREATE OR REPLACE FUNCTION ledger.store_customer_escrow_forward(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid,
    key_ uuid,intent_ bytea,quote_ bytea,approval_ bytea,stream_ uuid,encrypted_ bytea,current_grant_ bytea,
    sponsor_until_ timestamptz,deployment_ bytea,escrow_until_ timestamptz)
RETURNS TABLE(sale_ref uuid,new_sponsored_admission boolean)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT result.sale_ref,result.new_sponsored_admission
    FROM ledger.store_customer_escrow_forward_bounded(tenant_,branch_,actor_,sale_,key_,intent_,quote_,approval_,
        stream_,encrypted_,current_grant_,sponsor_until_,deployment_,escrow_until_) result
$$;

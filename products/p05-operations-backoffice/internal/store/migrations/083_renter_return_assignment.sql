CREATE TABLE ledger.renter_return_assignment (
    assignment_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id uuid NOT NULL,
    device_ref uuid NOT NULL,
    generation bigint NOT NULL,
    customer_ref uuid NOT NULL,
    offered_by uuid NOT NULL REFERENCES ledger.login_subject(subject_ref),
    account_ref text NOT NULL CHECK(account_ref~'^0x[0-9a-f]{40}$'),
    enrollment_digest bytea NOT NULL CHECK(octet_length(enrollment_digest)=32),
    expires_at timestamptz NOT NULL CHECK(isfinite(expires_at)),
    accepted_at timestamptz,
    revoked_at timestamptz,
    FOREIGN KEY(branch_id,device_ref,generation) REFERENCES ledger.rental_enrollment
);
CREATE UNIQUE INDEX renter_return_one_current_assignment ON ledger.renter_return_assignment(branch_id,device_ref,generation)
    WHERE revoked_at IS NULL;
REVOKE ALL ON ledger.renter_return_assignment FROM PUBLIC,nu54_p05_commerce;

CREATE FUNCTION ledger.offer_renter_return(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,
    customer uuid,account_ text,expires_ timestamptz)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE enrolled ledger.rental_enrollment;
DECLARE id uuid;
BEGIN
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
        AND a.revoked_at IS NULL;
    IF FOUND THEN RETURN id; END IF;
    INSERT INTO ledger.renter_return_assignment(branch_id,device_ref,generation,customer_ref,offered_by,account_ref,enrollment_digest,expires_at)
        VALUES(branch,device,generation_,customer,actor,account_,enrolled.enrollment_digest,expires_) RETURNING assignment_id INTO id;
    RETURN id;
END $$;

CREATE FUNCTION ledger.change_renter_return(tenant uuid,subject uuid,assignment uuid,action_ text,account_ text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE offer ledger.renter_return_assignment;
BEGIN
    SELECT a.* INTO offer FROM ledger.renter_return_assignment a JOIN ledger.branch b ON b.id=a.branch_id
        WHERE a.assignment_id=assignment AND b.tenant_ref=tenant;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='rental assignment unavailable'; END IF;
    PERFORM ledger.lock_rental_evidence_device(offer.branch_id,offer.device_ref);
    IF action_='revoke' THEN
        PERFORM ledger.rental_require_live_grant(tenant,offer.branch_id,subject);
        UPDATE ledger.renter_return_assignment SET revoked_at=clock_timestamp() WHERE assignment_id=assignment AND revoked_at IS NULL;
    ELSIF action_='accept' AND offer.customer_ref=subject AND offer.account_ref=account_ THEN
        PERFORM ledger.rental_require_live_grant(tenant,offer.branch_id,offer.offered_by);
        UPDATE ledger.renter_return_assignment a SET accepted_at=coalesce(a.accepted_at,clock_timestamp())
         WHERE a.assignment_id=assignment AND a.revoked_at IS NULL AND a.expires_at>clock_timestamp()
           AND EXISTS (SELECT FROM ledger.rental_enrollment e WHERE
              (e.branch_id,e.device_ref,e.generation,e.account_ref,e.enrollment_digest)=(a.branch_id,a.device_ref,a.generation,a.account_ref,a.enrollment_digest)
              AND e.attested_until>clock_timestamp())
           AND NOT EXISTS (SELECT FROM ledger.rental_enrollment e WHERE (e.branch_id,e.device_ref)=(a.branch_id,a.device_ref) AND e.generation>a.generation);
    ELSE
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='rental assignment unavailable';
    END IF;
    RETURN FOUND;
END $$;

CREATE FUNCTION ledger.read_renter_return(tenant uuid,customer uuid,assignment uuid)
RETURNS jsonb LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT jsonb_build_object('assignment',a.assignment_id,'generation',a.generation::text,'account',a.account_ref,
        'assignmentUntil',a.expires_at,'slot','unconfirmed','settlement','unknown')
    FROM ledger.renter_return_assignment a
      JOIN ledger.branch b ON b.id=a.branch_id
      JOIN ledger.rental_enrollment e USING(branch_id,device_ref,generation)
    WHERE a.assignment_id=assignment AND b.tenant_ref=tenant AND a.customer_ref=customer
      AND a.revoked_at IS NULL AND a.accepted_at IS NOT NULL AND a.expires_at>clock_timestamp()
      AND EXISTS (SELECT FROM ledger.finance_grant g WHERE (g.branch_id,g.actor_id,g.capability)=(a.branch_id,a.offered_by,'rental_manage')
          AND g.valid_until>clock_timestamp())
      AND e.attested_until>clock_timestamp() AND e.account_ref=a.account_ref AND e.enrollment_digest=a.enrollment_digest
      AND NOT EXISTS (SELECT FROM ledger.rental_enrollment latest WHERE (latest.branch_id,latest.device_ref)=(a.branch_id,a.device_ref)
          AND latest.generation>a.generation)
$$;

REVOKE ALL ON FUNCTION ledger.offer_renter_return(uuid,uuid,uuid,uuid,bigint,uuid,text,timestamptz),
    ledger.change_renter_return(uuid,uuid,uuid,text,text),ledger.read_renter_return(uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.offer_renter_return(uuid,uuid,uuid,uuid,bigint,uuid,text,timestamptz),
    ledger.change_renter_return(uuid,uuid,uuid,text,text),ledger.read_renter_return(uuid,uuid,uuid) TO nu54_p05_commerce;

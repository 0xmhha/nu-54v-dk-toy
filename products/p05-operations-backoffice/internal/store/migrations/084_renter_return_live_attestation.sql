-- Preserve 083 handovers and their identity; require current attestation at use.
CREATE OR REPLACE FUNCTION ledger.change_renter_return(tenant uuid,subject uuid,assignment uuid,action_ text,account_ text)
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
              AND e.attested_at<=clock_timestamp() AND e.attested_until>clock_timestamp())
           AND NOT EXISTS (SELECT FROM ledger.rental_enrollment e WHERE (e.branch_id,e.device_ref)=(a.branch_id,a.device_ref) AND e.generation>a.generation);
    ELSE
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='rental assignment unavailable';
    END IF;
    RETURN FOUND;
END $$;

CREATE OR REPLACE FUNCTION ledger.read_renter_return(tenant uuid,customer uuid,assignment uuid)
RETURNS jsonb LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT jsonb_build_object('assignment',a.assignment_id,'generation',a.generation::text,'account',a.account_ref,
        'assignmentUntil',least(a.expires_at,e.attested_until,g.valid_until),'slot','unconfirmed','settlement','unknown')
    FROM ledger.renter_return_assignment a
      JOIN ledger.branch b ON b.id=a.branch_id
      JOIN ledger.rental_enrollment e USING(branch_id,device_ref,generation)
      JOIN ledger.finance_grant g ON (g.branch_id,g.actor_id,g.capability)=(a.branch_id,a.offered_by,'rental_manage')
    WHERE a.assignment_id=assignment AND b.tenant_ref=tenant AND a.customer_ref=customer
      AND a.revoked_at IS NULL AND a.accepted_at IS NOT NULL AND a.expires_at>clock_timestamp()
      AND g.valid_until>clock_timestamp()
      AND e.attested_at<=clock_timestamp() AND e.attested_until>clock_timestamp() AND e.account_ref=a.account_ref AND e.enrollment_digest=a.enrollment_digest
      AND NOT EXISTS (SELECT FROM ledger.rental_enrollment latest WHERE (latest.branch_id,latest.device_ref)=(a.branch_id,a.device_ref)
          AND latest.generation>a.generation)
$$;


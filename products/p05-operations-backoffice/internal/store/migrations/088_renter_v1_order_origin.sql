-- The v1 terminal's original order is attached only to an already accepted
-- handover at issuance. The stored witness is a digest, never a signature.
CREATE TABLE ledger.renter_v1_order_origin (
    origin_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    assignment_id uuid NOT NULL REFERENCES ledger.renter_return_assignment(assignment_id),
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    terminal_id uuid NOT NULL,
    deployment_digest bytea NOT NULL CHECK (octet_length(deployment_digest)=32),
    merchant bytea NOT NULL CHECK (octet_length(merchant)=20),
    order_key bytea NOT NULL CHECK (octet_length(order_key)=32),
    merchant_digest bytea NOT NULL CHECK (octet_length(merchant_digest)=32),
    asset_ref text NOT NULL CHECK (asset_ref ~ '^0x[0-9a-f]{40}$'),
    amount ledger.units NOT NULL CHECK (amount>0),
    reserved_at timestamptz NOT NULL,
    authorization_until timestamptz NOT NULL CHECK (isfinite(authorization_until) AND authorization_until>reserved_at),
    approval_digest bytea CHECK (octet_length(approval_digest)=32),
    nonce bytea CHECK (octet_length(nonce)=32),
    CHECK ((approval_digest IS NULL)=(nonce IS NULL)),
    UNIQUE (deployment_digest,merchant,order_key)
);
CREATE INDEX renter_v1_order_origin_assignment ON ledger.renter_v1_order_origin(assignment_id,origin_id);

CREATE FUNCTION ledger.reserve_renter_v1_order(tenant_ uuid,branch_ uuid,terminal_ uuid,account_ text,
    deployment_ bytea,merchant_ bytea,order_ bytea,digest_ bytea,asset_ text,amount_ numeric,until_ timestamptz)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE active_ids uuid[];
DECLARE a ledger.renter_return_assignment;
DECLARE prior ledger.renter_v1_order_origin;
DECLARE result uuid;
DECLARE at_ timestamptz;
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,terminal_)).role_code IS DISTINCT FROM 'terminal' THEN RETURN NULL; END IF;
    SELECT array_agg(assignment_id) INTO active_ids FROM (
        SELECT r.assignment_id FROM ledger.renter_return_assignment r
        WHERE r.branch_id=branch_ AND r.account_ref=account_
          AND ledger.read_renter_return(tenant_,r.customer_ref,r.assignment_id) IS NOT NULL
          AND EXISTS (SELECT FROM ledger.rental_generation_source s WHERE
            (s.branch_id,s.device_ref,s.generation,s.account_ref,s.enrollment_digest,s.deployment_digest)
             =(r.branch_id,r.device_ref,r.generation,r.account_ref,r.enrollment_digest,deployment_)
            AND s.valid_until>clock_timestamp())
        LIMIT 2
    ) matched;
    IF cardinality(active_ids) IS DISTINCT FROM 1 THEN RETURN NULL; END IF;
    SELECT * INTO a FROM ledger.renter_return_assignment WHERE assignment_id=active_ids[1];
    PERFORM ledger.lock_rental_evidence_device(a.branch_id,a.device_ref);
    IF ledger.read_renter_return(tenant_,a.customer_ref,a.assignment_id) IS NULL OR NOT EXISTS (
        SELECT FROM ledger.rental_generation_source s
        WHERE (s.branch_id,s.device_ref,s.generation,s.account_ref,s.enrollment_digest,s.deployment_digest)
            =(a.branch_id,a.device_ref,a.generation,a.account_ref,a.enrollment_digest,deployment_)
            AND s.valid_until>clock_timestamp()
    ) THEN RETURN NULL; END IF;
    SELECT * INTO prior FROM ledger.renter_v1_order_origin
        WHERE (deployment_digest,merchant,order_key)=(deployment_,merchant_,order_) FOR UPDATE;
    IF FOUND THEN
        IF (prior.assignment_id,prior.branch_id,prior.terminal_id,prior.merchant_digest,prior.asset_ref,prior.amount,prior.authorization_until)
            IS DISTINCT FROM (a.assignment_id,branch_,terminal_,digest_,asset_,amount_,until_) THEN RETURN NULL; END IF;
        RETURN prior.origin_id;
    END IF;
    at_:=clock_timestamp();
    IF until_<=at_ OR until_>at_+interval '120 seconds' THEN RETURN NULL; END IF;
    INSERT INTO ledger.renter_v1_order_origin(assignment_id,branch_id,terminal_id,deployment_digest,merchant,order_key,
        merchant_digest,asset_ref,amount,reserved_at,authorization_until)
    VALUES(a.assignment_id,branch_,terminal_,deployment_,merchant_,order_,digest_,asset_,amount_,at_,until_)
    ON CONFLICT (deployment_digest,merchant,order_key) DO NOTHING RETURNING origin_id INTO result;
    RETURN result;
END $$;

CREATE FUNCTION ledger.confirm_renter_v1_order(tenant_ uuid,branch_ uuid,terminal_ uuid,origin_ uuid,account_ text,
    deployment_ bytea,merchant_ bytea,order_ bytea,merchant_digest_ bytea,approval_digest_ bytea,nonce_ bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE prior ledger.renter_v1_order_origin;
DECLARE a ledger.renter_return_assignment;
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,terminal_)).role_code IS DISTINCT FROM 'terminal' THEN RETURN false; END IF;
    SELECT * INTO prior FROM ledger.renter_v1_order_origin WHERE origin_id=origin_
      AND (branch_id,terminal_id,deployment_digest,merchant,order_key,merchant_digest)
        =(branch_,terminal_,deployment_,merchant_,order_,merchant_digest_);
    IF NOT FOUND THEN RETURN false; END IF;
    SELECT * INTO a FROM ledger.renter_return_assignment WHERE assignment_id=prior.assignment_id AND account_ref=account_;
    IF NOT FOUND THEN RETURN false; END IF;
    PERFORM ledger.lock_rental_evidence_device(a.branch_id,a.device_ref);
    IF ledger.read_renter_return(tenant_,a.customer_ref,a.assignment_id) IS NULL OR NOT EXISTS (
        SELECT FROM ledger.rental_generation_source s
        WHERE (s.branch_id,s.device_ref,s.generation,s.account_ref,s.enrollment_digest,s.deployment_digest)
            =(a.branch_id,a.device_ref,a.generation,a.account_ref,a.enrollment_digest,deployment_)
            AND s.valid_until>clock_timestamp()
    ) THEN RETURN false; END IF;
    UPDATE ledger.renter_v1_order_origin
        SET approval_digest=approval_digest_,nonce=nonce_
      WHERE origin_id=prior.origin_id AND (approval_digest IS NULL OR (approval_digest,nonce)=(approval_digest_,nonce_));
    RETURN FOUND;
END $$;

REVOKE ALL ON FUNCTION ledger.reserve_renter_v1_order(uuid,uuid,uuid,text,bytea,bytea,bytea,bytea,text,numeric,timestamptz),
  ledger.confirm_renter_v1_order(uuid,uuid,uuid,uuid,text,bytea,bytea,bytea,bytea,bytea,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.reserve_renter_v1_order(uuid,uuid,uuid,text,bytea,bytea,bytea,bytea,text,numeric,timestamptz),
  ledger.confirm_renter_v1_order(uuid,uuid,uuid,uuid,text,bytea,bytea,bytea,bytea,bytea,bytea) TO nu54_p05_commerce;

CREATE UNIQUE INDEX renter_v1_order_approval_nonce ON ledger.renter_v1_order_origin(assignment_id,nonce)
    WHERE approval_digest IS NOT NULL;

CREATE TABLE ledger.renter_v1_pending_witness (
    branch_id uuid NOT NULL,
    device_ref uuid NOT NULL,
    generation bigint NOT NULL,
    request_key uuid NOT NULL,
    witness bytea NOT NULL CHECK (octet_length(witness)=32),
    PRIMARY KEY(branch_id,device_ref,generation,request_key,witness),
    FOREIGN KEY(branch_id,device_ref,generation,request_key) REFERENCES ledger.rental_generation_authorized_proof
);

CREATE FUNCTION ledger.add_renter_v1_pending_witness(branch_ uuid,device_ uuid,generation_ bigint,request_ uuid,witness_ bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.lock_rental_evidence_device(branch_,device_);
    IF NOT EXISTS (SELECT FROM ledger.rental_generation_source s WHERE
        (s.branch_id,s.device_ref,s.generation,s.writer_login)=(branch_,device_,generation_,session_user::text)
        AND s.valid_until>clock_timestamp()) THEN RETURN false; END IF;
    IF (SELECT count(*) FROM ledger.renter_v1_pending_witness
            WHERE (branch_id,device_ref,generation,request_key)=(branch_,device_,generation_,request_)) >=
        (SELECT pending_payments FROM ledger.rental_generation_authorized_proof
            WHERE (branch_id,device_ref,generation,request_key)=(branch_,device_,generation_,request_)) THEN RETURN false; END IF;
    INSERT INTO ledger.renter_v1_pending_witness VALUES(branch_,device_,generation_,request_,witness_);
    RETURN true;
END $$;

CREATE FUNCTION ledger.read_renter_v1_pending_orders(tenant_ uuid,customer_ uuid,assignment_ uuid,writer_ bytea,scope_ text)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE live jsonb;
DECLARE result jsonb;
BEGIN
    live:=ledger.renter_return_assessment(tenant_,customer_,assignment_,writer_);
    IF live->>'scopeDigest' IS DISTINCT FROM scope_ OR scope_ IS NULL THEN RETURN '[]'::jsonb; END IF;
    SELECT jsonb_agg(jsonb_build_object('origin',o.origin_id,'amount',o.amount::text,'asset',o.asset_ref)
                     ORDER BY o.origin_id) INTO result
      FROM ledger.renter_return_assignment r
      JOIN ledger.rental_generation_authorized_proof p USING(branch_id,device_ref,generation)
      JOIN ledger.renter_v1_pending_witness w USING(branch_id,device_ref,generation,request_key)
      JOIN ledger.renter_v1_order_origin o ON o.assignment_id=r.assignment_id AND o.branch_id=r.branch_id
        AND o.deployment_digest=decode(substr(live->>'deployment',3),'hex')
        AND o.approval_digest IS NOT NULL
        AND sha256(o.nonce||o.merchant||o.order_key||o.approval_digest)=w.witness
      WHERE r.assignment_id=assignment_ AND r.customer_ref=customer_
        AND 'sha256:'||encode(p.assessment_digest,'hex')=live->>'assessment';
    RETURN coalesce(result,'[]'::jsonb);
END $$;

REVOKE ALL ON FUNCTION ledger.add_renter_v1_pending_witness(uuid,uuid,bigint,uuid,bytea),
    ledger.read_renter_v1_pending_orders(uuid,uuid,uuid,bytea,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.add_renter_v1_pending_witness(uuid,uuid,bigint,uuid,bytea) TO nu54_p05_rental_evidence;
GRANT EXECUTE ON FUNCTION ledger.read_renter_v1_pending_orders(uuid,uuid,uuid,bytea,text) TO nu54_p05_commerce;

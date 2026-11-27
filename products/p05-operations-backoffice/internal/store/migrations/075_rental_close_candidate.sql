-- Staging for the exact protected close transaction. The public 073 entry
-- remains held until the witnessed close executor is enabled separately.
DO $role$
BEGIN
    IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='nu54_p05_rental_close') THEN
        CREATE ROLE nu54_p05_rental_close NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
    ELSIF EXISTS(SELECT FROM pg_roles WHERE rolname='nu54_p05_rental_close' AND
        (rolcanlogin OR rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication OR rolbypassrls)) THEN
        RAISE EXCEPTION 'unsafe protected close role';
    END IF;
END
$role$;
GRANT USAGE ON SCHEMA ledger TO nu54_p05_rental_close;

CREATE TABLE ledger.rental_close_candidate (
    branch_id uuid NOT NULL,
    device_ref uuid NOT NULL,
    generation bigint NOT NULL,
    request_key uuid NOT NULL,
    actor_id uuid NOT NULL REFERENCES ledger.login_subject(subject_ref),
    proof_key uuid NOT NULL,
    review_ref text NOT NULL CHECK(review_ref~'^[0-9a-f]{64}$'),
    authority_digest bytea NOT NULL CHECK(octet_length(authority_digest)=32),
    account_ref text NOT NULL CHECK(account_ref~'^0x[0-9a-f]{40}$'),
    signer text NOT NULL CHECK(signer~'^0x[0-9a-f]{40}$'),
    nonce numeric NOT NULL CHECK(nonce=trunc(nonce) AND nonce>=0 AND nonce<18446744073709551616),
    tx_hash bytea NOT NULL UNIQUE CHECK(octet_length(tx_hash)=32),
    sealed_transaction bytea CHECK(octet_length(sealed_transaction) BETWEEN 65 AND 2048),
    state text NOT NULL CHECK(state IN ('prepared','attempting','submitted','confirmed','reverted','superseded')),
    CHECK((sealed_transaction IS NULL)=(state IN ('confirmed','reverted','superseded'))),
    attempt_until timestamptz CHECK(isfinite(attempt_until)),
    CHECK((attempt_until IS NOT NULL)=(state IN ('attempting','submitted'))),
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(recorded_at)),
    PRIMARY KEY(branch_id,device_ref,generation),
    UNIQUE(branch_id,actor_id,request_key),
    FOREIGN KEY(branch_id,device_ref,generation,proof_key) REFERENCES ledger.rental_proof_submission
);
CREATE UNIQUE INDEX rental_close_live_signer_nonce ON ledger.rental_close_candidate(signer,nonce)
    WHERE state IN ('prepared','attempting','submitted');
REVOKE ALL ON ledger.rental_close_candidate FROM PUBLIC,nu54_p05_commerce,nu54_p05_rental_close;

CREATE FUNCTION ledger.rental_close_live_scope(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,writer_ bytea)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE actor_until timestamptz;
DECLARE authority record;
DECLARE source ledger.rental_generation_source;
DECLARE audience ledger.rental_authority_audience;
DECLARE collector ledger.rental_collector_registration;
DECLARE proof ledger.rental_generation_authorized_proof;
DECLARE submission bytea;
DECLARE review jsonb;
DECLARE expiry timestamptz;
DECLARE binding bytea;
BEGIN
    PERFORM ledger.lock_rental_evidence_device(branch,device);
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||device::text||generation_::text,25));
    actor_until:=ledger.rental_require_live_grant(tenant,branch,actor);
    SELECT * INTO source FROM ledger.rental_generation_source s
       WHERE (s.branch_id,s.device_ref,s.generation)=(branch,device,generation_) FOR SHARE;
    SELECT * INTO audience FROM ledger.rental_authority_audience a
       WHERE (a.branch_id,a.device_ref,a.generation)=(branch,device,generation_) FOR SHARE;
    IF NOT FOUND OR audience.writer_certificate IS DISTINCT FROM writer_
       OR audience.authority_source IS DISTINCT FROM ledger.rental_generation_source_digest(source)
       OR NOT EXISTS (SELECT FROM pg_roles r WHERE r.rolname=source.writer_login AND r.rolcanlogin
           AND pg_has_role(r.oid,'nu54_p05_rental_evidence','USAGE')
           AND NOT (r.rolsuper OR r.rolcreaterole OR r.rolcreatedb OR r.rolbypassrls)) THEN
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='protected close authority unavailable';
    END IF;
    SELECT * INTO collector FROM ledger.rental_collector_registration c
       WHERE (c.branch_id,c.device_ref,c.generation)=(branch,device,generation_) FOR SHARE;
    SELECT * INTO authority FROM ledger.rental_collector_authority(tenant,branch,device,generation_,collector.collector_certificate);
    review:=ledger.rental_generation_authorized_review(tenant,branch,actor,device,generation_);
    SELECT * INTO proof FROM ledger.rental_generation_authorized_proof p
       WHERE (p.branch_id,p.device_ref,p.generation)=(branch,device,generation_)
         AND p.source_digest=ledger.rental_generation_source_digest(source)
       ORDER BY p.recorded_at DESC,p.request_key DESC LIMIT 1 FOR SHARE;
    SELECT q.submitted_digest INTO submission FROM ledger.rental_proof_submission q
       WHERE (q.branch_id,q.device_ref,q.generation,q.request_key)=(branch,device,generation_,proof.request_key) FOR SHARE;
    expiry:=least(actor_until,audience.valid_until,authority.deadline,proof.fresh_until);
    IF NOT FOUND OR proof.challenge IS DISTINCT FROM source.challenge OR proof.pending_payments<>0 OR proof.pending_limits<>0
       OR review->>'returnReviewRef' IS NULL OR expiry<=clock_timestamp() THEN
        RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='current submitted zero proof required';
    END IF;
    binding:=sha256(convert_to(jsonb_build_array('rental-close-authority-v1',tenant,branch,device,generation_,actor,
       proof.source_digest,proof.request_key,proof.freeze_digest,proof.journal_root,proof.assessment_digest,
       proof.finalized_height,proof.finalized_hash,submission,collector.registration_proof,authority.identity_key,
       audience.audience_certificate,audience.writer_certificate,extract(epoch FROM expiry),review->>'returnReviewRef')::text,'UTF8'));
    RETURN jsonb_build_object('scopeDigest',encode(binding,'hex'),'proofKey',proof.request_key,'reviewRef',review->>'returnReviewRef',
       'account',source.account_ref,'destination',source.withdraw_ref,'registration','0x'||encode(source.enrollment_digest,'hex'),
       'measurement','0x'||encode(source.board_measurement,'hex'),'deployment','0x'||encode(source.deployment_digest,'hex'),
       'settlement',source.settlement_ref,'challenge','0x'||encode(source.challenge,'hex'),'stream',source.stream_ref,
       'streamGeneration',source.stream_generation,'assessment','sha256:'||encode(proof.assessment_digest,'hex'),
       'height',proof.finalized_height,'hash','0x'||encode(proof.finalized_hash,'hex'),'expiresAt',floor(extract(epoch FROM expiry))::bigint);
END $$;

CREATE FUNCTION ledger.rental_close_recover(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,key_ uuid,review_ text)
RETURNS TABLE(state text,tx_hash bytea,sealed_transaction bytea,nonce numeric,account_ref text,signer text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE prior ledger.rental_close_candidate;
BEGIN
    PERFORM ledger.rental_require_live_grant(tenant,branch,actor);
    SELECT * INTO prior FROM ledger.rental_close_candidate c WHERE (c.branch_id,c.device_ref,c.generation)=(branch,device,generation_);
    IF NOT FOUND THEN RETURN; END IF;
    IF (prior.request_key,prior.actor_id,prior.review_ref) IS DISTINCT FROM (key_,actor,review_) THEN
        RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='protected close already has an original request';
    END IF;
    RETURN QUERY SELECT prior.state,prior.tx_hash,prior.sealed_transaction,prior.nonce,prior.account_ref,prior.signer;
END $$;

CREATE FUNCTION ledger.rental_close_admit(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,key_ uuid,
    review_ text,writer_ bytea,scope_ bytea,lease_ bigint,signer_ text,nonce_ numeric,hash_ bytea,sealed_ bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE live jsonb;
BEGIN
    PERFORM ledger.lock_rental_evidence_device(branch,device);
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||device::text||generation_::text,25));
    IF EXISTS(SELECT FROM ledger.rental_close_recover(tenant,branch,actor,device,generation_,key_,review_)) THEN RETURN false; END IF;
    live:=ledger.rental_close_live_scope(tenant,branch,actor,device,generation_,writer_);
    IF live->>'scopeDigest' IS DISTINCT FROM encode(scope_,'hex') OR live->>'reviewRef' IS DISTINCT FROM review_
       OR lease_ IS NULL OR lease_>(live->>'expiresAt')::bigint OR lease_>floor(extract(epoch FROM clock_timestamp()))+5
       OR lease_<=extract(epoch FROM clock_timestamp())
       OR EXISTS(SELECT FROM ledger.rental_operation o WHERE (o.branch_id,o.device_ref,o.generation,o.action)=(branch,device,generation_,'close')) THEN
        RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='protected close source changed before admission';
    END IF;
    INSERT INTO ledger.rental_close_candidate(branch_id,device_ref,generation,request_key,actor_id,proof_key,
        review_ref,authority_digest,account_ref,signer,nonce,tx_hash,sealed_transaction,state)
    VALUES(branch,device,generation_,key_,actor,(live->>'proofKey')::uuid,review_,scope_,live->>'account',signer_,nonce_,hash_,sealed_,'prepared');
    RETURN true;
END $$;

CREATE FUNCTION ledger.rental_close_mark(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,key_ uuid,
    review_ text,hash_ bytea,next_ text,writer_ bytea,scope_ bytea,lease_ bigint)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE original record;
DECLARE live jsonb;
BEGIN
    PERFORM ledger.lock_rental_evidence_device(branch,device);
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||device::text||generation_::text,25));
    SELECT * INTO original FROM ledger.rental_close_recover(tenant,branch,actor,device,generation_,key_,review_);
    IF NOT FOUND OR original.tx_hash IS DISTINCT FROM hash_ THEN
        RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='original protected close hash changed';
    END IF;
    IF original.state IN ('confirmed','reverted','superseded') THEN RETURN false; END IF;
    IF next_='attempting' THEN
        IF EXISTS(SELECT FROM ledger.rental_close_candidate c WHERE (c.branch_id,c.device_ref,c.generation)=(branch,device,generation_)
                  AND c.attempt_until>clock_timestamp()) THEN RETURN false; END IF;
        live:=ledger.rental_close_live_scope(tenant,branch,actor,device,generation_,writer_);
        IF live->>'scopeDigest' IS DISTINCT FROM encode(scope_,'hex') OR live->>'reviewRef' IS DISTINCT FROM review_
           OR lease_ IS NULL OR lease_>(live->>'expiresAt')::bigint OR lease_>floor(extract(epoch FROM clock_timestamp()))+5
           OR lease_<=extract(epoch FROM clock_timestamp())
           OR NOT EXISTS(SELECT FROM ledger.rental_close_candidate c
             WHERE (c.branch_id,c.device_ref,c.generation)=(branch,device,generation_) AND c.authority_digest=scope_) THEN
            RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='protected close source changed before send';
        END IF;
    ELSIF next_ NOT IN ('submitted','confirmed','reverted','superseded') OR next_ IS NULL OR
          (next_<>'superseded' AND original.state NOT IN ('attempting','submitted')) THEN
        RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='protected close transition denied';
    END IF;
    UPDATE ledger.rental_close_candidate c SET state=next_,
        attempt_until=CASE WHEN next_='attempting' THEN clock_timestamp()+interval '20 seconds'
                           WHEN next_='submitted' THEN c.attempt_until END,
        sealed_transaction=CASE WHEN next_ IN ('attempting','submitted') THEN c.sealed_transaction END
      WHERE (c.branch_id,c.device_ref,c.generation)=(branch,device,generation_);
    RETURN true;
END $$;

REVOKE ALL ON FUNCTION ledger.rental_close_live_scope(uuid,uuid,uuid,uuid,bigint,bytea),
    ledger.rental_close_recover(uuid,uuid,uuid,uuid,bigint,uuid,text),
    ledger.rental_close_admit(uuid,uuid,uuid,uuid,bigint,uuid,text,bytea,bytea,bigint,text,numeric,bytea,bytea),
    ledger.rental_close_mark(uuid,uuid,uuid,uuid,bigint,uuid,text,bytea,text,bytea,bytea,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.rental_close_live_scope(uuid,uuid,uuid,uuid,bigint,bytea),
    ledger.rental_close_recover(uuid,uuid,uuid,uuid,bigint,uuid,text),
    ledger.rental_close_admit(uuid,uuid,uuid,uuid,bigint,uuid,text,bytea,bytea,bigint,text,numeric,bytea,bytea),
    ledger.rental_close_mark(uuid,uuid,uuid,uuid,bigint,uuid,text,bytea,text,bytea,bytea,bigint) TO nu54_p05_rental_close;

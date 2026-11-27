-- Protected approvals are opaque and now identify the exact authority scope.
-- Stored older approvals remain observation-only via the original 075 identity.
CREATE OR REPLACE FUNCTION ledger.rental_close_live_scope(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,writer_ bytea)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE actor_until timestamptz;
DECLARE authority record;
DECLARE enrollment ledger.rental_enrollment;
DECLARE installed ledger.firmware_member;
DECLARE collector_operator_until timestamptz;
DECLARE source ledger.rental_generation_source;
DECLARE audience ledger.rental_authority_audience;
DECLARE collector ledger.rental_collector_registration;
DECLARE proof ledger.rental_generation_authorized_proof;
DECLARE submission bytea;
DECLARE review jsonb;
DECLARE expiry timestamptz;
DECLARE binding bytea;
DECLARE legacy_binding bytea;
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
    collector_operator_until:=ledger.rental_require_live_grant(tenant,branch,collector.operator_ref);
    SELECT * INTO enrollment FROM ledger.rental_enrollment e
       WHERE (e.branch_id,e.device_ref,e.generation)=(branch,device,generation_) FOR SHARE;
    installed:=ledger.rental_installed_board(branch,device,enrollment.revision,source.board_measurement);
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
    legacy_binding:=sha256(convert_to(jsonb_build_array('rental-close-authority-v1',tenant,branch,device,generation_,actor,
       proof.source_digest,proof.request_key,proof.freeze_digest,proof.journal_root,proof.assessment_digest,
       proof.finalized_height,proof.finalized_hash,submission,collector.registration_proof,authority.identity_key,
       audience.audience_certificate,audience.writer_certificate,extract(epoch FROM expiry),review->>'returnReviewRef')::text,'UTF8'));
    binding:=sha256(convert_to(jsonb_build_array('rental-close-authority-v2',tenant,branch,device,generation_,actor,
       proof.source_digest,proof.request_key,proof.freeze_digest,proof.journal_root,proof.assessment_digest,
       proof.finalized_height,proof.finalized_hash,submission,collector.registration_proof,authority.identity_key,
       collector.operator_ref,collector.collector_certificate,collector.collector_peer,collector.first_nonce,
       installed.board_id,installed.installed_counter,installed.installed_version,
       extract(epoch FROM enrollment.attested_at),extract(epoch FROM enrollment.attested_until),
       extract(epoch FROM collector_operator_until),
       audience.audience_certificate,audience.writer_certificate,audience.reader_login,
       extract(epoch FROM audience.valid_until),extract(epoch FROM collector.valid_until),extract(epoch FROM actor_until),
       extract(epoch FROM expiry),review->>'returnReviewRef')::text,'UTF8'));
    RETURN jsonb_build_object('scopeDigest',encode(binding,'hex'),'legacyScopeDigest',encode(legacy_binding,'hex'),
       'legacyReviewRef',review->>'returnReviewRef','proofKey',proof.request_key,'reviewRef',encode(sha256(convert_to('rental-close-operator-review-v1:'||encode(binding,'hex'),'UTF8')),'hex'),
       'account',source.account_ref,'destination',source.withdraw_ref,'registration','0x'||encode(source.enrollment_digest,'hex'),
       'measurement','0x'||encode(source.board_measurement,'hex'),'deployment','0x'||encode(source.deployment_digest,'hex'),
       'settlement',source.settlement_ref,'challenge','0x'||encode(source.challenge,'hex'),'stream',source.stream_ref,
       'streamGeneration',source.stream_generation,'assessment','sha256:'||encode(proof.assessment_digest,'hex'),
       'height',proof.finalized_height,'hash','0x'||encode(proof.finalized_hash,'hex'),'expiresAt',floor(extract(epoch FROM expiry))::bigint);
END $$;

ALTER TABLE ledger.rental_close_candidate ADD COLUMN legacy_migration_scope bytea
    CHECK(octet_length(legacy_migration_scope)=32);

-- An old scope never authorizes a fresh candidate. Fence existing active
-- originals to the full currently verifiable scope at this migration, so any
-- later rotation, including a non-minimum deadline, requires fresh review.
DO $legacy_recovery$
DECLARE original record;
DECLARE live jsonb;
BEGIN
    FOR original IN
        SELECT c.*, b.tenant_ref, a.writer_certificate FROM ledger.rental_close_candidate c
        JOIN ledger.branch b ON b.id=c.branch_id
        JOIN ledger.rental_authority_audience a USING(branch_id,device_ref,generation)
        WHERE c.state IN ('prepared','attempting','submitted')
    LOOP
        BEGIN
            live:=ledger.rental_close_live_scope(original.tenant_ref,original.branch_id,original.actor_id,
                original.device_ref,original.generation,original.writer_certificate);
            IF live->>'legacyScopeDigest'=encode(original.authority_digest,'hex')
               AND live->>'legacyReviewRef'=original.review_ref THEN
                UPDATE ledger.rental_close_candidate SET legacy_migration_scope=decode(live->>'scopeDigest','hex')
                   WHERE (branch_id,device_ref,generation)=(original.branch_id,original.device_ref,original.generation);
            END IF;
        EXCEPTION WHEN insufficient_privilege OR SQLSTATE 'P0501' THEN
            -- No current basis can be attested for this older original.
        END;
    END LOOP;
END $legacy_recovery$;

CREATE OR REPLACE FUNCTION ledger.rental_close_mark(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,key_ uuid,
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
        IF NOT ((live->>'scopeDigest'=encode(scope_,'hex'))
                OR (live->>'legacyScopeDigest'=encode(scope_,'hex') AND live->>'legacyReviewRef'=review_
                    AND EXISTS(SELECT FROM ledger.rental_close_candidate c
                        WHERE (c.branch_id,c.device_ref,c.generation)=(branch,device,generation_)
                          AND c.legacy_migration_scope=decode(live->>'scopeDigest','hex'))))
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


CREATE FUNCTION ledger.rental_close_reauthorize(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,
    key_ uuid,review_ text,live_review_ text,hash_ bytea,writer_ bytea,scope_ bytea,lease_ bigint)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE original record;
DECLARE live jsonb;
BEGIN
    PERFORM ledger.lock_rental_evidence_device(branch,device);
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||device::text||generation_::text,25));
    SELECT * INTO original FROM ledger.rental_close_recover(tenant,branch,actor,device,generation_,key_,review_);
    IF NOT FOUND OR original.tx_hash IS DISTINCT FROM hash_ OR original.state NOT IN ('prepared','attempting','submitted')
       OR original.sealed_transaction IS NULL THEN
        RETURN false;
    END IF;
    live:=ledger.rental_close_live_scope(tenant,branch,actor,device,generation_,writer_);
    IF live->>'scopeDigest' IS DISTINCT FROM encode(scope_,'hex') OR live->>'reviewRef' IS DISTINCT FROM live_review_
        OR live->>'account' IS DISTINCT FROM original.account_ref OR lease_ IS NULL
        OR lease_>(live->>'expiresAt')::bigint OR lease_>floor(extract(epoch FROM clock_timestamp()))+5
        OR lease_<=extract(epoch FROM clock_timestamp()) THEN
        RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='protected original recovery review changed';
    END IF;
    UPDATE ledger.rental_close_candidate c SET authority_digest=scope_,legacy_migration_scope=NULL
        WHERE (c.branch_id,c.device_ref,c.generation)=(branch,device,generation_)
        AND c.tx_hash=hash_ AND EXISTS(
            SELECT FROM ledger.rental_generation_authorized_proof prior_proof
              JOIN ledger.rental_generation_authorized_proof live_proof
              ON (live_proof.branch_id,live_proof.device_ref,live_proof.generation)=
                 (prior_proof.branch_id,prior_proof.device_ref,prior_proof.generation)
            WHERE (prior_proof.branch_id,prior_proof.device_ref,prior_proof.generation,prior_proof.request_key)=(branch,device,generation_,c.proof_key)
              AND live_proof.request_key=(live->>'proofKey')::uuid
              AND (prior_proof.enrollment_digest,prior_proof.journal_root,prior_proof.first_nonce,prior_proof.next_nonce,prior_proof.frozen_at,prior_proof.stream_ref,
                   prior_proof.stream_generation,prior_proof.payment_entries,prior_proof.limit_entries,prior_proof.burned_entries)
                = (live_proof.enrollment_digest,live_proof.journal_root,live_proof.first_nonce,live_proof.next_nonce,live_proof.frozen_at,live_proof.stream_ref,
                   live_proof.stream_generation,live_proof.payment_entries,live_proof.limit_entries,live_proof.burned_entries));
    RETURN FOUND;
END $$;
REVOKE ALL ON FUNCTION ledger.rental_close_reauthorize(uuid,uuid,uuid,uuid,bigint,uuid,text,text,bytea,bytea,bytea,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.rental_close_reauthorize(uuid,uuid,uuid,uuid,bigint,uuid,text,text,bytea,bytea,bytea,bigint) TO nu54_p05_rental_close;

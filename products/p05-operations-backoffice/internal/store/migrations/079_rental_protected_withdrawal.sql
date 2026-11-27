CREATE TABLE ledger.rental_withdrawal_candidate (
    branch_id uuid NOT NULL,
    device_ref uuid NOT NULL,
    generation bigint NOT NULL,
    request_key uuid NOT NULL,
    actor_id uuid NOT NULL REFERENCES ledger.login_subject(subject_ref),
    review_ref text NOT NULL CHECK(review_ref~'^[0-9a-f]{64}$'),
    signer text NOT NULL CHECK(signer~'^0x[0-9a-f]{40}$'),
    nonce numeric NOT NULL CHECK(nonce=trunc(nonce) AND nonce>=0 AND nonce<18446744073709551616),
    sealed_transaction bytea CHECK(octet_length(sealed_transaction) BETWEEN 65 AND 2048),
    attempt_until timestamptz CHECK(isfinite(attempt_until)),
    tx_hash bytea NOT NULL UNIQUE CHECK(octet_length(tx_hash)=32),
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(recorded_at)),
    PRIMARY KEY(branch_id,device_ref,generation),
    UNIQUE(branch_id,actor_id,request_key),
    UNIQUE(signer,nonce),
    FOREIGN KEY(branch_id,device_ref,generation) REFERENCES ledger.rental_close_candidate
);
REVOKE ALL ON ledger.rental_withdrawal_candidate FROM PUBLIC,nu54_p05_commerce,nu54_p05_rental_close;

CREATE FUNCTION ledger.rental_protected_withdrawal_snapshot(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE original ledger.rental_close_candidate;
DECLARE prior ledger.rental_withdrawal_candidate;
DECLARE member ledger.rental_enrollment;
DECLARE proof ledger.rental_generation_authorized_proof;
DECLARE source ledger.rental_generation_source;
DECLARE current_ boolean;
BEGIN
    PERFORM ledger.rental_require_live_grant(tenant,branch,actor);
    SELECT * INTO original FROM ledger.rental_close_candidate c
        WHERE (c.branch_id,c.device_ref,c.generation)=(branch,device,generation_);
    IF NOT FOUND THEN RETURN '{}'::jsonb; END IF;
    SELECT * INTO member FROM ledger.rental_enrollment e WHERE (e.branch_id,e.device_ref,e.generation)=(branch,device,generation_);
    SELECT * INTO source FROM ledger.rental_generation_source s WHERE (s.branch_id,s.device_ref,s.generation)=(branch,device,generation_);
    SELECT * INTO proof FROM ledger.rental_generation_authorized_proof p
        WHERE (p.branch_id,p.device_ref,p.generation,p.request_key)=(branch,device,generation_,original.proof_key);
    SELECT * INTO prior FROM ledger.rental_withdrawal_candidate w WHERE (w.branch_id,w.device_ref,w.generation)=(branch,device,generation_);
    current_ := original.state='confirmed' AND member.attested_at<=clock_timestamp() AND member.attested_until>clock_timestamp()
        AND (member.account_ref, member.withdraw_ref, member.enrollment_digest, member.revision) =
            (original.account_ref, source.withdraw_ref, proof.enrollment_digest, source.enrollment_revision)
        AND (source.account_ref,source.enrollment_digest,source.stream_ref,source.stream_generation) =
            (original.account_ref,proof.enrollment_digest,proof.stream_ref,proof.stream_generation)
        AND NOT EXISTS(SELECT FROM ledger.rental_enrollment e
                    WHERE (e.branch_id,e.device_ref)=(branch,device) AND e.generation>generation_);
    RETURN jsonb_build_object('protected',true,'current',coalesce(current_,false),
        'closeState',original.state,'closeHash','0x'||encode(original.tx_hash,'hex'),
        'account',original.account_ref,'destination',member.withdraw_ref,
        'withdrawalHash',CASE WHEN prior.tx_hash IS NOT NULL THEN '0x'||encode(prior.tx_hash,'hex') END)
        || CASE WHEN prior.actor_id=actor THEN jsonb_build_object('key',prior.request_key,'review',prior.review_ref) ELSE '{}'::jsonb END;
END $$;

CREATE FUNCTION ledger.rental_protected_signer_nonce()
RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(hashtextextended(NEW.signer||':'||NEW.nonce::text,79));
    IF (TG_TABLE_NAME='rental_close_candidate' AND EXISTS(
            SELECT FROM ledger.rental_withdrawal_candidate w WHERE (w.signer,w.nonce)=(NEW.signer,NEW.nonce)))
        OR (TG_TABLE_NAME='rental_withdrawal_candidate' AND EXISTS(
            SELECT FROM ledger.rental_close_candidate c WHERE (c.signer,c.nonce)=(NEW.signer,NEW.nonce)
                  AND c.state IN ('prepared','attempting','submitted'))) THEN
        RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='protected rental signer nonce already reserved';
    END IF;
    RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION ledger.rental_protected_signer_nonce() FROM PUBLIC;
CREATE TRIGGER rental_close_protected_nonce BEFORE INSERT ON ledger.rental_close_candidate
    FOR EACH ROW EXECUTE FUNCTION ledger.rental_protected_signer_nonce();
CREATE TRIGGER rental_withdrawal_protected_nonce BEFORE INSERT ON ledger.rental_withdrawal_candidate
    FOR EACH ROW EXECUTE FUNCTION ledger.rental_protected_signer_nonce();

CREATE FUNCTION ledger.rental_protected_withdrawal_recover(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,key_ uuid,review_ text)
RETURNS TABLE(signer text,nonce numeric,sealed_transaction bytea) LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.rental_require_live_grant(tenant,branch,actor);
    RETURN QUERY SELECT w.signer,w.nonce,w.sealed_transaction FROM ledger.rental_withdrawal_candidate w
        WHERE (w.branch_id,w.device_ref,w.generation,w.actor_id,w.request_key,w.review_ref)=(branch,device,generation_,actor,key_,review_);
END $$;

CREATE FUNCTION ledger.rental_protected_withdrawal_admit(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,
    key_ uuid,review_ text,close_hash_ bytea,withdrawal_hash_ bytea,writer_ bytea,scope_ text,lease_ bigint,signer_ text,nonce_ numeric,sealed_ bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE snapshot jsonb;
DECLARE live jsonb;
BEGIN
    PERFORM ledger.lock_rental_evidence_device(branch,device);
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||device::text||generation_::text,25));
    snapshot:=ledger.rental_protected_withdrawal_snapshot(tenant,branch,actor,device,generation_);
    IF snapshot->>'withdrawalHash' IS NOT NULL THEN
        IF (snapshot->>'key',snapshot->>'review') IS DISTINCT FROM (key_::text,review_) THEN
            RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='protected withdrawal already has an original request';
        END IF;
        RETURN false;
    END IF;
    live:=ledger.rental_close_live_scope(tenant,branch,actor,device,generation_,writer_);
    IF NOT coalesce((snapshot->>'current')::boolean,false)
        OR snapshot->>'closeHash' IS DISTINCT FROM '0x'||encode(close_hash_,'hex')
        OR snapshot->>'account' IS DISTINCT FROM live->>'account'
        OR snapshot->>'destination' IS DISTINCT FROM live->>'destination'
        OR live->>'scopeDigest' IS DISTINCT FROM scope_
        OR lease_ IS NULL OR lease_>floor(extract(epoch FROM clock_timestamp()))+5
        OR lease_>(live->>'expiresAt')::bigint OR lease_<=extract(epoch FROM clock_timestamp())
        OR EXISTS(SELECT FROM ledger.rental_operation w WHERE
            (w.branch_id,w.device_ref,w.generation,w.action)=(branch,device,generation_,'withdraw')) THEN
        RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='protected withdrawal scope changed before admission';
    END IF;
    INSERT INTO ledger.rental_withdrawal_candidate(branch_id,device_ref,generation,request_key,actor_id,review_ref,tx_hash,signer,nonce,sealed_transaction)
        VALUES(branch,device,generation_,key_,actor,review_,withdrawal_hash_,signer_,nonce_,sealed_);
    RETURN true;
END $$;

CREATE FUNCTION ledger.rental_protected_withdrawal_claim(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,
    key_ uuid,review_ text,close_hash_ bytea,withdrawal_hash_ bytea,writer_ bytea,scope_ text,lease_ bigint,signer_ text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE snapshot jsonb;
DECLARE live jsonb;
BEGIN
    PERFORM ledger.lock_rental_evidence_device(branch,device);
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||device::text||generation_::text,25));
    snapshot:=ledger.rental_protected_withdrawal_snapshot(tenant,branch,actor,device,generation_);
    live:=ledger.rental_close_live_scope(tenant,branch,actor,device,generation_,writer_);
    IF NOT coalesce((snapshot->>'current')::boolean,false) OR snapshot->>'closeHash' IS DISTINCT FROM '0x'||encode(close_hash_,'hex')
        OR snapshot->>'withdrawalHash' IS DISTINCT FROM '0x'||encode(withdrawal_hash_,'hex')
        OR (snapshot->>'key',snapshot->>'review') IS DISTINCT FROM (key_::text,review_)
        OR snapshot->>'account' IS DISTINCT FROM live->>'account' OR snapshot->>'destination' IS DISTINCT FROM live->>'destination'
        OR live->>'scopeDigest' IS DISTINCT FROM scope_ OR lease_ IS NULL
        OR lease_>(live->>'expiresAt')::bigint OR lease_>floor(extract(epoch FROM clock_timestamp()))+5
        OR lease_<=extract(epoch FROM clock_timestamp()) THEN
        RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='protected withdrawal scope changed before send';
    END IF;
    UPDATE ledger.rental_withdrawal_candidate w SET attempt_until=clock_timestamp()+interval '20 seconds'
        WHERE (w.branch_id,w.device_ref,w.generation,w.signer)=(branch,device,generation_,signer_)
        AND w.sealed_transaction IS NOT NULL AND (w.attempt_until IS NULL OR w.attempt_until<=clock_timestamp());
    RETURN FOUND;
END $$;

CREATE FUNCTION ledger.rental_protected_withdrawal_clear(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,hash_ bytea)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.rental_require_live_grant(tenant,branch,actor);
    UPDATE ledger.rental_withdrawal_candidate w SET sealed_transaction=NULL,attempt_until=NULL
        WHERE (w.branch_id,w.device_ref,w.generation,w.tx_hash)=(branch,device,generation_,hash_);
END $$;

REVOKE ALL ON FUNCTION ledger.rental_protected_withdrawal_snapshot(uuid,uuid,uuid,uuid,bigint),
    ledger.rental_protected_withdrawal_admit(uuid,uuid,uuid,uuid,bigint,uuid,text,bytea,bytea,bytea,text,bigint,text,numeric,bytea),
    ledger.rental_protected_withdrawal_recover(uuid,uuid,uuid,uuid,bigint,uuid,text),
    ledger.rental_protected_withdrawal_claim(uuid,uuid,uuid,uuid,bigint,uuid,text,bytea,bytea,bytea,text,bigint,text),
    ledger.rental_protected_withdrawal_clear(uuid,uuid,uuid,uuid,bigint,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.rental_protected_withdrawal_snapshot(uuid,uuid,uuid,uuid,bigint),
    ledger.rental_protected_withdrawal_admit(uuid,uuid,uuid,uuid,bigint,uuid,text,bytea,bytea,bytea,text,bigint,text,numeric,bytea),
    ledger.rental_protected_withdrawal_recover(uuid,uuid,uuid,uuid,bigint,uuid,text),
    ledger.rental_protected_withdrawal_claim(uuid,uuid,uuid,uuid,bigint,uuid,text,bytea,bytea,bytea,text,bigint,text),
    ledger.rental_protected_withdrawal_clear(uuid,uuid,uuid,uuid,bigint,bytea) TO nu54_p05_rental_close;

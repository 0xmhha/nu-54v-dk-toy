CREATE TABLE ledger.rental_withdrawal_attempt_archive (
    branch_id uuid NOT NULL,
    device_ref uuid NOT NULL,
    generation bigint NOT NULL,
    actor_id uuid NOT NULL REFERENCES ledger.login_subject(subject_ref),
    request_key uuid NOT NULL,
    review_ref text NOT NULL CHECK(review_ref~'^[0-9a-f]{64}$'),
    tx_hash bytea NOT NULL UNIQUE CHECK(octet_length(tx_hash)=32),
    successor_hash bytea NOT NULL UNIQUE CHECK(octet_length(successor_hash)=32),
    signer text NOT NULL CHECK(signer~'^0x[0-9a-f]{40}$'),
    nonce numeric NOT NULL CHECK(nonce=trunc(nonce) AND nonce>=0 AND nonce<18446744073709551616),
    status text NOT NULL CHECK(status IN ('reverted','superseded')),
    evidence_hash bytea NOT NULL CHECK(octet_length(evidence_hash)=32),
    finalized_height bigint NOT NULL CHECK(finalized_height>0),
    finalized_hash bytea NOT NULL CHECK(octet_length(finalized_hash)=32),
    confirmed_at timestamptz NOT NULL CHECK(isfinite(confirmed_at)),
    PRIMARY KEY(branch_id,actor_id,request_key),
    UNIQUE(signer,nonce),
    FOREIGN KEY(branch_id,device_ref,generation) REFERENCES ledger.rental_close_candidate
);
REVOKE ALL ON ledger.rental_withdrawal_attempt_archive FROM PUBLIC,nu54_p05_commerce,
    nu54_p05_rental_close,nu54_p05_rental_reconcile;

CREATE FUNCTION ledger.rental_withdrawal_archived(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,key_ uuid,review_ text)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE prior ledger.rental_withdrawal_attempt_archive;
DECLARE close_ bytea;
BEGIN
    PERFORM ledger.rental_require_live_grant(tenant,branch,actor);
    SELECT * INTO prior FROM ledger.rental_withdrawal_attempt_archive a
        WHERE (a.branch_id,a.actor_id,a.request_key)=(branch,actor,key_);
    IF NOT FOUND THEN RETURN '{}'::jsonb; END IF;
    IF (prior.device_ref,prior.generation,prior.review_ref) IS DISTINCT FROM (device,generation_,review_) THEN
        RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='archived protected withdrawal scope differs';
    END IF;
    SELECT c.tx_hash INTO close_ FROM ledger.rental_close_candidate c
        WHERE (c.branch_id,c.device_ref,c.generation)=(branch,device,generation_);
    RETURN jsonb_build_object('state','withdrawal-'||prior.status,'requestKey',prior.request_key,
        'closeHash','0x'||encode(close_,'hex'),'withdrawalHash','0x'||encode(prior.tx_hash,'hex'),
        'attempt',jsonb_build_object('state',prior.status,'originalHash','0x'||encode(prior.tx_hash,'hex'),
          'evidenceHash','0x'||encode(prior.evidence_hash,'hex'),'confirmedAt',prior.confirmed_at));
END $$;

CREATE FUNCTION ledger.rental_withdrawal_predecessor(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,successor_ bytea)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE prior ledger.rental_withdrawal_attempt_archive;
BEGIN
    PERFORM ledger.rental_require_live_grant(tenant,branch,actor);
    SELECT * INTO prior FROM ledger.rental_withdrawal_attempt_archive a
        WHERE (a.branch_id,a.device_ref,a.generation,a.successor_hash)=(branch,device,generation_,successor_);
    IF NOT FOUND THEN RETURN '{}'::jsonb; END IF;
    RETURN jsonb_build_object('state',prior.status,'originalHash','0x'||encode(prior.tx_hash,'hex'),
          'evidenceHash','0x'||encode(prior.evidence_hash,'hex'),'confirmedAt',prior.confirmed_at,
          'finalizedHeight',prior.finalized_height,'finalizedHash','0x'||encode(prior.finalized_hash,'hex'));
END $$;
REVOKE ALL ON FUNCTION ledger.rental_withdrawal_predecessor(uuid,uuid,uuid,uuid,bigint,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.rental_withdrawal_predecessor(uuid,uuid,uuid,uuid,bigint,bytea) TO nu54_p05_rental_close;

CREATE FUNCTION ledger.rental_protected_withdrawal_retry(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,
    prior_ bytea,evidence_ bytea,key_ uuid,review_ text,close_hash_ bytea,withdrawal_hash_ bytea,writer_ bytea,
    scope_ text,lease_ bigint,signer_ text,nonce_ numeric,sealed_ bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.lock_rental_evidence_device(branch,device);
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||device::text||generation_::text,25));
    PERFORM ledger.rental_require_live_grant(tenant,branch,actor);
    IF EXISTS (SELECT FROM ledger.rental_withdrawal_attempt_archive a WHERE
               (a.branch_id,a.actor_id,a.request_key)=(branch,actor,key_))
       OR EXISTS (SELECT FROM ledger.rental_withdrawal_candidate c
                  WHERE (c.branch_id,c.actor_id,c.request_key)=(branch,actor,key_))
       OR EXISTS (SELECT FROM ledger.rental_withdrawal_attempt_archive a WHERE (a.signer,a.nonce)=(signer_,nonce_)) THEN
        RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='protected withdrawal retry requires a new key and nonce';
    END IF;
    INSERT INTO ledger.rental_withdrawal_attempt_archive(branch_id,device_ref,generation,actor_id,request_key,review_ref,
        tx_hash,successor_hash,signer,nonce,status,evidence_hash,finalized_height,finalized_hash,confirmed_at)
      SELECT w.branch_id,w.device_ref,w.generation,w.actor_id,w.request_key,w.review_ref,w.tx_hash,withdrawal_hash_,w.signer,w.nonce,
        r.status,r.evidence_hash,r.finalized_height,r.finalized_hash,r.confirmed_at
      FROM ledger.rental_withdrawal_candidate w JOIN ledger.rental_withdrawal_resolution r USING(branch_id,device_ref,generation)
      WHERE (w.branch_id,w.device_ref,w.generation,w.tx_hash)=(branch,device,generation_,prior_)
        AND w.sealed_transaction IS NULL AND r.original_hash=prior_ AND r.evidence_hash=evidence_
        AND r.status IN ('reverted','superseded') AND (w.signer,w.nonce)<>(signer_,nonce_);
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='original withdrawal remains unresolved';
    END IF;
    DELETE FROM ledger.rental_withdrawal_resolution r
        WHERE (r.branch_id,r.device_ref,r.generation,r.original_hash)=(branch,device,generation_,prior_);
    DELETE FROM ledger.rental_withdrawal_candidate w
        WHERE (w.branch_id,w.device_ref,w.generation,w.tx_hash)=(branch,device,generation_,prior_);
    IF NOT ledger.rental_protected_withdrawal_admit(tenant,branch,actor,device,generation_,key_,review_,close_hash_,
        withdrawal_hash_,writer_,scope_,lease_,signer_,nonce_,sealed_) THEN
        RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='protected withdrawal retry admission changed';
    END IF;
    RETURN true;
END $$;

REVOKE ALL ON FUNCTION ledger.rental_withdrawal_archived(uuid,uuid,uuid,uuid,bigint,uuid,text),
    ledger.rental_protected_withdrawal_retry(uuid,uuid,uuid,uuid,bigint,bytea,bytea,uuid,text,bytea,bytea,bytea,text,bigint,text,numeric,bytea)
    FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.rental_withdrawal_archived(uuid,uuid,uuid,uuid,bigint,uuid,text),
    ledger.rental_protected_withdrawal_retry(uuid,uuid,uuid,uuid,bigint,bytea,bytea,uuid,text,bytea,bytea,bytea,text,bigint,text,numeric,bytea)
    TO nu54_p05_rental_close;

CREATE OR REPLACE FUNCTION ledger.rental_protected_signer_nonce()
RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(hashtextextended(NEW.signer||':'||NEW.nonce::text,79));
    IF EXISTS(SELECT FROM ledger.rental_withdrawal_attempt_archive a WHERE (a.signer,a.nonce)=(NEW.signer,NEW.nonce))
        OR (TG_TABLE_NAME='rental_withdrawal_candidate' AND EXISTS(
            SELECT FROM ledger.rental_withdrawal_attempt_archive a WHERE (a.branch_id,a.actor_id,a.request_key)=(NEW.branch_id,NEW.actor_id,NEW.request_key)))
        OR (TG_TABLE_NAME='rental_close_candidate'  AND EXISTS(
            SELECT FROM ledger.rental_withdrawal_candidate w WHERE (w.signer,w.nonce)=(NEW.signer,NEW.nonce)))
        OR (TG_TABLE_NAME='rental_withdrawal_candidate' AND EXISTS(
            SELECT FROM ledger.rental_close_candidate c WHERE (c.signer,c.nonce)=(NEW.signer,NEW.nonce)
                  AND c.state IN ('prepared','attempting','submitted'))) THEN
        RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='protected rental signer nonce already reserved';
    END IF;
    RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION ledger.rental_withdrawal_resolution_view(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE result ledger.rental_withdrawal_resolution;
BEGIN
    PERFORM ledger.rental_require_live_grant(tenant,branch,actor);
    SELECT * INTO result FROM ledger.rental_withdrawal_resolution r
       WHERE (r.branch_id,r.device_ref,r.generation)=(branch,device,generation_);
    IF NOT FOUND THEN RETURN '{}'::jsonb; END IF;
    RETURN jsonb_build_object('state',result.status,'originalHash','0x'||encode(result.original_hash,'hex'),
        'evidenceHash','0x'||encode(result.evidence_hash,'hex'),'confirmedAt',result.confirmed_at,
        'finalizedHeight',result.finalized_height,'finalizedHash','0x'||encode(result.finalized_hash,'hex'));
END $$;

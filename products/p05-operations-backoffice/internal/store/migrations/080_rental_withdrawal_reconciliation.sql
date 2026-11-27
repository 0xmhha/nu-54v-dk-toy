DO $role$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p05_rental_reconcile') THEN
        CREATE ROLE nu54_p05_rental_reconcile NOLOGIN;
    END IF;
END
$role$;
GRANT USAGE ON SCHEMA ledger TO nu54_p05_rental_reconcile;

ALTER TABLE ledger.rental_withdrawal_candidate ADD COLUMN reconcile_after timestamptz
    NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(reconcile_after));
CREATE INDEX rental_withdrawal_reconcile_queue ON ledger.rental_withdrawal_candidate(reconcile_after,recorded_at)
    WHERE sealed_transaction IS NOT NULL;

CREATE TABLE ledger.rental_withdrawal_resolution (
    branch_id uuid NOT NULL,
    device_ref uuid NOT NULL,
    generation bigint NOT NULL,
    original_hash bytea NOT NULL UNIQUE CHECK(octet_length(original_hash)=32),
    status text NOT NULL CHECK(status IN ('reverted','superseded','executed')),
    evidence_hash bytea NOT NULL CHECK(octet_length(evidence_hash)=32),
    finalized_height bigint NOT NULL CHECK(finalized_height>0),
    finalized_hash bytea NOT NULL CHECK(octet_length(finalized_hash)=32),
    confirmed_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(confirmed_at)),
    PRIMARY KEY(branch_id,device_ref,generation),
    FOREIGN KEY(branch_id,device_ref,generation) REFERENCES ledger.rental_withdrawal_candidate,
    CHECK(status<>'reverted' OR evidence_hash=original_hash),
    CHECK(status<>'superseded' OR evidence_hash<>original_hash)
);
REVOKE ALL ON ledger.rental_withdrawal_resolution FROM PUBLIC,nu54_p05_commerce,nu54_p05_rental_close,nu54_p05_rental_reconcile;

CREATE FUNCTION ledger.rental_withdrawal_reconcile_next(deployment_ bytea,settlement_ text)
RETURNS TABLE(branch_id uuid,device_ref uuid,generation bigint,original_hash bytea,signer text,nonce numeric,
    account_ref text,destination_ref text,close_hash bytea)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    WITH ready AS (
        SELECT w.branch_id,w.device_ref,w.generation,c.account_ref,s.withdraw_ref,c.tx_hash AS close_hash
          FROM ledger.rental_withdrawal_candidate w
          JOIN ledger.rental_close_candidate c USING(branch_id,device_ref,generation)
          JOIN ledger.rental_generation_source s USING(branch_id,device_ref,generation)
         WHERE w.sealed_transaction IS NOT NULL AND w.reconcile_after<=clock_timestamp() AND c.state='confirmed'
           AND s.deployment_digest=deployment_ AND s.settlement_ref=settlement_ AND s.account_ref=c.account_ref
           AND NOT EXISTS (SELECT FROM ledger.rental_withdrawal_resolution r
                           WHERE (r.branch_id,r.device_ref,r.generation)=(w.branch_id,w.device_ref,w.generation))
         ORDER BY w.reconcile_after,w.recorded_at,w.branch_id,w.device_ref,w.generation
         LIMIT 8 FOR UPDATE OF w SKIP LOCKED
    )
    UPDATE ledger.rental_withdrawal_candidate w SET reconcile_after=clock_timestamp()+interval '20 seconds'
      FROM ready
     WHERE (w.branch_id,w.device_ref,w.generation)=(ready.branch_id,ready.device_ref,ready.generation)
    RETURNING w.branch_id,w.device_ref,w.generation,w.tx_hash,w.signer,w.nonce,ready.account_ref,ready.withdraw_ref,ready.close_hash
$$;

CREATE FUNCTION ledger.rental_withdrawal_reconcile_finish(branch_ uuid,device_ uuid,generation_ bigint,original_ bytea,
    state_ text,evidence_ bytea,height_ bigint,block_hash_ bytea,deployment_ bytea,settlement_ text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.lock_rental_evidence_device(branch_,device_);
    PERFORM pg_advisory_xact_lock(hashtextextended(branch_::text||device_::text||generation_::text,25));
    IF NOT EXISTS (
        SELECT FROM ledger.rental_withdrawal_candidate w
          JOIN ledger.rental_generation_source s USING(branch_id,device_ref,generation)
          JOIN ledger.rental_close_candidate c USING(branch_id,device_ref,generation)
         WHERE (w.branch_id,w.device_ref,w.generation,w.tx_hash)=(branch_,device_,generation_,original_)
           AND s.deployment_digest=deployment_ AND s.settlement_ref=settlement_
           AND s.account_ref=c.account_ref AND c.state='confirmed') THEN
       RETURN false;
    END IF;
    INSERT INTO ledger.rental_withdrawal_resolution(branch_id,device_ref,generation,original_hash,status,evidence_hash,finalized_height,finalized_hash)
       VALUES(branch_,device_,generation_,original_,state_,evidence_,height_,block_hash_) ON CONFLICT DO NOTHING;
    IF NOT FOUND THEN RETURN false; END IF;
    UPDATE ledger.rental_withdrawal_candidate w SET sealed_transaction=NULL,attempt_until=NULL
       WHERE (w.branch_id,w.device_ref,w.generation,w.tx_hash)=(branch_,device_,generation_,original_);
    RETURN true;
END $$;

CREATE FUNCTION ledger.rental_withdrawal_resolution_view(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE result ledger.rental_withdrawal_resolution;
BEGIN
    PERFORM ledger.rental_require_live_grant(tenant,branch,actor);
    SELECT * INTO result FROM ledger.rental_withdrawal_resolution r
       WHERE (r.branch_id,r.device_ref,r.generation)=(branch,device,generation_);
    IF NOT FOUND THEN RETURN '{}'::jsonb; END IF;
    RETURN jsonb_build_object('state',result.status,'originalHash','0x'||encode(result.original_hash,'hex'),
        'evidenceHash','0x'||encode(result.evidence_hash,'hex'),'confirmedAt',result.confirmed_at);
END $$;

REVOKE ALL ON FUNCTION ledger.rental_withdrawal_reconcile_next(bytea,text),
    ledger.rental_withdrawal_reconcile_finish(uuid,uuid,bigint,bytea,text,bytea,bigint,bytea,bytea,text),
    ledger.rental_withdrawal_resolution_view(uuid,uuid,uuid,uuid,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.rental_withdrawal_reconcile_next(bytea,text),
    ledger.rental_withdrawal_reconcile_finish(uuid,uuid,bigint,bytea,text,bytea,bigint,bytea,bytea,text) TO nu54_p05_rental_reconcile;
GRANT EXECUTE ON FUNCTION ledger.rental_withdrawal_resolution_view(uuid,uuid,uuid,uuid,bigint) TO nu54_p05_rental_close;

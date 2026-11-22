ALTER TABLE ledger.rental_withdrawal_resolution
    DROP CONSTRAINT rental_withdrawal_resolution_status_check,
    ADD CONSTRAINT rental_withdrawal_resolution_status_check
        CHECK(status IN ('reverted','superseded','executed','settled_other'));
UPDATE ledger.rental_withdrawal_resolution r SET status='settled_other'
    WHERE r.status='executed' AND r.evidence_hash<>r.original_hash;
ALTER TABLE ledger.rental_withdrawal_resolution
    ADD CONSTRAINT rental_withdrawal_resolution_execution_identity
        CHECK((status<>'executed' OR evidence_hash=original_hash)
              AND (status<>'settled_other' OR evidence_hash<>original_hash));

CREATE OR REPLACE FUNCTION ledger.rental_withdrawal_reconcile_next(deployment_ bytea,settlement_ text)
RETURNS TABLE(branch_id uuid,device_ref uuid,generation bigint,original_hash bytea,signer text,nonce numeric,
    account_ref text,destination_ref text,close_hash bytea)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    WITH ready AS (
        SELECT w.branch_id,w.device_ref,w.generation,c.account_ref,s.withdraw_ref,c.tx_hash AS close_hash
          FROM ledger.rental_withdrawal_candidate w
          JOIN ledger.rental_close_candidate c USING(branch_id,device_ref,generation)
          JOIN ledger.rental_generation_source s USING(branch_id,device_ref,generation)
          LEFT JOIN ledger.rental_withdrawal_resolution r USING(branch_id,device_ref,generation)
         WHERE (w.sealed_transaction IS NOT NULL OR r.status='settled_other')
           AND w.reconcile_after<=clock_timestamp() AND c.state='confirmed'
           AND s.deployment_digest=deployment_ AND s.settlement_ref=settlement_ AND s.account_ref=c.account_ref
           AND (r.status IS NULL OR r.status='settled_other')
         ORDER BY w.reconcile_after,w.recorded_at,w.branch_id,w.device_ref,w.generation
         LIMIT 8 FOR UPDATE OF w SKIP LOCKED
    )
    UPDATE ledger.rental_withdrawal_candidate w SET reconcile_after=clock_timestamp()+interval '20 seconds'
      FROM ready
     WHERE (w.branch_id,w.device_ref,w.generation)=(ready.branch_id,ready.device_ref,ready.generation)
    RETURNING w.branch_id,w.device_ref,w.generation,w.tx_hash,w.signer,w.nonce,ready.account_ref,ready.withdraw_ref,ready.close_hash
$$;

CREATE OR REPLACE FUNCTION ledger.rental_withdrawal_reconcile_finish(branch_ uuid,device_ uuid,generation_ bigint,original_ bytea,
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
    INSERT INTO ledger.rental_withdrawal_resolution AS r(branch_id,device_ref,generation,original_hash,status,evidence_hash,finalized_height,finalized_hash)
       VALUES(branch_,device_,generation_,original_,state_,evidence_,height_,block_hash_)
       ON CONFLICT (branch_id,device_ref,generation) DO UPDATE SET status=excluded.status,
          evidence_hash=excluded.evidence_hash,finalized_height=excluded.finalized_height,finalized_hash=excluded.finalized_hash,
          confirmed_at=clock_timestamp()
       WHERE r.original_hash=excluded.original_hash AND r.status='settled_other'
         AND excluded.status IN ('reverted','superseded') AND excluded.finalized_height>=r.finalized_height;
    IF NOT FOUND THEN RETURN false; END IF;
    UPDATE ledger.rental_withdrawal_candidate w SET sealed_transaction=NULL,attempt_until=NULL
       WHERE (w.branch_id,w.device_ref,w.generation,w.tx_hash)=(branch_,device_,generation_,original_);
    RETURN true;
END $$;


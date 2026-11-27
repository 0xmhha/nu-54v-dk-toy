CREATE INDEX settlement_work_due ON ledger.settlement_work(retry_at,id) WHERE NOT finished;

CREATE OR REPLACE FUNCTION ledger.claim_settlement_work()
RETURNS TABLE(work_id uuid, fence bigint, branch_id uuid, sale_id uuid, refund_id uuid, refund_key bytea,
    refund_amount text, quote_cipher bytea, quote_digest bytea, merchant_signer bytea, approval_cipher bytea,
    approval_digest bytea, stream_id uuid, relay_nonce text, relay_expiry timestamptz,
    relay_intent bytea, relay_hash text, relay_cipher bytea, relay_state text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $claim$
DECLARE
    chosen ledger.settlement_work;
BEGIN
    SELECT w.* INTO chosen FROM ledger.settlement_work w
    LEFT JOIN ledger.relay_attempt a ON a.work_id=w.id
    WHERE NOT w.finished AND w.retry_at<=clock_timestamp() AND (w.lease_until IS NULL OR w.lease_until<=clock_timestamp())
    -- A signed head cannot be replaced by reserving a higher nonce.
    -- Retry backoff and age still order reconciliation and new work normally.
    ORDER BY CASE WHEN a.state IN ('reserved','signed') THEN 0 ELSE 1 END,w.retry_at,w.id
    LIMIT 1 FOR UPDATE OF w SKIP LOCKED;
    IF NOT FOUND THEN RETURN; END IF;
    UPDATE ledger.settlement_work w SET fence=w.fence+1,lease_until=clock_timestamp()+interval '25 seconds'
      WHERE w.id=chosen.id RETURNING w.* INTO chosen;
    RETURN QUERY
    SELECT chosen.id,chosen.fence,s.branch_id,s.id,r.id,r.chain_refund_id,r.amount::text,q.payload_cipher,
        q.signed_digest,q.merchant_signer,p.approval_cipher,p.approval_digest,
        COALESCE(p.stream_id,paid.stream_id),relay.nonce::text,relay.expires_at,
        relay.intent_digest,relay.tx_hash,relay.tx_cipher,relay.state
    FROM ledger.sale s JOIN ledger.quote_work q ON q.sale_id=s.id
    LEFT JOIN ledger.refund r ON r.id=chosen.refund_id
    LEFT JOIN ledger.settlement_submission p ON p.sale_id=s.id
    LEFT JOIN ledger.payment paid ON paid.sale_id=s.id
    LEFT JOIN ledger.relay_attempt relay ON relay.work_id=chosen.id
    WHERE s.id=chosen.sale_id;
END
$claim$;

CREATE OR REPLACE FUNCTION ledger.defer_settlement(work_ uuid,fence_ bigint,state_ text,finished_ boolean) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $resume$
DECLARE
    w ledger.settlement_work := ledger.valid_settlement_lease(work_,fence_);
BEGIN
    IF state_='prepared' THEN
        IF NOT EXISTS (SELECT FROM ledger.relay_attempt WHERE work_id=work_ AND state='signed' AND tx_hash IS NOT NULL) THEN
            RAISE EXCEPTION 'signed handoff missing' USING ERRCODE='P1002';
        END IF;
        UPDATE ledger.settlement_work SET retry_at=clock_timestamp(),lease_until=NULL WHERE id=work_;
        RETURN;
    END IF;
    IF state_ IS NOT NULL THEN
        IF state_ NOT IN ('unknown','broadcast','observed','reverted','final') THEN
            RAISE EXCEPTION 'unsupported relay state' USING ERRCODE='P1001';
        END IF;
        UPDATE ledger.relay_attempt SET state=state_ WHERE work_id=work_ AND tx_hash IS NOT NULL AND state<>'final';
    END IF;
    -- Expired leases are recoverable. Preserve signed bytes and the recorded nonce across retries.
    UPDATE ledger.settlement_work SET finished=finished_,retry_at=clock_timestamp()+interval '2 seconds',lease_until=NULL
      WHERE id=work_;
END
$resume$;

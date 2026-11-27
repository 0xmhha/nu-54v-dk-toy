CREATE TABLE ledger.settlement_work (
    id uuid PRIMARY KEY,
    sale_id uuid NOT NULL REFERENCES ledger.quote_work,
    refund_id uuid UNIQUE REFERENCES ledger.refund,
    fence bigint NOT NULL DEFAULT 0,
    retry_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    lease_until timestamptz,
    finished boolean NOT NULL DEFAULT false
);
CREATE UNIQUE INDEX work_one_payment ON ledger.settlement_work(sale_id) WHERE refund_id IS NULL;

CREATE FUNCTION ledger.enqueue_payment_work() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $queue$
BEGIN
    INSERT INTO ledger.settlement_work(id,sale_id) VALUES(NEW.sale_id,NEW.sale_id) ON CONFLICT DO NOTHING;
    RETURN NEW;
END
$queue$;
CREATE TRIGGER queue_payment_work AFTER INSERT ON ledger.quote_outbox
    FOR EACH ROW EXECUTE FUNCTION ledger.enqueue_payment_work();
INSERT INTO ledger.settlement_work(id,sale_id)
    SELECT q.sale_id,q.sale_id FROM ledger.quote_outbox q JOIN ledger.sale s ON s.id=q.sale_id
    WHERE s.state IN ('issued','checking') ON CONFLICT DO NOTHING;

CREATE FUNCTION ledger.enqueue_refund_work() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $queue$
BEGIN
    INSERT INTO ledger.settlement_work(id,sale_id,refund_id) VALUES(NEW.id,NEW.sale_id,NEW.id) ON CONFLICT DO NOTHING;
    RETURN NEW;
END
$queue$;
CREATE TRIGGER queue_refund_work AFTER UPDATE OF state ON ledger.refund
    FOR EACH ROW WHEN (OLD.state='requested' AND NEW.state='approved')
    EXECUTE FUNCTION ledger.enqueue_refund_work();

CREATE TABLE ledger.relay_attempt (
    work_id uuid PRIMARY KEY REFERENCES ledger.settlement_work,
    chain_id integer NOT NULL CHECK (chain_id=8283),
    sender text NOT NULL CHECK (sender ~ '^0x[0-9a-f]{40}$'),
    nonce numeric NOT NULL CHECK (nonce>=0 AND nonce=trunc(nonce) AND nonce<power(2::numeric,64)),
    intent_digest bytea NOT NULL CHECK (octet_length(intent_digest)=32),
    expires_at timestamptz NOT NULL CHECK (isfinite(expires_at)),
    tx_hash text UNIQUE CHECK (tx_hash ~ '^0x[0-9a-f]{64}$'),
    tx_cipher bytea CHECK (octet_length(tx_cipher) BETWEEN 29 AND 4096),
    state text NOT NULL DEFAULT 'reserved' CHECK (state IN ('reserved','signed','unknown','broadcast','observed','reverted','final')),
    CHECK ((tx_hash IS NULL)=(tx_cipher IS NULL)),
    CHECK ((state='reserved')=(tx_hash IS NULL)),
    UNIQUE(chain_id,sender,nonce)
);

CREATE FUNCTION ledger.claim_settlement_work()
RETURNS TABLE(work_id uuid, fence bigint, branch_id uuid, sale_id uuid, refund_id uuid, refund_key bytea,
    refund_amount text, quote_cipher bytea, quote_digest bytea, merchant_signer bytea, approval_cipher bytea,
    approval_digest bytea, stream_id uuid, relay_nonce text, relay_expiry timestamptz,
    relay_intent bytea, relay_hash text, relay_cipher bytea, relay_state text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $claim$
DECLARE
    chosen ledger.settlement_work;
BEGIN
    SELECT w.* INTO chosen FROM ledger.settlement_work w
    WHERE NOT w.finished AND w.retry_at<=clock_timestamp() AND (w.lease_until IS NULL OR w.lease_until<=clock_timestamp())
    ORDER BY w.retry_at,w.id LIMIT 1 FOR UPDATE SKIP LOCKED;
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

CREATE FUNCTION ledger.valid_settlement_lease(work_ uuid,fence_ bigint) RETURNS ledger.settlement_work
LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $lease$
DECLARE
    w ledger.settlement_work;
BEGIN
    SELECT * INTO w FROM ledger.settlement_work WHERE id=work_ FOR UPDATE;
    IF NOT FOUND OR w.finished OR w.fence<>fence_ OR w.lease_until IS NULL OR w.lease_until<=clock_timestamp() THEN
        RAISE EXCEPTION 'settlement lease expired' USING ERRCODE='P1002';
    END IF;
    RETURN w;
END
$lease$;

CREATE FUNCTION ledger.reserve_relay(work_ uuid,fence_ bigint,sender_ text,pending_nonce numeric,intent_ bytea,expiry_ timestamptz)
RETURNS text LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $reserve$
DECLARE
    w ledger.settlement_work := ledger.valid_settlement_lease(work_,fence_);
    old ledger.relay_attempt;
BEGIN
    IF pending_nonce IS NULL OR pending_nonce<0 OR pending_nonce<>trunc(pending_nonce) OR
        sender_!~'^0x[0-9a-f]{40}$' OR expiry_<=clock_timestamp() THEN
        RAISE EXCEPTION 'invalid relay nonce or lifetime' USING ERRCODE='P1002';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended('8283:'||sender_,10));
    SELECT * INTO old FROM ledger.relay_attempt WHERE work_id=work_;
    IF FOUND THEN
        IF (old.sender,old.intent_digest,old.expires_at) IS DISTINCT FROM (sender_,intent_,expiry_) THEN
            RAISE EXCEPTION 'relay intent changed' USING ERRCODE='P1001';
        END IF;
        RETURN old.nonce::text;
    END IF;
    DELETE FROM ledger.relay_attempt stalled
    WHERE stalled.sender=sender_ AND stalled.chain_id=8283 AND stalled.tx_hash IS NULL
      AND stalled.expires_at<=clock_timestamp()
      AND NOT EXISTS (SELECT FROM ledger.relay_attempt later WHERE later.sender=stalled.sender
            AND later.chain_id=stalled.chain_id AND later.nonce>stalled.nonce);
    IF EXISTS (SELECT FROM ledger.relay_attempt WHERE sender=sender_ AND chain_id=8283 AND nonce>=pending_nonce) THEN
        RAISE EXCEPTION 'earlier signed or reserved transaction is not visible on chain' USING ERRCODE='P1004';
    END IF;
    INSERT INTO ledger.relay_attempt(work_id,chain_id,sender,nonce,intent_digest,expires_at)
        VALUES(work_,8283,sender_,pending_nonce,intent_,expiry_);
    RETURN pending_nonce::text;
END
$reserve$;

CREATE FUNCTION ledger.store_signed_relay(work_ uuid,fence_ bigint,intent_ bytea,hash_ text,cipher_ bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $signed$
DECLARE
    w ledger.settlement_work := ledger.valid_settlement_lease(work_,fence_);
    relay ledger.relay_attempt;
BEGIN
    SELECT * INTO STRICT relay FROM ledger.relay_attempt WHERE work_id=work_ FOR UPDATE;
    IF relay.intent_digest IS DISTINCT FROM intent_ OR (relay.tx_hash IS NOT NULL AND relay.tx_hash IS DISTINCT FROM hash_) THEN
        RAISE EXCEPTION 'signed transaction differs from relay intent' USING ERRCODE='P1001';
    END IF;
    IF relay.tx_hash IS NOT NULL THEN RETURN false; END IF;
    IF w.refund_id IS NOT NULL THEN PERFORM ledger.mark_refund_inflight(w.refund_id); END IF;
    UPDATE ledger.relay_attempt SET tx_hash=hash_,tx_cipher=cipher_,state='signed' WHERE work_id=work_;
    RETURN true;
END
$signed$;

CREATE FUNCTION ledger.defer_settlement(work_ uuid,fence_ bigint,state_ text,finished_ boolean) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $resume$
DECLARE
    w ledger.settlement_work := ledger.valid_settlement_lease(work_,fence_);
BEGIN
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

CREATE FUNCTION ledger.claim_fulfillment()
RETURNS TABLE(event_id uuid, branch_id uuid, sale_id uuid, kind text, source_ref uuid, revision bigint,
    effect_key uuid, pickup_no bigint, receipt_ref uuid, fence bigint, lookup_only boolean, stream_id uuid, merchant_key text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $claim$
DECLARE
    chosen ledger.fulfillment_outbox;
    check_remote boolean;
BEGIN
    SELECT row.* INTO chosen FROM ledger.fulfillment_outbox row
      WHERE row.state<>'delivered' AND (row.lease_until IS NULL OR row.lease_until<=clock_timestamp())
      ORDER BY COALESCE(row.lease_until,row.created_at),row.event_id LIMIT 1 FOR UPDATE SKIP LOCKED;
    IF NOT FOUND THEN RETURN; END IF;
    check_remote:=chosen.state<>'pending';
    UPDATE ledger.fulfillment_outbox row SET state='sending',fence=row.fence+1,
      lease_until=clock_timestamp()+interval '20 seconds' WHERE row.event_id=chosen.event_id RETURNING row.* INTO chosen;
    RETURN QUERY SELECT chosen.event_id,p.branch_id,chosen.sale_id,chosen.kind,chosen.source_ref,chosen.revision,
        chosen.effect_key,CASE WHEN chosen.kind='paid' THEN p.number END,
        CASE WHEN chosen.kind='paid' THEN p.receipt_ref END,chosen.fence,check_remote,paid.stream_id,paid.merchant_key
    FROM ledger.pickup p JOIN ledger.payment paid ON paid.sale_id=p.sale_id WHERE p.sale_id=chosen.sale_id;
END
$claim$;

CREATE FUNCTION ledger.finish_fulfillment(event_ uuid,fence_ bigint,effect_ uuid,result_ text,payload_ bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $ack$
DECLARE
    message ledger.fulfillment_outbox;
    inbox ledger.fulfillment_inbox;
BEGIN
    SELECT * INTO STRICT message FROM ledger.fulfillment_outbox WHERE event_id=event_ FOR UPDATE;
    IF message.effect_key<>effect_ OR message.fence<>fence_ THEN
        RAISE EXCEPTION 'delivery identity or lease changed' USING ERRCODE='P1002';
    END IF;
    IF message.state='delivered' THEN
        SELECT * INTO STRICT inbox FROM ledger.fulfillment_inbox WHERE event_id=event_;
        IF inbox.payload_digest IS DISTINCT FROM payload_ THEN
            RAISE EXCEPTION 'delivery idempotency conflict' USING ERRCODE='P1001';
        END IF;
        RETURN false;
    END IF;
    IF message.state<>'sending' OR message.lease_until<=clock_timestamp() THEN
        RAISE EXCEPTION 'delivery lease expired' USING ERRCODE='P1002';
    END IF;
    IF result_='delivered' THEN
        INSERT INTO ledger.fulfillment_inbox(event_id,effect_key,payload_digest) VALUES(event_,effect_,payload_);
        UPDATE ledger.fulfillment_outbox SET state='delivered',delivered_at=clock_timestamp(),lease_until=NULL WHERE event_id=event_;
    ELSIF result_ IN ('absent','unknown') THEN
        UPDATE ledger.fulfillment_outbox SET state=CASE WHEN result_='absent' THEN 'pending' ELSE 'unknown' END,
            lease_until=clock_timestamp()+interval '2 seconds' WHERE event_id=event_;
    ELSE RAISE EXCEPTION 'unknown delivery outcome' USING ERRCODE='P1001';
    END IF;
    RETURN true;
END
$ack$;

REVOKE ALL ON FUNCTION ledger.enqueue_payment_work(),ledger.enqueue_refund_work(),
    ledger.valid_settlement_lease(uuid,bigint),ledger.claim_settlement_work(),
    ledger.reserve_relay(uuid,bigint,text,numeric,bytea,timestamptz),
    ledger.store_signed_relay(uuid,bigint,bytea,text,bytea),
    ledger.defer_settlement(uuid,bigint,text,boolean),ledger.claim_fulfillment(),
    ledger.finish_fulfillment(uuid,bigint,uuid,text,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.claim_settlement_work(),
    ledger.reserve_relay(uuid,bigint,text,numeric,bytea,timestamptz),
    ledger.store_signed_relay(uuid,bigint,bytea,text,bytea),
    ledger.defer_settlement(uuid,bigint,text,boolean)
    TO nu54_p05_settlement;
GRANT EXECUTE ON FUNCTION ledger.claim_fulfillment(),ledger.finish_fulfillment(uuid,bigint,uuid,text,bytea)
    TO nu54_p05_dispatch;

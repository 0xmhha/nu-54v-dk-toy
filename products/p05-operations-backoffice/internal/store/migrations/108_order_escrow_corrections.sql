-- Corrections after the third audit of the order escrow.

-- A block that was not final can disappear. Its transaction is then sent or replaced like any other.
CREATE OR REPLACE FUNCTION ledger.authorize_relay_send(work_ uuid,fence_ bigint,hash_ text,epoch_ text)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $authorize$
BEGIN
    PERFORM ledger.valid_settlement_lease(work_,fence_);
    UPDATE ledger.relay_attempt SET broadcast_epoch=epoch_,broadcast_fence=fence_,state='unknown'
    WHERE work_id=work_ AND tx_hash=hash_ AND state IN ('signed','unknown','broadcast','observed','reverted')
      AND expires_at>clock_timestamp() AND (broadcast_fence IS NULL OR broadcast_fence<fence_);
    IF NOT FOUND THEN RAISE EXCEPTION 'relay attempt no longer authorized' USING ERRCODE='P1002'; END IF;
END
$authorize$;

CREATE OR REPLACE FUNCTION ledger.replace_signed_relay(work_ uuid,fence_ bigint,old_ text,hash_ text,cipher_ bytea)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $replace$
BEGIN
    PERFORM ledger.valid_settlement_lease(work_,fence_);
    INSERT INTO ledger.relay_version(work_id,tx_hash,tx_cipher,sent)
        SELECT r.work_id,r.tx_hash,r.tx_cipher,r.broadcast_fence IS NOT NULL FROM ledger.relay_attempt r
        WHERE r.work_id=work_ AND r.tx_hash=old_ AND r.state IN ('signed','unknown','broadcast','observed','reverted') AND r.expires_at>clock_timestamp() FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION 'relay attempt changed' USING ERRCODE='P1002'; END IF;
    UPDATE ledger.relay_attempt SET tx_hash=hash_,tx_cipher=cipher_,state='signed',broadcast_epoch=NULL,broadcast_fence=NULL WHERE work_id=work_;
END
$replace$;

-- A chain that started over gives out old nonces again; finished work keeps its record beside the new one.
ALTER TABLE ledger.relay_attempt DROP CONSTRAINT relay_attempt_chain_id_sender_nonce_key;
CREATE UNIQUE INDEX relay_attempt_open_nonce ON ledger.relay_attempt(chain_id,sender,nonce) WHERE state<>'final';
CREATE OR REPLACE FUNCTION ledger.reserve_relay(work_ uuid,fence_ bigint,sender_ text,pending_nonce numeric,intent_ bytea,expiry_ timestamptz)
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
    -- A nonce is free again only when no stored bytes can still take effect: nothing was signed, or
    -- the signed authorization expired, the chain has not used the nonce, and the order is already
    -- decided or the expiry passed long enough ago for any difference between the clocks.
    DELETE FROM ledger.relay_attempt stalled USING ledger.settlement_work job
    WHERE job.id=stalled.work_id AND stalled.sender=sender_ AND stalled.chain_id=8283 AND (
        (stalled.tx_hash IS NULL AND (stalled.expires_at<=clock_timestamp()
            OR (job.action_id IS NOT NULL AND (job.lease_until IS NULL OR job.lease_until<=clock_timestamp()))))
        OR (stalled.tx_hash IS NOT NULL AND stalled.nonce>=pending_nonce
            AND stalled.expires_at<=clock_timestamp()
            AND ((job.finished AND job.refund_id IS NULL) OR stalled.expires_at<=clock_timestamp()-interval '5 minutes')
            AND NOT EXISTS (SELECT FROM ledger.relay_version v JOIN ledger.payment p ON p.tx_hash=v.tx_hash WHERE v.work_id=stalled.work_id)
            AND NOT EXISTS (SELECT FROM ledger.payment p WHERE p.tx_hash=stalled.tx_hash)
            AND NOT EXISTS (SELECT FROM ledger.refund_confirmation c WHERE c.tx_hash=stalled.tx_hash)))
      -- A later transaction waits for this nonce; freeing it is what lets the next job fill the gap.
      AND (stalled.nonce=pending_nonce OR NOT EXISTS (SELECT FROM ledger.relay_attempt later WHERE later.sender=stalled.sender
            AND later.chain_id=stalled.chain_id AND later.nonce>stalled.nonce));
    -- A finished action's transaction is final and its effect is on record, so it cannot act again.
    IF EXISTS (SELECT FROM ledger.relay_attempt r JOIN ledger.settlement_work job ON job.id=r.work_id
        WHERE r.sender=sender_ AND r.chain_id=8283 AND r.nonce=pending_nonce
          AND NOT (job.action_id IS NOT NULL AND job.finished AND r.state='final')) THEN
        RAISE EXCEPTION 'earlier signed or reserved transaction is not visible on chain' USING ERRCODE='P1004';
    END IF;
    INSERT INTO ledger.relay_attempt(work_id,chain_id,sender,nonce,intent_digest,expires_at)
        VALUES(work_,8283,sender_,pending_nonce,intent_,expiry_);
    RETURN pending_nonce::text;
END
$reserve$;

-- The newest version that was sent. A higher price that is stored and not yet sent does not undo a submission.
CREATE FUNCTION ledger.relay_sent_hash(work_ uuid) RETURNS text
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT coalesce((SELECT r.tx_hash FROM ledger.relay_attempt r WHERE r.work_id=work_ AND r.broadcast_fence IS NOT NULL),
        (SELECT v.tx_hash FROM ledger.relay_version v WHERE v.work_id=work_ AND v.sent ORDER BY v.replaced_at DESC LIMIT 1))
$$;
REVOKE ALL ON FUNCTION ledger.relay_sent_hash(uuid) FROM PUBLIC;

CREATE OR REPLACE FUNCTION ledger.escrow_claim_view(action_ uuid) RETURNS jsonb
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT jsonb_strip_nulls(jsonb_build_object('batchId',a.id,
        'state',CASE WHEN a.state='queued' THEN CASE WHEN ledger.relay_sent_hash(a.id) IS NULL THEN 'queued' ELSE 'submitted' END
            WHEN a.state='final' THEN 'final' ELSE 'failed' END,
        'createdAt',floor(extract(epoch FROM a.created_at))::bigint,'orders',count(*),'requested',sum(p.principal)::text,
        'paid',CASE WHEN a.state='final' THEN coalesce(sum(p.principal) FILTER (WHERE c.outcome='paid'),0)::text END,
        'transaction',ledger.relay_sent_hash(a.id),
        'skipped',coalesce(jsonb_agg(jsonb_strip_nulls(jsonb_build_object('saleId',c.sale_id,'pickupNo',n.number,'reason',c.outcome)) ORDER BY c.position)
            FILTER (WHERE c.outcome<>'paid'),'[]'::jsonb)))
    FROM ledger.escrow_action a JOIN ledger.escrow_claim_order c ON c.action_id=a.id JOIN ledger.payment p ON p.sale_id=c.sale_id
    LEFT JOIN ledger.pickup n ON n.sale_id=c.sale_id
    WHERE a.id=action_ GROUP BY a.id
$$;

CREATE OR REPLACE FUNCTION ledger.escrow_decision_view(decision ledger.escrow_decision,actor uuid,branch uuid) RETURNS jsonb
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT jsonb_strip_nulls(jsonb_build_object('decisionId',(decision).id,'saleId',(decision).sale_id,'outcome',(decision).outcome,
        'reason',(decision).reason,'state',CASE WHEN (decision).state='approved' AND ledger.relay_sent_hash((decision).action_id) IS NOT NULL THEN 'submitted' ELSE (decision).state END,
        'canApprove',(decision).state='proposed' AND (decision).proposer<>actor AND e.due_at>extract(epoch FROM clock_timestamp())
            AND EXISTS (SELECT FROM ledger.finance_grant g WHERE g.branch_id=branch AND g.actor_id=actor
                AND g.capability='escrow_dispute_approve' AND g.valid_until>clock_timestamp()),
        'transaction',coalesce(done.tx_hash,ledger.relay_sent_hash((decision).action_id)),'resolveBy',e.due_at))
    FROM ledger.order_escrow_event e
    LEFT JOIN ledger.order_escrow_event done ON (decision).state='final' AND done.sale_id=(decision).sale_id AND done.kind IN ('released','cancelled')
    WHERE e.sale_id=(decision).sale_id AND e.kind='disputed'
$$;

CREATE OR REPLACE FUNCTION ledger.wallet_return_status(tenant_ uuid,branch_ uuid,actor_ uuid)
RETURNS TABLE(payer bytea,recipient bytea,amount text,valid_before bigint,state text,tx_hash text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $status$
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code<>'customer' THEN RAISE EXCEPTION 'customer scope required' USING ERRCODE='42501'; END IF;
    RETURN QUERY SELECT r.payer,r.recipient,r.amount::text,r.valid_before,
        CASE a.state WHEN 'queued' THEN CASE WHEN sent.hash IS NULL THEN 'queued' ELSE 'submitted' END
            -- Used on chain by a transaction the node no longer shows: done, without a hash to prove it.
            WHEN 'final' THEN CASE WHEN r.tx_hash IS NOT NULL THEN 'final' WHEN sent.hash IS NULL THEN 'queued' ELSE 'submitted' END ELSE 'failed' END,
        CASE WHEN r.tx_hash IS NOT NULL THEN r.tx_hash ELSE sent.hash END
    FROM ledger.escrow_action a JOIN ledger.wallet_return r ON r.action_id=a.id CROSS JOIN LATERAL (SELECT ledger.relay_sent_hash(a.id) AS hash) sent
    WHERE a.branch_id=branch_ AND a.requester=actor_ AND a.kind='return' ORDER BY a.created_at DESC LIMIT 1;
END
$status$;

-- What this customer itself still has open on the account.
CREATE FUNCTION ledger.customer_uses_payer(actor_ uuid,payer_ bytea) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT EXISTS (SELECT FROM ledger.quote_work q JOIN ledger.sale s ON s.id=q.sale_id LEFT JOIN ledger.order_escrow e ON e.sale_id=s.id
        WHERE q.payer=payer_ AND q.actor_id=actor_ AND (s.state='checking' OR (s.state IN ('preview','issued') AND q.expires_at>clock_timestamp())
            OR (e.sale_id IS NOT NULL AND NOT e.settled)))
        OR EXISTS (SELECT FROM ledger.wallet_return r JOIN ledger.escrow_action a ON a.id=r.action_id
            WHERE r.payer=payer_ AND a.requester=actor_ AND a.state='queued')
$$;
REVOKE ALL ON FUNCTION ledger.customer_uses_payer(uuid,bytea) FROM PUBLIC;

CREATE OR REPLACE FUNCTION ledger.enroll_payer(tenant_ uuid,branch_ uuid,actor_ uuid,key_ uuid,intent_ bytea,payer_ bytea,generation_ bigint)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $enroll$
DECLARE grant_ ledger.commerce_grant := ledger.commerce_allowed(tenant_,branch_,actor_); prior ledger.payer_enrollment; request ledger.payer_enrollment_request;
BEGIN
    IF grant_.role_code<>'customer' THEN RAISE EXCEPTION 'customer scope required' USING ERRCODE='42501'; END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch_::text||actor_::text,108));
    SELECT * INTO request FROM ledger.payer_enrollment_request r WHERE (r.branch_id,r.actor_id,r.request_key)=(branch_,actor_,key_);
    IF FOUND THEN
        IF request.intent_digest IS DISTINCT FROM intent_ THEN RAISE EXCEPTION 'enrollment key conflict' USING ERRCODE='P1001'; END IF;
        RETURN;
    END IF;
    SELECT * INTO prior FROM ledger.payer_enrollment e WHERE (e.branch_id,e.actor_id)=(branch_,actor_) FOR UPDATE;
    IF FOUND AND (prior.payer_role<>1 OR ((prior.payer,prior.payer_generation)<>(payer_,generation_)
        AND ledger.customer_uses_payer(actor_,prior.payer))) THEN
        RAISE EXCEPTION 'the current account still has an order' USING ERRCODE='P1060';
    END IF;
    -- The newest proof decides who pays from the account, unless the other customer still has something open on it.
    PERFORM pg_advisory_xact_lock(hashtextextended(encode(payer_,'hex'),107));
    IF EXISTS (SELECT FROM ledger.payer_enrollment e JOIN ledger.branch b ON b.id=e.branch_id
        WHERE b.tenant_ref=tenant_ AND e.payer=payer_ AND e.payer_role=1 AND e.actor_id<>actor_ AND e.expires_at>clock_timestamp()
          AND ledger.customer_uses_payer(e.actor_id,payer_)) THEN
        RAISE EXCEPTION 'another customer still uses the account' USING ERRCODE='P1060';
    END IF;
    UPDATE ledger.payer_enrollment e SET expires_at=clock_timestamp() FROM ledger.branch b
        WHERE b.id=e.branch_id AND b.tenant_ref=tenant_ AND e.payer=payer_ AND e.payer_role=1 AND e.actor_id<>actor_ AND e.expires_at>clock_timestamp();
    INSERT INTO ledger.payer_enrollment_request VALUES(branch_,actor_,key_,intent_,payer_,generation_);
    INSERT INTO ledger.payer_enrollment VALUES(branch_,actor_,payer_,generation_,1,LEAST(grant_.valid_until,clock_timestamp()+interval '30 days'))
        ON CONFLICT(branch_id,actor_id) DO UPDATE SET payer=excluded.payer,payer_generation=excluded.payer_generation,expires_at=excluded.expires_at;
END
$enroll$;

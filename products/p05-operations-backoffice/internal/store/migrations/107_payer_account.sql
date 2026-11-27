-- Higher-priced versions of one relay transaction, a filled nonce gap, and the customer's own account:
-- enrolling the board that pays and sending its balance home.

-- Earlier signed bytes of an attempt. Every version has the same sender, nonce, recipient and call, so
-- at most one is ever included. relay_attempt keeps the newest.
CREATE TABLE ledger.relay_version (
    work_id uuid NOT NULL REFERENCES ledger.relay_attempt ON DELETE CASCADE,
    tx_hash text NOT NULL UNIQUE CHECK (tx_hash ~ '^0x[0-9a-f]{64}$'),
    tx_cipher bytea NOT NULL CHECK (octet_length(tx_cipher) BETWEEN 29 AND 4096),
    sent boolean NOT NULL,
    replaced_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(work_id,tx_hash)
);
REVOKE ALL ON ledger.relay_version FROM PUBLIC;

CREATE FUNCTION ledger.relay_versions(work_ uuid,fence_ bigint) RETURNS text[]
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $versions$
BEGIN
    PERFORM ledger.valid_settlement_lease(work_,fence_);
    RETURN ARRAY(SELECT v.tx_hash FROM ledger.relay_version v WHERE v.work_id=work_ ORDER BY v.replaced_at DESC);
END
$versions$;

-- The higher-priced bytes are stored before they are sent, like the first ones.
CREATE FUNCTION ledger.replace_signed_relay(work_ uuid,fence_ bigint,old_ text,hash_ text,cipher_ bytea)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $replace$
BEGIN
    PERFORM ledger.valid_settlement_lease(work_,fence_);
    INSERT INTO ledger.relay_version(work_id,tx_hash,tx_cipher,sent)
        SELECT r.work_id,r.tx_hash,r.tx_cipher,r.broadcast_fence IS NOT NULL FROM ledger.relay_attempt r
        WHERE r.work_id=work_ AND r.tx_hash=old_ AND r.state IN ('signed','unknown','broadcast') AND r.expires_at>clock_timestamp() FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION 'relay attempt changed' USING ERRCODE='P1002'; END IF;
    UPDATE ledger.relay_attempt SET tx_hash=hash_,tx_cipher=cipher_,state='signed',broadcast_epoch=NULL,broadcast_fence=NULL WHERE work_id=work_;
END
$replace$;

-- An earlier version was included. It becomes the attempt again, so every reader names the real transaction.
CREATE FUNCTION ledger.adopt_relay_version(work_ uuid,hash_ text) RETURNS void
LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $adopt$
DECLARE mined ledger.relay_version; latest ledger.relay_attempt;
BEGIN
    SELECT * INTO latest FROM ledger.relay_attempt WHERE work_id=work_ FOR UPDATE;
    DELETE FROM ledger.relay_version WHERE work_id=work_ AND tx_hash=hash_ RETURNING * INTO mined;
    IF mined.work_id IS NULL THEN RETURN; END IF;
    UPDATE ledger.relay_attempt SET tx_hash=mined.tx_hash,tx_cipher=mined.tx_cipher,state=CASE state WHEN 'signed' THEN 'unknown' ELSE state END,
        broadcast_epoch=coalesce(broadcast_epoch,repeat('0',64)),broadcast_fence=coalesce(broadcast_fence,1) WHERE work_id=work_;
    INSERT INTO ledger.relay_version(work_id,tx_hash,tx_cipher,sent) VALUES(work_,latest.tx_hash,latest.tx_cipher,latest.broadcast_fence IS NOT NULL);
END
$adopt$;

CREATE FUNCTION ledger.adopt_included_relay(work_ uuid,fence_ bigint,hash_ text) RETURNS bytea
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $adopt$
BEGIN
    PERFORM ledger.valid_settlement_lease(work_,fence_);
    PERFORM ledger.adopt_relay_version(work_,hash_);
    RETURN (SELECT r.tx_cipher FROM ledger.relay_attempt r WHERE r.work_id=work_ AND r.tx_hash=hash_);
END
$adopt$;

-- A payment can be confirmed from evidence before the worker looks at its receipt.
CREATE FUNCTION ledger.adopt_paid_relay() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $paid$
BEGIN
    PERFORM ledger.adopt_relay_version(v.work_id,v.tx_hash) FROM ledger.relay_version v WHERE v.tx_hash=NEW.tx_hash;
    RETURN NEW;
END
$paid$;
CREATE TRIGGER adopt_paid_relay AFTER INSERT ON ledger.payment FOR EACH ROW EXECUTE FUNCTION ledger.adopt_paid_relay();

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
    IF EXISTS (SELECT FROM ledger.relay_attempt WHERE sender=sender_ AND chain_id=8283 AND nonce=pending_nonce) THEN
        RAISE EXCEPTION 'earlier signed or reserved transaction is not visible on chain' USING ERRCODE='P1004';
    END IF;
    INSERT INTO ledger.relay_attempt(work_id,chain_id,sender,nonce,intent_digest,expires_at)
        VALUES(work_,8283,sender_,pending_nonce,intent_,expiry_);
    RETURN pending_nonce::text;
END
$reserve$;

-- ---- escrow ----

ALTER TABLE ledger.escrow_action ADD COLUMN refused_at timestamptz;

CREATE FUNCTION ledger.note_escrow_refusal(work_ uuid,fence_ bigint) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $refused$
BEGIN
    PERFORM ledger.valid_settlement_lease(work_,fence_);
    UPDATE ledger.escrow_action SET refused_at=coalesce(refused_at,clock_timestamp()) WHERE id=work_;
END
$refused$;

CREATE OR REPLACE FUNCTION ledger.escrow_order_page(branch uuid,actor uuid,status_ text,cursor_ uuid,sale_ uuid)
RETURNS jsonb LANGUAGE plpgsql STABLE SET search_path=pg_catalog,pg_temp AS $orders$
DECLARE result jsonb;
BEGIN
    WITH orders AS MATERIALIZED (
        SELECT e.sale_id,p.included_at,p.principal,p.asset_ref,pickup.number,e.deliver_by,delivered.due_at AS claimable_at,
            disputed.due_at AS resolve_by,done.kind,done.reason,done.tx_hash,
            CASE WHEN done.kind='released' THEN 'paid' WHEN done.kind='cancelled' THEN 'refunded'
                WHEN disputed.due_at IS NOT NULL THEN 'disputed' WHEN delivered.due_at IS NULL THEN 'held'
                WHEN EXISTS (SELECT FROM ledger.escrow_claim_order c JOIN ledger.escrow_action a ON a.id=c.action_id
                    WHERE c.sale_id=e.sale_id AND a.state='queued' AND c.outcome IS DISTINCT FROM 'in-another-batch') THEN 'claiming'
                WHEN delivered.due_at<=extract(epoch FROM clock_timestamp()) THEN 'claimable' ELSE 'waiting' END AS status
        FROM ledger.order_escrow e JOIN ledger.payment p ON p.sale_id=e.sale_id
        JOIN ledger.sale s ON s.id=e.sale_id AND s.branch_id=branch
        LEFT JOIN ledger.pickup pickup ON pickup.sale_id=e.sale_id
        LEFT JOIN ledger.order_escrow_event delivered ON delivered.sale_id=e.sale_id AND delivered.kind='delivered'
        LEFT JOIN ledger.order_escrow_event disputed ON disputed.sale_id=e.sale_id AND disputed.kind='disputed'
        LEFT JOIN ledger.order_escrow_event done ON done.sale_id=e.sale_id AND done.kind IN ('released','cancelled')
        WHERE sale_ IS NULL OR e.sale_id=sale_
    ), page AS (
        SELECT * FROM orders o WHERE (status_='' OR o.status=status_) AND (cursor_ IS NULL OR (o.included_at,o.sale_id)<
            (SELECT last.included_at,last.sale_id FROM orders last WHERE last.sale_id=cursor_))
        ORDER BY o.included_at DESC,o.sale_id DESC LIMIT 51
    ), shown AS (SELECT * FROM page ORDER BY included_at DESC,sale_id DESC LIMIT 50)
    SELECT jsonb_strip_nulls(jsonb_build_object(
        'items',coalesce((SELECT jsonb_agg(jsonb_strip_nulls(jsonb_build_object('saleId',o.sale_id,'pickupNo',o.number,'amount',o.principal::text,
            'asset',o.asset_ref,'status',o.status,'lockedAt',floor(extract(epoch FROM o.included_at))::bigint,'deliverBy',o.deliver_by,
            'claimableAt',o.claimable_at,'resolveBy',o.resolve_by,
            'payout',CASE WHEN o.kind='released' THEN (ARRAY['receipt','claim','arbiter','dispute-withdrawn'])[o.reason+1] END,
            'refund',CASE WHEN o.kind='cancelled' THEN (ARRAY['store','undelivered','arbiter','dispute-expired'])[o.reason+1] END,
            'transaction',o.tx_hash,
            -- The delivered mark came after the contract's delivery deadline and was never accepted.
            'deliveryFailed',CASE WHEN o.status='held' AND EXISTS (SELECT FROM ledger.escrow_action a
                WHERE a.sale_id=o.sale_id AND a.kind='deliver' AND a.state='failed') THEN true END,
            -- The store's cancel is tried again, but the token keeps refusing to pay this buyer.
            'cancelRefused',CASE WHEN o.kind IS NULL AND EXISTS (SELECT FROM ledger.escrow_action a
                WHERE a.sale_id=o.sale_id AND a.kind='cancel' AND a.state='queued' AND a.refused_at IS NOT NULL) THEN true END,
            'items',CASE WHEN o.status='disputed' THEN (SELECT jsonb_agg(jsonb_build_object(
                'label',x.label,'option',l.option_key,'quantity',l.quantity) ORDER BY l.sku COLLATE "C",l.option_key COLLATE "C")
                FROM ledger.order_line l JOIN ledger.price x ON
                  (x.branch_id,x.version,x.sku,x.option_key)=(l.branch_id,l.menu_version,l.sku,l.option_key)
                WHERE l.sale_id=o.sale_id) END,
            'decision',(SELECT ledger.escrow_decision_view(d,actor,branch) FROM ledger.escrow_decision d
                WHERE d.sale_id=o.sale_id ORDER BY d.created_at DESC LIMIT 1))) ORDER BY o.included_at DESC,o.sale_id DESC) FROM shown o),'[]'::jsonb),
        'totals',(SELECT jsonb_build_object(
            'claimable',coalesce(sum(o.principal) FILTER (WHERE o.status='claimable'),0)::text,
            'waiting',coalesce(sum(o.principal) FILTER (WHERE o.status='waiting'),0)::text,
            'disputed',coalesce(sum(o.principal) FILTER (WHERE o.status='disputed'),0)::text,
            'held',coalesce(sum(o.principal) FILTER (WHERE o.status='held'),0)::text) FROM orders o),
        'nextCursor',CASE WHEN (SELECT count(*) FROM page)>50 THEN
            (SELECT o.sale_id FROM shown o ORDER BY o.included_at,o.sale_id LIMIT 1) END)) INTO result;
    RETURN result;
END
$orders$;

-- An operator with evidence access sees the same item beside the payment evidence.
CREATE FUNCTION ledger.finance_escrow_item(tenant uuid,branch uuid,actor uuid,sale_ uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'reconcile_read');
    RETURN ledger.escrow_order_page(branch,actor,'',NULL,sale_)->'items'->0;
END $$;

-- Refunds that still need the old contract also hold a branch there.
CREATE OR REPLACE FUNCTION ledger.open_payment_contracts(branch_ uuid)
RETURNS TABLE(sale_id uuid,payload_cipher bytea,contract_ref text)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT s.id,q.payload_cipher,NULL::text FROM ledger.sale s JOIN ledger.quote_work q ON q.sale_id=s.id
      WHERE s.branch_id=branch_ AND q.payload_cipher IS NOT NULL AND (s.state IN ('issued','checking')
        OR EXISTS (SELECT FROM ledger.refund r WHERE r.sale_id=s.id AND r.state IN ('requested','approved','inflight')))
    UNION ALL
    SELECT s.id,NULL,e.contract_ref FROM ledger.sale s JOIN ledger.order_escrow e ON e.sale_id=s.id
      WHERE s.branch_id=branch_ AND NOT e.settled
$$;

-- ---- the customer's account ----

-- One row per accepted enrollment request. A repeated request changes nothing, so an old one cannot
-- bring an old account back.
CREATE TABLE ledger.payer_enrollment_request (
    branch_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    request_key uuid NOT NULL,
    intent_digest bytea NOT NULL CHECK (octet_length(intent_digest)=32),
    payer bytea NOT NULL CHECK (octet_length(payer)=20),
    payer_generation bigint NOT NULL CHECK (payer_generation>0),
    accepted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(branch_id,actor_id,request_key),
    FOREIGN KEY(branch_id,actor_id) REFERENCES ledger.commerce_grant ON DELETE CASCADE
);

ALTER TABLE ledger.escrow_action DROP CONSTRAINT escrow_action_kind_check, DROP CONSTRAINT escrow_action_check;
ALTER TABLE ledger.escrow_action
    ADD CONSTRAINT escrow_action_kind_check CHECK (kind IN ('deliver','cancel','receipt','dispute','withdraw','refund-undelivered',
        'refund-dispute','claim','resolve','return')),
    ADD CONSTRAINT escrow_action_check CHECK ((kind IN ('claim','return'))=(sale_id IS NULL));

CREATE TABLE ledger.wallet_return (
    action_id uuid PRIMARY KEY REFERENCES ledger.escrow_action,
    payer bytea NOT NULL CHECK (octet_length(payer)=20),
    recipient bytea NOT NULL CHECK (octet_length(recipient)=20),
    asset_ref text NOT NULL CHECK (asset_ref ~ '^0x[0-9a-f]{40}$'),
    amount numeric NOT NULL CHECK (amount>0 AND amount=trunc(amount) AND amount<power(2::numeric,256)),
    valid_before bigint NOT NULL CHECK (valid_before>0),
    nonce bytea NOT NULL CHECK (octet_length(nonce)=32),
    intent_digest bytea NOT NULL CHECK (octet_length(intent_digest)=32),
    signature_cipher bytea NOT NULL CHECK (octet_length(signature_cipher)=93),
    tx_hash text CHECK (tx_hash ~ '^0x[0-9a-f]{64}$'),
    UNIQUE(payer,nonce)
);
CREATE INDEX wallet_return_payer ON ledger.wallet_return(payer);
CREATE INDEX quote_work_payer ON ledger.quote_work(payer);
REVOKE ALL ON ledger.payer_enrollment_request,ledger.wallet_return FROM PUBLIC;

-- Anything that still may take or return money of this account, in any branch.
CREATE FUNCTION ledger.payer_has_open_order(payer_ bytea) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT EXISTS (SELECT FROM ledger.quote_work q JOIN ledger.sale s ON s.id=q.sale_id LEFT JOIN ledger.order_escrow e ON e.sale_id=s.id
        WHERE q.payer=payer_ AND (s.state='checking' OR (s.state IN ('preview','issued') AND q.expires_at>clock_timestamp())
            OR (e.sale_id IS NOT NULL AND NOT e.settled)))
$$;
CREATE FUNCTION ledger.payer_has_open_return(payer_ bytea) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT EXISTS (SELECT FROM ledger.wallet_return r JOIN ledger.escrow_action a ON a.id=r.action_id WHERE r.payer=payer_ AND a.state='queued')
$$;

-- A return and a new quote for the same account take the same lock, so only one of them is accepted.
CREATE FUNCTION ledger.quote_return_guard() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $guard$
BEGIN
    PERFORM pg_advisory_xact_lock(hashtextextended(encode(NEW.payer,'hex'),107));
    IF ledger.payer_has_open_return(NEW.payer) THEN
        RAISE EXCEPTION 'the account is being emptied' USING ERRCODE='P1060';
    END IF;
    RETURN NEW;
END
$guard$;
CREATE TRIGGER quote_return_guard BEFORE INSERT ON ledger.quote_work FOR EACH ROW EXECUTE FUNCTION ledger.quote_return_guard();

-- A POS order has no kitchen record of its handover, so escrow could never be released for it.
CREATE OR REPLACE FUNCTION ledger.order_escrow_allowed(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid,claim_ bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $allowed$
BEGIN
    PERFORM ledger.commerce_allowed(tenant_,branch_,actor_);
    RETURN NOT EXISTS (SELECT FROM ledger.sale s WHERE s.branch_id=branch_ AND s.id=sale_ AND s.origin<>'menu')
       AND NOT EXISTS (SELECT FROM ledger.travel_pickup_order p WHERE p.sale_id=sale_)
       AND NOT EXISTS (SELECT FROM ledger.checkout_claim cl WHERE cl.branch_id=branch_ AND cl.sale_id=sale_
            AND cl.claim_digest=claim_ AND cl.payer_role<>1);
END
$allowed$;

CREATE FUNCTION ledger.enroll_payer(tenant_ uuid,branch_ uuid,actor_ uuid,key_ uuid,intent_ bytea,payer_ bytea,generation_ bigint)
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
        AND (ledger.payer_has_open_order(prior.payer) OR ledger.payer_has_open_return(prior.payer)))) THEN
        RAISE EXCEPTION 'the current account still has an order' USING ERRCODE='P1060';
    END IF;
    INSERT INTO ledger.payer_enrollment_request VALUES(branch_,actor_,key_,intent_,payer_,generation_);
    INSERT INTO ledger.payer_enrollment VALUES(branch_,actor_,payer_,generation_,1,LEAST(grant_.valid_until,clock_timestamp()+interval '30 days'))
        ON CONFLICT(branch_id,actor_id) DO UPDATE SET payer=excluded.payer,payer_generation=excluded.payer_generation,expires_at=excluded.expires_at;
END
$enroll$;

CREATE FUNCTION ledger.enrolled_payer(tenant_ uuid,branch_ uuid,actor_ uuid)
RETURNS TABLE(payer bytea,generation bigint,expires_at bigint)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $payer$
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code<>'customer' THEN RAISE EXCEPTION 'customer scope required' USING ERRCODE='42501'; END IF;
    RETURN QUERY SELECT e.payer,e.payer_generation,floor(extract(epoch FROM e.expires_at))::bigint FROM ledger.payer_enrollment e
        WHERE (e.branch_id,e.actor_id)=(branch_,actor_) AND e.payer_role=1 AND e.expires_at>clock_timestamp();
END
$payer$;

CREATE FUNCTION ledger.store_wallet_return(tenant_ uuid,branch_ uuid,actor_ uuid,key_ uuid,intent_ bytea,payer_ bytea,recipient_ bytea,
    asset_ text,amount_ numeric,valid_before_ bigint,nonce_ bytea,cipher_ bytea,sponsored_until_ timestamptz)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $return$
DECLARE action_ uuid; enrolled bytea; now_ timestamptz;
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code<>'customer' THEN RAISE EXCEPTION 'customer scope required' USING ERRCODE='42501'; END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(encode(payer_,'hex'),107));
    PERFORM pg_advisory_xact_lock(hashtextextended(actor_::text,109));
    SELECT a.id INTO action_ FROM ledger.escrow_action a WHERE (a.branch_id,a.requester,a.request_key)=(branch_,actor_,key_);
    IF FOUND THEN
        IF NOT EXISTS (SELECT FROM ledger.wallet_return r WHERE r.action_id=action_ AND r.intent_digest=intent_) THEN
            RAISE EXCEPTION 'return request key conflict' USING ERRCODE='P1001';
        END IF;
        RETURN;
    END IF;
    now_:=clock_timestamp();
    SELECT e.payer INTO enrolled FROM ledger.payer_enrollment e
        WHERE (e.branch_id,e.actor_id)=(branch_,actor_) AND e.payer_role=1 AND e.expires_at>now_ FOR SHARE;
    IF enrolled IS DISTINCT FROM payer_ THEN RAISE EXCEPTION 'account is not enrolled' USING ERRCODE='P1061'; END IF;
    IF valid_before_<=extract(epoch FROM now_) THEN RAISE EXCEPTION 'return expired' USING ERRCODE='P1002'; END IF;
    IF valid_before_>extract(epoch FROM now_)+600 THEN RAISE EXCEPTION 'return is valid for too long' USING ERRCODE='P1062'; END IF;
    IF ledger.payer_has_open_order(payer_) THEN RAISE EXCEPTION 'the account still has an order' USING ERRCODE='P1060'; END IF;
    IF ledger.payer_has_open_return(payer_) OR EXISTS (SELECT FROM ledger.wallet_return r WHERE (r.payer,r.nonce)=(payer_,nonce_)) THEN
        RAISE EXCEPTION 'another return is in progress' USING ERRCODE='P1001';
    END IF;
    IF (SELECT count(*) FROM ledger.escrow_action a WHERE a.kind='return' AND a.requester=actor_ AND a.created_at>now_-interval '1 day')>=3 THEN
        RAISE EXCEPTION 'too many returns today' USING ERRCODE='P1004';
    END IF;
    -- The store pays the gas, so it must still agree to.
    IF sponsored_until_ IS NULL OR sponsored_until_<=now_ THEN RAISE EXCEPTION 'gas sponsorship unavailable' USING ERRCODE='P1050'; END IF;
    action_:=ledger.queue_escrow_action(branch_,'return',NULL,NULL,NULL,actor_,key_);
    INSERT INTO ledger.wallet_return VALUES(action_,payer_,recipient_,asset_,amount_,valid_before_,nonce_,intent_,cipher_,NULL);
END
$return$;

CREATE FUNCTION ledger.wallet_return_status(tenant_ uuid,branch_ uuid,actor_ uuid)
RETURNS TABLE(payer bytea,recipient bytea,amount text,valid_before bigint,state text,tx_hash text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $status$
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code<>'customer' THEN RAISE EXCEPTION 'customer scope required' USING ERRCODE='42501'; END IF;
    RETURN QUERY SELECT r.payer,r.recipient,r.amount::text,r.valid_before,
        CASE a.state WHEN 'queued' THEN CASE WHEN relay.broadcast_fence IS NULL THEN 'queued' ELSE 'submitted' END
            WHEN 'final' THEN 'final' ELSE 'failed' END,
        CASE WHEN a.state='final' THEN r.tx_hash WHEN relay.broadcast_fence IS NOT NULL THEN relay.tx_hash END
    FROM ledger.escrow_action a JOIN ledger.wallet_return r ON r.action_id=a.id LEFT JOIN ledger.relay_attempt relay ON relay.work_id=a.id
    WHERE a.branch_id=branch_ AND a.requester=actor_ AND a.kind='return' ORDER BY a.created_at DESC LIMIT 1;
END
$status$;

CREATE FUNCTION ledger.wallet_return_job(work_ uuid,fence_ bigint)
RETURNS TABLE(payer bytea,recipient bytea,asset_ref text,amount text,valid_before bigint,nonce bytea,signature_cipher bytea)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $job$
BEGIN
    PERFORM ledger.valid_settlement_lease(work_,fence_);
    RETURN QUERY SELECT r.payer,r.recipient,r.asset_ref,r.amount::text,r.valid_before,r.nonce,r.signature_cipher
        FROM ledger.wallet_return r WHERE r.action_id=work_;
END
$job$;

CREATE FUNCTION ledger.record_wallet_return(work_ uuid,fence_ bigint,tx_ text) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $record$
BEGIN
    PERFORM ledger.valid_settlement_lease(work_,fence_);
    UPDATE ledger.wallet_return SET tx_hash=tx_ WHERE action_id=work_ AND (tx_hash IS NULL OR tx_hash=tx_);
    IF NOT FOUND THEN RAISE EXCEPTION 'conflicting wallet return' USING ERRCODE='P1001'; END IF;
END
$record$;

REVOKE ALL ON FUNCTION ledger.relay_versions(uuid,bigint),ledger.replace_signed_relay(uuid,bigint,text,text,bytea),
    ledger.adopt_relay_version(uuid,text),ledger.adopt_included_relay(uuid,bigint,text),ledger.adopt_paid_relay(),
    ledger.note_escrow_refusal(uuid,bigint),ledger.finance_escrow_item(uuid,uuid,uuid,uuid),ledger.payer_has_open_order(bytea),
    ledger.payer_has_open_return(bytea),ledger.quote_return_guard(),ledger.enroll_payer(uuid,uuid,uuid,uuid,bytea,bytea,bigint),
    ledger.enrolled_payer(uuid,uuid,uuid),
    ledger.store_wallet_return(uuid,uuid,uuid,uuid,bytea,bytea,bytea,text,numeric,bigint,bytea,bytea,timestamptz),
    ledger.wallet_return_status(uuid,uuid,uuid),ledger.wallet_return_job(uuid,bigint),ledger.record_wallet_return(uuid,bigint,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.relay_versions(uuid,bigint),ledger.replace_signed_relay(uuid,bigint,text,text,bytea),
    ledger.adopt_included_relay(uuid,bigint,text),ledger.note_escrow_refusal(uuid,bigint),
    ledger.wallet_return_job(uuid,bigint),ledger.record_wallet_return(uuid,bigint,text) TO nu54_p05_settlement;
GRANT EXECUTE ON FUNCTION ledger.finance_escrow_item(uuid,uuid,uuid,uuid) TO nu54_p05_finance;
GRANT EXECUTE ON FUNCTION ledger.enroll_payer(uuid,uuid,uuid,uuid,bytea,bytea,bigint),ledger.enrolled_payer(uuid,uuid,uuid),
    ledger.store_wallet_return(uuid,uuid,uuid,uuid,bytea,bytea,bytea,text,numeric,bigint,bytea,bytea,timestamptz),
    ledger.wallet_return_status(uuid,uuid,uuid) TO nu54_p05_commerce;

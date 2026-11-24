-- A customer's transfer to any address, signed by the board and relayed at the store's cost. It is the same
-- kind of token authorization as a return and uses the same relay rules, but it does not empty the account.

ALTER TABLE ledger.escrow_action DROP CONSTRAINT escrow_action_kind_check, DROP CONSTRAINT escrow_action_check;
ALTER TABLE ledger.escrow_action
    ADD CONSTRAINT escrow_action_kind_check CHECK (kind IN ('deliver','cancel','receipt','dispute','withdraw','refund-undelivered',
        'refund-dispute','claim','resolve','return','faucet','send')),
    ADD CONSTRAINT escrow_action_check CHECK ((kind IN ('claim','return','faucet','send'))=(sale_id IS NULL));

CREATE TABLE ledger.wallet_send (
    action_id uuid PRIMARY KEY REFERENCES ledger.escrow_action,
    payer bytea NOT NULL CHECK (octet_length(payer)=20),
    recipient bytea NOT NULL CHECK (octet_length(recipient)=20 AND recipient<>payer),
    asset_ref text NOT NULL CHECK (asset_ref ~ '^0x[0-9a-f]{40}$'),
    amount numeric NOT NULL CHECK (amount>0 AND amount=trunc(amount) AND amount<power(2::numeric,256)),
    valid_before bigint NOT NULL CHECK (valid_before>0),
    nonce bytea NOT NULL CHECK (octet_length(nonce)=32),
    intent_digest bytea NOT NULL CHECK (octet_length(intent_digest)=32),
    signature_cipher bytea NOT NULL CHECK (octet_length(signature_cipher)=93),
    tx_hash text CHECK (tx_hash ~ '^0x[0-9a-f]{64}$'),
    UNIQUE(payer,nonce)
);
REVOKE ALL ON ledger.wallet_send FROM PUBLIC;

CREATE FUNCTION ledger.payer_has_open_send(payer_ bytea) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT EXISTS (SELECT FROM ledger.wallet_send s JOIN ledger.escrow_action a ON a.id=s.action_id WHERE s.payer=payer_ AND a.state='queued')
$$;

-- Another customer may not take over an account with a transfer on its way.
CREATE OR REPLACE FUNCTION ledger.customer_uses_payer(actor_ uuid,payer_ bytea) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT EXISTS (SELECT FROM ledger.quote_work q JOIN ledger.sale s ON s.id=q.sale_id LEFT JOIN ledger.order_escrow e ON e.sale_id=s.id
        WHERE q.payer=payer_ AND q.actor_id=actor_ AND (s.state='checking' OR (s.state IN ('preview','issued') AND q.expires_at>clock_timestamp())
            OR (e.sale_id IS NOT NULL AND NOT e.settled)))
        OR EXISTS (SELECT FROM ledger.wallet_return r JOIN ledger.escrow_action a ON a.id=r.action_id
            WHERE r.payer=payer_ AND a.requester=actor_ AND a.state='queued')
        OR EXISTS (SELECT FROM ledger.wallet_send s JOIN ledger.escrow_action a ON a.id=s.action_id
            WHERE s.payer=payer_ AND a.requester=actor_ AND a.state='queued')
$$;

-- A return empties the account, so no send may still be on its way. Both take the account's lock first.
CREATE FUNCTION ledger.return_send_guard() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $guard$
BEGIN
    IF ledger.payer_has_open_send(NEW.payer) THEN RAISE EXCEPTION 'a transfer is in progress' USING ERRCODE='P1060'; END IF;
    IF EXISTS (SELECT FROM ledger.wallet_send s WHERE (s.payer,s.nonce)=(NEW.payer,NEW.nonce)) THEN
        RAISE EXCEPTION 'the authorization belongs to a transfer' USING ERRCODE='P1001';
    END IF;
    RETURN NEW;
END
$guard$;
CREATE TRIGGER return_send_guard BEFORE INSERT ON ledger.wallet_return FOR EACH ROW EXECUTE FUNCTION ledger.return_send_guard();

CREATE FUNCTION ledger.store_wallet_send(tenant_ uuid,branch_ uuid,actor_ uuid,key_ uuid,intent_ bytea,payer_ bytea,recipient_ bytea,
    asset_ text,amount_ numeric,valid_before_ bigint,nonce_ bytea,cipher_ bytea,sponsored_until_ timestamptz,per_day_ integer)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $send$
DECLARE action_ uuid; enrolled bytea; now_ timestamptz;
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code<>'customer' THEN RAISE EXCEPTION 'customer scope required' USING ERRCODE='42501'; END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(encode(payer_,'hex'),107));
    PERFORM pg_advisory_xact_lock(hashtextextended(actor_::text,109));
    SELECT a.id INTO action_ FROM ledger.escrow_action a WHERE (a.branch_id,a.requester,a.request_key)=(branch_,actor_,key_);
    IF FOUND THEN
        IF NOT EXISTS (SELECT FROM ledger.wallet_send s WHERE s.action_id=action_ AND s.intent_digest=intent_) THEN
            RAISE EXCEPTION 'transfer request key conflict' USING ERRCODE='P1001';
        END IF;
        RETURN;
    END IF;
    IF per_day_ IS NULL OR per_day_<1 THEN RAISE EXCEPTION 'transfers are not offered' USING ERRCODE='P1050'; END IF;
    now_:=clock_timestamp();
    SELECT e.payer INTO enrolled FROM ledger.payer_enrollment e
        WHERE (e.branch_id,e.actor_id)=(branch_,actor_) AND e.payer_role=1 AND e.expires_at>now_ FOR SHARE;
    IF enrolled IS DISTINCT FROM payer_ THEN RAISE EXCEPTION 'account is not enrolled' USING ERRCODE='P1061'; END IF;
    IF valid_before_<=extract(epoch FROM now_) THEN RAISE EXCEPTION 'transfer expired' USING ERRCODE='P1002'; END IF;
    IF valid_before_>extract(epoch FROM now_)+600 THEN RAISE EXCEPTION 'transfer is valid for too long' USING ERRCODE='P1062'; END IF;
    IF ledger.payer_has_open_return(payer_) THEN RAISE EXCEPTION 'the account is being emptied' USING ERRCODE='P1060'; END IF;
    IF ledger.payer_has_open_send(payer_) OR EXISTS (SELECT FROM ledger.wallet_send s WHERE (s.payer,s.nonce)=(payer_,nonce_))
       OR EXISTS (SELECT FROM ledger.wallet_return r WHERE (r.payer,r.nonce)=(payer_,nonce_)) THEN
        RAISE EXCEPTION 'another transfer is in progress' USING ERRCODE='P1001';
    END IF;
    IF (SELECT count(*) FROM ledger.wallet_send s JOIN ledger.escrow_action a ON a.id=s.action_id
        WHERE (s.payer=payer_ OR a.requester=actor_) AND a.created_at>now_-interval '1 day')>=per_day_ THEN
        RAISE EXCEPTION 'too many transfers today' USING ERRCODE='P1004';
    END IF;
    -- The store pays the gas, so it must still agree to.
    IF sponsored_until_ IS NULL OR sponsored_until_<=now_ THEN RAISE EXCEPTION 'gas sponsorship unavailable' USING ERRCODE='P1050'; END IF;
    action_:=ledger.queue_escrow_action(branch_,'send',NULL,NULL,NULL,actor_,key_);
    INSERT INTO ledger.wallet_send VALUES(action_,payer_,recipient_,asset_,amount_,valid_before_,nonce_,intent_,cipher_,NULL);
END
$send$;

-- The newest transfers of this customer, and how many of the last 24 hours count.
CREATE FUNCTION ledger.wallet_send_status(tenant_ uuid,branch_ uuid,actor_ uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $status$
DECLARE payer_ bytea;
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code<>'customer' THEN RAISE EXCEPTION 'customer scope required' USING ERRCODE='42501'; END IF;
    SELECT e.payer INTO payer_ FROM ledger.payer_enrollment e
        WHERE (e.branch_id,e.actor_id)=(branch_,actor_) AND e.payer_role=1 AND e.expires_at>clock_timestamp();
    RETURN jsonb_build_object(
        'used',(SELECT count(*) FROM ledger.wallet_send s JOIN ledger.escrow_action a ON a.id=s.action_id
            WHERE (s.payer=payer_ OR a.requester=actor_) AND a.created_at>clock_timestamp()-interval '1 day'),
        'sends',(SELECT coalesce(jsonb_agg(row ORDER BY at DESC),'[]'::jsonb) FROM (
            SELECT a.created_at AS at,jsonb_strip_nulls(jsonb_build_object('payer','0x'||encode(s.payer,'hex'),'to','0x'||encode(s.recipient,'hex'),
                'token',s.asset_ref,'amount',s.amount::text,'validBefore',s.valid_before,'nonce',encode(s.nonce,'hex'),
                'createdAt',floor(extract(epoch FROM a.created_at))::bigint,
                'state',CASE a.state WHEN 'queued' THEN CASE WHEN sent.hash IS NULL THEN 'queued' ELSE 'submitted' END
                    WHEN 'final' THEN CASE WHEN s.tx_hash IS NOT NULL THEN 'final' WHEN sent.hash IS NULL THEN 'queued' ELSE 'submitted' END ELSE 'failed' END,
                'txHash',coalesce(s.tx_hash,sent.hash))) AS row
            FROM ledger.escrow_action a JOIN ledger.wallet_send s ON s.action_id=a.id
            CROSS JOIN LATERAL (SELECT ledger.relay_sent_hash(a.id) AS hash) sent
            WHERE a.branch_id=branch_ AND a.requester=actor_ AND a.kind='send' ORDER BY a.created_at DESC LIMIT 20) listed));
END
$status$;

-- The relay reads and closes both kinds of transfer the same way.
CREATE OR REPLACE FUNCTION ledger.wallet_return_job(work_ uuid,fence_ bigint)
RETURNS TABLE(payer bytea,recipient bytea,asset_ref text,amount text,valid_before bigint,nonce bytea,signature_cipher bytea)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $job$
BEGIN
    PERFORM ledger.valid_settlement_lease(work_,fence_);
    RETURN QUERY SELECT r.payer,r.recipient,r.asset_ref,r.amount::text,r.valid_before,r.nonce,r.signature_cipher
            FROM ledger.wallet_return r WHERE r.action_id=work_
        UNION ALL SELECT s.payer,s.recipient,s.asset_ref,s.amount::text,s.valid_before,s.nonce,s.signature_cipher
            FROM ledger.wallet_send s WHERE s.action_id=work_;
END
$job$;

CREATE OR REPLACE FUNCTION ledger.record_wallet_return(work_ uuid,fence_ bigint,tx_ text) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $record$
BEGIN
    PERFORM ledger.valid_settlement_lease(work_,fence_);
    UPDATE ledger.wallet_return SET tx_hash=tx_ WHERE action_id=work_ AND (tx_hash IS NULL OR tx_hash=tx_);
    IF FOUND THEN RETURN; END IF;
    UPDATE ledger.wallet_send SET tx_hash=tx_ WHERE action_id=work_ AND (tx_hash IS NULL OR tx_hash=tx_);
    IF NOT FOUND THEN RAISE EXCEPTION 'conflicting wallet return' USING ERRCODE='P1001'; END IF;
END
$record$;

REVOKE ALL ON FUNCTION ledger.payer_has_open_send(bytea),ledger.return_send_guard(),
    ledger.store_wallet_send(uuid,uuid,uuid,uuid,bytea,bytea,bytea,text,numeric,bigint,bytea,bytea,timestamptz,integer),
    ledger.wallet_send_status(uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.store_wallet_send(uuid,uuid,uuid,uuid,bytea,bytea,bytea,text,numeric,bigint,bytea,bytea,timestamptz,integer),
    ledger.wallet_send_status(uuid,uuid,uuid) TO nu54_p05_commerce;

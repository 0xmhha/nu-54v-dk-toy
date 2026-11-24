-- Test tokens that a dedicated account sends to a customer's enrolled account. A claim is one escrow_action,
-- so it uses the existing lease, nonce and stored-bytes rules. It is signed once and never signed again.

ALTER TABLE ledger.escrow_action DROP CONSTRAINT escrow_action_kind_check, DROP CONSTRAINT escrow_action_check;
ALTER TABLE ledger.escrow_action
    ADD CONSTRAINT escrow_action_kind_check CHECK (kind IN ('deliver','cancel','receipt','dispute','withdraw','refund-undelivered',
        'refund-dispute','claim','resolve','return','faucet')),
    ADD CONSTRAINT escrow_action_check CHECK ((kind IN ('claim','return','faucet'))=(sale_id IS NULL));

CREATE TABLE ledger.faucet_claim (
    action_id uuid PRIMARY KEY REFERENCES ledger.escrow_action,
    actor_id uuid NOT NULL,
    payer bytea NOT NULL CHECK (octet_length(payer)=20),
    asset_ref text NOT NULL CHECK (asset_ref ~ '^0x[0-9a-f]{40}$'),
    amount numeric NOT NULL CHECK (amount>0 AND amount=trunc(amount) AND amount<power(2::numeric,256)),
    sender_ref text NOT NULL CHECK (sender_ref ~ '^0x[0-9a-f]{40}$'),
    intent_digest bytea NOT NULL CHECK (octet_length(intent_digest)=32),
    claimed_at timestamptz NOT NULL,
    -- A claim that certainly moved nothing gives its place in the daily limits back.
    counted boolean NOT NULL DEFAULT true,
    -- The nonce was used and no transaction of this claim shows it.
    uncertain boolean NOT NULL DEFAULT false,
    tx_hash text CHECK (tx_hash ~ '^0x[0-9a-f]{64}$')
);
CREATE INDEX faucet_claim_asset ON ledger.faucet_claim(asset_ref,claimed_at);
REVOKE ALL ON ledger.faucet_claim FROM PUBLIC;

CREATE FUNCTION ledger.store_faucet_claim(tenant_ uuid,branch_ uuid,actor_ uuid,key_ uuid,intent_ bytea,asset_ text,amount_ numeric,
    per_day_ integer,daily_total_ numeric,available_ numeric,sender_ text)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $claim$
DECLARE action_ uuid; payer_ bytea; now_ timestamptz;
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code<>'customer' THEN RAISE EXCEPTION 'customer scope required' USING ERRCODE='42501'; END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(actor_::text,109));
    SELECT a.id INTO action_ FROM ledger.escrow_action a WHERE (a.branch_id,a.requester,a.request_key)=(branch_,actor_,key_);
    IF FOUND THEN
        IF NOT EXISTS (SELECT FROM ledger.faucet_claim c WHERE c.action_id=action_ AND c.intent_digest=intent_) THEN
            RAISE EXCEPTION 'claim request key conflict' USING ERRCODE='P1001';
        END IF;
        RETURN;
    END IF;
    IF per_day_ IS NULL OR per_day_<1 OR daily_total_ IS NULL OR daily_total_<amount_ THEN
        RAISE EXCEPTION 'faucet limits missing' USING ERRCODE='P1050';
    END IF;
    -- Every claim of a token is decided one after another: the limits and the balance are shared.
    PERFORM pg_advisory_xact_lock(hashtextextended(asset_,110));
    now_:=clock_timestamp();
    SELECT e.payer INTO payer_ FROM ledger.payer_enrollment e
        WHERE (e.branch_id,e.actor_id)=(branch_,actor_) AND e.payer_role=1 AND e.expires_at>now_ FOR SHARE;
    IF NOT FOUND THEN RAISE EXCEPTION 'account is not enrolled' USING ERRCODE='P1061'; END IF;
    IF ledger.payer_has_open_return(payer_) THEN RAISE EXCEPTION 'the account is being emptied' USING ERRCODE='P1060'; END IF;
    IF EXISTS (SELECT FROM ledger.faucet_claim c JOIN ledger.escrow_action a ON a.id=c.action_id
        WHERE c.asset_ref=asset_ AND a.state='queued' AND (c.payer=payer_ OR c.actor_id=actor_)) THEN
        RAISE EXCEPTION 'another claim is in progress' USING ERRCODE='P1001';
    END IF;
    IF (SELECT count(*) FROM ledger.faucet_claim c WHERE c.asset_ref=asset_ AND c.counted AND c.claimed_at>now_-interval '1 day'
            AND (c.payer=payer_ OR c.actor_id=actor_))>=per_day_
       OR (SELECT coalesce(sum(c.amount),0) FROM ledger.faucet_claim c WHERE c.asset_ref=asset_ AND c.counted
            AND c.claimed_at>now_-interval '1 day')+amount_>daily_total_ THEN
        RAISE EXCEPTION 'daily limit' USING ERRCODE='P1004';
    END IF;
    IF available_ IS NULL THEN RAISE EXCEPTION 'faucet balance unknown' USING ERRCODE='P1004'; END IF;
    IF available_<amount_+(SELECT coalesce(sum(c.amount),0) FROM ledger.faucet_claim c JOIN ledger.escrow_action a ON a.id=c.action_id
        WHERE c.asset_ref=asset_ AND a.state='queued') THEN
        RAISE EXCEPTION 'faucet is empty' USING ERRCODE='P1063';
    END IF;
    action_:=ledger.queue_escrow_action(branch_,'faucet',NULL,NULL,NULL,actor_,key_);
    INSERT INTO ledger.faucet_claim(action_id,actor_id,payer,asset_ref,amount,sender_ref,intent_digest,claimed_at)
        VALUES(action_,actor_,payer_,asset_,amount_,sender_,intent_,now_);
END
$claim$;

CREATE FUNCTION ledger.faucet_status(tenant_ uuid,branch_ uuid,actor_ uuid,assets_ text[]) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $status$
DECLARE payer_ bytea;
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code<>'customer' THEN RAISE EXCEPTION 'customer scope required' USING ERRCODE='42501'; END IF;
    SELECT e.payer INTO payer_ FROM ledger.payer_enrollment e
        WHERE (e.branch_id,e.actor_id)=(branch_,actor_) AND e.payer_role=1 AND e.expires_at>clock_timestamp();
    RETURN jsonb_strip_nulls(jsonb_build_object('payer','0x'||encode(payer_,'hex'),'tokens',(
        SELECT jsonb_object_agg(listed.asset,jsonb_build_object(
            'times',(SELECT coalesce(jsonb_agg(floor(extract(epoch FROM c.claimed_at))::bigint ORDER BY c.claimed_at DESC),'[]'::jsonb)
                FROM ledger.faucet_claim c WHERE c.asset_ref=listed.asset AND c.counted AND c.claimed_at>clock_timestamp()-interval '1 day'
                  AND (c.payer=payer_ OR c.actor_id=actor_)),
            'total',(SELECT coalesce(sum(c.amount),0)::text FROM ledger.faucet_claim c WHERE c.asset_ref=listed.asset AND c.counted
                AND c.claimed_at>clock_timestamp()-interval '1 day'),
            'held',(SELECT coalesce(sum(c.amount),0)::text FROM ledger.faucet_claim c JOIN ledger.escrow_action a ON a.id=c.action_id
                WHERE c.asset_ref=listed.asset AND a.state='queued'),
            'open',EXISTS (SELECT FROM ledger.faucet_claim c JOIN ledger.escrow_action a ON a.id=c.action_id
                WHERE c.asset_ref=listed.asset AND a.state='queued' AND (c.payer=payer_ OR c.actor_id=actor_)),
            'claim',(SELECT jsonb_strip_nulls(jsonb_build_object('amount',c.amount::text,'to','0x'||encode(c.payer,'hex'),
                    'createdAt',floor(extract(epoch FROM c.claimed_at))::bigint,
                    'state',CASE a.state WHEN 'queued' THEN CASE WHEN sent.hash IS NULL THEN 'queued' ELSE 'submitted' END
                        WHEN 'final' THEN 'final' ELSE CASE WHEN c.uncertain THEN 'unknown' ELSE 'failed' END END,
                    'txHash',coalesce(c.tx_hash,sent.hash)))
                FROM ledger.faucet_claim c JOIN ledger.escrow_action a ON a.id=c.action_id
                CROSS JOIN LATERAL (SELECT ledger.relay_sent_hash(a.id) AS hash) sent
                WHERE a.branch_id=branch_ AND c.actor_id=actor_ AND c.asset_ref=listed.asset ORDER BY c.claimed_at DESC LIMIT 1)))
        FROM unnest(assets_) AS listed(asset))));
END
$status$;

CREATE FUNCTION ledger.faucet_job(work_ uuid,fence_ bigint)
RETURNS TABLE(payer bytea,asset_ref text,amount text,sender_ref text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $job$
BEGIN
    PERFORM ledger.valid_settlement_lease(work_,fence_);
    RETURN QUERY SELECT c.payer,c.asset_ref,c.amount::text,c.sender_ref FROM ledger.faucet_claim c WHERE c.action_id=work_;
END
$job$;

-- What the chain established about one claim, stored before the job ends.
CREATE FUNCTION ledger.record_faucet(work_ uuid,fence_ bigint,tx_ text,counted_ boolean,uncertain_ boolean) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $record$
BEGIN
    PERFORM ledger.valid_settlement_lease(work_,fence_);
    UPDATE ledger.faucet_claim SET tx_hash=coalesce(tx_,tx_hash),counted=counted_,uncertain=uncertain_
        WHERE action_id=work_ AND (tx_hash IS NULL OR tx_ IS NULL OR tx_hash=tx_);
    IF NOT FOUND THEN RAISE EXCEPTION 'conflicting faucet result' USING ERRCODE='P1001'; END IF;
END
$record$;

REVOKE ALL ON FUNCTION ledger.store_faucet_claim(uuid,uuid,uuid,uuid,bytea,text,numeric,integer,numeric,numeric,text),
    ledger.faucet_status(uuid,uuid,uuid,text[]),ledger.faucet_job(uuid,bigint),ledger.record_faucet(uuid,bigint,text,boolean,boolean) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.store_faucet_claim(uuid,uuid,uuid,uuid,bytea,text,numeric,integer,numeric,numeric,text),
    ledger.faucet_status(uuid,uuid,uuid,text[]) TO nu54_p05_commerce;
GRANT EXECUTE ON FUNCTION ledger.faucet_job(uuid,bigint),ledger.record_faucet(uuid,bigint,text,boolean,boolean) TO nu54_p05_settlement;

-- After an order is locked: delivery, the buyer's answer, dispute, claim and the matching income.
-- Every transaction is a settlement_work job, so it uses the existing nonce, lease and recovery rules.

ALTER TABLE ledger.finance_grant DROP CONSTRAINT finance_grant_capability_check;
ALTER TABLE ledger.finance_grant ADD CONSTRAINT finance_grant_capability_check CHECK(capability IN (
    'refund_request','refund_approve','refund_submit','day_close','report_read','report_export','reconcile_read','reconcile_retry',
    'payout_propose','payout_approve','payout_notice','rental_manage','firmware_manage','firmware_station',
    'experience_read','experience_cancel','access_manage','audit_read','legacy_ledger','legacy_cashout','legacy_reconcile','pickup_manage',
    'escrow_claim','escrow_dispute_request','escrow_dispute_approve'
));
CREATE OR REPLACE FUNCTION ledger.operator_scopes(tenant uuid,actor uuid,role_ text)
RETURNS jsonb LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT coalesce(jsonb_agg(jsonb_build_object('id',b.id,'label',coalesce(n.label,'등록 이름 없음 · '||right(b.id::text,8)),'timezone',b.timezone,
        'capabilities',g.capabilities) ORDER BY n.label,b.id),'[]'::jsonb)
    FROM ledger.branch b LEFT JOIN ledger.operator_branch_label n ON n.branch_id=b.id
    JOIN LATERAL (
       SELECT array_agg(DISTINCT permission ORDER BY permission) AS capabilities
       FROM (
         SELECT 'catalog'::text AS permission FROM ledger.staff_grant s WHERE role_='operator' AND s.branch_id=b.id
           AND s.subject_ref=actor AND s.role_code='operator' AND s.valid_from<=clock_timestamp() AND s.revoked_at IS NULL
         UNION ALL
         SELECT f.capability FROM ledger.finance_grant f WHERE f.branch_id=b.id AND f.actor_id=actor AND
           f.valid_until>clock_timestamp() AND
           (role_='finance' AND f.capability IN ('refund_request','refund_approve','refund_submit','day_close','report_read','report_export','reconcile_read','reconcile_retry',
                'escrow_claim','escrow_dispute_request','escrow_dispute_approve')
            OR role_='operator' AND f.capability IN ('experience_read','experience_cancel','payout_propose','payout_approve','payout_notice',
                   'rental_manage','firmware_manage','firmware_station','access_manage','audit_read','legacy_ledger','legacy_cashout','legacy_reconcile')
            OR role_='operator' AND f.capability='pickup_manage' AND EXISTS (
                SELECT FROM ledger.staff_grant active WHERE active.branch_id=b.id AND active.subject_ref=actor
                  AND active.role_code='operator' AND active.valid_from<=clock_timestamp() AND active.revoked_at IS NULL))
       ) authorized
    ) g ON cardinality(g.capabilities)>0
    WHERE b.tenant_ref=tenant;
$$;

ALTER TABLE ledger.command_receipt DROP CONSTRAINT command_receipt_operation_check;
ALTER TABLE ledger.command_receipt ADD CONSTRAINT command_receipt_operation_check
    CHECK (operation IN ('create_sale','authorize_checkout','issue_quote','payment_forward','order_answer'));

ALTER TABLE ledger.order_escrow
    ADD COLUMN settled boolean NOT NULL DEFAULT false,
    ADD COLUMN check_at timestamptz NOT NULL DEFAULT clock_timestamp();
CREATE INDEX order_escrow_open ON ledger.order_escrow(check_at) WHERE NOT settled;

-- One row per finalized contract event. The contract reaches each state at most once.
CREATE TABLE ledger.order_escrow_event (
    evidence_id uuid PRIMARY KEY,
    sale_id uuid NOT NULL REFERENCES ledger.order_escrow,
    kind text NOT NULL CHECK (kind IN ('delivered','disputed','released','cancelled')),
    due_at bigint CHECK (due_at>0),
    reason smallint CHECK (reason BETWEEN 0 AND 3),
    tx_hash text NOT NULL CHECK (tx_hash ~ '^0x[0-9a-f]{64}$'),
    log_index integer NOT NULL CHECK (log_index>=0),
    inclusion_hash text NOT NULL CHECK (inclusion_hash ~ '^0x[0-9a-f]{64}$'),
    included_at timestamptz NOT NULL CHECK (isfinite(included_at)),
    UNIQUE(sale_id,kind),
    CHECK ((kind IN ('delivered','disputed'))=(due_at IS NOT NULL) AND (due_at IS NULL)=(reason IS NOT NULL))
);
CREATE UNIQUE INDEX order_escrow_settled_once ON ledger.order_escrow_event(sale_id) WHERE kind IN ('released','cancelled');

-- Income and the end of the deposit, booked when the money leaves the escrow.
CREATE TABLE ledger.escrow_journal (
    evidence_id uuid NOT NULL REFERENCES ledger.order_escrow_event,
    sale_id uuid NOT NULL,
    branch_id uuid NOT NULL,
    business_day date NOT NULL,
    asset_ref text NOT NULL,
    amount ledger.units NOT NULL CHECK (amount>0),
    debit_account text NOT NULL,
    credit_account text NOT NULL,
    booked_at timestamptz NOT NULL,
    PRIMARY KEY(evidence_id,debit_account),
    FOREIGN KEY(branch_id,sale_id) REFERENCES ledger.sale(branch_id,id),
    CHECK ((debit_account,credit_account) IN (('escrow_deposits','order_escrow'),('settlement_clearing','sales')))
);
CREATE INDEX escrow_journal_day ON ledger.escrow_journal(branch_id,business_day,asset_ref);

CREATE OR REPLACE VIEW ledger.posting AS
SELECT id AS journal_id, sale_id, refund_id, branch_id, business_day, asset_ref, account, side, amount
FROM ledger.journal
CROSS JOIN LATERAL (VALUES(debit_account,'debit'),(credit_account,'credit')) AS entry(account,side)
UNION ALL
SELECT evidence_id, sale_id, NULL::uuid, branch_id, business_day, asset_ref, account, side, amount
FROM ledger.escrow_journal
CROSS JOIN LATERAL (VALUES(debit_account,'debit'),(credit_account,'credit')) AS entry(account,side);

CREATE TABLE ledger.escrow_action (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    kind text NOT NULL CHECK (kind IN ('deliver','cancel','receipt','dispute','withdraw','refund-undelivered','refund-dispute','claim','resolve')),
    sale_id uuid REFERENCES ledger.order_escrow,
    claimable_at bigint,
    approval_cipher bytea CHECK (octet_length(approval_cipher)=93),
    requester uuid,
    request_key uuid,
    state text NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','final','superseded','failed')),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    closed_at timestamptz,
    UNIQUE(branch_id,requester,request_key),
    CHECK ((kind='claim')=(sale_id IS NULL)),
    CHECK ((kind IN ('receipt','dispute','withdraw'))=(approval_cipher IS NOT NULL AND claimable_at IS NOT NULL)),
    CHECK ((state='queued')=(closed_at IS NULL))
);
CREATE UNIQUE INDEX escrow_action_open ON ledger.escrow_action(sale_id,kind) WHERE state='queued';
CREATE UNIQUE INDEX escrow_answer_open ON ledger.escrow_action(sale_id)
    WHERE state='queued' AND kind IN ('receipt','dispute','withdraw');

CREATE TABLE ledger.escrow_claim_order (
    action_id uuid NOT NULL REFERENCES ledger.escrow_action,
    sale_id uuid NOT NULL REFERENCES ledger.order_escrow,
    position smallint NOT NULL CHECK (position BETWEEN 1 AND 100),
    outcome text CHECK (outcome IN ('paid','in-another-batch','unknown','not-delivered','window-open','disputed','settled','other-beneficiary')),
    PRIMARY KEY(action_id,sale_id),
    UNIQUE(action_id,position)
);
CREATE INDEX escrow_claim_by_order ON ledger.escrow_claim_order(sale_id);

CREATE TABLE ledger.escrow_decision (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    sale_id uuid NOT NULL REFERENCES ledger.order_escrow,
    outcome text NOT NULL CHECK (outcome IN ('pay-merchant','refund-buyer')),
    reason text NOT NULL CHECK (reason IN ('not_delivered','delivered_proof','wrong_item','goodwill')),
    proposer uuid NOT NULL,
    request_key uuid NOT NULL,
    decider uuid CHECK (decider<>proposer),
    state text NOT NULL DEFAULT 'proposed' CHECK (state IN ('proposed','approved','rejected','final','failed')),
    action_id uuid UNIQUE REFERENCES ledger.escrow_action,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    decided_at timestamptz,
    UNIQUE(sale_id,proposer,request_key),
    CHECK ((state IN ('approved','final'))<=(action_id IS NOT NULL)),
    CHECK ((decider IS NULL)=(decided_at IS NULL))
);
CREATE UNIQUE INDEX escrow_decision_open ON ledger.escrow_decision(sale_id) WHERE state IN ('proposed','approved');
REVOKE ALL ON ledger.order_escrow_event,ledger.escrow_journal,ledger.escrow_action,ledger.escrow_claim_order,ledger.escrow_decision FROM PUBLIC;

-- An action job names no sale, so every reader of the payment job of a sale still finds exactly one.
ALTER TABLE ledger.settlement_work ALTER COLUMN sale_id DROP NOT NULL,
    ADD COLUMN action_id uuid UNIQUE REFERENCES ledger.escrow_action,
    ADD CONSTRAINT settlement_work_subject CHECK ((sale_id IS NULL)=(action_id IS NOT NULL) AND (action_id IS NULL OR refund_id IS NULL));

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
    WHERE NOT w.finished AND w.sale_id IS NOT NULL AND w.retry_at<=clock_timestamp() AND (w.lease_until IS NULL OR w.lease_until<=clock_timestamp())
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
        OR (stalled.tx_hash IS NOT NULL AND job.action_id IS NULL AND stalled.nonce>=pending_nonce
            AND stalled.expires_at<=clock_timestamp()
            AND ((job.finished AND job.refund_id IS NULL) OR stalled.expires_at<=clock_timestamp()-interval '5 minutes')
            AND NOT EXISTS (SELECT FROM ledger.payment p WHERE p.tx_hash=stalled.tx_hash)
            AND NOT EXISTS (SELECT FROM ledger.refund_confirmation c WHERE c.tx_hash=stalled.tx_hash)))
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

-- The chain used this nonce for something else, or included our transaction and reverted it, in a
-- finalized block. The stored bytes can never act, so the job may be prepared again.
CREATE FUNCTION ledger.release_spent_relay(work_ uuid,fence_ bigint,hash_ text)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $spent$
BEGIN
    PERFORM ledger.valid_settlement_lease(work_,fence_);
    DELETE FROM ledger.relay_attempt WHERE work_id=work_ AND tx_hash=hash_ AND state<>'final';
    IF NOT FOUND THEN RAISE EXCEPTION 'relay attempt changed' USING ERRCODE='P1002'; END IF;
END
$spent$;

CREATE FUNCTION ledger.queue_escrow_action(branch_ uuid,kind_ text,sale_ uuid,claimable_ bigint,cipher_ bytea,requester_ uuid,key_ uuid)
RETURNS uuid LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $queue$
DECLARE action_ uuid;
BEGIN
    INSERT INTO ledger.escrow_action(branch_id,kind,sale_id,claimable_at,approval_cipher,requester,request_key)
        VALUES(branch_,kind_,sale_,claimable_,cipher_,requester_,key_) RETURNING id INTO action_;
    INSERT INTO ledger.settlement_work(id,action_id) VALUES(action_,action_);
    RETURN action_;
END
$queue$;

CREATE FUNCTION ledger.close_escrow_action(action_ uuid,state_ text) RETURNS void
LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $close$
BEGIN
    UPDATE ledger.escrow_action SET state=state_,closed_at=clock_timestamp() WHERE id=action_ AND state='queued';
    UPDATE ledger.escrow_decision SET state=CASE state_ WHEN 'final' THEN 'final' ELSE 'failed' END
        WHERE action_id=action_ AND state='approved';
    UPDATE ledger.settlement_work SET retry_at=clock_timestamp() WHERE id=action_ AND NOT finished;
END
$close$;

-- The store's own record of the handover starts the chain action, once.
CREATE FUNCTION ledger.queue_pickup_escrow() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $pickup$
DECLARE branch_ uuid;
BEGIN
    SELECT p.branch_id INTO branch_ FROM ledger.order_escrow e JOIN ledger.pickup p ON p.sale_id=e.sale_id
        WHERE e.sale_id=NEW.sale_id AND NOT e.settled;
    IF FOUND THEN
        PERFORM ledger.queue_escrow_action(branch_,CASE NEW.state WHEN 'collected' THEN 'deliver' ELSE 'cancel' END,
            NEW.sale_id,NULL,NULL,NULL,NULL);
    END IF;
    RETURN NEW;
END
$pickup$;
CREATE TRIGGER queue_pickup_escrow_insert AFTER INSERT ON ledger.travel_pickup_progress FOR EACH ROW
    WHEN (NEW.state IN ('collected','unfulfillable')) EXECUTE FUNCTION ledger.queue_pickup_escrow();
CREATE TRIGGER queue_pickup_escrow_update AFTER UPDATE OF state ON ledger.travel_pickup_progress FOR EACH ROW
    WHEN (NEW.state IN ('collected','unfulfillable') AND OLD.state<>NEW.state) EXECUTE FUNCTION ledger.queue_pickup_escrow();

CREATE FUNCTION ledger.claim_escrow_action(branches uuid[])
RETURNS TABLE(work_id uuid,fence bigint,branch_id uuid,kind text,is_open boolean,created_at timestamptz,claimable_at bigint,
    approval_cipher bytea,deliver_by bigint,resolve_by bigint,confirmer text,outcome text,stream_id uuid,
    relay_nonce text,relay_expiry timestamptz,relay_intent bytea,relay_hash text,relay_cipher bytea,relay_state text,
    sales uuid[],quotes bytea[],digests bytea[],signers bytea[])
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $claim$
DECLARE chosen ledger.settlement_work;
BEGIN
    SELECT w.* INTO chosen FROM ledger.settlement_work w JOIN ledger.escrow_action a ON a.id=w.action_id
    LEFT JOIN ledger.relay_attempt r ON r.work_id=w.id
    WHERE NOT w.finished AND a.branch_id=ANY(branches) AND w.retry_at<=clock_timestamp()
      AND (w.lease_until IS NULL OR w.lease_until<=clock_timestamp())
    ORDER BY CASE WHEN r.state IN ('reserved','signed') THEN 0 ELSE 1 END,w.retry_at,w.id
    LIMIT 1 FOR UPDATE OF w SKIP LOCKED;
    IF NOT FOUND THEN RETURN; END IF;
    UPDATE ledger.settlement_work w SET fence=w.fence+1,lease_until=clock_timestamp()+interval '25 seconds'
      WHERE w.id=chosen.id RETURNING w.* INTO chosen;
    RETURN QUERY
    SELECT chosen.id,chosen.fence,a.branch_id,a.kind,a.state='queued',a.created_at,a.claimable_at,a.approval_cipher,
        held.deliver_by,(SELECT e.due_at FROM ledger.order_escrow_event e WHERE e.sale_id=a.sale_id AND e.kind='disputed'),
        held.confirmer_ref,(SELECT d.outcome FROM ledger.escrow_decision d WHERE d.action_id=a.id),orders.stream,
        relay.nonce::text,relay.expires_at,relay.intent_digest,relay.tx_hash,relay.tx_cipher,relay.state,
        orders.sales,orders.quotes,orders.digests,orders.signers
    FROM ledger.escrow_action a
    LEFT JOIN ledger.order_escrow held ON held.sale_id=a.sale_id
    LEFT JOIN ledger.relay_attempt relay ON relay.work_id=chosen.id
    CROSS JOIN LATERAL (
        SELECT min(p.stream_id::text)::uuid AS stream,array_agg(s.sale_id ORDER BY s.position) AS sales,
            array_agg(q.payload_cipher ORDER BY s.position) AS quotes,array_agg(q.signed_digest ORDER BY s.position) AS digests,
            array_agg(q.merchant_signer ORDER BY s.position) AS signers
        FROM (SELECT a.sale_id,1::smallint AS position WHERE a.sale_id IS NOT NULL
              UNION ALL SELECT c.sale_id,c.position FROM ledger.escrow_claim_order c
                WHERE c.action_id=a.id AND c.outcome IS DISTINCT FROM 'in-another-batch') s
        JOIN ledger.quote_work q ON q.sale_id=s.sale_id JOIN ledger.payment p ON p.sale_id=s.sale_id
    ) orders
    WHERE a.id=chosen.action_id;
END
$claim$;

CREATE FUNCTION ledger.escrow_action_open(work_ uuid,fence_ bigint) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $open$
BEGIN
    PERFORM ledger.valid_settlement_lease(work_,fence_);
    RETURN EXISTS (SELECT FROM ledger.escrow_action a WHERE a.id=work_ AND a.state='queued');
END
$open$;

-- state_ records the relay observation; outcome_ closes the action. The job ends only when finished_,
-- which the worker sets once no signed bytes remain unspent.
CREATE FUNCTION ledger.defer_escrow_action(work_ uuid,fence_ bigint,state_ text,outcome_ text,finished_ boolean,delay_ interval)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $defer$
BEGIN
    PERFORM ledger.defer_settlement(work_,fence_,state_,finished_);
    IF outcome_ IS NOT NULL THEN
        IF outcome_ NOT IN ('final','superseded','failed') THEN RAISE EXCEPTION 'unsupported outcome' USING ERRCODE='P1001'; END IF;
        PERFORM ledger.close_escrow_action(work_,outcome_);
    END IF;
    IF finished_ THEN DELETE FROM ledger.relay_attempt WHERE work_id=work_ AND tx_hash IS NULL; END IF;
    UPDATE ledger.settlement_work SET retry_at=clock_timestamp()+delay_ WHERE id=work_ AND outcome_ IS NULL;
END
$defer$;

CREATE FUNCTION ledger.claim_escrow_follow(branches uuid[])
RETURNS TABLE(sale_id uuid,branch_id uuid,quote_cipher bytea,quote_digest bytea,merchant_signer bytea,stream_id uuid)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $follow$
DECLARE chosen uuid; due record;
BEGIN
    FOR due IN SELECT e.sale_id,p.branch_id,CASE WHEN d.due_at IS NULL THEN 'refund-undelivered' ELSE 'refund-dispute' END AS kind
        FROM ledger.order_escrow e JOIN ledger.pickup p ON p.sale_id=e.sale_id
        LEFT JOIN ledger.order_escrow_event d ON d.sale_id=e.sale_id AND d.kind='disputed'
        WHERE NOT e.settled AND p.branch_id=ANY(branches)
          AND (d.due_at<extract(epoch FROM clock_timestamp()) OR (d.due_at IS NULL AND e.deliver_by<extract(epoch FROM clock_timestamp())
              AND NOT EXISTS (SELECT FROM ledger.order_escrow_event v WHERE v.sale_id=e.sale_id AND v.kind='delivered')))
          AND NOT EXISTS (SELECT FROM ledger.escrow_action a WHERE a.sale_id=e.sale_id AND a.state='queued'
              AND a.kind IN ('refund-undelivered','refund-dispute'))
        ORDER BY e.deliver_by LIMIT 20 FOR UPDATE OF e SKIP LOCKED
    LOOP
        PERFORM ledger.queue_escrow_action(due.branch_id,due.kind,due.sale_id,NULL,NULL,NULL,NULL);
    END LOOP;
    SELECT e.sale_id INTO chosen FROM ledger.order_escrow e JOIN ledger.pickup p ON p.sale_id=e.sale_id
        WHERE NOT e.settled AND e.check_at<=clock_timestamp() AND p.branch_id=ANY(branches)
        ORDER BY e.check_at LIMIT 1 FOR UPDATE OF e SKIP LOCKED;
    IF NOT FOUND THEN RETURN; END IF;
    -- Orders with a running action are read by that action. A delivered order changes rarely.
    UPDATE ledger.order_escrow e SET check_at=clock_timestamp()+CASE WHEN EXISTS (
        SELECT FROM ledger.order_escrow_event v WHERE v.sale_id=chosen) THEN interval '5 minutes' ELSE interval '1 minute' END
      WHERE e.sale_id=chosen;
    RETURN QUERY SELECT chosen,s.branch_id,q.payload_cipher,q.signed_digest,q.merchant_signer,p.stream_id
        FROM ledger.sale s JOIN ledger.quote_work q ON q.sale_id=s.id JOIN ledger.payment p ON p.sale_id=s.id WHERE s.id=chosen;
END
$follow$;

CREATE FUNCTION ledger.record_escrow_event(sale_ uuid,contract_ text,evidence_ uuid,kind_ text,due_ bigint,reason_ integer,
    amount_ numeric,tx_ text,index_ integer,block_ text,included_ timestamptz)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $event$
DECLARE held ledger.order_escrow; paid ledger.payment; prior ledger.order_escrow_event; s ledger.sale; day_ date; done text; a record;
BEGIN
    SELECT * INTO STRICT s FROM ledger.sale WHERE id=sale_ FOR UPDATE;
    SELECT * INTO STRICT held FROM ledger.order_escrow WHERE sale_id=sale_ FOR UPDATE;
    SELECT * INTO STRICT paid FROM ledger.payment WHERE sale_id=sale_;
    IF held.contract_ref<>contract_ OR included_ IS NULL OR included_<paid.included_at THEN
        RAISE EXCEPTION 'escrow event outside the locked order' USING ERRCODE='P1001';
    END IF;
    SELECT * INTO prior FROM ledger.order_escrow_event e WHERE e.sale_id=sale_ AND e.kind=kind_;
    IF FOUND THEN
        IF (prior.evidence_id,prior.due_at,prior.reason::integer,prior.tx_hash,prior.log_index,prior.inclusion_hash)
            IS DISTINCT FROM (evidence_,due_,reason_,tx_,index_,block_) THEN
            RAISE EXCEPTION 'conflicting finalized escrow event' USING ERRCODE='P1001';
        END IF;
        RETURN false;
    END IF;
    IF kind_ IN ('released','cancelled') AND amount_ IS DISTINCT FROM paid.principal::numeric THEN
        RAISE EXCEPTION 'escrow payout differs from the locked amount' USING ERRCODE='P1001';
    END IF;
    INSERT INTO ledger.order_escrow_event VALUES(evidence_,sale_,kind_,due_,reason_,tx_,index_,block_,included_);
    UPDATE ledger.order_escrow SET check_at=clock_timestamp(),settled=kind_ IN ('released','cancelled') WHERE sale_id=sale_;
    IF kind_ IN ('released','cancelled') THEN
        day_:=ledger.ledger_business_day(s.branch_id,included_);
        PERFORM pg_advisory_xact_lock(hashtextextended(s.branch_id::text||day_::text||paid.asset_ref,10));
        INSERT INTO ledger.escrow_journal VALUES(evidence_,sale_,s.branch_id,day_,paid.asset_ref,paid.principal,'escrow_deposits','order_escrow',clock_timestamp());
        IF kind_='released' THEN
            INSERT INTO ledger.escrow_journal VALUES(evidence_,sale_,s.branch_id,day_,paid.asset_ref,paid.principal,'settlement_clearing','sales',clock_timestamp());
        END IF;
    END IF;
    done:=CASE kind_ WHEN 'delivered' THEN 'deliver' WHEN 'disputed' THEN 'dispute'
        WHEN 'released' THEN (ARRAY['receipt',NULL,'resolve','withdraw'])[reason_+1]
        ELSE (ARRAY['cancel','refund-undelivered','resolve','refund-dispute'])[reason_+1] END;
    FOR a IN SELECT x.id,x.kind,d.outcome FROM ledger.escrow_action x LEFT JOIN ledger.escrow_decision d ON d.action_id=x.id
        WHERE x.sale_id=sale_ AND x.state='queued'
    LOOP
        IF a.kind=done AND (a.outcome IS NULL OR (a.outcome='pay-merchant')=(kind_='released')) THEN PERFORM ledger.close_escrow_action(a.id,'final');
        ELSIF kind_ IN ('released','cancelled') OR (kind_='disputed' AND a.kind='receipt')
            OR (kind_='delivered' AND a.kind='refund-undelivered') THEN
            PERFORM ledger.close_escrow_action(a.id,'superseded');
        END IF;
    END LOOP;
    IF kind_ IN ('released','cancelled') THEN
        UPDATE ledger.escrow_decision SET state='failed' WHERE sale_id=sale_ AND state='proposed';
    END IF;
    RETURN true;
END
$event$;

CREATE FUNCTION ledger.record_escrow_claim(work_ uuid,fence_ bigint,sale_ uuid,outcome_ text) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $outcome$
BEGIN
    PERFORM ledger.valid_settlement_lease(work_,fence_);
    UPDATE ledger.escrow_claim_order SET outcome=outcome_ WHERE action_id=work_ AND sale_id=sale_ AND outcome IS NULL;
    IF NOT FOUND AND NOT EXISTS (SELECT FROM ledger.escrow_claim_order c WHERE c.action_id=work_ AND c.sale_id=sale_ AND c.outcome=outcome_) THEN
        RAISE EXCEPTION 'conflicting claim outcome' USING ERRCODE='P1001';
    END IF;
END
$outcome$;

-- Orders that still need the contract they were made for. Startup compares them with the enrolled one.
CREATE FUNCTION ledger.open_payment_contracts(branch_ uuid)
RETURNS TABLE(sale_id uuid,payload_cipher bytea,contract_ref text)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT s.id,q.payload_cipher,NULL::text FROM ledger.sale s JOIN ledger.quote_work q ON q.sale_id=s.id
      WHERE s.branch_id=branch_ AND s.state IN ('issued','checking') AND q.payload_cipher IS NOT NULL
    UNION ALL
    SELECT s.id,NULL,e.contract_ref FROM ledger.sale s JOIN ledger.order_escrow e ON e.sale_id=s.id
      WHERE s.branch_id=branch_ AND NOT e.settled
$$;

REVOKE ALL ON FUNCTION ledger.release_spent_relay(uuid,bigint,text),
    ledger.queue_escrow_action(uuid,text,uuid,bigint,bytea,uuid,uuid),ledger.close_escrow_action(uuid,text),ledger.queue_pickup_escrow(),
    ledger.claim_escrow_action(uuid[]),ledger.escrow_action_open(uuid,bigint),ledger.defer_escrow_action(uuid,bigint,text,text,boolean,interval),ledger.claim_escrow_follow(uuid[]),
    ledger.record_escrow_event(uuid,text,uuid,text,bigint,integer,numeric,text,integer,text,timestamptz),
    ledger.record_escrow_claim(uuid,bigint,uuid,text),ledger.open_payment_contracts(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.release_spent_relay(uuid,bigint,text),
    ledger.claim_escrow_action(uuid[]),ledger.escrow_action_open(uuid,bigint),ledger.defer_escrow_action(uuid,bigint,text,text,boolean,interval),ledger.claim_escrow_follow(uuid[]),
    ledger.record_escrow_event(uuid,text,uuid,text,bigint,integer,numeric,text,integer,text,timestamptz),
    ledger.record_escrow_claim(uuid,bigint,uuid,text),ledger.open_payment_contracts(uuid) TO nu54_p05_settlement;

-- ---- what customers and terminals read and send ----

DROP FUNCTION ledger.order_escrow_status(uuid,uuid,uuid,uuid);
CREATE FUNCTION ledger.order_escrow_status(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid)
RETURNS TABLE(revision bigint,payload_cipher bytea,approval_cipher bytea,relay_hash text,relay_sent boolean,paid_hash text,
    confirmer text,deliver_by bigint,claimable_at bigint,resolve_by bigint,settled_kind text,settled_reason smallint,
    settled_hash text,pending text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $status$
DECLARE grant_ ledger.commerce_grant := ledger.commerce_allowed(tenant_,branch_,actor_);
BEGIN
    RETURN QUERY
    SELECT s.revision,q.payload_cipher,lock.approval_cipher,attempt.tx_hash,attempt.broadcast_fence IS NOT NULL,paid.tx_hash,
        held.confirmer_ref,held.deliver_by,delivered.due_at,disputed.due_at,done.kind,done.reason,done.tx_hash,
        (SELECT a.kind FROM ledger.escrow_action a WHERE a.sale_id=s.id AND a.state='queued' AND a.kind IN ('receipt','dispute','withdraw'))
    FROM ledger.sale s
    JOIN ledger.order_checkout c ON (c.branch_id,c.sale_id)=(s.branch_id,s.id)
    JOIN ledger.quote_work q ON q.sale_id=s.id AND q.signed_digest IS NOT NULL
    LEFT JOIN ledger.settlement_submission lock ON lock.sale_id=s.id AND octet_length(lock.approval_cipher)=113
    LEFT JOIN ledger.settlement_work w ON w.sale_id=s.id AND w.refund_id IS NULL
    LEFT JOIN ledger.relay_attempt attempt ON attempt.work_id=w.id
    LEFT JOIN ledger.payment paid ON paid.sale_id=s.id
    LEFT JOIN ledger.order_escrow held ON held.sale_id=s.id
    LEFT JOIN ledger.order_escrow_event delivered ON delivered.sale_id=s.id AND delivered.kind='delivered'
    LEFT JOIN ledger.order_escrow_event disputed ON disputed.sale_id=s.id AND disputed.kind='disputed'
    LEFT JOIN ledger.order_escrow_event done ON done.sale_id=s.id AND done.kind IN ('released','cancelled')
    WHERE s.branch_id=branch_ AND s.id=sale_ AND (lock.sale_id IS NOT NULL OR held.sale_id IS NOT NULL)
      AND ((grant_.role_code='customer' AND q.actor_id=actor_) OR (grant_.role_code='terminal' AND c.creator=actor_));
END
$status$;

CREATE FUNCTION ledger.store_order_answer(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid,key_ uuid,intent_ bytea,
    kind_ text,claimable_ bigint,cipher_ bytea)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $answer$
DECLARE receipt ledger.command_receipt; rule ledger.command_rule; held ledger.order_escrow; delivered bigint; disputed boolean;
    open_ ledger.escrow_action; now_ timestamptz;
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code IS DISTINCT FROM 'customer' OR NOT EXISTS (
        SELECT FROM ledger.sale s JOIN ledger.quote_work q ON q.sale_id=s.id
        WHERE s.id=sale_ AND s.branch_id=branch_ AND q.actor_id=actor_) THEN
        RAISE EXCEPTION 'order answer scope denied' USING ERRCODE='42501';
    END IF;
    PERFORM FROM ledger.sale WHERE id=sale_ FOR UPDATE;
    SELECT * INTO receipt FROM ledger.command_receipt r
        WHERE r.branch_id=branch_ AND r.scope_ref=actor_ AND r.operation='order_answer' AND r.request_key=key_;
    IF FOUND THEN
        IF receipt.result_ref<>sale_ OR receipt.intent_digest IS DISTINCT FROM intent_ THEN
            RAISE EXCEPTION 'answer request key conflict' USING ERRCODE='P1001';
        END IF;
        RETURN sale_;
    END IF;
    SELECT * INTO held FROM ledger.order_escrow e WHERE e.sale_id=sale_;
    IF NOT FOUND THEN RAISE EXCEPTION 'order is not in escrow' USING ERRCODE='P1060'; END IF;
    SELECT e.due_at INTO delivered FROM ledger.order_escrow_event e WHERE e.sale_id=sale_ AND e.kind='delivered';
    SELECT EXISTS(SELECT FROM ledger.order_escrow_event e WHERE e.sale_id=sale_ AND e.kind='disputed') INTO disputed;
    IF held.settled OR disputed<>(kind_='withdraw') THEN
        RAISE EXCEPTION 'order is already decided' USING ERRCODE='P1060';
    END IF;
    IF delivered IS DISTINCT FROM claimable_ THEN
        RAISE EXCEPTION 'answer names another delivery' USING ERRCODE='P1061';
    END IF;
    now_:=clock_timestamp();
    SELECT * INTO open_ FROM ledger.escrow_action a WHERE a.sale_id=sale_ AND a.state='queued' AND a.kind IN ('receipt','dispute','withdraw');
    IF FOUND AND open_.kind<>kind_ THEN RAISE EXCEPTION 'another answer is in progress' USING ERRCODE='P1001'; END IF;
    IF open_.id IS NULL THEN
        IF kind_='dispute' AND extract(epoch FROM now_)>=claimable_ THEN
            RAISE EXCEPTION 'dispute window is over' USING ERRCODE='P1002';
        END IF;
        PERFORM ledger.queue_escrow_action(branch_,kind_,sale_,claimable_,cipher_,NULL,NULL);
    END IF;
    SELECT * INTO STRICT rule FROM ledger.command_rule WHERE branch_id=branch_;
    INSERT INTO ledger.command_receipt VALUES(branch_,actor_,'order_answer',key_,intent_,sale_,now_,now_+rule.receipt_window);
    RETURN sale_;
END
$answer$;
REVOKE ALL ON FUNCTION ledger.order_escrow_status(uuid,uuid,uuid,uuid),
    ledger.store_order_answer(uuid,uuid,uuid,uuid,uuid,bytea,text,bigint,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.order_escrow_status(uuid,uuid,uuid,uuid),
    ledger.store_order_answer(uuid,uuid,uuid,uuid,uuid,bytea,text,bigint,bytea) TO nu54_p05_commerce;

CREATE FUNCTION ledger.order_escrow_stage(sale_ uuid) RETURNS text
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT coalesce((SELECT CASE v.kind WHEN 'released' THEN 'paid' WHEN 'cancelled' THEN 'refunded' ELSE v.kind END
        FROM ledger.order_escrow_event v WHERE v.sale_id=sale_
        ORDER BY array_position(ARRAY['delivered','disputed','released','cancelled'],v.kind) DESC LIMIT 1),'locked')
    FROM ledger.order_escrow e WHERE e.sale_id=sale_
$$;
REVOKE ALL ON FUNCTION ledger.order_escrow_stage(uuid) FROM PUBLIC;

-- The kitchen list names what to prepare, as its detail already did, and the confirmed escrow stage.
CREATE OR REPLACE FUNCTION ledger.kiosk_pickup_queue(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid,role_ text,include_cafe_ boolean)
RETURNS SETOF jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $kitchen$
BEGIN
    PERFORM ledger.kiosk_require_pickup(tenant_,branch_,actor_,role_);
    RETURN QUERY SELECT jsonb_strip_nulls(jsonb_build_object(
      'saleRef',s.id,'kind',CASE WHEN include_cafe_ THEN CASE WHEN o.sale_id IS NULL THEN 'cafe' ELSE 'travel' END END,
      'startsAt',coalesce(o.starts_at,paid.included_at,submission.received_at,c.expires_at),'timezone',b.timezone,
      'pickupNo',CASE WHEN ack.accepted THEN p.number END,
      'eventRef',CASE WHEN include_cafe_ AND ack.accepted THEN e.event_id END,
      'state',CASE WHEN ack.accepted THEN coalesce(k.state,'accepted') ELSE 'checking' END,
      'revision',coalesce(k.revision,0),
      'escrow',CASE WHEN ack.accepted AND ledger.order_escrow_stage(s.id) IS NOT NULL
        THEN jsonb_build_object('stage',ledger.order_escrow_stage(s.id)) END,
      'settlement',CASE WHEN detail.booked_at IS NOT NULL THEN jsonb_build_object(
        'amount',paid.principal::text,'asset',paid.asset_ref,
        'finalizedAt',to_char(detail.booked_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
        'pickupReadyAt',to_char(detail.ready_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) END,
      'items',CASE WHEN ack.accepted THEN (SELECT jsonb_agg(jsonb_build_object(
        'label',x.label,'option',l.option_key,'quantity',l.quantity) ORDER BY l.sku COLLATE "C",l.option_key COLLATE "C")
        FROM ledger.order_line l JOIN ledger.price x ON
          (x.branch_id,x.version,x.sku,x.option_key)=(l.branch_id,l.menu_version,l.sku,l.option_key)
        WHERE l.sale_id=s.id) END))
    FROM ledger.sale s JOIN ledger.order_checkout c ON (c.branch_id,c.sale_id)=(s.branch_id,s.id)
    JOIN ledger.branch b ON b.id=s.branch_id
    LEFT JOIN ledger.travel_pickup_order o ON o.sale_id=s.id
    LEFT JOIN ledger.travel_pickup_progress k ON k.sale_id=s.id
    LEFT JOIN ledger.payment paid ON paid.sale_id=s.id
    LEFT JOIN ledger.settlement_submission submission ON submission.sale_id=s.id
    LEFT JOIN ledger.pickup p ON p.sale_id=s.id
    LEFT JOIN ledger.fulfillment_outbox e ON e.sale_id=s.id AND e.kind='paid' AND e.source_ref=s.id
    LEFT JOIN ledger.consumer_receipt response ON response.event_id=e.event_id AND response.effect_key=e.effect_key
    CROSS JOIN LATERAL (SELECT s.state='paid' AND p.sale_id IS NOT NULL AND response.event_id IS NOT NULL AS accepted) ack
    LEFT JOIN LATERAL (
        SELECT booked.booked_at, first_ready.ready_at
          FROM ledger.journal booked
          CROSS JOIN LATERAL (
            SELECT transition.ready_at FROM ledger.travel_pickup_transition transition
             WHERE transition.sale_id=s.id AND transition.target='ready' AND transition.revision<=k.revision
             ORDER BY transition.revision LIMIT 1
          ) first_ready
         WHERE (booked.branch_id,booked.sale_id,booked.evidence_id)=(s.branch_id,s.id,paid.evidence_id)
           AND booked.refund_id IS NULL AND (booked.asset_ref,booked.amount)=(paid.asset_ref,paid.principal)
           AND isfinite(booked.booked_at) AND first_ready.ready_at>=booked.booked_at
    ) detail ON sale_ IS NOT NULL AND include_cafe_ AND role_='operator' AND o.sale_id IS NULL
          AND ack.accepted AND k.state IN ('ready','collected')
          AND (paid.principal,paid.asset_ref)=(s.amount,s.asset_ref)
          AND paid.reserved=0 AND paid.in_flight=0 AND paid.refunded=0
          AND e.state='delivered' AND e.payload_digest=response.payload_digest
    WHERE s.branch_id=branch_ AND (sale_ IS NULL OR s.id=sale_) AND (
       (o.sale_id IS NOT NULL AND ack.accepted) OR
       (o.sale_id IS NULL AND include_cafe_ AND role_='operator' AND s.state IN ('checking','paid') AND ledger.kiosk_menu_pickup(s.id))
    ) AND (sale_ IS NOT NULL OR coalesce(k.state,'accepted') NOT IN ('collected','unfulfillable'))
    ORDER BY ack.accepted DESC,coalesce(o.starts_at,paid.included_at,submission.received_at,c.expires_at),s.id LIMIT 32;
END
$kitchen$;

-- ---- finance ----

CREATE FUNCTION ledger.escrow_decision_view(decision ledger.escrow_decision,actor uuid,branch uuid) RETURNS jsonb
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT jsonb_strip_nulls(jsonb_build_object('decisionId',(decision).id,'saleId',(decision).sale_id,'outcome',(decision).outcome,
        'reason',(decision).reason,'state',CASE WHEN (decision).state='approved' AND r.broadcast_fence IS NOT NULL THEN 'submitted' ELSE (decision).state END,
        'canApprove',(decision).state='proposed' AND (decision).proposer<>actor AND e.due_at>extract(epoch FROM clock_timestamp())
            AND EXISTS (SELECT FROM ledger.finance_grant g WHERE g.branch_id=branch AND g.actor_id=actor
                AND g.capability='escrow_dispute_approve' AND g.valid_until>clock_timestamp()),
        'transaction',coalesce(done.tx_hash,CASE WHEN r.broadcast_fence IS NOT NULL THEN r.tx_hash END),'resolveBy',e.due_at))
    FROM ledger.order_escrow_event e
    LEFT JOIN ledger.relay_attempt r ON r.work_id=(decision).action_id
    LEFT JOIN ledger.order_escrow_event done ON (decision).state='final' AND done.sale_id=(decision).sale_id AND done.kind IN ('released','cancelled')
    WHERE e.sale_id=(decision).sale_id AND e.kind='disputed'
$$;

CREATE FUNCTION ledger.escrow_order_page(branch uuid,actor uuid,status_ text,cursor_ uuid,sale_ uuid)
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

CREATE FUNCTION ledger.finance_escrow_orders(tenant uuid,branch uuid,actor uuid,status_ text,cursor_ uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'report_read');
    RETURN ledger.escrow_order_page(branch,actor,status_,cursor_,NULL);
END $$;

-- One order by its id shows the same item.
CREATE OR REPLACE FUNCTION ledger.finance_order_snapshot(tenant uuid,branch uuid,actor uuid,order_ uuid)
RETURNS TABLE (detail jsonb, chain_scope jsonb)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT snapshot.detail || CASE WHEN ledger.order_escrow_stage(order_) IS NULL THEN '{}'::jsonb ELSE jsonb_build_object(
        'fundsStatus',CASE ledger.order_escrow_stage(order_) WHEN 'paid' THEN 'confirmed' WHEN 'refunded' THEN 'closed' ELSE 'held' END,
        'escrow',ledger.escrow_order_page(branch,actor,'',NULL,order_)->'items'->0) END, snapshot.chain_scope
    FROM ledger.finance_order_snapshot_settled(tenant,branch,actor,order_) snapshot
$$;

CREATE FUNCTION ledger.escrow_claim_view(action_ uuid) RETURNS jsonb
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT jsonb_strip_nulls(jsonb_build_object('batchId',a.id,
        'state',CASE WHEN a.state='queued' THEN CASE WHEN r.broadcast_fence IS NULL THEN 'queued' ELSE 'submitted' END
            WHEN a.state='final' THEN 'final' ELSE 'failed' END,
        'createdAt',floor(extract(epoch FROM a.created_at))::bigint,'orders',count(*),'requested',sum(p.principal)::text,
        'paid',CASE WHEN a.state='final' THEN coalesce(sum(p.principal) FILTER (WHERE c.outcome='paid'),0)::text END,
        'transaction',CASE WHEN r.broadcast_fence IS NOT NULL THEN r.tx_hash END,
        'skipped',coalesce(jsonb_agg(jsonb_strip_nulls(jsonb_build_object('saleId',c.sale_id,'pickupNo',n.number,'reason',c.outcome)) ORDER BY c.position)
            FILTER (WHERE c.outcome<>'paid'),'[]'::jsonb)))
    FROM ledger.escrow_action a JOIN ledger.escrow_claim_order c ON c.action_id=a.id JOIN ledger.payment p ON p.sale_id=c.sale_id
    LEFT JOIN ledger.relay_attempt r ON r.work_id=a.id LEFT JOIN ledger.pickup n ON n.sale_id=c.sale_id
    WHERE a.id=action_ GROUP BY a.id,r.work_id
$$;

CREATE FUNCTION ledger.finance_escrow_claim(tenant uuid,branch uuid,actor uuid,key_ uuid,sales_ uuid[])
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $claim$
DECLARE action_ uuid; busy uuid[];
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'escrow_claim');
    IF cardinality(sales_) NOT BETWEEN 1 AND 100 OR (SELECT count(DISTINCT x) FROM unnest(sales_) x)<>cardinality(sales_) THEN
        RAISE EXCEPTION 'invalid claim list' USING ERRCODE='23514';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||'escrow-claim',10));
    SELECT a.id INTO action_ FROM ledger.escrow_action a WHERE (a.branch_id,a.requester,a.request_key)=(branch,actor,key_);
    IF FOUND THEN
        IF sales_ IS DISTINCT FROM (SELECT array_agg(c.sale_id ORDER BY c.position) FROM ledger.escrow_claim_order c WHERE c.action_id=action_) THEN
            RAISE EXCEPTION 'claim request key conflict' USING ERRCODE='P1001';
        END IF;
        RETURN ledger.escrow_claim_view(action_);
    END IF;
    IF (SELECT count(*) FROM ledger.order_escrow e JOIN ledger.sale s ON s.id=e.sale_id WHERE s.branch_id=branch AND e.sale_id=ANY(sales_))<>cardinality(sales_) THEN
        RAISE EXCEPTION 'claim scope denied' USING ERRCODE='42501';
    END IF;
    busy:=ARRAY(SELECT c.sale_id FROM ledger.escrow_claim_order c JOIN ledger.escrow_action a ON a.id=c.action_id
        WHERE c.sale_id=ANY(sales_) AND a.state='queued' AND c.outcome IS DISTINCT FROM 'in-another-batch');
    IF cardinality(busy)=cardinality(sales_) THEN
        RAISE EXCEPTION 'every order is already being claimed' USING ERRCODE='P1001';
    END IF;
    action_:=ledger.queue_escrow_action(branch,'claim',NULL,NULL,NULL,actor,key_);
    INSERT INTO ledger.escrow_claim_order(action_id,sale_id,position,outcome)
        SELECT action_,x.sale,x.at,CASE WHEN x.sale=ANY(busy) THEN 'in-another-batch' END FROM unnest(sales_) WITH ORDINALITY x(sale,at);
    RETURN ledger.escrow_claim_view(action_);
END
$claim$;

CREATE FUNCTION ledger.finance_escrow_claims(tenant uuid,branch uuid,actor uuid,cursor_ uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $claims$
DECLARE result jsonb;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'report_read');
    WITH page AS (
        SELECT a.id,a.created_at FROM ledger.escrow_action a WHERE a.branch_id=branch AND a.kind='claim' AND (cursor_ IS NULL OR
            (a.created_at,a.id)<(SELECT last.created_at,last.id FROM ledger.escrow_action last WHERE last.id=cursor_ AND last.branch_id=branch))
        ORDER BY a.created_at DESC,a.id DESC LIMIT 21
    ), shown AS (SELECT * FROM page ORDER BY created_at DESC,id DESC LIMIT 20)
    SELECT jsonb_strip_nulls(jsonb_build_object(
        'items',coalesce((SELECT jsonb_agg(ledger.escrow_claim_view(x.id) ORDER BY x.created_at DESC,x.id DESC) FROM shown x),'[]'::jsonb),
        'nextCursor',CASE WHEN (SELECT count(*) FROM page)>20 THEN (SELECT x.id FROM shown x ORDER BY x.created_at,x.id LIMIT 1) END)) INTO result;
    RETURN result;
END
$claims$;

CREATE FUNCTION ledger.finance_escrow_resolve(tenant uuid,branch uuid,actor uuid,key_ uuid,sale_ uuid,outcome_ text,reason_ text)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $propose$
DECLARE d ledger.escrow_decision; held ledger.order_escrow; due bigint;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'escrow_dispute_request');
    SELECT e.* INTO held FROM ledger.order_escrow e JOIN ledger.sale s ON s.id=e.sale_id
        WHERE e.sale_id=sale_ AND s.branch_id=branch FOR UPDATE OF e;
    IF NOT FOUND THEN RAISE EXCEPTION 'decision scope denied' USING ERRCODE='42501'; END IF;
    SELECT * INTO d FROM ledger.escrow_decision x WHERE (x.sale_id,x.proposer,x.request_key)=(sale_,actor,key_);
    IF FOUND THEN
        IF (d.outcome,d.reason) IS DISTINCT FROM (outcome_,reason_) THEN
            RAISE EXCEPTION 'decision request key conflict' USING ERRCODE='P1001';
        END IF;
        RETURN ledger.escrow_decision_view(d,actor,branch);
    END IF;
    SELECT e.due_at INTO due FROM ledger.order_escrow_event e WHERE e.sale_id=sale_ AND e.kind='disputed';
    IF held.settled OR due IS NULL OR due<=extract(epoch FROM clock_timestamp()) OR EXISTS (
        SELECT FROM ledger.escrow_decision x WHERE x.sale_id=sale_ AND x.state IN ('proposed','approved')) THEN
        RAISE EXCEPTION 'dispute is not open for a decision' USING ERRCODE='P1001';
    END IF;
    INSERT INTO ledger.escrow_decision(sale_id,outcome,reason,proposer,request_key) VALUES(sale_,outcome_,reason_,actor,key_) RETURNING * INTO d;
    RETURN ledger.escrow_decision_view(d,actor,branch);
END
$propose$;

CREATE FUNCTION ledger.finance_escrow_decide(tenant uuid,branch uuid,actor uuid,decision_ uuid,approve_ boolean)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $decide$
DECLARE d ledger.escrow_decision; held ledger.order_escrow; due bigint;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'escrow_dispute_approve');
    SELECT e.* INTO held FROM ledger.escrow_decision x JOIN ledger.order_escrow e ON e.sale_id=x.sale_id JOIN ledger.sale s ON s.id=e.sale_id
        WHERE x.id=decision_ AND s.branch_id=branch FOR UPDATE OF e;
    IF NOT FOUND THEN RAISE EXCEPTION 'decision scope denied' USING ERRCODE='42501'; END IF;
    SELECT * INTO STRICT d FROM ledger.escrow_decision WHERE id=decision_ FOR UPDATE;
    IF d.proposer=actor THEN RAISE EXCEPTION 'independent decision required' USING ERRCODE='42501'; END IF;
    IF d.decider=actor AND approve_=(d.state<>'rejected') THEN RETURN ledger.escrow_decision_view(d,actor,branch); END IF;
    SELECT e.due_at INTO STRICT due FROM ledger.order_escrow_event e WHERE e.sale_id=d.sale_id AND e.kind='disputed';
    IF d.state<>'proposed' OR held.settled OR due<=extract(epoch FROM clock_timestamp()) THEN
        RAISE EXCEPTION 'decision is already made or too late' USING ERRCODE='P1001';
    END IF;
    UPDATE ledger.escrow_decision SET state=CASE WHEN approve_ THEN 'approved' ELSE 'rejected' END,decider=actor,decided_at=clock_timestamp(),
        action_id=CASE WHEN approve_ THEN ledger.queue_escrow_action(branch,'resolve',d.sale_id,NULL,NULL,NULL,NULL) END
      WHERE id=decision_ RETURNING * INTO d;
    RETURN ledger.escrow_decision_view(d,actor,branch);
END
$decide$;

REVOKE ALL ON FUNCTION ledger.escrow_decision_view(ledger.escrow_decision,uuid,uuid),ledger.escrow_claim_view(uuid),
    ledger.escrow_order_page(uuid,uuid,text,uuid,uuid),ledger.finance_escrow_orders(uuid,uuid,uuid,text,uuid),ledger.finance_escrow_claim(uuid,uuid,uuid,uuid,uuid[]),
    ledger.finance_escrow_claims(uuid,uuid,uuid,uuid),ledger.finance_escrow_resolve(uuid,uuid,uuid,uuid,uuid,text,text),
    ledger.finance_escrow_decide(uuid,uuid,uuid,uuid,boolean) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.finance_escrow_orders(uuid,uuid,uuid,text,uuid),ledger.finance_escrow_claim(uuid,uuid,uuid,uuid,uuid[]),
    ledger.finance_escrow_claims(uuid,uuid,uuid,uuid),ledger.finance_escrow_resolve(uuid,uuid,uuid,uuid,uuid,text,text),
    ledger.finance_escrow_decide(uuid,uuid,uuid,uuid,boolean) TO nu54_p05_finance;

-- Every posting of one business day with its chain identity, in the order the indexer digests them.
CREATE FUNCTION ledger.finance_day_facts(branch uuid,day_ date,asset_ text)
RETURNS TABLE(kind text,tx_hash text,log_index integer,inclusion_hash text,amount numeric)
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT CASE WHEN j.refund_id IS NOT NULL THEN 'refunded' WHEN j.credit_account='escrow_deposits' THEN 'locked' ELSE 'paid' END,
        coalesce(f.tx_hash,p.tx_hash),coalesce(f.log_index,p.log_index),coalesce(f.inclusion_hash,p.inclusion_hash),j.amount::numeric
    FROM ledger.journal j JOIN ledger.payment p ON p.sale_id=j.sale_id
    LEFT JOIN ledger.refund_confirmation f ON f.refund_id=j.refund_id
    WHERE j.branch_id=branch AND j.business_day=day_ AND j.asset_ref=asset_
    UNION ALL
    SELECT e.kind,e.tx_hash,e.log_index,e.inclusion_hash,x.amount::numeric
    FROM ledger.escrow_journal x JOIN ledger.order_escrow_event e USING (evidence_id)
    WHERE x.branch_id=branch AND x.business_day=day_ AND x.asset_ref=asset_ AND x.debit_account='escrow_deposits'
$$;
REVOKE ALL ON FUNCTION ledger.finance_day_facts(uuid,date,text) FROM PUBLIC;

CREATE OR REPLACE FUNCTION ledger.finance_journal_hash(branch uuid,day_ date,asset_ text)
RETURNS text LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT encode(sha256(convert_to(coalesce(jsonb_agg(jsonb_build_array(j.kind,j.tx_hash,j.log_index,j.inclusion_hash,
        trim_scale(j.amount)::text) ORDER BY j.tx_hash COLLATE "C",j.log_index,j.inclusion_hash COLLATE "C"),'[]'::jsonb)::text,'UTF8')),'hex')
    FROM ledger.finance_day_facts(branch,day_,asset_) j
$$;

CREATE OR REPLACE FUNCTION ledger.finance_day_report(tenant uuid,branch uuid,actor uuid,day_ date,asset_ text,export_ boolean)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    scope ledger.branch;
    report jsonb;
    from_ timestamptz;
    until_ timestamptz;
BEGIN
    IF export_ IS TRUE THEN
        PERFORM ledger.require_finance(tenant,branch,actor,'report_export');
    ELSE
        PERFORM ledger.require_finance(tenant,branch,actor,'report_read');
    END IF;
    IF NOT isfinite(day_) OR asset_ !~ '^0x[0-9a-f]{40}$' THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid reporting scope';
    END IF;
    SELECT * INTO STRICT scope FROM ledger.branch b WHERE b.id=branch;
    from_:=(day_+scope.close_local) AT TIME ZONE scope.timezone;
    until_:=((day_+1)+scope.close_local) AT TIME ZONE scope.timezone;
    SELECT jsonb_build_object(
        'branch',branch,'day',day_,'asset',asset_,'zone',scope.timezone,'from',from_,'until',until_,
        'gross',trim_scale(coalesce(sum(j.amount) FILTER (WHERE j.kind IN ('paid','released')),0))::text,
        'refunds',trim_scale(coalesce(sum(j.amount) FILTER (WHERE j.kind='refunded'),0))::text,
        'net',trim_scale(coalesce(sum(CASE j.kind WHEN 'refunded' THEN -j.amount ELSE j.amount END) FILTER (WHERE j.kind IN ('paid','released','refunded')),0))::text,
        'postings',count(*),'digest',ledger.finance_journal_hash(branch,day_,asset_),
        'closeRevision',(SELECT max(c.revision) FROM ledger.day_close c
            WHERE c.branch_id=branch AND c.business_day=day_ AND c.asset_ref=asset_),
        'openFulfillment',(SELECT count(*) FROM ledger.fulfillment_outbox o JOIN ledger.journal p
            ON (o.kind='paid' AND o.source_ref=p.sale_id AND p.refund_id IS NULL) OR
               (o.kind='refunded' AND o.source_ref=p.refund_id)
            WHERE p.branch_id=branch AND p.business_day=day_ AND p.asset_ref=asset_ AND o.state<>'delivered'),
        'activeRefunds',(SELECT count(*) FROM ledger.refund r JOIN ledger.journal p
            ON p.sale_id=r.sale_id AND p.refund_id IS NULL
            WHERE p.branch_id=branch AND p.business_day=day_ AND p.asset_ref=asset_
                AND r.state IN ('requested','approved','inflight'))
    ) || CASE WHEN count(*) FILTER (WHERE j.kind IN ('locked','released','cancelled'))=0 THEN '{}'::jsonb ELSE jsonb_build_object(
        'escrowLocked',trim_scale(coalesce(sum(j.amount) FILTER (WHERE j.kind='locked'),0))::text,
        'escrowReturned',trim_scale(coalesce(sum(j.amount) FILTER (WHERE j.kind='cancelled'),0))::text) END
      INTO report FROM ledger.finance_day_facts(branch,day_,asset_) j;
    RETURN report;
END $$;

CREATE OR REPLACE FUNCTION ledger.close_business_day(tenant uuid,branch uuid,actor uuid,day_ date,asset_ text,
    gross_ numeric,refunds_ numeric,evidence_ uuid)
RETURNS bigint LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $close$
DECLARE
    gross_total numeric;
    refunds_total numeric;
    revision_ bigint;
    postings bytea;
    previous ledger.day_close;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'day_close');
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||day_::text||asset_,10));
    SELECT * INTO previous FROM ledger.day_close
       WHERE branch_id=branch AND business_day=day_ AND asset_ref=asset_ AND evidence_ref=evidence_;
    IF FOUND THEN
        IF (previous.gross,previous.refunds) IS DISTINCT FROM (gross_,refunds_) THEN
            RAISE EXCEPTION 'closing evidence reused with different totals' USING ERRCODE='P1001';
        END IF;
        RETURN previous.revision;
    END IF;
    -- Escrow deposits stay in the digest and out of the totals; a release counts as the sale.
    SELECT COALESCE(sum(amount) FILTER(WHERE refund_id IS NULL AND account='sales'),0),
        COALESCE(sum(amount) FILTER(WHERE refund_id IS NOT NULL),0),
        sha256(convert_to(COALESCE(string_agg(
          jsonb_build_array(id,evidence_id,sale_id,refund_id,amount)::text, chr(10) ORDER BY id,account),''),'UTF8'))
      INTO gross_total,refunds_total,postings FROM (
        SELECT id,evidence_id,sale_id,refund_id,amount::numeric,credit_account AS account FROM ledger.journal
          WHERE branch_id=branch AND business_day=day_ AND asset_ref=asset_
        UNION ALL
        SELECT evidence_id,evidence_id,sale_id,NULL,amount::numeric,credit_account FROM ledger.escrow_journal
          WHERE branch_id=branch AND business_day=day_ AND asset_ref=asset_) posted;
    IF (gross_total,refunds_total) IS DISTINCT FROM (gross_,refunds_) THEN
        RAISE EXCEPTION 'day-close reconciliation mismatch' USING ERRCODE='P1001';
    END IF;
    SELECT COALESCE(max(revision),0)+1 INTO revision_ FROM ledger.day_close
      WHERE branch_id=branch AND business_day=day_ AND asset_ref=asset_;
    INSERT INTO ledger.day_close(branch_id,business_day,asset_ref,revision,gross,refunds,evidence_ref,posting_digest,closed_by)
      VALUES(branch,day_,asset_,revision_,gross_,refunds_,evidence_,postings,actor);
    RETURN revision_;
END
$close$;

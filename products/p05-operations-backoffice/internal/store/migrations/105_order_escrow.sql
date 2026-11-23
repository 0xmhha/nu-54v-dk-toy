-- Orders paid into OrderEscrow. Admission, relay work, the paid transition, pickup number and
-- kitchen delivery stay the existing ones; this adds only what an escrow lock carries and the
-- guards that keep held money out of sales totals and out of the SettlementV2 refund path.

-- An order lock seals the 65-byte board signature with the 20-byte phone key address.
ALTER TABLE ledger.settlement_submission DROP CONSTRAINT settlement_submission_approval_cipher_check;
ALTER TABLE ledger.settlement_submission ADD CONSTRAINT settlement_submission_approval_cipher_check
    CHECK (octet_length(approval_cipher) IN (93,113));

-- The terms the contract fixed when the order was locked. Delivery, dispute and payout facts
-- refer to this row later.
CREATE TABLE ledger.order_escrow (
    sale_id uuid PRIMARY KEY REFERENCES ledger.payment,
    contract_ref text NOT NULL CHECK (contract_ref ~ '^0x[0-9a-f]{40}$'),
    confirmer_ref text NOT NULL CHECK (confirmer_ref ~ '^0x[0-9a-f]{40}$'),
    deliver_by bigint NOT NULL CHECK (deliver_by>0),
    confirm_window bigint NOT NULL CHECK (confirm_window>0)
);

-- A locked order owes its money to the buyer or the merchant until it settles: a deposit, not a sale.
ALTER TABLE ledger.journal DROP CONSTRAINT journal_check,
    DROP CONSTRAINT journal_debit_account_check, DROP CONSTRAINT journal_credit_account_check;
ALTER TABLE ledger.journal
    ADD CONSTRAINT journal_debit_account_check
        CHECK (debit_account IN ('settlement_clearing','sales_returns','order_escrow')),
    ADD CONSTRAINT journal_credit_account_check
        CHECK (credit_account IN ('sales','settlement_clearing','escrow_deposits')),
    ADD CONSTRAINT journal_check CHECK (
        (refund_id IS NULL AND debit_account='settlement_clearing' AND credit_account='sales')
        OR (refund_id IS NULL AND debit_account='order_escrow' AND credit_account='escrow_deposits')
        OR (refund_id IS NOT NULL AND debit_account='sales_returns' AND credit_account='settlement_clearing'));

CREATE FUNCTION ledger.confirm_order_lock(sale_ uuid, quote_ bytea, stream_ uuid, evidence_ uuid, tx_ text,
    index_ integer, block_ text, included_ timestamptz, asset_ text, amount_ numeric, payer_ text,
    generation_ bigint, merchant_ text, contract_ text, confirmer_ text, deliver_by_ bigint, window_ bigint)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $lock$
BEGIN
    IF NOT ledger.confirm_payment(sale_,quote_,stream_,evidence_,tx_,index_,block_,included_,asset_,amount_,payer_,
        generation_,1,merchant_) THEN
        IF NOT EXISTS (SELECT FROM ledger.order_escrow e WHERE e.sale_id=sale_
            AND (e.contract_ref,e.confirmer_ref,e.deliver_by,e.confirm_window)=(contract_,confirmer_,deliver_by_,window_)) THEN
            RAISE EXCEPTION 'conflicting finalized order lock' USING ERRCODE='P1001';
        END IF;
        RETURN false;
    END IF;
    INSERT INTO ledger.order_escrow VALUES(sale_,contract_,confirmer_,deliver_by_,window_);
    -- Same transaction as the posting above, so the order is never visible as a sale.
    UPDATE ledger.journal SET debit_account='order_escrow',credit_account='escrow_deposits'
        WHERE sale_id=sale_ AND refund_id IS NULL;
    RETURN true;
END
$lock$;
REVOKE ALL ON FUNCTION ledger.confirm_order_lock(uuid,bytea,uuid,uuid,text,integer,text,timestamptz,text,numeric,text,bigint,text,text,text,bigint,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.confirm_order_lock(uuid,bytea,uuid,uuid,text,integer,text,timestamptz,text,numeric,text,bigint,text,text,text,bigint,bigint)
    TO nu54_p05_settlement;

-- What the paying customer and the terminal that made the order may read. The caller holds the keys.
CREATE FUNCTION ledger.order_escrow_status(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid)
RETURNS TABLE(revision bigint,payload_cipher bytea,approval_cipher bytea,relay_hash text,paid_hash text,
    confirmer text,deliver_by bigint)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $status$
DECLARE grant_ ledger.commerce_grant := ledger.commerce_allowed(tenant_,branch_,actor_);
BEGIN
    RETURN QUERY
    SELECT s.revision,q.payload_cipher,lock.approval_cipher,attempt.tx_hash,paid.tx_hash,held.confirmer_ref,held.deliver_by
    FROM ledger.sale s
    JOIN ledger.order_checkout c ON (c.branch_id,c.sale_id)=(s.branch_id,s.id)
    JOIN ledger.quote_work q ON q.sale_id=s.id AND q.signed_digest IS NOT NULL
    LEFT JOIN ledger.settlement_submission lock ON lock.sale_id=s.id AND octet_length(lock.approval_cipher)=113
    LEFT JOIN ledger.settlement_work w ON w.sale_id=s.id AND w.refund_id IS NULL
    LEFT JOIN ledger.relay_attempt attempt ON attempt.work_id=w.id
    LEFT JOIN ledger.payment paid ON paid.sale_id=s.id
    LEFT JOIN ledger.order_escrow held ON held.sale_id=s.id
    WHERE s.branch_id=branch_ AND s.id=sale_ AND (lock.sale_id IS NOT NULL OR held.sale_id IS NOT NULL)
      AND ((grant_.role_code='customer' AND q.actor_id=actor_) OR (grant_.role_code='terminal' AND c.creator=actor_));
END
$status$;
REVOKE ALL ON FUNCTION ledger.order_escrow_status(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.order_escrow_status(uuid,uuid,uuid,uuid) TO nu54_p05_commerce;

-- False only for an order OrderEscrow cannot serve; order scope stays with prepare_quote.
CREATE FUNCTION ledger.order_escrow_allowed(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid,claim_ bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $allowed$
BEGIN
    PERFORM ledger.commerce_allowed(tenant_,branch_,actor_);
    RETURN NOT EXISTS (SELECT FROM ledger.sale s WHERE s.branch_id=branch_ AND s.id=sale_ AND s.origin NOT IN ('menu','pos'))
       AND NOT EXISTS (SELECT FROM ledger.travel_pickup_order p WHERE p.sale_id=sale_)
       AND NOT EXISTS (SELECT FROM ledger.checkout_claim cl WHERE cl.branch_id=branch_ AND cl.sale_id=sale_
            AND cl.claim_digest=claim_ AND cl.payer_role<>1);
END
$allowed$;
REVOKE ALL ON FUNCTION ledger.order_escrow_allowed(uuid,uuid,uuid,uuid,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.order_escrow_allowed(uuid,uuid,uuid,uuid,bytea) TO nu54_p05_commerce;

-- Every refund entry point inserts here, so none can send merchant funds for money the escrow holds.
CREATE FUNCTION ledger.order_escrow_refund_guard() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $guard$
BEGIN
    IF EXISTS (SELECT FROM ledger.order_escrow e WHERE e.sale_id=NEW.sale_id) THEN
        RAISE EXCEPTION 'an escrow order is refunded by the escrow contract' USING ERRCODE='P1001';
    END IF;
    RETURN NEW;
END
$guard$;
REVOKE ALL ON FUNCTION ledger.order_escrow_refund_guard() FROM PUBLIC;
CREATE TRIGGER order_escrow_refund_guard BEFORE INSERT ON ledger.refund
    FOR EACH ROW EXECUTE FUNCTION ledger.order_escrow_refund_guard();

ALTER FUNCTION ledger.console_refunds(uuid,uuid,uuid,uuid) RENAME TO console_refunds_settled;
REVOKE ALL ON FUNCTION ledger.console_refunds_settled(uuid,uuid,uuid,uuid) FROM PUBLIC,nu54_p05_finance;
CREATE FUNCTION ledger.console_refunds(tenant uuid,branch uuid,actor uuid,sale_ uuid)
RETURNS jsonb LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT view.report || CASE WHEN EXISTS (SELECT FROM ledger.order_escrow e WHERE e.sale_id=sale_)
        THEN jsonb_build_object('available','0','canRequest',false) ELSE '{}'::jsonb END
    FROM (SELECT ledger.console_refunds_settled(tenant,branch,actor,sale_) AS report) view
$$;
REVOKE ALL ON FUNCTION ledger.console_refunds(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.console_refunds(uuid,uuid,uuid,uuid) TO nu54_p05_finance;

-- The operator's order page says the funds are held, not confirmed.
ALTER FUNCTION ledger.finance_order_snapshot(uuid,uuid,uuid,uuid) RENAME TO finance_order_snapshot_settled;
REVOKE ALL ON FUNCTION ledger.finance_order_snapshot_settled(uuid,uuid,uuid,uuid) FROM PUBLIC,nu54_p05_finance;
CREATE FUNCTION ledger.finance_order_snapshot(tenant uuid,branch uuid,actor uuid,order_ uuid)
RETURNS TABLE (detail jsonb, chain_scope jsonb)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT snapshot.detail || CASE WHEN EXISTS (SELECT FROM ledger.order_escrow e WHERE e.sale_id=order_)
        THEN jsonb_build_object('fundsStatus','held') ELSE '{}'::jsonb END, snapshot.chain_scope
    FROM ledger.finance_order_snapshot_settled(tenant,branch,actor,order_) snapshot
$$;
REVOKE ALL ON FUNCTION ledger.finance_order_snapshot(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.finance_order_snapshot(uuid,uuid,uuid,uuid) TO nu54_p05_finance;

-- The three totals below change only by leaving escrow deposits out.
CREATE OR REPLACE FUNCTION ledger.finance_journal_hash(branch uuid,day_ date,asset_ text)
RETURNS text LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT encode(sha256(convert_to(coalesce(jsonb_agg(jsonb_build_array(
        CASE WHEN j.refund_id IS NULL THEN 'paid' ELSE 'refunded' END,
        coalesce(f.tx_hash,p.tx_hash),coalesce(f.log_index,p.log_index),coalesce(f.inclusion_hash,p.inclusion_hash),
        trim_scale(j.amount)::text)
        ORDER BY coalesce(f.tx_hash,p.tx_hash) COLLATE "C",coalesce(f.log_index,p.log_index),
            coalesce(f.inclusion_hash,p.inclusion_hash) COLLATE "C"),'[]'::jsonb)::text,'UTF8')),'hex')
    FROM ledger.journal j JOIN ledger.payment p ON p.sale_id=j.sale_id
    LEFT JOIN ledger.refund_confirmation f ON f.refund_id=j.refund_id
    WHERE j.branch_id=branch AND j.business_day=day_ AND j.asset_ref=asset_
      AND j.credit_account<>'escrow_deposits';
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
        'gross',trim_scale(coalesce(sum(j.amount) FILTER (WHERE j.refund_id IS NULL),0))::text,
        'refunds',trim_scale(coalesce(sum(j.amount) FILTER (WHERE j.refund_id IS NOT NULL),0))::text,
        'net',trim_scale(coalesce(sum(CASE WHEN j.refund_id IS NULL THEN j.amount ELSE -j.amount END),0))::text,
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
    ) INTO report FROM ledger.journal j
      WHERE j.branch_id=branch AND j.business_day=day_ AND j.asset_ref=asset_
        AND j.credit_account<>'escrow_deposits';
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
    IF EXISTS (SELECT FROM ledger.journal WHERE branch_id=branch AND business_day=day_ AND asset_ref=asset_
        AND credit_account='escrow_deposits') THEN
        RAISE EXCEPTION 'orders held in escrow have no settled total yet' USING ERRCODE='P1001';
    END IF;
    SELECT COALESCE(sum(amount) FILTER(WHERE refund_id IS NULL),0),
        COALESCE(sum(amount) FILTER(WHERE refund_id IS NOT NULL),0),
        sha256(convert_to(COALESCE(string_agg(
          jsonb_build_array(id,evidence_id,sale_id,refund_id,amount)::text, chr(10) ORDER BY id),''),'UTF8'))
      INTO gross_total,refunds_total,postings FROM ledger.journal
      WHERE branch_id=branch AND business_day=day_ AND asset_ref=asset_;
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

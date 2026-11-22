-- P05 settlement is additive. Existing quote ciphertext and v1 records stay untouched.
DO $roles$
DECLARE capability text;
BEGIN
    FOREACH capability IN ARRAY ARRAY['nu54_p05_finance','nu54_p05_settlement','nu54_p05_dispatch']
    LOOP
        IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname=capability) THEN
            EXECUTE format('CREATE ROLE %I NOLOGIN', capability);
        ELSIF EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname=capability
            AND (rolcanlogin OR rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb OR rolreplication)) THEN
            RAISE EXCEPTION 'unsafe role %', capability;
        END IF;
    END LOOP;
END
$roles$;

ALTER TABLE ledger.command_receipt DROP CONSTRAINT command_receipt_operation_check;
ALTER TABLE ledger.command_receipt ADD CHECK (operation IN ('create_sale','authorize_checkout','issue_quote','payment_forward'));

CREATE TABLE ledger.settlement_submission (
    sale_id uuid PRIMARY KEY REFERENCES ledger.quote_work,
    stream_id uuid NOT NULL,
    quote_digest bytea NOT NULL CHECK (octet_length(quote_digest)=32),
    approval_digest bytea NOT NULL CHECK (octet_length(approval_digest)=32),
    approval_cipher bytea NOT NULL CHECK (octet_length(approval_cipher)=93),
    received_at timestamptz NOT NULL
);

CREATE TABLE ledger.payment (
    sale_id uuid PRIMARY KEY REFERENCES ledger.quote_work,
    stream_id uuid NOT NULL,
    merchant_key text NOT NULL CHECK (merchant_key ~ '^0x[0-9a-f]{64}$'),
    evidence_id uuid NOT NULL,
    tx_hash text NOT NULL CHECK (tx_hash ~ '^0x[0-9a-f]{64}$'),
    log_index integer NOT NULL CHECK (log_index>=0),
    inclusion_hash text NOT NULL CHECK (inclusion_hash ~ '^0x[0-9a-f]{64}$'),
    included_at timestamptz NOT NULL CHECK (isfinite(included_at)),
    asset_ref text NOT NULL,
    payer text NOT NULL CHECK (payer ~ '^0x[0-9a-f]{40}$'),
    payer_generation bigint NOT NULL CHECK (payer_generation>0),
    payment_mode integer NOT NULL CHECK (payment_mode=1),
    principal ledger.units NOT NULL CHECK (principal>0),
    reserved ledger.units NOT NULL DEFAULT 0,
    in_flight ledger.units NOT NULL DEFAULT 0,
    refunded ledger.units NOT NULL DEFAULT 0,
    UNIQUE(stream_id,tx_hash,log_index),
    CHECK (reserved+in_flight+refunded<=principal)
);

CREATE TABLE ledger.finance_grant (
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    actor_id uuid NOT NULL,
    capability text NOT NULL CHECK (capability IN ('refund_request','refund_approve','day_close')),
    valid_until timestamptz NOT NULL CHECK (isfinite(valid_until)),
    PRIMARY KEY(branch_id,actor_id,capability)
);

CREATE TABLE ledger.refund (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    sale_id uuid NOT NULL REFERENCES ledger.payment,
    request_key uuid NOT NULL,
    chain_refund_id bytea NOT NULL UNIQUE CHECK (octet_length(chain_refund_id)=32),
    amount ledger.units NOT NULL CHECK (amount>0),
    requester uuid NOT NULL,
    approver uuid CHECK (approver<>requester),
    requested_at timestamptz NOT NULL,
    state text NOT NULL CHECK (state IN ('requested','approved','inflight','final','cancelled')),
    CHECK (state NOT IN ('approved','inflight','final') OR approver IS NOT NULL),
    UNIQUE(sale_id,request_key),
    UNIQUE(sale_id,id)
);

CREATE TABLE ledger.journal (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    sale_id uuid NOT NULL REFERENCES ledger.payment,
    refund_id uuid UNIQUE,
    evidence_id uuid NOT NULL UNIQUE,
    branch_id uuid NOT NULL,
    business_day date NOT NULL,
    asset_ref text NOT NULL,
    amount ledger.units NOT NULL CHECK (amount>0),
    debit_account text NOT NULL CHECK (debit_account IN ('settlement_clearing','sales_returns')),
    credit_account text NOT NULL CHECK (credit_account IN ('sales','settlement_clearing')),
    booked_at timestamptz NOT NULL,
    CHECK ((refund_id IS NULL AND debit_account='settlement_clearing' AND credit_account='sales')
        OR (refund_id IS NOT NULL AND debit_account='sales_returns' AND credit_account='settlement_clearing')),
    FOREIGN KEY(branch_id,sale_id) REFERENCES ledger.sale(branch_id,id),
    FOREIGN KEY(sale_id,refund_id) REFERENCES ledger.refund(sale_id,id)
);
CREATE UNIQUE INDEX journal_one_sale_entry ON ledger.journal(sale_id) WHERE refund_id IS NULL;

CREATE VIEW ledger.posting AS
SELECT id AS journal_id, sale_id, refund_id, branch_id, business_day, asset_ref, account, side, amount
FROM ledger.journal
CROSS JOIN LATERAL (VALUES(debit_account,'debit'),(credit_account,'credit')) AS entry(account,side);

CREATE TABLE ledger.pickup_counter (
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    business_day date NOT NULL,
    next_number bigint NOT NULL CHECK (next_number>0),
    PRIMARY KEY(branch_id,business_day)
);
CREATE TABLE ledger.pickup (
    sale_id uuid PRIMARY KEY REFERENCES ledger.payment,
    branch_id uuid NOT NULL,
    business_day date NOT NULL,
    number bigint NOT NULL CHECK (number>0),
    receipt_ref uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    FOREIGN KEY(branch_id,sale_id) REFERENCES ledger.sale(branch_id,id),
    UNIQUE(branch_id,business_day,number)
);

CREATE TABLE ledger.fulfillment_outbox (
    event_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    sale_id uuid NOT NULL REFERENCES ledger.payment,
    kind text NOT NULL CHECK (kind IN ('paid','refunded')),
    source_ref uuid NOT NULL,
    revision bigint NOT NULL CHECK (revision>0),
    effect_key uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','sending','unknown','delivered')),
    fence bigint NOT NULL DEFAULT 0 CHECK (fence>=0),
    lease_until timestamptz,
    delivered_at timestamptz,
    CHECK ((state='delivered')=(delivered_at IS NOT NULL)),
    UNIQUE(kind,source_ref,revision)
);

CREATE TABLE ledger.fulfillment_inbox (
    event_id uuid PRIMARY KEY,
    effect_key uuid NOT NULL UNIQUE,
    payload_digest bytea NOT NULL CHECK (octet_length(payload_digest)=32),
    received_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE ledger.sale_resolution (
    sale_id uuid PRIMARY KEY REFERENCES ledger.quote_work,
    evidence_id uuid NOT NULL UNIQUE,
    resolved_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

CREATE FUNCTION ledger.ledger_business_day(branch_ uuid, moment timestamptz) RETURNS date
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $sql$
    SELECT (moment AT TIME ZONE timezone)::date
        - CASE WHEN (moment AT TIME ZONE timezone)::time < close_local THEN 1 ELSE 0 END
    FROM ledger.branch WHERE id=branch_
$sql$;

CREATE FUNCTION ledger.forward_quote(tenant uuid, branch uuid, actor uuid, sale_ uuid)
RETURNS TABLE(payload_cipher bytea, signed_digest bytea, merchant_signer bytea, state text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $forward$
DECLARE
    allowed ledger.commerce_grant := ledger.commerce_allowed(tenant,branch,actor);
BEGIN
    RETURN QUERY
    SELECT q.payload_cipher,q.signed_digest,q.merchant_signer,s.state
    FROM ledger.quote_work q JOIN ledger.sale s ON s.id=q.sale_id
    WHERE s.id=sale_ AND s.branch_id=branch AND q.signed_digest IS NOT NULL
      AND (q.actor_id=actor OR (allowed.role_code='terminal' AND EXISTS (
        SELECT FROM ledger.order_checkout c WHERE c.sale_id=s.id AND c.creator=actor
      )));
END
$forward$;

CREATE FUNCTION ledger.store_forward(tenant uuid, branch uuid, actor uuid, sale_ uuid, key_ uuid,
    intent_ bytea, quote_ bytea, approval_ bytea, stream_ uuid, encrypted bytea)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $store$
DECLARE
    q ledger.quote_work;
    current_sale ledger.sale;
    previous ledger.settlement_submission;
    receipt ledger.command_receipt;
    rule ledger.command_rule;
    now_ timestamptz;
BEGIN
    IF NOT EXISTS (SELECT FROM ledger.forward_quote(tenant,branch,actor,sale_)) THEN
        RAISE EXCEPTION 'payment forwarding scope denied' USING ERRCODE='42501';
    END IF;
    SELECT * INTO STRICT current_sale FROM ledger.sale WHERE id=sale_ AND branch_id=branch FOR UPDATE;
    SELECT * INTO STRICT q FROM ledger.quote_work WHERE sale_id=sale_;
    IF q.signed_digest IS DISTINCT FROM quote_ THEN RAISE EXCEPTION 'issued quote does not match' USING ERRCODE='P1001'; END IF;
    SELECT * INTO receipt FROM ledger.command_receipt
       WHERE branch_id=branch AND scope_ref=actor AND operation='payment_forward' AND request_key=key_;
    IF FOUND AND (receipt.result_ref<>sale_ OR receipt.intent_digest IS DISTINCT FROM intent_) THEN
        RAISE EXCEPTION 'forward request key conflict' USING ERRCODE='P1001';
    END IF;
    SELECT * INTO STRICT rule FROM ledger.command_rule WHERE branch_id=branch;
    now_:=clock_timestamp();
    SELECT * INTO previous FROM ledger.settlement_submission WHERE sale_id=sale_;
    IF FOUND THEN
        IF (previous.quote_digest,previous.approval_digest,previous.stream_id) IS DISTINCT FROM (quote_,approval_,stream_) THEN
            RAISE EXCEPTION 'different approval for order' USING ERRCODE='P1001';
        END IF;
        IF receipt.request_key IS NULL THEN
            INSERT INTO ledger.command_receipt VALUES(branch,actor,'payment_forward',key_,intent_,sale_,now_,now_+rule.receipt_window);
        END IF;
        RETURN sale_;
    END IF;
    IF current_sale.state<>'issued' OR now_<date_trunc('second',q.issued_at) OR now_>=date_trunc('second',q.expires_at) THEN
        RAISE EXCEPTION 'new approval no longer accepted' USING ERRCODE='P1002';
    END IF;
    INSERT INTO ledger.settlement_submission VALUES(sale_,stream_,quote_,approval_,encrypted,now_);
    INSERT INTO ledger.command_receipt VALUES(branch,actor,'payment_forward',key_,intent_,sale_,now_,now_+rule.receipt_window);
    UPDATE ledger.sale SET state='checking',revision=revision+1 WHERE id=sale_;
    RETURN sale_;
END
$store$;

CREATE FUNCTION ledger.confirm_payment(sale_ uuid, quote_ bytea, stream_ uuid, evidence_ uuid, tx_ text,
    index_ integer, block_ text, included_ timestamptz, asset_ text, amount_ numeric, payer_ text,
    generation_ bigint, mode_ integer, merchant_ text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $pay$
DECLARE
    s ledger.sale;
    q ledger.quote_work;
    prior ledger.payment;
    day_ date;
    pickup_no bigint;
BEGIN
    SELECT * INTO STRICT s FROM ledger.sale WHERE id=sale_ FOR UPDATE;
    SELECT * INTO STRICT q FROM ledger.quote_work WHERE sale_id=sale_;
    IF quote_ IS DISTINCT FROM q.signed_digest OR q.signed_digest IS NULL OR
       (amount_,asset_,payer_,generation_,mode_) IS DISTINCT FROM
       (s.amount::numeric,s.asset_ref,'0x'||encode(q.payer,'hex'),q.payer_generation,1) OR
       included_ IS NULL OR included_<date_trunc('second',q.issued_at) OR included_>=date_trunc('second',q.expires_at) THEN
        RAISE EXCEPTION 'finalized payment does not match issued instruction' USING ERRCODE='P1001';
    END IF;
    SELECT * INTO prior FROM ledger.payment WHERE sale_id=sale_;
    IF FOUND THEN
        IF (prior.stream_id,prior.tx_hash,prior.log_index,prior.inclusion_hash,prior.included_at,prior.merchant_key)
            IS DISTINCT FROM (stream_,tx_,index_,block_,included_,merchant_) THEN
            RAISE EXCEPTION 'conflicting finalized payment identity' USING ERRCODE='P1001';
        END IF;
        RETURN false;
    END IF;
    IF s.state NOT IN ('issued','checking') OR EXISTS (SELECT FROM ledger.sale_resolution WHERE sale_id=sale_) THEN
        RAISE EXCEPTION 'order cannot accept payment' USING ERRCODE='P1002';
    END IF;
    IF EXISTS (SELECT FROM ledger.settlement_submission op WHERE op.sale_id=sale_ AND op.stream_id<>stream_) THEN
        RAISE EXCEPTION 'finalized stream differs' USING ERRCODE='P1001';
    END IF;
    day_:=ledger.ledger_business_day(s.branch_id,included_);
    PERFORM pg_advisory_xact_lock(hashtextextended(s.branch_id::text||day_::text||asset_,10));
    INSERT INTO ledger.payment(sale_id,stream_id,merchant_key,evidence_id,tx_hash,log_index,inclusion_hash,included_at,asset_ref,
        payer,payer_generation,payment_mode,principal)
        VALUES(sale_,stream_,merchant_,evidence_,tx_,index_,block_,included_,asset_,payer_,generation_,mode_,amount_);
    INSERT INTO ledger.journal(sale_id,evidence_id,branch_id,business_day,asset_ref,amount,debit_account,credit_account,booked_at)
        VALUES(sale_,evidence_,s.branch_id,day_,asset_,amount_,'settlement_clearing','sales',clock_timestamp());
    INSERT INTO ledger.pickup_counter VALUES(s.branch_id,day_,1)
        ON CONFLICT(branch_id,business_day) DO UPDATE SET next_number=ledger.pickup_counter.next_number+1
        RETURNING next_number INTO pickup_no;
    INSERT INTO ledger.pickup(sale_id,branch_id,business_day,number) VALUES(sale_,s.branch_id,day_,pickup_no);
    UPDATE ledger.quote_work SET consumed_at=clock_timestamp() WHERE sale_id=sale_;
    UPDATE ledger.sale SET state='paid',revision=revision+1 WHERE id=sale_;
    INSERT INTO ledger.fulfillment_outbox(sale_id,kind,source_ref,revision) VALUES(sale_,'paid',sale_,s.revision+1);
    RETURN true;
END
$pay$;

CREATE FUNCTION ledger.close_unpaid(sale_ uuid, quote_ bytea, evidence_ uuid, post_expiry timestamptz)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $unpaid$
DECLARE
    s ledger.sale;
    q ledger.quote_work;
    line ledger.order_line;
BEGIN
    SELECT * INTO STRICT s FROM ledger.sale WHERE id=sale_ FOR UPDATE;
    SELECT * INTO STRICT q FROM ledger.quote_work WHERE sale_id=sale_;
    IF q.signed_digest IS NULL OR q.signed_digest IS DISTINCT FROM quote_ OR
       post_expiry IS NULL OR post_expiry<date_trunc('second',q.expires_at) THEN
        RAISE EXCEPTION 'missing finalized post-expiry coverage' USING ERRCODE='P1001';
    END IF;
    IF EXISTS (SELECT FROM ledger.sale_resolution WHERE sale_id=sale_) THEN RETURN false; END IF;
    IF s.state NOT IN ('issued','checking') OR EXISTS (SELECT FROM ledger.payment WHERE sale_id=sale_) THEN
        RAISE EXCEPTION 'order has a conflicting disposition' USING ERRCODE='P1002';
    END IF;
    FOR line IN SELECT * FROM ledger.order_line WHERE sale_id=sale_ ORDER BY sku LOOP
        UPDATE ledger.stock SET available=available+line.quantity WHERE branch_id=s.branch_id AND sku=line.sku;
    END LOOP;
    INSERT INTO ledger.sale_resolution(sale_id,evidence_id) VALUES(sale_,evidence_);
    UPDATE ledger.sale SET state='closed',revision=revision+1 WHERE id=sale_;
    RETURN true;
END
$unpaid$;

CREATE FUNCTION ledger.require_finance(tenant uuid, branch uuid, actor uuid, capability_ text) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $scope$
BEGIN
    IF NOT EXISTS (
        SELECT FROM ledger.finance_grant g JOIN ledger.branch b ON b.id=g.branch_id
        WHERE b.tenant_ref=tenant AND b.id=branch AND g.actor_id=actor
          AND g.capability=capability_ AND g.valid_until>statement_timestamp()
    ) THEN RAISE EXCEPTION 'finance scope denied' USING ERRCODE='42501'; END IF;
END
$scope$;

CREATE FUNCTION ledger.reserve_refund(tenant uuid,branch uuid,actor uuid,sale_ uuid,key_ uuid,amount_ numeric)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $reserve$
DECLARE
    p ledger.payment;
    prior ledger.refund;
    result uuid;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'refund_request');
    SELECT payment.* INTO p FROM ledger.payment payment JOIN ledger.sale s ON s.id=payment.sale_id
      WHERE payment.sale_id=sale_ AND s.branch_id=branch FOR UPDATE OF payment;
    IF NOT FOUND THEN RAISE EXCEPTION 'payment scope denied' USING ERRCODE='42501'; END IF;
    SELECT * INTO prior FROM ledger.refund WHERE sale_id=sale_ AND request_key=key_;
    IF FOUND THEN
        IF prior.amount IS DISTINCT FROM amount_ OR prior.requester<>actor THEN
            RAISE EXCEPTION 'refund request key conflict' USING ERRCODE='P1001';
        END IF;
        RETURN prior.id;
    END IF;
    IF amount_ IS NULL OR amount_<=0 OR amount_<>trunc(amount_) OR p.reserved+p.in_flight+p.refunded+amount_>p.principal THEN
        RAISE EXCEPTION 'refund exceeds available principal' USING ERRCODE='P1003';
    END IF;
    UPDATE ledger.payment SET reserved=reserved+amount_ WHERE sale_id=sale_;
    INSERT INTO ledger.refund(sale_id,request_key,chain_refund_id,amount,requester,requested_at,state)
      VALUES(sale_,key_,sha256(convert_to(gen_random_uuid()::text||gen_random_uuid()::text,'UTF8')),
        amount_,actor,clock_timestamp(),'requested') RETURNING id INTO result;
    RETURN result;
END
$reserve$;

CREATE FUNCTION ledger.approve_refund(tenant uuid,branch uuid,actor uuid,sale_ uuid,refund_ uuid)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $approve$
DECLARE
    r ledger.refund;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'refund_approve');
    SELECT f.* INTO r FROM ledger.refund f JOIN ledger.sale s ON s.id=f.sale_id
        WHERE f.id=refund_ AND f.sale_id=sale_ AND s.branch_id=branch FOR UPDATE OF f;
    IF NOT FOUND OR r.requester=actor THEN RAISE EXCEPTION 'independent refund approval required' USING ERRCODE='42501'; END IF;
    IF r.approver=actor AND r.state<>'cancelled' THEN RETURN false; END IF;
    IF r.state<>'requested' THEN RAISE EXCEPTION 'refund is already decided' USING ERRCODE='P1001'; END IF;
    UPDATE ledger.refund SET approver=actor,state='approved' WHERE id=refund_;
    RETURN true;
END
$approve$;

CREATE FUNCTION ledger.mark_refund_inflight(refund_ uuid)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $inflight$
DECLARE
    r ledger.refund;
BEGIN
    SELECT * INTO STRICT r FROM ledger.refund WHERE id=refund_;
    PERFORM FROM ledger.payment WHERE sale_id=r.sale_id FOR UPDATE;
    SELECT * INTO STRICT r FROM ledger.refund WHERE id=refund_ FOR UPDATE;
    IF r.state='inflight' THEN RETURN false; END IF;
    IF r.state<>'approved' THEN RAISE EXCEPTION 'refund is not approved' USING ERRCODE='P1002'; END IF;
    UPDATE ledger.payment SET reserved=reserved-r.amount,in_flight=in_flight+r.amount WHERE sale_id=r.sale_id;
    UPDATE ledger.refund SET state='inflight' WHERE id=refund_;
    RETURN true;
END
$inflight$;

CREATE TABLE ledger.refund_confirmation (
    refund_id uuid PRIMARY KEY REFERENCES ledger.refund,
    stream_id uuid NOT NULL,
    evidence_id uuid NOT NULL UNIQUE,
    tx_hash text NOT NULL CHECK (tx_hash ~ '^0x[0-9a-f]{64}$'),
    log_index integer NOT NULL CHECK (log_index>=0),
    inclusion_hash text NOT NULL CHECK (inclusion_hash ~ '^0x[0-9a-f]{64}$'),
    included_at timestamptz NOT NULL CHECK (isfinite(included_at)),
    UNIQUE(stream_id,tx_hash,log_index)
);

CREATE FUNCTION ledger.confirm_refund(refund_ uuid, stream_ uuid, evidence_ uuid, tx_ text,index_ integer,
    block_ text,included_ timestamptz,asset_ text,amount_ numeric,payer_ text,mode_ integer,chain_refund_ bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $refund$
DECLARE
    r ledger.refund;
    p ledger.payment;
    s ledger.sale;
    prior ledger.refund_confirmation;
    refund_day date;
BEGIN
    SELECT * INTO STRICT r FROM ledger.refund WHERE id=refund_;
    SELECT * INTO STRICT s FROM ledger.sale WHERE id=r.sale_id FOR UPDATE;
    SELECT * INTO STRICT p FROM ledger.payment WHERE sale_id=r.sale_id FOR UPDATE;
    SELECT * INTO STRICT r FROM ledger.refund WHERE id=refund_ FOR UPDATE;
    IF (stream_,asset_,amount_,payer_,mode_,chain_refund_) IS DISTINCT FROM
        (p.stream_id,p.asset_ref,r.amount::numeric,p.payer,p.payment_mode,r.chain_refund_id)
        OR included_ IS NULL OR included_<p.included_at THEN
        RAISE EXCEPTION 'refund observation differs from original route or amount' USING ERRCODE='P1001';
    END IF;
    SELECT * INTO prior FROM ledger.refund_confirmation WHERE refund_id=refund_;
    IF FOUND THEN
        IF (prior.tx_hash,prior.log_index,prior.inclusion_hash,prior.included_at) IS DISTINCT FROM (tx_,index_,block_,included_) THEN
            RAISE EXCEPTION 'conflicting finalized refund' USING ERRCODE='P1001';
        END IF;
        RETURN false;
    END IF;
    IF r.state<>'inflight' OR r.approver IS NULL THEN
        RAISE EXCEPTION 'refund not submitted under approved route' USING ERRCODE='P1002';
    END IF;
    INSERT INTO ledger.refund_confirmation VALUES(refund_,stream_,evidence_,tx_,index_,block_,included_);
    UPDATE ledger.payment SET in_flight=in_flight-r.amount,refunded=refunded+r.amount WHERE sale_id=r.sale_id;
    UPDATE ledger.refund SET state='final' WHERE id=refund_;
    refund_day:=ledger.ledger_business_day(s.branch_id,included_);
    PERFORM pg_advisory_xact_lock(hashtextextended(s.branch_id::text||refund_day::text||asset_,10));
    INSERT INTO ledger.journal(sale_id,refund_id,evidence_id,branch_id,business_day,asset_ref,amount,debit_account,credit_account,booked_at)
        VALUES(r.sale_id,refund_,evidence_,s.branch_id,refund_day,asset_,amount_,
          'sales_returns','settlement_clearing',clock_timestamp());
    UPDATE ledger.sale SET revision=revision+1 WHERE id=s.id;
    INSERT INTO ledger.fulfillment_outbox(sale_id,kind,source_ref,revision)
        VALUES(s.id,'refunded',refund_,s.revision+1);
    RETURN true;
END
$refund$;

CREATE TABLE ledger.day_close (
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    business_day date NOT NULL,
    asset_ref text NOT NULL,
    revision bigint NOT NULL CHECK (revision>0),
    gross numeric NOT NULL CHECK (gross>=0 AND gross=trunc(gross)),
    refunds numeric NOT NULL CHECK (refunds>=0 AND refunds=trunc(refunds)),
    evidence_ref uuid NOT NULL,
    posting_digest bytea NOT NULL CHECK (octet_length(posting_digest)=32),
    closed_by uuid NOT NULL,
    closed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(branch_id,business_day,asset_ref,revision),
    UNIQUE(branch_id,business_day,asset_ref,evidence_ref)
);

CREATE FUNCTION ledger.close_business_day(tenant uuid,branch uuid,actor uuid,day_ date,asset_ text,
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

REVOKE ALL ON FUNCTION ledger.ledger_business_day(uuid,timestamptz),ledger.require_finance(uuid,uuid,uuid,text),
    ledger.forward_quote(uuid,uuid,uuid,uuid),ledger.store_forward(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea),
    ledger.confirm_payment(uuid,bytea,uuid,uuid,text,integer,text,timestamptz,text,numeric,text,bigint,integer,text),
    ledger.close_unpaid(uuid,bytea,uuid,timestamptz),ledger.reserve_refund(uuid,uuid,uuid,uuid,uuid,numeric),
    ledger.approve_refund(uuid,uuid,uuid,uuid,uuid),ledger.mark_refund_inflight(uuid),
    ledger.confirm_refund(uuid,uuid,uuid,text,integer,text,timestamptz,text,numeric,text,integer,bytea),
    ledger.close_business_day(uuid,uuid,uuid,date,text,numeric,numeric,uuid) FROM PUBLIC;
GRANT USAGE ON SCHEMA ledger TO nu54_p05_finance,nu54_p05_settlement,nu54_p05_dispatch;
GRANT EXECUTE ON FUNCTION ledger.forward_quote(uuid,uuid,uuid,uuid),
    ledger.store_forward(uuid,uuid,uuid,uuid,uuid,bytea,bytea,bytea,uuid,bytea) TO nu54_p05_commerce;
GRANT EXECUTE ON FUNCTION ledger.reserve_refund(uuid,uuid,uuid,uuid,uuid,numeric),
    ledger.approve_refund(uuid,uuid,uuid,uuid,uuid),ledger.close_business_day(uuid,uuid,uuid,date,text,numeric,numeric,uuid)
    TO nu54_p05_finance;
GRANT EXECUTE ON FUNCTION ledger.confirm_payment(uuid,bytea,uuid,uuid,text,integer,text,timestamptz,text,numeric,text,bigint,integer,text),
    ledger.close_unpaid(uuid,bytea,uuid,timestamptz),ledger.mark_refund_inflight(uuid),
    ledger.confirm_refund(uuid,uuid,uuid,text,integer,text,timestamptz,text,numeric,text,integer,bytea)
    TO nu54_p05_settlement;

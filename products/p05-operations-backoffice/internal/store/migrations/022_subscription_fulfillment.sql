-- Preserve pre-upgrade contracts and require each newly published offer to specify its fulfillment.
ALTER TABLE ledger.subscription_offer ADD COLUMN fulfillment_kind text NOT NULL DEFAULT 'physical'
    CHECK (fulfillment_kind IN ('physical','digital'));
ALTER TABLE ledger.subscription_offer ALTER COLUMN fulfillment_kind DROP DEFAULT;
ALTER TABLE ledger.invoice ADD COLUMN fulfillment_kind text NOT NULL DEFAULT 'physical'
    CHECK (fulfillment_kind IN ('physical','digital'));
ALTER TABLE ledger.invoice ALTER COLUMN fulfillment_kind DROP DEFAULT;

CREATE TABLE ledger.subscription_entitlement (
    invoice_id uuid PRIMARY KEY REFERENCES ledger.invoice,
    receipt_ref uuid NOT NULL UNIQUE,
    granted_at timestamptz NOT NULL CHECK (isfinite(granted_at)),
    revoked_at timestamptz CHECK (isfinite(revoked_at) AND revoked_at>=granted_at)
);

CREATE FUNCTION ledger.keep_subscription_terms() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $guard$
BEGIN
    IF to_jsonb(NEW)-'enabled' IS DISTINCT FROM to_jsonb(OLD)-'enabled' THEN
        RAISE EXCEPTION 'published terms require a new version' USING ERRCODE='P0501';
    END IF;
    RETURN NEW;
END
$guard$;
REVOKE ALL ON FUNCTION ledger.keep_subscription_terms() FROM PUBLIC;
CREATE TRIGGER subscription_terms_immutable BEFORE UPDATE ON ledger.subscription_offer
    FOR EACH ROW EXECUTE FUNCTION ledger.keep_subscription_terms();

CREATE OR REPLACE FUNCTION ledger.make_due_invoice() RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $issue$
DECLARE
    s ledger.subscription;
    invoice_ uuid := gen_random_uuid();
    sale_ uuid;
    next_ date;
    transacted timestamptz := clock_timestamp();
    due_ timestamptz;
BEGIN
    SELECT sch.* INTO s FROM ledger.subscription sch JOIN ledger.subscription_offer offer ON offer.id=sch.offer_id
      WHERE sch.state='active' AND offer.enabled AND sch.next_due_at<=transacted
      ORDER BY sch.next_due_at,sch.id LIMIT 1 FOR UPDATE OF sch SKIP LOCKED FOR SHARE OF offer;
    IF NOT FOUND THEN RETURN NULL; END IF;
    -- The unique calendar date is independent of monotonic clocks and UTC offsets.
    due_ := ((transacted AT TIME ZONE s.timezone)+make_interval(days=>s.due_days)) AT TIME ZONE s.timezone;
    INSERT INTO ledger.sale(branch_id,origin,origin_ref,state,asset_ref,amount)
      VALUES(s.branch_id,'subscription',invoice_,'preview',s.asset_ref,s.amount) RETURNING id INTO sale_;
    INSERT INTO ledger.order_checkout(sale_id,branch_id,creator,menu_version,expires_at,intent_digest)
      VALUES(sale_,s.branch_id,s.customer_scope,NULL,due_,
        sha256(convert_to(jsonb_build_array('subscription',s.id,s.next_cycle,s.asset_ref,s.amount)::text,'UTF8')));
    INSERT INTO ledger.invoice(id,branch_id,schedule_ref,cycle_key,due_at,sale_id,fulfillment_kind)
      SELECT invoice_,s.branch_id,s.id,s.next_cycle,due_,sale_,o.fulfillment_kind FROM ledger.subscription_offer o WHERE o.id=s.offer_id;
    next_:=ledger.next_subscription_cycle(s.next_cycle,s.cadence,s.anchor_day);
    UPDATE ledger.subscription SET next_cycle=next_,next_due_at=next_::timestamp AT TIME ZONE s.timezone WHERE id=s.id;
    RETURN invoice_;
END
$issue$;

CREATE OR REPLACE FUNCTION ledger.confirm_payment(sale_ uuid, quote_ bytea, stream_ uuid, evidence_ uuid, tx_ text,
    index_ integer, block_ text, included_ timestamptz, asset_ text, amount_ numeric, payer_ text,
    generation_ bigint, mode_ integer, merchant_ text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $pay$
DECLARE
    s ledger.sale;
    q ledger.quote_work;
    prior ledger.payment;
    day_ date;
    pickup_no bigint;
    entitlement ledger.invoice;
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
    IF s.origin='subscription' THEN
        SELECT * INTO STRICT entitlement FROM ledger.invoice WHERE sale_id=sale_;
    END IF;
    IF s.origin<>'travel' AND entitlement.fulfillment_kind IS DISTINCT FROM 'digital' THEN
        INSERT INTO ledger.pickup_counter VALUES(s.branch_id,day_,1)
            ON CONFLICT(branch_id,business_day) DO UPDATE SET next_number=ledger.pickup_counter.next_number+1
            RETURNING next_number INTO pickup_no;
        INSERT INTO ledger.pickup(sale_id,branch_id,business_day,number) VALUES(sale_,s.branch_id,day_,pickup_no);
    END IF;
    UPDATE ledger.quote_work SET consumed_at=clock_timestamp() WHERE sale_id=sale_;
    UPDATE ledger.sale SET state='paid',revision=revision+1 WHERE id=sale_;
    IF entitlement.fulfillment_kind='digital' THEN
        INSERT INTO ledger.subscription_entitlement(invoice_id,receipt_ref,granted_at) VALUES(entitlement.id,gen_random_uuid(),clock_timestamp());
    ELSIF s.origin<>'travel' THEN
            INSERT INTO ledger.fulfillment_outbox(sale_id,kind,source_ref,revision) VALUES(sale_,'paid',sale_,s.revision+1);
    END IF;
    RETURN true;
END
$pay$;

CREATE OR REPLACE FUNCTION ledger.confirm_refund(refund_ uuid, stream_ uuid, evidence_ uuid, tx_ text,index_ integer,
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
    IF s.origin='subscription' AND EXISTS(SELECT FROM ledger.invoice WHERE sale_id=s.id AND fulfillment_kind='digital') THEN
        IF p.refunded+r.amount=p.principal THEN
            UPDATE ledger.subscription_entitlement SET revoked_at=clock_timestamp()
            WHERE invoice_id=(SELECT id FROM ledger.invoice WHERE sale_id=s.id) AND revoked_at IS NULL;
        END IF;
    ELSIF s.origin<>'travel' THEN
        INSERT INTO ledger.fulfillment_outbox(sale_id,kind,source_ref,revision)
            VALUES(s.id,'refunded',refund_,s.revision+1);
    END IF;
    RETURN true;
END
$refund$;


-- Private order evidence and at-counter QR comparison for existing orders.
CREATE OR REPLACE FUNCTION ledger.commerce_order_view(tenant uuid, branch uuid, actor uuid, sale_ uuid)
RETURNS TABLE(id uuid, revision bigint, state text, asset_ref text, amount numeric, expires_at timestamptz, pickup_no bigint, receipt_ref uuid, quote_digest text, order_key text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.commerce_allowed(tenant,branch,actor);
    RETURN QUERY
      SELECT o.id,o.revision,o.state,o.asset_ref,o.amount::numeric,c.expires_at,p.number,COALESCE(p.receipt_ref,e.receipt_ref),
        CASE WHEN o.state='issued' THEN encode(q.signed_digest,'hex') END,
        CASE WHEN o.state='issued' THEN encode(q.order_id,'hex') END
      FROM ledger.sale o JOIN ledger.order_checkout c ON c.sale_id=o.id AND c.branch_id=o.branch_id
      LEFT JOIN ledger.pickup p ON p.branch_id=o.branch_id AND p.sale_id=o.id AND o.state='paid'
      LEFT JOIN ledger.invoice i ON i.sale_id=o.id AND o.origin='subscription'
      LEFT JOIN ledger.subscription_entitlement e ON e.invoice_id=i.id AND o.state='paid'
      LEFT JOIN ledger.quote_work q ON q.sale_id=o.id AND q.signed_digest IS NOT NULL
      WHERE o.branch_id=branch AND o.id=sale_ AND (c.creator=actor OR
        EXISTS(SELECT FROM ledger.checkout_claim issued WHERE issued.branch_id=branch AND issued.sale_id=o.id
            AND issued.actor_id=actor AND issued.expires_at>statement_timestamp()) OR
        (o.state='paid' AND COALESCE(p.receipt_ref,e.receipt_ref) IS NOT NULL AND q.actor_id=actor));
END $$;

-- Skip closes a preview atomically against quote issuance. A signed quote can
-- still rely on its original chain deadline and must be reconciled normally.
CREATE FUNCTION ledger.skip_subscription_invoice(tenant_ uuid,branch_ uuid,actor_ uuid,invoice_ uuid,request_ uuid)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $skip$
DECLARE
    sale_ uuid;
    schedule_ uuid;
    state_ text;
    prior ledger.subscription_command;
    intent bytea := sha256(convert_to(jsonb_build_array('skip',invoice_)::text,'UTF8'));
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code<>'customer' THEN
        RAISE EXCEPTION 'customer scope required' USING ERRCODE='42501';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch_::text||actor_::text||request_::text,61));
    SELECT * INTO prior FROM ledger.subscription_command WHERE branch_id=branch_ AND customer_scope=actor_ AND request_key=request_;
    IF FOUND THEN
        IF prior.intent_digest IS DISTINCT FROM intent THEN
            RAISE EXCEPTION 'subscription request changed' USING ERRCODE='P0501';
        END IF;
        RETURN invoice_;
    END IF;
    SELECT x.id,x.state,s.id INTO sale_,state_,schedule_ FROM ledger.invoice i
       JOIN ledger.subscription s ON s.id=i.schedule_ref AND s.customer_scope=actor_
       JOIN ledger.sale x ON x.id=i.sale_id
       WHERE i.id=invoice_ AND i.branch_id=branch_ FOR UPDATE OF x;
    IF NOT FOUND THEN RAISE EXCEPTION 'invoice scope denied' USING ERRCODE='42501'; END IF;
    IF state_='preview' THEN
        UPDATE ledger.sale SET state='closed',revision=revision+1 WHERE id=sale_;
    END IF;
    INSERT INTO ledger.subscription_command VALUES(branch_,actor_,request_,intent,schedule_);
    RETURN invoice_;
END
$skip$;
REVOKE ALL ON FUNCTION ledger.skip_subscription_invoice(uuid,uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.skip_subscription_invoice(uuid,uuid,uuid,uuid,uuid) TO nu54_p05_commerce;

DROP FUNCTION ledger.subscription_invoices(uuid,uuid,uuid), ledger.subscription_offers(uuid,uuid,uuid),
    ledger.subscriptions(uuid,uuid,uuid);

CREATE FUNCTION ledger.subscription_invoices(tenant_ uuid,branch_ uuid,actor_ uuid)
RETURNS TABLE(id uuid,subscription_id uuid,sale_id uuid,cycle_key date,due_at timestamptz,state text,asset_ref text,amount text,fulfillment_kind text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $open$
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code<>'customer' THEN
      RAISE EXCEPTION 'customer scope required' USING ERRCODE='42501';
    END IF;
    RETURN QUERY SELECT i.id,i.schedule_ref,i.sale_id,i.cycle_key,i.due_at,x.state,x.asset_ref,x.amount::text,i.fulfillment_kind
      FROM ledger.invoice i JOIN ledger.subscription s ON s.id=i.schedule_ref AND s.branch_id=i.branch_id
      JOIN ledger.sale x ON x.id=i.sale_id AND x.branch_id=i.branch_id
      WHERE i.branch_id=branch_ AND s.customer_scope=actor_ ORDER BY i.due_at DESC,i.id LIMIT 100;
END
$open$;

CREATE FUNCTION ledger.subscription_offers(tenant_ uuid,branch_ uuid,actor_ uuid)
RETURNS TABLE(id uuid,series_ref uuid,label text,terms_version text,terms_digest text,cadence text,timezone text,anchor_day integer,due_days integer,asset_ref text,amount text,fulfillment_kind text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $offers$
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code<>'customer' THEN
      RAISE EXCEPTION 'customer scope required' USING ERRCODE='42501';
    END IF;
    RETURN QUERY SELECT o.id,o.series_ref,o.label,o.terms_version,encode(o.terms_digest,'hex'),o.cadence,o.timezone,o.anchor_day,o.due_days,o.asset_ref,o.amount::text,o.fulfillment_kind
      FROM ledger.subscription_offer o WHERE o.branch_id=branch_ AND o.enabled ORDER BY o.id LIMIT 100;
END
$offers$;

CREATE FUNCTION ledger.subscriptions(tenant_ uuid,branch_ uuid,actor_ uuid)
RETURNS TABLE(id uuid,offer_id uuid,series_ref uuid,label text,terms_version text,terms_digest text,cadence text,timezone text,anchor_day integer,due_days integer,
    asset_ref text,amount text,state text,next_due_at timestamptz,revision bigint,needs_consent boolean,fulfillment_kind text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $status$
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code<>'customer' THEN
      RAISE EXCEPTION 'customer scope required' USING ERRCODE='42501';
    END IF;
    RETURN QUERY SELECT s.id,s.offer_id,o.series_ref,o.label,s.terms_version,encode(s.terms_digest,'hex'),s.cadence,s.timezone,s.anchor_day,s.due_days,
      s.asset_ref,s.amount::text,s.state,s.next_due_at,s.revision,NOT o.enabled AND s.state<>'cancelled',o.fulfillment_kind
      FROM ledger.subscription s JOIN ledger.subscription_offer o ON o.id=s.offer_id
      WHERE s.branch_id=branch_ AND s.customer_scope=actor_ ORDER BY s.accepted_at DESC,s.id LIMIT 100;
END
$status$;

REVOKE ALL ON FUNCTION ledger.subscription_invoices(uuid,uuid,uuid), ledger.subscription_offers(uuid,uuid,uuid),
    ledger.subscriptions(uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.subscription_invoices(uuid,uuid,uuid), ledger.subscription_offers(uuid,uuid,uuid),
    ledger.subscriptions(uuid,uuid,uuid) TO nu54_p05_commerce;

CREATE OR REPLACE FUNCTION ledger.accept_subscription(tenant_ uuid,branch_ uuid,actor_ uuid,offer_ uuid,digest_ bytea,request_ uuid)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $accept$
DECLARE
    o ledger.subscription_offer;
    result_ uuid;
    prior ledger.subscription_command;
    first_ date;
    last_ date;
    accepted timestamptz := clock_timestamp();
    intent bytea := sha256(convert_to(jsonb_build_array('accept',offer_,encode(digest_,'hex'))::text,'UTF8'));
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code<>'customer' THEN
      RAISE EXCEPTION 'customer scope required' USING ERRCODE='42501';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch_::text||actor_::text||request_::text,61));
    SELECT * INTO prior FROM ledger.subscription_command WHERE branch_id=branch_ AND customer_scope=actor_ AND request_key=request_;
    IF FOUND THEN
      IF prior.intent_digest IS DISTINCT FROM intent THEN RAISE EXCEPTION 'subscription request changed' USING ERRCODE='P0501'; END IF;
      RETURN prior.subscription_id;
    END IF;
    SELECT * INTO o FROM ledger.subscription_offer WHERE branch_id=branch_ AND id=offer_ AND enabled FOR SHARE;
    IF NOT FOUND OR o.terms_digest IS DISTINCT FROM digest_ OR o.timezone !~ '^(UTC|[A-Za-z_]+(/[-A-Za-z0-9_+]+)+)$' THEN
      RAISE EXCEPTION 'offer or terms unavailable' USING ERRCODE='P0702';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch_::text||actor_::text||o.series_ref::text,62));
    IF EXISTS (SELECT FROM ledger.subscription current_subscription
               JOIN ledger.subscription_offer current_offer ON current_offer.id=current_subscription.offer_id
               WHERE current_subscription.branch_id=branch_ AND current_subscription.customer_scope=actor_
                 AND current_subscription.state<>'cancelled' AND current_offer.series_ref=o.series_ref
                 AND current_offer.id<>o.id) THEN
      RAISE EXCEPTION 'explicit replacement consent required' USING ERRCODE='P0501';
    END IF;
    BEGIN
      first_ := (accepted AT TIME ZONE o.timezone)::date;
    EXCEPTION WHEN invalid_parameter_value THEN
      RAISE EXCEPTION 'offer timezone unavailable' USING ERRCODE='P0702';
    END;
    IF o.cadence='monthly' THEN
      first_ := date_trunc('month',first_::timestamp)::date
         + least(o.anchor_day,extract(day FROM date_trunc('month',first_::timestamp)+interval '1 month - 1 day')::integer)-1;
      IF (first_::timestamp AT TIME ZONE o.timezone)<accepted THEN
         first_ := ledger.next_subscription_cycle(first_,o.cadence,o.anchor_day);
      END IF;
    END IF;
    -- Reviving or replacing a contract never opens an already issued family cycle.
    SELECT max(i.cycle_key) INTO last_ FROM ledger.invoice i
       JOIN ledger.subscription old_schedule ON old_schedule.id=i.schedule_ref
       JOIN ledger.subscription_offer prior_terms ON prior_terms.id=old_schedule.offer_id
       WHERE i.branch_id=branch_ AND old_schedule.customer_scope=actor_ AND prior_terms.series_ref=o.series_ref;
    WHILE last_ IS NOT NULL AND first_<=last_ LOOP
        first_:=ledger.next_subscription_cycle(first_,o.cadence,o.anchor_day);
    END LOOP;
    INSERT INTO ledger.subscription(branch_id,customer_scope,offer_id,terms_version,terms_digest,accepted_at,cadence,timezone,
      anchor_day,due_days,asset_ref,amount,next_cycle,next_due_at)
    VALUES(branch_,actor_,offer_,o.terms_version,digest_,accepted,o.cadence,o.timezone,o.anchor_day,o.due_days,o.asset_ref,o.amount,
      first_,first_::timestamp AT TIME ZONE o.timezone)
    ON CONFLICT (branch_id,customer_scope,offer_id) WHERE state<>'cancelled' DO NOTHING RETURNING id INTO result_;
    IF result_ IS NULL THEN
      SELECT id INTO STRICT result_ FROM ledger.subscription WHERE branch_id=branch_ AND customer_scope=actor_ AND offer_id=offer_ AND state<>'cancelled';
    END IF;
    INSERT INTO ledger.subscription_command VALUES(branch_,actor_,request_,intent,result_);
    RETURN result_;
END
$accept$;


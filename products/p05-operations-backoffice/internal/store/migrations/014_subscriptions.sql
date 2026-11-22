-- Enrollment, schedule and consent are separate from payment authority.
ALTER TABLE ledger.sale DROP CONSTRAINT sale_origin_check;
ALTER TABLE ledger.sale ADD CHECK (origin IN ('menu','pos','subscription'));
ALTER TABLE ledger.order_checkout ALTER COLUMN menu_version DROP NOT NULL;

CREATE TABLE ledger.subscription_offer (
    id uuid PRIMARY KEY,
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    label text NOT NULL CHECK (length(label) BETWEEN 1 AND 120),
    terms_version text NOT NULL CHECK (length(terms_version) BETWEEN 1 AND 40),
    terms_digest bytea NOT NULL CHECK (octet_length(terms_digest)=32),
    cadence text NOT NULL CHECK (cadence IN ('weekly','monthly')),
    timezone text NOT NULL DEFAULT 'Asia/Seoul',
    anchor_day integer NOT NULL DEFAULT 1 CHECK (anchor_day BETWEEN 1 AND 31),
    due_days integer NOT NULL DEFAULT 7 CHECK (due_days BETWEEN 1 AND 28),
    asset_ref text NOT NULL CHECK (asset_ref ~ '^0x[0-9a-f]{40}$'),
    amount ledger.units NOT NULL CHECK (amount>0),
    series_ref uuid NOT NULL DEFAULT gen_random_uuid(),
    enabled boolean NOT NULL DEFAULT true,
    UNIQUE(branch_id,id),
    UNIQUE(branch_id,series_ref,terms_version)
);
CREATE TABLE ledger.subscription (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    customer_scope uuid NOT NULL,
    offer_id uuid NOT NULL,
    terms_version text NOT NULL,
    terms_digest bytea NOT NULL CHECK (octet_length(terms_digest)=32),
    accepted_at timestamptz NOT NULL,
    cadence text NOT NULL CHECK (cadence IN ('weekly','monthly')),
    timezone text NOT NULL,
    anchor_day integer NOT NULL CHECK (anchor_day BETWEEN 1 AND 31),
    due_days integer NOT NULL CHECK (due_days BETWEEN 1 AND 28),
    asset_ref text NOT NULL,
    amount ledger.units NOT NULL CHECK (amount>0),
    next_cycle date,
    next_due_at timestamptz,
    state text NOT NULL DEFAULT 'active' CHECK (state IN ('active','paused','cancelled')),
    cancelled_at timestamptz,
    revision bigint NOT NULL DEFAULT 0 CHECK (revision>=0),
    UNIQUE(branch_id,id),
    FOREIGN KEY(branch_id,offer_id) REFERENCES ledger.subscription_offer(branch_id,id),
    CHECK ((state='cancelled' AND cancelled_at>=accepted_at AND next_cycle IS NULL AND next_due_at IS NULL)
       OR (state IN ('active','paused') AND cancelled_at IS NULL AND next_cycle IS NOT NULL AND next_due_at IS NOT NULL))
);
CREATE UNIQUE INDEX subscription_active_offer ON ledger.subscription(branch_id,customer_scope,offer_id) WHERE state<>'cancelled';
CREATE INDEX subscription_due ON ledger.subscription(next_due_at,id) WHERE state='active';
CREATE TABLE ledger.invoice (
    id uuid PRIMARY KEY,
    branch_id uuid NOT NULL,
    schedule_ref uuid NOT NULL,
    cycle_key date NOT NULL,
    due_at timestamptz NOT NULL,
    sale_id uuid NOT NULL UNIQUE,
    UNIQUE(branch_id,schedule_ref,cycle_key),
    FOREIGN KEY(branch_id,schedule_ref) REFERENCES ledger.subscription(branch_id,id),
    FOREIGN KEY(branch_id,sale_id) REFERENCES ledger.sale(branch_id,id)
);
CREATE TABLE ledger.subscription_command (
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    customer_scope uuid NOT NULL,
    request_key uuid NOT NULL,
    intent_digest bytea NOT NULL CHECK (octet_length(intent_digest)=32),
    subscription_id uuid NOT NULL,
    PRIMARY KEY(branch_id,customer_scope,request_key),
    FOREIGN KEY(branch_id,subscription_id) REFERENCES ledger.subscription(branch_id,id)
);

CREATE FUNCTION ledger.next_subscription_cycle(cycle_ date, cadence_ text, anchor_ integer) RETURNS date
LANGUAGE sql IMMUTABLE SET search_path=pg_catalog AS $next$
    SELECT CASE cadence_ WHEN 'weekly' THEN cycle_+7 ELSE
      (date_trunc('month',cycle_::timestamp)+interval '1 month')::date
      + least(anchor_,extract(day FROM date_trunc('month',cycle_::timestamp)+interval '2 months - 1 day')::integer)-1 END
$next$;

CREATE FUNCTION ledger.accept_subscription(tenant_ uuid,branch_ uuid,actor_ uuid,offer_ uuid,digest_ bytea,request_ uuid)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $accept$
DECLARE
    o ledger.subscription_offer;
    result_ uuid;
    prior ledger.subscription_command;
    first_ date;
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
    IF NOT FOUND OR o.terms_digest IS DISTINCT FROM digest_ OR o.timezone !~ '^(UTC|[A-Za-z_]+(/[-A-Za-z0-9_+]+)+)$'
       OR NOT EXISTS (SELECT FROM pg_timezone_names WHERE name=o.timezone) THEN
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
    first_ := (accepted AT TIME ZONE o.timezone)::date;
    IF o.cadence='monthly' THEN
      first_ := date_trunc('month',first_::timestamp)::date
         + least(o.anchor_day,extract(day FROM date_trunc('month',first_::timestamp)+interval '1 month - 1 day')::integer)-1;
      IF (first_::timestamp AT TIME ZONE o.timezone)<accepted THEN
         first_ := ledger.next_subscription_cycle(first_,o.cadence,o.anchor_day);
      END IF;
    END IF;
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

CREATE FUNCTION ledger.make_due_invoice() RETURNS uuid
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
    INSERT INTO ledger.invoice VALUES(invoice_,s.branch_id,s.id,s.next_cycle,due_,sale_);
    next_:=ledger.next_subscription_cycle(s.next_cycle,s.cadence,s.anchor_day);
    UPDATE ledger.subscription SET next_cycle=next_,next_due_at=next_::timestamp AT TIME ZONE s.timezone WHERE id=s.id;
    RETURN invoice_;
END
$issue$;

CREATE FUNCTION ledger.change_subscription(tenant_ uuid,branch_ uuid,actor_ uuid,id_ uuid,expected_ bigint,action_ text,request_ uuid)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $change$
DECLARE
    s ledger.subscription;
    prior ledger.subscription_command;
    intent bytea := sha256(convert_to(jsonb_build_array(action_,id_,expected_)::text,'UTF8'));
    moment timestamptz := clock_timestamp();
    cycle_ date;
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
    SELECT * INTO s FROM ledger.subscription WHERE branch_id=branch_ AND customer_scope=actor_ AND id=id_ FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION 'subscription scope denied' USING ERRCODE='42501'; END IF;
    IF s.revision<>expected_ OR s.state='cancelled' THEN RAISE EXCEPTION 'subscription already changed' USING ERRCODE='P0501'; END IF;
    IF action_='cancel' THEN
      UPDATE ledger.subscription SET state='cancelled',cancelled_at=moment,next_due_at=NULL,next_cycle=NULL,revision=revision+1 WHERE id=id_;
      -- UPDATE takes the sale locks used by normal quote commit. An already
      -- issued sale leaves the set when it wins the lock and stays authoritative.
      UPDATE ledger.sale x SET state='closed',revision=x.revision+1
        FROM ledger.invoice i WHERE i.schedule_ref=id_ AND i.sale_id=x.id AND x.state='preview';
    ELSIF action_='pause' AND s.state='active' THEN
      UPDATE ledger.subscription SET state='paused',revision=revision+1 WHERE id=id_;
    ELSIF action_='resume' AND s.state='paused' THEN
      IF NOT EXISTS(SELECT FROM ledger.subscription_offer WHERE id=s.offer_id AND enabled) THEN
        RAISE EXCEPTION 'current terms require fresh consent' USING ERRCODE='P0702';
      END IF;
      cycle_:=s.next_cycle;
      IF cycle_::timestamp AT TIME ZONE s.timezone<moment THEN
        IF s.cadence='weekly' THEN
          cycle_:=cycle_ + greatest(0,((moment AT TIME ZONE s.timezone)::date-cycle_)/7)*7;
        ELSE
          cycle_:=date_trunc('month',moment AT TIME ZONE s.timezone)::date;
          cycle_:=cycle_+least(s.anchor_day,extract(day FROM date_trunc('month',cycle_::timestamp)+interval '1 month - 1 day')::integer)-1;
        END IF;
        IF cycle_::timestamp AT TIME ZONE s.timezone<moment THEN
          cycle_:=ledger.next_subscription_cycle(cycle_,s.cadence,s.anchor_day);
        END IF;
      END IF;
      UPDATE ledger.subscription SET state='active',revision=revision+1,next_cycle=cycle_,next_due_at=cycle_::timestamp AT TIME ZONE s.timezone WHERE id=id_;
    ELSE
      RAISE EXCEPTION 'invalid subscription action' USING ERRCODE='P0501';
    END IF;
    INSERT INTO ledger.subscription_command VALUES(branch_,actor_,request_,intent,id_);
    RETURN id_;
END
$change$;

CREATE FUNCTION ledger.subscription_invoices(tenant_ uuid,branch_ uuid,actor_ uuid)
RETURNS TABLE(id uuid,subscription_id uuid,sale_id uuid,cycle_key date,due_at timestamptz,state text,asset_ref text,amount text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $open$
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code<>'customer' THEN
      RAISE EXCEPTION 'customer scope required' USING ERRCODE='42501';
    END IF;
    RETURN QUERY SELECT i.id,i.schedule_ref,i.sale_id,i.cycle_key,i.due_at,x.state,x.asset_ref,x.amount::text
      FROM ledger.invoice i JOIN ledger.subscription s ON s.id=i.schedule_ref AND s.branch_id=i.branch_id
      JOIN ledger.sale x ON x.id=i.sale_id AND x.branch_id=i.branch_id
      WHERE i.branch_id=branch_ AND s.customer_scope=actor_ ORDER BY i.due_at DESC,i.id LIMIT 100;
END
$open$;

CREATE FUNCTION ledger.subscription_offers(tenant_ uuid,branch_ uuid,actor_ uuid)
RETURNS TABLE(id uuid,series_ref uuid,label text,terms_version text,terms_digest text,cadence text,timezone text,anchor_day integer,due_days integer,asset_ref text,amount text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $offers$
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code<>'customer' THEN
      RAISE EXCEPTION 'customer scope required' USING ERRCODE='42501';
    END IF;
    RETURN QUERY SELECT o.id,o.series_ref,o.label,o.terms_version,encode(o.terms_digest,'hex'),o.cadence,o.timezone,o.anchor_day,o.due_days,o.asset_ref,o.amount::text
      FROM ledger.subscription_offer o WHERE o.branch_id=branch_ AND o.enabled ORDER BY o.id LIMIT 100;
END
$offers$;

CREATE FUNCTION ledger.subscriptions(tenant_ uuid,branch_ uuid,actor_ uuid)
RETURNS TABLE(id uuid,offer_id uuid,series_ref uuid,label text,terms_version text,terms_digest text,cadence text,timezone text,anchor_day integer,due_days integer,
    asset_ref text,amount text,state text,next_due_at timestamptz,revision bigint,needs_consent boolean)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $status$
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code<>'customer' THEN
      RAISE EXCEPTION 'customer scope required' USING ERRCODE='42501';
    END IF;
    RETURN QUERY SELECT s.id,s.offer_id,o.series_ref,o.label,s.terms_version,encode(s.terms_digest,'hex'),s.cadence,s.timezone,s.anchor_day,s.due_days,
      s.asset_ref,s.amount::text,s.state,s.next_due_at,s.revision,NOT o.enabled AND s.state<>'cancelled'
      FROM ledger.subscription s JOIN ledger.subscription_offer o ON o.id=s.offer_id
      WHERE s.branch_id=branch_ AND s.customer_scope=actor_ ORDER BY s.accepted_at DESC,s.id LIMIT 100;
END
$status$;

DO $role$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p05_subscription_worker') THEN
      CREATE ROLE nu54_p05_subscription_worker NOLOGIN NOSUPERUSER NOBYPASSRLS;
    END IF;
END
$role$;
REVOKE ALL ON FUNCTION ledger.next_subscription_cycle(date,text,integer),
    ledger.make_due_invoice(),ledger.accept_subscription(uuid,uuid,uuid,uuid,bytea,uuid),
    ledger.change_subscription(uuid,uuid,uuid,uuid,bigint,text,uuid),ledger.subscription_invoices(uuid,uuid,uuid),
    ledger.subscription_offers(uuid,uuid,uuid),ledger.subscriptions(uuid,uuid,uuid) FROM PUBLIC;
GRANT USAGE ON SCHEMA ledger TO nu54_p05_subscription_worker;
GRANT EXECUTE ON FUNCTION ledger.make_due_invoice() TO nu54_p05_subscription_worker;
GRANT EXECUTE ON FUNCTION ledger.accept_subscription(uuid,uuid,uuid,uuid,bytea,uuid),
    ledger.change_subscription(uuid,uuid,uuid,uuid,bigint,text,uuid),ledger.subscription_invoices(uuid,uuid,uuid),
    ledger.subscription_offers(uuid,uuid,uuid),ledger.subscriptions(uuid,uuid,uuid) TO nu54_p05_commerce;

CREATE FUNCTION ledger.reconsent_subscription(tenant_ uuid,branch_ uuid,actor_ uuid,old_ uuid,expected_ bigint,
    offer_ uuid,digest_ bytea,request_ uuid)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $replace$
DECLARE
    previous ledger.subscription;
    candidate ledger.subscription_offer;
    prior ledger.subscription_command;
    result_ uuid;
    intent bytea := sha256(convert_to(jsonb_build_array('reconsent',old_,expected_,offer_,encode(digest_,'hex'))::text,'UTF8'));
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
    SELECT * INTO candidate FROM ledger.subscription_offer WHERE id=offer_ AND branch_id=branch_ AND enabled FOR SHARE;
    IF NOT FOUND OR candidate.terms_digest IS DISTINCT FROM digest_ THEN
      RAISE EXCEPTION 'replacement terms unavailable' USING ERRCODE='P0702';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch_::text||actor_::text||candidate.series_ref::text,62));
    SELECT * INTO previous FROM ledger.subscription WHERE id=old_ AND branch_id=branch_ AND customer_scope=actor_ FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION 'subscription scope denied' USING ERRCODE='42501'; END IF;
    IF previous.state='cancelled' OR candidate.id=previous.offer_id
       OR candidate.series_ref IS DISTINCT FROM (SELECT series_ref FROM ledger.subscription_offer WHERE id=previous.offer_id) THEN
      RAISE EXCEPTION 'replacement terms unavailable' USING ERRCODE='P0702';
    END IF;
    PERFORM ledger.change_subscription(tenant_,branch_,actor_,old_,expected_,'cancel',gen_random_uuid());
    result_:=ledger.accept_subscription(tenant_,branch_,actor_,offer_,digest_,gen_random_uuid());
    INSERT INTO ledger.subscription_command VALUES(branch_,actor_,request_,intent,result_);
    RETURN result_;
END
$replace$;
REVOKE ALL ON FUNCTION ledger.reconsent_subscription(uuid,uuid,uuid,uuid,bigint,uuid,bytea,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.reconsent_subscription(uuid,uuid,uuid,uuid,bigint,uuid,bytea,uuid) TO nu54_p05_commerce;

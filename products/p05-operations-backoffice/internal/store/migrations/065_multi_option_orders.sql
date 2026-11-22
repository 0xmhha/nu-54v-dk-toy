-- Preserve persisted order bodies and identities while allowing another option of the same SKU.
ALTER TABLE ledger.order_line
    DROP CONSTRAINT order_line_pkey,
    ADD CONSTRAINT order_line_pkey PRIMARY KEY (sale_id,sku,option_key);
ALTER TABLE ledger.catalog_option_hold
    DROP CONSTRAINT catalog_option_hold_pkey,
    ADD CONSTRAINT catalog_option_hold_pkey PRIMARY KEY (sale_id,sku,option_key);

CREATE FUNCTION ledger.canonical_order_lines(lines jsonb) RETURNS jsonb
LANGUAGE plpgsql IMMUTABLE SET search_path=pg_catalog,pg_temp AS $$
DECLARE result jsonb;
BEGIN
    IF jsonb_typeof(lines) IS DISTINCT FROM 'array' THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid order lines';
    END IF;
    IF jsonb_array_length(lines) NOT BETWEEN 1 AND 20 OR EXISTS (
        SELECT FROM jsonb_array_elements(lines) item
        WHERE jsonb_typeof(item) IS DISTINCT FROM 'object'
    ) THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid order lines';
    END IF;
    IF EXISTS (
        SELECT FROM jsonb_to_recordset(lines) x(sku text,option_key text,quantity bigint)
        WHERE sku IS NULL OR sku !~ '^[a-z0-9][a-z0-9_-]{0,31}$'
           OR option_key IS NULL OR option_key !~ '^[a-z0-9][a-z0-9_-]{0,31}$'
           OR quantity IS NULL OR quantity NOT BETWEEN 1 AND 1000
    ) OR EXISTS (
        SELECT FROM jsonb_to_recordset(lines) x(sku text,option_key text)
        GROUP BY sku,option_key HAVING count(*)>1
    ) THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid order lines';
    END IF;
    SELECT jsonb_agg(item ORDER BY (item->>'sku') COLLATE "C",(item->>'option_key') COLLATE "C")
      INTO result FROM jsonb_array_elements(lines) item;
    RETURN result;
END $$;
REVOKE ALL ON FUNCTION ledger.canonical_order_lines(jsonb) FROM PUBLIC;

-- The original bytes are also checked for old direct callers that stored an
-- unsorted array before 065. New writes always use the canonical array.
CREATE OR REPLACE FUNCTION ledger.reserve_order(tenant uuid, branch uuid, actor uuid, request_key uuid, origin_ text,
    origin_ref_ uuid, menu_ bigint, asset_ text, lines jsonb, role_ text)
RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
DECLARE
    actor_grant ledger.commerce_grant := ledger.commerce_allowed(tenant,branch,actor);
    policy ledger.command_rule;
    published ledger.menu_release;
    prior ledger.command_receipt;
    existing ledger.sale;
    checkout ledger.order_checkout;
    hold_until timestamptz;
    order_ uuid;
    intent bytea;
    supplied_intent bytea;
    normalized jsonb;
    amount_ numeric;
    priced integer;
    held record;
BEGIN
    IF role_ IS DISTINCT FROM actor_grant.role_code OR origin_ NOT IN ('pos','menu') OR (actor_grant.role_code='customer' AND origin_<>'menu') OR
       (actor_grant.role_code='terminal' AND actor_grant.terminal_mode<>origin_) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied';
    END IF;
    IF request_key IS NULL OR origin_ref_ IS NULL OR lines IS NULL OR asset_ IS NULL OR asset_ !~ '^0x[0-9a-f]{40}$' THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid order';
    END IF;
    normalized := ledger.canonical_order_lines(lines);
    supplied_intent := sha256(convert_to(jsonb_build_array('reserve-order-v2',tenant,branch,origin_,origin_ref_,menu_,asset_,lines)::text,'UTF8'));
    intent := sha256(convert_to(jsonb_build_array('reserve-order-v2',tenant,branch,origin_,origin_ref_,menu_,asset_,normalized)::text,'UTF8'));
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||actor::text||'create_sale'||request_key::text,0));
    SELECT * INTO prior FROM ledger.command_receipt r WHERE r.branch_id=branch AND r.scope_ref=actor
      AND r.operation='create_sale' AND r.request_key=reserve_order.request_key;
    IF FOUND THEN
        IF prior.intent_digest NOT IN (intent,supplied_intent) THEN RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='request conflict'; END IF;
        IF prior.retire_after<=statement_timestamp() THEN RAISE EXCEPTION USING ERRCODE='P0502', MESSAGE='request expired'; END IF;
        RETURN prior.result_ref;
    END IF;

    -- The same POS origin owns exactly one sale across QR/BLE and client retries.
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||origin_||origin_ref_::text,1));
    SELECT * INTO existing FROM ledger.sale s WHERE s.branch_id=branch AND s.origin=origin_ AND s.origin_ref=origin_ref_ FOR UPDATE;
    IF FOUND THEN
        SELECT * INTO checkout FROM ledger.order_checkout c
          WHERE c.sale_id=existing.id AND c.creator=actor AND c.intent_digest IN (intent,supplied_intent);
        IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='request conflict'; END IF;
        IF checkout.expires_at<=clock_timestamp() THEN
            RAISE EXCEPTION USING ERRCODE='P0502', MESSAGE='request expired';
        END IF;
        INSERT INTO ledger.command_receipt
          VALUES (branch,actor,'create_sale',request_key,checkout.intent_digest,existing.id,statement_timestamp(),checkout.expires_at);
        RETURN existing.id;
    END IF;

    -- Coordinate new reservations with an operator publication. Acknowledging a known
    -- request above stays independent of the current pricing version and hours.
    PERFORM pg_advisory_xact_lock_shared(hashtextextended(branch::text||'operator-catalog',0));
    IF EXISTS (SELECT 1 FROM ledger.catalog_pricing p WHERE p.branch_id=branch) AND (
       menu_ IS DISTINCT FROM (SELECT max(m.version) FROM ledger.menu_release m WHERE m.branch_id=branch)
       OR NOT ledger.catalog_is_open(branch,menu_,clock_timestamp())
    ) THEN
        RAISE EXCEPTION USING ERRCODE='P0702', MESSAGE='stale or closed menu';
    END IF;
    SELECT * INTO policy FROM ledger.command_rule c WHERE c.branch_id=branch FOR SHARE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0503', MESSAGE='order policy unavailable'; END IF;
    SELECT * INTO published FROM ledger.menu_release m WHERE m.branch_id=branch AND m.version=menu_
      AND m.published_at<=statement_timestamp() AND m.expires_at>statement_timestamp() FOR SHARE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0702', MESSAGE='stale menu'; END IF;
    hold_until := LEAST(published.expires_at, statement_timestamp()+policy.receipt_window,actor_grant.valid_until);
    SELECT sum(p.unit_price*x.quantity),count(p.sku) INTO amount_,priced
      FROM jsonb_to_recordset(normalized) x(sku text,option_key text,quantity bigint)
      LEFT JOIN ledger.price p ON p.branch_id=branch AND p.version=menu_ AND
        p.sku=x.sku AND p.option_key=x.option_key AND p.asset_ref=asset_;
    IF priced<>jsonb_array_length(normalized) THEN
        RAISE EXCEPTION USING ERRCODE='P0702', MESSAGE='price version mismatch';
    END IF;
    -- Acquire SKU locks in canonical order and debit the complete SKU demand
    -- exactly once. The ordered line inserts acquire option locks afterwards.
    FOR held IN SELECT x.sku,sum(x.quantity)::bigint AS quantity
        FROM jsonb_to_recordset(normalized) x(sku text,quantity bigint)
        GROUP BY x.sku ORDER BY x.sku COLLATE "C"
    LOOP
        UPDATE ledger.stock s SET available=s.available-held.quantity
         WHERE s.branch_id=branch AND s.sku=held.sku AND s.available>=held.quantity;
        IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0701', MESSAGE='insufficient stock'; END IF;
    END LOOP;
    INSERT INTO ledger.sale(branch_id,origin,origin_ref,asset_ref,amount)
      VALUES (branch,origin_,origin_ref_,asset_,amount_) RETURNING id INTO order_;
    INSERT INTO ledger.order_checkout VALUES (order_,branch,actor,menu_,hold_until,intent);
    INSERT INTO ledger.order_line
      SELECT order_,branch,menu_,p.sku,p.option_key,x.quantity,p.unit_price
      FROM jsonb_to_recordset(normalized) x(sku text,option_key text,quantity bigint)
      JOIN ledger.price p ON p.branch_id=branch AND p.version=menu_ AND p.sku=x.sku AND p.option_key=x.option_key
      ORDER BY p.sku COLLATE "C",p.option_key COLLATE "C";
    INSERT INTO ledger.command_receipt
      VALUES (branch,actor,'create_sale',request_key,intent,order_,statement_timestamp(),hold_until);
    RETURN order_;
END $$;

CREATE OR REPLACE FUNCTION ledger.resume_reserve_order(tenant uuid, branch uuid, actor uuid, key_ uuid, origin_ text,
    origin_ref_ uuid, menu_ bigint, asset_ text, lines jsonb, role_ text)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    grant_ ledger.commerce_grant := ledger.commerce_allowed(tenant,branch,actor);
    prior ledger.command_receipt;
    intent bytea;
    normalized_intent bytea;
BEGIN
    IF grant_.role_code IS DISTINCT FROM role_ OR key_ IS NULL OR lines IS NULL THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied';
    END IF;
    SELECT * INTO prior FROM ledger.command_receipt r
      WHERE r.branch_id=branch AND r.scope_ref=actor AND r.operation='create_sale' AND r.request_key=key_;
    IF NOT FOUND THEN RETURN NULL; END IF;
    intent := sha256(convert_to(jsonb_build_array('reserve-order-v2',tenant,branch,origin_,origin_ref_,menu_,asset_,lines)::text,'UTF8'));
    normalized_intent := sha256(convert_to(jsonb_build_array(
        'reserve-order-v2',tenant,branch,origin_,origin_ref_,menu_,asset_,ledger.canonical_order_lines(lines))::text,'UTF8'));
    IF prior.intent_digest NOT IN (intent,normalized_intent) THEN RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='request conflict'; END IF;
    IF prior.retire_after<=statement_timestamp() THEN RAISE EXCEPTION USING ERRCODE='P0502', MESSAGE='request expired'; END IF;
    RETURN prior.result_ref;
END $$;

CREATE FUNCTION ledger.restore_order_stock(branch_ uuid,sale_ uuid) RETURNS void
LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $$
DECLARE held record;
BEGIN
    FOR held IN SELECT l.sku,sum(l.quantity)::bigint AS quantity
        FROM ledger.order_line l WHERE l.sale_id=sale_ AND l.branch_id=branch_
        GROUP BY l.sku ORDER BY l.sku COLLATE "C"
    LOOP
        UPDATE ledger.stock SET available=available+held.quantity
         WHERE branch_id=branch_ AND sku=held.sku;
    END LOOP;
END $$;
REVOKE ALL ON FUNCTION ledger.restore_order_stock(uuid,uuid) FROM PUBLIC;


CREATE OR REPLACE FUNCTION ledger.reclaim_expired_order()
RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
DECLARE
    checkout ledger.order_checkout;
BEGIN
    SELECT c.* INTO checkout FROM ledger.order_checkout c JOIN ledger.sale s ON s.id=c.sale_id
      WHERE c.expires_at<=clock_timestamp() AND s.state='preview'
      ORDER BY c.expires_at,c.sale_id LIMIT 1 FOR UPDATE OF s SKIP LOCKED;
    IF NOT FOUND THEN RETURN false; END IF;
    -- A prepared signature that was never committed was never exposed. The
    -- state change below fences delayed signing and leaves issued orders alone.
    PERFORM ledger.restore_order_stock(checkout.branch_id,checkout.sale_id);
    UPDATE ledger.sale SET state='closed',revision=revision+1 WHERE id=checkout.sale_id;
    RETURN true;
END $$;

CREATE OR REPLACE FUNCTION ledger.close_unpaid(sale_ uuid, quote_ bytea, evidence_ uuid, post_expiry timestamptz)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $unpaid$
DECLARE
    s ledger.sale;
    q ledger.quote_work;
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
    PERFORM ledger.restore_order_stock(s.branch_id,sale_);
    INSERT INTO ledger.sale_resolution(sale_id,evidence_id) VALUES(sale_,evidence_);
    UPDATE ledger.sale SET state='closed',revision=revision+1 WHERE id=sale_;
    RETURN true;
END
$unpaid$;

CREATE OR REPLACE FUNCTION ledger.restore_catalog_option() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE held record;
BEGIN
    IF OLD.state<>'closed' AND NEW.state='closed' THEN
        FOR held IN
            WITH released AS (DELETE FROM ledger.catalog_option_hold h WHERE h.sale_id=NEW.id
                             RETURNING h.branch_id,h.sku,h.option_key,h.quantity)
            SELECT * FROM released ORDER BY sku COLLATE "C",option_key COLLATE "C"
        LOOP
            UPDATE ledger.catalog_option_stock s SET available=s.available+held.quantity
             WHERE (s.branch_id,s.sku,s.option_key)=(held.branch_id,held.sku,held.option_key);
        END LOOP;
    ELSIF NEW.state='paid' THEN
        DELETE FROM ledger.catalog_option_hold h WHERE h.sale_id=NEW.id;
    END IF;
    RETURN NEW;
END $$;


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
      'items',CASE WHEN sale_ IS NOT NULL AND ack.accepted THEN (SELECT jsonb_agg(jsonb_build_object(
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
    WHERE s.branch_id=branch_ AND (sale_ IS NULL OR s.id=sale_) AND (
       (o.sale_id IS NOT NULL AND ack.accepted) OR
       (o.sale_id IS NULL AND include_cafe_ AND role_='operator' AND s.state IN ('checking','paid') AND ledger.kiosk_menu_pickup(s.id))
    ) AND (sale_ IS NOT NULL OR coalesce(k.state,'accepted') NOT IN ('collected','unfulfillable'))
    ORDER BY ack.accepted DESC,coalesce(o.starts_at,paid.included_at,submission.received_at,c.expires_at),s.id LIMIT 32;
END
$kitchen$;

-- Capture the line where the inventory decision happens.
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
        IF NOT FOUND THEN
            RAISE EXCEPTION USING ERRCODE='P0701', MESSAGE='insufficient stock',
                DETAIL=jsonb_build_object('sku',held.sku)::text;
        END IF;
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

CREATE OR REPLACE FUNCTION ledger.reserve_catalog_option() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    UPDATE ledger.catalog_option_stock s SET available=s.available-NEW.quantity
      WHERE (s.branch_id,s.sku,s.option_key)=(NEW.branch_id,NEW.sku,NEW.option_key)
        AND s.available>=NEW.quantity;
    IF FOUND THEN
        INSERT INTO ledger.catalog_option_hold VALUES(NEW.sale_id,NEW.branch_id,NEW.sku,NEW.option_key,NEW.quantity);
    ELSIF EXISTS (SELECT FROM ledger.catalog_option_stock s WHERE (s.branch_id,s.sku,s.option_key)=(NEW.branch_id,NEW.sku,NEW.option_key)) THEN
        RAISE EXCEPTION USING ERRCODE='P0701', MESSAGE='insufficient option stock',
            DETAIL=jsonb_build_object('sku', NEW.sku, 'option_key', NEW.option_key)::text;
    END IF;
    RETURN NEW;
END $$;

CREATE TABLE ledger.menu_reservation_outcome (
    branch_id uuid NOT NULL REFERENCES ledger.branch(id) ON DELETE CASCADE,
    actor_id uuid NOT NULL,
    request_key uuid NOT NULL,
    origin_ref uuid NOT NULL,
    nearby_customer uuid,
    intent_digest bytea NOT NULL CHECK (octet_length(intent_digest)=32),
    sale_id uuid,
    rejection text,
    unavailable jsonb NOT NULL DEFAULT '[]',
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    retire_after timestamptz,
    PRIMARY KEY (branch_id, actor_id, request_key),
    UNIQUE (branch_id, sale_id),
    FOREIGN KEY (branch_id,sale_id) REFERENCES ledger.sale(branch_id,id) ON DELETE CASCADE,
    CHECK ((sale_id IS NOT NULL AND rejection IS NULL AND retire_after IS NULL) OR
           (sale_id IS NULL AND retire_after IS NOT NULL AND rejection IS NOT NULL
              AND rejection IN ('OUT_OF_STOCK','CATALOG_CHANGED','EXPIRED')))
);
CREATE INDEX ON ledger.menu_reservation_outcome (branch_id, origin_ref);
CREATE INDEX ON ledger.menu_reservation_outcome (retire_after) WHERE rejection IS NOT NULL;
REVOKE ALL ON ledger.menu_reservation_outcome FROM PUBLIC;

CREATE TABLE ledger.menu_reservation_replay_fence (
    branch_id uuid NOT NULL REFERENCES ledger.branch(id) ON DELETE CASCADE,
    request_digest bytea NOT NULL CHECK (octet_length(request_digest)=32),
    origin_digest bytea NOT NULL CHECK (octet_length(origin_digest)=32),
    PRIMARY KEY (branch_id,request_digest)
);
CREATE INDEX ON ledger.menu_reservation_replay_fence (branch_id,origin_digest);
REVOKE ALL ON ledger.menu_reservation_replay_fence FROM PUBLIC;

CREATE FUNCTION ledger.reserve_menu_with_outcome(tenant uuid, branch uuid, actor uuid, key_ uuid,
    origin_ uuid, menu_ bigint, asset_ text, lines jsonb, role_ text, customer_ uuid, deadline_ bigint)
RETURNS TABLE(sale_ref uuid,rejection_code text,unavailable_lines jsonb)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    g ledger.commerce_grant := ledger.commerce_allowed(tenant,branch,actor);
    existing ledger.menu_reservation_outcome;
    normalized jsonb := ledger.canonical_order_lines(lines);
    intent bytea := sha256(convert_to(jsonb_build_array(
        'menu-reservation-v1',tenant,branch,actor,origin_,menu_,asset_,normalized,customer_,deadline_)::text,'UTF8'));
    failure_detail text;
    failed_line jsonb;
    sale_ uuid;
    code_ text;
    retain_ timestamptz;
    rejected_lines jsonb := '[]';
BEGIN
    IF g.role_code IS DISTINCT FROM role_ OR (customer_ IS NOT NULL AND
       (role_<>'terminal' OR deadline_ IS NULL OR
        (ledger.commerce_allowed(tenant,branch,customer_)).role_code IS DISTINCT FROM 'customer')) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='private menu scope denied';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||actor::text||'create_sale'||key_::text,0));
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||'menu'||origin_::text,1));
    SELECT * INTO existing FROM ledger.menu_reservation_outcome r
      WHERE r.branch_id=branch AND r.actor_id=actor AND r.request_key=key_;
    IF FOUND THEN
        IF existing.intent_digest<>intent THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='request conflict';
        END IF;
        IF existing.rejection IS NOT NULL AND existing.retire_after<=clock_timestamp() THEN
            RAISE EXCEPTION USING ERRCODE='P0502', MESSAGE='reservation receipt expired';
        END IF;
        IF existing.sale_id IS NOT NULL AND NOT EXISTS (SELECT FROM ledger.order_checkout c
            WHERE c.branch_id=branch AND c.sale_id=existing.sale_id AND c.expires_at>clock_timestamp()) THEN
            RAISE EXCEPTION USING ERRCODE='P0502', MESSAGE='reservation checkout expired';
        END IF;
        RETURN QUERY SELECT existing.sale_id,existing.rejection,existing.unavailable;
        RETURN;
    END IF;
    IF EXISTS (SELECT FROM ledger.menu_reservation_outcome r WHERE r.branch_id=branch AND r.origin_ref=origin_
        AND (customer_ IS NOT NULL OR r.nearby_customer IS NOT NULL OR r.actor_id<>actor OR r.sale_id IS NOT NULL)) THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='origin conflict';
    END IF;
    IF EXISTS (SELECT FROM ledger.menu_reservation_replay_fence f WHERE f.branch_id=branch AND
        (f.request_digest=sha256(convert_to(actor::text||key_::text,'UTF8')) OR
         f.origin_digest=sha256(convert_to(origin_::text,'UTF8')))) THEN
        RAISE EXCEPTION USING ERRCODE='P0502', MESSAGE='reservation receipt expired';
    END IF;
    -- Legacy successes can only be recovered with their original authenticated key.
    sale_ := ledger.resume_reserve_order(tenant,branch,actor,key_,'menu',origin_,menu_,asset_,normalized,role_);
    IF sale_ IS NOT NULL AND customer_ IS NOT NULL AND NOT EXISTS (
        SELECT FROM ledger.checkout_claim c WHERE c.branch_id=branch AND c.sale_id=sale_
            AND c.actor_id=customer_ AND c.expires_at>clock_timestamp()) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='legacy order has no private customer binding';
    END IF;
    IF sale_ IS NULL THEN
        IF EXISTS (SELECT FROM ledger.sale s WHERE s.branch_id=branch AND s.origin='menu' AND s.origin_ref=origin_) THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='origin conflict';
        END IF;
        IF deadline_>extract(epoch FROM clock_timestamp())+300 THEN
            RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='reservation deadline';
        ELSIF deadline_<=extract(epoch FROM clock_timestamp()) THEN
            code_ := 'EXPIRED';
        ELSE
            BEGIN
                sale_ := ledger.reserve_order(tenant,branch,actor,key_,'menu',origin_,menu_,asset_,normalized,role_);
            EXCEPTION WHEN SQLSTATE 'P0701' THEN
                GET STACKED DIAGNOSTICS failure_detail = PG_EXCEPTION_DETAIL;
                failed_line := failure_detail::jsonb;
                SELECT coalesce(jsonb_agg(line ORDER BY line->>'sku',line->>'option_key'),'[]') INTO rejected_lines
                  FROM jsonb_array_elements(normalized) line WHERE line->>'sku'=failed_line->>'sku'
                    AND (failed_line->>'option_key' IS NULL OR line->>'option_key'=failed_line->>'option_key');
                IF rejected_lines='[]'::jsonb THEN RAISE; END IF;
                code_ := 'OUT_OF_STOCK';
            WHEN SQLSTATE 'P0702' THEN
                code_ := 'CATALOG_CHANGED';
            END;
        END IF;
    END IF;
    IF code_ IS NOT NULL THEN
        SELECT clock_timestamp()+greatest(receipt_window,interval '5 minutes') INTO STRICT retain_
          FROM ledger.command_rule WHERE branch_id=branch;
    END IF;
    INSERT INTO ledger.menu_reservation_outcome
        (branch_id,actor_id,request_key,origin_ref,nearby_customer,intent_digest,sale_id,rejection,unavailable,retire_after)
      VALUES (branch,actor,key_,origin_,customer_,intent,sale_,code_,rejected_lines,retain_);
    RETURN QUERY SELECT sale_,code_,rejected_lines;
END $$;

CREATE FUNCTION ledger.nearby_menu_outcome(tenant uuid, branch uuid, actor uuid, customer_ uuid,
    target uuid, by_order boolean)
RETURNS TABLE(sale_ref uuid,rejection_code text,unavailable_lines jsonb)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF (ledger.commerce_allowed(tenant,branch,actor)).role_code IS DISTINCT FROM 'terminal' OR
       (ledger.commerce_allowed(tenant,branch,customer_)).role_code IS DISTINCT FROM 'customer' THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='private menu scope denied';
    END IF;
    RETURN QUERY SELECT r.sale_id,
        CASE WHEN r.rejection IS NOT NULL AND r.retire_after<=clock_timestamp() THEN 'EXPIRED' ELSE r.rejection END,
        CASE WHEN r.rejection IS NOT NULL AND r.retire_after<=clock_timestamp() THEN '[]'::jsonb ELSE r.unavailable END
      FROM ledger.menu_reservation_outcome r
      WHERE r.branch_id=branch AND r.actor_id=actor AND r.nearby_customer=customer_
        AND (CASE WHEN by_order THEN r.sale_id ELSE r.request_key END)=target;
END $$;

-- Reduce expired rejections and deleted commercial orders to unlinkable replay fences.
CREATE FUNCTION ledger.fence_retired_menu_reservation() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF EXISTS (SELECT FROM ledger.branch WHERE id=OLD.branch_id) THEN
        INSERT INTO ledger.menu_reservation_replay_fence
          VALUES (OLD.branch_id,sha256(convert_to(OLD.actor_id::text||OLD.request_key::text,'UTF8')),
            sha256(convert_to(OLD.origin_ref::text,'UTF8'))) ON CONFLICT DO NOTHING;
    END IF;
    RETURN OLD;
END $$;
REVOKE ALL ON FUNCTION ledger.fence_retired_menu_reservation() FROM PUBLIC;
CREATE TRIGGER preserve_menu_replay_fence AFTER DELETE ON ledger.menu_reservation_outcome
    FOR EACH ROW EXECUTE FUNCTION ledger.fence_retired_menu_reservation();

CREATE FUNCTION ledger.retire_menu_reservation_details() RETURNS void
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    DELETE FROM ledger.menu_reservation_outcome outcome WHERE
        (outcome.branch_id,outcome.actor_id,outcome.request_key) IN (
            SELECT branch_id,actor_id,request_key FROM ledger.menu_reservation_outcome
            WHERE rejection IS NOT NULL AND retire_after<=statement_timestamp()
            ORDER BY retire_after LIMIT 120 FOR UPDATE SKIP LOCKED
        );
$$;
REVOKE ALL ON FUNCTION ledger.retire_menu_reservation_details() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.retire_menu_reservation_details() TO nu54_p05_commerce;

REVOKE ALL ON FUNCTION ledger.reserve_menu_with_outcome(uuid,uuid,uuid,uuid,uuid,bigint,text,jsonb,text,uuid,bigint),
    ledger.nearby_menu_outcome(uuid,uuid,uuid,uuid,uuid,boolean) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.reserve_menu_with_outcome(uuid,uuid,uuid,uuid,uuid,bigint,text,jsonb,text,uuid,bigint),
    ledger.nearby_menu_outcome(uuid,uuid,uuid,uuid,uuid,boolean) TO nu54_p05_commerce;

CREATE OR REPLACE FUNCTION ledger.authorize_checkout(tenant uuid, branch uuid, actor uuid, sale_ uuid, customer uuid,
    request_key_ uuid, intent_ bytea, candidate bytea)
RETURNS TABLE(claim_digest bytea, expires_at timestamptz)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF EXISTS (SELECT FROM ledger.menu_reservation_outcome r WHERE r.branch_id=branch AND r.sale_id=sale_
        AND r.nearby_customer IS NOT NULL AND (r.nearby_customer<>customer OR r.actor_id<>actor)) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='private checkout scope denied';
    END IF;
    IF EXISTS (SELECT FROM ledger.pos_order p WHERE p.branch_id=branch AND p.sale_id=sale_)
       AND NOT EXISTS (SELECT FROM ledger.command_receipt r WHERE r.branch_id=branch AND r.scope_ref=actor
           AND r.operation='authorize_checkout' AND r.request_key=request_key_ AND r.result_ref=sale_) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='registered POS requires an approved candidate';
    END IF;
    RETURN QUERY SELECT c.claim_digest,c.expires_at FROM ledger.authorize_checkout_internal(
        tenant,branch,actor,sale_,customer,request_key_,intent_,candidate) c;
END $$;

CREATE FUNCTION ledger.fence_private_menu_claim() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF EXISTS (SELECT FROM ledger.menu_reservation_outcome r
        WHERE r.branch_id=NEW.branch_id AND r.sale_id=NEW.sale_id AND r.nearby_customer IS NOT NULL
          AND r.nearby_customer IS DISTINCT FROM NEW.actor_id) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='private checkout scope denied';
    END IF;
    RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION ledger.fence_private_menu_claim() FROM PUBLIC;
CREATE TRIGGER enforce_private_menu_claim BEFORE INSERT OR UPDATE ON ledger.checkout_claim
    FOR EACH ROW EXECUTE FUNCTION ledger.fence_private_menu_claim();

CREATE FUNCTION ledger.fence_private_menu_cancellation() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF EXISTS (SELECT FROM ledger.menu_reservation_outcome r WHERE r.branch_id=NEW.branch_id AND r.nearby_customer IS NOT NULL
        AND (r.origin_ref=NEW.session_ref OR (r.actor_id=NEW.terminal_ref AND r.request_key=NEW.reserve_key))) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='private order cannot be closed by a public session';
    END IF;
    RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION ledger.fence_private_menu_cancellation() FROM PUBLIC;
CREATE TRIGGER protect_private_menu_from_public_cancellation BEFORE INSERT ON ledger.public_menu_cancellation
    FOR EACH ROW EXECUTE FUNCTION ledger.fence_private_menu_cancellation();

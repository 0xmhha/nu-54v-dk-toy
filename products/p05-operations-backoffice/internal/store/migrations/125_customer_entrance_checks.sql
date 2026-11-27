-- Checks of the customer entrance that came out of its review. No table or column is added.

-- Before the cart of a proven account holds stock, opsd reads that account's balance from the chain and compares it
-- with what the cart costs (owner decision). This gives opsd the two values it needs for that and writes nothing:
-- the account behind the customer subject, and the amount of the cart in base units of the asset, priced as
-- ledger.reserve_order prices it.
--   payer_ NULL: the subject is not a proven account (a store-issued customer); there is no balance check.
--   amount_ NULL: the reservation already answered this request key, or a line has no price in this menu version
--     and asset; the reservation gives its own answer to both.
CREATE FUNCTION ledger.customer_hold_check(tenant uuid, branch uuid, actor uuid, key_ uuid, menu_ bigint, asset_ text, lines jsonb)
RETURNS TABLE(payer_ bytea, amount_ numeric)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $check$
DECLARE
    g ledger.commerce_grant := ledger.commerce_allowed(tenant, branch, actor);
    normalized jsonb;
    account bytea;
    total numeric;
    priced integer;
BEGIN
    IF g.role_code IS DISTINCT FROM 'customer' THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied';
    END IF;
    SELECT a.payer INTO account FROM ledger.customer_account a WHERE a.actor_id = actor;
    IF account IS NULL
       OR EXISTS (SELECT FROM ledger.menu_reservation_outcome r
                  WHERE r.branch_id = branch AND r.actor_id = actor AND r.request_key = key_)
       OR EXISTS (SELECT FROM ledger.command_receipt r
                  WHERE r.branch_id = branch AND r.scope_ref = actor AND r.operation = 'create_sale' AND r.request_key = key_) THEN
        RETURN QUERY SELECT account, NULL::numeric;
        RETURN;
    END IF;
    normalized := ledger.canonical_order_lines(lines);
    SELECT sum(p.unit_price * x.quantity), count(p.sku) INTO total, priced
      FROM jsonb_to_recordset(normalized) x(sku text, option_key text, quantity bigint)
      LEFT JOIN ledger.price p ON p.branch_id = branch AND p.version = menu_
        AND p.sku = x.sku AND p.option_key = x.option_key AND p.asset_ref = asset_;
    IF priced IS DISTINCT FROM jsonb_array_length(normalized) THEN
        total := NULL;
    END IF;
    RETURN QUERY SELECT account, total;
END
$check$;

REVOKE ALL ON FUNCTION ledger.customer_hold_check(uuid, uuid, uuid, uuid, bigint, text, jsonb) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.customer_hold_check(uuid, uuid, uuid, uuid, bigint, text, jsonb) TO nu54_p05_commerce;

-- A pickup QR is scanned only by a store operator of the operations web who holds the handover authority
-- (pickup_manage), by owner decision: a kiosk or a pickup terminal does not scan it. This is the function of
-- migration 122 with that one rule added before anything else is looked at; the kitchen's own change keeps its two
-- roles (ledger.kiosk_require_pickup).
CREATE OR REPLACE FUNCTION ledger.scan_pickup_token(tenant_ uuid,branch_ uuid,actor_ uuid,role_ text,sale_ uuid,hash_ bytea,
    rescan_seconds_ integer)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $scan$
DECLARE t ledger.pickup_token; k ledger.travel_pickup_progress; revision_ bigint; again boolean:=false;
BEGIN
    IF rescan_seconds_ IS NULL OR rescan_seconds_ NOT BETWEEN 0 AND 600 THEN
        RAISE EXCEPTION 'invalid rescan window' USING ERRCODE='23514';
    END IF;
    IF role_ IS DISTINCT FROM 'operator' THEN
        RAISE EXCEPTION 'a pickup QR is scanned by a store operator' USING ERRCODE='42501';
    END IF;
    PERFORM ledger.kiosk_require_pickup(tenant_,branch_,actor_,role_);
    SELECT * INTO t FROM ledger.pickup_token x WHERE x.token_hash=hash_ FOR UPDATE;
    IF NOT FOUND THEN RETURN jsonb_build_object('outcome','expired'); END IF;
    IF t.branch_id IS DISTINCT FROM branch_ THEN RETURN jsonb_build_object('outcome','other_store'); END IF;
    IF t.sale_id IS DISTINCT FROM sale_ THEN RETURN jsonb_build_object('outcome','other_order'); END IF;
    IF t.used_at IS NOT NULL THEN
        SELECT * INTO k FROM ledger.travel_pickup_progress p WHERE p.sale_id=t.sale_id;
        IF t.used_by IS DISTINCT FROM actor_ OR t.used_at<=clock_timestamp()-make_interval(secs=>rescan_seconds_)
           OR k.state IS DISTINCT FROM 'collected' THEN
            RETURN jsonb_build_object('outcome','already_used','recordedAt',floor(extract(epoch FROM t.used_at))::bigint);
        END IF;
        again:=true;
        revision_:=k.revision;
    ELSE
        IF t.expires_at<=clock_timestamp() THEN RETURN jsonb_build_object('outcome','expired'); END IF;
        SELECT * INTO k FROM ledger.travel_pickup_progress p WHERE p.sale_id=t.sale_id FOR UPDATE;
        IF k.state IS DISTINCT FROM 'ready' THEN RETURN jsonb_build_object('outcome','not_ready'); END IF;
        UPDATE ledger.pickup_token SET used_at=clock_timestamp(),used_by=actor_ WHERE token_hash=hash_ RETURNING * INTO t;
        revision_:=ledger.kiosk_pickup_change(tenant_,branch_,actor_,t.sale_id,encode(substring(hash_ FROM 1 FOR 16),'hex')::uuid,
            k.revision,'collected',role_);
    END IF;
    -- Lines read like the kitchen queue and the order page: the option key, and its name when the menu gave one.
    RETURN jsonb_build_object('outcome','ok','reply',jsonb_build_object('saleRef',t.sale_id,'revision',revision_,
        'pickupNo',(SELECT p.number FROM ledger.pickup p WHERE p.sale_id=t.sale_id),
        'recordedAt',floor(extract(epoch FROM t.used_at))::bigint,
        'items',coalesce((SELECT jsonb_agg(jsonb_strip_nulls(jsonb_build_object('label',x.label,'option',l.option_key,
                'optionLabel',ledger.order_option_label(l.branch_id,l.menu_version,l.sku,l.option_key),'quantity',l.quantity))
            ORDER BY l.sku COLLATE "C",l.option_key COLLATE "C")
            FROM ledger.order_line l JOIN ledger.price x ON
              (x.branch_id,x.version,x.sku,x.option_key)=(l.branch_id,l.menu_version,l.sku,l.option_key)
            WHERE l.sale_id=t.sale_id),'[]'::jsonb))
        ||CASE WHEN again THEN jsonb_build_object('alreadyRecorded',true) ELSE '{}'::jsonb END);
END
$scan$;

-- A pickup proof for an order that was already handed over says so. Migration 122 answered it not_ready, like an
-- order still being made, so the app could not tell "wait" from "you already have it". This is the function of 122
-- with one outcome added:
--   collected (pickup only): the order was handed over; expires_ carries the time the handover was recorded, since
--     this outcome has no expiry to tell. No pickup token is made.
-- The other outcomes are unchanged: ok, mismatch (the account is not the one the purpose requires) and not_ready.
CREATE OR REPLACE FUNCTION ledger.prove_customer_account(hash_ bytea,payer_ bytea,actor_ uuid,generation_ bigint,token_ bytea,
    checkout_seconds_ integer,order_seconds_ integer,pickup_seconds_ integer)
RETURNS TABLE(outcome_ text,purpose_ text,branch_ uuid,sale_ uuid,tenant_ uuid,expires_ timestamptz)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $prove$
DECLARE c ledger.customer_challenge; known ledger.customer_account; tenant uuid; expiry timestamptz; prior ledger.payer_enrollment;
    handed timestamptz;
BEGIN
    IF octet_length(payer_) IS DISTINCT FROM 20 OR octet_length(token_) IS DISTINCT FROM 32 OR actor_ IS NULL THEN
        RAISE EXCEPTION 'invalid proof' USING ERRCODE='23514';
    END IF;
    UPDATE ledger.customer_challenge x SET used_at=clock_timestamp()
        WHERE x.challenge_hash=hash_ AND x.used_at IS NULL AND x.expires_at>clock_timestamp() RETURNING x.* INTO c;
    IF NOT FOUND THEN RAISE EXCEPTION 'challenge unavailable' USING ERRCODE='P0502'; END IF;
    SELECT b.tenant_ref INTO STRICT tenant FROM ledger.branch b WHERE b.id=c.branch_id;

    IF c.purpose='order' AND NOT EXISTS (SELECT FROM ledger.sale s JOIN ledger.quote_work q ON q.sale_id=s.id
        WHERE s.id=c.sale_id AND s.branch_id=c.branch_id AND s.state IN ('checking','paid') AND q.signed_digest IS NOT NULL AND q.payer=payer_)
    THEN
        RETURN QUERY SELECT 'mismatch',c.purpose,c.branch_id,c.sale_id,tenant,NULL::timestamptz; RETURN;
    END IF;
    IF c.purpose='pickup' THEN
        IF NOT EXISTS (SELECT FROM ledger.sale s JOIN ledger.payment p ON p.sale_id=s.id JOIN ledger.quote_work q ON q.sale_id=s.id
            WHERE s.id=c.sale_id AND s.branch_id=c.branch_id AND s.state='paid' AND q.payer=payer_
              AND p.payer='0x'||encode(payer_,'hex')) THEN
            RETURN QUERY SELECT 'mismatch',c.purpose,c.branch_id,c.sale_id,tenant,NULL::timestamptz; RETURN;
        END IF;
        -- Only the account that paid learns that its order was handed over, and when.
        SELECT k.updated_at INTO handed FROM ledger.travel_pickup_progress k WHERE k.sale_id=c.sale_id AND k.state='collected';
        IF FOUND THEN
            RETURN QUERY SELECT 'collected',c.purpose,c.branch_id,c.sale_id,tenant,handed; RETURN;
        END IF;
        IF NOT EXISTS (SELECT FROM ledger.travel_pickup_progress k WHERE k.sale_id=c.sale_id AND k.state='ready') THEN
            RETURN QUERY SELECT 'not_ready',c.purpose,c.branch_id,c.sale_id,tenant,NULL::timestamptz; RETURN;
        END IF;
        expiry:=clock_timestamp()+make_interval(secs=>pickup_seconds_);
        INSERT INTO ledger.pickup_token(token_hash,branch_id,sale_id,expires_at) VALUES(token_,c.branch_id,c.sale_id,expiry);
        RETURN QUERY SELECT 'ok',c.purpose,c.branch_id,c.sale_id,tenant,expiry; RETURN;
    END IF;

    -- The derived subject belongs to this account only, and never to a store-issued grant.
    SELECT * INTO known FROM ledger.customer_account a WHERE a.actor_id=actor_ OR a.payer=payer_ FOR UPDATE;
    IF FOUND AND (known.actor_id,known.payer) IS DISTINCT FROM (actor_,payer_) THEN
        RAISE EXCEPTION 'customer subject collision' USING ERRCODE='42501';
    END IF;
    IF NOT FOUND THEN
        IF EXISTS (SELECT FROM ledger.commerce_grant g WHERE g.actor_id=actor_) THEN
            RAISE EXCEPTION 'customer subject collision' USING ERRCODE='42501';
        END IF;
        INSERT INTO ledger.customer_account(actor_id,payer) VALUES(actor_,payer_);
    END IF;

    expiry:=clock_timestamp()+make_interval(secs=>CASE c.purpose WHEN 'checkout' THEN checkout_seconds_ ELSE order_seconds_ END);
    INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,valid_until) VALUES(c.branch_id,actor_,'customer',expiry)
        ON CONFLICT (branch_id,actor_id) DO UPDATE SET valid_until=greatest(ledger.commerce_grant.valid_until,excluded.valid_until)
        WHERE ledger.commerce_grant.role_code='customer';
    IF NOT FOUND THEN RAISE EXCEPTION 'customer subject collision' USING ERRCODE='42501'; END IF;
    IF c.purpose='checkout' THEN
        IF generation_ IS NULL OR generation_<=0 THEN RAISE EXCEPTION 'board generation required' USING ERRCODE='23514'; END IF;
        SELECT * INTO prior FROM ledger.payer_enrollment e WHERE (e.branch_id,e.actor_id)=(c.branch_id,actor_) FOR UPDATE;
        IF FOUND AND prior.payer_generation<>generation_ AND ledger.payer_has_open_order(payer_) THEN
            RAISE EXCEPTION 'the account still has an order' USING ERRCODE='P1060';
        END IF;
        INSERT INTO ledger.payer_enrollment VALUES(c.branch_id,actor_,payer_,generation_,1,expiry)
            ON CONFLICT(branch_id,actor_id) DO UPDATE SET payer_generation=excluded.payer_generation,
                expires_at=greatest(ledger.payer_enrollment.expires_at,excluded.expires_at);
    END IF;
    INSERT INTO ledger.customer_session(token_hash,branch_id,actor_id,purpose,sale_id,expires_at)
        VALUES(token_,c.branch_id,actor_,c.purpose,c.sale_id,expiry);
    RETURN QUERY SELECT 'ok',c.purpose,c.branch_id,c.sale_id,tenant,expiry;
END
$prove$;

-- A proven account takes back the cart it has not paid, so that changing the cart does not wait for the hold to run
-- out (an account holds one unpaid cart at a time, migration 122). It is what ledger.reclaim_expired_order does when
-- the hold runs out, done early by the cart's own customer, and under the same condition: the order is still a
-- preview, so no quote of it was ever issued and nothing that could be paid exists. Its stock comes back and the
-- order closes. Outcomes:
--   cancelled     the hold is released now, or the order was already closed without a payment
--   quote_issued  a quote was issued: the order ends with that quote, as before
--   paid          a payment arrived or is being checked: paid orders are not cancelled here
-- An order of another account, of another branch, or of a store-issued customer is no order at all (42501).
CREATE FUNCTION ledger.cancel_customer_hold(tenant uuid, branch uuid, actor uuid, sale_ uuid) RETURNS text
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $cancel$
DECLARE
    g ledger.commerce_grant := ledger.commerce_allowed(tenant, branch, actor);
    s ledger.sale;
BEGIN
    IF g.role_code IS DISTINCT FROM 'customer' OR NOT EXISTS (SELECT FROM ledger.customer_account a WHERE a.actor_id = actor) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied';
    END IF;
    SELECT x.* INTO s FROM ledger.sale x JOIN ledger.order_checkout c ON c.sale_id = x.id
      WHERE x.id = sale_ AND x.branch_id = branch AND x.origin = 'menu' AND c.creator = actor FOR UPDATE OF x;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied';
    END IF;
    IF s.state = 'preview' THEN
        PERFORM ledger.restore_order_stock(branch, s.id);
        UPDATE ledger.sale SET state='closed', revision=revision+1 WHERE id = s.id;
        RETURN 'cancelled';
    END IF;
    IF s.state = 'issued' THEN
        RETURN 'quote_issued';
    END IF;
    IF s.state = 'closed' AND NOT EXISTS (SELECT FROM ledger.payment p WHERE p.sale_id = s.id) THEN
        RETURN 'cancelled';
    END IF;
    RETURN 'paid';
END
$cancel$;

REVOKE ALL ON FUNCTION ledger.cancel_customer_hold(uuid, uuid, uuid, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.cancel_customer_hold(uuid, uuid, uuid, uuid) TO nu54_p05_commerce;

-- The three functions that read one page of a menu for a customer return exactly one row each. A function that
-- returns a table is planned as 1000 rows unless it says otherwise, so the work their callers do for each row (the
-- options and the photos of the page) was costed a thousand times over: about 214,000 for a read that runs for two
-- milliseconds. That estimate is above PostgreSQL's jit_above_cost (100,000), so a server built with JIT compiled
-- the read every time it ran, and the first one in a connection also loaded the compiler. On a busy host that took
-- longer than the 7 seconds a statement of opsd may run, and a customer's first menu read ended as UNKNOWN_RESULT.
-- Saying the truth about the row count brings the estimate to a few hundred.
ALTER FUNCTION ledger.catalog_category_window(uuid, uuid, uuid, integer) ROWS 1;
ALTER FUNCTION ledger.catalog_menu_window(uuid, uuid, uuid, integer) ROWS 1;
ALTER FUNCTION ledger.catalog_menu_photo_window(uuid, uuid, uuid, integer) ROWS 1;

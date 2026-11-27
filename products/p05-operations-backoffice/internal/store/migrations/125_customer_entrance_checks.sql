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

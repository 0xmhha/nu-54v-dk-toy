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

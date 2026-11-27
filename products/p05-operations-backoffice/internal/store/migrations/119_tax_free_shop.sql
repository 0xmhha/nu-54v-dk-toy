-- A store designated as a tax-free shop for foreign visitors (by its district tax office) may tell customers which take-away
-- goods visitors can claim the tax back on. The number is what the store entered from its designation; nothing here asks
-- the tax office. Without it no product may be published as eligible, and apps show no refund notice.

CREATE TABLE ledger.branch_tax_free_shop (
    branch_id uuid PRIMARY KEY REFERENCES ledger.branch,
    designation text NOT NULL CHECK (designation ~ '^[0-9A-Za-z-]{4,40}$'),
    designated_on date NOT NULL,
    refund_operator text NOT NULL CHECK (length(refund_operator) BETWEEN 1 AND 80 AND refund_operator !~ '[[:cntrl:]]'),
    confirmed_by uuid NOT NULL,
    confirmed_at timestamptz NOT NULL
);
REVOKE ALL ON ledger.branch_tax_free_shop FROM PUBLIC, nu54_p05_commerce;

CREATE FUNCTION ledger.set_tax_free_shop(tenant uuid, branch uuid, actor uuid, designation_ text, on_ date, operator_ text)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.require_terminal_operator(tenant, branch, actor);
    IF designation_ IS NULL THEN
        DELETE FROM ledger.branch_tax_free_shop s WHERE s.branch_id = branch;
        IF FOUND THEN
            INSERT INTO ledger.branch_state_log (branch_id, actor, at, change)
            VALUES (branch, actor, clock_timestamp(), jsonb_build_object('taxFreeShop', NULL::text));
        END IF;
        RETURN ledger.operator_order_state(tenant, branch, actor);
    END IF;
    IF on_ IS NULL OR on_ > ledger.branch_today(branch, clock_timestamp()) OR operator_ IS NULL THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'invalid tax-free shop';
    END IF;
    INSERT INTO ledger.branch_tax_free_shop VALUES (branch, designation_, on_, btrim(operator_), actor, clock_timestamp())
    ON CONFLICT (branch_id) DO UPDATE SET designation = EXCLUDED.designation, designated_on = EXCLUDED.designated_on,
        refund_operator = EXCLUDED.refund_operator, confirmed_by = EXCLUDED.confirmed_by, confirmed_at = EXCLUDED.confirmed_at;
    INSERT INTO ledger.branch_state_log (branch_id, actor, at, change)
    VALUES (branch, actor, clock_timestamp(), jsonb_build_object('taxFreeShop', designation_));
    RETURN ledger.operator_order_state(tenant, branch, actor);
END $$;

-- Whether the store may publish a product as eligible now.
CREATE FUNCTION ledger.tax_free_shop_named(tenant uuid, branch uuid, actor uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.require_terminal_operator(tenant, branch, actor);
    RETURN EXISTS (SELECT FROM ledger.branch_tax_free_shop s WHERE s.branch_id = branch);
END $$;

-- The operations page also sees the designation; customers see only its number.
CREATE OR REPLACE FUNCTION ledger.operator_order_state(tenant uuid, branch uuid, actor uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.require_terminal_operator(tenant, branch, actor);
    RETURN jsonb_build_object(
        'mode', coalesce((SELECT s.mode FROM ledger.branch_order_state s WHERE s.branch_id = branch), 'open'),
        'busyMinutes', (SELECT s.busy_minutes FROM ledger.branch_order_state s WHERE s.branch_id = branch),
        'resumeAt', (SELECT s.resume_note FROM ledger.branch_order_state s WHERE s.branch_id = branch),
        'changedBy', (SELECT s.changed_by FROM ledger.branch_order_state s WHERE s.branch_id = branch),
        'changedAt', (SELECT s.changed_at FROM ledger.branch_order_state s WHERE s.branch_id = branch),
        'today', ledger.branch_today(branch, clock_timestamp()),
        'closedDays', coalesce((SELECT jsonb_agg(jsonb_build_object('day', d.day, 'addedBy', d.added_by, 'addedAt', d.added_at) ORDER BY d.day)
            FROM ledger.branch_closed_day d WHERE d.branch_id = branch AND d.day >= ledger.branch_today(branch, clock_timestamp())), '[]'),
        'soldOut', coalesce((SELECT jsonb_agg(jsonb_build_object('sku', s.sku, 'by', s.marked_by, 'at', s.marked_at) ORDER BY s.sku COLLATE "C")
            FROM ledger.menu_sold_out s WHERE s.branch_id = branch), '[]'),
        'taxFreeShop', (SELECT jsonb_build_object('designation', t.designation, 'designatedOn', t.designated_on,
            'refundOperator', t.refund_operator, 'by', t.confirmed_by, 'at', t.confirmed_at)
            FROM ledger.branch_tax_free_shop t WHERE t.branch_id = branch),
        'log', coalesce((SELECT jsonb_agg(jsonb_build_object('by', l.actor, 'at', l.at, 'change', l.change) ORDER BY l.id DESC)
            FROM (SELECT * FROM ledger.branch_state_log l WHERE l.branch_id = branch ORDER BY l.id DESC LIMIT 20) l), '[]'));
END $$;

CREATE OR REPLACE FUNCTION ledger.customer_order_state(tenant uuid, branch uuid, actor uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.commerce_allowed(tenant, branch, actor);
    RETURN jsonb_strip_nulls(jsonb_build_object(
        'mode', CASE WHEN EXISTS (SELECT FROM ledger.branch_closed_day d WHERE d.branch_id = branch
                                    AND d.day = ledger.branch_today(branch, clock_timestamp())) THEN 'closed'
                     ELSE coalesce((SELECT s.mode FROM ledger.branch_order_state s WHERE s.branch_id = branch), 'open') END,
        'busyMinutes', (SELECT s.busy_minutes FROM ledger.branch_order_state s WHERE s.branch_id = branch),
        'resumeAt', (SELECT s.resume_note FROM ledger.branch_order_state s WHERE s.branch_id = branch),
        'taxFreeShop', (SELECT t.designation FROM ledger.branch_tax_free_shop t WHERE t.branch_id = branch),
        'soldOut', (SELECT jsonb_agg(s.sku ORDER BY s.sku COLLATE "C") FROM ledger.menu_sold_out s WHERE s.branch_id = branch)));
END $$;

REVOKE ALL ON FUNCTION ledger.set_tax_free_shop(uuid, uuid, uuid, text, date, text), ledger.tax_free_shop_named(uuid, uuid, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.set_tax_free_shop(uuid, uuid, uuid, text, date, text), ledger.tax_free_shop_named(uuid, uuid, uuid) TO nu54_p05_commerce;

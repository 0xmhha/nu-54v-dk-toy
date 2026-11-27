-- Whether a store takes new orders: open, busy (with the minutes it tells customers) or paused (with the time it says it
-- will take orders again, never applied by itself); its closed days; the products it marked sold out apart from their
-- counts; and a record of who changed which, when. Changed only from the operations page.

CREATE TABLE ledger.branch_order_state (
    branch_id uuid PRIMARY KEY REFERENCES ledger.branch,
    mode text NOT NULL CHECK (mode IN ('open', 'busy', 'paused')),
    busy_minutes integer CHECK (busy_minutes BETWEEN 5 AND 60 AND busy_minutes % 5 = 0),
    resume_note text CHECK (resume_note ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'),
    changed_by uuid NOT NULL,
    changed_at timestamptz NOT NULL,
    CHECK ((mode = 'busy') = (busy_minutes IS NOT NULL)),
    CHECK (mode = 'paused' OR resume_note IS NULL)
);
CREATE TABLE ledger.branch_closed_day (
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    day date NOT NULL,
    added_by uuid NOT NULL,
    added_at timestamptz NOT NULL,
    PRIMARY KEY (branch_id, day)
);
CREATE TABLE ledger.menu_sold_out (
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    sku text NOT NULL CHECK (sku ~ '^[a-z0-9][a-z0-9_-]{0,31}$'),
    marked_by uuid NOT NULL,
    marked_at timestamptz NOT NULL,
    PRIMARY KEY (branch_id, sku)
);
-- Kept as written: nothing updates or deletes a row.
CREATE TABLE ledger.branch_state_log (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    actor uuid NOT NULL,
    at timestamptz NOT NULL,
    change jsonb NOT NULL CHECK (jsonb_typeof(change) = 'object')
);
CREATE INDEX branch_state_log_recent ON ledger.branch_state_log (branch_id, id DESC);
REVOKE ALL ON ledger.branch_order_state, ledger.branch_closed_day, ledger.menu_sold_out, ledger.branch_state_log FROM PUBLIC, nu54_p05_commerce;

-- The day in the store's own time zone.
CREATE FUNCTION ledger.branch_today(branch uuid, at_time timestamptz) RETURNS date
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT (at_time AT TIME ZONE b.timezone)::date FROM ledger.branch b WHERE b.id = branch
$$;
REVOKE ALL ON FUNCTION ledger.branch_today(uuid, timestamptz) FROM PUBLIC;

-- New orders and quotes ask this. A paused store and a closed day refuse them as a closed menu does.
CREATE OR REPLACE FUNCTION ledger.catalog_is_open(branch uuid, menu bigint, at_time timestamptz)
RETURNS boolean LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT coalesce((SELECT (at_time AT TIME ZONE (p.hours->>'zone'))::time >= (p.hours->>'opens')::time
                            AND (at_time AT TIME ZONE (p.hours->>'zone'))::time < (p.hours->>'closes')::time
                     FROM ledger.catalog_pricing p WHERE p.branch_id = branch AND p.version = menu), true)
       AND NOT EXISTS (SELECT FROM ledger.branch_order_state s WHERE s.branch_id = branch AND s.mode = 'paused')
       AND NOT EXISTS (SELECT FROM ledger.branch_closed_day d WHERE d.branch_id = branch AND d.day = ledger.branch_today(branch, at_time));
$$;

-- A product marked sold out refuses a new order line the way a count of zero does, naming the product.
CREATE FUNCTION ledger.refuse_sold_out() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF EXISTS (SELECT FROM ledger.menu_sold_out s WHERE (s.branch_id, s.sku) = (NEW.branch_id, NEW.sku)) THEN
        RAISE EXCEPTION USING ERRCODE = 'P0701', MESSAGE = 'sold out', DETAIL = jsonb_build_object('sku', NEW.sku)::text;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER refuse_sold_out BEFORE INSERT ON ledger.order_line FOR EACH ROW EXECUTE FUNCTION ledger.refuse_sold_out();

-- What the operations page shows: the state, the closed days from today, the sold-out products and the last changes.
CREATE FUNCTION ledger.operator_order_state(tenant uuid, branch uuid, actor uuid) RETURNS jsonb
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
        'log', coalesce((SELECT jsonb_agg(jsonb_build_object('by', l.actor, 'at', l.at, 'change', l.change) ORDER BY l.id DESC)
            FROM (SELECT * FROM ledger.branch_state_log l WHERE l.branch_id = branch ORDER BY l.id DESC LIMIT 20) l), '[]'));
END $$;

CREATE FUNCTION ledger.set_order_state(tenant uuid, branch uuid, actor uuid, mode_ text, minutes integer, resume text) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE before ledger.branch_order_state;
BEGIN
    PERFORM ledger.require_terminal_operator(tenant, branch, actor);
    IF mode_ IS NULL OR mode_ NOT IN ('open', 'busy', 'paused') OR (mode_ = 'busy') <> (minutes IS NOT NULL)
       OR (mode_ <> 'paused' AND resume IS NOT NULL) THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'invalid order state';
    END IF;
    SELECT * INTO before FROM ledger.branch_order_state s WHERE s.branch_id = branch FOR UPDATE;
    IF coalesce(before.mode, 'open') = mode_ AND before.busy_minutes IS NOT DISTINCT FROM minutes
       AND before.resume_note IS NOT DISTINCT FROM resume THEN
        RETURN ledger.operator_order_state(tenant, branch, actor); -- the same again changes nothing
    END IF;
    INSERT INTO ledger.branch_order_state VALUES (branch, mode_, minutes, resume, actor, clock_timestamp())
    ON CONFLICT (branch_id) DO UPDATE SET mode = EXCLUDED.mode, busy_minutes = EXCLUDED.busy_minutes,
        resume_note = EXCLUDED.resume_note, changed_by = EXCLUDED.changed_by, changed_at = EXCLUDED.changed_at;
    INSERT INTO ledger.branch_state_log (branch_id, actor, at, change)
    VALUES (branch, actor, clock_timestamp(), jsonb_strip_nulls(jsonb_build_object('mode', mode_, 'busyMinutes', minutes, 'resumeAt', resume)));
    RETURN ledger.operator_order_state(tenant, branch, actor);
END $$;

CREATE FUNCTION ledger.set_closed_day(tenant uuid, branch uuid, actor uuid, day_ date, closed boolean) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE today date := ledger.branch_today(branch, clock_timestamp());
BEGIN
    PERFORM ledger.require_terminal_operator(tenant, branch, actor);
    IF day_ IS NULL OR closed IS NULL OR day_ < today OR day_ > today + 366 THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'invalid closed day';
    END IF;
    IF closed THEN
        INSERT INTO ledger.branch_closed_day VALUES (branch, day_, actor, clock_timestamp()) ON CONFLICT DO NOTHING;
    ELSE
        DELETE FROM ledger.branch_closed_day d WHERE (d.branch_id, d.day) = (branch, day_);
    END IF;
    IF FOUND THEN
        INSERT INTO ledger.branch_state_log (branch_id, actor, at, change)
        VALUES (branch, actor, clock_timestamp(), jsonb_build_object('closedDay', day_, 'closed', closed));
    END IF;
    RETURN ledger.operator_order_state(tenant, branch, actor);
END $$;

-- Marks several products sold out, or on sale again, at once. Only products on the published menu.
CREATE FUNCTION ledger.set_sold_out(tenant uuid, branch uuid, actor uuid, skus jsonb, sold boolean) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE changed text[];
BEGIN
    PERFORM ledger.require_terminal_operator(tenant, branch, actor);
    IF sold IS NULL OR jsonb_typeof(skus) IS DISTINCT FROM 'array' OR jsonb_array_length(skus) NOT BETWEEN 1 AND 200
       OR EXISTS (SELECT FROM jsonb_array_elements(skus) e WHERE jsonb_typeof(e) <> 'string' OR NOT EXISTS (
            SELECT FROM ledger.price p WHERE p.branch_id = branch AND p.sku = e #>> '{}'
              AND p.version = (SELECT max(m.version) FROM ledger.menu_release m WHERE m.branch_id = branch))) THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'invalid sold-out products';
    END IF;
    IF sold THEN
        WITH added AS (INSERT INTO ledger.menu_sold_out SELECT DISTINCT branch, e #>> '{}', actor, clock_timestamp() FROM jsonb_array_elements(skus) e
            ON CONFLICT DO NOTHING RETURNING sku)
        SELECT array_agg(sku ORDER BY sku COLLATE "C") INTO changed FROM added;
    ELSE
        WITH gone AS (DELETE FROM ledger.menu_sold_out s WHERE s.branch_id = branch AND s.sku IN (SELECT e #>> '{}' FROM jsonb_array_elements(skus) e)
            RETURNING sku)
        SELECT array_agg(sku ORDER BY sku COLLATE "C") INTO changed FROM gone;
    END IF;
    IF changed IS NOT NULL THEN
        INSERT INTO ledger.branch_state_log (branch_id, actor, at, change)
        VALUES (branch, actor, clock_timestamp(), jsonb_build_object('soldOut', sold, 'products', to_jsonb(changed)));
    END IF;
    RETURN ledger.operator_order_state(tenant, branch, actor);
END $$;

-- What a customer or terminal sees with the first menu page. No one's identity, only the state.
CREATE FUNCTION ledger.customer_order_state(tenant uuid, branch uuid, actor uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.commerce_allowed(tenant, branch, actor);
    RETURN jsonb_strip_nulls(jsonb_build_object(
        'mode', CASE WHEN EXISTS (SELECT FROM ledger.branch_closed_day d WHERE d.branch_id = branch
                                    AND d.day = ledger.branch_today(branch, clock_timestamp())) THEN 'closed'
                     ELSE coalesce((SELECT s.mode FROM ledger.branch_order_state s WHERE s.branch_id = branch), 'open') END,
        'busyMinutes', (SELECT s.busy_minutes FROM ledger.branch_order_state s WHERE s.branch_id = branch),
        'resumeAt', (SELECT s.resume_note FROM ledger.branch_order_state s WHERE s.branch_id = branch),
        'soldOut', (SELECT jsonb_agg(s.sku ORDER BY s.sku COLLATE "C") FROM ledger.menu_sold_out s WHERE s.branch_id = branch)));
END $$;

REVOKE ALL ON FUNCTION ledger.refuse_sold_out(), ledger.operator_order_state(uuid, uuid, uuid),
    ledger.set_order_state(uuid, uuid, uuid, text, integer, text), ledger.set_closed_day(uuid, uuid, uuid, date, boolean),
    ledger.set_sold_out(uuid, uuid, uuid, jsonb, boolean), ledger.customer_order_state(uuid, uuid, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.operator_order_state(uuid, uuid, uuid),
    ledger.set_order_state(uuid, uuid, uuid, text, integer, text), ledger.set_closed_day(uuid, uuid, uuid, date, boolean),
    ledger.set_sold_out(uuid, uuid, uuid, jsonb, boolean), ledger.customer_order_state(uuid, uuid, uuid) TO nu54_p05_commerce;

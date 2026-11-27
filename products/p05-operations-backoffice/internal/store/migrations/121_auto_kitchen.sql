-- A test store, and its automatic kitchen. The platform marks a store as a test store when it deploys it; no operator
-- route sets this. While a test store's automatic kitchen is on, opsd records start, ready and handed over for each paid
-- cafe order in turn, so a store with one tablet can run. It never records "cannot be prepared"; that stays a person's
-- decision on the operations page. opsd also refuses to run it on any chain but the test chain.

CREATE TABLE ledger.branch_test_store (
    branch_id uuid PRIMARY KEY REFERENCES ledger.branch,
    auto_kitchen boolean NOT NULL DEFAULT false,
    -- The identity the kitchen's records carry, so a reader can tell them from a person's.
    kitchen_actor uuid NOT NULL DEFAULT gen_random_uuid(),
    start_after interval NOT NULL DEFAULT interval '30 seconds' CHECK (start_after BETWEEN interval '0' AND interval '10 minutes'),
    -- Minutes a store that is not busy tells customers; a busy store's own minutes win.
    usual_minutes integer NOT NULL DEFAULT 5 CHECK (usual_minutes BETWEEN 1 AND 60),
    hand_over_after interval NOT NULL DEFAULT interval '2 minutes' CHECK (hand_over_after BETWEEN interval '0' AND interval '30 minutes'),
    marked_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
REVOKE ALL ON ledger.branch_test_store FROM PUBLIC, nu54_p05_commerce;

-- The test stores whose automatic kitchen is on. opsd checks each one's chain before stepping it.
CREATE FUNCTION ledger.auto_kitchen_branches() RETURNS SETOF uuid
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT t.branch_id FROM ledger.branch_test_store t WHERE t.auto_kitchen ORDER BY t.branch_id
$$;

-- One step for each of a test store's open cafe orders that is due: received → making → ready → handed over.
-- At most one stage per order per call, so a kitchen that was off picks up where it stopped and skips nothing.
CREATE FUNCTION ledger.auto_kitchen_step(branch uuid) RETURNS integer
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE kitchen ledger.branch_test_store;
DECLARE minutes integer;
DECLARE due record;
DECLARE moved integer := 0;
BEGIN
    SELECT * INTO kitchen FROM ledger.branch_test_store t WHERE t.branch_id = branch AND t.auto_kitchen;
    IF NOT FOUND THEN RETURN 0; END IF;
    minutes := coalesce((SELECT s.busy_minutes FROM ledger.branch_order_state s WHERE s.branch_id = branch AND s.mode = 'busy'),
                        kitchen.usual_minutes);
    FOR due IN
        SELECT s.id AS sale_id, coalesce(k.state, 'accepted') AS stage, coalesce(k.revision, 0) AS revision
          FROM ledger.sale s
          JOIN ledger.pickup p ON (p.branch_id, p.sale_id) = (s.branch_id, s.id)
          JOIN ledger.fulfillment_outbox e ON e.sale_id = s.id AND e.kind = 'paid' AND e.source_ref = s.id AND e.state = 'delivered'
          LEFT JOIN ledger.travel_pickup_progress k ON k.sale_id = s.id
         WHERE s.branch_id = branch AND s.state = 'paid' AND ledger.kiosk_menu_pickup(s.id) AND ledger.travel_pickup_accepted(s.id)
           AND NOT EXISTS (SELECT FROM ledger.travel_pickup_order o WHERE o.sale_id = s.id)
           AND CASE coalesce(k.state, 'accepted')
                 WHEN 'accepted' THEN e.delivered_at + kitchen.start_after <= clock_timestamp()
                 WHEN 'preparing' THEN k.updated_at + make_interval(mins => minutes) <= clock_timestamp()
                 WHEN 'ready' THEN k.updated_at + kitchen.hand_over_after <= clock_timestamp()
                 ELSE false END
         ORDER BY p.number
         LIMIT 50
         FOR UPDATE OF s SKIP LOCKED
    LOOP
        INSERT INTO ledger.travel_pickup_progress(sale_id, state, revision, updated_at)
        VALUES (due.sale_id, CASE due.stage WHEN 'accepted' THEN 'preparing' WHEN 'preparing' THEN 'ready' ELSE 'collected' END,
                due.revision + 1, clock_timestamp())
        ON CONFLICT (sale_id) DO UPDATE SET state = excluded.state, revision = excluded.revision, updated_at = excluded.updated_at
        WHERE ledger.travel_pickup_progress.revision = due.revision; -- a person's change in between wins
        IF FOUND THEN
            INSERT INTO ledger.travel_pickup_transition VALUES (due.sale_id, kitchen.kitchen_actor, gen_random_uuid(), due.revision + 1,
                CASE due.stage WHEN 'accepted' THEN 'preparing' WHEN 'preparing' THEN 'ready' ELSE 'collected' END);
            moved := moved + 1;
        END IF;
    END LOOP;
    RETURN moved;
END $$;

-- The operations page finds one of today's orders by its number, for cancelling it before it is handed over.
CREATE FUNCTION ledger.operator_find_order(tenant uuid, branch uuid, actor uuid, number_ bigint) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.kiosk_require_pickup(tenant, branch, actor, 'operator');
    RETURN (
        SELECT jsonb_strip_nulls(jsonb_build_object(
            'sale', s.id, 'number', p.number, 'stage', coalesce(k.state, 'accepted'), 'revision', coalesce(k.revision, 0),
            'amount', s.amount::text, 'asset', s.asset_ref,
            'refund', (SELECT a.state FROM ledger.escrow_action a WHERE a.sale_id = s.id AND a.kind = 'cancel' ORDER BY a.created_at DESC LIMIT 1),
            'byKitchen', (SELECT x.actor_id = t.kitchen_actor FROM ledger.travel_pickup_transition x
                           WHERE x.sale_id = s.id ORDER BY x.revision DESC LIMIT 1),
            'cancelledBy', (SELECT x.actor_id FROM ledger.travel_pickup_transition x WHERE x.sale_id = s.id AND x.target = 'unfulfillable' LIMIT 1),
            'items', (SELECT jsonb_agg(jsonb_build_object('label', x.label, 'option', nullif(l.option_key, 'standard'), 'quantity', l.quantity)
                        ORDER BY l.sku COLLATE "C", l.option_key COLLATE "C")
                FROM ledger.order_line l JOIN ledger.price x ON (x.branch_id, x.version, x.sku, x.option_key) = (l.branch_id, l.menu_version, l.sku, l.option_key)
                WHERE l.sale_id = s.id)))
          FROM ledger.pickup p
          JOIN ledger.sale s ON (s.branch_id, s.id) = (p.branch_id, p.sale_id)
          LEFT JOIN ledger.travel_pickup_progress k ON k.sale_id = s.id
          LEFT JOIN ledger.branch_test_store t ON t.branch_id = branch
         WHERE p.branch_id = branch AND p.number = number_ AND p.business_day = ledger.ledger_business_day(branch, clock_timestamp())
           AND ledger.kiosk_menu_pickup(s.id));
END $$;

-- Whether the store runs as a test store with its kitchen on, for the words on customer and operator screens.
CREATE FUNCTION ledger.branch_auto_kitchen(branch uuid) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT coalesce((SELECT t.auto_kitchen FROM ledger.branch_test_store t WHERE t.branch_id = branch), false)
$$;

REVOKE ALL ON FUNCTION ledger.auto_kitchen_branches(), ledger.auto_kitchen_step(uuid), ledger.operator_find_order(uuid, uuid, uuid, bigint),
    ledger.branch_auto_kitchen(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.auto_kitchen_branches(), ledger.auto_kitchen_step(uuid), ledger.operator_find_order(uuid, uuid, uuid, bigint)
    TO nu54_p05_commerce;

-- The store state also says whether the automatic kitchen of a test store is on.
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
        'autoKitchen', ledger.branch_auto_kitchen(branch),
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
        'autoKitchen', CASE WHEN ledger.branch_auto_kitchen(branch) THEN true END,
        'taxFreeShop', (SELECT t.designation FROM ledger.branch_tax_free_shop t WHERE t.branch_id = branch),
        'soldOut', (SELECT jsonb_agg(s.sku ORDER BY s.sku COLLATE "C") FROM ledger.menu_sold_out s WHERE s.branch_id = branch)));
END $$;

-- The board says so too, for the words on the tablet while nobody is ordering.
CREATE OR REPLACE FUNCTION ledger.ready_board(tenant uuid, branch uuid, actor uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE grant_ ledger.commerce_grant := ledger.commerce_allowed(tenant, branch, actor);
DECLARE day_ date := ledger.ledger_business_day(branch, clock_timestamp());
BEGIN
    IF grant_.role_code IS DISTINCT FROM 'terminal' THEN
        RAISE EXCEPTION USING ERRCODE = '42501', MESSAGE = 'ready board is for the store tablet';
    END IF;
    RETURN (
        WITH today AS (
            SELECT p.number, coalesce(k.state, 'received') AS stage, k.updated_at
              FROM ledger.pickup p
              JOIN ledger.sale s ON (s.branch_id, s.id) = (p.branch_id, p.sale_id)
              LEFT JOIN ledger.travel_pickup_progress k ON k.sale_id = p.sale_id
             WHERE p.branch_id = branch AND p.business_day = day_ AND s.state = 'paid'
               AND coalesce(k.state, 'received') IN ('received', 'preparing', 'ready')
               AND NOT EXISTS (SELECT FROM ledger.refund r WHERE r.sale_id = p.sale_id))
        SELECT jsonb_build_object(
            'checkedAt', to_char(clock_timestamp() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
            'autoKitchen', ledger.branch_auto_kitchen(branch),
            -- Newest ready first: the band shows the first four.
            'ready', coalesce((SELECT jsonb_agg(number ORDER BY updated_at DESC, number DESC)
                FROM (SELECT * FROM today WHERE stage = 'ready' ORDER BY updated_at DESC, number DESC LIMIT 40) r), '[]'),
            'making', coalesce((SELECT jsonb_agg(number ORDER BY number)
                FROM (SELECT * FROM today WHERE stage = 'preparing' ORDER BY number LIMIT 40) m), '[]'),
            'received', coalesce((SELECT jsonb_agg(number ORDER BY number)
                FROM (SELECT * FROM today WHERE stage = 'received' ORDER BY number LIMIT 40) n), '[]'))
    );
END $$;

-- Two reads of the customer entrance that the first walk of the route without a store registration found closed
-- too early (2026-11-28). No table, column or grant changes: both functions are replaced in place and keep the
-- privileges migrations 122 and 124 gave them (EXECUTE for nu54_p05_commerce, none for PUBLIC).

-- 1. The menu of a store that takes no order now.
--
-- The menu is public (owner decision 2026-11-27, migration 124), and a store page says "the menu can be read" when
-- the store paused its orders or today is its closed day. Migration 124 chose the rows with ledger.catalog_is_open,
-- which migration 118 made false for a paused store and a closed day, so such a store showed no row at all.
--
-- The rows are now those of the newest release while it is on sale: published, not run out, and inside the hours of
-- its price list. The store's pause and its closed day no longer take the rows away; they are said by the state
-- beside the rows, as before (`mode` paused or closed). Nothing else changes: no count of stock is public, a store
-- without a published page answers as no such store, and a menu that is not on sale has no row.
--
-- Ordering stays closed. A new order still asks ledger.catalog_is_open (ledger.reserve_order), and the menu a
-- session reads (ledger.catalog_menu_photo_window) still has no row for a paused store or on a closed day.
CREATE OR REPLACE FUNCTION ledger.public_menu_window(branch_ uuid, page_ integer)
RETURNS TABLE(snapshot_version bigint, items jsonb, options jsonb, photos jsonb, state jsonb, label text)
LANGUAGE plpgsql STABLE SECURITY DEFINER ROWS 1 SET search_path=pg_catalog,pg_temp AS $menu$
DECLARE newest bigint; on_sale boolean; name_ text;
BEGIN
    IF page_ IS NULL OR page_ < 0 OR page_ > 32767 THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid page';
    END IF;
    SELECT l.label INTO name_ FROM ledger.operator_branch_label l WHERE l.branch_id = branch_;
    IF name_ IS NULL OR NOT ledger.store_page_published(branch_) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='no such store';
    END IF;
    -- The newest release, as ledger.catalog_page chooses it.
    IF EXISTS (SELECT FROM ledger.catalog_pricing p WHERE p.branch_id = branch_) THEN
        SELECT max(m.version) INTO newest FROM ledger.menu_release m WHERE m.branch_id = branch_;
    ELSE
        SELECT max(m.version) INTO newest FROM ledger.menu_release m WHERE m.branch_id = branch_
            AND m.published_at <= statement_timestamp() AND m.expires_at > statement_timestamp();
    END IF;
    -- On sale now: the release's own dates and the hours of its price list (the first half of
    -- ledger.catalog_is_open). Whether the store takes orders now is not asked here.
    on_sale := EXISTS (SELECT FROM ledger.menu_release m WHERE m.branch_id = branch_ AND m.version = newest
            AND m.published_at <= statement_timestamp() AND m.expires_at > statement_timestamp())
        AND coalesce((SELECT (statement_timestamp() AT TIME ZONE (p.hours->>'zone'))::time >= (p.hours->>'opens')::time
                         AND (statement_timestamp() AT TIME ZONE (p.hours->>'zone'))::time < (p.hours->>'closes')::time
                        FROM ledger.catalog_pricing p WHERE p.branch_id = branch_ AND p.version = newest), true);
    RETURN QUERY
    WITH prices AS MATERIALIZED (
        SELECT m.version, m.expires_at, p.sku, p.option_key, p.label AS name, p.asset_ref, p.unit_price::numeric AS unit_price,
               least(coalesce(s.available, 0), item.available) AS available, coalesce(s.available, 0) AS sku_available,
               (SELECT CASE WHEN e.row->'required' = 'true'::jsonb
                           THEN coalesce(nullif(e.row->'metadata', 'null'::jsonb), '{}'::jsonb) || '{"optionRequired":true}'::jsonb
                           ELSE e.row->'metadata' END
                  FROM jsonb_array_elements(c.breakdown) e(row)
                 WHERE e.row->>'sku' = p.sku AND e.row->>'option' = p.option_key) AS metadata,
               CASE WHEN k.category_key IS NOT NULL
                    THEN jsonb_build_object('key', k.category_key, 'sortOrder', k.sort_order, 'labels', k.labels) END AS category
          FROM ledger.menu_release m JOIN ledger.price p USING (branch_id, version)
          LEFT JOIN ledger.stock s ON s.branch_id = branch_ AND s.sku = p.sku
          LEFT JOIN ledger.catalog_pricing c ON c.branch_id = branch_ AND c.version = m.version
          LEFT JOIN ledger.catalog_option_stock item ON (item.branch_id, item.sku, item.option_key) = (branch_, p.sku, p.option_key)
          LEFT JOIN ledger.menu_sku_category sc ON (sc.branch_id, sc.version, sc.sku) = (branch_, m.version, p.sku)
          LEFT JOIN ledger.menu_category k ON (k.branch_id, k.version, k.category_key) = (sc.branch_id, sc.version, sc.category_key)
         WHERE on_sale AND m.branch_id = branch_ AND m.version = newest
         ORDER BY p.sku, p.option_key LIMIT 50 OFFSET page_ * 50
    ), page AS (
        SELECT CASE WHEN on_sale AND EXISTS (SELECT FROM ledger.price p WHERE p.branch_id = branch_ AND p.version = newest) THEN newest END AS version,
               coalesce((SELECT jsonb_agg(jsonb_build_object('version', p.version, 'expiresAt', p.expires_at, 'sku', p.sku,
                   'optionKey', p.option_key, 'label', p.name, 'asset', p.asset_ref, 'unitPrice', p.unit_price::text,
                   'available', CASE WHEN p.available > 0 THEN 1000 ELSE 0 END,
                   'skuAvailable', CASE WHEN p.sku_available > 0 THEN 1000 ELSE 0 END, 'metadata', p.metadata, 'category', p.category)
                   ORDER BY p.sku, p.option_key) FROM prices p), '[]'::jsonb) AS rows
    )
    SELECT page.version, page.rows,
        coalesce((SELECT jsonb_agg(jsonb_build_object('sku', o.sku, 'groups', (
            SELECT jsonb_agg(jsonb_build_object('key', g->'key', 'labels', g->'labels', 'values', (
                SELECT jsonb_agg(jsonb_build_object('key', v->'key', 'labels', v->'labels') ORDER BY place)
                FROM jsonb_array_elements(g->'values') WITH ORDINALITY x(v, place))) ORDER BY seq)
            FROM jsonb_array_elements(o.groups) WITH ORDINALITY y(g, seq))) ORDER BY o.sku COLLATE "C")
          FROM ledger.menu_sku_options o WHERE o.branch_id = branch_ AND o.version = page.version
           AND o.sku IN (SELECT item->>'sku' FROM jsonb_array_elements(page.rows) item)), '[]'::jsonb),
        coalesce((SELECT jsonb_agg(jsonb_build_object('sku', s.sku, 'photo', s.digest, 'thumb', p.thumb_digest) ORDER BY s.sku COLLATE "C")
          FROM ledger.menu_sku_photo s JOIN ledger.menu_photo p USING (branch_id, digest)
         WHERE s.branch_id = branch_ AND s.version = page.version
           AND s.sku IN (SELECT item->>'sku' FROM jsonb_array_elements(page.rows) item)), '[]'::jsonb),
        jsonb_strip_nulls(jsonb_build_object(
            'mode', CASE WHEN EXISTS (SELECT FROM ledger.branch_closed_day d WHERE d.branch_id = branch_
                                        AND d.day = ledger.branch_today(branch_, clock_timestamp())) THEN 'closed'
                         ELSE coalesce((SELECT s.mode FROM ledger.branch_order_state s WHERE s.branch_id = branch_), 'open') END,
            'busyMinutes', (SELECT s.busy_minutes FROM ledger.branch_order_state s WHERE s.branch_id = branch_),
            'resumeAt', (SELECT s.resume_note FROM ledger.branch_order_state s WHERE s.branch_id = branch_),
            'autoKitchen', CASE WHEN ledger.branch_auto_kitchen(branch_) THEN true END,
            'taxFreeShop', (SELECT t.designation FROM ledger.branch_tax_free_shop t WHERE t.branch_id = branch_),
            'soldOut', (SELECT jsonb_agg(s.sku ORDER BY s.sku COLLATE "C") FROM ledger.menu_sold_out s WHERE s.branch_id = branch_))),
        name_
    FROM page;
END
$menu$;

-- 2. The session of an order that ended.
--
-- An order session follows one paid order (migration 122). Migrations 122 and 126 ended it at the moment the escrow
-- settled, so the phone that followed the order never read its end: the answer to its next status request was "no
-- such session", it proved its account again, and that session was over as well. The phone kept the order as
-- unfinished and held the board's reset and return back for an order that was paid out.
--
-- The session now reads its order for a bounded time after the order ended:
--   a day after the ledger wrote the end, for a session that was there, so the phone that follows the order reads
--     its final state the next time it asks, also when it was closed for hours;
--   ten minutes after its own proof, for a session proven later, so a phone that was away for longer learns the
--     end with one proof of the paying board (ledger.prove_customer_account still asks for the account that paid).
-- When the ledger wrote the end is order_escrow.check_at: ledger.record_escrow_event sets it to its own clock in the
-- statement that sets `settled`, and nothing moves it afterwards (ledger.claim_escrow_follow takes only orders that
-- are not settled).
--
-- What the session may do is unchanged and decided elsewhere: a status read of its own order answers the final
-- state (ledger.order_escrow_status), and an answer to an order that ended is refused by ledger.store_order_answer.
-- A sale that was closed has no session, as before; a checkout session is untouched.
CREATE OR REPLACE FUNCTION ledger.customer_session_live(token_ bytea)
RETURNS TABLE(token_hash bytea,branch_id uuid,actor_id uuid,purpose text,sale_id uuid,tenant_ref uuid,expires_at timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $live$
    SELECT x.token_hash,x.branch_id,x.actor_id,x.purpose,x.sale_id,b.tenant_ref,least(x.expires_at,g.valid_until)
    FROM ledger.customer_session x JOIN ledger.branch b ON b.id=x.branch_id
    JOIN ledger.commerce_grant g ON (g.branch_id,g.actor_id)=(x.branch_id,x.actor_id) AND g.role_code='customer'
    WHERE x.token_hash=token_ AND x.expires_at>clock_timestamp() AND g.valid_until>clock_timestamp()
      AND (x.purpose='checkout' OR NOT EXISTS (SELECT FROM ledger.sale s WHERE s.id=x.sale_id AND s.state='closed'))
      AND (x.purpose='checkout' OR x.created_at>clock_timestamp()-interval '10 minutes'
           OR NOT EXISTS (SELECT FROM ledger.order_escrow e WHERE e.sale_id=x.sale_id AND e.settled
                          AND e.check_at<=clock_timestamp()-interval '1 day'))
      AND (x.purpose<>'checkout' OR ledger.store_page_published(x.branch_id))
$live$;

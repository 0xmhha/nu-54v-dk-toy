-- A store's menu is public (owner decision 2026-11-27): a customer reads the menu, its photos and what is sold out
-- from the store page, before any account proof, and keeps the cart on the phone. Stock is held only after the
-- proof, by the session's order (migration 122). These two reads take no actor and no grant, so they answer only
-- what a customer session reads as the same store's menu: nothing about a customer, an order or how the store is paid.
-- Only a store that published its store page (migration 123) has a public menu; any other answers as no such store.

-- Whether the store's page is published, which is what makes its menu and photos public.
CREATE FUNCTION ledger.store_page_published(branch_ uuid) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $published$
    SELECT EXISTS (SELECT FROM ledger.branch_store_page p JOIN ledger.operator_branch_label l ON l.branch_id = p.branch_id
                   WHERE p.branch_id = branch_ AND p.published)
$published$;
REVOKE ALL ON FUNCTION ledger.store_page_published(uuid) FROM PUBLIC;

-- One page of the published menu with its choices, photo names, the store's name and whether it takes orders now.
-- The rows are those of ledger.catalog_menu_photo_window for a customer of the branch, and the state is that of
-- ledger.customer_order_state; customer_entrance_postgres_test.go holds the two side by side.
--
-- With one difference: no count of stock is public. `available` and `skuAvailable` say only whether the row is
-- sold out: 0 when it is, 1000 (the most one order line may ask for) when it is not. A session reads the counts.
CREATE FUNCTION ledger.public_menu_window(branch_ uuid, page_ integer)
RETURNS TABLE(snapshot_version bigint, items jsonb, options jsonb, photos jsonb, state jsonb, label text)
LANGUAGE plpgsql STABLE SECURITY DEFINER ROWS 1 SET search_path=pg_catalog,pg_temp AS $menu$
DECLARE newest bigint; live boolean; name_ text;
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
    live := EXISTS (SELECT FROM ledger.menu_release m WHERE m.branch_id = branch_ AND m.version = newest
            AND m.published_at <= statement_timestamp() AND m.expires_at > statement_timestamp())
        AND ledger.catalog_is_open(branch_, newest, statement_timestamp());
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
         WHERE live AND m.branch_id = branch_ AND m.version = newest
         ORDER BY p.sku, p.option_key LIMIT 50 OFFSET page_ * 50
    ), page AS (
        SELECT CASE WHEN live AND EXISTS (SELECT FROM ledger.price p WHERE p.branch_id = branch_ AND p.version = newest) THEN newest END AS version,
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

-- A part of a published photo of a store with a published store page, by its name, as ledger.catalog_photo_part reads it.
-- A part is at most 18,000 bytes; a photo is at most 150 KB (migration 115).
CREATE FUNCTION ledger.public_menu_photo_part(branch_ uuid, digest_ text, offset_ integer)
RETURNS TABLE(size integer, part bytea)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $photo$
DECLARE image bytea;
BEGIN
    IF NOT ledger.store_page_published(branch_) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='no such store';
    END IF;
    SELECT CASE WHEN p.digest = digest_ THEN p.photo ELSE p.thumb END INTO image
      FROM ledger.menu_photo p WHERE p.branch_id = branch_ AND digest_ IN (p.digest, p.thumb_digest)
       AND EXISTS (SELECT FROM ledger.menu_sku_photo s WHERE (s.branch_id, s.digest) = (p.branch_id, p.digest));
    IF image IS NULL OR offset_ IS NULL OR offset_ < 0 OR offset_ >= length(image) THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='menu photo not found';
    END IF;
    RETURN QUERY SELECT length(image), substring(image FROM offset_ + 1 FOR 18000);
END
$photo$;

REVOKE ALL ON FUNCTION ledger.public_menu_window(uuid, integer), ledger.public_menu_photo_part(uuid, text, integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.public_menu_window(uuid, integer), ledger.public_menu_photo_part(uuid, text, integer) TO nu54_p05_commerce;

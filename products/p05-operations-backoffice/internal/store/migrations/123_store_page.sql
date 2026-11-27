-- A store can have a store page in the customer app: what the store wrote about itself, where it is, when it is open,
-- and the way to its menu. Customers pick a store from the app's list or open one with the store's shortcut QR before
-- they prove anything, so the public reads take no actor and give out only what the store published here, the store's
-- registered name and whether it takes orders now. Nothing about customers, orders, staff or the tenant is read.
--
-- Nothing of a store is public until the store says so: its page, its menu and its photos answer only while the
-- store's page is published, which the store switches on the operations web and which is off until then. Knowing a
-- store's reference opens nothing by itself.

-- Monday first: seven entries, each null (closed that weekday) or {"opens":"HH:MM","closes":"HH:MM"} in the store's
-- own time zone. A closing time before the opening time is on the next day.
CREATE FUNCTION ledger.store_hours_valid(hours jsonb) RETURNS boolean
LANGUAGE sql IMMUTABLE SET search_path=pg_catalog,pg_temp AS $hours$
    SELECT CASE WHEN jsonb_typeof(hours) IS DISTINCT FROM 'array' OR jsonb_array_length(hours) <> 7 THEN false
    ELSE NOT EXISTS (SELECT FROM jsonb_array_elements(hours) d(day) WHERE NOT CASE jsonb_typeof(d.day)
        WHEN 'null' THEN true
        WHEN 'object' THEN d.day - 'opens' - 'closes' = '{}'::jsonb
            AND coalesce(d.day->>'opens', '') ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'
            AND coalesce(d.day->>'closes', '') ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'
            AND d.day->>'opens' <> d.day->>'closes'
        ELSE false END) END
$hours$;

-- Text a store publishes is one line of characters a customer can see. It holds no control character (Unicode
-- category Cc), no format character (Cf: zero-width spaces and joiners, the marks that reorder text, the soft hyphen,
-- the byte order mark, tags), no line or paragraph separator (Zl, Zp), no other character a display leaves out
-- (Default_Ignorable_Code_Point: the Hangul fillers, variation selectors, the combining grapheme joiner) and no
-- blank other than the ordinary space (the no-break space and the other Zs spaces, the blank Braille pattern). None
-- of them shows, and they can make a page read as something the store did not write, or a name that is nothing but
-- blanks. Text that is not empty also holds at least one character that is not a space. The code points are listed,
-- so the answer is the same under any locale of the database; the operations web holds the same list
-- (web/src/storePageForm.ts) and opsd too (commerce/visible_text.go).
CREATE FUNCTION ledger.store_text_plain(value text) RETURNS boolean
LANGUAGE sql IMMUTABLE SET search_path=pg_catalog,pg_temp AS $plain$
    SELECT value !~ ('[\u0001-\u001F\u007F-\u009F'                                               -- Cc
        || '\u00AD\u0600-\u0605\u061C\u06DD\u070F\u0890\u0891\u08E2\u180E\u200B-\u200F'    -- Cf
        || '\u2028-\u202E'                                                                        -- Zl, Zp, Cf
        || '\u2060-\u206F\uFEFF\uFFF9-\uFFFB\U000110BD\U000110CD\U00013430-\U0001343F'         -- Cf
        || '\U0001BCA0-\U0001BCA3\U0001D173-\U0001D17A'                                           -- Cf
        || '\u034F\u115F\u1160\u17B4\u17B5\u180B-\u180D\u180F\u3164\uFE00-\uFE0F\uFFA0'       -- default ignorable
        || '\uFFF0-\uFFF8\U000E0000-\U000E0FFF'                                                  -- default ignorable, tags
        || '\u00A0\u1680\u2000-\u200A\u202F\u205F\u3000\u2800]')                              -- blanks but the space
        AND (value = '' OR value ~ '[^ ]')
$plain$;

CREATE TABLE ledger.branch_store_page (
    -- The page goes with its branch: the record of what is kept (field-purpose.json) says "delete with the branch".
    branch_id uuid PRIMARY KEY REFERENCES ledger.branch ON DELETE CASCADE,
    -- Published: customers can open the store's page and its menu. Off, both answer as a store that does not exist.
    published boolean NOT NULL,
    -- A listed store appears in the app's store list; it matters only while the page is published. A published,
    -- unlisted store is opened by its shortcut QR.
    listed boolean NOT NULL,
    kind text NOT NULL CHECK (length(kind) BETWEEN 1 AND 40 AND ledger.store_text_plain(kind) AND kind = btrim(kind)),
    intro text NOT NULL CHECK (length(intro) <= 200 AND ledger.store_text_plain(intro) AND intro = btrim(intro)),
    address text NOT NULL CHECK (length(address) <= 200 AND ledger.store_text_plain(address) AND address = btrim(address)),
    directions text NOT NULL CHECK (length(directions) <= 200 AND ledger.store_text_plain(directions) AND directions = btrim(directions)),
    phone text NOT NULL CHECK (phone = '' OR phone ~ '^\+?[0-9][0-9 -]{5,18}[0-9]$'),
    amenities text NOT NULL CHECK (length(amenities) <= 200 AND ledger.store_text_plain(amenities) AND amenities = btrim(amenities)),
    parking text NOT NULL CHECK (length(parking) <= 200 AND ledger.store_text_plain(parking) AND parking = btrim(parking)),
    closed_note text NOT NULL CHECK (length(closed_note) <= 120 AND ledger.store_text_plain(closed_note) AND closed_note = btrim(closed_note)),
    -- NULL: the store has not said its hours.
    hours jsonb CHECK (hours IS NULL OR ledger.store_hours_valid(hours)),
    changed_by uuid NOT NULL,
    changed_at timestamptz NOT NULL
);
REVOKE ALL ON ledger.branch_store_page FROM PUBLIC, nu54_p05_commerce;

-- Why an order cannot be placed with the store now, or NULL when one can. It is what a new order is asked
-- (ledger.reserve_order), read without placing one:
--   closed_day     today is one of the store's closed days
--   paused         the store paused its orders
--   no_menu        there is no menu on sale now: none was published, the newest is not on sale yet, or it ran out
--   outside_hours  the menu is on sale, but not at this time of the day (the hours of the price list)
-- The menu is the one ledger.public_menu_window shows.
CREATE FUNCTION ledger.store_order_refusal(branch_ uuid, at_ timestamptz) RETURNS text
LANGUAGE plpgsql STABLE SET search_path=pg_catalog,pg_temp AS $refusal$
DECLARE newest bigint;
BEGIN
    IF EXISTS (SELECT FROM ledger.branch_closed_day d WHERE d.branch_id = branch_ AND d.day = ledger.branch_today(branch_, at_)) THEN
        RETURN 'closed_day';
    END IF;
    IF EXISTS (SELECT FROM ledger.branch_order_state s WHERE s.branch_id = branch_ AND s.mode = 'paused') THEN
        RETURN 'paused';
    END IF;
    IF EXISTS (SELECT FROM ledger.catalog_pricing p WHERE p.branch_id = branch_) THEN
        SELECT max(m.version) INTO newest FROM ledger.menu_release m WHERE m.branch_id = branch_;
    ELSE
        SELECT max(m.version) INTO newest FROM ledger.menu_release m WHERE m.branch_id = branch_
            AND m.published_at <= at_ AND m.expires_at > at_;
    END IF;
    IF newest IS NULL
       OR NOT EXISTS (SELECT FROM ledger.menu_release m WHERE m.branch_id = branch_ AND m.version = newest
                      AND m.published_at <= at_ AND m.expires_at > at_)
       OR NOT EXISTS (SELECT FROM ledger.price p WHERE p.branch_id = branch_ AND p.version = newest) THEN
        RETURN 'no_menu';
    END IF;
    IF NOT ledger.catalog_is_open(branch_, newest, at_) THEN
        RETURN 'outside_hours';
    END IF;
    RETURN NULL;
END
$refusal$;

-- Whether an order can be placed with the store now, and the store's own weekday and wall clock, so the app needs
-- no time zone arithmetic to say "open until 21:00".
--   orderable  true when a new order would be taken now
--   reason     why not, when orderable is false (ledger.store_order_refusal)
--   mode       what the store shows of itself: open, busy (with busyMinutes) or paused (with resumeAt) as the store
--              set it while an order can be placed or the store paused; closed in every other case, so a store
--              with no menu on sale, outside its hours or on a closed day never reads as open
CREATE FUNCTION ledger.store_page_status(branch uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $status$
    SELECT jsonb_strip_nulls(jsonb_build_object(
        'mode', CASE WHEN r.reason IS NULL OR r.reason = 'paused' THEN coalesce(s.mode, 'open') ELSE 'closed' END,
        'orderable', r.reason IS NULL,
        'reason', r.reason,
        'busyMinutes', s.busy_minutes,
        'resumeAt', s.resume_note,
        'weekday', extract(isodow FROM t.at AT TIME ZONE b.timezone)::int - 1,
        'time', to_char(t.at AT TIME ZONE b.timezone, 'HH24:MI'),
        'timezone', b.timezone))
    FROM ledger.branch b CROSS JOIN (SELECT clock_timestamp() AS at) t
    CROSS JOIN LATERAL (SELECT ledger.store_order_refusal(b.id, t.at) AS reason) r
    LEFT JOIN ledger.branch_order_state s ON s.branch_id = b.id
    WHERE b.id = branch
$status$;

-- One store's page for anyone who has its reference (the shortcut QR or the list), once the store published it. A
-- store that did not, or that has no registered name, has no page: the answer is the one of no such store.
CREATE FUNCTION ledger.public_store_page(branch uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $page$
    SELECT jsonb_strip_nulls(jsonb_build_object(
        'branchRef', b.id, 'name', n.label, 'kind', p.kind, 'intro', nullif(p.intro, ''),
        'address', nullif(p.address, ''), 'directions', nullif(p.directions, ''), 'phone', nullif(p.phone, ''),
        'amenities', nullif(p.amenities, ''), 'parking', nullif(p.parking, ''), 'closedNote', nullif(p.closed_note, ''),
        'hours', p.hours, 'status', ledger.store_page_status(b.id)))
    FROM ledger.branch b JOIN ledger.operator_branch_label n ON n.branch_id = b.id
    JOIN ledger.branch_store_page p ON p.branch_id = b.id AND p.published
    WHERE b.id = branch
$page$;

-- The published stores that chose to be listed, by name, with what a list row says: kind, address, hours and order state.
-- One page of at most limit_ rows (200 at most). When more stores follow, `next` is the reference of the page's
-- last store, and the list goes on from there when it is given back as after_; without `next` the list is complete.
-- The app searches the rows by name.
CREATE FUNCTION ledger.public_store_list(limit_ integer, after_ uuid) RETURNS jsonb
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $list$
DECLARE
    size integer := greatest(1, least(coalesce(limit_, 200), 200));
    from_ text;
    page jsonb;
BEGIN
    IF after_ IS NOT NULL THEN
        -- The place to go on from is a store of the list itself. A store that is not published and listed is no
        -- place in it, whether or not it exists: the answer does not tell the two apart.
        SELECT n.label INTO from_ FROM ledger.operator_branch_label n JOIN ledger.branch_store_page p ON p.branch_id = n.branch_id
         WHERE n.branch_id = after_ AND p.published AND p.listed;
        IF from_ IS NULL THEN
            RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'invalid list position';
        END IF;
    END IF;
    WITH found AS MATERIALIZED (
        SELECT b.id, n.label, jsonb_strip_nulls(jsonb_build_object('branchRef', b.id, 'name', n.label, 'kind', p.kind,
            'address', nullif(p.address, ''), 'hours', p.hours, 'status', ledger.store_page_status(b.id))) AS store,
            row_number() OVER (ORDER BY n.label, b.id) AS place
        FROM ledger.branch_store_page p JOIN ledger.branch b ON b.id = p.branch_id
        JOIN ledger.operator_branch_label n ON n.branch_id = b.id
        WHERE p.published AND p.listed AND (after_ IS NULL OR (n.label, b.id) > (from_, after_))
        ORDER BY n.label, b.id LIMIT size + 1)
    SELECT jsonb_build_object('stores', coalesce(jsonb_agg(f.store ORDER BY f.place) FILTER (WHERE f.place <= size), '[]'::jsonb))
        || CASE WHEN count(*) > size THEN jsonb_build_object('next', (array_agg(f.id ORDER BY f.place))[size]) ELSE '{}'::jsonb END
      INTO page FROM found f;
    RETURN page;
END
$list$;

-- What the operations web shows and edits: the page as the store wrote it, or `saved: false` before the first save.
CREATE FUNCTION ledger.operator_store_page(tenant uuid, branch uuid, actor uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $read$
BEGIN
    PERFORM ledger.require_terminal_operator(tenant, branch, actor);
    RETURN (SELECT jsonb_build_object('name', n.label, 'saved', p.branch_id IS NOT NULL,
            'published', coalesce(p.published, false), 'listed', coalesce(p.listed, false),
            'kind', coalesce(p.kind, ''), 'intro', coalesce(p.intro, ''), 'address', coalesce(p.address, ''),
            'directions', coalesce(p.directions, ''), 'phone', coalesce(p.phone, ''), 'amenities', coalesce(p.amenities, ''),
            'parking', coalesce(p.parking, ''), 'closedNote', coalesce(p.closed_note, ''), 'hours', p.hours,
            'changedBy', p.changed_by, 'changedAt', p.changed_at)
        FROM ledger.branch b LEFT JOIN ledger.operator_branch_label n ON n.branch_id = b.id
        LEFT JOIN ledger.branch_store_page p ON p.branch_id = b.id WHERE b.id = branch);
END $read$;

-- The store writes its whole page at once, with whether it is published and listed. A store without a registered
-- name cannot save one.
CREATE FUNCTION ledger.set_store_page(tenant uuid, branch uuid, actor uuid, page jsonb) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $write$
DECLARE hours_ jsonb;
BEGIN
    PERFORM ledger.require_terminal_operator(tenant, branch, actor);
    IF jsonb_typeof(page) IS DISTINCT FROM 'object'
       OR page - ARRAY['published','listed','kind','intro','address','directions','phone','amenities','parking','closedNote','hours'] <> '{}'::jsonb
       OR jsonb_typeof(page->'published') IS DISTINCT FROM 'boolean'
       OR jsonb_typeof(page->'listed') IS DISTINCT FROM 'boolean'
       OR EXISTS (SELECT FROM unnest(ARRAY['kind','intro','address','directions','phone','amenities','parking','closedNote']) f(name)
                  WHERE jsonb_typeof(page->f.name) IS DISTINCT FROM 'string')
       OR NOT EXISTS (SELECT FROM ledger.operator_branch_label n WHERE n.branch_id = branch) THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'invalid store page';
    END IF;
    hours_ := nullif(page->'hours', 'null'::jsonb);
    IF hours_ IS NOT NULL AND NOT ledger.store_hours_valid(hours_) THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'invalid store hours';
    END IF;
    INSERT INTO ledger.branch_store_page VALUES (branch, (page->>'published')::boolean, (page->>'listed')::boolean,
        btrim(page->>'kind'), btrim(page->>'intro'),
        btrim(page->>'address'), btrim(page->>'directions'), btrim(page->>'phone'), btrim(page->>'amenities'),
        btrim(page->>'parking'), btrim(page->>'closedNote'), hours_, actor, clock_timestamp())
    ON CONFLICT (branch_id) DO UPDATE SET published = EXCLUDED.published, listed = EXCLUDED.listed, kind = EXCLUDED.kind, intro = EXCLUDED.intro,
        address = EXCLUDED.address, directions = EXCLUDED.directions, phone = EXCLUDED.phone, amenities = EXCLUDED.amenities,
        parking = EXCLUDED.parking, closed_note = EXCLUDED.closed_note, hours = EXCLUDED.hours,
        changed_by = EXCLUDED.changed_by, changed_at = EXCLUDED.changed_at;
    RETURN ledger.operator_store_page(tenant, branch, actor);
END $write$;

REVOKE ALL ON FUNCTION ledger.store_hours_valid(jsonb), ledger.store_text_plain(text), ledger.store_order_refusal(uuid, timestamptz), ledger.store_page_status(uuid), ledger.public_store_page(uuid),
    ledger.public_store_list(integer, uuid), ledger.operator_store_page(uuid, uuid, uuid),
    ledger.set_store_page(uuid, uuid, uuid, jsonb) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.public_store_page(uuid), ledger.public_store_list(integer, uuid),
    ledger.operator_store_page(uuid, uuid, uuid), ledger.set_store_page(uuid, uuid, uuid, jsonb) TO nu54_p05_commerce;

-- Every store has a store page in the customer app: what the store wrote about itself, where it is, when it is open,
-- and the way to its menu. Customers pick a store from the app's list or open one with the store's shortcut QR before
-- they prove anything, so the public reads take no actor and give out only what the store published here, the store's
-- registered name and whether it takes orders now. Nothing about customers, orders, staff or the tenant is read.

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

CREATE TABLE ledger.branch_store_page (
    branch_id uuid PRIMARY KEY REFERENCES ledger.branch,
    -- A listed store appears in the app's store list. An unlisted one is still opened by its shortcut QR.
    listed boolean NOT NULL,
    kind text NOT NULL CHECK (length(kind) BETWEEN 1 AND 40 AND kind !~ '[[:cntrl:]]' AND kind = btrim(kind)),
    intro text NOT NULL CHECK (length(intro) <= 200 AND intro !~ '[[:cntrl:]]' AND intro = btrim(intro)),
    address text NOT NULL CHECK (length(address) <= 200 AND address !~ '[[:cntrl:]]' AND address = btrim(address)),
    directions text NOT NULL CHECK (length(directions) <= 200 AND directions !~ '[[:cntrl:]]' AND directions = btrim(directions)),
    phone text NOT NULL CHECK (phone = '' OR phone ~ '^\+?[0-9][0-9 -]{5,18}[0-9]$'),
    amenities text NOT NULL CHECK (length(amenities) <= 200 AND amenities !~ '[[:cntrl:]]' AND amenities = btrim(amenities)),
    parking text NOT NULL CHECK (length(parking) <= 200 AND parking !~ '[[:cntrl:]]' AND parking = btrim(parking)),
    closed_note text NOT NULL CHECK (length(closed_note) <= 120 AND closed_note !~ '[[:cntrl:]]' AND closed_note = btrim(closed_note)),
    -- NULL: the store has not said its hours.
    hours jsonb CHECK (hours IS NULL OR ledger.store_hours_valid(hours)),
    changed_by uuid NOT NULL,
    changed_at timestamptz NOT NULL
);
REVOKE ALL ON ledger.branch_store_page FROM PUBLIC, nu54_p05_commerce;

-- Whether the store takes orders now, and the store's own weekday and wall clock, so the app needs no time zone
-- arithmetic to say "open until 21:00". A closed day reads as closed, like the customer order state.
CREATE FUNCTION ledger.store_page_status(branch uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $status$
    SELECT jsonb_strip_nulls(jsonb_build_object(
        'mode', CASE WHEN EXISTS (SELECT FROM ledger.branch_closed_day d WHERE d.branch_id = b.id
                                    AND d.day = (t.at AT TIME ZONE b.timezone)::date) THEN 'closed'
                     ELSE coalesce(s.mode, 'open') END,
        'busyMinutes', s.busy_minutes,
        'resumeAt', s.resume_note,
        'weekday', extract(isodow FROM t.at AT TIME ZONE b.timezone)::int - 1,
        'time', to_char(t.at AT TIME ZONE b.timezone, 'HH24:MI'),
        'timezone', b.timezone))
    FROM ledger.branch b CROSS JOIN (SELECT clock_timestamp() AS at) t
    LEFT JOIN ledger.branch_order_state s ON s.branch_id = b.id
    WHERE b.id = branch
$status$;

-- One store's page for anyone who has its reference (the shortcut QR or the list). A store that wrote no page still
-- has its registered name and its order state; a branch without a registered name has no page.
CREATE FUNCTION ledger.public_store_page(branch uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $page$
    SELECT jsonb_strip_nulls(jsonb_build_object(
        'branchRef', b.id, 'name', n.label, 'kind', p.kind, 'intro', nullif(p.intro, ''),
        'address', nullif(p.address, ''), 'directions', nullif(p.directions, ''), 'phone', nullif(p.phone, ''),
        'amenities', nullif(p.amenities, ''), 'parking', nullif(p.parking, ''), 'closedNote', nullif(p.closed_note, ''),
        'hours', p.hours, 'status', ledger.store_page_status(b.id)))
    FROM ledger.branch b JOIN ledger.operator_branch_label n ON n.branch_id = b.id
    LEFT JOIN ledger.branch_store_page p ON p.branch_id = b.id
    WHERE b.id = branch
$page$;

-- The stores that chose to be listed, by name. At most limit_ rows; the app searches these by name.
CREATE FUNCTION ledger.public_store_list(limit_ integer) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $list$
    SELECT coalesce(jsonb_agg(x.store ORDER BY x.label, x.id), '[]'::jsonb) FROM (
        SELECT b.id, n.label, jsonb_strip_nulls(jsonb_build_object('branchRef', b.id, 'name', n.label, 'kind', p.kind,
            'address', nullif(p.address, ''), 'status', ledger.store_page_status(b.id))) AS store
        FROM ledger.branch_store_page p JOIN ledger.branch b ON b.id = p.branch_id
        JOIN ledger.operator_branch_label n ON n.branch_id = b.id
        WHERE p.listed ORDER BY n.label, b.id LIMIT greatest(1, least(coalesce(limit_, 200), 200))) x
$list$;

-- What the operations web shows and edits: the page as the store wrote it, or `saved: false` before the first save.
CREATE FUNCTION ledger.operator_store_page(tenant uuid, branch uuid, actor uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $read$
BEGIN
    PERFORM ledger.require_terminal_operator(tenant, branch, actor);
    RETURN (SELECT jsonb_build_object('name', n.label, 'saved', p.branch_id IS NOT NULL, 'listed', coalesce(p.listed, false),
            'kind', coalesce(p.kind, ''), 'intro', coalesce(p.intro, ''), 'address', coalesce(p.address, ''),
            'directions', coalesce(p.directions, ''), 'phone', coalesce(p.phone, ''), 'amenities', coalesce(p.amenities, ''),
            'parking', coalesce(p.parking, ''), 'closedNote', coalesce(p.closed_note, ''), 'hours', p.hours,
            'changedBy', p.changed_by, 'changedAt', p.changed_at)
        FROM ledger.branch b LEFT JOIN ledger.operator_branch_label n ON n.branch_id = b.id
        LEFT JOIN ledger.branch_store_page p ON p.branch_id = b.id WHERE b.id = branch);
END $read$;

-- The store writes its whole page at once. A store without a registered name cannot publish one.
CREATE FUNCTION ledger.set_store_page(tenant uuid, branch uuid, actor uuid, page jsonb) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $write$
DECLARE hours_ jsonb;
BEGIN
    PERFORM ledger.require_terminal_operator(tenant, branch, actor);
    IF jsonb_typeof(page) IS DISTINCT FROM 'object'
       OR page - ARRAY['listed','kind','intro','address','directions','phone','amenities','parking','closedNote','hours'] <> '{}'::jsonb
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
    INSERT INTO ledger.branch_store_page VALUES (branch, (page->>'listed')::boolean, btrim(page->>'kind'), btrim(page->>'intro'),
        btrim(page->>'address'), btrim(page->>'directions'), btrim(page->>'phone'), btrim(page->>'amenities'),
        btrim(page->>'parking'), btrim(page->>'closedNote'), hours_, actor, clock_timestamp())
    ON CONFLICT (branch_id) DO UPDATE SET listed = EXCLUDED.listed, kind = EXCLUDED.kind, intro = EXCLUDED.intro,
        address = EXCLUDED.address, directions = EXCLUDED.directions, phone = EXCLUDED.phone, amenities = EXCLUDED.amenities,
        parking = EXCLUDED.parking, closed_note = EXCLUDED.closed_note, hours = EXCLUDED.hours,
        changed_by = EXCLUDED.changed_by, changed_at = EXCLUDED.changed_at;
    RETURN ledger.operator_store_page(tenant, branch, actor);
END $write$;

REVOKE ALL ON FUNCTION ledger.store_hours_valid(jsonb), ledger.store_page_status(uuid), ledger.public_store_page(uuid),
    ledger.public_store_list(integer), ledger.operator_store_page(uuid, uuid, uuid),
    ledger.set_store_page(uuid, uuid, uuid, jsonb) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.public_store_page(uuid), ledger.public_store_list(integer),
    ledger.operator_store_page(uuid, uuid, uuid), ledger.set_store_page(uuid, uuid, uuid, jsonb) TO nu54_p05_commerce;

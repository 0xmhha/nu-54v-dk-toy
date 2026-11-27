-- Menu photos. The server keeps only what it made itself: a JPEG for a product's page and a small one for lists,
-- each named by its SHA-256. The upload is not kept, so nothing the camera wrote (place, device) stays.

CREATE TABLE ledger.menu_photo (
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    digest text NOT NULL CHECK (digest ~ '^[0-9a-f]{64}$'),
    thumb_digest text NOT NULL CHECK (thumb_digest ~ '^[0-9a-f]{64}$'),
    photo bytea NOT NULL CHECK (length(photo) BETWEEN 1 AND 153600),
    thumb bytea NOT NULL CHECK (length(thumb) BETWEEN 1 AND 18000),
    uploaded_by uuid NOT NULL,
    rights_confirmed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (branch_id, digest),
    UNIQUE (branch_id, thumb_digest),
    CHECK (encode(sha256(photo), 'hex') = digest AND encode(sha256(thumb), 'hex') = thumb_digest)
);
REVOKE ALL ON ledger.menu_photo FROM PUBLIC, nu54_p05_commerce;

-- The photo of each product in one published version.
CREATE TABLE ledger.menu_sku_photo (
    branch_id uuid NOT NULL,
    version bigint NOT NULL,
    sku text NOT NULL CHECK (sku ~ '^[a-z0-9][a-z0-9_-]{0,31}$'),
    digest text NOT NULL,
    PRIMARY KEY (branch_id, version, sku),
    FOREIGN KEY (branch_id, version) REFERENCES ledger.menu_release,
    FOREIGN KEY (branch_id, digest) REFERENCES ledger.menu_photo
);
REVOKE ALL ON ledger.menu_sku_photo FROM PUBLIC, nu54_p05_commerce;

-- An operator's upload. The same picture twice is one photo. At most 50 photos a day that no menu uses yet.
CREATE FUNCTION ledger.store_menu_photo(tenant uuid, branch uuid, actor uuid, photo_ bytea, thumb_ bytea)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE digest_ text := encode(sha256(photo_), 'hex');
BEGIN
    PERFORM ledger.require_terminal_operator(tenant, branch, actor);
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text || 'menu-photo', 0));
    IF NOT EXISTS (SELECT FROM ledger.menu_photo p WHERE (p.branch_id, p.digest) = (branch, digest_))
       AND (SELECT count(*) FROM ledger.menu_photo p WHERE p.branch_id = branch AND p.created_at > clock_timestamp() - interval '1 day'
            AND NOT EXISTS (SELECT FROM ledger.menu_sku_photo s WHERE (s.branch_id, s.digest) = (p.branch_id, p.digest))) >= 50 THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'too many unpublished menu photos';
    END IF;
    INSERT INTO ledger.menu_photo (branch_id, digest, thumb_digest, photo, thumb, uploaded_by, rights_confirmed_at)
    VALUES (branch, digest_, encode(sha256(thumb_), 'hex'), photo_, thumb_, actor, clock_timestamp())
    ON CONFLICT (branch_id, digest) DO NOTHING;
    RETURN (SELECT jsonb_build_object('photo', p.digest, 'thumb', p.thumb_digest, 'bytes', length(p.photo), 'thumbBytes', length(p.thumb))
            FROM ledger.menu_photo p WHERE (p.branch_id, p.digest) = (branch, digest_));
END $$;

-- The operator's own view of one photo of the branch, published or not.
CREATE FUNCTION ledger.operator_menu_photo(tenant uuid, branch uuid, actor uuid, digest_ text)
RETURNS bytea LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.require_terminal_operator(tenant, branch, actor);
    RETURN (SELECT p.photo FROM ledger.menu_photo p WHERE (p.branch_id, p.digest) = (branch, digest_));
END $$;

-- Binds the photos of a version just published. The same request again must name the same photos.
CREATE FUNCTION ledger.bind_menu_photos(tenant uuid, branch uuid, actor uuid, version_ bigint, photos jsonb)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.require_terminal_operator(tenant, branch, actor);
    IF jsonb_typeof(photos) IS DISTINCT FROM 'array' OR jsonb_array_length(photos) > 200
       OR EXISTS (SELECT FROM jsonb_array_elements(photos) e WHERE jsonb_typeof(e) IS DISTINCT FROM 'object' OR e - ARRAY['sku', 'photo'] <> '{}'
            OR NOT EXISTS (SELECT FROM ledger.price x WHERE (x.branch_id, x.version, x.sku) = (branch, version_, e->>'sku'))
            OR NOT EXISTS (SELECT FROM ledger.menu_photo p WHERE (p.branch_id, p.digest) = (branch, e->>'photo')))
       OR (SELECT count(DISTINCT e->>'sku') FROM jsonb_array_elements(photos) e) <> jsonb_array_length(photos) THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'invalid menu photos';
    END IF;
    IF EXISTS (SELECT FROM ledger.menu_sku_photo s WHERE (s.branch_id, s.version) = (branch, version_)) THEN
        IF (SELECT jsonb_agg(jsonb_build_object('sku', s.sku, 'photo', s.digest) ORDER BY s.sku) FROM ledger.menu_sku_photo s
              WHERE (s.branch_id, s.version) = (branch, version_))
           IS DISTINCT FROM (SELECT jsonb_agg(e ORDER BY e->>'sku') FROM jsonb_array_elements(photos) e) THEN
            RAISE EXCEPTION USING ERRCODE = 'P0501', MESSAGE = 'menu photos differ from the earlier request';
        END IF;
        RETURN;
    END IF;
    INSERT INTO ledger.menu_sku_photo SELECT branch, version_, e->>'sku', e->>'photo' FROM jsonb_array_elements(photos) e;
END $$;

-- The photos of the published version, for the operator's next edit.
CREATE FUNCTION ledger.operator_menu_photos(tenant uuid, branch uuid, actor uuid, version_ bigint)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.require_terminal_operator(tenant, branch, actor);
    RETURN coalesce((SELECT jsonb_agg(jsonb_build_object('sku', s.sku, 'photo', s.digest, 'thumb', p.thumb_digest,
            'bytes', length(p.photo), 'confirmedAt', p.rights_confirmed_at) ORDER BY s.sku COLLATE "C")
        FROM ledger.menu_sku_photo s JOIN ledger.menu_photo p USING (branch_id, digest)
        WHERE (s.branch_id, s.version) = (branch, version_)), '[]');
END $$;

-- A menu page for customers and terminals, with the photo names of its products.
CREATE FUNCTION ledger.catalog_menu_photo_window(tenant uuid, branch uuid, actor uuid, page integer)
RETURNS TABLE(snapshot_version bigint, items jsonb, options jsonb, photos jsonb)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT w.snapshot_version, w.items, w.options, coalesce((
        SELECT jsonb_agg(jsonb_build_object('sku', s.sku, 'photo', s.digest, 'thumb', p.thumb_digest) ORDER BY s.sku COLLATE "C")
        FROM ledger.menu_sku_photo s JOIN ledger.menu_photo p USING (branch_id, digest)
        WHERE s.branch_id = branch AND s.version = w.snapshot_version
          AND s.sku IN (SELECT item->>'sku' FROM jsonb_array_elements(w.items) item)), '[]')
    FROM ledger.catalog_menu_window(tenant, branch, actor, page) w
$$;

-- A part of a published photo, either size, by its name. Replies stay under the apps' 32 KB limit.
CREATE FUNCTION ledger.catalog_photo_part(tenant uuid, branch uuid, actor uuid, digest_ text, offset_ integer)
RETURNS TABLE(size integer, part bytea)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE image bytea;
BEGIN
    PERFORM ledger.commerce_allowed(tenant, branch, actor);
    SELECT CASE WHEN p.digest = digest_ THEN p.photo ELSE p.thumb END INTO image
      FROM ledger.menu_photo p WHERE p.branch_id = branch AND digest_ IN (p.digest, p.thumb_digest)
       AND EXISTS (SELECT FROM ledger.menu_sku_photo s WHERE (s.branch_id, s.digest) = (p.branch_id, p.digest));
    IF image IS NULL OR offset_ < 0 OR offset_ >= length(image) THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'menu photo not found';
    END IF;
    RETURN QUERY SELECT length(image), substring(image FROM offset_ + 1 FOR 18000);
END $$;

REVOKE ALL ON FUNCTION ledger.store_menu_photo(uuid, uuid, uuid, bytea, bytea), ledger.operator_menu_photo(uuid, uuid, uuid, text),
    ledger.bind_menu_photos(uuid, uuid, uuid, bigint, jsonb), ledger.operator_menu_photos(uuid, uuid, uuid, bigint),
    ledger.catalog_menu_photo_window(uuid, uuid, uuid, integer), ledger.catalog_photo_part(uuid, uuid, uuid, text, integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.store_menu_photo(uuid, uuid, uuid, bytea, bytea), ledger.operator_menu_photo(uuid, uuid, uuid, text),
    ledger.bind_menu_photos(uuid, uuid, uuid, bigint, jsonb), ledger.operator_menu_photos(uuid, uuid, uuid, bigint),
    ledger.catalog_menu_photo_window(uuid, uuid, uuid, integer), ledger.catalog_photo_part(uuid, uuid, uuid, text, integer) TO nu54_p05_commerce;

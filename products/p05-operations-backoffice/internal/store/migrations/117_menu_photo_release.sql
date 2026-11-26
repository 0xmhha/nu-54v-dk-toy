-- Two fixes to menu photos.
-- 1. The same picture uploaded again counts as a fresh upload, so the day before unpublished photos go starts again.
-- 2. Every publication records the photos it asked for, even none, so the same request sent again with other photos
--    is refused instead of adding photos to a version already published.

CREATE TABLE ledger.menu_photo_release (
    branch_id uuid NOT NULL,
    version bigint NOT NULL,
    photos jsonb NOT NULL CHECK (jsonb_typeof(photos) = 'array'),
    PRIMARY KEY (branch_id, version),
    FOREIGN KEY (branch_id, version) REFERENCES ledger.menu_release
);
REVOKE ALL ON ledger.menu_photo_release FROM PUBLIC, nu54_p05_commerce;

CREATE OR REPLACE FUNCTION ledger.store_menu_photo(tenant uuid, branch uuid, actor uuid, photo_ bytea, thumb_ bytea)
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
    ON CONFLICT (branch_id, digest) DO UPDATE SET created_at = clock_timestamp(), uploaded_by = EXCLUDED.uploaded_by,
        rights_confirmed_at = EXCLUDED.rights_confirmed_at;
    RETURN (SELECT jsonb_build_object('photo', p.digest, 'thumb', p.thumb_digest, 'bytes', length(p.photo), 'thumbBytes', length(p.thumb))
            FROM ledger.menu_photo p WHERE (p.branch_id, p.digest) = (branch, digest_));
END $$;

CREATE OR REPLACE FUNCTION ledger.bind_menu_photos(tenant uuid, branch uuid, actor uuid, version_ bigint, photos jsonb)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE asked jsonb;
BEGIN
    PERFORM ledger.require_terminal_operator(tenant, branch, actor);
    IF jsonb_typeof(photos) IS DISTINCT FROM 'array' OR jsonb_array_length(photos) > 200
       OR EXISTS (SELECT FROM jsonb_array_elements(photos) e WHERE jsonb_typeof(e) IS DISTINCT FROM 'object' OR e - ARRAY['sku', 'photo'] <> '{}'
            OR NOT EXISTS (SELECT FROM ledger.price x WHERE (x.branch_id, x.version, x.sku) = (branch, version_, e->>'sku'))
            OR NOT EXISTS (SELECT FROM ledger.menu_photo p WHERE (p.branch_id, p.digest) = (branch, e->>'photo')))
       OR (SELECT count(DISTINCT e->>'sku') FROM jsonb_array_elements(photos) e) <> jsonb_array_length(photos) THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'invalid menu photos';
    END IF;
    asked := coalesce((SELECT jsonb_agg(e ORDER BY e->>'sku') FROM jsonb_array_elements(photos) e), '[]');
    -- A version published before this migration has its photos only in menu_sku_photo.
    IF EXISTS (SELECT FROM ledger.menu_photo_release r WHERE (r.branch_id, r.version) = (branch, version_))
       OR EXISTS (SELECT FROM ledger.menu_sku_photo s WHERE (s.branch_id, s.version) = (branch, version_)) THEN
        IF coalesce((SELECT r.photos FROM ledger.menu_photo_release r WHERE (r.branch_id, r.version) = (branch, version_)),
                    (SELECT jsonb_agg(jsonb_build_object('sku', s.sku, 'photo', s.digest) ORDER BY s.sku) FROM ledger.menu_sku_photo s
                      WHERE (s.branch_id, s.version) = (branch, version_))) IS DISTINCT FROM asked THEN
            RAISE EXCEPTION USING ERRCODE = 'P0501', MESSAGE = 'menu photos differ from the earlier request';
        END IF;
        RETURN;
    END IF;
    INSERT INTO ledger.menu_photo_release VALUES (branch, version_, asked);
    INSERT INTO ledger.menu_sku_photo SELECT branch, version_, e->>'sku', e->>'photo' FROM jsonb_array_elements(photos) e;
END $$;

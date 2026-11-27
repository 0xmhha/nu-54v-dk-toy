-- An upload no published menu version names goes after one day. A photo a version still names stays with it.
CREATE INDEX menu_photo_created ON ledger.menu_photo (created_at);

CREATE FUNCTION ledger.prune_menu_photos() RETURNS integer
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    WITH gone AS (
        DELETE FROM ledger.menu_photo p WHERE (p.branch_id, p.digest) IN (
            SELECT o.branch_id, o.digest FROM ledger.menu_photo o
            WHERE o.created_at < clock_timestamp() - interval '1 day'
              AND NOT EXISTS (SELECT FROM ledger.menu_sku_photo s WHERE (s.branch_id, s.digest) = (o.branch_id, o.digest))
            ORDER BY o.created_at LIMIT 50 FOR UPDATE SKIP LOCKED)
        RETURNING 1)
    SELECT count(*)::integer FROM gone
$$;
REVOKE ALL ON FUNCTION ledger.prune_menu_photos() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.prune_menu_photos() TO nu54_p05_commerce;

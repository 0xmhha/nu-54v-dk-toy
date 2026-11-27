-- Declarations are part of the same immutable published breakdown and command
-- digest as price. Do not join to mutable inventory metadata or infer values.
CREATE FUNCTION ledger.catalog_published_page(tenant uuid,branch uuid,actor uuid,page integer)
RETURNS TABLE(version bigint,expires_at timestamptz,sku text,option_key text,label text,
    asset_ref text,unit_price numeric,available bigint,metadata jsonb)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT p.version,p.expires_at,p.sku,p.option_key,p.label,p.asset_ref,p.unit_price,p.available,
        (SELECT e.row->'metadata' FROM jsonb_array_elements(c.breakdown) e(row)
         WHERE e.row->>'sku'=p.sku AND e.row->>'option'=p.option_key)
    FROM ledger.catalog_stock_page(tenant,branch,actor,page) p
    LEFT JOIN ledger.catalog_pricing c ON c.branch_id=branch AND c.version=p.version
    ORDER BY p.sku,p.option_key
$$;
REVOKE ALL ON FUNCTION ledger.catalog_published_page(uuid,uuid,uuid,integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.catalog_published_page(uuid,uuid,uuid,integer) TO nu54_p05_commerce;

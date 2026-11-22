-- Project the immutable published SKU rule through the existing public page.
-- Keep old releases, row identities, paging and stock calculation unchanged.
CREATE OR REPLACE FUNCTION ledger.catalog_published_page(tenant uuid,branch uuid,actor uuid,page integer)
RETURNS TABLE(version bigint,expires_at timestamptz,sku text,option_key text,label text,
    asset_ref text,unit_price numeric,available bigint,metadata jsonb)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT p.version,p.expires_at,p.sku,p.option_key,p.label,p.asset_ref,p.unit_price,least(p.available,item.available),
        (SELECT CASE WHEN e.row->'required'='true'::jsonb
                    THEN coalesce(nullif(e.row->'metadata','null'::jsonb),'{}'::jsonb) || '{"optionRequired":true}'::jsonb
                    ELSE e.row->'metadata'
                END
         FROM jsonb_array_elements(c.breakdown) e(row)
         WHERE e.row->>'sku'=p.sku AND e.row->>'option'=p.option_key)
    FROM ledger.catalog_stock_page(tenant,branch,actor,page) p
    LEFT JOIN ledger.catalog_pricing c ON c.branch_id=branch AND c.version=p.version
    LEFT JOIN ledger.catalog_option_stock item ON (item.branch_id,item.sku,item.option_key)=(branch,p.sku,p.option_key)
    ORDER BY p.sku,p.option_key
$$;

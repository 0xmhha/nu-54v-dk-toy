CREATE TABLE ledger.menu_category (
    branch_id uuid NOT NULL,
    version bigint NOT NULL,
    category_key text NOT NULL CHECK(category_key ~ '^[a-z0-9][a-z0-9_-]{0,31}$'),
    sort_order integer NOT NULL CHECK(sort_order BETWEEN 0 AND 9999),
    labels jsonb NOT NULL CHECK(jsonb_typeof(labels)='object' AND labels ?& ARRAY['ko','en']
        AND labels-ARRAY['ko','en']='{}'::jsonb),
    PRIMARY KEY(branch_id,version,category_key),
    FOREIGN KEY(branch_id,version) REFERENCES ledger.menu_release
);

CREATE TABLE ledger.menu_sku_category (
    branch_id uuid NOT NULL,
    version bigint NOT NULL,
    sku text NOT NULL CHECK(sku ~ '^[a-z0-9][a-z0-9_-]{0,31}$'),
    category_key text NOT NULL,
    PRIMARY KEY(branch_id,version,sku),
    FOREIGN KEY(branch_id,version,category_key) REFERENCES ledger.menu_category
);
REVOKE ALL ON ledger.menu_category,ledger.menu_sku_category FROM PUBLIC,nu54_p05_commerce;

-- Keep the original price, receipt, and lock implementation. Only the new
-- category entry point may put a category commitment in its private intent row.
ALTER FUNCTION ledger.publish_operator_catalog_pricing(uuid,uuid,uuid,uuid,bigint,timestamptz,jsonb,jsonb)
    RENAME TO publish_catalog_pricing_internal;
REVOKE ALL ON FUNCTION ledger.publish_catalog_pricing_internal(uuid,uuid,uuid,uuid,bigint,timestamptz,jsonb,jsonb)
    FROM PUBLIC,nu54_p05_commerce;

CREATE FUNCTION ledger.publish_operator_catalog_pricing(tenant uuid,branch uuid,actor uuid,key_ uuid,
    expected bigint,deadline timestamptz,lines jsonb,hours_ jsonb)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF jsonb_typeof(lines)='array' AND EXISTS (SELECT FROM jsonb_array_elements(lines) item WHERE item ? '_categoryIntent') THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='reserved menu category intent field';
    END IF;
    RETURN ledger.publish_catalog_pricing_internal(tenant,branch,actor,key_,expected,deadline,lines,hours_);
END $$;

CREATE FUNCTION ledger.publish_operator_catalog_categories(tenant uuid,branch uuid,actor uuid,key_ uuid,
    expected bigint,deadline timestamptz,lines jsonb,hours_ jsonb,categories_ jsonb,bindings_ jsonb)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE ordered_categories jsonb;
DECLARE ordered_bindings jsonb;
DECLARE replay boolean;
DECLARE published jsonb;
DECLARE version_ bigint;
BEGIN
    PERFORM ledger.require_terminal_operator(tenant,branch,actor);
    IF jsonb_typeof(lines) IS DISTINCT FROM 'array' OR jsonb_array_length(lines) NOT BETWEEN 1 AND 150
       OR jsonb_typeof(categories_) IS DISTINCT FROM 'array' OR jsonb_array_length(categories_)>50
       OR jsonb_typeof(bindings_) IS DISTINCT FROM 'array' OR jsonb_array_length(bindings_)>150 THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid menu category publication shape';
    END IF;
    IF EXISTS (SELECT FROM jsonb_array_elements(lines) item WHERE item ? '_categoryIntent')
       OR EXISTS (
        SELECT FROM jsonb_array_elements(categories_) item
        WHERE jsonb_typeof(item) IS DISTINCT FROM 'object' OR item-ARRAY['key','sortOrder','labels']<>'{}'::jsonb
           OR jsonb_typeof(item->'key') IS DISTINCT FROM 'string' OR item->>'key' !~ '^[a-z0-9][a-z0-9_-]{0,31}$'
           OR jsonb_typeof(item->'sortOrder') IS DISTINCT FROM 'number' OR item->>'sortOrder' !~ '^(0|[1-9][0-9]{0,3})$'
           OR jsonb_typeof(item->'labels') IS DISTINCT FROM 'object' OR (item->'labels')-ARRAY['ko','en']<>'{}'::jsonb
           OR jsonb_typeof(item->'labels'->'ko') IS DISTINCT FROM 'string' OR jsonb_typeof(item->'labels'->'en') IS DISTINCT FROM 'string'
           OR EXISTS (SELECT FROM (VALUES (item->'labels'->>'ko'),(item->'labels'->>'en')) text_(label)
              WHERE label<>btrim(label) OR length(label) NOT BETWEEN 1 AND 80 OR label ~ '[[:cntrl:]]')
       ) OR EXISTS (
        SELECT FROM jsonb_array_elements(bindings_) item
        WHERE jsonb_typeof(item) IS DISTINCT FROM 'object' OR item-ARRAY['sku','categoryKey']<>'{}'::jsonb
           OR jsonb_typeof(item->'sku') IS DISTINCT FROM 'string' OR jsonb_typeof(item->'categoryKey') IS DISTINCT FROM 'string'
           OR NOT EXISTS (SELECT FROM jsonb_array_elements(categories_) c WHERE c->>'key'=item->>'categoryKey')
           OR NOT EXISTS (SELECT FROM jsonb_array_elements(lines) p WHERE p->>'sku'=item->>'sku')
       ) OR (SELECT count(DISTINCT item->>'key') FROM jsonb_array_elements(categories_) item)<>jsonb_array_length(categories_)
         OR (SELECT count(DISTINCT item->>'sku') FROM jsonb_array_elements(bindings_) item)<>jsonb_array_length(bindings_) THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid or conflicting menu category';
    END IF;
    SELECT coalesce(jsonb_agg(item ORDER BY (item->>'key') COLLATE "C"),'[]')
      INTO ordered_categories FROM jsonb_array_elements(categories_) item;
    SELECT coalesce(jsonb_agg(item ORDER BY (item->>'sku') COLLATE "C"),'[]')
      INTO ordered_bindings FROM jsonb_array_elements(bindings_) item;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||'operator-catalog',0));
    SELECT EXISTS (SELECT FROM ledger.operator_catalog_receipt r WHERE r.branch_id=branch AND r.actor_ref=actor
        AND r.operation='publish' AND r.request_key=key_) INTO replay;
    lines:=jsonb_set(lines,'{0}',(lines->0)||jsonb_build_object('_categoryIntent',
        encode(sha256(convert_to(jsonb_build_array('menu-category-v1',ordered_categories,ordered_bindings)::text,'UTF8')),'hex')));
    published:=ledger.publish_catalog_pricing_internal(tenant,branch,actor,key_,expected,deadline,lines,hours_);
    IF replay THEN RETURN published; END IF;
    version_:=(published->>'version')::bigint;
    UPDATE ledger.catalog_pricing SET breakdown=jsonb_set(breakdown,'{0}',(breakdown->0)-'_categoryIntent')
      WHERE branch_id=branch AND version=version_;
    INSERT INTO ledger.menu_category
      SELECT branch,version_,c.key,c."sortOrder",c.labels
      FROM jsonb_to_recordset(ordered_categories) c(key text,"sortOrder" int,labels jsonb);
    INSERT INTO ledger.menu_sku_category
      SELECT branch,version_,b.sku,b."categoryKey" FROM jsonb_to_recordset(ordered_bindings) b(sku text,"categoryKey" text);
    RETURN published;
END $$;

CREATE OR REPLACE FUNCTION ledger.operator_catalog_home(tenant uuid,branch uuid,actor uuid)
RETURNS jsonb LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    WITH base AS MATERIALIZED (
      SELECT ledger.operator_catalog_with_inventory(tenant,branch,actor) || jsonb_build_object('timezone',b.timezone) AS model
      FROM ledger.branch b WHERE b.id=branch
    )
    SELECT model || jsonb_build_object(
        'categories',coalesce((SELECT jsonb_agg(jsonb_build_object('key',c.category_key,'sortOrder',c.sort_order,'labels',c.labels)
            ORDER BY c.sort_order,c.category_key COLLATE "C") FROM ledger.menu_category c WHERE c.branch_id=branch
            AND c.version=(model->>'version')::bigint),'[]'::jsonb),
        'categoryBindings',coalesce((SELECT jsonb_agg(jsonb_build_object('sku',c.sku,'categoryKey',c.category_key)
            ORDER BY c.sku COLLATE "C") FROM ledger.menu_sku_category c WHERE c.branch_id=branch
            AND c.version=(model->>'version')::bigint),'[]'::jsonb))
    FROM base
$$;

CREATE FUNCTION ledger.catalog_category_window(tenant uuid,branch uuid,actor uuid,page integer)
RETURNS TABLE(snapshot_version bigint, items jsonb)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    WITH prices AS MATERIALIZED (
      SELECT p.*, CASE WHEN c.category_key IS NOT NULL
        THEN jsonb_build_object('key',c.category_key,'sortOrder',c.sort_order,'labels',c.labels) END AS category
      FROM ledger.catalog_published_stock_page(tenant,branch,actor,page) p
      LEFT JOIN ledger.menu_sku_category s ON (s.branch_id,s.version,s.sku)=(branch,p.version,p.sku)
      LEFT JOIN ledger.menu_category c ON (c.branch_id,c.version,c.category_key)=(s.branch_id,s.version,s.category_key)
    )
    SELECT coalesce((SELECT version FROM prices LIMIT 1),
                    (SELECT version FROM ledger.catalog_page(tenant,branch,actor,0) LIMIT 1)),
           coalesce((SELECT jsonb_agg(jsonb_build_object('version',p.version,'expiresAt',p.expires_at,'sku',p.sku,
               'optionKey',p.option_key,'label',p.label,'asset',p.asset_ref,'unitPrice',p.unit_price::text,
               'available',p.available,'skuAvailable',p.sku_available,'metadata',p.metadata,'category',p.category)
               ORDER BY p.sku,p.option_key) FROM prices p),'[]'::jsonb)
$$;

REVOKE ALL ON FUNCTION ledger.publish_operator_catalog_pricing(uuid,uuid,uuid,uuid,bigint,timestamptz,jsonb,jsonb),
    ledger.publish_operator_catalog_categories(uuid,uuid,uuid,uuid,bigint,timestamptz,jsonb,jsonb,jsonb,jsonb),
    ledger.catalog_category_window(uuid,uuid,uuid,integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.publish_operator_catalog_pricing(uuid,uuid,uuid,uuid,bigint,timestamptz,jsonb,jsonb),
    ledger.publish_operator_catalog_categories(uuid,uuid,uuid,uuid,bigint,timestamptz,jsonb,jsonb,jsonb,jsonb),
    ledger.catalog_category_window(uuid,uuid,uuid,integer) TO nu54_p05_commerce;

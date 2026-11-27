-- Independent choices (temperature, size, shots). A publication still has one price row for every combination, so prices,
-- stock, carts and receipts keep their shape; this table keeps the choices the rows were made from.

-- The one changed number is the row limit. The rest of each function stays as it was published.
DO $$
DECLARE function_ regprocedure;
DECLARE body text;
BEGIN
    FOREACH function_ IN ARRAY ARRAY[
        'ledger.publish_operator_catalog(uuid,uuid,uuid,uuid,bigint,timestamptz,jsonb)',
        'ledger.publish_catalog_pricing_internal(uuid,uuid,uuid,uuid,bigint,timestamptz,jsonb,jsonb)',
        'ledger.publish_operator_catalog_categories(uuid,uuid,uuid,uuid,bigint,timestamptz,jsonb,jsonb,jsonb,jsonb)']::regprocedure[]
    LOOP
        body:=pg_get_functiondef(function_);
        IF position('BETWEEN 1 AND 150' IN body)=0 THEN RAISE EXCEPTION 'row limit not found in %', function_; END IF;
        EXECUTE replace(replace(body,'BETWEEN 1 AND 150','BETWEEN 1 AND 600'),'jsonb_array_length(bindings_)>150','jsonb_array_length(bindings_)>600');
    END LOOP;
END $$;

CREATE TABLE ledger.menu_sku_options (
    branch_id uuid NOT NULL,
    version bigint NOT NULL,
    sku text NOT NULL CHECK(sku ~ '^[a-z0-9][a-z0-9_-]{0,31}$'),
    base jsonb NOT NULL CHECK(jsonb_typeof(base)='object'),
    groups jsonb NOT NULL CHECK(jsonb_typeof(groups)='array' AND jsonb_array_length(groups) BETWEEN 1 AND 4),
    PRIMARY KEY(branch_id,version,sku),
    FOREIGN KEY(branch_id,version) REFERENCES ledger.menu_release
);
REVOKE ALL ON ledger.menu_sku_options FROM PUBLIC,nu54_p05_commerce;

-- Only the options entry point may put its commitment in the private intent row.
ALTER FUNCTION ledger.publish_operator_catalog_categories(uuid,uuid,uuid,uuid,bigint,timestamptz,jsonb,jsonb,jsonb,jsonb)
    RENAME TO publish_catalog_categories_internal;
REVOKE ALL ON FUNCTION ledger.publish_catalog_categories_internal(uuid,uuid,uuid,uuid,bigint,timestamptz,jsonb,jsonb,jsonb,jsonb)
    FROM PUBLIC,nu54_p05_commerce;

CREATE FUNCTION ledger.publish_operator_catalog_categories(tenant uuid,branch uuid,actor uuid,key_ uuid,
    expected bigint,deadline timestamptz,lines jsonb,hours_ jsonb,categories_ jsonb,bindings_ jsonb)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF jsonb_typeof(lines)='array' AND EXISTS (SELECT FROM jsonb_array_elements(lines) item WHERE item ? '_optionIntent') THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='reserved menu option intent field';
    END IF;
    RETURN ledger.publish_catalog_categories_internal(tenant,branch,actor,key_,expected,deadline,lines,hours_,categories_,bindings_);
END $$;

CREATE OR REPLACE FUNCTION ledger.publish_operator_catalog_pricing(tenant uuid,branch uuid,actor uuid,key_ uuid,
    expected bigint,deadline timestamptz,lines jsonb,hours_ jsonb)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF jsonb_typeof(lines)='array' AND EXISTS (SELECT FROM jsonb_array_elements(lines) item
        WHERE item ?| ARRAY['_categoryIntent','_optionIntent']) THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='reserved menu intent field';
    END IF;
    RETURN ledger.publish_catalog_pricing_internal(tenant,branch,actor,key_,expected,deadline,lines,hours_);
END $$;

CREATE FUNCTION ledger.menu_options_valid(groups_ jsonb) RETURNS boolean
LANGUAGE sql IMMUTABLE SET search_path=pg_catalog,pg_temp AS $$
    WITH entries AS (
        SELECT g AS entry,'^[a-z0-9]{1,16}$' AS pattern FROM jsonb_array_elements(groups_) g
        UNION ALL
        SELECT v,'^[a-z0-9]{1,31}$' FROM jsonb_array_elements(groups_) g,
            jsonb_array_elements(CASE WHEN jsonb_typeof(g->'values')='array' THEN g->'values' ELSE '[]' END) v
    )
    SELECT jsonb_array_length(groups_) BETWEEN 1 AND 4
        AND NOT EXISTS (SELECT FROM jsonb_array_elements(groups_) g WHERE jsonb_typeof(g->'values') IS DISTINCT FROM 'array'
            OR jsonb_array_length(g->'values') NOT BETWEEN 2 AND 12
            OR (SELECT count(DISTINCT v->>'key') FROM jsonb_array_elements(g->'values') v)<>jsonb_array_length(g->'values'))
        AND (SELECT count(DISTINCT g->>'key') FROM jsonb_array_elements(groups_) g)=jsonb_array_length(groups_)
        AND NOT EXISTS (SELECT FROM entries WHERE jsonb_typeof(entry->'key') IS DISTINCT FROM 'string' OR entry->>'key' !~ pattern
            OR jsonb_typeof(entry->'labels') IS DISTINCT FROM 'object' OR NOT entry->'labels' ? 'ko'
            OR (entry->'labels')-ARRAY['ko','en']<>'{}'
            OR EXISTS (SELECT FROM jsonb_each(entry->'labels') l WHERE jsonb_typeof(l.value)<>'string'
                OR btrim(l.value#>>'{}')<>l.value#>>'{}' OR length(l.value#>>'{}') NOT BETWEEN 1 AND 24 OR l.value#>>'{}' ~ '[[:cntrl:]]'))
$$;

CREATE FUNCTION ledger.publish_operator_catalog_options(tenant uuid,branch uuid,actor uuid,key_ uuid,expected bigint,
    deadline timestamptz,lines jsonb,hours_ jsonb,categories_ jsonb,bindings_ jsonb,options_ jsonb)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE option_ jsonb;
DECLARE group_ jsonb;
DECLARE keys_ text[];
DECLARE ordered jsonb;
DECLARE replay boolean;
DECLARE published jsonb;
DECLARE version_ bigint;
BEGIN
    PERFORM ledger.require_terminal_operator(tenant,branch,actor);
    IF jsonb_typeof(lines) IS DISTINCT FROM 'array' OR jsonb_array_length(lines) NOT BETWEEN 1 AND 600
       OR EXISTS (SELECT FROM jsonb_array_elements(lines) item WHERE item ?| ARRAY['_categoryIntent','_optionIntent'])
       OR jsonb_typeof(options_) IS DISTINCT FROM 'array' OR jsonb_array_length(options_) NOT BETWEEN 1 AND 300
       OR EXISTS (SELECT FROM jsonb_array_elements(options_) o WHERE jsonb_typeof(o) IS DISTINCT FROM 'object'
            OR o-ARRAY['sku','base','groups']<>'{}' OR jsonb_typeof(o->'sku') IS DISTINCT FROM 'string'
            OR jsonb_typeof(o->'base') IS DISTINCT FROM 'object' OR jsonb_typeof(o->'groups') IS DISTINCT FROM 'array')
       OR (SELECT count(DISTINCT o->>'sku') FROM jsonb_array_elements(options_) o)<>jsonb_array_length(options_) THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid menu option publication shape';
    END IF;
    FOR option_ IN SELECT o FROM jsonb_array_elements(options_) o LOOP
        IF NOT ledger.menu_options_valid(option_->'groups') THEN
            RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid menu option group';
        END IF;
        keys_:=ARRAY[''];
        FOR group_ IN SELECT g FROM jsonb_array_elements(option_->'groups') WITH ORDINALITY x(g,seq) ORDER BY seq LOOP
            SELECT array_agg(CASE WHEN k='' THEN '' ELSE k||'-' END||(v->>'key')) INTO keys_
                FROM unnest(keys_) k, jsonb_array_elements(group_->'values') v;
            IF cardinality(keys_)>600 THEN RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='too many option combinations'; END IF;
        END LOOP;
        -- Exactly one row, and a required choice, for every combination of this product.
        IF cardinality(keys_)<>(SELECT count(*) FROM jsonb_array_elements(lines) p WHERE p->>'sku'=option_->>'sku')
           OR EXISTS (SELECT FROM jsonb_array_elements(lines) p WHERE p->>'sku'=option_->>'sku'
                AND (p->'required' IS DISTINCT FROM 'true'::jsonb OR NOT coalesce(p->>'option',''::text)=ANY(keys_)))
           OR cardinality(keys_)<>(SELECT count(DISTINCT p->>'option') FROM jsonb_array_elements(lines) p WHERE p->>'sku'=option_->>'sku') THEN
            RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='option rows differ from combinations';
        END IF;
    END LOOP;
    SELECT jsonb_agg(o ORDER BY (o->>'sku') COLLATE "C") INTO ordered FROM jsonb_array_elements(options_) o;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||'operator-catalog',0));
    SELECT EXISTS (SELECT FROM ledger.operator_catalog_receipt r WHERE r.branch_id=branch AND r.actor_ref=actor
        AND r.operation='publish' AND r.request_key=key_) INTO replay;
    lines:=jsonb_set(lines,'{0}',(lines->0)||jsonb_build_object('_optionIntent',
        encode(sha256(convert_to(jsonb_build_array('menu-options-v1',ordered)::text,'UTF8')),'hex')));
    published:=ledger.publish_catalog_categories_internal(tenant,branch,actor,key_,expected,deadline,lines,hours_,categories_,bindings_);
    IF replay THEN RETURN published; END IF;
    version_:=(published->>'version')::bigint;
    UPDATE ledger.catalog_pricing SET breakdown=jsonb_set(breakdown,'{0}',(breakdown->0)-'_optionIntent')
      WHERE branch_id=branch AND version=version_;
    INSERT INTO ledger.menu_sku_options
      SELECT branch,version_,o->>'sku',o->'base',o->'groups' FROM jsonb_array_elements(ordered) o;
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
            AND c.version=(model->>'version')::bigint),'[]'::jsonb),
        'options',coalesce((SELECT jsonb_agg(jsonb_build_object('sku',o.sku,'base',o.base,'groups',o.groups)
            ORDER BY o.sku COLLATE "C") FROM ledger.menu_sku_options o WHERE o.branch_id=branch
            AND o.version=(model->>'version')::bigint),'[]'::jsonb))
    FROM base
$$;

-- The choices of the products on one page, without the amounts the operator typed.
CREATE FUNCTION ledger.catalog_menu_window(tenant uuid,branch uuid,actor uuid,page integer)
RETURNS TABLE(snapshot_version bigint,items jsonb,options jsonb)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT w.snapshot_version,w.items,coalesce((
        SELECT jsonb_agg(jsonb_build_object('sku',o.sku,'groups',(
            SELECT jsonb_agg(jsonb_build_object('key',g->'key','labels',g->'labels','values',(
                SELECT jsonb_agg(jsonb_build_object('key',v->'key','labels',v->'labels') ORDER BY place)
                FROM jsonb_array_elements(g->'values') WITH ORDINALITY x(v,place))) ORDER BY seq)
            FROM jsonb_array_elements(o.groups) WITH ORDINALITY y(g,seq))) ORDER BY o.sku COLLATE "C")
        FROM ledger.menu_sku_options o WHERE o.branch_id=branch AND o.version=w.snapshot_version
          AND o.sku IN (SELECT item->>'sku' FROM jsonb_array_elements(w.items) item)),'[]')
    FROM ledger.catalog_category_window(tenant,branch,actor,page) w
$$;

REVOKE ALL ON FUNCTION ledger.menu_options_valid(jsonb),
    ledger.publish_operator_catalog_categories(uuid,uuid,uuid,uuid,bigint,timestamptz,jsonb,jsonb,jsonb,jsonb),
    ledger.publish_operator_catalog_options(uuid,uuid,uuid,uuid,bigint,timestamptz,jsonb,jsonb,jsonb,jsonb,jsonb),
    ledger.catalog_menu_window(uuid,uuid,uuid,integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION
    ledger.publish_operator_catalog_categories(uuid,uuid,uuid,uuid,bigint,timestamptz,jsonb,jsonb,jsonb,jsonb),
    ledger.publish_operator_catalog_options(uuid,uuid,uuid,uuid,bigint,timestamptz,jsonb,jsonb,jsonb,jsonb,jsonb),
    ledger.catalog_menu_window(uuid,uuid,uuid,integer) TO nu54_p05_commerce;

-- The key of a combination is a code the console makes up, so staff and the dispute page also get its name.

CREATE FUNCTION ledger.order_option_label(branch uuid,version_ bigint,sku_ text,option_ text) RETURNS text
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT string_agg(v->'labels'->>'ko',' · ' ORDER BY seq)
    FROM ledger.menu_sku_options o, jsonb_array_elements(o.groups) WITH ORDINALITY x(g,seq), jsonb_array_elements(g->'values') v
    WHERE (o.branch_id,o.version,o.sku)=(branch,version_,sku_) AND v->>'key'=split_part(option_,'-',seq::int)
$$;
REVOKE ALL ON FUNCTION ledger.order_option_label(uuid,bigint,text,text) FROM PUBLIC;

-- Added next to the key: an installed kitchen app refuses a longer key. Both callers leave out a null.
DO $$
DECLARE function_ regprocedure;
DECLARE body text;
DECLARE key_ constant text := '''option'',l.option_key,';
BEGIN
    FOREACH function_ IN ARRAY ARRAY[
        'ledger.kiosk_pickup_queue(uuid,uuid,uuid,uuid,text,boolean)',
        'ledger.escrow_order_page(uuid,uuid,text,uuid,uuid)']::regprocedure[]
    LOOP
        body:=pg_get_functiondef(function_);
        IF (length(body)-length(replace(body,key_,'')))<>length(key_) THEN RAISE EXCEPTION 'order lines not found in %', function_; END IF;
        EXECUTE replace(body,key_,key_||'''optionLabel'',ledger.order_option_label(l.branch_id,l.menu_version,l.sku,l.option_key),');
    END LOOP;
END $$;

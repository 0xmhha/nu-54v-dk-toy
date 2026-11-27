CREATE TABLE ledger.catalog_option_stock(
    branch_id uuid NOT NULL,
    sku text NOT NULL,
    option_key text NOT NULL CHECK(option_key ~ '^[a-z0-9][a-z0-9_-]{0,31}$'),
    available bigint NOT NULL CHECK(available BETWEEN 0 AND 9007199254740991),
    PRIMARY KEY(branch_id,sku,option_key),
    FOREIGN KEY(branch_id,sku) REFERENCES ledger.stock
);
CREATE TABLE ledger.catalog_option_hold(
    sale_id uuid NOT NULL,
    branch_id uuid NOT NULL,
    sku text NOT NULL,
    option_key text NOT NULL,
    quantity bigint NOT NULL CHECK(quantity>0),
    PRIMARY KEY(sale_id,sku),
    FOREIGN KEY(branch_id,sale_id) REFERENCES ledger.sale(branch_id,id),
    FOREIGN KEY(branch_id,sku,option_key) REFERENCES ledger.catalog_option_stock
);
REVOKE ALL ON ledger.catalog_option_stock,ledger.catalog_option_hold FROM PUBLIC;

CREATE FUNCTION ledger.reserve_catalog_option() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    UPDATE ledger.catalog_option_stock s SET available=s.available-NEW.quantity
      WHERE (s.branch_id,s.sku,s.option_key)=(NEW.branch_id,NEW.sku,NEW.option_key)
        AND s.available>=NEW.quantity;
    IF FOUND THEN
        INSERT INTO ledger.catalog_option_hold VALUES(NEW.sale_id,NEW.branch_id,NEW.sku,NEW.option_key,NEW.quantity);
    ELSIF EXISTS (SELECT FROM ledger.catalog_option_stock s WHERE (s.branch_id,s.sku,s.option_key)=(NEW.branch_id,NEW.sku,NEW.option_key)) THEN
        RAISE EXCEPTION USING ERRCODE='P0701',MESSAGE='insufficient option stock';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER reserve_catalog_option AFTER INSERT ON ledger.order_line
    FOR EACH ROW EXECUTE FUNCTION ledger.reserve_catalog_option();

CREATE FUNCTION ledger.restore_catalog_option() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF OLD.state<>'closed' AND NEW.state='closed' THEN
        WITH released AS (DELETE FROM ledger.catalog_option_hold h WHERE h.sale_id=NEW.id RETURNING *)
        UPDATE ledger.catalog_option_stock s SET available=s.available+r.quantity FROM released r
        WHERE (s.branch_id,s.sku,s.option_key)=(r.branch_id,r.sku,r.option_key);
    ELSIF NEW.state='paid' THEN
        DELETE FROM ledger.catalog_option_hold h WHERE h.sale_id=NEW.id;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER restore_catalog_option AFTER UPDATE OF state ON ledger.sale
    FOR EACH ROW EXECUTE FUNCTION ledger.restore_catalog_option();
REVOKE ALL ON FUNCTION ledger.reserve_catalog_option(),ledger.restore_catalog_option() FROM PUBLIC;

ALTER TABLE ledger.operator_catalog_receipt DROP CONSTRAINT operator_catalog_receipt_operation_check;
ALTER TABLE ledger.operator_catalog_receipt ADD CHECK(operation IN ('publish','stock','option-stock'));

CREATE FUNCTION ledger.adjust_catalog_option(tenant uuid,branch uuid,actor uuid,key_ uuid,
    expected bigint,sku_ text,option_ text,delta bigint)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE prior ledger.operator_catalog_receipt;
DECLARE command_policy ledger.command_rule;
DECLARE count_ bigint;
DECLARE intent bytea;
DECLARE result jsonb;
BEGIN
    PERFORM ledger.require_terminal_operator(tenant,branch,actor);
    IF delta IS NULL OR delta NOT BETWEEN -1000000 AND 1000000 THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid option inventory change';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||'operator-catalog',0));
    intent:=sha256(convert_to(jsonb_build_array(expected,sku_,option_,delta)::text,'UTF8'));
    SELECT * INTO prior FROM ledger.operator_catalog_receipt o WHERE o.branch_id=branch AND o.actor_ref=actor
        AND o.request_key=key_ AND o.operation='option-stock';
    IF FOUND THEN
        IF prior.intent_digest<>intent THEN RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='request conflict'; END IF;
        IF prior.retire_after<=statement_timestamp() THEN RAISE EXCEPTION USING ERRCODE='P0502', MESSAGE='receipt expired'; END IF;
        RETURN prior.result;
    END IF;
    SELECT * INTO command_policy FROM ledger.command_rule c WHERE c.branch_id=branch FOR SHARE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='stock policy unavailable'; END IF;
    IF (SELECT max(m.version) FROM ledger.menu_release m WHERE m.branch_id=branch) IS DISTINCT FROM expected
        OR NOT EXISTS (SELECT FROM ledger.price p WHERE p.branch_id=branch AND p.version=expected AND p.sku=sku_ AND p.option_key=option_) THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='catalog changed';
    END IF;
    INSERT INTO ledger.catalog_option_stock VALUES(branch,sku_,option_,0) ON CONFLICT DO NOTHING;
    UPDATE ledger.catalog_option_stock s SET available=s.available+delta
      WHERE (s.branch_id,s.sku,s.option_key)=(branch,sku_,option_)
        AND s.available+delta BETWEEN 0 AND 9007199254740991
        AND s.available+delta+coalesce((SELECT sum(h.quantity) FROM ledger.catalog_option_hold h
            WHERE (h.branch_id,h.sku,h.option_key)=(branch,sku_,option_)),0)<=9007199254740991
      RETURNING s.available INTO count_;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0701', MESSAGE='option stock limit'; END IF;
    result:=jsonb_build_object('version',expected,'sku',sku_,'option',option_,'available',count_);
    INSERT INTO ledger.operator_catalog_receipt VALUES(branch,actor,'option-stock',key_,intent,result,
        statement_timestamp()+command_policy.receipt_window);
    RETURN result;
END $$;

CREATE OR REPLACE FUNCTION ledger.catalog_published_page(tenant uuid,branch uuid,actor uuid,page integer)
RETURNS TABLE(version bigint,expires_at timestamptz,sku text,option_key text,label text,
    asset_ref text,unit_price numeric,available bigint,metadata jsonb)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT p.version,p.expires_at,p.sku,p.option_key,p.label,p.asset_ref,p.unit_price,least(p.available,item.available),
        (SELECT e.row->'metadata' FROM jsonb_array_elements(c.breakdown) e(row)
         WHERE e.row->>'sku'=p.sku AND e.row->>'option'=p.option_key)
    FROM ledger.catalog_stock_page(tenant,branch,actor,page) p
    LEFT JOIN ledger.catalog_pricing c ON c.branch_id=branch AND c.version=p.version
    LEFT JOIN ledger.catalog_option_stock item ON (item.branch_id,item.sku,item.option_key)=(branch,p.sku,p.option_key)
    ORDER BY p.sku,p.option_key
$$;

CREATE FUNCTION ledger.operator_catalog_with_inventory(tenant uuid,branch uuid,actor uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE answer jsonb;
DECLARE entries jsonb;
BEGIN
    answer:=ledger.operator_catalog_pricing(tenant,branch,actor);
    SELECT coalesce(jsonb_agg(r.value || jsonb_build_object('optionAvailable',s.available) ORDER BY r.seq),'[]'::jsonb)
       INTO entries FROM jsonb_array_elements(answer->'rows') WITH ORDINALITY r(value,seq)
       LEFT JOIN ledger.catalog_option_stock s ON (s.branch_id,s.sku,s.option_key)=(branch,r.value->>'sku',r.value->>'option');
    RETURN jsonb_set(answer,'{rows}',entries);
END $$;

REVOKE ALL ON FUNCTION ledger.adjust_catalog_option(uuid,uuid,uuid,uuid,bigint,text,text,bigint),
    ledger.operator_catalog_with_inventory(uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.adjust_catalog_option(uuid,uuid,uuid,uuid,bigint,text,text,bigint),
    ledger.operator_catalog_with_inventory(uuid,uuid,uuid) TO nu54_p05_commerce;

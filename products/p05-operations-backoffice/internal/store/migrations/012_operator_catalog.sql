CREATE TABLE ledger.operator_catalog_receipt (
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    actor_ref uuid NOT NULL REFERENCES ledger.login_subject(subject_ref),
    operation text NOT NULL CHECK (operation IN ('publish','stock')),
    request_key uuid NOT NULL,
    intent_digest bytea NOT NULL CHECK (octet_length(intent_digest)=32),
    result jsonb NOT NULL,
    retire_after timestamptz NOT NULL CHECK (isfinite(retire_after)),
    PRIMARY KEY (branch_id,actor_ref,operation,request_key)
);
REVOKE ALL ON ledger.operator_catalog_receipt FROM PUBLIC;

CREATE FUNCTION ledger.operator_catalog(tenant uuid,branch uuid,actor uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE latest ledger.menu_release;
BEGIN
    PERFORM ledger.require_terminal_operator(tenant,branch,actor);
    SELECT * INTO latest FROM ledger.menu_release m WHERE m.branch_id=branch ORDER BY m.version DESC LIMIT 1;
    RETURN jsonb_build_object('version',coalesce(latest.version,0),'expiresAt',latest.expires_at,
        'rows',coalesce((SELECT jsonb_agg(jsonb_build_object('sku',p.sku,'option',p.option_key,'label',p.label,
            'asset',p.asset_ref,'price',p.unit_price::text,'available',coalesce(s.available,0)) ORDER BY p.sku,p.option_key)
            FROM ledger.price p LEFT JOIN ledger.stock s ON s.branch_id=p.branch_id AND s.sku=p.sku
            WHERE p.branch_id=branch AND p.version=latest.version),'[]'::jsonb));
END $$;

CREATE FUNCTION ledger.publish_operator_catalog(tenant uuid,branch uuid,actor uuid,key_ uuid,
    expected bigint,deadline timestamptz,lines jsonb)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE prior ledger.operator_catalog_receipt;
    policy ledger.command_rule;
    version_ bigint;
    fingerprint bytea;
    response jsonb;
BEGIN
    PERFORM ledger.require_terminal_operator(tenant,branch,actor);
    IF key_ IS NULL OR expected<0 OR expected>=9007199254740991 OR
       NOT isfinite(deadline) OR jsonb_typeof(lines) IS DISTINCT FROM 'array' OR
       jsonb_array_length(lines) NOT BETWEEN 1 AND 150 THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid catalog publication';
    END IF;
    fingerprint := sha256(convert_to(jsonb_build_array(expected,extract(epoch from deadline),lines)::text,'UTF8'));
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||'operator-catalog',0));
    SELECT * INTO prior FROM ledger.operator_catalog_receipt p WHERE p.branch_id=branch AND p.actor_ref=actor
      AND p.operation='publish' AND p.request_key=key_;
    IF FOUND THEN
        IF prior.intent_digest<>fingerprint THEN RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='request conflict'; END IF;
        IF prior.retire_after<=statement_timestamp() THEN RAISE EXCEPTION USING ERRCODE='P0502', MESSAGE='receipt expired'; END IF;
        RETURN prior.result;
    END IF;
    IF deadline<statement_timestamp()+interval '1 minute' OR deadline>statement_timestamp()+interval '30 days' THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid publication window';
    END IF;
    SELECT * INTO policy FROM ledger.command_rule c WHERE c.branch_id=branch FOR SHARE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='catalog policy unavailable'; END IF;
    SELECT coalesce(max(r.version),0) INTO version_ FROM ledger.menu_release r WHERE r.branch_id=branch;
    IF version_<>expected THEN RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='catalog changed'; END IF;

    INSERT INTO ledger.menu_release VALUES (branch,version_+1,statement_timestamp(),deadline);
    INSERT INTO ledger.price(branch_id,version,sku,option_key,label,asset_ref,unit_price)
      SELECT branch,version_+1,line.sku,line.option,line.label,line.asset,line.price::ledger.units
      FROM jsonb_to_recordset(lines) AS line(sku text,option text,label text,asset text,price text);
    INSERT INTO ledger.stock(branch_id,sku,available)
      SELECT branch,sku,0 FROM ledger.price WHERE branch_id=branch AND version=version_+1 GROUP BY sku ORDER BY sku
      ON CONFLICT (branch_id,sku) DO NOTHING;
    response:=jsonb_build_object('version',version_+1);
    INSERT INTO ledger.operator_catalog_receipt VALUES (branch,actor,'publish',key_,fingerprint,response,
      statement_timestamp()+policy.receipt_window);
    RETURN response;
END $$;

CREATE FUNCTION ledger.adjust_operator_stock(tenant uuid,branch uuid,actor uuid,key_ uuid,
    expected bigint,sku_ text,delta bigint)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE prior ledger.operator_catalog_receipt;
    policy ledger.command_rule;
    available_ bigint;
    fingerprint bytea;
    response jsonb;
BEGIN
    PERFORM ledger.require_terminal_operator(tenant,branch,actor);
    IF key_ IS NULL OR expected<1 OR sku_ !~ '^[a-z0-9][a-z0-9_-]{0,31}$' OR delta IS NULL OR delta NOT BETWEEN -1000000 AND 1000000 OR delta=0 THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid inventory change';
    END IF;
    fingerprint:=sha256(convert_to(jsonb_build_array(expected,sku_,delta)::text,'UTF8'));
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||'operator-catalog',0));
    SELECT * INTO prior FROM ledger.operator_catalog_receipt p WHERE p.branch_id=branch AND p.actor_ref=actor
      AND p.operation='stock' AND p.request_key=key_;
    IF FOUND THEN
        IF prior.intent_digest<>fingerprint THEN RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='request conflict'; END IF;
        IF prior.retire_after<=statement_timestamp() THEN RAISE EXCEPTION USING ERRCODE='P0502', MESSAGE='receipt expired'; END IF;
        RETURN prior.result;
    END IF;
    SELECT * INTO policy FROM ledger.command_rule c WHERE c.branch_id=branch FOR SHARE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='catalog policy unavailable'; END IF;
    IF (SELECT max(m.version) FROM ledger.menu_release m WHERE m.branch_id=branch) IS DISTINCT FROM expected OR
       NOT EXISTS(SELECT FROM ledger.price p WHERE p.branch_id=branch AND p.version=expected AND p.sku=sku_) THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='catalog changed';
    END IF;
    UPDATE ledger.stock s SET available=s.available+delta WHERE s.branch_id=branch AND s.sku=sku_
      AND s.available+delta BETWEEN 0 AND 9007199254740991 RETURNING s.available INTO available_;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0701', MESSAGE='inventory limit'; END IF;
    response:=jsonb_build_object('version',expected,'sku',sku_,'available',available_);
    INSERT INTO ledger.operator_catalog_receipt VALUES(branch,actor,'stock',key_,fingerprint,response,
      statement_timestamp()+policy.receipt_window);
    RETURN response;
END $$;

CREATE FUNCTION ledger.expire_operator_catalog_receipts()
RETURNS void LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    DELETE FROM ledger.operator_catalog_receipt r WHERE (r.branch_id,r.actor_ref,r.operation,r.request_key) IN (
      SELECT branch_id,actor_ref,operation,request_key FROM ledger.operator_catalog_receipt WHERE retire_after<=statement_timestamp()
      ORDER BY retire_after LIMIT 120 FOR UPDATE SKIP LOCKED
    );
$$;
REVOKE ALL ON FUNCTION ledger.operator_catalog(uuid,uuid,uuid),
    ledger.publish_operator_catalog(uuid,uuid,uuid,uuid,bigint,timestamptz,jsonb),
    ledger.adjust_operator_stock(uuid,uuid,uuid,uuid,bigint,text,bigint),ledger.expire_operator_catalog_receipts() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.operator_catalog(uuid,uuid,uuid),
    ledger.publish_operator_catalog(uuid,uuid,uuid,uuid,bigint,timestamptz,jsonb),
    ledger.adjust_operator_stock(uuid,uuid,uuid,uuid,bigint,text,bigint),ledger.expire_operator_catalog_receipts() TO nu54_p05_commerce;

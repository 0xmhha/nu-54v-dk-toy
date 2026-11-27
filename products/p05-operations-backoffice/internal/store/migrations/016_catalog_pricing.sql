CREATE TABLE ledger.catalog_pricing(
    branch_id uuid NOT NULL,
    version bigint NOT NULL,
    hours jsonb CHECK (hours IS NULL OR jsonb_typeof(hours)='object'),
    breakdown jsonb NOT NULL CHECK (jsonb_typeof(breakdown)='array'),
    PRIMARY KEY(branch_id,version),
    FOREIGN KEY(branch_id,version) REFERENCES ledger.menu_release
);
REVOKE ALL ON ledger.catalog_pricing FROM PUBLIC;

CREATE FUNCTION ledger.catalog_is_open(branch uuid,menu bigint,at_time timestamptz)
RETURNS boolean LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT coalesce((SELECT (at_time AT TIME ZONE (p.hours->>'zone'))::time >= (p.hours->>'opens')::time
                            AND (at_time AT TIME ZONE (p.hours->>'zone'))::time < (p.hours->>'closes')::time
                     FROM ledger.catalog_pricing p WHERE p.branch_id=branch AND p.version=menu),true);
$$;
REVOKE ALL ON FUNCTION ledger.catalog_is_open(uuid,bigint,timestamptz) FROM PUBLIC;

CREATE FUNCTION ledger.operator_catalog_pricing(tenant uuid,branch uuid,actor uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE result jsonb;
DECLARE publication ledger.catalog_pricing;
BEGIN
    result:=ledger.operator_catalog(tenant,branch,actor);
    SELECT * INTO publication FROM ledger.catalog_pricing p WHERE p.branch_id=branch AND p.version=(result->>'version')::bigint;
    RETURN result || jsonb_build_object('hours',publication.hours,'breakdown',coalesce(publication.breakdown,'[]'::jsonb));
END $$;

CREATE FUNCTION ledger.publish_operator_catalog_pricing(tenant uuid,branch uuid,actor uuid,key_ uuid,
    expected bigint,deadline timestamptz,lines jsonb,hours_ jsonb)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE result jsonb;
DECLARE receipt_lines jsonb := lines;
DECLARE version_ bigint;
BEGIN
    PERFORM ledger.require_terminal_operator(tenant,branch,actor);
    hours_ := nullif(hours_,'null'::jsonb);
    IF hours_ IS NOT NULL AND (
       hours_->>'opens' !~ '^([01][0-9]|2[0-3]):[0-5][0-9]$' OR
       hours_->>'closes' !~ '^([01][0-9]|2[0-3]):[0-5][0-9]$' OR
       hours_->>'opens' >= hours_->>'closes' OR
       NOT EXISTS (SELECT FROM pg_timezone_names t WHERE t.name=hours_->>'zone') OR
       NOT (hours_ ?& array['opens','closes','zone'])
    ) THEN
       RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid business hours';
    END IF;
    IF jsonb_typeof(lines) IS DISTINCT FROM 'array' OR jsonb_array_length(lines) NOT BETWEEN 1 AND 150 THEN
       RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid price list';
    END IF;
    IF EXISTS (SELECT FROM jsonb_to_recordset(lines) p(price text,"displayPrice" text,tax text,discount text,required boolean,option text)
               WHERE (hours_ IS NOT NULL OR p.tax IS NOT NULL OR p.discount IS NOT NULL OR p.required)
                     AND p."displayPrice" IS NULL
                  OR p."displayPrice" IS NOT NULL AND (
                     p.tax IS NULL OR p.discount IS NULL OR
                     p."displayPrice"::ledger.units + p.tax::ledger.units - p.discount::ledger.units IS DISTINCT FROM p.price::ledger.units
                  )) THEN
       RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid price breakdown';
    END IF;
    -- A required choice has named options and no implicit/default option. Every
    -- row of a SKU declares the same choice policy. The reservation still carries
    -- the exact selected option key, even for the optional standard option.
    IF EXISTS (
       SELECT p.sku FROM jsonb_to_recordset(lines) p(sku text,option text,required boolean,"displayPrice" text)
       GROUP BY p.sku HAVING count(p."displayPrice") NOT IN (0,count(*))
         OR bool_or(coalesce(p.required,false)) AND
            (bool_or(NOT coalesce(p.required,false)) OR bool_or(p.option='standard'))
    ) THEN
       RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid option group';
    END IF;
    -- Include the hours in the original receipt digest without changing the price row schema.
    IF hours_ IS NOT NULL THEN
        receipt_lines:=jsonb_set(lines,'{0}',(lines->0)||jsonb_build_object('hours',hours_));
    END IF;
    result:=ledger.publish_operator_catalog(tenant,branch,actor,key_,expected,deadline,receipt_lines);
    version_:=(result->>'version')::bigint;
    IF EXISTS (SELECT FROM jsonb_array_elements(lines) p WHERE p ? 'displayPrice') THEN
        INSERT INTO ledger.catalog_pricing VALUES(branch,version_,hours_,lines) ON CONFLICT DO NOTHING;
    END IF;
    RETURN result;
END $$;
REVOKE EXECUTE ON FUNCTION ledger.publish_operator_catalog(uuid,uuid,uuid,uuid,bigint,timestamptz,jsonb) FROM nu54_p05_commerce;
REVOKE ALL ON FUNCTION ledger.operator_catalog_pricing(uuid,uuid,uuid),
    ledger.publish_operator_catalog_pricing(uuid,uuid,uuid,uuid,bigint,timestamptz,jsonb,jsonb) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.operator_catalog_pricing(uuid,uuid,uuid),
    ledger.publish_operator_catalog_pricing(uuid,uuid,uuid,uuid,bigint,timestamptz,jsonb,jsonb) TO nu54_p05_commerce;

CREATE OR REPLACE FUNCTION ledger.reserve_order(tenant uuid, branch uuid, actor uuid, request_key uuid, origin_ text,
    origin_ref_ uuid, menu_ bigint, asset_ text, lines jsonb, role_ text)
RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
DECLARE
    actor_grant ledger.commerce_grant := ledger.commerce_allowed(tenant,branch,actor);
    policy ledger.command_rule;
    published ledger.menu_release;
    prior ledger.command_receipt;
    existing ledger.sale;
    checkout ledger.order_checkout;
    hold_until timestamptz;
    order_ uuid;
    intent bytea;
    amount_ numeric := 0;
    line record;
    unit ledger.units;
BEGIN
    IF role_ IS DISTINCT FROM actor_grant.role_code OR origin_ NOT IN ('pos','menu') OR (actor_grant.role_code='customer' AND origin_<>'menu') OR
       (actor_grant.role_code='terminal' AND actor_grant.terminal_mode<>origin_) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied';
    END IF;
    IF request_key IS NULL OR origin_ref_ IS NULL OR lines IS NULL OR asset_ IS NULL OR asset_ !~ '^0x[0-9a-f]{40}$' THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid order';
    END IF;
    IF jsonb_typeof(lines)<>'array' OR jsonb_array_length(lines) NOT BETWEEN 1 AND 20 OR
       (SELECT count(DISTINCT x.sku) FROM jsonb_to_recordset(lines) x(sku text)) <> jsonb_array_length(lines) THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid order lines';
    END IF;
    intent := sha256(convert_to(jsonb_build_array('reserve-order-v2',tenant,branch,origin_,origin_ref_,menu_,asset_,lines)::text,'UTF8'));
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||actor::text||'create_sale'||request_key::text,0));
    SELECT * INTO prior FROM ledger.command_receipt r WHERE r.branch_id=branch AND r.scope_ref=actor
      AND r.operation='create_sale' AND r.request_key=reserve_order.request_key;
    IF FOUND THEN
        IF prior.intent_digest<>intent THEN RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='request conflict'; END IF;
        IF prior.retire_after<=statement_timestamp() THEN RAISE EXCEPTION USING ERRCODE='P0502', MESSAGE='request expired'; END IF;
        RETURN prior.result_ref;
    END IF;

    -- The same POS origin owns exactly one sale across QR/BLE and client retries.
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||origin_||origin_ref_::text,1));
    SELECT * INTO existing FROM ledger.sale s WHERE s.branch_id=branch AND s.origin=origin_ AND s.origin_ref=origin_ref_ FOR UPDATE;
    IF FOUND THEN
        SELECT * INTO checkout FROM ledger.order_checkout c
          WHERE c.sale_id=existing.id AND c.creator=actor AND c.intent_digest=intent;
        IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='request conflict'; END IF;
        IF checkout.expires_at<=clock_timestamp() THEN
            RAISE EXCEPTION USING ERRCODE='P0502', MESSAGE='request expired';
        END IF;
        INSERT INTO ledger.command_receipt
          VALUES (branch,actor,'create_sale',request_key,intent,existing.id,statement_timestamp(),checkout.expires_at);
        RETURN existing.id;
    END IF;

    -- Coordinate new reservations with an operator publication. Acknowledging a known
    -- request above stays independent of the current pricing version and hours.
    PERFORM pg_advisory_xact_lock_shared(hashtextextended(branch::text||'operator-catalog',0));
    IF EXISTS (SELECT 1 FROM ledger.catalog_pricing p WHERE p.branch_id=branch) AND (
       menu_ IS DISTINCT FROM (SELECT max(m.version) FROM ledger.menu_release m WHERE m.branch_id=branch)
       OR NOT ledger.catalog_is_open(branch,menu_,clock_timestamp())
    ) THEN
        RAISE EXCEPTION USING ERRCODE='P0702', MESSAGE='stale or closed menu';
    END IF;
    SELECT * INTO policy FROM ledger.command_rule c WHERE c.branch_id=branch FOR SHARE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0503', MESSAGE='order policy unavailable'; END IF;
    SELECT * INTO published FROM ledger.menu_release m WHERE m.branch_id=branch AND m.version=menu_
      AND m.published_at<=statement_timestamp() AND m.expires_at>statement_timestamp() FOR SHARE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0702', MESSAGE='stale menu'; END IF;
    hold_until := LEAST(published.expires_at, statement_timestamp()+policy.receipt_window,actor_grant.valid_until);
    FOR line IN SELECT * FROM jsonb_to_recordset(lines) x(sku text, option_key text, quantity bigint) ORDER BY sku LOOP
        IF line.quantity IS NULL OR line.quantity NOT BETWEEN 1 AND 1000 THEN
            RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid line quantity';
        END IF;
        SELECT p.unit_price INTO unit FROM ledger.price p WHERE p.branch_id=branch AND p.version=menu_
          AND p.sku=line.sku AND p.option_key=line.option_key AND p.asset_ref=asset_;
        IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0702', MESSAGE='price version mismatch'; END IF;
        UPDATE ledger.stock s SET available=available-line.quantity
          WHERE s.branch_id=branch AND s.sku=line.sku AND s.available>=line.quantity;
        IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0701', MESSAGE='insufficient stock'; END IF;
        amount_ := amount_ + unit * line.quantity;
    END LOOP;
    INSERT INTO ledger.sale(branch_id,origin,origin_ref,asset_ref,amount)
      VALUES (branch,origin_,origin_ref_,asset_,amount_) RETURNING id INTO order_;
    INSERT INTO ledger.order_checkout VALUES (order_,branch,actor,menu_,hold_until,intent);
    INSERT INTO ledger.order_line
      SELECT order_,branch,menu_,p.sku,p.option_key,x.quantity,p.unit_price
      FROM jsonb_to_recordset(lines) x(sku text,option_key text,quantity bigint)
      JOIN ledger.price p ON p.branch_id=branch AND p.version=menu_ AND p.sku=x.sku AND p.option_key=x.option_key;
    INSERT INTO ledger.command_receipt
      VALUES (branch,actor,'create_sale',request_key,intent,order_,statement_timestamp(),hold_until);
    RETURN order_;
END $$;


CREATE OR REPLACE FUNCTION ledger.catalog_page(tenant uuid, branch uuid, actor uuid, page integer)
RETURNS TABLE(version bigint, expires_at timestamptz, sku text, option_key text, label text, asset_ref text, unit_price numeric)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE newest bigint;
BEGIN
    PERFORM ledger.commerce_allowed(tenant,branch,actor);
    IF page < 0 OR page > 32767 THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid page';
    END IF;
    IF EXISTS (SELECT 1 FROM ledger.catalog_pricing p WHERE p.branch_id=branch) THEN
        SELECT max(m.version) INTO newest FROM ledger.menu_release m WHERE m.branch_id=branch;
    ELSE
        SELECT max(m.version) INTO newest FROM ledger.menu_release m WHERE m.branch_id=branch
            AND m.published_at<=statement_timestamp() AND m.expires_at>statement_timestamp();
    END IF;
    RETURN QUERY
        SELECT m.version,m.expires_at,p.sku,p.option_key,p.label,p.asset_ref,p.unit_price::numeric
        FROM ledger.menu_release m JOIN ledger.price p USING(branch_id,version)
        WHERE m.branch_id=branch AND m.version=newest
            AND m.published_at<=statement_timestamp() AND m.expires_at>statement_timestamp()
            AND ledger.catalog_is_open(branch,newest,statement_timestamp())
        ORDER BY p.sku,p.option_key LIMIT 50 OFFSET page*50;
END $$;

CREATE OR REPLACE FUNCTION ledger.prepare_quote(tenant uuid, branch uuid, actor uuid, sale_ uuid, expected bigint,
    request_key_ uuid, request_intent_ bytea,
    claim_ bytea, profile_ bytea, signer_ bytea, quote_id_ bytea, order_id_ bytea, nonce_ bytea)
RETURNS TABLE(quote_id bytea, order_id bytea, nonce bytea, payer bytea, payer_generation bigint, payer_role smallint,
    issued_at timestamptz,expires_at timestamptz,fence bigint,payload_cipher bytea,signed_digest bytea,intent_digest bytea,amount text,asset text,issued_signer bytea)
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
DECLARE
    g ledger.commerce_grant := ledger.commerce_allowed(tenant,branch,actor);
    s ledger.sale;
    c ledger.order_checkout;
    cl ledger.checkout_claim;
    q ledger.quote_work;
    p ledger.issuance_profile;
    receipt ledger.command_receipt;
    clock timestamptz;
    lease timestamptz;
BEGIN
    IF request_key_ IS NULL OR request_intent_ IS NULL OR octet_length(request_intent_)<>32 THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid quote intent';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||actor::text||'issue_quote'||request_key_::text,0));
    SELECT * INTO receipt FROM ledger.command_receipt r WHERE r.branch_id=branch AND r.scope_ref=actor
      AND r.operation='issue_quote' AND r.request_key=request_key_;
    IF FOUND AND (receipt.intent_digest<>request_intent_ OR receipt.result_ref<>sale_) THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='request conflict';
    END IF;
    SELECT * INTO s FROM ledger.sale WHERE id=sale_ AND branch_id=branch FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied'; END IF;
    clock := date_trunc('second',clock_timestamp());
    SELECT * INTO c FROM ledger.order_checkout WHERE sale_id=sale_;
    SELECT claim_row.* INTO cl FROM ledger.checkout_claim claim_row
      JOIN ledger.payer_enrollment e USING (branch_id,actor_id)
      JOIN ledger.commerce_grant customer_scope USING (branch_id,actor_id)
      WHERE claim_row.sale_id=sale_ AND claim_row.claim_digest=claim_ AND claim_row.expires_at>clock
        AND e.expires_at>clock AND customer_scope.valid_until>clock AND customer_scope.role_code='customer'
        AND (claim_row.payer,claim_row.payer_generation,claim_row.payer_role)=(e.payer,e.payer_generation,e.payer_role)
        AND (claim_row.actor_id=actor OR (g.role_code='terminal' AND c.creator=actor))
        FOR SHARE OF claim_row,e,customer_scope;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied'; END IF;
    SELECT * INTO q FROM ledger.quote_work WHERE sale_id=sale_ FOR UPDATE;
    IF FOUND AND (q.actor_id<>cl.actor_id OR q.claim_digest<>claim_) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied';
    END IF;
    IF q.sale_id IS NOT NULL AND (q.payer<>cl.payer OR q.payer_generation<>cl.payer_generation OR q.payer_role<>cl.payer_role) THEN
        RAISE EXCEPTION USING ERRCODE='P0702', MESSAGE='payer authority changed';
    END IF;
    IF q.signed_digest IS NOT NULL THEN
        IF q.consumed_at IS NOT NULL OR q.expires_at<=clock OR s.state<>'issued' OR
           expected NOT IN (q.expected_revision,s.revision) THEN
            RAISE EXCEPTION USING ERRCODE='P0702', MESSAGE='quote no longer available';
        END IF;
        IF receipt.request_key IS NULL THEN
            INSERT INTO ledger.command_receipt VALUES
              (branch,actor,'issue_quote',request_key_,request_intent_,sale_,clock,q.expires_at);
        END IF;
        RETURN QUERY SELECT q.quote_id,q.order_id,q.nonce,q.payer,q.payer_generation,q.payer_role,q.issued_at,q.expires_at,
            q.fence,q.payload_cipher,q.signed_digest,c.intent_digest,s.amount::text,s.asset_ref,q.merchant_signer;
        RETURN;
    END IF;
    PERFORM pg_advisory_xact_lock_shared(hashtextextended(branch::text||'operator-catalog',0));
    IF EXISTS (SELECT 1 FROM ledger.catalog_pricing cp WHERE cp.branch_id=branch) AND
       ( c.menu_version IS DISTINCT FROM (SELECT max(m.version) FROM ledger.menu_release m WHERE m.branch_id=branch)
         OR NOT ledger.catalog_is_open(branch,c.menu_version,clock_timestamp()) )
    THEN
        RAISE EXCEPTION USING ERRCODE='P0702', MESSAGE='stale or closed menu';
    END IF;
    SELECT * INTO p FROM ledger.issuance_profile WHERE branch_id=branch AND enabled
      AND authorized_digest=profile_ FOR SHARE;
    IF NOT FOUND OR c.sale_id IS NULL OR s.state<>'preview' OR s.revision<>expected
       OR c.expires_at<=clock OR (q.sale_id IS NOT NULL AND (p.version<>q.profile_version OR q.merchant_signer<>signer_ OR q.expires_at<=clock)) THEN
        RAISE EXCEPTION USING ERRCODE='P0702', MESSAGE='order no longer issuable';
    END IF;
    IF q.sale_id IS NOT NULL THEN
        IF q.lease_until>clock THEN RAISE EXCEPTION USING ERRCODE='P0703', MESSAGE='quote signing in progress'; END IF;
        UPDATE ledger.quote_work w SET fence=w.fence+1,lease_until=LEAST(clock+interval '5 seconds',w.expires_at)
          WHERE w.sale_id=sale_ RETURNING * INTO q;
    ELSE
        lease := LEAST(clock + make_interval(secs=>p.quote_lifetime_seconds), c.expires_at, cl.expires_at, g.valid_until);
        IF lease<=clock THEN RAISE EXCEPTION USING ERRCODE='P0702', MESSAGE='quote expired'; END IF;
        INSERT INTO ledger.quote_work
          (sale_id,actor_id,claim_digest,quote_id,order_id,nonce,expected_revision,profile_version,merchant_signer,payer,payer_generation,payer_role,issued_at,expires_at,fence,lease_until)
          VALUES (sale_,cl.actor_id,claim_,quote_id_,order_id_,nonce_,expected,p.version,signer_,cl.payer,cl.payer_generation,cl.payer_role,
            clock,lease,1,LEAST(clock+interval '5 seconds',lease)) RETURNING * INTO q;
    END IF;
    IF receipt.request_key IS NULL THEN
        INSERT INTO ledger.command_receipt VALUES
          (branch,actor,'issue_quote',request_key_,request_intent_,sale_,clock,q.expires_at);
    END IF;
    RETURN QUERY SELECT q.quote_id,q.order_id,q.nonce,q.payer,q.payer_generation,q.payer_role,q.issued_at,q.expires_at,
        q.fence,q.payload_cipher,q.signed_digest,c.intent_digest,s.amount::text,s.asset_ref,q.merchant_signer;
END $$;

CREATE OR REPLACE FUNCTION ledger.finish_quote(tenant uuid, branch uuid, actor uuid, sale_ uuid, claim_ bytea,
    profile_ bytea, fence_ bigint, ciphertext bytea, digest_ bytea)
RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
DECLARE
    g ledger.commerce_grant := ledger.commerce_allowed(tenant,branch,actor);
    s ledger.sale;
    q ledger.quote_work;
    cl ledger.checkout_claim;
    p ledger.issuance_profile;
    clock timestamptz;
    menu_version_ bigint;
BEGIN
    SELECT * INTO s FROM ledger.sale WHERE id=sale_ AND branch_id=branch FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied'; END IF;
    SELECT * INTO q FROM ledger.quote_work WHERE sale_id=sale_ AND claim_digest=claim_ FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied'; END IF;
    SELECT c.menu_version INTO menu_version_ FROM ledger.order_checkout c WHERE c.sale_id=sale_;
    PERFORM pg_advisory_xact_lock_shared(hashtextextended(branch::text||'operator-catalog',0));
    IF EXISTS (SELECT 1 FROM ledger.catalog_pricing cp WHERE cp.branch_id=branch) AND
       ( menu_version_ IS DISTINCT FROM (SELECT max(m.version) FROM ledger.menu_release m WHERE m.branch_id=branch)
         OR NOT ledger.catalog_is_open(branch,menu_version_,clock_timestamp()) )
    THEN
        RAISE EXCEPTION USING ERRCODE='P0702', MESSAGE='stale or closed menu';
    END IF;
    clock := clock_timestamp();
    SELECT claim_row.* INTO cl FROM ledger.checkout_claim claim_row
      JOIN ledger.payer_enrollment e USING (branch_id,actor_id)
      JOIN ledger.commerce_grant customer_scope USING (branch_id,actor_id)
      WHERE claim_row.sale_id=sale_ AND claim_row.actor_id=q.actor_id AND claim_row.claim_digest=claim_
        AND claim_row.expires_at>clock AND e.expires_at>clock AND customer_scope.valid_until>clock AND customer_scope.role_code='customer'
        AND (claim_row.payer,claim_row.payer_generation,claim_row.payer_role)=(e.payer,e.payer_generation,e.payer_role)
        AND (claim_row.actor_id=actor OR (g.role_code='terminal' AND EXISTS
          (SELECT FROM ledger.order_checkout c WHERE c.sale_id=sale_ AND c.creator=actor)))
      FOR SHARE OF claim_row,e,customer_scope;
    SELECT * INTO p FROM ledger.issuance_profile WHERE branch_id=branch AND enabled
      AND authorized_digest=profile_ FOR SHARE;
    IF p.branch_id IS NULL OR cl.sale_id IS NULL OR q.fence<>fence_ OR q.expires_at<=clock OR q.lease_until<=clock
       OR q.profile_version<>p.version OR q.payer<>cl.payer OR q.payer_generation<>cl.payer_generation OR q.payer_role<>cl.payer_role
       OR s.state<>'preview' OR s.revision<>q.expected_revision OR ciphertext IS NULL OR octet_length(ciphertext)<>765
       OR digest_ IS NULL OR octet_length(digest_)<>32 THEN
        RAISE EXCEPTION USING ERRCODE='P0702', MESSAGE='quote lease or authority changed';
    END IF;
    UPDATE ledger.quote_work SET payload_cipher=ciphertext,signed_digest=digest_ WHERE sale_id=sale_;
    UPDATE ledger.sale SET state='issued',revision=revision+1 WHERE id=sale_;
    INSERT INTO ledger.quote_outbox(sale_id,quote_digest) VALUES (sale_,digest_);
    RETURN true;
END $$;


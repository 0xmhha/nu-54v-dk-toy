-- P05 order/quote boundary. Legacy v1 and the foundation ledger stay available.
DO $roles$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p05_commerce') THEN
        CREATE ROLE nu54_p05_commerce NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEROLE NOCREATEDB;
    END IF;
    IF EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p05_commerce'
        AND (rolcanlogin OR rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb OR rolreplication)) THEN
        RAISE EXCEPTION 'nu54_p05_commerce is not a restricted capability role';
    END IF;
END
$roles$;

ALTER TABLE ledger.command_receipt DROP CONSTRAINT command_receipt_operation_check;
ALTER TABLE ledger.command_receipt ADD CHECK (operation IN ('create_sale','authorize_checkout','issue_quote'));

CREATE TABLE ledger.commerce_grant (
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    actor_id uuid NOT NULL,
    role_code text NOT NULL CHECK (role_code IN ('customer','terminal')),
    terminal_mode text CHECK (terminal_mode IN ('menu','pos')),
    valid_until timestamptz NOT NULL CHECK (isfinite(valid_until)),
    PRIMARY KEY (branch_id,actor_id),
    CHECK ((role_code='terminal')=(terminal_mode IS NOT NULL))
);
CREATE TABLE ledger.payer_enrollment (
    branch_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    payer bytea NOT NULL CHECK (octet_length(payer)=20),
    payer_generation bigint NOT NULL CHECK (payer_generation>0),
    payer_role smallint NOT NULL CHECK (payer_role IN (1,2)),
    expires_at timestamptz NOT NULL CHECK (isfinite(expires_at)),
    PRIMARY KEY (branch_id,actor_id),
    FOREIGN KEY (branch_id,actor_id) REFERENCES ledger.commerce_grant
);
CREATE TABLE ledger.menu_release (
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    version bigint NOT NULL CHECK (version>0),
    published_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL CHECK (isfinite(expires_at) AND expires_at>published_at),
    PRIMARY KEY (branch_id,version)
);
CREATE TABLE ledger.price (
    branch_id uuid NOT NULL,
    version bigint NOT NULL,
    sku text NOT NULL CHECK (sku ~ '^[a-z0-9][a-z0-9_-]{0,31}$'),
    option_key text NOT NULL CHECK (option_key ~ '^[a-z0-9][a-z0-9_-]{0,31}$'),
    label text NOT NULL CHECK (length(label) BETWEEN 1 AND 120),
    asset_ref text NOT NULL CHECK (asset_ref ~ '^0x[0-9a-f]{40}$'),
    unit_price ledger.units NOT NULL CHECK (unit_price>0),
    PRIMARY KEY (branch_id,version,sku,option_key),
    FOREIGN KEY (branch_id,version) REFERENCES ledger.menu_release
);
CREATE TABLE ledger.stock (
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    sku text NOT NULL CHECK (sku ~ '^[a-z0-9][a-z0-9_-]{0,31}$'),
    available bigint NOT NULL CHECK (available>=0),
    PRIMARY KEY (branch_id,sku)
);
CREATE TABLE ledger.order_checkout (
    sale_id uuid PRIMARY KEY,
    branch_id uuid NOT NULL,
    creator uuid NOT NULL,
    menu_version bigint NOT NULL,
    expires_at timestamptz NOT NULL CHECK (isfinite(expires_at)),
    intent_digest bytea NOT NULL CHECK (octet_length(intent_digest)=32),
    FOREIGN KEY (branch_id,sale_id) REFERENCES ledger.sale(branch_id,id),
    FOREIGN KEY (branch_id,menu_version) REFERENCES ledger.menu_release
);
CREATE TABLE ledger.order_line (
    sale_id uuid NOT NULL,
    branch_id uuid NOT NULL,
    menu_version bigint NOT NULL,
    sku text NOT NULL,
    option_key text NOT NULL,
    quantity bigint NOT NULL CHECK (quantity>0),
    price ledger.units NOT NULL CHECK (price>0),
    PRIMARY KEY (sale_id,sku),
    FOREIGN KEY (branch_id,sale_id) REFERENCES ledger.sale(branch_id,id),
    FOREIGN KEY (branch_id,sku) REFERENCES ledger.stock,
    FOREIGN KEY (branch_id,menu_version,sku,option_key) REFERENCES ledger.price
);
CREATE TABLE ledger.checkout_claim (
    sale_id uuid NOT NULL,
    branch_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    claim_digest bytea NOT NULL CHECK (octet_length(claim_digest)=32),
    payer bytea NOT NULL CHECK (octet_length(payer)=20),
    payer_generation bigint NOT NULL CHECK (payer_generation>0),
    payer_role smallint NOT NULL CHECK (payer_role IN (1,2)),
    expires_at timestamptz NOT NULL CHECK (isfinite(expires_at)),
    PRIMARY KEY (sale_id,actor_id),
    UNIQUE (sale_id,claim_digest),
    FOREIGN KEY (branch_id,sale_id) REFERENCES ledger.sale(branch_id,id),
    FOREIGN KEY (branch_id,actor_id) REFERENCES ledger.commerce_grant
);
CREATE TABLE ledger.issuance_profile (
    branch_id uuid PRIMARY KEY REFERENCES ledger.branch,
    version bigint NOT NULL CHECK (version>0),
    authorized_digest bytea NOT NULL CHECK (octet_length(authorized_digest)=32),
    quote_lifetime_seconds integer NOT NULL CHECK (quote_lifetime_seconds BETWEEN 1 AND 120),
    enabled boolean NOT NULL
);
CREATE TABLE ledger.quote_work (
    sale_id uuid PRIMARY KEY REFERENCES ledger.sale,
    actor_id uuid NOT NULL,
    claim_digest bytea NOT NULL CHECK (octet_length(claim_digest)=32),
    quote_id bytea NOT NULL UNIQUE CHECK (octet_length(quote_id)=32),
    order_id bytea NOT NULL UNIQUE CHECK (octet_length(order_id)=32),
    nonce bytea NOT NULL UNIQUE CHECK (octet_length(nonce)=32),
    expected_revision bigint NOT NULL CHECK (expected_revision>=0),
    profile_version bigint NOT NULL CHECK (profile_version>0),
    merchant_signer bytea NOT NULL CHECK (octet_length(merchant_signer)=20),
    payer bytea NOT NULL CHECK (octet_length(payer)=20),
    payer_generation bigint NOT NULL CHECK (payer_generation>0),
    payer_role smallint NOT NULL CHECK (payer_role IN (1,2)),
    issued_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL CHECK (isfinite(expires_at) AND expires_at>issued_at),
    fence bigint NOT NULL CHECK (fence>0),
    lease_until timestamptz NOT NULL,
    payload_cipher bytea,
    signed_digest bytea CHECK (octet_length(signed_digest)=32),
    consumed_at timestamptz,
    CHECK ((payload_cipher IS NULL)=(signed_digest IS NULL)),
    CHECK (consumed_at IS NULL OR signed_digest IS NOT NULL)
);
CREATE TABLE ledger.quote_outbox (
    event_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    sale_id uuid NOT NULL UNIQUE REFERENCES ledger.quote_work,
    quote_digest bytea NOT NULL CHECK (octet_length(quote_digest)=32),
    created_at timestamptz NOT NULL DEFAULT statement_timestamp()
);

CREATE FUNCTION ledger.commerce_allowed(tenant uuid, branch uuid, actor uuid)
RETURNS ledger.commerce_grant
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
DECLARE found_grant ledger.commerce_grant;
BEGIN
    SELECT g.* INTO found_grant FROM ledger.commerce_grant g JOIN ledger.branch b ON b.id=g.branch_id
      WHERE g.branch_id=branch AND g.actor_id=actor AND b.tenant_ref=tenant AND g.valid_until>statement_timestamp()
      FOR SHARE OF g;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied'; END IF;
    RETURN found_grant;
END $$;

CREATE FUNCTION ledger.catalog_page(tenant uuid, branch uuid, actor uuid, page integer)
RETURNS TABLE(version bigint, expires_at timestamptz, sku text, option_key text, label text, asset_ref text, unit_price numeric)
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
BEGIN
    PERFORM ledger.commerce_allowed(tenant,branch,actor);
    IF page < 0 OR page > 32767 THEN RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid page'; END IF;
    RETURN QUERY
      SELECT m.version,m.expires_at,p.sku,p.option_key,p.label,p.asset_ref,p.unit_price::numeric
      FROM ledger.menu_release m JOIN ledger.price p USING (branch_id,version)
      WHERE m.branch_id=branch AND m.version=(
        SELECT max(r.version) FROM ledger.menu_release r WHERE r.branch_id=branch
          AND r.published_at<=statement_timestamp() AND r.expires_at>statement_timestamp())
      ORDER BY p.sku,p.option_key LIMIT 50 OFFSET page*50;
END $$;

CREATE FUNCTION ledger.reserve_order(tenant uuid, branch uuid, actor uuid, request_key uuid, origin_ text,
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

CREATE FUNCTION ledger.order_status(tenant uuid, branch uuid, actor uuid, sale_ uuid)
RETURNS TABLE(id uuid, revision bigint, state text, asset_ref text, amount numeric, expires_at timestamptz)
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
BEGIN
    PERFORM ledger.commerce_allowed(tenant,branch,actor);
    RETURN QUERY
      SELECT s.id,s.revision,s.state,s.asset_ref,s.amount::numeric,c.expires_at
      FROM ledger.sale s JOIN ledger.order_checkout c ON c.sale_id=s.id
      WHERE s.id=sale_ AND s.branch_id=branch AND (c.creator=actor OR EXISTS
        (SELECT FROM ledger.checkout_claim cl WHERE cl.sale_id=sale_ AND cl.actor_id=actor AND cl.expires_at>statement_timestamp()));
END $$;

CREATE FUNCTION ledger.c3_scope(tenant uuid, branch uuid, actor uuid, role_ text, target uuid, operation text, claim_ bytea)
RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
DECLARE
    g ledger.commerce_grant := ledger.commerce_allowed(tenant,branch,actor);
BEGIN
    IF role_ IS DISTINCT FROM g.role_code OR target IS NULL THEN RETURN false; END IF;
    CASE operation
    WHEN 'catalog.read' THEN RETURN target=branch;
    WHEN 'order.create' THEN RETURN true;
    WHEN 'checkout.authorize' THEN
        RETURN EXISTS (SELECT FROM ledger.order_checkout c WHERE c.branch_id=branch AND c.sale_id=target AND c.creator=actor);
    WHEN 'order.status' THEN
        RETURN EXISTS (SELECT FROM ledger.order_checkout c WHERE c.branch_id=branch AND c.sale_id=target AND
            (c.creator=actor OR EXISTS (SELECT FROM ledger.checkout_claim cl WHERE cl.sale_id=target AND cl.actor_id=actor
             AND cl.expires_at>statement_timestamp())));
    WHEN 'quote.issue' THEN
        RETURN EXISTS (SELECT FROM ledger.checkout_claim cl
          WHERE cl.branch_id=branch AND cl.sale_id=target AND cl.claim_digest=claim_ AND cl.expires_at>statement_timestamp()
            AND (cl.actor_id=actor OR (g.role_code='terminal' AND EXISTS
             (SELECT FROM ledger.order_checkout c WHERE c.branch_id=branch AND c.sale_id=target AND c.creator=actor))));
    ELSE RETURN false;
    END CASE;
END $$;

CREATE FUNCTION ledger.authorize_checkout(tenant uuid, branch uuid, actor uuid, sale_ uuid, customer uuid,
    request_key_ uuid, intent_ bytea, candidate bytea)
RETURNS TABLE(claim_digest bytea, expires_at timestamptz)
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
DECLARE
    owner_grant ledger.commerce_grant := ledger.commerce_allowed(tenant,branch,actor);
    customer_grant ledger.commerce_grant;
    checkout ledger.order_checkout;
    enrolled ledger.payer_enrollment;
    cl ledger.checkout_claim;
    s ledger.sale;
    receipt ledger.command_receipt;
    expiry timestamptz;
    moment timestamptz;
BEGIN
    IF owner_grant.role_code='customer' AND customer IS DISTINCT FROM actor THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied';
    END IF;
    IF request_key_ IS NULL OR intent_ IS NULL OR candidate IS NULL OR octet_length(candidate)<>32 OR octet_length(intent_)<>32 THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid checkout claim request';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||actor::text||'authorize_checkout'||request_key_::text,0));
    SELECT * INTO receipt FROM ledger.command_receipt r WHERE r.branch_id=branch AND r.scope_ref=actor
      AND r.operation='authorize_checkout' AND r.request_key=request_key_;
    IF FOUND AND (receipt.intent_digest<>intent_ OR receipt.result_ref<>sale_) THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='request conflict';
    END IF;
    SELECT * INTO s FROM ledger.sale WHERE id=sale_ AND branch_id=branch FOR UPDATE;
    SELECT * INTO checkout FROM ledger.order_checkout WHERE sale_id=sale_ AND creator=actor;
    IF s.id IS NULL OR checkout.sale_id IS NULL THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied';
    END IF;
    moment := clock_timestamp();
    customer_grant := ledger.commerce_allowed(tenant,branch,customer);
    SELECT * INTO enrolled FROM ledger.payer_enrollment e WHERE e.branch_id=branch AND e.actor_id=customer
      AND e.expires_at>moment FOR SHARE;
    IF NOT FOUND OR customer_grant.role_code<>'customer' THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied';
    END IF;
    SELECT * INTO cl FROM ledger.checkout_claim WHERE sale_id=sale_ AND actor_id=customer;
    IF FOUND THEN
        IF cl.expires_at<=moment OR cl.payer<>enrolled.payer OR cl.payer_generation<>enrolled.payer_generation
           OR cl.payer_role<>enrolled.payer_role THEN
            RAISE EXCEPTION USING ERRCODE='P0702', MESSAGE='checkout authorization expired';
        END IF;
    ELSE
        expiry := LEAST(checkout.expires_at,enrolled.expires_at,customer_grant.valid_until,owner_grant.valid_until);
        IF expiry<=moment OR s.state<>'preview' THEN
            RAISE EXCEPTION USING ERRCODE='P0702', MESSAGE='checkout authorization expired';
        END IF;
        INSERT INTO ledger.checkout_claim
          VALUES (sale_,branch,customer,candidate,enrolled.payer,enrolled.payer_generation,enrolled.payer_role,expiry)
          RETURNING * INTO cl;
    END IF;
    IF receipt.request_key IS NULL THEN
        INSERT INTO ledger.command_receipt VALUES (branch,actor,'authorize_checkout',request_key_,intent_,sale_,moment,cl.expires_at);
    END IF;
    RETURN QUERY SELECT cl.claim_digest,cl.expires_at;
END $$;

CREATE FUNCTION ledger.prepare_quote(tenant uuid, branch uuid, actor uuid, sale_ uuid, expected bigint,
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

CREATE FUNCTION ledger.finish_quote(tenant uuid, branch uuid, actor uuid, sale_ uuid, claim_ bytea,
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
BEGIN
    SELECT * INTO s FROM ledger.sale WHERE id=sale_ AND branch_id=branch FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied'; END IF;
    SELECT * INTO q FROM ledger.quote_work WHERE sale_id=sale_ AND claim_digest=claim_ FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied'; END IF;
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

CREATE FUNCTION ledger.reclaim_expired_order()
RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
DECLARE
    checkout ledger.order_checkout;
    held record;
BEGIN
    SELECT c.* INTO checkout FROM ledger.order_checkout c JOIN ledger.sale s ON s.id=c.sale_id
      WHERE c.expires_at<=clock_timestamp() AND s.state='preview'
      ORDER BY c.expires_at,c.sale_id LIMIT 1 FOR UPDATE OF s SKIP LOCKED;
    IF NOT FOUND THEN RETURN false; END IF;
    -- A prepared signature that was never committed was never exposed. The
    -- state change below fences delayed signing and leaves issued orders alone.
    FOR held IN SELECT l.sku,l.quantity FROM ledger.order_line l WHERE l.sale_id=checkout.sale_id ORDER BY l.sku LOOP
        UPDATE ledger.stock SET available=available+held.quantity
         WHERE branch_id=checkout.branch_id AND sku=held.sku;
    END LOOP;
    UPDATE ledger.sale SET state='closed',revision=revision+1 WHERE id=checkout.sale_id;
    RETURN true;
END $$;

REVOKE ALL ON FUNCTION ledger.reclaim_expired_order() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.reclaim_expired_order() TO nu54_p05_commerce;

REVOKE ALL ON FUNCTION ledger.c3_scope(uuid,uuid,uuid,text,uuid,text,bytea) FROM PUBLIC;
REVOKE ALL ON FUNCTION ledger.authorize_checkout(uuid,uuid,uuid,uuid,uuid,uuid,bytea,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.c3_scope(uuid,uuid,uuid,text,uuid,text,bytea),
  ledger.authorize_checkout(uuid,uuid,uuid,uuid,uuid,uuid,bytea,bytea) TO nu54_p05_commerce;

REVOKE ALL ON FUNCTION ledger.commerce_allowed(uuid,uuid,uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION ledger.catalog_page(uuid,uuid,uuid,integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION ledger.reserve_order(uuid,uuid,uuid,uuid,text,uuid,bigint,text,jsonb,text) FROM PUBLIC;
REVOKE ALL ON FUNCTION ledger.order_status(uuid,uuid,uuid,uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION ledger.prepare_quote(uuid,uuid,uuid,uuid,bigint,uuid,bytea,bytea,bytea,bytea,bytea,bytea,bytea) FROM PUBLIC;
REVOKE ALL ON FUNCTION ledger.finish_quote(uuid,uuid,uuid,uuid,bytea,bytea,bigint,bytea,bytea) FROM PUBLIC;
GRANT USAGE ON SCHEMA ledger TO nu54_p05_commerce;
GRANT EXECUTE ON FUNCTION ledger.catalog_page(uuid,uuid,uuid,integer),
  ledger.reserve_order(uuid,uuid,uuid,uuid,text,uuid,bigint,text,jsonb,text),
  ledger.order_status(uuid,uuid,uuid,uuid),
  ledger.prepare_quote(uuid,uuid,uuid,uuid,bigint,uuid,bytea,bytea,bytea,bytea,bytea,bytea,bytea),
  ledger.finish_quote(uuid,uuid,uuid,uuid,bytea,bytea,bigint,bytea,bytea)
  TO nu54_p05_commerce;

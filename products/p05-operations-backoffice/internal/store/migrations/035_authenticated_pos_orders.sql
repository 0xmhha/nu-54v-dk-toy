-- Registered external POS amounts are independent of menu versions and inventory.
ALTER TABLE ledger.order_checkout ALTER COLUMN menu_version DROP NOT NULL;

CREATE TABLE ledger.pos_source (
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    actor_id uuid NOT NULL,
    terminal_id uuid NOT NULL,
    valid_until timestamptz NOT NULL CHECK(isfinite(valid_until)),
    PRIMARY KEY(branch_id,actor_id),
    CHECK(actor_id<>terminal_id),
    FOREIGN KEY(branch_id,terminal_id) REFERENCES ledger.commerce_grant(branch_id,actor_id)
);

CREATE TABLE ledger.pos_order (
    branch_id uuid NOT NULL,
    external_ref text NOT NULL CHECK(external_ref ~ '^[A-Za-z0-9][A-Za-z0-9._/-]{0,63}$'),
    source_id uuid NOT NULL,
    terminal_id uuid NOT NULL,
    handoff_ref uuid NOT NULL UNIQUE,
    sale_id uuid NOT NULL UNIQUE,
    PRIMARY KEY(branch_id,external_ref),
    FOREIGN KEY(branch_id,source_id) REFERENCES ledger.pos_source(branch_id,actor_id),
    FOREIGN KEY(branch_id,sale_id) REFERENCES ledger.sale(branch_id,id)
);
REVOKE ALL ON ledger.pos_source,ledger.pos_order FROM PUBLIC;

CREATE FUNCTION ledger.submit_pos_order(tenant_ uuid, branch_ uuid, source_ uuid,
    reference_ text, asset_ text, amount_ numeric)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE enrollment ledger.pos_source;
DECLARE target ledger.commerce_grant;
DECLARE prior ledger.pos_order;
DECLARE sale_ ledger.sale;
DECLARE rule ledger.command_rule;
DECLARE handoff_ uuid;
DECLARE committed_at timestamptz;
DECLARE expires_ timestamptz;
BEGIN
    IF reference_ !~ '^[A-Za-z0-9][A-Za-z0-9._/-]{0,63}$' OR asset_ !~ '^0x[0-9a-f]{40}$'
       OR amount_ IS NULL OR amount_<=0 THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid POS order';
    END IF;
    SELECT p.* INTO enrollment FROM ledger.pos_source p JOIN ledger.branch b ON b.id=p.branch_id
        WHERE p.branch_id=branch_ AND p.actor_id=source_ AND b.tenant_ref=tenant_ FOR SHARE OF p;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='POS source denied'; END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch_::text||reference_,39));
    SELECT g.* INTO target FROM ledger.commerce_grant g WHERE g.branch_id=branch_
        AND g.actor_id=enrollment.terminal_id AND g.role_code='terminal' FOR SHARE;
    IF NOT FOUND OR enrollment.valid_until<=clock_timestamp() OR target.valid_until<=clock_timestamp() THEN
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='POS source denied';
    END IF;
    SELECT p.* INTO prior FROM ledger.pos_order p WHERE p.branch_id=branch_ AND p.external_ref=reference_;
    IF FOUND THEN
        IF (prior.source_id,prior.terminal_id) IS DISTINCT FROM (source_,target.actor_id) THEN
            RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='POS scope denied';
        END IF;
        SELECT s.* INTO STRICT sale_ FROM ledger.sale s WHERE s.id=prior.sale_id;
        IF (sale_.asset_ref,sale_.amount) IS DISTINCT FROM (asset_,amount_) THEN
            RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='POS reference already bound';
        END IF;
        RETURN prior.handoff_ref;
    END IF;
    IF target.terminal_mode<>'pos' THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='POS mode denied'; END IF;
    SELECT c.* INTO rule FROM ledger.command_rule c WHERE c.branch_id=branch_ FOR SHARE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0503',MESSAGE='POS policy unavailable'; END IF;
    committed_at:=clock_timestamp();
    IF enrollment.valid_until<=committed_at OR target.valid_until<=committed_at THEN
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='POS source denied';
    END IF;
    handoff_:=gen_random_uuid();
    expires_:=LEAST(committed_at+rule.receipt_window,enrollment.valid_until,target.valid_until);
    INSERT INTO ledger.sale(branch_id,origin,origin_ref,asset_ref,amount)
        VALUES(branch_,'pos',handoff_,asset_,amount_) RETURNING * INTO sale_;
    INSERT INTO ledger.order_checkout(sale_id,branch_id,creator,menu_version,expires_at,intent_digest)
        VALUES(sale_.id,branch_,target.actor_id,NULL,expires_,
          sha256(convert_to(jsonb_build_array('registered-pos-v1',tenant_,branch_,source_,target.actor_id,reference_,asset_,amount_)::text,'UTF8')));
    INSERT INTO ledger.pos_order VALUES(branch_,reference_,source_,target.actor_id,handoff_,sale_.id);
    RETURN handoff_;
END $$;

CREATE FUNCTION ledger.lookup_pos_order(tenant_ uuid,branch_ uuid,actor_ uuid,handoff_ uuid)
RETURNS TABLE(sale_ref uuid,external_ref text,branch_label text,payer_bound boolean)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE g ledger.commerce_grant := ledger.commerce_allowed(tenant_,branch_,actor_);
BEGIN
    IF g.role_code<>'terminal' OR g.valid_until<=clock_timestamp() THEN
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='POS scope denied';
    END IF;
    RETURN QUERY SELECT p.sale_id,p.external_ref,coalesce(branch_name.label,'등록 가맹점'),EXISTS(
        SELECT 1 FROM ledger.checkout_claim c WHERE c.sale_id=p.sale_id
    ) FROM ledger.pos_order p LEFT JOIN ledger.operator_branch_label branch_name ON branch_name.branch_id=p.branch_id WHERE p.branch_id=branch_ AND p.terminal_id=actor_ AND p.handoff_ref=handoff_;
END $$;
REVOKE ALL ON FUNCTION ledger.submit_pos_order(uuid,uuid,uuid,text,text,numeric),
    ledger.lookup_pos_order(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.submit_pos_order(uuid,uuid,uuid,text,text,numeric),
    ledger.lookup_pos_order(uuid,uuid,uuid,uuid) TO nu54_p05_commerce;

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
    IF c.menu_version IS NOT NULL AND EXISTS (SELECT 1 FROM ledger.catalog_pricing cp WHERE cp.branch_id=branch) AND
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
    IF menu_version_ IS NOT NULL AND EXISTS (SELECT 1 FROM ledger.catalog_pricing cp WHERE cp.branch_id=branch) AND
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

CREATE OR REPLACE FUNCTION ledger.request_authority_live(
    tenant_ uuid, branch_ uuid, actor_ uuid, role_ text, capabilities_ text[]
) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE ends_ timestamptz;
DECLARE windows_ tstzrange[];
BEGIN
    IF NOT EXISTS(SELECT FROM ledger.branch b WHERE b.id=branch_ AND b.tenant_ref=tenant_) THEN
        RETURN false;
    END IF;
    IF role_='pos' THEN
        SELECT least(p.valid_until,g.valid_until) INTO ends_ FROM ledger.pos_source p
        JOIN ledger.commerce_grant g ON g.branch_id=p.branch_id AND g.actor_id=p.terminal_id
            AND g.role_code='terminal'
        WHERE p.branch_id=branch_ AND p.actor_id=actor_ FOR SHARE OF p,g;
        RETURN FOUND AND ends_>clock_timestamp();
    ELSIF role_ IN ('customer','terminal') THEN
        SELECT g.valid_until INTO ends_ FROM ledger.commerce_grant g
        WHERE g.branch_id=branch_ AND g.actor_id=actor_ AND g.role_code=role_ FOR SHARE;
        RETURN FOUND AND ends_>clock_timestamp();
    ELSIF role_='operator' AND cardinality(capabilities_)=0 THEN
        -- First lock all matching versions in a stable order. The next
        -- statement uses a new clock sample after every lock wait.
        SELECT array_agg(tstzrange(locked.valid_from,locked.revoked_at,'[)')) INTO windows_ FROM (
            SELECT g.valid_from,g.revoked_at FROM ledger.staff_grant g
            WHERE g.branch_id=branch_ AND g.subject_ref=actor_ AND g.role_code='operator'
            ORDER BY g.valid_from FOR SHARE
        ) locked;
        RETURN EXISTS(SELECT FROM unnest(windows_) AS windows(active) WHERE windows.active @> clock_timestamp());
    ELSIF role_ IN ('operator','finance') AND cardinality(capabilities_)>0 THEN
        SELECT max(locked.valid_until) INTO ends_ FROM (
            SELECT g.valid_until FROM ledger.finance_grant g
            WHERE g.branch_id=branch_ AND g.actor_id=actor_ AND g.capability=ANY(capabilities_)
            ORDER BY g.capability FOR SHARE
        ) locked;
        RETURN COALESCE(ends_>clock_timestamp(),false);
    END IF;
    RETURN false;
END $$;

CREATE FUNCTION ledger.pos_source_order(tenant_ uuid,branch_ uuid,source_ uuid,reference_ text)
RETURNS TABLE(terminal_ref uuid,handoff_ref uuid)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT p.terminal_id,p.handoff_ref FROM ledger.pos_order p JOIN ledger.pos_source a
      ON a.actor_id=p.source_id AND a.branch_id=p.branch_id AND a.terminal_id=p.terminal_id
    WHERE p.branch_id=branch_ AND p.source_id=source_ AND p.external_ref=reference_
      AND ledger.request_authority_live(tenant_,branch_,source_,'pos',ARRAY[]::text[]);
$$;

REVOKE ALL ON FUNCTION ledger.pos_source_order(uuid,uuid,uuid,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.pos_source_order(uuid,uuid,uuid,text) TO nu54_p05_commerce;

-- Private order evidence and at-counter QR comparison for existing orders.
CREATE FUNCTION ledger.commerce_order_view(tenant uuid, branch uuid, actor uuid, sale_ uuid)
RETURNS TABLE(id uuid, revision bigint, state text, asset_ref text, amount numeric, expires_at timestamptz, pickup_no bigint, receipt_ref uuid, quote_digest text, order_key text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.commerce_allowed(tenant,branch,actor);
    RETURN QUERY
      SELECT o.id,o.revision,o.state,o.asset_ref,o.amount::numeric,c.expires_at,p.number,p.receipt_ref,
        CASE WHEN o.state='issued' THEN encode(q.signed_digest,'hex') END,
        CASE WHEN o.state='issued' THEN encode(q.order_id,'hex') END
      FROM ledger.sale o JOIN ledger.order_checkout c ON c.sale_id=o.id AND c.branch_id=o.branch_id
      LEFT JOIN ledger.pickup p ON p.branch_id=o.branch_id AND p.sale_id=o.id AND o.state='paid'
      LEFT JOIN ledger.quote_work q ON q.sale_id=o.id AND q.signed_digest IS NOT NULL
      WHERE o.branch_id=branch AND o.id=sale_ AND (c.creator=actor OR
        EXISTS(SELECT FROM ledger.checkout_claim issued WHERE issued.branch_id=branch AND issued.sale_id=o.id
            AND issued.actor_id=actor AND issued.expires_at>statement_timestamp()) OR
        (o.state='paid' AND p.receipt_ref IS NOT NULL AND q.actor_id=actor));
END $$;
REVOKE ALL ON FUNCTION ledger.commerce_order_view(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.commerce_order_view(uuid,uuid,uuid,uuid) TO nu54_p05_commerce;

CREATE TABLE ledger.checkout_pair (
    branch_id uuid NOT NULL,
    sale_id uuid NOT NULL,
    customer_ref uuid NOT NULL,
    request_key uuid NOT NULL,
    comparison_code text NOT NULL CHECK (comparison_code ~ '^[0-9a-f]{8}$'),
    expires_at timestamptz NOT NULL CHECK (isfinite(expires_at)),
    PRIMARY KEY (branch_id,sale_id,customer_ref),
    UNIQUE (branch_id,sale_id,comparison_code),
    FOREIGN KEY (branch_id,sale_id) REFERENCES ledger.sale(branch_id,id),
    FOREIGN KEY (branch_id,customer_ref) REFERENCES ledger.commerce_grant(branch_id,actor_id)
);
REVOKE ALL ON ledger.checkout_pair FROM PUBLIC;

CREATE FUNCTION ledger.request_checkout_pair(tenant uuid, branch uuid, actor uuid, sale_ uuid, key_ uuid, code_ text)
RETURNS TABLE(code text, expiry timestamptz) LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    grant_ ledger.commerce_grant := ledger.commerce_allowed(tenant,branch,actor);
    prior ledger.checkout_pair;
    order_expiry timestamptz;
BEGIN
    IF grant_.role_code<>'customer' OR code_ !~ '^[0-9a-f]{8}$' THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='checkout scope denied';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||sale_::text||'checkout-pair',0));
    SELECT c.expires_at INTO order_expiry FROM ledger.order_checkout c
      JOIN ledger.sale s ON s.id=c.sale_id AND s.branch_id=c.branch_id
      JOIN ledger.commerce_grant creator ON creator.branch_id=c.branch_id AND creator.actor_id=c.creator
      WHERE c.branch_id=branch AND c.sale_id=sale_ AND c.expires_at>statement_timestamp()
        AND s.state IN ('preview','issued') AND creator.role_code='terminal' AND creator.valid_until>statement_timestamp()
      FOR SHARE OF c,s,creator;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='checkout scope denied'; END IF;
    DELETE FROM ledger.checkout_pair WHERE branch_id=branch AND sale_id=sale_ AND expires_at<=statement_timestamp();
    SELECT * INTO prior FROM ledger.checkout_pair p WHERE p.branch_id=branch AND p.sale_id=sale_ AND p.customer_ref=actor;
    IF FOUND THEN
        RETURN QUERY SELECT prior.comparison_code,prior.expires_at;
        RETURN;
    END IF;
    IF (SELECT count(*) FROM ledger.checkout_pair p WHERE p.branch_id=branch AND p.sale_id=sale_)>=8 THEN
        RAISE EXCEPTION USING ERRCODE='P0703', MESSAGE='checkout comparison busy';
    END IF;
    order_expiry := LEAST(order_expiry,grant_.valid_until,statement_timestamp()+interval '35 seconds');
    INSERT INTO ledger.checkout_pair VALUES(branch,sale_,actor,key_,code_,order_expiry);
    RETURN QUERY SELECT code_,order_expiry;
END $$;

CREATE FUNCTION ledger.pending_checkout_pairs(tenant uuid, branch uuid, actor uuid, sale_ uuid)
RETURNS TABLE(customer_ref uuid, code text, expiry timestamptz) LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE grant_ ledger.commerce_grant := ledger.commerce_allowed(tenant,branch,actor);
BEGIN
    IF grant_.role_code<>'terminal' OR NOT EXISTS(SELECT FROM ledger.order_checkout c
        WHERE c.branch_id=branch AND c.creator=actor AND c.sale_id=sale_ AND c.expires_at>statement_timestamp()) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='checkout scope denied';
    END IF;
    RETURN QUERY SELECT p.customer_ref,p.comparison_code,p.expires_at FROM ledger.checkout_pair p
      WHERE p.branch_id=branch AND p.sale_id=sale_ AND p.expires_at>statement_timestamp()
      ORDER BY p.expires_at,p.customer_ref LIMIT 8;
END $$;

CREATE FUNCTION ledger.confirm_checkout_pair(tenant uuid, branch uuid, actor uuid, sale_ uuid, customer uuid,
    code_ text, key_ uuid, intent bytea, random_claim bytea)
RETURNS TABLE(claim_digest bytea, expires_at timestamptz) LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE grant_ ledger.commerce_grant := ledger.commerce_allowed(tenant,branch,actor);
BEGIN
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||sale_::text||'checkout-pair',0));
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||actor::text||'authorize_checkout'||key_::text,0));
    IF grant_.role_code<>'terminal' OR NOT EXISTS(
        SELECT FROM ledger.checkout_pair p WHERE p.branch_id=branch AND p.sale_id=sale_ AND p.customer_ref=customer
          AND p.comparison_code=code_ AND p.expires_at>statement_timestamp() FOR SHARE) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='checkout scope denied';
    END IF;
    PERFORM FROM ledger.sale s WHERE s.id=sale_ AND s.branch_id=branch FOR UPDATE;
    IF EXISTS(SELECT FROM ledger.checkout_claim c WHERE c.branch_id=branch AND c.sale_id=sale_ AND c.actor_id<>customer) THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='checkout already paired';
    END IF;
    RETURN QUERY SELECT c.claim_digest,c.expires_at FROM ledger.authorize_checkout(tenant,branch,actor,sale_,customer,key_,intent,random_claim) c;
    DELETE FROM ledger.checkout_pair p WHERE p.branch_id=branch AND p.sale_id=sale_ AND p.customer_ref<>customer;

END $$;

CREATE FUNCTION ledger.read_checkout_claim(tenant uuid, branch uuid, actor uuid, sale_ uuid)
RETURNS TABLE(claim_digest bytea, expires_at timestamptz) LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE grant_ ledger.commerce_grant := ledger.commerce_allowed(tenant,branch,actor);
BEGIN
    IF grant_.role_code<>'customer' THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='checkout scope denied'; END IF;
    RETURN QUERY SELECT c.claim_digest,c.expires_at FROM ledger.checkout_claim c
      WHERE c.branch_id=branch AND c.sale_id=sale_ AND c.actor_id=actor AND c.expires_at>statement_timestamp();
END $$;

REVOKE ALL ON FUNCTION ledger.request_checkout_pair(uuid,uuid,uuid,uuid,uuid,text),
    ledger.pending_checkout_pairs(uuid,uuid,uuid,uuid),ledger.confirm_checkout_pair(uuid,uuid,uuid,uuid,uuid,text,uuid,bytea,bytea),
    ledger.read_checkout_claim(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.request_checkout_pair(uuid,uuid,uuid,uuid,uuid,text),
    ledger.pending_checkout_pairs(uuid,uuid,uuid,uuid),ledger.confirm_checkout_pair(uuid,uuid,uuid,uuid,uuid,text,uuid,bytea,bytea),
    ledger.read_checkout_claim(uuid,uuid,uuid,uuid) TO nu54_p05_commerce;

-- Look up an existing reserve receipt before applying the terminal's current mode to a new sale.
CREATE FUNCTION ledger.resume_reserve_order(tenant uuid, branch uuid, actor uuid, key_ uuid, origin_ text,
    origin_ref_ uuid, menu_ bigint, asset_ text, lines jsonb, role_ text)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    grant_ ledger.commerce_grant := ledger.commerce_allowed(tenant,branch,actor);
    prior ledger.command_receipt;
    intent bytea;
BEGIN
    IF grant_.role_code IS DISTINCT FROM role_ OR key_ IS NULL OR lines IS NULL THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied';
    END IF;
    SELECT * INTO prior FROM ledger.command_receipt r
      WHERE r.branch_id=branch AND r.scope_ref=actor AND r.operation='create_sale' AND r.request_key=key_;
    IF NOT FOUND THEN RETURN NULL; END IF;
    intent := sha256(convert_to(jsonb_build_array('reserve-order-v2',tenant,branch,origin_,origin_ref_,menu_,asset_,lines)::text,'UTF8'));
    IF prior.intent_digest IS DISTINCT FROM intent THEN RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='request conflict'; END IF;
    IF prior.retire_after<=statement_timestamp() THEN RAISE EXCEPTION USING ERRCODE='P0502', MESSAGE='request expired'; END IF;
    RETURN prior.result_ref;
END $$;
REVOKE ALL ON FUNCTION ledger.resume_reserve_order(uuid,uuid,uuid,uuid,text,uuid,bigint,text,jsonb,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.resume_reserve_order(uuid,uuid,uuid,uuid,text,uuid,bigint,text,jsonb,text) TO nu54_p05_commerce;

CREATE FUNCTION ledger.expire_terminal_receipts()
RETURNS void LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    DELETE FROM ledger.checkout_pair p WHERE (p.branch_id,p.sale_id,p.customer_ref) IN (
      SELECT branch_id,sale_id,customer_ref FROM ledger.checkout_pair WHERE expires_at<=statement_timestamp()
      ORDER BY expires_at LIMIT 120 FOR UPDATE SKIP LOCKED
    );
    DELETE FROM ledger.terminal_mode_receipt r WHERE (r.branch_id,r.operator_ref,r.request_key) IN (
      SELECT branch_id,operator_ref,request_key FROM ledger.terminal_mode_receipt WHERE retire_after<=statement_timestamp()
      ORDER BY retire_after LIMIT 120 FOR UPDATE SKIP LOCKED
    );
$$;
REVOKE ALL ON FUNCTION ledger.expire_terminal_receipts() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.expire_terminal_receipts() TO nu54_p05_commerce;

-- C3 terminal operating mode. Existing orders retain their original mode and sale ID.
ALTER TABLE ledger.commerce_grant ADD COLUMN mode_revision bigint NOT NULL DEFAULT 1 CHECK (mode_revision>0);
CREATE TABLE ledger.terminal_mode_receipt (
    branch_id uuid NOT NULL,
    operator_ref uuid NOT NULL REFERENCES ledger.login_subject(subject_ref),
    request_key uuid NOT NULL,
    target_ref uuid NOT NULL,
    intent_digest bytea NOT NULL CHECK (octet_length(intent_digest)=32),
    retire_after timestamptz NOT NULL CHECK (isfinite(retire_after)),
    PRIMARY KEY (branch_id,operator_ref,request_key),
    FOREIGN KEY (branch_id,target_ref) REFERENCES ledger.commerce_grant(branch_id,actor_id)
);
REVOKE ALL ON ledger.terminal_mode_receipt FROM PUBLIC;

CREATE FUNCTION ledger.require_terminal_operator(tenant uuid, branch uuid, actor uuid)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM FROM ledger.staff_grant s JOIN ledger.branch b ON b.id=s.branch_id
      WHERE b.tenant_ref=tenant AND b.id=branch AND s.subject_ref=actor AND s.role_code='operator'
        AND s.valid_from<=statement_timestamp() AND (s.revoked_at IS NULL OR s.revoked_at>statement_timestamp())
      FOR SHARE OF s;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='terminal scope denied'; END IF;
END $$;

CREATE FUNCTION ledger.terminal_mode_state(tenant uuid, branch uuid, actor uuid, target uuid, role_ text)
RETURNS TABLE(mode text, revision bigint) LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE enrollment ledger.commerce_grant;
BEGIN
    IF role_='operator' THEN
        PERFORM ledger.require_terminal_operator(tenant,branch,actor);
    ELSIF role_ IS DISTINCT FROM 'terminal' OR target IS DISTINCT FROM actor THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='terminal scope denied';
    END IF;
    enrollment := ledger.commerce_allowed(tenant,branch,target);
    IF enrollment.role_code IS DISTINCT FROM 'terminal' THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='terminal scope denied';
    END IF;
    RETURN QUERY SELECT enrollment.terminal_mode,enrollment.mode_revision;
END $$;

CREATE FUNCTION ledger.change_terminal_mode(tenant uuid, branch uuid, actor uuid, target uuid, request_key uuid,
    expected_revision bigint, new_mode text, intent bytea)
RETURNS TABLE(mode text, revision bigint) LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    prior ledger.terminal_mode_receipt;
    policy ledger.command_rule;
BEGIN
    PERFORM ledger.require_terminal_operator(tenant,branch,actor);
    IF expected_revision < 1 OR expected_revision >= 9223372036854775807 OR new_mode NOT IN ('menu','pos')
        OR intent IS NULL OR octet_length(intent)<>32 OR request_key IS NULL THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid terminal mode update';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||actor::text||'terminal_mode'||request_key::text,0));
    SELECT * INTO prior FROM ledger.terminal_mode_receipt r WHERE r.branch_id=branch AND r.operator_ref=actor
      AND r.request_key=change_terminal_mode.request_key;
    IF FOUND THEN
        IF prior.intent_digest<>intent OR prior.target_ref<>target THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='request conflict';
        END IF;
        IF prior.retire_after<=statement_timestamp() THEN
            RAISE EXCEPTION USING ERRCODE='P0502', MESSAGE='request expired';
        END IF;
        RETURN QUERY SELECT new_mode,expected_revision+1;
        RETURN;
    END IF;
    SELECT * INTO policy FROM ledger.command_rule c WHERE c.branch_id=branch FOR SHARE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0503', MESSAGE='command policy unavailable'; END IF;
    UPDATE ledger.commerce_grant g SET terminal_mode=new_mode,mode_revision=expected_revision+1
      FROM ledger.branch b WHERE b.id=g.branch_id AND b.tenant_ref=tenant AND b.id=branch
      AND g.actor_id=target AND g.role_code='terminal' AND g.valid_until>statement_timestamp() AND g.mode_revision=expected_revision;
    IF NOT FOUND THEN
        -- Deny a missing/expired enrollment without disclosing its existence.
        PERFORM ledger.terminal_mode_state(tenant,branch,actor,target,'operator');
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='mode revision changed';
    END IF;
    INSERT INTO ledger.terminal_mode_receipt VALUES (branch,actor,request_key,target,intent,
      statement_timestamp()+policy.receipt_window);
    RETURN QUERY SELECT new_mode,expected_revision+1;
END $$;

REVOKE ALL ON FUNCTION ledger.require_terminal_operator(uuid,uuid,uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION ledger.terminal_mode_state(uuid,uuid,uuid,uuid,text) FROM PUBLIC;
REVOKE ALL ON FUNCTION ledger.change_terminal_mode(uuid,uuid,uuid,uuid,uuid,bigint,text,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.terminal_mode_state(uuid,uuid,uuid,uuid,text),
    ledger.change_terminal_mode(uuid,uuid,uuid,uuid,uuid,bigint,text,bytea) TO nu54_p05_commerce;

CREATE FUNCTION ledger.catalog_stock_page(tenant uuid, branch uuid, actor uuid, page integer)
RETURNS TABLE(version bigint, expires_at timestamptz, sku text, option_key text, label text, asset_ref text, unit_price numeric, available bigint)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT c.version,c.expires_at,c.sku,c.option_key,c.label,c.asset_ref,c.unit_price,coalesce(s.available,0)
    FROM ledger.catalog_page(tenant,branch,actor,page) c
    LEFT JOIN ledger.stock s ON s.branch_id=branch AND s.sku=c.sku;
$$;
REVOKE ALL ON FUNCTION ledger.catalog_stock_page(uuid,uuid,uuid,integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.catalog_stock_page(uuid,uuid,uuid,integer) TO nu54_p05_commerce;

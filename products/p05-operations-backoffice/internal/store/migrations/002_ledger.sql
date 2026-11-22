-- P05 owns branch membership, command receipts and the minimal order ledger.
CREATE SCHEMA ledger;
REVOKE ALL ON SCHEMA ledger FROM PUBLIC;

DO $roles$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'nu54_p05_staff') THEN
        CREATE ROLE nu54_p05_staff NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEROLE NOCREATEDB;
    END IF;
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'nu54_p05_staff'
                AND (rolcanlogin OR rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb OR rolreplication)) THEN
        RAISE EXCEPTION 'nu54_p05_staff is not a restricted capability role';
    END IF;
END
$roles$;

CREATE DOMAIN ledger.units AS numeric
    CHECK (VALUE >= 0 AND VALUE = trunc(VALUE) AND VALUE < power(2::numeric,256));
CREATE FUNCTION ledger.is_timezone(zone_name text) RETURNS boolean
    LANGUAGE sql STABLE SET search_path = pg_catalog, pg_temp
    AS $$SELECT EXISTS (SELECT FROM pg_catalog.pg_timezone_names WHERE name = zone_name)$$;
REVOKE ALL ON FUNCTION ledger.is_timezone(text) FROM PUBLIC;

CREATE TABLE ledger.branch (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_ref uuid NOT NULL,
    timezone text NOT NULL CHECK (ledger.is_timezone(timezone)),
    close_local time NOT NULL,
    UNIQUE (tenant_ref,id)
);

-- An approved identity connector maps a real PostgreSQL login to a local random subject.
CREATE TABLE ledger.login_subject (
    login_role name PRIMARY KEY,
    subject_ref uuid UNIQUE NOT NULL
);

CREATE TABLE ledger.staff_grant (
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    subject_ref uuid NOT NULL REFERENCES ledger.login_subject(subject_ref),
    role_code text NOT NULL CHECK (role_code IN ('operator','finance')),
    valid_from timestamptz NOT NULL,
    revoked_at timestamptz CHECK (revoked_at >= valid_from),
    PRIMARY KEY (branch_id,subject_ref,role_code,valid_from)
);

-- Configuration is explicit and scoped by branch; missing purpose/retention disables commands.
CREATE TABLE ledger.command_rule (
    branch_id uuid PRIMARY KEY REFERENCES ledger.branch,
    purpose_ref text NOT NULL CHECK (length(trim(purpose_ref)) BETWEEN 1 AND 120),
    basis_ref text NOT NULL CHECK (length(trim(basis_ref)) BETWEEN 1 AND 120),
    disposal_ref text NOT NULL CHECK (length(trim(disposal_ref)) BETWEEN 1 AND 120),
    receipt_window interval NOT NULL CHECK (receipt_window > interval '0')
);

CREATE TABLE ledger.sale (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    origin text NOT NULL CHECK (origin IN ('menu','pos')),
    origin_ref uuid NOT NULL,
    revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
    state text NOT NULL DEFAULT 'preview' CHECK (state IN ('preview','issued','checking','paid','closed')),
    asset_ref text NOT NULL CHECK (length(trim(asset_ref)) BETWEEN 1 AND 120),
    amount ledger.units NOT NULL CHECK (amount > 0),
    UNIQUE (branch_id,origin,origin_ref),
    UNIQUE (branch_id,id)
);

CREATE TABLE ledger.command_receipt (
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    scope_ref uuid NOT NULL,
    operation text NOT NULL CHECK (operation = 'create_sale'),
    request_key uuid NOT NULL,
    intent_digest bytea NOT NULL CHECK (octet_length(intent_digest) = 32),
    result_ref uuid NOT NULL,
    created_at timestamptz NOT NULL,
    retire_after timestamptz NOT NULL CHECK (retire_after > created_at),
    PRIMARY KEY (branch_id,scope_ref,operation,request_key),
    FOREIGN KEY (branch_id,result_ref) REFERENCES ledger.sale(branch_id,id)
);

CREATE FUNCTION ledger.require_branch(branch uuid, roles text[]) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
DECLARE
    actor uuid;
BEGIN
    SELECT grant_row.subject_ref INTO actor
    FROM ledger.staff_grant AS grant_row
    JOIN ledger.login_subject AS subject USING (subject_ref)
    WHERE subject.login_role = session_user
        AND grant_row.branch_id = branch
        AND grant_row.role_code = ANY (roles)
        AND grant_row.valid_from <= statement_timestamp()
        AND (grant_row.revoked_at IS NULL OR grant_row.revoked_at > statement_timestamp())
    ORDER BY grant_row.valid_from DESC
    LIMIT 1 FOR SHARE OF grant_row, subject;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='branch operation denied';
    END IF;
    RETURN actor;
END
$$;

CREATE FUNCTION ledger.create_sale(branch uuid, request_key uuid, sale_origin text, source_ref uuid,
                                   asset text, units ledger.units) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
DECLARE
    actor uuid := ledger.require_branch(branch, ARRAY['operator','finance']);
    intent bytea;
    prior ledger.command_receipt;
    rule ledger.command_rule;
    order_id uuid;
    issued timestamptz;
BEGIN
    IF request_key IS NULL OR source_ref IS NULL OR sale_origin IS NULL OR asset IS NULL OR units IS NULL
        OR units <= 0 OR sale_origin NOT IN ('menu','pos') OR length(trim(asset)) NOT BETWEEN 1 AND 120 THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid order input';
    END IF;

    -- Trim numeric scale to normalize identical integer amounts (23 and 23.0).
    intent := sha256(convert_to(jsonb_build_array(sale_origin,source_ref,asset,trim_scale(units))::text,'UTF8'));
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text || actor::text || 'create_sale' || request_key::text,0));
    issued := clock_timestamp();
    SELECT * INTO prior FROM ledger.command_receipt AS receipt
    WHERE receipt.branch_id=branch AND receipt.scope_ref=actor
        AND receipt.operation='create_sale' AND receipt.request_key=create_sale.request_key;
    IF FOUND THEN
        IF prior.intent_digest <> intent THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='request key reused with another intent';
        END IF;
        IF prior.retire_after <= issued THEN
            RAISE EXCEPTION USING ERRCODE='P0502', MESSAGE='command receipt expired; query the branch order';
        END IF;
        RETURN prior.result_ref;
    END IF;

    SELECT * INTO rule FROM ledger.command_rule WHERE branch_id=branch FOR SHARE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE='P0503', MESSAGE='purpose and retention policy required';
    END IF;
    INSERT INTO ledger.sale(branch_id,origin,origin_ref,asset_ref,amount)
        VALUES (branch,sale_origin,source_ref,asset,units)
        RETURNING id INTO order_id;
    INSERT INTO ledger.command_receipt
        VALUES (branch,actor,'create_sale',request_key,intent,order_id,issued,issued+rule.receipt_window);
    RETURN order_id;
END
$$;

CREATE FUNCTION ledger.export_sales(branch uuid)
RETURNS TABLE(id uuid, origin text, revision bigint, state text, asset_ref text, amount ledger.units)
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
BEGIN
    PERFORM ledger.require_branch(branch, ARRAY['finance']);
    RETURN QUERY SELECT s.id,s.origin,s.revision,s.state,s.asset_ref,s.amount
        FROM ledger.sale AS s WHERE s.branch_id=branch ORDER BY s.id;
END
$$;

REVOKE ALL ON ALL TABLES IN SCHEMA ledger FROM PUBLIC;
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA ledger FROM PUBLIC;
GRANT USAGE ON SCHEMA ledger TO nu54_p05_staff;
GRANT EXECUTE ON FUNCTION ledger.create_sale(uuid,uuid,text,uuid,text,ledger.units) TO nu54_p05_staff;
GRANT EXECUTE ON FUNCTION ledger.export_sales(uuid) TO nu54_p05_staff;

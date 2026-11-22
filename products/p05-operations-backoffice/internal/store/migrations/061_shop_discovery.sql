-- Entrance identity reveals only the branch's registered operating name.
CREATE FUNCTION ledger.shop_branch_label(tenant uuid, branch uuid, actor uuid)
RETURNS text LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE grant_ ledger.commerce_grant;
BEGIN
    grant_ := ledger.commerce_allowed(tenant,branch,actor);
    IF grant_.role_code NOT IN ('terminal','customer') THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='shop discovery scope denied';
    END IF;
    RETURN (SELECT label FROM ledger.operator_branch_label WHERE branch_id=branch);
END $$;

REVOKE ALL ON FUNCTION ledger.shop_branch_label(uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.shop_branch_label(uuid,uuid,uuid) TO nu54_p05_commerce;

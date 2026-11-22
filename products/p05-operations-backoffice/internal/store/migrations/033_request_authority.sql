-- Revalidate authenticated HTTP branch authority after any business lock wait.
-- The matching current grant is held through the caller's commit.
CREATE FUNCTION ledger.request_authority_live(
    tenant_ uuid, branch_ uuid, actor_ uuid, role_ text, capabilities_ text[]
) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF NOT EXISTS(SELECT FROM ledger.branch b WHERE b.id=branch_ AND b.tenant_ref=tenant_) THEN
        RETURN false;
    END IF;
    IF role_ IN ('customer','terminal') THEN
        PERFORM FROM ledger.commerce_grant g
        WHERE g.branch_id=branch_ AND g.actor_id=actor_ AND g.role_code=role_
            AND g.valid_until>clock_timestamp() FOR SHARE;
        RETURN FOUND;
    ELSIF role_='operator' AND cardinality(capabilities_)=0 THEN
        PERFORM FROM ledger.staff_grant g
        WHERE g.branch_id=branch_ AND g.subject_ref=actor_ AND g.role_code='operator'
            AND g.valid_from<=clock_timestamp() AND (g.revoked_at IS NULL OR g.revoked_at>clock_timestamp()) FOR SHARE;
        RETURN FOUND;
    ELSIF role_ IN ('operator','finance') AND cardinality(capabilities_)>0 THEN
        PERFORM FROM ledger.finance_grant g
        WHERE g.branch_id=branch_ AND g.actor_id=actor_ AND g.capability=ANY(capabilities_)
            AND g.valid_until>clock_timestamp() ORDER BY g.capability FOR SHARE;
        RETURN FOUND;
    END IF;
    RETURN false;
END $$;

REVOKE ALL ON FUNCTION ledger.request_authority_live(uuid,uuid,uuid,text,text[]) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.request_authority_live(uuid,uuid,uuid,text,text[])
TO nu54_p05_commerce, nu54_p05_settlement, nu54_p05_finance;

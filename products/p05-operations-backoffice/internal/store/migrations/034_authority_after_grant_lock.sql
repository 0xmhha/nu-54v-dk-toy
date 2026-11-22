-- Select and lock the grant before examining its wall-clock lifetime.
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
    IF role_ IN ('customer','terminal') THEN
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

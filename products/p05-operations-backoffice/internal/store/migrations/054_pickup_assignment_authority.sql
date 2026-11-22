-- Keep the assigned travel-pickup terminal capability separate from
-- the base shared terminal credential, including at transaction release.
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
    IF role_='printer_staff' THEN
        RETURN ledger.print_staff_live(tenant_,branch_,actor_);
    ELSIF role_='pos' THEN
        SELECT least(p.valid_until,g.valid_until) INTO ends_ FROM ledger.pos_source p
        JOIN ledger.commerce_grant g ON g.branch_id=p.branch_id AND g.actor_id=p.terminal_id
            AND g.role_code='terminal'
        WHERE p.branch_id=branch_ AND p.actor_id=actor_ FOR SHARE OF p,g;
        RETURN FOUND AND ends_>clock_timestamp();
    ELSIF role_ IN ('customer','terminal') THEN
        SELECT g.valid_until INTO ends_ FROM ledger.commerce_grant g
        WHERE g.branch_id=branch_ AND g.actor_id=actor_ AND g.role_code=role_ FOR SHARE;
        IF NOT FOUND THEN RETURN false; END IF;
        IF role_='terminal' AND capabilities_=ARRAY['pickup_assignment'] THEN
            PERFORM FROM ledger.travel_pickup_terminal t
              WHERE t.branch_id=branch_ AND t.actor_id=actor_ FOR SHARE;
            IF NOT FOUND THEN RETURN false; END IF;
        END IF;
        RETURN ends_>clock_timestamp();
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

CREATE OR REPLACE FUNCTION ledger.kiosk_require_pickup(tenant_ uuid,branch_ uuid,actor_ uuid,role_ text)
RETURNS void LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF role_='terminal' AND ledger.request_authority_live(tenant_,branch_,actor_,role_,ARRAY['pickup_assignment']) THEN
        RETURN;
    ELSIF role_='operator' AND ledger.request_authority_live(tenant_,branch_,actor_,role_,ARRAY['pickup_manage']) THEN
        RETURN;
    ELSE
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='pickup assignment unavailable';
    END IF;
END $$;


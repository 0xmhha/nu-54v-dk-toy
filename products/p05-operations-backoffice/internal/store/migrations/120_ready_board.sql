-- The ready board on the customer tablet: today's pickup numbers by stage, and nothing else. No amount, no item,
-- no order reference and no customer leave this function. A number goes once it is handed over, refused or refunded.

CREATE FUNCTION ledger.ready_board(tenant uuid, branch uuid, actor uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE grant_ ledger.commerce_grant := ledger.commerce_allowed(tenant, branch, actor);
DECLARE day_ date := ledger.ledger_business_day(branch, clock_timestamp());
BEGIN
    IF grant_.role_code IS DISTINCT FROM 'terminal' THEN
        RAISE EXCEPTION USING ERRCODE = '42501', MESSAGE = 'ready board is for the store tablet';
    END IF;
    RETURN (
        WITH today AS (
            SELECT p.number, coalesce(k.state, 'received') AS stage, k.updated_at
              FROM ledger.pickup p
              JOIN ledger.sale s ON (s.branch_id, s.id) = (p.branch_id, p.sale_id)
              LEFT JOIN ledger.travel_pickup_progress k ON k.sale_id = p.sale_id
             WHERE p.branch_id = branch AND p.business_day = day_ AND s.state = 'paid'
               AND coalesce(k.state, 'received') IN ('received', 'preparing', 'ready')
               AND NOT EXISTS (SELECT FROM ledger.refund r WHERE r.sale_id = p.sale_id))
        SELECT jsonb_build_object(
            'checkedAt', to_char(clock_timestamp() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
            -- Newest ready first: the band shows the first four.
            'ready', coalesce((SELECT jsonb_agg(number ORDER BY updated_at DESC, number DESC)
                FROM (SELECT * FROM today WHERE stage = 'ready' ORDER BY updated_at DESC, number DESC LIMIT 40) r), '[]'),
            'making', coalesce((SELECT jsonb_agg(number ORDER BY number)
                FROM (SELECT * FROM today WHERE stage = 'preparing' ORDER BY number LIMIT 40) m), '[]'),
            'received', coalesce((SELECT jsonb_agg(number ORDER BY number)
                FROM (SELECT * FROM today WHERE stage = 'received' ORDER BY number LIMIT 40) n), '[]'))
    );
END $$;
REVOKE ALL ON FUNCTION ledger.ready_board(uuid, uuid, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.ready_board(uuid, uuid, uuid) TO nu54_p05_commerce;

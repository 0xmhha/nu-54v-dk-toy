CREATE FUNCTION ledger.rental_close_operator_snapshot(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE prior ledger.rental_close_candidate;
BEGIN
    PERFORM ledger.rental_require_live_grant(tenant,branch,actor);
    SELECT * INTO prior FROM ledger.rental_close_candidate c
       WHERE (c.branch_id,c.device_ref,c.generation)=(branch,device,generation_);
    IF FOUND THEN
        RETURN jsonb_build_object('protected',true,'state',prior.state,'hash','0x'||encode(prior.tx_hash,'hex'))
            || CASE WHEN prior.actor_id=actor THEN jsonb_build_object('key',prior.request_key,'review',prior.review_ref) ELSE '{}'::jsonb END;
    END IF;
    RETURN jsonb_build_object('protected',
        ledger.rental_runtime_close_hold(tenant,branch,actor,device,generation_)
        AND NOT EXISTS(SELECT FROM ledger.rental_operation o
             WHERE (o.branch_id,o.device_ref,o.generation,o.action)=(branch,device,generation_,'close')));
END $$;
REVOKE ALL ON FUNCTION ledger.rental_close_operator_snapshot(uuid,uuid,uuid,uuid,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.rental_close_operator_snapshot(uuid,uuid,uuid,uuid,bigint) TO nu54_p05_rental_close;

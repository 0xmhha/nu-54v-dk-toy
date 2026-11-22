CREATE FUNCTION ledger.experience_with_timezones(branch uuid, snapshot jsonb)
RETURNS jsonb LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT jsonb_build_object(
      'bookings',coalesce((SELECT jsonb_agg(b.value || jsonb_build_object('timezone',site.timezone) ORDER BY b.seq)
        FROM jsonb_array_elements(snapshot->'bookings') WITH ORDINALITY b(value,seq)
        JOIN ledger.branch site ON site.id=branch),'[]'::jsonb),
      'subscriptions',coalesce((SELECT jsonb_agg(s.value || jsonb_build_object('timezone',model.timezone) ORDER BY s.seq)
        FROM jsonb_array_elements(snapshot->'subscriptions') WITH ORDINALITY s(value,seq)
        JOIN ledger.subscription model ON model.branch_id=branch AND model.id=(s.value->>'id')::uuid),'[]'::jsonb));
$$;
REVOKE ALL ON FUNCTION ledger.experience_with_timezones(uuid,jsonb) FROM PUBLIC;

CREATE FUNCTION ledger.operator_local_experience(tenant uuid,branch uuid,actor uuid,detail_ uuid)
RETURNS jsonb LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT ledger.experience_with_timezones(branch,ledger.operator_experience(tenant,branch,actor,detail_));
$$;
CREATE FUNCTION ledger.operator_local_cancel_subscription(tenant uuid,branch uuid,actor uuid,subject uuid,key_ uuid,revision_ bigint)
RETURNS jsonb LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT ledger.experience_with_timezones(branch,ledger.operator_cancel_subscription(tenant,branch,actor,subject,key_,revision_));
$$;
REVOKE ALL ON FUNCTION ledger.operator_local_experience(uuid,uuid,uuid,uuid),
    ledger.operator_local_cancel_subscription(uuid,uuid,uuid,uuid,uuid,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.operator_local_experience(uuid,uuid,uuid,uuid),
    ledger.operator_local_cancel_subscription(uuid,uuid,uuid,uuid,uuid,bigint) TO nu54_p05_commerce;

CREATE FUNCTION travel_private.recommendation_allowed(actor_ uuid,revision_ bigint,plan_ uuid)
RETURNS timestamptz LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $permit$
    SELECT travel_private.require_consent(actor_,'recommendation',revision_,'p09',plan_)
$permit$;
REVOKE ALL ON FUNCTION travel_private.recommendation_allowed(uuid,bigint,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION travel_private.recommendation_allowed(uuid,bigint,uuid) TO nu54_p09_private_user;

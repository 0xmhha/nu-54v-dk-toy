CREATE FUNCTION travel_private.list_itineraries(actor_ uuid)
RETURNS TABLE(id uuid, revision bigint, sealed bytea)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $list$
DECLARE candidate uuid;
BEGIN
    -- Read through the detail guard so fences and current consent govern both routes.
    FOR candidate IN
        SELECT i.id FROM travel_private.itinerary i
        JOIN travel_private.consent c ON c.subject_ref=i.subject_ref
            AND c.purpose='recommendation' AND c.revision=i.consent_revision
        WHERE i.subject_ref=actor_ AND i.expires_at>clock_timestamp()
        ORDER BY i.expires_at DESC,i.id LIMIT 50
    LOOP
        BEGIN
            RETURN QUERY SELECT candidate, visible.revision, visible.sealed
              FROM travel_private.read_itinerary(actor_,candidate) visible;
        EXCEPTION WHEN insufficient_privilege THEN
            CONTINUE;
        END;
    END LOOP;
END
$list$;

REVOKE ALL ON FUNCTION travel_private.list_itineraries(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION travel_private.list_itineraries(uuid) TO nu54_p09_private_user;

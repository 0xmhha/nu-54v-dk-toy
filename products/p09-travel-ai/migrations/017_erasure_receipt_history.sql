CREATE FUNCTION travel_private.list_erasure_receipts(actor_ uuid,before_ uuid)
RETURNS TABLE(id uuid, requested_at timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $history$
    SELECT e.id,e.requested_at
      FROM travel_private.erasure_request e
      WHERE e.subject_ref=actor_ AND e.requested_at>statement_timestamp()-interval '90 days'
        AND (before_ IS NULL OR (e.requested_at,e.id) < (
          SELECT cursor_row.requested_at,cursor_row.id FROM travel_private.erasure_request cursor_row
          WHERE cursor_row.subject_ref=actor_ AND cursor_row.id=before_
            AND cursor_row.requested_at>statement_timestamp()-interval '90 days'
        ))
      ORDER BY e.requested_at DESC,e.id DESC
      LIMIT 51
$history$;

REVOKE ALL ON FUNCTION travel_private.list_erasure_receipts(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION travel_private.list_erasure_receipts(uuid,uuid) TO nu54_p09_private_user;

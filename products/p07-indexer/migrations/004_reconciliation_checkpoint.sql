-- Keep the stream fence at the P07 owner boundary while P05 records exact evidence.
CREATE FUNCTION chainview.reconciliation_ready(p_stream uuid, p_merchant text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $ready$
DECLARE
    ready boolean;
BEGIN
    IF NOT EXISTS (SELECT FROM chainview.receipt_scope scope WHERE scope.login_role=session_user
        AND scope.stream_id=p_stream AND scope.merchant_key=p_merchant AND scope.valid_until>clock_timestamp()) THEN
        RETURN false;
    END IF;
    PERFORM pg_advisory_xact_lock_shared(hashtextextended(p_stream::text,9));
    SELECT g.state='ready' AND EXISTS (
        SELECT FROM chainview.receipt_scope scope WHERE scope.login_role=session_user
            AND scope.stream_id=p_stream AND scope.merchant_key=p_merchant AND scope.valid_until>clock_timestamp()
    ) AND NOT EXISTS (
        SELECT FROM chainview.incident i JOIN chainview.generation affected ON affected.id=i.generation_id
        WHERE affected.stream_id=p_stream AND i.resolved_at IS NULL
    ) INTO ready
    FROM chainview.read_target target JOIN chainview.generation g ON g.id=target.generation_id
    WHERE target.stream_id=p_stream;
    RETURN COALESCE(ready,false);
END
$ready$;
REVOKE ALL ON FUNCTION chainview.reconciliation_ready(uuid,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION chainview.reconciliation_ready(uuid,text) TO nu54_p07_reconcile;

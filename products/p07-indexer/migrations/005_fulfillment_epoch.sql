CREATE FUNCTION chainview.fulfillment_epoch(p_stream uuid, p_merchant text)
RETURNS text LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $epoch$
DECLARE
    token text;
BEGIN
    IF NOT chainview.reconciliation_ready(p_stream,p_merchant) THEN RETURN NULL; END IF;
    PERFORM 1 FROM chainview.receipt_scope s WHERE s.login_role=session_user AND s.stream_id=p_stream
        AND s.merchant_key=p_merchant AND s.valid_until>clock_timestamp() FOR SHARE;
    IF NOT FOUND THEN RETURN NULL; END IF;
    SELECT encode(sha256(convert_to(jsonb_build_array(t.stream_id,t.generation_id,t.revision,
        (SELECT COALESCE(jsonb_agg(i.id ORDER BY i.id),'[]'::jsonb)
         FROM chainview.incident i JOIN chainview.generation g ON g.id=i.generation_id
         WHERE g.stream_id=p_stream))::text,'UTF8')),'hex')
    INTO token FROM chainview.read_target t WHERE t.stream_id=p_stream FOR SHARE OF t;
    RETURN token;
END
$epoch$;
REVOKE ALL ON FUNCTION chainview.fulfillment_epoch(uuid,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION chainview.fulfillment_epoch(uuid,text) TO nu54_p07_reconcile;

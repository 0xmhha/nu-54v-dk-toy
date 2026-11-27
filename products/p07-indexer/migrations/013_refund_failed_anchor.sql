-- Receipt roots are checked by the settlement reader. This function pins a
-- failed receipt's block to the existing authorized active finalized view.
CREATE FUNCTION chainview.reconciliation_finalized_block(stream_ uuid,merchant_ text,height_ bigint,hash_ text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
BEGIN
    IF NOT chainview.reconciliation_ready(stream_,merchant_) THEN RETURN false; END IF;
    RETURN EXISTS (
        SELECT FROM chainview.read_target t
        JOIN chainview.generation g ON g.id=t.generation_id
        JOIN chainview.block b ON b.generation_id=g.id
        WHERE t.stream_id=stream_ AND b.canonical AND b.height=height_ AND b.block_hash=hash_
          AND b.height<=g.finalized_height
    );
END
$$;
REVOKE ALL ON FUNCTION chainview.reconciliation_finalized_block(uuid,text,bigint,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION chainview.reconciliation_finalized_block(uuid,text,bigint,text) TO nu54_p07_reconcile;

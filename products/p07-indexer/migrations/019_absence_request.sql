-- The operations server asks for an absence proof; the chain reader works the requests.
CREATE TABLE chainview.absence_request (
    stream_id uuid NOT NULL REFERENCES chainview.stream,
    merchant_key text NOT NULL CHECK (merchant_key ~ '^0x[0-9a-f]{64}$'),
    order_key text NOT NULL CHECK (order_key ~ '^0x[0-9a-f]{64}$'),
    quote_expires bigint NOT NULL CHECK (quote_expires>0),
    requested_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts>=0),
    next_attempt_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (stream_id,merchant_key,order_key,quote_expires)
);
CREATE INDEX absence_request_due ON chainview.absence_request(stream_id,next_attempt_at);
REVOKE ALL ON chainview.absence_request FROM PUBLIC;
GRANT SELECT,DELETE ON chainview.absence_request TO nu54_p07_ingest;
GRANT UPDATE (attempts,next_attempt_at) ON chainview.absence_request TO nu54_p07_ingest;

-- A confirmed payment or lock, or a live proof for the same signed expiry, leaves nothing to prove.
-- The proof text is the one absence.go writes. A proof about to expire may be requested again.
CREATE FUNCTION chainview.absence_answered(p_stream uuid, p_merchant text, p_order text, p_expires bigint)
RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $sql$
    SELECT EXISTS (
        SELECT FROM chainview.current_evidence e JOIN chainview.read_target t ON t.generation_id=e.generation_id
        WHERE t.stream_id=p_stream AND e.merchant_key=p_merchant AND e.order_key=p_order
          AND (e.kind IN ('paid','locked') OR (e.kind='absent' AND e.expires_at>clock_timestamp()+interval '2 minutes'
            AND e.state_proof_ref='sha256:'||encode(sha256(convert_to(concat_ws(chr(10),'nu54-absence-v2',e.policy_ref,
                p_stream::text,p_merchant,p_order,e.anchor_hash,p_expires::text,'false'),'UTF8')),'hex')))
    )
$sql$;
REVOKE ALL ON FUNCTION chainview.absence_answered(uuid,text,text,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION chainview.absence_answered(uuid,text,text,bigint) TO nu54_p07_ingest;

-- True means the request is waiting or its proof is already readable. False means it was not kept:
-- no live grant, not a settlement or escrow stream, an invalid or far expiry, a paid order, or a full backlog.
CREATE FUNCTION chainview.request_absence(p_stream uuid, p_merchant text, p_order text, p_expires bigint)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $request$
BEGIN
    IF p_expires IS NULL OR p_expires<=0 OR p_expires>3600+extract(epoch FROM greatest(clock_timestamp(),
            (SELECT anchor.block_time FROM chainview.read_target t JOIN chainview.generation g ON g.id=t.generation_id
             JOIN chainview.block anchor ON anchor.generation_id=g.id AND anchor.block_hash=g.finalized_hash
             WHERE t.stream_id=p_stream)))
       OR p_merchant IS NULL OR p_merchant !~ '^0x[0-9a-f]{64}$'
       OR p_order IS NULL OR p_order !~ '^0x[0-9a-f]{64}$' OR ltrim(substr(p_order,3),'0')='' OR ltrim(substr(p_merchant,3),'0')=''
       OR NOT EXISTS (SELECT FROM chainview.receipt_scope scope WHERE scope.login_role=session_user
            AND scope.stream_id=p_stream AND scope.merchant_key=p_merchant AND scope.valid_until>clock_timestamp())
       OR NOT EXISTS (SELECT FROM chainview.deployment_profile p WHERE p.stream_id=p_stream AND p.version IN (2,3)) THEN
        RETURN false;
    END IF;
    IF chainview.absence_answered(p_stream,p_merchant,p_order,p_expires) THEN
        RETURN NOT EXISTS (
            SELECT FROM chainview.current_evidence e JOIN chainview.read_target t ON t.generation_id=e.generation_id
            WHERE t.stream_id=p_stream AND e.merchant_key=p_merchant AND e.order_key=p_order AND e.kind IN ('paid','locked'));
    END IF;
    -- One caller at a time counts the merchant's backlog.
    PERFORM pg_advisory_xact_lock(hashtextextended(p_stream::text||p_merchant,19));
    IF EXISTS (SELECT FROM chainview.absence_request r
        WHERE (r.stream_id,r.merchant_key,r.order_key,r.quote_expires)=(p_stream,p_merchant,p_order,p_expires)) THEN
        RETURN true;
    END IF;
    IF (SELECT count(*) FROM chainview.absence_request r WHERE r.stream_id=p_stream AND r.merchant_key=p_merchant)>=1000 THEN
        RETURN false;
    END IF;
    INSERT INTO chainview.absence_request(stream_id,merchant_key,order_key,quote_expires)
        VALUES(p_stream,p_merchant,p_order,p_expires) ON CONFLICT DO NOTHING;
    RETURN true;
END
$request$;
REVOKE ALL ON FUNCTION chainview.request_absence(uuid,text,text,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION chainview.request_absence(uuid,text,text,bigint) TO nu54_p07_reconcile;

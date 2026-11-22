ALTER TABLE ledger.relay_attempt ADD COLUMN broadcast_epoch text CHECK (broadcast_epoch ~ '^[0-9a-f]{64}$'),
    ADD COLUMN broadcast_fence bigint CHECK (broadcast_fence>0),
    ADD CONSTRAINT relay_epoch_complete CHECK ((broadcast_epoch IS NULL)=(broadcast_fence IS NULL));

CREATE FUNCTION ledger.authorize_relay_send(work_ uuid,fence_ bigint,hash_ text,epoch_ text)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $authorize$
BEGIN
    PERFORM ledger.valid_settlement_lease(work_,fence_);
    UPDATE ledger.relay_attempt SET broadcast_epoch=epoch_,broadcast_fence=fence_,state='unknown'
    WHERE work_id=work_ AND tx_hash=hash_ AND state IN ('signed','unknown','broadcast')
      AND expires_at>clock_timestamp() AND (broadcast_fence IS NULL OR broadcast_fence<fence_);
    IF NOT FOUND THEN RAISE EXCEPTION 'relay attempt no longer authorized' USING ERRCODE='P1002'; END IF;
END
$authorize$;

CREATE FUNCTION ledger.ack_relay_send(work_ uuid,fence_ bigint,hash_ text,epoch_ text)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $ack$
BEGIN
    PERFORM ledger.valid_settlement_lease(work_,fence_);
    UPDATE ledger.relay_attempt SET state='broadcast' WHERE work_id=work_ AND tx_hash=hash_
      AND broadcast_epoch=epoch_ AND broadcast_fence=fence_ AND state='unknown';
    IF NOT FOUND THEN RAISE EXCEPTION 'relay broadcast authorization changed' USING ERRCODE='P1002'; END IF;
END
$ack$;

REVOKE ALL ON FUNCTION ledger.authorize_relay_send(uuid,bigint,text,text),
    ledger.ack_relay_send(uuid,bigint,text,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.authorize_relay_send(uuid,bigint,text,text),
    ledger.ack_relay_send(uuid,bigint,text,text) TO nu54_p05_settlement;

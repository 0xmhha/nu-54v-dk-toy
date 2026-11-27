DO $roles$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p05_receiver') THEN
        CREATE ROLE nu54_p05_receiver NOLOGIN NOSUPERUSER NOBYPASSRLS;
    END IF;
END
$roles$;

ALTER TABLE ledger.fulfillment_outbox ADD COLUMN authorized_epoch text CHECK (authorized_epoch ~ '^[0-9a-f]{64}$'),
    ADD COLUMN authorized_fence bigint CHECK (authorized_fence>0),
    ADD COLUMN authorized_post boolean,
    ADD COLUMN payload_digest bytea CHECK (octet_length(payload_digest)=32),
    ADD CONSTRAINT fulfillment_authorization_complete CHECK (
        (authorized_epoch IS NULL AND authorized_fence IS NULL AND authorized_post IS NULL AND payload_digest IS NULL) OR
        (authorized_epoch IS NOT NULL AND authorized_fence IS NOT NULL AND authorized_post IS NOT NULL AND payload_digest IS NOT NULL));

CREATE FUNCTION ledger.prepare_fulfillment(event_ uuid,fence_ bigint,effect_ uuid,epoch_ text,digest_ bytea,post_ boolean)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $prepare$
BEGIN
    UPDATE ledger.fulfillment_outbox e SET authorized_epoch=epoch_,authorized_fence=fence_,authorized_post=post_,payload_digest=digest_
    WHERE e.event_id=event_ AND e.effect_key=effect_ AND e.fence=fence_ AND e.state='sending'
      AND e.lease_until>clock_timestamp() AND (e.authorized_fence IS NULL OR e.authorized_fence<fence_)
      AND (e.payload_digest IS NULL OR e.payload_digest=digest_);
    IF NOT FOUND THEN RAISE EXCEPTION 'invalid fulfillment authorization' USING ERRCODE='P1002'; END IF;
END
$prepare$;

CREATE TABLE ledger.consumer_receipt (
    event_id uuid PRIMARY KEY REFERENCES ledger.fulfillment_outbox,
    effect_key uuid NOT NULL UNIQUE,
    payload_digest bytea NOT NULL CHECK(octet_length(payload_digest)=32),
    accepted_epoch text NOT NULL CHECK(accepted_epoch ~ '^[0-9a-f]{64}$'),
    accepted_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

CREATE FUNCTION ledger.receiver_source(event_ uuid,effect_ uuid,fence_ bigint,epoch_ text,digest_ bytea,
    allowed_branches uuid[])
RETURNS TABLE(stream_id uuid,merchant_key text,branch_id uuid,sale_id uuid,kind text,source_ref uuid,revision bigint,
    pickup_no bigint,receipt_ref uuid)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $scope$
    SELECT p.stream_id,p.merchant_key,s.branch_id,e.sale_id,e.kind,e.source_ref,e.revision,
        CASE WHEN e.kind='paid' THEN pickup.number END,CASE WHEN e.kind='paid' THEN pickup.receipt_ref END
    FROM ledger.fulfillment_outbox e JOIN ledger.sale s ON s.id=e.sale_id JOIN ledger.payment p ON p.sale_id=s.id
    JOIN ledger.pickup pickup ON pickup.sale_id=s.id
    WHERE e.event_id=event_ AND e.effect_key=effect_ AND e.fence=fence_ AND e.authorized_fence=fence_
      AND e.authorized_epoch=epoch_ AND e.payload_digest=digest_
      AND e.lease_until>clock_timestamp() AND e.state='sending' AND s.branch_id=ANY(allowed_branches)
$scope$;

CREATE FUNCTION ledger.receive_fulfillment(event_ uuid,effect_ uuid,fence_ bigint,epoch_ text,digest_ bytea,
    branch_ uuid,stream_ uuid,merchant_ text,accept_ boolean)
RETURNS text LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $receive$
DECLARE
    pending ledger.fulfillment_outbox;
    previous ledger.consumer_receipt;
BEGIN
    -- Serialize the decision with sender lease replacement and prior acceptance.
    SELECT e.* INTO pending FROM ledger.fulfillment_outbox e
    JOIN ledger.sale s ON s.id=e.sale_id JOIN ledger.payment p ON p.sale_id=s.id
    WHERE e.event_id=event_ AND e.effect_key=effect_ AND s.branch_id=branch_
      AND p.stream_id=stream_ AND p.merchant_key=merchant_ FOR UPDATE OF e;
    IF NOT FOUND OR pending.fence IS DISTINCT FROM fence_ OR pending.authorized_fence IS DISTINCT FROM fence_
       OR pending.authorized_epoch IS DISTINCT FROM epoch_ OR pending.payload_digest IS DISTINCT FROM digest_
       OR pending.state<>'sending' OR pending.authorized_post IS DISTINCT FROM accept_ OR pending.lease_until<=clock_timestamp() THEN
        RETURN 'unknown';
    END IF;
    SELECT * INTO previous FROM ledger.consumer_receipt WHERE event_id=event_;
    IF FOUND THEN
        IF previous.effect_key<>effect_ OR previous.payload_digest<>digest_ THEN RETURN 'unknown'; END IF;
        RETURN 'delivered';
    END IF;
    IF NOT accept_ THEN RETURN 'absent'; END IF;
    INSERT INTO ledger.consumer_receipt(event_id,effect_key,payload_digest,accepted_epoch) VALUES(event_,effect_,digest_,epoch_);
    RETURN 'delivered';
END
$receive$;

REVOKE ALL ON FUNCTION ledger.prepare_fulfillment(uuid,bigint,uuid,text,bytea,boolean),
    ledger.receiver_source(uuid,uuid,bigint,text,bytea,uuid[]),
    ledger.receive_fulfillment(uuid,uuid,bigint,text,bytea,uuid,uuid,text,boolean) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.prepare_fulfillment(uuid,bigint,uuid,text,bytea,boolean) TO nu54_p05_dispatch;
GRANT USAGE ON SCHEMA ledger TO nu54_p05_receiver;
GRANT EXECUTE ON FUNCTION ledger.receiver_source(uuid,uuid,bigint,text,bytea,uuid[]),
    ledger.receive_fulfillment(uuid,uuid,bigint,text,bytea,uuid,uuid,text,boolean) TO nu54_p05_receiver;

CREATE FUNCTION ledger.finish_fenced_fulfillment(event_ uuid,fence_ bigint,effect_ uuid,epoch_ text,result_ text,digest_ bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $ack$
BEGIN
    IF NOT EXISTS (SELECT FROM ledger.fulfillment_outbox e WHERE e.event_id=event_ AND e.effect_key=effect_
        AND e.fence=fence_ AND e.authorized_fence=fence_ AND e.authorized_epoch=epoch_ AND e.payload_digest=digest_
        AND (NOT e.authorized_post OR result_<>'absent') FOR UPDATE) THEN
        RAISE EXCEPTION 'fulfillment acknowledgement differs from authorization' USING ERRCODE='P1002';
    END IF;
    RETURN ledger.finish_fulfillment(event_,fence_,effect_,result_,digest_);
END
$ack$;
REVOKE EXECUTE ON FUNCTION ledger.finish_fulfillment(uuid,bigint,uuid,text,bytea) FROM nu54_p05_dispatch;
REVOKE ALL ON FUNCTION ledger.finish_fenced_fulfillment(uuid,bigint,uuid,text,text,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.finish_fenced_fulfillment(uuid,bigint,uuid,text,text,bytea) TO nu54_p05_dispatch;

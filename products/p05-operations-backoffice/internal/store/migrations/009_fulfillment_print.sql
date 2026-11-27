DO $role$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p05_print') THEN
        CREATE ROLE nu54_p05_print NOLOGIN NOSUPERUSER NOBYPASSRLS;
    END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p05_print_staff') THEN
        CREATE ROLE nu54_p05_print_staff NOLOGIN NOSUPERUSER NOBYPASSRLS;
    END IF;
END
$role$;

CREATE TABLE ledger.print_attempt (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id uuid NOT NULL REFERENCES ledger.consumer_receipt,
    previous_id uuid UNIQUE REFERENCES ledger.print_attempt,
    authorizing_epoch text NOT NULL CHECK (authorizing_epoch ~ '^[0-9a-f]{64}$'),
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','unknown','printed','superseded')),
    next_check timestamptz NOT NULL DEFAULT clock_timestamp(),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    finished_at timestamptz
);
CREATE UNIQUE INDEX print_initial_attempt ON ledger.print_attempt(event_id) WHERE previous_id IS NULL;
CREATE INDEX print_pending_attempt ON ledger.print_attempt(next_check,id) WHERE state='pending';

CREATE TABLE ledger.print_staff_grant (
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    actor_id uuid NOT NULL,
    valid_until timestamptz NOT NULL CHECK (isfinite(valid_until)),
    PRIMARY KEY(branch_id,actor_id)
);
CREATE TABLE ledger.print_decision (
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    actor_id uuid NOT NULL,
    request_key uuid NOT NULL,
    attempt_id uuid NOT NULL UNIQUE REFERENCES ledger.print_attempt,
    action text NOT NULL CHECK (action IN ('confirm_printed','reprint')),
    result_id uuid NOT NULL REFERENCES ledger.print_attempt,
    decided_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(branch_id,actor_id,request_key)
);

CREATE FUNCTION ledger.enqueue_print_receipt() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $queue$
BEGIN
    INSERT INTO ledger.print_attempt(event_id,authorizing_epoch)
    SELECT NEW.event_id,NEW.accepted_epoch FROM ledger.fulfillment_outbox e WHERE e.event_id=NEW.event_id AND e.kind='paid';
    RETURN NEW;
END
$queue$;
CREATE TRIGGER enqueue_print_receipt AFTER INSERT ON ledger.consumer_receipt
FOR EACH ROW EXECUTE FUNCTION ledger.enqueue_print_receipt();

CREATE FUNCTION ledger.next_print()
RETURNS TABLE(attempt_id uuid,stream_id uuid,merchant_key text)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $next$
    SELECT a.id,p.stream_id,p.merchant_key FROM ledger.print_attempt a
    JOIN ledger.fulfillment_outbox e ON e.event_id=a.event_id JOIN ledger.payment p ON p.sale_id=e.sale_id
    WHERE a.state='pending' AND a.next_check<=clock_timestamp()
    ORDER BY a.next_check,a.id LIMIT 1
$next$;

CREATE FUNCTION ledger.delay_print(attempt_ uuid)
RETURNS void LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $delay$
    UPDATE ledger.print_attempt SET next_check=clock_timestamp()+interval '5 seconds' WHERE id=attempt_ AND state='pending'
$delay$;

CREATE FUNCTION ledger.begin_print(attempt_ uuid,epoch_ text)
RETURNS TABLE(receipt_ref uuid,branch_id uuid,pickup_no bigint)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $begin$
    WITH accepted AS (
      UPDATE ledger.print_attempt SET state='unknown' WHERE id=attempt_ AND state='pending' AND authorizing_epoch=epoch_
      RETURNING event_id
    )
    SELECT p.receipt_ref,p.branch_id,p.number FROM accepted
    JOIN ledger.fulfillment_outbox e USING(event_id) JOIN ledger.pickup p ON p.sale_id=e.sale_id
$begin$;

CREATE FUNCTION ledger.complete_print(attempt_ uuid,epoch_ text)
RETURNS void LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $done$
    UPDATE ledger.print_attempt SET state='printed',finished_at=clock_timestamp()
    WHERE id=attempt_ AND state='unknown' AND authorizing_epoch=epoch_
$done$;

CREATE FUNCTION ledger.print_staff_source(tenant_ uuid,branch_ uuid,actor_ uuid,attempt_ uuid)
RETURNS TABLE(stream_id uuid,merchant_key text)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $staff$
    SELECT p.stream_id,p.merchant_key FROM ledger.print_attempt a
    JOIN ledger.fulfillment_outbox e ON e.event_id=a.event_id JOIN ledger.payment p ON p.sale_id=e.sale_id
    JOIN ledger.sale s ON s.id=e.sale_id JOIN ledger.branch b ON b.id=s.branch_id
    JOIN ledger.print_staff_grant g ON g.branch_id=b.id
    WHERE a.id=attempt_ AND b.id=branch_ AND b.tenant_ref=tenant_ AND g.actor_id=actor_ AND g.valid_until>clock_timestamp()
$staff$;

CREATE FUNCTION ledger.resolve_print(tenant_ uuid,branch_ uuid,actor_ uuid,attempt_ uuid,key_ uuid,action_ text,epoch_ text)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $resolve$
DECLARE
    prior ledger.print_attempt;
    decided ledger.print_decision;
    result_ uuid;
BEGIN
    PERFORM 1 FROM ledger.print_staff_grant g JOIN ledger.branch b ON b.id=g.branch_id
      WHERE b.id=branch_ AND b.tenant_ref=tenant_ AND g.actor_id=actor_ AND g.valid_until>clock_timestamp() FOR SHARE OF g;
    IF NOT FOUND OR NOT EXISTS (SELECT FROM ledger.print_staff_source(tenant_,branch_,actor_,attempt_)) THEN
        RAISE EXCEPTION 'print decision denied' USING ERRCODE='P1003';
    END IF;
    -- An operator's idempotency key fences retries across different attempts.
    PERFORM pg_advisory_xact_lock(hashtextextended(branch_::text||actor_::text||key_::text,59));
    SELECT * INTO decided FROM ledger.print_decision WHERE branch_id=branch_ AND actor_id=actor_ AND request_key=key_;
    IF FOUND THEN
        IF (decided.attempt_id,decided.action) IS DISTINCT FROM (attempt_,action_) THEN
            RAISE EXCEPTION 'print decision key conflict' USING ERRCODE='P1001';
        END IF;
        RETURN decided.result_id;
    END IF;
    SELECT * INTO STRICT prior FROM ledger.print_attempt WHERE id=attempt_ FOR UPDATE;
    IF action_='confirm_printed' AND prior.state='unknown' THEN
        UPDATE ledger.print_attempt SET state='printed',finished_at=clock_timestamp() WHERE id=attempt_;
        result_:=attempt_;
    ELSIF action_='reprint' AND (prior.state='unknown' OR (prior.state='pending' AND prior.authorizing_epoch<>epoch_)) THEN
        UPDATE ledger.print_attempt SET state='superseded',finished_at=clock_timestamp() WHERE id=attempt_;
        INSERT INTO ledger.print_attempt(event_id,previous_id,authorizing_epoch) VALUES(prior.event_id,attempt_,epoch_) RETURNING id INTO result_;
    ELSE
        RAISE EXCEPTION 'print attempt cannot use that decision' USING ERRCODE='P1001';
    END IF;
    INSERT INTO ledger.print_decision(branch_id,actor_id,request_key,attempt_id,action,result_id)
      VALUES(branch_,actor_,key_,attempt_,action_,result_);
    RETURN result_;
END
$resolve$;

REVOKE ALL ON FUNCTION ledger.enqueue_print_receipt(),ledger.next_print(),ledger.delay_print(uuid),
    ledger.begin_print(uuid,text),ledger.complete_print(uuid,text),
    ledger.print_staff_source(uuid,uuid,uuid,uuid),
    ledger.resolve_print(uuid,uuid,uuid,uuid,uuid,text,text) FROM PUBLIC;
GRANT USAGE ON SCHEMA ledger TO nu54_p05_print,nu54_p05_print_staff;
GRANT EXECUTE ON FUNCTION ledger.next_print(),ledger.delay_print(uuid),
    ledger.begin_print(uuid,text),ledger.complete_print(uuid,text) TO nu54_p05_print;
GRANT EXECUTE ON FUNCTION ledger.print_staff_source(uuid,uuid,uuid,uuid),
    ledger.resolve_print(uuid,uuid,uuid,uuid,uuid,text,text) TO nu54_p05_print_staff;

CREATE FUNCTION ledger.print_staff_attempts(tenant_ uuid,branch_ uuid,actor_ uuid,receipt_ uuid)
RETURNS TABLE(attempt_id uuid, state text, previous_id uuid)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $attempts$
    SELECT a.id,a.state,a.previous_id
    FROM ledger.pickup p JOIN ledger.fulfillment_outbox e ON e.sale_id=p.sale_id
    JOIN ledger.print_attempt a ON a.event_id=e.event_id
    JOIN ledger.branch b ON b.id=p.branch_id JOIN ledger.print_staff_grant g ON g.branch_id=b.id
    WHERE p.receipt_ref=receipt_ AND b.id=branch_ AND b.tenant_ref=tenant_ AND g.actor_id=actor_
      AND g.valid_until>clock_timestamp()
    ORDER BY a.created_at DESC,a.id DESC LIMIT 50
$attempts$;
REVOKE ALL ON FUNCTION ledger.print_staff_attempts(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.print_staff_attempts(uuid,uuid,uuid,uuid) TO nu54_p05_print_staff;

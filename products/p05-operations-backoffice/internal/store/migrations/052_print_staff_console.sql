-- A confirmed print can be the parent of one separately reviewed intentional copy.
ALTER TABLE ledger.print_decision DROP CONSTRAINT print_decision_attempt_id_key;
ALTER TABLE ledger.print_decision ADD UNIQUE(attempt_id,action);

-- Print authorizations are scoped to an individually enrolled branch employee.
CREATE FUNCTION ledger.print_staff_live(tenant_ uuid, branch_ uuid, actor_ uuid)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE expires_ timestamptz;
BEGIN
    SELECT g.valid_until INTO expires_ FROM ledger.print_staff_grant g
    JOIN ledger.branch b ON b.id=g.branch_id
    WHERE b.id=branch_ AND b.tenant_ref=tenant_ AND g.actor_id=actor_ FOR SHARE OF g;
    RETURN FOUND AND expires_>clock_timestamp();
END $$;
REVOKE ALL ON FUNCTION ledger.print_staff_live(uuid,uuid,uuid) FROM PUBLIC;

CREATE OR REPLACE FUNCTION ledger.request_authority_live(
    tenant_ uuid, branch_ uuid, actor_ uuid, role_ text, capabilities_ text[]
) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE ends_ timestamptz;
DECLARE windows_ tstzrange[];
BEGIN
    IF NOT EXISTS(SELECT FROM ledger.branch b WHERE b.id=branch_ AND b.tenant_ref=tenant_) THEN
        RETURN false;
    END IF;
    IF role_='printer_staff' THEN
        RETURN ledger.print_staff_live(tenant_,branch_,actor_);
    ELSIF role_='pos' THEN
        SELECT least(p.valid_until,g.valid_until) INTO ends_ FROM ledger.pos_source p
        JOIN ledger.commerce_grant g ON g.branch_id=p.branch_id AND g.actor_id=p.terminal_id
            AND g.role_code='terminal'
        WHERE p.branch_id=branch_ AND p.actor_id=actor_ FOR SHARE OF p,g;
        RETURN FOUND AND ends_>clock_timestamp();
    ELSIF role_ IN ('customer','terminal') THEN
        SELECT g.valid_until INTO ends_ FROM ledger.commerce_grant g
        WHERE g.branch_id=branch_ AND g.actor_id=actor_ AND g.role_code=role_ FOR SHARE;
        RETURN FOUND AND ends_>clock_timestamp();
    ELSIF role_='operator' AND cardinality(capabilities_)=0 THEN
        -- First lock all matching versions in a stable order. The next
        -- statement uses a new clock sample after every lock wait.
        SELECT array_agg(tstzrange(locked.valid_from,locked.revoked_at,'[)')) INTO windows_ FROM (
            SELECT g.valid_from,g.revoked_at FROM ledger.staff_grant g
            WHERE g.branch_id=branch_ AND g.subject_ref=actor_ AND g.role_code='operator'
            ORDER BY g.valid_from FOR SHARE
        ) locked;
        RETURN EXISTS(SELECT FROM unnest(windows_) AS windows(active) WHERE windows.active @> clock_timestamp());
    ELSIF role_ IN ('operator','finance') AND cardinality(capabilities_)>0 THEN
        SELECT max(locked.valid_until) INTO ends_ FROM (
            SELECT g.valid_until FROM ledger.finance_grant g
            WHERE g.branch_id=branch_ AND g.actor_id=actor_ AND g.capability=ANY(capabilities_)
            ORDER BY g.capability FOR SHARE
        ) locked;
        RETURN COALESCE(ends_>clock_timestamp(),false);
    END IF;
    RETURN false;
END $$;

CREATE OR REPLACE FUNCTION ledger.kiosk_staff_access(tenant_ uuid,branch_ uuid,actor_ uuid,role_ text)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE permissions text[] := ARRAY[]::text[];
BEGIN
    IF role_='terminal' THEN
        PERFORM ledger.kiosk_require_pickup(tenant_,branch_,actor_,role_);
        permissions := ARRAY['pickup'];
    ELSIF role_='operator' THEN
        IF ledger.request_authority_live(tenant_,branch_,actor_,role_,ARRAY[]::text[]) THEN
            permissions := array_append(permissions,'terminal_settings');
        END IF;
        IF ledger.request_authority_live(tenant_,branch_,actor_,role_,ARRAY['pickup_manage']) THEN
            permissions := array_append(permissions,'pickup');
        END IF;
        IF cardinality(permissions)>0 AND ledger.print_staff_live(tenant_,branch_,actor_) THEN
            permissions := array_append(permissions,'print_receipts');
        END IF;
    ELSIF role_='finance' AND ledger.request_authority_live(tenant_,branch_,actor_,role_,ARRAY['refund_request','refund_approve','refund_submit']) THEN
        permissions := ARRAY['refund_console'];
    END IF;
    IF cardinality(permissions)=0 THEN
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='staff scope denied';
    END IF;
    RETURN jsonb_build_object('branchRef',branch_,'actorRef',actor_,'permissions',permissions);
END $$;

CREATE OR REPLACE FUNCTION ledger.resolve_print(tenant_ uuid,branch_ uuid,actor_ uuid,attempt_ uuid,key_ uuid,action_ text,epoch_ text)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $resolve$
DECLARE
    prior ledger.print_attempt;
    decided ledger.print_decision;
    result_ uuid;
BEGIN
    IF NOT EXISTS (SELECT FROM ledger.print_staff_source(tenant_,branch_,actor_,attempt_)) THEN
        RAISE EXCEPTION 'print decision denied' USING ERRCODE='42501';
    END IF;
    -- An operator's idempotency key fences retries across different attempts.
    PERFORM pg_advisory_xact_lock(hashtextextended(branch_::text||actor_::text||key_::text,59));
    SELECT * INTO decided FROM ledger.print_decision WHERE branch_id=branch_ AND actor_id=actor_ AND request_key=key_;
    IF FOUND THEN
        IF NOT ledger.print_staff_live(tenant_,branch_,actor_) THEN
            RAISE EXCEPTION 'print decision denied' USING ERRCODE='42501';
        END IF;
        IF (decided.attempt_id,decided.action) IS DISTINCT FROM (attempt_,action_) THEN
            RAISE EXCEPTION 'print decision key conflict' USING ERRCODE='P1001';
        END IF;
        RETURN decided.result_id;
    END IF;
    SELECT * INTO STRICT prior FROM ledger.print_attempt WHERE id=attempt_ FOR UPDATE;
    IF NOT ledger.print_staff_live(tenant_,branch_,actor_) THEN
        RAISE EXCEPTION 'print decision denied' USING ERRCODE='42501';
    END IF;
    IF action_='confirm_printed' AND prior.state='unknown' THEN
        UPDATE ledger.print_attempt SET state='printed',finished_at=clock_timestamp() WHERE id=attempt_;
        result_:=attempt_;
    ELSIF action_='reprint' AND (prior.state IN ('unknown','printed') OR (prior.state='pending' AND prior.authorizing_epoch<>epoch_))
          AND NOT EXISTS (SELECT FROM ledger.print_attempt a WHERE a.previous_id=attempt_) THEN
        IF prior.state<>'printed' THEN
            UPDATE ledger.print_attempt SET state='superseded',finished_at=clock_timestamp() WHERE id=attempt_;
        END IF;
        INSERT INTO ledger.print_attempt(event_id,previous_id,authorizing_epoch) VALUES(prior.event_id,attempt_,epoch_) RETURNING id INTO result_;
    ELSE
        RAISE EXCEPTION 'print attempt cannot use that decision' USING ERRCODE='P1001';
    END IF;
    INSERT INTO ledger.print_decision(branch_id,actor_id,request_key,attempt_id,action,result_id)
      VALUES(branch_,actor_,key_,attempt_,action_,result_);
    RETURN result_;
END
$resolve$;


CREATE FUNCTION ledger.print_staff_queue(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE result jsonb;
BEGIN
    IF NOT ledger.print_staff_live(tenant_,branch_,actor_) THEN
        RAISE EXCEPTION 'print queue denied' USING ERRCODE='42501';
    END IF;
    SELECT coalesce(jsonb_agg(jsonb_build_object('saleRef',q.sale_id,'receiptRef',q.receipt_ref,
        'pickupNo',q.number) ORDER BY q.created_at DESC,q.sale_id),'[]'::jsonb) INTO result
    FROM (
        SELECT p.sale_id,p.receipt_ref,p.number,e.created_at
        FROM ledger.pickup p JOIN ledger.fulfillment_outbox e ON e.sale_id=p.sale_id AND e.kind='paid'
        JOIN ledger.print_attempt a ON a.event_id=e.event_id AND a.previous_id IS NULL
        WHERE p.branch_id=branch_ AND (sale_ IS NULL OR sale_=p.sale_id)
        ORDER BY e.created_at DESC,p.sale_id LIMIT 50
    ) q;
    RETURN result;
END $$;

CREATE FUNCTION ledger.print_staff_attempt_view(tenant_ uuid,branch_ uuid,actor_ uuid,receipt_ uuid)
RETURNS jsonb LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT coalesce(jsonb_agg(jsonb_strip_nulls(jsonb_build_object(
      'attemptId',a.attempt_id,'state',a.state,'previousId',a.previous_id)) ORDER BY a.ordinality),'[]'::jsonb)
    FROM ledger.print_staff_attempts(tenant_,branch_,actor_,receipt_) WITH ORDINALITY AS a;
$$;
REVOKE ALL ON FUNCTION ledger.print_staff_queue(uuid,uuid,uuid,uuid),
    ledger.print_staff_attempt_view(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.print_staff_queue(uuid,uuid,uuid,uuid),
    ledger.print_staff_attempt_view(uuid,uuid,uuid,uuid) TO nu54_p05_print_staff;
GRANT EXECUTE ON FUNCTION ledger.request_authority_live(uuid,uuid,uuid,text,text[]) TO nu54_p05_print_staff;

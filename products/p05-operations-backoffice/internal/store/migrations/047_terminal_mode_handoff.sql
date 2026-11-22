-- Menu-to-POS transitions freeze only new terminal orders. Existing checkout,
-- POS and command receipts keep their original IDs and authority.
CREATE TABLE ledger.terminal_transition (
    branch_id uuid NOT NULL,
    terminal_ref uuid NOT NULL,
    request_ref uuid NOT NULL,
    operator_ref uuid NOT NULL REFERENCES ledger.login_subject(subject_ref),
    source_revision bigint NOT NULL CHECK (source_revision BETWEEN 1 AND 9007199254740990),
    intent_digest bytea NOT NULL CHECK (octet_length(intent_digest)=32),
    requested_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    handoff_at timestamptz,
    handoff_by uuid REFERENCES ledger.login_subject(subject_ref),
    ack_at timestamptz,
    activated_at timestamptz,
    PRIMARY KEY (branch_id,terminal_ref,request_ref),
    UNIQUE (branch_id,operator_ref,request_ref),
    UNIQUE (branch_id,terminal_ref,source_revision),
    FOREIGN KEY (branch_id,terminal_ref) REFERENCES ledger.commerce_grant(branch_id,actor_id),
    CHECK ((handoff_at IS NULL)=(handoff_by IS NULL)),
    CHECK (activated_at IS NULL OR (handoff_at IS NOT NULL AND ack_at IS NOT NULL))
);
CREATE UNIQUE INDEX terminal_transition_one_waiting
    ON ledger.terminal_transition(branch_id,terminal_ref) WHERE activated_at IS NULL;

CREATE TABLE ledger.terminal_handoff_order (
    branch_id uuid NOT NULL,
    terminal_ref uuid NOT NULL,
    request_ref uuid NOT NULL,
    sale_id uuid NOT NULL,
    PRIMARY KEY (branch_id,terminal_ref,request_ref,sale_id),
    FOREIGN KEY (branch_id,terminal_ref,request_ref) REFERENCES ledger.terminal_transition,
    FOREIGN KEY (branch_id,sale_id) REFERENCES ledger.sale(branch_id,id)
);
REVOKE ALL ON ledger.terminal_transition, ledger.terminal_handoff_order FROM PUBLIC;

CREATE OR REPLACE FUNCTION ledger.require_terminal_operator(tenant uuid, branch uuid, actor uuid)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF NOT ledger.request_authority_live(tenant,branch,actor,'operator',ARRAY[]::text[]) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='terminal scope denied';
    END IF;
END $$;

CREATE FUNCTION ledger.reject_checkout_during_terminal_transition()
RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    -- Normal menu and registered POS reservations already have this grant locked.
    -- Lock it for any other checkout writer as well before testing the fence.
    PERFORM FROM ledger.commerce_grant g
        WHERE g.branch_id=NEW.branch_id AND g.actor_id=NEW.creator AND g.role_code='terminal'
        FOR SHARE;
    IF FOUND AND EXISTS (SELECT FROM ledger.terminal_transition t
        WHERE t.branch_id=NEW.branch_id AND t.terminal_ref=NEW.creator AND t.activated_at IS NULL) THEN
        RAISE EXCEPTION USING ERRCODE='P0703', MESSAGE='terminal transition awaiting handoff and acknowledgement';
    END IF;
    RETURN NEW;
END $$;

CREATE TRIGGER terminal_transition_checkout_fence BEFORE INSERT ON ledger.order_checkout
    FOR EACH ROW EXECUTE FUNCTION ledger.reject_checkout_during_terminal_transition();

CREATE FUNCTION ledger.terminal_transition_state(tenant_ uuid,branch_ uuid,actor_ uuid,terminal_ uuid,role_ text,request_ uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE live record;
DECLARE transition_ ledger.terminal_transition;
DECLARE count_ bigint;
DECLARE orders_ jsonb;
BEGIN
    SELECT * INTO STRICT live FROM ledger.terminal_mode_state(tenant_,branch_,actor_,terminal_,role_);
    SELECT t.* INTO transition_ FROM ledger.terminal_transition t
      WHERE t.branch_id=branch_ AND t.terminal_ref=terminal_
        AND ((request_ IS NOT NULL AND t.request_ref=request_) OR
             (request_ IS NULL AND (t.activated_at IS NULL OR
              (live.mode='pos' AND live.revision=t.source_revision+1))))
      ORDER BY t.source_revision DESC LIMIT 1;
    IF request_ IS NOT NULL AND NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='transition scope denied';
    END IF;
    IF role_='operator' THEN
        WITH owned_orders AS (
            SELECT h.sale_id FROM ledger.terminal_handoff_order h
              WHERE (h.branch_id,h.terminal_ref,h.request_ref)=(branch_,terminal_,transition_.request_ref)
            UNION ALL
            SELECT c.sale_id FROM ledger.order_checkout c JOIN ledger.sale s ON (s.branch_id,s.id)=(c.branch_id,c.sale_id)
              WHERE transition_.request_ref IS NULL AND c.branch_id=branch_ AND c.creator=terminal_ AND
                (s.state IN ('issued','checking') OR (s.state='preview' AND c.expires_at>clock_timestamp()))
        ), ordered AS (
            SELECT sale_id,row_number() OVER (ORDER BY sale_id) AS position FROM owned_orders
        )
        SELECT count(*),coalesce(jsonb_agg(sale_id ORDER BY sale_id) FILTER (WHERE position<=20),'[]'::jsonb)
          INTO count_,orders_ FROM ordered;
    END IF;
    RETURN jsonb_strip_nulls(jsonb_build_object('mode',live.mode,'revision',live.revision,
      'orderCount',count_,'orderRefs',orders_,
      'transition',CASE WHEN transition_.request_ref IS NOT NULL THEN jsonb_build_object(
        'requestRef',transition_.request_ref,'sourceRevision',transition_.source_revision,
        'targetRevision',transition_.source_revision+1,'targetMode','pos',
        'handoff',transition_.handoff_at IS NOT NULL,'ack',transition_.ack_at IS NOT NULL,
        'state',CASE WHEN transition_.activated_at IS NULL THEN 'waiting' ELSE 'active' END) END));
END $$;

ALTER FUNCTION ledger.change_terminal_mode(uuid,uuid,uuid,uuid,uuid,bigint,text,bytea)
    RENAME TO change_terminal_mode_before_handoff;
REVOKE EXECUTE ON FUNCTION ledger.change_terminal_mode_before_handoff(uuid,uuid,uuid,uuid,uuid,bigint,text,bytea)
    FROM nu54_p05_commerce;

CREATE OR REPLACE FUNCTION ledger.change_terminal_mode_before_handoff(tenant uuid, branch uuid, actor uuid, target uuid, request_key uuid,
    expected_revision bigint, new_mode text, intent bytea)
RETURNS TABLE(mode text, revision bigint) LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    prior ledger.terminal_mode_receipt;
    policy ledger.command_rule;
BEGIN
    PERFORM ledger.require_terminal_operator(tenant,branch,actor);
    IF expected_revision < 1 OR expected_revision >= 9223372036854775807 OR new_mode NOT IN ('menu','pos')
        OR intent IS NULL OR octet_length(intent)<>32 OR request_key IS NULL THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid terminal mode update';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||actor::text||'terminal_mode'||request_key::text,0));
    SELECT * INTO prior FROM ledger.terminal_mode_receipt r WHERE r.branch_id=branch AND r.operator_ref=actor
      AND r.request_key=change_terminal_mode_before_handoff.request_key;
    IF FOUND THEN
        IF prior.intent_digest<>intent OR prior.target_ref<>target THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='request conflict';
        END IF;
        IF prior.retire_after<=statement_timestamp() THEN
            RAISE EXCEPTION USING ERRCODE='P0502', MESSAGE='request expired';
        END IF;
        RETURN QUERY SELECT new_mode,expected_revision+1;
        RETURN;
    END IF;
    SELECT * INTO policy FROM ledger.command_rule c WHERE c.branch_id=branch FOR SHARE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0503', MESSAGE='command policy unavailable'; END IF;
    UPDATE ledger.commerce_grant g SET terminal_mode=new_mode,mode_revision=expected_revision+1
      FROM ledger.branch b WHERE b.id=g.branch_id AND b.tenant_ref=tenant AND b.id=branch
      AND g.actor_id=target AND g.role_code='terminal' AND g.valid_until>statement_timestamp() AND g.mode_revision=expected_revision;
    IF NOT FOUND THEN
        -- Deny a missing/expired enrollment without disclosing its existence.
        PERFORM ledger.terminal_mode_state(tenant,branch,actor,target,'operator');
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='mode revision changed';
    END IF;
    INSERT INTO ledger.terminal_mode_receipt VALUES (branch,actor,request_key,target,intent,
      statement_timestamp()+policy.receipt_window);
    RETURN QUERY SELECT new_mode,expected_revision+1;
END $$;

CREATE FUNCTION ledger.request_terminal_transition(tenant_ uuid,branch_ uuid,actor_ uuid,terminal_ uuid,
    request_ uuid,revision_ bigint,mode_ text,intent_ bytea)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE live ledger.commerce_grant;
DECLARE prior ledger.terminal_transition;
DECLARE legacy ledger.terminal_mode_receipt;
DECLARE result_ record;
BEGIN
    PERFORM ledger.require_terminal_operator(tenant_,branch_,actor_);
    IF revision_ IS NULL OR revision_ NOT BETWEEN 1 AND 9007199254740990 OR
       mode_ IS NULL OR mode_ NOT IN ('menu','pos') OR request_ IS NULL OR
       intent_ IS NULL OR octet_length(intent_)<>32 THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid terminal transition';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch_::text||actor_::text||'terminal_mode'||request_::text,0));
    SELECT g.* INTO live FROM ledger.commerce_grant g JOIN ledger.branch b ON b.id=g.branch_id
      WHERE g.branch_id=branch_ AND g.actor_id=terminal_ AND g.role_code='terminal'
        AND b.tenant_ref=tenant_ FOR UPDATE OF g;
    IF NOT FOUND OR live.valid_until<=clock_timestamp() THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='terminal scope denied';
    END IF;
    SELECT t.* INTO prior FROM ledger.terminal_transition t
      WHERE (t.branch_id,t.operator_ref,t.request_ref)=(branch_,actor_,request_);
    IF FOUND THEN
        IF (prior.terminal_ref,prior.source_revision,prior.intent_digest) IS DISTINCT FROM (terminal_,revision_,intent_) OR mode_<>'pos' THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='transition request conflict';
        END IF;
        RETURN ledger.terminal_transition_state(tenant_,branch_,actor_,terminal_,'operator',request_);
    END IF;
    SELECT r.* INTO legacy FROM ledger.terminal_mode_receipt r
      WHERE (r.branch_id,r.operator_ref,r.request_key)=(branch_,actor_,request_);
    IF FOUND OR mode_='menu' THEN
        IF legacy.request_key IS NULL AND EXISTS (SELECT FROM ledger.terminal_transition t
          WHERE (t.branch_id,t.terminal_ref)=(branch_,terminal_) AND t.activated_at IS NULL) THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='transition already pending';
        END IF;
        SELECT * INTO STRICT result_ FROM ledger.change_terminal_mode_before_handoff(
          tenant_,branch_,actor_,terminal_,request_,revision_,mode_,intent_);
        RETURN jsonb_build_object('mode',result_.mode,'revision',result_.revision);
    END IF;
    IF live.mode_revision<>revision_ OR live.terminal_mode<>'menu' OR EXISTS (
      SELECT FROM ledger.terminal_transition t WHERE (t.branch_id,t.terminal_ref)=(branch_,terminal_) AND t.activated_at IS NULL) THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='mode revision changed or transition already pending';
    END IF;
    PERFORM FROM ledger.command_rule c WHERE c.branch_id=branch_ FOR SHARE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0503', MESSAGE='command policy unavailable'; END IF;
    INSERT INTO ledger.terminal_transition(branch_id,terminal_ref,request_ref,operator_ref,source_revision,intent_digest)
      VALUES (branch_,terminal_,request_,actor_,revision_,intent_);
    INSERT INTO ledger.terminal_handoff_order
      SELECT branch_,terminal_,request_,c.sale_id FROM ledger.order_checkout c
      JOIN ledger.sale s ON (s.branch_id,s.id)=(c.branch_id,c.sale_id)
      WHERE (c.branch_id,c.creator)=(branch_,terminal_) AND
        (s.state IN ('issued','checking') OR (s.state='preview' AND c.expires_at>clock_timestamp()));
    RETURN ledger.terminal_transition_state(tenant_,branch_,actor_,terminal_,'operator',request_);
END $$;

-- Preserve SQL callers of the original signature; a POS request now returns live state until both gates pass.
CREATE FUNCTION ledger.change_terminal_mode(tenant uuid,branch uuid,actor uuid,target uuid,request_key uuid,
    expected_revision bigint,new_mode text,intent bytea)
RETURNS TABLE(mode text,revision bigint) LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT value->>'mode',(value->>'revision')::bigint
    FROM ledger.request_terminal_transition(tenant,branch,actor,target,request_key,expected_revision,new_mode,intent) value;
$$;

CREATE FUNCTION ledger.confirm_terminal_transition(tenant_ uuid,branch_ uuid,actor_ uuid,terminal_ uuid,role_ text,
    request_ uuid,version_ bigint)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE live ledger.commerce_grant;
DECLARE transition_ ledger.terminal_transition;
BEGIN
    IF role_='operator' THEN
        PERFORM ledger.require_terminal_operator(tenant_,branch_,actor_);
    ELSIF role_ IS DISTINCT FROM 'terminal' OR actor_ IS DISTINCT FROM terminal_ THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='transition scope denied';
    END IF;
    SELECT g.* INTO live FROM ledger.commerce_grant g JOIN ledger.branch b ON b.id=g.branch_id
      WHERE g.branch_id=branch_ AND g.actor_id=terminal_ AND g.role_code='terminal'
        AND b.tenant_ref=tenant_ FOR UPDATE OF g;
    IF NOT FOUND OR live.valid_until<=clock_timestamp() THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='terminal scope denied';
    END IF;
    SELECT t.* INTO transition_ FROM ledger.terminal_transition t
      WHERE (t.branch_id,t.terminal_ref,t.request_ref)=(branch_,terminal_,request_);
    IF NOT FOUND OR version_ IS DISTINCT FROM transition_.source_revision+1 THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='transition scope denied';
    END IF;
    IF transition_.activated_at IS NULL THEN
        IF (live.terminal_mode,live.mode_revision) IS DISTINCT FROM ('menu'::text,transition_.source_revision) THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='mode revision changed';
        END IF;
        UPDATE ledger.terminal_transition t SET
          handoff_at=CASE WHEN role_='operator' THEN coalesce(t.handoff_at,clock_timestamp()) ELSE t.handoff_at END,
          handoff_by=CASE WHEN role_='operator' THEN coalesce(t.handoff_by,actor_) ELSE t.handoff_by END,
          ack_at=CASE WHEN role_='terminal' THEN coalesce(t.ack_at,clock_timestamp()) ELSE t.ack_at END
          WHERE (t.branch_id,t.terminal_ref,t.request_ref)=(branch_,terminal_,request_) RETURNING * INTO transition_;
        IF transition_.handoff_at IS NOT NULL AND transition_.ack_at IS NOT NULL THEN
            UPDATE ledger.commerce_grant SET terminal_mode='pos',mode_revision=version_
              WHERE (branch_id,actor_id)=(branch_,terminal_);
            UPDATE ledger.terminal_transition t SET activated_at=clock_timestamp()
              WHERE (t.branch_id,t.terminal_ref,t.request_ref)=(branch_,terminal_,request_);
        END IF;
    END IF;
    RETURN ledger.terminal_transition_state(tenant_,branch_,actor_,terminal_,role_,request_);
END $$;

REVOKE ALL ON FUNCTION ledger.reject_checkout_during_terminal_transition(),
    ledger.terminal_transition_state(uuid,uuid,uuid,uuid,text,uuid),
    ledger.request_terminal_transition(uuid,uuid,uuid,uuid,uuid,bigint,text,bytea),
    ledger.change_terminal_mode(uuid,uuid,uuid,uuid,uuid,bigint,text,bytea),
    ledger.confirm_terminal_transition(uuid,uuid,uuid,uuid,text,uuid,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.terminal_transition_state(uuid,uuid,uuid,uuid,text,uuid),
    ledger.request_terminal_transition(uuid,uuid,uuid,uuid,uuid,bigint,text,bytea),
    ledger.change_terminal_mode(uuid,uuid,uuid,uuid,uuid,bigint,text,bytea),
    ledger.confirm_terminal_transition(uuid,uuid,uuid,uuid,text,uuid,bigint) TO nu54_p05_commerce;

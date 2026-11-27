ALTER TABLE ledger.finance_grant DROP CONSTRAINT finance_grant_capability_check;
ALTER TABLE ledger.finance_grant ADD CONSTRAINT finance_grant_capability_check CHECK(capability IN (
    'refund_request','refund_approve','day_close','report_read','report_export','reconcile_read','reconcile_retry',
    'payout_propose','payout_approve','payout_notice','rental_manage','firmware_manage','firmware_station',
    'experience_read','experience_cancel','access_manage','audit_read'
));

CREATE TABLE ledger.operator_access_change(
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    key uuid NOT NULL,
    subject uuid NOT NULL REFERENCES ledger.login_subject(subject_ref),
    role_code text NOT NULL CHECK(role_code IN ('operator','terminal')),
    operation text NOT NULL CHECK(operation IN ('register','revoke')),
    proposer uuid NOT NULL REFERENCES ledger.login_subject(subject_ref),
    approver uuid REFERENCES ledger.login_subject(subject_ref),
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    applied_at timestamptz,
    acknowledged_at timestamptz,
    PRIMARY KEY(branch_id,key),
    CHECK(subject<>proposer AND (approver IS NULL OR (approver<>subject AND approver<>proposer))),
    CHECK((approver IS NULL)=(applied_at IS NULL))
);
REVOKE ALL ON ledger.operator_access_change FROM PUBLIC;

CREATE FUNCTION ledger.operator_access_status(tenant uuid,branch uuid,actor uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE response jsonb;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'access_manage');
    SELECT coalesce(jsonb_agg(item.payload ORDER BY item.recorded_at DESC,item.key),'[]'::jsonb) INTO response FROM (
       SELECT c.key,c.recorded_at,jsonb_build_object('requestKey',c.key,'target',c.subject,'role',c.role_code,
          'operation',c.operation,'appliedAt',c.applied_at,'acknowledgedAt',c.acknowledged_at,
          'canApprove',c.applied_at IS NULL AND c.proposer<>actor AND c.subject<>actor,
          'openOrders',(SELECT count(*) FROM ledger.order_checkout x JOIN ledger.sale s ON s.id=x.sale_id
              WHERE x.branch_id=branch AND x.creator=c.subject AND s.state IN ('preview','issued','checking'))) AS payload
       FROM ledger.operator_access_change c WHERE c.branch_id=branch ORDER BY c.recorded_at DESC,c.key LIMIT 100
    ) item;
    RETURN response;
END $$;

CREATE FUNCTION ledger.propose_operator_access(tenant uuid,branch uuid,actor uuid,key_ uuid,target_ uuid,role_ text,op_ text)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE prior ledger.operator_access_change;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'access_manage');
    IF actor=target_ OR NOT EXISTS(SELECT FROM ledger.login_subject x WHERE x.subject_ref=target_) THEN
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='enrolled target required';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||key_::text,31));
    SELECT * INTO prior FROM ledger.operator_access_change c WHERE c.branch_id=branch AND c.key=key_;
    IF FOUND THEN
        IF (prior.subject,prior.role_code,prior.operation,prior.proposer) IS DISTINCT FROM (target_,role_,op_,actor) THEN
            RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='access request key changed';
        END IF;
    ELSE
        INSERT INTO ledger.operator_access_change(branch_id,key,subject,role_code,operation,proposer)
          VALUES(branch,key_,target_,role_,op_,actor);
    END IF;
    RETURN ledger.operator_access_status(tenant,branch,actor);
END $$;

CREATE FUNCTION ledger.apply_operator_access(tenant uuid,branch uuid,actor uuid,key_ uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE item ledger.operator_access_change;
DECLARE moment timestamptz;
DECLARE current_reviewer uuid;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'access_manage');
    SELECT * INTO item FROM ledger.operator_access_change c WHERE c.branch_id=branch AND c.key=key_ FOR UPDATE;
    IF NOT FOUND OR actor IN (item.subject,item.proposer) THEN
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='independent access approval required';
    END IF;
    IF NOT EXISTS(SELECT FROM ledger.finance_grant g WHERE g.branch_id=branch AND g.actor_id=actor
      AND g.capability='access_manage' AND g.valid_until>clock_timestamp()) THEN
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='access reviewer expired';
    END IF;
    IF item.applied_at IS NOT NULL THEN
        IF item.approver<>actor THEN RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='access already applied'; END IF;
        RETURN ledger.operator_access_status(tenant,branch,actor);
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||item.subject::text||item.role_code,32));
    -- Recheck both grants after waiting for request and target locks. Wall
    -- clock expiry and concurrent revocation must win over a stale session.
    FOR current_reviewer IN SELECT g.actor_id FROM ledger.finance_grant g
        WHERE g.branch_id=branch AND g.actor_id IN (actor,item.proposer)
          AND g.capability='access_manage'
        ORDER BY g.actor_id FOR SHARE
    LOOP
        IF NOT EXISTS(SELECT FROM ledger.finance_grant g WHERE g.branch_id=branch AND g.actor_id=current_reviewer
             AND g.capability='access_manage' AND g.valid_until>clock_timestamp()) THEN
            RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='access approval credential expired';
        END IF;
    END LOOP;
    IF 2<>(SELECT count(*) FROM ledger.finance_grant g WHERE g.branch_id=branch AND g.actor_id IN(actor,item.proposer)
       AND g.capability='access_manage' AND g.valid_until>clock_timestamp()) THEN
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='two active access reviewers required';
    END IF;
    moment:=clock_timestamp();
    -- Never use an older still-open proposal to undo a newer approved decision.
    IF EXISTS(SELECT FROM ledger.operator_access_change c
       WHERE c.branch_id=branch AND c.subject=item.subject AND c.role_code=item.role_code AND c.applied_at>item.recorded_at) THEN
        RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='access changed since review';
    END IF;
    IF item.role_code='terminal' THEN
        IF item.operation='register' THEN
            INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,terminal_mode,valid_until)
            VALUES(branch,item.subject,'terminal','pos',moment+interval '30 days')
            ON CONFLICT(branch_id,actor_id) DO UPDATE SET valid_until=EXCLUDED.valid_until
              WHERE ledger.commerce_grant.role_code='terminal';
        ELSE
            UPDATE ledger.commerce_grant g SET valid_until=moment
              WHERE g.branch_id=branch AND g.actor_id=item.subject AND g.role_code='terminal';
        END IF;
    ELSIF item.operation='register' THEN
        IF NOT EXISTS(SELECT FROM ledger.staff_grant s WHERE s.branch_id=branch AND s.subject_ref=item.subject
             AND s.role_code='operator' AND s.revoked_at IS NULL) THEN
            INSERT INTO ledger.staff_grant VALUES(branch,item.subject,'operator',moment,NULL);
        END IF;
    ELSE
        UPDATE ledger.staff_grant s SET revoked_at=moment
          WHERE s.branch_id=branch AND s.subject_ref=item.subject AND s.revoked_at IS NULL AND s.valid_from<=moment;
        UPDATE ledger.finance_grant g SET valid_until=moment
          WHERE g.branch_id=branch AND g.actor_id=item.subject;
    END IF;
    UPDATE ledger.operator_access_change c SET approver=actor,applied_at=moment WHERE c.branch_id=branch AND c.key=key_;
    RETURN ledger.operator_access_status(tenant,branch,actor);
END $$;

CREATE FUNCTION ledger.record_operator_access_ack(tenant uuid,branch uuid,actor uuid)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF (ledger.commerce_allowed(tenant,branch,actor)).role_code<>'terminal' THEN RETURN; END IF;
    UPDATE ledger.operator_access_change c SET acknowledged_at=clock_timestamp()
    WHERE c.branch_id=branch AND c.subject=actor AND c.role_code='terminal' AND c.operation='register'
      AND c.applied_at IS NOT NULL AND c.acknowledged_at IS NULL
      AND NOT EXISTS(SELECT FROM ledger.operator_access_change newer
        WHERE newer.branch_id=branch AND newer.subject=actor AND newer.role_code='terminal' AND newer.applied_at>c.applied_at);
END $$;

REVOKE ALL ON FUNCTION ledger.operator_access_status(uuid,uuid,uuid),
    ledger.propose_operator_access(uuid,uuid,uuid,uuid,uuid,text,text),
    ledger.apply_operator_access(uuid,uuid,uuid,uuid),ledger.record_operator_access_ack(uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.operator_access_status(uuid,uuid,uuid),
    ledger.propose_operator_access(uuid,uuid,uuid,uuid,uuid,text,text),
    ledger.apply_operator_access(uuid,uuid,uuid,uuid),ledger.record_operator_access_ack(uuid,uuid,uuid) TO nu54_p05_commerce;

CREATE FUNCTION ledger.operator_audit(tenant uuid,branch uuid,actor uuid,from_ date,until_ date)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE site ledger.branch;
DECLARE report jsonb;
DECLARE first_ timestamptz;
DECLARE last_ timestamptz;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'audit_read');
    IF until_<from_ OR until_-from_>31 THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid audit window';
    END IF;
    SELECT b.* INTO STRICT site FROM ledger.branch b WHERE b.id=branch;
    first_:=from_::timestamp AT TIME ZONE site.timezone;
    last_:=(until_+1)::timestamp AT TIME ZONE site.timezone;
    SELECT coalesce(jsonb_agg(jsonb_build_object('kind',event.kind,'at',event.at,'reference',event.ref,
      'actor',event.actor,'before',event.previous,'after',event.latest) ORDER BY event.at DESC,event.ref),'[]'::jsonb)
      INTO report FROM (
       SELECT * FROM (
         SELECT 'catalog'::text AS kind, m.published_at AS at,o.request_key::text AS ref,o.actor_ref::text AS actor,
            (m.version-1)::text AS previous,m.version::text AS latest
         FROM ledger.menu_release m LEFT JOIN ledger.operator_catalog_receipt o ON o.branch_id=m.branch_id
            AND o.operation='publish' AND (o.result->>'version')::bigint=m.version
         WHERE m.branch_id=branch
         UNION ALL
         SELECT 'access',coalesce(c.applied_at,c.recorded_at),c.key::text,coalesce(c.approver,c.proposer)::text,
            c.role_code,c.operation FROM ledger.operator_access_change c WHERE c.branch_id=branch
         UNION ALL
         SELECT 'fulfillment-retry',r.accepted_at,r.event_id::text,r.actor_id::text,'pending','retry requested'
            FROM ledger.finance_fulfillment_retry r WHERE r.branch_id=branch
       ) source WHERE source.at>=first_ AND source.at<last_ ORDER BY source.at DESC,source.ref LIMIT 100
    ) event;
    RETURN jsonb_build_object('timezone',site.timezone,'events',report);
END $$;
REVOKE ALL ON FUNCTION ledger.operator_audit(uuid,uuid,uuid,date,date) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.operator_audit(uuid,uuid,uuid,date,date) TO nu54_p05_commerce;

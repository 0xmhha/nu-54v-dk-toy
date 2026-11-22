ALTER TABLE ledger.operator_access_change ADD COLUMN handoff_to uuid REFERENCES ledger.login_subject(subject_ref),
    ADD COLUMN review_digest text CHECK(review_digest ~ '^[0-9a-f]{64}$'),
    ADD CHECK(handoff_to IS NULL OR (operation='revoke' AND handoff_to<>subject AND review_digest IS NOT NULL));

CREATE TABLE ledger.operator_access_handoff(
    branch_id uuid NOT NULL,
    access_key uuid NOT NULL,
    sale_id uuid NOT NULL,
    accepted_at timestamptz CHECK(isfinite(accepted_at)),
    PRIMARY KEY(branch_id,access_key,sale_id),
    FOREIGN KEY(branch_id,access_key) REFERENCES ledger.operator_access_change,
    FOREIGN KEY(branch_id,sale_id) REFERENCES ledger.sale(branch_id,id)
);
CREATE INDEX operator_access_handoff_sale ON ledger.operator_access_handoff(branch_id,sale_id);
REVOKE ALL ON ledger.operator_access_handoff FROM PUBLIC;

CREATE FUNCTION ledger.access_handoff_staff(tenant_ uuid,branch_ uuid,staff_ uuid)
RETURNS boolean LANGUAGE sql SET search_path=pg_catalog,pg_temp AS $$
    SELECT ledger.request_authority_live(tenant_,branch_,staff_,'operator',ARRAY[]::text[])
       AND ledger.request_authority_live(tenant_,branch_,staff_,'operator',ARRAY['pickup_manage'])
$$;

CREATE FUNCTION ledger.access_revoke_orders(branch_ uuid,subject_ uuid)
RETURNS uuid[] LANGUAGE sql SET search_path=pg_catalog,pg_temp AS $$
    SELECT coalesce(array_agg(s.id ORDER BY s.id),ARRAY[]::uuid[])
    FROM ledger.order_checkout x JOIN ledger.sale s ON (s.branch_id,s.id)=(x.branch_id,x.sale_id)
    LEFT JOIN ledger.travel_pickup_progress p ON p.sale_id=s.id
    LEFT JOIN LATERAL (
        SELECT c.handoff_to FROM ledger.operator_access_handoff h JOIN ledger.operator_access_change c
          ON (c.branch_id,c.key)=(h.branch_id,h.access_key)
        WHERE h.branch_id=s.branch_id AND h.sale_id=s.id AND c.applied_at IS NOT NULL
        ORDER BY c.applied_at DESC,c.key DESC LIMIT 1
    ) assigned ON true
    LEFT JOIN LATERAL (
        SELECT a.actor_id FROM ledger.travel_pickup_transition a WHERE a.sale_id=s.id
        ORDER BY a.revision DESC LIMIT 1
    ) last_pickup ON assigned.handoff_to IS NULL AND s.state='paid'
    WHERE s.branch_id=branch_
      AND (assigned.handoff_to=subject_ OR
          (assigned.handoff_to IS NULL AND (x.creator=subject_ OR last_pickup.actor_id=subject_)))
      AND (s.state IN ('issued','checking') OR (s.state='preview' AND x.expires_at>clock_timestamp())
        OR (s.state='paid' AND (
            EXISTS (SELECT FROM ledger.fulfillment_outbox o WHERE o.sale_id=s.id AND o.kind='paid' AND o.state<>'delivered')
            OR (coalesce(p.state,'accepted') NOT IN ('collected','unfulfillable') AND
                (ledger.kiosk_menu_pickup(s.id) OR EXISTS (SELECT FROM ledger.travel_pickup_order r WHERE r.sale_id=s.id)))
        )));
$$;

CREATE FUNCTION ledger.access_target_live(tenant_ uuid,branch_ uuid,target_ uuid,role_ text)
RETURNS boolean LANGUAGE sql SET search_path=pg_catalog,pg_temp AS $$
    SELECT CASE role_
      WHEN 'terminal' THEN ledger.request_authority_live(tenant_,branch_,target_,'terminal',ARRAY[]::text[])
      WHEN 'operator' THEN ledger.request_authority_live(tenant_,branch_,target_,'operator',ARRAY[]::text[])
        OR EXISTS (SELECT FROM ledger.finance_grant g WHERE g.branch_id=branch_ AND g.actor_id=target_ AND g.valid_until>clock_timestamp())
      ELSE false END
$$;

CREATE FUNCTION ledger.access_revoke_digest(branch_ uuid,target_ uuid,role_ text,handoff_ uuid,orders_ uuid[])
RETURNS text LANGUAGE sql IMMUTABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT encode(sha256(convert_to(jsonb_build_array('access-revoke-v1',branch_,target_,role_,handoff_,orders_)::text,'UTF8')),'hex')
$$;

CREATE FUNCTION ledger.operator_access_revoke_review(tenant_ uuid,branch_ uuid,actor_ uuid,target_ uuid,role_ text,handoff_ uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE orders_ uuid[];
BEGIN
    PERFORM ledger.require_finance(tenant_,branch_,actor_,'access_manage');
    IF actor_=target_ OR NOT ledger.access_target_live(tenant_,branch_,target_,role_) OR
       (handoff_ IS NOT NULL AND (handoff_=target_ OR NOT ledger.access_handoff_staff(tenant_,branch_,handoff_))) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='revocation scope denied';
    END IF;
    PERFORM ledger.require_finance(tenant_,branch_,actor_,'access_manage');
    orders_:=ledger.access_revoke_orders(branch_,target_);
    RETURN jsonb_build_object('target',target_,'role',role_,'handoffTo',handoff_,'orderRefs',orders_,
        'reviewKey',ledger.access_revoke_digest(branch_,target_,role_,handoff_,orders_));
END $$;

CREATE OR REPLACE FUNCTION ledger.operator_access_status(tenant uuid,branch uuid,actor uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE response jsonb;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'access_manage');
    SELECT coalesce(jsonb_agg(item.payload ORDER BY item.recorded_at DESC,item.key),'[]'::jsonb) INTO response FROM (
       SELECT c.key,c.recorded_at,jsonb_build_object('requestKey',c.key,'target',c.subject,'role',c.role_code,
          'operation',c.operation,'appliedAt',c.applied_at,'acknowledgedAt',c.acknowledged_at,
          'canApprove',c.applied_at IS NULL AND c.proposer<>actor AND c.subject<>actor,
          'handoffTo',c.handoff_to,
          'openOrders',CASE WHEN c.operation='revoke' THEN (SELECT count(*) FROM ledger.operator_access_handoff h
             WHERE h.branch_id=branch AND h.access_key=c.key)
          ELSE (SELECT count(*) FROM ledger.order_checkout x JOIN ledger.sale s ON s.id=x.sale_id
              WHERE x.branch_id=branch AND x.creator=c.subject AND s.state IN ('preview','issued','checking')) END,
          'orderRefs',ARRAY(SELECT h.sale_id FROM ledger.operator_access_handoff h
             WHERE h.branch_id=branch AND h.access_key=c.key ORDER BY h.sale_id)) AS payload
       FROM ledger.operator_access_change c WHERE c.branch_id=branch ORDER BY c.recorded_at DESC,c.key LIMIT 100
    ) item;
    RETURN response;
END $$;

CREATE FUNCTION ledger.propose_operator_access(tenant uuid,branch uuid,actor uuid,key_ uuid,target_ uuid,role_ text,op_ text,handoff_ uuid,review_ text)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE prior ledger.operator_access_change;
DECLARE orders_ uuid[]:=ARRAY[]::uuid[];
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'access_manage');
    IF actor=target_ OR NOT EXISTS(SELECT FROM ledger.login_subject x WHERE x.subject_ref=target_) THEN
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='enrolled target required';
    END IF;
    -- Access changes in one branch take shared participant locks in one order.
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text,62));
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||key_::text,31));
    PERFORM ledger.require_finance(tenant,branch,actor,'access_manage');
    SELECT * INTO prior FROM ledger.operator_access_change c WHERE c.branch_id=branch AND c.key=key_;
    IF FOUND THEN
        IF (prior.subject,prior.role_code,prior.operation,prior.proposer,prior.handoff_to,prior.review_digest)
            IS DISTINCT FROM (target_,role_,op_,actor,handoff_,review_) THEN
            RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='access request key changed';
        END IF;
    ELSE
        IF op_='revoke' THEN
            IF NOT ledger.access_target_live(tenant,branch,target_,role_) OR
               (handoff_ IS NOT NULL AND (handoff_=target_ OR NOT ledger.access_handoff_staff(tenant,branch,handoff_))) THEN
                RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='revocation scope denied';
            END IF;
            orders_:=ledger.access_revoke_orders(branch,target_);
            IF (cardinality(orders_)>0 AND (handoff_ IS NULL OR review_ IS NULL)) OR
               (review_ IS NOT NULL AND review_<>ledger.access_revoke_digest(branch,target_,role_,handoff_,orders_)) THEN
                RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='revocation impact changed';
            END IF;
        ELSIF handoff_ IS NOT NULL OR review_ IS NOT NULL THEN
            RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='handoff requires revocation';
        END IF;
        PERFORM ledger.require_finance(tenant,branch,actor,'access_manage');
        INSERT INTO ledger.operator_access_change(branch_id,key,subject,role_code,operation,proposer,handoff_to,review_digest)
          VALUES(branch,key_,target_,role_,op_,actor,handoff_,review_);
        INSERT INTO ledger.operator_access_handoff(branch_id,access_key,sale_id)
          SELECT branch,key_,unlisted.sale FROM unnest(orders_) AS unlisted(sale);
    END IF;
    RETURN ledger.operator_access_status(tenant,branch,actor);
END $$;

CREATE OR REPLACE FUNCTION ledger.apply_operator_access(tenant uuid,branch uuid,actor uuid,key_ uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE item ledger.operator_access_change;
DECLARE moment timestamptz;
DECLARE current_reviewer uuid;
DECLARE orders_ uuid[];
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'access_manage');
    -- Access changes in one branch take shared participant locks in one order.
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text,62));
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
    IF item.operation='revoke' THEN
        IF item.role_code='terminal' THEN
            PERFORM FROM ledger.commerce_grant g WHERE g.branch_id=branch AND g.actor_id=item.subject
                AND g.role_code='terminal' FOR UPDATE;
        ELSE
            PERFORM FROM ledger.staff_grant g WHERE g.branch_id=branch AND g.subject_ref=item.subject
                ORDER BY g.valid_from FOR UPDATE;
            PERFORM FROM ledger.finance_grant g WHERE g.branch_id=branch AND g.actor_id=item.subject
                ORDER BY g.capability FOR UPDATE;
        END IF;
        IF item.handoff_to IS NOT NULL AND NOT ledger.access_handoff_staff(tenant,branch,item.handoff_to) THEN
            RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='handoff staff no longer authorized';
        END IF;
        orders_:=ledger.access_revoke_orders(branch,item.subject);
        IF orders_ IS DISTINCT FROM ARRAY(SELECT h.sale_id FROM ledger.operator_access_handoff h
            WHERE h.branch_id=branch AND h.access_key=key_ ORDER BY h.sale_id) THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='revocation impact changed';
        END IF;
        -- Target and recipient locks can wait after the first grant check.
        PERFORM ledger.require_finance(tenant,branch,actor,'access_manage');
        PERFORM ledger.require_finance(tenant,branch,item.proposer,'access_manage');
        moment:=clock_timestamp();
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

CREATE OR REPLACE FUNCTION ledger.propose_operator_access(tenant uuid,branch uuid,actor uuid,key_ uuid,target_ uuid,role_ text,op_ text)
RETURNS jsonb LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT ledger.propose_operator_access(tenant,branch,actor,key_,target_,role_,op_,NULL,NULL)
$$;

CREATE FUNCTION ledger.operator_access_handoffs(tenant_ uuid,branch_ uuid,actor_ uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE response jsonb;
BEGIN
    IF NOT ledger.access_handoff_staff(tenant_,branch_,actor_) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='handoff scope denied';
    END IF;
    SELECT coalesce(jsonb_agg(jsonb_build_object('requestKey',x.access_key,'saleId',x.sale_id,
        'orderState',s.state,'acceptedAt',x.accepted_at) ORDER BY x.accepted_at NULLS FIRST,x.applied_at DESC,x.sale_id),'[]'::jsonb)
    INTO response FROM (
        SELECT DISTINCT ON(h.sale_id) h.access_key,h.sale_id,h.accepted_at,c.handoff_to,c.applied_at
        FROM ledger.operator_access_handoff h JOIN ledger.operator_access_change c
          ON (c.branch_id,c.key)=(h.branch_id,h.access_key)
        WHERE h.branch_id=branch_ AND c.applied_at IS NOT NULL
        ORDER BY h.sale_id,c.applied_at DESC,c.key DESC
    ) x JOIN ledger.sale s ON (s.branch_id,s.id)=(branch_,x.sale_id)
    WHERE x.handoff_to=actor_;
    RETURN response;
END $$;

CREATE FUNCTION ledger.accept_operator_access_handoff(tenant_ uuid,branch_ uuid,actor_ uuid,key_ uuid,sale_ uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(hashtextextended(branch_::text,62));
    IF NOT ledger.access_handoff_staff(tenant_,branch_,actor_) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='handoff scope denied';
    END IF;
    UPDATE ledger.operator_access_handoff h SET accepted_at=coalesce(h.accepted_at,clock_timestamp())
    FROM ledger.operator_access_change c
    WHERE (h.branch_id,h.access_key,h.sale_id)=(branch_,key_,sale_)
      AND (c.branch_id,c.key)=(h.branch_id,h.access_key) AND c.handoff_to=actor_ AND c.applied_at IS NOT NULL
      AND NOT EXISTS (SELECT FROM ledger.operator_access_handoff next
        JOIN ledger.operator_access_change newer ON (newer.branch_id,newer.key)=(next.branch_id,next.access_key)
        WHERE (next.branch_id,next.sale_id)=(h.branch_id,h.sale_id) AND (newer.applied_at,newer.key)>(c.applied_at,c.key));
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='handoff scope denied'; END IF;
    RETURN ledger.operator_access_handoffs(tenant_,branch_,actor_);
END $$;

REVOKE ALL ON FUNCTION ledger.access_handoff_staff(uuid,uuid,uuid),ledger.access_revoke_orders(uuid,uuid),
    ledger.access_target_live(uuid,uuid,uuid,text),ledger.access_revoke_digest(uuid,uuid,text,uuid,uuid[]),
    ledger.operator_access_revoke_review(uuid,uuid,uuid,uuid,text,uuid),
    ledger.propose_operator_access(uuid,uuid,uuid,uuid,uuid,text,text,uuid,text),
    ledger.operator_access_handoffs(uuid,uuid,uuid),ledger.accept_operator_access_handoff(uuid,uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.operator_access_revoke_review(uuid,uuid,uuid,uuid,text,uuid),
    ledger.propose_operator_access(uuid,uuid,uuid,uuid,uuid,text,text,uuid,text),
    ledger.operator_access_handoffs(uuid,uuid,uuid),ledger.accept_operator_access_handoff(uuid,uuid,uuid,uuid,uuid) TO nu54_p05_commerce;

CREATE OR REPLACE FUNCTION ledger.operator_scopes(tenant uuid,actor uuid,role_ text)
RETURNS jsonb LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT coalesce(jsonb_agg(jsonb_build_object('id',b.id,'label',coalesce(n.label,'등록 이름 없음 · '||right(b.id::text,8)),'timezone',b.timezone,
        'capabilities',g.capabilities) ORDER BY n.label,b.id),'[]'::jsonb)
    FROM ledger.branch b LEFT JOIN ledger.operator_branch_label n ON n.branch_id=b.id
    JOIN LATERAL (
       SELECT array_agg(DISTINCT permission ORDER BY permission) AS capabilities
       FROM (
         SELECT 'catalog'::text AS permission FROM ledger.staff_grant s WHERE role_='operator' AND s.branch_id=b.id
           AND s.subject_ref=actor AND s.role_code='operator' AND s.valid_from<=clock_timestamp() AND s.revoked_at IS NULL
         UNION ALL
         SELECT f.capability FROM ledger.finance_grant f WHERE f.branch_id=b.id AND f.actor_id=actor AND
           f.valid_until>clock_timestamp() AND
           (role_='finance' AND f.capability IN ('refund_request','refund_approve','refund_submit','day_close','report_read','report_export','reconcile_read','reconcile_retry')
            OR role_='operator' AND f.capability IN ('experience_read','experience_cancel','payout_propose','payout_approve','payout_notice',
                   'rental_manage','firmware_manage','firmware_station','access_manage','audit_read','legacy_ledger','legacy_cashout','legacy_reconcile')
            OR role_='operator' AND f.capability='pickup_manage' AND EXISTS (
                SELECT FROM ledger.staff_grant active WHERE active.branch_id=b.id AND active.subject_ref=actor
                  AND active.role_code='operator' AND active.valid_from<=clock_timestamp() AND active.revoked_at IS NULL))
       ) authorized
    ) g ON cardinality(g.capabilities)>0
    WHERE b.tenant_ref=tenant;
$$;


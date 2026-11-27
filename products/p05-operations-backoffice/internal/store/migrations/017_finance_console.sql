ALTER TABLE ledger.finance_grant DROP CONSTRAINT finance_grant_capability_check;
ALTER TABLE ledger.finance_grant ADD CHECK (capability IN (
    'refund_request','refund_approve','day_close','report_read','report_export','reconcile_read','reconcile_retry'
));

CREATE TABLE ledger.finance_export (
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    actor_id uuid NOT NULL,
    request_key uuid NOT NULL,
    business_day date NOT NULL,
    asset_ref text NOT NULL,
    snapshot jsonb NOT NULL,
    exported_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(branch_id,actor_id,request_key)
);
REVOKE ALL ON ledger.finance_export FROM PUBLIC;

CREATE FUNCTION ledger.finance_journal_hash(branch uuid,day_ date,asset_ text)
RETURNS text LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT encode(sha256(convert_to(coalesce(jsonb_agg(jsonb_build_array(
        CASE WHEN j.refund_id IS NULL THEN 'paid' ELSE 'refunded' END,
        coalesce(f.tx_hash,p.tx_hash),coalesce(f.log_index,p.log_index),coalesce(f.inclusion_hash,p.inclusion_hash),
        trim_scale(j.amount)::text)
        ORDER BY coalesce(f.tx_hash,p.tx_hash) COLLATE "C",coalesce(f.log_index,p.log_index),
            coalesce(f.inclusion_hash,p.inclusion_hash) COLLATE "C"),'[]'::jsonb)::text,'UTF8')),'hex')
    FROM ledger.journal j JOIN ledger.payment p ON p.sale_id=j.sale_id
    LEFT JOIN ledger.refund_confirmation f ON f.refund_id=j.refund_id
    WHERE j.branch_id=branch AND j.business_day=day_ AND j.asset_ref=asset_;
$$;
REVOKE ALL ON FUNCTION ledger.finance_journal_hash(uuid,date,text) FROM PUBLIC;

CREATE FUNCTION ledger.finance_day_report(tenant uuid,branch uuid,actor uuid,day_ date,asset_ text,export_ boolean)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    scope ledger.branch;
    report jsonb;
    from_ timestamptz;
    until_ timestamptz;
BEGIN
    IF export_ IS TRUE THEN
        PERFORM ledger.require_finance(tenant,branch,actor,'report_export');
    ELSE
        PERFORM ledger.require_finance(tenant,branch,actor,'report_read');
    END IF;
    IF NOT isfinite(day_) OR asset_ !~ '^0x[0-9a-f]{40}$' THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid reporting scope';
    END IF;
    SELECT * INTO STRICT scope FROM ledger.branch b WHERE b.id=branch;
    from_:=(day_+scope.close_local) AT TIME ZONE scope.timezone;
    until_:=((day_+1)+scope.close_local) AT TIME ZONE scope.timezone;
    SELECT jsonb_build_object(
        'branch',branch,'day',day_,'asset',asset_,'zone',scope.timezone,'from',from_,'until',until_,
        'gross',trim_scale(coalesce(sum(j.amount) FILTER (WHERE j.refund_id IS NULL),0))::text,
        'refunds',trim_scale(coalesce(sum(j.amount) FILTER (WHERE j.refund_id IS NOT NULL),0))::text,
        'net',trim_scale(coalesce(sum(CASE WHEN j.refund_id IS NULL THEN j.amount ELSE -j.amount END),0))::text,
        'postings',count(*),'digest',ledger.finance_journal_hash(branch,day_,asset_),
        'closeRevision',(SELECT max(c.revision) FROM ledger.day_close c
            WHERE c.branch_id=branch AND c.business_day=day_ AND c.asset_ref=asset_),
        'openFulfillment',(SELECT count(*) FROM ledger.fulfillment_outbox o JOIN ledger.journal p
            ON (o.kind='paid' AND o.source_ref=p.sale_id AND p.refund_id IS NULL) OR
               (o.kind='refunded' AND o.source_ref=p.refund_id)
            WHERE p.branch_id=branch AND p.business_day=day_ AND p.asset_ref=asset_ AND o.state<>'delivered'),
        'activeRefunds',(SELECT count(*) FROM ledger.refund r JOIN ledger.journal p
            ON p.sale_id=r.sale_id AND p.refund_id IS NULL
            WHERE p.branch_id=branch AND p.business_day=day_ AND p.asset_ref=asset_
                AND r.state IN ('requested','approved','inflight'))
    ) INTO report FROM ledger.journal j
      WHERE j.branch_id=branch AND j.business_day=day_ AND j.asset_ref=asset_;
    RETURN report;
END $$;

CREATE FUNCTION ledger.finance_export_day(tenant uuid,branch uuid,actor uuid,key_ uuid,day_ date,asset_ text)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE prior ledger.finance_export;
DECLARE result jsonb;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'report_export');
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||actor::text||key_::text,17));
    SELECT * INTO prior FROM ledger.finance_export e WHERE e.branch_id=branch AND e.actor_id=actor AND e.request_key=key_;
    IF FOUND THEN
        IF (prior.business_day,prior.asset_ref) IS DISTINCT FROM (day_,asset_) THEN
            RAISE EXCEPTION USING ERRCODE='P1001', MESSAGE='export already bound to another scope';
        END IF;
        RETURN prior.snapshot;
    END IF;
    result:=ledger.finance_day_report(tenant,branch,actor,day_,asset_,true);
    INSERT INTO ledger.finance_export (branch_id,actor_id,request_key,business_day,asset_ref,snapshot)
        VALUES(branch,actor,key_,day_,asset_,result);
    RETURN result;
END $$;

CREATE FUNCTION ledger.finance_order(tenant uuid,branch uuid,actor uuid,order_ uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE result jsonb;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'report_read');
    SELECT jsonb_build_object(
        'saleId',s.id,'asset',s.asset_ref,'amount',s.amount::text,'state',s.state,'revision',s.revision,
        'paidAt',p.included_at,'transaction',p.tx_hash,'refunds',coalesce(p.refunded,0)::text,
        'reserved',coalesce(p.reserved,0)::text,'inFlight',coalesce(p.in_flight,0)::text,
        'events',coalesce((SELECT jsonb_agg(jsonb_build_object('eventId',o.event_id,'kind',o.kind,'state',o.state)
                    ORDER BY o.created_at,o.event_id) FROM ledger.fulfillment_outbox o WHERE o.sale_id=s.id),'[]'::jsonb),
        'refundDetails',coalesce((SELECT jsonb_agg(jsonb_build_object('refundId',r.id,'amount',r.amount::text,'state',r.state,
                    'confirmedAt',f.included_at,'transaction',f.tx_hash) ORDER BY r.requested_at,r.id)
                    FROM ledger.refund r LEFT JOIN ledger.refund_confirmation f ON f.refund_id=r.id WHERE r.sale_id=s.id),'[]'::jsonb)
    ) INTO result FROM ledger.sale s LEFT JOIN ledger.payment p ON p.sale_id=s.id
      WHERE s.branch_id=branch AND s.id=order_;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='order scope denied';
    END IF;
    RETURN result;
END $$;

CREATE FUNCTION ledger.finance_cases(tenant uuid,branch uuid,actor uuid,day_ date,asset_ text)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE result jsonb;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'reconcile_read');
    SELECT coalesce(jsonb_agg(c.detail ORDER BY c.created_at,c.event_id),'[]'::jsonb)
    INTO result FROM (
      SELECT o.created_at,o.event_id,jsonb_build_object('eventId',o.event_id,'saleId',o.sale_id,'kind',o.kind,'state',o.state) AS detail
      FROM ledger.fulfillment_outbox o JOIN ledger.journal j
        ON (o.kind='paid' AND o.source_ref=j.sale_id AND j.refund_id IS NULL) OR
           (o.kind='refunded' AND o.source_ref=j.refund_id)
      WHERE j.branch_id=branch AND j.business_day=day_ AND j.asset_ref=asset_ AND o.state<>'delivered'
      ORDER BY o.created_at,o.event_id LIMIT 100
    ) c;
    RETURN result;
END $$;

REVOKE ALL ON FUNCTION ledger.finance_day_report(uuid,uuid,uuid,date,text,boolean),
    ledger.finance_export_day(uuid,uuid,uuid,uuid,date,text),
    ledger.finance_order(uuid,uuid,uuid,uuid),ledger.finance_cases(uuid,uuid,uuid,date,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.finance_day_report(uuid,uuid,uuid,date,text,boolean),
    ledger.finance_export_day(uuid,uuid,uuid,uuid,date,text),
    ledger.finance_order(uuid,uuid,uuid,uuid),ledger.finance_cases(uuid,uuid,uuid,date,text) TO nu54_p05_finance;

CREATE TABLE ledger.verified_finance_close(
    branch_id uuid NOT NULL,
    business_day date NOT NULL,
    asset_ref text NOT NULL,
    revision bigint NOT NULL,
    anchor_hash text NOT NULL CHECK(anchor_hash ~ '^0x[0-9a-f]{64}$'),
    anchor_height bigint NOT NULL CHECK(anchor_height>=0),
    fact_digest text NOT NULL CHECK(fact_digest ~ '^[0-9a-f]{64}$'),
    PRIMARY KEY(branch_id,business_day,asset_ref,revision),
    FOREIGN KEY(branch_id,business_day,asset_ref,revision) REFERENCES ledger.day_close
);
REVOKE ALL ON ledger.verified_finance_close FROM PUBLIC;

CREATE FUNCTION ledger.finance_close_status(tenant uuid,branch uuid,actor uuid,key_ uuid,day_ date,asset_ text)
RETURNS bigint LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE revision_ bigint;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'day_close');
    SELECT c.revision INTO revision_ FROM ledger.day_close c
      JOIN ledger.verified_finance_close v USING(branch_id,business_day,asset_ref,revision)
      WHERE c.branch_id=branch AND c.business_day=day_ AND c.asset_ref=asset_ AND c.evidence_ref=key_ AND c.closed_by=actor;
    RETURN revision_;
END $$;
REVOKE ALL ON FUNCTION ledger.finance_close_status(uuid,uuid,uuid,uuid,date,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.finance_close_status(uuid,uuid,uuid,uuid,date,text) TO nu54_p05_finance;

CREATE FUNCTION ledger.finance_close_verified(tenant uuid,branch uuid,actor uuid,key_ uuid,day_ date,asset_ text,
    gross_ numeric,refunds_ numeric,journal_ text,anchor_ text,height_ bigint)
RETURNS bigint LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE prior ledger.day_close;
DECLARE snapshot jsonb;
DECLARE revision_ bigint;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'day_close');
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||day_::text||asset_,10));
    SELECT * INTO prior FROM ledger.day_close c
      WHERE c.branch_id=branch AND c.business_day=day_ AND c.asset_ref=asset_ AND c.evidence_ref=key_;
    IF FOUND THEN
        IF prior.closed_by<>actor OR (prior.gross,prior.refunds) IS DISTINCT FROM (gross_,refunds_)
           OR NOT EXISTS (SELECT FROM ledger.verified_finance_close v WHERE v.branch_id=branch
             AND v.business_day=day_ AND v.asset_ref=asset_ AND v.revision=prior.revision) THEN
            RAISE EXCEPTION USING ERRCODE='P1001', MESSAGE='closing key scope changed';
        END IF;
        RETURN prior.revision;
    END IF;
    snapshot:=ledger.finance_day_report(tenant,branch,actor,day_,asset_,false);
    IF (snapshot->>'digest') IS DISTINCT FROM journal_ OR (snapshot->>'activeRefunds')::bigint<>0
       OR (snapshot->>'openFulfillment')::bigint<>0 THEN
        RAISE EXCEPTION USING ERRCODE='P1001', MESSAGE='day close remains held';
    END IF;
    revision_:=ledger.close_business_day(tenant,branch,actor,day_,asset_,gross_,refunds_,key_);
    INSERT INTO ledger.verified_finance_close VALUES(branch,day_,asset_,revision_,anchor_,height_,journal_);
    RETURN revision_;
END $$;
REVOKE ALL ON FUNCTION ledger.finance_close_verified(uuid,uuid,uuid,uuid,date,text,numeric,numeric,text,text,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.finance_close_verified(uuid,uuid,uuid,uuid,date,text,numeric,numeric,text,text,bigint) TO nu54_p05_settlement;

CREATE TABLE ledger.finance_fulfillment_retry (
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    actor_id uuid NOT NULL,
    request_key uuid NOT NULL,
    event_id uuid NOT NULL REFERENCES ledger.fulfillment_outbox,
    accepted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(branch_id,actor_id,request_key)
);
REVOKE ALL ON ledger.finance_fulfillment_retry FROM PUBLIC;

CREATE FUNCTION ledger.finance_retry_fulfillment(tenant uuid,branch uuid,actor uuid,key_ uuid,event_ uuid)
RETURNS text LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE previous uuid;
DECLARE message ledger.fulfillment_outbox;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'reconcile_retry');
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||actor::text||key_::text,18));
    SELECT o.* INTO message FROM ledger.fulfillment_outbox o
      JOIN ledger.sale s ON s.id=o.sale_id WHERE o.event_id=event_ AND s.branch_id=branch FOR UPDATE OF o;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='event scope denied'; END IF;
    SELECT r.event_id INTO previous FROM ledger.finance_fulfillment_retry r
      WHERE r.branch_id=branch AND r.actor_id=actor AND r.request_key=key_;
    IF FOUND THEN
        IF previous<>event_ THEN
            RAISE EXCEPTION USING ERRCODE='P1001', MESSAGE='request key belongs to a different event';
        END IF;
        RETURN message.state;
    END IF;
    -- Keep event/effect IDs and the current worker fence. An unknown result
    -- continues through the dispatch sender's GET, never through a forced POST.
    UPDATE ledger.fulfillment_outbox SET lease_until=NULL
      WHERE event_id=event_ AND state IN ('pending','unknown');
    INSERT INTO ledger.finance_fulfillment_retry(branch_id,actor_id,request_key,event_id) VALUES(branch,actor,key_,event_);
    RETURN message.state;
END $$;
REVOKE ALL ON FUNCTION ledger.finance_retry_fulfillment(uuid,uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.finance_retry_fulfillment(uuid,uuid,uuid,uuid,uuid) TO nu54_p05_finance;

CREATE FUNCTION ledger.finance_notification_review(tenant uuid,branch uuid,actor uuid,day_ date,asset_ text,event_ uuid,key_ uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE result jsonb;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'reconcile_read');
    SELECT jsonb_build_object(
        'eventId',o.event_id,'saleId',o.sale_id,'kind',o.kind,'state',o.state,
        'recipient','merchant-fulfillment','businessDay',j.business_day,'bookedAt',j.booked_at,
        'ledgerAmount',j.amount::text,'requestId',r.request_key,'acceptedAt',r.accepted_at,
        'deliveredAt',o.delivered_at,'receivedAt',c.accepted_at,
        'canRetry',o.state<>'delivered' AND EXISTS(
            SELECT FROM ledger.finance_grant g WHERE g.branch_id=branch AND g.actor_id=actor
              AND g.capability='reconcile_retry' AND g.valid_until>clock_timestamp()))
      INTO result FROM ledger.fulfillment_outbox o JOIN ledger.journal j ON
        ((o.kind='paid' AND o.source_ref=j.sale_id AND j.refund_id IS NULL) OR
         (o.kind='refunded' AND o.source_ref=j.refund_id))
      LEFT JOIN ledger.finance_fulfillment_retry r ON
        r.branch_id=branch AND r.actor_id=actor AND r.event_id=o.event_id AND r.request_key=key_
      LEFT JOIN ledger.consumer_receipt c ON c.event_id=o.event_id
      WHERE j.branch_id=branch AND j.business_day=day_ AND j.asset_ref=asset_ AND o.event_id=event_
        AND (key_ IS NULL OR r.request_key IS NOT NULL);
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='notification scope denied'; END IF;
    RETURN result;
END $$;

CREATE FUNCTION ledger.finance_notification_history(tenant uuid,branch uuid,actor uuid,day_ date,asset_ text)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE result jsonb;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'reconcile_read');
    SELECT coalesce(jsonb_agg(item.detail ORDER BY item.accepted_at DESC,item.request_key),'[]'::jsonb)
      INTO result FROM (
        SELECT r.accepted_at,r.request_key,jsonb_build_object('eventId',o.event_id,'saleId',o.sale_id,
          'requestId',r.request_key,'kind',o.kind,'state',o.state) AS detail
        FROM ledger.finance_fulfillment_retry r JOIN ledger.fulfillment_outbox o ON o.event_id=r.event_id
        JOIN ledger.journal j ON ((o.kind='paid' AND o.source_ref=j.sale_id AND j.refund_id IS NULL) OR
                                  (o.kind='refunded' AND o.source_ref=j.refund_id))
        WHERE r.branch_id=branch AND r.actor_id=actor AND j.branch_id=branch
          AND j.business_day=day_ AND j.asset_ref=asset_
        ORDER BY r.accepted_at DESC,r.request_key LIMIT 100
      ) item;
    RETURN result;
END $$;

CREATE OR REPLACE FUNCTION ledger.finance_retry_fulfillment(tenant uuid,branch uuid,actor uuid,key_ uuid,event_ uuid)
RETURNS text LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE previous uuid;
DECLARE message ledger.fulfillment_outbox;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'reconcile_retry');
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||actor::text||key_::text,18));
    SELECT o.* INTO message FROM ledger.fulfillment_outbox o
      JOIN ledger.sale s ON s.id=o.sale_id WHERE o.event_id=event_ AND s.branch_id=branch FOR UPDATE OF o;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='event scope denied'; END IF;
    IF NOT ledger.request_authority_live(tenant,branch,actor,'finance',ARRAY['reconcile_retry']) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='retry scope expired';
    END IF;
    SELECT r.event_id INTO previous FROM ledger.finance_fulfillment_retry r
      WHERE r.branch_id=branch AND r.actor_id=actor AND r.request_key=key_;
    IF FOUND THEN
        IF previous<>event_ THEN
            RAISE EXCEPTION USING ERRCODE='P1001', MESSAGE='request key belongs to a different event';
        END IF;
        RETURN message.state;
    END IF;
    IF message.state='delivered' THEN
        RAISE EXCEPTION USING ERRCODE='P1001', MESSAGE='notification already delivered';
    END IF;
    -- Keep event/effect IDs and the current worker fence. An unknown result
    -- continues through the dispatch sender's GET, never through a forced POST.
    UPDATE ledger.fulfillment_outbox SET lease_until=NULL
      WHERE event_id=event_ AND state IN ('pending','unknown');
    INSERT INTO ledger.finance_fulfillment_retry(branch_id,actor_id,request_key,event_id) VALUES(branch,actor,key_,event_);
    RETURN message.state;
END $$;
REVOKE ALL ON FUNCTION ledger.finance_notification_review(uuid,uuid,uuid,date,text,uuid,uuid),
    ledger.finance_notification_history(uuid,uuid,uuid,date,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.finance_notification_review(uuid,uuid,uuid,date,text,uuid,uuid),
    ledger.finance_notification_history(uuid,uuid,uuid,date,text) TO nu54_p05_finance;

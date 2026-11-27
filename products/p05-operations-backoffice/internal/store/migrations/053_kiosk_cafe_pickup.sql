-- A travel appointment remains optional for cafe pickup. Keep the existing
-- paid pickup and per-actor transition receipt as the shared write targets.
ALTER TABLE ledger.travel_pickup_progress DROP CONSTRAINT travel_pickup_progress_sale_id_fkey;
ALTER TABLE ledger.travel_pickup_progress ADD FOREIGN KEY(sale_id) REFERENCES ledger.pickup(sale_id);
ALTER TABLE ledger.travel_pickup_transition DROP CONSTRAINT travel_pickup_transition_sale_id_fkey;
ALTER TABLE ledger.travel_pickup_transition ADD FOREIGN KEY(sale_id) REFERENCES ledger.pickup(sale_id);

CREATE FUNCTION ledger.kiosk_menu_pickup(sale_ uuid) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT EXISTS(SELECT FROM ledger.sale s JOIN ledger.order_line l ON l.sale_id=s.id
      WHERE s.id=sale_ AND s.origin='menu');
$$;
REVOKE ALL ON FUNCTION ledger.kiosk_menu_pickup(uuid) FROM PUBLIC;

CREATE OR REPLACE FUNCTION ledger.kiosk_pickup_queue(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid,role_ text)
RETURNS SETOF jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $kitchen$
BEGIN
    PERFORM ledger.kiosk_require_pickup(tenant_,branch_,actor_,role_);
    RETURN QUERY SELECT jsonb_strip_nulls(jsonb_build_object(
      'saleRef',s.id,'kind',CASE WHEN o.sale_id IS NULL THEN 'cafe' ELSE 'travel' END,
      'startsAt',coalesce(o.starts_at,paid.included_at,submission.received_at,c.expires_at),'timezone',b.timezone,
      'pickupNo',CASE WHEN ack.accepted THEN p.number END,
      'eventRef',CASE WHEN ack.accepted THEN e.event_id END,
      'state',CASE WHEN ack.accepted THEN coalesce(k.state,'accepted') ELSE 'checking' END,
      'revision',coalesce(k.revision,0),
      'items',CASE WHEN sale_ IS NOT NULL AND ack.accepted THEN (SELECT jsonb_agg(jsonb_build_object(
        'label',x.label,'option',l.option_key,'quantity',l.quantity) ORDER BY l.sku)
        FROM ledger.order_line l JOIN ledger.price x ON
          (x.branch_id,x.version,x.sku,x.option_key)=(l.branch_id,l.menu_version,l.sku,l.option_key)
        WHERE l.sale_id=s.id) END))
    FROM ledger.sale s JOIN ledger.order_checkout c ON (c.branch_id,c.sale_id)=(s.branch_id,s.id)
    JOIN ledger.branch b ON b.id=s.branch_id
    LEFT JOIN ledger.travel_pickup_order o ON o.sale_id=s.id
    LEFT JOIN ledger.travel_pickup_progress k ON k.sale_id=s.id
    LEFT JOIN ledger.payment paid ON paid.sale_id=s.id
    LEFT JOIN ledger.settlement_submission submission ON submission.sale_id=s.id
    LEFT JOIN ledger.pickup p ON p.sale_id=s.id
    LEFT JOIN ledger.fulfillment_outbox e ON e.sale_id=s.id AND e.kind='paid' AND e.source_ref=s.id
    LEFT JOIN ledger.consumer_receipt response ON response.event_id=e.event_id AND response.effect_key=e.effect_key
    CROSS JOIN LATERAL (SELECT s.state='paid' AND p.sale_id IS NOT NULL AND response.event_id IS NOT NULL AS accepted) ack
    WHERE s.branch_id=branch_ AND (sale_ IS NULL OR s.id=sale_) AND (
       (o.sale_id IS NOT NULL AND ack.accepted) OR
       (o.sale_id IS NULL AND role_='operator' AND s.state IN ('checking','paid') AND ledger.kiosk_menu_pickup(s.id))
    ) AND (sale_ IS NOT NULL OR coalesce(k.state,'accepted') NOT IN ('collected','unfulfillable'))
    ORDER BY ack.accepted DESC,coalesce(o.starts_at,paid.included_at,submission.received_at,c.expires_at),s.id LIMIT 32;
END
$kitchen$;

CREATE OR REPLACE FUNCTION ledger.kiosk_pickup_change(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid,key_ uuid,
    expected_ bigint,target_ text,role_ text)
RETURNS bigint LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $step$
DECLARE current_ ledger.travel_pickup_progress; prior ledger.travel_pickup_transition; state_ text; version_ bigint;
BEGIN
    PERFORM ledger.kiosk_require_pickup(tenant_,branch_,actor_,role_);
    PERFORM FROM ledger.sale s JOIN ledger.pickup p ON (p.branch_id,p.sale_id)=(s.branch_id,s.id)
      WHERE s.branch_id=branch_ AND s.id=sale_ AND s.state='paid' AND (
          EXISTS (SELECT FROM ledger.travel_pickup_order o WHERE o.sale_id=s.id) OR
          (role_='operator' AND ledger.kiosk_menu_pickup(s.id))
      ) FOR UPDATE OF s;
    IF NOT FOUND OR NOT ledger.travel_pickup_accepted(sale_) THEN
      RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='pickup unavailable';
    END IF;
    PERFORM ledger.kiosk_require_pickup(tenant_,branch_,actor_,role_);
    SELECT * INTO prior FROM ledger.travel_pickup_transition x
       WHERE (x.sale_id,x.actor_id,x.request_key)=(sale_,actor_,key_);
    IF FOUND THEN
      IF (prior.target,prior.revision-1) IS DISTINCT FROM (target_,expected_) THEN
        RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='pickup action changed';
      END IF;
      RETURN prior.revision;
    END IF;
    SELECT * INTO current_ FROM ledger.travel_pickup_progress p WHERE p.sale_id=sale_ FOR UPDATE;
    PERFORM ledger.kiosk_require_pickup(tenant_,branch_,actor_,role_);
    state_:=coalesce(current_.state,'accepted');
    version_:=coalesce(current_.revision,0);
    IF expected_ IS DISTINCT FROM version_ OR NOT coalesce(
        (state_='accepted' AND target_ IN ('preparing','unfulfillable')) OR
        (state_='preparing' AND target_ IN ('ready','unfulfillable')) OR
        (state_='ready' AND target_ IN ('collected','unfulfillable')),false) THEN
      RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='pickup state changed';
    END IF;
    INSERT INTO ledger.travel_pickup_progress(sale_id,state,revision,updated_at)
      VALUES(sale_,target_,version_+1,clock_timestamp())
      ON CONFLICT(sale_id) DO UPDATE SET state=excluded.state,revision=excluded.revision,updated_at=excluded.updated_at;
    INSERT INTO ledger.travel_pickup_transition VALUES(sale_,actor_,key_,version_+1,target_);
    RETURN version_+1;
END
$step$;

CREATE OR REPLACE FUNCTION ledger.link_travel_pickup_refund() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $refund$
DECLARE progress ledger.travel_pickup_progress; prior ledger.refund; paid ledger.payment;
BEGIN
    IF NEW.decision_path<>'manual' OR NEW.reason_code<>'merchant_cancel'
       OR NOT EXISTS (SELECT FROM ledger.travel_pickup_order o WHERE o.sale_id=NEW.sale_id)
    THEN RETURN NEW; END IF;
    SELECT * INTO progress FROM ledger.travel_pickup_progress p
      WHERE p.sale_id=NEW.sale_id AND p.state='unfulfillable' FOR UPDATE;
    IF NOT FOUND THEN RETURN NEW; END IF;
    SELECT * INTO prior FROM ledger.refund r WHERE r.id=progress.refund_id;
    SELECT * INTO paid FROM ledger.payment p WHERE p.sale_id=NEW.sale_id;
    IF (prior.id IS NOT NULL AND prior.state<>'cancelled') OR NEW.amount<>paid.principal-paid.refunded THEN
      RAISE EXCEPTION USING ERRCODE='P1001',MESSAGE='pickup refund already linked or amount differs';
    END IF;
    UPDATE ledger.travel_pickup_progress p SET refund_id=NEW.id WHERE p.sale_id=NEW.sale_id;
    RETURN NEW;
END
$refund$;
CREATE OR REPLACE FUNCTION ledger.commerce_order_receipt(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid)
RETURNS TABLE(id uuid,revision bigint,state text,asset_ref text,amount numeric,expires_at timestamptz,
    pickup_no bigint,receipt_ref uuid,quote_digest text,order_key text,receipt_kind text,subscription jsonb,
    payment jsonb,access_ends_at timestamptz)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $receipt$
    SELECT v.*,
        CASE WHEN evidence.stage IS NOT NULL THEN jsonb_strip_nulls(jsonb_build_object(
            'stage',evidence.stage,
            'chainId',8283,
            'relayTxHash',attempt.tx_hash,
            'txHash',paid.tx_hash,
            'includedAt',paid.included_at,
            'pickupStage',CASE WHEN v.state='paid' AND v.pickup_no IS NOT NULL THEN coalesce(
                pickup.state, CASE WHEN ledger.kiosk_menu_pickup(v.id)
                    AND NOT EXISTS (SELECT FROM ledger.travel_pickup_order o WHERE o.sale_id=v.id)
                    AND ledger.travel_pickup_accepted(v.id) THEN 'accepted' END) END
        )) END,
        CASE WHEN grant_record.role_code='customer' AND checkout.creator<>actor_
                    AND (v.state<>'paid' OR v.receipt_ref IS NULL OR payer.sale_id IS NULL)
             THEN claim.expires_at END
    FROM ledger.commerce_order_status_with_invoice(tenant_,branch_,actor_,sale_) v
    CROSS JOIN LATERAL ledger.commerce_allowed(tenant_,branch_,actor_) grant_record
    JOIN ledger.order_checkout checkout ON (checkout.branch_id,checkout.sale_id)=(branch_,v.id)
    LEFT JOIN ledger.checkout_claim claim ON (claim.branch_id,claim.sale_id,claim.actor_id)=(branch_,v.id,actor_)
    LEFT JOIN ledger.quote_work payer ON payer.sale_id=v.id AND payer.signed_digest IS NOT NULL
        AND payer.actor_id=actor_ AND grant_record.role_code='customer'
    LEFT JOIN ledger.settlement_submission acceptance ON acceptance.sale_id=payer.sale_id
    LEFT JOIN ledger.settlement_work work ON work.sale_id=payer.sale_id AND work.refund_id IS NULL
    LEFT JOIN ledger.relay_attempt attempt ON attempt.work_id=work.id
    LEFT JOIN ledger.payment paid ON paid.sale_id=payer.sale_id AND v.state='paid'
    LEFT JOIN ledger.sale_resolution unpaid ON unpaid.sale_id=payer.sale_id AND v.state='closed'
    LEFT JOIN ledger.travel_pickup_progress pickup ON pickup.sale_id=paid.sale_id
    CROSS JOIN LATERAL (
        SELECT CASE
            WHEN paid.sale_id IS NOT NULL THEN 'finalized'
            WHEN unpaid.sale_id IS NOT NULL THEN 'unpaid'
            WHEN v.state='checking' AND attempt.state='signed' THEN 'relaySigned'
            WHEN v.state='checking' AND attempt.state='broadcast' THEN 'submitted'
            WHEN v.state='checking' AND attempt.state IN ('unknown','observed','reverted','final') THEN 'checking'
            WHEN v.state='checking' AND acceptance.sale_id IS NOT NULL THEN 'received'
        END AS stage
    ) evidence
    WHERE grant_record.role_code<>'customer' OR v.state<>'paid' OR payer.sale_id IS NOT NULL
          OR checkout.creator=actor_
$receipt$;

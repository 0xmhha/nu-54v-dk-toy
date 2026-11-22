CREATE TABLE ledger.travel_pickup_terminal (
    branch_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    PRIMARY KEY(branch_id,actor_id),
    FOREIGN KEY(branch_id,actor_id) REFERENCES ledger.commerce_grant ON DELETE CASCADE
);

CREATE TABLE ledger.travel_pickup_progress (
    sale_id uuid PRIMARY KEY REFERENCES ledger.travel_pickup_order,
    state text NOT NULL CHECK (state IN ('preparing','ready','collected','unfulfillable')),
    revision bigint NOT NULL CHECK(revision>0),
    updated_at timestamptz NOT NULL CHECK(isfinite(updated_at)),
    refund_id uuid,
    FOREIGN KEY(sale_id,refund_id) REFERENCES ledger.refund(sale_id,id),
    CHECK (refund_id IS NULL OR state='unfulfillable')
);

CREATE TABLE ledger.travel_pickup_transition (
    sale_id uuid NOT NULL REFERENCES ledger.travel_pickup_order,
    actor_id uuid NOT NULL,
    request_key uuid NOT NULL,
    revision bigint NOT NULL CHECK(revision>0),
    target text NOT NULL CHECK(target IN ('preparing','ready','collected','unfulfillable')),
    PRIMARY KEY(sale_id,actor_id,request_key),
    UNIQUE(sale_id,revision)
);

CREATE FUNCTION ledger.travel_pickup_accepted(sale_ uuid) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $accepted$
    SELECT EXISTS(SELECT FROM ledger.fulfillment_outbox e JOIN ledger.consumer_receipt c USING(event_id)
      WHERE e.sale_id=sale_ AND e.kind='paid' AND c.effect_key=e.effect_key)
$accepted$;

CREATE FUNCTION ledger.require_pickup_terminal(tenant_ uuid,branch_ uuid,actor_ uuid) RETURNS void
LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $terminal$
DECLARE grant_ ledger.commerce_grant := ledger.commerce_allowed(tenant_,branch_,actor_);
BEGIN
    IF grant_.role_code<>'terminal' OR NOT EXISTS(SELECT FROM ledger.travel_pickup_terminal t
         WHERE (t.branch_id,t.actor_id)=(branch_,actor_) FOR SHARE) THEN
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='pickup assignment unavailable';
    END IF;
END
$terminal$;

CREATE FUNCTION ledger.travel_pickup_customer(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid)
RETURNS SETOF jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $owned$
DECLARE grant_ ledger.commerce_grant := ledger.commerce_allowed(tenant_,branch_,actor_);
BEGIN
    IF grant_.role_code<>'customer' THEN
      RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='pickup unavailable';
    END IF;
    RETURN QUERY SELECT jsonb_strip_nulls(jsonb_build_object(
      'saleRef',s.id,'startsAt',o.starts_at,'timezone',b.timezone,'amount',s.amount::text,'asset',s.asset_ref,
      'state',CASE WHEN s.state='closed' THEN 'closed' WHEN s.state<>'paid' THEN 'waiting-payment'
        WHEN NOT ack.accepted THEN 'awaiting-acceptance' ELSE coalesce(k.state,'accepted') END,
      'pickupNo',CASE WHEN s.state='paid' AND ack.accepted THEN p.number END,
      'receiptRef',CASE WHEN s.state='paid' THEN p.receipt_ref END,
      'refund',CASE WHEN k.state='unfulfillable' THEN jsonb_build_object(
          'state', CASE WHEN evidence.refund_id IS NOT NULL AND r.state='final' THEN 'confirmed'
            WHEN r.state='cancelled' THEN 'rejected' WHEN r.id IS NOT NULL THEN 'pending' ELSE 'awaiting-request' END,
          'amount',r.amount::text,'receiptRef',CASE WHEN r.state='final' THEN evidence.evidence_id END) END))
    FROM ledger.travel_pickup_order o JOIN ledger.sale s ON s.id=o.sale_id
    JOIN ledger.order_checkout c ON c.sale_id=o.sale_id AND c.creator=actor_
    JOIN ledger.branch b ON b.id=o.branch_id
    LEFT JOIN ledger.pickup p ON p.sale_id=o.sale_id
    LEFT JOIN ledger.travel_pickup_progress k ON k.sale_id=o.sale_id
    LEFT JOIN ledger.refund r ON r.id=k.refund_id AND r.sale_id=o.sale_id
    LEFT JOIN ledger.refund_confirmation evidence ON evidence.refund_id=r.id
    CROSS JOIN LATERAL (SELECT ledger.travel_pickup_accepted(o.sale_id) AS accepted) ack
    WHERE o.branch_id=branch_ AND (sale_ IS NULL OR o.sale_id=sale_)
    ORDER BY o.starts_at DESC,o.sale_id LIMIT 32;
END
$owned$;

CREATE FUNCTION ledger.travel_pickup_kitchen(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid)
RETURNS SETOF jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $kitchen$
BEGIN
    PERFORM ledger.require_pickup_terminal(tenant_,branch_,actor_);
    RETURN QUERY SELECT jsonb_strip_nulls(jsonb_build_object(
      'saleRef',s.id,'startsAt',o.starts_at,'timezone',b.timezone,'pickupNo',p.number,
      'state',coalesce(k.state,'accepted'),'revision',coalesce(k.revision,0),
      'items',CASE WHEN sale_ IS NOT NULL THEN (SELECT jsonb_agg(jsonb_build_object(
        'label',x.label,'option',l.option_key,'quantity',l.quantity) ORDER BY l.sku)
        FROM ledger.order_line l JOIN ledger.price x ON
          (x.branch_id,x.version,x.sku,x.option_key)=(l.branch_id,l.menu_version,l.sku,l.option_key)
        WHERE l.sale_id=o.sale_id) END))
    FROM ledger.travel_pickup_order o JOIN ledger.sale s ON s.id=o.sale_id AND s.state='paid'
    JOIN ledger.pickup p ON p.sale_id=o.sale_id
    JOIN ledger.branch b ON b.id=o.branch_id
    LEFT JOIN ledger.travel_pickup_progress k ON k.sale_id=o.sale_id
    WHERE o.branch_id=branch_ AND (sale_ IS NULL OR o.sale_id=sale_) AND ledger.travel_pickup_accepted(o.sale_id)
      AND (sale_ IS NOT NULL OR coalesce(k.state,'accepted') NOT IN ('collected','unfulfillable'))
    ORDER BY o.starts_at,o.sale_id LIMIT 32;
END
$kitchen$;

CREATE FUNCTION ledger.change_travel_pickup(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid,key_ uuid,
    expected_ bigint,target_ text)
RETURNS bigint LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $step$
DECLARE current_ ledger.travel_pickup_progress; prior ledger.travel_pickup_transition; state_ text; version_ bigint;
BEGIN
    PERFORM ledger.require_pickup_terminal(tenant_,branch_,actor_);
    PERFORM FROM ledger.travel_pickup_order o JOIN ledger.sale s ON s.id=o.sale_id
      WHERE o.branch_id=branch_ AND o.sale_id=sale_ AND s.state='paid' FOR UPDATE OF s;
    IF NOT FOUND OR NOT ledger.travel_pickup_accepted(sale_) THEN
      RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='pickup unavailable';
    END IF;
    SELECT * INTO prior FROM ledger.travel_pickup_transition x
       WHERE (x.sale_id,x.actor_id,x.request_key)=(sale_,actor_,key_);
    IF FOUND THEN
      IF (prior.target,prior.revision-1) IS DISTINCT FROM (target_,expected_) THEN
        RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='pickup action changed';
      END IF;
      RETURN prior.revision;
    END IF;
    SELECT * INTO current_ FROM ledger.travel_pickup_progress p WHERE p.sale_id=sale_ FOR UPDATE;
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

CREATE FUNCTION ledger.link_travel_pickup_refund() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $refund$
DECLARE progress ledger.travel_pickup_progress; prior ledger.refund; paid ledger.payment;
BEGIN
    IF NEW.decision_path<>'manual' OR NEW.reason_code<>'merchant_cancel' THEN RETURN NEW; END IF;
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
CREATE TRIGGER travel_pickup_refund_link AFTER INSERT ON ledger.refund
 FOR EACH ROW EXECUTE FUNCTION ledger.link_travel_pickup_refund();

REVOKE ALL ON FUNCTION ledger.travel_pickup_accepted(uuid),ledger.require_pickup_terminal(uuid,uuid,uuid),
    ledger.travel_pickup_customer(uuid,uuid,uuid,uuid),ledger.travel_pickup_kitchen(uuid,uuid,uuid,uuid),
    ledger.change_travel_pickup(uuid,uuid,uuid,uuid,uuid,bigint,text),ledger.link_travel_pickup_refund() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.travel_pickup_customer(uuid,uuid,uuid,uuid),ledger.travel_pickup_kitchen(uuid,uuid,uuid,uuid),
    ledger.change_travel_pickup(uuid,uuid,uuid,uuid,uuid,bigint,text) TO nu54_p05_commerce;

CREATE TABLE ledger.travel_pickup_window (
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    starts_at timestamptz NOT NULL CHECK (isfinite(starts_at)),
    ends_at timestamptz NOT NULL CHECK (isfinite(ends_at) AND ends_at>starts_at),
    capacity integer NOT NULL CHECK (capacity BETWEEN 1 AND 500),
    active boolean NOT NULL DEFAULT true,
    PRIMARY KEY(branch_id,id),
    UNIQUE(branch_id,starts_at)
);

CREATE TABLE ledger.travel_pickup_order (
    sale_id uuid PRIMARY KEY,
    branch_id uuid NOT NULL,
    window_id uuid NOT NULL,
    starts_at timestamptz NOT NULL CHECK(isfinite(starts_at)),
    FOREIGN KEY(branch_id,sale_id) REFERENCES ledger.sale(branch_id,id),
    FOREIGN KEY(branch_id,window_id) REFERENCES ledger.travel_pickup_window(branch_id,id)
);
CREATE INDEX travel_pickup_by_window ON ledger.travel_pickup_order(branch_id,window_id);

CREATE FUNCTION ledger.travel_pickup_remaining(branch_ uuid,window_ uuid,capacity_ integer,now_ timestamptz)
RETURNS integer LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $remain$
    SELECT greatest(0,capacity_-count(*)::integer)
    FROM ledger.travel_pickup_order p JOIN ledger.sale s ON s.id=p.sale_id
    JOIN ledger.order_checkout c ON c.sale_id=p.sale_id
    WHERE p.branch_id=branch_ AND p.window_id=window_
      AND (s.state IN ('checking','paid') OR (s.state IN ('preview','issued') AND c.expires_at>now_))
$remain$;

CREATE FUNCTION ledger.travel_pickup_options(tenant_ uuid,branch_ uuid,actor_ uuid)
RETURNS TABLE(id uuid,starts_at timestamptz,ends_at timestamptz,timezone text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $options$
DECLARE grant_ ledger.commerce_grant := ledger.commerce_allowed(tenant_,branch_,actor_);
DECLARE menu_ bigint; now_ timestamptz := statement_timestamp();
BEGIN
    IF grant_.role_code<>'customer' THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='pickup scope unavailable';
    END IF;
    SELECT max(m.version) INTO menu_ FROM ledger.menu_release m WHERE m.branch_id=branch_
       AND m.published_at<=now_ AND m.expires_at>now_;
    IF menu_ IS NULL OR NOT ledger.catalog_is_open(branch_,menu_,now_) THEN RETURN; END IF;
    RETURN QUERY SELECT w.id,w.starts_at,w.ends_at,b.timezone FROM ledger.travel_pickup_window w
      JOIN ledger.branch b ON b.id=w.branch_id
      WHERE w.branch_id=branch_ AND w.active AND w.starts_at>now_ AND
        ledger.catalog_is_open(branch_,menu_,w.starts_at) AND
        ledger.catalog_is_open(branch_,menu_,w.ends_at-interval '1 microsecond') AND
        ledger.travel_pickup_remaining(branch_,w.id,w.capacity,now_)>0
      ORDER BY w.starts_at,w.id LIMIT 48;
END
$options$;

CREATE FUNCTION ledger.reserve_travel_pickup(tenant_ uuid,branch_ uuid,actor_ uuid,key_ uuid,origin_ref_ uuid,
    menu_ bigint,asset_ text,lines_ jsonb,window_ uuid,starts_ timestamptz)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $reserve$
DECLARE grant_ ledger.commerce_grant := ledger.commerce_allowed(tenant_,branch_,actor_);
DECLARE slot ledger.travel_pickup_window; old_ ledger.travel_pickup_order; existing uuid; result uuid; now_ timestamptz;
BEGIN
    IF grant_.role_code<>'customer' OR key_ IS NULL OR origin_ref_ IS NULL OR window_ IS NULL OR
        starts_ IS NULL OR NOT isfinite(starts_) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='pickup scope unavailable';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch_::text||actor_::text||'create_sale'||key_::text,0));
    existing := ledger.resume_reserve_order(tenant_,branch_,actor_,key_,'menu',origin_ref_,menu_,asset_,lines_,'customer');
    PERFORM pg_advisory_xact_lock(hashtextextended(branch_::text||'menu'||origin_ref_::text,1));
    IF existing IS NULL THEN
      SELECT s.id INTO existing FROM ledger.sale s WHERE (s.branch_id,s.origin,s.origin_ref)=(branch_,'menu',origin_ref_);
    END IF;
    IF existing IS NOT NULL THEN
       SELECT * INTO old_ FROM ledger.travel_pickup_order p WHERE p.sale_id=existing;
       IF NOT FOUND OR (old_.window_id,old_.starts_at) IS DISTINCT FROM (window_,starts_) THEN
           RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='pickup request conflict';
       END IF;
       RETURN ledger.reserve_order(tenant_,branch_,actor_,key_,'menu',origin_ref_,menu_,asset_,lines_,'customer');
    END IF;

    PERFORM pg_advisory_xact_lock_shared(hashtextextended(branch_::text||'operator-catalog',0));
    SELECT * INTO slot FROM ledger.travel_pickup_window w WHERE (w.branch_id,w.id)=(branch_,window_) FOR UPDATE;
    now_ := clock_timestamp();
    IF NOT FOUND OR NOT slot.active OR slot.starts_at<>starts_ OR slot.starts_at<=now_ OR
       NOT ledger.catalog_is_open(branch_,menu_,slot.starts_at) OR
       NOT ledger.catalog_is_open(branch_,menu_,slot.ends_at-interval '1 microsecond') OR
       ledger.travel_pickup_remaining(branch_,window_,slot.capacity,now_)=0 THEN
      RAISE EXCEPTION USING ERRCODE='P0702', MESSAGE='pickup window changed';
    END IF;
    result := ledger.reserve_order(tenant_,branch_,actor_,key_,'menu',origin_ref_,menu_,asset_,lines_,'customer');
    INSERT INTO ledger.travel_pickup_order VALUES(result,branch_,window_,slot.starts_at);
    UPDATE ledger.order_checkout c SET expires_at=least(c.expires_at,slot.starts_at) WHERE c.sale_id=result;
    UPDATE ledger.command_receipt r SET retire_after=least(r.retire_after,slot.starts_at)
      WHERE (r.branch_id,r.scope_ref,r.operation,r.request_key)=(branch_,actor_,'create_sale',key_);
    RETURN result;
END
$reserve$;

REVOKE ALL ON FUNCTION ledger.travel_pickup_remaining(uuid,uuid,integer,timestamptz),
    ledger.travel_pickup_options(uuid,uuid,uuid),
    ledger.reserve_travel_pickup(uuid,uuid,uuid,uuid,uuid,bigint,text,jsonb,uuid,timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.travel_pickup_options(uuid,uuid,uuid),
    ledger.reserve_travel_pickup(uuid,uuid,uuid,uuid,uuid,bigint,text,jsonb,uuid,timestamptz) TO nu54_p05_commerce;

CREATE FUNCTION ledger.commerce_order_status(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid)
RETURNS TABLE(id uuid,revision bigint,state text,asset_ref text,amount numeric,expires_at timestamptz,
    pickup_no bigint,receipt_ref uuid,quote_digest text,order_key text,receipt_kind text)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $status$
    SELECT v.id,v.revision,v.state,v.asset_ref,v.amount,v.expires_at,v.pickup_no,
        coalesce(v.receipt_ref,j.id),v.quote_digest,v.order_key,
        CASE WHEN v.state='paid' AND i.fulfillment_kind='digital' THEN 'digital'
             WHEN v.state='paid' AND leg.sale_id IS NOT NULL THEN 'booking' END
    FROM ledger.commerce_order_view(tenant_,branch_,actor_,sale_) v
    JOIN ledger.sale s ON s.id=v.id AND s.branch_id=branch_
    LEFT JOIN ledger.invoice i ON i.sale_id=s.id AND s.origin='subscription'
    LEFT JOIN ledger.travel_booking_leg leg ON leg.sale_id=s.id AND leg.branch_id=branch_ AND s.origin='travel'
    LEFT JOIN ledger.payment p ON p.sale_id=leg.sale_id AND v.state='paid'
    LEFT JOIN ledger.journal j ON j.sale_id=p.sale_id AND j.refund_id IS NULL
$status$;
REVOKE ALL ON FUNCTION ledger.commerce_order_status(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.commerce_order_status(uuid,uuid,uuid,uuid) TO nu54_p05_commerce;

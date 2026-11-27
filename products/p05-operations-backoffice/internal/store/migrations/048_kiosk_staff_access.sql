-- K12 uses individual staff credentials and branch-scoped permissions, or
-- the pre-existing assigned credential of a dedicated kitchen station.
ALTER TABLE ledger.finance_grant DROP CONSTRAINT finance_grant_capability_check;
ALTER TABLE ledger.finance_grant ADD CONSTRAINT finance_grant_capability_check CHECK(capability IN (
    'refund_request','refund_approve','refund_submit','day_close','report_read','report_export','reconcile_read','reconcile_retry',
    'payout_propose','payout_approve','payout_notice','rental_manage','firmware_manage','firmware_station',
    'experience_read','experience_cancel','access_manage','audit_read','legacy_ledger','legacy_cashout','legacy_reconcile','pickup_manage'
));

CREATE FUNCTION ledger.kiosk_require_pickup(tenant_ uuid,branch_ uuid,actor_ uuid,role_ text)
RETURNS void LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF role_='terminal' AND ledger.request_authority_live(tenant_,branch_,actor_,role_,ARRAY[]::text[]) THEN
        PERFORM ledger.require_pickup_terminal(tenant_,branch_,actor_);
        IF NOT ledger.request_authority_live(tenant_,branch_,actor_,role_,ARRAY[]::text[]) THEN
            RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='pickup assignment unavailable';
        END IF;
    ELSIF role_='operator' AND ledger.request_authority_live(tenant_,branch_,actor_,role_,ARRAY['pickup_manage']) THEN
        RETURN;
    ELSE
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='pickup assignment unavailable';
    END IF;
END $$;

CREATE FUNCTION ledger.kiosk_staff_access(tenant_ uuid,branch_ uuid,actor_ uuid,role_ text)
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
    ELSIF role_='finance' AND ledger.request_authority_live(tenant_,branch_,actor_,role_,ARRAY['refund_request','refund_approve','refund_submit']) THEN
        permissions := ARRAY['refund_console'];
    END IF;
    IF cardinality(permissions)=0 THEN
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='staff scope denied';
    END IF;
    RETURN jsonb_build_object('branchRef',branch_,'actorRef',actor_,'permissions',permissions);
END $$;

CREATE FUNCTION ledger.kiosk_pickup_queue(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid,role_ text)
RETURNS SETOF jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $kitchen$
BEGIN
    PERFORM ledger.kiosk_require_pickup(tenant_,branch_,actor_,role_);
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

CREATE FUNCTION ledger.kiosk_pickup_change(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid,key_ uuid,
    expected_ bigint,target_ text,role_ text)
RETURNS bigint LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $step$
DECLARE current_ ledger.travel_pickup_progress; prior ledger.travel_pickup_transition; state_ text; version_ bigint;
BEGIN
    PERFORM ledger.kiosk_require_pickup(tenant_,branch_,actor_,role_);
    PERFORM FROM ledger.travel_pickup_order o JOIN ledger.sale s ON s.id=o.sale_id
      WHERE o.branch_id=branch_ AND o.sale_id=sale_ AND s.state='paid' FOR UPDATE OF s;
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

CREATE OR REPLACE FUNCTION ledger.travel_pickup_kitchen(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid)
RETURNS SETOF jsonb LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT * FROM ledger.kiosk_pickup_queue(tenant_,branch_,actor_,sale_,'terminal');
$$;

CREATE OR REPLACE FUNCTION ledger.change_travel_pickup(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid,key_ uuid,
    expected_ bigint,target_ text)
RETURNS bigint LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT ledger.kiosk_pickup_change(tenant_,branch_,actor_,sale_,key_,expected_,target_,'terminal');
$$;

CREATE FUNCTION ledger.terminal_orders_page(tenant_ uuid,branch_ uuid,actor_ uuid,terminal_ uuid,
    kind_ text, ref_ uuid,source_ bigint,after_ uuid)
RETURNS text[] LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE live record;
DECLARE items text[];
BEGIN
    SELECT * INTO STRICT live FROM ledger.terminal_mode_state(tenant_,branch_,actor_,terminal_,'operator');
    IF kind_='transition' THEN
        IF NOT EXISTS(SELECT FROM ledger.terminal_transition t
            WHERE (t.branch_id,t.terminal_ref,t.request_ref,t.source_revision)=(branch_,terminal_,ref_,source_)) OR
           NOT EXISTS(SELECT FROM ledger.terminal_handoff_order h
            WHERE (h.branch_id,h.terminal_ref,h.request_ref,h.sale_id)=(branch_,terminal_,ref_,after_)) THEN
            RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='transition order scope denied';
        END IF;
        SELECT array(SELECT h.sale_id::text FROM ledger.terminal_handoff_order h
            WHERE (h.branch_id,h.terminal_ref,h.request_ref)=(branch_,terminal_,ref_) AND h.sale_id>after_
            ORDER BY h.sale_id LIMIT 20) INTO items;
    ELSIF kind_='preview' THEN
        IF ref_ IS DISTINCT FROM terminal_ OR live.revision IS DISTINCT FROM source_ OR
           NOT EXISTS(SELECT FROM ledger.order_checkout c WHERE (c.branch_id,c.creator,c.sale_id)=(branch_,terminal_,after_)) THEN
            RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='preview order scope denied';
        END IF;
        SELECT array(SELECT c.sale_id::text FROM ledger.order_checkout c JOIN ledger.sale s ON (s.branch_id,s.id)=(c.branch_id,c.sale_id)
            WHERE (c.branch_id,c.creator)=(branch_,terminal_) AND c.sale_id>after_ AND
              (s.state IN ('issued','checking') OR (s.state='preview' AND c.expires_at>clock_timestamp()))
            ORDER BY c.sale_id LIMIT 20) INTO items;
    ELSE
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='order scope denied';
    END IF;
    RETURN items;
END $$;

REVOKE ALL ON FUNCTION ledger.kiosk_require_pickup(uuid,uuid,uuid,text),
    ledger.kiosk_staff_access(uuid,uuid,uuid,text),ledger.kiosk_pickup_queue(uuid,uuid,uuid,uuid,text),
    ledger.kiosk_pickup_change(uuid,uuid,uuid,uuid,uuid,bigint,text,text),
    ledger.terminal_orders_page(uuid,uuid,uuid,uuid,text,uuid,bigint,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.kiosk_staff_access(uuid,uuid,uuid,text),
    ledger.kiosk_pickup_queue(uuid,uuid,uuid,uuid,text),
    ledger.kiosk_pickup_change(uuid,uuid,uuid,uuid,uuid,bigint,text,text),
    ledger.terminal_orders_page(uuid,uuid,uuid,uuid,text,uuid,bigint,uuid) TO nu54_p05_commerce;

-- Nullable for existing records: later slot edits cannot establish past booking terms.
ALTER TABLE ledger.travel_slot
    ADD COLUMN price_version bigint CHECK (price_version BETWEEN 1 AND 9007199254740991),
    ADD COLUMN no_show_terms text CHECK (octet_length(btrim(no_show_terms)) BETWEEN 1 AND 1000),
    ADD COLUMN chain_id integer CHECK (chain_id = 8283);

ALTER TABLE ledger.travel_booking
    ADD COLUMN price_version bigint CHECK (price_version BETWEEN 1 AND 9007199254740991),
    ADD COLUMN no_show_terms text CHECK (octet_length(btrim(no_show_terms)) BETWEEN 1 AND 1000),
    ADD COLUMN chain_id integer CHECK (chain_id = 8283),
    ADD COLUMN starts_at timestamptz CHECK (isfinite(starts_at));

-- The published terms version fences every field a customer reviews before holding a slot.
CREATE FUNCTION ledger.travel_slot_review_version()
RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF NEW.terms_version < OLD.terms_version OR (
        NEW.terms_version = OLD.terms_version AND
        (NEW.starts_at,NEW.cancellation_terms,NEW.cancellation_cutoff_seconds,
         NEW.deposit_refund_bps,NEW.balance_refund_bps,NEW.asset_ref,NEW.deposit,NEW.balance,
         NEW.price_version,NEW.no_show_terms,NEW.chain_id) IS DISTINCT FROM
        (OLD.starts_at,OLD.cancellation_terms,OLD.cancellation_cutoff_seconds,
         OLD.deposit_refund_bps,OLD.balance_refund_bps,OLD.asset_ref,OLD.deposit,OLD.balance,
         OLD.price_version,OLD.no_show_terms,OLD.chain_id)
    ) THEN
        RAISE EXCEPTION USING ERRCODE='P0003', MESSAGE='booking terms version must advance';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER travel_slot_review_version BEFORE UPDATE ON ledger.travel_slot
FOR EACH ROW EXECUTE FUNCTION ledger.travel_slot_review_version();

CREATE OR REPLACE FUNCTION ledger.travel_hold(tenant_ uuid, branch_ uuid, actor_ uuid, request_ uuid,
    slot_ uuid, party_ integer, terms_ bigint, deposit_ text, balance_ text, asset_ text)
RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
DECLARE auth ledger.commerce_grant := ledger.commerce_allowed(tenant_,branch_,actor_);
    slot ledger.travel_slot;
    prior ledger.travel_booking;
    result uuid;
BEGIN
    IF auth.role_code <> 'customer' THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='booking scope denied';
    END IF;
    IF party_ IS NULL OR party_ NOT BETWEEN 1 AND 20 THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid party size';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch_::text||actor_::text||request_::text, 17));
    SELECT * INTO prior FROM ledger.travel_booking WHERE branch_id=branch_ AND actor_id=actor_ AND request_key=request_;
    IF FOUND THEN
        IF prior.slot_id IS DISTINCT FROM slot_ OR prior.party_size IS DISTINCT FROM party_ OR prior.terms_version IS DISTINCT FROM terms_
            OR prior.deposit::text IS DISTINCT FROM deposit_ OR prior.balance::text IS DISTINCT FROM balance_
            OR prior.asset_ref IS DISTINCT FROM asset_ THEN
            RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='booking request conflict';
        END IF;
        RETURN prior.id;
    END IF;
    SELECT * INTO slot FROM ledger.travel_slot WHERE branch_id=branch_ AND id=slot_ AND enabled FOR UPDATE;
    IF NOT FOUND OR slot.starts_at<=clock_timestamp() THEN
        RAISE EXCEPTION USING ERRCODE='P0002', MESSAGE='booking slot unavailable';
    END IF;
    IF slot.terms_version IS DISTINCT FROM terms_ OR (slot.deposit*party_)::text IS DISTINCT FROM deposit_
        OR (slot.balance*party_)::text IS DISTINCT FROM balance_ OR slot.asset_ref IS DISTINCT FROM asset_ THEN
        RAISE EXCEPTION USING ERRCODE='P0003', MESSAGE='booking terms changed';
    END IF;
    PERFORM ledger.travel_reconcile_slot(branch_,slot_);
    IF (SELECT coalesce(sum(b.party_size),0) FROM ledger.travel_booking b
        WHERE b.branch_id=branch_ AND b.slot_id=slot_ AND b.released_at IS NULL AND
            (b.expires_at>clock_timestamp() OR b.issuance_started_at IS NOT NULL)) + party_ > slot.capacity THEN
        RAISE EXCEPTION USING ERRCODE='P0004', MESSAGE='booking sold out';
    END IF;
    INSERT INTO ledger.travel_booking(branch_id,actor_id,request_key,slot_id,party_size,expires_at,terms_version,
        cancellation_terms,asset_ref,deposit,balance,cancellation_deadline,deposit_refund_bps,balance_refund_bps,
        price_version,no_show_terms,chain_id,starts_at)
    VALUES(branch_,actor_,request_,slot_,party_,least(clock_timestamp()+interval '10 minutes',slot.starts_at),
        slot.terms_version,slot.cancellation_terms,slot.asset_ref,slot.deposit*party_,slot.balance*party_,
        slot.starts_at-make_interval(secs=>slot.cancellation_cutoff_seconds),slot.deposit_refund_bps,slot.balance_refund_bps,
        slot.price_version,slot.no_show_terms,slot.chain_id,slot.starts_at)
    RETURNING id INTO result;
    RETURN result;
END $$;

CREATE OR REPLACE FUNCTION ledger.travel_start_leg(tenant_ uuid,branch_ uuid,actor_ uuid,booking_ uuid,kind_ text)
RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $leg$
DECLARE
    auth ledger.commerce_grant := ledger.commerce_allowed(tenant_,branch_,actor_);
    booking ledger.travel_booking;
    prior uuid;
    sale_ uuid;
    units numeric;
    expires timestamptz;
BEGIN
    IF auth.role_code <> 'customer' OR kind_ NOT IN ('deposit','balance') OR kind_ IS NULL THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='booking scope denied';
    END IF;
    SELECT * INTO booking FROM ledger.travel_booking
      WHERE branch_id=branch_ AND id=booking_ AND actor_id=actor_ FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE='P0002', MESSAGE='booking unavailable';
    END IF;
    SELECT sale_id INTO prior FROM ledger.travel_booking_leg
      WHERE branch_id=branch_ AND booking_id=booking_ AND kind=kind_;
    IF FOUND THEN RETURN prior; END IF;
    IF booking.cancel_requested_at IS NOT NULL OR booking.released_at IS NOT NULL THEN
        RAISE EXCEPTION USING ERRCODE='P0002', MESSAGE='booking payment unavailable';
    END IF;
    IF kind_ = 'deposit' THEN
        units := booking.deposit;
        expires := booking.expires_at;
    ELSE
        units := booking.balance;
        expires := booking.starts_at;
        IF expires IS NULL THEN
            -- Legacy bookings retained their deadline policy but did not record a service time.
            SELECT starts_at INTO STRICT expires FROM ledger.travel_slot
              WHERE branch_id=branch_ AND id=booking.slot_id;
        END IF;
        IF NOT EXISTS (SELECT FROM ledger.travel_booking_leg l JOIN ledger.payment p ON p.sale_id=l.sale_id
            WHERE l.branch_id=branch_ AND l.booking_id=booking_ AND l.kind='deposit'
              AND p.principal=booking.deposit AND p.asset_ref=booking.asset_ref
              AND p.refunded=0 AND p.in_flight=0 AND p.reserved=0) THEN
            RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='deposit not finalized or being refunded';
        END IF;
    END IF;
    IF units<=0 OR expires<=clock_timestamp() THEN
        RAISE EXCEPTION USING ERRCODE='P0002', MESSAGE='booking payment unavailable';
    END IF;
    -- Persist the fence before checkout can admit a quote signer.
    UPDATE ledger.travel_booking SET issuance_started_at=coalesce(issuance_started_at,clock_timestamp())
      WHERE branch_id=branch_ AND id=booking_;
    INSERT INTO ledger.sale(branch_id,origin,origin_ref,state,asset_ref,amount)
      VALUES(branch_,'travel',gen_random_uuid(),'preview',booking.asset_ref,units) RETURNING id INTO sale_;
    INSERT INTO ledger.order_checkout(sale_id,branch_id,creator,menu_version,expires_at,intent_digest)
      VALUES(sale_,branch_,actor_,NULL,expires,
        sha256(convert_to(jsonb_build_array('travel',booking_,kind_,booking.terms_version,booking.asset_ref,units)::text,'UTF8')));
    INSERT INTO ledger.travel_booking_leg VALUES(branch_,booking_,kind_,sale_);
    RETURN sale_;
END
$leg$;

CREATE FUNCTION ledger.travel_review_options(tenant_ uuid, branch_ uuid, actor_ uuid)
RETURNS TABLE(id uuid, starts_at timestamptz, terms_version bigint, cancellation_terms text,
              asset_ref text, deposit text, balance text, remaining integer,
              price_version bigint, no_show_terms text, chain_id integer, cancellation_deadline timestamptz)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT o.*,s.price_version,s.no_show_terms,s.chain_id,
           s.starts_at-make_interval(secs=>s.cancellation_cutoff_seconds)
    FROM ledger.travel_slots(tenant_,branch_,actor_) o
    JOIN ledger.travel_slot s ON s.branch_id=branch_ AND s.id=o.id AND s.terms_version=o.terms_version
    ORDER BY o.starts_at,o.id;
$$;

CREATE FUNCTION ledger.travel_review_owned(tenant_ uuid, branch_ uuid, actor_ uuid, booking_ uuid, request_ uuid)
RETURNS TABLE(id uuid, slot_id uuid, party_size integer, expires_at timestamptz, terms_version bigint,
              cancellation_terms text, asset_ref text, deposit text, balance text, state text,
              price_version bigint, no_show_terms text, chain_id integer, cancellation_deadline timestamptz,
              starts_at timestamptz)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT o.*,b.price_version,b.no_show_terms,b.chain_id,b.cancellation_deadline,b.starts_at
    FROM ledger.travel_owned(tenant_,branch_,actor_,booking_,request_) o
    JOIN ledger.travel_booking b ON b.branch_id=branch_ AND b.id=o.id AND b.actor_id=actor_
    ORDER BY o.expires_at DESC,o.id;
$$;

REVOKE ALL ON FUNCTION ledger.travel_review_options(uuid,uuid,uuid),
    ledger.travel_review_owned(uuid,uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.travel_review_options(uuid,uuid,uuid),
    ledger.travel_review_owned(uuid,uuid,uuid,uuid,uuid) TO nu54_p05_commerce;

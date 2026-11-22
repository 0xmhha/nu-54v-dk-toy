CREATE TABLE ledger.travel_slot (
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    starts_at timestamptz NOT NULL CHECK (isfinite(starts_at)),
    capacity integer NOT NULL CHECK (capacity BETWEEN 1 AND 500),
    terms_version bigint NOT NULL CHECK (terms_version BETWEEN 1 AND 9007199254740991),
    cancellation_terms text NOT NULL CHECK (octet_length(cancellation_terms) BETWEEN 1 AND 1000),
    asset_ref text NOT NULL CHECK (asset_ref ~ '^0x[0-9a-f]{40}$'),
    deposit ledger.units NOT NULL CHECK (deposit > 0),
    balance ledger.units NOT NULL CHECK (balance >= 0),
    enabled boolean NOT NULL DEFAULT true,
    cancellation_cutoff_seconds integer NOT NULL DEFAULT 0 CHECK (cancellation_cutoff_seconds BETWEEN 0 AND 2592000),
    deposit_refund_bps integer NOT NULL DEFAULT 0 CHECK (deposit_refund_bps BETWEEN 0 AND 10000),
    balance_refund_bps integer NOT NULL DEFAULT 0 CHECK (balance_refund_bps BETWEEN 0 AND 10000),
    CHECK ((deposit+balance)*capacity < power(2::numeric,256)),
    PRIMARY KEY (branch_id,id)
);

CREATE TABLE ledger.travel_booking (
    branch_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    actor_id uuid NOT NULL,
    request_key uuid NOT NULL,
    slot_id uuid NOT NULL,
    party_size integer NOT NULL CHECK (party_size BETWEEN 1 AND 20),
    expires_at timestamptz NOT NULL CHECK (isfinite(expires_at)),
    terms_version bigint NOT NULL CHECK (terms_version BETWEEN 1 AND 9007199254740991),
    cancellation_terms text NOT NULL,
    asset_ref text NOT NULL,
    deposit ledger.units NOT NULL CHECK (deposit>0),
    balance ledger.units NOT NULL CHECK (balance>=0),
    issuance_started_at timestamptz CHECK (isfinite(issuance_started_at)),
    cancellation_deadline timestamptz NOT NULL CHECK (isfinite(cancellation_deadline)),
    deposit_refund_bps integer NOT NULL CHECK (deposit_refund_bps BETWEEN 0 AND 10000),
    balance_refund_bps integer NOT NULL CHECK (balance_refund_bps BETWEEN 0 AND 10000),
    cancel_requested_at timestamptz CHECK (isfinite(cancel_requested_at)),
    released_at timestamptz CHECK (isfinite(released_at)),
    PRIMARY KEY (branch_id,id),
    FOREIGN KEY (branch_id,slot_id) REFERENCES ledger.travel_slot(branch_id,id),
    FOREIGN KEY (branch_id,actor_id) REFERENCES ledger.commerce_grant(branch_id,actor_id),
    UNIQUE (branch_id,actor_id,request_key)
);
CREATE INDEX travel_capacity ON ledger.travel_booking(branch_id,slot_id,expires_at);

CREATE FUNCTION ledger.travel_slots(tenant_ uuid, branch_ uuid, actor_ uuid)
RETURNS TABLE(id uuid, starts_at timestamptz, terms_version bigint, cancellation_terms text,
              asset_ref text, deposit text, balance text, remaining integer)
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
DECLARE auth ledger.commerce_grant := ledger.commerce_allowed(tenant_,branch_,actor_);
BEGIN
    IF auth.role_code <> 'customer' THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='booking scope denied';
    END IF;
    RETURN QUERY
        SELECT s.id,s.starts_at,s.terms_version,s.cancellation_terms,s.asset_ref,s.deposit::text,s.balance::text,
            greatest(0,s.capacity-(SELECT coalesce(sum(b.party_size),0) FROM ledger.travel_booking b
                WHERE b.branch_id=s.branch_id AND b.slot_id=s.id AND b.released_at IS NULL AND
                    (b.expires_at>statement_timestamp() OR b.issuance_started_at IS NOT NULL)))::integer
        FROM ledger.travel_slot s
        WHERE s.branch_id=branch_ AND s.enabled AND s.starts_at>statement_timestamp()
        ORDER BY s.starts_at,s.id LIMIT 50;
END $$;

CREATE FUNCTION ledger.travel_hold(tenant_ uuid, branch_ uuid, actor_ uuid, request_ uuid,
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
        cancellation_terms,asset_ref,deposit,balance,cancellation_deadline,deposit_refund_bps,balance_refund_bps)
    VALUES(branch_,actor_,request_,slot_,party_,least(clock_timestamp()+interval '10 minutes',slot.starts_at),
        slot.terms_version,slot.cancellation_terms,slot.asset_ref,slot.deposit*party_,slot.balance*party_,
        slot.starts_at-make_interval(secs=>slot.cancellation_cutoff_seconds),slot.deposit_refund_bps,slot.balance_refund_bps)
    RETURNING id INTO result;
    RETURN result;
END $$;

CREATE FUNCTION ledger.travel_owned(tenant_ uuid, branch_ uuid, actor_ uuid, booking_ uuid, request_ uuid)
RETURNS TABLE(id uuid, slot_id uuid, party_size integer, expires_at timestamptz, terms_version bigint, cancellation_terms text,
              asset_ref text, deposit text, balance text, state text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
DECLARE auth ledger.commerce_grant := ledger.commerce_allowed(tenant_,branch_,actor_);
BEGIN
    IF auth.role_code <> 'customer' THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='booking scope denied';
    END IF;
    RETURN QUERY SELECT b.id,b.slot_id,b.party_size,b.expires_at,b.terms_version,b.cancellation_terms,b.asset_ref,b.deposit::text,
        b.balance::text,CASE WHEN b.cancel_requested_at IS NOT NULL AND b.released_at IS NOT NULL THEN 'cancelled'
            WHEN b.cancel_requested_at IS NOT NULL THEN 'cancelling'
            WHEN b.released_at IS NOT NULL THEN 'expired'
            WHEN b.issuance_started_at IS NOT NULL THEN 'pending_payment'
            WHEN b.expires_at<=statement_timestamp() THEN 'expired' ELSE 'held' END
        FROM ledger.travel_booking b WHERE b.branch_id=branch_ AND b.actor_id=actor_
            AND (booking_ IS NULL OR b.id=booking_) AND (request_ IS NULL OR b.request_key=request_)
        ORDER BY b.expires_at DESC,b.id LIMIT 50;
END $$;

REVOKE ALL ON FUNCTION ledger.travel_slots(uuid,uuid,uuid),
    ledger.travel_hold(uuid,uuid,uuid,uuid,uuid,integer,bigint,text,text,text),
    ledger.travel_owned(uuid,uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.travel_slots(uuid,uuid,uuid),
    ledger.travel_hold(uuid,uuid,uuid,uuid,uuid,integer,bigint,text,text,text),
    ledger.travel_owned(uuid,uuid,uuid,uuid,uuid) TO nu54_p05_commerce;

ALTER TABLE ledger.sale DROP CONSTRAINT sale_origin_check;
ALTER TABLE ledger.sale ADD CHECK (origin IN ('menu','pos','subscription','travel'));

CREATE TABLE ledger.travel_booking_leg (
    branch_id uuid NOT NULL,
    booking_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('deposit','balance')),
    sale_id uuid NOT NULL UNIQUE,
    PRIMARY KEY (branch_id,booking_id,kind),
    FOREIGN KEY (branch_id,booking_id) REFERENCES ledger.travel_booking(branch_id,id),
    FOREIGN KEY (branch_id,sale_id) REFERENCES ledger.sale(branch_id,id)
);

CREATE FUNCTION ledger.travel_start_leg(tenant_ uuid,branch_ uuid,actor_ uuid,booking_ uuid,kind_ text)
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
        SELECT starts_at INTO STRICT expires FROM ledger.travel_slot
          WHERE branch_id=branch_ AND id=booking.slot_id;
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

CREATE FUNCTION ledger.travel_legs(tenant_ uuid,branch_ uuid,actor_ uuid,booking_ uuid)
RETURNS TABLE(kind text, sale_id uuid, state text, amount text, receipt_ref uuid, refunded text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $legs$
DECLARE auth ledger.commerce_grant := ledger.commerce_allowed(tenant_,branch_,actor_);
BEGIN
    IF auth.role_code <> 'customer' OR NOT EXISTS (SELECT FROM ledger.travel_booking b
        WHERE b.branch_id=branch_ AND b.actor_id=actor_ AND b.id=booking_) THEN
        RAISE EXCEPTION USING ERRCODE='P0002', MESSAGE='booking unavailable';
    END IF;
    RETURN QUERY
      SELECT l.kind,s.id,s.state,s.amount::text,j.id,coalesce(p.refunded,0)::text
      FROM ledger.travel_booking_leg l JOIN ledger.sale s ON (s.branch_id,s.id)=(l.branch_id,l.sale_id)
      LEFT JOIN ledger.payment p ON p.sale_id=s.id
      LEFT JOIN ledger.journal j ON j.sale_id=p.sale_id AND j.refund_id IS NULL
      WHERE l.branch_id=branch_ AND l.booking_id=booking_
      ORDER BY CASE l.kind WHEN 'deposit' THEN 0 ELSE 1 END;
END
$legs$;

REVOKE ALL ON FUNCTION ledger.travel_start_leg(uuid,uuid,uuid,uuid,text),
    ledger.travel_legs(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.travel_start_leg(uuid,uuid,uuid,uuid,text),
    ledger.travel_legs(uuid,uuid,uuid,uuid) TO nu54_p05_commerce;

-- Reservation payment keeps the original posting and evidence without creating a pickup.
CREATE OR REPLACE FUNCTION ledger.confirm_payment(sale_ uuid, quote_ bytea, stream_ uuid, evidence_ uuid, tx_ text,
    index_ integer, block_ text, included_ timestamptz, asset_ text, amount_ numeric, payer_ text,
    generation_ bigint, mode_ integer, merchant_ text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $pay$
DECLARE
    s ledger.sale;
    q ledger.quote_work;
    prior ledger.payment;
    day_ date;
    pickup_no bigint;
BEGIN
    SELECT * INTO STRICT s FROM ledger.sale WHERE id=sale_ FOR UPDATE;
    SELECT * INTO STRICT q FROM ledger.quote_work WHERE sale_id=sale_;
    IF quote_ IS DISTINCT FROM q.signed_digest OR q.signed_digest IS NULL OR
       (amount_,asset_,payer_,generation_,mode_) IS DISTINCT FROM
       (s.amount::numeric,s.asset_ref,'0x'||encode(q.payer,'hex'),q.payer_generation,1) OR
       included_ IS NULL OR included_<date_trunc('second',q.issued_at) OR included_>=date_trunc('second',q.expires_at) THEN
        RAISE EXCEPTION 'finalized payment does not match issued instruction' USING ERRCODE='P1001';
    END IF;
    SELECT * INTO prior FROM ledger.payment WHERE sale_id=sale_;
    IF FOUND THEN
        IF (prior.stream_id,prior.tx_hash,prior.log_index,prior.inclusion_hash,prior.included_at,prior.merchant_key)
            IS DISTINCT FROM (stream_,tx_,index_,block_,included_,merchant_) THEN
            RAISE EXCEPTION 'conflicting finalized payment identity' USING ERRCODE='P1001';
        END IF;
        RETURN false;
    END IF;
    IF s.state NOT IN ('issued','checking') OR EXISTS (SELECT FROM ledger.sale_resolution WHERE sale_id=sale_) THEN
        RAISE EXCEPTION 'order cannot accept payment' USING ERRCODE='P1002';
    END IF;
    IF EXISTS (SELECT FROM ledger.settlement_submission op WHERE op.sale_id=sale_ AND op.stream_id<>stream_) THEN
        RAISE EXCEPTION 'finalized stream differs' USING ERRCODE='P1001';
    END IF;
    day_:=ledger.ledger_business_day(s.branch_id,included_);
    PERFORM pg_advisory_xact_lock(hashtextextended(s.branch_id::text||day_::text||asset_,10));
    INSERT INTO ledger.payment(sale_id,stream_id,merchant_key,evidence_id,tx_hash,log_index,inclusion_hash,included_at,asset_ref,
        payer,payer_generation,payment_mode,principal)
        VALUES(sale_,stream_,merchant_,evidence_,tx_,index_,block_,included_,asset_,payer_,generation_,mode_,amount_);
    INSERT INTO ledger.journal(sale_id,evidence_id,branch_id,business_day,asset_ref,amount,debit_account,credit_account,booked_at)
        VALUES(sale_,evidence_,s.branch_id,day_,asset_,amount_,'settlement_clearing','sales',clock_timestamp());
    IF s.origin<>'travel' THEN
        INSERT INTO ledger.pickup_counter VALUES(s.branch_id,day_,1)
            ON CONFLICT(branch_id,business_day) DO UPDATE SET next_number=ledger.pickup_counter.next_number+1
            RETURNING next_number INTO pickup_no;
        INSERT INTO ledger.pickup(sale_id,branch_id,business_day,number) VALUES(sale_,s.branch_id,day_,pickup_no);
    END IF;
    UPDATE ledger.quote_work SET consumed_at=clock_timestamp() WHERE sale_id=sale_;
    UPDATE ledger.sale SET state='paid',revision=revision+1 WHERE id=sale_;
    IF s.origin<>'travel' THEN
            INSERT INTO ledger.fulfillment_outbox(sale_id,kind,source_ref,revision) VALUES(sale_,'paid',sale_,s.revision+1);
    END IF;
    RETURN true;
END
$pay$;

CREATE OR REPLACE FUNCTION ledger.confirm_refund(refund_ uuid, stream_ uuid, evidence_ uuid, tx_ text,index_ integer,
    block_ text,included_ timestamptz,asset_ text,amount_ numeric,payer_ text,mode_ integer,chain_refund_ bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $refund$
DECLARE
    r ledger.refund;
    p ledger.payment;
    s ledger.sale;
    prior ledger.refund_confirmation;
    refund_day date;
BEGIN
    SELECT * INTO STRICT r FROM ledger.refund WHERE id=refund_;
    SELECT * INTO STRICT s FROM ledger.sale WHERE id=r.sale_id FOR UPDATE;
    SELECT * INTO STRICT p FROM ledger.payment WHERE sale_id=r.sale_id FOR UPDATE;
    SELECT * INTO STRICT r FROM ledger.refund WHERE id=refund_ FOR UPDATE;
    IF (stream_,asset_,amount_,payer_,mode_,chain_refund_) IS DISTINCT FROM
        (p.stream_id,p.asset_ref,r.amount::numeric,p.payer,p.payment_mode,r.chain_refund_id)
        OR included_ IS NULL OR included_<p.included_at THEN
        RAISE EXCEPTION 'refund observation differs from original route or amount' USING ERRCODE='P1001';
    END IF;
    SELECT * INTO prior FROM ledger.refund_confirmation WHERE refund_id=refund_;
    IF FOUND THEN
        IF (prior.tx_hash,prior.log_index,prior.inclusion_hash,prior.included_at) IS DISTINCT FROM (tx_,index_,block_,included_) THEN
            RAISE EXCEPTION 'conflicting finalized refund' USING ERRCODE='P1001';
        END IF;
        RETURN false;
    END IF;
    IF r.state<>'inflight' OR r.approver IS NULL THEN
        RAISE EXCEPTION 'refund not submitted under approved route' USING ERRCODE='P1002';
    END IF;
    INSERT INTO ledger.refund_confirmation VALUES(refund_,stream_,evidence_,tx_,index_,block_,included_);
    UPDATE ledger.payment SET in_flight=in_flight-r.amount,refunded=refunded+r.amount WHERE sale_id=r.sale_id;
    UPDATE ledger.refund SET state='final' WHERE id=refund_;
    refund_day:=ledger.ledger_business_day(s.branch_id,included_);
    PERFORM pg_advisory_xact_lock(hashtextextended(s.branch_id::text||refund_day::text||asset_,10));
    INSERT INTO ledger.journal(sale_id,refund_id,evidence_id,branch_id,business_day,asset_ref,amount,debit_account,credit_account,booked_at)
        VALUES(r.sale_id,refund_,evidence_,s.branch_id,refund_day,asset_,amount_,
          'sales_returns','settlement_clearing',clock_timestamp());
    UPDATE ledger.sale SET revision=revision+1 WHERE id=s.id;
    IF s.origin<>'travel' THEN
        INSERT INTO ledger.fulfillment_outbox(sale_id,kind,source_ref,revision)
            VALUES(s.id,'refunded',refund_,s.revision+1);
    END IF;
    RETURN true;
END
$refund$;


CREATE FUNCTION ledger.travel_cancel(tenant_ uuid,branch_ uuid,actor_ uuid,booking_ uuid)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $cancel$
DECLARE
    auth ledger.commerce_grant := ledger.commerce_allowed(tenant_,branch_,actor_);
    booking ledger.travel_booking;
BEGIN
    IF auth.role_code <> 'customer' THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='booking scope denied';
    END IF;
    SELECT * INTO booking FROM ledger.travel_booking
      WHERE branch_id=branch_ AND id=booking_ AND actor_id=actor_ FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE='P0002', MESSAGE='booking unavailable';
    END IF;
    IF booking.cancel_requested_at IS NOT NULL THEN RETURN booking_; END IF;
    IF booking.released_at IS NOT NULL OR booking.cancellation_deadline<=clock_timestamp() THEN
        RAISE EXCEPTION USING ERRCODE='P0002', MESSAGE='booking cancellation unavailable';
    END IF;
    UPDATE ledger.travel_booking b SET cancel_requested_at=clock_timestamp()
      WHERE b.branch_id=branch_ AND b.id=booking_;
    RETURN booking_;
END
$cancel$;

CREATE FUNCTION ledger.travel_quote_guard()
RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $guard$
DECLARE booking ledger.travel_booking;
BEGIN
    SELECT b.* INTO booking FROM ledger.travel_booking_leg l
     JOIN ledger.travel_booking b ON (b.branch_id,b.id)=(l.branch_id,l.booking_id)
     WHERE l.sale_id=NEW.sale_id FOR SHARE OF b;
    IF FOUND AND (booking.cancel_requested_at IS NOT NULL OR booking.released_at IS NOT NULL) THEN
        RAISE EXCEPTION USING ERRCODE='P0702', MESSAGE='booking payment unavailable';
    END IF;
    RETURN NEW;
END
$guard$;
CREATE TRIGGER travel_quote_guard BEFORE INSERT OR UPDATE OF signed_digest, fence ON ledger.quote_work
FOR EACH ROW EXECUTE FUNCTION ledger.travel_quote_guard();

CREATE FUNCTION ledger.travel_refund_guard()
RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $guard$
DECLARE
    policy record;
    p ledger.payment;
    maximum numeric;
BEGIN
    SELECT b.cancel_requested_at,b.cancellation_deadline,b.deposit_refund_bps,b.balance_refund_bps,l.kind INTO policy
     FROM ledger.travel_booking_leg l
     JOIN ledger.travel_booking b ON (b.branch_id,b.id)=(l.branch_id,l.booking_id)
     WHERE l.sale_id=NEW.sale_id;
    IF NOT FOUND THEN RETURN NEW; END IF;
    SELECT * INTO STRICT p FROM ledger.payment WHERE sale_id=NEW.sale_id;
    maximum := trunc(p.principal * CASE policy.kind WHEN 'deposit' THEN policy.deposit_refund_bps
        ELSE policy.balance_refund_bps END / 10000);
    IF policy.cancel_requested_at IS NULL OR policy.cancel_requested_at>=policy.cancellation_deadline
       OR p.reserved+p.in_flight+p.refunded>maximum THEN
       RAISE EXCEPTION USING ERRCODE='P1003', MESSAGE='refund exceeds accepted reservation policy';
    END IF;
    RETURN NEW;
END
$guard$;
CREATE TRIGGER travel_refund_guard BEFORE INSERT ON ledger.refund
FOR EACH ROW EXECUTE FUNCTION ledger.travel_refund_guard();

CREATE FUNCTION ledger.travel_reconcile_slot(branch_ uuid,slot_ uuid) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $reconcile$
DECLARE booking ledger.travel_booking;
BEGIN
    -- Callers hold the slot row. A booking lock excludes new legs and quote issuance.
    FOR booking IN SELECT * FROM ledger.travel_booking b WHERE b.branch_id=branch_ AND b.slot_id=slot_ AND b.released_at IS NULL
      AND (b.cancel_requested_at IS NOT NULL OR b.expires_at<=clock_timestamp()) ORDER BY b.id FOR UPDATE LOOP
        IF booking.issuance_started_at IS NOT NULL AND NOT EXISTS (
            SELECT FROM ledger.travel_booking_leg l WHERE (l.branch_id,l.booking_id)=(branch_,booking.id)
        ) THEN
            CONTINUE;
        END IF;
        IF EXISTS (
          SELECT FROM ledger.travel_booking_leg l
          JOIN ledger.sale s ON (s.branch_id,s.id)=(l.branch_id,l.sale_id)
          LEFT JOIN ledger.quote_work q ON q.sale_id=s.id
          LEFT JOIN ledger.sale_resolution resolution ON resolution.sale_id=s.id
          LEFT JOIN ledger.payment p ON p.sale_id=s.id
          WHERE (l.branch_id,l.booking_id)=(branch_,booking.id)
          AND NOT (
            (s.state='closed' AND p.sale_id IS NULL AND (q.sale_id IS NULL OR resolution.sale_id IS NOT NULL))
            OR (booking.cancel_requested_at IS NOT NULL AND s.state='paid' AND p.sale_id IS NOT NULL
                AND p.reserved=0 AND p.in_flight=0
                AND p.refunded=trunc(p.principal * CASE l.kind WHEN 'deposit' THEN booking.deposit_refund_bps
                    ELSE booking.balance_refund_bps END / 10000))
          )
        ) THEN
            CONTINUE;
        END IF;
        UPDATE ledger.travel_booking SET released_at=clock_timestamp() WHERE branch_id=branch_ AND id=booking.id;
    END LOOP;
END
$reconcile$;

CREATE FUNCTION ledger.travel_reconcile_owned(tenant_ uuid,branch_ uuid,actor_ uuid,booking_ uuid) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $scope$
DECLARE auth ledger.commerce_grant := ledger.commerce_allowed(tenant_,branch_,actor_);
    slot_ uuid;
BEGIN
    IF auth.role_code <> 'customer' THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='booking scope denied';
    END IF;
    SELECT b.slot_id INTO slot_ FROM ledger.travel_booking b WHERE b.branch_id=branch_ AND b.id=booking_ AND b.actor_id=actor_;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0002', MESSAGE='booking unavailable'; END IF;
    PERFORM 1 FROM ledger.travel_slot s WHERE s.branch_id=branch_ AND s.id=slot_ FOR UPDATE;
    PERFORM ledger.travel_reconcile_slot(branch_,slot_);
END
$scope$;

REVOKE ALL ON FUNCTION ledger.travel_cancel(uuid,uuid,uuid,uuid),
    ledger.travel_quote_guard(),ledger.travel_refund_guard(),ledger.travel_reconcile_slot(uuid,uuid),
    ledger.travel_reconcile_owned(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.travel_cancel(uuid,uuid,uuid,uuid),
    ledger.travel_reconcile_owned(uuid,uuid,uuid,uuid) TO nu54_p05_commerce;

-- Whole won outside the menu (owner decision 2026-11-27). opsd already refuses a part of a won in menu prices; the
-- ledger did not know which asset is won, so booking terms, subscription amounts and refunds could carry one, and a
-- booking refund was cut off below the base unit (50 % of 33,333 won = 16,666.50 won).
--
-- opsd now tells the ledger what one won of each won asset is. For such an asset the ledger takes new booking terms,
-- subscription amounts and refunds in whole won only, and what a cancelled booking gets back is rounded up to a whole
-- won, never above what was paid. An asset nobody declared (a dollar token) keeps every rule it had.
--
-- Rows stored before this keep their amounts: only a new or changed amount is checked. A refund of an older payment
-- is rounded up like any other, and where that would pass what was paid it is all that is left of the payment.

CREATE TABLE ledger.whole_amount_asset (
    asset_ref text PRIMARY KEY CHECK (asset_ref ~ '^0x[0-9a-f]{40}$'),
    -- One won in base units: ten to the power of the token's decimals (1 to 18).
    unit numeric NOT NULL CHECK (unit::text ~ '^10{1,18}$')
);
REVOKE ALL ON ledger.whole_amount_asset FROM PUBLIC, nu54_p05_commerce;

-- Written once. Another unit for the same asset would change what was already agreed, so it is refused.
CREATE FUNCTION ledger.declare_whole_amount(asset_ text,unit_ numeric) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $declare$
BEGIN
    INSERT INTO ledger.whole_amount_asset VALUES(asset_,unit_) ON CONFLICT (asset_ref) DO NOTHING;
    IF NOT EXISTS (SELECT FROM ledger.whole_amount_asset a WHERE a.asset_ref=asset_ AND a.unit=unit_) THEN
        RAISE EXCEPTION 'the whole unit of an asset does not change' USING ERRCODE='P1001';
    END IF;
END
$declare$;
REVOKE ALL ON FUNCTION ledger.declare_whole_amount(text,numeric) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.declare_whole_amount(text,numeric) TO nu54_p05_commerce;

-- One for an asset that is not won: every amount in base units is a whole amount of it.
CREATE FUNCTION ledger.whole_unit(asset_ text) RETURNS numeric
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $unit$
    SELECT coalesce((SELECT a.unit FROM ledger.whole_amount_asset a WHERE a.asset_ref=asset_),1)
$unit$;
REVOKE ALL ON FUNCTION ledger.whole_unit(text) FROM PUBLIC;

-- What a cancelled booking gets back of one paid leg. Won: rounded up to a whole won, and never more than was paid.
-- Any other asset: cut off at the base unit, as before.
CREATE FUNCTION ledger.travel_refund_due(principal_ numeric,bps_ integer,asset_ text) RETURNS numeric
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $due$
    SELECT CASE WHEN whole.unit=1 THEN trunc(principal_ * bps_ / 10000)
        ELSE least(principal_,div(principal_ * bps_ + 10000*whole.unit - 1,10000*whole.unit) * whole.unit) END
    FROM (SELECT ledger.whole_unit(asset_) AS unit) whole
$due$;
REVOKE ALL ON FUNCTION ledger.travel_refund_due(numeric,integer,text) FROM PUBLIC;

-- The four places that computed the booking refund now share travel_refund_due; nothing else in them changes.
CREATE OR REPLACE FUNCTION ledger.travel_refund_guard()
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
    maximum := ledger.travel_refund_due(p.principal,CASE policy.kind WHEN 'deposit' THEN policy.deposit_refund_bps
        ELSE policy.balance_refund_bps END,p.asset_ref);
    IF policy.cancel_requested_at IS NULL OR policy.cancel_requested_at>=policy.cancellation_deadline
       OR p.reserved+p.in_flight+p.refunded>maximum THEN
       RAISE EXCEPTION USING ERRCODE='P1003', MESSAGE='refund exceeds accepted reservation policy';
    END IF;
    RETURN NEW;
END
$guard$;

CREATE OR REPLACE FUNCTION ledger.travel_reconcile_slot(branch_ uuid,slot_ uuid) RETURNS void
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
                AND p.refunded=ledger.travel_refund_due(p.principal,CASE l.kind WHEN 'deposit' THEN booking.deposit_refund_bps
                    ELSE booking.balance_refund_bps END,p.asset_ref))
          )
        ) THEN
            CONTINUE;
        END IF;
        UPDATE ledger.travel_booking SET released_at=clock_timestamp() WHERE branch_id=branch_ AND id=booking.id;
    END LOOP;
END
$reconcile$;

CREATE OR REPLACE FUNCTION ledger.travel_legs_with_refunds(tenant_ uuid,branch_ uuid,actor_ uuid,booking_ uuid)
RETURNS TABLE(kind text,sale_id uuid,state text,amount text,receipt_ref uuid,refunded text,
              refund_expected text,refund_state text)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $progress$
    SELECT leg.kind,leg.sale_id,leg.state,leg.amount,leg.receipt_ref,leg.refunded,expected.amount::text,
        CASE WHEN booking.cancel_requested_at IS NULL THEN 'not-requested'
             WHEN leg.state='closed' AND paid.sale_id IS NULL THEN 'not-due'
             WHEN expected.amount IS NULL THEN 'waiting-payment'
             WHEN expected.amount=0 THEN 'not-due'
             WHEN paid.refunded=expected.amount THEN 'settled'
             ELSE 'waiting-refund' END
    FROM ledger.travel_legs(tenant_,branch_,actor_,booking_) leg
    JOIN ledger.travel_booking booking ON booking.branch_id=branch_ AND booking.id=booking_ AND booking.actor_id=actor_
    LEFT JOIN ledger.payment paid ON paid.sale_id=leg.sale_id
    CROSS JOIN LATERAL (SELECT CASE WHEN booking.cancel_requested_at IS NOT NULL THEN
        ledger.travel_refund_due(paid.principal,CASE leg.kind WHEN 'deposit' THEN booking.deposit_refund_bps
            ELSE booking.balance_refund_bps END,paid.asset_ref)
    END AS amount) expected
    ORDER BY CASE leg.kind WHEN 'deposit' THEN 0 ELSE 1 END
$progress$;

CREATE OR REPLACE FUNCTION ledger.travel_cancellation_preview(tenant_ uuid,branch_ uuid,actor_ uuid,booking_ uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $preview$
DECLARE
    auth ledger.commerce_grant := ledger.commerce_allowed(tenant_,branch_,actor_);
    booking ledger.travel_booking;
    can_cancel boolean;
    legs jsonb;
BEGIN
    IF auth.role_code <> 'customer' THEN
        RAISE EXCEPTION USING ERRCODE='P0002', MESSAGE='booking unavailable';
    END IF;
    SELECT * INTO booking FROM ledger.travel_booking
      WHERE branch_id=branch_ AND actor_id=actor_ AND id=booking_;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE='P0002', MESSAGE='booking unavailable';
    END IF;
    can_cancel := booking.cancel_requested_at IS NULL AND booking.released_at IS NULL
        AND booking.cancellation_deadline > clock_timestamp();
    SELECT coalesce(jsonb_agg(jsonb_strip_nulls(jsonb_build_object(
        'kind',l.kind,'saleRef',l.sale_id,'paymentState',s.state,
        'refundExpected',CASE WHEN can_cancel AND s.state='paid' THEN
          ledger.travel_refund_due(p.principal,CASE l.kind WHEN 'deposit' THEN booking.deposit_refund_bps
             ELSE booking.balance_refund_bps END,p.asset_ref)::text END))
        ORDER BY CASE l.kind WHEN 'deposit' THEN 0 ELSE 1 END), '[]'::jsonb)
      INTO legs
      FROM ledger.travel_booking_leg l JOIN ledger.sale s ON (s.branch_id,s.id)=(l.branch_id,l.sale_id)
      LEFT JOIN ledger.payment p ON p.sale_id=l.sale_id
      WHERE (l.branch_id,l.booking_id)=(branch_,booking_);
    RETURN jsonb_build_object('canCancel',can_cancel,'cancellationDeadline',booking.cancellation_deadline,'legs',legs);
END
$preview$;

-- New booking terms: what a slot offers, and what a hold copies from it. A slot stored with a part of a won keeps
-- it until its amounts change, and takes no new hold.
CREATE FUNCTION ledger.whole_travel_terms() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $guard$
DECLARE unit numeric := ledger.whole_unit(NEW.asset_ref);
BEGIN
    IF TG_OP='UPDATE' THEN
        IF (NEW.asset_ref,NEW.deposit,NEW.balance) IS NOT DISTINCT FROM (OLD.asset_ref,OLD.deposit,OLD.balance) THEN RETURN NEW; END IF;
    END IF;
    IF NEW.deposit % unit <> 0 OR NEW.balance % unit <> 0 THEN
        RAISE EXCEPTION 'booking amounts of a won asset are whole won' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END
$guard$;
REVOKE ALL ON FUNCTION ledger.whole_travel_terms() FROM PUBLIC;
CREATE TRIGGER whole_travel_terms BEFORE INSERT OR UPDATE OF asset_ref,deposit,balance ON ledger.travel_slot
    FOR EACH ROW EXECUTE FUNCTION ledger.whole_travel_terms();
CREATE TRIGGER whole_travel_terms BEFORE INSERT ON ledger.travel_booking
    FOR EACH ROW EXECUTE FUNCTION ledger.whole_travel_terms();

-- New subscription amounts: what an offer charges each cycle, and what an acceptance copies from it. A subscription
-- accepted before keeps its amount.
CREATE FUNCTION ledger.whole_subscription_amount() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $guard$
BEGIN
    IF TG_OP='UPDATE' THEN
        IF (NEW.asset_ref,NEW.amount) IS NOT DISTINCT FROM (OLD.asset_ref,OLD.amount) THEN RETURN NEW; END IF;
    END IF;
    IF NEW.amount % ledger.whole_unit(NEW.asset_ref) <> 0 THEN
        RAISE EXCEPTION 'a subscription amount of a won asset is whole won' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END
$guard$;
REVOKE ALL ON FUNCTION ledger.whole_subscription_amount() FROM PUBLIC;
CREATE TRIGGER whole_subscription_amount BEFORE INSERT OR UPDATE OF asset_ref,amount ON ledger.subscription_offer
    FOR EACH ROW EXECUTE FUNCTION ledger.whole_subscription_amount();
CREATE TRIGGER whole_subscription_amount BEFORE INSERT OR UPDATE OF asset_ref,amount ON ledger.subscription
    FOR EACH ROW EXECUTE FUNCTION ledger.whole_subscription_amount();

-- Every refund is inserted here, after its amount was added to the reserved principal. A won refund is whole won.
-- The one exception takes all that is left of a payment: that is the cap, and only a payment stored with a part of
-- a won can leave such a rest.
CREATE FUNCTION ledger.whole_refund_guard() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $guard$
DECLARE p ledger.payment;
BEGIN
    SELECT * INTO STRICT p FROM ledger.payment WHERE sale_id=NEW.sale_id;
    IF NEW.amount % ledger.whole_unit(p.asset_ref) <> 0 AND p.reserved+p.in_flight+p.refunded<>p.principal THEN
        RAISE EXCEPTION 'a refund of a won asset is whole won' USING ERRCODE='P1030';
    END IF;
    RETURN NEW;
END
$guard$;
REVOKE ALL ON FUNCTION ledger.whole_refund_guard() FROM PUBLIC;
CREATE TRIGGER whole_refund_guard BEFORE INSERT ON ledger.refund
    FOR EACH ROW EXECUTE FUNCTION ledger.whole_refund_guard();

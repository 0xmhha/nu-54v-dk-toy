CREATE TABLE ledger.public_menu_cancellation (
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    terminal_ref uuid NOT NULL,
    session_ref uuid NOT NULL,
    reserve_key uuid NOT NULL,
    ended_at timestamptz NOT NULL CHECK(isfinite(ended_at)),
    PRIMARY KEY(branch_id,terminal_ref,session_ref),
    UNIQUE(branch_id,terminal_ref,reserve_key)
);
REVOKE ALL ON ledger.public_menu_cancellation FROM PUBLIC,nu54_p05_commerce;

-- The existing reserve transaction holds both its request and origin
-- locks. Fencing the receipt rolls back that same transaction's stock writes
-- and leaves the previous reservation/retry implementation intact.
CREATE FUNCTION ledger.fence_ended_public_menu()
RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF NEW.operation='create_sale' AND EXISTS (
        SELECT FROM ledger.sale s JOIN ledger.public_menu_cancellation x
          ON x.branch_id=s.branch_id AND x.terminal_ref=NEW.scope_ref
          AND (x.session_ref=s.origin_ref OR x.reserve_key=NEW.request_key)
        WHERE s.id=NEW.result_ref AND s.branch_id=NEW.branch_id AND s.origin='menu') THEN
        RAISE EXCEPTION USING ERRCODE='P0502', MESSAGE='public checkout session ended';
    END IF;
    RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION ledger.fence_ended_public_menu() FROM PUBLIC;
CREATE TRIGGER menu_reservation_cancellation_fence BEFORE INSERT ON ledger.command_receipt
    FOR EACH ROW EXECUTE FUNCTION ledger.fence_ended_public_menu();

CREATE FUNCTION ledger.cancel_public_menu_order(tenant uuid,branch uuid,actor uuid,session_ uuid,reserve_ uuid)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE grant_ ledger.commerce_grant := ledger.commerce_allowed(tenant,branch,actor);
DECLARE submitted ledger.sale;
DECLARE prior ledger.public_menu_cancellation;
DECLARE receipt_sale uuid;
BEGIN
    IF grant_.role_code IS DISTINCT FROM 'terminal' OR session_ IS NULL OR reserve_ IS NULL THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='public checkout scope denied';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||actor::text||'create_sale'||reserve_::text,0));
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||'menu'||session_::text,1));
    IF grant_.valid_until<=clock_timestamp() THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='public checkout scope expired';
    END IF;
    SELECT * INTO prior FROM ledger.public_menu_cancellation x
      WHERE x.branch_id=branch AND x.terminal_ref=actor AND (x.session_ref=session_ OR x.reserve_key=reserve_);
    IF FOUND THEN
        IF (prior.session_ref,prior.reserve_key) IS DISTINCT FROM (session_,reserve_) THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='public checkout cancellation binding changed';
        END IF;
        RETURN;
    END IF;
    SELECT * INTO submitted FROM ledger.sale s WHERE s.branch_id=branch AND s.origin='menu' AND s.origin_ref=session_ FOR UPDATE;
    SELECT r.result_ref INTO receipt_sale FROM ledger.command_receipt r
      WHERE r.branch_id=branch AND r.scope_ref=actor AND r.operation='create_sale' AND r.request_key=reserve_;
    IF submitted.id IS NOT NULL AND (receipt_sale IS DISTINCT FROM submitted.id OR NOT EXISTS(
        SELECT FROM ledger.order_checkout c WHERE (c.branch_id,c.sale_id,c.creator)=(branch,submitted.id,actor))) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='public checkout scope denied';
    END IF;
    IF submitted.id IS NULL AND receipt_sale IS NOT NULL THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='public checkout cancellation binding changed';
    END IF;
    IF grant_.valid_until<=clock_timestamp() THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='public checkout scope expired';
    END IF;
    INSERT INTO ledger.public_menu_cancellation VALUES(branch,actor,session_,reserve_,clock_timestamp());

    -- Claim and issuance both use the sale lock. A phone with an already
    -- committed claim retains its original order, even if no quote exists yet.
    IF submitted.state='preview' AND NOT EXISTS (
         SELECT FROM ledger.checkout_claim c WHERE c.branch_id=branch AND c.sale_id=submitted.id)
       AND NOT EXISTS (SELECT FROM ledger.quote_work q WHERE q.sale_id=submitted.id AND q.signed_digest IS NOT NULL) THEN
        PERFORM ledger.restore_order_stock(branch,submitted.id);
        UPDATE ledger.sale SET state='closed',revision=revision+1 WHERE id=submitted.id;
    END IF;
END $$;
REVOKE ALL ON FUNCTION ledger.cancel_public_menu_order(uuid,uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.cancel_public_menu_order(uuid,uuid,uuid,uuid,uuid) TO nu54_p05_commerce;

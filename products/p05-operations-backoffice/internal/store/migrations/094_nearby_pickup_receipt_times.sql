-- Preserve the first authenticated ready timestamp across later fulfillment.
-- Existing transitions have no durable original timestamp and remain NULL.
ALTER TABLE ledger.travel_pickup_transition ADD COLUMN ready_at timestamptz
    CHECK (ready_at IS NULL OR (target='ready' AND isfinite(ready_at)));

CREATE FUNCTION ledger.record_pickup_ready_at() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $capture$
BEGIN
    IF NEW.target='ready' THEN
        SELECT progress.updated_at INTO STRICT NEW.ready_at
        FROM ledger.travel_pickup_progress progress
        WHERE progress.sale_id=NEW.sale_id AND progress.revision=NEW.revision AND progress.state='ready';
    END IF;
    RETURN NEW;
END
$capture$;
REVOKE ALL ON FUNCTION ledger.record_pickup_ready_at() FROM PUBLIC;
CREATE TRIGGER record_pickup_ready_at BEFORE INSERT ON ledger.travel_pickup_transition
FOR EACH ROW EXECUTE FUNCTION ledger.record_pickup_ready_at();

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
            'finalizedAt',journal.booked_at,
            'pickupReadyAt',CASE WHEN pickup.state IN ('ready','collected')
                AND ready.ready_at>=journal.booked_at THEN ready.ready_at END,
            'pickupStage',CASE WHEN v.state='paid' AND v.pickup_no IS NOT NULL THEN pickup.state END
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
    -- Canonical block time and PostgreSQL booking time use different clocks.
    LEFT JOIN ledger.journal journal ON journal.sale_id=paid.sale_id AND journal.refund_id IS NULL
        AND journal.evidence_id=paid.evidence_id
        AND ledger.kiosk_menu_pickup(paid.sale_id)
    LEFT JOIN LATERAL (
        SELECT step.ready_at FROM ledger.travel_pickup_transition step
         WHERE step.sale_id=paid.sale_id AND step.target='ready' AND step.revision<=pickup.revision
         ORDER BY step.revision LIMIT 1
    ) ready ON journal.sale_id IS NOT NULL
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

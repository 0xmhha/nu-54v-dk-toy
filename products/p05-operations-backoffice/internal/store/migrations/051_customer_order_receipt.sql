-- Retain the existing response fields. Add the payer's own transaction
-- summary from the original submission, attempt, settlement and pickup state.
CREATE FUNCTION ledger.commerce_order_receipt(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid)
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
REVOKE ALL ON FUNCTION ledger.commerce_order_receipt(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.commerce_order_receipt(uuid,uuid,uuid,uuid) TO nu54_p05_commerce;

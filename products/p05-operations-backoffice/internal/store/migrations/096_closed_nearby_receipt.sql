-- Read a closed nearby sale only for the active customer identified by both
-- its original authenticated reservation and its signed quote. An expired
-- comparison claim cannot establish durable order ownership by itself.
CREATE FUNCTION ledger.owns_closed_nearby_order(branch_ uuid,actor_ uuid,sale_ uuid)
RETURNS boolean LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $owner$
    SELECT EXISTS (
        SELECT FROM ledger.sale s
        JOIN ledger.menu_reservation_outcome r ON (r.branch_id,r.sale_id,r.origin_ref)=(s.branch_id,s.id,s.origin_ref)
        JOIN ledger.order_checkout c ON (c.branch_id,c.sale_id,c.creator)=(r.branch_id,r.sale_id,r.actor_id)
        JOIN ledger.quote_work q ON q.sale_id=s.id AND q.signed_digest IS NOT NULL AND q.actor_id=actor_
        JOIN ledger.sale_resolution absent ON absent.sale_id=q.sale_id
        WHERE (s.branch_id,s.id)=(branch_,sale_) AND s.state='closed' AND s.origin='menu'
          AND r.nearby_customer=actor_ AND r.rejection IS NULL
          AND NOT EXISTS (SELECT FROM ledger.payment p WHERE p.sale_id=s.id)
          AND NOT EXISTS (SELECT FROM ledger.pickup p WHERE p.sale_id=s.id)
    )
$owner$;
REVOKE ALL ON FUNCTION ledger.owns_closed_nearby_order(uuid,uuid,uuid) FROM PUBLIC;

CREATE OR REPLACE FUNCTION ledger.commerce_order_view(tenant uuid, branch uuid, actor uuid, sale_ uuid)
RETURNS TABLE(id uuid, revision bigint, state text, asset_ref text, amount numeric, expires_at timestamptz, pickup_no bigint, receipt_ref uuid, quote_digest text, order_key text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE grant_ ledger.commerce_grant := ledger.commerce_allowed(tenant,branch,actor);
BEGIN
    RETURN QUERY
      SELECT o.id,o.revision,o.state,o.asset_ref,o.amount::numeric,c.expires_at,p.number,COALESCE(p.receipt_ref,e.receipt_ref),
        CASE WHEN o.state='issued' THEN encode(q.signed_digest,'hex') END,
        CASE WHEN o.state='issued' THEN encode(q.order_id,'hex') END
      FROM ledger.sale o JOIN ledger.order_checkout c ON c.sale_id=o.id AND c.branch_id=o.branch_id
      LEFT JOIN ledger.pickup p ON p.branch_id=o.branch_id AND p.sale_id=o.id AND o.state='paid'
      LEFT JOIN ledger.invoice i ON i.sale_id=o.id AND o.origin='subscription'
      LEFT JOIN ledger.subscription_entitlement e ON e.invoice_id=i.id AND o.state='paid'
      LEFT JOIN ledger.quote_work q ON q.sale_id=o.id AND q.signed_digest IS NOT NULL
      WHERE o.branch_id=branch AND o.id=sale_ AND (c.creator=actor OR
        EXISTS(SELECT FROM ledger.checkout_claim issued WHERE issued.branch_id=branch AND issued.sale_id=o.id
            AND issued.actor_id=actor AND issued.expires_at>statement_timestamp()) OR
        (o.state='paid' AND COALESCE(p.receipt_ref,e.receipt_ref) IS NOT NULL AND q.actor_id=actor) OR
        (grant_.role_code='customer' AND ledger.owns_closed_nearby_order(branch,actor,o.id)));
END $$;

CREATE OR REPLACE FUNCTION ledger.customer_order_rows(tenant_ uuid,branch_ uuid,actor_ uuid,after_ uuid)
RETURNS TABLE(id uuid,state text,asset_ref text,amount text,access_ends_at timestamptz)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $rows$
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code <> 'customer' THEN
        RAISE EXCEPTION 'customer scope required' USING ERRCODE='42501';
    END IF;
    RETURN QUERY
    WITH candidates AS (
        SELECT c.sale_id FROM ledger.order_checkout c
        WHERE c.branch_id=branch_ AND c.creator=actor_
        UNION
        SELECT cl.sale_id FROM ledger.checkout_claim cl
        WHERE cl.branch_id=branch_ AND cl.actor_id=actor_ AND cl.expires_at>statement_timestamp()
        UNION
        SELECT q.sale_id FROM ledger.quote_work q
        JOIN ledger.sale s ON s.id=q.sale_id AND s.branch_id=branch_ AND (s.state='paid' OR
              ledger.owns_closed_nearby_order(branch_,actor_,s.id))
        WHERE q.actor_id=actor_ AND q.signed_digest IS NOT NULL
    )
    SELECT v.id,v.state,v.asset_ref,v.amount::text,
        CASE WHEN oc.creator=actor_ OR (v.state='paid' AND v.receipt_ref IS NOT NULL AND q.actor_id=actor_)
                  OR ledger.owns_closed_nearby_order(branch_,actor_,v.id)
             THEN NULL ELSE cl.expires_at END
    FROM candidates c
    JOIN ledger.sale s ON s.id=c.sale_id AND s.branch_id=branch_ AND s.origin IN ('menu','pos')
    JOIN ledger.order_checkout oc ON oc.sale_id=s.id AND oc.branch_id=branch_
    LEFT JOIN ledger.checkout_claim cl ON cl.sale_id=s.id AND cl.branch_id=branch_ AND cl.actor_id=actor_
    LEFT JOIN ledger.quote_work q ON q.sale_id=s.id AND q.signed_digest IS NOT NULL
    CROSS JOIN LATERAL ledger.commerce_order_view(tenant_,branch_,actor_,c.sale_id) v
    WHERE (after_=actor_ OR c.sale_id>after_)
      AND (v.state<>'paid' OR oc.creator=actor_ OR q.actor_id=actor_)
    ORDER BY c.sale_id LIMIT 50;
END
$rows$;

CREATE OR REPLACE FUNCTION ledger.commerce_order_receipt(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid)
RETURNS TABLE(id uuid,revision bigint,state text,asset_ref text,amount numeric,expires_at timestamptz,
    pickup_no bigint,receipt_ref uuid,quote_digest text,order_key text,receipt_kind text,subscription jsonb,
    payment jsonb,access_ends_at timestamptz)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $receipt$
    SELECT v.*,
        CASE WHEN evidence.stage IS NOT NULL THEN jsonb_strip_nulls(jsonb_build_object(
            'stage',evidence.stage,
            'nearbyUnpaid',CASE WHEN ownership.nearby_unpaid THEN true END,
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
                    AND NOT ownership.nearby_unpaid
                    AND (v.state<>'paid' OR v.receipt_ref IS NULL OR payer.sale_id IS NULL)
             THEN claim.expires_at END
    FROM ledger.commerce_order_status_with_invoice(tenant_,branch_,actor_,sale_) v
    CROSS JOIN LATERAL ledger.commerce_allowed(tenant_,branch_,actor_) grant_record
    CROSS JOIN LATERAL (
        SELECT grant_record.role_code='customer' AND ledger.owns_closed_nearby_order(branch_,actor_,v.id) AS nearby_unpaid
    ) ownership
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

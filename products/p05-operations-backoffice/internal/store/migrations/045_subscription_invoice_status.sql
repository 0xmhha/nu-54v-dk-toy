-- Customer read projection. The invoice due date limits issuance, while the
-- signed quote has its own on-chain deadline. Only settlement may establish
-- that the full eligible block range was finalized without payment.
CREATE VIEW ledger.subscription_invoice_projection AS
SELECT i.id,i.branch_id,s.customer_scope,i.schedule_ref,i.sale_id,i.cycle_key,i.due_at,
       sale.state,sale.asset_ref,sale.amount::text AS amount,i.fulfillment_kind,
       jsonb_build_object(
          'invoiceRef',i.id,'cycle',i.cycle_key,'dueAt',i.due_at,'termsVersion',s.terms_version,
          'stage',CASE
            WHEN sale.state='paid' THEN 'paid'
            WHEN sale.state='closed' AND r.sale_id IS NOT NULL AND q.sale_id IS NOT NULL THEN 'expired'
            WHEN sale.state='closed' THEN 'closed'
            WHEN q.sale_id IS NOT NULL AND statement_timestamp()>=date_trunc('second',q.expires_at) THEN 'reconcile'
            WHEN sale.state='checking' THEN 'checking'
            WHEN q.sale_id IS NOT NULL THEN 'issued'
            WHEN statement_timestamp()>=i.due_at THEN 'awaitingClosure'
            ELSE 'preview'
          END
       ) || CASE WHEN q.sale_id IS NOT NULL THEN jsonb_build_object(
          'quoteRef','0x'||encode(q.quote_id,'hex'),
          'quoteExpiresAt',date_trunc('second',q.expires_at)
       ) ELSE '{}'::jsonb END AS lifecycle
FROM ledger.invoice i
JOIN ledger.subscription s ON s.id=i.schedule_ref AND s.branch_id=i.branch_id
JOIN ledger.sale sale ON sale.id=i.sale_id AND sale.branch_id=i.branch_id AND sale.origin='subscription'
LEFT JOIN ledger.quote_work q ON q.sale_id=sale.id AND q.actor_id=s.customer_scope AND q.signed_digest IS NOT NULL
LEFT JOIN ledger.sale_resolution r ON r.sale_id=q.sale_id;
REVOKE ALL ON ledger.subscription_invoice_projection FROM PUBLIC,nu54_p05_commerce;

CREATE FUNCTION ledger.subscription_invoice_rows(tenant_ uuid,branch_ uuid,actor_ uuid)
RETURNS TABLE(id uuid,subscription_id uuid,sale_id uuid,cycle_key date,due_at timestamptz,state text,
              asset_ref text,amount text,fulfillment_kind text,lifecycle jsonb)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $rows$
BEGIN
    IF (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code<>'customer' THEN
        RAISE EXCEPTION 'customer scope required' USING ERRCODE='42501';
    END IF;
    RETURN QUERY SELECT p.id,p.schedule_ref,p.sale_id,p.cycle_key,p.due_at,p.state,
        p.asset_ref,p.amount,p.fulfillment_kind,p.lifecycle
    FROM ledger.subscription_invoice_projection p
    WHERE p.branch_id=branch_ AND p.customer_scope=actor_
    ORDER BY p.due_at DESC,p.id LIMIT 100;
END
$rows$;

CREATE FUNCTION ledger.commerce_order_status_with_invoice(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid)
RETURNS TABLE(id uuid,revision bigint,state text,asset_ref text,amount numeric,expires_at timestamptz,
              pickup_no bigint,receipt_ref uuid,quote_digest text,order_key text,receipt_kind text,subscription jsonb)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $status$
    SELECT v.*,p.lifecycle
    FROM ledger.commerce_order_status(tenant_,branch_,actor_,sale_) v
    LEFT JOIN ledger.subscription_invoice_projection p
      ON p.sale_id=v.id AND p.branch_id=branch_ AND p.customer_scope=actor_
      AND (ledger.commerce_allowed(tenant_,branch_,actor_)).role_code='customer'
$status$;
REVOKE ALL ON FUNCTION ledger.subscription_invoice_rows(uuid,uuid,uuid),
    ledger.commerce_order_status_with_invoice(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.subscription_invoice_rows(uuid,uuid,uuid),
    ledger.commerce_order_status_with_invoice(uuid,uuid,uuid,uuid) TO nu54_p05_commerce;

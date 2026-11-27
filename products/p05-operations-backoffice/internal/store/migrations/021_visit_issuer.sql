DO $roles$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p05_visit_issuer') THEN
       CREATE ROLE nu54_p05_visit_issuer NOLOGIN NOSUPERUSER NOCREATEROLE NOCREATEDB NOBYPASSRLS;
    END IF;
END
$roles$;

CREATE FUNCTION ledger.visit_origin(tenant_ uuid,branch_ uuid,actor_ uuid,sale_ uuid)
RETURNS TABLE(stream uuid,merchant text,order_key text,transaction_ref text,state text,source_revision bigint,observed_at timestamptz)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $visit$
DECLARE auth ledger.commerce_grant := ledger.commerce_allowed(tenant_,branch_,actor_);
BEGIN
    IF auth.role_code<>'customer' THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='visit scope unavailable';
    END IF;
    RETURN QUERY
      SELECT p.stream_id,p.merchant_key,'0x'||encode(q.order_id,'hex'),p.tx_hash,
        CASE WHEN p.refunded>0 THEN 'refunded' ELSE 'settled' END,
        s.revision+1,coalesce(last_refund.included_at,p.included_at)
      FROM ledger.sale s
      JOIN ledger.order_checkout checkout ON (checkout.sale_id,checkout.branch_id)=(s.id,s.branch_id)
      JOIN ledger.payment p ON p.sale_id=s.id
      JOIN ledger.quote_work q ON q.sale_id=s.id
      LEFT JOIN LATERAL (SELECT max(confirm.included_at) AS included_at FROM ledger.refund r
        JOIN ledger.refund_confirmation confirm ON confirm.refund_id=r.id WHERE r.sale_id=s.id) last_refund ON true
      WHERE s.id=sale_ AND s.branch_id=branch_ AND checkout.creator=actor_ AND s.state='paid';
END
$visit$;

REVOKE ALL ON FUNCTION ledger.visit_origin(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT USAGE ON SCHEMA ledger TO nu54_p05_visit_issuer;
GRANT EXECUTE ON FUNCTION ledger.visit_origin(uuid,uuid,uuid,uuid) TO nu54_p05_visit_issuer;

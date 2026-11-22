CREATE FUNCTION ledger.operator_quote_boundary(p_tenant uuid,p_branch uuid,p_actor uuid,p_sale uuid)
RETURNS TABLE(payload_cipher bytea,signed_digest bytea,merchant_signer bytea,
              quote_id bytea,order_id bytea,issued_at timestamptz,expires_at timestamptz)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.require_finance(p_tenant,p_branch,p_actor,'reconcile_read');
    RETURN QUERY
      SELECT q.payload_cipher,q.signed_digest,q.merchant_signer,q.quote_id,q.order_id,q.issued_at,q.expires_at
        FROM ledger.sale s JOIN ledger.quote_work q ON q.sale_id=s.id
        WHERE s.branch_id=p_branch AND s.id=p_sale AND q.payload_cipher IS NOT NULL;
END $$;
REVOKE ALL ON FUNCTION ledger.operator_quote_boundary(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.operator_quote_boundary(uuid,uuid,uuid,uuid) TO nu54_p05_finance;

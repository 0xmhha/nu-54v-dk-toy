CREATE FUNCTION ledger.operator_payment_evidence(tenant uuid,branch uuid,actor uuid,sale_ uuid)
RETURNS TABLE(sale_state text,quote_cipher bytea,quote_digest bytea,signer bytea,relay_state text,relay_hash text,
    settled_hash text,settled_log integer,settled_block text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'reconcile_read');
    RETURN QUERY SELECT s.state,q.payload_cipher,q.signed_digest,q.merchant_signer,r.state,r.tx_hash,p.tx_hash,p.log_index,p.inclusion_hash
      FROM ledger.sale s LEFT JOIN ledger.quote_work q ON q.sale_id=s.id
      LEFT JOIN ledger.settlement_work job ON job.sale_id=s.id AND job.refund_id IS NULL
      LEFT JOIN ledger.relay_attempt r ON r.work_id=job.id LEFT JOIN ledger.payment p ON p.sale_id=s.id
      WHERE s.id=sale_ AND s.branch_id=branch;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='payment evidence scope denied'; END IF;
END $$;
REVOKE ALL ON FUNCTION ledger.operator_payment_evidence(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.operator_payment_evidence(uuid,uuid,uuid,uuid) TO nu54_p05_finance;

-- Private projection for the already issued and still-live checkout.
CREATE FUNCTION ledger.private_quote_context(tenant uuid, branch uuid, actor uuid, sale_ uuid, claim_ bytea)
RETURNS TABLE(customer_ref uuid, origin_ref uuid)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE active ledger.commerce_grant := ledger.commerce_allowed(tenant,branch,actor);
BEGIN
    IF NOT ledger.c3_scope(tenant,branch,actor,active.role_code,sale_,'quote.issue',claim_) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='presentation scope denied';
    END IF;
    RETURN QUERY SELECT q.actor_id,s.origin_ref FROM ledger.quote_work q
      JOIN ledger.sale s ON s.id=q.sale_id
      WHERE q.sale_id=sale_ AND s.branch_id=branch AND s.state='issued'
        AND q.claim_digest=claim_ AND q.signed_digest IS NOT NULL AND q.consumed_at IS NULL
        AND q.expires_at>clock_timestamp();
END $$;
REVOKE ALL ON FUNCTION ledger.private_quote_context(uuid,uuid,uuid,uuid,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.private_quote_context(uuid,uuid,uuid,uuid,bytea) TO nu54_p05_commerce;

-- The caller can reconcile only one enrolled merchant and a bounded business window.
-- Digest the exact chain facts rather than trusting equality of totals alone.
CREATE FUNCTION chainview.finance_day_evidence(stream_ uuid,merchant_ text,asset_ text,from_ timestamptz,until_ timestamptz,
    chain_ text,contract_ text,abi_ text,manifest_ text)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    generation_ uuid;
    final_hash text;
    final_height bigint;
    proof jsonb;
BEGIN
    IF NOT isfinite(from_) OR NOT isfinite(until_) OR until_<from_+interval '22 hours'
       OR until_>from_+interval '26 hours' OR asset_ !~ '^0x[0-9a-f]{40}$'
       OR NOT chainview.reconciliation_ready(stream_,merchant_) THEN
        RETURN NULL;
    END IF;
    SELECT g.id,g.finalized_hash,g.finalized_height INTO generation_,final_hash,final_height
      FROM chainview.read_target t JOIN chainview.generation g ON g.id=t.generation_id
      JOIN chainview.stream s ON s.id=t.stream_id
      JOIN chainview.block f ON f.generation_id=g.id AND f.block_hash=g.finalized_hash AND f.canonical
      JOIN chainview.block b ON b.generation_id=g.id AND b.height=g.base_height AND b.canonical
      WHERE t.stream_id=stream_ AND (s.chain_ref,s.contract_ref,s.abi_hash,s.manifest_ref)=(chain_,contract_,abi_,manifest_)
        AND b.block_time<=from_ AND f.block_time>=until_;
    IF NOT FOUND THEN RETURN NULL; END IF;
    SELECT jsonb_build_object('anchor',final_hash,'height',final_height,
        'gross',trim_scale(coalesce(sum(e.amount) FILTER (WHERE e.kind='paid'),0))::text,
        'refunds',trim_scale(coalesce(sum(e.amount) FILTER (WHERE e.kind='refunded'),0))::text,
        'count',count(*),
        'digest',encode(sha256(convert_to(coalesce(jsonb_agg(jsonb_build_array(
            e.kind,e.tx_hash,e.log_index,e.block_hash,trim_scale(e.amount)::text)
            ORDER BY e.tx_hash COLLATE "C",e.log_index,e.block_hash COLLATE "C"),'[]'::jsonb)::text,'UTF8')),'hex'))
      INTO proof
      FROM chainview.observed_event e JOIN chainview.block b
        ON b.generation_id=e.generation_id AND b.block_hash=e.block_hash
      WHERE e.generation_id=generation_ AND b.canonical AND b.height<=final_height
        AND b.block_time>=from_ AND b.block_time<until_ AND e.receipt_success
        AND e.merchant_key=merchant_ AND e.asset_ref=asset_ AND e.kind IN ('paid','refunded');
    RETURN proof;
END $$;
REVOKE ALL ON FUNCTION chainview.finance_day_evidence(uuid,text,text,timestamptz,timestamptz,text,text,text,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION chainview.finance_day_evidence(uuid,text,text,timestamptz,timestamptz,text,text,text,text) TO nu54_p07_reconcile;

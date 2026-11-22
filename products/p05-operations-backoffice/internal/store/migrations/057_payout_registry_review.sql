-- Bind the quote lineage at proposal time and retain the registry's effective
-- time only after the production worker verifies the finalized receipt.
ALTER TABLE ledger.payout_change
    ADD COLUMN predecessor_profile_version bigint CHECK (predecessor_profile_version>0),
    ADD COLUMN activated_at timestamptz CHECK (
        activated_at IS NULL OR (isfinite(activated_at) AND phase='active' AND finalized_block IS NOT NULL)
    );

CREATE OR REPLACE FUNCTION ledger.payout_state(tenant uuid,branch uuid,actor uuid,key_ uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE proposal ledger.payout_change;
DECLARE current_registry jsonb;
DECLARE predecessor bigint;
DECLARE quote_status jsonb;
BEGIN
    PERFORM ledger.payout_scope(tenant,branch,actor,array['payout_propose','payout_approve','payout_notice']);
    SELECT jsonb_build_object('currentAddress','0x'||encode(active.address,'hex'),'currentVersion',active.version)
      INTO current_registry FROM ledger.active_payout(branch) active;
    SELECT c.* INTO proposal FROM ledger.payout_change c
      WHERE c.branch_id=branch AND (key_ IS NULL OR c.request_key=key_)
      ORDER BY c.created_at DESC,c.request_key DESC LIMIT 1;
    IF NOT FOUND THEN RETURN current_registry; END IF;

    predecessor := proposal.predecessor_profile_version;
    IF predecessor IS NULL AND proposal.phase IN ('review','frozen','signed') THEN
        -- Older requests can still identify their unchanged issuer profile.
        SELECT p.version INTO predecessor FROM ledger.issuance_profile p
          WHERE p.branch_id=branch AND p.authorized_digest=proposal.profile_digest;
    END IF;
    WITH tracked AS (
        SELECT q.quote_id,q.expires_at,s.id AS sale_id,p.tx_hash,unpaid.evidence_id,
            CASE WHEN p.sale_id IS NOT NULL THEN 'paid'
                 WHEN unpaid.sale_id IS NOT NULL THEN 'expired'
                 ELSE 'pending' END AS status,
            CASE
                WHEN q.signed_digest=ANY(proposal.frozen_hashes) THEN
                    CASE WHEN proposal.phase='active' THEN 'grandfathered' ELSE 'snapshotted' END
                WHEN p.sale_id IS NULL AND proposal.activated_at IS NOT NULL AND q.expires_at>proposal.activated_at THEN 'cutover-rejected'
            END AS admission
        FROM ledger.sale s
        JOIN ledger.quote_work q ON q.sale_id=s.id
        JOIN ledger.quote_outbox published ON published.sale_id=s.id AND published.quote_digest=q.signed_digest
        LEFT JOIN ledger.payment p ON p.sale_id=s.id
        LEFT JOIN ledger.sale_resolution unpaid ON unpaid.sale_id=s.id
        WHERE s.branch_id=branch AND (
            q.signed_digest=ANY(proposal.frozen_hashes)
            OR (q.profile_version=predecessor AND (
                q.expires_at>proposal.created_at OR
                coalesce(q.consumed_at,unpaid.resolved_at,'infinity'::timestamptz)>=proposal.created_at
            ))
        )
    )
    SELECT jsonb_build_object(
        'quoteTracking',CASE WHEN predecessor IS NOT NULL THEN 'issuer-profile'
                             WHEN proposal.frozen_hashes IS NOT NULL THEN 'rotation-archive'
                             ELSE 'unavailable' END,
        'oldQuoteCount',(SELECT count(*) FROM tracked),
        'oldQuotes',coalesce((
            SELECT jsonb_agg(jsonb_strip_nulls(jsonb_build_object(
                'saleId',x.sale_id,'quoteId','0x'||encode(x.quote_id,'hex'),'expiresAt',x.expires_at,
                'status',x.status,'admission',x.admission,'transaction',x.tx_hash,'expiryEvidence',x.evidence_id
            )) ORDER BY x.expires_at DESC,x.sale_id)
            FROM (SELECT * FROM tracked ORDER BY expires_at DESC,sale_id LIMIT 50) x
        ),'[]'::jsonb)
    ) INTO quote_status;

    RETURN current_registry || quote_status || jsonb_build_object(
        'requestKey',proposal.request_key,'reason',proposal.reason,'address','0x'||encode(proposal.new_address,'hex'),
        'oldAddress','0x'||encode(proposal.old_address,'hex'),'oldVersion',proposal.old_version,
        'proposedVersion',proposal.old_version+1,'phase',proposal.phase,'effectiveAt',proposal.ready_at,
        'notifiedAt',proposal.notified_at,'activatedAt',proposal.activated_at,
        'effectiveBlock','0x'||encode(proposal.finalized_block,'hex'),
        'approvals',(SELECT count(*) FROM ledger.payout_approval a
            WHERE a.branch_id=branch AND a.request_key=proposal.request_key),
        'txHash','0x'||encode(proposal.chain_tx_hash,'hex')
    );
END $$;

CREATE OR REPLACE FUNCTION ledger.propose_payout(tenant uuid,branch uuid,actor uuid,key_ uuid,reason_ text,
    old_ bytea,new_ bytea,version_ bigint,profile_ bytea,proof_ bytea)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE prior ledger.payout_change;
DECLARE predecessor bigint;
BEGIN
    PERFORM ledger.payout_scope(tenant,branch,actor,array['payout_propose']);
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text,21));
    SELECT * INTO prior FROM ledger.payout_change c WHERE c.branch_id=branch AND c.request_key=key_;
    IF FOUND THEN
        IF (prior.proposed_by,prior.reason,prior.old_address,prior.new_address,prior.old_version,prior.profile_digest,prior.proof_digest)
            IS DISTINCT FROM (actor,reason_,old_,new_,version_,profile_,proof_) THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='payout proposal scope changed';
        END IF;
    ELSE
        SELECT p.version INTO predecessor FROM ledger.issuance_profile p
            WHERE p.branch_id=branch AND p.enabled AND p.authorized_digest=profile_ FOR SHARE;
        IF NOT FOUND THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='payout profile changed';
        END IF;
        INSERT INTO ledger.payout_change(branch_id,request_key,proposed_by,reason,old_address,new_address,old_version,
            profile_digest,proof_digest,predecessor_profile_version)
          VALUES(branch,key_,actor,reason_,old_,new_,version_,profile_,proof_,predecessor);
    END IF;
    RETURN ledger.payout_state(tenant,branch,actor,key_);
END $$;

-- Keep the original signature for existing workers. Legacy final rows have
-- a verified block already and no inferred chain timestamp.
CREATE FUNCTION ledger.confirm_payout(branch uuid,key_ uuid,hash_ bytea,block_ bytea,next_profile bytea,activated_ timestamptz)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF activated_ IS NULL OR NOT isfinite(activated_) THEN RETURN false; END IF;
    IF NOT ledger.confirm_payout(branch,key_,hash_,block_,next_profile) THEN RETURN false; END IF;
    UPDATE ledger.payout_change c SET activated_at=activated_ WHERE c.branch_id=branch AND c.request_key=key_;
    RETURN true;
END $$;

REVOKE ALL ON FUNCTION ledger.confirm_payout(uuid,uuid,bytea,bytea,bytea,timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.confirm_payout(uuid,uuid,bytea,bytea,bytea,timestamptz) TO nu54_p05_payout_worker;

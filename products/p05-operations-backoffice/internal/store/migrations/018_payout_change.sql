ALTER TABLE ledger.finance_grant DROP CONSTRAINT finance_grant_capability_check;
ALTER TABLE ledger.finance_grant ADD CONSTRAINT finance_grant_capability_check CHECK(capability IN (
    'refund_request','refund_approve','day_close','report_read','report_export','reconcile_read','reconcile_retry',
    'payout_propose','payout_approve','payout_notice'
));

CREATE TABLE ledger.payout_change(
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    request_key uuid NOT NULL,
    proposed_by uuid NOT NULL REFERENCES ledger.login_subject(subject_ref),
    reason text NOT NULL CHECK(reason IN ('custody_rotation','governance')),
    old_address bytea NOT NULL CHECK(octet_length(old_address)=20),
    new_address bytea NOT NULL CHECK(octet_length(new_address)=20 AND new_address<>old_address),
    old_version bigint NOT NULL CHECK(old_version>0),
    profile_digest bytea NOT NULL CHECK(octet_length(profile_digest)=32),
    proof_digest bytea NOT NULL CHECK(octet_length(proof_digest)=32),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    ready_at timestamptz,
    notified_by uuid REFERENCES ledger.login_subject(subject_ref),
    notified_at timestamptz,
    phase text NOT NULL DEFAULT 'review' CHECK(phase IN ('review','frozen','signed','active','held')),
    frozen_hashes bytea[],
    chain_tx_hash bytea CHECK(octet_length(chain_tx_hash)=32),
    signed_transaction bytea CHECK(octet_length(signed_transaction) BETWEEN 100 AND 15000),
    finalized_block bytea CHECK(octet_length(finalized_block)=32),
    PRIMARY KEY(branch_id,request_key)
);
CREATE UNIQUE INDEX payout_one_pending_per_branch ON ledger.payout_change(branch_id) WHERE phase<>'active';
CREATE UNIQUE INDEX payout_one_inflight ON ledger.payout_change((1)) WHERE phase IN ('frozen','signed');
CREATE UNIQUE INDEX payout_chain_tx_once ON ledger.payout_change(chain_tx_hash) WHERE chain_tx_hash IS NOT NULL;
CREATE TABLE ledger.payout_approval(
    branch_id uuid NOT NULL,
    request_key uuid NOT NULL,
    actor_id uuid NOT NULL REFERENCES ledger.login_subject(subject_ref),
    approved_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(branch_id,request_key,actor_id),
    FOREIGN KEY(branch_id,request_key) REFERENCES ledger.payout_change
);
REVOKE ALL ON ledger.payout_change,ledger.payout_approval FROM PUBLIC;

CREATE FUNCTION ledger.payout_scope(tenant uuid,branch uuid,actor uuid,capabilities text[])
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF NOT EXISTS (SELECT FROM ledger.finance_grant g JOIN ledger.branch b ON b.id=g.branch_id
        WHERE b.id=branch AND b.tenant_ref=tenant AND g.actor_id=actor AND
            g.capability=ANY(capabilities) AND g.valid_until>clock_timestamp()) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='payout authority denied';
    END IF;
END $$;

CREATE FUNCTION ledger.payout_state(tenant uuid,branch uuid,actor uuid,key_ uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE response jsonb;
BEGIN
    PERFORM ledger.payout_scope(tenant,branch,actor,array['payout_propose','payout_approve','payout_notice']);
    SELECT jsonb_build_object('requestKey',c.request_key,'reason',c.reason,'address','0x'||encode(c.new_address,'hex'),
        'oldAddress','0x'||encode(c.old_address,'hex'),'oldVersion',c.old_version,
        'phase',c.phase,'effectiveAt',c.ready_at,'notifiedAt',c.notified_at,
        'approvals',(SELECT count(*) FROM ledger.payout_approval a WHERE a.branch_id=branch AND a.request_key=c.request_key),
        'txHash',CASE WHEN c.chain_tx_hash IS NOT NULL THEN '0x'||encode(c.chain_tx_hash,'hex') END)
      INTO response FROM ledger.payout_change c
      WHERE c.branch_id=branch AND (key_ IS NULL OR c.request_key=key_)
      ORDER BY c.created_at DESC LIMIT 1;
    RETURN coalesce(response,'{}'::jsonb);
END $$;

CREATE FUNCTION ledger.propose_payout(tenant uuid,branch uuid,actor uuid,key_ uuid,reason_ text,
    old_ bytea,new_ bytea,version_ bigint,profile_ bytea,proof_ bytea)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE prior ledger.payout_change;
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
        IF NOT EXISTS (SELECT FROM ledger.issuance_profile p WHERE p.branch_id=branch AND p.enabled AND p.authorized_digest=profile_) THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='payout profile changed';
        END IF;
        INSERT INTO ledger.payout_change(branch_id,request_key,proposed_by,reason,old_address,new_address,old_version,profile_digest,proof_digest)
          VALUES(branch,key_,actor,reason_,old_,new_,version_,profile_,proof_);
    END IF;
    RETURN ledger.payout_state(tenant,branch,actor,key_);
END $$;

CREATE FUNCTION ledger.approve_payout(tenant uuid,branch uuid,actor uuid,key_ uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE proposal ledger.payout_change;
DECLARE approvals integer;
BEGIN
    PERFORM ledger.payout_scope(tenant,branch,actor,array['payout_approve']);
    SELECT * INTO proposal FROM ledger.payout_change c WHERE c.branch_id=branch AND c.request_key=key_ FOR UPDATE;
    IF NOT FOUND OR proposal.proposed_by=actor THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='separate approval required';
    END IF;
    IF EXISTS (SELECT FROM ledger.payout_approval a WHERE a.branch_id=branch AND a.request_key=key_ AND a.actor_id=actor) THEN
        RETURN ledger.payout_state(tenant,branch,actor,key_);
    END IF;
    SELECT count(*) INTO approvals FROM ledger.payout_approval a WHERE a.branch_id=branch AND a.request_key=key_;
    IF approvals>=2 OR proposal.phase<>'review' THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='approval closed';
    END IF;
    INSERT INTO ledger.payout_approval(branch_id,request_key,actor_id) VALUES(branch,key_,actor);
    IF approvals=1 THEN
        UPDATE ledger.payout_change c SET ready_at=clock_timestamp()+interval '24 hours'
          WHERE c.branch_id=branch AND c.request_key=key_;
    END IF;
    RETURN ledger.payout_state(tenant,branch,actor,key_);
END $$;

CREATE FUNCTION ledger.acknowledge_payout_notice(tenant uuid,branch uuid,actor uuid,key_ uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE proposal ledger.payout_change;
BEGIN
    PERFORM ledger.payout_scope(tenant,branch,actor,array['payout_notice']);
    SELECT * INTO proposal FROM ledger.payout_change c WHERE c.branch_id=branch AND c.request_key=key_ FOR UPDATE;
    IF NOT FOUND OR proposal.proposed_by=actor OR EXISTS (SELECT FROM ledger.payout_approval a
        WHERE a.branch_id=branch AND a.request_key=key_ AND a.actor_id=actor) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='independent notification receipt required';
    END IF;
    IF proposal.ready_at IS NULL THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='approval pending';
    END IF;
    UPDATE ledger.payout_change c SET notified_by=actor,notified_at=clock_timestamp()
      WHERE c.branch_id=branch AND c.request_key=key_ AND c.notified_at IS NULL AND c.phase='review';
    RETURN ledger.payout_state(tenant,branch,actor,key_);
END $$;

REVOKE ALL ON FUNCTION ledger.payout_scope(uuid,uuid,uuid,text[]),
    ledger.payout_state(uuid,uuid,uuid,uuid),ledger.propose_payout(uuid,uuid,uuid,uuid,text,bytea,bytea,bigint,bytea,bytea),
    ledger.approve_payout(uuid,uuid,uuid,uuid),ledger.acknowledge_payout_notice(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.payout_scope(uuid,uuid,uuid,text[]),
    ledger.payout_state(uuid,uuid,uuid,uuid),ledger.propose_payout(uuid,uuid,uuid,uuid,text,bytea,bytea,bigint,bytea,bytea),
    ledger.approve_payout(uuid,uuid,uuid,uuid),ledger.acknowledge_payout_notice(uuid,uuid,uuid,uuid) TO nu54_p05_commerce;

CREATE FUNCTION ledger.active_payout(branch uuid)
RETURNS TABLE(address bytea,version bigint)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
    SELECT newest.new_address,newest.old_version+1 FROM (SELECT 1) dummy
    LEFT JOIN LATERAL (SELECT c.new_address,c.old_version FROM ledger.payout_change c
        WHERE c.branch_id=branch AND c.phase='active' ORDER BY c.old_version DESC LIMIT 1) newest ON true;
$$;
REVOKE ALL ON FUNCTION ledger.active_payout(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.active_payout(uuid) TO nu54_p05_commerce;

DO $$
BEGIN
    IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='nu54_p05_payout_worker') THEN
        CREATE ROLE nu54_p05_payout_worker NOLOGIN;
    ELSIF EXISTS(SELECT FROM pg_roles WHERE rolname='nu54_p05_payout_worker' AND
        (rolcanlogin OR rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb OR rolreplication)) THEN
        RAISE EXCEPTION 'unsafe payout worker role';
    END IF;
END $$;
GRANT USAGE ON SCHEMA ledger TO nu54_p05_payout_worker;

CREATE FUNCTION ledger.claim_payout(branch uuid,profile_ bytea)
RETURNS TABLE(request_key uuid,new_address bytea,old_address bytea,old_version bigint,phase text,
    frozen_hashes bytea[],tx_hash bytea,signed_tx bytea)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE proposal ledger.payout_change;
DECLARE enrolled ledger.issuance_profile;
DECLARE snapshot bytea[];
BEGIN
    PERFORM pg_advisory_xact_lock(hashtextextended('nu54 payout governor',22));
    IF EXISTS (SELECT FROM ledger.payout_change c WHERE c.phase IN ('frozen','signed') AND c.branch_id<>branch) THEN
        RETURN;
    END IF;
    SELECT * INTO proposal FROM ledger.payout_change c WHERE c.branch_id=branch AND c.phase IN ('review','frozen','signed')
      AND c.ready_at<=clock_timestamp() AND c.notified_at IS NOT NULL AND c.profile_digest=profile_ FOR UPDATE;
    IF NOT FOUND THEN RETURN; END IF;
    IF proposal.phase='review' THEN
        IF NOT EXISTS (SELECT FROM ledger.finance_grant g WHERE g.branch_id=branch AND g.actor_id=proposal.proposed_by
                  AND g.capability='payout_propose' AND g.valid_until>clock_timestamp())
           OR NOT EXISTS (SELECT FROM ledger.finance_grant g WHERE g.branch_id=branch AND g.actor_id=proposal.notified_by
                  AND g.capability='payout_notice' AND g.valid_until>clock_timestamp())
           OR 2<>(SELECT count(*) FROM ledger.payout_approval a JOIN ledger.finance_grant g
                  ON g.branch_id=a.branch_id AND g.actor_id=a.actor_id AND g.capability='payout_approve'
                  WHERE a.branch_id=branch AND a.request_key=proposal.request_key AND g.valid_until>clock_timestamp()) THEN
            RETURN;
        END IF;
        SELECT * INTO enrolled FROM ledger.issuance_profile p WHERE p.branch_id=branch FOR UPDATE;
        IF enrolled.authorized_digest IS DISTINCT FROM profile_ OR NOT enrolled.enabled THEN
            RETURN;
        END IF;
        -- Each finish_quote takes the conflicting profile FOR SHARE lock before
        -- committing. Pending signatures without a committed hash cannot cross.
        SELECT coalesce(array_agg(q.signed_digest ORDER BY q.signed_digest),ARRAY[]::bytea[]) INTO snapshot
          FROM ledger.sale s JOIN ledger.quote_work q ON q.sale_id=s.id
          WHERE s.branch_id=branch AND s.state IN ('issued','checking') AND q.signed_digest IS NOT NULL
            AND q.consumed_at IS NULL AND q.expires_at>clock_timestamp();
        IF cardinality(snapshot)>256 THEN RETURN; END IF;
        UPDATE ledger.issuance_profile p SET enabled=false WHERE p.branch_id=branch;
        UPDATE ledger.payout_change c SET phase='frozen',frozen_hashes=snapshot WHERE c.branch_id=branch
          AND c.request_key=proposal.request_key RETURNING c.* INTO proposal;
    END IF;
    RETURN QUERY SELECT proposal.request_key,proposal.new_address,proposal.old_address,proposal.old_version,proposal.phase,
       proposal.frozen_hashes,proposal.chain_tx_hash,proposal.signed_transaction;
END $$;

CREATE FUNCTION ledger.remember_payout_transaction(branch uuid,key_ uuid,hash_ bytea,raw_ bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    UPDATE ledger.payout_change c SET phase='signed',chain_tx_hash=hash_,signed_transaction=raw_
      WHERE c.branch_id=branch AND c.request_key=key_ AND c.phase='frozen';
    RETURN FOUND;
END $$;

CREATE FUNCTION ledger.confirm_payout(branch uuid,key_ uuid,hash_ bytea,block_ bytea,next_profile bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE proposal ledger.payout_change;
BEGIN
    SELECT * INTO proposal FROM ledger.payout_change c WHERE c.branch_id=branch AND c.request_key=key_ FOR UPDATE;
    IF NOT FOUND OR proposal.phase<>'signed' OR proposal.chain_tx_hash IS DISTINCT FROM hash_ THEN RETURN false; END IF;
    UPDATE ledger.issuance_profile p SET enabled=true,authorized_digest=next_profile,version=p.version+1
      WHERE p.branch_id=branch AND p.authorized_digest=proposal.profile_digest AND NOT p.enabled;
    IF NOT FOUND THEN RETURN false; END IF;
    UPDATE ledger.payout_change c SET phase='active',finalized_block=block_ WHERE c.branch_id=branch AND c.request_key=key_;
    RETURN true;
END $$;

REVOKE ALL ON FUNCTION ledger.claim_payout(uuid,bytea),ledger.remember_payout_transaction(uuid,uuid,bytea,bytea),
    ledger.confirm_payout(uuid,uuid,bytea,bytea,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.claim_payout(uuid,bytea),ledger.remember_payout_transaction(uuid,uuid,bytea,bytea),
    ledger.confirm_payout(uuid,uuid,bytea,bytea,bytea),ledger.active_payout(uuid) TO nu54_p05_payout_worker;

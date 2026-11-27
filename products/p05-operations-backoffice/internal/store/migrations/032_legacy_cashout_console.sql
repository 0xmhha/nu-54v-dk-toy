ALTER TABLE ledger.finance_grant DROP CONSTRAINT finance_grant_capability_check;
ALTER TABLE ledger.finance_grant ADD CONSTRAINT finance_grant_capability_check CHECK(capability IN (
    'refund_request','refund_approve','day_close','report_read','report_export','reconcile_read','reconcile_retry',
    'payout_propose','payout_approve','payout_notice','rental_manage','firmware_manage','firmware_station',
    'experience_read','experience_cancel','access_manage','audit_read','legacy_ledger','legacy_cashout','legacy_reconcile'
));
CREATE TABLE ledger.legacy_cashout_request(
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    key uuid NOT NULL,
    actor uuid NOT NULL REFERENCES ledger.login_subject(subject_ref),
    intent bytea NOT NULL CHECK(octet_length(intent)=32),
    review jsonb NOT NULL,
    candidate bytea CHECK(octet_length(candidate) BETWEEN 65 AND 800),
    outcome jsonb,
    dispatch_started boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(branch_id,key)
);
CREATE UNIQUE INDEX legacy_cashout_signer_slot ON ledger.legacy_cashout_request((1)) WHERE outcome IS NULL;
REVOKE ALL ON ledger.legacy_cashout_request FROM PUBLIC;

CREATE FUNCTION ledger.legacy_cashout_scope(tenant uuid,branch uuid,actor uuid,write_ boolean)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'legacy_ledger');
    IF write_ THEN PERFORM ledger.require_finance(tenant,branch,actor,'legacy_cashout'); END IF;
    RETURN EXISTS (SELECT FROM ledger.finance_grant g WHERE g.branch_id=branch AND g.actor_id=actor
        AND g.capability='legacy_cashout' AND g.valid_until>clock_timestamp());
END $$;

CREATE FUNCTION ledger.open_legacy_cashout(tenant uuid,branch uuid,actor_ uuid,key_ uuid,intent_ bytea,review_ jsonb)
RETURNS TABLE(candidate bytea,outcome jsonb) LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE prior ledger.legacy_cashout_request;
BEGIN
    PERFORM ledger.legacy_cashout_scope(tenant,branch,actor_,true);
    PERFORM pg_advisory_xact_lock(hashtextextended('nu54 legacy payout slot',33));
    PERFORM ledger.legacy_cashout_scope(tenant,branch,actor_,true);
    UPDATE ledger.legacy_cashout_request r SET outcome='{"status":"expired-before-signing"}'::jsonb
      WHERE r.outcome IS NULL AND r.candidate IS NULL AND r.created_at<=clock_timestamp()-interval '1 minute';
    SELECT * INTO prior FROM ledger.legacy_cashout_request r WHERE r.branch_id=branch AND r.key=key_ FOR UPDATE;
    IF FOUND THEN
        IF (prior.actor,prior.intent) IS DISTINCT FROM (actor_,intent_) THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='cashout request conflict';
        END IF;
        RETURN QUERY SELECT prior.candidate,prior.outcome;
        RETURN;
    END IF;
    INSERT INTO ledger.legacy_cashout_request(branch_id,key,actor,intent,review) VALUES(branch,key_,actor_,intent_,review_);
    RETURN QUERY SELECT NULL::bytea,NULL::jsonb;
END $$;

CREATE FUNCTION ledger.save_legacy_cashout(tenant uuid,branch uuid,actor_ uuid,key_ uuid,raw_ bytea)
RETURNS bytea LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE result bytea;
BEGIN
    PERFORM ledger.legacy_cashout_scope(tenant,branch,actor_,true);
    UPDATE ledger.legacy_cashout_request r SET candidate=coalesce(r.candidate,raw_)
      WHERE r.branch_id=branch AND r.key=key_ AND r.actor=actor_ AND r.outcome IS NULL
        AND (r.candidate IS NOT NULL OR r.created_at>clock_timestamp()-interval '1 minute')
      RETURNING r.candidate INTO result;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='cashout scope denied'; END IF;
    RETURN result;
END $$;

CREATE FUNCTION ledger.find_legacy_cashout(tenant uuid,branch uuid,actor_ uuid,key_ uuid)
RETURNS TABLE(candidate bytea,outcome jsonb,review jsonb) LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.legacy_cashout_scope(tenant,branch,actor_,false);
    RETURN QUERY SELECT r.candidate,r.outcome,r.review FROM ledger.legacy_cashout_request r
      WHERE r.branch_id=branch AND (key_ IS NULL OR r.key=key_) ORDER BY r.created_at DESC LIMIT 1;
END $$;

CREATE FUNCTION ledger.finish_legacy_cashout(tenant uuid,branch uuid,actor_ uuid,key_ uuid,result jsonb)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.legacy_cashout_scope(tenant,branch,actor_,false);
    UPDATE ledger.legacy_cashout_request r SET outcome=result
      WHERE r.branch_id=branch AND r.key=key_ AND r.candidate IS NOT NULL AND
      (r.outcome IS NULL OR r.outcome->>'status'='transfer-held')
      AND ((r.actor=actor_ AND EXISTS (SELECT FROM ledger.finance_grant g WHERE g.branch_id=branch AND g.actor_id=actor_
                AND g.capability='legacy_cashout' AND g.valid_until>clock_timestamp()))
         OR EXISTS (SELECT FROM ledger.finance_grant g WHERE g.branch_id=branch AND g.actor_id=actor_
                AND g.capability='legacy_reconcile' AND g.valid_until>clock_timestamp()));
END $$;

REVOKE ALL ON FUNCTION ledger.legacy_cashout_scope(uuid,uuid,uuid,boolean),ledger.open_legacy_cashout(uuid,uuid,uuid,uuid,bytea,jsonb),
    ledger.save_legacy_cashout(uuid,uuid,uuid,uuid,bytea),ledger.find_legacy_cashout(uuid,uuid,uuid,uuid),
    ledger.finish_legacy_cashout(uuid,uuid,uuid,uuid,jsonb) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.legacy_cashout_scope(uuid,uuid,uuid,boolean),ledger.open_legacy_cashout(uuid,uuid,uuid,uuid,bytea,jsonb),
    ledger.save_legacy_cashout(uuid,uuid,uuid,uuid,bytea),ledger.find_legacy_cashout(uuid,uuid,uuid,uuid),
    ledger.finish_legacy_cashout(uuid,uuid,uuid,uuid,jsonb) TO nu54_p05_commerce;

CREATE FUNCTION ledger.resume_legacy_cashout(tenant uuid,branch uuid,actor_ uuid,key_ uuid)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.legacy_cashout_scope(tenant,branch,actor_,false);
    PERFORM ledger.require_finance(tenant,branch,actor_,'legacy_reconcile');
    IF NOT EXISTS(SELECT FROM ledger.legacy_cashout_request r WHERE r.branch_id=branch AND r.key=key_ AND r.candidate IS NOT NULL) THEN
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='archived cashout scope denied';
    END IF;
END $$;
REVOKE ALL ON FUNCTION ledger.resume_legacy_cashout(uuid,uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.resume_legacy_cashout(uuid,uuid,uuid,uuid) TO nu54_p05_commerce;

CREATE FUNCTION ledger.dispatch_legacy_cashout(tenant uuid,branch uuid,actor_ uuid,key_ uuid,matched boolean,result jsonb)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE prior ledger.legacy_cashout_request;
BEGIN
    SELECT * INTO prior FROM ledger.legacy_cashout_request r WHERE r.branch_id=branch AND r.key=key_ FOR UPDATE;
    PERFORM ledger.legacy_cashout_scope(tenant,branch,actor_,false);
    IF prior.actor=actor_ THEN
        PERFORM ledger.require_finance(tenant,branch,actor_,'legacy_cashout');
    ELSE
        PERFORM ledger.require_finance(tenant,branch,actor_,'legacy_reconcile');
    END IF;
    IF prior.candidate IS NULL OR prior.outcome IS NOT NULL THEN RETURN false; END IF;
    IF NOT matched THEN
        IF NOT prior.dispatch_started THEN
            UPDATE ledger.legacy_cashout_request r SET outcome=result WHERE r.branch_id=branch AND r.key=key_;
        END IF;
        RETURN false;
    END IF;
    UPDATE ledger.legacy_cashout_request r SET dispatch_started=true WHERE r.branch_id=branch AND r.key=key_;
    RETURN true;
END $$;
REVOKE ALL ON FUNCTION ledger.dispatch_legacy_cashout(uuid,uuid,uuid,uuid,boolean,jsonb) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.dispatch_legacy_cashout(uuid,uuid,uuid,uuid,boolean,jsonb) TO nu54_p05_commerce;

CREATE FUNCTION ledger.can_resume_legacy_cashout(tenant uuid,branch uuid,actor_ uuid)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.legacy_cashout_scope(tenant,branch,actor_,false);
    RETURN EXISTS (SELECT FROM ledger.finance_grant g WHERE g.branch_id=branch AND g.actor_id=actor_
        AND g.capability='legacy_reconcile' AND g.valid_until>clock_timestamp());
END $$;
REVOKE ALL ON FUNCTION ledger.can_resume_legacy_cashout(uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.can_resume_legacy_cashout(uuid,uuid,uuid) TO nu54_p05_commerce;

CREATE TABLE ledger.rental_cleanup_intent (
    branch_id uuid NOT NULL,
    device_ref uuid NOT NULL,
    generation bigint NOT NULL,
    actor_id uuid NOT NULL REFERENCES ledger.login_subject(subject_ref),
    request_key uuid NOT NULL,
    disposition bytea NOT NULL CHECK(octet_length(disposition)=32),
    PRIMARY KEY(branch_id,device_ref,generation),
    UNIQUE(branch_id,actor_id,request_key),
    FOREIGN KEY(branch_id,device_ref,generation) REFERENCES ledger.rental_close_candidate
);
REVOKE ALL ON ledger.rental_cleanup_intent FROM PUBLIC,nu54_p05_commerce,nu54_p05_rental_close;

CREATE FUNCTION ledger.rental_cleanup_pending(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE pending jsonb;
BEGIN
    PERFORM ledger.rental_require_live_grant(tenant,branch,actor);
    SELECT jsonb_build_object('state','board-receipt-required')
        || CASE WHEN c.actor_id=actor THEN jsonb_build_object('requestKey',c.request_key) ELSE '{}'::jsonb END
      INTO pending FROM ledger.rental_cleanup_intent c
      WHERE (c.branch_id,c.device_ref,c.generation)=(branch,device,generation_);
    RETURN coalesce(pending,'{"state":"settlement-proof-required"}'::jsonb);
END $$;

CREATE FUNCTION ledger.rental_cleanup_freeze(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,
    writer_ bytea,scope_ text,close_ bytea)
RETURNS bytea LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE live jsonb;
DECLARE snapshot jsonb;
DECLARE frozen bytea;
BEGIN
    live:=ledger.rental_close_live_scope(tenant,branch,actor,device,generation_,writer_);
    snapshot:=ledger.rental_protected_withdrawal_snapshot(tenant,branch,actor,device,generation_);
    IF live->>'scopeDigest' IS DISTINCT FROM scope_ OR snapshot->>'current' IS DISTINCT FROM 'true'
       OR snapshot->>'closeHash' IS DISTINCT FROM '0x'||encode(close_,'hex') THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='current protected withdrawal and freeze required';
    END IF;
    SELECT original.freeze_digest INTO frozen
        FROM ledger.rental_close_candidate c
        JOIN ledger.rental_generation_authorized_proof original
          ON (original.branch_id,original.device_ref,original.generation,original.request_key)=
             (c.branch_id,c.device_ref,c.generation,c.proof_key)
        JOIN ledger.rental_generation_authorized_proof latest
          ON (latest.branch_id,latest.device_ref,latest.generation,latest.request_key)=
             (original.branch_id,original.device_ref,original.generation,(live->>'proofKey')::uuid)
        WHERE (c.branch_id,c.device_ref,c.generation)=(branch,device,generation_)
          AND c.state='confirmed'
          AND (latest.enrollment_digest,latest.journal_root,latest.first_nonce,latest.next_nonce,
               latest.payment_entries,latest.limit_entries,latest.burned_entries,latest.frozen_at)=
              (original.enrollment_digest,original.journal_root,original.first_nonce,original.next_nonce,
               original.payment_entries,original.limit_entries,original.burned_entries,original.frozen_at)
          AND latest.pending_payments=0 AND latest.pending_limits=0;
    IF frozen IS NULL THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='original zero-pending freeze required';
    END IF;
    RETURN frozen;
END $$;

CREATE FUNCTION ledger.admit_rental_cleanup(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,
    key_ uuid,writer_ bytea,scope_ text,close_ bytea,freeze_ bytea,disposition_ bytea,lease_ bigint)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE prior ledger.rental_cleanup_intent;
BEGIN
    PERFORM ledger.rental_require_live_grant(tenant,branch,actor);
    PERFORM ledger.lock_rental_evidence_device(branch,device);
    PERFORM ledger.rental_require_live_grant(tenant,branch,actor);
    SELECT * INTO prior FROM ledger.rental_cleanup_intent c WHERE (c.branch_id,c.device_ref,c.generation)=(branch,device,generation_);
    IF FOUND THEN
        IF (prior.actor_id,prior.request_key) IS DISTINCT FROM (actor,key_) THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='cleanup request already pinned';
        END IF;
        RETURN;
    END IF;
    IF lease_ IS NULL OR lease_<=extract(epoch FROM clock_timestamp()) OR
       disposition_ IS NULL OR disposition_=decode(repeat('00',32),'hex') OR
       ledger.rental_cleanup_freeze(tenant,branch,actor,device,generation_,writer_,scope_,close_) IS DISTINCT FROM freeze_ THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='cleanup authority changed';
    END IF;
    -- The proof and finance reads can wait on locks after the first lease check.
    IF lease_<=extract(epoch FROM clock_timestamp()) THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='cleanup authority expired';
    END IF;
    INSERT INTO ledger.rental_cleanup_intent VALUES(branch,device,generation_,actor,key_,disposition_);
EXCEPTION WHEN unique_violation THEN
    RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='cleanup request already pinned';
END $$;

REVOKE ALL ON FUNCTION ledger.rental_cleanup_pending(uuid,uuid,uuid,uuid,bigint),
    ledger.rental_cleanup_freeze(uuid,uuid,uuid,uuid,bigint,bytea,text,bytea),
    ledger.admit_rental_cleanup(uuid,uuid,uuid,uuid,bigint,uuid,bytea,text,bytea,bytea,bytea,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.rental_cleanup_pending(uuid,uuid,uuid,uuid,bigint),
    ledger.rental_cleanup_freeze(uuid,uuid,uuid,uuid,bigint,bytea,text,bytea),
    ledger.admit_rental_cleanup(uuid,uuid,uuid,uuid,bigint,uuid,bytea,text,bytea,bytea,bytea,bigint) TO nu54_p05_rental_close;

CREATE FUNCTION ledger.rental_predecessor_has_unverified_cleanup(branch uuid,device uuid,generation_ bigint)
RETURNS boolean LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT EXISTS (SELECT FROM ledger.rental_close_candidate c
        WHERE (c.branch_id,c.device_ref)=(branch,device) AND c.generation<generation_)
      OR EXISTS (SELECT FROM ledger.rental_operation o
        WHERE (o.branch_id,o.device_ref)=(branch,device) AND o.generation<generation_
          AND o.action='cleanup')
$$;
REVOKE ALL ON FUNCTION ledger.rental_predecessor_has_unverified_cleanup(uuid,uuid,bigint) FROM PUBLIC;

CREATE FUNCTION ledger.rental_successor_cleanup_blocked(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.rental_require_live_grant(tenant,branch,actor);
    RETURN ledger.rental_predecessor_has_unverified_cleanup(branch,device,generation_);
END $$;
REVOKE ALL ON FUNCTION ledger.rental_successor_cleanup_blocked(uuid,uuid,uuid,uuid,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.rental_successor_cleanup_blocked(uuid,uuid,uuid,uuid,bigint) TO nu54_p05_commerce;

CREATE OR REPLACE FUNCTION ledger.lock_rental_enrollment_insert()
RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.lock_rental_evidence_device(NEW.branch_id,NEW.device_ref);
    IF ledger.rental_predecessor_has_unverified_cleanup(NEW.branch_id,NEW.device_ref,NEW.generation) THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='verified predecessor board cleanup required';
    END IF;
    RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION ledger.begin_rental_action(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,
    action_ text,key_ uuid,amount_ numeric,review_ text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF action_ IN ('open','cleanup') THEN
        PERFORM ledger.rental_require_live_grant(tenant,branch,actor);
        PERFORM ledger.lock_rental_evidence_device(branch,device);
        IF action_='cleanup' OR (ledger.rental_predecessor_has_unverified_cleanup(branch,device,generation_)
           AND NOT EXISTS (SELECT FROM ledger.rental_operation o
              WHERE (o.branch_id,o.device_ref,o.generation,o.action)=(branch,device,generation_,'open'))) THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='verified board cleanup required';
        END IF;
    ELSIF action_='close' THEN
        PERFORM ledger.lock_rental_evidence_device(branch,device);
        IF ledger.rental_runtime_close_hold(tenant,branch,actor,device,generation_)
           AND NOT EXISTS (SELECT FROM ledger.rental_operation o
             WHERE (o.branch_id,o.device_ref,o.generation,o.action)=(branch,device,generation_,'close')) THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='live rental chain authority required';
        END IF;
    END IF;
    RETURN ledger.begin_rental_action_with_cached_proof(tenant,branch,actor,device,generation_,action_,key_,amount_,review_);
END $$;

CREATE OR REPLACE FUNCTION ledger.record_rental_action(tenant uuid,branch uuid,actor uuid,device uuid,generation_ bigint,
    action_ text,key_ uuid,status_ text,evidence_ jsonb)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'rental_manage');
    IF action_='cleanup' THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='legacy cleanup records are historical only';
    END IF;
    IF status_ NOT IN ('submitted','unknown','acknowledged') THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid rental execution status';
    END IF;
    UPDATE ledger.rental_operation SET status=status_,evidence=evidence_
      WHERE branch_id=branch AND device_ref=device AND generation=generation_ AND action=action_
        AND request_key=key_ AND actor_id=actor AND status='dispatching';
    RETURN FOUND;
END $$;

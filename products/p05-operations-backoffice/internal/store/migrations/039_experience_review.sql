CREATE OR REPLACE FUNCTION ledger.operator_cancel_subscription(tenant uuid,branch uuid,actor uuid,subject uuid,key_ uuid,revision_ bigint)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE prior ledger.operator_subscription_cancel;
DECLARE schedule ledger.subscription;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'experience_cancel');
    PERFORM ledger.require_finance(tenant,branch,actor,'experience_read');
    IF NOT EXISTS(SELECT FROM ledger.login_subject WHERE subject_ref=actor) THEN
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='experience decision requires enrolled operator';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||actor::text||key_::text,28));
    SELECT * INTO prior FROM ledger.operator_subscription_cancel c WHERE c.branch_id=branch AND c.actor_id=actor AND c.key=key_;
    IF NOT ledger.request_authority_live(tenant,branch,actor,'operator',ARRAY['experience_read'])
        OR NOT ledger.request_authority_live(tenant,branch,actor,'operator',ARRAY['experience_cancel']) THEN
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='experience authority ended';
    END IF;
    IF FOUND THEN
        IF (prior.subscription_id,prior.reviewed_revision) IS DISTINCT FROM (subject,revision_) THEN
            RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='cancel request conflict';
        END IF;
        RETURN ledger.operator_experience(tenant,branch,actor,subject);
    END IF;
    SELECT * INTO schedule FROM ledger.subscription s WHERE s.branch_id=branch AND s.id=subject FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='experience scope denied'; END IF;
    IF NOT ledger.request_authority_live(tenant,branch,actor,'operator',ARRAY['experience_read'])
        OR NOT ledger.request_authority_live(tenant,branch,actor,'operator',ARRAY['experience_cancel']) THEN
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='experience authority ended';
    END IF;
    IF schedule.revision<>revision_ OR schedule.state='cancelled' THEN
        RAISE EXCEPTION USING ERRCODE='P0501',MESSAGE='subscription changed';
    END IF;
    UPDATE ledger.subscription s SET state='cancelled',cancelled_at=clock_timestamp(),
        next_cycle=NULL,next_due_at=NULL,revision=s.revision+1 WHERE s.branch_id=branch AND s.id=subject;
    INSERT INTO ledger.operator_subscription_cancel VALUES(branch,actor,key_,subject,revision_,clock_timestamp());
    RETURN ledger.operator_experience(tenant,branch,actor,subject);
END $$;

CREATE OR REPLACE FUNCTION ledger.experience_with_timezones(branch uuid, snapshot jsonb)
RETURNS jsonb LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $$
    SELECT jsonb_build_object(
      'bookings',coalesce((SELECT jsonb_agg(b.value || jsonb_build_object('timezone',site.timezone,'provider',coalesce(site_name.label,'등록 예약 공급 지점 · '||right(site.id::text,8))) ORDER BY b.seq)
        FROM jsonb_array_elements(snapshot->'bookings') WITH ORDINALITY b(value,seq)
        JOIN ledger.branch site ON site.id=branch LEFT JOIN ledger.operator_branch_label site_name ON site_name.branch_id=site.id),'[]'::jsonb),
      'subscriptions',coalesce((SELECT jsonb_agg(s.value || jsonb_build_object('timezone',model.timezone) ORDER BY s.seq)
        FROM jsonb_array_elements(snapshot->'subscriptions') WITH ORDINALITY s(value,seq)
        JOIN ledger.subscription model ON model.branch_id=branch AND model.id=(s.value->>'id')::uuid),'[]'::jsonb));
$$;

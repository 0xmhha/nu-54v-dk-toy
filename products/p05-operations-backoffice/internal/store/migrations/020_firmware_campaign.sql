ALTER TABLE ledger.finance_grant DROP CONSTRAINT finance_grant_capability_check;
ALTER TABLE ledger.finance_grant ADD CONSTRAINT finance_grant_capability_check CHECK(capability IN (
    'refund_request','refund_approve','day_close','report_read','report_export','reconcile_read','reconcile_retry',
    'payout_propose','payout_approve','payout_notice','rental_manage','firmware_manage','firmware_station'
));

CREATE TABLE ledger.firmware_member(
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    device_ref uuid NOT NULL,
    revision text NOT NULL CHECK(revision ~ '^[a-z0-9_-]{1,32}$'),
    board_id bytea NOT NULL UNIQUE CHECK(octet_length(board_id)=32),
    identity_p256 bytea NOT NULL UNIQUE CHECK(octet_length(identity_p256)=65),
    installed_counter integer NOT NULL CHECK(installed_counter>0),
    installed_version text NOT NULL CHECK(length(installed_version) BETWEEN 1 AND 40),
    withdrawn boolean NOT NULL,
    no_pending_until timestamptz CHECK(isfinite(no_pending_until)),
    PRIMARY KEY(branch_id,device_ref)
);
CREATE TABLE ledger.firmware_campaign(
    branch_id uuid NOT NULL,
    device_ref uuid NOT NULL,
    campaign_ref uuid NOT NULL,
    actor_id uuid NOT NULL REFERENCES ledger.login_subject(subject_ref),
    release_digest text NOT NULL CHECK(release_digest ~ '^[0-9a-f]{64}$'),
    version text NOT NULL CHECK(length(version) BETWEEN 1 AND 40),
    counter integer NOT NULL CHECK(counter>0),
    measurement bytea NOT NULL CHECK(octet_length(measurement)=32),
    challenge bytea NOT NULL CHECK(octet_length(challenge)=32),
    challenge_until timestamptz NOT NULL CHECK(isfinite(challenge_until)),
    state text NOT NULL CHECK(state IN ('running','paused','verified')),
    proof_digest bytea CHECK(octet_length(proof_digest)=32),
    started_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(branch_id,campaign_ref,device_ref),
    FOREIGN KEY(branch_id,device_ref) REFERENCES ledger.firmware_member,
    CHECK((state='verified')=(proof_digest IS NOT NULL))
);
CREATE UNIQUE INDEX firmware_one_inflight ON ledger.firmware_campaign(branch_id,device_ref) WHERE state<>'verified';
REVOKE ALL ON ledger.firmware_member,ledger.firmware_campaign FROM PUBLIC;

CREATE FUNCTION ledger.firmware_scope(tenant uuid,branch uuid,actor uuid)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    IF NOT EXISTS (SELECT FROM ledger.finance_grant g JOIN ledger.branch b ON b.id=g.branch_id
        WHERE b.id=branch AND b.tenant_ref=tenant AND g.actor_id=actor AND
          g.capability IN ('firmware_manage','firmware_station') AND g.valid_until>clock_timestamp()) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='firmware scope denied';
    END IF;
END $$;

CREATE FUNCTION ledger.firmware_status(tenant uuid,branch uuid,actor uuid,device uuid,campaign uuid)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE subject ledger.firmware_member;
DECLARE work ledger.firmware_campaign;
BEGIN
    PERFORM ledger.firmware_scope(tenant,branch,actor);
    SELECT * INTO subject FROM ledger.firmware_member d WHERE d.branch_id=branch AND d.device_ref=device;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='invalid firmware target'; END IF;
    SELECT * INTO work FROM ledger.firmware_campaign c WHERE c.branch_id=branch AND c.device_ref=device
        AND (campaign IS NULL OR c.campaign_ref=campaign) ORDER BY c.started_at DESC LIMIT 1;
    RETURN jsonb_build_object('revision',subject.revision,'installedVersion',subject.installed_version,
        'installedCounter',subject.installed_counter,
        'hold',subject.withdrawn OR coalesce(subject.no_pending_until<=clock_timestamp(),true),
        'campaign',work.campaign_ref,'state',work.state,'version',work.version,'counter',work.counter,
        'releaseDigest',work.release_digest,'challengeUntil',work.challenge_until);
END $$;

CREATE FUNCTION ledger.begin_firmware(tenant uuid,branch uuid,actor uuid,device uuid,campaign uuid,
    release_ text,version_ text,counter_ integer,revision_ text,measurement_ bytea,challenge_ bytea)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE subject ledger.firmware_member;
DECLARE work ledger.firmware_campaign;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'firmware_manage');
    PERFORM pg_advisory_xact_lock(hashtextextended(branch::text||campaign::text,27));
    IF EXISTS (SELECT FROM ledger.firmware_campaign c WHERE c.branch_id=branch AND c.campaign_ref=campaign
         AND (c.release_digest<>release_ OR c.actor_id<>actor)) THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='cohort release scope differs';
    END IF;
    SELECT * INTO subject FROM ledger.firmware_member d WHERE d.branch_id=branch AND d.device_ref=device FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='invalid firmware target'; END IF;
    SELECT * INTO work FROM ledger.firmware_campaign c WHERE c.branch_id=branch AND c.campaign_ref=campaign AND c.device_ref=device;
    IF FOUND THEN
        IF (work.actor_id,work.device_ref,work.release_digest) IS DISTINCT FROM (actor,device,release_) THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='campaign scope differs';
        END IF;
    ELSE
        IF subject.withdrawn OR subject.no_pending_until IS NULL OR subject.no_pending_until<=clock_timestamp()
           OR subject.revision<>revision_ OR subject.installed_counter>=counter_ THEN
            RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='firmware activation held';
        END IF;
        INSERT INTO ledger.firmware_campaign(branch_id,device_ref,campaign_ref,actor_id,release_digest,version,counter,
            measurement,challenge,challenge_until,state) VALUES(branch,device,campaign,actor,release_,version_,counter_,
            measurement_,challenge_,clock_timestamp()+interval '10 minutes','running');
    END IF;
    RETURN ledger.firmware_status(tenant,branch,actor,device,campaign);
END $$;

CREATE FUNCTION ledger.control_firmware(tenant uuid,branch uuid,actor uuid,device uuid,campaign uuid,
    next_ text,challenge_ bytea)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE subject ledger.firmware_member;
DECLARE work ledger.firmware_campaign;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'firmware_manage');
    SELECT * INTO subject FROM ledger.firmware_member d WHERE d.branch_id=branch AND d.device_ref=device FOR UPDATE;
    SELECT * INTO work FROM ledger.firmware_campaign c WHERE c.branch_id=branch AND c.device_ref=device
        AND c.campaign_ref=campaign FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='campaign scope denied'; END IF;
    IF work.state='verified' THEN RETURN ledger.firmware_status(tenant,branch,actor,device,campaign); END IF;
    IF next_='running' AND (subject.withdrawn OR subject.no_pending_until IS NULL OR subject.no_pending_until<=clock_timestamp()
       OR subject.installed_counter>=work.counter) THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='firmware activation held';
    END IF;
    IF next_='paused' THEN
        UPDATE ledger.firmware_campaign c SET state=next_ WHERE c.branch_id=branch AND c.campaign_ref=campaign AND c.device_ref=device;
    ELSIF next_='running' AND (work.state='paused' OR work.challenge_until<=clock_timestamp()) THEN
        UPDATE ledger.firmware_campaign c SET state=next_,challenge=challenge_,
            challenge_until=clock_timestamp()+interval '10 minutes' WHERE c.branch_id=branch AND c.campaign_ref=campaign AND c.device_ref=device;
    END IF;
    RETURN ledger.firmware_status(tenant,branch,actor,device,campaign);
END $$;

CREATE FUNCTION ledger.firmware_station(tenant uuid,branch uuid,actor uuid,device uuid,campaign uuid)
RETURNS TABLE(release_digest text,challenge bytea,board_id bytea,identity_p256 bytea,measurement bytea)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'firmware_station');
    RETURN QUERY SELECT c.release_digest,c.challenge,d.board_id,d.identity_p256,c.measurement
    FROM ledger.firmware_campaign c JOIN ledger.firmware_member d USING(branch_id,device_ref)
    WHERE c.branch_id=branch AND c.device_ref=device AND c.campaign_ref=campaign AND c.state='running'
        AND c.challenge_until>clock_timestamp() AND NOT d.withdrawn AND d.no_pending_until>clock_timestamp();
END $$;

CREATE FUNCTION ledger.firmware_ack(tenant uuid,branch uuid,actor uuid,device uuid,campaign uuid,
    challenge_ bytea,receipt_ bytea)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
DECLARE subject ledger.firmware_member;
DECLARE work ledger.firmware_campaign;
BEGIN
    PERFORM ledger.require_finance(tenant,branch,actor,'firmware_station');
    SELECT * INTO subject FROM ledger.firmware_member d WHERE d.branch_id=branch AND d.device_ref=device FOR UPDATE;
    SELECT * INTO work FROM ledger.firmware_campaign c WHERE c.branch_id=branch AND c.device_ref=device AND c.campaign_ref=campaign FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='campaign scope denied'; END IF;
    IF work.state='verified' AND work.proof_digest=receipt_ THEN
        RETURN ledger.firmware_status(tenant,branch,actor,device,campaign);
    END IF;
    IF work.state<>'running' OR work.challenge IS DISTINCT FROM challenge_ OR work.challenge_until<=clock_timestamp() OR
       subject.withdrawn OR subject.no_pending_until IS NULL OR subject.no_pending_until<=clock_timestamp() OR
       subject.installed_counter>=work.counter THEN
        RAISE EXCEPTION USING ERRCODE='P0501', MESSAGE='firmware acknowledgement no longer applicable';
    END IF;
    UPDATE ledger.firmware_member d SET installed_counter=work.counter,installed_version=work.version
      WHERE d.branch_id=branch AND d.device_ref=device;
    UPDATE ledger.firmware_campaign c SET state='verified',proof_digest=receipt_ WHERE c.branch_id=branch AND c.campaign_ref=campaign AND c.device_ref=device;
    RETURN ledger.firmware_status(tenant,branch,actor,device,campaign);
END $$;

REVOKE ALL ON FUNCTION ledger.firmware_scope(uuid,uuid,uuid),ledger.firmware_status(uuid,uuid,uuid,uuid,uuid),
    ledger.begin_firmware(uuid,uuid,uuid,uuid,uuid,text,text,integer,text,bytea,bytea),
    ledger.control_firmware(uuid,uuid,uuid,uuid,uuid,text,bytea),ledger.firmware_station(uuid,uuid,uuid,uuid,uuid),
    ledger.firmware_ack(uuid,uuid,uuid,uuid,uuid,bytea,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.firmware_status(uuid,uuid,uuid,uuid,uuid),
    ledger.begin_firmware(uuid,uuid,uuid,uuid,uuid,text,text,integer,text,bytea,bytea),
    ledger.control_firmware(uuid,uuid,uuid,uuid,uuid,text,bytea),ledger.firmware_station(uuid,uuid,uuid,uuid,uuid),
    ledger.firmware_ack(uuid,uuid,uuid,uuid,uuid,bytea,bytea) TO nu54_p05_commerce;

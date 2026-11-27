CREATE TABLE chainview.replay_job (
    id uuid PRIMARY KEY,
    stream_id uuid NOT NULL,
    previous_id uuid NOT NULL,
    expected_revision bigint NOT NULL,
    anchor_height bigint NOT NULL,
    anchor_hash text NOT NULL,
    state text NOT NULL CHECK (state IN ('running','matched','diff')),
    reason text CHECK (reason IN ('projection_diff','anchor_changed')),
    comparison_id uuid REFERENCES chainview.projection_comparison,
    requested_at timestamptz NOT NULL,
    finished_at timestamptz,
    differing_logs bigint CHECK (differing_logs>=0),
    paid_delta numeric,
    refund_delta numeric,
    FOREIGN KEY (stream_id,id) REFERENCES chainview.generation(stream_id,id),
    FOREIGN KEY (stream_id,previous_id) REFERENCES chainview.generation(stream_id,id),
    CHECK ((state='running')=(finished_at IS NULL)),
    CHECK ((state='matched')=(comparison_id IS NOT NULL)),
    CHECK ((state='diff')=(reason IS NOT NULL)),
    CHECK ((differing_logs IS NOT NULL AND paid_delta IS NOT NULL AND refund_delta IS NOT NULL)
        = (state='matched' OR COALESCE(reason='projection_diff',false))),
    CHECK (num_nonnulls(differing_logs,paid_delta,refund_delta) IN (0,3))
);
CREATE UNIQUE INDEX replay_job_one_running ON chainview.replay_job(stream_id) WHERE state='running';

DO $roles$
BEGIN
    IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='nu54_p07_replay_request') THEN
        CREATE ROLE nu54_p07_replay_request NOLOGIN;
    ELSIF EXISTS(SELECT FROM pg_roles WHERE rolname='nu54_p07_replay_request'
        AND (rolcanlogin OR rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb OR rolreplication)) THEN
        RAISE EXCEPTION 'unsafe replay request role';
    END IF;
END
$roles$;
GRANT USAGE ON SCHEMA chainview TO nu54_p07_replay_request;
GRANT SELECT ON chainview.replay_job TO nu54_p07_ingest;
CREATE VIEW chainview.explorer_replay_status WITH (security_barrier=true) AS
SELECT j.*,g.tip_height AS replayed_height
FROM chainview.replay_job j JOIN chainview.generation g ON g.id=j.id AND g.stream_id=j.stream_id;
GRANT SELECT ON chainview.explorer_replay_status TO nu54_p07_explorer;

CREATE FUNCTION chainview.start_replay(p_stream uuid,p_job uuid,p_revision bigint,p_expires timestamptz)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE active chainview.read_target;
DECLARE observed chainview.generation;
DECLARE prior chainview.replay_job;
BEGIN
    PERFORM pg_advisory_xact_lock(hashtextextended(p_stream::text,61));
    IF p_expires<=clock_timestamp() THEN
        RAISE EXCEPTION 'replay grant ended' USING ERRCODE='42501';
    END IF;
    SELECT * INTO prior FROM chainview.replay_job WHERE id=p_job;
    IF FOUND THEN
        IF prior.stream_id<>p_stream OR prior.expected_revision<>p_revision THEN
            RAISE EXCEPTION 'replay request identity differs' USING ERRCODE='55000';
        END IF;
        RETURN;
    END IF;
    IF EXISTS(SELECT FROM chainview.replay_job WHERE stream_id=p_stream AND state='running') THEN
        RAISE EXCEPTION 'a replay is already running' USING ERRCODE='55000';
    END IF;
    SELECT * INTO active FROM chainview.read_target WHERE stream_id=p_stream FOR SHARE;
    SELECT * INTO observed FROM chainview.generation WHERE id=active.generation_id FOR SHARE;
    IF NOT FOUND OR active.revision<>p_revision OR observed.state<>'ready' OR
       NOT COALESCE(chainview.complete_prefix(observed.id,observed.finalized_height),false) THEN
        RAISE EXCEPTION 'replay needs a complete active checkpoint' USING ERRCODE='55000';
    END IF;
    IF p_expires<=clock_timestamp() THEN
        RAISE EXCEPTION 'replay grant ended' USING ERRCODE='42501';
    END IF;
    INSERT INTO chainview.generation(id,stream_id,base_height,fence,state)
        VALUES(p_job,p_stream,observed.base_height,0,'building');
    INSERT INTO chainview.replay_job(id,stream_id,previous_id,expected_revision,anchor_height,anchor_hash,state,requested_at)
         VALUES(p_job,p_stream,observed.id,p_revision,observed.finalized_height,observed.finalized_hash,'running',clock_timestamp());
END
$$;
REVOKE ALL ON FUNCTION chainview.start_replay(uuid,uuid,bigint,timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION chainview.start_replay(uuid,uuid,bigint,timestamptz) TO nu54_p07_replay_request;

CREATE FUNCTION chainview.finish_replay(p_job uuid,p_diverged boolean)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE job chainview.replay_job;
DECLARE candidate chainview.generation;
DECLARE current_target chainview.read_target;
DECLARE current_generation chainview.generation;
DECLARE proof uuid;
DECLARE diff_reason text;
DECLARE logs_delta bigint;
DECLARE pays_delta numeric;
DECLARE refunds_delta numeric;
BEGIN
    SELECT * INTO job FROM chainview.replay_job WHERE id=p_job FOR UPDATE;
    IF NOT FOUND OR job.state<>'running' THEN RETURN; END IF;
    PERFORM pg_advisory_xact_lock_shared(hashtextextended(job.stream_id::text,9));
    IF p_diverged THEN
        UPDATE chainview.replay_job SET state='diff',reason='anchor_changed',finished_at=clock_timestamp() WHERE id=p_job;
        RETURN;
    END IF;
    SELECT * INTO candidate FROM chainview.generation WHERE id=p_job AND stream_id=job.stream_id FOR SHARE;
    IF NOT FOUND OR candidate.finalized_height IS NULL OR candidate.finalized_height<job.anchor_height THEN RETURN; END IF;
    SELECT * INTO current_target FROM chainview.read_target WHERE stream_id=job.stream_id FOR SHARE;
    SELECT * INTO current_generation FROM chainview.generation WHERE id=current_target.generation_id FOR SHARE;
    IF current_target.revision IS DISTINCT FROM job.expected_revision OR
       current_target.generation_id IS DISTINCT FROM job.previous_id OR
       current_generation.finalized_height IS DISTINCT FROM job.anchor_height OR
       current_generation.finalized_hash IS DISTINCT FROM job.anchor_hash OR
       candidate.finalized_height IS DISTINCT FROM job.anchor_height OR
       candidate.finalized_hash IS DISTINCT FROM job.anchor_hash OR
       NOT COALESCE(chainview.complete_prefix(candidate.id,job.anchor_height),false) OR
       NOT COALESCE(chainview.complete_prefix(current_generation.id,job.anchor_height),false) OR
       EXISTS(SELECT FROM chainview.incident i JOIN chainview.generation affected ON affected.id=i.generation_id
           WHERE affected.stream_id=job.stream_id AND i.resolved_at IS NULL) THEN
        diff_reason:='anchor_changed';
    ELSE
        BEGIN
            proof:=chainview.record_comparison(job.stream_id,p_job,job.expected_revision);
        EXCEPTION WHEN object_not_in_prerequisite_state THEN
            diff_reason:='projection_diff';
            WITH facts AS (
                SELECT e.generation_id,e.block_hash,e.tx_hash,e.log_index,e.kind,e.merchant_key,
                    e.order_key,e.refund_key,e.asset_ref,e.amount,e.payer_ref,e.recipient_ref,e.signer_nonce,
                    e.quote_digest,e.payment_mode,e.payer_generation
                FROM chainview.observed_event e JOIN chainview.block b USING(generation_id,block_hash)
                WHERE e.generation_id IN (job.previous_id,p_job) AND b.canonical AND b.height<=job.anchor_height
            ), compared AS (
                SELECT block_hash,tx_hash,log_index,kind,merchant_key,order_key,refund_key,asset_ref,
                    amount,payer_ref,recipient_ref,signer_nonce,quote_digest,payment_mode,payer_generation
                FROM facts GROUP BY block_hash,tx_hash,log_index,kind,merchant_key,order_key,refund_key,asset_ref,
                    amount,payer_ref,recipient_ref,signer_nonce,quote_digest,payment_mode,payer_generation
                HAVING count(DISTINCT generation_id)<>2
            )
            SELECT (SELECT count(*) FROM (SELECT DISTINCT block_hash,tx_hash,log_index FROM compared) differing),
                   COALESCE(sum(CASE WHEN generation_id=p_job THEN amount ELSE -amount END) FILTER(WHERE kind='paid'),0),
                   COALESCE(sum(CASE WHEN generation_id=p_job THEN amount ELSE -amount END) FILTER(WHERE kind='refunded'),0)
            INTO logs_delta,pays_delta,refunds_delta FROM facts;
        END;
    END IF;
    IF proof IS NOT NULL THEN logs_delta:=0; pays_delta:=0; refunds_delta:=0; END IF;
    UPDATE chainview.replay_job SET state=CASE WHEN proof IS NOT NULL THEN 'matched' ELSE 'diff' END,
           reason=diff_reason,comparison_id=proof,finished_at=clock_timestamp(),
           differing_logs=logs_delta,paid_delta=pays_delta,refund_delta=refunds_delta WHERE id=p_job;
END
$$;
REVOKE ALL ON FUNCTION chainview.finish_replay(uuid,boolean) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION chainview.finish_replay(uuid,boolean) TO nu54_p07_ingest;

CREATE TABLE platform.fleet_verification (
    candidate text NOT NULL REFERENCES platform.fleet_candidate,
    artifact text NOT NULL,
    kind text NOT NULL CHECK (kind IN ('rescan','regression','artifactSigning','sbom')),
    evidence jsonb NOT NULL,
    PRIMARY KEY(candidate,artifact,kind)
);

CREATE TABLE platform.fleet_hold_enrollment (
    login_role name NOT NULL,
    actor uuid NOT NULL,
    cohort uuid NOT NULL REFERENCES platform.fleet_cohort,
    owner_role text NOT NULL CHECK (owner_role ~ '^[a-z][a-z0-9-]{0,63}$'),
    expires_at timestamptz NOT NULL CHECK (isfinite(expires_at)),
    PRIMARY KEY(login_role,actor,cohort,owner_role)
);

CREATE TABLE platform.fleet_advisory_hold (
    advisory uuid NOT NULL REFERENCES platform.fleet_advisory,
    sequence bigint NOT NULL CHECK(sequence>0),
    request_id uuid NOT NULL,
    actor uuid NOT NULL,
    owner_role text NOT NULL,
    reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 512),
    review_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(advisory,sequence),
    UNIQUE(advisory,request_id)
);

CREATE OR REPLACE VIEW platform.fleet_public_cohort WITH (security_barrier=true) AS
SELECT c.id, c.revision, c.current AS manifest, b.signer,
       b.metadata::jsonb->>'channel' AS channel
FROM platform.fleet_cohort c JOIN platform.fleet_candidate b ON b.id=c.current
WHERE c.catalog_public
AND NOT EXISTS (SELECT FROM platform.fleet_revocation r WHERE (r.cohort,r.key)=(c.id,b.signer))
AND NOT EXISTS (SELECT FROM jsonb_each(b.metadata::jsonb->'artifacts') item
                WHERE COALESCE((item.value->>'fixtureEvidence')::boolean,true))
AND NOT EXISTS (SELECT FROM platform.fleet_advisory_hold h JOIN platform.fleet_advisory a ON a.id=h.advisory
                WHERE a.cohort=c.id AND a.report::jsonb->>'candidate'=c.current);

DO $role$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p10_fleet_hold') THEN
        CREATE ROLE nu54_p10_fleet_hold NOLOGIN NOSUPERUSER NOBYPASSRLS;
    ELSIF EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p10_fleet_hold'
        AND (rolcanlogin OR rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication OR rolbypassrls)) THEN
        RAISE EXCEPTION 'unsafe fleet hold capability';
    END IF;
END
$role$;

GRANT USAGE ON SCHEMA platform TO nu54_p10_fleet_hold;
GRANT SELECT,INSERT ON platform.fleet_verification TO nu54_p10_fleet_service;
GRANT SELECT ON platform.fleet_advisory_hold TO nu54_p10_fleet_service;

CREATE FUNCTION platform.fleet_hold_authorized(scope_ uuid,actor_ uuid,role_ text,lock_ boolean)
RETURNS boolean LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $hold$
DECLARE expiry timestamptz;
BEGIN
    IF lock_ THEN
        SELECT expires_at INTO expiry FROM platform.fleet_hold_enrollment
        WHERE (login_role,actor,cohort,owner_role)=(session_user,actor_,scope_,role_) FOR SHARE;
    ELSE
        SELECT expires_at INTO expiry FROM platform.fleet_hold_enrollment
        WHERE (login_role,actor,cohort,owner_role)=(session_user,actor_,scope_,role_);
    END IF;
    RETURN COALESCE(expiry>clock_timestamp(),false);
END
$hold$;

CREATE FUNCTION platform.place_fleet_hold(scope_ uuid, advisory_ uuid, candidate_ text, expected_ bigint,
    request_ uuid, actor_ uuid, role_ text, reason_ text, review_ bigint, deadline_ bigint)
RETURNS boolean LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $hold$
DECLARE sequence_ bigint;
DECLARE prior platform.fleet_advisory_hold%ROWTYPE;
BEGIN
    PERFORM pg_advisory_xact_lock(440060);
    IF clock_timestamp()>=to_timestamp(deadline_) OR NOT platform.fleet_hold_authorized(scope_,actor_,role_,true) THEN
        RETURN false;
    END IF;
    PERFORM 1 FROM platform.fleet_advisory WHERE (id,cohort)=(advisory_,scope_)
      AND report::jsonb->>'candidate'=candidate_ AND published_job IS NULL FOR UPDATE;
    IF NOT FOUND THEN RETURN false; END IF;
    SELECT * INTO prior FROM platform.fleet_advisory_hold WHERE (advisory,request_id)=(advisory_,request_);
    IF FOUND THEN
        RETURN (prior.sequence,prior.actor,prior.owner_role,prior.reason,prior.review_at)
           IS NOT DISTINCT FROM (expected_+1,actor_,role_,reason_,to_timestamp(review_));
    END IF;
    IF to_timestamp(review_)<=clock_timestamp() OR to_timestamp(review_)>clock_timestamp()+interval '365 days' THEN
        RETURN false;
    END IF;
    SELECT COALESCE(max(sequence),0) INTO sequence_ FROM platform.fleet_advisory_hold WHERE advisory=advisory_;
    IF sequence_<>expected_ THEN RETURN false; END IF;
    INSERT INTO platform.fleet_advisory_hold(advisory,sequence,request_id,actor,owner_role,reason,review_at)
      VALUES(advisory_,sequence_+1,request_,actor_,role_,reason_,to_timestamp(review_));
    UPDATE platform.advisory_case SET decision='hold',owner_role=role_,next_review_at=to_timestamp(review_)
      WHERE advisory_ref=advisory_;
    IF clock_timestamp()>=to_timestamp(deadline_) OR NOT platform.fleet_hold_authorized(scope_,actor_,role_,true) THEN
        RAISE EXCEPTION 'fleet hold grant ended';
    END IF;
    RETURN true;
END
$hold$;

CREATE FUNCTION platform.fleet_review_overlay(scope_ uuid) RETURNS jsonb
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $review$
    SELECT COALESCE(jsonb_object_agg(a.id, jsonb_build_object('checks',COALESCE((
        SELECT jsonb_object_agg(p.artifact,p.evidence) FROM (
           SELECT v.artifact,jsonb_object_agg(v.kind,v.evidence) AS evidence
           FROM platform.fleet_verification v WHERE v.candidate=a.report::jsonb->>'candidate'
             AND (a.report::jsonb->'oldVersions') ? v.artifact GROUP BY v.artifact
        ) p
      ),'{}'::jsonb),'hold', (
        SELECT jsonb_build_object('revision',h.sequence,'owner',h.owner_role,'reason',h.reason,
            'reviewBy',extract(epoch FROM h.review_at)::bigint)
        FROM platform.fleet_advisory_hold h WHERE h.advisory=a.id ORDER BY h.sequence DESC LIMIT 1
      ),'published',a.published_job IS NOT NULL)), '{}'::jsonb)
    FROM platform.fleet_advisory a WHERE a.cohort=scope_ AND platform.fleet_review_authorized(scope_)
$review$;

REVOKE ALL ON FUNCTION platform.fleet_hold_authorized(uuid,uuid,text,boolean) FROM PUBLIC;
REVOKE ALL ON FUNCTION platform.place_fleet_hold(uuid,uuid,text,bigint,uuid,uuid,text,text,bigint,bigint) FROM PUBLIC;
REVOKE ALL ON FUNCTION platform.fleet_review_overlay(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.fleet_hold_authorized(uuid,uuid,text,boolean),
    platform.place_fleet_hold(uuid,uuid,text,bigint,uuid,uuid,text,text,bigint,bigint) TO nu54_p10_fleet_hold;
GRANT EXECUTE ON FUNCTION platform.fleet_review_overlay(uuid) TO nu54_p10_fleet_review;

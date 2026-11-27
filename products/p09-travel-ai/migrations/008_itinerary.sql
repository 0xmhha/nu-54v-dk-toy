CREATE TABLE travel_private.itinerary (
    subject_ref uuid NOT NULL,
    id uuid NOT NULL,
    revision bigint NOT NULL CHECK(revision>0),
    consent_revision bigint NOT NULL CHECK(consent_revision>0),
    catalog_version bigint NOT NULL CHECK(catalog_version>0),
    latest_intent uuid NOT NULL,
    packet_hash bytea NOT NULL CHECK(octet_length(packet_hash)=32),
    sealed bytea NOT NULL CHECK(octet_length(sealed) BETWEEN 29 AND 16384),
    expires_at timestamptz NOT NULL CHECK(isfinite(expires_at)),
    PRIMARY KEY(subject_ref,id)
);

CREATE FUNCTION travel_private.apply_itinerary(actor_ uuid,plan_ uuid,prior_ bigint,consent_ bigint,catalog_ bigint,
    request_ uuid,digest_ bytea,sealed_ bytea,expires_ timestamptz)
RETURNS bigint LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $plan$
DECLARE existing travel_private.itinerary; expiration timestamptz;
BEGIN
    expiration:=travel_private.require_consent(actor_,'recommendation',consent_,'p09',plan_);
    IF expires_<=clock_timestamp() OR expires_>expiration OR travel_private.fenced(actor_,'itinerary',plan_,'recommendation',consent_) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='itinerary scope unavailable';
    END IF;
    SELECT * INTO existing FROM travel_private.itinerary WHERE subject_ref=actor_ AND id=plan_ FOR UPDATE;
    IF FOUND AND existing.latest_intent=request_ THEN
        IF (existing.revision-1,existing.packet_hash) IS DISTINCT FROM (prior_,digest_) THEN
            RAISE EXCEPTION USING ERRCODE='P1801', MESSAGE='itinerary intent conflict';
        END IF;
        RETURN existing.revision;
    END IF;
    IF coalesce(existing.revision,0)<>prior_ THEN
        RAISE EXCEPTION USING ERRCODE='P1801', MESSAGE='itinerary revision conflict';
    END IF;
    INSERT INTO travel_private.itinerary VALUES(actor_,plan_,prior_+1,consent_,catalog_,request_,digest_,sealed_,expires_)
      ON CONFLICT(subject_ref,id) DO UPDATE SET revision=excluded.revision,consent_revision=excluded.consent_revision,
        catalog_version=excluded.catalog_version,latest_intent=excluded.latest_intent,packet_hash=excluded.packet_hash,
        sealed=excluded.sealed,expires_at=excluded.expires_at;
    RETURN prior_+1;
END
$plan$;

CREATE FUNCTION travel_private.read_itinerary(actor_ uuid,plan_ uuid)
RETURNS TABLE(revision bigint,sealed bytea)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $read$
DECLARE saved travel_private.itinerary;
BEGIN
    SELECT * INTO saved FROM travel_private.itinerary WHERE subject_ref=actor_ AND id=plan_;
    IF NOT FOUND OR saved.expires_at<=clock_timestamp() OR travel_private.fenced(actor_,'itinerary',plan_,'recommendation',saved.consent_revision) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='itinerary scope unavailable';
    END IF;
    PERFORM travel_private.require_consent(actor_,'recommendation',saved.consent_revision,'p09',plan_);
    RETURN QUERY SELECT saved.revision,saved.sealed;
END
$read$;

CREATE FUNCTION travel_private.restore_itinerary_fence() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $restore$
BEGIN
    IF NEW.kind='purpose' AND NEW.purpose='recommendation' THEN
        DELETE FROM travel_private.itinerary WHERE subject_ref=NEW.subject_ref AND consent_revision<=NEW.through_revision;
    ELSIF NEW.kind='object' AND NEW.object_kind='itinerary' THEN
        DELETE FROM travel_private.itinerary WHERE subject_ref=NEW.subject_ref AND id=NEW.object_token;
    END IF;
    RETURN NEW;
END
$restore$;
CREATE TRIGGER restore_itinerary_fence AFTER INSERT ON travel_private.restore_fence
FOR EACH ROW EXECUTE FUNCTION travel_private.restore_itinerary_fence();

REVOKE ALL ON FUNCTION travel_private.apply_itinerary(uuid,uuid,bigint,bigint,bigint,uuid,bytea,bytea,timestamptz),
    travel_private.read_itinerary(uuid,uuid),travel_private.restore_itinerary_fence() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION travel_private.apply_itinerary(uuid,uuid,bigint,bigint,bigint,uuid,bytea,bytea,timestamptz),
    travel_private.read_itinerary(uuid,uuid) TO nu54_p09_private_user;

ALTER TABLE travel_private.erasure_request
    ADD COLUMN installation_proof_digest bytea
    CHECK (octet_length(installation_proof_digest) = 32);

CREATE FUNCTION travel_private.request_selected_erasure(actor_ uuid,key_ uuid,object_ uuid,proof_ bytea)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $request$
DECLARE existing travel_private.erasure_request;
        selected uuid;
BEGIN
    IF proof_ IS NULL OR octet_length(proof_) <> 32 THEN
        RAISE EXCEPTION USING ERRCODE='22023', MESSAGE='invalid erasure confirmation';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(actor_::text||':voice',900018));
    PERFORM pg_advisory_xact_lock(hashtextextended(actor_::text||':transcription',900018));
    SELECT * INTO existing FROM travel_private.erasure_request WHERE subject_ref=actor_ AND id=key_;
    IF FOUND AND existing.installation_proof_digest IS DISTINCT FROM proof_ THEN
        RAISE EXCEPTION USING ERRCODE='P1801', MESSAGE='erasure conflict';
    END IF;
    selected := travel_private.request_selected_erasure(actor_,key_,object_);
    IF selected=key_ AND existing.id IS NULL THEN
        UPDATE travel_private.erasure_request SET installation_proof_digest=proof_
            WHERE subject_ref=actor_ AND id=key_;
    END IF;
    RETURN selected;
END
$request$;

CREATE OR REPLACE FUNCTION travel_private.mark_device_erased(actor_ uuid,key_ uuid)
RETURNS boolean LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $legacy$
    WITH changed AS (
        UPDATE travel_private.erasure_request SET this_device_erased_at=coalesce(this_device_erased_at,clock_timestamp())
        WHERE subject_ref=actor_ AND id=key_ AND installation_proof_digest IS NULL RETURNING 1
    ) SELECT EXISTS(SELECT FROM changed)
$legacy$;

CREATE FUNCTION travel_private.mark_device_erased(actor_ uuid,key_ uuid,proof_ bytea)
RETURNS boolean LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $device$
    WITH changed AS (
        UPDATE travel_private.erasure_request SET this_device_erased_at=coalesce(this_device_erased_at,clock_timestamp())
        WHERE subject_ref=actor_ AND id=key_ AND installation_proof_digest=proof_ RETURNING 1
    ) SELECT EXISTS(SELECT FROM changed)
$device$;

CREATE FUNCTION travel_private.selected_erasure_status_v2(actor_ uuid,key_ uuid)
RETURNS TABLE(status text,local_device_ack boolean,provider_pending boolean,backup_expiry timestamptz,
              local_device_evidence text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $status$
    SELECT s.*,CASE WHEN NOT s.local_device_ack THEN 'none'
                    WHEN e.installation_proof_digest IS NULL THEN 'subject-reported'
                    ELSE 'installation-reported' END
    FROM travel_private.selected_erasure_status(actor_,key_) s
    JOIN travel_private.erasure_request e ON e.subject_ref=actor_ AND e.id=key_
$status$;

REVOKE ALL ON FUNCTION travel_private.request_selected_erasure(uuid,uuid,uuid,bytea),
    travel_private.mark_device_erased(uuid,uuid,bytea),travel_private.selected_erasure_status_v2(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION travel_private.request_selected_erasure(uuid,uuid,uuid,bytea),
    travel_private.mark_device_erased(uuid,uuid,bytea),travel_private.selected_erasure_status_v2(uuid,uuid) TO nu54_p09_private_user;

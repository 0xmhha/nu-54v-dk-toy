CREATE SCHEMA travel_erasure;
REVOKE ALL ON SCHEMA travel_erasure FROM PUBLIC;
CREATE TABLE travel_erasure.namespace (
    singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
    namespace uuid NOT NULL UNIQUE,
    high_water bigint NOT NULL DEFAULT 0 CHECK (high_water>=0)
);
CREATE TABLE travel_erasure.fence (
    sequence bigint PRIMARY KEY CHECK (sequence>0),
    subject_ref uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('purpose','object')),
    purpose text,
    through_revision bigint,
    object_kind text,
    object_token uuid,
    cutoff timestamptz NOT NULL CHECK (isfinite(cutoff)),
    expire_after timestamptz NOT NULL CHECK (isfinite(expire_after) AND expire_after>=cutoff+interval '90 days'),
    CHECK ((kind='purpose' AND purpose IN ('location','voice','transcription','recommendation','review','challenge')
              AND through_revision IS NOT NULL AND through_revision>0 AND object_kind IS NULL AND object_token IS NULL)
       OR (kind='object' AND purpose IS NULL AND through_revision IS NULL AND object_kind IN ('record','job','review','itinerary')
              AND object_token IS NOT NULL))
);
CREATE UNIQUE INDEX fence_object_once ON travel_erasure.fence(subject_ref,object_kind,object_token) WHERE kind='object';
CREATE UNIQUE INDEX fence_purpose_once ON travel_erasure.fence(subject_ref,purpose,through_revision) WHERE kind='purpose';

-- Only the destination namespace is writable by the restore role.
CREATE TABLE travel_private.restore_checkpoint (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    namespace uuid NOT NULL,
    applied_through bigint NOT NULL DEFAULT 0 CHECK (applied_through>=0)
);
CREATE TABLE travel_private.restore_fence (LIKE travel_erasure.fence INCLUDING CONSTRAINTS INCLUDING INDEXES);

DO $roles$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p09_erasure_export') THEN
    CREATE ROLE nu54_p09_erasure_export NOLOGIN NOSUPERUSER NOCREATEROLE NOCREATEDB NOBYPASSRLS;
  END IF;
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p09_erasure_append') THEN
    CREATE ROLE nu54_p09_erasure_append NOLOGIN NOSUPERUSER NOCREATEROLE NOCREATEDB NOBYPASSRLS;
  END IF;
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p09_restore_apply') THEN
    CREATE ROLE nu54_p09_restore_apply NOLOGIN NOSUPERUSER NOCREATEROLE NOCREATEDB NOBYPASSRLS;
  END IF;
END
$roles$;

CREATE FUNCTION travel_erasure.append_fence(subject_ uuid,kind_ text,purpose_ text,revision_ bigint,
    object_kind_ text,object_ uuid,cutoff_ timestamptz,expires_ timestamptz) RETURNS bigint
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $append$
DECLARE tail bigint;
BEGIN
  PERFORM pg_advisory_xact_lock(900021);
  SELECT high_water INTO STRICT tail FROM travel_erasure.namespace FOR UPDATE;
  SELECT sequence INTO tail FROM travel_erasure.fence WHERE subject_ref=subject_ AND kind=kind_
    AND (purpose,through_revision,object_kind,object_token) IS NOT DISTINCT FROM (purpose_,revision_,object_kind_,object_);
  IF FOUND THEN RETURN tail; END IF;
  UPDATE travel_erasure.namespace SET high_water=high_water+1 RETURNING high_water INTO tail;
  INSERT INTO travel_erasure.fence VALUES(tail,subject_,kind_,purpose_,revision_,object_kind_,object_,cutoff_,expires_);
  RETURN tail;
END
$append$;

REVOKE ALL ON FUNCTION travel_erasure.append_fence(uuid,text,text,bigint,text,uuid,timestamptz,timestamptz) FROM PUBLIC;
GRANT USAGE ON SCHEMA travel_erasure TO nu54_p09_erasure_export,nu54_p09_erasure_append;
GRANT SELECT ON travel_erasure.namespace,travel_erasure.fence TO nu54_p09_erasure_export;
GRANT EXECUTE ON FUNCTION travel_erasure.append_fence(uuid,text,text,bigint,text,uuid,timestamptz,timestamptz) TO nu54_p09_erasure_append;
GRANT USAGE ON SCHEMA travel_private TO nu54_p09_restore_apply;
GRANT SELECT,INSERT ON travel_private.restore_checkpoint,travel_private.restore_fence TO nu54_p09_restore_apply;
GRANT UPDATE(applied_through) ON travel_private.restore_checkpoint TO nu54_p09_restore_apply;

CREATE FUNCTION travel_private.restored() RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $state$
 SELECT EXISTS(SELECT FROM travel_private.restore_checkpoint)
$state$;
REVOKE ALL ON FUNCTION travel_private.restored() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION travel_private.restored() TO nu54_p09_private_user;


CREATE FUNCTION travel_erasure.guard_restore(namespace_ uuid,expected_ bigint) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $guard$
BEGIN
    PERFORM pg_advisory_xact_lock_shared(900021);
    RETURN EXISTS (SELECT FROM travel_erasure.namespace WHERE namespace=namespace_ AND high_water=expected_);
END
$guard$;
REVOKE ALL ON FUNCTION travel_erasure.guard_restore(uuid,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION travel_erasure.guard_restore(uuid,bigint) TO nu54_p09_erasure_export;

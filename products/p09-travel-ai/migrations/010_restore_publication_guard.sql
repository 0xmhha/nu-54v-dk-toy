-- Expose only the local cursor to ordinary private/public-review runtimes.
CREATE FUNCTION travel_private.restore_cursor()
RETURNS TABLE(namespace uuid, applied_through bigint)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $state$
    SELECT c.namespace,c.applied_through FROM travel_private.restore_checkpoint c
$state$;
REVOKE ALL ON FUNCTION travel_private.restore_cursor() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION travel_private.restore_cursor() TO nu54_p09_private_user,nu54_p09_review_read;
DO $roles$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p09_erasure_guard') THEN
    CREATE ROLE nu54_p09_erasure_guard NOLOGIN NOSUPERUSER NOCREATEROLE NOCREATEDB NOBYPASSRLS;
  ELSIF EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p09_erasure_guard'
      AND (rolcanlogin OR rolsuper OR rolcreaterole OR rolcreatedb OR rolbypassrls)) THEN
    RAISE EXCEPTION 'unsafe erasure guard capability';
  END IF;
END
$roles$;
GRANT USAGE ON SCHEMA travel_erasure TO nu54_p09_erasure_guard;
GRANT EXECUTE ON FUNCTION travel_erasure.guard_restore(uuid,bigint) TO nu54_p09_erasure_guard;

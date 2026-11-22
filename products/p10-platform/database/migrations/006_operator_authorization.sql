CREATE TABLE platform.recovery_operator_enrollment (
    login_role name NOT NULL,
    actor_ref uuid NOT NULL,
    scope_ref uuid NOT NULL REFERENCES platform.recovery_route,
    permission text NOT NULL CHECK (permission IN ('read','requester','applier')),
    expires_at timestamptz NOT NULL CHECK (isfinite(expires_at)),
    PRIMARY KEY(login_role,actor_ref,scope_ref,permission)
);

CREATE FUNCTION platform.recovery_operator_authorized(scope_ uuid,actor_ uuid,permission_ text,lock_ boolean DEFAULT false)
RETURNS boolean LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $auth$
DECLARE deadline timestamptz;
BEGIN
    IF lock_ THEN
        SELECT e.expires_at INTO deadline FROM platform.recovery_operator_enrollment e
        WHERE (e.login_role,e.actor_ref,e.scope_ref,e.permission)=(session_user,actor_,scope_,permission_) FOR SHARE;
    ELSE
        SELECT e.expires_at INTO deadline FROM platform.recovery_operator_enrollment e
        WHERE (e.login_role,e.actor_ref,e.scope_ref,e.permission)=(session_user,actor_,scope_,permission_);
    END IF;
    RETURN COALESCE(deadline>clock_timestamp(),false);
END
$auth$;
REVOKE ALL ON FUNCTION platform.recovery_operator_authorized(uuid,uuid,text,boolean) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.recovery_operator_authorized(uuid,uuid,text,boolean) TO
    nu54_p10_recovery_status,nu54_p10_recovery_coordinator;

CREATE FUNCTION platform.fleet_review_authorized(scope_ uuid) RETURNS boolean
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $review$
    SELECT EXISTS (SELECT FROM platform.fleet_review_enrollment e
      WHERE e.login_role=session_user AND e.cohort=scope_ AND e.expires_at>clock_timestamp())
$review$;
REVOKE ALL ON FUNCTION platform.fleet_review_authorized(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.fleet_review_authorized(uuid) TO nu54_p10_fleet_review;

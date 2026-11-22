DO $roles$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname='nu54_p07_rental_reader') THEN
        CREATE ROLE nu54_p07_rental_reader NOLOGIN;
    ELSIF EXISTS (
        SELECT FROM pg_catalog.pg_roles WHERE rolname='nu54_p07_rental_reader'
        AND (rolcanlogin OR rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication OR rolbypassrls)
    ) THEN
        RAISE EXCEPTION 'unsafe rental generation reader role';
    END IF;
END
$roles$;
GRANT USAGE ON SCHEMA chainview TO nu54_p07_rental_reader;
GRANT SELECT (id,chain_ref,contract_ref,abi_hash,manifest_ref) ON chainview.stream TO nu54_p07_rental_reader;
GRANT SELECT (id,stream_id,base_height,state,finalized_height,finalized_hash) ON chainview.generation TO nu54_p07_rental_reader;
GRANT SELECT (generation_id,block_hash,height,parent_hash,block_time,canonical) ON chainview.block TO nu54_p07_rental_reader;
GRANT SELECT (stream_id,version,asset_ref,code_hash,start_height,start_hash) ON chainview.deployment_profile TO nu54_p07_rental_reader;
GRANT SELECT (stream_id,generation_id) ON chainview.read_target TO nu54_p07_rental_reader;
GRANT SELECT (generation_id,resolved_at) ON chainview.incident TO nu54_p07_rental_reader;
GRANT SELECT (stream_id,generation_id) ON chainview.profile_hold TO nu54_p07_rental_reader;
GRANT EXECUTE ON FUNCTION chainview.complete_prefix(uuid,bigint),
    chainview.lock_absence_anchor(uuid,uuid,bigint,text) TO nu54_p07_rental_reader;

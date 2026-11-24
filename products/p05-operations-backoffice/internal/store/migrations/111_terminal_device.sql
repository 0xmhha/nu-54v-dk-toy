-- Device settings of one kiosk. Only a branch operator with terminal_settings changes them, and every
-- change keeps who made it, when, and both values.

CREATE TABLE ledger.terminal_device (
    branch_id uuid NOT NULL,
    terminal_ref uuid NOT NULL,
    revision bigint NOT NULL CHECK (revision>0),
    language text NOT NULL DEFAULT 'ko' CHECK (language IN ('ko','en')),
    idle_seconds integer NOT NULL DEFAULT 60 CHECK (idle_seconds IN (30,60,90,120,180)),
    read_server text CHECK (octet_length(read_server)<=512 AND read_server ~ '^https://[][A-Za-z0-9.:_-]+(/[A-Za-z0-9/_.-]*)?$'),
    PRIMARY KEY(branch_id,terminal_ref),
    FOREIGN KEY(branch_id,terminal_ref) REFERENCES ledger.commerce_grant(branch_id,actor_id) ON DELETE CASCADE
);
CREATE TABLE ledger.terminal_device_change (
    branch_id uuid NOT NULL REFERENCES ledger.branch,
    terminal_ref uuid NOT NULL,
    revision bigint NOT NULL CHECK (revision>0),
    name text NOT NULL CHECK (name IN ('read-server','language','idle-seconds')),
    old_value text NOT NULL CHECK (octet_length(old_value)<=512),
    new_value text NOT NULL CHECK (octet_length(new_value)<=512),
    actor_ref uuid NOT NULL REFERENCES ledger.login_subject(subject_ref),
    request_key uuid NOT NULL,
    intent_digest bytea NOT NULL CHECK (octet_length(intent_digest)=32),
    changed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(branch_id,terminal_ref,revision),
    UNIQUE(branch_id,actor_ref,request_key)
);
REVOKE ALL ON ledger.terminal_device,ledger.terminal_device_change FROM PUBLIC;

CREATE FUNCTION ledger.terminal_device_state(tenant_ uuid,branch_ uuid,actor_ uuid,terminal_ uuid,role_ text) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $state$
DECLARE kept ledger.terminal_device;
BEGIN
    PERFORM ledger.terminal_mode_state(tenant_,branch_,actor_,terminal_,role_);
    SELECT * INTO kept FROM ledger.terminal_device d WHERE (d.branch_id,d.terminal_ref)=(branch_,terminal_);
    RETURN jsonb_strip_nulls(jsonb_build_object('revision',coalesce(kept.revision,0),'language',coalesce(kept.language,'ko'),
        'idleSeconds',coalesce(kept.idle_seconds,60),'readServer',kept.read_server,
        'changes',CASE WHEN role_='operator' THEN (SELECT coalesce(jsonb_agg(jsonb_build_object('name',c.name,'from',c.old_value,'to',c.new_value,
            'actor',c.actor_ref,'at',floor(extract(epoch FROM c.changed_at))::bigint) ORDER BY c.revision DESC),'[]'::jsonb)
            FROM (SELECT * FROM ledger.terminal_device_change x WHERE (x.branch_id,x.terminal_ref)=(branch_,terminal_)
                ORDER BY x.revision DESC LIMIT 10) c) END,
        'registrationExpiresAt',CASE WHEN role_='operator' THEN (SELECT floor(extract(epoch FROM g.valid_until))::bigint
            FROM ledger.commerce_grant g WHERE (g.branch_id,g.actor_id)=(branch_,terminal_)) END));
END
$state$;

CREATE FUNCTION ledger.change_terminal_device(tenant_ uuid,branch_ uuid,actor_ uuid,terminal_ uuid,key_ uuid,expected_ bigint,
    name_ text,value_ text,intent_ bytea) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $change$
DECLARE kept ledger.terminal_device; prior ledger.terminal_device_change; old_ text;
BEGIN
    PERFORM ledger.terminal_mode_state(tenant_,branch_,actor_,terminal_,'operator');
    IF key_ IS NULL OR octet_length(intent_)<>32 OR expected_ IS NULL OR expected_<0 OR name_ NOT IN ('read-server','language','idle-seconds') THEN
        RAISE EXCEPTION 'invalid terminal setting' USING ERRCODE='23514';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(branch_::text||terminal_::text,111));
    SELECT * INTO prior FROM ledger.terminal_device_change c WHERE (c.branch_id,c.actor_ref,c.request_key)=(branch_,actor_,key_);
    IF FOUND THEN
        IF (prior.terminal_ref,prior.intent_digest) IS DISTINCT FROM (terminal_,intent_) THEN
            RAISE EXCEPTION 'request conflict' USING ERRCODE='P0501';
        END IF;
        RETURN ledger.terminal_device_state(tenant_,branch_,actor_,terminal_,'operator');
    END IF;
    SELECT * INTO kept FROM ledger.terminal_device d WHERE (d.branch_id,d.terminal_ref)=(branch_,terminal_) FOR UPDATE;
    IF coalesce(kept.revision,0)<>expected_ THEN RAISE EXCEPTION 'terminal setting changed' USING ERRCODE='P0501'; END IF;
    old_:=CASE name_ WHEN 'language' THEN coalesce(kept.language,'ko') WHEN 'idle-seconds' THEN coalesce(kept.idle_seconds,60)::text
        ELSE coalesce(kept.read_server,'') END;
    INSERT INTO ledger.terminal_device(branch_id,terminal_ref,revision,language,idle_seconds,read_server)
        VALUES(branch_,terminal_,expected_+1,coalesce(kept.language,'ko'),coalesce(kept.idle_seconds,60),kept.read_server)
        ON CONFLICT(branch_id,terminal_ref) DO UPDATE SET revision=excluded.revision;
    UPDATE ledger.terminal_device d SET
        language=CASE WHEN name_='language' THEN value_ ELSE d.language END,
        idle_seconds=CASE WHEN name_='idle-seconds' THEN value_::integer ELSE d.idle_seconds END,
        read_server=CASE WHEN name_='read-server' THEN nullif(value_,'') ELSE d.read_server END
        WHERE (d.branch_id,d.terminal_ref)=(branch_,terminal_);
    INSERT INTO ledger.terminal_device_change(branch_id,terminal_ref,revision,name,old_value,new_value,actor_ref,request_key,intent_digest)
        VALUES(branch_,terminal_,expected_+1,name_,old_,value_,actor_,key_,intent_);
    RETURN ledger.terminal_device_state(tenant_,branch_,actor_,terminal_,'operator');
END
$change$;

REVOKE ALL ON FUNCTION ledger.terminal_device_state(uuid,uuid,uuid,uuid,text),
    ledger.change_terminal_device(uuid,uuid,uuid,uuid,uuid,bigint,text,text,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ledger.terminal_device_state(uuid,uuid,uuid,uuid,text),
    ledger.change_terminal_device(uuid,uuid,uuid,uuid,uuid,bigint,text,text,bytea) TO nu54_p05_commerce;

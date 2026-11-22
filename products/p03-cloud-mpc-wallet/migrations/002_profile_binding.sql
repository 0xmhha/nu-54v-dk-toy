ALTER TABLE wallet_mpc.operation ADD COLUMN request_ref uuid;
UPDATE wallet_mpc.operation SET request_ref=operation_id;
ALTER TABLE wallet_mpc.operation ALTER COLUMN request_ref SET NOT NULL;
ALTER TABLE wallet_mpc.operation ADD UNIQUE (wallet_id,request_ref);

CREATE TABLE wallet_mpc.profile_binding (
    subject_ref uuid PRIMARY KEY REFERENCES wallet_mpc.identity,
    issuer text NOT NULL,
    profile jsonb NOT NULL CHECK (jsonb_typeof(profile)='object'),
    profile_digest bytea NOT NULL UNIQUE CHECK (octet_length(profile_digest)=32)
);
CREATE TABLE wallet_mpc.participant_binding (
    subject_ref uuid NOT NULL REFERENCES wallet_mpc.profile_binding,
    participant_ref text PRIMARY KEY,
    certificate_pin text NOT NULL UNIQUE CHECK (certificate_pin ~ '^[0-9a-f]{64}$'),
    public_key_pin text NOT NULL UNIQUE CHECK (public_key_pin ~ '^[0-9a-f]{64}$'),
    endpoint_origin text NOT NULL UNIQUE
);
GRANT SELECT, INSERT ON wallet_mpc.profile_binding, wallet_mpc.participant_binding TO nu54_p03_service;

CREATE TABLE wallet_mpc.authority_binding (
    subject_ref uuid NOT NULL REFERENCES wallet_mpc.profile_binding,
    authority_pin bytea PRIMARY KEY CHECK (octet_length(authority_pin)=32)
);
GRANT SELECT, INSERT ON wallet_mpc.authority_binding TO nu54_p03_service;

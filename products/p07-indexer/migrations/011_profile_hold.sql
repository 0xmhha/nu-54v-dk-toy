CREATE TABLE chainview.profile_hold (
    stream_id uuid NOT NULL,
    generation_id uuid NOT NULL,
    chain_ref text NOT NULL CHECK (chain_ref ~ '^[0-9]{1,20}$'),
    contract_ref text NOT NULL CHECK (contract_ref ~ '^0x[0-9a-f]{40}$'),
    version smallint NOT NULL CHECK (version BETWEEN 0 AND 255),
    asset_ref text NOT NULL CHECK (asset_ref ~ '^0x[0-9a-f]{40}$'),
    abi_hash text CHECK (abi_hash ~ '^[0-9a-f]{64}$'),
    base_height bigint CHECK (base_height>=0),
    reason text NOT NULL CHECK (reason IN ('unknown_abi','unapproved_profile','identity_mismatch')),
    observed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(stream_id,generation_id)
);
REVOKE ALL ON chainview.profile_hold FROM PUBLIC;
GRANT SELECT,INSERT,UPDATE,DELETE ON chainview.profile_hold TO nu54_p07_ingest;
GRANT SELECT ON chainview.profile_hold TO nu54_p07_explorer;

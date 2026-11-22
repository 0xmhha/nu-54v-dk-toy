CREATE TABLE wallet_mpc.closure_receipt (
    wallet_id uuid PRIMARY KEY REFERENCES wallet_mpc.wallet,
    request_ref uuid NOT NULL,
    epoch bigint NOT NULL CHECK (epoch > 0),
    accepted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (wallet_id, epoch) REFERENCES wallet_mpc.key_epoch(wallet_id, epoch)
);
REVOKE ALL ON wallet_mpc.closure_receipt FROM PUBLIC;
GRANT SELECT,INSERT ON wallet_mpc.closure_receipt TO nu54_p03_service;

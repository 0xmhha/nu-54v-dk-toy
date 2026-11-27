ALTER TABLE wallet_mpc.operation DROP CONSTRAINT operation_state_check;
ALTER TABLE wallet_mpc.operation ADD CONSTRAINT operation_state_check CHECK (
  state IN ('reserved','running','succeeded','failed','held','expired')
  OR (state='denied' AND kind='refresh')
);
ALTER TABLE wallet_mpc.audit_outbox DROP CONSTRAINT audit_outbox_event_kind_check;
ALTER TABLE wallet_mpc.audit_outbox ADD CONSTRAINT audit_outbox_event_kind_check CHECK (
  event_kind IN ('created','sign_finished','recovery_started','epoch_activated','recovery_held',
    'closed','incident_hold','recovery_denied','recovery_review_permitted')
);
CREATE TABLE wallet_mpc.recovery_refusal (
  operation_id uuid PRIMARY KEY REFERENCES wallet_mpc.operation,
  vote_id uuid NOT NULL UNIQUE,
  decision jsonb NOT NULL CHECK (jsonb_typeof(decision)='object' AND pg_column_size(decision)<=2048),
  signature bytea NOT NULL CHECK (octet_length(signature)=64),
  authority_key bytea NOT NULL CHECK (octet_length(authority_key)=32),
  recorded_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE wallet_mpc.recovery_review (
  operation_id uuid PRIMARY KEY REFERENCES wallet_mpc.recovery_refusal,
  operator_ref uuid NOT NULL,
  permitted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  successor_operation uuid UNIQUE REFERENCES wallet_mpc.operation,
  CHECK (successor_operation IS DISTINCT FROM operation_id)
);
REVOKE ALL ON wallet_mpc.recovery_refusal,wallet_mpc.recovery_review FROM PUBLIC;
GRANT SELECT,INSERT ON wallet_mpc.recovery_refusal,wallet_mpc.recovery_review TO nu54_p03_service;
GRANT UPDATE(successor_operation) ON wallet_mpc.recovery_review TO nu54_p03_service;

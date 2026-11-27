ALTER TABLE wallet_mpc.operation DROP CONSTRAINT operation_state_check;
ALTER TABLE wallet_mpc.operation ADD CONSTRAINT operation_state_check CHECK (
  state IN ('reserved','running','succeeded','failed','held','expired')
  OR (state='denied' AND kind='refresh')
  OR (state='refused' AND kind='sign' AND approval_proof IS NULL)
);
ALTER TABLE wallet_mpc.audit_outbox DROP CONSTRAINT audit_outbox_event_kind_check;
ALTER TABLE wallet_mpc.audit_outbox ADD CONSTRAINT audit_outbox_event_kind_check CHECK (
  event_kind IN ('created','sign_finished','recovery_started','epoch_activated','recovery_held',
    'closed','incident_hold','recovery_denied','recovery_review_permitted','sign_refused')
);

CREATE UNIQUE INDEX operation_sign_refusal_once ON wallet_mpc.audit_outbox(operation_id)
WHERE event_kind='sign_refused';

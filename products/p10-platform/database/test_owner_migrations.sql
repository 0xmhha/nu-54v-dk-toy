-- Assertions run in a rollback-only transaction using the production roles.
BEGIN;

SET LOCAL ROLE nu54_p07_legacy;
INSERT INTO public.receipts VALUES ('other-tx', 0, 'merchant', 'order', 'device', 30, 2, 43, 1800000001);
INSERT INTO public.cursor VALUES (1, 43) ON CONFLICT (id) DO UPDATE SET block_number = EXCLUDED.block_number;
DO $test$
BEGIN
    IF (SELECT block_number FROM public.cursor WHERE id = 1) <> 43 THEN
        RAISE EXCEPTION 'legacy cursor failed to advance';
    END IF;
    IF has_table_privilege(current_user, 'public.receipts', 'DELETE')
        OR has_schema_privilege(current_user, 'chainview', 'USAGE') THEN
        RAISE EXCEPTION 'legacy writer granted excess rights';
    END IF;
END
$test$;
RESET ROLE;
SET LOCAL ROLE nu54_p07_ingest;
INSERT INTO chainview.stream VALUES ('00000000-0000-0000-0000-000000000001', 'test-chain', 'test-contract', 'abi', 'manifest');
INSERT INTO chainview.generation (id, stream_id, base_height, fence, state) VALUES
    ('00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000001', 0, 1, 'building'),
    ('00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000001', 0, 1, 'building');
INSERT INTO chainview.block VALUES
    ('00000000-0000-0000-0000-000000000002', 'active', 1, 'parent', '2026-11-20Z', true),
    ('00000000-0000-0000-0000-000000000002', 'fork', 1, 'parent', '2026-11-20Z', false),
    ('00000000-0000-0000-0000-000000000003', 'active', 1, 'parent', '2026-11-20Z', true);
UPDATE chainview.generation SET tip_height = 1, tip_hash = 'active', finalized_height = 1, finalized_hash = 'active';
UPDATE chainview.generation SET state = 'ready' WHERE id = '00000000-0000-0000-0000-000000000002';
INSERT INTO chainview.observed_event VALUES
    ('00000000-0000-0000-0000-000000000002', 'active', 'tx', 0, 'paid', true, 'recipient', 'nonce', 'merchant', 'order', NULL, 'asset', 30),
    ('00000000-0000-0000-0000-000000000002', 'fork', 'tx', 0, 'paid', true, 'recipient', 'nonce', 'merchant', 'order', NULL, 'asset', 30);
INSERT INTO chainview.evidence
    (id, generation_id, merchant_key, order_key, kind, anchor_height, anchor_hash, policy_ref, log_block, tx_hash, log_index, created_at)
VALUES
    (gen_random_uuid(), '00000000-0000-0000-0000-000000000002', 'merchant', 'order', 'paid', 1, 'active', 'policy', 'active', 'tx', 0, now()),
    (gen_random_uuid(), '00000000-0000-0000-0000-000000000002', 'other-merchant', 'order', 'paid', 1, 'active', 'policy', 'active', 'tx', 0, now()),
    (gen_random_uuid(), '00000000-0000-0000-0000-000000000002', 'merchant', 'order', 'paid', 1, 'active', 'policy', 'fork', 'tx', 0, now());
INSERT INTO chainview.evidence
    (id, generation_id, merchant_key, order_key, kind, anchor_height, anchor_hash, policy_ref, expires_at, coverage_start, state_proof_ref, created_at)
VALUES
    (gen_random_uuid(), '00000000-0000-0000-0000-000000000002', 'merchant', 'order', 'absent', 1, 'active', 'policy', now() - interval '1 day', 0, 'proof', now()),
    (gen_random_uuid(), '00000000-0000-0000-0000-000000000003', 'merchant', 'order', 'absent', 1, 'active', 'policy', now() + interval '1 day', 0, 'proof', now());
RESET ROLE;
SET LOCAL ROLE nu54_p07_switch;
INSERT INTO chainview.switch_request
    (id, stream_id, target_id, expected_revision, operator_ref, compare_ref, requested_at)
VALUES
    ('00000000-0000-0000-0000-000000000030', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000002', 0, gen_random_uuid(), gen_random_uuid(), now()),
    ('00000000-0000-0000-0000-000000000031', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000002', 0, gen_random_uuid(), gen_random_uuid(), now()),
    ('00000000-0000-0000-0000-000000000032', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000003', 1, gen_random_uuid(), gen_random_uuid(), now());
DO $test$
DECLARE applied timestamptz;
BEGIN
    applied := chainview.apply_switch('00000000-0000-0000-0000-000000000030');
    IF applied IS DISTINCT FROM chainview.apply_switch('00000000-0000-0000-0000-000000000030') THEN
        RAISE EXCEPTION 'switch retry changed the receipt';
    END IF;
    BEGIN
        PERFORM chainview.apply_switch('00000000-0000-0000-0000-000000000031');
        RAISE EXCEPTION 'stale revision accepted';
    EXCEPTION WHEN object_not_in_prerequisite_state THEN NULL; END;
    BEGIN
        PERFORM chainview.apply_switch('00000000-0000-0000-0000-000000000032');
        RAISE EXCEPTION 'building generation accepted';
    EXCEPTION WHEN object_not_in_prerequisite_state THEN NULL; END;
    BEGIN
        UPDATE chainview.read_target SET revision = -1;
        RAISE EXCEPTION 'switch role bypassed compare and swap';
    EXCEPTION WHEN insufficient_privilege THEN NULL; END;
    BEGIN
        INSERT INTO chainview.switch_request
            (id, stream_id, target_id, expected_revision, operator_ref, compare_ref, requested_at)
        VALUES (gen_random_uuid(), '00000000-0000-0000-0000-000000000001',
            '00000000-0000-0000-0000-000000000002', -1, gen_random_uuid(), gen_random_uuid(), now());
        RAISE EXCEPTION 'negative expected revision accepted';
    EXCEPTION WHEN check_violation THEN NULL; END;
    IF (SELECT revision FROM chainview.read_target) <> 1 THEN
        RAISE EXCEPTION 'failed or repeated switch advanced the revision';
    END IF;
END
$test$;
RESET ROLE;
SET LOCAL ROLE nu54_p07_read;
DO $test$
BEGIN
    IF (SELECT count(*) FROM chainview.active_evidence) <> 1
        OR (SELECT kind FROM chainview.active_evidence) <> 'paid' THEN
        RAISE EXCEPTION 'inactive, mismatched, expired, or fork evidence exposed';
    END IF;
    BEGIN
        PERFORM * FROM chainview.observed_event;
        RAISE EXCEPTION 'raw chain events exposed';
    EXCEPTION WHEN insufficient_privilege THEN NULL; END;
    BEGIN
        PERFORM chainview.apply_switch('00000000-0000-0000-0000-000000000030');
        RAISE EXCEPTION 'read role can invoke the switch';
    EXCEPTION WHEN insufficient_privilege THEN NULL; END;
    BEGIN
        UPDATE chainview.read_target SET revision = 2;
        RAISE EXCEPTION 'read role can switch generations';
    EXCEPTION WHEN insufficient_privilege THEN NULL; END;
END
$test$;
RESET ROLE;

SET LOCAL ROLE nu54_p10_restore_request;
INSERT INTO platform.restore_job VALUES
    ('00000000-0000-0000-0000-000000000010', gen_random_uuid(), gen_random_uuid(), 1,
        '00000000-0000-0000-0000-000000000011', now(), 60, 5);
DO $test$
BEGIN
    BEGIN
        INSERT INTO platform.restore_approval VALUES
            ('00000000-0000-0000-0000-000000000010', '00000000-0000-0000-0000-000000000011',
                gen_random_uuid(), decode(repeat('00', 32), 'hex'), now(), now() + interval '1 hour', 30, 0);
        RAISE EXCEPTION 'requester can approve a restore';
    EXCEPTION WHEN insufficient_privilege THEN NULL; END;
END
$test$;
RESET ROLE;
SET LOCAL ROLE nu54_p10_restore_verify;
DO $test$
BEGIN
    BEGIN
        INSERT INTO platform.restore_approval VALUES
            ('00000000-0000-0000-0000-000000000010', '00000000-0000-0000-0000-000000000011',
                '00000000-0000-0000-0000-000000000011', decode(repeat('00', 32), 'hex'), now(), now() + interval '1 hour', 30, 0);
        RAISE EXCEPTION 'self-verification accepted';
    EXCEPTION WHEN check_violation THEN NULL; END;
END
$test$;
INSERT INTO platform.restore_approval VALUES
    ('00000000-0000-0000-0000-000000000010', '00000000-0000-0000-0000-000000000011',
        gen_random_uuid(), decode(repeat('00', 32), 'hex'), now(), now() + interval '1 hour', 30, 0);
RESET ROLE;
SET LOCAL ROLE nu54_p10_restore_apply;
INSERT INTO platform.cutover_receipt VALUES
    ('00000000-0000-0000-0000-000000000010', decode(repeat('00', 32), 'hex'), 2, now());
RESET ROLE;

SET LOCAL ROLE nu54_p10_retention_admin;
DO $test$
BEGIN
    BEGIN
        INSERT INTO privacy_ops.retention_rule VALUES
            (gen_random_uuid(), 'test', 1, '', 'test-basis', 'audit', 'access', 'test-period', 'expiry', now());
        RAISE EXCEPTION 'missing purpose accepted';
    EXCEPTION WHEN check_violation THEN NULL; END;
END
$test$;
INSERT INTO privacy_ops.retention_rule VALUES
    ('00000000-0000-0000-0000-000000000021', 'test', 1, 'audit', 'test-basis', 'audit', 'access', 'test-period', 'expiry', now()),
    ('00000000-0000-0000-0000-000000000022', 'test-rights', 1, 'rights', 'test-basis', 'rights', 'case', 'test-period', 'expiry', now());
RESET ROLE;
SET LOCAL ROLE nu54_p10_audit_writer;
DO $test$
BEGIN
    BEGIN
        INSERT INTO privacy_ops.access_record VALUES
            (gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), now(), decode('ff','hex'),
                'download', 'allowed', NULL, false, true, '00000000-0000-0000-0000-000000000021', 'infinity');
        RAISE EXCEPTION 'unbounded retention accepted';
    EXCEPTION WHEN check_violation THEN NULL; END;
    BEGIN
        PERFORM * FROM privacy_ops.access_record;
        RAISE EXCEPTION 'access writer can read audit records';
    EXCEPTION WHEN insufficient_privilege THEN NULL; END;
    BEGIN
        DELETE FROM privacy_ops.access_record;
        RAISE EXCEPTION 'access writer can erase audit records';
    EXCEPTION WHEN insufficient_privilege THEN NULL; END;
END
$test$;
INSERT INTO privacy_ops.access_record VALUES
    (gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), now(), decode('ff','hex'), 'download',
        'allowed', gen_random_uuid(), true, true, '00000000-0000-0000-0000-000000000021', now() + interval '1 year');
RESET ROLE;
SET LOCAL ROLE nu54_p10_audit_reader;
DO $test$
BEGIN
    IF (SELECT count(*) FROM privacy_ops.access_record WHERE downloaded) <> 1 THEN
        RAISE EXCEPTION 'audit read failed';
    END IF;
    BEGIN
        UPDATE privacy_ops.access_record SET downloaded = false;
        RAISE EXCEPTION 'auditor can write audit records';
    EXCEPTION WHEN insufficient_privilege THEN NULL; END;
END
$test$;
RESET ROLE;
SET LOCAL ROLE nu54_p10_rights;
INSERT INTO privacy_ops.rights_case
    (case_id, service_scope, local_claim_ref, request_type, authorized_scope_digest, received_at, due_at, state, rule_id, keep_until)
VALUES
    ('00000000-0000-0000-0000-000000000023', gen_random_uuid(), gen_random_uuid(), 'erase', decode(repeat('aa',32), 'hex'),
        now(), now() + interval '1 day', 'working', '00000000-0000-0000-0000-000000000022', now() + interval '1 year');
DO $test$
BEGIN
    BEGIN
        UPDATE privacy_ops.rights_case SET state = 'complete';
        RAISE EXCEPTION 'erase case completed without receipt and notice';
    EXCEPTION WHEN check_violation THEN NULL; END;
    BEGIN
        PERFORM * FROM privacy_ops.access_record;
        RAISE EXCEPTION 'rights staff can read access logs';
    EXCEPTION WHEN insufficient_privilege THEN NULL; END;
END
$test$;
UPDATE privacy_ops.rights_case SET state = 'complete', result_receipt = gen_random_uuid(), notice_at = now();
ROLLBACK;

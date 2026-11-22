-- Integration assertions as distinct, non-owning database logins.
BEGIN;
SET LOCAL plpgsql.check_asserts = on;
CREATE ROLE nu54_test_branch_a LOGIN;
CREATE ROLE nu54_test_branch_b LOGIN;
CREATE ROLE nu54_test_operator LOGIN;
GRANT nu54_p05_staff TO nu54_test_branch_a, nu54_test_branch_b, nu54_test_operator;
INSERT INTO ledger.branch VALUES
  ('20000000-0000-4000-8000-000000000001','10000000-0000-4000-8000-000000000001','Asia/Seoul','04:00'),
  ('20000000-0000-4000-8000-000000000002','10000000-0000-4000-8000-000000000001','Asia/Seoul','04:00');
INSERT INTO ledger.command_rule(branch_id,purpose_ref,basis_ref,disposal_ref,receipt_window)
SELECT id,'synthetic-order','synthetic-contract','expire-fixture',interval '1 day' FROM ledger.branch;
INSERT INTO ledger.login_subject VALUES
  ('nu54_test_branch_a','30000000-0000-4000-8000-000000000001'),
  ('nu54_test_branch_b','30000000-0000-4000-8000-000000000002'),
  ('nu54_test_operator','30000000-0000-4000-8000-000000000003');
INSERT INTO ledger.staff_grant(branch_id,subject_ref,role_code,valid_from)
SELECT id, ('30000000-0000-4000-8000-00000000000' || right(id::text,1))::uuid, 'finance', now() - interval '1 day'
FROM ledger.branch;
INSERT INTO ledger.staff_grant VALUES
  ('20000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000003','operator',now()-interval '1 day',NULL);

SET LOCAL SESSION AUTHORIZATION nu54_test_branch_a;
DO $$
DECLARE
    a uuid := '20000000-0000-4000-8000-000000000001';
    b uuid := '20000000-0000-4000-8000-000000000002';
    key_ uuid := gen_random_uuid();
    origin_ uuid := gen_random_uuid();
    sale_ uuid;
BEGIN
    sale_ := ledger.create_sale(a,key_,'pos',origin_,'test-asset',23);
    ASSERT ledger.create_sale(a,key_,'pos',origin_,'test-asset',23.00)=sale_, 'same command result';
    ASSERT (SELECT count(*) FROM ledger.export_sales(a))=1, 'scoped export';
    ASSERT (SELECT amount FROM ledger.export_sales(a) WHERE id=sale_)=23, 'atomic units';
    PERFORM set_config('nu54.subject_ref','30000000-0000-4000-8000-000000000002',true);
    BEGIN
        PERFORM ledger.export_sales(b);
        ASSERT false,'foreign branch CSV';
    EXCEPTION WHEN insufficient_privilege THEN NULL; END;
    BEGIN
        PERFORM ledger.create_sale(b,key_,'pos',origin_,'test-asset',23);
        ASSERT false,'foreign branch write';
    EXCEPTION WHEN insufficient_privilege THEN NULL; END;
    BEGIN
        PERFORM ledger.create_sale(a,key_,'pos',origin_,'test-asset',24);
        ASSERT false,'changed intent';
    EXCEPTION WHEN SQLSTATE 'P0501' THEN NULL; END;
    BEGIN
        PERFORM ledger.create_sale(a,gen_random_uuid(),'pos',gen_random_uuid(),'test-asset',2.3);
        ASSERT false,'fractional units admitted';
    EXCEPTION WHEN check_violation THEN NULL; END;
    BEGIN
        PERFORM intent_digest FROM ledger.command_receipt;
        ASSERT false,'direct table disclosure';
    EXCEPTION WHEN insufficient_privilege THEN NULL; END;
END
$$;

SET LOCAL SESSION AUTHORIZATION nu54_test_operator;
DO $$
BEGIN
    PERFORM ledger.create_sale('20000000-0000-4000-8000-000000000001',gen_random_uuid(),'menu',gen_random_uuid(),'test-asset',5);
    BEGIN
        PERFORM ledger.export_sales('20000000-0000-4000-8000-000000000001');
        ASSERT false,'operator exported financial data';
    EXCEPTION WHEN insufficient_privilege THEN NULL; END;
END
$$;
RESET SESSION AUTHORIZATION;
INSERT INTO ledger.sale(id,branch_id,origin,origin_ref,asset_ref,amount)
VALUES ('40000000-0000-4000-8000-000000000001','20000000-0000-4000-8000-000000000001','pos',
        '40000000-0000-4000-8000-000000000002','test-asset',23);
INSERT INTO ledger.command_receipt
VALUES ('20000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000001','create_sale',
        '40000000-0000-4000-8000-000000000003',
        sha256(convert_to(jsonb_build_array('pos','40000000-0000-4000-8000-000000000002','test-asset',23)::text,'UTF8')),
        '40000000-0000-4000-8000-000000000001', now()-interval '2 days',now()-interval '1 day');
SET LOCAL SESSION AUTHORIZATION nu54_test_branch_a;
DO $$
BEGIN
    BEGIN
        PERFORM ledger.create_sale('20000000-0000-4000-8000-000000000001','40000000-0000-4000-8000-000000000003',
                    'pos','40000000-0000-4000-8000-000000000002','test-asset',23);
        ASSERT false, 'expired command replay';
    EXCEPTION WHEN SQLSTATE 'P0502' THEN NULL; END;
END
$$;
RESET SESSION AUTHORIZATION;
DELETE FROM ledger.command_rule WHERE branch_id='20000000-0000-4000-8000-000000000001';
UPDATE ledger.staff_grant SET revoked_at=now() WHERE subject_ref='30000000-0000-4000-8000-000000000003';
SET LOCAL SESSION AUTHORIZATION nu54_test_branch_a;
DO $$
BEGIN
    BEGIN
        PERFORM ledger.create_sale('20000000-0000-4000-8000-000000000001',gen_random_uuid(),'pos',gen_random_uuid(),'test-asset',23);
        ASSERT false, 'collected fields without approved rule';
    EXCEPTION WHEN SQLSTATE 'P0503' THEN NULL; END;
END
$$;
SET LOCAL SESSION AUTHORIZATION nu54_test_operator;
DO $$
BEGIN
    BEGIN
        PERFORM ledger.create_sale('20000000-0000-4000-8000-000000000001',gen_random_uuid(),'pos',gen_random_uuid(),'test-asset',23);
        ASSERT false, 'revoked login accepted';
    EXCEPTION WHEN insufficient_privilege THEN NULL; END;
END
$$;
RESET SESSION AUTHORIZATION;
ROLLBACK;

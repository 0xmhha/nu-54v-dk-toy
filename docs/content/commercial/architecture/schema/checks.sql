-- Run on a disposable database containing all reference DDLs.
-- This transaction uses only synthetic data and ends in ROLLBACK.
BEGIN;
SET LOCAL plpgsql.check_asserts = on;
DO $checks$
DECLARE
    branch_a uuid := gen_random_uuid(); tenant uuid := gen_random_uuid();
    sale_a uuid := gen_random_uuid(); actor_a uuid := gen_random_uuid();
    route_a uuid := gen_random_uuid(); key_a uuid := gen_random_uuid();
    refund_a uuid := gen_random_uuid(); wallet_a uuid := gen_random_uuid();
    digest bytea := decode(repeat('1f',32),'hex'); job uuid := gen_random_uuid();
    rule uuid := gen_random_uuid(); person text := 'synthetic-local-scope';
    moment timestamptz := now(); venue uuid := gen_random_uuid();
BEGIN
    INSERT INTO ledger.branch VALUES (branch_a,tenant,'Asia/Seoul','04:00');
    INSERT INTO ledger.command_receipt VALUES(branch_a,tenant,'order',key_a,digest,sale_a,moment,moment+interval '1 hour');
    BEGIN
        INSERT INTO ledger.command_receipt VALUES(branch_a,tenant,'order',key_a,digest,sale_a,moment,moment+interval '1 hour');
        ASSERT false,'duplicate scoped command';
    EXCEPTION WHEN unique_violation THEN NULL; END;
    INSERT INTO ledger.sale VALUES (sale_a,branch_a,NULL,'pos',gen_random_uuid(),1,'paid');
    INSERT INTO ledger.quote VALUES (sale_a,digest,'synthetic-domain','shop','order','nonce','test-token',30,'synthetic-route',digest,moment,moment+interval '1 hour');
    INSERT INTO ledger.payment VALUES (sale_a,gen_random_uuid(),30,0);
    ASSERT ledger.reserve_refund(refund_a,sale_a,key_a,20,route_a,actor_a,'partial_service')=refund_a;
    ASSERT ledger.reserve_refund(gen_random_uuid(),sale_a,key_a,20,route_a,actor_a,'partial_service')=refund_a;
    BEGIN
        PERFORM ledger.reserve_refund(gen_random_uuid(),sale_a,key_a,21,route_a,actor_a,'partial_service');
        RAISE EXCEPTION USING ERRCODE='XX000', MESSAGE='mismatched refund accepted';
    EXCEPTION WHEN raise_exception THEN ASSERT SQLERRM='request key reused'; END;
    BEGIN
        PERFORM ledger.reserve_refund(gen_random_uuid(),sale_a,gen_random_uuid(),20,route_a,actor_a,'partial_service');
        RAISE EXCEPTION USING ERRCODE='XX000', MESSAGE='over-refund accepted';
    EXCEPTION WHEN raise_exception THEN ASSERT SQLERRM='missing payment or insufficient refundable amount'; END;
    ASSERT (SELECT committed_refunds=20 FROM ledger.payment WHERE sale_id=sale_a);
    BEGIN
        UPDATE ledger.refund SET approver=actor_a,state='approved' WHERE id=refund_a;
        ASSERT false,'self-approved refund';
    EXCEPTION WHEN check_violation THEN NULL; END;

    INSERT INTO wallet_mpc.wallet(wallet_id,subject_ref,create_request_id,create_intent_hash,profile_ref,policy_ref,state,created_at)
        VALUES(wallet_a,gen_random_uuid(),gen_random_uuid(),digest,'test','test','creating',moment);
    INSERT INTO wallet_mpc.key_epoch VALUES(wallet_a,1,'active',moment,moment+interval '1 day');
    BEGIN
        INSERT INTO wallet_mpc.key_epoch VALUES(wallet_a,2,'active',moment,moment+interval '1 day');
        ASSERT false,'two active epochs';
    EXCEPTION WHEN unique_violation THEN NULL; END;

    INSERT INTO market.venue VALUES (venue,'8283','test-market','pool',true);
    BEGIN
        INSERT INTO market.venue VALUES (gen_random_uuid(),'1','test-market','pool',true);
        ASSERT false,'non-pilot domain accepted';
    EXCEPTION WHEN check_violation THEN NULL; END;
    BEGIN
        PERFORM 0.1::market.uint78;
        ASSERT false,'fractional atomic units accepted';
    EXCEPTION WHEN check_violation THEN NULL; END;

    INSERT INTO travel.consent VALUES(person,'location',1,'test',NULL,true,moment,moment+interval '1 day');
    INSERT INTO travel.consent_decision VALUES(person,'location',1,'test',NULL,true,moment,moment+interval '1 day','test-proof');
    BEGIN
        INSERT INTO travel.record VALUES(gen_random_uuid(),person,'location',1,'audio',NULL,'test-object',moment,moment+interval '1 hour','active');
        ASSERT false,'audio under location consent';
    EXCEPTION WHEN check_violation THEN NULL; END;
    BEGIN
        INSERT INTO travel.processing_job VALUES(job,person,'location',2,NULL,'queued',moment,NULL);
        ASSERT false,'unrecorded decision revision';
    EXCEPTION WHEN foreign_key_violation THEN NULL; END;

    INSERT INTO platform.restore_job VALUES(job,tenant,gen_random_uuid(),1,actor_a,moment,60,5);
    BEGIN
        INSERT INTO platform.restore_approval VALUES(job,actor_a,actor_a,digest,moment,moment+interval '1 hour',30,0);
        ASSERT false,'restore operator approved own scope';
    EXCEPTION WHEN check_violation THEN NULL; END;
    BEGIN
        INSERT INTO platform.cutover_receipt VALUES(job,digest,2,moment);
        ASSERT false,'cutover without approval';
    EXCEPTION WHEN foreign_key_violation THEN NULL; END;

    INSERT INTO privacy_ops.retention_rule VALUES(rule,'access',1,'audit','local-test','security','access','reviewed-policy','expiry',moment);
    BEGIN
        INSERT INTO privacy_ops.access_record VALUES(job,tenant,actor_a,moment,digest,'download','allowed',NULL,true,true,rule,moment+interval '2 years');
        ASSERT false,'missing affected subject scope';
    EXCEPTION WHEN check_violation THEN NULL; END;
END
$checks$;
ROLLBACK;

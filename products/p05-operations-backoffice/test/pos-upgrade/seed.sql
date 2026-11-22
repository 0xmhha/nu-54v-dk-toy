\set tenant '00000000-0000-4000-8000-000000000001'
\set branch '77390000-0000-4000-8000-000000000001'
\set terminal '77390000-0000-4000-8000-000000000002'
\set source '77390000-0000-4000-8000-000000000003'
\set customer_a '77390000-0000-4000-8000-000000000004'
\set customer_b '77390000-0000-4000-8000-000000000005'
INSERT INTO ledger.branch VALUES (:'branch',:'tenant','Asia/Seoul','04:00');
INSERT INTO ledger.command_rule VALUES (:'branch','POS upgrade fixture','synthetic','expiry',interval '10 minutes');
INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,terminal_mode,valid_until) VALUES
    (:'branch',:'terminal','terminal','pos',now()+interval '10 minutes'),
    (:'branch',:'customer_a','customer',NULL,now()+interval '10 minutes'),
    (:'branch',:'customer_b','customer',NULL,now()+interval '10 minutes');
INSERT INTO ledger.pos_source VALUES(:'branch',:'source',:'terminal',now()+interval '10 minutes');
INSERT INTO ledger.payer_enrollment VALUES
    (:'branch',:'customer_a',decode(repeat('01',20),'hex'),1,1,now()+interval '10 minutes'),
    (:'branch',:'customer_b',decode(repeat('02',20),'hex'),1,1,now()+interval '10 minutes');
INSERT INTO ledger.issuance_profile VALUES(:'branch',1,decode(repeat('03',32),'hex'),120,true);
SET ROLE nu54_p05_commerce;
SELECT ledger.submit_pos_order(:'tenant',:'branch',:'source','legacy-issued','0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',7) AS issued \gset
SELECT ledger.submit_pos_order(:'tenant',:'branch',:'source','legacy-held','0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',7) AS held \gset
SELECT sale_ref AS issued_sale FROM ledger.lookup_pos_order(:'tenant',:'branch',:'terminal',:'issued') \gset
SELECT sale_ref AS held_sale FROM ledger.lookup_pos_order(:'tenant',:'branch',:'terminal',:'held') \gset
SELECT count(*) FROM ledger.authorize_checkout(:'tenant',:'branch',:'terminal',:'issued_sale',:'customer_a',gen_random_uuid(),decode(repeat('04',32),'hex'),decode(repeat('04',32),'hex'));
SELECT count(*) FROM ledger.authorize_checkout(:'tenant',:'branch',:'terminal',:'issued_sale',:'customer_b',gen_random_uuid(),decode(repeat('05',32),'hex'),decode(repeat('05',32),'hex'));
SELECT count(*) FROM ledger.authorize_checkout(:'tenant',:'branch',:'terminal',:'held_sale',:'customer_a',gen_random_uuid(),decode(repeat('06',32),'hex'),decode(repeat('06',32),'hex'));
SELECT count(*) FROM ledger.authorize_checkout(:'tenant',:'branch',:'terminal',:'held_sale',:'customer_b',gen_random_uuid(),decode(repeat('07',32),'hex'),decode(repeat('07',32),'hex'));
SELECT fence AS issued_fence FROM ledger.prepare_quote(:'tenant',:'branch',:'terminal',:'issued_sale',0,gen_random_uuid(),
   decode(repeat('08',32),'hex'),decode(repeat('04',32),'hex'),decode(repeat('03',32),'hex'),decode(repeat('09',20),'hex'),
   decode(repeat('0a',32),'hex'),decode(repeat('0b',32),'hex'),decode(repeat('0c',32),'hex')) \gset
SELECT ledger.finish_quote(:'tenant',:'branch',:'terminal',:'issued_sale',decode(repeat('04',32),'hex'),decode(repeat('03',32),'hex'),
    :issued_fence,decode(repeat('0d',765),'hex'),decode(repeat('0e',32),'hex'));
SELECT fence AS held_fence FROM ledger.prepare_quote(:'tenant',:'branch',:'terminal',:'held_sale',0,gen_random_uuid(),
   decode(repeat('08',32),'hex'),decode(repeat('06',32),'hex'),decode(repeat('03',32),'hex'),decode(repeat('09',20),'hex'),
   decode(repeat('1a',32),'hex'),decode(repeat('1b',32),'hex'),decode(repeat('1c',32),'hex')) \gset
RESET ROLE;

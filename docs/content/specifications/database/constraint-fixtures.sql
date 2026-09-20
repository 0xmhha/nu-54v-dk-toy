-- Synthetic values, only for the disposable review cluster.
SET search_path=wallet_platform,pg_catalog;
INSERT INTO accounts(id,status) VALUES ('a1','active'),('a2','active');
INSERT INTO stores(id,display_name,timezone,state) VALUES ('s1','Demo 1','Asia/Seoul','active'),('s2','Demo 2','Asia/Seoul','active');
INSERT INTO protocol_profiles(id,version,kind,schema_digest,verifier_ref,state) VALUES ('p1','draft','test','synthetic','synthetic','enabled');
INSERT INTO wallets(id,kind,chain_id,address,profile_id) VALUES ('w1','hardware_eoa',8283,'0x1111111111111111111111111111111111111111','p1');
INSERT INTO assets(id,chain_id,kind,token_address,decimals,display_symbol,is_test_asset) VALUES
('t1',8283,'erc20','0x2222222222222222222222222222222222222222',6,'TEST1',true),
('t2',8283,'erc20','0x3333333333333333333333333333333333333333',6,'TEST2',true);
INSERT INTO recipient_configurations(id,store_id,wallet_id,address,ownership_proof_ref) VALUES
('rc1','s1','w1','0x1111111111111111111111111111111111111111','synthetic'),
('rc2','s2','w1','0x1111111111111111111111111111111111111111','synthetic');
INSERT INTO orders(id,store_id,state,recipient_config_id,recipient_snapshot,price_minor,currency,fraction_digits,capability_hash) VALUES
('o1','s1','paid','rc1','0x1111111111111111111111111111111111111111',10,'KRW',0,'demo1'),
('o2','s1','paid','rc1','0x1111111111111111111111111111111111111111',10,'KRW',0,'demo2');
INSERT INTO payment_attempts(id,order_id,asset_id,amount,recipient_snapshot,quote_expires_at,state) VALUES
('pa1','o1','t1',10,'0x1111111111111111111111111111111111111111','2026-09-18T10:00:00Z','accepted'),
('pa2','o2','t1',10,'0x1111111111111111111111111111111111111111','2026-09-18T10:00:00Z','accepted');
INSERT INTO payment_evidence(id,chain_id,tx_hash,kind,log_index,asset_id,payer_address,recipient_address,amount) VALUES
('ev1',8283,'0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','erc20_log',0,'t1','0x4444444444444444444444444444444444444444','0x1111111111111111111111111111111111111111',10),
('ev2',8283,'0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb','erc20_log',0,'t1','0x4444444444444444444444444444444444444444','0x1111111111111111111111111111111111111111',10);
INSERT INTO chain_observations(id,evidence_id,chain_id,tx_hash,block_hash,block_number,receipt_status,canonicality,policy_id,policy_satisfied,observed_at) VALUES
('obs1','ev1',8283,'0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc',100,'success','canonical','test',true,'2026-09-18T09:00:00Z'),
('obs2','ev2',8283,'0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb','0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc',100,'success','canonical','test',true,'2026-09-18T09:00:00Z');
INSERT INTO payment_allocations(id,evidence_id,order_id,attempt_id,state,observation_id,asset_id) VALUES ('alloc1','ev1','o1','pa1','accepted','obs1','t1');
INSERT INTO refund_balances(id,allocation_id,asset_id,policy_refundable,order_id) VALUES ('bal1','alloc1','t1',10,'o1');
INSERT INTO devices(id,model,state) VALUES ('d1','NU-54V-DK','active'),('d2','NU-54V-DK','active');
INSERT INTO rentals(id,device_id,account_id,state) VALUES ('r1','d1','a1','active'),('r2','d2','a2','active');
INSERT INTO device_bindings(id,device_id,account_id,rental_id,wallet_id,wallet_origin) VALUES ('b1','d1','a1','r1','w1','imported'),('b2','d2','a2','r2','w1','new_travel');
INSERT INTO transaction_intents(id,wallet_id,source_kind,source_ref,profile_id,chain_id,payload_digest,review_payload_ref,expires_at) VALUES ('i1','w1','test','test','p1',8283,'0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd','synthetic','2026-09-18T10:00:00Z');

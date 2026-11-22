-- PostgreSQL 14+. Independent target design, run only in an empty scratch database.
BEGIN;
CREATE SCHEMA ledger;
CREATE SCHEMA kiosk;
CREATE SCHEMA chainview;
SET LOCAL search_path TO ledger, pg_catalog;
CREATE DOMAIN units AS numeric CHECK (VALUE >= 0 AND VALUE = trunc(VALUE)
  AND VALUE < power(2::numeric,256));
CREATE TABLE branch (
  id uuid PRIMARY KEY, tenant_ref uuid NOT NULL, timezone text NOT NULL,
  close_local time NOT NULL, UNIQUE (tenant_ref,id)
);
-- Written by the same transaction as the domain command. Scope comes from
-- the authenticated customer session, terminal credential or branch grant.
CREATE TABLE command_receipt (
  branch_id uuid NOT NULL REFERENCES branch, scope_ref uuid NOT NULL,
  operation text NOT NULL CHECK (length(operation) BETWEEN 1 AND 48),
  request_key uuid NOT NULL, intent_digest bytea NOT NULL CHECK(octet_length(intent_digest)=32),
  result_ref uuid NOT NULL, created_at timestamptz NOT NULL, retire_after timestamptz NOT NULL,
  PRIMARY KEY(branch_id,scope_ref,operation,request_key), CHECK(retire_after>created_at)
);
CREATE TABLE terminal (
  id uuid PRIMARY KEY, branch_id uuid NOT NULL REFERENCES branch,
  credential_ref uuid NOT NULL UNIQUE, desired_mode text NOT NULL CHECK (desired_mode IN ('menu','pos')),
  desired_rev bigint NOT NULL CHECK (desired_rev > 0), ack_rev bigint CHECK (ack_rev <= desired_rev),
  handoff_ref uuid, revoked_at timestamptz, UNIQUE(branch_id,id)
);
CREATE TABLE menu_release (
  branch_id uuid NOT NULL REFERENCES branch, version bigint NOT NULL CHECK(version>0),
  published_at timestamptz NOT NULL, PRIMARY KEY(branch_id,version)
);
CREATE TABLE price (
  branch_id uuid NOT NULL, version bigint NOT NULL, sku text NOT NULL, option_key text NOT NULL,
  label text NOT NULL, asset_ref text NOT NULL, unit_price units NOT NULL,
  PRIMARY KEY(branch_id,version,sku,option_key),
  FOREIGN KEY(branch_id,version) REFERENCES menu_release
);
CREATE TABLE stock (
  branch_id uuid NOT NULL REFERENCES branch, sku text NOT NULL,
  available bigint NOT NULL CHECK(available>=0), PRIMARY KEY(branch_id,sku)
);
CREATE TABLE sale (
  id uuid PRIMARY KEY, branch_id uuid NOT NULL REFERENCES branch, terminal_id uuid,
  origin text NOT NULL CHECK (origin IN ('menu','pos','booking','installment')),
  origin_ref uuid NOT NULL, revision bigint NOT NULL CHECK(revision>=0),
  state text NOT NULL CHECK(state IN ('preview','issued','checking','paid','closed')),
  UNIQUE(branch_id,origin,origin_ref), UNIQUE(branch_id,id), UNIQUE(branch_id,id,origin),
  FOREIGN KEY(branch_id,terminal_id) REFERENCES terminal(branch_id,id)
);
CREATE TABLE staff_grant (
  branch_id uuid NOT NULL REFERENCES branch, subject_ref uuid NOT NULL, role_code text NOT NULL,
  valid_from timestamptz NOT NULL, revoked_at timestamptz, PRIMARY KEY(branch_id,subject_ref,role_code,valid_from)
);
CREATE TABLE slot (
  id uuid PRIMARY KEY, branch_id uuid NOT NULL REFERENCES branch, starts_at timestamptz NOT NULL,
  ends_at timestamptz NOT NULL CHECK(ends_at>starts_at), capacity integer NOT NULL CHECK(capacity>=0),
  available integer NOT NULL CHECK(available BETWEEN 0 AND capacity), UNIQUE(branch_id,id)
);
CREATE TABLE booking (
  id uuid PRIMARY KEY, branch_id uuid NOT NULL, slot_id uuid NOT NULL, customer_scope uuid NOT NULL,
  request_key uuid NOT NULL, held_at timestamptz NOT NULL, hold_until timestamptz NOT NULL CHECK (hold_until > held_at),
  policy_version text NOT NULL, cancel_cutoff_at timestamptz NOT NULL,
  no_show_rule text NOT NULL, released_at timestamptz, seats integer NOT NULL CHECK(seats>0), price_version text NOT NULL,
  asset_ref text NOT NULL, total units NOT NULL CHECK(total>0), deposit units NOT NULL CHECK(deposit>0 AND deposit<=total),
  state text NOT NULL CHECK(state IN ('held','deposit_paid','paid','cancel_pending','closed')),
  FOREIGN KEY(branch_id,slot_id) REFERENCES slot(branch_id,id), UNIQUE(customer_scope,request_key), UNIQUE(branch_id,id)
);
CREATE TABLE booking_leg (
  branch_id uuid NOT NULL, booking_id uuid NOT NULL, leg text NOT NULL CHECK(leg IN ('deposit','balance')),
  origin text NOT NULL DEFAULT 'booking' CHECK(origin='booking'),
  asset_ref text NOT NULL, amount units NOT NULL CHECK(amount>0),
  sale_id uuid UNIQUE NOT NULL, PRIMARY KEY(booking_id,leg),
  FOREIGN KEY(branch_id,booking_id) REFERENCES booking(branch_id,id),
  FOREIGN KEY(branch_id,sale_id,origin) REFERENCES sale(branch_id,id,origin)
);
CREATE TABLE subscription (
  id uuid PRIMARY KEY, branch_id uuid NOT NULL REFERENCES branch, customer_scope uuid NOT NULL,
  terms_version text NOT NULL, terms_digest bytea NOT NULL CHECK(octet_length(terms_digest)=32),
  accepted_at timestamptz NOT NULL, cadence_ref text NOT NULL, asset_ref text NOT NULL,
  amount units NOT NULL CHECK(amount>0), next_due_at timestamptz, cancelled_at timestamptz,
  revision bigint NOT NULL CHECK(revision>=0), UNIQUE(branch_id,id),
  CHECK(cancelled_at IS NULL OR (next_due_at IS NULL AND cancelled_at>=accepted_at))
);
CREATE TABLE invoice (
  branch_id uuid NOT NULL, schedule_ref uuid NOT NULL, cycle_key text NOT NULL, due_at timestamptz NOT NULL,
  sale_id uuid UNIQUE NOT NULL, PRIMARY KEY(branch_id,schedule_ref,cycle_key),
  FOREIGN KEY(branch_id,schedule_ref) REFERENCES subscription(branch_id,id),
  FOREIGN KEY(branch_id,sale_id) REFERENCES sale(branch_id,id)
);
CREATE TABLE hold (
  branch_id uuid NOT NULL, sale_id uuid NOT NULL, sku text NOT NULL,
  quantity bigint NOT NULL CHECK(quantity>0),
  state text NOT NULL CHECK(state IN ('held','consumed','released')),
  PRIMARY KEY(sale_id,sku), FOREIGN KEY(branch_id,sale_id) REFERENCES sale(branch_id,id),
  FOREIGN KEY(branch_id,sku) REFERENCES stock
);
CREATE TABLE quote (
  sale_id uuid PRIMARY KEY REFERENCES sale, claim_digest bytea NOT NULL, domain_ref text NOT NULL,
  merchant_ref text NOT NULL, chain_order_key text NOT NULL, signer_nonce text NOT NULL,
  asset_ref text NOT NULL, amount units NOT NULL CHECK(amount>0), payout_ref text NOT NULL,
  signed_payload_enc bytea NOT NULL, issued_at timestamptz NOT NULL, expires_at timestamptz NOT NULL,
  gas_sponsor_digest bytea CHECK(gas_sponsor_digest IS NULL OR octet_length(gas_sponsor_digest)=32),
  CHECK(expires_at>issued_at), UNIQUE(domain_ref,merchant_ref,chain_order_key),
  UNIQUE(domain_ref,merchant_ref,signer_nonce)
);
CREATE TABLE sale_line (
  branch_id uuid NOT NULL, sale_id uuid NOT NULL, line_no integer NOT NULL CHECK(line_no>0),
  version bigint NOT NULL, sku text NOT NULL, option_key text NOT NULL,
  quantity bigint NOT NULL CHECK(quantity>0), PRIMARY KEY(sale_id,line_no),
  FOREIGN KEY(branch_id,sale_id) REFERENCES sale(branch_id,id),
  FOREIGN KEY(branch_id,version,sku,option_key) REFERENCES price
);
CREATE TABLE payment (
  sale_id uuid PRIMARY KEY REFERENCES quote, evidence_ref uuid NOT NULL UNIQUE,
  principal units NOT NULL CHECK(principal>0), committed_refunds units NOT NULL DEFAULT 0,
  CHECK(committed_refunds<=principal)
);
CREATE TABLE refund (
  id uuid PRIMARY KEY, sale_id uuid NOT NULL REFERENCES payment, request_key uuid NOT NULL,
  amount units NOT NULL CHECK(amount>0), route_ref uuid NOT NULL, requester uuid NOT NULL,
  approver uuid, state text NOT NULL CHECK(state IN ('requested','approved','funding','submitted','final','cancelled')),
  reason_code text CHECK(reason_code IN ('partial_service','merchant_cancel','duplicate_charge')),
  decision_path text NOT NULL DEFAULT 'manual',
  review_revision bigint NOT NULL DEFAULT 0 CHECK(review_revision>=0),
  CHECK((decision_path='prior' AND reason_code IS NULL) OR (decision_path='manual' AND reason_code IS NOT NULL)),
  rejection_code text CHECK(rejection_code IN ('route_review','amount_review','policy_review')),
  rejected_at timestamptz, treasury_actor uuid, treasury_submitted_at timestamptz,
  CHECK((rejection_code IS NULL AND rejected_at IS NULL) OR
        (rejection_code IS NOT NULL AND rejected_at IS NOT NULL AND state='cancelled' AND approver IS NOT NULL)),
  CHECK((treasury_actor IS NULL AND treasury_submitted_at IS NULL) OR
        (treasury_actor IS NOT NULL AND treasury_submitted_at IS NOT NULL AND treasury_actor<>requester
          AND approver IS NOT NULL AND treasury_actor<>approver AND state IN ('approved','funding','submitted','final'))),
  CHECK(approver IS NULL OR approver<>requester),
  CHECK(state NOT IN ('approved','funding','submitted','final') OR approver IS NOT NULL),
  UNIQUE(sale_id,request_key), UNIQUE(sale_id,id)
);
CREATE FUNCTION reserve_refund(p_id uuid,p_sale uuid,p_key uuid,p_amount units,p_route uuid,p_actor uuid,p_reason text)
RETURNS uuid LANGUAGE plpgsql SET search_path=ledger,pg_catalog AS $$
DECLARE previous refund%ROWTYPE;
BEGIN
  PERFORM 1 FROM payment WHERE sale_id=p_sale FOR UPDATE;
  SELECT * INTO previous FROM refund WHERE sale_id=p_sale AND request_key=p_key;
  IF FOUND THEN
    IF ROW(previous.amount,previous.route_ref,previous.requester,previous.reason_code)
       IS DISTINCT FROM ROW(p_amount,p_route,p_actor,p_reason) THEN RAISE EXCEPTION 'request key reused'; END IF;
    RETURN previous.id;
  END IF;
  UPDATE payment SET committed_refunds=committed_refunds+p_amount
    WHERE sale_id=p_sale AND committed_refunds+p_amount<=principal;
  IF NOT FOUND THEN RAISE EXCEPTION 'missing payment or insufficient refundable amount'; END IF;
  INSERT INTO refund(id,sale_id,request_key,amount,route_ref,requester,state,reason_code)
    VALUES(p_id,p_sale,p_key,p_amount,p_route,p_actor,'requested',p_reason);
  RETURN p_id;
END $$;
CREATE TABLE refund_failed_attempt (
    refund_id uuid NOT NULL REFERENCES refund,
    review_revision bigint NOT NULL CHECK(review_revision>=0),
    tx_hash text NOT NULL UNIQUE CHECK(tx_hash ~ '^0x[0-9a-f]{64}$'),
    sender text NOT NULL CHECK(sender ~ '^0x[0-9a-f]{40}$'),
    nonce numeric NOT NULL CHECK(nonce>=0 AND nonce=trunc(nonce) AND nonce<power(2::numeric,64)),
    block_hash text NOT NULL CHECK(block_hash ~ '^0x[0-9a-f]{64}$'),
    height bigint NOT NULL CHECK(height>=0),
    checker uuid NOT NULL,
    treasury uuid NOT NULL,
    recorded_at timestamptz NOT NULL CHECK(isfinite(recorded_at)),
    PRIMARY KEY(refund_id,review_revision),
    UNIQUE(sender,nonce)
);
CREATE TABLE posting (
  id uuid PRIMARY KEY, sale_id uuid NOT NULL REFERENCES payment, refund_id uuid UNIQUE,
  FOREIGN KEY(sale_id,refund_id) REFERENCES refund(sale_id,id),
  evidence_ref uuid NOT NULL UNIQUE, booked_at timestamptz NOT NULL, business_day date NOT NULL,
  asset_ref text NOT NULL, amount units NOT NULL CHECK(amount>0), account_ref text NOT NULL
);
CREATE UNIQUE INDEX one_sale_posting ON posting(sale_id) WHERE refund_id IS NULL;
CREATE TABLE outbox (
  event_id uuid PRIMARY KEY, sale_id uuid NOT NULL REFERENCES sale, event_type text NOT NULL,
  source_ref uuid NOT NULL, revision bigint NOT NULL, delivered_at timestamptz,
  UNIQUE(event_type,source_ref,revision)
);
CREATE TABLE receipt_delivery (
  sale_id uuid PRIMARY KEY, branch_id uuid NOT NULL, business_day date NOT NULL,
  pickup_no integer NOT NULL CHECK(pickup_no>0), receipt_ref uuid UNIQUE NOT NULL,
  FOREIGN KEY(branch_id,sale_id) REFERENCES sale(branch_id,id),
  UNIQUE(branch_id,business_day,pickup_no)
);
CREATE TABLE benefit_claim (
  id uuid PRIMARY KEY, sale_id uuid NOT NULL REFERENCES payment, subject_scope uuid NOT NULL,
  rule_version text NOT NULL, fulfillment_ref uuid NOT NULL, source_evidence_ref uuid NOT NULL,
  environment text NOT NULL CHECK(environment IN ('test','live')),
  UNIQUE(subject_scope,sale_id,rule_version), UNIQUE(id,environment)
);
CREATE TABLE benefit_entry (
  id uuid PRIMARY KEY, claim_id uuid NOT NULL, environment text NOT NULL CHECK(environment='live'),
  source_event_ref uuid NOT NULL, kind text NOT NULL CHECK(kind IN ('grant','hold','adjust')),
  delta integer NOT NULL, recorded_at timestamptz NOT NULL,
  FOREIGN KEY(claim_id,environment) REFERENCES benefit_claim(id,environment),
  CHECK((kind='grant' AND delta>0) OR (kind='hold' AND delta=0) OR (kind='adjust' AND delta<0)),
  UNIQUE(claim_id,source_event_ref,kind)
);
CREATE UNIQUE INDEX one_benefit_grant ON benefit_entry(claim_id) WHERE kind='grant';
CREATE TABLE rental (
  id uuid PRIMARY KEY, branch_id uuid NOT NULL REFERENCES branch, device_ref uuid NOT NULL,
  slot_generation bigint NOT NULL, account_ref text NOT NULL, return_route_ref uuid NOT NULL,
  deployment_ref text NOT NULL, state text NOT NULL CHECK(state IN ('opening','active','returning','closed')),
  return_evidence_ref uuid, UNIQUE(device_ref,slot_generation), UNIQUE(branch_id,id),
  CHECK(state<>'closed' OR return_evidence_ref IS NOT NULL)
);
CREATE TABLE campaign_target (
  campaign_id uuid NOT NULL, target_key uuid NOT NULL, branch_id uuid NOT NULL REFERENCES branch,
  terminal_id uuid, rental_id uuid,
  FOREIGN KEY(branch_id,terminal_id) REFERENCES terminal(branch_id,id),
  FOREIGN KEY(branch_id,rental_id) REFERENCES rental(branch_id,id),
  CHECK(num_nonnulls(terminal_id,rental_id)=1 AND target_key=coalesce(terminal_id,rental_id)),
  release_ref uuid NOT NULL, supported_revision text NOT NULL, last_ack_revision bigint,
  state text NOT NULL CHECK(state IN ('queued','applying','verified','held')),
  PRIMARY KEY(campaign_id,target_key)
);
CREATE TABLE resolution (
  sale_id uuid PRIMARY KEY REFERENCES quote, kind text NOT NULL CHECK(kind IN ('absent','invalidated')),
  evidence_ref uuid NOT NULL UNIQUE, recorded_at timestamptz NOT NULL
);
CREATE TABLE payout_change (
  id uuid PRIMARY KEY, branch_id uuid NOT NULL REFERENCES branch, domain_ref text NOT NULL,
  payout_ref text NOT NULL, ownership_proof_ref uuid NOT NULL, requested_by uuid NOT NULL,
  first_approver uuid, second_approver uuid, effective_at timestamptz,
  CHECK(first_approver<>second_approver AND requested_by<>first_approver AND requested_by<>second_approver),
  CHECK(effective_at IS NULL OR (first_approver IS NOT NULL AND second_approver IS NOT NULL))
);
CREATE TABLE day_close (
  branch_id uuid NOT NULL REFERENCES branch, business_day date NOT NULL, asset_ref text NOT NULL,
  revision bigint NOT NULL, evidence_cutoff_ref uuid NOT NULL, discrepancy numeric NOT NULL,
  submitted_by uuid, PRIMARY KEY(branch_id,business_day,asset_ref,revision),
  CHECK(submitted_by IS NULL OR discrepancy=0)
);
CREATE TABLE relay_intent (
  id uuid PRIMARY KEY, sale_id uuid NOT NULL REFERENCES sale, refund_id uuid UNIQUE REFERENCES refund,
  chain_ref text NOT NULL, sender_ref text NOT NULL, tx_nonce numeric NOT NULL CHECK(tx_nonce>=0 AND tx_nonce=trunc(tx_nonce)),
  intent_digest bytea NOT NULL, UNIQUE(chain_ref,sender_ref,tx_nonce),
  FOREIGN KEY(sale_id,refund_id) REFERENCES refund(sale_id,id)
);
CREATE UNIQUE INDEX one_payment_intent ON relay_intent(sale_id) WHERE refund_id IS NULL;
CREATE TABLE relay_attempt (
  intent_id uuid NOT NULL REFERENCES relay_intent, attempt integer NOT NULL CHECK(attempt>0),
  tx_hash text NOT NULL UNIQUE, tx_ciphertext bytea NOT NULL, result text NOT NULL
  CHECK(result IN ('signed','unknown','included','reverted','final')),
  PRIMARY KEY(intent_id,attempt)
);
CREATE TABLE kiosk.delivery_inbox (
  event_ref uuid PRIMARY KEY, terminal_ref uuid NOT NULL, order_ref uuid NOT NULL,
  action text NOT NULL, effect_key uuid NOT NULL UNIQUE, received_at timestamptz NOT NULL,
  acknowledged_at timestamptz, UNIQUE(terminal_ref,order_ref,action)
);
SET LOCAL search_path TO chainview,pg_catalog;
CREATE TABLE stream (
  id uuid PRIMARY KEY, chain_ref text NOT NULL, contract_ref text NOT NULL, abi_hash text NOT NULL,
  manifest_ref text NOT NULL, UNIQUE(chain_ref,contract_ref,abi_hash,manifest_ref)
);
CREATE TABLE generation (
  id uuid PRIMARY KEY, stream_id uuid NOT NULL REFERENCES stream,
  base_height bigint NOT NULL CHECK(base_height>=0), tip_height bigint, tip_hash text,
  finalized_height bigint, finalized_hash text, fence bigint NOT NULL CHECK(fence>=0),
  lease_until timestamptz, state text NOT NULL CHECK(state IN ('building','ready','frozen')),
  CHECK((tip_height IS NULL)=(tip_hash IS NULL)), CHECK((finalized_height IS NULL)=(finalized_hash IS NULL)),
  CHECK(finalized_height IS NULL OR (tip_height IS NOT NULL AND finalized_height BETWEEN base_height AND tip_height)),
  UNIQUE(stream_id,id)
);
CREATE TABLE block (
  generation_id uuid NOT NULL REFERENCES generation, block_hash text NOT NULL,
  height bigint NOT NULL CHECK(height>=0), parent_hash text NOT NULL, block_time timestamptz NOT NULL,
  canonical boolean NOT NULL, PRIMARY KEY(generation_id,block_hash), UNIQUE(generation_id,height,block_hash)
);
CREATE UNIQUE INDEX one_canonical_height ON block(generation_id,height) WHERE canonical;
ALTER TABLE generation ADD FOREIGN KEY(id,tip_height,tip_hash) REFERENCES block(generation_id,height,block_hash);
ALTER TABLE generation ADD FOREIGN KEY(id,finalized_height,finalized_hash) REFERENCES block(generation_id,height,block_hash);
CREATE TABLE observed_event (
  generation_id uuid NOT NULL, block_hash text NOT NULL, tx_hash text NOT NULL, log_index integer NOT NULL CHECK(log_index>=0),
  kind text NOT NULL CHECK(kind IN ('paid','refunded','invalidated','cashout','rental')),
  receipt_success boolean NOT NULL CHECK(receipt_success), recipient_ref text, signer_nonce text,
  merchant_key text NOT NULL, order_key text NOT NULL, refund_key text, asset_ref text,
  amount numeric CHECK(amount>=0 AND amount=trunc(amount) AND amount<power(2::numeric,256)),
  PRIMARY KEY(generation_id,block_hash,tx_hash,log_index),
  FOREIGN KEY(generation_id,block_hash) REFERENCES block,
  CHECK((kind='refunded')=(refund_key IS NOT NULL)),
  CHECK(kind NOT IN ('paid','refunded') OR (asset_ref IS NOT NULL AND recipient_ref IS NOT NULL AND amount IS NOT NULL AND amount>0)),
  CHECK(kind<>'paid' OR signer_nonce IS NOT NULL)
);
CREATE INDEX event_lookup ON observed_event(generation_id,merchant_key,order_key);
CREATE TABLE evidence (
  id uuid PRIMARY KEY, generation_id uuid NOT NULL, merchant_key text NOT NULL, order_key text NOT NULL,
  kind text NOT NULL CHECK(kind IN ('paid','refunded','absent','invalidated')),
  anchor_height bigint NOT NULL, anchor_hash text NOT NULL, policy_ref text NOT NULL,
  expires_at timestamptz, coverage_start bigint, state_proof_ref text,
  log_block text, tx_hash text, log_index integer, created_at timestamptz NOT NULL,
  FOREIGN KEY(generation_id,anchor_height,anchor_hash) REFERENCES block(generation_id,height,block_hash),
  FOREIGN KEY(generation_id,log_block,tx_hash,log_index) REFERENCES observed_event,
  CHECK((kind='absent' AND expires_at IS NOT NULL AND coverage_start IS NOT NULL
    AND coverage_start BETWEEN 0 AND anchor_height AND state_proof_ref IS NOT NULL
    AND log_block IS NULL AND tx_hash IS NULL AND log_index IS NULL)
    OR (kind<>'absent' AND log_block IS NOT NULL AND tx_hash IS NOT NULL AND log_index IS NOT NULL))
);
CREATE TABLE read_target (
  stream_id uuid PRIMARY KEY REFERENCES stream, generation_id uuid NOT NULL, revision bigint NOT NULL,
  FOREIGN KEY(stream_id,generation_id) REFERENCES generation(stream_id,id)
);
CREATE TABLE projection_comparison (
    id uuid PRIMARY KEY,
    stream_id uuid NOT NULL,
    candidate_id uuid NOT NULL,
    previous_id uuid,
    expected_revision bigint NOT NULL CHECK (expected_revision >= 0),
    anchor_height bigint NOT NULL,
    anchor_hash text NOT NULL,
    compared_at timestamptz NOT NULL,
    FOREIGN KEY(stream_id, candidate_id) REFERENCES generation(stream_id,id),
    FOREIGN KEY(stream_id, previous_id) REFERENCES generation(stream_id,id)
);

CREATE TABLE profile_hold (
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
CREATE TABLE replay_job (
    id uuid PRIMARY KEY,
    stream_id uuid NOT NULL,
    previous_id uuid NOT NULL,
    expected_revision bigint NOT NULL,
    anchor_height bigint NOT NULL,
    anchor_hash text NOT NULL,
    state text NOT NULL CHECK (state IN ('running','matched','diff')),
    reason text CHECK (reason IN ('projection_diff','anchor_changed')),
    comparison_id uuid REFERENCES projection_comparison,
    requested_at timestamptz NOT NULL,
    finished_at timestamptz,
    differing_logs bigint CHECK (differing_logs>=0),
    paid_delta numeric,
    refund_delta numeric,
    FOREIGN KEY (stream_id,id) REFERENCES generation(stream_id,id),
    FOREIGN KEY (stream_id,previous_id) REFERENCES generation(stream_id,id),
    CHECK ((state='running')=(finished_at IS NULL)),
    CHECK ((state='matched')=(comparison_id IS NOT NULL)),
    CHECK ((state='diff')=(reason IS NOT NULL)),
    CHECK ((differing_logs IS NOT NULL AND paid_delta IS NOT NULL AND refund_delta IS NOT NULL)
        = (state='matched' OR COALESCE(reason='projection_diff',false))),
    CHECK (num_nonnulls(differing_logs,paid_delta,refund_delta) IN (0,3))
);
CREATE UNIQUE INDEX replay_job_one_running ON replay_job(stream_id) WHERE state='running';

CREATE TABLE switch_request (
  id uuid PRIMARY KEY, stream_id uuid NOT NULL, target_id uuid NOT NULL, expected_revision bigint NOT NULL,
  operator_ref uuid NOT NULL, compare_ref uuid NOT NULL, requested_at timestamptz NOT NULL,
  applied_at timestamptz, FOREIGN KEY(stream_id,target_id) REFERENCES generation(stream_id,id)
);
CREATE TABLE incident (
  id uuid PRIMARY KEY, generation_id uuid NOT NULL REFERENCES generation, disputed_height bigint NOT NULL,
  old_hash text NOT NULL, new_hash text NOT NULL CHECK(new_hash<>old_hash), opened_at timestamptz NOT NULL,
  resolved_at timestamptz, notified_at timestamptz
);
COMMIT;

# 테이블·컬럼·관계 사전

PostgreSQL 참조 설계. [적용 순서·검사 범위](README.md). 모든 업무 테이블에는 id, created_at, revision이 있다. FK는 기본 NO ACTION이며 별도 CASCADE 삭제는 정의하지 않았다.

## accounts

저장 책임: identity. migration: 001. 작업: AUTH-01.

```sql
id app_id PRIMARY KEY,
status text NOT NULL CHECK (status IN ('active','deletion_pending','deactivated')),
display_name_ref text,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## auth_identities

저장 책임: identity. migration: 001. 작업: AUTH-02, AUTH-03, AUTH-04.

```sql
id app_id PRIMARY KEY,
account_id app_id NOT NULL REFERENCES accounts(id),
provider text NOT NULL CHECK (provider IN ('google','apple')),
provider_subject text NOT NULL,
revoked_at timestamptz,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (provider, provider_subject)
```

## auth_sessions

저장 책임: identity. migration: 001. 작업: AUTH-04.

```sql
id app_id PRIMARY KEY,
account_id app_id NOT NULL REFERENCES accounts(id),
credential_hash text NOT NULL UNIQUE,
expires_at timestamptz NOT NULL,
revoked_at timestamptz,
auth_flow_ref text,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## stores

저장 책임: merchant. migration: 001. 작업: SHOP-01.

```sql
id app_id PRIMARY KEY,
display_name text NOT NULL,
timezone text NOT NULL,
state text NOT NULL CHECK (state IN ('active','suspended','closed')),
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## memberships

저장 책임: merchant. migration: 001. 작업: AUTH-01.

```sql
id app_id PRIMARY KEY,
store_id app_id NOT NULL REFERENCES stores(id),
account_id app_id NOT NULL REFERENCES accounts(id),
role_code text NOT NULL,
revoked_at timestamptz,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (store_id, account_id)
```

## protocol_profiles

저장 책임: platform. migration: 001. 작업: BASE-03, BASE-06.

```sql
id app_id PRIMARY KEY,
version text NOT NULL,
kind text NOT NULL,
schema_digest text NOT NULL,
verifier_ref text NOT NULL,
state text NOT NULL CHECK (state IN ('draft','enabled','revoked')),
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## assets

저장 책임: chain_registry. migration: 001. 작업: TOKEN-01.

```sql
id app_id PRIMARY KEY,
chain_id bigint NOT NULL CHECK (chain_id = 8283),
kind text NOT NULL CHECK (kind IN ('native','erc20')),
token_address evm_address,
decimals smallint NOT NULL CHECK (decimals BETWEEN 0 AND 255),
display_symbol text NOT NULL,
is_test_asset boolean NOT NULL,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
CHECK ((kind = 'native' AND token_address IS NULL) OR (kind = 'erc20' AND token_address IS NOT NULL)),
UNIQUE (id, chain_id)
```

인덱스:

```sql
CREATE UNIQUE INDEX assets_native_unique ON assets(chain_id) WHERE kind = 'native';
CREATE UNIQUE INDEX assets_token_unique ON assets(chain_id, token_address) WHERE kind = 'erc20';
```

## operations

저장 책임: platform. migration: 001. 작업: BASE-02.

```sql
id app_id PRIMARY KEY,
account_id app_id REFERENCES accounts(id),
principal_scope text NOT NULL,
kind text NOT NULL,
state text NOT NULL CHECK (state IN ('queued','running','succeeded','failed','unknown','cancelled')),
result_ref text,
safe_error_code text,
updated_at timestamptz NOT NULL DEFAULT now(),
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## idempotency_records

저장 책임: platform. migration: 001. 작업: BASE-03.

```sql
id app_id PRIMARY KEY,
principal_scope text NOT NULL,
action text NOT NULL,
request_key text NOT NULL,
normalized_digest text NOT NULL,
operation_id app_id REFERENCES operations(id),
result_ref text,
expires_at timestamptz NOT NULL,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (principal_scope, action, request_key)
```

## outbox

저장 책임: platform. migration: 001. 작업: INDEX-06.

```sql
id app_id PRIMARY KEY,
aggregate_type text NOT NULL,
aggregate_id app_id NOT NULL,
aggregate_revision bigint NOT NULL CHECK (aggregate_revision >= 0),
event_type text NOT NULL,
safe_payload jsonb NOT NULL,
correlation_id app_id NOT NULL,
published_at timestamptz,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

인덱스:

```sql
CREATE INDEX outbox_unpublished ON outbox(created_at,id) WHERE published_at IS NULL;
```

## inbox

저장 책임: platform. migration: 001. 작업: INDEX-06.

```sql
id app_id PRIMARY KEY,
consumer_id text NOT NULL,
event_id app_id NOT NULL,
processed_at timestamptz NOT NULL DEFAULT now(),
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (consumer_id, event_id)
```

## audit_events

저장 책임: operations. migration: 001. 작업: OPS-01.

```sql
id app_id PRIMARY KEY,
actor_ref text NOT NULL,
subject_type text NOT NULL,
subject_id app_id NOT NULL,
action text NOT NULL,
safe_diff_ref text,
reason text,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## wallets

저장 책임: wallet. migration: 002. 작업: APP-02, SMART-01.

```sql
id app_id PRIMARY KEY,
kind text NOT NULL CHECK (kind IN ('hardware_eoa','cloud_mpc_eoa','smart_account')),
chain_id bigint NOT NULL CHECK (chain_id = 8283),
address evm_address NOT NULL,
signer_ref text,
profile_id app_id REFERENCES protocol_profiles(id),
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (id, chain_id)
```

## wallet_bindings

저장 책임: wallet. migration: 002. 작업: APP-02, SHOP-06.

```sql
id app_id PRIMARY KEY,
wallet_id app_id NOT NULL REFERENCES wallets(id),
account_id app_id REFERENCES accounts(id),
store_id app_id REFERENCES stores(id),
permission_profile text NOT NULL,
revoked_at timestamptz,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
CHECK ((account_id IS NOT NULL)::int + (store_id IS NOT NULL)::int = 1)
```

인덱스:

```sql
CREATE UNIQUE INDEX wallet_account_active ON wallet_bindings(wallet_id,account_id) WHERE account_id IS NOT NULL AND revoked_at IS NULL;
CREATE UNIQUE INDEX wallet_store_active ON wallet_bindings(wallet_id,store_id) WHERE store_id IS NOT NULL AND revoked_at IS NULL;
```

## devices

저장 책임: device. migration: 002. 작업: HW-01.

```sql
id app_id PRIMARY KEY,
model text NOT NULL,
firmware_version text,
identity_verifier_ref text,
state text NOT NULL CHECK (state IN ('unprovisioned','assigned','active','return_pending','quarantined')),
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## rentals

저장 책임: rental. migration: 002. 작업: STAMP-03.

```sql
id app_id PRIMARY KEY,
device_id app_id NOT NULL REFERENCES devices(id),
account_id app_id NOT NULL REFERENCES accounts(id),
state text NOT NULL CHECK (state IN ('assigned','active','return_requested','return_pending','returned')),
returned_at timestamptz,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (id, device_id, account_id),
CHECK ((state = 'returned') = (returned_at IS NOT NULL))
```

인덱스:

```sql
CREATE UNIQUE INDEX one_active_rental ON rentals(device_id) WHERE state <> 'returned';
```

## device_bindings

저장 책임: device. migration: 002. 작업: HW-03, HW-04.

```sql
id app_id PRIMARY KEY,
device_id app_id NOT NULL REFERENCES devices(id),
account_id app_id NOT NULL REFERENCES accounts(id),
rental_id app_id,
wallet_id app_id REFERENCES wallets(id),
wallet_origin text NOT NULL CHECK (wallet_origin IN ('new_travel','imported')),
revoked_at timestamptz,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
FOREIGN KEY (rental_id,device_id,account_id) REFERENCES rentals(id,device_id,account_id),
UNIQUE (id,device_id,account_id),
UNIQUE (id,rental_id,wallet_origin)
```

인덱스:

```sql
CREATE UNIQUE INDEX one_active_device_owner ON device_bindings(device_id) WHERE revoked_at IS NULL;
```

## enrollments

저장 책임: device. migration: 002. 작업: HW-03.

```sql
id app_id PRIMARY KEY,
device_id app_id NOT NULL REFERENCES devices(id),
account_id app_id NOT NULL REFERENCES accounts(id),
challenge_hash text NOT NULL,
profile_id app_id NOT NULL REFERENCES protocol_profiles(id),
expires_at timestamptz NOT NULL,
consumed_at timestamptz,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## mpc_participants

저장 책임: mpc. migration: 002. 작업: MPC-02, MPC-04.

```sql
id app_id PRIMARY KEY,
wallet_id app_id NOT NULL REFERENCES wallets(id),
participant_ref text NOT NULL,
share_store_ref text NOT NULL,
profile_id app_id NOT NULL REFERENCES protocol_profiles(id),
state text NOT NULL CHECK (state IN ('pending','active','revoked')),
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (wallet_id,participant_ref)
```

## signing_sessions

저장 책임: mpc. migration: 002. 작업: MPC-03, MPC-05.

```sql
id app_id PRIMARY KEY,
wallet_id app_id NOT NULL REFERENCES wallets(id),
operation_id app_id NOT NULL REFERENCES operations(id),
profile_id app_id NOT NULL REFERENCES protocol_profiles(id),
payload_digest hash32 NOT NULL,
state text NOT NULL CHECK (state IN ('pending','signing','completed','aborted','failed')),
expires_at timestamptz NOT NULL,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## recipient_configurations

저장 책임: merchant_funds. migration: 003. 작업: SHOP-06.

```sql
id app_id PRIMARY KEY,
store_id app_id NOT NULL REFERENCES stores(id),
wallet_id app_id NOT NULL REFERENCES wallets(id),
address evm_address NOT NULL,
ownership_proof_ref text NOT NULL,
retired_at timestamptz,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (id, store_id)
```

인덱스:

```sql
CREATE UNIQUE INDEX current_store_recipient ON recipient_configurations(store_id) WHERE retired_at IS NULL;
```

## menu_items

저장 책임: merchant. migration: 003. 작업: SHOP-02.

```sql
id app_id PRIMARY KEY,
store_id app_id NOT NULL REFERENCES stores(id),
name text NOT NULL,
options_snapshot jsonb NOT NULL,
price_minor atomic_total NOT NULL,
currency text NOT NULL,
fraction_digits smallint NOT NULL CHECK (fraction_digits BETWEEN 0 AND 18),
availability text NOT NULL CHECK (availability IN ('available','sold_out','hidden')),
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (id,store_id)
```

## orders

저장 책임: orders. migration: 003. 작업: PAY-03, SHOP-02.

```sql
id app_id PRIMARY KEY,
store_id app_id NOT NULL REFERENCES stores(id),
account_id app_id REFERENCES accounts(id),
state text NOT NULL CHECK (state IN ('awaiting_payment','paid','cancelled','expired','fulfilled')),
recipient_config_id app_id NOT NULL,
recipient_snapshot evm_address NOT NULL,
price_minor atomic_total NOT NULL,
currency text NOT NULL,
fraction_digits smallint NOT NULL CHECK (fraction_digits BETWEEN 0 AND 18),
capability_hash text NOT NULL,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
FOREIGN KEY (recipient_config_id,store_id) REFERENCES recipient_configurations(id,store_id),
UNIQUE (id,store_id)
```

인덱스:

```sql
CREATE INDEX orders_store_cursor ON orders(store_id,created_at,id);
```

## order_lines

저장 책임: orders. migration: 003. 작업: SHOP-02.

```sql
id app_id PRIMARY KEY,
order_id app_id NOT NULL REFERENCES orders(id),
line_no integer NOT NULL CHECK (line_no > 0),
item_snapshot jsonb NOT NULL,
quantity integer NOT NULL CHECK (quantity > 0),
line_total atomic_total NOT NULL,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (order_id,line_no)
```

## payment_attempts

저장 책임: payment. migration: 003. 작업: PAY-01, PAY-03.

```sql
id app_id PRIMARY KEY,
order_id app_id NOT NULL REFERENCES orders(id),
asset_id app_id NOT NULL REFERENCES assets(id),
amount uint256_amount NOT NULL CHECK (amount > 0),
recipient_snapshot evm_address NOT NULL,
quote_expires_at timestamptz NOT NULL,
state text NOT NULL,
payer_address evm_address,
session_ref text,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (id,order_id),
UNIQUE (id,asset_id),
UNIQUE (id,order_id,asset_id)
```

## transaction_intents

저장 책임: transaction. migration: 003. 작업: PAY-01, SMART-02.

```sql
id app_id PRIMARY KEY,
attempt_id app_id REFERENCES payment_attempts(id),
wallet_id app_id REFERENCES wallets(id),
source_kind text NOT NULL,
source_ref app_id NOT NULL,
profile_id app_id NOT NULL REFERENCES protocol_profiles(id),
chain_id bigint NOT NULL CHECK (chain_id = 8283),
payload_digest hash32 NOT NULL,
review_payload_ref text NOT NULL,
expires_at timestamptz NOT NULL,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## transaction_submissions

저장 책임: transaction. migration: 003. 작업: PAY-02.

```sql
id app_id PRIMARY KEY,
intent_id app_id NOT NULL REFERENCES transaction_intents(id),
chain_id bigint NOT NULL CHECK (chain_id = 8283),
tx_hash hash32,
userop_hash hash32,
state text NOT NULL CHECK (state IN ('prepared','submitted','unknown','observed','failed')),
signed_payload_ref text,
submission_kind text NOT NULL CHECK (submission_kind IN ('eoa','user_operation')),
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
CHECK ((submission_kind = 'eoa' AND tx_hash IS NOT NULL AND userop_hash IS NULL) OR (submission_kind = 'user_operation' AND userop_hash IS NOT NULL))
```

인덱스:

```sql
CREATE UNIQUE INDEX submission_eoa_unique ON transaction_submissions(chain_id,tx_hash) WHERE submission_kind = 'eoa';
CREATE UNIQUE INDEX submission_userop_unique ON transaction_submissions(chain_id,userop_hash) WHERE submission_kind = 'user_operation';
CREATE INDEX submission_bundle_lookup ON transaction_submissions(chain_id,tx_hash) WHERE tx_hash IS NOT NULL;
```

## payment_evidence

저장 책임: reconciliation. migration: 003. 작업: INDEX-03, PAY-04.

```sql
id app_id PRIMARY KEY,
chain_id bigint NOT NULL CHECK (chain_id = 8283),
tx_hash hash32 NOT NULL,
kind text NOT NULL CHECK (kind IN ('erc20_log','native_transaction')),
log_index integer,
asset_id app_id NOT NULL,
payer_address evm_address NOT NULL,
recipient_address evm_address NOT NULL,
amount uint256_amount NOT NULL,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
CHECK ((kind = 'erc20_log' AND log_index IS NOT NULL AND log_index >= 0) OR (kind = 'native_transaction' AND log_index IS NULL)),
FOREIGN KEY (asset_id,chain_id) REFERENCES assets(id,chain_id),
UNIQUE (id,asset_id),
UNIQUE (id,chain_id,tx_hash)
```

인덱스:

```sql
CREATE UNIQUE INDEX evidence_log_unique ON payment_evidence(chain_id,tx_hash,log_index) WHERE kind = 'erc20_log';
CREATE UNIQUE INDEX evidence_native_unique ON payment_evidence(chain_id,tx_hash) WHERE kind = 'native_transaction';
```

## chain_observations

저장 책임: indexer_adapter. migration: 003. 작업: INDEX-03, INDEX-06.

```sql
id app_id PRIMARY KEY,
evidence_id app_id REFERENCES payment_evidence(id),
chain_id bigint NOT NULL CHECK (chain_id = 8283),
tx_hash hash32 NOT NULL,
block_hash hash32 NOT NULL,
block_number numeric NOT NULL CHECK (block_number >= 0 AND block_number = trunc(block_number)),
receipt_status text NOT NULL CHECK (receipt_status IN ('success','reverted')),
canonicality text NOT NULL CHECK (canonicality IN ('canonical','orphaned','unknown')),
policy_id text NOT NULL,
policy_satisfied boolean NOT NULL,
observed_at timestamptz NOT NULL,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
CHECK (evidence_id IS NULL OR receipt_status = 'success'),
FOREIGN KEY (evidence_id,chain_id,tx_hash) REFERENCES payment_evidence(id,chain_id,tx_hash),
UNIQUE (id,evidence_id)
```

## payment_allocations

저장 책임: reconciliation. migration: 003. 작업: PAY-04.

```sql
id app_id PRIMARY KEY,
evidence_id app_id NOT NULL UNIQUE REFERENCES payment_evidence(id),
order_id app_id NOT NULL REFERENCES orders(id),
attempt_id app_id NOT NULL,
state text NOT NULL CHECK (state IN ('pending','accepted','exception','invalidated')),
observation_id app_id NOT NULL REFERENCES chain_observations(id),
asset_id app_id NOT NULL,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
FOREIGN KEY (attempt_id,order_id) REFERENCES payment_attempts(id,order_id),
FOREIGN KEY (attempt_id,order_id,asset_id) REFERENCES payment_attempts(id,order_id,asset_id),
FOREIGN KEY (evidence_id,asset_id) REFERENCES payment_evidence(id,asset_id),
FOREIGN KEY (observation_id,evidence_id) REFERENCES chain_observations(id,evidence_id),
UNIQUE (id,order_id,asset_id)
```

## refund_balances

저장 책임: refund. migration: 003. 작업: SHOP-04.

```sql
id app_id PRIMARY KEY,
allocation_id app_id NOT NULL UNIQUE REFERENCES payment_allocations(id),
asset_id app_id NOT NULL REFERENCES assets(id),
policy_refundable atomic_total NOT NULL,
reserved atomic_total NOT NULL DEFAULT 0,
confirmed atomic_total NOT NULL DEFAULT 0,
order_id app_id NOT NULL,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
CHECK (reserved + confirmed <= policy_refundable),
UNIQUE (id,asset_id),
FOREIGN KEY (allocation_id,order_id,asset_id) REFERENCES payment_allocations(id,order_id,asset_id),
UNIQUE (id,asset_id,order_id)
```

## refunds

저장 책임: refund. migration: 003. 작업: SHOP-04.

```sql
id app_id PRIMARY KEY,
order_id app_id NOT NULL REFERENCES orders(id),
balance_id app_id NOT NULL,
asset_id app_id NOT NULL,
amount uint256_amount NOT NULL CHECK (amount > 0),
destination evm_address NOT NULL,
destination_proof_ref text NOT NULL,
state text NOT NULL CHECK (state IN ('requested','authorized','signing','submitted','unknown','confirmed','rejected','failed')),
intent_id app_id REFERENCES transaction_intents(id),
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
FOREIGN KEY (balance_id,asset_id) REFERENCES refund_balances(id,asset_id),
FOREIGN KEY (balance_id,asset_id,order_id) REFERENCES refund_balances(id,asset_id,order_id)
```

## refund_reservations

저장 책임: refund. migration: 003. 작업: SHOP-04.

```sql
id app_id PRIMARY KEY,
refund_id app_id NOT NULL UNIQUE REFERENCES refunds(id),
amount uint256_amount NOT NULL CHECK (amount > 0),
state text NOT NULL CHECK (state IN ('active','consumed','released')),
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## settlements

저장 책임: settlement. migration: 003. 작업: SHOP-05.

```sql
id app_id PRIMARY KEY,
store_id app_id NOT NULL REFERENCES stores(id),
period_start timestamptz NOT NULL,
period_end timestamptz NOT NULL,
asset_id app_id NOT NULL REFERENCES assets(id),
state text NOT NULL CHECK (state IN ('open','closed','corrected')),
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
CHECK (period_start < period_end),
UNIQUE (store_id,period_start,period_end,asset_id)
```

## settlement_revisions

저장 책임: settlement. migration: 003. 작업: SHOP-05.

```sql
id app_id PRIMARY KEY,
settlement_id app_id NOT NULL REFERENCES settlements(id),
version bigint NOT NULL CHECK (version >= 0),
totals_snapshot jsonb NOT NULL,
source_revision bigint NOT NULL,
reason text NOT NULL,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (settlement_id,version)
```

## benefit_entries

저장 책임: benefits. migration: 003. 작업: STAMP-01.

```sql
id app_id PRIMARY KEY,
account_id app_id NOT NULL REFERENCES accounts(id),
store_id app_id NOT NULL REFERENCES stores(id),
allocation_id app_id REFERENCES payment_allocations(id),
rule_version text NOT NULL,
kind text NOT NULL CHECK (kind IN ('grant','consume','correction')),
source_key text NOT NULL,
units bigint NOT NULL CHECK (units <> 0),
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (account_id,store_id,rule_version,kind,source_key)
```

## benefit_redemptions

저장 책임: benefits. migration: 003. 작업: STAMP-01.

```sql
id app_id PRIMARY KEY,
grant_entry_id app_id NOT NULL REFERENCES benefit_entries(id),
order_id app_id NOT NULL REFERENCES orders(id),
state text NOT NULL CHECK (state IN ('reserved','used','released')),
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

인덱스:

```sql
CREATE UNIQUE INDEX one_live_redemption ON benefit_redemptions(grant_entry_id) WHERE state IN ('reserved','used');
```

## return_checks

저장 책임: rental. migration: 004. 작업: STAMP-04.

```sql
id app_id PRIMARY KEY,
rental_id app_id NOT NULL REFERENCES rentals(id),
binding_id app_id NOT NULL REFERENCES device_bindings(id),
check_version bigint NOT NULL CHECK (check_version >= 0),
wallet_origin text NOT NULL CHECK (wallet_origin IN ('new_travel','imported')),
policy_id text NOT NULL,
checks jsonb NOT NULL,
state text NOT NULL CHECK (state IN ('pending','eligible_for_reset','reset_confirmed','completed')),
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (rental_id,check_version),
FOREIGN KEY (binding_id,rental_id,wallet_origin) REFERENCES device_bindings(id,rental_id,wallet_origin)
```

## reset_clearances

저장 책임: rental. migration: 004. 작업: STAMP-04.

```sql
id app_id PRIMARY KEY,
return_check_id app_id NOT NULL REFERENCES return_checks(id),
challenge_hash text NOT NULL UNIQUE,
expires_at timestamptz NOT NULL,
owner_approval_ref text,
consumed_at timestamptz,
reset_evidence_ref text,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
CHECK (consumed_at IS NULL OR (owner_approval_ref IS NOT NULL AND reset_evidence_ref IS NOT NULL))
```

## firmware_releases

저장 책임: release. migration: 004. 작업: OTA-02, OTA-04.

```sql
id app_id PRIMARY KEY,
model text NOT NULL,
firmware_version text NOT NULL,
package_hash hash32 NOT NULL,
package_ref text NOT NULL,
manifest jsonb NOT NULL,
compatibility_profile app_id NOT NULL REFERENCES protocol_profiles(id),
state text NOT NULL CHECK (state IN ('draft','published','withdrawn')),
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (model,firmware_version)
```

## device_jobs

저장 책임: device. migration: 004. 작업: OTA-03, HW-07.

```sql
id app_id PRIMARY KEY,
device_id app_id NOT NULL REFERENCES devices(id),
release_id app_id REFERENCES firmware_releases(id),
kind text NOT NULL CHECK (kind IN ('fota','reset','configuration')),
state text NOT NULL,
progress_ref text,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## consents

저장 책임: privacy. migration: 005. 작업: TRIP-04.

```sql
id app_id PRIMARY KEY,
account_id app_id NOT NULL REFERENCES accounts(id),
purpose text NOT NULL,
version bigint NOT NULL CHECK (version >= 0),
enabled boolean NOT NULL,
superseded_at timestamptz,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (account_id,purpose,version)
```

인덱스:

```sql
CREATE UNIQUE INDEX current_consent ON consents(account_id,purpose) WHERE superseded_at IS NULL;
```

## recordings

저장 책임: recording. migration: 005. 작업: REC-02, REC-03.

```sql
id app_id PRIMARY KEY,
account_id app_id NOT NULL REFERENCES accounts(id),
format_profile app_id NOT NULL REFERENCES protocol_profiles(id),
duration_seconds numeric NOT NULL CHECK (duration_seconds >= 0),
completeness text NOT NULL CHECK (completeness IN ('complete','partial','failed')),
cloud_object_ref text,
access_blocked_at timestamptz,
deleted_at timestamptz,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## processing_jobs

저장 책임: processing. migration: 005. 작업: REC-03, AI-02.

```sql
id app_id PRIMARY KEY,
account_id app_id NOT NULL REFERENCES accounts(id),
recording_id app_id REFERENCES recordings(id),
operation_id app_id NOT NULL REFERENCES operations(id),
consent_id app_id NOT NULL REFERENCES consents(id),
kind text NOT NULL,
state text NOT NULL CHECK (state IN ('queued','running','succeeded','failed','cancelled')),
result_ref text,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## privacy_requests

저장 책임: privacy. migration: 005. 작업: TRIP-04, AUTH-04.

```sql
id app_id PRIMARY KEY,
account_id app_id NOT NULL REFERENCES accounts(id),
kind text NOT NULL CHECK (kind IN ('export','delete','account_deletion')),
scope jsonb NOT NULL,
state text NOT NULL,
operation_id app_id NOT NULL REFERENCES operations(id),
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## data_lineage

저장 책임: privacy. migration: 005. 작업: TRIP-04.

```sql
id app_id PRIMARY KEY,
owner_id app_id NOT NULL REFERENCES accounts(id),
source_type text NOT NULL,
source_id app_id NOT NULL,
derived_type text NOT NULL,
derived_id app_id NOT NULL,
source_revision bigint NOT NULL CHECK (source_revision >= 0),
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (source_type,source_id,derived_type,derived_id,source_revision)
```

## credential_metadata

저장 책임: credentials. migration: 006. 작업: DID-02, DID-03.

```sql
id app_id PRIMARY KEY,
account_id app_id NOT NULL REFERENCES accounts(id),
issuer_ref text NOT NULL,
subject_ref text NOT NULL,
profile_id app_id NOT NULL REFERENCES protocol_profiles(id),
private_content_ref text NOT NULL,
status text NOT NULL CHECK (status IN ('active','expired','revoked','unknown')),
expires_at timestamptz,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## credential_status_history

저장 책임: credentials. migration: 006. 작업: DID-02.

```sql
id app_id PRIMARY KEY,
credential_id app_id NOT NULL REFERENCES credential_metadata(id),
status_version bigint NOT NULL CHECK (status_version >= 0),
status text NOT NULL,
evidence_ref text NOT NULL,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (credential_id,status_version)
```

## paid_resource_requests

저장 책임: paid_resources. migration: 006. 작업: X402-02, X402-03.

```sql
id app_id PRIMARY KEY,
account_id app_id NOT NULL REFERENCES accounts(id),
wallet_id app_id NOT NULL REFERENCES wallets(id),
resource_ref text NOT NULL,
profile_id app_id NOT NULL REFERENCES protocol_profiles(id),
price_snapshot jsonb NOT NULL,
payment_state text NOT NULL,
delivery_state text NOT NULL CHECK (delivery_state IN ('awaiting_payment','processing','delivered','failed')),
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## entitlements

저장 책임: paid_resources. migration: 006. 작업: X402-02.

```sql
id app_id PRIMARY KEY,
request_id app_id NOT NULL UNIQUE REFERENCES paid_resource_requests(id),
payment_evidence_ref text NOT NULL UNIQUE,
resource_result_ref text,
state text NOT NULL,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## market_quotes

저장 책임: markets. migration: 006. 작업: DEX-02, FX-02.

```sql
id app_id PRIMARY KEY,
market_ref text NOT NULL,
profile_id app_id NOT NULL REFERENCES protocol_profiles(id),
action text NOT NULL,
quote_snapshot jsonb NOT NULL,
price_source_ref text NOT NULL,
expires_at timestamptz NOT NULL,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## chain_projections

저장 책임: indexer_projection. migration: 006. 작업: INDEX-04, PERP-05, STO-03.

```sql
id app_id PRIMARY KEY,
chain_id bigint NOT NULL CHECK (chain_id = 8283),
profile_id app_id NOT NULL REFERENCES protocol_profiles(id),
entity_type text NOT NULL,
entity_key text NOT NULL,
observed_block_hash hash32 NOT NULL,
observed_block_number numeric NOT NULL CHECK (observed_block_number >= 0 AND observed_block_number = trunc(observed_block_number)),
projection jsonb NOT NULL,
validity text NOT NULL,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (chain_id,profile_id,entity_type,entity_key)
```

## keeper_jobs

저장 책임: market_operations. migration: 006. 작업: PERP-06.

```sql
id app_id PRIMARY KEY,
market_ref text NOT NULL,
action text NOT NULL,
interval_key text NOT NULL,
operation_id app_id NOT NULL REFERENCES operations(id),
submission_id app_id REFERENCES transaction_submissions(id),
state text NOT NULL,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (market_ref,action,interval_key)
```

## places

저장 책임: places. migration: 007. 작업: TRIP-01.

```sql
id app_id PRIMARY KEY,
provider text NOT NULL,
provider_place_id text NOT NULL,
name text NOT NULL,
latitude numeric CHECK (latitude BETWEEN -90 AND 90),
longitude numeric CHECK (longitude BETWEEN -180 AND 180),
source_observed_at timestamptz NOT NULL,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (provider,provider_place_id)
```

## store_place_links

저장 책임: places. migration: 007. 작업: TRIP-04.

```sql
id app_id PRIMARY KEY,
store_id app_id NOT NULL REFERENCES stores(id),
place_id app_id NOT NULL REFERENCES places(id),
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (store_id,place_id)
```

## reviews

저장 책임: reviews. migration: 007. 작업: TRIP-02.

```sql
id app_id PRIMARY KEY,
account_id app_id NOT NULL REFERENCES accounts(id),
place_id app_id NOT NULL REFERENCES places(id),
allocation_id app_id REFERENCES payment_allocations(id),
rating smallint NOT NULL CHECK (rating BETWEEN 1 AND 5),
private_text_ref text NOT NULL,
source_type text NOT NULL,
visibility text NOT NULL,
deleted_at timestamptz,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## trips

저장 책임: travel. migration: 007. 작업: TRIP-03.

```sql
id app_id PRIMARY KEY,
account_id app_id NOT NULL REFERENCES accounts(id),
region_ref text NOT NULL,
locale text NOT NULL,
timezone text NOT NULL,
state text NOT NULL,
deleted_at timestamptz,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## travel_evidence

저장 책임: travel. migration: 007. 작업: TRIP-04.

```sql
id app_id PRIMARY KEY,
account_id app_id NOT NULL REFERENCES accounts(id),
trip_id app_id NOT NULL REFERENCES trips(id),
place_id app_id NOT NULL REFERENCES places(id),
source_type text NOT NULL CHECK (source_type IN ('location_visit','testnet_payment','commercial_purchase','user_report')),
source_ref text NOT NULL,
validity text NOT NULL CHECK (validity IN ('pending','valid','invalidated','unknown')),
consent_id app_id REFERENCES consents(id),
observed_at timestamptz NOT NULL,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## itineraries

저장 책임: travel_ai. migration: 007. 작업: AI-02.

```sql
id app_id PRIMARY KEY,
account_id app_id NOT NULL REFERENCES accounts(id),
trip_id app_id NOT NULL REFERENCES trips(id),
constraints_snapshot jsonb NOT NULL,
validated_stops jsonb NOT NULL,
validation_state text NOT NULL,
consent_id app_id NOT NULL REFERENCES consents(id),
deleted_at timestamptz,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
```

## participations

저장 책임: challenges. migration: 007. 작업: AI-03.

```sql
id app_id PRIMARY KEY,
account_id app_id NOT NULL REFERENCES accounts(id),
trip_id app_id NOT NULL REFERENCES trips(id),
challenge_ref text NOT NULL,
state text NOT NULL,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (account_id,trip_id,challenge_ref)
```

## challenge_entries

저장 책임: challenges. migration: 007. 작업: AI-03.

```sql
id app_id PRIMARY KEY,
participation_id app_id NOT NULL REFERENCES participations(id),
step_ref text NOT NULL,
evidence_id app_id NOT NULL REFERENCES travel_evidence(id),
evidence_revision bigint NOT NULL CHECK (evidence_revision >= 0),
state text NOT NULL,
reward_key text UNIQUE,
created_at timestamptz NOT NULL DEFAULT now(),
revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
UNIQUE (participation_id,step_ref,evidence_id)
```


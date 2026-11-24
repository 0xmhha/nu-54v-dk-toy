# 10 Payment, finance and fulfillment handoff

[Ledger relationship and invariants](../../docs/content/commercial/architecture/ledger.md) · [P06 v2 signatures](../p06-stablenet-contracts/settlement-v2.md) · [P07 scoped evidence](../p07-indexer/chainview-v2.md)

## Interfaces and responsibilities

- `POST /v2/commerce` accepts the existing authenticated C3 `payment.forward` command and a detached `approvalSignature` field with 65 RSV bytes in hex. The request binds the stored quote digest, payer approval digest, order, request ID, expiry and idempotency key. The handler reopens the original encrypted issuance bytes, verifies both signers and persists the approval once. New approvals require a live quote. A retry returns the recorded result without altering the original bytes.
- The P05 relay worker resolves the configured deployment and gas account. It reserves one chain/account nonce, constructs only the prescribed SettlementV2 call and asks the isolated relay signer to sign. It encrypts and commits the exact signed raw transaction and its EVM hash before broadcasting. A lost response becomes unknown. Recovery checks the recorded hash and may retransmit those same bytes. The first price is twice the suggested one, up to `P05_RELAY_MAX_GAS_PRICE`. When the node later asks for more than the signed price, the worker signs the same nonce and call again at least an eighth higher, stores it and sends it; every stored version stays on record and the one the chain includes becomes the attempt. A nonce is reused only after the stored transaction can no longer act: its authorization expired and the order is decided, or it expired more than five minutes ago, and the chain has not used the nonce. A revert is an attempt outcome. It does not close the order. P07 verified absence at a post-expiry finalized anchor can close the order and release held stock.
- The reconciliation worker connects to P07 with the scoped SQL reader. In a P07 transaction it takes the shared stream incident lock, checks the active finalized view and matches chain, ABI, deployment, merchant/order, asset, exact amount, payer, beneficiary, nonce, authority generation, escrow mode, quote digest and inclusion interval. The P07 stream lock stays held through the short P05 commit. A conflict, missing proof or a different fact produces no payment or fulfillment entry. Finality after expiry is accepted when inclusion met the signed interval.
- Confirmation locks the sale. Payment, one balanced debit/credit journal record, branch/day pickup sequence, sale state and outbox event commit together. Event and transaction/log identities are unique and retry-safe. Each refund journal reverses the appropriate payment accounts; gas is outside the sales/refunds totals.
- Refund request and approval are independent branch roles tied to separately authenticated actors. The locked payment tracks reserved plus in-flight plus finalized refund amounts below principal. Identical request keys return the existing refund. The request cannot supply a new payout address. Refund routing is derived from the matched original payer, escrow generation and mode. The worker verifies the merchant's on-chain reserve, obtains the separately enrolled refund authorization and records the exact signed transaction before submission. A refund finalizes only from the scoped original-order/refund evidence. Failed transport leaves the reservation intact until authoritative reconciliation.
- The fulfillment dispatcher sends one stable event reference and effect key per payment/refund. Its JSON frame carries `eventId`, `branchId`, `saleId`, `kind`, `sourceRef`, `revision`, `effectKey`, optional `pickupNo` and `receiptRef`. The kiosk/consumer inbox records the same effect key and payload digest before acknowledging. After dispatch interruption P05 queries by the same event ID. An unknown result leaves the delivery on hold. Only an authoritative absence or a committed inbox result permits a same-key retry or completion. The output does not contain wallet or raw transaction data.

## First acceptance scenarios

1. Submit a board-signed v2 approval through the actual authenticated opsd route. Lose the first relay response, restart and verify the second network delivery is byte-identical with one EVM hash. Confirm exactly once using P07 even if the chain finalizes after quote expiry.
2. Reject a forged approval, altered signed quote, mismatched evidence or a frozen stream. Confirm the ledger posts two balanced sides, one pickup and one event at most once.
3. Race reserve requests for 70 and 50 against principal 100. At most one wins. Verify same-actor approval rejection, reserve exhaustion, wrong route/amount rejection and a partial refund confirmed after a late finality observation.
4. Crash the dispatcher after downstream acceptance. Recovery queries the same event; an unknown downstream state performs no new effect. Verify tenant/branch separation for each HTTP action.
5. Run migrations on fresh and populated v1 PostgreSQL fixtures and compare pre/post v1 data. Use only loopback Anvil for chain transactions.

## Runtime enrollment

Enable the worker and finance endpoints by setting `P05_SETTLEMENT_TARGETS`, a JSON map from branch UUID to its `Target` with stream UUID, contract, runtime code hash, asset, deployment manifest digest, refund signer and refund authorization version. The worker checks chain 8283 and each enrolled runtime code hash at startup. `P05_SETTLEMENT_RPC`, `P05_RELAY_ADDRESS` and `P05_RELAY_MAX_GAS_PRICE` enroll the RPC, dedicated gas account and positive gas price cap. Set `P05_RELAY_SIGNER` and `P05_REFUND_SIGNER` to separately enrolled executables and supply their matching `_SHA256` values. The relay storage key file `P05_SETTLEMENT_STORAGE_KEY_FILE` uses a different key from both quote storage and commitment files.

### Merchant gas commitment for nearby review

Set `P05_MERCHANT_GAS_SPONSORS` to an absolute pathname for a JSON array of branch grants. This option requires verified deployment profiles, the existing `P05_SETTLEMENT_DEPLOYMENT_APPROVAL_KEY`, and `P05_PRESENTATION_KEY_FILE`. A gas account in the worker configuration alone supplies no commitment. Each grant explicitly assigns the network fee to the merchant relay for one branch, merchant, asset and deployed settlement contract. Its JSON fields are `branch`, `chain`, `contract`, `asset`, `manifest`, `merchant`, `relay`, `startsAt`, `expiresAt` and `signature`. Times are integer Unix seconds and the end is exclusive.

The existing deployment approval authority signs the byte string `NU54-merchant-gas-commitment-v1` followed by a zero byte, the raw 16-byte branch UUID, 8-byte big-endian chain ID (8283), the contract and token addresses (20 bytes each), manifest digest (32 bytes), merchant identifier (32 bytes), relay address (20 bytes), and start and end times as 8-byte big-endian unsigned integers. Publish the 64-byte Ed25519 signature in hex. The deployment authority signs only after approving the merchant's network-fee commitment. The worker enrollment must name the same relay account. Publish grants with atomic rename. An empty array revokes all commitments and removing one entry revokes that branch. A missing or invalid publication provides no gas sponsorship proof.

A live C3 quote read checks the current deployment code and pinned deployment anchor before returning the gas proof. P05 signs the grant digest, relay account and proof expiry for the exact original payer-bound presentation and signed quote bytes. The addition leaves prior quote and presentation signatures unchanged. The nearby terminal carries the proof inside the existing C2 quote reply, after authenticating the original customer or checkout authority. It exposes no additional account, location or contact data about the customer.

The displayed sponsorship proof lasts at most 30 seconds and never beyond its grant or quote expiry. A disconnected phone can verify an unexpired signed proof, with no claim about revocations that happened after it was issued. Opening the board prompt and explicitly forwarding an approval each require rechecking the same original signed quote and its current gas terms through the terminal. The button session is bounded by the refreshed proof's expiry. A stopped grant or changed proof terms cannot silently reuse an old approval. P05 reads the publication again after the runtime check. Publication replacement and PostgreSQL commit are separate systems. A first submission racing replacement is fenced by the last successful publication read and its exclusive term deadline at SQL admission. P05 pins the first active gas grant digest on the live original quote. A different active grant for the same branch cannot take its place. P05 checks the pinned digest and the current live term again before admitting a new terminal submission. The SQL transaction distinguishes new admission from the original accepted key. After revocation that key still retrieves its prior acceptance. Existing accepted submission and relay effects keep their original identities and are reconciled through normal status reads. Revocation does not cancel accepted or broadcast transactions. A customer without Internet can ask the authenticated P04 terminal to make both reads using its own P05 connection. A terminal that cannot contact P05 cannot confirm current sponsorship or admit a new submission. The gas grant digest follows the quote work row's existing retention rule. The actual chain sender and gas spent remain settlement evidence after submission.

Configure `P05_SETTLEMENT_DATABASE_URL`, `P05_FINANCE_DATABASE_URL` and `P05_P07_RECONCILIATION_DATABASE_URL` with their respective scoped login roles. Pass `P05_FINANCE_GRANTS` as a JSON map of trusted mTLS certificate fingerprints to expiring finance principals. Per-branch capabilities are separately checked in P05 by SQL. The finance endpoints are `POST /v2/finance/refund-reserve`, `POST /v2/finance/refund-approve` `POST /v2/finance/refund-submit` and `POST /v2/finance/day-close`. New staff refunds require a classification and a separate treasury submitter regardless of whether the server is configured with an operator origin. The reason-free historical format is limited to an exact existing request. Approval and treasury submission bind the refund review revision.

Optional fulfillment uses `P05_FULFILLMENT_TARGETS` (branch UUID to HTTPS base URL), `P05_DISPATCH_DATABASE_URL`, and the `P05_DISPATCH_CA`, `P05_DISPATCH_CERT` and `P05_DISPATCH_KEY` files. The dispatcher holds the scoped P07 stream fence only for short preparation and acknowledgement transactions. The receiver separately gates durable acceptance. A queued delivery stays held when the stream has an unresolved finalized conflict. After recovery, the interrupted attempt uses a lookup because it may already have reached the consumer. No P07 identity fields are added to the fulfillment payload. See the [fulfillment receiver and printer operation](fulfillment-v2.md).

Day close uses the location's timezone and local cutoff on the recorded chain inclusion time. Gross groups by original payment day and refunds by their own inclusion day. The immutable closing revision stores an operator evidence reference and a SHA-256 digest of the sorted posting IDs, evidence IDs, order/refund IDs and exact units used to calculate that revision. The beneficiary receipt and separate on-chain refund reserve are distinct movements. Test gas costs stay outside those asset totals.

Explicit settlement or dispatcher configuration requires commerce enrollment. Incomplete enrollment stops startup before the listener opens. With all finance and fulfillment settings absent, the legacy probe-only mode remains available.

The local integration exercises payment and refund using the production handlers and worker on loopback mTLS. It also launches the built opsd binary, verifies that incomplete settings exit and legacy probe-only mode starts, and closes the same day through the finance mTLS endpoint using the configured role and signer files. The fixture runs the P07 indexer as a separate process and uses a disposable PostgreSQL database with the owner migrations and restricted runtime logins. Run `test/settlement/anvil.sh` from this product with `P05_SETTLEMENT_TEST_DATABASE_URL` configured for a disposable `nu54_settlement_*` database and pinned Foundry 1.5.1 on the path.

### Order escrow branches

A branch pays either through SettlementV2 or through OrderEscrow ([wire agreement](../../docs/content/commercial/architecture/spending-wallet-wire.md)). The optional `abiSha256` field selects the contract. It appears in the branch entry of `P05_COMMERCE_PROFILES` and of `P05_SETTLEMENT_TARGETS`, and both must name the same contract or `opsd` does not start. An empty field means SettlementV2, so existing files keep their meaning. The escrow value is the `OrderEscrow` entry of `packages/contracts-abi/abi/manifest.json`. A signed deployment profile for an escrow branch uses `version` 3 and the same ABI hash.

```json
// P05_COMMERCE_PROFILES
{"<branch uuid>": {"chainId": 8283, "contract": "0x<OrderEscrow>", "asset": "0x<TestUSDC3009>", "beneficiary": "0x<payout>",
  "signer": "0x<quote key>", "merchantId": "<64 hex>", "keyVersion": 1, "beneficiaryVersion": 1,
  "abiSha256": "e326762a55880c673cec09249bab8a76725183e812f3b3709dec2b92bd936c29"}}
// P05_SETTLEMENT_TARGETS (refundSigner and refundKeyVersion are not used by an escrow branch)
{"<branch uuid>": {"streamId": "<uuid>", "manifestSha256": "<64 hex>", "contract": "0x<OrderEscrow>", "codeHash": "0x<64 hex>",
  "asset": "0x<TestUSDC3009>", "abiSha256": "e326762a55880c673cec09249bab8a76725183e812f3b3709dec2b92bd936c29"}}
```

- **Before switching a branch**: register the merchant with `OrderEscrow.configureMerchant` and call the token's `setTrustedSender(escrow, true)`.
- **Quote lifetime**: `ledger.issuance_profile.quote_lifetime_seconds` already allows at most 120 seconds. The board refuses an order that expires more than 180 seconds after its own earliest possible time, so the board may lag this server by up to 60 seconds.
- **Payment**: `order.lock` is accepted from the terminal or the paying customer. The 65-byte board signature and the phone key are sealed together in `ledger.settlement_submission`. The relay worker signs, stores and sends `OrderEscrow.lock` with the same nonce, lease and recovery rules as `settle`. It first asks the node whether the lock would run, and leaves it unsent if not. `payment.forward` on an escrow branch and `order.lock` on a SettlementV2 branch answer `UNSUPPORTED_CAPABILITY`.
- **Orders that are not offered**: rented payers, POS, travel and subscription orders get `UNSUPPORTED_CAPABILITY` at `quote.issue`.
- **After the lock**: kitchen `collected` queues `markDelivered` and `unfulfillable` queues `cancel`, signed by the branch quote signer. `order.answer` queues `release`, `dispute` or `withdrawDispute` with the phone signature. The server itself queues `refundUndelivered` after `deliverBy` and `refundExpiredDispute` after `resolveBy`. Every one is a `ledger.settlement_work` job with an `action_id`, so it uses the same lease, nonce, sealing and recovery as a payment. These calls have no deadline, so the same bytes are sent until a finalized block contains them. If another transaction takes the nonce for good, the job signs again. A call that would revert is not sent. A late delivered mark, a late dispute, a late decision and a claim that cannot run for an hour end as `failed`; the rest are judged again.
- **Facts**: the worker reads `chainview.escrow_receipt` after each action and once a minute (five minutes after delivery) for orders nobody here acted on, so a direct claim or a buyer's own call is booked too. A claim batch is settled from `chainview.escrow_transaction`.
- **Expired orders**: for an issued or checking order whose quote expired without a payment fact, on either contract, the worker calls `chainview.request_absence` (P07 migration 019) and closes the order when the proof appears. Without that migration the worker runs as before and the operator's `prove-absence` command gives the proof. The same lock sent over the kiosk and over the customer's phone returns the same order; a different lock conflicts.
- **Accounts**: a lock posts `order_escrow` / `escrow_deposits`. `released` ends the deposit and posts `settlement_clearing` / `sales` on the day it was confirmed on chain; `cancelled` only ends the deposit. Gross counts releases, refunds never count returned deposits. A day with escrow postings also reports `escrowLocked` and `escrowReturned`, and its CSV gains two columns. Day close compares the released, locked and returned totals and the digest with `chainview.finance_day_evidence`. The SettlementV2 refund path and payout rotation refuse escrow orders.
- **Disputes**: set `P05_ESCROW_RESOLVER`, `P05_ESCROW_RESOLVER_SIGNER` and `P05_ESCROW_RESOLVER_SIGNER_SHA256`. The account must be the contract's arbiter, must not be the relay account and needs its own gas. Without these settings the decision operations answer 404 and undecided disputes return to the buyer. Grants `escrow_dispute_request`, `escrow_dispute_approve` and `escrow_claim` live in `ledger.finance_grant`; the proposer can neither approve nor reject.
- **Switching contracts**: `opsd` does not start while a branch has an open order, an open refund or held money of another payment contract. After the switch the refund console offers nothing for the old orders.
- **The customer's account**: `payer.enroll` records the board account after its proof for this tenant, branch, customer and request. The newest proof of an account ends other customers' enrollment of it in the same tenant, unless one of them still has something open on it. It lasts as long as the customer's branch access, at most 30 days, and can be renewed. It cannot change while the old account has an open order or return. `wallet.return` queues the board's signed `transferWithAuthorization`, sent by the relay account with the store's gas commitment for that payer: one at a time per account, three a day per customer, valid for at most ten minutes, and never beside an open order. While it is queued no quote is issued for the account. It is `final` when a finalized block contains the transfer and `failed` when a finalized block after `validBefore` shows the authorization unused. The transfer is searched for in the hour before `validBefore`, in log ranges the node accepts, however late the worker looks. If the authorization is used and no log shows its transaction an hour after the deadline, the account is freed and the status keeps its last `queued` or `submitted` value, because `final` needs a transaction hash; the worker reports it.

```sh
export P05_ESCROW_RESOLVER=0x<arbiter> P05_ESCROW_RESOLVER_SIGNER=/opt/nu54/resolver-signer P05_ESCROW_RESOLVER_SIGNER_SHA256=<64 hex>
```

```json
// POST /v2/finance/escrow-claim
{"branchId": "<uuid>", "idempotencyKey": "<uuid>", "saleIds": ["<uuid>", "<uuid>"]}
// POST /v2/finance/escrow-resolve, then another person: POST /v2/finance/escrow-resolve-approve
{"branchId": "<uuid>", "idempotencyKey": "<uuid>", "saleId": "<uuid>", "outcome": "refund-buyer", "reason": "not_delivered"}
{"branchId": "<uuid>", "idempotencyKey": "<uuid>", "decisionId": "<uuid>"}
```

`TestLoopbackOrderEscrow` runs the whole path on Anvil with the real indexer. It needs a database with the P05 and P07 migrations.

```sh
P05_SETTLEMENT_TEST_DATABASE_URL="host=<socket dir> dbname=nu54_settlement_escrow" \
  sh test/settlement/anvil.sh -run TestLoopbackOrderEscrow -v
```

### Transfers to any address

Optional. `walletSendsPerDay` (1 to 100) in a branch's target lets its enrolled customers send the store's token to any address ([wire agreement](../../docs/content/commercial/architecture/spending-wallet-wire.md#11-다른-주소로-보내기)). The board signs an EIP-3009 authorization, the relay account sends it as it sends a return (through `gasSponsor` when there is one) and the store pays the gas. Without the setting `wallet.send` and `wallet.send-status` answer `UNSUPPORTED_CAPABILITY`. The number counts every accepted transfer of an account or customer in 24 hours, also failed ones. Migration 112 keeps them in `ledger.wallet_send`.

### Test-token faucet

Optional. With none of the settings no store has a faucet and `faucet.claim` and `faucet.status` answer `UNSUPPORTED_CAPABILITY` ([wire agreement](../../docs/content/commercial/architecture/spending-wallet-wire.md#9-시험용-토큰-받기)).

- `P05_FAUCET_ADDRESS`, `P05_FAUCET_SIGNER`, `P05_FAUCET_SIGNER_SHA256`: a dedicated account and its enrolled signer executable, as for the relay account. The account holds pre-minted tokens and its own gas money. `opsd` does not start if it is the relay, dispute or refund account or owns a listed token.
- `P05_FAUCET_TOKENS`: path of a JSON map from branch UUID to its tokens. Every field is required; amounts are base units. `perDay` bounds the claims of one account and of one customer, `dailyTotal` all claims of a token, each in any 24 hours and over every branch. A token has the same `dailyTotal` everywhere. Symbol and decimals must match the chain.

```json
{"<branch uuid>": [{"address": "0x<token>", "symbol": "tUSDC", "decimals": 6, "amount": "<base units>", "perDay": 1, "dailyTotal": "<base units>"}]}
```

A claim is an `escrow_action` of kind `faucet` (migration 110), so it has the relay worker's lease, nonce, sealed bytes and replacement rules, and goes to the token directly, not through GasSponsor. A transfer has no deadline of its own, so it is signed once: if the chain uses its nonce for another transaction, the claim waits an hour for the node to show it and then ends as `unknown` without a second signature. An unsigned claim ends as `failed` after an hour or when its token is withdrawn, and gives its place in the limits back. Do not change `P05_FAUCET_ADDRESS` while a signed claim is open; the worker reports it and waits.

`TestLoopbackOrderEscrow/TestTokenFaucet` covers the empty account, the limits, a lost reply with a restart and a spent nonce on Anvil.

### Original quote read for the expiry investigation

Migration 044 grants the existing finance role a read of the original encrypted quote scoped by tenant, branch, finance actor, sale and `reconcile_read`. The P05 issuer projection revalidates the stored signature and exact signed order and quote identifiers, then returns the public digest, signed time limits and deployment identity to an enrolled explorer over mutual TLS. The operator's finance credential and live branch grant authorize the read. The endpoint adds no ledger data and cannot release a reservation or produce chain evidence. Setup and verification are documented in the [protected explorer guide](../p07-indexer/chainview-v2.md#x06-original-quote-and-contiguous-coverage).


### O06 환불 대기열

`POST /v2/finance/refund-queue`는 기존 정산 인증서와 현재 해당 지점의
환불 신청, 승인 또는 제출 중 하나 이상의 권한을 요구한다. 지점과 선택
커서로 환불 ID 순의 최대 50건을 조회한다. 결과는 원주문·환불 참조,
자산·금액·원금, 장부 상태와 현재 계정의 업무 인계 사유만 제공한다.
목적지·고객·직원 식별자는 반환하지 않는다. 대기열에서 같은 원주문을
열면 기존 심사 화면이 원금, 경로와 권한을 다시 조회하며 실제 결정은
기존 독립 승인·재무·심사 회차 정책을 적용한다.

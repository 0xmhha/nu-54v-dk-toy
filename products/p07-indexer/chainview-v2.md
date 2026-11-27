# 09 Chain reader acceptance notes

[Ledger chainview design](../../docs/content/commercial/architecture/ledger.md) · [Settlement v2 ABI](../../packages/contracts-abi/abi/SettlementV2.json) · [Local 8283 receipt vectors](../../packages/contracts-abi/abi/vectors/settlement-v2.json)

Bind each stream to an approved chain, deployment address, ABI digest and manifest. Preserve v1 `PaymentSettled` and the existing receipt/cursor API while ingesting v2 `Paid` and `Refunded` on the corresponding streams. Decode by the explicit profile. Preserve opaque v2 merchant/order/refund IDs and exact minor-unit values. Include beneficiary, payer, quote digest, payer generation and nonce for v2 payment reconciliation. A successful token transfer log alone does not establish settlement.

| Negative input | Expected result |
|---|---|
| V2 topic from another address or chain, v1 topic at v2 profile, unapproved ABI | Reject the log before writing a commitment |
| Reverted transaction or receipt whose block hash contradicts the retained header | No accepted observation or evidence |
| Same event replay in one generation | Idempotent on generation, block, transaction and log index |
| Same transaction hash on two forks | Store observations with the correct block identity, project only the active canonical branch |
| Missing header, parent mismatch or stale lease fence | Keep the prior complete cursor and no partial event rows |
| Reorg above finalized checkpoint | Find the common ancestor, reconcile headers/events/cursor atomically, and invalidate noncanonical candidates |
| Contradictory finalized block or confirmation regression | Freeze the affected generation, record an incident and withhold active evidence pending governed review |
| Ready but inactive replay generation or stale expected read revision | No projected evidence or unauthorized pointer change |
| Unauthorized service login requests another merchant, bulk receipts or raw restricted rows | Database and API refuse the request. An arbitrary order ID or caller-supplied scope is never authority |
| Included just before expiry then finalized later | Preserve inclusion time and policy reference so P05 can confirm an on-time payment |
| Payer is an escrow account, P07 receives an unrelated direct refund | Decode mode and original order from the v2 event, and withhold inconsistent evidence |

Use one transaction for the block, accepted events and cursor/fence advancement. A reader resolves its authorization from the authenticated principal and deployment scope. DB row filtering uses authenticated grants, not caller-supplied session settings. The migration's broad `nu54_p07_read` capability is appropriate for a service authorized for the complete domain. Merchant and customer access require a narrower owner API/projection, with deny-by-default cross-scope SQL and HTTP tests.

Run v1 and v2 listeners over the same controlled block interval. Reconcile counts, summed exact amounts, canonical checkpoints and final hashes with the configured chain. Change projection only after comparison passes and the requested revision is still current. Continue using the existing receipt and cursor state for v1 during the transition.

## Running the indexer

Apply owner migrations through 019 using the existing P07 migration target. Configure separate database logins for ingest, switching, public queries and merchant-scoped reconciliation. The default `indexer` invocation still opens the v1 receipt/cursor service.

```sh
P07_CHAINVIEW_DATABASE_URL='service=indexer-ingest' \
P07_CHAINVIEW_PROFILES=/path/to/approved-profiles.json P07_RPC=http://127.0.0.1:8545 \
  indexer chainview

P07_PUBLIC_DATABASE_URL='service=indexer-public' indexer public-receipts
```

The profile file is an array of locally approved deployment entries. Each entry binds `streamId`, `generationId`, `version`, `chainId`, `contract`, `asset`, `abiSha256`, `manifestSha256`, `codeHash`, `startHeight` and `startHash`. The hashes and addresses come from the signed deployment record. The RPC must expose canonical block-hash state lookups and a finalized header. The reader verifies chain ID 8283 and deployed code identity before recording a stream. It verifies transaction and receipt roots against the anchored headers. Individual receipt lookups support RPCs without block-wide receipts. Blocks and observations commit in batches fenced by a generation lease.

A replay can run against the same stream with another generation ID. Use the separately authenticated `nu54_p07_switch` connection to call `chainview.record_comparison(stream_id, candidate_id, expected_revision)`. Store the returned ID as the switch request's `compare_ref` and use `chainview.apply_switch(request_id)` to atomically advance the read pointer. Initial publication uses revision 0 and requires continuous coverage from deployment. Later switches also require matching finalized checkpoints and identical canonical event fields. A confirmed conflict in any generation holds all active evidence for the stream and queues a durable row in `chainview.pending_incident`. A `nu54_p07_incident` notification consumer acknowledges delivery with `chainview.acknowledge_incident(id)`. Delivery acknowledgement never releases the hold.

Public `GET /v2/receipts?stream=<uuid>&merchant=<merchant-key>&order=<chain-order>` uses the projection-only login. It exposes exact units and canonical inclusion time. Payer, recipient, signer nonce, authority generation, mode and quote digest are available to the restricted role through `chainview.reconciliation_receipt(stream_id, merchant, order)`. The role must hold an unexpired `chainview.receipt_scope` entry bound to its authenticated SQL session login, stream and merchant. The restricted result also carries deployment, ABI, policy, inclusion block/hash and finalized anchor. The P05 consumer authenticates the quote/merchant/order and matches exact payment values before producing ledger effects. The existing mutually authenticated C7 endpoint serves its narrower proof-ID contract.

The owner-only command `indexer prove-absence <generation> <merchant> <order> <signed-expiry-unix-seconds>` uses expiry from the issuer's stored signed quote. It requires continuous canonical coverage from deployment, a finalized block at or after expiry, no successful payment in the covered observations, and unconsumed state from an EIP-1898 block-hash call. It checks the anchor around the RPC call and binds the observation to the still-active generation in the proof transaction. Authorized consumers can read the unexpired result through `chainview.reconciliation_absence(stream_id, merchant, order)`. They need no operator for it: see [Requested absence proofs](#requested-absence-proofs). The state reference is SHA-256 over the canonical fields in [absence.go](internal/chainview/absence.go), including the original signed expiry, so owners can compare the proof to their stored quote. This reference attests the approved RPC observation. Ethereum storage proofs are outside this receipt format. P07 only produces observations and evidence. No P07 entry point submits a payment, refund or shipment.

For local verification, set `P07_CHAINVIEW_TEST_URL` to a disposable migrated PostgreSQL database and run `sh products/p07-indexer/test/chainview-anvil.sh` with Foundry 1.5.1 on `PATH`. The harness deploys both Solidity versions on loopback Anvil, mines their payments together, compares exact amounts and checkpoint hash, runs the real ingest command through a restricted login, and changes one fork below and one at the protected finalized boundary. The PostgreSQL tests also cover expiry, no-gap absence, projection privilege, governed read switching and incident redelivery.

## OrderEscrow profile

Profile version 3 pins the OrderEscrow ABI digest `e326762a…`. It runs through the same `indexer chainview` command, generation lease, receipt-root checks, finality, reorganization, hold, signed approval and replay as versions 1 and 2. Migration 017 adds the order columns to `observed_event`; there is no second event store. The operations server reads the confirmed facts with `chainview.escrow_receipt(stream_id, merchant, order)` as specified in the [shared wire contract](../../docs/content/commercial/architecture/spending-wallet-wire.md), under the existing `receipt_scope` grant and the `nu54_p07_reconcile` role. `reconciliation_ready` and `reconciliation_absence` are used unchanged.

| Event | Fact | Carries |
|---|---|---|
| `OrderLocked` | `locked` | payer, beneficiary, confirmer, amount, quote digest, `deliver_by`, `confirm_window` |
| `OrderDelivered` | `delivered` | `claimable_at` |
| `OrderDisputed` | `disputed` | `resolve_by` |
| `OrderReleased` | `released` | beneficiary, amount, `reason` (contract `Payout`) |
| `OrderCancelled` | `cancelled` | payer, amount, `reason` (contract `Refund`) |
| `ClaimSkipped` | `claim-skipped` | `reason` (contract `SkipReason`) |

The contract events do not carry the token, so the asset is the one in the profile. A batch may emit many facts in one transaction; the log index orders them. An event with a wrong topic count, a noncanonical word, an unknown reason or a zero identity stops the poll like a malformed settlement event. There are two exceptions, because anyone can cause them. A `ClaimSkipped` for an order that was never locked, including a zero merchant or order, is ignored, and one transaction keeps one skip for each order. A lock whose quote lies outside the SDK form (the contract does not check payer role and generation) keeps its event and only loses the quote window.

A lock does not have to be a direct call. When the transaction calls `lock` on the escrow itself, its quote calldata must hash to the event digest and the block time must be inside the signed window, which is stored. A lock sent through another contract keeps its event without the window. The operations server matches `quote_digest` and `included_at` to its stored quote in both cases.

`prove-absence` requires that no `locked` event is in the covered observations and that `orders(merchantId, orderId).state` is 0 at the finalized block, which is at or after the signed expiry. The proof reference keeps the same form as version 2.

Escrow facts are not in `active_evidence`, so the public, explorer, finance and C7 evidence readers do not see them. Those readers report nothing for an escrow stream; this is not evidence of nonpayment. Replay comparison covers the order columns, and its payment and refund totals count locks and cancellations.

`escrow_receipt` returns every other fact of the order and the 20 most recently confirmed skips, so a stranger's skipped claims do not slow or hide it. `chainview.escrow_transaction(stream_id, merchant, tx_hash)` returns, under the same grant, every fact of the merchant in one confirmed transaction with `order_ref` in front of the `escrow_receipt` columns. The server settles a batch claim from its own transaction there. An order it named that has neither `released` nor `claim-skipped` in that confirmed transaction was never locked. A poll stops after a block that brings it to 5000 events.

`chainview.finance_day_evidence` answers an escrow stream with its own object, under the same arguments, grant, hold and finality rules. A settlement stream keeps its earlier object. An escrow object has no `gross` or `refunds`, so it cannot pass as a settlement day.

| Field | Meaning |
|---|---|
| `anchor`, `height` | finalized block the proof stands on; its time is at or after the end of the day |
| `locked`, `lockedCount` | amount (minor-unit string) and number of orders locked in the day |
| `released`, `releasedCount` | paid to the merchant in the day: sales of that day |
| `cancelled`, `cancelledCount` | returned to the payer in the day |
| `count` | all facts above |
| `digest` | SHA-256 hex of the JSON array of `[kind, tx_hash, log_index, block_hash, amount]`, ordered by transaction, log and block as for a settlement day, with kinds `locked`, `released`, `cancelled` |

The day of a fact is the time of the block that contains it. An order locked on one day and released on the next is in both days.

Run the local fixture with `sh products/p07-indexer/test/chainview-anvil.sh -run '^TestAnvilOrderEscrowFacts$' -v` and `P07_CHAINVIEW_TEST_URL` set to a disposable database migrated through 019. It covers receipt, dispute, batch claim, cancellation, a lock removed by a reorganization and absence.

## Requested absence proofs

`chainview.request_absence(stream_id, merchant, order, signed_expiry)` asks for the proof that `prove-absence` makes, for settlement v2 and OrderEscrow streams. The `nu54_p07_reconcile` login needs a live `receipt_scope` row and a transaction that may write. It returns true when the request is waiting or the same proof is readable for at least two more minutes, so the server can call it on every pass until `reconciliation_absence` answers. It returns false when nothing was kept: no grant, another profile version, a malformed key, an expiry more than one hour ahead of both the wall clock and the finalized block, an order with a confirmed payment or lock, or 1000 requests of that merchant already waiting.

The running `indexer chainview` process works the requests with the same `ProveAbsence` code, so the proof and its reference are identical. A request becomes due when the active generation has a finalized block at or after the expiry. A payment or lock confirmed meanwhile drops it. A failure delays the request from 2 seconds up to 5 minutes, and its stream up to 1 minute. Requests leave when they are answered or after seven days. One worker serves all streams apart from ingest and rests at least as long as it worked. Each proof checks the whole stored chain twice (about 0.25 seconds per 200,000 blocks on a local machine), the second time under the lock that also guards a checkpoint advance.

## Browser surfaces and disclosure

With migrations through 014 applied, open the root of `indexer public-receipts` for deployment-scoped search and canonical receipts (X01 and X02). The v2 browser searches within one deployment and public merchant scope by order key or by full transaction hash with optional log index. Order lookup shows the original payment, finalized partial refunds, and net payment in raw minor units and, when verified against the token at the pinned deployment block, its decimal unit. The result includes a domain-separated SHA-256 value binding the chain, contract, transaction, log index, and recipient. The browser compares a customer's expected address from their own authorized original quote locally using Web Crypto. The address is neither searched remotely nor stored by the explorer. The digest compares the value to the observed event, and the public transaction hash and log index allow independent chain verification. The sample beneficiary in the design belongs to the synthetic design fixture. The production public projection does not disclose beneficiary, payer, internal branch or customer identifiers, pickup or shipment data. Beneficiary verification remains in the existing restricted reconciliation query. The indexed v1 deployment labels `PaymentSettled` as a merchant credit and distinguishes a later merchant cash-out transaction. Configure `P07_LEGACY_OPERATIONS_ORIGIN` with the trusted P05 HTTPS operator origin to open the authenticated v1 ledger screen without passing any branch, order or user identifier. The separate legacy `/receipts/{merchant}/{orderId}` response and its device/address display behavior retain their compatibility semantics. Incidents block canonical results and stale observations show their last successfully verified checkpoint. An empty public result supplies no evidence of nonpayment.

Run the protected X03/X04/X05/X06 browser independently. Enroll the SQL login only in `nu54_p07_explorer`, with the configured operator certificate's DER SHA-256 fingerprint granting selected stream UUIDs and, for expiry reconciliation, public merchant keys in those streams. Publish changes to the grants file by atomic rename. Both this server and C7 reopen their grant files for authorization and again before releasing database results. A missing or invalid publication fails closed. The operator service cannot switch a reader or issue proofs.

```sh
P07_EXPLORER_DATABASE_URL='service=indexer-explorer' \
P07_EXPLORER_CA=/path/to/operators-ca.pem \
P07_EXPLORER_CERT=/path/to/explorer.pem \
P07_EXPLORER_KEY=/path/to/explorer-key.pem \
P07_EXPLORER_GRANTS=/path/to/explorer-grants.json \
  indexer explorer-operations
```

Each fingerprint's grant has `expiresAt` (Unix seconds), `streams` (stream UUID array) and optional `merchants` (stream UUID to permitted public merchant-key array). X06 displays inclusion and attestation times or the existing unconsumed proof and coverage. For direct v2 settlement, X06 recomputes the quote digest from the canonical transaction calldata and compares it to the accepted event before presenting the signed issue/expiry window. Historical or indirect transactions without this binding require the issuer's original-order check. The P05 authorized evidence page independently verifies the digest against its archived signed quote and tests the same inclusive issue and exclusive expiry window. The browser makes no ledger, inventory or settlement decisions.

### X06 original quote and contiguous coverage

On the protected operations page, enter the P05 branch and original sale IDs together with the selected deployment, public merchant key, and public order key. The read uses the existing P07 certificate grant and a TLS-authenticated issuer projection. The issuer opens the encrypted quote from its original order, recomputes the digest, verifies the original merchant signature and checks its archived identifiers and time window. It checks the operator’s active P05 finance identity and branch `reconcile_read` grant before releasing the result. Unknown and out-of-scope originals receive the same response. The source accepts no submitted cutoff.

Set `P07_QUOTE_ORIGIN` to the P05 HTTPS origin, `P07_QUOTE_ORIGIN_CA` to the P05 server CA, and `P07_QUOTE_ORIGIN_CERT` and `P07_QUOTE_ORIGIN_KEY` to the dedicated explorer client identity. At P05 set `P05_QUOTE_BOUNDARY_READERS` to a JSON mapping of the enrolled reader’s DER SHA-256 certificate fingerprint to an object with `expiresAt` in Unix seconds. Use atomic rename for the reader and existing `P05_FINANCE_GRANTS` publications. The original operator uses the fingerprint enrolled in both operator grant files. The dedicated service certificate is only enrolled for this quote archive read.

The explorer compares the issuer's chain, asset, contract, ABI, runtime code hash and manifest with the active deployment, then rechecks the active revision and its own live operator grant. The last verified finalized block is shown with its height, hash and chain timestamp after checking continuity from the deployed start block. When that timestamp precedes the signed cutoff, the same quote remains pending. An equal or later timestamp by itself still leaves the order pending. The paid view requires the original digest and an inclusion time in the signed interval. The absent view requires an unexpired P07 unconsumed proof bound to the same original expiry, deployment, merchant, order and finalized anchor. No financial or inventory action runs from this screen.

Run `sh products/p05-operations-backoffice/test/settlement/anvil.sh -run '^TestQuoteBoundaryOriginalArchive$' -v` with a disposable PostgreSQL database migrated through P05 044 and P07 014. Set `P07_QUOTE_BOUNDARY_BROWSER_SCRIPT` to the absolute path of [test/quote-boundary-browser.cjs](test/quote-boundary-browser.cjs) to exercise the real protected browser with its mTLS certificate.

For browser verification of an actual dual-version Anvil fixture, set `P07_EXPLORER_BROWSER_SCRIPT` to the absolute path of [test/explorer-browser.cjs](test/explorer-browser.cjs) for the above PostgreSQL/Anvil harness. Use `P07_PLAYWRIGHT_MODULE` and `P07_CHROMIUM` to select a local Playwright installation and Chromium binary.

### X03 profile holds

A configured generation can be held before its first cursor exists. An ingest-only observation stores the stream and generation aliases, proposed deployment identity, reason and observation time. The operator monitor may read it in its existing mTLS stream scope. The public explorer and evidence reader continue to use the existing active target. An unregistered ABI or unverified signed deployment must neither create a deployment profile nor move a cursor. Other configured streams keep polling.

The approved entry binds the entire profile, including generation, chain, contract, asset, ABI and code hashes, manifest digest and initial block anchor. The reader also checks the deployed chain and bytecode. A held alias can clear only when the corresponding approved profile has passed these checks and been registered. Scope keys and status reasons are operational data. Do not persist raw signatures or the trust key in the hold.

### X05 isolated replay design

Each replay request identifies a stream and its durable job ID. The requested job records the active generation and expected read revision, the finalized anchor, the candidate generation and the comparison result. The replay writer uses the existing generation lease and pinned deployment profile. A restart attaches to the same candidate; duplicate requests cannot allocate competing candidate generations. The operator monitor reads these records through a stream grant. Request and promotion permissions are distinct.

Rebuild from the registered deployment anchor to the recorded finalized anchor. Compare complete prefix coverage, block hash, full event keys and payloads, and exact integer payment and refund totals. A mismatch records a diagnosable result without moving the active target. A verified comparison is passed to the existing revision-and-anchor-guarded P07 switch and P10 signed promotion authority. Public receipt/evidence readers continue to follow the existing active generation until that atomic publication as specified by the recovery plan.

For X03, enable signed ingest with `P07_CHAINVIEW_APPROVAL_KEY` set to the approved Ed25519 public key in hex and `P07_CHAINVIEW_SIGNATURES` pointing to a JSON object of generation UUID to detached signature in hex. Sign the domain bytes `NU54-chainview-profile-v1` followed by a zero byte and the compact Go `encoding/json.Marshal(Profile)` bytes for the profile as configured. Select this payload before discovering token decimals from the canonical RPC. The signature covers the manifest digest as one of the profile fields. Restart the producer after publishing updated profiles and signatures together. Legacy deployments without configured signature files retain their previous ingest behavior for previously unheld entries. Any held stream-generation alias requires a signed approval even in that compatibility mode.

For X05, keep the existing explorer login with only `nu54_p07_explorer` membership. Optional `P07_REPLAY_REQUEST_DATABASE_URL` uses a separate login enrolled only in `nu54_p07_replay_request`. Operators with a scoped `replay` grant can request a job in their existing stream allowlist. Read grants alone can inspect jobs and cannot start or publish them. Running jobs have no terminal timestamp, comparison or difference reason. Successful jobs carry a comparison ID, and failed jobs carry a reason. The same operation ID is idempotent. A matching result supplies the existing switch's comparison reference. If the active anchor or revision has advanced, start a new replay before promotion. An isolated divergent candidate never freezes the active generation. The browser never sends a promotion command.

Run the real browser fixture by exporting `P07_EXPLORER_BROWSER_SCRIPT` as the absolute path to `products/p07-indexer/test/explorer-browser.cjs`, and set `P07_PLAYWRIGHT_MODULE` to the installed Playwright module and `P07_CHROMIUM` to the local Chrome executable when needed. The fixture uses restricted PostgreSQL accounts, the normal operator executable and isolated Anvil receipts. It tests X03 hold with another live stream, X05 job persistence and rejection, and an independent existing switch request.

## v1 대여 세대 대사

P07의 대여 세대 Reader는 P01의 공개 [RJP1과 동결 영수증](../p01-device-firmware/RENTAL-JOURNAL-DESIGN.md)을 받는다. 전용 mTLS 어댑터는 호출 인증서에 지정한 스트림·지점·보드와 현재 등록의 보드 인증키를 확인한다. RFR1은 P01의 EIP-712 영수증 필드 8개와 보드 인증서명을 전달하는 253바이트 형식이다. 등록된 챌린지, 결속, 저널 루트, nonce 구간, 항목 수, 동결 시각과 만료가 현재 권위와 일치해야 한다. 숫자 결과를 보내기 직전에 등록과 호출자 권한을 다시 확인한다. 공개 조회 경로와 기존 C7을 별도로 운용한다.

Reader는 현재 수집 세대의 연속된 확정 블록, 배포 시작점 및 기준점의 런타임 바이트코드를 확인한다. 지급과 한도 승인에는 공유 nonce 비트가 사용된다. 사용된 비트는 확정 블록의 트랜잭션·영수증 루트, 직접 호출의 정규 calldata, v1 도메인과 승인 다이제스트, 계약 이벤트가 일치할 때 종결한다. 지급은 같은 기준점의 주문 사용 상태도 확인한다. 미사용 비트는 보호 저널의 서명 전 소각 또는 만료가 지난 기준점과 대조한다. v1은 만료와 같은 초에도 허용하므로 그보다 늦게 확정된 기준점이 필요하다.

같은 기준점에서 유효한 미사용 승인은 `pending`으로 보고하고 지급과 한도 변경 수를 구분한다. 모두 종결된 세대는 그 기준점과 유효기간을 포함한 `verified` 근거가 된다. 원호출을 확인할 수 없는 간접 실행, 등록 변경, 분기, 확정 범위 공백 또는 RPC 불완전 결과가 있으면 건수를 보류한다. 응답에는 집계, 기준점과 결속 다이제스트만 포함한다. 거래 calldata와 승인 서명은 요청 범위에서 검증하고 데이터베이스와 응답에 저장하지 않는다.

[공용 RJP1 벡터](../../packages/protocol/v2/testdata/rjp1.vectors) · [공용 RFR1 벡터](../../packages/protocol/v2/testdata/rfr1.vectors) · [검증 재현](test/README.md)

### 전용 대여 세대 대사 서비스

`indexer rental-generation`은 자체 TLS 1.3 상호 인증 리스너에서
`POST /v2/private/rental-generation/reconcile`을 제공한다. 기존 C7·공개 영수증·
Explorer 리스너는 각 원래 명령으로 실행한다.

- `P07_RENTAL_DATABASE_URL`은 016의 `nu54_p07_rental_reader` 전용 로그인이다.
  이 세션은 현재 체크포인트와 정본 블록 범위를 읽으며 배포·정본·영수증을 생성하거나
  갱신할 수 없다.
- `P07_RENTAL_PROFILE`은 단일 v1/8283 JSON 배포 프로필이다.
  `P07_RENTAL_PROFILE_KEY`와 `P07_RENTAL_PROFILE_SIGNATURE`에 별도 승인자의
  Ed25519 공개키와 승인 서명을 각각 hex로 제공한다. `P07_RENTAL_RPC`는 HTTPS를
  사용한다. 로컬 검증에는 loopback HTTP를 사용할 수 있다. 부팅 시 등록되어 활성화된
  정확한 스트림 세대와 핵심 배포 식별값·확정 블록을 확인한다.
- `P07_RENTAL_CA`, `P07_RENTAL_CERT`, `P07_RENTAL_KEY`는 인입 리스너용이다.
  `P07_RENTAL_GRANTS`는 호출자의 인증서 DER SHA-256 hex를 키로 하고
  `Stream`, `Branch`, `Board`, `ExpiresAt`을 담는 전용 grant JSON이다.
  철회는 grant 파일을 원자적으로 교체한다. 기본 주소는
  `P07_RENTAL_HTTP_ADDR=127.0.0.1:8445`이다.
- `P07_RENTAL_AUTHORITY_ORIGIN`, `P07_RENTAL_AUTHORITY_CA`,
  `P07_RENTAL_AUTHORITY_CERT`, `P07_RENTAL_AUTHORITY_KEY`는 P05 등록 권위
  조회에 사용한다. `P07_RENTAL_AUTHORITY_SPKI`는 P05 서버 리프 공개키의
  SHA-256 hex다. 권위 호출의 mTLS 인증서와 원래 P05 요청자의 인입 인증서 지문은
  별도 범위로 검증한다.

등록과 챌린지의 정본은 P05의 전용 `/v2/private/rental-generation/authority`다.
바인딩과 자격 유효성을 확인한 영수증만 대사하며, 응답 전에 현재 등록 권위와
확정 기준점을 다시 확인한다. 권위·체인·검증기 조건이 충족되지 않으면 건수 없는
상태를 반환한다. 등록 자료를 환경변수로 대신 입력하는 경로는 없다. 전용 회귀는
`test/generation-v1.py`에서 실행한다.

같은 포트의 `POST /v2/private/rental-generation/close-anchor`는 이미 보관한
평가의 기준점이 여전히 활성 프로필의 정본 조상인지 확인한다. 요청은 기존 인증서
범위, 대여 세대, 등록 결속, 챌린지, 기존 평가 다이제스트와 만료를 묶는다. P07은
현재 등록 권위 및 활성 확정점을 각각 두 번 확인하고, 활성 스트림의 PostgreSQL
정본 계보와 RPC 블록 해시가 같은 경우에만 최장 5초 유효한 확인을 반환한다.
확인에는 미결 건수나 반납 실행 권한이 포함되지 않는다. P05가 동일 원동결과
보관한 0건 평가를 직접 확인하고 실제 전송 경계를 별도로 보호한다.

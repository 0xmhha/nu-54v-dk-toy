# 구현 진입 설계 동결 checkpoint

2026-09-20 · `DF-20260920-01` · **20개 선택 완료 / 10개 제품 상세 설계 완료 / 8개 계약 묶음 설계 기준 채택 / 7개 환경 준비**

사용자는 미선택 결정을 끝내고 제품 구현 직전까지 설계를 진행하도록 요청했다. 이 문서는 그 요청에 따른 권장 선택을 구현 진입 기준으로 확정한다. 기존 문서에서 `미선택`으로 남은 기록은 당시 상태를 보존하며, 이후 구현 판단에는 이 checkpoint가 우선한다. 제품 코드·DB migration·펌웨어 build·계정 생성·컨트랙트 배포는 수행하지 않았다.

## 1. 정책·기술 결정 20개

| 결정 | 채택값 | 실패·축소 규칙 |
|---|---|---|
| **D01 · 앱·인증 대상** | 유저 앱과 키오스크를 React Native 0.87.x 기반의 별도 앱으로 만들고 TypeScript 도메인 패키지만 공유한다. 유저 앱은 Android 15+/Galaxy S25 Ultra와 iOS 18+를, 키오스크는 Android 15+ 태블릿을 기준으로 한다. 한국어·영어, Google·Apple 로그인을 지원하고 내부 배포 후 스토어 배포로 전환한다. | iOS 네이티브 BLE 모듈이 기준을 충족하지 못하면 iOS는 조회·Cloud Wallet 기능만 출시하고 기기 설정은 Android로 제한한다. |
| **D02 · 연결 규칙·데이터 경계** | HTTP는 JSON/UTF-8과 RFC 8785 JCS digest, BLE는 deterministic CBOR를 사용한다. 모든 명령은 contractVersion, requestId, idempotencyKey, actor, issuedAt, expiresAt, payloadDigest를 가진 v1 envelope를 사용한다. 업무 원장·권한·outbox는 PostgreSQL 한 transaction에 두고 체인·객체 저장소는 대사 가능한 외부 효과로 취급한다. | 같은 멱등키와 다른 digest는 conflict, 같은 digest는 원 결과를 반환한다. 서버 UTC와 기기 monotonic deadline을 사용하며 허용 시각 편차는 30초다. |
| **D03 · HW 지갑 키·복구** | HW와 Cloud Wallet은 별도 주소·signer로 표시한다. HW는 BIP-39 24단어 신규 생성, BIP-39 12/24단어 및 32-byte raw private key import, secp256k1 경로 m/44'/60'/0'/0/0을 지원한다. import는 X25519+HKDF-SHA-256+ChaCha20-Poly1305의 ephemeral 채널과 기기 표시 SAS 확인을 사용한다. 키는 TF-M/PSA secure partition의 비반출 service가 보관·서명한다. | NU 보드에서 opaque secp256k1 key가 불가하면 seed를 secure partition 내부에서 AEAD로 봉인하고 secure service 밖으로 평문을 내보내지 않는다. 일반 flash 분할 저장만으로 secure storage를 대체하지 않는다. |
| **D04 · MPC 신뢰 구조** | 1차 Cloud Wallet은 cb-mpc/cb-mpc-go v0.2.1 기반 2-of-3 ECDSA로 고정한다. 독립 배포·KMS 도메인의 signer A/B와 오프라인 recovery custodian C가 share를 가지며, 모바일의 별도 approval key가 모든 정상 서명에 필수다. 소셜 로그인만으로 서명·복구하지 않는다. | 현재 라이브러리의 모바일/browser 미지원 때문에 폰은 MPC share가 아니라 승인자다. 모바일 share 구현은 별도 revision에서만 교체하며 1차 완료 기준에 섞지 않는다. |
| **D05 · NU 보드·FOTA** | NCS v3.4.0 tag와 그 manifest가 고정한 Zephyr 4.4 계열을 사용한다. NU-54V-DK는 Nordic DK target으로 가장하지 않고 project-owned board definition을 별도 pin한다. MCUboot+MCUmgr SMP/BLE, DTS fixed-partitions, signed image, trial boot 후 confirm을 적용한다. | 실측 flash에서 dual-slot이 불가능하면 외부/보조 slot 설계로 전환한다. key journal은 image slot과 분리하며 업데이트 실패가 키 초기화를 일으키지 않는다. |
| **D06 · 패스키 호환 범위** | CTAP 2.2 external authenticator를 통제된 WebAuthn RP와 Galaxy S25 Ultra/Chrome 조합에서 우선 지원한다. BLE transport를 1차 후보로 검증하고, 보드 배선이 허용하면 USB HID를 companion fallback으로 둔다. user presence는 물리 버튼, user verification은 device PIN으로 한다. | BLE CTAP 상호운용이 실패하면 자체 근접승인을 passkey로 표기하지 않고 USB HID 검증 범위로 축소한다. |
| **D07 · 녹음·찾기 목표** | 기기는 16 kHz/16-bit/mono PCM을 20 ms·640-byte frame으로 보내고 폰이 WAV를 저장한다. 64 KiB ring buffer, 30분 session, sequence/CRC/retransmit/end manifest를 사용한다. Android foreground service에서 저장하며 찾기는 BLE RSSI+버저를 쓴다. Channel Sounding 거리값은 별도 peer module이 있을 때만 보조 신호로 사용한다. | 지속 throughput 48 KiB/s 또는 gap 복구 기준을 못 맞추면 LC3/ADPCM profile을 새 revision으로 검증한다. 거리 측정 실패는 결제 거절의 단독 근거가 되지 않는다. |
| **D08 · 결제·환불·정산** | 메뉴 기준가는 KRW이며 운영자가 근거 URL·관측 시각과 함께 게시한 signed daily KRW/USD 시험 환율로 dummy USDC 6자리 금액을 계산한다. quote TTL 60초, half-even rounding, 주문당 정확한 한 payment allocation을 정상 경로로 한다. 2 block 확인과 Indexer canonical 관측 후 paid로 확정한다. | 부분·초과·중복·지연 payment는 자동 완료하지 않고 대사 queue로 보낸다. reorg 시 confirmed를 reconciling으로 되돌린다. |
| **D09 · 대여·혜택** | 확정 결제 1건당 해당 매장 stamp 1개를 paymentAllocationId+ruleVersion으로 멱등 발급하고 10개를 1회 혜택으로 교환한다. 매장 간 합산하지 않는다. 부분 환불은 환불 금액 비율에 따라 stamp debt를 만들고 이미 사용한 혜택은 회수 대신 이후 적립에서 상계한다. | 반납 evidence가 불명확하면 자산을 건드리지 않고 재대여만 hold한다. |
| **D10 · StableNet 시험 환경** | 환경 ID stablenet-test-8283, RPC https://api.test.stablenet.network/, explorer https://explorer.stablenet.network/, EVM chainId 8283(0x205b), native WKRC 18자리, dummy USDC 6자리로 고정한다. 주소는 배포 manifest와 codeHash/ABI digest/deployment block이 모두 있을 때만 활성화한다. | 주소가 비어 있거나 eth_getCode가 0x이면 호출·index 등록을 차단한다. |
| **D11 · 스마트 계정 전환** | 1차 결제는 EOA로 완료한다. 2차는 stable-poc-contract/poc-platform의 Kernel/ERC-7579 및 ERC-4337 v0.7 compatible stack을 별도 smart-account 주소로 추가한다. 사용자가 전환을 선택하며 기존 자산·allowance를 자동 이동하지 않는다. | EntryPoint·factory·bundler·SDK exact revision과 on-chain code가 일치하기 전 UserOperation을 활성화하지 않는다. |
| **D12 · DeFi·FX 상품** | dummy USDC/WKRC constant-product AMM의 swap과 LP만 1차 범위로 한다. 운영자가 시험 유동성을 공급하며 quote TTL 30초, deadline 120초, 기본 slippage 0.5%·최대 1%, exact per-operation allowance를 사용한다. 같은 pool의 비율 표시는 TEST FX로만 제공한다. | 가격/유동성 snapshot이 만료되면 재견적하고 자동 재서명하지 않는다. |
| **D13 · Perpetual 상품** | TEST BTC/WKRC 단일 격리 시장, dummy USDC 담보, 운영자 서명 시험 oracle을 사용한다. 최대 3배, initial margin 40%, maintenance 25%, hourly funding 절대값 0.1% 상한, 1e18 fixed-point를 채택한다. | oracle age 60초 초과 시 신규 주문·청산을 중지하고 포지션 조회/추가 담보만 허용한다. |
| **D14 · STO 범위** | CafePass Test Membership 10,000 unit을 발행하되 배당·상환·소유권 등 실물 권리를 부여하지 않는다. 유효한 membership DID 자격 보유자끼리만 transfer 가능한 test restricted token으로 구현한다. | 법적 권리나 실제 판매가 필요한 변경은 별도 법률 검토와 새 profile 없이는 금지한다. |
| **D15 · DID 범위** | issuer는 did:web, holder는 did:key, W3C VC Data Model 2.0의 JWT enveloped credential과 ES256K를 사용한다. 상태는 Bitstring Status List로 제공하고 민감한 transfer·혜택에는 online current-status 확인을 요구한다. | offline presentation은 정보 표시만 허용하고 자산·보상 효과를 만들지 않는다. |
| **D16 · x402 범위** | x402 v2 exact payment로 AI 여행 코스 1개 자원을 판매한다. CAIP-2 network eip155:8283, dummy USDC EIP-3009 authorization, 가격 1.000000 dUSDC, 지급 후 entitlement 24시간을 적용한다. | 지급 확인 뒤 생성 실패 시 동일 payment로 재시도하고 제공자가 끝내 전달하지 못하면 원 payer로 환불한다. 성공 전달 뒤 자동 환불은 없다. |
| **D17 · 위치·후기·실제 데이터** | 한국 장소 검색은 Kakao Local API를 사용한다. 여행자 모드 session을 사용자가 켠 동안만 위치를 수집하고, 기본은 앱 사용 중 권한이다. 배경 발자취는 별도 동의 후 100 m 또는 5분 간격으로 저장한다. 결제 검증 후기만 verified 표시한다. | 지도 공급자 장애 시 저장 코스와 결제 장소만 보여주며 AI가 장소를 창작하지 않는다. |
| **D18 · 추천·챌린지 기준** | 전사는 OpenAI Audio Transcriptions의 gpt-4o-mini-transcribe, 요약·추천은 OpenAI Responses API의 gpt-5.6-luna를 고정한다. 장소·영업시간·이동·예산은 구조화 입력으로 주고 AI 결과는 제안 revision으로 저장한 뒤 사용자가 apply한다. | OpenAI 공급자 미등록 또는 장애 시 원음/텍스트 저장과 규칙 기반 근처 결제 장소 정렬만 제공한다. 모델 변경은 환경 manifest revision과 회귀 평가를 요구한다. |
| **D19 · 운영·수용 목표** | 비체인 API p95 1초, 결제 제출 UI 응답 2초, 체인 확정 목표 60초, 데모 사용 배터리 8시간, RPO 15분, RTO 60분을 채택한다. 감사/재무 역할, 개인정보 운영자, 릴리스 승인자, incident commander를 분리하고 emergency access는 2인 승인을 요구한다. | 실측이 목표를 넘으면 silent success 대신 degraded 상태와 재조회 경로를 보여준다. |
| **RR-DEC-01 · 신규 여행 EOA 늦은 자산·반납 복구** | 신규 생성 여행 EOA는 사용자가 통제하는 암호화 복구 package를 확인한 뒤에만 반납 reset을 허용한다. package는 Argon2id로 유도한 키와 AEAD로 client-side 암호화하며 사용자가 선택한 파일/클라우드에 저장하고 운영자·서비스는 복호화 키를 갖지 않는다. | 백업 생성·복원 점검이 없으면 잔액 0이어도 reset을 hold한다. import 지갑은 사용자의 기존 백업을 전제로 하며 강제 sweep을 하지 않는다. |

## 2. 제품별 상세 설계 10개 영역

### B-01 · NU 펌웨어·주변 부품

- **범위:** Zephyr app, secure service, BLE, MCUboot, buttons/display/mic/buzzer
- **구성:** boot/profile manager, secure wallet signer, CTAP authenticator, audio streamer, find/ranging adapter, stamp cache, SMP FOTA
- **계약:** CBOR BLE envelope v1, MCUmgr SMP, CTAP 2.2, signed image manifest
- **저장:** DTS partitions: bootloader, slot0, slot1/alternate, scratch if required, secure key journal, non-secret settings journal, crash-safe operation journal
- **상태:** BOOT→LOCKED→READY; exclusive SIGN/IMPORT/RESET/UPDATE, RECORD concurrent only under arbitration; trial image confirms after self-test
- **장애 규칙:** power loss resumes journal; BLE loss preserves sequence window; invalid image never changes active slot; uncertain reset blocks reuse
- **구현 수용 증거:** actual pin/flash/RAM report; wallet sign/import zeroization; 30-minute audio; CTAP RP test; FOTA interruption and rollback

### B-02 · 유저 앱·로그인·기기 설정

- **범위:** RN traveler app for identity, two wallets, rental device, payment, records and travel
- **구성:** Google/Apple OIDC, session/device binding, native BLE adapter, HW/Cloud wallet selector, payment/history, recording/transcript, travel mode, merchant-owner view
- **계약:** OIDC authorization code+PKCE, API envelope v1, BLE CBOR v1, deep link callback
- **저장:** OS keychain/keystore for refresh and device approval key; SQLite cache contains no raw wallet secret; server remains authority for business state
- **상태:** SIGNED_OUT→AUTHENTICATED→DEVICE_BOUND; each protected result is pending/succeeded/failed/unknown with original-result lookup
- **장애 규칙:** permission denial gives manual path; token refresh generation prevents old-token return; BLE reconnect does not repeat non-idempotent action
- **구현 수용 증거:** Android S25 end-to-end; iOS auth and supported BLE scope; account unlink/logout revocation; offline/read cache labeling; no secret in logs/backups

### B-03 · Cloud MPC

- **범위:** 2-of-3 ECDSA wallet creation, signing, refresh and controlled recovery
- **구성:** participant A, participant B, offline recovery C, mobile approval verifier, coordinator, transcript/evidence store
- **계약:** mTLS participant protocol, versioned DKG/sign/refresh request, protected result reader, approval capability
- **저장:** shares in separate KMS/HSM domains; coordinator stores encrypted transcripts, epoch and hashes but no reconstructable key
- **상태:** UNINITIALIZED→DKG→ACTIVE→REFRESHING→ACTIVE; COMPROMISED/RECOVERY_HOLD require independent approvals
- **장애 규칙:** quorum loss stops signing; stale epoch rejects; timeout returns unknown and resumes same session; social takeover alone cannot sign
- **구현 수용 증거:** DKG without full key; A/B and A/C threshold tests; refresh invalidates old epoch; one-participant compromise drill; recovery audit

### B-04 · RN 키오스크·매장 운영

- **범위:** Android tablet POS, owner operations, payment/refund/sales/settlement
- **구성:** store auth, menu/order, NU payment handoff, receipt, refund, sales/settlement, terminal administration
- **계약:** order snapshot DTO, payment allocation, refund command, settlement reader, BLE approval request
- **저장:** encrypted local queue for unsigned orders and display cache; no merchant signing key in JS storage; server transaction+outbox owns effects
- **상태:** CART→QUOTED→AWAITING_APPROVAL→SUBMITTED→CONFIRMING→PAID; EXPIRED/RECONCILING/REFUNDED explicit
- **장애 규칙:** tablet handover revokes former operator; duplicate payment maps to original result; over/partial payment enters reconciliation
- **구현 수용 증거:** shared-tablet lock; one exact payment; partial refund; day close/correction; role access and terminal transfer

### B-05 · 백오피스·관리 신뢰

- **범위:** merchant/device/rental/FOTA/payment exceptions/profile rollout/audit/recovery
- **구성:** merchant/device registry, rental lifecycle, payment reconciliation, FOTA campaign, profile rollout, audit evidence, incident/recovery panels
- **계약:** typed admin command, typed protected reader, two-person approval evidence, outbox/replay
- **저장:** PostgreSQL authoritative gates and append-only audit; object store for signed evidence; independent trust bootstrap registry
- **상태:** DRAFT→REVIEW→APPROVED→STAGED→ACTIVE; REJECTED/EXPIRED/REVOKED; emergency unlock is separate two-person action
- **장애 규칙:** new manifest cannot bootstrap itself; old authority cannot reactivate; partial rollout holds new effects while reads remain available
- **구현 수용 증거:** O01-O04 operator journeys; approval expiry/replay; key rotation; incident restriction; restore does not revive revoked authority

### B-06 · 테스트넷 토큰·상품 컨트랙트

- **범위:** dummy USDC/WKRC adapters, smart account, AMM, perpetual, restricted membership, DID/status and x402
- **구성:** MockUSDC+EIP-3009, WKRC adapter, Kernel/EntryPoint adapters, AMM, perpetual market, restricted membership, credential/status registry, x402 facilitator
- **계약:** ABI/event catalog, deployment manifest, role matrix, integer golden vectors
- **저장:** chain is execution source; signed deployment registry binds address, codeHash, ABI digest, deploy block and admin roles
- **상태:** contract state follows explicit pause/upgrade/admin profiles; unregistered address is unsupported
- **장애 규칙:** stale oracle pauses risk actions; expired quote rejects; reorg is reconciled by indexer; admin role cannot impersonate customer
- **구현 수용 증거:** unit/invariant/fuzz vectors; role-negative calls; events decoded; deployment/code hash; refund and reorg cases

### B-07 · Indexer·조회 프런트엔드

- **범위:** reuse indexer-go and indexer-frontend for chain observation and product projections
- **구성:** RPC ingest, canonical block tracker, ABI decoder registry, raw event store, projection workers, query API, operator explorer
- **계약:** chain 8283 RPC, event v1/v2, cursor bound to filter/schema/deployment, typed payment/product readers
- **저장:** raw block/log plus canonicality; consumer checkpoint and effect-dedup key; rebuild generation separated from active projection
- **상태:** HEAD→CONFIRMING→CANONICAL; REORGED reverses projection by generation; rebuild swaps only after completeness check
- **장애 규칙:** RPC outage marks stale; decoder mismatch quarantines event; cursor generation mismatch forces fresh query
- **구현 수용 증거:** duplicate/reorg fixtures; 2-block payment confirmation; full rebuild; old/new decoder coexistence; permission-safe queries

### B-08 · DEX·FX·Perpetual 앱 서비스

- **범위:** quotes, integer risk calculations, signing adapters, keeper and user result UI
- **구성:** AMM quote, allowance planner, trade submitter, position/margin, oracle guard, funding, liquidation keeper
- **계약:** versioned quote, approval source, transaction intent, position result, keeper lease
- **저장:** quote and risk snapshot immutable; execution linked to chain tx/log; keeper lease and attempt idempotency in PostgreSQL
- **상태:** QUOTED→SIGNED→SUBMITTED→SETTLED; PRICE_STALE/SLIPPAGE/REVERT/REORG explicit
- **장애 규칙:** never refresh and auto-resign; allowance cleanup shown separately; duplicate keeper has one effect
- **구현 수용 증거:** rounding boundary vectors; stale price; max leverage; slippage; reorg/liquidation duplicate

### B-09 · 여행·기록·추천·챌린지

- **범위:** location search, footprints, verified review, audio transcript/summary, itinerary and challenge
- **구성:** Kakao place adapter, travel session, audio object/segments, transcription, recommendation, review evidence, challenge/stamp
- **계약:** consent revision, place snapshot, audio manifest, AI job/result, itinerary draft/apply, deletion fanout
- **저장:** encrypted object storage for audio; relational metadata and source provenance; tombstone/generation seal prevents deleted backup revival
- **상태:** CAPTURED→TRANSCRIBING→DRAFTED→USER_APPLIED; DELETE_REQUESTED→PURGED with late-result rejection
- **장애 규칙:** provider outage keeps source data and rule-based view; hallucinated place rejected; refund revises verified purchase/challenge evidence
- **구현 수용 증거:** 30-minute transcript linkage; 20 itinerary corpus; consent withdrawal; retention purge/restore; verified review and refund

### B-10 · 공통 기반·릴리스·운영 도구

- **범위:** configuration, secrets, authorization, observability, backup/restore, release and evidence index
- **구성:** environment registry, secret references, policy/role engine, telemetry, backup/restore, release manifest, evidence index, incident tooling
- **계약:** signed environment manifest, health/readiness, audit event, release/rollback record
- **저장:** no secret values in repo; secretRef only. Immutable release/evidence metadata and encrypted backups with deletion generation
- **상태:** BUILD→VERIFY→STAGE→ACTIVATE→OBSERVE; rollback changes code/profile while chain/external effects are reconciled
- **장애 규칙:** missing manifest field blocks startup; restore requires current authority and deletion seal; emergency mode is time-bound
- **구현 수용 증거:** config lint; RPO/RTO drill; key rotation; rollback/reconciliation; 15-requirement evidence index

## 3. 기준 계약 채택 8개 묶음

이 채택은 **구현 진입용 설계 기준**이다. wire schema 생성, SQL 적용, firmware/profile 활성화, 체인 배포, runtime 검증은 아직 0이다. 상세 필드 목록은 기존 카탈로그를 유지하고, 충돌 시 [구조화 기준 계약](../specifications/design-baseline-contract.json)의 선택·불변조건을 우선한다.

| 묶음 | 함께 채택한 범위 | 기준 규칙 | 보존할 금지 조건 |
|---|---|---|---|
| **UA-01 · 인증·MPC·보호 결과** | OC-01~03, OC-16~20 | refresh generation, logout/unlink revocation, mobile approval capability, MPC epoch/quorum, typed protected result reader를 v1 기준으로 채택 | 과거 token 반환·일반 reader의 signed bytes 노출·늦은 MPC 결과의 무권한 활성화를 금지 |
| **UA-02 · 상거래·영수증·단말** | DI-01, OC-04~08, OC-23~24 | order/recipient snapshot, exact payment allocation, fulfillment, refund, settlement correction, receipt ownership, terminal handover를 채택 | 원 지급 한도 초과·전 단말의 고객 결과 접근·마감 덮어쓰기를 금지 |
| **UA-03 · 대여·반납·기기 연속성** | DI-04~06 | enrollment holder, BLE/HTTP owner, reset journal, returned/cleanup/reuse gate와 RR-DEC-01 백업 확인을 채택 | 불명 reset 재시작·import 자산 강제 sweep·증거 없는 재대여를 금지 |
| **UA-04 · 시장·자격·유료 자원** | OC-09~12, OC-21~22 | quote/keeper, DID/status, restricted membership, x402 payment-entitlement-delivery-refund의 typed source를 채택 | unsupported signer의 EOA 우회·만료 quote 재서명·지급과 전달 결과 합치기를 금지 |
| **UA-05 · 녹음·여행·개인정보** | OC-13~15, OC-25~26 | audio owner/segment, transcript/AI provenance, consent/deletion generation, itinerary draft/apply, verified visit/review를 채택 | 삭제 뒤 AI 결과·backup 부활, 수동 편집의 AI 결과 위장, 보상 writer 중복을 금지 |
| **UA-06 · 설정 적용·관리 신뢰** | PRC 7, PCP 7, BLE candidate 3, TMC 8 | profile stage/approve/activate/apply, 독립 bootstrap, key rotation, restriction/recovery 및 O03/O04 reader를 채택 | manifest self-bootstrap·단독 emergency unlock·구 authority 재활성화를 금지 |
| **UA-07 · 공유 저장·이벤트·화면** | DI-02, DI-03, DI-07 | event v1/v2, transaction+outbox, consumer checkpoint, single effect writer, unknown-result UI와 rebuild generation을 채택 | double effect, cursor 세대 혼용, 처리 성공을 화면 성공으로 추정하는 것을 금지 |
| **UA-08 · 통합 checkpoint** | UA-01~07 and 21 shared target files | DF-20260920-01를 implementation-entry design baseline으로 채택하고 각 candidate는 preserve/change/add/hold disposition을 가진다 | 기존 7 migration은 불변이며 후속 migration 번호를 사용한다. 문서 채택을 runtime·SQL·device·chain activation으로 간주하지 않는다. |

## 4. 외부 환경 준비 7개

| 항목 | 준비 상태 | 저장소 산출물 | 남은 외부 행위 |
|---|---|---|---|
| **E-01 · 구현 전환 경계** | prepared | [README.md](../environment/README.md) | 사용자의 별도 구현 착수 지시 |
| **E-02 · 개발 기준 고정** | prepared_conditional_hardware | [toolchain-pins.json](../environment/toolchain-pins.json) | NU-54V-DK 실제 revision·board definition commit·flash 측정 |
| **E-03 · 계정·공급자** | template_ready_external_registration | [provider-accounts.template.json](../environment/provider-accounts.template.json) | Google/Apple/Kakao/OpenAI 계정·redirect·비밀 등록 |
| **E-04 · StableNet 환경** | rpc_verified_deployments_pending | [stablenet-8283.manifest.json](../environment/stablenet-8283.manifest.json) | dummy USDC와 상품 계약 배포·주소/codeHash/ABI digest |
| **E-05 · Indexer 환경** | revision_pinned_runtime_pending | [indexer.manifest.json](../environment/indexer.manifest.json) | 실 endpoint·DB/object store·decoder registry 배포 |
| **E-06 · 운영 신뢰 bootstrap** | template_ready_external_registration | [trust-bootstrap.template.json](../environment/trust-bootstrap.template.json) | 실제 인물·workload identity·KMS/HSM key reference의 2인 등록 |
| **E-07 · 개발·시험 데이터** | prepared | [test-fixtures.manifest.json](../environment/test-fixtures.manifest.json) | 실 카페 정보는 동의 후 별도 tenant로 입력 |

## 동결 후 남은 조건

- 실제 NU-54V-DK의 revision·pin·flash/RAM·secure service를 측정해야 펌웨어 layout을 활성화할 수 있다.
- Google·Apple·Kakao·OpenAI 계정과 secret은 저장소 밖에서 등록해야 한다.
- StableNet 계약 주소는 현재 모두 비활성이다. 배포 주소, code hash, ABI digest, deployment block, 관리자 권한 증거가 있어야 활성화한다.
- MPC, BLE passkey, 오디오, FOTA, 거리 측정과 12개 통합 여정은 실제 실행되지 않았다.
- 실자산·실권리·상용 대여는 최신 법률과 공급자 약관을 별도로 검토해야 한다.

다음 동작은 저장소 정리 checkpoint다. 정리 뒤에는 사용자의 명시적 구현 착수 지시 전까지 제품 구현을 시작하지 않는다.

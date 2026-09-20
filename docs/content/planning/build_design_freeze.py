#!/usr/bin/env python3
"""Build the implementation-entry design freeze and its environment templates."""

from __future__ import annotations

import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DATE = "2026-09-20"
CHECKPOINT = "DF-20260920-01"


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def dump(path: Path, value: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")


decisions = [
    dict(id="D01", title="앱·인증 대상", selection="유저 앱과 키오스크를 React Native 0.87.x 기반의 별도 앱으로 만들고 TypeScript 도메인 패키지만 공유한다. 유저 앱은 Android 15+/Galaxy S25 Ultra와 iOS 18+를, 키오스크는 Android 15+ 태블릿을 기준으로 한다. 한국어·영어, Google·Apple 로그인을 지원하고 내부 배포 후 스토어 배포로 전환한다.", fallback="iOS 네이티브 BLE 모듈이 기준을 충족하지 못하면 iOS는 조회·Cloud Wallet 기능만 출시하고 기기 설정은 Android로 제한한다.", evidence="RN 0.87은 2026-09-20 현재 활성 stable 계열이며 실제 BLE·백그라운드·소셜 로그인 bridge는 구현 단계에서 기종별 검증한다."),
    dict(id="D02", title="연결 규칙·데이터 경계", selection="HTTP는 JSON/UTF-8과 RFC 8785 JCS digest, BLE는 deterministic CBOR를 사용한다. 모든 명령은 contractVersion, requestId, idempotencyKey, actor, issuedAt, expiresAt, payloadDigest를 가진 v1 envelope를 사용한다. 업무 원장·권한·outbox는 PostgreSQL 한 transaction에 두고 체인·객체 저장소는 대사 가능한 외부 효과로 취급한다.", fallback="같은 멱등키와 다른 digest는 conflict, 같은 digest는 원 결과를 반환한다. 서버 UTC와 기기 monotonic deadline을 사용하며 허용 시각 편차는 30초다.", evidence="기존 OC 26개, API 110개, BLE 34개의 공통 식별·재조회 규칙을 한 profile로 고정한다."),
    dict(id="D03", title="HW 지갑 키·복구", selection="HW와 Cloud Wallet은 별도 주소·signer로 표시한다. HW는 BIP-39 24단어 신규 생성, BIP-39 12/24단어 및 32-byte raw private key import, secp256k1 경로 m/44'/60'/0'/0/0을 지원한다. import는 X25519+HKDF-SHA-256+ChaCha20-Poly1305의 ephemeral 채널과 기기 표시 SAS 확인을 사용한다. 키는 TF-M/PSA secure partition의 비반출 service가 보관·서명한다.", fallback="NU 보드에서 opaque secp256k1 key가 불가하면 seed를 secure partition 내부에서 AEAD로 봉인하고 secure service 밖으로 평문을 내보내지 않는다. 일반 flash 분할 저장만으로 secure storage를 대체하지 않는다.", evidence="앱 로그·클립보드·분석·crash dump에 mnemonic/private key를 남기지 않고, 표시·서명·초기화는 물리 버튼 확인을 요구한다."),
    dict(id="D04", title="MPC 신뢰 구조", selection="1차 Cloud Wallet은 cb-mpc/cb-mpc-go v0.2.1 기반 2-of-3 ECDSA로 고정한다. 독립 배포·KMS 도메인의 signer A/B와 오프라인 recovery custodian C가 share를 가지며, 모바일의 별도 approval key가 모든 정상 서명에 필수다. 소셜 로그인만으로 서명·복구하지 않는다.", fallback="현재 라이브러리의 모바일/browser 미지원 때문에 폰은 MPC share가 아니라 승인자다. 모바일 share 구현은 별도 revision에서만 교체하며 1차 완료 기준에 섞지 않는다.", evidence="DKG, threshold sign, refresh transcript를 보존하고 mTLS·participant identity·epoch·quorum을 검증한다. 백업 시 전체 키 재조립을 금지한다."),
    dict(id="D05", title="NU 보드·FOTA", selection="NCS v3.4.0 tag와 그 manifest가 고정한 Zephyr 4.4 계열을 사용한다. NU-54V-DK는 Nordic DK target으로 가장하지 않고 project-owned board definition을 별도 pin한다. MCUboot+MCUmgr SMP/BLE, DTS fixed-partitions, signed image, trial boot 후 confirm을 적용한다.", fallback="실측 flash에서 dual-slot이 불가능하면 외부/보조 slot 설계로 전환한다. key journal은 image slot과 분리하며 업데이트 실패가 키 초기화를 일으키지 않는다.", evidence="정확한 board revision·pin map·flash/RAM·slot 크기는 실기 bring-up의 blocking measurement다. 사용자 유예가 기본이고 강제 업데이트는 현재 비활성이다."),
    dict(id="D06", title="패스키 호환 범위", selection="CTAP 2.2 external authenticator를 통제된 WebAuthn RP와 Galaxy S25 Ultra/Chrome 조합에서 우선 지원한다. BLE transport를 1차 후보로 검증하고, 보드 배선이 허용하면 USB HID를 companion fallback으로 둔다. user presence는 물리 버튼, user verification은 device PIN으로 한다.", fallback="BLE CTAP 상호운용이 실패하면 자체 근접승인을 passkey로 표기하지 않고 USB HID 검증 범위로 축소한다.", evidence="discoverable credential은 device-bound로 표시하며 cloud sync/backup을 제공하지 않는다. 반납 전에 rental credential 삭제와 RP revoke 증거를 확인한다."),
    dict(id="D07", title="녹음·찾기 목표", selection="기기는 16 kHz/16-bit/mono PCM을 20 ms·640-byte frame으로 보내고 폰이 WAV를 저장한다. 64 KiB ring buffer, 30분 session, sequence/CRC/retransmit/end manifest를 사용한다. Android foreground service에서 저장하며 찾기는 BLE RSSI+버저를 쓴다. Channel Sounding 거리값은 별도 peer module이 있을 때만 보조 신호로 사용한다.", fallback="지속 throughput 48 KiB/s 또는 gap 복구 기준을 못 맞추면 LC3/ADPCM profile을 새 revision으로 검증한다. 거리 측정 실패는 결제 거절의 단독 근거가 되지 않는다.", evidence="목표는 30분 완주, 재전송 후 frame loss 1% 미만, 누락 구간 표시, 원음과 transcript segment 연결이다."),
    dict(id="D08", title="결제·환불·정산", selection="메뉴 기준가는 KRW이며 운영자가 근거 URL·관측 시각과 함께 게시한 signed daily KRW/USD 시험 환율로 dummy USDC 6자리 금액을 계산한다. quote TTL 60초, half-even rounding, 주문당 정확한 한 payment allocation을 정상 경로로 한다. 2 block 확인과 Indexer canonical 관측 후 paid로 확정한다.", fallback="부분·초과·중복·지연 payment는 자동 완료하지 않고 대사 queue로 보낸다. reorg 시 confirmed를 reconciling으로 되돌린다.", evidence="점주 주소로 직접 수령한다. 부분 환불을 지원하고 원 payer/allocation 한도를 넘지 않는다. 환불 gas는 점주가, 고객의 최초 결제 gas는 고객이 부담한다."),
    dict(id="D09", title="대여·혜택", selection="확정 결제 1건당 해당 매장 stamp 1개를 paymentAllocationId+ruleVersion으로 멱등 발급하고 10개를 1회 혜택으로 교환한다. 매장 간 합산하지 않는다. 부분 환불은 환불 금액 비율에 따라 stamp debt를 만들고 이미 사용한 혜택은 회수 대신 이후 적립에서 상계한다.", fallback="반납 evidence가 불명확하면 자산을 건드리지 않고 재대여만 hold한다.", evidence="import 지갑은 잔액 이체를 강제하지 않으며 신규 여행 EOA는 RR-DEC-01 복구 조건을 충족해야 reset 가능하다."),
    dict(id="D10", title="StableNet 시험 환경", selection="환경 ID stablenet-test-8283, RPC https://api.test.stablenet.network/, explorer https://explorer.stablenet.network/, EVM chainId 8283(0x205b), native WKRC 18자리, dummy USDC 6자리로 고정한다. 주소는 배포 manifest와 codeHash/ABI digest/deployment block이 모두 있을 때만 활성화한다.", fallback="주소가 비어 있거나 eth_getCode가 0x이면 호출·index 등록을 차단한다.", evidence="2026-09-20 RPC에서 chainId 0x205b와 block 0x13435dd를 관측했다. 기존 EntryPoint 후보 0xEf...8A14는 해당 RPC에서 code 0x이므로 채택하지 않는다."),
    dict(id="D11", title="스마트 계정 전환", selection="1차 결제는 EOA로 완료한다. 2차는 stable-poc-contract/poc-platform의 Kernel/ERC-7579 및 ERC-4337 v0.7 compatible stack을 별도 smart-account 주소로 추가한다. 사용자가 전환을 선택하며 기존 자산·allowance를 자동 이동하지 않는다.", fallback="EntryPoint·factory·bundler·SDK exact revision과 on-chain code가 일치하기 전 UserOperation을 활성화하지 않는다.", evidence="초기 gas는 사용자 WKRC, sponsorship/paymaster는 후속 profile이다. EOA 결과를 smart-account 성공으로 대체하지 않는다."),
    dict(id="D12", title="DeFi·FX 상품", selection="dummy USDC/WKRC constant-product AMM의 swap과 LP만 1차 범위로 한다. 운영자가 시험 유동성을 공급하며 quote TTL 30초, deadline 120초, 기본 slippage 0.5%·최대 1%, exact per-operation allowance를 사용한다. 같은 pool의 비율 표시는 TEST FX로만 제공한다.", fallback="가격/유동성 snapshot이 만료되면 재견적하고 자동 재서명하지 않는다.", evidence="실제 환전·수익 상품으로 표기하지 않고 integer reserve·fee·rounding golden vector로 검증한다."),
    dict(id="D13", title="Perpetual 상품", selection="TEST BTC/WKRC 단일 격리 시장, dummy USDC 담보, 운영자 서명 시험 oracle을 사용한다. 최대 3배, initial margin 40%, maintenance 25%, hourly funding 절대값 0.1% 상한, 1e18 fixed-point를 채택한다.", fallback="oracle age 60초 초과 시 신규 주문·청산을 중지하고 포지션 조회/추가 담보만 허용한다.", evidence="keeper는 가격 게시자·관리자와 분리하고 liquidation idempotency와 reorg 재대사를 검증한다. 실제 투자상품이 아니다."),
    dict(id="D14", title="STO 범위", selection="CafePass Test Membership 10,000 unit을 발행하되 배당·상환·소유권 등 실물 권리를 부여하지 않는다. 유효한 membership DID 자격 보유자끼리만 transfer 가능한 test restricted token으로 구현한다.", fallback="법적 권리나 실제 판매가 필요한 변경은 별도 법률 검토와 새 profile 없이는 금지한다.", evidence="모든 화면·metadata·explorer 안내에 TEST / NO REAL-WORLD RIGHTS를 표시한다."),
    dict(id="D15", title="DID 범위", selection="issuer는 did:web, holder는 did:key, W3C VC Data Model 2.0의 JWT enveloped credential과 ES256K를 사용한다. 상태는 Bitstring Status List로 제공하고 민감한 transfer·혜택에는 online current-status 확인을 요구한다.", fallback="offline presentation은 정보 표시만 허용하고 자산·보상 효과를 만들지 않는다.", evidence="issuer key rotation, status list version, subject consent, 최소 claim, revoke reason의 공개 범위를 manifest로 관리한다."),
    dict(id="D16", title="x402 범위", selection="x402 v2 exact payment로 AI 여행 코스 1개 자원을 판매한다. CAIP-2 network eip155:8283, dummy USDC EIP-3009 authorization, 가격 1.000000 dUSDC, 지급 후 entitlement 24시간을 적용한다.", fallback="지급 확인 뒤 생성 실패 시 동일 payment로 재시도하고 제공자가 끝내 전달하지 못하면 원 payer로 환불한다. 성공 전달 뒤 자동 환불은 없다.", evidence="payment, entitlement, delivery 세 결과를 분리하고 응답 유실 때 동일 idempotency key로 원 결과를 조회한다."),
    dict(id="D17", title="위치·후기·실제 데이터", selection="한국 장소 검색은 Kakao Local API를 사용한다. 여행자 모드 session을 사용자가 켠 동안만 위치를 수집하고, 기본은 앱 사용 중 권한이다. 배경 발자취는 별도 동의 후 100 m 또는 5분 간격으로 저장한다. 결제 검증 후기만 verified 표시한다.", fallback="지도 공급자 장애 시 저장 코스와 결제 장소만 보여주며 AI가 장소를 창작하지 않는다.", evidence="정밀 위치·원음 30일, transcript 90일, 파생 발자취 1년, testnet 거래·감사 증거 1년, 리뷰는 삭제 요청까지 보관한다. 삭제 fanout은 backup restore도 차단한다."),
    dict(id="D18", title="추천·챌린지 기준", selection="전사는 OpenAI Audio Transcriptions의 gpt-4o-mini-transcribe, 요약·추천은 OpenAI Responses API의 gpt-5.6-luna를 고정한다. 장소·영업시간·이동·예산은 구조화 입력으로 주고 AI 결과는 제안 revision으로 저장한 뒤 사용자가 apply한다.", fallback="OpenAI 공급자 미등록 또는 장애 시 원음/텍스트 저장과 규칙 기반 근처 결제 장소 정렬만 제공한다. 모델 변경은 환경 manifest revision과 회귀 평가를 요구한다.", evidence="20개 고정 코스에서 존재하지 않는 장소 0건, hard constraint 통과 90% 이상, 구매·후기·위치 근거 표시를 수용 기준으로 한다."),
    dict(id="D19", title="운영·수용 목표", selection="비체인 API p95 1초, 결제 제출 UI 응답 2초, 체인 확정 목표 60초, 데모 사용 배터리 8시간, RPO 15분, RTO 60분을 채택한다. 감사/재무 역할, 개인정보 운영자, 릴리스 승인자, incident commander를 분리하고 emergency access는 2인 승인을 요구한다.", fallback="실측이 목표를 넘으면 silent success 대신 degraded 상태와 재조회 경로를 보여준다.", evidence="릴리스 키는 연 1회 또는 사건 즉시 회전한다. 실자산·실권리·상용 여행자 대여는 최신 법률·약관 검토 전 활성화하지 않는다."),
    dict(id="RR-DEC-01", title="신규 여행 EOA 늦은 자산·반납 복구", selection="신규 생성 여행 EOA는 사용자가 통제하는 암호화 복구 package를 확인한 뒤에만 반납 reset을 허용한다. package는 Argon2id로 유도한 키와 AEAD로 client-side 암호화하며 사용자가 선택한 파일/클라우드에 저장하고 운영자·서비스는 복호화 키를 갖지 않는다.", fallback="백업 생성·복원 점검이 없으면 잔액 0이어도 reset을 hold한다. import 지갑은 사용자의 기존 백업을 전제로 하며 강제 sweep을 하지 않는다.", evidence="늦은 입금은 복구 package로 사용자가 접근한다. 반납 시 backup digest·주소·확인 시각만 서버에 남기고 mnemonic/private key는 남기지 않는다."),
]

products = [
    dict(id="B-01", title="NU 펌웨어·주변 부품", scope="Zephyr app, secure service, BLE, MCUboot, buttons/display/mic/buzzer", modules=["boot/profile manager", "secure wallet signer", "CTAP authenticator", "audio streamer", "find/ranging adapter", "stamp cache", "SMP FOTA"], interfaces=["CBOR BLE envelope v1", "MCUmgr SMP", "CTAP 2.2", "signed image manifest"], persistence="DTS partitions: bootloader, slot0, slot1/alternate, scratch if required, secure key journal, non-secret settings journal, crash-safe operation journal", state="BOOT→LOCKED→READY; exclusive SIGN/IMPORT/RESET/UPDATE, RECORD concurrent only under arbitration; trial image confirms after self-test", failures="power loss resumes journal; BLE loss preserves sequence window; invalid image never changes active slot; uncertain reset blocks reuse", acceptance=["actual pin/flash/RAM report", "wallet sign/import zeroization", "30-minute audio", "CTAP RP test", "FOTA interruption and rollback"]),
    dict(id="B-02", title="유저 앱·로그인·기기 설정", scope="RN traveler app for identity, two wallets, rental device, payment, records and travel", modules=["Google/Apple OIDC", "session/device binding", "native BLE adapter", "HW/Cloud wallet selector", "payment/history", "recording/transcript", "travel mode", "merchant-owner view"], interfaces=["OIDC authorization code+PKCE", "API envelope v1", "BLE CBOR v1", "deep link callback"], persistence="OS keychain/keystore for refresh and device approval key; SQLite cache contains no raw wallet secret; server remains authority for business state", state="SIGNED_OUT→AUTHENTICATED→DEVICE_BOUND; each protected result is pending/succeeded/failed/unknown with original-result lookup", failures="permission denial gives manual path; token refresh generation prevents old-token return; BLE reconnect does not repeat non-idempotent action", acceptance=["Android S25 end-to-end", "iOS auth and supported BLE scope", "account unlink/logout revocation", "offline/read cache labeling", "no secret in logs/backups"]),
    dict(id="B-03", title="Cloud MPC", scope="2-of-3 ECDSA wallet creation, signing, refresh and controlled recovery", modules=["participant A", "participant B", "offline recovery C", "mobile approval verifier", "coordinator", "transcript/evidence store"], interfaces=["mTLS participant protocol", "versioned DKG/sign/refresh request", "protected result reader", "approval capability"], persistence="shares in separate KMS/HSM domains; coordinator stores encrypted transcripts, epoch and hashes but no reconstructable key", state="UNINITIALIZED→DKG→ACTIVE→REFRESHING→ACTIVE; COMPROMISED/RECOVERY_HOLD require independent approvals", failures="quorum loss stops signing; stale epoch rejects; timeout returns unknown and resumes same session; social takeover alone cannot sign", acceptance=["DKG without full key", "A/B and A/C threshold tests", "refresh invalidates old epoch", "one-participant compromise drill", "recovery audit"]),
    dict(id="B-04", title="RN 키오스크·매장 운영", scope="Android tablet POS, owner operations, payment/refund/sales/settlement", modules=["store auth", "menu/order", "NU payment handoff", "receipt", "refund", "sales/settlement", "terminal administration"], interfaces=["order snapshot DTO", "payment allocation", "refund command", "settlement reader", "BLE approval request"], persistence="encrypted local queue for unsigned orders and display cache; no merchant signing key in JS storage; server transaction+outbox owns effects", state="CART→QUOTED→AWAITING_APPROVAL→SUBMITTED→CONFIRMING→PAID; EXPIRED/RECONCILING/REFUNDED explicit", failures="tablet handover revokes former operator; duplicate payment maps to original result; over/partial payment enters reconciliation", acceptance=["shared-tablet lock", "one exact payment", "partial refund", "day close/correction", "role access and terminal transfer"]),
    dict(id="B-05", title="백오피스·관리 신뢰", scope="merchant/device/rental/FOTA/payment exceptions/profile rollout/audit/recovery", modules=["merchant/device registry", "rental lifecycle", "payment reconciliation", "FOTA campaign", "profile rollout", "audit evidence", "incident/recovery panels"], interfaces=["typed admin command", "typed protected reader", "two-person approval evidence", "outbox/replay"], persistence="PostgreSQL authoritative gates and append-only audit; object store for signed evidence; independent trust bootstrap registry", state="DRAFT→REVIEW→APPROVED→STAGED→ACTIVE; REJECTED/EXPIRED/REVOKED; emergency unlock is separate two-person action", failures="new manifest cannot bootstrap itself; old authority cannot reactivate; partial rollout holds new effects while reads remain available", acceptance=["O01-O04 operator journeys", "approval expiry/replay", "key rotation", "incident restriction", "restore does not revive revoked authority"]),
    dict(id="B-06", title="테스트넷 토큰·상품 컨트랙트", scope="dummy USDC/WKRC adapters, smart account, AMM, perpetual, restricted membership, DID/status and x402", modules=["MockUSDC+EIP-3009", "WKRC adapter", "Kernel/EntryPoint adapters", "AMM", "perpetual market", "restricted membership", "credential/status registry", "x402 facilitator"], interfaces=["ABI/event catalog", "deployment manifest", "role matrix", "integer golden vectors"], persistence="chain is execution source; signed deployment registry binds address, codeHash, ABI digest, deploy block and admin roles", state="contract state follows explicit pause/upgrade/admin profiles; unregistered address is unsupported", failures="stale oracle pauses risk actions; expired quote rejects; reorg is reconciled by indexer; admin role cannot impersonate customer", acceptance=["unit/invariant/fuzz vectors", "role-negative calls", "events decoded", "deployment/code hash", "refund and reorg cases"]),
    dict(id="B-07", title="Indexer·조회 프런트엔드", scope="reuse indexer-go and indexer-frontend for chain observation and product projections", modules=["RPC ingest", "canonical block tracker", "ABI decoder registry", "raw event store", "projection workers", "query API", "operator explorer"], interfaces=["chain 8283 RPC", "event v1/v2", "cursor bound to filter/schema/deployment", "typed payment/product readers"], persistence="raw block/log plus canonicality; consumer checkpoint and effect-dedup key; rebuild generation separated from active projection", state="HEAD→CONFIRMING→CANONICAL; REORGED reverses projection by generation; rebuild swaps only after completeness check", failures="RPC outage marks stale; decoder mismatch quarantines event; cursor generation mismatch forces fresh query", acceptance=["duplicate/reorg fixtures", "2-block payment confirmation", "full rebuild", "old/new decoder coexistence", "permission-safe queries"]),
    dict(id="B-08", title="DEX·FX·Perpetual 앱 서비스", scope="quotes, integer risk calculations, signing adapters, keeper and user result UI", modules=["AMM quote", "allowance planner", "trade submitter", "position/margin", "oracle guard", "funding", "liquidation keeper"], interfaces=["versioned quote", "approval source", "transaction intent", "position result", "keeper lease"], persistence="quote and risk snapshot immutable; execution linked to chain tx/log; keeper lease and attempt idempotency in PostgreSQL", state="QUOTED→SIGNED→SUBMITTED→SETTLED; PRICE_STALE/SLIPPAGE/REVERT/REORG explicit", failures="never refresh and auto-resign; allowance cleanup shown separately; duplicate keeper has one effect", acceptance=["rounding boundary vectors", "stale price", "max leverage", "slippage", "reorg/liquidation duplicate"]),
    dict(id="B-09", title="여행·기록·추천·챌린지", scope="location search, footprints, verified review, audio transcript/summary, itinerary and challenge", modules=["Kakao place adapter", "travel session", "audio object/segments", "transcription", "recommendation", "review evidence", "challenge/stamp"], interfaces=["consent revision", "place snapshot", "audio manifest", "AI job/result", "itinerary draft/apply", "deletion fanout"], persistence="encrypted object storage for audio; relational metadata and source provenance; tombstone/generation seal prevents deleted backup revival", state="CAPTURED→TRANSCRIBING→DRAFTED→USER_APPLIED; DELETE_REQUESTED→PURGED with late-result rejection", failures="provider outage keeps source data and rule-based view; hallucinated place rejected; refund revises verified purchase/challenge evidence", acceptance=["30-minute transcript linkage", "20 itinerary corpus", "consent withdrawal", "retention purge/restore", "verified review and refund"]),
    dict(id="B-10", title="공통 기반·릴리스·운영 도구", scope="configuration, secrets, authorization, observability, backup/restore, release and evidence index", modules=["environment registry", "secret references", "policy/role engine", "telemetry", "backup/restore", "release manifest", "evidence index", "incident tooling"], interfaces=["signed environment manifest", "health/readiness", "audit event", "release/rollback record"], persistence="no secret values in repo; secretRef only. Immutable release/evidence metadata and encrypted backups with deletion generation", state="BUILD→VERIFY→STAGE→ACTIVATE→OBSERVE; rollback changes code/profile while chain/external effects are reconciled", failures="missing manifest field blocks startup; restore requires current authority and deletion seal; emergency mode is time-bound", acceptance=["config lint", "RPO/RTO drill", "key rotation", "rollback/reconciliation", "15-requirement evidence index"]),
]

adoptions = [
    dict(id="UA-01", title="인증·MPC·보호 결과", adopts="OC-01~03, OC-16~20", rules="refresh generation, logout/unlink revocation, mobile approval capability, MPC epoch/quorum, typed protected result reader를 v1 기준으로 채택", preserve="과거 token 반환·일반 reader의 signed bytes 노출·늦은 MPC 결과의 무권한 활성화를 금지"),
    dict(id="UA-02", title="상거래·영수증·단말", adopts="DI-01, OC-04~08, OC-23~24", rules="order/recipient snapshot, exact payment allocation, fulfillment, refund, settlement correction, receipt ownership, terminal handover를 채택", preserve="원 지급 한도 초과·전 단말의 고객 결과 접근·마감 덮어쓰기를 금지"),
    dict(id="UA-03", title="대여·반납·기기 연속성", adopts="DI-04~06", rules="enrollment holder, BLE/HTTP owner, reset journal, returned/cleanup/reuse gate와 RR-DEC-01 백업 확인을 채택", preserve="불명 reset 재시작·import 자산 강제 sweep·증거 없는 재대여를 금지"),
    dict(id="UA-04", title="시장·자격·유료 자원", adopts="OC-09~12, OC-21~22", rules="quote/keeper, DID/status, restricted membership, x402 payment-entitlement-delivery-refund의 typed source를 채택", preserve="unsupported signer의 EOA 우회·만료 quote 재서명·지급과 전달 결과 합치기를 금지"),
    dict(id="UA-05", title="녹음·여행·개인정보", adopts="OC-13~15, OC-25~26", rules="audio owner/segment, transcript/AI provenance, consent/deletion generation, itinerary draft/apply, verified visit/review를 채택", preserve="삭제 뒤 AI 결과·backup 부활, 수동 편집의 AI 결과 위장, 보상 writer 중복을 금지"),
    dict(id="UA-06", title="설정 적용·관리 신뢰", adopts="PRC 7, PCP 7, BLE candidate 3, TMC 8", rules="profile stage/approve/activate/apply, 독립 bootstrap, key rotation, restriction/recovery 및 O03/O04 reader를 채택", preserve="manifest self-bootstrap·단독 emergency unlock·구 authority 재활성화를 금지"),
    dict(id="UA-07", title="공유 저장·이벤트·화면", adopts="DI-02, DI-03, DI-07", rules="event v1/v2, transaction+outbox, consumer checkpoint, single effect writer, unknown-result UI와 rebuild generation을 채택", preserve="double effect, cursor 세대 혼용, 처리 성공을 화면 성공으로 추정하는 것을 금지"),
    dict(id="UA-08", title="통합 checkpoint", adopts="UA-01~07 and 21 shared target files", rules=f"{CHECKPOINT}를 implementation-entry design baseline으로 채택하고 각 candidate는 preserve/change/add/hold disposition을 가진다", preserve="기존 7 migration은 불변이며 후속 migration 번호를 사용한다. 문서 채택을 runtime·SQL·device·chain activation으로 간주하지 않는다."),
]

environment = [
    dict(id="E-01", title="구현 전환 경계", status="prepared", artifact="content/environment/README.md", ready="설계 checkpoint와 구현/배포 비활성 상태를 명시", pending="사용자의 별도 구현 착수 지시"),
    dict(id="E-02", title="개발 기준 고정", status="prepared_conditional_hardware", artifact="content/environment/toolchain-pins.json", ready="RN/NCS/Zephyr 및 4개 재사용 repo revision 고정", pending="NU-54V-DK 실제 revision·board definition commit·flash 측정"),
    dict(id="E-03", title="계정·공급자", status="template_ready_external_registration", artifact="content/environment/provider-accounts.template.json", ready="OAuth/지도/OpenAI/MPC scope와 secretRef schema", pending="Google/Apple/Kakao/OpenAI 계정·redirect·비밀 등록"),
    dict(id="E-04", title="StableNet 환경", status="rpc_verified_deployments_pending", artifact="content/environment/stablenet-8283.manifest.json", ready="RPC chain identity와 주소 활성화 gate", pending="dummy USDC와 상품 계약 배포·주소/codeHash/ABI digest"),
    dict(id="E-05", title="Indexer 환경", status="revision_pinned_runtime_pending", artifact="content/environment/indexer.manifest.json", ready="backend/frontend revision, source RPC, confirmation/cursor/rebuild 설정", pending="실 endpoint·DB/object store·decoder registry 배포"),
    dict(id="E-06", title="운영 신뢰 bootstrap", status="template_ready_external_registration", artifact="content/environment/trust-bootstrap.template.json", ready="관리자/workload/release/recovery 역할과 증거 필드", pending="실제 인물·workload identity·KMS/HSM key reference의 2인 등록"),
    dict(id="E-07", title="개발·시험 데이터", status="prepared", artifact="content/environment/test-fixtures.manifest.json", ready="매장/메뉴/기기/사용자/장소/가격/환불/보상 fixture와 삭제 등급", pending="실 카페 정보는 동의 후 별도 tenant로 입력"),
]

source_paths = [
    "content/planning/remaining-work-list.md",
    "content/planning/decision-briefing.json",
    "content/planning/unified-adoption-plan.json",
    "content/specifications/preimplementation-contract-overlay.json",
    "content/specifications/selection-profile-contract.json",
]

checkpoint = {
    "date": DATE,
    "checkpointId": CHECKPOINT,
    "status": "implementation_entry_design_frozen",
    "implementationStarted": False,
    "runtimeVerified": False,
    "sqlApplied": False,
    "contractsDeployedByThisCheckpoint": False,
    "decisionAuthority": "user_requested_completion_of_the_20_pending_decisions_on_2026-09-20",
    "counts": {"selectedDecisions": len(decisions), "detailedProductAreas": len(products), "adoptedContractBundles": len(adoptions), "environmentPreparationItems": len(environment)},
    "sourceHashes": {p: sha256(ROOT / p) for p in source_paths},
    "externalBasis": [
        {"topic": "React Native", "url": "https://reactnative.dev/releases/", "usedFor": "0.87.x active baseline"},
        {"topic": "nRF Connect SDK", "url": "https://github.com/nrfconnect/sdk-nrf/blob/main/doc/nrf/releases_and_maturity/releases/release-notes-3.4.0.rst", "usedFor": "v3.4.0 tag, Zephyr 4.4, DTS partition direction"},
        {"topic": "OpenAI transcription", "url": "https://developers.openai.com/api/reference/cli/resources/audio/subresources/transcriptions/methods/create", "usedFor": "audio transcription endpoint and model"},
        {"topic": "OpenAI models", "url": "https://developers.openai.com/api/docs/models", "usedFor": "Responses API model selection"},
        {"topic": "NIST threshold cryptography", "url": "https://csrc.nist.gov/projects/threshold-cryptography", "usedFor": "no full-key reconstruction principle"},
        {"topic": "cb-mpc", "url": "https://github.com/coinbase/cb-mpc", "usedFor": "ECDSA DKG/sign/refresh capability boundary"},
    ],
    "decisions": [{**d, "status": "selected_for_design_baseline", "effectiveCheckpoint": CHECKPOINT} for d in decisions],
    "productDesigns": products,
    "contractAdoptions": [{**a, "status": "adopted_design_baseline", "runtimeActive": False} for a in adoptions],
    "environmentPreparation": environment,
    "residualConditions": [
        "NU-54V-DK revision, pin map, memory and secure-service capability require the actual board.",
        "OAuth, Kakao and OpenAI credentials must be registered outside the repository; only secret references belong here.",
        "All StableNet contract addresses except verified chain identity remain disabled until deployment evidence is registered.",
        "MPC, passkey BLE, FOTA, audio throughput, Chain Sounding and all application journeys are runtime-unverified.",
        "Commercial use with real assets, tokenized rights or traveler rental requires a current legal and provider-terms review.",
    ],
    "nextPhase": "repository_cleanup_checkpoint_then_wait_for_explicit_implementation_start",
}

dump(ROOT / "content/planning/design-freeze-checkpoint.json", checkpoint)

contract = {
    "date": DATE,
    "checkpointId": CHECKPOINT,
    "status": "canonical_design_baseline_adopted",
    "authority": "This overlay is the implementation-entry authority for selected values and joint contract rules. Existing catalog files remain the detailed field inventory until implementation migrations are authored.",
    "runtimeActive": False,
    "databaseApplied": False,
    "firmwareApplied": False,
    "chainApplied": False,
    "wireVersion": {"http": "v1-json-jcs", "ble": "v1-deterministic-cbor", "profile": "v1"},
    "bundles": checkpoint["contractAdoptions"],
    "globalInvariants": [
        "No raw mnemonic, private key, MPC share or signed transaction bytes in general logs, analytics or public readers.",
        "Same idempotency key and digest returns the original result; a different digest conflicts.",
        "Authority is checked at issuance, execution and protected-result release.",
        "Chain submission, canonical observation and business finality remain distinct states.",
        "Deletion, revocation, return and profile rollback cannot be undone by delayed workers or backup restore.",
        "Unregistered contract address, decoder, signer, board profile or provider is disabled by default.",
    ],
    "migrationPolicy": "Preserve the seven reference migrations. Add new expand migrations, dual-read/write observation, activation checkpoint and later cleanup migration; no in-place rewrite.",
    "activationGate": "Implementation, generated schemas, migrations, deployed profiles and runtime acceptance evidence must all pass before any bundle becomes runtime-active.",
}
dump(ROOT / "content/specifications/design-baseline-contract.json", contract)

dump(ROOT / "content/environment/toolchain-pins.json", {
    "date": DATE, "checkpointId": CHECKPOINT,
    "mobile": {"reactNative": "0.87.x", "node": ">=22", "android": "15+/API 35+", "ios": "18+", "primaryDevice": "Galaxy S25 Ultra"},
    "firmware": {"sdk": "nRF Connect SDK", "tag": "v3.4.0", "zephyr": "NCS manifest-pinned 4.4 line", "boardTarget": None, "boardTargetStatus": "awaiting_actual_NU-54V-DK_revision_and_project_board_commit"},
    "repositories": {
        "indexer-go": {"revision": "5baff647f3cfab935f73db1bbd742ad42317045f", "workingTreeAtInspection": "untracked_CLAUDE.md"},
        "indexer-frontend": {"revision": "e0f8100f94cf9c3862b02f4132fb542bdeae0ae9", "workingTreeAtInspection": "dirty_do_not_treat_as_revision_content"},
        "stable-poc-contract": {"revision": "5d9d6550ef572bd86a036fe92c730e4ff0a1ca94", "workingTreeAtInspection": "clean"},
        "poc-platform": {"revision": "fcad7900de5ceea5e3ef586774609ad00739b760", "workingTreeAtInspection": "dirty_do_not_treat_as_revision_content"},
    },
    "rule": "A dirty sibling working tree is inspection context only. Implementation must checkout the exact revision in an isolated clean worktree or record a new reviewed revision."
})

dump(ROOT / "content/environment/provider-accounts.template.json", {
    "status": "template_only_no_credentials", "secretPolicy": "Only secretRef identifiers are permitted in repository files.",
    "providers": [
        {"id": "google_oidc", "required": ["clientIdAndroid", "clientIdIos", "serverClientId", "redirectUris"], "secretRef": "secret://oauth/google/client-secret"},
        {"id": "apple_sign_in", "required": ["teamId", "serviceId", "bundleId", "redirectUris"], "secretRef": "secret://oauth/apple/signing-key"},
        {"id": "kakao_local", "required": ["appId", "allowedOrigins", "allowedBundleIds"], "secretRef": "secret://maps/kakao/rest-key"},
        {"id": "openai", "required": ["projectId", "regionPolicy", "monthlyBudget"], "modelAllowlist": ["gpt-4o-mini-transcribe", "gpt-5.6-luna"], "secretRef": "secret://ai/openai/project-key"},
        {"id": "mpc_participant_a", "required": ["workloadIdentity", "mtlsCert", "kmsKeyRef"], "secretRef": "secret://mpc/a"},
        {"id": "mpc_participant_b", "required": ["workloadIdentity", "mtlsCert", "kmsKeyRef"], "secretRef": "secret://mpc/b"},
        {"id": "mpc_recovery_c", "required": ["offlineCustodian", "sealedShareRef", "twoPersonPolicy"], "secretRef": "offline://mpc/recovery-c"},
    ]
})

dump(ROOT / "content/environment/stablenet-8283.manifest.json", {
    "environmentId": "stablenet-test-8283", "status": "chain_verified_contracts_disabled", "observedAt": DATE,
    "chain": {"rpc": "https://api.test.stablenet.network/", "explorer": "https://explorer.stablenet.network/", "chainId": 8283, "chainIdHex": "0x205b", "nativeCurrency": {"symbol": "WKRC", "decimals": 18}, "observedBlockHex": "0x13435dd"},
    "contracts": [
        {"id": "dummyUSDC", "address": None, "decimals": 6, "abiDigest": None, "codeHash": None, "deploymentBlock": None, "enabled": False},
        *[{"id": x, "address": None, "abiDigest": None, "codeHash": None, "deploymentBlock": None, "enabled": False} for x in ["entryPoint", "kernelFactory", "amm", "perpetual", "membershipSTO", "didStatus", "x402Facilitator"]],
    ],
    "rejectedCandidate": {"address": "0xEf6817fe73741A8F10088f9511c64b666a338A14", "reason": "eth_getCode returned 0x on the selected RPC at inspection time"},
    "activationRule": "enabled requires address, ABI digest, runtime code hash, deployment block, admin-role evidence and indexer decoder registration."
})

dump(ROOT / "content/environment/indexer.manifest.json", {
    "status": "revisions_pinned_runtime_not_deployed", "chainEnvironment": "stablenet-test-8283",
    "backend": {"repo": "https://github.com/0xmhha/indexer-go", "revision": "5baff647f3cfab935f73db1bbd742ad42317045f", "endpoint": None, "databaseSecretRef": "secret://indexer/postgres", "rpc": "https://api.test.stablenet.network/"},
    "frontend": {"repo": "https://github.com/0xmhha/indexer-frontend", "revision": "e0f8100f94cf9c3862b02f4132fb542bdeae0ae9", "endpoint": None},
    "policy": {"paymentConfirmations": 2, "startBlock": None, "activeDecoderGeneration": None, "cursorBinds": ["chainId", "deploymentRevision", "decoderGeneration", "filterDigest"], "rebuildMode": "new_generation_then_atomic_reader_switch"},
    "activationRule": "No startBlock or decoder is inferred. Register deployed contract manifest first, then backfill and compare completeness before serving product reads."
})

dump(ROOT / "content/environment/trust-bootstrap.template.json", {
    "status": "template_only_no_real_identity_or_key", "checkpointId": CHECKPOINT,
    "roles": ["security_admin", "release_approver", "privacy_operator", "finance_reconciler", "incident_commander", "recovery_custodian"],
    "workloads": ["api", "indexer", "mpc_a", "mpc_b", "ai_worker", "release_signer"],
    "recordFields": ["subjectId", "role", "publicKeyOrWorkloadIdentity", "issuer", "issuedAt", "expiresAt", "scope", "evidenceDigest", "revocationRef"],
    "bootstrap": ["verify independent out-of-band identity", "two approvers sign initial registry", "store registry and digest in separate protected locations", "test revoke and recovery before activation"],
    "prohibitions": ["no private key in this file", "no self-signed manifest as sole trust root", "no one-person emergency unlock"]
})

dump(ROOT / "content/environment/test-fixtures.manifest.json", {
    "status": "prepared_synthetic_only", "namespace": "df20260920",
    "fixtures": {
        "stores": [{"id": "store-demo-seoul-01", "synthetic": True, "timezone": "Asia/Seoul"}],
        "menus": [{"id": "menu-coffee-01", "priceKRW": 5000}, {"id": "menu-tea-01", "priceKRW": 4500}],
        "devices": [{"id": "nu-demo-01", "profile": "unmeasured", "rentalState": "available"}],
        "users": [{"id": "traveler-a", "walletMode": "import"}, {"id": "traveler-b", "walletMode": "new_with_recovery"}, {"id": "merchant-a", "role": "owner"}],
        "places": [{"id": "place-synthetic-01", "source": "synthetic", "lat": 37.5665, "lon": 126.9780}],
        "commerce": {"fxKRWPerUSD": "1400.00", "quoteTTLSeconds": 60, "refundCases": ["full", "partial", "reorg", "duplicate"], "stampThreshold": 10},
    },
    "classification": {"synthetic": "safe_to_reset", "provider_cache": "delete_by_provider_terms", "personal": "never_seed_into_shared_environment"}
})

env_readme = f"""# 구현 전 외부 환경 준비\n\n{DATE} · checkpoint `{CHECKPOINT}`\n\n이 디렉터리는 구현을 시작하지 않고도 준비할 수 있는 환경 계약만 담는다. 실제 비밀값은 저장하지 않으며 `secretRef`만 기록한다. 현재 사용자의 요청은 이 설계·준비 checkpoint까지이며 제품 코드, 계정 생성, 컨트랙트 배포, 서버 기동은 아직 시작하지 않는다.\n\n| 항목 | 상태 | 산출물 | 외부에서 남은 일 |\n|---|---|---|---|\n"""
for e in environment:
    env_readme += f"| {e['id']} · {e['title']} | {e['status']} | [{Path(e['artifact']).name}](../{e['artifact'].split('content/',1)[1]}) | {e['pending']} |\n"
env_readme += "\n`prepared`는 저장소 안의 템플릿·고정값·차단 규칙이 준비됐다는 뜻이다. 계정 발급, 실제 키 등록, 배포 주소, 실기 검증이 끝났다는 뜻은 아니다.\n"
(ROOT / "content/environment/README.md").write_text(env_readme)

md = f"""# 구현 진입 설계 동결 checkpoint\n\n{DATE} · `{CHECKPOINT}` · **20개 선택 완료 / 10개 제품 상세 설계 완료 / 8개 계약 묶음 설계 기준 채택 / 7개 환경 준비**\n\n사용자는 미선택 결정을 끝내고 제품 구현 직전까지 설계를 진행하도록 요청했다. 이 문서는 그 요청에 따른 권장 선택을 구현 진입 기준으로 확정한다. 기존 문서에서 `미선택`으로 남은 기록은 당시 상태를 보존하며, 이후 구현 판단에는 이 checkpoint가 우선한다. 제품 코드·DB migration·펌웨어 build·계정 생성·컨트랙트 배포는 수행하지 않았다.\n\n## 1. 정책·기술 결정 20개\n\n| 결정 | 채택값 | 실패·축소 규칙 |\n|---|---|---|\n"""
for d in decisions:
    md += f"| **{d['id']} · {d['title']}** | {d['selection']} | {d['fallback']} |\n"

md += "\n## 2. 제품별 상세 설계 10개 영역\n\n"
for p in products:
    md += f"### {p['id']} · {p['title']}\n\n- **범위:** {p['scope']}\n- **구성:** " + ", ".join(p["modules"]) + "\n- **계약:** " + ", ".join(p["interfaces"]) + f"\n- **저장:** {p['persistence']}\n- **상태:** {p['state']}\n- **장애 규칙:** {p['failures']}\n- **구현 수용 증거:** " + "; ".join(p["acceptance"]) + "\n\n"

md += "## 3. 기준 계약 채택 8개 묶음\n\n이 채택은 **구현 진입용 설계 기준**이다. wire schema 생성, SQL 적용, firmware/profile 활성화, 체인 배포, runtime 검증은 아직 0이다. 상세 필드 목록은 기존 카탈로그를 유지하고, 충돌 시 [구조화 기준 계약](../specifications/design-baseline-contract.json)의 선택·불변조건을 우선한다.\n\n| 묶음 | 함께 채택한 범위 | 기준 규칙 | 보존할 금지 조건 |\n|---|---|---|---|\n"
for a in adoptions:
    md += f"| **{a['id']} · {a['title']}** | {a['adopts']} | {a['rules']} | {a['preserve']} |\n"

md += "\n## 4. 외부 환경 준비 7개\n\n| 항목 | 준비 상태 | 저장소 산출물 | 남은 외부 행위 |\n|---|---|---|---|\n"
for e in environment:
    rel = "../" + e["artifact"].split("content/", 1)[1]
    md += f"| **{e['id']} · {e['title']}** | {e['status']} | [{Path(e['artifact']).name}]({rel}) | {e['pending']} |\n"

md += """\n## 동결 후 남은 조건\n\n- 실제 NU-54V-DK의 revision·pin·flash/RAM·secure service를 측정해야 펌웨어 layout을 활성화할 수 있다.\n- Google·Apple·Kakao·OpenAI 계정과 secret은 저장소 밖에서 등록해야 한다.\n- StableNet 계약 주소는 현재 모두 비활성이다. 배포 주소, code hash, ABI digest, deployment block, 관리자 권한 증거가 있어야 활성화한다.\n- MPC, BLE passkey, 오디오, FOTA, 거리 측정과 12개 통합 여정은 실제 실행되지 않았다.\n- 실자산·실권리·상용 대여는 최신 법률과 공급자 약관을 별도로 검토해야 한다.\n\n다음 동작은 저장소 정리 checkpoint다. 정리 뒤에는 사용자의 명시적 구현 착수 지시 전까지 제품 구현을 시작하지 않는다.\n"""
(ROOT / "content/planning/design-freeze-checkpoint.md").write_text(md)

contract_md = f"""# 구현 진입 기준 계약\n\n{DATE} · `{CHECKPOINT}` · **설계 기준 채택 / runtime 비활성**\n\n이 문서는 [설계 동결 checkpoint](../planning/design-freeze-checkpoint.md)의 8개 묶음을 공동 채택한 기준 overlay다. 기존 API 110개, BLE 34개, 정책 60개, 7개 reference migration의 상세 inventory는 유지한다. 구현 중 schema·migration·generated client를 만들 때 이 overlay의 선택과 불변조건을 우선한다.\n\n## 공통 wire\n\n- HTTP: `v1-json-jcs`\n- BLE: `v1-deterministic-cbor`\n- profile: `v1`\n- 같은 idempotency key와 payload digest는 원 결과, 다른 digest는 conflict\n- chain 제출, canonical 관측, 업무 확정은 서로 다른 상태\n\n## 채택 묶음\n\n| ID | 범위 | 채택 상태 |\n|---|---|---|\n"""
for a in adoptions:
    contract_md += f"| {a['id']} · {a['title']} | {a['rules']} | `adopted_design_baseline` |\n"
contract_md += "\n## 활성화 gate\n\n각 묶음은 생성 schema, 후속 migration, 실제 profile, 배포 manifest와 runtime 수용 증거가 모두 통과하기 전까지 실행 비활성이다. 기존 migration을 수정하지 않고 expand→dual observation→activation→cleanup 순서로 새 migration을 추가한다.\n"
(ROOT / "content/specifications/design-baseline-contract.md").write_text(contract_md)

print(json.dumps({
    "checkpoint": CHECKPOINT,
    "decisions": len(decisions),
    "products": len(products),
    "adoptions": len(adoptions),
    "environments": len(environment),
}, ensure_ascii=False))

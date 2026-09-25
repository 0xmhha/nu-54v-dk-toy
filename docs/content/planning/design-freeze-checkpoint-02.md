# 설계 동결 DF-20260925-02

2026-09-25 · `DF-20260925-02` · `DF-20260920-01`를 대체한다 · 문서 기준선, 구현 이전

이 문서는 `design-freeze-checkpoint-02.json`을 `build_design_freeze_02.py`로 렌더링한 결과다. 값을 바꿀 때는 JSON을 고치고 다시 렌더링한다. 제품 문서와 프로토콜 문서는 값을 다시 적지 않고 `[N03]`처럼 결정 ID를 인용한다.

`DF-20260920-01`와 그 생성 산출물은 바이트 단위로 그대로 두고, 우선순위는 저장소 읽기 순서와 아래 배너로 바뀐다. baseCommit 이전 문서 전체는 충돌하는 부분에서 이 동결에 따른다.

## 1. 새 결정

| ID | 제목 | 결정 | 인용해야 하는 문서 | 해소하는 충돌 |
|---|---|---|---|---|
| **N01** | 제품 정의 기준 | 결제 HW 지갑 방향(2026-09-23 조사)이 제품 정의를 정한다. 만드는 제품은 P01·P04·P05(Foundry 스크립트)·P06·P07(최소)·P10(공유 EIP-712)이고 P02·P03·P08·P09는 설계만 한다. | [p01/plan.md](../products/p01/plan.md), [p04/plan.md](../products/p04/plan.md), [p05/plan.md](../products/p05/plan.md), [p06/plan.md](../products/p06/plan.md), [p07/plan.md](../products/p07/plan.md), [p10/plan.md](../products/p10/plan.md) | F1 |
| **N02** | 외부 secure element | W8부터 키는 외부 SE로 감싼다. 그 전까지 TF-M secure partition 봉인은 testnet waiver로 기록한다. | [p01/srs.md](../products/p01/srs.md), [p01/design.md](../products/p01/design.md) | F2 |
| **N03** | 게이트와 컷 순서 | W4 증거 게이트(BR-01..BR-08, 소프트웨어 서명 PaymentSettled, SE 데이터시트), W6 보드 내장 키 실기 end-to-end(첫 실결제 게이트이자 컷 트리거), W8 외부 SE. W6 실패 시 컷 순서는 외부 SE, P07, 지연 출금 시연이며 기기 가맹점 표시는 자르지 않는다. | [planning/product-worklist-and-12week-wbs-02.md](product-worklist-and-12week-wbs-02.md), [p01/plan.md](../products/p01/plan.md), [p04/plan.md](../products/p04/plan.md), [p06/plan.md](../products/p06/plan.md) | F3 |
| **N04** | 기기 서명 타입 | 기기는 PaymentAuthorization {chainId, contract, merchant, payout, token, amount, orderId, nonce, expiry}와 LimitChange 두 EIP-712 타입만 서명하고 나머지(트랜잭션, approve, Permit)는 거절한다. | [protocol/payment-protocol.md](../specifications/protocol/payment-protocol.md), [p01/srs.md](../products/p01/srs.md), [p06/srs.md](../products/p06/srs.md), [p10/srs.md](../products/p10/srs.md) | - |
| **N05** | 가맹점 신뢰 | 운영자(P05)가 EIP-712 MerchantAttestation(이름, 서명 주소, payout, attestationValidity)을 발급하고 기기는 provisioning 때 한 번 기록한 운영자 주소로 검증한다. 온체인 registry가 최종 권한이다. | [protocol/payment-protocol.md](../specifications/protocol/payment-protocol.md), [p01/design.md](../products/p01/design.md), [p05/design.md](../products/p05/design.md), [p06/design.md](../products/p06/design.md) | - |
| **N06** | 기기 시간 기준 | 대여 셋업(device.reset 후) 때 운영자가 서명한 TimeAnchor를 기록하고 RTC로 이어 간다. 새 anchor는 운영자 서명이 있고 이전 값보다 늦어야 하며 전원이 끊기면 무효다. 이번 사이클은 P05 provisioning 스크립트가 BLE 셋업 세션으로 전달하고, 목표 경로인 P02 설정 앱은 설계만 한다. | [protocol/payment-protocol.md](../specifications/protocol/payment-protocol.md), [p01/design.md](../products/p01/design.md), [p05/design.md](../products/p05/design.md) | - |
| **N07** | 정산 컨트랙트 | 운영자가 depositFor(deviceAddress, amount, withdrawAddress)로 사전 예치한다. 컨트랙트는 registry, payout 일치, 건당·일일 한도, unordered nonce, expiry, (merchant, orderId) 유일성(ORDER_ALREADY_PAID)을 검사하고 가맹점 잔액으로 옮긴다. 출금과 closeAccount는 등록 주소로만 지급한다. | [p06/srs.md](../products/p06/srs.md), [p06/design.md](../products/p06/design.md) | - |
| **N08** | paid 판정 | finalized PaymentSettled(merchant, orderId) 이벤트가 paid를 정한다. 같은 orderId의 ORDER_ALREADY_PAID revert는 성공으로 본다. P07은 판정 경로에 없다. | [protocol/payment-protocol.md](../specifications/protocol/payment-protocol.md), [p04/srs.md](../products/p04/srs.md), [p06/srs.md](../products/p06/srs.md), [p07/srs.md](../products/p07/srs.md) | - |
| **N09** | 전송 계층 | BLE GATT만 규범 전송이다(키오스크 central, 기기 peripheral, LE Secure Connections Numeric Comparison, Passkey fallback). NFC는 BLE 주소 handover와 LESC OOB 데이터에만 쓰고 BLE scan이 대체 경로다. MTU 초과 메시지는 length/sequence/digest로 재조립한다. USB CDC는 시험 harness일 뿐 게이트 증거가 아니다. | [protocol/payment-protocol.md](../specifications/protocol/payment-protocol.md), [p01/design.md](../products/p01/design.md), [p04/design.md](../products/p04/design.md) | - |
| **N10** | 키오스크 제출 | 키오스크가 가스를 내고 kioskMinGasBalance 미만이면 busy다. 제출 전 eth_call로 시뮬레이션하고, 요청 전달 완료부터 10 s hard timeout 뒤 Checking 상태로 재결제를 막는다. 같은 서명 재전송 외 자동 재시도는 없다. | [p04/srs.md](../products/p04/srs.md), [p04/design.md](../products/p04/design.md) | - |
| **N11** | 키·계정 수명주기 | 키는 대여 시 TRNG로 기기 안에서 만들고 내보내지 않으며 백업이 없다. PIN은 대여 때 정하고 LimitChange에서 다시 확인한다. 반납 시 운영자가 closeAccount를 호출하고 device.reset이 키를 지운다. | [p01/srs.md](../products/p01/srs.md), [p05/srs.md](../products/p05/srs.md) | - |
| **N12** | 펌웨어 갱신 | MCUboot 유선 serial recovery만 쓴다. BLE DFU는 보류하고 서명 키는 오프라인에 둔다. | [p01/design.md](../products/p01/design.md) | - |
| **N13** | 정산 파라미터 | perPaymentCap, dailyCap, withdrawalDelay, payoutChangeDelay, authorizationExpiry 값은 이 레지스터의 parameters 한 곳에만 둔다. | [p06/design.md](../products/p06/design.md) | - |
| **N14** | 보안 waiver | anti-exfil(req 10), 의존성 pinning(req 12), 정품 기기 attestation(req 13)은 설계만 하는 waiver다. TF-M 봉인은 SE 도입 전까지 waiver다. 개인정보는 수집하지 않는다(req 14). | [p01/srs.md](../products/p01/srs.md), [p10/srs.md](../products/p10/srs.md) | - |
| **N15** | 역할 분담 | 3명이 12주 동안 만든다. 사용자는 P06과 P10, role A는 P01, role B는 P04·P05·P07을 맡는다. 이름은 WBS 담당자 열에만 적는다. | [planning/product-worklist-and-12week-wbs-02.md](product-worklist-and-12week-wbs-02.md) | F1 |
| **N16** | 증거와 redaction | 원본 증거는 git-ignored 경로에 두고 docs/content에는 줄인 해시·주소와 sha256: checksum만 둔다. 시험 전용 EOA만 쓴다. | [acceptance/week12-log.md](../acceptance/week12-log.md), [p10/design.md](../products/p10/design.md) | - |
| **N17** | 환불 범위 | refund는 이번 사이클 범위 밖이다. 3주차 메모에 적은 협력 카페 시연의 refund 단계는 철회한다. | [p04/srs.md](../products/p04/srs.md), [p06/srs.md](../products/p06/srs.md) | - |
| **N18** | 12주 수용 기준 | W12-01..W12-12(기능 7, 보안 5)로 판정한다. W12-05 거절 시연은 인터뷰에서 합의한 5종(ATTESTATION_EXPIRED, MERCHANT_REVOKED, OVER_CAP, NONCE_REPLAYED, MERCHANT_FORGED)에 기기 거절 3종(USER_REJECTED, UNSUPPORTED_TYPE, TIME_ANCHOR_MISSING)을 더한 8종이다. P07 영수증 항목은 P07이 컷되지 않았을 때만 적용하고, 컷되면 키오스크가 PaymentSettled를 직접 읽은 기록으로 대신한다. | [acceptance/week12-log.md](../acceptance/week12-log.md), [p10/plan.md](../products/p10/plan.md) | - |
| **N19** | P07 최소 indexer | P07은 PaymentSettled 이벤트를 모아 영수증 조회를 제공하는 최소 indexer다. paid 판정에는 쓰지 않는다. | [p07/plan.md](../products/p07/plan.md), [p07/srs.md](../products/p07/srs.md) | - |
| **N20** | P05 운영 도구 | P05는 백오피스 대신 Foundry 스크립트로 가맹점 등록·attestation 발급·depositFor·closeAccount·TimeAnchor 발급을 수행한다. | [p05/plan.md](../products/p05/plan.md) | - |
| **N21** | P10 공유 타입 | P10은 EIP-712 타입 정의와 시험 벡터(eip712-vectors.json)를 펌웨어·키오스크·컨트랙트 시험이 함께 쓰도록 관리한다. | [protocol/payment-protocol.md](../specifications/protocol/payment-protocol.md), [p10/design.md](../products/p10/design.md) | - |
| **N22** | 거절 코드 | 제출 전 거절은 refused로 보고하며 코드는 ATTESTATION_EXPIRED(기기), MERCHANT_REVOKED(컨트랙트), OVER_CAP(컨트랙트), NONCE_REPLAYED(컨트랙트), MERCHANT_FORGED(기기와 컨트랙트)다. 제출 후 status=0은 failed와 tx hash로 보고한다. 기기만 내는 코드는 USER_REJECTED, UNSUPPORTED_TYPE, TIME_ANCHOR_MISSING, TIMEOUT, BAD_FRAME이며 앞의 셋은 W12-05에서 시연하고 뒤의 둘은 시험으로만 확인한다. | [protocol/payment-protocol.md](../specifications/protocol/payment-protocol.md), [p04/srs.md](../products/p04/srs.md), [p01/srs.md](../products/p01/srs.md) | - |

## 2. DF-20260920-01 결정의 처분

| ID | 처분 | 이전 값 | 새 값 | 이유 |
|---|---|---|---|---|
| **D01** | 변경 | RN 0.87.x 유저 앱·키오스크, Google·Apple 로그인 | 이번 사이클은 P04 키오스크(RN, Android 태블릿)만 만든다. 유저 앱(P02)과 소셜 로그인은 설계만 한다 [N01]. | 결제 HW 지갑 방향으로 제품 정의 기준이 바뀌었다. |
| **D02** | 변경 | HTTP JSON+JCS, BLE deterministic CBOR, v1 envelope, PostgreSQL 원장 | BLE deterministic CBOR는 유지하고 결제 경로의 envelope는 payment-protocol.md가 정한다. 서버·PostgreSQL 원장은 이번 사이클 범위 밖이다 [N09]. | 결제 판정 권한이 컨트랙트 이벤트로 옮겨가 업무 서버가 필요 없다. |
| **D03** | 변경 | BIP-39 신규 생성·import, TF-M 비반출 키 | 대여 시 TRNG로 기기 안에서 키를 만들고 내보내지 않으며 백업·import가 없다. SE 도입 전까지 TF-M 봉인(waiver), W8부터 SE [N02][N11]. | 대여 기기에서 사용자 시드 백업·import는 유출 면만 늘린다. |
| **D04** | 철회 | cb-mpc 2-of-3 Cloud Wallet | P03 Cloud MPC는 이번 사이클에서 설계만 한다 [N01]. | 결제 서명은 기기 키 하나로 끝나며 MPC는 12주 범위 밖이다. |
| **D05** | 변경 | NCS v3.4.0/Zephyr 4.4, MCUboot+MCUmgr SMP/BLE FOTA | NCS v3.4.0/Zephyr 4.4는 유지한다. 펌웨어 갱신은 MCUboot 유선 serial recovery만 쓰고 BLE DFU는 보류한다 [N12]. | BLE DFU는 서명 키 운영과 공격면을 늘린다. |
| **D06** | 철회 | CTAP 2.2 패스키 | 패스키는 이번 사이클 범위 밖이다 [N01]. | 결제 서명 외 기능을 뺐다. |
| **D07** | 철회 | 녹음·찾기 | 녹음·찾기는 이번 사이클 범위 밖이다 [N01]. | 결제 서명 외 기능을 뺐다. |
| **D08** | 변경 | KRW 기준가·signed 환율, quote TTL 60초, 2 block+Indexer canonical 후 paid, 환불 | 금액은 토큰 단위로 표시·서명한다. finalized PaymentSettled 이벤트가 paid를 정하고 환불은 범위 밖이다 [N08][N17]. | StableNet 8283은 1초 블록에서 finalized==latest이며 P07은 판정 경로에 두지 않는다. |
| **D09** | 철회 | 스탬프·혜택 | 스탬프·혜택은 이번 사이클 범위 밖이다 [N01]. | 결제 서명 외 기능을 뺐다. |
| **D10** | 유지 | StableNet testnet 8283, RPC, WKRC 18자리, dummy USDC 6자리 | 그대로 유지한다. 배포 주소는 manifest가 있을 때만 활성화한다. | 변경할 근거가 없다. |
| **D11** | 철회 | EOA 후 Kernel/ERC-4337 스마트 계정 | 기기는 EIP-712 두 타입만 서명하고 트랜잭션은 키오스크가 낸다. 스마트 계정은 범위 밖이다 [N04]. | 사전 예치 컨트랙트가 계정 역할을 대신한다. |
| **D12** | 철회 | AMM swap·LP | P08은 설계만 한다 [N01]. | 결제 서명 외 기능을 뺐다. |
| **D13** | 철회 | Perpetual | P08은 설계만 한다 [N01]. | 결제 서명 외 기능을 뺐다. |
| **D14** | 철회 | CafePass STO | 범위 밖이다 [N01]. | 결제 서명 외 기능을 뺐다. |
| **D15** | 철회 | DID/VC | 범위 밖이다 [N01]. | 결제 서명 외 기능을 뺐다. |
| **D16** | 철회 | x402 유료 자원 | 범위 밖이다 [N01]. | 결제 서명 외 기능을 뺐다. |
| **D17** | 철회 | 위치·후기(Kakao Local) | P09는 설계만 한다 [N01]. | 결제 서명 외 기능을 뺐다. |
| **D18** | 철회 | 전사·AI 추천 | P09는 설계만 한다 [N01]. | 결제 서명 외 기능을 뺐다. |
| **D19** | 변경 | 비체인 API p95 1초, 제출 UI 2초, 체인 확정 60초, RPO/RTO | 요청 전달 완료부터 10초 hard timeout 하나만 확정한다. p95 수치는 W5~6에 정하고 서버 SLO는 범위 밖이다 [N10]. | 서버가 없고 판정은 finalized 이벤트로 끝난다. |
| **RR-DEC-01** | 철회 | 신규 여행 EOA 암호화 복구 package | 키 백업이 없다. 반납 시 closeAccount 후 device.reset이며 늦은 입금은 등록된 출금 주소로 나간다 [N11]. | 기기 키를 사용자 자산 보관에 쓰지 않는다. |

## 3. 파라미터

값은 여기 한 곳에만 둔다. 다른 문서는 [N13]·[N10]·[N05]를 인용한다.

| 이름 | 값 | 단위 | 범위 | 근거 |
|---|---:|---|---|---|
| `perPaymentCap` | 50 | dUSDC | 0 < perPaymentCap <= dailyCap | 카페 한 번 결제 상한. 서명 하나가 새도 손실을 50 dUSDC로 묶는다. |
| `dailyCap` | 200 | dUSDC | dailyCap >= perPaymentCap | 하루 여러 번 결제를 허용하되 키 유출 시 하루 손실을 예치금보다 작게 둔다. |
| `withdrawalDelay` | 3600 | s | 600 <= withdrawalDelay <= 604800 | 분실 신고 후 운영자가 closeAccount를 호출할 시간을 주면서 12주 시연 한 세션 안에서 지연 출금을 보일 수 있다. |
| `payoutChangeDelay` | 86400 | s | payoutChangeDelay >= 3600 | attestationValidity와 같게 두어 바뀌기 전 payout을 담은 attestation이 모두 만료된 뒤 새 payout이 효력을 갖는다. |
| `authorizationExpiry` | 120 | s | 30 <= authorizationExpiry <= 300 | 10 s 키오스크 timeout과 같은 서명 재전송 여유를 덮고 오래된 서명의 재사용 창을 2분으로 제한한다. |
| `attestationValidity` | 86400 | s | fixed 86400 | 인터뷰에서 매일 재발급으로 확정했다. |
| `kioskMinGasBalance` | 215 | WKRC | value > 0 | 추정값. 결제 1건 약 7.14 WKRC(150,000 gas x 47,600 gwei)에 W12 연속 20회와 재전송 여유 10회를 곱했다. W4 게이트에서 실측으로 다시 정한다. |

## 4. 제품 범위

| DF-01 ID | 제품 | 이번 사이클 |
|---|---|---|
| B-01 | P01 | 만든다 |
| B-02 | P02 | 설계만 한다 |
| B-03 | P03 | 설계만 한다 |
| B-04 | P04 | 만든다 |
| B-05 | P05 | 만든다 |
| B-06 | P06 | 만든다 |
| B-07 | P07 | 만든다 |
| B-08 | P08 | 설계만 한다 |
| B-09 | P09 | 설계만 한다 |
| B-10 | P10 | 만든다 |

## 5. 우선순위와 배너

다음 파일의 읽기 순서에서 이 동결이 이전 동결보다 먼저 나온다.

- [REPOSITORY-CHECKPOINT.md](../../../REPOSITORY-CHECKPOINT.md)
- [docs/README.md](../../README.md)

다음 문서는 첫 줄에 대체 배너가 있고 나머지는 baseCommit 원문과 같다.

- [stablenet-testnet-baseline.md](../stablenet-testnet-baseline.md) `sha256:14a380b78853…`
- [three-person-delivery-plan.md](../three-person-delivery-plan.md) `sha256:afd0a941ef22…`
- [week-03-maker-team-meeting-memo.md](../week-03-maker-team-meeting-memo.md) `sha256:831b48c6c639…`
- [twelve-week-completion-scope-v3.md](../twelve-week-completion-scope-v3.md) `sha256:5d205e2447e3…`
- [planning/product-wbs-overview.md](product-wbs-overview.md) `sha256:bc04393aaf77…`
- [payment-integration-workplan.md](../payment-integration-workplan.md) `sha256:f6fd1b044e34…`
- [payment-platform-scope-v2.md](../payment-platform-scope-v2.md) `sha256:0c7744145896…`
- [poc-platform-reuse-plan.md](../poc-platform-reuse-plan.md) `sha256:f268ca39ea44…`
- [stable-contract-reuse-plan.md](../stable-contract-reuse-plan.md) `sha256:68bfecc19f49…`
- [indexer-integration-and-test-token.md](../indexer-integration-and-test-token.md) `sha256:2685ac84bf0f…`

## 6. 검증 범위

- baseCommit: `e2b15d92c8bb`
- stale literal: SL-01(가스는 키오스크가 낸다 [N10]), SL-02(제품 정의 기준이 바뀌었다 [N01]), SL-03(finalized 이벤트가 paid를 정한다 [N08]), SL-04(환불은 범위 밖이다 [N17])
- 예외 경로: DF-01 산출물과 입력, `docs/content/planning/seeds/`, `docs/design-history/`, `docs/content/planning/fixtures/df02/`
- redaction 예외: `specifications/protocol/eip712-vectors.json`(public test-only EOA, deterministic vector)
- waiver: req 10 anti-exfil, req 12 dependency pinning, req 13 genuine-device attestation, TF-M key sealing until the external SE lands

## 7. 사이클 중 변경

| 날짜 | 결정 | 변경 | 이유 | 유지한 합의 | 비용 |
|---|---|---|---|---|---|
| 2026-09-25 | [N18], [N22] | W12-05 거절 시연을 5종에서 8종(합의 5종 + 기기 3종)으로 늘렸다. | 프로토콜 설계에서 기기 거절 코드가 새로 생겼는데 수용 기준이 따라가지 않았다. 세 코드는 버튼 승인, 서명 타입 제한, 시간 기준이라는 보안 약속을 직접 증명한다. | 인터뷰에서 합의한 5종은 그대로 두고 별도 묶음으로 기록한다. | 시연 준비 1일 추가(WBS2-P04-03), 전원 재투입과 재-anchor 절차는 week12-log 3.3절 runbook으로 고정한다. |

검증: `python3 docs/content/planning/validate_design_freeze_02.py --check <group>` 와 `--self-test`.

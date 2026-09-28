# 설계 동결 DF-20260925-02

2026-09-25 · `DF-20260925-02` · `DF-20260920-01`를 대체한다 · 문서 기준선, 구현 이전

이 문서는 `design-freeze-checkpoint-02.json`을 `build_design_freeze_02.py`로 렌더링한 결과다. 값을 바꿀 때는 JSON을 고치고 다시 렌더링한다. 제품 문서와 프로토콜 문서는 값을 다시 적지 않고 `[N03]`처럼 결정 ID를 인용한다.

`DF-20260920-01`와 그 생성 산출물은 바이트 단위로 그대로 두고, 우선순위는 저장소 읽기 순서와 아래 배너로 바뀐다. baseCommit 이전 문서 전체는 충돌하는 부분에서 이 동결에 따른다.

## 1. 새 결정

| ID | 제목 | 결정 | 인용해야 하는 문서 | 해소하는 충돌 |
|---|---|---|---|---|
| **N01** | 제품 정의 기준 | 결제 HW 지갑 방향(2026-09-23 조사)이 제품 정의를 정한다. 만드는 제품은 P01·P02(대여자 폰 앱, 2026-09-28 추가)·P04·P05(Go 운영 코어와 opsctl)·P06·P07(최소)·P10(공유 EIP-712)이고 P03·P08·P09는 설계만 한다. | [p01/plan.md](../products/p01/plan.md), [p02/plan.md](../products/p02/plan.md), [p04/plan.md](../products/p04/plan.md), [p05/plan.md](../products/p05/plan.md), [p06/plan.md](../products/p06/plan.md), [p07/plan.md](../products/p07/plan.md), [p10/plan.md](../products/p10/plan.md) | F1 |
| **N02** | 외부 secure element | 외부 secure element는 이번 사이클에 쓰지 않는다(2026-09-29 컷). 기기 키는 nRF54L15의 TrustZone 위 TF-M secure partition에서 만들고 서명하며, secp256k1 키는 TF-M 보호 저장소(ITS, 암호화 사용)에 두고 CRACEN으로 서명한다. 키와 서명 경로는 non-secure 코드가 접근할 수 없고 버튼 토큰 경계(N24)를 지킨다. 물리 공격(칩 덤프, fault injection, 부채널)에 대한 방어가 SE보다 약한 점은 testnet waiver로 W12 수용 로그에 기록한다. W4에 확인한 SE050 자료는 다음 사이클을 위한 기록으로 남긴다. | [p01/srs.md](../products/p01/srs.md), [p01/design.md](../products/p01/design.md) | F2 |
| **N03** | 게이트와 컷 순서 | 게이트는 세 개다(W9 SE 게이트는 2026-09-29에 SE 컷으로 없앴다). W4(9/30) 증거 게이트: BR-01..BR-08 bring-up과 외부 SE 데이터시트의 secp256k1 확인. W6(10/14) 컨트랙트 게이트: 소프트웨어 서명으로 8283에 낸 finalized PaymentSettled. W7(10/21) 실결제 게이트: 보드 내장 키로 기기 버튼 승인부터 finalized PaymentSettled까지 end-to-end 1건(첫 실결제 게이트이자 컷 트리거). 동결일(9/25)이 W4 안이므로 모든 작업은 W4부터 배정한다. W7 실패 시 컷 대상은 P07(W12-06 대체 증거)이다. 외부 SE와 NFC handover는 이미 뺐고, 가맹점 정보 확인 화면은 자르지 않는다. | [planning/product-worklist-and-12week-wbs-02.md](product-worklist-and-12week-wbs-02.md), [p01/plan.md](../products/p01/plan.md), [p04/plan.md](../products/p04/plan.md), [p06/plan.md](../products/p06/plan.md) | F3 |
| **N04** | 기기 서명 타입 | 기기는 PaymentAuthorization {chainId, contract, merchant, payout, token, amount, orderId, nonce, expiry}와 LimitChange 두 EIP-712 타입만 서명하고 나머지(트랜잭션, approve, Permit)는 거절한다. | [protocol/payment-protocol.md](../specifications/protocol/payment-protocol.md), [p01/srs.md](../products/p01/srs.md), [p06/srs.md](../products/p06/srs.md), [p10/srs.md](../products/p10/srs.md) | - |
| **N05** | 가맹점 신뢰 | 운영자(P05)가 EIP-712 MerchantAttestation(이름, 서명 주소, payout, attestationValidity)을 발급한다. 기기는 셋업 때 기록한 운영자 주소로 attestation을 검증하고, authorization.merchant와 payout이 attestation과 같은지, authorization.chainId와 contract가 셋업 때 기록한 값과 같은지 확인한다. 온체인 registry가 최종 권한이다. payout 변경 요청 뒤 P05는 옛 payout attestation의 validUntil을 변경 효력 시각으로 잘라 발급한다. | [protocol/payment-protocol.md](../specifications/protocol/payment-protocol.md), [p01/design.md](../products/p01/design.md), [p05/design.md](../products/p05/design.md), [p06/design.md](../products/p06/design.md) | - |
| **N06** | 기기 시간 기준 | 셋업 세션에서 운영자가 서명한 TimeAnchor를 기록하고 RTC로 이어 간다. 새 anchor는 운영자 서명이 있고 이전 anchor보다 엄격히 늦어야 한다. 전원 손실을 포함해 RAM이 초기화되는 모든 reset 뒤에는 무효이며, 기기는 PROVISIONED_NO_ANCHOR 상태에서 셋업 세션을 다시 열어 키를 유지한 채 재-anchor 받는다. 시간 비교에는 anchorClockSkew를 허용한다. 이번 사이클은 P05 운영 도구(opsctl)가 BLE 셋업 세션으로 전달하고 목표 경로인 P02 설정 앱은 설계만 한다. | [protocol/payment-protocol.md](../specifications/protocol/payment-protocol.md), [p01/design.md](../products/p01/design.md), [p05/design.md](../products/p05/design.md) | - |
| **N07** | 정산 컨트랙트 | 운영자가 depositFor(deviceAddress, amount, withdrawAddress)로 사전 예치한다. settle 검사 순서는 domain(chainId, contract, token) → 주문 유일성(ORDER_ALREADY_PAID) → expiry → 서명자 계정(AccountInactive) → registry(MERCHANT_REVOKED) → payout(MERCHANT_FORGED) → nonce(NONCE_REPLAYED) → 한도(OVER_CAP) → 잔액(INSUFFICIENT_BALANCE)이다. LimitChange 한도가 0이면 register cap을 적용하고, 일일 한도는 첫 결제부터 시작하는 고정 24시간 창이다. 출금 요청은 운영자만 하고, 출금과 closeAccount는 등록 주소로만 지급한다. 닫힌 계정으로의 depositFor는 revert한다. | [p06/srs.md](../products/p06/srs.md), [p06/design.md](../products/p06/design.md) | - |
| **N08** | paid 판정 | finalized PaymentSettled(address indexed merchant, bytes32 indexed orderId, address indexed device, uint256 amount, uint256 nonce) 이벤트가 paid를 정한다. 키오스크는 시뮬레이션(제출 전, 또는 채굴된 트랜잭션이 status=0일 때의 재시뮬레이션)이 ORDER_ALREADY_PAID이면, finalized PaymentSettled(merchant, orderId)의 device·amount·nonce가 자기가 받은 서명과 같을 때만 approved로 본다. P07은 판정 경로에 없다. | [protocol/payment-protocol.md](../specifications/protocol/payment-protocol.md), [p04/srs.md](../products/p04/srs.md), [p06/srs.md](../products/p06/srs.md), [p07/srs.md](../products/p07/srs.md) | - |
| **N09** | 전송 계층 | BLE GATT만 규범 전송이다(기기 peripheral). 대여자 폰 앱과 운영자 도구는 LE Secure Connections Passkey Entry로 본딩하고, 키오스크는 페어링하지 않으며 결제 세션을 응용 계층 보안 채널로 보호한다(N27). 기기에 화면이 없어 Numeric Comparison은 쓰지 않는다. 이 보드는 NFC 핀을 I2C로 써서 NFC handover를 쓰지 않고 BLE scan으로 찾는다(N29). MTU 초과 메시지는 length/sequence/digest로 재조립한다. USB CDC는 시험 harness일 뿐 게이트 증거가 아니다. | [protocol/payment-protocol.md](../specifications/protocol/payment-protocol.md), [p01/design.md](../products/p01/design.md), [p04/design.md](../products/p04/design.md) | - |
| **N10** | 키오스크 제출 | 키오스크가 가스를 내고 잔액이 kioskMinGasBalance 미만이면 busy다. 제출 전 eth_call로 시뮬레이션한다. 요청 전달 완료부터 10 s 안에 기기 서명이 없으면 session.cancel을 보내고 주문을 취소한다(재결제 가능). 같은 10 s 안에 finalized 결과가 없고 이미 제출했으면 Checking으로 바꾸고 재결제를 막는다(타이머는 요청 전달 완료부터 하나다). 같은 서명 재전송 외 자동 재시도는 없다. | [p04/srs.md](../products/p04/srs.md), [p04/design.md](../products/p04/design.md) | - |
| **N11** | 키·계정 수명주기 | 키는 대여 셋업에서 TRNG로 기기 안에서 만들고 내보내지 않으며 백업이 없다. PIN은 셋업 때 기기 버튼으로 정하고(N28) LimitChange에서 다시 확인하며 pinMaxRetries를 넘으면 서명을 잠근다. 반납 시 운영자가 closeAccount를 호출하고 finalized 이벤트를 확인한 뒤 운영자 서명 device.reset 명령으로 키를 지운다. | [p01/srs.md](../products/p01/srs.md), [p05/srs.md](../products/p05/srs.md) | - |
| **N12** | 펌웨어 갱신 | MCUboot 유선 serial recovery만 쓴다. BLE DFU는 보류하고 서명 키는 오프라인에 둔다. | [p01/design.md](../products/p01/design.md) | - |
| **N13** | 정산 파라미터 | perPaymentCap, dailyCap, withdrawalDelay, payoutChangeDelay, authorizationExpiry 값은 이 레지스터의 parameters 한 곳에만 둔다. | [p06/design.md](../products/p06/design.md) | - |
| **N14** | 보안 waiver | anti-exfil(req 10), 의존성 pinning(req 12), 정품 기기 attestation(req 13)은 설계만 하는 waiver이며 설계안은 P01 design의 waiver 절에 둔다. 외부 SE를 컷했으므로 TF-M(TrustZone) 키 봉인은 이번 사이클 끝까지 waiver다. 개인정보는 수집하지 않는다(req 14). | [p01/srs.md](../products/p01/srs.md), [p10/srs.md](../products/p10/srs.md) | - |
| **N15** | 역할 분담 | 3명이 W4부터 W12까지 만든다. 사용자는 P06, P10, P07과 보드 bring-up, role A는 P01과 P02(폰 앱, 2026-09-28 추가), role B는 P04와 P05를 맡는다. 동결일 이후 일정에서 role B 부하가 넘쳐 P07을 사용자에게 옮겼다. role A의 W8–W12 부하가 가용 일수를 넘는 것은 담당자가 감수하기로 했다(N30). 이름은 WBS 담당자 열에만 적는다. | [planning/product-worklist-and-12week-wbs-02.md](product-worklist-and-12week-wbs-02.md) | F1 |
| **N16** | 증거와 redaction | 원본 증거는 git-ignored 경로에 두고 docs/content에는 줄인 해시·주소와 sha256: checksum만 둔다. 시험 전용 EOA만 쓴다. | [acceptance/week12-log.md](../acceptance/week12-log.md), [p10/design.md](../products/p10/design.md) | - |
| **N17** | 환불 범위 | refund는 이번 사이클 범위 밖이다. 3주차 메모에 적은 협력 카페 시연의 refund 단계는 철회한다. | [p04/srs.md](../products/p04/srs.md), [p06/srs.md](../products/p06/srs.md) | - |
| **N18** | 12주 수용 기준 | W12-01..W12-12(기능 7, 보안 5)로 판정한다. W12-05 거절 시연은 인터뷰에서 합의한 5종(ATTESTATION_EXPIRED, MERCHANT_REVOKED, OVER_CAP, NONCE_REPLAYED, MERCHANT_FORGED)에 기기 거절 3종(USER_REJECTED, UNSUPPORTED_TYPE, TIME_ANCHOR_MISSING)을 더한 8종이다. P07 영수증 항목은 P07이 컷되지 않았을 때만 적용하고, 컷되면 키오스크가 PaymentSettled를 직접 읽은 기록으로 대신한다. | [acceptance/week12-log.md](../acceptance/week12-log.md), [p10/plan.md](../products/p10/plan.md) | - |
| **N19** | P07 최소 indexer | P07은 PaymentSettled 이벤트를 모아 영수증 조회를 제공하는 최소 indexer다. paid 판정에는 쓰지 않는다. 담당은 사용자다 [N15]. | [p07/plan.md](../products/p07/plan.md), [p07/srs.md](../products/p07/srs.md) | - |
| **N20** | P05 운영 백오피스 | P05는 백오피스 전체(HTTP API, React UI, PostgreSQL)를 전제로 설계하고, 이번 사이클에는 그 아래의 Go 운영 코어와 CLI(opsctl)를 구현한다. 코어는 체인 호출, EIP-712 서명, keystore, 감사 기록을 맡아 가맹점 등록·attestation 발급·depositFor·closeAccount·TimeAnchor 발급·DeviceReset 서명을 수행하고, BLE 셋업 도구도 같은 Go 바이너리에 둔다. | [p05/plan.md](../products/p05/plan.md) | - |
| **N21** | P10 공유 타입 | P10은 EIP-712 타입 정의와 시험 벡터(eip712-vectors.json)를 펌웨어·키오스크·컨트랙트 시험이 함께 쓰도록 관리한다. | [protocol/payment-protocol.md](../specifications/protocol/payment-protocol.md), [p10/design.md](../products/p10/design.md) | - |
| **N22** | 거절 코드 | 기기 결제 흐름 안의 거절은 payment.result refused로, 틀 오류와 모르는 메시지는 error로 보낸다. 합의 5종은 ATTESTATION_EXPIRED(기기), MERCHANT_REVOKED(컨트랙트), OVER_CAP(컨트랙트), NONCE_REPLAYED(컨트랙트), MERCHANT_FORGED(기기와 컨트랙트)다. 기기 코드는 USER_REJECTED, TIME_ANCHOR_MISSING, TIMEOUT(payment.result refused)과 UNSUPPORTED_TYPE(error)이며, BAD_FRAME(error)은 기기와 키오스크가 모두 낸다. USER_REJECTED, UNSUPPORTED_TYPE, TIME_ANCHOR_MISSING은 W12-05에서 시연하고 TIMEOUT과 BAD_FRAME은 시험으로 확인한다. 컨트랙트 코드 EXPIRED, WRONG_DOMAIN, ACCOUNT_INACTIVE, INSUFFICIENT_BALANCE도 시험으로만 확인한다. 채굴된 트랜잭션이 status=0이면 재시뮬레이션한다. ORDER_ALREADY_PAID이면 [N08] 판정(이벤트 일치 approved, 불일치 refused)을 적용하고, 그 밖의 결과면 failed와 tx hash로 보고한다. | [protocol/payment-protocol.md](../specifications/protocol/payment-protocol.md), [p04/srs.md](../products/p04/srs.md), [p01/srs.md](../products/p01/srs.md) | - |
| **N23** | 셋업 명령 인증 | 운영자 주소 기록(setup.operator)은 기기가 UNPROVISIONED 상태이고 대여자가 기기 버튼으로 확인할 때만 받는다. 그 뒤 device.reset은 기기 주소와 nonce를 담은 운영자 서명 DeviceReset 명령으로만 받는다. 셋업 세션은 UNPROVISIONED 또는 PROVISIONED_NO_ANCHOR 상태에서만 열리며, LESC 페어링만으로는 셋업 권한이 생기지 않는다. | [protocol/payment-protocol.md](../specifications/protocol/payment-protocol.md), [p01/design.md](../products/p01/design.md), [p05/design.md](../products/p05/design.md) | - |
| **N24** | 서명 권한 경계 | secure partition의 sign_digest는 버튼 대기 직전에 등록한 digest와 purpose 한 건에만 버튼 토큰을 발급하고, LimitChange purpose는 secure 쪽 PIN 확인을 요구한다. 표시 내용과 서명 내용의 일치는 non-secure 코드가 무결하다는 가정 아래의 주장이며 trusted display는 범위 밖이다. | [p01/design.md](../products/p01/design.md), [p01/srs.md](../products/p01/srs.md) | - |
| **N25** | 기술 스택과 작업공간 | 펌웨어(P01)는 C(Zephyr/NCS), 컨트랙트(P06)는 Solidity와 Foundry, 체인 연동 서비스와 운영 도구(P05, P07)는 Go, 키오스크(P04)는 React Native와 TypeScript에 Kotlin·Swift native module(Turbo Module), 웹 프런트엔드는 React와 TypeScript, 스크립트와 코드 생성기는 Python, DB는 PostgreSQL(필요할 때만 MongoDB)이다. 성능이 결정적인 제품이 생기면 Rust를 검토한다. 공유 코드는 packages/에 도메인별(protocol, contracts-abi)로 두고 그 아래를 언어별로 나누며, 작업공간은 pnpm workspaces, go.work, uv를 쓴다. 로컬 검증은 docker compose sandbox(anvil chainId 8283, PostgreSQL)에서 하고, 컨테이너 이미지로 AWS 또는 Google Cloud에 배포한다. | [products/README.md](../../../products/README.md), [packages/README.md](../../../packages/README.md) | - |
| **N26** | 기기 화면 없음과 폰 확인 화면 | 이번 사이클의 기기에는 화면이 없다. 대여자 본인의 폰 앱(P02)이 기기와 본딩한 링크로 받은 confirm.show 필드(가맹점 이름, orderId, token, payout, amount)만 표시하고, 키오스크에서 받은 값은 표시하지 않는다. 폰 앱에는 승인 버튼이 없고 승인은 기기 버튼으로만 한다. 표시와 서명의 일치는 기기 non-secure 코드와 폰 앱이 무결하다는 가정 아래의 주장이며, 폰이 뚫린 경우는 한도가 손실을 제한한다. | [protocol/payment-protocol.md](../specifications/protocol/payment-protocol.md), [p01/design.md](../products/p01/design.md), [p02/design.md](../products/p02/design.md) | - |
| **N27** | 페어링과 결제 세션 보안 | 폰 앱과 운영자 도구는 기기별 6자리 passkey(대여 셋업 때 운영자 도구가 정해 기기에 기록하고 라벨 QR로 인쇄)로 LE Secure Connections Passkey Entry 본딩을 하며, 기기는 버튼을 길게 눌렀을 때만 페어링 모드를 연다. 키오스크는 카드 리더기처럼 페어링 없이 결제 세션만 연다. 결제 세션은 응용 계층 보안 채널로 보호한다: 키오스크 1회용 secp256k1 키를 가맹점 키로 서명(kioskKeySignature)하고, 기기가 attestation과 함께 확인한 뒤 ECDH와 HKDF-SHA256으로 session.key를 만들어 본문을 AES-GCM으로 암호화한다. 키오스크는 RSSI 기준값 이상인 기기에만 연결하며 기준값은 W8에 실측한다. 셋업 세션과 confirm.show는 본딩한 링크에서만 주고받는다. | [protocol/payment-protocol.md](../specifications/protocol/payment-protocol.md), [p01/design.md](../products/p01/design.md), [p04/design.md](../products/p04/design.md) | - |
| **N28** | PIN 입력 | 이번 단계의 PIN은 기기 버튼으로 입력하고 LED가 자릿수와 누를 버튼을 안내하므로 PIN이 BLE로 나가지 않는다. 폰 앱에서 PIN을 설정하는 기능은 다음 단계에 추가하며, 그때 PIN이 BLE를 거치는 위험을 위험 수용으로 기록한다. | [p01/srs.md](../products/p01/srs.md), [p02/srs.md](../products/p02/srs.md) | - |
| **N29** | NFC 미사용 | NU-54V-DK는 NFC 핀(P1.02/P1.03)을 Qwiic와 PMIC용 I2C로 쓰고 NFC 안테나가 없다. 이번 사이클은 NFC handover를 쓰지 않고 BLE scan으로 기기를 찾으며, WBS에서 NFC 작업을 뺀다. | [protocol/payment-protocol.md](../specifications/protocol/payment-protocol.md), [p01/design.md](../products/p01/design.md) | - |
| **N30** | W7 단계 적용과 role A 부하 | W7 실결제 게이트는 non-secure PSA 키, 개발 빌드 전용 고정 셋업, 평문 키오스크 링크로 통과한다. TF-M 키 서비스와 BLE 셋업 명령은 W8, 결제 세션 보안 채널은 기기 W9·키오스크 W10–W11에 넣는다. W7 게이트 증거에는 이 사실을 waiver로 적는다. role A가 P02를 함께 맡아 W8–W12 부하가 가용 일수를 넘는다. 2026-09-29 SE 컷으로 6일이 빠졌고, 남은 초과는 담당자가 감수한다. | [planning/product-worklist-and-12week-wbs-02.md](product-worklist-and-12week-wbs-02.md), [p01/plan.md](../products/p01/plan.md) | - |
| **N31** | 금액 표시 | 폰 앱은 금액을 토큰 단위(이번 사이클은 시험용 USDC)로 소수점 둘째 자리까지 표시하며, 셋째 자리 이하는 반올림하지 않고 버린다(예: 5.009999 USDC는 5.00 USDC). 그래서 화면 금액과 서명 금액은 결제 한 건에 0.01 USDC 미만만큼 다를 수 있고, 이 차이는 컨트랙트 한도 안에서 받아들인다. 토큰 기호와 소수 자릿수는 앱의 토큰 표에 둔다. 원화 환산 표시는 이번 사이클에 넣지 않는다. | [p02/srs.md](../products/p02/srs.md), [p02/design.md](../products/p02/design.md) | - |
| **N32** | 운영 키 보관 | 배포자, 운영자, registry 관리자처럼 PC에서 쓰는 키는 Foundry와 go-ethereum이 함께 읽는 암호화 JSON keystore 파일로 두고 secretRef(keystore 경로와 암호 환경 변수)로만 연다. 키오스크의 가스 키와 가맹점 서명 키는 Android Keystore가 secp256k1을 직접 지원하지 않는다는 가정 아래 Keystore의 AES 키로 원시 키를 감싸 앱 저장소에 둔다. 두 방식은 개발 중(W5)에 검증하고, 결과가 다르면 이 결정을 개정한다. 사용자 기기 키는 이 결정의 대상이 아니다(N11). | [p05/srs.md](../products/p05/srs.md), [p04/design.md](../products/p04/design.md) | - |

## 2. DF-20260920-01 결정의 처분

| ID | 처분 | 이전 값 | 새 값 | 이유 |
|---|---|---|---|---|
| **D01** | 변경 | RN 0.87.x 유저 앱·키오스크, Google·Apple 로그인 | 이번 사이클은 P04 키오스크(RN, Android 태블릿)만 만든다. 유저 앱(P02)과 소셜 로그인은 설계만 한다 [N01]. | 결제 HW 지갑 방향으로 제품 정의 기준이 바뀌었다. |
| **D02** | 변경 | HTTP JSON+JCS, BLE deterministic CBOR, v1 envelope, PostgreSQL 원장 | BLE deterministic CBOR는 유지하고 결제 경로의 envelope는 payment-protocol.md가 정한다. 서버·PostgreSQL 원장은 이번 사이클 범위 밖이다 [N09]. | 결제 판정 권한이 컨트랙트 이벤트로 옮겨가 업무 서버가 필요 없다. |
| **D03** | 변경 | BIP-39 신규 생성·import, TF-M 비반출 키 | 대여 셋업 때 TRNG로 기기 안에서 키를 만들고 내보내지 않으며 백업·import가 없다. SE 도입 전까지 TF-M 봉인(waiver), W9부터 SE로 감싼다 [N02][N11]. | 대여 기기에서 사용자 시드 백업·import는 유출 면만 늘린다. |
| **D04** | 철회 | cb-mpc 2-of-3 Cloud Wallet | P03 Cloud MPC는 이번 사이클에서 설계만 한다 [N01]. | 결제 서명은 기기 키 하나로 끝나며 MPC는 12주 범위 밖이다. |
| **D05** | 변경 | NCS v3.4.0/Zephyr 4.4, MCUboot+MCUmgr SMP/BLE FOTA | NCS v3.4.1/Zephyr 4.4.2로 올린다(보드 제조사 패키지 nu54v_dk와 같은 버전). 보드 타깃은 nu54v_dk/nrf54l15/cpuapp이고 플래시는 온보드 CMSIS-DAP과 pyOCD로 한다. 펌웨어 갱신은 MCUboot 유선 serial recovery만 쓰고 BLE DFU는 보류한다 [N12]. | BLE DFU는 서명 키 운영과 공격면을 늘린다. 제조사 board package가 NCS v3.4.1 기준이라 패치 버전을 맞췄다. |
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
| **D19** | 변경 | 비체인 API p95 1초, 제출 UI 2초, 체인 확정 60초, RPO/RTO | 요청 전달 완료부터 10초 hard timeout 하나만 확정한다. p95 수치는 W7 실결제 게이트 이후 측정으로 정하고 서버 SLO는 범위 밖이다 [N10]. | 서버가 없고 판정은 finalized 이벤트로 끝난다. |
| **RR-DEC-01** | 철회 | 신규 여행 EOA 암호화 복구 package | 키 백업이 없다. 반납 시 closeAccount 후 운영자 서명 device.reset이다. 닫힌 계정으로의 depositFor는 revert하고, 기기 EOA로 직접 보낸 토큰은 회수 대상이 아니다 [N07][N11]. | 기기 키를 사용자 자산 보관에 쓰지 않는다. |

## 3. 파라미터

값은 여기 한 곳에만 둔다. 다른 문서는 [N13]·[N10]·[N05]·[N06]·[N11]을 인용한다.

| 이름 | 값 | 단위 | 범위 | 근거 |
|---|---:|---|---|---|
| `perPaymentCap` | 50 | dUSDC | 0 < perPaymentCap <= dailyCap | 카페 한 번 결제 상한. 서명 하나가 새도 손실을 50 dUSDC로 묶는다. |
| `dailyCap` | 200 | dUSDC | dailyCap >= perPaymentCap | 하루 여러 번 결제를 허용하는 상한. 일일 창은 고정 24시간이라 임의의 24시간 동안 최악 손실은 min(예치금, 2 x dailyCap)이다. |
| `withdrawalDelay` | 3600 | s | 600 <= withdrawalDelay <= 604800 and withdrawalDelay >= authorizationExpiry | 출금 요청은 잔액을 바로 옮기지 않고 지연 뒤 실행 시점의 잔액 안에서 지급한다. 지연 동안 요청 전에 서명된 결제(최대 authorizationExpiry)가 정산될 수 있고, 운영자는 cancelWithdrawal로 오조작을 되돌릴 수 있다. 12주 시연 한 세션 안에서 지연 출금을 보일 수 있는 길이로 정했다. |
| `payoutChangeDelay` | 86400 | s | payoutChangeDelay >= attestationValidity | payout 변경이 이벤트로 공개된 뒤 효력이 생기기까지의 창이다. 가맹점은 이 기간에 자기 payout 변경을 확인해 이의를 제기하고, registry admin은 cancelPayoutChange로 되돌린다(admin 키 자체의 탈취는 막지 못하며 위험으로 기록한다). P05는 변경 요청 뒤 옛 payout attestation의 validUntil을 효력 시각으로 잘라 발급하므로, 효력 시각 뒤 옛 payout으로 나온 서명은 anchorClockSkew 이내뿐이고 컨트랙트가 MERCHANT_FORGED로 막는다 [N05]. |
| `authorizationExpiry` | 120 | s | 30 <= authorizationExpiry <= 300 | 10 s 키오스크 timeout과 같은 서명 재전송 여유를 덮고 오래된 서명의 재사용 창을 2분으로 제한한다. |
| `attestationValidity` | 86400 | s | fixed 86400 | 인터뷰에서 매일 재발급으로 확정했다. |
| `kioskMinGasBalance` | 15 | WKRC | value > 0 | busy 하한. 결제 1건 약 7.14 WKRC(150,000 gas x 47,600 gwei)에 같은 서명 재전송 1건을 더한 2건분을 올렸다. W12 시작 잔액은 이 값과 별개로 week12-log 1절에 적는다. W4 이후 실측으로 다시 정한다. |
| `pinMaxRetries` | 5 | count | 3 <= pinMaxRetries <= 10 | 오입력 여유를 두면서 추측 공격을 막는다. 넘으면 서명을 잠그고 운영자 반납 절차(closeAccount 후 DeviceReset)로만 풀린다 [N11]. |
| `anchorClockSkew` | 60 | s | 0 < anchorClockSkew <= 300 | RTC 오차(수십 ppm, 대여 기간 수일에 수십 초)와 블록 시각 차이를 덮는다. attestation 유효 기간 비교에만 쓴다 [N06]. |

## 4. 제품 범위

| DF-01 ID | 제품 | 이번 사이클 |
|---|---|---|
| B-01 | P01 | 만든다 |
| B-02 | P02 | 만든다 |
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

- [stablenet-testnet-baseline.md](../stablenet-testnet-baseline.md) `sha256:14a380b78853c046c4dd0a35dbe8d5ed7a13a6fc0271097f744f359fbf890c17`
- [three-person-delivery-plan.md](../three-person-delivery-plan.md) `sha256:afd0a941ef22650babed88d13922e81376cbf2bc3f674892f65f0f701f152d72`
- [week-03-maker-team-meeting-memo.md](../week-03-maker-team-meeting-memo.md) `sha256:831b48c6c639c9c1b72052a1d36ba3427503193623f6fdc9ab2055d64ccd07dc`
- [twelve-week-completion-scope-v3.md](../twelve-week-completion-scope-v3.md) `sha256:5d205e2447e3287ba8867251d55c8fb231ee42fcf4d7db6a984d7094eb7bf897`
- [planning/product-wbs-overview.md](product-wbs-overview.md) `sha256:bc04393aaf7715affe2bd47d3e3b5d4f249e930a10c9f0d9b9792bb3066a3144`
- [payment-integration-workplan.md](../payment-integration-workplan.md) `sha256:f6fd1b044e34c1ce2118a0db37085bb47bf326c362fe4ad97414d369b5709b5a`
- [payment-platform-scope-v2.md](../payment-platform-scope-v2.md) `sha256:0c77441458962cbce3b262f993278c3a98dc1d6400b40d1dd06a2d31139ba8ce`
- [poc-platform-reuse-plan.md](../poc-platform-reuse-plan.md) `sha256:f268ca39ea4465f92331a2ea1533362b8c48b65ad90d617b4bb12bcf1bcc127c`
- [stable-contract-reuse-plan.md](../stable-contract-reuse-plan.md) `sha256:68bfecc19f495a246b827fd62893c6214c56982ab53a2b54b577616b268e02a9`
- [indexer-integration-and-test-token.md](../indexer-integration-and-test-token.md) `sha256:2685ac84bf0f01f3b5aa8b8643d89fbad47a6fedf3404d0bd4b58a16a4bff133`

## 6. 검증 범위

- baseCommit: `c9d66aa2e439`
- stale literal: SL-01(가스는 키오스크가 낸다 [N10]), SL-02(제품 정의 기준이 바뀌었다 [N01]), SL-03(finalized 이벤트가 paid를 정한다 [N08]), SL-04(환불은 범위 밖이다 [N17])
- 예외 경로: DF-01 산출물과 입력, `docs/content/planning/seeds/`, `docs/design-history/`, `docs/content/planning/fixtures/df02/`
- redaction 예외: `specifications/protocol/eip712-vectors.json`(public test-only EOA, deterministic vector)
- waiver: req 10 anti-exfil, req 12 dependency pinning, req 13 genuine-device attestation, W7 gate with a non-secure key, fixed development provisioning and a plaintext kiosk link, TF-M (TrustZone) key sealing for the whole cycle; the external SE was cut

## 7. 사이클 중 변경

| 날짜 | 결정 | 변경 | 이유 | 유지한 합의 | 비용 |
|---|---|---|---|---|---|
| 2026-09-25 | [N18], [N22] | W12-05 거절 시연을 5종에서 8종(합의 5종 + 기기 3종)으로 늘렸다. | 프로토콜 설계에서 기기 거절 코드가 새로 생겼는데 수용 기준이 따라가지 않았다. 세 코드는 버튼 승인, 서명 타입 제한, 시간 기준이라는 보안 약속을 직접 증명한다. | 인터뷰에서 합의한 5종은 그대로 두고 별도 묶음으로 기록한다. | 시연 준비 1일 추가(WBS2-P04-04), 전원 재투입과 재-anchor 절차는 week12-log 3.3절 runbook으로 고정한다. |
| 2026-09-25 | [N03], [N05], [N06], [N07], [N08], [N10], [N11], [N15], [N22], [N23], [N24] | 문서 간 논리 검토 결과를 반영했다: 일정을 W4부터 재배치하고 게이트를 W4/W6/W7/W9로 나눴다, settle 검사 순서를 주문 유일성 우선으로 바꿨다, 셋업 명령 인증(N23)과 서명 권한 경계(N24)를 추가했다, 파라미터 pinMaxRetries·anchorClockSkew를 추가하고 kioskMinGasBalance를 busy 하한으로 다시 정했다. | 재전송이 NONCE_REPLAYED로 끝나 이중 결제를 유발하고, W1–W3이 동결 전 과거였으며, 셋업·reset 명령이 인증 없이 열려 있었다. | 합의한 목표(W12 20회 연속, 거절 시연, 보안 항목)와 제품 범위는 그대로다. | W7 실결제 게이트가 한 주 늦어지고 role A는 W4–W7에 가용 일수를 모두 쓴다. |
| 2026-09-26 | [D05] | NCS v3.4.0/Zephyr 4.4.0에서 NCS v3.4.1/Zephyr 4.4.2로 올렸다. 보드 타깃을 제조사 board package의 nu54v_dk/nrf54l15/cpuapp로 정했다. | 보드 제조사 레퍼런스(nu54v-dk)가 NCS v3.4.1 기준이고, 같은 패치 버전을 써야 board package를 수정 없이 쓸 수 있다. | Zephyr 4.4 계열, 유선 serial recovery 전용 갱신 규칙(N12)은 그대로다. 레퍼런스의 BLE DFU 예제는 가져오지 않는다. | SDK 재설치(약 12 GB). W12-11 증거는 레퍼런스 저장소에 공개된 개발용 MCUboot 키가 아니라 비공개 키로 서명한 이미지로 만든다. |
| 2026-09-27 | [N20], [N25] | 기술 스택(N25)을 정했고, P05를 Foundry 스크립트에서 Go 운영 코어와 CLI로 바꾸되 설계는 백오피스 전체를 전제로 한다(N20). | 체인 호출과 서명 키를 다루는 코드를 한 언어(Go)로 모아 실수를 줄이고, P05를 나중에 백오피스로 키울 때 코어를 그대로 쓰기 위해서다. | P05의 기능 범위와 WBS 공수, 결제 프로토콜은 바꾸지 않는다. | P05 문서와 WBS 작업 제목을 고친다. Go BLE 라이브러리(tinygo-org/bluetooth)는 아직 이 환경에서 시험하지 않았다. |
| 2026-09-28 | [N01], [N09], [N11], [N15], [N26], [N27], [N28], [N29], [N30], [N31], [N32] | 기기 화면을 없애고 대여자 폰 앱(P02)을 확인 화면으로 이번 사이클에 넣었다(N26). 폰 앱·운영자 도구는 QR passkey로 본딩하고 키오스크는 페어링 없이 보안 채널로 결제한다(N27). PIN은 기기 버튼으로 입력한다(N28). NFC를 뺐다(N29). W7은 non-secure 키와 평문으로 먼저 통과하고, role A가 P02를 맡는다(N30). 금액은 토큰 단위 소수점 둘째 자리까지 버림으로 표시하고(N31), 운영 키는 keystore와 secretRef, 키오스크 키는 Android Keystore로 감싼 저장으로 정했다(N32). | 보드를 확인해 보니 화면과 NFC 안테나가 없고, TF-M용 보드 변형도 없었다. 화면 없는 기기에서 Numeric Comparison을 할 수 없고, 키오스크 링크는 카드 결제처럼 페어링 없이 쓰는 편이 사용 흐름에 맞다. | EIP-712 서명 타입 두 개, 버튼 승인, 컨트랙트 한도, 게이트 주차(W4·W6·W7·W9)와 cut order의 SE·P07 순서는 바꾸지 않는다. | P02 작업 약 6일과 보안 채널 약 4.5일(기기 2.5, 키오스크 2)이 늘었다. role A의 W8–W12 부하가 가용 20일에 약 28.5일이며 폰 결제 확인 화면은 W12에 끝난다. 앱에서 PIN을 설정하는 기능은 다음 단계로 미뤘다. |
| 2026-09-29 | [N02] | 외부 SE에서 키를 만들고 SE 안에서 서명하기로 바꿨다. 이전에는 TRNG로 만든 키를 SE로 감쌌다. | 4주차 게이트에서 NXP SE050이 secp256k1 ECDSA를 지원한다는 것을 AN12436 표 1로 확인했다. SE 안에서 서명하면 개인 키가 MCU 메모리를 지나지 않는다. | W9 SE 게이트와 cut order, 버튼 토큰 경계(N24), SE가 컷될 때의 TF-M waiver는 바꾸지 않는다. | SE 명령 경로를 secure partition에 두어야 하고, SE 서명의 low-s 정규화와 recovery id 계산을 MCU에서 한다. 실물에서 secp256k1 키 생성이 실패하면 감싸기 방식으로 되돌린다. |
| 2026-09-29 | [N02], [N03], [N14], [N30] | 외부 SE 통합(WBS2-P01-05)과 W9 SE 게이트를 없애고, 키 생성과 서명을 TrustZone 위 TF-M secure partition에서 한다. 같은 날 앞선 개정(SE 안에서 서명)은 이 개정으로 대체된다. | 외부 SE를 조달해 연결할 시간이 없다. nRF54L15의 TrustZone과 CRACEN(secp256k1 ECDSA)으로 non-secure 코드에 대한 키 격리와 버튼 토큰 경계는 유지된다. | 게이트 W4·W6·W7, 버튼 토큰 경계(N24), 결제 프로토콜, 컨트랙트 한도는 바꾸지 않는다. | 물리 공격 방어가 SE보다 약하고 이를 waiver로 남긴다. KMU는 secp256k1 키를 받지 않는 것으로 보여 키를 ITS에 두므로 ITS 암호화와 AP-Protect가 필수다. role A 부하는 6일 줄어 W8–W12 합계 22.5일이 된다. |

검증: `python3 docs/content/planning/validate_design_freeze_02.py --check <group>` 와 `--self-test`.

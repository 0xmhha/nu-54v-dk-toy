# P01 요구사항 명세 (SRS)

[DF-20260925-02](../../planning/design-freeze-checkpoint-02.md) 기준 P01 펌웨어가 만족해야 하는 요구를 정한다. 메시지와 필드는 [payment-protocol.md](../../specifications/protocol/payment-protocol.md)와 스키마를 따르고, 수치 파라미터(authorizationExpiry, attestationValidity, pinMaxRetries, anchorClockSkew 등)는 register의 parameters에만 있다 [N13]. 각 요구는 검증 방법과 [12주 수용 로그](../../acceptance/week12-log.md) 항목에 연결된다.

## 1. 목적과 범위

P01은 결제 서명기다. 기기는 트랜잭션을 만들거나 보내지 않고, 두 EIP-712 타입에 대한 서명만 만든다 [N04]. 가스와 제출은 키오스크(P04)가, 최종 판정은 정산 컨트랙트(P06)가 맡는다.

## 2. 용어

| 용어 | 뜻 |
|---|---|
| 기기 키 | 대여 셋업 때 TRNG로 만든 secp256k1 키. 기기 밖으로 나가지 않는다 |
| 운영자 주소 | 대여 셋업마다 `setup.operator`로 기록하는 P05 운영자 서명 주소. device.reset 전까지 바꿀 수 없다 [N23] |
| 셋업 값 | 운영자 주소와 함께 기록하는 정산 컨트랙트 주소와 chainId |
| anchor 시간 | 마지막 TimeAnchor의 timestamp에 RTC 경과 시간을 더한 값 |
| 세션 | session.open과 session.confirm으로 열린 BLE 대화. `mode`가 payment 또는 setup이다 |
| 서명 권한 경계 | secure partition의 `sign_digest(digest, purpose)`가 버튼 토큰을 등록된 digest·purpose 한 건에만 발급하는 규칙 [N24] |

## 3. 기능 요구

| ID | 요구 | 검증 방법 | W12 |
|---|---|---|---|
| P01-FR-01 | 기기는 PaymentAuthorization과 LimitChange 두 EIP-712 타입에만 서명하고, 다른 타입·원시 트랜잭션·approve·Permit·스키마에 없는 요청은 `error{UNSUPPORTED_TYPE}`으로 거절한다 [N04] | 타입별 요청 주입 시험 | W12-05, W12-12 |
| P01-FR-02 | 서명 digest는 eip712-vectors.json의 모든 벡터와 바이트 단위로 같다 [N21] | P10 적합성 harness | W12-03 |
| P01-FR-03 | 키는 대여 셋업에서 TRNG로만 만들며 import, 니모닉, 백업 경로가 없다 [N11] | 코드 경로 검토, 셋업 로그 | W12-08 |
| P01-FR-04 | 키는 W9 전에는 TF-M secure partition에 봉인하고, W9부터는 TRNG로 만든 키를 외부 secure element로 감싼다. SE가 컷되면 W12까지 TF-M 봉인으로 남고 waiver로 기록한다 [N14]. 키 원문은 어느 경우에도 non-secure 영역에 나오지 않는다 [N02] | NVM 덤프 검색, SE 서명 로그 | W12-09 |
| P01-FR-05 | 셋업에서 PIN을 정하고, LimitChange 서명은 `sign_digest`의 LimitChange purpose 안에서 secure 쪽 PIN 확인과 버튼을 모두 요구한다 [N11][N24] | LimitChange 시험 | W12-12 |
| P01-FR-06 | device.reset은 운영자가 DeviceReset `{device, nonce}`에 서명한 명령만 받으며, 기기 키, PIN, 운영자 주소, 셋업 값, TimeAnchor, 사용한 nonce 기록을 지우고 UNPROVISIONED가 된다. 반납 순서(closeAccount 확인 뒤 reset)는 P05 도구가 지킨다 [N11][N23] | 초기화 후 NVM 덤프, 서명 없는 reset 거절, 재셋업 시험 | W12-09 |
| P01-FR-07 | MerchantAttestation 서명자가 운영자 주소가 아니거나, MerchantOrder 서명자가 attestation의 merchant가 아니거나, authorization.merchant가 attestation의 merchant와 다르거나, payout이 attestation·주문과 다르거나, orderId·token·amount·expiry가 주문과 다르거나, chainId·contract가 셋업 값과 다르면 `payment.result refused{MERCHANT_FORGED}`로 거절한다 [N22] | 위조 attestation·주문·필드 주입 | W12-05 |
| P01-FR-08 | anchor 시간이 attestation `validFrom..validUntil`(± anchorClockSkew) 밖이거나 authorization expiry가 현재부터 authorizationExpiry를 넘으면 `ATTESTATION_EXPIRED`로 거절한다 [N22] | 만료 attestation 주입 | W12-05 |
| P01-FR-09 | TimeAnchor가 없거나 RAM이 초기화되는 reset(전원 손실, watchdog, System OFF 깨어남, serial recovery 재부팅)으로 무효이면 결제를 `TIME_ANCHOR_MISSING`으로 거절하고 `anchorValid=false`를 보고한다. 재-anchor는 PROVISIONED_NO_ANCHOR 상태의 셋업 세션에서 키를 유지한 채 받는다 [N06] | 전원 차단 후 결제 시도, 재-anchor | W12-05 |
| P01-FR-10 | 서명 전 가맹점 이름, orderId, token, payout, amount를 서명할 필드에서 직접 만들어 표시하고, 버튼을 눌렀을 때만 서명한다. 표시와 서명의 일치는 non-secure 코드가 무결하다는 가정 아래의 주장이다 [N24] | 화면 사진과 서명 필드 비교 | W12-02 |
| P01-FR-16 | 표시 중 대여자가 거절 버튼을 누르면 서명하지 않고 `USER_REJECTED`로 `refused`를 보낸다 [N22] | 거절 버튼 시연 | W12-05 |
| P01-FR-11 | `MERCHANT_REVOKED`, `OVER_CAP`, `NONCE_REPLAYED`는 컨트랙트 층 코드다. 기기는 이 코드를 만들지 않고, 키오스크가 `payment.outcome`으로 보낸 최종 결과를 화면에 보여 준다 [N22] | 컨트랙트 거절 시연 | W12-05 |
| P01-FR-12 | 결제와 LimitChange nonce는 기기가 256비트 난수로 고르고 `payment.result`·`limit.result`로 돌려주며 같은 대여 기간에 다시 쓰지 않는다. payment.prepare에는 nonce가 없다 | 연속 결제 1,000건 nonce 중복 검사 | W12-04 |
| P01-FR-13 | BLE 결제 서비스는 LE Secure Connections로 인증·암호화된 링크에서만 열린다. 페어링만으로는 셋업 권한이 생기지 않는다 [N09][N23] | 비인증 연결 시도 | W12-01 |
| P01-FR-14 | 조각은 sequence와 index 순서를 지키고 length와 digest가 맞을 때만 받아들이며, 아니면 `error{BAD_FRAME}`을 보내고 세션을 닫는다 [N09] | 조각 순서·digest 변조 주입 | W12-01 |
| P01-FR-15 | 펌웨어는 MCUboot가 서명을 확인한 이미지만 부팅하고, 갱신은 유선 serial recovery로만 받는다 [N12] | 서명 안 된 이미지 serial recovery | W12-11 |
| P01-FR-17 | `setup.operator{operator, contract, chainId}`는 UNPROVISIONED 상태에서 대여자가 버튼으로 확인할 때만 기록한다. 셋업 세션(`session.open` mode=setup)은 UNPROVISIONED 또는 PROVISIONED_NO_ANCHOR에서만 열리고 그 밖에는 `NOT_PERMITTED`다 [N23] | 상태별 셋업 명령 주입 | W12-12 |
| P01-FR-18 | `sign_digest(digest, purpose)`는 버튼 대기 직전에 등록한 digest·purpose 한 건에만 secure 쪽 버튼 인터럽트가 발급한 토큰으로 서명한다. 등록되지 않은 purpose나 다른 digest는 거절한다 [N24] | secure 쪽 purpose 시험 | W12-12 |
| P01-FR-19 | `session.cancel`을 받으면 버튼 대기를 멈추고 아무것도 서명하지 않는다 [N10] | 대기 중 cancel 주입 | W12-04 |
| P01-FR-20 | PIN을 pinMaxRetries번 틀리면 PIN_LOCKED가 되어 모든 서명을 거절하고, 잠금은 secure 저장소에 남아 전원 손실·watchdog 같은 RAM 초기화 reset 뒤에도 유지되며, 반납 절차의 운영자 서명 DeviceReset으로만 풀린다 [N11] | 오입력 반복 시험 | W12-12 |

## 4. 비기능 요구

| ID | 요구 | 검증 방법 | W12 |
|---|---|---|---|
| P01-NFR-01 | payment.prepare 수신부터 표시까지 500 ms 안, 버튼 입력부터 payment.result 송신까지 1 s 안에 끝나 키오스크의 10 s 예산 안에 사람 입력 시간을 남긴다 [N10] | 로직 분석기 타임스탬프 | W12-04 |
| P01-NFR-02 | AP-Protect를 켜서 디버거로 메모리를 읽을 수 없다. NVM 덤프 시험은 AP-Protect를 켜기 직전의 같은 image hash로 한다 | 디버거 연결 시도 | W12-10 |
| P01-NFR-03 | PIN 재시도 카운터는 secure 저장소에 있어 전원 재시작에도 유지된다 | 카운터 시험 | W12-12 |
| P01-NFR-04 | 모든 전송(BLE, UART)에 키 반출 명령이 없고, 서명은 버튼 승인 없이 나오지 않는다 [N24] | 전송별 명령 목록, 버튼 없는 요청 | W12-12 |
| P01-NFR-05 | 연속 20회 결제 동안 재부팅이나 세션 누수가 없다 | 반복 시험 로그 | W12-04 |
| P01-NFR-06 | W12 릴리스 이미지에는 USB CDC 시험 harness를 컴파일하지 않는다 | 이미지 구성 파일 | W12-12 |

## 5. 인터페이스

- **BLE GATT (P04, P05 host 도구):** peripheral 역할. `rx` write, `tx` notify, deterministic CBOR 메시지. 규칙은 payment-protocol.md 3–7절.
- **셋업 세션 (P05 스크립트):** 같은 GATT 서비스에서 `session.open` mode=setup으로 열고 `setup.operator`, `setup.timeAnchor{device, timestamp, operatorSignature}`를 받아 단계마다 `setup.ack`로 답한다. 키 생성과 PIN 설정 뒤에는 `setup.ack{step: keygen, device}`로 새 주소를 알린다. UNPROVISIONED에서는 `session.open.ok`에 device가 없다. `device.reset`은 어느 세션에서나 받는다. 목표 경로는 P02 설정 앱이지만 이번 사이클은 설계만 한다 [N06][N23].
- **NFC 태그:** 펌웨어가 NFCT로 에뮬레이션하며 페어링마다 BLE 주소와 새 LESC OOB 데이터를 쓴다.
- **유선 serial(UART):** MCUboot recovery 전용. USB CDC 시험 harness는 개발 빌드에만 있고 게이트 증거가 아니다.

## 6. 제약과 waiver

- 대상 보드 NU-54V-DK(nRF54L15), NCS v3.4.0/Zephyr 4.4 [D05]
- SE가 붙기 전 TF-M 봉인은 testnet waiver다 [N02][N14]
- anti-exfil, 의존성 pinning, 정품 기기 attestation은 설계만 하는 waiver이며 설계안은 [design.md](design.md) 8절에 있다 [N14]
- 개인정보를 저장하지 않는다 [N14]

## 7. 추적

| FR/NFR | 결정 | W12 |
|---|---|---|
| P01-FR-01, FR-02 | [N04][N21] | W12-03, W12-05, W12-12 |
| P01-FR-03, FR-05, FR-06, FR-20 | [N11][N23] | W12-08, W12-09, W12-12 |
| P01-FR-04 | [N02][N14] | W12-09 |
| P01-FR-07, FR-08, FR-11, FR-16 | [N22][N05] | W12-05 |
| P01-FR-09 | [N06] | W12-05 |
| P01-FR-10, FR-18 | [N24] | W12-02, W12-12 |
| P01-FR-12, FR-19 | [N10] | W12-04 |
| P01-FR-13, FR-14 | [N09] | W12-01 |
| P01-FR-15 | [N12] | W12-11 |
| P01-FR-17 | [N23] | W12-12 |
| P01-NFR-01 | [N10] | W12-04 |

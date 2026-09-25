# P01 요구사항 명세 (SRS)

[DF-20260925-02](../../planning/design-freeze-checkpoint-02.md) 기준 P01 펌웨어가 만족해야 하는 요구를 정한다. 메시지와 필드는 [payment-protocol.md](../../specifications/protocol/payment-protocol.md)와 스키마를 따르고, 수치 파라미터는 register의 parameters에만 있다 [N13]. 각 요구는 검증 방법과 [12주 수용 로그](../../acceptance/week12-log.md) 항목에 연결된다.

## 1. 목적과 범위

P01은 결제 서명기다. 기기는 트랜잭션을 만들거나 보내지 않고, 두 EIP-712 타입에 대한 서명만 만든다 [N04]. 가스와 제출은 키오스크(P04)가, 최종 판정은 정산 컨트랙트(P06)가 맡는다.

## 2. 용어

| 용어 | 뜻 |
|---|---|
| 기기 키 | 대여 셋업 때 TRNG로 만든 secp256k1 키. 기기 밖으로 나가지 않는다 |
| 운영자 주소 | provisioning 때 한 번 기록하는 P05 운영자 서명 주소 |
| anchor 시간 | 마지막 TimeAnchor의 timestamp에 RTC 경과 시간을 더한 값 |
| 세션 | session.open과 session.confirm으로 열린 BLE 결제 대화 |

## 3. 기능 요구

| ID | 요구 | 검증 방법 | W12 |
|---|---|---|---|
| P01-FR-01 | 기기는 PaymentAuthorization과 LimitChange 두 EIP-712 타입에만 서명하고, 다른 타입·원시 트랜잭션·approve·Permit 요청은 `UNSUPPORTED_TYPE`으로 거절한다 [N04] | 타입별 요청 주입 시험 | W12-05, W12-12 |
| P01-FR-02 | 서명 digest는 eip712-vectors.json의 모든 벡터와 바이트 단위로 같다 [N21] | P10 적합성 harness | W12-03 |
| P01-FR-03 | 키는 대여 셋업에서 TRNG로만 만들며 import, 니모닉, 백업 경로가 없다 [N11] | 코드 경로 검토, 셋업 로그 | W12-08 |
| P01-FR-04 | 키는 W8 전에는 TF-M secure partition에 봉인하고, W8부터는 외부 secure element로 감싼다. 키 원문은 두 경우 모두 non-secure 영역에 나오지 않는다 [N02] | NVM 덤프 검색, SE 서명 로그 | W12-09 |
| P01-FR-05 | 셋업에서 PIN을 정하고, LimitChange 서명은 PIN과 버튼을 모두 요구한다 [N11] | LimitChange 시험 | W12-12 |
| P01-FR-06 | device.reset은 기기 키, PIN, 운영자 주소, TimeAnchor, 사용한 nonce 기록을 지운다. 반납 절차에서 운영자가 closeAccount를 호출한 뒤 실행한다 [N11] | 초기화 후 NVM 덤프, 재셋업 시험 | W12-09 |
| P01-FR-07 | MerchantAttestation의 서명자가 운영자 주소가 아니거나, MerchantOrder의 서명자가 attestation의 merchant가 아니거나, payout이 attestation과 다르면 `MERCHANT_FORGED`로 거절한다 [N22] | 위조 attestation·주문 주입 | W12-05 |
| P01-FR-08 | anchor 시간이 attestation `validUntil`을 넘거나 `validFrom` 전이면 `ATTESTATION_EXPIRED`로 거절한다 [N22] | 만료 attestation 주입 | W12-05 |
| P01-FR-09 | TimeAnchor가 없거나 전원 손실로 무효이면 결제를 `TIME_ANCHOR_MISSING`으로 거절하고 `anchorValid=false`를 보고한다 [N06] | 전원 차단 후 결제 시도 | W12-05 |
| P01-FR-10 | 서명 전 가맹점 이름, orderId, token, payout, amount를 서명할 필드에서 직접 만들어 표시하고, 버튼을 눌렀을 때만 서명한다 | 화면 사진과 서명 필드 비교 | W12-02 |
| P01-FR-16 | 표시 중 대여자가 거절 버튼을 누르면 서명하지 않고 `USER_REJECTED`로 `refused`를 보낸다 [N22] | 거절 버튼 시연 | W12-05 |
| P01-FR-11 | `MERCHANT_REVOKED`, `OVER_CAP`, `NONCE_REPLAYED`는 컨트랙트 층 코드다. 기기는 이 코드를 만들지 않고, 키오스크가 보고한 결과를 화면에 그대로 보여 준다 [N22] | 컨트랙트 거절 시연 | W12-05 |
| P01-FR-12 | 결제 nonce는 256비트 난수로 고르고 같은 대여 기간에 다시 쓰지 않는다 | 연속 결제 1,000건 nonce 중복 검사 | W12-04 |
| P01-FR-13 | BLE 결제 서비스는 LE Secure Connections로 인증·암호화된 링크에서만 열린다 [N09] | 비인증 연결 시도 | W12-01 |
| P01-FR-14 | 조각은 sequence와 index 순서를 지키고 length와 digest가 맞을 때만 받아들이며, 아니면 `BAD_FRAME`을 보낸다 [N09] | 조각 순서·digest 변조 주입 | W12-01 |
| P01-FR-15 | 펌웨어는 MCUboot가 서명을 확인한 이미지만 부팅하고, 갱신은 유선 serial recovery로만 받는다 [N12] | 서명 안 된 이미지 flash | W12-11 |

## 4. 비기능 요구

| ID | 요구 | 검증 방법 | W12 |
|---|---|---|---|
| P01-NFR-01 | payment.prepare 수신부터 표시까지 500 ms 안, 버튼 입력부터 payment.result 송신까지 1 s 안에 끝나 키오스크의 10 s 예산 안에 사람 입력 시간을 남긴다 [N10] | 로직 분석기 타임스탬프 | W12-04 |
| P01-NFR-02 | AP-Protect를 켜서 디버거로 메모리를 읽을 수 없다 | 디버거 연결 시도 | W12-10 |
| P01-NFR-03 | PIN 재시도 카운터는 전원 재시작에도 유지되며, 정한 횟수를 넘으면 LimitChange를 잠근다 | 카운터 시험 | W12-12 |
| P01-NFR-04 | 키 반출 BLE 명령이 없고, 서명은 버튼 승인 없이 나오지 않는다 | GATT 명령 목록, 버튼 없는 요청 | W12-12 |
| P01-NFR-05 | 연속 20회 결제 동안 재부팅이나 세션 누수가 없다 | 반복 시험 로그 | W12-04 |

## 5. 인터페이스

- **BLE GATT (P04):** peripheral 역할. `rx` write, `tx` notify, deterministic CBOR 메시지. 규칙은 payment-protocol.md 3–6절.
- **셋업 세션 (P05 스크립트):** 같은 GATT 서비스에서 `setup.timeAnchor`와 provisioning 명령을 받는다. 목표 경로는 P02 설정 앱이지만 이번 사이클은 설계만 한다 [N06].
- **NFC 태그:** BLE 주소와 LESC OOB 데이터를 담는 수동 태그. 읽기 전용.
- **유선 serial:** MCUboot recovery 전용. USB CDC 시험 harness는 게이트 증거가 아니다.

## 6. 제약과 waiver

- 대상 보드 NU-54V-DK(nRF54L15), NCS v3.4.0/Zephyr 4.4 [D05]
- SE가 붙기 전 TF-M 봉인은 testnet waiver다 [N02][N14]
- anti-exfil, 의존성 pinning, 정품 기기 attestation은 설계만 하는 waiver다 [N14]
- 개인정보를 저장하지 않는다 [N14]

## 7. 추적

| FR/NFR | 결정 | W12 |
|---|---|---|
| P01-FR-01, FR-02 | [N04][N21] | W12-03, W12-12 |
| P01-FR-03, FR-05, FR-06 | [N11] | W12-08, W12-09, W12-12 |
| P01-FR-04 | [N02][N14] | W12-09 |
| P01-FR-07, FR-08, FR-11 | [N22][N05] | W12-05 |
| P01-FR-09 | [N06] | W12-05 |
| P01-FR-13, FR-14 | [N09] | W12-01 |
| P01-FR-15 | [N12] | W12-11 |
| P01-NFR-01 | [N10] | W12-04 |

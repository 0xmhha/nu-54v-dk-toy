# P04 요구사항 명세 — 가맹점 키오스크

DF-20260925-02 기준 P04의 기능·비기능 요구사항이다. 각 요구사항은 시험으로 판정할 수 있도록 검증 방법과 연결된 W12 항목을 함께 적는다 [N18]. 메시지 형식은 [결제 프로토콜](../../specifications/protocol/payment-protocol.md)이 정한다.

## 1. 용어

| 용어 | 뜻 |
|---|---|
| 요청 전달 완료 | payment.prepare의 마지막 조각을 기기가 받았다고 GATT write 응답이 온 시점 |
| finalized | StableNet 8283의 finalized 블록 태그. 1초 블록이며 latest와 같다 |
| 결과 | approved, refused, failed, Checking 네 가지 |
| 가맹점 대리 | 이번 사이클에서 키오스크가 가맹점 서명 키로 MerchantOrder에 서명하는 역할 |

## 2. 기능 요구사항

| ID | 요구사항 | 검증 | W12 |
|---|---|---|---|
| P04-FR-01 | 서비스 UUID로 BLE scan을 해서 결제 모드의 기기를 찾고, RSSI 기준값 이상인 기기에만 연결한다. NFC는 쓰지 않는다 [N27][N29] | 거리별 연결 로그 | W12-01 |
| P04-FR-02 | 기기와 페어링하지 않는다. W10–W11부터 결제 세션을 보안 채널(1회용 키, `kioskKeySignature`, AES-GCM)로 열고, 그 전(W7 게이트)에는 평문 세션을 쓴다 [N27][N30] | 암호화 세션 로그와 평문 세션 거부 확인 | W12-01 |
| P04-FR-03 | session.open(mode payment) → session.confirm을 수행하고 deviceNonce를 그대로 돌려준다 | 프로토콜 로그 | W12-01 |
| P04-FR-04 | payment.identify로 MerchantAttestation을 전달한다. 키오스크는 attestation을 만들거나 바꾸지 않는다 [N05] | 변조 attestation이 기기에서 거절되는지 확인 | W12-05 |
| P04-FR-05 | payment.prepare로 nonce를 뺀 PaymentAuthorization 필드와 가맹점 서명을 보낸다. 필드는 주문 입력에서만 만든다 [N04] | 서명 필드 덤프와 주문 기록 비교 | W12-02 |
| P04-FR-06 | 기기가 approved와 함께 보낸 서명과 nonce를 저장하고, settle 호출을 eth_call로 먼저 시뮬레이션한다. custom error는 설계 7절 표대로 매핑해 refused로 보고한다. 합의 5종 가운데 MERCHANT_REVOKED, OVER_CAP, NONCE_REPLAYED, MERCHANT_FORGED가 revert로 오고, ATTESTATION_EXPIRED는 기기가 payment.result로 보내는 거절이다 [N22] | 코드별 거절 시연 | W12-05 |
| P04-FR-07 | 기기가 refused를 보내면 reason을 그대로 표시하고 제출하지 않는다. 기기가 error(UNSUPPORTED_TYPE, BAD_FRAME, NOT_PERMITTED)를 보내면 설계 3절의 전이를 따른다 [N22] | 기기 거절 시연 | W12-05 |
| P04-FR-08 | 새 주문을 받기 전(Idle)에 가스 잔액을 확인하고, kioskMinGasBalance 미만이면 주문을 받지 않고 busy를 표시한다 [N10] | 잔액을 낮춘 상태의 화면 | W12-07 |
| P04-FR-09 | 시뮬레이션이 통과하면 키오스크 가스 키로 트랜잭션을 서명·제출하고 가스를 낸다 [N10] | 제출 로그 | W12-03 |
| P04-FR-10 | finalized 블록에서 PaymentSettled를 indexed merchant·orderId로 찾고 device·amount·nonce가 자기 서명과 같으면 approved로 확정한다 [N08] | 이벤트 로그와 화면 | W12-03 |
| P04-FR-11 | 시뮬레이션이 ORDER_ALREADY_PAID이면 finalized PaymentSettled를 조회해 device·amount·nonce가 자기 서명과 같을 때만 approved로 보고, 다르면 다른 결제로 처리된 주문으로 refused를 표시한다 [N08] | 같은 서명 재제출 시험, 다른 결제로 처리된 주문 시험 | W12-03 |
| P04-FR-12 | 제출된 트랜잭션이 status=0이면 같은 호출을 한 번 더 시뮬레이션해 P04-FR-11을 적용하고, 해당하지 않으면 failed와 줄인 tx hash를 표시한다 | 강제 실패 시험 | W12-03 |
| P04-FR-13 | 요청 전달 완료부터 10 s 안에 payment.result가 없으면 session.cancel을 보내고 주문을 취소한다. 서명이 없었으므로 재결제를 허용한다 [N10] | 기기 버튼을 누르지 않는 시험 | W12-04 |
| P04-FR-14 | 서명을 받아 제출한 뒤 요청 전달 완료부터 10 s 안에 최종 결과가 없으면 Checking으로 바꾸고 같은 주문의 재결제를 막는다. Checking에서 자동 재시도는 같은 서명의 재전송만 허용하며, 재전송도 먼저 시뮬레이션을 거친다 [N10] | 재전송 로그에서 새 서명 요청이 없는지 확인 | W12-04 |
| P04-FR-15 | Checking 중 체인 시각이 서명의 expiry를 넘었는데 PaymentSettled가 없으면 failed로 끝내고 재결제를 허용한다 [N10] | expiry 경과 시험 | W12-04 |
| P04-FR-16 | 최종 결과가 정해지면 기기에 payment.outcome으로 알린다 | 기기 로그와 폰 앱 화면 | W12-02 |
| P04-FR-17 | 한도 변경은 limit.change를 기기에 중계하고 limit.result의 서명·nonce로 setLimits를 제출한다 [N04] | LimitChange 적용 로그 | W12-05 |
| P04-FR-18 | refund 기능은 제공하지 않는다. refund는 이번 사이클 범위 밖이다 [N17] | 화면 목록 검토 | - |

## 3. 비기능 요구사항

| ID | 요구사항 | 검증 |
|---|---|---|
| P04-NFR-01 | 연속 20회 결제가 모두 요청 전달 완료부터 10 s 안에 approved로 끝난다 [N10] | W12-04 시간 기록 |
| P04-NFR-02 | p95 지연 수치 목표는 W7 실결제 게이트 이후 정한다. 그 전까지 기준은 10 s hard timeout 하나다 | W7 이후 측정 보고 |
| P04-NFR-03 | 가스 키는 트랜잭션 제출에만, 가맹점 서명 키는 MerchantOrder에만 쓴다. 키오스크는 사용자 결제 서명을 만들 수 없다 | 코드 검토 |
| P04-NFR-04 | 로그와 화면에는 줄인 주소·해시만 남긴다 [N16] | 로그 샘플 검토 |
| P04-NFR-05 | 앱 재시작 후에도 Checking 주문의 orderId, 서명, nonce를 잃지 않는다 | 강제 종료 후 재전송 시험 |

## 4. 인터페이스

| 상대 | 인터페이스 | 기준 |
|---|---|---|
| P01 | BLE GATT rx/tx, deterministic CBOR | payment-protocol.schema.json |
| P06 | settle·setLimits 호출, PaymentSettled 이벤트, custom error | P06 설계의 ABI |
| P05 | MerchantAttestation, 시험 가맹점 키(키오스크 설정의 secretRef로 전달) | [N05][N20] |
| StableNet 8283 | eth_call, eth_sendRawTransaction, eth_getLogs(finalized) | [N08] |

## 5. 추적

| 결정 | 요구사항 |
|---|---|
| N04 | P04-FR-05, 17 |
| N08 | P04-FR-10, 11 |
| N09 | P04-FR-01..03 |
| N10 | P04-FR-08, 09, 13, 14, 15, P04-NFR-01 |
| N17 | P04-FR-18 |
| N22 | P04-FR-06, 07 |

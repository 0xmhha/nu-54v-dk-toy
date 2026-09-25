# P04 요구사항 명세 — 가맹점 키오스크

DF-20260925-02 기준 P04의 기능·비기능 요구사항이다. 각 요구사항은 시험으로 판정할 수 있도록 검증 방법과 연결된 W12 항목을 함께 적는다 [N18]. 메시지 형식은 [결제 프로토콜](../../specifications/protocol/payment-protocol.md)이 정한다.

## 1. 용어

| 용어 | 뜻 |
|---|---|
| 요청 전달 완료 | payment.prepare의 마지막 조각을 기기가 받았다고 GATT write 응답이 온 시점 |
| finalized | StableNet 8283의 finalized 블록 태그. 1초 블록이며 latest와 같다 |
| 결과 | approved, refused, failed, Checking 네 가지 |

## 2. 기능 요구사항

| ID | 요구사항 | 검증 | W12 |
|---|---|---|---|
| P04-FR-01 | 기기의 NFC 태그에서 BLE 주소와 OOB 데이터를 읽고, 실패하면 서비스 UUID로 BLE scan을 한다 [N09] | NFC 있는/없는 두 경우 연결 로그 | W12-01 |
| P04-FR-02 | LE Secure Connections로 페어링하고 암호화 링크에서만 rx/tx를 연다 [N09] | 비암호화 연결에서 write 거부 확인 | W12-01 |
| P04-FR-03 | session.open → session.confirm을 수행하고 deviceNonce를 그대로 돌려준다 | 프로토콜 로그 | W12-01 |
| P04-FR-04 | payment.identify로 MerchantAttestation을 전달한다. 키오스크는 attestation을 만들거나 바꾸지 않는다 [N05] | 변조 attestation이 기기에서 거절되는지 확인 | W12-05 |
| P04-FR-05 | payment.prepare로 PaymentAuthorization 필드와 가맹점 서명을 보낸다. 필드는 주문 입력에서만 만든다 [N04] | 서명 필드 덤프와 주문 기록 비교 | W12-02 |
| P04-FR-06 | 서명을 받으면 settle 호출을 eth_call로 먼저 시뮬레이션하고, revert 사유를 MERCHANT_REVOKED, OVER_CAP, NONCE_REPLAYED, MERCHANT_FORGED 중 하나로 매핑해 refused로 보고한다. ATTESTATION_EXPIRED는 기기가 payment.result로 보내는 거절이며 revert로 오지 않는다 [N22] | 코드별 거절 시연 | W12-05 |
| P04-FR-07 | 기기가 refused를 보내면 reason을 그대로 표시하고 제출하지 않는다 [N22] | 기기 거절 시연 | W12-05 |
| P04-FR-08 | 키오스크 가스 잔액이 kioskMinGasBalance 미만이면 새 결제를 받지 않고 busy를 표시한다 [N10] | 잔액을 낮춘 상태의 화면 | W12-07 |
| P04-FR-09 | 시뮬레이션이 통과하면 키오스크 키로 트랜잭션을 서명·제출하고 가스를 낸다 [N10] | 제출 로그 | W12-03 |
| P04-FR-10 | finalized 블록에서 PaymentSettled(merchant, orderId)를 찾으면 approved로 확정한다 [N08] | 이벤트 로그와 화면 | W12-03 |
| P04-FR-11 | 같은 orderId로 ORDER_ALREADY_PAID revert가 나면 성공(approved)으로 본다 [N08] | 같은 주문 재제출 시험 | W12-03 |
| P04-FR-12 | 제출된 트랜잭션이 status=0이면 failed와 줄인 tx hash를 표시한다 | 강제 실패 시험 | W12-05 |
| P04-FR-13 | 요청 전달 완료부터 10 s 안에 결과가 없으면 Checking으로 바꾸고 같은 주문의 재결제를 막는다 [N10] | 기기 버튼을 누르지 않는 시험 | W12-04 |
| P04-FR-14 | Checking 동안 자동 재시도는 같은 서명의 재전송만 허용한다 [N10] | 재전송 로그에서 새 서명 요청이 없는지 확인 | W12-04 |
| P04-FR-15 | refund 기능은 제공하지 않는다. refund는 이번 사이클 범위 밖이다 [N17] | 화면 목록 검토 | - |

## 3. 비기능 요구사항

| ID | 요구사항 | 검증 |
|---|---|---|
| P04-NFR-01 | 연속 20회 결제가 모두 요청 전달 완료부터 10 s 안에 approved로 끝난다 [N10] | W12-04 시간 기록 |
| P04-NFR-02 | p95 지연 수치 목표는 W5–6에 정한다. 그 전까지 기준은 10 s hard timeout 하나다 | W5–6 측정 보고 |
| P04-NFR-03 | 키오스크 키는 가스 지불 트랜잭션만 서명하고 사용자 결제 서명을 만들 수 없다 | 코드 검토 |
| P04-NFR-04 | 로그와 화면에는 줄인 주소·해시만 남긴다 [N16] | 로그 샘플 검토 |
| P04-NFR-05 | 앱 재시작 후에도 Checking 주문의 orderId와 서명을 잃지 않는다 | 강제 종료 후 재전송 시험 |

## 4. 인터페이스

| 상대 | 인터페이스 | 기준 |
|---|---|---|
| P01 | BLE GATT rx/tx, deterministic CBOR | payment-protocol.schema.json |
| P06 | settle 호출, PaymentSettled 이벤트, revert 사유 | P06 설계의 ABI |
| P05 | MerchantAttestation, 가맹점 서명 키 | [N05][N20] |
| StableNet 8283 | eth_call, eth_sendRawTransaction, eth_getLogs(finalized) | [N08] |

## 5. 추적

| 결정 | 요구사항 |
|---|---|
| N08 | P04-FR-10, P04-FR-11 |
| N09 | P04-FR-01..03 |
| N10 | P04-FR-08, 09, 13, 14, P04-NFR-01 |
| N17 | P04-FR-15 |
| N22 | P04-FR-06, 07 |

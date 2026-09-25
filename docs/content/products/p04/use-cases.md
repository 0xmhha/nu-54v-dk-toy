# P04 유즈케이스 — 가맹점 키오스크

키오스크에서 일어나는 결제 흐름을 행위자 관점으로 정리한다. 요구사항 번호는 [SRS](srs.md)를, 메시지는 [결제 프로토콜](../../specifications/protocol/payment-protocol.md)을 따른다 [N09].

## 1. 행위자

| 행위자 | 역할 |
|---|---|
| 점원 | 금액을 입력하고 결과를 확인한다 |
| 대여자 | 기기 화면을 보고 버튼으로 승인하거나 거절한다 |
| 기기(P01) | 가맹점을 검증하고 nonce를 골라 PaymentAuthorization에 서명한다 |
| 정산 컨트랙트(P06) | 주문 유일성·registry·한도·nonce를 검사하고 PaymentSettled를 낸다 |

## 2. UC-P04-01 정상 결제

- **사전 조건:** 키오스크 가스 잔액이 kioskMinGasBalance 이상이다 [N10]. 기기는 대여 셋업을 마쳐 유효한 TimeAnchor와 예치금이 있다 [N06].
- **주 흐름:**
  1. 점원이 금액을 넣으면 키오스크가 orderId를 만들고 가맹점 대리로 MerchantOrder에 서명한다.
  2. 대여자가 기기를 NFC에 대거나 키오스크가 BLE scan으로 찾아 페어링한다.
  3. 키오스크가 session.open, session.confirm, payment.identify, payment.prepare(nonce 없음)를 차례로 보낸다.
  4. 대여자가 기기 화면의 금액과 가맹점을 확인하고 버튼을 누른다. 기기는 서명과 nonce를 payment.result로 보낸다.
  5. 키오스크가 eth_call로 시뮬레이션한 뒤 제출하고, finalized PaymentSettled의 device·amount·nonce를 확인해 approved를 표시한다 [N08].
  6. 키오스크가 payment.outcome으로 기기에 결과를 알린다.
- **사후 조건:** 주문이 approved로 저장되고 영수증 화면이 뜬다.
- **요구사항:** P04-FR-01..05, 09, 10, 16

## 3. UC-P04-02 거절이 키오스크에 보이는 경우

| 코드 | 거절 층 | 키오스크 화면 |
|---|---|---|
| ATTESTATION_EXPIRED | 기기 | "가맹점 인증이 만료되었습니다" |
| MERCHANT_FORGED | 기기 또는 eth_call | "가맹점 정보가 일치하지 않습니다" |
| MERCHANT_REVOKED | eth_call | "등록이 취소된 가맹점입니다" |
| OVER_CAP | eth_call | "결제 한도를 넘었습니다" |
| NONCE_REPLAYED | eth_call | "이미 사용된 승인입니다" |
| USER_REJECTED, TIME_ANCHOR_MISSING | 기기 | "대여자가 거절했습니다", "기기 재설정이 필요합니다" |

- **주 흐름:** 기기가 refused를 보내거나 eth_call이 revert하면 키오스크는 제출하지 않고 refused와 코드를 표시한다 [N22].
- **NONCE_REPLAYED 시연:** 결제 서명을 다시 내면 주문 유일성에서 먼저 걸려 UC-P04-05로 끝나므로, 시연 중에 LimitChange에 서명·적용한 뒤 그 expiry(2분) 안에 같은 서명을 setLimits로 다시 제출해 만든다.
- **사후 조건:** 체인에 결제가 남지 않는다.
- **요구사항:** P04-FR-06, 07

## 4. UC-P04-03 시간 초과와 Checking 복구

- **서명 전 시간 초과:** 요청 전달 완료부터 10 s 안에 payment.result가 없으면 session.cancel을 보내고 주문을 취소한다. 서명이 없으므로 점원은 다시 결제를 받을 수 있다 [N10].
- **제출 후 시간 초과:** 요청 전달 완료부터 같은 10 s 안에 최종 결과가 없고 이미 제출했으면 Checking으로 바꾸고 "다시 결제하지 마세요"를 표시한다.
- **복구:** 같은 서명만 재전송하되 먼저 시뮬레이션을 거치고 finalized PaymentSettled를 계속 찾는다. 찾으면 approved로 바꾼다. 체인 시각이 서명의 expiry를 넘었는데 이벤트가 없으면 failed로 끝내고 재결제를 허용한다.
- **예외:** 앱이 재시작되어도 Checking 주문을 복원해 같은 흐름을 이어 간다.
- **요구사항:** P04-FR-13, 14, 15, P04-NFR-05

## 5. UC-P04-04 가스 잔액 부족

- **흐름:** 새 주문을 받기 전에 잔액을 확인하고, kioskMinGasBalance 미만이면 새 결제 버튼을 막고 busy를 표시한다 [N10]. 운영자가 잔액을 채우면 다시 받는다.
- **요구사항:** P04-FR-08

## 6. UC-P04-05 이미 결제된 주문

- **흐름:** 같은 서명을 재전송하면 시뮬레이션이 ORDER_ALREADY_PAID로 끝난다. 키오스크는 finalized PaymentSettled를 조회해 device·amount·nonce가 자기 서명과 같으면 approved를 유지한다 [N08]. 다르면 다른 결제로 처리된 주문이므로 refused를 표시한다.
- **요구사항:** P04-FR-11

## 7. UC-P04-06 제출 실패

- **흐름:** 트랜잭션이 status=0으로 끝나면 한 번 더 시뮬레이션한다. ORDER_ALREADY_PAID이고 이벤트가 자기 서명과 맞으면 approved, 이벤트가 다르면 refused(다른 결제로 처리된 주문), 그 밖의 결과면 자동 재제출 없이 failed와 줄인 tx hash를 표시하고 점원이 새 주문으로 다시 시작한다.
- **요구사항:** P04-FR-12

## 8. UC-P04-07 한도 변경

- **흐름:** 대여자가 요청하면 키오스크가 limit.change를 보내고, 기기가 PIN과 버튼 확인 뒤 limit.result로 서명과 nonce를 돌려준다. 키오스크가 setLimits로 제출한다 [N04].
- **요구사항:** P04-FR-17

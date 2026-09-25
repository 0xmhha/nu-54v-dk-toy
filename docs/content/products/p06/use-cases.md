# P06 유즈케이스 — 정산 컨트랙트

정산 컨트랙트를 호출하는 주체별 흐름이다. 요구 ID는 [srs.md](srs.md)를, 메시지는 [결제 프로토콜](../../specifications/protocol/payment-protocol.md)을 따른다.

## 1. 행위자

| 행위자 | 역할 |
|---|---|
| 운영자(P05 스크립트) | 예치, closeAccount, 출금 실행 |
| registry admin(P05 스크립트) | 가맹점 등록·철회·payout 변경 |
| 키오스크(P04) | settle 시뮬레이션과 제출, 가스 부담 [N10] |
| 가맹점 | cashOut 호출 |
| 누구나 | 지연이 끝난 출금 실행 |

## 2. UC-P06-01 대여 시 예치

- **행위자:** 운영자
- **선행 조건:** 기기가 셋업을 마치고 기기 주소를 보고했다 [N11].
- **주 흐름:** 운영자가 토큰 approve 뒤 `depositFor(deviceAddress, amount, withdrawAddress)`를 호출한다. 컨트랙트가 기기 계정을 활성으로 만들고 `Deposited` 이벤트를 낸다 [N07].
- **예외:** 이미 닫힌 기기 주소면 revert한다. 새 대여는 새 키, 새 주소로 한다.
- **사후 조건:** 기기 계정 잔액이 늘고 withdrawAddress가 고정된다.
- **요구:** P06-FR-01

## 3. UC-P06-02 결제 정산

- **행위자:** 키오스크
- **선행 조건:** 기기가 PaymentAuthorization에 서명했다 [N04].
- **주 흐름:**
  1. 키오스크가 `settle(auth, signature)`를 eth_call로 시뮬레이션한다.
  2. 통과하면 트랜잭션을 보낸다.
  3. 컨트랙트가 서명자, registry, payout, 한도, nonce, 만료, 주문 유일성을 검사하고 가맹점 잔액으로 옮긴 뒤 PaymentSettled를 낸다.
  4. 키오스크는 finalized 블록에서 이벤트를 보고 approved로 표시한다 [N08].
- **대체 흐름:** 시뮬레이션이 MERCHANT_REVOKED, OVER_CAP, NONCE_REPLAYED, MERCHANT_FORGED로 revert하면 제출하지 않고 refused로 보고한다 [N22]. ORDER_ALREADY_PAID면 이미 결제된 것으로 보고 성공 처리한다.
- **사후 조건:** (merchant, orderId)가 결제됨으로 기록되고 nonce가 소비된다.
- **요구:** P06-FR-02~P06-FR-11

## 4. UC-P06-03 가맹점 등록과 철회

- **행위자:** registry admin
- **주 흐름:** admin이 가맹점 서명 주소와 payout을 등록한다. P05가 같은 값으로 MerchantAttestation을 발급한다 [N05].
- **대체 흐름:** 철회하면 즉시 settle이 MERCHANT_REVOKED로 막힌다. 기기가 아직 유효한 attestation을 갖고 있어도 컨트랙트가 막는다.
- **요구:** P06-FR-17

## 5. UC-P06-04 payout 변경

- **행위자:** registry admin, 가맹점
- **주 흐름:** admin이 새 payout을 요청한다. payoutChangeDelay가 지나기 전까지 이전 payout이 유효하고, 그동안 이전 payout으로 서명된 결제는 계속 정산된다. 지연이 지나면 새 payout이 적용되고 이전 payout을 담은 서명은 MERCHANT_FORGED로 거절된다 [N13].
- **요구:** P06-FR-06, P06-FR-12

## 6. UC-P06-05 한도 낮추기

- **행위자:** 대여자(기기에서 PIN과 버튼), 키오스크(제출)
- **주 흐름:** 기기가 LimitChange에 서명하고 키오스크가 `setLimits`를 제출한다. 컨트랙트는 register 상한 이하일 때만 적용한다.
- **요구:** P06-FR-13

## 7. UC-P06-06 반납과 closeAccount

- **행위자:** 운영자, 누구나
- **선행 조건:** 기기가 반납되었다.
- **주 흐름:** 운영자가 `closeAccount(device)`를 호출한다. 기기 계정은 즉시 비활성이 되어 남은 서명이 있어도 정산되지 않는다. withdrawalDelay 뒤 누구나 지급을 실행할 수 있고 잔액은 withdrawAddress로 간다 [N07][N11]. 이후 기기는 device.reset으로 키를 지운다.
- **요구:** P06-FR-15, P06-FR-16

## 8. UC-P06-07 가맹점 cash-out

- **행위자:** 가맹점
- **주 흐름:** 가맹점이 `cashOut()`을 호출하면 가맹점 잔액이 registry payout으로 전송된다.
- **예외:** payout 변경이 대기 중이면 이전 payout으로 보낸다.
- **요구:** P06-FR-12

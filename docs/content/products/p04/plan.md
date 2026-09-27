# P04 기획 — 가맹점 키오스크

이 문서는 DF-20260925-02 기준으로 P04 키오스크가 이번 12주 사이클에서 무엇을 만들고 언제 무엇으로 완료를 판정하는지 정한다 [N01]. 규칙과 값은 [DF-20260925-02](../../planning/design-freeze-checkpoint-02.md)와 [결제 프로토콜](../../specifications/protocol/payment-protocol.md)을 따르고, 이 문서는 결정 ID로만 인용한다.

## 1. 목표

키오스크는 Android 태블릿에서 동작하는 React Native 앱이다. 점원이 주문 금액을 넣으면 BLE로 기기(P01)에 결제 서명을 요청하고, 받은 PaymentAuthorization을 정산 컨트랙트(P06)에 직접 제출한 뒤 finalized 이벤트로 결제 완료를 판정한다 [N08]. 가스는 키오스크가 낸다 [N10]. 이번 사이클에서 키오스크는 가맹점의 POS이므로 가맹점 서명 키를 갖고 가맹점을 대리한다 [N05].

## 2. 범위

| 영역 | 이번 사이클에 만드는 것 |
|---|---|
| 주문 입력 | 금액·토큰 입력, orderId 생성, 가맹점 대리 MerchantOrder 서명 |
| BLE central | BLE scan과 RSSI 기준으로 기기 발견, 페어링 없는 연결, 결제 세션 보안 채널(W10–W11), 메시지 조각 재조립 [N09][N27] |
| 결제 흐름 | session.open부터 payment.result, payment.outcome까지 [N09] |
| 제출 | eth_call 시뮬레이션, 트랜잭션 제출, 가스 잔액 관리 [N10] |
| 판정 | finalized PaymentSettled 확인, 네 가지 결과 표시 [N08][N22] |
| 한도 변경 | limit.change 중계와 setLimits 제출 [N04] |
| W12 리허설 | 연속 20회 결제와 거절 8종(합의 5종 + 기기 3종) 시연 [N18] |

## 3. 산출물

1. Android 태블릿용 키오스크 APK와 빌드 스크립트
2. 프로토콜 codec과 BLE 모듈의 단위 시험(eip712-vectors.json 적합성 포함)
3. 테스트넷 8283 제출 로그와 W12 리허설 기록

## 4. 일정

| WBS | 작업 | 기간 | 게이트 |
|---|---|---|---|
| WBS2-P04-01 | RN 골격, 주문 입력, BLE central과 조각 처리 | W4–W6 | W7 |
| WBS2-P04-02 | 최소 제출과 finalized PaymentSettled 확인(P06-02 이후) | W7 | W7 |
| WBS2-P04-03 | eth_call 시뮬레이션, 가스 하한, session.cancel, Checking, 오류 매핑, LimitChange 중계(limit.change → limit.result → setLimits) | W8–W9 | - |
| WBS2-P04-04 | 연속 20회 결제와 거절 8종 리허설 | W11–W12 | - |

게이트는 W4 증거 게이트, W6 컨트랙트 게이트, W7 실결제 게이트, W9 SE 게이트다 [N03]. P04의 첫 게이트는 W7이다. W7에 보드 내장 키로 실기 end-to-end 1건을 보이려면 WBS2-P04-02가 W6 컨트랙트 게이트(소프트웨어 서명 PaymentSettled)를 통과한 배포에 맞춰 W7 안에 끝나야 한다. W9에는 SE 키로 같은 흐름을 다시 확인한다.

## 5. 의존성

- P01: GATT 서비스와 payment.result 응답. W7 이전에는 USB CDC harness로 시험하되 게이트 증거로 쓰지 않는다 [N09].
- P06: 8283에 배포된 정산 컨트랙트 주소와 ABI(W6, WBS2-P06-02).
- P05: 가맹점 등록과 MerchantAttestation 발급(`opsctl`), 시험 가맹점 키를 키오스크 설정의 secretRef로 전달 [N05][N20].
- P07: 영수증 조회. 선택 사항이며 판정에는 쓰지 않는다 [N19].

## 6. 위험

| 위험 | 영향 | 대응 |
|---|---|---|
| p95 지연 목표 미정 | 성능 기준이 10 s hard timeout 하나뿐이다 | W7 실결제 게이트 이후 측정 결과로 수치 p95 목표를 정하고 register에 기록 요청 |
| 테스트넷 가스 확보 | 잔액이 kioskMinGasBalance 미만이면 키오스크가 busy가 되어 시연이 멈춘다 | W12 시작 잔액은 [수용 기록지](../../acceptance/week12-log.md) 1절 기준으로 미리 확보한다 [N10] |
| Android 태블릿 BLE 편차 | 기종별 MTU·페어링 동작이 다를 수 있다 | W6까지 실기 태블릿 1대로 고정하고 재조립 시험 |
| role B 부하 | W6과 W9에 P04·P05 작업이 가용 일수를 채운다 | cut order를 따른다 [N03] |
| 키오스크 탈취 | 가맹점 서명 키가 함께 노출된다 | 기기가 attestation의 payout과 버튼 승인을 강제하므로 피해는 등록된 payout으로의 한도 내 결제로 제한된다 [N05] |

## 7. 범위 밖

소셜 로그인, 메뉴·매출·정산 백오피스, refund 처리는 이번 사이클에서 만들지 않는다 [N17]. 이전 WBS의 매장 운영 기능은 [N01]에 따라 설계 자료로만 남는다.

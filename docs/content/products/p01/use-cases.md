# P01 유즈케이스

P01 펌웨어가 참여하는 사용 흐름이다. 요구 ID는 [srs.md](srs.md), 메시지 이름은 [payment-protocol.md](../../specifications/protocol/payment-protocol.md)를 따른다. 행위자는 대여자, 운영자(P05 스크립트), 키오스크(P04)다.

## 1. UC-P01-01 대여 셋업

- **행위자:** 운영자, 대여자
- **사전 조건:** 기기가 UNPROVISIONED 상태다. 운영자 스크립트가 운영자 키를 가진다.
- **기본 흐름:**
  1. 운영자 스크립트가 `session.open` mode=setup으로 셋업 세션을 연다. 키가 아직 없으므로 `session.open.ok`에는 device가 없다 [N23].
  2. 운영자 스크립트가 `setup.operator{operator, contract, chainId}`를 보낸다. 기기가 세 값을 표시하고 대여자가 버튼으로 확인하면 기록하고 `setup.ack{step: setup.operator}`로 답한다.
  3. 기기가 TRNG로 새 키를 만들고 기기 주소를 표시한다 [N11].
  4. 대여자가 기기 버튼으로 PIN을 정한다. 기기가 `setup.ack{step: keygen, device}`로 새 주소를 알린다.
  5. 운영자 스크립트가 받은 주소로 TimeAnchor에 서명해 `setup.timeAnchor{device, timestamp, operatorSignature}`를 보낸다. 이번 사이클은 P05 스크립트가 보내고, 목표 경로인 P02 설정 앱은 설계만 한다 [N06].
  6. 기기가 `device`가 자기 주소인지, 서명자가 기록한 운영자 주소인지, timestamp가 이전 값보다 엄격히 늦은지 확인하고 anchor를 기록한다.
  7. 기기가 `setup.ack{step: setup.timeAnchor, accepted, lastAnchor}`로 답한다.
  8. 운영자가 ack를 확인한 뒤 depositFor로 기기 주소에 예치한다 [N07].
- **대안 흐름:** 2에서 대여자가 확인하지 않으면 운영자 주소를 기록하지 않는다. 6에서 조건이 하나라도 틀리면 `setup.ack{accepted: false}`로 anchor를 거절하고 셋업을 끝내지 않는다.
- **사후 조건:** 기기가 READY 상태다.
- **요구:** P01-FR-03, FR-05, FR-09, FR-17

## 2. UC-P01-02 페어링과 세션

- **행위자:** 키오스크, 대여자
- **사전 조건:** 셋업이 끝났다.
- **기본 흐름:**
  1. 키오스크가 NFC 태그(펌웨어가 에뮬레이션)에서 BLE 주소와 OOB 데이터를 읽는다 [N09].
  2. OOB로 LE Secure Connections 페어링을 한다.
  3. 키오스크가 `session.open` mode=payment를 보내고, 기기가 `session.open.ok`로 deviceNonce, anchorValid, state를 보낸다.
  4. 키오스크가 session.confirm으로 deviceNonce를 돌려보내 세션을 연다.
- **대안 흐름:** NFC를 읽지 못하면 BLE scan으로 찾고 Numeric Comparison으로 페어링한다. 대여자가 기기 화면의 숫자를 보고 버튼으로 확인한다.
- **예외 흐름:** deviceNonce가 다르면 세션을 열지 않고 `error`를 보낸다.
- **사후 조건:** 인증된 결제 세션이 열린다. 셋업 권한은 생기지 않는다.
- **요구:** P01-FR-13, FR-14

## 3. UC-P01-03 결제 승인

- **행위자:** 대여자, 키오스크
- **사전 조건:** 세션이 열려 있고 anchor가 유효하다.
- **기본 흐름:**
  1. 키오스크가 payment.identify로 attestation을 보낸다. 기기가 서명자와 유효 기간(± anchorClockSkew)을 확인하고 가맹점 이름을 표시한다 [N05].
  2. 키오스크가 payment.prepare로 nonce 없는 PaymentAuthorization 필드와 가맹점 서명을 보낸다.
  3. 기기가 주문 서명자, authorization.merchant, payout, 주문 필드, chainId·contract를 확인하고 orderId, token, payout, amount를 표시한다.
  4. 기기가 digest와 purpose를 secure 쪽에 등록하고 버튼을 기다린다 [N24].
  5. 대여자가 버튼을 누른다.
  6. 기기가 nonce를 골라 서명하고 `payment.result{approved, signature, nonce}`를 보낸다 [N04].
  7. 키오스크가 `payment.outcome`으로 최종 결과를 보내고 기기가 표시한다.
- **대안 흐름:** 5에서 대여자가 거절 버튼을 누르면 `refused`와 `USER_REJECTED`를 보낸다. 키오스크가 10 s 뒤 `session.cancel`을 보내면 버튼 대기를 멈추고 서명하지 않는다.
- **사후 조건:** 키오스크가 제출하고, 판정은 finalized PaymentSettled로 정해진다 [N08].
- **요구:** P01-FR-01, FR-02, FR-10, FR-12, FR-18, FR-19

## 4. UC-P01-04 기기 층 거절

- **행위자:** 키오스크(시연에서는 P05 host 도구)
- **사전 조건:** 세션이 열려 있다.
- **흐름과 결과:**

| 경우 | 입력 | 기기 응답 |
|---|---|---|
| 만료 attestation | anchor 시간이 `validUntil`(+ anchorClockSkew)을 넘은 attestation | `payment.result refused`, ATTESTATION_EXPIRED [N22] |
| 위조 attestation | 운영자가 아닌 키로 서명한 attestation | `payment.result refused`, MERCHANT_FORGED |
| 위조 주문 | attestation의 merchant가 아닌 키로 서명한 주문, 또는 authorization.merchant가 다름 | `payment.result refused`, MERCHANT_FORGED |
| payout·필드 변조 | attestation·주문과 다른 payout, 주문과 다른 amount, 셋업 값과 다른 contract | `payment.result refused`, MERCHANT_FORGED |
| anchor 없음 | reset 뒤 재-anchor 전 결제 | `payment.result refused`, TIME_ANCHOR_MISSING |
| 지원 안 하는 타입 | 원시 트랜잭션이나 Permit, 스키마에 없는 메시지 | `error`, UNSUPPORTED_TYPE |

- **사후 조건:** 서명이 나오지 않고 체인에 아무것도 남지 않는다.
- **요구:** P01-FR-01, FR-07, FR-08, FR-09

## 5. UC-P01-05 한도 변경

- **행위자:** 대여자
- **사전 조건:** 세션이 열려 있다.
- **기본 흐름:**
  1. 키오스크가 limit.change로 새 perPaymentLimit, dailyLimit을 보낸다.
  2. 기기가 값을 표시하고 PIN 입력을 요구한다.
  3. 대여자가 PIN을 입력하고 버튼을 누른다. PIN 확인은 `sign_digest`의 LimitChange purpose 안에서 secure 쪽이 한다 [N24].
  4. 기기가 nonce를 골라 LimitChange에 서명하고 `limit.result{approved, signature, nonce}`로 돌려준다 [N04].
- **대안 흐름:** PIN이 틀리면 재시도 카운터를 늘린다. pinMaxRetries를 넘으면 PIN_LOCKED가 되어 모든 서명을 거절한다. 잠금은 secure 저장소에 남아 전원을 다시 넣어도 풀리지 않는다.
- **사후 조건:** 키오스크가 setLimits로 제출하고, 컨트랙트가 register 상한(perPaymentCap, dailyCap) 안에서 새 한도를 적용한다 [N13].
- **요구:** P01-FR-05, FR-20, NFR-03

## 6. UC-P01-06 reset 후 재-anchor

- **행위자:** 대여자, 운영자
- **사전 조건:** 대여 중 전원 손실, watchdog, System OFF 깨어남, serial recovery 재부팅 중 하나가 일어났다.
- **기본 흐름:**
  1. 기기가 재부팅 후 anchor를 무효로 표시하고 PROVISIONED_NO_ANCHOR가 된다.
  2. 키오스크 세션에서 `anchorValid=false`를 보고하고 결제를 `TIME_ANCHOR_MISSING`으로 거절한다.
  3. 운영자 스크립트가 셋업 세션을 열고 새 `setup.timeAnchor`를 보낸다 [N06].
  4. 기기가 새 anchor가 이전 값보다 엄격히 늦은지 확인하고 받아들인다.
- **사후 조건:** 키와 예치금은 그대로이고 결제가 다시 가능하다.
- **요구:** P01-FR-09

## 7. UC-P01-07 반납과 초기화

- **행위자:** 운영자
- **사전 조건:** 대여자가 기기를 반납했다.
- **기본 흐름:**
  1. 운영자가 closeAccount를 호출해 기기 주소를 비활성화하고 finalized 이벤트를 확인한다. 잔액은 지연 뒤 등록된 출금 주소로 나간다 [N07][N11].
  2. 운영자 도구가 DeviceReset `{device, nonce}`에 운영자 키로 서명해 `device.reset`을 보낸다 [N23].
  3. 기기가 서명자와 device를 확인하고 키, PIN, 운영자 주소, 셋업 값, TimeAnchor, nonce 기록을 지운 뒤 UNPROVISIONED로 재부팅한다.
- **예외 흐름:** 운영자 서명이 없거나 틀린 device.reset은 거절한다. 반납 순서는 기기가 판정하지 않고 P05 도구가 지킨다.
- **사후 조건:** 다음 대여를 위한 셋업만 가능하다.
- **요구:** P01-FR-06

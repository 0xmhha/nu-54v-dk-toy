# 결제 프로토콜 v1

NU-54V-DK 기기(P01), 키오스크(P04), 운영 스크립트(P05), 정산 컨트랙트(P06)가 주고받는 메시지와 서명 형식을 정한다. 이 문서는 규칙을 설명하고, 타입과 필드는 [payment-protocol.schema.json](payment-protocol.schema.json)이, 서명 해시의 정답은 [eip712-vectors.json](eip712-vectors.json)이 정한다 [N21]. 값(한도, 유효 기간, 가스 잔액)은 [DF-20260925-02](../../planning/design-freeze-checkpoint-02.md)의 parameters에만 있고 여기서는 이름으로만 부른다 [N13].

## 1. 누가 무엇을 믿는가

| 주체 | 가진 키 | 서명하는 것 | 검증하는 쪽 |
|---|---|---|---|
| 기기(P01) | 대여 때 만든 기기 키 | PaymentAuthorization, LimitChange [N04] | 컨트랙트 |
| 운영자(P05) | 운영자 키 | MerchantAttestation, TimeAnchor [N05][N06] | 기기 |
| 가맹점 | 가맹점 서명 키 | MerchantOrder | 기기 |
| 키오스크(P04) | 가스용 키오스크 키 | 트랜잭션 제출만 | 체인 |

기기는 provisioning 때 운영자 주소를 한 번 기록하고 그 주소로 MerchantAttestation과 TimeAnchor를 검증한다. 키오스크는 기기와 가맹점 사이를 중계할 뿐 어떤 서명도 대신 만들 수 없다. 온체인 registry가 가맹점의 최종 권한이며, 기기가 오프라인이라 모르는 가맹점 철회는 컨트랙트가 MERCHANT_REVOKED로 막는다 [N05].

## 2. 서명 타입

기기는 EIP-712 타입 두 개만 서명한다 [N04]. 다른 타입, 원시 트랜잭션, approve, Permit 요청은 `UNSUPPORTED_TYPE`으로 거절한다.

- **PaymentAuthorization** `{chainId, contract, merchant, payout, token, amount, orderId, nonce, expiry}` — 결제 1건. `nonce`는 기기가 고르는 256비트 unordered nonce이고 `expiry`는 `authorizationExpiry` 안이어야 한다 [N13].
- **LimitChange** `{chainId, contract, perPaymentLimit, dailyLimit, nonce, expiry}` — 사용자가 자기 한도를 register의 상한(`perPaymentCap`, `dailyCap`) 안에서 낮추거나 되돌린다. 기기에서 PIN과 버튼을 모두 요구한다 [N11].

운영자와 가맹점이 서명하는 타입은 `operatorSignedTypes`에 있다. MerchantAttestation `{merchant, payout, name, validFrom, validUntil}`의 유효 기간은 `attestationValidity`이고 [N05], MerchantOrder는 가맹점 키가 주문 내용을 보증한다. TimeAnchor `{device, timestamp}`는 아래 5절에서 쓴다 [N06].

EIP-712 domain은 `{name: "NU54 Payment Settlement", version: "1", chainId: 8283, verifyingContract: 정산 컨트랙트}`다. 구현은 eip712-vectors.json의 digest를 그대로 재현해야 하며, P10의 적합성 harness가 펌웨어·키오스크·컨트랙트 시험에서 같은 벡터를 돌린다 [N21].

## 3. 연결과 페어링

BLE GATT가 유일한 규범 전송이다 [N09]. 키오스크가 central, 기기가 peripheral이다. 서비스에는 키오스크→기기 `rx`(write)와 기기→키오스크 `tx`(notify) 두 characteristic이 있고, 둘 다 LE Secure Connections로 인증·암호화된 링크에서만 열린다. UUID는 스키마의 `gatt` 항목을 따른다.

1. **발견.** 키오스크는 기기의 NFC 태그에서 BLE 주소와 LESC out-of-band 데이터를 읽는다. NFC를 못 쓰면 서비스 UUID로 BLE scan을 해서 찾는다. NFC는 W6 게이트 조건이 아니다 [N09].
2. **페어링.** NFC OOB 데이터가 있으면 OOB로, 없으면 Numeric Comparison으로 페어링한다. 기기 화면과 키오스크 화면에 같은 6자리 숫자가 뜨고 사용자가 기기 버튼으로 확인한다. 기기 화면을 쓸 수 없는 시험 환경에서만 Passkey entry를 쓴다.
3. **USB CDC.** 개발용 시험 harness로만 쓰며 게이트 증거로 인정하지 않는다.

## 4. 메시지 틀

모든 메시지는 deterministic CBOR map이다. 전송 전에 다음 envelope로 감싼다.

`length(u16, big-endian) | digest(CBOR 본문 SHA-256의 앞 8바이트) | CBOR 본문`

envelope가 협상된 ATT MTU보다 크면 조각으로 나눈다. 각 조각은 `sequence(u8) | index(u8) | 데이터`이며 데이터 길이는 `ATT_MTU - 5`까지다. `sequence`는 방향별 메시지 번호로 메시지마다 1씩 늘고, `index`는 0부터 센다. 받는 쪽은 `length`만큼 모일 때까지 이어 붙인 뒤 digest를 확인한다. 순서가 어긋나거나 digest가 다르거나 본문이 2048바이트를 넘으면 버리고 `error{BAD_FRAME}`을 보낸다.

## 5. 대여 셋업과 TimeAnchor

기기에는 믿을 수 있는 시계가 없다. 그래서 대여 셋업 때 운영자가 서명한 TimeAnchor를 기록하고 그 뒤로는 RTC로 시간을 이어 간다 [N06].

1. 반납된 기기는 device.reset으로 키와 기록을 지운다 [N11].
2. 셋업에서 기기가 TRNG로 새 키를 만들고 사용자가 PIN을 정한다.
3. 운영자 도구가 `setup.timeAnchor{timestamp, operatorSignature}`를 BLE 셋업 세션으로 보낸다. 목표 경로는 대여자 휴대폰의 설정 앱(P02)이지만 P02는 이번 사이클에서 설계만 하므로 P05 provisioning 스크립트가 같은 메시지를 보낸다.
4. 기기는 서명자가 운영자 주소이고 `device`가 자기 주소이며 `timestamp`가 이전 anchor보다 늦을 때만 받아들인다.
5. 전원이 끊기면 anchor는 무효가 된다. 이때 `session.open.ok.anchorValid=false`를 보내고 셋업을 다시 할 때까지 결제를 `TIME_ANCHOR_MISSING`으로 거절한다.

## 6. 결제 흐름

| 순서 | 메시지 | 방향 | 기기가 하는 일 |
|---|---|---|---|
| 1 | `session.open{sessionId, kioskNonce}` | 키오스크→기기 | `session.open.ok{device, deviceNonce, anchorValid, firmware}`로 답한다 |
| 2 | `session.confirm{sessionId, deviceNonce}` | 키오스크→기기 | 자기가 보낸 deviceNonce와 같을 때만 세션을 연다 |
| 3 | `payment.identify{attestation}` | 키오스크→기기 | MerchantAttestation 서명자가 운영자인지, anchor 시간으로 `validFrom..validUntil` 안인지 확인하고 가맹점 이름을 표시한다 |
| 4 | `payment.prepare{authorization, merchantSignature}` | 키오스크→기기 | MerchantOrder 서명자가 attestation의 merchant인지, payout이 attestation과 같은지 확인하고 orderId·token·payout·amount를 표시한 뒤 버튼을 기다린다 |
| 5 | `payment.result{outcome, signature \| reason}` | 기기→키오스크 | 버튼을 누르면 PaymentAuthorization 서명과 `approved`, 거절하거나 검증에 실패하면 `refused`와 reason을 보낸다 |

기기 거절 사유는 `ATTESTATION_EXPIRED`(anchor 시간 기준 만료), `MERCHANT_FORGED`(attestation 또는 주문 서명자 불일치), `USER_REJECTED`, `TIME_ANCHOR_MISSING`이다. 기기의 표시 내용과 서명 내용이 다를 수 없도록, 표시는 서명할 PaymentAuthorization 필드에서 직접 만든다.

## 7. 제출과 판정

키오스크는 서명을 받은 뒤 다음 순서로 처리한다 [N10].

1. 키오스크 가스 잔액이 `kioskMinGasBalance`보다 적으면 새 결제를 받지 않고 busy를 표시한다.
2. `settle(...)`을 eth_call로 시뮬레이션한다. revert 사유가 `MERCHANT_REVOKED`, `OVER_CAP`, `NONCE_REPLAYED`, `MERCHANT_FORGED`이면 제출하지 않고 refused로 보고한다 [N22].
3. 통과하면 트랜잭션을 보내고 finalized 블록에서 `PaymentSettled(merchant, orderId)` 이벤트를 찾는다. 찾으면 approved다 [N08]. StableNet 8283은 1초 블록이고 finalized가 latest와 같다.
4. 같은 orderId로 `ORDER_ALREADY_PAID` revert가 나면 이미 결제된 것이므로 성공으로 본다.
5. 제출된 트랜잭션이 status=0이면 failed와 줄인 tx hash를 보고한다.
6. 요청 전달이 끝난 시점부터 10 s 안에 결과가 없으면 Checking으로 바꾸고 "다시 결제하지 마세요"를 표시한다. 재결제는 막고, 같은 서명을 다시 보내는 것 말고는 자동 재시도하지 않는다.

키오스크가 사용자에게 보이는 결과는 approved, refused, failed, Checking 네 가지다. P07은 영수증 조회용이며 이 판정에 쓰지 않는다 [N08].

## 8. 거절 코드

| 코드 | 거절하는 층 | 조건 |
|---|---|---|
| `ATTESTATION_EXPIRED` | 기기 | anchor 기반 현재 시간이 attestation `validUntil`을 넘었다 |
| `MERCHANT_REVOKED` | 컨트랙트 | registry에서 가맹점이 철회되었다 |
| `OVER_CAP` | 컨트랙트 | 건당 또는 일일 한도를 넘는다 |
| `NONCE_REPLAYED` | 컨트랙트 | 이미 쓴 nonce다 |
| `MERCHANT_FORGED` | 기기와 컨트랙트 | 서명자나 payout이 attestation 또는 registry와 다르다 |
| `USER_REJECTED` | 기기 | 대여자가 거절 버튼을 눌렀다 |
| `UNSUPPORTED_TYPE` | 기기 | 두 서명 타입 밖의 요청(원시 트랜잭션, approve, Permit)이다 |
| `TIME_ANCHOR_MISSING` | 기기 | TimeAnchor가 없거나 전원 손실로 무효다 |
| `TIMEOUT` | 기기 | 버튼 입력을 기다리다 시간이 지났다 |
| `BAD_FRAME` | 기기와 키오스크 | 조각 순서, 길이, digest가 맞지 않는다 |

W12-05는 앞의 다섯 코드와 `USER_REJECTED`, `UNSUPPORTED_TYPE`, `TIME_ANCHOR_MISSING`을 시연한다 [N18]. `TIMEOUT`과 `BAD_FRAME`은 시험으로만 확인한다.

## 9. 이전 설계에서 없앤 것

- proximityRef 필드는 removed. 근접 증명은 BLE LESC 페어링과 기기 버튼으로 대신한다.
- QR 경로는 없다. 기기에 카메라가 없다.
- 기기가 원시 트랜잭션에 서명하던 경로는 없앴다. 트랜잭션은 키오스크가 내고 가스도 키오스크가 낸다 [N10].

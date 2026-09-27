# 결제 프로토콜 v1

NU-54V-DK 기기(P01), 키오스크(P04), 운영 백오피스(P05), 정산 컨트랙트(P06)가 주고받는 메시지와 서명 형식을 정한다. 이 문서는 규칙을 설명하고, 타입과 필드는 [payment-protocol.schema.json](payment-protocol.schema.json)이, 서명 해시의 정답은 [eip712-vectors.json](eip712-vectors.json)이 정한다 [N21]. 값(한도, 유효 기간, 가스 잔액, 시계 오차)은 [DF-20260925-02](../../planning/design-freeze-checkpoint-02.md)의 parameters에만 있고 여기서는 이름으로만 부른다 [N13].

## 1. 누가 무엇을 믿는가

| 주체 | 가진 키 | 서명하는 것 | 검증하는 쪽 |
|---|---|---|---|
| 기기(P01) | 대여 셋업 때 만든 기기 키 | PaymentAuthorization, LimitChange [N04] | 컨트랙트 |
| 운영자(P05) | 운영자 키 | MerchantAttestation, TimeAnchor, DeviceReset [N05][N06][N23] | 기기 |
| 가맹점 | 가맹점 서명 키(키오스크에 둔다) | MerchantOrder | 기기 |
| 키오스크(P04) | 가스용 키오스크 키, 가맹점 서명 키 | 트랜잭션 제출, 가맹점 대리 MerchantOrder | 체인, 기기 |

이번 사이클에서 키오스크는 가맹점의 POS이므로 가맹점 서명 키를 갖고 가맹점을 대리한다. 따라서 키오스크가 뚫리면 가맹점이 뚫린 것과 같다. 그래도 기기는 운영자가 서명한 attestation의 payout과 다른 곳으로는 서명하지 않으므로, 뚫린 키오스크가 할 수 있는 일은 대여자가 버튼으로 승인한 금액을 등록된 payout으로 결제받는 것뿐이고 손실은 한도로 제한된다.

온체인 registry가 가맹점의 최종 권한이며, 오프라인 기기가 모르는 가맹점 철회는 컨트랙트가 MERCHANT_REVOKED로 막는다 [N05]. 키오스크에는 특별한 BLE 신원이 없다. 페어링한 central은 누구든 결제 세션을 열 수 있고, 기기는 서명 규칙과 버튼 승인으로만 자신을 지킨다. 셋업과 reset 권한은 5절의 규칙으로만 생긴다 [N23].

## 2. 서명 타입

기기는 EIP-712 타입 두 개만 서명한다 [N04]. 그 밖의 요청(원시 트랜잭션, approve, Permit, 스키마에 없는 메시지)은 `error{UNSUPPORTED_TYPE}`으로 거절한다.

- **PaymentAuthorization** `{chainId, contract, merchant, payout, token, amount, orderId, nonce, expiry}` — 결제 1건. 키오스크는 nonce를 뺀 필드를 보내고, 기기가 256비트 난수 nonce를 골라 서명과 함께 돌려준다. `expiry`는 현재 시각부터 `authorizationExpiry` 안이어야 한다 [N13].
- **LimitChange** `{chainId, contract, perPaymentLimit, dailyLimit, nonce, expiry}` — 대여자가 자기 한도를 register 상한(`perPaymentCap`, `dailyCap`) 안에서 정한다. 한도 0은 "상한을 그대로 적용"이다. nonce는 기기가 고르고 결제와 같은 nonce 공간을 쓰며, `expiry`는 `authorizationExpiry` 안이어야 한다. 기기에서 PIN과 버튼을 모두 요구하고, 키오스크가 `setLimits`로 제출한다 [N11].

운영자와 가맹점이 서명하는 타입은 `operatorSignedTypes`에 있다. MerchantAttestation `{merchant, payout, name, validFrom, validUntil}`의 유효 기간은 `attestationValidity`다 [N05]. MerchantOrder `{orderId, token, amount, payout, expiry}`는 가맹점 키가 주문 내용을 보증한다. TimeAnchor `{device, timestamp}`와 DeviceReset `{device, nonce}`는 5절에서 쓴다.

EIP-712 domain은 `{name: "NU54 Payment Settlement", version: "1", chainId: 8283, verifyingContract: 정산 컨트랙트}`다. 기기는 이 domain의 chainId와 verifyingContract를 셋업 때 기록한 값으로만 만든다. 시험 벡터는 역할마다 다른 키(기기, 운영자, 가맹점)로 서명되어 있어, 서명자 역할을 뒤섞는 구현은 벡터를 통과하지 못한다 [N21].

## 3. 연결과 페어링

BLE GATT가 유일한 규범 전송이다 [N09]. 키오스크나 운영자 도구가 central, 기기가 peripheral이다. 서비스에는 central→기기 `rx`(write)와 기기→central `tx`(notify) 두 characteristic이 있고, 둘 다 LE Secure Connections로 인증·암호화된 링크에서만 열린다. UUID는 스키마의 `gatt` 항목을 따른다.

1. **발견.** central은 기기의 NFC 태그에서 BLE 주소와 LESC out-of-band 데이터를 읽는다. 태그는 기기가 NFCT로 에뮬레이션하며 페어링마다 OOB 값을 새로 쓴다. NFC를 못 쓰면 서비스 UUID로 BLE scan을 해서 찾는다. NFC는 게이트 조건이 아니다 [N09].
2. **페어링.** NFC OOB 데이터가 있으면 OOB로, 없으면 Numeric Comparison으로 페어링한다. 기기 화면과 central 화면에 같은 6자리 숫자가 뜨고 사용자가 기기 버튼으로 확인한다. 기기 화면을 쓸 수 없는 시험 환경에서만 Passkey entry를 쓴다. 페어링만으로는 셋업 권한이 생기지 않는다 [N23].
3. **USB CDC.** 개발용 시험 harness로만 쓰고 게이트 증거로 인정하지 않는다. W12 릴리스 이미지에는 넣지 않는다.

## 4. 메시지 틀

모든 메시지는 deterministic CBOR map이다. 전송 전에 다음 envelope로 감싼다.

`length(u16, big-endian) | digest(CBOR 본문 SHA-256의 앞 8바이트) | CBOR 본문`

`length`는 헤더 10바이트(`length` 2바이트와 digest 8바이트)를 포함한 envelope 전체 길이다. 본문이 최대 2048바이트이므로 `length`는 10 이상 2058 이하이며, 이 범위를 벗어나면 `BAD_FRAME`이다.

모든 envelope는 조각 헤더를 붙여 보낸다(조각이 하나여도 붙인다). 각 조각은 `sequence(u8) | index(u8) | 데이터`이며 데이터 길이는 협상된 MTU에서 `ATT_MTU - 5`까지다. `sequence`는 방향별 메시지 번호로 메시지마다 1씩 늘고, `index`는 0부터 센다. 받는 쪽은 `length`만큼 모일 때까지 이어 붙인 뒤 digest를 확인한다. 순서가 어긋나거나 digest가 다르거나 본문이 2048바이트를 넘으면, 받은 쪽(기기든 central이든)이 메시지를 버리고 `error{BAD_FRAME}`을 보낸 뒤 세션을 닫는다. 키오스크가 아직 서명을 받지 못했으면 새 세션에서 처음부터 다시 시작한다. 이미 서명을 받았으면 세션 없이 7절의 제출 단계로 계속 간다(서명은 키오스크에 있으므로 새 서명을 요청하지 않는다).

## 5. 셋업, TimeAnchor, reset

기기 상태는 `UNPROVISIONED`, `PROVISIONED_NO_ANCHOR`, `READY`, `PIN_LOCKED` 넷이다. `session.open`의 `mode`가 `setup`이면 셋업 세션이고, 셋업 세션은 `UNPROVISIONED` 또는 `PROVISIONED_NO_ANCHOR`에서만 열린다. 다른 상태에서는 `error{NOT_PERMITTED}`로 거절한다 [N23].

**대여 셋업**은 다음 순서로 한다. 대상 기기는 반납 절차의 device.reset으로 `UNPROVISIONED` 상태다.

1. 운영자 도구가 셋업 세션을 연다. `UNPROVISIONED`에서는 키가 없으므로 `session.open.ok`에 `device`가 없다. 운영자 도구가 `setup.operator{operator, contract, chainId}`를 보내면, 기기는 세 값을 화면에 보여 주고 대여자가 버튼으로 확인할 때만 기록한 뒤 `setup.ack{step: setup.operator}`로 답한다. 이 기록은 `UNPROVISIONED`에서 한 번만 가능하다 [N23]. 기기는 운영자 값을 2단계의 키 생성과 함께 한 번에 저장하므로, keygen ack 전에 세션이 끊기면 아무것도 남지 않고 `UNPROVISIONED`로 다시 시작한다.
2. 기기가 TRNG로 새 키를 만들고 대여자가 기기에서 PIN을 정한다. 기기는 `setup.ack{step: keygen, device}`로 새 주소를 알린다 [N11].
3. 운영자 도구는 받은 주소와 최신 finalized 블록 시각으로 TimeAnchor `{device, timestamp}`에 서명해 `setup.timeAnchor{device, timestamp, operatorSignature}`를 보낸다. 기기는 `device`가 자기 주소이고, 서명자가 기록한 운영자 주소이며, `timestamp`가 이전 anchor보다 엄격히 늦을 때만 받아들이고 `setup.ack{step: setup.timeAnchor, accepted, lastAnchor}`로 답한다 [N06].
4. 운영자 도구는 anchor가 받아들여진 뒤에만 `depositFor(device, amount, withdrawAddress)`를 호출한다.

목표 경로는 대여자 휴대폰의 설정 앱(P02)이지만 P02는 이번 사이클에서 설계만 하므로 P05 운영 도구 `opsctl`이 같은 메시지를 보낸다.

**재-anchor.** RAM이 초기화되는 모든 reset(전원 손실, watchdog, System OFF에서 깨어남, serial recovery 뒤 재부팅) 뒤에는 anchor가 무효가 되고 기기는 `PROVISIONED_NO_ANCHOR`가 된다. 키와 예치금은 그대로다. 이 상태에서 결제 요청은 `TIME_ANCHOR_MISSING`으로 거절하고, 운영자 도구가 셋업 세션을 열어 3단계(TimeAnchor)만 다시 하면 `READY`로 돌아간다. 예치는 다시 하지 않는다.

**시간 비교.** 기기의 현재 시각은 마지막 anchor에 RTC 경과 시간을 더한 값이다. attestation 유효 기간을 비교할 때만 `anchorClockSkew`를 허용한다.

**reset.** `device.reset{device, nonce, operatorSignature}`는 운영자가 DeviceReset `{device, nonce}`에 서명한 명령이며 어떤 상태·세션에서도 받는다. 서명자가 기록된 운영자 주소이고 `device`가 자기 주소일 때만 키, PIN, 운영자 주소, 컨트랙트 값, TimeAnchor, nonce 기록을 지우고 `UNPROVISIONED`가 된다. 반납 순서(closeAccount의 finalized 이벤트 확인 → device.reset)는 운영자 도구가 지킨다 [N11].

**PIN 잠금.** PIN을 `pinMaxRetries`번 틀리면 `PIN_LOCKED`가 되어 모든 서명을 거절한다(`PIN_LOCKED`). 잠금 상태는 secure 저장소에 남아 reset 뒤에도 `PIN_LOCKED`이며, 풀 방법은 반납 절차(DeviceReset)뿐이다.

## 6. 결제 흐름

| 순서 | 메시지 | 방향 | 기기가 하는 일 |
|---|---|---|---|
| 1 | `session.open{sessionId, mode: payment, kioskNonce}` | 키오스크→기기 | `session.open.ok{device, deviceNonce, anchorValid, state, lastAnchor, firmware}`로 답한다 |
| 2 | `session.confirm{sessionId, deviceNonce}` | 키오스크→기기 | 자기가 보낸 deviceNonce와 같을 때만 세션을 연다 |
| 3 | `payment.identify{attestation}` | 키오스크→기기 | MerchantAttestation 서명자가 운영자인지, 현재 시각이 `validFrom..validUntil`(± `anchorClockSkew`) 안인지 확인하고 가맹점 이름을 표시한다 |
| 4 | `payment.prepare{authorization, merchantSignature}` | 키오스크→기기 | 아래 검사를 모두 통과하면 orderId·token·payout·amount를 표시하고 버튼을 기다린다 |
| 5 | `payment.result{outcome, signature, nonce \| reason}` | 기기→키오스크 | 버튼을 누르면 nonce를 골라 서명하고 `approved`를, 거절 버튼이나 검사 실패면 `refused`와 reason을 보낸다 |
| 6 | `payment.outcome{orderId, outcome, reason}` | 키오스크→기기 | 키오스크의 최종 결과를 화면에 보여 준다 |

4단계 검사는 다음과 같다. 하나라도 틀리면 `refused`다.

- MerchantOrder 서명자가 attestation의 `merchant`이고, `authorization.merchant`도 attestation의 `merchant`다. 아니면 `MERCHANT_FORGED`.
- `authorization.payout`이 attestation과 MerchantOrder의 payout과 같다. 아니면 `MERCHANT_FORGED`.
- `authorization`의 orderId·token·amount·expiry가 MerchantOrder와 같고, chainId·contract가 셋업 때 기록한 값과 같다. 아니면 `MERCHANT_FORGED`.
- `expiry`가 현재 시각부터 `authorizationExpiry` 안이다. 아니면 `ATTESTATION_EXPIRED`.
- anchor가 유효하다. 아니면 `TIME_ANCHOR_MISSING`.

**한도 변경.** 키오스크가 `limit.change{change}`를 보내면 기기는 PIN과 버튼을 확인하고 nonce를 골라 `limit.result{approved, signature, nonce}`를, 거절하면 `refused`와 reason을 보낸다. 키오스크는 `setLimits`로 제출하고, 컨트랙트는 expiry와 nonce를 결제와 같은 방식으로 확인한다 [N04].

버튼 승인은 서명 권한 경계를 거친다. 기기는 서명할 digest와 purpose를 secure partition에 먼저 등록하고, secure 쪽 버튼 인터럽트는 그 digest 한 건에만 서명 토큰을 발급한다 [N24]. 표시 내용과 서명 내용이 같다는 보장은 non-secure 코드가 무결하다는 가정 아래의 주장이다.

키오스크는 `payment.prepare` 전송이 끝난 뒤 10 s 안에 `payment.result`가 없으면 `session.cancel`을 보낸다. 기기는 버튼 대기를 멈추고 아무것도 서명하지 않는다. 키오스크는 주문을 취소하고, 서명이 없었으므로 다시 결제를 받아도 된다 [N10].

## 7. 제출과 판정

키오스크는 서명을 받은 뒤 다음 순서로 처리한다 [N10].

1. 키오스크는 새 주문을 받기 전(대기 상태)에 가스 잔액을 확인하고, `kioskMinGasBalance`보다 적으면 주문을 받지 않고 busy를 표시한다.
2. `settle(...)`을 eth_call로 시뮬레이션한다. 컨트랙트 검사 순서는 domain → 주문 유일성 → expiry → 서명자 계정 → registry → payout → nonce → 한도 → 잔액이다 [N07].
3. 시뮬레이션이 `OrderAlreadyPaid`이면, finalized `PaymentSettled`를 `merchant`와 `orderId`(둘 다 indexed)로 조회한다. 이벤트의 device·amount·nonce가 자기가 받은 서명과 같으면 approved다. 다르면 다른 결제로 이미 처리된 주문이므로 refused로 보고한다 [N08].
4. 다른 custom error이면 8절 표에 따라 refused로 보고하고 제출하지 않는다 [N22].
5. 통과하면 트랜잭션을 보내고 finalized 블록에서 `PaymentSettled(address indexed merchant, bytes32 indexed orderId, address indexed device, uint256 amount, uint256 nonce)`를 찾는다. 찾으면 approved다. StableNet 8283은 1초 블록이고 finalized가 latest와 같다.
6. 채굴된 트랜잭션이 status=0이면 같은 호출을 한 번 더 시뮬레이션한다. 결과가 `OrderAlreadyPaid`이면 3번 판정을 적용하고, 그 밖의 결과면 재제출하지 않고 failed와 줄인 tx hash를 보고한다.
7. `payment.prepare` 전송이 끝난 시점부터 10 s 안에 5번 결과가 없으면 Checking으로 바꾸고 "다시 결제하지 마세요"를 표시한다. Checking에서는 이벤트를 계속 찾고, 같은 서명을 다시 보낼 때도 2–3번을 먼저 거친다. 체인 시각이 서명의 `expiry`를 넘었는데도 이벤트가 없으면(재시뮬레이션이 `Expired`) 그 서명은 더는 정산될 수 없으므로 refused가 아니라 failed로 끝내고 재결제를 허용한다.

키오스크가 사용자에게 보이는 결과는 approved, refused, failed, Checking 네 가지다. P07은 영수증 조회용이며 이 판정에 쓰지 않는다 [N08].

## 8. 거절 코드

| 코드 | 거절하는 층 | 보내는 방법 | 조건 | W12-05 |
|---|---|---|---|---|
| `ATTESTATION_EXPIRED` | 기기 | payment.result refused | attestation 유효 기간 밖이거나 authorization expiry가 너무 멀다 | 시연(합의) |
| `MERCHANT_REVOKED` | 컨트랙트 | 시뮬레이션 revert | registry에서 가맹점이 철회되었다 | 시연(합의) |
| `OVER_CAP` | 컨트랙트 | 시뮬레이션 revert | 건당 또는 일일 한도를 넘는다 | 시연(합의) |
| `NONCE_REPLAYED` | 컨트랙트 | 시뮬레이션 revert | 이미 쓴 nonce다. 결제 재전송은 주문 유일성에서 먼저 걸리므로, 실제로는 적용된 LimitChange를 그 expiry 안에 다시 제출할 때 난다 | 시연(합의) |
| `MERCHANT_FORGED` | 기기와 컨트랙트 | refused 또는 revert | 가맹점 서명자, merchant, payout, 주문 내용이 attestation·registry와 다르다 | 시연(합의, 기기 층). 컨트랙트 층은 P06 시험 |
| `USER_REJECTED` | 기기 | payment.result refused | 대여자가 거절 버튼을 눌렀다 | 시연(기기) |
| `TIME_ANCHOR_MISSING` | 기기 | payment.result refused | TimeAnchor가 없거나 reset으로 무효다 | 시연(기기) |
| `UNSUPPORTED_TYPE` | 기기 | error | 두 서명 타입 밖의 요청이다 | 시연(기기) |
| `BAD_FRAME` | 기기와 키오스크 | error | 조각 순서, 길이, digest가 맞지 않는다 | 시험 |
| `TIMEOUT` | 기기 | payment.result refused | 버튼을 기다리다 서명의 expiry를 넘겼다 | 시험 |
| `EXPIRED` | 컨트랙트 | 시뮬레이션 revert | 체인 시각이 expiry를 넘었다 | 시험 |
| `WRONG_DOMAIN` | 컨트랙트 | 시뮬레이션 revert | chainId, contract, token이 다르다 | 시험 |
| `ACCOUNT_INACTIVE` | 컨트랙트 | 시뮬레이션 revert | 서명자 계정이 없거나 닫혔다 | 시험 |
| `INSUFFICIENT_BALANCE` | 컨트랙트 | 시뮬레이션 revert | 예치 잔액이 모자란다 | 시험 |

`CANCELLED`, `PIN_LOCKED`, `NOT_PERMITTED`는 세션·상태 코드이며 거절 시연 대상이 아니다. W12-05는 합의 5종과 기기 3종을 시연한다 [N18][N22].

## 9. 이전 설계에서 없앤 것

- proximityRef 필드는 removed. 근접 증명은 BLE LESC 페어링과 기기 버튼으로 대신한다.
- QR 경로는 없다. 기기에 카메라가 없다.
- 기기가 원시 트랜잭션에 서명하던 경로는 없앴다. 트랜잭션은 키오스크가 내고 가스도 키오스크가 낸다 [N10].

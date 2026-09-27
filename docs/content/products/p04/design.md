# P04 설계 — 가맹점 키오스크

[SRS](srs.md)의 요구사항을 React Native 앱 구조로 옮긴다. 전송·서명 형식은 [결제 프로토콜](../../specifications/protocol/payment-protocol.md)과 [스키마](../../specifications/protocol/payment-protocol.schema.json)가 기준이다 [N09].

## 1. 구조

| 모듈 | 책임 |
|---|---|
| `ble/central` | NFC handover 읽기, BLE scan, LE Secure Connections 페어링, rx write와 tx notify 구독 |
| `ble/framing` | envelope(length, digest) 생성·검증, MTU에 맞춘 조각 분할과 sequence/index 재조립 |
| `protocol/codec` | deterministic CBOR 인코딩·디코딩, 메시지 스키마 검증 |
| `merchant/signer` | 가맹점 서명 키로 MerchantOrder EIP-712 서명(가맹점 대리) |
| `chain/client` | eth_call 시뮬레이션, eth_sendRawTransaction, finalized 태그 기준 eth_getLogs |
| `chain/gas` | 키오스크 잔액 조회와 kioskMinGasBalance 비교 [N10] |
| `orders/store` | 주문, 서명, nonce, 제출 상태의 영속 저장 |
| `ui/flow` | 결제 상태 기계와 화면 |

## 2. 연결과 조각 처리

키오스크는 central이고 기기는 peripheral이다. NFC로 BLE 주소와 OOB 데이터를 얻으면 OOB 페어링을, 없으면 BLE scan 후 Numeric Comparison 페어링을 쓴다 [N09]. 연결 후 ATT MTU를 협상하고, 조각 크기를 `ATT_MTU - 5`로 둔다. 모든 envelope에 조각 헤더를 붙인다. 받는 쪽은 sequence가 연속인지, index가 0부터 이어지는지 확인하고 length만큼 모이면 digest를 검증한다. 어긋나면 `error{BAD_FRAME}`을 보내고 세션을 닫는다. 서명을 받기 전이면 session.open부터 다시 시작하고, 이미 서명을 받았으면 세션 없이 제출 단계를 계속한다.

## 3. 결제 상태 기계

```
Idle → Pairing → Session → Identifying → Preparing → WaitingDevice
WaitingDevice → Simulating      (approved 서명·nonce 수신)
WaitingDevice → Refused         (기기 refused, TIMEOUT 포함)
WaitingDevice → Cancelled       (요청 전달 완료 후 10 s 경과 → session.cancel, 재결제 허용)
Simulating    → Approved        (ORDER_ALREADY_PAID이고 finalized 이벤트의 device·amount·nonce 일치)
Simulating    → Refused         (그 밖의 custom error → 코드 매핑, 또는 다른 결제로 처리된 주문)
Simulating    → Submitted       (시뮬레이션 통과, 트랜잭션 전송)
Submitted     → Approved        (finalized PaymentSettled의 device·amount·nonce 일치)
Submitted     → Resimulating    (채굴된 트랜잭션 status=0 → 한 번 재시뮬레이션)
Submitted     → Checking        (요청 전달 완료 후 10 s 경과)
Resimulating  → Approved        (ORDER_ALREADY_PAID이고 이벤트 일치)
Resimulating  → Refused         (ORDER_ALREADY_PAID이고 이벤트 불일치 → 다른 결제로 처리된 주문)
Resimulating  → Failed          (그 밖의 모든 결과, 자동 재제출 없음)
Checking      → Approved        (finalized 이벤트 확인)
Checking      → Rechecking      (같은 서명 재전송 전 시뮬레이션)
Rechecking    → Submitted       (시뮬레이션 통과 → 같은 서명 재전송)
Rechecking    → Approved        (ORDER_ALREADY_PAID이고 이벤트 일치)
Rechecking    → Refused         (ORDER_ALREADY_PAID이고 이벤트 불일치)
Rechecking    → Failed          (Expired이고 이벤트 없음 → 더는 정산될 수 없음, 재결제 허용)
Rechecking    → Checking        (그 밖의 결과 → 이벤트를 계속 찾고 expiry 뒤 다시 확인)
WaitingDevice → Idle            (error BAD_FRAME, 연결 끊김: 서명 수신 전 → 세션 종료 후 session.open부터 재시도)
Simulating    → Simulating      (서명 수신 후 연결 끊김: 세션 없이 제출 단계 계속, 새 서명 요청 없음)
any           → Refused         (error UNSUPPORTED_TYPE 또는 NOT_PERMITTED)
```

사용자에게 보이는 결과는 approved, refused, failed, Checking 네 가지다. Cancelled는 서명이 없던 주문의 취소이며 결과로 표시하지 않는다. 최종 결과가 정해지면 payment.outcome으로 기기에 알린다. Checking 상태의 주문은 새 서명을 요청하지 않는다 [N10].

## 4. 체인 처리

1. 새 주문을 받기 전(Idle)에 잔액을 확인하고, kioskMinGasBalance 미만이면 주문을 받지 않고 busy를 표시한다. W12 시작 잔액은 [수용 기록지](../../acceptance/week12-log.md) 1절을 따른다.
2. 서명을 받으면 settle 호출 데이터를 만들어 eth_call로 시뮬레이션한다. revert 데이터는 P06 ABI의 custom error로 디코딩해 7절 표로 바꾼다 [N22].
3. 시뮬레이션이 ORDER_ALREADY_PAID이면 `PaymentSettled(address indexed merchant, bytes32 indexed orderId, address indexed device, uint256 amount, uint256 nonce)`를 merchant·orderId topic으로 finalized 조회해 device·amount·nonce를 자기 서명과 비교한다 [N08].
4. 통과하면 가스 키로 서명한 트랜잭션을 보낸다. 가스는 키오스크가 낸다.
5. 1초 주기로 finalized 태그 기준 같은 이벤트를 조회한다.

## 5. 키 관리

키오스크에는 키가 둘 있다. 가스 키는 트랜잭션 제출에만, 가맹점 서명 키는 가맹점 대리 MerchantOrder 서명에만 쓰며 둘 다 Android Keystore에 둔다. 사용자 결제 서명은 항상 기기에서 오며 키오스크는 PaymentAuthorization을 만들거나 고칠 수 없다 [N04]. 키오스크가 탈취되면 가맹점 서명 키도 함께 노출되지만, 기기는 운영자가 서명한 attestation의 payout과 다른 곳으로 서명하지 않고 버튼 승인을 요구하므로, 공격자가 할 수 있는 일은 등록된 payout으로 한도 안의 결제를 대여자 승인 아래 받는 것뿐이다 [N05].

## 6. 저장

| 레코드 | 필드 | 용도 |
|---|---|---|
| order | orderId, amount, token, merchant, payout, 상태 | 화면과 재시작 복원 |
| authorization | orderId, 서명, nonce(기기가 고른 값), expiry | 같은 서명 재전송과 이벤트 비교 |
| submission | 줄인 tx hash, 제출 시각, 결과 | failed·Checking 표시 |

앱이 다시 시작되면 Checking 주문을 불러와 저장된 서명으로 시뮬레이션·재전송하고 이벤트 조회를 이어 간다.

## 7. 오류 매핑

| 원인 | 결과 | 표시 |
|---|---|---|
| 기기 refused(reason) | refused | reason 그대로 |
| `MerchantRevoked` | refused | MERCHANT_REVOKED |
| `OverCap` | refused | OVER_CAP |
| `NonceReplayed` | refused | NONCE_REPLAYED |
| `MerchantForged` | refused | MERCHANT_FORGED |
| `Expired` | refused | EXPIRED(시험 전용) |
| `WrongDomain` | refused | WRONG_DOMAIN(시험 전용) |
| `AccountInactive` | refused | ACCOUNT_INACTIVE(시험 전용) |
| `InsufficientBalance` | refused | INSUFFICIENT_BALANCE(시험 전용) |
| `OrderAlreadyPaid`, 이벤트 일치 | approved | 결제 완료 |
| `OrderAlreadyPaid`, 이벤트 불일치 | refused | 다른 결제로 처리된 주문 |
| status=0, 재시뮬레이션이 ORDER_ALREADY_PAID 일치가 아님 | failed | 줄인 tx hash(자동 재제출 없음) |
| Checking 중 Expired, 이벤트 없음 | failed | 다시 결제해 주세요 |
| 기기 refused(TIMEOUT) | refused | 시간 초과, 주문 취소 가능(시험 전용) |
| 서명 전 10 s 경과 | 취소 | 다시 결제해 주세요 |
| 제출 후 10 s 경과 | Checking | 다시 결제하지 마세요 |
| error UNSUPPORTED_TYPE, NOT_PERMITTED | refused | 코드 그대로 |
| error BAD_FRAME, 연결 끊김 | 서명 수신 전이면 세션을 닫고 Idle(재시도 가능), 후면 세션 없이 제출 계속(제출 전에는 Checking으로 가지 않음) | 재연결 |

ATTESTATION_EXPIRED는 기기가 payment.result로 보내며 revert로 오지 않는다. 마지막 네 컨트랙트 코드는 시험으로만 확인하고 W12 거절 시연 대상이 아니다.

## 8. 시험 설계

- `protocol/codec`: [eip712-vectors.json](../../specifications/protocol/eip712-vectors.json)의 PaymentAuthorization·MerchantOrder 벡터로 digest를 재현하고 역할별 서명 복원 주소를 확인한다 [N21].
- `ble/framing`: MTU 23, 185, 247에서 분할·재조립, 순서 바뀜과 digest 불일치 거부.
- `chain/client`: 로컬 anvil에 P06 컨트랙트를 올려 custom error 매핑, 같은 서명 재제출의 ORDER_ALREADY_PAID 처리, LimitChange 재제출의 NONCE_REPLAYED를 시험한다.
- 상태 기계: 서명 전·제출 후 10 s 타이머, expiry 경과, 재시작 복원을 시간 주입으로 시험한다.

# P04 설계 — 가맹점 키오스크

[SRS](srs.md)의 요구사항을 React Native 앱 구조로 옮긴다. 전송·서명 형식은 [결제 프로토콜](../../specifications/protocol/payment-protocol.md)과 [스키마](../../specifications/protocol/payment-protocol.schema.json)가 기준이다 [N09].

## 1. 구조

| 모듈 | 책임 |
|---|---|
| `ble/central` | NFC handover 읽기, BLE scan, LE Secure Connections 페어링, rx write와 tx notify 구독 |
| `ble/framing` | envelope(length, digest) 생성·검증, MTU에 맞춘 조각 분할과 sequence/index 재조립 |
| `protocol/codec` | deterministic CBOR 인코딩·디코딩, 메시지 스키마 검증 |
| `chain/client` | eth_call 시뮬레이션, eth_sendRawTransaction, finalized 태그 기준 eth_getLogs |
| `chain/gas` | 키오스크 잔액 조회와 kioskMinGasBalance 비교 [N10] |
| `orders/store` | 주문, 서명, 제출 상태의 영속 저장 |
| `ui/flow` | 결제 상태 기계와 화면 |

## 2. 연결과 조각 처리

키오스크는 central이고 기기는 peripheral이다. NFC로 BLE 주소와 OOB 데이터를 얻으면 OOB 페어링을, 없으면 BLE scan 후 Numeric Comparison 페어링을 쓴다 [N09]. 연결 후 ATT MTU를 협상하고, 조각 크기를 `ATT_MTU - 5`로 둔다. 받는 쪽은 sequence가 연속인지, index가 0부터 이어지는지 확인하고 length만큼 모이면 digest를 검증한다. 어긋나면 `error{BAD_FRAME}`을 보내고 세션을 닫는다.

## 3. 결제 상태 기계

```
Idle → Pairing → Session → Identifying → Preparing → WaitingDevice
WaitingDevice → Simulating      (approved 서명 수신)
WaitingDevice → Refused         (기기 refused)
WaitingDevice → Checking        (요청 전달 완료 후 10 s 경과)
Simulating    → Refused         (eth_call revert → 코드 매핑)
Simulating    → Submitted       (시뮬레이션 통과, 트랜잭션 전송)
Submitted     → Approved        (finalized PaymentSettled 또는 ORDER_ALREADY_PAID)
Submitted     → Failed          (status=0)
Submitted     → Checking        (10 s 경과)
Checking      → Approved        (같은 서명 재전송 후 finalized 이벤트 확인)
```

사용자에게 보이는 결과는 approved, refused, failed, Checking 네 가지다. Checking 상태의 주문은 새 서명을 요청하지 않는다 [N10].

## 4. 체인 처리

1. 잔액이 kioskMinGasBalance 미만이면 Idle에서 새 주문을 받지 않는다.
2. 서명을 받으면 settle 호출 데이터를 만들어 eth_call로 시뮬레이션한다. revert 데이터는 P06 ABI의 custom error로 디코딩해 거절 코드로 바꾼다 [N22].
3. 통과하면 키오스크 키로 서명한 트랜잭션을 보낸다. 가스는 키오스크가 낸다.
4. 1초 주기로 finalized 태그 기준 `PaymentSettled(merchant, orderId)` 로그를 조회한다 [N08].

## 5. 키 관리

키오스크 키는 가스 지불 전용이다. Android Keystore에 보관하고 트랜잭션 서명에만 쓴다. 사용자 결제 서명은 항상 기기에서 오며 키오스크는 PaymentAuthorization을 만들거나 고칠 수 없다 [N04].

## 6. 저장

| 레코드 | 필드 | 용도 |
|---|---|---|
| order | orderId, amount, token, merchant, payout, 상태 | 화면과 재시작 복원 |
| authorization | orderId, 서명, nonce, expiry | 같은 서명 재전송 |
| submission | 줄인 tx hash, 제출 시각, 결과 | failed·Checking 표시 |

앱이 다시 시작되면 Checking 주문을 불러와 저장된 서명으로 재전송하고 이벤트 조회를 이어 간다.

## 7. 오류 매핑

| 원인 | 결과 | 표시 |
|---|---|---|
| 기기 refused(reason) | refused | reason 그대로 |
| eth_call revert | refused | 매핑된 거절 코드 |
| ORDER_ALREADY_PAID | approved | 결제 완료 |
| status=0 | failed | 줄인 tx hash |
| 10 s 경과 | Checking | 다시 결제하지 마세요 |
| BAD_FRAME, 연결 끊김 | Checking 또는 Idle | 서명 수신 전이면 Idle |

## 8. 시험 설계

- `protocol/codec`: [eip712-vectors.json](../../specifications/protocol/eip712-vectors.json)의 PaymentAuthorization 벡터로 digest를 재현하고 서명 복원 주소를 확인한다 [N21].
- `ble/framing`: MTU 23, 185, 247에서 분할·재조립, 순서 바뀜과 digest 불일치 거부.
- `chain/client`: 로컬 anvil에 P06 컨트랙트를 올려 컨트랙트 거절 코드 매핑과 ORDER_ALREADY_PAID 처리를 시험한다.
- 상태 기계: 10 s 타이머와 재시작 복원을 시간 주입으로 시험한다.

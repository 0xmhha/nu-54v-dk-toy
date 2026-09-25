# P06 설계 — 정산 컨트랙트

[srs.md](srs.md)의 요구를 만족하는 컨트랙트 구조다. 타입과 도메인은 [payment-protocol.schema.json](../../specifications/protocol/payment-protocol.schema.json), 해시 정답은 [eip712-vectors.json](../../specifications/protocol/eip712-vectors.json)이 정한다. 파라미터 perPaymentCap, dailyCap, withdrawalDelay, payoutChangeDelay, authorizationExpiry는 [register](../../planning/design-freeze-checkpoint-02.md)의 값을 배포 constructor 인자로 받고 immutable로 둔다 [N13].

## 1. 구성

| 계약 | 책임 |
|---|---|
| `PaymentSettlement` | 기기 계정 예치, settle, 한도, nonce, 출금, closeAccount, 가맹점 잔액과 cashOut |
| `MerchantRegistry` | 가맹점 등록·철회·payout 변경 대기열. settle이 읽는 최종 가맹점 권한 [N05] |

두 계약을 나누는 이유는 registry admin 키와 운영자 키를 분리하기 위해서다. 키오스크 EOA는 어떤 특별 권한도 없다. 누구나 settle을 제출할 수 있지만 유효한 기기 서명 없이는 아무것도 바뀌지 않는다.

기기가 검증하는 MerchantAttestation은 오프체인 문서이고, 컨트랙트는 attestation을 읽지 않는다. 컨트랙트는 registry만 본다. 두 값이 어긋나는 경우(철회, payout 변경)는 registry가 이긴다.

## 2. 저장소 레이아웃

```solidity
struct Account {            // key: device address
    uint128 balance;
    uint64  closedAt;       // 0 = active
    uint64  withdrawAfter;  // pending withdrawal release time
    address withdrawAddress;
    uint128 pendingWithdrawal; // requested amount; balance is not moved until execute
    uint128 perPaymentLimit; // from LimitChange, <= perPaymentCap; 0 = use perPaymentCap
    uint128 dailyLimit;      // from LimitChange, <= dailyCap; 0 = use dailyCap
    uint64  windowStart;
    uint128 windowSpent;
}
mapping(address => Account) accounts;
mapping(address => mapping(uint256 => uint256)) nonceBitmap; // device => word => bits
mapping(address => mapping(bytes32 => bool)) paid;            // merchant => orderId
mapping(address => uint256) merchantBalance;

// MerchantRegistry
struct Merchant { bool active; address payout; address pendingPayout; uint64 payoutEffectiveAt; }
mapping(address => Merchant) merchants;
```

## 3. 함수·이벤트·오류

| 함수 | 호출자 | 동작 |
|---|---|---|
| `depositFor(device, amount, withdrawAddress)` | 운영자 | 토큰을 받아 잔액 증가, 첫 호출에만 withdrawAddress 기록. 닫힌 계정이면 revert [N07] |
| `settle(PaymentAuthorization auth, bytes sig)` | 누구나(키오스크) | 5절의 검사 후 잔액 이동, PaymentSettled |
| `setLimits(LimitChange change, bytes sig)` | 누구나(키오스크) | 기기 서명, expiry(authorizationExpiry 안), 결제와 공유하는 nonce bitmap을 확인한 뒤 한도 변경 |
| `requestWithdrawal(device, amount)` | 운영자 | pendingWithdrawal = amount, withdrawAfter = now + withdrawalDelay. 잔액은 옮기지 않는다 |
| `cancelWithdrawal(device)` | 운영자 | 지연 중 대기 출금을 지운다(오조작 되돌리기). 닫힌 계정에는 쓸 수 없다 |
| `executeWithdrawal(device)` | 누구나 | 지연 경과 시 min(pendingWithdrawal, balance)를 잔액에서 빼 withdrawAddress로 전송 |
| `closeAccount(device)` | 운영자 | closedAt 기록, pendingWithdrawal = 전체 잔액, withdrawAfter = now + withdrawalDelay |
| `registerMerchant/revokeMerchant` | registry admin | 가맹점 등록·즉시 철회 |
| `requestPayoutChange(merchant, payout)` | registry admin | pendingPayout 기록, payoutEffectiveAt = now + payoutChangeDelay |
| `cancelPayoutChange(merchant)` | registry admin | 효력 전 pendingPayout을 지운다 |
| `cashOut()` | 가맹점 | merchantBalance를 현재 payout으로 전송 |

| 이벤트 | 용도 |
|---|---|
| `Deposited(device, amount)` | 예치 관측 |
| `PaymentSettled(address indexed merchant, bytes32 indexed orderId, address indexed device, uint256 amount, uint256 nonce)` | paid 판정의 유일한 근거. P04와 P07은 merchant·orderId topic으로 조회한다 [N08] |
| `LimitsChanged(device, perPayment, daily)` | 한도 이력 |
| `WithdrawalRequested/Executed(device, amount)` | 출금 관측 |
| `AccountClosed(device)` | 반납 관측 |
| `WithdrawalCancelled(device)` | 출금 취소 관측 |
| `MerchantRegistered/Revoked/PayoutChangeQueued/PayoutChangeCancelled(merchant, ...)` | registry 이력. payout 변경은 이 이벤트로 공개되어 가맹점이 확인할 수 있다 |

오류: `WrongDomain`, `OrderAlreadyPaid`(ORDER_ALREADY_PAID), `Expired`, `AccountInactive`, `MerchantRevoked`, `MerchantForged`, `NonceReplayed`, `OverCap`, `InsufficientBalance`, `NotOperator`. 키오스크의 매핑은 srs 3절에 있다.

## 4. EIP-712 해시

도메인은 `{name: "NU54 Payment Settlement", version: "1", chainId: 8283, verifyingContract: this}`다. typehash는 스키마의 `encodeType` 문자열을 그대로 keccak한다. 시험은 `vm.readFile`로 eip712-vectors.json을 읽어 PaymentAuthorization·LimitChange 벡터의 digest를 재계산하고, `ecrecover` 결과가 벡터의 signer와 같은지 확인한다. 벡터의 verifyingContract가 시험 배포 주소와 다르므로 시험용 해시 함수는 도메인 separator를 인자로 받는다.

서명은 low-s만 받는다. `ecrecover`가 0 주소나 계정 없는 주소를 돌려주면 AccountInactive로 거절한다. 컨트랙트 층의 MerchantForged는 payout 불일치 하나뿐이다.

## 5. settle 검사 순서

checks-effects-interactions를 지킨다. settle에는 외부 호출이 없다.

순서는 [N07]을 따른다.

1. `auth.chainId == block.chainid`, `auth.contract == address(this)`, `auth.token == token`(배포 토큰) — 아니면 WrongDomain.
2. `paid[auth.merchant][auth.orderId]` — 켜져 있으면 OrderAlreadyPaid. 만료·nonce보다 먼저 보므로 첫 제출이 정산된 서명을 다시 내면 언제나 이 오류로 끝난다(첫 제출이 revert했다면 재제출도 원래 오류, 만료 뒤면 Expired).
3. `block.timestamp <= auth.expiry` 그리고 `auth.expiry <= block.timestamp + authorizationExpiry` — 아니면 Expired.
4. 서명 복원 → device. `accounts[device].closedAt == 0` 그리고 잔액 계정이 있어야 한다 — 아니면 AccountInactive(0 주소, 잘못된 서명자 포함).
5. `merchants[auth.merchant].active` — 아니면 MerchantRevoked.
6. `auth.payout == 현재 유효 payout` — 아니면 MerchantForged.
7. nonce 비트: `word = nonce >> 8`, `bit = 1 << (nonce & 0xff)`. 이미 켜져 있으면 NonceReplayed.
8. 한도: 건당 `amount <= cap(perPaymentCap, perPaymentLimit)`, 여기서 `cap(c, l)`은 l이 0이면 c, 아니면 min(c, l)이다. 일일 창은 첫 결제부터 시작하는 고정 24시간 창이다. `block.timestamp >= windowStart + 1 days`이면 windowStart를 지금으로 옮기고 windowSpent를 0으로 한 뒤 `windowSpent + amount <= cap(dailyCap, dailyLimit)`을 검사 — 아니면 OverCap. 창 경계 양쪽에서 결제하면 임의의 24시간 동안 최대 2 x dailyCap이 나갈 수 있다.
9. `amount <= accounts[device].balance` — 아니면 InsufficientBalance.
10. effects: nonce 비트 켜기, paid 기록, 잔액 이동, windowSpent 증가.
11. emit PaymentSettled.

nonce bitmap은 기기가 nonce를 아무 순서로 써도 되게 한다. 오프라인 기기가 여러 서명을 만들어도 순서에 묶이지 않는다.

## 6. 권한과 키

| 역할 | 키 | 할 수 있는 일 | 할 수 없는 일 |
|---|---|---|---|
| 운영자 | 역할 EOA | depositFor, closeAccount, requestWithdrawal, cancelWithdrawal | 잔액을 다른 주소로 보내기, 가맹점 등록 |
| registry admin | 역할 EOA | 가맹점 등록·철회, payout 변경 요청과 cancelPayoutChange | 즉시 payout 바꾸기, 잔액 이동 |
| 키오스크 | 가스용 EOA | 트랜잭션 제출 | 서명 없는 상태 변경 |

upgrade proxy와 pause는 없다(P06-NFR-01). 역할 키는 constructor에서 정하고 바꾸는 함수를 두지 않는다. 키 교체는 새 배포로 한다.

## 7. 위협과 대응

| 위협 | 대응 |
|---|---|
| 기기 서명 재사용 | (merchant, orderId) 유일성을 먼저 검사하고, nonce bitmap(결제·LimitChange 공유), 짧은 authorizationExpiry |
| 다른 체인·다른 배포에서 재사용 | 메시지와 도메인 모두에 chainId·contract |
| 운영자 키 유출 | 운영자는 잔액을 임의 주소로 보낼 수 없다. closeAccount·requestWithdrawal 남용은 withdrawAddress로만 지급되어 자산은 대여자에게 간다. 지연 동안 요청 전에 서명된 결제는 계속 정산되고, 오조작은 cancelWithdrawal로 되돌린다 |
| registry admin 키 유출 | 가짜 가맹점 등록과 payout 변경 요청이 가능하다. payout 변경은 payoutChangeDelay(≥ attestationValidity) 뒤에야 효력을 갖고 이벤트로 공개되므로 가맹점이 확인해 이의를 제기할 수 있다. 다만 cancelPayoutChange도 같은 admin 키로 하므로 키 탈취 자체는 막지 못한다. 역할 키는 constructor에서 고정되며 남는 위험으로 기록한다 |
| 재진입 | settle은 외부 호출 없음. cashOut·출금은 상태를 먼저 0으로 만든 뒤 전송 |
| 토큰 이상 동작 | 배포마다 알려진 dummy 토큰 하나. fee-on-transfer 토큰은 받지 않는다 |

## 8. 시험 계획

- **단위:** srs의 각 FR 시험 이름 그대로 `test/PaymentSettlement.t.sol`에 둔다.
- **벡터:** `test/Eip712Vectors.t.sol`이 eip712-vectors.json의 PaymentAuthorization·LimitChange 벡터를 모두 돌린다 [N21].
- **fuzz:** 금액·nonce·만료·창 경계(`testFuzz_dailyWindow`), 임의 순서 nonce.
- **invariant:** 모든 기기 잔액 + 가맹점 잔액 합 <= 컨트랙트가 가진 토큰(대기 출금은 잔액 안에 있고, 직접 전송된 토큰이 있으면 부등호), 닫힌 계정은 settle 성공 0회, 같은 (merchant, orderId) PaymentSettled 최대 1회, 정산된 서명의 재제출은 항상 OrderAlreadyPaid.
- **배포 시험:** 8283에 배포 후 소프트웨어 키로 settle 1회, finalized 블록에서 이벤트 확인(W6 컨트랙트 게이트, [N03]).
- **가스:** `forge snapshot`과 W6 배포 실측으로 settle 가스를 기록해 register의 kioskMinGasBalance(결제 2건분 busy 하한) 재측정에 넘긴다.

## 9. depositFor와 closeAccount 순서 예

대여: 셋업(TimeAnchor 수락) → depositFor(device, amount, withdrawAddress) → 결제 반복. 반납: closeAccount(device) → AccountClosed finalized 확인 → 운영자 서명 DeviceReset으로 device.reset. 잔액 지급은 이와 별개로 withdrawalDelay 경과 뒤 executeWithdrawal로 한다. ORDER_ALREADY_PAID는 이미 정산된 주문이 다시 제출될 때 나며, 정산된 서명의 재제출이 대표적인 경우다 [N07].

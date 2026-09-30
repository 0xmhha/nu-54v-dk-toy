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
struct Account {            // key: device address; settle reads slots A-C and writes only B
    address withdrawAddress; // A: non-zero means the account exists
    uint64  closedAt;        //    0 = active
    uint128 balance;         // B
    uint88  windowSpent;     //    <= dailyCap, so the constructor requires dailyCap <= 2^88 - 1
    uint40  windowStart;     //    seconds; uint40 lasts about 35,000 years
    uint128 perPaymentLimit; // C: from LimitChange, <= perPaymentCap; 0 = use perPaymentCap
    uint128 dailyLimit;      //    from LimitChange, <= dailyCap; 0 = use dailyCap
    uint128 pendingWithdrawal; // D: requested amount; balance is not moved until execute
    uint64  withdrawAfter;   //    pending withdrawal release time
}
mapping(address => Account) accounts;
mapping(address => mapping(uint256 => uint256)) nonceBitmap; // device => word => bits
mapping(address => mapping(bytes32 => bool)) paid;            // merchant => orderId
mapping(address => uint256) merchantBalance;
uint256 totalOwed; // sum of every device balance and merchant balance (P06-FR-19)

// MerchantRegistry
struct Merchant { bool active; bool hasPending; address payout; address pendingPayout; uint64 payoutEffectiveAt; }
// hasPending shares the first slot with payout, so settle reads one slot when no change is queued.
mapping(address => Merchant) merchants;
```

## 3. 함수·이벤트·오류

| 함수 | 호출자 | 동작 |
|---|---|---|
| `depositFor(device, amount, withdrawAddress)` | 운영자 | 토큰을 받아 잔액 증가, 첫 호출에만 withdrawAddress 기록. 이후 호출은 같은 주소나 0 주소만 받고, 다른 주소면 WithdrawAddressMismatch. 닫힌 계정이면 revert [N07] |
| `settle(PaymentAuthorization auth, bytes sig)` | 누구나(키오스크) | 5절의 검사 후 잔액 이동, PaymentSettled |
| `setLimits(LimitChange change, bytes sig)` | 누구나(키오스크) | 기기 서명, expiry(authorizationExpiry 안), 결제와 공유하는 nonce bitmap을 확인한 뒤 한도 변경 |
| `requestWithdrawal(device, amount)` | 운영자 | pendingWithdrawal = amount, withdrawAfter = now + withdrawalDelay. 잔액은 옮기지 않는다 |
| `cancelWithdrawal(device)` | 운영자 | 지연 중 대기 출금을 지운다(오조작 되돌리기). 지연이 지나면 WithdrawalMatured, 닫힌 계정에는 쓸 수 없다 |
| `executeWithdrawal(device)` | 누구나 | 지연 경과 시 min(pendingWithdrawal, balance)를 잔액에서 빼 withdrawAddress로 전송 |
| `closeAccount(device)` | 운영자 | closedAt 기록, pendingWithdrawal = 전체 잔액, withdrawAfter = now + withdrawalDelay |
| `merchantStatus(merchant)` | 조회 | 활성 여부와 현재 유효 payout을 한 번에 돌려준다. settle은 이 한 번의 호출로 5·6단계를 검사한다 |
| `registerMerchant/revokeMerchant` | registry admin | 가맹점 등록·즉시 철회. 이미 payout이 있는 가맹점은 같은 payout으로 재활성화만 할 수 있고, 다른 payout이면 PayoutChangeRequired로 거절한다(즉시 payout 변경 차단). 등록된 적 없는 가맹점의 철회는 UnknownMerchant |
| `requestPayoutChange(merchant, payout)` | registry admin | pendingPayout 기록, payoutEffectiveAt = now + payoutChangeDelay |
| `cancelPayoutChange(merchant)` | registry admin 또는 해당 가맹점 | 효력 전 pendingPayout을 지운다. 가맹점도 취소할 수 있으므로 admin 키만 탈취해서는 payout을 옮길 수 없다 |
| `cashOut()` | 가맹점 | merchantBalance를 현재 payout으로 전송 |
| `recoverSurplus(to, amount)` | 운영자 | 컨트랙트가 가진 토큰 중 totalOwed(모든 기기 잔액과 가맹점 잔액의 합)를 넘는 잉여분만 회수한다. 거절되어 돌아온 지급이나 직접 전송된 토큰이 대상이다. SurplusRecovered |
| `cashOutFor(merchant)` | 누구나 | cashOut과 같되 가맹점 키 없이 호출한다. 목적지는 registry payout뿐이므로 가맹점 키(키오스크)를 잃어도 정산금이 묶이지 않는다 |
| `accountOf(device)` | 조회 | 기기 계정 전체(잔액, 종료 시각, withdrawAddress, 대기 출금, 한도, 일일 창) |

| 이벤트 | 용도 |
|---|---|
| `Deposited(device, amount)` | 예치 관측 |
| `PaymentSettled(address indexed merchant, bytes32 indexed orderId, address indexed device, uint256 amount, uint256 nonce)` | paid 판정의 유일한 근거. P04와 P07은 merchant·orderId topic으로 조회한다 [N08] |
| `CashedOut(merchant, payout, amount)` | 가맹점 인출 관측 |
| `LimitsChanged(device, perPayment, daily)` | 한도 이력 |
| `WithdrawalRequested/Executed(device, amount)` | 출금 관측 |
| `AccountClosed(device)` | 반납 관측 |
| `WithdrawalCancelled(device)` | 출금 취소 관측 |
| `MerchantRegistered/Revoked/PayoutChangeQueued/PayoutChangeCancelled(merchant, ...)` | registry 이력. payout 변경은 이 이벤트로 공개되어 가맹점이 확인할 수 있다 |

오류: `WrongDomain`, `OrderAlreadyPaid`(ORDER_ALREADY_PAID), `Expired`, `AccountInactive`, `MerchantRevoked`, `MerchantForged`, `NonceReplayed`, `OverCap`, `InsufficientBalance`, `NotOperator`, `ZeroAddress`, `TransferFailed`, 생성자·인자 범위 오류 `InvalidParameters`. 확장(`IPaymentSettlementExtensions`)은 `NoPendingWithdrawal`, `WithdrawalNotReady`, `WithdrawAddressMismatch`. registry는 `NotRegistryAdmin`, `ZeroAddress`, 확장(`IMerchantRegistryExtensions`)은 `InvalidParameters`, `UnknownMerchant`, `PayoutChangeRequired`, `NoPendingPayoutChange`. 키오스크의 매핑은 srs 3절에 있다.

W6에 고정하는 ABI는 `IPaymentSettlement`와 `IMerchantRegistry`이고, 확장 함수·이벤트·오류는 별도 interface에만 더한다. 그래서 확장을 구현해도 고정 ABI의 sha256은 바뀌지 않는다.

생성자는 다음을 거절한다. settlement: 0 주소, 코드가 없는 token·registry 주소(immutable이라 오타를 배포 때 잡는다), dailyCap이 uint128을 넘는 값, 3절 범위 밖 파라미터. registry: 0 주소 admin, payoutChangeDelay 0. 시험 토큰: 0 주소 owner. 배포 스크립트는 역할 주소 셋이 0이 아니고 서로 다른지, payoutChangeDelay ≥ attestationValidity인지 확인한 뒤에만 전송한다.

토큰 호출은 반환값이 `true`이거나 없을 때만 성공으로 본다(`false`나 다른 값은 TransferFailed). 지금은 시험 토큰뿐이지만 반환값이 없는 실토큰으로 바꿔도 동작하게 하기 위해서다.

## 4. EIP-712 해시

도메인은 `{name: "NU54 Payment Settlement", version: "1", chainId: 8283, verifyingContract: this}`다. typehash는 스키마의 `encodeType` 문자열을 그대로 keccak한다. 시험은 `vm.readFile`로 eip712-vectors.json을 읽어 PaymentAuthorization·LimitChange 벡터의 digest를 재계산하고, `ecrecover` 결과가 벡터의 signer와 같은지 확인한다. 벡터의 verifyingContract가 시험 배포 주소와 다르므로 시험용 해시 함수는 도메인 separator를 인자로 받는다.

서명은 low-s만 받는다. `ecrecover`가 0 주소나 계정 없는 주소를 돌려주면 AccountInactive로 거절한다. 컨트랙트 층의 MerchantForged는 payout 불일치 하나뿐이다.

## 5. settle 검사 순서

checks-effects-interactions를 지킨다. settle은 상태를 바꾸는 외부 호출을 하지 않는다. 배포 때 고정한 MerchantRegistry의 view 함수(`isActive`, `payoutOf`)만 읽는다.

순서는 [N07]을 따른다.

1. `auth.chainId == block.chainid`, `auth.contract == address(this)`, `auth.token == token`(배포 토큰) — 아니면 WrongDomain. 이어서 `auth.amount == 0`이면 InvalidParameters. 금액 0 결제로 남의 (merchant, orderId)를 공짜로 선점해 진짜 주문을 막는 것을 막는다.
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
| 운영자 키 유출 | 운영자는 장부에 있는 잔액(`totalOwed`)을 임의 주소로 보낼 수 없다. 임의 주소로 보낼 수 있는 것은 장부 밖 잉여분(`surplus`)뿐이다. closeAccount·requestWithdrawal 남용은 withdrawAddress로만 지급되어 자산은 대여자에게 간다. 지연 동안 요청 전에 서명된 결제는 계속 정산되고, 오조작은 cancelWithdrawal로 되돌린다 |
| registry admin 키 유출 | 가짜 가맹점 등록, 철회(서비스 중단), payout 변경 요청이 가능하다. payout 변경은 payoutChangeDelay(≥ attestationValidity) 뒤에야 효력을 갖고 이벤트로 공개되며, 가맹점이 자기 키로 cancelPayoutChange를 호출해 거부할 수 있다. 따라서 admin 키만으로는 정산금을 옮길 수 없다. 남는 위험은 가맹점 키 분실과 admin 키 탈취가 겹치는 경우, 철회·가짜 등록에 의한 서비스 중단, 그리고 가맹점 키가 탈취되면 정당한 payout 변경도 계속 거부될 수 있다는 점(복구는 새 배포)이다. 역할 키는 constructor에서 고정된다 |
| 재진입 | settle은 외부 호출 없음. cashOut·출금은 상태를 먼저 0으로 만든 뒤 전송 |
| 가맹점 키 분실 | cashOutFor로 누구나 registry payout으로 보낼 수 있다 |
| 운영자 입금 실수(잘못된 기기 주소) | closeAccount 후 withdrawalDelay 뒤 withdrawAddress로 돌려받는다 |
| 기기와 PIN 동시 도난 | closeAccount가 즉시 settle을 막는다. 그 전까지 최악 손실은 min(잔액, 2 x dailyCap) |
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

## 10. 시험 토큰의 받지않을권리

ERC-20은 받는 쪽의 동의 없이 잔액이 늘어난다. 시험 토큰(`TestUSDC`)은 계정마다 받지않을권리 모드를 둔다. 기본은 꺼져 있고, 계정 주인이 `setRefusalMode(true/false)` 트랜잭션으로 켜고 끈다.

모드가 켜진 계정으로 모르는 송신자가 보내면 두 단계로 처리한다.

1. 토큰이 송신자에게서 빠져 토큰 컨트랙트 주소로 옮겨지고, 보류 이체(id, from, to, amount, heldAt)로 기록된다. `TransferHeld(id, from, to, amount)` 이벤트를 알림 서비스가 보고 수신자에게 web2 알림을 보낸다. escrow와 달리 송신자는 취소할 수 없다.
2. 수신자가 송신자를 확인해 `acceptTransfer(id)`로 받거나, `rejectTransfer(id)`로 거절한다. 거절하면 토큰은 즉시 송신자에게 돌아가고 `TransferRejected`가 나간다. 수신자가 무시하면 송신자는 운영팀(토큰 owner)에 오송금을 알리고, 운영팀은 확인 후 `returnToSender(id)`로 원래 송신자에게만 돌려준다. 이 반환은 보류 뒤 `RETURN_DELAY`(3일)가 지나야 가능하다. 수신자가 판단할 시간을 먼저 보장하기 위해서다.

송신자가 명확한 자금은 신뢰 송신자 목록으로 보류를 건너뛴다. 운영팀이 `setTrustedSender`로 등록한 송신자는 모든 계정에 기본 적용되고, 계정마다 `setTrustOptOut(sender, true)`로 개별 제외할 수 있다. 정산 컨트랙트는 배포 뒤, 첫 입금 전에 `script/TrustSettlement.s.sol`로 등록한다(`SoftwareSettle.s.sol`은 등록되지 않았으면 멈춘다). 정산 컨트랙트는 계약상 정해진 주소로만 지급하기 때문이다. 가맹점 payout은 가맹점 계약에 따라 registry admin이 등록하고, 대여자 withdrawAddress는 대여 계약에 따라 운영자가 첫 입금 때 고정한다. 정산 컨트랙트는 토큰 컨트랙트나 자기 자신을 지급 주소로 받지 않는다(withdrawAddress는 입금 때, payout은 cashOut 때 InvalidParameters).

세부 규칙:

- 보류 기록은 두 storage slot(from과 heldAt, to와 amount)에 담으므로, 보류되는 한 건의 금액은 uint96(6자리 소수 기준 약 7.9×10^22달러) 이하여야 한다. 넘으면 AmountTooLarge.

- 발행(mint)은 "반드시 받을 수 있음" 조건(`mustReceive`)을 먼저 확인한다. 받을 계정의 모드가 켜져 있으면 `RecipientRefusing`으로 바로 실패하고, 그 계정은 모드를 꺼야 발행을 받을 수 있다. 이 조건은 transfer 경로와 분리된 early return이므로 transfer는 언제나 같은 규칙(보류 또는 즉시 수신)으로 동작한다.
- 자기 자신에게 보내는 이체와 거절·반환으로 송신자에게 돌아가는 이체는 보류하지 않는다(되돌림이 다시 보류되는 순환을 막는다).
- `transferFrom`의 송신자는 spender가 아니라 토큰 주인(`from`)으로 기록한다. 거절하면 주인에게 돌아간다.
- 토큰 컨트랙트 주소와 0 주소로의 직접 이체는 거절한다(보류 잔액과 섞이지 않게 한다).
- 모드를 꺼도 이미 보류된 이체는 그대로 남아 수령·거절할 수 있다.
- 금액 0 이체는 보류하지 않는다(받을 것이 없고, 알림 스팸이 된다). 정산 컨트랙트도 잔액 0인 cashOut과 0원 출금에서는 토큰을 보내지 않는다.
- 토큰 컨트랙트 자신의 잔액은 항상 보류 이체 금액의 합과 같다.

모드가 켜진 수신자에게 보낸 이체의 체인상 수신자는 토큰 컨트랙트다. `Transfer(from, 토큰 컨트랙트, amount)`와 `TransferHeld`가 함께 나가고, 수령하면 `Transfer(토큰 컨트랙트, 수신자, amount)`가 나간다. 잔액과 이벤트가 어긋나지 않는다.

정산 지급이 거절되어 정산 컨트랙트로 돌아오거나(신뢰 목록에서 정산 컨트랙트를 제외한 수신자가 거절한 경우, 운영팀이 반환한 경우) 누군가 정산 컨트랙트로 직접 보낸 토큰은 장부 밖 잉여분이 된다. 운영자가 `recoverSurplus`로 회수해 원래 주인에게 운영 절차로 돌려준다(P06-FR-19).

소액 반복 이체로 알림을 채우는 스팸은 막지 않는다. 보류는 송신자의 가스로 기록되고 수신자 잔액에는 영향이 없다.

운영 주체와 내부자: 이 토큰과 정산 컨트랙트는 서비스 제공자가 운영한다. 검토는 운영 주체가 선의로 운영한다는 전제에서 한다. 내부자의 악의적 행위는 컨트랙트 안의 개별 제한을 늘려 막지 않는다. 운영 역할 키 하나로는 할 수 없도록 여러 사람의 승인이 필요한 통제(다중 서명 등)로 다룬다. 운영팀의 신뢰 목록 변경(`TrustedSenderSet`)과 운영자의 잉여분 회수(`SurplusRecovered`)는 모두 이벤트로 공개된다.


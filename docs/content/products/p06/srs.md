# P06 SRS — 정산 컨트랙트 요구사항

정산 컨트랙트와 가맹점 registry가 만족해야 하는 요구다. 각 요구는 Foundry 시험 이름으로 판정하고, 관련 12주 수용 항목([week12-log](../../acceptance/week12-log.md))에 연결한다. 값은 [register](../../planning/design-freeze-checkpoint-02.md) parameters가 정하며 여기서는 이름만 쓴다 [N13]. 메시지 형식은 [결제 프로토콜](../../specifications/protocol/payment-protocol.md)을 따른다.

## 1. 용어

| 용어 | 뜻 |
|---|---|
| 기기 계정 | 기기 키 주소로 식별되는 예치 계정. depositFor가 만든다 |
| 가맹점 | registry에 등록된 가맹점 서명 주소와 payout 주소 쌍 |
| 운영자 | depositFor, closeAccount, requestWithdrawal, cancelWithdrawal을 호출하는 역할 EOA |
| registry admin | registry 등록·철회, payout 변경 요청과 cancelPayoutChange를 하는 역할 EOA |

## 2. 기능 요구

| ID | 요구 | 판정(Foundry 시험) | W12 |
|---|---|---|---|
| P06-FR-01 | 운영자는 `depositFor(deviceAddress, amount, withdrawAddress)`로 기기 계정을 만들거나 잔액을 늘린다. withdrawAddress는 처음 한 번만 기록되고 이후 호출로 바뀌지 않는다. 이후 호출에 다른 주소를 주면 `WithdrawAddressMismatch`로 revert한다 [N07] | `test_depositFor_createsAccount`, `test_depositFor_withdrawAddressImmutable` | W12-03 |
| P06-FR-02 | `settle(auth, signature)`는 PaymentAuthorization을 EIP-712로 해시하고 서명자가 활성 기기 계정일 때만 진행한다. 서명자가 계정이 없거나 닫혔거나 0 주소면 `AccountInactive`로 revert한다. 기기는 이 타입과 LimitChange만 서명한다 [N04] | `test_settle_recoversDeviceSigner`, `test_settle_rejectsUnknownSigner` | W12-03 |
| P06-FR-03 | 해시 결과는 P10 벡터의 digest와 같아야 한다 [N21] | `test_eip712_matchesVectors` | W12-03 |
| P06-FR-04 | `auth.chainId`, `auth.contract`, `auth.token`이 이 배포(배포 토큰 포함)와 다르면 `WrongDomain`으로 revert한다 | `test_settle_rejectsWrongDomain`, `test_settle_rejectsWrongToken` | - |
| P06-FR-05 | `auth.merchant`가 registry에서 활성이 아니면 `MERCHANT_REVOKED`로 revert한다 | `test_settle_revokedMerchant` | W12-05 |
| P06-FR-06 | 서명된 `auth.payout`이 registry의 현재 유효 payout과 다르면 `MERCHANT_FORGED`로 revert한다. 컨트랙트 층의 MERCHANT_FORGED는 이 경우 하나뿐이다 | `test_settle_payoutMismatch` | W12-05 |
| P06-FR-07 | 금액이 건당 한도 또는 일일 누적 한도를 넘으면 `OVER_CAP`으로 revert한다. 한도는 register의 perPaymentCap·dailyCap과 기기 계정의 LimitChange 값 중 작은 쪽이며, LimitChange 값이 0이면 register cap을 그대로 적용한다. 일일 한도는 첫 결제부터 시작하는 고정 24시간 창이므로 임의의 24시간 동안 최악 손실은 min(예치금, 2 x dailyCap)이다 [N13] | `test_settle_overPerPayment`, `test_settle_overDaily`, `test_limits_zeroMeansCap` | W12-05 |
| P06-FR-08 | nonce는 256비트 unordered bitmap으로 관리하며 settle과 setLimits가 같은 bitmap을 쓴다. 이미 쓴 nonce면 `NONCE_REPLAYED`로 revert한다. 결제 서명 재제출은 주문 유일성 검사에서 먼저 걸리므로, 이 코드는 이미 적용된 LimitChange를 다시 제출할 때 난다 | `test_settle_nonceReplay`, `test_limitChange_replay`, `testFuzz_nonce_anyOrder` | W12-05 |
| P06-FR-09 | `block.timestamp > auth.expiry`이거나 `auth.expiry - block.timestamp`가 authorizationExpiry보다 크면 `Expired`로 revert한다 [N13] | `test_settle_expired`, `test_settle_expiryTooFar` | - |
| P06-FR-10 | 이미 결제된 (merchant, orderId)로 정산이 오면 도메인 검사 바로 다음, 만료·nonce 검사보다 먼저 `ORDER_ALREADY_PAID`로 revert한다. 그래서 첫 제출이 정산된 서명을 다시 제출하면 만료 뒤라도 이 코드가 난다(첫 제출이 다른 이유로 revert했다면 재제출도 그 오류, 만료 뒤면 Expired가 난다). 이때 키오스크는 finalized PaymentSettled의 device·amount·nonce가 자기 서명과 같을 때만 성공으로 본다 [N07][N08] | `test_settle_orderAlreadyPaid`, `test_settle_resubmitAfterExpiryIsAlreadyPaid` | W12-03 |
| P06-FR-11 | 기기 계정 잔액이 금액보다 적으면 `InsufficientBalance`로 revert한다. 통과하면 기기 계정 잔액에서 가맹점 잔액으로 옮기고 `PaymentSettled(address indexed merchant, bytes32 indexed orderId, address indexed device, uint256 amount, uint256 nonce)`를 내보낸다. 금액 0은 도메인 검사 직후 `InvalidParameters`로 거절한다(주문 선점 방지). 토큰 전송은 일어나지 않는다 [N08] | `test_settle_insufficientBalance`, `test_settle_emitsPaymentSettled`, `test_settle_zeroAmountRejected`, `invariant_balancesNotAboveTokenHeld` | W12-03 |
| P06-FR-12 | 가맹점은 `cashOut()`으로 잔액을 registry payout으로만 받는다. 가맹점 키를 잃은 경우를 위해 누구나 `cashOutFor(merchant)`로 같은 payout에 보낼 수 있다. payout 변경은 payoutChangeDelay가 지난 뒤 효력을 갖고, cashOut은 그 시점에 유효한 payout으로 보낸다 [N13] | `test_cashOut_toPayoutOnly`, `test_cashOutFor_permissionless`, `test_cashOut_followsDelayedPayoutChange`, `test_payoutChange_delayed` | W12-03 |
| P06-FR-13 | `setLimits(change, signature)`는 LimitChange 서명을 검증하고, expiry가 authorizationExpiry 안이며 nonce가 결제와 같은 bitmap에서 처음 쓰일 때만 기기 계정의 한도를 register 상한 이하로 바꾼다 [N04] | `test_limitChange_withinCap`, `test_limitChange_aboveCapRejected`, `test_limitChange_expired` | W12-05 |
| P06-FR-14 | 기기 계정 출금은 운영자만 `requestWithdrawal`로 요청한다. 요청은 대기 금액만 기록하고 잔액을 옮기지 않으므로, 지연 동안 요청 전에 서명된 결제는 전체 잔액으로 계속 정산된다. withdrawalDelay가 지나면 `executeWithdrawal`이 min(대기 금액, 그 시점 잔액)을 withdrawAddress로만 지급한다. 운영자는 지연 동안에만 `cancelWithdrawal`로 오조작을 되돌린다. withdrawAddress로 토큰 컨트랙트나 정산 컨트랙트 자신은 받지 않는다 [N13] | `test_withdrawal_operatorOnly`, `test_withdrawal_delayed`, `test_withdrawal_fixedDestination`, `test_withdrawal_settleDuringDelay`, `test_withdrawal_cancel`, `test_withdrawal_cancelOnlyDuringDelay`, `test_depositFor_unpayableWithdrawAddressRejected` | - |
| P06-FR-15 | `closeAccount(device)`는 호출 즉시 기기 계정을 비활성으로 만들어 이후 settle을 막고, 전체 잔액을 대기 출금으로 기록해 withdrawalDelay 뒤 withdrawAddress로 지급된다. 닫힌 계정으로의 depositFor는 revert한다 [N07][N11] | `test_closeAccount_blocksSettle`, `test_closeAccount_paysAfterDelay`, `test_depositFor_closedReverts` | - |
| P06-FR-16 | 출금 실행과 closeAccount 이후 지급은 운영자 또는 누구나 호출할 수 있지만 목적지는 항상 등록 주소다 | `test_execute_permissionlessFixedDestination` | - |
| P06-FR-17 | registry admin은 가맹점을 등록·철회하고 payout 변경을 요청하며, 효력 전에는 `cancelPayoutChange`로 되돌린다. 철회는 즉시 효력을 갖고, payout 변경은 payoutChangeDelay(≥ attestationValidity) 뒤 효력을 갖는다. 이 지연은 변경이 이벤트로 공개된 뒤 가맹점이 확인해 이의를 제기하고 admin이 되돌릴 창이다. 가맹점도 자기 payout 변경을 효력 전에 취소할 수 있어, admin 키만 탈취해서는 payout을 옮길 수 없다. `registerMerchant`로 이미 등록된 가맹점의 payout을 바꿀 수는 없다(`PayoutChangeRequired`). P05는 변경 요청 뒤 옛 payout attestation의 validUntil을 효력 시각으로 잘라 발급한다 [N05] | `test_registry_revokeImmediate`, `test_payoutChange_delayed`, `test_payoutChange_cancel`, `test_register_cannotChangePayoutImmediately`, `test_payoutChange_merchantVeto` | W12-05 |
| P06-FR-18 | 시험 토큰은 계정별 받지않을권리 모드를 둔다(기본 꺼짐, 계정 주인이 켜고 끈다). 모드가 켜진 계정으로 신뢰 목록 밖 송신자가 보내면 토큰은 토큰 컨트랙트에 보류되고 `TransferHeld`가 나간다. 수신자만 수령 또는 거절하고, 거절은 즉시 송신자에게 돌려준다. 송신자는 취소할 수 없다. 무시된 보류는 운영팀이 RETURN_DELAY(3일) 뒤 원래 송신자에게만 돌려준다. 금액 0 이체는 보류하지 않는다. 발행은 모드가 꺼진 계정에만 하며, 켜져 있으면 `RecipientRefusing`으로 실패한다. 운영팀의 신뢰 송신자 목록은 모든 계정에 기본 적용되고 계정별로 제외할 수 있다([design.md](design.md) 10절) | `test_refusal_holdsTransfer`, `test_refusal_accept`, `test_refusal_rejectReturnsAtOnce`, `test_refusal_onlyRecipientDecides`, `test_refusal_teamReturnAfterDelayOnly`, `test_trustedSender_skipsHoldByDefault`, `test_trustedSender_userOptOut`, `test_cashOut_payoutInRefusalMode`, `test_refusal_zeroTransferNotHeld`, `test_mint_requiresReceivingAccount` | - |
| P06-FR-19 | 정산 컨트랙트는 모든 기기 잔액과 가맹점 잔액의 합을 `totalOwed`로 유지한다. 운영자는 보유 토큰 중 totalOwed를 넘는 잉여분(거절되어 돌아온 지급, 직접 전송된 토큰)만 `recoverSurplus`로 회수하고 `SurplusRecovered`를 남긴다. 장부에 있는 잔액은 회수할 수 없다 | `test_recoverSurplus_rejectedPayout`, `test_recoverSurplus_operatorOnlyAndDirectTransfers`, `test_totalOwed_tracksEveryMove`, `invariant_totalOwedIsTheSumOfBalances` | - |

refund 경로는 없다 [N17]. 잘못 결제된 금액은 이번 사이클에서 운영 절차 밖이다.

## 3. revert 사유와 거절 코드

키오스크는 eth_call 결과의 custom error 이름을 거절 코드로 바꾼다. 이름을 바꾸면 P04와 week12-log가 함께 바뀌어야 하므로 W6에 settle, PaymentSettled, error 이름을 고정한 뒤 바꾸지 않는다. W7–W9에 더하는 함수는 이 고정분을 바꾸지 않는 추가만 하고, W12 배포는 P06-05 뒤의 재배포다 [N22]. 검사 순서는 WrongDomain → OrderAlreadyPaid → Expired → AccountInactive → MerchantRevoked → MerchantForged → NonceReplayed → OverCap → InsufficientBalance다([design.md](design.md) 5절).

| custom error | 거절 코드 | W12-05 |
|---|---|---|
| `MerchantRevoked()` | MERCHANT_REVOKED | 시연 |
| `OverCap()` | OVER_CAP | 시연 |
| `NonceReplayed()` | NONCE_REPLAYED | 시연(적용한 LimitChange를 expiry 안에 재제출) |
| `MerchantForged()` | MERCHANT_FORGED | 시연 |
| `OrderAlreadyPaid()` | ORDER_ALREADY_PAID. 키오스크가 finalized PaymentSettled의 device·amount·nonce를 자기 서명과 비교해 같으면 성공, 다르면 refused | - |
| `Expired()` | EXPIRED | 시험만 |
| `WrongDomain()` | WRONG_DOMAIN | 시험만 |
| `AccountInactive()` | ACCOUNT_INACTIVE | 시험만 |
| `InsufficientBalance()` | INSUFFICIENT_BALANCE | 시험만 |

ATTESTATION_EXPIRED는 기기가 거절하므로 컨트랙트에는 해당 error가 없다.

## 4. 비기능 요구

| ID | 요구 | 판정 |
|---|---|---|
| P06-NFR-01 | 업그레이드 proxy와 pause를 두지 않는다. 결함이 나면 새 배포와 closeAccount 후 재예치로 옮긴다. 관리자 권한이 적을수록 운영자 키 유출 때 피해가 줄기 때문이다. 운영자가 옮길 수 있는 것은 장부 밖 잉여분뿐이다(P06-FR-19) | 소스 검토, `test_noAdminDrain`, `test_recoverSurplus_rejectedPayout` |
| P06-NFR-02 | settle 한 번의 가스는 register가 kioskMinGasBalance 추정에 쓴 settleGasEstimate(170,000, 2026-09-29 sandbox 실측 165,399에 여유를 더한 값) 이하로 둔다. kioskMinGasBalance는 결제 2건분(정산 1건과 같은 서명 재전송 1건)의 busy 하한이므로, W6 실측이 이 가스를 넘으면 kioskMinGasBalance를 다시 정한다 [N10] | `forge snapshot` |
| P06-NFR-03 | paid 판정은 finalized 블록의 PaymentSettled 이벤트로 한다. 모든 상태 변경은 이벤트를 남긴다 [N08] | 이벤트 목록 검토 |
| P06-NFR-04 | 상태를 바꾸는 외부 호출은 cashOut·출금의 토큰 전송뿐이며 상태 변경 뒤에 한다. settle은 배포 때 고정한 registry의 view 함수만 읽는다 | `test_reentrancy_cashOut` |
| P06-NFR-05 | 개인정보를 저장하지 않는다. 가맹점 이름은 attestation에만 있고 체인에는 없다 [N14] | 저장소 레이아웃 검토 |

## 5. 추적

| 결정 | 요구 |
|---|---|
| [N04] | P06-FR-02, P06-FR-13 |
| [N07] | P06-FR-01, P06-FR-10, P06-FR-11, P06-FR-15, 3절 검사 순서 |
| [N08] | P06-FR-10, P06-FR-11, P06-NFR-03 |
| [N13] | P06-FR-07, P06-FR-09, P06-FR-12, P06-FR-14 |
| [N05] | P06-FR-17 |
| [N17] | refund 없음 |
| [N22] | 3절 |

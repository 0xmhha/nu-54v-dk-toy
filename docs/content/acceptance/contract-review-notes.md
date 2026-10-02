# 정산 컨트랙트 강화 review notes (10/2)

컨트랙트 강화 작업(WBS2-P06-05)의 완료 조건 가운데 review notes를 채운다. 완료 조건은 review notes, 3600초 finality 관측, 12주차 최종 배포 셋이다. finality 관측은 [finality 관측 기록](finality-8283.md)에서 끝났다. 이 문서는 invariant 시험, 정적 분석, [설계 7절 위협 표](../products/p06/design.md) 대조, 가스 재측정을 정리하고 최종 배포를 판단한다.

대상은 main `0a9c4d1`의 `products/p06-stablenet-contracts/src`다. 이 소스는 테스트넷에 배포한 소스(배포 기록의 main 대응 커밋 `a8c0c44`)와 한 줄도 다르지 않다(`git diff a8c0c44 HEAD -- src`가 비어 있다).

## 1. 결론

소스를 고쳐야 할 결함은 찾지 못했다. 따라서 지금 배포가 12주차 배포이며, 다시 배포하지 않는다. 9주차 재배포 여부는 "컨트랙트가 바뀔 때 정한다"(10/2 결정)였는데 바뀐 것이 없으므로 재배포는 없다. 이 판단으로 12주 WKRC 필요량은 배포 1세트 기준으로 계산할 수 있다.

## 2. invariant 시험

handler(`test/Invariant.t.sol`)가 입금, 결제, 재제출, 출금 요청·취소·실행, 계정 닫기, 직접 전송과 잉여분 회수, 시간 이동을 임의 순서로 부른다. 설계 8절의 invariant 넷을 모두 둔다.

| invariant | 뜻 |
|---|---|
| `invariant_holdsWhatItOwes` | 기기 잔액과 가맹점 잔액의 합이 컨트랙트가 가진 토큰을 넘지 않는다 |
| `invariant_totalOwedIsTheSumOfBalances` | `totalOwed`가 잔액 합과 같다 |
| `invariant_closedAccountNeverSettles` | 닫힌 계정은 settle에 한 번도 성공하지 않는다 |
| `invariant_orderSettlesAtMostOnce` | 한 (merchant, orderId)는 한 번만 정산된다 |

handler의 재제출 동작은 정산된 서명을 다시 보내고, 그 결과가 항상 `OrderAlreadyPaid`인지 단언한다(설계 8절의 다섯째 조건).

실행 결과는 다음과 같다. 기본 설정(64 runs × depth 200, 12,800 calls)으로 넷 모두 통과했다. 256 runs(51,200 calls)에서도 통과했고, handler 동작 10개가 각각 약 5,000번씩 고르게 불렸다.

시험이 아무것도 걸러내지 못한 채 통과하는 것은 아닌지 결함을 일부러 넣어 확인했다.
- settle에서 주문 유일성 검사를 지우면 `invariant_orderSettlesAtMostOnce`가 실패하고("2 > 1"), 재제출 단언도 실패한다.
- 서명자 계정 확인에서 닫힌 계정 조건을 지우면 `invariant_closedAccountNeverSettles`가 실패한다("1 != 0").

두 경우 모두 확인한 뒤 소스를 되돌렸다. 한 run에서 실제로 정산되는 주문은 몇 건 수준이다(표본 run에서 3건). orderId를 16개로 묶어 중복을 일부러 만들기 때문이며, 위 두 결함을 잡기에는 충분했다.

전체 시험은 96개(단위, fuzz, invariant, 벡터, 재진입)가 모두 통과했다.

## 3. 정적 분석 (slither 0.11.5)

`slither . --filter-paths "lib/|test/|script/"` 결과는 12건이다. 소스를 고쳐야 할 것은 없다.

| 검사 | 건수 | 위치 | 판단 |
|---|---|---|---|
| incorrect-equality (Medium) | 1 | `TestUSDC._takeHeld`: `h.heldAt == 0` | 오탐. `heldAt`은 보류를 만들 때의 블록 시각이라 0이 될 수 없고, 0은 "보류 없음" 표시다. 시험용 토큰이며 정산 컨트랙트가 아니다 |
| timestamp (Low) | 10 | 서명 만료, 일일 한도 창, 출금 지연, payout 변경 지연, 반송 지연 | 의도한 시각 비교다. StableNet은 허가된 검증자가 1초 블록을 만들므로 블록 시각의 흔들림은 수 초 수준이다. 가장 짧은 창(authorizationExpiry 120초)도 그보다 훨씬 길다 |
| low-level-calls (Info) | 1 | `PaymentSettlement._call` | 의도한 패턴이다. 반환값이 없거나 `true`인 토큰만 받는다. 코드 없는 주소를 호출하면 성공으로 읽히는 문제는 constructor가 토큰 주소의 코드 길이를 확인해(`InvalidParameters`) 막는다. 토큰 주소는 바꿀 수 없다 |

## 4. 위협 표 대조

[설계 7절](../products/p06/design.md)의 위협마다 대응이 코드에 있는지, 그리고 어떤 시험이 그것을 확인하는지 적었다.

| 위협 | 대응을 확인하는 시험 | 판단 |
|---|---|---|
| 기기 서명 재사용 | `test_settle_orderAlreadyPaid`, `test_settle_resubmitAfterExpiryIsAlreadyPaid`, `test_settle_nonceReplay`, `test_limitChange_replay`, `testFuzz_nonce_anyOrder`, invariant의 재제출 단언 | 확인 |
| 다른 체인·배포에서 재사용 | `test_settle_rejectsWrongDomain`, `test_settle_rejectsWrongToken`, `test_limitChange_wrongDomainAndSigner`, EIP-712 벡터 | 확인 |
| 운영자 키 유출 | `test_noAdminDrain`, `test_recoverSurplus_*`(장부 안의 잔액은 회수할 수 없음), `test_withdrawal_fixedDestination`, `test_execute_permissionlessFixedDestination`, `test_withdrawal_settleDuringDelay`, `test_withdrawal_cancel` | 확인 |
| registry admin 키 유출 | `test_payoutChange_delayed`, `test_payoutChange_merchantVeto`, `test_register_cannotChangePayoutImmediately`, `test_cashOut_followsDelayedPayoutChange`, `test_registry_revokeImmediate` | 확인. 설계가 적은 남는 위험(서비스 중단, 가맹점 키와 admin 키가 함께 털린 경우)은 그대로다 |
| 재진입 | `test_reentrancy_cashOut`. settle에는 외부 호출이 없다(소스 확인) | 확인 |
| 가맹점 키 분실 | `test_cashOutFor_permissionless` | 확인 |
| 운영자 입금 실수 | `test_closeAccount_paysAfterDelay`, `test_depositFor_withdrawAddressImmutable` | 확인 |
| 기기와 PIN 동시 도난 | `test_closeAccount_blocksSettle`, `test_settle_overDaily`, `testFuzz_dailyWindow`, invariant `closedAccountNeverSettles` | 확인. 최악 손실 min(잔액, 2 × dailyCap)은 창 경계 fuzz로 확인했다 |
| 토큰 이상 동작 | `test_token_noReturnValueAccepted`, `test_token_falseReturnRejected`, `test_constructor_rejectsAddressesWithoutCode`, `test_cashOut_payoutInRefusalMode`, `test_cashOut_unpayablePayoutKeepsBalance` | 확인. fee-on-transfer 토큰은 코드로 막지 않는다. 대신 배포마다 알려진 시험 토큰 하나로 고정한다(설계대로) |

## 5. 가스 재측정

[6주차 게이트 기록](gate-w6.md)의 테스트넷 실측과 10/2 register 개정을 그대로 쓴다.
- 첫 결제 settle: 120,888 gas.
- 이후 결제: 103,473 gas.
- 시험 추정: 121,206 gas. settleGasEstimate 130,000 안이며, `test_settle_txGasRecorded`가 이 상한을 지킨다.

배포 1세트의 가스는 3,484,141(약 166 WKRC)이다. 다시 배포하지 않으므로 12주 동안 더 들지 않는다.

## 6. 남는 위험

- [권장] invariant handler는 기기 셋, 가맹점 하나, orderId 16개로 상태 공간을 좁힌다. 가맹점이 여럿이거나 payout 변경과 결제가 섞이는 순서는 단위 시험으로만 확인했다.
- [권장] slither만 돌렸고 aderyn은 설치되어 있지 않아 돌리지 않았다. 외부 감사는 이번 사이클 범위 밖이다.
- 테스트넷 배포 소스와 main이 같다는 판단은 git diff에 근거한다. 배포된 바이트코드를 다시 빌드해 비교하는 검증은 [6주차 게이트 기록](gate-w6.md)에서 배포 때 한 번 했다.

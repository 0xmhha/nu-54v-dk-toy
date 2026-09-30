# 6주차 컨트랙트 게이트 기록 (10/1)

[DF-20260925-02](../planning/design-freeze-checkpoint-02.md)의 6주차 컨트랙트 게이트 결과다 [N03][N08]. 조건은 소프트웨어 서명으로 StableNet 테스트넷(8283)에 낸 결제가 finalized `PaymentSettled`로 남는 것이다(WBS2-P06-02). 판정 항목은 [구현 계획](../planning/implementation-plan-12week.md) 5장 WBS2-P06-02의 완료 판정을 따른다.

전체 주소와 해시는 `products/p06-stablenet-contracts/deployments/8283.json`에 있다. 이 문서는 줄인 값만 쓴다. 원본 로그는 저장소에 올리지 않는 `evidence/w6/`에 있고, 아래에 sha256만 남긴다.

## 배포

| 항목 | 값 |
|---|---|
| 소스 커밋 | `66a5f433d11b` (solc 0.8.30, optimizer 200, evm prague). 스쿼시 머지 뒤 main `a8c0c440256e`의 `src`, `foundry.toml`, `lib`가 같은 git tree다 |
| TestUSDC | `0x500ef6…A2BA`, 블록 21129130, 990,001 gas |
| MerchantRegistry | `0x0c293f…2ae0`, 블록 21129131, 539,362 gas |
| PaymentSettlement | `0xDe7596…caA4`, 블록 21129134, 1,953,730 gas |
| 배포 합계 | 3,483,093 gas, 약 165.8 WKRC (47,600 gwei) |
| bytecode 대조 | 세 계약의 체인 runtime code가 소스 커밋 빌드와 같다(immutable 위치 제외) |
| 설정 | 정산 컨트랙트를 시험 토큰 신뢰 송신자로 등록(블록 21129151, 45,816 gas) |

## 판정 항목

| 조건 | 판정 | 근거 |
|---|---|---|
| chain id가 8283이다 | 충족 | `cast chain-id` 결과 8283 |
| settle receipt status가 1이다 | 충족 | settle 트랜잭션 블록 21129166, status 1, 120,888 gas |
| finalized 블록이 결제 블록 이상이고 hash가 같다 | 충족 | 확인 시 finalized 21129178 ≥ 21129166. 블록 21129166의 hash가 receipt의 blockHash와 같다(`0x80bf5d…c45e`) |
| PaymentSettled의 merchant, orderId, device가 서명 값과 같다 | 충족 | merchant = 키오스크 역할 주소(`0xF92a32…61A4`), orderId `0x209f73…40e8`, device = 소프트웨어 기기 키 주소(`0xa8Edf3…C3dF`), amount 4,500,000(4.5 tUSDC) |
| 같은 서명 재제출은 `OrderAlreadyPaid`다 | 충족 | 같은 calldata의 eth_call이 selector `0x7f61b868`(OrderAlreadyPaid)로 revert |

정산 뒤 상태: 기기 잔액 45,500,000, 가맹점 잔액 4,500,000, totalOwed 50,000,000, surplus 0.

**6주차 컨트랙트 게이트 판정: 통과(10/1, 기한 10/14).**

## 증거

| 파일 | sha256 |
|---|---|
| `evidence/w6/deploy.log` | `sha256:faec5208e6b6b26ea03c2f18ae4187e966e348d428d1d3fbd5ebf37799e55cbb` |
| `evidence/w6/trust.log` | `sha256:5cc29031b1cbbb2648d16e38025f80b4fa56d8a8be1a7518cee2971dd7bd4aa2` |
| `evidence/w6/settle.log` | `sha256:370df44ecfcb95a2ac7d13495e5f065290cc8d802e0dcd0273bb3dbc22fb16d1` |

## 배포 중 확인한 체인 사실

StableNet 테스트넷은 우선순위 수수료(tip)가 27,600 gwei보다 낮거나 fee cap이 base fee(20,000 gwei)와 tip의 합보다 낮은 트랜잭션을 거절한다. forge 기본값(tip 1 wei)으로는 배포가 실패했다. `script/testnet-forge.sh`가 노드의 `eth_maxPriorityFeePerGas`를 tip으로, 2 × base fee + tip을 fee cap으로 넘기도록 고쳤다. 실패한 시도는 전송 전에 거절되어 nonce와 잔액이 바뀌지 않았다.

## 배포 뒤 재검토 결과

[구현 계획](../planning/implementation-plan-12week.md) 5장 WBS2-P06-02의 "배포 뒤 재검토" 표를 테스트넷 실측으로 채운다. 값을 바꾸는 결정은 사용자가 하며, 결정 전까지 register 값은 그대로 둔다.

| 재검토 항목 | 지금 값 | 테스트넷 실측 | 제안 | 상태 |
|---|---|---|---|---|
| settleGasEstimate | 170,000 gas | 기기 첫 결제 settle 120,888 gas | 130,000 gas(실측에 약 7% 여유) | 결정 필요 |
| kioskMinGasBalance | 20 WKRC | 결제 2건 × 120,888 gas × 47,600 gwei = 약 11.5 WKRC | 13 WKRC(제안한 settleGasEstimate 기준 12.4 WKRC를 올림) | 결정 필요 |
| 12주 WKRC 필요량 | 약 550 WKRC | 배포 1세트 약 165.8 WKRC, 준비 트랜잭션(등록·mint·approve·입금·신뢰 등록) 합계 약 15.8 WKRC | 배포 횟수(9주차 재배포 여부)가 정해진 뒤 다시 계산 | 결정 필요 |
| 9주차 재배포 | 계획 문서 네 곳에 남아 있음 | 확장 기능이 이번 배포에 들어 있다 | 10주차 강화 결과가 컨트랙트를 바꿀 때만 재배포 | 결정 필요(일정) |
| 기기 순차 nonce | 결제 프로토콜 2절 규칙 | 소프트웨어 서명 정산이 256 배수 시작값의 순차 nonce로 성공 | 펌웨어 서명 모듈 구현 때 같은 규칙을 적합성 harness로 확인 | 펌웨어 구현 대기 |

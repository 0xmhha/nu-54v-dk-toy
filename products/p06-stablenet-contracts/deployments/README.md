# 정산 컨트랙트 배포 기록

체인별 배포 기록을 `<chainId>.json`으로 둔다. 이 폴더의 값이 컨트랙트 주소의 원본이다. `docs/content` 아래 문서는 줄인 주소만 쓰고 여기를 가리킨다.

## StableNet 테스트넷 (8283)

[`8283.json`](8283.json). 2026-10-01(UTC 9/30) 배포, 소스 커밋 `66a5f43`(브랜치 커밋). 스쿼시 머지된 main 커밋 `a8c0c44`의 `src`, `foundry.toml`, `lib`가 같은 git tree라서 main에서 같은 코드를 다시 빌드할 수 있다. 6주차 컨트랙트 게이트를 통과했다([게이트 기록](../../../docs/content/acceptance/gate-w6.md)).

| 계약 | 주소 | 배포 블록 |
|---|---|---|
| PaymentSettlement | `0xDe7596556D35Fa62F238F074A0f0E59cF730caA4` | 21129134 |
| MerchantRegistry | `0x0c293f18815D8336f4F299bF59b796a6CFDA2ae0` | 21129131 |
| TestUSDC (시험 토큰, 6 decimals) | `0x500ef69da230E42bFF34487B578eA0cBefbdA2BA` | 21129130 |

| 파라미터 | 값 |
|---|---|
| perPaymentCap / dailyCap | 50 / 200 tUSDC |
| authorizationExpiry | 120초 |
| withdrawalDelay | 3,600초 |
| payoutChangeDelay | 86,400초 |
| 시험 토큰 보류 반환 대기(RETURN_DELAY) | 3일 |

### 쓰는 쪽별 안내

**키오스크(TypeScript).** `@nu54/contracts-abi`의 `paymentSettlementAbi`로 `settle`을 만들고, 먼저 eth_call로 시뮬레이션한다. revert는 custom error로 디코딩해 거절 코드로 바꾼다([결제 프로토콜](../../../docs/content/specifications/protocol/payment-protocol.md) 7절 제출과 판정, 8절 거절 코드). 결제 판정은 finalized 블록의 `PaymentSettled(merchant, orderId, device, amount, nonce)`로 한다. 8283은 finalized가 latest와 같다.

**운영 도구·indexer(Go).** `settlement`·`registry` 패키지(고정 ABI)와 `settlementext`·`registryext` 패키지(확장 ABI)를 같은 주소에 붙여 쓴다. indexer는 배포 블록부터 읽는다. 가장 이른 블록은 21129130(TestUSDC)이다.

**펌웨어(기기 셋업).** 기기는 셋업 때 chainId 8283과 PaymentSettlement 주소를 기록하고, EIP-712 domain을 이 값으로만 만든다. 결제·한도 변경 nonce는 결제 프로토콜 2절의 순차 규칙을 따른다.

### 역할과 요청 경로

개발용 고정 셋업(가맹점 등록, attestation, TimeAnchor, 입금)은 [opsctl 안내](../../p05-operations-backoffice/README.md)의 명령으로 한다.

역할 키는 배포 때 고정되어 바꿀 수 없다. 역할 주소는 `8283.json`의 `roles`에 있다.

| 필요한 일 | 누가 한다 | 요청 방법 |
|---|---|---|
| 가맹점 등록·철회, payout 변경 요청 | registry admin | 가맹점 서명 주소와 payout 주소를 운영 담당에게 전달 |
| 기기 계정 입금, 출금 요청, 반납(closeAccount) | 운영자 | 기기 주소와 대여자 withdrawAddress를 전달 |
| 시험 토큰 발행 | 시험 토큰 owner | 받을 주소를 전달. 받을 계정의 받지않을권리 모드가 켜져 있으면 발행이 실패한다 |
| 결제·한도 변경 제출 | 키오스크 가스 키 | 권한이 필요 없다. 누구나 제출할 수 있다 |

### 트랜잭션 수수료

8283은 tip이 27,600 gwei보다 낮거나 fee cap이 base fee + tip보다 낮은 트랜잭션을 거절한다(2026-10-01 관측, base fee 20,000 gwei). 노드의 `eth_maxPriorityFeePerGas`를 tip으로 쓰고, fee cap은 `2 × base fee + tip`으로 둔다. `script/testnet-forge.sh`가 이렇게 한다. 기기 첫 결제 settle은 120,888 gas, 약 5.75 WKRC였다.

### 받지않을권리 모드와 연동

- 정산 컨트랙트는 시험 토큰의 신뢰 송신자로 등록되어 있다. 그래서 가맹점 payout과 대여자 withdrawAddress는 모드가 켜져 있어도 지급을 바로 받는다. 수신자가 정산 컨트랙트를 신뢰 목록에서 제외하면 지급은 보류되고, 수신자가 `acceptTransfer`로 받는다.
- 보류된 이체의 체인상 수신자는 토큰 컨트랙트다. 알림 서비스는 `TransferHeld(id, from, to, amount)`를 본다.
- 거절되어 정산 컨트랙트로 돌아온 지급은 잉여분이 되고, 운영자가 `recoverSurplus`로 회수해 원래 주인에게 돌려준다.

### 다시 배포할 때

1. `script/testnet-forge.sh --simulate script/Deploy.s.sol deployer`로 모의 실행한다.
2. `script/testnet-forge.sh script/Deploy.s.sol deployer`로 배포한다.
3. `NU54_SETTLEMENT=<새 주소> script/testnet-forge.sh script/TrustSettlement.s.sol token-owner`로 신뢰 송신자를 등록한다.
4. 이 폴더의 `<chainId>.json`, 환경 manifest의 `settlementDeployment`, 게이트 또는 수용 기록을 갱신한다.
5. 주소가 바뀌면 모든 기기를 다시 셋업해야 한다. 기기는 셋업 때 기록한 컨트랙트 주소로만 서명한다.

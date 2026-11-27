# P06 · StableNet contracts

> **DF-20260925-02 기준 (2026-09-25):** 이번 12주 사이클의 범위와 설계는 [기획](../../docs/content/products/p06/plan.md) · [SRS](../../docs/content/products/p06/srs.md) · [유즈케이스](../../docs/content/products/p06/use-cases.md) · [설계](../../docs/content/products/p06/design.md)가 정한다. 아래 원문은 DF-20260920-01 기준이며 충돌하면 위 문서를 따른다.

## 개발 (DF-20260925-02)

| 경로 | 내용 |
|---|---|
| `src/IPaymentSettlement.sol` | W6에 고정하는 정산 인터페이스: 결제, 예치, cashOut, 조회 |
| `src/IPaymentSettlementExtensions.sol` | 나중에 더하는 확장: 한도 변경, 출금, 반납(WBS2-P06-03, P06-04) |
| `src/PaymentSettlement.sol` | 정산 컨트랙트: depositFor, settle(설계 5절 검사 순서), cashOut |
| `src/IMerchantRegistry.sol`, `src/MerchantRegistry.sol` | 가맹점 registry: 등록, 철회, 현재 payout 조회 |
| `src/test-token/TestUSDC.sol` | 시험 토큰(6자리, owner만 mint). 실제 자산이 아니다 |
| `src/PaymentTypes.sol` | EIP-712 구조체, typehash, domain separator |
| `test/` | 요구사항 ID별 단위 시험, EIP-712 벡터 시험, 재진입 시험, invariant 시험 |
| `script/Deploy.s.sol` | 시험 토큰, registry, 정산 컨트랙트 배포. 파라미터는 register에서 읽는다 |
| `script/SoftwareSettle.s.sol` | 소프트웨어 서명으로 결제 1건(6주차 게이트 경로) |
| `src/charge/` | 충전 계약: 출발 체인의 입금 계약 `ChargeDeposit`, StableNet의 원화 풀 `WonPool`, 키와 변경 절차, 고정 환율 공급자. 배포하지 않았다. [충전 계약 설계](../../docs/content/commercial/traveller-exchange-contracts.md) |
| `src/test-token/TestChargeUSDC.sol` | 충전 시험용 USDC(6자리, `version()` 있음, 거절 기능 없음). 실제 자산이 아니다 |
| `test/vectors/charge.vectors` | 충전의 서명 기준값. `test/ChargeVectors.t.sol`이 실제 계약으로 확인한다 |
| `lib/forge-std/` | forge-std v1.10.0(MIT/Apache-2.0, `VENDORED.md`) |

```bash
make build        # verified Forge build
make test         # compatibility, Solidity tests, fixed gas
make sandbox-up   # 저장소 루트에서. anvil chainId 8283

# sandbox 배포 (anvil 키는 로컬 전용)
NU54_OPERATOR=... NU54_REGISTRY_ADMIN=... NU54_TOKEN_OWNER=... \
  python3 ../../scripts/contract_forge.py p06 script script/Deploy.s.sol --rpc-url sandbox --broadcast --private-key <anvil key>
```

ABI와 Go·TypeScript 바인딩은 [`packages/contracts-abi`](../../packages/contracts-abi/README.md)가 이 폴더의 빌드에서 생성한다.

현재 P06 및 P08 빌드는 Solidity 0.8.39, Prague, optimizer 200회, legacy codegen을 사용한다. Foundry 1.5.1에서 컴파일러를 미리 준비하거나 오프라인 테스트를 실행할 때는 아래와 같이 저장소 루트에서 공식 바이너리의 SHA-256을 확인하고 해당 경로를 넘긴다. macOS용 공식 바이너리는 Apple Silicon에서 Rosetta가 필요하다.

```sh
solc_root=$(mktemp -d)
FOUNDRY_SOLC=$(python3 scripts/install_contract_compiler.py "$solc_root/solc-0.8.39")
export FOUNDRY_SOLC
make -C products/p06-stablenet-contracts test
make -C products/p08-market-services test
```

두 테스트는 [0.8.30 호환성 기준](compiler-compatibility.json)과 실제 계약의 전체 ABI 및 스토리지 레이아웃을 대조한다. 새 바이너리로 생성된 배포물의 코드 해시는 해당 배포에서 다시 측정한다. 8283 기존 배포의 소스와 빌드 입력은 [배포 기록](deployments/README.md)에 있다.



StableNet testnet의 자산, 계정, 자격과 유료 리소스 계약 제품이다.

- **소유 범위:** dummy USDC, WKRC, 일반 EOA 경로, Smart Account, DID,
  제한형 CafePass 테스트 자산, x402 지급 계약과 배포 manifest
- **주요 경계:** P03/P08/P10과 C5, P07과 C6(event)
- **WBS:** `WBS-P06-01`부터 `WBS-P06-05`

설계 진입점:

- [제품 WBS](../../docs/content/planning/product-wbs-overview.md#p06--stablenet-토큰스마트계정자격x402)
- [StableNet 호환 설계](../../docs/content/specifications/stablenet-compatibility-design.md)
- [자격과 유료 리소스](../../docs/content/specifications/credential-paid-resource-design.md)
- [기존 계약 재사용 계획](../../docs/content/stable-contract-reuse-plan.md)

구현 시 계약 source, test, deployment script와 주소·ABI·code hash·배포 block
manifest를 이 폴더에 둔다. 실제 key와 관리자 secret은 저장하지 않는다.

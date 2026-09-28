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
| `lib/forge-std/` | forge-std v1.10.0(MIT/Apache-2.0, `VENDORED.md`) |

```bash
make build        # forge build
make test         # forge test
make sandbox-up   # 저장소 루트에서. anvil chainId 8283

# sandbox 배포 (anvil 키는 로컬 전용)
NU54_OPERATOR=... NU54_REGISTRY_ADMIN=... NU54_TOKEN_OWNER=... \
  forge script script/Deploy.s.sol --rpc-url sandbox --broadcast --private-key <anvil key>
```

ABI와 Go·TypeScript 바인딩은 [`packages/contracts-abi`](../../packages/contracts-abi/README.md)가 이 폴더의 빌드에서 생성한다.


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

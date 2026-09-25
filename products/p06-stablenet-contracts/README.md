# P06 · StableNet contracts

> **DF-20260925-02 기준 (2026-09-25):** 이번 12주 사이클의 범위와 설계는 [기획](../../docs/content/products/p06/plan.md) · [SRS](../../docs/content/products/p06/srs.md) · [유즈케이스](../../docs/content/products/p06/use-cases.md) · [설계](../../docs/content/products/p06/design.md)가 정한다. 아래 원문은 DF-20260920-01 기준이며 충돌하면 위 문서를 따른다.

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

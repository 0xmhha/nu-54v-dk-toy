# P08 · Market services

StableNet testnet에서 학습·시연할 시장 상품 제품 경계다.

- **소유 범위:** AMM/LP/swap, TEST FX, perpetual 포지션·펀딩·청산,
  oracle·keeper, 견적과 앱 결과
- **주요 경계:** P06과 C5, P07과 C7, P02와 C8
- **WBS:** `WBS-P08-01`부터 `WBS-P08-07`

설계 진입점:

- [제품 WBS](../../docs/content/planning/product-wbs-overview.md#p08--dexfxperpetual-서비스)
- [시장 상품 설계](../../docs/content/specifications/market-product-design.md)
- [StableNet 호환 설계](../../docs/content/specifications/stablenet-compatibility-design.md)
- [공통 API 계약](../../docs/content/specifications/interface-contracts.md)

구현 시 계약과 off-chain service의 경계를 하위 디렉터리로 분리하고 가격
source, stale 처리, keeper 실패 수용 시험을 함께 둔다.

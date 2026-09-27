# P04 · Merchant kiosk

> **DF-20260925-02 기준 (2026-09-25):** 이번 12주 사이클의 범위와 설계는 [기획](../../docs/content/products/p04/plan.md) · [SRS](../../docs/content/products/p04/srs.md) · [유즈케이스](../../docs/content/products/p04/use-cases.md) · [설계](../../docs/content/products/p04/design.md)가 정한다. 아래 원문은 DF-20260920-01 기준이며 충돌하면 위 문서를 따른다.

## 개발 (DF-20260925-02)

| 경로 | 내용 |
|---|---|
| `src/` | React Native 0.87(New Architecture, TypeScript) 키오스크 앱 |
| `src/specs/NativeNusBle.ts` | BLE central Turbo Module 명세. Kotlin·Swift 구현은 WBS2-P04-01에서 붙인다 |
| `android/`, `ios/` | 네이티브 프로젝트(Turbo Module 구현 위치) |
| `test/` | Jest 시험 |

```bash
make setup        # 저장소 루트에서 pnpm install
make test
make lint
make run          # react-native run-android
```


가맹점 Android 태블릿에서 실행하는 React Native 키오스크 제품이다.

- **소유 범위:** 점주 로그인, 매장·메뉴·재고·주문, BLE 결제 세션,
  stablecoin 결제, 부분/전체 환불, 매출·정산 화면
- **주요 경계:** P01과 C2, P10과 C3, P07의 결제 projection
- **WBS:** `WBS-P04-01`부터 `WBS-P04-05`

설계 진입점:

- [제품 WBS](../../docs/content/planning/product-wbs-overview.md#p04--rn-키오스크매장-운영)
- [키오스크 상거래 여정](../../docs/content/specifications/kiosk-commerce-journey-design.md)
- [결제·환불·반납 상태](../../docs/content/specifications/commerce-lifecycle-design.md)
- [매출·정산 계약](../../docs/content/specifications/settlement-ops-contracts.md)

구현을 시작할 때 RN 앱, 태블릿 build profile, 오프라인·재연결 상태와 E2E
fixture를 이 폴더에 둔다.

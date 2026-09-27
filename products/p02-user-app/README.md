# P02 · User app

> **DF-20260925-02 기준 (2026-09-25):** 이 제품은 이번 12주 사이클에서 설계만 하고 구현하지 않는다(out of cycle). [설계 동결 DF-20260925-02](../../docs/content/planning/design-freeze-checkpoint-02.md) 참조. 아래 원문은 DF-20260920-01 기준이다.

여행자와 점주가 기기와 지갑 기능을 사용하는 React Native 모바일 앱이다.

- **소유 범위:** Google/Apple 로그인, 계정 수명주기, 기기 등록·설정,
  두 지갑과 자산 선택, 승인·송금·영수증 UI, FOTA 진행 상태
- **주요 경계:** P01과 C1, P03과 C4, P10과 C3, P08/P09와 C8
- **WBS:** `WBS-P02-01`부터 `WBS-P02-04`

설계 진입점:

- [제품 WBS](../../docs/content/planning/product-wbs-overview.md#p02--유저-앱소셜-로그인기기-설정)
- [소셜 로그인과 두 지갑](../../docs/content/specifications/social-wallet-recovery-design.md)
- [화면 흐름](../../docs/content/specifications/screen-flows.md)
- [기기 등록·반납 수용](../../docs/content/specifications/lifecycle-journey-acceptance.md)

구현을 시작할 때 앱 package, Android/iOS target, BLE native module 경계를 이
폴더에 둔다. 현재 제품 코드는 아직 없다.

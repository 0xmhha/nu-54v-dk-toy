# P01 · NU-54V-DK device firmware

> **DF-20260925-02 기준 (2026-09-25):** 이번 12주 사이클의 범위와 설계는 [기획](../../docs/content/products/p01/plan.md) · [SRS](../../docs/content/products/p01/srs.md) · [유즈케이스](../../docs/content/products/p01/use-cases.md) · [설계](../../docs/content/products/p01/design.md)가 정한다. 아래 원문은 DF-20260920-01 기준이며 충돌하면 위 문서를 따른다.

NU-54V-DK에서 실행되는 Zephyr 기반 제품이다.

- **소유 범위:** 보드 bring-up, 보호 키와 EOA 서명, 인증 BLE, 기기 설정,
  결제 표시·물리 승인, FOTA, 패스키, 오디오 전송, 찾기, 자원 중재
- **주요 경계:** P02와 C1(BLE/SMP/CTAP/Audio), P04와 C2(임시 결제 승인)
- **선행 위험:** 실제 board revision, pin, flash/RAM, bootloader와 BLE 처리량
- **WBS:** `WBS-P01-01`부터 `WBS-P01-11`

설계 진입점:

- [제품 WBS](../../docs/content/planning/product-wbs-overview.md#p01--nu-54v-dk-펌웨어주변-부품)
- [기기 동시동작과 FOTA](../../docs/content/specifications/device-coexistence-design.md)
- [구현 인터페이스](../../docs/content/specifications/implementation-interfaces.md)
- [기본 페리페럴 bring-up 기록지](../../docs/content/nu54v-basic-peripheral-bringup-log.md)

구현을 시작할 때 `app/`, `boards/`, `modules/`, `tests/` 구조와 실제 Zephyr
버전을 이 폴더에 기록한다. 현재 제품 코드는 아직 없다.

# P05 · Operations backoffice

> **DF-20260925-02 기준 (2026-09-25):** 이번 12주 사이클의 범위와 설계는 [기획](../../docs/content/products/p05/plan.md) · [SRS](../../docs/content/products/p05/srs.md) · [유즈케이스](../../docs/content/products/p05/use-cases.md) · [설계](../../docs/content/products/p05/design.md)가 정한다. 아래 원문은 DF-20260920-01 기준이며 충돌하면 위 문서를 따른다.

운영자가 가맹점, 대여 기기와 결제 예외를 관리하는 제품이다.

- **소유 범위:** 운영자 RBAC와 감사, 가맹점 승인, 대여·반납, FOTA 캠페인,
  잔액 복구 확인, 늦은 결제·중복·부분·초과 지급 대사
- **주요 경계:** P07 projection, P10 권한·감사, P01/P02 기기 수명주기
- **WBS:** `WBS-P05-01`부터 `WBS-P05-04`

설계 진입점:

- [제품 WBS](../../docs/content/planning/product-wbs-overview.md#p05--백오피스대여운영-신뢰)
- [운영 신뢰 설계](../../docs/content/specifications/trust-operations-design.md)
- [관리 신뢰 수명주기](../../docs/content/specifications/management-trust-lifecycle.md)
- [운영·릴리스 수용](../../docs/content/specifications/operations-release-acceptance-design.md)

구현 시 web UI와 운영 API의 배포 단위, 고위험 작업의 승인 증거와 복구
runbook을 이 폴더에서 관리한다.

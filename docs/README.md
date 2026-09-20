# Documentation

제품 구현과 분리된 설계 authority, 발행 원고, 분석 결과를 보관한다.

| 경로 | 역할 |
|---|---|
| `content/planning/` | 범위, 결정, WBS, 인계, 설계 동결 |
| `content/specifications/` | API, BLE, DTO, 저장, 상태, 보안 계약과 검사기 |
| `content/environment/` | 비밀값 없는 toolchain, chain, provider manifest |
| `content/analysis/document-logic/` | 문서 그래프, 추적성, 논리 검토 결과와 snapshot |
| `content/assets/` | 프로젝트 이미지와 다이어그램 |
| `design-history/` | 이전 승인 기준의 읽기 전용 archive |

현재 기준은 다음 순서로 읽는다.

1. [저장소 checkpoint](../REPOSITORY-CHECKPOINT.md)
2. [설계 동결 checkpoint](content/planning/design-freeze-checkpoint.md)
3. [채택된 기준 계약](content/specifications/design-baseline-contract.md)
4. [12주 WBS](content/planning/product-wbs-overview.md)
5. [제품별 작업 원장](content/planning/product-worklist-and-12week-wbs.md)

제품별 진입점은 [products 인덱스](../products/README.md)에서 확인한다.

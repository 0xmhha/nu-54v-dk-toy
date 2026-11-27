# Documentation

제품 구현과 분리된 설계 authority, 발행 원고, 분석 결과를 보관한다.

| 경로 | 역할 |
|---|---|
| `development-overview.md` | [개발 현황과 문서 안내](development-overview.md). 이전의 루트 README 내용 |
| `development-conventions.md` | [개발 규칙](development-conventions.md). 제품 scope, 커밋, 브랜치, 로컬 검증 |
| `content/planning/` | 범위, 결정, WBS, 인계, 설계 동결 |
| `content/specifications/` | API, BLE, DTO, 저장, 상태, 보안 계약과 검사기 |
| `content/environment/` | 비밀값 없는 toolchain, chain, provider manifest |
| `content/analysis/document-logic/` | 문서 그래프, 추적성, 논리 검토 결과와 snapshot |
| `content/products/` | DF-20260925-02 제품별 기획·SRS·유즈케이스·설계 |
| [제품별 PRD·기획·Use Case·IA](content/commercial/products/README.md) | 10개 제품의 기능별 완료 기준, 정상·실패 흐름, 화면·서비스 권한, 원 요구사항 12개와의 연결 |
| [상용 하드월렛·결제 시스템 설계](content/commercial/README.md) | 현행 코드와 실기기 증거, 여섯 네트워크, 모바일·키오스크·프로토콜, 보안 및 출시 수용 기준 |
| `content/series/` | 12주 제작기 Medium 시리즈 포맷과 10편 목차 |
| `content/assets/` | 프로젝트 이미지와 다이어그램 |
| `design-history/` | 이전 승인 기준의 읽기 전용 archive |

현재 기준은 다음 순서로 읽는다.

1. [저장소 checkpoint](../REPOSITORY-CHECKPOINT.md)
2. [개발 현황과 문서 안내](development-overview.md)
3. [설계 동결 DF-20260925-02](content/planning/design-freeze-checkpoint-02.md)
4. [12주 WBS (DF-20260925-02)](content/planning/product-worklist-and-12week-wbs-02.md)
5. [결제 프로토콜](content/specifications/protocol/payment-protocol.md)
6. [이전 설계 동결 DF-20260920-01](content/planning/design-freeze-checkpoint.md) — DF-20260925-02와 충돌하지 않는 부분만 유효
7. [채택된 기준 계약](content/specifications/design-baseline-contract.md)
8. [이전 12주 WBS](content/planning/product-wbs-overview.md)
9. [이전 제품별 작업 원장](content/planning/product-worklist-and-12week-wbs.md)

제품별 진입점은 [products 인덱스](../products/README.md)에서 확인한다.

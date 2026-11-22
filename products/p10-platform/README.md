# P10 · Common platform

> **DF-20260925-02 기준 (2026-09-25):** 이번 12주 사이클의 범위와 설계는 [기획](../../docs/content/products/p10/plan.md) · [SRS](../../docs/content/products/p10/srs.md) · [유즈케이스](../../docs/content/products/p10/use-cases.md) · [설계](../../docs/content/products/p10/design.md)가 정한다. 아래 원문은 DF-20260920-01 기준이며 충돌하면 위 문서를 따른다.

## 개발 (DF-20260925-02)

| 경로 | 내용 |
|---|---|
| `harness/run_conformance.py` | 모든 구현이 공유 EIP-712 벡터를 재현하는지 한 번에 확인한다 |
| [`baseline/`](baseline/README.md) | 상용화 이전 v1 소스·배포 기준선, 소스 SBOM과 패치 장부, 합성 결제·거절·인덱서 재시작 |
| [`database/`](database/README.md) | P05·P07·P10 소유자별 PostgreSQL 마이그레이션, 목적 목록, 빈 DB·v1 전진 적용 검사 |
| [`release/`](release/README.md) | CI 입력 잠금, 독립 소비 SDK 빌드, 산출물·서명·설치 증거 검증 |
| `../../packages/protocol/` | 스키마에서 Go, TS, Python, C 타입을 생성하는 공유 package |

```bash
make test                 # 이 폴더에서
make conformance          # 저장소 루트에서, 같은 검사
```


모든 제품이 같은 주문, 결제, 권한과 릴리스 의미를 사용하도록 하는 공통
플랫폼 제품이다.

- **소유 범위:** API와 인증·인가, 공통 ID·상태·오류·멱등성, schema 생성,
  환경 manifest, CI, 합성 fixture, 수용 시험, 관측·백업·릴리스·인수
- **주요 경계:** C1~C9 계약의 authority와 호환성 검증
- **WBS:** `WBS-P10-01`부터 `WBS-P10-06`

설계 진입점:

- [제품 WBS](../../docs/content/planning/product-wbs-overview.md#p10--공통-기반검증릴리스-도구)
- [채택된 기준 계약](../../docs/content/specifications/design-baseline-contract.md)
- [API·BLE·데이터 계약](../../docs/content/specifications/interface-contracts.md)
- [구현 순서](../../docs/content/planning/implementation-sequence.md)
- [환경 manifest](../../docs/content/environment/README.md)

공유 생성 코드는 `packages/`에 두되 원본 schema와 compatibility test는 이
제품의 수용 범위로 유지한다.

## 릴리스 및 개인정보 저장소 권한

[제품 마이그레이션](database/README.md)으로 릴리스 DB와 개인정보 통제 스키마를 적용한다. 처리자별 데이터베이스와 개별 서비스 로그인을 사용한다. 아래 그룹 역할에 필요한 로그인만 연결한다.

| 그룹 역할 | 권한 |
|---|---|
| `nu54_p10_release_build`, `nu54_p10_release_approve` | 빌드 증거·권고 기록과 승격 영수증 저장을 분리 |
| `nu54_p10_restore_request`, `nu54_p10_restore_verify`, `nu54_p10_restore_apply` | 복원 요청, 별도 검증, 전환 영수증 저장을 분리 |
| `nu54_p10_retention_admin` | 목적·근거·기산점·기간 참조가 있는 보유 규칙 등록 |
| `nu54_p10_audit_writer`, `nu54_p10_audit_reader` | 접근 기록의 추가와 감사를 분리 |
| `nu54_p10_rights` | 범위가 한정된 요청 및 삭제 결과 영수증 관리 |

접근 기록과 권리 접수는 등록된 보유 규칙과 유한한 보유 종료 시각을 저장한다. 암호화 키와 사용자 범위 명세는 별도 경계에 보관한다. 대상별 실제 삭제와 복원 시 삭제 대장 재적용은 실행 계획 18·21에서 검증한다.

## 격리 복원과 승격

[격리 복원·삭제 대장·대상별 서명 전환](recovery/README.md)

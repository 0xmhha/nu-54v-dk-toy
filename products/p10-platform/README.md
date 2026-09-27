# P10 · Common platform

> **DF-20260925-02 기준 (2026-09-25):** 이번 12주 사이클의 범위와 설계는 [기획](../../docs/content/products/p10/plan.md) · [SRS](../../docs/content/products/p10/srs.md) · [유즈케이스](../../docs/content/products/p10/use-cases.md) · [설계](../../docs/content/products/p10/design.md)가 정한다. 아래 원문은 DF-20260920-01 기준이며 충돌하면 위 문서를 따른다.

## 개발 (DF-20260925-02)

| 경로 | 내용 |
|---|---|
| `harness/run_conformance.py` | 모든 구현이 공유 EIP-712 벡터를 재현하는지 한 번에 확인한다 |
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

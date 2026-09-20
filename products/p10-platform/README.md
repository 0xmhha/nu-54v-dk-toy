# P10 · Common platform

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

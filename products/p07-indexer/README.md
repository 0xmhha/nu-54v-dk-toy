# P07 · Indexer and query frontend

StableNet의 canonical event를 업무 projection으로 만드는 제품이다.

- **소유 범위:** 배포 계약 등록, RPC ingest, cursor, deduplication, backfill,
  reorg rollback, decoder, projection, 조회 API와 탐색 프런트엔드
- **주요 경계:** P06에서 C6, P05/P08/P09/P10으로 C7
- **WBS:** `WBS-P07-01`부터 `WBS-P07-06`
- **재사용 후보:** `0xmhha/indexer-go`, `0xmhha/indexer-frontend`

설계 진입점:

- [제품 WBS](../../docs/content/planning/product-wbs-overview.md#p07--indexer조회-프런트엔드)
- [Indexer 통합 계획](../../docs/content/indexer-integration-and-test-token.md)
- [StableNet 기준](../../docs/content/stablenet-testnet-baseline.md)
- [상거래 보정 설계](../../docs/content/specifications/commerce-consumer-repair-design.md)

기존 저장소를 가져오기 전에 commit pin, 라이선스, 이력 보존 방식을 기록한다.

# P07 · Indexer and query frontend

> **DF-20260925-02 기준 (2026-09-25):** 이번 12주 사이클의 범위와 설계는 [기획](../../docs/content/products/p07/plan.md) · [SRS](../../docs/content/products/p07/srs.md) · [유즈케이스](../../docs/content/products/p07/use-cases.md) · [설계](../../docs/content/products/p07/design.md)가 정한다. 아래 원문은 DF-20260920-01 기준이며 충돌하면 위 문서를 따른다.

## 개발 (DF-20260925-02)

| 경로 | 내용 |
|---|---|
| `internal/ingest/` | finalized PaymentSettled 수집 루프: 1000블록 단위 `eth_getLogs`, 정산 컨트랙트와 이벤트 topic0만, 오류면 cursor 유지 |
| `internal/store/` | PostgreSQL 저장: 로그 행과 cursor를 한 트랜잭션에, `(txHash, logIndex)` 중복 무시, 조회는 가장 이른 로그와 `duplicate` 표시 |
| `internal/receipt/` | 읽기 전용 조회 API `GET /receipts/{merchant}/{orderId}`: 200 영수증, 404 `NOT_INDEXED`, cursor가 60블록 넘게 뒤처지면 503 `RPC_STALE` |
| `internal/chain/` | JSON-RPC(finalized 헤더, 로그, 블록 시각) |
| `cmd/indexer/` | 서버 진입점. 환경 변수: `P07_DATABASE_URL`(필수), `P07_RPC`, `P07_DEPLOYMENT`, `P07_HTTP_ADDR`, `P07_POLL` |
| `migrations/` | PostgreSQL 스키마(시작할 때 적용) |
| `Dockerfile` | 저장소 루트를 빌드 컨텍스트로 쓰는 이미지 |

```bash
make test                                                   # PostgreSQL 시험은 P07_TEST_DATABASE_URL이 있을 때만
P07_DATABASE_URL=postgres://... make run                    # :8080, 저장소 루트에서 배포 기록을 읽는다
curl localhost:8080/receipts/<merchant>/<orderId>
make docker
```

2026-10-02 테스트넷 실행: 배포 블록부터 finalized까지 따라잡는 데 45초가 걸리지 않았다. 지금까지의 결제 3건(6주차 게이트, 10/1 키오스크 모듈, 10/1 보드)이 모두 조회되었다. 중지했다가 다시 시작하면 저장된 cursor 다음부터 이어 읽었다. 이벤트가 finalized 된 뒤 조회가 되기까지 걸리는 시간(P07-NFR-01, 10초)은 새 결제가 있어야 잴 수 있어 아직 재지 않았다.


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

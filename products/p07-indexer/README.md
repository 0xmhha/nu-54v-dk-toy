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

2026-10-02 테스트넷 실행: 배포 블록부터 finalized까지 따라잡는 데 45초가 걸리지 않았다. 지금까지의 결제 3건(6주차 게이트, 10/1 키오스크 모듈, 10/1 보드)이 모두 조회되었다. 중지했다가 다시 시작하면 저장된 cursor 다음부터 이어 읽었다. 이벤트가 finalized 된 뒤 조회가 되기까지 걸리는 시간(P07-NFR-01, 목표 10초)은 2026-10-02에 쟀다. 소프트웨어 기기로 testnet 결제 1건(`0x238c31…8df7`, 블록 21276884)을 보내고, 수집 DB를 100 ms마다 보다가 행이 생긴 순간을 블록 시각과 비교했다. 0.12초였다(8283은 finalized가 latest와 같고, 이때 체인과 Mac의 시계 차이는 0초였다). 표본은 1건이며, 폴링 주기가 2초이므로 최악은 약 2초에 RPC 응답 시간을 더한 값이다. 조회 API의 값(블록, merchant, orderId, device, amount, 줄인 tx hash)은 `cast receipt`와 같았다.


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

## 데이터베이스 배포

공용 v1 영수증과 커서에 마이그레이션 001을 유지하고, 002에서 독립된 `chainview` 스키마를 적용한다. `P07_MIGRATION_DATABASE_URL`을 사용하는 [제품 마이그레이션](../p10-platform/database/README.md)으로 배포한다. 런타임 로그인에는 기능별 그룹을 부여한다.

| 그룹 역할 | 권한 |
|---|---|
| `nu54_p07_legacy` | 기존 인덱서의 영수증 읽기·삽입 및 커서 전진 |
| `nu54_p07_ingest` | 세대별 블록·이벤트·증거 기록과 체크포인트 관리 |
| `nu54_p07_read` | `chainview.active_evidence`에서 활성 정규 체인의 확정 증거 조회 |
| `nu54_p07_switch` | 비교 증적이 있는 전환 요청 등록과 `chainview.apply_switch(request_id)` 실행 |

전환 서비스가 운영자 인증과 비교 증적을 검증한다. DB는 준비된 세대를 잠그고 기대 revision을 대조해 읽기 대상과 적용 영수증을 한 번에 저장한다. 같은 요청을 재시도하면 기존 적용 시각을 반환한다. 신규 읽기 대상의 기대 revision은 0이다. `P07_DATABASE_URL`로 실행하는 기존 인덱서는 `nu54_p07_legacy`를 사용한다.

[버전별 체인 수집·조회·장애 검증](chainview-v2.md)에서 P07 003의 프로필, 운영 명령, 읽기 권한 및 포크 시험을 확인할 수 있다. 주문 에스크로(OrderEscrow) 프로필과 `chainview.escrow_receipt`는 [같은 문서의 해당 절](chainview-v2.md#orderescrow-profile)에 있다.

## 격리 복원 입구

복원 관리 배포는 `P07_RECOVERY_DATABASE_URL`, `P07_RECOVERY_SCOPE`, `P07_RECOVERY_MANIFEST`를 함께 설정합니다. 일반 영수증·C7 조회, 체인 수집 및 부재 증명은 현재 세대의 입구 임대를 확인합니다. [P10 운영 구성과 복원 검증](../p10-platform/recovery/README.md)

# P07 기획 — 최소 PaymentSettled indexer

## 1. 목표와 범위

P07은 [DF-20260925-02](../../planning/design-freeze-checkpoint-02.md)에서 만드는 여섯 제품 중 하나다 [N01]. 정산 컨트랙트(P06)가 내는 `PaymentSettled(merchant, orderId)` 이벤트를 finalized 블록에서 모아, 키오스크가 영수증을 다시 조회할 수 있게 하는 최소 indexer다 [N19].

P07은 영수증 조회용이다. 결제가 끝났는지(paid)는 키오스크가 체인에서 finalized PaymentSettled를 직접 확인해서 정하고, P07은 그 판정에 쓰지 않는다 [N08].

## 2. 산출물

| 산출물 | 내용 |
|---|---|
| ingest 프로세스 | StableNet 8283 RPC에서 finalized 블록 범위의 PaymentSettled 로그를 읽어 저장한다 |
| 저장소 | `(txHash, logIndex)`로 중복을 막는 영수증 테이블과 cursor 한 행 |
| 조회 API | `(merchant, orderId)`로 영수증 1건을 돌려주는 읽기 전용 HTTP 엔드포인트 |
| 영수증 페이지 | 키오스크가 여는 단순 영수증 화면(API 응답을 그대로 표시) |
| 시험 기록 | receipt lookup log(WBS2-P07-01의 증거) |

## 3. 일정

| WBS | 작업 | 담당 | 기간 |
|---|---|---|---|
| WBS2-P07-01 | Minimal PaymentSettled indexer and receipt lookup | role B | W9–W10 |

P07은 W4·W6·W8 게이트 조건이 아니다. 선행 작업은 P06 컨트랙트의 8283 배포(WBS2-P06-02)와 키오스크 제출 경로(WBS2-P04-02)다. 두 작업이 끝나야 실제 PaymentSettled 로그가 생긴다.

## 4. 재사용 판단

`0xmhha/indexer-go`는 이벤트 하나를 읽는 데 필요한 부분만 싸게 가져올 수 있을 때만 쓴다. 판단 기준은 W9 첫날 하루 안에 8283 RPC 연결과 로그 decode가 되는가이다. 안 되면 직접 `eth_getLogs` 폴링으로 만든다. 폴링 구현은 반나절 정도 규모라 재사용이 일정을 줄이지 못하면 버린다.

## 5. 위험과 컷

- P07은 W6 게이트가 실패했을 때 두 번째로 자르는 항목이다 [N03]. 잘리면 WBS2-P07-01을 하지 않는다.
- 잘린 경우 W12-06(P07 영수증)은 conditional 항목이 되어, 키오스크가 finalized PaymentSettled 이벤트를 직접 읽은 기록으로 대신한다 [N18].
- role B는 W9–W10에 P07만 맡는다. W8까지 키오스크 제출(WBS2-P04-02)이 밀리면 P07 시작도 밀리므로, W8 끝에 P07 착수 여부를 다시 정한다.

## 6. 범위 밖

- 다른 제품용 projection(스탬프, 매출 집계, 시장 상품 등)
- finalized 이전 블록을 읽는 reorg rollback 처리
- 과거 구간 backfill 화면, 운영자용 탐색 UI
- 정산·환불 대사

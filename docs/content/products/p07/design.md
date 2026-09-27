# P07 설계 — 최소 PaymentSettled indexer

## 1. 구조

P07은 프로세스 하나에 두 부분이 있다. 폴링 루프가 체인에서 로그를 모으고, 읽기 전용 HTTP 서버가 영수증을 돌려준다. 둘은 같은 SQLite 파일을 쓴다. 근거 결정은 [N19]이며, 결제 판정에는 참여하지 않는다 [N08].

```
StableNet 8283 RPC ──(finalized, eth_getLogs)──> ingest loop ──> SQLite(receipts, cursor)
                                                                    │
키오스크(P04) ──GET /receipts/{merchant}/{orderId}──> HTTP 서버 ──────┘
```

## 2. ingest 루프

1. `eth_getBlockByNumber("finalized")`로 finalized 번호 F를 읽는다.
2. cursor C가 F 이상이면 폴링 주기만큼 기다린다.
3. `eth_getLogs{address: 정산 컨트랙트, topics: [topic0], fromBlock: C+1, toBlock: min(F, C+1000)}`를 호출한다. topic0는 `keccak256("PaymentSettled(address,bytes32,address,uint256,uint256)")`이다 [N08].
4. topics[1..3]에서 merchant·orderId·device를, data에서 amount·nonce를 decode해 한 transaction 안에서 영수증 행을 넣고 cursor를 toBlock으로 옮긴다.
5. RPC나 decode 오류는 transaction 전체를 되돌리고 cursor를 유지한다.

finalized 블록만 읽으므로 reorg rollback은 두지 않는다. 폴링 주기는 2초로 두어 제품 목표 P07-NFR-01(10 s)을 여유 있게 맞춘다. 이 값은 영수증 조회 목표이며 paid 판정과 무관하다.

## 3. 저장

| 테이블 | 열 | 제약 |
|---|---|---|
| `receipts` | merchant, orderId, device, amount, nonce, txHash, logIndex, blockNumber, blockTime | PK `(txHash, logIndex)`, 인덱스 `(merchant, orderId, blockNumber)` |
| `cursor` | id(=1), blockNumber | 한 행 |

`(merchant, orderId)`에 여러 행이 있으면 조회는 가장 작은 blockNumber를 돌려주고 `duplicate: true`를 붙인다. 컨트랙트가 ORDER_ALREADY_PAID로 막으므로 정상이라면 생기지 않는다.

## 4. 조회 API

`GET /receipts/{merchant}/{orderId}`

| 응답 | 본문 |
|---|---|
| 200 | `{merchant, orderId, device, amount, blockNumber, blockTime, txHashShort, duplicate}` |
| 404 | `{error: "NOT_INDEXED"}` |
| 503 | `{error: "RPC_STALE", cursor}` (cursor가 finalized보다 60블록 넘게 뒤처질 때) |

`txHashShort`는 앞 6자리…뒤 4자리로 줄인 값이다 [N16]. 영수증 페이지는 이 응답을 표 하나로 보여 준다.

## 5. 재사용

`0xmhha/indexer-go`에서 RPC 클라이언트와 로그 decode만 가져올 수 있으면 쓴다. 전체 projection 구조는 가져오지 않는다. 판단은 [기획](plan.md) 4절을 따른다.

## 6. 시험 설계

| 시험 | 방법 | 기대 |
|---|---|---|
| 중복 ingest | 같은 블록 범위를 두 번 처리 | 행 수 동일 |
| 재시작 | 루프 중간에 프로세스 종료 후 재시작 | `eth_getLogs` 전체 결과와 행 집합 동일 |
| 다른 컨트랙트 로그 | 다른 주소에서 같은 이벤트 발생 | 저장 안 됨 |
| 조회 지연 | 결제 후 finalized 시각부터 조회 성공까지 | 10 s 이내 |
| RPC 장애 | RPC를 끊고 30초 뒤 복구 | cursor 유지, 복구 후 누락 없음 |

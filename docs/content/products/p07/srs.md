# P07 SRS — 최소 PaymentSettled indexer

## 1. 목적

P07이 해야 하는 일을 시험으로 판정할 수 있게 적는다. 근거 결정은 P07 최소 indexer를 정한 [N19]와 paid 판정을 정한 [N08]이다. 메시지·이벤트 형식은 [결제 프로토콜](../../specifications/protocol/payment-protocol.md)을 따른다.

## 2. 용어

| 용어 | 뜻 |
|---|---|
| PaymentSettled | 정산 컨트랙트가 결제 1건을 가맹점 잔액으로 옮길 때 내는 이벤트 `PaymentSettled(address indexed merchant, bytes32 indexed orderId, address indexed device, uint256 amount, uint256 nonce)`. topic0는 이 전체 시그니처로 계산하고, merchant·orderId·device는 indexed topic이다 [N08] |
| finalized | StableNet 8283에서 되돌려지지 않는 블록. 8283은 1초 블록이고 finalized가 latest와 같다 |
| cursor | 마지막으로 처리한 finalized 블록 번호 |
| 영수증 | PaymentSettled 로그 1건과 그 블록 번호·시간·tx hash |

## 3. 판정 경로와의 관계

P07은 paid 판정 경로에 없다 [N08]. 결제가 끝났는지는 키오스크가 체인에서 finalized PaymentSettled를 직접 확인해서 정한다. P07이 멈추거나 늦어도 결제 판정과 키오스크 결과(approved, refused, failed, Checking)는 바뀌지 않아야 한다.

## 4. 기능 요구

| ID | 요구 | 확인 방법 |
|---|---|---|
| P07-FR-01 | 설정된 정산 컨트랙트 주소 하나의 PaymentSettled 로그(위 전체 시그니처의 topic0)만 finalized 블록 범위에서 읽는다 [N19] | 다른 주소·다른 이벤트 로그가 저장되지 않는 시험 |
| P07-FR-02 | 처리한 블록 번호를 cursor로 저장하고, 재시작하면 cursor 다음 블록부터 읽는다 | 중간 종료 후 재시작 시 누락·중복 없음 |
| P07-FR-03 | `(txHash, logIndex)`가 같은 로그는 한 번만 저장한다 | 같은 범위를 두 번 ingest해도 행 수가 같음 |
| P07-FR-04 | indexed topic인 `(merchant, orderId)`로 영수증 1건을 조회한다. 없으면 404 | 결제 전·후 조회 결과 |
| P07-FR-05 | 한 `(merchant, orderId)`에 로그가 두 건 이상이면 가장 이른 로그를 돌려주고 `duplicate` 표시를 붙인다 | 컨트랙트 유일성 위반 탐지용 합성 로그 시험 |
| P07-FR-06 | 영수증 응답은 merchant, orderId, device, amount, 블록 번호, 블록 시간, 줄인 tx hash를 담는다 | 응답 필드 검사 |

## 5. 비기능 요구

| ID | 요구 |
|---|---|
| P07-NFR-01 | 영수증 조회 가능 시점의 제품 목표: 이벤트가 finalized 된 뒤 10 s 안(폴링 주기 포함). paid 판정이나 서버 SLO가 아니며, D19에서 확정한 값은 키오스크 10 s timeout 하나뿐이다 [D19] |
| P07-NFR-02 | RPC 오류 시 cursor를 앞으로 옮기지 않고 재시도한다 |
| P07-NFR-03 | 쓰기 API가 없다. 조회 API는 읽기 전용이다 |
| P07-NFR-04 | 로그와 화면에는 줄인 주소·해시만 쓴다 [N16] |

## 6. 인터페이스

- 입력: 8283 RPC `eth_getBlockByNumber("finalized")`, `eth_getLogs`
- 출력: `GET /receipts/{merchant}/{orderId}` → JSON 영수증 또는 404
- 소비자: 키오스크(P04) 영수증 화면, W12-06 증거 수집

## 7. 추적

| 요구 | 결정 | 수용 항목 |
|---|---|---|
| P07-FR-01..03 | [N19] | W12-06 |
| P07-FR-04..06 | [N19] | W12-06 |
| 3절 판정 경로 분리 | [N08] | W12-03 |

# 결제·환불 대사: 현재 승인 계약과의 연결 설계

2026-09-18 · **현재 기준과 호환되는 변경안 작성. 기준 카탈로그 병합·제품 구현·SQL 적용·실행 시험은 미수행.**

과거 결제 후보는 API 107개 시점의 body/data fragment였다. 현재 API 110개 승인 기준과 대조하여 **HTTP 5개 경로의 전체 논리 요청·응답, 이벤트 2개, 저장 자원 4개, 원자 처리 4개**를 연결했다. 정책 선택을 대신하지 않으며, 현재 기준 카탈로그의 API 수·권한 수·이벤트 수는 바꾸지 않았다.

[연결 원본](commerce-reconciliation-integration.json) · [독립형 Schema](commerce-reconciliation-integration.schema.json) · [합성 예제](commerce-reconciliation-integration.examples.json) · [검사기](validate_commerce_reconciliation.py) · [현재 승인 기준](approval-baseline.md) · [승인 저장·잠금 설계](approval-physical-storage-design.md)

## 1. 기존 후보와 현재 기준의 차이

| 항목 | 확인한 차이 | 이번 연결안 |
|---|---|---|
| API-037 환불 승인 | 과거 후보는 revision 두 개만 요구. 현재는 merchantSigner와 전체 승인 snapshot도 필수 | 현재 요청·응답 schema를 그대로 포함. 과거 body로 덮어쓰지 않음 |
| API-036 환불 요청 | 현재 기준에 정확한 paymentId/funding revision이 없음 | 두 값 필수화와 전체 요청·응답/오류 envelope 제안 |
| API-030/035/038 조회 | 기존 지급 이력만으로 현재 유효성·환불 보류를 표현하기 부족 | 고객/점주 projection, allocation 유효성, 환불 대사 상태 연결 |
| 잠금 순서 | 과거 commerce 문서는 order부터 시작. 새 승인 설계는 shared gate부터 시작 | 모든 writer에 APS gate→권한→원천 원장 순서 적용; companion은 anchor 직후 잠금 |
| source 무효화 | 관측·주문 상태만 갱신하면 이미 열린 refund gate가 남을 수 있음 | allocation 무효화와 funding hold·revision·shared gate 변경을 같은 commit에 포함 |
| 이벤트 | v1 이름은 있으나 대사 revision 정보가 부족 | 기존 이름 2개에 v2 typed envelope와 단일 적용/전환 규칙 제안 |
| 검증 기반 | 과거 후보의 hash와 validator는 병합 전 기준 | 현재 입력 15개 hash 고정, 독립형 로컬 참조 schema와 별도 검사기. 과거 파일/검증 이력 유지 |

`paymentId`는 **payment_allocations.id**이다. 주문 ID·attempt ID·txHash·evidence ID와 다르다. 어떤 화면에서 선택했더라도 서버가 allocation→원주문→매장→자산→관측 근거를 다시 확인한다.

## 2. HTTP 계약과 인증·버전 경계

### API-030 · 조회 projection과 원천 목록 query

`GET /v1/orders/{orderId}`

- 요청/응답: `API030Request` / `API030Response`
- 버전: `commerce-reconciliation-v1-draft`
- 연결 자원: OrderReconciliation, PaymentException, RefundFundingGuard
- 권한·처리: 현재 exact-store 환불 요청 권한일 때만 원천/잔액 목록; 일반 주문 권한은 order_public. cursor는 principal/store/order/sourceListRevision 결합.

### API-035 · 원 attempt의 allocation 현재 유효성

`GET /v1/payment-attempts/{attemptId}`

- 요청/응답: `API035Request` / `API035Response`
- 버전: `commerce-reconciliation-v1-draft`
- 연결 자원: OrderReconciliation, PaymentException
- 권한·처리: 경로 attempt의 증거·allocation만 반환. 다른 지급 승자/주소/환불 한도 노출 금지.

### API-036 · paymentId와 funding revision 필수

`POST /v1/orders/{orderId}/refunds`

- 요청/응답: `API036Request` / `API036Response`
- 버전: `commerce-reconciliation-v1-draft`
- 연결 자원: RefundFundingGuard, RefundReconciliation
- 권한·처리: allocation→order/store/asset를 서버가 확인. 현재 환불 요청권, 목적지 proof와 양수 금액, 보류/예약 한도 재검사.

### API-037 · 현재 승인 schema 그대로 유지

`POST /v1/refunds/{refundId}/authorize`

- 요청/응답: `API037Request` / `API037Response`
- 버전: `approval-v1-draft`
- 연결 자원: RefundFundingGuard, RefundReconciliation, ApprovalBinding
- 권한·처리: expectedRevision+expectedFundingRevision+merchantSigner 유지. 업무 승인자와 실제 signer 구분; source/context/예약을 불변 snapshot에 결합.

### API-038 · 자기 환불 또는 권한 있는 source 조회

`GET /v1/refunds/{refundId}`

- 요청/응답: `API038Request` / `API038Response`
- 버전: `commerce-reconciliation-v1-draft`
- 연결 자원: RefundReconciliation, RefundFundingGuard
- 권한·처리: 기본 조회: 해당 refund의 정확한 store에 대해 현재 refund_read 허용 역할 또는 refund→balance→allocation의 현재 ReceiptOwnership/Eligibility. 같은 주문만으로 고객 권한 확장 금지. 원천 예산 포함 store_refund_operator projection은 추가로 API030과 동일한 exact-store store_refund_request 권한이 필요하다. 일반 매출 조회권만으로 refund 조회 불가. source 권한 없는 합법적 조회자는 budget 없는 refund_customer 형태를 사용하며 이 literal은 응답 형식이지 고객 신분 인증이 아니다.

네 경로(API-030/035/036/038)는 논리 header `commerceContractVersion=commerce-reconciliation-v1-draft`를 사용하는 제안이다. 실제 HTTP header 이름/대소문자 매핑과 배포 협상은 D02 결정 전이다. API-037은 이미 존재하는 `approval-v1-draft` 전체 envelope를 그대로 사용한다. API-018과 BLE 승인 schema도 변경하지 않는다. commerce 버전과 approval 버전을 하나로 대체하지 않는다.

조회 API-030/038은 단일 요청에서 `authorization` 또는 `resourceCapability` 중 하나를 지정한다. capability의 action/audience/만료/자원 범위는 별도 현재 검증 대상이며 문자열 존재만으로 인증이 되지 않는다. API-035는 정확한 attempt의 capability, API-036/037은 현재 계정 인증이 필요하다. source 목록 query가 있다고 더 강한 projection으로 승격하지 않는다.

API-030의 refundSourcesLimit은 1..100의 제안 범위를 검사한다. 생략 시 기본값은 D19 미선정이며 예제의 20을 확정값으로 읽지 않는다. cursor는 주체·매장·주문·sourceListRevision에 결합하고 권한 검사 뒤 해석한다. 내용이 바뀐 cursor는 재조회 대상이다. 공개 주문 조회자가 목록 query를 보내면 권한 오류이며 존재 정보나 예산을 제공하지 않는다. query/projection의 교차 권한은 JSON Schema가 아니라 서버 검사 책임이다.

API-038의 `refund_customer`는 제한 응답의 기존 literal이다. 현재 정확한 매장 refund_read 권한이 있는 일반 직원도 이 제한 형태를 받을 수 있으나, 고객 신분이나 고객 영수증 소유권을 얻는다는 의미는 아니다. 고객은 반드시 자기 allocation 소유를 확인한다. source 예산 포함 `store_refund_operator`에는 **추가로 exact-store store_refund_request 권한**을 검사한다. 일반 sales_viewer·주문 조회권을 환불 읽기 권한으로 자동 확대하지 않는다.

모든 새 응답은 no-store다. signed bytes/access token을 commerce 응답에 추가하지 않는다. 보관된 승인 원문과 실제 signer의 권한은 기존 승인 계약으로만 다룬다.

### 오류·복구 제안

| HTTP | 코드 예 | 화면/재시도 처리 |
|---|---|---|
| 401 | AUTH_REQUIRED | 현재 인증 복구 후 재조회 |
| 403/404 | SCOPE_DENIED / RESOURCE_UNAVAILABLE | 타인 source 존재를 드러내지 않는 정책 적용, 다른 원천 자동 탐색 금지 |
| 409 | SOURCE_CHANGED / CURSOR_STALE / IDEMPOTENCY_CONFLICT | 원천·목록 재조회; 동일 key의 body를 바꿔 재시도하지 않음 |
| 409 | FUNDING_HELD | 보류 사유의 허용된 표현 표시; 잔액이 남아도 새 환불 금지 |
| 422 | CONTRACT_UNSUPPORTED / INVALID_REQUEST | 지원되지 않는 버전/구조를 legacy로 낮추지 않음 |
| 429/503 | RATE_LIMITED / SERVICE_UNAVAILABLE / POLICY_UNAVAILABLE | 안전한 재조회 또는 운영 보류. 동일 자금 mutation 자동 반복 금지 |

새 CommerceErrorResponse는 이 오류 envelope의 형태를 제안한다. HTTP status와 code의 정확한 조합/정보 은닉/Retry-After는 adapter에서 위 표대로 검증할 대상이다. 현재 승인 API-037의 오류 envelope는 원본을 유지한다.

## 3. 원장·정책의 경계

주문의 paid/fulfilled는 과거 업무 이력일 수 있다. 현재 지급 유효성은 별도 paymentSummary/allocation/observation으로 판정한다. 체인 재구성 때 과거 상품 인도 기록을 지우거나 주문을 자동 unpaid로 덮어쓰지 않는다.

D08의 최초 유효 지급 수락 방식, 부분 지급 합산, 늦은/예외 지급 반환은 미확정이다. 새로운 수락 판정에는 선택된 policyVersion이 필요하며 미선정이면 운영 보류 상태로 둔다. 이미 수락된 기록의 재관측은 원 저장 정책과 증거를 사용한다. 이번 작업의 합성 예제가 특정 수락 정책을 승인한 것은 아니다.

예외 지급에도 별도 allocation과 정확한 자기 자금의 refund balance를 연결할 수 있도록 설계한다. 예외라는 문자열만으로 환불권을 주거나 다른 payer의 한도를 빌리지 않는다. 잘못된 자산·수취인·미확정 귀속은 검증 전 자동 반환하지 않는다.

## 4. 원자 처리 연결

### CRI-T01 · 주문 수락·취소·예외 대사

진입: validated observation or cancellation command

잠금: APS rank0 gates → rank20 order/attempt/allocation/balance; companion rows lock immediately after anchor

같은 commit에 포함:

- observation application idempotency
- allocation validity and exception
- order reconciliation/primary/history
- source list revision
- outbox event
- affected allocation funding guard hold/reasons/funding_revision + shared authorization gate revision
- affected refund read revision/hold projection (lock each affected refund/reservation in APS order)

규칙: Known policy version required for choosing a new primary; unresolved policy holds new acceptance decision. Existing accepted history and reorg correction use stored policy. Never mark fulfilled history unpaid automatically. Source invalidation and blocking fresh refund authorization are the same transaction. Preserve cap/reserved/confirmed exposure; revalidated source clears only its own hold reason after stored policy checks, never other independent holds. Affected scope gates are resolved before any ledger mutation; missing scope discovery aborts/restarts with full lock set.

### CRI-T02 · 환불 요청·예약

진입: API-036

잠금: APS gates → memberships → order/allocation/balance+guard → refund/reservation

같은 commit에 포함:

- verify current same-key outcome before creating second reservation
- new refund snapshot bound to payment/asset/destination/amount
- reserved counter/reservation
- funding and source-list revisions
- idempotency outcome + outbox

규칙: Same key different normalized request conflicts. Authorized exact replay returns original outcome without new reservation; current auth required. New request requires current expectedFundingRevision and no hold.

### CRI-T03 · 업무 승인·서명·전송 재확인

진입: API-037 then API-015/hardware acceptance then API-018 SS-T03/SS-T04

잠금: same shared authorization gates and source guards per APS lock order

같은 commit에 포함:

- API037 preserves signer selection/current authority checks
- approval intent/parent operation/binding/snapshot + refund authorization
- immutable authorized refund/funding revision
- later prepare/permit current gate snapshots

규칙: No HTTP shape change for API037/018. Guard change must be visible before each controllable new signing/release/permit. Changed authorization binding requires conflict/reapproval under current contract, never rewrite original snapshot. Existing signatures/permit remain possible exposure.

### CRI-T04 · 환불 관측 보정·재확정

진입: refund observation revision

잠금: APS gates → order/allocation/balance+guard/refund/reservation → observation application

같은 commit에 포함:

- exact original refund/intent observation binding
- confirmed to reserved or reverse
- refund revision/reconciliation and funding revision/source-list revision
- observation application + outbox + inbox

규칙: cap10/reserved2/confirmed6 -> orphaned6 -> reserved8/confirmed0/available2. A held funding source cannot authorize fresh movement; cap never silently lowered below exposure. Absence/timeouts/lease/UI cancel do not release possible exposure.

원지급 무효화 시 그 allocation의 funding guard를 보류하고 fundingRevision과 공유 authorization gate를 같은 transaction에서 갱신한다. 원환불들의 조회 revision/보류 projection과 원천 목록 revision도 반영한다. 영향 gate/행 목록을 잠근 후 누락 대상을 발견하면 transaction 전체를 다시 시작한다. 큰 fan-out을 임의의 비원자 batch로 쪼개지 않는다. 성능 시험 후 분할이 필요하면 먼저 동일 차단 효과를 갖는 protocol을 별도로 설계해야 한다.

재확정으로 원지급이 복구되어도 원지급 사유의 hold만 정책 확인 후 해제한다. 분실·권한 철회·기타 위험으로 생긴 독립 hold까지 지우지 않는다. 정상적인 신규 승인/제출은 현재 guard를 재확인하며, 이미 외부로 나갈 수 있었던 서명·permit·거래의 추적과 예약은 계속 유지한다.

환불 reorg의 계산 예:

| 시점 | 환불 상한 | 예약 | 확정 | 추가 예약 가능액 |
|---|---:|---:|---:|---:|
| 최초 | 10 | 2 | 6 | 2 |
| 확정 6의 reorg | 10 | 8 | 0 | 2 |
| 동일 환불 재확정 | 10 | 2 | 6 | 2 |

보류 중에는 수학적 가능액이 2여도 새 자금 이동을 승인하지 않는다. timeout/not-found/lease 만료/UI 취소만으로 예약을 해제하지 않는다. 노출 가능성이 없는 실패 또는 기존 설계가 요구하는 종결 근거를 확인해야 한다.

## 5. 논리 자원 4개의 물리 연결 제안

추가 4개 companion 구조는 후보 이름이며 기존 61개 테이블이나 이전 승인 후보 16개와 합쳐 구현 완료 수치로 계산하지 않는다. 실행 DDL은 없다. PK·FK·조건을 실제 SQL로 작성/검증하기 전 필요한 관계를 명시한다.

### OrderReconciliation → `commerce_order_reconciliations`

기준키: `order_id FK orders.id`

필드: `primary_payment_id? FK payment_allocations.id`, `state`, `reconciliation_revision`, `source_list_revision`, `policy_version`, `review_reasons`, `observed_at?`

- PK order_id; primary allocation must belong to same order via composite FK(candidate unique allocation.id/order_id)
- primary invalidation retains primary_payment_id; no silent alternate promotion
- source_list_revision increments on source add/budget/hold changes, separate from summary revision

공유 gate: source/order

### PaymentException → `commerce_payment_exceptions`

기준키: `payment_id FK payment_allocations.id`

필드: `reason_code`, `resolution_state`, `policy_version`, `revision`

- PK payment_id; original evidence/attempt/order/asset remain immutable in allocation
- resolution cannot imply ordinary sale or automatic refund; D08 unresolved

공유 gate: source/order

### RefundFundingGuard → `commerce_refund_funding_guards`

기준키: `balance_id FK refund_balances.id`

필드: `payment_id`, `funding_revision`, `authorization_held`, `hold_reasons`, `source_observation_id?`, `source_observation_revision?`, `policy_version`

- PK balance_id; UNIQUE payment_id; composite balance/payment relation requires matching unique target on existing balance
- all reserve/confirm/release/reverse/hold mutations increment same funding_revision
- source observation ID/revision both null or both nonnull; authority gate references same stored guard, no independent stale hold copy

공유 gate: source/order and source/refund funding authority

### RefundReconciliation → `commerce_refund_reconciliations`

기준키: `refund_id FK refunds.id`

필드: `observation_id?`, `observation_revision?`, `canonicality`, `confirmation`, `execution`, `reason_codes`, `observed_at?`

- PK refund_id; API reconciliation.revision derives from refunds.revision, not separately incremented counter
- reservationState derives from refund_reservations.state; no second mutable reservation authority
- exact observation/submission belongs to same refund intent, source revision preserved with application evidence

공유 gate: source/order and source/refund

`fundingRevision`, `orderReconciliationRevision`, `sourceListRevision`, allocation/refund/observation revision은 서로 다른 권위의 숫자다. sourceListRevision을 funding revision으로 제출하거나 두 카운터의 크기를 비교하지 않는다. API-038 reconciliation.revision은 refunds.revision으로 표현하고 예약 상태는 기존 refund_reservations에서 읽어 복제된 두 권위를 만들지 않는다.

기존 SQL의 소속 FK/고유키가 새 composite 관계의 충분한 target인지 차기 DDL에서 확인해야 한다. reference SQL의 기존 제약 시험 통과가 이 구조를 검증한 것은 아니다. gate는 실제 guard 상태를 같은 transaction에서 읽으며 다른 저장소의 지연된 hold 복사본을 사용하지 않는다.

## 6. 이벤트 v2·중복·역순·버전 전환

| 이벤트 | aggregate 기준 | revision 기준 | 원천 관측 |
|---|---|---|---|
| payment.acceptance.changed v2 | payment_allocation / paymentId | allocation.revision | ID/revision 필수 |
| refund.state.changed v2 | refund / refundId | refund.revision | 관측 외 업무 원인이면 ID/revision 모두 NULL |

aggregateId는 payload의 paymentId/refundId와 같아야 한다. 관측 ID/revision은 같은 저장된 observation 이력으로 검증한다. 단순 형식 검사만으로 관측의 진위·귀속을 입증하지 않는다.

- Same source transaction writes outbox. A redelivery retains eventId; consumer inbox uniqueness by consumer/eventId.
- Also deduplicate logical application by UNIQUE(consumer,aggregateType,aggregateId,aggregateRevision); normalized content digest is a comparison value, not part of the unique key. Version1/2 copies with different event IDs must not double apply; disagreement at the same aggregate revision quarantines and reconciles.
- All revision comparisons confined to their authority: allocation, order reconciliation, source list, funding, refund, observation. Never compare across counters.
- Unknown version, missing checkpoint, gap or out-of-order input triggers source snapshot/history reconciliation; no blind additive delta. Older event cannot regress applied target.
- Version1 lacks enough reconciliation fields: resolve authoritative current snapshot/history or hold. Do not synthesize zero funding/observation revisions.
- Record consumer checkpoint and applied projection atomically. Only one ledger-writing normalization path during version migration; shadow reader cannot write ledger.
- Internal events contain source links; mobile/SSE/device output is re-projected under current resource authorization. Public order capability never receives raw internal event.

전환 순서는 (1) 원천 snapshot과 consumer checkpoint 초기화 → (2) v2 shadow 검토(원장 쓰기 없음) → (3) 정규화된 단일 ledger writer 전환 → (4) v1 재전송/잔여 queue 대사 → (5) v1 종료다. 새 버전의 정보가 부족한 v1 메시지는 권위 있는 snapshot을 조회하거나 보류하며 누락 revision을 0으로 채우지 않는다. rollback 시에도 미지원 v2를 paid로 축약하여 구 writer에 전달하지 않는다.

consumer마다 적용 키는 `(consumer, aggregateType, aggregateId, aggregateRevision)`다. 정규화 content digest는 동일 key 충돌 검사용 값이며 unique key 구성요소가 아니다. 같은 revision의 다른 내용이 새 정상 이벤트로 들어오지 않도록 한다. projection과 checkpoint/inbox는 같은 원자 경계로 반영한다.

매출·정산·혜택·영수증·여행별 목표 projection의 구체 계산과 snapshot repair 증거는 다음 설계 항목이다. 이번에 원장 재처리 구현이나 실제 Indexer 재구성을 시험한 것은 아니다.

## 7. 화면 연결

| 화면 | 적용할 행동 |
|---|---|
| K02 | 주문 이력과 현재 지급 유효성을 분리; canStartPayment는 안내이며 mutation 권한이 아님. |
| K03 | submit 성공만으로 주문 완료 금지. 해당 attempt allocation/observation 현재 결과 표시; reconfirmation/exception일 때 자동 재결제 안내 금지. |
| K05 | 권한 있는 source 목록에서 paymentId 선택→036 예약→038 최신 상태→037 업무 승인→지정 signer. 충돌 시 source 재조회, source 자동 교체 금지. |
| U03 | 현재 refund approval snapshot 그대로 확인. store approver와 signer가 다른 경우 원 source를 유지; held 상태는 새 승인/전송 제한. |
| U11 | 자기 allocation에 속한 환불만; 지급 재확인/환불 관측중/확정 표시를 구분. 매장 source 예산·다른 고객 정보 노출 금지. |

## 8. 실행 수용 사례

모든 사례는 **not_run**이며 실제 서비스·동시 요청·기기·체인 시험이 필요하다.

| ID | 상황 | 기대 결과 |
|---|---|---|
| CRI-R01 | 동시 지급 두 건 | 정책 버전이 확정된 경우에만 주수락 판정; 원천별 allocation을 보존하고 중복 매출 없음. |
| CRI-R02 | 취소와 수락 경쟁 | 동일 order gate/원장 잠금으로 직렬화; 늦은 지급은 미확정 D08 정책에 따라 보류, 자동 환불 금지. |
| CRI-R03 | 주수락 지급 reorg | 기존 paid/fulfilled 이력과 primary 참조 유지; reconfirmation, 신규 지출 보류, 대체 지급 자동 승격 금지. |
| CRI-R04 | 부분 지급 여러 건 | 합산 정책 미선정이면 자동 완납으로 만들지 않음; 원 지급마다 증거와 자산을 유지. |
| CRI-R05 | 다른 payer의 주문 내 환불 조회 | 같은 주문이어도 allocation 소유가 다르면 거절; source budget 비공개. |
| CRI-R06 | 같은 source 동시 예약 | 최신 funding revision/한도 검사로 한도 초과 예약 방지; 실패자는 원 source 재조회. |
| CRI-R07 | 예약 commit 후 응답 유실 | 현재 권한과 같은 멱등 digest로 원 결과 반환; 오래된 revision으로 두 번째 예약 생성 금지. |
| CRI-R08 | 승인과 funding hold 경쟁 | 공유 gate/guard 직렬화; 먼저 hold면 거절, 이미 승인 후 hold면 다음 controllable 단계 재확인. |
| CRI-R09 | 허가/서명 후 통신 두절 | 원 tx 관측과 예약 유지; 새 환불 자동 생성이나 원 서명 폐기 완료 주장 금지. |
| CRI-R10 | 환불 확정 후 reorg와 재확정 | confirmed↔reserved가 모든 counter/revision/outbox와 함께 한 번씩 반영. |
| CRI-R11 | 이벤트 v1/v2 이중 수신 | 하나의 논리 적용 경로와 source revision으로 중복 계상 방지; 같은 revision 내용 충돌은 격리. |
| CRI-R12 | 이벤트 누락·역순 | 현재 source snapshot으로 목표 projection 재계산; 무조건 delta 적용 금지. |
| CRI-R13 | 원천 목록 조회 중 권한/내용 변경 | principal/store/order/revision을 확인; 권한 철회 거절, cursor stale 시 첫 페이지 재조회. |
| CRI-R14 | 구버전 화면이 재확인을 이해 못함 | 버전 미지원 오류 및 새 자금 동작 차단; paid로 축약하거나 legacy source 자동 선택 금지. |
| CRI-R15 | 점주 일반 조회와 원천 예산 권한 분리 | 현재 exact-store refund_read만 있으면 제한 응답; store_refund_request까지 확인된 경우만 source budget 포함. sales_viewer나 주문 조회권만 있으면 환불 읽기 권한을 만들지 않음. |

## 9. 검증·채택 상태와 다음 순서

현재 입력 파일 15개의 hash와 로컬 schema 참조를 검사한다. 합성 정상 17개·거절 12개, 자원 귀속/한도/reorg/cursor 등 유한 의미 예제 16개를 검증하며 API-037이 현재 승인 정의와 동일함을 별도로 대조한다. 이는 인증·동시성·DB·실제 이벤트 전송을 검증한 결과가 아니다.

이 안은 현재 기준에 **연결 가능한 변경 명세**로 등록했으며 API/권한/화면/event 기준 파일은 덮어쓰지 않았다. 원 후보와 현재 승인 기준을 함께 가리킨다. 정식 기준 병합 때는 D08 정책 선택, D02 버전/저장 adapter, D19 기본 페이지/운영 임계값, 소비자 전환과 화면 수용을 묶어서 판정해야 한다. RR-DEC-01과 구현 보류 상태도 유지한다.

후속 [소비자 보정·재처리 계약](commerce-consumer-repair-design.md)에 매출·정산·혜택·영수증·여행의 목표 기여분, 불변 보정, 완전한 source manifest와 재구축 절차를 정리했다. 다음은 매출·정산 API의 조회·마감·보정과 운영 재처리 명령 상세 계약이다.

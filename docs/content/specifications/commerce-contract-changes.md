# 결제·환불 API·이벤트·저장 계약 변경안

**상세 설계 검토안. 구현 미착수, 기존 계약에 병합 전.** [상태 전이 설계](commerce-lifecycle-design.md)의 LC-GAP-01~03을 구체화한다. 기존 107개 API·10개 이벤트·61개 참조 SQL 테이블은 그대로다. 새 테이블/서버를 생성하지 않았다. 최초 업무 수락 1건·늦은 지급 예외 처리 등 정책은 제안 상태를 유지한다.

- [변경 목록과 원본 hash](commerce-contract-candidate.json)
- [후보 JSON Schema](commerce-contract-candidate.schema.json)
- [합성 검토 예제](commerce-contract-examples.json)

`commerce-reconciliation-candidate-1`은 검토용 프로필 이름이다. 실제 API가 이 프로필을 제공한다는 뜻이 아니다. schema는 변경할 request.body 또는 response.body.data와 내부 이벤트 전체를 정의한다. HTTP/path/header/기존 오류 envelope는 별도로 유지한다. 공통 타입은 검증 도구가 현재 `extended-dtos.schema.json`을 로컬 `urn:nu:commerce:baseline` 리소스로 결합한다. 네트워크에서 schema를 가져오지 않는다.

## 1. 바뀌는 계약 5개

| API | 변경안 | 판단 책임 |
|---|---|---|
| API-030 주문 조회 | 자유 문자열 PaymentSummary.state를 상태 enum으로 제한. 고객용과 환불 담당자용 projection 분리 | 주문 대사 결과와 현재 조회 권한으로 서버가 선택 |
| API-035 결제 시도 조회 | 해당 attempt의 allocation 유효성·주수락 여부·보정 사유 추가 | 경로 attempt에 결합된 증거만 반환 |
| API-036 환불 요청 | `paymentId`, `expectedFundingRevision` 추가 | 서버가 지급 귀속·자산·주문·목적지·예약 가능액 재검증 |
| API-037 환불 승인 | 기존 `expectedRevision` 외 `expectedFundingRevision` 추가 | 실제 환불 source의 보류·예약·최신 권한을 재검증 |
| API-038 환불 조회 | 재관측 이유·예약 상태·정규성/확정 정보, 권한 있는 점주의 source 정보 추가 | 원환불 조회 권한과 해당 지급 귀속으로 정보 제한 |

이 변경안에서 **paymentId는 payment_allocations.id**를 뜻한다. txHash, evidenceId, attemptId, orderId와 다르다. 기존 RefundView.paymentId와 이벤트 paymentId도 같은 의미로 매핑한다. 이름만으로 추정하지 않고 adapter 계약으로 고정해야 한다.

### API-030: 주문 이력과 현재 지급 유효성

`paymentSummary` 필드:

| 필드 | 의미 |
|---|---|
| state | unpaid / processing / accepted / reconfirmation_required / exception_only |
| acceptedAmount | 현재 유효하게 수락된 금액. 유효성 재확인 중에는 null |
| reasonCodes | 지급 재확인 등 공개 가능한 요약 사유. 다른 payer의 주소·환불 내역은 제외 |
| revision | 주문 대사 projection의 단조 증가 revision |
| observedAt | 이 projection이 반영한 관측 시점. 아직 없으면 null |
| canStartPayment | 현 주문/대사 상태로 새 결제 요청을 제안할 수 있는지. 쓰기 권한이나 서버 검증을 대신하지 않음 |

판정 우선순위는 다음과 같다.

1. 주수락 지급이 invalidated/확인 불명확이면 reconfirmation_required. 과거 주문 paid/fulfilled는 이력으로 유지한다.
2. 유효한 주수락이 있으면 accepted. 별개 중복 지급 예외가 생겨도 최초 payer의 정상 지급을 무효화하지 않는다.
3. 주수락은 없고 미해결 예외 지급만 있으면 exception_only. 자동 재결제를 안내하지 않는다.
4. 서명/제출/관측 대기 attempt가 있으면 processing.
5. 그 외는 unpaid. cancelled/expired/fulfilled 주문에는 canStartPayment=false다. unpaid가 곧 payable이라는 뜻이 아니다.

여러 이유가 겹치면 주수락 재확인을 우선하며, 운영 예외 상세는 별도 source 목록으로 확인한다. 모든 상태에서 서버는 실제 mutation 직전에 권한·revision·주문 조건을 다시 검사한다.

`projection=order_public`은 order와 요약만 반환한다. 공개 요약에는 primaryPaymentId·다른 payer 주소·환불 budget·다른 attempt ID 목록을 넣지 않는다. 주문 조회권은 환불 조회권으로 확장되지 않는다.

`projection=store_refund_operator`는 **현재 매장의 환불 요청 권한**이 있는 사용자에게만 제공하며 `refundSources`와 `sourcePage`를 추가한다. 일반 sales_viewer나 고객 order capability는 이 projection을 받을 수 없다. 요청자가 projection 문자열을 전달해 선택하는 방식은 없다.

목록은 무제한 배열을 가정하지 않는다. 검토안에는 API-030의 `refundSourcesCursor`와 `refundSourcesLimit(1..100, 기본 20 제안)` query를 추가한다. cursor는 주체·매장·주문·snapshotRevision에 결합한다. 공개 조회에서 이 query를 보내면 거절한다. 변경된 snapshot에서 예전 cursor를 사용하면 재조회하도록 한다. limit 값과 오류 코드는 병합 전 결정 사항이다. 이번 schema는 body/data fragment만 검사하므로 query 범위와 cursor 진위 검사는 검증 범위에 포함되지 않는다.

`sourcePage.snapshotRevision`은 OrderReconciliation의 sourceListRevision이다. source 추가·금액/보류 변경 시 같은 order 잠금 안에서 증가시킨다. 공개 지급 요약 revision과 구분하며, 이전 목록에서 읽은 값으로 쓰기 요청을 하면 최신 fundingRevision으로 다시 검사한다.

### API-035: 해당 지급만 보여주기

기존 attempt/observation/exception에 `allocation`을 추가한다. 아직 배정되지 않았으면 null이며, 배정됐다면 paymentId·state·isPrimary·reasonCodes·revision을 반환한다. 다른 attempt의 승자 ID나 환불 상세를 넣지 않는다.

AttemptView.state=accepted는 과거 진행 이력일 수 있다. 화면의 현재 지급 판정은 allocation과 observation을 우선한다. invalidated인데 과거 attempt.state만 읽고 완료 표시하는 조합을 수용하지 않는다. exception 상세의 subjectRef도 path attempt에 속해야 한다.

### API-036/037: 환불 원천과 변경 충돌

API-036 body 예시의 형태:

```json
{
  "paymentId": "allocation-second-payment",
  "expectedFundingRevision": 7,
  "amount": {"assetId": "dummy-usdc", "atomicAmount": "2000000"},
  "destinationProof": {"proofId": "proof-refund-destination", "profileId": "refund-destination-profile"},
  "reason": "duplicate_payment"
}
```

예시는 합성값이며 proof 또는 자금 이동을 실행할 수 없다.

서버는 paymentId로 allocation → attempt/order/store/asset/evidence를 도출한다. 해당 매장 권한, 양수 금액, 실제 원지급, 목적지 증거와 한도를 검사한다. **paymentId는 선택자이며 권한 증명이 아니다.**

`expectedFundingRevision`은 예약/완료/해제/재관측/보류가 반영되는 source별 revision이다. 같은 잠금 경계에서 revision 검사와 한도 예약을 수행한다. 원지급 선택·금액·자산·목적지 proof·revision은 멱등 digest와 refund snapshot에 고정한다. 같은 key로 원천을 바꾸면 충돌이다. 이미 완료된 동일 요청의 재시도는 현재 읽기 권한을 검사한 뒤 기존 결과를 반환하고 오래된 revision 때문에 새 예약을 만들지 않는다.

API-036 후 점주는 API-038을 읽어 최신 fundingRevision을 얻고 API-037에 환불 expectedRevision과 함께 제출한다. 승인 단계에서도 source 보류, 유효 예약, 목적지 snapshot, 최근 인증·자금 승인 권한을 재검증한다. 두 revision 중 하나라도 바뀌면 충돌 후 재조회한다. 서버가 다른 paymentId나 수취 주소로 조용히 바꾸지 않는다.

승인 이후 보류가 생기면 새 서명 요청과 아직 통제 가능한 중계 제출을 제한한다. API-018에도 저장된 source/intent/보류 조건의 의미 검사를 연결해야 한다. 그러나 이미 외부에 전달된 유효 서명을 무효화할 수 있다고 가정하지 않으며, 그 거래는 관측을 계속한다. API-018의 schema 변경은 이번 후보에 포함하지 않는다.

### API-038: 고객의 조회 범위

`refund_customer`는 refund·transactionRef·reconciliation을 반환한다. `store_refund_operator`는 그 외 환불 source의 budget/hold/fundingRevision을 추가한다.

고객 권한은 **refund → balance → allocation → 현재 ReceiptOwnership/Eligibility**로 확인한다. 같은 주문에 속한 고객이라는 사실만으로 다른 payer의 환불을 볼 수 없다. 현재 authorization-policies.json의 넓은 “원주문에 연결된 고객” 문구는 이 경로로 구체화할 변경 대상으로 표시한다. 기존 보안 영수증 설계의 allocation별 제한을 유지한다.

`reconciliation`은 fundingState(clear/held), 사유, 예약 상태, execution/canonicality/confirmation, observedAt, revision을 반환한다. 이 정보도 해당 환불에 한정한다. 소스 전체의 남은 환불 한도, 다른 환불 ID, 점주 signer 내부 정보는 고객에게 주지 않는다.

## 2. 내부 이벤트 버전 2 후보

기존 DomainEvent.schemaVersion=1을 조용히 바꾸지 않는다. 두 이벤트를 version 2 전체 envelope로 제안하며, 기존 10개 이벤트 이름의 개수는 유지한다.

| 이벤트 | aggregate / revision | payload 핵심 |
|---|---|---|
| payment.acceptance.changed | payment_allocation / allocation.revision | paymentId, orderId, attemptId, evidenceId, 이전/새 유효성, sourceObservationId/revision, orderReconciliationRevision, isPrimary, 사유, 금액 |
| refund.state.changed | refund / refund.revision | refundId, paymentId, 이전/새 상태, 금액, reservationState, fundingRevision, sourceObservationId/revision, 사유 |

환불의 관측/보류/예약 상태가 의미 있게 바뀌면 업무 state 문자열이 같아도 refund.revision을 증가시키고 이벤트를 낸다. API-038 reconciliation.revision은 이 refund revision에 맞춘다. chain observation revision, allocation revision, order reconciliation revision, fundingRevision은 서로 다른 범위의 숫자이므로 대소를 교차 비교하지 않는다.

최초 생성 이벤트의 previousState는 null이며, 존재하지 않았던 이전 상태를 임의로 만들지 않는다.

sourceObservationId와 revision은 같은 저장된 관측을 가리킨다. 관측 외 업무 조치가 원인이면 환불 이벤트의 두 값은 함께 null이다. 이벤트 aggregateId=payload.paymentId 또는 payload.refundId를 검증한다. 시각은 정렬/회계의 유일 근거가 아니다.

소비 규칙:

1. 원천 변경과 outbox는 동일 업무 transaction이다. eventId는 고유하고 재전송에도 같다.
2. 소비자별 inbox에서 eventId와 **해당 aggregate revision**을 검사한다. 중복은 추가 증감하지 않는다.
3. revision이 건너뛰었거나 역순이면 단순 금액 delta 적용 대신 원천 snapshot/누락 이력을 복구한다. 최신 snapshot 기준으로 계산한 목표 결과와 기존 적용 결과의 차이를 보정한다.
4. 원장 projection 변경과 inbox 반영은 소비자의 원자 경계다. 외부 adapter이면 operation/outbox와 재처리 규칙을 별도로 둔다.
5. 내부 이벤트는 고객 스트림이 아니다. SSE/WS/알림 adapter는 전달 시 현재 자원 권한과 allocation별 소유 범위를 검사해 허용된 조회 표현만 만든다.

첫 이벤트를 수신했더라도 baseline snapshot 없이 “초기 이벤트겠지”라고 추정하지 않는다. consumer version별 checkpoint와 source snapshot을 초기화해야 한다. version 1과 2를 함께 읽는 전환기에는 이중 원장 반영을 막는 단일 소비 경로/정규화 버전을 정한다.

## 3. 논리 저장 계약 4개

아래는 기존 테이블에 column/관계로 합칠지 별도 테이블로 둘지 결정하기 전의 논리 자원이다. 실행 DDL이나 물리 테이블 수에 더하지 않는다.

| 논리 자원 | 기준 키·기존 anchor | 추가 필드·불변식 |
|---|---|---|
| OrderReconciliation | orderId / orders·payment_allocations | primaryPaymentId?, state, reconciliationRevision, sourceListRevision, policyVersion, reviewReasons, observedAt. 한 주문의 주수락은 최대 하나; invalidated 원천도 참조 유지 |
| PaymentException | paymentId / payment_allocations | reasonCode, resolutionState, policyVersion, revision. 예외 증거/attempt/payer/asset 보존; 최초 수락과 분리 |
| RefundFundingGuard | balanceId / refund_balances | paymentId, fundingRevision, authorizationHeld, holdReasons, sourceObservationRevision, policyVersion. 예약과 보류의 동시성 기준 공유 |
| RefundReconciliation | refundId / refunds·refund_reservations·transaction_submissions | reconciliationRevision, observationReference?, canonicality, confirmation, reasonCodes, reservationState. 확정↔재관측 전이를 원자 회계와 연결 |

### TX-04: 동시 수락·취소

제안 잠금 순서는 order → 해당 allocation들(ID 정렬) → 관련 balance들(ID 정렬)이다. 취소와 일반 수락은 같은 order 기준으로 직렬화한다. 관측 저장 작업은 이 잠금을 역순 획득하지 않고 대사 입력만 만든다.

주수락이 없고 payable이며 전체 수락 기준을 통과한 증거 하나만 primary에 연결한다. 다른 지급은 자기 allocation의 exception이다. allocation 상태·OrderReconciliation·주문 변경·outbox를 같은 원자 경계로 저장한다. 동일 원천 관측을 다시 읽어도 새 매출은 만들지 않는다.

주수락 invalidation은 primary 참조를 삭제하지 않는다. 다른 지급의 자동 승격은 이번 기본 제안에 없다. 대체 수락 절차가 필요하면 별도 승인·이력·환불 영향 설계를 한다.

### 예외 지급의 환불 원천

기존 refund_balances는 allocation별 고유하다. accepted allocation만을 대상으로 한다는 암묵적 구현 조건을 두지 않는다. exception 지급도 **실제 수취·정규성·확정·귀속·동일 자산·정책**이 검증되면 자기 balance의 반환 대상이 될 수 있다. exception이라는 문자열만으로 환불을 허용하지 않는다.

잘못된 자산/수취인·귀속 불명확 증거는 지급 해석부터 재검토한다. 다른 payer의 자금·원주문 금액에서 예외 반환 한도를 빌려오지 않는다. 늦은/초과 지급의 자동 반환, 분할 합산 여부는 D08 미확정이다.

### TX-05: 예약과 확정 보정

제안 공통 잠금 순서는 order → allocation → balance/guard → refund → reservation이다. 모든 관련 쓰기 경로가 같은 순서를 사용하거나 선택 DB의 동등한 직렬화/재시도 조건을 제공해야 한다. 여러 대상은 같은 종류 안에서 ID 순으로 획득한다.

`available = policy_refundable − reserved − confirmed`는 source 하나·자산 하나 기준이다. 합계가 음수가 되거나 서로 다른 자산을 섞으면 거절한다. authorizationHeld=true이면 수학적 available이 남아 있어도 새 환불 요청/승인을 거절한다.

재관측 시 완료액 q를 예약액으로 돌리고, 재확정 시 반대로 이동한다. refund·reservation·balance/guard revision·outbox가 함께 바뀐다. 예: cap=10, reserved=2, confirmed=6이라면 available=2다. 6이 재관측되면 reserved=8, confirmed=0, available=2로 유지한다. 재확정 뒤 원래 값으로 돌아간다.

원지급 invalidation은 cap을 미결액 아래로 낮추지 않고 guard에 보류 사유와 근거 revision을 기록한다. 기존 서명 거래 추적은 계속한다. guard 상태가 바뀌면 영향받는 환불 조회 revision 및 내부 알림을 갱신한다. 기존 배포가 없으므로 과거 운영 DB 마이그레이션을 했다고 주장하지 않는다.

## 4. 호환·병합 조건

이 후보는 optional 필드만 추가하는 변경이 아니다. required 필드, state enum, 고객/점주 projection 및 query가 달라진다.

- 새 profile/version을 지원하는 앱·키오스크·서버 조합을 호환 행렬에 추가한다. 구체 HTTP 협상 방식은 D02 설계 대상으로 유지한다.
- 구버전 클라이언트가 reconfirmation을 paid로 오해할 수 있으므로 호환 판정 없이 같은 응답을 보내지 않는다. 필요한 자금 동작은 명확한 업그레이드/지원 불가 결과로 중단한다.
- API-036/037의 source/revision 필수화를 적용할 때, 원천 누락을 서버가 임의 선택하는 fallback을 두지 않는다.
- canonical 통합 시 API030/035/036/037/038와 API018 의미 검사, authorization/access mappings, 화면, 이벤트 소비자, 저장 adapter를 함께 변경한다. 단순 schema 복사만으로 완료 처리하지 않는다.
- 현재 파일들의 hash를 후보 목록에 기록했다. 기반이 바뀌면 예전 검증 결과를 재사용하지 않고 차이를 다시 검토한다.

## 5. 이번 검토의 범위와 남은 결정

후보 schema의 합성 정상/거절 예제와 일부 상태·회계 불변식을 로컬에서 대조한다. 실제 인증·proof·DB 잠금·동시 지급·Indexer reorg·앱 화면 실행 시험은 하지 않는다. 예제 통과가 이 기능들의 구현 완료를 의미하지 않는다.

LC-GAP-01~03은 **구체 변경안 작성, 정책/통합 검토 대기** 상태다. D08의 최초 수락·분할 지급·늦은 지급 반환 정책, D02의 버전 협상/물리 저장 구조는 아직 확정하지 않는다. LC-GAP-04~05는 다음 설계에서 초기화 후 증거 접수·반납 중 작업 제한·늦은 자산 복구를 다룬다.

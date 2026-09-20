# 매출·정산·혜택·영수증·여행: 보정과 재처리 계약

LG17 설계 보완 반영 · 정책 미선택 · 기준 미병합 · 제품 구현 및 실행 검증 보류

## status

consumer_projection_repair_design_proposal

## implementation

deferred_by_user

## canonicalMerged

False

## runtimeVerified

False

## sqlApplied

False

## policySelectionUnchanged

True

## baseline

```json
{
  "api": 110,
  "events": 10,
  "physicalSqlTables": 61
}
```

## consumers

### CP-SALES

**title**: 매출

**consumer**: sales

**eventTypes**

```json
[
  "payment.acceptance.changed",
  "refund.state.changed"
]
```

**projectionKey**: storeId/assetId/paymentId

**anchors**

```json
[
  "orders",
  "payment_allocations",
  "payment_evidence",
  "refund_balances",
  "refunds",
  "refund_reservations"
]
```

**targetRules**

```json
[
  "유효 주수락 지급의 grossAcceptedAtomic, 일반 매출 지급에 연결된 confirmedSalesRefundAtomic, 예외 지급 수취/반환을 각각 분리한다.",
  "수취액과 매출을 혼용하지 않는다. canonicalAttributedReceiptsAtomic에는 확인된 예외 수취도 들어갈 수 있지만 grossAcceptedAtomic에는 포함하지 않는다.",
  "netSalesAtomic=grossAcceptedAtomic-confirmedSalesRefundAtomic; netAttributedCashAtomic=canonicalAttributedReceiptsAtomic-allCanonicalRefundOutflowAtomic. 두 net은 signed integer로 표현하며 음수/미해결 차이를 0으로 숨기지 않는다.",
  "기존 SalesTotals는 모든 필드가 UInt이므로 새 net/유효성/진행 정보는 후속 DTO 변경 대상이다. receivedAtomic의 의미를 새 net으로 몰래 바꾸지 않는다."
]
```

**repairRules**

```json
[
  "현재 원천 snapshot에서 지급별 목표 기여분을 계산하고 이전 목표와의 차이만 한 번 반영. 주수락 이력은 원환불의 sales/exception 분류를 유지한다.",
  "source reorg 후 outgoing refund만 canonical이면 음수 차이와 review_required로 표시; 새로운 판매/환불을 자동 생성하지 않음.",
  "자산·chain·환경별 독립 집계. 주문 KRW 가격은 이력 표시이며 token 수량과 합산/자동 환전하지 않음."
]
```

**apiRefs**

```json
[
  "API-039"
]
```

**forbidden**

```json
[
  "원지급과 다른 자산 환불 합산 금지",
  "메뉴 현재 가격으로 과거 매출 재계산 금지",
  "지갑의 주문 외 입금을 매출로 처리 금지"
]
```

**status**: design_proposed_not_implemented

### CP-SETTLEMENT

**title**: 정산

**consumer**: settlements

**eventTypes**

```json
[
  "payment.acceptance.changed",
  "refund.state.changed"
]
```

**projectionKey**: storeId/period/assetId/settlementId

**anchors**

```json
[
  "settlements",
  "settlement_revisions"
]
```

**targetRules**

```json
[
  "정산은 가맹점 코인 수취·환불 대조 기록이며 자동 지급/원화 전환 작업이 아니다.",
  "마감은 완전성 검증된 SourceManifest와 계산된 합계를 고정한다. settlement_revisions.source_revision은 manifest의 로컬 버전으로 연결할 제안이며 모든 원천 revision의 max가 아니다.",
  "마감 후 보정은 이전 version과 새 manifest를 가리키는 새 settlement revision. 원마감 totals_snapshot은 불변.",
  "periodStart<=businessEffectiveAt<periodEnd, 매장 timezone과 period 정책 버전 고정. event 수신 시각으로 날짜를 재배정하지 않는다. 최초 수락/확정의 신뢰 가능한 업무 시각이 없으면 backfill 보류."
]
```

**repairRules**

```json
[
  "open 기간은 목표 projection 갱신; closed 기간은 correction_pending으로 표시하고 exact-store settlement_manage 승인 후 동일 원기간 새 version을 만든다는 제안. 자동 승인 정책은 미선정.",
  "같은 target manifest 재처리는 새 보정 version을 만들지 않는다. 동시 마감/보정은 settlement revision CAS와 consumer fence 검사.",
  "기간/자산별 시각 정책상 sales cohort와 cash movement period가 다를 수 있으므로 각 metric의 attribution manifest를 따로 남긴다. 다음 기간에도 동일 조정을 이중 반영하지 않는다."
]
```

**apiRefs**

```json
[
  "API-040",
  "API-041",
  "API-093"
]
```

**forbidden**

```json
[
  "gap/hold/unknown watermark 상태에서 정상 마감 금지",
  "자동 자금 이동 금지",
  "원마감 기록 덮어쓰기 금지"
]
```

**status**: design_proposed_not_implemented

### CP-BENEFITS

**title**: 혜택·스탬프

**consumer**: benefits

**eventTypes**

```json
[
  "payment.acceptance.changed",
  "refund.state.changed"
]
```

**projectionKey**: 기존 purchase: eligibilityId/ruleVersion/benefitKind. 확장 후보: benefitEffectOwnership.effectKeyFields + 고정 rule binding; challenge는 별도 tagged entitlement source, purchase eligibility 위조 금지.

**anchors**

```json
[
  "benefit_entries",
  "benefit_redemptions",
  "ReceiptEligibility",
  "ReceiptOwnership",
  "PendingBenefitEntitlement"
]
```

**targetRules**

```json
[
  "지급별 목표 적립량은 저장된 ruleVersion으로 계산하며 현재 마케팅 규칙으로 과거 보상을 재발급하지 않는다. 부분환불/이미 사용한 혜택 처리 정책이 없으면 policy_pending.",
  "계정 없는 guest는 PendingBenefitEntitlement에만 기록하고 가짜 계정을 만들지 않는다. 소유 연결 확정 후 같은 source identity로 계정 원장에 한 번 materialize한다.",
  "원장 순혜택=sum(grant+correction+consume). consume 이력은 보정 계산에서 유지한다. 적립 목표만 바뀌므로 사용 이력을 지우거나 자동 재사용권으로 바꾸지 않는다.",
  "보정 후 잔량<0이면 화면에서 deficit/review_required와 추가사용 보류로 표현하는 제안. 돈을 청구하거나 다른 지갑 자산을 차감하지 않는다. 회수/면제 정책은 미정.",
  "내부 스탬프/챌린지 혜택의 원장 효과 writer는 CP-BENEFITS 하나다. CP-TRAVEL의 assessment는 입력이며 현재 원천·규칙·소유권 검증을 생략하는 명령이 아니다."
]
```

**repairRules**

```json
[
  "reorg로 목표 적립량 1→0이면 correction -1; 재확정 0→1이면 correction +1. stable entitlement identity 유지, 새 grant 중복 생성 금지.",
  "claim과 reorg/환불이 겹치면 현재 eligibility/owner/target를 잠금·revision으로 재검사하고 unclaimed 또는 materialized 정확히 한 경로에만 반영.",
  "실제 혜택 사용은 projection 최신성만 믿지 않고 현재 source/eligibility gate를 직렬화해 확인; 원천 무효화 후 소비자 지연 동안 사용 불가.",
  "BENEFIT-APPLY와 typed effectKey로 구매/챌린지 entitlement를 구분한다. unknown target은 차감 0이 아니라 보류 상태이며 과거 target/consume 이력을 보존한다."
]
```

**apiRefs**

```json
[
  "API-044",
  "API-104",
  "API-105",
  "API-106"
]
```

**forbidden**

```json
[
  "generation 교체로 중복 적립 금지",
  "이미 소비한 상품 자동 취소 금지",
  "정책 미선정 상태에서 부분환불 비율 임의 적용 금지"
]
```

**status**: design_proposed_not_implemented

### CP-RECEIPTS

**title**: 영수증

**consumer**: receipts

**eventTypes**

```json
[
  "payment.acceptance.changed",
  "refund.state.changed"
]
```

**projectionKey**: receiptId/eligibilityId/allocationId

**anchors**

```json
[
  "ReceiptEligibility",
  "ReceiptOwnership",
  "ReceiptClaim"
]
```

**targetRules**

```json
[
  "원영수증 발행 시각·지급자 snapshot·claim 이력과 현재 payment/refund validity를 분리한다. chain reorg로 원소유 기록을 다른 계정에 재할당하지 않는다.",
  "같은 주문이라도 현재 소유/eligibility로 허용된 allocation과 그 환불만 표시한다. order.account_id를 claim으로 덮어쓰지 않는다.",
  "결제 유효성 재확인과 소유권 revision은 별개다. receipt data revision/freshness와 ownershipRevision을 구분해야 한다."
]
```

**repairRules**

```json
[
  "영수증을 삭제 후 재발급하는 대신 동일 receipt identity에서 validity/환불 projection을 갱신한다. claim eligibility의 현재 사용 가능성은 원권한 엔진이 재판정한다.",
  "API-042/038과 SSE 전달 시 현재 ACL/삭제 상태를 재검사. 이전 snapshot이 존재해도 회수된 접근권을 복원하지 않는다.",
  "ownership 연결은 보기/혜택 materialization의 추가 trigger; claim 자체가 새 지급/매출/스탬프 원천은 아니다."
]
```

**apiRefs**

```json
[
  "API-042",
  "API-038",
  "API-104",
  "API-105",
  "API-106"
]
```

**forbidden**

```json
[
  "주문 단위 공유로 타 payer 환불 노출 금지",
  "receipt rebuild로 revoke된 소유권 복원 금지",
  "보호 proof/민감 payer 자료를 고객 이벤트에 복사 금지"
]
```

**status**: design_proposed_not_implemented

### CP-TRAVEL

**title**: 여행·챌린지

**consumer**: travel

**eventTypes**

```json
[
  "payment.acceptance.changed",
  "refund.state.changed"
]
```

**projectionKey**: travelEvidenceId/sourceType/sourceRef

**anchors**

```json
[
  "travel_evidence",
  "reviews",
  "challenge_entries",
  "participations",
  "itineraries"
]
```

**targetRules**

```json
[
  "location_visit/user_report와 testnet_payment/commercial_purchase는 서로 다른 증거다. 결제 reorg는 결제 증거와 이에 의존한 badge/challenge만 재판정한다. 실제 방문 위치·사용자 작성 후기를 자동 삭제하지 않는다.",
  "testnet dummy 지급은 commercial_purchase로 승격하지 않으며 추천/통계에 테스트넷 출처를 보존한다.",
  "환불이 실제 방문을 부정하지 않는다. 구매 인증/혜택 자격을 계속 인정할지는 저장된 여행/챌린지 ruleVersion으로 판정, 미선정이면 해당 badge/reward 보류.",
  "AI가 생성한 코스와 사용자 작성 데이터는 자동 덮어쓰기 대상이 아니다. 영향을 받은 evidence dependency를 stale/검증대기로 표시하고 현재 동의가 있을 때만 재생성 제안."
]
```

**repairRules**

```json
[
  "payment/refund/consent/삭제 변경을 받은 경우 관련 evidence 현재 validity와 revision을 계산, downstream travel.evidence.changed를 outbox로 발행.",
  "챌린지 단계는 의존 evidence revision을 다시 확인하고 원 entitlement의 assessment를 outbox로 전달한다. CP-TRAVEL은 benefit_entries/grant/correction/consume을 직접 쓰지 않는다. CP-BENEFITS가 BENEFIT-APPLY로 현재 목표를 재검증하여 같은 effectKey를 보정한다. 재구축 때 새 entitlement/reward 지급 금지.",
  "삭제 tombstone/동의 철회는 오래된 payment event보다 우선한다. 캐시/검색/추천 재생성도 같은 publication gate를 통과해야 한다."
]
```

**apiRefs**

```json
[
  "API-082"
]
```

**forbidden**

```json
[
  "결제 무효화로 모든 방문 이력 삭제 금지",
  "테스트 결제를 상업 구매로 변환 금지",
  "AI 재생성 자동 실행/동의 복원 금지",
  "CP-BENEFITS와 같은 혜택 원장을 별도 consumer key로 중복 갱신 금지"
]
```

**status**: design_proposed_not_implemented

## resources

### CP-R01

**name**: SourceManifest

**key**: manifestId/version

**fields**

```json
[
  "scope: {storeId, environment, chainId, assetId, period?, timezone?, periodPolicyVersion?}",
  "sourceVector: [{authority, authorityEpoch, resourceType, resourceId, revision}]",
  "completenessProof: {providerId, snapshotId, collectionMembershipRevisions, complete, count, contentDigest}",
  "policyVersions",
  "ownership/privacyRevisions",
  "contentDigest",
  "createdAt"
]
```

**invariant**: immutable snapshot + exact member listing/tombstones or same-DB snapshot boundary; missing refund row is not zero. Duplicate vector members rejected; partial/paged union without stable cut is incomplete. Provider identity/authentication and scope must be verified; a caller-supplied complete=true/hash is not proof. Every vector member carries authorityEpoch, so restored/recreated authority cannot reuse old revision numbers unnoticed.

**physicalMapping**: logical_adapter_proposal_no_SQL

**runtimeVerified**: False

### CP-R02

**name**: ConsumerCheckpoint

**key**: consumer/projectionKey/generation

**fields**

```json
[
  "activeGeneration",
  "fence",
  "sourceManifestId",
  "appliedProjectionRevision",
  "status",
  "watermark",
  "lastErrorCode"
]
```

**invariant**: one active generation per projection; fence CAS + current source/privacy guard recheck. Broker offset is transport progress, not business currentness.

**physicalMapping**: logical_adapter_proposal_no_SQL

**runtimeVerified**: False

### CP-R03

**name**: SourceContribution

**key**: consumer/projectionKey/sourceIdentity

**fields**

```json
[
  "targetVersion",
  "sourceManifestId",
  "metricAmounts",
  "assetId",
  "policyVersion",
  "contentDigest",
  "revision"
]
```

**invariant**: unique source identity independent of rebuild generation; immutable version history + current pointer; newTarget-oldTarget is signed delta. Store old value even if target becomes zero.

**physicalMapping**: logical_adapter_proposal_no_SQL

**runtimeVerified**: False

### CP-R04

**name**: ConsumerApplication

**key**: consumer/aggregateType/aggregateId/aggregateRevision

**fields**

```json
[
  "eventIds",
  "normalizedContentDigest",
  "status",
  "manifestId",
  "projectionRevision",
  "appliedAt"
]
```

**invariant**: logical unique key excludes digest; same revision different digest quarantines. Event receipt can be durable pending; applied marker commits only with actual target change/checkpoint/outbox.

**physicalMapping**: logical_adapter_proposal_no_SQL

**runtimeVerified**: False

### CP-R05

**name**: CorrectionRecord

**key**: consumer/projectionKey/targetManifestId/correctionKind

**fields**

```json
[
  "previousRevision",
  "newRevision",
  "signedDeltaOrStateChange",
  "reason",
  "sourceRefs",
  "policyVersion",
  "operatorRef?",
  "createdAt"
]
```

**invariant**: append-only effect identity independent of eventId/generation. Financial/effect ledger unique key remains across rebuild, while derived read views may be recreated.

**physicalMapping**: logical_adapter_proposal_no_SQL

**runtimeVerified**: False

### CP-R06

**name**: RepairRun

**key**: runId

**fields**

```json
[
  "scope",
  "reason",
  "dryRun",
  "currentGeneration",
  "candidateGeneration",
  "sourceManifestId",
  "diffDigest",
  "fence",
  "status",
  "unresolvedSources",
  "publicationDecision",
  "checkpoint"
]
```

**invariant**: states requested→capturing→building→diff_ready→publishing→completed; any failure→held/quarantined. Completion is not permission to perform external effects.

**physicalMapping**: logical_adapter_proposal_no_SQL

**runtimeVerified**: False

## rebuildSteps

### CP-B01

**title**: 원인·범위 고정

**rule**: 원천 누락/버전 전환/백업/계산 오류를 기록하고 exact-store 운영권한·projection 범위를 고정; 외부 지급/보상 실행 권한과 분리.

**dependsOn**

```json
[]
```

### CP-B02

**title**: 권위 snapshot 확보

**rule**: 완전성·source vector·policy·privacy/ownership revision을 갖는 불변 manifest 확보. refund 목록 일부 누락/unknown hold이면 중단.

**dependsOn**

```json
[
  "CP-B01"
]
```

### CP-B03

**title**: 후보 generation 계산

**rule**: 원본 증거와 원규칙으로 shadow 계산. live writer는 유지해도 외부효과/효력있는 적립·사용·정산마감은 shadow에서 금지.

**dependsOn**

```json
[
  "CP-B02"
]
```

### CP-B04

**title**: 차이 검토

**rule**: 기존 기여분 대비 signed delta, 누락/추가 evidence, 이미 사용한 보상, closed period 영향·삭제 충돌 목록 생성. 정책 미정은 held.

**dependsOn**

```json
[
  "CP-B03"
]
```

### CP-B05

**title**: 최신성 재확인

**rule**: 현재 gate/vector와 후보 manifest를 대조해 전환 준비만 한다. source/소유권/삭제 변경 시 recapture. 이 단계는 active pointer/fence를 바꾸거나 공개 권한을 만들지 않는다.

**dependsOn**

```json
[
  "CP-B04"
]
```

### CP-B06

**title**: 보정·발행

**rule**: publication transaction 안에서 최신 source/privacy/fence를 다시 확인하고 active generation pointer·새 fence·기여분·보정·application·checkpoint·outbox를 함께 commit한다. 불변 shadow view는 미리 작성 가능하지만 pointer 전환은 마지막 commit에만 포함한다. immutable ledger는 동일 논리 effect key의 보정만 append하며 외부 알림은 commit 이후 현재 권한으로 전달한다. 큰 rebuild 분할이 필요하면 publication held/resume protocol을 별도 설계하기 전 공개를 차단한다.

**dependsOn**

```json
[
  "CP-B05"
]
```

### CP-B07

**title**: 증거·복구 종료

**rule**: before/after manifest/diff/effect key/결과 checkpoint 보관. rollback은 검증된 view/fence만 전환하며 이미 발행한 원장 보정/권한 철회/삭제를 되돌리지 않음.

**dependsOn**

```json
[
  "CP-B06"
]
```

## runtimeCases

### CP-T01

**scenario**: 중복 eventId

**expected**: 동일 consumer에서 pending 또는 applied 결과 반환; 금액·혜택 추가 반영 없음.

**status**: not_run

### CP-T02

**scenario**: v1/v2의 다른 eventId·같은 원천 revision

**expected**: 동일 논리 application 한 번; 다른 내용은 격리.

**status**: not_run

### CP-T03

**scenario**: 누락·역순 이벤트

**expected**: 부분 delta 덧셈 금지; 완전한 snapshot 기반 목표로 재계산.

**status**: not_run

### CP-T04

**scenario**: 동일 revision의 다른 digest

**expected**: 한쪽을 자동 최신으로 선택하지 않고 source 재검증·격리.

**status**: not_run

### CP-T05

**scenario**: 원입금 reorg 후 환불만 유효

**expected**: 영업 이력 보존; 현재 입금0/환불6이면 cash net -6 및 차이 보류; 0으로 clamp 금지.

**status**: not_run

### CP-T06

**scenario**: 예외 지급 수취와 반환

**expected**: 실수취 대조에 포함하지만 정상 매출/매출 환불로 이중 분류하지 않음.

**status**: not_run

### CP-T07

**scenario**: 원천 snapshot에서 refund 페이지 누락

**expected**: complete=false 처리; 누락 환불0으로 계산 금지.

**status**: not_run

### CP-T08

**scenario**: 마감 후 원지급 재확인

**expected**: 원마감 snapshot 보존; 원기간 correction_pending 및 새 manifest. 신규 자금 지급 없음.

**status**: not_run

### CP-T09

**scenario**: 동시 마감·보정

**expected**: settlement revision/fence CAS 한 경로만 성공; stale 결과 다시 조회.

**status**: not_run

### CP-T10

**scenario**: 다중 자산·테스트넷 혼재

**expected**: chain/environment/asset별 분리; decimals 동일해도 단순합산 금지.

**status**: not_run

### CP-T11

**scenario**: guest claim과 reorg 경합

**expected**: 실제 owner와 현재 target를 함께 검사; pending/materialized 한 경로만 보정.

**status**: not_run

### CP-T12

**scenario**: 사용한 스탬프의 원지급 무효화

**expected**: consume 이력 유지; 보정 후 deficit와 추가사용 보류, 자동 금전청구 없음.

**status**: not_run

### CP-T13

**scenario**: 동일 지급 재확정

**expected**: original entitlement target 복구 보정 한 번; 새 grant/새 reward_key 생성 금지.

**status**: not_run

### CP-T14

**scenario**: 원천 차단 직후 혜택 사용

**expected**: consumer 지연과 무관하게 현재 eligibility/source gate에서 거절.

**status**: not_run

### CP-T15

**scenario**: 영수증 소유권 회수 후 과거 snapshot 재생

**expected**: 현재 ACL이 거절; 이전 snapshot으로 소유권/공개 복원 금지.

**status**: not_run

### CP-T16

**scenario**: 결제 무효화와 독립 위치 방문

**expected**: 결제 badge만 재판정; 위치 방문·사용자 후기는 보존하며 규칙/동의에 따라 표기.

**status**: not_run

### CP-T17

**scenario**: 부분환불과 미선정 여행 규칙

**expected**: 방문은 자동 삭제하지 않고 해당 구매 badge/reward를 policy_pending 처리.

**status**: not_run

### CP-T18

**scenario**: 사용자 삭제 뒤 오래된 이벤트

**expected**: tombstone 우선, 공개 view/AI입력/보상 수신자 자동 복원 금지.

**status**: not_run

### CP-T19

**scenario**: 재구축 중 신규 환불 도착

**expected**: publication 전 vector/목록revision 불일치로 recapture; 과거 target로 덮어쓰기 금지.

**status**: not_run

### CP-T20

**scenario**: commit 성공 후 worker/응답 소실

**expected**: checkpoint/effect key로 완료를 확인; 다시 consume/grant/보정하지 않음.

**status**: not_run

### CP-T21

**scenario**: 구 generation worker 생존

**expected**: stale fence DB commit 거절; 이미 허가된 외부효과는 추적하고 취소 완료로 간주하지 않음.

**status**: not_run

### CP-T22

**scenario**: 복구 snapshot에 최근 삭제/철회 누락

**expected**: 최신 deny 권위를 복원할 때까지 publication/신규 사용 보류; 미검증 backup으로 재공개 금지.

**status**: not_run

## subscriptionDeltas

### subscriptionDeltas

**event**: payment.acceptance.changed

**addConsumers**

```json
[
  "settlements"
]
```

**existingConsumers**

```json
[
  "sales",
  "benefits",
  "receipts",
  "travel"
]
```

**status**: catalog_change_proposed_not_applied

### subscriptionDeltas

**event**: refund.state.changed

**addConsumers**

```json
[
  "settlements",
  "travel"
]
```

**existingConsumers**

```json
[
  "sales",
  "benefits",
  "receipts"
]
```

**status**: catalog_change_proposed_not_applied

## auxiliaryTriggers

### auxiliaryTriggers

**trigger**: ReceiptOwnership commit

**consumers**

```json
[
  "receipts",
  "benefits",
  "travel"
]
```

**rule**: source transaction durable outbox/reconciliation work item; stable eligibility identity; original claim auth unchanged

**catalogStatus**: explicit_adapter_contract_needed_not_registered_event

### auxiliaryTriggers

**trigger**: benefit redemption commit

**consumers**

```json
[
  "benefits"
]
```

**rule**: existing benefit.ledger.changed plus current redemption/source gates; immutable consume contribution

### auxiliaryTriggers

**trigger**: privacy.request.changed / consent authority change

**consumers**

```json
[
  "receipts",
  "travel"
]
```

**rule**: latest deny/tombstone blocks read/publication; causal event lag cannot temporarily restore access

### auxiliaryTriggers

**trigger**: settlement close/correction command

**consumers**

```json
[
  "settlements"
]
```

**rule**: current store permission and manifest/fence CAS; actor decision audited

## atomicApply

- Persist received event as pending with eventId dedup; do not ACK semantic application before verifying version/source scope. Transport ACK after durable pending is separate from applied state.
- Resolve authoritative snapshot manifest; cross-resource reads need same source transaction cut or an immutable complete manifest protocol. Unknown or incomparable vectors -> hold/recapture, never max(revisions).
- Compute target from current stored policy; lock current shared source/eligibility/privacy gates, then consumer checkpoint and effect/contribution keys in stable order. Check captured versions and completeness again.
- Commit target contribution, signed correction or view update, application marker, checkpoint/projection revision and downstream outbox atomically within consumer store. Cross-store effects use durable deduped effect intent; no distributed atomicity assumed.
- External messages/device sync/search/AI are outbox deliveries with their own effect identity and current publication authorization. A stale worker fence only prevents DB writes, not previously permitted remote effects.
## pendingPolicies

- D08 최초수락/부분지급/예외반환 정책 미정
- 이미 소비한 혜택·부분환불·챌린지 보상 보정 정책과 규칙 버전 미정
- 정산 기간별 attribution/마감 요건/보정 승인 규칙 제안 상태
- D02 source manifest provider·일관성 모델·물리 저장/메시지 브로커 미선정
- D19 지연/격리 경보 임계값·운영 보관 기간 미정
- RR-DEC-01 변경 없음
## nextDesign

매출·정산 조회/마감/보정 API의 freshness·manifest·음수 차이 응답과 운영 재처리 명령 계약

## sourceCutContract

```json
{
  "required": [
    "verifiedProviderIdentity",
    "exactScope",
    "authorityEpochs",
    "consistentSnapshotId",
    "collectionMembershipRevisions",
    "completeMemberSetOrVerifiablePagination",
    "sourceVector",
    "policyVersions",
    "currentDenyAuthority"
  ],
  "rules": [
    "Immutable vector is not automatically a consistent cut. Provider must attest to same-DB snapshot or a documented cross-service cut protocol; mixed unsynchronized reads stay incomplete.",
    "Concurrent additions/removals require collection membership revision/tombstones, including refund lists and ownership/consent dependencies. Count/digest alone cannot prove no omission.",
    "An unavailable latest deny/epoch authority blocks publication and current use. Rebuild cannot infer authority from old snapshot or broker offset."
  ]
}
```

## publicationAtomicUnit

```json
{
  "preparationStep": "CP-B05",
  "commitStep": "CP-B06",
  "preparationMayPublish": false,
  "finalRecheck": [
    "sourceVector",
    "authorityEpochs",
    "privacyOwnership",
    "consumerFence",
    "scopeCompleteness"
  ],
  "sameCommit": [
    "activeGenerationPointer",
    "newFence",
    "sourceContributions",
    "correctionRecords",
    "consumerApplications",
    "checkpoint",
    "outbox"
  ],
  "crossStoreAtomicityAssumed": false,
  "splitPublicationProtocol": "not_designed; block until specified"
}
```

## benefitEffectOwnership

```json
{
  "status": "logical_candidate_not_adopted",
  "scope": "internal_nontransferable_stamp_or_benefit_ledger_only",
  "assessmentWriter": "CP-TRAVEL",
  "effectWriter": "CP-BENEFITS",
  "transportAuthority": false,
  "effectKeyFields": [
    "environmentId",
    "benefitProgramId",
    "entitlementSourceKind",
    "entitlementSourceId"
  ],
  "excludedFromEffectKey": [
    "consumerGeneration",
    "eventId",
    "sourceVector",
    "assessmentRevision",
    "claimAccountId",
    "mutablePolicyRevision"
  ],
  "sourceVariants": [
    {
      "id": "BE-PURCHASE",
      "kind": "purchase_eligibility",
      "sourceId": "original ReceiptEligibility.eligibilityId",
      "ownerRule": "원 eligibility와 현재 ReceiptOwnership. guest는 pending entitlement, 임의 account 생성 금지."
    },
    {
      "id": "BE-CHALLENGE",
      "kind": "challenge_entitlement",
      "sourceId": "등록 challengeVersion + 원 participantRef + rewardSlotId + occurrenceId의 불변 entitlementId",
      "ownerRule": "검증된 원 participant의 현재 권한. purchase eligibility를 가장하거나 paymentId를 합성하지 않는다. 반복 보상 occurrence는 선택된 원 규칙으로 확정하며 이벤트 ID로 생성하지 않는다."
    }
  ],
  "ruleBinding": "원 entitlement에 선정 ruleVersion/termsDigest를 불변 고정한다. 마케팅 규칙 변경은 기존 sourceId/효과를 새 grant로 재발급하는 근거가 아니다. 명시 migration 정책이 없다면 변경 적용 보류.",
  "programSeparation": "한 지급에 구매 스탬프와 챌린지 혜택이 함께 적용되는 것은 선택된 서로 다른 program의 독립 효과다. 두 program을 한 paymentId로 전역 중복 제거하지 않는다. 미선정 stacking/반복/부분환불 규칙은 held.",
  "effectRule": "현재 권위 원천과 완전한 source cut을 재검증해 targetEarned를 산출한다. 원 효과의 이전 target과 차이만 같은 원장 revision CAS에 기록하며 consume 이력은 유지한다.",
  "unknownTarget": "원천/규칙/소유권 불명은 assessment/effect held. 불명을 target=0으로 바꿔 차감하지 않는다. 현 source gate는 혜택 사용을 차단하고 과거 원장 이력은 유지한다.",
  "nonLocalRewards": "온체인 토큰/현금성/양도 가능한 보상 실행은 이 내부 원장 권한이 아니다. 별도 등록 typed payout adapter·자금/서명 권한·노출 대사 계약 없으면 실행 불가.",
  "crossStore": "CP-TRAVEL의 assessment outbox와 CP-BENEFITS 적용은 한 DB 트랜잭션이라고 가정하지 않는다. durable inbox/dedup 뒤 현재 원천 재조회·원장 CAS로 적용한다. 수신 ACK는 지급/적립 완료 아님."
}
```

## benefitHandoffPredicates

### BENEFIT-APPLY

**allOf**

```json
[
  "registered_internal_program",
  "typed_source_identity_valid",
  "original_rule_binding_current",
  "source_cut_complete_and_current",
  "owner_or_pending_eligibility_current",
  "privacy_and_source_use_allowed",
  "assessment_not_stale_or_conflicting",
  "writer_is_benefits",
  "expected_effect_revision_matches"
]
```

**effect**: 동일 effectKey에서 target delta·consumer application·checkpoint·outbox를 원자 반영. 이벤트 transport 권한은 적립/사용 권한이 아니다.

## benefitAssessmentContract

```json
{
  "identity": [
    "environmentId",
    "benefitProgramId",
    "entitlementSourceKind",
    "entitlementSourceId",
    "assessmentRevision"
  ],
  "fields": [
    "assessmentDigest",
    "sourceManifestRef",
    "sourceVector",
    "ruleVersion",
    "termsDigest",
    "ownerBindingRevision",
    "targetEarned",
    "targetStatus",
    "reasonRefs"
  ],
  "ordering": "assessmentRevision은 해당 entitlement의 권위 평가 세대다. 낮은 revision은 현재 원천 재조회 대상으로만 사용한다. 동일 revision의 다른 digest는 quarantine. 높은 revision도 source authority/현재 gate 검사 없이 적용하지 않는다.",
  "projection": "travel/challenge completion 상태와 reward applied/held/deficit 상태는 별도. UI는 효과 원장의 현재 revision이 확인되기 전 보상 지급 완료로 표시하지 않는다.",
  "catalogStatus": "logical_outbox_inbox_adapter_not_registered_event",
  "predicateRef": "BENEFIT-APPLY"
}
```


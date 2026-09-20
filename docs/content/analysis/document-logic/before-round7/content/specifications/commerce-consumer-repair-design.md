# 매출·정산·혜택·영수증·여행: 보정과 재처리 계약

2026-09-18 · **상세 설계 제안. 제품 구현·기준 API/event 병합·SQL 적용·실행 시험은 하지 않았다.**

[결제·환불 대사 연결안](commerce-reconciliation-integration.md)의 후속으로 **소비자 5개, 논리 자원 6개, 재구축 7단계, 실행 수용 사례 22개**를 정리했다. 이벤트가 도착한 만큼 금액을 더하는 방식 대신, 검증한 현재 원천에서 목표 값을 계산하고 이미 반영한 기여분과의 차이만 적용한다.

[구조 원본](commerce-consumer-repair-design.json) · [합성 예제](commerce-consumer-repair-examples.json) · [문서 검사기](validate_commerce_consumer_repair.py) · [승인 gate·잠금·장애 복구 기준](approval-physical-storage-design.md)

## 1. 다섯 소비자의 책임

| 소비자 | 보존할 이력 | 현재 상태에서 바꿀 것 | 자동 실행하지 않을 것 |
|---|---|---|---|
| 매출 | 원주문·주수락·예외 지급의 귀속 | 지급별 매출·수취·환불 기여분과 차이 표시 | 새 판매/환불·다른 자산 합산 |
| 정산 | 마감 당시 snapshot/version | 보정 대기와 새 보정 version | 원마감 덮어쓰기·자동 송금/환전 |
| 혜택 | 적립·소비·claim 이력 | 원규칙의 목표 적립량 차이 | 재구축마다 재적립·이미 쓴 혜택 자동 청구 |
| 영수증 | 발행 identity·원소유 증거 | 현재 지급/환불 유효성 | 다른 계정에 소유권 재배정 |
| 여행 | 위치 방문·사용자 후기·코스 | 결제 인증·의존 badge/challenge 유효성 | 방문 삭제·AI 자동 재생성·삭제 데이터 부활 |

이 설계에서 ‘정산’은 **가맹점이 받은 코인의 대조와 마감 기록**이다. 사용자 요청대로 가맹점이 스테이블코인을 수취하며, 자동 원화 환전이나 플랫폼의 가맹점 지급 업무를 추가하지 않는다. 아래 합계는 제품의 대사 표시 제안이며 법정 회계·세무 처리를 확정하는 문서가 아니다.

## 2. 원천 snapshot과 최신성 계약

이벤트는 원천 변경을 알려주는 입력이다. event에 없는 환불·소유권·동의 상태를 0/미존재로 추측하지 않는다. 여러 원천이 필요하면 권위 있는 provider가 만든 SourceManifest로 읽는다.

| 필수 정보 | 역할 |
|---|---|
| scope | store/environment/chain/asset, 필요한 period/timezone/policy 범위 |
| sourceVector | authority·authorityEpoch·resourceType·resourceId·revision의 고유 목록 |
| snapshotId | 같은 DB snapshot 또는 명시된 서비스 간 일관된 읽기 경계 |
| collectionMembershipRevisions | 환불 목록 등 신규/삭제 구성원 변경을 감지하는 revision |
| completenessProof | 인증한 provider, 완전한 목록/안정된 pagination, count/digest, 누락 여부 |
| policyVersions | 원주문/혜택/정산/챌린지 판정에 사용한 고정 규칙 |
| ownership/privacyRevisions | 현재 소유권·동의·삭제/거부 권위의 비교점 |

`complete=true`나 hash가 있다는 것만으로 완전성을 인정하지 않는다. 검증된 provider와 일관된 snapshot의 목록·소속을 확인해야 한다. 서로 다른 시점에 읽은 allocation/refund를 임의로 합쳐 불변 JSON으로 만들더라도 일관된 snapshot이 되지 않는다. 외부 서비스가 일관된 읽기 경계를 제공하지 못하면 incomplete로 보류하며, 분산 transaction을 가정하지 않는다.

vector는 각 권위 안에서만 비교한다. allocation revision 4와 refund revision 7 중 큰 숫자 하나를 현재 revision으로 쓰지 않는다. 새 refund가 추가됐는지는 refundSetRevision으로 확인한다. 백업 복원 등으로 숫자가 재사용될 수 있으므로 authorityEpoch도 비교한다. transport offset/블록 높이는 업무 원장·동의 최신성의 대체값이 아니다.

공개 직전 현재 gate·source vector·소유권·privacy를 다시 검사한다. 최신 deny 권위를 읽지 못하면 공개와 신규 혜택 사용을 보류한다. 백업의 과거 접근권을 복원 근거로 삼지 않는다. 업무 승인 gate와 별도로 느리게 복사한 권한 캐시를 권위로 사용하지 않는다.

## 3. 목표 기여분과 signed 보정

같은 `(consumer, projectionKey, sourceIdentity)`의 이전 목표와 새 목표를 비교한다. 차이를 signed 정수로 기록하되 원액은 자산별 atomic 정수다. 소수점/환율로 임의 변환하거나 `float`로 계산하지 않는다. 재구축 generation은 원지급·entitlement·effect identity를 바꾸지 않는다.

### 매출과 수취 대조를 분리

- `grossAcceptedAtomic`: 현재 유효한 주수락 지급의 매출 기여분.
- `confirmedSalesRefundAtomic`: 원래 일반 매출 지급에 귀속된 확정 환불. 원입금이 무효화돼도 이미 나간 환불을 지우지 않는다.
- `canonicalAttributedReceiptsAtomic`: 유효하게 확인된 귀속 수취. 예외 지급도 포함할 수 있으나 정상 매출로 자동 분류하지 않는다.
- `allCanonicalRefundOutflowAtomic`: 같은 범위·자산에 귀속된 모든 유효 환불 지출. 예외 지급 반환도 포함한다.
- `netSalesAtomic = grossAcceptedAtomic − confirmedSalesRefundAtomic`.
- `netAttributedCashAtomic = canonicalAttributedReceiptsAtomic − allCanonicalRefundOutflowAtomic`.

원입금이 무효화된 후 환불만 유효한 경우 net이 음수일 수 있다. 이를 0으로 숨기지 않고 대사 차이/검토 필요로 표시한다. 이 값은 확정 회계 매출이나 지갑 전체 잔액을 의미하지 않는다. 특히 지갑의 주문 외 입금·이체·가스·DeFi 포지션을 포함한 전체 잔액 대조는 별도 범위다.

| 예 | 유효 매출 | 매출 환불 | 귀속 수취 | 모든 환불 지출 | 매출 net | 귀속 cash net |
|---|---:|---:|---:|---:|---:|---:|
| 매출 10, 환불 2 | 10 | 2 | 10 | 2 | 8 | 8 |
| 원입금 reorg, 환불 6 유지 | 0 | 6 | 0 | 6 | -6 | -6 |
| 같은 원입금 재확정 | 10 | 6 | 10 | 6 | 4 | 4 |
| 예외 입금 5를 전액 반환 | 0 | 0 | 5 | 5 | 0 | 0 |

위 예는 동일 asset/environment의 합성 atomic 단위다. KRW 주문 가격과 token 수량을 섞지 않는다. 현재 DTO `SalesTotals`의 UInt 필드에 음수를 넣거나 기존 receivedAtomic을 몰래 net으로 재정의하지 않는다. **API-039/040/041/093의 명시적 응답 변경이 후속 작업**이다.

## 4. 소비자별 처리 규칙

### CP-SALES · 매출

원천 연결: `orders`, `payment_allocations`, `payment_evidence`, `refund_balances`, `refunds`, `refund_reservations`

기여분 키: `storeId/assetId/paymentId`

**목표 계산**

- 유효 주수락 지급의 grossAcceptedAtomic, 일반 매출 지급에 연결된 confirmedSalesRefundAtomic, 예외 지급 수취/반환을 각각 분리한다.
- 수취액과 매출을 혼용하지 않는다. canonicalAttributedReceiptsAtomic에는 확인된 예외 수취도 들어갈 수 있지만 grossAcceptedAtomic에는 포함하지 않는다.
- netSalesAtomic=grossAcceptedAtomic-confirmedSalesRefundAtomic; netAttributedCashAtomic=canonicalAttributedReceiptsAtomic-allCanonicalRefundOutflowAtomic. 두 net은 signed integer로 표현하며 음수/미해결 차이를 0으로 숨기지 않는다.
- 기존 SalesTotals는 모든 필드가 UInt이므로 새 net/유효성/진행 정보는 후속 DTO 변경 대상이다. receivedAtomic의 의미를 새 net으로 몰래 바꾸지 않는다.

**보정·복구**

- 현재 원천 snapshot에서 지급별 목표 기여분을 계산하고 이전 목표와의 차이만 한 번 반영. 주수락 이력은 원환불의 sales/exception 분류를 유지한다.
- source reorg 후 outgoing refund만 canonical이면 음수 차이와 review_required로 표시; 새로운 판매/환불을 자동 생성하지 않음.
- 자산·chain·환경별 독립 집계. 주문 KRW 가격은 이력 표시이며 token 수량과 합산/자동 환전하지 않음.

### CP-SETTLEMENT · 정산

원천 연결: `settlements`, `settlement_revisions`

기여분 키: `storeId/period/assetId/settlementId`

**목표 계산**

- 정산은 가맹점 코인 수취·환불 대조 기록이며 자동 지급/원화 전환 작업이 아니다.
- 마감은 완전성 검증된 SourceManifest와 계산된 합계를 고정한다. settlement_revisions.source_revision은 manifest의 로컬 버전으로 연결할 제안이며 모든 원천 revision의 max가 아니다.
- 마감 후 보정은 이전 version과 새 manifest를 가리키는 새 settlement revision. 원마감 totals_snapshot은 불변.
- periodStart<=businessEffectiveAt<periodEnd, 매장 timezone과 period 정책 버전 고정. event 수신 시각으로 날짜를 재배정하지 않는다. 최초 수락/확정의 신뢰 가능한 업무 시각이 없으면 backfill 보류.

**보정·복구**

- open 기간은 목표 projection 갱신; closed 기간은 correction_pending으로 표시하고 exact-store settlement_manage 승인 후 동일 원기간 새 version을 만든다는 제안. 자동 승인 정책은 미선정.
- 같은 target manifest 재처리는 새 보정 version을 만들지 않는다. 동시 마감/보정은 settlement revision CAS와 consumer fence 검사.
- 기간/자산별 시각 정책상 sales cohort와 cash movement period가 다를 수 있으므로 각 metric의 attribution manifest를 따로 남긴다. 다음 기간에도 동일 조정을 이중 반영하지 않는다.

### CP-BENEFITS · 혜택·스탬프

원천 연결: `benefit_entries`, `benefit_redemptions`, `ReceiptEligibility`, `ReceiptOwnership`, `PendingBenefitEntitlement`

기여분 키: `eligibilityId/ruleVersion/benefitKind`

**목표 계산**

- 지급별 목표 적립량은 저장된 ruleVersion으로 계산하며 현재 마케팅 규칙으로 과거 보상을 재발급하지 않는다. 부분환불/이미 사용한 혜택 처리 정책이 없으면 policy_pending.
- 계정 없는 guest는 PendingBenefitEntitlement에만 기록하고 가짜 계정을 만들지 않는다. 소유 연결 확정 후 같은 source identity로 계정 원장에 한 번 materialize한다.
- 원장 순혜택=sum(grant+correction+consume). consume 이력은 보정 계산에서 유지한다. 적립 목표만 바뀌므로 사용 이력을 지우거나 자동 재사용권으로 바꾸지 않는다.
- 보정 후 잔량<0이면 화면에서 deficit/review_required와 추가사용 보류로 표현하는 제안. 돈을 청구하거나 다른 지갑 자산을 차감하지 않는다. 회수/면제 정책은 미정.

**보정·복구**

- reorg로 목표 적립량 1→0이면 correction -1; 재확정 0→1이면 correction +1. stable entitlement identity 유지, 새 grant 중복 생성 금지.
- claim과 reorg/환불이 겹치면 현재 eligibility/owner/target를 잠금·revision으로 재검사하고 unclaimed 또는 materialized 정확히 한 경로에만 반영.
- 실제 혜택 사용은 projection 최신성만 믿지 않고 현재 source/eligibility gate를 직렬화해 확인; 원천 무효화 후 소비자 지연 동안 사용 불가.

### CP-RECEIPTS · 영수증

원천 연결: `ReceiptEligibility`, `ReceiptOwnership`, `ReceiptClaim`

기여분 키: `receiptId/eligibilityId/allocationId`

**목표 계산**

- 원영수증 발행 시각·지급자 snapshot·claim 이력과 현재 payment/refund validity를 분리한다. chain reorg로 원소유 기록을 다른 계정에 재할당하지 않는다.
- 같은 주문이라도 현재 소유/eligibility로 허용된 allocation과 그 환불만 표시한다. order.account_id를 claim으로 덮어쓰지 않는다.
- 결제 유효성 재확인과 소유권 revision은 별개다. receipt data revision/freshness와 ownershipRevision을 구분해야 한다.

**보정·복구**

- 영수증을 삭제 후 재발급하는 대신 동일 receipt identity에서 validity/환불 projection을 갱신한다. claim eligibility의 현재 사용 가능성은 원권한 엔진이 재판정한다.
- API-042/038과 SSE 전달 시 현재 ACL/삭제 상태를 재검사. 이전 snapshot이 존재해도 회수된 접근권을 복원하지 않는다.
- ownership 연결은 보기/혜택 materialization의 추가 trigger; claim 자체가 새 지급/매출/스탬프 원천은 아니다.

### CP-TRAVEL · 여행·챌린지

원천 연결: `travel_evidence`, `reviews`, `challenge_entries`, `participations`, `itineraries`

기여분 키: `travelEvidenceId/sourceType/sourceRef`

**목표 계산**

- location_visit/user_report와 testnet_payment/commercial_purchase는 서로 다른 증거다. 결제 reorg는 결제 증거와 이에 의존한 badge/challenge만 재판정한다. 실제 방문 위치·사용자 작성 후기를 자동 삭제하지 않는다.
- testnet dummy 지급은 commercial_purchase로 승격하지 않으며 추천/통계에 테스트넷 출처를 보존한다.
- 환불이 실제 방문을 부정하지 않는다. 구매 인증/혜택 자격을 계속 인정할지는 저장된 여행/챌린지 ruleVersion으로 판정, 미선정이면 해당 badge/reward 보류.
- AI가 생성한 코스와 사용자 작성 데이터는 자동 덮어쓰기 대상이 아니다. 영향을 받은 evidence dependency를 stale/검증대기로 표시하고 현재 동의가 있을 때만 재생성 제안.

**보정·복구**

- payment/refund/consent/삭제 변경을 받은 경우 관련 evidence 현재 validity와 revision을 계산, downstream travel.evidence.changed를 outbox로 발행.
- 챌린지 단계는 의존 evidence revision을 다시 확인한다. 이미 지급한 reward는 동일 reward_key 보정 정책으로 처리하고 재구축 때 새 reward 지급 금지.
- 삭제 tombstone/동의 철회는 오래된 payment event보다 우선한다. 캐시/검색/추천 재생성도 같은 publication gate를 통과해야 한다.

### 정산 기간과 마감 경계의 구체안

기간은 `[periodStart, periodEnd)`이며 매장 timezone을 고정한다. 일반 매출 cohort는 처음 업무 수락이 확정된 시각을 고정하고, 수취/환불 cash movement는 각 최초 업무 확정 시각을 고정하는 attribution 제안이다. 지연 도착한 이벤트의 수신 시각이나 현재 block timestamp로 과거 날짜를 옮기지 않는다. reorg 보정은 이 원귀속 기간에 연결한다. 신뢰 가능한 최초 업무 시각이 없는 과거 기록은 사실을 만들어 채우지 않고 보류한다.

기간 정산의 metric별 source manifest에 귀속 규칙을 남긴다. 같은 보정을 원기간 revision과 다음 기간 실적에 동시에 계상하지 않는다. 현재 SQL은 이 최초 시각/manifest 구조를 충분히 담지 않으므로 `totals_snapshot`의 구조와 별도 attribution 기록을 구체화해야 한다. 기간 경계·정책 선택 전 실제 마감을 허용했다고 간주하지 않는다.

마감 전에는 해당 source 범위의 모든 필요한 소비 작업이 기준 manifest까지 반영됐는지 확인한다. 전체 시스템이 영원히 멈춘 상태를 요구하는 것이 아니라 **그 마감의 명시적 cut**을 고정하고 이후 변경은 보정으로 다룬다. 알려진 누락·hold·불완전 manifest가 있으면 정상 마감 성공으로 표시하지 않는다.

### 사용한 혜택의 보정 예

원 적립 1, 소비 1이면 잔량은 0이다. 원입금 무효화로 목표 적립이 0이 되면 보정 -1을 추가해 원장 잔량 -1을 보존한다. 화면에는 미사용 수량을 음수 스탬프로 그리기보다 부족/검토 필요와 사용 보류를 구분해 보여주는 제안이다. 이것이 금전 채무를 만든다는 뜻은 아니다. 같은 지급이 재확정되면 +1 보정으로 잔량 0이 되며, 소비한 상품을 다시 지급하지 않는다.

부족 사유가 해소돼도 분실·권한·다른 원천의 독립 hold를 해제하지 않는다. `expectedDeficitHold` 예제는 부족 사유 하나만 검사한다. 부분환불과 이미 사용한 보상 회수·면제는 원 ruleVersion으로 결정해야 하며 미정 상태에서 임의 비율을 적용하지 않는다.

## 5. 저장·원자 적용 계약

새 물리 SQL 테이블 수를 확정하지 않고 다음 여섯 논리 adapter를 제안한다. 기존 inbox/outbox, settlement_revisions, benefit_entries 등과 연결하되 새 제약을 이미 실행했다고 주장하지 않는다.

### CP-R01 · SourceManifest

키: `manifestId/version`

필드:

- `scope: {storeId, environment, chainId, assetId, period?, timezone?, periodPolicyVersion?}`
- `sourceVector: [{authority, authorityEpoch, resourceType, resourceId, revision}]`
- `completenessProof: {providerId, snapshotId, collectionMembershipRevisions, complete, count, contentDigest}`
- `policyVersions`
- `ownership/privacyRevisions`
- `contentDigest`
- `createdAt`

불변식: immutable snapshot + exact member listing/tombstones or same-DB snapshot boundary; missing refund row is not zero. Duplicate vector members rejected; partial/paged union without stable cut is incomplete. Provider identity/authentication and scope must be verified; a caller-supplied complete=true/hash is not proof. Every vector member carries authorityEpoch, so restored/recreated authority cannot reuse old revision numbers unnoticed.

### CP-R02 · ConsumerCheckpoint

키: `consumer/projectionKey/generation`

필드:

- `activeGeneration`
- `fence`
- `sourceManifestId`
- `appliedProjectionRevision`
- `status`
- `watermark`
- `lastErrorCode`

불변식: one active generation per projection; fence CAS + current source/privacy guard recheck. Broker offset is transport progress, not business currentness.

### CP-R03 · SourceContribution

키: `consumer/projectionKey/sourceIdentity`

필드:

- `targetVersion`
- `sourceManifestId`
- `metricAmounts`
- `assetId`
- `policyVersion`
- `contentDigest`
- `revision`

불변식: unique source identity independent of rebuild generation; immutable version history + current pointer; newTarget-oldTarget is signed delta. Store old value even if target becomes zero.

### CP-R04 · ConsumerApplication

키: `consumer/aggregateType/aggregateId/aggregateRevision`

필드:

- `eventIds`
- `normalizedContentDigest`
- `status`
- `manifestId`
- `projectionRevision`
- `appliedAt`

불변식: logical unique key excludes digest; same revision different digest quarantines. Event receipt can be durable pending; applied marker commits only with actual target change/checkpoint/outbox.

### CP-R05 · CorrectionRecord

키: `consumer/projectionKey/targetManifestId/correctionKind`

필드:

- `previousRevision`
- `newRevision`
- `signedDeltaOrStateChange`
- `reason`
- `sourceRefs`
- `policyVersion`
- `operatorRef?`
- `createdAt`

불변식: append-only effect identity independent of eventId/generation. Financial/effect ledger unique key remains across rebuild, while derived read views may be recreated.

### CP-R06 · RepairRun

키: `runId`

필드:

- `scope`
- `reason`
- `dryRun`
- `currentGeneration`
- `candidateGeneration`
- `sourceManifestId`
- `diffDigest`
- `fence`
- `status`
- `unresolvedSources`
- `publicationDecision`
- `checkpoint`

불변식: states requested→capturing→building→diff_ready→publishing→completed; any failure→held/quarantined. Completion is not permission to perform external effects.

### 적용 순서

1. Persist received event as pending with eventId dedup; do not ACK semantic application before verifying version/source scope. Transport ACK after durable pending is separate from applied state.
2. Resolve authoritative snapshot manifest; cross-resource reads need same source transaction cut or an immutable complete manifest protocol. Unknown or incomparable vectors -> hold/recapture, never max(revisions).
3. Compute target from current stored policy; lock current shared source/eligibility/privacy gates, then consumer checkpoint and effect/contribution keys in stable order. Check captured versions and completeness again.
4. Commit target contribution, signed correction or view update, application marker, checkpoint/projection revision and downstream outbox atomically within consumer store. Cross-store effects use durable deduped effect intent; no distributed atomicity assumed.
5. External messages/device sync/search/AI are outbox deliveries with their own effect identity and current publication authorization. A stale worker fence only prevents DB writes, not previously permitted remote effects.

`ConsumerApplication`의 논리 고유키는 `(consumer, aggregateType, aggregateId, aggregateRevision)`이다. 정규화 digest를 이 key에 넣으면 서로 다른 내용이 별개의 정상 행으로 삽입될 수 있으므로 digest는 충돌 검사용 값으로 둔다. 동일 revision 충돌은 격리하고 권위 원천을 재확인한다.

소비자의 pending 기록과 메시지 transport ACK는 업무 적용 완료와 다르다. pending까지 내구성 있게 기록한 뒤 ACK했다면, 장애 후 pending을 반드시 다시 처리할 durable 작업 목록이 필요하다. target/correction/applied checkpoint가 함께 commit되기 전에는 applied로 표시하지 않는다.

같은 저장 영역에서는 shared gate/원천 잠금 이후 consumer checkpoint·기여분·effect를 고정 순서로 잠근다. 원격 source를 사용하는 경우 서비스 간 검증/예약 또는 동등한 일관성 protocol이 있어야 효과를 확정할 수 있다. publication 직전 원격 읽기 한 번만으로 경쟁이 없어졌다고 가정하지 않는다. 해당 adapter가 미선정이면 효과 확정/공개 경로도 미검증·보류 대상이다.

## 6. 이벤트 구독과 추가 trigger

| 원 이벤트 | 기존 소비자 | 추가할 소비자 제안 |
|---|---|---|
| payment.acceptance.changed | sales, benefits, receipts, travel | settlements |
| refund.state.changed | sales, benefits, receipts | settlements, travel |

기존 이벤트 10개 카탈로그는 변경하지 않았다. 위 구독 변경과 v2 envelope 채택은 후속 기준 병합 대상이다. settlement 서비스에 이 이벤트가 이미 배달된다고 가정하지 않는다.

영수증 ownership commit도 benefits/receipts/travel 재평가의 trigger다. durable outbox 또는 같은 transaction에 기록한 reconciliation work item의 계약이 필요하며 새 공개 이벤트 이름을 등록한 것은 아니다. claim 하나가 새 매출·새 지급을 만들지는 않는다. privacy.request.changed/동의 변경, 혜택 소비 commit, 정산 마감·보정 명령도 각각 현재 권한과 자체 revision을 확인한다.

여행 소비자의 결과는 기존 travel.evidence.changed를 통해 챌린지/코스의 의존성을 재평가한다. consumer 결과가 원 payment event를 다시 발행하는 순환을 만들지 않는다. 추천/검색/NU 스탬프 화면 갱신은 outbox 전달 결과이며 실제 혜택 사용권의 권위가 아니다.

## 7. 재구축 7단계

### CP-B01 · 원인·범위 고정

원천 누락/버전 전환/백업/계산 오류를 기록하고 exact-store 운영권한·projection 범위를 고정; 외부 지급/보상 실행 권한과 분리.

### CP-B02 · 권위 snapshot 확보

완전성·source vector·policy·privacy/ownership revision을 갖는 불변 manifest 확보. refund 목록 일부 누락/unknown hold이면 중단.

### CP-B03 · 후보 generation 계산

원본 증거와 원규칙으로 shadow 계산. live writer는 유지해도 외부효과/효력있는 적립·사용·정산마감은 shadow에서 금지.

### CP-B04 · 차이 검토

기존 기여분 대비 signed delta, 누락/추가 evidence, 이미 사용한 보상, closed period 영향·삭제 충돌 목록 생성. 정책 미정은 held.

### CP-B05 · 최신성 재확인

현재 gate/vector와 후보 manifest를 대조해 전환 준비만 한다. source/소유권/삭제 변경 시 recapture. 이 단계는 active pointer/fence를 바꾸거나 공개 권한을 만들지 않는다.

### CP-B06 · 보정·발행

publication transaction 안에서 최신 source/privacy/fence를 다시 확인하고 active generation pointer·새 fence·기여분·보정·application·checkpoint·outbox를 함께 commit한다. 불변 shadow view는 미리 작성 가능하지만 pointer 전환은 마지막 commit에만 포함한다. immutable ledger는 동일 논리 effect key의 보정만 append하며 외부 알림은 commit 이후 현재 권한으로 전달한다. 큰 rebuild 분할이 필요하면 publication held/resume protocol을 별도 설계하기 전 공개를 차단한다.

### CP-B07 · 증거·복구 종료

before/after manifest/diff/effect key/결과 checkpoint 보관. rollback은 검증된 view/fence만 전환하며 이미 발행한 원장 보정/권한 철회/삭제를 되돌리지 않음.

view 재구축과 불변 원장 재생성을 구분한다. shadow generation은 조회값을 계산할 수 있지만 효과가 있는 grant/consume/환불/정산 승인/외부 알림을 실행하지 않는다. 전환 시 동일 지급·entitlement의 기존 effect identity와 원장 잔액을 기준으로 필요한 correction만 한 번 기록한다. generation을 키에 넣어 새로운 혜택을 발급하는 경로는 금지한다.

아직 발행하지 않은 검증된 view를 되돌릴 수는 있지만, 이미 공개한 정보·발행한 보정·허가된 외부효과를 generation rollback으로 없던 일로 만들 수는 없다. 최신 fence와 deny 상태를 유지하면서 앞으로 보정한다.

## 8. 운영 관측·검증 사례

운영 화면에는 consumer별 pending/held/quarantined 수, 마지막 정상 manifest, 목표와 적용 vector 차이, correction_pending 정산, 미소유 entitlement, 혜택 deficit, 삭제 publication 차단을 표시하는 제안이다. 경보 임계값·보관 기간(D19)은 미선정이다. 로그에는 토큰/원본 proof/니모닉/개인 위치 경로를 복사하지 않는다.

| ID | 사례 | 기대 결과 |
|---|---|---|
| CP-T01 | 중복 eventId | 동일 consumer에서 pending 또는 applied 결과 반환; 금액·혜택 추가 반영 없음. |
| CP-T02 | v1/v2의 다른 eventId·같은 원천 revision | 동일 논리 application 한 번; 다른 내용은 격리. |
| CP-T03 | 누락·역순 이벤트 | 부분 delta 덧셈 금지; 완전한 snapshot 기반 목표로 재계산. |
| CP-T04 | 동일 revision의 다른 digest | 한쪽을 자동 최신으로 선택하지 않고 source 재검증·격리. |
| CP-T05 | 원입금 reorg 후 환불만 유효 | 영업 이력 보존; 현재 입금0/환불6이면 cash net -6 및 차이 보류; 0으로 clamp 금지. |
| CP-T06 | 예외 지급 수취와 반환 | 실수취 대조에 포함하지만 정상 매출/매출 환불로 이중 분류하지 않음. |
| CP-T07 | 원천 snapshot에서 refund 페이지 누락 | complete=false 처리; 누락 환불0으로 계산 금지. |
| CP-T08 | 마감 후 원지급 재확인 | 원마감 snapshot 보존; 원기간 correction_pending 및 새 manifest. 신규 자금 지급 없음. |
| CP-T09 | 동시 마감·보정 | settlement revision/fence CAS 한 경로만 성공; stale 결과 다시 조회. |
| CP-T10 | 다중 자산·테스트넷 혼재 | chain/environment/asset별 분리; decimals 동일해도 단순합산 금지. |
| CP-T11 | guest claim과 reorg 경합 | 실제 owner와 현재 target를 함께 검사; pending/materialized 한 경로만 보정. |
| CP-T12 | 사용한 스탬프의 원지급 무효화 | consume 이력 유지; 보정 후 deficit와 추가사용 보류, 자동 금전청구 없음. |
| CP-T13 | 동일 지급 재확정 | original entitlement target 복구 보정 한 번; 새 grant/새 reward_key 생성 금지. |
| CP-T14 | 원천 차단 직후 혜택 사용 | consumer 지연과 무관하게 현재 eligibility/source gate에서 거절. |
| CP-T15 | 영수증 소유권 회수 후 과거 snapshot 재생 | 현재 ACL이 거절; 이전 snapshot으로 소유권/공개 복원 금지. |
| CP-T16 | 결제 무효화와 독립 위치 방문 | 결제 badge만 재판정; 위치 방문·사용자 후기는 보존하며 규칙/동의에 따라 표기. |
| CP-T17 | 부분환불과 미선정 여행 규칙 | 방문은 자동 삭제하지 않고 해당 구매 badge/reward를 policy_pending 처리. |
| CP-T18 | 사용자 삭제 뒤 오래된 이벤트 | tombstone 우선, 공개 view/AI입력/보상 수신자 자동 복원 금지. |
| CP-T19 | 재구축 중 신규 환불 도착 | publication 전 vector/목록revision 불일치로 recapture; 과거 target로 덮어쓰기 금지. |
| CP-T20 | commit 성공 후 worker/응답 소실 | checkpoint/effect key로 완료를 확인; 다시 consume/grant/보정하지 않음. |
| CP-T21 | 구 generation worker 생존 | stale fence DB commit 거절; 이미 허가된 외부효과는 추적하고 취소 완료로 간주하지 않음. |
| CP-T22 | 복구 snapshot에 최근 삭제/철회 누락 | 최신 deny 권위를 복원할 때까지 publication/신규 사용 보류; 미검증 backup으로 재공개 금지. |

모든 위 실행 사례는 **not_run**이다. 별도 합성 예제 22개는 금액 목표차이 6개·혜택 보정 4개·publication 조건 7개·이벤트 순서 5개를 검사한다. 이 검사는 실제 source snapshot의 완전성, DB 원자성, 권한 경쟁, 메시지 재전송 또는 기기 동작을 검증하지 않는다.

## 9. 채택 상태와 다음 작업

원본 입력 9개의 hash를 고정했다. 기존 API 110개·이벤트 10개·SQL 61개와 승인/commerce 기준 파일을 유지한다. 이번 계약은 현재 상세 설계에 연결된 제안이며 운영 중인 worker나 배포 완료를 뜻하지 않는다.

D08, D02, D19, 혜택/여행 보정과 정산 attribution 정책, RR-DEC-01은 미선정 상태를 유지한다. 사용자의 12주·15개 요구사항·3명 범위와 역할/공수 산정 보류에도 변경이 없다.

후속 [매출·정산/운영 명령 계약](settlement-ops-contracts.md)에 freshness·manifest·음수 차이·마감 보류·재처리 조회/반영을 연결했다. 다음은 미병합 설계의 통합 묶음과 수용 기준 정리다.

# 구현 인계용 통합 계약 보완안

2026-09-19 · logical overlay · API/SQL/firmware 미병합 · 구현 보류

DS01~08 사이의 미연결 요청·상태·저장·권한을 논리적으로 닫는 overlay. 아래 OC 이름/경로는 API 번호를 부여하지 않은 후보이며 현재 서버/펌웨어가 지원한다는 뜻이 아니다. 원본 카탈로그 110경로를 직접 바꾸지 않는다.

## 공통 봉투

- **request**: schemaVersion · environmentId · requestId · idempotencyKey(for mutation) · expectedRevision(for existing mutable object) · typedBody
- **response**: operationId · operationClass · parentRef · status · revision · resultRef(optional) · error(optional)
- **identity**: idempotency scope = environment + authenticated actor/scope + route + idempotencyKey; canonical request digest 불일치 CONFLICT
- **versioning**: unknown version/discriminator/additional security fields 거절. field optional/required·wire encoding·limits는 선택 protocol schema로 고정. 클라이언트가 보낸 actor/role/owner는 권한 근거 아님.
- **amount**: chain/environment + asset identity + amountAtomic 정수 문자열; display decimals는 manifest에서만. floating point 금액 금지.
- **errors**: INVALID_INPUT · UNSUPPORTED_PROFILE · UNAUTHORIZED · FORBIDDEN · REVISION_CONFLICT · SOURCE_STALE · POLICY_UNSELECTED · AUTHORITY_HELD · EXPOSURE_UNKNOWN · ALREADY_APPLIED · PRIVACY_DENIED · RESULT_PENDING
- **retry**: 네트워크 실패는 실패확정이 아니다. 동일 operation/idem 조회부터 수행; 원 노출의 결론 없이 새로운 signer payload/nonce/payment 생성 금지.

## 16개 경계 계약

### OC-01 auth provider flow와identity

- **existingApiRefs**: API-103 · API-001 · API-002
- **candidateRoute**: 현재 경로의 provider-discriminated adapter
- **authority**: 선정 client/OS의 provider flow binding+nonce/state/PKCE profile,현재 account link 권한
- **requestFields**: provider,flowId,clientProfile,authorizationResponse,linkIntentRef?,expectedAccountRevision?
- **responseFields**: accountRef,sessionRef,identityRef,linkState,operationRef
- **atomicBoundary**: issuer+subject unique와link claim; 계정생성/연결을 이메일로 자동병합 금지
- **recovery**: 응답유실은flowId 원결과. auth response 재교환/재연결 무한재시도 금지

### OC-02 session revoke/logout scope

- **existingApiRefs**: API-003 · API-004
- **candidateRoute**: 후보 POST /v1/session-revocations
- **authority**: current session owner;all_sessions는 recent-auth scope; account_security_hold는 별도권한
- **requestFields**: scope=this_session\|all_sessions,expectedAccountRevision,reason
- **responseFields**: revocationId,scope,sessionEpoch,affectedCount,status
- **atomicBoundary**: session epoch+outbox CAS; 클라이언트locallogout과서버globalrevoke구별
- **recovery**: 불명응답에서도로컬노출차단;로그아웃으로이미서명된tx를없애지않음

### OC-03 Cloud 생성·복구 parent

- **existingApiRefs**: API-014 · API-016 · API-020
- **candidateRoute**: API014/016의 lifecycle 요청 확장 + 후보 GET /v1/wallet-lifecycle-operations/{operationId}
- **authority**: account owner 또는 operation-bound recovery read credential; 후자는 서명/자금권한 없음
- **requestFields**: POST: createRequestId 또는 recoveryRequestId,expectedParticipantEpoch,mode,proofRef,newParticipantRef,protocolProfile. GET: path operationId, 별도 body 없음
- **responseFields**: operationClass=cloud_wallet_lifecycle,phase,walletRef,publicKey,participantEpoch,holdReasons,addressOutcome,revision
- **atomicBoundary**: verified recovery권한 후fence→prepared→commit→requiredACK→verify→release ownfence;current policy/authority CAS
- **recovery**: read credential environment/op/expiry/read-only 결합·발급때복구증명 확인. accountsession상실시임의조회토큰발급금지

### OC-04 terminal provisioning·종료

- **existingApiRefs**: API-023
- **candidateRoute**: 후보 POST /v1/stores/{storeId}/terminal-enrollments; POST /v1/terminal-sessions/{id}/end
- **authority**: current store terminal admin+physical enrollment proof;guest는자기session종료만
- **requestFields**: terminalPublicKey,challengeRef,proof,mode,expectedTerminalRevision;end:expectedSessionRevision
- **responseFields**: terminalId,grantRef,sessionId,closedAt,clearanceStatus
- **atomicBoundary**: store-bound terminalgrant+epoch unique;end는customercontextdeny/outbox와원자
- **recovery**: guest가주문결과를다시볼ticket은order/session-bound별도능력. terminal분실현재grantrevoke

### OC-05 order snapshot·recipient cut

- **existingApiRefs**: API-027 · API-028 · API-029 · API-032
- **candidateRoute**: 현재 경로 확장 후보
- **authority**: current store menu/address role와order actor; 수취주소변경은signer통제증명
- **requestFields**: menuRevision,items/options,recipientRevision,assetRef,quoteRef,expectedStoreRevision
- **responseFields**: orderId,immutableItemTotals,recipientSnapshot,assetUnits,priceVersion,quoteExpiry
- **atomicBoundary**: order commit때current menu/recipient snapshot고정. concurrent변경은conflict·새가격확인
- **recovery**: 기존order주소를새설정으로rewrite금지;expired quote새승인. nativegas별도 표시

### OC-06 fulfillment 명령

- **existingApiRefs**: API-030 · API-092
- **candidateRoute**: 후보 POST /v1/stores/{storeId}/orders/{orderId}/fulfillment-actions
- **authority**: current exact-store fulfillment role;customer/ops readonly에mutation승격금지
- **requestFields**: action=prepare\|ready\|handover\|cancel_preparation,expectedOrderRevision,expectedPaymentRevision,reason
- **responseFields**: fulfillmentState,revision,actorAuditRef,paymentDisplayState
- **atomicBoundary**: payment accepted/hold 정책과currentrevision 검사+state/outbox atomic;handover는한번효과
- **recovery**: payment reorg가실물인도를되돌리지는않음;fulfilled 유지+payment_exception 별도

### OC-07 guest receipt·claim ticket

- **existingApiRefs**: API-042 · API-043 · API-044 · API-104 · API-105 · API-106
- **candidateRoute**: 기존 claim 경로의 proof-bound 전달/조회 후보
- **authority**: guest session proof와claimant current account/walletbinding;order번호단독불가
- **requestFields**: ticketId,claimChallenge,holderProof,expectedClaimRevision,selectedBenefitPolicy
- **responseFields**: claimId,receiptProjection,benefitState,sourceVector,privacyRevision
- **atomicBoundary**: single claim+benefit effect원장;잘못된소유자claim차단
- **recovery**: 앱/기기간응답유실동일claim조회;ticket만유출돼도임의owner가되지않음

### OC-08 settlement correction/read/publish

- **existingApiRefs**: API-039 · API-040 · API-041 · API-093 · API-087
- **candidateRoute**: 기존 settlement-ops SR-01/SR-02 후보 참조
- **authority**: read/preview/source_refresh와publish 역할분리;exact store currentpolicy
- **requestFields**: sourceVectorDigest,expectedProjectionRevision,reason,preparedCorrectionRef
- **responseFields**: prepared\|published\|stale,projectionRevision,signedDelta,auditRef
- **atomicBoundary**: CP-B05 prepare→CP-B06 singleconsumer atomic publish;API087자동publish금지
- **recovery**: 재시도는원correction;은행지급/merchant서명자동생성금지

### OC-09 market quote/action

- **existingApiRefs**: API-057 · API-058 · API-059 · API-061 · API-062
- **candidateRoute**: 현재 경로의 action-discriminated market adapter
- **authority**: current wallet_use/market actor+typedapproval/source/currentrisk
- **requestFields**: marketProfile,actionVariant,quoteRef,expectedPositionRevision,priceSnapshot,allowancePlan,executionPlan,reviewDigest
- **responseFields**: quote\|review_ready\|allowance_pending\|execution_pending\|result_observed\|completed\|unknown,actualEffects,sourceVector
- **atomicBoundary**: allowance child와execution child ancestry;standaloneapprove 관측 vs bundledpermit 준비구별
- **recovery**: UserOp/bundle내실제효과 확인. readonly결과와새시장행위권 분리

### OC-10 market order cancel/keeper

- **existingApiRefs**: API-060 · API-061 · API-062
- **candidateRoute**: 후보 POST /v1/market-orders/{id}/cancel; 내부 keeper command
- **authority**: owner currentordercontrol;keeper는등록market/period/contractscope
- **requestFields**: orderId,expectedOrderRevision,cancelPlan;keeper:periodId,positionRevision,priceEvidence
- **responseFields**: cancel_requested\|cancelled\|filled\|partial\|unknown,exposureRef,actualEffects
- **atomicBoundary**: cancel은contractcapability존재시만. fill/cancel경합최종체인판정;keepercontract idempotency필수
- **recovery**: timeout은cancel완료아님;반대매매자동실행금지

### OC-11 credential/offering typed source

- **existingApiRefs**: API-064 · API-065 · API-067 · API-068 · API-069
- **candidateRoute**: 현재 경로의 credential_proof 및 offering_action adapter 후보
- **authority**: issuer/verifier/holder 각각 현재trust/scope;offering actor와wallet통제추가
- **requestFields**: DS06 CC01~04,challenge/audience/rightsVersion/sourceVector
- **responseFields**: credential\|verification\|offeringAction,eligibilityReasons,statusEvidence,operationRef
- **atomicBoundary**: challenge단회소비와currenttrust/status;offeringcontract에서직접호출제약
- **recovery**: offering_action은기존sourceKind없음.typed등록없이personal_transfer또는market_actionfallback금지

### OC-12 x402 binding·entitlement

- **existingApiRefs**: API-070 · API-071 · API-072
- **candidateRoute**: 실제 유료 resource HTTP adapter + 기존 lifecycle 경로
- **authority**: owner/request+paid_resource currentapproval;외부x402 client는선정구매주체binding별도
- **requestFields**: DS06 CC05~06+protocolHeaderMapping,schemeProof,attemptRef
- **responseFields**: paymentRequirement,settlementState,entitlementState,deliveryState,receipt,resultRef
- **atomicBoundary**: payment/refund/entitlement/delivery독립축CAS+outbox. refund예약과generation/publish가동일entitlementrevision/fence검사. proof와settlement구별
- **recovery**: 일반APIwrapper만으로x402상호운용주장불가.실제402wire→proof→settle→result시험필수

### OC-13 audio upload/processing

- **existingApiRefs**: API-051 · API-052 · API-053 · API-094 · API-107
- **candidateRoute**: 후보 POST /v1/recordings/{id}/uploads; POST .../uploads/{uploadId}/complete
- **authority**: owner currentpurposeconsent+registeredsource;crossownerobject거절
- **requestFields**: manifestDigest,byteLength,formatProfile,chunkChecksums,expectedRecordingRevision,consentRevision
- **responseFields**: uploadId,scopedObjectRef,receivedParts,processingJob,protectedResultRef
- **atomicBoundary**: complete는stored bytes/checksum검사후sourceobjectregister;metadata POST로업로드승인추정금지
- **recovery**: resume동일upload;공간부족/파일손상partial;외부STT요청직전동의재검사

### OC-14 travel evidence/recommendation

- **existingApiRefs**: API-073 · API-074 · API-075 · API-076 · API-077 · API-078 · API-079 · API-080 · API-081 · API-082 · API-083 · API-095 · API-096 · API-097
- **candidateRoute**: 현재경로 typed evidence와job/result 확장
- **authority**: owner/review_author/trip_owner/participant currentprivacy;placeprovider신뢰 별도
- **requestFields**: DS07 TC03~05+constraints/sourceVector/placeMappingRevision
- **responseFields**: provenanceKind,validationReasons,courseRevision,stepValidity,rewardState
- **atomicBoundary**: 현재source/privacy/revision으로publish;챌린지step과rewardeffect각unique
- **recovery**: payment correction은badge/혜택재판정만;원후기/독립위치자동삭제금지

### OC-15 privacy request progress

- **existingApiRefs**: API-084 · API-085 · API-107
- **candidateRoute**: 후보 GET /v1/me/data-requests/{requestId}
- **authority**: current owner+recent-auth exportpolicy;account삭제후한정receipt는별도readcredential
- **requestFields**: scopeSnapshot,consentRevision,privacyRevision,artifactGraphRef
- **responseFields**: denyEffectiveAt,targetStatuses,pendingExternal,retentionExceptions,receiptRef
- **atomicBoundary**: deny/tombstone+outbox먼저CAS;삭제target별revision/attempt;모든read/publish최신deny검사
- **recovery**: privacy원장복구불가면외부서비스재개차단. lateartifact도원job/source삭제scope에자동추가·증거갱신;새동의로부활금지

### OC-16 protected result / operation readers

- **existingApiRefs**: API-020 · API-107
- **candidateRoute**: 현재경로 typed registry 확장
- **authority**: current parent object/actor/session/epoch/purpose 검증;worker결과작성권과사용자읽기권분리
- **requestFields**: operationClass,parentRef,resultId,sourceVector,authorityEpoch,privacyRevision
- **responseFields**: typed status,redacted reason,permitted resultRef,poll policy
- **atomicBoundary**: 등록된class+parentloader+ACL+immutableartifactmetadata만조회.unknownclass거절
- **recovery**: 원부모삭제/철회는읽기차단;임의URL/resultId소지만으로접근불가

## 행위별 필수 입력·실제 효과

### AV-01 

- **tag**: swap_exact_input
- **requiredFields**: tokenIn,tokenOut,amountInAtomic,minOutAtomic,route,recipient,deadline
- **effectEvidence**: actual in/out,router/pool evidence
- **restriction**: quote 단위·route/code allowlist

### AV-02 

- **tag**: swap_exact_output
- **requiredFields**: tokenIn,tokenOut,amountOutAtomic,maxInAtomic,route,recipient,deadline
- **effectEvidence**: actual out 및 최대지출 준수
- **restriction**: 잔여 allowance 별도 표시

### AV-03 

- **tag**: liquidity_add
- **requiredFields**: poolRef,positionKind,tokens/maxima,sharesOrLiquidityMin,range(if supported)
- **effectEvidence**: 실제 투입과 LP/position 증가
- **restriction**: fungible shares와NFT/range schema분리

### AV-04 

- **tag**: liquidity_remove
- **requiredFields**: positionRef,amountKind=shares\|liquidity,amount,minima,recipient
- **effectEvidence**: 지분축소와실제수취 별도
- **restriction**: collect 별도면 회수완료에수수료포함추정금지

### AV-05 

- **tag**: fee_collect
- **requiredFields**: positionRef,tokens,requestedMaxima,recipient
- **effectEvidence**: fees owed 변화와실제수취
- **restriction**: 범위형외모델은capability확인

### AV-06 

- **tag**: fx_spot
- **requiredFields**: baseAsset,quoteAsset,direction,rateNum,rateDen,amounts,validity
- **effectEvidence**: 해당모델swap실제효과
- **restriction**: FX파생선정시별도tag/schema필요

### AV-07 

- **tag**: perp_change
- **requiredFields**: market,positionId,side,sizeDelta,reduceOnly,collateral,priceLimit,riskRevision
- **effectEvidence**: position 전후·fee/funding·collateral
- **restriction**: linear/inverse/cross/isolated profile선정필수

### AV-08 

- **tag**: collateral_change
- **requiredFields**: position/accountRef,direction,asset,amount,healthSnapshot
- **effectEvidence**: 실제담보전후와remaininghealth
- **restriction**: withdraw는현재risk검사

### AV-09 

- **tag**: offering_action
- **requiredFields**: offeringId,rightsVersion,action,quantity,recipient,eligibilityRefs
- **effectEvidence**: issuance/transfer/burn와지급각효과
- **restriction**: STO를금전이체tag로위장금지

### AV-10 

- **tag**: credential_presentation
- **requiredFields**: requestRef,challengeRef,audience,disclosureDigest,holderBinding,expiry
- **effectEvidence**: verificationRecord,challenge consumption
- **restriction**: tx nonce/gas와무관한domain 분리

### AV-11 

- **tag**: paid_resource
- **requiredFields**: requestId,resourceVersion,inputDigest,paymentRequirementDigest,attemptRef
- **effectEvidence**: settlementReceipt+entitlement+delivery
- **restriction**: 자동재요청이새지급승인이아님

## 저장 단위·원자성

### AC-01 

- **unit**: typed operation
- **records**: operation,typed parent,idem digest,source vector
- **uniqueOrCas**: environment+actor+route+idem;operationclass/parent immutable
- **transaction**: 권한/revision확인+예약+outbox
- **outsideTransaction**: network call은commit후,외부결과는currentguard다시확인

### AC-02 

- **unit**: terminal/customer boundary
- **records**: terminalgrant,session,customercontext
- **uniqueOrCas**: terminal epoch+session revision
- **transaction**: 종료deny+clearqueue
- **outsideTransaction**: 단말ack없으면clearance pending,고객재사용금지

### AC-03 

- **unit**: fulfillment
- **records**: order/payment snapshot,fulfillment log
- **uniqueOrCas**: order revision+action idem
- **transaction**: 현재권한/허용상태+history+outbox
- **outsideTransaction**: 실물인도는DBrollback불가,확인기록별도

### AC-04 

- **unit**: credential verify
- **records**: challenge,verification,trust/statusvector
- **uniqueOrCas**: (environment,verifier,challengeRef) 단회소비 unique/CAS; proofDigest는 동일 재시도 비교값
- **transaction**: challengeconsume와result저장
- **outsideTransaction**: 외부status조회시점/정책유효성재검사;unknown이면거절

### AC-05 

- **unit**: paid resource
- **records**: attempt,exposure,paymenteffect,entitlement,delivery
- **uniqueOrCas**: payment effect exclusive allocation+request/version
- **transaction**: receipt할당/권리부여/outbox
- **outsideTransaction**: facilitator/chain/generator는원자DB밖;정확히한번외부실행주장금지

### AC-06 

- **unit**: content upload
- **records**: uploadsession,parts,artifactmetadata
- **uniqueOrCas**: owner+upload+partchecksum
- **transaction**: finalize CAS후source등록
- **outsideTransaction**: objectbytes저장은외부;orphan수거는privacy정책내

### AC-07 

- **unit**: privacy
- **records**: deny,tombstone,artifactgraph,deletiontargets
- **uniqueOrCas**: privacyrevision monotonic+targetrevision
- **transaction**: 즉시deny와삭제queue
- **outsideTransaction**: backup/provider/phone비동기확인,실패는pending

### AC-08 

- **unit**: reward/consumer publish
- **records**: sourcevector,prepared projection,benefiteffect
- **uniqueOrCas**: stable entitlement/reward business source identity는 sourceVector와 분리; correctionId unique,expectedprojectionrevision/sourceVector CAS
- **transaction**: 동일 stable effect의 목표-반영량 delta와 CP-B06 단일consumerpublish; 새sourceVector가새보상을생성하지않음
- **outsideTransaction**: 새환불/이체/서명 자동발생금지

## 설계 반례

| id | case | expected |
| --- | --- | --- |
| FC-01 | 동일idem 다른payload | CONFLICT,외부호출0 |
| FC-02 | 동일idem 같은payload 응답유실 | 원operation조회,새operation0 |
| FC-03 | 허용되지않은sourceKind | UNSUPPORTED_PROFILE,서명0 |
| FC-04 | 원source개정뒤늦은worker | SOURCE_STALE,공개0 |
| FC-05 | 철회된issuer인데유효서명 | 현재trust거절,새자격사용0 |
| FC-06 | settlement timeout | EXPOSURE_UNKNOWN,새결제0 |
| FC-07 | delivery failed after paid | 원entitlement 유지,새결제0 |
| FC-08 | fulfilled order payment reorg | fulfillment유지,paymentexception표시 |
| FC-09 | delete tombstone then backup restore | PRIVACY_DENIED,원문공개0 |
| FC-10 | permitprepared notmined | allowanceapplied로표시0 |
| FC-11 | 다른store fulfillment | FORBIDDEN,mutation0 |
| FC-12 | 확인안된방문을realpurchase로 | INVALID_INPUT,실제구매배지0 |

## 카탈로그 채택 순서

1. 각 OC를 기존 route 확장 또는 새 route로 분류하고 API 번호·operationClass·discriminated request/response를 함께 부여한다. 스키마/DTO/access/source dispatch/typed reader/화면/저장 매핑 중 하나만 반영하지 않는다.
2. SDK·board·provider·contract model 선택값을 profile manifest로 pin한 뒤 wire codecs/limits/신뢰체인을 구체화한다. 미선정 profile은 문서placeholder를 실행값으로 사용하지 않는다.
3. 기존 DI01..08와lifecycle 후보의fullenvelope를 보존하고 이 overlay의새입력을충돌대조한다. API037 merchantSigner·expectedRevision·expectedFundingRevision과현재approvalgate는필수보존.
4. 새물리DDL은논리키/unique/CAS/권한/삭제정책을매핑한후별도migration문서로작성·검토. 기존61business tables/7migrations를설계문서작성으로변경했다고표시하지않는다.
5. valid/invalid/duplicate/reordered/lostresponse/revoked/reorg/restore 예제를각route/actiontag별준비하고schema/ACL/semanticchecks를같이실행한다.
6. baseline hash를새checkpoint에기록하고기존후보를superseded reference로남긴다. 원본자료자동삭제/기존hash상태덮어쓰기금지. 기준계약채택은제품구현/외부배포승인이아니다.

반례는 기대 결과를 정의한 설계 자료다. 제품 실행 결과가 아니다.

# DID·STO·x402 자격과 지급 상세 설계

후속 설계 LG09~11 반영 · 미병합 후보 · 제품 실행 미검증

## status

design_proposal_not_merged

## packageRef

DS-06

## implementation

deferred_by_user

## canonicalMerged

False

## runtimeVerified

False

## policySelectionUnchanged

True

## requirementRefs

- 11
## taskRefs

- STO-01
- STO-02
- STO-03
- DID-01
- DID-02
- DID-03
- X402-01
- X402-02
- X402-03
## decisionRefs

- D14
- D15
- D16
## boundary

자격의 진위·현재 이용자 권한·온체인 지급·서비스 제공을 각각 판정한다. 이 문서는 앱 내부 계약 후보이며 VC/x402 표준 wire 형식의 대체가 아니다. 표준 버전과 서명 suite, StableNet 자산/facilitator binding은 선택 전 실행 불가.

## journeys

### CJ-01

**title**: 자격 발급

**apiRefs**

```json
[
  "API-065",
  "API-066"
]
```

**screenRefs**

```json
[
  "U18"
]
```

**flow**: 등록 issuer의 현재 scope/trustRevision → subjectProof 검증 → 최소 claims/schema/expiry → issuanceId 고정 → issuer 서명 → 소유자 조회

**completion**: 발급 서명·issuer·subject binding·상태 근거를 확인; 점주 로그인만으로 issuer 권한 부여 금지

### CJ-02

**title**: 선택 공개 및 검증

**apiRefs**

```json
[
  "API-067",
  "API-068"
]
```

**screenRefs**

```json
[
  "U18"
]
```

**flow**: 등록 verifier가 audience/purpose/nonce/expiry 요청 → holder가 공개 필드 확인 → domain 분리 proof → verifier가 trust/status/freshness 검사

**completion**: challenge 단회소비와 검증 결과 연결; 재시도는 동일 digest의 원 결과만 반환. proof 유효와 서비스 자격 통과를 구별

### CJ-03

**title**: 자격 철회

**apiRefs**

```json
[
  "API-069",
  "API-066"
]
```

**screenRefs**

```json
[
  "U18"
]
```

**flow**: issuer 권한/expectedRevision 검사 → 상태 revision 갱신 → 영향받는 검증/자격 projection 재평가

**completion**: 과거 제출 기록은 보존하되 새 행위는 현재 철회 상태로 거절. 외부 복사본 삭제를 보장하지 않음

### CJ-04

**title**: 시험 발행물 발행·취득·이전

**apiRefs**

```json
[
  "API-063",
  "API-064",
  "API-102"
]
```

**screenRefs**

```json
[
  "U17"
]
```

**flow**: 발행물 권리/issuer/수량/제한 표시 → actor와 상대방 자격 → exact action intent 검토/서명 → 체인 실행 → 보유 조회

**completion**: 앱 표기와 실제 contract 제한이 일치. 본인 지갑 통제 증명만으로 적격자·발행자가 되지 않음

### CJ-05

**title**: 유료 HTTP 자원

**apiRefs**

```json
[
  "API-070",
  "API-071",
  "API-072"
]
```

**screenRefs**

```json
[
  "U19"
]
```

**flow**: 자원 버전/가격/네트워크/자산 확인 → 명시 지급 승인 → protocol adapter 검증/settle → entitlement → 결과 조회

**completion**: 지급 증거와 현재 접근 허가가 모두 있어야 제공. 402/202, 서명 생성, facilitator 검증만으로 지급 완료 처리 금지

## trustRules

### CT-01

**subject**: issuer

**rule**: 환경·credential type·권한범위·키 버전·활성기간·trustRevision으로 등록; issuance 시점의 유효 키와 현재 신뢰 정책을 함께 검증

**unavailable**: 알 수 없는 issuer/key는 unknown/거절, URL 임의 fetch 금지

### CT-02

**subject**: verifier

**rule**: 검증 목적·허용 claim·audience·challenge issuer를 registry에서 고정; holder 동의 범위를 넘는 공개 금지

**unavailable**: 미등록 목적 또는 필드에는 제출 거절

### CT-03

**subject**: holder

**rule**: subject와 signer의 관계를 credential profile로 확인; 이메일/계정ID/주소를 자동 동일시하지 않음

**unavailable**: holder binding 부재는 해당 profile 미지원

### CT-04

**subject**: status

**rule**: expiry와 revocation/suspension은 별도; status source의 서명·시점·최신성 정책 검사

**unavailable**: 조회 실패는 valid가 아닌 unknown; 새 권리 행사는 hold

### CT-05

**subject**: disclosure

**rule**: 선택 공개 지원은 선택 proof suite의 실제 능력으로 제한; 평문 claims 삭제만으로 재서명 없는 selective disclosure 주장 금지

**unavailable**: 지원하지 않는 공개 최소화는 다른 credential 발급 또는 제출 취소

### CT-06

**subject**: schema/context

**rule**: 승인된 버전/해시의 스키마·context·DID method resolver만 허용, 크기/깊이/네트워크 접근 제한

**unavailable**: 알 수 없는 context 또는 algorithm은 검증 거절

### CT-07

**subject**: signer separation

**rule**: credential proof·offering action·paid_resource 각각 domain/typed source/화면 표시를 구분; transaction blind signing 금지

**unavailable**: 현재 미등록 adapter는 execution_blocked 유지

## credentialTransitions

### CV-01

**fromState**: draft

**toState**: active

**guard**: 현재 issuer/subject/schema/profile 허가와 idem digest 일치

**effect**: credential 발급과 status revision 저장; presentation 결과와 독립

### CV-02

**fromState**: active|unknown

**toState**: revoked

**guard**: 현재 issuer 철회권한·expectedRevision 또는 권위있는 status 증거

**effect**: 철회 단조성 유지; 과거 verification 기록 보존

### CV-03

**fromState**: active|unknown

**toState**: expired

**guard**: profile clock/expiry 조건 검증

**effect**: 새 presentation 승인·검증 차단; 과거 기록 보존

### CV-04

**fromState**: active

**toState**: unknown

**guard**: status/trust 원천 불가 또는 상충

**effect**: 새 사용 보류; presentation 이력은 변경하지 않음

### CV-05

**fromState**: unknown

**toState**: active

**guard**: 현재 trust/status·미철회·미만료 확인 및 expectedRevision CAS

**effect**: credential의 현재 사용가능성만 회복, 새 challenge는 별도 session

## offeringActions

### OA-01

**action**: issue

**preconditions**: 등록 issuer·권리문서 version·cap/supply·수취자 eligibility

**contractBoundary**: 발행 권한/cap/수취 제한 on-chain enforcement; off-chain UI 검사만으로 대체 금지

**evidence**: 발행 event+현재 supply/holding

### OA-02

**action**: acquire

**preconditions**: payer와 recipient 분리, 필요 자격·가격·deadline·asset

**contractBoundary**: 자격 attestation의 chain/contract/action/subject/nonce/deadline 결합; 직접 contract 호출 우회 차단

**evidence**: 대금과 발행/이전 효과를 각각 판정

### OA-03

**action**: transfer

**preconditions**: 보유자 및 수취자 자격·transfer rule·잔량

**contractBoundary**: 현재 자격을 contract가 어떻게 확인하는지 adapter profile 필요; 만료 snapshot 재사용 금지

**evidence**: 송수신 balance와 실제 transfer event

### OA-04

**action**: redeem

**preconditions**: 선택한 시험 권리/기간/보유량·자격

**contractBoundary**: 소각과 지급/권리처리는 atomic 여부 명시; 비원자식이면 별도 미이행 claim 추적

**evidence**: 소각만 성공이면 상환 완료 아님

### OA-05

**action**: freeze_or_revoke_eligibility

**preconditions**: 권한있는 정책 변경과 expectedRevision

**contractBoundary**: 자격 상실과 보유량 몰수/소각은 별개. 선택 권리규칙 없는 강제이전 금지

**evidence**: 권리 제약 이유·유효시점·기존 보유량 표시

## paidResourceTransitions

### PX-01

**fromState**: created

**toState**: payment_required

**guard**: resourceVersion/inputDigest/price/network/asset/recipient/expiry 고정

**effect**: paymentRequirementDigest 생성; 변경 시 사용자 새 검토

### PX-02

**fromState**: payment_required

**toState**: proof_received

**guard**: owner+request+paymentAttemptId+proof digest 일치; typed adapter 등록

**effect**: 202는 접수만 의미, proof bytes 민감 로그 제외

### PX-03

**fromState**: proof_received

**toState**: settlement_pending

**guard**: 서명/nonce/자산/scheme/자금 검증, 현재 승인권한

**effect**: 같은 paymentAttemptId settle 단일 작업; 출처 불명 응답은 unknown

### PX-04

**fromState**: settlement_pending

**toState**: paid

**guard**: 선택 binding에 따른 실제 settlement 증거 및 확정 정책

**effect**: receipt와 stable business effect 연결; 같은 지급을 두 요청에 할당 금지

### PX-05

**fromState**: paid

**toState**: entitled

**guard**: 현재 owner/consent/resource terms 충족; entitlement 단일키 CAS; PR-GRANT 공통 predicate 필수: 현재 refund fence·authority/privacy/source 및 동일 request/entitlement revision CAS

**effect**: 결과 생성 job 예약과 outbox 원자 저장

**predicateRefs**

```json
[
  "PR-GRANT"
]
```

### PX-06

**fromState**: entitled

**toState**: delivering

**guard**: 현재 privacy/tombstone/entitlement 및 refund fence 재검사; 동일 request/entitlement revision CAS; PR-GENERATE 공통 predicate 필수: 현재 refund fence·authority/privacy/source 및 동일 request/entitlement revision CAS

**effect**: 고정 input/model/resource revision으로 job 실행

**predicateRefs**

```json
[
  "PR-GENERATE"
]
```

### PX-07

**fromState**: delivering

**toState**: delivered

**guard**: 결과 digest 저장과 publish CAS; 공개 직전 현재 권한·refund fence·entitlement revision 재검사; PR-PUBLISH 공통 predicate 필수: 현재 refund fence·authority/privacy/source 및 동일 request/entitlement revision CAS

**effect**: response loss는 기존 result 재조회, 새 과금 금지

**predicateRefs**

```json
[
  "PR-PUBLISH"
]
```

### PX-08

**fromState**: proof_received|settlement_pending|paid|entitled|delivering|delivered

**toState**: unknown

**guard**: 응답 유실·reorg·상충 receipt·원천 불가

**effect**: 원 지급/결과 조회, 과거 delivery 보존; 이미 전달한 내용을 회수했다고 주장 금지; 지급 원천이 불확실해진 경우 동일 request/entitlement CAS에서 payment=unknown + payment_uncertain fence를 원자 반영하고 신규 생성/공개를 차단한다. 단순 delivery 응답 유실은 payment를 unknown으로 바꾸지 않고 delivery 축/원 결과만 조회한다.

**axisDispatch**

```json
{
  "payment_evidence_invalid_or_unavailable": [
    "payment_unknown",
    "set_payment_uncertain_fence"
  ],
  "delivery_response_lost": [
    "preserve_payment",
    "lookup_original_delivery_result"
  ]
}
```

### PX-09

**fromState**: unknown

**toState**: paid

**guard**: 원 attempt settlement 재확인·동일 지급효과·최신 source revision

**effect**: payment 축만 갱신. refund_pending/refunded·privacy deny·delivery 이력 및 각 reason fence를 덮지 않음; delivered 결과 있으면 별도 새 생성 불필요; 현재 권위 원천에서 paid 재확인 후 CAS로 해당 payment_uncertain fence만 해제한다. refund/privacy/authority fence가 남으면 entitlement는 held 유지; 과거 delivered 이력은 그대로 보존.

### PX-10

**fromState**: entitled|delivering

**toState**: delivery_failed

**guard**: 비용 지출 후 결과 생성 실패

**effect**: 유료 재요청 자동 생성 금지; 같은 권리로 재시도 또는 별도 승인 환불

### PX-11

**fromState**: delivery_failed

**toState**: delivering

**guard**: 현재 권한·입력·제공기한·재시도 정책 유효; PR-GENERATE 공통 predicate 필수: 현재 refund fence·authority/privacy/source 및 동일 request/entitlement revision CAS

**effect**: same entitlement, jobAttempt만 증가

**predicateRefs**

```json
[
  "PR-GENERATE"
]
```

### PX-12

**fromState**: paid|entitled|delivery_failed|unknown

**toState**: refund_pending

**guard**: 현재 환불권한과 이미 지급된 effect 확인·중복환불 exposure 예약

**effect**: 동일 request/entitlement revision CAS로 refund reservation+별도 refund fence 설정. 정책 미선정이면 새 생성/공개 hold. 이미 전달한 사실은 보존. 일반 카페 merchant_refund로 위장하지 않고 별도 환불 계약 필요

### PX-13

**fromState**: refund_pending

**toState**: refunded

**guard**: 실제 반환 지급과 확정 기준 충족

**effect**: 현재 선택된 접근정책 적용, 미선정이면 refund fence 유지. 이미 제공한 파일 삭제보장 없음

### PX-14

**fromState**: created|payment_required

**toState**: cancelled

**guard**: 서명/제출/지급 exposure 없음

**effect**: 로컬 취소만. 노출된 attempt는 settlement 확인 전 취소완료 불가

## independentStateAxes

```json
{
  "payment": "required|proof_received|settlement_pending|paid|unknown|rejected|expired|failed_confirmed; paymentAttemptTransitions가 권위있는 지급축 상태표",
  "refund": "none|reserved|submitted|confirmed|unknown|failed_confirmed|cancelled_unexposed; refundAttemptTransitions와원 노출 원장 유지",
  "entitlement": "none|active|held|revoked; 원인별 fence와 entitlement revision",
  "delivery": "none|running|delivered|failed; 이미 전달한 사실은 결제보정으로 지우지 않음",
  "rule": "PX는 흐름의 주요 단계이며 단일 덮어쓰기 state enum이 아니다. 모든 callback은 해당 축 expectedRevision과 source/authority를 검사한다. refund reorg는 confirmed→unknown으로 노출 유지, 동일 attempt만 재조회하며 entitlement fence 자동 해제 금지."
}
```

## logicalContracts

### CC-01

**name**: CredentialRecord

**identity**: environment+credentialId

**fields**: ownerRef,issuerRef,subjectBinding,schemaVersion,proofFormat,statusRef,issuedAt,expiresAt,trustRevision,privacyRevision,credentialStatus,credentialRevision

**constraints**: claims/proof는 보호 저장; 공개체인은 필요한 status/commitment만, PII 원문/저엔트로피 hash 공개 금지; verified/presentation_ready 상태를 credential에 저장하지 않음

**existingApiRefs**

```json
[
  "API-065",
  "API-066",
  "API-069"
]
```

### CC-02

**name**: PresentationRequest

**identity**: environment+challengeRef

**fields**: verifierRef,audience,purpose,allowedClaims,nonce,expiresAt,holderBinding,revision,presentationId,presentationState,credentialRef,credentialRevision

**constraints**: challenge는 CSPRNG profile·단회사용·audience 결합. TTL 수치는 D15 선택; challenge별 독립 session, credential status 변경 없음

**existingApiRefs**

```json
[
  "API-067",
  "API-068"
]
```

### CC-03

**name**: VerificationRecord

**identity**: verifier+challengeRef+presentationDigest

**fields**: issuer/status/trust evidence,verifiedAt,decision,reason,sourceVector

**constraints**: 결과는 특정 purpose 시점 검증. STO 실행 때 현재 자격 추가 검사

**existingApiRefs**

```json
[
  "API-068"
]
```

### CC-04

**name**: OfferingActionIntent

**identity**: environment+offeringId+operationId

**fields**: action,rightsVersion,actorWallet,recipient,amountAtomic,asset,eligibilityRefs,policyRevision,contractBinding,deadline,reviewDigest

**constraints**: amount 정수 문자열; 계약 capability 미선정이면 action unsupported; sourceKind 신규 등록 또는 명시 typed 확장 필요

**existingApiRefs**

```json
[
  "API-064",
  "API-102"
]
```

### CC-05

**name**: PaidRequest

**identity**: owner+requestId

**fields**: resourceId,resourceVersion,inputDigest,network,asset,amountAtomic,recipient,schemeVersion,paymentAttemptId,requirementDigest,expiry

**constraints**: 같은 idem key 다른 body 충돌; 새 가격은 새 승인, 결제부터 생성을 재실행하지 않음

**existingApiRefs**

```json
[
  "API-070",
  "API-071"
]
```

### CC-06

**name**: EntitlementAndDelivery

**identity**: environment+requestId+resourceVersion

**fields**: paymentEffectId,receiptRef,ownerRef,accessPolicyRevision,deliveryState,resultRef,resultDigest,consentRevision,sourceVector

**constraints**: 하나의 결제효과 배타 할당; resultRef 보유는 조회 권한 아님; owner 범위 API072/API107으로 접근

**existingApiRefs**

```json
[
  "API-072",
  "API-107"
]
```

## policyInputs

### CP-01

**decisionRef**: D14

**needed**: 시험 권리/issuer/발행·취득·이전·상환 대표행위 및 contract 제한

**selection**: None

### CP-02

**decisionRef**: D15

**needed**: DID method, VC format/proof suite, issuer/verifier registry, status 방식·최신성

**selection**: None

### CP-03

**decisionRef**: D15

**needed**: 선택공개/holder binding/nonce TTL/키교체 정책

**selection**: None

### CP-04

**decisionRef**: D16

**needed**: x402 버전·scheme·StableNet network binding·token capability·facilitator 및 지급자

**selection**: None

### CP-05

**decisionRef**: D16

**needed**: 유료자원/가격/제공기한/환불·재시도 규칙

**selection**: None

### CP-06

**decisionRef**: D19

**needed**: 보관기간·실자산 서비스 전환 심사·운영권한

**selection**: None

## runtimeCases

### CQ-01

**scenario**: 정상 credential 발급/제시

**expected**: 선택 profile 실서명→현재 verifier 검증 성공

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-02

**scenario**: 미등록 issuer

**expected**: 점주 로그인 유효해도 발급/신뢰 거절

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-03

**scenario**: 다른 audience proof

**expected**: 서명 유효해도 사용 거절

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-04

**scenario**: nonce 재사용/응답 유실

**expected**: 원 digest에는 원 결과, 다른 proof 충돌

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-05

**scenario**: 철회 직후 STO intent

**expected**: 사전 검증 성공이어도 실행 보류

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-06

**scenario**: status source 장애

**expected**: valid 대신 unknown

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-07

**scenario**: unsupported selective disclosure

**expected**: 원문 과다공개 없이 제출 중단

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-08

**scenario**: 키교체된 issuer의 과거 credential

**expected**: issuance 시점 키+현재 trust 정책으로 재검증

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-09

**scenario**: credential 삭제 요청

**expected**: 보호 claims 제거/차단; 외부/온체인 기록 삭제 약속 금지

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-10

**scenario**: 발행 cap 초과 직접호출

**expected**: contract에서 거절

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-11

**scenario**: 부적격 수취인 직접 이전

**expected**: UI 우회해도 contract 거절

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-12

**scenario**: 상환 중 지급 실패

**expected**: 소각/지급 실제 효과별 부분상태 표시

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-13

**scenario**: 정상 유료 요청

**expected**: 402→승인→실제 지급→하나의 결과

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-14

**scenario**: 다른 요청에 proof 붙이기

**expected**: request/amount/recipient binding 불일치 거절

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-15

**scenario**: facilitator 성공만 반환

**expected**: 체인/선택 settlement 증거 없으면 paid 아님

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-16

**scenario**: 결제 전송 후 timeout

**expected**: 원 attempt 확인, 자동 재과금 없음

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-17

**scenario**: 지급 성공 후 응답 유실

**expected**: 원 result 조회로 복구

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-18

**scenario**: AI 결과 생성 실패

**expected**: 권리 보존·재시도 또는 별도환불

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-19

**scenario**: 지급 reorg 후 이미 읽은 결과

**expected**: 추가접근 정책 재평가·delivery 역사 유지

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-20

**scenario**: 동의 철회 후 늦은 결과

**expected**: publish 차단

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-21

**scenario**: 미등록 paid_resource adapter

**expected**: API 존재해도 서명/실행 차단

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-22

**scenario**: dummy token 기능 부족

**expected**: 정확한 x402 scheme 미지원 표시; ERC20 존재를 지원으로 간주하지 않음

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-23

**scenario**: 환불 응답 유실

**expected**: 원 환불 exposure 재조회, 중복반환 금지

**status**: not_run

**evidenceRefs**

```json
[]
```

### CQ-24

**scenario**: 철회된 verifier worker

**expected**: 수집된 proof 있어도 현재 권한 없는 결과 열람 차단

**status**: not_run

**evidenceRefs**

```json
[]
```

## externalReferences

### W3C VC Data Model 2.0

**title**: W3C VC Data Model 2.0

**url**: https://www.w3.org/TR/vc-data-model-2.0/

**note**: issuer/holder/verifier 모델을 참조. 유효한 서명만으로 주장 사실성·서비스 권한을 보장하지 않는다.

### W3C Bitstring Status List

**title**: W3C Bitstring Status List

**url**: https://www.w3.org/TR/vc-bitstring-status-list/

**note**: credential 상태 방식의 선택 후보. 지원 proof 형식과 상태 최신성 정책은 별도 결정한다.

### x402 HTTP 402

**title**: x402 HTTP 402

**url**: https://docs.x402.org/core-concepts/http-402

**note**: 공식 V2 문서는 PAYMENT-REQUIRED, PAYMENT-SIGNATURE, PAYMENT-RESPONSE 헤더를 설명한다. 이를 내부 API070~072와 동일 형식으로 간주하지 않으며 실제 버전/scheme binding을 별도 고정한다.

## credentialObjects

```json
{
  "credential": "credentialId별 active/revoked/expired/unknown; verified는 credential 상태가 아니다.",
  "presentation": "presentationId와(environment,verifier,challengeRef)별 독립 session; 서로 다른 challenge는 같은 active credential을 동시에 참조할 수 있다.",
  "verification": "immutable verificationId+presentationDigest+credential/status/trust revision+verifiedAt; 이력은 보존하되 새 행위에는 현재 자격 재검사.",
  "revocationRace": "verifier commit 직전 선택 status freshness와 현재 로컬 issuer/trust/status revision을 검사한다. 외부 issuer의 미래 철회를 원자적으로 막았다고 주장하지 않는다. 철회가 확인되면 후속 행위는 보류/거절한다."
}
```

## presentationTransitions

### VP-01

**fromState**: absent

**toState**: requested

**guard**: 등록 verifier/audience/purpose/nonce/기한, active credential 참조

**effect**: 새 presentationId 생성; credential 상태는 active 유지

### VP-02

**fromState**: requested

**toState**: approved

**guard**: 현재 holder/credential/동의 및 공개필드 확인

**effect**: reviewDigest·credential/status/trust revisions 결합

### VP-03

**fromState**: approved

**toState**: submitted

**guard**: 현재 승인과 challenge/기한 유효; typed credential_proof adapter

**effect**: 별도 domain proof 제출; credential 수명주기 상태 불변

### VP-04

**fromState**: submitted

**toState**: verified

**guard**: proof/holder/audience/current trust/status 검증; (environment,verifier,challengeRef) 소비 CAS

**effect**: verification 불변 저장. 동일 proof digest 재시도는 원 결과, 다른 digest 거절

### VP-05

**fromState**: requested|approved|submitted

**toState**: rejected

**guard**: 철회·무권한·proof/audience 불일치가 확인됨

**effect**: 실패 이유 저장; 다른 presentation session을 덮지 않음

### VP-06

**fromState**: requested|approved|submitted

**toState**: expired

**guard**: challenge 기한 만료

**effect**: 새 제출 금지; 기존 verified 기록은 만료 상태로 덮지 않음

### VP-07

**fromState**: submitted

**toState**: verification_unknown

**guard**: 외부 status 응답 유실 등 판단 불가

**effect**: 동일 session/소비예약 보존, 성공/실패로 단정하지 않음

### VP-08

**fromState**: verification_unknown

**toState**: submitted

**guard**: 원 요청 현재 권한/기한 재검사; 원 결과 없고 동일 digest 안전 재검증 가능

**effect**: 동일 challenge/operation 재조회·검증; 새 session으로 소비 우회 금지

### VP-09

**fromState**: verification_unknown

**toState**: verified

**guard**: 이미 durable 저장된 동일 challenge/proof 결과 확인 및 현재 조회권한

**effect**: 원 verification 반환; 새로운 권리행사 허가는 별도 현재 자격 검사

### VP-10

**fromState**: verification_unknown

**toState**: rejected

**guard**: 권위있는 철회·무효 증거 또는 현재 검증권한 거절

**effect**: 원 예약 종료 근거와 실패 기록

### VP-11

**fromState**: verification_unknown

**toState**: expired

**guard**: 기한 만료 및 원 durable 결과 없음 확인

**effect**: 판정 미완료 상태를 성공으로 승격하지 않음

## paidResourcePredicates

### PR-COMMON

**allOf**

```json
[
  "authority_current",
  "privacy_allowed",
  "source_current",
  "refund_clear",
  "entitlement_revision_matches",
  "request_revision_matches",
  "resource_terms_current",
  "payment_currently_confirmed"
]
```

**atomicity**: refund reservation과 entitlement/job reservation/publish가 동일 request/entitlement revision으로 CAS한다. 외부 호출 직전 guard를 재검사한다. 이미 전송된 외부 job은 환불로 회수 보장할 수 없으며 결과 publish는 다시 제한한다.; current payment state와 entitlement source fence를 같은 serialization boundary에서 검사한다. source_current만으로 지급 확정을 대체하지 않는다.

### PR-GRANT

**includes**

```json
[
  "PR-COMMON"
]
```

**allOf**

```json
[
  "entitlement_absent_or_same_identity"
]
```

**effect**: 기존 hold를 해제하지 않고 동일 entitlement를 생성/조회

### PR-GENERATE

**includes**

```json
[
  "PR-COMMON"
]
```

**allOf**

```json
[
  "entitlement_active",
  "input_digest_matches",
  "delivery_window_open"
]
```

**effect**: 예약/최초 실행/재시도에 동일 predicate; refund hold 중 새 외부 generation 금지

### PR-PUBLISH

**includes**

```json
[
  "PR-COMMON"
]
```

**allOf**

```json
[
  "entitlement_active",
  "result_digest_matches",
  "artifact_not_tombstoned"
]
```

**effect**: 보호 결과 공개 직전 current guard 및 CAS

## declaredScopeRequirementRefs

- 11
## transitiveTaskImpactRequirementRefs

- 11
## requirementRefsMeaning

legacy alias of declaredScopeRequirementRefs; not exhaustive impact or completion evidence

## paidSourceRecovery

```json
{
  "identity": "원 requestId/paymentEffectId/operationId와 source revision 유지",
  "holdReason": "payment_uncertain",
  "releaseOnlyOwnReason": true,
  "paymentTruthRequiredFor": [
    "grant",
    "generate",
    "publish"
  ],
  "preserveReasons": [
    "refund",
    "privacy",
    "authority"
  ],
  "deliveryResponseLossChangesPayment": false,
  "externalEffectLimit": "이미 외부 생성 요청을 전송한 뒤의 reorg는 그 요청을 취소했다고 보장하지 않는다. 결과 공개와 후속 실행은 현재 guard로 제한한다."
}
```

## paymentAttemptTransitions

### PA-01

**fromState**: required

**toState**: proof_received

**guard**: 현재 request/profile/owner와 불변 requirementDigest 검증

**effect**: attemptId와proofDigest 저장

### PA-02

**fromState**: proof_received

**toState**: settlement_pending

**guard**: 유효 proof와현재 서명권한·expiry·선정 scheme 검증

**effect**: 전송 전 exposure 예약과outbox 원자 저장

### PA-03

**fromState**: settlement_pending|unknown

**toState**: paid

**guard**: 원 attempt의 실제 canonical 지급효과와확정정책 충족

**effect**: 원 paymentEffectId로 한 번 배분; PX09/PX05 현재 guard 적용

### PA-04

**fromState**: required|proof_received

**toState**: rejected

**guard**: 검증 실패 확정 AND 이 attempt의 외부 지급 authorization/제출 노출 없음

**effect**: 검증 오류로 종료; 다른 attempt의 예약은 건드리지 않음

**requires**

```json
[
  "proof_invalid",
  "attempt_exposure_absent"
]
```

### PA-05

**fromState**: required|proof_received

**toState**: expired

**guard**: requirement/proof 기한 만료 AND 외부 노출 없음

**effect**: 미실행 만료 기록; 기발급 권한의 무효화를 추정하지 않음

**requires**

```json
[
  "deadline_expired",
  "attempt_exposure_absent"
]
```

### PA-06

**fromState**: proof_received|settlement_pending|paid

**toState**: unknown

**guard**: 출처 불가·응답 유실·reorg·상충; 노출 유무 불확실

**effect**: 원 노출 유지와payment_uncertain fence; 원 attempt 조회

### PA-07

**fromState**: unknown

**toState**: settlement_pending

**guard**: 원 attempt가 현재 pending이고 같은 요청/nonce/자산/목적지임을 확인

**effect**: 같은 payload 조회/안전 재전송만; 새지급 없음

### PA-08

**fromState**: settlement_pending|unknown

**toState**: failed_confirmed

**guard**: 원 실행의 권위있는 확정 실패·부분 지급 없음·잔여 지급 노출 해소

**effect**: 실패 증거/sourceVector와원 attempt 종료. 새승인은 별도 사용자 검토

**requires**

```json
[
  "failure_final",
  "no_partial_payment",
  "exposure_resolved"
]
```

### PA-09

**fromState**: failed_confirmed

**toState**: unknown

**guard**: 실패 근거 reorg 또는 상충

**effect**: 원 exposure와지급현재성 재평가; 과거 실패 이력 보존

### PA-10

**fromState**: unknown

**toState**: expired

**guard**: 미실행 기한 만료와이 attempt 외부 지급 노출 해소 증거

**effect**: 단순 timeout은 이 전이로 처리하지 않음

**requires**

```json
[
  "deadline_expired",
  "attempt_exposure_absent"
]
```

## paymentAttemptContract

```json
{
  "identity": [
    "environmentId",
    "requestId",
    "attemptId"
  ],
  "immutable": [
    "requirementDigest",
    "proofDigest",
    "walletRef",
    "assetRef",
    "recipient",
    "amountAtomic"
  ],
  "failureDoesNotAuthorizeRetry": true,
  "unknownExposureRetained": true,
  "uiStates": {
    "rejected": "승인 증명 확인 실패",
    "expired": "결제 요청 만료",
    "failed_confirmed": "지급 실패 확인",
    "unknown": "원 지급 확인 중"
  },
  "reorgRule": "확정 근거 변경 시 paid/failed_confirmed를 unknown으로 재검토한다. 이전 결과 이력과 stable effect identity는 보존한다."
}
```

## attemptStateAuthority

```json
{
  "payment": "paymentAttemptTransitions",
  "refund": "refundAttemptTransitions",
  "overview": "paidResourceTransitions(PX)는 여러 객체 여정의 표시 단계; 단일 persisted enum으로 사용하지 않는다."
}
```

## refundAttemptTransitions

### RF-01

**fromState**: none

**toState**: reserved

**guard**: OC21 현재 operator/signer/funding/policy/revision 및잔여한도 검증

**effect**: refundId+attemptId+한도 예약+refund fence+outbox 원자 저장

### RF-02

**fromState**: reserved

**toState**: submitted

**guard**: 등록 paid_resource_refund adapter와현재 승인·source/authority gate 확인

**effect**: 서명/전송 노출을 durable 기록. 예약은 계속 차감

### RF-03

**fromState**: submitted|unknown

**toState**: confirmed

**guard**: 원 환불의 실제 반환효과·금액·목적지·선정확정정책 확인

**effect**: reservation→confirmed 이동을 같은 원장에서 원자 반영, 중복 차감 금지

### RF-04

**fromState**: reserved|submitted|confirmed

**toState**: unknown

**guard**: 전송 여부 불명·응답 유실·환불 reorg·상충

**effect**: 노출 계속 차감; confirmed reorg는 같은 한도를 reserved로 되돌리고 history 유지

### RF-05

**fromState**: unknown

**toState**: submitted

**guard**: 원 환불 attempt가 현재 pending임을 확인

**effect**: 동일 attempt만 재조회; 새 환불 자동 생성 금지

### RF-06

**fromState**: submitted|unknown

**toState**: failed_confirmed

**guard**: 원 환불 확정 실패와부분반환 없음·노출해소 증거

**effect**: 동일 reservation만 해제. 다른 환불/원인 fence 자동 해제 금지

**requires**

```json
[
  "failure_final",
  "no_partial_refund",
  "exposure_resolved"
]
```

### RF-07

**fromState**: reserved

**toState**: cancelled_unexposed

**guard**: 서명·전송 등 외부 노출이 없음을 원천에서 확인하고 현재 취소권한 검증

**effect**: 동일 reservation만 해제. 서명노출 여부 불명이면 unknown

**requires**

```json
[
  "attempt_exposure_absent"
]
```

### RF-08

**fromState**: failed_confirmed

**toState**: unknown

**guard**: 환불 실패 근거 reorg/상충

**effect**: 이 attempt의 한도 예약 복원. 한도 초과가 되면 deficit hold 및 새 환불 금지; 이미 다른 환불을 취소했다고 표시하지 않음

## paidRefundContract

```json
{
  "sourceKind": "paid_resource_refund",
  "adapterStatus": "unregistered_execution_blocked",
  "routes": [
    "OC-21",
    "OC-22"
  ],
  "identity": [
    "environmentId",
    "requestId",
    "refundId",
    "attemptId"
  ],
  "requiredBindings": [
    "originalPaymentEffectId",
    "fundingRevision",
    "resourceOperatorScope",
    "merchantSignerRef",
    "refundPolicyRevision",
    "assetRef",
    "amountAtomic",
    "destinationProofRef",
    "expectedRequestRevision",
    "expectedRefundLedgerRevision"
  ],
  "ledgerRule": "refundableAtomic = allocatedPaidAtomic - confirmedRefundAtomic - outstandingRefundExposureAtomic. outstanding에는 reserved/submitted/unknown을 포함하고 같은 attempt의 재조회는 추가 차감하지 않는다. 단위가 다른 자산/가스를 합산하지 않는다.",
  "unknownKeepsReservation": true,
  "partialOrAmbiguousEffect": "부분 반환/추적 모호성은 unknown으로 원 노출 유지 후 실제 부분효과 대사; failed_confirmed로 잔여 전부 해제 금지.",
  "sourceDeficit": "원 지급 reorg 또는 실패근거 reorg로 잔여 한도가 음수가 되면 deficit hold. 완료 반환 이력 보존, 추가 환불 금지.",
  "fenceRelease": "환불 예약 해소는 상품 접근 복원 승인이 아니다. 선택된 제공/환불 정책·현재payment/privacy/authority guard 아래에서 해당 refund reason만 재평가한다. 미선정이면 held.",
  "policy": "D16의 목적지/금액/가스/제공 후 환불 규칙이 선택되고 해당 adapter가 채택되기 전 실행불가. 고객의 환불 요청과 판매자 지갑 서명 권한은 별개."
}
```


# 녹음·여행·AI·개인정보 수명주기 상세 설계

LG18 설계 보완 반영 · 정책 미선택 · 기준 미병합 · 제품 구현 및 실행 검증 보류

## status

design_proposal_not_merged

## packageRef

DS-07

## implementation

deferred_by_user

## canonicalMerged

False

## runtimeVerified

False

## policySelectionUnchanged

True

## requirementRefs

- 3
- 7
- 14
## taskRefs

- REC-03
- TRIP-01
- TRIP-02
- TRIP-03
- AI-01
- AI-02
- AI-03
- TRIP-04
## decisionRefs

- D07
- D17
- D18
## boundary

원음은 폰에 저장하고 서버 전송·전사·요약·추천은 각각 동의/목적별로 분리한다. 실제 구매, 테스트 결제, 위치 방문, 후기는 서로 다른 근거다. 결제 reorg는 여행 원본 사실을 삭제할 권한이 아니다.

## dataClasses

### DP-01

**data**: 원음/청크

**storage**: 폰 private 암호화 파일 우선; 명시 동의한 처리 job에만 서버 object store

**purpose**: recording/playback/transcription 개별 목적

**deletion**: 접근 차단 tombstone→active job 취소→원음 및 파생 object 삭제/키폐기 증거

**limits**: 백업/외부제공자 삭제 확인 전 전부 삭제완료 표기 금지

### DP-02

**data**: 전사/요약

**storage**: owner 보호 object, 원음/구간 ref와 model revision

**purpose**: transcription와summary 분리

**deletion**: 원음 및 파생물 scope 규칙에 따라 연쇄삭제; 보존 선택은 사용자 확인

**limits**: 요약 사실은 전사/시간구간에 연결, AI 내용은 검증된 사실 보증 아님

### DP-03

**data**: 정밀 위치

**storage**: 기본 필요시 폰 수집; 서버 전송은 목적별 동의

**purpose**: 검색/방문/코스 각각 사용

**deletion**: 위치 원본/embedding/추천입력 cache까지 scope 연결

**limits**: 연속 background 위치는 별도 선택; 위치 미허용 시 지역/장소 직접선택

### DP-04

**data**: 장소/공공 데이터

**storage**: provider namespace+placeId+version으로 cache

**purpose**: 검색/경로 계산

**deletion**: provider 계약 TTL·삭제·표시 의무 적용

**limits**: 우리 영업점과 provider place mapping 검증, 무단 후기 복제 금지

### DP-05

**data**: 후기

**storage**: 작성자/노출범위/수정 revision과 원문 보호

**purpose**: 일반 후기 또는 증거 연동 후기

**deletion**: 본인 삭제/운영 moderation 상태 구분; 검색/embedding 제거 전 노출차단

**limits**: 후기 텍스트 삭제와 법적 보관 예외는 별도 정책; 기본 공개 동의 필요

### DP-06

**data**: 구매/방문 provenance

**storage**: paymentRef/placeRef/visitRef 각 별도

**purpose**: 구매표식/발자취/혜택

**deletion**: 개인 연결 차단 및 선택 보관 규칙; 온체인 기록 제거 불가

**limits**: testnet 결제만으로 실제 구매 인증 금지

### DP-07

**data**: 코스/챌린지

**storage**: 입력 sourceVector·동의 revision·model/policy version

**purpose**: 추천/진행·보상

**deletion**: 입력 삭제/철회 시 의존 output 비공개 또는 안전한 재계산

**limits**: 사용자 수동 작성 메모와 AI 파생 부분 구분

### DP-08

**data**: 감사/삭제 증거

**storage**: 최소 식별자·scope·timestamp·처리상태; 별도 접근권한

**purpose**: 분쟁/복구시 삭제 재적용

**deletion**: 기간 정책 D19, 민감 원문 재복사 금지

**limits**: 삭제증거로 개인 기록을 재구성할 수 없도록 최소화

## recordingStages

### AR-01

**stage**: start

**guard**: 등록 기기·활성 소유자·폰 권한/저장가능·녹음표시; 다른 사용자 session 혼합 금지

**persist**: recordingId,deviceSessionId,formatProfile,codec/sampleRate/channels,startedAt,owner/consent revision; 불변 recordingCaptureContract binding과 local appContextGeneration 저장

**failure**: 권한/폰 연결 없으면 시작불가 표시; 기기에 전체 저장된다고 표시 금지

### AR-02

**stage**: stream

**guard**: 청크 session+sequence+sample offset+auth 확인; format 변경은 새 epoch; REC-CHUNK: 수신 commit 직전 불변 capture binding·현재 lease/generation·원 owner namespace 재검사

**persist**: 폰이 durable write한 sequence/offset만 ACK; 짧은 device buffer는 profile 한도; account/return 차단과 같은 native gate에서 commit 순서화, 동일 frame digest 멱등 ACK

**failure**: 누락구간 gap 기록, 재전송 가능 범위만 요청; 단절 이후 오디오를 복원했다고 주장 금지

**predicateRefs**

```json
[
  "REC-CHUNK"
]
```

### AR-03

**stage**: finish

**guard**: stop/drain 또는 disconnect/OS suspension/공간부족 원인 기록; complete 표시는 REC-COMPLETE의 인증된 종료 표식·정확한 durable 구간·현재 manifest CAS가 모두 필요

**persist**: 파일 checksum,received intervals,gaps,container finalization,completeness complete/partial/failed; captureCompleteness와 finishEvidenceState를 uploadState와 분리

**failure**: 마지막 ACK만으로 complete 불가; 실제 파일 decode/재생검증 필요; 후미 유실/종료 근거 없음은 재생 가능해도 partial

**predicateRefs**

```json
[
  "REC-COMPLETE"
]
```

### AR-04

**stage**: sync_metadata

**guard**: API051/094 owner 검증, 기기의 녹음이 다른 계정 metadata로 연결되지 않음; 불변 원 owner/capture binding을 사용하고 현재 device renter로 재귀속 금지

**persist**: durationReceived와wallDuration 구분·파일 localRef·업로드 상태

**failure**: 메타데이터 동기화가 오디오 cloud upload 동의가 아님

### AR-05

**stage**: prepare_processing

**guard**: API052 현재 목적 동의, sourceRef 소유권/완전성/파일type·size limits

**persist**: jobId,sourceDigest,ownerEpoch,consentRevision,privacyRevision,jobType,provider/model profile

**failure**: sourceRef는 arbitrary URL 금지; partial면 사용자가 확인하고 누락정보 전달

### AR-06

**stage**: process

**guard**: 외부전송 직전 동의 재검사; 녹음 내 텍스트는 명령이 아닌 입력 데이터

**persist**: segment timestamps/confidence와transcript/summary 연결; retry attempt/outbox

**failure**: 환각·화자추정은 명시, 비밀/결제/도구실행 지시를 AI가 수행하지 않음

### AR-07

**stage**: publish_export

**guard**: API053/107 현재 권한·tombstone·source 일치, signed URL도 짧은 정책범위; RB04/05 적용, 반납 이력이나 기기 소지가 현재 녹음 읽기권을 부여하지 않음

**persist**: 결과 digest·artifact graph·export 대상/동의

**failure**: 외부 export는 회수 보장 불가; 실패 재시도는 같은 job 결과 확인부터

### AR-08

**stage**: delete

**guard**: API085 삭제 scope 동결·현재 owner·recent auth·expected privacy revision

**persist**: deny/tombstone 선반영 후 device/app/server/provider별 삭제 작업과 evidence

**failure**: offline phone/외부provider미확인은 pending으로 표시, 완료를 과장하지 않음

## travelRules

### TR-01

**topic**: 장소 검색

**input**: region/query,선택 위치,provider placeId

**decision**: 권한 없으면 수동지역검색; source/observedAt/cache 상태 표시

**correction**: 폐업/이전/mapping변경은 version으로 갱신, 기존 결제 장소 스냅샷 유지

### TR-02

**topic**: 구매 후기

**input**: 본인 purchaseRef+가게/place mapping+실제 fulfill evidence

**decision**: 테스트 결제/실제구매/일반후기 배지를 구분; 타인 purchase 불가

**correction**: 환불/reorg는 구매배지 재평가; 원문 후기 자동 삭제 금지

### TR-03

**topic**: 발자취

**input**: 위치관측 source/time/accuracy,사용자선택,결제 projection

**decision**: 지불됨·방문관측됨·직접기록됨을 다른 entry로 표시

**correction**: GPS 위조/정밀도 부족이면 검증 방문 표기 제한

### TR-04

**topic**: 추천 입력

**input**: 허가된 source IDs+consentRevision+입력제약

**decision**: 장소/후기/결제 정규화, PII 최소화, 출처별신뢰/신선도 기록; itineraryGenerationContract의 원 base revision과 selectedGenerationId에 후보 결합

**correction**: source변경 시 stale; 자동 유료 regenerate 금지; 원 generation OC26 조회, 새로운 생성은 OC25의 별도 명시 요청

### TR-05

**topic**: 코스 검증

**input**: 허용 place 후보,시간대/날짜,예산/이동수단/영업시간

**decision**: LLM은 후보 ID/순서/설명 제안; deterministic validator가 운영시간/이동시간/예산/중복 검사

**correction**: 없는 장소/모르는 영업정보는 invalid/unknown, 모델 설명으로 덮지 않음

### TR-06

**topic**: 사용자 코스 편집

**input**: API081 expectedRevision+stops+constraints

**decision**: 수동편집도 동일 검증; 결과 일부 불가면 이유별 표시; IW03/04와 mode별 expectedRevision CAS로 수동 version/후보 적용을 구분

**correction**: 직전 valid코스 유지 또는 invalid초안 저장; 조용히 일정 덮어쓰기 금지; 늦은 worker는 후보만 저장, 이전 base revision 결과를 현재 코스로 자동 적용하지 않음

### TR-07

**topic**: 챌린지 증거

**input**: participant+challengeVersion+stepId+evidenceRef

**decision**: 참여자 소유·중복·기간·장소·source검사; selected rule로 step평가

**correction**: 참여 완료와 reward지급별도; 같은 source 재전송 한 번 반영; 완료 assessment와 실제 효과 원장 반영은 다른 상태이며 outbox ACK로 보상 완료 표시 금지.

### TR-08

**topic**: 혜택 보정

**input**: 현재 sourceVector+benefit ruleVersion

**decision**: 현재 sourceVector와 원 규칙으로 목표 assessment 생성; 내부 혜택 원장 적용은 CP-BENEFITS의 BENEFIT-APPLY 단일 writer 경로. 원천 불명은 hold이며 target=0으로 변환하지 않음.

**correction**: 사용후 환불 deficit은 선택규칙만 사용, 임의 잔액 음수/다른자산 인출 금지; program별 typed entitlementId를 유지하며 source/event/rebuild 변경으로 새 reward_key 생성 금지.

## privacyTransitions

### PV-01

**fromState**: active

**toState**: deny_effective

**guard**: owner 동의 철회/삭제 request 현재권한 및 scope 검증; actionKind와 purpose/object scope를 검증

**effect**: privacyRevision 증가와 지정 purpose 이용 deny+outbox 원자 저장; 해당 목적의 새 read/process/publish 금지. 개인 playback 등 다른 허가 목적은 별도 판정한다. delete_scope이면 승인된 plan의 object tombstone을 추가한다. 이미 실행된 외부 job은 추적 대상으로 보존.

### PV-02

**fromState**: deny_effective

**toState**: deleting

**guard**: deletionPlanAuthorized=true; 사용자 명시 삭제 scope 또는 선택된 보존/삭제 정책의 revision에 결합된 plan. artifact graph/backup/provider 대상은 그 scope 안에서만 확정

**effect**: 대상별 idempotent 삭제 예약, 보존예외는 별도 reason/status; 단순 동의 철회만으로 전체 원음 삭제를 예약하지 않음

**predicateRefs**

```json
[
  "PV-DELETE"
]
```

### PV-03

**fromState**: deleting

**toState**: partially_deleted

**guard**: 일부 저장소 완료, offline/외부provider/backup 처리 미완료

**effect**: 완료대상/미완료대상 구분·retry, 원문 로그 금지

### PV-04

**fromState**: deleting|partially_deleted

**toState**: completed

**guard**: 정의된 scope 모든 대상의 삭제/보존예외 증거 수집; PV-COMPLETE 필수: 현재 targetSetRevision/deletionRevision CAS, 발견 queue 비어있고 알려진 producer job이 종료 또는 승인된 보존예외로 처리됨

**effect**: 범위와 예외를 표시한 완료 receipt; 체인/사용자외부복사 삭제 약속 금지; completedAtTargetRevision과증거digest를 불변 receipt에 기록하며 현재 완료상태와과거 완료이력을 분리한다.

**predicateRefs**

```json
[
  "PV-COMPLETE"
]
```

### PV-05

**fromState**: deny_effective|deleting|partially_deleted|completed|purpose_blocked

**toState**: deny_effective

**guard**: 늦은 job/event 또는 오래된 backup restore

**effect**: 최신 tombstone 우선 적용; late artifact를 원 job/source 삭제 scope에 등록하고 대상 추가·삭제 증거 갱신. completed 뒤 도착해도 새 결과 비공개+추가삭제 추적. 새동의로 과거 object 부활 금지; 대상 추가는 원 삭제 plan의 범위 또는 명시 승인된 정책 범위 안에서만. purpose 철회만 있는 경우 늦은 결과는 비공개 격리 후 선택된 처리 정책으로 처리하며 다른 목적의 원음을 임의 삭제하지 않음.; 원 scope에 속한 late target을 추가할 때 targetSetRevision 증가+pending 전환+completion 무효화를 같은 deletionRevision CAS로 반영한다. stale 완료 worker는 재조회해야 하며, 이전 완료 receipt 자체는 이력으로 보존한다.

### PV-06

**fromState**: deny_effective

**toState**: purpose_blocked

**guard**: actionKind=revoke_purpose이고 승인된 deletionPlan 없음

**effect**: 목적별 이용 차단을 유지하며 기존 독립 playback 권한과 원음은 보존; 물리 삭제를 완료했다고 표시하지 않음

### PV-07

**fromState**: purpose_blocked

**toState**: active

**guard**: 사용자가 해당 목적에 새 동의; expectedPrivacyRevision CAS; source에 object tombstone/삭제 plan 없음 및 현재 소유권/목적 정책 유효

**effect**: 해당 목적의 새 작업 허용만 부여. 철회 당시 job/outbox를 자동 재개하지 않으며 새로운 사용자 요청과 입력/source revision이 필요; 삭제된 object는 복원하지 않음

## logicalContracts

### TC-01

**name**: RecordingManifest

**identity**: owner+recordingId+deviceSessionId

**fields**: formatProfile,expectedIntervals,receivedIntervals,gaps,byteLength,checksum,completeness,localRef,storageState,privacyRevision,captureBindingDigest,rentalId,bindingRevision,deviceEpoch,streamGeneration,formatEpoch,manifestRevision,endMarkerRef,endSampleExclusive,finishEvidenceState

**constraints**: 오디오 임시 buffer보다 큰 복구 보장 금지; session간 seq 혼합 거절; REC-CHUNK/REC-COMPLETE 및 RB01~05 적용. localRef는 owner namespace의 내부 참조이며 다른 계정으로 전달할 수 있는 읽기 capability가 아니다.

**existingApiRefs**

```json
[
  "API-051",
  "API-094"
]
```

### TC-02

**name**: ProcessingJob

**identity**: owner+recordingId+jobType+sourceDigest+profileVersion+consentRevision

**fields**: jobId,attempt,sourceRef,sourceVector,ownerEpoch,privacyRevision,state,resultRef,errorCode

**constraints**: idem key 동일 내용일 때 반환, 새전사와 재조회 구분; billing exposure 별도 추적

**existingApiRefs**

```json
[
  "API-052",
  "API-053",
  "API-107"
]
```

### TC-03

**name**: TravelEvidence

**identity**: owner+evidenceId+revision

**fields**: kind=location|manual|test_payment|real_purchase|review,placeBinding,occurredAt,observedAt,sourceRef,accuracy,consentRevision

**constraints**: 증거 종류간 승격은 명시 검증필요; test label immutable

**existingApiRefs**

```json
[
  "API-074",
  "API-078",
  "API-079",
  "API-083"
]
```

### TC-04

**name**: ItinerarySnapshot

**identity**: owner+itineraryId+revision

**fields**: tripId,sourceVector,constraints,placeVersions,modelProfile,stops,validationReport,consentRevision,privacyRevision,selectedGenerationId,baseItineraryRevision,candidateId,resultDigest,validationRevision,selectedPlanRef,applyReceiptRef

**constraints**: 각 stop 근거/영업시간/이동/비용에 valid|invalid|unknown; 자동결제권 없음; IW01~06/ITINERARY-APPLY 적용. 생성 성공·유효 후보·현재 적용 완료는 별개 상태.

**existingApiRefs**

```json
[
  "API-080",
  "API-081",
  "API-095"
]
```

### TC-05

**name**: ChallengeLedger

**identity**: participant+challengeVersion+stepId+ruleVersion

**fields**: evidenceRefs,sourceVector,validity,completionRevision,rewardEffectRef,correctionState,benefitProgramId,entitlementSourceKind,entitlementSourceId,rewardSlotId,occurrenceId,assessmentRevision,assessmentDigest,appliedEffectRevision,rewardApplicationState

**constraints**: step 중복과 reward 중복 별도 unique; 재조정은 기존 원장 history 보존; CP-TRAVEL은 assessment만 생성, CP-BENEFITS의 현재 effect revision 확인 전 보상 지급 완료 표시 금지. 서로 다른 program의 정당한 혜택은 별도로 유지.

**existingApiRefs**

```json
[
  "API-082",
  "API-083",
  "API-096"
]
```

### TC-06

**name**: DataRequest

**identity**: owner+requestId

**fields**: type,scopeSnapshot,privacyRevision,artifactGraphRef,targetStatuses,exceptionReasons,evidenceRefs,actionKind,purposeRefs,artifactRefs,deletionPlanRef,deletionAuthorizationRevision,targetSetRevision,deletionRevision,completedAtTargetRevision,producerClosureEvidenceRefs

**constraints**: 접근차단과 물리삭제 상태 구분; legal exception은 승인된 정책 없으면 임의추가 금지; revoke_purpose와delete_scope 구별, 새 동의로 삭제 취소/부활 금지; PV-COMPLETE CAS 없는 완료 projection 금지

**existingApiRefs**

```json
[
  "API-084",
  "API-085"
]
```

## evaluationSamples

### AE-01

**sample**: 위치 권한 거절

**expected**: 지역 직접선택으로 장소검색 가능, 몰래 위치사용 없음

### AE-02

**sample**: 존재하지 않는 장소 ID

**expected**: 코스 valid 처리 거절

### AE-03

**sample**: 영업시간 누락

**expected**: unknown 표시와 확인 필요; 영업중이라고 생성 금지

### AE-04

**sample**: 마감시간 이후 도착

**expected**: 이동+방문시간 검증으로 invalid

### AE-05

**sample**: 예산 초과/환율 미확인

**expected**: amount units와 source확인, 초과/unknown 분리

### AE-06

**sample**: 후기 속 prompt injection

**expected**: 권한 변경·결제·외부도구 호출 없이 입력 데이터로 처리

### AE-07

**sample**: 테스트 결제만 있는 사용자

**expected**: 실제 구매 기반이라고 표시하지 않음

### AE-08

**sample**: 입력 삭제 후 job완료

**expected**: 결과 비공개, 파생물 삭제 queue

### AE-09

**sample**: 동일 방문 반복 제출

**expected**: 선택 step 규칙에 따라 1개 효과

### AE-10

**sample**: 언어/시간대 전환

**expected**: placeID/금액/UTC근거 유지, locale로 검증의미 변경 없음

### AE-11

**sample**: 결제 reorg

**expected**: 구매표식/혜택 보정, 독립 위치방문 유지

### AE-12

**sample**: 데이터 부족

**expected**: 부족 사유와 수동코스/공개장소 기반 대안, 가짜후기/결제 생성 없음

## policyInputs

### TP-01

**decisionRef**: D07

**needed**: codec/sampleRate/length/buffer/background별 녹음 목표

**selection**: None

### TP-02

**decisionRef**: D17

**needed**: 지도/장소/후기 provider·이용조건·캐시/삭제

**selection**: None

### TP-03

**decisionRef**: D17

**needed**: 목적별 동의/보관기간/공개범위·실제구매 증거

**selection**: None

### TP-04

**decisionRef**: D18

**needed**: 코스 제약/평가 표본 합격률 및 방문·부정참여 규칙

**selection**: None

### TP-05

**decisionRef**: D18

**needed**: 보상사용 후 증거철회 처리

**selection**: None

### TP-06

**decisionRef**: D19

**needed**: STT/AI provider·model·외부처리 지역·삭제증거·운영 보관

**selection**: None

## runtimeCases

### TQ-01

**scenario**: 정상 기기 녹음→폰 재생→전사→요약

**expected**: 구간/출처 연결과 명시동의 확인

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-02

**scenario**: BLE 청크 중복/역순

**expected**: 중복저장 없이 seq/sample offset 복구

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-03

**scenario**: BLE 단절 buffer 초과

**expected**: 실제 gap 있는 partial 파일

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-04

**scenario**: 폰 강제종료/공간부족

**expected**: 복구한 durable구간만 재생·누락표시

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-05

**scenario**: 녹음 중 계정 전환

**expected**: 기존 소유자와 파일 격리, 새계정 목록 유출 없음

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-06

**scenario**: 메타데이터 등록만

**expected**: cloud 원음 자동 업로드 없음

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-07

**scenario**: 악성 sourceRef URL

**expected**: 임의 host/다른 owner 파일 접근 거절

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-08

**scenario**: partial 원음 전사

**expected**: 누락구간 표시·원음없는 문장 근거생성 금지

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-09

**scenario**: 전사 성공 응답 유실

**expected**: 같은 job 결과 조회; 이중과금 점검

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-10

**scenario**: 삭제와 publish 경합

**expected**: 최신 tombstone 우선

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-11

**scenario**: 백업 복원 후 삭제자료

**expected**: 외부 공개 전 삭제원장 replay

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-12

**scenario**: 외부 provider 삭제 미확인

**expected**: partial/pending 표기

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-13

**scenario**: 지도 권한 거절

**expected**: 수동검색 정상

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-14

**scenario**: 타인 구매 후기

**expected**: 구매표식 거절

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-15

**scenario**: 점포 mapping 변경

**expected**: 과거 provenance 보존

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-16

**scenario**: 정상 코스 생성/편집

**expected**: 근거와 시간/예산 검증

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-17

**scenario**: 없는 장소/불가능 이동

**expected**: invalid

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-18

**scenario**: LLM prompt injection

**expected**: 데이터 취급, signer/운영 권한 없음

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-19

**scenario**: 동일 방문 여러번 증거

**expected**: 중복 보상 없음

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-20

**scenario**: 테스트지급 실제구매 오표기

**expected**: 서로 구분

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-21

**scenario**: 결제취소 후 독립 위치방문

**expected**: 방문유지·지급혜택만 보정

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-22

**scenario**: 원문후기 삭제 후 embedding

**expected**: cache/추천 파생물 차단

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-23

**scenario**: 동의철회 후 외부전송 대기

**expected**: 전송하지 않음

**status**: not_run

**evidenceRefs**

```json
[]
```

### TQ-24

**scenario**: 오프라인폰 삭제요청

**expected**: 서버 deny 즉시·폰삭제 확인은 pending

**status**: not_run

**evidenceRefs**

```json
[]
```

## externalReferences

## declaredScopeRequirementRefs

- 3
- 7
- 14
## transitiveTaskImpactRequirementRefs

- 3
- 4
- 7
- 14
## requirementRefsMeaning

legacy alias of declaredScopeRequirementRefs; not exhaustive impact or completion evidence

## privacyActionContract

```json
{
  "actions": [
    "revoke_purpose",
    "delete_scope"
  ],
  "scopeIdentity": [
    "ownerRef",
    "purposeRefs",
    "artifactRefs",
    "scopeRevision"
  ],
  "deletionAuthorization": "explicit_user_scope_or_selected_policy_plan",
  "unknownPolicy": "차단/격리는 유지하되 범위 불명인 파괴적 삭제를 실행하지 않는다. 외부 처리/보관 제한이 필요하면 해당 profile 사용도 보류한다.",
  "irreversible": "object tombstone/삭제 plan은 새 consent로 해제 불가. 삭제 후 새로 수집한 자료는 새 artifact ID와새동의에 결합한다.",
  "lateArtifact": "원 job/source와 해당 삭제 plan의 관계를 기록하고 원 scope에 포함된 파생물만 삭제 대상으로 추가한다. 삭제 증거는 target별 갱신한다."
}
```

## privacyPredicates

### PV-DELETE

**allOf**

```json
[
  "deletion_plan_authorized",
  "target_within_plan_scope",
  "owner_and_policy_revision_current",
  "privacy_revision_matches"
]
```

**effect**: 실제 삭제 queue에 넣기 전 동일 plan/revision으로 검사

### PV-COMPLETE

**allOf**

```json
[
  "all_targets_terminal_with_evidence",
  "target_set_revision_matches",
  "deletion_revision_matches",
  "discovery_queue_empty",
  "producer_closure_accounted"
]
```

**effect**: 동일 CAS에서 현재 완료 projection+불변 receipt 저장. 조건 미충족은 deleting/partially_deleted 유지. 전역 외부복사 완전삭제 보장이 아니라 명시 scope/revision의 완료다.

## deletionCompletionContract

```json
{
  "scopeKey": [
    "ownerRef",
    "requestId",
    "deletionPlanRef"
  ],
  "casFields": [
    "targetSetRevision",
    "deletionRevision"
  ],
  "lateTargetAtomicWrites": [
    "register_target_with_source",
    "increment_target_set_revision",
    "mark_completion_pending",
    "increment_deletion_revision"
  ],
  "receiptFields": [
    "scopeDigest",
    "targetSetRevision",
    "completedAt",
    "evidenceDigest",
    "explicitExceptionRefs"
  ],
  "producerRule": "이미 알려진 외부 job/폰/저장소가 종료되거나 선택된 정책상 명시 예외로 증거화되기 전 완료 금지. 단순 timeout/연락불가를 승인된 예외로 바꾸지 않는다.",
  "dedup": "같은 artifact identity 재통지는 idempotent, 새 target을 중복등록하지 않는다. 기존 target의증거/상태가바뀌면 deletionRevision을증가시킨다.",
  "history": "새 late target은 현재 완료 표시를 pending으로 바꾸되 과거 scope/revision 완료 receipt를 지우지 않는다. known scope 밖 데이터는 격리/범위확인 후 처리한다."
}
```

## recordingPredicates

### REC-CHUNK

**allOf**

```json
[
  "capture_binding_matches",
  "authenticated_frame",
  "receive_lease_current",
  "capture_generation_matches",
  "recording_purpose_allowed",
  "capture_phase_accepts_frames",
  "frame_identity_consistent",
  "local_owner_namespace_matches"
]
```

**effect**: 현재 수신 gate와 동일 직렬화 경계에서 검증된 원 owner namespace로만 durable 저장 후 ACK. ACK는 저장된 정확한 frame identity/digest에 결합한다.

### REC-COMPLETE

**allOf**

```json
[
  "authenticated_end_marker",
  "end_binding_matches",
  "exact_durable_interval_coverage",
  "frame_integrity_consistent",
  "no_unresolved_gaps",
  "container_finalized",
  "decode_verified",
  "manifest_revision_matches"
]
```

**effect**: 같은 manifest revision CAS에서 captureCompleteness=complete 기록. 업로드/처리/현재 읽기권은 별도 축이다.

## recordingCaptureContract

```json
{
  "immutableBinding": [
    "ownerRef",
    "deviceId",
    "rentalId",
    "bindingRevision",
    "deviceEpoch",
    "deviceSessionId",
    "recordingId",
    "formatEpoch",
    "streamGeneration"
  ],
  "frameIdentity": [
    "recordingId",
    "streamGeneration",
    "formatEpoch",
    "sequence",
    "sampleOffset",
    "sampleCount",
    "payloadDigest"
  ],
  "wireStatus": "logical_candidate_not_registered",
  "wireRule": "기존 recording.start/stop/audio.frame/flow-control의 의미 확장 후보. 각 식별자·payload·종료 표식의 인증 방식/인코딩은 선택된 보호 transport profile에 결합한다. 현재 BLE 카탈로그가 이 wire를 구현했다고 주장하지 않는다.",
  "localContext": "appContextGeneration은 callback/UI 수용을 위한 로컬 값이며 서버의 owner 권한 근거가 아니다. accountId나 현재 선택 계정으로 capture owner를 교체하지 않는다.",
  "receiveGate": "수신 commit과 계정 전환·로그아웃·관측된 반납/권한 철회는 동일 native capture gate/streamGeneration에서 순서화한다. 차단이 먼저면 staged 청크를 durable 녹음으로 승격하거나 ACK하지 않는다. 이미 저장된 청크는 원 owner에게만 남는다.",
  "duplicateRule": "같은 frame identity와 digest 재전송은 한 번 저장하고 기존 durable ACK만 반환. 동일 sequence의 다른 offset/payload 또는 충돌하는 겹침은 무결성 오류로 스트림 보류; 새 계정/새 파일에 우회 저장하지 않는다.",
  "remoteRevocation": "오프라인 기기에 원격 철회가 즉시 도달했다고 표시하지 않는다. 폰이 관측한 차단과 기기 중단 ACK는 별도 증거. lease freshness/expiry 기준은 profile 선택과 실기 검증 대상이며 만료·현재성 불명 시 새 수신을 허용하지 않는다.",
  "reconnect": "끊긴 owner transport 또는 계정/대여 경계 이후에는 fresh start와 새 recordingId/streamGeneration이 필요하다. 기존 파일을 새 owner에 붙이거나 숨은 자동 재녹음을 하지 않는다.",
  "ephemeralCleanup": "수신 중단 후 기기의 RAM ring·DMA·전송 queue는 원 스트림 경계에서 정리한다. 정리 증거 없이 다른 대여자에게 buffer를 재사용하지 않는다. 원음 저장된 폰 파일의 삭제는 별도 승인된 삭제 scope를 따른다."
}
```

## recordingBoundaryRules

### RB-01

**event**: account_switch_or_logout

**rule**: 폰 수신 gate 차단·streamGeneration 무효화·old callback/UI 배제부터 적용하고 owner transport를 닫는다. 이미 durable 저장된 파일의 소유자는 유지하고 이전 계정 private namespace를 잠근다.

**predicateRef**: REC-CHUNK

### RB-02

**event**: rental_return_or_binding_change

**rule**: 기존 반납 권한·초기화 절차와 현재 epoch를 따른다. 관측된 경계 후 이전 녹음 수신 금지. 기기 재사용은 원 audio buffer/queue 처리 증거까지 확인해야 하며, 다음 대여자가 이전 파일 읽기권을 얻지 않는다.

**predicateRef**: REC-CHUNK

### RB-03

**event**: purpose_revocation

**rule**: 녹음 목적 자체 철회만 새 캡처/수신을 차단한다. transcription/summary 목적만 철회한 경우 해당 처리·공개를 차단하되 별도 허용된 로컬 원음 재생/녹음을 자동 삭제·중단하지 않는다.

**predicateRef**: REC-CHUNK

### RB-04

**event**: historical_recording_read

**rule**: 반납 후에도 원 owner의 현재 account 또는 로컬 unlock 권한·목적·privacy/tombstone으로 판정한다. 현재 기기 소지·새 rental·과거 결제 eligibility만으로 녹음 접근 불가. 운영자/점주의 매출 읽기권은 녹음 읽기권이 아니다.

### RB-05

**event**: late_worker_or_local_callback

**rule**: 원 owner/job/source binding으로 결과 관측만 저장하고 현재 공개 권한은 별도 검사한다. 새 계정 화면/검색 cache/미리보기/업로드 queue로 결과를 재귀속하지 않는다. owner 연결 불명은 비공개 보류하며 민감 원문을 로그에 넣지 않는다.

## recordingCompletionContract

```json
{
  "predicateRef": "REC-COMPLETE",
  "endMarkerFields": [
    "captureBindingDigest",
    "formatEpoch",
    "streamGeneration",
    "endSequence",
    "endSampleExclusive",
    "stopReason",
    "authenticatedMarkerDigest"
  ],
  "coverageRule": "검증된 frame의 sample 구간을 정렬·중복 제거한 union이 [0,endSampleExclusive)와 정확히 일치해야 한다. 범위 밖 frame, 충돌하는 중복/겹침, sequence/offset 모순은 complete 불가. endSequence도 마지막 frame과 대조한다.",
  "terminalRule": "종료 표식 수신만으로 freeze하지 않는다. 동일 수신 lease의 허용된 drain 범위 내 누락 청크를 받은 뒤 최종 manifest revision CAS. freeze 이후 파일/manifest는 불변이며 늦은 다른 bytes를 덧붙이지 않는다.",
  "missingTerminal": "정상 재생되더라도 종료 범위 미확인 또는 수신 gap이 있으면 partial; 사용할 수 있는 원음이 없거나 무결성 불량으로 재생 불가면 failed. 종료 표식 확인 대기는 finishEvidenceState=pending으로 표현하고 complete를 추정하지 않는다.",
  "accountBoundary": "계정 전환/반납 차단 전에 durable 저장한 청크·종료 증거만 원 owner 파일의 제한된 마무리에 사용한다. 마무리 write 권한은 녹음 재개·원음 읽기·새 계정 공개 권한이 아니다.",
  "separateAxes": [
    "captureCompleteness",
    "finishEvidenceState",
    "uploadState",
    "processingState",
    "currentReadAuthority"
  ],
  "uploadCannotUpgradeCapture": true,
  "partialProcessing": "partial 파일 처리 요청은 사용자 확인과 gap/확인 불가 구간 정보를 결합한다. 서버 upload 완료는 전송 대상 bytes의 저장 성공이며 원 녹음의 완전성 증거가 아니다."
}
```

## benefitEffectBoundary

```json
{
  "source": "content/specifications/commerce-consumer-repair-design.json",
  "contractPointer": "/benefitEffectOwnership",
  "predicateRef": "BENEFIT-APPLY",
  "completionIsReward": false,
  "unknownTargetIsZero": false,
  "originalVisitOrReviewDeletionAllowed": false
}
```

## itineraryPredicates

### ITINERARY-APPLY

**allOf**

```json
[
  "current_owner_authority",
  "current_privacy_and_consent",
  "source_vector_current_and_complete",
  "base_revision_matches",
  "selected_generation_matches",
  "candidate_digest_matches_review",
  "validation_current_and_valid",
  "candidate_not_applied_or_discarded",
  "explicit_user_apply",
  "expected_head_revision_matches"
]
```

**effect**: 동일 itinerary head CAS에서 불변 새 version·selectedPlanRef·적용 receipt·outbox를 기록. worker write 권한은 apply 권한이 아니다.

### ITINERARY-EDIT

**allOf**

```json
[
  "current_owner_authority",
  "current_privacy_and_consent",
  "expected_head_revision_matches",
  "edit_payload_validated"
]
```

**effect**: 새 수동 version 저장과 head revision 증가. 유효하지 않은 일정은 초안으로만 보존하고 이전 selectedPlanRef를 덮지 않는다.

## itineraryGenerationContract

```json
{
  "routes": [
    "OC-25",
    "OC-26"
  ],
  "identity": [
    "environmentId",
    "ownerRef",
    "itineraryId",
    "generationRequestId"
  ],
  "immutableInputs": [
    "baseItineraryRevision",
    "constraintsDigest",
    "sourceManifestRef",
    "sourceVector",
    "consentRevision",
    "privacyRevision",
    "modelProfileVersion",
    "validationProfileVersion",
    "requestDigest",
    "paidRequestRef_if_applicable",
    "acceptedGenerationSelectionRevision"
  ],
  "candidateIdentity": [
    "generationRequestId",
    "candidateId",
    "resultDigest"
  ],
  "initialCreate": "API080 후보 확장: owner/trip에 빈 draft itineraryId와 원 generationRequestId/base revision을 영속 예약 후 job 발행. worker는 candidate를 기록한다. 초기 추천도 확인된 사용자 적용 전 확정 코스로 표시하지 않는다.",
  "regenerate": "OC25는 같은 itinerary의 expected base revision으로 별도 명시 요청을 시작하고 selectedGenerationId를 갱신한다. 기존 코스는 유지한다. 같은 requestId/digest는 원 작업 조회, 다른 digest는 conflict. source 변경은 자동 재생성/새 과금 승인이 아니다.",
  "observation": "등록 worker는 원 generation/source/result digest를 검증한 불변 candidate를 저장한다. 다른 digest의 같은 candidate identity는 quarantine. 늦은 worker가 itinerary head, selectedPlanRef, selectedGenerationId를 변경할 수 없다.",
  "independentAxes": [
    "generationExecution",
    "candidateValidation",
    "candidateApplicability",
    "itineraryHeadRevision",
    "selectedPlanRef"
  ],
  "applicabilityStates": [
    "current",
    "stale_base",
    "superseded",
    "source_stale",
    "privacy_denied"
  ],
  "statusRule": "실행 succeeded는 결과 생성 사실이며 적용 완료가 아니다. candidateValidation valid도 현재 applicability가 허용됨을 의미하지 않는다. 결과 read/release는 현재 parent/owner/privacy gate를 별도로 검사한다.",
  "responseLoss": "원 generationRequestId/operation과 유료 요청이 있으면 paidRequestRef를 재조회한다. timeout은 확정 실패가 아니며 새 job/유료 요청/서명을 자동 생성하지 않는다.",
  "profileStatus": "logical_candidate_not_registered; generation/result typed parent/ACL/worker registry와 정확한 wire schema 채택 전 실행 불가",
  "selectionRevisionRule": "코스 내용의 itineraryHeadRevision과 generationSelectionRevision은 별개다. generation 예약은 내용 revision을 바꾸지 않고 expectedGenerationSelectionRevision CAS로 선택 generation만 갱신한다. apply는 둘의 현재값을 같은 경계에서 검사한다. idempotent 재시도는 선택 pointer를 다시 쓰지 않는다."
}
```

## itineraryApplyContract

```json
{
  "existingApiRef": "API-081",
  "candidateModes": [
    "manual_edit",
    "apply_candidate"
  ],
  "manualFields": [
    "stops",
    "constraints",
    "expectedRevision",
    "saveAsDraft"
  ],
  "applyFields": [
    "candidateId",
    "generationRequestId",
    "baseItineraryRevision",
    "expectedRevision",
    "reviewedCandidateDigest",
    "validationRevision",
    "applyRequestId"
  ],
  "applyPredicateRef": "ITINERARY-APPLY",
  "editPredicateRef": "ITINERARY-EDIT",
  "separateBranches": "mode별 discriminated 후보. apply_candidate는 서버의 원 candidate bytes를 사용하며 다른 stops를 함께 보내 혼합하지 않는다. 수동 편집은 별도 검증된 manual_edit이다. 현재 API081의 기준 wire를 변경 완료했다고 주장하지 않는다.",
  "revisionRule": "수동 초안 저장도 itinerary head revision을 증가시켜 이전 base의 AI 적용을 충돌 처리한다. 과거 selected plan은 불변 version ref로 유지하고 초안·선택 코스·최신 검증 상태를 구분 표시한다.",
  "conflict": "수동 편집 또는 다른 적용이 먼저 반영되면 stale_base/revision conflict. 조용한 병합·expectedRevision 자동 갱신·다른 candidate 대체 금지. 사용자가 다시 검토하여 명시 재생성을 요청할 수 있다.",
  "receipt": "applyRequestId+requestDigest의 결과는 적용한 candidate/appliedAtRevision을 가리키는 불변 receipt. 재전송은 현재 읽기권을 검사해 원 receipt를 반환하고 현재 코스를 다시 쓰지 않는다. 화면의 현재 코스는 API095/currentRevision으로 따로 조회하며 늦은 성공 응답으로 되돌리지 않는다.",
  "staleness": "영업시간/장소 mapping/결제·환불 근거/동의 변경은 현재 validity/applicability를 재판정한다. 이미 선택한 코스의 사용자 텍스트와 일정 이력을 임의로 지우거나 새 AI 코스로 교체하지 않는다. privacy deny는 현재 공개 차단에 우선한다.",
  "validation": "invalid/unknown은 확정 적용 불가. 원 candidate를 수정하려면 새 manual draft와 검증으로 처리하며 AI 설명만으로 unknown을 valid로 바꾸지 않는다."
}
```

## itineraryWriteActions

### IW-01

**action**: start_generation

**writer**: current_owner_request

**guard**: 원 base revision과 request digest 검증

**effect**: 원 job과 후보 생성만 예약, 현재 selected plan 불변

### IW-02

**action**: record_result

**writer**: registered_worker

**guard**: 원 generation/context/digest 검증

**effect**: 원 불변 candidate 저장; current head/selected plan 변경 없음

### IW-03

**action**: manual_edit

**writer**: current_owner_request

**predicateRefs**

```json
[
  "ITINERARY-EDIT"
]
```

**guard**: ITINERARY-EDIT

**effect**: head revision 증가와 검증된 수동 version; invalid 초안은 기존 selected plan 유지

### IW-04

**action**: apply_candidate

**writer**: current_owner_request

**predicateRefs**

```json
[
  "ITINERARY-APPLY"
]
```

**guard**: ITINERARY-APPLY

**effect**: 원 candidate의 새 itinerary version과 적용 receipt를 CAS 기록

### IW-05

**action**: source_invalidation

**writer**: verified_source_reconciler

**guard**: 현재 source/privacy revision과 정확한 의존 관계

**effect**: validity/applicability 갱신, 현재 privacy deny 적용; 새 유료 생성/일정 덮어쓰기 없음

### IW-06

**action**: retry_read

**writer**: current_scoped_reader

**guard**: OC26/OC16의 현 parent/owner/expiry/privacy 검증

**effect**: 원 실행/candidate/적용 receipt를 구분 조회; 새 job·적용·결제 없음


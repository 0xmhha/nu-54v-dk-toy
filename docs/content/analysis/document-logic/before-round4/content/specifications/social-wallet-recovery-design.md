# 소셜 로그인·두 지갑·MPC 복구 연결 설계

2026-09-19 · LG-01~05 설계 보완 반영 · 정책 미선택 유지 · 기준 API 미등록/제품 구현 보류 · 런타임 미검증

## status

design_proposal_not_merged

## packageRef

DS-02

## implementation

deferred_by_user

## canonicalMerged

False

## runtimeVerified

False

## policySelectionUnchanged

True

## requirementRefs

- 4
- 5
- 6
- 8
## taskRefs

- APP-01
- APP-02
- APP-03
- AUTH-01
- AUTH-02
- AUTH-03
- AUTH-04
- BASE-06
- MPC-01
- MPC-02
- MPC-03
- MPC-04
- MPC-05
## decisionRefs

- D01
- D02
- D03
- D04
- D08
- D11
- D12
- D15
- D16
- D19
## selections

```json
{
  "providerSdk": null,
  "mpcProvider": null,
  "threshold": null,
  "participantOperators": null,
  "recoveryEvidenceProfile": null,
  "hwCloudAddressPolicy": null,
  "sessionTtl": null,
  "recoveryDelay": null
}
```

## authTransitions

### AU-01

**fromState**: flow_absent

**toState**: flow_open

**route**: API-103

**guard**: 등록 provider/client/redirect/profile, purpose=login/link 분리; login은 비로그인 진입 가능, link는 현재 계정 recent auth

**durableBoundary**: flowId/purpose/initiator/sessionGeneration/nonce·state 검증값/expiry를 저장

**uiLabel**: 인증 진행

### AU-02

**fromState**: flow_open

**toState**: proof_verifying

**route**: API-001/API-002

**guard**: flow 소유·단일소비 reservation, 해당 provider proof·요청 digest 검증

**durableBoundary**: provider 외부 통신 동안 DB lock 유지 금지; result_unknown과 failed 분리

**uiLabel**: 로그인 확인 중

### AU-03

**fromState**: proof_verifying

**toState**: session_active

**route**: API-001

**guard**: 서명/issuer/audience/expiry/flow nonce 등 선택 profile 검증 후 issuer+subject로 조회

**durableBoundary**: flow consume와 유일 identity/account/session 결과를 원자 반영; 사적 지갑 권한은 따로 조회

**uiLabel**: 로그인 완료·지갑 준비 상태 별도

### AU-04

**fromState**: proof_verifying

**toState**: identity_linked

**route**: API-002

**guard**: 대상 account/session/revision 재검사 + 새 provider의 현 증명; identity가 다른 account에 속하면 conflict

**durableBoundary**: identity unique constraint + flow consumed + audit/outcome 함께 반영; 계정 자동 merge 없음

**uiLabel**: 로그인 수단 연결 완료

### AU-05

**fromState**: proof_verifying

**toState**: restart_required

**route**: API-001/API-002

**guard**: code 소비 여부 불명이고 안전하게 저장된 결과 없음

**durableBoundary**: 실패로 재사용하지 말고 새 flow. 이미 기록된 결과는 원 flow sender 증명으로만 복구

**uiLabel**: 로그인을 다시 시작해 주세요

### AU-06

**fromState**: session_active

**toState**: session_active

**route**: OC-17/OC-18

**guard**: current refresh family/generation + 선택된 sender binding 검증

**durableBoundary**: 회전 결과와 generation CAS; 보호 결과만 보관. 이전 bearer만으로 최신 token 공개 금지; preimplementation-contract-overlay의 OC17 회전과 OC18 현재권한 결과조회 계약 적용

**uiLabel**: 세션 갱신

### AU-07

**fromState**: session_active

**toState**: local_context_switched

**route**: local_app

**guard**: 개인/매장/account context 명시 선택. 다른 account는 그 account의 유효 session 필요

**durableBoundary**: old view generation 폐기, native 요청/화면 정리·participant 잠금; 새 context 권한 재조회

**uiLabel**: 선택 계정/매장으로 전환

### AU-08

**fromState**: session_active

**toState**: session_revoked

**route**: API-003

**guard**: 현재 actor가 대상 session을 종료할 권한 확인; 전체세션 종료는 별도 미등록 계약

**durableBoundary**: family/session revoke revision 및 scope별 outbox; 미확인 offline전달은 pending

**uiLabel**: 이 앱 로그아웃; 잔여 기기 제한은 별도

### AU-09

**fromState**: identity_linked

**toState**: unlink_pending

**route**: OC-19/OC-20

**guard**: recent auth + 잔존 로그인/복구 경로 검증. 마지막 수단은 단순 클릭으로 제거하지 않음

**durableBoundary**: identity revision/관련 flow·session 영향 검토. provider unlink와 share 삭제 분리; account-level CAS로 병렬 마지막수단 제거 방지; 결과조회는 OC20

**uiLabel**: 로그인 수단 해제 확인 필요

### AU-10

**fromState**: session_active

**toState**: account_restricted

**route**: unregistered_security_event

**guard**: 검증된 provider 보안 통지/관리 정책·현재 account revision

**durableBoundary**: 영향 session과 sensitive authority에 reason별 hold, 다른 provider가 있다고 자동 해제 금지

**uiLabel**: 계정 보호 조치·별도 복구 필요

### AU-11

**fromState**: session_active

**toState**: deletion_pending

**route**: API-004

**guard**: recent auth + 자산 접근/대여/Cloud/HW/영수증·데이터 보관 영향 확인

**durableBoundary**: acknowledgedWalletRecovery boolean은 증거 아님. 검증된 독립 접근/처리 계획 전 파괴적 삭제 금지

**uiLabel**: 탈퇴 검토 중; 키 삭제 완료 아님

### AU-12

**fromState**: unlink_pending

**toState**: identity_unlinked

**route**: OC-19/OC-20

**guard**: 현재 recent-auth·잔존접근증명·account/identity revision CAS, 제거대상 identity 동일

**durableBoundary**: local unlink/flow·session영향/audit/outbox 원자 저장; 외부provider revoke 결과는 별도 pending 상태

**uiLabel**: 로그인 수단 해제 결과·외부 처리 상태 별도

## mpcTransitions

### MP-01

**fromState**: absent

**toState**: enrolling

**route**: API-014

**guard**: 최근 계정 인증 + 별도 participantEnrollmentProof + 선택 policy/profile

**durableBoundary**: account/createRequestId/requestDigest와 protocolSession 하나 예약; identity 로그인만으로 사용자 share 발급 금지

### MP-02

**fromState**: enrolling

**toState**: dkg_pending

**route**: IF-11

**guard**: 등록 참가자·역할·독립 신뢰영역·protocol version 검증

**durableBoundary**: DKG transcript를 같은 session에 결합; partial share/주소는 사용 가능으로 공개하지 않음

### MP-03

**fromState**: dkg_pending

**toState**: active

**route**: IF-11/API-014

**guard**: 프로토콜상 필요한 참가자들의 일치하는 publicKey/address/epoch·내구 저장 증거

**durableBoundary**: 공개 지갑 연결 commit 후 사용 가능. 완료 응답 유실은 같은 createRequest 조회; 새 주소 자동 생성 금지

### MP-04

**fromState**: active

**toState**: signing

**route**: API-015

**guard**: 현 wallet binding/approval context/source/participant epoch와 새 intent 승인

**durableBoundary**: roundId·messageDigest·policyRevision·epoch·presign material 사용을 예약; old nonce/presign 재사용 금지

### MP-05

**fromState**: signing

**toState**: active

**route**: IF-11

**guard**: selected protocol의 quorum transcript·서명 검증 + 원 intent 일치

**durableBoundary**: 원 승인 operation에 결과와 노출 가능성을 영속 기록하는 것과 외부 공개를 분리. 공개 직전 현 read/release 권한·source gate·signer epoch 재검사; 불일치면 결과 보관/관측만. submit은 API-018 현 권한 별도; 결과 불명은 자동 재서명 금지; active 복귀는 현재 상태가 동일 epoch의 signing이고 제한이 없는 경우에만 CAS. 늦은 결과는 기록만 하며 recovery/security hold와 현 wallet state를 덮어쓰지 않음

### MP-06

**fromState**: active|signing

**toState**: recovery_proving

**route**: API-016

**guard**: 사전 등록된 독립 recovery proof + 새 participant proof + wallet/epoch/challenge 결합

**durableBoundary**: 미검증 신청만으로 wallet fence를 잠그지 않음; 자격 검증 후 원 epoch CAS로 recoveryId와 신규서명 차단 fence 예약

### MP-07

**fromState**: recovery_proving

**toState**: replacement_prepared

**route**: IF-11

**guard**: 정책상 요구되는 복구/quorum 증명, current revisions 재검사, 진행 signing round를 protocol별 정리

**durableBoundary**: 기존 round의 complete/exposure/unknown 기록; 새 epoch 후보와 participant 준비 ACK. deadline이면 자동 rollback하지 않음

### MP-08

**fromState**: replacement_prepared

**toState**: epoch_committed

**route**: IF-11

**guard**: expected old epoch, 새 participant set과 protocol 결과/주소 유지 또는 변경 기대값 검증; commit 직전 현 recovery authority/policy/security revision 재검사

**durableBoundary**: coordinator가 monotonic 새 epoch+transcript digest+commit decision을 저장; 참가자 저장소와 원자 DB인 척하지 않음

### MP-09

**fromState**: epoch_committed

**toState**: verifying

**route**: IF-11

**guard**: 선정 프로토콜의 새 활성 집합/존속 참가자 중 필수 주체가 동일 commit decision을 확인; 분실한 old phone ACK를 필수로 요구하지 않음

**durableBoundary**: 새 참가자별 activation receipt와 서비스 구세대 거절, 접근 가능한 보관소의 old material 비활성/처리 증거. 분실한 폰의 물리 share 삭제는 증명하지 못함. 필수 새 집합 ACK 부족이면 fence 유지; old epoch 복귀 금지

### MP-10

**fromState**: verifying

**toState**: active

**route**: IF-11

**guard**: 목적 제한 challenge 서명 검증, 선택 policy의 준비 조건 충족, 현재 보안 제한 재검사

**durableBoundary**: 새 participant generation 활성과 자기 recovery fence 해제·outcome 기록. 다른 account/security hold 유지

### MP-11

**fromState**: enrolling|dkg_pending|signing|recovery_proving|replacement_prepared|epoch_committed|verifying

**toState**: recovery_hold

**route**: IF-11

**guard**: 응답 유실/불일치/프로토콜 실패/timeout/authority 철회

**durableBoundary**: 같은 원 session/epoch/commit만 조사·재개. 완성됐을 수 있는 서명과 소모됐을 수 있는 material을 미사용으로 복원하지 않음

## authorityImpacts

### IM-01

**event**: 앱 내 개인→매장 전환

**account**: 선택 UI/context만 변경; 원 account session 유지

**hardware**: 원 account device binding 유지, 새 store signer 권한 재검사

**cloud**: 개인 participant 자동 점주 signer 전용 금지

**existingWork**: 이미 제출된 거래 관측 지속, 새 view는 현 scope만

### IM-02

**event**: 다른 계정으로 전환

**account**: old view generation/requests 폐기, 새 session으로 조회

**hardware**: BLE owner session 닫기; 대여자 변경 아님, old binding을 새 계정으로 재할당 금지

**cloud**: native participant namespace 잠금, 새 account가 old handle 사용 불가

**existingWork**: 늦은 old 응답은 old owner scoped 저장·조회만

### IM-03

**event**: 현재 앱 로그아웃

**account**: 대상 session/family만 철회

**hardware**: 앱 owner transport 종료; 별도 kiosk guest 권한은 ancestry/scope에 따라 판정

**cloud**: 앱 participant 잠금, 자동 share 삭제 없음; 해당 session 승인 새 round 차단

**existingWork**: 기존 노출 서명/체인 결과는 남고 현 읽기권 별도

### IM-04

**event**: 폰 분실·계정 침해 대응

**account**: 인증된 신고 후 관련 sessions/authority에 reason별 제한

**hardware**: 현재 bound NU 접근과 별도 device loss 여부 구분; offline 즉시전달 보장 금지

**cloud**: 사전 recovery proof로 교체, social 재로그인만으로 해제 금지

**existingWork**: 서버 확인 제한과 기기 미확인 제한을 구분 표시

### IM-05

**event**: Google/Apple 연결 해제

**account**: provider identity 연결만; 잔존 로그인/복구 확인

**hardware**: NU 키/대여/credential 자동삭제 없음

**cloud**: participant 소유권 또는 threshold 변경 아님

**existingWork**: 기존 기록 ownership 병합/이전 없음

### IM-06

**event**: NU 반납

**account**: 로그인/Cloud 계정은 별도 유지

**hardware**: 원 rental/epoch의 반환·초기화 조건, RR-DEC-01 유지

**cloud**: Cloud share 자동삭제/다른 wallet 강제이동 없음

**existingWork**: 개인 기록은 과거 eligibility 권한 기준

### IM-07

**event**: Cloud participant 교체

**account**: 로그인 identity와 독립

**hardware**: HW 키 및 signer 정책 불변

**cloud**: 원 epoch fence→프로토콜 교체→새epoch 검증; old quorum 무효 보장 아님

**existingWork**: 새 지갑이면 자산/권한 이전은 별도 승인

### IM-08

**event**: 탈퇴

**account**: deletion scope와 identity/session 처리 계획 분리

**hardware**: import 원지갑/unrelated 자산 강제 sweep 없음

**cloud**: 키 삭제 전에 독립 접근과 잔여자산·미결과 처리 검증

**existingWork**: 개인 콘텐츠 삭제/감사자료 보관 정책 분리; 운영자 임의 복구 금지

## logicalResources

### SR-01

**name**: AuthFlow·ProviderIdentity

**identity**: flowId; canonical issuer+subject unique within validated provider namespace

**rule**: flow owner·purpose·accountRevision과 검증결과 commit. 이메일은 unique 소유 근거 아님

### SR-02

**name**: RefreshFamily·RotationOutcome

**identity**: familyId+generation, bound sender/request digest

**rule**: token 원문은 전용 보호 저장 adapter, 일반 operations/로그/보호 객체 API에 넣지 않음

### SR-03

**name**: AppContextLease

**identity**: account/session/store/contextGeneration

**rule**: UI/native callback stale generation 거절, 서버 권한 발급 근거가 아닌 local 분리

### SR-04

**name**: WalletControlSnapshot

**identity**: walletId+bindingRevision+signerRef+participantEpoch

**rule**: chain/address/current source·approvalVersion 결합. 클라이언트 supplied epoch가 authoritative 아님

### SR-05

**name**: MpcEnrollment·ParticipantRegistry

**identity**: createRequestId; wallet/participantId/epoch

**rule**: 공개키·role·transcript digest만 업무 DB. share 원문은 선택된 격리 엔진 저장

### SR-06

**name**: MpcRound·MaterialUse

**identity**: wallet/epoch/protocolSession/round/messageDigest; materialId unique

**rule**: 예약·소모/불확정 상태 단조 이력; rollback/retry로 nonce/presign을 미사용으로 돌리지 않음

### SR-07

**name**: RecoveryAttempt·EpochCommit

**identity**: wallet/currentEpoch/recoveryId; single active commit decision

**rule**: 증명검증 후 wallet fence CAS; 참가자 prepared/activation ACK와 재개 상태 기록

### SR-08

**name**: SecurityRestriction·AuditOutcome

**identity**: authority scope+reason+revision+original event

**rule**: shared authorization gate 원칙 준수, 자기 reason만 해제. 외부 참가자와 한 DB 트랜잭션 아님

## contractDeltas

### SD-01

**apiRefs**

```json
[
  "API-103",
  "API-001",
  "API-002"
]
```

**proposal**: provider/client OS profile·flow sender binding·unique identity/세션 결과 복구를 auth adapter로 구체화

**limitation**: 기준 fields 유지; 정책/형태 후보, 기존 전체 schema 병합 안 함

### SD-02

**apiRefs**

```json
[
  "API-003",
  "API-004"
]
```

**proposal**: 세션별 종료와 전체세션/보안 hold 분리; 삭제 acknowledgedWalletRecovery를 검증증거로 대체하지 않음

**limitation**: logout scope 확대·unlink·전체세션 API는 미등록

### SD-03

**apiRefs**

```json
[
  "API-014"
]
```

**proposal**: createRequestId→MPC session/publickey/participantEpoch의 영속 연결 및 pending 상태

**limitation**: participantEnrollmentProof exact schema와 signer 제공자 미선정

### SD-04

**apiRefs**

```json
[
  "API-016"
]
```

**proposal**: recover 목적 challenge, expectedEpoch, new participant, mode/addressOutcome, 진행상태·읽기권 제안

**limitation**: 현재 API 필드 외 추가 shape는 후속 전체 요청/응답 계약; HTTP200을 복구완료로 해석 금지

### SD-05

**apiRefs**

```json
[
  "API-015",
  "API-017",
  "API-018",
  "API-020"
]
```

**proposal**: 현 approval baseline typed ancestry/gate/revision 유지, participant epoch·material사용 검증 추가

**limitation**: 개인/환불 지원 경로만 연결; guest HW결제를 Cloud로 자동변환하지 않음

### SD-06

**apiRefs**

```json
[
  "API-020"
]
```

**proposal**: Cloud create/recovery 조회는 별도 typed parent/reader 필요; social session 상실 시 제한 recovery 조회 credential 제안

**limitation**: 현재 operationClass/dispatch에 등록·schema·ACL이 필요하며 generic fallback 금지. 새 recovery credential은 키/서명권 아님

## runtimeCases

### SW-T01

**scenario**: 같은 이메일 다른 provider

**expected**: 자동 계정/지갑/영수증 병합 없음

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T02

**scenario**: 다른 계정에 연결된 provider 추가

**expected**: identity unique conflict; 양쪽 account 정보 노출 없음

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T03

**scenario**: 동시 첫 로그인

**expected**: 검증된 동일 issuer+subject는 한 account 연결, flow별 결과 멱등

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T04

**scenario**: 계정 전환 후 old callback

**expected**: flow가 old account에만 귀속; 새 화면/지갑에 결과 주입 없음

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T05

**scenario**: 잘못된 audience/nonce/expired proof

**expected**: 세션·participant 발급 거절

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T06

**scenario**: provider 교환응답 유실

**expected**: 검증결과 없으면 새 flow; 소비된 code 재사용 금지

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T07

**scenario**: refresh 정상 동시요청

**expected**: native 단일 실행 + server generation CAS; 안전한 결과 조회 아니면 재로그인

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T08

**scenario**: 탈취된 old refresh 재사용

**expected**: sender 증명 없는 최신 token 반환 금지, 해당 family 보호 처리

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T09

**scenario**: old account 늦은 MPC 응답

**expected**: 새 account view에서 노출/사용 불가; 원 operation의 현재 read권 검사

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T10

**scenario**: 소셜 로그인만으로 서명

**expected**: 독립 approval/participant proof 없으면 거절

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T11

**scenario**: kiosk 로그인 후 개인 자산조회

**expected**: 해당 store 권한 외 개인키/복구자료 접근 불가

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T12

**scenario**: 최종 로그인 수단 해제

**expected**: 복구 접근 증거 없으면 해제 보류; wallet 삭제 아님

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T13

**scenario**: 일반 로그아웃 후 NU 독립결제

**expected**: 현재 guest authority ancestry로 판정; 앱 session 종료를 전역 키 삭제로 확대하지 않음

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T14

**scenario**: 계정 침해 중 offline NU

**expected**: server restriction 확인과 device delivery pending 분리

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T15

**scenario**: Cloud DKG 결과응답 유실

**expected**: 원 createRequest/publickey를 복구; 새 주소 자동생성 금지

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T16

**scenario**: 부분 DKG 참가자 저장 실패

**expected**: active/입금주소 표시 금지; 검증된 protocol 재개/정리

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T17

**scenario**: nonce/presign reservation 후 crash

**expected**: 사용 가능으로 되돌리지 않고 consumed_or_unknown 처리

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T18

**scenario**: 재로그인만으로 폰 교체

**expected**: 사전 독립 recovery proof 없으면 거절

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T19

**scenario**: 미검증 recovery 요청 flood

**expected**: 등록 wallet fence 생성 금지; rate limit/열거 방지

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T20

**scenario**: 복구와 새 서명 경쟁

**expected**: 동일 wallet epoch/gate 원자 예약에서 하나의 허용 상태만; 완성가능서명 별도 기록

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T21

**scenario**: 참가자 교체 중 서비스 장애

**expected**: epoch/commit 조회로 재개, 이전epoch 자동복원 금지

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T22

**scenario**: epoch commit 뒤 ACK 유실

**expected**: exact decision 재전달, 구세대 신규 round 거절, 준비 부족시 hold

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T23

**scenario**: 구세대 메시지/다른 wallet round

**expected**: session/epoch/purpose/digest 불일치 거절

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T24

**scenario**: 새 참가자 등록 후 challenge 실패

**expected**: 복구완료 금지, own recovery fence 유지

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T25

**scenario**: 이미 유출된 quorum + refresh

**expected**: 키 안전 복구 선언 금지; 새키/자산이동 별도계획

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T26

**scenario**: 서비스 B 중단

**expected**: A+C 독립 경로 미검증이면 독립복구 가능 표시 금지

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T27

**scenario**: NU 반납 Cloud 잔고 존재

**expected**: Cloud share 유지, import 외부자산 sweep 없음

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T28

**scenario**: 탈퇴 checkbox만 true

**expected**: 독립 접근/잔여작업 증거 없는 파괴적 삭제 금지

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T29

**scenario**: API020 create/recovery 타입 미등록

**expected**: typed reader 미지원 거절; generic approval fallback 없음

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T30

**scenario**: 서명 공개 후 세션 철회

**expected**: 이미 공개된 서명 무효 주장 금지; 중복 서명/지급 없이 원거래 관측

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T31

**scenario**: participant 준비완료 후 security hold

**expected**: 복구완료가 다른 hold를 해제하지 않음

**status**: not_run

**evidenceRefs**

```json
[]
```

### SW-T32

**scenario**: 새키 복구 주소 변경

**expected**: same-address recovered 표시 금지; 명시적 migration 경로 필요

**status**: not_run

**evidenceRefs**

```json
[]
```

## externalReferences

### EXT-01

**url**: https://developers.google.com/identity/openid-connect/openid-connect

**checkedAt**: 2026-09-19

**use**: Google verified sub identity; email not primary ID

### EXT-02

**url**: https://developer.apple.com/documentation/signinwithapple/verifying-a-user

**checkedAt**: 2026-09-19

**use**: Apple server verification; provider profile distinct

### EXT-03

**url**: https://www.rfc-editor.org/rfc/rfc9700.html

**checkedAt**: 2026-09-19

**use**: Public client refresh rotation or sender constraint

## nextDesign

DS-03 키오스크·결제·환불·스탬프·정산 종단 연결

## holdResumeRule

```json
{
  "source": "server_durable_operation_phase",
  "directActiveAllowed": false,
  "beforeCommit": "verified_protocol_abort_or_resume_same_operation",
  "afterCommit": "same_committed_epoch_only",
  "authorityRecheck": true
}
```

## authContractRefs

```json
{
  "OC-17": "content/specifications/preimplementation-contract-overlay.json#/routeContracts/16",
  "OC-18": "content/specifications/preimplementation-contract-overlay.json#/routeContracts/17",
  "OC-19": "content/specifications/preimplementation-contract-overlay.json#/routeContracts/18",
  "OC-20": "content/specifications/preimplementation-contract-overlay.json#/routeContracts/19"
}
```

## declaredScopeRequirementRefs

- 4
- 5
- 6
- 8
## transitiveTaskImpactRequirementRefs

- 4
- 5
- 6
- 7
- 8
- 9
- 10
- 15
## requirementRefsMeaning

legacy alias of declaredScopeRequirementRefs; not exhaustive impact or completion evidence


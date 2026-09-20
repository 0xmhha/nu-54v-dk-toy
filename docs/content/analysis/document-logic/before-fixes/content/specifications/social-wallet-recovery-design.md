# 소셜 로그인·두 지갑·MPC 복구 연결 설계

2026-09-19 · **DS-02 설계 후보. 제품 구현·SDK 설치·MPC 선택·기준 계약 병합·실제 인증/서명 시험은 수행하지 않았다.** 기존 15개 요구·12주 앱 연동·3명과 WBS 104개/320개를 유지한다.

[구조화 원본](social-wallet-recovery-design.json) · [검증기](validate_social_wallet_recovery.py) · [기존 지갑 통제 설계](wallet-control-recovery-design.md) · [기기 공존 설계](device-coexistence-design.md)

## 1. 사용자에게 보여줄 결과

로그인 성공 뒤 지갑 카드에는 계정 접근과 서명 준비를 따로 표시한다. `로그인 완료 · NU 연결 필요`, `로그인 완료 · Cloud 참여자 복구 필요`처럼 구분한다. 폰 교체 후 Google/Apple 로그인이 성공해도 자산을 바로 보낼 수 있다고 표시하지 않는다. HW 서명은 NU에서, Cloud 서명은 선택된 MPC 경로에서 수행하며 자동 대체하지 않는다.

HW/Cloud를 별도 주소로 표시하는 안은 WC-P01/D03의 미선택 제안이다. 이 문서는 두 signer의 권한을 분리하지만 주소 배치를 확정하지 않는다. 동일 chain/address가 여러 지갑 카드에 있으면 잔액은 중복 합산하지 않고, 계정의 사적 기록·매장 소속은 병합하지 않는다.

로그인 복구, 사용자 MPC 참여자 교체, 지갑 키 교체, NU 대여 반납, 계정 데이터 삭제는 서로 다른 작업이다. `복구 완료`는 무엇이 복구됐는지 표시하고 account access·participant readiness·signing enabled·remaining restrictions를 별도 상태로 제공한다.

## 2. Google·Apple 제공자 경계

Google은 이메일을 사용자 기본 식별자로 삼지 말도록 설명한다. 검증된 issuer와 sub로 연결하고, 이메일 일치는 이 제품의 자동 병합 근거로 쓰지 않는다. [Google OpenID Connect](https://developers.google.com/identity/openid-connect/openid-connect)

Apple도 서버에서 identity token을 검증하는 절차를 제공한다. Apple과 Google의 client/SDK/nonce 처리를 하나의 고정 구현으로 가정하지 않고 각각의 profile로 검증한다. [Apple 사용자 검증](https://developer.apple.com/documentation/signinwithapple/verifying-a-user)

공개 클라이언트의 refresh token에는 rotation 또는 sender constraint가 요구된다. 여기서는 기존 회전 설계를 유지하되 제공자 token과 서비스가 발급한 앱 session token의 저장·철회 책임을 분리한다. [RFC 9700 §2.2.2](https://www.rfc-editor.org/rfc/rfc9700.html#section-2.2.2)

제품 profile에는 provider/issuer, clientId/audience, OS/app identity, redirect registry, proof 형식, state/nonce/PKCE 적용 방식, JWKS/서명 검증, token lifetime 검증과 callback 복구 경로가 필요하다. client secret은 앱에 내장하지 않는다. Android S25 Ultra/태블릿 대상의 Google·Apple 실제 로그인 경로는 각각 검증해야 한다. 지원 OS·SDK 버전·키 교체 cache 정책은 미선정이다. provider subject namespace 변경/앱 이전은 별도 검증 mapping 없이는 기존 identity에 자동 연결하지 않는다.

## 3. 인증·연결·전환 상태

| ID | 전이 | 연결 | 조건 | 원자/복구 경계 | 화면 |
|---|---|---|---|---|---|
| AU-01 | flow_absent → flow_open | API-103 | 등록 provider/client/redirect/profile, purpose=login/link 분리; login은 비로그인 진입 가능, link는 현재 계정 recent auth | flowId/purpose/initiator/sessionGeneration/nonce·state 검증값/expiry를 저장 | 인증 진행 |
| AU-02 | flow_open → proof_verifying | API-001/API-002 | flow 소유·단일소비 reservation, 해당 provider proof·요청 digest 검증 | provider 외부 통신 동안 DB lock 유지 금지; result_unknown과 failed 분리 | 로그인 확인 중 |
| AU-03 | proof_verifying → session_active | API-001 | 서명/issuer/audience/expiry/flow nonce 등 선택 profile 검증 후 issuer+subject로 조회 | flow consume와 유일 identity/account/session 결과를 원자 반영; 사적 지갑 권한은 따로 조회 | 로그인 완료·지갑 준비 상태 별도 |
| AU-04 | proof_verifying → identity_linked | API-002 | 대상 account/session/revision 재검사 + 새 provider의 현 증명; identity가 다른 account에 속하면 conflict | identity unique constraint + flow consumed + audit/outcome 함께 반영; 계정 자동 merge 없음 | 로그인 수단 연결 완료 |
| AU-05 | proof_verifying → restart_required | API-001/API-002 | code 소비 여부 불명이고 안전하게 저장된 결과 없음 | 실패로 재사용하지 말고 새 flow. 이미 기록된 결과는 원 flow sender 증명으로만 복구 | 로그인을 다시 시작해 주세요 |
| AU-06 | session_active → session_active | unregistered_refresh_adapter | current refresh family/generation + 선택된 sender binding 검증 | 회전 결과와 generation CAS; 보호 결과만 보관. 이전 bearer만으로 최신 token 공개 금지 | 세션 갱신 |
| AU-07 | session_active → local_context_switched | local_app | 개인/매장/account context 명시 선택. 다른 account는 그 account의 유효 session 필요 | old view generation 폐기, native 요청/화면 정리·participant 잠금; 새 context 권한 재조회 | 선택 계정/매장으로 전환 |
| AU-08 | session_active → session_revoked | API-003 | 현재 actor가 대상 session을 종료할 권한 확인; 전체세션 종료는 별도 미등록 계약 | family/session revoke revision 및 scope별 outbox; 미확인 offline전달은 pending | 이 앱 로그아웃; 잔여 기기 제한은 별도 |
| AU-09 | identity_linked → unlink_pending | unregistered_identity_unlink | recent auth + 잔존 로그인/복구 경로 검증. 마지막 수단은 단순 클릭으로 제거하지 않음 | identity revision/관련 flow·session 영향 검토. provider unlink와 share 삭제 분리 | 로그인 수단 해제 확인 필요 |
| AU-10 | session_active → account_restricted | unregistered_security_event | 검증된 provider 보안 통지/관리 정책·현재 account revision | 영향 session과 sensitive authority에 reason별 hold, 다른 provider가 있다고 자동 해제 금지 | 계정 보호 조치·별도 복구 필요 |
| AU-11 | session_active → deletion_pending | API-004 | recent auth + 자산 접근/대여/Cloud/HW/영수증·데이터 보관 영향 확인 | acknowledgedWalletRecovery boolean은 증거 아님. 검증된 독립 접근/처리 계획 전 파괴적 삭제 금지 | 탈퇴 검토 중; 키 삭제 완료 아님 |

위 상태는 하나의 거대 account enum이 아니다. AuthFlow, session, provider link, local UI context, deletion 각각의 상태를 설명한다. 다른 계정 callback은 원 flow에만 귀속하고 화면 generation이 바뀌면 새 화면으로 전달하지 않는다. 로컬 전환이 서버 로그아웃을 자동 의미하지 않으므로 사용자는 명시적으로 세션 종료를 선택할 수 있어야 한다. 전환 시 native의 이전 참가자를 잠그고 새 계정이 handle을 재사용하지 못하게 한다.

flowId/operationId를 아는 것만으로 세션 token을 재조회하지 못한다. 서버가 이미 성공 결과를 기록한 경우에도 원 flow의 검증된 sender binding이 있어야 결과 복구가 가능하다. 없거나 code 교환 결과가 불명확하면 새 로그인 flow로 돌아간다. 같은 identity의 새 flow는 새 지갑을 자동 생성하지 않는다.

## 4. 권한 변화의 영향 범위

| ID/사건 | 계정 | HW | Cloud | 진행 작업/기록 |
|---|---|---|---|---|
| IM-01 앱 내 개인→매장 전환 | 선택 UI/context만 변경; 원 account session 유지 | 원 account device binding 유지, 새 store signer 권한 재검사 | 개인 participant 자동 점주 signer 전용 금지 | 이미 제출된 거래 관측 지속, 새 view는 현 scope만 |
| IM-02 다른 계정으로 전환 | old view generation/requests 폐기, 새 session으로 조회 | BLE owner session 닫기; 대여자 변경 아님, old binding을 새 계정으로 재할당 금지 | native participant namespace 잠금, 새 account가 old handle 사용 불가 | 늦은 old 응답은 old owner scoped 저장·조회만 |
| IM-03 현재 앱 로그아웃 | 대상 session/family만 철회 | 앱 owner transport 종료; 별도 kiosk guest 권한은 ancestry/scope에 따라 판정 | 앱 participant 잠금, 자동 share 삭제 없음; 해당 session 승인 새 round 차단 | 기존 노출 서명/체인 결과는 남고 현 읽기권 별도 |
| IM-04 폰 분실·계정 침해 대응 | 인증된 신고 후 관련 sessions/authority에 reason별 제한 | 현재 bound NU 접근과 별도 device loss 여부 구분; offline 즉시전달 보장 금지 | 사전 recovery proof로 교체, social 재로그인만으로 해제 금지 | 서버 확인 제한과 기기 미확인 제한을 구분 표시 |
| IM-05 Google/Apple 연결 해제 | provider identity 연결만; 잔존 로그인/복구 확인 | NU 키/대여/credential 자동삭제 없음 | participant 소유권 또는 threshold 변경 아님 | 기존 기록 ownership 병합/이전 없음 |
| IM-06 NU 반납 | 로그인/Cloud 계정은 별도 유지 | 원 rental/epoch의 반환·초기화 조건, RR-DEC-01 유지 | Cloud share 자동삭제/다른 wallet 강제이동 없음 | 개인 기록은 과거 eligibility 권한 기준 |
| IM-07 Cloud participant 교체 | 로그인 identity와 독립 | HW 키 및 signer 정책 불변 | 원 epoch fence→프로토콜 교체→새epoch 검증; old quorum 무효 보장 아님 | 새 지갑이면 자산/권한 이전은 별도 승인 |
| IM-08 탈퇴 | deletion scope와 identity/session 처리 계획 분리 | import 원지갑/unrelated 자산 강제 sweep 없음 | 키 삭제 전에 독립 접근과 잔여자산·미결과 처리 검증 | 개인 콘텐츠 삭제/감사자료 보관 정책 분리; 운영자 임의 복구 금지 |

원 권한의 ancestry, 현재 membership/binding revision, source별 gate를 기준으로 영향받는 자식만 철회한다. 개인 앱 로그아웃을 모든 키오스크 고객 세션 종료로 확대하지 않는다. 반면 계정 침해 대응은 별도 보안 정책 범위로 제한한다. 신규 실행 차단과 기존 transaction 관측을 분리하고, 오프라인 기기는 실제 ACK 전까지 제한 전달 확인으로 표시하지 않는다.

## 5. MPC 생성·서명·교체 상태

| ID | 전이 | 경로 | 조건 | 영속/복구 경계 |
|---|---|---|---|---|
| MP-01 | absent → enrolling | API-014 | 최근 계정 인증 + 별도 participantEnrollmentProof + 선택 policy/profile | account/createRequestId/requestDigest와 protocolSession 하나 예약; identity 로그인만으로 사용자 share 발급 금지 |
| MP-02 | enrolling → dkg_pending | IF-11 | 등록 참가자·역할·독립 신뢰영역·protocol version 검증 | DKG transcript를 같은 session에 결합; partial share/주소는 사용 가능으로 공개하지 않음 |
| MP-03 | dkg_pending → active | IF-11/API-014 | 프로토콜상 필요한 참가자들의 일치하는 publicKey/address/epoch·내구 저장 증거 | 공개 지갑 연결 commit 후 사용 가능. 완료 응답 유실은 같은 createRequest 조회; 새 주소 자동 생성 금지 |
| MP-04 | active → signing | API-015 | 현 wallet binding/approval context/source/participant epoch와 새 intent 승인 | roundId·messageDigest·policyRevision·epoch·presign material 사용을 예약; old nonce/presign 재사용 금지 |
| MP-05 | signing → active | IF-11 | selected protocol의 quorum transcript·서명 검증 + 원 intent 일치 | 원 승인 operation에 결과와 노출 가능성을 영속 기록하는 것과 외부 공개를 분리. 공개 직전 현 read/release 권한·source gate·signer epoch 재검사; 불일치면 결과 보관/관측만. submit은 API-018 현 권한 별도; 결과 불명은 자동 재서명 금지; active 복귀는 현재 상태가 동일 epoch의 signing이고 제한이 없는 경우에만 CAS. 늦은 결과는 기록만 하며 recovery/security hold와 현 wallet state를 덮어쓰지 않음 |
| MP-06 | active/signing → recovery_proving | API-016 | 사전 등록된 독립 recovery proof + 새 participant proof + wallet/epoch/challenge 결합 | 미검증 신청만으로 wallet fence를 잠그지 않음; 자격 검증 후 원 epoch CAS로 recoveryId와 신규서명 차단 fence 예약 |
| MP-07 | recovery_proving → replacement_prepared | IF-11 | 정책상 요구되는 복구/quorum 증명, current revisions 재검사, 진행 signing round를 protocol별 정리 | 기존 round의 complete/exposure/unknown 기록; 새 epoch 후보와 participant 준비 ACK. deadline이면 자동 rollback하지 않음 |
| MP-08 | replacement_prepared → epoch_committed | IF-11 | expected old epoch, 새 participant set과 protocol 결과/주소 유지 또는 변경 기대값 검증; commit 직전 현 recovery authority/policy/security revision 재검사 | coordinator가 monotonic 새 epoch+transcript digest+commit decision을 저장; 참가자 저장소와 원자 DB인 척하지 않음 |
| MP-09 | epoch_committed → verifying | IF-11 | 선정 프로토콜의 새 활성 집합/존속 참가자 중 필수 주체가 동일 commit decision을 확인; 분실한 old phone ACK를 필수로 요구하지 않음 | 새 참가자별 activation receipt와 서비스 구세대 거절, 접근 가능한 보관소의 old material 비활성/처리 증거. 분실한 폰의 물리 share 삭제는 증명하지 못함. 필수 새 집합 ACK 부족이면 fence 유지; old epoch 복귀 금지 |
| MP-10 | verifying → active | IF-11 | 목적 제한 challenge 서명 검증, 선택 policy의 준비 조건 충족, 현재 보안 제한 재검사 | 새 participant generation 활성과 자기 recovery fence 해제·outcome 기록. 다른 account/security hold 유지 |
| MP-11 | enrolling/dkg_pending/signing/recovery_proving/replacement_prepared/epoch_committed/verifying → recovery_hold | IF-11 | 응답 유실/불일치/프로토콜 실패/timeout/authority 철회 | 같은 원 session/epoch/commit만 조사·재개. 완성됐을 수 있는 서명과 소모됐을 수 있는 material을 미사용으로 복원하지 않음 |

MPC 메시지 논리 envelope는 walletId, protocolProfile/version, protocolSession, roundId, participantId/epoch, messageSequence, purpose, payload/transcript digest, policyRevision, expiry와 sender 인증을 결합한다. exact codec/곡선/프로토콜·오류 증명은 선정 엔진의 검증된 규칙을 사용하고 자체 임계 서명 알고리즘을 만들지 않는다. 업무 DB/API/로그에는 share 원문·전체 private key·presign 비밀이 들어가지 않는다.

### 교체와 실패 복구의 핵심

복구 요청 접수만으로 지갑을 중지시키면 walletId를 아는 공격자가 사용을 막을 수 있다. proof를 검증한 뒤 현 epoch를 CAS하여 하나의 활성 recovery 작업과 신규 서명 제한을 예약한다. 이미 진행 중인 round는 엔진의 안전한 중단/완료 경계를 따르고, 외부에 완성 서명이 존재할 수 있으면 별도 관측한다. 임의 timeout으로 material을 재사용 가능하게 바꾸지 않는다.

DB의 새 epoch commit과 참가자의 share 저장은 같은 트랜잭션이 아니다. 준비 증거→coordinator의 durable commit decision→참가자별 적용 ACK→기능 확인→활성화 순서로 추적하는 안이다. 실제 프로토콜이 이를 지원하지 않으면 adapter를 바꾸거나 해당 복구 경로를 unsupported로 남겨야 하며, 불완전한 교체를 완료로 표시하지 않는다. DB/백업 복원 시 최신 epoch/commit/material 소모 이력을 외부 내구 기록과 대조하기 전 서명을 열지 않는다.

recovery_hold는 작업 종료가 아니다. 재개 시 원작업 resumePhase와 durable commit decision, 현재 권한/epoch를 조회하여 해당 단계의 동일 guard를 다시 수행한다. phase는 클라이언트가 지정하지 않는다. commit 이전은 검증된 프로토콜의 abort/restart만 가능하고 commit 이후는 동일 새 epoch 활성화/검증을 계속하거나 hold를 유지한다. hold→active 직접 전이는 없다.

서명 테스트는 테스트넷 송금을 강제하지 않는다. 선택 엔진과 verifier가 지원하는 목적 제한 challenge를 검증하고 주소·epoch·participant readiness를 확인한다. 이 증거를 결제/일반 동의 서명으로 재사용하지 않는다. 복구 작업 완료와 계정 보안 제한 해제는 별도다.

share refresh나 서비스의 구세대 거절이 이미 유출된 유효 quorum/전체 키/완성 EOA 서명을 무효화하지는 않는다. 침해 범위에 따라 새 키/주소·자산·권한 이동 계획이 필요하며, 이는 주소 유지 교체와 다른 사용자 승인 작업이다. 공유 파일을 합쳐 전체키를 만든 뒤 서명하는 구현은 MPC 요건을 충족한 것으로 인정하지 않는다.

## 6. MPC 구성 비교: 아직 선택하지 않음

| 후보 | 일상 서명/복구 가능성 | 검증할 대가 |
|---|---|---|
| 사용자 폰 A + 서비스 B + 독립 복구 C, 2-of-3 | 일상 A+B; A 상실 시 B+C; B 중단 시 A+C 후보 | B+C가 사용자 폰 없이 quorum이 됨. C 운영/백업/자격발급이 B와 독립인지, 서비스 장애 시 A+C 도구·인증도 독립인지 검증 |
| 사용자 통제 C를 포함한 2-of-3 | 복구에 별도 사용자 수단 요구 가능 | 그 수단의 보관/유실/새 폰 전달; 같은 social login으로 C 재발급하면 독립성 상실 |
| A+B 2-of-2 | 평소 두 참가자 필요 | 하나 상실/장기중단 시 두 share만으로 복구 불가. 별도 경로 없으면 전체 복구 요구 미충족 |

threshold·참가자 운영주체·curve/프로토콜·복구 근거·지연/알림 정책은 D04 미선택이다. 선택 항목에는 예외/실패 시 동작, 서비스 중단 시 사용 경로, 유출 quorum 대응, old/new epoch 호환 증거를 포함한다. SDK만 선정했다고 이 신뢰 구조가 해결되지는 않는다.

소셜 계정도 접근 불가한 경우에는 사전 등록 recovery proof로 시작하는 제한 bootstrap/read 경로가 필요하다. API-016의 wallet_recovery_proof를 account bearer로 대체하지 않는다. recovery credential은 특정 wallet/recoveryId/challenge/new participant/epoch/purpose/기한에 묶고 주소·operationId만으로 발급하지 않는다. 발급 프로파일·typed result reader는 미등록 후속 계약이다. 고객센터 재량이나 이메일만으로 key/share를 재발급하는 대안은 넣지 않는다.

## 7. 저장 책임과 기존 계약 반영점

아래는 논리 adapter 후보다. 신규 SQL 테이블 8개를 만들었다는 의미가 아니며 현재 61개 업무 테이블과 canonical schema는 그대로다.

| ID | 논리 자원 | 식별/유일성 | 경계 |
|---|---|---|---|
| SR-01 | AuthFlow·ProviderIdentity | flowId; canonical issuer+subject unique within validated provider namespace | flow owner·purpose·accountRevision과 검증결과 commit. 이메일은 unique 소유 근거 아님 |
| SR-02 | RefreshFamily·RotationOutcome | familyId+generation, bound sender/request digest | token 원문은 전용 보호 저장 adapter, 일반 operations/로그/보호 객체 API에 넣지 않음 |
| SR-03 | AppContextLease | account/session/store/contextGeneration | UI/native callback stale generation 거절, 서버 권한 발급 근거가 아닌 local 분리 |
| SR-04 | WalletControlSnapshot | walletId+bindingRevision+signerRef+participantEpoch | chain/address/current source·approvalVersion 결합. 클라이언트 supplied epoch가 authoritative 아님 |
| SR-05 | MpcEnrollment·ParticipantRegistry | createRequestId; wallet/participantId/epoch | 공개키·role·transcript digest만 업무 DB. share 원문은 선택된 격리 엔진 저장 |
| SR-06 | MpcRound·MaterialUse | wallet/epoch/protocolSession/round/messageDigest; materialId unique | 예약·소모/불확정 상태 단조 이력; rollback/retry로 nonce/presign을 미사용으로 돌리지 않음 |
| SR-07 | RecoveryAttempt·EpochCommit | wallet/currentEpoch/recoveryId; single active commit decision | 증명검증 후 wallet fence CAS; 참가자 prepared/activation ACK와 재개 상태 기록 |
| SR-08 | SecurityRestriction·AuditOutcome | authority scope+reason+revision+original event | shared authorization gate 원칙 준수, 자기 reason만 해제. 외부 참가자와 한 DB 트랜잭션 아님 |

| 변경 | 기존 API | 보강 내용 | 미채택 경계 |
|---|---|---|---|
| SD-01 | API-103, API-001, API-002 | provider/client OS profile·flow sender binding·unique identity/세션 결과 복구를 auth adapter로 구체화 | 기준 fields 유지; 정책/형태 후보, 기존 전체 schema 병합 안 함 |
| SD-02 | API-003, API-004 | 세션별 종료와 전체세션/보안 hold 분리; 삭제 acknowledgedWalletRecovery를 검증증거로 대체하지 않음 | logout scope 확대·unlink·전체세션 API는 미등록 |
| SD-03 | API-014 | createRequestId→MPC session/publickey/participantEpoch의 영속 연결 및 pending 상태 | participantEnrollmentProof exact schema와 signer 제공자 미선정 |
| SD-04 | API-016 | recover 목적 challenge, expectedEpoch, new participant, mode/addressOutcome, 진행상태·읽기권 제안 | 현재 API 필드 외 추가 shape는 후속 전체 요청/응답 계약; HTTP200을 복구완료로 해석 금지 |
| SD-05 | API-015, API-017, API-018, API-020 | 현 approval baseline typed ancestry/gate/revision 유지, participant epoch·material사용 검증 추가 | 개인/환불 지원 경로만 연결; guest HW결제를 Cloud로 자동변환하지 않음 |
| SD-06 | API-020 | Cloud create/recovery 조회는 별도 typed parent/reader 필요; social session 상실 시 제한 recovery 조회 credential 제안 | 현재 operationClass/dispatch에 등록·schema·ACL이 필요하며 generic fallback 금지. 새 recovery credential은 키/서명권 아님 |

API-020의 원 operation 조회 원칙은 유지하지만 create/recovery typed parent와 result schema가 실제 등록되었다고 가정하지 않는다. 등록되지 않은 클래스는 거절하고 개인 송금/guest/refund approval reader에 끼워 넣지 않는다. API-015/017/018은 현재 approval-baseline의 source dispatch·typed ancestry·현재 action 권한을 그대로 사용한다. 스마트계정/상품/DID/x402 미완성 adapter는 별도 전체 범위 작업으로 남긴다.

## 8. 개발 단계 수용 사례

모두 **not_run**이다. 제공자/모바일/기기/MPC 엔진/장애주입 시험 없이 문서 정합성 통과를 인증·서명·복구 성공으로 세지 않는다.

| ID | 사건 | 기대 결과 |
|---|---|---|
| SW-T01 | 같은 이메일 다른 provider | 자동 계정/지갑/영수증 병합 없음 |
| SW-T02 | 다른 계정에 연결된 provider 추가 | identity unique conflict; 양쪽 account 정보 노출 없음 |
| SW-T03 | 동시 첫 로그인 | 검증된 동일 issuer+subject는 한 account 연결, flow별 결과 멱등 |
| SW-T04 | 계정 전환 후 old callback | flow가 old account에만 귀속; 새 화면/지갑에 결과 주입 없음 |
| SW-T05 | 잘못된 audience/nonce/expired proof | 세션·participant 발급 거절 |
| SW-T06 | provider 교환응답 유실 | 검증결과 없으면 새 flow; 소비된 code 재사용 금지 |
| SW-T07 | refresh 정상 동시요청 | native 단일 실행 + server generation CAS; 안전한 결과 조회 아니면 재로그인 |
| SW-T08 | 탈취된 old refresh 재사용 | sender 증명 없는 최신 token 반환 금지, 해당 family 보호 처리 |
| SW-T09 | old account 늦은 MPC 응답 | 새 account view에서 노출/사용 불가; 원 operation의 현재 read권 검사 |
| SW-T10 | 소셜 로그인만으로 서명 | 독립 approval/participant proof 없으면 거절 |
| SW-T11 | kiosk 로그인 후 개인 자산조회 | 해당 store 권한 외 개인키/복구자료 접근 불가 |
| SW-T12 | 최종 로그인 수단 해제 | 복구 접근 증거 없으면 해제 보류; wallet 삭제 아님 |
| SW-T13 | 일반 로그아웃 후 NU 독립결제 | 현재 guest authority ancestry로 판정; 앱 session 종료를 전역 키 삭제로 확대하지 않음 |
| SW-T14 | 계정 침해 중 offline NU | server restriction 확인과 device delivery pending 분리 |
| SW-T15 | Cloud DKG 결과응답 유실 | 원 createRequest/publickey를 복구; 새 주소 자동생성 금지 |
| SW-T16 | 부분 DKG 참가자 저장 실패 | active/입금주소 표시 금지; 검증된 protocol 재개/정리 |
| SW-T17 | nonce/presign reservation 후 crash | 사용 가능으로 되돌리지 않고 consumed_or_unknown 처리 |
| SW-T18 | 재로그인만으로 폰 교체 | 사전 독립 recovery proof 없으면 거절 |
| SW-T19 | 미검증 recovery 요청 flood | 등록 wallet fence 생성 금지; rate limit/열거 방지 |
| SW-T20 | 복구와 새 서명 경쟁 | 동일 wallet epoch/gate 원자 예약에서 하나의 허용 상태만; 완성가능서명 별도 기록 |
| SW-T21 | 참가자 교체 중 서비스 장애 | epoch/commit 조회로 재개, 이전epoch 자동복원 금지 |
| SW-T22 | epoch commit 뒤 ACK 유실 | exact decision 재전달, 구세대 신규 round 거절, 준비 부족시 hold |
| SW-T23 | 구세대 메시지/다른 wallet round | session/epoch/purpose/digest 불일치 거절 |
| SW-T24 | 새 참가자 등록 후 challenge 실패 | 복구완료 금지, own recovery fence 유지 |
| SW-T25 | 이미 유출된 quorum + refresh | 키 안전 복구 선언 금지; 새키/자산이동 별도계획 |
| SW-T26 | 서비스 B 중단 | A+C 독립 경로 미검증이면 독립복구 가능 표시 금지 |
| SW-T27 | NU 반납 Cloud 잔고 존재 | Cloud share 유지, import 외부자산 sweep 없음 |
| SW-T28 | 탈퇴 checkbox만 true | 독립 접근/잔여작업 증거 없는 파괴적 삭제 금지 |
| SW-T29 | API020 create/recovery 타입 미등록 | typed reader 미지원 거절; generic approval fallback 없음 |
| SW-T30 | 서명 공개 후 세션 철회 | 이미 공개된 서명 무효 주장 금지; 중복 서명/지급 없이 원거래 관측 |
| SW-T31 | participant 준비완료 후 security hold | 복구완료가 다른 hold를 해제하지 않음 |
| SW-T32 | 새키 복구 주소 변경 | same-address recovered 표시 금지; 명시적 migration 경로 필요 |

정상 로그인·연결·Cloud 생성·서명·동일주소 교체의 정상 대조군과 각 거절/장애 분기의 증거를 따로 모은다. provider/application/engine build·policy/epoch tuple에 결합한 evidence만 판정하며 `not_run`, `blocked`, `failed`, `passed`를 구분한다.

## 9. 보존한 결정과 다음 설계

작업: APP-01, APP-02, APP-03, AUTH-01, AUTH-02, AUTH-03, AUTH-04, BASE-06, MPC-01, MPC-02, MPC-03, MPC-04, MPC-05. 미결정 19개를 닫거나 인원별 역할/공수를 배정하지 않았다. 선택 8개는 모두 null이다. RR-DEC-01은 계속 답변 대기이며 기존 import 지갑의 unrelated 자산 강제 sweep 또는 Cloud share 자동삭제로 우회하지 않는다.

**다음은 DS-03 키오스크·결제·환불·스탬프·정산 종단 연결**이다. DS-02의 제공자/복구 증명 exact schema·typed reader·참가자 저장/교체 profile·삭제/철회 정책은 채택 전 항목으로 추적한다. 기존 결제·대사 상세 계약을 실제 고객/점주 앱 흐름과 연결한다.

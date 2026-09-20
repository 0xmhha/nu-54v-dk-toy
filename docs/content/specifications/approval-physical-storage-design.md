# 승인 저장소: 물리 제약·이행 순서·장애 복구 설계

2026-09-18 · **설계 제안. SQL 추가·수정·DB 실행·제품 구현은 하지 않았다.**

현재 승인 기준의 논리 자원 13개를 **추가 테이블 후보 16개**로 풀었다. 기존 61개 테이블에 적용된 결과가 아니며, 실제 총 테이블 수를 77개로 변경한 것도 아니다. PostgreSQL 15는 기존 참조 DDL과 비교하기 위한 엔진이고 운영 DB 선정은 미정이다.

[구조 원본](approval-physical-storage-design.json) · [현재 승인 기준](approval-baseline.md) · [논리 자원 매핑](approval-storage-mapping.json) · [문서 검사기](validate_approval_physical_storage.py)

## 1. 이번 설계에서 구체화한 것

| 경계 | 설계안 | 기존 구조와 연결 |
|---|---|---|
| 승인 문맥·작업 소속 | immutable binding + parent/child link | transaction_intents / operations |
| 서명 증거·보호 파일 | provenance + versioned object metadata | 기기 증거 또는 signing_sessions; 실제 bytes는 보호 저장소 |
| 접근 자격 | challenge + grant lineage + issuance outcome | 현재 account/terminal 인증과 분리된 제한 action |
| 권한 최신성 | shared gate + decision snapshot | 실제 세션·membership·기기 epoch·원천 업무 행과 같은 transaction |
| 응답 노출 | durable release decision | 소켓 수신 ACK가 없어도 노출 가능성 보존 |
| 제출 실행 | dispatch + permit + worker attempt | 기존 submission의 txHash 고유성 유지 |
| 체인·환불 | nonce slot/claims + observation application | 기존 관측·환불 예약·잔액·outbox 동시 갱신 |

테이블과 컬럼은 **차기 DDL 작성을 위한 제약 사전**이다. 아래의 CHECK/FK 표기는 의도이며 실행 가능한 SQL이 아니다. 서비스 검증을 DB CHECK로 구현했다고 읽으면 안 된다. `?`/nullable 표기 외 컬럼은 NOT NULL 제안이다. 모든 ID·주소·금액은 기존 domain을 재사용하며 nonce는 EOA 전용이다. HW 키·니모닉·MPC 전체키·평문 access token을 일반 DB에 저장하지 않는다.

PostgreSQL에서는 CHECK의 NULL 결과가 행을 거절하지 않으므로 필수 컬럼의 NOT NULL을 별도로 요구한다. 다른 행과의 관계는 CHECK에 숨기지 않고 FK·고유 제약과 transaction 검증으로 나눈다. [공식 제약 문서](https://www.postgresql.org/docs/15/ddl-constraints.html)

## 2. 원자 처리와 잠금 순서

**같은 관계형 transaction 영역에 승인 gate·원천 원장·grant lineage·멱등 결과·outbox를 둔다**는 설계 전제다. 이를 다른 DB/서비스의 독립 commit으로 나누려면 별도 프로토콜 설계부터 다시 해야 한다. 암호화 blob 저장과 RPC는 이 transaction에 포함되지 않는다.

모든 관련 writer는 서버가 정한 gate 목록을 `gate_key` 정렬 순서로 먼저 잠근다. 이어서 원천 업무 행을 `(table_rank, primary_key)` 순서로, 마지막으로 lineage/challenge/dispatch/nonce 행을 고정 순서로 잠근다. 잠금 후 gate 목록을 다시 계산해 누락이 있으면 transaction을 중단하고 전체 목록으로 재시작한다. 한 경로만 이 순서를 지켜서는 부족하며 철회·membership 변경·기기 재등록·환불 대사 writer에도 적용한다. 아래 잠금 순위는 제안이며 isolation level은 미정이다. 동일 그룹에서는 표에 나열한 테이블 순서, 같은 테이블에서는 PK 오름차순을 따른다. 신규 행처럼 잠글 대상이 아직 없으면 상위 scope gate와 고유 제약으로 생성 경쟁을 제어한다.

| 잠금 순위 | 대상(왼쪽부터) |
|---|---|
| 0 | approval_authorization_gates (gate_key 정렬) |
| 10 | auth_sessions, memberships, wallet_bindings, device_bindings 및 실제 terminal/session authority adapter |
| 20 | orders, payment_attempts, payment_allocations, refund_balances, refunds, refund_reservations |
| 30 | transaction_intents, operations, approval_bindings, approval_operation_links |
| 40 | approval_access_grants, approval_access_challenges, approval_issuance_outcomes |
| 50 | approval_nonce_slots, approval_nonce_claims, transaction_submissions, approval_dispatches, approval_dispatch_attempts |
| 60 | chain_observations, approval_observation_applications 및 기존 inbox/outbox/멱등 결과 |

보호 객체·proof replay·release decision·gate snapshot은 해당 gate 아래에서 참조 검증 또는 append한다. 삭제 writer도 같은 gate를 거쳐 연결 여부를 확인해야 한다. 기존 writer가 다른 순서로 원장 행을 먼저 잠그면 이 표와 충돌하므로 APS-M03 전환 조건에서 함께 수정할 대상이다. 이 순위는 실제 deadlock 부재를 증명하지 않는다.

READ COMMITTED를 사용할 경우 명시적 잠금과 revision CAS가 모든 경로에 있어야 한다. SERIALIZABLE 선택도 외부 RPC 원자성이나 권한 검사를 대신하지 않는다. deadlock/serialization 실패는 전체 DB transaction의 제한된 재시도 대상으로 설계하며, 재시도 블록 안에 네트워크 부작용을 넣지 않는다. 실제 lock 시험은 미수행이다. [공식 잠금 문서](https://www.postgresql.org/docs/15/explicit-locking.html)

| 처리 | 같은 commit에 포함할 내용 | 외부 동작/중단 경계 |
|---|---|---|
| 승인 생성 API-017/034/037 | 원 source 확인 → intent + parent operation → binding + parent link + 멱등 결과 | guest account/wallet 생성 없이 실제 actor로 기록 |
| AI-T01 challenge | 현재 primary auth, action, predecessor 확인 + challenge + gate snapshot | secret 발급 없음; issuance_key는 원 outcome으로 해석 |
| AI-T02 발급·교체 | gate/lineage 잠금 → proof 검증·replay 소비 → predecessor CAS → grant/outcome + linked blob + challenge 소비 | token 응답은 commit 후; blob은 먼저 보호 저장소에 stage |
| AI-T03 철회 | 관련 gate revision/상태와 grant 상태, 감사 기록 | 이미 발행한 permit/노출된 서명은 지우지 않음 |
| AI-T04 보호 읽기 | 현재 자격·scope·proof·gate 검사 + replay 소비 + release decision/snapshot | commit이 허가 시점; 전송 성공 여부와 노출 가능성을 구분 |
| SS-T03 제출 준비 | 정확한 bytes 검증 + provenance/object 참조 + submission/child link/dispatch + 멱등/outbox | 아직 전송 허가가 아님 |
| SS-T04 전송 허가 | 최신 gate/source/예약/nonce 확인 + 한 번의 permit + gate snapshot + wake-up outbox | worker는 permit과 exact bytes를 확인하고 RPC; outbox 자체는 권한 아님 |
| 관측 반영 | inbox/현재 관측 확인 + application + 원천/예약/잔액 + outbox | 중복 무효; reorg는 보정 이력으로 남김 |

서명 생성 또는 검증이 긴 작업이면 DB 잠금을 계속 쥔 채 기다리지 않는다. 결과가 도착한 뒤 최신 gate와 epoch를 다시 확인하는 별도 commit 경계로 처리한다. 이미 만들어졌을 수 있는 서명은 접수 거절 여부와 별도로 노출 가능성을 남긴다.

## 3. 순환 참조와 소속 위조 방지

- parent operation을 먼저 삽입하고 intent/binding/link를 같은 commit으로 완성한다. 부모 소속은 binding의 parent_operation_id와 일치해야 한다. 자식 operations는 원 intent/parent 소속을 바꾸지 않는다.
- challenge의 predecessor FK 때문에 **테이블 생성**은 challenge/grant를 만든 뒤 FK를 연결한다. **행 생성**은 기존 predecessor → 새 challenge → 새 grant → outcome 순서다. 최초 발급은 predecessor가 NULL이다.
- challenge consumedIssuanceId와 grant successorId를 중복 저장하지 않는다. outcome.challenge_id 및 grant.predecessor_grant_id의 고유 인덱스로 역참조한다. 소모 상태와 outcome 존재의 일치 여부는 같은 transaction에서 검사한다.
- grant/challenge/outcome은 intent·actor·sender를 복합 FK로 묶는다. actions, 실제 signer, 현재 권한, object kind/digest, submission/operation의 동일 intent는 별도 필수 transaction 검사다. nullable FK가 NULL이면 검사가 생략될 수 있으므로 variant CHECK와 NULL 쌍 검사를 함께 적용한다.
- grant 교체는 새 ID를 생성하고 기존 predecessor를 바꾸지 않는다. predecessor CAS와 단일 successor 고유성 모두 필요하다. 만료는 재인증 후 복구 대상이 될 수 있지만 revoked/compromised lineage를 부활시키지 않는다.
- 기존 데이터 중 ancestry가 없는 행은 자동 general로 분류하지 않는다. 기존 kind/원천 증거로 general임을 확인하거나 observation_only/quarantine으로 둔다. 부모·자식 result_ref는 NULL이어야 하며 보호 결과 URL 우회 경로를 만들지 않는다.

## 4. 테이블·컬럼·제약 사전

`databaseConstraints`는 향후 DDL 책임, `transactionResponsibilities`는 서비스의 원자적 검사 책임이다. 아직 둘 다 실제 실행 검증하지 않았다. timestamp/revision은 서버가 기록하며 client 입력을 그대로 신뢰하지 않는다.

### 1. `approval_bindings`

논리 자원: ApprovalBinding

| 컬럼 | 타입 | NULL |
|---|---|---|
| intent_id | app_id | 불가 |
| context_id | app_id | 불가 |
| parent_operation_id | app_id | 불가 |
| source_kind | text | 불가 |
| source_ref | app_id | 불가 |
| contract_version | text | 불가 |
| profile_id | app_id | 불가 |
| payload_digest | hash32 | 불가 |
| signer_kind | text | 불가 |
| signer_ref | app_id | 불가 |
| signer_epoch | bigint | 불가 |
| account_id | app_id | 허용 |
| wallet_id | app_id | 허용 |
| attempt_id | app_id | 허용 |
| refund_id | app_id | 허용 |
| device_session_id | app_id | 허용 |
| merchant_approver_id | app_id | 허용 |
| created_at | timestamptz | 불가 |
| revision | bigint | 불가 |
| context_digest | hash32 | 불가 |
| review_digest | hash32 | 불가 |
| snapshot_object_id | app_id | 불가 |
| snapshot_object_version | app_id | 불가 |

**DB 제약 제안**

- PK(intent_id); FK intent_id → transaction_intents.id
- UNIQUE(context_id); UNIQUE(parent_operation_id); FK parent_operation_id → operations.id
- FK profile_id → protocol_profiles.id; FK nullable account/wallet/attempt/refund/merchant_approver → existing accounts/wallets/payment_attempts/refunds/accounts
- UNIQUE(intent_id, context_id); UNIQUE(intent_id, parent_operation_id)
- CHECK source_kind IN (personal_transfer,payment,merchant_refund); signer_kind IN (hardware,cloud_mpc); signer_epoch >= 0; revision >= 0
- CHECK personal_transfer requires account_id/wallet_id, no attempt/refund/merchant_approver; payment requires attempt_id/device_session_id, signer_kind=hardware, account_id/wallet_id/refund_id/merchant_approver_id NULL; merchant_refund requires refund_id/wallet_id/merchant_approver_id, no attempt/device_session
- CHECK payment source_ref=attempt_id; merchant_refund source_ref=refund_id
- FK(snapshot_object_id,snapshot_object_version) → approval_protected_objects PK (objects created before binding or FK added afterward)

**동일 transaction/adapter 검사**

- Immutable approved context, source, profile, digest and signer epoch. Source equality with transaction_intents and parent ancestry is checked in same transaction.
- Actual signer ownership is independent from merchant approver. Device session and signer_ref resolve through authenticated registry adapters; do not invent guest accounts.
- Personal source_ref semantics follow current source contract; do not infer it equals wallet_id.
- Snapshot object kind=approval_snapshot contains exact immutable source-specific reviewProof/reviewSnapshot, context, contractVersions, VerifiedSigner selection and its revision, source associations and expiry under the existing canonical schema. Verify context/review/payload digests by selected encoding profile, never reconstruct from current state.
- transaction_intents.review_payload_ref resolves to the same versioned protected snapshot as binding.snapshot_object_id/version. Commit object metadata, intent, parent, binding and link atomically; object staged first. Binding context/review digests duplicate only verified canonical values.

### 2. `approval_operation_links`

논리 자원: ApprovalBinding

| 컬럼 | 타입 | NULL |
|---|---|---|
| operation_id | app_id | 불가 |
| intent_id | app_id | 불가 |
| role | text | 불가 |
| created_at | timestamptz | 불가 |

**DB 제약 제안**

- PK(operation_id); FK operation_id → operations.id; FK intent_id → approval_bindings.intent_id
- CHECK role IN (parent,signing_child,submission_child); partial UNIQUE(intent_id) WHERE role=parent

**동일 transaction/adapter 검사**

- Insert parent operation, intent, binding, then parent link in one transaction; children link before response.
- Parent operation_id must equal binding.parent_operation_id; all approval-linked operations.result_ref must be NULL. Cross-table service check (future constrained writer/trigger), not row CHECK.
- Missing ancestry is denied/quarantined, never classified general by absence alone. General classification requires known operation kind/provenance.

### 3. `approval_protected_objects`

논리 자원: SignedPayloadObject, ProtectedIssuanceResponse

| 컬럼 | 타입 | NULL |
|---|---|---|
| object_id | app_id | 불가 |
| object_version | app_id | 불가 |
| kind | text | 불가 |
| opaque_store_ref | text | 불가 |
| content_digest | hash32 | 불가 |
| byte_length | bigint | 불가 |
| encryption_key_ref | text | 불가 |
| state | text | 불가 |
| replay_expires_at | timestamptz | 허용 |
| created_at | timestamptz | 불가 |
| revision | bigint | 불가 |

**DB 제약 제안**

- PK(object_id,object_version); UNIQUE(opaque_store_ref); CHECK byte_length>0; revision>=0
- CHECK kind IN (approval_snapshot,signed_payload,issuance_response); state IN (linked,delete_pending,deleted,missing); issuance_response requires replay_expires_at

**동일 transaction/adapter 검사**

- Encrypted bytes durably staged and digest/length checked before reference commit; storage adapter has no public URL.
- Staged unlinked blobs live outside this table under upload ID. Orphan cleanup checks committed reference and in-flight staging grace; deletion is idempotent and does not remove evidence metadata.
- Signed payload retention is independent of issuance response TTL. Key reference is not key material. Missing bytes cannot erase signature exposure.
- approval_snapshot is immutable canonical-schema content with encrypted storage and digest/length metadata; snapshot retention is independent of issuance replay TTL.

### 4. `approval_provenance`

논리 자원: SignatureProvenance

| 컬럼 | 타입 | NULL |
|---|---|---|
| evidence_id | app_id | 불가 |
| intent_id | app_id | 불가 |
| kind | text | 불가 |
| verifier_profile_id | app_id | 불가 |
| verifier_version | text | 불가 |
| signed_payload_digest | hash32 | 불가 |
| device_id | app_id | 허용 |
| device_epoch | bigint | 허용 |
| signing_session_id | app_id | 허용 |
| proof_object_ref | text | 불가 |
| verified_at | timestamptz | 불가 |

**DB 제약 제안**

- PK(evidence_id); FK intent_id → approval_bindings.intent_id; FK verifier_profile_id → protocol_profiles.id; FK device_id → devices.id; FK signing_session_id → signing_sessions.id
- UNIQUE(evidence_id,intent_id,signed_payload_digest)
- CHECK kind=hardware requires device_id/device_epoch>=0 and signing_session_id NULL; kind=cloud_mpc requires signing_session_id and device_id/device_epoch NULL

**동일 transaction/adapter 검사**

- Recompute signature, decoded transaction and approved payload equality with selected verifier; profile/version and current signer epoch validated.
- Hardware guest evidence has no invented signing_sessions or wallets row. proof_object_ref uses protected evidence adapter with access/retention contract.

### 5. `approval_authorization_gates`

논리 자원: AuthorizationGate, SourceExecutionGate

| 컬럼 | 타입 | NULL |
|---|---|---|
| gate_key | text | 불가 |
| scope_kind | text | 불가 |
| scope_ref | text | 불가 |
| generation | bigint | 불가 |
| revision | bigint | 불가 |
| state | text | 불가 |
| updated_at | timestamptz | 불가 |

**DB 제약 제안**

- PK(gate_key); UNIQUE(scope_kind,scope_ref); CHECK generation>=0; revision>=0; state IN (open,held,revoked)

**동일 transaction/adapter 검사**

- Shared authority for source and access. No independent source-gate shadow. Scope registry covers account, auth session, terminal session, device binding/epoch, membership, source and grant lineage.
- Every revocation/source transition writer locks and updates these same gates with actual authority rows in one transaction. External identity events put relevant gate on hold until reconciled.
- Missing gate is denial, never implicit open. Generation increments on new authority; old rows/generation never resurrect grants.

### 6. `approval_access_challenges`

논리 자원: AccessChallenge

| 컬럼 | 타입 | NULL |
|---|---|---|
| challenge_id | app_id | 불가 |
| intent_id | app_id | 불가 |
| context_id | app_id | 불가 |
| actor_kind | text | 불가 |
| actor_ref | text | 불가 |
| sender_key_digest | hash32 | 불가 |
| actions | text[] | 불가 |
| nonce_hash | hash32 | 불가 |
| request_digest | hash32 | 불가 |
| predecessor_grant_id | app_id | 허용 |
| predecessor_revision | bigint | 허용 |
| expires_at | timestamptz | 불가 |
| state | text | 불가 |
| created_at | timestamptz | 불가 |
| revision | bigint | 불가 |

**DB 제약 제안**

- PK(challenge_id); FK(intent_id,context_id) → approval_bindings(intent_id,context_id)
- UNIQUE(challenge_id,intent_id,actor_kind,actor_ref,sender_key_digest); UNIQUE(nonce_hash)
- FK predecessor_grant_id → approval_access_grants.grant_id (added after both tables exist)
- CHECK actor_kind IN (account,terminal); actions nonempty subset of read_progress/read_snapshot/read_signing_result/submit_exact; state IN (open,consumed,cancelled,expired); expires_at>created_at
- CHECK predecessor_grant_id and predecessor_revision are both NULL or both NOT NULL

**동일 transaction/adapter 검사**

- actor_ref is server-canonical logical account or exact terminal session; registry/auth adapters verify it. Challenge snapshots source/device/session generations through gate snapshots.
- Replacement issuance_key is resolved server-side to original grant before creating challenge; never uses expired grant as primary auth.
- Consumed challenge ↔ one issuance outcome checked at commit; no consumedIssuanceId back-reference needed.

### 7. `approval_access_grants`

논리 자원: AccessGrantLineage

| 컬럼 | 타입 | NULL |
|---|---|---|
| grant_id | app_id | 불가 |
| intent_id | app_id | 불가 |
| challenge_id | app_id | 불가 |
| actor_kind | text | 불가 |
| actor_ref | text | 불가 |
| sender_key_digest | hash32 | 불가 |
| actions | text[] | 불가 |
| token_verifier_hash | hash32 | 불가 |
| predecessor_grant_id | app_id | 허용 |
| state | text | 불가 |
| expires_at | timestamptz | 불가 |
| created_at | timestamptz | 불가 |
| revision | bigint | 불가 |

**DB 제약 제안**

- PK(grant_id); UNIQUE(token_verifier_hash); UNIQUE(challenge_id); UNIQUE(predecessor_grant_id) WHERE predecessor_grant_id IS NOT NULL
- UNIQUE(grant_id,intent_id,actor_kind,actor_ref,sender_key_digest)
- FK(challenge_id,intent_id,actor_kind,actor_ref,sender_key_digest) → approval_access_challenges(challenge_id,intent_id,actor_kind,actor_ref,sender_key_digest)
- FK(predecessor_grant_id,intent_id,actor_kind,actor_ref,sender_key_digest) → approval_access_grants(grant_id,intent_id,actor_kind,actor_ref,sender_key_digest)
- CHECK state IN (active,replaced,revoked,expired,compromised); revision>=0; expires_at>created_at; predecessor_grant_id<>grant_id when present; actions allowed nonempty subset

**동일 transaction/adapter 검사**

- Root predecessor NULL may repeat. Successor is derived by predecessor index; avoid reciprocal successor_id cycle.
- Under lineage gate, predecessor CAS on expected revision/state; new node must not exist, parent immutable. Check action scope, lineage generations and challenge predecessor equality; no cycle or revoked/compromised ancestry resurrection.
- One successor and one challenge consumption do not alone authorize issuance; current primary actor/sender/authority rechecked.

### 8. `approval_issuance_outcomes`

논리 자원: ProtectedIssuanceResponse

| 컬럼 | 타입 | NULL |
|---|---|---|
| issuance_id | app_id | 불가 |
| challenge_id | app_id | 불가 |
| grant_id | app_id | 불가 |
| intent_id | app_id | 불가 |
| actor_kind | text | 불가 |
| actor_ref | text | 불가 |
| sender_key_digest | hash32 | 불가 |
| action | text | 불가 |
| request_key | text | 불가 |
| request_digest | hash32 | 불가 |
| response_object_id | app_id | 불가 |
| response_object_version | app_id | 불가 |
| created_at | timestamptz | 불가 |

**DB 제약 제안**

- PK(issuance_id); UNIQUE(challenge_id); UNIQUE(grant_id)
- UNIQUE(actor_kind,actor_ref,action,intent_id,sender_key_digest,request_key)
- FK(challenge_id,intent_id,actor_kind,actor_ref,sender_key_digest) → approval_access_challenges composite key; FK(grant_id,intent_id,actor_kind,actor_ref,sender_key_digest) → approval_access_grants composite key
- FK(response_object_id,response_object_version) → approval_protected_objects PK; CHECK action=issue_access

**동일 transaction/adapter 검사**

- Grant.challenge_id must equal outcome.challenge_id; encrypted response references original grant token, never a newly minted retry token.
- Same business key + different request_digest conflicts. Same logical actor can reauthenticate in another session; guest retains exact original terminal/session/attempt/device epoch.
- Metadata retained independently from expiring blob and ordinary idempotency_records. Blob deletion does not delete locator; current authorization required for replay and issuance_key recovery.

### 9. `approval_gate_snapshots`

논리 자원: AuthorizationGate, AccessChallenge, AccessGrantLineage, SubmissionDispatch

| 컬럼 | 타입 | NULL |
|---|---|---|
| snapshot_id | app_id | 불가 |
| gate_key | text | 불가 |
| generation | bigint | 불가 |
| gate_revision | bigint | 불가 |
| challenge_id | app_id | 허용 |
| grant_id | app_id | 허용 |
| dispatch_id | app_id | 허용 |
| release_id | app_id | 허용 |
| phase | text | 불가 |

**DB 제약 제안**

- PK(snapshot_id); FK gate_key → approval_authorization_gates.gate_key; FK nonnull owner to corresponding approval table
- CHECK exactly one of challenge_id/grant_id/dispatch_id/release_id is nonnull; generation>=0; gate_revision>=0
- Partial UNIQUE(owner_id,phase,gate_key) for each of four owner kinds

**동일 transaction/adapter 검사**

- Persist complete server-resolved required gate set, not client supplied subset. Dispatch phases prepared/permit distinguished.
- Stored revision is historical decision evidence, not permission cache. Lock and check latest gates at every issue/replace/release/permit.

### 10. `approval_sender_replay`

논리 자원: SenderProofReplay

| 컬럼 | 타입 | NULL |
|---|---|---|
| replay_id | app_id | 불가 |
| profile_id | app_id | 불가 |
| namespace_kind | text | 불가 |
| namespace_id | app_id | 불가 |
| nonce_digest | hash32 | 불가 |
| request_digest | hash32 | 불가 |
| consumed_at | timestamptz | 불가 |
| reject_until | timestamptz | 불가 |
| outcome_ref | app_id | 허용 |

**DB 제약 제안**

- PK(replay_id); FK profile_id → protocol_profiles.id; UNIQUE(profile_id,namespace_kind,namespace_id,nonce_digest)
- CHECK namespace_kind IN (challenge,grant); reject_until>consumed_at

**동일 transaction/adapter 검사**

- Namespace target is validated under same transaction; profile fixes freshness/skew/max lifetime so collection cannot admit still-valid proof.
- Same transport proof is rejected even for same business request; recover idempotent outcome with fresh proof/current authorization. Business GET may write only authorization metadata.
- Replay consume and associated issuance/release/permit decision commit atomically; proof validation alone does not consume business nonce.

### 11. `approval_release_decisions`

논리 자원: AuthorizationGate, SignedPayloadObject, ProtectedIssuanceResponse

| 컬럼 | 타입 | NULL |
|---|---|---|
| release_id | app_id | 불가 |
| intent_id | app_id | 불가 |
| grant_id | app_id | 허용 |
| actor_kind | text | 불가 |
| actor_ref | text | 불가 |
| sender_key_digest | hash32 | 허용 |
| action | text | 불가 |
| object_id | app_id | 허용 |
| object_version | app_id | 허용 |
| decision_digest | hash32 | 불가 |
| committed_at | timestamptz | 불가 |
| delivery_state | text | 불가 |
| auth_path | text | 불가 |
| primary_session_ref | text | 허용 |

**DB 제약 제안**

- PK(release_id); FK intent_id → approval_bindings.intent_id; FK(grant_id,intent_id,actor_kind,actor_ref,sender_key_digest) → approval_access_grants composite key
- FK(object_id,object_version) → approval_protected_objects PK; CHECK object_id/object_version both null or both nonnull
- CHECK action IN (read_progress,read_snapshot,read_signing_result,replay_issuance); delivery_state IN (not_observed,delivery_attempted,acknowledged)
- CHECK auth_path IN (primary,grant); primary requires primary_session_ref and grant_id NULL; grant requires grant_id/sender_key_digest and primary_session_ref NULL; replay_issuance requires sender_key_digest

**동일 transaction/adapter 검사**

- Commit is release authorization linearization point after gate/primary or grant checks. A revoke committed before it prevents release; after it cannot recall permitted bytes.
- Decision is not a reusable download capability. Every retry/new response requires fresh authorization. A crash after decision is possible exposure even without delivery ACK.
- Snapshot/progress release records may omit object reference; signed bytes/replay require correct protected object kind and exact digest.
- Approval API019/020 primary account/source ACL reads may have no sender key: store real authenticated session and NULL sender_key_digest, never synthetic digest. Grant path requires actual sender proof. Generic nonapproval operations remain outside this table.
- primary_session_ref resolves to current authenticated account or terminal session through authority adapter; optional sender proof, when present, is verified and replay-consumed. Replay issuance uses current primary auth plus original sender binding.

### 12. `approval_dispatches`

논리 자원: SubmissionDispatch

| 컬럼 | 타입 | NULL |
|---|---|---|
| dispatch_id | app_id | 불가 |
| intent_id | app_id | 불가 |
| submission_id | app_id | 불가 |
| operation_id | app_id | 불가 |
| evidence_id | app_id | 불가 |
| signed_payload_digest | hash32 | 불가 |
| object_id | app_id | 불가 |
| object_version | app_id | 불가 |
| state | text | 불가 |
| permit_issued_at | timestamptz | 허용 |
| permit_revision | bigint | 허용 |
| created_at | timestamptz | 불가 |
| revision | bigint | 불가 |

**DB 제약 제안**

- PK(dispatch_id); UNIQUE(intent_id); UNIQUE(submission_id); UNIQUE(operation_id)
- FK intent_id → approval_bindings.intent_id; FK submission_id → transaction_submissions.id; FK operation_id → approval_operation_links.operation_id
- FK(evidence_id,intent_id,signed_payload_digest) → approval_provenance composite key; FK(object_id,object_version) → approval_protected_objects PK
- CHECK state IN (prepared,permitted,broadcast_unknown,observed,rejected); permit_issued_at/permit_revision both null or both nonnull; revision>=0

**동일 transaction/adapter 검사**

- One accepted payload per immutable intent; changing payload requires new approval, not overwrite. Verify submission.intent_id and operation link intent/role and protected object digest/kind in transaction.
- SS-T03 prepares only. SS-T04 locks current gates/source/reservations/nonce slot, appends permit snapshots and sets permit once. Nonnull permit never cleared or replaced.
- Existing EOA tx hash uniqueness retained; submission generic state does not replace dispatch authority. Outbox wake-up cannot grant permit.

### 13. `approval_dispatch_attempts`

논리 자원: DispatchAttempt

| 컬럼 | 타입 | NULL |
|---|---|---|
| attempt_id | app_id | 불가 |
| dispatch_id | app_id | 불가 |
| attempt_number | bigint | 불가 |
| fence | bigint | 불가 |
| worker_ref | text | 불가 |
| lease_until | timestamptz | 불가 |
| permit_revision | bigint | 불가 |
| request_digest | hash32 | 불가 |
| state | text | 불가 |
| response_ref | text | 허용 |
| created_at | timestamptz | 불가 |

**DB 제약 제안**

- PK(attempt_id); UNIQUE(dispatch_id,attempt_number); FK dispatch_id → approval_dispatches.dispatch_id; CHECK attempt_number>0; fence>0
- CHECK state IN (claimed,rpc_possible,rpc_returned,uncertain,completed)

**동일 transaction/adapter 검사**

- Only permitted dispatch can create send attempt; permit revision and exact payload must match. CAS lease/fence protects DB updates.
- Record rpc_possible before remote call; crash or timeout after that is uncertain. External RPC cannot participate in DB transaction or be undone by newer fence.
- Retry only exact raw transaction after reconciliation; do not allocate new nonce or new spend automatically.

### 14. `approval_nonce_slots`

논리 자원: AddressNonceTrack

| 컬럼 | 타입 | NULL |
|---|---|---|
| slot_id | app_id | 불가 |
| chain_id | bigint | 불가 |
| payer | evm_address | 불가 |
| nonce | uint256_amount | 불가 |
| revision | bigint | 불가 |

**DB 제약 제안**

- PK(slot_id); UNIQUE(chain_id,payer,nonce); CHECK chain_id=8283; revision>=0

**동일 transaction/adapter 검사**

- EOA nonce slot only; smart account nonce namespace needs separate typed adapter. Not an external-wallet lock.

### 15. `approval_nonce_claims`

논리 자원: AddressNonceTrack

| 컬럼 | 타입 | NULL |
|---|---|---|
| claim_id | app_id | 불가 |
| slot_id | app_id | 불가 |
| tx_hash | hash32 | 불가 |
| intent_id | app_id | 허용 |
| origin | text | 불가 |
| state | text | 불가 |
| created_at | timestamptz | 불가 |

**DB 제약 제안**

- PK(claim_id); FK slot_id → approval_nonce_slots.slot_id; FK intent_id → approval_bindings.intent_id; UNIQUE(slot_id,tx_hash)
- CHECK origin IN (local,external); local requires intent_id; CHECK state IN (possible,pending,canonical,orphaned,replaced)

**동일 transaction/adapter 검사**

- Allow competing hashes in same slot. Decode/verify chain,payer,nonce against slot, prove replacement via current canonical observation; notfound alone not proof.
- One tx can have reorg history in observations. Privacy permissions remain intent/source-bound, never shared solely by address/nonce.

### 16. `approval_observation_applications`

논리 자원: ObservationApplication

| 컬럼 | 타입 | NULL |
|---|---|---|
| application_id | app_id | 불가 |
| source_kind | text | 불가 |
| source_ref | app_id | 불가 |
| observation_id | app_id | 불가 |
| observation_revision | bigint | 불가 |
| prior_ledger_revision | bigint | 불가 |
| applied_ledger_revision | bigint | 불가 |
| effect_digest | hash32 | 불가 |
| outbox_id | app_id | 불가 |
| created_at | timestamptz | 불가 |

**DB 제약 제안**

- PK(application_id); FK observation_id → chain_observations.id; FK outbox_id → outbox.id
- UNIQUE(source_kind,source_ref,observation_id,observation_revision); CHECK observation_revision>=0; applied_ledger_revision>prior_ledger_revision

**동일 transaction/adapter 검사**

- Revision event is applied only with locked current source/canonicality and ledger revision. Existing chain_observations row is mutable: archive validated event revision in inbox/evidence adapter; FK alone does not prove historical revision.
- Duplicate application has no ledger effect. Out-of-order events cannot overwrite newer state; reconcile current observation or hold for missing history.
- Refund reorg moves confirmed back to reserved atomically with reservation/balance/refund/outbox. Never free capacity while exposure remains.

## 5. 8단계 이행 순서

아래는 향후 마이그레이션의 작업 명세이며 이번 턴에 실행한 절차가 아니다. 단계별 출구 조건을 만족하기 전 다음 단계 실행을 허용하지 않는다. 빈 DB 적용과 기존 데이터 이행을 구분하고, 이력을 채우기 위해 존재하지 않았던 암호 증거를 만들지 않는다.

### APS-M01 · inventory_and_freeze

선행: 없음

Record SQL hashes, counts, source/operation classifications and active workers; select reference engine/version for later rehearsal. Inventory ambiguous signatures/permits; hold affected sources.

출구 조건: No execution enabled; RR-DEC-01 unchanged.

### APS-M02 · expand_storage

선행: APS-M01

Propose new tables in dependency groups: object/gate/nonce slot then binding; operation links/provenance; challenges+grants without cross FKs then add; outcomes/releases/dispatch; attempts/claims/applications/snapshots/replay. Table creation order differs from row insertion order.

출구 조건: All FK targets expose full PK/UNIQUE keys. Partial successor uniqueness is not used as an FK target. Existing seven migrations immutable.

### APS-M03 · coordinate_writers

선행: APS-M02

Route authority/source/revoke writers through shared gate protocol; quiesce incompatible workers during switch. New records atomically write companion records and existing rows. Queue messages carry contract version; old sender cannot bypass permit.

출구 조건: No eventual dual-write for gates/lineage. Reject unknown/new approval work on old writers; preserve existing authorized observers.

### APS-M04 · classify_and_backfill

선행: APS-M03

Batch with source revision+cursor+digest; concurrent change causes retry. Categories: verified_complete, observation_only, quarantine. Backfill only independently present identity/context/provenance; preserve actual existing exposure.

출구 조건: No generated historical signatures, epochs, permits or guest accounts. NULL unknown is classification outside active binding, not fake zero. Legacy rows lacking proof remain blocked for new execution.

### APS-M05 · validate_constraints

선행: APS-M04

Reconcile counts and source ownership; scan duplicates and missing ancestors. Future migration rehearsal must verify constraints and lock impact. NOT VALID/VALIDATE applies only eligible constraints; unique indexes require separate duplicate remediation.

출구 조건: All enabled-source records satisfy constraints; quarantined old records excluded from execution, not discarded. Time/lock budget and recovery checkpoints required before real rollout.

### APS-M06 · cutover_per_source

선행: APS-M05

Enable typed personal/payment/refund source adapters separately; readers select persisted version. Compare projections; switch current authority/permit writer together. Unsupported smart-account/market/DID/x402 sources remain execution blocked pending their adapters.

출구 조건: Runtime evidence is mandatory later; this design does not authorize live cutover. No generic fallback for failed/new approval records.

### APS-M07 · observe_and_retain

선행: APS-M06

Track denied stale grants, uncertain broadcasts, replay conflicts, reconciliation lag and object availability with secret-free metrics. Preserve issuance metadata, lineage and exposure until retention policy permits disposal.

출구 조건: No arbitrary TTL selected. RR-DEC-01 and deletion/recovery policies retained; cleanup requires auditable eligibility.

### APS-M08 · forward_recovery_or_reader_rollback

선행: APS-M07

On fault disable new release/permit, keep observations and reconcile. Restore only a compatible reader that understands blocked/new records. Recover with forward fix; retain grants/revocations/permit history and object tombstones.

출구 조건: Never rollback to writer that ignores gates; never truncate exposure history or re-enable revoked tokens. No destructive down migration.

NOT VALID/VALIDATE를 모든 제약에 동일하게 적용할 수 있다고 가정하지 않는다. 실제 ALTER TABLE과 인덱스 작업의 잠금, 재시작 방식, 대용량 검증 시간은 선택한 엔진/버전과 데이터로 확인해야 한다. 이 문서는 무중단 이행을 보장하지 않는다. [PostgreSQL ALTER TABLE 공식 문서](https://www.postgresql.org/docs/15/sql-altertable.html)

이행 실행 기록에는 최소 `migration_id, batch_id, source_revision, cursor, input_digest, classification, checked_count, conflict_count, quarantine_reason, started_at, completed_at`을 남기는 제안이다. 이 기록의 실제 물리 테이블/저장 도구는 아직 선정하지 않았다. 개인정보/토큰/원본 서명 bytes를 기록에 넣지 않는다.

## 6. 장애·동시성 수용 사례

모든 사례는 **not_run**이다. 문서 구조 검사가 이 시나리오의 동시 실행이나 DB 복구를 입증하지 않는다.

| ID | 상황 | 기대 결과 |
|---|---|---|
| APS-R01 Concurrent replacements of one grant | Two challenges target same predecessor revision. | Only one successor commits; loser conflict, fresh auth and reread; no second token response. |
| APS-R02 Revoke wins before issuance/release | Revocation commits before gate lock decision. | Issuance/release denied; no protected response; independent observation continues. |
| APS-R03 Release wins before revoke | Release decision commits, then revoke, before socket delivery. | Possible exposure persists; cannot promise recalled bytes. Subsequent response/replay needs current authorization. |
| APS-R04 Staged object then process crash | Encrypted blob durable, no SQL commit. | No authority granted. Orphan GC checks transaction reference/grace; retry uses business key without inventing commit. |
| APS-R05 Issuance committed, response lost | Grant/outcome/challenge commit then connection loss. | Fresh sender proof + current primary auth resolves original outcome; exact response while retained, otherwise issuance_key replacement through single-successor check. |
| APS-R06 Response blob expired | Token verifier remains but encrypted replay object deleted. | Keep issuance locator; cannot reconstruct token from verifier; authenticated replacement only; revoked lineage stays revoked. |
| APS-R07 Prepared worker wakes late | Outbox delivered after epoch/source revoked. | Prepared is no permit. Latest gate rejects SS-T04; outbox itself never authorizes broadcast. |
| APS-R08 Permit committed, RPC result lost | rpc_possible marker exists; response/worker disappears. | Mark uncertain; query original hash and nonce claims. Exact-byte retry only under reconciliation policy; no new spend or refund reservation release. |
| APS-R09 Old worker outlives lease | New fence assigned while old worker reaches RPC. | Reject stale DB updates; external duplicate broadcast remains possible. Same raw transaction hash, preserve all attempts; lease does not revoke permit. |
| APS-R10 Duplicate or reversed observations | Same revision redelivered or older revision arrives late. | Application unique key deduplicates; locked ledger/current observation rejects stale overwrite; reconcile missing revision history. |
| APS-R11 Refund confirmation becomes orphaned | Limit10/reserved2/confirmed6/available2 before reorg. | Atomic result reserved8/confirmed0/available2; keep exposure, no free refund capacity; emit compensating outbox. |
| APS-R12 Foreign transaction consumes nonce | Different hash observed for same payer+nonce. | Preserve competing claims; verify canonical receipt/finality and source obligations before terminal replacement decision. Never inherit other intent ACL. |
| APS-R13 Backup omits recent revocation | Restore snapshot older than latest gate/revoke/permit. | Hold affected scopes and all protected release/permit until authoritative recovery watermark proven. Reconcile external chain independently; absent log is not evidence of no revoke/exposure. |
| APS-R14 Database deadlock or serialization retry | Transaction aborted during multiple-source lock contention. | Retry bounded whole transaction with same business key and fresh authorization; never repeat external side effect inside retry closure. |
| APS-R15 Backfill overlaps live change | Source revision differs from cursor snapshot. | CAS rejects stale write; reclassify/retry; no overwrite of newer signer epoch or ancestry. |
| APS-R16 Linked payload disappears | Blob/key unavailable after reference commit. | Hold release/broadcast and repair availability; keep digest/provenance/permit/possible exposure. Do not report unsigned or release reservation. |
| APS-R17 Rollback sees unfamiliar operation | Old reader cannot interpret approval child or source. | Block incompatible endpoint while keeping supported observation path; never generic resultRef or legacy signing fallback. |

## 7. 보관·복구·미결정 항목

토큰 verifier, 암호화 응답 blob, issuance locator, 서명 bytes, provenance, permit, replay tombstone은 같은 TTL로 삭제하지 않는다. response blob이 만료되어도 원 발급 위치를 찾아 현재 인증으로 교체할 수 있어야 한다. replay 기록은 선택한 proof profile의 최대 유효 기간·허용 clock skew·복구 지연을 감안해 유효한 증거가 재사용되지 않는 기간까지 남긴다. 구체 기간은 미정이다.

백업 복원 후 최신 revocation/permit/삭제 상태를 증명할 기준 watermark가 없으면 해당 scope의 release/permit을 보류한다. 예전 snapshot에 없다는 이유로 새 token이나 서명을 발급하지 않는다. WAL/감사 원장/외부 최신 authority 중 무엇을 복구 근거로 쓸지와 보관·삭제 tombstone 재적용은 후속 운영 설계에서 정한다. chain 관측은 별도로 재구축할 수 있지만 최신 off-chain 철회 기록을 대신할 수 없다.

운영 DB·isolation·writer 권한/trigger 선택, 보호 저장소와 키 관리, crypto profile/MPC 구성, 보관 기간, RR-DEC-01은 확정하지 않았다. 이번 안은 승인 경로의 물리 설계 제안이지 제품 전체 스키마 완성이나 Seed 승인이 아니다. smart account/market/DID/x402 adapter도 범위에서 제거하지 않고 기존 실행 차단 상태를 유지한다.

다음 설계는 **결제·환불 대사 후보를 현재 기준에 연결하는 변경 명세**다. observation application과 기존 주문/배정/환불/정산 원장의 상태·API·event revision을 맞추고, 미병합 commerce 계약의 차이를 줄인다.

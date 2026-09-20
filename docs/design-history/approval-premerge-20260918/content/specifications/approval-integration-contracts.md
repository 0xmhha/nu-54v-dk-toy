# 승인 API·권한·화면·저장 경계 연결

2026-09-18 · **상세 설계 후보. 기준 명세 미병합, 제품 구현·실기·암호·DB 실행 검증 미수행.**

개인 송금·guest NU 결제·가맹점 환불의 공통 승인 타입을 기존 API 10개와 신규 제안 AC-01~03의 전체 논리 HTTP 요청/응답으로 연결했다. 화면 17개를 변경 대상과 기존 기능 회귀 대상으로 구분하고, 접근 자격의 저장 자원 5개·원자 처리 경계 5개를 정의했다.

[계약·매핑 원본](approval-integration-candidate.json) · [HTTP Schema](approval-integration.schema.json) · [합성 예제](approval-integration-examples.json) · [검증기](validate_approval_integration.py) · [선행 공통 타입·자격 계약](approval-access-contracts.md)

후속 [기준 반영 순서·호환 수용표](approval-adoption-plan.md)에 이력 보존, 실제 반영 단계와 버전/profile 수용 조건을 연결했다. 후보를 기준 명세나 실행 호환 결과로 승격한 것은 아니다.

## 1. 버전과 적용 범위

HTTP 버전은 `approval-v1-draft`다. 공통 타입, 개인/guest/환불 후보, 기존 DTO는 URI alias와 작성 시점 파일 해시로 참조한다. 기존 파일을 덮어쓰거나 해시만 갱신하지 않았다. HTTP 버전과 BLE 버전·증거 profile을 별도로 검증한다.

새 계약은 개인 EOA 송금·NU guest 결제·매장 EOA 환불의 세 source를 대상으로 한다. smart account, DeFi/FX/perpetual/STO/x402 등 다른 거래 source를 이 타입으로 자동 변환하지 않는다. 해당 기존 operation·거래 관측은 원 계약과 접근 정책을 유지한다. 전체 15개 요구를 축소한 것이 아니며 기능별 adapter/호환 판정은 계속 필요하다.

API-007/008/009의 지갑 조회는 기존 WalletChoice/잔액/선택 조건을 유지한다. API-032의 attempt/challenge와 API-035/038의 업무 결과도 기존 연결 경계다. 새 AC 경로 3개는 아직 카탈로그에 등록하지 않았으므로 기준 API 수는 107개다.

## 2. 전체 HTTP 계약

모든 요청은 `path`, `headers`, `query` 또는 `body`를 구분한다. 응답은 `statusCode`, `headers`, `body={requestId,data}`이며 오류는 `body={requestId,error}`다. additionalProperties를 허용하지 않는 envelope로 정의했다. 표의 요약과 달리 실제 schema에는 전체 요청/응답이 있다.

| 경로 ID | 주요 요청 | 성공 응답의 data | 현재 자격 |
|---|---|---|---|
| API-007 GET /v1/wallets | 개인 목록 query/커서 | 기존 개인 WalletChoice 목록 | 계정 |
| API-008 GET /v1/stores/{storeId}/wallets | store path/목록 query | 기존 매장 WalletChoice 목록 | 해당 매장 자산 조회 |
| API-009 GET /v1/wallets/{walletId}/balances | wallet path/명시 context | 기존 잔액 projection | 해당 wallet/context 조회 |
| API-017 POST /v1/transaction-intents | 개인 transfer와 예상 selection revision | PersonalSnapshot, 부모 operation | 현재 개인 wallet_use |
| API-034 POST /v1/payment-attempts/{attemptId}/intents | signerAddress/session/identify 증거 | PaymentSnapshot, 부모 operation | 원 attempt/device/session |
| API-037 POST /v1/refunds/{refundId}/authorize | 두 예상 revision, merchantSigner | RefundView, RefundSnapshot, 부모 operation | 해당 매장 자금 관리·최근 인증 |
| API-015 POST /v1/wallets/{walletId}/signing-sessions | intent/context/사용자 approvalProof | MPC 자식 operation, approvalOperationId, progress | 실제 Cloud signer의 현재 서명권 |
| API-018 POST /v1/transaction-submissions | SigningResult 전체 | 제출 operation, transactionRef, approvalOperationId | sender 결합 submit_exact grant |
| API-019 GET /v1/transactions/{transactionRef} | 정확한 transactionRef | 기존 체인 관측/확정 projection | transaction_read 또는 원 grant의 제한된 진행 조회 |
| API-020 GET /v1/operations/{operationId} | includeSnapshot/includeSigningResult | operation, operationClass, approvalOperationId, approval(null 또는 공통 projection) | 원 operation/source의 현재 조회권 |
| AC-01 POST …/access-challenges | action·sender·replacement locator | challenge·대상·발급 가능 action·replacesGrantId | 현재 계정 또는 terminal 인증 |
| AC-02 POST …/access-grants | challengeId/senderProof | 실제 accessToken과 서버 확정 grant | 위 현재 인증과 sender proof |
| AC-03 POST …/access-grants/{grantId}/revoke | 철회 사유 | grantId/철회 상태·시각 | 해당 자격 철회권 |

AC 접두 경로는 `/v1/approval-contexts/{approvalContextId}`다. API 번호·method/path는 기존 것을 유지했으며, AC 세 경로만 미등록 제안이다. API-020의 202는 기존 설계 envelope를 유지한 값으로, operation이나 지급이 완료됐다는 뜻이 아니다. 생성/승인/서명/제출/체인 확정은 서로 다른 결과다.

### Header와 query의 논리 값↔HTTP 값

| 논리 필드 | HTTP 표현/검사 |
|---|---|
| headers.requestId | X-Request-Id |
| headers.authorization | Authorization: Bearer …; 서버가 실제 자격 종류와 발급 기록을 판별 |
| headers.contractVersion | X-Contract-Version: approval-v1-draft |
| headers.idempotencyKey | Idempotency-Key; 변경 요청에 필수 |
| headers.senderProof | Approval-Sender-Proof; EvidencePacket을 UTF-8 JSON 후 base64url로 운반하는 후보 |
| 응답 headers.cacheControl | Cache-Control: no-store |
| 응답 headers.contractVersion | X-Contract-Version: approval-v1-draft |

JSON header 운반은 proof의 암호 profile을 확정한 것이 아니다. decoder는 중복 key, 모호한 인코딩, profile/게이트웨이 크기 제한 초과를 거절한다. proof가 결합할 실제 method/path/query/body/token/audience와 nonce 규칙은 선정 profile에 고정해야 한다. raw HTTP header/body를 로그로 출력하지 않는다. 인프라 크기 제한과 profile은 배포 전 확인 항목이다.

query의 includeSnapshot/includeSigningResult는 각각 생략하면 false다. HTTP에서는 정확히 `true`/`false` 문자열만 파싱하고 중복 key·미지원 query·그 밖의 값은 400으로 처리한다. schema 예제는 파싱 후 boolean이다. 사용자 입력 문자열을 truthy로 변환하지 않는다.

API-018은 senderProof가 필수다. API-019/020에서는 기존 계정/기능별 권한 경로도 유지하므로 schema의 senderProof는 선택 필드지만, **approval grant 사용 시 반드시 검증**한다. 선택 필드라는 이유로 grant의 sender 결합을 생략하지 않는다. API-015/017/034/037/AC-01~03의 Authorization을 approval accessToken으로 대체해 서명·업무 승인·발급 권한을 우회할 수 없다.

## 3. 부모·자식 operation과 결과 공개

API-017/034/037이 만든 operation이 승인 부모다. snapshot.context.operationId와 일치한다. API-015의 operation은 실제 MPC 실행 자식이고 별도의 approvalOperationId를 반환한다. API-018의 operation은 제출 작업이며 동일하게 approvalOperationId를 명시한다. 클라이언트가 마지막으로 받은 자식 ID로 승인 부모를 덮어쓰지 않는다.

승인 진행·원 snapshot·서명 결과는 **부모 API-020**에서 조회한다. 자식의 기존 실행 상태를 조회하더라도 parent의 signed result를 자동 공개하지 않는다. DB에 기록된 parent/context/signer/epoch 관계로 완료된 자식 결과를 연결한다. 자식 상태=succeeded만으로 원 승인 결과가 맞다고 인정하지 않는다.

API-020에서 서버가 저장된 승인 ancestry를 확인해 operationClass를 approval_parent/approval_child/general로 결정한다. 클라이언트가 분류를 선택할 수 없다. 부모/자식 모두 operation.resultRef는 반드시 null이며, approvalOperationId로 원 부모를 명시한다. 부모만 approval projection을 갖고 자식은 approval=null이다. 자식이 null projection이라는 이유로 일반 operation의 보호 결과 경로를 사용하지 못한다. ancestry를 확정할 수 없는 승인 작업은 일반 작업으로 fallback하지 않고 결과 공개를 차단한다.

API-015/017/018/034/037의 승인 관련 operation 응답도 resultRef=null이다. 원시 서명·token·보호 객체 주소를 우회 전달하지 않는다. 실제 일반 녹음/AI/FOTA operation은 operationClass=general, approvalOperationId=null, approval=null이고 기존 기능별 보호 resultRef/조회 정책을 유지한다. 승인 자식에 projection query=true를 보내면 부모를 새로 조회하도록 422로 거절하며 부모의 현재 결과 ACL을 재검사한다.

| 요청 | 필요한 현재 권한 | 반환 |
|---|---|---|
| API-020 기본 | read_progress 또는 이에 대응하는 source ACL | progress, snapshot=null, signingResult=null |
| includeSnapshot=true | 위 권한 + read_snapshot | 원 snapshot; 현재 화면의 selection으로 재생성하지 않음 |
| includeSigningResult=true | 위 권한 + read_signing_result | 서버가 보유·검증한 정확한 결과. 아직 없으면 null |
| 두 선택 항목 모두 true | 세 조회 조건 모두 | 두 projection을 각각 반환 |

명시적으로 요청한 항목의 권한이 부족하면 전체 요청을 403 또는 존재를 숨기는 404로 거절한다. 권한이 없어 생략한 결과와 아직 없는 결과를 같은 성공 응답으로 뭉개지 않는다. 단, 성공 응답의 result=null도 서명 미생성을 증명하지 않는다. 일반 operation에 승인 projection query=true를 보내면 승인 조회로 전환하지 않고 422로 거절한다.

read_progress 없는 read_signing_result 자격만으로 API-020을 호출할 수는 없다. AC-01에서 이 화면에 필요한 action 조합을 요청해야 하며 서버는 허용 여부를 각각 판단한다. 조회 action은 자동으로 서로를 포함하지 않는다.

API-019의 grant 조회는 새 권한 연결이다. 서버가 grant.target→원 approval/context→해당 dispatch→transactionRef를 확인한 뒤 제한된 체인 진행 정보를 제공한다. 같은 주소의 다른 거래까지 허용하지 않는다. 기존 transaction_read 경로도 유지하며 API-035/038의 업무 결과·고객 allocation 접근을 대체하지 않는다.

## 4. 화면별 승인·실패·복구

| 화면 | 변경할 동작 | 실패/복구 기준 |
|---|---|---|
| U02 지갑 목록·선택·자산 | 계정/매장 context, wallet/binding/revision과 사용 가능한 signer 표시 | 목록 조회 성공을 서명권으로 표시하지 않음; 최신 snapshot에서 다시 검증 |
| U03 송금 검토·승인·결과 | 원 snapshot 검토, HW 또는 Cloud 분기, 필요한 자격 발급, 정확한 제출, 부모/체인 조회 | 만료 시 현재 인증으로 원 context 복구. 미확정 서명이 있으면 새 송금 자동 생성 금지 |
| K03 NU 결제 | 원 attempt/identify→snapshot→NU 승인→sender 결합 grant→제출 | guest 계정 불필요. 거리/연결 유실 시 unknown 가능; 같은 검증 세션 복구만 허용 |
| K05 주문·환불 | 업무 승인 A와 실제 signer B를 구분. 원 allocation/예약/승인 후 revision 표시 | 매장 조회권만 있는 A에게 signed result/submit 권한을 자동 발급하지 않음 |
| D01 상태·서명 승인 | owner 개인/환불, payment_terminal 결제의 검토 내용을 구분 | HTTP token 저장이나 모바일 계정 강제 없음; 키/세대/profile/기한/물리 승인 확인 |
| O03 결제 예외·서비스 상태 | 허용된 운영 대사 상태와 비민감 체인 관측 | 일반 운영자에게 token·signed bytes·보호 객체 다운로드를 주지 않음 |

K05가 B의 서명을 대신 수행하지 않는다. B가 유저 앱에서 원 approvalOperationId/approvalContextId를 받아 현재 매장 지갑 signer 권한으로 U03의 **merchant_refund 변형**을 연다. U03의 기존 personal_transfer 생성 버튼과 분리하여 원 환불 snapshot을 조회하며 API-017로 환불을 새 개인 송금으로 만들지 않는다. B의 앱은 API-015 또는 owner BLE, AC-01/02, API-018/020을 이용한다. A와 B가 같은 사람이어도 업무 승인과 실제 서명권 검사는 분리된다.

그 외 직접 참조 화면 11개(U09/U11/U12/U13/U14/U15/U16/U17/U19/U22/U25)는 기존 기능 회귀 범위다. 녹음·MPC 복구·AI·개인정보 작업은 일반 operation 경로를, smart account/상품/x402는 해당 원 거래/source 계약을 유지한다. U11 고객 환불 조회는 자신의 지급 allocation 범위이며 매장 signer 결과를 노출하지 않는다. [원본 JSON](approval-integration-candidate.json)에 총 17개 화면의 route/BLE 참조·복구 규칙을 기록했다. 기본 화면 파일과 37개 화면 수는 변경하지 않았다.

앱에는 비밀 없는 원 context/부모 operation/발급 요청 멱등키·진행 상태를 복구용으로 남긴다. token과 sender key는 그에 맞는 보호 저장/세션 정책으로 분리한다. analytics·알림·공유 링크에는 token/서명 bytes를 넣지 않는다. 로그아웃 UI는 즉시 사용을 막고 현재 세션 자격 철회를 요청하며, 철회 응답 유실을 성공으로 단정하지 않는다.

## 5. 저장 자원과 원자 처리

다음은 논리 자원이다. 신규 테이블/SQL을 실행한 결과가 아니며 기존 ScopedCapability/ProtectedObject/SS-R01~08 adapter와 병합할 대상이다.

| 자원 | 저장할 핵심 | 제약 |
|---|---|---|
| AI-R01 AccessChallenge | actor/source/context/sender/action/nonce·기한/auth revision/predecessor revision | 일회성 상태, consumedWithIssuanceId로 원 발급에 고정 |
| AI-R02 AccessGrantLineage | grant target/principal/sender/actions/token verifier hash/기한/상태/revision/predecessor/successor | token verifier 유일, predecessor당 successor 최대 하나, revoked/compromised 부활 금지 |
| AI-R03 ProtectedIssuanceResponse | 논리 멱등 범위·입력 digest→grant, 보호 응답 blob 참조·기한 | 원 lineage metadata와 비밀 blob 수명 분리 |
| AI-R04 SenderProofReplay | profile별 namespace/nonce/요청 digest·기한/소비 결과 | 다른 요청에 proof 재사용 금지; 논리 재시도에는 새 proof와 같은 멱등키 |
| AI-R05 AuthorizationGate | source/principal/session의 현재 권한 revision·hold·epoch | SS-R07과 현재 정책 연결; 읽기와 전송 허가를 구분 |

| 원자 단위 | 처리 | 기존 경계와의 연결 |
|---|---|---|
| AI-T01 challenge 생성 | 현재 권한·predecessor 확인 후 고정된 challenge 저장 | SS-R01의 불변 승인 문맥 참조 |
| AI-T02 발급·교체 | 권한 재검사, proof/challenge 소비, predecessor CAS, 새 grant·응답 참조·멱등 결과 commit | 하나의 successor; challenge 검증과 commit 사이 철회 재검사 |
| AI-T03 철회 | grant/lineage 상태와 gate revision 변경 | SS-T04 전송 permit과 공유하는 직렬화 조건 |
| AI-T04 보호 조회/제출 허가 | 현재 grant, sender proof/replay, 정확한 원천 검증 | 조회 release decision 또는 SS-T03 저장 접수·SS-T04 전송 허가 |
| AI-T05 원 응답 재조회·만료 | 같은 actor/sender·현재 권한으로 원 보호 응답 반환, 기한 후 blob 삭제 | 원 멱등 metadata는 유지해 AC-01 issuance_key 교체에 사용 |

### 분산 저장·잠금·외부 전송

비밀 응답 blob을 외부 저장소에 둔다면 blob staging과 DB commit이 원자적이라고 가정하지 않는다. 암호화된 immutable blob을 준비하고 검증된 객체 참조/digest를 DB에 commit한다. 실패한 staging은 접근 불가 orphan으로 정리하며, commit된 참조의 객체가 없으면 새 grant를 만들지 않고 원 grant 상태를 대사한다. DB에 token hash만 남았다는 이유로 토큰 원문을 복원한다고 주장하지 않는다.

잠금 순서는 기존 source/권한 gate→업무 원천/예약→계정 nonce 경계를 보존하고, 해당되는 grant lineage→challenge/replay→멱등 결과/응답 참조를 일관되게 취한다. read-only 경로가 필요 없는 nonce/업무 쓰기 잠금까지 요구하는 뜻은 아니다. 구체 DB 매핑은 후속 검토 대상이나 서로 다른 순서로 동일 자원을 잠그도록 설계하지 않는다.

보호 조회도 **release decision**을 철회와 직렬화한다. 철회가 먼저 이기면 응답을 허가하지 않고, release decision이 먼저 이기면 이미 허가된 응답의 네트워크 도착을 되돌린다고 보장하지 않는다. SS-T04 permit도 같은 원칙이다. 외부 RPC/HTTP가 끝날 때까지 DB 잠금을 잡아 원격 취소가 가능하다고 가정하지 않는다. 통신 불명확 상태는 원 기록으로 대사한다.

proof nonce 소비는 business idempotency를 대체하지 않는다. grant로 보호된 API의 같은 논리 요청을 재시도할 때 새 요청용 sender proof를 만들되 원 idempotency key를 유지한다. 같은 key의 다른 거래 내용/context/sender는 충돌이다. 이미 소비된 proof를 그대로 재생해 mutation을 다시 수행하지 않는다. 권한 거절 때문에 nonce가 소비됐더라도 새 논리 지급을 만들 근거가 되지 않는다.

AC-02 body의 challenge 소유 증거와 보호 API header의 요청별 sender proof는 다른 목적이다. 원 challenge가 이미 소비됐을 때는 독립적인 현재 인증·동일 actor/sender/발급 멱등 범위를 확인한 원 응답 재전달만 가능하고 발급 효과를 반복하지 않는다. 멱등 입력은 검증 후 확정된 논리 내용으로 비교한다. requestId·요청별 nonce/proof bytes가 바뀌었다는 이유로 새 발급/지급을 만들지 않지만, proof 검증을 생략하고 단순히 해당 필드를 지워 비교해서도 안 된다.

### 자격 교체와 폐기

응답 전체 유실 시 AC-01의 issuance_key locator로 현재 인증·동일 주체/source/sender 범위에서 원 grant를 찾는다. challenge에는 predecessor revision을 고정한다. AC-02에서 단일 successor CAS를 수행해 병렬 교체 요청의 형제 grant를 금지한다. 승자가 있으면 허용된 원 결과 대사 또는409이며 새 자격을 자동 발급하지 않는다.

단순 token 만료는 현재 독립 인증 후 교체할 수 있지만 revoked/compromised lineage는 교체로 부활시키지 않는다. 계정 reauth는 동일 accountId와 현재 binding을 재확인하며 새 authSessionId를 기록한다. guest는 terminal/attempt/session/device/epoch가 동일해야 한다. 새 guest 세션 복구·정확한 TTL/보관 기간·crypto profile은 미선정이다. 민감 blob 만료와 lineage/감사 metadata 정리는 별도이며 실제 보존 기간은 운영·개인정보 정책으로 정한다.

## 6. 오류와 재시도

| HTTP | 의미 | 화면/호출 측 처리 |
|---|---|---|
| 400 | envelope/query/version 형식 오류 | 입력/호환 확인. 구버전 자동 fallback 금지 |
| 401 | 인증/자격 만료·검증 불가 | 현재 인증 후 원 context 복구; 기존 서명 상태 유지 |
| 403/404 | 해당 행위 권한 부족 또는 비공개 자원 | 권한 확대 요청이나 다른 ID 추측으로 재시도하지 않음 |
| 409 | selection/revision/멱등/교체 경쟁 충돌 | 원 operation/lineage 대사 후 명시적 다음 행동 |
| 422 | 지원하지 않는 profile/source·증거/내용 불일치 | 실제 지원/승인 문맥 확인; blind signing 금지 |
| 429/503 | 제한/의존 서비스 문제 | 안전한 backoff 뒤 원 멱등키·새 proof로 재시도; 새 지급 생성 금지 |

Error의 retryable/nextAction은 실제 원 상태에 따라 서버가 결정한다. 503/응답 유실이 저장 commit 실패나 온체인 미전송을 증명하지 않는다. 같은 transactionRef/부모 operation을 유지한다. 환불 reservation 역시 실패 알림·조회 지연만으로 해제하지 않는다.

## 7. 검증과 다음 설계

검증기는 13개 경로의 전체 논리 envelope, schema 참조·기준 해시, 17개 실제 화면 ID, 권한 매핑, 저장 참조와 유한 projection 필드 일치 사례를 확인한다. 실제 사용자 권한·암호·MPC·기기·동시성/장애 검토 사례 19개는 전부 미실행이다. 문서 예제의 token/proof/hash는 합성 값이다.

다음은 **기준 명세 반영 순서와 버전·profile 호환 수용표**를 고정하는 설계다. 현재 후보를 바로 기준에 복사하면 이전 후보의 기준 해시·source adapter·전체 기능 회귀가 깨질 수 있으므로 보존할 이력, 교체할 validator 책임, 미지원 조합의 화면 동작까지 연결한다. AC 경로 등록과 구체 저장/권한 파일 반영도 이 절차에 포함한다.

12주 전체 15개 요구·104개 작업·320개 세부 작업은 유지한다. 역할·공수 배정과 구현 착수는 하지 않았다. RR-DEC-01은 사용자 답변 대기이며 이 설계가 키 백업 추가나 복구 없는 초기화 승인을 의미하지 않는다.

# 보안 API 통합 결과

보충 경로 5개를 **API-103~107**로 통합했다. 현재 카탈로그는 **107개 API**, 핵심 DTO 8개 + 확장 DTO 99개, 권한 정책 57개다. 기존 SEC-API-01~05는 이력/별칭이며 별도 실행 경로가 아니다. [카탈로그](api-catalog.md) · [확장 DTO](extended-dtos.md) · [화면](screen-flows.md) · [접근 표](api-access-transactions.md)

전체 범위와 앱 연동·12주 완료 전제는 유지한다. 이번 결과는 설계 통합이며 구현·배포·실기 검증 결과가 아니다. 개발자 담당/공수는 비워 두었다.

## 1. 경로와 화면

| 정식 ID / 별칭 | 경로 | 화면·후속 호출 | 저장 책임 |
|---|---|---|---|
| API-103 / SEC-API-01 | POST /v1/auth/flows | U01·K01 → API-001/002 | AuthFlow |
| API-104 / SEC-API-02 | POST /v1/receipt-claims/challenges | U11·U24 → API-105 | ReceiptEligibility 확인·ReceiptClaim 생성 |
| API-105 / SEC-API-03 | POST /v1/receipt-claims/{claimId}/confirm | U11·U24 → API-106·042·043 | ReceiptOwnership·outbox, challenge 소비 복구 |
| API-106 / SEC-API-04 | GET /v1/receipt-claims/{claimId} | U11·U24의 응답 유실·앱 재시작 복구 | 본인 ReceiptClaim/Ownership 조회 |
| API-107 / SEC-API-05 | GET /v1/protected-results/{resultId} | U09·U13·U19·U22·U25 | ProtectedObject·ObjectPayload·DeletionTombstone |

API path 변수는 request.path, POST 입력은 body, GET 입력은 query에 분리했다. 선언된 JSON envelope는 기존 requestId·변경 요청 idempotencyKey 규칙을 따른다. 실제 Authorization header 해석·token 보관은 보안 adapter 책임이며 DTO에 가짜 인증 필드를 추가하지 않았다.

## 2. 인증 입력 변경

API-001/002는 `provider, authFlowId, providerProof: ProfileInput`을 받는다. 기존 최상위 authorizationCode/redirectUri/pkceVerifier 입력을 함께 허용하지 않는다. API-103의 `flowId`를 후속 요청의 `authFlowId`로 전달한다.

server AuthFlow가 provider/client/OS/redirect/purpose와 profile을 고정한다. providerProof.profileId가 이 선택과 일치해야 하며 적용 profile에서 code/PKCE/state/nonce/SDK 증명을 엄격히 검증한다. Google·Apple에 동일한 SDK 입력을 강제하지 않는다. ProfileInput의 data는 여전히 미선택 표준별 상세 검증 경계다. 형식 검사 통과를 OAuth 인증 성공으로 보지 않는다.

새 flow의 중복 요청 방지는 AuthFlow adapter가 담당한다. 일반 idempotency_records에 authorization parameters/token을 넣어 복구하지 않는다. `link` 목적은 현재 계정의 최근 인증을 요구하며 공개 flow 시작만으로 계정 연결 권한을 주지 않는다. [인증 복구 설계](security-integration-design.md)

## 3. 영수증·혜택·반납 연결

API-104는 `eligibilityProofRef` 대신 **실제 검증 자료를 담는 eligibilityProof: ProfileInput**을 받는다. 별도 proof 업로드 API가 있다고 가정하지 않는다. 결제 당시 기기에 전달된 ticket이나 당시 계정 결합 증명을 현재 앱 계정에 맞는 profile로 제출하며, 서버가 ReceiptEligibility의 검증값·source·기한을 대조한다. locator와 reference ID만으로 구매 내역을 열람할 수 없다. proof 본문은 TLS를 통한 제한된 처리 경로에서만 사용하고 일반 요청 로그/멱등 cache에 저장하지 않는다.

challenge는 purpose=`receipt_link`, accountBinding, eligibilityId, chainId=8283, audience/domain, nonce, expiry와 선택 profile payload를 포함한다. 바깥 필드와 실제 서명 검증 payload의 값이 같아야 한다. API-105는 소유 연결을 한 번만 반영하고 API-106은 상태를 조회한다. JSON schema는 linked 상태의 receiptId 필수와 미연결 상태의 receiptId=null을 검사한다. proof 진위·expiry·동일 계정/귀속 검증은 실제 서비스 책임이다.

API-042는 ReceiptOwnership → ReceiptEligibility → 해당 allocation으로 영수증을 조회한다. 기존 `orders.account_id`를 나중 claim 계정으로 덮어쓰지 않는다. ReceiptView에 `ownershipRevision`, `eligiblePaymentIds`를 추가했다. 공동 주문에서는 payments/refunds와 표시 범위도 본인 eligible allocation으로 제한한다. receiptId만으로 주문 전체 고객 자료에 접근할 수 없다.

**익명 결제 혜택은 누락시키지 않는다.** 현재 benefit_entries는 account_id가 필수이므로 익명 지급을 가짜 계정에 넣지 않는다. 논리 adapter `PendingBenefitEntitlement`에 eligibility+ruleVersion을 고유키로 pending 혜택을 기록한다. claim 이후 검증된 소유 계정에 한 번 결합하고 기존 source key로 benefit_entries를 생성/대조한다. 이것은 기존 혜택의 귀속 확정이며 두 번째 적립이 아니다. 별도 저장소를 쓸 경우 pending→materialized와 업무 원장을 분산 transaction으로 가정하지 않고 source key·immutable boundAccount·outbox/inbox로 복구한다. reorg/환불은 pending과 이미 귀속된 혜택을 모두 보정한다. API-043은 현재 소유 계정의 귀속/처리 중 상태만 표시한다.

API-046의 기존 checklist.checks에 `receipt_link_before_reset`을 포함한다. `passed`는 희망 기록의 실제 claim 완료, `pending`은 연결 필요, `not_applicable`은 해당 기록이 없거나 복구 제한을 이해한 사용자의 기록 포기 증거가 있는 경우다. 단순 ticket 내보내기를 passed로 표시하지 않는다. API-047은 최신 체크 revision과 초기화 증거를 확인한다. 이 결과는 owner projection이며 운영자에게 ticket·개인 영수증 내용을 노출하지 않는다. 신규 익명 여행 지갑은 키 초기화 후 ticket만으로 추가 claim할 수 없고, import 지갑은 원외부 signer 접근과 당시 증명이 있으면 후속 claim할 수 있다.

## 4. 보호 결과 참조 해석

API-043의 `pendingEntitlements`는 본인 소유 연결이 확인된 처리 중 혜택만 반환한다. 상태는 pending/materializing이고 spendable=false다. usable stamps/benefits에는 이를 합산하지 않는다. materialized 이후 같은 원천의 pending 항목을 제거하고 확정된 원장 수치로 표시한다. 해당 전환은 일관된 원장 revision으로 조회하며, 합성 예제는 처리 중/완료 화면과 같은 source의 양쪽 중복 표시 거절 규칙을 검증한다.

| 생산 응답 | 참조 의미와 소비 |
|---|---|
| API-020 operation.resultRef | 결과 종류가 protected object인 경우 API-107의 resultId. 그 밖의 리소스 종류는 operation.kind의 명시적 resolver를 사용 |
| API-053 transcriptRef / summaryRef / jobs[].resultRef | 보호된 원문/요약의 resultId; API-107에서 현재 동의 검사 |
| API-055 transitionPlan.resultRef | immutable 계획 객체의 resultId; planId와 혼동하지 않음 |
| API-081 itinerary.resultRef | 보호된 코스 결과의 resultId; 원래 itineraryId/수정 revision과 구분 |
| API-072 resourceRef | 허용된 유료 자원 profile이 protected object를 사용하는 경우 resultId; entitlement 확인 후 조회 |

TransitionPlan·ItineraryView에 nullable resultRef를 추가했다. 아직 생성되지 않았으면 null이며 내용을 가리키는 유효한 ID를 추측해 만들지 않는다. kind/profile별 resolver를 등록하지 않은 참조는 실행하지 않는다. 내용이 이미 inline으로 제공되는 API-055/081도 같은 owner/source/consent·무결성 검사를 적용하며 API-107만 막고 기존 응답에서 우회 공개하지 않는다.

### Binary delivery

<a id="binary-delivery"></a>

1. API-107을 `Accept: application/json`(또는 생략), query `purpose`로 조회한다. 결과는 inline payload 또는 gateway_stream metadata다. 두 형태는 배타적이며 schema로 검사한다.
2. gateway_stream이면 **같은 API-107 경로**에 `Accept: application/octet-stream`, query `purpose, streamRef`로 요청한다. streamRef는 resultId·object version·purpose·현재 owner/session·만료에 결합한 선택자이며 bearer URL이 아니다. 별도 임의 gateway 주소를 받지 않는다.
3. 서버는 실제 스트림을 열기 전에 현재 권한·동의·tombstone과 해당 버전/digest를 다시 확인한다. 응답은 HTTP 200, Content-Type application/octet-stream, Cache-Control no-store, X-Result-Version과 X-Payload-Digest 및 원시 bytes다. Content-Length는 알려진 경우만 포함한다. JSON response schema의 적용 범위는 JSON metadata이며 bytes를 JSON wrapper로 가장하지 않는다.
4. 이번 스트림 계약은 Range 재개를 제공하지 않는다. 재연결은 현재 권한으로 동일 버전을 다시 요청한다. 열린 스트림은 정의된 짧은 청크 경계에서 차단/동의 철회를 다시 확인하고 이후 전송을 중단한다. 이미 전송된 bytes를 회수할 수 있다고 표시하지 않는다. 체크 간격/최대 청크는 adapter 성능·철회 SLA 검증 후 설정한다.

metadata 조회 성공 뒤 철회된 권한으로 stream을 여는 요청은 거절한다. JSON의 payloadDigest는 `sha256:<64자리 hex>` 작업용 형식이며 producer/gateway/consumer가 같은 immutable bytes를 해시한다. inline payload의 canonical byte serialization은 해당 profile에서 확정해야 한다.

## 5. 저장 통합 경계

[논리 저장 계약 15개](security-storage-contracts.json)는 API 접근 표의 adapterReads/adapterWrites와 연결했다. 기존 참조 SQL 테이블 61개와 별도로 관리하며, 실제 새 테이블 15개를 만들었다는 의미가 아니다. readTables/writeTables는 검증된 기존 참조 테이블 이름만 사용한다.

ReceiptOwnership과 업무 outbox는 선정된 ledger transaction 안에서 함께 반영한다. SQL을 고르면 같은 DB transaction으로 묶고, 다른 ledger를 고르면 그곳의 원자 outbox를 사용한다. challenge 저장소는 commit된 claimId로 소비 결과를 복구한다. 이 선택을 구현 전에 고정하고 crash 지점별 시험을 수행한다.

보호 객체 producer·chain/benefit worker는 검증된 업무 source로만 logical resource를 변경한다. API-105가 pending 혜택의 존재를 확인하더라도 실제 적립은 source를 검증한 worker 책임이다. 읽기 API는 domain/adapter 쓰기를 하지 않으며 감사는 별도 허용된 경로다.

## 6. 검증과 남은 구현 입력

검증기는 107개 API와 107개 접근 매핑, 8+99개 DTO, 57개 정책, alias→정식 ID, 37개 화면, 15개 논리 저장 자원, 작업 ID 연결을 검사한다. 합성 예제는 누락 proof·부적절한 claim 상태·stream 입력 및 응답 혼합을 거절하는지 검사한다. 이것은 실제 공격·암호·권한 동작 시험이 아니다.

별도 보충 API 병합 상태는 완료했다. 남은 것은 각 profile의 엄격한 내부 타입/검증기, 보안·객체·ledger adapter 및 실제 마이그레이션, 라이브러리/배포 단위와 운영 설정, 실기·경쟁·장애 시험이다. 다음 계획 작업은 이 구현 선택을 결정 항목·인터페이스별로 정리하는 것이다.

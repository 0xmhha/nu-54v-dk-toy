# 보안 연결 5개 항목의 실행 설계

2026-09-18. 전체 15개 기능·실제 앱 연동·12주 완료 범위를 유지한다. 이 문서는 기존 GAP-01~05를 구현 가능한 흐름·입출력·저장 책임·실패/복구·인수 조건으로 구체화한 **설계 제안**이다. 제품 구현·프로토콜 선택 확정·보안 인증 결과는 아니다. 개인별 배정과 공수는 정하지 않는다.

[작업별 설계 원본](security-integration-design.json) · [보충 API 5개](security-api-addendum.json) · [수용 시나리오](security-integration-cases.json) · [기존 권한 설계](access-transactions.md)

기존 102개 API에 보충 5개를 API-103~107로 통합하여 현재 **107개 제안 경로**다. [통합 결과](security-api-integration.md)에서 DTO·화면·권한·저장 계약을 확인한다. SEC-API 별칭은 이력용이다. 참조 SQL 61개 테이블과 보안 논리 adapter 자원 15개를 구분하며 후자를 구현된 테이블로 간주하지 않는다.

## 1. 전체 연결과 설계 상태

| 항목 | 앱에서의 결과 | 주 저장 책임 | 설계 이후 남은 선택 |
|---|---|---|---|
| GAP-01 인증 상태 | 소셜 로그인·재로그인·계정 연결과 안전한 세션 종료 | 인증 상태 저장 adapter + auth_sessions | Google/Apple 각 OS SDK 경로, 제공자 proof 형식, 저장 구현 |
| GAP-02 임시 권한 | 폰 없이 주문·기기 승인, 연결 종료 후 결과 확인 | capability adapter + 기존 주문/intent/operation | BLE 인증 profile, TTL·철회 전달 시간과 peer 증명 |
| GAP-03 신뢰 registry | 운영자·issuer·verifier·worker 업무 권한 관리 | 버전 있는 신뢰 registry + 감사 | 최초 관리자 bootstrap·승인 주체·service identity 구현 |
| GAP-04 영수증 연결 | 기기 단독 결제를 나중에 개인 앱에 연결 | claim 증거 저장 + receipt ownership ledger | 익명 claim ticket 전달/보관·분실 처리, signer profile |
| GAP-05 보호 결과 | 전환 계획·견적·처리 결과를 본인 앱에서 재조회 | object metadata + 보호 객체 저장 | 객체 저장 제품·키 관리·보관기간·다운로드 방식 |

`design_specified`는 흐름 명세를 작성했다는 의미다. adapter 선정·런타임 검증은 미완료이며 이 항목들을 `closed`로 바꾸지 않는다. 구현에서 필요한 각 선택은 문서의 대안/제안 상태로 남긴다.

## 2. GAP-01 — 소셜 로그인과 세션 보안 상태

### 흐름과 입출력

1. 앱은 설치된 provider/OS profile을 선택하고 **SEC-API-01**로 로그인 또는 계정 연결 flow를 시작한다. 연결 목적이면 기존 계정의 최근 인증이 필요하다. 서버는 등록 client/redirect 조합을 조회하며 임의 redirect를 허용하지 않는다.
2. 서버는 `flowId`, 공개 가능한 authorization parameters, 만료시각을 반환한다. 앱은 provider profile에 따른 시스템 인증 UI 또는 공식 SDK를 연다. 브라우저 방식은 앱 내부 WebView에 로그인 비밀번호를 받는 방식으로 구현하지 않는다. [RFC 8252](https://www.rfc-editor.org/info/rfc8252/)
3. profile에 필요한 PKCE S256·state·nonce를 흐름에 결합한다. PKCE verifier는 앱의 임시 보호 영역에서 유지하고 일반 로그/분석 SDK에 전달하지 않는다. 제공자별 실제 적용 가능성은 profile에서 검증한다. Google과 Apple이 동일한 PKCE/SDK 입력을 쓴다고 단정하지 않는다.
4. API-001 또는 API-002에서 flow 소유·purpose·provider/client·redirect·challenge·기한을 확인한 뒤 provider proof를 검증한다. 검증되지 않은 email은 기존 계정 합병 근거가 아니며, 계정 식별은 검증된 issuer+subject 기준이다.
5. flow를 원자적으로 예약하고 검증 결과와 account/session 연결을 한 번만 반영한다. 외부 provider 응답을 기다리는 동안 업무 DB 잠금을 잡지 않는다. 같은 flow·요청 digest로 재시도하면 생성한 세션의 중복 생성을 막고, 다른 요청으로 재사용하면 거절한다.

인증 code 교환 결과가 유실되어 소비 여부를 모르면 `AUTH_RESTART_REQUIRED`로 새 flow를 시작한다. 검증되지 않은 성공을 복원하지 않는다. 이미 서버에 완료 결과가 안전하게 저장되어 있다면 현재 flow 소유 증명을 확인한 후 동일 결과를 복구할 수 있다.

### 저장과 회전

| 논리 레코드 | 핵심 필드 | 유일성·원자 변경 |
|---|---|---|
| AuthFlow | flowId, purpose, providerProfile, client/redirect ID, initiating account/session(연결 시), challenge/nonce/state 검증값, issued/expiresAt, status, requestDigest, resultSessionId | flowId당 한 번의 검증 소비; 다른 계정으로 목적 변경 금지 |
| RefreshFamily | familyId, account/session, client binding, generation, current token 검증값, revoke revision | generation 비교 후 한 번만 회전 |
| RotationOutcome | family/generation, requestDigest, sender binding(선택 profile), 새 token의 보호된 단기 결과 참조, 만료 | 검증된 동일 재시도만 결과 복구; 일반 멱등 cache에 token 원문 금지 |

refresh token 회전 또는 sender-constrained 방식으로 재사용에 대응해야 한다. 이는 표준의 방향이며 구체 회전 저장 구조는 이 제품의 설계 제안이다. [RFC 9700](https://www.rfc-editor.org/rfc/rfc9700.html)

앱은 refresh를 한 번에 하나만 실행하고, 새 token을 보호 저장소에 기록한 후 후속 요청을 진행한다. **응답 유실 뒤 이전 token을 다시 보낸 요청**은 정상 재시도와 탈취를 token·멱등 key만으로 구분할 수 없다. 제안 기본값은 구분 불가 시 family 철회와 재로그인이다. 재시도 복구를 제공하려면 검증된 sender binding과 짧은 보호 결과 수명을 함께 구현·시험한다. 이전 token만으로 최신 token을 돌려주는 grace window는 두지 않는다.

로그아웃/계정 잠금/탈퇴는 family와 해당 세션 revision을 철회한다. access token 캐시가 남아 있어도 중요 변경 API는 최신 철회 상태를 확인한다. 저장소 장애 시 새 세션 발급·회전을 거절하며, 앱은 재로그인/재시도를 안내한다. 인증 상태 복구본은 철회 기록을 되살리지 않게 대조한 뒤 사용한다.

**완료 증거:** Google·Apple 각각 실제 대상 OS에서 로그인/연결/취소/복귀/앱 재시작 성공, callback 재사용 거절, refresh 동시 요청·응답 유실·logout 후 재사용 시험. 로그·crash report·일반 멱등 저장소에 code/verifier/token 원문이 없음을 확인한다.

## 3. GAP-02 — 주문부터 서명 제출까지 임시 권한

### 권한을 발급하고 소멸시키는 흐름

| 단계 | 발급 근거 | 결합할 대상 | 완료/철회 이후 |
|---|---|---|---|
| 단말 세션 | 현재 store 관리 권한 + 등록 terminal 증명 | store, terminal, mode, membership revision | 관리 로그아웃과 고객 단말 운영 수명은 별도 정책. 관리 권한이 고객 모드로 유출되지 않음 |
| 주문 권한 | 유효 고객 단말 세션 + 주문 생성 | store, terminal, order, allowed actions | 취소/만료 시 신규 결제 시도 불가 |
| 기기 결제 권한 | attempt + 검증된 장치/session challenge | order, attempt, terminal, device, 당시 rental/binding revision | 연결 종료/거리 정책 실패/기한 만료 시 신규 intent·승인 요청 종료 |
| 제출 권한 | 승인 대상 intent와 signer proof | intent/source, chain, payer, payload digest, audience | 한 번의 동일 제출만; 다른 payload는 거절 |
| 결과 조회 권한 | 원래 요청 주체 또는 별도 조회 credential | order/attempt/operation, 최소 응답 범위 | 쓰기 권한을 재활성화하지 않고 결과 추적 |

권한 token은 추측하기 어려운 opaque credential + 서버 검증 레코드를 작업용 기본안으로 둔다. 고엔트로피 token의 검증값, parent ID, scope/resource, issued/expiresAt, revision, revokedAt, 소비 상태를 저장한다. token 원문은 일반 DB/URL query/로그에 남기지 않는다. 다른 서명형 token을 선택하더라도 최신 철회·사용량 확인을 생략하지 않는다.

새 행위를 허용하는 하위 쓰기 권한 만료는 부모 권한·quote·intent 중 가장 이른 적용 만료를 넘지 않는다. 결과 조회 권한은 별도 읽기 증명과 수명으로 발급하며 quote/intent 만료에 종속시키지 않는다. 읽기 권한이 만료되면 현재 계정·원래 단말/주문 조회 증명을 재검증해 재발급하고, 종료된 쓰기 token만으로 재발급하지 않는다. 부모 철회 사유가 침해·분실이면 기존 읽기 권한도 재검증/철회한다. 정확한 TTL은 `authFlowTtl`, `terminalIdleTtl`, `attemptTtl`, `submitTtl`, `resultReadTtl`이라는 정책 설정으로 분리한다. 값과 허용 철회 전달 지연은 실기/네트워크 시험에서 정하고 출시 설정에 필수로 기록한다.

### 원자 처리와 장애

제출은 `intentId + payloadDigest`를 기준으로 예약 → 검증 → durable submission 기록 → 외부 전송으로 진행한다. secure store와 DB가 분리되면 두 저장소가 하나의 transaction인 것처럼 가정하지 않는다. 예약 레코드에 operationId와 상태를 남기고, DB의 고유 제출 기록을 조회해 복구한다. 장애 시 권한을 다시 미사용으로 돌려 다른 payload를 허용하지 않는다.

chain broadcast 응답이 유실되면 같은 signed payload의 hash로 조회/허용된 동일 전송만 재시도한다. 새 nonce/새 결제로 자동 대체하지 않는다. 이후 앱/기기가 연결되지 않아도 서비스는 이미 기록한 거래를 추적한다.

**철회는 새 권한 행사와 이미 전송된 거래 관측을 구분한다.** 서버는 철회 후 새 요청·미전송 작업을 정책에 따라 막지만, 이미 방송된 거래의 체인 실행 자체를 취소할 수 없다. raw signed transaction이 이미 외부에 전달됐다면 다른 경로에서 전송될 수도 있다. 업무 API의 철회와 온체인 취소를 같은 것으로 표시하지 않는다. 관측 worker는 검증된 receipt/canonicality/원지급 귀속을 계속 반영하며, 사용자 조회는 현재 조회 권한을 따로 검사한다.

BLE 근접 결과는 인증된 session·freshness에 결합한 보조 조건이다. 단순 RSSI나 거리만으로 payer·단말을 인증하지 않는다. 장치 측 거절/세션 키 폐기와 서버 측 권한 철회를 함께 시험한다.

**완료 증거:** 같은 승인 2회 제출 시 제출 기록 1개, 다른 attempt 전용 거절, 단말 교체/재대여/연결 종료의 신규 승인 차단, 전송 응답 유실 후 중복 결제 없음, 철회 후 기존 거래 결과의 정상 반영 및 제한 조회.

## 4. GAP-03 — 신뢰 registry와 운영 권한 변경

### 관리 대상과 행위

| 주체 종류 | 신뢰 등록 항목 | 허용 업무의 예 |
|---|---|---|
| operator | identity issuer+subject, 업무 scope, 승인 revision, 유효기간 | 예외 조회·재대사 작업 요청·릴리스 관리 중 부여된 범위 |
| issuer | issuer ID, profile/chain, 검증키 버전, credential 종류, 허용 목적 | 자신의 credential 발급/철회 |
| verifier | verifier ID, profile·목적·요청 필드 범위 | 허용 presentation의 최소 결과 검증 |
| service | 배포 workload identity, 환경, 역할, resource scope | 명시된 worker job 실행과 원장 반영 |

사람 주체와 서비스 주체를 같은 bearer 관리자 계정으로 묶지 않는다. `memberships`의 owner 값을 운영자 registry로 복사하지 않는다. issuer 등록은 protocol_profiles에 상세 검증기가 있다는 것과도 구분한다.

변경 흐름은 **요청(draft) → 검증/승인(reviewed) → revision 비교 활성화(active) → 중지(suspended)/철회(revoked)**다. 변경에는 대상·권한 diff·이유·요청자·승인자·시각·expectedRevision·감사 ID를 남긴다. 권한 상승 요청자는 자기 요청의 승인자가 될 수 없도록 제안한다. 최소 승인 수와 실제 승인 주체는 운영 정책 선택이며 개발자 역할 배정으로 처리하지 않는다.

최초 관리자는 보호된 배포 설정에서 신뢰 anchor를 등록하고 첫 활성화 증거를 남긴다. 공개 회원가입이나 임의 DB row 삽입을 bootstrap으로 삼지 않는다. 구체 배포/승인 도구는 미선정이다. 비상 중지는 이미 그 권한을 받은 주체가 수행할 수 있게 하되 재활성화는 별도 검토한다.

매 요청/민감 worker 실행 직전 registry revision을 확인한다. 캐시의 최대 노후 시간과 철회 SLA를 설정하며, 조회 불가 또는 지나치게 오래된 snapshot은 새 민감 행위를 거절한다. issuer 키 교체는 key version·유효 구간·과거 검증 자료를 유지한다. 오래된 credential의 표시 정책과 최신 신뢰 판정은 분리하고, 철회된 issuer를 과거 캐시로 현재 유효하다고 판정하지 않는다.

**저장 계약:** TrustPrincipal, TrustGrantRevision, TrustChangeRequest, AuditReference. actor/subject ID·환경/범위·policy revision을 검증하며 DB role/GRANT는 선정된 배포 단위에 맞춰 별도 작성한다. 신규 공개 registry 관리 API를 자동 추가하지 않고 우선 보호된 운영 service command 계약으로 둔다.

**완료 증거:** 자기 권한 상승 거절, 타 issuer 자격 취소 거절, 점주를 운영자로 취급하지 않음, 철회된 worker의 새 job 실행 거절, 키 교체 전후 허용 credential 검증, 감사 누락 시 권한 변경 미반영.

## 5. GAP-04 — 기기 단독 결제의 개인 영수증 연결

### 결제 당시 증거를 보관한다

`ReceiptEligibility`는 order/payment allocation과 payer address, chain, signer model, 결제 당시 rental/binding revision, verified account(알려진 경우), claim ticket 검증값(익명인 경우), evidence revision을 연결한다. **현재 주소 소유 또는 현재 기기 소유만으로 과거 구매자의 신원을 인정하지 않는다.** 한 기기를 재대여하거나 같은 지갑 키의 통제권이 바뀔 수 있기 때문이다.

이미 계정에 연결된 기기는 결제 당시의 유효 binding과 승인 증거로 해당 계정의 eligibility를 기록할 수 있다. 폰 로그인을 매 결제의 필수로 추가하지 않는다. 계정이 없는 익명 경로는 당시 결제 session에 묶인 claim credential을 본인에게 전달하는 절차가 필요하다. 제안은 기기에 보호된 소량 metadata로 보관하고 앱 연결 시 암호화된 경로로 이전하는 방식이다. 키오스크 화면의 주문번호/공개 QR만으로 claim 권한을 얻지 못하게 한다.

익명 claim credential의 저장 용량·손실/재발급·앱 이전 증명은 HW/APP 작업에서 검증한다. 반납 전 UI는 **자산 회수와 별도로 영수증/발도장 연결 상태**를 보여준다. 반납 후 원대여자는 이미 계정에 연결된 기록을 계속 조회한다. 익명 ticket을 내보내지 않고 초기화해 증거가 사라진 경우, 다음 대여자의 기기 서명을 대체 증거로 쓰지 않는다. 본인에게 결합된 과거 계정/대여 증거가 없으면 자동 연결을 거절하고 지원 절차로 넘긴다. 지원 담당자도 주소만으로 개인정보를 공개하지 않는다.

신규 여행 지갑의 익명 경로는 **키 초기화 전에 희망 영수증의 계정 claim을 완료**하는 것을 기본안으로 둔다. ticket만 내보내고 payer 키를 삭제하면 뒤에 필요한 signer 증명을 만들 수 없다. 반납 UI에서 ticket 내보내기와 claim 완료를 구분하고, 미연결 기록이 있는 초기화는 복구 제한을 명확히 표시한다. 기존 지갑 import 경로는 외부 지갑에 같은 signer 접근이 남고 당시 ticket을 보관했다면 반납 후에도 선택 profile로 claim할 수 있다. 미래 계정/후속 signer로의 위임 증명은 현재 기본안에 암묵적으로 포함하지 않는다.

### 앱 연결 API

1. **SEC-API-02**: 로그인 계정이 `receiptLocator`와 결제 당시 eligibilityProof 검증 자료로 claim challenge 요청. locator는 비밀이 아니며 요청 단계부터 증명을 확인해 주문 존재/타인 구매 내용의 열거를 막는다.
2. 서버가 claimId, one-time challenge, account binding, order/evidence reference, audience/domain, chain, purpose=`receipt_link`, expiry를 구성한다. 화면에는 결제·자산 승인과 다른 영수증 연결임을 표시한다.
3. 당시 account binding 경로는 현재 해당 계정의 최근 인증을 검사한다. 익명 ticket 경로는 ticket 증명과 payer signer의 목적 제한 서명으로 확인한다. 지원 signer의 표준 메시지 검증 profile은 별도로 선택한다. SIWE를 쓸 경우 domain/URI/chain/nonce 등 표준 검사를 지키되, SIWE 서명만으로 구매 당시 소유를 대체하지 않는다. [ERC-4361](https://eips.ethereum.org/EIPS/eip-4361)
4. **SEC-API-03**: challenge/source/account 일치와 proof를 검증하고 claim ledger의 대상 고유키를 잠근다. 같은 계정의 재요청은 기존 결과, 다른 계정의 경쟁 요청은 충돌. 먼저 요청했다는 사실은 권한 근거가 아니다. 계정 병합/소유 이전은 별도 증명·감사 없이 처리하지 않는다.
5. 서버는 ownership link와 outbox를 같은 업무 transaction에 반영한다. 외부 challenge 저장소 소비는 claimId를 통해 복구한다. 응답 유실은 **SEC-API-04**로 본인 claim 결과를 조회한다. API-042는 검증된 연결 내역만 반환한다.

payer가 merchant/공용 지갑이면 매장 wallet_read 권한만으로 개인 여행 영수증을 가져오지 않는다. 다중 지급 주문은 eligibility의 해당 payment allocation 범위를 조회하고 다른 지급자의 개인정보를 합쳐 공개하지 않는다. 1인 EOA 결제 이후 스마트 계정도 동일한 당시 증거 연결을 유지한다. 스마트 계정 검증은 지정 체인의 해당 계정 검증 profile을 적용하며 단순 EOA 주소 복구로 대체하지 않는다.

claim 연결은 결제 수락·스탬프 적립을 새로 발생시키지 않는다. 기존 지급 identity로 중복을 방지한다. reorg/환불 이후에도 소유 관계의 감사 기록은 유지하고 영수증 지급 상태·혜택·여행 증거를 별도 보정한다. 소유 연결이 있다는 이유로 취소된 지급을 유효한 챌린지 구매로 표시하지 않는다.

**완료 증거:** 폰 없이 실제 기기 결제→나중 앱 로그인→본인 영수증 조회, 재대여자의 과거 기록 접근 거절, 다른 계정/nonce/challenge replay 거절, 동시 claim 1개 연결, 반납 후 원계정 조회, 환불/reorg 후 중복 스탬프 없음.

## 6. GAP-05 — 보호된 결과 참조와 객체 저장

### 객체의 생성·조회·삭제

`ProtectedObject`는 resultId, kind, owner scope(account/store/capability), source kind/id/revision, schema/profile version, payload digest, object version, encryption key reference, consent revision(해당 시), expiry, status를 가진다. key reference는 키 자체가 아니다. metadata와 실제 bytes의 저장소를 구분한다.

작업용 흐름은 **staging → 검증 → available → blocked → deleting → deleted**다. 실패한 업로드는 `failed`로 기록하고 소비하지 않는다. producer는 할당된 위치에만 쓰고, content/schema/digest 검사 후 DB source 참조와 공개 상태를 반영한다. metadata가 먼저 생성됐어도 실제 객체가 없거나 digest가 다르면 사용할 수 없다. staging 고아 객체는 참조 확인 뒤 청소한다.

- quote와 transition plan은 source revision·만료를 가진 immutable 버전이다. 참조 ID의 bytes를 바꿔도 같은 intent 서명 대상이 되게 하지 않는다. intent 생성 시 원본 payload digest와 review digest를 다시 결합한다.
- processing 결과는 원자료·동의 revision과 lineage를 유지한다. worker 완료 시 철회되었다면 결과를 공개하지 않는다.
- **SEC-API-05**는 현재 principal/source 권한·만료·동의·무결성을 확인하고 내용 또는 보호된 스트림 참조를 반환한다. resultId나 operations.result_ref 자체는 bearer 권한이 아니다.
- 제안 기본값은 인증된 API를 통한 작은 결과 반환/stream이다. 대용량 signed URL을 선택하면 URL이 유효한 동안 완전한 즉시 철회가 어렵다는 특성을 반영해 짧은 수명·범위 제한을 두고, 즉시 철회가 필수인 내용은 gateway를 사용한다.
- 삭제 요청은 먼저 접근을 막고 원본·파생본·캐시·보호된 재시도 결과를 처리한다. 백업의 보관·삭제 수명은 별도 정책에 기록한다. 복원 시 최신 차단/철회 tombstone을 적용하기 전 재공개하지 않는다.

서버 보호 객체 영역은 HW private key·mnemonic·MPC share의 저장소가 아니다. OAuth 회전 복구 token 같은 제한된 인증 비밀은 GAP-01의 별도 인증 저장 adapter가 관리한다. 사용자 키 import의 폰→기기 암호화 경로 역시 이 일반 결과 API에 넣지 않는다.

미완료 금융 거래의 불변 승인 증거와 삭제 대상 원음/AI 내용은 서로 다른 보관 정책으로 분리한다. 개인정보 삭제를 근거로 진행 중 지급의 귀속·감사 자료를 무조건 지우거나, 금융 감사 필요를 근거로 원음·정밀 위치를 일괄 보존하지 않는다. 필요한 최소 증거와 보관 근거·수명은 각각 확정한다.

**저장 계약:** ProtectedObject metadata, payload bytes, source/consent lineage, deletion tombstone. 실제 암호 라이브러리·KMS/OS 저장소·보관 수명은 선정 결과를 기록하고, 암호화되어 있다는 이유로 소유 검사를 생략하지 않는다.

**완료 증거:** 타인 resultId 거절, 동일 ID bytes 변경/누락 거절, consent 철회와 worker 완료의 경쟁 시 미공개, staging 장애 복구, 삭제 후 백업 복원에서도 재공개 없음, 만료된 계획으로 서명 생성 거절.

## 7. 통합 순서와 완료 판정

1. BASE/AUTH에서 보안 저장 adapter 인터페이스·일회 소비·revision·감사·실패 코드를 확정한다. 실제 저장소를 정하고 기존 DB와의 복구 protocol을 검증한다.
2. GAP-01 로그인과 GAP-03 사람/서비스 권한을 연결한다. Google/Apple provider별 앱 계약 차이를 기존 API-001/002 DTO에 반영한다.
3. GAP-02의 기기·단말·주문 권한과 제출/결과 조회 수명을 통합한다. 기존 단독 기기 결제와 고객 가스 전제는 유지한다.
4. GAP-04의 당시 eligibility와 영수증 연결을 APP/PAY/STAMP/TRIP 화면에서 연결한다. 반납 UI의 개인정보/영수증 이전 상태도 검증한다.
5. GAP-05를 스마트 계정 계획·녹음 결과·여행 추천에 적용하고 공통 장애/삭제 시험을 실행한다.

이 순서는 통합 선행 조건이며 인력·주차 배정이 아니다. 설계 원본의 acceptance ID는 정상·실패·경쟁·복구 시나리오에 연결한다. 각 완료 증거에는 빌드/환경/profile 버전, 입력·판정, 민감정보를 제거한 trace, 앱 화면 결과, 재현 방법을 남긴다.

현재 검증 도구는 ID·작업·기존/보충 API 참조·상태/전이·시나리오 연결의 일관성만 검사한다. 암호 검증·OAuth 제공자 실행·BLE·동시성·실제 권한 검사는 **미실행**이다. 보충 5개 API의 카탈로그/DTO/화면/저장 명세 병합은 완료했다. 위 adapter/profile의 선택과 실제 구현 검증을 남긴다.

# 공통 승인 스키마와 권한 자격 발급·전달 계약

2026-09-18 · **설계 후보. 제품 구현·기준 카탈로그 병합·암호 및 실기 검증은 미수행.**

개인 송금, 계정 없는 NU 키오스크 결제, 가맹점 환불에 공통 승인 타입을 적용한다. 발급된 접근 자격은 원 승인 문맥의 제한된 조회·제출에만 사용한다. 자격 발급이 실제 사용자 서명이나 기기의 물리 승인을 대신하지 않는다.

[스키마](approval-contracts.schema.json) · [계약 원본](approval-access-candidate.json) · [합성 예제](approval-contract-examples.json) · [선행 병합 계획](approval-contract-merge-plan.md)

후속 [전체 HTTP·권한·화면·저장 연결 후보](approval-integration-contracts.md)에서 기존 API 10개와 AC 3개의 envelope와 API-020 projection query를 구체화했다. 기준 카탈로그 병합·실행 검증은 여전히 미수행이다.

## 1. 이번 설계의 범위

- `approval-v1-draft`: 공통 문맥, 세 원천의 snapshot, 결과 증거, 공통 operation projection과 접근 자격 타입.
- `approval-ble-v1-draft`: 개인/환불 owner prepare와 guest payment prepare, 기기 결과의 논리 envelope. 실제 GATT 바이트 규격은 아님.
- AC-01~03: 접근 challenge 생성, 자격 발급/교체, 철회에 필요한 **신규 제안 HTTP 경로**. API 번호는 아직 부여하지 않는다.

기존 API 107개·BLE 명령 34개의 기준 파일은 그대로 둔다. 전체 10개 API의 통합 HTTP envelope와 오류 projection, 권한/화면/저장 병합은 후속 작업이다. 새로운 제품 기능이나 작업 패키지를 추가한 것이 아니라, 기존 승인 경로에 필요한 호출 계약을 상세화했다.

암호·송신자 proof profile과 신뢰 bootstrap은 아직 선정하지 않았다. 아래 profileId와 evidenceBytes는 증거의 운반 타입이며, 해당 verifier가 없으면 실행할 수 없다. 보안 구현이 완료됐다는 의미가 아니다.

## 2. 공통 문맥과 원천별 차이

모든 snapshot은 sourceKind, typed intent, context, server reviewProof를 포함한다. context에는 승인/intent/부모 operation ID, chain/payer, payload/review/context digest, 기한, requestKind, profile과 세 버전(HTTP/BLE/evidence)을 고정한다. 세 버전 문자열은 독립적으로 지원 여부를 검사한다.

| 원천 | 추가 문맥 | 실제 서명 경로 |
|---|---|---|
| personal_transfer | sourceId, 서버에서 검증한 accountBindingId, VerifiedSigner | owner BLE 기기 또는 본인 Cloud MPC |
| payment | store/order/attempt/revision, terminal/session/device/epoch, identify 증거, 근접 정책 | payment_terminal BLE NU. 앱 계정 불필요 |
| merchant_refund | store/refund/원 payment allocation/예약, 승인 후 refund/funding revision, 업무 승인 ID, VerifiedSigner | 매장에 지정된 실제 HW 또는 Cloud signer |

`VerifiedSigner`는 서버가 확인한 walletId/walletBindingId, **현재 검증한** walletRevision/bindingRevision, signerRef, signerKind, 종류별 epoch를 갖는다. HW는 deviceEpoch, Cloud는 participantEpoch만 허용한다. 요청의 expectedRevision을 이름만 바꿔 응답하지 않는다. API-037 입력에는 기존 두 예상 revision과 MerchantSignerSelection을 가진 별도 strict body를 사용한다. 두 strict schema를 단순 allOf로 합치지 않는다.

개인 typed intent도 금액·자산·수취인·실제 transaction을 표시한다. 임의 ProfileInput의 data를 그대로 기기에 넘기는 blind signing 경로는 아니다. native/erc20의 거래 의미와 feeParameters의 허용 형태는 선택 encoding profile이 검증해야 한다. JSON Schema 통과만으로 calldata·금액·가스·서명 대상 일치가 확인되지는 않는다.

contextDigest는 자기 자신과 외부 proof bytes를 제외한 문맥을 profile에 따라 정규화한 digest다. 세 버전과 검증한 선택/epoch를 포함한다. reviewDigest와 payloadDigest, signedPayloadDigest는 서로 다른 입력/domain이다. 이번 합성 예제의 반복 hash는 실제 계산값이 아니다.

### 개인 BLE와 Cloud 결과 연결

`wallet.sign.prepare`는 PersonalSnapshot 또는 RefundSnapshot만, `payment.prepare`는 PaymentSnapshot만 받는다. 세션 role은 메시지 문자열이 아니라 실제 인증된 연결 상태와 일치해야 한다. 기기는 reviewProof, 표시할 거래, 선택한 로컬 키/epoch와 원 context를 검증한 뒤 물리 승인을 받는다. owner role만 맞는다고 모든 원천을 서명하지 않는다.

API-015의 공통 body는 intentId/approvalContextId/approvalProof다. 개인 Cloud와 매장 Cloud 모두 **현재 실제 signer의 별도 서명 권한과 사용자 승인**을 검사한다. guest NU 경로에는 적용하지 않는다. 생성되는 MPC 자식 operation은 부모 approval operation·context·signer·participantEpoch에 고정한다.

API-020은 원 부모 operation ID로 접근한다. 서버는 검증 완료된 해당 MPC 자식의 결과를 보관하고, 현재 read_signing_result 권한이 있을 때 같은 부모 응답으로 전달한다. 임의 자식 ID나 operation.resultRef를 공개 다운로드 주소로 사용하지 않는다. HW 결과도 API-018 등에서 서버가 실제 수신·검증·보관한 경우에만 조회할 수 있다.

## 3. 네 가지 접근 권한과 실제 서명권

| action | 허용 범위 | 허용하지 않는 것 |
|---|---|---|
| read_progress | 원 operation의 제한된 진행 상태 | 원 snapshot·서명 bytes의 자동 공개 |
| read_snapshot | 원 검토 snapshot | 새로운 거래 검토/서명 생성 |
| read_signing_result | 서버가 보유·검증한 정확한 원 SigningResult | 다른 source/다른 signer 결과 조회 |
| submit_exact | 원 context와 payloadDigest에 맞는 증거·signed bytes 제출 | recipient/amount/nonce 수정, 새 intent 생성 |

실제 서명 시작은 이 네 action에 포함하지 않는다. API-015의 wallet_sign_authorized 및 기기의 로컬 승인 조건을 별도로 유지한다. `submit_exact`도 provenance/source hold/현재 signer epoch/SS-T03·04 gate를 생략하지 못한다. 최초 수락 이후에는 승인된 하나의 signedPayloadDigest로 더 좁혀 후속 재시도를 검사한다.

read_signing_result로 전달한 EOA signed bytes는 외부 제출에 사용될 수 있다. 따라서 결과 읽기 권한은 단순 상태 조회보다 강한 권한으로 취급한다. 이후 자격 철회로 이미 전달한 서명 자체가 취소되지는 않는다.

| 주체 | 발급의 선행 조건 | 발급 상한 |
|---|---|---|
| 개인 앱 | 현재 계정 인증·원 계정 binding·선택 signer 접근 조건 | 해당 개인 source에 현재 허용된 action만 |
| guest 키오스크 | 인증된 terminal, 원 attempt/session, API-034에서 검증한 identify, NU 세대/연결 결합 | 해당 결제의 중계/조회/정확한 제출만. owner 권한 없음 |
| 환불 업무 승인자 A | 매장 업무 권한과 해당 refund | 기본 진행/검토 조회. B의 서명 결과/제출권 자동 부여 없음 |
| 실제 매장 signer B | 현재 해당 매장 지갑 서명권·지정 signer·정확한 refund | 별도 확인된 결과 조회/정확한 제출. 타 매장 환불 접근 없음 |

기타 중계자에게 서명 결과를 재위임하는 경로는 이번 후보에 없다. 매출 조회권, 소셜 로그인 성공, operationId 소지만으로 자격을 발급하지 않는다.

## 4. 발급 API 후보와 보호 전달

세 경로는 일반 API catalog에 아직 등록되지 않은 제안이다. JSON 파일에는 method/path, strict request/response/error schema를 기록했다. HTTP header 이름과 wire proof 인코딩은 통합 단계에서 기존 envelope에 매핑한다.

| ID | POST 경로 | 입력/응답 핵심 |
|---|---|---|
| AC-01 | /v1/approval-contexts/{approvalContextId}/access-challenges | requestedActions, senderBinding, replacement(null 또는 grant_id/issuance_key) → 서버 challenge/대상/발급 가능한 actions/기한/권한 revision/replacesGrantId |
| AC-02 | /v1/approval-contexts/{approvalContextId}/access-grants | challengeId, senderProof → 실제 accessToken과 서버가 확정한 grant |
| AC-03 | /v1/approval-contexts/{approvalContextId}/access-grants/{grantId}/revoke | reason → 철회 상태/시각 |

공통 요청은 requestId, Authorization, 명시적 contractVersion, Idempotency-Key를 갖는다. **AC-01/02의 Authorization은 현재 계정/terminal 인증 자격**이다. 만료된 approval accessToken만으로 자격을 재발급하지 않는다. AC-03도 동일 현재 주체 또는 정책상 해당 자격을 철회할 수 있는 주체를 인증한다. 철회 권한은 결과 조회권을 부여하지 않는다.

1. API-017/034/037이 원 snapshot을 commit하고 approvalContextId/부모 operationId를 해당 현재 권한의 보호 응답으로 돌려준다. ID는 참조일 뿐 권한이 아니다. 승인자 A가 B에게 ID를 전달해도 B의 독립 인증은 필수다.
2. AC-01이 저장된 원 source를 읽고 현재 인증된 주체에게 허용되는 action의 교집합을 산출한다. 요청 action 중 허용되지 않는 것이 있으면 전체 요청을 거절한다. challenge에는 audience/issuer/domain, 대상 digest, actor/session, sender binding, action set, 일회성 nonce, 기한과 권한 revision을 서버 측에서 결합한다.
3. 앱/키오스크가 sender key 소유 증거를 만든다. NU의 자산 키·니모닉은 전달하지 않는다. 계정 앱 또는 인증된 키오스크가 이 sender key를 보관한다. guest NU에는 HTTP 호출이나 Cloud 참여를 요구하지 않는다.
4. AC-02가 challenge와 senderProof를 검증하고 **발급 시점의 현재 권한·세션·세대·해당 action의 source gate를 재검사**한다. 지급 hold가 있다는 이유만으로 허용된 진행 조회까지 막지 않으며, 제출 권한에는 hold를 적용한다. challenge 소비, grant 저장, 멱등 결과 기록을 원자적으로 처리한다. challenge 단계의 허가를 그대로 신뢰하지 않는다.
5. 응답은 grant 메타데이터와 실제 사용할 수 있는 `accessToken`을 TLS 보호 응답으로 전달한다. 토큰은 server-resolved opaque credential이며 로컬 capabilityRef만 반환하지 않는다. `Cache-Control: no-store`; 로그·분석·URL·알림·일반 outbox에는 토큰/서명 bytes를 넣지 않는다.

grant에는 issuer, 고정 audience=approval_api, target(context/intent/parent operation/source/digest), actions, 서버 도출 principal, senderBindingDigest, authorizationRevision, 발급/만료 시각, replacesGrantId가 있다. 이 응답 JSON을 클라이언트가 수정해 권한으로 사용할 수 없다. 서버는 토큰에 대응하는 자체 grant 기록을 기준으로 검사한다.

**sender-bound는 설계 요구이며 특정 표준 구현을 선정했다는 뜻이 아니다.** 프로파일은 기존 검증된 기술 중에서 후속 선정한다. 매 요청 proof가 최소한 method·정규화된 endpoint/중복 query 거절·정확한 body digest·token digest·request nonce/기한·audience를 결합하고 재전송 방지를 제공해야 한다. 단순 서명 문자열이나 TLS 사용만으로 이 조건을 충족했다고 보지 않는다. 프로파일·trust bootstrap 미선정 상태에서는 실제 자격 발급을 활성화하지 않는다.

## 5. 만료·재발급·응답 유실

| 사건 | 처리 |
|---|---|
| 발급 응답 유실 | 동일 actor/sender/멱등키·논리 입력이면 원 grant와 원 보호 응답을 재전달. 재시도 때도 현재 접근권/철회 검사 |
| 원 토큰 응답의 보호 캐시 만료 | 원문 토큰을 token hash에서 복원한다고 가정하지 않음. AC-01의 replacement.issuanceIdempotencyKey로 원 발급 결과를 찾아 명시적으로 교체 |
| 기한 전/후 교체 | AC-01 replacement의 grantId 또는 원 issuanceIdempotencyKey로 predecessor를 지정. 같은 target·주체·sender와 action 부분집합만. AC-02에서 새 grant 활성화와 구 grant 교체 처리를 원자 수행 |
| sender 변경/앱 재설치 | 단순 교체로 처리하지 않음. 계정 기반 현재 권한을 다시 입증하는 신규 발급; 이전 세션 자격 철회. guest 세션 교체는 아래 제한 유지 |
| 승인 snapshot 기한 만료 | 새 서명/전송 권한 자동 연장 금지. 현재 인증으로 원 결과의 read-only 자격을 별도 발급할 수 있으나 원 context 기한과 기록을 바꾸지 않음 |
| grant 철회/로그아웃/세대 변경 | 새 보호 API 사용 차단. 실제 permit/서명 노출 상태는 유지하고 대사 계속 |
| 서버에 결과가 없음 | signingResult=null 및 signature_unknown 가능. “미서명”으로 단정하거나 자동 재결제하지 않음 |

숫자로 된 token/challenge TTL은 운영·profile 선택에서 정한다. 제출 자격 기한은 원 승인 기한과 관련 인증·세션·정책 기한의 최솟값을 넘지 않는다. read-only 복구는 별도 현재 인증으로 기한을 정하되, 과거 signed bytes를 반환해도 되는 현재 위험/권한을 재검사한다. token이 만료되지 않았더라도 현재 권한 revision/세션/epoch가 틀리면 거절한다.

opaque token의 검증용 hash와 원 응답 재전달용 암호화 저장은 역할이 다르다. 재전달 blob은 짧은 보호 보관 기간과 동일 수신자 접근 제한을 갖는다. 그 보관 기간을 벗어난 멱등 재시도는 새 토큰을 몰래 만들어 주지 않고 재인증을 요구한다. 실제 저장 방식과 보관 기간은 다음 저장 설계에서 확정한다.

발급 응답 전체를 잃으면 grantId도 모를 수 있다. 이때 AC-01의 `replacement={kind:issuance_key, issuanceIdempotencyKey:원 AC-02 키}`를 사용한다. 서버는 현재 인증·원 context·원 actor·sender·논리 발급 행위의 멱등 범위에서만 predecessor를 찾고, challenge 응답의 replacesGrantId로 비밀 없는 참조를 돌려준다. 다른 사람의 키나 무관한 source를 조회할 수 없다. 원 발급/lineage 기록까지 없으면 재발급으로 덮어쓰지 않고 대사 오류로 멈춘다.

동일 predecessor에서 challenge 둘이 발급되어도 AC-02는 저장된 predecessor state/revision을 CAS하여 **successor 하나만** 연결한다. 먼저 완료한 후속 grant와 충돌하면 새 형제 grant를 만들지 않고 현재 권한에 따른 기존 결과 대사 또는 409로 처리한다. 철회와 교체도 같은 경계에서 직렬화한다. revoked/compromised lineage는 replacement로 되살리지 않는다. active 또는 단순 expired인 미교체 자격만 독립적인 현재 인증 후 교체할 수 있다. 계정 주체는 같은 accountId여야 하며, 재인증으로 바뀐 authSessionId는 새 grant에 기록하고 원 grant를 종료한다. guest는 같은 terminal/attempt/session/device/epoch를 모두 유지해야 한다.

### guest 복구의 한계와 bootstrap

API-032의 order_payment_capability와 pairing challenge, API-034의 주소 소유 증거는 기존 진입 경계다. 이번 자격은 그 경계를 인증된 terminal/session 및 검증된 기기 peer에 결합한 **서버 측 원 기록**에서만 발급한다. 단말이 자기 sessionId와 payer 문자열을 주장하는 것으로 충분하지 않다. 현장 물리 승인·trust registry와 이 결합을 입증하는 구체 profile은 미선정 항목으로 남는다.

같은 검증 세션에서 단순 연결이 재개되고 관련 인증이 유효하면 AC-01/02로 원 context의 현재 허용 자격을 재취득할 수 있다. 연결 복구가 암호학적 같은 세션임을 확인하지 못하면 새 세션으로 취급한다. 원 세션 만료·단말 교체·기기 초기화 후의 guest 결과 공개/재제출은 자동 허용하지 않는다. 대사와 비민감 매장 주문 상태 확인은 계속하되, 과거 guest 서명 결과의 새 수신자 복구는 별도 신뢰 설계가 필요하다. 모바일 계정을 강제로 만들어 대체하지 않는다.

## 6. API-020 projection과 제출 경계

```text
data.operation = 기존 Operation
data.approval = null | {
  sourceKind,
  progress,
  snapshot: null | 원천별 Snapshot,
  signingResult: null | SigningResult
}
```

`approval=null`인 녹음·AI·FOTA 등 operation은 기존 기능별 접근/결과 정책을 유지한다. 승인 operation은 progress 권한이 기본이며 snapshot과 signed result를 별도로 검사한다. 승인 결과 bytes나 보호 객체의 접근 주소를 기존 operation.resultRef에 우회 노출하지 않는다. 명시적으로 요청한 결과의 조회권이 없으면 존재를 숨기는 403/404 정책에 따른다. includeSigningResult 기본값은 false이며 true는 권한 부여가 아니다. query 및 전체 HTTP shape는 다음 통합 envelope에서 고정한다.

sourceKind와 snapshot variant는 schema에서 묶는다. progress/snapshot/result의 모든 ID·digest와 부모/자식 관계, source별 signer·epoch·기한·current policy 일치는 서버가 추가로 검증한다. context와 결과 증거의 ID만 같은 것으로 진위를 인정하지 않는다. `closed`는 종료 표현, `blocked`는 정책상 차단으로 구분하고 둘 다 온체인 서명 취소가 아니다.

API-018에는 공통 SigningResult 전체와 submit_exact 자격 및 sender proof를 보낸다. 저장된 source·버전으로 adapter를 선택하고 검증한다. SS-T03 저장 접수와 SS-T04 전송 permit은 분리한다. 자격 철회/hold와 permit을 직렬화하되 이미 permit이 발급되거나 signed bytes가 노출된 경우 원격 전송 취소를 보장하지 않는다. 환불 예약은 조회 실패·기한 경과만으로 해제하지 않는다.

## 7. 검증과 남은 설계

합성 예제로 세 source·HW/Cloud 선택·owner/terminal 메시지·일반 operation·grant 요청/응답의 허용/거절 형태를 검사한다. [문서 검증기](validate_approval_access.py)는 참조·기준 해시·필드 일치도 검사한다. 실제 인증/암호/동시성/실기 검토 사례 22개는 모두 `not_run`이다. profile 검증, 접근권의 실시간 변화, 기존 세션 복구와 보호 토큰 전달을 schema 테스트 결과로 대체하지 않는다.

다음은 이 공통 타입을 **기존 API의 전체 HTTP envelope, 접근 정책·화면, 자격 저장의 원자 경계**에 연결하는 작업이다. AC 신규 경로를 기준 catalog에 등록할지와 이전 후보 이력 보존도 함께 처리한다. guest 새 세션 복구, 정확한 암호/프로토콜 profile, MPC 구성, HW/Cloud 주소 관계는 미확정 상태다. RR-DEC-01 답변이 없으므로 백업 추가나 복구 없는 새 초기화 허가를 확정하지 않는다.

12주 15개 요구·104개 작업·320개 세부 작업을 유지한다. 담당자와 공수는 배정하지 않는다.

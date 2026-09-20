# 지갑 선택·승인·제출 API 계약 후보

2026-09-18 · **상세 설계, 기준 명세 병합 전.** [지갑 사용·복구 설계](wallet-control-recovery-design.md)의 WC-C01/C03을 구체화한다. 8개 기존 API의 전체 논리 요청·성공/오류 응답을 작성했다. 원래 경로와 권한 이름을 유지하며, 개인 송금 경로부터 승인 문맥을 연결한다. 암호 profile·실제 HTTP 서버·앱·펌웨어 구현은 포함하지 않는다.

[계약 원본](wallet-api-candidate.json) · [JSON Schema](wallet-api-candidate.schema.json) · [합성 예제·검토 사례](wallet-api-examples.json) · [검증기](validate_wallet_api.py)

후속 [승인·제출 저장 경계](signing-submission-storage-design.md)에서 준비 commit·worker 전송 허가·관측 적용과 개인/guest/환불 원천 책임을 구체화했다. 기준 계약 병합 전 상태는 유지한다.

## 1. 이번 계약의 범위

| API | 경로 | 후보에서 정한 내용 |
|---|---|---|
| API-007 | GET `/v1/wallets` | 로그인한 본인의 지갑 연결과 승인 수단 조회, 페이지 처리 |
| API-008 | GET `/v1/stores/{storeId}/wallets` | 해당 매장 조회권으로 지갑 연결 조회, 개인 지갑과 분리 |
| API-009 | GET `/v1/wallets/{walletId}/balances` | 명시한 개인/매장 문맥의 주소·자산 잔액과 관측 시점 |
| API-017 | POST `/v1/transaction-intents` | 개인 EOA 송금용 지갑 선택·검토·서명 문맥 고정 |
| API-015 | POST `/v1/wallets/{walletId}/signing-sessions` | 이번 후보의 개인 송금에 대한 Cloud MPC 승인 요청 |
| API-018 | POST `/v1/transaction-submissions` | 원 intent와 정확한 서명 결과 대조, 원천별 제출 조건 유지 |
| API-019 | GET `/v1/transactions/{transactionRef}` | 기존 체인 실행·정규성·확정 조회 유지 |
| API-020 | GET `/v1/operations/{operationId}` | 원 작업·승인 snapshot·서명/제출 진행 복원 |

다른 상품의 범위를 삭제하지 않는다. 주문 결제 API-034, 환불 승인 API-037, 스마트 계정 API-056, DID/x402 등은 각각의 기존 경로를 유지한다. 이 후보만으로 모든 상품의 승인 adapter 연결을 완료했다고 보지 않는다. API-015의 기존 상품 경로에도 이번 개인 송금용 필드를 무조건 강제하지 않는다.

이번 JSON Schema는 HTTP 요청을 `path/headers/query` 또는 `path/headers/body`로 정규화한 전체 envelope다. 메서드·경로·인증·헤더 변환·오류 의미는 계약 원본과 이 문서에 함께 정의한다. 실제 배포 가능한 OpenAPI나 암호 검증기라는 뜻은 아니다.

## 2. HTTP 연결 규칙

- 보호된 전송, JSON 응답, `Cache-Control: no-store`를 적용하는 안이다. 인증·승인 proof·서명 payload를 access log나 오류 메시지에 넣지 않는다.
- `headers.requestId`는 `X-Request-ID`, `authorization`은 `Authorization: Bearer …`, `contractVersion`은 `X-Contract-Version`, 변경 요청의 `idempotencyKey`는 `Idempotency-Key`로 전달한다. guest Bearer는 제한된 capability일 수 있으며 앱 계정 로그인으로 제한하지 않는다.
- 새 계약은 `wallet-approval-candidate-1`의 명시적 버전 선택을 전제로 한다. 지원하지 않으면 400 `INVALID_REQUEST`로 거절하고 구버전으로 조용히 재시도하지 않는다. `/v1` 경로가 같다는 이유로 기준 DTO와 섞지 않는다. 협상 방식 자체도 병합 전 제안이다.
- GET query는 단일 값으로 인코딩한다. 생략 가능한 필드는 생략하며 문자열 `null`을 보내지 않는다. API-009는 `contextKind=personal` 또는 `contextKind=store&storeId=…`다. 개인 문맥에 storeId를 붙이면 거절한다.
- API-007/008은 limit 1~100, 기본 20과 불투명 cursor를 제안한다. cursor는 현재 호출자·문맥·snapshot·마지막 정렬 키·필터와 결합한다. `walletId, bindingId` 기준의 안정된 순서를 사용하며, snapshot을 유지할 수 없으면 409 `CURSOR_STALE`로 처음부터 조회한다. 다음 페이지도 현재 권한을 검사한다.
- 007/008/009/019는 200, 017은 201, 015/018/020은 기준 계약과 같은 202다. API-020의 202만으로 작업이 아직 진행 중이라고 판단하지 않고 응답 상태를 읽는다.

## 3. 지갑 선택 정보

API-007/008의 각 `WalletChoice`는 기존 WalletView에 문맥, bindingId/revision, 승인 수단 목록을 연결한다. wallet의 signerRef는 기존 참조를 보존하는 필드이고, 후보의 signers 목록을 물리 DB에 이미 다중 signer로 구현했다는 뜻은 아니다. 목록은 현재 권한으로 확인 가능한 항목만 반환한다.

| 필드 | 의미·검사 |
|---|---|
| `context` | personal 또는 store+storeId. 사용자 계정 ID는 인증에서 결정하며 클라이언트가 지정하지 않음 |
| `bindingId`, `bindingRevision` | 해당 문맥의 지갑 연결·권한 버전. 주소가 같아도 다른 binding의 권한을 합치지 않음 |
| `signerRef`, `epoch`, `profileId` | 승인 경로의 공개 참조와 세대. 원시 키·key handle·share·기기 신원 비밀을 반환하지 않음 |
| `supportedRequestKinds` | 지원하는 요청 형식. 특정 거래의 승인권을 의미하지 않음 |
| `state`, `observedAt` | 사용 가능/실기 확인 필요/복구 중/차단/미지원 안내와 관측 시점 |

`available`은 조회 시점의 안내다. 실제 연결·최신 권한·세대·반납 제한은 승인 직전에 다시 확인한다. 서버가 실기 상태를 모르면 `requires_device_check`를 표시하고 추정으로 사용 가능을 보장하지 않는다.

API-009는 선택한 문맥의 walletId·chainId·address와 기존 BalanceView들을 반환한다. 잔액 합산은 같은 체인/주소를 중복 계산하지 않지만 개인/매장 기록은 합치지 않는다. 조회 실패·unknown을 0원으로 표시하지 않으며, 소수점·토큰 주소는 등록 자산 설정으로 확인한다.

## 4. API-017 — 개인 송금 검토를 고정

요청은 기존 walletId/chainId/action/actionInput에 `selection`을 추가한 후보다.

```json
{
  "walletId": "wallet_a",
  "chainId": 8283,
  "action": "personal_transfer",
  "actionInput": {
    "recipient": "0x1111111111111111111111111111111111111111",
    "assetId": "dummy_usdc",
    "atomicAmount": "1000000"
  },
  "selection": {
    "walletId": "wallet_a",
    "context": {"kind": "personal"},
    "bindingId": "binding_a",
    "expectedBindingRevision": 2,
    "expectedWalletRevision": 1,
    "signerRef": "signer_a",
    "expectedSignerEpoch": "3"
  }
}
```

주소·금액은 합성 예시다. 이 경로는 개인 EOA의 native 또는 등록 ERC-20 전송만 구체화한다. store 문맥, smart account, 임의 calldata·sourceId·refundId·payment attempt 덮어쓰기는 지원하지 않는다. 지갑이 목록에 보인다는 이유로 이 경로에서 모든 동작이 허용되는 것은 아니다.

서버는 두 walletId의 일치, 실제 사용자 문맥·binding·지갑 버전·signer 세대·자산·지원 profile·가스·nonce를 확인한다. `sourceKind=personal_transfer`와 sourceId는 서버가 만든다. 같은 수취인·금액으로 송금해도 주문 결제나 환불 원장 완료로 자동 승격하지 않는다.

응답은 기존 intent/review/expiry에 `approvalContext`와 `operation`을 추가한다. 승인 문맥은 다음을 고정한다.

- 원 intent/operation/source, 선택 wallet·binding·권한 revision·signer·세대
- 체인·payer 주소·요청 형식·서명 profile
- 정확한 transaction의 payloadDigest와 사용자 검토 내용의 reviewDigest, 기한

금액·수취·nonce·가스·토큰 호출은 intent의 선정 profile에 포함하고 서버/앱/NU가 실제 unsigned payload와 대조한다. 이번 schema는 ProfileInput 내부의 인코딩·해시 계산·curve 지원을 증명하지 않는다. 정확한 거래를 디코딩·검증할 수 없는 profile은 거절한다.

불변 snapshot과 현재 권한은 별개다. 서명/제출 시 snapshot을 새 값으로 덮어쓰는 대신 변경을 감지해 새 검토를 요구한다. 지갑·주소뿐 아니라 가스·nonce 변경도 기존 승인을 재사용하지 않는다.

## 5. API-015 / BLE — 실제 서명 경로 확인

Cloud는 API-015에 intentId, approvalContextId, 승인 proof를 보낸다. 서버는 path walletId와 원 intent를 대조하고 현재 권한·세대·지원 profile을 검사한 뒤 MPC 세션을 만든다. 서명 세션 operation은 상위 wallet-flow operation과 별도 ID일 수 있으며 관계를 서버에 저장한다. API-020으로 각각의 원 작업을 조회한다.

HW는 기존 `wallet.sign.prepare/result`와 IF-02/10을 사용한다. API-015의 MPC 참여를 추가로 요구하지 않는다. 기기에서 원 transaction과 결합된 검토·물리 승인이 필요하다.

**같은 EOA 서명이 검증돼도 특정 NU나 MPC 세대가 서명했다는 사실까지 확인되지는 않는다.** 따라서 API-018의 개인 송금 후보는 별도 `approvalEvidence`를 요구한다.

| 증거 종류 | API-018 후보 값 | 서버가 확인할 결합 |
|---|---|---|
| `device_result` | profileId와 검증할 profile proof | 신뢰된 현재 기기/세대·approvalContext·원 digest·정확한 서명 결과·승인 사건 |
| `mpc_session` | MPC operationId | 승인한 동일 intent·참여자 세대·완료된 세션·검증된 서명 결과 |

객체 모양이나 클라이언트가 보낸 signerRef만으로 통과시키지 않는다. 특정 기기/세대 증거를 제공하지 못하는 profile은 그 보장을 요구하는 개인 송금 경로에서 거절한다. 증거의 구체 인코딩과 장치 신뢰 검증은 미선정이며, proof 필드가 존재한다고 실기 증명이 완료된 것으로 표시하지 않는다.

## 6. API-018 — 제출과 재시도

기존 body의 intentId/signedPayload는 유지한다. 개인 송금 후보에는 위 approvalEvidence를 추가하며 필수 여부는 **클라이언트 분기가 아니라 저장된 원 intent**로 판단한다. schema가 guest 호환을 위해 evidence 생략을 허용해도 개인 후보의 생략은 의미 검증에서 거절한다.

1. 원 intent와 제한된 제출 권한을 확인한다. API-034의 guest는 attempt/device/session capability, API-037의 환불은 매장 승인·원지급·예약·funding 조건을 유지한다. guest에 accountId나 모바일 로그인을 새로 요구하지 않는다.
2. 원 요청 종류에 맞는 검증기를 선택하고 파싱한 체인·payer·nonce·수취·값·calldata·가스와 정확한 서명 결과를 대조한다. 일반 EOA 제출을 UserOperation·DID·x402 proof로 확대하지 않는다.
3. 개인 후보는 검증된 승인 출처까지 결합한다. 동일 주소로 서명한 외부 지갑의 결과를 선택한 NU의 승인으로 간주하지 않는다.
4. 현재 제출 제한과 원천별 가용 상태를 확인한다. 권한 철회·기한 경과·환불 hold 등은 새 제출을 차단할 수 있지만 이미 제출된 거래의 관측을 삭제하지 않는다.
5. 서명 payload 지문·예상 transaction hash·operation·원천 연결·dispatch 기록을 내구 저장한 뒤 네트워크에 보낸다. 응답 유실은 원기록을 조회한다. exact bytes의 네트워크 재전송도 기존 승인 작업을 이행하는 worker 정책으로 다루고 새 사용자 결제로 생성하지 않는다.

| 사건 | 처리 |
|---|---|
| 같은 권한 범위·멱등키·같은 정규화 입력 | 현재 결과 조회권을 검사하고 기존 결과 반환 |
| 같은 키·다른 입력 | 409 IDEMPOTENCY_CONFLICT |
| 다른 키로 같은 intent/동일 bytes 재접수 | 같은 submission/operation으로 수렴; 새 dispatch 생성 금지 |
| 같은 intent에 다른 bytes·nonce·가스·서명 결과 | 충돌. 기존 서명의 외부 제출 가능성을 대사하고 새 검토 필요 |
| 수락 기록 없는 최초 제출이 만료/철회됨 | 새 제출 거절. 기존 서명 자체가 온체인에서 무효라고 표시하지 않음 |
| 수락 기록은 있는데 응답이 유실된 뒤 만료됨 | 현재 조회권으로 원결과 재조회; 만료를 새 거래 생성 이유로 사용하지 않음 |
| 같은 주소의 외부 지갑이 nonce 사용 | 원거래/경쟁 거래 대사. 서버가 외부 지갑을 잠갔다고 가정하지 않음 |

업무 원장과 네트워크 전송의 원자성을 하나의 DB transaction으로 보장한다고 쓰지 않는다. 원천 상태 검사·중복 예약·outbox의 DB 경계, worker 재전송, RPC/Indexer 관측을 각각 구현하고 개발 단계에서 경쟁·중단 시험을 해야 한다.

## 7. API-019/020 — 다시 열었을 때의 화면

API-019는 기존 체인 실행·정규성·확정을 반환한다. HTTP 202, MPC 서명 완료, API-018 수락을 결제 성공으로 표시하지 않는다. 주문/환불 완료는 원천별 업무 조회로 확인한다.

API-020은 기존 operation과 함께 `walletFlow`, `approvalSnapshot`을 추가한 후보다. 해당 개인 송금의 현재 조회권이 있을 때만 서버가 원 snapshot을 제공한다. 원 snapshot은 API-017의 intent/review/expiry/approvalContext이며, 앱이 현재 선택값으로 재구성하지 않는다. 그 외 operation은 두 필드가 null이고 기존 source-specific 조회를 따른다. snapshot이 없거나 현재 유효성을 검증할 수 없으면 새 승인을 진행하지 않는다.

`walletFlow.phase`의 제안 값은 review_ready, signing, signature_unknown, signed, submission_unknown, submitted, closed다. 서명 전달 여부를 모르는 signature_unknown과 제출 결과를 모르는 submission_unknown을 구분한다. closed도 원 operation의 실패/종료 정보와 같이 읽으며 온체인 서명이 폐기됐다는 뜻은 아니다. 기존 Operation.state를 이 enum으로 덮어쓰지 않는다.

현재 계정/매장/원천별 조회권 또는 정확한 operation capability를 재검사한다. 이전 대여자·다른 매장·새 사용자에게 과거 snapshot·MPC 내부정보·기기 proof를 넘기지 않는다. 개인 조회권이 사라져도 내부 체인 대사는 지속한다.

## 8. 오류와 화면 행동

| HTTP | 코드 | 사용자/클라이언트 행동 |
|---|---|---|
| 400 | INVALID_REQUEST | 필드·버전·query 수정; 무시한 필드로 진행 금지 |
| 401 | AUTH_REQUIRED | 해당 계정/제한 capability 인증 회복 |
| 403/404 | FORBIDDEN/NOT_FOUND | 허용 범위 안내. 사적 자원 존재가 드러나지 않는 응답 유지 |
| 409 | SELECTION_STALE | 지갑·binding·signer 상태 새로 조회하고 재검토 |
| 409 | SIGNER_STATE_CONFLICT | 복구/반납/기존 작업 상태 확인; 다른 지갑 자동 전환 금지 |
| 409 | IDEMPOTENCY_CONFLICT | 원 요청과 비교; 새 키만 발급해서 재결제하지 않음 |
| 409 | CURSOR_STALE / NONCE_CONFLICT | 목록 재조회 / 원거래 대사 |
| 422 | UNSUPPORTED_ACTION / APPROVAL_INVALID / PAYLOAD_MISMATCH | 승인 진행 중단; 미지원 형식 우회 금지 |
| 422 | INTENT_EXPIRED | 원 제출 존재부터 확인하고 필요한 경우 새 검토 |
| 429/503 | RATE_LIMITED / DEPENDENCY_UNAVAILABLE | 원 operation 조회 또는 대기; 서명/제출 자동 중복 금지 |

오류 envelope는 `{requestId, error:{code,retryable,operationId,safeMessage,nextAction}}`다. operationId도 현재 조회권이 있을 때만 노출한다. `retryable`은 동일 조회/요청 복구 가능성이지 새로운 서명·결제 허가가 아니다. 세부 원천 오류는 기존 계약을 유지하며 위 공통 코드로 환불/반납 검사를 지우지 않는다.

## 9. 남은 병합·검증

WalletChoiceProjection, ApprovalBinding, SignatureProvenance, SubmissionDispatch를 기존 테이블/operation에 연결할 논리 자원으로 식별했다. 새 테이블 4개를 확정한 것은 아니다. 기존 카탈로그 107개·BLE 34개·화면 37개와 기존 schema·권한 파일을 유지하고 해시로 확인한다.

검증기는 8개 endpoint의 참조, 전체 요청/성공/오류 envelope 합성 예제 18건, 거절 shape 11건을 확인한다. 별도 16개 검토 사례는 기대 결과만 작성했고 아직 실행하지 않았다. schema 통과는 주소·digest의 진위나 권한·동시성·원자성의 실행 검증이 아니다.

MPC 구성, HW/Cloud 별도 주소, 개인 백업 정책은 이번 계약으로 확정하지 않는다. 기존 RR-DEC-01은 답변 대기이며 초기화 제한을 유지한다. 다음 설계는 **승인 기록·서명 출처 증거·제출 상태의 저장 경계와 원천별 adapter 연결**이다. 코드 구현 착수와는 구분한다.

# 키오스크 결제·점주 환불의 승인 문맥과 증거 전달

2026-09-18 · **상세 설계 후보, 기준 계약 미병합·제품 구현 미착수.** [승인·제출 저장 설계](signing-submission-storage-design.md)의 SS-A02/03을 HTTP·BLE 메시지에 연결한다. NU는 Zephyr 기반이며, 원래의 기기 단독 결제와 별도 Cloud MPC 요구를 유지한다.

**주소 소유 증명 → 거래 내용 검토 → 최종 서명·승인 증거 → 원천별 제출**을 구분한다. 결제는 계정 없는 attempt/device/session 경로를 유지하고, 환불은 점주의 업무 승인과 실제 매장 지갑 서명권을 별도로 확인한다.

[계약 원본](source-approval-candidate.json) · [후보 Schema](source-approval-candidate.schema.json) · [합성 예제와 검토 사례](source-approval-examples.json)

후속 [승인 계약 병합 계획](approval-contract-merge-plan.md)에서 개인·guest·환불 후보의 충돌과 버전·권한·기준 반영 범위를 정리했다. 후보의 독립 버전을 이미 통합한 것은 아니다.

## 1. 적용 범위와 버전

| 경계 | 이번 후보에서 연결한 내용 |
|---|---|
| API-034 | inline 주소 소유 증거를 검증해 guest 결제 snapshot 생성 |
| API-037 | 환불/funding revision과 매장 signer 선택을 검증해 환불 snapshot 생성 |
| API-015 | 선정된 매장 Cloud signer의 환불 MPC 승인 요청 |
| API-018 | 정확한 서명 bytes와 원천별 승인 증거 제출 |
| API-020 | 진행·원 snapshot·제출 가능한 서명 결과를 권한별로 조회 |
| payment.identify | intent 생성 전 주소 공개 동의·소유 증명 |
| payment.prepare/result | 실제 결제 내용 검토·기기 승인·서명 출처 증거 |
| wallet.sign.prepare/result | owner 세션에서 매장 HW 지갑 환불 승인·서명 출처 증거 |

새 후보 버전은 `source-approval-candidate-1`이다. HTTP의 `X-Contract-Version`, 논리 BLE envelope의 protocolVersion에 명시한다. 기존 `wallet-approval-candidate-1`은 개인 송금 후보로 유지하며, 이 버전의 추가 필드를 구버전 DTO에 섞지 않는다. API-017로 결제·환불을 우회하거나 지원하지 않는 버전에서 자동 강등하지 않는다.

기존 API 경로·기본 권한 이름은 유지한다. API-020의 지정 signer 조회와 제출 가능 결과 접근은 아래에 설명한 **추가 권한 정책 후보**다. 이름이 같다고 기존 접근 매핑이 이미 이를 지원한다고 보지 않는다. API-036의 원지급별 환불 예약, API-019의 체인 조회, API-035/038의 업무 결과 조회도 유지한다.

HTTP 요청은 path/headers/body 또는 query, 응답은 statusCode와 `{requestId,data}`/`{requestId,error}`로 표현한다. X-Request-ID, Authorization, POST의 Idempotency-Key는 기존 후보와 같은 변환을 사용한다. JSON 응답·no-store를 제안하며 proof와 signed bytes는 로그에 남기지 않는다. BLE는 sessionId/role/messageId/sequence/kind/command/payload를 가진 **인증된 논리 envelope**다. GATT UUID·MTU·분할 전송·암호 profile은 미선정이다.

## 2. guest 결제 — 주소 증명과 결제 승인을 분리

### 2.1 payment.identify

입력은 attemptId, 서버 challenge, inline merchantProof다. 기존 API-032의 challenge 발급과 인증된 결제 세션이 선행하며, 발급/검증 profile의 호환성은 후속 병합 조건이다. 키오스크가 임의로 만든 challenge나 매장 표시 문자열을 신뢰하지 않는다.

사용자가 NU에서 매장·주소 공개를 확인하면 다음을 반환한다.

- selectedPayerAddress
- ownershipEvidence.binding: chain/store/order/attempt, terminal/session, 세션용 deviceSessionRef, challengeId/challenge, 선택 payer 주소, 기한
- ownershipEvidence.proof: profileId와 inline evidenceBytes

이 단계에는 아직 intent가 없으므로 intentId나 거래 digest를 요구하지 않는다. 증명 목적은 선택한 주소의 통제와 현재 거래 상대·세션 연결이다. 주소 소유 증명이 최종 금액 승인이나 임의 송금 동의가 되지 않는다.

deviceSessionRef는 검증된 세션 안의 기기 참조다. 전체 대여 이력·계정 ID·모든 지갑 목록·기기 장기 식별자를 키오스크에 공개하는 용도로 쓰지 않는다. profile에 필요한 공개 증명 정보와 장기 상관관계 노출의 범위는 별도 검토한다.

### 2.2 API-034

path attemptId와 body signerAddress/sessionId/ownershipEvidence를 보낸다. 기존 ownershipProof 참조 대신 **검증할 수 있는 inline 증명**을 전달하는 버전 변경안이다. 서버에는 없는 앱·기기 로컬 proofRef만 보내지 않는다.

서버는 원 challenge 발급 기록·store/order/terminal/attempt/session·payer·기한을 대조하고 proof의 실제 서명/신뢰 근거를 검증한다. 성공하면 EoaTokenIntent와 PaymentContext, reviewProof를 묶은 PaymentSnapshot, 복구용 operation을 반환한다. 같은 발급 요청의 재시도는 기존 결과로 수렴하며 새 지급을 만들지 않는다.

PaymentContext에는 다음을 고정한다.

| 묶음 | 내용 |
|---|---|
| 원천 | sourceKind=payment, store/order/attempt와 attemptRevision |
| 승인 사건 | approvalContextId/contextDigest, intentId/operationId, 기한 |
| 거래 | chain/payer, payloadDigest/reviewDigest, eoa_transaction, profileId |
| 상대·기기 | terminal/session/deviceSessionRef/deviceEpoch, 검증된 identifyEvidenceId |
| 근접 정책 | 서버가 정한 proximityPolicyId/proximityRequired |

계정 ID와 walletId를 새 필수값으로 요구하지 않는다. deviceEpoch와 기기 참조는 검증된 세션에서 결정하며 단순 클라이언트 주장으로 받아들이지 않는다. reviewProof는 서버가 승인한 snapshot을 기기가 검증할 수 있도록 전달하는 증명이다. 정확한 서명 형식·신뢰 키·정규화 규칙을 선정하기 전에는 이를 실제 구현 호환으로 표시하지 않는다.

### 2.3 payment.prepare/result

payment.prepare는 attemptId, **전체 PaymentSnapshot**, 기기 세션 안에서 이미 검증한 merchantProofRef/proximityRef를 전달한다. NU가 HTTP로 intent를 내려받을 수 있다고 가정하지 않는다. merchantProofRef는 로컬 세션 참조이므로 서버에서 검증할 inline proof와 혼동하지 않는다.

기기는 서버 reviewProof, context/거래/세션 결합, 실제 calldata와 표시 금액·수취·토큰·가스, 지원 profile을 확인한다. proximityRequired=true이면 올바른 peer의 신선한 인증 관측이 필요하며 null/과거 관측으로 통과시키지 않는다. proximityRef의 null 허용은 서버 정책이 명시적으로 요구하지 않는 경우만 표현한다. 클라이언트가 요구 여부를 끄지 못한다.

prepare 응답의 reviewId, approvalContextId/contextDigest/payloadDigest를 확인한 뒤 **별도의 물리 승인**으로 서명한다. payment.result는 동일 session·reviewId·approvalContextId에 대해서만 조회한다. 결제 단말 role은 payment_terminal이며 일반 owner 명령·import·다른 지갑 서명 권한으로 넓어지지 않는다.

## 3. 점주 환불 — 업무 승인과 지갑 서명 분리

### 3.1 API-037 요청과 승인 완료 snapshot

API-036에서 원 payment allocation·자산·금액·목적지·예약을 먼저 만들고, API-037에 다음을 보낸다.

```json
{
  "expectedRevision": 1,
  "expectedFundingRevision": 5,
  "merchantSigner": {
    "walletId": "merchant_wallet",
    "walletBindingId": "merchant_binding",
    "expectedWalletRevision": 2,
    "expectedBindingRevision": 4,
    "signerRef": "merchant_signer",
    "signerKind": "hardware_eoa",
    "expectedSignerEpoch": "3"
  }
}
```

값은 합성 예시다. request의 revision은 **승인 전 조건**이고 응답 context는 **승인 commit 후 값**이다. 예를 들어 refund revision이 1→2로 증가하면 authorizedRefundRevision=2, RefundView.revision=2로 일치해야 한다. funding revision도 같은 commit에서 확정된 값을 기록한다. 이전 요청 숫자를 그대로 복사하지 않는다.

동일 저장 경계에서 현재 매장 자금 승인권·최근 인증, 원 refund/payment allocation·활성 reservation·funding hold, 금액·목적지 snapshot과 선택 signer를 확인한다. 수취 주소·paymentId·금액은 원 예약에서 가져오며 이번 요청에서 바꾸지 않는다. 선택 지갑이 허용된 매장 자금 원천인지도 확인하고 임의 개인 지갑으로 자동 대체하지 않는다.

응답은 RefundView, RefundSnapshot, operation이다. RefundContext에는 refundId/paymentId/reservationId, 승인 후 refund/funding revision, merchantAuthorizationId, store와 선택 signer를 포함한다. amount/asset/recipient는 정확한 EoaRefundIntent에 결합한다. paymentId는 원 지급 allocation ID이며 txHash나 주문 ID가 아니다.

### 3.2 실제 signer의 승인

- HW 지갑: 유저 앱이 현재 owner BLE 세션에서 wallet.sign.prepare에 전체 RefundSnapshot을 보낸다. 기기는 환불 동작과 실제 거래를 검토하고 물리 승인한다. payment_terminal role은 이 경로를 사용할 수 없다.
- Cloud 지갑: API-015에 path walletId, intentId, approvalContextId와 실제 사용자 승인 proof를 보낸다. 서버가 원 RefundContext의 cloud_mpc_eoa 선택·현재 epoch·정확한 매장 지갑 서명권을 확인하고 원천에 연결된 MPC operation을 만든다.

업무 승인자 A와 실제 signer 사용자 B가 달라도 될 수 있도록 후보 정책을 둔다. A의 accountId와 같다는 이유만으로 서명권을 부여하지 않으며, 다르다는 이유만으로 적법한 B를 거절하지 않는다. B는 선택된 signer와 해당 매장 지갑에 대한 현재 권한을 독립적으로 입증해야 한다. 단순 매장 소속·자산 조회권은 부족하다.

API-020의 operation 소유자 검사만으로 B의 접근을 허용할 수 없다면, **정확한 source/선택 signer의 현재 권한을 확인하는 접근 조건**을 추가한다. 업무 승인자가 공유한 operationId는 탐색 참조일 뿐 권한 증명이 아니다. 승인 요청을 B에게 전달하는 앱 화면/알림 방식은 별도 UX이며 새로운 외부 전송을 실행한 것은 아니다.

## 4. 서명 결과에 포함할 증거

기기 결과가 signed이면 다음 SigningResult를 반환한다. 그 외 awaiting_user/rejected/expired/unknown은 result=null이다.

| 필드 | 의미 |
|---|---|
| intentId / approvalContextId | 원 승인 사건 |
| signedPayload | 정확한 EOA signed transaction bytes |
| approvalEvidence.kind | device_result 또는 mpc_session |
| contextDigest / payloadDigest / signedPayloadDigest | 승인 문맥, 서명 대상, 실제 반환 결과의 별도 결합 |
| reviewId / deviceSessionRef / deviceEpoch / proof | 기기 결과일 때 실제 승인 사건과 검증 가능한 inline 증거 |
| operationId / participantEpoch | MPC 결과일 때 서버가 확인할 원 signing session과 세대 |

BLE payment.result와 wallet.sign.result는 device_result만 반환한다. mpc_session 참조는 Cloud 결과 경로에서만 사용한다. 기기가 “MPC가 완료됐다”는 임의 operationId를 보내 통과하지 못한다.

EvidencePacket은 profileId/evidenceBytes를 담는 제한된 운반 형식이다. bytes 모양이나 클라이언트가 함께 보낸 digest가 증명의 진위를 보장하지 않는다. 실제 verifier는 증명 안의 목적·대상·세션·세대·digest·승인 사실을 기대 문맥과 대조하고 signed bytes의 hash도 직접 계산해야 한다. 식별 proof, 서버 reviewProof, 최종 기기 승인 proof는 목적/domain을 분리한다.

digest 입력의 순환도 피한다. payloadDigest는 선정 profile의 실제 서명 대상, signedPayloadDigest는 정확한 반환 bytes, reviewDigest는 버전이 지정된 검토 데이터에서 계산한다. contextDigest는 자기 자신과 외부 proof bytes를 제외한 승인 문맥을 정규화해 계산하는 안이다. 문자열을 임의로 이어 붙이지 않으며, 인코딩·domain·해시 규칙은 profile 선정 시 고정한다. 이 문서의 합성 digest가 계산 결과와 일치한다는 뜻은 아니다.

같은 EOA 키가 다른 기기에 import됐을 수 있으므로 EOA 서명만으로 선택한 NU·세대의 물리 승인을 증명했다고 표현하지 않는다. 증거를 만들어 검증할 수 없는 profile은 해당 경로를 거절한다. 새로운 암호 알고리즘이나 구현 완료를 주장하지 않는다.

## 5. API-018 제출과 저장 연결

이 버전은 intentId/approvalContextId/signedPayload/approvalEvidence를 받는다. guest/refund 모두 해당 증거를 필수로 전달하지만 서버 adapter 선택은 **저장된 원 intent/source**로 한다. sourceKind나 금액·목적지를 제출 요청으로 덮어쓰지 않는다.

API-034의 guest는 원 attempt/device/session capability로 제출하며 모바일 로그인·Cloud quorum을 요구하지 않는다. API-037의 환불은 원 예약/승인·현재 source hold·매장 signer 권한을 확인한다. MPC evidence는 동일 원천의 완료 세션·세대·정확한 결과와 맞아야 한다.

수락 뒤 SS-R01→SS-R02→SS-R04에 불변 연결을 저장한다. SS-T03 제출 준비와 SS-T04 worker 전송 허가는 별도다. 환불 승인 이후 funding 상태가 변하면 새 전송 허가를 다시 검사하고, 이미 존재할 수 있는 유효 서명의 예약을 함부로 해제하지 않는다.

같은 원 intent/bytes는 기존 제출로 수렴한다. 다른 context·다른 결과 또는 다른 intent에 같은 txHash를 재귀속하려는 요청은 충돌/비공개 거절한다. source 버전 거절을 개인 송금 API나 구버전 무증거 제출로 우회하지 않는다. 저장된 intent의 요구 버전을 서버가 검사해야 한다.

## 6. API-020: 진행 조회와 제출 가능한 결과 조회

`includeSigningResult`는 선택 query이며 기본값 false다. HTTP에서는 true/false 문자열을 파싱해 논리 DTO의 boolean으로 변환한다. 알 수 없는 값·중복 query는 거절한다.

| 요청 권한 | 반환 범위 |
|---|---|
| 원 operation 진행 조회권 | operation/progress. 필드마다 해당 source의 최소 공개 범위 |
| 원 승인 snapshot 조회권 | 원 PaymentSnapshot 또는 RefundSnapshot. 현재 사용자 선택으로 재구성하지 않음 |
| 정확한 서명 결과 조회권 + includeSigningResult=true | 서버가 실제 보유·검증한 해당 SigningResult. 현재 source/signer/audience를 재검사 |
| 결과 조회권 없이 includeSigningResult=true | 403 또는 존재를 숨기는 거절. 단순 필드 선택으로 권한 확대 금지 |

read_progress, read_snapshot, read_signing_result, submit_exact는 구분할 권한 의미다. 토큰에 들어갈 실제 scope 문법·발급·갱신 profile은 미선정이며, 클라이언트가 scope 문자열을 보내면 허용되는 구조가 아니다. 점주의 일반 매출·환불 조회와 제출 가능한 signed bytes 조회도 별개다.

서명 결과는 원시 키가 아니어도 자금을 이동시킬 수 있는 민감 자료다. generic operation.resultRef에 공개 URL로 담거나 일반 백오피스 다운로드로 제공하지 않는다. 현재 권한·객체 버전/digest·내용 존재를 확인한 보호 응답만 제안한다.

Cloud 결과는 서버의 완료 MPC session에서 실제 결과를 확보할 수 있을 때 반환한다. HW 결과는 기기에서 서버로 전달·검증되어 저장된 경우에만 반환할 수 있다. API-020이 아직 전달받지 않은 기기 서명을 만들어내지 않으며, 없으면 signingResult=null이다. null은 “서명되지 않았다”가 아니라 “이 응답에서 결과를 제공할 수 없다”다.

기기 연결만 끊겼다면 여전히 검증된 동일 session·review로 BLE 결과를 재조회할 수 있는지 검사한다. 새 세션에는 과거 결과를 자동 공개하지 않는다. 원 세션/제한 capability가 만료됐으면 새 인증·복구 절차가 필요하며, 그 갱신 경로가 정해지기 전에는 signature_unknown과 원 기록을 유지한다. 앱 재로그인이나 새 결제 생성으로 guest 복구를 대신하지 않는다.

API-020의 phase=signed, API-018의 202는 결제 성공이 아니다. 체인 실행은 API-019, 지급/환불 업무 결과는 API-035/038로 확인한다.

## 7. 오류와 필수 교차 검사

공통 오류 envelope는 기존 Error 형식을 사용한다. 형태/버전 오류 400, 인증 401, 권한/비공개 자원 403/404, 상태·revision·멱등 충돌 409, proof/세션/지원 형식·payload 의미 오류 422, 제한/의존 서비스 429/503을 제안한다. 반환 문구에는 비밀·타인의 ID·서명 원문을 넣지 않는다.

| 검사 | 수행 경계 |
|---|---|
| path attemptId, body payer/session, identify binding, 서버 challenge가 일치 | API-034 |
| snapshot context의 원천·intent·chain/payer/digest/기한/가게와 실제 거래 일치 | prepare, API-015/018 |
| 승인 완료 RefundView.revision과 authorizedRefundRevision 일치; 원 예약/지급/금액/목적지 일치 | API-037 생성·후속 검증 |
| 선택 signer/profile/epoch와 실제 증거 경로 일치 | API-015, BLE result, API-018 |
| 기기 세션 role/peer와 실제 인증된 role/peer 일치 | 모든 BLE 메시지 |
| null 근접 참조가 정책 우회가 되지 않음 | payment.prepare |
| 제출 가능한 결과 접근과 진행 조회가 분리됨 | API-020 |
| 원천/버전별 재시도와 제출 허가·관측이 이어짐 | API-018 및 SS-T03~06 |

JSON Schema는 이러한 실제 권한·암호·시계·관측 사실을 확인하지 않는다. schema로 가능한 variant/필수값 검사는 수행하고 나머지는 명시한 교차 검증과 개발 단계 사례에 남긴다.

## 8. 기준 명세에 반영할 것

1. API-034/037/015/018/020의 source 버전과 역할별 응답·증거 타입을 기존 개인 지갑 후보와 함께 병합한다.
2. BLE 5개 명령의 요청/결과 타입과 서버 review/기기 결과 proof 검증 경계를 연결한다. 명령 수를 늘리는 작업은 아니다.
3. API-020의 지정 signer 접근 및 결과 읽기 조건을 기존 authorization/access mappings와 화면에 반영한다.
4. SS-R01/02/04의 source/증거/결과 관계를 물리 저장 제약·키 보관 경계에 연결한다.

현재 API 107개·BLE 명령 34개·기준 schema/권한/DDL은 변경하지 않았다. 5개 HTTP와 5개 BLE 메시지 쌍의 후보를 검토했으며, 실기·MPC·암호·네트워크 시험은 미실행이다. 복구 백업·MPC 구성·지갑 주소 관계도 이번에 확정하지 않는다.

다음 설계는 **개인·guest·환불 후보를 하나의 버전/권한 표로 대조하고 기준 명세에 병합할 범위를 결정하는 작업**이다. 새로운 기능을 추가하기보다 지금까지의 후보와 기본 계약이 충돌하는 지점을 닫는다.

## 9. 검토 사례와 확인 범위

후보 검증기는 24개 허용·11개 거절 형태와 8개 필드 일치 예제를 확인한다. 아래 16개는 실제 인증·암호·실기·동시성 검증 전에 준비한 기대 결과다.

| 사례 | 입력/사건 | 기대 결과 |
|---|---|---|
| SA-C01 선행 주소 증명 | payment.identify가 intent 생성 전에 동작 | attempt/challenge/단말/session/payer만 결합; 아직 없는 intent digest 요구 금지 |
| SA-C02 주소 증명 재사용 | identify proof를 최종 결제 승인으로 제출 | 최종 intent/context/서명 결과와 물리 승인 증거가 없으면 거절 |
| SA-C03 식별 입력 충돌 | API034 path attempt/session/payer와 ownershipEvidence binding이 다름 | 서버 원 challenge/단말/session과 대조해 거절 |
| SA-C04 기기 없는 proofRef | 키오스크에만 있는 proofRef 전달 | 필요한 inline packet/실기 검증자료 없으면 거절; 서버가 해당 로컬 참조를 조회한다고 가정 금지 |
| SA-C05 근접 정보 유실 | proximityRequired=true인데 관측 없음/낡음/다른 peer | 승인 차단; null 또는 과거 관측으로 우회 금지 |
| SA-C06 승인 전 revision 반환 | API037 승인 후 refund revision이 증가했는데 context는 요청 revision | 후조건 불일치로 응답 후보 거절/재조회; 승인완료 snapshot 사용 |
| SA-C07 역할 분리 | 업무 승인자 A와 실제 매장 signer 사용자 B가 다름 | 정확한 매장/선택 signer의 현재 권한과 승인 증거를 별도 확인; 동일 account 강제 또는 임의매장회원 허용 금지 |
| SA-C08 서명 수단 변경 | 승인 후 HW에서 Cloud 또는 signer epoch 변경 | 기존 context 재사용 거절; 미확정 서명 대사 후 새검토 |
| SA-C09 오직 EOA 서명 | 서명 주소는 같으나 기기 승인 증거 없음 | 특정 장치/세대/승인 사건으로 간주하지 않음 |
| SA-C10 기기 결과 끊김 | signed 결과가 생성됐을 수 있으나 BLE결과 유실 | 동일 검증세션 원review 조회; 새session에 과거result 자동공개/새결제 금지 |
| SA-C11 결과 조회 확대 | read_progress만 가진 capability로 includeSigningResult=true | 403/비공개거절; 반환필드선택으로 권한상승 금지 |
| SA-C12 서버에 없는 결과 | API020조회 시 HW 결과가 서버에 저장된 적 없음 | signingResult=null; 기기 결과 재취득 또는 signature_unknown 유지 |
| SA-C13 MPC 결과 다른원천 | 동일 wallet의 다른 refund/epoch operationId를 증거로 제출 | 원 intent/context/선택signer/epoch/정확한 bytes 대조 후 거절 |
| SA-C14 guest 재조회 | 앱계정 없이 유효 원attempt capability로 진행조회 | 허용된 원진행/snapshot만 반환; 일반 ownerBLE/앱로그인 강제 금지 |
| SA-C15 기존 환불hold | 승인 이후 funding guard가 보류로 변경 | API018 및 SS-T04 gate에서 새전송 차단, 이미노출된서명예약은 유지 |
| SA-C16 버전 자동 강등 | source후보가 거절되자 wallet/personal 또는 구버전으로 재시도 | 자동강등 거절; 운영/기기호환확인 후명시버전 사용 |

# 개인·키오스크·환불 승인 계약 — 버전·권한 비교와 병합 범위

2026-09-18 · **설계 병합 계획. 실제 병합·제품 구현·배포는 미수행.** [개인 지갑 API](wallet-api-contracts.md), [키오스크·환불 계약](source-approval-contracts.md), [결제·환불 원장 후보](commerce-contract-changes.md), [저장 경계](signing-submission-storage-design.md)를 대조했다.

**10개 기존 API와 5개 기존 BLE 명령을 하나의 승인 계약군으로 정리하되, 개인·guest·환불의 권한과 원천은 각각 유지한다.** 같은 경로를 공유하는 API-015/018/020을 느슨한 union으로 합치지 않는다. 원 요청에 저장된 source와 계약 버전으로 검증기를 선택한다.

[병합 대상·차이 원본](approval-contract-merge-plan.json) · [전체 설계 현황](../planning/design-integration-register.md)

후속 [공통 승인 스키마·권한 발급/전달 후보](approval-access-contracts.md)를 작성했다. 아래의 미작성 표기는 이 계획의 작성 당시 상태이며, 새 후보도 기준 카탈로그 병합과 실제 실행 검증은 대기 상태다.

## 1. 현재 버전과 병합 목표

| 계약군 | 현재 상태 | 의미 |
|---|---|---|
| 기준 카탈로그·core draft | 기존 설계, 실행 미검증 | API 107개·BLE 34개. 통합된 승인 proof/profile을 모두 갖춘 상태 아님 |
| wallet-approval-candidate-1 | 개인 지갑 후보 | 지갑 조회·개인 EOA 송금·선택/승인·제출·진행 조회 |
| source-approval-candidate-1 | guest/환불 후보 | source별 snapshot·기기/MPC 결과 증거·제한된 결과 조회 |
| commerce reconciliation 후보 | body/data/event fragment | 환불 원지급·funding revision·예약·보정. 독립된 전체 HTTP 버전으로 사용하지 않음 |
| approval-v1-draft | **이번 계획의 통합 목표 이름** | 세 승인 source를 명시한 후속 설계용 계약. 현재 schema나 서비스에 설치된 버전 아님 |

HTTP 계약 버전, BLE 제어 메시지 버전, 암호/증거 profile 버전은 각각 기록한다. HTTP 버전을 올렸다는 이유로 기기가 새 proof나 메시지를 해석할 수 있다고 보지 않는다. 기술 버전 선정은 후속 호환 표에 연결하며 Zephyr 기반만 확정 상태를 유지한다.

통합 버전은 형식/adapter 선택을 위한 것이지 권한 증명이 아니다. 클라이언트의 header·sourceKind가 서버에 저장된 원 intent의 요구를 낮추지 못한다. 미지원 버전·profile은 거절하고 새 거래나 구버전 제출로 자동 재시도하지 않는다.

## 2. 직접 병합할 API·BLE 범위

| API | 병합 목표 | 유지할 경계 |
|---|---|---|
| 007/008/009 | 개인/매장 WalletChoice·잔액·문맥·승인 수단 조회 | 현재 binding별 권한; 같은 주소 중복 합산 방지와 사적 기록 병합 금지 |
| 017 | personal_transfer EOA의 검토 snapshot 생성 | 주문/환불 sourceId를 받지 않음. 매장 환불·smart account 우회 금지 |
| 034 | guest 결제 identify 증거→PaymentSnapshot | 계정/Cloud quorum 요구 없음; attempt/device/session과 주문 견적 |
| 037 | 환불 revision+funding revision+매장 signer 선택 | 원지급·예약·hold·목적지·업무 승인; 승인 후 revision 반환 |
| 015 | 개인/매장 Cloud 서명 요청과 결과 연결 | 원천별 현재 서명권. NU 승인 경로에는 강제하지 않음 |
| 018 | 정확한 context·signed bytes·검증된 provenance 제출 | 저장된 source별 adapter, submit 권한, SS-T03/04 경계 |
| 019 | 기존 체인 실행·정규성·확정 조회 보존 | HTTP/서명/제출 성공과 체인 확정 분리 |
| 020 | 공통 진행/원 snapshot/제출 가능한 결과 조회 | 진행·snapshot·결과·제출 권한을 따로 검사; 일반 operation도 유지 |

BLE 대상은 payment.identify, payment.prepare, payment.result, wallet.sign.prepare, wallet.sign.result다. payment 계열은 guest PaymentSnapshot과 payment_terminal 역할을 유지한다. wallet.sign 계열에는 **개인 PersonalSnapshot과 환불 RefundSnapshot을 명시적인 variant로 모두 포함**해야 한다. 현재 source 후보의 환불 전용 타입으로 덮어쓰면 개인 송금이 사라지므로 그대로 채택하지 않는다.

API-030/035/036/038의 commerce 변경, API-032 challenge 발급·세션 설정, API-056 이후 스마트 계정/상품별 adapter는 연결 의존성으로 추적한다. 이를 직접 수정 대상 10개에 포함했다고 세거나, 이번 승인 계약군에 모든 15개 요구가 구현됐다고 표시하지 않는다. 로그인한 Cloud 지갑의 카페 지급도 NU guest variant와 같은 것으로 가정하지 않고 BASE-06의 기능별 signer 지원 표에서 별도로 추적한다.

## 3. 확인한 차이와 통합 설계 선택

| ID | 실제 차이 | 이번 계획에서 정한 처리 |
|---|---|---|
| AM-C01 | API-015의 approvalProof가 ProfileInput / EvidencePacket, 응답이 signing / progress | 원천별 증명 profile을 검증하는 공통 운반 타입과 `progress` 명칭으로 정리. bytes·data를 이름만 바꿔 변환하지 않음 |
| AM-C02 | 개인 API-018은 contextId 미포함·evidence 생략 가능, source는 필수 | 새 통합 승인 경로에는 contextId와 증거 필수. 원본 source/요구 버전 검증 후 처리; 약한 variant fallback 금지 |
| AM-C03 | 개인 API-020은 walletFlow/approvalSnapshot, source는 progress/snapshot/signingResult | 기존 Operation을 유지하고 `approval` 영역에 공통 projection을 넣는 목표안. 비승인 operation은 approval=null |
| AM-C04 | 개인 closed와 source blocked, 승인 문맥의 contextDigest·epoch 표현 차이 | closed와 blocked를 별도 상태로 유지. 공통 문맥 digest를 정의하고 서버 snapshot·검증 증거가 없는 필드는 생성하지 않음 |
| AM-C05 | wallet.sign의 개인 상세 payload 없음, source payload는 환불 전용 | 개인/환불 tagged union을 새로 정의. owner role 일치만으로 다른 source를 허용하지 않음 |
| AM-C06 | commerce API-037 body는 추가 필드 금지, source는 merchantSigner 필요 | 두 schema의 단순 allOf 합성 금지. revision 둘과 merchantSigner를 가진 새 strict body를 정의하고 양쪽 의미 검사를 보존 |
| AM-C07 | 버전 선택·멱등 재시도·기존 baseline 참조가 후보마다 다름 | 원 intent에 버전 고정, 공통 논리 멱등 경계 정의, 검토 당시 baseline과 후보 이력 보존 |
| AM-C08 | submit capability·결과 조회권의 검사 조건은 있으나 모든 발급/갱신 경로가 완성된 것은 아님 | 새 계약에서 자격의 발급자·수신자·대상·기한·재조회 경로까지 명시. Bearer 형식만으로 유효 권한을 가정하지 않음 |

이 표는 **병합 설계 방향을 정리한 것**이며 관련 API·schema를 변경한 결과가 아니다. 전체 D02/D03/D04/D08/D09 결정을 닫거나 사용자 정책을 대신 선택하지 않는다.

### 공통 승인 문맥의 목표 형태

공통 부분은 approvalContextId, contextDigest, intentId, 부모 operationId, sourceKind, chainId/payerAddress, requestKind, profileId, payloadDigest/reviewDigest, expiresAt이다. 별도 source variant에는 다음을 유지한다.

- personal_transfer: 개인 계정에서 도출한 wallet binding, 서버가 확인한 signer·세대·지갑/권한 revision.
- payment: 원 store/order/attempt·terminal/session·기기 참조/세대·identify 증거·근접 정책. 계정·wallet row는 필수 아님.
- merchant_refund: store/refund/payment allocation/reservation, 승인 후 refund/funding revision, 업무 승인 참조와 선택된 실제 매장 signer.

입력의 expectedRevision들은 낙관적 동시성 검사용이다. 저장된 승인 snapshot에는 **검증·commit된 값**임을 명시하고 예상값을 그대로 신뢰하지 않는다. bindingId/walletBindingId 같은 명칭은 공통 내부 개념에 매핑하되 개인·매장 권한 범위를 합치지 않는다. deviceEpoch와 participantEpoch도 각각의 타입을 유지한다.

공통 SigningResult는 intentId/approvalContextId/signedPayload와 device_result 또는 mpc_session 증거를 갖는 방향으로 정리한다. 구버전의 원시 EOA 서명에 기기·MPC 세대 증거가 없으면 이를 새 provenance로 승격하지 않는다. 보유한 실제 검증 증거로 변환 가능한 경우만 기록된 adapter가 변환하며, 그렇지 않으면 새 승인 진행을 막고 기존 거래 관측을 유지한다.

### API-020 응답의 목표 형태

```text
data.operation = 기존 Operation
data.approval = null | {
  sourceKind,
  progress,
  snapshot: null | PersonalSnapshot | PaymentSnapshot | RefundSnapshot,
  signingResult: null | SigningResult
}
```

진행 상태는 review_ready/signing/signature_unknown/signed/submission_unknown/submitted/blocked/closed를 구분하는 안이다. blocked는 현 정책상 중단이며 closed는 작업 종료 표현이다. 서로 자동 치환하거나 둘 중 하나를 유효 서명의 온체인 취소로 해석하지 않는다. 일반 녹음·AI·FOTA 등 operation의 결과 공개는 기존 보호 결과 정책을 유지한다.

개인 Cloud 결과 복구도 이 계약에 맞는 실제 결과 조회 경로를 추가해야 한다. source의 signingResult 필드를 복사했다고 개인 Cloud 결과 접근이 완성된 것이 아니다. 부모 wallet/source operation과 자식 MPC operation의 연결, 결과 보유 여부, 현재 권한을 확인해야 한다.

## 4. 권한 비교표

| 행위 | 개인 송금 | guest NU 결제 | 점주 환불 |
|---|---|---|---|
| 검토 생성 | 본인 계정+현재 개인 wallet binding | 원 attempt/device/session capability | 해당 매장 업무 승인권+원 refund/예약/funding |
| 실제 HW 승인 | 현재 owner 세션+선택 NU+물리 승인 | payment_terminal 세션의 해당 결제+NU 물리 승인 | 현재 owner 세션+선택 매장 HW signer+물리 승인 |
| 실제 MPC 승인 | 본인 선택 Cloud signer의 현재 참여자 정책 | 이 NU guest 경로에는 없음 | 선택된 매장 Cloud signer의 현재 사용자/참여자 정책 |
| 진행/snapshot 조회 | 원 operation·source의 현재 조회권 | 해당 attempt/session에 제한된 현재 조회권 | 업무 승인자 또는 지정 signer의 정확한 source 조회 조건 |
| signed result 조회 | 해당 개인 승인 결과의 별도 권한 | 명시적으로 부여된 원 결과 접근 범위만 | 지정 signer/허용 중계자 등 정확한 결과 접근 조건; 일반 매장 회원은 부족 |
| 플랫폼 제출 | 해당 intent의 검증된 submit 권한 | 원 attempt/source의 exact-submit capability | 원 refund·현재 signer/권한·예약/hold와 exact-submit 권한 |

`includeSigningResult=true`는 결과 요청일 뿐 권한 부여가 아니다. 이미 전달된 signed bytes는 외부에서 제출될 수 있으므로 이후 플랫폼 조회/제출권 철회로 해당 bytes의 효력이 사라졌다고 표시하지 않는다.

업무 승인자 A와 실제 signer B가 다를 수 있다. API-020의 기존 operation owner 조건을 전 매장 회원으로 넓히지 않고, **선택된 signer와 그 source에 한정된 현재 권한 조건**을 추가한다. 고객의 환불 조회는 API-038의 원 allocation 범위를 유지하고 매장 서명 결과 조회로 연결하지 않는다.

### 권한 자격의 발급·갱신도 병합 대상

API-018은 intent_submit_capability를 요구하지만 후보의 Authorization 문자열 모양은 자격 발급 사실을 보장하지 않는다. 통합 설계에는 다음을 추가해야 한다.

1. 개인/guest/refund 각각에서 누구의 어떤 검증 결과로 read/submit 자격을 발급하는지 지정한다. API-017/034/037의 검토 생성만으로 무제한 서명·제출권을 발급하지 않는다.
2. 원 intent/source, audience, 행위, 기한, 필요한 sender 결합과 현재 철회 상태를 검사한다. 업무 승인자에게 지정 signer B의 서명권을 자동 전달하지 않는다.
3. credential은 실제 수신 가능한 보호 응답/연결로 전달한다. 서버가 해석할 수 없는 로컬 capabilityRef만 응답해 호출 가능으로 표시하지 않는다. 실제 필드/발급 profile은 다음 공통 schema 작업의 명시적 산출물이다.
4. 만료 뒤 원 결과 조회·재인증 경로와 새 승인 권한을 구분한다. guest 복구에 앱 계정을 필수로 추가하거나 만료 자격으로 새 서명을 허용하지 않는다.

발급·갱신 경계가 설계되지 않은 조합은 배포 준비 완료가 아니다. 이 항목은 새로운 사용자 정책 질문이 아니라 기존 권한 요구를 호출 경로까지 연결하는 기술 설계다.

## 5. 버전·source에 따른 라우팅과 재시도

| 입력 상황 | 처리 기준 |
|---|---|
| 새 검토 생성 | 지원하는 명시적 버전과 API별 허용 source로만 생성, 요구 버전을 원 intent에 저장 |
| 서명/제출 요청 | 저장된 source·버전·profile과 요청을 대조한 후 adapter 선택. shape가 우연히 맞는 다른 source로 전달 금지 |
| 개인 구형 envelope로 새 guest/refund 제출 | 거절. evidence 생략 가능 schema를 우회로 사용하지 않음 |
| 같은 논리 요청을 다른 header 버전으로 재시도 | 기존 멱등 기록·원 intent로 대사. 버전 변경만으로 새 지갑·승인·송금 작업을 만들지 않음 |
| 이미 수락된 같은 intent/bytes의 응답 유실 | 현재 결과 조회권으로 원결과 반환; payload/승인 조건을 바꿔 새 dispatch 생성 금지 |
| 구형 기록에 새 proof/epoch 정보가 없음 | 없는 증거를 만들지 않음. 원 결과 관측/허용된 조회와 새 승인 가능 여부 분리 |
| 요청 버전은 지원하지만 BLE/proof profile 미지원 | 해당 실행 거절; 호환되지 않는 기기에서 blind signing 또는 구버전 자동 전환 금지 |

멱등 키 범위는 actor/source의 권한 문맥·논리 행위·key를 기준으로 하며 HTTP 버전만 달리해 중복 효과를 만들지 않는다. 서로 다른 버전 입력은 검증된 adapter가 논리 입력을 정규화할 수 있을 때만 같은 요청으로 간주한다. 같은 키의 실제 내용이 달라지거나 변환에 필요한 검증값이 없으면 충돌이다. 구형 데이터를 새 버전으로 덮어써 멱등 의미를 바꾸지 않는다.

## 6. 설계 파일 병합 묶음과 완료 조건

| 묶음 | 함께 반영할 파일/범위 | 완료로 확인할 것 |
|---|---|---|
| AM-B01 공통 타입·버전 | 공통 승인 schema, critical/extended DTO와 catalog, 개인·guest·환불 variant, credential 전달 계약 | 10개 API 요청/응답과 3 source의 교차 연결, 지원하지 않는 variant 거절 |
| AM-B02 권한·조회 | authorization-policies, api-access-transactions, 역할별 API-020 응답, common-flows | 실제 signer와 승인자 분리, 4개 조회/제출 권한 구분, 일반 operation 접근 보존 |
| AM-B03 BLE·화면 | BLE catalog/core envelope, 개인/guest/환불 prepare/result, U02/U03/K03/K05/D01 등 실제 참조 화면 | 명령 권한·snapshot·증거 운반이 API와 일치; 개인 HW 경로 누락 없음 |
| AM-B04 저장·원장 | SS-R/SS-T와 기존 TX-04/05/13, 참조 DDL/관계·호환 매핑, commerce API-037 합성 | 원 source·서명 증거·결과·permit 연결과 환불 예약/보정; SQL 제약과 서비스 책임 구분 |
| AM-B05 회귀·이력 | 생성 문서·예제·검증기·compatibility·추적 metadata, 후보 baseline 보관 | 전체 107 API/34 BLE/37 화면 참조 보존, 기존 unrelated 상품/operation 검증도 통과 |

파일명은 기준 문서의 실제 위치를 [원본 JSON](approval-contract-merge-plan.json)에 연결했다. AM-B01~03은 같은 계약 변경의 일부이므로 중간 파일 하나만 반영한 상태를 통합 완료로 표시하지 않는다. AM-B04의 실제 DB 실행 시험은 구현 단계이며, 지금은 관계·제약의 설계까지 다룬다.

후속 설계 작업은 AM-B01의 **공통 schema와 credential 전달 계약을 먼저 작성하고**, B02/B03을 함께 대조하는 것이다. 암호 profile을 임의로 확정할 필요 없이 미선정 verifier 경계를 명시할 수 있다. 다만 profile이 미선정인 채로 실기 호환·보안·배포 준비 완료를 선언하지 않는다.

기존 후보 validator는 작성 시점 baseline 해시를 고정하고 있다. 병합 후 오류를 없애려고 해시만 최신 값으로 바꾸지 않는다. 검토 당시 baseline을 별도 보관해 후보를 재현하거나, 후보를 superseded로 표시하고 검증 책임을 통합 명세로 이전하는 절차를 기록한다. 무엇을 검증하는지와 원본 이력 없이 이전 테스트 통과를 재사용하지 않는다.

## 7. 이번에 정리된 것과 남은 것

이번 작업으로 API/BLE 영향 범위, 후보 간 차이 8개, 원천별 권한, 버전 라우팅·이력 보존, 설계 병합 묶음 5개를 정리했다. **새 통합 schema와 권한 파일을 실제로 병합한 것은 아니다.** 특히 개인 BLE 상세 variant, 개인 Cloud 결과 전달, credential 발급·갱신의 실제 타입은 다음 산출물에 포함해야 한다.

12주 전체 범위와 104개 작업·320개 세부 작업은 유지한다. HW/Cloud 별도 주소·MPC 참여자 구성·늦은 자산 복구 백업은 이번 기술 정리로 확정하지 않는다. RR-DEC-01은 답변 대기이며 필요한 복구 확인이 없는 새 초기화 허가는 허용하지 않는다.

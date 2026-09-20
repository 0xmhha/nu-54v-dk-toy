# 핵심 API 상세 요청·응답 v0.1

후속 [책임·상태 전이 설계](commerce-lifecycle-design.md)는 다중 지급·환불 재관측·초기화 후 복구의 상세 제안을 담는다. 해당 변경은 아직 현재 DTO에 반영하지 않았으며 LC-GAP-01~05로 추적한다.

[화면별 동작](screen-flows.md) · [API 공통 규칙](interface-contracts.md) · [타입 원본](critical-dtos.schema.json) · [계약/예제](critical-dtos.json)

107개 API 경계 중 **주문·결제·환불·반납 8개 API**를 이 문서에서 다룬다. 나머지는 [99개 API 상세 타입](extended-contracts.md)으로 연결했다. 이 구분은 구현 완료나 기능 범위 제외를 의미하지 않는다. 모든 예제는 합성값이다. 주소·proof·서명 bytes·환율은 실제 사용 가능한 값이 아니며 서버에 전송하지 않는다.

## 1. 적용 범위와 공통 타입

- 요청 타입은 `path`, `headers`, `body`로 나눴다. `headers.requestId/idempotencyKey`는 논리 이름이며 실제 HTTP에서는 공통 규칙의 header 이름으로 매핑한다. 인증/capability header는 보안 채널에서 별도 주입하며 예제에 넣지 않는다.
- 객체는 정의한 필드만 받는다. 니모닉/private key·클라이언트 계산 최종 가격처럼 받으면 안 되는 필드가 섞이면 거절한다.
- `ID`는 길이 제한 문자열, `revision`은 0 이상의 정수, 금액은 기본 단위 정수 문자열, 주소는 20바이트 hex 형태다. 정규화·체인·권한·상한·실제 자산 검사는 별도다.
- `ProofReference`는 `proofId, profileId`를 가진다. 서버/기기에서 이미 확보해 검증할 수 있는 증거만 참조한다. 클라이언트가 임의 ID를 만드는 것만으로 증명되지 않는다. 필요한 검증 프로필과 proof 취득 동작은 상세 설계에서 확정한다.
- 같은 요청 key의 다른 내용은 409다. 재전송으로 이전 결과를 반환할 때에도 현재 조회 권한을 검사한다. 새 견적/새 자금 이동은 새 요청 의도로 다시 검토한다.
- 아래 EOA intent 타입은 일반 ERC20 거래 경로다. 스마트 계정·typed data·DID·x402의 proof를 EOA raw transaction으로 강제 변환하지 않는다.

## 2. 화면과 API 계약

| API / 연결 화면 | 필수 요청 body | 정상 응답 | 서버 의미 검사 |
|---|---|---|---|
| API-029 주문 생성 / K02 | items[{itemId, optionIds, quantity}], menuRevision | 201: order, orderCapability | 서버 메뉴·품절·가격을 검증해 snapshot; 수취 주소도 서버 설정에서 고정 |
| API-032 결제 시도 / K03 | assetId, accountMode | 201: attempt, quote, pairingChallenge | payable 주문·자산 지원·최신 가격 규칙; chain/수취/금액은 서버 생성 |
| API-034 기기 결제 intent / K03·D01 | signerAddress, ownershipProof, sessionId | 201: intent, review | payer proof와 device/terminal/attempt·challenge 결합; 견적/세션 기한 이하로 intent 만료 |
| API-018 서명 거래 제출 / U03·K03·K05 | intentId, signedPayload | 202: operation, transactionRef | 서명 복원·체인·nonce·가스·호출·금액을 고정 intent와 비교; EOA 예제만 상세화 |
| API-036 환불 요청 / K05 | amount{assetId, atomicAmount}, destinationProof, reason | 201: refund, reservedAmount | 원지급 자산·양수 금액·목적지 증명; 원자적 한도 예약 |
| API-037 환불 업무 승인 / K05 | expectedRevision | 200: refund, signingIntent | 최신 revision·자금 승인 권한·예약 유효성; 매장 signer의 실제 서명은 다음 단계 |
| API-046 반납 검사 / U24·O01 | walletOrigin, externalAccessProof 또는 null | 200: checklist, conditionalResetClearance 또는 null | 서버에 기록된 출처와 대조; 미확정/회수/외부 접근/자격 정리 검사 |
| API-047 반납 완료 / U24 | checklistRevision, clearanceId, deviceResetEvidence, ownerApprovalRef | 200: rental, bindingRevoked=true | 실제 device/rental/binding/challenge 결합과 초기화 증거·소유자 승인; 최신 점검 완료 |

API-046의 200은 **검사 결과를 읽었다는 뜻**이다. `pending`이면 clearance는 null이고 초기화 버튼은 비활성이다. `eligible_for_reset`일 때만 clearance가 존재한다. 필수 점검은 비어 있을 수 없고 모든 항목이 passed/not_applicable이어야 한다. 서버는 정책별 필수 점검 목록을 생성하며 클라이언트의 축약 목록을 신뢰하지 않는다. `not_applicable` 역시 지갑 출처에 맞는 서버 정책 판정이다.

## 3. 주문·결제 예제 읽는 순서

`critical-dtos.json`의 API-029 → API-032 → API-034 → API-018 request/response를 연결해 읽는다.

```text
K02: 메뉴 revision 3의 coffee-demo 1개를 주문
  → order-demo (awaiting_payment), 서버 가격 snapshot
K03: order-demo에 dummy-usdc / EOA 결제 시도
  → attempt-demo, quote-demo, pairing challenge
NU: 가게/대상 확인 → payment.identify → 선택 주소 증명
K03: 증명과 session-demo를 intent 생성에 전달
  → intent-demo; quote·session보다 늦지 않은 승인 기한
NU: 실제 payload 검토 → 물리적 승인 → 서명
K03: intent-demo에 결합된 signed payload 제출
  → op-submit, tx-demo, state=unknown이면 조회를 이어감
Indexer: 실제 receipt/log/지급 주체 대조
  → acceptance 변경; 그때 주문·영수증·매출·혜택 갱신
```

예제의 `signedPayload=0x00`은 필드 형태를 검증하기 위한 값으로 실제 EVM 거래가 아니다. 202/unknown 응답 예제는 결제 성공 증거가 아니다. 실제 시스템은 같은 intent의 허용된 signed payload인지 검사한 뒤 접수해야 한다.

intent의 기한은 **서명 승인 요청을 받아들일 기한**이다. 그 시간이 지났다고 이미 서명/제출한 토큰 전송이 체인에서 취소되는 것은 아니다. 견적보다 늦은 intent는 승인 전에 거절하고, 이미 제출한 거래는 별도 추적한다.

## 4. 공통 서명 화면 F-SIGN

[공통 흐름 원본](common-flows.json)을 U03·U13~U19·K05가 재사용한다. 화면 레이아웃을 공유해도 승인 내용을 단순 송금으로 바꾸지 않는다.

| 출처 | 확인 내용 | 승인·제출 경로 |
|---|---|---|
| 개인 송금 | 지갑·체인·수취·금액·가스 | 선택 HW의 wallet.sign 또는 MPC signing-session → EOA 제출 |
| DeFi/FX/Perp/STO | 상품 행위·대상 계약·금액·기한·한도·권한 변화 | 지원되는 signer profile → 상품 실행 어댑터 |
| 스마트 계정 | account·실행 call·signer·가스·권한 | 선택한 UserOperation/계정 모델 어댑터 |
| DID/x402 | 요청자·목적·공개 내용 또는 자원·과금·증명 범위 | 선택 표준의 proof adapter; 항상 raw tx가 되는 것은 아님 |
| 점주 환불 | 원주문·refund ID·목적지·환불액·매장 가스 | **매장에 권한 있는 signer** → 별도 환불 거래 |

K05는 고객의 payment.prepare를 호출하지 않는다. 점주 signer가 모바일 HW/Cloud에 있으면 보호된 refund intent 참조로 이어서 승인하고 키오스크는 refund 상태를 조회한다. 점주가 로그인한 사실만으로 서명을 생성하지 않는다. 이 연결의 구체 handoff 방식은 점주 지갑 선택에 맞춰 구현한다.

`EoaRefundIntent`는 `attemptId` 대신 `refundId`로 원천을 연결한다. 원지급의 승인/서명을 재사용하지 않는다. 같은 컴포넌트를 사용하더라도 source kind와 승인 지갑을 바꾸지 못하게 고정한다. 지원하지 않는 서명 형식은 거절하고 무조건 서명하는 우회 경로를 두지 않는다.

## 5. 환불 예제와 복구

1. 원지급에서 2,000,000 기본 단위를 환불 요청한다. 서버가 목적지 증명과 한도를 확인하고 예약한다.
2. 자금 승인자가 refund revision 1을 승인한다. authorized revision 2와 **refund 전용 intent**가 반환된다.
3. 점주 signer가 intent를 승인하고 별도 지급을 제출한다. authorized는 체인 지급 완료가 아니다.
4. 앱/키오스크 응답이 끊기면 refund ID로 조회한다. 제출 불명확 상태의 예약을 자동 해제해 추가 환불을 가능하게 만들지 않는다.
5. 별도 지급이 확정되면 환불 완료액과 예약을 원자적으로 갱신하고 혜택/정산 보정을 발행한다.

한도 조건: `완료 환불액 + 유효 예약액 + 새 요청액 ≤ 원지급에서 정책상 환불 가능한 금액`. 동시 요청·같은 key 재시도·다른 목적지로 바꾼 재시도를 별도로 검증한다. schema는 금액 형식을 검사하고, 실제 한도/자산/목적지 진위는 서버 의미 검사로 처리한다.

## 6. 반납 예제와 복구

- `API-046-response`: import 지갑의 외부 접근이 아직 확인되지 않아 pending이며 clearance가 없다.
- `reset-eligible-response`: 외부 접근 및 서버의 해당 정책 점검이 완료된 상태의 형태 예제다. 실제로는 필요한 모든 점검 항목을 반환해야 한다.
- 기기는 유효한 clearance와 사용자 물리적 확인 후 초기화한다. 지갑 키와 별도의 장치 신원/초기화 증명 경로를 검증한다.
- `API-047-request`: clearance/checklist/device 증거와 ownerApprovalRef를 제출한다. 서버는 path rentalId와 증거의 rental/device/binding이 모두 일치하는지 검사한다.
- `API-047-response`: 검사·증거 검증·binding 철회가 끝난 뒤에만 returned와 bindingRevoked=true를 반환한다.

초기화 후 앱이 종료되면 rental 조회와 실기 미설정 상태를 다시 확인한다. 증거를 재취득하는 방법과 장치 신원 키 보호는 상세 firmware 설계 항목이다. 운영자 수동 체크만으로 물리적 삭제가 증명됐다고 표시하지 않는다.

## 7. 검사 범위

`python3 content/specifications/validate_specs.py`는 화면/API/BLE/작업 참조, 공통 signer 연결, JSON schema, 합성 요청·응답의 허용/거절, intent 기한 관계를 검사한다. 실제 제공자 인증·서명 진위·가스·온체인 실행·기기 초기화의 검증은 포함하지 않는다.

보완된 화면의 조회/재실행을 위해 기기/대여·매장 주문/정산·기록 메타데이터·코스/챌린지·여행·STO 보유 조회와 기기 지갑 binding API를 추가했다. 이 조회는 다시 실행한 앱이 화면 캐시 대신 원천 상태를 읽는 연결이다.

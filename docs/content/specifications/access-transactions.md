# API 권한·읽기/쓰기·서비스 트랜잭션

현재 승인 경로는 [승인 기준 통합](approval-baseline.md)을 따른다. 물리 SQL·실행 검증과 미선택 정책은 별도다.


110개 API를 [읽기/쓰기 표](api-access-transactions.md)와 [60개 권한 정책](authorization-policies.json)에 연결했다. 전체 기능·앱 연동·12주 완료 전제는 유지하며 역할/공수는 배정하지 않는다. 여기의 사용자/점주/운영 **서비스 역할**은 개발자 3명의 업무 배정이 아니다.

이 문서는 구현 계약이다. 인증 middleware, 실제 권한 엔진, DB GRANT/RLS, worker를 구현·배포한 결과가 아니다. 제안된 점주 역할의 기본 권한은 제품 설정으로 확정할 대상이다.

## 1. 요청 처리 순서

```text
인증된 principal 또는 검증된 capability 해석
  → endpoint 행위와 대상 resource 조회
  → 현재 소유/소속/범위/기한/철회 상태 검사
  → 입력 타입 + profile + 업무 상태 검사
  → 멱등 key 확인
  → 짧은 DB transaction: 최신 권한/revision 재검사 + 원장 변경 + outbox
  → 외부 작업 실행/위임
  → 결과 검증 후 별도 transaction으로 상태 반영
```

인증 전 요청의 accountId/storeId/role은 권한 근거가 아니다. 자원 ID를 알아도 소유 권한이 생기지 않는다. 로그인·지갑 연결·업무 승인·실제 서명은 서로 다른 검사다. 비동기 작업도 enqueue 시점과 실제 변경 직전에 필요한 권한·동의·상태를 확인한다.

멱등 key가 일치하더라도 현재 조회 권한이 없어졌으면 과거 민감 응답을 다시 제공하지 않는다. `READ`는 도메인 원장을 수정하지 않는다는 의미이며 안전한 접근 감사는 별도 기록할 수 있다.

## 2. 정책을 해석하는 기준

| 주체 | 신뢰 근거 | 허용 범위 |
|---|---|---|
| 사용자 계정 | 서버 세션과 활성 account, 필요한 최근 인증 | 본인 자료와 활성 wallet/device binding |
| 점주 계정 | 서버 계정 + 요청 store의 활성 membership | 해당 store의 행위별 권한 |
| 고객 모드 키오스크 | 서버 발급 terminal/store 세션 | 메뉴·주문과 해당 고객의 제한된 결제 요청 |
| 임시 기기 결제 세션 | 검증된 device/terminal/attempt/challenge | 선택 지급 주소 확인·검토·승인 결과·해당 요청 종료 |
| 운영자 | 별도 운영 권한 registry와 업무 범위 | 메타데이터·예외 조회·재처리 요청; 자금 서명 제외 |
| issuer/verifier | 등록된 profile·issuer/verifier 신뢰 registry | 자격 발급/검증 목적과 대상 범위 |
| worker | 서비스 주체·검증된 job/source binding | 명시된 원장 변경만; 클라이언트 임의 관리자 역할 금지 |

### 점주 역할의 작업용 기본값

| 역할 | 기본 행위 제안 | 별도 허용하지 않는 행위 |
|---|---|---|
| owner | 매장/단말/메뉴/소속 관리, 매출, 환불 요청·업무 승인, 정산 | 고객 키·녹음·정밀 위치, 운영 릴리스 관리 |
| manager | 메뉴/단말·주문·매출·환불 요청·정산 | 수취 주소 변경·환불 자금 승인·소속 owner 변경 |
| menu_editor | 메뉴/가격/품절 | 매출/환불/지갑 자금 |
| sales_viewer | 주문/매출·정산 읽기·허용 매장 지갑 자산 조회 | 메뉴·자금·마감 쓰기 |
| refund_requester | 주문 조회·환불 요청 | 환불 승인/실제 서명 |

표는 서버 정책의 초안이다. owner도 연결된 signer의 실제 사용자 확인·MPC 정책을 통과해야 자금을 전송한다. 운영자 권한과 issuer 권한을 store owner 역할로 겸하지 않는다.

## 3. 관계 조회와 응답 projection

| 자원 | 검사할 관계 | 응답에서 제외할 것 |
|---|---|---|
| 개인 지갑 | session.account → active wallet_bindings.account → wallet | private key, MPC share, 다른 binding의 개인정보 |
| 매장 지갑 | session.account → active memberships(store) → active store wallet binding | 자산 조회와 무관한 signer/복구 자료 |
| 주문 | order.account 본인 또는 해당 store의 주문 역할 또는 정확한 order capability | guest 경로의 사용자 프로필·다른 주문·관리 이력 |
| 거래 | transaction → intent.source → order/wallet/refund → 해당 권한 | hash만 아는 사람에게 private 원장 관계 공개 금지 |
| 환불 | refund → order.store 또는 검증된 고객 영수증 연결 | 고객에게 매장 signer 설정/운영 메모 공개 금지 |
| 기기/대여 | 현 binding.account 또는 rental.account; 운영자는 별도 projection | 운영자 응답의 owner capability·민감 proof |
| 녹음/후기/여행 | recording/review/trip/itinerary/participation의 owner | 다른 사용자 원음·위치·후기 비공개 내용 |
| 작업 상태 | operation.principal_scope 또는 그 operation에 묶인 capability | 추측한 operationId로 다른 사용자의 결과 참조 획득 금지 |

목록도 각 행에 같은 검사를 적용한다. cursor는 store/account/필터에 결합하고 다른 범위에서 재사용할 수 없게 한다. 여러 권한 경로가 있는 endpoint는 ‘아무 경로로든 읽을 수 있음’과 ‘관리자 전체 응답을 읽음’을 구분한다.

API-057 시장 견적은 지갑을 선택하지 않는 계정 권한 요청이다. 실제 자금 사용 권한은 API-058 intent 생성 때 선택 지갑과 현재 소속을 확인한다. API-070 유료 자원 요청은 walletId를 받으므로 생성 시점부터 해당 지갑 사용 권한을 확인한다.

## 4. 임시 결제 권한의 범위와 수명

capability는 선택한 검증 방식으로 issuer/audience·scope·resource·principal/terminal/device binding·기한·철회 상태를 확인한다. 해당 값이 있다는 것만으로 신뢰하지 않는다. 원문 bearer token은 일반 DB 원장·감사·화면 navigation 상태에 저장하지 않는다.

| 권한 | 결합 대상 | 허용 행위 |
|---|---|---|
| order | store, terminal, order | 해당 주문 조회·허용 시 취소/결제 시도 생성 |
| attempt | order, attempt, terminal | 기기 페어링 및 해당 시도 조회 |
| device payment | attempt, terminal, device, session, payer proof | intent 생성·기기 검토·해당 결과 조회 |
| submit | intent, source kind/id, signer digest | 검증된 정확한 signed payload 제출 |
| operation read | operation, 원래 principal/source | 해당 작업의 진행/안전한 결과 조회 |

write capability 종료와 이미 제출한 지급 추적은 분리한다. 종료된 결제 권한으로 새 서명·제출을 허용하지 않지만 주문의 안전한 결과는 현재 허용된 조회 경로로 확인할 수 있다. 재실행/만료 뒤 재인증·조회 권한 재발급은 현재 소유/단말 증명으로 검사하고 과거 key만으로 재발급하지 않는다.

카페 결제는 고객 폰 세션 없이 기기·키오스크 증명으로 진행한다. 이후 개인 앱 영수증 연결은 **주소를 입력했다는 이유만으로 허용하지 않는다.** 해당 주문/지급과 연결한 서명 소유 증명 및 계정 연결 절차가 필요하다. 아직 별도 연결 API/보관 방식이 확정되지 않았으므로 아래 GAP-04로 추적한다.

## 5. 외부 호출과 transaction 경계

읽기/쓰기 표의 TX-01~16은 [저장 작업](storage-operations.md)의 관련 원장 묶음이다. 일반 intent 생성과 제출을 원장 수락 transaction으로 오인하지 않도록 다음 단계를 별도 이름으로 두었다.

| 단계 | DB transaction 안 | 밖에서 수행 |
|---|---|---|
| STORE_CONFIGURATION | store 잠금·소속/최근 인증 재검사·설정/감사 | 주소 소유 proof의 선택 프로토콜 검증 준비 |
| QUOTE_CREATION | 원주문/시장 설정 revision 검사·quote snapshot 저장 | 실제 가격/pool/가스 조회 |
| INTENT_CREATION | 검증된 source/quote·권한·만료와 digest를 고정 | signer 호출 전 파싱·지원 profile 검증 |
| REQUEST_SUBMISSION | intent와 signed payload 일치 검사 결과·tx hash/제출 의도 저장 | 체인 전송·receipt/UserOperation 조회 |
| TX-04 지급 수락 | evidence·observation·attempt 귀속 검증 결과를 원장/outbox에 반영 | Indexer/RPC 원천 확인 |
| TX-05 환불 | 동일 balance의 한도 예약/소진·refund 상태·outbox | 실제 점주 signer 승인·별도 지급 |
| TX-07 반납 | 현재 binding/checklist/clearance 검사와 종료 반영 | 자산 회수·실기 초기화/증거 획득 |
| TX-09 개인정보/AI | 동의·접근 차단·job/삭제 상태 | 저장소/전사/AI 제공자 작업 |
| SECURITY_STORE | 선택 보안 저장소의 원자 소비/철회/회전 | peer proof 검증·세션 보호 |

DB 잠금을 유지한 채 버튼 입력·BLE·MPC round·RPC/AI 응답을 기다리지 않는다. 외부 결과가 돌아왔을 때 source revision·동의·기한을 다시 검사한다. 방송된 거래의 상태 불명확은 failed나 새 결제 요청으로 바꾸지 않는다.

위 기한 재검사는 새 권한 행사·결과 공개 조건이다. 이미 방송된 거래의 관측 증거를 권한 만료만으로 폐기하지 않는다. 유효한 worker는 원지급 귀속과 체인 상태를 계속 대사하며, 사용자 결과 조회 권한은 별도로 확인한다. [상세 수명·복구 설계](security-integration-design.md)를 따른다.

### 일관된 잠금 순서 제안

- 소속/수취 설정: store → membership/configuration.
- 환불: order → payment allocation → refund balance → refund → reservation.
- 반납: device → rental → device binding → return check → reset clearance.
- 혜택: 해당 grant/잔액 → redemption → correction/outbox.
- 여러 행이면 같은 종류 안에서 정렬된 ID 순서로 잠근다. 충돌/직렬화 재시도는 원래 멱등 key를 유지한다.

같은 저장 명령을 수행하는 모든 API/worker가 순서를 공유해야 한다. READ COMMITTED와 row lock 또는 선택한 동등한 방식의 **실제 다중 세션 검증은 구현 단계에 남아 있다.** 기존 single-user DDL 시험을 경쟁 조건 검증으로 대체하지 않는다.

## 6. 비동기 worker의 최소 쓰기 범위

| worker | 검증 입력 | 변경 가능한 도메인 결과 | 금지 |
|---|---|---|---|
| 체인 관측 | 지정 RPC·chain/tx/log·블록 revision | payment_evidence, chain_observations, chain_projections | 클라이언트의 성공 claim을 관측으로 채택 |
| 결제 대사 | intent/payer/receipt/canonicality/정책 | payment_allocations, orders, outbox | 운영 API의 임의 accepted 입력 |
| 환불 추적 | 원refund와 실제 별도 지급 | refunds, reservations/balances, outbox | unknown 상태에서 한도 자동 해제 |
| 혜택/여행 보정 | 검증된 acceptance/refund/evidence revision | benefit_entries, projections, challenge_entries | 재전송으로 중복 적립·보상 |
| MPC 생성/복구 | 계정·profile·참여자/복구 증명 | wallets, wallet_bindings, mpc_participants, signing_sessions, operations | 업무 DB에 키 조각/전체키 복사 |
| 기록/AI | owner·source·동의 revision | processing_jobs, result references, data_lineage | 철회 후 새 결과 공개 |
| 여행 추천 | 검증 가능한 장소/후기/결제 입력 | itineraries와 검증 결과 | 생성 결과를 근거 없는 실제 구매로 표시 |
| 개인자료 삭제 | 승인된 privacy request·scope | 접근 차단·저장소별 삭제 상태·계보 보정 | 계정 삭제를 자산 전송/고객키 임의 파기로 해석 |
| keeper | 시장 profile·가격·작업 고유키 | keeper_jobs, transaction_intents/submissions, operations | 사용자 결제 gas 정책을 임의 후원 모드로 전환 |

표는 허용된 업무 책임이다. 실제 SQL GRANT는 API/worker의 배포 단위·role·저장소 결정 뒤 작성한다. 서비스 주체에도 요청의 source/account/store 범위 검사를 유지한다.

## 7. 연결 중 드러난 구현 입력

기존 61개 참조 테이블에 없는 보안 상태를 임의 JSON 컬럼에 넣었다고 처리하지 않는다. 기존 작업에 다음 하위 설계 결과를 연결한다. 이 항목들은 새 제품 범위를 추가하는 것이 아니다.

| ID | 필요한 결과 | 관련 작업 |
|---|---|---|
| GAP-01 | OAuth flow·PKCE binding·one-time 소비·refresh 계보를 담당할 보호 저장소와 만료/철회 adapter | AUTH-02~04, BASE-05 |
| GAP-02 | terminal/order/attempt/submit/operation capability의 발급·검증·재인증/갱신·철회 저장 방식 | HW-03, SHOP-01/03, BASE-03 |
| GAP-03 | 운영자/issuer/verifier의 신뢰 registry·변경 승인·감사와 실제 service role 배포 | OPS-01, DID-01/02, BASE-05 |
| GAP-04 | 폰 없이 결제한 주문을 이후 로그인 계정의 영수증에 연결하는 증명/중복 연결 방지 API | APP-04, PAY-03/04, AUTH-01 |
| GAP-05 | 보호된 quote/transition plan/result 참조 저장소·owner ACL·보관 수명. operations.result_ref만으로 내용을 저장한 것으로 보지 않음 | SMART-01/03, APP-03, BASE-05 |

GAP 상태가 미정인 경로는 권한 검사 생략으로 임시 통과시키지 않는다. 선택 adapter가 준비되기 전에는 그 경로의 검증 실패를 명확히 반환한다. 전체 12주 범위 안에서 이 입력을 확정한다.

[미결 설계 입력 원본](security-integration-inputs.json)은 각 GAP의 산출물과 기존 작업 ID를 유지한다. 작업 목록의 `securityIntegrationInputs`에서 같은 연결을 확인할 수 있다.

5개 입력의 흐름·저장 책임·완료 조건은 [보안 연결 상세 설계](security-integration-design.md)로 구체화했다. 현재 상태는 설계 작성 후 adapter 선정·구현 검증 대기이며, 보충 API는 API-103~107로 통합했다. [통합 결과](security-api-integration.md)를 따른다.

## 8. 검증 계획

[권한 시나리오](authorization-scenarios.json)는 같은 매장/타 매장, 사용자/운영자, 유효/철회 capability, 자금 승인/서명, 동의/삭제의 허용·거절 기대값이다. 현재 검사기는 API/정책/테이블 참조와 읽기 endpoint의 도메인 쓰기 여부 등 **명세 일관성**만 검증한다. 시나리오를 실제 middleware나 worker에서 실행했다고 주장하지 않는다.

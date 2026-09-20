# 설계 통합 현황과 남은 결정

2026-09-19 · 상세 설계 중 · 제품 구현 보류 · 개인 배정/공수 산정 보류

**승인 HTTP·BLE·권한·화면·논리 저장은 현재 기준에 통합됐다. 결제 원장·반납의 별도 후보 및 새 물리 SQL/상품별 adapter는 추가 설계가 필요하다.** 이 문서는 그 차이와 전체 제품의 남은 결정을 한곳에서 추적한다. 명세 수나 예제 검증 수는 개발 진척률이 아니다.

[기준 데이터](design-integration-register.json) · [전체 작업](../work-breakdown-plan.md) · [기존 결정 카드](decisions.md) · [기술 선택](technology-selection.md)

## 1. 유지하는 범위와 현재 상태

- 12주 내 15개 요구사항 모두 앱 연동
- 참여자 3명, 개인 배정/공수 보류
- NU-54V-DK 펌웨어 Zephyr
- RN 태블릿 키오스크, S25 Ultra 보유
- StableNet testnet, dummy USDC 우선, 초기 고객 native gas
- EOA 우선 후 12주 범위 스마트 계정, 운영자 후원은 후속
- HW 단독 서명과 Cloud MPC 요구 구분
- 일반적인 다음 진행은 설계 연속이며 구현/정책 승인 아님

현재 작업은 26개 작업군·104개 패키지·320개 세부 작업이다. 19개 결정, 19개 기술 선택, 23개 향후 검증 카드를 유지한다. 이번 통합 정리로 작업이나 기능을 추가하지 않았다.

기준 설계 카탈로그: 110개 API, 34개 BLE 논리 명령, 10개 이벤트, 37개 화면. DB 참조 DDL과 보안·결제·반납의 논리 저장 자원은 서로 다른 수준이다. 후보 자원 수를 기존 물리 테이블 수에 더해 신규 테이블로 확정하지 않는다.

| 설계 계층 | 기준 파일 | 해석 |
| --- | --- | --- |
| 사용자 확정 범위 | [twelve-week-completion-scope-v3.md](../twelve-week-completion-scope-v3.md) · [work-breakdown.json](work-breakdown.json) | 범위/진행단계 기준; 세부 제안을 확정으로 승격하지 않음 |
| 기준 설계 카탈로그 | [api-catalog.json](../specifications/api-catalog.json) · [ble-catalog.json](../specifications/ble-catalog.json) · [core.schema.json](../specifications/core.schema.json) · [README.md](../specifications/database/README.md) | 명세와 참조 DDL; 운영 중 서비스나 구현 완료가 아님 |
| 미병합 상세 후보 | [commerce-contract-candidate.json](../specifications/commerce-contract-candidate.json) · [return-recovery-contract.json](../specifications/return-recovery-contract.json) · [return-protocol-candidate.json](../specifications/return-protocol-candidate.json) · [commerce-reconciliation-integration.json](../specifications/commerce-reconciliation-integration.json) · [commerce-consumer-repair-design.json](../specifications/commerce-consumer-repair-design.json) · [settlement-ops-contracts.json](../specifications/settlement-ops-contracts.json) | 변경안/검토 예제; 실제 호출 경로/물리 저장/선택 정책 완료 아님 |
| 화면 서술 반영 | [return-screen-design.json](../specifications/return-screen-design.json) · [screen-flows.json](../specifications/screen-flows.json) | 5개 기존 화면에 문구·복구 원칙 반영; 프로토콜 병합/실제 UI 구현과 다름 |
| 현재 승인 기준 | [approval-baseline.json](../specifications/approval-baseline.json) · [approval-baseline.schema.json](../specifications/approval-baseline.schema.json) · [approval-storage-mapping.json](../specifications/approval-storage-mapping.json) · [approval-source-dispatch.json](../specifications/approval-source-dispatch.json) · [approval-physical-storage-design.json](../specifications/approval-physical-storage-design.json) · [approval-physical-storage-design.md](../specifications/approval-physical-storage-design.md) | HTTP13경로·BLE5쌍·권한/화면/논리저장 통합. 타 상품 adapter와 새 물리 SQL은 미완료; runtime unverified. |
| 미병합 통합 묶음·수용 기준 | [integration-adoption-matrix.json](integration-adoption-matrix.json) | DI8/수용24/후보처분20/15요구/19결정 재대조. 새로운 카탈로그 병합이나 구현 완료 아님. |
| 구현 전 논리설계 인계 후보 | [preimplementation-handoff.json](preimplementation-handoff.json) · [preimplementation-contract-overlay.json](../specifications/preimplementation-contract-overlay.json) | 8개 설계 묶음·104작업·320단계 연결. 프로필선택/기준채택/실제실행과 분리하며 전체 구현준비 완료 아님. |

후속 상세 설계: [wallet-control-recovery-design.md](../specifications/wallet-control-recovery-design.md) — 최신 [승인 기준 통합](../specifications/approval-baseline.md)은 체크포인트 보존 후 현재 API110/권한60/논리저장28로 반영됐다. 이전 후보는 이력이며 물리 SQL/타 상품별 adapter/실행 검증은 남아 있다.

## 2. 먼저 해소할 명세 차이

### DC-01 · 반납 종료와 재대여 원자 경계

- 기준 명세: storage-operations.md TX-07은 returned와 재대여 가능을 함께 반영
- 후보: return-protocol/return-screen은 cleanup/gate를 별도 확인
- 통합 기준: 화면 최신 서술이 API/저장 계약까지 병합됐다는 뜻이 아니다
- 연결 묶음: DI-06, DI-07

### DC-02 · 온라인 commit·relay·cleanup 호출 경로 부재

- 기준 명세: 현재 기준 API110/BLE34에 반납 commit·relay·cleanup의 새 전용 경로는 아직 없음
- 후보: HTTP 기존3/미등록7·BLE 기존2/미등록4의 전체 논리 envelope 후보 작성; 후속 API045 예약/활성화·grant 전 취소 경로 후보 추가
- 통합 기준: 후보 타입/화면 버튼이 있어도 호출 가능한 것으로 표시하지 않는다
- 연결 묶음: DI-04, DI-05

### DC-03 · 화면의 gate 판단 입력

- 기준 명세: API-091 RentalView/ResetStateView는 후보 정리 상태/현재 epoch/gate 상세를 담지 않음
- 후보: API091 opt-in return projection에 같은 작업/epoch의 current readiness와 job 없는 null 분기 설계; 조회와 API045 현재 조건 잠금/예약 분리
- 통합 기준: 다른 job/시점의 조회를 혼합해 eligible을 추론하지 않는다
- 연결 묶음: DI-04, DI-06, DI-07

### DC-04 · API fragment와 전체 endpoint 차이

- 기준 명세: 기준 request/envelope/query/access mappings 유지
- 후보: commerce-reconciliation-integration은 현재 API110 기준의 전체 HTTP 5개 경로. API037 signer/snapshot을 유지하고 나머지 4개 API에 version/auth/query/error를 포함한 변경안을 제시
- 통합 기준: candidate schema 통과를 전체 HTTP/권한 구현 통과로 보지 않는다
- 연결 묶음: DI-01, DI-08

### DC-05 · 환불 원천별 고객 조회 권한

- 기준 명세: refund_read 문구의 원주문 고객 범위는 다중 payer에서 지나치게 넓게 읽힐 수 있음
- 후보: 현재 호환 연결안에서 allocation 소유권과 exact-store 환불 원천 예산 권한을 분리; canonical refund_read 문구 반영은 대기
- 통합 기준: 주문 공유가 다른 payer의 환불 조회권을 부여하지 않는다
- 연결 묶음: DI-01

## 3. 설계 반영 묶음과 완료 기준

DI 번호는 새 개발 작업이 아니라 기존 작업에 연결할 명세 변경 묶음이다. 선행 관계는 설계 병합 완료에 필요한 결과를 뜻하며, 조사나 초안 작성을 순서대로만 하라는 뜻은 아니다. 완료 기준을 만족해도 제품 실행 검증을 대신하지 않는다.

### DI-01 · 결제·환불·매출·정산·운영 API와 권한

상태: `current_compatible_full_envelopes_drafted_not_merged` · 결정: D02, D08, D19 · 선행 설계 결과: 없음

원본: [commerce-contract-candidate.json](../specifications/commerce-contract-candidate.json) · [commerce-reconciliation-integration.json](../specifications/commerce-reconciliation-integration.json) · [settlement-ops-contracts.json](../specifications/settlement-ops-contracts.json)

반영 대상:

- API-030/035/036/037/038 request/response; API-030 query and pagination
- API-018 refund source/hold semantic checks
- authorization-policies + access mappings + screen projection
- API-039/040/041/093/087 전체 계약과 미등록 SR-01/02; 단일 consumer/projection 반영 권한

설계 완료로 확인할 결과:

- 원지급 선택·funding revision·고객 allocation별 조회 조건의 단일 명세
- 현재 후보 fragment 밖의 HTTP/error/query와 전체 response 연결
- D08 제안과 확정된 운영 정책의 구분

### DI-02 · 결제·환불 이벤트 버전과 소비 규칙

상태: `consumer_projection_repair_contract_drafted_not_merged` · 결정: D02, D08, D09, D17, D18, D19 · 선행 설계 결과: DI-01

원본: [commerce-contract-candidate.json](../specifications/commerce-contract-candidate.json) · [commerce-reconciliation-integration.json](../specifications/commerce-reconciliation-integration.json) · [commerce-consumer-repair-design.json](../specifications/commerce-consumer-repair-design.json)

반영 대상:

- payment.acceptance.changed / refund.state.changed v2
- DomainEvent envelope and producer/consumer version/checkpoint
- order/allocation/refund/funding/observation revisions

설계 완료로 확인할 결과:

- 버전1/2 전환 규칙·단일 원장 반영 경로
- 중복/역순/누락 snapshot 복구 명세
- 소비자5개 목표기여분/불변보정/manifest 완전성/원자publication/재구축7단계 제안; 실행미검증22사례

### DI-03 · 결제·환불과 보안 저장 경계

상태: `companion_mapping_and_atomic_boundaries_proposed_sql_unapplied` · 결정: D02, D03, D08, D09, D17, D19 · 선행 설계 결과: DI-01, DI-02

원본: [commerce-contract-candidate.json](../specifications/commerce-contract-candidate.json) · [security-storage-contracts.json](../specifications/security-storage-contracts.json) · [commerce-reconciliation-integration.json](../specifications/commerce-reconciliation-integration.json)

반영 대상:

- OrderReconciliation / PaymentException / RefundFundingGuard / RefundReconciliation
- TX-04/05 and reference DDL relationship/constraints
- current 28 security logical resources and adapter atomicity

설계 완료로 확인할 결과:

- 새 물리 테이블로 합산하지 않는 실제 column/관계 매핑안
- 잠금 순서·idempotency/outbox·한도 보정·승인 보류의 일관된 계약
- 기존 61테이블 검증이 새 제약까지 검사했다고 표시하지 않음

### DI-04 · 반납 API·허가·증거 접수

상태: `admission_cancel_route_proposal_not_adopted` · 결정: D02, D03, D09 · 선행 설계 결과: 없음

원본: [return-protocol-candidate.json](../specifications/return-protocol-candidate.json) · [return-recovery-contract.json](../specifications/return-recovery-contract.json) · [return-route-contracts.json](../specifications/return-route-contracts.json) · [rental-admission-cancel.json](../specifications/rental-admission-cancel.json) · [lifecycle-storage-design.json](../specifications/lifecycle-storage-design.json) · [lifecycle-bootstrap-contracts.json](../specifications/lifecycle-bootstrap-contracts.json) · [enrollment-continuity-adoption.json](../specifications/enrollment-continuity-adoption.json) · [lifecycle-journey-acceptance.json](../specifications/lifecycle-journey-acceptance.json)

반영 대상:

- API-046/047/091 typed changes
- RP-02 online commit; relay authority issuance; cleanup submission/challenge paths
- HTTP method/path/authorization envelope/expiry/idempotency/errors

설계 완료로 확인할 결과:

- RP별 실제 endpoint/명령 대응표; 입력/출력/권한/오류 전체
- 기존 clearance 만료 의미와 새로운 commit/late evidence 구분
- 복구 정책 unresolved면 새 commit 불가 유지

### DI-05 · 기기·BLE·신뢰 profile

상태: `admission_cancel_route_proposal_not_adopted` · 결정: D02, D03, D05, D06, D09 · 선행 설계 결과: DI-04

원본: [return-protocol-candidate.json](../specifications/return-protocol-candidate.json) · [return-recovery-contract.json](../specifications/return-recovery-contract.json) · [implementation-interfaces.json](../specifications/implementation-interfaces.json) · [return-route-contracts.json](../specifications/return-route-contracts.json) · [rental-admission-cancel.json](../specifications/rental-admission-cancel.json) · [lifecycle-storage-design.json](../specifications/lifecycle-storage-design.json) · [lifecycle-bootstrap-contracts.json](../specifications/lifecycle-bootstrap-contracts.json) · [enrollment-continuity-adoption.json](../specifications/enrollment-continuity-adoption.json) · [lifecycle-journey-acceptance.json](../specifications/lifecycle-journey-acceptance.json)

반영 대상:

- device.reset.prepare/confirm; reset_recovery_relay role and action catalogue
- IF-02/07/14; identity/proof/epoch/journal/cleanup profile
- encoding/digest/encryption/transport bounds and version negotiation

설계 완료로 확인할 결과:

- firmware-base Zephyr와 미선정 SDK/board target 구분
- 새 role을 기존 owner role로 우회하지 않는 ACL 명세
- 보호 능력 가정과 개발 단계 실기 검증 입력 분리

### DI-06 · 반납 저장·정리·재대여 조건

상태: `lifecycle_storage_proposal_not_applied` · 결정: D02, D03, D09 · 선행 설계 결과: DI-04, DI-05

원본: [return-recovery-contract.json](../specifications/return-recovery-contract.json) · [return-protocol-candidate.json](../specifications/return-protocol-candidate.json) · [rental-admission-cancel.json](../specifications/rental-admission-cancel.json) · [lifecycle-storage-design.json](../specifications/lifecycle-storage-design.json) · [lifecycle-bootstrap-contracts.json](../specifications/lifecycle-bootstrap-contracts.json) · [enrollment-continuity-adoption.json](../specifications/enrollment-continuity-adoption.json) · [lifecycle-journey-acceptance.json](../specifications/lifecycle-journey-acceptance.json)

반영 대상:

- ResetJob / DeviceResetJournal / RecoveryRelayGrant / ReenrollmentGate
- TX-07; rentals/device binding/reset clearances/jobs
- API-045/enrollment admission and authoritative device epoch

설계 완료로 확인할 결과:

- returned와 cleanup/gate를 분리한 transaction·조회·이벤트 계약
- 완료 후 cleanup 실패/marker missing 상태 조합
- 새 등록 mutation도 최신 gate/epoch/활성 binding 재검증

### DI-07 · 화면·권한·서버 상태 정합

상태: `admission_cancel_route_proposal_not_adopted` · 결정: D01, D02, D03, D08, D09, D17, D19 · 선행 설계 결과: DI-01, DI-04, DI-06

원본: [return-screen-design.json](../specifications/return-screen-design.json) · [screen-flows.json](../specifications/screen-flows.json) · [return-route-contracts.json](../specifications/return-route-contracts.json) · [rental-admission-cancel.json](../specifications/rental-admission-cancel.json) · [lifecycle-storage-design.json](../specifications/lifecycle-storage-design.json) · [lifecycle-bootstrap-contracts.json](../specifications/lifecycle-bootstrap-contracts.json) · [enrollment-continuity-adoption.json](../specifications/enrollment-continuity-adoption.json) · [lifecycle-journey-acceptance.json](../specifications/lifecycle-journey-acceptance.json)

반영 대상:

- U24/D01/O01/U04/U05 existing narrative updates
- RU-01..11 and RO-01..06, guarded RA actions
- UI action → API/BLE/mutation authorization → result state

설계 완료로 확인할 결과:

- 37개 화면 유지, 문구 반영과 프로토콜 준비를 구별
- RP-08 상태 조회만으로 구체 gate/binding facts를 얻는다고 가정하지 않음
- 모든 행동의 실제 route/refetch/source proof 연결; 없는 취소/재인증 경로 명시

### DI-08 · 단일 기준 명세로 검토·병합

상태: `adoption_matrix_drafted_no_new_canonical_merge` · 결정: D02, D19 · 선행 설계 결과: DI-01, DI-02, DI-03, DI-04, DI-05, DI-06, DI-07

원본: [work-breakdown.json](work-breakdown.json) · [compatibility-matrix.json](../specifications/compatibility-matrix.json) · [settlement-ops-contracts.json](../specifications/settlement-ops-contracts.json) · [integration-adoption-matrix.json](integration-adoption-matrix.json) · [lifecycle-journey-acceptance.json](../specifications/lifecycle-journey-acceptance.json)

반영 대상:

- catalogues + shared/extended/critical schemas + examples + access policies + screens
- state/store/event/compatibility/WBS references and generated views
- candidate disposition and baseline source hashes

설계 완료로 확인할 결과:

- 각 정책 선택의 출처·미정·후보 상태 기록
- 영향 파일 일괄 변경 후 shape/참조 검증과 미구현 수용 조건 유지
- 후보 폐기/부분반영/전체반영 표기; 검증되지 않은 runtime 완료 금지
- 매출·정산/운영 기존경로5개와 미등록SR후보2개 전체envelope,단일consumer publication권한·scope/fence·화면 연결 제안

## 4. 19개 결정의 성격 구분

D 번호는 기존 결정 ID다. 모든 항목이 열려 있다는 이유로 확정된 하위 조건까지 미정으로 되돌리지 않는다. 아래 정책 입력은 앞으로 확인할 선택의 목록이며 이번에 19개 질문을 보냈다는 뜻이 아니다. 기술 설계는 현재 단계에서 진행하고, 실기·서비스 검증은 개발 단계에 수행한다.

| 결정 | 사용자/운영 정책 입력 | 현재 진행할 기술 설계 | 개발 단계 검증 |
| --- | --- | --- | --- |
| D01 앱·인증 대상 | 추가 OS·태블릿·배포 경로·언어 우선순위 | 유저 앱 RN 채택안·native bridge 공유 경계 | 플랫폼별 설치/Google·Apple 인증 |
| D02 연결 규칙·데이터 경계 | — | API/BLE/proof 버전·인증 envelope·역할별 projection·정규화·원자 저장 경계 | 실제 profile 호환·권한/동시성 |
| D03 지갑 키·복구 | 신규 여행 지갑의 개인 복구 백업 추가 여부 또는 복구 없을 때 초기화 보류；HW/Cloud 지갑의 주소·복구 관계 | 키 파생·격리·import·장치 신원/epoch/journal 보호·늦은 증거 검증 | 원시 키 비노출·서명·삭제·초기화 후 복구 |
| D04 MPC 신뢰 구조 | MPC 참여 주체·복구 권한·서비스 중단 시 접근 정책 | 제공자/라이브러리·임계값·조각 보관 경계·세션 중단 규칙 | 실제 DKG/임계 서명·참여자 변경·복구 |
| D05 FOTA 운영 | 업데이트 시 사용자 중단/강제 여부와 운영 배포·복귀 정책 | Zephyr SDK/board target·부트로더/슬롯·서명 키 운영·저장 호환 | 서명 이미지·전원 차단·부팅 복귀·키 보존 |
| D06 패스키 호환 범위 | 우선 사용할 passkey 서비스와 기기/브라우저 | 표준 transport·등록/인증·대체 로그인 및 반납 자격 정리 | 실제 RP/OS/브라우저 조합 상호운용 |
| D07 녹음·찾기 목표 | 녹음 품질/길이·백그라운드 사용·찾기 반응 목표 | 마이크/출력 부품·버퍼·BLE 전송·동시 동작·거리 유효성 | 음성 누락·배터리/메모리·실제 거리/백그라운드 |
| D08 결제·환불·정산 | 최초 전체 지급 수락·분할 지급·늦은/중복 지급 반환 정책；환불·수취·정산 운영 규칙과 비용 부담의 예외 | 원지급별 한도·확정/관측 보정·quote/환율 출처·목적지 증명 | 동시 지급/환불·reorg·정산 보정 |
| D09 대여·혜택 | 기존 D03 복구 질문을 공유하며 중복 질문하지 않음；적립/사용·환불 후 사용된 혜택·회수 불가 기기 운영 정책 | 반납 fence·commit·증거 relay·ACK/cleanup·재대여 gate | 다음 대여자 격리·증거 유실·혜택 중복/보정 |
| D10 배포/시험 환경 | — | StableNet manifest·dummy token 권한·native/wrapped 구분·ABI/배포 block·Indexer endpoint | 실제 가스 수급·배포/조회·토큰 호출 |
| D11 스마트 계정 전환 | EOA 이후 주소/자산/권한 전환 경험과 복구 방식 | account/EntryPoint/SDK/Bundler 버전 묶음·HW/Cloud signer 지원 | 실제 UserOperation·전환/복구·거래 조회 |
| D12 DeFi·FX 상품 | DeFi 대표 동작·FX 상품 모델·시험 자산 쌍 | pool·유동성·가격·슬리피지·견적/실행 adapter | 실제 swap/LP/FX 실행·실패/가격 기한 |
| D13 Perpetual 상품 | 시험 시장·담보·레버리지·가격 장애 시 상품 정책 | 가격/오라클·펀딩·청산·keeper·위험 계산 규칙 | 실제 포지션/펀딩/청산·장애 복구 |
| D14 STO 범위 | 시험 발행물의 권리·발행자·전송/보유 자격 | STO 계약·자격·앱 동작·이벤트 모델 | 허용/거부/철회·잔액/권리 조회 |
| D15 DID 범위 | 자격의 내용·발급자/검증자·공개할 정보 | DID/credential/proof 표준·철회 모델·STO 연결 | 실제 발급/검증/철회·개인 정보 노출 방지 |
| D16 x402 범위 | 유료 자원과 재요청 과금 경험·고객 가스 부담 예외 여부 | x402 버전·token authorization 방식·facilitator·지급/전달 분리 | 선택한 토큰 방식·실제 gas payer·반복 과금 방지 |
| D17 위치·후기·실제 데이터 | 위치 수집·보관·삭제·후기 출처 정책 | 장소 제공자·장소/매장 연결·동의/삭제·실제/시험 구매 구분 | 위치 거절·삭제 전파·다중 출처 일치 |
| D18 추천·챌린지 기준 | 추천 코스 목표·제약·챌린지 완료/보상 기준 | AI 입력/검증·데이터 부족 대안·증거 철회 재평가 | 평가 표본·근거/영업시간·중복 보상 방지 |
| D19 운영·수용 목표 | 운영 역할·품질/복구 목표·실자산 파일럿 여부 | 관측/보존/복구·알림·지원·출시 증거·운영 접근 설계 | 성능/배터리·백업 복구·장애 대응·실자산 전 별도 검토 |

D02·D10은 새 사용자 질문 없이 기술 명세를 더 진행할 수 있다. D03·D09의 복구 정책은 같은 대기 질문을 공유한다. 그 외 정책 입력은 관련 제품을 구체화할 때 제안과 선택 영향을 준비한다. 상세 작업·TECH 연결은 JSON과 기존 결정 카드에 보존했다.

## 5. 전체 15개 요구사항의 추적

관련 작업 수는 중복을 포함한다. 결정 ID는 해당 요구 작업의 기존 decisionInputs 합집합으로, 모든 결정이 모든 하위 기능을 차단한다는 의미가 아니다. 완료율이나 공수로 사용하지 않는다.

| 원문 번호 | 요구사항 | 관련 작업 수 | 결정 연결 맥락 |
| --- | --- | --- | --- |
| 1 | 실제기기에서 hw wallet 으로 동작하는 펌웨어 구현 | 17 | D02, D03, D04, D05, D06, D07, D08, D09, D10, D11, D12, D13, D14, D15, D16, D17, D18, D19 |
| 2 | 실제기기에서 펌웨어 업그레이드를 지원하는 fota 구현 | 9 | D03, D04, D05, D06, D07, D08, D09, D10, D11, D12, D13, D14, D15, D16, D17, D18, D19 |
| 3 | 실제기기에서 passkey, 녹음기, 디바이스 찾기, 결제 스탬프 지원 | 17 | D03, D04, D05, D06, D07, D08, D09, D10, D11, D12, D13, D14, D15, D16, D17, D18, D19 |
| 4 | 실제기기를 설정하는 유저 App 구현 | 25 | D01, D02, D03, D04, D05, D06, D07, D08, D09, D10, D11, D12, D13, D14, D15, D16, D17, D18, D19 |
| 5 | 유저 App 에서 Social Login 지원 (google, apple) | 14 | D01, D02, D03, D04, D05, D06, D07, D08, D09, D10, D11, D12, D13, D14, D15, D16, D17, D18, D19 |
| 6 | 유저 App 의 Social Login 에 따른 Cloud Wallet 지원 ( 키관리 MPC 지원 ) | 14 | D01, D02, D03, D04, D05, D06, D07, D08, D09, D10, D11, D12, D13, D14, D15, D16, D17, D18, D19 |
| 7 | 유저 App 에서 위치 기반 맛집 검색, 결제 기록, 발자취 지원 (여행자 모드 지원) | 22 | D01, D03, D04, D05, D06, D07, D08, D09, D10, D11, D12, D13, D14, D15, D16, D17, D18, D19 |
| 8 | 유저 App 에서 실제기기 설정 지원 | 35 | D01, D02, D03, D04, D05, D06, D07, D08, D09, D10, D11, D12, D13, D14, D15, D16, D17, D18, D19 |
| 9 | 키오스크 App 구현 ( 가게 회원 가입 및 로그인(소셜 로그인과 동일), 가게 관리 지원(메뉴 관리, 매출 관리, 환불 처리, 정산 처리 등) ) | 26 | D01, D02, D03, D04, D05, D06, D07, D08, D09, D10, D11, D12, D13, D14, D15, D16, D17, D18, D19 |
| 10 | 백오피스 서비스 구현 및 지원 | 26 | D01, D02, D03, D04, D05, D06, D07, D08, D09, D10, D11, D12, D13, D14, D15, D16, D17, D18, D19 |
| 11 | stablenet testnet 기반 usdc, wkrc, defi, smart account, fx, perpetual , sto, did, x402 컨트랙트 지원 | 32 | D03, D04, D05, D06, D07, D08, D09, D10, D11, D12, D13, D14, D15, D16, D17, D18, D19 |
| 12 | stablenet testnet 기반 indexer 구현 및 지원 | 16 | D02, D03, D04, D05, D06, D07, D08, D09, D10, D11, D12, D13, D14, D15, D16, D17, D18, D19 |
| 13 | stablenet testnet 기반 dex 서비스 구현 및 지원 ( defi , fx, perpetual ) | 17 | D03, D04, D05, D06, D07, D08, D09, D10, D11, D12, D13, D14, D15, D16, D17, D18, D19 |
| 14 | stablenet testnet 기반 여행자 서비스 지원 ( ai 기반 + 실제 결제 데이터 + 유저 후기 + 위치  = 추천 코스 생성 ( 따라하기 챌린지, 여행 발도장 ) | 16 | D01, D03, D04, D05, D06, D07, D08, D09, D10, D11, D12, D13, D14, D15, D16, D17, D18, D19 |
| 15 | 그외 기타 필요한 툴 및 서비스 구현 | 17 | D01, D02, D03, D04, D05, D06, D07, D08, D09, D10, D11, D12, D13, D14, D15, D16, D17, D18, D19 |

## 6. 다음 설계 순서

- **NEXT-01 · 두 지갑·신규 여행 지갑 복구와 반납 정책** (D03, D04, D09): 사용자 경험/키 통제/복구 경계의 선택안과 미병합 계약 영향. RR-DEC-01 답변 전 백업/반납 정책을 확정하지 않음.
  현재 상태: `design_candidate_drafted_policy_pending` · [wallet-control-recovery-design.md](../specifications/wallet-control-recovery-design.md)
- **NEXT-02 · 지갑 선택·승인 문맥의 전체 계약 연결, 결제·반납 후보 정합성** (D02, D08, D09): 지갑 조회·signer 가용성·승인 결합과 기존 DI-01..08의 경로·권한·응답·저장 매핑안. 전체15요구/104작업/320세부작업·설계8묶음 유지. DS01~08 논리 후보와 구현 전 인계서 작성. 정책19개 미선택·역할/공수 미배정·구현 보류..
  현재 상태: `full_scope_review_drafted` · [full-scope-design-review.md](full-scope-design-review.md)
- **NEXT-03 · 기기 동시동작·FOTA 공존 상세 설계** (D03, D05, D06, D07, D09): 서명/패스키/녹음/찾기/반납/FOTA의 자원·권한·중단/복구 상태표. 공존100쌍·FOTA전이10개·실기미실행28사례. SDK/profile/수치·실제 부트 commit 순서는 미선정; 후속 후보는 최신 인계서 참조..
  현재 상태: `device_coexistence_candidate_drafted` · [device-coexistence-design.md](../specifications/device-coexistence-design.md)
- **NEXT-04 · 소셜 로그인·두 지갑·MPC 복구 연결 상세 설계** (D01, D03, D04): 계정 연결/전환·세션 철회와 HW/Cloud 각각의 서명/복구 권한, MPC 참여자/복구 선택표. 인증12/MPC11 전이·권한영향8종·미실행32사례. 제공자/threshold/주소정책 미선정, 후속 후보는 최신 인계서 참조..
  현재 상태: `social_wallet_recovery_candidate_drafted` · [social-wallet-recovery-design.md](../specifications/social-wallet-recovery-design.md)
- **NEXT-05 · 키오스크·결제·환불·스탬프·정산 종단 연결** (D01, D08, D09): 고객 주문/결제와 점주 메뉴·매출·환불·정산 화면, 현재 승인/대사 후보의 종단 실패 복구. 12여정·표시10개·정책입력7개·미실행34사례. fulfillment/단말복구/후보채택 미완료, 후속 후보는 최신 인계서 참조..
  현재 상태: `kiosk_commerce_journeys_candidate_drafted` · [kiosk-commerce-journey-design.md](../specifications/kiosk-commerce-journey-design.md)
- **NEXT-06 · StableNet 자산·스마트계정·Indexer 호환 설계** (D10, D11, D19): 자산/가스/ABI/address/version manifest, EOA 이후 스마트계정과 이벤트·조회모델 연결. manifest8경계·온체인9영역·미실행34사례. 실제RPC/배포·ABI/AA adapter 미검증; 후속 후보는 최신 인계서 참조..
  현재 상태: `stablenet_compatibility_candidate_drafted` · [stablenet-compatibility-design.md](../specifications/stablenet-compatibility-design.md)
- **NEXT-07 · DeFi·FX·Perpetual 상품별 상태/위험 계약** (D12, D13, D19): 견적·유동성·슬리피지·가격원·증거금·펀딩·청산과 앱 상태 연결. 상품4영역·가격행위10종·keeper6단계·미실행32사례. 모델/수치/market adapter 미선정; 후속 후보는 최신 인계서 참조..
  현재 상태: `market_product_candidate_drafted` · [market-product-design.md](../specifications/market-product-design.md)
- **NEXT-08 · DID·STO·x402 자격/지급 계약** (D14, D15, D16): 발급·검증·철회, 시험 발행물 자격/권리, 유료자원 지급·제공 결과 연결. DS06 후보: 자격/시험권리/지급·제공분리·미실행24사례. 프로필/권리 선택 및 기준채택 미완료..
  현재 상태: `credential_paid_resource_candidate_drafted` · [credential-paid-resource-design.md](../specifications/credential-paid-resource-design.md)
- **NEXT-09 · 녹음·여행·AI·개인정보 수명주기** (D07, D17, D18, D19): 원음·전사·추천·위치·후기 provenance와동의/삭제·수용 연결. DS07 후보·평가표본12개·미실행24사례. 공급자/수치/보존정책 미선정..
  현재 상태: `recording_travel_ai_candidate_drafted` · [recording-travel-ai-design.md](../specifications/recording-travel-ai-design.md)
- **NEXT-10 · 운영·릴리스·백업복구·전체 증거** (D10, D19): 환경5구분·릴리스8단계·runbook10개·증거계약. DS08 후보·미실행16사례. 운영 목표/실제복구/출시결정 미완료..
  현재 상태: `operations_release_acceptance_candidate_drafted` · [operations-release-acceptance-design.md](../specifications/operations-release-acceptance-design.md)
- **NEXT-11 · 구현 전 전체 설계 인계** (D01, D02, D03, D04, D05, D06, D07, D08, D09, D10, D11, D12, D13, D14, D15, D16, D17, D18, D19): 15요구/104작업/320단계·경계계약24개·행위11종·남은조건/선택근거 연결. 독립 작성 가능한 논리 초안 연결. 선택의존 wire/물리설계 고정·미병합 기준채택과 실제검증은 남음. 즉시구현가능/설계확정 선언이 아님..
  현재 상태: `design_handoff_candidate_selection_pending` · [preimplementation-handoff.md](preimplementation-handoff.md)

**이미 질문한 복구 정책은 답변 대기 중이다.** 신규 여행 지갑에 본인 전용 암호화 복구 백업을 추가할지, 복구 수단이 없으면 초기화를 보류할지는 선택되지 않았다. 이 문서에서는 질문을 반복하거나 일반적인 진행 요청을 동의로 해석하지 않는다. 답변 전에도 두 지갑의 사용·권한·복구 비교 설계를 진행할 수 있다. 필수 복구 조건이 충족되지 않은 상태에서 새 파괴적 초기화 허가를 발급하는 것으로 설계하지 않는다.

## 7. 이 문서의 검증 범위

렌더러는 원본 파일, WBS의 19개 결정·작업·기술 참조, 15개 요구 추적, 설계 묶음 의존 관계, 미답변 복구 정책과 구현 보류 상태, Markdown 동기화를 확인한다. 프로토콜 선택이나 보안·성능·법적 적합성·실제 기기 동작을 검증한 결과는 아니다.

- 새 기능/작업 패키지 증설
- 최종 Seed 또는 구현 착수 선언
- 미답변 정책 선택
- 개발환경 설치·제품 코드·계약 배포·실기 시험
- 개인별 역할/공수 배정

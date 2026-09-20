# 상세 작업 카드 — 세부 실행 작업

2026-09-18. 26개 작업군, 104개 작업 패키지, 320개 세부 작업. 전체 범위와 앱 연동을 12주 안에 완료하는 계획 전제다. 담당자·공수·마감일은 비워 두었다.

[읽는 방법](../work-breakdown-plan.md) · [실행 시나리오](functional-execution-spec.md) · [JSON 원본](work-breakdown.json) · [패키지 CSV](work-breakdown.csv) · [세부 작업 CSV](implementation-steps.csv)

작업 분해·상세 수용 기준·설계 제안은 검토용이다. 결정 입력은 구현 완료 전에 해결할 선택이며, 조사·설계 착수를 금지하지 않는다. 선행 작업은 최종 통합 완료 관계다. 세부 작업 번호는 식별자이며 모든 하위 작업의 순차 실행을 강제하지 않는다. 시나리오 연결은 직접 실행 또는 해당 패키지의 제품 간 검증 맥락을 뜻한다. 현재 제품 실행 증거는 비어 있으며, 기존 코드의 존재를 완료로 표시하지 않았다.

## BASE — 공통 기반·연결 규칙

### BASE-01 · 15개 요구사항을 기능 시나리오와 화면으로 연결

- 원문 연결: 4, 8, 9, 10, 15번
- 산출물: 요구사항-행위자-화면-완료 증거 지도
- 완료 기준: 15개 모두 사용자 시작/결과 화면 지정; 중복 4·8의 책임 구분
- 선행 작업: 없음
- 결정 입력: D01, D02
- 앱/제품 연결: 전체 제품
- 재사용 기준: 기존 검토 문서·네 저장소
- 검증 환경: 명세/예제 대조
- 연결 시나리오: J19
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 착수·완료 증거: BASE-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| BASE-01.01 | 가입·대여·주문·반납 여정을 행위자별로 연결 | 사용자/점주/운영자 여정표 |
| BASE-01.02 | 15개 요구를 화면·서비스·기기 동작에 대응 | 요구사항 추적표 |
| BASE-01.03 | 정상·거절·복구별 증거 위치 지정 | 수용 시나리오 목록 |

### BASE-02 · 공통 데이터·식별자·상태 정의

- 원문 연결: 4, 8, 9, 10, 15번
- 산출물: 계정/지갑/매장/기기/대여/주문/거래 모델
- 완료 기준: 주소와 로그인 계정 구별; 주문·지급·환불 상태 별도
- 선행 작업: BASE-01
- 결정 입력: D02, D08, D09
- 앱/제품 연결: 전체 제품
- 재사용 기준: 기존 검토 문서·네 저장소
- 검증 환경: 명세/예제 대조
- 연결 시나리오: J19
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 저장 연결: operations. [참조 DDL](../specifications/database/README.md)
- 착수·완료 증거: BASE-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| BASE-02.01 | 계정·주소·기기·매장 식별자와 관계 정의 | 엔터티 사전 |
| BASE-02.02 | 주문·지급·환불·혜택 상태를 별도 정의 | 상태 전이표 |
| BASE-02.03 | 금액·시각·이벤트 고유키·버전 규칙 정의 | 데이터 예제와 제약 조건 |

### BASE-03 · 앱·서비스·기기 연결 규칙 작성

- 원문 연결: 4, 8, 9, 10, 15번
- 산출물: API/BLE 메시지·오류·버전 예제
- 완료 기준: 정상·거절·만료·재연결 예제; 키가 일반 API 로그에 포함되지 않음
- 선행 작업: BASE-02
- 결정 입력: D02, D03
- 앱/제품 연결: 전체 제품
- 재사용 기준: 기존 검토 문서·네 저장소
- 검증 환경: 명세/예제 대조
- 연결 시나리오: J19
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-108, API-109. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 저장 연결: protocol_profiles, idempotency_records. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-108, API-109. [API 접근 표](../specifications/api-access-transactions.md)
- 남은 보안 연결 입력: GAP-02. [설계 결과](../specifications/security-integration-inputs.json)
- 보안 수용 시나리오: SEC-02-01, SEC-02-02, SEC-02-03, SEC-02-04, SEC-02-05, SEC-02-06. [흐름·완료 조건](../specifications/security-integration-design.md)
- 보안 논리 저장 계약: ScopedCapability, CapabilityReservation, AccessChallenge, AccessGrantLineage, ProtectedIssuanceResponse, SenderProofReplay, AuthorizationGate. [Adapter 자원](../specifications/security-storage-contracts.json)
- 기술 선택: TECH-01. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-01. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-03. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-03, COMP-18, COMP-19, COMP-20. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: BASE-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| BASE-03.01 | 앱→서버와 앱→기기 메시지 봉투 정의 | 요청 ID/버전/만료/오류 스키마 |
| BASE-03.02 | 재연결·중복 요청·결과 조회 규칙 작성 | 왕복 메시지 예제 |
| BASE-03.03 | 로그 허용 필드와 비밀 필드 구분 | 로그/진단 데이터 규칙 |

### BASE-04 · 기존 코드 호환성 차이를 수정 작업으로 확정

- 원문 연결: 4, 8, 9, 10, 15번
- 산출물: 저장소/커밋·ABI·API 변경 목록
- 완료 기준: SDK–Indexer 및 AA 형식 차이를 재현 입력에 연결; mock/실제 경로 구분
- 선행 작업: BASE-03
- 결정 입력: D10
- 앱/제품 연결: 전체 제품
- 재사용 기준: 기존 검토 문서·네 저장소
- 검증 환경: 명세/예제 대조
- 연결 시나리오: J19
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 기술 선택: TECH-03. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-03. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: BASE-04. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| BASE-04.01 | 고정 커밋의 실제 API와 SDK 호출 비교 | 불일치 재현 목록 |
| BASE-04.02 | Indexer 어댑터·AA 해시/서명 버전 수정 위치 지정 | 파일별 변경 명세 |
| BASE-04.03 | mock·stub·미구현 경로를 수용 시험과 연결 | 대체/검증 추적표 |

### BASE-05 · 실행 환경·데이터 보존·비밀 설정 구성

- 원문 연결: 4, 8, 9, 10, 15번
- 산출물: 환경/DB/파일 저장·접근 설정
- 완료 기준: 개발/시연 값 분리; 재시작 후 업무 데이터 유지; 실제 비밀 없는 설정 예제
- 선행 작업: BASE-02
- 결정 입력: D10, D19
- 앱/제품 연결: 전체 제품
- 재사용 기준: 기존 검토 문서·네 저장소
- 검증 환경: 명세/예제 대조
- 연결 시나리오: J19
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-103, API-107, API-108, API-109, API-110. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 권한/트랜잭션 연결: API-103, API-107, API-108, API-109, API-110. [API 접근 표](../specifications/api-access-transactions.md)
- 남은 보안 연결 입력: GAP-01, GAP-03, GAP-05. [설계 결과](../specifications/security-integration-inputs.json)
- 보안 수용 시나리오: SEC-01-01, SEC-01-02, SEC-01-03, SEC-01-04, SEC-01-05, SEC-01-06, SEC-03-01, SEC-03-02, SEC-03-03, SEC-03-04, SEC-03-05, SEC-03-06, SEC-05-01, SEC-05-02, SEC-05-03, SEC-05-04, SEC-05-05, SEC-05-06. [흐름·완료 조건](../specifications/security-integration-design.md)
- 보안 논리 저장 계약: AuthFlow, RefreshFamily, RotationOutcome, TrustPrincipal, TrustGrantRevision, TrustChangeRequest, ProtectedObject, ObjectPayload, DeletionTombstone, AccessChallenge, AccessGrantLineage, ProtectedIssuanceResponse, SenderProofReplay, AuthorizationGate, ApprovalBinding, SignatureProvenance, SignedPayloadObject, SubmissionDispatch, DispatchAttempt, AddressNonceTrack, SourceExecutionGate, ObservationApplication. [Adapter 자원](../specifications/security-storage-contracts.json)
- 기술 선택: TECH-02, TECH-04, TECH-05. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-02, VAL-04, VAL-05. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-09, IF-13. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-09, COMP-13. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: BASE-05. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| BASE-05.01 | DB·파일·작업 큐·설정의 보관 책임 구분 | 저장소 및 환경 구성표 |
| BASE-05.02 | 재시작·마이그레이션·백업 경로 구성 | 지속성 확인 절차 |
| BASE-05.03 | 개발/시연 자격 증명 주입과 접근 제한 | 비밀 없는 환경 예제 |

### BASE-06 · 두 지갑·기능·서명 형식 지원 행렬 정의

- 원문 연결: 4, 8, 9, 10, 15번
- 산출물: HW/MPC × 송금/결제/스마트계정/DEX/DID/x402 행렬
- 완료 기준: 각 지원 조합의 주소·승인·서명 형식 명시; 지원하지 않는 조합을 완료로 오표시하지 않음
- 선행 작업: BASE-03
- 결정 입력: D03, D04, D11, D12, D15, D16
- 앱/제품 연결: 전체 제품
- 재사용 기준: 기존 검토 문서·네 저장소
- 검증 환경: 명세/예제 대조
- 연결 시나리오: J01, J12, J16, J19
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 저장 연결: protocol_profiles. [참조 DDL](../specifications/database/README.md)
- 기술 선택: TECH-14. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-14. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-10. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-10, COMP-21, COMP-22, COMP-23, COMP-25, COMP-26. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: BASE-06. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| BASE-06.01 | HW와 Cloud 지갑의 거래별 서명 요구 수집 | 거래/메시지/typed data/UserOperation 행렬 |
| BASE-06.02 | 각 조합의 승인 화면과 signer 연결 지정 | 지원 조합별 연결 명세 |
| BASE-06.03 | 필수 조합의 검증과 미지원 안내 구분 | 지원 행렬과 검증 ID |

## AUTH — 소셜 계정·권한

### AUTH-01 · 계정·매장 소속·역할 서버 구현

- 원문 연결: 5, 6, 9, 10번
- 산출물: 계정/소속/권한 API와 저장 모델
- 완료 기준: 다른 매장 접근 거절; 개인 지갑과 매장 관리 권한 분리
- 선행 작업: BASE-02, BASE-05
- 결정 입력: D02
- 앱/제품 연결: 유저 앱·키오스크·백오피스
- 재사용 기준: 신규/기존 계정 코드 재검토
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J01, J04
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-005, API-104, API-105, API-106. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: K01, K04. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: accounts, memberships. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-005, API-104, API-105, API-106. [API 접근 표](../specifications/api-access-transactions.md)
- 남은 보안 연결 입력: GAP-04. [설계 결과](../specifications/security-integration-inputs.json)
- 보안 수용 시나리오: SEC-04-01, SEC-04-02, SEC-04-03, SEC-04-04, SEC-04-05, SEC-04-06. [흐름·완료 조건](../specifications/security-integration-design.md)
- 보안 논리 저장 계약: ReceiptEligibility, ReceiptClaim, ReceiptOwnership. [Adapter 자원](../specifications/security-storage-contracts.json)
- 기술 선택: TECH-03. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-03. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-09. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-09. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: AUTH-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| AUTH-01.01 | 계정·가게 소속·역할 데이터 모델 구현 | 마이그레이션과 조회 API |
| AUTH-01.02 | 자원마다 account/store 범위 검사 | 권한 미들웨어 |
| AUTH-01.03 | 가입·초대·소속 해제 시 권한 변화 확인 | 계정/매장 격리 시나리오 |

### AUTH-02 · Google 로그인 앱·서버 연결

- 원문 연결: 5, 6, 9, 10번
- 산출물: 실제 Google 인증 흐름
- 완료 기준: 가입·재로그인·취소·만료 처리; 서버에서 제공자 증명 검증
- 선행 작업: AUTH-01, APP-01
- 결정 입력: D01
- 앱/제품 연결: 유저 앱·키오스크·백오피스
- 재사용 기준: 신규/기존 계정 코드 재검토
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J01, J04
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-001, API-103. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U01. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: auth_identities. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-001, API-103. [API 접근 표](../specifications/api-access-transactions.md)
- 남은 보안 연결 입력: GAP-01. [설계 결과](../specifications/security-integration-inputs.json)
- 보안 수용 시나리오: SEC-01-01, SEC-01-02, SEC-01-03, SEC-01-04, SEC-01-05, SEC-01-06. [흐름·완료 조건](../specifications/security-integration-design.md)
- 보안 논리 저장 계약: AuthFlow. [Adapter 자원](../specifications/security-storage-contracts.json)
- 기술 선택: TECH-02. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-02. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: AUTH-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| AUTH-02.01 | 앱 로그인 시작·취소·콜백 처리 | Google 앱 인증 모듈 |
| AUTH-02.02 | 서버에서 제공자 증명을 검증하고 계정 연결 | 검증/세션 API |
| AUTH-02.03 | 만료·재로그인·중복 콜백 처리 | 실제 제공자 실행 기록 |

### AUTH-03 · Apple 로그인 앱·서버 연결

- 원문 연결: 5, 6, 9, 10번
- 산출물: 실제 Apple 인증 흐름
- 완료 기준: 가입·재로그인·취소·만료 처리; 제공자 식별자 기반 계정 연결
- 선행 작업: AUTH-01, APP-01
- 결정 입력: D01
- 앱/제품 연결: 유저 앱·키오스크·백오피스
- 재사용 기준: 신규/기존 계정 코드 재검토
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J01, J04
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-001, API-103. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U01. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: auth_identities. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-001, API-103. [API 접근 표](../specifications/api-access-transactions.md)
- 남은 보안 연결 입력: GAP-01. [설계 결과](../specifications/security-integration-inputs.json)
- 보안 수용 시나리오: SEC-01-01, SEC-01-02, SEC-01-03, SEC-01-04, SEC-01-05, SEC-01-06. [흐름·완료 조건](../specifications/security-integration-design.md)
- 보안 논리 저장 계약: AuthFlow. [Adapter 자원](../specifications/security-storage-contracts.json)
- 기술 선택: TECH-02. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-02. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: AUTH-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| AUTH-03.01 | 대상 플랫폼별 Apple 시작·콜백 구성 | Apple 앱 인증 모듈 |
| AUTH-03.02 | 제공자 식별자로 계정 연결하고 최초 정보 처리 | 서버 검증/계정 모델 |
| AUTH-03.03 | 재로그인·정보 미제공·취소 처리 | 실제 제공자 실행 기록 |

### AUTH-04 · 계정 연결·로그아웃·탈퇴 정책 구현

- 원문 연결: 5, 6, 9, 10번
- 산출물: 계정 수명주기 화면/API
- 완료 기준: Google/Apple 중복 연결 정책 적용; 탈퇴와 지갑 자산/복구 관계 표시
- 선행 작업: AUTH-02, AUTH-03
- 결정 입력: D03, D04, D19
- 앱/제품 연결: 유저 앱·키오스크·백오피스
- 재사용 기준: 신규/기존 계정 코드 재검토
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J01, J04
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-002, API-003, API-004, API-006, API-103, API-110. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U01, U25. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: auth_identities, auth_sessions, privacy_requests. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-002, API-003, API-004, API-006, API-103, API-110. [API 접근 표](../specifications/api-access-transactions.md)
- 남은 보안 연결 입력: GAP-01. [설계 결과](../specifications/security-integration-inputs.json)
- 보안 수용 시나리오: SEC-01-01, SEC-01-02, SEC-01-03, SEC-01-04, SEC-01-05, SEC-01-06. [흐름·완료 조건](../specifications/security-integration-design.md)
- 보안 논리 저장 계약: AuthFlow, RefreshFamily, RotationOutcome, DeletionTombstone. [Adapter 자원](../specifications/security-storage-contracts.json)
- 기술 선택: TECH-02, TECH-04. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-02, VAL-04. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-09. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-09. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: AUTH-04. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| AUTH-04.01 | 기존 계정 인증 후 다른 로그인 수단 연결 | 계정 연결/충돌 화면 |
| AUTH-04.02 | 로그아웃·세션 철회·연결 해제 구현 | 세션 수명주기 API |
| AUTH-04.03 | 탈퇴 전 자산·기기·기록 처리 경로 연결 | 탈퇴 안내 및 상태 명세 |

## APP — 유저 앱 공통 흐름

### APP-01 · 유저 앱 실행·탐색·권한 골격 구성

- 원문 연결: 4, 7, 8번
- 산출물: 설치 가능한 앱과 공통 화면/상태
- 완료 기준: 실제 대상 폰 설치; 오프라인·로딩·오류·재시작 구분
- 선행 작업: BASE-01, BASE-05
- 결정 입력: D01
- 앱/제품 연결: 유저 앱
- 재사용 기준: poc-platform 화면·순수 TS 로직 선별 재사용
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J01
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 기술 선택: TECH-01. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-01. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-08. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-08. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: APP-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| APP-01.01 | 대상 빌드·앱 탐색·공통 오류 화면 구성 | 설치 앱 골격 |
| APP-01.02 | 알림·BLE·마이크·위치 권한 안내 연결 | 권한 상태 관리 |
| APP-01.03 | 앱 재시작·백그라운드 복귀 시 진행 작업 복원 | 복원 가능한 앱 상태 |

### APP-02 · 기기 지갑·Cloud Wallet 선택과 자산 화면

- 원문 연결: 4, 7, 8번
- 산출물: 현재 지갑/주소/잔액/네트워크 UI
- 완료 기준: 두 주소·키 책임 혼동 없음; 네이티브 가스와 결제 토큰 잔액 구분; 점주 로그인 시 소속 매장 지갑 자산 조회; 개인 자산/서명 권한과 구분
- 선행 작업: APP-01, BASE-03, INDEX-02
- 결정 입력: D03
- 앱/제품 연결: 유저 앱
- 재사용 기준: poc-platform 화면·순수 TS 로직 선별 재사용
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J01, J04
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-007, API-008, API-009, API-090, API-098. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U02. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: wallets, wallet_bindings. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-007, API-008, API-009, API-090, API-098. [API 접근 표](../specifications/api-access-transactions.md)
- 착수·완료 증거: APP-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| APP-02.01 | 지갑 종류·주소·체인을 구분하는 선택기 구현 | 지갑 선택 화면 |
| APP-02.02 | 개인/소속 매장 지갑의 네이티브·토큰 잔액과 조회 시각 표시 | 개인/점주 컨텍스트별 자산 목록·상세 |
| APP-02.03 | 지갑 변경 시 진행 거래와 서명 대상을 고정 | 거래 대상 혼동 방지 시나리오 |

### APP-03 · 송금·견적·승인·거래 결과 공통 UI

- 원문 연결: 4, 7, 8번
- 산출물: EOA/MPC/DApp에서 재사용할 거래 컴포넌트
- 완료 기준: 정수 금액과 표시 단위 일치; 승인 전 수취인/체인/가스 상한 확인
- 선행 작업: APP-02, PAY-01
- 결정 입력: D08
- 앱/제품 연결: 유저 앱
- 재사용 기준: poc-platform 화면·순수 TS 로직 선별 재사용
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J01
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-017, API-107, API-108, API-109, API-110. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U03. [상태/복구 흐름](../specifications/screen-flows.md)
- 권한/트랜잭션 연결: API-017, API-107, API-108, API-109, API-110. [API 접근 표](../specifications/api-access-transactions.md)
- 남은 보안 연결 입력: GAP-05. [설계 결과](../specifications/security-integration-inputs.json)
- 보안 수용 시나리오: SEC-05-01, SEC-05-02, SEC-05-03, SEC-05-04, SEC-05-05, SEC-05-06. [흐름·완료 조건](../specifications/security-integration-design.md)
- 보안 논리 저장 계약: ProtectedObject, ObjectPayload, AccessChallenge, AccessGrantLineage, ProtectedIssuanceResponse, SenderProofReplay, AuthorizationGate, ApprovalBinding, SignatureProvenance, SignedPayloadObject, SubmissionDispatch, DispatchAttempt, AddressNonceTrack, SourceExecutionGate, ObservationApplication. [Adapter 자원](../specifications/security-storage-contracts.json)
- 구현 인터페이스: IF-10. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-10, COMP-21, COMP-22, COMP-23, COMP-25, COMP-26. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: APP-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| APP-03.01 | 금액 입력·소수점·최대 금액·가스 부족 처리 | 공통 거래 입력 컴포넌트 |
| APP-03.02 | 견적·수취인·체인·수수료 확인 화면 구현 | 서명 전 검토 화면 |
| APP-03.03 | 승인·제출 불명확·확정·실패 결과 연결 | 거래 상태 컴포넌트 |

### APP-04 · 개인 결제·영수증·환불 기록 연결

- 원문 연결: 4, 7, 8번
- 산출물: 필터/상세/진행 상태 화면
- 완료 기준: 실제 주문과 지급·환불 연결; 상태 불명확을 성공으로 표시하지 않음
- 선행 작업: APP-01, PAY-04, SHOP-04
- 결정 입력: D08, D17
- 앱/제품 연결: 유저 앱
- 재사용 기준: poc-platform 화면·순수 TS 로직 선별 재사용
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J01, J03, J05
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-042, API-104, API-105, API-106. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U11. [상태/복구 흐름](../specifications/screen-flows.md)
- 권한/트랜잭션 연결: API-042, API-104, API-105, API-106. [API 접근 표](../specifications/api-access-transactions.md)
- 남은 보안 연결 입력: GAP-04. [설계 결과](../specifications/security-integration-inputs.json)
- 보안 수용 시나리오: SEC-04-01, SEC-04-02, SEC-04-03, SEC-04-04, SEC-04-05, SEC-04-06. [흐름·완료 조건](../specifications/security-integration-design.md)
- 보안 논리 저장 계약: ReceiptEligibility, ReceiptClaim, ReceiptOwnership, PendingBenefitEntitlement. [Adapter 자원](../specifications/security-storage-contracts.json)
- 착수·완료 증거: APP-04. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| APP-04.01 | 주문/지급/환불을 묶은 내역 API 연결 | 개인 거래 목록 |
| APP-04.02 | 영수증·수수료·상태·탐색기 링크 표시 | 내역 상세 화면 |
| APP-04.03 | 진행 거래 갱신·필터·비소유자 차단 확인 | 이력 조회 시나리오 |

## HW — 실제 기기 지갑·연결

### HW-01 · 보드·부품·SDK 빌드와 자원 측정

- 원문 연결: 1, 4, 8번
- 산출물: 재현 가능한 보드 빌드/부품·메모리 기록; Zephyr 기반 NU 보드 빌드 기준과 선택 SDK/Zephyr revision manifest
- 완료 기준: 실제 NU 부팅·버튼/화면 확인; FOTA/녹음 공존 자원 기록; 펌웨어 RTOS는 Zephyr; SDK 배포판/보드 target/정확한 버전과 실제 부팅 증거를 구분하여 기록
- 선행 작업: BASE-01
- 결정 입력: D05, D07
- 앱/제품 연결: 유저 앱·키오스크·NU 화면
- 재사용 기준: 별도 NU 펌웨어·SDK signer 접점
- 검증 환경: 실제 NU 기기 + 대상 앱
- 연결 시나리오: J02
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: device.info, API-013. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 저장 연결: devices. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-013. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-06. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-06. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-01. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-01. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: HW-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| HW-01.01 | 보드 SDK·핀맵·디스플레이/버튼/마이크 배선 확정 | BOM/핀맵/빌드 설정 |
| HW-01.02 | 실기 부팅과 주변장치 진단 실행 | 보드 진단 펌웨어 |
| HW-01.03 | 기본/녹음/서명 시 메모리·전력 측정 | 자원 기준선 |

### HW-02 · 키 저장·생성·주소·초기화 구현

- 원문 연결: 1, 4, 8번
- 산출물: 기기 지갑 수명주기 펌웨어
- 완료 기준: 기기 생성 주소 검증; 재부팅 유지; 초기화 뒤 이전 키 사용 불가
- 선행 작업: HW-01
- 결정 입력: D03
- 앱/제품 연결: 유저 앱·키오스크·NU 화면
- 재사용 기준: 별도 NU 펌웨어·SDK signer 접점
- 검증 환경: 실제 NU 기기 + 대상 앱
- 연결 시나리오: J02
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: wallet.create, wallet.address. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U04, U24. [상태/복구 흐름](../specifications/screen-flows.md)
- 기술 선택: TECH-07. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-07, VAL-20. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-02, IF-14. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-02, COMP-14. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: HW-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)
- 결제·환불·반납 설계 사례: LC-08. [책임·상태 전이 설계](../specifications/commerce-lifecycle-design.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| HW-02.01 | 키 생성·저장·주소 도출 경계 구현 | 키 저장/지갑 모듈 |
| HW-02.02 | 잠금·잠금 해제·재부팅 후 접근 구현 | 접근 상태 머신 |
| HW-02.03 | 초기화 후 이전 키 접근 불가 검증 | 키 수명주기 시험 기록 |

### HW-03 · 인증된 BLE 세션·기기 등록 구현

- 원문 연결: 1, 4, 8번
- 산출물: 기기/앱 연결 모듈
- 완료 기준: 올바른 기기 확인; 미인증 요청·재전송 거절; 끊김/재연결 상태 표시; 소유 앱과 임시 매장 세션 권한 구분; 키오스크 설정/import 접근 차단
- 선행 작업: HW-01, BASE-03, APP-01
- 결정 입력: D02, D03
- 앱/제품 연결: 유저 앱·키오스크·NU 화면
- 재사용 기준: 별도 NU 펌웨어·SDK signer 접점
- 검증 환경: 실제 NU 기기 + 대상 앱
- 연결 시나리오: J02
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: session.open, session.confirm, session.close, payment.identify, device.binding.revoked, API-010, API-011, API-012, API-033, API-090. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U04, K03. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: device_bindings, enrollments. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-010, API-011, API-012, API-033, API-090. [API 접근 표](../specifications/api-access-transactions.md)
- 남은 보안 연결 입력: GAP-02. [설계 결과](../specifications/security-integration-inputs.json)
- 보안 수용 시나리오: SEC-02-01, SEC-02-02, SEC-02-03, SEC-02-04, SEC-02-05, SEC-02-06. [흐름·완료 조건](../specifications/security-integration-design.md)
- 보안 논리 저장 계약: ScopedCapability. [Adapter 자원](../specifications/security-storage-contracts.json)
- 기술 선택: TECH-09. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-09. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-03. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-03, COMP-18, COMP-19, COMP-20. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: HW-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)
- 결제·환불·반납 설계 사례: LC-08, LC-09, LC-10. [책임·상태 전이 설계](../specifications/commerce-lifecycle-design.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| HW-03.01 | 기기 탐색·등록·소유 증명 흐름 구현 | 페어링 화면/등록 API |
| HW-03.02 | 검증된 프로토콜로 인증·암호화·재전송 방지 연결 | 보호된 BLE 세션 |
| HW-03.03 | 해제·만료·재연결 시 권한 재검증 | 세션 전이와 실패 응답 |
| HW-03.04 | 소유 앱과 임시 키오스크 세션의 명령/권한 구분 | 설정·import·서명·조회 권한 표 |
| HW-03.05 | 가게·기기 확인 후 주문에 묶인 결제 세션 수립 | 임시 세션 만료/철회 상태표 |

### HW-04 · 앱에서 키 가져오기·설정 연결

- 원문 연결: 1, 4, 8번
- 산출물: 암호화 import·설정 화면/펌웨어
- 완료 기준: 기존 키로 예상 주소 도출; 실패 시 상태 보존; 키가 일반 저장/로그에 남지 않음
- 선행 작업: HW-02, HW-03
- 결정 입력: D03
- 앱/제품 연결: 유저 앱·키오스크·NU 화면
- 재사용 기준: 별도 NU 펌웨어·SDK signer 접점
- 검증 환경: 실제 NU 기기 + 대상 앱
- 연결 시나리오: J02
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: wallet.import.begin, wallet.import.chunk, wallet.import.commit, wallet.import.abort, device.settings.update, API-098. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U04, U05. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: device_bindings. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-098. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-07. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-07. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-02. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-02. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: HW-04. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)
- 결제·환불·반납 설계 사례: LC-12. [책임·상태 전이 설계](../specifications/commerce-lifecycle-design.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| HW-04.01 | 신규/가져오기 선택·입력 검증·주소 확인 구현 | 온보딩 화면 |
| HW-04.02 | 보호된 세션으로 import 전달 후 기기 내부 저장 | import 명령과 결과 |
| HW-04.03 | 중단·재시도·민감 입력 정리 검증 | import 실패/완료 절차 |

### HW-05 · 기기 결제 표시·승인·EOA 서명 구현

- 원문 연결: 1, 4, 8번
- 산출물: NU signer 어댑터와 기기 승인 UI
- 완료 기준: 표시 내용과 서명 거래 일치; 거절·잘못된 chain ID·요청 변경 처리
- 선행 작업: HW-02, HW-03, PAY-01
- 결정 입력: D03, D08
- 앱/제품 연결: 유저 앱·키오스크·NU 화면
- 재사용 기준: 별도 NU 펌웨어·SDK signer 접점
- 검증 환경: 실제 NU 기기 + 대상 앱
- 연결 시나리오: J02, J03
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: payment.identify, payment.prepare, payment.result, wallet.sign.prepare, wallet.sign.result, request.cancel, API-034. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: K03, D01. [상태/복구 흐름](../specifications/screen-flows.md)
- 권한/트랜잭션 연결: API-034. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-07. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-07. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-02, IF-04. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-02, COMP-04. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: HW-05. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| HW-05.01 | 거래 파싱과 지원 호출의 표시 항목 구현 | 기기 승인 데이터 모델 |
| HW-05.02 | 버튼 승인/거절과 요청 digest를 결합 | 실기 signer 모듈 |
| HW-05.03 | 변조·다른 체인·미지원 호출을 검증 | 서명 일치/거절 시험 |

### HW-06 · 근접 측정 모듈·세션 승인 연결

- 원문 연결: 1, 4, 8번
- 산출물: 보유 모듈 연결과 근접 판정
- 완료 기준: 실제 보드/모듈 측정; 근접 결과를 요청/세션에 연결; 서명 이후 단절은 거래 취소로 표시하지 않음
- 선행 작업: HW-03, HW-05
- 결정 입력: D07, D08
- 앱/제품 연결: 유저 앱·키오스크·NU 화면
- 재사용 기준: 별도 NU 펌웨어·SDK signer 접점
- 검증 환경: 실제 NU 기기 + 대상 앱
- 연결 시나리오: J02, J03, J10
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: proximity.observe. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: K03. [상태/복구 흐름](../specifications/screen-flows.md)
- 기술 선택: TECH-09. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-09. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-03. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-03, COMP-18, COMP-19, COMP-20. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: HW-06. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| HW-06.01 | 외부 거리 모듈을 앱/키오스크 연결 경로에 통합 | 모듈 어댑터 |
| HW-06.02 | 측정 시각·대상·세션·신뢰 상태를 승인 요청과 연결 | 근접 증거 구조 |
| HW-06.03 | 이탈·측정 지연·서명 전후 단절 구분 | 거리 상태 전이 시나리오 |

### HW-07 · 서명·FOTA·녹음·찾기 자원 경합 처리

- 원문 연결: 1, 4, 8번
- 산출물: 기기 동작 우선순위·공존 제어
- 완료 기준: 동시 요청의 허용/거절 정의; 버퍼/메모리/전력·BLE 실측; 승인 내용 혼동 없음
- 선행 작업: HW-05, OTA-01, REC-02, FIND-01
- 결정 입력: D05, D07
- 앱/제품 연결: 유저 앱·키오스크·NU 화면
- 재사용 기준: 별도 NU 펌웨어·SDK signer 접점
- 검증 환경: 실제 NU 기기 + 대상 앱
- 연결 시나리오: J02, J07, J08, J09
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 화면 연결: D01, D02. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: device_jobs. [참조 DDL](../specifications/database/README.md)
- 기술 선택: TECH-06, TECH-11. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-06, VAL-11. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-01, IF-04, IF-07. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-01, COMP-04, COMP-07. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: HW-07. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)
- 결제·환불·반납 설계 사례: LC-11. [책임·상태 전이 설계](../specifications/commerce-lifecycle-design.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| HW-07.01 | 서명·녹음·찾기·FOTA 동시 요청 허용표 작성 | 기기 자원 우선순위표 |
| HW-07.02 | busy/중지/재개를 앱과 기기에 구현 | 동작 중재 모듈 |
| HW-07.03 | 동시 동작 시 버퍼·응답·전력 측정 | 공존 시험 결과 |
| HW-07.04 | 폰 소유 연결과 키오스크 결제 연결의 공존/전환 처리 | 실기 연결 전환·복귀 시험 |

## OTA — 펌웨어 업데이트

### OTA-01 · 업데이트 이미지·부트·복구 경로 검증

- 원문 연결: 2, 8, 10번
- 산출물: 실제 보드 부트/FOTA 기술 검증 기록
- 완료 기준: 목표 이미지 수용; 중단 뒤 재부팅 경로 확인; 키 보존 정책 제시
- 선행 작업: HW-01
- 결정 입력: D05
- 앱/제품 연결: 앱 업데이트·운영 배포 화면
- 재사용 기준: 신규 보드 부트/업데이트 경로
- 검증 환경: 실제 NU 기기 + 대상 앱
- 연결 시나리오: J07
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 화면 연결: U06. [상태/복구 흐름](../specifications/screen-flows.md)
- 기술 선택: TECH-06, TECH-08. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-06, VAL-08. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-05. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-05, COMP-15, COMP-16, COMP-17. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: OTA-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| OTA-01.01 | 부트로더·이미지 슬롯·키 영역 배치 확인 | 플래시 배치도 |
| OTA-01.02 | 이미지 검증·부팅 확인·실패 복구 구현 | 업데이트 부트 경로 |
| OTA-01.03 | 전원 중단과 부팅 실패 시험 | 복구 증거 |

### OTA-02 · 서명된 업데이트 패키지·배포 메타데이터

- 원문 연결: 2, 8, 10번
- 산출물: 펌웨어 패키징/버전 도구
- 완료 기준: 손상·허용하지 않은 이미지 거절; 호환 모델/버전 검사
- 선행 작업: OTA-01, RELEASE-01
- 결정 입력: D05
- 앱/제품 연결: 앱 업데이트·운영 배포 화면
- 재사용 기준: 신규 보드 부트/업데이트 경로
- 검증 환경: 실제 NU 기기 + 대상 앱
- 연결 시나리오: J07
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-048, API-049. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U06, O02. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: firmware_releases. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-048, API-049. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-08. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-08. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-05. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-05, COMP-15, COMP-16, COMP-17. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: OTA-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| OTA-02.01 | 모델·버전·길이·해시·호환 범위를 패키징 | 업데이트 manifest |
| OTA-02.02 | 릴리스 서명과 검증 키 관리 경로 연결 | 서명/검증 도구 |
| OTA-02.03 | 잘못된 대상·손상·금지 버전 거절 | 패키지 검증 샘플 |

### OTA-03 · 앱 FOTA 진행·재연결·실기 복구 구현

- 원문 연결: 2, 8, 10번
- 산출물: 앱 업데이트 화면과 기기 전송
- 완료 기준: 성공 후 버전 확인; 전원/통신 중단 시험; 지갑/설정 복구 정책 검증
- 선행 작업: OTA-02, HW-03, HW-04
- 결정 입력: D05
- 앱/제품 연결: 앱 업데이트·운영 배포 화면
- 재사용 기준: 신규 보드 부트/업데이트 경로
- 검증 환경: 실제 NU 기기 + 대상 앱
- 연결 시나리오: J07
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: fota.begin, fota.chunk, fota.finalize, fota.apply, fota.status. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U06, D02. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: device_jobs. [참조 DDL](../specifications/database/README.md)
- 기술 선택: TECH-08. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-08. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-05, IF-08. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-05, COMP-08, COMP-15, COMP-16, COMP-17. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: OTA-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| OTA-03.01 | 다운로드·검증·전송·적용 상태 UI 구현 | 앱 FOTA 화면 |
| OTA-03.02 | 전송 중단·재연결·재시작 처리 | 전송 세션 저장/복원 |
| OTA-03.03 | 업데이트 후 버전·키·설정 확인 | 실기 업데이트 기록 |

### OTA-04 · 앱·펌웨어·프로토콜·키 저장 호환 구현

- 원문 연결: 2, 8, 10번
- 산출물: 버전 호환표·업데이트 철회/차단 경로
- 완료 기준: 구버전 앱 접속 결과 정의; 잘못된 대상 차단; 저장 형식 변경/키 보존 시험
- 선행 작업: OTA-03, BASE-03, OPS-02
- 결정 입력: D05
- 앱/제품 연결: 앱 업데이트·운영 배포 화면
- 재사용 기준: 신규 보드 부트/업데이트 경로
- 검증 환경: 실제 NU 기기 + 대상 앱
- 연결 시나리오: J07
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: firmware.release.withdrawn, API-050, API-101. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U06, O02. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: firmware_releases. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-050, API-101. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-08. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-08, VAL-20. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-05. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-05, COMP-15, COMP-16, COMP-17. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: OTA-04. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| OTA-04.01 | 앱/펌웨어/BLE/키 저장 버전 행렬 작성 | 호환성 표 |
| OTA-04.02 | 업데이트 대상 제한·배포 중지·철회 반영 | 릴리스 제어 API |
| OTA-04.03 | 저장 형식 변경·구버전 앱 접근 검증 | 호환/마이그레이션 시나리오 |

## KEY — 실제 기기 패스키

### KEY-01 · 패스키 대상과 실제 기기 연결 경로 검증

- 원문 연결: 3, 8번
- 산출물: 호환성 표·실기 등록/인증 실험
- 완료 기준: 대상 OS/서비스/전송 명시; 자체 서명 승인과 표준 패스키 구분
- 선행 작업: HW-01, BASE-03
- 결정 입력: D06
- 앱/제품 연결: 유저 앱·합의한 인증 대상
- 재사용 기준: WebAuthn SDK는 참고; 기기 인증기는 별도
- 검증 환경: 실제 NU 기기 + 대상 앱
- 연결 시나리오: J08
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 화면 연결: U07. [상태/복구 흐름](../specifications/screen-flows.md)
- 기술 선택: TECH-10. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-10, VAL-21. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-06. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-06, COMP-24. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: KEY-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| KEY-01.01 | 대상 서비스·OS·브라우저·전송별 요구 확인 | 패스키 호환성 시험표 |
| KEY-01.02 | 실기에서 등록·인증 왕복 실험 | 전송/인증기 검증 결과 |
| KEY-01.03 | 필요 프로토콜·사용자 검증·저장 형식 확정 | 인증기 구현 명세 |

### KEY-02 · 기기 자격 생성·인증·사용자 검증 구현

- 원문 연결: 3, 8번
- 산출물: 패스키 펌웨어와 관리 화면
- 완료 기준: 대상 서비스 등록/인증 성공; 다른 서비스/잘못된 요청 거절
- 선행 작업: KEY-01, HW-02, HW-03
- 결정 입력: D06
- 앱/제품 연결: 유저 앱·합의한 인증 대상
- 재사용 기준: WebAuthn SDK는 참고; 기기 인증기는 별도
- 검증 환경: 실제 NU 기기 + 대상 앱
- 연결 시나리오: J08
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: credentials.list. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U07. [상태/복구 흐름](../specifications/screen-flows.md)
- 기술 선택: TECH-07, TECH-10. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-07, VAL-10, VAL-21. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-02, IF-06. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-02, COMP-06, COMP-24. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: KEY-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| KEY-02.01 | 자격 생성·RP 범위·식별자 저장 구현 | 자격 저장 모듈 |
| KEY-02.02 | challenge·사용자 존재/검증·서명 연결 | 등록/인증 처리기 |
| KEY-02.03 | 잘못된 RP·재전송·거절 요청 검증 | 실제 대상 서비스 인증 기록 |

### KEY-03 · 패스키 삭제·기기 분실/반납 처리

- 원문 연결: 3, 8번
- 산출물: 등록 해제·복구 안내/실행 경로
- 완료 기준: 반납 후 이전 자격 사용 불가; 합의한 복구 수단으로 접근 회복
- 선행 작업: KEY-02, STAMP-03
- 결정 입력: D06, D09
- 앱/제품 연결: 유저 앱·합의한 인증 대상
- 재사용 기준: WebAuthn SDK는 참고; 기기 인증기는 별도
- 검증 환경: 실제 NU 기기 + 대상 앱
- 연결 시나리오: J06, J08
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: credentials.delete. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U07. [상태/복구 흐름](../specifications/screen-flows.md)
- 기술 선택: TECH-10. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-10. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-06, IF-14. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-06, COMP-14, COMP-24. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: KEY-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| KEY-03.01 | 기기 자격 목록·삭제·서비스 해제 안내 구현 | 패스키 관리 화면 |
| KEY-03.02 | 분실/반납 전 대체 로그인 확보 흐름 연결 | 복구 안내와 확인 상태 |
| KEY-03.03 | 초기화 후 이전 자격 사용 불가 확인 | 반납/복구 시험 |

## REC — 녹음·전사·요약

### REC-01 · 마이크 캡처·버튼·기기 녹음 상태

- 원문 연결: 3, 4번
- 산출물: 실제 녹음 펌웨어
- 완료 기준: 시작/종료가 눈에 보임; 합의한 품질·버퍼/길이 측정
- 선행 작업: HW-01
- 결정 입력: D07
- 앱/제품 연결: 유저 앱 녹음 목록·상세·NU 상태
- 재사용 기준: 신규 기기 오디오·앱 저장/AI 연결
- 검증 환경: 실제 NU 기기 + 대상 앱
- 연결 시나리오: J09
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: recording.start, recording.stop. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U08, D02. [상태/복구 흐름](../specifications/screen-flows.md)
- 기술 선택: TECH-11. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-11. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-07. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-07. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: REC-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| REC-01.01 | 마이크 샘플·코덱 후보·버퍼 경계 구현 | 캡처 파이프라인 |
| REC-01.02 | 버튼 시작/종료와 표시등/화면 동기화 | 녹음 상태 머신 |
| REC-01.03 | 음질·전송량·버퍼 포화 측정 | 품질/자원 측정 기록 |

### REC-02 · BLE 음성 전송·모바일 저장/재생

- 원문 연결: 3, 4번
- 산출물: 음성 스트림·파일·목록/재생 UI
- 완료 기준: 실제 음성 재생; 단절·누락·저장공간 부족 표시; 파일과 세션 연결
- 선행 작업: REC-01, HW-03, APP-01
- 결정 입력: D07
- 앱/제품 연결: 유저 앱 녹음 목록·상세·NU 상태
- 재사용 기준: 신규 기기 오디오·앱 저장/AI 연결
- 검증 환경: 실제 NU 기기 + 대상 앱
- 연결 시나리오: J09
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: audio.frame, audio.flow-control. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U08. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: recordings. [참조 DDL](../specifications/database/README.md)
- 기술 선택: TECH-11. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-11. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-07, IF-08. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-07, COMP-08. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: REC-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| REC-02.01 | 프레임 번호·세션·포맷·종료 메타데이터 전송 | 음성 스트림 규약 |
| REC-02.02 | 모바일 수신·파일 마감·목록/재생 연결 | 오디오 파일 저장 모듈 |
| REC-02.03 | 끊김·누락·백그라운드·공간 부족 구분 | 완전/부분 녹음 상태 |

### REC-03 · 전사·AI 정리·내보내기·삭제 연결

- 원문 연결: 3, 4번
- 산출물: 녹음 상세·전사·요약 작업 서비스
- 완료 기준: 원음/전사/요약 연결; 실패 재시도·개인 접근·삭제 범위 검증
- 선행 작업: REC-02, AUTH-01
- 결정 입력: D07, D19
- 앱/제품 연결: 유저 앱 녹음 목록·상세·NU 상태
- 재사용 기준: 신규 기기 오디오·앱 저장/AI 연결
- 검증 환경: 실제 NU 기기 + 대상 앱
- 연결 시나리오: J09
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: recording.processing.changed, API-020, API-051, API-052, API-053, API-054, API-094, API-107. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U09. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: recordings, processing_jobs. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-020, API-051, API-052, API-053, API-054, API-094, API-107. [API 접근 표](../specifications/api-access-transactions.md)
- 보안 논리 저장 계약: ProtectedObject, ObjectPayload, DeletionTombstone. [Adapter 자원](../specifications/security-storage-contracts.json)
- 기술 선택: TECH-05, TECH-11, TECH-18. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-05, VAL-11, VAL-18. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-13. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-13. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: REC-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| REC-03.01 | 사용자 선택에 따라 전사 작업 요청 | 전사 큐/API |
| REC-03.02 | 전사 구간과 요약/할 일을 원음에 연결 | 녹음 상세/AI 결과 |
| REC-03.03 | 재시도·내보내기·원음/파생 데이터 삭제 | 기록 수명주기 API |

## FIND — 디바이스 찾기

### FIND-01 · 앱 찾기와 기기 반응 구현

- 원문 연결: 3, 8번
- 산출물: 찾기 화면·LED/부저 등 선택 출력
- 완료 기준: 등록된 실제 기기만 반응; 연결 불가를 위치 탐지로 표시하지 않음
- 선행 작업: HW-03, APP-01
- 결정 입력: D07
- 앱/제품 연결: 유저 앱 찾기·NU 응답
- 재사용 기준: 기존 거리 측정 검토
- 검증 환경: 실제 NU 기기 + 대상 앱
- 연결 시나리오: J10
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: find.start, find.stop. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U05, D02. [상태/복구 흐름](../specifications/screen-flows.md)
- 기술 선택: TECH-09. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-09. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-07. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-07. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: FIND-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| FIND-01.01 | 등록 기기 대상으로 찾기 명령 구성 | 찾기 요청 API |
| FIND-01.02 | LED/부저 응답과 중지 구현 | 기기 찾기 동작 |
| FIND-01.03 | 연결 불가·다른 기기·반복 요청 시험 | 찾기 결과 상태 |

### FIND-02 · 근접 안내·끊김 상태·기기 설정 통합

- 원문 연결: 3, 8번
- 산출물: 찾기/기기 상태 통합 화면
- 완료 기준: 실측 결과와 UI 일치; 권한/모듈 미지원 표시; 반납 기기 접근 차단
- 선행 작업: FIND-01, HW-06, STAMP-03
- 결정 입력: D07, D09
- 앱/제품 연결: 유저 앱 찾기·NU 응답
- 재사용 기준: 기존 거리 측정 검토
- 검증 환경: 실제 NU 기기 + 대상 앱
- 연결 시나리오: J10
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-013. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U05. [상태/복구 흐름](../specifications/screen-flows.md)
- 권한/트랜잭션 연결: API-013. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-09. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-09. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: FIND-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| FIND-02.01 | 연결 상태와 근접 측정 유효성을 표시 | 기기 상태 화면 |
| FIND-02.02 | 모듈/권한 미지원과 오래된 관측 구별 | 상태 안내 규칙 |
| FIND-02.03 | 반납·등록 해제 후 찾기 권한 철회 | 접근 해제 확인 |

## MPC — Cloud Wallet

### MPC-01 · Cloud Wallet 신뢰/복구·도입 방식 검증

- 원문 연결: 5, 6번
- 산출물: MPC 선택 근거·참여자/조각/복구 구조
- 완료 기준: 실제 MPC 제공 경로 확인; 단순 전체키 재조립과 구분; 로그인과 서명 권한 분리
- 선행 작업: BASE-02
- 결정 입력: D03, D04
- 앱/제품 연결: 유저 앱 지갑·복구
- 재사용 기준: 브리지 MPC를 대체품으로 간주하지 않음
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J01, J12
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 기술 선택: TECH-12. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-12. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-11. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-11. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: MPC-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| MPC-01.01 | 후보 제공 경로의 키 생성/서명/복구 확인 | 도입 비교와 검증 보고 |
| MPC-01.02 | 참여자·임계값·보관·탈취 모델 정의 | MPC 신뢰 경계도 |
| MPC-01.03 | 전체키 재조립 없는 서명 검증 | 선택한 구현의 검증 증거 |

### MPC-02 · 사용자 연결·분산 키 생성/주소 구성

- 원문 연결: 5, 6번
- 산출물: Cloud Wallet 생성/연결 서비스
- 완료 기준: 소셜 사용자와 올바른 지갑 연결; 반복 요청 중복 생성 방지; 조각 보관 경계 확인
- 선행 작업: MPC-01, AUTH-01, BASE-05
- 결정 입력: D04
- 앱/제품 연결: 유저 앱 지갑·복구
- 재사용 기준: 브리지 MPC를 대체품으로 간주하지 않음
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J01, J12
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-014. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U02. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: mpc_participants. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-014. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-12. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-12. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-11. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-11. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: MPC-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| MPC-02.01 | account와 wallet 생성 요청 멱등성 구현 | 지갑 생성 상태/고유키 |
| MPC-02.02 | 분산 키 생성과 참여자별 조각 보관 연결 | 실제 MPC 지갑 생성 |
| MPC-02.03 | 주소 확인·재로그인·실패 생성 복구 | 생성/연결 시나리오 |

### MPC-03 · MPC 송금 승인·서명·결과 UI 연결

- 원문 연결: 5, 6번
- 산출물: Cloud Wallet 실제 송금 흐름
- 완료 기준: 테스트넷 거래 확인; 불충분 참여/거절 때 서명 불가; 하드웨어 지갑과 표시 분리
- 선행 작업: MPC-02, APP-03, PAY-02
- 결정 입력: D04
- 앱/제품 연결: 유저 앱 지갑·복구
- 재사용 기준: 브리지 MPC를 대체품으로 간주하지 않음
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J01, J12
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-015. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U03. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: signing_sessions. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-015. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-12. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-12. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-10, IF-11. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-10, COMP-11, COMP-21, COMP-22, COMP-23, COMP-25, COMP-26. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: MPC-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| MPC-03.01 | 거래 확인 후 서명 세션 생성 | 사용자 승인과 digest |
| MPC-03.02 | 참여자 정책·서명 집계·검증 연결 | MPC signer 어댑터 |
| MPC-03.03 | 제출/결과를 공통 거래 UI에 연결 | Cloud 송금 실행 기록 |

### MPC-04 · 기기 변경·복구·회전/철회 구현

- 원문 연결: 5, 6번
- 산출물: 복구·키 수명주기 서비스/화면
- 완료 기준: 합의한 복구 시나리오 통과; 소셜 계정만 탈취한 경우 정책 검증; 이전 참여자 철회
- 선행 작업: MPC-03, AUTH-04
- 결정 입력: D04
- 앱/제품 연결: 유저 앱 지갑·복구
- 재사용 기준: 브리지 MPC를 대체품으로 간주하지 않음
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J01, J12
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-016. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U12. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: mpc_participants. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-016. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-12. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-12, VAL-22. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-11. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-11. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: MPC-04. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| MPC-04.01 | 새 폰/분실/참여자 교체별 복구 증명 정의 | 복구 요청·검증 API |
| MPC-04.02 | 선택 프로토콜에 맞게 조각 교체/재분산 구현 | 참여자 교체 절차 |
| MPC-04.03 | 기존 참여자 철회와 자산 접근 확인 | 복구 전후 시험 |

### MPC-05 · 참여자 장애·중단된 서명 세션 처리

- 원문 연결: 5, 6번
- 산출물: MPC 실패/재시도·복구 검증
- 완료 기준: 일부 참여자 장애 때 정책 준수; 중단된 세션 재사용·중복 송금 차단; 임계값 미달 서명 불가
- 선행 작업: MPC-03, PAY-02
- 결정 입력: D04
- 앱/제품 연결: 유저 앱 지갑·복구
- 재사용 기준: 브리지 MPC를 대체품으로 간주하지 않음
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J01, J12
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 화면 연결: U12. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: signing_sessions. [참조 DDL](../specifications/database/README.md)
- 기술 선택: TECH-12. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-12, VAL-22. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-11. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-11. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: MPC-05. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| MPC-05.01 | 참여자 timeout·거절·중단 상태 정의 | 서명 세션 상태 모델 |
| MPC-05.02 | 중단 세션 폐기와 새 세션 정책 구현 | 재시도 처리기 |
| MPC-05.03 | 임계값 미달·중복 콜백·오래된 응답 시험 | 장애 주입 시나리오 |

## TOKEN — 테스트 자산·가스

### TOKEN-01 · 환경·자산 주소/단위·가스 준비 정리

- 원문 연결: 11번
- 산출물: StableNet 자산 등록·시험 공급 경로
- 완료 기준: 8283 네트워크/배포 코드 일치 확인; 네이티브와 wrapped·더미 구분
- 선행 작업: BASE-04, BASE-05
- 결정 입력: D10
- 앱/제품 연결: 유저 앱 잔액·송수신
- 재사용 기준: stable-poc-contract ERC20Mock 기반 정리
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J13
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 저장 연결: assets. [참조 DDL](../specifications/database/README.md)
- 기술 선택: TECH-13. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-13. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-12. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-12. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: TOKEN-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| TOKEN-01.01 | 체인·네이티브·ERC20 자산 메타데이터 등록 | 자산 registry |
| TOKEN-01.02 | 시험 가스·토큰 수급과 충전 안내 연결 | 시험 자산 준비 절차 |
| TOKEN-01.03 | RPC/탐색기/배포 코드 일치 확인 | 환경 점검 결과 |

### TOKEN-02 · 더미 토큰·발행 권한·배포 기록 구현

- 원문 연결: 11번
- 산출물: 테스트 ERC20·배포/발행 도구
- 완료 기준: 표준 전송/잔액 확인; 무권한 mint·타인 burn 차단; 실제 USDC로 오표시하지 않음
- 선행 작업: TOKEN-01, RELEASE-01
- 결정 입력: D10
- 앱/제품 연결: 유저 앱 잔액·송수신
- 재사용 기준: stable-poc-contract ERC20Mock 기반 정리
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J13
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 기술 선택: TECH-13, TECH-17. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-13, VAL-17, VAL-23. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: TOKEN-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| TOKEN-02.01 | 더미 ERC20의 단위·발행/소각 권한 구현 | 테스트 토큰 계약 |
| TOKEN-02.02 | 배포·주소·ABI·버전 manifest 생성 | 재현 배포 도구 |
| TOKEN-02.03 | 전송·권한 거절·이벤트 검증 | 토큰 시험 기록 |

### TOKEN-03 · WKRC 네이티브·필요 wrapped 자산 연결

- 원문 연결: 11번
- 산출물: 잔액/송수신/가스 UI와 선택한 wrapping 흐름
- 완료 기준: 가스는 네이티브 잔액으로 확인; wrapped 포함 여부에 맞는 앱 검증
- 선행 작업: TOKEN-01, APP-02, PAY-02
- 결정 입력: D10
- 앱/제품 연결: 유저 앱 잔액·송수신
- 재사용 기준: stable-poc-contract ERC20Mock 기반 정리
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J13
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-009. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 권한/트랜잭션 연결: API-009. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-13. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-13. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: TOKEN-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| TOKEN-03.01 | 네이티브 WKRC와 wrapped 사용 여부 구분 | 자산 타입/주소 명세 |
| TOKEN-03.02 | 네이티브 송금·가스 잔액 UI 연결 | 가스/송금 경로 |
| TOKEN-03.03 | wrapped 채택 시 입출금과 잔액 연동 검증 | 자산별 수용 시나리오 |

## INDEX — Indexer·조회 계약

### INDEX-01 · Indexer 대상 체인·배포 계약 구성

- 원문 연결: 12번
- 산출물: 노드/저장/계약/이벤트 설정
- 완료 기준: 지정 RPC와 관측 블록 해시 대조; 체인별 데이터 혼합 방지
- 선행 작업: BASE-04, BASE-05
- 결정 입력: D10
- 앱/제품 연결: 거래 상세·백오피스 상태
- 재사용 기준: indexer-go/frontend 재사용·SDK 어댑터 수정
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J13
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 기술 선택: TECH-19. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-19. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: INDEX-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| INDEX-01.01 | 체인·배포 시작 블록·계약 목록 설정 | Indexer 실행 설정 |
| INDEX-01.02 | 블록/receipt/log 수집과 지속 저장 연결 | 수집 파이프라인 |
| INDEX-01.03 | 동일 높이 해시와 재시작 진행점 확인 | 노드 대조 기록 |

### INDEX-02 · SDK 조회 계약·금액/이벤트 변환 수정

- 원문 연결: 12번
- 산출물: RPC/GraphQL 어댑터와 조회 예제
- 완료 기준: 객체/배열·wrapper·receipt 필드 차이 해소; 오류와 빈 결과 구분
- 선행 작업: INDEX-01
- 결정 입력: D02
- 앱/제품 연결: 거래 상세·백오피스 상태
- 재사용 기준: indexer-go/frontend 재사용·SDK 어댑터 수정
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J13
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 기술 선택: TECH-13. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-13. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-12. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-12. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: INDEX-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| INDEX-02.01 | RPC params/응답 wrapper 불일치 수정 | SDK RPC 어댑터 |
| INDEX-02.02 | receipt/status/timestamp 조회 경로 수정 | GraphQL 질의/변환기 |
| INDEX-02.03 | 정수 금액·빈 결과·오류 fixture 검증 | 조회 계약 시험 |

### INDEX-03 · 진행 커서·backfill·중복/재구성 처리

- 원문 연결: 12번
- 산출물: 복구 가능한 이벤트 수집 흐름
- 완료 기준: 재연결 누락 복구; 같은 log 중복 반영 없음; 되돌림 정책 검증
- 선행 작업: INDEX-02
- 결정 입력: D08, D10
- 앱/제품 연결: 거래 상세·백오피스 상태
- 재사용 기준: indexer-go/frontend 재사용·SDK 어댑터 수정
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J13
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: payment.observation.changed. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 저장 연결: payment_evidence, chain_observations. [참조 DDL](../specifications/database/README.md)
- 기술 선택: TECH-19. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-19. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: INDEX-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| INDEX-03.01 | 커서·backfill 구간·재시작 체크포인트 구현 | 수집 진행 상태 |
| INDEX-03.02 | chain/tx/log 고유키와 정규 블록 검사 구현 | 중복/재구성 처리 |
| INDEX-03.03 | 중단 복구·되돌림·재적재 시험 | 수집 복구 기록 |

### INDEX-04 · 확장 계약군 ABI·이벤트·조회 등록

- 원문 연결: 12번
- 산출물: 영역별 이벤트 명세와 조회 API
- 완료 기준: 각 필수 계약의 실제 호출 결과 조회; 알려지지 않은 ABI/지연을 명시
- 선행 작업: INDEX-03, SMART-02, DEX-02, FX-02, PERP-04, STO-02, DID-02, X402-02
- 결정 입력: D10
- 앱/제품 연결: 거래 상세·백오피스 상태
- 재사용 기준: indexer-go/frontend 재사용·SDK 어댑터 수정
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J13
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 저장 연결: chain_projections. [참조 DDL](../specifications/database/README.md)
- 기술 선택: TECH-19. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-19. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-12. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-12. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: INDEX-04. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| INDEX-04.01 | 필수 계약 ABI와 이벤트 버전 등록 | 이벤트 registry |
| INDEX-04.02 | 각 상품의 앱 조회 모델 구성 | 자산/포지션/자격 조회 API |
| INDEX-04.03 | 실제 호출 이벤트와 모델 대조 | 영역별 이벤트 샘플 |

### INDEX-05 · 앱 거래 상세·탐색기·상태 관측 연결

- 원문 연결: 12번
- 산출물: 거래/이벤트 상세와 상태 화면
- 완료 기준: 모든 앱 거래에서 식별자 추적; 표시상 confirmed와 최종 확정 구분
- 선행 작업: INDEX-04, OPS-03
- 결정 입력: D08, D10
- 앱/제품 연결: 거래 상세·백오피스 상태
- 재사용 기준: indexer-go/frontend 재사용·SDK 어댑터 수정
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J13
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-019, API-089. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U11. [상태/복구 흐름](../specifications/screen-flows.md)
- 권한/트랜잭션 연결: API-019, API-089. [API 접근 표](../specifications/api-access-transactions.md)
- 착수·완료 증거: INDEX-05. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| INDEX-05.01 | 거래·receipt·log·확정 정보 조회 연결 | 거래 상세 API |
| INDEX-05.02 | 대기·관측·확정·실패·재관측 UI 구현 | 거래 진행 화면 |
| INDEX-05.03 | 앱별 transaction ID로 탐색기 추적 | 조회 연결 기록 |

### INDEX-06 · 관측 변경을 원장·혜택·여행에 전파

- 원문 연결: 12번
- 산출물: 재구성/재처리 후 보정 흐름
- 완료 기준: 주문/매출/스탬프/추천 입력 보정과 이력; 반복 처리 결과 동일; 사용자 상태 갱신
- 선행 작업: INDEX-03, PAY-04, STAMP-01, TRIP-03
- 결정 입력: D08, D09, D17
- 앱/제품 연결: 거래 상세·백오피스 상태
- 재사용 기준: indexer-go/frontend 재사용·SDK 어댑터 수정
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J03, J05, J11, J13, J18
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: payment.observation.changed, payment.acceptance.changed, travel.evidence.changed. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: O03. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: outbox, inbox, chain_observations. [참조 DDL](../specifications/database/README.md)
- 기술 선택: TECH-19. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-19. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-12. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-12. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: INDEX-06. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)
- 결제·환불·반납 설계 사례: LC-03, LC-05, LC-06. [책임·상태 전이 설계](../specifications/commerce-lifecycle-design.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| INDEX-06.01 | 관측 추가/변경/철회에 버전·원인 부여 | 변경 이벤트 계약 |
| INDEX-06.02 | 주문·매출·스탬프·여행 소비자 갱신 | 재처리 가능한 projection |
| INDEX-06.03 | 반복 전달·순서 변경·보정 후 집계 검증 | 종단 보정 시험 |

## PAY — EOA 결제·주문 대사

### PAY-01 · EOA 견적·수수료·서명 요청 구성

- 원문 연결: 1, 7, 9, 12번
- 산출물: 토큰 전송·nonce/가스·승인 데이터 계약
- 완료 기준: 구매대금/가스 분리; 잘못된 체인·금액·수취 거절; 견적 유효성 규칙
- 선행 작업: BASE-03, TOKEN-01
- 결정 입력: D08
- 앱/제품 연결: 유저 앱·키오스크·매장·백오피스
- 재사용 기준: 기존 거래 SDK·Indexer + 신규 업무 상태
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J03
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-017, API-032, API-034. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U03. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: payment_attempts, transaction_intents. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-017, API-032, API-034. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-13. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-13. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: PAY-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| PAY-01.01 | 주문 금액을 자산 단위로 고정하고 기한 부여 | 결제 견적 |
| PAY-01.02 | nonce·가스·수취·호출 데이터 구성 | 서명 요청 payload |
| PAY-01.03 | 잘못된 체인·부족 잔액·기한 만료 처리 | 견적/승인 검증 |

### PAY-02 · 서명 거래 제출·재시도·상태 추적

- 원문 연결: 1, 7, 9, 12번
- 산출물: 중계 API·트랜잭션 시도 저장
- 완료 기준: 중계에 고객 키 불필요; 제출 불명확 때 재조회; 재시도와 신규 거래 구분
- 선행 작업: PAY-01, BASE-05
- 결정 입력: D08
- 앱/제품 연결: 유저 앱·키오스크·매장·백오피스
- 재사용 기준: 기존 거래 SDK·Indexer + 신규 업무 상태
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J03
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-018, API-020. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U03. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: transaction_submissions. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-018, API-020. [API 접근 표](../specifications/api-access-transactions.md)
- 보안 논리 저장 계약: CapabilityReservation, AccessChallenge, AccessGrantLineage, ProtectedIssuanceResponse, SenderProofReplay, AuthorizationGate, ApprovalBinding, SignatureProvenance, SignedPayloadObject, SubmissionDispatch, DispatchAttempt, AddressNonceTrack, SourceExecutionGate, ObservationApplication. [Adapter 자원](../specifications/security-storage-contracts.json)
- 착수·완료 증거: PAY-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)
- 결제·환불·반납 설계 사례: LC-04. [책임·상태 전이 설계](../specifications/commerce-lifecycle-design.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| PAY-02.01 | 서명 bytes 검증·tx hash·시도 상태 저장 | 제출 API/원장 |
| PAY-02.02 | 중계 timeout 시 tx hash로 조회 | 불명확 제출 복구 |
| PAY-02.03 | 동일 제출 재시도와 대체 거래 분리 | 중복/nonce 처리 시험 |

### PAY-03 · 주문 견적·결제 시도와 거래 연결

- 원문 연결: 1, 7, 9, 12번
- 산출물: 주문-결제 시도 원장
- 완료 기준: 메뉴 가격 스냅샷 보관; 거래/이벤트를 여러 주문에 재사용 불가
- 선행 작업: PAY-02, SHOP-02
- 결정 입력: D08
- 앱/제품 연결: 유저 앱·키오스크·매장·백오피스
- 재사용 기준: 기존 거래 SDK·Indexer + 신규 업무 상태
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J03
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-029, API-032, API-104, API-105. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: K02. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: orders, payment_attempts. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-029, API-032, API-104, API-105. [API 접근 표](../specifications/api-access-transactions.md)
- 남은 보안 연결 입력: GAP-04. [설계 결과](../specifications/security-integration-inputs.json)
- 보안 수용 시나리오: SEC-04-01, SEC-04-02, SEC-04-03, SEC-04-04, SEC-04-05, SEC-04-06. [흐름·완료 조건](../specifications/security-integration-design.md)
- 보안 논리 저장 계약: ReceiptEligibility, ReceiptClaim, ReceiptOwnership. [Adapter 자원](../specifications/security-storage-contracts.json)
- 착수·완료 증거: PAY-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| PAY-03.01 | 가격·메뉴·수취 주소를 주문에 snapshot | 주문 생성 API |
| PAY-03.02 | attempt ID에 견적·지갑·서명 요청 연결 | 결제 시도 원장 |
| PAY-03.03 | 하나의 지급 증거를 한 주문에만 배정 | 대사 고유 제약 |

### PAY-04 · 입금 검증·확정·매출 대사 구현

- 원문 연결: 1, 7, 9, 12번
- 산출물: 결제 관측→주문 상태 전이
- 완료 기준: 체인/토큰/수취/금액/receipt/확정 정책 검증; 중복/오입금/지연 분리; 사전 승인 거래·지급 주체·attempt 귀속 검증; 스마트 계정은 외부 tx.from만으로 소유자 판정하지 않음
- 선행 작업: PAY-03, INDEX-03, TOKEN-02
- 결정 입력: D08
- 앱/제품 연결: 유저 앱·키오스크·매장·백오피스
- 재사용 기준: 기존 거래 SDK·Indexer + 신규 업무 상태
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J03
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: payment.acceptance.changed, API-035, API-104, API-105. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 저장 연결: payment_evidence, payment_allocations. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-035, API-104, API-105. [API 접근 표](../specifications/api-access-transactions.md)
- 남은 보안 연결 입력: GAP-04. [설계 결과](../specifications/security-integration-inputs.json)
- 보안 수용 시나리오: SEC-04-01, SEC-04-02, SEC-04-03, SEC-04-04, SEC-04-05, SEC-04-06. [흐름·완료 조건](../specifications/security-integration-design.md)
- 보안 논리 저장 계약: ReceiptEligibility, ReceiptClaim, ReceiptOwnership, PendingBenefitEntitlement, ApprovalBinding, SignatureProvenance, SignedPayloadObject, SubmissionDispatch, DispatchAttempt, AddressNonceTrack, SourceExecutionGate, ObservationApplication. [Adapter 자원](../specifications/security-storage-contracts.json)
- 기술 선택: TECH-04. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-04. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-12. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-12. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: PAY-04. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)
- 결제·환불·반납 설계 사례: LC-01, LC-02, LC-03, LC-07. [책임·상태 전이 설계](../specifications/commerce-lifecycle-design.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| PAY-04.01 | receipt와 자산/수취/금액/확정 검증 | 입금 검증기 |
| PAY-04.02 | 정상·부족/초과·기한 뒤 입금 분리 | 예외 입금 처리 |
| PAY-04.03 | 주문·매출 반영과 중복 이벤트 차단 | 대사 결과/감사 로그 |
| PAY-04.04 | 사전 승인 거래·지급 주체·attempt 귀속 검증 | 타 고객 동일 금액 거래 재사용 거절 시험 |

### PAY-05 · 실제 NU 키오스크 결제 수직 연결

- 원문 연결: 1, 7, 9, 12번
- 산출물: 기기 승인부터 주문 완료까지 실행 기록
- 완료 기준: 실기 승인·거절·거리 이탈·앱 재시작 시험; 정상 결제 1건이 매출 1회
- 선행 작업: PAY-04, HW-05, HW-06, SHOP-03
- 결정 입력: D08
- 앱/제품 연결: 유저 앱·키오스크·매장·백오피스
- 재사용 기준: 기존 거래 SDK·Indexer + 신규 업무 상태
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J03
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 화면 연결: K03. [상태/복구 흐름](../specifications/screen-flows.md)
- 구현 인터페이스: IF-04. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-04. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: PAY-05. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| PAY-05.01 | 키오스크 주문에서 NU 승인 요청 연결 | 실기 결제 수직 경로 |
| PAY-05.02 | 서명·중계·Indexer·주문 결과 동기화 | 결제 종단 실행 |
| PAY-05.03 | 거절·거리 이탈·앱 재시작 재현 | 카페 결제 시나리오 증거 |

## SHOP — 키오스크·매장 운영

### SHOP-01 · RN 태블릿·매장 가입·로그인·관리 모드

- 원문 연결: 9번
- 산출물: 설치 앱·가게 프로필/소속
- 완료 기준: Google/Apple 실제 로그인; 관리 복귀 인증; 다른 매장 차단
- 선행 작업: AUTH-02, AUTH-03, BASE-03
- 결정 입력: D01
- 앱/제품 연결: RN 키오스크·사장님 화면
- 재사용 기준: 기존 가맹점 화면 참고; PG 시뮬레이터 분리
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J04, J05
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-021, API-022, API-023, API-024, API-025. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: K01. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: stores. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-021, API-022, API-023, API-024, API-025. [API 접근 표](../specifications/api-access-transactions.md)
- 남은 보안 연결 입력: GAP-02. [설계 결과](../specifications/security-integration-inputs.json)
- 보안 수용 시나리오: SEC-02-01, SEC-02-02, SEC-02-03, SEC-02-04, SEC-02-05, SEC-02-06. [흐름·완료 조건](../specifications/security-integration-design.md)
- 보안 논리 저장 계약: ScopedCapability. [Adapter 자원](../specifications/security-storage-contracts.json)
- 기술 선택: TECH-01. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-01. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-08. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-08. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: SHOP-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| SHOP-01.01 | RN 태블릿 빌드와 로그인/매장 선택 구현 | 키오스크 설치 앱 |
| SHOP-01.02 | 고객 모드와 관리 모드 권한 분리 | 모드 전환/잠금 |
| SHOP-01.03 | 소셜 가입·다른 매장 접근 거절 확인 | 매장 인증 시험 |

### SHOP-02 · 메뉴·가격·품절·장바구니·주문 구현

- 원문 연결: 9번
- 산출물: 매장 메뉴/주문 API와 RN 화면
- 완료 기준: 메뉴 변경이 과거 주문을 바꾸지 않음; 주문 중복/품절 처리
- 선행 작업: SHOP-01
- 결정 입력: D08
- 앱/제품 연결: RN 키오스크·사장님 화면
- 재사용 기준: 기존 가맹점 화면 참고; PG 시뮬레이터 분리
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J04, J05
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-026, API-027, API-029, API-031, API-092. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: K02. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: menu_items, orders, order_lines. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-026, API-027, API-029, API-031, API-092. [API 접근 표](../specifications/api-access-transactions.md)
- 착수·완료 증거: SHOP-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| SHOP-02.01 | 메뉴/옵션/가격/품절 CRUD 구현 | 메뉴 관리 UI/API |
| SHOP-02.02 | 장바구니·수량·주문 가격 검증 | 주문 화면/API |
| SHOP-02.03 | 중복 주문·품절 변경·과거 가격 확인 | 주문 경계 시험 |

### SHOP-03 · 키오스크 결제 화면·기기 세션 연결

- 원문 연결: 9번
- 산출물: 주문→기기 승인 요청 화면
- 완료 기준: 매장·금액 확인; 승인 대기/거절/미확정 상태 구분; 중복 탭 처리
- 선행 작업: SHOP-02, HW-03, PAY-03
- 결정 입력: D08
- 앱/제품 연결: RN 키오스크·사장님 화면
- 재사용 기준: 기존 가맹점 화면 참고; PG 시뮬레이터 분리
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J04, J05
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-030, API-033, API-108, API-109, API-110. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: K03. [상태/복구 흐름](../specifications/screen-flows.md)
- 권한/트랜잭션 연결: API-030, API-033, API-108, API-109, API-110. [API 접근 표](../specifications/api-access-transactions.md)
- 남은 보안 연결 입력: GAP-02. [설계 결과](../specifications/security-integration-inputs.json)
- 보안 수용 시나리오: SEC-02-01, SEC-02-02, SEC-02-03, SEC-02-04, SEC-02-05, SEC-02-06. [흐름·완료 조건](../specifications/security-integration-design.md)
- 보안 논리 저장 계약: ScopedCapability. [Adapter 자원](../specifications/security-storage-contracts.json)
- 착수·완료 증거: SHOP-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)
- 결제·환불·반납 설계 사례: LC-02, LC-03. [책임·상태 전이 설계](../specifications/commerce-lifecycle-design.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| SHOP-03.01 | 결제 수단·금액·연결 대상 확인 UI 구현 | 결제 시작 화면 |
| SHOP-03.02 | 승인 대기·거절·끊김·기한 만료 표시 | 키오스크 세션 상태 |
| SHOP-03.03 | 앱 재시작 후 기존 주문 복원 | 중복 탭/복원 시험 |
| SHOP-03.04 | 임시 결제 세션 종료 후 키오스크 권한 폐기 | 소유 등록 유지·세션 종료 시험 |

### SHOP-04 · 환불 요청·승인·서명·입금 추적

- 원문 연결: 9번
- 산출물: 환불 UI/원장/별도 거래
- 완료 기준: 원지급 유지; 누적 한도 확인; 반납 주소·수수료·부분 환불 정책 반영
- 선행 작업: PAY-04, APP-03
- 결정 입력: D08, D09
- 앱/제품 연결: RN 키오스크·사장님 화면
- 재사용 기준: 기존 가맹점 화면 참고; PG 시뮬레이터 분리
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J04, J05
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: refund.state.changed, API-036, API-037, API-038, API-092. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: K05. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: refund_balances, refunds, refund_reservations. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-036, API-037, API-038, API-092. [API 접근 표](../specifications/api-access-transactions.md)
- 보안 논리 저장 계약: ApprovalBinding, SignatureProvenance, SignedPayloadObject, SubmissionDispatch, DispatchAttempt, AddressNonceTrack, SourceExecutionGate, ObservationApplication. [Adapter 자원](../specifications/security-storage-contracts.json)
- 착수·완료 증거: SHOP-04. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)
- 결제·환불·반납 설계 사례: LC-04, LC-05, LC-06, LC-07. [책임·상태 전이 설계](../specifications/commerce-lifecycle-design.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| SHOP-04.01 | 원주문 기준 환불 가능액 계산 | 환불 요청/잔여 한도 |
| SHOP-04.02 | 권한 확인·환불 주소 검증·서명 연결 | 환불 승인 흐름 |
| SHOP-04.03 | 별도 지급 확정 후 잔여액/혜택 보정 | 부분/중복 환불 시험 |

### SHOP-05 · 매출·수취·환불·정산 대조 화면

- 원문 연결: 9번
- 산출물: 일자/매장별 집계·차이 조회
- 완료 기준: 지갑 잔액을 매출로 취급하지 않음; 주문과 체인 원장 대조 가능
- 선행 작업: SHOP-04, PAY-04
- 결정 입력: D08
- 앱/제품 연결: RN 키오스크·사장님 화면
- 재사용 기준: 기존 가맹점 화면 참고; PG 시뮬레이터 분리
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J04, J05
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-039, API-040, API-041, API-093. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: K06. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: settlements, settlement_revisions. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-039, API-040, API-041, API-093. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-03. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-03. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: SHOP-05. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)
- 결제·환불·반납 설계 사례: LC-05. [책임·상태 전이 설계](../specifications/commerce-lifecycle-design.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| SHOP-05.01 | 기간별 주문·입금·환불 집계 | 매출/수취 대시보드 |
| SHOP-05.02 | 지갑 입출금과 주문 원장 대조 | 정산 차이 목록 |
| SHOP-05.03 | 마감 기록·재개방/보정·내보내기 구현 | 정산 처리 이력 |

### SHOP-06 · 수취 주소 변경·환불 서명 권한 분리

- 원문 연결: 9번
- 산출물: 중요 매장 자금 행위의 권한/승인 UI
- 완료 기준: 메뉴 수정 권한만으로 주소/환불 변경 불가; 업무 승인과 실제 서명 분리; 감사 이력
- 선행 작업: SHOP-01, SHOP-04, OPS-01
- 결정 입력: D08
- 앱/제품 연결: RN 키오스크·사장님 화면
- 재사용 기준: 기존 가맹점 화면 참고; PG 시뮬레이터 분리
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J04, J05
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-008, API-028. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: K04. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: wallet_bindings, recipient_configurations. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-008, API-028. [API 접근 표](../specifications/api-access-transactions.md)
- 착수·완료 증거: SHOP-06. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| SHOP-06.01 | 수취 주소 등록/변경 시 소유·권한 확인 | 주소 관리 승인 흐름 |
| SHOP-06.02 | 메뉴 권한과 자금 승인/서명 권한 구분 | 권한 행렬/API |
| SHOP-06.03 | 기존 주문 주소 snapshot 유지·변경 감사 | 중요 설정 시험 |

## STAMP — 결제 스탬프·대여

### STAMP-01 · 스탬프 발급·사용·취소 원장

- 원문 연결: 3, 7, 8, 10, 14번
- 산출물: 혜택 API·발급 규칙
- 완료 기준: 확정 결제 1회 적립; 중복 사용 차단; 환불 정책 적용
- 선행 작업: PAY-04, AUTH-01
- 결정 입력: D09
- 앱/제품 연결: 유저 앱·NU·운영 화면
- 재사용 기준: 신규 혜택·대여 모델
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J06, J11
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: benefit.ledger.changed, API-043, API-044. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U10. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: benefit_entries, benefit_redemptions. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-043, API-044. [API 접근 표](../specifications/api-access-transactions.md)
- 보안 논리 저장 계약: PendingBenefitEntitlement. [Adapter 자원](../specifications/security-storage-contracts.json)
- 기술 선택: TECH-04. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-04. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: STAMP-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| STAMP-01.01 | 발급/사용/취소 정책과 고유키 정의 | 혜택 원장 스키마 |
| STAMP-01.02 | 확정 결제 이벤트에 적립 연결 | 스탬프 지급 처리기 |
| STAMP-01.03 | 중복 사용·부분 환불·취소 보정 처리 | 혜택 상태 시험 |

### STAMP-02 · 앱·기기 스탬프 표시/사용 연결

- 원문 연결: 3, 7, 8, 10, 14번
- 산출물: 카페 패스포트 UI·기기 표시
- 완료 기준: 두 화면이 같은 원장 결과 표시; 기기 교체/오프라인 뒤 동기화
- 선행 작업: STAMP-01, HW-03, APP-04
- 결정 입력: D09
- 앱/제품 연결: 유저 앱·NU·운영 화면
- 재사용 기준: 신규 혜택·대여 모델
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J06, J11
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: passport.sync, API-043. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U10, D02. [상태/복구 흐름](../specifications/screen-flows.md)
- 권한/트랜잭션 연결: API-043. [API 접근 표](../specifications/api-access-transactions.md)
- 보안 논리 저장 계약: PendingBenefitEntitlement. [Adapter 자원](../specifications/security-storage-contracts.json)
- 구현 인터페이스: IF-07. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-07. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: STAMP-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| STAMP-02.01 | 카페별 스탬프/사용 가능 혜택 화면 구현 | 패스포트 UI |
| STAMP-02.02 | 기기 표시 데이터 동기화 | 기기 스탬프 명령 |
| STAMP-02.03 | 오프라인·기기 변경 후 최신 원장 복원 | 동기화 검증 |

### STAMP-03 · 대여·반납·잔액 회수·기기 해제

- 원문 연결: 3, 7, 8, 10, 14번
- 산출물: 운영/앱 대여 수명주기
- 완료 기준: 신규 여행 지갑은 가스를 고려해 회수; import 지갑은 외부 접근 확인 후 기기 사본 삭제; 키 삭제 전 상태 확인; 이전 사용자 접근 해제
- 선행 작업: HW-04, PAY-02, AUTH-01
- 결정 입력: D03, D09
- 앱/제품 연결: 유저 앱·NU·운영 화면
- 재사용 기준: 신규 혜택·대여 모델
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J06, J11
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-045, API-046, API-091. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U24, O01. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: rentals. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-045, API-046, API-091. [API 접근 표](../specifications/api-access-transactions.md)
- 구현 인터페이스: IF-14. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-14. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: STAMP-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)
- 결제·환불·반납 설계 사례: LC-08, LC-09, LC-11, LC-12. [책임·상태 전이 설계](../specifications/commerce-lifecycle-design.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| STAMP-03.01 | 기기 할당·사용자 등록·반납 요청 구현 | 대여 원장 |
| STAMP-03.02 | 지갑 종류별 회수/외부 접근 확인 후 초기화·해제 연결 | 반납 단계 화면 |
| STAMP-03.03 | 반납 보류와 운영자 확인 경로 구성 | 대여/반납 기록 |
| STAMP-03.04 | 여행용 신규 지갑의 반환 주소·자산 회수 확인 | 신규 여행 지갑 반납 경로 |
| STAMP-03.05 | import 지갑의 외부 접근 확인 후 기기 사본만 삭제 | 기존 지갑 잔액/권한 보존 반납 경로 |

### STAMP-04 · 반납 시 미확정 거래·자격·계정 권한 정리

- 원문 연결: 3, 7, 8, 10, 14번
- 산출물: 재대여 전 검사·완료/보류 상태
- 완료 기준: 미확정 거래 처리 정책; 기기 신규 자격/위임과 기존 권한을 구분해 패스키/스마트 계정/이전 BLE 권한 정리; 다음 대여자 격리
- 선행 작업: STAMP-03, KEY-03, SMART-03, PAY-04
- 결정 입력: D03, D06, D09, D11
- 앱/제품 연결: 유저 앱·NU·운영 화면
- 재사용 기준: 신규 혜택·대여 모델
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J06, J11
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: device.reset.prepare, device.reset.confirm, device.binding.revoked, API-047, API-091. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: D01, O01, U04, U05, U24. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: return_checks, reset_clearances. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-047, API-091. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-10. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-10. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-06, IF-14. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-06, COMP-14, COMP-24. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: STAMP-04. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)
- 결제·환불·반납 설계 사례: LC-08, LC-09, LC-10, LC-11, LC-12. [책임·상태 전이 설계](../specifications/commerce-lifecycle-design.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| STAMP-04.01 | 미확정 거래·잔액·자격·위임 검사 | 반납 사전 점검 결과 |
| STAMP-04.02 | 기기에서 등록한 자격/위임과 기존 권한을 구분해 정리 | 반납 cleanup 절차 |
| STAMP-04.03 | 초기화 확인 후 재대여 허용 | 사용자 간 격리 시험 |
| STAMP-04.04 | 기기 신규 자격/위임과 기존 지갑의 권한을 구별해 정리 | 등록 출처별 권한 cleanup 목록 |

## OPS — 운영 백오피스

### OPS-01 · 운영자 권한·감사·관리 화면 골격

- 원문 연결: 10, 15번
- 산출물: 백오피스 인증·행위 로그
- 완료 기준: 최소 역할별 접근; 고객 키/녹음/위치 기본 열람 금지; 변경 이력 보존
- 선행 작업: AUTH-01, BASE-05
- 결정 입력: D19
- 앱/제품 연결: 백오피스
- 재사용 기준: 기존 웹 화면·탐색기 + 신규 운영 데이터
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J19
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-088. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: O01, O04. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: audit_events. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-088. [API 접근 표](../specifications/api-access-transactions.md)
- 남은 보안 연결 입력: GAP-03. [설계 결과](../specifications/security-integration-inputs.json)
- 보안 수용 시나리오: SEC-03-01, SEC-03-02, SEC-03-03, SEC-03-04, SEC-03-05, SEC-03-06. [흐름·완료 조건](../specifications/security-integration-design.md)
- 보안 논리 저장 계약: TrustPrincipal, TrustGrantRevision, TrustChangeRequest. [Adapter 자원](../specifications/security-storage-contracts.json)
- 기술 선택: TECH-03. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-03. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-09. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-09. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: OPS-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| OPS-01.01 | 운영 계정·역할·권한 검사 구현 | 백오피스 로그인/권한 |
| OPS-01.02 | 가맹점·기기·거래 조회 탐색 구성 | 관리 화면 골격 |
| OPS-01.03 | 변경자·대상·이전/이후 값 감사 기록 | 감사 조회 화면 |

### OPS-02 · 가맹점·대여·펌웨어 릴리스 운영 연결

- 원문 연결: 10, 15번
- 산출물: 운영 조회·등록·정책 변경 UI
- 완료 기준: 실제 매장/기기/업데이트 상태 연결; 중요 설정 변경 추적
- 선행 작업: OPS-01, STAMP-03, OTA-02
- 결정 입력: D05, D09
- 앱/제품 연결: 백오피스
- 재사용 기준: 기존 웹 화면·탐색기 + 신규 운영 데이터
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J19
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-049, API-050, API-099, API-100, API-101. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: O01, O02. [상태/복구 흐름](../specifications/screen-flows.md)
- 권한/트랜잭션 연결: API-049, API-050, API-099, API-100, API-101. [API 접근 표](../specifications/api-access-transactions.md)
- 착수·완료 증거: OPS-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| OPS-02.01 | 가맹점 활성/정지와 대여 배정 관리 | 매장/기기 관리 |
| OPS-02.02 | 호환 대상별 FOTA 배포·중지 제어 | 릴리스 운영 화면 |
| OPS-02.03 | 설정 변경과 앱/기기 실제 상태 대조 | 운영 변경 기록 |

### OPS-03 · 결제 예외·Indexer 지연·대사 지원

- 원문 연결: 10, 15번
- 산출물: 거래 재조회/차이 처리 도구
- 완료 기준: 운영자가 임의 온체인 성공 생성 불가; 재조회로 중복 매출 없음
- 선행 작업: OPS-01, PAY-04, INDEX-03
- 결정 입력: D08, D19
- 앱/제품 연결: 백오피스
- 재사용 기준: 기존 웹 화면·탐색기 + 신규 운영 데이터
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J05, J13, J19
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-086, API-087, API-089. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: O03. [상태/복구 흐름](../specifications/screen-flows.md)
- 권한/트랜잭션 연결: API-086, API-087, API-089. [API 접근 표](../specifications/api-access-transactions.md)
- 착수·완료 증거: OPS-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)
- 결제·환불·반납 설계 사례: LC-01, LC-06, LC-07. [책임·상태 전이 설계](../specifications/commerce-lifecycle-design.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| OPS-03.01 | 미확정·오입금·Indexer 지연 큐 구성 | 결제 예외 화면 |
| OPS-03.02 | 원천 재조회·재처리·처리 메모 구현 | 운영 조치 API |
| OPS-03.03 | 반복 조치 시 중복 매출/혜택 방지 | 대사 운영 시험 |

## SMART — 스마트 계정

### SMART-01 · EOA 이후 계정 전환·서명 모델 설계

- 원문 연결: 11번
- 산출물: 주소/자산/권한·복구 전환 명세
- 완료 기준: 키 가져오기 유지; 자산 자동 이전으로 가정하지 않음; 대상 버전 결정
- 선행 작업: PAY-01, MPC-01, BASE-04
- 결정 입력: D11
- 앱/제품 연결: 유저 앱 계정 전환·거래
- 재사용 기준: Kernel·SDK·Bundler 재사용 검증
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J12
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-055, API-107. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U13. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: wallets. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-055, API-107. [API 접근 표](../specifications/api-access-transactions.md)
- 남은 보안 연결 입력: GAP-05. [설계 결과](../specifications/security-integration-inputs.json)
- 보안 수용 시나리오: SEC-05-01, SEC-05-02, SEC-05-03, SEC-05-04, SEC-05-05, SEC-05-06. [흐름·완료 조건](../specifications/security-integration-design.md)
- 보안 논리 저장 계약: ProtectedObject. [Adapter 자원](../specifications/security-storage-contracts.json)
- 기술 선택: TECH-14. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-14. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: SMART-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| SMART-01.01 | 계정 생성/위임 방식과 signer 후보 비교 | 전환 설계 |
| SMART-01.02 | 기존 EOA 자산·주소·권한 변화 정의 | 전환/복구 화면 명세 |
| SMART-01.03 | 선택 버전의 해시/서명/권한 경계 확정 | AA 호환 명세 |

### SMART-02 · 계정 계약·SDK·Bundler 실행 연결

- 원문 연결: 11번
- 산출물: 배포/해시·서명·실행 경로
- 완료 기준: SDK와 EntryPoint 결과 일치; 개별 UserOperation 성공/실패 구분
- 선행 작업: SMART-01, TOKEN-02, RELEASE-01
- 결정 입력: D11
- 앱/제품 연결: 유저 앱 계정 전환·거래
- 재사용 기준: Kernel·SDK·Bundler 재사용 검증
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J12
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-056. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U13. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: transaction_intents. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-056. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-14. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-14. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-12. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-12. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: SMART-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| SMART-02.01 | 계정·EntryPoint·Bundler 버전 맞춤 | 실행 구성 manifest |
| SMART-02.02 | UserOperation 구성·서명·제출 연결 | AA 실행 어댑터 |
| SMART-02.03 | 온체인 해시 일치와 개별 실행 결과 검증 | AA 성공/실패 시험 |

### SMART-03 · 앱 계정 전환·실제 서명·결과 연결

- 원문 연결: 11번
- 산출물: EOA/스마트 계정 선택·전환 UI
- 완료 기준: 합의한 signer로 거래; 자산/주소/권한 변화 표시; 복구 시나리오
- 선행 작업: SMART-02, HW-05, MPC-03, APP-03
- 결정 입력: D11
- 앱/제품 연결: 유저 앱 계정 전환·거래
- 재사용 기준: Kernel·SDK·Bundler 재사용 검증
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J06, J12
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-055, API-107. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U13. [상태/복구 흐름](../specifications/screen-flows.md)
- 권한/트랜잭션 연결: API-055, API-107. [API 접근 표](../specifications/api-access-transactions.md)
- 남은 보안 연결 입력: GAP-05. [설계 결과](../specifications/security-integration-inputs.json)
- 보안 수용 시나리오: SEC-05-01, SEC-05-02, SEC-05-03, SEC-05-04, SEC-05-05, SEC-05-06. [흐름·완료 조건](../specifications/security-integration-design.md)
- 보안 논리 저장 계약: ProtectedObject. [Adapter 자원](../specifications/security-storage-contracts.json)
- 기술 선택: TECH-14. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-14. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-10. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-10, COMP-21, COMP-22, COMP-23, COMP-25, COMP-26. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: SMART-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| SMART-03.01 | EOA/스마트 계정 선택과 전환 안내 구현 | 계정 화면 |
| SMART-03.02 | 선택 signer의 승인·실행 연결 | 스마트 계정 앱 거래 |
| SMART-03.03 | 복구·기기 반납 시 권한 변화 검증 | 계정 수명주기 시험 |

## DEX — DeFi·DEX

### DEX-01 · DeFi 대표 기능·pool·유동성 명세

- 원문 연결: 11, 13번
- 산출물: 상품/토큰/가격·실행 API 명세
- 완료 기준: 필수 DeFi 행위 지정; V2 가상 reserve·V3 실제 경로 구분
- 선행 작업: BASE-04, TOKEN-01
- 결정 입력: D12
- 앱/제품 연결: 유저 앱 거래·유동성
- 재사용 기준: DEXIntegration·V3 quote 코드 선별
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J14
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 화면 연결: U14. [상태/복구 흐름](../specifications/screen-flows.md)
- 기술 선택: TECH-15. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-15. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: DEX-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| DEX-01.01 | 지원 swap·유동성 입출금 행위 명세 | DeFi 상품 카드 |
| DEX-01.02 | 토큰 쌍·pool·초기 유동성·수수료 지정 | 시장 설정 |
| DEX-01.03 | 기존 V2/V3 실행 경로와 필요한 수정 연결 | DEX 재사용 변경표 |

### DEX-02 · pool·견적·승인·swap/유동성 실행

- 원문 연결: 11, 13번
- 산출물: 계약/DEX 서비스·실제 실행 경로
- 완료 기준: 만료/슬리피지·잔액/allowance 처리; 고정 가상값을 실제 quote로 사용하지 않음
- 선행 작업: DEX-01, TOKEN-02, PAY-02
- 결정 입력: D12
- 앱/제품 연결: 유저 앱 거래·유동성
- 재사용 기준: DEXIntegration·V3 quote 코드 선별
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J14
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-057. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U14. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: market_quotes. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-057. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-15. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-15. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: DEX-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| DEX-02.01 | 실제 pool 상태로 quote 생성 | 견적 서비스 |
| DEX-02.02 | allowance·최소 수취·기한·실행 구현 | swap/유동성 어댑터 |
| DEX-02.03 | 실패·가격 변동·유동성 부족 검증 | DEX 거래 시험 |

### DEX-03 · 앱 DeFi·DEX 거래/유동성·이력 연결

- 원문 연결: 11, 13번
- 산출물: 사용자 입력→서명→실행→결과 UI
- 완료 기준: 필수 swap/입출금 행위 실제 테스트넷 수행; 실패 때 상태/자산 대조
- 선행 작업: DEX-02, APP-03, INDEX-03
- 결정 입력: D12
- 앱/제품 연결: 유저 앱 거래·유동성
- 재사용 기준: DEXIntegration·V3 quote 코드 선별
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J14
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-058, API-059. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U14. [상태/복구 흐름](../specifications/screen-flows.md)
- 권한/트랜잭션 연결: API-058, API-059. [API 접근 표](../specifications/api-access-transactions.md)
- 착수·완료 증거: DEX-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| DEX-03.01 | 자산 선택·견적·승인·swap 화면 구현 | DEX 앱 화면 |
| DEX-03.02 | 유동성 추가/회수·보유분 조회 연결 | LP 앱 화면 |
| DEX-03.03 | Indexer 결과로 잔액/이력 갱신 | DEX 종단 실행 기록 |

## FX — FX 서비스

### FX-01 · FX 통화 쌍·상품·가격·규칙 정의

- 원문 연결: 11, 13번
- 산출물: FX 모델과 앱/계약 명세
- 완료 기준: spot/파생상품 범위 구별; 시험 자산·견적/정밀도·가격 출처 지정
- 선행 작업: DEX-01
- 결정 입력: D12
- 앱/제품 연결: 유저 앱 FX
- 재사용 기준: 일반 swap은 일부 기반; FX 상품 미정
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J14
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 화면 연결: U15. [상태/복구 흐름](../specifications/screen-flows.md)
- 기술 선택: TECH-15. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-15. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: FX-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| FX-01.01 | 통화 쌍·현물/파생 성격·시험 자산 정의 | FX 상품 카드 |
| FX-01.02 | 가격 출처·갱신·정밀도·수수료 정의 | FX quote 명세 |
| FX-01.03 | DEX와 공유/별도 구현 경계 지정 | FX 작업 경계표 |

### FX-02 · FX 계약/거래 서비스 연결

- 원문 연결: 11, 13번
- 산출물: 선택 통화 쌍 실행 API/이벤트
- 완료 기준: 가격 만료·잘못된 단위·최소 수취 처리; 테스트넷 실행 확인
- 선행 작업: FX-01, DEX-02
- 결정 입력: D12
- 앱/제품 연결: 유저 앱 FX
- 재사용 기준: 일반 swap은 일부 기반; FX 상품 미정
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J14
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-057. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U15. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: market_quotes. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-057. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-15. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-15. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: FX-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| FX-02.01 | 통화 금액·단위·유효기한 검증 | FX 견적 API |
| FX-02.02 | 선택 모델의 실행 계약/서비스 연결 | FX 실행 어댑터 |
| FX-02.03 | 가격 만료·최소 수취·이벤트 검증 | FX 경계 시험 |

### FX-03 · 앱 FX 견적·교환·기록 연결

- 원문 연결: 11, 13번
- 산출물: 통화 선택·견적·승인·결과 UI
- 완료 기준: 일반 swap과 FX 표시 규칙 일치; 실제 실행 금액/수수료 기록
- 선행 작업: FX-02, APP-03, INDEX-03
- 결정 입력: D12
- 앱/제품 연결: 유저 앱 FX
- 재사용 기준: 일반 swap은 일부 기반; FX 상품 미정
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J14
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-058. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U15. [상태/복구 흐름](../specifications/screen-flows.md)
- 권한/트랜잭션 연결: API-058. [API 접근 표](../specifications/api-access-transactions.md)
- 착수·완료 증거: FX-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| FX-03.01 | 통화 입력·환산·수수료 확인 UI 구현 | FX 화면 |
| FX-03.02 | 지갑 승인·실행·실패 복구 연결 | FX 거래 흐름 |
| FX-03.03 | 실제 체결 금액/가격을 이력에 표시 | FX 영수증 |

## PERP — Perpetual 서비스

### PERP-01 · 시장·위험·가격/청산 명세

- 원문 연결: 11, 13번
- 산출물: 시장 파라미터·정산/실패 시나리오
- 완료 기준: 고정 가격/TODO 대체 작업 목록; 증거금·펀딩·청산 기준 명시
- 선행 작업: BASE-04
- 결정 입력: D13
- 앱/제품 연결: 유저 앱 시장·포지션
- 재사용 기준: 기존 계약의 고정 가격·미완성 처리 보완
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J15
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 화면 연결: U16. [상태/복구 흐름](../specifications/screen-flows.md)
- 기술 선택: TECH-15. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-15. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: PERP-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| PERP-01.01 | 시장·담보·레버리지·손익 계산 경계 정의 | 시장 파라미터 |
| PERP-01.02 | 가격·펀딩·청산·중단 정책 명세 | 위험/실행 규칙 |
| PERP-01.03 | 기존 TODO·고정 가격 대체 위치 지정 | 계약/서비스 수정표 |

### PERP-02 · 가격 공급·오라클 상태 처리

- 원문 연결: 11, 13번
- 산출물: 테스트 가격/오라클 어댑터
- 완료 기준: 가격 시점·이상/지연 상태 검증; 고정값 경로가 완료 증거에 섞이지 않음
- 선행 작업: PERP-01, BASE-05
- 결정 입력: D13
- 앱/제품 연결: 유저 앱 시장·포지션
- 재사용 기준: 기존 계약의 고정 가격·미완성 처리 보완
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J15
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 화면 연결: U16. [상태/복구 흐름](../specifications/screen-flows.md)
- 기술 선택: TECH-15. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-15. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: PERP-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| PERP-02.01 | 가격 소스와 업데이트 권한 구성 | 오라클 어댑터 |
| PERP-02.02 | 가격 시점·이상치·stale 검사 구현 | 가격 유효성 모듈 |
| PERP-02.03 | 지연·급변·공급 중단 시 동작 검증 | 가격 장애 시나리오 |

### PERP-03 · 증거금·포지션 개설/축소/종료 구현

- 원문 연결: 11, 13번
- 산출물: 계약과 주문/포지션 서비스
- 완료 기준: 담보 이동·손익·포지션 상태 일치; 초과/잘못된 주문 거절
- 선행 작업: PERP-02, TOKEN-02, PAY-02
- 결정 입력: D13
- 앱/제품 연결: 유저 앱 시장·포지션
- 재사용 기준: 기존 계약의 고정 가격·미완성 처리 보완
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J15
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-061. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U16. [상태/복구 흐름](../specifications/screen-flows.md)
- 권한/트랜잭션 연결: API-061. [API 접근 표](../specifications/api-access-transactions.md)
- 착수·완료 증거: PERP-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| PERP-03.01 | 담보 입금/출금과 사용 가능액 구현 | 증거금 계약/API |
| PERP-03.02 | 포지션 개설·축소·종료 실행 | 포지션 명령 |
| PERP-03.03 | 실현/미실현 손익·잔액 대조 | 포지션 계산 시험 |

### PERP-04 · 펀딩·청산·장애 시 상태 복구 구현

- 원문 연결: 11, 13번
- 산출물: 펀딩/청산 실행·기록
- 완료 기준: 항상 0/불가인 stub 해소; 실제 잔액/포지션 변화와 경계 조건 검증
- 선행 작업: PERP-03
- 결정 입력: D13
- 앱/제품 연결: 유저 앱 시장·포지션
- 재사용 기준: 기존 계약의 고정 가격·미완성 처리 보완
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J15
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 화면 연결: U16. [상태/복구 흐름](../specifications/screen-flows.md)
- 기술 선택: TECH-15. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-15. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: PERP-04. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| PERP-04.01 | 펀딩 누적·적용·계정 반영 구현 | 펀딩 계산/이벤트 |
| PERP-04.02 | 청산 조건·집행·잔여 담보 처리 | 청산 경로 |
| PERP-04.03 | 경계 가격·반복 실행·중단 복구 검증 | 펀딩/청산 시험 |

### PERP-05 · 앱 시장·주문·포지션·청산 이력 연결

- 원문 연결: 11, 13번
- 산출물: Perpetual 전체 앱 흐름
- 완료 기준: 실제 주문·손익·펀딩·청산 결과 조회; 오류/가격 중단 표시
- 선행 작업: PERP-04, APP-03, INDEX-03, PERP-06
- 결정 입력: D13
- 앱/제품 연결: 유저 앱 시장·포지션
- 재사용 기준: 기존 계약의 고정 가격·미완성 처리 보완
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J15
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-060, API-061, API-062. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U16. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: chain_projections. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-060, API-061, API-062. [API 접근 표](../specifications/api-access-transactions.md)
- 착수·완료 증거: PERP-05. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| PERP-05.01 | 시장·담보·주문 입력과 승인 화면 구현 | Perp 주문 UI |
| PERP-05.02 | 포지션·손익·펀딩·청산 내역 연결 | 포지션 상세 |
| PERP-05.03 | 가격 지연·주문 실패·복구 상태 표시 | Perp 앱 실행 기록 |

### PERP-06 · 가격·펀딩·청산 실행 서비스 운영 연결

- 원문 연결: 11, 13번
- 산출물: 필요 keeper/스케줄·상태·장애 감지
- 완료 기준: 합의한 실행 주체가 실제 호출; 누락·반복 실행 처리; 가격 장애 시 정책 적용
- 선행 작업: PERP-04, BASE-05
- 결정 입력: D13, D19
- 앱/제품 연결: 유저 앱 시장·포지션
- 재사용 기준: 기존 계약의 고정 가격·미완성 처리 보완
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J15
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 화면 연결: U16. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: keeper_jobs. [참조 DDL](../specifications/database/README.md)
- 기술 선택: TECH-15. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-15. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: PERP-06. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| PERP-06.01 | 가격 갱신·펀딩·청산 실행 주체 구성 | keeper/작업 설정 |
| PERP-06.02 | 작업 고유키·재시도·실행 nonce 관리 | 실행 서비스 |
| PERP-06.03 | 중단 감지·재시작·누락 구간 처리 | 실행 운영 기록 |

## STO — STO

### STO-01 · 테스트 발행물·자격·권리 수명주기 정의

- 원문 연결: 11번
- 산출물: STO 대표 사용자 행위/계약 명세
- 완료 기준: 발행/보유/전송 조건 지정; 실물 권리와 시험 데이터 표시 구분
- 선행 작업: BASE-02
- 결정 입력: D14
- 앱/제품 연결: 유저 앱 발행물·보유·전송
- 재사용 기준: 전용 구현 미확인
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J16
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 화면 연결: U17. [상태/복구 흐름](../specifications/screen-flows.md)
- 기술 선택: TECH-16. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-16. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: STO-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| STO-01.01 | 시험 발행물과 표시할 권리 정의 | STO 상품 카드 |
| STO-01.02 | 발행자·보유자·전송 대상 자격 정의 | 권한/전송 규칙 |
| STO-01.03 | 발행/조회/전송/제한 앱 흐름 지정 | STO 수용 시나리오 |

### STO-02 · 발행·보유·전송 제한 계약/서비스 구현

- 원문 연결: 11번
- 산출물: 테스트넷 계약·조회·이벤트
- 완료 기준: 허용/거절 시나리오 검증; 권한 없는 발급/전송 차단
- 선행 작업: STO-01, TOKEN-01, RELEASE-01
- 결정 입력: D14
- 앱/제품 연결: 유저 앱 발행물·보유·전송
- 재사용 기준: 전용 구현 미확인
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J16
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-064. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U17. [상태/복구 흐름](../specifications/screen-flows.md)
- 권한/트랜잭션 연결: API-064. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-16. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-16. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: STO-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| STO-02.01 | 발행·공급·보유·자격 검사 구현 | 테스트 계약 |
| STO-02.02 | 허용/거절 전송과 관리 명령 연결 | STO 서비스 |
| STO-02.03 | 권한 없음·자격 철회·이벤트 검증 | STO 계약 시험 |

### STO-03 · 앱 STO 자격·보유·거래 결과 연결

- 원문 연결: 11번
- 산출물: 발행물 상세/행위/결과 UI
- 완료 기준: 합의한 대표 동작 실제 실행; 제한 사유·자격과 지갑 통제 분리
- 선행 작업: STO-02, APP-03, AUTH-01, INDEX-03
- 결정 입력: D14
- 앱/제품 연결: 유저 앱 발행물·보유·전송
- 재사용 기준: 전용 구현 미확인
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J16
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-063, API-064, API-102. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U17. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: chain_projections. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-063, API-064, API-102. [API 접근 표](../specifications/api-access-transactions.md)
- 착수·완료 증거: STO-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| STO-03.01 | 발행물 설명·자격·보유량 화면 구현 | STO 앱 화면 |
| STO-03.02 | 허용된 동작의 승인/실행 연결 | STO 거래 흐름 |
| STO-03.03 | 거절 사유와 실제 상태 갱신 | STO 앱 시험 |

## DID — DID

### DID-01 · 식별자·발급자·자격·철회 모델 정의

- 원문 연결: 11번
- 산출물: DID/자격 명세와 데이터 공개 범위
- 완료 기준: 검증자/발급자 구분; 체인·오프체인 저장과 삭제 경계 명시
- 선행 작업: BASE-02
- 결정 입력: D15
- 앱/제품 연결: 유저 앱 자격·검증
- 재사용 기준: 전용 구현 미확인
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J16
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 화면 연결: U18. [상태/복구 흐름](../specifications/screen-flows.md)
- 남은 보안 연결 입력: GAP-03. [설계 결과](../specifications/security-integration-inputs.json)
- 보안 수용 시나리오: SEC-03-01, SEC-03-02, SEC-03-03, SEC-03-04, SEC-03-05, SEC-03-06. [흐름·완료 조건](../specifications/security-integration-design.md)
- 보안 논리 저장 계약: TrustPrincipal, TrustGrantRevision. [Adapter 자원](../specifications/security-storage-contracts.json)
- 기술 선택: TECH-16. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-16. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: DID-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| DID-01.01 | 식별자·발급자·검증자와 표준 선택 | DID/자격 모델 |
| DID-01.02 | 공개 데이터·개인 데이터 보관 분리 | 데이터 공개 명세 |
| DID-01.03 | 발급·제시·만료·철회 시나리오 지정 | 자격 수명주기 |

### DID-02 · 발급·검증·철회 서비스/필요 계약 구현

- 원문 연결: 11번
- 산출물: 테스트 자격 수명주기 API/이벤트
- 완료 기준: 잘못된 발급자·만료·철회 거절; 재검증 결과 일치
- 선행 작업: DID-01, AUTH-01, RELEASE-01
- 결정 입력: D15
- 앱/제품 연결: 유저 앱 자격·검증
- 재사용 기준: 전용 구현 미확인
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J16
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: credential.status.changed, API-065, API-068, API-069. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U18. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: credential_metadata, credential_status_history. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-065, API-068, API-069. [API 접근 표](../specifications/api-access-transactions.md)
- 남은 보안 연결 입력: GAP-03. [설계 결과](../specifications/security-integration-inputs.json)
- 보안 수용 시나리오: SEC-03-01, SEC-03-02, SEC-03-03, SEC-03-04, SEC-03-05, SEC-03-06. [흐름·완료 조건](../specifications/security-integration-design.md)
- 보안 논리 저장 계약: TrustPrincipal, TrustGrantRevision. [Adapter 자원](../specifications/security-storage-contracts.json)
- 기술 선택: TECH-16. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-16. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: DID-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| DID-02.01 | 발급 권한과 소유자 결합 검증 구현 | 자격 발급 API |
| DID-02.02 | 서명·발급자·만료·철회 상태 검증 | 검증 API |
| DID-02.03 | 필요 계약/상태 registry와 연결 | 자격 검증 시험 |

### DID-03 · 앱 자격 관리·제시·검증 결과 연결

- 원문 연결: 11번
- 산출물: DID 화면/검증 흐름
- 완료 기준: 실제 발급/철회 결과 표시; 불필요한 개인 정보 공개 방지
- 선행 작업: DID-02, APP-01
- 결정 입력: D15
- 앱/제품 연결: 유저 앱 자격·검증
- 재사용 기준: 전용 구현 미확인
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J16
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-066, API-067. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U18. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: credential_metadata. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-066, API-067. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-16. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-16. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: DID-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| DID-03.01 | 자격 목록·상세·사용 동의 화면 구현 | DID 앱 화면 |
| DID-03.02 | 요청 대상에 필요한 자격만 제시 | 제시/검증 흐름 |
| DID-03.03 | 만료·철회·다른 지갑 서명 거절 표시 | DID 앱 시험 |

## X402 — x402

### X402-01 · 유료 자원·지불 규약·검증자 명세

- 원문 연결: 11번
- 산출물: HTTP 결제 시나리오/지원 조합
- 완료 기준: StableNet 자산·버전·facilitator 지원 확인; 실패/재시도 규칙
- 선행 작업: BASE-03, TOKEN-01
- 결정 입력: D16
- 앱/제품 연결: 유저 앱 유료 서비스 이용
- 재사용 기준: 전용 구현 미확인
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J17
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 화면 연결: U19. [상태/복구 흐름](../specifications/screen-flows.md)
- 기술 선택: TECH-17. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-17, VAL-23. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: X402-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| X402-01.01 | 유료 자원·가격·지원 자산/버전 명세 | HTTP 결제 상품 카드 |
| X402-01.02 | 검증자·결제 수단·지원 네트워크 확인 | 호환성 검증 결과 |
| X402-01.03 | 동일 요청 재시도·응답 실패 정책 정의 | 과금/권한 규칙 |

### X402-02 · 유료 요청·승인·지급 검증 연결

- 원문 연결: 11번
- 산출물: 자원 서버·결제 검증·필요 계약
- 완료 기준: 미지불/위조/다른 요청 증명 거절; 재요청 중복 과금 방지
- 선행 작업: X402-01, PAY-02, TOKEN-02
- 결정 입력: D16
- 앱/제품 연결: 유저 앱 유료 서비스 이용
- 재사용 기준: 전용 구현 미확인
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J17
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-071. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U19. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: paid_resource_requests, entitlements. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-071. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-17. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-17. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-12. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-12. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: X402-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| X402-02.01 | 지불 요구와 요청별 지불 증명 처리 | 유료 자원 서버 |
| X402-02.02 | 검증·지급·이용권 상태 분리 | 지불 검증 서비스 |
| X402-02.03 | 위조·다른 자원·재요청·응답 실패 시험 | x402 통합 시험 |

### X402-03 · 앱 유료 자원 이용·결제 결과 연결

- 원문 연결: 11번
- 산출물: 가격 안내·승인·응답·이력 UI
- 완료 기준: 실제 HTTP 지불 흐름 통과; 지불 후 응답 실패 복구 정책 적용
- 선행 작업: X402-02, APP-03, INDEX-03
- 결정 입력: D16
- 앱/제품 연결: 유저 앱 유료 서비스 이용
- 재사용 기준: 전용 구현 미확인
- 검증 환경: StableNet 8283 + 대상 앱/서비스
- 연결 시나리오: J17
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-070, API-071, API-072. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U19. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: paid_resource_requests. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-070, API-071, API-072. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-17. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-17. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: X402-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| X402-03.01 | 유료 자원·가격·사용 지갑 표시 | 유료 이용 화면 |
| X402-03.02 | 승인·지급·자원 응답 연결 | 앱 유료 요청 모듈 |
| X402-03.03 | 지불 후 응답 재조회와 이력 연결 | 유료 요청 복구 기록 |

## TRIP — 장소·후기·발자취

### TRIP-01 · 장소 데이터·위치 권한·맛집 검색 연결

- 원문 연결: 7, 14번
- 산출물: 지도/목록/상세·위치 API
- 완료 기준: 출처·갱신 시점 확인; 권한 거절/대체 검색·위치 오류 처리
- 선행 작업: APP-01, BASE-05
- 결정 입력: D17
- 앱/제품 연결: 유저 앱 여행 모드
- 재사용 기준: 결제 데이터 재사용·여행 도메인 신규
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J18
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-073. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U20. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: places. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-073. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-18. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-18. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: TRIP-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| TRIP-01.01 | 장소 제공자와 store/place ID 매핑 | 장소 어댑터 |
| TRIP-01.02 | 위치·거리·검색/목록/상세 구현 | 맛집 화면 |
| TRIP-01.03 | 위치 거절·수동 지역·오래된 정보 표시 | 장소 검색 시험 |

### TRIP-02 · 후기 작성·구매 출처·노출 정책 구현

- 원문 연결: 7, 14번
- 산출물: 후기 API/화면
- 완료 기준: 본인 작성·수정/삭제; 검증 구매·일반 후기 구별; 운영 처리 기록
- 선행 작업: TRIP-01, AUTH-01, PAY-04
- 결정 입력: D17
- 앱/제품 연결: 유저 앱 여행 모드
- 재사용 기준: 결제 데이터 재사용·여행 도메인 신규
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J18
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-074, API-075, API-076. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U20. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: reviews. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-074, API-075, API-076. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-18. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-18. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: TRIP-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| TRIP-02.01 | 후기 생성·수정·삭제·첨부 정책 구현 | 후기 API |
| TRIP-02.02 | 결제 소유·출처와 구매 표식 연결 | 후기 증거 검증 |
| TRIP-02.03 | 신고/숨김·환불/관측 변경 처리 | 후기 노출 이력 |

### TRIP-03 · 결제/위치 발자취·여행 모드 구현

- 원문 연결: 7, 14번
- 산출물: 타임라인·언어/시간대·필터 UI
- 완료 기준: 실제 구매/테스트 결제/위치 방문 출처 구분; 동의/삭제/권한 처리
- 선행 작업: TRIP-01, APP-04
- 결정 입력: D01, D17
- 앱/제품 연결: 유저 앱 여행 모드
- 재사용 기준: 결제 데이터 재사용·여행 도메인 신규
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J18
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-077, API-078, API-079, API-097. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U21. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: trips. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-077, API-078, API-079, API-097. [API 접근 표](../specifications/api-access-transactions.md)
- 착수·완료 증거: TRIP-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| TRIP-03.01 | 여행 시작/종료·언어·시간대 설정 | 여행 모드 |
| TRIP-03.02 | 결제와 위치 방문을 구분해 타임라인 표시 | 발자취 화면 |
| TRIP-03.03 | 권한 변경·수동 기록·삭제 반영 | 여행 기록 시험 |

### TRIP-04 · 결제·장소 식별 및 개인 데이터 수명주기 연결

- 원문 연결: 7, 14번
- 산출물: 원천/동의/삭제·내보내기 매핑
- 완료 기준: 결제와 장소 연결 검증; 실제/시험 출처 구분; 녹음/후기/위치 삭제가 추천 입력에 반영
- 선행 작업: TRIP-03, TRIP-02, REC-03, AUTH-04
- 결정 입력: D07, D17, D18, D19
- 앱/제품 연결: 유저 앱 여행 모드
- 재사용 기준: 결제 데이터 재사용·여행 도메인 신규
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J09, J18
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: travel.evidence.changed, privacy.request.changed, API-054, API-084, API-085. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U09, U21, U25. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: consents, privacy_requests, data_lineage, store_place_links, travel_evidence. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-054, API-084, API-085. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-18. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-18. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-13. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-13. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: TRIP-04. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| TRIP-04.01 | 주문·매장·장소·위치·후기 출처 연결 | 데이터 계보 |
| TRIP-04.02 | 수집/AI 사용 동의와 보관 정책 적용 | 동의/보관 API |
| TRIP-04.03 | 원음·위치·후기 삭제/내보내기 전파 | 개인 데이터 처리 기록 |

## AI — 추천 코스·챌린지

### AI-01 · 추천 입력·코스 제약·평가 표본 정의

- 원문 연결: 14번
- 산출물: 추천 규칙·데이터 연결·평가 시나리오
- 완료 기준: 위치/결제/후기 입력 근거; 이동/영업시간·부족 데이터 처리 기준
- 선행 작업: TRIP-02, TRIP-03
- 결정 입력: D18
- 앱/제품 연결: 유저 앱 여행 코스·도전
- 재사용 기준: 신규 추천/검증 서비스
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J18
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 화면 연결: U22. [상태/복구 흐름](../specifications/screen-flows.md)
- 기술 선택: TECH-18. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-18. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-13. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-13. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: AI-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| AI-01.01 | 추천 입력과 사용 가능 출처 지정 | 추천 입력 스키마 |
| AI-01.02 | 시간·이동·영업시간·예산 제약 정의 | 코스 검증 규칙 |
| AI-01.03 | 충분/부족/오염 데이터 평가 표본 구성 | 추천 평가 데이터 |

### AI-02 · AI 코스 생성·근거·수정/저장 구현

- 원문 연결: 14번
- 산출물: 추천 서비스와 앱 코스 UI
- 완료 기준: 실제 출처 연결; 없는 장소/근거와 불가능한 코스 처리; 저장/수정 가능
- 선행 작업: AI-01
- 결정 입력: D18
- 앱/제품 연결: 유저 앱 여행 코스·도전
- 재사용 기준: 신규 추천/검증 서비스
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J18
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-080, API-081, API-095, API-107. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U22. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: processing_jobs, itineraries. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-080, API-081, API-095, API-107. [API 접근 표](../specifications/api-access-transactions.md)
- 보안 논리 저장 계약: ProtectedObject, ObjectPayload. [Adapter 자원](../specifications/security-storage-contracts.json)
- 기술 선택: TECH-05, TECH-18. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-05, VAL-18. [검증 카드](technology-validation-plan.md)
- 구현 인터페이스: IF-13. [구성요소 경계](../specifications/implementation-interfaces.md)
- 버전 호환 판정: COMP-13. [호환 행렬](../specifications/compatibility-matrix.md)
- 착수·완료 증거: AI-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| AI-02.01 | 후보 검색·AI 생성·구조화 검증 연결 | 추천 서비스 |
| AI-02.02 | 장소·근거·이동 순서를 검증해 표시 | 추천 코스 화면 |
| AI-02.03 | 수정·저장·실패/데이터 부족 대안 구현 | 코스 수명주기 |

### AI-03 · 따라하기 챌린지·방문/결제 증명 구현

- 원문 연결: 14번
- 산출물: 참여·진행·발도장 원장/UI
- 완료 기준: 방문/결제/완료 구별; 재시도·부정 참여·중복 보상 처리
- 선행 작업: AI-02, STAMP-01, TRIP-03, INDEX-06
- 결정 입력: D09, D18
- 앱/제품 연결: 유저 앱 여행 코스·도전
- 재사용 기준: 신규 추천/검증 서비스
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J18
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 연결 명세: API-082, API-083, API-096. [규칙과 카탈로그](../specifications/interface-contracts.md)
- 화면 연결: U23. [상태/복구 흐름](../specifications/screen-flows.md)
- 저장 연결: participations, challenge_entries. [참조 DDL](../specifications/database/README.md)
- 권한/트랜잭션 연결: API-082, API-083, API-096. [API 접근 표](../specifications/api-access-transactions.md)
- 기술 선택: TECH-18. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-18. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: AI-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| AI-03.01 | 참여·단계·방문/구매 증거 규칙 구현 | 챌린지 모델 |
| AI-03.02 | 진행·발도장·중복 보상 차단 연결 | 챌린지 앱/원장 |
| AI-03.03 | 환불·증거 철회·위치 불확실 시 보정 | 챌린지 재검증 시험 |

## VERIFY — 전체 수용·복구 검증

### VERIFY-01 · 두 지갑·소셜·대여 수용 시험

- 원문 연결: 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15번
- 산출물: 실기/Cloud/계정 변경 통합 증거
- 완료 기준: 키 가져오기/신규·MPC 복구·계정 연결·반납 후 접근 검증
- 선행 작업: PAY-05, MPC-04, AUTH-04, STAMP-03, SMART-03, STAMP-04, MPC-05, BASE-06
- 결정 입력: D03, D04, D09, D19
- 앱/제품 연결: 전체 앱·실기·테스트넷
- 재사용 기준: 영역별 결과를 합친 실제 통합 시험
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J19
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 착수·완료 증거: VERIFY-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| VERIFY-01.01 | 계정/지갑/대여 시험 데이터 준비 | 시험 사용자·기기 조합 |
| VERIFY-01.02 | 신규/import/MPC·복구·반납 시나리오 실행 | 통합 실행 증거 |
| VERIFY-01.03 | 불일치 원인과 수정 후 재검증 기록 | 인수 판정 |

### VERIFY-02 · 기기 전체 기능·업데이트 공존 시험

- 원문 연결: 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15번
- 산출물: 실제 보드·앱 회귀 결과
- 완료 기준: FOTA/패스키/녹음/찾기/스탬프 함께 검증; 자원·배터리·단절 수치 기록
- 선행 작업: OTA-03, KEY-03, REC-03, FIND-02, STAMP-02, HW-07, OTA-04
- 결정 입력: D05, D06, D07, D19
- 앱/제품 연결: 전체 앱·실기·테스트넷
- 재사용 기준: 영역별 결과를 합친 실제 통합 시험
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J19
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 착수·완료 증거: VERIFY-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| VERIFY-02.01 | 기기 기능/동시 동작 시험 순서 구성 | 실기 시험 매트릭스 |
| VERIFY-02.02 | FOTA·패스키·음성·찾기·혜택 실행 | 기기/앱 증거 |
| VERIFY-02.03 | 자원·전력·단절 목표와 결과 대조 | 기기 인수 판정 |

### VERIFY-03 · 매장·환불·정산·운영 수용 시험

- 원문 연결: 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15번
- 산출물: 실제 카페 환경 실행 증거
- 완료 기준: 정상/취소/중복/지연/오입금·환불·매장 권한·운영 재조회 검증
- 선행 작업: PAY-05, SHOP-05, OPS-02, OPS-03, SHOP-06, INDEX-06
- 결정 입력: D08, D09, D19
- 앱/제품 연결: 전체 앱·실기·테스트넷
- 재사용 기준: 영역별 결과를 합친 실제 통합 시험
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J19
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 착수·완료 증거: VERIFY-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| VERIFY-03.01 | 카페 주문·입금·환불 예외 데이터 준비 | 매장 시험 시나리오 |
| VERIFY-03.02 | RN 키오스크·점주 앱·운영 도구 연결 검증 | 매장 종단 증거 |
| VERIFY-03.03 | 원장/체인/정산·권한 결과 대조 | 매장 인수 판정 |

### VERIFY-04 · 9개 온체인 영역·DEX 앱 수용 시험

- 원문 연결: 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15번
- 산출물: 영역별 앱 시작→실행→이벤트 증거
- 완료 기준: 토큰/WKRC/DeFi/스마트 계정/FX/Perp/STO/DID/x402 각각 검증; mock 제외
- 선행 작업: TOKEN-03, DEX-03, FX-03, PERP-05, STO-03, DID-03, X402-03, SMART-03, INDEX-05
- 결정 입력: D10, D11, D12, D13, D14, D15, D16, D19
- 앱/제품 연결: 전체 앱·실기·테스트넷
- 재사용 기준: 영역별 결과를 합친 실제 통합 시험
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J19
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 착수·완료 증거: VERIFY-04. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| VERIFY-04.01 | 9개 계약 영역의 배포/앱 조합 확인 | 온체인 수용 행렬 |
| VERIFY-04.02 | 각 앱에서 승인→실행→이벤트 확인 | 거래/로그 증거 |
| VERIFY-04.03 | DEX·FX·Perp 실패/복구 결과 대조 | 온체인 인수 판정 |

### VERIFY-05 · 여행 추천·데이터/혜택 수용 시험

- 원문 연결: 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15번
- 산출물: 실제 데이터 출처·여행 시나리오 결과
- 완료 기준: 권한 거절·삭제·데이터 부족·후기/결제 증명·중복 발도장 검증
- 선행 작업: AI-03, TRIP-03, STAMP-02, TRIP-04, INDEX-06
- 결정 입력: D17, D18, D19
- 앱/제품 연결: 전체 앱·실기·테스트넷
- 재사용 기준: 영역별 결과를 합친 실제 통합 시험
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J19
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 착수·완료 증거: VERIFY-05. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| VERIFY-05.01 | 여행·결제·후기·위치 출처 준비 | 여행 평가 표본 |
| VERIFY-05.02 | 코스·수정·따라하기·발도장 실행 | 여행 종단 증거 |
| VERIFY-05.03 | 동의 철회·삭제·관측 보정 반영 확인 | 여행 인수 판정 |

## RELEASE — 배포·운영 지원

### RELEASE-01 · 재현 빌드·CI·배포/ABI·시험 도구 구성

- 원문 연결: 15번
- 산출물: 앱/기기/계약/서버 릴리스 도구
- 완료 기준: 버전·환경 추적; 비밀 제외; 개발 배포와 외부 배포 분리
- 선행 작업: BASE-04, BASE-05
- 결정 입력: D10, D19
- 앱/제품 연결: 배포 도구·운영 화면·설치 앱
- 재사용 기준: 기존 빌드/배포 코드 선별
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J19
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 기술 선택: TECH-19. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-19. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: RELEASE-01. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| RELEASE-01.01 | 앱/펌웨어/서버/계약 빌드 명령 고정 | 빌드/배포 문서 |
| RELEASE-01.02 | ABI·주소·버전·환경 산출물 묶기 | 릴리스 manifest |
| RELEASE-01.03 | 깨끗한 환경에서 재현 확인 | 재현 빌드 기록 |

### RELEASE-02 · 관측·백업/복구·운영 절차 검증

- 원문 연결: 15번
- 산출물: 로그/상태·복구 도구·런북
- 완료 기준: 업무 데이터·Indexer 복구 재현; 민감 로그 배제; 장애를 사용자에게 표시
- 선행 작업: OPS-03, INDEX-03, BASE-05
- 결정 입력: D19
- 앱/제품 연결: 배포 도구·운영 화면·설치 앱
- 재사용 기준: 기존 빌드/배포 코드 선별
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J19
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 화면 연결: O04. [상태/복구 흐름](../specifications/screen-flows.md)
- 기술 선택: TECH-05, TECH-19. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-05, VAL-19. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: RELEASE-02. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| RELEASE-02.01 | 서비스·체인·작업 큐 상태 관측 연결 | 운영 지표/알림 |
| RELEASE-02.02 | 업무 DB·파일·Indexer 백업/복구 실행 | 복구 도구/런북 |
| RELEASE-02.03 | 장애 주입 후 사용자 상태·복구 대조 | 장애 복구 기록 |

### RELEASE-03 · 전체 증거 묶음·설치/운영 인수

- 원문 연결: 15번
- 산출물: 15개 요구사항 인수 목록·릴리스 패키지
- 완료 기준: 각 항목 실제 앱 증거와 잔여 이슈 연결; 실패 항목을 완료로 처리하지 않음
- 선행 작업: VERIFY-01, VERIFY-02, VERIFY-03, VERIFY-04, VERIFY-05, RELEASE-02
- 결정 입력: D19
- 앱/제품 연결: 배포 도구·운영 화면·설치 앱
- 재사용 기준: 기존 빌드/배포 코드 선별
- 검증 환경: 실제 대상 앱·서비스와 역할별 시험 계정
- 연결 시나리오: J19
- 상태: planned
- 담당자 / 공수 / 마감일: 미배정 / 미산정 / 미지정
- 기술 선택: TECH-19. [선택안](technology-selection.md)
- 구현 단계 기술 검증: VAL-19. [검증 카드](technology-validation-plan.md)
- 착수·완료 증거: RELEASE-03. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)

| 세부 ID | 실행할 작업 | 검토할 산출물 |
|---|---|---|
| RELEASE-03.01 | 15개 요구와 실제 증거 연결 | 최종 인수 표 |
| RELEASE-03.02 | 앱 설치·기기 배포·운영 절차 묶기 | 릴리스 패키지 |
| RELEASE-03.03 | 잔여 결함과 수정 확인을 판정에 반영 | 인수 결과 기록 |

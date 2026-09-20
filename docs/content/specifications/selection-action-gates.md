# 미선택 상태의 행위·화면·계약 연결

대표 행위 53개의 설계 연결. 화면/API 전체의 모든 method·subtype·edge case를 완전 열거한 라우터가 아니다. API와 화면 참조는 확장 설계이며 현재 wire 등록/화면 변경 완료를 뜻하지 않는다.

조회가 가능하다는 말은 무인증 접근을 뜻하지 않는다. 현재 읽기권·동의 범위와 해당 버전을 이해하는 신뢰된 reader가 필요하다. 새 profile 선택이 없어도 기존의 검증된 reader를 쓸 수 있다는 설계다. 현재 제품 reader가 구현됐다는 뜻은 아니다.

## 판정 순서

1. unknown action/subtype => reject
2. 현재 인증/권한/목적/owner와 신뢰된 해당 reader/writer capability 확인
3. 불변 원 operation/profile/exposure와 현재 security fence 확인
4. action+branch별 필요한 선택값과 호환/교차 제약 확인
5. 현재 revision CAS에서 효과/ACK/공개 전 최종 재검사

## 행위 유형

| 유형 | 의미 |
|---|---|
| read_observation | 새 권리/서명/지급/생성/외부 데이터 공개를 만들지 않는 현재 권한 내 조회 |
| safety_restriction | 권한/사용을 축소하는 제한만; 소유권 이전·키 삭제·복구 export 제외 |
| scoped_cleanup | 원 승인 plan/challenge의 허용 대상만 정리; 돈/키 초기화·재대여 제외 |
| continue_existing | 검증된 원 profile을 사용하는 같은 operation의 제한된 변경 재개; 새 효과 생성이 아님을 입증 필요 |
| new_effect | 새 인증/키/서명/결제/권한/공개/처리/원장 효과. 명시 선택·신뢰 profile·기능별 실행 근거 필요 |
| destructive_cleanup | 키·자격·위임의 비가역 정리; 현재 권한과 원 반납·복구 증거 별도 요구 |

### 모든 행위에 필요한 조건

- 현재 요청자의 해당 actor/scope 권한 또는 원 결과에 대한 유효한 제한 recovery proof
- 현재 owner/tenant/rental/context 및 개인정보 purpose/retention fence
- 해당 mode를 지원하는 신뢰된 reader/writer·schema·clock/lease capability
- 내부 idempotency/원 request digest·current resource revision 계약; 입력 role/self-asserted profile만으로 허가 불가

### 기존 작업 재개

- 새로운 선택이 미완료여도 원 불변 profile이 있고 현 보안 정책이 허용한 원 mode만 재개 가능.
- retired profile은 신규 사용 불가; current deny/revocation은 과거 active 표식보다 우선.
- 원 profile 미확보/지원 중지/unknown exposure에서는 조회·제한을 분리하며 변형 payload나 새 nonce로 대체 금지.
- 조회/관측을 저장해야 하는 대사는 별도의 continue_existing writer 권한으로 평가한다.

## 대표 행위별 연결

| ID | 행위 | 유형 | 화면 | 계약 |
|---|---|---|---|---|
| GA-01 | 소셜 로그인·identity 연결 | new_effect | U01, K01 | API-103, API-001, API-002, OC-01 |
| GA-02 | 기존 계정·기기·지갑 상태 조회 | read_observation | U01, U02, U04, U05, K01, O01 | API-005, API-007, API-008, API-009, API-013, API-090, API-091, API-099, API-100 |
| GA-03 | 로그아웃·이용 차단·로컬 잠금 | safety_restriction | U01, U05, U25, K01, D01 | API-003, API-110, OC-02 |
| GA-04 | HW 생성/import·기기 연결 | new_effect | U04, D01 | API-010, API-011, API-098 |
| GA-05 | HW 신규 서명·송금 제출 | new_effect | U03, D01 | API-017, API-018, API-108, API-109 |
| GA-06 | Cloud DKG·새 서명 | new_effect | U02, U03, U12 | API-014, API-015, OC-03 |
| GA-07 | MPC 보류·참여자 복구 재개 | continue_existing | U12 | API-016, OC-03 |
| GA-08 | 기기 설정 변경·등록 기기 알림 | new_effect | U05, D02 | 기기/native 계약 후보 |
| GA-09 | 거리 의존 신규 승인 | new_effect | K03, D01 | 기기/native 계약 후보 |
| GA-10 | FOTA 릴리스·새 설치 | new_effect | U06, O02, D02 | API-049 |
| GA-11 | 원 FOTA 복구·확정 | continue_existing | U06, D02 | 기기/native 계약 후보 |
| GA-12 | 업데이트 제안·원 릴리스 조회 | read_observation | U06, O02 | API-048, API-101 |
| GA-13 | 패스키 등록·인증·자격 변경 | new_effect | U07, D01 | 기기/native 계약 후보 |
| GA-14 | 녹음 신규 시작 | new_effect | U08, D02 | API-051 |
| GA-15 | 녹음 중단·수신 차단 | safety_restriction | U08, U25, D02 | 기기/native 계약 후보 |
| GA-16 | 원 녹음 청크 이어받기·종료 반영 | continue_existing | U08, D02 | OC-13 |
| GA-17 | 녹음/보호 결과 목록·조회 | read_observation | U08, U09, U25 | API-053, API-094, API-107, API-020, OC-13, OC-15, OC-16 |
| GA-18 | 전사·요약·파일 내보내기 | new_effect | U09, U25 | API-052, API-085, OC-13, OC-15 |
| GA-19 | 주문·quote·결제 수락 | new_effect | K02, K03, D01 | API-029, API-032, API-033, API-034, OC-05 |
| GA-20 | 원 지급·환불·영수증 결과 조회 | read_observation | U03, U11, K03, K05, O03 | API-019, API-020, API-030, API-035, API-038, API-042, API-106, OC-16 |
| GA-21 | 환불·취소·상품 인도 변경 | new_effect | K05, K03, O03 | API-031, API-036, API-037, OC-06 |
| GA-22 | 원 지급/환불 재대사·원 제출 재개 | continue_existing | O03, U11, K05 | API-087, API-018 |
| GA-23 | 매장·메뉴·수령인·권한 변경 | new_effect | K01, K02, K04, O04 | API-021, API-024, API-025, API-027, API-028 |
| GA-24 | 메뉴·매출·정산·감사 조회 | read_observation | K02, K06, O03, O04 | API-026, API-039, API-086, API-088, API-089, API-092, API-093 |
| GA-25 | 정산 마감·보정·발행 | new_effect | K06, O03 | API-040, API-041, API-087, OC-08 |
| GA-26 | terminal 시작·고객 전환 | new_effect | K01, K03 | API-023, OC-04 |
| GA-27 | terminal 종료·원 고객 정리 ACK | scoped_cleanup | K03, K01, O01 | OC-23 |
| GA-28 | terminal 종료 결과 조회 | read_observation | K01, K03 | OC-24 |
| GA-29 | 영수증 소유 claim·스탬프 적립/사용 | new_effect | U10, U11, U23, D02 | API-044, API-104, API-105, OC-07 |
| GA-30 | 카페 패스포트·스탬프 조회 | read_observation | U10, D02 | API-043 |
| GA-31 | 스마트 계정 전환·신규 실행 | new_effect | U13 | API-055, API-056 |
| GA-32 | DeFi·FX 견적·실행 | new_effect | U14, U15 | API-057, API-058, OC-09 |
| GA-33 | Perpetual 주문·종료·keeper | new_effect | U16 | API-061, OC-09, OC-10 |
| GA-34 | 시장·포지션·STO 보유 조회 | read_observation | U13, U14, U15, U16, U17 | API-059, API-060, API-062, API-063, API-102, API-107 |
| GA-35 | DID 발급·제시·검증 | new_effect | U18 | API-065, API-067, API-068, OC-11 |
| GA-36 | STO 발행·자격 제한 전송 | new_effect | U17 | API-064, OC-11 |
| GA-37 | 기존 자격 조회 | read_observation | U18 | API-066 |
| GA-38 | 유료 자원 신규 요청·지급·환불 | new_effect | U19 | API-070, API-071, OC-12, OC-21 |
| GA-39 | 유료 원 요청 결과·환불 조회 | read_observation | U19 | API-072, API-107, OC-12, OC-22 |
| GA-40 | 신규 대여·반납 완료·재대여 | new_effect | U24, O01 | API-045, API-047 |
| GA-41 | 키 초기화·자격/위임 정리 | destructive_cleanup | U07, U24, O01 | 기기/native 계약 후보 |
| GA-42 | 반납 현황·원 체크 결과 조회 | read_observation | U24, O01 | API-091 |
| GA-43 | 장소 검색·후기·여행 기록 생성 | new_effect | U20, U21 | API-073, API-074, API-075, API-077, API-078, OC-14 |
| GA-44 | 추천 생성·편집·후보 적용 | new_effect | U22 | API-080, API-081, OC-25 |
| GA-45 | 챌린지 참가·증거·보상 | new_effect | U23 | API-082, API-083, OC-14 |
| GA-46 | 여행·코스·챌린지 원 결과 조회 | read_observation | U20, U21, U22, U23 | API-079, API-095, API-096, API-097, API-107, OC-26 |
| GA-47 | 목적별 동의 철회·축소 | safety_restriction | U25, U21, U09 | API-084, OC-15 |
| GA-48 | 자격·릴리스 철회 | safety_restriction | U18, O02 | API-069, API-050 |
| GA-49 | 반납 체크 요청·권한 제한 시작 | safety_restriction | U24, O01 | API-046 |
| GA-50 | 동의 부여·확장 | new_effect | U25, U21 | API-084, OC-15 |
| GA-51 | 후기·녹음·개인정보 삭제 | scoped_cleanup | U25, U08, U20 | API-004, API-054, API-076, API-085, OC-15 |
| GA-52 | 세션 갱신·로그인 수단 해제 | new_effect | U01, U25 | API-006, OC-17, OC-19 |
| GA-53 | 인증 갱신·해제 원 결과 조회 | read_observation | U01, U25 | OC-18, OC-20 |

## 입력과 보류·복구 기준

### GA-01 · 소셜 로그인·identity 연결

- 필요한 선택: PF-D01-01, PF-D01-02, PF-D01-03, PF-D01-04, PF-D02-01, PF-D02-02, PF-D02-03, PF-D02-04
- 도메인 조건: provider별 flow/proof와 현재 link 권한을 결합; 이메일 자동 합치기 금지
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-02 · 기존 계정·기기·지갑 상태 조회

- 필요한 선택: 새 업무 선택값 없음; 공통 권한·reader/writer 조건은 필수
- 도메인 조건: 현재 owner/store/capability projection; 잔액 freshness와 지급 완료를 구분
- 미선택 때: current_scope_read_only_if_trusted_reader; 권한/원천 불명은 unknown 또는 제한 상태, 완료 추정 금지

### GA-03 · 로그아웃·이용 차단·로컬 잠금

- 필요한 선택: 새 업무 선택값 없음; 공통 권한·reader/writer 조건은 필수
- 도메인 조건: 권한을 줄이는 scope 제한만. 인증된 현재 경로 또는 해당 로컬 소유 세션의 잠금; 임의 계정/서버 상태 변경 금지
- 미선택 때: 가능한 현재 제한을 적용하고 미전달 서버 철회는 pending; key erase/복구 실패로 확장 금지

### GA-04 · HW 생성/import·기기 연결

- 필요한 선택: PF-D02-01, PF-D02-02, PF-D02-03, PF-D02-04, PF-D03-01, PF-D03-02, PF-D03-03, PF-D05-01, PF-D05-02
- 도메인 조건: 물리 확인·기기 identity·민감정보 수명, 신규 생성과 import 구분
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-05 · HW 신규 서명·송금 제출

- 필요한 선택: PF-D02-01, PF-D02-02, PF-D02-03, PF-D02-04, PF-D03-01, PF-D03-02, PF-D03-03, PF-D10-01, PF-D10-02, PF-D10-03
- 도메인 조건: 선택 signer와 원 intent/payload binding, 해당 도메인의 quote/자금/권한 guard도 필요
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-06 · Cloud DKG·새 서명

- 필요한 선택: PF-D02-01, PF-D02-02, PF-D02-03, PF-D02-04, PF-D04-01, PF-D04-02, PF-D04-03, PF-D04-04, PF-D10-01, PF-D10-02, PF-D10-03
- 도메인 조건: 소셜 로그인만으로 자금 권한 부여 금지; 원 operation/participant/epoch
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-07 · MPC 보류·참여자 복구 재개

- 필요한 선택: PF-D04-01, PF-D04-02, PF-D04-03, PF-D04-04
- 도메인 조건: MR01~07의 원 checkpoint/commit epoch와 현재 권한 검사; 새로운 DKG로 대체 금지
- 미선택 때: hold_mutating_resume_unless_original_profile_and_current_gate_proven; 조회/차단은 별도 평가

### GA-08 · 기기 설정 변경·등록 기기 알림

- 필요한 선택: PF-D02-01, PF-D02-02, PF-D02-04, PF-D05-01, PF-D07-03
- 도메인 조건: 등록 owner의 설정/알림 범위; 거리가 미선택이어도 그 값에 의존하지 않는 검증된 알림은 별도 허용
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-09 · 거리 의존 신규 승인

- 필요한 선택: PF-D02-01, PF-D02-02, PF-D02-04, PF-D05-01, PF-D07-04
- 도메인 조건: 인증된 현재 ranging peer/관측/시간과 원 승인 context; 자체 거리 승인이 passkey를 대체하지 않음
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-10 · FOTA 릴리스·새 설치

- 필요한 선택: PF-D05-01, PF-D05-02, PF-D05-03, PF-D05-04, PF-D07-03, PF-D19-04
- 도메인 조건: 릴리스 서명·선정 board·공존 중재·업데이트 승인
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-11 · 원 FOTA 복구·확정

- 필요한 선택: PF-D05-01, PF-D05-02, PF-D05-03, PF-D05-04
- 도메인 조건: 원 image/digest/partition과 부트 검증 상태; 위험한 이미지로 임의 rollback 금지
- 미선택 때: hold_mutating_resume_unless_original_profile_and_current_gate_proven; 조회/차단은 별도 평가

### GA-12 · 업데이트 제안·원 릴리스 조회

- 필요한 선택: 새 업무 선택값 없음; 공통 권한·reader/writer 조건은 필수
- 도메인 조건: 조회 성공은 설치 허가 아님; 철회/호환 상태 별도
- 미선택 때: current_scope_read_only_if_trusted_reader; 권한/원천 불명은 unknown 또는 제한 상태, 완료 추정 금지

### GA-13 · 패스키 등록·인증·자격 변경

- 필요한 선택: PF-D03-02, PF-D03-03, PF-D05-01, PF-D06-01, PF-D06-02, PF-D06-03, PF-D06-04
- 도메인 조건: 대상 RP와 표준 transport/UV 범위; 반납 삭제는 GA-41 별도
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-14 · 녹음 신규 시작

- 필요한 선택: PF-D02-01, PF-D02-02, PF-D02-04, PF-D07-01, PF-D07-02, PF-D07-03, PF-D17-03
- 도메인 조건: 현재 owner/rental/binding/stream namespace·동의·수신 gate
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-15 · 녹음 중단·수신 차단

- 필요한 선택: 새 업무 선택값 없음; 공통 권한·reader/writer 조건은 필수
- 도메인 조건: 현재 소유 capture를 중지·fence만 설정; 인증된 end/coverage 없이 complete 표시 금지
- 미선택 때: 현재 녹음을 멈추고 partial을 보존. 파일 삭제나 다른 계정 저장으로 전환하지 않음

### GA-16 · 원 녹음 청크 이어받기·종료 반영

- 필요한 선택: PF-D07-01, PF-D07-02, PF-D07-03, PF-D17-03
- 도메인 조건: 원 stream generation·owner와 현재 receive gate; fence 뒤 late chunk 저장/ACK 금지
- 미선택 때: hold_mutating_resume_unless_original_profile_and_current_gate_proven; 조회/차단은 별도 평가

### GA-17 · 녹음/보호 결과 목록·조회

- 필요한 선택: 새 업무 선택값 없음; 공통 권한·reader/writer 조건은 필수
- 도메인 조건: 현재 목적/owner 범위와 삭제 tombstone 확인; revoked owner의 과거 snapshot은 읽기권 아님
- 미선택 때: current_scope_read_only_if_trusted_reader; 권한/원천 불명은 unknown 또는 제한 상태, 완료 추정 금지

### GA-18 · 전사·요약·파일 내보내기

- 필요한 선택: PF-D17-01, PF-D17-03, PF-D18-02, PF-D19-03
- 도메인 조건: export는 새로운 데이터 공개 행위; 읽기 UI와 별도 목적/수령/보관 검사
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-19 · 주문·quote·결제 수락

- 필요한 선택: PF-D02-01, PF-D02-02, PF-D02-03, PF-D02-04, PF-D08-01, PF-D08-02, PF-D08-03, PF-D08-04, PF-D08-05, PF-D10-01, PF-D10-02, PF-D10-03, PF-D10-04
- 도메인 조건: 주문 snapshot/수령/원 payment identity와 고객 gas; exact/partial 선택 없으면 업무 수락 보류
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-20 · 원 지급·환불·영수증 결과 조회

- 필요한 선택: 새 업무 선택값 없음; 공통 권한·reader/writer 조건은 필수
- 도메인 조건: chain 관측만으로 새 allocation/claim/혜택을 만들지 않음; 현재 customer context 투영
- 미선택 때: current_scope_read_only_if_trusted_reader; 권한/원천 불명은 unknown 또는 제한 상태, 완료 추정 금지

### GA-21 · 환불·취소·상품 인도 변경

- 필요한 선택: PF-D08-01, PF-D08-02, PF-D08-03, PF-D08-04, PF-D08-05, PF-D10-01, PF-D10-02, PF-D10-03, PF-D19-04
- 도메인 조건: 취소/환불/인도는 주문 상태를 바꿈; 금액·원 allocation·current signer와 revision 검사
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-22 · 원 지급/환불 재대사·원 제출 재개

- 필요한 선택: PF-D08-01, PF-D08-02, PF-D08-03, PF-D08-04, PF-D08-05, PF-D10-01, PF-D10-02, PF-D10-04
- 도메인 조건: 원 상태 관측과 업무 commit 구분; 재전파는 새로운 payload/nonce 아닌 검증된 원 바이트만 해당 계약이 허용할 때
- 미선택 때: hold_mutating_resume_unless_original_profile_and_current_gate_proven; 조회/차단은 별도 평가

### GA-23 · 매장·메뉴·수령인·권한 변경

- 필요한 선택: PF-D01-02, PF-D01-03, PF-D02-02, PF-D02-03, PF-D08-01, PF-D08-03, PF-D19-04
- 도메인 조건: 메뉴 표시 읽기와 수령인/sign 권한 변경을 분리; 이미 열린 order snapshot 자동 변경 금지
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-24 · 메뉴·매출·정산·감사 조회

- 필요한 선택: 새 업무 선택값 없음; 공통 권한·reader/writer 조건은 필수
- 도메인 조건: role별 projection/조회 시각/원천 revision; 조회가 정산 송금/마감 아님
- 미선택 때: current_scope_read_only_if_trusted_reader; 권한/원천 불명은 unknown 또는 제한 상태, 완료 추정 금지

### GA-25 · 정산 마감·보정·발행

- 필요한 선택: PF-D08-02, PF-D08-05, PF-D19-04
- 도메인 조건: 현재 기간·원장/정책 revision과 manifest CAS; 마감 후 원 기록 덮어쓰기 금지
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-26 · terminal 시작·고객 전환

- 필요한 선택: PF-D01-02, PF-D02-01, PF-D02-02, PF-D02-03, PF-D02-04, PF-D19-04
- 도메인 조건: clearance receipt 현재 boot/customer/head와 단회 소비; 새 고객 권한 별도
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-27 · terminal 종료·원 고객 정리 ACK

- 필요한 선택: 새 업무 선택값 없음; 공통 권한·reader/writer 조건은 필수
- 도메인 조건: 원 종료 challenge/고객/boot에 바인딩한 local view/cache 청소만; 자산·계정·기기 키 삭제 아님
- 미선택 때: 신뢰된 원 정리 계약과 현재 권한으로 제한 수행. 신규 고객 전환은 GA-26, 불명 ACK는 정리 미확인

### GA-28 · terminal 종료 결과 조회

- 필요한 선택: 새 업무 선택값 없음; 공통 권한·reader/writer 조건은 필수
- 도메인 조건: 조회 자체가 clearance 재발급/소비 또는 새 세션을 만들지 않음
- 미선택 때: current_scope_read_only_if_trusted_reader; 권한/원천 불명은 unknown 또는 제한 상태, 완료 추정 금지

### GA-29 · 영수증 소유 claim·스탬프 적립/사용

- 필요한 선택: PF-D08-02, PF-D08-04, PF-D09-01, PF-D09-02, PF-D09-03, PF-D09-04, PF-D17-04
- 도메인 조건: claim은 권한 이전·혜택은 효과 변경; 내부 원장 단일 writer와 현재 source/entitlement 확인
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-30 · 카페 패스포트·스탬프 조회

- 필요한 선택: 새 업무 선택값 없음; 공통 권한·reader/writer 조건은 필수
- 도메인 조건: 확정/보류/이미 사용 상태 구분, 조회 시 미선택 규칙으로 재산출하지 않음
- 미선택 때: current_scope_read_only_if_trusted_reader; 권한/원천 불명은 unknown 또는 제한 상태, 완료 추정 금지

### GA-31 · 스마트 계정 전환·신규 실행

- 필요한 선택: PF-D03-02, PF-D03-03, PF-D10-01, PF-D10-02, PF-D10-04, PF-D11-01, PF-D11-02, PF-D11-03, PF-D11-04
- 도메인 조건: 주소·signer·자산 이관 확인 및 account bundle; EOA 독립 경로는 별도
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-32 · DeFi·FX 견적·실행

- 필요한 선택: PF-D10-01, PF-D10-02, PF-D10-04, PF-D12-01, PF-D12-02, PF-D12-03, PF-D12-04
- 도메인 조건: 상품 typed source·가격/allowance/슬리피지·현재 서명 gate
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-33 · Perpetual 주문·종료·keeper

- 필요한 선택: PF-D10-01, PF-D10-02, PF-D10-04, PF-D13-01, PF-D13-02, PF-D13-03, PF-D13-04
- 도메인 조건: 포지션 종료/청산도 변경 효과; 위험 축소를 이유로 price/권한 guard 우회 금지
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-34 · 시장·포지션·STO 보유 조회

- 필요한 선택: 새 업무 선택값 없음; 공통 권한·reader/writer 조건은 필수
- 도메인 조건: 평가액/원천 freshness/확정 상태 표시; 읽기 결과를 신규 quote로 사용하지 않음
- 미선택 때: current_scope_read_only_if_trusted_reader; 권한/원천 불명은 unknown 또는 제한 상태, 완료 추정 금지

### GA-35 · DID 발급·제시·검증

- 필요한 선택: PF-D03-02, PF-D03-03, PF-D15-01, PF-D15-02, PF-D15-03, PF-D15-04
- 도메인 조건: 기존 credential 읽기와 새 presentation/verification challenge 기록 구분
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음
- 분기: 명시 branch의 필드로 대체, top-level은 영향 합집합. unknown branch는 거절.
  - issue_or_present_with_device: PF-D15-01, PF-D15-02, PF-D15-03, PF-D15-04, PF-D03-02, PF-D03-03
  - verify_presentation: PF-D15-01, PF-D15-02, PF-D15-03, PF-D15-04

### GA-36 · STO 발행·자격 제한 전송

- 필요한 선택: PF-D10-01, PF-D10-02, PF-D10-04, PF-D14-01, PF-D14-02, PF-D14-03, PF-D14-04, PF-D15-01, PF-D15-02, PF-D15-03, PF-D15-04
- 도메인 조건: offering 권리·issuer·현재 자격, 실행 환경 분리
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-37 · 기존 자격 조회

- 필요한 선택: 새 업무 선택값 없음; 공통 권한·reader/writer 조건은 필수
- 도메인 조건: 읽기는 현재 holder/verifier 범위. 자격 철회 mutation은 GA-48
- 미선택 때: current_scope_read_only_if_trusted_reader; 권한/원천 불명은 unknown 또는 제한 상태, 완료 추정 금지

### GA-38 · 유료 자원 신규 요청·지급·환불

- 필요한 선택: PF-D08-03, PF-D08-04, PF-D10-01, PF-D10-02, PF-D10-03, PF-D16-01, PF-D16-02, PF-D16-03, PF-D16-04
- 도메인 조건: gas 역할 일치·current entitlement/source/refund gate; 신규 요청과 기존 result 구분
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음
- 결합 gate: resource result generation requires GA-44 generate_candidate when itinerary resource, refund requires selected refund subtype and original funding exposure checks

### GA-39 · 유료 원 요청 결과·환불 조회

- 필요한 선택: 새 업무 선택값 없음; 공통 권한·reader/writer 조건은 필수
- 도메인 조건: 현재 read authority 및 privacy/refund/entitlement 상태에 맞는 projection; 조회를 생성/공개 허가로 바꾸지 않음
- 미선택 때: current_scope_read_only_if_trusted_reader; 권한/원천 불명은 unknown 또는 제한 상태, 완료 추정 금지

### GA-40 · 신규 대여·반납 완료·재대여

- 필요한 선택: PF-D03-03, PF-D09-01, PF-D09-02, PF-D09-03, PF-D09-04, PF-D19-04
- 도메인 조건: 단계별 branch 평가: RR 초기화 정책은 신규 여행 EOA 초기화에만 필요. import는 외부 접근/기기 사본 삭제 규칙; 자산 전체 sweep 금지
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음
- 결합 gate: complete_return_or_readmit requires immutable successful cleanup evidence from GA-41 where applicable; initial rental requires clear inventory. Neither branch may bypass cleanup.
- 분기: 명시 branch의 필드로 대체, top-level은 영향 합집합. unknown branch는 거절.
  - new_rental: PF-D03-03, PF-D09-03, PF-D19-04
  - complete_return_or_readmit: PF-D03-03, PF-D09-01, PF-D09-02, PF-D09-03, PF-D09-04, PF-D19-04

### GA-41 · 키 초기화·자격/위임 정리

- 필요한 선택: PF-D03-03, PF-D03-04, PF-D09-03, PF-D09-04, PF-RR-DEC-01-01, PF-RR-DEC-01-02, PF-RR-DEC-01-03, PF-RR-DEC-01-04
- 도메인 조건: 종류별 반납/복구 증거·현재 권한·원 return commit 필요. RR 필드는 신규 여행 EOA 초기화 branch에만 적용
- 미선택 때: 미선택/복구 불명은 초기화·재대여 보류. 로그아웃과 혼동 금지
- 분기: 명시 branch의 필드로 대체, top-level은 영향 합집합. unknown branch는 거절.
  - new_travel_backup_reset: PF-D03-03, PF-D03-04, PF-D09-03, PF-D09-04, PF-RR-DEC-01-01, PF-RR-DEC-01-02, PF-RR-DEC-01-03, PF-RR-DEC-01-04 · selected policy=add_user_encrypted_recovery_backup + independently proven current recovery + original return commit
  - new_travel_hold_without_recovery: PF-D03-03, PF-D03-04, PF-D09-03, PF-D09-04, PF-RR-DEC-01-01, PF-RR-DEC-01-04 · always hold key reset; policy=hold_reset_without_recovery does not itself establish recovery
  - imported_wallet_copy_reset: PF-D03-03, PF-D09-03, PF-D09-04 · external access evidence + original return commit; no unrelated sweep
  - credential_or_delegation_cleanup: PF-D03-03, PF-D09-04 · typed credential/delegation scope + alternative access + original authorized cleanup plan

### GA-42 · 반납 현황·원 체크 결과 조회

- 필요한 선택: 새 업무 선택값 없음; 공통 권한·reader/writer 조건은 필수
- 도메인 조건: 현재 rental scope. API046의 체크 요청/차단 상태 변경은 GA-49 별도
- 미선택 때: current_scope_read_only_if_trusted_reader; 권한/원천 불명은 unknown 또는 제한 상태, 완료 추정 금지

### GA-43 · 장소 검색·후기·여행 기록 생성

- 필요한 선택: PF-D17-01, PF-D17-02, PF-D17-03, PF-D17-04, PF-D19-03
- 도메인 조건: 검색도 외부 위치 전달이면 새 공개 동작; 저장된 목록 조회와 구분
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-44 · 추천 생성·편집·후보 적용

- 필요한 선택: PF-D02-03, PF-D17-01, PF-D17-02, PF-D17-03, PF-D18-01, PF-D18-02, PF-D18-03
- 도메인 조건: 수동 편집은 AI provider 필드를 요구하지 않는 branch. 유료 생성은 GA-38도 필요; 적용은 현재 head CAS
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음
- 분기: 명시 branch의 필드로 대체, top-level은 영향 합집합. unknown branch는 거절.
  - generate_candidate: PF-D17-01, PF-D17-02, PF-D17-03, PF-D18-01, PF-D18-02, PF-D18-03, PF-D02-03
  - manual_edit: PF-D17-03, PF-D18-01, PF-D02-03
  - apply_candidate: PF-D17-01, PF-D17-03, PF-D18-01, PF-D18-03, PF-D02-03

### GA-45 · 챌린지 참가·증거·보상

- 필요한 선택: PF-D09-01, PF-D09-02, PF-D17-02, PF-D17-04, PF-D18-04
- 도메인 조건: 방문과 실제 구매 증거를 구분; 재평가에서 target revision 확인
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-46 · 여행·코스·챌린지 원 결과 조회

- 필요한 선택: 새 업무 선택값 없음; 공통 권한·reader/writer 조건은 필수
- 도메인 조건: 현재 owner/동의 범위의 기존 결과; 후보 조회가 selected plan 변경/유료 regen을 유발하지 않음
- 미선택 때: current_scope_read_only_if_trusted_reader; 권한/원천 불명은 unknown 또는 제한 상태, 완료 추정 금지

### GA-47 · 목적별 동의 철회·축소

- 필요한 선택: 새 업무 선택값 없음; 공통 권한·reader/writer 조건은 필수
- 도메인 조건: 이 row는 기존 목적의 철회/축소만. 동의 부여/확장은 별도 GA-50
- 미선택 때: 철회는 현재 인증된 목적 범위에 차단을 먼저 기록; 아직 선택 안 된 보관기간을 이유로 계속 처리하지 않음

### GA-48 · 자격·릴리스 철회

- 필요한 선택: 새 업무 선택값 없음; 공통 권한·reader/writer 조건은 필수
- 도메인 조건: 현재 issuer/release authority와 신뢰된 restriction writer 필요. 조작된 요청으로 다른 사람 자격을 지우지 않음
- 미선택 때: 권한이 검증되는 해당 제한만 수행; 결과는 새 발급/설치 허가 아님

### GA-49 · 반납 체크 요청·권한 제한 시작

- 필요한 선택: 새 업무 선택값 없음; 공통 권한·reader/writer 조건은 필수
- 도메인 조건: 현재 owner/rental scope의 check/fence 요청; key erase commit 또는 재대여를 수행하지 않음
- 미선택 때: 확인 가능한 제한/검토만 수행, 복구 선택을 추정하거나 초기화하지 않음

### GA-50 · 동의 부여·확장

- 필요한 선택: PF-D17-02, PF-D17-03, PF-D19-03
- 도메인 조건: 목적·제공자·기간 고지 후 consent revision; 과거 삭제 tombstone/작업 자동 부활 금지
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-51 · 후기·녹음·개인정보 삭제

- 필요한 선택: PF-D17-03, PF-D19-03
- 도메인 조건: 현재 owner/명시 scope의 승인된 plan·보존 의무·대상 revision·원천 종료 검사. scope 없는 일괄 삭제 금지
- 미선택 때: 원 plan이 있으면 현재 scope/hold/target revision 검사 후 정리 가능. plan 없으면 요청 접수·해당 목적 차단만, 완료로 표시하지 않음

### GA-52 · 세션 갱신·로그인 수단 해제

- 필요한 선택: PF-D01-01, PF-D01-02, PF-D01-03, PF-D01-04, PF-D02-01, PF-D02-02, PF-D02-03, PF-D02-04
- 도메인 조건: 회전 successor/현재 family와 마지막 로그인 수단 보호; logout 제한과 별도
- 미선택 때: hold_new_effect; 이유와 해당 필드만 표시, 자동 재시도/서명 없음

### GA-53 · 인증 갱신·해제 원 결과 조회

- 필요한 선택: 새 업무 선택값 없음; 공통 권한·reader/writer 조건은 필수
- 도메인 조건: current scoped recovery proof로 제한 조회, 유효 access token만 강제하지 않음. refresh 이전 generation bearer 반환 금지
- 미선택 때: current_scope_read_only_if_trusted_reader; 권한/원천 불명은 unknown 또는 제한 상태, 완료 추정 금지

## 화면 응답 계약

**필드** — actionId, actionVariant, availability, reasonCode, missingFieldIds, profileSetDigest, evaluatedRevision, nextAllowedActions, publicExplanationKey

**표시 상태** — read_only, restriction_only, cleanup_only, hold, eligible_for_domain_checks

서버/기기 판정을 앱이 임의 enabled로 바꾸지 않는다. 진단 필드는 해당 사용자에게 공개 가능한 범위로 축약하며 타인 profile/보유 자산/권한 존재를 노출하지 않는다.

설정 변경·계정 전환·기기 세대 변경 때 재평가; 이전 enabled 캐시로 서명/결제 금지.

화면 문구 후보:

- 이 기능의 설정이 아직 완료되지 않았어요. 기존 내역은 확인할 수 있어요.
- 원래 거래의 결과를 확인하고 있어요. 새 결제를 만들지 마세요.
- 복구 확인이 필요해 기기 초기화를 보류했어요.
- 녹음은 중단됐으며 수신한 부분만 보관했어요.

API 참조는 제안 동작의 연결이다. 예를 들어 동일 API-084의 동의 부여와 철회는 서로 다른 gate를 쓰며, POST 여부로 새 효과·조회·제한을 판정하지 않는다. 현재 wire에 없는 subtype은 해당 계약을 고정하기 전 실행하지 않는다.

[필드·교차 제약](selection-profile-contract.md) · [구조화 행위표](selection-action-gates.json) · [설계 검사](selection-profile-validation.json)

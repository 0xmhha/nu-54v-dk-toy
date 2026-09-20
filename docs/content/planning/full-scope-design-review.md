# 전체 15개 요구사항 설계 점검과 다음 순서

작성: 2026-09-18. **다음 상세 설계는 기기 동시동작·FOTA 공존을 권장한다.** 서명·패스키·녹음·찾기·반납·업데이트가 같은 보드/통신/저장을 사용하므로, 개별 기능보다 먼저 자원과 중단 경계를 연결할 필요가 있다.

이번 작업은 현재 로컬 설계 산출물의 점검이다. 구현·설치·배포·실기 검증을 시작하지 않았다. 기존 15개 요구사항, WBS 104개·세부 작업 320개, 3명·12주·앱 연동 완료 목표를 유지한다. 개인별 역할이나 작업량을 산정하지 않는다.

[요구사항·작업·결정 대응 원본](full-scope-design-review.json) · [검증기](validate_full_scope_design.py) · [원 완료 범위](../twelve-week-completion-scope-v3.md)

## 1. 현재 판단

모든 요구에 작업 목록과 공통 화면/API·인터페이스 초안이 존재한다. 그러나 상세 설계 수준은 균일하지 않다. 승인·결제 대사·대여/반납은 실패·동시성·저장·복구 후보까지 이어져 있고, 다른 영역에는 다음 구체화가 남아 있다.

- **기기:** FOTA와 서명/녹음/패스키/반납의 공존, 실제 보드 profile과 보호 저장·자원 제약
- **계정·Cloud Wallet:** 소셜 계정 연결과 MPC 참여자/복구·세션 중단의 구체 경계
- **온체인 상품:** 스마트계정 호환, DeFi/FX/Perpetual 상품 규칙, DID/STO/x402의 역할과 실제 승인·실행 계약
- **여행·운영:** 데이터 출처·보관/삭제·추천/챌린지 판정, 제품 전체의 운영/릴리스 증거

설계 문서·API·작업 수를 구현 완료율로 환산하지 않는다. 현재 API 110개, BLE 34개, 참조 SQL 업무 테이블 61개/마이그레이션 7개가 있다고 실제 동작 검증이 끝난 것은 아니다. 최신 대여 종단 문서의 107개 사례도 참조가 연결된 상태이며 실행 결과가 아니다.

기존 코드 재사용 판단은 저장된 검토 스냅샷에 한정한다. 이번에 외부 저장소 최신 버전이나 현재 배포 상태를 다시 검증하지 않았다. 기술 후보 이름은 기존 제안의 인용이며 신규 라이브러리 선정이 아니다.

## 2. 요구사항별 점검

아래의 ‘남은 설계’는 범위를 줄이는 항목이 아니다. 각 요구를 실제 앱 연동까지 완성하기 위해 더 연결해야 할 부분이다. 모든 행은 제품 구현 완료로 표시하지 않는다.

| 번호·요구 | 현재 설계 | 다음 구체화 | 연결 묶음 |
| --- | --- | --- | --- |
| 1. 실기 HW wallet | Zephyr 기반 확정, 키/서명 인터페이스와 승인 기준·대여 복구 후보 있음. 실제 보드 구현 증거는 없음. | 기기 서비스 중재/자원 소유권; 설정·import·반납 전환과 키 backend 입력 계약 | DS-01 |
| 2. 실기 FOTA | IF05와 업데이트 상태 초안, 기술 후보 및 저장 표식 보존 제약 있음. 부트/저장 이행 상세는 미채택. | 서명/녹음/반납과 FOTA 경합 상태표; 이미지·키 설정·저널 migration/rollback 허용 경계; 앱 중단/재연결·boot confirm 증거 | DS-01 |
| 3. 패스키·녹음·찾기·결제 스탬프 | 네 기능의 작업·인터페이스 존재. 스탬프/반납 보정이 상세하며 패스키/음성/찾기는 대상·품질/profile 미정. | 네 기능 공통 자원/권한 중재; 패스키 실제 RP 흐름; 녹음 단절·부분 파일·백그라운드 흐름; 찾기와 거리 peer 실패 상태 | DS-01 |
| 4. 유저 앱 | 화면/공통 DTO/native 경계와 지갑·기기 후보 존재. 제품 전체 앱 연결은 미검증. | 앱 foreground/background/restart 동작; native 서비스와 계정전환·작업복구; 상품별 화면 계약 연결 | DS-02 |
| 5. Google·Apple 소셜 로그인 | 인증 흐름·provider proof·계정 경계 설계 있음. 실제 제공자 설정/모바일 콜백 조합 미선정. | 제공자별 콜백/nonce/계정연결 경합; 계정전환·세션철회가 signer/작업에 미치는 범위 | DS-02 |
| 6. 소셜 계정 Cloud Wallet MPC | IF11·지갑 통제 및 승인 연결 후보 있음. 참여자 구성/threshold/제공자는 미선정; 기존 브리지 서명 코드를 완료로 인정하지 않음. | 키 조각 보관·복구 주체와 현재 로그인 구분; 중단된 생성/서명·참여자 교체의 상태/증거; 모바일 참여자와 서버 경계 | DS-02 |
| 7. 위치 검색·결제 기록·발자취 | 영수증/출처/보정 경계가 상세. 장소·위치·여행 UI/정책은 공통 초안. | 장소와 매장/결제 식별 연결; 위치거절·삭제·타임존/언어; 추천에 전달할 검증 출처 | DS-07 |
| 8. 앱에서 실제기기 설정 | 등록·반납·세션 복구는 상세 후보와 16개 종단 흐름 있음. 모두 실행 미검증. | 다른 기능과 기기 설정 동시성; 미완성 holder 분실/재승인·불명확 등록 회수; 설정 profile과 버전 채택 | DS-01 |
| 9. RN 키오스크·점주 매장 운영 | 키오스크 RN 확정, 승인·환불·대사·정산의 현재 호환 후보 존재. 매장/단말 운영 UI와 정책 추가 연결 필요. | 관리 모드 진입·소속/수취 주소 변경; 메뉴/가격/품절 snapshot과 주문; 부분/초과/지연 입금과 환불 마감 조합 | DS-03 |
| 10. 운영 백오피스 | 권한/운영 재처리/대여 조회 설계 있음. 전체 운영 화면·릴리스·재해 복구 운영 규칙 미완성. | 운영 action별 권한·감사/조회 최소화; FOTA 배포 승인/회수 경로; 서비스별 장애 진단과 보정 권한 | DS-08 |
| 11. StableNet 테스트넷 계약군 | 9개 영역 모두 WBS에 있음. 재사용 스냅샷/공통 인터페이스 존재, 상품·표준·배포 호환 선택은 분리 필요. | 자산/가스/ABI/address/version manifest; EOA 다음 스마트 계정 전환과 signer adapter; 상품별 수명주기/가격/자격/지급 계약 | DS-04 |
| 12. 기존 Indexer 확장 | 재사용 검토와 소비자 보정·대사 연결 후보 있음. 대상 ABI/decoder/배포와 기존 SDK 차이 실제 적용은 미수행. | 모든 9개 계약군 이벤트→조회 모델; decoder/배포 버전 교체·cursor 재개; 체인 관측과 업무원장 권한/정정 분리 | DS-04 |
| 13. DEX·DeFi·FX·Perpetual 앱 서비스 | API/앱 시나리오·재사용 소스 조사 있음. 고정가격/TODO를 완료로 보지 않으며 상품 규칙 미결정. | 현물/FX와 파생상품 모델 분리; 견적 유효성·가격오류·정밀도/슬리피지; 펀딩/청산 keeper·중단 복구와 앱 포지션 | DS-05 |
| 14. AI 여행 추천·챌린지·발도장 | 출처·정정·개인정보 연결과 추천 인터페이스 있음. 코스 품질·완료/보상·실제 구매 범위 미정. | 추천 입력 근거/시간/제약·데이터 부족 대안; 결제·방문·챌린지 완료 구분; 후기/코스 삭제와 증거 철회 영향 | DS-07 |
| 15. 필수 운영·검증·배포 도구 | 작업/증거 카드·호환 manifest·이행/복구 후보 있음. 실제 환경·SLO·배포/인수 증거 없음. | 전체 제품 manifest와 요구사항별 증거 묶음; 백업/복구·비밀회전·부하/배터리 관측; 실패 사례와 운영 인수 기준 | DS-08 |

## 3. 큰 요구 안의 세부 범위도 유지

3번은 패스키·녹음·찾기·결제 스탬프 네 가지를 각각 완료해야 한다. 스탬프와 반납을 상세화했다고 패스키·녹음까지 완료된 것으로 보지 않는다.

11번은 다음 아홉 영역을 각각 다룬다.

| 영역 | 다음 설계 연결 | 혼동하지 않을 경계 |
| --- | --- | --- |
| USDC/시연 더미 토큰 | DS-04 | 더미 토큰 동작과 실제 USDC 지원/조달은 별도 |
| WKRC | DS-04 | native gas와 wrapped 자산 계약 구분 |
| DeFi | DS-05 | 지정 상품의 실제 실행/유동성 조건 필요 |
| 스마트 계정 | DS-04 | EOA 이후 전환이며 같은 12주 범위 안에 유지 |
| FX | DS-05 | 자산 쌍·가격·현물/파생 모델 선택 필요 |
| Perpetual | DS-05 | 가격·증거금·포지션·펀딩·청산·keeper 모두 연결 |
| STO | DS-06 | 시험 발행물의 자격/권리/전송 조건 필요 |
| DID | DS-06 | 발급자·검증·철회·공개 정보 경계 필요 |
| x402 | DS-06 | 토큰 메커니즘·유료 자원·facilitator·gas payer 검증 필요 |

13번 DeFi/FX/Perpetual 서비스는 위 계약의 호출 성공 외에도 앱의 견적·실행·이력/포지션 흐름을 검증해야 한다. 계약 증거 하나를 서비스와 앱 완료 증거로 중복 계산하지 않는다.

확정된 순서는 USDC 우선, 시연 더미 토큰 활용, EOA 먼저/스마트 계정 이후, 고객 gas 부담 먼저/운영자 후원 이후다. 이번 점검으로 후원을 초기 필수로 바꾸거나 실제 자산 운영을 승인하지 않는다.

## 4. 권장 상세 설계 묶음 8개

아래 순서는 **설계 입력과 공통 기반을 고려한 권장 순서**다. 주차별 일정이나 사람별 작업 배정이 아니며, 모든 문서 작업을 반드시 직렬로 해야 한다는 뜻도 아니다. 선택이 필요한 정책을 임의로 확정하지 않고 가능한 경계·선택표를 먼저 구체화한다.

### DS-01 · 기기 동시동작·FOTA 공존 설계

우선 이유: 여러 필수 기능이 같은 보드/전송/저장을 사용한다. 수치나 SDK를 고르지 않고도 공존 계약을 먼저 구체화할 수 있다.

작성할 산출물:

- 서명/패스키/녹음/FOTA/반납의 허용·대기·중단 상태표
- CPU/메모리/BLE/플래시 소유권·측정 변수
- 앱/기기/서버 오류·재연결 및 펌웨어 migration 증거 계획

설계 선행: 다른 묶음의 완료 없이 독립적으로 구체화 가능. 주요 결정: D05, D06, D07.

기존 작업 연결: 19개를 이 묶음의 주된 설계 검토 위치로 매핑했다. 작업·공수의 증감이나 담당자 배정은 아니다.

### DS-02 · 소셜 로그인·두 지갑·MPC 복구 연결

우선 이유: 로그인이 서명/복구 권한을 대신하지 않도록 전체 앱 신뢰 경계를 확정할 필요가 있다.

작성할 산출물:

- 제공자별 계정/세션 전이
- MPC 참여자/복구 선택표와 구현무관 요구
- HW/Cloud 분리 및 계정변경 승인 영향

설계 선행: 다른 묶음의 완료 없이 독립적으로 구체화 가능. 주요 결정: D01, D03, D04.

기존 작업 연결: 13개를 이 묶음의 주된 설계 검토 위치로 매핑했다. 작업·공수의 증감이나 담당자 배정은 아니다.

### DS-03 · 키오스크·결제·환불·스탬프·정산 종단 연결

우선 이유: 이미 상세한 승인·대사를 유지하면서 카페의 실제 고객/점주 흐름을 닫는다.

작성할 산출물:

- 현재 승인/대사 후보를 점주/고객 화면에 연결
- 주문·주소·환불/정산 정책 선택표
- EOA 고객가스 결제의 실제 수용 입력

설계 선행: DS-01, DS-02. 주요 결정: D01, D08, D09.

기존 작업 연결: 15개를 이 묶음의 주된 설계 검토 위치로 매핑했다. 작업·공수의 증감이나 담당자 배정은 아니다.

### DS-04 · StableNet 자산·스마트계정·Indexer 호환

우선 이유: 시장·자격 기능들이 동일한 signer와 관측 입력을 사용하도록 연결한다.

작성할 산출물:

- 자산/gas/ABI/주소/버전·decoder manifest
- EOA→스마트계정 주소/권한/자산 전환 선택표
- 9개영역 관측·보정 계약

설계 선행: DS-02. 주요 결정: D10, D11.

기존 작업 연결: 13개를 이 묶음의 주된 설계 검토 위치로 매핑했다. 작업·공수의 증감이나 담당자 배정은 아니다.

### DS-05 · DeFi·FX·Perpetual 상품별 상태/위험 계약

우선 이유: 계약 이름만으로 앱 서비스 범위가 결정되지 않으므로 상품별 동작을 명확히 한다.

작성할 산출물:

- 대표 상품/자산쌍 선택 항목
- 견적/실행/포지션·펀딩·청산 상태
- 가격 장애/중복 keeper·정밀도 수용기준

설계 선행: DS-04. 주요 결정: D12, D13.

기존 작업 연결: 12개를 이 묶음의 주된 설계 검토 위치로 매핑했다. 작업·공수의 증감이나 담당자 배정은 아니다.

### DS-06 · DID·STO·x402 자격/지급 계약

우선 이유: 세 영역을 별도 수명주기로 설계한다. DID/STO 조건을 x402 결제에 자동 강제하지 않는다.

작성할 산출물:

- 발급자/검증자/권리·자격 제시/철회
- 유료 자원/토큰 메커니즘·gas payer 선택표
- 소비/실패/철회별 앱/체인 증거

설계 선행: DS-04. 주요 결정: D14, D15, D16.

기존 작업 연결: 9개를 이 묶음의 주된 설계 검토 위치로 매핑했다. 작업·공수의 증감이나 담당자 배정은 아니다.

### DS-07 · 녹음 처리·여행 데이터·AI 코스

우선 이유: 개인 데이터와 실제/시험 출처를 유지하며 일상 사용 흐름을 구체화한다.

작성할 산출물:

- 동의·파일/위치/후기 출처·삭제 경계
- 코스와 챌린지 근거/평가 표본
- 결제 보정이 여행/혜택에 미치는 영향

설계 선행: DS-01, DS-03. 주요 결정: D07, D17, D18.

기존 작업 연결: 8개를 이 묶음의 주된 설계 검토 위치로 매핑했다. 작업·공수의 증감이나 담당자 배정은 아니다.

### DS-08 · 전체 운영·배포·수용 증거 정리

우선 이유: 운영 요구는 각 묶음에서 일찍 수집하고, 마지막 통합 판정만 전체 입력에 의존한다.

작성할 산출물:

- 전체 요구별 실제 증거 인덱스
- 운영 접근/릴리스·중단/복구/인수 기준
- 남은 설계와 구현/실기 게이트 분리

설계 선행: DS-01, DS-02, DS-03, DS-04, DS-05, DS-06, DS-07. 주요 결정: D10, D19.

기존 작업 연결: 15개를 이 묶음의 주된 설계 검토 위치로 매핑했다. 작업·공수의 증감이나 담당자 배정은 아니다.

## 5. 가장 먼저 작성할 DS-01의 범위

다음 작업에서는 아래 표를 구체화한다. Zephyr 사용은 확정했지만 SDK/board target·부트로더·암호 backend·BLE/음성 profile은 선택하지 않은 상태를 유지한다. 사용자는 기기 검증을 실제 개발 단계에 해도 된다고 했으므로 지금 보드 연결이나 SDK 설치를 진행하지 않는다.

| 설계 축 | 구체화할 질문 |
| --- | --- |
| 기능 공존 | 서명·패스키·녹음·찾기·FOTA·import·반납 중 무엇을 동시에 허용/대기/차단할까? |
| 자원 소유권 | 키 backend, flash, audio buffer, BLE 연결/대역폭, 화면/버튼은 어떤 서비스가 점유할까? |
| 중단 의미 | 이미 서명/허가가 생성됐거나 녹음 일부가 전송됐을 때 중단을 어떻게 표시할까? |
| FOTA 상태 | 업데이트 준비·전송·적용·새 부팅 확인과 기존 진행 작업을 어떻게 연결할까? |
| 영속 정보 | 키·설정·대여 epoch·reset 저널·취소/continuation 표식을 어떤 호환 조건으로 보존할까? |
| 앱·운영 안내 | busy/보류/재개/복구필요를 누가 판단하고 어디서 조회할까? |
| 개발 단계 증거 | 자원/배터리/전송 측정, 전원 중단, 잘못된 이미지·호환 거절에서 무엇을 기록할까? |

CPU/메모리·속도·녹음 시간 같은 수치를 추정해 확정하지 않는다. 먼저 변수·측정 위치·허용 상태와 실패 시 동작을 설계하고 실제 목표/측정은 해당 결정과 검증 단계에서 연결한다.

## 6. 결정과 검증을 분리해서 관리

기존 결정 D01..D19는 모두 open으로 유지한다. 요구별 참조는 **원 WBS에서 유래한 taskDecisionRefs**와 이번 점검의 명시적 **policyDecisionRefs**를 구분하고, decisionRefs는 두 집합의 합으로 기록한다. 예를 들어 FOTA와 Indexer의 운영/복구 목표는 D19에도 연결되지만 원 WBS를 고쳐서 그렇게 보이게 만들지는 않는다.

정책 입력은 해당 설계에서 선택이 실제로 필요할 때 사용자에게 묻는다. 기술 구조를 정리할 수 있는 부분은 먼저 진행한다. 현재 미답변인 RR-DEC-01은 반복 질문하거나 ‘다음 진행’을 답변으로 간주하지 않는다. 같은 D03/D09에 속한 다른 세부 결정도 별개의 입력으로 관리한다.

| 종류 | 지금 할 수 있는 일 | 아직 완료라 할 수 없는 일 |
| --- | --- | --- |
| 엔지니어링 설계 | 책임·상태·권한·호환 조건·오류·증거 계약 작성 | 미선정 profile의 실제 동작 보장 |
| 사용자 정책 | 선택에 필요한 옵션과 영향 정리 | 임계값·복구/상품/보존 정책 임의 확정 |
| 개발 검증 | 실기/앱/체인 시험 계획과 입력 명세 | 시험을 실행하지 않고 passed 표시 |
| 기준 채택 | 원본과 후보의 차이·의존성·이행 조건 정리 | 검토 후보를 이미 배포된 API/DB처럼 취급 |

## 7. 최근 대여 설계의 잔여 항목

대여 영역을 더 상세화했어도 다음 다섯 항목은 열려 있다. 전체 범위 점검으로 이동하면서 잊지 않도록 원 종단 명세와 동일하게 유지했다.

- **LJG-01 신규 여행 지갑 복구 정책**: LJ07 configured new_travel destructive branch held; denial scenario specified, no inferred decision
- **LJG-02 원 holder 키 상실/철회 후 재승인**: LJ05/06 safe hold defined, successful replacement/reconsent not designed
- **LJG-03 staged/unknown 등록의 안전한 회수**: no automatic reuse; support/reset completion path still needed
- **LJG-04 실제 profile과 출고 root/erase/영속 기능**: all real-device success paths await selected profile and evidence
- **LJG-05 최종 wire/SQL/권한/화면 공동 채택**: route aliases and logical fields not yet callable implementation contracts

이 항목들은 관련 묶음에서 입력을 모아 보완한다. 안전하게 보류하는 동작의 설계가 성공적인 키 분실 복구나 회수 완료를 대신하지 않는다.

## 8. 검증 범위

검증기는 다음을 확인한다.

- 요구 번호 1~15가 모두 있고 각 원 WBS task/decision 참조가 일치함
- 3번의 네 기능, 11번의 아홉 계약 영역, 13번의 세 서비스 영역이 보존됨
- 기존 104개 작업이 하나씩 주 설계 묶음에 연결되고 세부 작업 320개는 변경되지 않음
- 설계 묶음 8개의 의존성에 순환이 없고 기존 19개 결정이 미선택 상태임
- 근거 문서 21개의 해시와 대여 잔여 항목이 보존됨

주 설계 묶음은 리뷰 위치이지 구현팀·독립 배포 단위가 아니다. 여러 묶음이 동일 기능의 입력을 참조할 수 있다. 이 검사는 완료율·일정 가능성·실제 제품 동작을 증명하지 않는다.

**다음 작업:** DS-01 기기 동시동작·FOTA 공존 상태표와 자원/중단/복구 계약을 작성한다. 구현은 계속 보류한다.

## 부록 · 요구별 결정·실행 증거

### 요구 1 · 실기 HW wallet

정책 입력: D03 키 통제/복구; D05 보드/부트 조합.

개발 단계에 필요한 증거: NU 실제 주소/서명·잘못된 요청 거절; 저장 보호·전원중단.

근거: [implementation-interfaces.json](../specifications/implementation-interfaces.json), [approval-baseline.json](../specifications/approval-baseline.json), [wallet-control-recovery-design.json](../specifications/wallet-control-recovery-design.json), [lifecycle-journey-acceptance.json](../specifications/lifecycle-journey-acceptance.json), [technology-selection.json](technology-selection.json).

### 요구 2 · 실기 FOTA

정책 입력: D05 배포/복구 정책; D19 운영 목표.

개발 단계에 필요한 증거: 실제 무선 업데이트·손상 이미지 거절; 전원중단 후 원 키와 표식 보존.

근거: [implementation-interfaces.json](../specifications/implementation-interfaces.json), [technology-selection.json](technology-selection.json), [functional-execution-spec.md](functional-execution-spec.md), [lifecycle-storage-design.json](../specifications/lifecycle-storage-design.json).

### 요구 3 · 패스키·녹음·찾기·결제 스탬프

정책 입력: D06 대상 OS/RP·사용자 검증; D07 녹음/찾기 목표; D09 스탬프 정책.

개발 단계에 필요한 증거: 표준 RP 등록·인증; 마이크→BLE→폰 파일→전사; 실기 찾기 및 결제 스탬프 중복/환불.

근거: [implementation-interfaces.json](../specifications/implementation-interfaces.json), [technology-selection.json](technology-selection.json), [functional-execution-spec.md](functional-execution-spec.md), [commerce-consumer-repair-design.json](../specifications/commerce-consumer-repair-design.json), [lifecycle-journey-acceptance.json](../specifications/lifecycle-journey-acceptance.json).

### 요구 4 · 유저 앱

정책 입력: D01 앱 OS/배포 대상; D03 지갑 선택·복구 UX.

개발 단계에 필요한 증거: 선택 OS 설치·권한거절·재로그인; 실제 서비스·기기 결과 화면 연결.

근거: [functional-execution-spec.md](functional-execution-spec.md), [implementation-interfaces.json](../specifications/implementation-interfaces.json), [extended-dtos.schema.json](../specifications/extended-dtos.schema.json), [approval-baseline.json](../specifications/approval-baseline.json), [lifecycle-bootstrap-contracts.json](../specifications/lifecycle-bootstrap-contracts.json).

### 요구 5 · Google·Apple 소셜 로그인

정책 입력: D01 인증 대상 플랫폼; D03/D04/D19 복구·탈퇴 정책.

개발 단계에 필요한 증거: 각 제공자 실제 로그인·중복/만료/탈퇴; 이메일 일치만으로 자동 계정병합 안 함.

근거: [security-api-integration.md](../specifications/security-api-integration.md), [technology-selection.json](technology-selection.json), [functional-execution-spec.md](functional-execution-spec.md), [implementation-interfaces.json](../specifications/implementation-interfaces.json).

### 요구 6 · 소셜 계정 Cloud Wallet MPC

정책 입력: D04 임계값/참여자/복구 책임; D03 키 통제 선택.

개발 단계에 필요한 증거: 실제 분산 서명·참여자 장애·회전/복구; 전체키 재조립 없는 프로토콜 증거.

근거: [implementation-interfaces.json](../specifications/implementation-interfaces.json), [technology-selection.json](technology-selection.json), [wallet-control-recovery-design.json](../specifications/wallet-control-recovery-design.json), [approval-baseline.json](../specifications/approval-baseline.json), [expanded-scope-reuse-inventory.md](../expanded-scope-reuse-inventory.md).

### 요구 7 · 위치 검색·결제 기록·발자취

정책 입력: D17 실제/시험 데이터·위치 보관; D01 여행자 모드 대상.

개발 단계에 필요한 증거: 실제 장소/위치와 이력 연결; 삭제/결제 보정 전파.

근거: [functional-execution-spec.md](functional-execution-spec.md), [commerce-consumer-repair-design.json](../specifications/commerce-consumer-repair-design.json), [settlement-ops-contracts.json](../specifications/settlement-ops-contracts.json), [technology-selection.json](technology-selection.json).

### 요구 8 · 앱에서 실제기기 설정

정책 입력: D03/RR-DEC-01 복구 정책; D05/D06/D07 기능 profile.

개발 단계에 필요한 증거: 올바른 기기/수령인 연결·이전 사용자 격리; 실기 import/FOTA/반납 재시도.

근거: [implementation-interfaces.json](../specifications/implementation-interfaces.json), [approval-baseline.json](../specifications/approval-baseline.json), [lifecycle-bootstrap-contracts.json](../specifications/lifecycle-bootstrap-contracts.json), [lifecycle-journey-acceptance.json](../specifications/lifecycle-journey-acceptance.json), [lifecycle-storage-design.json](../specifications/lifecycle-storage-design.json).

### 요구 9 · RN 키오스크·점주 매장 운영

정책 입력: D08 확정/금액/환불/정산 정책; D01 태블릿/로그인 대상.

개발 단계에 필요한 증거: RN 태블릿→NU 서명→체인→주문/매출; 다른 매장 접근차단·환불 별도 서명.

근거: [functional-execution-spec.md](functional-execution-spec.md), [approval-baseline.json](../specifications/approval-baseline.json), [commerce-reconciliation-integration.json](../specifications/commerce-reconciliation-integration.json), [settlement-ops-contracts.json](../specifications/settlement-ops-contracts.json), [technology-selection.json](technology-selection.json).

### 요구 10 · 운영 백오피스

정책 입력: D19 운영 역할/보존/복구 목표; D05 배포 운영.

개발 단계에 필요한 증거: 실제 운영 권한·작업 추적·장애복구; 고객키/음성/위치 무차별 노출 없음.

근거: [functional-execution-spec.md](functional-execution-spec.md), [settlement-ops-contracts.json](../specifications/settlement-ops-contracts.json), [lifecycle-journey-acceptance.json](../specifications/lifecycle-journey-acceptance.json), [lifecycle-storage-design.json](../specifications/lifecycle-storage-design.json), [security-api-integration.md](../specifications/security-api-integration.md).

### 요구 11 · StableNet 테스트넷 계약군

정책 입력: D10 환경; D11 스마트계정; D12/D13 시장; D14/D15 자격; D16 유료요청.

개발 단계에 필요한 증거: 9개 각 앱 호출·실행·이벤트·실패; 더미토큰을 실USDC나 x402지원으로 오표시하지 않음.

근거: [functional-execution-spec.md](functional-execution-spec.md), [implementation-interfaces.json](../specifications/implementation-interfaces.json), [technology-selection.json](technology-selection.json), [expanded-scope-reuse-inventory.md](../expanded-scope-reuse-inventory.md), [stable-contract-reuse-plan.md](../stable-contract-reuse-plan.md), [stablenet-testnet-baseline.md](../stablenet-testnet-baseline.md).

### 요구 12 · 기존 Indexer 확장

정책 입력: D10 배포/시험 환경; D08 확정 정책; D19 운영 복구 목표.

개발 단계에 필요한 증거: 대상 소스 revision에서 backfill/reorg/중복/지연; 앱·매출·혜택·여행의 실제 보정.

근거: [implementation-interfaces.json](../specifications/implementation-interfaces.json), [technology-selection.json](technology-selection.json), [commerce-reconciliation-integration.json](../specifications/commerce-reconciliation-integration.json), [commerce-consumer-repair-design.json](../specifications/commerce-consumer-repair-design.json), [indexer-integration-and-test-token.md](../indexer-integration-and-test-token.md).

### 요구 13 · DEX·DeFi·FX·Perpetual 앱 서비스

정책 입력: D12 자산쌍/기능; D13 시장·담보·위험 규칙.

개발 단계에 필요한 증거: 앱에서 실제 swap/LP/FX/포지션·이벤트 확인; 오라클/펀딩/청산 정상 및 실패.

근거: [functional-execution-spec.md](functional-execution-spec.md), [implementation-interfaces.json](../specifications/implementation-interfaces.json), [technology-selection.json](technology-selection.json), [expanded-scope-reuse-inventory.md](../expanded-scope-reuse-inventory.md).

### 요구 14 · AI 여행 추천·챌린지·발도장

정책 입력: D17 출처/동의/보관; D18 추천/완료/보상.

개발 단계에 필요한 증거: 근거 있는 실제 앱 코스; 위치거절·삭제·중복보상·결제정정.

근거: [functional-execution-spec.md](functional-execution-spec.md), [commerce-consumer-repair-design.json](../specifications/commerce-consumer-repair-design.json), [technology-selection.json](technology-selection.json), [implementation-interfaces.json](../specifications/implementation-interfaces.json).

### 요구 15 · 필수 운영·검증·배포 도구

정책 입력: D10 실행환경; D19 품질/복구/보존.

개발 단계에 필요한 증거: 빌드·설치·배포·복구 재현; 전체15요구의 개별 실증거.

근거: [functional-execution-spec.md](functional-execution-spec.md), [technology-selection.json](technology-selection.json), [lifecycle-journey-acceptance.json](../specifications/lifecycle-journey-acceptance.json), [lifecycle-storage-design.json](../specifications/lifecycle-storage-design.json), [execution-readiness.md](execution-readiness.md).


## LG-05 반영: 선언 범위와 전이 영향

requirementRefs는 declaredScopeRequirementRefs의 호환 별칭이다. 실제 변경 영향은 transitiveTaskImpactRequirementRefs로 별도 추적한다.

| 설계 | 주요 선언 범위 | 작업으로부터 계산한 영향 |
| --- | --- | --- |
| DS-01 | [1, 2, 3, 4, 8, 10] | [1, 2, 3, 4, 8, 9, 10, 15] |
| DS-02 | [4, 5, 6, 8] | [4, 5, 6, 7, 8, 9, 10, 15] |
| DS-03 | [1, 3, 7, 9, 10, 12] | [1, 3, 4, 7, 8, 9, 10, 12, 14, 15] |
| DS-04 | [11, 12, 13] | [4, 8, 9, 10, 11, 12, 15] |
| DS-05 | [11, 13] | [11, 13] |
| DS-06 | [11] | [11] |
| DS-07 | [3, 7, 14] | [3, 4, 7, 14] |
| DS-08 | [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15] | [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15] |

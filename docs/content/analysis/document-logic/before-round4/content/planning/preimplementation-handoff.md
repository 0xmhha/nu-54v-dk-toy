# 구현 전 설계 인계서

2026-09-19 · 15개 요구 / 104개 작업 / 320개 세부 작업 / 8개 설계 묶음

추가로 독립 작성 가능한 DS06/07/08와 제품 간 논리계약·운영/인수 기준을 작성했다. 8개 설계 묶음 전체의 논리 초안과 104개 작업 인계가 연결되었다. 모든 상세 설계가 확정되거나 즉시 구현 가능한 상태는 아니다. 프로토콜·공급자·보드/상품 모델 선택에 따른 wire/물리/성능 설계 고정, 후보 카탈로그 채택, 실제 검증은 남아 있다.

개인 역할 배정과 공수 산정은 하지 않았다. 펌웨어는 Zephyr, 키오스크는 React Native, HW와Cloud MPC는 별개 지갑이며 EOA·고객가스 우선 조건을 유지한다.

## 읽는 순서

1. 이 문서의 단계 구분·남은 조건을 확인한다.
2. [전체 작업 인계표](preimplementation-task-handoffs.md)에서 작업별 입력·선행작업·검증 조건을 찾는다.
3. 아래 DS 상세 문서와 [통합 계약 보완안](../specifications/preimplementation-contract-overlay.md)을 함께 읽는다.
4. [기존 실행 준비표](execution-readiness.md), [기존 통합 순서](implementation-sequence.md)의 독립 구현/통합/완료 기준을 유지한다. 구현 승인 전에는 실행하지 않는다.

## 8개 상세 설계

- DS-01: [device-coexistence-design](../specifications/device-coexistence-design.md)
- DS-02: [social-wallet-recovery-design](../specifications/social-wallet-recovery-design.md)
- DS-03: [kiosk-commerce-journey-design](../specifications/kiosk-commerce-journey-design.md)
- DS-04: [stablenet-compatibility-design](../specifications/stablenet-compatibility-design.md)
- DS-05: [market-product-design](../specifications/market-product-design.md)
- DS-06: [credential-paid-resource-design](../specifications/credential-paid-resource-design.md)
- DS-07: [recording-travel-ai-design](../specifications/recording-travel-ai-design.md)
- DS-08: [operations-release-acceptance-design](../specifications/operations-release-acceptance-design.md)

## 단계별 판정

| id | gate | passWhen | current |
| --- | --- | --- | --- |
| HG-01 | 논리 설계 인계 | 요구/행위/경계/실패복구/증거목록 연결,참조검증 통과 | 이번 산출물에 후보 작성,검증 보고 별도 |
| HG-02 | 정책·기술 프로필 고정 | 19결정의선택값·근거·유효범위와미선택기능 처리 명시 | 모두 open 유지; workingProposal는 확정 아님 |
| HG-03 | 기준 계약 채택 | DI01..08/OC01..16을API/BLE/ACL/DTO/storage/source dispatch에원자적설계변경으로반영 | 후보 단계; wire/ABI/물리배치 미고정 |
| HG-04 | 구현 착수 | 사용자 구현 전환 지시+해당 task 입력 충분;미선정 부분은fixture와분리 | deferred_by_user |
| HG-05 | 실기·서비스 통합 | target/profile/실배포/권한/원천과선정정책존재 | not_run |
| HG-06 | 12주 완료 수용 | 15요구·16복합facet 모두 실제 앱+기기/체인/서비스 필요한증거와정상/실패/복구 통과 | not_run; 설계파일수나fixture pass로대체불가 |


## 15개 요구사항의 수용 연결

### 1 실기 HW wallet

- **facets**: 키 생성/import · 기기 표시·승인 · EOA 서명 · 키 저장·복구
- **taskRefs**: HW-01 · HW-02 · HW-03 · HW-04 · HW-05 · HW-06 · PAY-01 · PAY-02 · PAY-03 · PAY-04 · PAY-05 · HW-07
- **verificationTaskRefs**: VERIFY-01 · VERIFY-02 · VERIFY-03 · VERIFY-04 · VERIFY-05
- **designPackages**: DS-01 · DS-03
- **runtimeEvidenceNeeded**: NU 실제 주소/서명·잘못된 요청 거절 · 저장 보호·전원중단
- **status**: not_run

### 2 실기 FOTA

- **facets**: 서명 이미지 · 앱 전송 · 새 버전 부팅 · 실패 복구·키 보존
- **taskRefs**: OTA-01 · OTA-02 · OTA-03 · OTA-04
- **verificationTaskRefs**: VERIFY-01 · VERIFY-02 · VERIFY-03 · VERIFY-04 · VERIFY-05
- **designPackages**: DS-01
- **runtimeEvidenceNeeded**: 실제 무선 업데이트·손상 이미지 거절 · 전원중단 후 원 키와 표식 보존
- **status**: not_run

### 3 패스키·녹음·찾기·결제 스탬프

- **facets**: passkey · recorder · device_find · payment_stamp
- **taskRefs**: KEY-01 · KEY-02 · KEY-03 · REC-01 · REC-02 · REC-03 · FIND-01 · FIND-02 · STAMP-01 · STAMP-02 · STAMP-03 · STAMP-04
- **verificationTaskRefs**: VERIFY-01 · VERIFY-02 · VERIFY-03 · VERIFY-04 · VERIFY-05
- **designPackages**: DS-01 · DS-03 · DS-07 · DS-08
- **runtimeEvidenceNeeded**: 표준 RP 등록·인증 · 마이크→BLE→폰 파일→전사 · 실기 찾기 및 결제 스탬프 중복/환불
- **status**: not_run

### 4 유저 앱

- **facets**: 기기·지갑·기록·여행·상품 화면 · 실제 설치·서비스 연결
- **taskRefs**: BASE-01 · BASE-02 · BASE-03 · BASE-04 · BASE-05 · APP-01 · APP-02 · APP-03 · APP-04 · HW-01 · HW-02 · HW-03 · HW-04 · HW-05 · HW-06 · REC-01 · REC-02 · REC-03 · BASE-06 · HW-07
- **verificationTaskRefs**: VERIFY-01 · VERIFY-02 · VERIFY-03 · VERIFY-04 · VERIFY-05
- **designPackages**: DS-01 · DS-02 · DS-03 · DS-04 · DS-07 · DS-08
- **runtimeEvidenceNeeded**: 선택 OS 설치·권한거절·재로그인 · 실제 서비스·기기 결과 화면 연결
- **status**: not_run

### 5 Google·Apple 소셜 로그인

- **facets**: Google 가입/로그인 · Apple 가입/로그인 · 연결·로그아웃·탈퇴
- **taskRefs**: AUTH-01 · AUTH-02 · AUTH-03 · AUTH-04 · MPC-01 · MPC-02 · MPC-03 · MPC-04 · MPC-05
- **verificationTaskRefs**: VERIFY-01 · VERIFY-02 · VERIFY-03 · VERIFY-04 · VERIFY-05
- **designPackages**: DS-02
- **runtimeEvidenceNeeded**: 각 제공자 실제 로그인·중복/만료/탈퇴 · 이메일 일치만으로 자동 계정병합 안 함
- **status**: not_run

### 6 소셜 계정 Cloud Wallet MPC

- **facets**: 분산 키 생성 · 임계 서명 · 참여자 장애 · 기기변경·복구·회전
- **taskRefs**: AUTH-01 · AUTH-02 · AUTH-03 · AUTH-04 · MPC-01 · MPC-02 · MPC-03 · MPC-04 · MPC-05
- **verificationTaskRefs**: VERIFY-01 · VERIFY-02 · VERIFY-03 · VERIFY-04 · VERIFY-05
- **designPackages**: DS-02
- **runtimeEvidenceNeeded**: 실제 분산 서명·참여자 장애·회전/복구 · 전체키 재조립 없는 프로토콜 증거
- **status**: not_run

### 7 위치 검색·결제 기록·발자취

- **facets**: 맛집 검색 · 개인 결제/환불 기록 · 여행자 모드 · 발자취
- **taskRefs**: APP-01 · APP-02 · APP-03 · APP-04 · PAY-01 · PAY-02 · PAY-03 · PAY-04 · PAY-05 · STAMP-01 · STAMP-02 · STAMP-03 · TRIP-01 · TRIP-02 · TRIP-03 · STAMP-04 · TRIP-04
- **verificationTaskRefs**: VERIFY-01 · VERIFY-02 · VERIFY-03 · VERIFY-04 · VERIFY-05
- **designPackages**: DS-02 · DS-03 · DS-07 · DS-08
- **runtimeEvidenceNeeded**: 실제 장소/위치와 이력 연결 · 삭제/결제 보정 전파
- **status**: not_run

### 8 앱에서 실제기기 설정

- **facets**: 등록·페어링 · 생성/import · FOTA·찾기 · 연결해제·반납
- **taskRefs**: BASE-01 · BASE-02 · BASE-03 · BASE-04 · BASE-05 · APP-01 · APP-02 · APP-03 · APP-04 · HW-01 · HW-02 · HW-03 · HW-04 · HW-05 · HW-06 · OTA-01 · OTA-02 · OTA-03 · KEY-01 · KEY-02 · KEY-03 · FIND-01 · FIND-02 · STAMP-01 · STAMP-02 · STAMP-03 · BASE-06 · HW-07 · OTA-04 · STAMP-04
- **verificationTaskRefs**: VERIFY-01 · VERIFY-02 · VERIFY-03 · VERIFY-04 · VERIFY-05
- **designPackages**: DS-01 · DS-02 · DS-03 · DS-04 · DS-08
- **runtimeEvidenceNeeded**: 올바른 기기/수령인 연결·이전 사용자 격리 · 실기 import/FOTA/반납 재시도
- **status**: not_run

### 9 RN 키오스크·점주 매장 운영

- **facets**: Google/Apple 매장 로그인 · 메뉴·주문 · USDC 우선 결제 · 환불·매출·코인 정산대조
- **taskRefs**: BASE-01 · BASE-02 · BASE-03 · BASE-04 · BASE-05 · AUTH-01 · AUTH-02 · AUTH-03 · AUTH-04 · PAY-01 · PAY-02 · PAY-03 · PAY-04 · PAY-05 · SHOP-01 · SHOP-02 · SHOP-03 · SHOP-04 · SHOP-05 · BASE-06 · SHOP-06
- **verificationTaskRefs**: VERIFY-01 · VERIFY-02 · VERIFY-03 · VERIFY-04 · VERIFY-05
- **designPackages**: DS-01 · DS-02 · DS-03 · DS-04 · DS-08
- **runtimeEvidenceNeeded**: RN 태블릿→NU 서명→체인→주문/매출 · 다른 매장 접근차단·환불 별도 서명
- **status**: not_run

### 10 운영 백오피스

- **facets**: 매장·계정/권한 · 대여·FOTA 릴리스 · 거래 예외·서비스 상태 · 감사
- **taskRefs**: BASE-01 · BASE-02 · BASE-03 · BASE-04 · BASE-05 · AUTH-01 · AUTH-02 · AUTH-03 · AUTH-04 · OTA-01 · OTA-02 · OTA-03 · STAMP-01 · STAMP-02 · STAMP-03 · OPS-01 · OPS-02 · OPS-03 · BASE-06 · OTA-04 · STAMP-04
- **verificationTaskRefs**: VERIFY-01 · VERIFY-02 · VERIFY-03 · VERIFY-04 · VERIFY-05
- **designPackages**: DS-01 · DS-02 · DS-03 · DS-04 · DS-08
- **runtimeEvidenceNeeded**: 실제 운영 권한·작업 추적·장애복구 · 고객키/음성/위치 무차별 노출 없음
- **status**: not_run

### 11 StableNet 테스트넷 계약군

- **facets**: USDC/더미 토큰 · WKRC native/wrapped 구분 · DeFi · smart_account · FX · perpetual · STO · DID · x402
- **taskRefs**: TOKEN-01 · TOKEN-02 · TOKEN-03 · SMART-01 · SMART-02 · SMART-03 · DEX-01 · DEX-02 · DEX-03 · FX-01 · FX-02 · FX-03 · PERP-01 · PERP-02 · PERP-03 · PERP-04 · PERP-05 · STO-01 · STO-02 · STO-03 · DID-01 · DID-02 · DID-03 · X402-01 · X402-02 · X402-03 · PERP-06
- **verificationTaskRefs**: VERIFY-01 · VERIFY-02 · VERIFY-03 · VERIFY-04 · VERIFY-05
- **designPackages**: DS-04 · DS-05 · DS-06
- **runtimeEvidenceNeeded**: 9개 각 앱 호출·실행·이벤트·실패 · 더미토큰을 실USDC나 x402지원으로 오표시하지 않음
- **status**: not_run

### 12 기존 Indexer 확장

- **facets**: 체인/배포 등록 · 조회 SDK · backfill·재구성 · 확장 이벤트 · 앱/업무 보정
- **taskRefs**: INDEX-01 · INDEX-02 · INDEX-03 · INDEX-04 · INDEX-05 · PAY-01 · PAY-02 · PAY-03 · PAY-04 · PAY-05 · INDEX-06
- **verificationTaskRefs**: VERIFY-01 · VERIFY-02 · VERIFY-03 · VERIFY-04 · VERIFY-05
- **designPackages**: DS-03 · DS-04
- **runtimeEvidenceNeeded**: 대상 소스 revision에서 backfill/reorg/중복/지연 · 앱·매출·혜택·여행의 실제 보정
- **status**: not_run

### 13 DEX·DeFi·FX·Perpetual 앱 서비스

- **facets**: DeFi 실행/유동성 · FX 견적·교환 · Perpetual 포지션·펀딩·청산
- **taskRefs**: DEX-01 · DEX-02 · DEX-03 · FX-01 · FX-02 · FX-03 · PERP-01 · PERP-02 · PERP-03 · PERP-04 · PERP-05 · PERP-06
- **verificationTaskRefs**: VERIFY-01 · VERIFY-02 · VERIFY-03 · VERIFY-04 · VERIFY-05
- **designPackages**: DS-05
- **runtimeEvidenceNeeded**: 앱에서 실제 swap/LP/FX/포지션·이벤트 확인 · 오라클/펀딩/청산 정상 및 실패
- **status**: not_run

### 14 AI 여행 추천·챌린지·발도장

- **facets**: 위치+후기+결제 추천 · 코스 생성/수정/저장 · 따라하기 챌린지 · 발도장/보상
- **taskRefs**: STAMP-01 · STAMP-02 · STAMP-03 · TRIP-01 · TRIP-02 · TRIP-03 · AI-01 · AI-02 · AI-03 · STAMP-04 · TRIP-04
- **verificationTaskRefs**: VERIFY-01 · VERIFY-02 · VERIFY-03 · VERIFY-04 · VERIFY-05
- **designPackages**: DS-03 · DS-07 · DS-08
- **runtimeEvidenceNeeded**: 근거 있는 실제 앱 코스 · 위치거절·삭제·중복보상·결제정정
- **status**: not_run

### 15 필수 운영·검증·배포 도구

- **facets**: 환경/비밀 · 주소/ABI 등록 · CI/재현빌드 · 시험·관측·백업·인수
- **taskRefs**: BASE-01 · BASE-02 · BASE-03 · BASE-04 · BASE-05 · OPS-01 · OPS-02 · OPS-03 · RELEASE-01 · RELEASE-02 · RELEASE-03 · BASE-06
- **verificationTaskRefs**: VERIFY-01 · VERIFY-02 · VERIFY-03 · VERIFY-04 · VERIFY-05
- **designPackages**: DS-01 · DS-02 · DS-03 · DS-04 · DS-08
- **runtimeEvidenceNeeded**: 빌드·설치·배포·복구 재현 · 전체15요구의 개별 실증거
- **status**: not_run


## 제품 간 최종 시연·실패 복구

| id | flow | requiredProof | failureRecovery |
| --- | --- | --- | --- |
| EA-01 | 소셜가입→Cloud MPC→서명→복구 | Google/Apple 각각actuallogin+분산생성/서명/참여자교체·앱상태 | 탈취session/중단복구/lateworker/threshold불충분 |
| EA-02 | 대여→HW신규/import→지급→반납 | 실NU기기키동작+폰설정+같은주문실지급+키삭제/재대여 | BLE단절/import실패·unknown송금·RR선택된복구절차 |
| EA-03 | RN키오스크→카페주문→HW승인→매출 | 실태블릿/가게역할·메뉴/수취주소snapshot·chain/indexer·앱영수증 | 중복/오입금/응답유실/가격변경/chain reorg |
| EA-04 | 부분환불→스탬프→정산→백오피스 | 실점주signature/원지급배분·원장보정·음수가능정산차이 | 환불미확정/사용한혜택/마감뒤보정/무권한publish |
| EA-05 | passkey·녹음·찾기·스탬프+FOTA | 각실동작과지원RP/전송/폰profile·동시동작/실부트 | 녹음gap/권한거절/거리unknown/FOTA전원중단과키보존 |
| EA-06 | EOA→스마트계정 | 실주소/권한/자산전환표시와실UserOp·개별효과 | bundle성공/내부실패·allowance잔존·latehold |
| EA-07 | USDC대체·WKRC·DeFi·FX·Perp | 9영역중각상품실계약+앱결과·수량/단위/가스·risk | unsupportedtoken/staleprice·청산/keeper중복·reorg |
| EA-08 | DID→STO자격→시험권리동작 | 실발급/제시/검증/철회+contract직접호출제약 | revocation race·unknownstatus·부적격이전 |
| EA-09 | x402→유료결과→재조회 | 실HTTP402/proof/settlement+하나의entitlement/결과 | 결제후응답유실·provider실패·중복재시도 |
| EA-10 | 기기녹음→폰→전사/AI→삭제 | 실원음구간·보호결과·현재동의·삭제대상증거 | 부분녹음/외부pending/backuprestore |
| EA-11 | 맛집→구매후기→코스→챌린지 | 장소출처·실제/시험/방문 구분·valid코스·보상원장 | 위치거절/없는장소/삭제입력/환불보정 |
| EA-12 | 전체릴리스→백업복구→운영인계 | manifest/digest·실복구로그·15요구별evidenceindex | 오래된권한/삭제자료부활/중복금융효과 차단 |


## 남은 조건과 실제 종결 증거

### DR-01 

- **kind**: user_policy
- **remaining**: D01..19 및RR-DEC01선택
- **preparedNow**: 원질문/제안/영향작업/결정자료를ledger에그대로연결
- **closureEvidence**: 선택값+범위+근거+사용자결정 기록;계속진행을승인으로간주금지

### DR-02 

- **kind**: profile_specific_design
- **remaining**: board/pins/memory·wirecodec·MPC/VC/x402suite·contract상품모델
- **preparedNow**: 추상 인터페이스·필수필드·상태·거절경계·profile입력목록
- **closureEvidence**: 선정profile에기초한핀맵/slotlayout/schema/ABI/testvectors

### DR-03 

- **kind**: canonical_adoption
- **remaining**: 미병합DI/lifecycle/DS/OC 후보의카탈로그반영
- **preparedNow**: OC16경계와채택절차·sourcehash·원계약보존조건
- **closureEvidence**: 변경 API/BLE/ACL/DTO/storage/source-reader를하나의checkpoint로대조

### DR-04 

- **kind**: external_configuration
- **remaining**: OAuthclient·MPC/AI/지도provider·실chain배포·indexerendpoint
- **preparedNow**: environmentmanifest필수입력·서비스책임·오류/복구계약
- **closureEvidence**: 실존endpoint/account/address의권한있는등록+capabilityevidence

### DR-05 

- **kind**: implementation_runtime
- **remaining**: 실기/앱/계약/서비스/성능/운영복구
- **preparedNow**: 104task와320step·실행상태미실행·evidence형식
- **closureEvidence**: 제품구현후kind별실측증거;문서검사로대체불가

### DR-06 

- **kind**: commercial_operation
- **remaining**: 실자산·개인정보·발행물권리의실제운영조건
- **preparedNow**: 실자금/키통제도·서비스별역할·검토질문 아래정리
- **closureEvidence**: 실제사업모델/관할/약관/공급자계약에대한전문검토·출시결정

## 19개 결정의 인계

원본 결정은 모두 open이다. 아래는 기존 제안과 필요한 산출물이며 선택 완료 기록이 아니다.

### D01 앱·인증 대상

제안: 유저 앱도 RN 공통 기반을 검토하고 키오스크와 인증/금액/거래 상태 모듈을 공유한다. 화면·기기 드라이버는 제품별로 분리한다. S25 Ultra에서 실제 실행하고 한국어/영어 화면을 제안한다.

선택 후 산출물: 지원할 추가 OS·태블릿 모델·배포 경로와 언어를 지정하고 플랫폼별 Google/Apple 인증을 확인한다.

영향 작업: BASE-01 · AUTH-02 · AUTH-03 · APP-01 · SHOP-01 · TRIP-03.

### D02 연결 규칙·데이터 경계

제안: 계정·지갑·기기·매장·주문·지급을 별도 식별자로 관리한다. 요청 ID/멱등키/버전/만료/오류를 공통 봉투로 두고, 체인 관측과 업무 확정을 분리한다.

선택 후 산출물: functional-execution-spec.md의 데이터/상태 표를 API·BLE 예제로 고정한다.

영향 작업: BASE-01 · BASE-02 · BASE-03 · AUTH-01 · HW-03 · INDEX-02.

### D03 지갑 키·복구

제안: HW와 Cloud를 독립 signer/주소로 계정에 연결하고 화면에서 명시적으로 선택한다. HW의 신규 생성과 import 모두 유지한다.

선택 후 산출물: 파생 경로·백업/복구 UX·잠금 정책과 스마트 계정에서 두 signer를 쓰는 조합을 결정한다.

영향 작업: BASE-03 · AUTH-04 · APP-02 · HW-02 · HW-03 · HW-04 · HW-05 · MPC-01 · STAMP-03 · VERIFY-01 · BASE-06 · STAMP-04.

### D04 MPC 신뢰 구조

제안: 검증된 MPC 제공 경로나 라이브러리를 비교하고 실제 분산 생성/서명/참여자 교체를 실험한다. 소셜 로그인만으로 자금 서명 권한을 부여하지 않는다.

선택 후 산출물: 제공 경로·참여자·임계값·키 조각 저장 위치·복구 증명·서비스 중단 시 접근 정책을 선택한다.

영향 작업: AUTH-04 · MPC-01 · MPC-02 · MPC-03 · MPC-04 · VERIFY-01 · BASE-06 · MPC-05.

### D05 FOTA 운영

제안: 서명된 이미지와 manifest를 앱이 전달하고 부트 단계에서 검증한다. 중단 복구와 키/설정 보존을 함께 구현한다.

선택 후 산출물: 실제 보드의 슬롯·부트로더·메모리를 검증해 저장 배치, 복구 경로, 서명 키 운영을 고정한다.

영향 작업: HW-01 · OTA-01 · OTA-02 · OTA-03 · OPS-02 · VERIFY-02 · HW-07 · OTA-04.

### D06 패스키 호환 범위

제안: 대상 RP/OS/브라우저/전송 행렬부터 만들고 실제 표준 등록/인증 경로를 검증한다. 자체 근접 서명은 별도 기능으로 추적한다.

선택 후 산출물: 서비스·전송·사용자 검증 방식과 분실/반납 후 대체 로그인 정책을 지정한다.

영향 작업: KEY-01 · KEY-02 · KEY-03 · VERIFY-02 · STAMP-04.

### D07 녹음·찾기 목표

제안: 녹음은 연결된 폰으로 스트리밍하고 누락은 부분 파일로 표시한다. 찾기는 등록 기기 반응과 외부 모듈의 유효한 근접 관측을 구분해 표시한다.

선택 후 산출물: 목표 녹음 길이/음질·백그라운드 조건·버퍼·근접 임계값/유효시간·출력 부품을 측정 가능한 값으로 정한다.

영향 작업: HW-01 · HW-06 · REC-01 · REC-02 · REC-03 · FIND-01 · FIND-02 · VERIFY-02 · HW-07 · TRIP-04.

### D08 결제·환불·정산

제안: 주문 시 가격·토큰·수취 주소를 고정한다. 환불은 원지급에 연결한 별도 거래로 처리하고 정산은 주문/입금/환불 대조·마감으로 구성한다.

선택 후 산출물: 견적/환율 출처·확정 기준·환불 목적지 증명·가스 부담·부분 환불·마감 후 보정 정책을 정한다.

영향 작업: BASE-02 · APP-03 · APP-04 · HW-05 · HW-06 · INDEX-03 · INDEX-05 · PAY-01 · PAY-02 · PAY-03 · PAY-04 · PAY-05 · SHOP-02 · SHOP-03 · SHOP-04 · SHOP-05 · OPS-03 · VERIFY-03 · SHOP-06 · INDEX-06.

### D09 대여·혜택

제안: 기록은 계정에 보존하고 신규 여행 지갑은 자산 회수, import 지갑은 외부 접근 확인 후 기기 사본 삭제로 반납한다. 기기로 새로 만든 자격/위임과 기존 권한을 구분하고 초기화 확인 후 재대여한다. 스탬프는 원장 기준으로 앱/기기에 동기화한다.

선택 후 산출물: 적립/사용 규칙과 환불 시 이미 사용한 혜택 처리, 회수 불가/미확정 거래의 반납 보류 정책을 정한다.

영향 작업: BASE-02 · KEY-03 · FIND-02 · SHOP-04 · STAMP-01 · STAMP-02 · STAMP-03 · OPS-02 · AI-03 · VERIFY-01 · VERIFY-03 · STAMP-04 · INDEX-06.

### D10 배포/시험 환경

제안: 체인 8283과 더미 토큰 환경을 manifest로 묶고 주소·ABI·배포 블록·자산 단위·가스 수급을 등록한다. 네이티브와 wrapped를 별도 타입으로 표시한다.

선택 후 산출물: 실제 배포값, wrapped 사용 여부, Indexer endpoint 및 실제 USDC 지원 확인 방법을 정한다.

영향 작업: BASE-04 · BASE-05 · TOKEN-01 · TOKEN-02 · TOKEN-03 · INDEX-01 · INDEX-03 · INDEX-04 · INDEX-05 · VERIFY-04 · RELEASE-01.

### D11 스마트 계정 전환

제안: EOA 거래 경로를 먼저 연결한 뒤 선택한 스마트 계정 경로를 같은 앱에 추가한다. 주소/자산/권한 전환은 사용자 화면에서 명시한다.

선택 후 산출물: 계정 생성/위임 방식, EntryPoint/SDK/Bundler 버전, signer 조합과 복구를 선택한다. 운영자 후원은 기존 후속 범위 유지.

영향 작업: SMART-01 · SMART-02 · SMART-03 · VERIFY-04 · BASE-06 · STAMP-04.

### D12 DeFi·FX 상품

제안: DeFi는 swap과 유동성 추가/회수를 대표 흐름으로 구체화한다. FX는 USD/KRW 시험 자산의 현물 교환을 상품 초안으로 두고 공통 견적/승인 UI를 재사용한다.

선택 후 산출물: 상품 모델·토큰 쌍·pool·유동성·가격 출처·수수료를 선택한다. FX 모델 선택 전 파생상품으로 고정하지 않는다.

영향 작업: DEX-01 · DEX-02 · DEX-03 · FX-01 · FX-02 · FX-03 · VERIFY-04 · BASE-06.

### D13 Perpetual 상품

제안: 담보 입출금·개설/축소/종료·펀딩·청산과 keeper 운영을 한 흐름으로 구성한다. 테스트 가격 공급도 시점/출처/권한을 명시한다.

선택 후 산출물: 시장·담보·레버리지·가격·펀딩·청산/중단 파라미터를 지정하고 계산 예제를 만든다.

영향 작업: PERP-01 · PERP-02 · PERP-03 · PERP-04 · PERP-05 · VERIFY-04 · PERP-06.

### D14 STO 범위

제안: 권리가 명시된 테스트 발행물에 발행·보유·전송 제한·자격 상태를 구현한다. 앱에서 시험 데이터임을 표시한다.

선택 후 산출물: 발행물의 구체 권리·발행자·자격·허용 동작·전송 제한 조건을 정한다.

영향 작업: STO-01 · STO-02 · STO-03 · VERIFY-04.

### D15 DID 범위

제안: 발급·보관·제시·검증·철회를 연결한다. 개인 정보는 공개 체인 데이터와 분리하고, STO 자격 입력으로 재사용하는 연결을 검토한다.

선택 후 산출물: DID/자격 표준·issuer/verifier·서명 방식·철회 상태·공개 필드를 선택한다.

영향 작업: DID-01 · DID-02 · DID-03 · VERIFY-04 · BASE-06.

### D16 x402 범위

제안: 앱에서 가격을 확인하고 사용하는 유료 HTTP 자원 하나를 기준으로 지불 요구·증명·검증·응답 실패 복구를 구현한다. 예시 자원은 유료 여행 코스 응답이다.

선택 후 산출물: 자원·프로토콜 버전·네트워크/자산·검증/지급 서비스와 재요청 과금 규칙을 선택한다.

영향 작업: X402-01 · X402-02 · X402-03 · VERIFY-04 · BASE-06.

### D17 위치·후기·실제 데이터

제안: 장소 제공자 ID와 매장 ID를 연결하고 위치 방문·테스트 결제·실제 구매를 별도 출처로 보관한다. 후기 구매 표식은 결제 검증에 연결한다.

선택 후 산출물: 장소/후기 제공자·위치 수집 방식·동의/보관/삭제·실제 구매 자료 범위를 정한다.

영향 작업: APP-04 · TRIP-01 · TRIP-02 · TRIP-03 · VERIFY-05 · INDEX-06 · TRIP-04.

### D18 추천·챌린지 기준

제안: 장소 후보를 출처와 함께 제공하고 AI 코스를 이동/영업시간/예산 규칙으로 검증한다. 챌린지는 방문/결제 증거와 중복 보상 방지 원장으로 관리한다.

선택 후 산출물: 입력 제약·평가 표본·데이터 부족 대안·방문 판정·증거 철회 시 혜택 정책을 선택한다.

영향 작업: AI-01 · AI-02 · AI-03 · VERIFY-05 · TRIP-04.

### D19 운영·수용 목표

제안: 릴리스마다 J01~J19의 정상/실패/복구 결과와 버전을 기록한다. 업무 DB·Indexer·파일 복구를 검증하고 운영자는 원천 재조회로 예외를 처리한다.

선택 후 산출물: 성능/품질 수치·보관 기간·복구 목표·운영 역할·실자산 운영 여부의 판단 기준을 정한다.

영향 작업: BASE-05 · AUTH-04 · REC-03 · OPS-01 · OPS-03 · VERIFY-01 · VERIFY-02 · VERIFY-03 · VERIFY-04 · VERIFY-05 · RELEASE-01 · RELEASE-02 · RELEASE-03 · PERP-06 · TRIP-04.

## 반납 정책

가져온 지갑의 무관한 전체 자산 sweep 강제 금지; 사용자 답변 없이 신규 여행 지갑 복구/반납 정책 선택 금지

## 실제 서비스 운영 검토를 위한 자료

여행자 보유자산→선택HW/Cloud signer→가게지정주소. 초기고객가스·가게스테이블코인수령. custody/control은이도식만으로결론내지않고MPC참여자·복구권한·운영자권한실제설계로판단.

법률 적합성 결론이나 현재 제도 확인 보고가 아니다. 테스트넷이라는 이유만으로 실제 카페 운영 전체가 규제 검토에서 제외된다고 가정하지 않는다.

- 누가 자산이전·키복구를 단독/공동 통제하는가? MPC provider와운영자의keyshare·승인권한 및중단시접근은?
- 가게가 받는 자산과 주문 KRW가격/환율·회계/세무 기록·환불 자산/금액/가스·분쟁책임을 어떻게 정하는가?
- 운영자는 단순소프트웨어·기기대여인지, 교환/이전/보관·중개 기능을 실제 제공하는지, 적용요건 검토에 필요한 행위는 무엇인가?
- 테스트 STO의권리가실제권리/수익약속으로바뀌는지, DeFi/FX/Perp노출과모집·이용대상의조건은 무엇인가?
- 여행자/가게/녹음참여자의동의·위치/외부AI전송·국외이전·삭제·보관 예외·민원처리는어떻게증명하는가?
- 시험결제와실물구매를어떻게구분하며실자산활성화전기능·지역·자산별검토/출시승인자료는무엇인가?

## 검증의 범위

동명 JSON의 source hash·104 task/320 step 대조·15요구/16복합facet·19결정 보존·route/action 참조를 로컬 검증한다. 기존 문서의 API/SQL 상태는 변경하지 않았다. 실기·체인·서비스 테스트는 모두 미실행이다.

## 후속 작업

의사결정·기술프로필 선택값을 반영해 관련 wire/물리 계약을 고정하고 미병합 후보를 기준 카탈로그에 채택. 구현은 사용자 전환 지시 이후. 이미 가능한 논리 설계는 위 산출물에 작성되어 있다.

## 논리 검토 보완 반영

LG-01~05를 원본 상세 설계에 반영했다. 시장 unknown 복구 전이, credential/presentation/verification 객체 분리, 지급·재시도의 공통 guard, 세션 갱신/로그인 해제 요청·결과조회 계약을 추가했다. 통합 계약은 OC-01~20이며 선언 범위와 파생 영향 범위를 분리했다. 실기/앱/체인 실행은 미검증이다.

## 후속 경합 검토 반영 — LG06~08

유료 결과 생성/공개는 현재 지급 확정도 직접 확인한다. 지급 reorg 제한은 환불·동의·권한 제한과 별도로 유지한다. 세션 refresh 원 결과는 현재 generation과 일치할 때만 보호 토큰을 반환하며, 폰도 늦은 이전 generation을 설치하지 않는다. AI 동의 철회는 목적별 이용 차단이며, 파일 삭제는 별도 승인된 범위/정책 plan이 있어야 한다.

## 지급·환불·삭제 완료 보완 — LG09~11

PA 지급축의 거절/만료/확정실패/unknown 복구를 정의했다. 유료자원 환불은 OC21/22·RF 전이표로 별도 권한과노출원장을 사용한다. 삭제완료는 대상집합과삭제revision CAS 및원천job종료증거가 필요하다. 총 후보계약22개이며 실제 API등록·구현 완료는 아니다.

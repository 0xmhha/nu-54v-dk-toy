# 12주 WBS 한눈에 보기

상세 추적 정보는 [제품별 전체 작업 목록·우선순위·12주 WBS](product-worklist-and-12week-wbs.md), 실제 상태 갱신은 [관리용 CSV](product-worklist-and-12week-wbs.csv)를 사용한다. 이 문서는 회의와 일정 검토를 위한 읽기 전용 요약이다.

## 한 페이지 제품·통신 아키텍처

![NU-54V-DK 제품별 Local·Cloud·Blockchain 통합 아키텍처](../assets/diagrams/product-architecture-one-page.png)

[확대용 SVG](../assets/diagrams/product-architecture-one-page.svg) · [PNG 원본](../assets/diagrams/product-architecture-one-page.png) · [생성 스크립트](../assets/diagrams/generate_product_architecture.py)

- **파랑(Local)**: 실제 NU-54V-DK, 유저 앱, 매장 키오스크처럼 사용자나 매장 가까이에서 실행되는 제품이다.
- **초록(Cloud)**: 공통 API, MPC, 백오피스, Indexer, 시장·여행 서비스를 배치한다.
- **보라(Blockchain)**: StableNet 테스트넷의 계약, 이벤트 로그, 시장 계약을 구분한다.
- **주황(External)**: Google·Apple 인증과 장소·전사·추천 제공자를 표시한다.
- `C1~C9`는 BLE, HTTPS, 체인 RPC·event, 외부 API 경계를 뜻한다. 같은 번호의 화살표와 하단 범례를 함께 읽는다.

## 읽는 순서

1. **C0부터 착수**해 보드·환경·앱·체인 기준선을 만든다.
2. **M2(4주)**에서 첫 실제 NU 결제를 통과한다.
3. **M4(8주)**까지 15개 요구의 첫 실제 연결을 확보한다.
4. **9주부터** 실패·복구·대사·권한·삭제·성능 검증을 우선한다.
5. **M7(12주)**에서 릴리스와 운영 인수로 닫는다.

## 제품과 작업량

| 제품 | 작업 영역 | 상위 패키지 | 세부 작업 |
|---|---|---:|---:|
| P01 | NU-54V-DK 펌웨어·주변 부품 | 11 | 18 |
| P02 | 유저 앱·소셜 로그인·기기 설정 | 4 | 8 |
| P03 | Cloud MPC Wallet | 4 | 5 |
| P04 | RN 키오스크·매장 운영 | 5 | 11 |
| P05 | 백오피스·대여·운영 신뢰 | 4 | 5 |
| P06 | StableNet 토큰·스마트계정·자격·x402 | 5 | 15 |
| P07 | Indexer·조회 프런트엔드 | 6 | 6 |
| P08 | DEX·FX·Perpetual 서비스 | 7 | 12 |
| P09 | 여행·기록·추천·혜택 | 5 | 10 |
| P10 | 공통 기반·검증·릴리스 도구 | 6 | 14 |
| **합계** | **10개 제품** | **57** | **104** |

## 12주 제품 로드맵

| 제품 | W1–2 기반 | W3–4 첫 연결 | W5–6 핵심 확장 | W7–8 전 범위 통합 | W9–10 안정화 | W11–12 수용·인수 |
|---|---|---|---|---|---|---|
| P01 NU-54V-DK 펌웨어·주변 부품 | 기반·지갑·BLE | 서명·FOTA 기반 | FOTA·패스키·녹음 | 동시동작·반납 연계 | 수용·성능 | 릴리스 |
| P02 유저 앱·소셜 로그인·기기 설정 | 앱·계정 골격 | 소셜·두 지갑·거래 UI | 영수증·기기 통합 | 확장 DApp/여행 연결 | 수용·수정 | 앱 릴리스 |
| P03 Cloud MPC Wallet | MPC spike | DKG·주소·승인 | 서명·복구 | refresh·장애 | 수용·수정 | 운영 인수 |
| P04 RN 키오스크·매장 운영 | 매장·메뉴·주문 | 첫 NU 결제 | 환불·매출·정산 | 권한·대사 완성 | 카페 회귀 | 키오스크 릴리스 |
| P05 백오피스·대여·운영 신뢰 | 권한·감사 골격 | 운영 UI 기반 | 대여·FOTA·예외 | 반납·대사 완성 | 복구 훈련 | 운영 인수 |
| P06 StableNet 토큰·스마트계정·자격·x402 | 토큰·가스·배포 | Smart/DID 설계 | Smart·DID·STO·x402 | 앱 실행·이벤트 | 온체인 회귀 | ABI 인수 |
| P07 Indexer·조회 프런트엔드 | source·schema | canonical ingest | 확장 decoder·앱 조회 | projection·업무 보정 | rebuild·reorg 회귀 | 운영 인수 |
| P08 DEX·FX·Perpetual 서비스 | 상품 기준선 | AMM·Perp 기반 | DEX·FX·Perp 실행 | 앱·keeper 완성 | 위험/실패 회귀 | 서비스 인수 |
| P09 여행·기록·추천·혜택 | 입력·권한 기준 | 장소·후기·발자취 | 음성 AI·코스 | 챌린지·스탬프 | 삭제·환불 보정 | 여행 릴리스 |
| P10 공통 기반·검증·릴리스 도구 | 계약·환경·CI | 첫 결제 관문 관리 | 전 범위 통합 관리 | 수용 시험 시작 | RC·수용 판정 | 릴리스·인수 |

## 가장 먼저 여는 C0 작업

| 순서 | WBS | 제품 | 작업 | 주차 | 통과 증거 |
|---:|---|---|---|---|---|
| 1 | WBS-P01-01 | P01 | 실기기 bring-up과 자원 기준선 | W1 | 실물 사진, 재현 빌드/플래시 로그, pin·Flash·RAM 보고서 |
| 2 | WBS-P01-02 | P01 | 보호 키와 HW 지갑 수명주기 | W1–3 | 실기기 생성/초기화/서명, 키 비반출·잘못된 요청 거절 증거 |
| 3 | WBS-P01-03 | P01 | 인증 BLE 등록과 세션 | W1–2 | S25에서 등록·재연결·오류/만료 재현 로그 |
| 4 | WBS-P02-01 | P02 | 앱 골격·계정·권한 기반 | W1–2 | S25 설치 빌드, 로그인 전/후 탐색, 역할 거부와 재시작 증거 |
| 5 | WBS-P03-01 | P03 | MPC 도입·신뢰·복구 기준선 | W1–2 | 선택 revision, threat boundary, 참여자 격리·실행 spike |
| 6 | WBS-P04-01 | P04 | 태블릿·매장·메뉴·주문 기반 | W1–3 | 태블릿 설치, 매장 격리, 메뉴→주문 snapshot 증거 |
| 7 | WBS-P06-01 | P06 | 네트워크·dummy USDC·WKRC 기반 | W1–3 | 배포 manifest, address/codeHash/ABI/block, 단위·가스 golden vector |
| 8 | WBS-P07-01 | P07 | 체인·배포 계약 등록 | W1–2 | RPC capability, 등록/미등록 주소 gate, source revision |
| 9 | WBS-P07-02 | P07 | SDK·조회 schema 정합화 | W1–3 | contract tests, amount/unit golden vector, 오류 schema |
| 10 | WBS-P07-03 | P07 | canonical ingest·backfill·reorg | W2–4 | 중복/reorg fixture, 재시작 cursor, full backfill 결과 |
| 11 | WBS-P10-01 | P10 | 요구·데이터·연결 계약 기준선 | W1–2 | 계약 예제, 상태/권한/멱등성 contract test와 소비자 확인 |
| 12 | WBS-P10-02 | P10 | 재사용·환경·지원 행렬 | W1–2 | pinned revision, 호환 gap, 비밀 없는 환경 manifest, 지원/거절표 |
| 13 | WBS-P10-03 | P10 | CI·재현 빌드·시험 도구 | W1–3 | clean build, artifact digest, CI 결과, synthetic fixture 실행 |

## 제품별 간결 WBS

우선순위: **C0 즉시 착수**, **C1 첫 종단 경로**, **C2 필수 확장**, **C3 통합·출시**. 선행 관계와 상세 완료 증거는 상세 문서에서 확인한다.

### P01 · NU-54V-DK 펌웨어·주변 부품

| WBS | 상위 작업 | 우선 | 주차 | 관문 | taskRefs |
|---|---|---|---|---|---|
| WBS-P01-01 | 실기기 bring-up과 자원 기준선 | C0 | W1 | M0 | HW-01 |
| WBS-P01-02 | 보호 키와 HW 지갑 수명주기 | C0 | W1–3 | M2 | HW-02 |
| WBS-P01-03 | 인증 BLE 등록과 세션 | C0 | W1–2 | M1 | HW-03 |
| WBS-P01-04 | 지갑 import와 기기 설정 | C1 | W2–3 | M2 | HW-04 |
| WBS-P01-05 | 결제 표시·물리 승인·EOA 서명 | C1 | W2–4 | M2 | HW-05 |
| WBS-P01-06 | 근접 모듈과 결제 세션 | C2 | W3–5 | M3 | HW-06 |
| WBS-P01-07 | FOTA 부트·패키지 기반 | C1 | W2–4 | M2 | OTA-01, OTA-02 |
| WBS-P01-08 | 앱 FOTA와 호환 복구 | C1 | W4–6 | M3 | OTA-03, OTA-04 |
| WBS-P01-09 | 패스키 등록·인증·해제 | C2 | W3–6 | M3 | KEY-01, KEY-02, KEY-03 |
| WBS-P01-10 | 녹음 전송과 기기 찾기 | C2 | W3–6 | M3 | REC-01, REC-02, FIND-01, FIND-02 |
| WBS-P01-11 | 동시 동작과 자원 중재 | C2 | W5–7 | M4 | HW-07 |

### P02 · 유저 앱·소셜 로그인·기기 설정

| WBS | 상위 작업 | 우선 | 주차 | 관문 | taskRefs |
|---|---|---|---|---|---|
| WBS-P02-01 | 앱 골격·계정·권한 기반 | C0 | W1–2 | M1 | APP-01, AUTH-01 |
| WBS-P02-02 | Google·Apple 계정 수명주기 | C1 | W1–4 | M2 | AUTH-02, AUTH-03, AUTH-04 |
| WBS-P02-03 | 두 지갑·자산 선택 경험 | C1 | W2–4 | M2 | APP-02 |
| WBS-P02-04 | 송금·견적·승인·영수증 공통 UI | C1 | W2–5 | M3 | APP-03, APP-04 |

### P03 · Cloud MPC Wallet

| WBS | 상위 작업 | 우선 | 주차 | 관문 | taskRefs |
|---|---|---|---|---|---|
| WBS-P03-01 | MPC 도입·신뢰·복구 기준선 | C0 | W1–2 | M1 | MPC-01 |
| WBS-P03-02 | 분산 키 생성과 계정 연결 | C1 | W2–4 | M2 | MPC-02 |
| WBS-P03-03 | MPC 승인·서명·결과 | C1 | W3–5 | M3 | MPC-03 |
| WBS-P03-04 | 복구·refresh·참여자 장애 | C2 | W5–8 | M4 | MPC-04, MPC-05 |

### P04 · RN 키오스크·매장 운영

| WBS | 상위 작업 | 우선 | 주차 | 관문 | taskRefs |
|---|---|---|---|---|---|
| WBS-P04-01 | 태블릿·매장·메뉴·주문 기반 | C0 | W1–3 | M2 | SHOP-01, SHOP-02 |
| WBS-P04-02 | EOA 견적·제출·주문 연결 | C1 | W2–4 | M2 | PAY-01, PAY-02, PAY-03 |
| WBS-P04-03 | 입금 대사와 NU 수직 결제 | C1 | W3–5 | M2 | PAY-04, PAY-05, SHOP-03 |
| WBS-P04-04 | 점주 환불 수명주기 | C2 | W4–6 | M3 | SHOP-04 |
| WBS-P04-05 | 매출·정산·수취 권한 | C2 | W5–8 | M4 | SHOP-05, SHOP-06 |

### P05 · 백오피스·대여·운영 신뢰

| WBS | 상위 작업 | 우선 | 주차 | 관문 | taskRefs |
|---|---|---|---|---|---|
| WBS-P05-01 | 운영자 권한·감사·관리 골격 | C1 | W2–4 | M2 | OPS-01 |
| WBS-P05-02 | 가맹점·대여·FOTA 운영 | C2 | W4–7 | M4 | OPS-02, STAMP-03 |
| WBS-P05-03 | 반납 후 권한·늦은 효과 정리 | C2 | W6–8 | M4 | STAMP-04 |
| WBS-P05-04 | 결제 예외·Indexer 지연·대사 | C2 | W5–8 | M4 | OPS-03 |

### P06 · StableNet 토큰·스마트계정·자격·x402

| WBS | 상위 작업 | 우선 | 주차 | 관문 | taskRefs |
|---|---|---|---|---|---|
| WBS-P06-01 | 네트워크·dummy USDC·WKRC 기반 | C0 | W1–3 | M2 | TOKEN-01, TOKEN-02, TOKEN-03 |
| WBS-P06-02 | EOA→스마트 계정 전환 | C2 | W4–7 | M4 | SMART-01, SMART-02, SMART-03 |
| WBS-P06-03 | DID 자격 발급·검증·철회 | C2 | W4–7 | M4 | DID-01, DID-02, DID-03 |
| WBS-P06-04 | 제한형 CafePass 테스트 발행물 | C2 | W5–8 | M4 | STO-01, STO-02, STO-03 |
| WBS-P06-05 | x402 유료 AI 코스 | C2 | W5–8 | M4 | X402-01, X402-02, X402-03 |

### P07 · Indexer·조회 프런트엔드

| WBS | 상위 작업 | 우선 | 주차 | 관문 | taskRefs |
|---|---|---|---|---|---|
| WBS-P07-01 | 체인·배포 계약 등록 | C0 | W1–2 | M1 | INDEX-01 |
| WBS-P07-02 | SDK·조회 schema 정합화 | C0 | W1–3 | M2 | INDEX-02 |
| WBS-P07-03 | canonical ingest·backfill·reorg | C0 | W2–4 | M2 | INDEX-03 |
| WBS-P07-04 | 확장 계약 decoder·projection | C2 | W4–8 | M4 | INDEX-04 |
| WBS-P07-05 | 앱 조회·탐색기·상태 관측 | C1 | W3–8 | M4 | INDEX-05 |
| WBS-P07-06 | 원장·혜택·여행 보정 전파 | C2 | W6–9 | M5 | INDEX-06 |

### P08 · DEX·FX·Perpetual 서비스

| WBS | 상위 작업 | 우선 | 주차 | 관문 | taskRefs |
|---|---|---|---|---|---|
| WBS-P08-01 | AMM 상품·pool·swap/LP 실행 | C2 | W4–6 | M3 | DEX-01, DEX-02 |
| WBS-P08-02 | DEX 앱 경험 | C2 | W5–8 | M4 | DEX-03 |
| WBS-P08-03 | TEST FX 모델·교환 서비스 | C2 | W5–7 | M4 | FX-01, FX-02 |
| WBS-P08-04 | FX 앱 경험 | C2 | W6–8 | M4 | FX-03 |
| WBS-P08-05 | Perpetual 시장·오라클 기준 | C2 | W4–6 | M3 | PERP-01, PERP-02 |
| WBS-P08-06 | 포지션·펀딩·청산 실행 | C2 | W5–8 | M4 | PERP-03, PERP-04, PERP-06 |
| WBS-P08-07 | Perpetual 앱 경험 | C2 | W6–8 | M4 | PERP-05 |

### P09 · 여행·기록·추천·혜택

| WBS | 상위 작업 | 우선 | 주차 | 관문 | taskRefs |
|---|---|---|---|---|---|
| WBS-P09-01 | 전사·AI 회의 기록 | C2 | W4–7 | M4 | REC-03 |
| WBS-P09-02 | 장소 검색·verified 후기 | C2 | W3–6 | M3 | TRIP-01, TRIP-02 |
| WBS-P09-03 | 여행 모드·발자취·데이터 수명주기 | C2 | W4–8 | M4 | TRIP-03, TRIP-04 |
| WBS-P09-04 | 근거 있는 AI 코스 | C2 | W5–8 | M4 | AI-01, AI-02 |
| WBS-P09-05 | 챌린지·스탬프·보상 | C2 | W6–9 | M5 | AI-03, STAMP-01, STAMP-02 |

### P10 · 공통 기반·검증·릴리스 도구

| WBS | 상위 작업 | 우선 | 주차 | 관문 | taskRefs |
|---|---|---|---|---|---|
| WBS-P10-01 | 요구·데이터·연결 계약 기준선 | C0 | W1–2 | M1 | BASE-01, BASE-02, BASE-03 |
| WBS-P10-02 | 재사용·환경·지원 행렬 | C0 | W1–2 | M1 | BASE-04, BASE-05, BASE-06 |
| WBS-P10-03 | CI·재현 빌드·시험 도구 | C0 | W1–3 | M2 | RELEASE-01 |
| WBS-P10-04 | 두 지갑·기기 수용 시험 | C3 | W8–11 | M6 | VERIFY-01, VERIFY-02 |
| WBS-P10-05 | 매장·온체인·여행 수용 시험 | C3 | W8–11 | M6 | VERIFY-03, VERIFY-04, VERIFY-05 |
| WBS-P10-06 | 관측·백업·릴리스·인수 | C3 | W10–12 | M7 | RELEASE-02, RELEASE-03 |

## 통합 관문

| 관문 | 주차 | 완료 기준 |
|---|---:|---|
| M0 | 1 | 보드 flash·앱 실행·RPC·재현 build |
| M1 | 2 | 인증 BLE·token manifest·Indexer 원 이벤트·주문 골격 |
| M2 | 4 | NU 승인→EOA→Indexer→paid→키오스크 영수증 |
| M3 | 6 | 기기 부가기능·환불/정산·MPC·여행 core |
| M4 | 8 | 15개 요구 전체의 첫 실제 연결 |
| M5 | 10 | 카페 회귀·장애/권한/삭제/성능·복구 통과 |
| M6 | 11 | VERIFY-01~05 수용 판정 |
| M7 | 12 | 재현 릴리스·운영 문서·evidence index·시연 |

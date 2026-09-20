# 104개 작업의 구현 인계 카드

원 WBS의 범위·수용 기준·선행 관계를 보존한다. owner/effort는 미배정, 모든 실행은 not_run이다. 아래 독립 준비는 구현 허가가 아니라 향후 착수 조건이다.

## BASE-01 15개 요구사항을 기능 시나리오와 화면으로 연결

- 설계: [DS-08](../specifications/operations-release-acceptance-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 4 · 8 · 9 · 10 · 15 / 세부 작업: BASE-01.01 · BASE-01.02 · BASE-01.03
- 선행 작업: — / 결정: D01 · D02
- 산출물: 요구사항-행위자-화면-완료 증거 지도
- 수용 기준: 15개 모두 사용자 시작/결과 화면 지정 · 중복 4·8의 책임 구분
- 독립 준비: 복구/릴리스/운영 권한·evidence manifest 예제
- 통합 입력: 선정환경·정량목표·접근권한·실복구·모든기능실앱증거
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / —

## BASE-02 공통 데이터·식별자·상태 정의

- 설계: [DS-03](../specifications/kiosk-commerce-journey-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 4 · 8 · 9 · 10 · 15 / 세부 작업: BASE-02.01 · BASE-02.02 · BASE-02.03
- 선행 작업: BASE-01 / 결정: D02 · D08 · D09
- 산출물: 계정/지갑/매장/기기/대여/주문/거래 모델
- 수용 기준: 주소와 로그인 계정 구별 · 주문·지급·환불 상태 별도
- 독립 준비: order/payment/refund/fulfillment/settlement 계약 예제
- 통합 입력: 태블릿/배포·점주권한·payment policy·가게/메뉴/recipient·현재체인/indexer
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / —

## BASE-03 앱·서비스·기기 연결 규칙 작성

- 설계: [DS-01](../specifications/device-coexistence-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 4 · 8 · 9 · 10 · 15 / 세부 작업: BASE-03.01 · BASE-03.02 · BASE-03.03
- 선행 작업: BASE-02 / 결정: D02 · D03
- 산출물: API/BLE 메시지·오류·버전 예제
- 수용 기준: 정상·거절·만료·재연결 예제 · 키가 일반 API 로그에 포함되지 않음
- 독립 준비: service arbitration·key/approval·FOTA 상태 예제
- 통합 입력: 실제 NU board revision/pins/power,Zephyr/NCS/bootloader,key isolation,passkey/audio/ranging profile
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-108 · API-109 / —

## BASE-04 기존 코드 호환성 차이를 수정 작업으로 확정

- 설계: [DS-04](../specifications/stablenet-compatibility-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 4 · 8 · 9 · 10 · 15 / 세부 작업: BASE-04.01 · BASE-04.02 · BASE-04.03
- 선행 작업: BASE-03 / 결정: D10
- 산출물: 저장소/커밋·ABI·API 변경 목록
- 수용 기준: SDK–Indexer 및 AA 형식 차이를 재현 입력에 연결 · mock/실제 경로 구분
- 독립 준비: manifest/asset/decoder/AA typed attribution 예제
- 통합 입력: 실chainidentity·addresses/ABI/codehash·tokenunits·AA버전·Indexerendpoint
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / —

## BASE-05 실행 환경·데이터 보존·비밀 설정 구성

- 설계: [DS-08](../specifications/operations-release-acceptance-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 4 · 8 · 9 · 10 · 15 / 세부 작업: BASE-05.01 · BASE-05.02 · BASE-05.03
- 선행 작업: BASE-02 / 결정: D10 · D19
- 산출물: 환경/DB/파일 저장·접근 설정
- 수용 기준: 개발/시연 값 분리 · 재시작 후 업무 데이터 유지 · 실제 비밀 없는 설정 예제
- 독립 준비: 복구/릴리스/운영 권한·evidence manifest 예제
- 통합 입력: 선정환경·정량목표·접근권한·실복구·모든기능실앱증거
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-103 · API-107 · API-108 · API-109 · API-110 / —

## AUTH-01 계정·매장 소속·역할 서버 구현

- 설계: [DS-02](../specifications/social-wallet-recovery-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 5 · 6 · 9 · 10 / 세부 작업: AUTH-01.01 · AUTH-01.02 · AUTH-01.03
- 선행 작업: BASE-02 · BASE-05 / 결정: D02
- 산출물: 계정/소속/권한 API와 저장 모델
- 수용 기준: 다른 매장 접근 거절 · 개인 지갑과 매장 관리 권한 분리
- 독립 준비: auth/wallet/MPC phase·epoch·권한 fixture
- 통합 입력: Google/Apple client환경,MPC protocol/provider/threshold/recovery policy,실제participant
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-005 · API-104 · API-105 · API-106 / K01 · K04

## AUTH-02 Google 로그인 앱·서버 연결

- 설계: [DS-02](../specifications/social-wallet-recovery-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 5 · 6 · 9 · 10 / 세부 작업: AUTH-02.01 · AUTH-02.02 · AUTH-02.03
- 선행 작업: AUTH-01 · APP-01 / 결정: D01
- 산출물: 실제 Google 인증 흐름
- 수용 기준: 가입·재로그인·취소·만료 처리 · 서버에서 제공자 증명 검증
- 독립 준비: auth/wallet/MPC phase·epoch·권한 fixture
- 통합 입력: Google/Apple client환경,MPC protocol/provider/threshold/recovery policy,실제participant
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-001 · API-103 / U01

## AUTH-03 Apple 로그인 앱·서버 연결

- 설계: [DS-02](../specifications/social-wallet-recovery-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 5 · 6 · 9 · 10 / 세부 작업: AUTH-03.01 · AUTH-03.02 · AUTH-03.03
- 선행 작업: AUTH-01 · APP-01 / 결정: D01
- 산출물: 실제 Apple 인증 흐름
- 수용 기준: 가입·재로그인·취소·만료 처리 · 제공자 식별자 기반 계정 연결
- 독립 준비: auth/wallet/MPC phase·epoch·권한 fixture
- 통합 입력: Google/Apple client환경,MPC protocol/provider/threshold/recovery policy,실제participant
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-001 · API-103 / U01

## AUTH-04 계정 연결·로그아웃·탈퇴 정책 구현

- 설계: [DS-02](../specifications/social-wallet-recovery-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 5 · 6 · 9 · 10 / 세부 작업: AUTH-04.01 · AUTH-04.02 · AUTH-04.03
- 선행 작업: AUTH-02 · AUTH-03 / 결정: D03 · D04 · D19
- 산출물: 계정 수명주기 화면/API
- 수용 기준: Google/Apple 중복 연결 정책 적용 · 탈퇴와 지갑 자산/복구 관계 표시
- 독립 준비: auth/wallet/MPC phase·epoch·권한 fixture
- 통합 입력: Google/Apple client환경,MPC protocol/provider/threshold/recovery policy,실제participant
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-002 · API-003 · API-004 · API-006 · API-103 · API-110 / U01 · U25

## APP-01 유저 앱 실행·탐색·권한 골격 구성

- 설계: [DS-02](../specifications/social-wallet-recovery-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 4 · 7 · 8 / 세부 작업: APP-01.01 · APP-01.02 · APP-01.03
- 선행 작업: BASE-01 · BASE-05 / 결정: D01
- 산출물: 설치 가능한 앱과 공통 화면/상태
- 수용 기준: 실제 대상 폰 설치 · 오프라인·로딩·오류·재시작 구분
- 독립 준비: auth/wallet/MPC phase·epoch·권한 fixture
- 통합 입력: Google/Apple client환경,MPC protocol/provider/threshold/recovery policy,실제participant
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / —

## APP-02 기기 지갑·Cloud Wallet 선택과 자산 화면

- 설계: [DS-02](../specifications/social-wallet-recovery-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 4 · 7 · 8 / 세부 작업: APP-02.01 · APP-02.02 · APP-02.03
- 선행 작업: APP-01 · BASE-03 · INDEX-02 / 결정: D03
- 산출물: 현재 지갑/주소/잔액/네트워크 UI
- 수용 기준: 두 주소·키 책임 혼동 없음 · 네이티브 가스와 결제 토큰 잔액 구분 · 점주 로그인 시 소속 매장 지갑 자산 조회; 개인 자산/서명 권한과 구분
- 독립 준비: auth/wallet/MPC phase·epoch·권한 fixture
- 통합 입력: Google/Apple client환경,MPC protocol/provider/threshold/recovery policy,실제participant
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-007 · API-008 · API-009 · API-090 · API-098 / U02

## APP-03 송금·견적·승인·거래 결과 공통 UI

- 설계: [DS-02](../specifications/social-wallet-recovery-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 4 · 7 · 8 / 세부 작업: APP-03.01 · APP-03.02 · APP-03.03
- 선행 작업: APP-02 · PAY-01 / 결정: D08
- 산출물: EOA/MPC/DApp에서 재사용할 거래 컴포넌트
- 수용 기준: 정수 금액과 표시 단위 일치 · 승인 전 수취인/체인/가스 상한 확인
- 독립 준비: auth/wallet/MPC phase·epoch·권한 fixture
- 통합 입력: Google/Apple client환경,MPC protocol/provider/threshold/recovery policy,실제participant
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-017 · API-107 · API-108 · API-109 · API-110 / U03

## APP-04 개인 결제·영수증·환불 기록 연결

- 설계: [DS-03](../specifications/kiosk-commerce-journey-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 4 · 7 · 8 / 세부 작업: APP-04.01 · APP-04.02 · APP-04.03
- 선행 작업: APP-01 · PAY-04 · SHOP-04 / 결정: D08 · D17
- 산출물: 필터/상세/진행 상태 화면
- 수용 기준: 실제 주문과 지급·환불 연결 · 상태 불명확을 성공으로 표시하지 않음
- 독립 준비: order/payment/refund/fulfillment/settlement 계약 예제
- 통합 입력: 태블릿/배포·점주권한·payment policy·가게/메뉴/recipient·현재체인/indexer
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-042 · API-104 · API-105 · API-106 / U11

## HW-01 보드·부품·SDK 빌드와 자원 측정

- 설계: [DS-01](../specifications/device-coexistence-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 1 · 4 · 8 / 세부 작업: HW-01.01 · HW-01.02 · HW-01.03
- 선행 작업: BASE-01 / 결정: D05 · D07
- 산출물: 재현 가능한 보드 빌드/부품·메모리 기록; Zephyr 기반 NU 보드 빌드 기준과 선택 SDK/Zephyr revision manifest
- 수용 기준: 실제 NU 부팅·버튼/화면 확인 · FOTA/녹음 공존 자원 기록 · 펌웨어 RTOS는 Zephyr; SDK 배포판/보드 target/정확한 버전과 실제 부팅 증거를 구분하여 기록
- 독립 준비: service arbitration·key/approval·FOTA 상태 예제
- 통합 입력: 실제 NU board revision/pins/power,Zephyr/NCS/bootloader,key isolation,passkey/audio/ranging profile
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: device.info · API-013 / —

## HW-02 키 저장·생성·주소·초기화 구현

- 설계: [DS-01](../specifications/device-coexistence-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 1 · 4 · 8 / 세부 작업: HW-02.01 · HW-02.02 · HW-02.03
- 선행 작업: HW-01 / 결정: D03
- 산출물: 기기 지갑 수명주기 펌웨어
- 수용 기준: 기기 생성 주소 검증 · 재부팅 유지 · 초기화 뒤 이전 키 사용 불가
- 독립 준비: service arbitration·key/approval·FOTA 상태 예제
- 통합 입력: 실제 NU board revision/pins/power,Zephyr/NCS/bootloader,key isolation,passkey/audio/ranging profile
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: wallet.create · wallet.address / U04 · U24

## HW-03 인증된 BLE 세션·기기 등록 구현

- 설계: [DS-01](../specifications/device-coexistence-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 1 · 4 · 8 / 세부 작업: HW-03.01 · HW-03.02 · HW-03.03 · HW-03.04 · HW-03.05
- 선행 작업: HW-01 · BASE-03 · APP-01 / 결정: D02 · D03
- 산출물: 기기/앱 연결 모듈
- 수용 기준: 올바른 기기 확인 · 미인증 요청·재전송 거절 · 끊김/재연결 상태 표시 · 소유 앱과 임시 매장 세션 권한 구분; 키오스크 설정/import 접근 차단
- 독립 준비: service arbitration·key/approval·FOTA 상태 예제
- 통합 입력: 실제 NU board revision/pins/power,Zephyr/NCS/bootloader,key isolation,passkey/audio/ranging profile
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: session.open · session.confirm · session.close · payment.identify · device.binding.revoked · API-010 · API-011 · API-012 · API-033 · API-090 / U04 · K03

## HW-04 앱에서 키 가져오기·설정 연결

- 설계: [DS-01](../specifications/device-coexistence-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 1 · 4 · 8 / 세부 작업: HW-04.01 · HW-04.02 · HW-04.03
- 선행 작업: HW-02 · HW-03 / 결정: D03
- 산출물: 암호화 import·설정 화면/펌웨어
- 수용 기준: 기존 키로 예상 주소 도출 · 실패 시 상태 보존 · 키가 일반 저장/로그에 남지 않음
- 독립 준비: service arbitration·key/approval·FOTA 상태 예제
- 통합 입력: 실제 NU board revision/pins/power,Zephyr/NCS/bootloader,key isolation,passkey/audio/ranging profile
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: wallet.import.begin · wallet.import.chunk · wallet.import.commit · wallet.import.abort · device.settings.update · API-098 / U04 · U05

## HW-05 기기 결제 표시·승인·EOA 서명 구현

- 설계: [DS-01](../specifications/device-coexistence-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 1 · 4 · 8 / 세부 작업: HW-05.01 · HW-05.02 · HW-05.03
- 선행 작업: HW-02 · HW-03 · PAY-01 / 결정: D03 · D08
- 산출물: NU signer 어댑터와 기기 승인 UI
- 수용 기준: 표시 내용과 서명 거래 일치 · 거절·잘못된 chain ID·요청 변경 처리
- 독립 준비: service arbitration·key/approval·FOTA 상태 예제
- 통합 입력: 실제 NU board revision/pins/power,Zephyr/NCS/bootloader,key isolation,passkey/audio/ranging profile
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: payment.identify · payment.prepare · payment.result · wallet.sign.prepare · wallet.sign.result · request.cancel · API-034 / K03 · D01

## HW-06 근접 측정 모듈·세션 승인 연결

- 설계: [DS-01](../specifications/device-coexistence-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 1 · 4 · 8 / 세부 작업: HW-06.01 · HW-06.02 · HW-06.03
- 선행 작업: HW-03 · HW-05 / 결정: D07 · D08
- 산출물: 보유 모듈 연결과 근접 판정
- 수용 기준: 실제 보드/모듈 측정 · 근접 결과를 요청/세션에 연결 · 서명 이후 단절은 거래 취소로 표시하지 않음
- 독립 준비: service arbitration·key/approval·FOTA 상태 예제
- 통합 입력: 실제 NU board revision/pins/power,Zephyr/NCS/bootloader,key isolation,passkey/audio/ranging profile
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: proximity.observe / K03

## OTA-01 업데이트 이미지·부트·복구 경로 검증

- 설계: [DS-01](../specifications/device-coexistence-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 2 · 8 · 10 / 세부 작업: OTA-01.01 · OTA-01.02 · OTA-01.03
- 선행 작업: HW-01 / 결정: D05
- 산출물: 실제 보드 부트/FOTA 기술 검증 기록
- 수용 기준: 목표 이미지 수용 · 중단 뒤 재부팅 경로 확인 · 키 보존 정책 제시
- 독립 준비: service arbitration·key/approval·FOTA 상태 예제
- 통합 입력: 실제 NU board revision/pins/power,Zephyr/NCS/bootloader,key isolation,passkey/audio/ranging profile
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / U06

## OTA-02 서명된 업데이트 패키지·배포 메타데이터

- 설계: [DS-01](../specifications/device-coexistence-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 2 · 8 · 10 / 세부 작업: OTA-02.01 · OTA-02.02 · OTA-02.03
- 선행 작업: OTA-01 · RELEASE-01 / 결정: D05
- 산출물: 펌웨어 패키징/버전 도구
- 수용 기준: 손상·허용하지 않은 이미지 거절 · 호환 모델/버전 검사
- 독립 준비: service arbitration·key/approval·FOTA 상태 예제
- 통합 입력: 실제 NU board revision/pins/power,Zephyr/NCS/bootloader,key isolation,passkey/audio/ranging profile
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-048 · API-049 / U06 · O02

## OTA-03 앱 FOTA 진행·재연결·실기 복구 구현

- 설계: [DS-01](../specifications/device-coexistence-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 2 · 8 · 10 / 세부 작업: OTA-03.01 · OTA-03.02 · OTA-03.03
- 선행 작업: OTA-02 · HW-03 · HW-04 / 결정: D05
- 산출물: 앱 업데이트 화면과 기기 전송
- 수용 기준: 성공 후 버전 확인 · 전원/통신 중단 시험 · 지갑/설정 복구 정책 검증
- 독립 준비: service arbitration·key/approval·FOTA 상태 예제
- 통합 입력: 실제 NU board revision/pins/power,Zephyr/NCS/bootloader,key isolation,passkey/audio/ranging profile
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: fota.begin · fota.chunk · fota.finalize · fota.apply · fota.status / U06 · D02

## KEY-01 패스키 대상과 실제 기기 연결 경로 검증

- 설계: [DS-01](../specifications/device-coexistence-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 3 · 8 / 세부 작업: KEY-01.01 · KEY-01.02 · KEY-01.03
- 선행 작업: HW-01 · BASE-03 / 결정: D06
- 산출물: 호환성 표·실기 등록/인증 실험
- 수용 기준: 대상 OS/서비스/전송 명시 · 자체 서명 승인과 표준 패스키 구분
- 독립 준비: service arbitration·key/approval·FOTA 상태 예제
- 통합 입력: 실제 NU board revision/pins/power,Zephyr/NCS/bootloader,key isolation,passkey/audio/ranging profile
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / U07

## KEY-02 기기 자격 생성·인증·사용자 검증 구현

- 설계: [DS-01](../specifications/device-coexistence-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 3 · 8 / 세부 작업: KEY-02.01 · KEY-02.02 · KEY-02.03
- 선행 작업: KEY-01 · HW-02 · HW-03 / 결정: D06
- 산출물: 패스키 펌웨어와 관리 화면
- 수용 기준: 대상 서비스 등록/인증 성공 · 다른 서비스/잘못된 요청 거절
- 독립 준비: service arbitration·key/approval·FOTA 상태 예제
- 통합 입력: 실제 NU board revision/pins/power,Zephyr/NCS/bootloader,key isolation,passkey/audio/ranging profile
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: credentials.list / U07

## KEY-03 패스키 삭제·기기 분실/반납 처리

- 설계: [DS-01](../specifications/device-coexistence-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 3 · 8 / 세부 작업: KEY-03.01 · KEY-03.02 · KEY-03.03
- 선행 작업: KEY-02 · STAMP-03 / 결정: D06 · D09
- 산출물: 등록 해제·복구 안내/실행 경로
- 수용 기준: 반납 후 이전 자격 사용 불가 · 합의한 복구 수단으로 접근 회복
- 독립 준비: service arbitration·key/approval·FOTA 상태 예제
- 통합 입력: 실제 NU board revision/pins/power,Zephyr/NCS/bootloader,key isolation,passkey/audio/ranging profile
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: credentials.delete / U07

## REC-01 마이크 캡처·버튼·기기 녹음 상태

- 설계: [DS-01](../specifications/device-coexistence-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 3 · 4 / 세부 작업: REC-01.01 · REC-01.02 · REC-01.03
- 선행 작업: HW-01 / 결정: D07
- 산출물: 실제 녹음 펌웨어
- 수용 기준: 시작/종료가 눈에 보임 · 합의한 품질·버퍼/길이 측정
- 독립 준비: service arbitration·key/approval·FOTA 상태 예제
- 통합 입력: 실제 NU board revision/pins/power,Zephyr/NCS/bootloader,key isolation,passkey/audio/ranging profile
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: recording.start · recording.stop / U08 · D02

## REC-02 BLE 음성 전송·모바일 저장/재생

- 설계: [DS-01](../specifications/device-coexistence-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 3 · 4 / 세부 작업: REC-02.01 · REC-02.02 · REC-02.03
- 선행 작업: REC-01 · HW-03 · APP-01 / 결정: D07
- 산출물: 음성 스트림·파일·목록/재생 UI
- 수용 기준: 실제 음성 재생 · 단절·누락·저장공간 부족 표시 · 파일과 세션 연결
- 독립 준비: service arbitration·key/approval·FOTA 상태 예제
- 통합 입력: 실제 NU board revision/pins/power,Zephyr/NCS/bootloader,key isolation,passkey/audio/ranging profile
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: audio.frame · audio.flow-control / U08

## REC-03 전사·AI 정리·내보내기·삭제 연결

- 설계: [DS-07](../specifications/recording-travel-ai-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 3 · 4 / 세부 작업: REC-03.01 · REC-03.02 · REC-03.03
- 선행 작업: REC-02 · AUTH-01 / 결정: D07 · D19
- 산출물: 녹음 상세·전사·요약 작업 서비스
- 수용 기준: 원음/전사/요약 연결 · 실패 재시도·개인 접근·삭제 범위 검증
- 독립 준비: 녹음gap/consent/tombstone/provenance/course 예제
- 통합 입력: 실audio/backgroundprofile·place/STT/AIprovider·privacy/retention·challengepolicy
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: recording.processing.changed · API-020 · API-051 · API-052 · API-053 · API-054 · API-094 · API-107 / U09

## FIND-01 앱 찾기와 기기 반응 구현

- 설계: [DS-01](../specifications/device-coexistence-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 3 · 8 / 세부 작업: FIND-01.01 · FIND-01.02 · FIND-01.03
- 선행 작업: HW-03 · APP-01 / 결정: D07
- 산출물: 찾기 화면·LED/부저 등 선택 출력
- 수용 기준: 등록된 실제 기기만 반응 · 연결 불가를 위치 탐지로 표시하지 않음
- 독립 준비: service arbitration·key/approval·FOTA 상태 예제
- 통합 입력: 실제 NU board revision/pins/power,Zephyr/NCS/bootloader,key isolation,passkey/audio/ranging profile
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: find.start · find.stop / U05 · D02

## FIND-02 근접 안내·끊김 상태·기기 설정 통합

- 설계: [DS-01](../specifications/device-coexistence-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 3 · 8 / 세부 작업: FIND-02.01 · FIND-02.02 · FIND-02.03
- 선행 작업: FIND-01 · HW-06 · STAMP-03 / 결정: D07 · D09
- 산출물: 찾기/기기 상태 통합 화면
- 수용 기준: 실측 결과와 UI 일치 · 권한/모듈 미지원 표시 · 반납 기기 접근 차단
- 독립 준비: service arbitration·key/approval·FOTA 상태 예제
- 통합 입력: 실제 NU board revision/pins/power,Zephyr/NCS/bootloader,key isolation,passkey/audio/ranging profile
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-013 / U05

## MPC-01 Cloud Wallet 신뢰/복구·도입 방식 검증

- 설계: [DS-02](../specifications/social-wallet-recovery-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 5 · 6 / 세부 작업: MPC-01.01 · MPC-01.02 · MPC-01.03
- 선행 작업: BASE-02 / 결정: D03 · D04
- 산출물: MPC 선택 근거·참여자/조각/복구 구조
- 수용 기준: 실제 MPC 제공 경로 확인 · 단순 전체키 재조립과 구분 · 로그인과 서명 권한 분리
- 독립 준비: auth/wallet/MPC phase·epoch·권한 fixture
- 통합 입력: Google/Apple client환경,MPC protocol/provider/threshold/recovery policy,실제participant
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / —

## MPC-02 사용자 연결·분산 키 생성/주소 구성

- 설계: [DS-02](../specifications/social-wallet-recovery-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 5 · 6 / 세부 작업: MPC-02.01 · MPC-02.02 · MPC-02.03
- 선행 작업: MPC-01 · AUTH-01 · BASE-05 / 결정: D04
- 산출물: Cloud Wallet 생성/연결 서비스
- 수용 기준: 소셜 사용자와 올바른 지갑 연결 · 반복 요청 중복 생성 방지 · 조각 보관 경계 확인
- 독립 준비: auth/wallet/MPC phase·epoch·권한 fixture
- 통합 입력: Google/Apple client환경,MPC protocol/provider/threshold/recovery policy,실제participant
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-014 / U02

## MPC-03 MPC 송금 승인·서명·결과 UI 연결

- 설계: [DS-02](../specifications/social-wallet-recovery-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 5 · 6 / 세부 작업: MPC-03.01 · MPC-03.02 · MPC-03.03
- 선행 작업: MPC-02 · APP-03 · PAY-02 / 결정: D04
- 산출물: Cloud Wallet 실제 송금 흐름
- 수용 기준: 테스트넷 거래 확인 · 불충분 참여/거절 때 서명 불가 · 하드웨어 지갑과 표시 분리
- 독립 준비: auth/wallet/MPC phase·epoch·권한 fixture
- 통합 입력: Google/Apple client환경,MPC protocol/provider/threshold/recovery policy,실제participant
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-015 / U03

## MPC-04 기기 변경·복구·회전/철회 구현

- 설계: [DS-02](../specifications/social-wallet-recovery-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 5 · 6 / 세부 작업: MPC-04.01 · MPC-04.02 · MPC-04.03
- 선행 작업: MPC-03 · AUTH-04 / 결정: D04
- 산출물: 복구·키 수명주기 서비스/화면
- 수용 기준: 합의한 복구 시나리오 통과 · 소셜 계정만 탈취한 경우 정책 검증 · 이전 참여자 철회
- 독립 준비: auth/wallet/MPC phase·epoch·권한 fixture
- 통합 입력: Google/Apple client환경,MPC protocol/provider/threshold/recovery policy,실제participant
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-016 / U12

## TOKEN-01 환경·자산 주소/단위·가스 준비 정리

- 설계: [DS-04](../specifications/stablenet-compatibility-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 / 세부 작업: TOKEN-01.01 · TOKEN-01.02 · TOKEN-01.03
- 선행 작업: BASE-04 · BASE-05 / 결정: D10
- 산출물: StableNet 자산 등록·시험 공급 경로
- 수용 기준: 8283 네트워크/배포 코드 일치 확인 · 네이티브와 wrapped·더미 구분
- 독립 준비: manifest/asset/decoder/AA typed attribution 예제
- 통합 입력: 실chainidentity·addresses/ABI/codehash·tokenunits·AA버전·Indexerendpoint
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / —

## TOKEN-02 더미 토큰·발행 권한·배포 기록 구현

- 설계: [DS-04](../specifications/stablenet-compatibility-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 / 세부 작업: TOKEN-02.01 · TOKEN-02.02 · TOKEN-02.03
- 선행 작업: TOKEN-01 · RELEASE-01 / 결정: D10
- 산출물: 테스트 ERC20·배포/발행 도구
- 수용 기준: 표준 전송/잔액 확인 · 무권한 mint·타인 burn 차단 · 실제 USDC로 오표시하지 않음
- 독립 준비: manifest/asset/decoder/AA typed attribution 예제
- 통합 입력: 실chainidentity·addresses/ABI/codehash·tokenunits·AA버전·Indexerendpoint
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / —

## TOKEN-03 WKRC 네이티브·필요 wrapped 자산 연결

- 설계: [DS-04](../specifications/stablenet-compatibility-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 / 세부 작업: TOKEN-03.01 · TOKEN-03.02 · TOKEN-03.03
- 선행 작업: TOKEN-01 · APP-02 · PAY-02 / 결정: D10
- 산출물: 잔액/송수신/가스 UI와 선택한 wrapping 흐름
- 수용 기준: 가스는 네이티브 잔액으로 확인 · wrapped 포함 여부에 맞는 앱 검증
- 독립 준비: manifest/asset/decoder/AA typed attribution 예제
- 통합 입력: 실chainidentity·addresses/ABI/codehash·tokenunits·AA버전·Indexerendpoint
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-009 / —

## INDEX-01 Indexer 대상 체인·배포 계약 구성

- 설계: [DS-04](../specifications/stablenet-compatibility-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 12 / 세부 작업: INDEX-01.01 · INDEX-01.02 · INDEX-01.03
- 선행 작업: BASE-04 · BASE-05 / 결정: D10
- 산출물: 노드/저장/계약/이벤트 설정
- 수용 기준: 지정 RPC와 관측 블록 해시 대조 · 체인별 데이터 혼합 방지
- 독립 준비: manifest/asset/decoder/AA typed attribution 예제
- 통합 입력: 실chainidentity·addresses/ABI/codehash·tokenunits·AA버전·Indexerendpoint
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / —

## INDEX-02 SDK 조회 계약·금액/이벤트 변환 수정

- 설계: [DS-04](../specifications/stablenet-compatibility-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 12 / 세부 작업: INDEX-02.01 · INDEX-02.02 · INDEX-02.03
- 선행 작업: INDEX-01 / 결정: D02
- 산출물: RPC/GraphQL 어댑터와 조회 예제
- 수용 기준: 객체/배열·wrapper·receipt 필드 차이 해소 · 오류와 빈 결과 구분
- 독립 준비: manifest/asset/decoder/AA typed attribution 예제
- 통합 입력: 실chainidentity·addresses/ABI/codehash·tokenunits·AA버전·Indexerendpoint
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / —

## INDEX-03 진행 커서·backfill·중복/재구성 처리

- 설계: [DS-04](../specifications/stablenet-compatibility-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 12 / 세부 작업: INDEX-03.01 · INDEX-03.02 · INDEX-03.03
- 선행 작업: INDEX-02 / 결정: D08 · D10
- 산출물: 복구 가능한 이벤트 수집 흐름
- 수용 기준: 재연결 누락 복구 · 같은 log 중복 반영 없음 · 되돌림 정책 검증
- 독립 준비: manifest/asset/decoder/AA typed attribution 예제
- 통합 입력: 실chainidentity·addresses/ABI/codehash·tokenunits·AA버전·Indexerendpoint
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: payment.observation.changed / —

## INDEX-04 확장 계약군 ABI·이벤트·조회 등록

- 설계: [DS-04](../specifications/stablenet-compatibility-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 12 / 세부 작업: INDEX-04.01 · INDEX-04.02 · INDEX-04.03
- 선행 작업: INDEX-03 · SMART-02 · DEX-02 · FX-02 · PERP-04 · STO-02 · DID-02 · X402-02 / 결정: D10
- 산출물: 영역별 이벤트 명세와 조회 API
- 수용 기준: 각 필수 계약의 실제 호출 결과 조회 · 알려지지 않은 ABI/지연을 명시
- 독립 준비: manifest/asset/decoder/AA typed attribution 예제
- 통합 입력: 실chainidentity·addresses/ABI/codehash·tokenunits·AA버전·Indexerendpoint
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / —

## INDEX-05 앱 거래 상세·탐색기·상태 관측 연결

- 설계: [DS-04](../specifications/stablenet-compatibility-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 12 / 세부 작업: INDEX-05.01 · INDEX-05.02 · INDEX-05.03
- 선행 작업: INDEX-04 · OPS-03 / 결정: D08 · D10
- 산출물: 거래/이벤트 상세와 상태 화면
- 수용 기준: 모든 앱 거래에서 식별자 추적 · 표시상 confirmed와 최종 확정 구분
- 독립 준비: manifest/asset/decoder/AA typed attribution 예제
- 통합 입력: 실chainidentity·addresses/ABI/codehash·tokenunits·AA버전·Indexerendpoint
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-019 · API-089 / U11

## PAY-01 EOA 견적·수수료·서명 요청 구성

- 설계: [DS-03](../specifications/kiosk-commerce-journey-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 1 · 7 · 9 · 12 / 세부 작업: PAY-01.01 · PAY-01.02 · PAY-01.03
- 선행 작업: BASE-03 · TOKEN-01 / 결정: D08
- 산출물: 토큰 전송·nonce/가스·승인 데이터 계약
- 수용 기준: 구매대금/가스 분리 · 잘못된 체인·금액·수취 거절 · 견적 유효성 규칙
- 독립 준비: order/payment/refund/fulfillment/settlement 계약 예제
- 통합 입력: 태블릿/배포·점주권한·payment policy·가게/메뉴/recipient·현재체인/indexer
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-017 · API-032 · API-034 / U03

## PAY-02 서명 거래 제출·재시도·상태 추적

- 설계: [DS-03](../specifications/kiosk-commerce-journey-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 1 · 7 · 9 · 12 / 세부 작업: PAY-02.01 · PAY-02.02 · PAY-02.03
- 선행 작업: PAY-01 · BASE-05 / 결정: D08
- 산출물: 중계 API·트랜잭션 시도 저장
- 수용 기준: 중계에 고객 키 불필요 · 제출 불명확 때 재조회 · 재시도와 신규 거래 구분
- 독립 준비: order/payment/refund/fulfillment/settlement 계약 예제
- 통합 입력: 태블릿/배포·점주권한·payment policy·가게/메뉴/recipient·현재체인/indexer
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-018 · API-020 / U03

## PAY-03 주문 견적·결제 시도와 거래 연결

- 설계: [DS-03](../specifications/kiosk-commerce-journey-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 1 · 7 · 9 · 12 / 세부 작업: PAY-03.01 · PAY-03.02 · PAY-03.03
- 선행 작업: PAY-02 · SHOP-02 / 결정: D08
- 산출물: 주문-결제 시도 원장
- 수용 기준: 메뉴 가격 스냅샷 보관 · 거래/이벤트를 여러 주문에 재사용 불가
- 독립 준비: order/payment/refund/fulfillment/settlement 계약 예제
- 통합 입력: 태블릿/배포·점주권한·payment policy·가게/메뉴/recipient·현재체인/indexer
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-029 · API-032 · API-104 · API-105 / K02

## PAY-04 입금 검증·확정·매출 대사 구현

- 설계: [DS-03](../specifications/kiosk-commerce-journey-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 1 · 7 · 9 · 12 / 세부 작업: PAY-04.01 · PAY-04.02 · PAY-04.03 · PAY-04.04
- 선행 작업: PAY-03 · INDEX-03 · TOKEN-02 / 결정: D08
- 산출물: 결제 관측→주문 상태 전이
- 수용 기준: 체인/토큰/수취/금액/receipt/확정 정책 검증 · 중복/오입금/지연 분리 · 사전 승인 거래·지급 주체·attempt 귀속 검증; 스마트 계정은 외부 tx.from만으로 소유자 판정하지 않음
- 독립 준비: order/payment/refund/fulfillment/settlement 계약 예제
- 통합 입력: 태블릿/배포·점주권한·payment policy·가게/메뉴/recipient·현재체인/indexer
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: payment.acceptance.changed · API-035 · API-104 · API-105 / —

## PAY-05 실제 NU 키오스크 결제 수직 연결

- 설계: [DS-03](../specifications/kiosk-commerce-journey-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 1 · 7 · 9 · 12 / 세부 작업: PAY-05.01 · PAY-05.02 · PAY-05.03
- 선행 작업: PAY-04 · HW-05 · HW-06 · SHOP-03 / 결정: D08
- 산출물: 기기 승인부터 주문 완료까지 실행 기록
- 수용 기준: 실기 승인·거절·거리 이탈·앱 재시작 시험 · 정상 결제 1건이 매출 1회
- 독립 준비: order/payment/refund/fulfillment/settlement 계약 예제
- 통합 입력: 태블릿/배포·점주권한·payment policy·가게/메뉴/recipient·현재체인/indexer
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / K03

## SHOP-01 RN 태블릿·매장 가입·로그인·관리 모드

- 설계: [DS-03](../specifications/kiosk-commerce-journey-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 9 / 세부 작업: SHOP-01.01 · SHOP-01.02 · SHOP-01.03
- 선행 작업: AUTH-02 · AUTH-03 · BASE-03 / 결정: D01
- 산출물: 설치 앱·가게 프로필/소속
- 수용 기준: Google/Apple 실제 로그인 · 관리 복귀 인증 · 다른 매장 차단
- 독립 준비: order/payment/refund/fulfillment/settlement 계약 예제
- 통합 입력: 태블릿/배포·점주권한·payment policy·가게/메뉴/recipient·현재체인/indexer
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-021 · API-022 · API-023 · API-024 · API-025 / K01

## SHOP-02 메뉴·가격·품절·장바구니·주문 구현

- 설계: [DS-03](../specifications/kiosk-commerce-journey-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 9 / 세부 작업: SHOP-02.01 · SHOP-02.02 · SHOP-02.03
- 선행 작업: SHOP-01 / 결정: D08
- 산출물: 매장 메뉴/주문 API와 RN 화면
- 수용 기준: 메뉴 변경이 과거 주문을 바꾸지 않음 · 주문 중복/품절 처리
- 독립 준비: order/payment/refund/fulfillment/settlement 계약 예제
- 통합 입력: 태블릿/배포·점주권한·payment policy·가게/메뉴/recipient·현재체인/indexer
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-026 · API-027 · API-029 · API-031 · API-092 / K02

## SHOP-03 키오스크 결제 화면·기기 세션 연결

- 설계: [DS-03](../specifications/kiosk-commerce-journey-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 9 / 세부 작업: SHOP-03.01 · SHOP-03.02 · SHOP-03.03 · SHOP-03.04
- 선행 작업: SHOP-02 · HW-03 · PAY-03 / 결정: D08
- 산출물: 주문→기기 승인 요청 화면
- 수용 기준: 매장·금액 확인 · 승인 대기/거절/미확정 상태 구분 · 중복 탭 처리
- 독립 준비: order/payment/refund/fulfillment/settlement 계약 예제
- 통합 입력: 태블릿/배포·점주권한·payment policy·가게/메뉴/recipient·현재체인/indexer
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-030 · API-033 · API-108 · API-109 · API-110 / K03

## SHOP-04 환불 요청·승인·서명·입금 추적

- 설계: [DS-03](../specifications/kiosk-commerce-journey-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 9 / 세부 작업: SHOP-04.01 · SHOP-04.02 · SHOP-04.03
- 선행 작업: PAY-04 · APP-03 / 결정: D08 · D09
- 산출물: 환불 UI/원장/별도 거래
- 수용 기준: 원지급 유지 · 누적 한도 확인 · 반납 주소·수수료·부분 환불 정책 반영
- 독립 준비: order/payment/refund/fulfillment/settlement 계약 예제
- 통합 입력: 태블릿/배포·점주권한·payment policy·가게/메뉴/recipient·현재체인/indexer
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: refund.state.changed · API-036 · API-037 · API-038 · API-092 / K05

## SHOP-05 매출·수취·환불·정산 대조 화면

- 설계: [DS-03](../specifications/kiosk-commerce-journey-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 9 / 세부 작업: SHOP-05.01 · SHOP-05.02 · SHOP-05.03
- 선행 작업: SHOP-04 · PAY-04 / 결정: D08
- 산출물: 일자/매장별 집계·차이 조회
- 수용 기준: 지갑 잔액을 매출로 취급하지 않음 · 주문과 체인 원장 대조 가능
- 독립 준비: order/payment/refund/fulfillment/settlement 계약 예제
- 통합 입력: 태블릿/배포·점주권한·payment policy·가게/메뉴/recipient·현재체인/indexer
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-039 · API-040 · API-041 · API-093 / K06

## STAMP-01 스탬프 발급·사용·취소 원장

- 설계: [DS-03](../specifications/kiosk-commerce-journey-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 3 · 7 · 8 · 10 · 14 / 세부 작업: STAMP-01.01 · STAMP-01.02 · STAMP-01.03
- 선행 작업: PAY-04 · AUTH-01 / 결정: D09
- 산출물: 혜택 API·발급 규칙
- 수용 기준: 확정 결제 1회 적립 · 중복 사용 차단 · 환불 정책 적용
- 독립 준비: order/payment/refund/fulfillment/settlement 계약 예제
- 통합 입력: 태블릿/배포·점주권한·payment policy·가게/메뉴/recipient·현재체인/indexer
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: benefit.ledger.changed · API-043 · API-044 / U10

## STAMP-02 앱·기기 스탬프 표시/사용 연결

- 설계: [DS-03](../specifications/kiosk-commerce-journey-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 3 · 7 · 8 · 10 · 14 / 세부 작업: STAMP-02.01 · STAMP-02.02 · STAMP-02.03
- 선행 작업: STAMP-01 · HW-03 · APP-04 / 결정: D09
- 산출물: 카페 패스포트 UI·기기 표시
- 수용 기준: 두 화면이 같은 원장 결과 표시 · 기기 교체/오프라인 뒤 동기화
- 독립 준비: order/payment/refund/fulfillment/settlement 계약 예제
- 통합 입력: 태블릿/배포·점주권한·payment policy·가게/메뉴/recipient·현재체인/indexer
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: passport.sync · API-043 / U10 · D02

## STAMP-03 대여·반납·잔액 회수·기기 해제

- 설계: [DS-08](../specifications/operations-release-acceptance-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 3 · 7 · 8 · 10 · 14 / 세부 작업: STAMP-03.01 · STAMP-03.02 · STAMP-03.03 · STAMP-03.04 · STAMP-03.05
- 선행 작업: HW-04 · PAY-02 · AUTH-01 / 결정: D03 · D09
- 산출물: 운영/앱 대여 수명주기
- 수용 기준: 신규 여행 지갑은 가스를 고려해 회수; import 지갑은 외부 접근 확인 후 기기 사본 삭제 · 키 삭제 전 상태 확인 · 이전 사용자 접근 해제
- 독립 준비: 복구/릴리스/운영 권한·evidence manifest 예제
- 통합 입력: 선정환경·정량목표·접근권한·실복구·모든기능실앱증거
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-045 · API-046 · API-091 / U24 · O01

## OPS-01 운영자 권한·감사·관리 화면 골격

- 설계: [DS-08](../specifications/operations-release-acceptance-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 10 · 15 / 세부 작업: OPS-01.01 · OPS-01.02 · OPS-01.03
- 선행 작업: AUTH-01 · BASE-05 / 결정: D19
- 산출물: 백오피스 인증·행위 로그
- 수용 기준: 최소 역할별 접근 · 고객 키/녹음/위치 기본 열람 금지 · 변경 이력 보존
- 독립 준비: 복구/릴리스/운영 권한·evidence manifest 예제
- 통합 입력: 선정환경·정량목표·접근권한·실복구·모든기능실앱증거
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-088 / O01 · O04

## OPS-02 가맹점·대여·펌웨어 릴리스 운영 연결

- 설계: [DS-08](../specifications/operations-release-acceptance-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 10 · 15 / 세부 작업: OPS-02.01 · OPS-02.02 · OPS-02.03
- 선행 작업: OPS-01 · STAMP-03 · OTA-02 / 결정: D05 · D09
- 산출물: 운영 조회·등록·정책 변경 UI
- 수용 기준: 실제 매장/기기/업데이트 상태 연결 · 중요 설정 변경 추적
- 독립 준비: 복구/릴리스/운영 권한·evidence manifest 예제
- 통합 입력: 선정환경·정량목표·접근권한·실복구·모든기능실앱증거
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-049 · API-050 · API-099 · API-100 · API-101 / O01 · O02

## OPS-03 결제 예외·Indexer 지연·대사 지원

- 설계: [DS-08](../specifications/operations-release-acceptance-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 10 · 15 / 세부 작업: OPS-03.01 · OPS-03.02 · OPS-03.03
- 선행 작업: OPS-01 · PAY-04 · INDEX-03 / 결정: D08 · D19
- 산출물: 거래 재조회/차이 처리 도구
- 수용 기준: 운영자가 임의 온체인 성공 생성 불가 · 재조회로 중복 매출 없음
- 독립 준비: 복구/릴리스/운영 권한·evidence manifest 예제
- 통합 입력: 선정환경·정량목표·접근권한·실복구·모든기능실앱증거
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-086 · API-087 · API-089 / O03

## SMART-01 EOA 이후 계정 전환·서명 모델 설계

- 설계: [DS-04](../specifications/stablenet-compatibility-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 / 세부 작업: SMART-01.01 · SMART-01.02 · SMART-01.03
- 선행 작업: PAY-01 · MPC-01 · BASE-04 / 결정: D11
- 산출물: 주소/자산/권한·복구 전환 명세
- 수용 기준: 키 가져오기 유지 · 자산 자동 이전으로 가정하지 않음 · 대상 버전 결정
- 독립 준비: manifest/asset/decoder/AA typed attribution 예제
- 통합 입력: 실chainidentity·addresses/ABI/codehash·tokenunits·AA버전·Indexerendpoint
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-055 · API-107 / U13

## SMART-02 계정 계약·SDK·Bundler 실행 연결

- 설계: [DS-04](../specifications/stablenet-compatibility-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 / 세부 작업: SMART-02.01 · SMART-02.02 · SMART-02.03
- 선행 작업: SMART-01 · TOKEN-02 · RELEASE-01 / 결정: D11
- 산출물: 배포/해시·서명·실행 경로
- 수용 기준: SDK와 EntryPoint 결과 일치 · 개별 UserOperation 성공/실패 구분
- 독립 준비: manifest/asset/decoder/AA typed attribution 예제
- 통합 입력: 실chainidentity·addresses/ABI/codehash·tokenunits·AA버전·Indexerendpoint
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-056 / U13

## SMART-03 앱 계정 전환·실제 서명·결과 연결

- 설계: [DS-04](../specifications/stablenet-compatibility-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 / 세부 작업: SMART-03.01 · SMART-03.02 · SMART-03.03
- 선행 작업: SMART-02 · HW-05 · MPC-03 · APP-03 / 결정: D11
- 산출물: EOA/스마트 계정 선택·전환 UI
- 수용 기준: 합의한 signer로 거래 · 자산/주소/권한 변화 표시 · 복구 시나리오
- 독립 준비: manifest/asset/decoder/AA typed attribution 예제
- 통합 입력: 실chainidentity·addresses/ABI/codehash·tokenunits·AA버전·Indexerendpoint
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-055 · API-107 / U13

## DEX-01 DeFi 대표 기능·pool·유동성 명세

- 설계: [DS-05](../specifications/market-product-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 · 13 / 세부 작업: DEX-01.01 · DEX-01.02 · DEX-01.03
- 선행 작업: BASE-04 · TOKEN-01 / 결정: D12
- 산출물: 상품/토큰/가격·실행 API 명세
- 수용 기준: 필수 DeFi 행위 지정 · V2 가상 reserve·V3 실제 경로 구분
- 독립 준비: actionvariant/견적/가격상태/노출/position 예제
- 통합 입력: 선정pool/FX/Perpmodel·risk/formula/oracle/keeper·liquidity·실contract
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / U14

## DEX-02 pool·견적·승인·swap/유동성 실행

- 설계: [DS-05](../specifications/market-product-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 · 13 / 세부 작업: DEX-02.01 · DEX-02.02 · DEX-02.03
- 선행 작업: DEX-01 · TOKEN-02 · PAY-02 / 결정: D12
- 산출물: 계약/DEX 서비스·실제 실행 경로
- 수용 기준: 만료/슬리피지·잔액/allowance 처리 · 고정 가상값을 실제 quote로 사용하지 않음
- 독립 준비: actionvariant/견적/가격상태/노출/position 예제
- 통합 입력: 선정pool/FX/Perpmodel·risk/formula/oracle/keeper·liquidity·실contract
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-057 / U14

## DEX-03 앱 DeFi·DEX 거래/유동성·이력 연결

- 설계: [DS-05](../specifications/market-product-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 · 13 / 세부 작업: DEX-03.01 · DEX-03.02 · DEX-03.03
- 선행 작업: DEX-02 · APP-03 · INDEX-03 / 결정: D12
- 산출물: 사용자 입력→서명→실행→결과 UI
- 수용 기준: 필수 swap/입출금 행위 실제 테스트넷 수행 · 실패 때 상태/자산 대조
- 독립 준비: actionvariant/견적/가격상태/노출/position 예제
- 통합 입력: 선정pool/FX/Perpmodel·risk/formula/oracle/keeper·liquidity·실contract
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-058 · API-059 / U14

## FX-01 FX 통화 쌍·상품·가격·규칙 정의

- 설계: [DS-05](../specifications/market-product-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 · 13 / 세부 작업: FX-01.01 · FX-01.02 · FX-01.03
- 선행 작업: DEX-01 / 결정: D12
- 산출물: FX 모델과 앱/계약 명세
- 수용 기준: spot/파생상품 범위 구별 · 시험 자산·견적/정밀도·가격 출처 지정
- 독립 준비: actionvariant/견적/가격상태/노출/position 예제
- 통합 입력: 선정pool/FX/Perpmodel·risk/formula/oracle/keeper·liquidity·실contract
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / U15

## FX-02 FX 계약/거래 서비스 연결

- 설계: [DS-05](../specifications/market-product-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 · 13 / 세부 작업: FX-02.01 · FX-02.02 · FX-02.03
- 선행 작업: FX-01 · DEX-02 / 결정: D12
- 산출물: 선택 통화 쌍 실행 API/이벤트
- 수용 기준: 가격 만료·잘못된 단위·최소 수취 처리 · 테스트넷 실행 확인
- 독립 준비: actionvariant/견적/가격상태/노출/position 예제
- 통합 입력: 선정pool/FX/Perpmodel·risk/formula/oracle/keeper·liquidity·실contract
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-057 / U15

## FX-03 앱 FX 견적·교환·기록 연결

- 설계: [DS-05](../specifications/market-product-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 · 13 / 세부 작업: FX-03.01 · FX-03.02 · FX-03.03
- 선행 작업: FX-02 · APP-03 · INDEX-03 / 결정: D12
- 산출물: 통화 선택·견적·승인·결과 UI
- 수용 기준: 일반 swap과 FX 표시 규칙 일치 · 실제 실행 금액/수수료 기록
- 독립 준비: actionvariant/견적/가격상태/노출/position 예제
- 통합 입력: 선정pool/FX/Perpmodel·risk/formula/oracle/keeper·liquidity·실contract
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-058 / U15

## PERP-01 시장·위험·가격/청산 명세

- 설계: [DS-05](../specifications/market-product-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 · 13 / 세부 작업: PERP-01.01 · PERP-01.02 · PERP-01.03
- 선행 작업: BASE-04 / 결정: D13
- 산출물: 시장 파라미터·정산/실패 시나리오
- 수용 기준: 고정 가격/TODO 대체 작업 목록 · 증거금·펀딩·청산 기준 명시
- 독립 준비: actionvariant/견적/가격상태/노출/position 예제
- 통합 입력: 선정pool/FX/Perpmodel·risk/formula/oracle/keeper·liquidity·실contract
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / U16

## PERP-02 가격 공급·오라클 상태 처리

- 설계: [DS-05](../specifications/market-product-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 · 13 / 세부 작업: PERP-02.01 · PERP-02.02 · PERP-02.03
- 선행 작업: PERP-01 · BASE-05 / 결정: D13
- 산출물: 테스트 가격/오라클 어댑터
- 수용 기준: 가격 시점·이상/지연 상태 검증 · 고정값 경로가 완료 증거에 섞이지 않음
- 독립 준비: actionvariant/견적/가격상태/노출/position 예제
- 통합 입력: 선정pool/FX/Perpmodel·risk/formula/oracle/keeper·liquidity·실contract
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / U16

## PERP-03 증거금·포지션 개설/축소/종료 구현

- 설계: [DS-05](../specifications/market-product-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 · 13 / 세부 작업: PERP-03.01 · PERP-03.02 · PERP-03.03
- 선행 작업: PERP-02 · TOKEN-02 · PAY-02 / 결정: D13
- 산출물: 계약과 주문/포지션 서비스
- 수용 기준: 담보 이동·손익·포지션 상태 일치 · 초과/잘못된 주문 거절
- 독립 준비: actionvariant/견적/가격상태/노출/position 예제
- 통합 입력: 선정pool/FX/Perpmodel·risk/formula/oracle/keeper·liquidity·실contract
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-061 / U16

## PERP-04 펀딩·청산·장애 시 상태 복구 구현

- 설계: [DS-05](../specifications/market-product-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 · 13 / 세부 작업: PERP-04.01 · PERP-04.02 · PERP-04.03
- 선행 작업: PERP-03 / 결정: D13
- 산출물: 펀딩/청산 실행·기록
- 수용 기준: 항상 0/불가인 stub 해소 · 실제 잔액/포지션 변화와 경계 조건 검증
- 독립 준비: actionvariant/견적/가격상태/노출/position 예제
- 통합 입력: 선정pool/FX/Perpmodel·risk/formula/oracle/keeper·liquidity·실contract
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / U16

## PERP-05 앱 시장·주문·포지션·청산 이력 연결

- 설계: [DS-05](../specifications/market-product-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 · 13 / 세부 작업: PERP-05.01 · PERP-05.02 · PERP-05.03
- 선행 작업: PERP-04 · APP-03 · INDEX-03 · PERP-06 / 결정: D13
- 산출물: Perpetual 전체 앱 흐름
- 수용 기준: 실제 주문·손익·펀딩·청산 결과 조회 · 오류/가격 중단 표시
- 독립 준비: actionvariant/견적/가격상태/노출/position 예제
- 통합 입력: 선정pool/FX/Perpmodel·risk/formula/oracle/keeper·liquidity·실contract
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-060 · API-061 · API-062 / U16

## STO-01 테스트 발행물·자격·권리 수명주기 정의

- 설계: [DS-06](../specifications/credential-paid-resource-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 / 세부 작업: STO-01.01 · STO-01.02 · STO-01.03
- 선행 작업: BASE-02 / 결정: D14
- 산출물: STO 대표 사용자 행위/계약 명세
- 수용 기준: 발행/보유/전송 조건 지정 · 실물 권리와 시험 데이터 표시 구분
- 독립 준비: issuer/holder/verifier/지급-제공 상태 예제
- 통합 입력: VC/DID/status suite·STO권리/제약contract·x402scheme/asset/facilitator
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / U17

## STO-02 발행·보유·전송 제한 계약/서비스 구현

- 설계: [DS-06](../specifications/credential-paid-resource-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 / 세부 작업: STO-02.01 · STO-02.02 · STO-02.03
- 선행 작업: STO-01 · TOKEN-01 · RELEASE-01 / 결정: D14
- 산출물: 테스트넷 계약·조회·이벤트
- 수용 기준: 허용/거절 시나리오 검증 · 권한 없는 발급/전송 차단
- 독립 준비: issuer/holder/verifier/지급-제공 상태 예제
- 통합 입력: VC/DID/status suite·STO권리/제약contract·x402scheme/asset/facilitator
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-064 / U17

## STO-03 앱 STO 자격·보유·거래 결과 연결

- 설계: [DS-06](../specifications/credential-paid-resource-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 / 세부 작업: STO-03.01 · STO-03.02 · STO-03.03
- 선행 작업: STO-02 · APP-03 · AUTH-01 · INDEX-03 / 결정: D14
- 산출물: 발행물 상세/행위/결과 UI
- 수용 기준: 합의한 대표 동작 실제 실행 · 제한 사유·자격과 지갑 통제 분리
- 독립 준비: issuer/holder/verifier/지급-제공 상태 예제
- 통합 입력: VC/DID/status suite·STO권리/제약contract·x402scheme/asset/facilitator
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-063 · API-064 · API-102 / U17

## DID-01 식별자·발급자·자격·철회 모델 정의

- 설계: [DS-06](../specifications/credential-paid-resource-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 / 세부 작업: DID-01.01 · DID-01.02 · DID-01.03
- 선행 작업: BASE-02 / 결정: D15
- 산출물: DID/자격 명세와 데이터 공개 범위
- 수용 기준: 검증자/발급자 구분 · 체인·오프체인 저장과 삭제 경계 명시
- 독립 준비: issuer/holder/verifier/지급-제공 상태 예제
- 통합 입력: VC/DID/status suite·STO권리/제약contract·x402scheme/asset/facilitator
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / U18

## DID-02 발급·검증·철회 서비스/필요 계약 구현

- 설계: [DS-06](../specifications/credential-paid-resource-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 / 세부 작업: DID-02.01 · DID-02.02 · DID-02.03
- 선행 작업: DID-01 · AUTH-01 · RELEASE-01 / 결정: D15
- 산출물: 테스트 자격 수명주기 API/이벤트
- 수용 기준: 잘못된 발급자·만료·철회 거절 · 재검증 결과 일치
- 독립 준비: issuer/holder/verifier/지급-제공 상태 예제
- 통합 입력: VC/DID/status suite·STO권리/제약contract·x402scheme/asset/facilitator
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: credential.status.changed · API-065 · API-068 · API-069 / U18

## DID-03 앱 자격 관리·제시·검증 결과 연결

- 설계: [DS-06](../specifications/credential-paid-resource-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 / 세부 작업: DID-03.01 · DID-03.02 · DID-03.03
- 선행 작업: DID-02 · APP-01 / 결정: D15
- 산출물: DID 화면/검증 흐름
- 수용 기준: 실제 발급/철회 결과 표시 · 불필요한 개인 정보 공개 방지
- 독립 준비: issuer/holder/verifier/지급-제공 상태 예제
- 통합 입력: VC/DID/status suite·STO권리/제약contract·x402scheme/asset/facilitator
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-066 · API-067 / U18

## X402-01 유료 자원·지불 규약·검증자 명세

- 설계: [DS-06](../specifications/credential-paid-resource-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 / 세부 작업: X402-01.01 · X402-01.02 · X402-01.03
- 선행 작업: BASE-03 · TOKEN-01 / 결정: D16
- 산출물: HTTP 결제 시나리오/지원 조합
- 수용 기준: StableNet 자산·버전·facilitator 지원 확인 · 실패/재시도 규칙
- 독립 준비: issuer/holder/verifier/지급-제공 상태 예제
- 통합 입력: VC/DID/status suite·STO권리/제약contract·x402scheme/asset/facilitator
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / U19

## X402-02 유료 요청·승인·지급 검증 연결

- 설계: [DS-06](../specifications/credential-paid-resource-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 / 세부 작업: X402-02.01 · X402-02.02 · X402-02.03
- 선행 작업: X402-01 · PAY-02 · TOKEN-02 / 결정: D16
- 산출물: 자원 서버·결제 검증·필요 계약
- 수용 기준: 미지불/위조/다른 요청 증명 거절 · 재요청 중복 과금 방지
- 독립 준비: issuer/holder/verifier/지급-제공 상태 예제
- 통합 입력: VC/DID/status suite·STO권리/제약contract·x402scheme/asset/facilitator
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-071 / U19

## X402-03 앱 유료 자원 이용·결제 결과 연결

- 설계: [DS-06](../specifications/credential-paid-resource-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 / 세부 작업: X402-03.01 · X402-03.02 · X402-03.03
- 선행 작업: X402-02 · APP-03 · INDEX-03 / 결정: D16
- 산출물: 가격 안내·승인·응답·이력 UI
- 수용 기준: 실제 HTTP 지불 흐름 통과 · 지불 후 응답 실패 복구 정책 적용
- 독립 준비: issuer/holder/verifier/지급-제공 상태 예제
- 통합 입력: VC/DID/status suite·STO권리/제약contract·x402scheme/asset/facilitator
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-070 · API-071 · API-072 / U19

## TRIP-01 장소 데이터·위치 권한·맛집 검색 연결

- 설계: [DS-07](../specifications/recording-travel-ai-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 7 · 14 / 세부 작업: TRIP-01.01 · TRIP-01.02 · TRIP-01.03
- 선행 작업: APP-01 · BASE-05 / 결정: D17
- 산출물: 지도/목록/상세·위치 API
- 수용 기준: 출처·갱신 시점 확인 · 권한 거절/대체 검색·위치 오류 처리
- 독립 준비: 녹음gap/consent/tombstone/provenance/course 예제
- 통합 입력: 실audio/backgroundprofile·place/STT/AIprovider·privacy/retention·challengepolicy
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-073 / U20

## TRIP-02 후기 작성·구매 출처·노출 정책 구현

- 설계: [DS-07](../specifications/recording-travel-ai-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 7 · 14 / 세부 작업: TRIP-02.01 · TRIP-02.02 · TRIP-02.03
- 선행 작업: TRIP-01 · AUTH-01 · PAY-04 / 결정: D17
- 산출물: 후기 API/화면
- 수용 기준: 본인 작성·수정/삭제 · 검증 구매·일반 후기 구별 · 운영 처리 기록
- 독립 준비: 녹음gap/consent/tombstone/provenance/course 예제
- 통합 입력: 실audio/backgroundprofile·place/STT/AIprovider·privacy/retention·challengepolicy
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-074 · API-075 · API-076 / U20

## TRIP-03 결제/위치 발자취·여행 모드 구현

- 설계: [DS-07](../specifications/recording-travel-ai-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 7 · 14 / 세부 작업: TRIP-03.01 · TRIP-03.02 · TRIP-03.03
- 선행 작업: TRIP-01 · APP-04 / 결정: D01 · D17
- 산출물: 타임라인·언어/시간대·필터 UI
- 수용 기준: 실제 구매/테스트 결제/위치 방문 출처 구분 · 동의/삭제/권한 처리
- 독립 준비: 녹음gap/consent/tombstone/provenance/course 예제
- 통합 입력: 실audio/backgroundprofile·place/STT/AIprovider·privacy/retention·challengepolicy
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-077 · API-078 · API-079 · API-097 / U21

## AI-01 추천 입력·코스 제약·평가 표본 정의

- 설계: [DS-07](../specifications/recording-travel-ai-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 14 / 세부 작업: AI-01.01 · AI-01.02 · AI-01.03
- 선행 작업: TRIP-02 · TRIP-03 / 결정: D18
- 산출물: 추천 규칙·데이터 연결·평가 시나리오
- 수용 기준: 위치/결제/후기 입력 근거 · 이동/영업시간·부족 데이터 처리 기준
- 독립 준비: 녹음gap/consent/tombstone/provenance/course 예제
- 통합 입력: 실audio/backgroundprofile·place/STT/AIprovider·privacy/retention·challengepolicy
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / U22

## AI-02 AI 코스 생성·근거·수정/저장 구현

- 설계: [DS-07](../specifications/recording-travel-ai-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 14 / 세부 작업: AI-02.01 · AI-02.02 · AI-02.03
- 선행 작업: AI-01 / 결정: D18
- 산출물: 추천 서비스와 앱 코스 UI
- 수용 기준: 실제 출처 연결 · 없는 장소/근거와 불가능한 코스 처리 · 저장/수정 가능
- 독립 준비: 녹음gap/consent/tombstone/provenance/course 예제
- 통합 입력: 실audio/backgroundprofile·place/STT/AIprovider·privacy/retention·challengepolicy
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-080 · API-081 · API-095 · API-107 / U22

## AI-03 따라하기 챌린지·방문/결제 증명 구현

- 설계: [DS-07](../specifications/recording-travel-ai-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 14 / 세부 작업: AI-03.01 · AI-03.02 · AI-03.03
- 선행 작업: AI-02 · STAMP-01 · TRIP-03 · INDEX-06 / 결정: D09 · D18
- 산출물: 참여·진행·발도장 원장/UI
- 수용 기준: 방문/결제/완료 구별 · 재시도·부정 참여·중복 보상 처리
- 독립 준비: 녹음gap/consent/tombstone/provenance/course 예제
- 통합 입력: 실audio/backgroundprofile·place/STT/AIprovider·privacy/retention·challengepolicy
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-082 · API-083 · API-096 / U23

## VERIFY-01 두 지갑·소셜·대여 수용 시험

- 설계: [DS-08](../specifications/operations-release-acceptance-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 1 · 2 · 3 · 4 · 5 · 6 · 7 · 8 · 9 · 10 · 11 · 12 · 13 · 14 · 15 / 세부 작업: VERIFY-01.01 · VERIFY-01.02 · VERIFY-01.03
- 선행 작업: PAY-05 · MPC-04 · AUTH-04 · STAMP-03 · SMART-03 · STAMP-04 · MPC-05 · BASE-06 / 결정: D03 · D04 · D09 · D19
- 산출물: 실기/Cloud/계정 변경 통합 증거
- 수용 기준: 키 가져오기/신규·MPC 복구·계정 연결·반납 후 접근 검증
- 독립 준비: 복구/릴리스/운영 권한·evidence manifest 예제
- 통합 입력: 선정환경·정량목표·접근권한·실복구·모든기능실앱증거
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / —

## VERIFY-02 기기 전체 기능·업데이트 공존 시험

- 설계: [DS-08](../specifications/operations-release-acceptance-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 1 · 2 · 3 · 4 · 5 · 6 · 7 · 8 · 9 · 10 · 11 · 12 · 13 · 14 · 15 / 세부 작업: VERIFY-02.01 · VERIFY-02.02 · VERIFY-02.03
- 선행 작업: OTA-03 · KEY-03 · REC-03 · FIND-02 · STAMP-02 · HW-07 · OTA-04 / 결정: D05 · D06 · D07 · D19
- 산출물: 실제 보드·앱 회귀 결과
- 수용 기준: FOTA/패스키/녹음/찾기/스탬프 함께 검증 · 자원·배터리·단절 수치 기록
- 독립 준비: 복구/릴리스/운영 권한·evidence manifest 예제
- 통합 입력: 선정환경·정량목표·접근권한·실복구·모든기능실앱증거
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / —

## VERIFY-03 매장·환불·정산·운영 수용 시험

- 설계: [DS-08](../specifications/operations-release-acceptance-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 1 · 2 · 3 · 4 · 5 · 6 · 7 · 8 · 9 · 10 · 11 · 12 · 13 · 14 · 15 / 세부 작업: VERIFY-03.01 · VERIFY-03.02 · VERIFY-03.03
- 선행 작업: PAY-05 · SHOP-05 · OPS-02 · OPS-03 · SHOP-06 · INDEX-06 / 결정: D08 · D09 · D19
- 산출물: 실제 카페 환경 실행 증거
- 수용 기준: 정상/취소/중복/지연/오입금·환불·매장 권한·운영 재조회 검증
- 독립 준비: 복구/릴리스/운영 권한·evidence manifest 예제
- 통합 입력: 선정환경·정량목표·접근권한·실복구·모든기능실앱증거
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / —

## VERIFY-04 9개 온체인 영역·DEX 앱 수용 시험

- 설계: [DS-08](../specifications/operations-release-acceptance-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 1 · 2 · 3 · 4 · 5 · 6 · 7 · 8 · 9 · 10 · 11 · 12 · 13 · 14 · 15 / 세부 작업: VERIFY-04.01 · VERIFY-04.02 · VERIFY-04.03
- 선행 작업: TOKEN-03 · DEX-03 · FX-03 · PERP-05 · STO-03 · DID-03 · X402-03 · SMART-03 · INDEX-05 / 결정: D10 · D11 · D12 · D13 · D14 · D15 · D16 · D19
- 산출물: 영역별 앱 시작→실행→이벤트 증거
- 수용 기준: 토큰/WKRC/DeFi/스마트 계정/FX/Perp/STO/DID/x402 각각 검증 · mock 제외
- 독립 준비: 복구/릴리스/운영 권한·evidence manifest 예제
- 통합 입력: 선정환경·정량목표·접근권한·실복구·모든기능실앱증거
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / —

## VERIFY-05 여행 추천·데이터/혜택 수용 시험

- 설계: [DS-08](../specifications/operations-release-acceptance-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 1 · 2 · 3 · 4 · 5 · 6 · 7 · 8 · 9 · 10 · 11 · 12 · 13 · 14 · 15 / 세부 작업: VERIFY-05.01 · VERIFY-05.02 · VERIFY-05.03
- 선행 작업: AI-03 · TRIP-03 · STAMP-02 · TRIP-04 · INDEX-06 / 결정: D17 · D18 · D19
- 산출물: 실제 데이터 출처·여행 시나리오 결과
- 수용 기준: 권한 거절·삭제·데이터 부족·후기/결제 증명·중복 발도장 검증
- 독립 준비: 복구/릴리스/운영 권한·evidence manifest 예제
- 통합 입력: 선정환경·정량목표·접근권한·실복구·모든기능실앱증거
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / —

## RELEASE-01 재현 빌드·CI·배포/ABI·시험 도구 구성

- 설계: [DS-08](../specifications/operations-release-acceptance-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 15 / 세부 작업: RELEASE-01.01 · RELEASE-01.02 · RELEASE-01.03
- 선행 작업: BASE-04 · BASE-05 / 결정: D10 · D19
- 산출물: 앱/기기/계약/서버 릴리스 도구
- 수용 기준: 버전·환경 추적 · 비밀 제외 · 개발 배포와 외부 배포 분리
- 독립 준비: 복구/릴리스/운영 권한·evidence manifest 예제
- 통합 입력: 선정환경·정량목표·접근권한·실복구·모든기능실앱증거
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / —

## RELEASE-02 관측·백업/복구·운영 절차 검증

- 설계: [DS-08](../specifications/operations-release-acceptance-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 15 / 세부 작업: RELEASE-02.01 · RELEASE-02.02 · RELEASE-02.03
- 선행 작업: OPS-03 · INDEX-03 · BASE-05 / 결정: D19
- 산출물: 로그/상태·복구 도구·런북
- 수용 기준: 업무 데이터·Indexer 복구 재현 · 민감 로그 배제 · 장애를 사용자에게 표시
- 독립 준비: 복구/릴리스/운영 권한·evidence manifest 예제
- 통합 입력: 선정환경·정량목표·접근권한·실복구·모든기능실앱증거
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / O04

## RELEASE-03 전체 증거 묶음·설치/운영 인수

- 설계: [DS-08](../specifications/operations-release-acceptance-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 15 / 세부 작업: RELEASE-03.01 · RELEASE-03.02 · RELEASE-03.03
- 선행 작업: VERIFY-01 · VERIFY-02 · VERIFY-03 · VERIFY-04 · VERIFY-05 · RELEASE-02 / 결정: D19
- 산출물: 15개 요구사항 인수 목록·릴리스 패키지
- 수용 기준: 각 항목 실제 앱 증거와 잔여 이슈 연결 · 실패 항목을 완료로 처리하지 않음
- 독립 준비: 복구/릴리스/운영 권한·evidence manifest 예제
- 통합 입력: 선정환경·정량목표·접근권한·실복구·모든기능실앱증거
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / —

## BASE-06 두 지갑·기능·서명 형식 지원 행렬 정의

- 설계: [DS-02](../specifications/social-wallet-recovery-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 4 · 8 · 9 · 10 · 15 / 세부 작업: BASE-06.01 · BASE-06.02 · BASE-06.03
- 선행 작업: BASE-03 / 결정: D03 · D04 · D11 · D12 · D15 · D16
- 산출물: HW/MPC × 송금/결제/스마트계정/DEX/DID/x402 행렬
- 수용 기준: 각 지원 조합의 주소·승인·서명 형식 명시 · 지원하지 않는 조합을 완료로 오표시하지 않음
- 독립 준비: auth/wallet/MPC phase·epoch·권한 fixture
- 통합 입력: Google/Apple client환경,MPC protocol/provider/threshold/recovery policy,실제participant
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / —

## HW-07 서명·FOTA·녹음·찾기 자원 경합 처리

- 설계: [DS-01](../specifications/device-coexistence-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 1 · 4 · 8 / 세부 작업: HW-07.01 · HW-07.02 · HW-07.03 · HW-07.04
- 선행 작업: HW-05 · OTA-01 · REC-02 · FIND-01 / 결정: D05 · D07
- 산출물: 기기 동작 우선순위·공존 제어
- 수용 기준: 동시 요청의 허용/거절 정의 · 버퍼/메모리/전력·BLE 실측 · 승인 내용 혼동 없음
- 독립 준비: service arbitration·key/approval·FOTA 상태 예제
- 통합 입력: 실제 NU board revision/pins/power,Zephyr/NCS/bootloader,key isolation,passkey/audio/ranging profile
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / D01 · D02

## OTA-04 앱·펌웨어·프로토콜·키 저장 호환 구현

- 설계: [DS-01](../specifications/device-coexistence-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 2 · 8 · 10 / 세부 작업: OTA-04.01 · OTA-04.02 · OTA-04.03
- 선행 작업: OTA-03 · BASE-03 · OPS-02 / 결정: D05
- 산출물: 버전 호환표·업데이트 철회/차단 경로
- 수용 기준: 구버전 앱 접속 결과 정의 · 잘못된 대상 차단 · 저장 형식 변경/키 보존 시험
- 독립 준비: service arbitration·key/approval·FOTA 상태 예제
- 통합 입력: 실제 NU board revision/pins/power,Zephyr/NCS/bootloader,key isolation,passkey/audio/ranging profile
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: firmware.release.withdrawn · API-050 · API-101 / U06 · O02

## MPC-05 참여자 장애·중단된 서명 세션 처리

- 설계: [DS-02](../specifications/social-wallet-recovery-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 5 · 6 / 세부 작업: MPC-05.01 · MPC-05.02 · MPC-05.03
- 선행 작업: MPC-03 · PAY-02 / 결정: D04
- 산출물: MPC 실패/재시도·복구 검증
- 수용 기준: 일부 참여자 장애 때 정책 준수 · 중단된 세션 재사용·중복 송금 차단 · 임계값 미달 서명 불가
- 독립 준비: auth/wallet/MPC phase·epoch·권한 fixture
- 통합 입력: Google/Apple client환경,MPC protocol/provider/threshold/recovery policy,실제participant
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / U12

## SHOP-06 수취 주소 변경·환불 서명 권한 분리

- 설계: [DS-03](../specifications/kiosk-commerce-journey-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 9 / 세부 작업: SHOP-06.01 · SHOP-06.02 · SHOP-06.03
- 선행 작업: SHOP-01 · SHOP-04 · OPS-01 / 결정: D08
- 산출물: 중요 매장 자금 행위의 권한/승인 UI
- 수용 기준: 메뉴 수정 권한만으로 주소/환불 변경 불가 · 업무 승인과 실제 서명 분리 · 감사 이력
- 독립 준비: order/payment/refund/fulfillment/settlement 계약 예제
- 통합 입력: 태블릿/배포·점주권한·payment policy·가게/메뉴/recipient·현재체인/indexer
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: API-008 · API-028 / K04

## STAMP-04 반납 시 미확정 거래·자격·계정 권한 정리

- 설계: [DS-08](../specifications/operations-release-acceptance-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 3 · 7 · 8 · 10 · 14 / 세부 작업: STAMP-04.01 · STAMP-04.02 · STAMP-04.03 · STAMP-04.04
- 선행 작업: STAMP-03 · KEY-03 · SMART-03 · PAY-04 / 결정: D03 · D06 · D09 · D11
- 산출물: 재대여 전 검사·완료/보류 상태
- 수용 기준: 미확정 거래 처리 정책 · 기기 신규 자격/위임과 기존 권한을 구분해 패스키/스마트 계정/이전 BLE 권한 정리 · 다음 대여자 격리
- 독립 준비: 복구/릴리스/운영 권한·evidence manifest 예제
- 통합 입력: 선정환경·정량목표·접근권한·실복구·모든기능실앱증거
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: device.reset.prepare · device.reset.confirm · device.binding.revoked · API-047 · API-091 / D01 · O01 · U04 · U05 · U24

## INDEX-06 관측 변경을 원장·혜택·여행에 전파

- 설계: [DS-04](../specifications/stablenet-compatibility-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 12 / 세부 작업: INDEX-06.01 · INDEX-06.02 · INDEX-06.03
- 선행 작업: INDEX-03 · PAY-04 · STAMP-01 · TRIP-03 / 결정: D08 · D09 · D17
- 산출물: 재구성/재처리 후 보정 흐름
- 수용 기준: 주문/매출/스탬프/추천 입력 보정과 이력 · 반복 처리 결과 동일 · 사용자 상태 갱신
- 독립 준비: manifest/asset/decoder/AA typed attribution 예제
- 통합 입력: 실chainidentity·addresses/ABI/codehash·tokenunits·AA버전·Indexerendpoint
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: payment.observation.changed · payment.acceptance.changed · travel.evidence.changed / O03

## PERP-06 가격·펀딩·청산 실행 서비스 운영 연결

- 설계: [DS-05](../specifications/market-product-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 11 · 13 / 세부 작업: PERP-06.01 · PERP-06.02 · PERP-06.03
- 선행 작업: PERP-04 · BASE-05 / 결정: D13 · D19
- 산출물: 필요 keeper/스케줄·상태·장애 감지
- 수용 기준: 합의한 실행 주체가 실제 호출 · 누락·반복 실행 처리 · 가격 장애 시 정책 적용
- 독립 준비: actionvariant/견적/가격상태/노출/position 예제
- 통합 입력: 선정pool/FX/Perpmodel·risk/formula/oracle/keeper·liquidity·실contract
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: — / U16

## TRIP-04 결제·장소 식별 및 개인 데이터 수명주기 연결

- 설계: [DS-07](../specifications/recording-travel-ai-design.md) + [통합계약](../specifications/preimplementation-contract-overlay.md)
- 요구사항: 7 · 14 / 세부 작업: TRIP-04.01 · TRIP-04.02 · TRIP-04.03
- 선행 작업: TRIP-03 · TRIP-02 · REC-03 · AUTH-04 / 결정: D07 · D17 · D18 · D19
- 산출물: 원천/동의/삭제·내보내기 매핑
- 수용 기준: 결제와 장소 연결 검증 · 실제/시험 출처 구분 · 녹음/후기/위치 삭제가 추천 입력에 반영
- 독립 준비: 녹음gap/consent/tombstone/provenance/course 예제
- 통합 입력: 실audio/backgroundprofile·place/STT/AIprovider·privacy/retention·challengepolicy
- 실패·복구: 해당 package runtimeCases 및원scenario/access/securitycases에서 정상대조+거절+응답유실+현재권한/원천변경+복구를선택; 적용되지않으면 이유기록
- 원 API/화면 참조: travel.evidence.changed · privacy.request.changed · API-054 · API-084 · API-085 / U09 · U21 · U25


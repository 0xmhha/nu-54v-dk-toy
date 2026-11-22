# 기능·유즈케이스와 화면 연결표

요구사항이나 유즈케이스 한 행에서 해당 페이지를 연다. 페이지의 ‘설계 검토’에서 정상·예외 상태와 외부 사건을 바꾸고 원문 인수 조건을 대조한다. [전체 페이지 설계](book.html) · [검토 결과와 기획 변경](audit.md)

## P01 · device-firmware

| 기준 | 기능 또는 사용자 목표 | 대응 페이지 |
|---|---|---|
| [P01.R1](../products/p01-device-firmware/prd.md) | 설치·등록 | [W03 빈 보드 설치](book.html#W03) · [W04 실물 보드 등록과 잠금](book.html#W04) · [W12 기기 유지관리와 반납](book.html#W12) · [D05 공개 보드 제작·설치](book.html#D05) |
| [P01.R2](../products/p01-device-firmware/prd.md) | 잠금·서명 승인 | [W04 실물 보드 등록과 잠금](book.html#W04) · [W08 보내기와 거래 추적](book.html#W08) · [W10 매장 결제 확인](book.html#W10) · [W14 DApp 로그인과 지출 요청](book.html#W14) · [W19 회차 청구와 영수증](book.html#W19) · [W27 설정·접근성·지원](book.html#W27) · [W28 체인별 거래 원문 확인](book.html#W28) |
| [P01.R3](../products/p01-device-firmware/prd.md) | 멀티체인 | [W07 받기](book.html#W07) · [W08 보내기와 거래 추적](book.html#W08) · [W17 Bitcoin PSBT 공동서명](book.html#W17) · [W25 계정·주소록·자산 관리](book.html#W25) · [W26 네트워크·RPC 관리](book.html#W26) · [W28 체인별 거래 원문 확인](book.html#W28) |
| [P01.R4](../products/p01-device-firmware/prd.md) | 개인 백업·복구 | [W05 백업과 첫 주소 검증](book.html#W05) · [W06 분실·교체 복구](book.html#W06) · [W27 설정·접근성·지원](book.html#W27) |
| [P01.R5](../products/p01-device-firmware/prd.md) | 대여 반납·개인 초기화 | [W12 기기 유지관리와 반납](book.html#W12) |
| [P01.R6](../products/p01-device-firmware/prd.md) | 계정 재발견·고급 승인 | [W15 세션과 온체인 권한](book.html#W15) · [W16 공개정보 내보내기](book.html#W16) · [W17 Bitcoin PSBT 공동서명](book.html#W17) · [W25 계정·주소록·자산 관리](book.html#W25) |
| [P01.UC1](../products/p01-device-firmware/use-cases.md) | 처음 만든 보드로 수신하기 | [W02 지갑 선택과 첫 연습](book.html#W02) · [W03 빈 보드 설치](book.html#W03) · [W04 실물 보드 등록과 잠금](book.html#W04) · [W05 백업과 첫 주소 검증](book.html#W05) · [W07 받기](book.html#W07) · [D05 공개 보드 제작·설치](book.html#D05) |
| [P01.UC2](../products/p01-device-firmware/use-cases.md) | 거래 승인 또는 거절 | [W08 보내기와 거래 추적](book.html#W08) · [W10 매장 결제 확인](book.html#W10) · [W11 매장 주문 영수증](book.html#W11) · [W14 DApp 로그인과 지출 요청](book.html#W14) · [W26 네트워크·RPC 관리](book.html#W26) · [W28 체인별 거래 원문 확인](book.html#W28) |
| [P01.UC3](../products/p01-device-firmware/use-cases.md) | 폰 분실과 대여 반납 | [W06 분실·교체 복구](book.html#W06) · [W12 기기 유지관리와 반납](book.html#W12) |
| [P01.UC4](../products/p01-device-firmware/use-cases.md) | 읽기 전용 내보내기와 공동 서명 | [W15 세션과 온체인 권한](book.html#W15) · [W16 공개정보 내보내기](book.html#W16) · [W17 Bitcoin PSBT 공동서명](book.html#W17) · [W25 계정·주소록·자산 관리](book.html#W25) |

## P02 · user-app

| 기준 | 기능 또는 사용자 목표 | 대응 페이지 |
|---|---|---|
| [P02.R1](../products/p02-user-app/prd.md) | 설정·튜토리얼 | [W02 지갑 선택과 첫 연습](book.html#W02) · [W04 실물 보드 등록과 잠금](book.html#W04) · [W05 백업과 첫 주소 검증](book.html#W05) · [W27 설정·접근성·지원](book.html#W27) |
| [P02.R2](../products/p02-user-app/prd.md) | 지갑·자산 | [W01 내 지갑](book.html#W01) · [W07 받기](book.html#W07) · [W08 보내기와 거래 추적](book.html#W08) · [W11 매장 주문 영수증](book.html#W11) · [W25 계정·주소록·자산 관리](book.html#W25) · [W26 네트워크·RPC 관리](book.html#W26) · [W28 체인별 거래 원문 확인](book.html#W28) |
| [P02.R3](../products/p02-user-app/prd.md) | 네트워크 관리 | [W26 네트워크·RPC 관리](book.html#W26) |
| [P02.R4](../products/p02-user-app/prd.md) | DApp 브라우저 | [W13 둘러보기와 DApp 연결](book.html#W13) · [W14 DApp 로그인과 지출 요청](book.html#W14) · [W15 세션과 온체인 권한](book.html#W15) |
| [P02.R5](../products/p02-user-app/prd.md) | 구독 | [W09 결제와 미결 목록](book.html#W09) · [W18 구독 약관과 일정](book.html#W18) · [W19 회차 청구와 영수증](book.html#W19) |
| [P02.R6](../products/p02-user-app/prd.md) | 매장·여행 | [T01 여행 탐색](book.html#T01) · [T04 예약금 지급 검토](book.html#T04) · [T06 좌석 없는 미리 주문](book.html#T06) |
| [P02.R7](../products/p02-user-app/prd.md) | 표시·분실 대응 | [W04 실물 보드 등록과 잠금](book.html#W04) · [W06 분실·교체 복구](book.html#W06) · [W08 보내기와 거래 추적](book.html#W08) · [W09 결제와 미결 목록](book.html#W09) · [W10 매장 결제 확인](book.html#W10) · [W11 매장 주문 영수증](book.html#W11) · [W27 설정·접근성·지원](book.html#W27) |
| [P02.R8](../products/p02-user-app/prd.md) | 계정·공개정보·보호 설정 | [W16 공개정보 내보내기](book.html#W16) · [W17 Bitcoin PSBT 공동서명](book.html#W17) · [W25 계정·주소록·자산 관리](book.html#W25) · [W27 설정·접근성·지원](book.html#W27) |
| [P02.R9](../products/p02-user-app/prd.md) | 단말과 계정 유지관리 | [W12 기기 유지관리와 반납](book.html#W12) · [W23 인증수단과 계정 종료](book.html#W23) · [W27 설정·접근성·지원](book.html#W27) |
| [P02.UC1](../products/p02-user-app/use-cases.md) | 초기 설정과 네트워크 전환 | [W01 내 지갑](book.html#W01) · [W02 지갑 선택과 첫 연습](book.html#W02) · [W04 실물 보드 등록과 잠금](book.html#W04) · [W05 백업과 첫 주소 검증](book.html#W05) · [W07 받기](book.html#W07) · [W25 계정·주소록·자산 관리](book.html#W25) · [W26 네트워크·RPC 관리](book.html#W26) · [W27 설정·접근성·지원](book.html#W27) · [W28 체인별 거래 원문 확인](book.html#W28) |
| [P02.UC2](../products/p02-user-app/use-cases.md) | DApp 접속·로그인·지출 | [W13 둘러보기와 DApp 연결](book.html#W13) · [W14 DApp 로그인과 지출 요청](book.html#W14) · [W15 세션과 온체인 권한](book.html#W15) |
| [P02.UC3](../products/p02-user-app/use-cases.md) | 회차 알림과 해지 | [W09 결제와 미결 목록](book.html#W09) · [W18 구독 약관과 일정](book.html#W18) · [W19 회차 청구와 영수증](book.html#W19) |
| [P02.UC4](../products/p02-user-app/use-cases.md) | 여행지 예약·디파짓·픽업 | [T02 시간 예약](book.html#T02) · [T04 예약금 지급 검토](book.html#T04) · [T05 예약 상세와 잔금](book.html#T05) · [T07 취소와 환불](book.html#T07) |
| [P02.UC5](../products/p02-user-app/use-cases.md) | 계정 재발견과 공개 범위 검토 | [W16 공개정보 내보내기](book.html#W16) · [W17 Bitcoin PSBT 공동서명](book.html#W17) · [W25 계정·주소록·자산 관리](book.html#W25) · [W27 설정·접근성·지원](book.html#W27) |
| [P02.UC6](../products/p02-user-app/use-cases.md) | 업데이트 중단과 진단 자료 확인 | [W12 기기 유지관리와 반납](book.html#W12) · [W23 인증수단과 계정 종료](book.html#W23) · [W27 설정·접근성·지원](book.html#W27) |

## P03 · cloud-mpc-wallet

| 기준 | 기능 또는 사용자 목표 | 대응 페이지 |
|---|---|---|
| [P03.R1](../products/p03-cloud-mpc-wallet/prd.md) | 생성·연계 | [W02 지갑 선택과 첫 연습](book.html#W02) · [W20 클라우드 지갑 만들기](book.html#W20) |
| [P03.R2](../products/p03-cloud-mpc-wallet/prd.md) | 일상 서명 | [W21 클라우드 서명 요청](book.html#W21) · [W24 MPC 사고·감사 조회](book.html#W24) |
| [P03.R3](../products/p03-cloud-mpc-wallet/prd.md) | 복구·갱신 | [W22 클라우드 참여자 복구](book.html#W22) · [W24 MPC 사고·감사 조회](book.html#W24) |
| [P03.R4](../products/p03-cloud-mpc-wallet/prd.md) | 계정 수명주기 | [W20 클라우드 지갑 만들기](book.html#W20) · [W22 클라우드 참여자 복구](book.html#W22) · [W23 인증수단과 계정 종료](book.html#W23) |
| [P03.UC1](../products/p03-cloud-mpc-wallet/use-cases.md) | 클라우드 계정 만들기 | [W20 클라우드 지갑 만들기](book.html#W20) · [W23 인증수단과 계정 종료](book.html#W23) |
| [P03.UC2](../products/p03-cloud-mpc-wallet/use-cases.md) | 거래 서명 | [W21 클라우드 서명 요청](book.html#W21) |
| [P03.UC3](../products/p03-cloud-mpc-wallet/use-cases.md) | 분실 휴대폰 교체 | [W22 클라우드 참여자 복구](book.html#W22) · [W23 인증수단과 계정 종료](book.html#W23) |
| [P03.UC4](../products/p03-cloud-mpc-wallet/use-cases.md) | 서비스 중단 또는 정족수 유출 | [W24 MPC 사고·감사 조회](book.html#W24) |

## P04 · merchant-kiosk

| 기준 | 기능 또는 사용자 목표 | 대응 페이지 |
|---|---|---|
| [P04.R1](../products/p04-merchant-kiosk/prd.md) | 모드·메뉴 | [K01 공용 메뉴](book.html#K01) · [K02 공용 장바구니](book.html#K02) · [K04 POS 결제 전용](book.html#K04) · [K06 단말 모드와 설정](book.html#K06) · [K07 메뉴와 옵션](book.html#K07) · [K09 공용 화면 인계와 종료](book.html#K09) |
| [P04.R2](../products/p04-merchant-kiosk/prd.md) | 병렬 주문 | [K01 공용 메뉴](book.html#K01) · [K02 공용 장바구니](book.html#K02) · [K03 폰에서 매장 찾기](book.html#K03) · [K07 메뉴와 옵션](book.html#K07) · [K09 공용 화면 인계와 종료](book.html#K09) · [K10 내 폰의 매장 장바구니](book.html#K10) |
| [P04.R3](../products/p04-merchant-kiosk/prd.md) | QR·BLE | [K02 공용 장바구니](book.html#K02) · [K03 폰에서 매장 찾기](book.html#K03) · [K04 POS 결제 전용](book.html#K04) · [K08 공용 주문 결제 연결](book.html#K08) · [K10 내 폰의 매장 장바구니](book.html#K10) · [K11 POS 지급자 선택](book.html#K11) |
| [P04.R4](../products/p04-merchant-kiosk/prd.md) | 조회·픽업 | [K05 확정 주문과 픽업](book.html#K05) · [K08 공용 주문 결제 연결](book.html#K08) · [K09 공용 화면 인계와 종료](book.html#K09) · [K11 POS 지급자 선택](book.html#K11) · [O02 지점 원주문과 이행](book.html#O02) |
| [P04.R5](../products/p04-merchant-kiosk/prd.md) | 데모 운영 | [K04 POS 결제 전용](book.html#K04) · [K08 공용 주문 결제 연결](book.html#K08) · [K10 내 폰의 매장 장바구니](book.html#K10) |
| [P04.R6](../products/p04-merchant-kiosk/prd.md) | 점주 현장 운영 | [K05 확정 주문과 픽업](book.html#K05) · [K06 단말 모드와 설정](book.html#K06) · [K12 직원 인증](book.html#K12) · [O01 지점 정산과 마감](book.html#O01) · [O02 지점 원주문과 이행](book.html#O02) · [O05 부분 환불 신청](book.html#O05) |
| [P04.UC1](../products/p04-merchant-kiosk/use-cases.md) | 공용 단말과 근거리 고객 동시 주문 | [K01 공용 메뉴](book.html#K01) · [K02 공용 장바구니](book.html#K02) · [K03 폰에서 매장 찾기](book.html#K03) · [K07 메뉴와 옵션](book.html#K07) · [K08 공용 주문 결제 연결](book.html#K08) · [K09 공용 화면 인계와 종료](book.html#K09) · [K10 내 폰의 매장 장바구니](book.html#K10) |
| [P04.UC2](../products/p04-merchant-kiosk/use-cases.md) | 결제 전용 POS의 QR/BLE 경쟁 | [K04 POS 결제 전용](book.html#K04) · [K11 POS 지급자 선택](book.html#K11) |
| [P04.UC3](../products/p04-merchant-kiosk/use-cases.md) | 제출 결과 유실 뒤 영업 재개 | [K05 확정 주문과 픽업](book.html#K05) · [K08 공용 주문 결제 연결](book.html#K08) · [K09 공용 화면 인계와 종료](book.html#K09) · [O02 지점 원주문과 이행](book.html#O02) |
| [P04.UC4](../products/p04-merchant-kiosk/use-cases.md) | 관리자 모드 전환과 점주 환불 | [K06 단말 모드와 설정](book.html#K06) · [K12 직원 인증](book.html#K12) · [O01 지점 정산과 마감](book.html#O01) · [O02 지점 원주문과 이행](book.html#O02) · [O05 부분 환불 신청](book.html#O05) |

## P05 · operations-backoffice

| 기준 | 기능 또는 사용자 목표 | 대응 페이지 |
|---|---|---|
| [P05.R1](../products/p05-operations-backoffice/prd.md) | 등록·권한 | [K06 단말 모드와 설정](book.html#K06) · [O03 메뉴와 가격 게시](book.html#O03) · [O08 지급 주소 변경](book.html#O08) · [O12 지점 운영 업무](book.html#O12) · [O13 지점 직원과 단말 권한](book.html#O13) |
| [P05.R2](../products/p05-operations-backoffice/prd.md) | 메뉴·예약·가격 | [O03 메뉴와 가격 게시](book.html#O03) · [O07 예약금·잔금과 구독 회차](book.html#O07) |
| [P05.R3](../products/p05-operations-backoffice/prd.md) | 주문·견적·릴레이 | [K11 POS 지급자 선택](book.html#K11) · [O02 지점 원주문과 이행](book.html#O02) · [O04 대사 차이와 이행 재처리](book.html#O04) · [O07 예약금·잔금과 구독 회차](book.html#O07) |
| [P05.R4](../products/p05-operations-backoffice/prd.md) | 구독·예약금 | [O07 예약금·잔금과 구독 회차](book.html#O07) |
| [P05.R5](../products/p05-operations-backoffice/prd.md) | 정산·환불 | [O01 지점 정산과 마감](book.html#O01) · [O02 지점 원주문과 이행](book.html#O02) · [O04 대사 차이와 이행 재처리](book.html#O04) · [O05 부분 환불 신청](book.html#O05) · [O06 환불 승인과 전송](book.html#O06) · [O08 지급 주소 변경](book.html#O08) · [O15 기존 계약 적립과 인출](book.html#O15) |
| [P05.R6](../products/p05-operations-backoffice/prd.md) | 운영·감사 | [K05 확정 주문과 픽업](book.html#K05) · [K12 직원 인증](book.html#K12) · [O01 지점 정산과 마감](book.html#O01) · [O02 지점 원주문과 이행](book.html#O02) · [O03 메뉴와 가격 게시](book.html#O03) · [O04 대사 차이와 이행 재처리](book.html#O04) · [O06 환불 승인과 전송](book.html#O06) · [O12 지점 운영 업무](book.html#O12) · [O13 지점 직원과 단말 권한](book.html#O13) |
| [P05.R7](../products/p05-operations-backoffice/prd.md) | 기존 대여·운영 | [O09 대여 기기 개설과 반납](book.html#O09) · [O10 FOTA 캠페인](book.html#O10) · [O12 지점 운영 업무](book.html#O12) · [O15 기존 계약 적립과 인출](book.html#O15) |
| [P05.UC1](../products/p05-operations-backoffice/use-cases.md) | 가격 게시와 지점 운영 | [O03 메뉴와 가격 게시](book.html#O03) · [O08 지급 주소 변경](book.html#O08) · [O12 지점 운영 업무](book.html#O12) · [O13 지점 직원과 단말 권한](book.html#O13) |
| [P05.UC2](../products/p05-operations-backoffice/use-cases.md) | 예약·회차 견적 후 고객 이탈 | [O07 예약금·잔금과 구독 회차](book.html#O07) |
| [P05.UC3](../products/p05-operations-backoffice/use-cases.md) | 부분 환불과 일마감 | [O02 지점 원주문과 이행](book.html#O02) · [O05 부분 환불 신청](book.html#O05) · [O06 환불 승인과 전송](book.html#O06) |
| [P05.UC4](../products/p05-operations-backoffice/use-cases.md) | 지점 마감과 동시성 복구 | [K05 확정 주문과 픽업](book.html#K05) · [O01 지점 정산과 마감](book.html#O01) · [O04 대사 차이와 이행 재처리](book.html#O04) · [O06 환불 승인과 전송](book.html#O06) |
| [P05.UC5](../products/p05-operations-backoffice/use-cases.md) | 대여 개설·반납과 FOTA 캠페인 | [O09 대여 기기 개설과 반납](book.html#O09) · [O10 FOTA 캠페인](book.html#O10) · [O12 지점 운영 업무](book.html#O12) · [O15 기존 계약 적립과 인출](book.html#O15) |

## P06 · stablenet-contracts

| 기준 | 기능 또는 사용자 목표 | 대응 페이지 |
|---|---|---|
| [P06.R1](../products/p06-stablenet-contracts/prd.md) | 거래 도메인 | [O08 지급 주소 변경](book.html#O08) · [O14 배포 프로필과 직접 지급 증거](book.html#O14) |
| [P06.R2](../products/p06-stablenet-contracts/prd.md) | 직접 지급 | [O14 배포 프로필과 직접 지급 증거](book.html#O14) · [X01 지급 영수증](book.html#X01) |
| [P06.R3](../products/p06-stablenet-contracts/prd.md) | 이벤트·환불 | [O05 부분 환불 신청](book.html#O05) · [O06 환불 승인과 전송](book.html#O06) · [O14 배포 프로필과 직접 지급 증거](book.html#O14) |
| [P06.R4](../products/p06-stablenet-contracts/prd.md) | 배포·공존 | [O09 대여 기기 개설과 반납](book.html#O09) · [O14 배포 프로필과 직접 지급 증거](book.html#O14) · [O15 기존 계약 적립과 인출](book.html#O15) |
| [P06.UC1](../products/p06-stablenet-contracts/use-cases.md) | 지정 상점으로 지급 | [O14 배포 프로필과 직접 지급 증거](book.html#O14) |
| [P06.UC2](../products/p06-stablenet-contracts/use-cases.md) | 두 부분 환불 | [O05 부분 환불 신청](book.html#O05) · [O06 환불 승인과 전송](book.html#O06) · [O14 배포 프로필과 직접 지급 증거](book.html#O14) |
| [P06.UC3](../products/p06-stablenet-contracts/use-cases.md) | v1/v2 공존 | [O09 대여 기기 개설과 반납](book.html#O09) · [O14 배포 프로필과 직접 지급 증거](book.html#O14) · [O15 기존 계약 적립과 인출](book.html#O15) |

## P07 · indexer

| 기준 | 기능 또는 사용자 목표 | 대응 페이지 |
|---|---|---|
| [P07.R1](../products/p07-indexer/prd.md) | 대상·커서 | [X01 지급 영수증](book.html#X01) · [X02 영수증 검색](book.html#X02) · [X03 수집 상태와 체크포인트](book.html#X03) · [X05 격리 재생과 조회 전환](book.html#X05) |
| [P07.R2](../products/p07-indexer/prd.md) | 확정·재구성 | [X01 지급 영수증](book.html#X01) · [X03 수집 상태와 체크포인트](book.html#X03) · [X04 분기·확정 충돌 조사](book.html#X04) · [X05 격리 재생과 조회 전환](book.html#X05) |
| [P07.R3](../products/p07-indexer/prd.md) | 영수증 조회 | [X01 지급 영수증](book.html#X01) · [X02 영수증 검색](book.html#X02) · [X06 만료 경계의 지급 대사](book.html#X06) |
| [P07.R4](../products/p07-indexer/prd.md) | 대사·재생 | [X04 분기·확정 충돌 조사](book.html#X04) · [X05 격리 재생과 조회 전환](book.html#X05) · [X06 만료 경계의 지급 대사](book.html#X06) · [D06 서비스 복원·승격](book.html#D06) |
| [P07.UC1](../products/p07-indexer/use-cases.md) | 지급·환불 내역 찾기 | [X01 지급 영수증](book.html#X01) · [X02 영수증 검색](book.html#X02) · [X03 수집 상태와 체크포인트](book.html#X03) |
| [P07.UC2](../products/p07-indexer/use-cases.md) | 분기와 노드 장애 복구 | [X03 수집 상태와 체크포인트](book.html#X03) · [X04 분기·확정 충돌 조사](book.html#X04) · [X05 격리 재생과 조회 전환](book.html#X05) |
| [P07.UC3](../products/p07-indexer/use-cases.md) | 늦은 지급 재대사 | [X02 영수증 검색](book.html#X02) · [X05 격리 재생과 조회 전환](book.html#X05) · [X06 만료 경계의 지급 대사](book.html#X06) |

## P08 · market-services

| 기준 | 기능 또는 사용자 목표 | 대응 페이지 |
|---|---|---|
| [P08.R1](../products/p08-market-services/prd.md) | 교환·유동성 | [L01 테스트 시장](book.html#L01) · [L02 테스트 교환](book.html#L02) · [L03 유동성 공급과 회수](book.html#L03) |
| [P08.R2](../products/p08-market-services/prd.md) | TEST FX | [L01 테스트 시장](book.html#L01) · [L04 TEST FX](book.html#L04) |
| [P08.R3](../products/p08-market-services/prd.md) | 무기한 포지션 | [L01 테스트 시장](book.html#L01) · [L05 모의 무기한 포지션](book.html#L05) |
| [P08.R4](../products/p08-market-services/prd.md) | 가격·keeper 운영 | [L01 테스트 시장](book.html#L01) · [L05 모의 무기한 포지션](book.html#L05) · [L06 가격과 keeper 운영](book.html#L06) |
| [P08.UC1](../products/p08-market-services/use-cases.md) | 교환·LP 입출금 | [L02 테스트 교환](book.html#L02) · [L03 유동성 공급과 회수](book.html#L03) |
| [P08.UC2](../products/p08-market-services/use-cases.md) | 모의 환전 재견적 | [L04 TEST FX](book.html#L04) |
| [P08.UC3](../products/p08-market-services/use-cases.md) | 펀딩·청산 경합 | [L05 모의 무기한 포지션](book.html#L05) · [L06 가격과 keeper 운영](book.html#L06) |

## P09 · travel-ai

| 기준 | 기능 또는 사용자 목표 | 대응 페이지 |
|---|---|---|
| [P09.R1](../products/p09-travel-ai/prd.md) | 장소 검색 | [T01 여행 탐색](book.html#T01) · [T03 장소 상세](book.html#T03) |
| [P09.R2](../products/p09-travel-ai/prd.md) | 예약·미리 주문 | [T02 시간 예약](book.html#T02) · [T03 장소 상세](book.html#T03) · [T04 예약금 지급 검토](book.html#T04) · [T05 예약 상세와 잔금](book.html#T05) · [T06 좌석 없는 미리 주문](book.html#T06) · [T07 취소와 환불](book.html#T07) |
| [P09.R3](../products/p09-travel-ai/prd.md) | 영수증 기반 후기·보상 | [T08 후기와 이용 자격](book.html#T08) · [T09 스탬프와 혜택](book.html#T09) · [T10 내 여행](book.html#T10) · [T14 따라하기 챌린지](book.html#T14) |
| [P09.R4](../products/p09-travel-ai/prd.md) | 발자취·음성·AI | [T10 내 여행](book.html#T10) · [T11 기록과 동의](book.html#T11) · [T12 기록 삭제](book.html#T12) · [T13 AI 코스](book.html#T13) |
| [P09.R5](../products/p09-travel-ai/prd.md) | AI 코스와 따라하기 | [T10 내 여행](book.html#T10) · [T13 AI 코스](book.html#T13) · [T14 따라하기 챌린지](book.html#T14) |
| [P09.UC1](../products/p09-travel-ai/use-cases.md) | 위치 거부 후 명소 예약 | [T01 여행 탐색](book.html#T01) · [T02 시간 예약](book.html#T02) · [T03 장소 상세](book.html#T03) · [T04 예약금 지급 검토](book.html#T04) · [T05 예약 상세와 잔금](book.html#T05) |
| [P09.UC2](../products/p09-travel-ai/use-cases.md) | 영수증 후기와 스탬프 | [T08 후기와 이용 자격](book.html#T08) · [T09 스탬프와 혜택](book.html#T09) |
| [P09.UC3](../products/p09-travel-ai/use-cases.md) | 여행 기록 삭제 | [T11 기록과 동의](book.html#T11) · [T12 기록 삭제](book.html#T12) |
| [P09.UC4](../products/p09-travel-ai/use-cases.md) | 좌석 예약 없이 미리 주문하기 | [T03 장소 상세](book.html#T03) · [T06 좌석 없는 미리 주문](book.html#T06) · [T07 취소와 환불](book.html#T07) |
| [P09.UC5](../products/p09-travel-ai/use-cases.md) | AI 코스 적용과 따라하기 | [T13 AI 코스](book.html#T13) · [T14 따라하기 챌린지](book.html#T14) |

## P10 · platform

| 기준 | 기능 또는 사용자 목표 | 대응 페이지 |
|---|---|---|
| [P10.R1](../products/p10-platform/prd.md) | 공통 스펙·배포 | [D01 프로토콜·서비스 계약](book.html#D01) · [D02 SDK·ABI 호환표](book.html#D02) · [D03 상호운용 시험 증거](book.html#D03) |
| [P10.R2](../products/p10-platform/prd.md) | 역할·세션 | [D01 프로토콜·서비스 계약](book.html#D01) · [D02 SDK·ABI 호환표](book.html#D02) · [D03 상호운용 시험 증거](book.html#D03) |
| [P10.R3](../products/p10-platform/prd.md) | 릴리스·보안 | [D04 보안 공지·수정 릴리스](book.html#D04) · [D05 공개 보드 제작·설치](book.html#D05) |
| [P10.R4](../products/p10-platform/prd.md) | 호환·수용 | [D02 SDK·ABI 호환표](book.html#D02) · [D03 상호운용 시험 증거](book.html#D03) · [D04 보안 공지·수정 릴리스](book.html#D04) |
| [P10.R5](../products/p10-platform/prd.md) | 공개 설치·커스텀 | [D05 공개 보드 제작·설치](book.html#D05) |
| [P10.R6](../products/p10-platform/prd.md) | 서비스 계약과 재해 복구 | [D01 프로토콜·서비스 계약](book.html#D01) · [D06 서비스 복원·승격](book.html#D06) |
| [P10.UC1](../products/p10-platform/use-cases.md) | 프로토콜 변경과 교차 제품 import | [D01 프로토콜·서비스 계약](book.html#D01) · [D02 SDK·ABI 호환표](book.html#D02) · [D03 상호운용 시험 증거](book.html#D03) |
| [P10.UC2](../products/p10-platform/use-cases.md) | 보안 수정 출시 | [D04 보안 공지·수정 릴리스](book.html#D04) |
| [P10.UC3](../products/p10-platform/use-cases.md) | 직접 제작·커스텀 업데이트 | [D05 공개 보드 제작·설치](book.html#D05) |
| [P10.UC4](../products/p10-platform/use-cases.md) | 서비스 원장 복구와 계약 재생 | [D01 프로토콜·서비스 계약](book.html#D01) · [D06 서비스 복원·승격](book.html#D06) |


# 지갑·보드·클라우드 · 페이지별 설계

[전체 화면](../book.html) · [원 요구사항 대조표](../coverage.md) · [통합 감사](../audit.md)

제품에서 누르는 동작과 상태를 바꾸는 외부 사건을 열로 구분했다. 입력은 예시 데이터이며 실제 제품의 인수에서는 진입 조건과 검증 결과까지 함께 확인한다.

## W01 · 내 지갑

**사용자** 소유자　 **진입** 앱 잠금 해제 후 지갑 탭

**완료 기준** 프로필·체인·조회 시각을 함께 표시하고 다른 망의 자산은 합산하지 않는다.

[P02.R2](../../products/p02-user-app/prd.md) · [P02.UC1](../../products/p02-user-app/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [내 자산](../book.html#W01/start) | **42.00 TestUSDC**<br>개인 보드 · 여행 계정 1. 마지막 확정 잔액을 표시합니다. | 네트워크 — StableNet 테스트넷 · 8283<br>조회 시각 — 14:20 KST<br>지갑 상태 — 백업 확인 완료 | [받기](../book.html#W07)<br>[보내기](../book.html#W08)<br>[자산·계정 관리](../book.html#W25)<br>[결제](../book.html#W09)<br>[둘러보기](../book.html#W13)<br>[설정](../book.html#W27)<br>[지갑 선택](../book.html#W02)<br>[잔액 가리기](../book.html#W01/hidden)<br>[환산 시세](../book.html#W25/rate) | [네트워크 단절](../book.html#W01/offline) |
| [잔액 숨김](../book.html#W01/hidden) | **잔액을 가렸습니다**<br>자산을 확인하려면 다시 표시하세요. | 계정 — 여행 계정 1<br>체인 — StableNet 테스트넷 | [잔액 보기](../book.html#W01) | — |
| [오프라인](../book.html#W01/offline) | **마지막 조회 잔액 42.00 TestUSDC**<br>네트워크 연결 후 기존 거래를 조회하세요. | 캐시 시각 — 14:20 KST<br>현재 지급 가능액 — 확인 대기 | [최근 활동](../book.html#W09)<br>[마지막 환산 시세](../book.html#W25/stalerate) | [연결 복구 후 잔액 수신](../book.html#W01) |

## W02 · 지갑 선택과 첫 연습

**사용자** 소유자　 **진입** 첫 실행 또는 내 지갑 선택

**완료 기준** 연습에서 RPC 제출·보드 서명은 0건이며 중단 후 같은 연습으로 돌아온다.

[P01.UC1](../../products/p01-device-firmware/use-cases.md) · [P02.R1](../../products/p02-user-app/prd.md) · [P02.UC1](../../products/p02-user-app/use-cases.md) · [P03.R1](../../products/p03-cloud-mpc-wallet/prd.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [프로필 선택](../book.html#W02/start) | **어떤 지갑을 사용할까요?**<br>개인 보드는 소셜 가입 없이 시작합니다. 클라우드는 출시 조합에서만 선택할 수 있습니다. | 개인 — 내가 소유한 보드<br>대여 — 기존 대여 주소와 잔액 유지<br>클라우드 — 독립 모바일 참여자 | [버튼 연습](../book.html#W02/practice)<br>[새 보드 설치](../book.html#W03)<br>[설치된 보드 연결](../book.html#W04)<br>[대여 프로필 보기](../book.html#W12/rental)<br>[클라우드 지갑](../book.html#W20)<br>[기존 지갑](../book.html#W01) | — |
| [모의 결제](../book.html#W02/practice) | **연습 카페 · 2 연습 코인**<br>화면 속 보드 그림 오른쪽 버튼을 연습합니다. 실제 키와 거래는 사용하지 않습니다. | 모드 — 연습 전용<br>읽기 순서 — 금액 → 상점 → 체인 → 취소 | [모의 버튼 누르기](../book.html#W02/done)<br>[틀린 금액 거절 연습](../book.html#W02/reject)<br>[건너뛰기](../book.html#W04) | — |
| [연습 완료](../book.html#W02/done) | **연습 결제를 마쳤습니다**<br>앱을 닫아도 완료 단계가 저장됩니다. | 실거래 제출 — 0건 | [실물 보드 등록](../book.html#W04)<br>[연습 다시 보기](../book.html#W02/practice) | — |
| [거절 연습](../book.html#W02/reject) | **연습 주문을 거절했습니다**<br>금액이 다르면 요청을 닫고 새 견적을 받으세요. | 서명 — 0건 | [연습 다시 보기](../book.html#W02/practice)<br>[실물 보드 등록](../book.html#W04) | — |

## W03 · 빈 보드 설치

**사용자** 소유자　 **진입** 새 보드 설치 또는 부트 복구

**완료 기준** 모델·이미지·롤백 검사를 통과한 보드만 등록 단계로 이동한다.

[P01.R1](../../products/p01-device-firmware/prd.md) · [P01.UC1](../../products/p01-device-firmware/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [이미지 검사](../book.html#W03/start) | **설치할 보드와 이미지를 확인하세요**<br>컴퓨터에서 보드를 USB로 연결하고 해당 리비전용 서명 manifest를 선택합니다. | 대상 — NU54V · 리비전 A<br>배포 — 공식 테스트 프로필<br>확인 항목 — 모델 ID · 서명 · 최소 버전 | [설치 절차 보기](../book.html#W03/flash)<br>[돌아가기](../book.html#W02) | — |
| [기록·재부팅](../book.html#W03/flash) | **설치 도구의 기록 상태를 확인하세요**<br>도구에서 검증된 이미지를 기록하고 재부팅합니다. 보드 LED가 준비 상태인지 확인하세요. | 1단계 — 설치 도구에서 모델 ID 대조<br>2단계 — 서명 검사 통과 후 기록<br>3단계 — 재부팅 후 실행 버전 읽기 | [검사 항목 보기](../book.html#W03) | [부트 검증 통과](../book.html#W04)<br>[모델·서명 오류 또는 중단](../book.html#W03/error) |
| [복구 필요](../book.html#W03/error) | **부트 검증을 마치지 못했습니다**<br>복구 모드에서 기존 등록 키와 실행 슬롯을 조회하고 같은 설치를 재개하세요. | 지갑 생성 — 보류<br>복구 대상 — 마지막 검증 이미지 | [이미지 다시 선택](../book.html#W03) | — |

## W04 · 실물 보드 등록과 잠금

**사용자** 소유자　 **진입** 설치 완료 또는 실물 보드 연결

**완료 기준** 독립 지문과 보드 로컬 PIN 확인 뒤 소유 세대 하나만 활성화한다.

[P01.R1](../../products/p01-device-firmware/prd.md) · [P01.R2](../../products/p01-device-firmware/prd.md) · [P01.UC1](../../products/p01-device-firmware/use-cases.md) · [P02.R1](../../products/p02-user-app/prd.md) · [P02.R7](../../products/p02-user-app/prd.md) · [P02.UC1](../../products/p02-user-app/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [주변 기기](../book.html#W04/start) | **연결할 보드의 지문을 확인하세요**<br>보드에서 등록 창을 열고 설치 때 확보한 지문을 대조하세요. 표시 이름이 같은 기기도 구분합니다. | 검색 — NU54V 두 대<br>선택 후보 — 끝자리 71C4<br>등록 상태 — 미등록 | [페어링 코드 입력](../book.html#W04/pair)<br>[취소](../book.html#W02) | [BLE 접근 거부](../book.html#W04/ble) |
| [권한 필요](../book.html#W04/ble) | **주변 기기 접근을 허용하세요**<br>휴대폰 설정에서 Bluetooth 권한을 켠 뒤 검색을 다시 시작하세요. | 등록 진행 — 저장된 단계에서 재개 | [검색 다시 시작](../book.html#W04) | — |
| [등록 창](../book.html#W04/pair) | **기기 지문과 등록 코드를 대조하세요**<br>코드는 등록 세션용입니다. 지문이 다르면 연결을 취소하세요. | 입력 · 페어링 코드 — 427618<br>기기 지문 — 07B1 9E47 322F 71C4<br>유효 시간 — 01:30 | [코드 제출](../book.html#W04/pin)<br>[다른 기기 선택](../book.html#W04) | — |
| [보드 입력 대기](../book.html#W04/pin) | **보드에서 PIN을 설정하거나 잠금을 해제하세요**<br>버튼과 LED를 사용하세요. 앱에는 시작과 종료 결과만 표시됩니다. | 소유 세대 — 1<br>입력 상태 — 보드 응답 대기 | [중단](../book.html#W04) | [보드 결합과 PIN 완료](../book.html#W05)<br>[실패 한도 도달](../book.html#W06) |

## W05 · 백업과 첫 주소 검증

**사용자** 소유자　 **진입** 개인 보드 소유 결합 완료

**완료 기준** 독립 복원 시험과 기준 주소 일치 전에는 수신 주소 공유를 열지 않는다.

[P01.R4](../../products/p01-device-firmware/prd.md) · [P01.UC1](../../products/p01-device-firmware/use-cases.md) · [P02.R1](../../products/p02-user-app/prd.md) · [P02.UC1](../../products/p02-user-app/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [백업 확인](../book.html#W05/start) | **독립 복구 장치를 연결하세요**<br>보드 내부에서 루트를 만든 뒤 독립 장치에 표시된 복구 자료를 기록하세요. 별도 빈 보드에서 복원해 기준 주소를 대조합니다. | 활성 슬롯 — 개인 · 초기 설정<br>입금 주소 공유 — 잠김<br>휴대폰 입력 — 없음 | [설정으로](../book.html#W27) | [복원 시험·주소 대조 통과](../book.html#W05/ready)<br>[장치 미지원·기준 불일치](../book.html#W05/blocked) |
| [복원 확인](../book.html#W05/ready) | **첫 주소를 확인했습니다**<br>검증한 계정에서 수신할 네트워크를 선택하세요. | 기준 계정 — 개인 계정 1<br>복원 대조 — 일치 | [첫 수신 주소](../book.html#W07)<br>[계정 선택](../book.html#W25) | — |
| [대조 실패](../book.html#W05/blocked) | **입금 안내가 잠겨 있습니다**<br>별도 보드의 임시 슬롯을 폐기하고 보관한 기준 주소와 파생 경로를 다시 대조하세요. | 기존 활성 슬롯 — 보존<br>대조 결과 — 미확인 | [다시 확인](../book.html#W05) | — |

## W06 · 분실·교체 복구

**사용자** 소유자　 **진입** 설정의 기기 복구

**완료 기준** 폰 교체는 주소를 보존하고 옛 세대 승인을 거절한다. 새 보드 복원은 기준 주소를 대조한다.

[P01.R4](../../products/p01-device-firmware/prd.md) · [P01.UC3](../../products/p01-device-firmware/use-cases.md) · [P02.R7](../../products/p02-user-app/prd.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [분실한 기기](../book.html#W06/start) | **어떤 기기를 교체하나요?**<br>보유한 기기와 복구 자료에 따라 필요한 증명이 달라집니다. | 휴대폰 분실 — 기존 보드와 독립 증명<br>보드 분실 — 새 보드와 검증된 백업 | [폰 교체](../book.html#W06/phone)<br>[보드 복원](../book.html#W06/board)<br>[클라우드 폰 복구](../book.html#W22) | — |
| [폰 교체](../book.html#W06/phone) | **새 폰의 공개키를 보드에서 대조하세요**<br>보드 PIN과 독립 증명 후 소유 세대와 대기 폐기를 한 번에 기록합니다. 중단 후 활성 세대를 조회합니다. | 현재 세대 — 4<br>새 폰 — 세대 5 후보<br>기존 본드 — 교체 완료 뒤 정리 | [복구 선택](../book.html#W06) | [물리 확인·교체 커밋](../book.html#W06/rebound)<br>[교체 직전·직후 전원 차단](../book.html#W06/interrupted) |
| [재결합 완료](../book.html#W06/rebound) | **새 폰만 승인 요청을 보낼 수 있습니다**<br>기존 주소를 대조하고 미결 지급을 다시 조회하세요. | 활성 세대 — 5<br>옛 요청 — 폐기<br>지갑 주소 — 교체 전과 일치 | [내 지갑](../book.html#W01)<br>[미결 조회](../book.html#W09) | [구 BLE 본드 정리 실패](../book.html#W06/bond) |
| [새 보드 복원](../book.html#W06/board) | **빈 보드의 독립 장치에 백업을 입력하세요**<br>보관한 주소와 파생 계정을 비교합니다. 도난 보드는 같은 키로 서명할 수 있어 새 루트 이전과 권한 회수를 검토하세요. | 복원 대상 — 자산 키<br>별도 복구 — 앱 세션 · 구독 · 예약 | [계정 재발견](../book.html#W25/discover)<br>[권한 조회](../book.html#W15) | [기준 불일치](../book.html#W05/blocked)<br>[새 보드에서 대조·물리 확인 완료](../book.html#W04) |
| [교체 중단](../book.html#W06/interrupted) | **재부팅한 보드의 소유 세대를 조회하세요**<br>저장된 활성 세대 하나를 읽기 전 새 승인을 시작하지 않습니다. | 이전 — 세대 4<br>후보 — 세대 5<br>대기 승인 — 사용 차단 | [활성 세대 조회](../book.html#W06/interrupted) | [커밋 전 · 4만 활성](../book.html#W06/phone)<br>[커밋 후 · 5만 활성](../book.html#W06/rebound) |
| [본드 정리 대기](../book.html#W06/bond) | **구 휴대폰 연결 기록을 정리하고 있습니다**<br>구 본드가 남아도 서비스 권한의 세대 검사는 이전 요청을 거절합니다. | 소유 세대 — 5<br>구세대 서명 — 0건<br>구 본드 — 정리 재시도 | [정리 상태 조회](../book.html#W06/bond) | [기존 본드 정리 완료](../book.html#W06/rebound) |

## W07 · 받기

**사용자** 소유자　 **진입** 지갑 홈에서 받기

**완료 기준** 검증한 망·계정과 같은 전체 주소에만 공유를 열고 태그가 필요한 망은 함께 표시한다.

[P01.R3](../../products/p01-device-firmware/prd.md) · [P01.UC1](../../products/p01-device-firmware/use-cases.md) · [P02.R2](../../products/p02-user-app/prd.md) · [P02.UC1](../../products/p02-user-app/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [수신 확인](../book.html#W07/start) | **개인 계정 1로 받기**<br>독립 표시 또는 지원된 확인 경로로 주소를 대조하세요. | 네트워크 — StableNet 테스트넷 8283<br>자산 — TestUSDC<br>전체 주소 — 0x7010…0142 (화면에서 합성값 전체 표시) | [네트워크 선택](../book.html#W26)<br>[다른 계정](../book.html#W25)<br>[체인별 수신 주소](../book.html#W07/choose) | [주소 대조 완료](../book.html#W07/share)<br>[백업 확인 누락](../book.html#W05/blocked) |
| [수신 주소](../book.html#W07/share) | **확인한 주소를 공유할 수 있습니다**<br>수신 QR에는 이 체인과 계정의 값을 사용합니다. | 전체 주소 — 0x7010…0142 (화면에서 합성값 전체 표시)<br>체인 — 8283<br>태그 — 사용하지 않음 | [QR·주소 공유 보기](../book.html#W07/qr)<br>[홈](../book.html#W01) | — |
| [공유 내용](../book.html#W07/qr) | **받는 사람에게 보낼 주소**<br>기본 공유 시트로 넘길 원문과 QR의 해독값을 대조합니다. | QR 해독값 — 0x7010…0142 (화면에서 합성값 전체 표시)<br>공유 항목 — StableNet 테스트넷 · 전체 주소 | [주소로 돌아가기](../book.html#W07/share) | — |
| [수신 네트워크](../book.html#W07/choose) | **받을 네트워크를 선택하세요**<br>지원 테스트망의 계정과 태그를 검증한 뒤 주소를 공유하세요. | 계정 — 테스트 계정 0<br>환경 — 테스트넷 전용 | [Bitcoin](../book.html#W07/btc)<br>[Ethereum](../book.html#W07/eth)<br>[Base](../book.html#W07/base)<br>[StableNet](../book.html#W07)<br>[Solana](../book.html#W07/sol)<br>[XRPL](../book.html#W07/xrp) | — |
| [주소 확인](../book.html#W07/btc) | **Bitcoin signet 받기**<br>선택한 계정의 전체 주소를 지원된 독립 확인 경로에서 대조하세요. | 체인 — Bitcoin signet<br>전체 주소 — tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx<br>수신 조건 — Native SegWit · 외부 주소 0 | [망 변경](../book.html#W07/choose) | [보드 주소 대조 완료](../book.html#W07/btcshare)<br>[주소 대조 실패](../book.html#W05/blocked) |
| [주소 공유](../book.html#W07/btcshare) | **Bitcoin signet 확인된 주소**<br>수신 QR과 시스템 공유 시트에서 전체 주소·네트워크 정보를 확인하세요. 태그가 있으면 함께 전달하세요. | 공유 주소 — tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx<br>네트워크 — Bitcoin signet<br>수신 조건 — Native SegWit · 외부 주소 0 | [다른 체인 받기](../book.html#W07/choose)<br>[지갑](../book.html#W01) | — |
| [주소 확인](../book.html#W07/eth) | **Ethereum Sepolia 받기**<br>선택한 계정의 전체 주소를 지원된 독립 확인 경로에서 대조하세요. | 체인 — Ethereum Sepolia<br>전체 주소 — 0x7010…0142 (화면에서 합성값 전체 표시)<br>수신 조건 — ETH · chainId 11155111 | [망 변경](../book.html#W07/choose) | [보드 주소 대조 완료](../book.html#W07/ethshare)<br>[주소 대조 실패](../book.html#W05/blocked) |
| [주소 공유](../book.html#W07/ethshare) | **Ethereum Sepolia 확인된 주소**<br>수신 QR과 시스템 공유 시트에서 전체 주소·네트워크 정보를 확인하세요. 태그가 있으면 함께 전달하세요. | 공유 주소 — 0x7010…0142 (화면에서 합성값 전체 표시)<br>네트워크 — Ethereum Sepolia<br>수신 조건 — ETH · chainId 11155111 | [다른 체인 받기](../book.html#W07/choose)<br>[지갑](../book.html#W01) | — |
| [주소 확인](../book.html#W07/base) | **Base Sepolia 받기**<br>선택한 계정의 전체 주소를 지원된 독립 확인 경로에서 대조하세요. | 체인 — Base Sepolia<br>전체 주소 — 0x7010…0142 (화면에서 합성값 전체 표시)<br>수신 조건 — 테스트 ETH · chainId 84532 | [망 변경](../book.html#W07/choose) | [보드 주소 대조 완료](../book.html#W07/baseshare)<br>[주소 대조 실패](../book.html#W05/blocked) |
| [주소 공유](../book.html#W07/baseshare) | **Base Sepolia 확인된 주소**<br>수신 QR과 시스템 공유 시트에서 전체 주소·네트워크 정보를 확인하세요. 태그가 있으면 함께 전달하세요. | 공유 주소 — 0x7010…0142 (화면에서 합성값 전체 표시)<br>네트워크 — Base Sepolia<br>수신 조건 — 테스트 ETH · chainId 84532 | [다른 체인 받기](../book.html#W07/choose)<br>[지갑](../book.html#W01) | — |
| [주소 확인](../book.html#W07/sol) | **Solana devnet 받기**<br>선택한 계정의 전체 주소를 지원된 독립 확인 경로에서 대조하세요. | 체인 — Solana devnet<br>전체 주소 — 7YttLkHDoNj9wyDur5B3s7MQ33Z6qe1VYMw73hq4wNxe<br>수신 조건 — SOL · 시스템 계정 | [망 변경](../book.html#W07/choose) | [보드 주소 대조 완료](../book.html#W07/solshare)<br>[주소 대조 실패](../book.html#W05/blocked) |
| [주소 공유](../book.html#W07/solshare) | **Solana devnet 확인된 주소**<br>수신 QR과 시스템 공유 시트에서 전체 주소·네트워크 정보를 확인하세요. 태그가 있으면 함께 전달하세요. | 공유 주소 — 7YttLkHDoNj9wyDur5B3s7MQ33Z6qe1VYMw73hq4wNxe<br>네트워크 — Solana devnet<br>수신 조건 — SOL · 시스템 계정 | [다른 체인 받기](../book.html#W07/choose)<br>[지갑](../book.html#W01) | — |
| [주소 확인](../book.html#W07/xrp) | **XRPL Testnet 받기**<br>선택한 계정의 전체 주소를 지원된 독립 확인 경로에서 대조하세요. | 체인 — XRPL Testnet<br>전체 주소 — rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh<br>수신 조건 — XRP · DestinationTag 142 | [망 변경](../book.html#W07/choose) | [보드 주소 대조 완료](../book.html#W07/xrpshare)<br>[주소 대조 실패](../book.html#W05/blocked) |
| [주소 공유](../book.html#W07/xrpshare) | **XRPL Testnet 확인된 주소**<br>수신 QR과 시스템 공유 시트에서 전체 주소·네트워크 정보를 확인하세요. 태그가 있으면 함께 전달하세요. | 공유 주소 — rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh<br>DestinationTag — 142<br>네트워크 — XRPL Testnet<br>공유 묶음 — 주소 + DestinationTag 142 + XRPL Testnet | [주소·태그 공유 카드](../book.html#W07/xrpcard)<br>[다른 체인 받기](../book.html#W07/choose)<br>[지갑](../book.html#W01) | — |
| [태그를 포함한 공유 카드](../book.html#W07/xrpcard) | **주소와 태그를 함께 보내세요**<br>공유 카드의 QR과 텍스트에는 같은 세 항목이 들어갑니다. 상대방은 지갑에서 네트워크·주소·태그를 각각 확인합니다. | 네트워크 — XRPL Testnet<br>주소 — rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh<br>DestinationTag — 142 | [시스템 공유 열기](../book.html#W07/xrpsheet)<br>[받기 상세](../book.html#W07/xrpshare) | — |
| [시스템 공유](../book.html#W07/xrpsheet) | **보낼 채널을 선택하세요**<br>운영체제 공유 시트에 주소와 태그를 함께 전달합니다. 취소하면 같은 수신 카드로 돌아옵니다. | 공유 내용 — XRPL Testnet · 수신 주소 · DestinationTag 142<br>지갑 상태 — 조회 전용 수신 정보 | [공유 취소](../book.html#W07/xrpcard) | [운영체제가 공유 시트를 닫음](../book.html#W07/xrpshare) |

## W08 · 보내기와 거래 추적

**사용자** 소유자　 **진입** 선택 계정에서 보내기

**완료 기준** 원문 변경·기한 경과·분리 후 서명은 0건이며 기존 거래를 식별자로 재조회한다.

[P01.R2](../../products/p01-device-firmware/prd.md) · [P01.R3](../../products/p01-device-firmware/prd.md) · [P01.UC2](../../products/p01-device-firmware/use-cases.md) · [P02.R2](../../products/p02-user-app/prd.md) · [P02.R7](../../products/p02-user-app/prd.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [송금 입력](../book.html#W08/start) | **보낼 금액과 주소를 입력하세요**<br>주소록의 검증된 주소를 선택하거나 전체 주소를 입력하세요. 선택 체인의 수수료 잔액도 검사합니다. | 입력 · 받는 주소 — 0x8020…0315 (화면에서 합성값 전체 표시)<br>입력 · 금액 — 3.00 TestUSDC<br>출금 — 개인 계정 1 · StableNet 테스트넷<br>잔액 — 42.00 TestUSDC | [검토 요청](../book.html#W08/review)<br>[주소록](../book.html#W25/contacts)<br>[취소](../book.html#W01) | [주소·태그·수수료 잔액 오류](../book.html#W08/error) |
| [보드 원문 요약](../book.html#W08/review) | **3.00 TestUSDC 보내기**<br>임의 수취인은 독립 표시에서 전체 주소를 대조한 뒤 보드 버튼으로 확인합니다. 수정하면 기존 대기를 폐기합니다. | 수취인 — 0x8020…0315 (화면에서 합성값 전체 표시)<br>체인 — StableNet 테스트넷 8283<br>최대 수수료 — 0.002 네이티브 가스<br>원문 — SEND-315 · 14:26 만료 | [내용 수정](../book.html#W08)<br>[요청 취소](../book.html#W08/error) | [검증된 서명 제출](../book.html#W08/pending)<br>[기기 거절·분리·표시 미지원](../book.html#W08/error) |
| [제출 조회](../book.html#W08/pending) | **SEND-315의 결과를 확인 중입니다**<br>앱 재실행 후 같은 해시를 조회합니다. | 지급 상태 — 미확정<br>서명 수 — 1 | [기존 거래 재조회](../book.html#W08/pending)<br>[홈](../book.html#W01) | [해당 거래 최종성 확인](../book.html#W08/done) |
| [전송 영수증](../book.html#W08/done) | **3.00 TestUSDC 전송 완료**<br>체인에서 확인한 지급 내역입니다. | 거래 — SEND-315<br>실제 수취인 — 0x8020…0315 (화면에서 합성값 전체 표시)<br>체인 — 8283 | [지갑으로](../book.html#W01) | — |
| [진행 중단](../book.html#W08/error) | **송금을 진행할 수 없습니다**<br>입력 오류이면 주소·태그·가스 잔액을 수정하세요. 만료·분리·취소 시 새 보드 확인이 필요합니다. | 현재 요청 — 종료<br>제출 — 0건 | [입력으로](../book.html#W08)<br>[홈](../book.html#W01) | — |

## W09 · 결제와 미결 목록

**사용자** 소유자　 **진입** 하단 결제 또는 알림 열기

**완료 기준** 푸시 없이도 원 주문을 찾고 딥링크 권한과 만료를 재검사한다.

[P02.R5](../../products/p02-user-app/prd.md) · [P02.UC3](../../products/p02-user-app/use-cases.md) · [P02.R7](../../products/p02-user-app/prd.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [결제 목록](../book.html#W09/start) | **내 결제와 미결 청구**<br>각 항목은 현재 사용자에게 조회권이 있는 주문입니다. | 오늘 · 매장 — S-0142 · 6.50 TestUSDC<br>이번 달 · 구독 — INV-07 · 4.00 TestUSDC | [매장 주문 조회](../book.html#W11)<br>[구독 회차](../book.html#W19)<br>[구독 관리](../book.html#W18)<br>[여행 예약](../book.html#T05)<br>[지갑](../book.html#W01) | [소유권 없는 딥링크](../book.html#W09/denied) |
| [접근 제한](../book.html#W09/denied) | **이 계정에서 열 수 없는 주문입니다**<br>계정이나 주문 내용을 알림 미리보기에 표시하지 않습니다. | 표시 가능 항목 — 0개 | [내 결제로](../book.html#W09) | — |

## W10 · 매장 결제 확인

**사용자** 소유자　 **진입** 결제 목록 또는 가맹점 요청 S-0142

**완료 기준** 보드가 계산한 6.50 TestUSDC와 S-0142가 같을 때만 물리 승인을 기다린다.

[P01.R2](../../products/p01-device-firmware/prd.md) · [P01.UC2](../../products/p01-device-firmware/use-cases.md) · [P02.R7](../../products/p02-user-app/prd.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [주문 확인](../book.html#W10/start) | **6.50 TestUSDC**<br>가맹점 서명 견적을 검증한 보드의 요약입니다. 매장과 금액이 맞으면 보드 SW1을 누르세요. SW2로 거절할 수 있습니다. | 요청 출처 — 인증한 가맹점 견적 · 보드 검증<br>주문 — S-0142<br>가맹점 — 여울 커피 성수점<br>네트워크 — StableNet 테스트넷 8283<br>결제 원천 — 대여 지갑 예치금<br>가스 비용 — 매장 부담<br>수령 주소 — 0x62c9…8e41 | [견적 상세](../book.html#W10/details)<br>[거절](../book.html#W10/cancel) | [보드 승인·릴레이 제출](../book.html#W11)<br>[만료 또는 기기 분리](../book.html#W10/cancel) |
| [원문 상세](../book.html#W10/details) | **S-0142 견적 상세**<br>원문이 바뀌면 다시 확인해야 합니다. | 수취 주소 — 0x62c9…8e41 (화면에서 합성값 전체 표시)<br>금액 — 6.50 TestUSDC<br>수수료 부담 — 매장 부담<br>체인 ID — 8283 | [확인으로](../book.html#W10)<br>[거절](../book.html#W10/cancel) | — |
| [승인 종료](../book.html#W10/cancel) | **S-0142 승인 대기가 끝났습니다**<br>기존 주문 상세에서 이미 제출된 결과가 있는지 확인하세요. | 이 승인 — 폐기<br>자동 재승인 — 없음 | [결제 목록](../book.html#W09) | — |
| [원주문 확인](../book.html#W10/bleReview) | **8.00 TestUSDC · S-0151**<br>여울 커피 성수점의 서명 견적입니다. 원 주문과 변경 가격을 대조하고 보드 SW1으로 확인하거나 SW2로 거절하세요. | 요청 출처 — 인증한 가맹점 견적 · 보드 검증<br>가맹점 — 여울 커피 성수점<br>주문 — S-0151<br>품목 — 아이스 아메리카노 3.50 + 라떼 4.50<br>체인 — StableNet 테스트넷 8283<br>수령 주소 — 0x62c9…8e41 (화면에서 합성값 전체 표시)<br>가스 비용 — 매장 부담 | [내 결제](../book.html#W09) | [해당 견적의 보드 승인·제출](../book.html#W11/blePending)<br>[보드 거절·만료](../book.html#W10/bleReviewclosed) |
| [승인 종료](../book.html#W10/bleReviewclosed) | **S-0151 승인을 종료했습니다**<br>원 견적의 상태를 조회합니다. | 주문 — S-0151<br>이 기기의 승인 — 거절 또는 만료 | [기존 주문 조회](../book.html#W11/blePending)<br>[결제 목록](../book.html#W09) | — |
| [원주문 확인](../book.html#W10/repriced) | **7.00 TestUSDC · S-0144**<br>여울 커피 성수점의 서명 견적입니다. 원 주문과 변경 가격을 대조하고 보드 SW1으로 확인하거나 SW2로 거절하세요. | 요청 출처 — 인증한 가맹점 견적 · 보드 검증<br>가맹점 — 여울 커피 성수점<br>주문 — S-0144<br>품목 — 아이스 아메리카노 3.50 + 크루아상 3.50<br>체인 — StableNet 테스트넷 8283<br>수령 주소 — 0x62c9…8e41 (화면에서 합성값 전체 표시)<br>가스 비용 — 매장 부담 | [내 결제](../book.html#W09) | [해당 견적의 보드 승인·제출](../book.html#W11/repricedPending)<br>[보드 거절·만료](../book.html#W10/repricedclosed) |
| [승인 종료](../book.html#W10/repricedclosed) | **S-0144 승인을 종료했습니다**<br>원 견적의 상태를 조회합니다. | 주문 — S-0144<br>이 기기의 승인 — 거절 또는 만료 | [기존 주문 조회](../book.html#W11/repricedPending)<br>[결제 목록](../book.html#W09) | — |

## W11 · 매장 주문 영수증

**사용자** 소유자　 **진입** S-0142 상세 또는 결제 후 재진입

**완료 기준** 서명·제출·확정을 분리하고 앱 재시작 후 같은 주문의 영수증을 조회한다.

[P02.R2](../../products/p02-user-app/prd.md) · [P02.R7](../../products/p02-user-app/prd.md) · [P01.UC2](../../products/p01-device-firmware/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [정산 조회](../book.html#W11/start) | **S-0142 · 원주문 상태 조회**<br>릴레이가 응답하지 않아도 같은 주문을 조회합니다. | 지급액 — 6.50 TestUSDC<br>상태 — 결합 견적 · 기존 거래 조회 중<br>가맹점 — 여울 커피 성수점 | [다시 조회](../book.html#W11)<br>[내 결제](../book.html#W09) | [체인 확정과 주문 대사 완료](../book.html#W11/paid)<br>[조회 지연](../book.html#W11/unknown)<br>[미제출·유효 견적과 단일 지급자 결합 확인](../book.html#W10) |
| [결과 미확인](../book.html#W11/unknown) | **네트워크 응답을 기다립니다**<br>추가 주문을 만들지 않고 영수증 식별자를 유지합니다. | 조회 키 — S-0142<br>추가 청구 — 0건 | [같은 주문 조회](../book.html#W11) | — |
| [결제 영수증](../book.html#W11/paid) | **6.50 TestUSDC 결제 완료**<br>주문 원장과 체인 지급 결과가 일치합니다. | 주문 — S-0142<br>네트워크 — StableNet 테스트넷 8283<br>확정 지급 — 1건<br>확정 시각 — 10:42<br>픽업 번호 — A17 · 준비 중 | [내 결제](../book.html#W09)<br>[홈](../book.html#W01) | [매장 준비 완료 통지](../book.html#W11/pickup) |
| [픽업 준비 완료](../book.html#W11/pickup) | **A17 주문을 찾아가세요**<br>여울 커피 성수점 픽업대에서 번호를 보여주세요. | 픽업 번호 — A17<br>원주문 — S-0142<br>지급 — 6.50 TestUSDC · 10:42 확정<br>준비 — 완료 | [내 결제](../book.html#W09) | [직원 전달 완료 기록](../book.html#W11/collected) |
| [픽업 완료](../book.html#W11/collected) | **A17을 수령했습니다**<br>기존 주문 영수증을 보관합니다. | 원주문 — S-0142<br>영수증 — R-0142<br>지급 — 6.50 TestUSDC · 확정 | [내 결제](../book.html#W09) | — |
| [원거래 조회](../book.html#W11/blePending) | **S-0151 지급 결과 확인 중**<br>이 주문의 결합 견적과 네트워크 결과를 조회합니다. 중복 제출은 차단됩니다. | 주문 — S-0151<br>결제액 — 8.00 TestUSDC<br>지점 — 여울 커피 성수점 | [같은 주문 조회](../book.html#W11/blePending)<br>[내 결제](../book.html#W09) | [원 주문 확정·장부 대사 완료](../book.html#W11/blePaid)<br>[최종 미지급 증거 확인](../book.html#W11/blePendingclosed) |
| [미지급 종료](../book.html#W11/blePendingclosed) | **S-0151 미지급 확인**<br>원 견적 미소비와 유효 범위를 확인했습니다. | 주문 — S-0151<br>지급 — 0건 | [내 결제](../book.html#W09) | — |
| [픽업 영수증](../book.html#W11/blePaid) | **A16 · 8.00 TestUSDC 지급 완료**<br>본인 주문의 지급 영수증과 픽업 번호입니다. | 주문 — S-0151<br>픽업 번호 — A16<br>지급 — 8.00 TestUSDC<br>이행 — 11월 19일 · 10:40 지급 확정 · 10:41 픽업 가능 | [결제 목록](../book.html#W09) | — |
| [원거래 조회](../book.html#W11/repricedPending) | **S-0144 지급 결과 확인 중**<br>이 주문의 결합 견적과 네트워크 결과를 조회합니다. 중복 제출은 차단됩니다. | 주문 — S-0144<br>결제액 — 7.00 TestUSDC<br>지점 — 여울 커피 성수점 | [같은 주문 조회](../book.html#W11/repricedPending)<br>[내 결제](../book.html#W09) | [원 주문 확정·장부 대사 완료](../book.html#W11/repricedPaid)<br>[최종 미지급 증거 확인](../book.html#W11/repricedPendingclosed) |
| [미지급 종료](../book.html#W11/repricedPendingclosed) | **S-0144 미지급 확인**<br>원 견적 미소비와 유효 범위를 확인했습니다. | 주문 — S-0144<br>지급 — 0건 | [내 결제](../book.html#W09) | — |
| [픽업 영수증](../book.html#W11/repricedPaid) | **A18 · 7.00 TestUSDC 지급 완료**<br>본인 주문의 지급 영수증과 픽업 번호입니다. | 주문 — S-0144<br>픽업 번호 — A18<br>지급 — 7.00 TestUSDC<br>이행 — 준비 중 | [결제 목록](../book.html#W09) | — |

## W12 · 기기 유지관리와 반납

**사용자** 소유자　 **진입** 설정에서 기기 선택

**완료 기준** 대여 반납과 개인 초기화의 슬롯·권한을 분리하며 승인 진행 중 업데이트는 차단한다.

[P01.R1](../../products/p01-device-firmware/prd.md) · [P01.R5](../../products/p01-device-firmware/prd.md) · [P01.UC3](../../products/p01-device-firmware/use-cases.md) · [P02.R9](../../products/p02-user-app/prd.md) · [P02.UC6](../../products/p02-user-app/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [내 보드](../book.html#W12/start) | **기기 상태와 소유권 관리**<br>리비전별 기능표로 지원 기능을 선택하세요. | 프로필 — 개인 공식<br>기능 — 인증된 리비전의 기능표에서 확인<br>버튼 사용 — 서명 세션 없음 | [펌웨어 업데이트](../book.html#W12/update)<br>[개인 초기화](../book.html#W12/erase)<br>[신뢰 루트 변경](../book.html#W12/trust)<br>[기기 부가기능](../book.html#W12/tools)<br>[대여 기기 보기](../book.html#W12/rental) | — |
| [업데이트 준비](../book.html#W12/update) | **전원과 승인 세션을 확인하세요**<br>녹음·전송을 안전 종료한 다음 물리 확인을 받습니다. 중단 시 실제 실행 버전을 조회하세요. | 현재 버전 — 예시 2.0<br>후보 — 예시 2.1 · 서명 검사 대기<br>검사 — 전원 · 모델 · 최소 허용 버전 | [기기 상태](../book.html#W12) | [서명 작업·잘못된 이미지](../book.html#W12/busy)<br>[보드 적용·재부팅 후 버전 일치](../book.html#W12/updated) |
| [버전 확인](../book.html#W12/updated) | **새 실행 슬롯이 확인되었습니다**<br>재부팅 후 실행 이미지를 다시 읽었습니다. | 버전 — 2.1<br>부트 검증 — 통과 | [기기 상태](../book.html#W12) | — |
| [작업 충돌](../book.html#W12/busy) | **기존 작업이 끝날 때까지 기다리세요**<br>승인과 패스키는 버튼을 동시에 점유할 수 없습니다. 업데이트는 실행 슬롯을 확인하고 재개합니다. | 작업 — 기기 승인 진행<br>업데이트 적용 — 보류 | [상태 재조회](../book.html#W12) | — |
| [부가기능](../book.html#W12/tools) | **지원 기능을 선택하세요**<br>등록 기기의 기능을 조회해 찾기 신호를 요청하거나 취소합니다. 상태 안내는 휴대전화에서 읽습니다. | 찾기 — 인증된 개인 보드의 LED<br>보드 오디오 전송 — 이 리비전에서 미지원<br>휴대전화 — 음성으로 상태 듣기<br>TimeAnchor · 패스키 — 대여 프로필에서만 노출 | [찾기 신호 요청](../book.html#W12/find)<br>[오디오 지원 리비전의 전송 상태 조회](../book.html#W12/audio) | — |
| [기기 찾기](../book.html#W12/find) | **보드의 찾기 신호를 확인하세요**<br>보드가 요청을 접수하면 LED 신호를 보냅니다. 찾기를 취소해 중단할 수 있습니다. | 대상 — 지문 끝 71C4<br>결과 — LED 응답 대기 | [기기 기능](../book.html#W12/tools) | — |
| [오디오 진행](../book.html#W12/audio) | **중단한 전송을 조회합니다**<br>오디오 전송을 지원하는 보드에서, 휴대전화 저장이 확인된 마지막 구간부터 이어받습니다. | 완료 — 12 / 20<br>버퍼 — 재개 가능 | [기능으로](../book.html#W12/tools) | — |
| [대여 슬롯](../book.html#W12/rental) | **대여 보드 반납 상태**<br>인증된 운영자가 대여 정산 확인 후 반납을 처리합니다. 개인 슬롯은 접근 범위에서 제외됩니다. | 지급 — 미결 1건<br>대여 슬롯 — 열림<br>대여 패스키 사용기한 — 보드 또는 발급기관의 검증된 기한을 표시합니다. 증거가 없으면 확인 불가로 표시하고 담당자에게 해당 기기·대여 세대의 기한을 확인하도록 안내합니다. | [미결 지급 조회](../book.html#W09)<br>[프로필 선택](../book.html#W02) | [정산·인증된 대여 종료 수신](../book.html#W12/returned) |
| [반납 완료](../book.html#W12/returned) | **대여 권한이 종료되었습니다**<br>서명된 대여 슬롯 종료 증거를 서버가 확인한 뒤 회수 기록에 결과를 남깁니다. | 대여 슬롯 — 닫힘<br>개인 영역 — 변경 0건 | [프로필 선택](../book.html#W02) | — |
| [개인 초기화](../book.html#W12/erase) | **개인 지갑 삭제 범위를 확인하세요**<br>잔액·미결 지급·복구 시험을 확인한 뒤 실물에서 별도 확인해야 합니다. | 삭제 — 개인 키 슬롯 · 본드<br>체인 자산 — 주소에 남음<br>복구 수단 — 확인 필요 | [백업 확인](../book.html#W05)<br>[미결 주문](../book.html#W09)<br>[취소](../book.html#W12) | [백업 미확인 또는 미결 지급 발견](../book.html#W12/eraseblocked)<br>[보드가 삭제 조건 충족·별도 물리 확인 수신](../book.html#W12/erasing) |
| [배포 신뢰 루트](../book.html#W12/trust) | **자가 서명 배포로 전환하기**<br>독립 복구 수단과 데이터 삭제 범위를 대조한 뒤 보드에서 물리 확인하세요. | 현재 — 공식<br>전환 후 — 사용자 서명 · 커스텀 표기<br>데이터 — 개인 슬롯 삭제 필요 | [취소](../book.html#W12)<br>[복구 수단 확인](../book.html#W05) | [물리 전환·삭제 완료](../book.html#W03) |
| [삭제 보류](../book.html#W12/eraseblocked) | **개인 슬롯 삭제를 시작할 수 없습니다**<br>독립 복구 시험을 확인하고 미결 지급을 원주문에서 대사하세요. | 백업 — 복원 검사 미확인<br>미결 지급 — 1건<br>개인 슬롯 — 활성 유지 | [백업 시험](../book.html#W05)<br>[미결 지급](../book.html#W09)<br>[취소](../book.html#W12) | — |
| [삭제 상태 조회](../book.html#W12/erasing) | **개인 슬롯 상태를 다시 읽습니다**<br>보드의 재부팅 결과를 조회해 삭제된 슬롯과 폐기된 본드를 확인합니다. | 조회 대상 — 개인 슬롯<br>상태 — 커밋 확인 중 | [같은 보드 상태 조회](../book.html#W12/erasing) | [개인 슬롯 삭제·본드 해제 확인](../book.html#W02)<br>[중단 후 활성 슬롯 보존](../book.html#W12/eraseblocked) |

## W13 · 둘러보기와 DApp 연결

**사용자** 소유자　 **진입** 둘러보기 탭 또는 WalletConnect 제안

**완료 기준** 출처·계정·체인·메서드를 동의한 세션만 연결하고 서명은 0건이다.

[P02.R4](../../products/p02-user-app/prd.md) · [P02.UC2](../../products/p02-user-app/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [둘러보기](../book.html#W13/start) | **사이트와 서비스를 선택하세요**<br>접속 전에 출처를 확인합니다. 외부 지갑 연결 QR도 같은 권한 화면에서 검토합니다. | 입력 · 사이트 주소 — https://demo.example<br>브라우저 — https://demo.example<br>외부 연결 — WalletConnect | [사이트 연결](../book.html#W13/connect)<br>[연결 관리](../book.html#W15)<br>[여행 지도](../book.html#T01)<br>[학습 시장](../book.html#L01) | — |
| [접속 범위](../book.html#W13/connect) | **demo.example에 계정을 공개할까요?**<br>로그인과 지출 요청은 각각 다시 검토합니다. 허용할 범위만 선택하세요. | 계정 — 개인 계정 1<br>체인 — Ethereum Sepolia<br>요청 메서드 — 로그인 메시지 · 검토 가능한 거래<br>만료 — 오늘 15:00 KST | [선택 범위 연결](../book.html#W13/connected)<br>[거절](../book.html#W13) | — |
| [연결됨](../book.html#W13/connected) | **사이트가 계정 주소를 읽을 수 있습니다**<br>새 요청이 도착하면 원문을 별도 화면에서 확인하세요. | 서명 건수 — 0<br>연결 — demo.example | [세션 관리](../book.html#W15) | [로그인 메시지 수신](../book.html#W14)<br>[토큰 권한 요청 수신](../book.html#W14/spend)<br>[출처 변경·iframe·만료](../book.html#W13/expired) |
| [다시 동의 필요](../book.html#W13/expired) | **기존 연결로 요청할 수 없습니다**<br>새 출처와 요청 범위를 검토하세요. | 이전 세션 — 무효<br>서명 — 0건 | [새 연결 검토](../book.html#W13/connect)<br>[종료](../book.html#W13) | — |

## W14 · DApp 로그인과 지출 요청

**사용자** 소유자　 **진입** 허용된 사이트의 새 메시지

**완료 기준** 로그인 nonce 서명과 토큰 지출 승인을 분리하고 해석할 수 없는 원문은 거절한다.

[P01.R2](../../products/p01-device-firmware/prd.md) · [P01.UC2](../../products/p01-device-firmware/use-cases.md) · [P02.R4](../../products/p02-user-app/prd.md) · [P02.UC2](../../products/p02-user-app/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [로그인 메시지](../book.html#W14/start) | **demo.example에 로그인할까요?**<br>보드에서 검증된 로그인 메시지에만 한 번 서명합니다. 거래를 제출하지 않습니다. | SIWE 도메인 — demo.example<br>계정 — 개인 계정 1 · Sepolia<br>nonce — 8abc0023<br>기한 — 14:32 KST | [거절](../book.html#W13/connected)<br>[세션 보기](../book.html#W15) | [보드 단회 승인](../book.html#W14/login)<br>[도메인·nonce 불일치](../book.html#W14/deny) |
| [로그인 결과](../book.html#W14/login) | **사이트 로그인 서명을 반환했습니다**<br>다음 지출 요청은 별도로 확인합니다. | 메시지 — 8abc0023<br>온체인 지출 — 0건 | [연결 목록](../book.html#W15) | [새 거래 요청](../book.html#W14/spend) |
| [토큰 권한](../book.html#W14/spend) | **5.00 DEMO 사용 권한 요청**<br>독립 표시에서 spender·토큰·한도를 대조한 뒤 보드에서 확인하세요. | 체인 — Ethereum Sepolia<br>토큰 — 0x4020…0001 (화면에서 합성값 전체 표시)<br>spender — 0x5020…0002 (화면에서 합성값 전체 표시)<br>한도 — 5.00 DEMO<br>최대 가스비 — 0.0002 ETH | [거절](../book.html#W14/deny)<br>[연결 보기](../book.html#W15) | [보드 승인·제출](../book.html#W15/tracking)<br>[미해석 호출·신뢰 표시 없음](../book.html#W14/deny) |
| [요청 거절](../book.html#W14/deny) | **이번 요청을 종료했습니다**<br>다른 메서드나 새 출처는 별도의 요청으로 시작해야 합니다. | 이번 요청 서명 — 0건 | [사이트로](../book.html#W13/connected)<br>[세션 해제](../book.html#W15) | — |

## W15 · 세션과 온체인 권한

**사용자** 소유자　 **진입** 둘러보기 연결 목록 또는 계정 권한

**완료 기준** 세션 해제 후 기존 allowance를 보존하고 철회 거래 확정 후에만 0으로 표시한다.

[P01.R6](../../products/p01-device-firmware/prd.md) · [P01.UC4](../../products/p01-device-firmware/use-cases.md) · [P02.R4](../../products/p02-user-app/prd.md) · [P02.UC2](../../products/p02-user-app/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [연결 목록](../book.html#W15/start) | **연결한 사이트 1곳**<br>사이트 접속 권한과 토큰 계약에 기록된 한도를 따로 조회합니다. | 세션 — demo.example · Sepolia<br>계정 — 개인 계정 1<br>온체인 한도 — 5.00 DEMO | [세션 해제](../book.html#W15/off)<br>[전체 연결 해제](../book.html#W15/off)<br>[온체인 철회 검토](../book.html#W15/revoke) | — |
| [세션 종료](../book.html#W15/off) | **사이트 연결을 끊었습니다**<br>새 요청은 거절됩니다. 토큰 계약의 잔여 권한을 확인하세요. | 활성 세션 — 0<br>온체인 한도 — 5.00 DEMO | [철회 검토](../book.html#W15/revoke)<br>[둘러보기](../book.html#W13) | — |
| [철회 검토](../book.html#W15/revoke) | **DEMO 한도를 0으로 변경**<br>독립 표시와 보드에서 별도 철회 거래를 확인하세요. | 체인 — Ethereum Sepolia<br>spender — 0x5020…0002 (화면에서 합성값 전체 표시)<br>기존 → 새 한도 — 5.00 → 0 DEMO<br>최대 가스비 — 0.0002 ETH | [취소](../book.html#W15/off) | [보드 서명·철회 제출](../book.html#W15/revoking) |
| [철회 제출](../book.html#W15/revoking) | **철회 거래 확정 대기**<br>원 거래의 체인 확정을 조회합니다. | 현재 확정 한도 — 5.00 DEMO<br>요청 한도 — 0 DEMO | [같은 거래 조회](../book.html#W15/revoking) | [철회 확정·한도 재조회](../book.html#W15/zero) |
| [철회 완료](../book.html#W15/zero) | **남은 권한 0 DEMO**<br>철회 거래 확정 후 읽은 한도입니다. | 체인 — Sepolia<br>spender — 끝자리 0002 | [둘러보기](../book.html#W13) | — |
| [권한 부여 제출](../book.html#W15/tracking) | **권한 변경을 조회하고 있습니다**<br>확정 전에는 새 한도를 사용 가능으로 표시하지 않습니다. | 요청 — 5.00 DEMO<br>체인 — Sepolia | [기존 거래 조회](../book.html#W15/tracking) | [권한 부여 확정](../book.html#W15) |

## W16 · 공개정보 내보내기

**사용자** 소유자　 **진입** 계정에서 읽기 전용 내보내기

**완료 기준** 선택한 계정의 범위를 별도 동의하고 읽기 전용 프로필에는 서명 진입점이 없다.

[P01.R6](../../products/p01-device-firmware/prd.md) · [P01.UC4](../../products/p01-device-firmware/use-cases.md) · [P02.R8](../../products/p02-user-app/prd.md) · [P02.UC5](../../products/p02-user-app/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [공개 범위](../book.html#W16/start) | **Bitcoin 계정 0의 descriptor 공유**<br>공유받은 사람은 이 계정의 과거·미래 주소를 서로 연결할 수 있습니다. 내보낼 범위를 확인하세요. | 네트워크 — Bitcoin signet<br>주소 유형 — Native SegWit<br>범위 — 계정 0 · 외부/거스름돈<br>공유 형식 — 읽기 전용 descriptor | [공개 범위 동의](../book.html#W16/wait)<br>[취소](../book.html#W25) | — |
| [기기 확인](../book.html#W16/wait) | **보드에서 공개 범위를 확인하세요**<br>거절하면 공개정보를 전달하지 않습니다. | 대상 — 계정 0<br>상태 — 보드 확인 대기 | [취소](../book.html#W25) | [보드 승인](../book.html#W16/export)<br>[기기 거절](../book.html#W25) |
| [읽기 전용](../book.html#W16/export) | **계정 0의 공개정보가 준비됐습니다**<br>공유 범위와 가져온 지갑의 읽기 전용 표시를 대조하세요. | 외부·거스름돈 — 계정 0<br>서명 기능 — 없음 | [계정 관리](../book.html#W25) | — |

## W17 · Bitcoin PSBT 공동서명

**사용자** 소유자　 **진입** Bitcoin 계정에서 PSBT 가져오기

**완료 기준** 등록 서명자와 자기 change를 검증하고 미등록 descriptor·출력 위장은 거절한다.

[P01.R3](../../products/p01-device-firmware/prd.md) · [P01.R6](../../products/p01-device-firmware/prd.md) · [P01.UC4](../../products/p01-device-firmware/use-cases.md) · [P02.R8](../../products/p02-user-app/prd.md) · [P02.UC5](../../products/p02-user-app/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [파일 또는 QR](../book.html#W17/start) | **공동서명할 PSBT를 가져오세요**<br>등록할 descriptor와 공동서명자 지문을 독립 장치에서 대조하세요. | 계정 — Bitcoin signet · 금고 0<br>정책 — 2-of-3<br>입력 선택 — UTXO 2개 | [파일·QR 내용 검토](../book.html#W17/review)<br>[서명 정책 확인](../book.html#W17/policy)<br>[계정으로](../book.html#W25) | — |
| [정책 등록](../book.html#W17/policy) | **2-of-3 금고 정책을 대조하세요**<br>기기에서 세 서명자와 change 분기를 확인하고 등록합니다. | 지문 — C401 · C402 · C403<br>외부 / change — 0 / 1 | [취소](../book.html#W17) | [정책 물리 등록 완료](../book.html#W17) |
| [거래 검증](../book.html#W17/review) | **외부 출력 15,000 sat**<br>모든 prevout과 자기 거스름돈 경로, 수수료율을 독립 표시에서 확인하세요. | 입력 — 23,000 sat<br>외부 출력 — 15,000 sat · 수취 스크립트 0014aabbccddeeff00112233445566778899aabbccdd<br>자기 change — 7,600 sat · 계정 0/1/4<br>수수료 — 400 sat · 2 sat/vB<br>서명 정책 — 2-of-3 · SIGHASH_ALL | [거절](../book.html#W17)<br>[정책 보기](../book.html#W17/policy) | [보드 부분서명 반환](../book.html#W17/partial)<br>[descriptor·change 불일치](../book.html#W17/reject) |
| [부분서명](../book.html#W17/partial) | **서명 1 / 2개**<br>가져온 PSBT에 내 서명만 추가했습니다. 남은 서명자에게 전달하세요. | 제출 — 아직 없음<br>결과 — 부분서명 PSBT | [계정으로](../book.html#W25) | — |
| [검증 실패](../book.html#W17/reject) | **등록한 정책과 출력이 다릅니다**<br>해당 파일을 거절하고 송신자가 만든 원본을 확인하세요. | 거스름돈 — 자기 파생 경로 불일치<br>서명 — 0건 | [다른 파일 검토](../book.html#W17) | — |

## W18 · 구독 약관과 일정

**사용자** 구독자　 **진입** 결제의 구독 관리

**완료 기준** 약관 동의와 회차 지급을 분리하고 해지 후 미래 청구는 0건이다.

[P02.R5](../../products/p02-user-app/prd.md) · [P02.UC3](../../products/p02-user-app/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [구독 조건](../book.html#W18/start) | **도시 산책 패스**<br>약관 v2에 등록하면 매달 청구서를 받습니다. 각 회차는 직접 승인하세요. | 가맹점 — 도시 산책<br>주기 — 매월 1일 · Asia/Seoul<br>회차 상한 — 4.00 TestUSDC · StableNet 테스트넷<br>납기 — 발급 뒤 7일<br>해지 — 다음 미발급 회차부터 | [조건에 동의해 등록](../book.html#W18/active)<br>[결제로](../book.html#W09) | — |
| [구독 중](../book.html#W18/active) | **다음 청구 일정을 관리하세요**<br>가격이나 자산 변경은 새 약관 동의를 받습니다. | 현재 약관 — v2<br>발급 회차 — 7회 · INV-07 | [이번 회차](../book.html#W19)<br>[일시정지](../book.html#W18/paused)<br>[해지 검토](../book.html#W18/cancel) | [가격·자산 변경](../book.html#W18/changed) |
| [일시정지](../book.html#W18/paused) | **새 회차 발급을 멈췄습니다**<br>기존 발급 건은 같은 청구서에서 조회합니다. | 미래 발급 — 일시정지<br>기존 발급 — INV-07 | [구독 재개](../book.html#W18/active)<br>[기존 회차](../book.html#W19) | — |
| [약관 변경](../book.html#W18/changed) | **상한이 5.00 TestUSDC로 변경됩니다**<br>새 약관에 동의하기 전 새 조건의 청구를 보류합니다. | v2 — 4.00 TestUSDC<br>v3 — 5.00 TestUSDC<br>기존 INV-07 — v2 유지 | [변경 동의](../book.html#W18/newterms)<br>[해지 검토](../book.html#W18/cancel) | — |
| [변경 등록](../book.html#W18/newterms) | **다음 미발급 회차부터 v3 적용**<br>기존에 발급한 견적과 납기를 유지합니다. | 회차 상한 — 5.00 TestUSDC<br>적용 — 8회부터 | [기존 청구 조회](../book.html#W19)<br>[해지 검토](../book.html#W18/cancel) | — |
| [해지 검토](../book.html#W18/cancel) | **다음 회차부터 구독을 종료할까요?**<br>발급된 견적의 확정·만료는 청구서에서 추적합니다. | 종료 대상 — 미래 회차<br>기존 발급 — INV-07 | [미래 회차 해지](../book.html#W18/closed)<br>[돌아가기](../book.html#W18/active) | — |
| [해지 완료](../book.html#W18/closed) | **다음 청구서를 발급하지 않습니다**<br>이미 발급한 7회 청구서를 확인할 수 있습니다. | 미래 청구 — 0건<br>기존 조회 키 — INV-07 | [기존 청구서](../book.html#W19)<br>[결제 목록](../book.html#W09) | — |

## W19 · 회차 청구와 영수증

**사용자** 구독자　 **진입** 미결 목록 또는 회차 알림

**완료 기준** 중복 알림·두 폰은 INV-07 한 건으로 복귀하고 미승인 지급은 0건이다.

[P02.R5](../../products/p02-user-app/prd.md) · [P02.UC3](../../products/p02-user-app/use-cases.md) · [P01.R2](../../products/p01-device-firmware/prd.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [7회 청구](../book.html#W19/start) | **도시 산책 · 4.00 TestUSDC**<br>소유권과 만료를 다시 조회하고 가맹점 서명 견적 원본을 보드로 전달합니다. | 청구서 — INV-07 · 약관 v2<br>네트워크 — StableNet 테스트넷 8283<br>납기 — 11월 7일 18:00 KST<br>지급자 — 구독 계정 1 | [보드 요약 검토](../book.html#W19/review)<br>[회차 건너뛰기](../book.html#W19/skip)<br>[구독 일정](../book.html#W18/active) | [납기 경과](../book.html#W19/reconcile) |
| [회차 승인 대기](../book.html#W19/review) | **7회분 4.00 TestUSDC**<br>보드가 pay.prepare로 검증한 지급자·발급자·회차·해시를 대조하고 보드 버튼을 누르세요. | 상점 — 도시 산책<br>견적 — Q-INV-07<br>체인 — 8283<br>청구서 — INV-07<br>최대 지급 — 4.00 TestUSDC | [거절](../book.html#W19/skip) | [보드 승인·제출](../book.html#W19/pending)<br>[기한 경과](../book.html#W19/reconcile) |
| [조회 중](../book.html#W19/pending) | **INV-07 지급을 확인하고 있습니다**<br>다른 폰이나 중복 알림에서도 같은 회차를 조회합니다. | 합계 — 4.00 TestUSDC<br>제출 — 1건<br>확정 — 대기 | [같은 회차 재조회](../book.html#W19/pending) | [체인·원장 대사 완료](../book.html#W19/paid)<br>[납기 경과 후 기존 제출 정산](../book.html#W19/reconcile) |
| [회차 영수증](../book.html#W19/paid) | **7회분 4.00 TestUSDC 지급 완료**<br>다음 청구가 필요 없으면 미래 회차를 해지하세요. | 회차 — 7<br>지급 — 1건<br>영수증 — INV-07 | [미래 회차 해지](../book.html#W18/cancel)<br>[결제 목록](../book.html#W09) | — |
| [승인 생략](../book.html#W19/skip) | **이번 회차를 승인하지 않았습니다**<br>이미 발급된 견적은 원 회차에서 만료나 확정까지 조회합니다. | 이번 조작 지급 — 0건<br>조회 키 — INV-07 | [기존 회차 조회](../book.html#W19)<br>[구독 관리](../book.html#W18/active) | — |
| [납기 경과](../book.html#W19/expired) | **7회 견적이 만료됐습니다**<br>만료 견적을 다시 승인할 수 없습니다. | 이번 회차 지급 — 0건<br>청구서 — INV-07<br>원 견적 — Q-INV-07 · 미소비<br>대사 범위 — 기한 내 포함 가능 블록 전체 · 확정 경계 통과 | [구독 일정](../book.html#W18/active) | — |
| [납기 경과 · 대사](../book.html#W19/reconcile) | **INV-07의 기한 내 지급 여부를 확인합니다**<br>새 제출을 닫고 원래 견적·해시가 기한 내 포함되었는지 확인합니다. | 기존 견적 — Q-INV-07<br>추가 제출 — 금지<br>기존 지급 — 대사 중 | [기존 회차 재조회](../book.html#W19/reconcile) | [기한 내 포함·원장 대사 완료](../book.html#W19/paid)<br>[확정 경계까지 미지급 확인](../book.html#W19/expired) |

## W20 · 클라우드 지갑 만들기

**사용자** 소유자　 **진입** 지갑 선택에서 지원 프로필 선택

**완료 기준** 독립 사용자 증명과 DKG 완료 전 활성 주소는 0개이며 재시도는 원 요청을 조회한다.

[P03.R1](../../products/p03-cloud-mpc-wallet/prd.md) · [P03.R4](../../products/p03-cloud-mpc-wallet/prd.md) · [P03.UC1](../../products/p03-cloud-mpc-wallet/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [별도 지갑](../book.html#W20/start) | **클라우드 지갑 생성**<br>개인 보드와 별도의 클라우드 계정을 사용합니다. 계정 연계, 독립 사용자 증명, 분산 키 생성이 끝날 때까지 새 지갑의 활성 주소는 0개입니다. | 지갑 종류 — 클라우드 MPC · 테스트 프로필<br>인증 — 배포에서 등록한 제공자 또는 시험 인증 서비스<br>참여자 — 모바일 A · 서비스 B · 독립 복구 C | [클라우드 서비스에 별도 로그인](../book.html#W20/proof)<br>[개인 보드 선택](../book.html#W02) | — |
| [사용자 확인](../book.html#W20/proof) | **등록한 독립 수단으로 확인하세요**<br>로그인만으로 지갑을 만들 수 없습니다. | 생성 요청 — DKG-42<br>결과 — 증명 대기 | [기존 요청 조회](../book.html#W20/proof) | [독립 증명 검증 완료](../book.html#W20/dkg) |
| [분산 키 생성](../book.html#W20/dkg) | **참여자 공개키를 대조하고 있습니다**<br>일부 응답이 없으면 같은 요청을 조회하세요. | 생성 세션 — DKG-42<br>준비 — 2 / 3<br>활성 주소 — 0개 | [동일 요청 재조회](../book.html#W20/dkg) | [DKG 완료·공개키 일치](../book.html#W20/ready)<br>[참여자 시간 초과](../book.html#W20/delay) |
| [일부 응답 없음](../book.html#W20/delay) | **생성을 일시 보류했습니다**<br>기존 DKG-42 상태를 조회하세요. | 활성 주소 — 0개<br>누락 — 참여자 ACK | [생성 상태 재조회](../book.html#W20/dkg) | — |
| [클라우드 계정](../book.html#W20/ready) | **클라우드 계정 1**<br>활성 계정과 서명 요청을 확인하세요. | 계정 — MPC-42 · 테스트<br>세대 — 3<br>생성 결과 — 활성 계정 1개 | [서명 요청](../book.html#W21)<br>[휴대폰 복구](../book.html#W22)<br>[계정 연계](../book.html#W23) | — |

## W21 · 클라우드 서명 요청

**사용자** 소유자　 **진입** MPC-42의 서명 요청

**완료 기준** 지갑·원문·세대·기한을 모바일 별도 승인에 결합하고 라운드 서명을 검증한 뒤 반환한다.

[P03.R2](../../products/p03-cloud-mpc-wallet/prd.md) · [P03.UC2](../../products/p03-cloud-mpc-wallet/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [모바일 참여자 승인](../book.html#W21/start) | **0.01 테스트 ETH 전송 요청**<br>개별 모바일 인증으로 이 원문을 승인하세요. 지원 조합의 A와 B가 서명에 참여합니다. | 계정 — MPC-42 · Sepolia<br>받는 주소 — 0x6020…0004 (화면에서 합성값 전체 표시)<br>최대 가스비 — 0.0002 ETH<br>원문 / 세대 — SIG-32 / 3<br>만료 — 14:32 KST | [개별 인증 시작](../book.html#W21/auth)<br>[거절](../book.html#W21/deny) | — |
| [모바일 인증](../book.html#W21/auth) | **등록한 모바일 참여자로 확인하세요**<br>기한·원문·세대가 그대로인지 확인합니다. | 참여자 — A<br>서비스 — B 대기<br>결합 원문 — SIG-32 | [취소](../book.html#W21/deny) | [별도 인증·원문 결합 완료](../book.html#W21/round)<br>[기한 또는 세대 불일치](../book.html#W21/deny) |
| [라운드 진행](../book.html#W21/round) | **A와 B의 응답을 기다립니다**<br>단독 응답은 서명 결과로 반환하지 않습니다. | 라운드 — SIG-32<br>세대 — 3<br>결과 서명 — 검증 대기 | [같은 세션 조회](../book.html#W21/round) | [공개키로 결과 서명 검증](../book.html#W21/signed)<br>[단독 B·세대 변경](../book.html#W21/deny) |
| [서명 반환](../book.html#W21/signed) | **검증된 서명을 제출자에게 반환했습니다**<br>체인 제출과 확정은 별도 결과입니다. | 세션 — SIG-32<br>결과 — 검증된 서명 1건<br>체인 확정 — 미확인 | [클라우드 계정](../book.html#W20/ready) | — |
| [요청 종료](../book.html#W21/deny) | **라운드를 폐기했습니다**<br>새 원문에는 새 승인과 일회 자료가 필요합니다. | 서명 반환 — 0건<br>이전 일회 자료 — 폐기 | [계정으로](../book.html#W20/ready) | — |

## W22 · 클라우드 참여자 복구

**사용자** 소유자　 **진입** 새 휴대폰에서 클라우드 계정 복구

**완료 기준** 독립 승인·대기·필수 ACK가 모두 확인된 후 새 세대를 활성화한다.

[P03.R3](../../products/p03-cloud-mpc-wallet/prd.md) · [P03.R4](../../products/p03-cloud-mpc-wallet/prd.md) · [P03.UC3](../../products/p03-cloud-mpc-wallet/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [새 폰 등록](../book.html#W22/start) | **MPC-42의 휴대폰을 교체합니다**<br>복구를 접수하면 새 서명이 동결됩니다. 독립 승인자와 등록된 통지 채널을 확인하세요. | 기존 세대 — 3<br>복구 요청 — REC-18<br>새 단말 — A′ | [복구 접수](../book.html#W22/request)<br>[취소](../book.html#W20/ready) | — |
| [복구 동결](../book.html#W22/frozen) | **새 서명이 잠겼습니다**<br>독립 승인과 정책상 대기 뒤 새 참여자를 검증합니다. | 요청 — REC-18<br>기존 세대 — 차단<br>통지 — 등록 채널 발송<br>대기 — 정책 만료 확인 전 | [복구 상태 재조회](../book.html#W22/frozen) | [독립 승인·대기 충족](../book.html#W22/acks)<br>[독립 확인 거절](../book.html#W22/denied) |
| [교체 확인](../book.html#W22/acks) | **참여자 활성 영수증을 수집합니다**<br>누락 시 서명 차단을 유지합니다. | 세대 — 4 후보<br>현재 ACK — A′ 수신 · B 대기<br>주소 — 기존 공개키 대조 중 | [상태 재조회](../book.html#W22/acks) | [필수 ACK·주소 일치](../book.html#W22/active)<br>[활성 영수증 누락](../book.html#W22/hold) |
| [복구 보류](../book.html#W22/hold) | **서명을 계속 차단합니다**<br>필수 활성 ACK가 도착하지 않았습니다. REC-18에서 원래 교체 작업을 추적하세요. | 복구 참조 — REC-18<br>활성화 — 대기 | [같은 복구 상태 조회](../book.html#W22/hold) | [누락 증명 수신·정책 재검증](../book.html#W22/acks) |
| [새 세대 활성](../book.html#W22/active) | **새 폰으로 복구했습니다**<br>교체 전후 공개키와 새 참여자 영수증을 대조했습니다. | 기존 주소 — 일치<br>활성 세대 — 4<br>옛 참여자 — 거절 | [클라우드 계정 관리](../book.html#W23) | — |
| [접수 결과 조회](../book.html#W22/request) | **복구 요청 REC-18를 확인합니다**<br>동결 커밋의 응답을 잃어도 동일 복구 요청을 조회합니다. | 원 요청 — REC-18<br>동결 여부 — 서버 확인 대기 | [기존 요청 조회](../book.html#W22/request) | [복구 접수·동결 커밋 확인](../book.html#W22/frozen) |
| [독립 승인 거절](../book.html#W22/denied) | **휴대폰 교체를 승인하지 않았습니다**<br>REC-18에서 거절 근거와 동결 해제 여부를 조회하세요. 새 세대를 활성화할 수 없습니다. | 독립 승인 — 거절<br>새 세대 활성 — 0건<br>서명 동결 — 유지 | [거절 기록 조회](../book.html#W22/denied) | [재심사 허가·새 독립 증명 요청](../book.html#W22/frozen) |

## W23 · 인증수단과 계정 종료

**사용자** 소유자　 **진입** 설정의 인증수단 또는 클라우드 계정

**완료 기준** 동시 해제에도 마지막 인증수단 하나를 보존하고 계정 종료로 보드 키·자산을 이동하지 않는다.

[P03.R4](../../products/p03-cloud-mpc-wallet/prd.md) · [P03.UC1](../../products/p03-cloud-mpc-wallet/use-cases.md) · [P03.UC3](../../products/p03-cloud-mpc-wallet/use-cases.md) · [P02.R9](../../products/p02-user-app/prd.md) · [P02.UC6](../../products/p02-user-app/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [연계 계정](../book.html#W23/start) | **내 인증수단**<br>개인 하드월렛은 소셜 가입 없이 사용합니다. 클라우드 계정의 마지막 인증수단은 유지하세요. | 연계 — Google · Apple<br>지갑 — MPC-42<br>계정 종료 — 별도 절차 | [Google 해제](../book.html#W23/single)<br>[계정 종료 검토](../book.html#W23/exit)<br>[로그아웃](../book.html#W23/logout) | — |
| [남은 인증수단](../book.html#W23/single) | **Apple 한 개가 남았습니다**<br>동시에 해제해도 저장 시점에 마지막 수단을 검사합니다. | Google — 해제<br>Apple — 연계 중 | [추가 수단 연계](../book.html#W23/link)<br>[Apple 해제 검토](../book.html#W23/blocked) | — |
| [추가 인증](../book.html#W23/link) | **Google 연계를 인증하세요**<br>계정의 재인증 결과를 기다립니다. | 기존 — Apple<br>새 수단 — Google | [취소](../book.html#W23/single) | [제공자 인증·연계 확인](../book.html#W23) |
| [해제 제한](../book.html#W23/blocked) | **마지막 인증수단을 유지하세요**<br>새 수단을 연계한 뒤 기존 수단을 해제할 수 있습니다. | 보존된 연계 — Apple | [다른 수단 연계](../book.html#W23/link) | — |
| [로그아웃](../book.html#W23/logout) | **인증 세션을 종료했습니다**<br>다시 로그인하면 연계 계정을 조회할 수 있습니다. | 소셜 세션 — 종료<br>지갑 — 계정 유지 | [지갑 선택](../book.html#W02) | — |
| [탈퇴 검토](../book.html#W23/exit) | **계정 종료 범위를 확인하세요**<br>서비스 접근과 보존 대상 기록을 확인하고 탈퇴를 신청하세요. 외부 자산과 보드 키는 별도 관리합니다. | 대상 — 서비스 계정<br>자산 자동 이동 — 0건<br>종료 기록 — 정책에 따라 보관 | [탈퇴 신청](../book.html#W23/closing)<br>[취소](../book.html#W23) | — |
| [탈퇴 접수](../book.html#W23/closing) | **종료 요청을 접수했습니다**<br>등록 통지 채널에서 처리 결과를 확인하세요. | 요청 — EXIT-42<br>자산 이동 — 0건 | [지갑 선택](../book.html#W02) | — |

## W24 · MPC 사고·감사 조회

**사용자** 복구 담당자　 **진입** 권한 있는 복구 담당자의 서비스 상태

**완료 기준** 미검증 A+C 서명은 0건이며 과거 정족수 유출은 일반 갱신으로 종결하지 않는다.

[P03.R2](../../products/p03-cloud-mpc-wallet/prd.md) · [P03.R3](../../products/p03-cloud-mpc-wallet/prd.md) · [P03.UC4](../../products/p03-cloud-mpc-wallet/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [사고 분류](../book.html#W24/start) | **참여자 서비스 상태**<br>참여자별 가용성과 침해 신고를 분리해 조회합니다. | 서비스 B — 응답 지연<br>마지막 확인 — 14:20 KST<br>감사 자료 — 세션 참조 · 정책 버전 · 원인 코드 | [서비스 장애](../book.html#W24/outage)<br>[정족수 유출 신고](../book.html#W24/breach)<br>[감사 기록](../book.html#W24/audit) | — |
| [장애 대기](../book.html#W24/outage) | **검증된 대체 경로를 확인하세요**<br>예시 프로필은 A+C 미인수입니다. 서비스 복구까지 기존 상태를 유지합니다. | 프로필 — 정책 v3<br>A+C 우회 — 사용 불가<br>최근 세션 — SIG-32 | [서비스 재조회](../book.html#W24)<br>[감사 기록](../book.html#W24/audit) | — |
| [키 위험](../book.html#W24/breach) | **과거 정족수 자료 유출 신고**<br>세대 갱신 뒤에도 외부에서 기존 키를 사용할 위험이 있습니다. 새 키 생성과 사용자 동의 이전, 기존 권한 회수 가능성을 평가하세요. | 사고 — INC-42<br>기존 키 — 노출 가능<br>자산 이전 — 사용자 동의 대기<br>서비스 — 새 서명 동결 | [감사 기록](../book.html#W24/audit)<br>[현재 서비스 상태](../book.html#W24) | — |
| [감사 타임라인](../book.html#W24/audit) | **INC-42 · 정책 v3**<br>업무 감사에는 공개 참조와 transcript digest만 표시합니다. | 14:20 — B 지연 · B_TIMEOUT<br>14:24 — 유출 신고 · QUORUM_REPORT<br>14:25 — 동결 · SIGN_FREEZE | [사고 분류](../book.html#W24) | — |

## W25 · 계정·주소록·자산 관리

**사용자** 소유자　 **진입** 지갑의 계정·자산 관리

**완료 기준** 체인·계약별 검색과 즐겨찾기·숨김·재표시 후 확정 잔액과 원 미결 참조를 보존한다. 오프라인 시세에 출처·표시 통화·조회 시각이 남는다.

[P01.R3](../../products/p01-device-firmware/prd.md) · [P01.R6](../../products/p01-device-firmware/prd.md) · [P01.UC4](../../products/p01-device-firmware/use-cases.md) · [P02.R2](../../products/p02-user-app/prd.md) · [P02.UC1](../../products/p02-user-app/use-cases.md) · [P02.R8](../../products/p02-user-app/prd.md) · [P02.UC5](../../products/p02-user-app/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [계정과 자산](../book.html#W25/start) | **표시할 계정과 자산을 관리하세요**<br>체인과 계정 번호를 함께 확인하세요. 숨긴 자산도 주소와 거래 기록을 유지합니다. | 개인 계정 1 — StableNet · 42.00 TestUSDC<br>금고 0 — Bitcoin signet · 공동서명 | [계정 추가·이름 변경](../book.html#W25/edit)<br>[숨긴 계정 재발견](../book.html#W25/discover)<br>[토큰 관리](../book.html#W25/token)<br>[주소록](../book.html#W25/contacts)<br>[공개정보 내보내기](../book.html#W16)<br>[PSBT 가져오기](../book.html#W17)<br>[온체인 권한](../book.html#W15)<br>[받기](../book.html#W07) | — |
| [계정 편집](../book.html#W25/edit) | **계정 이름과 파생 인덱스를 확인하세요**<br>새 계정은 지원 경로에서 파생합니다. 숨기기는 계정 키를 유지합니다. | 입력 · 계정 이름 — 여행 계정<br>입력 · 다음 계정 번호 — 1<br>현재 이름 — 개인 계정 1<br>인덱스 — 0 | [이름 저장](../book.html#W25)<br>[계정 숨기기](../book.html#W25/discover) | — |
| [재발견](../book.html#W25/discover) | **숨은 계정과 사용한 주소 유형**<br>외부·거스름돈 분기와 사용한 계정 범위를 대조해 복원한 공개 주소를 찾습니다. | 범위 — 계정 0–2<br>발견 — 금고 0 · Native SegWit<br>주소 유형 — 프로필 BIP-84 | [선택 계정 표시](../book.html#W25)<br>[공개 범위 보기](../book.html#W16) | — |
| [토큰 목록](../book.html#W25/token) | **계약과 단위를 확인하세요**<br>같은 심볼도 체인·계약·issuer가 다르면 별도로 관리합니다. 수동 등록 시 검증한 단위를 대조합니다. | 입력 · 검색어 — TestUSDC<br>입력 · 토큰 계약 — 0x4020…0001 (화면에서 합성값 전체 표시)<br>네트워크 — StableNet 테스트넷 8283<br>표시 잔액 — 42.00 TestUSDC<br>즐겨찾기 — 0개 | [검색 결과 보기](../book.html#W25/search)<br>[즐겨찾기 목록](../book.html#W25/emptyfav)<br>[숨긴 자산](../book.html#W25/hiddenlist)<br>[수동 등록 검토](../book.html#W25/verify) | — |
| [토큰 대조](../book.html#W25/verify) | **토큰 메타데이터를 확인하세요**<br>토큰 식별자와 decimals를 검증된 프로필과 대조합니다. | 체인 — 8283<br>decimals — 6<br>표시 대상 — 이 계정만 | [검증된 토큰 추가](../book.html#W25)<br>[취소](../book.html#W25/token) | [decimals 불일치](../book.html#W25/bad) |
| [토큰 불일치](../book.html#W25/bad) | **단위가 검증값과 다릅니다**<br>동일 심볼만으로 자산을 합치지 않습니다. | 등록 — 보류 | [계약 다시 확인](../book.html#W25/token) | — |
| [주소록](../book.html#W25/contacts) | **검증된 수취인 목록**<br>허용목록 변경은 독립 확인과 정책상 대기 후 적용합니다. | 수취인 — 여행 경비 계정<br>체인 — StableNet 테스트넷<br>주소 — 0x8020…0315 (화면에서 합성값 전체 표시) | [이 주소로 보내기](../book.html#W08)<br>[허용목록 변경 검토](../book.html#W27/limits) | — |
| [자산 검색 결과](../book.html#W25/search) | **TestUSDC · 1개 자산**<br>네트워크와 계약 주소로 일치 여부를 확인하세요. | 네트워크 — StableNet 8283<br>계약 — 0xdec0…1010 (화면에서 합성값 전체 표시)<br>잔액 — 42.00 TestUSDC<br>미결 송금 — SEND-315 · 3.00 TestUSDC | [즐겨찾기에 추가](../book.html#W25/favorite)<br>[목록에서 숨기기](../book.html#W25/hiddenlist)<br>[환산 시세](../book.html#W25/rate)<br>[다시 검색](../book.html#W25/token) | — |
| [즐겨찾기](../book.html#W25/emptyfav) | **즐겨찾는 자산이 없습니다**<br>현재 네트워크에서 자주 보는 자산을 검색해 추가하세요. | 체인 — StableNet 8283<br>등록 — 0개 | [자산 검색](../book.html#W25/token) | — |
| [즐겨찾기](../book.html#W25/favorite) | **즐겨찾는 자산 1개**<br>즐겨찾기를 해제해도 계정의 잔액과 송금 내역은 유지됩니다. | 자산 — TestUSDC · 8283<br>잔액 — 42.00 TestUSDC<br>진행 중 — SEND-315 | [즐겨찾기 해제](../book.html#W25/search)<br>[환산 시세](../book.html#W25/rate)<br>[자산 관리](../book.html#W25) | — |
| [숨긴 자산](../book.html#W25/hiddenlist) | **표시 목록에서 숨긴 TestUSDC**<br>표시 설정은 자산 키와 거래 상태에 영향을 주지 않습니다. 미결 거래는 원래 참조로 조회하세요. | 일반 목록 표시 — 꺼짐<br>보관 잔액 — 42.00 TestUSDC<br>기존 미결 — SEND-315 · 3.00 TestUSDC | [목록에 다시 표시](../book.html#W25/restored)<br>[기존 송금 조회](../book.html#W08/pending)<br>[계정 관리](../book.html#W25) | — |
| [목록에 복귀](../book.html#W25/restored) | **TestUSDC를 다시 표시합니다**<br>숨기기 이전과 동일한 계약·확정 잔액·미결 참조를 표시합니다. | 체인 / 계약 — 8283 / 0xdec0…1010 (화면에서 합성값 전체 표시)<br>확정 잔액 — 42.00 TestUSDC<br>미결 — SEND-315 · 3.00 TestUSDC | [기존 송금 조회](../book.html#W08/pending)<br>[다시 숨기기](../book.html#W25/hiddenlist)<br>[자산 조회](../book.html#W25/search) | — |
| [환산 예시](../book.html#W25/rate) | **42.00 TestUSDC · 예시 환산 ₩54,600**<br>테스트 자산의 표시 연습용 고정값입니다. 실제 교환이나 현금 상환 가격으로 사용하지 않습니다. | 표시 통화 — KRW<br>예시 환율 — 1 TestUSDC = 1,300 KRW<br>출처 — 교육용 고정 시세 v1<br>기준 시각 — 11월 20일 14:20 KST | [자산으로](../book.html#W25/search) | [네트워크 단절](../book.html#W25/stalerate) |
| [오프라인 시세](../book.html#W25/stalerate) | **마지막으로 표시한 예시 환산 ₩54,600**<br>새 가격을 조회할 수 없어 캐시 시각을 유지합니다. | 표시 통화 — KRW<br>출처 — 교육용 고정 시세 v1<br>캐시 시각 — 11월 20일 14:20 KST<br>체인 확정 잔액 — 마지막 조회 42.00 TestUSDC | [기존 자산으로](../book.html#W25/search) | [시세 조회 복구](../book.html#W25/rate) |

## W26 · 네트워크·RPC 관리

**사용자** 소유자　 **진입** 지갑 또는 설정에서 네트워크 선택

**완료 기준** 등록과 활성 전환을 구분하며 잘못된 genesis·chainId·엔진은 저장하지 않는다. 각 체인별 검증 메서드만 선택 가능하고 미검증 staking·NFT·복합 호출의 서명은 0건이다.

[P01.R3](../../products/p01-device-firmware/prd.md) · [P01.UC2](../../products/p01-device-firmware/use-cases.md) · [P02.R2](../../products/p02-user-app/prd.md) · [P02.R3](../../products/p02-user-app/prd.md) · [P02.UC1](../../products/p02-user-app/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [네트워크 선택](../book.html#W26/start) | **테스트넷 프로필**<br>체인별 지원 범위와 수수료 단위를 확인하세요. 메인넷과 테스트넷의 계정·활동은 분리됩니다. | Bitcoin signet — UTXO · sat/vB<br>Ethereum Sepolia — EIP-1559 · ETH<br>Base Sepolia — L2 실행비 + L1 데이터 비용<br>StableNet 8283 — TestUSDC · 네이티브 가스<br>Solana devnet — instruction · SOL<br>XRPL Testnet — DestinationTag · drops | [커스텀 RPC 등록](../book.html#W26/rpc)<br>[활성 체인 전환](../book.html#W26/switch)<br>[체인별 송금 확인](../book.html#W28)<br>[지갑으로](../book.html#W01)<br>[체인별 기능표](../book.html#W26/capability) | — |
| [사용자 정의 RPC](../book.html#W26/rpc) | **같은 엔진의 네트워크 등록**<br>HTTPS RPC 응답의 chainId·genesis와 파서 기능을 대조합니다. | 입력 · RPC URL — https://rpc.example<br>입력 · 체인 ID — 8283<br>입력 · 기준 genesis — manifest의 전체 해시<br>입력 · 탐색기 URL — https://explorer.example<br>엔진 — EVM<br>검사 — 기준 블록 · 단위 · 확정 정책 | [검사 요청](../book.html#W26/check)<br>[취소](../book.html#W26) | — |
| [프로필 검사](../book.html#W26/check) | **RPC 응답을 대조합니다**<br>같은 체인 ID여도 기준 블록이 다르면 거절합니다. | 활성 체인 — 기존 프로필 유지<br>상태 — 검사 중 | [취소](../book.html#W26) | [대조 통과·등록 완료](../book.html#W26/switch)<br>[다른 genesis·미지원 엔진](../book.html#W26/error) |
| [전환 검토](../book.html#W26/switch) | **활성 체인을 바꿀까요?**<br>전환하면 기존 미서명 요청을 폐기합니다. 저장한 RPC만 바꾸는 경우에도 별도 동의가 필요합니다. | 대상 — StableNet 테스트넷 8283<br>기존 미결 거래 — 원 체인에서 조회 유지 | [전환에 동의](../book.html#W01)<br>[취소](../book.html#W26) | — |
| [등록 거부](../book.html#W26/error) | **네트워크 식별값을 확인하세요**<br>기존 활성 체인과 자산 표시를 유지합니다. | 검사 결과 — genesis 불일치 또는 어댑터 미지원<br>추가 — 0건 | [RPC 수정](../book.html#W26/rpc) | — |
| [릴리스 기능표](../book.html#W26/capability) | **이 보드가 확인할 수 있는 거래**<br>검증 프로필별 기능입니다. 고급 메서드는 파서·원문 표시·서명 시험을 통과한 조합에서만 엽니다. | Bitcoin signet — P2WPKH 전송 · 등록 PSBT 공동서명<br>Ethereum Sepolia — ETH·검증 ERC-20 · SIWE · 제한 allowance<br>Base Sepolia — ETH·검증 ERC-20 · L1/L2 비용 확인<br>StableNet 8283 — TestUSDC 전송 · 등록 가맹점 지급<br>Solana devnet — System Transfer · 검증 SPL Transfer<br>XRPL Testnet — Payment · DestinationTag · validated 수령액 | [Bitcoin PSBT 검토](../book.html#W17)<br>[체인별 전송 검토](../book.html#W28)<br>[기능 목록으로](../book.html#W26) | [미검증 staking 요청 도착](../book.html#W26/stake)<br>[미검증 NFT 메서드 요청 도착](../book.html#W26/nft)<br>[미검증 복합 호출 도착](../book.html#W26/batch) |
| [미지원 요청](../book.html#W26/stake) | **스테이킹 요청을 거절했습니다**<br>현재 실행 버전에서 원문 전체를 해석·표시하는 메서드로 검증되지 않았습니다. 이 요청의 보드 승인 대기를 폐기합니다. | 체인 — Solana devnet<br>요청 — Stake Program · DelegateStake<br>서명 / 제출 — 0 / 0건 | [검증된 기능 보기](../book.html#W26/capability)<br>[연결 관리](../book.html#W15) | — |
| [미지원 요청](../book.html#W26/nft) | **NFT 전송 요청을 거절했습니다**<br>현재 실행 버전에서 원문 전체를 해석·표시하는 메서드로 검증되지 않았습니다. 이 요청의 보드 승인 대기를 폐기합니다. | 체인 — Ethereum Sepolia<br>요청 — ERC-721 · safeTransferFrom<br>서명 / 제출 — 0 / 0건 | [검증된 기능 보기](../book.html#W26/capability)<br>[연결 관리](../book.html#W15) | — |
| [미지원 요청](../book.html#W26/batch) | **복합 호출 요청을 거절했습니다**<br>현재 실행 버전에서 원문 전체를 해석·표시하는 메서드로 검증되지 않았습니다. 이 요청의 보드 승인 대기를 폐기합니다. | 체인 — Base Sepolia<br>요청 — multicall(bytes[])<br>서명 / 제출 — 0 / 0건 | [검증된 기능 보기](../book.html#W26/capability)<br>[연결 관리](../book.html#W15) | — |

## W27 · 설정·접근성·지원

**사용자** 소유자　 **진입** 하단 설정

**완료 기준** 200% 글씨와 화면 읽기에서 금액·수취인·취소가 접근 가능하며 진단 공유 전 항목을 확인한다. 한국어·영어 연습 요청은 같은 금액과 수취인을 표시하고 민감 알림 숨김이나 푸시 거절 뒤에도 미결 목록 접근을 제공한다.

[P01.R2](../../products/p01-device-firmware/prd.md) · [P01.R4](../../products/p01-device-firmware/prd.md) · [P02.R1](../../products/p02-user-app/prd.md) · [P02.R7](../../products/p02-user-app/prd.md) · [P02.UC1](../../products/p02-user-app/use-cases.md) · [P02.R8](../../products/p02-user-app/prd.md) · [P02.UC5](../../products/p02-user-app/use-cases.md) · [P02.R9](../../products/p02-user-app/prd.md) · [P02.UC6](../../products/p02-user-app/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [설정](../book.html#W27/start) | **기기와 앱 설정**<br>중단한 승인과 등록은 원래 요청에서 다시 조회할 수 있습니다. | 활성 보드 — 끝자리 71C4<br>자동 잠금 — 유휴 2분 또는 연결 해제 | [기기·업데이트](../book.html#W12)<br>[분실 복구](../book.html#W06)<br>[한도와 잠금](../book.html#W27/limits)<br>[접근성](../book.html#W27/access)<br>[도움말·진단](../book.html#W27/diag)<br>[인증수단](../book.html#W23)<br>[네트워크](../book.html#W26)<br>[연습 다시 보기](../book.html#W02/practice)<br>[미결 결제](../book.html#W09)<br>[개인정보와 여행 동의](../book.html#T11) | — |
| [보호 정책](../book.html#W27/limits) | **보드 지출 한도와 허용 주소**<br>한도 상향과 허용목록 추가는 독립 표시·물리 확인·정책 대기를 거칩니다. | 건별 / 일별 — 10 / 30 TestUSDC<br>변경 요청 — 건별 12 TestUSDC<br>자동 잠금 — 유휴 2분 · 분리 즉시 | [변경 검토 요청](../book.html#W27/policy)<br>[취소](../book.html#W27) | — |
| [정책 대기](../book.html#W27/policy) | **보호 정책 변경 대기**<br>기기에서 내용을 확인하고 대기 시간을 마쳐야 합니다. | 효력 — 현재 10 TestUSDC 유지<br>변경 — 12 TestUSDC 후보 | [기기 상태](../book.html#W12) | [기기 확인·대기 만료](../book.html#W27/applied) |
| [정책 적용](../book.html#W27/applied) | **새 건별 한도가 적용되었습니다**<br>보드에서 읽은 활성 정책입니다. | 건별 — 12 TestUSDC<br>일별 — 30 TestUSDC | [설정](../book.html#W27) | — |
| [접근성](../book.html#W27/access) | **표시와 알림을 설정하세요**<br>화면 읽기는 금액·수취인·체인을 확인한 뒤 취소로 이동할 수 있습니다. 읽기 설정을 바꿔도 견적과 기기 승인의 만료 시각은 그대로입니다. | 글자 — 200%<br>동작 줄이기 — 켬<br>언어 — 한국어 / English<br>알림 미리보기 — 민감한 값 숨김 | [글자와 움직임](../book.html#W27/font)<br>[화면 읽기 순서](../book.html#W27/reader)<br>[언어 선택](../book.html#W27/language)<br>[알림 미리보기](../book.html#W27/notification)<br>[설정으로](../book.html#W27) | — |
| [진단 미리보기](../book.html#W27/diag) | **공유할 진단 항목을 확인하세요**<br>사용자가 선택한 경우에만 OS 공유 시트로 전달합니다. | 포함 — 펌웨어 버전 · 실패 코드 · 요청 참조<br>제외 — 시드 · PIN · 개인키<br>마지막 원인 — BLE_PERMISSION | [공유할 항목 검토](../book.html#W27/share)<br>[설정](../book.html#W27) | — |
| [공유 범위](../book.html#W27/share) | **선택한 진단 항목**<br>받을 사람과 앱을 OS 공유 시트에서 지정하세요. | 내용 — 펌웨어 버전 · BLE_PERMISSION<br>사용자 식별값 — 제외 | [미리보기로](../book.html#W27/diag) | — |
| [글자와 움직임](../book.html#W27/font) | **결제 내용을 큰 글씨로 보기**<br>200%와 동작 줄이기를 선택하면 애니메이션은 정적 상태 안내로 바뀝니다. | 현재 글자 — 기본<br>미리보기 — 연습 카페 · 2 연습 코인<br>동작 줄이기 — 끔 | [200%와 동작 줄이기 적용](../book.html#W27/font200)<br>[접근성으로](../book.html#W27/access) | — |
| [200% · 정적 표시](../book.html#W27/font200) | **2 연습 코인**<br>큰 글씨에서 금액·수취인·취소를 차례로 읽을 수 있습니다. | 수취인 — 연습 카페<br>체인 — 연습 모드<br>안내 — 보드 버튼 위치 · 정적 그림 | [연습 결제 취소](../book.html#W02/reject)<br>[기본 글자 선택](../book.html#W27/font)<br>[접근성으로](../book.html#W27/access) | — |
| [화면 읽기](../book.html#W27/reader) | **연습 주문을 순서대로 듣기**<br>연습 모드임을 먼저 알립니다. VoiceOver 또는 TalkBack으로 항목을 차례로 읽고, 다시 듣기를 눌러도 같은 연습 주문을 안내합니다. | 요청 출처 — 보드 없는 연습 모드<br>금액 — 2 연습 코인<br>받는 곳 — 연습 카페<br>체인 — 연습 모드<br>수수료 — 연습 수수료 0<br>확인 방법 — 가상 버튼 연습<br>마지막 행동 — 요청 취소 | [안내 다시 듣기](../book.html#W27/reader)<br>[거절 연습](../book.html#W02/reject)<br>[접근성으로](../book.html#W27/access)<br>[실제 기기 승인 화면 예시](../book.html#W10) | — |
| [언어 선택](../book.html#W27/language) | **앱 표시 언어**<br>금액·단위·수취인과 요청 식별자는 언어를 바꿔도 같은 주문을 가리킵니다. | 한국어 — 현재 선택<br>English — 선택 가능 | [English 선택](../book.html#W27/english)<br>[한국어 유지](../book.html#W27/access) | — |
| [English](../book.html#W27/english) | **Review practice payment**<br>Check the recipient and amount. Use the cancel action to decline this practice request. | Amount — 2 practice coins<br>Recipient — Practice Café<br>Network — Practice mode | [Cancel practice payment](../book.html#W02/reject)<br>[한국어로 변경](../book.html#W27/access) | — |
| [잠금 화면 알림](../book.html#W27/notification) | **알림에 표시할 정보**<br>잠금 화면에는 주소·금액·상점명을 싣지 않습니다. 알림을 끄면 결제 탭에서 미결 요청을 확인하세요. | 알림 예시 — 확인할 결제가 있습니다<br>민감 미리보기 — 가려짐<br>조회 — 앱 잠금 해제와 원주문 권한 확인 | [알림 끄기](../book.html#W27/notifyoff)<br>[접근성으로](../book.html#W27/access) | — |
| [푸시 꺼짐](../book.html#W27/notifyoff) | **결제 요청을 앱 안에서 확인하세요**<br>알림이 꺼져도 회차 발급이나 지급 상태는 유지됩니다. | 푸시 — 끔<br>미결 보기 — 결제 탭<br>잠금 화면 — 표시 없음 | [미결 목록](../book.html#W09)<br>[민감값 숨김으로 켜기](../book.html#W27/notification) | — |

## W28 · 체인별 거래 원문 확인

**사용자** 소유자　 **진입** 네트워크 지원표에서 체인별 확인

**완료 기준** 각 테스트 체인에서 식별자·금액·수수료와 고유 필드를 보여주고 미해석 요청은 거절한다.

[P01.R2](../../products/p01-device-firmware/prd.md) · [P01.R3](../../products/p01-device-firmware/prd.md) · [P01.UC2](../../products/p01-device-firmware/use-cases.md) · [P02.R2](../../products/p02-user-app/prd.md) · [P02.UC1](../../products/p02-user-app/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [확인할 체인](../book.html#W28/start) | **원문을 확인할 테스트망**<br>계정과 파서가 지원하는 종류를 선택하세요. | 계정 — 체인별 테스트 계정 0<br>출처 — 보드 파서의 원문 요약 | [Bitcoin](../book.html#W28/btc)<br>[Ethereum](../book.html#W28/eth)<br>[Base](../book.html#W28/base)<br>[StableNet](../book.html#W08/review)<br>[Solana](../book.html#W28/sol)<br>[XRPL](../book.html#W28/xrp) | — |
| [Ethereum Sepolia](../book.html#W28/eth) | **0.01 ETH 보내기**<br>독립 표시에서 수취인과 체인을 대조한 후 보드 버튼으로 확인하세요. | chainId — 11155111<br>nonce — 14<br>최대 가스비 — 0.0002 ETH<br>수취인 — 0x6020…0004 (화면에서 합성값 전체 표시) | [거절](../book.html#W28/reject)<br>[체인 선택](../book.html#W28) | [보드 서명·제출](../book.html#W28/ethpending)<br>[원문 오류·기한 경과](../book.html#W28/reject) |
| [제출 조회](../book.html#W28/ethpending) | **Ethereum Sepolia 거래 조회**<br>해당 체인의 확정 기준에 도달할 때까지 기존 거래만 조회합니다. | 망 — Ethereum Sepolia<br>상태 — 제출됨 · 미확정 | [기존 거래 조회](../book.html#W28/ethpending) | [체인별 확정 결과 확인](../book.html#W28/ethdone) |
| [전송 결과](../book.html#W28/ethdone) | **Ethereum Sepolia 송금 결과**<br>독립 클라이언트와 수취액을 대조한 결과입니다. | 네트워크 — Ethereum Sepolia<br>지급 결과 — 0.01 ETH · finalized | [체인 선택](../book.html#W28) | — |
| [Base Sepolia](../book.html#W28/base) | **0.01 테스트 ETH 보내기**<br>독립 표시에서 수취인과 체인을 대조한 후 보드 버튼으로 확인하세요. | chainId — 84532<br>L2 실행 상한 — 0.00001 ETH<br>L1 데이터 추정 — 0.000003 ETH<br>수취인 — 0x6020…0004 (화면에서 합성값 전체 표시) | [거절](../book.html#W28/reject)<br>[체인 선택](../book.html#W28) | [보드 서명·제출](../book.html#W28/basepending)<br>[원문 오류·기한 경과](../book.html#W28/reject) |
| [제출 조회](../book.html#W28/basepending) | **Base Sepolia 거래 조회**<br>해당 체인의 확정 기준에 도달할 때까지 기존 거래만 조회합니다. | 망 — Base Sepolia<br>상태 — 제출됨 · 미확정 | [기존 거래 조회](../book.html#W28/basepending) | [체인별 확정 결과 확인](../book.html#W28/basedone) |
| [전송 결과](../book.html#W28/basedone) | **Base Sepolia 송금 결과**<br>독립 클라이언트와 수취액을 대조한 결과입니다. | 네트워크 — Base Sepolia<br>지급 결과 — 0.01 ETH · finalized | [체인 선택](../book.html#W28) | — |
| [Solana devnet](../book.html#W28/sol) | **0.1 SOL 보내기**<br>독립 표시에서 수취인과 체인을 대조한 후 보드 버튼으로 확인하세요. | fee payer — 계정 0<br>수취인 — 7YttLkHDoNj9wyDur5B3s7MQ33Z6qe1VYMw73hq4wNxe<br>instruction — System Program · Transfer<br>수수료 — 0.000005 SOL<br>blockhash — 유효 높이 12400까지 | [거절](../book.html#W28/reject)<br>[체인 선택](../book.html#W28) | [보드 서명·제출](../book.html#W28/solpending)<br>[원문 오류·기한 경과](../book.html#W28/reject) |
| [제출 조회](../book.html#W28/solpending) | **Solana devnet 거래 조회**<br>해당 체인의 확정 기준에 도달할 때까지 기존 거래만 조회합니다. | 망 — Solana devnet<br>상태 — 제출됨 · 미확정 | [기존 거래 조회](../book.html#W28/solpending) | [체인별 확정 결과 확인](../book.html#W28/soldone) |
| [전송 결과](../book.html#W28/soldone) | **Solana devnet 송금 결과**<br>독립 클라이언트와 수취액을 대조한 결과입니다. | 네트워크 — Solana devnet<br>지급 결과 — 0.1 SOL · finalized | [체인 선택](../book.html#W28) | — |
| [XRPL Testnet](../book.html#W28/xrp) | **1 XRP 보내기**<br>독립 표시에서 수취인과 체인을 대조한 후 보드 버튼으로 확인하세요. | Destination — rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh<br>DestinationTag — 142<br>Fee — 12 drops<br>Sequence — 21<br>LastLedgerSequence — 12008 | [거절](../book.html#W28/reject)<br>[체인 선택](../book.html#W28) | [보드 서명·제출](../book.html#W28/xrppending)<br>[원문 오류·기한 경과](../book.html#W28/reject) |
| [제출 조회](../book.html#W28/xrppending) | **XRPL Testnet 거래 조회**<br>해당 체인의 확정 기준에 도달할 때까지 기존 거래만 조회합니다. | 망 — XRPL Testnet<br>상태 — 제출됨 · 미확정 | [기존 거래 조회](../book.html#W28/xrppending) | [체인별 확정 결과 확인](../book.html#W28/xrpdone) |
| [전송 결과](../book.html#W28/xrpdone) | **XRPL Testnet 송금 결과**<br>독립 클라이언트와 수취액을 대조한 결과입니다. | 네트워크 — XRPL Testnet<br>지급 결과 — validated · delivered_amount 1 XRP | [체인 선택](../book.html#W28) | — |
| [서명 거절](../book.html#W28/reject) | **이 원문을 승인할 수 없습니다**<br>지원 형식·수취인·기한을 확인한 후 새 원문을 검토하세요. | 서명 — 0건<br>제출 — 0건 | [체인별 원문으로](../book.html#W28) | — |
| [Bitcoin signet](../book.html#W28/btc) | **15,000 sat 보내기**<br>독립 표시에서 전체 수신 주소와 거스름돈 파생 경로를 확인한 뒤 보드 버튼을 누르세요. | 수신 주소 — tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx<br>입력 — 23,000 sat<br>자기 change — 7,600 sat · 계정 0/1/4<br>수수료 — 400 sat · 2 sat/vB | [거절](../book.html#W28/reject)<br>[공동서명 PSBT](../book.html#W17) | [보드 단독서명·제출](../book.html#W28/btcpending)<br>[검증 실패](../book.html#W28/reject) |
| [기존 거래 조회](../book.html#W28/btcpending) | **Bitcoin signet 포함 대기**<br>재실행 후 같은 txid로 관측된 블록과 확인 수를 조회합니다. | 참조 — BTC-315<br>확인 수 — 0<br>출력 — 15,000 sat | [같은 거래 조회](../book.html#W28/btcpending) | [프로필 확인 수 충족](../book.html#W28/btcdone) |
| [수신 확인](../book.html#W28/btcdone) | **15,000 sat 전송 확인**<br>현재 활성 체인의 원 거래를 대조했습니다. | 참조 — BTC-315<br>수신 — 15,000 sat<br>수수료 — 400 sat | [체인 선택](../book.html#W28) | — |


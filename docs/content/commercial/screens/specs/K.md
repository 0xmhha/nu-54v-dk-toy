# 매장·키오스크 · 페이지별 설계

[전체 화면](../book.html) · [원 요구사항 대조표](../coverage.md) · [통합 감사](../audit.md)

제품에서 누르는 동작과 상태를 바꾸는 외부 사건을 열로 구분했다. 입력은 예시 데이터이며 실제 제품의 인수에서는 진입 조건과 검증 결과까지 함께 확인한다.

## K01 · 공용 메뉴

**제품** 키오스크·결제 단말 앱　 **사용자** 공용 단말 고객　 **진입** 주문 모드가 활성화된 단말의 초기 화면

**완료 기준** 공용 고객 A가 메뉴를 5분 보는 동안 고객 B는 자기 폰에서 독립 주문을 확정할 수 있다. A는 전체 메뉴 52행의 어느 위치에서도 자신의 주문 요약을 열 수 있다. 언어·분류를 바꿔도 수량이 유지되고, 연결 복구 후에는 최신 재고·가격을 확인한 뒤 같은 장바구니에서 주문한다.

[P04.R1](../../products/p04-merchant-kiosk/prd.md) · [P04.R2](../../products/p04-merchant-kiosk/prd.md) · [P04.UC1](../../products/p04-merchant-kiosk/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [메뉴](../book.html#K01/start) | **여울 커피 성수점**<br>공용 주문 화면에서 메뉴를 고르거나 본인 폰으로 주문할 수 있습니다. | 입력 · 언어 — 한국어<br>입력 · 카테고리 — 전체<br>커피 — 아이스 아메리카노 3.50 TestUSDC<br>베이커리 — 크루아상 3.00 TestUSDC<br>품절 — 소금빵 | [아이스 아메리카노 옵션](../book.html#K07)<br>[크루아상 상세](../book.html#K07/croissant)<br>[내 장바구니](../book.html#K02)<br>[폰으로 주문하는 법](../book.html#K09)<br>[카테고리·언어 선택](../book.html#K01/view) | [연결이 끊겨 캐시된 메뉴를 표시](../book.html#K01/offline)<br>[공용 세션이 만료됨](../book.html#K09/reset) |
| [카테고리와 언어](../book.html#K01/view) | **메뉴 보기 설정**<br>카테고리를 누르면 해당 품목을 확인할 수 있습니다. 언어를 변경해도 현재 장바구니는 유지됩니다. | 현재 언어 — 한국어<br>카테고리 — 전체 | [커피 옵션](../book.html#K07)<br>[베이커리 상세](../book.html#K07/croissant)<br>[English menu](../book.html#K01/english)<br>[전체 메뉴](../book.html#K01) | — |
| [English menu](../book.html#K01/english) | **Yeoul Coffee Seongsu**<br>Choose a menu item or open your basket. | Coffee — Iced americano 3.50 TestUSDC<br>Bakery — Croissant 3.00 TestUSDC<br>Sold out — Salt bread | [Croissant details](../book.html#K07/croissant)<br>[한국어 메뉴](../book.html#K01)<br>[장바구니](../book.html#K02) | — |
| [매장 연결 끊김](../book.html#K01/offline) | **최근 메뉴를 보고 있습니다**<br>매장 서버 연결이 돌아오면 현재 가격과 재고를 확인할 수 있습니다. | 메뉴 동기화 — 10:39<br>새 견적 발급 — 일시 중지 | [매장 연결 다시 확인](../book.html#K01/offline) | [현재 메뉴 동기화 완료](../book.html#K01) |

## K02 · 공용 장바구니

**제품** 키오스크·결제 단말 앱　 **사용자** 공용 단말 고객　 **진입** 상품 옵션 저장 또는 메뉴의 장바구니

**완료 기준** 마지막 재고는 한 세션만 확보하고 발급 전 가격 변경은 다시 확인받는다.

[P04.R1](../../products/p04-merchant-kiosk/prd.md) · [P04.R2](../../products/p04-merchant-kiosk/prd.md) · [P04.R3](../../products/p04-merchant-kiosk/prd.md) · [P04.UC1](../../products/p04-merchant-kiosk/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [결제 전 확인](../book.html#K02/start) | **장바구니**<br>수량을 확인하세요. 결제 연결 전에 재고와 가격을 확인합니다. | 입력 · 크루아상 수량 — 1<br>아이스 아메리카노 ×1 — 3.50 TestUSDC<br>크루아상 ×1 — 3.00 TestUSDC<br>합계 — 6.50 TestUSDC | [재고 확인 후 결제 연결](../book.html#K08)<br>[옵션 수정](../book.html#K07)<br>[주문 그만하기](../book.html#K09/cancel) | [다른 주문이 마지막 수량 확보](../book.html#K02/stock)<br>[게시 가격 변경 감지](../book.html#K02/price) |
| [품목 재선택](../book.html#K02/stock) | **크루아상 재고가 소진됐습니다**<br>아이스 아메리카노 선택을 유지합니다. | 확보 못한 품목 — 크루아상 ×1 | [품목을 빼고 메뉴 열기](../book.html#K01) | — |
| [가격 재확인](../book.html#K02/price) | **크루아상 가격이 바뀌었습니다**<br>새 합계를 승인하면 재고를 다시 확인합니다. | 기존 합계 — 6.50 TestUSDC<br>새 합계 — 7.00 TestUSDC | [가격 반영 후 다시 검토](../book.html#K08/repriced)<br>[장바구니 수정](../book.html#K07) | — |

## K03 · 폰에서 매장 찾기

**제품** 사용자 모바일 앱　 **사용자** 근거리 고객　 **진입** 개인 폰의 근거리 주문 또는 매장 입구 QR

**완료 기준** 모바일 데이터를 끈 고객도 온라인 단말의 BLE를 통해 별도 견적을 받고 매장명이 다르면 계속할 수 없다.

[P04.R2](../../products/p04-merchant-kiosk/prd.md) · [P04.R3](../../products/p04-merchant-kiosk/prd.md) · [P04.UC1](../../products/p04-merchant-kiosk/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [매장 확인](../book.html#K03/start) | **여울 커피 성수점에서 주문**<br>간판의 지점과 발견한 단말을 비교하세요. 폰 데이터가 없으면 매장 단말로 연결합니다. | 발견된 단말 — 성수 주문 단말 02<br>연결 방식 — 단말 BLE 중계 | [이 매장 메뉴 열기](../book.html#K10)<br>[다른 매장 찾기](../book.html#K03/search) | [BLE 권한이 없거나 탐색이 어려움](../book.html#K03/fallback)<br>[폰과 매장 단말이 모두 오프라인](../book.html#K03/offline) |
| [매장 재검색](../book.html#K03/search) | **매장 이름과 단말을 확인하세요**<br>단말 신호는 근처 후보를 찾는 데 사용합니다. | 입력 · 지점명 — 여울 커피 성수점<br>선택한 매장 — 없음 | [성수점 다시 확인](../book.html#K03) | — |
| [연결 방법 선택](../book.html#K03/fallback) | **매장 입구의 QR을 스캔하세요**<br>온라인 연결이 가능하면 QR로 같은 매장을 열 수 있습니다. | BLE 권한 — 꺼짐 | [카메라로 매장 QR 확인](../book.html#K03)<br>[매장 연결 다시 찾기](../book.html#K03/search) | — |
| [연결 없음](../book.html#K03/offline) | **새 주문을 시작할 수 없습니다**<br>매장 단말이나 폰 연결이 돌아올 때까지 이전 주문은 기존 ID로 확인할 수 있습니다. | 신규 발급 — 일시 중지 | [매장 상태 다시 확인](../book.html#K03/offline)<br>[내 지갑](../book.html#W01) | [온라인 경로 복구](../book.html#K03) |

## K04 · POS 결제 전용

**제품** 키오스크·결제 단말 앱　 **사용자** 인증 POS 직원　 **진입** 등록 POS가 지점과 외부 주문 참조·금액을 전송

**완료 기준** 승인된 POS 최신 금액을 고정하고 QR/BLE 중복 20회에도 서명 가능 견적은 선택된 지급자에게만 발급한다.

[P04.R1](../../products/p04-merchant-kiosk/prd.md) · [P04.R3](../../products/p04-merchant-kiosk/prd.md) · [P04.R5](../../products/p04-merchant-kiosk/prd.md) · [P04.UC2](../../products/p04-merchant-kiosk/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [결제 미리보기](../book.html#K04/start) | **POS 주문 S-0142**<br>고객이 QR을 스캔하거나 BLE로 요청하면 고객 폰의 연결 표식을 비교하세요. | 매장 — 여울 커피 성수점<br>원주문 — S-0142<br>확정 금액 — 6.50 TestUSDC<br>네트워크 — StableNet 테스트넷 8283 | [원주문 상태 조회](../book.html#K04/lookup) | [지급자 후보가 접수됨](../book.html#K11)<br>[인증 POS가 미지원 자산 제출](../book.html#K04/asset)<br>[같은 외부 주문이 다시 도착](../book.html#K04/lookup) |
| [원주문 조회](../book.html#K04/lookup) | **S-0142의 기존 결제 요청**<br>POS 참조와 입금 상태를 조회합니다. | 동일 외부 참조 — S-0142<br>추가 발급 — 보류 | [다시 조회](../book.html#K04/lookup) | [기존 결합·견적 확인](../book.html#K11/issued)<br>[지급·영수증 확정 기록 확인](../book.html#K04/paid)<br>[새 결합 가능 확인](../book.html#K04) |
| [요청 거절](../book.html#K04/asset) | **TestUSDC 결제만 가능합니다**<br>POS에서 보낸 자산과 네트워크를 확인한 뒤 원래 참조를 조회하세요. | 지원 네트워크 — StableNet 테스트넷 8283<br>잘못된 자산 — 요청 거절 | [POS 주문 다시 확인](../book.html#K04/lookup) | — |
| [POS 확정](../book.html#K04/paid) | **S-0142 결제 완료**<br>POS에서 동일 참조의 결제 기록을 확인하세요. | 금액 — 6.50 TestUSDC<br>증거 — P05 확정 원장 및 POS 이행 통지 | [원주문 다시 조회](../book.html#K04/lookup) | — |

## K05 · 확정 주문과 픽업

**제품** 키오스크·결제 단말 앱　 **사용자** 지점 점원　 **진입** 성수점 직원 세션 또는 단말 재시작 복구

**완료 기준** 제출 직후 재시작해도 P05 이행 eventId를 한 번만 접수하고 주문과 영수증을 복제하지 않는다.

[P04.R4](../../products/p04-merchant-kiosk/prd.md) · [P04.R6](../../products/p04-merchant-kiosk/prd.md) · [P04.UC3](../../products/p04-merchant-kiosk/use-cases.md) · [P05.R6](../../products/p05-operations-backoffice/prd.md) · [P05.UC4](../../products/p05-operations-backoffice/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [대기열](../book.html#K05/start) | **성수점 준비 대기열**<br>지급과 회계가 함께 기록된 이행 통지를 받은 주문만 조리할 수 있습니다. | S-0142 · A17 — 준비 중 / 지급 10:42 / 이행 evt-142<br>병렬 시험 S-0151 · A16 — 11/19 픽업 가능 / 지급 10:40 / 이행 evt-151<br>확인 중 주문 — 이행 잠김 | [A17 조리 상세](../book.html#K05/prep)<br>[A16 픽업 상세](../book.html#K05/bleReady)<br>[제출 직후 재시작 시나리오](../book.html#K05/unknown)<br>[직원 화면 잠그기](../book.html#K09/reset) | [재시작 직후 기존 주문 체크포인트 복구](../book.html#K05/unknown) |
| [서버 대사](../book.html#K05/unknown) | **기존 주문과 거래를 조회합니다**<br>확정 높이와 RPC 관측이 일치하거나 원주문 이행 통지가 도착할 때까지 대기합니다. | 미확정 주문 — 조리·재청구 보류<br>마지막 조회 — 10:42 | [기존 지급 결과 재조회](../book.html#K05/unknown)<br>[대기열](../book.html#K05) | [P05가 직접 입금 증거와 장부를 기록하고 해당 orderId의 eventId가 수신됨](../book.html#K05) |
| [준비 중](../book.html#K05/prep) | **A17 준비 중**<br>크루아상과 아이스 아메리카노가 준비되면 픽업 가능을 기록하세요. | 이행 ID — evt-142<br>영수증 — R-0142 | [준비 완료 기록](../book.html#K05/ready)<br>[영수증 재출력](../book.html#K05/print) | — |
| [픽업 가능](../book.html#K05/ready) | **A17 픽업 가능**<br>픽업 번호를 확인하고 전달한 다음 완료 처리합니다. | 결제 상태 — 지급 완료<br>조리 상태 — 준비 완료 | [픽업 완료 기록](../book.html#K05/picked)<br>[영수증 재출력](../book.html#K05/print) | [픽업 가능 통지가 해당 고객에게 전달됨](../book.html#W11/pickup) |
| [이행 완료](../book.html#K05/picked) | **A17 전달 완료**<br>주문 이행과 결제 기록을 보존합니다. | 영수증 — R-0142 | [대기열](../book.html#K05)<br>[영수증 재출력](../book.html#K05/print) | [픽업 완료 통지가 해당 고객에게 전달됨](../book.html#W11/collected) |
| [폰 주문 픽업](../book.html#K05/bleReady) | **S-0151 · A16 픽업 가능**<br>11월 19일 병렬 주문 시험에서 공용 손님과 별도로 발급한 근거리 주문입니다. | 지급 — 8.00 TestUSDC / 10:40<br>픽업 가능 — 10:41<br>이행 ID — evt-151 | [대기열](../book.html#K05) | [근거리 고객에게 지급과 픽업 상태 전달](../book.html#W11/blePaid) |
| [재출력](../book.html#K05/print) | **영수증 R-0142**<br>같은 영수증 ID로 재출력합니다. | 출력 대상 — 기존 확정 영수증 R-0142 | [다시 출력 요청](../book.html#K05/print)<br>[대기열](../book.html#K05) | [프린터 응답 유실 또는 용지 부족](../book.html#K05/printer) |
| [출력 장애](../book.html#K05/printer) | **프린터를 확인해 주세요**<br>용지나 연결을 복구한 뒤 같은 영수증을 재출력할 수 있습니다. | 영수증 — R-0142<br>출력 — 미확인 | [같은 영수증 다시 열기](../book.html#K05/print) | — |

## K06 · 단말 모드와 설정

**제품** 키오스크·결제 단말 앱　 **사용자** 단말 관리자　 **진입** 인증된 관리자만 단말 설정 접근

**완료 기준** 인계와 버전 ACK가 모두 확인될 때만 POS 전용을 활성화하고 재부팅·레거시 주문 API는 원주문을 보존한다.

[P04.R1](../../products/p04-merchant-kiosk/prd.md) · [P04.R6](../../products/p04-merchant-kiosk/prd.md) · [P04.UC4](../../products/p04-merchant-kiosk/use-cases.md) · [P05.R1](../../products/p05-operations-backoffice/prd.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [설정](../book.html#K06/start) | **성수점 단말 02**<br>신규 고객 유입을 멈추고 발급된 주문을 인계한 다음 새 설정을 활성화합니다. | 현재 모드 — 주문 모드 v6<br>다음 버전 — POS 결제 전용 v7<br>발급된 견적 — 1건 | [전환 영향 검토](../book.html#K06/review)<br>[직원 화면 잠그기](../book.html#K09/reset) | — |
| [전환 영향](../book.html#K06/review) | **신규 주문 차단 및 인계**<br>이미 발급한 주문을 성수점 직원 대기열로 인계합니다. | 기존 주문 — S-0142<br>담당 — 성수점 주문 운영<br>새 설정 — 결제 전용 v7 | [전환 요청 저장](../book.html#K06/waiting)<br>[기존 모드로 돌아가기](../book.html#K06) | — |
| [인계·ACK 대기](../book.html#K06/waiting) | **결제 전용 전환 대기**<br>진행 중 원주문과 적용 버전을 계속 조회합니다. | 발급 주문 기록 — 운영 큐 인계 확인 중<br>단말 ACK — v7 응답 대기<br>새 주문 — 중지 | [기존 전환 조회](../book.html#K06/waiting) | [인계 및 ACK 모두 도착](../book.html#K06/active)<br>[재부팅 또는 ACK 실패](../book.html#K06/failed) |
| [전환 복구](../book.html#K06/failed) | **인계 또는 적용 결과를 확인할 수 없습니다**<br>기존 v7 전환 요청을 조회한 뒤 같은 버전 재전송을 요청할 수 있습니다. | 신규 주문 — 중지 유지<br>원주문 — 같은 주문 ID로 조회 | [기존 요청 재조회](../book.html#K06/waiting) | — |
| [전환 완료](../book.html#K06/active) | **결제 전용 v7이 적용됐습니다**<br>이전 주문은 직원 대기열에서 조회하고 새 주문은 등록 POS에서 받습니다. | 단말 ACK — v7<br>이전 주문 인계 — 확인<br>오래된 메뉴 주문 요청 — 서버 거절 | [직원 인증으로 돌아가기](../book.html#K12) | — |

## K07 · 메뉴와 옵션

**제품** 키오스크·결제 단말 앱　 **사용자** 공용 단말 고객　 **진입** 메뉴에서 상품 선택

**완료 기준** 필수 옵션 누락과 소진된 품목은 저장을 막으며 고객은 다른 항목을 고를 수 있다.

[P04.R1](../../products/p04-merchant-kiosk/prd.md) · [P04.R2](../../products/p04-merchant-kiosk/prd.md) · [P04.UC1](../../products/p04-merchant-kiosk/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [옵션](../book.html#K07/start) | **아이스 아메리카노**<br>필수 옵션을 고른 뒤 담아 주세요. | 입력 · 온도 (필수) — 아이스<br>입력 · 샷 — 기본<br>입력 · 수량 — 1<br>기본가 — 3.50 TestUSDC<br>알레르기 정보 — 기본 음료에 우유 미포함 / 제조 도구에서 우유 교차접촉 가능<br>선택한 옵션 포함 합계 — 3.50 TestUSDC | [옵션과 수량 저장](../book.html#K02)<br>[메뉴로 돌아가기](../book.html#K01) | [필수 옵션을 비우고 제출](../book.html#K07/required)<br>[선택한 품목 소진](../book.html#K07/soldout) |
| [베이커리 상세](../book.html#K07/croissant) | **크루아상**<br>수량을 고르고 장바구니에 담으세요. 알레르기가 있으면 주문 전에 직원에게 제조 환경을 확인하세요. | 입력 · 수량 — 1<br>가격 — 3.00 TestUSDC<br>알레르기 정보 — 밀·우유·달걀 함유 / 견과류를 다루는 제빵 설비 사용 | [크루아상 담기](../book.html#K02)<br>[다른 메뉴 선택](../book.html#K01) | [선택한 품목 소진](../book.html#K02/stock) |
| [필수 항목 누락](../book.html#K07/required) | **온도를 선택해 주세요**<br>선택한 수량은 유지됩니다. | 입력 오류 — 온도 선택 필요 | [옵션 수정](../book.html#K07) | — |
| [품절](../book.html#K07/soldout) | **선택한 메뉴가 품절됐습니다**<br>다른 품목을 취급하는 카테고리를 열어 주세요. | 아이스 아메리카노 — 품절 | [다른 메뉴 선택](../book.html#K01) | — |

## K08 · 공용 주문 결제 연결

**제품** 키오스크·결제 단말 앱　 **사용자** 공용 단말 고객　 **진입** 장바구니 재고 확보 후 연결

**완료 기준** 메뉴에서 만든 원주문의 지점, StableNet 테스트넷 8283, 자산과 확정 합계 6.50 TestUSDC를 연결 QR과 함께 표시한다. 서로 다른 고객 폰이 받은 확인 번호를 각각 표시하고, 실제 폰과 일치하는 번호 하나를 선택한다. 선택을 같은 요청 키로 기록하고 새 QR과 다른 확인 번호를 즉시 숨긴다. 응답이 유실되거나 앱이 재시작돼도 기존 주문과 선택으로만 조회하며, 서버가 확인한 연결만 현재 공용 세션에 표시한다. 폰을 결합하기 전에는 서명할 수 없고, 공용 화면을 비운 뒤에도 선택한 폰에서 원주문 추적이 계속된다.

[P04.R3](../../products/p04-merchant-kiosk/prd.md) · [P04.R4](../../products/p04-merchant-kiosk/prd.md) · [P04.R5](../../products/p04-merchant-kiosk/prd.md) · [P04.UC1](../../products/p04-merchant-kiosk/use-cases.md) · [P04.UC3](../../products/p04-merchant-kiosk/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [폰 연결](../book.html#K08/start) | **폰에서 이 주문을 열어 주세요**<br>QR 또는 BLE로 폰을 연결하고 양쪽 화면의 연결 표식을 비교하세요. | 매장 — 여울 커피 성수점<br>금액 — 6.50 TestUSDC<br>네트워크 — StableNet 테스트넷 8283<br>연결 표식 — 폰이 연결을 요청하면 확인 | [연결 다시 안내](../book.html#K08)<br>[공용 화면 닫기](../book.html#K09/reset) | [폰이 연결 요청](../book.html#K08/match)<br>[연결 제한 시간 경과](../book.html#K08/ended) |
| [고객 확인](../book.html#K08/match) | **내 폰의 표식을 확인하세요**<br>두 폰이 다른 번호를 받으면 자기 폰에 표시된 번호를 눌러 연결을 확인하세요. | 연결 표식 — 37ea19cd, 2ba8cf04<br>최종 지급 승인 — 고객 폰과 보드 | [이 폰으로 요청](../book.html#K08/binding)<br>[다른 폰 사용](../book.html#K08) | — |
| [결합 처리](../book.html#K08/binding) | **이 폰의 결제 요청을 준비합니다**<br>선택한 표식과 원주문을 확인하는 중입니다. 연결이 확인되면 선택한 폰에서 견적을 요청할 수 있습니다. | 매장 — 여울 커피 성수점<br>선택한 표식 — 37ea19cd | [공용 화면 닫기](../book.html#K09/reset) | [P05가 원주문과 선택한 고객의 연결을 확인](../book.html#K08/linked)<br>[결합 응답을 확인하지 못함](../book.html#K08/unknown) |
| [폰 검토](../book.html#K08/linked) | **선택한 폰에서 결제를 계속하세요**<br>연결된 폰에서 원주문의 견적을 요청하세요. 발행된 견적은 같은 폰에서 다시 열어 금액을 검토할 수 있습니다. | 금액 — 6.50 TestUSDC<br>연결한 폰 — 37ea19cd<br>견적 — 폰이 요청하기 전 | [공용 화면 닫기](../book.html#K09/reset) | [선택한 고객이 원주문의 서명 견적을 수령](../book.html#W10)<br>[원주문 P05 결제 확정 및 이행 기록, 현재 세션 표시](../book.html#K08/paid) |
| [발급 조회](../book.html#K08/unknown) | **기존 결제 연결을 확인하고 있습니다**<br>같은 세션의 원주문과 선택한 폰으로 연결 결과를 조회합니다. 확인 중에는 QR을 숨깁니다. | 마지막 확인 — 10:41 | [기존 연결 조회](../book.html#K08/unknown)<br>[공용 화면 닫기](../book.html#K09/reset) | [P05가 원주문의 기존 연결을 확인](../book.html#K08/linked)<br>[원주문 종료 확인](../book.html#K09/reset) |
| [새 가격](../book.html#K08/repriced) | **새 금액으로 결제를 연결합니다**<br>가격 변경을 별도로 검토하는 대체 시나리오입니다. 새 합계와 이 단말의 표식을 확인하세요. | 장바구니 합계 — 7.00 TestUSDC<br>새 주문 — S-0144<br>연결 표식 — cabef218 | [cabef218 연결 요청](../book.html#K08/repriceBinding)<br>[공용 화면 닫기](../book.html#K09/reset) | [연결 제한 시간 경과](../book.html#K08/ended) |
| [변경 금액 연결 확인](../book.html#K08/repriceBinding) | **S-0144의 결제 연결을 확인합니다**<br>응답을 받지 못하면 같은 원주문과 선택한 폰으로 연결 결과를 조회합니다. 연결이 확인된 뒤 선택한 폰에서만 7.00 TestUSDC의 서명 견적을 요청할 수 있습니다. | 주문 — S-0144<br>새 금액 — 7.00 TestUSDC | [기존 연결 조회](../book.html#K08/repriceBinding)<br>[공용 화면 닫기](../book.html#K09/reset) | [원주문 연결 복구 후 선택한 폰이 서명 견적을 요청·수령](../book.html#W10/repriced)<br>[P05 확정 지급 및 이행 기록을 현재 공용 세션에 표시](../book.html#K08/repricedPaid)<br>[연결 커밋 뒤 성공 응답 유실](../book.html#K08/repriceBinding) |
| [새 금액 확정](../book.html#K08/repricedPaid) | **결제 완료 · A18**<br>준비 상태는 연결한 폰에서 확인할 수 있습니다. | 주문 — S-0144<br>지급액 — 7.00 TestUSDC<br>픽업 번호 — A18 | [공용 화면 닫기](../book.html#K09/reset) | — |
| [지급 확인](../book.html#K08/paid) | **결제 완료 · A17**<br>준비 상태는 연결한 폰에서 확인할 수 있습니다. | 주문 — S-0143<br>지급액 — 6.50 TestUSDC<br>픽업 번호 — A17<br>준비 상태 — 준비 중<br>네트워크 수수료 — 가맹점 릴레이 부담 | [공용 화면 닫기](../book.html#K09/reset) | — |
| [연결 만료](../book.html#K08/ended) | **연결 시간이 지났습니다**<br>견적을 받은 경우 폰에서 원주문을 확인할 수 있습니다. | 공용 화면 — 자동 초기화 대기 | [공용 화면 닫기](../book.html#K09/reset) | — |

## K09 · 공용 화면 인계와 종료

**제품** 키오스크·결제 단말 앱　 **사용자** 공용 단말 고객　 **진입** 폰 주문 안내 또는 고객 종료·세션 만료

**완료 기준** 폰 주문 안내를 열고 돌아오면 공용 장바구니의 수량이 유지된다. 고객이 닫으면 미인계·미발급 예약의 상품과 옵션 재고는 한 번만 복구된다. 폰으로 인계한 원주문은 그 폰에서 계속 조회한다. 종료·자동 만료·재시작 뒤 다음 고객은 이전 주문을 볼 수 없다.

[P04.R1](../../products/p04-merchant-kiosk/prd.md) · [P04.R2](../../products/p04-merchant-kiosk/prd.md) · [P04.R4](../../products/p04-merchant-kiosk/prd.md) · [P04.UC1](../../products/p04-merchant-kiosk/use-cases.md) · [P04.UC3](../../products/p04-merchant-kiosk/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [독립 폰 주문](../book.html#K09/start) | **내 폰에서 주문하기**<br>매장 입구 QR 또는 지갑의 근거리 찾기를 사용하세요. 이 단말을 다른 고객이 사용 중이어도 폰 주문이 열립니다. | 매장 — 여울 커피 성수점<br>단말 — 성수 주문 단말 02 | [내 공용 장바구니로](../book.html#K02) | [고객 폰에서 매장 QR 열기 또는 근거리 찾기](../book.html#K03) |
| [종료 확인](../book.html#K09/cancel) | **공용 주문을 닫을까요?**<br>폰에 연결하기 전이고 결제 견적도 아직 없으면 예약한 재고를 돌려놓습니다. 이미 연결한 주문은 내 폰에서 계속 확인할 수 있습니다. | 정리 대상 — 현재 공용 세션 | [닫고 화면 지우기](../book.html#K09/reset)<br>[계속 주문하기](../book.html#K02) | — |
| [다음 고객](../book.html#K09/reset) | **새 주문을 시작할 수 있습니다**<br>이전 고객의 표시와 조작 세션을 지웠습니다. | 현재 화면 — 신규 주문 | [메뉴 열기](../book.html#K01) | — |

## K10 · 내 폰의 매장 장바구니

**제품** 사용자 모바일 앱　 **사용자** 근거리 고객　 **진입** 성수점 확인 후 고객별 개인 세션

**완료 기준** 11월 19일 별도 병렬 시험에서 공용 주문 탐색을 유지하는 동안 근거리 S-0151을 확정하고 A16을 해당 폰에만 전달한다.

[P04.R2](../../products/p04-merchant-kiosk/prd.md) · [P04.R3](../../products/p04-merchant-kiosk/prd.md) · [P04.R5](../../products/p04-merchant-kiosk/prd.md) · [P04.UC1](../../products/p04-merchant-kiosk/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [내 장바구니](../book.html#K10/start) | **여울 커피 성수점 메뉴**<br>옵션과 수량을 확인하세요. 주문 전에 가격이 바뀌면 변경된 옵션의 단가와 기존·새 총액을 확인한 후 새 금액을 승인하거나 수량을 수정합니다. | 입력 · 음료 온도 (필수) — 아이스<br>입력 · 카페 라떼 수량 — 1<br>아이스 아메리카노 — 3.50 TestUSDC<br>카페 라떼 — 4.50 TestUSDC<br>합계 — 8.00 TestUSDC<br>개인 주문 — S-0151<br>가격 변경 시의 예 — 기존 합계 8.00 → 새 합계 8.50 TestUSDC · 확인 전 주문 접수 보류 | [내 견적 요청](../book.html#K10/request)<br>[매장 다시 선택](../book.html#K03) | — |
| [가격·재고 확인](../book.html#K10/request) | **결제 견적을 확인하고 있습니다**<br>같은 개인 주문으로 재고와 발급 여부를 조회합니다. | 개인 주문 — S-0151<br>가맹점 — 여울 커피 성수점<br>발급 방식 — 온라인 단말 중계 | [이 요청 다시 확인](../book.html#K10/request) | [P05가 S-0151 재고·견적 저장 완료](../book.html#W10/bleReview)<br>[마지막 상품 소진 응답](../book.html#K10/soldout)<br>[기존 발급을 확인할 수 없음](../book.html#K10/unknown)<br>[수량·옵션의 게시 가격 변경, 장바구니 다시 검토](../book.html#K10) |
| [품절](../book.html#K10/soldout) | **카페 라떼가 품절됐습니다**<br>다른 상품을 고를 수 있습니다. | 아이스 아메리카노 — 선택 유지 | [수량·옵션 수정](../book.html#K10) | — |
| [발급 응답 유실](../book.html#K10/unknown) | **같은 견적을 조회하고 있습니다**<br>연결이 돌아오면 기존 주문의 결과를 이어서 확인하세요. | 개인 주문 — S-0151<br>상태 — 발급 여부 미확인 | [원래 요청 재조회](../book.html#K10/unknown) | [BLE 중계 복구 후 기존 내 견적 확인](../book.html#W10/bleReview)<br>[중계가 계속 끊겨 있음](../book.html#K03/offline) |

## K11 · POS 지급자 선택

**제품** 키오스크·결제 단말 앱　 **사용자** 인증 POS 직원　 **진입** 등록 POS 주문의 QR/BLE 후보 접수

**완료 기준** 선택 충돌과 응답 유실 시 동일 결합의 결과를 조회하고 견적 저장 전에 어느 후보에도 승인 UI가 열리지 않는다.

[P04.R3](../../products/p04-merchant-kiosk/prd.md) · [P04.R4](../../products/p04-merchant-kiosk/prd.md) · [P04.UC2](../../products/p04-merchant-kiosk/use-cases.md) · [P05.R3](../../products/p05-operations-backoffice/prd.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [후보 비교](../book.html#K11/start) | **고객 폰의 연결 표식을 선택하세요**<br>매장과 금액을 고객과 확인한 뒤 같은 표식을 선택합니다. | 외부 주문 — S-0142<br>결제 금액 — 6.50 TestUSDC<br>후보 폰 — 37ea19cd, 2ba8cf04 | [37ea19cd 연결 요청](../book.html#K11/locking)<br>[후보 닫기](../book.html#K04) | — |
| [결합 처리](../book.html#K11/locking) | **선택한 지급자의 견적을 기록하고 있습니다**<br>선택 표식이 같아도 저장 결과를 확인한 뒤 승인이 열립니다. | 선택한 표식 — 37ea19cd<br>대상 주문 — S-0142 | [기존 결합 조회](../book.html#K04/lookup) | [P05가 지급자·nonce·견적 기록 완료, POS 상태](../book.html#K11/issued)<br>[P05 견적 저장 뒤 선택된 폰에 전달](../book.html#W10)<br>[다른 후보 선택이 먼저 기록됨](../book.html#K11/conflict)<br>[응답 유실](../book.html#K04/lookup) |
| [고객 승인 대기](../book.html#K11/issued) | **선택한 폰에서 결제를 계속하세요**<br>폰 또는 보드 연결이 끊겨도 같은 주문에서 승인·제출 상태를 확인합니다. | 주문 — S-0142<br>승인 대상 — 결합한 폰 37ea19cd | [원주문 확인](../book.html#K04/lookup) | [견적이 기한에 도달했으나 거래 결과 미확인](../book.html#K04/lookup) |
| [이미 결합](../book.html#K11/conflict) | **다른 지급자가 선택됐습니다**<br>다른 후보에 새 견적을 발급하기 전에 원주문의 상태를 확인하세요. | 외부 주문 — S-0142<br>다른 후보 요청 — 거절 | [원주문 확인](../book.html#K04/lookup) | — |

## K12 · 직원 인증

**제품** 키오스크·결제 단말 앱　 **사용자** 지점 직원　 **진입** 관리용 진입에서 직원 인증 요청

**완료 기준** 점원은 지점 대기열에만 접근하고 관리자 모드와 타 지점 환불은 서버가 차단한다. 설치된 인증서와 현재 지점 권한을 P05에서 확인합니다. 전용 주방 단말은 배정된 픽업만 열고, 직원 인증서는 현재 배정된 점원 또는 단말 관리자 업무만 엽니다. 재무 인증서는 P05 운영 콘솔에서 다시 확인합니다. 인증 확인 중에 고객 화면으로 돌아가거나 앱이 백그라운드로 이동하면 기다리던 응답은 직원 화면을 열지 않습니다.

[P04.R6](../../products/p04-merchant-kiosk/prd.md) · [P04.UC4](../../products/p04-merchant-kiosk/use-cases.md) · [P05.R6](../../products/p05-operations-backoffice/prd.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [직원 로그인](../book.html#K12/start) | **직원 인증**<br>소속 지점과 운영 권한을 확인합니다. | 입력 · 직원 계정 — 성수점 직원<br>단말 — 성수 단말 02 | [직원 권한 확인](../book.html#K12/checking)<br>[고객 화면](../book.html#K09/reset) | — |
| [권한 확인](../book.html#K12/checking) | **지점 권한을 확인하고 있습니다**<br>인증 결과에 따라 접근 가능한 업무를 엽니다. | 대상 — 여울 커피 성수점 | [인증 취소](../book.html#K09/reset) | [지점 점원 확인](../book.html#K05)<br>[단말 관리자 확인](../book.html#K06)<br>[환불 신청 자격이 있는 점주 확인](../book.html#O01)<br>[권한 부족 또는 자격 만료](../book.html#K12/denied) |
| [권한 없음](../book.html#K12/denied) | **이 단말을 운영할 권한이 없습니다**<br>지점 관리자에게 직원 등록 상태를 문의하세요. | 요청 — 거절 | [다시 인증](../book.html#K12)<br>[고객 화면](../book.html#K09/reset) | — |


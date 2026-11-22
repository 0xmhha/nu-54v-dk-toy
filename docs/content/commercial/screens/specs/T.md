# 여행·예약 · 페이지별 설계

[전체 화면](../book.html) · [원 요구사항 대조표](../coverage.md) · [통합 감사](../audit.md)

제품에서 누르는 동작과 상태를 바꾸는 외부 사건을 열로 구분했다. 입력은 예시 데이터이며 실제 제품의 인수에서는 진입 조건과 검증 결과까지 함께 확인한다.

## T01 · 여행 탐색

**사용자** 여행자　 **진입** 둘러보기 → 여행

**완료 기준** 위치 거부 후 지역 검색으로 같은 장소 상세를 연다.

[P09.R1](../../products/p09-travel-ai/prd.md) · [P09.UC1](../../products/p09-travel-ai/use-cases.md) · [P02.R6](../../products/p02-user-app/prd.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [지도와 목록](../book.html#T01/start) | **성수에서 찾아보기**<br>위치 권한 없이 지역을 직접 검색할 수 있습니다. | 지역 — 성수<br>유형 — 맛집 · 명소<br>정보 갱신 — 오늘 10:00 | [맛집 상세](../book.html#T03/start)<br>[명소 상세](../book.html#T03/attraction)<br>[목록 보기](../book.html#T01/list)<br>[필터](../book.html#T01/filter)<br>[지역 직접 검색](../book.html#T01/manual)<br>[내 여행](../book.html#T10/start)<br>[학습 시장](../book.html#L01/start)<br>[지갑](../book.html#W01/start) | [지도 제공자 장애](../book.html#T01/map_error) |
| [직접 검색](../book.html#T01/manual) | **지역을 입력해 주세요**<br>위치 접근은 선택 사항입니다. | 입력 · 지역 — 성수<br>지역 — 성수<br>검색 결과 — 맛집 1곳 · 명소 1곳 | [맛집 상세](../book.html#T03/start)<br>[명소 상세](../book.html#T03/attraction)<br>[지도](../book.html#T01/start) | [지역 검색 결과 0건](../book.html#T01/empty) |
| [목록](../book.html#T01/list) | **성수의 맛집과 명소**<br>지도와 목록은 같은 장소 ID를 엽니다. | 맛집 · PLACE-01 — 여울 커피 성수점 · 영업 중<br>명소 · PLACE-02 — 서울숲 · 개방 중 | [맛집 상세](../book.html#T03/start)<br>[명소 상세](../book.html#T03/attraction)<br>[지도 보기](../book.html#T01/start)<br>[필터](../book.html#T01/filter) | — |
| [필터](../book.html#T01/filter) | **찾고 싶은 장소를 선택하세요**<br>맛집과 명소를 함께 또는 따로 검색합니다. | 입력 · 장소 유형 — 맛집<br>입력 · 영업 중 — 예<br>입력 · 예약 가능 — 예<br>유형 — 맛집 · 명소<br>조건 — 영업 중 · 예약 가능 | [필터 적용](../book.html#T01/filtered)<br>[취소](../book.html#T01/start) | — |
| [검색 결과 없음](../book.html#T01/empty) | **검색 결과가 없어요**<br>지역이나 필터를 바꿔 다시 검색하세요. | 지역 — 성수<br>검색 결과 — 0건 | [지역 다시 검색](../book.html#T01/manual)<br>[필터 변경](../book.html#T01/filter) | — |
| [지도 장애](../book.html#T01/map_error) | **지도를 불러오지 못했어요**<br>지역 직접 검색과 목록으로 장소를 계속 찾을 수 있습니다. | 지도 — 연결 장애<br>대안 — 지역 검색 · 목록 | [지역 직접 검색](../book.html#T01/manual)<br>[목록 보기](../book.html#T01/list) | — |
| [필터 결과](../book.html#T01/filtered) | **예약 가능한 영업 중 맛집**<br>적용된 유형과 조건에 맞는 장소만 표시합니다. | 필터 — 맛집 · 영업 중 · 예약 가능<br>맛집 · PLACE-01 — 여울 커피 성수점 · 예약 가능 | [맛집 상세](../book.html#T03/start)<br>[필터 해제](../book.html#T01/list)<br>[필터 변경](../book.html#T01/filter) | — |

## T02 · 시간 예약

**사용자** 여행자　 **진입** 장소 상세 → 예약

**완료 기준** 인원과 시간을 선택하고 재고가 소진되면 다른 시간을 고른다.

[P09.R2](../../products/p09-travel-ai/prd.md) · [P09.UC1](../../products/p09-travel-ai/use-cases.md) · [P02.UC4](../../products/p02-user-app/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [예약 선택](../book.html#T02/start) | **인원과 시간을 선택하세요**<br>실시간 재고와 취소 조건을 다음 단계에서 확인합니다. | 입력 · 날짜 — 11월 21일<br>입력 · 시간 — 12:00<br>입력 · 인원 — 2명<br>장소 — 여울 커피 성수점<br>날짜·시각 — 11월 21일 12:00<br>인원 — 2명 | [조건 확인](../book.html#T04/start)<br>[장소로](../book.html#T03/start) | [선택한 마지막 자리가 다른 예약에 확보됨](../book.html#T02/soldout) |
| [재고 소진](../book.html#T02/soldout) | **선택한 시간이 마감됐어요**<br>다른 시각을 선택하고 조건을 다시 확인하세요. | 가능한 대안 — 12:30 | [다른 시간 선택](../book.html#T02/start) | — |

## T03 · 장소 상세

**사용자** 여행자　 **진입** 여행 탐색 결과

**완료 기준** 같은 장소에서 예약과 좌석 없는 픽업을 각각 시작한다.

[P09.R1](../../products/p09-travel-ai/prd.md) · [P09.R2](../../products/p09-travel-ai/prd.md) · [P09.UC1](../../products/p09-travel-ai/use-cases.md) · [P09.UC4](../../products/p09-travel-ai/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [장소](../book.html#T03/start) | **여울 커피 성수점**<br>장소와 가맹 지점 정보를 확인하세요. | 장소 유형·ID — 맛집 · PLACE-01<br>가맹 지점 — 여울 커피 성수점 · 연결됨<br>영업 — 10:00–19:00<br>갱신 — 오늘 10:00<br>예약 — 12:00 가능 | [예약하기](../book.html#T02/start)<br>[좌석 없이 미리 주문](../book.html#T06/start)<br>[후기와 자격](../book.html#T08/start)<br>[돌아가기](../book.html#T01/start) | — |
| [명소](../book.html#T03/attraction) | **서울숲**<br>예약 가맹점이 아닌 명소입니다. 장소 정보만 확인할 수 있습니다. | 장소 유형·ID — 명소 · PLACE-02<br>가맹 지점 — 연결 없음<br>개방 — 05:30–21:30 · 예시<br>갱신 — 오늘 10:00 | [목록으로](../book.html#T01/list)<br>[지역 검색](../book.html#T01/manual) | — |

## T04 · 예약금 지급 검토

**사용자** 여행자　 **진입** 시간 선택 → 조건 확인

**완료 기준** 총액 30과 이번 예약금 10, 별도 잔금 20 및 취소 조건을 구분한다.

[P09.R2](../../products/p09-travel-ai/prd.md) · [P09.UC1](../../products/p09-travel-ai/use-cases.md) · [P02.R6](../../products/p02-user-app/prd.md) · [P02.UC4](../../products/p02-user-app/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [예약 조건](../book.html#T04/start) | **예약금 10 TestUSDC 검토**<br>보류한 좌석의 원조건입니다. 예약금 지급을 시작하거나 기존 예약금 주문을 다시 확인할 수 있습니다. | 총액 — 30 TestUSDC<br>이번 예약금 — 10 TestUSDC<br>별도 잔금 — 20 TestUSDC<br>취소 조건 — 전날 18:00까지 예약금 전액 환불 신청 가능 · 예시<br>네트워크 — StableNet 테스트넷 8283<br>조건 버전 — 예약 정책 v3 · 가격 v2<br>노쇼 — 이용 시각까지 미방문 시 예약금 환불 불가 · 예시 | [원예약의 예약금 주문·서명 견적 검토](../book.html#T04/board)<br>[시간 변경](../book.html#T02/start) | — |
| [기기 승인 대기](../book.html#T04/board) | **보드에서 확인하세요**<br>기기가 검증한 수취인·금액·네트워크와 수수료 부담을 대조한 뒤 승인하거나 거절하세요. 승인 후 앱에서 제출합니다. | 이번 지급 — 10 TestUSDC<br>상태 — 보드 승인 대기<br>수취 주소 — 0x62c9…8e41 (화면에서 합성값 전체 표시)<br>네트워크 — StableNet 테스트넷 8283<br>가스 부담 — 매장 부담 · 고객 가스 0 | [같은 예약금 주문 확인](../book.html#T04/start)<br>[주소 전체 보기](../book.html#T04/address) | [보드 거절 · 원예약에서 같은 주문 확인](../book.html#T04/start)<br>[제출 전 견적 만료 · 원주문 기한 확인](../book.html#T04/start)<br>[승인 후 명시적 제출 · 접수 또는 응답 유실](../book.html#T05/pending) |
| [전체 주소](../book.html#T04/address) | **수취 주소 확인**<br>기기가 산출한 실제 주소를 전체 표시하는 자리입니다. | 수취 주소 — 0x62c9…8e41 (화면에서 합성값 전체 표시) | [돌아가기](../book.html#T04/board) | — |

## T05 · 예약 상세와 잔금

**사용자** 여행자　 **진입** 결제 탭 또는 예약금 검토

**완료 기준** 확정된 예약금과 잔금을 별도 영수증으로 추적하고 미확정 시 재결제를 막는다.

[P09.R2](../../products/p09-travel-ai/prd.md) · [P09.UC1](../../products/p09-travel-ai/use-cases.md) · [P02.UC4](../../products/p02-user-app/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [예약 상세](../book.html#T05/start) | **예약금 확인됨**<br>잔금은 별도 주문이며 사용자가 따로 승인합니다. | 예약 — 여울 커피 · 11월 21일 12:00 · 2명<br>총액 — 30 TestUSDC<br>예약금 — 10 TestUSDC · 확정<br>잔금 — 20 TestUSDC · 미결 | [잔금 검토](../book.html#T05/balance)<br>[취소와 환불](../book.html#T07/start)<br>[여행 홈](../book.html#T01/start) | — |
| [확인 중](../book.html#T05/pending) | **같은 예약을 확인하고 있어요**<br>제출한 예약금의 확정 여부를 같은 주문에서 조회합니다. 응답을 잃어도 새 주문을 만들지 않습니다. | 예약금 — 10 TestUSDC · 확인 중<br>마지막 조회 — 10:42 | [상태 다시 조회](../book.html#T05/pending)<br>[여행 홈](../book.html#T01/start) | [예약금 확정 증거 수신](../book.html#T05/start)<br>[원주문 미지급 확인 · 좌석 보류 유효](../book.html#T04/start)<br>[미지급과 좌석 보류 종료 확인 · 새 시간 선택 가능](../book.html#T02/start) |
| [잔금 검토](../book.html#T05/balance) | **잔금 20 TestUSDC**<br>예약금 확정 뒤 별도 잔금 원주문의 수취인·전체 주소·네트워크·가스 부담을 확인합니다. 보드 승인 후 앱에서 이 잔금을 제출하세요. | 예약 총액 — 30 TestUSDC<br>기확정 예약금 — 10 TestUSDC<br>이번 잔금 — 20 TestUSDC<br>네트워크 — StableNet 테스트넷 8283<br>수취 주소 — 0x62c9…8e41 (화면에서 합성값 전체 표시)<br>가스 부담 — 매장 부담 · 고객 가스 0 | [같은 잔금의 보드 승인 요청](../book.html#T05/balance)<br>[예약 상세](../book.html#T05/start) | [보드 거절 또는 제출 전 견적 만료 · 같은 잔금 확인](../book.html#T05/balance)<br>[별도 승인 후 명시적 제출 · 접수 또는 응답 유실](../book.html#T05/balance_pending) |
| [잔금 확인 중](../book.html#T05/balance_pending) | **잔금 상태를 조회합니다**<br>보드 승인과 거래 확정은 별개입니다. 동일 주문만 재조회합니다. | 잔금 — 20 TestUSDC · 확인 중 | [다시 조회](../book.html#T05/balance_pending) | [잔금 확정 증거 수신](../book.html#T05/paid) |
| [정산 완료](../book.html#T05/paid) | **예약금과 잔금이 확인됐어요**<br>각 지급의 영수증은 같은 예약에서 확인합니다. | 예약금 — 10 TestUSDC · 확정<br>잔금 — 20 TestUSDC · 확정<br>누계 — 30 TestUSDC | [예약 및 영수증](../book.html#T05/paid)<br>[완납 예약 취소·환불](../book.html#T07/paid_booking) | — |

## T06 · 좌석 없는 미리 주문

**사용자** 여행자　 **진입** 장소 상세 → 미리 주문

**완료 기준** 좌석 예약 없이 옵션·재고·가격·픽업 시간을 검토하고, 원주문 지급 확정과 매장 이행 접수를 모두 확인한 뒤 동일한 픽업 번호를 표시한다.

[P09.R2](../../products/p09-travel-ai/prd.md) · [P09.UC4](../../products/p09-travel-ai/use-cases.md) · [P02.R6](../../products/p02-user-app/prd.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [메뉴와 픽업](../book.html#T06/start) | **픽업할 메뉴를 고르세요**<br>좌석 예약은 만들지 않습니다. | 입력 · 수량 — 1<br>입력 · 픽업 시간 — 12:30<br>지점 — 여울 커피 성수점<br>메뉴 — 아이스 아메리카노<br>픽업 — 12:30 | [재고와 가격 확인](../book.html#T06/review)<br>[장소로](../book.html#T03/start) | — |
| [주문 검토](../book.html#T06/review) | **픽업 주문을 확인하세요**<br>접수 전 메뉴 버전·옵션·가격 또는 재고가 바뀌면 새 조건을 확인하세요. | 메뉴 — 아이스 아메리카노<br>합계 — 3.50 TestUSDC<br>픽업 — 12:30 | [보드 승인 요청](../book.html#T06/pending)<br>[수정](../book.html#T06/start) | [주문 접수 전 조건 변경](../book.html#T06/changed) |
| [확인 중](../book.html#T06/pending) | **픽업 주문 확인 중**<br>원주문을 다시 확인합니다. 지급 확정과 매장 접수가 모두 확인되면 픽업 번호가 표시됩니다. | 주문 — 확인 중<br>픽업 번호 — 아직 없음 | [주문 재조회](../book.html#T06/pending) | [지급 확정 및 이행 접수](../book.html#T06/pickup)<br>[원요청 미접수 확인 후 변경된 조건 수신](../book.html#T06/changed)<br>[재조회에서 지급·매장 접수와 이후 이행 불가 확인](../book.html#T07/pickup) |
| [변경 확인](../book.html#T06/changed) | **주문 조건이 변경됐어요**<br>현재 옵션·가격과 재고를 확인하세요. 선택을 확인한 뒤 새 주문을 요청할 수 있습니다. | 상태 — 선택한 옵션 재확인 필요 | [다시 선택](../book.html#T06/start) | — |
| [픽업](../book.html#T06/pickup) | **픽업 번호 B08**<br>원주문 지급과 매장 접수가 확인됐습니다. 준비가 끝나면 같은 번호로 알려드립니다. | 지급 — 확정<br>픽업 — B08 · 준비 중 | [환불 상태](../book.html#T07/pickup)<br>[여행 홈](../book.html#T01/start) | [매장 준비 완료](../book.html#T06/ready)<br>[이행 불가 통지](../book.html#T07/pickup) |
| [준비 완료](../book.html#T06/ready) | **픽업할 수 있어요**<br>같은 주문 번호를 매장에 보여 주세요. | 픽업 — B08 · 준비 완료 | [픽업 상세](../book.html#T06/ready) | — |
| [환불된 픽업](../book.html#T06/refunded) | **픽업 주문이 취소됐어요**<br>이행 불가로 취소되었으며 환불 지급이 확정됐습니다. | 원주문 — P-030 · B08<br>환불 — 3.50 TestUSDC · 확정<br>픽업 — 취소됨 | [환불 증거](../book.html#T07/pickup_done)<br>[여행 홈](../book.html#T01/start) | — |

## T07 · 취소와 환불

**사용자** 여행자　 **진입** 예약 또는 픽업 상세

**완료 기준** 취소와 별도 환불 지급의 신청·대기·확정 및 원주문을 구분한다.

[P09.R2](../../products/p09-travel-ai/prd.md) · [P09.UC4](../../products/p09-travel-ai/use-cases.md) · [P02.UC4](../../products/p02-user-app/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [취소·환불](../book.html#T07/start) | **원주문과 환불을 확인하세요**<br>취소가 곧 환불 지급 확정은 아닙니다. | 원주문 — RSV-030 · 예약금 DEP-030<br>원금 — 10 TestUSDC<br>취소 — 요청 가능<br>환불 — 미신청<br>취소 기한 — 이용 전날 18:00 · 정책 v3<br>노쇼 — 예약금 환불 불가 · 예시 | [취소 요청](../book.html#T07/pending)<br>[예약 상세](../book.html#T05/start) | — |
| [환불 대기](../book.html#T07/pending) | **처리 상태를 확인하고 있어요**<br>이행 불가나 취소 후 별도 환불 재원 지급을 원주문에서 추적합니다. | 원주문 — RSV-030 · DEP-030<br>환불 요청 — 10 TestUSDC · 확인 중<br>취소 — 접수 확인 중 | [상태 다시 조회](../book.html#T07/pending) | [환불 지급 증거 확인](../book.html#T07/done) |
| [환불 확인](../book.html#T07/done) | **환불 지급이 확인됐어요**<br>원주문과 별도의 환불 증거를 표시합니다. | 원주문 — RSV-030 · DEP-030<br>환불 — 10 TestUSDC · 지급 확정 | [여행 홈](../book.html#T01/start) | — |
| [픽업 환불](../book.html#T07/pickup) | **픽업 주문의 환불 상태**<br>이행 불가 또는 취소와 별도 환불 지급 상태를 원주문에서 조회합니다. | 원주문 — 픽업 P-030 · B08<br>원금 — 3.50 TestUSDC<br>환불 — 대기 | [같은 환불 조회](../book.html#T07/pickup)<br>[픽업으로](../book.html#T06/pickup) | [픽업 환불 지급 증거 확인](../book.html#T07/pickup_done) |
| [픽업 환불 확정](../book.html#T07/pickup_done) | **픽업 환불이 확인됐어요**<br>원주문에 연결된 별도 지급 증거입니다. | 원주문 — 픽업 P-030 · B08<br>환불 — 3.50 TestUSDC · 지급 확정 | [환불된 픽업 상세](../book.html#T06/refunded) | — |
| [완납 예약 환불](../book.html#T07/paid_booking) | **완납 예약의 취소 조건**<br>예약금과 잔금은 별도 원주문이며 조건에 따라 각각 환불합니다. 예시 정책: 전날 18:00까지 전액 환불 신청 가능. | 예약 — RSV-030 · 총 30 TestUSDC<br>예약금 DEP-030 — 10 TestUSDC · 지급 확정<br>잔금 BAL-030 — 20 TestUSDC · 지급 확정<br>예상 환불 — 각 원주문에 10 + 20 TestUSDC<br>조건 식별 — 예약 정책 v3 · 가격 v2<br>노쇼 — 예약금 환불 불가 · 잔금 환불은 정책 검토 | [취소와 환불 신청](../book.html#T07/paid_pending)<br>[완납 예약 상세](../book.html#T05/paid) | — |
| [완납 환불 대기](../book.html#T07/paid_pending) | **예약 취소와 환불을 조회합니다**<br>요청 접수와 각 원주문의 환불 지급은 별도입니다. 확인 전 기존 요청을 조회합니다. | 예약 — RSV-030<br>DEP-030 — 10 TestUSDC · 환불 확인 중<br>BAL-030 — 20 TestUSDC · 환불 확인 중 | [같은 요청 조회](../book.html#T07/paid_pending) | [예약금 환불만 확정](../book.html#T07/paid_partial)<br>[두 환불 지급 모두 확정](../book.html#T07/paid_done) |
| [부분 환불](../book.html#T07/paid_partial) | **예약금 환불이 확인됐어요**<br>잔금 환불은 기존 요청을 계속 조회합니다. | DEP-030 — 10 TestUSDC · 환불 확정<br>BAL-030 — 20 TestUSDC · 확인 중 | [같은 요청 조회](../book.html#T07/paid_partial) | [잔금 환불 지급 확정](../book.html#T07/paid_done) |
| [완납 환불 완료](../book.html#T07/paid_done) | **예약 취소와 환불이 확인됐어요**<br>각 원주문의 별도 환불 증거가 연결되었습니다. | DEP-030 — 10 TestUSDC · 환불 확정<br>BAL-030 — 20 TestUSDC · 환불 확정<br>합계 — 30 TestUSDC | [환불 결과](../book.html#T07/paid_done)<br>[여행 홈](../book.html#T01/start) | — |

## T08 · 후기와 이용 자격

**사용자** 여행자　 **진입** 장소 또는 내 여행

**완료 기준** 테스트 후기와 실제 지급·이행 근거가 있는 실구매 배지를 구별하고 공개 동의를 받는다.

[P09.R3](../../products/p09-travel-ai/prd.md) · [P09.UC2](../../products/p09-travel-ai/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [후기 자격](../book.html#T08/start) | **후기 작성 자격 확인**<br>실구매 배지에는 실제 지급과 이용·이행 근거가 모두 필요합니다. | 영수증 — 테스트넷 · 본인 확인<br>후기 — 테스트 후기 가능<br>실구매 배지 — 해당 없음 | [후기 작성](../book.html#T08/write)<br>[스탬프](../book.html#T09/start)<br>[장소](../book.html#T03/start) | — |
| [후기 작성](../book.html#T08/write) | **공개할 내용을 확인하세요**<br>공개 후기에는 지갑 주소가 포함되지 않습니다. | 입력 · 후기 — 조용하고 편안했어요<br>구분 — 테스트 후기<br>공개 — 명시적 동의 필요 | [공개 동의 후 게시](../book.html#T08/posted)<br>[취소](../book.html#T08/start) | — |
| [게시됨](../book.html#T08/posted) | **후기가 게시됐어요**<br>중복 제출은 기존 결과를 반환합니다. | 후기 — 테스트 후기 · 1건<br>실구매 배지 — 없음 | [스탬프](../book.html#T09/start)<br>[내 여행](../book.html#T10/start) | — |

## T09 · 스탬프와 혜택

**사용자** 여행자　 **진입** 후기 또는 내 여행

**완료 기준** 혜택 확정 전 보류와 환불·재구성에 따른 조정 이력을 표시한다.

[P09.R3](../../products/p09-travel-ai/prd.md) · [P09.UC2](../../products/p09-travel-ai/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [진척](../book.html#T09/start) | **방문 근거를 확인합니다**<br>테스트넷 영수증으로 실제 혜택을 제공하지 않습니다. | 스탬프 — 검증 대기<br>혜택 — 미확정 | [상태 조회](../book.html#T09/start)<br>[내 여행](../book.html#T10/start) | [검증된 혜택 원장 결과](../book.html#T09/confirmed)<br>[환불 또는 체인 재구성](../book.html#T09/adjusted) |
| [확정](../book.html#T09/confirmed) | **자격과 혜택을 확인했어요**<br>동일 근거는 한 번만 반영됩니다. | 스탬프 — 1건<br>혜택 — 원장 확정 | [내 여행](../book.html#T10/start) | — |
| [조정 이력](../book.html#T09/adjusted) | **방문 근거가 변경됐어요**<br>배지와 혜택만 보류 또는 조정하며 후기와 별도 동의 기록은 독립 관리합니다. | 사유 — 환불 또는 체인 재구성<br>혜택 — 조정 이력 확인 | [후기](../book.html#T08/start) | — |

## T10 · 내 여행

**사용자** 여행자　 **진입** 여행 홈 → 내 여행

**완료 기준** 후기·기록·동의와 코스에 독립 진입한다.

[P09.R3](../../products/p09-travel-ai/prd.md) · [P09.R4](../../products/p09-travel-ai/prd.md) · [P09.R5](../../products/p09-travel-ai/prd.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [내 여행](../book.html#T10/start) | **나의 여행**<br>선택형 기록과 추천 동의 없이도 장소 탐색은 가능합니다. | 예약 — 예약 상세에서 확인<br>기록 — 선택 사항 | [예약](../book.html#T05/start)<br>[후기](../book.html#T08/start)<br>[스탬프](../book.html#T09/start)<br>[기록과 동의](../book.html#T11/start)<br>[AI 코스](../book.html#T13/start)<br>[챌린지](../book.html#T14/start)<br>[탐색](../book.html#T01/start) | — |

## T11 · 기록과 동의

**사용자** 여행자　 **진입** 내 여행 → 기록과 동의

**완료 기준** 위치·녹음·전사·추천 제공 목적별 동의와 보유·제공자를 각각 확인하고 철회한다.

[P09.R4](../../products/p09-travel-ai/prd.md) · [P09.UC3](../../products/p09-travel-ai/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [동의 관리](../book.html#T11/start) | **기록할 항목을 직접 고르세요**<br>목적별 선택이며 보유 기간과 제공자 조건을 확인합니다. | 위치 발자취 — 꺼짐 · 예시 30일 · 기기/서버<br>음성 녹음 — 꺼짐 · 예시 7일 · 기기/서버<br>전사·요약 — 꺼짐 · 예시 30일 · 서버<br>외부 추천 제공 — 꺼짐 · 예시 14일 · 여행 AI 제공자 | [동의 설정](../book.html#T11/choices)<br>[삭제 요청](../book.html#T12/start)<br>[내 여행](../book.html#T10/start) | — |
| [목적별 선택](../book.html#T11/choices) | **각 목적을 따로 선택하세요**<br>예시로 위치만 선택해 저장합니다. 각 목적의 동의는 별도로 기록합니다. | 입력 · 위치 동의 — 예<br>입력 · 음성 동의 — 아니요<br>입력 · 전사 동의 — 아니요<br>입력 · 추천 동의 — 아니요<br>위치 — 선택<br>음성 — 선택<br>전사·요약 — 선택<br>추천 제공 — 선택 | [위치 동의 저장](../book.html#T11/location_on)<br>[위치 동의 철회](../book.html#T11/withdrawn) | — |
| [철회됨](../book.html#T11/withdrawn) | **선택한 동의를 철회했어요**<br>진행 중 작업은 저장 전에 동의를 다시 확인합니다. | 철회 범위 — 위치 발자취<br>신규 위치 수집 — 중단<br>다른 동의 — 변경 없음<br>기존 데이터 — 삭제 요청 별도 | [삭제 요청](../book.html#T12/start)<br>[동의 관리](../book.html#T11/start) | — |
| [위치 동의 저장](../book.html#T11/location_on) | **위치 기록을 선택했어요**<br>음성·전사·추천은 동의하지 않았습니다. 언제든 목적별로 철회할 수 있습니다. | 위치 — 동의 · 예시 30일<br>음성·전사·추천 — 꺼짐 | [선택 변경](../book.html#T11/choices)<br>[내 여행](../book.html#T10/start) | — |
| [추천 철회](../book.html#T11/recommendation_withdrawn) | **추천 제공 동의를 철회했어요**<br>신규 추천 전송과 진행 중 결과의 저장·노출을 중단합니다. 다른 동의는 그대로입니다. | 철회 범위 — 외부 추천 제공<br>추천 생성 — 중단<br>기존 데이터 — 삭제 요청 별도<br>위치 발자취 — 기존 동의와 기록은 독립 유지 | [삭제 요청](../book.html#T12/start)<br>[동의 관리](../book.html#T11/start) | — |

## T12 · 기록 삭제

**사용자** 여행자　 **진입** 동의 관리 → 삭제 요청

**완료 기준** 기기·서버·제공자와 원본·전사·파생물·대기열의 삭제 결과를 확인한다.

[P09.R4](../../products/p09-travel-ai/prd.md) · [P09.UC3](../../products/p09-travel-ai/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [삭제 범위](../book.html#T12/start) | **삭제할 기록을 확인하세요**<br>선택한 기록을삭제합니다. 위치 기록은 별도 선택이며, 거래 영수증과 업무상 보존 증거는 별도입니다. | 대상 — 위치·음성 원본·전사·추천 파생물·대기 작업<br>처리 위치 — 기기·서버·외부 제공자 | [삭제 요청](../book.html#T12/pending)<br>[돌아가기](../book.html#T11/start) | — |
| [삭제 대기](../book.html#T12/pending) | **삭제 요청을 처리하고 있어요**<br>제공자 응답 지연 시 대기 상태를 유지하고 재시도합니다. | 기기 — 삭제 확인<br>서버·대기열 — 처리 중<br>외부 제공자 — 여행 AI · 삭제 응답 대기 | [상태 조회](../book.html#T12/pending) | [모든 대상 삭제 확인](../book.html#T12/done) |
| [삭제 완료](../book.html#T12/done) | **삭제 결과를 확인했어요**<br>기기·서버·제공자 결과와 삭제 영수증을 보관합니다. | 원본·전사·파생물·대기열 — 삭제 확인<br>거래 영수증 — 별도 보존 | [동의 관리](../book.html#T11/start) | — |

## T13 · AI 코스

**사용자** 여행자　 **진입** 내 여행 → AI 코스

**완료 기준** 출처와 휴무·이동·예산을 검토하고 버전 충돌은 비교 후 직접 적용한다.

[P09.R4](../../products/p09-travel-ai/prd.md) · [P09.R5](../../products/p09-travel-ai/prd.md) · [P09.UC5](../../products/p09-travel-ai/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [코스 요청](../book.html#T13/start) | **원하는 여행을 알려주세요**<br>사용자가 허용한 입력만 사용하며 생성만으로 예약하거나 지출하지 않습니다. | 입력 · 날짜 — 11월 21일<br>입력 · 예산 — 30<br>외부 추천 동의 — 확인 필요<br>예산 — 30 TestUSDC 예시 | [동의 확인](../book.html#T13/consent)<br>[동의 관리](../book.html#T11/start) | — |
| [후보 검토](../book.html#T13/result) | **추천 후보를 확인하세요**<br>출처와 검증 시각을 확인하고 미확인 항목은 직접 검토하세요. | 여울 커피 성수점 — 11:00 영업 확인 · 출처 매장 정보 v3 / 10:00<br>서울숲 — 12:00 방문 · 여울 커피에서 도보 20분 · 출처 장소 정보 v2<br>예산 — 커피 3.50 + 이동 0 = 3.50 TestUSDC 예시<br>미확인 후보 — 장소 B 휴무 여부 확인 필요 · 예산 초과 후보 별도 표시<br>기준 일정 — 버전 4 · 생성 시작 시점 | [일정에 적용](../book.html#T13/conflict)<br>[예약 따로 시작](../book.html#T03/start)<br>[다시 작성](../book.html#T13/start) | — |
| [일정 비교](../book.html#T13/conflict) | **일정 변경 사항을 비교하세요**<br>생성 중 수정한 일정은 덮어쓰지 않습니다. 선택한 후보만 직접 적용합니다. | 기존 일정 — 수정된 버전<br>추천 — 선택 적용 대기 | [선택한 후보 적용](../book.html#T13/applied)<br>[돌아가기](../book.html#T13/result) | — |
| [적용됨](../book.html#T13/applied) | **선택한 코스를 일정에 담았어요**<br>예약과 결제는 각각 별도로 시작합니다. | 일정 — 선택한 버전 저장 | [챌린지](../book.html#T14/start)<br>[내 여행](../book.html#T10/start) | — |
| [추천 동의](../book.html#T13/consent) | **추천 제공 동의를 확인하세요**<br>제공자와 보유·삭제 조건을 확인하고 동의한 경우에만 생성을 요청합니다. | 제공자 — 여행 AI · 예시<br>입력 — 선택한 날짜·예산<br>보유 — 예시 14일 · 삭제 요청 가능 | [동의하고 생성 요청](../book.html#T13/generating)<br>[동의하지 않음](../book.html#T13/start) | — |
| [생성 중](../book.html#T13/generating) | **추천을 생성하고 있어요**<br>원격 생성 중 동의가 철회되면 저장과 노출을 중단합니다. | 동의 — 생성 전 확인됨<br>예약·지출 — 없음 | [동의 관리](../book.html#T11/start) | [동의 유효 및 후보 검증 결과 수신](../book.html#T13/result)<br>[추천 동의 철회](../book.html#T11/recommendation_withdrawn) |

## T14 · 따라하기 챌린지

**사용자** 여행자　 **진입** 내 여행 또는 코스

**완료 기준** 방문 근거에 동의하고 검증된 방문만 진행률에 반영한다.

[P09.R3](../../products/p09-travel-ai/prd.md) · [P09.R5](../../products/p09-travel-ai/prd.md) · [P09.UC5](../../products/p09-travel-ai/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [챌린지](../book.html#T14/start) | **코스를 따라가 보세요**<br>방문 근거 제공은 선택 사항이며 미검증 방문에는 보상을 주지 않습니다. | 진척 — 0 / 3<br>방문 근거 — 동의 필요 | [방문 근거 선택](../book.html#T14/consent)<br>[스탬프](../book.html#T09/start)<br>[내 여행](../book.html#T10/start) | — |
| [방문 근거](../book.html#T14/consent) | **방문 근거 제공을 선택하세요**<br>목적과 보유 기간을 확인하고 동의한 근거만 검증합니다. | 목적 — 챌린지 진행 확인<br>진행 — 검증 전 반영 안 됨 | [동의하고 참여](../book.html#T14/progress)<br>[취소](../book.html#T14/start) | — |
| [진행](../book.html#T14/progress) | **검증된 방문을 기다립니다**<br>확인된 근거만 반영하고 혜택 원장 확정은 별도입니다. | 검증된 방문 — 0 / 3<br>혜택 — 없음 | [스탬프 상태](../book.html#T09/start) | [방문 근거 검증](../book.html#T14/verified) |
| [검증됨](../book.html#T14/verified) | **방문 한 곳을 확인했어요**<br>중복 근거는 추가 진척을 만들지 않습니다. | 검증된 방문 — 1 / 3 | [스탬프 상태](../book.html#T09/start) | — |


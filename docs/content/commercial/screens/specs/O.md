# 가맹점 운영 · 페이지별 설계

[전체 화면](../book.html) · [원 요구사항 대조표](../coverage.md) · [통합 감사](../audit.md)

제품에서 누르는 동작과 상태를 바꾸는 외부 사건을 열로 구분했다. 입력은 예시 데이터이며 실제 제품의 인수에서는 진입 조건과 검증 결과까지 함께 확인한다.

## O01 · 지점 정산과 마감

**제품** 운영·정산 시스템　 **사용자** 점주·지점 재무 담당자　 **진입** 인증 후 허용된 지점과 영업일 선택

**완료 기준** 필터와 CSV 범위가 일치하고 환불은 발생일로 집계하며 대사 차이 0.00인 범위만 마감한다.

[P04.R6](../../products/p04-merchant-kiosk/prd.md) · [P04.UC4](../../products/p04-merchant-kiosk/use-cases.md) · [P05.R5](../../products/p05-operations-backoffice/prd.md) · [P05.R6](../../products/p05-operations-backoffice/prd.md) · [P05.UC4](../../products/p05-operations-backoffice/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [영업일](../book.html#O01/start) | **성수점 정산**<br>영업일은 지점 시간대로 집계합니다. 환불의 원 매출 날짜는 주문 상세에서 확인할 수 있습니다. | 입력 · 허용된 지점 — 여울 커피 성수점<br>입력 · 영업일 — 2026-11-20<br>입력 · 자산 — TestUSDC<br>지점 — 여울 커피 성수점<br>영업일 — 2026-11-20 · Asia/Seoul<br>확정 매출 — 22.00 TestUSDC<br>확정 환불 — 1.50 TestUSDC<br>순매출 — 20.50 TestUSDC<br>대사 차이 — 0.00 TestUSDC | [S-0142 상세](../book.html#O02)<br>[마감 검토](../book.html#O01/close)<br>[운영 업무 열기](../book.html#O12) | [허용되지 않은 지점을 요청함](../book.html#O01/denied)<br>[장부와 체인 차이 감지, 대사 권한 확인](../book.html#O04)<br>[환불 승인 또는 재원 역할 확인](../book.html#O06) |
| [마감 검토](../book.html#O01/close) | **성수점 마감 검토**<br>지점, 영업일, 자산 및 등록 주소의 실제 입출금을 확인하세요. | 등록 주소 입금 — 22.00 TestUSDC<br>별도 환불 재원 출금 — 1.50 TestUSDC<br>대사 차이 — 0.00 TestUSDC<br>미해결 예외 — 0건 | [이 범위 CSV 요청](../book.html#O01/csv)<br>[마감 요청](../book.html#O01/closing)<br>[정산으로 돌아가기](../book.html#O01) | — |
| [내보내기](../book.html#O01/csv) | **정산 CSV 생성 상태**<br>화면과 동일한 지점·영업일·토큰 범위로 내보냅니다. | 대상 — 성수점 · 2026-11-20 · TestUSDC<br>기록 — 담당자와 내보내기 범위 | [마감 검토로 돌아가기](../book.html#O01/close) | — |
| [동시성 검증](../book.html#O01/closing) | **마감 조건을 다시 확인합니다**<br>요청 시점의 최신 차이와 권한을 확인하고 결과를 같은 마감 ID에 기록합니다. | 마감 조회 ID — CLS-1120 | [같은 마감 조회](../book.html#O01/closing) | [최신 차이와 권한 검증 뒤 마감 기록 완료](../book.html#O01/closed)<br>[새 차이 발생으로 보류](../book.html#O04)<br>[권한 확인 실패](../book.html#O01/denied) |
| [마감 완료](../book.html#O01/closed) | **성수점 11월 20일 마감**<br>추가 증거는 조정 이력과 함께 조회할 수 있습니다. | 마감 — CLS-1120<br>순매출 — 20.50 TestUSDC | [정산으로 돌아가기](../book.html#O01) | — |
| [조회 제한](../book.html#O01/denied) | **선택한 범위를 볼 수 없습니다**<br>계정에 배정된 지점에서 다시 선택하세요. | 처리 — 조회·내보내기 거절 | [허용된 지점으로](../book.html#O01) | — |

## O02 · 지점 원주문과 이행

**제품** 운영·정산 시스템　 **사용자** 점주·지점 운영자　 **진입** 해당 지점 지급 목록에서 주문 선택

**완료 기준** 확정된 지급, 회계, 이행 상태를 함께 조회하고 S-0142 환불 신청은 별도 거래로 추적한다.

[P04.R4](../../products/p04-merchant-kiosk/prd.md) · [P04.R6](../../products/p04-merchant-kiosk/prd.md) · [P04.UC3](../../products/p04-merchant-kiosk/use-cases.md) · [P04.UC4](../../products/p04-merchant-kiosk/use-cases.md) · [P05.R3](../../products/p05-operations-backoffice/prd.md) · [P05.R5](../../products/p05-operations-backoffice/prd.md) · [P05.R6](../../products/p05-operations-backoffice/prd.md) · [P05.UC3](../../products/p05-operations-backoffice/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [원주문](../book.html#O02/start) | **S-0142 결제 상세**<br>결제 확정 시각과 픽업 상태를 비교하세요. 환불도 이 주문에서 조회할 수 있습니다. | 원금 — 6.50 TestUSDC<br>등록 주소 — 주소 버전 v3<br>지급 — 10:42 확정<br>이행 — A17 준비 중 · evt-142<br>확정 환불 — 0.00 TestUSDC | [환불 신청 검토](../book.html#O05)<br>[공개 지급 증거 보기](../book.html#X01)<br>[정산으로](../book.html#O01) | [허용 범위를 벗어난 주문 조회](../book.html#O01/denied) |
| [미확정 주문](../book.html#O02/pending) | **기존 주문을 조회하고 있습니다**<br>저장된 견적, 해시와 관측 높이부터 확인합니다. | 체인 증거 — 확정 대기<br>이행 — 보류<br>새 지급·영수증 — 보류 | [이 주문 다시 조회](../book.html#O02/pending)<br>[조사 큐](../book.html#O04) | [실제 입금 증거 확정 및 P05 회계·이행 통지 커밋](../book.html#O02) |

## O03 · 메뉴와 가격 게시

**제품** 운영·정산 시스템　 **사용자** 지점 카탈로그 담당자　 **진입** 지점 운영 업무에서 카탈로그 선택

**완료 기준** 편집 버전이 충돌하면 비교를 허용하고 기존 확정 견적은 기존 가격을 보존한다.

[P05.R1](../../products/p05-operations-backoffice/prd.md) · [P05.R2](../../products/p05-operations-backoffice/prd.md) · [P05.R6](../../products/p05-operations-backoffice/prd.md) · [P05.UC1](../../products/p05-operations-backoffice/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [메뉴 편집](../book.html#O03/start) | **성수점 메뉴 편집본**<br>필수 옵션, 세금·할인, 재고와 운영 시간을 검증합니다. | 입력 · 크루아상 가격 — 3.50<br>입력 · 세금·할인 — 0.00<br>입력 · 판매 재고 — 12<br>입력 · 운영 시간 — 10:00–19:00<br>입력 · 온도 옵션 — 필수<br>현재 게시 — v1<br>작업본 — v2<br>기존 크루아상 — 3.00 TestUSDC | [검증한 게시 내용 보기](../book.html#O03/review)<br>[운영 업무로](../book.html#O12) | [입력 범위가 잘못됨](../book.html#O03/invalid) |
| [게시 검토](../book.html#O03/review) | **메뉴 v2 게시 검토**<br>신규 손님과 확정 전 견적에 적용합니다. 가격이 달라진 미리보기는 고객 확인을 다시 받습니다. | 가격 차이 — 크루아상 +0.50 TestUSDC<br>발급 완료된 견적 — 발급가 유지 | [게시 요청](../book.html#O03/publishing)<br>[계속 편집](../book.html#O03) | [다른 담당자가 기준 버전을 변경](../book.html#O03/conflict) |
| [게시 처리](../book.html#O03/publishing) | **게시 결과를 확인하고 있습니다**<br>서버에서 같은 편집본의 검증과 게시 버전을 조회합니다. | 편집본 — v2 | [게시 결과 다시 확인](../book.html#O03/publishing) | [게시 버전 기록 완료](../book.html#O03/published)<br>[서버가 버전 충돌 확인](../book.html#O03/conflict)<br>[재고·옵션 검증 실패](../book.html#O03/invalid) |
| [게시 완료](../book.html#O03/published) | **메뉴 v2가 게시됐습니다**<br>변경자, 지점, 게시 시각이 감사 내역에 기록됐습니다. | 게시 버전 — v2 | [운영 업무로](../book.html#O12) | — |
| [편집 충돌](../book.html#O03/conflict) | **새로 게시된 버전과 비교하세요**<br>현재 작업본을 보관하고 최신 버전에 합칠 항목을 확인합니다. | 서버 버전 — v3<br>내 편집본 — v2 | [가격과 재고 다시 확인](../book.html#O03) | — |
| [검증 실패](../book.html#O03/invalid) | **편집 항목을 확인하세요**<br>가격, 세금, 할인과 재고는 최소 단위 정수로 저장됩니다. | 오류 항목 — 선택 옵션 또는 금액 단위 | [입력 수정](../book.html#O03) | — |

## O04 · 대사 차이와 이행 재처리

**제품** 운영·정산 시스템　 **사용자** 지점 대사·운영 담당자　 **진입** 영업일 차이 또는 이행 실패 항목

**완료 기준** 미해결 차이는 마감을 막고 같은 eventId를 재전송해도 조리·영수증·POS 통지를 중복 처리하지 않는다.

[P05.R3](../../products/p05-operations-backoffice/prd.md) · [P05.R5](../../products/p05-operations-backoffice/prd.md) · [P05.R6](../../products/p05-operations-backoffice/prd.md) · [P05.UC4](../../products/p05-operations-backoffice/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [조사 대기열](../book.html#O04/start) | **성수점 조사 대기열**<br>차이를 조사하는 동안 이 영업일의 마감이 보류됩니다. | 원장 차이 — 0.50 TestUSDC / 마감 보류<br>수신 실패 — POS webhook evt-142<br>기존 주문 — S-0142 | [대사 증거 비교](../book.html#O04/evidence)<br>[실패 통지 보기](../book.html#O04/outbox)<br>[정산으로](../book.html#O01) | [락 충돌 재시도 예산 소진](../book.html#O04/lookup) |
| [증거 비교](../book.html#O04/evidence) | **지급 증거와 장부 비교**<br>체인 확인 시각과 등록 주소 버전, 장부 반영 시각을 비교합니다. | 현지 영업일 — 2026-11-20<br>등록 버전 — v3<br>관측 상태 — 체인 증거 재조회 필요 | [탐색기에서 증거 검색](../book.html#X02)<br>[같은 사건 재조회](../book.html#O04/lookup) | — |
| [재처리 검토](../book.html#O04/outbox) | **POS 통지 evt-142 재전송**<br>대상 지점과 통지 ID를 확인하세요. 수신자는 같은 eventId를 중복 제거합니다. | 영향 — 성수점 POS 통지 한 건<br>업무 주문 — S-0142<br>조리 지시 — 기존 이행 기록 유지 | [같은 통지 재전송 요청](../book.html#O04/lookup)<br>[취소](../book.html#O04) | — |
| [기존 처리 조회](../book.html#O04/lookup) | **조회 ID로 처리 현황을 확인합니다**<br>락 충돌 시 최대 2초 대기·3회 재시도·8초 누적 예산을 사용합니다. | 조회 ID — REC-1120-A<br>중복 업무 — 기존 ID 유지 | [같은 조회 ID 확인](../book.html#O04/lookup) | [정상화 후 마지막 처리 가능 이벤트부터 60초 내 기록 수렴](../book.html#O04/resolved)<br>[아직 차이 있음](../book.html#O04/evidence) |
| [대사 완료](../book.html#O04/resolved) | **대사 기록이 일치합니다**<br>마감 담당자에게 확인된 범위와 감사 내역을 전달합니다. | 차이 — 0.00 TestUSDC<br>감사 — 조회 ID REC-1120-A | [마감 검토 요청](../book.html#O01/close) | — |

## O05 · 부분 환불 신청

**제품** 운영·정산 시스템　 **사용자** 지점 점주·환불 신청자　 **진입** 확정 원주문의 환불 가능액 조회

**완료 기준** 신청자는 환불을 승인할 수 없고 경쟁 신청이 남은 원금을 넘으면 한도 초과로 거절한다.

[P04.R6](../../products/p04-merchant-kiosk/prd.md) · [P04.UC4](../../products/p04-merchant-kiosk/use-cases.md) · [P05.R5](../../products/p05-operations-backoffice/prd.md) · [P05.UC3](../../products/p05-operations-backoffice/use-cases.md) · [P06.R3](../../products/p06-stablenet-contracts/prd.md) · [P06.UC2](../../products/p06-stablenet-contracts/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [신청 입력](../book.html#O05/start) | **S-0142 환불 신청**<br>원주문과 검증된 수취 경로를 확인하세요. 예시로 지급된 한도의 일부를 신청합니다. | 입력 · 환불 금액 — 1.00 TestUSDC<br>입력 · 사유 — 상품 일부 환불<br>원금 — 6.50 TestUSDC<br>이전 확정 환불 — 0.00 TestUSDC<br>기존 환불 예약 — 0.00 TestUSDC<br>신청 가능액 — 6.50 TestUSDC<br>목적지 — 검증된 원결제 환불 경로 | [신청 내용 검토](../book.html#O05/review)<br>[원주문으로](../book.html#O02) | [대여가 종료되어 목적지 확인 필요](../book.html#O05/destination) |
| [신청 검토](../book.html#O05/review) | **환불 1.00 TestUSDC 신청**<br>접수 시 남은 원금을 다시 확인하고 이 환불 금액을 예약합니다. | 원주문 — S-0142<br>수취 경로 — 원결제 환불 목적지<br>다음 단계 — 재무 승인 | [환불 신청 보내기](../book.html#O05/submitPending)<br>[신청 수정](../book.html#O05) | [동시 신청으로 한도 부족](../book.html#O05/limit) |
| [접수 조회](../book.html#O05/submitPending) | **환불 요청을 확인하고 있습니다**<br>같은 요청에서 원금 예약과 신청 기록이 함께 저장됐는지 확인합니다. | 신청 조회 — RF-0142-01<br>기록 — 검증 대기 | [이 요청 다시 조회](../book.html#O05/submitPending) | [원금 예약과 신청 기록 함께 커밋](../book.html#O05/submitted)<br>[경쟁 신청으로 한도 부족](../book.html#O05/limit)<br>[기록 응답 유실](../book.html#O05/submitPending) |
| [접수 조회](../book.html#O05/submitted) | **환불 신청 결과**<br>승인 및 재원 확인 뒤에 지급이 시작됩니다. | 환불 요청 — RF-0142-01<br>상태 — 승인 대기 | [같은 환불 신청 조회](../book.html#O05/submitted)<br>[원주문으로](../book.html#O02) | [신청 기록이 실제로 확정됨, 승인자 대기열에 전달](../book.html#O06/review) |
| [한도 부족](../book.html#O05/limit) | **남은 환불 금액을 다시 확인하세요**<br>경쟁 신청이 먼저 이 주문의 환불 가능액을 확보했습니다. | 신청 — 저장 거절 · 최신 가능액 조회 후 수정 및 새 검토 | [남은 금액 조회](../book.html#O05) | — |
| [목적지 확인](../book.html#O05/destination) | **종료된 대여의 환불 경로를 확인합니다**<br>대여 전용 계정의 종료 후 검증된 반환 목적지를 대사 담당자가 확인해야 합니다. | 신청 제출 — 보류<br>임의 주소 입력 — 차단 | [원주문으로](../book.html#O02) | [검증된 반환 경로 확인](../book.html#O05) |

## O06 · 환불 승인과 전송

**제품** 운영·정산 시스템　 **사용자** 환불 승인자·환불 재원 운영자　 **진입** 권한에 따라 동일 refundId 검토 또는 전송 조회

**완료 기준** 지점과 현재 환불 권한으로 조회하고 본인 승인과 중복 역할을 제한한다. 같은 원주문을 조회하고 확정 실패 후에만 새 승인과 재원 제출을 진행한다.

[P05.R5](../../products/p05-operations-backoffice/prd.md) · [P05.R6](../../products/p05-operations-backoffice/prd.md) · [P05.UC3](../../products/p05-operations-backoffice/use-cases.md) · [P05.UC4](../../products/p05-operations-backoffice/use-cases.md) · [P06.R3](../../products/p06-stablenet-contracts/prd.md) · [P06.UC2](../../products/p06-stablenet-contracts/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [환불 대기열](../book.html#O06/start) | **성수점 환불 목록**<br>승인 자격과 재원 운영 권한에 따라 가능한 동작이 표시됩니다. | 확정 환불 — 1.50 TestUSDC · 별도 원주문 · 결과 조회<br>승인 대기 — RF-0142-01 · 원주문 S-0142 · 1.00 TestUSDC<br>원금 — 6.50 TestUSDC<br>현재 계정 업무 — 승인 또는 반려 검토 가능 | [RF-0142-01 검토](../book.html#O06/review)<br>[환불 대기열 다시 조회](../book.html#O06/start)<br>[정산으로](../book.html#O01) | — |
| [승인자 검토](../book.html#O06/review) | **RF-0142-01 승인 검토**<br>별도의 승인자가 원결제 경로와 예약 금액을 확인합니다. 이전 시도가 최종 실패한 경우 같은 환불의 새 승인을 기록합니다. | 원주문 — S-0142<br>근거 — 검증된 원결제 목적지<br>이 요청 — 1.00 TestUSDC<br>재원 — 가맹점 환불 전용 계정 | [승인 기록 요청](../book.html#O06/approved)<br>[반려 사유 입력](../book.html#O06/reject) | [신청자 본인 또는 권한 없는 계정 접근](../book.html#O06/denied) |
| [반려 입력](../book.html#O06/reject) | **환불 반려 사유**<br>반려를 감사 이력에 기록하고 예약 해제 결과를 확인합니다. | 입력 · 반려 사유 선택 — 목적지 검증 자료 필요<br>환불 ID — RF-0142-01<br>선택 가능한 사유 — 목적지 검증 자료 필요 · 금액 또는 남은 한도 재확인 · 환불 정책 재확인 | [반려 요청](../book.html#O06/rejected)<br>[검토로 돌아가기](../book.html#O06/review) | — |
| [반려 조회](../book.html#O06/rejected) | **반려 결과를 확인하고 있습니다**<br>반려가 확정되면 신청 내역에 사유를 표시합니다. | 환불 ID — RF-0142-01 | [환불 대기열](../book.html#O06) | — |
| [제출 담당 인계](../book.html#O06/approved) | **환불 승인 상태**<br>같은 환불 ID의 승인 기록을 확인한 재원 운영자만 제출할 수 있습니다. | 지급 — 대기<br>영수증 — 확정 대기 | [환불 목록](../book.html#O06) | [승인 상태 커밋 및 재원 운영자 자격 확인](../book.html#O06/funds)<br>[재원 부족 확인](../book.html#O06/low) |
| [재원 운영자 검토](../book.html#O06/funds) | **환불 재원과 제출 대상 확인**<br>제출 전에 최근 가용 재원 및 승인 기록을 다시 확인합니다. | 환불 ID — RF-0142-01<br>별도 재원 — 가용 5.00 TestUSDC<br>수수료 — 원금과 분리 | [이 환불만 제출 요청](../book.html#O06/pending) | [가용 재원이 부족함](../book.html#O06/low) |
| [재원 부족](../book.html#O06/low) | **승인 환불 지급 대기**<br>재원 확보 후 같은 환불 ID에서 제출을 검토합니다. | 환불 ID — RF-0142-01<br>영수증 — 발급 대기 | [같은 재원 재조회](../book.html#O06/low) | [검증된 환불 재원 확보 및 운영 권한 확인](../book.html#O06/funds) |
| [거래 조회](../book.html#O06/pending) | **환불 전송 결과를 확인합니다**<br>해시 응답을 잃어도 같은 환불 ID의 전송과 확정 결과를 조회합니다. | 환불 ID — RF-0142-01<br>지급 증거 — 확정 대기 | [같은 환불 조회](../book.html#O06/pending) | [P07 증거와 P05 환불 장부 기록 완료](../book.html#O06/paid)<br>[기존 전송 실패가 최종 확정됨, 같은 환불을 다시 승인 대기로 전환](../book.html#O06/review) |
| [환불 확정](../book.html#O06/paid) | **부분 환불 1.00 TestUSDC 확정**<br>환불은 실제 발생 영업일에 기록됩니다. 원 매출 일자는 원주문에 남습니다. | 원 매출 영업일 — 2026-11-20<br>환불 영업일 — 2026-11-21<br>환불 영수증 — RF-0142-01<br>남은 원금 — 5.50 TestUSDC | [정산으로](../book.html#O01) | — |
| [승인 제한](../book.html#O06/denied) | **이 환불을 승인할 수 없습니다**<br>신청한 환불은 본인이 승인·반려할 수 없습니다. 지점의 승인 권한을 가진 별도 담당자에게 전달하세요. | 환불 ID — RF-0142-01<br>원주문 — S-0142<br>재무 담당 — 신청·승인에 관여하지 않은 제출 권한자 | [같은 원주문 조회](../book.html#O02)<br>[환불 대기열](../book.html#O06) | — |

## O07 · 예약금·잔금과 구독 회차

**제품** 운영·정산 시스템　 **사용자** 지점 예약·구독 담당자　 **진입** 성수점 예약 또는 구독 목록에서 계약 선택

**완료 기준** 총액 30의 예약에서 예약금 10과 잔금 20은 별도 주문·기한·환불 한도를 가지며 구독 해지는 이미 발급된 회차를 추적한다.

[P05.R2](../../products/p05-operations-backoffice/prd.md) · [P05.R3](../../products/p05-operations-backoffice/prd.md) · [P05.R4](../../products/p05-operations-backoffice/prd.md) · [P05.UC2](../../products/p05-operations-backoffice/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [예약·청구](../book.html#O07/start) | **성수점 예약과 회차**<br>고객별 예약은 접근 권한이 있는 지점에서만 확인합니다. | 예약 RSV-030 — 총 30.00 TestUSDC<br>구독 SUB-052 — 2026-11 회차 청구 | [예약 주문 나누어 보기](../book.html#O07/booking)<br>[구독 회차 보기](../book.html#O07/cycle)<br>[운영 홈으로](../book.html#O12) | — |
| [예약 현황](../book.html#O07/booking) | **RSV-030 예약 주문**<br>각 주문의 서비스 기간·지급 기한과 확보된 재고를 확인합니다. | 예약금 DEP-030 — 10.00 TestUSDC / 지급 확정 / 발생 영업일 11-20 / 환불 한도 10.00<br>잔금 BAL-030 — 20.00 TestUSDC / 견적 발행 / 지급 기한 11-23 18:00 / 미지급<br>공급 지점 — 등록된 지점 이름<br>이용일 — 11-24 15:00 · 지점 시간대<br>예약 보존 — 발행된 잔금 관측 종료까지 | [잔금 원주문 관측](../book.html#O07/expiry)<br>[예약·회차 목록](../book.html#O07) | — |
| [구독 회차](../book.html#O07/cycle) | **SUB-052 · 2026-11**<br>청구 알림을 다시 보내도 회차와 지급 주문 ID는 유지됩니다. | 회차 주문 — INV-052-11<br>결제 기간 — 11-20–11-30<br>다음 회차 — 2026-12 | [발급 회차 상태](../book.html#O07/expiry)<br>[해지 범위 검토](../book.html#O07/cancel) | — |
| [해지 검토](../book.html#O07/cancel) | **앞으로의 구독 회차 닫기**<br>기발급 청구의 기존 견적과 지급을 계속 조회합니다. | 종료 적용 — 2026-12 이후<br>이미 발급 — INV-052-11 | [향후 회차 해지 요청](../book.html#O07/cancelStatus)<br>[계속 운영](../book.html#O07/cycle) | — |
| [해지 조회](../book.html#O07/cancelStatus) | **같은 해지 요청의 상태**<br>현재 회차의 결제·영수증을 원주문에서 조회합니다. | 해지 대상 — 2026-12 이후<br>현재 회차 — INV-052-11 별도 관측 | [발급 회차 조회](../book.html#O07/expiry) | [레코드 기록 완료](../book.html#O07/canceled) |
| [해지 반영](../book.html#O07/canceled) | **향후 회차를 종료했습니다**<br>발급된 회차의 결제 상태는 기존 주문에서 조회할 수 있습니다. | 열려 있는 기존 주문 — INV-052-11 | [원주문 관측](../book.html#O07/expiry) | — |
| [발행 원주문 조회](../book.html#O07/expiry) | **지급 관측 중**<br>견적이 발급된 뒤에는 만료 시각만으로 재고를 해제하지 않습니다. 만료 전에 포함된 거래를 계속 확인합니다. | 예약 — 가용 재고 보류<br>재발급 — 기한 이후 확정 미지급 또는 무효화 증거 대기 | [같은 주문 재조회](../book.html#O07/expiry)<br>[예약·회차 목록](../book.html#O07) | [기한 내 포함된 거래를 확정하고 기존 주문에 기록](../book.html#O07/settled)<br>[만료 후 지급이 없거나 무효화됐다는 증거가 확정됨](../book.html#O07/released) |
| [지급 확정](../book.html#O07/settled) | **기존 주문 지급이 확인됐습니다**<br>예약금, 잔금, 회차 중 해당 원주문에만 지급을 기록합니다. | 예약 또는 회차 — 이행 준비 | [예약·회차 목록](../book.html#O07) | — |
| [확정 미지급](../book.html#O07/released) | **만료 또는 무효화가 확인됐습니다**<br>재고 보류를 해제했습니다. 새 견적이 필요한 경우 가격을 다시 확인합니다. | 기존 주문 — 미지급 종료 | [예약·회차 목록](../book.html#O07) | — |

## O08 · 지급 주소 변경

**제품** 운영·정산 시스템　 **사용자** 주소 제안자·별도 보안 승인자　 **진입** 권한 있는 담당자가 등록부에서 다음 주소 버전 검토

**완료 기준** 소유 증명, 분리된 승인 두 건, 대기시간과 통지가 기록된 뒤 효력이 시작되며 옛 견적의 수취인은 변경되지 않는다.

[P05.R1](../../products/p05-operations-backoffice/prd.md) · [P05.R5](../../products/p05-operations-backoffice/prd.md) · [P05.UC1](../../products/p05-operations-backoffice/use-cases.md) · [P06.R1](../../products/p06-stablenet-contracts/prd.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [주소 제안](../book.html#O08/start) | **성수점 지급 주소 등록부**<br>신규 주소 소유 증명과 승인 후 효력 시작을 예약합니다. | 입력 · 다음 지급 주소 — 0x…v4 예시<br>입력 · 변경 사유 — 보관 계정 교체<br>유효 주소 — v3<br>제안 버전 — v4<br>대상 네트워크 — StableNet 테스트넷 8283 | [소유 증명 요청](../book.html#O08/proof)<br>[운영 홈으로](../book.html#O12) | — |
| [소유 증명](../book.html#O08/proof) | **신규 주소의 소유 증명을 확인합니다**<br>주소 검증을 통과하면 서로 다른 승인자의 대기열에 전달합니다. | 등록부 변경 요청 — ADDR-4<br>현재 지급 주소 — v3 | [증명 확인 다시 조회](../book.html#O08/proof) | [소유 증명 검증 및 별도 승인자 인증 완료](../book.html#O08/approve)<br>[증명 불일치](../book.html#O08/denied) |
| [분리 승인](../book.html#O08/approve) | **주소 변경 승인 1차·2차**<br>승인자는 자신의 계정으로 한 번씩 검토합니다. 제안자와 중복 승인자는 거절됩니다. | 요청 — ADDR-4<br>1차 — 보안 담당자 확인<br>2차 — 독립 승인자 대기 | [현재 승인자 검토 기록 요청](../book.html#O08/approvalStatus) | [동일인 또는 권한 없는 승인 요청](../book.html#O08/denied) |
| [승인 조회](../book.html#O08/approvalStatus) | **두 승인 기록을 확인합니다**<br>동일 변경 요청에서 승인 수와 통지 상태를 확인합니다. | 등록부 요청 — ADDR-4 | [다시 조회](../book.html#O08/approvalStatus) | [별도 승인 두 건 기록 및 변경 통지 발송](../book.html#O08/scheduled)<br>[독립 승인이 추가로 필요](../book.html#O08/approve) |
| [효력 예약](../book.html#O08/scheduled) | **새 주소 효력 시작 대기**<br>대기시간, 통지, 기발급 견적의 지급·만료·검증된 무효화 상태를 확인합니다. 옛 주소가 유효하지 않으면 기존 견적을 거절합니다. | 기발급 견적 — 결과 추적<br>변경 통지 — 성수점 발송 기록<br>효력 — 등록부 예정 시점 | [기존 요청 재조회](../book.html#O08/scheduled) | [대기시간 경과 및 등록부 효력 시작 확인](../book.html#O08/active)<br>[옛 견적의 상태가 불명확함](../book.html#O02/pending) |
| [효력 시작](../book.html#O08/active) | **지급 주소 버전 v4 활성**<br>새 견적과 주소 버전 불일치는 고객에게 새 금액·수취인 확인을 받습니다. | 신규 지급 — v4<br>과거 확정 지급 — 당시 주소 버전 유지 | [등록부로](../book.html#O08) | — |
| [변경 보류](../book.html#O08/denied) | **주소 변경 증거나 권한을 확인하세요**<br>인증된 보안 담당자에게 이 변경 요청을 확인해 달라고 요청하세요. | 유효 주소 — v3 유지 | [기존 요청 다시 조회](../book.html#O08/proof) | — |

## O09 · 대여 기기 개설과 반납

**제품** 운영·정산 시스템　 **사용자** 대여 전용 운영자　 **진입** 등록된 장비의 attestation과 대여 운영자 자격 확인

**완료 기준** 개설·예치·한도·지연 인출·반납을 원 계정에서 재조회하고 미결이 남으면 기기 정리를 막는다.

[P05.R7](../../products/p05-operations-backoffice/prd.md) · [P05.UC5](../../products/p05-operations-backoffice/use-cases.md) · [P06.R4](../../products/p06-stablenet-contracts/prd.md) · [P06.UC3](../../products/p06-stablenet-contracts/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [기기 확인](../book.html#O09/start) | **대여 기기 RNT-17**<br>검증된 기기와 대여 계정만 선택합니다. | 기기 — NU-54V-DK / 리비전 확인<br>대여 운영 범위 — v1<br>attestation — 10:10 검증 | [대여 개설 검토](../book.html#O09/open)<br>[기존 대여 반납](../book.html#O09/return)<br>[운영 홈으로](../book.html#O12) | [기기 증거 또는 대여 운영 권한 확인 실패](../book.html#O09/blocked) |
| [개설·예치 검토](../book.html#O09/open) | **대여 전용 계정 개설**<br>계약에서 계정과 한도·예치 상태를 확인합니다. | 요청 예치금 — 128.50 TestUSDC<br>일일 한도 — 40.00 TestUSDC<br>인증 시각 — 10:10 | [대여 전용 개설 요청](../book.html#O09/opening) | — |
| [개설 조회](../book.html#O09/opening) | **대여 계정 상태**<br>해시와 블록을 조회해서 개설·예치·한도를 확인합니다. | 계정 참조 — RNT-17 / v1<br>상태 — 온체인 대기 | [같은 개설 조회](../book.html#O09/opening) | [대여 개설, 예치 및 한도 기록 확인](../book.html#O09/return) |
| [반납 준비](../book.html#O09/return) | **RNT-17 반납과 잔액 인출**<br>미결 지급과 대여 계정의 인출 가능 시각을 검토합니다. | 대여 계약 — v1<br>반납 세대 — G-17<br>미결 지급 — 1건<br>환불·인출 경로 — 검증된 반환 목적지 | [미결 지급 및 인출 조회](../book.html#O09/hold)<br>[v1 계약 대사](../book.html#O15) | — |
| [반납 보류](../book.html#O09/hold) | **미결 지급과 지연 인출을 확인합니다**<br>기존 계정과 반납 참조를 보존하며 응답이 불명확하면 조회를 계속합니다. | 정리·재배정 — 보류<br>지연 인출 — 기존 계정에서 추적 | [원 계정 다시 조회](../book.html#O09/hold) | [미결 지급 대사 및 검증된 반환 경로 확인](../book.html#O09/returnReview) |
| [반납 검토](../book.html#O09/returnReview) | **반납 요청을 검토하세요**<br>등록된 반환 경로와 세대를 확인한 뒤 반납을 요청합니다. | 반납 세대 — G-17<br>반환 경로 — 검증된 반환 목적지<br>미결 지급 — 0건 | [이 세대의 반납 요청](../book.html#O09/returnPending)<br>[대여 내역으로](../book.html#O09/return) | — |
| [반납 결과 조회](../book.html#O09/returnPending) | **반납과 인출 상태 확인**<br>지연 인출이 아직 열리지 않았으면 같은 계정을 조회합니다. | 반납 세대 — G-17<br>기기 정리 — 확정 대기 | [같은 반납 요청 조회](../book.html#O09/returnPending) | [반납 확정과 인출 결과 증거 확인](../book.html#O09/closed)<br>[거래 또는 지연 인출 결과 확인 불가](../book.html#O09/returnPending) |
| [반납 확인](../book.html#O09/closed) | **대여 종료 기록 확인**<br>검증된 환불 경로와 반납 세대를 보관하고 기기 재배정 전 정리를 진행합니다. | 반납 세대 — G-17<br>정리 대상 — 대여 세션<br>환불 경로 — 검증됨 | [대여 세션 정리 요청](../book.html#O09/cleanup)<br>[운영 홈으로](../book.html#O12) | — |
| [정리 조회](../book.html#O09/cleanup) | **대여 세션 정리 상태**<br>반납 거래 참조와 지급 조회는 종료 뒤에도 남습니다. | 재배정 — 정리 확인 후 가능 | [기기 상태 다시 조회](../book.html#O09) | — |
| [운영 제한](../book.html#O09/blocked) | **기기 또는 운영 권한을 확인하세요**<br>같은 기기의 등록·검증 상태를 확인합니다. | 대여 개설·정리 — 중지 | [기기 다시 조회](../book.html#O09) | — |

## O10 · FOTA 캠페인

**제품** 운영·정산 시스템　 **사용자** 펌웨어 운영자　 **진입** 지원 기기와 서명된 펌웨어 버전 선택

**완료 기준** 지원하지 않는 리비전 또는 승인 대기 중 기기에 업데이트를 적용하지 않고 중단한 대상만 재개한다.

[P05.R7](../../products/p05-operations-backoffice/prd.md) · [P05.UC5](../../products/p05-operations-backoffice/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [캠페인 검토](../book.html#O10/start) | **지원 기기의 업데이트 캠페인**<br>지원 리비전, 서명된 버전과 회수 상태를 검토하세요. | 대상 — RNT-17 / 리비전 확인<br>현재 버전 — 0.6.2 예시<br>배포 버전 — 0.6.3 예시<br>지급 승인 대기 — 0건 | [대상 배포 요청](../book.html#O10/progress)<br>[운영 홈으로](../book.html#O12) | [비호환 기기, 회수 대상 또는 지급 승인 대기](../book.html#O10/blocked) |
| [적용 조회](../book.html#O10/progress) | **대상별 적용을 확인하고 있습니다**<br>캠페인과 대상의 체크포인트를 조회합니다. | 캠페인 — FW-063<br>대상 — RNT-17 | [같은 대상 조회](../book.html#O10/progress) | [네트워크 단절 또는 적용 응답 유실](../book.html#O10/retry)<br>[버전 적용과 건강 상태 확인](../book.html#O10/done) |
| [적용 지연](../book.html#O10/retry) | **중단된 대상의 상태를 확인하세요**<br>이미 적용한 대상은 건너뛰고 검증된 체크포인트에서 재개합니다. | 대상 — RNT-17<br>캠페인 — FW-063 | [대상 확인 후 같은 캠페인 재개 요청](../book.html#O10/progress) | — |
| [검증 완료](../book.html#O10/done) | **RNT-17 적용을 확인했습니다**<br>적용 버전과 캠페인 처리 결과를 기록했습니다. | 캠페인 — FW-063<br>보고 버전 — 0.6.3 예시 | [운영 홈으로](../book.html#O12) | — |
| [적용 금지](../book.html#O10/blocked) | **현재 이 기기에 배포할 수 없습니다**<br>지원 리비전과 회수 상태를 확인하고 지급 승인 대기가 있으면 정리 뒤 다시 검토하세요. | 배포 — 보류 | [대상 다시 검토](../book.html#O10) | — |

## O12 · 지점 운영 업무

**제품** 운영·정산 시스템　 **사용자** 인증된 지점 운영자　 **진입** 정산 또는 운영 로그인 뒤 지점 업무 선택

**완료 기준** 각 업무의 링크와 서버 접근을 역할로 제한하고 자격 철회 후 기존 원주문 조회 범위를 구분한다.

[P05.R1](../../products/p05-operations-backoffice/prd.md) · [P05.R6](../../products/p05-operations-backoffice/prd.md) · [P05.R7](../../products/p05-operations-backoffice/prd.md) · [P05.UC1](../../products/p05-operations-backoffice/use-cases.md) · [P05.UC5](../../products/p05-operations-backoffice/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [운영 홈](../book.html#O12/start) | **성수점 운영**<br>계정에 허용된 지점과 역할에서 업무를 선택합니다. | 지점 — 여울 커피 성수점<br>표시 범위 — 인증된 사용자 역할에 따라 변경 | [카탈로그](../book.html#O03)<br>[예약과 구독](../book.html#O07)<br>[직원과 단말](../book.html#O13)<br>[지급 주소](../book.html#O08)<br>[대여 기기](../book.html#O09)<br>[FOTA 캠페인](../book.html#O10)<br>[감사 이력](../book.html#O12/audit)<br>[정산으로](../book.html#O01) | [지점 또는 역할 권한이 없음](../book.html#O01/denied) |
| [감사 이력](../book.html#O12/audit) | **성수점 변경 이력**<br>변경자, 시각, 지점, 대상 식별자, 변경 전후 상태를 비교할 수 있습니다. | 입력 · 변경 유형 — 전체<br>입력 · 기간 — 2026-11-20<br>가격 변경 — 메뉴 v1 → v2<br>단말 변경 — v6 → v7<br>재처리 — evt-142 | [운영 홈으로](../book.html#O12) | — |

## O13 · 지점 직원과 단말 권한

**제품** 운영·정산 시스템　 **사용자** 가맹점 자격 관리자　 **진입** 가맹점 법인·지점 범위 승인 후 자격 업무 선택

**완료 기준** 분실 단말 및 퇴사자 자격을 회수하면 새 요청은 거절되고 미결 주문에는 담당자를 지정한다.

[P05.R1](../../products/p05-operations-backoffice/prd.md) · [P05.R6](../../products/p05-operations-backoffice/prd.md) · [P05.UC1](../../products/p05-operations-backoffice/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [권한 관리](../book.html#O13/start) | **성수점 직원·단말**<br>등록 또는 자격 회수 전에 지점과 역할을 확인합니다. | 입력 · 관리 대상 — 성수 주문 단말 02<br>입력 · 지점 — 여울 커피 성수점<br>입력 · 역할 — 점원<br>단말 — 성수 주문 단말 02 · 등록됨<br>직원 역할 — 성수점 점원 | [단말 등록 검토](../book.html#O13/register)<br>[자격 회수 검토](../book.html#O13/revoke)<br>[운영 홈으로](../book.html#O12) | — |
| [등록 검토](../book.html#O13/register) | **지점과 단말 신원을 확인하세요**<br>필요한 인증과 지점 등록을 확인하면 단말에서 적용 ACK를 기다립니다. | 요청 단말 — 성수 주문 단말 02<br>신규 자격 — 성수점에만 적용 | [등록 요청](../book.html#O13/status)<br>[등록 취소](../book.html#O13) | — |
| [회수 영향](../book.html#O13/revoke) | **자격 회수 검토**<br>미결 원주문과 권한이 있는 인계 담당자를 확인하세요. 제안 후 다른 운영자가 승인하면 신규 업무가 차단되고 지정한 원주문이 담당자에게 인계됩니다. | 미결 주문 — S-0142<br>인계 담당 — 성수점 주문 운영 | [자격 회수 요청](../book.html#O13/status)<br>[회수 취소](../book.html#O13) | — |
| [처리 조회](../book.html#O13/status) | **자격 변경 결과를 조회합니다**<br>같은 요청 번호로 제안·승인 상태를 조회합니다. 지정된 담당자는 해당 지점에서 인계받은 원주문을 확인하고 접수합니다. | 변경 요청 — ACCESS-1120<br>회수된 자격의 신규 업무 — 차단 | [같은 요청 확인](../book.html#O13/status)<br>[담당자가 내 인계 원주문 조회·접수](../book.html#O13/status)<br>[감사 이력](../book.html#O12/audit) | [다른 지점 또는 권한이 없는 변경 요청](../book.html#O01/denied) |

## O14 · 배포 프로필과 직접 지급 증거

**제품** 운영·정산 시스템　 **사용자** 권한 있는 배포 검증 담당자　 **진입** 배포 manifest 검증 및 테스트넷 주문의 효과 검사

**완료 기준** 서명한 배포와 등록 주소 버전을 확인한다. 원복은 확정 영수증에서 원금·주문 소비·릴레이 가스를 대조하고, 후속 성공은 별도 해시의 수취 순증 6.50을 확인한다.

[P06.R1](../../products/p06-stablenet-contracts/prd.md) · [P06.R2](../../products/p06-stablenet-contracts/prd.md) · [P06.R3](../../products/p06-stablenet-contracts/prd.md) · [P06.R4](../../products/p06-stablenet-contracts/prd.md) · [P06.UC1](../../products/p06-stablenet-contracts/use-cases.md) · [P06.UC2](../../products/p06-stablenet-contracts/use-cases.md) · [P06.UC3](../../products/p06-stablenet-contracts/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [배포 확인](../book.html#O14/start) | **StableNet 테스트넷 배포 프로필**<br>검증된 manifest에서 계약·ABI·등록부와 토큰을 확인합니다. | 체인 — 8283<br>허용 자산 — 지정 TestUSDC<br>선택 계약 — 직접 지급 v2<br>기준 블록 — manifest 기준<br>별도 재원 — v2 예치 및 가맹점 가스 | [직접 지급 증거 확인](../book.html#O14/lookup)<br>[구계약 대사](../book.html#O15) | [신뢰 manifest, ABI, 체인 또는 토큰 식별 불일치](../book.html#O14/invalid) |
| [지급 증거](../book.html#O14/lookup) | **S-0142 지급 효과**<br>같은 주문이라도 전송 시도별 해시를 조회합니다. 미채굴·미확정 구간은 조회 대기 중으로 표시합니다. | 금액 — 6.50 TestUSDC<br>주소 버전 — v3<br>증거 — 트랜잭션·블록·로그 위치<br>사용자 가스 — 0 / 테스트 가스 매장 부담 | [기존 거래 조회](../book.html#O14/lookup) | [수취 순증·로그·최종 확정 높이 일치](../book.html#O14/settled)<br>[동일 배포와 정규 블록에서 상태 0 영수증 확정](../book.html#O14/revert)<br>[미채굴·미확정 또는 근거 부족으로 재조회](../book.html#O14/lookup)<br>[서명한 배포의 체인·계약·ABI 불일치](../book.html#O14/invalid) |
| [증거 일치](../book.html#O14/settled) | **직접 지급 증거가 일치합니다**<br>운영 장부 확정 상태와 공개 영수증을 조회할 수 있습니다. | 수취 순증 — 6.50 TestUSDC<br>이미 사용된 업무 주문 — S-0142<br>지급 이벤트 — 1건 | [공개 영수증](../book.html#X01) | — |
| [실행 거절](../book.html#O14/revert) | **결제 시도가 원복됐습니다**<br>확정 영수증의 해시·포함 블록과 그 시도의 원금 효과를 확인합니다. 후속 지급은 해당 거래 해시에서 확인하세요. | 이 시도의 수취 순증 — 0.00 TestUSDC<br>이 시도의 주문 소비 — 0건<br>릴레이 가스 지출 — 가스 사용량 × 실효 단가 (wei)<br>가스 부담 계정 — 확정 거래의 릴레이 주소 | [같은 주문 증거 재조회](../book.html#O14/lookup)<br>[배포 프로필로](../book.html#O14) | — |
| [프로필 불일치](../book.html#O14/invalid) | **배포 설정을 사용할 수 없습니다**<br>현재 배포 검증을 통과해야 조회를 재개합니다. 이전 조회의 금액·거래 근거는 화면에서 지웁니다. | 결제 제출·인덱싱 활성 — 중단 | [검증 다시 실행](../book.html#O14) | — |

## O15 · 기존 계약 적립과 인출

**제품** 운영·정산 시스템　 **사용자** 기존 계약 점주·대사 담당자　 **진입** v1 계약 배포 선택 후 이전 적립 내역 확인

**완료 기준** 이전 적립 인출과 v2 직접 지급을 주소별로 대사하고 예치금·가스 간 이동이나 주문 재발행을 차단한다.

[P06.R4](../../products/p06-stablenet-contracts/prd.md) · [P06.UC3](../../products/p06-stablenet-contracts/use-cases.md) · [P05.R5](../../products/p05-operations-backoffice/prd.md) · [P05.R7](../../products/p05-operations-backoffice/prd.md) · [P05.UC5](../../products/p05-operations-backoffice/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [v1 원장](../book.html#O15/start) | **v1 적립금과 인출**<br>기존 배포의 점주 적립금은 해당 계약에서 조회합니다. | 계약 — v1 적립형<br>적립금 — 10.00 TestUSDC 예시<br>v2 예치 — 별도 배포 및 원장 | [기존 적립 인출 검토](../book.html#O15/withdraw)<br>[대여 잔액·인출](../book.html#O09/return)<br>[v2 배포로](../book.html#O14) | [구계약 서명을 v2에 제출하거나 버전이 다른 주문을 재발행 요청](../book.html#O15/wrong) |
| [점주 cashOut](../book.html#O15/withdraw) | **v1 인출 검토**<br>기존 적립 원장과 인출 권한을 확인합니다. | 인출 금액 — 10.00 TestUSDC 예시<br>인출 위치 — 기존 v1 계약<br>필요 권한 — v1 점주 | [v1 인출 요청](../book.html#O15/lookup)<br>[구 원장으로](../book.html#O15) | [권한 부여 범위와 다름](../book.html#O15/wrong) |
| [인출 조회](../book.html#O15/lookup) | **기존 cashOut 결과를 확인합니다**<br>인출 영수증과 적립금 감소를 같은 계약에서 대사합니다. | 계약 참조 — v1<br>요청 참조 — CO-017 | [같은 인출 조회](../book.html#O15/lookup) | [v1 인출 결과 확정](../book.html#O15/done) |
| [인출 확정](../book.html#O15/done) | **v1 인출 증거 확인**<br>인출 거래와 해당 영업일을 구계약 원장으로 연결했습니다. | 원장 — v1<br>v2 재원 변경 — 없음 | [구 원장으로](../book.html#O15) | — |
| [버전·권한 불일치](../book.html#O15/wrong) | **요청이 유효한 계약과 일치하지 않습니다**<br>원주문의 배포를 지정하고 기존 상태를 조회하세요. | 신규 제출·이체 — 거절 | [기존 계약 조회](../book.html#O15) | — |


# 테스트 시장 · 페이지별 설계

[전체 화면](../book.html) · [원 요구사항 대조표](../coverage.md) · [통합 감사](../audit.md)

제품에서 누르는 동작과 상태를 바꾸는 외부 사건을 열로 구분했다. 입력은 예시 데이터이며 실제 제품의 인수에서는 진입 조건과 검증 결과까지 함께 확인한다.

## L01 · 테스트 시장

**제품** 사용자 모바일 앱　 **사용자** 학습자　 **진입** 둘러보기 → 학습

**완료 기준** 모든 진입에서 테스트넷과 테스트 자산임을 표시한다.

[P08.R1](../../products/p08-market-services/prd.md) · [P08.R2](../../products/p08-market-services/prd.md) · [P08.R3](../../products/p08-market-services/prd.md) · [P08.R4](../../products/p08-market-services/prd.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [학습 시장](../book.html#L01/start) | **테스트 자산으로 연습하기**<br>StableNet 테스트넷 전용입니다. 실물 자산이나 실거래가 아닙니다. | 네트워크 — StableNet 테스트넷<br>자산 — 테스트 자산만 | [교환](../book.html#L02/start)<br>[유동성](../book.html#L03/start)<br>[TEST FX](../book.html#L04/start)<br>[모의 포지션](../book.html#L05/start)<br>[여행](../book.html#T01/start)<br>[운영자 인증](../book.html#L06/auth) | — |

## L02 · 테스트 교환

**제품** 사용자 모바일 앱　 **사용자** 학습자　 **진입** 학습 → 교환

**완료 기준** 경로·수수료·최소 수령·기한을 확인하고 불명확한 제출은 기존 해시를 조회한다.

[P08.R1](../../products/p08-market-services/prd.md) · [P08.UC1](../../products/p08-market-services/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [교환 입력](../book.html#L02/start) | **테스트 자산 교환**<br>StableNet 테스트넷 · 테스트 자산 | 입력 · 보내는 양 — 10<br>보내기 — 10 테스트 토큰<br>예상 수령 — 견적 필요 | [견적 확인](../book.html#L02/quote)<br>[학습 홈](../book.html#L01/start) | — |
| [견적](../book.html#L02/quote) | **교환 조건을 확인하세요**<br>테스트넷 견적이며 기기 승인은 별도입니다. | 경로 — TEST A → TEST B<br>입력 — 10.00 TEST A<br>예상 수령 — 9.90 TEST B<br>수수료 — 0.10 TEST A<br>슬리피지·최소 수령 — 1% · 9.80 TEST B<br>기한 — 10:43<br>하드월렛 단계 — 내용 대조 후 서명 1/4 독립 표시 대조: 독립 표시 장치에서 요청 내용을 대조하세요. 송금이면 금액·받는 곳·네트워크를 봅니다. (버튼 없음) | [기기 승인 요청](../book.html#L02/pending)<br>[다시 입력](../book.html#L02/start) | — |
| [확인 중](../book.html#L02/pending) | **같은 거래를 조회합니다**<br>승인·제출 후 응답이 불명확하면 같은 해시를 확인합니다. | 네트워크 — StableNet 테스트넷<br>상태 — 확인 중 | [재조회](../book.html#L02/pending) | [정규 체인 확정 증거](../book.html#L02/done)<br>[가격 한도 초과 또는 견적 만료](../book.html#L02/expired) |
| [재견적](../book.html#L02/expired) | **견적이 유효하지 않아요**<br>입력값을 유지하고 새 견적에 동의하세요. | 실행 — 없음 또는 기존 해시 조회 필요 | [새 견적](../book.html#L02/start) | — |
| [결과](../book.html#L02/done) | **테스트 교환 결과**<br>확정 증거와 실제 수령량을 대조합니다. | 상태 — 확정<br>실제 수령 — 9.90 TEST B | [학습 홈](../book.html#L01/start) | — |

## L03 · 유동성 공급과 회수

**제품** 사용자 모바일 앱　 **사용자** 학습자　 **진입** 학습 → 유동성

**완료 기준** 추가와 제거의 예상 지분·한도 및 확정 후 잔액을 비교한다.

[P08.R1](../../products/p08-market-services/prd.md) · [P08.UC1](../../products/p08-market-services/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [풀 선택](../book.html#L03/start) | **테스트 풀 선택**<br>StableNet 테스트넷 · 테스트 자산 | 풀 — 검증된 테스트 풀<br>보유 LP — 테스트 잔액 | [유동성 추가](../book.html#L03/add)<br>[유동성 제거](../book.html#L03/remove)<br>[학습 홈](../book.html#L01/start) | — |
| [추가](../book.html#L03/add) | **예상 지분을 확인하세요**<br>수량·최소 지분·기한을 독립 표시 장치에서 대조하고 기기에서 승인합니다. | 입력 · 입금 수량 — 10<br>입금 — 10 TEST A + 10 TEST B<br>예상 LP — 10.00 LP<br>슬리피지·최소 LP — 1% · 9.90 LP<br>기한 — 10:43<br>하드월렛 단계 — 내용 대조 후 서명 1/4 독립 표시 대조: 독립 표시 장치에서 요청 내용을 대조하세요. 송금이면 금액·받는 곳·네트워크를 봅니다. (버튼 없음) | [기기 승인 요청](../book.html#L03/pending)<br>[풀로](../book.html#L03/start) | — |
| [제거](../book.html#L03/remove) | **회수할 지분을 확인하세요**<br>최소 수령과 기한을 독립 표시 장치에서 대조하고 기기에서 승인합니다. | 입력 · LP 수량 — 1<br>소각 LP — 1.00 LP<br>예상 수령 — 1.00 TEST A + 1.00 TEST B<br>각 최소 수령 — 0.99 TEST A + 0.99 TEST B<br>기한 — 10:43<br>하드월렛 단계 — 내용 대조 후 서명 1/4 독립 표시 대조: 독립 표시 장치에서 요청 내용을 대조하세요. 송금이면 금액·받는 곳·네트워크를 봅니다. (버튼 없음) | [기기 승인 요청](../book.html#L03/pending)<br>[풀로](../book.html#L03/start) | — |
| [확인 중](../book.html#L03/pending) | **동일 거래를 조회합니다**<br>중단 후에도 기존 해시를 확인하고 중복 포지션을 만들지 않습니다. | 상태 — 확인 중 | [다시 조회](../book.html#L03/pending) | [정규 체인 확정 증거](../book.html#L03/done)<br>[견적 만료 또는 한도 이탈](../book.html#L03/start) |
| [결과](../book.html#L03/done) | **실제 잔액을 확인하세요**<br>예상치와 실제 토큰·LP 잔액을 비교합니다. | 추가 결과 예시 — 10 TEST A + 10 TEST B → 10 LP<br>추가 후 LP 잔액 — 10 LP · 확정 결과 | [풀로](../book.html#L03/start) | — |

## L04 · TEST FX

**제품** 사용자 모바일 앱　 **사용자** 학습자　 **진입** 학습 → TEST FX

**완료 기준** 통화쌍 방향·소수 단위·가격 시각과 기한을 표시하고 만료 후 재동의한다.

[P08.R2](../../products/p08-market-services/prd.md) · [P08.UC2](../../products/p08-market-services/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [방향과 금액](../book.html#L04/start) | **모의 환전 방향 선택**<br>StableNet 테스트넷 · TEST FX는 모의 환전입니다. | 입력 · 기준 금액 — 10<br>기준 → 상대 — TEST USD → TEST KRW<br>소수 단위 — 견적 기준 | [견적](../book.html#L04/quote)<br>[학습 홈](../book.html#L01/start) | — |
| [환전 견적](../book.html#L04/quote) | **방향과 가격을 확인하세요**<br>한 견적에 한 번만 실행합니다. | 방향 — 10.00 TEST USD → TEST KRW<br>가격 — 1 TEST USD = 1,300 TEST KRW<br>예상 수령 — 13,000 TEST KRW<br>소수 단위 — USD 2자리 · KRW 0자리<br>가격 시각·기한 — 10:42 · 10:43<br>하드월렛 단계 — 내용 대조 후 서명 1/4 독립 표시 대조: 독립 표시 장치에서 요청 내용을 대조하세요. 송금이면 금액·받는 곳·네트워크를 봅니다. (버튼 없음) | [기기 승인 요청](../book.html#L04/pending)<br>[방향 변경](../book.html#L04/start) | — |
| [조회 중](../book.html#L04/pending) | **기존 환전을 조회합니다**<br>승인 후 같은 견적과 거래를 조회합니다. | 상태 — 확인 중 | [다시 조회](../book.html#L04/pending) | [견적 만료 확인](../book.html#L04/expired)<br>[정규 체인 확정 증거](../book.html#L04/done) |
| [만료](../book.html#L04/expired) | **새 가격을 확인하세요**<br>새 견적에는 이전 승인을 재사용하지 않습니다. | 이전 견적 — 만료<br>새 가격과 차이 — 재견적 후 표시 | [새 견적](../book.html#L04/quote) | — |
| [완료](../book.html#L04/done) | **모의 환전 결과**<br>확정된 테스트 수량을 확인합니다. | 실행 — 1건 · 확정<br>실제 수령 — 13,000 TEST KRW | [학습 홈](../book.html#L01/start) | — |

## L05 · 모의 무기한 포지션

**제품** 사용자 모바일 앱　 **사용자** 학습자　 **진입** 학습 → 포지션

**완료 기준** 방향·증거금·가격·펀딩·청산 조건과 정산 이력을 표시한다.

[P08.R3](../../products/p08-market-services/prd.md) · [P08.R4](../../products/p08-market-services/prd.md) · [P08.UC3](../../products/p08-market-services/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [포지션](../book.html#L05/start) | **테스트 포지션**<br>StableNet 테스트넷 · LAB/LAQ 학습 풀의 모의 손익입니다. | 방향·크기 — 매도 · 1 LAB<br>증거금 — 12 LAQ<br>진입·기준 가격 — 1 LAB당 1.20 / 1.20 LAQ · 10:42<br>펀딩 — 5분마다 · 다음 10:45 · 현재 명목 금액의 0.10%<br>예상 청산 — 12.00 LAQ/LAB · 펀딩 전 예상 청산가 | [조건 검토](../book.html#L05/review)<br>[정산 이력](../book.html#L05/history)<br>[학습 홈](../book.html#L01/start) | — |
| [조건 검토](../book.html#L05/review) | **위험 조건을 확인하세요**<br>한도·가격·펀딩 시점과 예상 청산 조건을 독립 표시 장치에서 대조한 뒤 기기에서 승인합니다. | 방향·크기 — 매도 · 1 LAB<br>증거금·한도 — 12 LAQ · 1 LAB<br>기준 가격 — 1.20 LAQ/LAB · 10:42<br>펀딩·예상 청산 — 다음 10:45 · 0.10% / 펀딩 전 12.00 LAQ/LAB<br>하드월렛 단계 — 내용 대조 후 서명 1/4 독립 표시 대조: 독립 표시 장치에서 요청 내용을 대조하세요. 송금이면 금액·받는 곳·네트워크를 봅니다. (버튼 없음) | [기기 승인 요청](../book.html#L05/pending)<br>[돌아가기](../book.html#L05/start) | — |
| [확인 중](../book.html#L05/pending) | **포지션 상태 조회**<br>기존 해시와 포지션을 재조회합니다. | 상태 — 확인 중 | [재조회](../book.html#L05/pending) | [확정 포지션 증거](../book.html#L05/history)<br>[가격 이상 통지](../book.html#L05/stale) |
| [가격 지연](../book.html#L05/stale) | **신규 위험 증가가 중지됐어요**<br>기존 포지션과 확정 이력을 조회할 수 있어요. 새 가격을 검증할 때까지 위험 증가와 자동 청산을 보류합니다. | 가격 — 오래됨<br>포지션 — 기존 상태 유지 | [정산 이력](../book.html#L05/history) | — |
| [이력](../book.html#L05/history) | **펀딩과 정산 이력**<br>같은 기간 중복 정산과 동시 종료 결과를 대조합니다. | 포지션·기간 — 포지션 17 · 10:45 펀딩<br>펀딩 — 0.0012 LAQ 수령 · 1회 확정<br>동시 종료 경합 — 사용자 종료 확정 · 중복 청산 0건<br>증거 — 정규 블록 25,810,240 · 로그 2 | [포지션](../book.html#L05/start) | — |

## L06 · 가격과 keeper 운영

**제품** 운영·정산 시스템　 **사용자** 운영자　 **진입** 학습 → 운영 모니터

**완료 기준** 운영 권한에서 가격 신선도·작업 키·기존 해시·재개 근거를 확인한다.

[P08.R4](../../products/p08-market-services/prd.md) · [P08.UC3](../../products/p08-market-services/use-cases.md)

| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |
|---|---|---|---|---|
| [운영 현황](../book.html#L06/start) | **가격과 keeper 상태**<br>운영 역할만 접근합니다. 테스트넷 시장과 매장 결제는 별도입니다. | 가격 출처·시각 — 검증된 출처 · 마지막 관측 시각<br>keeper — 작업 키·포지션 revision·기간<br>관측 — 정규 체인과 대조 | [중지 상황](../book.html#L06/stale)<br>[작업 상세](../book.html#L06/job)<br>[학습 홈](../book.html#L01/start) | — |
| [가격 이상](../book.html#L06/stale) | **영향 상품을 일시 중지**<br>오래된 가격으로 신규 위험 증가와 자동 청산을 보류합니다. | 가격 — 1,010 TEST USD · 마지막 관측 10:42 · 신선도 만료<br>보류 — 신규 위험 증가·자동 청산<br>재개 — 새 피드·정규 체인·정책 검증 대기 | [작업 상세](../book.html#L06/job)<br>[운영 현황](../book.html#L06/start) | [신선한 피드와 정책 검증 및 기존 작업 대조 완료](../book.html#L06/resumed) |
| [작업 상세](../book.html#L06/job) | **같은 작업을 재조회하세요**<br>작업 키·기존 해시·포지션과 정규 체인 이벤트를 대조하며 중복 실행을 방지합니다. | 작업 키 — POS-17 · 기간 2026-11-20T10<br>제출 해시 — 0xa17f…d142 · 기존 해시<br>관측 증거 — 정규 블록 25,810,240 · 로그 2<br>복구 — cursor와 포지션 revision 재조회 후 같은 작업만 재개 | [현황](../book.html#L06/start) | — |
| [운영자 인증](../book.html#L06/auth) | **시장 운영자 권한 확인**<br>승인된 운영 역할만 가격과 keeper 작업에 접근합니다. | 권한 — 확인 대기 | [학습 홈](../book.html#L01/start) | [운영자 권한 확인](../book.html#L06/start)<br>[권한 거절](../book.html#L06/denied) |
| [접근 거절](../book.html#L06/denied) | **운영 화면을 열 수 없습니다**<br>시장 운영 권한이 필요합니다. | 접근 — 거절 | [학습 홈](../book.html#L01/start) | — |
| [재개 근거](../book.html#L06/resumed) | **검증된 가격으로 재개했어요**<br>기존 작업 키와 포지션 revision을 대조해 중복 정산을 방지합니다. | 가격 — 1,012 TEST USD · 10:45<br>정책 — 신선도와 출처 검증 통과<br>작업 — POS-17 · 기존 해시 재조회 완료 | [작업 상세](../book.html#L06/job)<br>[운영 현황](../book.html#L06/start) | — |


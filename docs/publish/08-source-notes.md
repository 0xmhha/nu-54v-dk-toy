# 08편 내부 근거 메모 — Medium 업로드 금지

작성일: 2026-11-20. 원고는 현재 코드, 과거 개발 기록, 이번 호스트 시험을 구분한다. 프로젝트 저장소는 마지막 09편에서 공개하므로 본문 레퍼런스에는 공식 문서만 쓴다.

## 원고와 소스 대응

| 설명 | 근거 |
|---|---|
| 주문 확인 후 confirm.show, 동일한 p를 pending_auth에 저장 | products/p01-device-firmware/core/src/nu54_session_pay.c, ss_prepare |
| 가맹점 이름은 검사한 attestation, 금액·주문·token·payout은 p | 같은 파일 ss_identify, ss_prepare. 실제 키·주소·서명 내부 구현은 원고에서 제외 |
| 폰의 역할과 본딩된 링크만 수용 | products/p02-user-app/src/link/confirmLink.ts, receive; products/p02-user-app/src/App.tsx, Home. 본딩은 앱/기기가 무결하다는 가정을 대체하지 않음 |
| require_phone는 연결·본딩·알림 구독을 검사하며 렌더 완료 ACK가 아님 | products/p01-device-firmware/core/src/nu54_links.c, nu54_links_is_phone/nu54_links_call; core/src/nu54_session_pay.c, require_phone; app/release.conf |
| BigInt 정수 나눗셈과 두 자리 버림, 알 수 없는 토큰의 base unit 표기 | products/p02-user-app/src/confirm/display.ts; test/confirm.test.ts. 5_009_999n, 999n은 기존 시험 값. 5.009999는 자릿수 6으로 해석한 정확한 값이며 실측 결제 금액이 아님 |
| SW1/SW2 짧게 누름과 길게 눌렀을 때 안내 | products/p01-device-firmware/app/src/main.c. PIN 입력 중에는 다른 의미로 쓰이므로 원고는 결제 승인 대기 상황만 설명 |
| USER_REJECTED, payment.result의 의미 | core/src/nu54_session_pay.c, ss_payment_button. 내부 서명 코드는 공개하지 않음 |
| 최종 판정과 finalized 이벤트 대조 | products/p04-merchant-kiosk/src/payment/submit.ts, findSettled/ours/submit; src/kiosk/pay.ts |
| 폰은 전달받은 최종 결과를 표시하고 직접 체인 조회하지 않음 | confirmLink.ts, payment.outcome 분기; App.tsx, OUTCOME_TEXT |
| 키오스크 결과 수신 대기 기본 10초, 없으면 session.cancel | products/p04-merchant-kiosk/src/payment/session.ts, WAIT_MS=10_000, runPayment |
| 제출 판정 10초는 requestedAt부터 누적 | src/payment/session.ts: requestedAt=Date.now()는 link.send 호출 직전. src/payment/submit.ts: BUDGET_MS=10_000. 프로토콜 문서의 '전송 완료부터'와 정확히 같다고 쓰지 않음 |
| 폰 120초 후 waiting, 취소 메시지는 보내지 않음 | confirmLink.ts, CONFIRM_TIMEOUT_MS=120_000, show |
| 결과 수신 전 취소와 실제로 서명이 생기지 않은 것은 다름 | test/session.test.ts, 'a press that races the cancel...' . 늦은 응답을 다음 세션 것으로 쓰지 않는 소프트웨어 시험이 있으며, 모든 무선 타이밍에 대한 보장은 아님 |
| 거절·취소·Checking에서 최종 결과 전달을 건너뛰는 현재 경로 | src/kiosk/pay.ts: cancelled/refused return이 outcome 구성보다 앞, Checking도 먼저 return. core ss_refused는 kiosk 응답, 폰 즉시 결과 알림은 없음. 기기 반납·코드 개선은 이번 글 작성 범위 밖 |
| 다른 order 결과는 상세 없이 표시 | confirmLink.ts: view는 orderId 일치일 때만, show(result)는 항상. 완전한 stale-message 방어·multi-order 지원으로 과장하지 않음 |
| 20행 시간 CSV가 연속 20회 성공 증거는 아님 | src/kiosk/timings.ts: signed+finishedAt+최종상태 필터, 최근20개. test/timings.test.ts의 4200+i는 가상 값으로 원고/그림에 싣지 않음 |

## 개발 이력

- docs/content/acceptance/gate-w7.md: 사람이 처음 보드 버튼을 누르던 실행의 `device refused: undefined`, 결과 없음과 거절의 혼동, SW1 짧게 누름 안내로 수정. 현재 코드는 별도로 cancelled를 반환한다. 당시 실패와 현재 처리를 같은 시점으로 합치지 않는다.
- 2026-10-02, 커밋 09d424e: 폰 결과 전달을 미루던 주석을 실제 payment.outcome 전달로 바꾸고 공유 벡터·폰 시험을 추가했다. 이것은 구현 이력이지 실물 폰 결제 통합 성공 기록이 아니다.
- docs/content/acceptance/gate-w7.md의 2026-10-05: 보드+Android 키오스크 실기 결제 1건. 폰 확인 화면 없음 waiver가 남아 있어 폰을 포함한 통합 결과로 쓰지 않는다.
- docs/content/acceptance/week12-log.md: 11/25 예정 수용 기록지. 화면 일치, 연속20회/각10초 이내, 거절 시연 모두 미실행. 미래 기록지를 실행 증거로 인용하지 않는다.
- products/p02-user-app/README.md도 폰 앱+기기 결제 확인 통합과 본딩 실기 미검증을 명시한다. 셋업 관련 10/7 수정과 결제 화면 전체 검증을 혼동하지 않는다.

## 이번 실행 — 2026-11-20

소스·테스트 파일을 바꾸지 않고 기존 시험만 실행했다.

1. `pnpm --filter @nu54/p02-renter test -- --runInBand test/confirm.test.ts test/App.test.tsx`

   - 2 suites, 15 tests passed; exit 0.
   - pnpm script의 `--` 때문에 Jest 출력은 `--runInBand`까지 파일 검색 패턴으로 표시했다. 선택된 두 파일은 정확히 일치했다. 실제 in-band 실행으로 주장하지 않는다.
   - `A worker process has failed to exit gracefully and has been force exited...` 경고가 있었다. 경고는 타이머 정리 가능성을 제시하지만 원인은 이번 작업에서 확정하지 않았다. 원고는 작업자 종료 경고로 적는다.
   - 14 confirm tests + 1 App first-screen test. React/native BLE 대신 mock, confirm 타이머 시험은 fake timer. 실기 화면 캡처나 무선 증거가 아님.

2. `pnpm --filter @nu54/p04-kiosk test -- --runInBand test/session.test.ts test/pay.test.ts test/timings.test.ts`

   - 3 suites, 26 tests passed; exit 0.
   - `--runInBand` 해석은 위와 동일. 소프트웨어 기기·대체 chain/submission을 쓴다. 50ms를 쓰는 취소 시험을 실제 10초 무선 실측으로 쓰지 않는다.

3. `cmake -S products/p01-device-firmware/test -B /private/tmp/nu54-08-host -DCMAKE_BUILD_TYPE=Debug`
   `cmake --build /private/tmp/nu54-08-host --target test_session test_links`
   `ctest --test-dir /private/tmp/nu54-08-host -R 'session_vectors|links' --output-on-failure`
   `/private/tmp/nu54-08-host/test_session`

   - 2 test groups passed, 0 failed; session binary: `ok: 38 session scenarios, 0 failures`.
   - 38 scenarios are within session_vectors, not extra independent groups.
   - New host build in temp directory. OpenSSL on host, not PSA on physical hardware. No firmware flash, real BLE, testnet transaction, security audit, or continuous-payment demonstration performed.

## 코드 발췌

- C: ss_emit → pending_auth → pending 3줄, nu54_session_pay.c의 연속 발췌.
- TypeScript: truncatedAmount 함수 전체, display.ts의 연속 발췌.
- TypeScript: runPayment의 result 대기→cancelled 분기, session.ts의 연속 발췌.
- 두 언어의 발췌는 같은 코드다. 원문의 들여쓰기만 본문에서 정규화한다.

## 공식 레퍼런스 확인

2026-11-20 웹으로 읽은 1차 자료만 사용한다.

1. ECMAScript BigInt::divide: 정수 나눗셈의 0 방향 절삭. 금액은 nonnegative 정수라는 사용 문맥이다.
2. Ethereum JSON-RPC block parameter: finalized 표기. StableNet 확정 시간 수치는 가져오지 않는다.
3. React Native testing overview: JavaScript/component tests가 native OS 동작을 대신하지 않는 범위. 우리 앱 시험 통과의 근거는 자체 실행이며 문서가 아니다.

## 공개 안전·이미지

- 본문과 도식에는 실제 키, 전체 주소, 트랜잭션 해시, 계정 이름, 내부 절대 경로, 저장소 링크가 없다.
- 폰·키오스크·펌웨어의 코드 이름은 필요할 때 쓰되 프로젝트 관리 ID는 싣지 않는다.
- 도식은 언어별3장. UI를 새로 그려 앱 캡처로 제시하지 않는다. 시간 막대는 설정값10/120에 비례하며 시작 사건은 서로 다르다.
- 금액 그림은 기존 시험 값의 소수 자릿수를 나타내며 새 측정값·잔액·비율을 만들지 않는다.
- 앱 캡처를 추가할 경우 실제 에뮬레이터 화면을 사용하고 공개 안전 규칙을 다시 확인한다.
- Medium 초안에 주제5개와 알림 해제만 설정한다. 공개 발행·예약 발행·구독자 메일 전송은 이번 요청에 포함되지 않는다.

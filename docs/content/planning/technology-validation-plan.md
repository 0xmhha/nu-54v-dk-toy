# 기술 선택 검증 작업 23개

[선택 카드](technology-selection.md) · [JSON 원본](technology-validation-plan.json). 실제 개발 단계에서 수행할 검증이다. 모두 planned_not_run이며 기존 104개 패키지/320개 세부 작업의 완료 증거를 구체화한다.

## VAL-01 · RN 앱·키오스크 검증

- 기술 선택: TECH-01; 관련 작업: APP-01, SHOP-01, BASE-03
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - S25 Ultra와 대상 태블릿에서 권한·백그라운드 복귀
  - Android/iOS release build에서 native bridge 로딩
  - 공유 패키지에 DOM/browser storage 의존 없음
- 증거: exact source commit/library/toolchain/profile versions; reproduction command and test inputs; expected vs observed result; redacted logs and app/device evidence
- 실패 시: 원인을 기록하고 대체 후보 또는 adapter 수정 후 동일 완료 기준으로 재검증; 기능 범위는 유지
- 상태: 미실행; 담당자·공수 미지정.

## VAL-02 · Google·Apple 인증 검증

- 기술 선택: TECH-02; 관련 작업: AUTH-02, AUTH-03, AUTH-04, BASE-05
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - 각 제공자 로그인/취소/계정 연결
  - callback·nonce·audience 바꾼 증명 거절
  - refresh 응답 유실 후 검증된 복구 또는 재인증
- 증거: exact source commit/library/toolchain/profile versions; reproduction command and test inputs; expected vs observed result; redacted logs and app/device evidence
- 실패 시: 원인을 기록하고 대체 후보 또는 adapter 수정 후 동일 완료 기준으로 재검증; 기능 범위는 유지
- 상태: 미실행; 담당자·공수 미지정.

## VAL-03 · 업무 API·백오피스 검증

- 기술 선택: TECH-03; 관련 작업: BASE-04, AUTH-01, OPS-01, SHOP-05
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - 107 API 중 결제/환불/claim 수직 경로 연결
  - 다른 매장 ID 접근 거절
  - 웹 세션이 고객/점주 signer 권한을 자동 획득하지 않음
- 증거: exact source commit/library/toolchain/profile versions; reproduction command and test inputs; expected vs observed result; redacted logs and app/device evidence
- 실패 시: 원인을 기록하고 대체 후보 또는 adapter 수정 후 동일 완료 기준으로 재검증; 기능 범위는 유지
- 상태: 미실행; 담당자·공수 미지정.

## VAL-04 · 업무·보안 상태 저장 검증

- 기술 선택: TECH-04; 관련 작업: BASE-05, PAY-04, STAMP-01, AUTH-04
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - 두 연결에서 환불/claim 경쟁 시 한 번만 반영
  - refresh 회전 crash 복구
  - outbox commit/재전송 뒤 중복 혜택 없음
- 증거: exact source commit/library/toolchain/profile versions; reproduction command and test inputs; expected vs observed result; redacted logs and app/device evidence
- 실패 시: 원인을 기록하고 대체 후보 또는 adapter 수정 후 동일 완료 기준으로 재검증; 기능 범위는 유지
- 상태: 미실행; 담당자·공수 미지정.

## VAL-05 · 보호 객체·작업 큐 검증

- 기술 선택: TECH-05; 관련 작업: BASE-05, REC-03, AI-02, RELEASE-02
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - metadata 후 동의 철회 시 stream 거절
  - 삭제 tombstone 적용 후 backup 복원
  - job 재전송/유실 복구와 민감 로그 부재
- 증거: exact source commit/library/toolchain/profile versions; reproduction command and test inputs; expected vs observed result; redacted logs and app/device evidence
- 실패 시: 원인을 기록하고 대체 후보 또는 adapter 수정 후 동일 완료 기준으로 재검증; 기능 범위는 유지
- 상태: 미실행; 담당자·공수 미지정.

## VAL-06 · NU 보드·RTOS 검증

- 기술 선택: TECH-06; 관련 작업: HW-01, HW-07, OTA-01
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - NU 실제 모델에서 reproducible build/boot
  - display IMU mic 각 핀·전원 확인
  - 전체 기능 RAM/flash/전원 여유를 map과 측정으로 기록
- 증거: exact source commit/library/toolchain/profile versions; reproduction command and test inputs; expected vs observed result; redacted logs and app/device evidence
- 실패 시: 원인을 기록하고 대체 후보 또는 adapter 수정 후 동일 완료 기준으로 재검증; 기능 범위는 유지
- 상태: 미실행; 담당자·공수 미지정.

## VAL-07 · HW 키·암호 backend 검증

- 기술 선택: TECH-07; 관련 작업: HW-02, HW-04, HW-05, KEY-02
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - 고정 test vector 주소/EOA 서명 검증
  - key handle 생성/import/사용/삭제/재부팅
  - 권한 없는 영역 접근 거절 및 실제 software key 노출 경계 기록
- 증거: exact source commit/library/toolchain/profile versions; reproduction command and test inputs; expected vs observed result; redacted logs and app/device evidence
- 실패 시: 원인을 기록하고 대체 후보 또는 adapter 수정 후 동일 완료 기준으로 재검증; 기능 범위는 유지
- 상태: 미실행; 담당자·공수 미지정.

## VAL-08 · FOTA·부트 복구 검증

- 기술 선택: TECH-08; 관련 작업: OTA-01, OTA-02, OTA-03, OTA-04
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - 서명 불일치/다른 모델 이미지 거절
  - 전송·swap·첫 부팅 중 전원 차단 복구
  - 새 firmware의 설정 migration 실패 시 안전 복구
- 증거: exact source commit/library/toolchain/profile versions; reproduction command and test inputs; expected vs observed result; redacted logs and app/device evidence
- 실패 시: 원인을 기록하고 대체 후보 또는 adapter 수정 후 동일 완료 기준으로 재검증; 기능 범위는 유지
- 상태: 미실행; 담당자·공수 미지정.

## VAL-09 · BLE 세션·거리·찾기 검증

- 기술 선택: TECH-09; 관련 작업: HW-03, HW-06, FIND-01, FIND-02
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - S25+보유 NU/Nordic DK의 GATT/거리 결과 session binding
  - peer 바꾸기/replay 거절
  - 거리 실패는 새 승인 종료하되 기존 거래 추적 유지
- 증거: exact source commit/library/toolchain/profile versions; reproduction command and test inputs; expected vs observed result; redacted logs and app/device evidence
- 실패 시: 원인을 기록하고 대체 후보 또는 adapter 수정 후 동일 완료 기준으로 재검증; 기능 범위는 유지
- 상태: 미실행; 담당자·공수 미지정.

## VAL-10 · 표준 패스키 검증

- 기술 선택: TECH-10; 관련 작업: KEY-01, KEY-02, KEY-03, STAMP-04
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - 지정 RP/브라우저/OS에서 등록/인증
  - user presence와 PIN/UV 정책 검증
  - 반납 후 resident credential·외부 계정 등록 처리
- 증거: exact source commit/library/toolchain/profile versions; reproduction command and test inputs; expected vs observed result; redacted logs and app/device evidence
- 실패 시: 원인을 기록하고 대체 후보 또는 adapter 수정 후 동일 완료 기준으로 재검증; 기능 범위는 유지
- 상태: 미실행; 담당자·공수 미지정.

## VAL-11 · 녹음 전송·모바일 보관 검증

- 기술 선택: TECH-11; 관련 작업: REC-01, REC-02, REC-03, HW-07
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - 실제 마이크→S25 파일 저장/재생
  - 화면 잠금·끊김의 누락 구간 표시
  - 서명/FOTA 동시 실행 우선순위와 메모리 최고치 측정
- 증거: exact source commit/library/toolchain/profile versions; reproduction command and test inputs; expected vs observed result; redacted logs and app/device evidence
- 실패 시: 원인을 기록하고 대체 후보 또는 adapter 수정 후 동일 완료 기준으로 재검증; 기능 범위는 유지
- 상태: 미실행; 담당자·공수 미지정.

## VAL-12 · Cloud Wallet MPC 검증

- 기술 선택: TECH-12; 관련 작업: MPC-01, MPC-02, MPC-03, MPC-04, MPC-05
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - 독립 프로세스 DKG/sign과 threshold 미달 거절
  - 앱 참여자 실제 build/서명
  - 참여자 교체·old-share 철회·중단 round 재시작 검증
- 증거: exact source commit/library/toolchain/profile versions; reproduction command and test inputs; expected vs observed result; redacted logs and app/device evidence
- 실패 시: 원인을 기록하고 대체 후보 또는 adapter 수정 후 동일 완료 기준으로 재검증; 기능 범위는 유지
- 상태: 미실행; 담당자·공수 미지정.

## VAL-13 · EOA·토큰·SDK 검증

- 기술 선택: TECH-13; 관련 작업: TOKEN-01, TOKEN-02, TOKEN-03, PAY-01, INDEX-02
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - SDK 금액/receipt/event adapter 교정
  - EOA 서명 복원·전송·재시도
  - dummy/native/wrapped 주소·decimals·가스 동작 대조
- 증거: exact source commit/library/toolchain/profile versions; reproduction command and test inputs; expected vs observed result; redacted logs and app/device evidence
- 실패 시: 원인을 기록하고 대체 후보 또는 adapter 수정 후 동일 완료 기준으로 재검증; 기능 범위는 유지
- 상태: 미실행; 담당자·공수 미지정.

## VAL-14 · 스마트 계정 검증

- 기술 선택: TECH-14; 관련 작업: SMART-01, SMART-02, SMART-03, BASE-06
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - 같은 UserOp hash를 SDK/contract에서 재현
  - 같은 bundle의 서로 다른 UserOp 결과 구분
  - 고객 gas 경로와 account 전환/자산 이전 앱 실행
- 증거: exact source commit/library/toolchain/profile versions; reproduction command and test inputs; expected vs observed result; redacted logs and app/device evidence
- 실패 시: 원인을 기록하고 대체 후보 또는 adapter 수정 후 동일 완료 기준으로 재검증; 기능 범위는 유지
- 상태: 미실행; 담당자·공수 미지정.

## VAL-15 · DeFi·FX·Perpetual 검증

- 기술 선택: TECH-15; 관련 작업: DEX-01, DEX-02, FX-01, FX-02, PERP-01, PERP-02, PERP-04, PERP-06
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - swap/LP add-remove 실제 자산 흐름
  - stale/조작 가격에서 주문 제한
  - margin/pnl/funding/liquidation 경계·keeper 중복 실행 시험
- 증거: exact source commit/library/toolchain/profile versions; reproduction command and test inputs; expected vs observed result; redacted logs and app/device evidence
- 실패 시: 원인을 기록하고 대체 후보 또는 adapter 수정 후 동일 완료 기준으로 재검증; 기능 범위는 유지
- 상태: 미실행; 담당자·공수 미지정.

## VAL-16 · DID·STO 자격 검증

- 기술 선택: TECH-16; 관련 작업: DID-01, DID-02, DID-03, STO-01, STO-02
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - 발급→앱 보관→목적 제한 제시→철회
  - issuer rotation 상태 일관성
  - 자격 만료/철회 사용자의 STO 전송 거절
- 증거: exact source commit/library/toolchain/profile versions; reproduction command and test inputs; expected vs observed result; redacted logs and app/device evidence
- 실패 시: 원인을 기록하고 대체 후보 또는 adapter 수정 후 동일 완료 기준으로 재검증; 기능 범위는 유지
- 상태: 미실행; 담당자·공수 미지정.

## VAL-17 · x402 유료 자원 검증

- 기술 선택: TECH-17; 관련 작업: X402-01, X402-02, X402-03, TOKEN-02
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - payment-required→proof→settle→resource 제공
  - 응답 유실에도 이중 과금 없음
  - 실제 gas payer·allowance·authorization/nonce·smart account signature 호환 기록
- 증거: exact source commit/library/toolchain/profile versions; reproduction command and test inputs; expected vs observed result; redacted logs and app/device evidence
- 실패 시: 원인을 기록하고 대체 후보 또는 adapter 수정 후 동일 완료 기준으로 재검증; 기능 범위는 유지
- 상태: 미실행; 담당자·공수 미지정.

## VAL-18 · 장소·전사·여행 AI 검증

- 기술 선택: TECH-18; 관련 작업: TRIP-01, TRIP-02, TRIP-04, AI-01, AI-02, AI-03, REC-03
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - 한/영 고정 녹음 표본 정확도/지연
  - 실제 장소 ID·사용자 후기·결제 provenance 연결
  - 코스 이동/시간/예산 검사와 동의 철회 삭제
- 증거: exact source commit/library/toolchain/profile versions; reproduction command and test inputs; expected vs observed result; redacted logs and app/device evidence
- 실패 시: 원인을 기록하고 대체 후보 또는 adapter 수정 후 동일 완료 기준으로 재검증; 기능 범위는 유지
- 상태: 미실행; 담당자·공수 미지정.

## VAL-19 · Indexer·관측·출시 검증

- 기술 선택: TECH-19; 관련 작업: INDEX-01, INDEX-03, INDEX-04, INDEX-06, RELEASE-01, RELEASE-02, RELEASE-03
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - backfill/reorg/재전송 후 지급·혜택·여행 일관성
  - DB/indexer/object backup 동시점 차이 복구
  - clean build와 J01~J19 증거 묶음
- 증거: exact source commit/library/toolchain/profile versions; reproduction command and test inputs; expected vs observed result; redacted logs and app/device evidence
- 실패 시: 원인을 기록하고 대체 후보 또는 adapter 수정 후 동일 완료 기준으로 재검증; 기능 범위는 유지
- 상태: 미실행; 담당자·공수 미지정.

## VAL-20 · 키 보존·FOTA 회귀

- 기술 선택: TECH-07; 관련 작업: HW-02, OTA-04
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - secure storage schema migration 후 기존 주소·passkey 자격 유지
  - downgrade/비인가 read 실패
  - reset 후 재대여 키 분리
- 증거: versioned compatibility matrix; reproducible success/failure evidence; retained scope and documented residual constraints
- 실패 시: 선택 보류 후 대체 경로 검증; 요구 범위 유지
- 상태: 미실행; 담당자·공수 미지정.

## VAL-21 · 패스키 전송 행렬

- 기술 선택: TECH-10; 관련 작업: KEY-01, KEY-02
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - 각 RP/OS/browser/transport에서 등록/인증 결과 기록
  - 외부 bridge에서 private key 취득 불가
  - 외부 근접만으로 UV 충족이라 표시하지 않음
- 증거: versioned compatibility matrix; reproducible success/failure evidence; retained scope and documented residual constraints
- 실패 시: 선택 보류 후 대체 경로 검증; 요구 범위 유지
- 상태: 미실행; 담당자·공수 미지정.

## VAL-22 · MPC 복구 신뢰 경계

- 기술 선택: TECH-12; 관련 작업: MPC-04, MPC-05
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - 폰 분실과 서버 단독 접근 조합 검증
  - 두 share가 한 운영자 권한으로 동시에 접근되지 않음
  - 정한 recovery protocol의 구 share 재사용 실패
- 증거: versioned compatibility matrix; reproducible success/failure evidence; retained scope and documented residual constraints
- 실패 시: 선택 보류 후 대체 경로 검증; 요구 범위 유지
- 상태: 미실행; 담당자·공수 미지정.

## VAL-23 · x402 고객 가스 적합성

- 기술 선택: TECH-17; 관련 작업: X402-01, TOKEN-02
- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.
- 판정 항목:
  - 카페 일반 EOA gas와 facilitator settlement gas 별도 기록
  - 고객 부담 지원 profile에서만 승인
  - custom scheme이면 상호운용 범위와 규격 차이를 문서화
- 증거: versioned compatibility matrix; reproducible success/failure evidence; retained scope and documented residual constraints
- 실패 시: 선택 보류 후 대체 경로 검증; 요구 범위 유지
- 상태: 미실행; 담당자·공수 미지정.


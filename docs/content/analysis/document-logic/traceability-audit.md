# 전체 설계 추적 관계·무결성 점검

15개 요구사항 → 104개 작업 → 320개 세부 작업 → 8개 설계 묶음의 연결을 확인했다. 추가 계약 26개에는 변경 영향 작업을 연결했다. 문서 검사 통과이며 제품 구현·실행 완료를 의미하지 않는다.

## 보완한 내용

- 계약별 작업 연결을 별도 검토표와 그래프에 반영했다. 여러 묶음에 걸치는 계약은 모든 관련 작업의 주 설계 묶음을 표시한다.
- 초기 보완 기록의 계약 개수는 당시 20개, 현재 26개임을 인계서에 명시했다.
- 현재 입력 참조와 과거 기준 참조를 구분했다. 과거 해시를 현재 값으로 덮어쓰지 않았다.

## 검증 결과

- WBS 의존 관계 272개: 순환 0개. 작업마다 주 설계 묶음 1개.
- 계약·작업 연결 65개: 없는 계약·작업 참조 0개.
- 참조 해시 476개: 현재 일치 393개, 과거 기준 83개 모두 보존본과 일치.
- 그중 현재와 다른 과거 참조 47개는 변경 전 기준의 정상 기록이다. 보존 파일 174개도 검증했다.
- 수정 사항 18개의 수정 전 근거·현재 해결 위치·파일 해시와 검증 명령 연결을 확인했다.
- 그래프 2083개 노드·9723개 관계: 끝점·문서 해시·JSON 위치를 확인했다.
- 이 검사는 기존 322개 설계 검사·예제 및 20개 정합성 검사와 별도의 참조 검증이다.

## 요구사항별 추적

| 번호 | 요구사항 | 작업 수 | 관련 설계 묶음 |
|---|---|---:|---|
| 1 | 실기 HW wallet | 17 | DS-01, DS-03, DS-08 |
| 2 | 실기 FOTA | 9 | DS-01, DS-08 |
| 3 | 패스키·녹음·찾기·결제 스탬프 | 17 | DS-01, DS-03, DS-07, DS-08 |
| 4 | 유저 앱 | 25 | DS-01, DS-02, DS-03, DS-04, DS-07, DS-08 |
| 5 | Google·Apple 소셜 로그인 | 14 | DS-02, DS-08 |
| 6 | 소셜 계정 Cloud Wallet MPC | 14 | DS-02, DS-08 |
| 7 | 위치 검색·결제 기록·발자취 | 22 | DS-02, DS-03, DS-07, DS-08 |
| 8 | 앱에서 실제기기 설정 | 35 | DS-01, DS-02, DS-03, DS-04, DS-08 |
| 9 | RN 키오스크·점주 매장 운영 | 26 | DS-01, DS-02, DS-03, DS-04, DS-08 |
| 10 | 운영 백오피스 | 26 | DS-01, DS-02, DS-03, DS-04, DS-08 |
| 11 | StableNet 테스트넷 계약군 | 32 | DS-04, DS-05, DS-06, DS-08 |
| 12 | 기존 Indexer 확장 | 16 | DS-03, DS-04, DS-08 |
| 13 | DEX·DeFi·FX·Perpetual 앱 서비스 | 17 | DS-05, DS-08 |
| 14 | AI 여행 추천·챌린지·발도장 | 16 | DS-03, DS-07, DS-08 |
| 15 | 필수 운영·검증·배포 도구 | 17 | DS-01, DS-02, DS-03, DS-04, DS-08 |

작업 수는 검증 작업을 포함하며 요구사항 간 중복을 포함한다. 관련 설계 묶음은 작업에서 유도한 영향 범위이며 묶음 자체의 선언 범위와 다를 수 있다.

## 추가 계약별 작업 연결

| 계약 | 작업 | 설계 묶음 |
|---|---|---|
| OC-01 | AUTH-02, AUTH-03, AUTH-04 | DS-02 |
| OC-02 | AUTH-04 | DS-02 |
| OC-03 | MPC-02, MPC-04, MPC-05 | DS-02 |
| OC-04 | SHOP-01, SHOP-03 | DS-03 |
| OC-05 | SHOP-02, SHOP-06, PAY-03 | DS-03 |
| OC-06 | SHOP-03, PAY-04 | DS-03 |
| OC-07 | APP-04, STAMP-01, STAMP-02, STAMP-04 | DS-03, DS-08 |
| OC-08 | SHOP-05, OPS-03 | DS-03, DS-08 |
| OC-09 | DEX-02, DEX-03, FX-02, FX-03, PERP-03, PERP-05 | DS-05 |
| OC-10 | PERP-03, PERP-04, PERP-06 | DS-05 |
| OC-11 | DID-02, DID-03, STO-02, STO-03 | DS-06 |
| OC-12 | X402-01, X402-02, X402-03 | DS-06 |
| OC-13 | REC-03 | DS-07 |
| OC-14 | TRIP-01, TRIP-02, TRIP-03, TRIP-04, AI-01, AI-02, AI-03 | DS-07 |
| OC-15 | TRIP-04, REC-03 | DS-07 |
| OC-16 | BASE-05, PAY-02, REC-03 | DS-03, DS-07, DS-08 |
| OC-17 | AUTH-04, APP-01 | DS-02 |
| OC-18 | AUTH-04, APP-01 | DS-02 |
| OC-19 | AUTH-04 | DS-02 |
| OC-20 | AUTH-04 | DS-02 |
| OC-21 | X402-02, X402-03 | DS-06 |
| OC-22 | X402-02, X402-03 | DS-06 |
| OC-23 | SHOP-01, SHOP-03 | DS-03 |
| OC-24 | SHOP-01, SHOP-03 | DS-03 |
| OC-25 | AI-02 | DS-07 |
| OC-26 | AI-02 | DS-07 |

## 현재 동결과 남은 실행 조건

정책 19개와 RR-DEC-01의 과거 open 기록은 보존한다. 현재 authority인 DF-20260920-01에는 20개 모두 선택되어 있으며 그래프의 decision 상태도 선택 상태로 overlay했다. 아래 항목은 구현·실증 전이라 계속 남는다.

| 번호 | 남은 실행 조건 |
|---|---|
| 1 | NU-54V-DK revision, pin map, memory and secure-service capability require the actual board. |
| 2 | OAuth, Kakao and OpenAI credentials must be registered outside the repository; only secret references belong here. |
| 3 | All StableNet contract addresses except verified chain identity remain disabled until deployment evidence is registered. |
| 4 | MPC, passkey BLE, FOTA, audio throughput, Chain Sounding and all application journeys are runtime-unverified. |
| 5 | Commercial use with real assets, tokenized rights or traveler rental requires a current legal and provider-terms review. |

담당자·작업량은 배정하지 않았다. 제품 코드·generated schema·후속 migration·실제 profile·배포 주소·실기와 서비스 검증은 아직 수행하지 않았다. x-theory의 정의가 미확인되어 현재 그래프는 중립적 관계 분석이다.

## 근거와 재현

[기존 검사 재실행 결과](traceability-regression-results.json) · [전체 검증 기록](traceability-audit.json) · [계약-작업 검토표](contract-task-trace.json) · [그래프](graph.json) · [논리 보완 결과](review.md)

```sh
python3 content/analysis/document-logic/build_graph.py
python3 content/analysis/document-logic/validate_traceability.py
python3 content/analysis/document-logic/render_review.py
```

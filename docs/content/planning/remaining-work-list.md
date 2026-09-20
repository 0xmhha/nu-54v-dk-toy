# 남은 작업 마스터 목록

2026-09-20 기준. 현재는 상세 설계 단계이며 제품 구현은 아직 시작하지 않았다.

이 문서는 지금까지 작성한 설계를 실제 작업 순서로 압축한 실행용 목차다. 세부 작업은 [104개 작업 인계표](preimplementation-task-handoffs.md), 결정 근거는 [결정 자료](decision-briefing.md), 최신 계약 채택 범위는 [통합 계약 채택 계획](unified-adoption-plan.md)을 기준으로 한다.

## 현재 상태

| 구분 | 전체 | 완료 | 남음 | 현재 판정 |
|---|---:|---:|---:|---|
| 요구사항 | 15 | 설계 연결 15 | 구현·실증 15 | 모든 요구가 작업과 연결됐지만 실행 전 |
| WBS | 104작업·320단계 | 인계 정의 완료 | 구현·검증 104작업 | owner·공수·기한 미배정 |
| 정책·기술 결정 | 20 | 선택 20 | 0 | DF-20260920-01에 구현 진입 기준으로 동결 |
| 제품별 상세 설계 | 10영역 | 10 | 0 | 선택값 기반 구성·계약·저장·상태·장애·수용 기준 고정 |
| 기준 계약 채택 | 8묶음·21파일 | 설계 기준 8 | runtime 활성화 8 | 설계 overlay 채택, schema/SQL/기기/체인 적용 전 |
| 제품 구현 | 10영역 | 0 | 10영역 | 사용자 지시에 따라 보류 |
| 통합 수용 | 12여정 | 0 | 12여정 | 실제 기기·앱·서비스 증거 없음 |
| 실운영 출시 판단 | 1 | 0 | 1 | 테스트넷 수용과 별도로 검토 |

문서 검사 통과는 제품 구현이나 실기 검증 완료로 계산하지 않는다.

## 전체 진행 순서

```mermaid
flowchart LR
 A["A. 정책·기술 선택"] --> B["B. 선택값 기반 상세 설계"]
 B --> C["C. 기준 계약 동시 채택"]
 C --> D["D. 구현 전환·외부 환경 준비"]
 D --> E["E. 10개 제품 영역 구현"]
 E --> F["F. 12개 통합 여정 검증"]
 F --> G["G. 테스트넷 완료 판정"]
 G -. "실자산 운영 시" .-> H["H. 상용 출시 검토"]
```

모든 선택을 기다린 뒤 한꺼번에 시작하는 전역 순서는 아니다. 독립된 작업은 병행할 수 있지만, 미선택 값을 임의 기본값으로 구현하거나 fixture 결과를 실제 통합 완료로 합산하지 않는다.

## A. 정책·기술 선택

각 항목은 선택값, 적용 범위, 선택 근거, 적용 버전, 미지원 분기의 동작을 기록해야 완료된다.

- [x] **A-01 · 앱·인증 대상 — D01**: RN 0.87.x 별도 앱·공유 도메인 패키지, Android/iOS 유저 앱, Android 태블릿 키오스크, 한·영/Google·Apple.
- [x] **A-02 · 연결 규칙·데이터 경계 — D02**: JSON/JCS HTTP, deterministic CBOR BLE, v1 envelope, transaction+outbox, 30초 clock skew.
- [x] **A-03 · HW 지갑 키·반납 복구 — D03 + RR-DEC-01**: BIP-39/import·secp256k1·secure service, 사용자 암호화 복구 package 확인 뒤 reset.
- [x] **A-04 · Cloud MPC — D04**: cb-mpc 2-of-3 server-native 참여자와 별도 mobile approval key.
- [x] **A-05 · NU 보드·FOTA — D05**: NCS v3.4.0·manifest Zephyr·project board definition, MCUboot/MCUmgr/DTS slot; 실제 revision은 bring-up 측정 gate.
- [x] **A-06 · 패스키·녹음·찾기 — D06 + D07**: CTAP 2.2 시험 RP, PCM 16k/16-bit/mono·30분·64KiB buffer, RSSI 찾기와 보조 ranging.
- [x] **A-07 · 결제·환불·정산·혜택 — D08 + D09**: 60초 signed 시험환율 quote, exact payment, 2 block, 부분환불, 매장별 10-stamp 혜택.
- [x] **A-08 · StableNet 시험 환경 — D10**: 8283 RPC identity 확인, WKRC 18/dummy USDC 6, 주소 활성화 evidence gate.
- [x] **A-09 · 스마트 계정·DeFi·FX — D11 + D12**: EOA 우선, Kernel/4337 후속, dUSDC/WKRC AMM·0.5% slippage·exact allowance.
- [x] **A-10 · Perpetual — D13**: TEST BTC/WKRC·dUSDC 담보·3x·40/25% margin·60초 oracle stale.
- [x] **A-11 · STO·DID·x402 — D14 + D15 + D16**: 무권리 test membership, did:web/did:key VC, 1 dUSDC·24시간 x402 자원.
- [x] **A-12 · 위치·AI·운영 목표 — D17 + D18 + D19**: Kakao Local, OpenAI 전사/추천, 보존 기간·성능·RPO/RTO·운영 권한 고정.

완료 증거: [DF-20260920-01 구조화 checkpoint](design-freeze-checkpoint.json)에 20개 선택값·실패/축소 규칙·근거가 기록됐고, 기존 selection profile의 필드/gate 구조보다 선택값에 관해서 우선한다.

## B. 선택값 기반 상세 설계

논리 설계는 작성되어 있다. 아래 작업은 A 단계에서 관련 선택이 확정된 범위부터 진행한다.

- [x] **B-01 · NU 펌웨어·주변 부품**: 구성·wire·partition 원칙·상태·장애·실기 수용 gate 고정.
- [x] **B-02 · 유저 앱·로그인·기기 설정**: 인증·binding·BLE·wallet·cache·unknown result 계약 고정.
- [x] **B-03 · Cloud MPC**: 참여자·승인·transcript·epoch·quorum loss·복구 계약 고정.
- [x] **B-04 · RN 키오스크·매장 운영**: 공유 태블릿·주문·결제·환불·매출/정산 상태와 예외 계약 고정.
- [x] **B-05 · 백오피스·관리 신뢰**: 관리 상태, 2인 승인, trust bootstrap·rotation·incident 계약 고정.
- [x] **B-06 · 테스트넷 토큰·상품 컨트랙트**: 계약군·ABI/event registry·역할·배포 증거·실패 gate 고정.
- [x] **B-07 · Indexer·조회 프런트엔드**: revision·canonicality·decoder·cursor·rebuild generation 계약 고정.
- [x] **B-08 · DEX·FX·Perpetual 앱 서비스**: quote·risk·allowance·keeper·결과 상태·수용 벡터 고정.
- [x] **B-09 · 여행·기록·추천·챌린지**: provider·동의·audio segment·AI revision·삭제·후기 근거 계약 고정.
- [x] **B-10 · 공통 기반·릴리스·운영 도구**: 환경·secretRef·관측·backup/restore·release/evidence 계약 고정.

완료 증거: [제품별 상세 설계](design-freeze-checkpoint.md)에 10개 영역의 범위·구성·계약·저장·상태·장애·수용 증거가 고정됐다. 실제 핀맵/메모리 수치, generated schema/ABI와 실행 벡터 결과는 구현·실기 단계의 활성화 gate로 남긴다.

## C. 기준 계약 채택

아래 묶음은 API·BLE·권한·DTO·저장·reader·화면을 같은 checkpoint에서 함께 검토한다. 현재 21개 대상 파일 중 19개는 둘 이상의 묶음이 공유한다.

- [x] **C-01 · 인증·MPC·보호 결과 — UA-01**: 설계 기준 채택.
- [x] **C-02 · 상거래·영수증·단말 — UA-02**: 설계 기준 채택.
- [x] **C-03 · 대여·반납·기기 연속성 — UA-03**: 설계 기준 채택.
- [x] **C-04 · 시장·자격·유료 자원 — UA-04**: 설계 기준 채택.
- [x] **C-05 · 녹음·여행·개인정보 — UA-05**: 설계 기준 채택.
- [x] **C-06 · 설정 적용·관리 신뢰 — UA-06**: 설계 기준 채택.
- [x] **C-07 · 공유 저장·이벤트·화면 — UA-07**: 설계 기준 채택.
- [x] **C-08 · 통합 checkpoint — UA-08**: `DF-20260920-01` 기준 overlay 채택. runtime 활성화는 구현 뒤 별도 판정.

공통 검사:

- [ ] 구 client→새 server, 새 client→구 server의 지원/거절 동작 확인.
- [ ] 일부 peer만 새 profile을 적용했을 때 신규 효과 hold와 원 결과 조회가 분리되는지 확인.
- [ ] 새 writer와 구 reader의 expand→전환→관측→정리 순서 확인.
- [ ] 기존 API-020/107 typed reader와 API-088 audit-only 의미 보존 확인.
- [ ] 기존 7개 migration은 수정하지 않고 필요한 DB 변경을 후속 migration으로 설계.
- [ ] 문서 rollback과 이미 노출된 서명·체인·외부 효과의 조정을 구분.

완료 증거: [구현 진입 기준 계약](../specifications/design-baseline-contract.md)에 8개 묶음과 공통 wire·불변조건·migration 원칙을 공동 채택했다. 21개 상세 inventory는 보존하며 generated schema/SQL/runtime 적용은 구현 단계의 별도 activation이다.

## D. 구현 전환과 외부 환경 준비

- [x] **D-01 · 구현 전환 경계**: 이번 checkpoint까지 설계, 제품 구현은 별도 착수 지시 전 보류로 기록.
- [x] **D-02 · 개발 기준 고정**: RN/NCS와 재사용 저장소 revision 고정; 실물 board target commit은 조건부 gate로 기록.
- [x] **D-03 · 계정·공급자 준비**: 필요한 scope·redirect·secretRef 템플릿 작성; 실제 계정/비밀 등록 대기.
- [x] **D-04 · StableNet 환경 manifest**: RPC/chain identity 확인, 계약은 배포 증거 전 비활성으로 등록.
- [x] **D-05 · Indexer 환경 manifest**: backend/frontend revision, source·confirmation·cursor/rebuild 계약 등록; endpoint/decoder 배포 대기.
- [x] **D-06 · 운영 신뢰 bootstrap 준비**: 역할·workload·2인 bootstrap 템플릿 작성; 실제 인물/KMS 등록 대기.
- [x] **D-07 · 개발·시험 데이터 준비**: synthetic store/menu/device/user/place/commerce fixture와 삭제 등급 등록.

D-03~07은 자료 준비를 선행할 수 있지만, 실제 계정·키·주소 등록은 해당 정책과 권한 설계가 확정된 뒤 수행한다.

## E. 제품 구현

세부 구현은 [104개 작업 인계표](preimplementation-task-handoffs.md)의 입력·선행 조건·수용 증거를 따른다.

- [ ] **E-01 · NU Zephyr 펌웨어**: HW wallet, FOTA, passkey, 녹음, 찾기, 결제 스탬프, 설정·반납, BLE 동시 동작 구현.
- [ ] **E-02 · 유저 모바일 앱**: 소셜 로그인, HW/Cloud wallet, 기기 설정, 결제·기록·여행·상품 화면 구현.
- [ ] **E-03 · Cloud MPC 서비스**: 지갑 생성, threshold 서명, refresh, 참여자 장애·교체·복구 구현.
- [ ] **E-04 · RN 키오스크**: 매장 가입/로그인, 메뉴·주문·NU 결제, 환불, 매출·정산, 공유 단말 관리 구현.
- [ ] **E-05 · 백오피스**: 매장·기기·대여, FOTA, 결제 예외, profile rollout, 권한·감사·복구 운영 구현.
- [ ] **E-06 · StableNet 컨트랙트**: dummy USDC, WKRC, smart account, DeFi, FX, perpetual, STO, DID, x402 구현·배포.
- [ ] **E-07 · Indexer·조회 프런트엔드**: 계약 이벤트 수집, 확정/reorg, 주문·환불·정산·혜택 projection과 조회 화면 구현.
- [ ] **E-08 · DEX·FX·Perpetual 서비스/UI**: 견적, allowance, 실행, 포지션·펀딩·청산, keeper와 결과 조회 구현.
- [ ] **E-09 · 여행·AI·기록 서비스**: 위치 맛집 검색, 결제 발자취, 녹음 전사/요약, 후기·코스·챌린지·발도장 구현.
- [ ] **E-10 · 공통 기반·운영 도구**: 인증·권한, 환경 설정, 관측, 백업·복구, 배포, 증거 수집·릴리스 도구 구현.

각 구현은 정상 경로뿐 아니라 거절·응답 유실·중복 요청·권한 철회·복구 경로를 포함해야 한다.

## F. 통합 검증

- [ ] **F-01** 소셜 가입 → Cloud MPC 생성·서명 → 참여자 장애·복구.
- [ ] **F-02** 여행 중 기기 대여 → 신규/import HW 지갑 → 결제 → 자산/접근 확인 → 반납·재대여 격리.
- [ ] **F-03** RN 키오스크 주문 → NU 기기 승인 → StableNet 지급 → Indexer → 매출·앱 영수증.
- [ ] **F-04** 부분환불 → 스탬프 회수/보정 → 정산 correction → 백오피스 확인.
- [ ] **F-05** passkey·녹음·찾기·스탬프 동시 사용 → FOTA 전원 중단 → 키와 상태 보존.
- [ ] **F-06** 일반 EOA → 스마트 계정 전환 → UserOperation 성공/내부 실패·allowance 잔존 확인.
- [ ] **F-07** dummy USDC/WKRC → DeFi·FX·Perpetual → stale price·keeper 중복·reorg 보정.
- [ ] **F-08** DID 자격 발급·제시·철회 → STO 자격 동작 → 부적격 직접 호출 차단.
- [ ] **F-09** x402 지급 → 유료 결과 전달 → 응답 유실·provider 실패 → 중복 과금 없는 재조회.
- [ ] **F-10** 기기 녹음 → BLE/폰 파일 → 전사·AI 요약 → 동의 철회·삭제·backup restore 차단.
- [ ] **F-11** 위치 맛집 → 실제/시험 결제·후기 → AI 코스 → 챌린지·발도장 → 환불 보정.
- [ ] **F-12** 전체 릴리스 → 권한 회전 → 백업 복원 → 오래된 권한·삭제 자료 부활·중복 금융 효과 차단.

완료 증거에는 실제 환경/버전, 입력, 기대 결과, 실측 결과, 실패·복구 결과와 앱 화면·기기·체인·서비스의 연결이 모두 포함돼야 한다.

## G. 완료와 출시 판정

- [ ] **G-01 · 요구사항 수용표**: 15개 요구와 16개 복합 기능별 실제 증거 index 완성.
- [ ] **G-02 · 회귀·보안·복구 확인**: 권한 분리, 키/개인정보 비노출, 중복 금융 효과 차단, restore/reorg/FOTA/오프라인 복구 통과.
- [ ] **G-03 · 테스트넷 릴리스 인계**: 버전·manifest·배포 주소·운영 runbook·known limitations·rollback/후속 변경 계획 고정.
- [ ] **G-04 · 12주 완료 판정**: 문서 수나 fixture 통과가 아니라 앱+기기+체인+서비스의 실제 수용 증거로 판정.
- [ ] **G-05 · 실운영 출시 검토**: 실제 스테이블코인 수령, 여행자 대여, 키 관리, 개인정보, 자금·환불·정산 책임과 관련 제도·약관을 별도 검토한 뒤 출시 범위 결정.

G-05는 테스트넷 프로젝트 완료 조건과 분리한다. 상용 출시가 목표가 되는 시점에 최신 법률·규제·공급자 약관을 다시 조사해야 한다.

## 현재 중단점

[DF-20260920-01](design-freeze-checkpoint.md)에서 A~D를 마쳤다. 구현 전환 경계는 기록했지만 제품 구현은 시작하지 않았다. 다음은 저장소 정리 checkpoint이며, 그 뒤 사용자가 구현 착수를 명시하면 E 단계로 이동한다.

## 범위에서 제외한 항목

- 개인별 담당 배정, 작업량·일정 산정, 완료율 계산.
- 동결 checkpoint 이후 새 범위·정책의 임의 변경.
- 실제 관리자·키·OAuth 계정·계약 주소 등록.
- 제품 코드, DB migration, 펌웨어, 앱, 컨트랙트 배포.
- 테스트넷 기능 완료를 실자산 상용 출시 허가로 해석하는 것.

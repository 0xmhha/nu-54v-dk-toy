# 제품별 구현 전 설계 종료·인계 조건 재점검

2026-09-20 · 설계 보완판 · 제품 구현 보류

15개 요구의 제품별 인계 조건을 현재 설계에 맞게 재분류한다. 기존 인계서의 보완판이며 이 문서의 준비 상태는 구현 허가·완료율·실기 검증이 아니다.

## 이번 판정

- 15개 요구·16개 복합 기능·104개 작업·320개 세부 작업의 범위는 유지한다. 12주 완료 목표와 3명 참여 조건을 변경하거나 개인별 공수를 산정하지 않는다.
- 일반 논리 흐름·실패/복구·역할·증거의 인계 후보는 작성되어 있다. 이것만으로 제품별 wire/ABI/물리 저장 설계까지 확정됐다고 판단하지 않는다.
- 19개 결정과 RR-DEC-01은 미선택이다. 다만 Zephyr·RN 키오스크·NU-54V-DK·S25 Ultra·StableNet testnet·USDC 우선/더미 토큰·EOA/고객 가스 우선은 기존 확정 조건이다.
- 제품 단위 일괄 통과 대신 작업 종류를 구분한다. 관련 선택이 없는 새로운 효과는 보류하되 별도 권한으로 허용된 읽기·제한·원 요청 조정까지 같은 이유로 막지 않는다.
- 기존 HG-03의 DI/OC 채택 목록에 profile rollout·관리 신뢰·증거/운영 화면 보완을 추가한다. 새로운 카탈로그 ID나 API가 이미 등록되었다는 뜻은 아니다.
- 기존 기록을 현재 값으로 덮어써 검증 통과를 만들지 않는다. 원 인계서·보존본을 유지하고 이 보완판에서 최신 채택 범위와 종료 증거를 연결한다.

[원 인계서](preimplementation-handoff.md) · [104개 작업 인계](preimplementation-task-handoffs.md) · [선택 결정 자료](decision-briefing.md)

## 남은 작업 분류

| 분류 | 닫는 데 필요한 증거 | 완료 근거가 아닌 것 | 기존 항목 |
|---|---|---|
| DC-01 · 정책·기술 선택 | 기존 결정에 선택값·근거·scope·적용 버전이 명시됨 | 권장안, 예제 값, 다음 진행이라는 요청 | DR-01 / HG-02 |
| DC-02 · 선택 후 구체 설계 | 선정 profile로 schema/ABI·핀맵/메모리·오류/시험 벡터를 재현 가능하게 고정 | 일반 상태표, 논리 필드 목록, 보드에 대한 미측정 성능 기대 | DR-02 / HG-02 |
| DC-03 · 기준 계약 동시 채택 | 변경 API/BLE/권한/DTO/storage/reader/UI를 같은 기준으로 대조한 채택 checkpoint | 후보 문서의 존재, 한쪽 API만 추가 | DR-03 / HG-03 |
| DC-04 · 외부 환경 등록 | 실제 계정·endpoint·주소·deployment identity·허용 scope와 연결 증거 | 예제 URL/주소, 문서의 공급자 이름 | DR-04 / HG-05 |
| DC-05 · 구현·통합 검증 | 구현 전환 지시 후 실제 제품과 선정 환경에서 정상·실패·복구의 증거 수집 | 설계 예제 통과, 테스트넷 배포만 성공, 화면 목업 | DR-05 / HG-06 |
| DC-06 · 실운영 출시 검토 | 실제 자금/키 통제·사업 역할·개인정보 처리·출시 범위에 맞춘 별도 판단 | 테스트넷 수용 통과를 실제 상용 운영 허용으로 해석 | DR-06 / HG-06 |

DC-06은 실제 자산·상용 운영으로 전환할 때 적용하는 별도 출시 판단이다. 테스트넷 기반 12주 기능 수용에 상용 출시 완료를 추가하지 않는다. DC-04의 계정/주소 등록도 설계 문서 작성 완료와 구분한다.

## 제품별 인계 카드

요구 기반 작업 영향은 제품 사이에 겹칠 수 있다. 아래 연결은 작업 소유권·담당 배정이 아니다. 결정 참조는 주요 검토 범위이며 모든 행위가 그 결정의 모든 필드를 요구한다는 뜻도 아니다. 실제 행위별 조건은 selection-action-gates를 따른다.

### DP-01 · NU 펌웨어·주변 부품

요구 1, 2, 3, 8 · [DS-01](../specifications/device-coexistence-design.md)

- 작성된 논리 설계: 서명/패스키/녹음/FOTA/반납 공존, 실패 복구와 앱 연결 경계
- 선택 대기: NU-54V-DK revision/Zephyr target·보호 저장·FOTA slot·마이크/버저/전원·패스키 호환·거리 측정 실패·반납 복구 정책 (D03, D05, D06, D07, D09, RR-DEC-01)
- 선택 후 구체화: 보드 핀맵·flash/RAM/slot 배치·키/epoch 저장 및 migration·BLE codec/흐름제어·버튼/디스플레이 상태·지원 RP 벡터
- 구현 단계 검증: 실기 키 생성/import/서명·서로 다른 요청 거절·FOTA 전원중단·녹음/찾기 공존·패스키 등록/인증·기기 반납과 앱 상태
- 판정 경계: 실기 자원/거리/배터리 수치를 설계 검사로 확정하지 않음. 하드웨어 측정은 구현 단계에서 수행한다는 기존 방침 유지

### DP-02 · 유저 앱·로그인·기기 설정

요구 4, 5, 7, 8 · [DS-01](../specifications/device-coexistence-design.md), [DS-02](../specifications/social-wallet-recovery-design.md), [DS-03](../specifications/kiosk-commerce-journey-design.md), [DS-07](../specifications/recording-travel-ai-design.md)

- 작성된 논리 설계: 로그인/연결/갱신·HW와Cloud 지갑 구분·기기 설정·원 결과/소유자 범위
- 선택 대기: 추가 OS·유저 앱 framework·설치 경로·OAuth profile·BLE/보호 객체 수명·동의/위치 정책 (D01, D02, D03, D07, D17)
- 선택 후 구체화: 플랫폼별 로그인/딥링크·세션/기기 binding·BLE adapter·권한 거절/재로그인·보호 객체 조회/캐시 schema
- 구현 단계 검증: Google/Apple 실제 로그인·앱 설치·실기 설정/import/FOTA·이전 계정 격리·원음/위치 동의·가맹점 지갑 컨텍스트
- 판정 경계: 키오스크 RN 확정과 유저 앱 RN 제안을 구분. 요청별 읽기와 신규 서명 권한을 합치지 않음

### DP-03 · Cloud Wallet·MPC

요구 6 · [DS-02](../specifications/social-wallet-recovery-design.md)

- 작성된 논리 설계: 참여자 역할·생성/임계 서명/refresh·원 복구 checkpoint·late worker 활성화 조건
- 선택 대기: MPC 프로토콜/provider revision·참여자 실제 통제·quorum·복구 인증·share 저장/백업 (D03, D04, D11, D19)
- 선택 후 구체화: 선택 MPC adapter transcript/메시지·epoch 전환·참여자 교체·정확한 signer 경로·저장/복구 계약
- 구현 단계 검증: 실제 분산 생성/서명·전체키 재조립 없음·참여자 장애·소셜 탈취·회전/복구·앱 결과
- 판정 경계: 소셜 로그인만으로 복구 승인 불가. HW 지갑 키 저장 설계를 MPC 기능으로 계산하지 않음

### DP-04 · RN 키오스크·매장 운영

요구 9 · [DS-03](../specifications/kiosk-commerce-journey-design.md)

- 작성된 논리 설계: 주문/가격 snapshot·지급/환불·정산/혜택 보정·공용 단말 인계
- 선택 대기: 태블릿/공유 잠금·견적/환율/라운딩·확정 기준·환불 주소/gas·마감/혜택 (D01, D08, D09, D19)
- 선택 후 구체화: RN 태블릿 업무 화면·가게/단말 역할·주문/지급 DTO·환불 signer·대사/마감 correction·단말 정리 채택
- 구현 단계 검증: 실기 주문→지갑 서명→테스트넷→Indexer→매출/앱 영수증·부분환불·가게 간 접근 차단
- 판정 경계: 점주 업무 승인과 자금 signer는 별도. 결제 성공 표시를 운영자가 수동 생성하지 않음

### DP-05 · 백오피스·관리 신뢰

요구 10 · [DS-08](../specifications/operations-release-acceptance-design.md), [DS-03](../specifications/kiosk-commerce-journey-design.md)

- 작성된 논리 설계: 대여/FOTA/예외·profile rollout·관리 bootstrap/교체/복구·증거 수명·운영 역할/화면
- 선택 대기: OOB/독립 복구 주체·명령별 승인/거절 집계·시계·보호 checkpoint·보존/export·긴급 권한 (D02, D05, D19)
- 선택 후 구체화: 선택 정책별 관리 command/reader·TE 증거 encoding·권한/저장 adapter·O04 확장·다중 저장소 조정/복구 runbook
- 구현 단계 검증: 실제 현재 권한·자기 승인 차단·키 교체·unknown 조정·복구 후 제한 유지·원문/비밀 노출 차단
- 판정 경계: O04 API-088은 조회다. TO 패널 추가가 쓰기 API나 관리자 등록을 의미하지 않음

### DP-06 · 테스트넷 토큰·상품 컨트랙트

요구 11 · [DS-04](../specifications/stablenet-compatibility-design.md), [DS-05](../specifications/market-product-design.md), [DS-06](../specifications/credential-paid-resource-design.md)

- 작성된 논리 설계: 자산/가스 구분·9개 컨트랙트 영역·스마트 계정·자격/유료 자원의 독립 상태
- 선택 대기: 체인 capability·배포 identity·dummy USDC/WKRC 의미·계정/상품 모델·DID/proof·x402 mechanism (D10, D11, D12, D13, D14, D15, D16)
- 선택 후 구체화: 각 선택 모델 ABI/이벤트/단위/권한·배포 manifest·signer/submitter/source-reader·오류/환불/재조회 벡터
- 구현 단계 검증: 9개 기능별 실제 testnet 호출/이벤트와 앱 결과·가스/단위·직접호출 제한·UserOp 내부 실패·유료 전달 실패
- 판정 경계: USDC 더미를 공식 USDC로 표기하지 않음. 네이티브 가스와 wrapped/결제 토큰은 실제 manifest로 구분

### DP-07 · Indexer·조회 프런트엔드

요구 12 · [DS-04](../specifications/stablenet-compatibility-design.md)

- 작성된 논리 설계: 수집/해석/최종성/투영·reorg/재처리·정확한 deployment와 cursor
- 선택 대기: 재사용 repo revision·실 endpoint·ABI/배포 block·확정 규칙·보존/재구축 (D02, D08, D10, D19)
- 선택 후 구체화: 기존 backend/frontend와 이벤트별 adapter·decoder·중복키/cursor·reorg 보정·조회 권한/schema를 고정
- 구현 단계 검증: 실체인 이벤트→조회 화면·중복/재시작/재구축·reorg→주문/환불/혜택 보정·지연 표시
- 판정 경계: repo 존재를 지원 증거로 삼지 않음. 최신 코드 재검토/네트워크 검증은 이 문서 감사에서 수행하지 않음

### DP-08 · DEX·FX·Perpetual 앱 서비스

요구 13 · [DS-05](../specifications/market-product-design.md), [DS-04](../specifications/stablenet-compatibility-design.md)

- 작성된 논리 설계: 견적/allowance/실행·포지션/펀딩/청산·keeper 중복/가격 만료 상태
- 선택 대기: pair/pool·가격/유동성·라운딩/슬리피지·담보/margin/funding·keeper (D11, D12, D13)
- 선택 후 구체화: 선정 상품 산식·정수 단위/한계값 벡터·quote/sign/execute adapter·가격 시계·keeper 권한·UI 위험/결과
- 구현 단계 검증: 각 상품 앱에서 실견적/실행/이력·allowance 잔존·stale 가격·펀딩/청산·중복 keeper
- 판정 경계: 계약 배포만으로 앱 서비스 완료 처리하지 않음. 실제 상용 상품 허용 여부와 테스트 기능 수용을 구분

### DP-09 · 여행·기록·추천·챌린지

요구 14 · [DS-07](../specifications/recording-travel-ai-design.md), [DS-03](../specifications/kiosk-commerce-journey-design.md)

- 작성된 논리 설계: 녹음→전사·보호 결과·동의/삭제·장소/코스·후기 근거·보상 효과 단일 작성자
- 선택 대기: 지도/전사/AI·위치 정밀도·데이터 출처/보존·추천 품질/이동·챌린지 혜택 (D07, D09, D17, D18, D19)
- 선택 후 구체화: 선정 provider 입출력·목적별 동의/삭제 fanout·코스 제약·원본 구간 연결·수정/apply revision·보상 규칙
- 구현 단계 검증: 실기 녹음/폰 파일·실위치/장소·구매후기·코스 제약·환불 뒤 혜택·외부 pending/backup 삭제
- 판정 경계: 시험 결제와 실제 구매/방문을 구분. AI 생성과 사용자의 수동 편집은 다른 조건으로 처리

### DP-10 · 공통 기반·릴리스·운영 도구

요구 15 · [DS-08](../specifications/operations-release-acceptance-design.md)

- 작성된 논리 설계: 환경 manifest·릴리스/백업·수용 증거·장애/운영 인계·관리 정책 적용
- 선택 대기: 배포/비밀 관리·관측 범위·RPO/RTO·성능/보관·긴급 접근·실운영 출시 단계 (D02, D10, D19)
- 선택 후 구체화: 배포 단위별 환경/권한·restore/checkpoint·관측 지표/민감정보 제외·제품별 evidence index·릴리스 runbook
- 구현 단계 검증: 실제 빌드/배포·백업복구·삭제/철회 유지·중복 금융효과 차단·15개 요구의 앱/실기 수용
- 판정 경계: 수용 증거 없는 완료율을 산출하지 않음. 테스트넷 준비와 실자산 출시 승인은 분리

## 기준 채택에서 빠뜨리지 않을 묶음

HG-03 채택 범위의 최신 보완 목록이다. 기존 DI/OC 수량에 새 명령 수를 합쳐 정식 API 수량으로 표기하지 않는다.

| 묶음 | 범위 | 함께 대조할 대상 | 종료 증거 |
|---|---|---|---|
| [CAF-01 · 기존 DI/OC·lifecycle 보완](../planning/integration-adoption-matrix.json) | DI 8개와 OC 26개 | 기존 API/BLE/ACL/DTO/storage/source-reader·환불/반납/삭제/혜택 보정 | 원 계약 보존 조건과 물리 저장·원자성의 동일 기준 |
| [CAF-02 · 선택 profile·행위 gate](../specifications/selection-profile-contract.json) | 20 profile·82 미선택 필드·53 행위 gate | 설정 schema·선택 기록·연산 분기·미선택 오류·각 화면 | 행위별 관련 필드만 gate, 미선택 상태/읽기/제한/원 요청 복구 구분 |
| [CAF-03 · 설정 적용·관리 제어](../specifications/profile-control-adoption-map.json) | PRC 7개·PCP 7개·BLE 후보 3개 | 권한/HTTP 후보·BLE·저장/중복 결과·participant checkpoint·오류/UI | 서버 활성화와 peer 적용 분리·현재 권한·원 결과·restore |
| [CAF-04 · 관리 신뢰·증거·운영 화면](../specifications/trust-operations-design.json) | TMC 8개·TE 7종·운영 역할 10개·패널 7개 | 보호 운영 command/reader·trust/evidence 저장·독립 검증·O04/O03 패널·보존 | 초기 OOB/복구·현재 권한/독립성·단회 소비·별도 resume·API088 읽기 보존 |

## 설계 종료와 실행의 순서

```mermaid
flowchart LR
 A["관련 정책·profile 선택"] --> B["선택 후 구체 설계"]
 B --> C["공통 채택 checkpoint"]
 C --> D["명시적 구현 전환 + task 입력"]
 D --> E["제품 구현·실환경 연결"]
 E --> F["15요구 기능 수용"]
 F -. "실자산 운영 시 별도" .-> G["출시 판단"]
```

위 순서는 전체 통합 기준의 종료 관계다. 모든 정책을 기다려 모든 작업을 막는 전역 gate나 개인별 일정이 아니다. 구현 전환 이후 개별 task는 해당 입력/fixture 경계에 따라 독립 진행 가능하나, fixture 통과를 선정 profile 통합 완료로 합산하지 않는다. 외부 계정 준비·실운영 검토 자료 준비는 선행 가능하며 CS-05/06은 완료 판단 시점을 나타낸다.

| 순서 | 종료 산출물 | 현재 상태 |
|---|---|---|
| CS-01 · 관련 선택 고정 | 기존 PC/PF에 선택 근거·scope·버전 기록. 선택하지 않은 연산의 보류 동작 유지 | awaiting_selections |
| CS-02 · 선정 모델의 구체 계약 | 제품별 핀맵/schema/ABI/저장·오류/벡터와 검토 결과 | conditional_design_remaining |
| CS-03 · 공통 채택 checkpoint | CAF-01~04의 적용 대상·호환/취소 조건·같은 기준의 변경 검증 | candidate_not_merged |
| CS-04 · 구현 전환 확인 | 사용자의 명시적 구현 전환 지시와 해당 task 입력/fixture 경계 확인 | deferred_by_user |
| CS-05 · 실환경 연결·기능 수용 | 실제 환경과 제품 구현에 연결된 15요구·16facet·12여정의 증거 | not_run |
| CS-06 · 실운영 출시 검토 | 테스트 수용과 별도의 실제 운영 범위·통제·출시 판단 | not_assessed |

## 복합 요구 누락 점검

| 요구 | 세부 기능 | 기준 |
|---|---|---|
| 3 | passkey | 실제 기기 + 앱 연동 및 각 기능 고유 증거 |
| 3 | recorder | 실제 기기 + 앱 연동 및 각 기능 고유 증거 |
| 3 | device_find | 실제 기기 + 앱 연동 및 각 기능 고유 증거 |
| 3 | payment_stamp | 실제 기기 + 앱 연동 및 각 기능 고유 증거 |
| 11 | USDC/더미 토큰 | 실제 테스트넷 호출/이벤트와 앱 결과, 상품별 조건 별도 |
| 11 | WKRC native/wrapped 구분 | 실제 테스트넷 호출/이벤트와 앱 결과, 상품별 조건 별도 |
| 11 | DeFi | 실제 테스트넷 호출/이벤트와 앱 결과, 상품별 조건 별도 |
| 11 | smart_account | 실제 테스트넷 호출/이벤트와 앱 결과, 상품별 조건 별도 |
| 11 | FX | 실제 테스트넷 호출/이벤트와 앱 결과, 상품별 조건 별도 |
| 11 | perpetual | 실제 테스트넷 호출/이벤트와 앱 결과, 상품별 조건 별도 |
| 11 | STO | 실제 테스트넷 호출/이벤트와 앱 결과, 상품별 조건 별도 |
| 11 | DID | 실제 테스트넷 호출/이벤트와 앱 결과, 상품별 조건 별도 |
| 11 | x402 | 실제 테스트넷 호출/이벤트와 앱 결과, 상품별 조건 별도 |
| 13 | DeFi 실행/유동성 | 계약 동작과 별개로 앱 견적/실행/결과 연결 |
| 13 | FX 견적·교환 | 계약 동작과 별개로 앱 견적/실행/결과 연결 |
| 13 | Perpetual 포지션·펀딩·청산 | 계약 동작과 별개로 앱 견적/실행/결과 연결 |

## 통합 수용 여정

다음 12개 여정은 모두 실행 전이다. 실제 제품·환경 증거는 원 인계서의 기준을 유지한다.

| ID | 여정 | 실패·복구 조건 |
|---|---|---|
| EA-01 | 소셜가입→Cloud MPC→서명→복구 | 탈취session/중단복구/lateworker/threshold불충분 |
| EA-02 | 대여→HW신규/import→지급→반납 | BLE단절/import실패·unknown송금·RR선택된복구절차 |
| EA-03 | RN키오스크→카페주문→HW승인→매출 | 중복/오입금/응답유실/가격변경/chain reorg |
| EA-04 | 부분환불→스탬프→정산→백오피스 | 환불미확정/사용한혜택/마감뒤보정/무권한publish |
| EA-05 | passkey·녹음·찾기·스탬프+FOTA | 녹음gap/권한거절/거리unknown/FOTA전원중단과키보존 |
| EA-06 | EOA→스마트계정 | bundle성공/내부실패·allowance잔존·latehold |
| EA-07 | USDC대체·WKRC·DeFi·FX·Perp | unsupportedtoken/staleprice·청산/keeper중복·reorg |
| EA-08 | DID→STO자격→시험권리동작 | revocation race·unknownstatus·부적격이전 |
| EA-09 | x402→유료결과→재조회 | 결제후응답유실·provider실패·중복재시도 |
| EA-10 | 기기녹음→폰→전사/AI→삭제 | 부분녹음/외부pending/backuprestore |
| EA-11 | 맛집→구매후기→코스→챌린지 | 위치거절/없는장소/삭제입력/환불보정 |
| EA-12 | 전체릴리스→백업복구→운영인계 | 오래된권한/삭제자료부활/중복금융효과 차단 |

## 보완 사항

- **DCR-01**: 기존 HG-03의 DI/OC 목록만으로 최근 profile/trust 보완의 채택을 완료했다고 해석할 수 있음 → CAF-02~04를 인계 종료 대상에 명시. 원 인계서를 보존하고 이 보완판을 README의 최신 진입점으로 연결.
- **DCR-02**: 제품 준비 상태 하나로 미선택·미채택·실기 미검증을 혼동할 수 있음 → DC-01~06 및 제품별 선택/구체 설계/실측 항목 분리. implementationReady/runtimeVerified 모두 false 유지.

## 검사 결과와 한계

10개 제품 관점에서 15개 요구·104개 작업의 영향 연결, 16개 복합 기능·12개 통합 여정, 미선택 상태와 참조 해시를 검사한다. 새 제품 코드·실환경 호출·실기 시험·공급자/법규 재조사는 수행하지 않았다. 해당 실검증 항목은 완료 처리하지 않는다. 완료율이나 12주 일정 타당성을 이 검사로 산출하지 않는다.

[구조화 보완판](design-closure-review.json) · [검증 기록](design-closure-validation.json) · [운영 책임·승인 화면](../specifications/trust-operations-design.md)

다음은 미선택 값을 채우지 않고도 작성 가능한 채택 checkpoint의 변경 대상·호환 조건·동시 반영 순서를 통합한다. 기존 DI/OC와 새 profile/trust 후보를 하나의 검토 목록으로 묶으며 실제 기준 병합·제품 구현은 하지 않는다.

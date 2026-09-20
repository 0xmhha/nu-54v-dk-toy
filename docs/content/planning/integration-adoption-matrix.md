# 미병합 설계 통합 묶음과 수용 기준

2026-09-18 · **기존 상세 설계의 통합표. 이번에 기준 카탈로그 병합·제품 구현·DB 실행은 하지 않았다.**

최근 결제·환불·정산·소비자 보정 설계를 기존 DI-01~08에 다시 연결했다. 새로운 WBS 작업이나 개인별 배정을 만든 것이 아니다. 설계가 작성됐다는 사실과 기준 명세에 채택됐다는 사실, 실제 동작 검증을 분리한다.

[구조 원본](integration-adoption-matrix.json) · [검사/렌더러](render_integration_adoption.py) · [검증 실행 기록](integration-adoption-validation.json) · [전체 설계 등록부](design-integration-register.md)

## 1. 현재 상태

| 영역 | 현재 판정 | 의미 |
|---|---|---|
| 승인 HTTP/BLE/권한/화면 | 기준에 이미 통합 | API110·권한60·논리자원28. 암호·실기·새SQL 검증은 미완료 |
| 승인 물리 저장 | 상세 후보 작성 | 테이블 후보16·이행8단계. 기존 SQL61/7에는 미반영 |
| 결제·환불/매출·정산/운영 | 전체 계약 후보 작성 | 기존 HTTP10개 중 API037은 보존 검토, 나머지9개는 변경안. SR2개는 미등록 |
| 이벤트·소비자 보정 | 상세 후보 작성 | 이벤트2개 v2, 소비자5개·논리자원6개·재구축7단계. 카탈로그 미병합 |
| 반납 | 논리 프로토콜·상태/화면 후보 | RP8개는 HTTP/BLE 전체 경로 8개라는 뜻이 아님. 호출 방향·권한·조회 조합 보완 필요 |
| 전체 제품 | 범위 유지, 설계 진행 | 15요구/104작업/320세부작업/19결정. 3명·12주·앱 연동 목표 유지 |

예전 후보의 API107/보안자원15 표기는 작성 당시 이력이다. 현재 기준과 더하거나 최신 숫자로 과거 파일을 덮어쓰지 않는다. logical resource·table candidate·실제 reference SQL table 수 역시 서로 다른 분류다.

## 2. 통합 묶음 8개

각 묶음의 파일은 **같은 설계 변경으로 함께 검토할 대상**이다. 여러 파일을 동시에 수정한다는 뜻이지 서비스 간 분산 transaction을 보장한다는 뜻이 아니다. 소스 후보는 원본 hash로 고정하고, 실제 채택 단계에서 전후 checkpoint와 변경 처분을 새로 남긴다.

### DI-01 · 결제·환불 및 매출·정산·운영 HTTP/권한

완료 선행: 없음 · 결정 연결: D02, D08, D19

근거: [commerce-reconciliation-integration.json](../specifications/commerce-reconciliation-integration.json) · [settlement-ops-contracts.json](../specifications/settlement-ops-contracts.json)

반영 대상: [api-catalog.json](../specifications/api-catalog.json) · [critical-dtos.json](../specifications/critical-dtos.json) · [critical-dtos.schema.json](../specifications/critical-dtos.schema.json) · [extended-dtos.json](../specifications/extended-dtos.json) · [extended-dtos.schema.json](../specifications/extended-dtos.schema.json) · [core.schema.json](../specifications/core.schema.json) · [authorization-policies.json](../specifications/authorization-policies.json) · [api-access-transactions.json](../specifications/api-access-transactions.json)

**남은 구체 작업**

- 두 제안의 서로 다른 contractVersion/header를 경로별로 협상하는 방식과 오류 호환
- SR별 정책 ID/정식 API ID 할당 및 ops general operation ACL 연결
- D08 미선정 정책을 명시적 hold/profile 경계로 보존

| 수용 ID | 계층 | 판정 기준 | 현재 |
|---|---|---|---|
| DI-01-A1 | 설계 채택 | API037 merchantSigner/snapshot과 API018 source guard 의미를 보존하고 나머지 변경 endpoint 전체 envelope/오류·권한을 함께 연결 | 기준 정의·채택 미완료 |
| DI-01-A2 | 결정/미정 경계 | D08 수락/합산/예외반환, 정산 partition/보정, SR publish권한 정책의 출처 또는 미선정 상태를 기록 | 기준 정의·채택 미완료 |
| DI-01-A3 | 실제 실행 | 다중payer 조회·동시예약·stale마감·반영대기권한철회·동시publication을 실제 환경에서 검사 | 미실행 |

### DI-02 · 이벤트 v2와 소비자 보정

완료 선행: DI-01 · 결정 연결: D02, D08, D09, D17, D18, D19

근거: [commerce-reconciliation-integration.json](../specifications/commerce-reconciliation-integration.json) · [commerce-consumer-repair-design.json](../specifications/commerce-consumer-repair-design.json)

반영 대상: [event-catalog.json](../specifications/event-catalog.json) · [core.schema.json](../specifications/core.schema.json) · [storage-operations.md](../specifications/storage-operations.md) · [screen-flows.json](../specifications/screen-flows.json)

**남은 구체 작업**

- DomainEvent v1과 두 v2 envelope의 명시적인 dispatcher/adapters 연결
- settlements 구독과 refund→travel 구독 추가
- ownership commit의 durable trigger, consumer manifest provider 및 checkpoint protocol

| 수용 ID | 계층 | 판정 기준 | 현재 |
|---|---|---|---|
| DI-02-A1 | 설계 채택 | 기존 이벤트 이름10개를 유지하고 v2가 바뀌는두이벤트만명시; 나머지8개회귀,구독delta·원천revision범위연결 | 기준 정의·채택 미완료 |
| DI-02-A2 | 결정/미정 경계 | 혜택/여행 ruleVersion과 이미소비한보상의 정책미정은 held로유지 | 기준 정의·채택 미완료 |
| DI-02-A3 | 실제 실행 | v1/v2중복·누락·역순·원천epoch변경·재구축중삭제·단일consumer최종commit검사 | 미실행 |

### DI-03 · 승인·결제·소비자 물리/논리 저장 정합

완료 선행: DI-01, DI-02 · 결정 연결: D02, D03, D08, D09, D17, D19

근거: [approval-storage-mapping.json](../specifications/approval-storage-mapping.json) · [approval-physical-storage-design.json](../specifications/approval-physical-storage-design.json) · [commerce-reconciliation-integration.json](../specifications/commerce-reconciliation-integration.json) · [commerce-consumer-repair-design.json](../specifications/commerce-consumer-repair-design.json)

반영 대상: [security-storage-contracts.json](../specifications/security-storage-contracts.json) · [storage-operations.md](../specifications/storage-operations.md) · [database/schema-catalog.json](../specifications/database/schema-catalog.json) · [database/README.md](../specifications/database/README.md)

**남은 구체 작업**

- 승인후보16구조·commerce후보4구조·consumer논리6자원의중복/공유권위를대조한단일mapping
- 운영DB/isolation/보호저장/manifest일관성protocol과잠금구현상세
- 새DDL은향후별도추가; 기존7파일과원검증대상보존

| 수용 ID | 계층 | 판정 기준 | 현재 |
|---|---|---|---|
| DI-03-A1 | 설계 채택 | source/authorization gate·funding guard·reservation·consumer checkpoint의권위를중복생성하지않고동일commit책임을고정 | 기준 정의·채택 미완료 |
| DI-03-A2 | 결정/미정 경계 | 삭제/보관·복원최신deny근거·기간/자산정책미정을가짜기본값으로채우지않음 | 기준 정의·채택 미완료 |
| DI-03-A3 | 실제 실행 | 새DDL이후FK/CAS/동시철회/전원또는process손실/복원replay시험; 기존61테이블시험을새제약증거로재사용금지 | 미실행 |

### DI-04 · 반납 서버 경로·허가·증거 접수

완료 선행: 없음 · 결정 연결: D02, D03, D09

근거: [return-protocol-candidate.json](../specifications/return-protocol-candidate.json) · [return-recovery-contract.json](../specifications/return-recovery-contract.json)

반영 대상: [api-catalog.json](../specifications/api-catalog.json) · [critical-dtos.schema.json](../specifications/critical-dtos.schema.json) · [extended-dtos.schema.json](../specifications/extended-dtos.schema.json) · [authorization-policies.json](../specifications/authorization-policies.json) · [api-access-transactions.json](../specifications/api-access-transactions.json)

**남은 구체 작업**

- RP-01..08의전송방향/HTTP·BLE소유자/전체envelope/오류경로확정
- 온라인commit/relay권한발급/cleanup challenge·증거접수경로의실제매핑
- RP08만으로재대여판정불가:현재job/epoch/binding/cleanup/gate같은시점의조회계약

| 수용 ID | 계층 | 판정 기준 | 현재 |
|---|---|---|---|
| DI-04-A1 | 설계 채택 | 각RP를실제기존API변경·신규HTTP후보·BLE메시지·내부증거로구분;논리RP를전부신규API로세지않음 | 기준 정의·채택 미완료 |
| DI-04-A2 | 결정/미정 경계 | RR-DEC-01 미답변이면신규파괴적reset허가를추론하지않고기존증거조회·복구설계를계속 | 기준 정의·채택 미완료 |
| DI-04-A3 | 실제 실행 | commit응답유실·오래된clearance·늦은증거·cleanup중단·다른대여세대접근차단시험 | 미실행 |

### DI-05 · 반납 BLE/펌웨어·신뢰 profile

완료 선행: DI-04 · 결정 연결: D02, D03, D05, D06, D09

근거: [return-protocol-candidate.json](../specifications/return-protocol-candidate.json) · [return-recovery-contract.json](../specifications/return-recovery-contract.json) · [implementation-interfaces.json](../specifications/implementation-interfaces.json)

반영 대상: [ble-catalog.json](../specifications/ble-catalog.json) · [core.schema.json](../specifications/core.schema.json) · [implementation-interfaces.json](../specifications/implementation-interfaces.json) · [compatibility-matrix.json](../specifications/compatibility-matrix.json)

**남은 구체 작업**

- owner와reset_recovery_relay역할별명령/허용목적
- digest/encoding/holder proof/encryption/ACK/epoch/journal의실제profile
- Zephyr SDK/board target/보호영역및원자지움검증입력

| 수용 ID | 계층 | 판정 기준 | 현재 |
|---|---|---|---|
| DI-05-A1 | 설계 채택 | reset.prepare/confirm의현재34명령기준변경과새role호환을연결;다른녹음/FOTA/결제명령회귀 | 기준 정의·채택 미완료 |
| DI-05-A2 | 결정/미정 경계 | Zephyr선정만확정; NU와Nordic보드·폰의실제지원능력을같게간주하지않음 | 기준 정의·채택 미완료 |
| DI-05-A3 | 실제 실행 | 전원차단/재부팅/초기화후서명된증거/rollback·재연결을실제기기에서검사 | 미실행 |

### DI-06 · 반납 종료·정리·재대여 원자 경계

완료 선행: DI-04, DI-05 · 결정 연결: D02, D03, D09

근거: [return-protocol-candidate.json](../specifications/return-protocol-candidate.json) · [return-recovery-contract.json](../specifications/return-recovery-contract.json) · [return-screen-design.json](../specifications/return-screen-design.json)

반영 대상: [storage-operations.md](../specifications/storage-operations.md) · [security-storage-contracts.json](../specifications/security-storage-contracts.json) · [database/004_device_lifecycle.sql](../specifications/database/004_device_lifecycle.sql) · [api-access-transactions.json](../specifications/api-access-transactions.json)

**남은 구체 작업**

- returned와cleanup/reenrollment gate분리
- ResetJob·DeviceResetJournal·RecoveryRelayGrant·ReenrollmentGate물리/기기저장mapping
- 등록mutation의현재epoch/binding/gate재검사와삭제tombstone

| 수용 ID | 계층 | 판정 기준 | 현재 |
|---|---|---|---|
| DI-06-A1 | 설계 채택 | 원SQL004는이번에수정하지않고향후추가migration으로의변경차이를설계;server/device원자성을가정하지않음 | 기준 정의·채택 미완료 |
| DI-06-A2 | 결정/미정 경계 | 신규여행지갑복구정책과import지갑의범위를분리;타자산강제이체없음 | 기준 정의·채택 미완료 |
| DI-06-A3 | 실제 실행 | 완료ACK후cleanup실패/marker유실/재등록경쟁/다음대여자격리시험 | 미실행 |

### DI-07 · 화면·행동·현재권한·조회 조합

완료 선행: DI-01, DI-04, DI-06 · 결정 연결: D01, D02, D03, D08, D09, D17, D19

근거: [screen-flows.json](../specifications/screen-flows.json) · [return-screen-design.json](../specifications/return-screen-design.json) · [commerce-reconciliation-integration.json](../specifications/commerce-reconciliation-integration.json) · [settlement-ops-contracts.json](../specifications/settlement-ops-contracts.json)

반영 대상: [screen-flows.json](../specifications/screen-flows.json) · [screen-flows.md](../specifications/screen-flows.md) · [common-flows.json](../specifications/common-flows.json)

**남은 구체 작업**

- 현재37화면의모든버튼에실제route/alias/guard/결과조회연결
- U24/D01/O01/U04/U05가동일job/epoch의상태로만반납/재등록판정
- K06/O03의stale/held/202/완료와U11 allocation별소유분리

| 수용 ID | 계층 | 판정 기준 | 현재 |
|---|---|---|---|
| DI-07-A1 | 설계 채택 | 새화면을무조건추가하지않고기존화면동작표갱신;미등록route는지원대기로표시 | 기준 정의·채택 미완료 |
| DI-07-A2 | 결정/미정 경계 | 데이터미가용을정상0/완료로표시하지않고알수없는정책은사용자승인으로포장하지않음 | 기준 정의·채택 미완료 |
| DI-07-A3 | 실제 실행 | 앱·태블릿·기기재연결/권한회수·중복클릭·부정확한완료표시종단시험 | 미실행 |

### DI-08 · 원본 보존·명세 채택·증거·범위 추적

완료 선행: DI-01, DI-02, DI-03, DI-04, DI-05, DI-06, DI-07 · 결정 연결: D02, D19

근거: [approval-baseline.json](../specifications/approval-baseline.json) · [settlement-ops-contracts.json](../specifications/settlement-ops-contracts.json)

반영 대상: [api-catalog.json](../specifications/api-catalog.json) · [authorization-policies.json](../specifications/authorization-policies.json) · [security-storage-contracts.json](../specifications/security-storage-contracts.json) · [compatibility-matrix.json](../specifications/compatibility-matrix.json)

**남은 구체 작업**

- 개별묶음의schema/카탈로그/예제/권한/화면/저장와generatedviews동시반영
- 현재source hash와새채택checkpoint를남기고후보상태를부분/전체채택으로기록
- WBS104/320·15요구·19결정·IF14/COMP26의추적과회귀

| 수용 ID | 계층 | 판정 기준 | 현재 |
|---|---|---|---|
| DI-08-A1 | 설계 채택 | 이번통합표는설계분류이며canonical병합아님;실제채택시전후원본보존·validator실행출처명시 | 기준 정의·채택 미완료 |
| DI-08-A2 | 결정/미정 경계 | 최종Seed/구현/개인배정/공수·정책을다음진행만으로승인하지않음 | 기준 정의·채택 미완료 |
| DI-08-A3 | 실제 실행 | 문서검사/DB시험/암호검증/실기종단증거를분리하고실제증거없는완료율표시금지 | 미실행 |

## 3. HTTP/프로토콜 처분표

| 분류 | ID | 처리 | 근거 |
|---|---|---|---|
| existing_http | API-030 | proposed_full_envelope_replacement | [commerce-reconciliation-integration.json](../specifications/commerce-reconciliation-integration.json) |
| existing_http | API-035 | proposed_full_envelope_replacement | [commerce-reconciliation-integration.json](../specifications/commerce-reconciliation-integration.json) |
| existing_http | API-036 | proposed_full_envelope_replacement | [commerce-reconciliation-integration.json](../specifications/commerce-reconciliation-integration.json) |
| existing_http | API-037 | preserve_current_schema_semantic_guard_alignment | [commerce-reconciliation-integration.json](../specifications/commerce-reconciliation-integration.json) |
| existing_http | API-038 | proposed_full_envelope_replacement | [commerce-reconciliation-integration.json](../specifications/commerce-reconciliation-integration.json) |
| existing_http | API-039 | proposed_full_envelope_replacement | [settlement-ops-contracts.json](../specifications/settlement-ops-contracts.json) |
| existing_http | API-040 | proposed_full_envelope_replacement | [settlement-ops-contracts.json](../specifications/settlement-ops-contracts.json) |
| existing_http | API-041 | proposed_full_envelope_replacement | [settlement-ops-contracts.json](../specifications/settlement-ops-contracts.json) |
| existing_http | API-093 | proposed_full_envelope_replacement | [settlement-ops-contracts.json](../specifications/settlement-ops-contracts.json) |
| existing_http | API-087 | proposed_full_envelope_replacement | [settlement-ops-contracts.json](../specifications/settlement-ops-contracts.json) |
| uncatalogued_http_alias | SR-01 | needs_identifier_policy_and_catalog_adoption | [settlement-ops-contracts.json](../specifications/settlement-ops-contracts.json) |
| uncatalogued_http_alias | SR-02 | needs_identifier_policy_and_catalog_adoption | [settlement-ops-contracts.json](../specifications/settlement-ops-contracts.json) |
| logical_return_protocol | RP-01 | transport_direction_and_full_route_mapping_pending | [return-protocol-candidate.json](../specifications/return-protocol-candidate.json) |
| logical_return_protocol | RP-02 | transport_direction_and_full_route_mapping_pending | [return-protocol-candidate.json](../specifications/return-protocol-candidate.json) |
| logical_return_protocol | RP-03 | transport_direction_and_full_route_mapping_pending | [return-protocol-candidate.json](../specifications/return-protocol-candidate.json) |
| logical_return_protocol | RP-04 | transport_direction_and_full_route_mapping_pending | [return-protocol-candidate.json](../specifications/return-protocol-candidate.json) |
| logical_return_protocol | RP-05 | transport_direction_and_full_route_mapping_pending | [return-protocol-candidate.json](../specifications/return-protocol-candidate.json) |
| logical_return_protocol | RP-06 | transport_direction_and_full_route_mapping_pending | [return-protocol-candidate.json](../specifications/return-protocol-candidate.json) |
| logical_return_protocol | RP-07 | transport_direction_and_full_route_mapping_pending | [return-protocol-candidate.json](../specifications/return-protocol-candidate.json) |
| logical_return_protocol | RP-08 | transport_direction_and_full_route_mapping_pending | [return-protocol-candidate.json](../specifications/return-protocol-candidate.json) |

기존 HTTP10개는 commerce5개와 settlement/ops5개의 합집합이다. API037은 이미 채택된 전체 승인 계약을 유지하므로 신규 변경 건수로 다시 계산하지 않는다. API018/020도 이번 후보를 이유로 광범위한 union이나 결과 URL 경로로 변경하지 않는다. SR2개와 RP8개를 합쳐 미래 정식 API 개수를 예측하지 않는다.

## 4. 덮어쓰면 안 되는 기준

- **IM-01**: API037 current merchantSigner, refund context/snapshot and approver-versus-signer separation; never replace with old two-revision-only body.
- **IM-02**: API018 stored-source adapter, exact payload, shared gates and one-way permit; new commerce schemas do not create new signing/submit rights.
- **IM-03**: API020 parent/child/general server ancestry and protected result rules preserved. Ops progress mapping requires current source ACL; no raw result URL.
- **IM-04**: Financial reorg preserves possible exposure/reservation; release or grant revoke does not erase signatures or permit history.
- **IM-05**: Physical reference SQL61/7 unchanged; logical28+candidate16/4/6 structures are different categories, not a summed deployed table count.
- **IM-06**: SR02 is separate publish permission, one consumer/projection/fence per repair; 202 is request accepted, CP-B06 is actual atomic publication.
- **IM-07**: RR-DEC-01 selection stays null; no backup/export/reset approval inferred. Existing evidence recovery remains a separate non-new-reset design.
- **IM-08**: smart_account/market_action/credential_proof/paid_resource execution adapters stay blocked until separately designed; scope retained.

## 5. 채택 순서와 실행 경계

여기서 순서는 문서 검토 단계다. W01은 DI08의 원본 보호 작업만 선행 수행하며 DI08 전체 완료를 뜻하지 않는다. 각 DI의 완료 의존성은 2절을 따른다.

### IM-W01 · 현재 기준 보호·차이 동결

연결: DI-08

진입: 현재hash/카탈로그수/체크포인트174파일확인

출구: API037/018/020보존규칙과후보처분표·참조검사

### IM-W02 · commerce 문서 채택 준비

연결: DI-01, DI-02, DI-03

진입: 엔드포인트envelope/권한/이벤트/저장묶음간미정경계를명시

출구: schema-only초안채택은정책미정으로자동차단하지않음;정책미정은명시guard로유지. 실제service활성화는별도gate

### IM-W03 · 반납 경로 누락 보완

연결: DI-04, DI-05, DI-06

진입: RP별전송역할/경로와복구미답변상태확인

출구: fullHTTP/BLE계약·현재상태조회·cleanup/재대여gate일치. 복구정책미정인신규reset는보류

### IM-W04 · 전체 화면·기준 채택 검토

연결: DI-07, DI-08

진입: 서로연결된문서묶음의정합성·보존본/validator준비

출구: 원카탈로그·스키마·예제·권한·화면·논리저장·참조/생성문서동시변경;이번에는미실행

### IM-W05 · 구현 이후 수용 증거

연결: DI-01, DI-02, DI-03, DI-04, DI-05, DI-06, DI-07, DI-08

진입: 사용자구현단계전환과선정환경/profile

출구: DB/암호/실기/앱종단관측증거로runtime판정;문서형태검증과별도

정책이 미정이어도 선택지를 명시하고 미지원 실행을 차단하는 **초안 명세 통합**은 진행할 수 있다. 운영 정책의 선택이나 실제 서비스 활성화까지 완료됐다고 표시할 수는 없다. 반대로 실제 하드웨어 시험을 아직 하지 않았다는 이유로 모든 문서 통합을 무기한 막지도 않는다. 구현 착수는 사용자가 아직 보류한 상태다.

실제 채택에서는 alias 정식 ID와 권한·버전 협상을 함께 등록하고, 후보별 상태를 보존/부분반영/전체반영으로 기록한다. 카탈로그 개수를 먼저 바꾸고 미완성 schema나 검증되지 않은 alias를 완료로 채우지 않는다.

## 6. 정책 결정과 엔지니어링 작업 분리

RR-DEC-01은 이미 질문한 복구 정책이며 selection=null을 유지한다. 이번 작업에서 재질문하거나 새 backup/export/reset 정책을 선택하지 않는다. 아래 나머지 정책도 새로 승인받은 것으로 처리하지 않는다. D02/D10 같은 엔지니어링 항목은 사용자 취향 질문으로 넘기지 않고 문서·코드·프로필 근거를 모아 계속 구체화할 수 있다.

| 결정 | 사용자 판단 항목 | 계속 설계할 항목 | 실제 검증 대상 |
|---|---|---|---|
| D01 앱·인증 대상 | 추가 OS·태블릿·배포 경로·언어 우선순위 | 유저 앱 RN 채택안·native bridge 공유 경계 | 플랫폼별 설치/Google·Apple 인증 |
| D02 연결 규칙·데이터 경계 | 별도 취향 질문 없음 | API/BLE/proof 버전·인증 envelope·역할별 projection·정규화·원자 저장 경계 | 실제 profile 호환·권한/동시성 |
| D03 지갑 키·복구 | 신규 여행 지갑의 개인 복구 백업 추가 여부 또는 복구 없을 때 초기화 보류; HW/Cloud 지갑의 주소·복구 관계 | 키 파생·격리·import·장치 신원/epoch/journal 보호·늦은 증거 검증 | 원시 키 비노출·서명·삭제·초기화 후 복구 |
| D04 MPC 신뢰 구조 | MPC 참여 주체·복구 권한·서비스 중단 시 접근 정책 | 제공자/라이브러리·임계값·조각 보관 경계·세션 중단 규칙 | 실제 DKG/임계 서명·참여자 변경·복구 |
| D05 FOTA 운영 | 업데이트 시 사용자 중단/강제 여부와 운영 배포·복귀 정책 | Zephyr SDK/board target·부트로더/슬롯·서명 키 운영·저장 호환 | 서명 이미지·전원 차단·부팅 복귀·키 보존 |
| D06 패스키 호환 범위 | 우선 사용할 passkey 서비스와 기기/브라우저 | 표준 transport·등록/인증·대체 로그인 및 반납 자격 정리 | 실제 RP/OS/브라우저 조합 상호운용 |
| D07 녹음·찾기 목표 | 녹음 품질/길이·백그라운드 사용·찾기 반응 목표 | 마이크/출력 부품·버퍼·BLE 전송·동시 동작·거리 유효성 | 음성 누락·배터리/메모리·실제 거리/백그라운드 |
| D08 결제·환불·정산 | 최초 전체 지급 수락·분할 지급·늦은/중복 지급 반환 정책; 환불·수취·정산 운영 규칙과 비용 부담의 예외 | 원지급별 한도·확정/관측 보정·quote/환율 출처·목적지 증명 | 동시 지급/환불·reorg·정산 보정 |
| D09 대여·혜택 | 기존 D03 복구 질문을 공유하며 중복 질문하지 않음; 적립/사용·환불 후 사용된 혜택·회수 불가 기기 운영 정책 | 반납 fence·commit·증거 relay·ACK/cleanup·재대여 gate | 다음 대여자 격리·증거 유실·혜택 중복/보정 |
| D10 배포/시험 환경 | 별도 취향 질문 없음 | StableNet manifest·dummy token 권한·native/wrapped 구분·ABI/배포 block·Indexer endpoint | 실제 가스 수급·배포/조회·토큰 호출 |
| D11 스마트 계정 전환 | EOA 이후 주소/자산/권한 전환 경험과 복구 방식 | account/EntryPoint/SDK/Bundler 버전 묶음·HW/Cloud signer 지원 | 실제 UserOperation·전환/복구·거래 조회 |
| D12 DeFi·FX 상품 | DeFi 대표 동작·FX 상품 모델·시험 자산 쌍 | pool·유동성·가격·슬리피지·견적/실행 adapter | 실제 swap/LP/FX 실행·실패/가격 기한 |
| D13 Perpetual 상품 | 시험 시장·담보·레버리지·가격 장애 시 상품 정책 | 가격/오라클·펀딩·청산·keeper·위험 계산 규칙 | 실제 포지션/펀딩/청산·장애 복구 |
| D14 STO 범위 | 시험 발행물의 권리·발행자·전송/보유 자격 | STO 계약·자격·앱 동작·이벤트 모델 | 허용/거부/철회·잔액/권리 조회 |
| D15 DID 범위 | 자격의 내용·발급자/검증자·공개할 정보 | DID/credential/proof 표준·철회 모델·STO 연결 | 실제 발급/검증/철회·개인 정보 노출 방지 |
| D16 x402 범위 | 유료 자원과 재요청 과금 경험·고객 가스 부담 예외 여부 | x402 버전·token authorization 방식·facilitator·지급/전달 분리 | 선택한 토큰 방식·실제 gas payer·반복 과금 방지 |
| D17 위치·후기·실제 데이터 | 위치 수집·보관·삭제·후기 출처 정책 | 장소 제공자·장소/매장 연결·동의/삭제·실제/시험 구매 구분 | 위치 거절·삭제 전파·다중 출처 일치 |
| D18 추천·챌린지 기준 | 추천 코스 목표·제약·챌린지 완료/보상 기준 | AI 입력/검증·데이터 부족 대안·증거 철회 재평가 | 평가 표본·근거/영업시간·중복 보상 방지 |
| D19 운영·수용 목표 | 운영 역할·품질/복구 목표·실자산 파일럿 여부 | 관측/보존/복구·알림·지원·출시 증거·운영 접근 설계 | 성능/배터리·백업 복구·장애 대응·실자산 전 별도 검토 |

## 7. 15개 요구사항 범위 확인

아래는 기존 WBS에서 다시 읽은 연결이다. 최근 문서가 결제·운영에 집중돼도 FOTA·패스키·녹음·MPC·온체인 상품·여행 기능을 범위에서 제거하지 않는다. 작업은 여러 요구사항에 겹칠 수 있으며 행별 작업 수를 합산해 총 작업량/진행률로 사용하지 않는다. 역할·공수·일정 배정은 이번에 하지 않았다.

| 요구 | 기존 WBS 연결 수 | 상태 |
|---|---:|---|
| 1. 실제기기에서 hw wallet 으로 동작하는 펌웨어 구현 | 17 | 범위 유지·완료 판정 아님 |
| 2. 실제기기에서 펌웨어 업그레이드를 지원하는 fota 구현 | 9 | 범위 유지·완료 판정 아님 |
| 3. 실제기기에서 passkey, 녹음기, 디바이스 찾기, 결제 스탬프 지원 | 17 | 범위 유지·완료 판정 아님 |
| 4. 실제기기를 설정하는 유저 App 구현 | 25 | 범위 유지·완료 판정 아님 |
| 5. 유저 App 에서 Social Login 지원 (google, apple) | 14 | 범위 유지·완료 판정 아님 |
| 6. 유저 App 의 Social Login 에 따른 Cloud Wallet 지원 ( 키관리 MPC 지원 ) | 14 | 범위 유지·완료 판정 아님 |
| 7. 유저 App 에서 위치 기반 맛집 검색, 결제 기록, 발자취 지원 (여행자 모드 지원) | 22 | 범위 유지·완료 판정 아님 |
| 8. 유저 App 에서 실제기기 설정 지원 | 35 | 범위 유지·완료 판정 아님 |
| 9. 키오스크 App 구현 ( 가게 회원 가입 및 로그인(소셜 로그인과 동일), 가게 관리 지원(메뉴 관리, 매출 관리, 환불 처리, 정산 처리 등) ) | 26 | 범위 유지·완료 판정 아님 |
| 10. 백오피스 서비스 구현 및 지원 | 26 | 범위 유지·완료 판정 아님 |
| 11. stablenet testnet 기반 usdc, wkrc, defi, smart account, fx, perpetual , sto, did, x402 컨트랙트 지원 | 32 | 범위 유지·완료 판정 아님 |
| 12. stablenet testnet 기반 indexer 구현 및 지원 | 16 | 범위 유지·완료 판정 아님 |
| 13. stablenet testnet 기반 dex 서비스 구현 및 지원 ( defi , fx, perpetual ) | 17 | 범위 유지·완료 판정 아님 |
| 14. stablenet testnet 기반 여행자 서비스 지원 ( ai 기반 + 실제 결제 데이터 + 유저 후기 + 위치  = 추천 코스 생성 ( 따라하기 챌린지, 여행 발도장 ) | 16 | 범위 유지·완료 판정 아님 |
| 15. 그외 기타 필요한 툴 및 서비스 구현 | 17 | 범위 유지·완료 판정 아님 |

현재 source dispatch에서 smart_account/market_action/credential_proof/paid_resource는 adapter_pending_execution_blocked다. 이들은 12주 범위에 남아 있으며 개인 EOA 승인 adapter로 우회하지 않는다. passkey 실연동, BLE 녹음, MPC 참여자/키 통제, FOTA board target 등은 각 D/IF/COMP 검증 작업을 유지한다.

## 8. 증거 종류와 실행 사례 재사용

| 증거 | 입증하는 것 | 입증하지 않는 것 |
|---|---|---|
| source hash·파일/참조 검사 | 어느 설계를 비교했는지, 참조가 존재하는지 | 보안·정책 승인·실제 호출 가능성 |
| 합성 schema·산술 예제 | 정해진 입력 형태와 유한 불변식 | 인증 진위·DB 경쟁·메시지 장애 복구 |
| 기존 reference SQL 시험 | 당시 61개 테이블에 한정한 제약 결과 | 새 companion/adapter 제약 |
| 향후 실환경 증거 | 실제 버전·입력·관측 결과가 기록된 범위 | 모든 환경/미실행 기능에 대한 일반 보장 |

기존 실행 수용 사례 inventory(모두 미실행):

| 근거 | 항목 수 | 상태 |
|---|---:|---|
| [approval-physical-storage-design.json](../specifications/approval-physical-storage-design.json) · recoveryCases | 17 | not_run |
| [commerce-reconciliation-integration.json](../specifications/commerce-reconciliation-integration.json) · runtimeCases | 15 | not_run |
| [commerce-consumer-repair-design.json](../specifications/commerce-consumer-repair-design.json) · runtimeCases | 22 | not_run |
| [settlement-ops-contracts.json](../specifications/settlement-ops-contracts.json) · runtimeCases | 18 | not_run |
| [return-recovery-contract.json](../specifications/return-recovery-contract.json) · cases | 16 | not_run |
| [return-screen-design.json](../specifications/return-screen-design.json) · cases | 13 | not_run |

위 사례는 중복될 수 있어 하나의 고유 테스트 총수로 합치지 않는다. 이번 DI 수용 기준24개도 기존 runtime 사례 개수에 더해 완료율을 만들지 않는다. 실제 실행 기록에는 artifact/profile/SDK/board/chain/권한 상태, 입력, 기대 결과, 관측 결과, 복구 결과가 필요하다.

## 9. 다음 작업

다음은 **반납 RP-01~08의 전송 방향·HTTP/BLE 경로·전체 envelope·현재 재대여 gate 조회 계약**이다. 먼저 빈 호출 경로와 상태 조회 부족을 해소한다. RR-DEC-01 미답변이어도 기존 증거 복구·권한 분리·조회 설계는 진행하고, 신규 파괴적 reset 승인 정책은 선택하지 않는다.

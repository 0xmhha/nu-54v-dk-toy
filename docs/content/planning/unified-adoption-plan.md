# 통합 계약 채택 checkpoint 계획

2026-09-20 · 후보 계획 · 기준 계약 병합·제품 구현 없음

기존 DI/OC와 profile·관리 신뢰 보완을 하나의 문서 채택 checkpoint 계획으로 연결한다. 변경 파일·공유 검토·호환 조건·중단/복구 기준을 정의하며 이번 작업에서 기준 계약을 병합하거나 실행하지 않는다.

## 계획의 범위

- UA 묶음은 검토 분류이며 배포 단위·새 WBS·담당자·승인 threshold가 아니다. 기존 DI의 완료 의존 관계를 보존하고 공유 파일은 모든 관련 묶음의 검토가 끝나야 통합 기준으로 채택한다.
- 논리적으로 같은 checkpoint에서 문서/권한/reader/schema를 대조한다는 뜻이며 Git commit, 서버 DB, 기기 journal, 체인을 하나의 원자적 commit으로 묶는다는 뜻이 아니다.
- 문서 checkpoint의 일관성, runtime profile 준비, 서버 activation, 개별 peer 적용, 실제 수용 결과를 별도 기록한다. 아래 검토 순서는 서비스 기동/배포 순서가 아니다.
- 후보 HTTP 경로/명령/정책은 기존 등록 목록과 다른 namespace로 유지한다. 기존 operation ID를 임의 재활용하거나 TMC 보호 명령을 공개 REST endpoint로 해석하지 않는다.
- 미선택 profile·wire·물리 저장·인증 채널은 차단 사유로 남긴다. 일반 다음 진행을 선택·구현 지시로 해석하지 않는다. 기존 읽기/제한/원 요청 조정은 해당 현재 권한과 별도 지원 조건을 따른다.
- 새 rollout manifest는 자기 자신의 최초 신뢰 근거가 아니다. 기존 신뢰 경로 또는 사전 독립 bootstrap 근거가 없으면 staging/승인 문서만으로 activation을 허용하지 않는다.

## 검토 묶음

아래 파일 목록은 공동 검토 범위다. 모든 파일을 반드시 수정한다는 뜻이 아니며, 실제 disposition에서 유지/수정/보류를 확인해야 한다.

| 묶음 | 기존 DI | OC 후보 | 변경 의도 | 보존할 의미 |
|---|---|---|---|
| UA-01 · 인증·MPC·보호 결과 | 교차 보완 | OC-01, OC-02, OC-03, OC-16, OC-17, OC-18, OC-19, OC-20 | refresh generation·logout/unlink·MPC 원 checkpoint·operationClass별 현재 ACL/reader | 이전 refresh로 최신 token 반환, 일반 operation 조회로 signed bytes/보호 객체 노출, 늦은 MPC 결과의 무권한 활성화 금지 |
| UA-02 · 상거래·영수증·단말 | DI-01 | OC-04, OC-05, OC-06, OC-07, OC-08, OC-23, OC-24 | order/recipient snapshot·allocation·fulfillment·환불/정산·영수증 귀속·단말 인계 | API018/020/037의 signer/source 의미 보존; 이전 단말 고객 결과 노출·원 지급 한도 초과·정산 덮어쓰기 금지 |
| UA-03 · 대여·반납·기기 연속성 | DI-04, DI-05, DI-06 | 별도 후보/공유 기반 | RP별 실제 전송 경로·기기 journal·owner/relay·server returned/cleanup/재대여 gate | 미확정 reset을 새 job으로 재시작하지 않음; RR 미선택 분기 보류·import 지갑 전체 자산 강제 이체 없음 |
| UA-04 · 시장·자격·유료 자원 | 교차 보완 | OC-09, OC-10, OC-11, OC-12, OC-21, OC-22 | market quote/keeper·DID/STO typed source·x402 payment/entitlement/delivery/refund | 미지원 signer를 EOA 일반 전송으로 대체하지 않음; 부분 성공/가격 만료·원 환불 한도·원 결과 재조회 구분 |
| UA-05 · 녹음·여행·개인정보 | 교차 보완 | OC-13, OC-14, OC-15, OC-25, OC-26 | 오디오 구간/owner·삭제 revision·코스 생성/편집/apply·구매/방문 근거 | 삭제 뒤 늦은 AI 결과/백업 부활 차단; 수동 편집과 유료 생성 구분; 보상 효과 작성자 중복 금지 |
| UA-06 · 설정 적용·관리 신뢰 | 교차 보완 | 별도 후보/공유 기반 | PC/PF/GA·PRC/PCP/PCB·TR/TE/TMC·역할/운영 패널의 현재 권한·증거·reader·저장 매핑 | API088 조회 의미 유지; profile author/점주/FOTA 담당을 activation 권한으로 해석하지 않음; 복구 후 제한 유지와 별도 resume |
| UA-07 · 공유 저장·이벤트·화면 | DI-02, DI-03, DI-07 | 별도 후보/공유 기반 | 원천/소비자 revision·v1/v2 adapter·중복키·outbox·현재 gate·화면 재시도/소유권·삭제/복원 | 공유 파일의 마지막 작성자가 다른 묶음 의미를 덮어쓰지 않음; 같은 자원의 권위/최종 effect writer를 중복 생성하지 않음 |
| UA-08 · 통합 검토·인계 | DI-08 | 별도 후보/공유 기반 | 전후 파일 hash·후보별 disposition·선택/미선택·호환 증거·제외/보류·generated views·회귀 목록 | 부분 문서 검토를 전체 채택이나 실기 성공으로 표시하지 않음; 기존 15요구/104작업/320단계 유지 |

## 변경 대상별 공동 검토

공유 파일은 한 묶음의 검토만으로 완료 처리하지 않는다. 원본 SQL은 참조 대상으로 남기고, 실제 채택 시 새 migration 계획과 검증을 별도로 작성한다.

| 대상 | 파일 | 함께 검토할 묶음 | 취급 |
|---|---|---|---|
| AT-01 | [api-access-transactions.json](../specifications/api-access-transactions.json) | UA-01, UA-02, UA-03, UA-04, UA-05, UA-06, UA-07 | proposed_contract_amendment |
| AT-02 | [api-catalog.json](../specifications/api-catalog.json) | UA-01, UA-02, UA-03, UA-04, UA-05, UA-06, UA-07, UA-08 | proposed_contract_amendment |
| AT-03 | [approval-source-dispatch.json](../specifications/approval-source-dispatch.json) | UA-01, UA-04 | proposed_contract_amendment |
| AT-04 | [authorization-policies.json](../specifications/authorization-policies.json) | UA-01, UA-02, UA-03, UA-04, UA-05, UA-06, UA-07, UA-08 | proposed_contract_amendment |
| AT-05 | [ble-catalog.json](../specifications/ble-catalog.json) | UA-03, UA-06 | proposed_contract_amendment |
| AT-06 | [common-flows.json](../specifications/common-flows.json) | UA-01, UA-05, UA-06, UA-07 | proposed_contract_amendment |
| AT-07 | [compatibility-matrix.json](../specifications/compatibility-matrix.json) | UA-03, UA-04, UA-06, UA-08 | proposed_contract_amendment |
| AT-08 | [core.schema.json](../specifications/core.schema.json) | UA-02, UA-03, UA-06, UA-07 | proposed_contract_amendment |
| AT-09 | [critical-dtos.json](../specifications/critical-dtos.json) | UA-01, UA-02, UA-03, UA-04, UA-05, UA-06, UA-07 | proposed_contract_amendment |
| AT-10 | [critical-dtos.schema.json](../specifications/critical-dtos.schema.json) | UA-01, UA-02, UA-03, UA-04, UA-05, UA-06, UA-07 | proposed_contract_amendment |
| AT-11 | [004_device_lifecycle.sql](../specifications/database/004_device_lifecycle.sql) | UA-03 | reference_only_append_future_migration |
| AT-12 | [README.md](../specifications/database/README.md) | UA-07 | review_generated_view_with_source |
| AT-13 | [schema-catalog.json](../specifications/database/schema-catalog.json) | UA-06, UA-07 | proposed_contract_amendment |
| AT-14 | [event-catalog.json](../specifications/event-catalog.json) | UA-02, UA-04, UA-05, UA-07 | proposed_contract_amendment |
| AT-15 | [extended-dtos.json](../specifications/extended-dtos.json) | UA-01, UA-02, UA-03, UA-04, UA-05, UA-06, UA-07 | proposed_contract_amendment |
| AT-16 | [extended-dtos.schema.json](../specifications/extended-dtos.schema.json) | UA-01, UA-02, UA-03, UA-04, UA-05, UA-06, UA-07 | proposed_contract_amendment |
| AT-17 | [implementation-interfaces.json](../specifications/implementation-interfaces.json) | UA-03, UA-06 | proposed_contract_amendment |
| AT-18 | [screen-flows.json](../specifications/screen-flows.json) | UA-02, UA-05, UA-06, UA-07 | proposed_contract_amendment |
| AT-19 | [screen-flows.md](../specifications/screen-flows.md) | UA-06, UA-07 | review_generated_view_with_source |
| AT-20 | [security-storage-contracts.json](../specifications/security-storage-contracts.json) | UA-01, UA-02, UA-03, UA-04, UA-05, UA-06, UA-07, UA-08 | proposed_contract_amendment |
| AT-21 | [storage-operations.md](../specifications/storage-operations.md) | UA-01, UA-02, UA-03, UA-04, UA-05, UA-06, UA-07 | review_generated_view_with_source |

## 후보 이름·등록 상태

기존 API 110개·정책 60개·BLE 34개·SQL 61개 테이블/7개 migration 기준을 유지한다. 아래는 정식 등록 수에 더하지 않는다.

CR 목록은 OC/PRC/PCP/PCB/TMC의 namespace별 연결이다. 기존 20개 disposition의 SR/RP 등 분류를 대체하거나 중복 합산하지 않는다. TR/TE/PCS 논리 저장 기록과 TO 역할/패널은 UA-06 검토 대상이며 HTTP endpoint 수에 포함하지 않는다.

| namespace | 후보 수 | 현재 취급 |
|---|---:|---|
| OC | 26 | 개별 등록 ID/채택 위치 미확정 |
| PRC | 7 | 개별 등록 ID/채택 위치 미확정 |
| PCP | 7 | 개별 등록 ID/채택 위치 미확정 |
| PCB | 3 | 개별 등록 ID/채택 위치 미확정 |
| TMC | 8 | 보호된 내부 관리 명령; 공개 API 아님 |

개별 후보→UA·기존 API·원 설계의 연결은 구조화 파일의 candidateRegistry에 있다. 원 DI의 20개 분류와 완료 의존 관계도 원형 그대로 보존했다.

## checkpoint 필수 항목

| 항목 | 필요한 내용 | 해석 경계 |
|---|---|---|
| CP-01 · identity | checkpointId·scope·parentBaselineDigest·candidateBundleDigest·createdAt | 작업 계획 ID와 runtime changeId/activationId를 분리 |
| CP-02 · sourceAndTargets | source file/hash·target before/after hash·UA별 diff·generated view/hash | 실제 after hash는 채택 검토 시 생성. 지금은 baseline hash만 존재 |
| CP-03 · decisions | 정확한 선택 record/profile/version·사용하는 PF/GA branch·미선택/hold 목록 | 미선택을 빈 값/기본값으로 정상화하지 않음 |
| CP-04 · dispositions | DI/OC/PRC/PCP/PCB/TMC별 유지/변경/신규/보류·실제 채택 ID/위치 | 개별 채택이 미해결이면 전체 채택 아님. namespace별 수량 분리 |
| CP-05 · compatibility | producer/consumer/profile 조합·reader/continuation·오류/거절·migration/cleanup 계획 | 부재를 호환으로 간주하지 않음. 단순 schema 유사성은 의미 호환 증거 아님 |
| CP-06 · reviewEvidence | 검사 종류·명령/버전·입력 hash·결과·한계·현재 승인 scope/evidence 참조 | 문서 검사·실제 암호·DB·기기·종단 검증을 다른 종류로 기록 |
| CP-07 · statusAndRecovery | 검토/채택 상태·보류 사유·원 결과 조회·후속 수정/제한 계획 | 채택됨과 runtime 준비/활성화/peer 적용/수용 완료는 별도 축 |
| CP-08 · retention | 구 reader·원 요청·노출/체인 서명·미확정 작업·삭제/seal 의존성 | retention 종료나 구버전 제거는 이 문서 채택의 자동 부수효과가 아님 |

현재 CP 값과 selectedCheckpoint는 null이다. 계획 문서의 hash와 실제 채택 후 target hash를 혼동하지 않는다.

## 호환·기존 작업 보존

| 조합 | 조건 |
|---|---|
| UC-01 · 구 client → 새 server | 명시된 지원 version/operationClass만 adapter로 해석. 누락된 새 필드가 승인/권한 의미를 바꾸면 새 효과 거절. 기존 원 결과 읽기는 별도 현재 reader로 검토 |
| UC-02 · 새 client → 구 server | capability/버전 미지원은 지원 대기. 통신 오류를 이유로 보호 약한 구 endpoint/다른 signer로 자동 fallback하지 않음 |
| UC-03 · 일부 peer만 새 profile 적용 | server activation과 필수 cohort별 적용 분리. 해당 새 효과만 hold; 현재 허용된 관측/제한/원 요청 조정 경로 보존 |
| UC-04 · 기존 operation과 갱신된 권한 | 원 payload/source/parent·epoch/노출을 보존. 이전 승인으로 새 effect 생성하지 않음. 현재 read proof가 grant 재활성화로 확장되지 않음 |
| UC-05 · 이벤트 v1/v2·중복/역순 | 명시 dispatcher와 원천 revision을 유지하고 소비자별 단일 effect writer 보존. event 이름만 같다고 payload 호환으로 판단하지 않음 |
| UC-06 · 저장 확장·새 writer·구 reader | expand/호환 reader→writer 전환→관측→별도 정리. 실제 layout 미선정이면 물리 호환 미검증. 구 7 migration 수정/재적용 금지 |
| UC-07 · 관리 root/키 교체·복구 | 현재 독립 신뢰와 purpose·scope·PoP·checkpoint·fence로 재검사. 구 공개키의 역사적 서명 검증은 현재 권한이 아님 |
| UC-08 · UI/권한/결과 경로 | 화면 버튼과 서버 권한 재검사 연결. pending/unknown/committed/current restriction을 분리. API020/107 typed reader와 API088 audit-only 의미 보존 |
| UC-09 · 되돌림 요구 | 문서 후보 폐기와 runtime rollback 구분. 활성화 후에는 현재 제약을 만족하는 더 높은 revision의 후속 변경; fence 감소·파괴적 down migration·외부 서명 취소 추정 금지 |

## 반영 순서

```mermaid
flowchart LR
 A["기준·공유 대상 보존"] --> B["관련 선택·호환 입력 확인"]
 B --> C["공동 계약 diff 검토"]
 C --> D["후보별 disposition·검사"]
 D --> E["문서 checkpoint 채택"]
 E -. "명시적 구현 전환 후" .-> F["실환경 준비·활성화·peer 적용"]
```

| 단계 | 산출물 | 중단 조건·한계 |
|---|---|---|
| UP-01 · 변경 대상과 기준 보존 | AT 목록·source pins·후보 namespace·기존 DI 의존성 | 기준 drift는 해당 범위를 재검토하며 과거 hash 덮어쓰기 금지 |
| UP-02 · 행위별 정책·호환 입력 확인 | 선정/미선정 PF/GA·정확한 변경 scope·UC 조건 | 미선정 분기의 신규 효과는 hold. 해당 분기의 wire/수명/암호 조건을 임의 확정하지 않음 |
| UP-03 · 계약·권한·reader·저장·화면 통합 diff | UA01~08 공동 검토·공유 AT별 reviewer 증거·CP 후보 | 이 단계부터 실제 계약 수정은 선택/입력이 충분한 범위의 별도 설계 채택 작업. 이번 계획에서는 수행 안 함 |
| UP-04 · 후보별 disposition과 문서 검사 | source/target hash·schema/ACL/reader·negative case·generated view·DI 완료 의존성 대조 | 호환 미입증/일부 실패를 남긴 checkpoint는 partial/held; 완전 채택으로 올리지 않음 |
| UP-05 · 문서 기준 채택 기록 | 실제 변경과 검토 증거가 고정된 checkpoint·인계/원 기준 보존 | 문서 채택은 runtime activation·실기검증·구현 허가가 아님 |
| UP-06 · 구현 이후 rollout 인계 | 명시적 구현 전환 후 환경·실증거→prepare→activation→peer apply→실제 수용 | 실제 적용은 PRC/TMC와 AD01~07 원 계약; 이 계획으로 원격 변경/SQL/키등록을 수행하지 않음 |

UP는 검토 완료 순서다. 기존 DI finishDependsOn을 삭제하거나 배포 순서를 재정의하지 않는다. 일부 기능만 채택할 때에는 그 기능에 필요한 공유 계약·권한·reader·저장 의존성을 닫고 제외된 기능을 명시해야 한다. 전체 15개 요구의 수용 목표는 유지한다.

## 중단·실패·복구

| 시점 | 조치 |
|---|---|
| UR-01 · 문서 검토 전/중 source drift | 해당 기준/diff 검토 보류·새 비교본 작성. 현재 canonical과 역사적 checkpoint를 임의 재기준화하지 않음 |
| UR-02 · 문서 후보 일부 실패 | 미완성 후보를 held/partial로 기록·원 문서 유지. 선택된 범위에서 필요한 모든 관련 target이 정합해야 그 범위 채택 가능 |
| UR-03 · runtime staging 이후 abort | 원 change/result와 commit 경합 확인. 미활성화가 확인된 staging만 중지; 응답 유실은 미실행 증거가 아님 |
| UR-04 · activation 이후 일부 적용/불명 | 원 activation 조회·현재 fence 유지·peer별 재검증. 전체 성공이나 head 감소를 하지 않음 |
| UR-05 · 새 writer/외부 서명 이후 실패 | 노출/체인/저장 상태 조정과 호환 후속 변경. 파일 되돌림으로 이미 일어난 외부 효과를 없앴다고 하지 않음 |
| UR-06 · 과거 reader·tombstone 폐기 요구 | 원 요청/노출/보존/복원·generation seal 의존성이 해소됐다는 현재 증거 필요. 미해결이면 읽기 adapter 또는 격리 유지 |

## 검증과 다음 단계

기존 DI 8개·OC 26개의 누락/중복, PRC/PCP/PCB/TMC 후보 연결, 공유 파일·원본 hash·검토 순서와 미채택 상태를 검사했다. 문서 구조/참조 검사이며 실제 호환성·동시성·DB migration·BLE·신뢰 복구의 통과 증거가 아니다.

[구조화 계획](unified-adoption-plan.json) · [검증 기록](unified-adoption-validation.json) · [제품별 인계 보완판](design-closure-review.md) · [기존 DI 채택표](integration-adoption-matrix.md)

후속 설계에서는 공유 AT의 충돌 가능 지점을 실제 필드/연산 단위로 대조한다. 새 정책값을 정하지 않고 변경 책임·보존 의미·검토 차단 사유를 구체화하며, 선택에 의존하는 항목은 미확정으로 유지한다.

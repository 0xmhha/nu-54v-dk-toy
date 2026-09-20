# 승인 계약 기준 통합 결과

2026-09-18 · **설계 기준에 반영 완료. 제품 구현·배포·기기·암호·DB migration 시험은 미수행.**

개인 EOA 송금, 계정 없는 NU 키오스크 결제, 가맹점 EOA 환불의 HTTP·BLE·권한·화면·논리 저장 계약을 현재 기준 명세에 반영했다. 새 권한 경로는 **AC-01→API-108, AC-02→API-109, AC-03→API-110**으로 등록했다.

[현재 기준 원본](approval-baseline.json) · [독립형 승인 Schema](approval-baseline.schema.json) · [현재 기준 예제](approval-baseline-examples.json) · [현재 검증기](validate_approval_baseline.py) · [저장 관계·제약 설계](approval-storage-mapping.json)

## 1. 반영 결과

| 항목 | 이전 | 현재 | 해석 |
|---|---:|---:|---|
| API | 107 | 110 | 기존 승인 API 10개 교체와 자격 경로 3개 추가 |
| 핵심/확장 DTO | 8 / 99 | 8 / 102 | 모든 API에 전체 논리 요청·응답 등록 |
| 접근 매핑/권한 정책 | 107 / 57 | 110 / 60 | 현재 actor/source/epoch 검사와 별도 조회 action |
| 논리 보안 저장 자원 | 15 | 28 | AI-R 5개와 SS-R 8개 반영. 신규 SQL 테이블 수가 아님 |
| BLE 명령/화면 | 34 / 37 | 34 / 37 | 승인 명령 5개와 기존 화면의 동작 연결 |
| 기존 물리 참조 DDL | 61개 테이블 | 동일 | 추가 승인 관계·제약은 별도 mapping 설계이며 SQL 미반영 |

`approval-baseline.schema.json`은 현재 승인 계약의 독립형 schema다. 기준 DTO/core에는 동일 정의를 포함해 로컬 참조만으로 검증할 수 있다. 과거 후보 schema나 변경된 현재 extended schema를 순환 참조하지 않는다. HTTP 13개 경로와 BLE 5개 명령의 요청·응답을 포함한다.

HTTP의 `approval-v1-draft`, BLE의 `approval-ble-v1-draft`는 **설계 버전 이름**이다. 공개 릴리스·실기 호환 선언이 아니다. crypto/profile·SDK·board target·MPC 구성·근접 정책은 선정과 실제 검증이 필요하다.

## 2. 현재 계약의 필수 경계

- API-017은 개인 송금 source, API-034는 원 guest attempt/session, API-037은 매장 업무 승인과 실제 signer를 결합한다. API-018의 adapter는 저장된 source/요구 버전으로 선택한다.
- API-108~110은 제한된 접근 자격의 challenge/발급·교체/철회다. 실제 기기 물리 승인이나 Cloud 서명권을 부여하지 않는다. 현재 계정 또는 인증된 terminal을 독립 확인한다.
- API-015/018은 approvalOperationId를 반환한다. API-020은 서버가 ancestry로 parent/child/general을 분류한다. 승인 부모·자식의 resultRef는 null이며 signed result는 부모에서 별도 현재 권한으로만 조회한다.
- API-019/020의 GET은 업무 원장을 바꾸지 않는다. sender proof 사용 시 replay 소비 같은 보안 metadata 쓰기는 `authorizationMetadataWrites`에 별도로 기록한다. 권한 검사를 생략하는 의미의 read-only가 아니다.
- BLE 승인 5개 명령은 새 상세 envelope만 사용한다. 느슨한 `draft-1` envelope로 그 명령을 호출하는 것을 차단했다. 기존 설정·녹음·FOTA 등 다른 명령의 역할 경계는 유지한다.
- 점주 승인자 A와 signer B는 별도로 확인한다. B는 앱 U03에서 원 환불 snapshot을 검토하며 개인 송금으로 환불을 재작성하지 않는다. guest에 모바일 계정을 강제하지 않는다.
- 자격 철회/세대 변경/네트워크 오류로 이미 존재할 수 있는 EOA 서명을 취소했다고 표시하지 않는다. 원 관측과 환불 예약 대사는 유지한다.

현재 이들 조건은 **계약과 검증 사례의 요구사항**이다. 실제 권한 엔진·proof verifier·CAS·네트워크 동작을 구현하거나 시험한 결과는 아니다.

[원천별 dispatch 등록부](approval-source-dispatch.json)는 이번 API-018 DTO가 허용하는 세 source와 아직 별도 adapter 설계가 필요한 smart_account/market_action/credential_proof/paid_resource를 구분한다. U13~U19의 계획 범위는 유지하지만 미등록 adapter는 실행 미지원으로 표시한다. 이들을 느슨한 legacy union이나 개인 송금으로 통과시키지 않는다. 기존 기록의 지원되는 원 reader와 현재 권한이 있으면 조회·관측은 유지한다. 전체 상품의 API-018 연동까지 완료했다고 주장하지 않는다.

API-017/034/037은 intent/snapshot·부모 operations row·ApprovalBinding ancestry·멱등 연결을 같은 commit으로 기록한다. API-015/018의 자식 작업은 원 부모/context와의 연결을 별도로 보존한다. 부모 operation을 응답에만 만들고 저장하지 않는 경로는 허용하지 않는다.

## 3. 검토 이력 보존과 검증 책임

[병합 전 체크포인트](../../design-history/approval-premerge-20260918/checkpoint.json)에 파일 174개의 원본 bytes와 SHA-256, Python/library 환경을 기록했다. repository-relative 경로를 유지했고 그 안에서 기존 검증 10개가 모두 통과했다. [원 실행 보고서](../../design-history/approval-premerge-20260918/validation-report.json)는 제품 실행 결과가 아닌 과거 문서 재현 결과다.

개인/source/common/integration/commerce/return protocol/adoption의 기존 검증 진입점은 보존된 원 validator를 실행한다. 화면 반납 검증처럼 현재 기준에서도 여전히 유효한 검사는 현재 화면을 검사한다. 현재 승인 기준은 `validate_approval_baseline.py`, 전체 현재 명세는 `validate_specs.py`가 담당한다. 후보 JSON의 예전 hash를 최신 값으로 덮어쓰지 않았다.

기존 후보 문서/JSON은 설계가 발전한 과정을 보여주는 이력이다. 그 안의 `canonicalMerged=false`나 API 107개 표기는 작성 당시 상태이며 현재 상태를 판정하는 파일은 이 문서와 `approval-baseline.json`이다. 과거 검증 통과를 현재 통합 통과나 실행 호환 증거로 재사용하지 않는다.

## 4. 저장 기준과 SQL 검증의 한계

논리 자원·키·관계·고유 조건·원자 처리·서비스 검증 책임을 [저장 mapping](approval-storage-mapping.json)에 고정했다. 기존 tables와 새 companion/adapter 관계를 구분하고, source gate/authorization gate가 같은 현재 권한 revision을 보게 한다. 다중 DB에 분산하면서 단일 transaction 보장을 가정하지 않는다.

기존 7개 SQL 파일과 과거 19개 제약 시험 결과는 변경하지 않았다. 그 결과는 **기존 61개 테이블만** 검증한 것이다. 승인 ancestry, provenance, 단일 successor, protected result release, permit/gate의 새 물리 제약은 아직 SQL로 반영하거나 실행 검증하지 않았다. 이 제한을 기준 adoption 기록의 잔여 설계 항목으로 명시한다.

## 5. 미완료 항목

12주 15개 요구·104개 작업·320개 세부 작업은 유지한다. 이번 병합은 승인 계약군의 기준 정리이며 전체 제품 설계 완료·최종 Seed 승인·구현 착수가 아니다.

[물리 저장 상세 설계](approval-physical-storage-design.md)에 테이블 후보 16개·이행 8단계·장애 복구 사례 17개를 정리했다. SQL 미반영이며 실행 검증은 하지 않았다. [결제·환불 대사 연결안](commerce-reconciliation-integration.md)에 현재 계약과 호환되는 전체 HTTP 5개 경로·이벤트 2개·저장 4개·원자 처리 4개를 정리했다. 아직 canonical 병합 전이며 정책 선택과 실제 동작은 미검증이다. 다음은 소비자별 projection 보정·재처리 계약이다. crypto/profile/실기 호환은 기존 수용표의 24개 시나리오와 COMP 26개 모두 실행 미검증으로 남긴다. RR-DEC-01과 MPC/지갑 정책도 사용자 선택을 대신하지 않는다.

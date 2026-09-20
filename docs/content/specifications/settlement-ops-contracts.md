# 매출·정산 API와 운영 재처리 명령 계약

2026-09-18 · **설계 제안. 현재 기준 API 110개와 SQL은 변경하지 않았으며 제품 구현·배포·실행 시험은 미수행이다.**

기존 경로 5개(API-039/040/041/093/087)의 전체 논리 요청·응답과, 미등록 후보 경로 2개(SR-01 조회 / SR-02 반영)를 작성했다. [소비자 보정 설계](commerce-consumer-repair-design.md)의 원천 manifest·기여분·generation/fence·불변 보정을 화면과 명령 계약으로 연결한다.

[계약 원본](settlement-ops-contracts.json) · [독립형 Schema](settlement-ops-contracts.schema.json) · [합성 예제](settlement-ops-contracts.examples.json) · [검사기](validate_settlement_ops.py)

## 1. API별 역할

| 경로 | 목적 | 권한·핵심 조건 |
|---|---|---|
| GET API-039 매출 | 자산/기간의 현재 수치, 최신성, 마감 또는 보정 후보 | exact-store store_sales_read; manifest/금액을 클라이언트가 만들지 않음 |
| POST API-040 마감 | 검토한 source cut으로 마감 snapshot 생성 | store_settlement_manage, 종료된 기간, complete/current/ready, scope revision |
| POST API-041 보정 | 원마감을 유지하고 새 snapshot version 추가 | store_settlement_manage, 원settlement 소속, 최신 revision과 target digest |
| GET API-093 정산 이력 | 마감/보정 version과 현재 원천 차이 조회 | store_sales_read; cursor와 이력 필터 소속 확인 |
| POST API-087 재조회·미리보기 | 검증된 원천 재조회 또는 shadow 계산 요청 | 기존 ops_reconcile의 범위. 직접 accepted/원장값 입력 금지 |
| GET SR-01 작업 상세 후보 | 검토안·보류·반영 결과 조회 | 제안 ops_repair_read, 현재 운영 범위 |
| POST SR-02 반영 후보 | 검토한 단일 소비자 목표의 조건부 반영 요청 | 별도 제안 ops_repair_publish; 검토 digest·fence·현재 권한 재확인 |

SR-01은 `/v1/ops/reconciliation-jobs/{repairId}`, SR-02는 같은 경로의 `/publications`다. 정식 API 번호를 배정하거나 카탈로그에 등록하지 않았다. 기존 API-087의 권한으로 SR-02를 호출할 수 있다고 해석하지 않는다.

논리 header `contractVersion=settlement-repair-v1-draft`, 인증, requestId, 쓰기 명령의 idempotencyKey를 명시했다. 실제 header 이름·배포 협상은 D02 미선정이다. 새 응답에는 no-store를 요구한다. 승인 API-020의 기존 schema·버전은 그대로다. 서로 다른 계약 버전을 하나로 대체하지 않는다.

## 2. 매출 조회와 마감 후보

API-039는 period와 assetId를 받는다. 같은 scope의 검증된 저장 projection/manifest를 조회하며, GET이 승인·반영 작업을 생성하지 않는다. 후보 자료가 없으면 candidate=null 또는 held/unavailable로 응답한다.

`freshness`는 state(current/stale/held/unavailable), asOf, projectionRevision, generation, manifest 참조, reasonCodes다. current는 검증한 원천 cut의 최신성을 뜻한다. candidate.ready는 완전성·정책·미해결 보류·기간 종료를 추가 통과했다는 뜻이다. 둘 다 mutation의 현재 권한 검사를 대체하지 않는다.

금액 필드는 기존 UInt 수취액을 재정의하지 않고 별도로 제안한다.

| 필드 | 의미 |
|---|---|
| grossAcceptedAtomic | 유효한 주수락 매출 기여분 |
| confirmedSalesRefundAtomic | 그 매출 원천에 귀속된 확정 환불 |
| canonicalAttributedReceiptsAtomic | 예외 지급도 구분해 포함하는 유효 귀속 수취 |
| allCanonicalRefundOutflowAtomic | 귀속된 모든 유효 환불 지출 |
| reservedRefundAtomic | 아직 해소되지 않은 환불 예약 |
| netSalesAtomic | grossAcceptedAtomic − confirmedSalesRefundAtomic |
| netAttributedCashAtomic | canonicalAttributedReceiptsAtomic − allCanonicalRefundOutflowAtomic |

마지막 두 값은 signed atomic 정수 문자열이다. 원입금 0, 유효 환불 6이면 -6을 표시하고 검토 필요 상태를 유지한다. 음수를 0으로 숨기지 않는다. 아직 알 수 없는 금액은 metrics=null과 보류/미가용 이유를 사용한다. 알려진 과거 값은 stale/asOf를 함께 표시하며 최신값으로 꾸미지 않는다. 지갑 전체 잔액·원화 환산·법정 회계 수치를 뜻하지 않는다.

settlementId 없이 조회하면 미마감 scope의 close 후보를, settlementId가 있으면 원기간의 correction 후보를 반환하는 제안이다. 후자는 원settlement의 store/asset/기간/policy에서 범위를 도출하고 path/query 불일치를 거절한다. 후보에는 manifest, projectionRevision, generation, scopeRevision, targetDigest, 금액과 차단 사유를 담는다. correction에는 settlementId/revision도 필수다.

## 3. 마감·보정 명령

API-040은 scope·manifest·expectedScopeRevision·expectedProjectionRevision·expectedGeneration·targetDigest를 받는다. API-041은 원settlement를 경로로 지정하고 manifest·expectedSettlementRevision·expectedProjectionRevision·expectedGeneration·targetDigest·reason을 받는다. 금액이나 새 수취 주소를 쓰기 body로 받지 않는다.

서버는 저장 후보와 현재 source를 확인한다. manifest ID/hash 자체는 권한·완전성 증명이 아니다. 원천 provider, authorityEpoch, 목록 완전성, policy, 현재 gate/삭제/소유권 상태를 앞선 SourceManifest 계약대로 검사한다.

처리 순서:

1. 현재 정확한 매장의 마감/보정 권한과 멱등 scope를 확인한다.
2. 같은 key의 완료 결과라면 저장 request digest를 비교해 원결과를 반환한다. 다른 body이면 충돌이다. 원결과 복구 때문에 두 번째 마감/보정을 만들지 않는다.
3. 새로운 요청은 shared authority gate→정산 scope/원settlement→소비자 checkpoint/manifest 순서로 잠그고 최신 revision/generation/target를 재검사한다.
4. 새 snapshot/version, scope/list revision, 멱등 결과, 감사/outbox를 같은 commit에 반영한다. 중간 source 변경은 conflict이며 금액을 조용히 다시 계산해 자동 승인하지 않는다.

처음 마감의 scopeRevision은 아직 존재하지 않는 settlement row의 revision이 아니다. 정규화된 사업기간·매장·환경·chain·asset 범위의 gate/상태에서 관리하는 **추가 논리 adapter 제안**이다. 물리 구조와 참조 DDL은 미작성이다. 같은 기간을 timezone 표시나 policyVersion만 바꿔 두 번 마감할 수 없도록 정규화하며, 겹치는 기간은 선정된 partition 정책과 맞지 않으면 거절한다. 그 정책이 미정이면 정상 마감으로 진행하지 않는다.

원마감 totals와 version은 불변이다. 같은 target에 새로운 idempotencyKey를 붙여 보정하더라도 NO_CHANGE로 끝내고 새 version을 만들지 않는다. reorg로 새 차이가 생기면 correction_pending을 표시하며, 새 보정 승인 없이 기존 마감값을 고치지 않는다. 이미 종료된 period에 대응하는 authoritative cut을 고정하며 이후 도착 데이터는 별도 보정으로 취급한다.

API-093은 `(settlementId, snapshotVersion)`별 이력 행을 페이지로 제공한다. settlementId 필터로 해당 이력을 조회할 수 있다. recordRevision은 그 이력의 값이고 현재 보정 명령에는 API-039에서 최신 후보를 다시 받아야 한다. currentSourceStatus(current/correction_pending/held/unknown)는 과거 snapshot 자체의 상태와 구분한다. cursor는 principal/store/모든 filter/listRevision에 결합하며 변경 시 재조회한다. limit 1..100은 제안 범위, 기본값은 D19 미선정이다.

## 4. 재조회·미리보기·반영의 구분

API-087 mode는 source_refresh 또는 preview다. source_refresh는 기존 검증 대사기로 원천을 다시 읽게 하므로, 확인된 실제 관측의 정상 대사와 downstream 이벤트는 가능하다. 운영자가 상태를 강제하거나 새 projection generation을 승인 없이 반영하는 명령은 아니다. preview는 shadow 목표·차이를 계산하며 실제 기여분·정산 승인·혜택 효과를 바꾸지 않는다.

**한 repair/publication은 단일 consumer와 단일 projectionKey에만 결합한다.** API-087의 consumer는 선택자이며 서버가 subjectRef/scope에서 정확한 projectionKey를 도출한다. RepairView와 Review에 consumer/projectionKey를 보존하고, 그 checkpoint의 generation/fence를 검토한다. 여러 소비자가 필요하면 별도 repairId로 각각 처리하고 독립 결과를 표시한다. 분산 저장 간 일괄 성공이나 하나의 fence를 가정하지 않는다.

작업 상태는 requested→capturing→building→diff_ready→publishing→completed다. 실패/불확실/정책 미정은 held/quarantined/failed로 남긴다. API-020의 일반 operation 상태와 같지 않다. 예를 들어 preview operation.succeeded는 검토안 계산 완료이지 publication 완료가 아니다.

SR-01은 현재 운영 권한과 저장된 repair 소속을 매번 확인한다. 보류 이유, 안전한 review 요약, publication 진행을 제공하지만 raw proof·서명 bytes·토큰·개인 위치·전체 원천 vector를 공개하지 않는다. API-020에는 기존 general 진행 형태만 연결하는 제안이며 approval=null, approvalOperationId=null, resultRef=null을 유지한다. 운영자 source ACL 매핑은 후속 권한 통합 대상이다. operation ID를 안다는 이유로 일반 조회권을 주지 않는다.

## 5. 반영 명령과 실제 commit

SR-02는 mode=preview, state=diff_ready이며 publishEligible=true인 검토안에만 요청할 수 있다. request는 expectedRunRevision, manifest, expectedGeneration, expectedFence, diffDigest, effectPlanDigest, reason을 포함한다. 서버에 저장된 scope/consumer/projectionKey/effect 범위를 바꿀 수 없다.

제안 `ops_repair_publish` 권한은 exact scope·consumer·effectClass에 결합한 현재 TrustPrincipal/TrustGrantRevision을 확인한다. 기존 ops_reconcile만 가진 운영자는 미리보기만 가능하다. 권한 승격이 필요하면 기존 TrustChangeRequest의 요청자/승인자 분리 원칙을 따른다. SR-02가 자체적으로 권한을 승격하거나 사용자 정책을 선택하지 않는다. 이 신규 권한은 아직 provisioning하거나 기준 정책에 병합하지 않았다.

202 응답은 publication intent를 CAS로 등록하고 outbox에 넣었다는 뜻이다. worker가 실행하기 전이나 대기 중에도 권한이 철회될 수 있다. 최종 CP-B06에서 현재 권한·source/epoch·privacy·fence를 다시 확인하고 다음을 같은 commit으로 바꾼다.

- active generation pointer와 새 fence
- source contribution과 correction record
- application marker와 checkpoint
- audit/outbox 및 publication 결과 연결

commit 실패면 공개 전환도 하지 않는다. 저장 영역이 분리되어 이 원자 경계를 제공하지 못하면 별도 protocol이 설계되기 전 반영을 차단한다. commit 뒤 응답/worker가 사라진 경우 동일 intent/effect key로 결과를 복구하며 보정을 반복하지 않는다. 오래된 worker의 DB 쓰기는 fence로 막지만 이미 허가된 외부효과를 취소했다고 주장하지 않는다.

허용 effectClass 제안은 view_replace, append_source_correction, mark_settlement_correction_pending, withhold_publication이다. 원규칙에 근거한 보정만 허용한다. 자동 신규 grant/consume/보상 지급·환불·서명·nonce 할당·정산 마감/보정 승인·삭제 취소를 수행하지 않는다. 정책이 없는 혜택 회수/면제는 held다.

동일 run의 publication intent는 하나만 활성화한다. 같은 key/digest의 재시도는 같은 결과를 반환하고, 다른 요청의 경쟁은 충돌 처리한다. source 변경으로 held가 되면 새 preview를 요청해 다른 repairId와 새로운 검토 digest를 받는 제안이다. 기존 요청에 새 target를 몰래 끼워 넣지 않는다.

## 6. 오류와 화면 동작

| 코드 | 의미 | K06/O03 동작 |
|---|---|---|
| AUTH_REQUIRED / SCOPE_DENIED / RESOURCE_UNAVAILABLE | 인증·현재 범위 불일치 | 권한 회복/운영 문의. 자원 존재 정보 최소화 |
| MANIFEST_STALE / REVISION_CONFLICT / DIGEST_CONFLICT | 검토 후 변경 | 원조회·preview를 다시 수행, 새 수치 자동승인 금지 |
| SOURCE_HELD / SOURCE_INCOMPLETE / POLICY_UNAVAILABLE | 원천 보류·미완전·규칙 미정 | 읽을 수 있는 사유 표시, 마감/반영 비활성 |
| IDEMPOTENCY_CONFLICT | 같은 key의 다른 요청 | key 재사용 오류 표시, 임의 body 변경 재시도 금지 |
| ALREADY_CLOSED / NO_CHANGE | 이미 마감 또는 같은 보정 목표 | 원마감/이력 조회, 중복 version 생성 금지 |
| CONTRACT_UNSUPPORTED / INVALID_REQUEST | 계약/입력 미지원 | 명시 오류, 구버전으로 묵시적 하향 금지 |
| RATE_LIMITED / SERVICE_UNAVAILABLE | 일시 운영 장애 | 안전한 상태 조회, 같은 key로 결과 확인 |

schema는 오류 envelope와 허용 code를 검사한다. 정확한 HTTP/code 매핑·은닉·retry 정책은 서비스 adapter 검사 책임이다. 충돌/보류는 일반적으로 409, 인증 401, 권한 403/404, 미지원/입력 422, 정책 가용성 503으로 제안하며 실제 적용은 미수행이다.

K06은 기간·자산·asOf·최신성·음수 차이·마감 가능 이유를 함께 표시한다. 금액 카드만 보고 마감 성공으로 처리하지 않는다. 이력에서 원마감과 보정 version을 구분한다. O03은 재조회/미리보기와 반영 버튼을 권한별로 분리하고, 검토안의 단일 consumer/projection·diff·효과 범위를 보여준다. 202 뒤에는 ‘반영 요청됨’이며 CP-B06 완료 뒤에만 반영 완료로 표시한다.

## 7. 실행 수용 사례

아래는 모두 not_run이며 실제 권한·DB 경쟁·메시지 전달을 시험한 결과가 아니다.

| ID | 상황 | 기대 결과 |
|---|---|---|
| SO-T01 | 다른store manifest | 거절 |
| SO-T02 | 원입금0/환불6 | -6표시 및보류;0clamp금지 |
| SO-T03 | 환불목록누락 | metrics미확정/null·candidate미승인 |
| SO-T04 | 조회후새환불 | 409 MANIFEST_STALE;원preview다시보기 |
| SO-T05 | 아직종료안된기간 | 마감보류 |
| SO-T06 | 동일scope동시마감 | 1개만commit;멱등/ALREADY_CLOSED |
| SO-T07 | 마감commit후응답유실 | 현재권한후원결과;중복snapshot없음 |
| SO-T08 | 같은target로새key | NO_CHANGE;새version없음 |
| SO-T09 | 시간대표시로중복기간우회 | 정규화범위/정책거절 |
| SO-T10 | ops_reconcile로SR02호출 | 403;승격자동허용없음 |
| SO-T11 | 검토후effectPlan변경 | 409 DIGEST_CONFLICT |
| SO-T12 | 202후worker실행전운영권한철회 | held/거절;발행없음 |
| SO-T13 | 동일run동시publish | CAS 단일intent;다른request충돌 |
| SO-T14 | CP-B06완료후worker손실 | 원publication조회·재적립/보정없음 |
| SO-T15 | preview후삭제 | 최신deny차단;재공개없음 |
| SO-T16 | client가accepted=true전달 | schema거절 |
| SO-T17 | 구버전서식 | 명시미지원;느슨한fallback없음 |
| SO-T18 | 구fenceworker잔존 | DBcommit거절;이미허가외부효과추적 |

## 8. 검증과 채택 상태

합성 정상 envelope 15개·거절 12개와 응답 metric 산술을 로컬 검사한다. 입력 명세 9개의 hash를 확인해 기존 승인/대사/소비자 기준 변경 여부를 검사한다. 새 alias 2개는 정식 API 수에 더하지 않는다. 모든 실행 수용 사례 18개는 미검증이다.

D02 버전·저장/일관성 protocol, D19 목록 기본값·운영 임계값, 정산 기간/보정 정책, 제안 운영 권한 scope, 혜택/여행 정책, RR-DEC-01은 미정 상태를 유지한다. 이 문서는 요구사항을 구현한 결과나 최종 Seed 승인이 아니다.

후속 [미병합 설계 통합표](../planning/integration-adoption-matrix.md)에 API·권한·화면·이벤트·저장 변경 묶음과 수용 기준을 정리했다. 다음은 반납 RP01..08의 전송방향·전체 호출 경로·현재 재대여 gate 조회 계약이다.

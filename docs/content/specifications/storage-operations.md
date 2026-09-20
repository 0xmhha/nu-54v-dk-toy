# 저장 작업·동시성·삭제/복구 명세

후속 [결제·환불·반납 설계](commerce-lifecycle-design.md)에서 TX-04/05/07의 경쟁·보정·초기화 후 복구 규칙과 미반영 저장 계약을 구분한다. 참조 DDL이 해당 추가 규칙까지 검증한 것은 아니다.

[논리 데이터 모델](data-model.md)을 실제 변경 단위로 나눴다. DB 엔진을 정하기 전의 transaction/조건부 갱신 명세이며 실행 SQL이나 제품 구현이 아니다. 별도 서비스·DB를 12개 만들라는 의미도 아니다.

후속 [PostgreSQL 참조 DDL](database/README.md)에서 테이블·관계·금액/고유 제약을 작성했다. 아래의 다중 행 처리·proof 검증은 서비스 transaction 구현 책임으로 계속 추적한다.

보안 API-103~107은 [추가 저장 계약](security-api-integration.md)에 통합했다. `SECURITY_STORE`의 flow/challenge 원자 소비, `RECEIPT_CLAIM`의 소유 연결+outbox, pending 혜택의 단일 귀속, 실제 객체 열람/삭제 경계를 따르며 외부 adapter와 기존 DB를 하나의 transaction으로 가정하지 않는다.

## 1. 공통 저장 규칙

- 권한·업무 precondition을 검사하고, 변경 대상의 revision을 확인한다. 변경과 필요한 outbox 이벤트를 동일 DB transaction에 기록한다.
- idempotency record는 actor scope·행위·key로 고유하다. 같은 key의 다른 입력은 충돌이다. 비밀 원문 대신 안전한 정규화 digest와 결과 참조를 저장한다.
- 조회 projection은 원장을 덮어쓰지 않는다. event ID 중복 제거와 aggregate revision을 검사하고 누락은 원천에서 복구한다.
- 체인 제출·외부 OAuth/MPC/AI 호출은 DB transaction과 함께 원자적으로 끝난다고 가정하지 않는다. intent/job을 먼저 저장하고 외부 결과를 별도로 대조한다.
- 일반 계정/업무 저장소에는 고객 HW private key·니모닉·MPC 전체키를 저장하지 않는다. 민감 proof/세션 token은 지정된 보호 경로와 수명주기를 적용한다.

## 2. 변경 단위

### TX-01 · 소셜 계정 생성/연결 — AUTH-01~04

입력: 실제 검증된 provider/subject와 auth flow, 현재 계정의 연결 승인.

1. 소비되지 않은 flow와 요청 목적을 확인한다.
2. `(provider, subject)` 고유 제약 아래 계정 연결을 생성하거나 기존 연결을 읽는다.
3. 기존 다른 계정에 묶였으면 자동 병합하지 않고 충돌을 반환한다.
4. session/identity 변경과 감사 참조를 저장한다. 제공자 코드/refresh token을 일반 감사 payload로 남기지 않는다.

동시 콜백은 계정/지갑을 두 개 만들지 않아야 한다. 계정 생성 성공 후 MPC 생성이 실패하면 계정은 보존하고 wallet operation만 이어서 처리한다.

### TX-02 · 기기 등록·지갑 binding — HW-03~04, STAMP-03

입력: 현재 enrollment, 기기 증명, 현 대여자, 기기의 주소 통제 증명.

활성 device binding 고유 제약과 rental 상태를 검사한다. enrollment challenge 소비, binding 생성, 주소/지갑 연결, 권한 이벤트를 묶는다. 주소 하나가 여러 사람이 사용하는 지갑일 수 있으므로 주소 자체를 전 계정의 단일 소유자 제약으로 만들지 않는다. 변경 없이 재요청하면 동일 결과를 반환한다.

키 import는 이 transaction의 payload가 아니다. 앱→기기의 보호된 경로에서 끝내고 서버에는 검증 가능한 공개 주소/참조만 등록한다.

### TX-03 · 주문 snapshot — SHOP-02, PAY-03

입력: 매장·메뉴 revision·item/option/수량과 주문 멱등 key.

서버 가격/품절/수취 설정을 읽어 검증하고 order·order_lines·recipient snapshot·멱등 결과를 함께 저장한다. 과거 snapshot은 이후 메뉴/주소 설정 변경으로 덮어쓰지 않는다. 메뉴가 변경됐으면 새 금액 확인을 요구하며 자동 결제하지 않는다.

### TX-04 · 지급 증거 배정/수락 — PAY-04, INDEX-06

입력: 검증된 tx/log·attempt 귀속·정규성/확정 정책·관측 revision.

`payment evidence → order/attempt` 배정 고유 제약을 검사한다. 이미 다른 주문에 배정된 증거는 거절한다. acceptance 변경·업무 revision·outbox를 묶고 매출/혜택/영수증 소비자는 멱등 반영한다. 같은 금액이라는 이유로 자동 귀속하지 않는다.

재구성으로 evidence가 철회되면 이전 배정을 삭제해 다른 주문이 가져가게 하지 않는다. 원배정 이력과 보정 상태를 유지한다. 이미 제공한 상품은 추가 확인 예외로 남긴다.

### TX-05 · 환불 한도 예약/완료 — SHOP-04

입력: 원지급·동일 자산·검증된 목적지·양수 금액·요청 key.

```text
lock 또는 compare-and-swap(payment_refund_balance.revision)
available = policy_refundable_amount - confirmed_refunds - active_reservations
if amount > available: reject REFUND_LIMIT_EXCEEDED
insert refund(requested, destination_snapshot)
insert reservation(active, amount)
increment revision
commit with idempotency result
```

업무 승인·서명·체인 제출은 후속 단계다. 제출이 unknown이면 예약을 유지한다. confirmed 관측 시 동일 transaction에서 예약을 소진하고 완료액에 더한다. 실제 실패/중단이 확인되어 예약을 해제할 때에도 아직 유효하게 제출될 서명 거래가 남았는지 정책상 확인한다. 같은 완료 이벤트를 두 번 처리하지 않는다.

### TX-06 · 혜택 발급·사용·보정 — STAMP-01

발급은 `(source_payment, rule_version, benefit_kind)` 고유 제약을 갖는다. 사용은 유효 상태/사용 가능 수량 검사와 소비 기록을 원자적으로 묶는다. 원지급 환불/철회는 보정 entry로 남긴다. 이미 사용한 혜택의 처리 정책은 규칙 버전에 연결하며 원장 과거 이력을 삭제하지 않는다.

### TX-07 · 반납 clearance·완료 — STAMP-03~04

점검에는 rental/device/binding·wallet origin·대상 자산·미확정 거래·자격/위임·외부 접근 proof의 revision을 기록한다. 서버가 정한 필수 점검을 모두 충족하면 만료가 있는 clearance를 발급한다. 자산 상태/권한이 바뀌면 기존 clearance의 사용 가능성을 다시 검사한다.

실기 초기화는 DB transaction 외부다. 완료 시 clearance와 장치 증거·물리적 승인·현 binding을 검증하고 다음을 함께 반영한다: clearance 소비, 이전 binding 철회, rental returned, 재대여 가능 상태, outbox. 증거 유실은 pending이며 운영자 checkbox만으로 통과하지 않는다.

신규 여행 지갑은 자산 회수 경로를, import 지갑은 외부 접근 확인 후 기기 사본 삭제 경로를 적용한다. 무관한 기존 지갑 자산/권한을 자동으로 변경하지 않는다.

### TX-08 · 정산 마감·보정 — SHOP-05

집계의 기간·자산·원장 revision·관측 시점을 고정한다. 마감 조건과 미해결 차이를 검사한 뒤 settlement snapshot을 저장한다. 마감 뒤 늦은 입금/환불/관측 보정은 새 revision 또는 조정 기록으로 연결하고 기존 마감 수치를 조용히 덮어쓰지 않는다.

### TX-09 · 녹음/AI 작업의 동의·삭제 — REC-03, TRIP-04

job 생성 시 owner/파일 접근/동의 purpose와 revision을 기록한다. 실행 전과 결과 저장 직전에 다시 검사한다. 철회·삭제가 먼저 도착하면 결과의 신규 공개/저장을 막고 처리 저장소·원음 사본·전사·요약·검색 입력의 삭제 작업을 추적한다. 로컬 원음과 클라우드 metadata 삭제는 별도 결과다.

작업 취소 응답만으로 외부 제공자의 저장까지 삭제됐다고 표시하지 않는다. 각 처리자의 확인 가능한 삭제 결과를 집계한다.

### TX-10 · 코스·챌린지 증거 — AI-02~03

코스 저장은 owner·revision·장소/시간 검증을 확인한다. 챌린지 진행은 `(participation, step, accepted_evidence)` 및 보상 고유 제약을 적용한다. 증거의 현재 validity·출처·관측 revision을 확인하고 진행 변경과 outbox를 저장한다. 후기/결제/위치 철회는 해당 evidence를 참조한 코스/진행을 찾아 재평가한다.

### TX-11 · FOTA 릴리스 게시/철회 — OTA-02/04

manifest·package hash·model·compatibility를 검사하고 변경 불가한 release 버전을 저장한다. 게시·철회는 revision과 운영 권한을 검사하고 이벤트를 발행한다. 패키지를 바꾸려면 새 릴리스를 만든다. 철회했다고 이미 부팅한 기기가 자동 이전 버전으로 바뀌지는 않는다.

### TX-12 · 운영 재처리·감사 — OPS-03, RELEASE-02

재처리 요청에는 actor·대상·사유·멱등 key를 저장한다. 원천 조회/검증으로 projection을 갱신하고 어떤 revision을 반영했는지 기록한다. 반복 조치에도 매출/혜택이 늘어나지 않아야 한다. 운영자가 체인 성공 상태를 임의 생성하는 저장 명령은 제공하지 않는다.

### TX-13 · MPC 생성·서명 세션 — MPC-02~05

account/wallet 생성 key와 참여자 참조·profile·세션 digest·상태를 저장한다. 실제 조각은 선택한 참여자 경계에만 둔다. 부분 생성/참여자 timeout은 별도 실패 상태로 남기고 기존 세션의 중간 결과를 새 세션에 임의 재사용하지 않는다. 지갑 생성이 끝나기 전 앱에는 provisioning 상태를 표시한다. 복구/참여자 교체 완료와 기존 참여자 철회 상태를 구분해 기록한다.

### TX-14 · 자격 발급·검증·철회 — DID-02~03, STO-02

발급 권한·subject binding·profile·자격 ID를 저장하고 개인 claims와 공개 검증 상태를 분리한다. 검증 결과는 검증 시각·challenge·status revision을 포함한다. 철회 이후에는 과거의 유효 판정을 최신으로 재사용하지 않는다. STO 자격 판정에 재사용할 경우 연결한 credential/status revision을 기록한다. 공개 체인 철회 transaction은 제출/확정 상태를 별도로 추적한다.

### TX-15 · 유료 자원 지급·전달 — X402-02~03

resource request에 자원/가격/지갑·지불 요구·증명 profile을 고정한다. 검증한 payment와 entitlement 배정을 중복 방지하고, 자원 전달 성공/실패는 지불 상태와 별도로 저장한다. 지불 후 응답 유실은 기존 이용권으로 재조회하며 새로운 과금을 자동 생성하지 않는다. 지급 실패/되돌림/이용권 소비 정책은 선택 profile과 자원 규칙에 연결한다.

### TX-16 · 시장 실행·keeper 체크포인트 — DEX-02, FX-02, PERP-02~06

quote/intent에는 실제 입력/최대 투입·예상 출력/최소 수령·유동성 지분·가격 출처/시점·기한을 고정한다. swap과 LP 입출금은 다른 action variant다. 계약 자산/포지션이 기준이며 업무 DB에는 요청·제출·실행 결과와 조회 projection을 저장한다. keeper 작업은 시장/행위/대상 구간으로 중복 방지하고, 외부 제출 timeout 후 nonce/tx를 조회한 뒤 재시도한다. 가격 갱신·펀딩·청산을 하나의 성공 boolean으로 묶지 않는다.

## 3. 최소 데이터 제약과 조회 경로

| 조회/변경 | 키·필터 | 필요한 제약/색인 방향 |
|---|---|---|
| 계정 로그인 | provider, subject | 고유 연결 |
| 매장 접근 | store_id, account_id | 소속 고유·역할 조회 |
| 현 기기 대여 | device_id, active state | 활성 binding/대여 중복 방지 |
| 주문 목록 | store_id, created_at, order_id | 기간별 안정 정렬·cursor |
| 거래 복구 | chain_id, tx_hash / intent_id | 제출 중복 제거·시도 이력 |
| ERC20 증거 | chain_id, tx_hash, log_index | 증거 정체성 고유, block/revision 이력 별도 |
| 환불 한도 | payment_id, revision | 원자 조건부 갱신/잠금 |
| 스탬프 발급 | source_payment, rule_version, kind | 재발급 중복 방지 |
| 개인 기록 | owner_id, created_at, recording_id | 소유자별 조회·삭제 인덱스 |
| 추천 영향 찾기 | evidence_id, consumer_id, source_revision | 출처 철회 역참조 |
| event 중복 | consumer_id, event_id | inbox 고유 |
| retry 결과 | principal_scope, action, idempotency_key | 멱등 고유·안전한 결과 참조 |

SQL 문법과 partial unique index 등 구체 수단은 선택 DB에 맞춰 정한다. 논리 제약은 어떤 저장소를 쓰더라도 유지한다. 전체 DB schema에는 nullable·FK·삭제 규칙·마이그레이션 순서를 추가해야 한다.

## 4. 복구/삭제 검사 시나리오

| 시나리오 | 기대 결과 |
|---|---|
| 같은 provider callback 동시 도착 | account/identity 한 번 연결 |
| 같은 key로 주문 가격을 바꿔 재시도 | 충돌, 새 주문 금액으로 자동 변경 금지 |
| 한도 10 중 동시 환불 7+7 | 합계 14 예약 불가; 한 건 거절/재조회 |
| 제출 불명확 환불에서 화면 닫기 | 예약 유지·원천 조회 |
| 한 개 쿠폰을 두 단말에서 사용 | 한 번만 소비 |
| clearance 이후 binding 또는 점검 revision 변경 | 반납 완료 거절·재점검 |
| AI 작업 중 동의 철회 | 결과 새 공개 차단·삭제 작업 추적 |
| 재구성 이벤트 중복/역순 전달 | revision 대조·원천 조회·원장 보정 |
| 백업 복원 후 과거 동의/소유 binding 등장 | 철회/삭제 로그 재적용 후 노출 재개 판단 |

실제 동시성·장애 주입 시험은 DB/서비스 구현 후 수행한다. 이 문서는 그 시험의 기대 결과와 저장 경계를 지정한다.


## 승인 기준의 추가 저장 경계

[승인 저장 mapping](approval-storage-mapping.json)에 AI-R 5개와 SS-R 8개를 기준 논리 자원으로 반영했다. API017/034/037은 원 intent·snapshot·부모 operation·ancestry를 같은 commit으로 생성한다. 발급/교체는 predecessor CAS, protected response 참조, replay 소비와 함께 처리한다. GET의 보안 replay metadata 쓰기와 업무 원장 쓰기를 구분한다. 기존 reference SQL/19개 시험은 이 추가 제약을 검증하지 않았다.

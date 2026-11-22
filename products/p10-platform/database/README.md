# 최초 데이터 마이그레이션 · 02

[원 설계와 소유자](../../../docs/content/commercial/architecture/README.md) · [데이터 관계와 운영 작업](../../../docs/content/commercial/architecture/ledger.md) · [수집 목적과 삭제](../../../docs/content/commercial/architecture/privacy.md)

| 설계 DDL | 배포 단위 | 이번 범위 |
|---|---|---|
| `ledger.sql` | [P05 002](../../p05-operations-backoffice/internal/store/migrations/002_ledger.sql), [P07 002](../../p07-indexer/migrations/002_chainview.sql) | P05 지점, 로컬 계정–역할, 최소 주문 및 요청 영수증. P07 배포 스트림, 리플레이 세대, 확정 관측과 조회 대상. 이후 범위는 주문·정산·키오스크 실행 계획에 배정 |
| `platform.sql` | [P10 001](migrations/001_platform.sql) | 배포·검증·공지/승격·복원 승인과 기록 |
| `privacy.sql` | [P10 002](migrations/002_privacy.sql) | 규칙 버전, 제한된 접근 증거, 제품 범위별 권리 요청 |
| `platform.sql` | [P10 003](migrations/003_recovery_control.sql) | 코호트 입구 펜스, 독립 서명 계획, 대상 CAS 및 전환 영수증 |
| `platform.sql` | [P10 004](migrations/004_release_fleet.sql) | 기존 배포 원장과 서명 승인·설치 영수증·코호트 배포 상태의 연결 |
| `platform.sql` | [P10 005](migrations/005_fleet_disclosure.sql) | 공개 공지·지원 증거의 최소 투영과 코호트별 만료 검토 권한 |
| `platform.sql` | [P10 006](migrations/006_operator_authorization.sql) | 복원 운영자의 만료 권한과 DB 대기 후 권한 재검증 |
| `platform.sql` | [P10 007](migrations/007_board_disclosure.sql) | 승인된 설치 영수증의 보드 부트 키와 보호 구간 해시 공개 투영 |
| `platform.sql` | [P10 008](migrations/008_fleet_reapproval.sql) | 부분 설치 후 재승인과 기존 영수증의 대체 관계 |
| `platform.sql` | [P10 009](migrations/009_ios_profile_disclosure.sql) | 새 설치와 기존 연습 기록 연속성의 공개 지원 범위 |
| `platform.sql` | [P10 010](migrations/010_android_profile_disclosure.sql) | Android 설치판과 연습 기록 보존의 공개 지원 범위 |
| `platform.sql` | [P10 011](migrations/011_recovery_rehearsal.sql) | 범위 제한 복원 접수·상태·서명 보고서와 작업자 권한 |
| `platform.sql` | [P10 012](migrations/012_sdk_interop_disclosure.sql) | C·TypeScript·Go·Python 소비자별 설치 영수증과 실제 측정 항목의 공개 범위 |
| `platform.sql` | [P10 013](migrations/013_fleet_review_work.sql) | SKU별 최소 검증 근거, 코호트 보류와 만료 쓰기 권한 |
| `wallet.sql` | [P03 005](../../p03-cloud-mpc-wallet/migrations/005_closure_receipt.sql) | 지갑별 원 종료 접수 참조·세대·접수 시각. 해당 세대의 실제 처리 결과에서 완료를 판정한다. |
| `wallet.sql` | [P03 006](../../p03-cloud-mpc-wallet/migrations/006_sign_refusal.sql) | 원래 서명 요청의 영속 거절 상태와 한 번의 최소 감사 사건 |
| `experience.sql` | P08과 P09 각 소유 | P09는 작업 17부터, P08은 작업 20부터 각 저장소 배포 |

PostgreSQL 15 이상을 사용한다. P05/P07의 초기 SQL과 데이터는 v1 `public` 스키마에서 보존한다. P07에는 출처, ABI 및 헤더 해시가 없는 기존 row를 신세대 정규 체인 근거로 옮길 수 있는 증거가 없다. 소비자는 각 소유자 API의 불투명 참조를 전달한다. P03/P08/P09는 각 제품 마이그레이션 진입점에서 별도 저장소에 적용한다. 키오스크 inbox 및 선택 수집은 해당 제품 진입점에서 확장한다.

## 적용과 재개

제품마다 DDL 권한을 가진 관리 연결을 따로 둔다. 배포 서비스의 런타임 연결은 소유자가 발급한 제한 역할을 쓴다. `Pxx_MIGRATION_DATABASE_URL`에는 암호 없는 libpq 서비스 참조나 접속 대상을 지정한다. 인증은 `PGSERVICEFILE`/`PGPASSFILE`에서 읽는다. 구버전에서 올린 DB의 `public`에 PUBLIC CREATE가 남아 있다면 DBA가 권한을 회수한 뒤 적용한다.

```sh
P05_MIGRATION_DATABASE_URL='service=ops-migrator' make -C products/p05-operations-backoffice migrate
P07_MIGRATION_DATABASE_URL='service=indexer-migrator' make -C products/p07-indexer migrate
P10_MIGRATION_DATABASE_URL='service=platform-migrator' make -C products/p10-platform migrate
```

각 소유자의 잠금, 마이그레이션 DDL, 버전·파일명·SHA-256 journal은 revision별 단일 DB 트랜잭션으로 처리한다. 적용 도중 세션이 종료되면 새 연결에서 같은 명령으로 재개한다. 적용된 파일의 digest가 달라지거나 DB 버전이 더 높으면 중지한다. DDL과 목적·근거·기간 참조·폐기 정책의 필드 목록을 같은 트랜잭션에서 대조한다. 새 필드의 등록이 빠졌으면 DDL까지 되돌린다. 적용 파일은 이후 변경 대신 다음 번호를 추가한다. 역할 생성·부여에는 DB 관리 권한이 필요하다.

## P05 초기 DB 계약

지점 업무 연결은 PostgreSQL 인증 계정마다 별도 pool 또는 연결을 사용한다. 원장 관리자가 로컬 난수 subject와 `ledger.login_subject`를 연결하고 `ledger.staff_grant`에 지점·역할·유효 시각을 부여한다. `nu54_p05_staff`는 제한 함수 실행 권한만 묶는 역할이다. 요청 본문이나 세션 변수는 로그인 또는 branch scope의 권위로 사용되지 않는다. 운영자는 주문 생성, 재무 역할은 주문 생성과 지점 CSV용 projection 조회를 사용한다. 재무 CSV 어댑터는 동일한 사용자 연결에서 `ledger.export_sales(branch)`를 호출한다. opsd의 인증 및 HTTP/CSV 경로는 작업 07·13의 범위다.

`ledger.create_sale`는 주문과 명령 영수증을 한 트랜잭션에 삽입한다. scope는 지점·인증 subject·작업·임의 요청 키다. 같은 intent 재시도는 기존 주문 ID를 반환한다. 다른 intent는 `P0501`, 만료된 동일 intent는 `P0502`, 승인 정책 미설정은 `P0503`, 지점/역할 거절은 `42501`이다. 금액은 부동소수점 변환 없이 uint256 범위의 양의 정수 최소 단위를 사용한다. HTTP에서 매핑할 공통 오류 봉투는 작업 04에서 합의한다. 명령 영수증 TTL은 지점의 목적·근거·폐기 규칙에 따른 설정이며, 사용 중인 계약·원주문과 결합된 fence 폐기는 후속 운영 절차에서 다룬다.

## P05/P07 견적·회차·환불 조회

| 배포 DDL | 목적과 공개 범위 |
|---|---|
| [P05 090](../../p05-operations-backoffice/internal/store/migrations/090_merchant_gas_sponsorship.sql) | 살아 있는 원견적에 최초 매장 가스 책임 지문을 한 번 결속한다. `nu54_p05_commerce`는 현재 권한을 다시 확인하는 제한 함수만 실행한다. 신규 단말 제출은 같은 유효 지문을 요구하고, 이전 접수는 정확한 기존 영수증으로 찾는다. 기존 암호문·서명과 보유기간을 유지한다. |
| [P05 044](../../p05-operations-backoffice/internal/store/migrations/044_quote_boundary_origin.sql) | 기존 재무 대사 권한 아래 같은 지점의 보관 견적을 검증한다. 발급 서버는 mTLS 탐색기에 서명 마감, 견적 해시와 배포 일치 정보만 제공한다. |
| [P05 045](../../p05-operations-backoffice/internal/store/migrations/045_subscription_invoice_status.sql) | 기존 청구서·동의 일정·원주문·서명 견적·미소비 확정 사건으로 회차 상태를 조회한다. 고객 함수에서 현재 지점과 등록 주체를 확인한다. 새 보유 필드는 없다. |
| [P05 046](../../p05-operations-backoffice/internal/store/migrations/046_refund_queue.sql) | 기존 지점·재무 권한·원매출·지급·환불·미처리 제출을 결합해 요청과 허용되는 다음 행동을 최대 50개씩 조회한다. 신청자·승인자 식별자와 지갑 주소는 결과에서 제외한다. |
| [P05 047](../../p05-operations-backoffice/internal/store/migrations/047_terminal_mode_handoff.sql) | 단말 모드를 전환할 때 요청·원버전·단말·원주문 참조를 기록한다. 지점 운영 인계와 지정 단말 확인이 모두 도착하면 전환한다. 이미 접수한 주문은 같은 영수증으로 조회하고, 인계 관계에는 구매자 식별자·결제 금액을 복제하지 않는다. |
| [P05 048](../../p05-operations-backoffice/internal/store/migrations/048_kiosk_staff_access.sql) | 지점별 기존 권한에 픽업 담당 역할을 추가한다. 단말 설정·픽업·환불 경로를 현재 인증 주체와 지점 권한에 따라 열고, 단말 인계 주문은 페이지별로 조회한다. 추가 고객 식별 필드는 없다. |
| [P05 049](../../p05-operations-backoffice/internal/store/migrations/049_notification_review.sql) | 재처리 검토 시 기존 사건·원주문·분개·수신 영수증을 연결한다. 조회 권한·지점·담당자 범위 안에서 완료 사건의 원래 조회 ID를 다시 읽는다. 이 변경에서 추가한 영속 필드는 0개다. |
| [P05 050](../../p05-operations-backoffice/internal/store/migrations/050_customer_order_list.sql) | 지점과 등록 구매자 권한으로 기존 주문, 임시 클레임, 확정 지급의 서명 견적을 결합한다. 페이지당 최대 50개, 임시 접근권은 응답 시각에 다시 확인한다. 추가 영속 필드는 0개다. |
| [P05 051](../../p05-operations-backoffice/internal/store/migrations/051_customer_order_receipt.sql) | 원견적의 구매자에게 같은 매출의 제출·릴레이·확정 지급·픽업 근거를 반환한다. 타인의 임시 조회 권한으로는 원지급 해시와 수령 영수증을 조회할 수 없다. 새 영속 필드·테이블은 없다. |
| [P05 052](../../p05-operations-backoffice/internal/store/migrations/052_print_staff_console.sql) | 직원의 인증서·지점 출력 권한으로 영수증별 기존 출력 시도와 결정을 조회한다. 한 시도에서 행동별로 한 결정을 허용해 결과 확인 후 별도 승인한 사본을 발급한다. 새 수집 필드는 없다. |
| [P05 053](../../p05-operations-backoffice/internal/store/migrations/053_kiosk_cafe_pickup.sql) | 두 픽업 상태·전이 외래 키가 확정 지급의 공통 픽업 ID를 참조한다. 지점 직원만 메뉴 주문을 이행하며, 고객 영수증은 기존 구매자 범위에서 상태를 조회한다. 추가 필드는 없다. |
| [P05 054](../../p05-operations-backoffice/internal/store/migrations/054_pickup_assignment_authority.sql) | 픽업 전용 단말은 응답 시점에 기존 지점 권한과 지정 행을 함께 확인한다. 지정 회수 뒤 고객 주문 노출을 막고 기존 픽업·영수증 참조를 유지한다. |
| [P05 055](../../p05-operations-backoffice/internal/store/migrations/055_pickup_client_compatibility.sql) | 기존 여행 목록·고객 영수증 형식을 보존한다. 카페 접수는 지점 직원의 명시 조회로 제한하고 기존 지급과 픽업을 참조한다. |
| [P05 056](../../p05-operations-backoffice/internal/store/migrations/056_finance_original_order.sql) | 지점 재무 조회 권한으로 기존 원주문·발급 견적·제출이 확인된 릴레이·확정 지급·이행을 연결한다. P07 조회용 상관 값은 운영자 응답에서 제외한다. 기존 필드만 사용한다. |
| [P05 057](../../p05-operations-backoffice/internal/store/migrations/057_payout_registry_review.sql) | 지급 주소 변경의 제안 당시 발급 프로필 버전과 확정 등록부의 실제 효력 시각을 기록한다. 구견적의 원지급·미지급과 등록부 채택 여부는 지점 권한으로 조회한다. 이전 기록은 보관된 변경 거래로 확인되는 연결만 표시한다. |
| [P05 058](../../p05-operations-backoffice/internal/store/migrations/058_rental_opening_review.sql) | 등록된 장비의 실제 검증 시각을 선택적으로 보관하고 해당 세대의 확인 화면을 현재 계정·반환 경로·인증 근거에 결속한다. 이전 등록에는 실제 유효 기한을 사용한다. |
| [P05 059](../../p05-operations-backoffice/internal/store/migrations/059_menu_checkout_context.sql) | 유효한 지점 단말 등록과 원주문 생성자를 확인한 뒤 메뉴 주문의 지점명·결합 여부를 조회한다. 단일 청구권의 고객 참조는 단말에서 선택 결과를 대조하는 데 쓰며 고객용 BLE 응답에서 제외한다. 새 저장 필드는 없다. |
| [P05 060](../../p05-operations-backoffice/internal/store/migrations/060_travel_booking_review.sql) | 원예약의 이용 시각·가격 버전·노쇼 조건·결제 체인을 보존한다. 과거 예약에 근거가 없는 값은 비워 두고, 예약금·잔금은 각각 같은 지급 단계의 원주문을 다시 조회한다. |
| [P05 062](../../p05-operations-backoffice/internal/store/migrations/062_access_order_handoff.sql) | 회수 검토의 원주문 집합과 함께 승인한 지점 담당자 참조를 고정한다. 두 운영자 승인 뒤 자격을 회수하고 같은 주문을 인계한다. 담당자만 접수하며 원주문 본문과 구매자 정보를 복제하지 않는다. |
| [P07 014](../../p07-indexer/migrations/014_explorer_quote_coverage.sql) | 기존 탐색기 역할에 활성 세대의 연속 확정 경계와 원 견적 대사에 필요한 기존 증거 필드를 투영한다. 조회 역할과 원장 처리 역할은 분리한다. |
| [P07 015](../../p07-indexer/migrations/015_finance_order_observation.sql) | 기존 대사 로그인과 배포·가맹점 권한으로 동일 거래의 정규 체인 관측 높이와 확정 근거 높이를 조회한다. 기존 증거에 추가 수집·보존 항목은 없다. |

P05 057·058은 기존 지급 변경·대여 등록에 선택 필드 세 개를 추가한다. 목적·보유·폐기 규칙은 [필드 목적 목록](field-purpose.json)에 있다. [지급 변경·대여 ERD](../../../docs/content/commercial/architecture/erd.md#지급-주소-변경과-대여-기기의-개설-검토) · [체인 관측 ERD](../../../docs/content/commercial/architecture/erd.md#chainview) · [설정 및 시험](../../p07-indexer/chainview-v2.md#x06-original-quote-and-contiguous-coverage)

## 검증

```sh
python3 products/p10-platform/database/test_migrate.py
python3 products/p10-platform/database/test_composition.py \
  --socket /tmp/<scratch-pg-directory> --port <port> --user <scratch-admin>
```

시험기는 로컬 임시 PostgreSQL 클러스터에 매번 임의 DB 두 개를 생성하고 끝에 삭제한다. 빈 DB와 v1의 합성 가맹점·반납 대기·attestation·감사·영수증·cursor에서 전진 적용한다. 002 중 오류로 rollback한 뒤 같은 명령으로 재개하고, 반복 적용·digest 변경·8개 동시 재제출·다른 지점 조회 및 CSV 거절·목적 없는 신규 수집 거절을 검증한다. 실제 DB catalog의 PK/FK 관계를 설계 범위와 대조하고 새 base table 필드는 [필드 목적 목록](field-purpose.json)과 비교한다.

필드 목적 목록은 설계 분류다. 법적 보유 기간, 국가·수신자, 승인자, 파기 집행과 복원 파기 전파 근거는 실제 출시 정책에서 확정한다. 테스트 정책·날짜는 합성값이다. 최초 v1 기준선에서는 P06 합성 결제가 여행 기록과 독립적으로 처리되는지 검증했다. 현재 P09는 별도 비공개 스키마에서 목적별 동의·철회와 삭제 요청을 처리한다. 공개 장소 탐색에는 개인 기록 자격이 필요하지 않으며 주문·지급 원장은 P05가 소유한다. 11월 21일 새 PostgreSQL에서 미가입 상태와 위치 동의 철회 상태를 각각 시험했다. 두 경우 모두 공개 지역 검색 뒤 인증한 고객이 좌석을 확보하고 같은 예약을 재조회했다. P09 개인 스키마의 17개 테이블에서 검사 구간의 행 변경·초기화가 0건이었다. 공개 조회 역할의 개인 테이블 권한도 0건이었다. 설치 앱에서 같은 사용자가 선택 수집을 거절한 뒤 수동 검색부터 동일 예약의 지급·영수증까지 이어지는 과정은 추가 검증한다. [v1 결제 회귀](../baseline/README.md) · [P09 동의와 업무 경계](../../p09-travel-ai/PRIVATE-TRAVEL.md)

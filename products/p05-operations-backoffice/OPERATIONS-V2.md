# 지점 운영 콘솔 · 단계 13

[구현 계획](../../docs/content/commercial/implementation-plan.md) · [운영 화면](../../docs/content/commercial/screens/specs/O.md) · [단말 운영](../../docs/content/commercial/screens/specs/K.md)

| 인수 단위 | 권한과 정상 경로 | 거절·재개 시험 |
|---|---|---|
| 13A · O03 메뉴·가격·재고 | 운영자 mTLS와 활성 지점 권한을 확인한 뒤 읽는다. 기존 메뉴 버전을 비교해 새 유효기간과 가격/옵션을 함께 게시한다. 재고 변경은 동일 버전에 건별 수량 증감으로 처리한다. | 다른 지점, 철회된 운영자, 동시 게시, 마이너스 재고와 서로 다른 본문의 동일 요청 키 거절. 응답을 잃으면 저장한 요청 키로 재개. |
| 13B · O01/O02/O04/O05/O06/O12 정산·출고 | 확정 지급/환불/마감과 출고 조회는 원장 권한을 분리한다. 범위가 제한된 CSV에는 확정 버전과 보류 근거를 제공한다. 프린터/메시지 결과 미상은 별도 직원 조회에서 확인하고, 동일 출고 건의 재시도를 사용한다. | 타 지점 및 셀프 환불 승인 거절. 방송·출고·보고 결과가 없으면 확정 표시 없음. |
| 13C · O08/O13 지급 주소·권한 전환 | 새 주소 소유 증명, 서로 다른 두 운영 주체와 효력 시점을 기록한다. P06 `rotateMerchant` 실행과 실제 온체인 버전 대사를 마친 뒤 다음 발급을 활성화한다. 기존 견적은 등록 조건·다이제스트를 고정한다. 직원/단말 역할 변경은 별도 동의 증명과 즉시 권한 평가를 사용한다. | 이전 유효 견적과 신규 버전 각각 지급, 서명 주체 중복·타 지점 거절, 응답 유실 후 같은 트랜잭션 조회. |
| 13D · O09/O15 대여 개설·반납 및 인출 | 기존 opsctl 로직을 지점·명령 인증에 감싸고 기존 v1 주문·정산 확인 및 재시도 키로 실행한다. | 거래 미확정과 결과 미상은 재조회, 같은 기기 중복 반납·다른 지점 및 개인 슬롯 처리 거절. |
| 13E · O10/K12 FOTA | 승인한 서명 이미지와 보드 호환성, 단계·지점·대상 버전을 확인한 뒤 디바이스 단위 ACK를 수집한다. 적용 도중 중단은 실제 실행 버전·검증 결과를 다시 읽는다. | 다른 지점·잘못된 이미지·낮은 버전 거절. 세션 단절·재부팅과 배포 중지 이후 기기별 대사. |

운영 콘솔은 배포에서 발급한 운영자 브라우저 인증서를 사용한다. 자격과 tenant는 HTTP JSON에서 선택할 수 없다. 큰 글씨 설정에서도 확인과 취소를 분리하고, 읽기 상태와 명령 결과 미확정을 구분한다.

## O03 콘솔과 서비스 실행

```sh
pnpm --filter @nu54/p05-backoffice-web build
make -C products/p05-operations-backoffice migrate
```

상용 백오피스 데이터베이스는 UTF8 인코딩을 사용한다. 기존 상용 opsd TLS 프로필에 `P05_OPERATOR_ORIGIN`을 콘솔의 HTTPS origin으로, `P05_CONSOLE_DIR`을 웹 빌드 `dist`의 절대 경로로 설정한다. 브라우저에서 발급한 운영자 클라이언트 인증서로 `/ops/`를 연다. 지점 참조를 조회하면 서버가 현재 버전·옵션·가격·수량을 제공한다. 확인 화면에서 제출할 때 요청 키를 먼저 저장한다. 네트워크 오류 또는 재시작 이후에는 같은 인증서와 지점으로 저장한 요청의 결과를 확인할 수 있다. 서버는 인증서에 등록된 tenant와 활성 운영자 부여를 매 요청에서 확인한다.

### 13A.2 메뉴 운영 후속 범위

O03 최초 화면은 원장 기준 단위와 등록 자산 주소를 입력받는다. 다음 단위는 인증된 자산 메타데이터에서 기호·소수점을 읽어 TestUSDC 금액을 표시하고, 세금·할인과 필수 옵션, 영업시간을 원장 버전 스키마와 단말 가격 합의까지 연결한다. 게시 확인에는 옵션별 이전/신규 금액과 제거를 비교한다. 다른 운영자의 게시가 먼저 확정된 경우 편집본을 유지한 채 새 기준 버전을 조회하고 사용자가 차이를 병합하도록 한다.

13A.2에서는 8283 발행 프로필에 등록된 자산과 6자리 소수점을 운영 API가 내려준다. 화면은 고객이 지불하는 금액을 표시한다. 서버는 옵션 단위의 기본 가격 + 세금 − 할인으로 양의 정수 최소 단위를 계산해 기존 주문 API의 `unitPrice`에 게시한다. 각 상품은 하나의 옵션 선택군을 쓰며 필수 선택군에는 묵시적인 `standard` 기본값을 허용하지 않는다. 영업 시간은 단일 IANA 시간대 내 매일 같은 당일 구간을 지원한다. 시작 포함, 종료 제외다. 새 판매·견적 발행은 현재 영업 중인 최신 가격 버전만 허용한다. 기존 예약의 같은 요청 결과를 보존하고 이미 발급된 견적은 원래 서명 바이트를 재사용한다. 가격과 시간은 게시 요청 영수증 및 가격 버전에 결합된다.

실제 합성 mTLS 운영 콘솔에서는 7.10 + 0.10 − 0.20을 7.00 TestUSDC로 표시해 게시했다. 다른 게시가 먼저 확정되자 7.50 편집값을 유지했고 최신 값 7.20 및 수정값 7.40을 다시 비교해 게시했다. 820 CSS 픽셀 뷰포트의 200% 글씨 및 키보드 진입에서 가로 넘침이 없었다. 격리된 PostgreSQL 정상 마이그레이션 001–016과 `TestPostgresOperatorCatalogRealTLS`는 같은 계산의 7,000,000 최소 단위 주문, 잘못된 자산·필수 옵션 거절, 공유 재고, 영업 종료·이전 버전 신규 주문 거절 및 기존 예약 재응답을 검증한다.

### O03 게시 카테고리 데이터

기존 게시 본문에 `categories`와 `categoryBindings`를 함께 보낼 수 있다. `categories`의 각 항목에는 `key`, `sortOrder`, 두 언어의 `labels.ko`·`labels.en`이 있다. `categoryBindings`는 `sku`와 `categoryKey`를 연결한다. 미분류 SKU는 연결 배열에서 생략한다. 오류가 있는 배열은 가격과 함께 거절한다. 배열의 순서만 바꾼 재전송은 같은 명령이며, 키·이름·표시 순서 또는 SKU 연결을 바꾸면 원 요청 키와 충돌한다. 두 필드를 모두 생략한 이전 본문의 요청 지문과 재시도는 유지된다.

운영 조회에는 게시된 분류와 연결 배열이 포함된다. 고객과 단말의 읽기에는 각 행의 선택적 `category`와 같은 버전의 `catalogVersion`이 포함된다. 인증된 근거리 경로는 같은 행을 전달하며 `snapshotVersion`으로 빈 마지막 페이지에서도 버전 변경을 감지한다. [069 게시와 조회](internal/store/migrations/069_menu_categories.sql) · [공통 SDK 검증](../../packages/protocol/ts/src/commerce-v2.ts)

### 13B 영업일 운영 콘솔

운영 보고서·CSV 발급·대사는 서로 다른 지점 금융 권한을 요구한다. 보고서 및 CSV는 지점의 IANA 시간대와 마감 시각으로 정한 영업일 및 등록 정산 자산으로 고정한다. CSV에는 집계 숫자만 포함한다. 내보내기 담당자, 범위 및 당시 스냅샷을 기록하고 동일 키 재시도에는 같은 바이트로 답한다. 원주문과 통지는 공개 지점 화면에서 읽을 수 없다. 조사 대기열에는 동일 사건만 나타나며, 체인 대사 또는 미완료 후속 처리가 남아 있으면 마감 보류로 표시한다.

### 13C 지급 주소 변경 계약

소유 증명은 새 지급 주소가 서명한 EIP-191 텍스트다. 체인, 정산 컨트랙트, 가맹점, 지점, 요청 ID, 이전 등록 버전, 기존·신규 주소, 변경 사유 및 증명 만료 시각을 서명 내용에 포함한다. 제안 담당자에 이어 별도 두 보안 승인 주체가 검토하며, 동일 사용자에게 발급한 다른 인증서도 두 명으로 세지 않는다. 두 번째 승인 후 24시간 대기와 독립 운영 통지 확인을 거쳐야 집행 가능하다. 지점과 체인 상태가 다르면 원래 주소로 남고, 전송을 시도한 뒤 결과가 불명확하면 같은 거래를 조회한다.

금융 주소가 지점별로 독립적인 8283 가맹점 키를 사용하는 경우 P07은 활성 세대의 확정 구간이 영업일 전체를 덮는지 확인한다. P05와 P07은 입출금 거래 해시·로그 인덱스·포함 블록·종류·금액의 정렬된 기록을 비교한다. 합계가 같아도 기록이 다르거나 확정 구간이 부족하면 마감 보류다. 이행 미확정은 정확한 이벤트와 재처리 요청의 원 ID를 유지한다. 콘솔에서 마감하면 고정된 P07 기준 블록과 원장 스냅샷이 같은 요청 영수증에 연결된다. 기확정 마감 영수증은 이후 P07 장애를 만나도 같은 사용자·지점·영업일·자산·키로 조회할 수 있다. 헤드리스 이전 API의 마감 경로는 콘솔용 HTTPS Origin이 없는 기존 프로필에서만 제공된다.

금융 콘솔을 재현하려면 전용 `nu54_settlement_*` PostgreSQL에 P05 및 P07 정상 마이그레이션을 적용하고 `TestPostgresFinanceConsoleScopesAndReplay`를 실행한다. 실제 로컬 Chrome 검증은 빌드한 웹 `dist`와 [브라우저 검증 스크립트](test/finance/browser.sh)의 절대 경로를 각각 `NU54_FINANCE_BROWSER_DIST`, `NU54_FINANCE_BROWSER_SCRIPT`에 설정해 `test/settlement/anvil.sh -run '^TestLoopbackSettlementEvidenceAndRefund$' -v`를 실행한다. 정수 단위 회귀와 구분해 `P05_PILOT_HUMAN_UNITS=1`과 격리된 `P05_PILOT_OUTPUT`을 설정한다. 검증 실행에서 실제 Chrome이 opsd의 mTLS 원주문 뷰와 CSV를 읽었고 30.00 매출·3.00 환불·27.00 순매출, 다른 지점 권한 거절, 390px 화면에서 200% 글씨를 확인했다.

O08 자동 집행은 `P05_PAYOUT_DATABASE_URL`의 전용 `nu54_p05_payout_worker` 로그인, 별도 `P05_PAYOUT_GOVERNOR`, 절대 경로의 `P05_PAYOUT_SIGNER`와 그 `_SHA256`으로 활성화한다. 지급 실행 계정과 거버너 서명자는 분리한다. 신규 발급을 동결하고 열려 있는 기존 견적의 다이제스트 최대 256개를 저장한다. 동일하게 보관된 서명 거래만 제출·복구한다. 네트워크 및 컨트랙트 등록 버전과 최종 수취 주소를 정식 RPC 확정 블록에서 재검증한 뒤 신규 발급을 연다. `TestLoopbackPayoutApprovalAndFinalizedRotation`은 분리 승인·대기·통지·같은 해시 재개, 기존 견적의 이전 주소 수령 및 신규 견적의 변경된 주소 수령을 로컬 Anvil에서 확인한다. 앱의 등록 프로필 교체는 기존 키 배포 절차를 따르며, 갱신 프로필을 받기 전 앱은 새 지급 버전에서 결제 승인을 거절한다.

### 13D v1 대여

`P05_V1_RENTAL_RPC`, `P05_V1_RENTAL_MANIFEST`, `P05_V1_RENTAL_MANIFEST_SHA256`, `P05_V1_RENTAL_KEYSTORE`, `P05_V1_RENTAL_SECRET_REF`는 전용 v1 배포와 대여 운영 키를 등록한다. 원래 v1 관리 노트에 따라 현장 기기에서 개별 PIN/시간 앵커를 먼저 준비하고, 검증된 보드·계정·수취 주소·세대를 `ledger.rental_enrollment`에 등록한다. 요청자의 `rental_manage` 권한이 필요하다. 동일 세대별 예치·반납·인출·정리는 한 번만 송신 기회를 갖는다. 응답이 없으면 같은 계정과 거래 관측 상태를 조회한다. 인증된 기존 입금 계정과 수취 경로의 효과, 지연 인출, 미결 지급 보류를 해소한 뒤에만 물리 스테이션에서 기존 v1 서명 리셋을 시작한다. 금액이 남은 상태·토큰 수취 보류·다른 목적지는 보류다. 임대 세대 및 독립 관측의 신선도는 운영 등록부가 관리한다. 개인 슬롯의 배포 권한은 대여 등록부에 부여하지 않는다.

Set optional `P05_PUBLIC_EXPLORER_ORIGIN` to the separately published P07 HTTPS origin that serves the approved stream. Loopback HTTP is supported for local rehearsals. O04 keeps its original review when the setting is missing or an approved deployment, reconciliation grant or public receipt cannot be verified. A public link contains only stream, merchant, transaction hash and log index. It opens without the operator referrer.

`test/settlement/anvil.sh -run '^TestLoopbackManagedRentalFinalWithdrawal$' -v`는 로컬 v1의 예치, 실제 기본 한도, 확정 반납, 지연 인출 전 거절, 원 목적지 토큰 입금, 다른 목적지 보류를 확인한다. mTLS 수용 시험에서 등록 세대·타 지점 거절과 동일 명령의 단회 디스패치를 검증했다. 실물의 현장 등록과 리셋 후 상태는 파일럿 관측 대상으로 남는다.

세대 전체의 미결 판정은 기존 운영 확인과 별도다. 조회 상태, 실제 P01 동결 영수증과 P07 확정 대사의 인계 조건을 정의했다. [O09 세대 증거](O09-GENERATION-EVIDENCE.md)

### 13E 서명 펌웨어 캠페인

배포 명세는 [P10 생성기](../p10-platform/release/fota_descriptor.py)로 독립 배포 키에서 서명한다. 공식 명세는 검증된 공식 P10 증거와 동일한 펌웨어 페이로드·리비전·소스 커밋이 필요하다. opsd의 `P05_FOTA_CHANNEL`(official 또는 custom), `P05_FOTA_RELEASE_SIGNER`, `P05_FOTA_MCUBOOT_PUBLIC_KEY`와 `P05_FOTA_RELEASES`가 모두 등록되어야 한다. 릴리스 구성은 절대 경로의 `descriptor`, `signature`, `image` 객체 배열이다. 로더는 배포 키와 부트 키를 분리하고 MCUboot Ed25519·SHA-512 서명과 키 해시, 보호 영역의 롤백 카운터 및 앱 버전을 확인한다. 캠페인은 `firmware_manage` 담당자가 같은 리비전과 새 카운터의 등록 기기만 열 수 있다. 각 기기별로 `firmware_station` 담당자가 동일 바이너리를 내려받아 [기존 물리 유지보수 창](../p01-device-firmware/README.md#independent-hd-recovery-and-personal-evm-payments)에서 보조 슬롯에 전송한다. 월렛이나 운영 웹이 MCUboot의 물리 승인이나 시험 재부팅을 대신하지 않는다.

운영 화면이 생성한 C1 관리 명령은 `0x10 || 32바이트 challenge || 16바이트 campaign UUID`다. 소유자 인증 TLS 연결에서만 요청하고 MCUboot 확인 이후 보드의 별도 설치 P-256 키가 `N54F || challenge || campaign || board_id || SHA256(실행 중인 MCUboot 보호 영역)`에 서명한다. 증거에서 물리 기기의 식별자와 캠페인 및 이미지 측정을 대조한다. P02의 등록 기기 화면에서 서명 증거를 읽고 운영 화면에 입력하면 설치 결과를 검증한다. 기기 단위로 중지·재개하며 완료된 다른 대상의 상태는 바뀌지 않는다. 되돌림·회수·미결 승인 또는 다른 기기·요청·이미지 서명은 허용하지 않는다.

확인한 후보는 커스텀 키로 서명된 0.3.0, 카운터 2다. 실제 P01 NCS 서명 빌드 및 바이너리·HEX 서명과 보호 레이아웃 검사, P05 패키지 로더, 합성 보드 키의 응답으로 실제 mTLS 캠페인·중단·재개·재응답을 검증했다. 실기 전송과 설치 관측은 기록하지 않았다.

### 후속 콘솔 화면과 인수 범위

| 화면 | 현재 확인한 범위 | 남은 사용자 경로 |
| --- | --- | --- |
| O05–O06 | 기존 P05 재무 서비스의 예약 한도, 별도 승인, 같은 요청과 환불 거래 대사 | 실운영 인증서와 재원 승인 |
| O07 | P05 및 P09 주문·구독 API와 P02 소비자 흐름은 독립 제품 검수 | 실제 mTLS 브라우저의 예약·구독 검수 |
| O12–O13 | 지점 역할별 Chrome에서 자격 변경 제안·독립 승인·단말 ACK·이력을 확인했다. 미결 원주문 인계와 지정 담당자의 접수·재조회도 확인했다. | 운영 인증서와 실제 지점 업무의 인수 확인 |
| O14–O15 | 공식 v1과 v2 배포 분리, P07 확정 대사·원거래 참조, 인출 지연 및 입금 검증 로컬 시험 | 실제 O14·O15 브라우저와 로컬 체인 실행 경합 검수 |
| O08–O10 실기 | O08 실제 서명·방송·최종 등록을 로컬 Anvil에서 확인. O09 실제 v1 로컬 체인. O10 실제 서명 후보와 합성 P-256 보드 mTLS 캠페인 | 운영 장비 attestation·물리 반납·재부팅과 설치 증거 |

### O05–O07 운영 화면 연결

환불 검토는 확정 원지급, 원금·예약·진행·확정 환불, 원래 반환 경로와 독립 재원 관측을 표시한다. O05는 제한된 사유를 원장 요청에 결합하며, 같은 인증서·지점·원주문의 미확정 요청은 금액과 사유가 같은 키로 복구된다. O06은 제안자가 아닌 활성 승인자가 승인하거나 거절할 수 있다. 거절은 한 거래에서 예약 원금을 해제한다. 이미 승인·전송된 환불은 원주문에서 기존 상태를 조회한다. 프로덕션 웹 번들 및 로컬 Anvil 연동에서 서로 다른 Chrome mTLS 프로필로 신청·자기 승인 404·별도 승인을 확인했다. 기존 30/3/27 원장·CSV 회귀도 통과했다.

O07은 P05가 소유하는 예약금·잔금 주문과 구독 회차, 이미 기록된 이용권의 상태를 지점별 운영 권한으로 조회한다. 예약 고객 식별자와 재원 자격은 응답에 포함하지 않는다. `experience_cancel`은 검토한 구독 버전과 같은 요청 키를 요구하고 향후 회차만 종료한다. 다른 지점의 참조와 오래된 버전은 거절한다. 통합 mTLS 테스트는 해지 직전 발급 회차의 주문 ID와 발급 상태가 반복 해지 뒤에도 유지되는 것을 확인했다. O07의 실제 브라우저 검수는 별도 연결 시험으로 남았다.

### O03 게시 이름·영업일과 O12 지점 선택

게시한 메뉴 버전은 운영자가 입력한 한국어·영어 상품명과 옵션명, 알레르기 원재료 및 교차접촉 선언을 가격과 함께 보관한다. 원재료 목록 미입력, 해당 원재료 없음 선언 및 교차접촉 정보 미확인은 별도 상태다. P02와 P04는 같은 C3 카탈로그 데이터에서 사용자 언어로 이름을 선택한다. 원래 SKU·옵션·가격 버전은 장바구니에서 유지한다. 개별 옵션에 재고 상한이 있으면 상품 전체 재고와 옵션 상한 중 적은 수량을 안내한다. 게시 기한은 지점 시간대로 편집한다. 일광 절약 시간으로 존재하지 않는 시간은 거절하고, 두 번 발생하는 시간은 운영자가 오프셋을 선택한다. 예약은 지점 시간대, 구독·회차는 구독 시간대를 표시한다.

권한이 있는 기존 지점도 이름을 등록하기 전에 선택할 수 있다. 아직 이름이 없는 경우 인증된 지점에 짧은 참조를 표시한다. 지점 이름의 입력 및 저장은 활성 운영자에게만 허용된다. 자격이나 조회 결과가 바뀌면 이전 선택의 업무 화면을 지운다. O13은 서로 다른 활성 운영자 두 명이 검토한다. 직원 권한을 회수하면 해당 지점 금융·v1 증거·인출 재개 권한도 회수한다. 단말 교체 수신 확인은 등록된 대상 단말의 인증 경로에서만 받아들인다. 

자격 관리자는 회수할 지점·대상·역할을 선택해 미결 원주문 목록을 확인한다. 미결 주문이 있으면 같은 지점의 활성 운영자 중 `pickup_manage` 권한이 있는 인계 담당자를 확인한 뒤 요청을 기록한다. 목록이 비면 담당자 없이 회수를 제안할 수 있다. 검토 화면에서 취소하면 대상 자격은 계속 유효하다.

다른 자격 관리자는 같은 요청에서 대상·담당자·원주문을 확인하고 승인한다. 원주문 목록이 바뀌거나 담당자 자격이 만료되면 승인이 중단된다. 현재 목록과 유효한 담당자를 다시 확인해 제안한다. 승인 후 담당자는 **내가 인계받은 원주문**에서 회수 요청과 주문을 대조하고 **이 원주문 인계 접수**를 누른다. 응답을 놓치면 같은 요청과 원주문으로 재조회하거나 접수를 재시도한다. 최초 접수 시각과 원주문은 유지된다.

### O14와 O15 결제 증거 및 원장 인출

O14는 활성 v2 배포의 체인·계약·자산 프로필과 저장된 서명 견적, P07 확정 근거를 원주문 단위로 읽는다. O15는 별도 v1 등록 가맹점·테스트 자산·수취 주소와 적립금을 조회한다. 운영 배포는 v1 읽기 manifest를 재사용하고 `P05_V1_LEGACY_MERCHANTS`에서 지점별 가맹점 주소를 등록한다. `P05_V1_CASHOUT_KEYSTORE`와 `P05_V1_CASHOUT_SECRET_REF`는 대여 운영 키와 다른 전용 거래 서명자를 가리킨다. 서명자를 공유하는 슬롯에서 서명된 요청의 결과가 불명확하면 다음 거래는 대기한다. 등록 변경에 앞서 기존 후보를 대사하거나, 확인하는 동안 원래 승인된 v1 배포와 서명자 프로필을 보존한다. 활성 `legacy_cashout`으로 검토한 금액과 등록 수취 주소를 저장한다. 디스패치 전에는 최신 원장을 다시 대조한다. 서명 없이 남은 예약은 1분 뒤 다음 요청에서 해제한다. 방송 시도 기록이 없는 후보의 검토가 달라진 경우 새 검토가 가능해진다. 방송 여부가 불확실할 때 `legacy_reconcile` 담당자는 동일 요청의 서명 거래만 다시 대사한다. 확정 원장에서 해당 거래의 순번이 이미 사용됐지만 원거래의 수취 근거를 찾지 못하면 순번 만료와 수취 미확인으로 표시한다.

O14의 서명된 배포 검토는 P07과 같은 프로필 배열, 승인 공개키, 세대별 분리 서명을 각각 `P05_SETTLEMENT_DEPLOYMENT_PROFILES`, `P05_SETTLEMENT_DEPLOYMENT_APPROVAL_KEY`, `P05_SETTLEMENT_DEPLOYMENT_SIGNATURES`에 등록한다. [P07 서명 형식](../p07-indexer/chainview-v2.md#x03-profile-holds)과 같은 필드 및 도메인을 사용한다. 이 입력 중 하나라도 설정하면 세 입력을 대조한다. 실패한 후보의 결제 전달과 릴레이는 보류하고 별도의 지점 업무는 계속 제공한다. P07은 자기 서명 검증이 끝난 후보만 활성화할 수 있다. 서명 설정을 사용하지 않는 기존 프로필은 별도 표시하고 원복 효과의 증거를 확정하지 않는다.

원복 거래는 보관된 원릴레이 해시, 서명 견적과 구매 승인, 정규 블록의 거래·영수증 루트, P07 확정 블록을 대조한다. 이 거래의 실행 원복과 가스 부담을 다른 거래의 성공 또는 이후 만료와 분리한다. 서명·배포·체인 근거가 불충분한 경우 이 거래의 수취 변화와 주문 소비를 표시하지 않는다.


v1 거래는 금액·수취 주소를 calldata에 고정하지 않는다. 실제 확정 영수증의 수취 주소 및 금액이 원래 검토와 다르면 양쪽 값과 거래·블록 근거를 보관하고 대사 보류로 표시한다. 로컬 v1 체인 시험에는 검토 시 적립금 7, 실제 실행 전에 별도 유효 지급을 통해 2가 추가된 상황을 포함하며, 같은 후보의 실제 수취액 9와 수취 주소를 관측한다. 토큰 수취 보류와 이후 확정 수취는 별도로 검증한다. `TestPostgresLegacyCashoutDispatchFence`는 새 PostgreSQL에서 슬롯 만료, 방송 전 취소, 방송 미확정 중 잠금 유지, 권한 회수 및 독립 재개를 검사한다. `TestLegacyCashoutKeepsChangedExecutionDistinct`는 일치·불일치·미확정 결과와 증거 보존을 검사한다.

### 기존 데이터베이스 인코딩 확인과 전환

상용 TLS 서비스 시작 전에 대상 데이터베이스에서 `SHOW server_encoding`으로 `UTF8`을 확인한다. 로케일 `C`도 사용할 수 있다. 새 인스턴스는 같은 로케일을 쓸 경우에도 UTF8을 명시한다. SQL_ASCII에는 유효성을 확인하지 않은 바이트가 이미 있을 수 있어 설정만 이름 변경하거나 상용 쓰기를 열지 않는다. SQL_ASCII의 기존 읽기 도구는 그 데이터베이스의 익숙한 인코딩과 기존 권한 범위에서 당분간 접근한다. v1 기준 블록·계약 조회는 기존 배포/권한으로 따로 검증한다. 새 C3 쓰기 콘솔과 v1 인출 콘솔은 UTF8 업무 데이터베이스가 준비된 뒤 사용한다.

이전 시 기존 DB의 읽기 스냅샷과 동일 시점의 별도 보존용 dump를 보관한다. 다른 이름의 격리된 UTF8 DB에 dump를 복원하고, 읽기 전용으로 셀·키·참조 관계를 대조한다. 원본 인코딩을 확인할 수 없는 비ASCII 값은 담당자가 원본과 대조 후 변환한다. 문자열 손실이나 대체문자 입력을 허용하지 않는다. 필수 검산에는 지점·주문·원주문/환불 키, 명령 영수증·지문, 계정/역할, 화폐 최소 단위·확정 블록, 레거시 대여·인출 참조가 포함된다. UTF8 복원에서 정상 마이그레이션, 역할별 읽기, 한국어 이름 게시와 동일 영수증 재응답을 검증하고 결과와 스냅샷 경계를 기록한다.

최종 전환 때는 기존 쓰기를 중지하고 최종 스냅샷을 만든 뒤 복원·검산을 반복한다. 인코딩·복원·대사가 실패하면 새 쓰기를 열기 전에 기존 DB의 읽기 경로로 돌아간다. 새 DB가 쓰기를 수신한 경우 이전 원본에 다시 쓰기 시작하기 전 그 차분과 거래·영수증을 대사하고 담당자의 전환 결정을 기록한다.


### 실제 브라우저와 앱 통합 검증

O08은 빌드한 웹 `dist`와 [주소 변경 브라우저 검증 스크립트](test/finance/payout-browser.sh)의 절대 경로를 `NU54_PAYOUT_BROWSER_DIST`, `NU54_PAYOUT_BROWSER_SCRIPT`로 넘긴다. `PLAYWRIGHT_CLI`는 사용하는 Playwright CLI 실행 파일을 가리킨다. 전용 DB에 정상 P05와 P07 마이그레이션을 적용한 뒤 `test/settlement/anvil.sh -run '^TestLoopbackPayoutApprovalAndFinalizedRotation$' -v`를 실행한다. 두 Chrome 프로필이 다른 mTLS 인증서로 동일 제안을 열고, 제안자 승인을 거절하며, 별도 승인과 다른 지점 거절을 확인한다. 서버에서 첫 승인이 한 건인지 대조한 뒤 같은 테스트가 다음 승인·체인 등록·기존 견적과 신규 견적의 실제 수취를 검증한다.

iOS 카탈로그 확인은 독립 번들 `com.nu54kiosk.cataloglab`에서 진행한다. `NU54_IOS_BUNDLE=com.nu54kiosk.cataloglab CODE_SIGNING_ALLOWED=YES CODE_SIGN_IDENTITY=-`를 Release 시뮬레이터 빌드에 사용한다. [Native 시나리오 실행기](../../scripts/ios/scene-smoke/run-subscription-native.sh)의 `testCommerceEnrollmentPublicKey`로 얻은 테스트 앱의 공개키에만 합성 시험 CA로 단말 인증서를 발급한다. 신선한 전용 PostgreSQL에 P05 마이그레이션을 적용하고 로컬 opsd에 독립 단말 주체, 지점, `catalog.read`와 `terminal.state`를 연결한다. 운영자 mTLS 경로로 시험 메뉴 가격·재고·번역·알레르기 선언을 게시한다. 같은 실행기의 `testNativeCatalogReadsPublishedDeclarations`에 시험 프로필과 기존에 게시한 메뉴의 기대값을 넣어 실제 native mTLS를 검증한다. `SceneSmoke/KioskCatalogSmoke/testPublishedCatalogKeepsSelectionAcrossLanguages`는 같은 등록을 사용해 실제 메뉴에서 한글 옵션을 추가하고 영어로 바꾼 뒤 수량·알레르기 선언을 확인한다. 기존 앱의 등록을 사용하지 않는다. 동일 시험은 독립 P02 번들 `com.nu54renter.cataloglab` 및 별도로 발급한 고객 인증서로도 실행했다. 두 인증서를 같은 지점·같은 PostgreSQL과 연결한 통합 시험에서는 서로 다른 주문 두 건이 기존 버전의 7.00으로 기록됐고, 두 번째 주문으로 재고가 소진됐다. P04 재실행 시 이전 주문의 QR은 사라지고, 운영자가 8.50으로 새 버전을 게시한 후에도 P02는 저장했던 주문을 재조회해 7.00을 표시했다. RN 회귀는 제출 전 가격 변경 시 기존·새 합계와 변경 옵션을 표시하고, 사용자가 바뀐 장바구니를 확인할 때까지 주문 접수를 보류하는 것을 확인한다. 일부 옵션 재고만 부족하면 확보하지 못한 수량을 표시하고 나머지 품목과 수량을 유지한다. 이미 주문을 저장했거나 결과를 받지 못한 경우에는 같은 원요청 참조로 재개한다. 물리 기기의 키와 BLE 관측은 이 시험 범위 밖이다.

O12·O13 브라우저 시험은 일반 P05와 P07 마이그레이션을 적용한 별도 UTF8 `nu54_settlement_*` PostgreSQL에서 실행한다. 웹 빌드와 [자격 변경 브라우저 시험](test/finance/access-browser.sh)의 절대 경로를 각각 `NU54_ACCESS_BROWSER_DIST`와 `NU54_ACCESS_BROWSER_SCRIPT`에 지정한다. `PLAYWRIGHT_CLI`는 Playwright CLI 파일을 가리킨다. `P05_SETTLEMENT_TEST_DATABASE_URL`을 전용 DB에 연결하고 `go test -count=1 -v ./products/p05-operations-backoffice/internal/settlement -run '^TestPostgresOperatorAccessRequiresTwoIdentities$'`를 실행한다. 운영 웹에서 한국어 지점을 고른 뒤 별도의 인증서로 제안과 승인을 기록한다. 단말의 실제 C3 상태 요청이 등록을 확인하면 운영 화면에서 동일 ACK를 조회한다. 이후 같은 단말과 다른 운영자 자격을 철회하고 기존 인증서의 요청이 거절되는지 확인한다. PostgreSQL의 제안자·승인자·대상·요청 참조와 지점 변경 이력을 대조한다. 같은 실행에서 원주문 인계를 확인하려면 `NU54_ACCESS_HANDOFF_DIST`에 같은 웹 빌드를 지정한다. [인계 브라우저 시험](test/finance/access-handoff-browser.cjs)은 기본 `playwright` 모듈 또는 `NU54_PLAYWRIGHT_MODULE`로 지정한 모듈과 설치된 Chrome을 사용해 제안 취소·제안 기록·독립 승인·담당자 접수를 확인한다.

O15는 [v1 인출 브라우저 시험](test/finance/legacy-browser.sh)의 절대 경로를 `NU54_LEGACY_BROWSER_SCRIPT`, 웹 빌드 디렉터리를 `NU54_LEGACY_BROWSER_DIST`에 설정한다. 같은 전용 DB에서 `test/settlement/anvil.sh -run '^TestLoopbackManagedRentalFinalWithdrawal$' -v`로 실행한다. Chrome의 인출 권한자는 검토한 금액을 기존 v1 계약으로 인출하고, 별도 대사 인증서는 같은 확정 영수증을 조회해 기록한다. 테스트는 원래 검토 금액, 실제 수취 금액, 거래 해시가 같은 원장 행에 저장됐는지 검증한다. 상용 인출과 실물 기기 설치는 사용하지 않는다.

### O05·O06 부분 환불, 결재와 재원 제출

콘솔 환불은 지점별 세 권한을 사용한다. `refund_request`는 원결제 한도 안에서 원금을 예약한다. `refund_approve`는 신청자와 다른 계정으로 승인하거나 정해진 코드로 반려한다. 승인 기록이 확정되어도 콘솔 환불은 재원 운영자가 제출하기 전까지 전송 작업을 만들지 않는다. `refund_submit`은 신청자와 승인자 모두와 다른 계정에 부여한다. 기존 서비스 간 환불과 배포 전에 이미 예약된 전송은 원래 작업을 유지한다.

신청 응답이 유실되면 같은 원주문 범위에서 동일한 요청 키를 다시 조회한다. 확인된 경합 한도 응답은 해당 시도가 기록되지 않았음을 뜻한다. 화면은 원주문에서 최신 가능액을 읽은 뒤 금액을 수정하고 다시 검토하게 한다. 반려는 같은 환불·승인자·사유 코드에 대해 같은 결정을 돌려준다. 이미 확정한 반려 사유는 수정할 수 없다. 반려 코드와 시각, 기존 승인자 범위만 기록한다.

등록 대여 계정의 원환불은 해당 원결제 권한 세대의 인출 주소가 승인 배포의 확정 체인 상태와 등록 반환 경로에서 일치해야 진행한다. 화면에서 주소를 변경할 수 없다. 재원 담당자는 최근 체인 재원에서 아직 확정되지 않은 제출을 보수적으로 제외한 금액을 본다. 재원이 부족하거나 관측을 확인할 수 없으면 작업을 만들지 않는다. 제출 뒤에는 같은 환불 ID의 기존 전송과 P07 확정 증거를 조회한다. 확정 영수증은 원 매출 영업일과 환불 발생 영업일을 구분한다.

로컬 인수는 마이그레이션을 적용한 P05/P07 임시 PostgreSQL 데이터베이스, Foundry 1.5.1, Chrome, Playwright CLI 및 웹 빌드를 사용한다. 웹 산출물 디렉터리를 `NU54_FINANCE_BROWSER_DIST`에 지정하고 `NU54_FINANCE_BROWSER_SCRIPT`로 저장소의 `products/p05-operations-backoffice/test/settlement/refund-browser.sh` 절대 경로를 넘긴다. `P05_PILOT_HUMAN_UNITS=1`과 빈 `P05_PILOT_OUTPUT` 디렉터리를 지정한 뒤 기존 `test/settlement/anvil.sh`를 실행한다. 기존 보고서 브라우저와 이 환불 브라우저는 독립 회귀다.

### O07 공급 지점의 원예약과 청구 회차

운영 홈의 예약과 구독은 지점의 `experience_read` 권한으로 열린다. 예약 상세의 공급 지점 표시는 등록된 지점 이름을 사용하고, 이용 시각은 예약 당시의 공급 슬롯과 지점 시간대로 표시한다. 예약금과 잔금은 각 원주문, 지급 기한, 확정 영업일 및 남은 환불 가능액을 따로 표시한다. POS 결제 전용 단말 자격은 이 운영 조회에 사용할 수 없다.

구독 해지는 이미 발급한 원주문과 별개다. `experience_read`와 `experience_cancel`를 모두 보유하고 원장에 등록된 담당자가 같은 요청 키·검토 revision으로 해지를 재조회한다. 권한은 구독 행이 잠긴 뒤 다시 확인한다. 응답이 유실되면 운영자가 같은 지점과 구독을 다시 열어 보낸 요청 키와 검토 revision으로 조회를 재개한다. 해지는 이후 청구만 닫고 원 회차의 주문·상태를 계속 보여 준다.

로컬 O07 인수는 같은 임시 P05/P07 DB와 웹 산출물을 사용한다. `P05_PILOT_OUTPUT`와 `P05_PILOT_HUMAN_UNITS`를 지정하지 않은 상태에서 `NU54_EXPERIENCE_BROWSER_SCRIPT`에 저장소의 `products/p05-operations-backoffice/test/experience/browser.cjs` 절대 경로를, `NU54_PLAYWRIGHT_MODULE`에 로컬 Playwright 모듈 경로를 지정한 뒤 `test/settlement/anvil.sh`를 실행한다. 실제 두 예약 주문과 등록 운영자 및 별도 POS 단말 인증서를 사용한다.

041 이후 웹 요청과 mTLS 서비스 요청 모두 신규 직원 환불에 같은 분류 코드와 권한을 요구한다. 사유 없는 기존 신청은 기존 요청 키·신청자·원주문·금액이 모두 같을 때 동일 기록을 조회할 수 있다. 배포 시점에 이미 존재한 미결 승인 및 서명 작업은 그대로 처리한다. 예약 취소·청구 정책은 원결제별 환불 가능액을 정하며, 새 직원 신청을 구 경로로 보낼 권한을 부여하지 않는다.

금액과 수취인 확인을 마친 작업의 실제 실패 영수증은 거래 위치와 receipt root를 검증한 뒤 P07의 인가된 확정 블록과 대조한다. 미확정 또는 다른 분기의 실패는 같은 전송을 계속 조회한다. 확정 실패가 확인되면 원환불과 원주문을 유지하면서 실패 해시·nonce와 이전 승인 기록을 보관하고 검토 revision을 올린다. 새 revision에 대한 별도 승인과 재원 제출이 있어야 새 전송을 준비한다. 최종 영수증이 있는 환불은 다시 열 수 없다.

`NU54_REFUND_RETRY_TEST=1`을 지정하면 정규 Anvil 실행에 `FinalizedRefundRetry`를 포함한다. 이 시험은 표준 통합 게이트에서도 필수다. 부족 재원 Chrome 시나리오는 정상 환불 브라우저 명령에 `NU54_REFUND_LOW_TEST=1`을 추가한다. 종료 대여 경로는 `NU54_REFUND_RENTAL_TEST=1`과 `test/settlement/rental-refund-browser.cjs`를 사용하고 `NU54_PLAYWRIGHT_MODULE`을 지정한다. 예약 브라우저는 같은 일반 실행으로 혼합 지급, 만료 시점 재고 보류 및 P07 미지급 증거 이후 해제를 확인한다.

O06 지점 환불 대기열은 선택한 지점의 현재 등록 재무 권한으로 조회한다.
원주문·환불 참조, 환불 금액·원금, 상태와 현재 계정이 맡을 수 있는
업무를 표시한다. 신청자 본인의 승인 제한과 신청자·승인자의 재무
제출 제한을 구분한다. 페이지를 넘기거나 처음부터 다시 조회할 수
있으며 해당 행을 열면 같은 원주문으로 돌아가 권한과 원장을 다시 확인한다.

실제 세 역할의 Chrome 회귀에는 `NU54_REFUND_REVIEW_BROWSER`의 값으로
[환불 심사 브라우저 시험](test/settlement/refund-review-browser.cjs)의
절대 경로를 지정한다. `NU54_FINANCE_BROWSER_DIST`는 웹 빌드,
`NU54_PLAYWRIGHT_MODULE`은 로컬 Playwright 모듈, `P05_PILOT_HUMAN_UNITS=1`은
합성 자산 단위를 지정한다. 빈 합성 시험 출력 디렉터리를
`P05_PILOT_OUTPUT`에 지정한다. 정규 P05/P07 마이그레이션을 적용한
`P05_SETTLEMENT_TEST_DATABASE_URL`로 아래 시험을 실행한다.

```sh
test/settlement/anvil.sh -run '^TestLoopbackSettlementEvidenceAndRefund$/^FinalizedRefundRetry$' -v
```

응답을 보류하면 신청 폼과 제출 버튼을 잠근다. 커밋 응답이 유실된 뒤
다시 열면 같은 원주문·요청 참조로 복구한다. 실패 영수증 확정 전에는
재승인과 제출이 나타나지 않는다. 최종 실패 뒤에 별도 승인자와 별도
재무 담당자가 같은 환불을 검토·지급하며 PostgreSQL의 확정 건수와
원결제 수취인·금액을 대조한다.

### O04 · 통지와 영향 검토

조사 대기열에서 통지를 열어 영업일, 장부 반영 시각, 원결제 등록 주소
버전과 확정 체인 증거를 비교한다. 원주문, 통지, 수신 지점의 이행
범위와 기존 기록 유지 여부를 확인한 뒤 재처리를 명시적으로 요청한다.
검토 취소는 원장 변경 없이 목록으로 복귀한다. 읽기와 재처리는 지점의
서로 다른 권한이다. 재처리는 현재 권한을 다시 확인하며 이미 전달된
통지에는 새 요청을 만들지 않는다.

서버가 접수한 요청은 전달 여부와 별도로 같은 운영자의 조사 목록에
남는다. 목록의 조회 ID 또는 통지 ID·조회 ID 입력으로 해당 접수와
수신 영수증을 다시 확인할 수 있다. 다른 지점과 다른 직원의 접수 ID는
조회할 수 없다. 기존 eventId와 적용 식별자, 전송 작업의 fence를 유지한다.

O04와 O01의 Chrome 회귀는 `NU54_NOTIFICATION_BROWSER`에
[검증 스크립트](test/finance/notification-browser.cjs)의 절대 경로를
넘겨 정규 `test/settlement/anvil.sh -run '^TestLoopbackSettlementEvidenceAndRefund$' -v`
로 실행한다. 같은 문서의 환불 브라우저와 동일한 로컬 P05/P07 DB,
웹 산출물, Playwright 모듈, 합성 자산 단위와 전용 pilot 출력 디렉터리를
지정한다. 다른 금융 브라우저 스크립트 환경 변수는 설정하지 않는다.
이 회귀는 통지 검토 취소, 응답 유실과 다시 열기, 실제 수신 영수증,
검증된 마감 응답 유실 후 기존 키와 버전을 재확인한다.

### O03 · 지점별 미확정 게시

카탈로그 변경의 복구 기록은 서버가 검증한 인증서와 지점 범위별로
독립적으로 저장한다. 브라우저는 발급자 origin별 저장 영역을 사용한다.
이전 버전의 공용 저장 기록은 같은 범위에 돌아왔을 때 원래 요청 키와
내용 그대로 이전한다. 한 지점의 응답 여부를 확인하는 중에도 다른
지점은 별개 버전으로 게시할 수 있다.

`NU54_CATALOG_BROWSER`에
[두 지점 브라우저 시험](test/catalog/scopes-browser.cjs)의 절대 경로를
넘긴다. 로컬 Playwright 모듈 `NU54_PLAYWRIGHT_MODULE`과 웹 산출물
`NU54_FINANCE_BROWSER_DIST`를 지정한다. 전용 UTF8 PostgreSQL에 P05
마이그레이션을 적용하고 `P05_COMMERCE_TEST_DATABASE_URL`을 설정한 뒤
`go test -count=1 -v ./products/p05-operations-backoffice/internal/commerce -run '^TestPostgresOperatorCatalogRealTLS$'`
를 실행한다. 두 지점의 게시 버전, 응답이 유실된 첫 게시의 원 요청
재개 및 이전 가격의 확정 주문 보존을 확인한다.

### 대여 반납 브라우저 회귀

P05·P07 마이그레이션을 적용한 로컬 전용 `nu54_settlement_*` DB와
`P05_SETTLEMENT_TEST_DATABASE_URL`, 로컬 Chrome 및 Playwright를 준비한 뒤
`make -C products/p05-operations-backoffice rental-browser-test`를 실행한다.
Playwright의 별도 설치 경로는 기존 `NU54_PLAYWRIGHT_MODULE`로 지정할 수 있다.
이 시험은 기존 반납과 보호 세대의 미결 2·1·0건, 유효 기한 만료, 같은 지점의
v1 대사 왕복, 같은 세대 반납·인출을 실제 mTLS, PostgreSQL과 Anvil에서 확인한다.
보호 세대의 등록·증거 입력은 시험 fixture에서 구성한다.

### Order escrow claims and disputes

Finance operations `escrow-orders`, `escrow-claim`, `escrow-claims`, `escrow-resolve`, `escrow-resolve-approve` and `escrow-resolve-reject` follow section 7 of the [wire agreement](../../docs/content/commercial/architecture/spending-wallet-wire.md). Grant `escrow_claim`, `escrow_dispute_request` and `escrow_dispute_approve` per branch, the last two to different people. One claim holds up to 100 orders; orders already in an unfinished claim are skipped as `in-another-batch`. An order whose delivered mark came too late shows `deliveryFailed` and is returned to the buyer after its delivery deadline. An unfulfilled order whose refund the buyer's token account keeps refusing shows `cancelRefused`; the server tries again, at most hourly, until the buyer accepts transfers. Setup and accounts: [order escrow branches](settlement-v2.md#order-escrow-branches).

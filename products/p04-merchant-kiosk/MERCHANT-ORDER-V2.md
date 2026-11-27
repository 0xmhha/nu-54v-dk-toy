# 매장 주문과 단말 인계 · 단계 11

[구현 계획](../../docs/content/commercial/implementation-plan.md) · [결제 모델](../../docs/content/commercial/merchant-payments.md) · [공통 전송 계약](../../packages/protocol/v2/interfaces.json) · [P02 관리/복구 세션](../p02-user-app/MOBILE-V2.md)

| 작업 단위 | 권한과 상태 |
|---|---|
| 11A 매장 원장·모드 | P05의 인증된 tenant·지점·단말과 활성 직원 권한이 메뉴/결제전용 모드를 정한다. 메뉴 모드는 버전·SKU·수량을 보내고 결제전용은 POS 참조를 보낸다. 서버가 주문·가격·재고·USD 표시용 원장을 결정한다. 결제전용 금액도 P05가 검증한 POS 주문에서 가져온다. 모드 전환 뒤 기존 주문은 같은 권한으로 조회만 재개할 수 있다. 이전 직원의 권한 회수 이후에는 새 직원의 범위로 대사한다. |
| 11B 공용 화면·단말 inbox | 고객별 화면 세션과 권한 있는 재조회 핸들을 서버 주문과 함께 분리한다. 전환·취소·시간 만료 뒤 공개 화면에서 이전 주문 상세·주소·영수증을 지운다. 공개 화면을 오래 점유한 고객 A와 이후 고객 B가 독립적으로 진행할 수 있다. 단말 inbox는 안정적인 요청 ID와 마지막 확인 상태를 기록해 정전 이후 같은 원주문·견적·제출을 재개한다. 캐시에 남은 유효 주문이 있어도 새 단말 인증과 조회 범위를 다시 확인한다. |
| 11C 동적 QR·근거리 주문 | 출처 범위를 제한한 동적 QR의 공개 locator는 주문/지점/유효기간만 참조한다. 금액·권한·지급자·복구 증명·결제 자격을 담지 않는다. QR과 C1/C2에 함께 쓰이는 중앙 주문 참조는 P05에서 결정한다. 근거리 주문 채널은 C1 관리와 분리된 운영 자격, 독립 ALPN 및 상호 인증의 C2 nearby 프로필로 전달한다. 폰이 오프라인이어도 단말이 P05에 접근할 수 있으면 같은 주문을 처리한다. 단말도 오프라인이면 고객에게 미접수/대기를 표시하고 내구성 있는 요청 ID로 복귀 후 대조한다. |
| 11D 보드 직접·폰 경유 | 직접 승인에서는 C2 단말의 가맹점 권한, 폰 중계에서는 활성 지갑 세대와 고객 권한을 확인한다. 각 경로는 P05가 동일한 서버 주문·지급자·용도에 묶어 서명한 같은 바이트의 견적을 가져간다. 단말은 보드의 지급자 승인을 대신하지 않는다. 지점·금액·네트워크·세대·만료가 일치하고 지원되는 승인 프로필에서만 요청을 보드로 전달한다. v1 기존 대여를 v2로 전환하는 숨은 경로가 없다. |

## 실행 및 검증

```sh
make lint P=p02
make lint P=p04
pnpm --filter @nu54/protocol test
pnpm --filter @nu54/p02-renter test --runInBand
pnpm --filter @nu54/p04-kiosk test --runInBand
bash products/p02-user-app/android/test_commerce_tls.sh emulator-5558
```

- [K06 단말 모드 전환](../../docs/content/commercial/screens/book.html#K06)은 조회와 승인을 분리한다. 직원 앱이 C3 `terminal.state`에서 해당 지점 단말의 현재 revision과 기존 주문 범위를 읽고, 별도 승인에서 대상 단말·원본 revision·목표 모드·요청 키를 보호 저널에 저장한 뒤 `terminal.mode`를 보낸다. 저장 결과를 잃거나 앱을 다시 실행하면 같은 키와 본문으로 영수증을 재조회한다.
- P05의 047은 단말의 신규 주문 접수와 같은 등록 행을 잠근 뒤 기존 주문 ID를 지점 인계 목록에 묶고 신규 접수를 막는다. 직원의 `terminal.handoff`와 등록 단말의 `terminal.ack`는 원요청의 키와 다음 revision을 확인한다. 단말 앱은 적용할 버전을 보호 저널에서 확인한 뒤 ACK를 보낸다. 양쪽 증거가 저장되면 P05가 결제 전용을 활성화한다. 기존 006 영수증과 메뉴 복귀 경로는 조회·동시에 진행 중인 전환이 없는 경우에 사용한다.
- 010의 기존 주문 예약 영수증, QR 비교, 비공개 주문 조회와 035의 등록 POS 인계 참조는 원래 주문을 이어서 사용한다. 인계 목록에는 종전 주문 ID가 남으며 각 고객의 지급 자격은 원 승인 범위로 계속 검증한다.
- P04 공용 단말, 직원 전용 설정과 재무 진입은 개별 인증서 등록으로 운영한다. 등록이 둘 이상이면 앱에서 권한 혼용을 차단한다. 등록서의 서버 핀은 별도 공급·설치 기록과 대조한다. C3 설정은 `endpoint`, `ca`, `pin`, `certificates`, 인증서의 주체와 일치하는 `scope`를 받는다. 단말의 선택적 `nearby` 및 `boardTransport`는 해당 연결 전용 공개 등록서다.
- P02는 OS에서 전달한 `nu54://order` 주소를 받는다. 카메라 또는 스캐너 사용이 어려우면 QR 내용을 입력할 수 있다. QR만으로 주문 상세를 조회할 수 없다. HTTPS와 근거리 모두 별도 고객 확인 번호를 단말과 함께 대조한 뒤 권한 있는 고객만 조회 가능하다. 화면을 비우거나 연결이 만료되면 공개 QR과 확인 번호는 폐기된다. 단말이 접수 응답을 받기 전 저장한 주문 요청은 동일한 고객과 요청 키로만 재개된다.
- 지급 전 P05가 인증된 주문 조회에 반환한 주문 키와 견적값을 별도 견적 발급 응답 및 실제 상점 서명 bytes와 대조한다. 같은 상점·금액·지급자가 들어간 다른 주문의 유효 견적도 거절한다. 승인 이후에는 원래 요청 키와 검증한 서명을 다시 사용하고 P05의 주문 상태를 조회한다. 지연되거나 결과가 없는 경우 지급 확정·픽업 발급으로 표시하지 않는다.
- 자동 시험은 두 고객이 서로 다른 TLS 범위에서 공개 화면과 무관하게 주문할 수 있는지, 두 단말이 오프라인일 때 동일한 요청을 유지하는지, 이전 공개 화면의 응답·확인 코드가 새 방문객에게 노출되지 않는지 검증한다. 실제 조리·프린터 송신과 수동 재인쇄는 원장 이벤트와 별도 복구 경로가 확인되어야 진행한다. 물리 보드·폰과 카메라 모델별 인수는 별도 증거를 기록한다.

## 직원 권한 확인과 전환 재현

K12는 설치된 인증서로 P05 C3 `staff.access`를 호출합니다. P05가 테넌트·지점·주체의 활성 권한을 확인하면 해당 업무를 엽니다.

| 등록 자격 | 열리는 업무 |
|---|---|
| 직원 인증서와 지점 `pickup_manage` | 같은 지점의 픽업 대기열과 이행 진행 |
| 직원 인증서와 활성 O13 `operator` 지점 권한 | K06 단말 설정 |
| 재무 인증서와 활성 `refund_request`, `refund_approve` 또는 `refund_submit` | P05 운영 콘솔 진입. 콘솔에서 별도 재인증 |
| 전용 주방 단말 인증서와 `travel_pickup_terminal` 배정 | 기존 지점 픽업 대기열 |

점원 배정에는 운영자 등록이나 재무 권한을 겸용하지 않습니다. 구체적인 지점 픽업 권한은 P05의 `pickup_manage` 배정으로 관리합니다. 인증을 취소하거나 고객 화면으로 돌아가면 이전 세션의 늦은 응답을 폐기하고 화면의 운영 정보를 지웁니다. 업무 중에는 P05가 각 작업의 권한을 다시 검사합니다. 직원 화면에서 원래 요청 키로 대사해야 하는 주문·모드 변경 의도는 보호 저널에 보관합니다.

미확정 픽업 요청은 등록 서버와 핀, 테넌트, 지점, 담당자, 등록 역할, 설치 공개키에 묶어 보호 저장소에 기록합니다. 같은 설치에 교대 담당자를 다시 등록해도 기존 요청은 원래 등록 범위에 남습니다. 네이티브 전송은 요청에 묶은 등록 스냅샷과 현재 사용하려는 등록이 일치할 때만 해당 인증서로 mTLS를 시작합니다. 기존 v1 기록은 지점과 담당자가 일치하는 인증 범위에서만 처음 복구하며, 원본을 보존하고 해당 범위에서 완료한 기록을 다시 가져오지 않습니다.

K06의 인계 검토와 저장된 인계 목록은 20개씩 읽습니다. 다음 페이지는 지점·단말·원본 revision에 묶이며, 저장된 인계에서는 같은 전환 요청에 속하는 주문의 커서를 사용합니다. 공개 단말과 다른 지점 직원에는 인계 목록을 돌려주지 않습니다.

iOS에서는 별도의 테스트 시뮬레이터에 Release `com.nu54kiosk.k06terminal`과 `com.nu54kiosk.k06operator`를 각각 설치합니다. [네이티브 호스트 실행기](../../scripts/ios/scene-smoke/run-subscription-native.sh)의 `testCommerceEnrollmentPublicKey`를 각 슬롯에서 실행해 공개키 DER를 확보합니다. [전용 fixture](../../scripts/ios/scene-smoke/fixtures/p05_kiosk_mode_native.sh)에 임시 경로와 두 공개키를 넣으면 루프백 P05와 21개 합성 원주문을 시작합니다. 출력 `ready`가 가리키는 디렉터리의 `terminal.json` 및 `operator.json`을 Base64로 `NU54_SUBSCRIPTION_UI_FIXTURE`에 넘기고 역할별 슬롯의 `testNativeKioskStaffEnrollment`를 실행합니다. 각 앱의 역할은 `NU54_SUBSCRIPTION_COMMERCE_SLOT`으로 선택합니다. [K06 설치 시나리오](../../scripts/ios/scene-smoke/run-kiosk-mode.sh)에 두 시뮬레이터 UUID와 결과 경로를 넘겨 영향 검토, 원요청 복구, 두 게이트와 21개 원주문을 확인합니다. 같은 직원 앱을 `clerk.json`으로 다시 등록하면 `SceneSmoke/KioskClerkAuthorizationSmoke`에서 픽업 전용 접근과 고객 화면 복귀를 확인할 수 있습니다.

## 단말과 휴대전화의 근거리 전송

- 공통 Nearby GATT service는 `41ba05a0-e0dd-4fd7-901c-4c25d5020000`, write는 `…0001`, notify는 `…0002`다. 단말 peripheral은 공통 서비스만 광고한다. 공개 주문, 지급자, 사용자 신원, 세션 자격과 비밀은 광고하지 않는다. OS가 주는 BLE 주소와 peripheral ID를 광고·응답·로그·다른 사용자 화면에서 재사용하지 않는다. 앱은 스캔 실행마다 새 임시 목록 핸들을 발급하고 스캔 종료·선택 취소 때 폐기한다. RSSI는 표시 및 신뢰에 사용하지 않는다.
- 앱은 23–517의 ATT MTU를 받고 한 번에 `min(MTU-3, 244)` 바이트까지 쓴다. GATT transport는 분할된 TLS 레코드 바이트만 운반하고 TLS 1.3의 상호 인증, `nu54-v2-nearby` ALPN, 16바이트 `EXPORTER-NU54-wire2`를 검증한다. P02는 등록된 branch의 단말 trust pin을 확인하고, P04는 등록·철회된 고객 인증서를 배제한 별도 권한 구성에서 TLS peer 지문에 맞는 branch/customer UUID를 얻는다. PHY, 광고, QR, 클라이언트 JSON은 권한을 제공하지 않는다.
- 각 연결은 자체 TLS 엔진과 SDK `V2Link('nearby', role, epoch, peer, journal)`을 사용한다. role은 terminal/customer, 명령은 `nearby.submit`/reply이며 frame은 SDK의 2바이트 길이 prefix로 구분한다. 보드 지급은 별도 C2 payment ALPN과 보드 핀을 사용한다. 동시 연결·종료·만료의 큐/키는 연결별이며 다음 방문객에게 전달되지 않는다.
- Nearby JSON 요청 본문에는 버전 2, 주문 동작, 서버 branch, 전용 요청/재시도 키와 입력 필드만 포함한다. 고객 UUID는 TLS peer 등록에서만 정한다. 단말은 P05 C3 명령으로 전달하고, 고객이 소유한 현재 주문에 대해서만 결과를 돌려준다. 요청자가 전달한 주문 locator만으로 액세스할 수 없다. 단말도 오프라인이면 `pending`을 돌려주고 묶인 durable intent를 재개한다. GATT 종료 시 이미 완료된 P05 결제·정산을 취소하거나 새 주문을 만들지 않는다.

### Native transport entry points

P02 and P04 expose `Nearby` through their native React Native module provider.
`publicIdentity()` returns the public P-256 enrollment key. `configure()`
loads the independently issued CA and local certificate chain along with the
branch and authorized peer pins. Each configured peer binds one pin to an
enrolled principal, branch and expiry. On Android the private key stays in
Android Keystore. On iOS it stays in Secure Enclave.

P04 opens advertising with `start()`. P02 uses `scan()` and chooses a
single-use candidate through `connect()`. Both receive a new link handle
and a native `ready` event after mutual authentication. The event carries
the authenticated pin, principal, branch, expiry and exporter epoch.
`write(link, clear)` and the `data` event carry only the local plaintext
for that authenticated link. BLE carries the encrypted TLS stream. Close
the individual visitor link when the interaction ends and use a fresh TLS
session when resuming a durable order. A QR transition retires the nearby
link before it continues with the same server-owned order.

P04's board client has a separate payment identity and entry-point namespace.
It binds to the enrolled board GATT service `41ba05a0-e0dd-4fd7-901c-4c25d5010000`
and prepends route byte 2 to the first write. It checks the board SAN and
pin and the payment ALPN, using its own exporter and link handle. Nearby
customer authorities cannot enter this profile. Real board approvals
also depend on the board's installed provisioning and local approval
profile.

휴대전화가 온라인이면 등록한 P05 서버와 직접 상호 인증한다. 오프라인 근거리 경로에서는 등록한 매장 단말이 P05 요청을 대행한다. 이 경로의 신뢰 경계에는 해당 단말이 포함된다. P05와 휴대전화 사이에서 단말을 우회하는 별도 종단 인증은 제공하지 않는다. 실제 운영 단말 신뢰와 별도 표시 장치의 승인 증거는 파일럿 배포 프로필에서 확인해야 한다.

### 11E 비공개 주문 제시 증명

P05는 `P05_PRESENTATION_KEY_FILE`의 별도 Ed25519 32바이트 seed로 사설 주문 제시 증명을 서명한다. 앱은 발급서의 `payment.presentationKey`에 있는 32바이트 공개키 지문을 별도 공급 기록과 대조한다. 주문, 지급, TLS, 복구 또는 표시 장치의 키를 이 서명으로 재사용하지 않는다.

서명 원문은 ASCII `NU54-private-presentation-v1`과 NUL, 지점·원주문·고객·원본 참조 각 UUID 16바이트, claim·주문 키·견적 digest 각 32바이트, 지급자 세대 및 만료 각 uint64 big-endian 순서다. P05는 원본 주문에 저장된 origin과 활성 claim을 사용한다. 해당 문맥은 인증된 C3 또는 인증된 근거리 전송으로만 운반한다. 광고와 공개 QR에는 기존 locator 외의 고객·claim·issuer 서명 정보가 들어가지 않는다. 공개 원장에도 새 필드를 추가하지 않는다.

P02는 스캔한 주문 또는 암호화한 장바구니의 원본 참조와 등록 고객을, P04는 현재 공개 슬롯의 원주문·장바구니 원본 참조와 확인한 고객을 기대값으로 사용한다. 사용자가 검토한 기대값은 재시작용 저널에도 기록한다. 근거리 릴레이가 유효한 서명 견적과 비공개 projection을 한꺼번에 바꿔도 원래 기대값에는 맞지 않는다. 보드의 독립 표시기는 별도로 확인한 주문 또는 원본 참조와 신뢰키로 같은 증명을 검사하고, 그 뒤에 기존 N54D 기기 챌린지에 서명할 수 있다. 운영 시 이 표시기는 별도로 인증된 비공개 P05 자료가 필요하다.

공개 결제 QR의 유효기간은 최대 60초다. 현재 v2 locator의 지점 참조는 원장의 지점 UUID와 같은 값이며 고객·기기 식별자, 권한 증명 및 비공개 주문 상태를 포함하지 않는다. 공개 안내용 지점 식별자와 원장 지점 식별자를 분리할 때는 새 locator를 단기 서버 매핑으로 해석하도록 단계적으로 전환한다. 기존 QR만으로 원장 조회나 결제 승인이 부여되지는 않는다.

The installed mobile SDK and separate virtual lab are documented in the
[mobile SDK README](../../packages/protocol/platform/README.md). The native
builds exercise the Android and iOS link paths. The virtual lab exercises
Android Keystore and Apple Network/Security TLS using bridged fragmentation
and the P01 C approval implementation. Physical radio, OS permission prompts
and independent board confirmation are separate acceptance steps.

### 인증된 외부 POS 인계

결제 전용 단말은 메뉴·수량·가격을 보내지 않는다. 독립 등록한 POS 서비스는 기존 mTLS `POST /v2/commerce`에 단일 `pos` envelope로 외부 원주문 참조와 최소 단위 정수 금액을 제출한다. 배포는 StableNet 테스트넷 8283과 해당 지점에 등록된 TestUSDC 계약을 고정한다. P05는 POS 인증서의 `role: pos`, 동일 지점의 `POS/submit` grant, 원장에 등록된 지점·POS·수신 단말을 함께 확인한다. P05 035가 POS 요청과 원주문을 연결한다.

```json
{"pos":{"operation":"submit","branch":"<등록 지점 UUID>","reference":"S-0142","chainId":8283,"asset":"<등록 TestUSDC 주소>","amount":"6500000"}}
```

응답의 임의 UUID `handoff`를 등록 수신 단말에 전달한다. 사람이 볼 원주문 번호만으로는 공용 단말에서 주문을 열 수 없다. 단말이 `{"pos":{"operation":"lookup","branch":"<지점 UUID>","handoff":"<인계 UUID>"}}`로 조회한 결과로 K04의 상점·원주문·금액·네트워크를 표시한다. 다른 지점·단말의 코드는 거절한다. POS는 자기 `reference`로 `operation: status`를 호출하여 같은 원주문과 확정 영수증을 확인한다. 잘못된 자산/망은 `POS_ASSET_UNSUPPORTED`, 이전 참조를 다른 금액으로 재사용하면 `POS_REFERENCE_CONFLICT`로 끝난다.

동일 지점·외부 참조의 재전송은 같은 인계 코드와 업무 주문을 반환한다. 미리보기 QR에는 기존 단기 공개 locator만 담는다. K11에서 확인 번호를 대조해 P05가 선택한 지급자를 한 번 기록한다. 선택지와 QR은 결합 확인 뒤 화면에서 지운다. 선택된 폰과 보드의 승인 및 확정 증거가 도착하기 전에는 지급을 완료로 표시하지 않는다. 앱 종료·공용 화면 초기화 뒤에는 인계 코드를 다시 제시하여 같은 원장·영수증을 조회한다. 이전 공용 화면에 원주문 또는 영수증을 복원하지 않는다. POS 원주문은 메뉴 재고나 메뉴 가격 게시와 독립적으로 처리한다.

#### K04·K11 개발 환경 인수

새 `nu54_settlement_*` PostgreSQL 데이터베이스에 P05·P07 마이그레이션을 적용하고 `P05_SETTLEMENT_TEST_DATABASE_URL`에 연결 문자열을 지정한다. 로컬 Foundry 도구를 PATH에 두고 아래를 실행한다.

```sh
P05_PILOT_HUMAN_UNITS=1 P05_PILOT_OUTPUT="$(mktemp -d)" \
  sh products/p05-operations-backoffice/test/settlement/anvil.sh \
  -run '^TestLoopbackSettlementEvidenceAndRefund$' -v
```

시험은 실제 `opsd`와 P07을 실행한다. POS와 수신 단말을 별도 인증서로 등록하고 잘못된 자산·다른 지점·권한 없는 발급 주체·재전송 금액 변경을 거절하는지 확인한다. 정상 재전송과 소켓 재연결은 같은 원주문·영수증을 돌려준다. 두 고객 중 하나만 결합되고 원장에 주문·결합·견적·지급·이행이 각각 한 건인지 검증한다.

Android 확인은 테스트용 앱에서 새 등록 공개키를 만들고, 비공개 실험 디렉터리의 `terminal-spki.der`에 공개키 DER를 적은 뒤 `NU54_POS_NATIVE_LAB`으로 그 디렉터리를 지정해 같은 시험을 실행한다. 생성된 단말 발급서와 독립 서버 지문을 앱에 등록한다. `orders.json`의 유일한 인계 코드로 원주문 S-0142, 6.50 TestUSDC와 결합 전 QR을 확인한다. `preview-open` 신호를 주면 시험이 고객 둘의 확인 번호를 발급한다. 앱에서 첫 고객을 선택하고 QR이 사라진 뒤 `payer-bound` 신호를 준다. 시험은 바로 그 원주문에 견적을 발급하고 P01 호스트 보드 승인과 P04 합성 보드 전송 검증을 거쳐 Anvil에 지급을 확정한다. `receipt.json`이 생기면 열린 화면의 POS 원주문 다시 조회 버튼으로 영수증을 확인한다. 앱 프로세스를 종료하고 다시 시작한 뒤 최초 인계 코드로 같은 원주문과 영수증이 열리면 `ui-completed`로 확인을 마친다. iOS 실행과 실제 BLE 전송은 이 시험 범위 밖이다.

저장소 루트에서 Android UI 인수도 자동으로 실행할 수 있다. 위와 같이 준비한 데이터베이스, Android SDK 34 및 빌드 도구, JDK, `adb`가 필요하다. 초기 상태의 테스트용 키오스크 앱을 별도 Android 에뮬레이터에 설치하고 그 에뮬레이터의 serial을 `POS_TEST_SERIAL`에 지정한다. 테스트 패키지 ID를 바꿔 빌드했다면 각 `check.py` 실행에 같은 `--package` 값을 전달한다.

```sh
pos_ui=products/p04-merchant-kiosk/test/android-pos
pos_lab="$(mktemp -d)"
sh "$pos_ui/install-input.sh" "$POS_TEST_SERIAL"
python3 "$pos_ui/check.py" "$pos_lab" --serial "$POS_TEST_SERIAL" --prepare
NU54_POS_NATIVE_LAB="$pos_lab" P05_PILOT_HUMAN_UNITS=1 P05_PILOT_OUTPUT="$(mktemp -d)" \
  sh products/p05-operations-backoffice/test/settlement/anvil.sh \
  -run '^TestLoopbackSettlementEvidenceAndRefund$' -v >"$pos_lab/anvil.log" 2>&1 &
pos_server=$!
python3 "$pos_ui/check.py" "$pos_lab" --serial "$POS_TEST_SERIAL"
wait "$pos_server"
```

서버가 테스트 견적의 발급·만료 시각 안에 다음 Anvil 블록을 두고, 실제 포함 영수증과 P07 확정 이후 원장에서 지급을 검사한다. 테스트 도구에는 이 로컬 시험이 만든 합성 등록 정보만 입력한다. UI 계층은 에뮬레이터 공유 디스크에 기록하지 않고, 합성 화면 네 장만 실험 디렉터리에 남긴다. 종료 또는 중단 시 테스트 앱의 등록과 시험 포트 연결을 해제한다. 완료하면 `terminal-profile.json`, `terminal-spki.der`, `orders.json`, `pairs.json`, `receipt.json` 및 에뮬레이터에 저장한 테스트 등록을 삭제한다.

### K05 직원 영수증과 출력

같은 P05 origin에 등록된 직원 단말에서 K12 인증을 통과한 뒤
**직원 영수증과 출력**을 연다. 직원 단말이 보유한 operator 인증서는
P05에서 같은 직원의 별도 `printer_staff` 인증서 등록과 지점별
`ledger.print_staff_grant`를 모두 갖춰야 한다. 공용·고객 역할에는
출력 진입점이 없다. 직원 픽업 상세에서 같은 원주문 영수증을 바로
열거나 직원 목록에서 픽업 번호를 고른다. 목록은 확정 이행 수신으로
출력 시도가 등록된 기존 영수증을 원주문과 함께 보여 준다.

출력 결과가 미확정이면 실제 종이를 확인한 뒤 기존 출력 완료를
확인하거나 사본을 요청한다. 두 동작 모두 확인 화면에서 취소할 수
있다. 출력 대기 중이나 자동 조회에서는 새 시도를 만들지 않는다.
사본은 새 출력 시도 참조에 원영수증과 직전 시도를 결합한다. 확정된
기존 출력은 이력에 남는다. 결과를 받지 못한 경우 같은 등록 직원과
지점에서 앱을 다시 열고 **같은 출력 요청 다시 확인**을 누른다.
이미 처리된 요청은 같은 시도를 반환한다. 화면 잠금, 교대 또는
백그라운드 진입은 화면에서 영수증과 응답을 지운다.

제품 회귀는 `test/ReceiptPrint.test.tsx`와 P05의
`test/settlement/anvil.sh -run '^TestLoopbackSettlementEvidenceAndRefund$' -v`
를 사용한다. 후자는 격리된 PostgreSQL, P07, Anvil 및 응답 유실을
모사하는 격리 프린터를 사용한다. iOS 설치판을 결합하려면 일회용
`NU54_PRINT_NATIVE_LAB`에 테스트 단말의 P-256 공개키 맵
`public-keys.json`을 놓는다. P05 테스트가 그 디렉터리에 일회용
직원 등록서를 제공한다. 검증 후 `done`을 기록하면 실제 원장의
기존 조회 ID와 원영수증, 새 시도 한 건을 대조한다. 이 회귀는 실물
프린터에서 종이가 출력되었는지 확인하는 절차와 별개다.

### K05 원주문 준비와 수령

지점의 `pickup_manage` 권한을 받은 개별 직원은 매장 메뉴와
근거리 주문의 동일 픽업 대기열을 조회한다. 지급이 확인 중이거나
확정 지급이 아직 지점의 이행 수신으로 이어지지 않은 주문은
조회만 가능하다. 접수에 사용한 같은 원주문을 열 수 있으며, 이 단계에
픽업 번호나 조리·수령 동작을 표시하지 않는다. 출고 시스템이 승인된
통지를 수신하면 품목, 픽업 번호와 이행 ID를 확인할 수 있다.

준비 시작, 준비 완료, 수령 완료는 같은 기존 픽업 행에 귀속된다.
응답이 유실된 경우 직원은 같은 원주문과 요청 키로 결과를 재확인한다.
완료한 주문도 원주문 식별값으로 다시 열 수 있다. 구매자가 W11에서
기존 주문을 다시 조회하면 같은 픽업 번호의 단계가 갱신된다. 지급
해시·원영수증과 원장, 이행 이벤트는 별도로 다시 생성되지 않는다.
여행 예약 시간과 여행의 환불 연결은 기존 데이터에 남는다. 결제 전용
POS와 구독 청구는 카페 조리 대기열에 포함되지 않는다.

정규 P05/P07 마이그레이션을 적용한 로컬 시험 DB에서
`test/settlement/anvil.sh -run '^TestLoopbackCustomerOrderReceipt$' -v`
를 실행한다. 등록된 직원과 구매자, 다른 구매자 및 공용 단말을
분리하고 P07에서 확정한 두 원지급과 실제 이행 수신을 대조한다.

여행 픽업 전용 단말은 일반 단말 등록과 별도의 지점 지정이 있어야
한다. 서버는 픽업 대기열·상세·변경과 직원 권한 조회에서 두 권한을
확인한다. 조회가 처리 중일 때 지정이 철회되면 이전 결과는 전달하지
않는다. 다시 지정하면 같은 단말에서 픽업 업무로 복귀할 수 있다.

K05와 W11의 iOS 설치 연동은 서로 다른 테스트 시뮬레이터와
`com.nu54kiosk.cafepickupqa`, `com.nu54renter.cafepickupqa` Release
빌드를 사용한다. [설치 연동 도우미](../p05-operations-backoffice/test/pickup-native/run-cafe-ios.sh)의
`prepare`에 일회용 디렉터리, 직원과 구매자 시뮬레이터 UUID, 두 앱
경로를 순서대로 넘긴다. 정상 로컬 P05/P07 마이그레이션 후 같은
디렉터리를 `NU54_CAFE_NATIVE_LAB`에 지정해
`test/settlement/anvil.sh -run '^TestLoopbackCustomerOrderReceipt$' -v`를
실행한다. 시험 원장이 실행 중일 때 같은 도우미를 `verify`로
호출한다. 준비 잠금, 지급 확정과 실제 이행 수신, 직원 준비·전달,
구매자 앱 재시작 뒤 동일 영수증을 검증한다.

기존 단말의 픽업 조회는 여행 주문과 종전 응답 형식을 유지한다.
업데이트된 직원 앱은 같은 지점 권한을 확인한 뒤 카페 조회를 명시적으로
요청한다. 결제·이행 확인 대기는 직원 화면에서 볼 수 있다. 구매자는
준비 전에도 기존 지급 영수증과 재조회 안내를 볼 수 있으며, 실제
준비가 시작되면 기존 규약의 준비·수령 상태가 표시된다.

### K08 메뉴 원주문의 지급 확정과 미지급

가격 변경을 확인한 고객은 새 합계를 승인한 뒤 주문을 접수합니다. 단말은 현재 연결한 원주문을 C3에서 다시 조회합니다. P05 063은 원 지점의 원 단말이 유효한 권한으로 조회할 때 한해, 같은 원주문의 정산 지급과 릴레이 대상 트랜잭션 및 픽업 영수증이 확인되면 `relay-paid`를 추가합니다. 동일한 원견적의 만료 및 P07 미지급 확인은 `expired-unpaid`로 표시합니다. 이 값이 없는 경우에는 기존 상태에 맞는 안내를 사용합니다.

공용 화면에는 현재 주문의 확인 결과, 확정 금액 및 지급 완료의 픽업 번호를 표시합니다. 원영수증 참조는 담당 단말의 인증된 주문 응답에서 대조합니다. 주문 QR 및 휴대전화 확인 번호는 최종 상태에서 제거합니다. 화면 초기화와 앱 재시작 후 새 고객에게 직전 주문 상태를 복원하지 않습니다. 근거리 고객과 주방에는 각 경로의 표시와 처리에 필요한 허용 필드만 보냅니다.

[설치 검증 실행기](../../scripts/ios/scene-smoke/run-kiosk-outcome.sh)에 부팅된 전용 iOS 시뮬레이터 UUID, `NU54_IOS_BUNDLE=com.nu54kiosk.outcomeqa`로 빌드한 Release `NU54Kiosk.app`, 새 결과 디렉터리를 순서대로 줍니다. 실행기는 로컬 PostgreSQL·P05·P07·Anvil 8283과 P01 승인 호스트를 사용합니다. 하나의 원주문에서 가맹점 부담 7.00 TestUSDC 지급·확정 영수증을 확인하고, 다른 서명 원견적에서 확정 미지급을 검증합니다. 두 원주문의 초기 화면 및 재시작 뒤 개인정보 제거와 원장 건수도 검사합니다.

K08의 공개 연결 만료는 위의 서명 견적 미지급 확정과 별도의 상태입니다. 폰을 결합하지 않은 메뉴 주문의 공개 연결이 끝나면 주문·금액·QR·표식을 지우고 자동 초기화 대기를 안내합니다. 진행 중이던 공개 연결의 뒤늦은 응답으로 상세 화면을 복원하지 않습니다. 1분 무조작 뒤 또는 고객이 화면을 닫을 때 새 고객용으로 돌아갑니다. 이미 결합한 폰은 단말과 별개로 자기 원주문과 유효한 서명 견적을 계속 조회할 수 있습니다. 공개 초기화는 P05 명령을 보내거나 주문·견적의 유효기간을 바꾸지 않습니다.

설치 실행기는 이 두 공개 세션에서 앱의 실제 타이머를 기다립니다. 미결합 원주문에는 견적이나 지급이 생기지 않고, 이미 견적을 받은 고객의 조회는 같은 주문·revision·견적 digest에 머뭅니다. 고객이 이어서 승인하고 같은 제출을 재전송하면 결제와 영수증이 각각 한 건으로 남습니다.

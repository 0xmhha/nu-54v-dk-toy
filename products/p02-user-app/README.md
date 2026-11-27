# P02 · User app

> **DF-20260925-02 기준 (2026-09-28 개정):** 이번 사이클에 대여자 폰 앱으로 만든다. 기기 설정(본딩)과 결제 확인 화면을 맡는다. 범위와 설계는 [기획](../../docs/content/products/p02/plan.md) · [SRS](../../docs/content/products/p02/srs.md) · [유즈케이스](../../docs/content/products/p02/use-cases.md) · [설계](../../docs/content/products/p02/design.md)가 정한다. 아래 원문은 DF-20260920-01 기준이며 충돌하면 위 문서를 따른다. 원문의 소셜 로그인, 두 지갑, 송금, FOTA는 이번 사이클 범위가 아니다.

여행자와 점주가 기기와 지갑 기능을 사용하는 React Native 모바일 앱이다.

- **소유 범위:** Google/Apple 로그인, 계정 수명주기, 기기 등록·설정,
  두 지갑과 자산 선택, 승인·송금·영수증 UI, FOTA 진행 상태
- **주요 경계:** P01과 C1, P03과 C4, P10과 C3, P08/P09와 C8
- **WBS:** `WBS-P02-01`부터 `WBS-P02-04`

설계 진입점:

- [제품 WBS](../../docs/content/planning/product-wbs-overview.md#p02--유저-앱소셜-로그인기기-설정)
- [소셜 로그인과 두 지갑](../../docs/content/specifications/social-wallet-recovery-design.md)
- [화면 흐름](../../docs/content/specifications/screen-flows.md)
- [기기 등록·반납 수용](../../docs/content/specifications/lifecycle-journey-acceptance.md)

## 개발 (DF-20260925-02)

React Native 0.87(TypeScript)과 Kotlin Turbo Module로 만든 Android 앱이다. 키오스크와 같은 구조이며 프로토콜 코드는 `@nu54/protocol`을 같이 쓴다.

| 경로 | 내용 |
|---|---|
| `src/App.tsx` | 설정 상태 머신의 화면(기기 찾기, 본딩, 지갑 설정, 페어링 코드, 지갑 확인)과 홈(지갑 주소, 결제 모드, 확인 화면, 결과와 영수증, 등록된 기기) |
| `src/setup/` | `flow.ts` 기기·지갑 설정 상태 머신(P02-FR-09~12): 스캔 → 본딩 → `device.info` → 키 없는 기기면 `setup.operator`·버튼 승인·버튼 PIN → 페어링 코드 → passkey 재본딩 → `wallet.check` 서명 확인 → 홈. 등록 기기가 있으면 다시 연결해 지갑 주소를 확인한다. `channel.ts` 메시지 대기와 시간 제한, `network.ts` 배포 기록의 운영자·컨트랙트·chainId |
| `src/confirm/display.ts` | 확인 화면 문자열: EIP-55 주소, 소수점 둘째 자리까지 버린 금액(N31), 토큰 표에 없는 토큰은 base unit과 주소 |
| `src/link/confirmLink.ts` | 본딩한 링크에서만 `confirm.show`와 전달받은 `payment.outcome`을 받아 화면 상태를 바꾼다(P02-FR-02). 결제 모드 켜기·끄기(`device.paymentMode`, 기본 120초, P02-FR-08)를 보내고 기기의 답을 받는다 |
| `src/App.tsx`의 `Receipt` | 승인된 `payment.outcome`에 실린 디지털 영수증(가맹점, 대표자, 주문 번호, 메뉴·수량·가격, 합계)과 탐색기 링크. 링크는 받은 URL이 아니라 `chainId`와 `txHash`로 앱이 만든다. 형식이 틀린 영수증은 버리고 결과만 보인다 |
| `src/qr.ts` | 운영자 도구가 인쇄한 라벨 QR `nu54://bond?addr=<BLE 주소>&passkey=<6자리>` 파싱 |
| `src/specs/NativeRenterBle.ts` | Turbo Module 명세. Kotlin 구현 `android/app/src/main/java/com/nu54renter/ble/RenterBleModule.kt`: 페어링 모드 기기 스캔, Just Works 또는 passkey 본딩, 본딩 해제, 본딩한 기기에만 연결, SecureRandom, 등록 기기 목록 저장(SharedPreferences) |

```bash
make test
cd android && ./gradlew :app:installDebug   # 폰을 USB로 연결
```

아직 없는 것과 시험하지 못한 것은 다음과 같다.
- 지갑 설정 흐름(스캔, 본딩, 버튼 승인, 버튼 PIN, 페어링 코드, 지갑 확인, 홈, 결제 모드 켜기)은 2026-10-07에 rental 빌드(`python3 scripts/fw.py build --rental`) 보드와 Android 폰으로 끝까지 실행했다. 이전 폰이 거절되는지는 두 번째 폰이 없어 확인하지 못했다.
- 페어링 코드 분실과 오염된 폰 앱의 셋업 값은 아직 다루지 않는다(P02 설계 5절).
- 기기가 폰으로 보내는 `confirm.show`, `confirm.limit`, `payment.outcome`과 결제 모드 메시지는 펌웨어·소프트웨어 기기에 들어갔지만, 이 앱과 실기를 함께 돌려 보지는 않았다.

Android 업데이트는 패키지 관리자의 데이터 보존 롤백 정책을 선언한다. API 34의 전용 QA 앱에서 최신 연습 기록까지 남는지 검증하고, 보호 지갑은 지원 자료가 있는 조합만 배포한다. [설치·롤백 검증](../p10-platform/release/README.md#p02-android-시뮬레이터-롤백-리허설)

### W12 기기 유지관리

등록된 개인 지갑에서 **설정 → 기기 상태 확인**으로 개인 보드 유지관리를 연다. 검색 목록은 연결 후보이며, 등록 TLS와 현재 소유자의 지문·세대를 확인한 뒤에만 운영 캠페인 증거를 읽는다. 저장된 미해결 승인이나 진행 중인 기기 승인이 있으면 증거 요청을 시작하지 않는다. 보드가 응답하지 않으면 소유 상태를 확인할 수 없다고 표시한다. 기기나 사용자 범위가 바뀌거나 앱이 비활성화되면 이전 응답을 화면에 적용하지 않는다. 캠페인 서비스가 서명과 실행 슬롯을 대조하기 전에는 업데이트 성공을 표시하지 않는다.

개인 초기화 조건 화면은 보관된 개인 서명·송금 미결, 독립 복원 확인, 기기 정산과 별도 물리 확인을 안내한다. 휴대전화에 미결이 보이지 않아도 초기화를 승인하지 않는다. 대여 반납은 지갑 선택의 별도 화면에서 미결 지급이나 기존 대여 연결을 열며, 개인 관리 요청을 만들지 않는다. 대여 완료는 운영자의 별도 정산·권한을 필요로 한다.

```sh
pnpm -C products/p02-user-app test --runInBand walletMaintenance ownerSession
make -C products/p01-device-firmware test HOST_BUILD=/tmp/nu54-w12-host-build
```

호스트 시험은 정상 설정·대여 진입, 인증된 OwnerSession의 캠페인 요청 결합, 승인 중 차단, 미결 보존, 이전 소유자의 늦은 응답 폐기를 사용한다. 실제 슬롯 초기화·플래시·업데이트 적용과 자산 이동은 실행하지 않는다.

### 개인 지갑 보호 정책

개인 지갑의 **설정 → 한도와 잠금**에서 등록한 보드를 찾아 정책을 읽습니다. 기기에서 읽은 활성 한도와 허용 주소가 표시됩니다. 한도 입력은 같은 보드, 소유자 세대, 체인과 자산에 대해 검증한 소수 자릿수가 있을 때 열립니다.

**변경 내용 검토**로 기존 값과 새 값을 대조합니다. **변경 검토 취소**를 누르면 입력값이 마지막으로 확인한 정책으로 돌아갑니다. 진행하려면 **독립 표시로 변경 요청**을 누르고 별도 표시에서 기기, 자산, 허용 주소와 두 한도를 대조한 뒤 보드의 SW1을 누르고 기기 PIN을 입력합니다. **보호 정책 읽기**를 다시 눌러 기존 한도와 대기 값을 확인합니다.

**대기 완료 확인**으로 기기에 적용을 요청합니다. 활성화는 기기의 신뢰 시간이 결정합니다. 앱은 같은 기기에서 활성 정책을 다시 읽은 뒤 새 한도가 적용됐다고 표시합니다. 앱을 다시 열거나 연결이 끊기면 같은 보드를 다시 찾아 보호 정책을 읽습니다.

변경은 초기 보안 설정을 마친 실험용 V1 보호 정책의 단일 허용 주소와 한도에 적용됩니다. 여러 주소의 허용목록과 최종성 전용 프로필의 변경은 지원 범위 밖입니다. 현재 장치에서 보호 정책 기능을 사용할 수 없으면 설정에 미지원 상태가 표시됩니다. 기존 키와 미결 요청은 각 원래 지갑과 요청에서 관리합니다. [지원 범위와 실험 검증](../p01-device-firmware/W27-POLICY-PROGRESS.md)

대기 중인 정책에서 **기기 상태 확인**을 열면 등록 보드를 인증해 상태를 조회할 수 있습니다. **보호 정책으로 돌아가기**는 같은 등록 기기의 정책을 다시 읽습니다. 적용 화면의 **설정으로 돌아가기**는 별도 설정 홈을 엽니다. 기기 응답이 끊기면 적용 완료를 표시하기 전에 정책을 다시 읽어야 합니다.

**기기 잠금 규칙 확인**은 인증한 기기가 응답했을 때만 유휴 2분과 분리 즉시 잠금 규칙을 표시합니다. 설정 홈은 확인된 보드 식별자의 끝자리를 표시하며, 기기에 연결하지 않은 상태는 미확인으로 표시합니다. 설정에서 분실 복구, 인증수단, 네트워크, 여행 기록과 동의로 이동할 수 있습니다. 클라우드 인증은 배포 프로필이 제공되는 경우 해당 인증 화면을 엽니다.

새 앱 세션에서는 **한도와 잠금**에서 등록 보드를 찾아 인증합니다. 같은 등록 보드의 잠금 규칙은 설정 홈에서도 재확인할 수 있습니다. 정책 적용 직후에는 활성 정책을 읽은 기기 연결에서 잠금 규칙도 확인합니다. 기기가 해당 기능을 제공하지 않거나 인증이 끊긴 경우에는 확인된 정책과 확인하지 못한 잠금 규칙을 각각 표시합니다. 활성 한도와 변경 검토의 단위는 검증된 체인과 자산에 따라 표시합니다.

설정의 **접근성**에서 화면 읽기, 동작 줄이기, 표시 언어와 알림 설정을 확인합니다. **도움말·진단**은 공유할 항목을 먼저 미리 보여줍니다. 두 화면 모두 설정으로 돌아갈 수 있습니다.

### 매장 등록 없는 주문의 빌드 값

매장 화면에서 등록 없이 주문하는 길은 앱 빌드에 고정한 값만 믿는다. 값은 `src/merchant/customerEntranceDeployment.ts`의 두 상수에 들어가고, 저장소에는 둘 다 `undefined`로 둔다. 비어 있으면 앱은 매장 목록, 매장 화면의 주문, 수령 QR을 열지 않고 그렇다고 알린다.

| 상수 | 값 | 쓰는 곳 |
|---|---|---|
| `approvedCustomerEntrance` | `origin`(opsd `P05_CUSTOMER_ADDR`의 https 주소), `ca`(서버 인증서를 발급한 CA, DER의 base64), `pin`(서버 인증서 공개키 SPKI의 SHA-256, 소문자 16진수 64자) | 매장 목록, 매장 화면, 메뉴, 계정 증명, 세션 요청. 시스템 인증서 저장소는 쓰지 않는다 |
| `approvedOrderPayment` | `chain`(8283), `contract`(주문 계약 `OrderEscrow` 주소), `asset`(그 계약의 토큰 주소), `token`(이름·기호·화면 이름·자릿수. 없으면 달러 시험 토큰), `rpc`(체인을 읽을 주소. 소유자가 고른 서버가 먼저다) | 견적 검증. 서버는 매장의 계약 번호(`merchantId`)만 알려 주고, 앱은 이 계약에서 그 번호의 견적 서명자와 받는 주소를 읽는다 |

빌드할 때 묶는 방법은 하나다. 아래 모양의 JSON 파일을 만들고 그 경로를 `NU54_CUSTOMER_ORDER_BUILD`에 넣어 번들한다(`build-tools/customer-order-build.js`, `babel.config.js`). 값이 형식에 맞지 않으면 번들이 멈춘다. `payment`는 빼도 되고(목록만 보는 빌드), `entrance` 없이 `payment`만 넣을 수는 없다. 필드 이름은 매장 등록서의 `payment` 항목과 같으므로, 로컬 배포 기록의 `chain`·`contract`·`asset`·`token`·`rpc`를 그대로 옮긴다. 체인의 첫 블록은 앱이 이미 가진 값(`src/wallet/readProfiles.ts`의 `stable.anchor`)을 쓴다.

```json
{
  "entrance": {"origin": "https://127.0.0.1:8444", "ca": "<base64>", "pin": "<64 hex>"},
  "payment": {"chain": 8283, "contract": "0x…", "asset": "0x…",
    "token": {"name": "NU54 Test KRW", "symbol": "tKRW", "label": "tKRW", "decimals": 2}, "rpc": "https://127.0.0.1:8545"}
}
```

```bash
openssl x509 -in ca.pem -outform der | base64 | tr -d '\n'                                             # ca
openssl x509 -in server.pem -pubkey -noout | openssl pkey -pubin -outform der | openssl dgst -sha256   # pin
NU54_CUSTOMER_ORDER_BUILD=/path/to/order-build.json npx react-native start --reset-cache               # 로컬 시험 번들
pnpm -C products/p02-user-app test customerOrderBuild orderPaymentTrust storeOrder                     # 묶는 도구, 검증, 주문 흐름
```

다시 배포해야 하는 때:

- 서버 인증서의 키를 바꾸면 `pin`이 달라진다. 새 키로 바꾸기 전에 새 `pin`을 묶은 앱을 먼저 내야 하고, 옛 앱은 새 서버에 연결하지 못한다. 같은 키로 인증서만 다시 받으면 `pin`은 그대로다.
- CA를 바꾸면 `ca`가, 서버 주소를 바꾸면 `origin`이 달라져 앱을 다시 내야 한다.
- 주문 계약을 새로 배포하거나 토큰을 바꾸면 `contract`·`asset`·`token`이 달라져 앱을 다시 내야 한다. 옛 앱은 새 계약의 견적을 거절한다.
- 매장의 견적 서명 키나 받는 주소를 바꾸는 것(`configureMerchant`)은 앱을 다시 내지 않는다. 앱이 결제 때마다 계약에서 읽는다. 확정된 블록을 읽으므로 바꾼 직후 잠깐은 옛 값으로 검증해 새 견적을 거절할 수 있다.

남는 위험: 서버가 등록된 다른 매장의 번호를 주면 그 매장의 견적이 검증을 통과한다. 돈은 같은 주문 계약에 잠기고 이의로 돌려받는다. 체인을 읽는 서버(`rpc` 또는 소유자가 고른 서버)가 거짓 기록을 주는 경우도 같은 범위다. 운영자 서명 매장 프로필은 실제 자산 전에 더한다([유즈케이스 점검과 결정](../../docs/content/commercial/use-case-review.md) 9장).

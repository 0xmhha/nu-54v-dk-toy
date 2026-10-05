# P04 · Merchant kiosk

> **DF-20260925-02 기준 (2026-09-25):** 이번 12주 사이클의 범위와 설계는 [기획](../../docs/content/products/p04/plan.md) · [SRS](../../docs/content/products/p04/srs.md) · [유즈케이스](../../docs/content/products/p04/use-cases.md) · [설계](../../docs/content/products/p04/design.md)가 정한다. 아래 원문은 DF-20260920-01 기준이며 충돌하면 위 문서를 따른다.

## 개발 (DF-20260925-02)

| 경로 | 내용 |
|---|---|
| `src/` | React Native 0.87(New Architecture, TypeScript) 키오스크 앱 |
| `src/App.tsx` | 결제 화면: 금액 입력 → 기기 승인 대기(10초) → approved, refused, failed, Checking, 시간 초과 취소, busy |
| `src/kiosk/` | `pay.ts` 한 건의 결제(가스 확인, 기기 세션, 제출, 기기에 `payment.outcome`), `config.ts` 개발 셋업 가져오기와 키 보관, `receipt.ts` P07 indexer 영수증 조회(10초 동안 다시 묻는다. 결제 판정과 무관하다) |
| `src/kiosk/orders.ts` | 재시작해도 남는 주문 기록(P04-NFR-05): 설계 3절의 전이만 허용하는 순수 함수, 서명은 제출 전에 저장, 열린 주문은 지우지 않는다. 앱이 다시 켜지면 서명된 주문은 같은 서명으로 재시뮬레이션·재전송하고(`resume`, P04-FR-14), 서명을 못 받은 주문은 취소한다 |
| `src/payment/limits.ts` | 한도 변경 중계: `limit.change`(기기에서 PIN과 버튼), 서명자 확인, `setLimits` 시뮬레이션·제출·finalized `LimitsChanged`. 같은 서명을 다시 내면 `NONCE_REPLAYED` |
| `src/payment/session.ts` | 결제 세션(프로토콜 5, 6절): 7주차 TimeAnchor 셋업, open, confirm, identify, prepare, 기기 서명 확인, 10초 안에 결과가 없으면 `session.cancel` |
| `src/payment/session.ts`의 TimeAnchor 처리 | 7주차 개발 셋업에서 넣은 anchor는 앱 설정에 남아 있다가 기기가 받아들일 때만 지운다. 체인 시각보다 90초 넘게 지난 anchor는 보내지 않고 `TIME_ANCHOR_STALE`로 멈추며, 보낸 세션에서는 결제 만료를 기기 시계 한도 안으로 줄인다. BLE 연결은 세 번까지 시도한다(`src/ble/central.ts`) |
| `scripts/anchor-server.ts` | 7주차 개발 셋업: 결제마다 그 기기용 TimeAnchor를 Mac에서 새로 서명해 준다(`opsctl anchor sign`, adb reverse). `provision-dev.ts --anchor-url http://127.0.0.1:8095/anchor`로 켜면 앱이 셋업 세션에서 기기 주소를 받은 뒤 anchor를 요청하고, 기기에 이미 시각 기준이 있으면 요청하지 않는다. 사람이 anchor를 미리 넣고 시간에 맞출 필요가 없다 |
| `src/payment/open.ts` | 결제 모드 세션 열기. 보안 채널(4.1절): 1회용 키를 가맹점 키로 서명(`KioskKey`)해 보내고, `session.open.ok`의 기기 1회용 키로 세션 키를 만든다. 기기가 1회용 키 없이 답하면 평문으로 내려가지 않고 `NOT_PERMITTED`로 멈춘다. 앱의 결제와 한도 변경은 모두 보안 세션이다 |
| `src/ble/` | `framing.ts` CBOR·envelope·조각(요청의 세션 응답만 받는다, 보안 세션이면 본문을 AES-GCM으로 봉인·복호화하고 새 세션 전에 `session.cancel`로 닫는다), `central.ts` 기기 찾기와 연결, `base64.ts` |
| `src/specs/` | Turbo Module 명세. `NativeNusBle.ts` BLE central(페어링 없음, N27), `NativeKioskVault.ts` Keystore AES 키로 감싼 키 보관과 SecureRandom(N32). Kotlin 구현은 `android/app/src/main/java/com/nu54kiosk/ble/` |
| `src/kiosk/timings.ts`, `scripts/pull-timings.ts` | W12-04 시간 기록: 서명받은 마지막 20건의 요청 전달→최종 결과 ms를 태블릿에서 CSV로 꺼낸다(`adb run-as`). `products/p10-platform/acceptance/w12.py timings W12-04 runs.csv`로 판정한다 |
| `scripts/provision-dev.ts` | 개발 셋업: 설정·attestation·키(`provision.json`)와 실행 직전 TimeAnchor(`anchor.json`)를 USB로 앱 전용 저장소에 넣는다 |
| `src/payment/signing.ts` | 가맹점 주문 서명, 기기가 돌려준 결제 서명 확인 |
| `src/payment/submit.ts` | 결제 프로토콜 7절의 제출과 판정: 가스 잔액(busy), eth_call 시뮬레이션, custom error → 거절 코드, OrderAlreadyPaid 대조, EIP-1559 전송, finalized PaymentSettled, status 0 재시뮬레이션, 10초 뒤 Checking, expiry 경과 failed |
| `src/chain/` | 외부 웹3 라이브러리 없이 쓰는 JSON-RPC, settle 호출 데이터·custom error·이벤트(공용 ABI package에서 계산), EIP-1559 서명(`Signer`는 앱에서 Android Keystore가 맡는다) |
| `android/`, `ios/` | 네이티브 프로젝트(Turbo Module 구현 위치) |
| `test/` | Jest 시험 |

```bash
make setup        # 저장소 루트에서 pnpm install
make test
make lint
make run          # react-native run-android
```

### 폰에서 결제해 보기 (보드 없이)

보드 대신 Mac을 BLE 기기로 쓴다([`packages/device-sim`](../../packages/device-sim/README.md)의 `serve-ble`). 폰은 USB 디버깅을 켜고 연결한다.

```bash
# 1. 앱 설치와 JS 서버
cd products/p04-merchant-kiosk/android && ./gradlew :app:installDebug && cd ..
adb reverse tcp:8081 tcp:8081 && pnpm start

# 2. 개발 셋업(한 번): 가맹점 attestation과 키오스크 키
O=products/p05-operations-backoffice/bin/opsctl      # 저장소 루트에서
$O attestation issue --merchant <kiosk 주소> --payout <payout> --name "NU54 Test Cafe" > att.json
node --experimental-strip-types products/p04-merchant-kiosk/scripts/provision-dev.ts --attestation att.json

# 3. Mac을 기기로 띄우고, 결제 직전에 그 기기 주소로 anchor를 넣는다
(cd packages/device-sim && pnpm -s serve-ble)        # 다른 터미널. 기기 주소를 출력한다
$O anchor sign --device <기기 주소> > anchor.json
node --experimental-strip-types products/p04-merchant-kiosk/scripts/provision-dev.ts --anchor anchor.json
```

영수증 화면을 쓰려면 2단계에 `--indexer http://<태블릿에서 닿는 주소>:8080`을 붙인다(P07 indexer, `products/p07-indexer`).

앱에서 금액을 넣고 결제를 요청한 뒤, `serve-ble` 터미널에 `confirm.show`가 나오면 10초 안에 `y`를 입력한다. 보드로 할 때는 SW4를 길게 눌러 결제 모드에 넣고, anchor를 보드 주소로 서명하고, LED2가 켜지면 SW1을 짧게 누른다.

개발 셋업은 키를 평문으로 USB로 보낸다(`adb shell run-as`, debuggable 빌드만). 앱은 받은 키를 Keystore AES 키로 감싸 저장하고 파일을 지운다.

settle 호출 데이터는 6주차 게이트 트랜잭션의 입력과 같고, 서명한 트랜잭션은 `cast mktx` 결과와 바이트까지 같다(`test/submit.test.ts`). 판정 흐름은 가짜 체인으로 P04-FR-06, 08~12, 14, 15를 시험한다. 테스트넷 결제 1건은 [`packages/device-sim`](../../packages/device-sim/README.md)의 `scripts/rehearse.ts --submit`으로 실행한다.


가맹점 Android 태블릿에서 실행하는 React Native 키오스크 제품이다.

- **소유 범위:** 점주 로그인, 매장·메뉴·재고·주문, BLE 결제 세션,
  stablecoin 결제, 부분/전체 환불, 매출·정산 화면
- **주요 경계:** P01과 C2, P10과 C3, P07의 결제 projection
- **WBS:** `WBS-P04-01`부터 `WBS-P04-05`

설계 진입점:

- [제품 WBS](../../docs/content/planning/product-wbs-overview.md#p04--rn-키오스크매장-운영)
- [키오스크 상거래 여정](../../docs/content/specifications/kiosk-commerce-journey-design.md)
- [결제·환불·반납 상태](../../docs/content/specifications/commerce-lifecycle-design.md)
- [매출·정산 계약](../../docs/content/specifications/settlement-ops-contracts.md)

구현을 시작할 때 RN 앱, 태블릿 build profile, 오프라인·재연결 상태와 E2E
fixture를 이 폴더에 둔다.

# device-sim

결제 기기의 소프트웨어 대역이다. 실제 기기와 같은 메시지를 같은 순서로 받고, 같은 검사와 거절 사유로 답하며, TF-M secure partition 대신 소프트웨어 키로 서명한다([결제 프로토콜](../../docs/content/specifications/protocol/payment-protocol.md) 5, 6절). 키오스크는 펌웨어가 준비되기 전에 결제 흐름 전체를 이것으로 시험하고, 펌웨어 담당은 메시지마다 무엇을 돌려줘야 하는지의 기준으로 쓴다. 시험 도구이며 기기에서 돌지 않는다.

| 파일 | 내용 |
|---|---|
| `src/device.ts` | `SoftwareDevice`: 상태(`PROVISIONED_NO_ANCHOR`, `READY`), 셋업 세션의 TimeAnchor, 결제 세션(`session.open` → `session.confirm` → `payment.identify` → `payment.prepare` → `payment.result`), 6절 4단계 검사와 거절 사유, 순차 nonce, 폰 앱으로 보내는 `confirm.show` |
| `src/link.ts` | 중앙 장치와 기기 사이의 선. 모든 메시지가 BLE와 같은 층(CBOR, envelope, 조각, 재조립)을 지난다. 기기 쪽은 본문 단위 `handleBody`를 써서 보안 세션(4.1절)이면 본문을 열고 응답을 봉인한다 |
| `src/keystore.ts` | `cast wallet new`가 만든 keystore를 Keychain 암호로 연다(Node 전용) |
| `scripts/gen-session-vectors.ts` | 공용 세션 벡터(`session-vectors.json`)를 만든다. 시나리오 29개(결제 12개, 대여 셋업과 reset 5개, 한도 변경과 PIN 잠금 5개, 스키마 밖 요청 1개, 보안 채널 6개), 메시지마다 보낸 바이트와 기기가 돌려줄 바이트. 펌웨어는 이 바이트를 그대로 내야 한다 |
| `scripts/rehearse.ts` | 테스트넷 결제 리허설. 키오스크의 결제 세션 코드(`src/payment/session.ts`)를 그대로 쓰고, 트랜잭션 없이 정산 컨트랙트에 eth_call로 확인한다 |
| `scripts/serve-ble.ts`, `peripheral/SimPeripheral.swift` | 이 Mac을 BLE 기기로 만든다. 보드 없이 폰의 키오스크 앱이 실제 BLE로 결제 세션을 연다 |

보안 채널(4.1절): 결제 모드 `session.open`에 `kioskEphemeral`, `attestation`, `kioskKeySignature`가 있으면 가맹점을 확인하고 1회용 키로 세션 키를 만든 뒤, 이후 본문을 AES-GCM으로 주고받는다(`@nu54/protocol`의 `SecureChannel`). `requireSecureSession`을 켜면 릴리스 기기처럼 평문 결제 세션을 거절한다.

대여 셋업(5절)도 흉내 낸다. 키를 주지 않으면 기기는 `UNPROVISIONED`로 시작한다. `setup.operator`(대여자 확인 버튼 `confirmSetup`) 다음에 키를 만들고 nonce 시작값을 정한 뒤, PIN(`enterPin`)이 정해지면 모든 값을 한 번에 기록한다. `device.reset`은 기록된 운영자가 이 기기 주소에 서명한 것만 받는다. 키를 주면 셋업이 끝난 `PROVISIONED_NO_ANCHOR` 상태로 시작하고, 셋업 세션의 TimeAnchor로 `READY`가 된다. 한도 변경(`limit.change`)은 폰에 `confirm.limit`을 보내고 PIN(`enterPin`)과 버튼(`approve`)을 받은 뒤 결제와 같은 순차 카운터로 서명한다. PIN을 `pinMaxRetries`(기본 5)번 틀리면 `PIN_LOCKED`가 되어 모든 서명을 거절하고, RAM reset 뒤에도 유지된다. 아직 흉내 내지 않는 것은 보안 채널(4.1절)이다.

```ts
import { connect, SoftwareDevice } from "@nu54/device-sim";

const device = new SoftwareDevice({ key, operator, contract, chainId: 8283, nonceStart: 256n * 7n });
const link = connect(device, 185);                       // ATT_MTU
const [ack] = link.send({ v: 1, type: "setup.timeAnchor", ... });
const [result] = link.send({ v: 1, type: "payment.prepare", ... });   // payment.result
```

## 테스트넷 리허설

운영자 키는 `opsctl`에만 있으므로 attestation과 TimeAnchor는 `opsctl`로 만든다. 기기 키(`nu54-device`)와 가맹점 키(`nu54-kiosk`)는 keystore에서 Keychain 암호로 연다.

```bash
O=products/p05-operations-backoffice/bin/opsctl
$O attestation issue --merchant <kiosk 주소> --payout <payout> --name "NU54 Test Cafe" > att.json
$O anchor sign --device <기기 주소> > anchor.json
cd packages/device-sim
node --experimental-strip-types scripts/rehearse.ts --attestation ../../att.json --anchor ../../anchor.json --amount 1000000
```

키오스크 쪽은 키오스크의 `src/payment/signing.ts`로 주문에 서명하고 기기 서명을 확인한다. 결과의 `settleSimulation`이 `success`면 그 서명은 배포된 컨트랙트에서 정산된다. 실패하면 revert 데이터(custom error selector)를 보여 준다.

2026-10-01 실행: 1 tUSDC 결제가 `success`, 60 tUSDC 결제는 `OverCap`(`0x342fa66d`)으로 거절되었다.

`--transport ble`이면 시뮬레이터 대신 보드와 BLE로 결제 세션을 연다(펌웨어 `tools/bringup/pay_bridge.py`). 이때 `--anchor`는 보드 주소로 서명한 것이어야 하고 실행 직전에 발급한다(오래된 anchor는 기기 시각을 늦춰 만료 검사에 걸린다). 승인은 보드의 SW1이며, `--press-sim`은 개발 실행에서 버튼 시뮬레이터로 누른다. 7주차 게이트는 사람이 누른다.

`--limit`을 붙이면 결제 대신 한도 변경을 한다(`--per-payment`, `--daily`, 기본 0 = 상한 그대로). 서명을 받아 `setLimits`를 제출하고, 같은 서명을 한 번 더 제출해 `NONCE_REPLAYED`로 거절되는 것까지 확인한다(W12-05 시연 경로). 2026-10-03 testnet 실행: 첫 제출 approved, 재제출 `NONCE_REPLAYED`.

`--submit`을 붙이면 시뮬레이션 뒤 키오스크의 `src/payment/submit.ts`가 키오스크 가스 키(`nu54-kiosk`)로 settle 트랜잭션을 보내고 finalized `PaymentSettled`까지 판정한 뒤, 기기에 `payment.outcome`을 보낸다. 실제 트랜잭션이므로 키오스크 계정의 WKRC와 기기 계정의 tUSDC가 쓰인다. 2026-10-01 실행: 1 tUSDC 결제가 approved(type-2 트랜잭션, 103,473 gas, 약 4.93 WKRC), 이벤트의 device·amount·nonce가 기기 서명과 같았다.

## Mac을 BLE 기기로 쓰기

보드가 없을 때 폰의 키오스크 앱을 시험하는 방법이다. `peripheral/SimPeripheral.swift`(CoreBluetooth)가 결제 GATT 서비스를 광고하고 조각을 그대로 넘기며, `scripts/serve-ble.ts`가 재조립·envelope·CBOR와 기기 자체를 맡는다. 서비스와 특성 UUID는 프로토콜 스키마에서 읽는다. 대여자의 버튼은 터미널이다. 폰 앱이 보여 줄 `confirm.show`를 출력하고, `y`는 승인, `n`은 거절이다.

```bash
cd packages/device-sim
pnpm -s serve-ble                 # 처음 한 번 swiftc로 peripheral을 빌드한다
pnpm -s serve-ble --approve yes   # 누르지 않고 항상 승인
```

anchor는 기기 주소 `nu54-device`(실행할 때 출력한다)로 서명한 것을 쓴다. 보드와 다른 점은 셋이다. macOS가 ATT MTU를 정하고 바꿀 수 없다. 링크는 페어링하지 않는다(7주차 결제 세션과 같다). 기기 키는 `nu54-device` 시험 키다. 같은 Mac의 BLE central(`pay_bridge.py`)은 자기 광고를 보지 못하므로 상대는 다른 기기여야 한다.

폰 앱도 함께 연결할 수 있다. 쓰기를 한 번도 하지 않고 듣기만 하는 central을 폰 앱으로 보고 `confirm.show`와 전달받은 `payment.outcome`을 보낸다. 보드는 본딩 여부로 구별하지만 macOS peripheral은 본딩 상태를 알 수 없어 이렇게 구별한다.

기기는 버튼을 기다리는 동안 `session.cancel`이나 연결 끊김이 오면 서명하지 않는다. 키오스크는 늦게 온 응답을 다음 요청의 답으로 받지 않는다(세션 id가 다르면 버리고, 보낼 때 이전에 쌓인 응답을 지운다).

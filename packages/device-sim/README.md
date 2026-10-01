# device-sim

결제 기기의 소프트웨어 대역이다. 실제 기기와 같은 메시지를 같은 순서로 받고, 같은 검사와 거절 사유로 답하며, TF-M secure partition 대신 소프트웨어 키로 서명한다([결제 프로토콜](../../docs/content/specifications/protocol/payment-protocol.md) 5, 6절). 키오스크는 펌웨어가 준비되기 전에 결제 흐름 전체를 이것으로 시험하고, 펌웨어 담당은 메시지마다 무엇을 돌려줘야 하는지의 기준으로 쓴다. 시험 도구이며 기기에서 돌지 않는다.

| 파일 | 내용 |
|---|---|
| `src/device.ts` | `SoftwareDevice`: 상태(`PROVISIONED_NO_ANCHOR`, `READY`), 셋업 세션의 TimeAnchor, 결제 세션(`session.open` → `session.confirm` → `payment.identify` → `payment.prepare` → `payment.result`), 6절 4단계 검사와 거절 사유, 순차 nonce, 폰 앱으로 보내는 `confirm.show` |
| `src/link.ts` | 중앙 장치와 기기 사이의 선. 모든 메시지가 BLE와 같은 층(CBOR, envelope, 조각, 재조립)을 지난다 |
| `src/keystore.ts` | `cast wallet new`가 만든 keystore를 Keychain 암호로 연다(Node 전용) |
| `scripts/rehearse.ts` | 테스트넷 결제 리허설. 트랜잭션 없이 정산 컨트랙트에 eth_call로 확인한다 |

아직 흉내 내지 않는 것: `setup.operator`, 키 생성, PIN, `limit.change`, `device.reset`(모두 `NOT_PERMITTED`), 보안 채널(4.1절). 기기는 셋업이 끝난 `PROVISIONED_NO_ANCHOR` 상태로 시작하고, 셋업 세션의 TimeAnchor로 `READY`가 된다.

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

`--submit`을 붙이면 시뮬레이션 뒤 키오스크의 `src/payment/submit.ts`가 키오스크 가스 키(`nu54-kiosk`)로 settle 트랜잭션을 보내고 finalized `PaymentSettled`까지 판정한 뒤, 기기에 `payment.outcome`을 보낸다. 실제 트랜잭션이므로 키오스크 계정의 WKRC와 기기 계정의 tUSDC가 쓰인다. 2026-10-01 실행: 1 tUSDC 결제가 approved(type-2 트랜잭션, 103,473 gas, 약 4.93 WKRC), 이벤트의 device·amount·nonce가 기기 서명과 같았다.

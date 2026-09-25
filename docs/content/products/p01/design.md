# P01 설계

[srs.md](srs.md)의 요구를 NCS v3.4.0/Zephyr 4.4 위에서 어떻게 나누어 구현하는지 정한다. 메시지 규칙은 [payment-protocol.md](../../specifications/protocol/payment-protocol.md)가, 서명 정답은 [eip712-vectors.json](../../specifications/protocol/eip712-vectors.json)이 정한다.

## 1. 구조

키 관련 동작과 버튼 토큰은 secure 쪽에만 두고, non-secure 앱은 메시지 처리와 화면만 맡는다.

| 모듈 | 위치 | 하는 일 |
|---|---|---|
| `ble_pay_svc` | non-secure | GATT 서비스 `rx`/`tx`, 조각 재조립과 envelope 확인 [N09] |
| `nfc_handover` | non-secure | NFCT로 태그를 에뮬레이션하고 페어링마다 BLE 주소와 새 LESC OOB 데이터를 쓴다 |
| `session` | non-secure | session.open(mode)/confirm/cancel, sessionId, deviceNonce 관리 |
| `setup_flow` | non-secure | 셋업 세션, `setup.operator`, `setup.timeAnchor`, `device.reset` 처리 [N23] |
| `pay_flow` | non-secure | identify/prepare/result 상태 기계, 표시 요청, 버튼 대기 |
| `eip712` | non-secure | 스키마 타입의 encodeType·hashStruct·digest 계산 |
| `verifier` | non-secure | MerchantAttestation, MerchantOrder, TimeAnchor, DeviceReset 서명 복구와 비교 [N05][N06] |
| `ui` | non-secure | 디스플레이 표시만. 버튼 입력은 secure 쪽이 받는다 |
| `key_service` | TF-M secure partition | 키 생성·서명·삭제, 버튼 토큰, PIN 카운터, 셋업 값과 anchor 저장 [N02] |
| `se_driver` | secure (W9부터) | 외부 secure element와 통신, 키 감싸기와 서명 |
| MCUboot | 부트로더 | 서명 이미지 확인, 유선 serial recovery [N12] |

## 2. 서명 권한 경계

`key_service`는 PSA 호출 몇 개만 연다. `key_generate`, `register_digest(digest, purpose)`, `sign_digest(digest, purpose)`, `pin_set`, `setup_store`, `anchor_store`, `anchor_read`, `reset_all` [N24].

1. `pay_flow`는 버튼을 기다리기 직전에 `register_digest`로 서명할 digest와 purpose(`payment` 또는 `limit_change`) 한 건을 secure 쪽에 등록한다.
2. 버튼은 secure 쪽 GPIO 인터럽트가 받는다. 인터럽트는 등록된 digest·purpose 한 건에만 1회용 토큰을 발급한다. `ui`는 표시만 하고 토큰을 만들 수 없다.
3. `sign_digest(digest, purpose)`는 등록된 값과 같고 토큰이 유효할 때만 서명하고, 서명 뒤 등록을 지운다. 등록되지 않은 purpose, 다른 digest, 재사용 토큰은 거절한다.
4. purpose가 `limit_change`이면 `sign_digest` 안에서 secure 쪽이 PIN을 확인하고 재시도 카운터를 관리한다. pinMaxRetries를 넘으면 PIN_LOCKED가 되어 모든 purpose를 거절한다.
5. Numeric Comparison 확인과 셋업 확인에 쓰인 버튼 입력은 서명 토큰이 되지 않는다.

이 경계는 non-secure 코드가 뚫려도 버튼 없이 서명이 나오거나 등록되지 않은 purpose로 서명하는 것을 막는다. 그러나 "표시한 내용과 서명한 내용이 같다"는 보장은 digest와 표시 문자열을 non-secure 코드가 만드므로 non-secure 코드가 무결하다는 가정 아래의 주장이다. trusted display는 범위 밖이며, 이 경우에도 손실은 컨트랙트 한도로 제한된다.

W9부터 `key_service`는 TRNG로 만든 키를 외부 secure element로 감싸 두고 TF-M 쪽에는 SE 세션 키와 키 핸들만 둔다. SE가 컷되면 TF-M 봉인으로 남고 이를 waiver로 기록한다 [N14].

## 3. 상태 기계

```
UNPROVISIONED --setup.operator(버튼 확인), 키 생성, PIN--> PROVISIONED_NO_ANCHOR
PROVISIONED_NO_ANCHOR --유효한 setup.timeAnchor--> READY
READY --session.open(payment)/confirm--> SESSION
SESSION --payment.identify 통과--> MERCHANT_SHOWN
MERCHANT_SHOWN --payment.prepare 통과--> AWAIT_BUTTON
AWAIT_BUTTON --승인--> SIGNED --payment.result--> SESSION
AWAIT_BUTTON --거절 / session.cancel / expiry 경과--> SESSION
READY, SESSION 등 --RAM 초기화 reset--> PROVISIONED_NO_ANCHOR (키 유지)
어느 상태든 --PIN 오입력 pinMaxRetries 초과--> PIN_LOCKED
PIN_LOCKED --RAM 초기화 reset--> PIN_LOCKED (잠금은 secure 저장소에 유지)
어느 상태든 --운영자 서명 DeviceReset--> UNPROVISIONED
```

셋업 세션(`session.open` mode=setup)은 UNPROVISIONED와 PROVISIONED_NO_ANCHOR에서만 열리고, 그 밖의 상태에서는 `error{NOT_PERMITTED}`다 [N23]. `setup.operator`는 UNPROVISIONED에서만 받는다. `device.reset`은 어느 상태·세션에서나 받되 운영자 서명이 맞아야 한다.

anchor를 무효로 만드는 reset은 다음과 같다. 모두 RAM이 초기화되기 때문이다.

| 사건 | anchor | 키 |
|---|---|---|
| 전원 손실 | 무효 | 유지 |
| watchdog reset | 무효 | 유지 |
| System OFF에서 깨어남 | 무효 | 유지 |
| serial recovery 뒤 재부팅 | 무효 | 유지 |
| 운영자 서명 DeviceReset | 삭제 | 삭제 |

## 4. 저장

| 항목 | 위치 | device.reset | RAM 초기화 reset |
|---|---|---|---|
| 기기 키 | TF-M 보호 저장소 또는 SE | 삭제 | 유지 |
| PIN 검증값과 재시도 카운터 | TF-M 보호 저장소 | 삭제 | 유지 |
| 운영자 주소, 정산 컨트랙트 주소, chainId | TF-M 보호 저장소(셋업 때 기록) | 삭제 | 유지 |
| 마지막 TimeAnchor timestamp | TF-M 보호 저장소 | 삭제 | 유지(단조성 비교용) |
| anchor 유효 표시 | RAM | 삭제 | 무효 |
| 사용한 nonce 목록 | TF-M 보호 저장소 | 삭제 | 유지 |
| BLE bonding 정보 | Zephyr settings | 삭제 | 유지 |

평문 키나 시드는 어느 non-secure 영역에도 쓰지 않는다. 마지막 anchor timestamp를 reset 뒤에도 남기는 이유는 이전 값보다 늦지 않은 TimeAnchor를 거절하기 위해서다.

## 5. 메시지 처리

- **발견과 페어링:** `nfc_handover`가 페어링마다 태그에 주소와 새 OOB 데이터를 쓴다. 링크 보안 요구는 LE Secure Connections 인증·암호화이며, OOB가 없으면 Numeric Comparison 숫자를 화면에 보이고 버튼으로 확인한다 [N09].
- **재조립:** 모든 envelope에 조각 헤더가 붙는다. 협상된 MTU에서 조각 크기 `ATT_MTU - 5`를 계산한다. sequence가 기대값과 다르거나 index가 건너뛰거나 digest가 다르거나 본문이 2048바이트를 넘으면 버퍼를 비우고 `error{BAD_FRAME}`을 보낸 뒤 세션을 닫는다.
- **setup.operator:** UNPROVISIONED에서만 받는다. operator, contract, chainId를 표시하고 대여자 버튼 확인 뒤 RAM에 보관하고 `setup.ack{step: setup.operator}`를 보낸다. 운영자 값은 키 생성과 함께 `setup_store`로 한 번에 저장하므로 keygen ack 전에 세션이 끊기면 아무것도 남지 않는다. 이어서 키 생성과 PIN 설정이 끝나면 `setup.ack{step: keygen, device}`로 새 주소를 보낸다. UNPROVISIONED에서는 키가 없으므로 `session.open.ok`에 device를 넣지 않는다 [N23].
- **setup.timeAnchor:** 셋업 세션에서만 받는다. `device`가 자기 주소인지, 서명자가 운영자 주소인지, timestamp가 이전 값보다 엄격히 늦은지 확인한 뒤 `anchor_store`를 호출하고 `setup.ack{step: setup.timeAnchor, accepted, lastAnchor}`를 보낸다 [N06].
- **device.reset:** DeviceReset `{device, nonce}` digest로 서명자를 복구해 운영자 주소와, device를 자기 주소와 비교한 뒤 `reset_all`을 호출한다.
- **identify:** `verifier`가 attestation digest를 계산하고 서명자를 복구해 운영자 주소와 비교한다. anchor 시간과 `validFrom..validUntil`을 anchorClockSkew를 허용해 비교한다.
- **prepare:** MerchantOrder digest로 가맹점 서명자를 복구해 attestation의 merchant와 비교한다. authorization.merchant가 attestation의 merchant와 같은지, payout이 attestation·주문과 같은지, orderId·token·amount·expiry가 주문과 같은지, chainId·contract가 셋업 값과 같은지 확인한다. expiry가 현재부터 authorizationExpiry 안인지도 확인한다. 화면 문자열은 authorization 필드에서만 만든다.
- **서명:** 기기가 nonce를 골라 PaymentAuthorization digest를 만들고 `register_digest` 후 버튼 토큰으로 `sign_digest`를 부른다. 서명과 nonce를 `payment.result{approved}`로 보낸다 [N04].
- **session.cancel:** 등록한 digest를 지우고 버튼 대기를 멈춘다. 아무것도 서명하지 않는다.
- **서명 뒤 링크 끊김:** `payment.result`를 보낸 뒤 BLE가 끊기면 키오스크는 이미 받은 서명으로 제출을 계속한다. 기기는 아무것도 하지 않으며 같은 주문에 다시 서명하지 않는다.
- **payment.outcome:** 키오스크의 최종 결과(approved, refused, failed, Checking)를 표시한다.

## 6. 오류와 거절 매핑

| 조건 | 모듈 | 응답 |
|---|---|---|
| attestation 서명자, 주문 서명자, authorization.merchant, payout, 주문 필드, 셋업 값 불일치 | `verifier` | `payment.result refused` MERCHANT_FORGED |
| anchor 시간이 유효 기간 밖, expiry가 너무 멂 | `verifier` | `payment.result refused` ATTESTATION_EXPIRED |
| anchor 무효 | `pay_flow` | `payment.result refused` TIME_ANCHOR_MISSING |
| 사용자 거절 | `pay_flow` | `payment.result refused` USER_REJECTED |
| PIN 잠김 | `key_service` | `payment.result refused` PIN_LOCKED |
| 버튼 대기 중 expiry 경과 | `pay_flow` | `payment.result refused` TIMEOUT (시험 전용) |
| 모르는 타입·메시지 | `pay_flow` | `error` UNSUPPORTED_TYPE |
| 셋업 권한 없는 상태의 셋업 요청 | `setup_flow` | `error` NOT_PERMITTED |
| 조각·digest 오류 | `ble_pay_svc` | `error` BAD_FRAME, 세션 닫음 |

MERCHANT_REVOKED, OVER_CAP, NONCE_REPLAYED는 컨트랙트가 만든다. 기기는 이 코드를 내지 않고 `payment.outcome`으로 받은 결과를 표시한다.

## 7. 보안

- 서명은 등록된 digest·purpose에 대한 secure 쪽 버튼 토큰이 있어야만 나온다 [N24]. 키 반출 명령은 어느 전송(BLE, UART)에도 없다.
- AP-Protect를 켜고 release 빌드에서 디버그 로그의 키 관련 출력을 막는다. W12 릴리스 이미지에는 USB CDC 시험 harness를 넣지 않는다. NVM 덤프 시험은 AP-Protect를 켜기 직전의 같은 image hash로 한다.
- MCUboot는 오프라인에 둔 서명 키로 서명한 이미지만 부팅하고, 갱신은 UART 위 SMP를 쓰는 유선 serial recovery로만 받는다. BLE 전송 SMP와 BLE DFU는 넣지 않는다 [N12].
- 운영자 주소와 셋업 값은 대여 셋업마다 버튼 확인으로 한 번 쓰고, 운영자 서명 DeviceReset 전에는 바꿀 수 없다 [N23]. LESC 페어링만으로는 셋업 권한이 생기지 않는다.
- 기기는 오프라인이어서 registry의 가맹점 철회를 알 수 없다. 이 공백은 컨트랙트가 막는다 [N05].

## 8. waiver 설계

설계만 하고 이번 사이클에 구현하지 않는 항목이다 [N14].

- **req 10 anti-exfil:** ECDSA 서명 nonce는 RFC 6979 결정론적 nonce로 만들어, 같은 digest에는 같은 서명이 나오게 한다. 이렇게 하면 호스트가 서명을 여러 번 받아 비교해 펌웨어가 nonce로 키를 흘리는지 검사할 수 있다. 호스트 엔트로피를 섞는 anti-exfil 프로토콜(호스트 commit 후 기기 nonce 공개)은 키오스크·P05 쪽 구현과 프로토콜 확장이 필요해 보류한다.
- **req 13 정품 기기 attestation:** 공장 provisioning 때 TF-M에 기기 증명 키를 넣고, 셋업 때 기기 주소를 이 키로 서명해 운영자가 확인하는 방식이 설계안이다. 이번 사이클에는 공장 provisioning 절차가 없어 보류하며, P05는 셋업 세션에서 기기가 `setup.ack{step: keygen, device}`로 보고한 주소를 그대로 믿는다.
- **req 12 의존성 pinning:** west manifest의 NCS·모듈 revision을 커밋 hash로 고정하고 빌드 도구의 lockfile을 두는 것이 설계안이다. 이번 사이클에는 재현 빌드 검증 절차를 만들지 않아 보류하고, 사용한 버전은 week12-log 1절의 도구·의존성 버전 행에 기록한다.

## 9. 시험 설계

| 시험 | 방법 | 통과 기준 |
|---|---|---|
| 벡터 적합성 | eip712-vectors.json의 모든 벡터를 `eip712`에 넣어 digest 비교, 역할별 서명자 복구 | 전부 일치 |
| 서명 복구 | PA 벡터 digest를 기기 키로 서명하고 host에서 복구 | 기기 주소와 일치 |
| 서명 권한 경계 | 등록하지 않은 digest·purpose, 재사용 토큰, 셋업 확인 버튼으로 `sign_digest` 호출 | 모두 거절 |
| 거절 주입 | 6절 표의 각 조건을 시험 도구로 주입 | 표의 응답과 일치 |
| 셋업 권한 | READY 상태의 셋업 세션, 서명 없는 device.reset, 두 번째 setup.operator | 모두 거절 |
| 조각 결함 주입 | 순서 바꿈, 누락, digest 변조, 초과 길이 | 모두 BAD_FRAME, 세션 닫힘, 재부팅 없음 |
| reset 종류 | 3절 표의 각 reset 후 결제 시도와 재-anchor | 키 유지, anchor 무효, 재-anchor 뒤 결제 |
| 초기화 | 운영자 서명 device.reset 후 보호 저장소 덤프 | 키·PIN·셋업 값·anchor 없음 |
| 부트 보호 | 서명 안 된 이미지 serial recovery | 부팅하지 않음, 대상 slot과 복구 과정 기록 |
| 반복 | 연속 20회 결제 | 전부 approved, 누수 없음 |

# P01 설계

[srs.md](srs.md)의 요구를 NCS v3.4.1/Zephyr 4.4.2(보드 타깃 `nu54v_dk/nrf54l15/cpuapp`) 위에서 어떻게 나누어 구현하는지 정한다. 메시지 규칙은 [payment-protocol.md](../../specifications/protocol/payment-protocol.md)가, 서명 정답은 [eip712-vectors.json](../../specifications/protocol/eip712-vectors.json)이 정한다.

## 1. 구조

키 관련 동작과 버튼 토큰은 secure 쪽에만 두고, non-secure 앱은 메시지 처리, LED 안내, 폰 앱으로 보내는 확인 필드만 맡는다. 기기에는 화면이 없고, 결제 내용은 대여자 폰 앱이 표시한다 [N26].

| 모듈 | 위치 | 하는 일 |
|---|---|---|
| `ble_pay_svc` | non-secure | GATT 서비스 `rx`/`tx`, 조각 재조립과 envelope 확인, 연결마다 역할(본딩한 폰 앱·운영자 도구, 페어링하지 않은 키오스크) 구분 [N09][N27] |
| `pairing` | non-secure | 버튼을 길게 눌렀을 때만 여는 페어링 모드, 셋업 때 기록한 기기별 passkey로 LE Secure Connections Passkey Entry 본딩 [N27] |
| `secure_channel` | non-secure | 결제 세션 보안 채널: 1회용 키 교환, `kioskKeySignature` 확인, HKDF 세션 키(`session.key`), AES-GCM (9주차) [N27] |
| `phone_link` | non-secure | 본딩한 폰 앱 링크로 `confirm.show`와 최종 결과 전달 [N26] |
| `session` | non-secure | session.open(mode)/confirm/cancel, sessionId, deviceNonce 관리 |
| `setup_flow` | non-secure | 셋업 세션, `setup.operator`, `setup.timeAnchor`, `device.reset` 처리 [N23] |
| `pay_flow` | non-secure | identify/prepare/result 상태 기계, `confirm.show` 요청, 버튼 대기 |
| `eip712` | non-secure | 스키마 타입의 encodeType·hashStruct·digest 계산 |
| `verifier` | non-secure | MerchantAttestation, MerchantOrder, TimeAnchor, DeviceReset 서명 복구와 비교 [N05][N06] |
| `indicator` | non-secure | LED 4개로 상태와 PIN 입력을 안내한다. 버튼 입력은 secure 쪽이 받는다 [N28] |
| `key_service` | TF-M secure partition | 키 생성·서명·삭제, 버튼 토큰, PIN 카운터, 셋업 값과 anchor 저장 [N02] |
| MCUboot | 부트로더 | 서명 이미지 확인, 유선 serial recovery [N12] |

## 2. 서명 권한 경계

`key_service`는 PSA 호출 몇 개만 연다. `key_generate`, `register_digest(digest, purpose)`, `sign_digest(digest, purpose)`, `pin_set`, `setup_store`, `anchor_store`, `anchor_read`, `reset_all` [N24].

1. `pay_flow`는 버튼을 기다리기 직전에 `register_digest`로 서명할 digest와 purpose(`payment` 또는 `limit_change`) 한 건을 secure 쪽에 등록한다.
2. 버튼은 secure 쪽 GPIO 인터럽트가 받는다. 인터럽트는 등록된 digest·purpose 한 건에만 1회용 토큰을 발급한다. `indicator`와 `phone_link`는 안내와 전달만 하고 토큰을 만들 수 없다.
3. `sign_digest(digest, purpose)`는 등록된 값과 같고 토큰이 유효할 때만 서명하고, 서명 뒤 등록을 지운다. 등록되지 않은 purpose, 다른 digest, 재사용 토큰은 거절한다.
4. purpose가 `limit_change`이면 `sign_digest` 안에서 secure 쪽이 기기 버튼으로 입력한 PIN을 확인하고 재시도 카운터를 관리한다. pinMaxRetries를 넘으면 PIN_LOCKED가 되어 모든 purpose를 거절한다.
5. 페어링 모드 진입, PIN 입력, 셋업 확인에 쓰인 버튼 입력은 서명 토큰이 되지 않는다. 기기에 화면이 없어 Numeric Comparison은 쓰지 않는다 [N26].

이 경계는 non-secure 코드가 뚫려도 버튼 없이 서명이 나오거나 등록되지 않은 purpose로 서명하는 것을 막는다. 그러나 "표시한 내용과 서명한 내용이 같다"는 보장은 digest와 `confirm.show` 필드를 non-secure 코드가 만들고 폰 앱이 그대로 표시한다는 가정 아래의 주장이다 [N26]. trusted display는 범위 밖이며, 이 경우에도 손실은 컨트랙트 한도로 제한된다.

`key_service`는 nRF54L15의 TrustZone 위 TF-M secure partition에서 키를 만들고 서명한다 [N02]. secp256k1 키는 칩의 KMU가 받지 않는 것으로 보여(NCS `cracen_psa_kmu.c`는 P-256, Ed25519, X25519만 다룬다) TF-M 보호 저장소(ITS)에 암호화해 두고, 서명할 때 secure RAM으로 불러와 CRACEN으로 서명한 뒤 지운다. 서명(r, s)의 low-s 정규화와 recovery id(v) 계산도 secure partition이 한다. keccak-256과 서명자 복원(운영자·가맹점 서명 확인, v 계산)은 소프트웨어로 한다. 복원은 libsecp256k1 v0.8.0의 recovery 모듈(MIT), keccak은 Keccak 팀의 compact 구현(CC0)을 커밋 고정으로 `third_party`에 두고 쓴다(2026-10-01 결정). 코드는 `core/src/nu54_eip712.c`, `nu54_sig.c`, `nu54_keccak.c`이고 host에서 공용 벡터로 시험한다. 외부 secure element는 2026-09-29에 컷했고, 칩 덤프·fault injection·부채널 같은 물리 공격 방어가 약한 점은 waiver로 기록한다 [N14]. ITS 암호화와 AP-Protect(WBS2-P01-07)가 켜져 있어야 이 설계가 성립한다.

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

- **발견과 페어링:** 기기는 결제 모드에서만 결제용 광고를 한다. 이 보드는 NFC 핀을 I2C로 쓰고 안테나가 없어 NFC handover를 쓰지 않는다 [N29]. 폰 앱과 운영자 도구는 페어링 모드에서 LE Secure Connections Passkey Entry로 본딩하고, 키오스크는 페어링 없이 결제 세션만 연다 [N27]. 셋업 세션과 `confirm.show`는 본딩한 링크에서만 처리한다.
- **보안 채널:** 9주차부터 결제 세션은 `secure_channel`로 연다. `session.open`의 attestation과 `kioskKeySignature`를 확인한 뒤 1회용 키로 `session.key`를 만들고, 이후 본문을 AES-GCM으로 복호화·암호화한다. 7주차 게이트는 평문으로 통과한다 [N27][N30].
- **재조립:** 모든 envelope에 조각 헤더가 붙는다. 협상된 MTU에서 조각 크기 `ATT_MTU - 5`를 계산한다. sequence가 기대값과 다르거나 index가 건너뛰거나 digest가 다르거나 본문이 2048바이트를 넘으면 버퍼를 비우고 `error{BAD_FRAME}`을 보낸 뒤 세션을 닫는다. CBOR 본문은 결제 프로토콜 4.2절의 필드 인코딩 규칙(결정적 순서, 고정 길이 byte string, uint 표현)으로 해석하고, 규칙을 어기면 같은 `BAD_FRAME`으로 처리한다.
- **setup.operator:** UNPROVISIONED에서만 받는다. 운영자 도구가 operator, contract, chainId를 보여 주고, 기기는 LED로 확인 대기를 알린 뒤 대여자 버튼 확인을 받아 RAM에 보관하고 `setup.ack{step: setup.operator}`를 보낸다. 운영자 값은 키 생성과 함께 `setup_store`로 한 번에 저장하므로 keygen ack 전에 세션이 끊기면 아무것도 남지 않는다. 이어서 키 생성과 버튼으로 하는 PIN 설정(LED가 자릿수 안내) [N28], 페어링 passkey 기록이 끝나면 `setup.ack{step: keygen, device}`로 새 주소를 보낸다. UNPROVISIONED에서는 키가 없으므로 `session.open.ok`에 device를 넣지 않는다 [N23].
- **setup.timeAnchor:** 셋업 세션에서만 받는다. `device`가 자기 주소인지, 서명자가 운영자 주소인지, timestamp가 이전 값보다 엄격히 늦은지 확인한 뒤 `anchor_store`를 호출하고 `setup.ack{step: setup.timeAnchor, accepted, lastAnchor}`를 보낸다 [N06].
- **device.reset:** DeviceReset `{device, nonce}` digest로 서명자를 복구해 운영자 주소와, device를 자기 주소와 비교한 뒤 `reset_all`을 호출한다.
- **identify:** `verifier`가 attestation digest를 계산하고 서명자를 복구해 운영자 주소와 비교한다. anchor 시간과 `validFrom..validUntil`을 anchorClockSkew를 허용해 비교한다.
- **prepare:** MerchantOrder digest로 가맹점 서명자를 복구해 attestation의 merchant와 비교한다. authorization.merchant가 attestation의 merchant와 같은지, payout이 attestation·주문과 같은지, orderId·token·amount·expiry가 주문과 같은지, chainId·contract가 셋업 값과 같은지 확인한다. expiry가 현재부터 authorizationExpiry 안인지도 확인한다. `confirm.show` 필드는 authorization 필드와 attestation의 가맹점 이름에서만 만들어 본딩한 폰 앱으로 보낸다 [N26].
- **서명:** 기기가 nonce를 골라 PaymentAuthorization digest를 만들고 `register_digest` 후 버튼 토큰으로 `sign_digest`를 부른다. 서명과 nonce를 `payment.result{approved}`로 보낸다 [N04].
- **session.cancel:** 등록한 digest를 지우고 버튼 대기를 멈춘다. 아무것도 서명하지 않는다.
- **서명 뒤 링크 끊김:** `payment.result`를 보낸 뒤 BLE가 끊기면 키오스크는 이미 받은 서명으로 제출을 계속한다. 기기는 아무것도 하지 않으며 같은 주문에 다시 서명하지 않는다.
- **payment.outcome:** 키오스크의 최종 결과(approved, refused, failed, Checking)를 LED로 알리고 폰 앱에 전달한다.

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

MERCHANT_REVOKED, OVER_CAP, NONCE_REPLAYED는 컨트랙트가 만든다. 기기는 이 코드를 내지 않고 `payment.outcome`으로 받은 결과를 LED와 폰 앱으로 알린다.

## 7. 보안

- 서명은 등록된 digest·purpose에 대한 secure 쪽 버튼 토큰이 있어야만 나온다 [N24]. 키 반출 명령은 어느 전송(BLE, UART)에도 없다.
- AP-Protect를 켜고 release 빌드에서 디버그 로그의 키 관련 출력을 막는다. W12 릴리스 이미지에는 USB CDC 시험 harness를 넣지 않는다. NVM 덤프 시험은 AP-Protect를 켜기 직전의 같은 image hash로 한다.
- MCUboot는 오프라인에 둔 서명 키로 서명한 이미지만 부팅하고, 갱신은 UART 위 SMP를 쓰는 유선 serial recovery로만 받는다. BLE 전송 SMP와 BLE DFU는 넣지 않는다 [N12].
- 운영자 주소와 셋업 값은 대여 셋업마다 버튼 확인으로 한 번 쓰고, 운영자 서명 DeviceReset 전에는 바꿀 수 없다 [N23]. LESC 페어링만으로는 셋업 권한이 생기지 않는다. 페어링하지 않은 키오스크 링크에서는 셋업 세션과 `confirm.show`를 처리하지 않는다 [N27].
- 기기는 오프라인이어서 registry의 가맹점 철회를 알 수 없다. 이 공백은 컨트랙트가 막는다 [N05].

## 8. waiver 설계

설계만 하고 이번 사이클에 구현하지 않는 항목이다 [N14].

- **req 10 anti-exfil:** ECDSA 서명 nonce는 RFC 6979 결정론적 nonce로 만들어, 같은 digest에는 같은 서명이 나오게 한다. 이렇게 하면 호스트가 서명을 여러 번 받아 비교해 펌웨어가 nonce로 키를 흘리는지 검사할 수 있다. 호스트 엔트로피를 섞는 anti-exfil 프로토콜(호스트 commit 후 기기 nonce 공개)은 키오스크·P05 쪽 구현과 프로토콜 확장이 필요해 보류한다.
- **req 13 정품 기기 attestation:** 공장 provisioning 때 TF-M에 기기 증명 키를 넣고, 셋업 때 기기 주소를 이 키로 서명해 운영자가 확인하는 방식이 설계안이다. 이번 사이클에는 공장 provisioning 절차가 없어 보류하며, P05는 셋업 세션에서 기기가 `setup.ack{step: keygen, device}`로 보고한 주소를 그대로 믿는다.
- **req 12 의존성 pinning:** west manifest의 NCS·모듈 revision을 커밋 hash로 고정하고 빌드 도구의 lockfile을 두는 것이 설계안이다. 이번 사이클에는 재현 빌드 검증 절차를 만들지 않아 보류하고, 사용한 버전은 week12-log 1절의 도구·의존성 버전 행에 기록한다.

### 7주차 개발 빌드에만 있는 것 (2026-10-01)

7주차 실결제 게이트는 "보드 내장 키로 버튼 승인 결제 1건"을 요구하고, 결정(2026-09-27)에 따라 non-secure 경로로 먼저 통과한다. 아래는 그 개발 빌드에만 있는 예외이며, 8주차 `/ns` 변형과 셋업 세션 구현에서 없앤다.

- **키:** PSA Crypto(CRACEN)가 secp256k1 키를 만들어 Zephyr Secure storage(ZMS)에 둔다. 서명은 결정론적 ECDSA(RFC 6979)로, 위 req 10의 결정론적 nonce 설계를 따른다. secure 쪽 버튼 토큰 경계는 아직 없다.
- **셋업:** 운영자 주소, 정산 컨트랙트, chainId는 `setup.operator` 대신 빌드 때 `deployments/8283.json`에서 생성한다(`app/gen_dev_setup.py`). TimeAnchor는 페어링하지 않은 링크의 셋업 세션으로도 받는다. 이 예외는 고정 셋업으로 프로비저닝된 기기에만 있고(`CONFIG_NU54_DEV_SETUP`), 그 세션에서도 `setup.operator`는 받지 않으며, reset 뒤 UNPROVISIONED 기기는 페어링하지 않은 셋업 세션을 거절한다(세션 벡터 SV-34).
- **폰 확인 화면:** 2026-10-03부터 본딩한 폰 앱 링크로 `confirm.show`, `confirm.limit`, 전달하는 `payment.outcome`을 보낸다(`app/src/pay_link.c`, 연결 2개). 본딩하고 TX를 구독했으며 아무것도 쓰지 않은 central을 폰 앱으로 본다. 개발 빌드는 폰 앱이 없으면 로그만 남기고 버튼을 기다리고(N30), 릴리스 빌드(`CONFIG_NU54_REQUIRE_PHONE`)는 결제와 한도 변경을 `refused{NOT_PERMITTED}`로 답한다(세션 벡터 SV-30). 셋업 세션은 본딩한 링크에서만 열린다(SV-31).
- **버튼과 LED(임시 배치):** SW4 길게 = 결제 모드(120초 광고), SW3 길게 = 페어링 모드(60초, 셋업 때 기록한 passkey로 Passkey Entry, UNPROVISIONED에서는 Just Works), SW1 = 승인, SW2 = 거절. 표시(2026-10-05, `app/src/status_led.c`만 LED를 켠다): LED 네 개가 붙어 있어 구분이 어려우므로 하나의 표시등으로 함께 켜고, 단계는 깜빡임 패턴으로 구분한다. 대기 = 꺼짐, 결제 모드 = 2초마다 짧게 한 번, 대여자 버튼 대기 = 빠른 깜빡임(0.2초), 서명함·키오스크 결과 대기 = 느린 깜빡임(0.5초, 최대 30초), 승인 = 2초 켜진 뒤 꺼짐, 거절·실패·취소(기기 거절, 키오스크 실패, 대기 중 연결 끊김) = 짧게 세 번 뒤 꺼짐. 새 세션을 열면 이전 결과 표시를 지운다. 기기는 응답마다 서명함·거절함 이벤트를 따로 보고하므로(보안 채널에서는 보드가 응답을 읽을 수 없다) 세션 벡터의 모든 단계가 이 이벤트까지 검증된다. PIN을 물을 때(셋업 키 생성, 한도 변경)는 버튼이 PIN 입력으로 바뀐다(2026-10-03, PIN 4자리): SW1을 누른 횟수가 숫자(0~9, 열 번째에 0), SW3 = 이 자리 확정, SW2 = 처음부터 다시. 표시등은 입력 시작에 길게 두 번, SW1마다 짧게 한 번, SW3 확정마다 0.5초 한 번 깜빡인다. 네 번째 확정으로 끝나고, 시작부터 45초가 지나면 시간 초과다(`core/src/nu54_pin_entry.c`). PIN 숫자는 로그에 남기지 않는다.
- **nonce 카운터:** 첫 부팅 때 256 배수 시작값을 TRNG로 정하고 Zephyr settings에 둔다. 서명하기 전에 다음 값을 먼저 저장한다(결제 프로토콜 2절).

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

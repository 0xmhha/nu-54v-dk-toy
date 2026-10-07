# 결제 프로토콜 v1

NU-54V-DK 기기(P01), 키오스크(P04), 운영 백오피스(P05), 정산 컨트랙트(P06)가 주고받는 메시지와 서명 형식을 정한다. 이 문서는 규칙을 설명하고, 타입과 필드는 [payment-protocol.schema.json](payment-protocol.schema.json)이, 서명 해시의 정답은 [eip712-vectors.json](eip712-vectors.json)이 정한다 [N21]. 값(한도, 유효 기간, 가스 잔액, 시계 오차)은 [DF-20260925-02](../../planning/design-freeze-checkpoint-02.md)의 parameters에만 있고 여기서는 이름으로만 부른다 [N13].

## 1. 누가 무엇을 믿는가

| 주체 | 가진 키 | 서명하는 것 | 검증하는 쪽 |
|---|---|---|---|
| 기기(P01) | 대여 셋업 때 만든 기기 키 | PaymentAuthorization, LimitChange [N04] | 컨트랙트 |
| 운영자(P05) | 운영자 키 | MerchantAttestation, TimeAnchor, DeviceReset [N05][N06][N23] | 기기 |
| 가맹점 | 가맹점 서명 키(키오스크에 둔다) | MerchantOrder | 기기 |
| 키오스크(P04) | 가스용 키오스크 키, 가맹점 서명 키 | 트랜잭션 제출, 가맹점 대리 MerchantOrder | 체인, 기기 |
| 대여자 폰 앱(P02) | 없음(기기와 본딩만 한다) | 없음. 기기가 보낸 결제 필드를 표시만 한다 [N26] | — |

이번 사이클에서 키오스크는 가맹점의 POS이므로 가맹점 서명 키를 갖고 가맹점을 대리한다. 따라서 키오스크가 뚫리면 가맹점이 뚫린 것과 같다. 그래도 기기는 운영자가 서명한 attestation의 payout과 다른 곳으로는 서명하지 않으므로, 뚫린 키오스크가 할 수 있는 일은 대여자가 버튼으로 승인한 금액을 등록된 payout으로 결제받는 것뿐이고 손실은 한도로 제한된다.

온체인 registry가 가맹점의 최종 권한이며, 오프라인 기기가 모르는 가맹점 철회는 컨트랙트가 MERCHANT_REVOKED로 막는다 [N05]. 키오스크는 기기와 페어링하지 않는다. 카드 리더기에 카드를 대는 것처럼, 근처의 central은 누구든 결제 세션을 열 수 있다. 보안 채널을 적용하기 전(7주차 게이트)에는 기기가 서명 규칙, 폰 확인 화면, 버튼 승인으로만 자신을 지키고, 적용한 뒤에는 4.1절의 보안 채널이 결제 세션을 암호화하고 키오스크를 가맹점 키로 인증한다 [N27]. 셋업과 reset 권한은 5절의 규칙으로만 생긴다 [N23].

## 2. 서명 타입

기기는 결제용 EIP-712 타입 두 개(PaymentAuthorization, LimitChange)와 지갑 확인용 WalletCheck 하나에만 서명한다 [N04]. 그 밖의 요청(원시 트랜잭션, approve, Permit, 스키마에 없는 메시지)은 `error{UNSUPPORTED_TYPE}`으로 거절한다.

- **PaymentAuthorization** `{chainId, contract, merchant, payout, token, amount, orderId, nonce, expiry}` — 결제 1건. 키오스크는 nonce를 뺀 필드를 보내고, 기기가 nonce를 골라 서명과 함께 돌려준다. nonce는 재사용 방지용이며 예측할 수 없을 필요가 없다(서명이 없으면 누구도 그 nonce를 쓸 수 없다). 기기는 셋업 때 256으로 나누어떨어지는 시작값을 무작위로 한 번 정하고, 이후 결제와 한도 변경마다 1씩 늘린 순차 nonce를 쓴다. 컨트랙트의 nonce bitmap은 연속한 256개 nonce를 storage slot 하나에 담으므로, 순차 nonce는 무작위 nonce보다 결제당 약 17,000 gas가 적다(2026-10-01 sandbox 실측 93,906 대 111,378). 기기는 카운터를 서명 전에 플래시에 먼저 기록한다. 카운터를 잃어 이미 쓴 nonce를 다시 쓰면 컨트랙트가 `NonceReplayed`로 거절하므로 자산 손실은 없고, 기기는 카운터를 다음 256 구간으로 넘겨 이어 쓴다. ECDSA 서명 내부의 난수 k는 이 nonce와 별개이며 서명 구현(결정론적 k 또는 하드웨어 난수)이 맡는다. `expiry`는 현재 시각부터 `authorizationExpiry` 안이어야 한다 [N13].
- **LimitChange** `{chainId, contract, perPaymentLimit, dailyLimit, nonce, expiry}` — 대여자가 자기 한도를 register 상한(`perPaymentCap`, `dailyCap`) 안에서 정한다. 한도 0은 "상한을 그대로 적용"이다. nonce는 결제와 같은 순차 카운터에서 받으며, `expiry`는 `authorizationExpiry` 안이어야 한다. 기기에서 PIN과 버튼을 모두 요구하고, 키오스크가 `setLimits`로 제출한다 [N11].
- **WalletCheck** `{device, challenge}` — 본딩한 폰 앱이 새 지갑을 확인할 때 쓴다(5절 지갑 확인). 스키마의 `deviceCheckTypes`에 따로 두며, 정산 컨트랙트에는 이 타입이 없어 이 서명으로 자금을 움직일 수 없다. `challenge`는 폰 앱이 고른 32바이트 난수이고, 기기는 승인 버튼을 누른 뒤에만 서명한다. 벡터는 [eip712-vectors.json](eip712-vectors.json)의 WC-01이다.

운영자와 가맹점이 서명하는 타입은 `operatorSignedTypes`에 있다. MerchantAttestation `{merchant, payout, name, validFrom, validUntil}`의 유효 기간은 `attestationValidity`다 [N05]. MerchantOrder `{orderId, token, amount, payout, expiry}`는 가맹점 키가 주문 내용을 보증한다. TimeAnchor `{device, timestamp}`와 DeviceReset `{device, nonce}`는 5절에서 쓴다.

EIP-712 domain은 `{name: "NU54 Payment Settlement", version: "1", chainId: 8283, verifyingContract: 정산 컨트랙트}`다. 기기는 이 domain의 chainId와 verifyingContract를 셋업 때 기록한 값으로만 만든다. 시험 벡터는 역할마다 다른 키(기기, 운영자, 가맹점)로 서명되어 있어, 서명자 역할을 뒤섞는 구현은 벡터를 통과하지 못한다 [N21].

## 3. 연결과 페어링

BLE GATT가 유일한 규범 전송이다 [N09]. 기기가 peripheral이고, central은 셋이다. 대여자 폰 앱과 운영자 도구는 기기와 본딩하고, 키오스크는 페어링하지 않는다. 서비스에는 central→기기 `rx`(write)와 기기→central `tx`(notify) 두 characteristic이 있다. 결제 세션 메시지는 페어링하지 않은 링크에서도 받고, 셋업 세션과 `confirm.show`는 본딩한 링크(LE Secure Connections)에서만 주고받는다. UUID는 스키마의 `gatt` 항목을 따른다.

1. **발견.** central은 서비스 UUID로 BLE scan을 해서 기기를 찾는다. 결제 모드와 페어링 모드는 광고가 다르다(2026-10-05). 결제 모드는 그 순간 결제할 키오스크만 찾으면 되므로 발견 플래그·이름·기기 종류 없이 서비스 UUID만 광고한다. 페어링 모드는 대여자가 폰에서 찾아야 하므로 LE Limited Discoverable 플래그, 서비스 UUID, 짧은 이름(`NU54-HW-`)을 광고 패킷에 넣고, 스캔 응답에 전체 이름 `NU54-HW-Wallet`과 기기 종류(Appearance 0x0240)를 넣는다. 어느 모드든 passive scan(광고 패킷만)과 active scan(스캔 응답까지) 모두에서 서비스 UUID로 찾을 수 있다. 기기는 본딩한 폰 앱이 결제 모드를 켜거나 대여자가 기기 버튼으로 결제 모드에 들어갔을 때만 결제용 광고를 한다. 폰 앱은 본딩한 링크로 `device.paymentMode{on, seconds}`(sessionId는 0)를 보내 결제 모드를 켜고(1~300초) 끄며, 기기는 `device.paymentMode.ack{accepted, on}`으로 답한다. 본딩하지 않은 링크, `UNPROVISIONED` 기기, 범위 밖의 초는 `NOT_PERMITTED`다. 이 메시지는 세션에 속하지 않으므로 열린 결제 세션(보안 채널 포함)을 건드리지 않는다(세션 벡터 SV-32, SV-33). 이 보드는 NFC 핀을 I2C로 쓰고 NFC 안테나가 없어 NFC handover를 쓰지 않는다 [N29]. 페어링 모드의 광고에는 서비스 UUID와 LE Limited Discoverable 플래그가 있고 결제용 광고에는 이 플래그가 없으므로, 폰 앱은 이 둘로 페어링 모드의 기기만 찾는다. 본딩한 폰 앱은 `device.info`(sessionId는 0)로 기기 상태를 묻고, 기기는 `device.info.ack{state, firmware, anchorValid, device}`로 답한다(`UNPROVISIONED`이면 `device`가 없다). 본딩하지 않은 링크에는 `error{NOT_PERMITTED}`로 답한다(SV-36~SV-38).
2. **폰 앱과 운영자 도구의 페어링.** LE Secure Connections Passkey Entry로 본딩한다. 기기에 화면이 없어 Numeric Comparison은 쓰지 않는다 [N26]. passkey는 기기별 6자리이며, 대여 셋업을 하는 쪽이 정해 `setup.operator`로 기기에 기록한다. 대여자 폰 앱이 셋업하면 앱이 난수로 정해 페어링 코드로 한 번 보여 주고 저장하지 않으며, 운영자 도구가 셋업하면 기기 라벨의 QR 코드(BLE 주소와 passkey)로 인쇄한다. 기기는 버튼을 길게 눌러 페어링 모드에 들어갔을 때만 새 본딩을 받는다. passkey가 아직 없는 `UNPROVISIONED` 기기의 첫 셋업은 운영 장소에서 Just Works 본딩과 기기 버튼 확인으로 하고, 이 셋업에서 기록한 passkey가 이후의 본딩(폰 앱, 재-anchor)에 쓰인다. 페어링만으로는 셋업 권한이 생기지 않는다 [N23][N27]. 셋업이 끝나면 Just Works 본딩은 더 이상 본딩한 링크로 인정되지 않으므로, 기기는 새 passkey로 페어링 모드를 120초 동안 스스로 연다. 셋업한 폰 앱은 버튼을 다시 길게 누르지 않고 passkey 본딩으로 바꾼다. 기기는 새 본딩이 끝나면 이전 본딩을 모두 지우고(주인 한 명) 페어링 모드를 닫는다. 예전 폰과 운영자 도구는 다시 본딩해야 셋업 세션과 `confirm.show`를 주고받는다.
3. **키오스크 연결.** 키오스크는 페어링하지 않고 연결한다. 연결은 신호 세기(RSSI)가 기준값 이상인 기기에만 하며, 기준값은 8주차에 실측해 이 절에 적는다. 기기는 이 링크에서 결제 세션(6절)과 한도 변경만 받는다 [N27].
4. **폰 확인 화면.** 결제 세션 중 기기는 서명할 필드를 본딩한 폰 앱 링크로 `confirm.show`에 담아 보내고, 폰 앱은 그 값만 표시한다. 폰 앱은 승인 버튼을 두지 않으며 승인은 기기 버튼으로만 한다 [N26]. 폰 앱이 연결되어 있지 않으면 12주차 릴리스 빌드는 `payment.prepare`와 `limit.change`의 검사를 마친 뒤 확인 화면을 보내는 대신 `refused{NOT_PERMITTED}`로 답한다(세션 벡터 SV-30). 폰 앱이 나오기 전의 개발 빌드(7주차 게이트)는 표시 없이 진행한다 [N30]. 본딩하지 않은 central의 셋업 `session.open`은 `error{NOT_PERMITTED}`다(SV-31). 7주차 개발 빌드만 예외로, 고정 셋업으로 프로비저닝된 기기는 키오스크가 TimeAnchor를 넘기도록 페어링하지 않은 셋업 세션을 열지만 그 세션에서 `setup.operator`는 받지 않는다(SV-34).
5. **USB CDC.** 개발용 시험 harness로만 쓰고 게이트 증거로 인정하지 않는다. W12 릴리스 이미지에는 넣지 않는다.

## 4. 메시지 틀

모든 메시지는 deterministic CBOR map이다. 전송 전에 다음 envelope로 감싼다.

`length(u16, big-endian) | digest(CBOR 본문 SHA-256의 앞 8바이트) | CBOR 본문`

`length`는 헤더 10바이트(`length` 2바이트와 digest 8바이트)를 포함한 envelope 전체 길이다. 본문이 최대 2048바이트이므로 `length`는 10 이상 2058 이하이며, 이 범위를 벗어나면 `BAD_FRAME`이다.

모든 envelope는 조각 헤더를 붙여 보낸다(조각이 하나여도 붙인다). 각 조각은 `sequence(u8) | index(u8) | 데이터`이며 데이터 길이는 협상된 MTU에서 `ATT_MTU - 5`까지다. `sequence`는 방향별 메시지 번호로 메시지마다 1씩 늘고, `index`는 0부터 센다. 받는 쪽은 `length`만큼 모일 때까지 이어 붙인 뒤 digest를 확인한다. 순서가 어긋나거나 digest가 다르거나 본문이 2048바이트를 넘으면, 받은 쪽(기기든 central이든)이 메시지를 버리고 `error{BAD_FRAME}`을 보낸 뒤 세션을 닫는다. 받는 쪽의 규칙은 다음과 같다. 메시지는 index 0으로 시작하고, 메시지 안에서 sequence는 같으며 index는 1씩 는다. 메시지와 메시지 사이의 sequence는 비교하지 않는다. 모든 조각은 데이터를 1바이트 이상 담는다. length는 10~2058이고 그것을 넘는 바이트는 거절한다. digest는 envelope가 다 모인 뒤 확인한다. 정상 분할(ATT_MTU 23, 185, 247)과 거절해야 하는 조각열은 [frame-vectors.json](frame-vectors.json)에 있다. 세션 단위로 기기가 메시지마다 돌려줘야 할 바이트는 [session-vectors.json](session-vectors.json)에 있다(5, 6절의 시나리오). 키오스크가 아직 서명을 받지 못했으면 새 세션에서 처음부터 다시 시작한다. 이미 서명을 받았으면 세션 없이 7절의 제출 단계로 계속 간다(서명은 키오스크에 있으므로 새 서명을 요청하지 않는다).

### 4.2 CBOR 필드 인코딩 규칙

스키마는 필드를 JSON 형식(`0x…` hex 문자열, 10진 문자열)으로 적는다. BLE로 보낼 때는 아래 규칙으로 CBOR에 옮긴다. 기기, 키오스크, 폰 앱, 운영 도구가 같은 바이트를 만들어야 envelope digest와 교차 시험 벡터가 맞는다.

| 스키마 타입 | CBOR 표현 | 비고 |
|---|---|---|
| 메시지와 중첩 객체 | map (major type 5), 키는 스키마 속성 이름 그대로의 text string | 키 순서는 RFC 8949 4.2.1절의 core deterministic 규칙(인코딩한 키 바이트의 사전순)을 따른다 |
| `hex20`(주소) | byte string 20바이트 | hex 문자열을 디코딩한 값. 대소문자 차이는 사라진다 |
| `hex32`(orderId, nonce, digest 등) | byte string 32바이트 | |
| `signature` | byte string 65바이트, `r ‖ s ‖ v`(v는 27 또는 28) | |
| `sessionId` | byte string 8바이트 | 스키마의 16자리 hex |
| `uint` | 2^64 미만이면 unsigned integer(major type 0), 이상이면 tag 2 bignum(앞자리 0 없는 big-endian) | 10진 문자열로 보내지 않는다 |
| `v`, 정수 필드 | unsigned integer | |
| 문자열(`type`, 이름, enum 값) | text string(UTF-8) | |
| boolean | `true`/`false` | |

정수와 길이는 가장 짧은 형식으로 인코딩한다. 부정 길이(indefinite length), 부동소수점, tag 2 외의 tag, 중복 키, 스키마에 없는 키는 쓰지 않는다. 받은 쪽은 이 규칙을 어긴 본문을 `BAD_FRAME`으로 거절한다. 메시지 8개의 기대 바이트는 [cbor-vectors.json](cbor-vectors.json)에 있고(`packages/protocol/cbor-gen/generate.py`로 생성), 모든 구현은 같은 바이트를 만들어야 한다. 로그와 운영 도구 출력, EIP-712 JSON은 스키마의 JSON 형식을 그대로 쓴다.

### 4.1 결제 세션 보안 채널 (기기 9주차, 키오스크 10~11주차 적용)

키오스크 링크는 페어링하지 않으므로 결제 세션을 응용 계층에서 보호한다 [N27]. 7주차 실결제 게이트는 이 채널 없이 평문으로 통과한다. 기기는 9주차, 키오스크는 10~11주차에 적용하고, 양쪽이 적용된 뒤 릴리스 빌드의 기기는 평문 결제 세션을 거절한다 [N30].

1. 키오스크는 세션마다 secp256k1 1회용 키 쌍을 만든다. 공개키는 x 좌표 32바이트로만 보내고(`hex32`), 받는 쪽은 y가 짝수인 점으로 복원한다. ECDH 결과의 x 좌표는 y의 부호와 무관하므로 양쪽이 같은 값을 얻는다. 결제 모드 `session.open`에 1회용 공개키(`kioskEphemeral`), 가맹점 attestation(`attestation`, payment.identify와 같은 형식), 그리고 가맹점 키로 EIP-712 `KioskKey{merchant, kioskEphemeral, kioskNonce}`에 한 서명(`kioskKeySignature`)을 함께 담는다. 셋 중 하나만 있으면 안 된다. 서명 형식은 스키마의 `operatorSignedTypes.KioskKey`, 벡터는 [eip712-vectors.json](eip712-vectors.json)의 KK-01이다.
2. 기기는 attestation 서명자가 기록된 운영자인지, `kioskKeySignature` 서명자가 attestation의 `merchant`인지, `kioskEphemeral`이 곡선 위의 점인지 확인한다. 하나라도 아니면 `error{MERCHANT_FORGED}`로 답하고 세션을 열지 않는다. attestation의 유효 기간은 시각 anchor가 있어야 판단할 수 있으므로 여기서 보지 않고 `payment.identify`에서 본다. 셋업 모드에 이 필드를 담거나 `UNPROVISIONED` 기기(대조할 운영자가 없다)에 보내면 `error{NOT_PERMITTED}`다. 통과하면 deviceNonce를 뽑은 다음 자기 1회용 키를 뽑고(유효한 스칼라가 아니면 다시 뽑는다), x 좌표를 `session.open.ok`의 `deviceEphemeral`에 담는다. `session.open`과 `session.open.ok`는 평문이다.
3. 두 쪽은 ECDH 공유값의 x 좌표(32바이트)에서 HKDF-SHA256(salt = `kioskNonce ‖ deviceNonce`, info = `nu54 session v1`)으로 128비트 세션 키(`session.key`)를 만든다.
4. `session.open.ok` 다음부터 세션이 끝날 때까지 이 링크의 모든 본문은 양방향 모두 AES-GCM(128비트 키) 암호문과 16바이트 태그를 이어 붙인 것이다(AAD 없음). IV 12바이트는 보내는 쪽 방향 1바이트(키오스크→기기 `0x01`, 기기→키오스크 `0x02`)와 방향별 메시지 번호 11바이트(big-endian, 0부터)다. envelope의 길이와 digest는 암호문 기준이고, 암호문은 태그를 포함해 2048바이트 이하다. 태그 검증이 실패하거나 암호문이 아닌 본문이 오면 기기는 `error{BAD_FRAME}`을 같은 채널로 암호화해 보내고 세션을 닫는다. 기기 오류 응답도 세션 안에서는 모두 암호화한다. 세션이 닫힌 뒤 도착한 암호문은 채널이 없으므로 평문 `BAD_FRAME`이다. 대여자 폰 앱으로 가는 `confirm.show`, `confirm.limit`, 전달하는 `payment.outcome`은 본딩한 링크로 가므로 평문 CBOR이다.
5. 기기는 세션 키를 만든 즉시 1회용 개인키와 공유값을 지우고, 세션이 끝나면(`session.cancel`, 오류, 링크 끊김, RAM 초기화) 세션 키를 지운다. 이전 세션의 `session.open`을 다시 보내도 1회용 개인키가 없으면 세션 키를 만들 수 없다. 보안 세션 중에 새 `session.open`을 평문으로 보내면 태그 검증에 실패해 세션이 닫히므로, 키오스크는 `session.cancel`로 세션을 끝낸 뒤 새로 연다.
6. 보안 세션의 `payment.identify`는 `session.open`에서 확인한 가맹점의 attestation이어야 한다. 다른 가맹점이면 운영자 서명이 맞아도 `MERCHANT_FORGED`다.
7. 평문 결제 세션은 개발 빌드만 받는다. 릴리스 빌드는 평문 결제 모드 `session.open`을 `error{NOT_PERMITTED}`로 거절한다(펌웨어 `CONFIG_NU54_REQUIRE_SECURE_SESSION`). 셋업 세션은 본딩한 링크에서 열리므로 평문 그대로다. 기대 바이트는 [session-vectors.json](session-vectors.json)의 SV-24~SV-29에 있고, `plain` 항목에 암호문 안의 CBOR 본문이 함께 있다.



기기 상태는 `UNPROVISIONED`, `PROVISIONED_NO_ANCHOR`, `READY`, `PIN_LOCKED` 넷이다. `session.open`의 `mode`가 `setup`이면 셋업 세션이고, 셋업 세션은 `UNPROVISIONED` 또는 `PROVISIONED_NO_ANCHOR`에서만 열린다. 다른 상태에서는 `error{NOT_PERMITTED}`로 거절한다 [N23].

**대여 셋업**은 다음 순서로 한다. 대상 기기는 반납 절차의 device.reset으로 `UNPROVISIONED` 상태다.

1. 운영자 도구가 셋업 세션을 연다. `UNPROVISIONED`에서는 키가 없으므로 `session.open.ok`에 `device`가 없다. 운영자 도구가 `setup.operator{operator, contract, chainId, passkey}`를 보낸다. `passkey`는 운영자 도구가 정한 기기별 페어링 코드(0~999999)이고 라벨 QR에 인쇄한다(3절). 기기에는 화면이 없으므로 운영자 도구가 값을 보여 주고, 기기는 LED로 확인 대기를 알린다. 대여자가 기기 버튼으로 승인하면 `setup.ack{step: setup.operator, accepted: true}`, 거절하면 `accepted: false, reason: USER_REJECTED`로 답한다. 이 기록은 `UNPROVISIONED`에서 한 번만 가능하며, 다른 상태이거나 passkey가 범위 밖이면 `accepted: false, reason: NOT_PERMITTED`다 [N23]. 기기는 이 값을 RAM에만 두었다가 2단계의 키·PIN과 함께 한 번에 저장하므로, keygen ack 전에 세션이 끊기면 아무것도 남지 않고 `UNPROVISIONED`로 다시 시작한다.
2. 승인 직후 기기는 TRNG로 새 키를 만들고, nonce 시작값을 정하고(31바이트 난수 뒤에 0x00을 붙인 32바이트 값, 즉 256의 배수, 2절), 대여자가 기기 버튼으로 4자리 숫자 PIN을 정한다. 기기는 버튼을 누를 때마다 입력 중인 PIN을 `pin.entry{digits, position}`(sessionId는 0)로 본딩한 폰 앱에 보내 화면에 보이게 한다(`digits`는 0000에서 시작해 SW1마다 현재 자리가 1씩 오르고 9 다음은 0, `position`은 입력 중인 자리 0~3, 네 자리를 모두 확정하면 4). PIN이 BLE를 거치는 위험은 수용했다 [N28]. LED가 입력할 자릿수와 누를 버튼을 안내한다 [N28]. PIN 입력은 시작부터 45초 안에 끝나야 한다(키오스크의 한도 변경 대기 60초 안에 버튼 승인 시간을 남긴다). PIN이 정해지면 운영자 값, passkey, 키, PIN, nonce 시작값을 함께 저장하고 `PROVISIONED_NO_ANCHOR`가 되어 `setup.ack{step: keygen, accepted: true, device}`로 새 주소를 알린다 [N11]. 운영자 도구는 setup.operator ack와 keygen ack를 차례로 받는다. PIN 입력이 시간 안에 끝나지 않으면 `setup.ack{step: keygen, accepted: false, reason: TIMEOUT}`을 보내고 아무것도 저장하지 않는다.
3. 운영자 도구는 받은 주소와 최신 finalized 블록 시각으로 TimeAnchor `{device, timestamp}`에 서명해 `setup.timeAnchor{device, timestamp, operatorSignature}`를 보낸다. 기기는 `device`가 자기 주소이고, 서명자가 기록한 운영자 주소이며, `timestamp`가 이전 anchor보다 엄격히 늦을 때만 받아들이고 `setup.ack{step: setup.timeAnchor, accepted, lastAnchor}`로 답한다. 받아들이지 않으면 `accepted: false`와 `reason: NOT_PERMITTED`를 보낸다 [N06].
4. 운영자 도구는 anchor가 받아들여진 뒤에만 `depositFor(device, amount, withdrawAddress)`를 호출한다.

대여자 폰 앱(P02)과 P05 운영 도구 `opsctl`이 같은 메시지로 1, 2단계를 한다. 폰 앱은 `setup.operator`에 배포 기록의 운영자 주소, 정산 컨트랙트, chainId를 넣는다. 3, 4단계(TimeAnchor와 예치)는 운영자 도구가 한다.

**지갑 확인.** 폰 앱은 keygen ack의 주소가 기기 키의 주소인지 `wallet.check{challenge}`(sessionId는 0)로 확인한다. 기기는 LED로 버튼 대기를 알리고, 승인 버튼을 누르면 WalletCheck `{device, challenge}`에 서명해 `wallet.check.result{accepted: true, signature}`를 본딩한 폰 앱에 보낸다. 거절 버튼이면 `USER_REJECTED`이고, 60초 안에 누르지 않거나 폰 앱 링크가 끊기면 `TIMEOUT`이다. 본딩하지 않은 링크, 키가 없는 기기, 다른 버튼 대기 중인 기기는 `NOT_PERMITTED`, `PIN_LOCKED` 기기는 `PIN_LOCKED`로 요청한 링크에 바로 답한다. 폰 앱은 서명자를 복원해 keygen 주소와 같을 때만 기기를 등록한다(SV-36, SV-37). 다른 폰에서 이미 셋업한 기기와 본딩한 경우에도 같은 확인을 거친다.

**재-anchor.** RAM이 초기화되는 모든 reset(전원 손실, watchdog, System OFF에서 깨어남, serial recovery 뒤 재부팅) 뒤에는 anchor가 무효가 되고 기기는 `PROVISIONED_NO_ANCHOR`가 된다. 키와 예치금은 그대로다. 이 상태에서 결제 요청은 `TIME_ANCHOR_MISSING`으로 거절하고, 운영자 도구가 셋업 세션을 열어 3단계(TimeAnchor)만 다시 하면 `READY`로 돌아간다. 예치는 다시 하지 않는다.

**시간 비교.** 기기의 현재 시각은 마지막 anchor에 RTC 경과 시간을 더한 값이다. attestation 유효 기간을 비교할 때만 `anchorClockSkew`를 허용한다.

**reset.** `device.reset{device, nonce, operatorSignature}`는 운영자가 DeviceReset `{device, nonce}`에 서명한 명령이며 어떤 상태에서든, 열려 있는 세션이면 mode와 관계없이 받는다. 서명자가 기록된 운영자 주소이고 `device`가 자기 주소일 때만 키, PIN, 운영자 주소, 컨트랙트 값, passkey, TimeAnchor, nonce 기록을 지우고 `UNPROVISIONED`가 되며 `setup.ack{step: device.reset, accepted: true}`로 답한다. 그렇지 않거나 지울 키가 없는 `UNPROVISIONED`이면 `accepted: false, reason: NOT_PERMITTED`다. 다시 셋업한 기기는 새 주소를 가지므로 이전 주소에 서명한 DeviceReset은 다시 쓸 수 없다. `nonce`는 운영자 도구가 명령을 구별하는 값이며 기기는 비교하지 않는다. 반납 순서(closeAccount의 finalized 이벤트 확인 → device.reset)는 운영자 도구가 지킨다 [N11].

**PIN 잠금.** PIN을 `pinMaxRetries`번 틀리면 `PIN_LOCKED`가 되어 모든 서명을 거절한다. 결제는 `payment.prepare`에서 `payment.result{refused, PIN_LOCKED}`로, 한도 변경은 `limit.result{refused, PIN_LOCKED}`로 답한다. 잠금 상태는 secure 저장소에 남아 reset 뒤에도 `PIN_LOCKED`이며, 풀 방법은 반납 절차(DeviceReset)뿐이다.

## 6. 결제 흐름

| 순서 | 메시지 | 방향 | 기기가 하는 일 |
|---|---|---|---|
| 1 | `session.open{sessionId, mode: payment, kioskNonce}` | 키오스크→기기 | `session.open.ok{device, deviceNonce, anchorValid, state, lastAnchor, firmware}`로 답한다 |
| 2 | `session.confirm{sessionId, deviceNonce}` | 키오스크→기기 | 자기가 보낸 deviceNonce와 같을 때만 세션을 연다 |
| 3 | `payment.identify{attestation}` | 키오스크→기기 | MerchantAttestation 서명자가 운영자인지, 현재 시각이 `validFrom..validUntil`(± `anchorClockSkew`) 안인지 확인하고 폰 앱에 보낼 가맹점 이름을 정한다 |
| 4 | `payment.prepare{authorization, merchantSignature}` | 키오스크→기기 | 아래 검사를 모두 통과하면 가맹점 이름·orderId·token·payout·amount를 `confirm.show`로 폰 앱에 보내고 버튼을 기다린다 |
| 5 | `payment.result{outcome, signature, nonce \| reason}` | 기기→키오스크 | 버튼을 누르면 nonce를 골라 서명하고 `approved`를, 거절 버튼이나 검사 실패면 `refused`와 reason을 보낸다 |
| 6 | `payment.outcome{orderId, outcome, reason, txHash, receipt}` | 키오스크→기기 | 키오스크의 최종 결과를 LED로 알리고, 같은 결제 세션의 것이면 받은 본문을 바꾸지 않고 폰 앱에 전달한다. 키오스크에는 답하지 않는다. 승인된 결제에는 정산 트랜잭션 `txHash`와 디지털 영수증 `receipt`가 붙는다 |

4단계 검사는 다음과 같다. 하나라도 틀리면 `refused`다.

- MerchantOrder 서명자가 attestation의 `merchant`이고, `authorization.merchant`도 attestation의 `merchant`다. 아니면 `MERCHANT_FORGED`.
- `authorization.payout`이 attestation과 MerchantOrder의 payout과 같다. 아니면 `MERCHANT_FORGED`.
- `authorization`의 orderId·token·amount·expiry가 MerchantOrder와 같고, chainId·contract가 셋업 때 기록한 값과 같다. 아니면 `MERCHANT_FORGED`.
- `expiry`가 현재 시각부터 `authorizationExpiry` 안이다. 아니면 `ATTESTATION_EXPIRED`.
- anchor가 유효하다. 아니면 `TIME_ANCHOR_MISSING`.

**한도 변경.** 키오스크가 확인을 마친 결제 세션에서 `limit.change{change}`를 보낸다. 기기는 다음 순서로 처리한다.

1. 기기가 `READY`가 아니면 거절한다. anchor가 없으면 `TIME_ANCHOR_MISSING`, `PIN_LOCKED`이면 `PIN_LOCKED`다.
2. `change`를 검사한다. chainId와 contract가 셋업 값과 다르면 `NOT_PERMITTED`, expiry가 기기 시각부터 `authorizationExpiry` 밖이면 `ATTESTATION_EXPIRED`다. 한도가 register 상한을 넘는지는 컨트랙트가 `OverCap`으로 확인한다.
3. 요청 내용을 본딩한 폰 앱에 `confirm.limit{perPaymentLimit, dailyLimit, expiry}`로 보낸다.
4. 대여자가 기기 버튼으로 PIN을 입력한다(N28). 시간 안에 입력하지 않으면 `TIMEOUT`이다. 틀리면 `NOT_PERMITTED`이고 실패 횟수가 늘어난다. 실패가 `pinMaxRetries`번이 되면 기기는 `PIN_LOCKED`가 되고 `PIN_LOCKED`로 답한다. 맞으면 실패 횟수를 0으로 되돌린다.
5. 대여자가 승인 버튼을 누르면, 기기는 결제와 같은 순차 카운터에서 nonce를 받아 LimitChange에 서명하고 `limit.result{approved, signature, nonce}`로 답한다. 거절 버튼이면 `USER_REJECTED`다.

거절은 모두 `limit.result{refused, reason}`으로 보낸다. 키오스크는 승인된 서명을 `setLimits`로 제출하고, 컨트랙트는 expiry와 nonce를 결제와 같은 방식으로 확인한다 [N04]. 같은 서명을 다시 제출하면 `NonceReplayed`다(8절의 `NONCE_REPLAYED` 시연).

버튼 승인은 서명 권한 경계를 거친다. 기기는 서명할 digest와 purpose를 secure partition에 먼저 등록하고, secure 쪽 버튼 인터럽트는 그 digest 한 건에만 서명 토큰을 발급한다 [N24]. 폰 앱이 표시하는 값은 기기가 보낸 값이며, 표시 내용과 서명 내용이 같다는 보장은 기기의 non-secure 코드와 폰 앱이 무결하다는 가정 아래의 주장이다 [N26].

키오스크는 `payment.prepare` 전송이 끝난 뒤 10 s 안에 `payment.result`가 없으면 `session.cancel`을 보낸다. 기기는 버튼 대기를 멈추고 아무것도 서명하지 않는다. 키오스크는 주문을 취소하고, 서명이 없었으므로 다시 결제를 받아도 된다 [N10].

**디지털 영수증.** 결제가 승인되면 키오스크는 디지털 영수증을 만들어 화면에 보이고, 같은 내용을 `payment.outcome`의 `receipt`에 JSON 문자열로 실어 폰 앱에 보낸다. 기기는 이 본문을 해석하지 않고 그대로 전달하며, 전달할 수 있는 본문은 2048 바이트까지다(SV-35). 영수증 JSON은 다음 필드를 갖는다.

| 필드 | 뜻 |
|---|---|
| `v` | 형식 버전, 1 |
| `store` | 가맹점 이름(attestation의 `name`) |
| `representative`, `businessNumber`, `address`, `phone` | 키오스크 설정 `merchantProfile`에서 온 대표자, 사업자 번호, 주소, 전화. 없으면 생략 |
| `orderNumber` | 손님에게 보이는 주문 번호(예: `A-0001`, 날마다 다시 시작) |
| `orderId` | MerchantOrder의 orderId |
| `time` | 승인 시각(Unix 초) |
| `items` | `{name, qty, unitPrice}` 목록. `unitPrice`는 토큰 최소 단위의 10진 문자열 |
| `total` | 합계. `items`의 `qty × unitPrice` 합과 같아야 한다 |
| `token` | `{symbol, decimals}` |
| `chainId` | 정산 체인 |
| `payer` | 결제한 기기의 주소 |

영수증 문자열은 1600자를 넘지 않는다. 넘으면 키오스크는 영수증 없이 `payment.outcome`을 보내고 키오스크 화면에만 영수증을 보인다. 폰 앱은 형식이 틀린 영수증을 버리고 결과만 보인다. 탐색기 링크는 영수증에 넣지 않는다. 폰 앱과 키오스크는 `chainId`와 `txHash`로 링크를 직접 만든다(체인 8283이면 `https://explorer.stablenet.network/tx/<txHash>`). 받은 본문의 URL을 열지 않으므로, 본문을 바꿀 수 있는 쪽이 사용자를 다른 사이트로 보낼 수 없다. 형식과 링크 규칙은 `@nu54/protocol`의 `receiptText`, `parseReceipt`, `explorerTxUrl`이 구현한다.

## 7. 제출과 판정

키오스크는 서명을 받은 뒤 다음 순서로 처리한다 [N10].

1. 키오스크는 새 주문을 받기 전(대기 상태)에 가스 잔액을 확인하고, `kioskMinGasBalance`보다 적으면 주문을 받지 않고 busy를 표시한다.
2. `settle(...)`을 eth_call로 시뮬레이션한다. 컨트랙트 검사 순서는 domain → 주문 유일성 → expiry → 서명자 계정 → registry → payout → nonce → 한도 → 잔액이다 [N07].
3. 시뮬레이션이 `OrderAlreadyPaid`이면, finalized `PaymentSettled`를 `merchant`와 `orderId`(둘 다 indexed)로 조회한다. 이벤트의 device·amount·nonce가 자기가 받은 서명과 같으면 approved다. 다르면 다른 결제로 이미 처리된 주문이므로 refused로 보고한다 [N08].
4. 다른 custom error이면 8절 표에 따라 refused로 보고하고 제출하지 않는다 [N22].
5. 통과하면 트랜잭션을 보내고 finalized 블록에서 `PaymentSettled(address indexed merchant, bytes32 indexed orderId, address indexed device, uint256 amount, uint256 nonce)`를 찾는다. 찾으면 approved다. StableNet 8283은 1초 블록이고 finalized가 latest와 같다.
6. 채굴된 트랜잭션이 status=0이면 같은 호출을 한 번 더 시뮬레이션한다. 결과가 `OrderAlreadyPaid`이면 3번 판정을 적용하고, 그 밖의 결과면 재제출하지 않고 failed와 줄인 tx hash를 보고한다.
7. `payment.prepare` 전송이 끝난 시점부터 10 s 안에 5번 결과가 없으면 Checking으로 바꾸고 "다시 결제하지 마세요"를 표시한다. Checking에서는 이벤트를 계속 찾고, 같은 서명을 다시 보낼 때도 2–3번을 먼저 거친다. 체인 시각이 서명의 `expiry`를 넘었는데도 이벤트가 없으면(재시뮬레이션이 `Expired`) 그 서명은 더는 정산될 수 없으므로 refused가 아니라 failed로 끝내고 재결제를 허용한다.

키오스크가 사용자에게 보이는 결과는 approved, refused, failed, Checking 네 가지다. P07은 영수증 조회용이며 이 판정에 쓰지 않는다 [N08].

## 8. 거절 코드

| 코드 | 거절하는 층 | 보내는 방법 | 조건 | W12-05 |
|---|---|---|---|---|
| `ATTESTATION_EXPIRED` | 기기 | payment.result refused | attestation 유효 기간 밖이거나 authorization expiry가 너무 멀다 | 시연(합의) |
| `MERCHANT_REVOKED` | 컨트랙트 | 시뮬레이션 revert | registry에서 가맹점이 철회되었다 | 시연(합의) |
| `OVER_CAP` | 컨트랙트 | 시뮬레이션 revert | 건당 또는 일일 한도를 넘는다 | 시연(합의) |
| `NONCE_REPLAYED` | 컨트랙트 | 시뮬레이션 revert | 이미 쓴 nonce다. 결제 재전송은 주문 유일성에서 먼저 걸리므로, 실제로는 적용된 LimitChange를 그 expiry 안에 다시 제출할 때 난다 | 시연(합의) |
| `MERCHANT_FORGED` | 기기와 컨트랙트 | refused 또는 revert | 가맹점 서명자, merchant, payout, 주문 내용이 attestation·registry와 다르다 | 시연(합의, 기기 층). 컨트랙트 층은 P06 시험 |
| `USER_REJECTED` | 기기 | payment.result refused | 대여자가 거절 버튼을 눌렀다 | 시연(기기) |
| `TIME_ANCHOR_MISSING` | 기기 | payment.result refused | TimeAnchor가 없거나 reset으로 무효다 | 시연(기기) |
| `UNSUPPORTED_TYPE` | 기기 | error | 두 서명 타입 밖의 요청이다 | 시연(기기) |
| `BAD_FRAME` | 기기와 키오스크 | error | 조각 순서, 길이, digest가 맞지 않는다 | 시험 |
| `TIMEOUT` | 기기 | payment.result refused | 버튼을 기다리다 서명의 expiry를 넘겼다 | 시험 |
| `EXPIRED` | 컨트랙트 | 시뮬레이션 revert | 체인 시각이 expiry를 넘었다 | 시험 |
| `WRONG_DOMAIN` | 컨트랙트 | 시뮬레이션 revert | chainId, contract, token이 다르다 | 시험 |
| `ACCOUNT_INACTIVE` | 컨트랙트 | 시뮬레이션 revert | 서명자 계정이 없거나 닫혔다 | 시험 |
| `INSUFFICIENT_BALANCE` | 컨트랙트 | 시뮬레이션 revert | 예치 잔액이 모자란다 | 시험 |

`CANCELLED`, `PIN_LOCKED`, `NOT_PERMITTED`는 세션·상태 코드이며 거절 시연 대상이 아니다. W12-05는 합의 5종과 기기 3종을 시연한다 [N18][N22].

## 9. 이전 설계에서 없앤 것

- proximityRef 필드는 removed. 근접 증명은 RSSI 기준 연결, 폰 확인 화면, 기기 버튼으로 대신한다 [N27].
- QR 경로는 없다. 기기에 카메라가 없다.
- 기기가 원시 트랜잭션에 서명하던 경로는 없앴다. 트랜잭션은 키오스크가 내고 가스도 키오스크가 낸다 [N10].

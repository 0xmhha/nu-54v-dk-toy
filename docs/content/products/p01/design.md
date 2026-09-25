# P01 설계

[srs.md](srs.md)의 요구를 NCS v3.4.0/Zephyr 4.4 위에서 어떻게 나누어 구현하는지 정한다. 메시지 규칙은 [payment-protocol.md](../../specifications/protocol/payment-protocol.md)가, 서명 정답은 [eip712-vectors.json](../../specifications/protocol/eip712-vectors.json)이 정한다.

## 1. 구조

키 관련 동작은 secure 쪽에만 두고, non-secure 앱은 메시지 처리와 화면만 맡는다.

| 모듈 | 위치 | 하는 일 |
|---|---|---|
| `ble_pay_svc` | non-secure | GATT 서비스 `rx`/`tx`, 조각 재조립과 envelope 확인 [N09] |
| `nfc_handover` | non-secure | NFC 태그에 BLE 주소와 LESC OOB 데이터 기록 |
| `session` | non-secure | session.open/confirm, sessionId, deviceNonce 관리 |
| `pay_flow` | non-secure | identify/prepare/result 상태 기계, 표시 요청, 버튼 대기 |
| `eip712` | non-secure | 스키마 타입의 encodeType·hashStruct·digest 계산 |
| `verifier` | non-secure | MerchantAttestation, MerchantOrder, TimeAnchor 서명 복구와 비교 [N05][N06] |
| `ui` | non-secure | 디스플레이와 버튼, PIN 입력 |
| `key_service` | TF-M secure partition | 키 생성·서명·삭제, PIN 카운터, anchor 저장 [N02] |
| `se_driver` | secure (W8부터) | 외부 secure element와 통신, 키 감싸기와 서명 |
| MCUboot | 부트로더 | 서명 이미지 확인, 유선 serial recovery [N12] |

`key_service`는 PSA 호출 몇 개만 연다. `key_generate`, `sign_digest(digest, purpose)`, `pin_set`, `pin_check`, `anchor_store`, `anchor_read`, `reset_all`. `sign_digest`는 `pay_flow`가 버튼 이벤트 토큰을 함께 넘길 때만 서명한다. 이 토큰은 secure 쪽 GPIO 인터럽트에서 만들어져 non-secure 코드가 위조할 수 없다.

W8부터 `key_service`는 키를 외부 secure element 안에 만들고, TF-M 쪽에는 SE 세션 키와 키 핸들만 둔다. SE가 컷되면 TF-M 봉인으로 남고 이를 waiver로 기록한다 [N14].

## 2. 상태 기계

```
UNPROVISIONED --셋업(키 생성, PIN, 운영자 주소)--> PROVISIONED_NO_ANCHOR
PROVISIONED_NO_ANCHOR --유효한 TimeAnchor--> READY
READY --session.open/confirm--> SESSION
SESSION --payment.identify 통과--> MERCHANT_SHOWN
MERCHANT_SHOWN --payment.prepare 통과--> AWAIT_BUTTON
AWAIT_BUTTON --승인--> SIGNED --payment.result--> SESSION
AWAIT_BUTTON --거절/시간 초과--> SESSION
어느 상태든 --전원 손실--> PROVISIONED_NO_ANCHOR (키 유지)
어느 상태든 --device.reset--> UNPROVISIONED
```

`AWAIT_BUTTON`은 authorization의 `expiry`가 지나면 스스로 `SESSION`으로 돌아가 `TIMEOUT`을 보낸다.

## 3. 저장

| 항목 | 위치 | device.reset | 전원 손실 |
|---|---|---|---|
| 기기 키 | TF-M 보호 저장소 또는 SE | 삭제 | 유지 |
| PIN 검증값과 재시도 카운터 | TF-M 보호 저장소 | 삭제 | 유지 |
| 운영자 주소 | TF-M 보호 저장소 | 삭제 | 유지 |
| 마지막 TimeAnchor timestamp | TF-M 보호 저장소 | 삭제 | 유지(단조성 비교용) |
| anchor 유효 표시 | RAM | 삭제 | 무효 |
| 최근 사용 nonce 목록 | non-secure 설정 저장소 | 삭제 | 유지 |
| BLE bonding 정보 | Zephyr settings | 삭제 | 유지 |

평문 키나 시드는 어느 non-secure 영역에도 쓰지 않는다. 마지막 anchor timestamp를 전원 손실 뒤에도 남기는 이유는 이전보다 이른 TimeAnchor를 거절하기 위해서다.

## 4. 메시지 처리

- **발견과 페어링:** `nfc_handover`가 태그에 주소와 OOB 데이터를 쓴다. 링크 보안 요구는 LE Secure Connections 인증·암호화이며, OOB가 없으면 Numeric Comparison 숫자를 화면에 보이고 버튼으로 확인한다 [N09].
- **재조립:** 협상된 MTU에서 조각 크기 `ATT_MTU - 5`를 계산한다. sequence가 기대값과 다르거나 index가 건너뛰거나 digest가 다르면 버퍼를 비우고 `BAD_FRAME`을 보낸다. 본문은 2048바이트를 넘지 않는다.
- **identify:** `verifier`가 attestation digest를 계산하고 서명자를 복구해 운영자 주소와 비교한다. anchor 시간과 `validFrom..validUntil`을 비교한다.
- **prepare:** MerchantOrder digest로 가맹점 서명자를 복구해 attestation의 merchant와 비교하고, payout·token·amount·orderId·expiry가 authorization과 같은지 확인한다. 화면 문자열은 authorization 필드에서만 만든다.
- **서명:** PaymentAuthorization digest를 `key_service.sign_digest`에 넘긴다. 결과 서명과 `approved`를 보낸다 [N04].
- **setup.timeAnchor:** 셋업 세션에서만 받는다. 서명자, `device` 주소, 단조성을 확인한 뒤 `anchor_store`를 호출한다.

## 5. 오류와 거절 매핑

| 조건 | 모듈 | 응답 |
|---|---|---|
| attestation 서명자 불일치, 주문 서명자 불일치, payout 불일치 | `verifier` | `refused` MERCHANT_FORGED |
| anchor 시간이 유효 기간 밖 | `verifier` | `refused` ATTESTATION_EXPIRED |
| anchor 무효 | `pay_flow` | `refused` TIME_ANCHOR_MISSING |
| 사용자 거절 | `ui` | `refused` USER_REJECTED |
| 버튼 대기 시간 초과 | `pay_flow` | `error` TIMEOUT |
| 모르는 타입 | `pay_flow` | `error` UNSUPPORTED_TYPE |
| 조각·digest 오류 | `ble_pay_svc` | `error` BAD_FRAME |

MERCHANT_REVOKED, OVER_CAP, NONCE_REPLAYED는 컨트랙트가 만든다. 기기는 이 코드를 내지 않는다.

## 6. 보안

- 서명은 버튼 이벤트 토큰이 있어야만 나온다. 키 반출 명령은 GATT에 없다.
- AP-Protect를 켜고 release 빌드에서 디버그 로그의 키 관련 출력을 막는다.
- MCUboot는 오프라인에 둔 서명 키로 서명한 이미지만 부팅하고, 갱신은 유선 serial recovery로만 받는다. BLE DFU는 넣지 않는다 [N12].
- 운영자 주소는 셋업 때 한 번만 쓰고, device.reset 전에는 바꿀 수 없다.
- 기기는 오프라인이어서 registry의 가맹점 철회를 알 수 없다. 이 공백은 컨트랙트가 막는다 [N05].

## 7. 시험 설계

| 시험 | 방법 | 통과 기준 |
|---|---|---|
| 벡터 적합성 | eip712-vectors.json의 모든 벡터를 `eip712`에 넣어 digest 비교 | 전부 일치 |
| 서명 복구 | PA 벡터 digest를 기기 키로 서명하고 host에서 복구 | 기기 주소와 일치 |
| 거절 주입 | 5절 표의 각 조건을 시험 도구로 주입 | 표의 응답과 일치 |
| 조각 결함 주입 | 순서 바꿈, 누락, digest 변조, 초과 길이 | 모두 BAD_FRAME, 재부팅 없음 |
| 전원 손실 | AWAIT_BUTTON 중 전원 차단 후 재부팅 | 키 유지, anchor 무효, 결제 거절 |
| 초기화 | device.reset 후 보호 저장소 덤프 | 키·PIN·anchor 없음 |
| 부트 보호 | 서명 안 된 이미지 serial recovery | 부팅 거부, 기존 slot 유지 |
| 반복 | 연속 20회 결제 | 전부 approved, 누수 없음 |

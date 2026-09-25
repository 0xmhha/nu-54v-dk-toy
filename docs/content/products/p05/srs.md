# P05 SRS — 운영 스크립트

## 1. 목적

이 문서는 P05 운영 스크립트가 만족해야 할 요구를 시험 가능한 형태로 적는다. 각 요구는 Foundry 테스트, anvil 실행 로그, 또는 기기 로그로 판정한다. 값(유효 기간, 지연 시간)은 [DF-20260925-02](../../planning/design-freeze-checkpoint-02.md) parameters에만 있고 여기서는 이름으로만 쓴다 [N13].

## 2. 용어

- **운영자 키**: MerchantAttestation과 TimeAnchor에 서명하는 EOA. 로컬 keystore 파일로만 두고 저장소에 올리지 않는다.
- **registry 관리자 키**: P06 registry에 가맹점을 등록·철회하는 EOA.
- **셋업 세션**: device.reset 직후 기기가 여는 BLE 세션. 운영자 도구만 `setup.timeAnchor`를 보낸다.

## 3. 기능 요구

| ID | 요구 | 검증 |
|---|---|---|
| P05-FR-01 | 가맹점 서명 주소와 payout을 registry에 등록하고, 이미 같은 값으로 등록되어 있으면 트랜잭션 없이 끝난다 | anvil fork 테스트에서 두 번 실행해 두 번째 트랜잭션 수 0 |
| P05-FR-02 | MerchantAttestation `{merchant, payout, name, validFrom, validUntil}`에 운영자 키로 서명하고 `validUntil - validFrom`을 attestationValidity로 둔다 [N05] | 서명이 운영자 주소로 복구되고 기간이 register 값과 같다 |
| P05-FR-03 | attestation의 payout이 registry의 payout과 다르면 서명하지 않는다 | payout을 바꾼 입력으로 실행해 거부 로그 |
| P05-FR-04 | 시험 가맹점 키로 MerchantOrder에 서명하는 보조 명령을 제공한다 | `eip712-vectors.json`의 MO-01과 digest 일치 [N21] |
| P05-FR-05 | 대여 셋업에서 기기가 TRNG로 만든 주소를 셋업 세션에서 읽어 depositFor(deviceAddress, amount, withdrawAddress)를 호출한다. 키는 기기 밖으로 나오지 않는다 [N11] | depositFor 이벤트의 device 필드가 기기 보고 주소와 같다 |
| P05-FR-06 | TimeAnchor `{device, timestamp}`에 운영자 키로 서명해 `setup.timeAnchor`로 보낸다. timestamp는 호스트 시계가 아니라 8283 최신 finalized 블록 시각을 쓴다 [N06] | 기기의 anchor 수락 로그와 블록 시각 비교 |
| P05-FR-07 | 반납 시 closeAccount를 호출하고, 성공 이벤트를 확인한 뒤에만 기기에 device.reset을 지시한다 [N11] | 이벤트 확인 전 reset 명령이 나가지 않는 로그 |
| P05-FR-08 | registry에서 가맹점을 철회한다. 철회 뒤 결제는 컨트랙트가 MERCHANT_REVOKED로 막는다 [N22] | 철회 후 eth_call revert 사유 |
| P05-FR-09 | payout 변경을 요청하고 payoutChangeDelay가 지나기 전에는 새 payout으로 attestation을 발급하지 않는다 [N13] | 지연 전 발급 시도 거부 로그 |
| P05-FR-10 | 전원이 끊겨 anchor가 무효인 기기에 새 TimeAnchor를 다시 보낸다. 이전 anchor보다 이른 timestamp는 보내지 않는다 | 재셋업 로그의 timestamp 단조 증가 |
| P05-FR-11 | 시연용 host 명령으로 같은 BLE 세션에 원시 트랜잭션 서명과 Permit 서명 요청을 보내고 기기의 `UNSUPPORTED_TYPE` 응답을 기록한다. 이 명령은 P05 host 도구에만 있고 키오스크 빌드에는 넣지 않는다 [N22] | 두 요청의 error 응답 로그 |

## 4. 비기능 요구

| ID | 요구 | 검증 |
|---|---|---|
| P05-NFR-01 | 개인 키, mnemonic, keystore 암호를 저장소·로그에 쓰지 않는다. 키는 `--account` keystore 이름이나 환경 변수 참조로만 받는다 | 저장소 grep, redaction 검사 |
| P05-NFR-02 | 실행 로그 원본은 git-ignored 경로에 두고 문서에는 줄인 주소·해시와 원본의 `sha256:` checksum만 적는다 [N16] | `validate_design_freeze_02.py --check redaction` |
| P05-NFR-03 | 모든 스크립트는 같은 입력으로 다시 실행해도 체인 상태를 한 번만 바꾼다(멱등) | 2회 실행 테스트 |
| P05-NFR-04 | 스크립트는 chainId가 8283이 아니면 즉시 멈춘다 | 다른 chainId fork에서 실행 거부 |
| P05-NFR-05 | 운영자·registry 관리자·키오스크 키는 서로 다른 EOA다. 한 키로 두 역할을 하려 하면 멈춘다 | 설정 검사 테스트 |

## 5. 인터페이스

- **P06 컨트랙트 호출**: 가맹점 등록·철회·payout 변경, depositFor, closeAccount. ABI는 P06 배포 manifest에서 읽는다 [N07].
- **EIP-712 서명**: [payment-protocol.schema.json](../../specifications/protocol/payment-protocol.schema.json)의 `operatorSignedTypes`(MerchantAttestation, MerchantOrder, TimeAnchor)와 `eip712Domain`을 그대로 쓴다.
- **BLE 셋업 세션**: [결제 프로토콜](../../specifications/protocol/payment-protocol.md) 4절 envelope와 5절 `setup.timeAnchor`.

## 6. 추적

| 요구 | 결정 | W12 항목 |
|---|---|---|
| P05-FR-02, 03 | [N05] | W12-05(MERCHANT_FORGED 시연 준비) |
| P05-FR-05, 07 | [N07] [N11] | W12-08 |
| P05-FR-06, 10 | [N06] | W12-05(ATTESTATION_EXPIRED 시연 준비) |
| P05-FR-08 | [N22] | W12-05(MERCHANT_REVOKED) |
| P05-FR-10, 11 | [N06] [N22] | W12-05(TIME_ANCHOR_MISSING 재-anchor, UNSUPPORTED_TYPE) |
| P05-NFR-02 | [N16] | 수용 기록지 증거 형식 |

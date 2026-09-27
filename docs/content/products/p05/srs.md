# P05 SRS — 운영 백오피스

## 1. 목적

이 문서는 P05 운영 코어와 CLI `opsctl`이 만족해야 할 요구를 시험 가능한 형태로 적는다 [N20]. 각 요구는 Go 테스트, 로컬 sandbox(anvil chainId 8283)의 실행 로그, 또는 기기 로그로 판정한다. 값(유효 기간, 지연 시간)은 [DF-20260925-02](../../planning/design-freeze-checkpoint-02.md) parameters에만 있고 여기서는 이름으로만 쓴다 [N13].

## 2. 용어

- **운영자 키**: MerchantAttestation, TimeAnchor, DeviceReset에 서명하는 EOA. secretRef로 참조하는 암호화 keystore 파일로만 두고 저장소에 올리지 않는다.
- **registry 관리자 키**: P06 registry에서 가맹점 등록·철회, payout 변경 요청과 cancelPayoutChange를 하는 EOA.
- **시험 가맹점 키**: MerchantOrder에 서명하는 가맹점 키. P05 도구가 만들고 키오스크 설정에 secretRef로 전달한다.
- **셋업 세션**: `session.open`의 mode가 `setup`인 BLE 세션. 기기가 `UNPROVISIONED` 또는 `PROVISIONED_NO_ANCHOR` 상태일 때만 열린다. 페어링만으로는 셋업 권한이 생기지 않는다 [N23].

## 3. 기능 요구

| ID | 요구 | 검증 |
|---|---|---|
| P05-FR-01 | registry 관리자 키로 가맹점 서명 주소와 payout을 registry에 등록하고, 이미 같은 값으로 등록되어 있으면 트랜잭션 없이 끝난다 | sandbox에서 두 번 실행해 두 번째 트랜잭션 수 0 |
| P05-FR-02 | MerchantAttestation `{merchant, payout, name, validFrom, validUntil}`에 운영자 키로 서명하고 `validUntil - validFrom`을 attestationValidity로 둔다. 예외는 P05-FR-09가 payout 변경 대기 중 validUntil을 변경 효력 시각으로 자르는 경우뿐이다 [N05] | 서명이 운영자 주소로 복구되고 기간이 register 값과 같다 |
| P05-FR-03 | attestation의 payout이 registry의 payout과 다르면 서명하지 않는다 | payout을 바꾼 입력으로 실행해 거부 로그 |
| P05-FR-04 | 시험 가맹점 키로 MerchantOrder에 서명하는 보조 명령을 제공한다 | `eip712-vectors.json`의 MO-01과 digest 일치 [N21] |
| P05-FR-05 | 대여 셋업은 `session.open`(setup) → `setup.operator{operator, contract, chainId}`(대여자 버튼 확인) → 기기 TRNG 키와 PIN → `setup.ack{keygen, device}` → `setup.timeAnchor` → 수락 `setup.ack` 순서로 진행하고, keygen ack가 알린 기기 주소로만 depositFor(deviceAddress, amount, withdrawAddress)를 호출한다. 키는 기기 밖으로 나오지 않는다 [N11][N23] | depositFor 이벤트의 device 필드가 keygen ack의 device와 같고, anchor 수락 ack 전에는 depositFor가 나가지 않는 로그 |
| P05-FR-06 | keygen ack를 받은 뒤 그 주소로 TimeAnchor `{device, timestamp}`에 운영자 키로 서명해 `setup.timeAnchor{device, timestamp, operatorSignature}`로 보낸다. timestamp는 호스트 시계가 아니라 8283 최신 finalized 블록 시각을 쓰고 기기가 보고한 lastAnchor보다 엄격히 늦어야 한다 [N06] | 기기의 anchor 수락 로그와 블록 시각 비교 |
| P05-FR-07 | 반납 시 closeAccount를 호출하고, finalized 이벤트를 확인한 뒤에만 운영자가 서명한 DeviceReset `{device, nonce}`를 `device.reset`으로 보낸다 [N11][N23] | 이벤트 확인 전 reset 명령이 나가지 않는 로그, 기기의 reset 수락 로그 |
| P05-FR-08 | registry 관리자 키로 가맹점을 철회한다. 철회 뒤 결제는 컨트랙트가 MERCHANT_REVOKED로 막는다 [N22] | 철회 후 eth_call revert 사유 |
| P05-FR-09 | registry 관리자 키로 payout 변경을 요청하고(되돌릴 때는 cancelPayoutChange) payoutChangeDelay가 지나기 전에는 새 payout으로 attestation을 발급하지 않는다. 대기 중 발급하는 옛 payout attestation은 validUntil을 변경 효력 시각으로 자르고, 효력 시각에 새 payout attestation을 발급한다 [N05][N13] | 지연 전 발급 시도 거부 로그, 대기 중 attestation의 validUntil |
| P05-FR-10 | RAM이 초기화되는 reset으로 anchor가 무효인 기기(`PROVISIONED_NO_ANCHOR`)에 셋업 세션을 열어 `setup.timeAnchor`만 다시 보낸다. 키와 예치금은 그대로이고, lastAnchor보다 엄격히 늦지 않은 timestamp는 보내지 않는다 [N06] | 재-anchor 로그의 timestamp 단조 증가 |
| P05-FR-11 | 거절 시연 명령 `opsctl refusal-host`는 기기와 따로 페어링해 결제 세션(`session.open`, `session.confirm`)을 연 뒤, 스키마에 없는 원시 트랜잭션 서명과 Permit 서명 요청을 보내고 기기의 `error{UNSUPPORTED_TYPE}` 응답을 기록한다. 이 명령은 P05에만 있고 키오스크 빌드에는 넣지 않는다 [N22] | 두 요청의 error 응답 로그 |
| P05-FR-12 | 만료된 attestation 시연 자료는 소급 발급 없이, 시연 시각보다 attestationValidity와 anchorClockSkew를 더한 시간 이상 먼저 발급해 둔다 [N05] | 발급 로그의 validUntil과 시연 시각 비교 |
| P05-FR-13 | 시험 가맹점 서명 키를 만들어 키오스크 설정에 secretRef로 전달하고 키 원문은 저장소·로그에 남기지 않는다 [N05] | 키오스크 설정 검사, 저장소 grep |

## 4. 비기능 요구

| ID | 요구 | 검증 |
|---|---|---|
| P05-NFR-01 | 개인 키, mnemonic, keystore 암호를 저장소·로그에 쓰지 않는다. 키는 secretRef(keystore 경로와 암호의 환경 변수 참조)로만 받는다 | 저장소 grep, redaction 검사 |
| P05-NFR-02 | 실행 로그 원본은 git-ignored 경로에 두고 문서에는 줄인 주소·해시와 원본의 `sha256:` checksum만 적는다 [N16] | `validate_design_freeze_02.py --check redaction` |
| P05-NFR-03 | 모든 `opsctl` 명령은 같은 입력으로 다시 실행해도 체인 상태를 한 번만 바꾼다(멱등) | 2회 실행 테스트 |
| P05-NFR-04 | `opsctl`은 chainId가 8283이 아니면 즉시 멈춘다 | 다른 chainId 노드에서 실행 거부 |
| P05-NFR-05 | 운영자·registry 관리자·키오스크·시험 가맹점 키는 서로 다른 EOA다. 한 키로 두 역할을 하려 하면 멈춘다 | 설정 검사 테스트 |
| P05-NFR-06 | 서명과 트랜잭션 전송마다 누가, 무엇을, 어떤 입력으로 했는지를 PostgreSQL 감사 테이블에 남긴다. 다음 사이클의 `opsd` API와 백오피스 화면이 같은 테이블을 읽는다 [N20] | sandbox PostgreSQL 조회 |

## 5. 인터페이스

- **P06 컨트랙트 호출**: 가맹점 등록·철회·payout 변경·cancelPayoutChange, depositFor, closeAccount, 출금. ABI와 주소는 P06 배포 manifest에서 생성한 packages/contracts-abi Go 바인딩으로 읽는다 [N07].
- **EIP-712 서명**: [payment-protocol.schema.json](../../specifications/protocol/payment-protocol.schema.json)의 `operatorSignedTypes`(MerchantAttestation, MerchantOrder, TimeAnchor, DeviceReset)와 `eip712Domain`을 그대로 쓴다.
- **BLE 셋업 세션**: [결제 프로토콜](../../specifications/protocol/payment-protocol.md) 4절 envelope와 5절 `setup.operator`, `setup.timeAnchor`, `setup.ack`, `device.reset`.

## 6. 추적

| 요구 | 결정 | W12 항목 |
|---|---|---|
| P05-FR-02, 03, 13 | [N05] | W12-05(MERCHANT_FORGED 시연 준비) |
| P05-FR-05, 07 | [N07] [N11] [N23] | W12-08 |
| P05-FR-06, 10, 12 | [N06] [N05] | W12-05(ATTESTATION_EXPIRED 시연 준비) |
| P05-FR-08 | [N22] | W12-05(MERCHANT_REVOKED) |
| P05-FR-10, 11 | [N06] [N22] | W12-05(TIME_ANCHOR_MISSING 재-anchor, UNSUPPORTED_TYPE) |
| P05-NFR-02 | [N16] | 수용 기록지 증거 형식 |

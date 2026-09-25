# P05 설계 — 운영 스크립트

P05는 Foundry `forge script`와 작은 호스트 도구로 이루어진다 [N20]. 운영자 키로 서명하는 세 가지(MerchantAttestation, TimeAnchor, DeviceReset)와 P06 registry·정산 컨트랙트 호출을 한곳에 모아, 운영 절차를 코드로 재현할 수 있게 한다.

## 1. 구조

```
products/p05-operations-backoffice/
  foundry.toml            # rpc_endpoints.stablenet8283, chain_id 검사
  config/ops.toml         # 계약 주소는 배포 manifest 경로만 참조, 키는 keystore 이름만
  script/
    RegisterMerchant.s.sol   IssueAttestation.s.sol   SignOrder.s.sol
    ProvisionRental.s.sol    IssueTimeAnchor.s.sol    CloseRental.s.sol
    SignDeviceReset.s.sol    RevokeMerchant.s.sol     ChangePayout.s.sol
    lib/Eip712Ops.sol        # domain과 operatorSignedTypes 해시
  tools/setup-client/      # 셋업 세션 클라이언트: setup.operator, setup.timeAnchor, device.reset
  tools/refusal-host/      # 거절 시연 도구(키오스크 빌드에 넣지 않음)
  test/                    # anvil fork 테스트, 벡터 적합성
```

키는 `cast wallet import`로 만든 로컬 keystore 이름(`--account operator`)으로만 참조한다. 역할별 keystore는 operator, registry-admin, kiosk, test-merchant 네 개이며 설정 검사에서 주소가 서로 다른지 확인한다.

## 2. 가맹점 신뢰 체인

운영자가 발급하는 MerchantAttestation이 기기의 가맹점 판단 근거이고, P06 registry가 최종 권한이다 [N05].

1. `RegisterMerchant`가 registry 관리자 키로 registry에 `(merchant, payout)`을 등록한다.
2. `IssueAttestation`이 registry에서 payout을 읽어 입력과 비교한 뒤, `{merchant, payout, name, validFrom, validUntil}`을 운영자 키로 EIP-712 서명한다. 기간은 attestationValidity이다 [N13]. payout 변경 대기 중이면 validUntil을 변경 효력 시각으로 자르고, 효력 시각에 새 payout으로 발급한다. payoutChangeDelay가 attestationValidity 이상이므로 효력 시각 뒤에 유효한 옛 payout attestation이 남지 않는다.
3. 결과 JSON(`attestation.json`)과 시험 가맹점 키의 secretRef를 키오스크 설정에 넣는다(키 원문은 저장소에 올리지 않는다). 키오스크는 이를 `payment.identify`로 기기에 넘긴다.
4. 기기는 셋업 때 `setup.operator`로 기록한 운영자 주소로 서명을 검증한다. 오프라인 기기가 모르는 철회는 registry가 MERCHANT_REVOKED로 막는다.

## 3. TimeAnchor 전달

기기는 믿을 수 있는 시계가 없어 운영자가 서명한 TimeAnchor를 기준 시간으로 쓴다 [N06]. 목표 경로는 대여자 휴대폰의 설정 앱(P02)이지만 P02는 이번 사이클에서 설계만 한다. 그래서 **P05 provisioning 스크립트가 같은 `setup.timeAnchor` 메시지를 BLE 셋업 세션으로 보내 P02 설정 앱을 대신한다.** 메시지 형식이 같으므로 P02가 생기면 호스트 도구만 바꾸면 된다.

| 단계 | 구성 요소 | 동작 |
|---|---|---|
| 1 | `tools/setup-client` | 기기와 LE Secure Connections로 페어링하고 `session.open`(mode=setup)으로 셋업 세션을 연다. 기기가 `UNPROVISIONED`나 `PROVISIONED_NO_ANCHOR`가 아니면 `NOT_PERMITTED`로 거절된다. `UNPROVISIONED`에서는 `session.open.ok`에 device가 없다 |
| 2 | 기기 | 첫 셋업이면 `setup.operator` 수락과 키 생성·PIN 뒤 `setup.ack{step: keygen, device}`로 새 주소를 알린다. 재-anchor면 `session.open.ok`의 device와 lastAnchor를 그대로 쓴다 |
| 3 | `IssueTimeAnchor.s.sol` | 2단계에서 받은 device와 8283 최신 finalized 블록 시각으로 `{device, timestamp}`에 운영자 키로 서명한다(세션 중 온라인 서명) |
| 4 | `tools/setup-client` | `setup.timeAnchor{sessionId, device, timestamp, operatorSignature}`를 CBOR envelope로 보낸다 |
| 5 | 기기 | device가 자기 주소인지, 서명자가 기록된 운영자인지, timestamp가 lastAnchor보다 엄격히 늦은지 검사하고 `setup.ack{step: setup.timeAnchor}`로 답한다 |

셋업 클라이언트는 서명 전에 lastAnchor로 단조성을 미리 확인한다. 전원 손실 등 RAM이 초기화되는 reset 뒤 재-anchor는 `PROVISIONED_NO_ANCHOR`에서 1단계와 3–5단계만 다시 한다.

## 4. 대여·반납 흐름

- **대여**: `session.open`(setup) → `setup.operator{operator, contract, chainId}`(대여자 버튼 확인) [N23] → 기기 TRNG 키 생성 → PIN → `setup.ack{keygen, device}` → TimeAnchor 서명 → `setup.timeAnchor` → `setup.ack` 수락 → `ProvisionRental`의 depositFor [N07] [N11].
- **반납**: `CloseRental`의 closeAccount → finalized 이벤트 확인 → `SignDeviceReset`으로 서명한 DeviceReset `{device, nonce}`를 `device.reset`으로 전송. 순서를 바꾸지 않는다. 계정을 먼저 비활성화해 키가 없는 활성 계정이 남지 않게 한다.
- **거절 시연**: `tools/refusal-host`는 따로 페어링해 결제 세션(`session.open`, `session.confirm`)을 열고 스키마에 없는 원시 트랜잭션·Permit 서명 요청을 보내 `error{UNSUPPORTED_TYPE}`을 기록한다 [N22].

## 5. EIP-712 서명

domain은 [payment-protocol.schema.json](../../specifications/protocol/payment-protocol.schema.json)의 `eip712Domain`(name, version, chainId 8283, verifyingContract=정산 컨트랙트)을 쓴다. `lib/Eip712Ops.sol`은 `operatorSignedTypes`의 encodeType 문자열을 그대로 상수로 둔다. 서명은 `vm.sign`이 아니라 `cast wallet sign --data`로 만들어 keystore 밖으로 키가 나오지 않게 한다. 결과는 `eip712-vectors.json`의 MA-01, MO-01, TA-01, DR-01과 같은 digest가 나오는지로 확인한다 [N21].

## 6. 실행 로그와 증거

각 스크립트는 `evidence/p05/<날짜>-<스크립트>.log`(git-ignored)에 원본 로그를 쓴다. 문서와 수용 기록지에는 줄인 주소·tx hash와 원본 로그의 `sha256:` checksum만 옮긴다 [N16].

## 7. 오류 처리

| 상황 | 동작 |
|---|---|
| chainId가 8283이 아님 | 서명·전송 전에 멈춘다 |
| registry payout과 입력 payout 불일치 | attestation 서명 거부 |
| payoutChangeDelay 진행 중 | 새 payout attestation 거부 |
| TimeAnchor 시각이 이전보다 이름 | 전송 거부 |
| closeAccount 실패 | DeviceReset 전송 보류 |
| `setup.ack`가 거부 | depositFor를 호출하지 않는다 |
| 트랜잭션 미확정 | 같은 nonce로만 재전송, 새 트랜잭션을 만들지 않는다 |

## 8. 시험 설계

- anvil로 8283 fork를 띄워 P06을 배포하고 각 스크립트를 두 번 실행해 멱등성을 본다.
- 벡터 적합성: `Eip712Ops` 해시가 `eip712-vectors.json` digest와 같아야 한다.
- 셋업 클라이언트는 먼저 USB CDC harness로 envelope·fragment를 시험하고, 게이트 증거는 BLE 셋업 세션으로만 만든다 [N09].

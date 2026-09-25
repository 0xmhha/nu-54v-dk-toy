# P05 설계 — 운영 스크립트

P05는 Foundry `forge script`와 작은 호스트 도구로 이루어진다 [N20]. 운영자 키로 서명하는 두 가지(MerchantAttestation, TimeAnchor)와 P06 registry·정산 컨트랙트 호출을 한곳에 모아, 운영 절차를 코드로 재현할 수 있게 한다.

## 1. 구조

```
products/p05-operations-backoffice/
  foundry.toml            # rpc_endpoints.stablenet8283, chain_id 검사
  config/ops.toml         # 계약 주소는 배포 manifest 경로만 참조, 키는 keystore 이름만
  script/
    RegisterMerchant.s.sol   IssueAttestation.s.sol   SignOrder.s.sol
    ProvisionRental.s.sol    IssueTimeAnchor.s.sol    CloseRental.s.sol
    RevokeMerchant.s.sol     ChangePayout.s.sol
    lib/Eip712Ops.sol        # domain과 operatorSignedTypes 해시
  tools/setup-client/      # BLE 셋업 세션 클라이언트(호스트 PC)
  test/                    # anvil fork 테스트, 벡터 적합성
```

키는 `cast wallet import`로 만든 로컬 keystore 이름(`--account operator`)으로만 참조한다. 역할별 keystore는 operator, registry-admin, kiosk 세 개이며 설정 검사에서 주소가 서로 다른지 확인한다.

## 2. 가맹점 신뢰 체인

운영자가 발급하는 MerchantAttestation이 기기의 가맹점 판단 근거이고, P06 registry가 최종 권한이다 [N05].

1. `RegisterMerchant`가 registry에 `(merchant, payout)`을 등록한다.
2. `IssueAttestation`이 registry에서 payout을 읽어 입력과 비교한 뒤, `{merchant, payout, name, validFrom, validUntil}`을 EIP-712로 서명한다. 기간은 attestationValidity이다 [N13].
3. 결과 JSON(`attestation.json`)을 키오스크 설정에 넣는다. 키오스크는 이를 `payment.identify`로 기기에 넘긴다.
4. 기기는 provisioning 때 기록한 운영자 주소로 서명을 검증한다. 오프라인 기기가 모르는 철회는 registry가 MERCHANT_REVOKED로 막는다.

## 3. TimeAnchor 전달

기기는 믿을 수 있는 시계가 없어 운영자가 서명한 TimeAnchor를 기준 시간으로 쓴다 [N06]. 목표 경로는 대여자 휴대폰의 설정 앱(P02)이지만 P02는 이번 사이클에서 설계만 한다. 그래서 **P05 provisioning 스크립트가 같은 `setup.timeAnchor` 메시지를 BLE 셋업 세션으로 보내 P02 설정 앱을 대신한다.** 메시지 형식이 같으므로 P02가 생기면 호스트 도구만 바꾸면 된다.

| 단계 | 구성 요소 | 동작 |
|---|---|---|
| 1 | `IssueTimeAnchor.s.sol` | 8283 finalized 블록 시각을 읽어 `{device, timestamp}`에 운영자 키로 서명하고 `anchor.json`을 쓴다 |
| 2 | `tools/setup-client` | 기기와 LE Secure Connections로 페어링하고 셋업 세션을 연다 |
| 3 | `tools/setup-client` | `setup.timeAnchor{sessionId, timestamp, operatorSignature}`를 CBOR envelope로 보낸다 |
| 4 | 기기 | 서명자·device·단조성을 검사하고 수락 또는 거부를 응답한다 |

셋업 클라이언트는 기기 보고 주소와 `anchor.json`의 device가 다르면 보내지 않는다. 이전 anchor 시각은 기기 응답에서 받아 단조성을 미리 확인한다.

## 4. 대여·반납 흐름

- **대여**: device.reset → 기기 TRNG 키 생성 → PIN → TimeAnchor → `ProvisionRental`의 depositFor [N07] [N11].
- **반납**: `CloseRental`의 closeAccount → finalized 이벤트 확인 → 기기 device.reset. 순서를 바꾸지 않는다.

## 5. EIP-712 서명

domain은 [payment-protocol.schema.json](../../specifications/protocol/payment-protocol.schema.json)의 `eip712Domain`(name, version, chainId 8283, verifyingContract=정산 컨트랙트)을 쓴다. `lib/Eip712Ops.sol`은 `operatorSignedTypes`의 encodeType 문자열을 그대로 상수로 둔다. 서명은 `vm.sign`이 아니라 `cast wallet sign --data`로 만들어 keystore 밖으로 키가 나오지 않게 한다. 결과는 `eip712-vectors.json`의 MA-01, MO-01, TA-01과 같은 digest가 나오는지로 확인한다 [N21].

## 6. 실행 로그와 증거

각 스크립트는 `evidence/p05/<날짜>-<스크립트>.log`(git-ignored)에 원본 로그를 쓴다. 문서와 수용 기록지에는 줄인 주소·tx hash와 원본 로그의 `sha256:` checksum만 옮긴다 [N16].

## 7. 오류 처리

| 상황 | 동작 |
|---|---|
| chainId가 8283이 아님 | 서명·전송 전에 멈춘다 |
| registry payout과 입력 payout 불일치 | attestation 서명 거부 |
| payoutChangeDelay 진행 중 | 새 payout attestation 거부 |
| TimeAnchor 시각이 이전보다 이름 | 전송 거부 |
| closeAccount 실패 | device.reset 지시 보류 |
| 트랜잭션 미확정 | 같은 nonce로만 재전송, 새 트랜잭션을 만들지 않는다 |

## 8. 시험 설계

- anvil로 8283 fork를 띄워 P06을 배포하고 각 스크립트를 두 번 실행해 멱등성을 본다.
- 벡터 적합성: `Eip712Ops` 해시가 `eip712-vectors.json` digest와 같아야 한다.
- 셋업 클라이언트는 먼저 USB CDC harness로 envelope·fragment를 시험하고, 게이트 증거는 BLE 셋업 세션으로만 만든다 [N09].

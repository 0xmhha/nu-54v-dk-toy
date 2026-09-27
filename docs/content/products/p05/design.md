# P05 설계 — 운영 백오피스

P05는 백오피스 전체를 전제로 설계한다 [N20]. 이번 사이클에는 Go 운영 코어와 CLI `opsctl`만 만들고, 같은 코어 위에 올라갈 HTTP API(`opsd`)와 React 백오피스 UI(`web/`)는 9절에 설계만 둔다. 운영자 키로 서명하는 세 가지(MerchantAttestation, TimeAnchor, DeviceReset)와 P06 registry·정산 컨트랙트 호출을 코어 한곳에 모아, 체인과 서명 키를 다루는 코드를 한 언어로 관리한다 [N25].

## 1. 구조

```
products/p05-operations-backoffice/
  go.mod
  cmd/opsctl/            # CLI (이번 사이클)
  cmd/opsd/              # HTTP API 서버 (설계만, 다음 사이클)
  internal/core/
    chain/               # go-ethereum ethclient, chainId 8283 검사, finalized 확인, nonce 관리
    signer/              # EIP-712 서명: packages/protocol/go 생성 타입과 eip712Domain
    keystore/            # 암호화 JSON keystore, secretRef(경로 + 암호 환경 변수)로만 연다
    audit/               # 서명·전송 감사 기록
    ops/                 # 업무 절차: 가맹점, attestation, 대여, 반납, 출금
  internal/ble/          # BLE 셋업 클라이언트(tinygo-org/bluetooth), 결제 프로토콜 4절 틀
  internal/store/        # PostgreSQL 저장소와 migrations/
  config/ops.example.toml # 계약 주소는 배포 manifest 경로만, 키는 secretRef만
  web/                   # React·TypeScript 백오피스 UI (설계만, 다음 사이클)
```

역할별 keystore는 operator, registry-admin, kiosk, test-merchant 네 개이며 `opsctl`이 시작할 때 주소가 서로 다른지 확인한다. `cmd/opsctl`의 명령은 `internal/core/ops`의 함수를 부르는 얇은 층이고, 다음 사이클의 `opsd`도 같은 함수를 HTTP 핸들러로 연다. 그래서 CLI로 검증한 절차가 API와 화면에서 그대로 쓰인다.

| `opsctl` 명령 | `internal/core/ops` 절차 | 다음 사이클 API 자원 |
|---|---|---|
| `merchant register`, `revoke`, `payout-change`, `payout-cancel` | 가맹점 관리 | `/merchants` |
| `attestation issue` | attestation 발급 | `/merchants/{id}/attestations` |
| `order sign` | 시험 가맹점 주문 서명 | 개발용, API로 열지 않음 |
| `rental provision`, `rental re-anchor`, `rental return` | 대여 셋업, 재-anchor, 반납 | `/rentals` |
| `withdraw request`, `cancel`, `execute` | 출금 | `/rentals/{id}/withdrawals` |
| `refusal-host` | 거절 시연 | 시연용, API로 열지 않음 |
| `merchant-key handover` | 시험 가맹점 키 전달 | `/merchants/{id}/kiosk-key` |

## 2. 가맹점 신뢰 체인

운영자가 발급하는 MerchantAttestation이 기기의 가맹점 판단 근거이고, P06 registry가 최종 권한이다 [N05].

1. `opsctl merchant register`가 registry 관리자 키로 registry에 `(merchant, payout)`을 등록한다.
2. `opsctl attestation issue`가 registry에서 payout을 읽어 입력과 비교한 뒤, `{merchant, payout, name, validFrom, validUntil}`을 운영자 키로 EIP-712 서명한다. 기간은 attestationValidity이다 [N13]. payout 변경 대기 중이면 validUntil을 변경 효력 시각으로 자르고, 효력 시각에 새 payout으로 발급한다. payoutChangeDelay가 attestationValidity 이상이므로 효력 시각 뒤에 유효한 옛 payout attestation이 남지 않는다.
3. 결과 JSON(`attestation.json`)과 시험 가맹점 키의 secretRef를 키오스크 설정에 넣는다(키 원문은 저장소에 올리지 않는다). 키오스크는 이를 `payment.identify`로 기기에 넘긴다.
4. 기기는 셋업 때 `setup.operator`로 기록한 운영자 주소로 서명을 검증한다. 오프라인 기기가 모르는 철회는 registry가 MERCHANT_REVOKED로 막는다.

## 3. TimeAnchor 전달

기기는 믿을 수 있는 시계가 없어 운영자가 서명한 TimeAnchor를 기준 시간으로 쓴다 [N06]. 목표 경로는 대여자 휴대폰의 설정 앱(P02)이지만 P02는 이번 사이클에서 설계만 한다. 그래서 **`opsctl rental provision`이 같은 `setup.timeAnchor` 메시지를 BLE 셋업 세션으로 보내 P02 설정 앱을 대신한다.** 메시지 형식이 같으므로 P02가 생기면 보내는 쪽만 바뀐다.

| 단계 | 구성 요소 | 동작 |
|---|---|---|
| 1 | `internal/ble` | 기기와 LE Secure Connections로 본딩하고(첫 셋업은 운영 장소에서 Just Works와 기기 버튼 확인, 재-anchor는 기기별 passkey로 Passkey Entry [N27]) `session.open`(mode=setup)으로 셋업 세션을 연다. 기기가 `UNPROVISIONED`나 `PROVISIONED_NO_ANCHOR`가 아니면 `NOT_PERMITTED`로 거절된다. `UNPROVISIONED`에서는 `session.open.ok`에 device가 없다 |
| 2 | 기기 | 첫 셋업이면 `setup.operator` 수락과 키 생성·PIN 뒤 `setup.ack{step: keygen, device}`로 새 주소를 알린다. 재-anchor면 `session.open.ok`의 device와 lastAnchor를 그대로 쓴다 |
| 3 | `internal/core/signer` | 2단계에서 받은 device와 8283 최신 finalized 블록 시각으로 `{device, timestamp}`에 운영자 키로 서명한다(세션 중 온라인 서명) |
| 4 | `internal/ble` | `setup.timeAnchor{sessionId, device, timestamp, operatorSignature}`를 CBOR envelope로 보낸다 |
| 5 | 기기 | device가 자기 주소인지, 서명자가 기록된 운영자인지, timestamp가 lastAnchor보다 엄격히 늦은지 검사하고 `setup.ack{step: setup.timeAnchor}`로 답한다 |

`opsctl`은 서명 전에 lastAnchor로 단조성을 미리 확인한다. 전원 손실 등 RAM이 초기화되는 reset 뒤 재-anchor(`opsctl rental re-anchor`)는 `PROVISIONED_NO_ANCHOR`에서 1단계와 3–5단계만 다시 한다.

## 4. 대여·반납 흐름

- **대여** (`opsctl rental provision`): `session.open`(setup) → `setup.operator{operator, contract, chainId}`(대여자 버튼 확인) [N23] → 기기 TRNG 키 생성 → 버튼으로 PIN 설정 → 기기별 passkey 기록과 라벨 QR 인쇄 [N27] → `setup.ack{keygen, device}` → TimeAnchor 서명 → `setup.timeAnchor` → `setup.ack` 수락 → depositFor [N07] [N11].
- **반납** (`opsctl rental return`): closeAccount → finalized 이벤트 확인 → 운영자가 서명한 DeviceReset `{device, nonce}`를 `device.reset`으로 전송. 순서를 바꾸지 않는다. 계정을 먼저 비활성화해 키가 없는 활성 계정이 남지 않게 한다.
- **거절 시연** (`opsctl refusal-host`): 키오스크처럼 페어링 없이 결제 세션(`session.open`, `session.confirm`)을 열고 스키마에 없는 원시 트랜잭션·Permit 서명 요청을 보내 `error{UNSUPPORTED_TYPE}`을 기록한다 [N22]. 키오스크 빌드에는 넣지 않는다.

## 5. EIP-712 서명

domain은 [payment-protocol.schema.json](../../specifications/protocol/payment-protocol.schema.json)의 `eip712Domain`(name, version, chainId 8283, verifyingContract=정산 컨트랙트)을 쓴다. `internal/core/signer`는 packages/protocol/go가 스키마에서 생성한 `operatorSignedTypes` 타입과 encodeType 문자열을 쓰고, 키는 keystore에서 연 프로세스 안에서만 쓴다. 결과는 `eip712-vectors.json`의 MA-01, MO-01, TA-01, DR-01과 같은 digest와 서명자가 나오는지로 확인한다 [N21].

## 6. 실행 로그와 증거

모든 서명과 전송은 `internal/core/audit`가 PostgreSQL 감사 테이블에 남기고, `opsctl`은 같은 내용을 `evidence/p05/<날짜>-<명령>.log`(git-ignored)에도 쓴다. 문서와 수용 기록지에는 줄인 주소·tx hash와 원본 로그의 `sha256:` checksum만 옮긴다 [N16].

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
| 역할 키 주소가 겹침 | `opsctl` 시작 시 멈춘다 |

## 8. 시험 설계

- 로컬 sandbox(docker compose의 anvil chainId 8283, PostgreSQL)에 P06을 배포하고 각 `opsctl` 명령을 두 번 실행해 멱등성을 본다. 게이트 증거는 StableNet 8283 테스트넷에서 만든다.
- 벡터 적합성: `internal/core/signer`의 digest와 서명자가 `eip712-vectors.json`과 같아야 한다(Go 테스트).
- BLE 셋업 클라이언트는 먼저 USB CDC harness로 envelope·fragment를 시험하고, 게이트 증거는 BLE 셋업 세션으로만 만든다 [N09].

## 9. 백오피스 구조 (설계만, 다음 사이클)

이번 사이클에는 만들지 않는다. 코어와 저장소는 이 구조를 전제로 만든다.

- **`opsd` API 서버**: `internal/core/ops`를 HTTP로 연다. 자원은 1절 표의 `/merchants`, `/merchants/{id}/attestations`, `/merchants/{id}/kiosk-key`, `/rentals`, `/rentals/{id}/withdrawals`, 그리고 조회용 `/audit`이다. 인증과 역할 권한(운영자, registry 관리자, 감사자)은 이때 정한다.
- **`web/` 백오피스 UI**: React·TypeScript. 화면은 가맹점 목록·등록·payout 변경, attestation 발급 이력, 대여 현황과 반납 처리, 출금 대기열, 감사 로그 조회다. BLE 셋업은 기기 옆에서 해야 하므로 UI가 아니라 `opsctl`에 남긴다.
- **PostgreSQL 테이블**: `merchants`, `attestations`, `rentals`, `withdrawals`, `audit_events`. 이번 사이클의 `internal/store` migration이 이 테이블을 만들고, `opsctl`이 먼저 채운다.

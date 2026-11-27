# P01 · NU-54V-DK device firmware

> **DF-20260925-02 기준 (2026-09-25):** 이번 12주 사이클의 범위와 설계는 [기획](../../docs/content/products/p01/plan.md) · [SRS](../../docs/content/products/p01/srs.md) · [유즈케이스](../../docs/content/products/p01/use-cases.md) · [설계](../../docs/content/products/p01/design.md)가 정한다. 아래 원문은 DF-20260920-01 기준이며 충돌하면 위 문서를 따른다.

## 개발 (DF-20260925-02)

| 경로 | 내용 |
|---|---|
| `core/` | 보드와 무관한 C 코드. Zephyr 헤더를 쓰지 않아 host에서 빌드·시험한다. BLE 메시지 조각 재조립(프로토콜 4절), EIP-712 digest(`nu54_eip712`), 서명자 복원과 low-s·v 계산(`nu54_sig`), keccak-256(`nu54_keccak`), 결정적 CBOR 디코딩·스키마 검사·인코딩(`nu54_cbor`), 결제·셋업 세션(`nu54_session*.c`: 공통·결제·셋업·한도·보안 채널로 나뉘고 내부 헤더 `nu54_session_int.h`, 고정 설정 `nu54_config_t`와 메시지별 링크 `nu54_link_t`를 받는다, 서명과 난수는 플랫폼 콜백, 버튼은 비동기), 버튼 PIN 입력(`nu54_pin_entry`, 4자리), central 판정(`nu54_links`: 세션을 가진 링크, 폰 앱, 다른 링크), 결제 세션 보안 채널(4.1절: `nu54_secure`의 ECDH, HKDF와 AES-GCM은 플랫폼 콜백으로 보드는 PSA, host 시험은 OpenSSL) |
| `third_party/` | libsecp256k1 v0.8.0(MIT), Keccak compact(CC0). 출처 커밋과 빌드 설정은 [`third_party/README.md`](third_party/README.md) |
| `app/` | Zephyr 앱(C). `src/ble_links.c`는 BLE(결제 GATT 서비스, central 2개, 페어링, 결제·페어링 모드 광고, 조각 송신)만 맡고, `src/pay_link.c`는 work queue 하나에서 세션(`nu54_session`), 링크 판정(`nu54_links`), 재조립을 돌린다. Bluetooth 스레드와 main 루프는 이벤트를 큐에 넣기만 한다. `src/device_setup.c`는 저장(셋업 기록·PIN HMAC·실패 횟수는 PSA ITS, nonce는 settings)과 플랫폼 콜백, `src/status_led.c`는 LED, `src/main.c`는 버튼과 PIN 입력이다. 셋업이 없으면 기본 빌드는 7주차 고정 셋업으로 키를 만들고, `fw.py build --rental`(`app/rental.conf`)은 UNPROVISIONED로 셋업 세션을 기다린다. 7주차 개발 빌드의 버튼·LED·셋업은 [설계](../../docs/content/products/p01/design.md) 8절 |
| `boards/nucode/nu54v_dk/` | 제조사 보드 패키지(MIT, 출처 커밋은 `VENDORED.md`) |
| `test/` | `core/`의 host 단위 시험(CMake + CTest). `eip712_vectors`, `frame_vectors`, `cbor_vectors`, `session_vectors`는 공용 벡터로 시험하며 적합성 harness가 실행한다 |
| `scripts/fw.py` | NCS 툴체인 안에서 west 빌드와 pyOCD 플래시를 실행한다. 릴리스 빌드, 서명 키 생성, 거부 시험 이미지(아래) |
| `tools/bringup/` | 보드 bring-up 도구: 시리얼 CLI, BLE 스캔·연결, 버튼 시뮬레이션, 레지스터 읽기, 결제 BLE 전송 브리지(`pay_bridge.py`) ([README](tools/bringup/README.md)) |

기기 키(7주차 경로, P01-FR-04의 non-secure 단계): PSA Crypto가 CRACEN으로 secp256k1 키를 한 번 만들고 Zephyr Secure storage(ZMS, `storage` 파티션)에 영속 저장한다. 키 정책은 서명만 허용하고 내보내기를 막는다. 서명은 결정론적 ECDSA(RFC 6979)라서 같은 digest에는 항상 같은 서명이 나오고, `nu54_sig_finish`가 low-s와 v를 맞춘다. 8주차에 TF-M(`/ns` 변형)과 암호화 ITS로 옮긴다. 2026-10-01 보드 확인: 리셋 뒤에도 같은 키를 열고, 기기 로그의 digest와 서명을 host에서 복원하면 기기 주소와 같다.

```bash
make test         # host에서 core/ 시험 (NCS 불필요)
make fw           # nu54v_dk/nrf54l15/cpuapp, NCS v3.4.2
make flash        # 또는 저장소 루트에서 make run P=p01
make reset
```

현재 P01 빌드 입력은 NCS v3.4.2와 공급사 매니페스트 커밋 `f0b8ad4c737745efc407b294d99fbc2279ea09a1`이다. CI는 SDK 설치 후 커밋을 대조하고 SDK 버전에 매핑된 도구 체인을 사용한다. v3.4.2에서도 Zephyr는 4.4.2이며, 서명된 P01 빌드가 별도로 선택하는 Mbed TLS 4.1.2 입력은 그대로 사용한다. [Nordic 릴리스 노트](https://nrfconnectdocs.nordicsemi.com/ncs/latest/nrf/releases_and_maturity/releases/release-notes-3.4.2.html) · [NCS 매니페스트](https://github.com/nrfconnect/sdk-nrf/blob/f0b8ad4c737745efc407b294d99fbc2279ea09a1/west.yml)

### 릴리스 빌드 (WBS2-P01-07, P01-FR-15, P01-NFR-02)

릴리스 빌드는 MCUboot와 서명한 앱 두 이미지로 나온다. MCUboot는 저장소 밖 오프라인 키로 서명한 이미지만 부팅하고, 갱신은 유선 serial recovery(UART 위 SMP)로만 받는다. SW1을 누른 채 리셋하면 recovery에 들어가고 LED1이 켜진다. 앱은 렌탈 셋업만 쓰고 콘솔과 로그를 끈다. UART가 recovery 전용이기 때문이다. 이미지 버전은 `app/VERSION`이며 세션의 firmware 값도 같다.

릴리스 경로는 Nordic MCUboot `5d8736c1fd98959f9a607f59fe1a2d82c7e37bde`를 임시 폴더에 별도로 체크아웃하고, MCUboot 2.5.0에 포함된 upstream TLV 경계 검사를 적용한다. NCS v3.4.2에는 이미지 헤더와 보호 TLV 영역 검사가 있지만 개별 TLV 순회 수정은 별도로 필요하다. 빌드 결과에는 사용한 TLV 소스와 링크된 ELF의 검사 코드에 대한 근거가 들어간다. 일반 `make fw` 개발 빌드는 이 릴리스 스테이징을 거치지 않는다. [MCUboot 보안 공지](https://github.com/mcu-tools/mcuboot/security/advisories/GHSA-4fp3-3qw2-25jv) · [Upstream 수정](https://github.com/mcu-tools/mcuboot/commit/90f4e7c3ebe4cf9a5eb7d981220388ffdefd6d19)

NCS의 알려진 문제 `NCSDK-23761`은 하드웨어 다운그레이드 방지와 FIH LOW를 함께 활성화하는 경우에 해당한다. 오프라인으로 검증한 `--release --approtect --security-counter=2`의 생성 설정은 하드웨어 다운그레이드 방지와 FIH OFF를 사용한다. 해당 확인은 생성 설정과 서명 이미지에 한정되며 보드 부팅과 카운터 읽기 증거는 별도다. [Nordic 알려진 문제](https://nrfconnectdocs.nordicsemi.com/ncs/latest/nrf/releases_and_maturity/known_issues.html?v=v3-4-2)

```bash
python3 scripts/fw.py keygen ~/.nu54-keys/release.pem     # ed25519, 저장소 안 경로는 거부
export NU54_SIGNING_KEY=~/.nu54-keys/release.pem
python3 scripts/fw.py build --pristine --release           # MCUboot + 서명 앱
python3 scripts/fw.py flash                                # MCUboot, 서명 앱 순서로 기록
python3 scripts/fw.py reject-images out/reject             # W12-11: 서명 없음, 다른 키 서명
python3 scripts/fw.py build --pristine --release --approtect  # W12-10: 디버그 잠금
```

- `app/sysbuild-release.conf`: MCUboot, ed25519, 단일 앱 슬롯. `app/sysbuild/mcuboot.conf`: serial recovery, 로그 끔. `app/release.conf`: 앱 설정. `app/approtect.conf`: AP-Protect 잠금(MCUboot와 앱 모두).
- 2026-10-03 빌드 확인(보드 없이): MCUboot 41,664 B로 62 KB boot 파티션에 들어간다. 앱은 193,396 B다. 서명한 앱은 `imgtool verify`가 릴리스 키로 통과하고, MCUboot 예제 키와 `reject-images`의 두 이미지는 통과하지 못한다. 보드에서 부팅·recovery·거부는 아직 확인하지 않았다.
- `--approtect` 이미지를 올리면 디버거로는 erase-all만 할 수 있고, 그러면 기기 키와 셋업이 지워진다. 보드가 1대이므로 12주차 끝에 NVM 덤프 시험을 마친 뒤 한 번만 한다.


NU-54V-DK에서 실행되는 Zephyr 기반 제품이다.

- **소유 범위:** 보드 bring-up, 보호 키와 EOA 서명, 인증 BLE, 기기 설정,
  결제 표시·물리 승인, FOTA, 패스키, 오디오 전송, 찾기, 자원 중재
- **주요 경계:** P02와 C1(BLE/SMP/CTAP/Audio), P04와 C2(임시 결제 승인)
- **선행 위험:** 실제 board revision, pin, flash/RAM, bootloader와 BLE 처리량
- **WBS:** `WBS-P01-01`부터 `WBS-P01-11`

설계 진입점:

- [제품 WBS](../../docs/content/planning/product-wbs-overview.md#p01--nu-54v-dk-펌웨어주변-부품)
- [기기 동시동작과 FOTA](../../docs/content/specifications/device-coexistence-design.md)
- [구현 인터페이스](../../docs/content/specifications/implementation-interfaces.md)
- [기본 페리페럴 bring-up 기록지](../../docs/content/nu54v-basic-peripheral-bringup-log.md)

폴더 구조와 Zephyr 버전(NCS v3.4.2)은 위 개발 절을 따른다.

## C1 owner management and C2 payments

P01 PIN entry stays on the board. Legacy setup and limit-change results
leave the board at the fixed 45-second PIN window deadline.

P01 keeps its stable PSA device identity, rental slots and personal key slot
separate. The C1 owner record carries a monotonic generation, active owner
SPKI fingerprint and personal root format, profile version and backup state.
Revoke advances the generation and clears the active owner binding. Reconnect
requires fresh TLS 1.3 client authentication against the current pin. Existing
single-key PSA roots are non-exportable and require a separate supported root
profile and independent restore provider before a personal receive address
can be enabled.

The runtime-enrolled profile uses the GATT service
`41ba05a0-e0dd-4fd7-901c-4c25d5010000`. Subscribe to notifications on the
characteristic ending in `0002` and write to the characteristic ending in
`0001`. The first write begins with the one-byte route (C1=1, C2=2) followed
by TLS records. After the mutual TLS handshake, the plaintext inside TLS
contains a two-byte big-endian frame length followed by the wire v2 frame.
Use a fresh TLS connection after disconnect, replacement or revocation. On
this board, long-press SW3 or SW4 to start advertising the pilot service.

- C1 uses outer command `MANAGE=1` and reply `REPLY=5`. The begin body
  is `[1, operation, owner-SPKI-SHA256(32)]`. Operations 1, 2 and 3 enroll,
  replace and revoke, respectively. The 130-byte challenge contains the
  version, operation, board identity hash, generation, proposed owner hash,
  random challenge, begin request ID and expiry. To prove possession,
  send `[2, challenge(130), key length, SPKI DER, signature length, signature DER]`
  using P-256 and SHA-256 over the exact challenge. The device then requires
  a gesture on the current physical prompt before changing the generation.
  The proof receives its final reply after physical confirmation. A lost
  connection before that reply leaves the outcome unknown.
  Send `[3]` to query the 77-byte status. Its fields are reply type, state,
  root format, root profile, backup state, generation, identity hash and
  current owner hash. Generation and expiry are unsigned 64-bit big-endian.
- C1 and C2 accept the same 737-byte P05 escrow quote. The board verifies the
  merchant signature, merchant ID and key version, chain, contract, token,
  beneficiary and version, payer, role, generation, amount and expiry. The
  registered branch terminal pin scopes C2. Tenant and branch commitments are
  order-specific. The separate display confirms the complete signed quote.
  SW1 and the board PIN then authorize signing
  `PaymentApproval(bytes32 quoteDigest)`. SW2 returns a one-byte refusal.
- Completed approvals have sixteen persistent receipt slots. Repeating a
  request ID with a different quote, repeating a quote with another request
  ID, and a full receipt store stop signing. A C1 or C2 `STATUS` request can
  retrieve a recorded signature using body
  `[1, originalRequestId(16), SHA256(paymentApprovalPreimage)(32)]` in the
  active payer generation. A one-byte zero means no matching result.
  Reclaim a closed receipt using the current owner management session and the
  independently signed disposition described below. The other slots, including
  unresolved approvals, keep their original records.

### Install and enroll a runtime board

A custom release pins the enrollment authority's public P-256 point. Use the
same signed application image for each board under that authority. Official
channels get the authority pin from the reviewed release policy. A custom
build without a pin continues to provide the legacy rental profile.

```sh
export NU54_INSTALL_AUTHORITY_HEX=$(python3 scripts/enroll_board.py authority-pin /secure/enrollment-authority.pub)
python3 scripts/fw.py build --release --pristine --approtect --security-counter=1
```

Use the installation or counted update path below to stage the signed
application. After first boot, connect one board's wired UART at a controlled
commissioning station. Long-press SW4 and read the device and TLS SPKI
fingerprints from the counted application's UART-only SMP read command. The
wired command returns just those two hashes during the local window. The app
accepts the offline QR through the phone camera, shows both values for explicit
comparison, then checks them against the separately issued owner registration
record before using the owner TLS session. A normal phone setup uses that QR
and issuer record without requiring a cable or desktop at the phone.

The authority-signed station ticket binds the public SW4 nonce to the exact
short-lived station certificate. Sign it in the issuer environment. Retain the
station's ephemeral private key locally and delete it after commissioning.
BLE publishes only the version and rotating nonce through the read
characteristic. The board acts as TLS 1.3 client to the authorized station,
checks its certificate and uses the dedicated `nu54-install/1` profile. The
board then sends the signed offer and a second signature over the TLS exporter
and offer hash. The station checks both signatures and compares the independent
wired fingerprints before accepting the offer or sending the grant. The offer
contains the generated TLS public key, the payer address and the stable device
identifier. Commissioning and renewal preserve existing hardware key slots and
addresses.

```sh
umask 077
station_dir=$(mktemp -d)
python3 scripts/enroll_board.py station-key "$station_dir/station.key" "$station_dir/station.crt"
# Long-press SW4 on the connected board.
python3 scripts/enroll_board.py wired-pins "$uart_port" "$station_dir/pins.json" --qr "$station_dir/owner-pins.svg"
python3 scripts/enroll_board.py challenge "$board_address" "$station_dir/window.bin"
# Run in the issuer environment with the challenge and station certificate.
python3 scripts/enroll_board.py authorize-station --challenge "$station_dir/window.bin" \
  --certificate "$station_dir/station.crt" --authority /secure/enrollment-authority.pem \
  --out "$station_dir/station.ticket"
python3 scripts/enroll_board.py offer "$board_address" offer.bin \
  --station-key "$station_dir/station.key" --ticket "$station_dir/station.ticket" \
  --board-pin "$board_spki_sha256" --device "$device_id"
python3 scripts/enroll_board.py offer-public-key offer.bin board-public.pem
```

Read `board_spki_sha256` and `device_id` from the wired record. The counted
release uses the existing `zephyr,uart-mcumgr` UART at 115200 baud. Image upload
still needs the separate service-mode gate and image approval. Uncounted and
experimental builds without the counted UART configuration require an
independently collected board record. The offline QR contains only the two
board-specific fingerprints. Keep it in the commissioning handoff and
compare it against the physical installation record. The firmware image,
its signing key and the issuer key are separate from the per-board keys.

Have the TLS CA issue a leaf containing that P-256 public key, with one URI SAN
`urn:nu54:role:board`, digital signature usage, server authentication EKU and
an expiry at or after the policy end. Owner and terminal client certificates
use the same TLS CA with their respective role URIs. Their public key
fingerprints use SHA-256 of canonical SPKI DER. The enrollment authority
uses a separate key. It accepts the offer only once it can independently
verify the two board fingerprints. Register the offer's payer address in P05
with the matching role and generation.

The offline issuer takes an authorized JSON policy with positive integer
`revision`, `payer_generation`, `not_after` (UTC epoch seconds),
`merchant_key_version`, `beneficiary_version`, `max_amount` (at most
1,000,000 base units), and `payer_role` (1 owner, 2 renter). It takes
hexadecimal addresses `contract`, `token`, `beneficiary`, `merchant_signer`,
a bytes32 `merchant`, and the SHA-256 SPKI pins `owner` and `terminal`.
The authorized independent display and recovery device each supply a
different P-256 public key. Register the recovery device during initial
commissioning and retain it independently of the phone and display.

```sh
python3 scripts/enroll_board.py issue --offer offer.bin --policy policy.json \
  --ca tls-ca.pem --board-certificate board.pem \
  --display-public display.pub --recovery-public recovery.pub \
  --board-pin "$board_spki_sha256" --device "$device_id" \
  --authority /secure/enrollment-authority.pem --out grant.bin
python3 scripts/enroll_board.py install "$board_address" grant.bin \
  --station-key "$station_dir/station.key" --ticket "$station_dir/station.ticket" \
  --board-pin "$board_spki_sha256" --device "$device_id"
```

Finish within the SW4 window and the one-minute offer lifetime. Reopening SW4
requires a fresh ticket and offer. Installation reports success after the
board confirms the commit over the authenticated TLS channel. A missing
confirmation leaves the outcome unresolved. Reconnect through an authenticated
route to determine the current revision before issuing a new grant. Existing
lease revision, trusted time and recovery authority checks also apply to
renewal.

For the StableNet 8283 v2 commercial testnet, add `--commercial-testnet` to
`issue` after independently verifying the deployed TestUSDC2 token address and
settlement contract. This issues an `N54Q` grant with an explicit
`max_amount` up to 30,000,000 base units (30.00 TestUSDC). The signed grant
binds that cap to the enrolled asset, settlement contract, beneficiary, payer,
and grant revision. Install it at a higher revision when changing the profile.
Ordinary `N54P` grants continue to authorize at most 1,000,000 base units.
Each payment still requires the exact independently displayed quote and
local approval.

The issuing workstation uses its authenticated UTC clock. Spend at most
60 seconds between reading the offer and installing the signed grant. Each
boot requires a fresh signed grant tied to the new board challenge. Write-once
markers distinguish interrupted first setup from loss of established keys,
policy or replay data. Experimental pre-marker installations with existing
keys and absent markers are locked in place; migration requires independent
reconciliation of their stored authority and evidence. Use a
higher revision for a policy change, including extending the lease or
changing the payer generation. The durable time, revision and generation
floors reject regressions. Board clocks use a conservative bound on grant
delivery and subtract that uncertainty from the effective quote lifetime.
Keep payment expiries long enough for independent review and board PIN entry.
The local scripts require Python `cryptography`, `bleak` and `pyOpenSSL`.
Wired pin readback uses `pyserial` and `cbor2`, with `qrcode` for the optional
offline SVG. Provisioning
communicates only with the selected board over BLE.

For payment on a separate trusted display, independently obtain the
exact P05 signed quote for the selected order, then run
`python3 recovery-accessory/confirm_board.py pay "$board_address" display-key.pem quote.bin`
while the board is waiting. The display renders chain, order, contract,
token, payer, beneficiary and integral amount. It signs a fresh board
challenge and the exact quote after local confirmation. The board checks
the display's signature before opening the SW1/SW2 prompt.

The cross-product regression uses the actual P05 quote signer with disposable
keys, then routes the signed bytes through P01's C1/C2 transport and approval
handlers. It also presents a different signed order with the same payer,
amount and merchant on the independent display channel and expects rejection.

```sh
make -C products/p01-device-firmware test
python3 products/p01-device-firmware/test/test_runtime_enrollment.py products/p01-device-firmware/out/host/test_install
NU54_P01_INSTALL_TEST="$PWD/products/p01-device-firmware/out/host/test_install" \
  go test ./products/p05-operations-backoffice/internal/commerce -run TestP01AcceptsTwoP05Quotes -v
```

With `P05_COMMERCE_TEST_DATABASE_URL` set to a disposable migrated local
PostgreSQL fixture, the same P01 runner can test three quotes issued by P05's
normal authenticated TLS API. Pass `NU54_P01_INSTALL_TEST` and run
`TestPostgresLastStockAndSinglePayerWithRestart` in the P05 commerce package.

### Operate the spending wallet profile

This section follows the code and the host tests. Nobody has run it on a board. The profile is for the
8283 test network only. Its rules are in [spending-wallet](../../docs/content/commercial/architecture/spending-wallet.md).
The commands share the station key, ticket and fingerprints of the section above.

**Build.** The profile takes the place of HD recovery in a custom release image. An official channel,
the recovery overlay and the other wallet chains are refused.

```sh
export NU54_SIGNING_KEY=/secure/release.pem
export NU54_INSTALL_AUTHORITY_HEX=$(python3 scripts/enroll_board.py authority-pin /secure/enrollment-authority.pub)
P01_BUILD_DIR=/tmp/p01-spending python3 scripts/fw.py build --release --spending --pristine --security-counter=1
```

**First enrollment.** Write a policy with `payer_role` 1, `payer_generation` 1, `revision` 1, the
OrderEscrow address as `contract`, the EIP-3009 test token as `token` and the phone's fingerprint as
`owner`. `max_amount` is the cap of one payment, at most 30,000,000. `terminal` and the display key are
not used, but the grant format still needs a fingerprint and a P-256 key that belong to nothing else.
Keep the recovery key: the board refuses every later grant with another one. Then take the offer and:

```sh
python3 scripts/enroll_board.py issue --commercial-testnet --spending --offer offer.bin --policy policy.json \
  --ca tls-ca.pem --board-certificate board.pem --display-public display.pub --recovery-public recovery.pub \
  --board-pin "$board_spki_sha256" --device "$device_id" \
  --authority /secure/enrollment-authority.pem --out grant.bin
python3 scripts/enroll_board.py install "$board_address" grant.bin --station-key "$station_dir/station.key" \
  --ticket "$station_dir/station.ticket" --board-pin "$board_spki_sha256" --device "$device_id"
```

The phone registers with SW1 and a new PIN entered twice. The board then has generation 1. This
registration is also when the board makes the wallet key: a board without an owner, new or reset, holds
none, and its status answers carry a zero address until then. A board that never had an owner (generation
0) does not answer the owner status (op 7) at all; the wallet status (0x7f) answers with the zero address.
Such a board holds no PIN either: a registration that does not complete takes back the PIN it set, and a
start without an owner removes any PIN it finds.

**Grant through restarts.** A grant runs until its `not_after`, at most 31 days after it was issued. The
spending profile keeps it in protected storage and takes it up again after a restart, if it is still the
grant the board committed, fits the owner's generation and has not ended. A factory reset removes it. The
certificates in the grant have to last as long as the grant, and so does the phone's owner certificate.

The board has no clock that runs without power. It saves its time every ten minutes and continues from
the saved time, so after a restart its clock is late by the time it was off and by what passed since the
last save. Two consequences:

- The grant's period is counted in powered-on time. Payments still end at the calendar `not_after`: an
  order that ends after it is refused, and the store signs the order's end by the calendar. Status, the
  return address and a reset can go on until the board's own clock reaches the end.
- A late clock refuses honest requests, because their end looks too far ahead (the limits in the table
  below). More than 60 seconds late and orders are refused; more than 210 seconds and every request is.
  Nothing but a new grant moves the clock forward, so after a longer time without power the operator
  installs a new grant as before. This is an open question in the design.

For a new grant open SW4, take a fresh ticket and offer, and issue with a new `not_after` and a higher
`revision`. Its time must not be before the time the board has reached. Keep `payer_generation` equal to
the generation the phone shows, or one above the board's generation while it has no owner. Only the same
policy and certificates may keep their revision.

The time between the offer and the installed grant, at most 60 seconds, is how far the board's latest
possible time may run ahead while that grant is in use, also after a restart. A later grant does not
shorten it, so run the commands without a pause. The phone allows for it:

| Request | Phone sends | Board limit, from its latest possible time |
|---|---|---|
| status (3, 7), 0x7d, 0x7f, receipt status | 90 s | transport 300 s |
| 0x79, running firmware, power, 0x70 to 0x72, 0x76 | 90 s | 95 s (10 s outside this profile) |
| registration, 0x7a, 0x7c, 0x7e | 150 s | transport 300 s, and the 120 s session |
| 0x7b | `validBefore`, at most 540 s ahead and 300 s as a request | 600 s from the board's earliest possible time |
| order | quote expiry, at most 120 s left | 180 s from the board's earliest possible time |

Nothing is accepted past its end: that is still judged by the latest possible time. The phone's clock may be
off by 5 seconds. A request record lasts for its TLS session (128 of them); a frame of an earlier session is
refused by its epoch and counter, and answered orders stay in protected storage.

**After a reset: owner wait.** A board without an owner starts advertising the owner service by itself:
30 s at 100 to 150 ms, then at 1 to 1.2 s until three hours have passed (`CONFIG_NU54_OWNER_WAIT_*` in
`app/Kconfig`). It stops when an owner registers, and SW4 starts it again. Advertising opens nothing: the
grant still needs the SW4 window, and registration needs SW1 and a new PIN. A board with an owner
advertises only after a button, as before. The power figures are estimates until measured; see
[the design](../../docs/content/commercial/architecture/board-secure-key-and-advertising.md).

The board takes two centrals. While it has no owner it goes on advertising as long as one slot is free,
and it disconnects a central that starts neither the installation ticket nor a TLS session within 30
seconds (`CONFIG_NU54_OWNER_WAIT_IDLE_SECONDS`, counted again from the end of each attempt; a ticket that
was started counts for 15 seconds). A tool that connects and waits longer than that has to connect again.
This drops idle centrals. It does not stop a central that means to block the board by reconnecting, by
stalling a TLS start for its 120 seconds, or by rewriting the first ticket fragment while the SW4 window
is open.

**Long standby (not measured).** `--release --spending --standby` builds the application without the UART
driver (`app/standby.conf`). The board default keeps that driver, and it leaves its receiver running, which
keeps the high-frequency clock on between advertising events. MCUboot keeps its UART for serial recovery.
The option cannot be combined with `--security-counter`, whose update path uses the application's UART.
Whether the board then sleeps, and at what current, has to be measured on a board.

**TrustZone build (not for a board yet).** `--secure-key` builds the same profile for
`nu54v_dk/nrf54l15/cpuapp/ns`, where TF-M generates, uses and stores the wallet key in the secure side:

```sh
P01_BUILD_DIR=/tmp/p01-secure-key python3 scripts/fw.py build --release --spending --secure-key --pristine
```

The build checks the generated configuration (TF-M, encrypted storage and persistent PSA keys), that the
linker placed the signing code only in the TF-M image, and the MCUboot signature. It writes no release
manifest. `fw.py flash` refuses every TF-M build, whichever option or board target shows it, any build
directory without its generated configuration, and any image that holds a secure gateway instruction or
lies outside the installed layout, whatever the files beside it say. Its flash layout differs from the installed boards, so it
is never an update for them. Nobody has run it on a board.

**Reset.** The owner resets in the app (SW1 and PIN) or with the buttons: SW2 and SW3 for 5 seconds, the
PIN, then SW1 long within 10 seconds. Every reset reports success only after the key store confirmed
that the wallet key is gone; otherwise the lamp shows a failure and the next start tries again. The
operator's signed reset is the reset of the return desk, and the one for a forgotten PIN. It also runs on
a board that was reset before. There it goes through the whole reset again: the key, the PIN and the
order records are already gone, a grant installed since then is removed with its saved time, and the
board confirms again that no key is left:

1. The board needs a current grant. On the enrolled phone, open the reset screen. On another phone with an
   owner certificate of the same CA, hold SW3 first. That window lasts 120 seconds and is needed again for
   the answer. A board that was reset before has no SW3 window: only the phone its grant names as the
   owner can ask.
2. Compare the request's board id with `device` in the commissioning record, then sign it:

   ```sh
   python3 scripts/enroll_board.py operator-reset "$request" --device "$device_id" \
     --authority /secure/enrollment-authority.pem
   ```

3. Give the `NU54-RESET1-OK:` text back to the phone and press SW1. No PIN is asked for.

The one-time value lasts 15 minutes of board uptime and survives reconnections, so a remote operator can
answer: the phone may disconnect and send the answer in a later session. The board holds one value for its
current generation, in RAM. A new request replaces it, one attempt uses it up, and a restart drops it. The
answer alone erases nothing: the board still checks the authority's signature over board, generation and
value, and SW1 must be pressed. Within the 15 minutes any phone that can open an owner session may present
it, so send the answer only to the owner. The board also refuses while a signed order can still execute,
which takes up to 240 seconds and does not use the value up. The remaining balance is lost.

**Re-enrollment.** The board restarts without a grant. A reset of generation G makes G+1, and the next
registration makes G+2. Issue the next grant with `payer_generation` G+2, a higher `revision`, the new
phone as `owner` and the same recovery key, then register the phone. Never go higher: the board keeps the
largest value and refuses a smaller one later. The wallet key is new, so the account address changes.

**Lamp.** The four LEDs are the board's display. The exact patterns per situation are in
[`app/led-patterns.json`](app/led-patterns.json) (`situations`). A press shows 0.6 seconds of light before
the PIN starts. A signed order or return blinks slowly for 30 seconds. SW2, a wrong PIN and any refusal
after the button are three short flashes, and the PIN wait is five very short ones.

### Reclaim archived payment receipts

The reconciliation authority exports a private JSON disposition for an
archived record. Export these fields from the independently checked settlement
evidence: `device`, `requestId`, `unsignedQuoteSha256`,
`approvalPreimageSha256`, and `disposition` equal to `finalized` or
`finalized_without_effect`. The authority verifies that the archived evidence
is final, or that the finalized chain passed the order expiry without a payment,
before issuing the disposition. Use the exact archived quote.

```sh
python3 scripts/enroll_board.py issue-reclaim --evidence closed.json --quote original-quote.bin \
  --authority /secure/enrollment-authority.pem --out reclaim.hex
```

Paste the disposition into **Payment receipt management** in P02, find the
original board, and select **Reclaim finalized receipt**. Management command
8 binds the proof to one board, original request, exact quote digest, payer
generation, expiry and terminal disposition. A trusted-time lower bound
must be at least 300 seconds past expiry. The device stores that replay
floor before deleting the slot. A reboot can leave the old record present or
the slot free, with the old quote still expired under subsequent trusted
leases. An unresolved result occupies its slot until reconciliation.

### Replace a lost phone without replacing the board

Issue a new owner TLS client certificate for the new phone's own P-256
public key via the established owner-credential workflow. Open the P02
original-board replacement flow, long-press SW3 on the original board,
and select it in P02. P02 reads the board's existing generation and address,
obtains a challenge for the new key and proves possession on the protected
phone key. While it waits for board confirmation, run the following on the
previously registered recovery device.

```sh
python3 recovery-accessory/confirm_board.py replace-phone "$board_address" recovery-key.pem "$challenge" \
  --device "$device_id" --replacement "$new_phone_spki"
```

Confirm the new phone fingerprint on the independent device, then approve on
the original board and enter its PIN there. The durable owner update advances
one generation and preserves the existing keys. The old session is retired,
and bond clearing is requested after commit. P02 reconnects to re-read both
the generation and the same address. To authorize fresh owner payments, the
authority and P05 must authorize the new payer generation. The original-board
flow uses a separate recovery key and works independently of HD backup.

## Counted firmware releases

`fw.py build --release --approtect --security-counter=N` builds the signed
dual-slot MCUboot profile. The receipt covers application and MCUboot
configuration, signed artifacts, the public trust key, source/toolchain
provenance and the manufacturing provisioning image. Hold SW1 and SW2 at startup to open the application's wired maintenance
window. Only image state reads and staging writes to the secondary slot are
accepted. After the transfer completes, long-press SW3 to request the trial
boot. MCUboot validates the trial before boot and preserves the prior image
through its swap and revert logic. The trial confirms itself after successful
application initialization and vault storage checks. The offline validation
checks the signed app fits the application slot, the slots exclude settings,
and truncated signed images are rejected. It records device recovery and OTP
counter readback as pending hardware evidence. The generic `fw.py flash`
command excludes this profile so manufacturing provisioning requires an
independent authorized step.

## Independent HD recovery and personal EVM payments

The signed release includes HD support. Execution requires a counted release
with debug protection, a mapped private accessory UART, and a runtime
authority-signed recovery grant. For a custom image, set
`NU54_RECOVERY_OVERLAY` to the public devicetree file mapping
`nu54-recovery-uart` to the dedicated physical connection. The connection
must be distinct from both diagnostic VCOM ports and MCUboot management. Use
a qualified board/pin mapping. Install the same application image on the
source and target boards with their board-specific runtime credentials. Each
board verifies its own running image digest before activating the grant.

```sh
python3 scripts/enroll_board.py issue-hd --offer offer.bin --board-pin "$board_spki_sha256" \
  --revision "$policy_revision" --source source.pem --target target.pem \
  --ca accessory-ca.pem --accessory accessory.pem \
  --image build/app/zephyr/zephyr.signed.bin --manifest build/release-manifest.json \
  --authority /secure/enrollment-authority.pem --out hd-grant.bin
python3 scripts/enroll_board.py install "$board_address" hd-grant.bin \
  --station-key "$station_dir/station.key" --ticket "$station_dir/station.ticket" \
  --board-pin "$board_spki_sha256" --device "$device_id"
```

Open the local enrollment window and obtain a fresh offer for the HD grant
after the base lease is installed. Grant verification binds the live offer,
current lease revision, measured image, dedicated accessory CA and separate
P-256 identities for the source, target and offline accessory. The private
UART transport uses TLS 1.3 and the exact accessory certificate key. An
operating lease and fresh HD grant are required after restart. HD seed
storage remains HUK-bound with a KMU seed witness. The existing independent
three-signature backup and restore workflow supplies the proof of the
address. [Recovery proof format](../../packages/protocol/v2/recovery.md)

The owner escrow quote payer is the exact backed-up HD EVM address. Use that
address when enrolling the owner in P05. The offer's separate generated
secp256k1 payer is for the renter escrow role. Existing rental and personal
storage slots remain independent. The direct transfer route uses the
same exact backed-up HD account and an EIP-1559 signature in a separate
C1 message; the escrow signature cannot authorize direct transfer.

### Personal EVM transfer

P02 accepts a separately enrolled wallet registry. Configure the wallet
authority with a device ID, exact HD sender address, owner generation, current
lease revision, expiry, and up to three approved network profiles. Each
network names its chain ID, independently verified genesis, HTTPS RPC, fee
budget, and allowed asset addresses, symbols and decimal precision. Run
`scripts/evm_service.py` with the protected registry authority key, HTTPS
certificate, and distinct SHA-256 access-token hashes for the app and the
independent display. The service
listens on loopback. Where needed, expose it to enrolled apps through the
operator's authenticated private HTTPS ingress.

In P02 **Network and asset settings**, enroll the service address and access
code once and verify its signed profile through the original board. The
normal **Receive on EVM** screen lists the enrolled networks. It can present
the previously verified address in read-only mode. Confirming the address
again checks the live HD account and owner generation plus the selected
RPC's chain ID and genesis. The tested profiles are Ethereum Sepolia
(11155111), Base Sepolia (84532), and StableNet (8283).

For **Personal transfer**, choose the account's network and asset and enter
the recipient and decimal amount or choose a saved network-scoped address.
The registered service checks the nonce, estimates gas, encodes the exact
canonical type-2 transaction and signs its network profile, genesis and
lease revision. Supported transfers are native and enrolled ERC-20
`transfer(address,uint256)`. Contract creation, access lists, extra
calldata, unrecognized profiles and unverified fee inputs fail closed.
Base quotes the L1 data estimate and operator fee from the same canonical
block. The actual Base cost can change before inclusion. [Base derivation rules](https://docs.base.org/base-chain/specs/protocol/consensus/derivation) · [GasPriceOracle source](https://github.com/ethereum-optimism/optimism/blob/develop/packages/contracts-bedrock/src/L2/GasPriceOracle.sol)

On the independent display, use `recovery-accessory/confirm_board.py evm`
with its independent `--wallet-service` and `--display-token-file`, or the
exact signed request file, and its independently recorded genesis, board,
sender and authority. The display pulls the same signed review capsule as
the app without using the app's credential. For offline integration, `scripts/evm_transfer.py`
issues the same signed request from prebuilt unsigned bytes. Press SW1 and
enter the board PIN only after comparing the destination, asset, network
and fees. P02 verifies the returned signer and transaction and saves the
immutable network scope. Submission requires a separate user action.
Subsequent status checks use the exact transaction hash and enrolled
genesis, including a recheck of the canonical block after finalized height.
Older checkpoints without a signed genesis retain the hash for external
reconciliation and cannot submit or finalize in the app.

```sh
go run products/p01-device-firmware/test/evm_vectors.go products/p01-device-firmware/out/host/test_evm
```

The local test vectors compare the firmware codec and signed bytes against
go-ethereum for each chain and native/ERC-20 combination. This is code and
build evidence. The actual UART wiring, approved independent display,
hardware-backed storage and chain-provider qualification remain physical
acceptance steps.

### 개인 HD DApp 승인

Owner C1 관리 명령에서 EVM 개인 송금은 9, 기존 비 EVM 공개 키 조회는 13, ERC-20 사용 권한 철회는 14, SIWE 로그인은 15, 외부 WalletConnect의 네이티브 자산 송금은 20을 쓴다. 보드는 한 번에 하나의 승인만 유지한다. 로그인은 `N54S` 권한과 `N54I` 독립 화면 확인을 사용하며 EIP-191 서명만 반환한다. 권한 철회는 `N54V`의 명령 번호까지 인증하고 토큰과 대상이 일치하는 표준 `approve(spender,0)`만 허용한다. 명령 20은 권한 및 독립 화면 승인에 HTTPS 출처, 페어링 토픽, 세션 토픽, 상대 공개 키를 함께 묶는다. 네트워크로 내보내는 서명된 원문은 표준 EIP-1559 형식이다.

명령 36은 등록된 DApp의 양수 토큰 사용 한도를 개인 HD 키로 승인합니다. 서명된 지갑 프로필의 `dapp.allowance`에 `token`, `spender`, `maximum`(최소 단위의 10진 문자열)을 등록하고 해당 체인의 자산에 토큰과 소수 자릿수를 등록합니다. 명령은 만료, 체인과 제네시스, 토큰, 수수료 상한, spender, 최대 토큰 한도, 소수 자릿수 및 HTTPS 출처와 세션 식별자를 원 거래의 해시에 묶습니다. 독립 화면은 별도의 `--dapp-allowance-profile` 파일에서 `chain`, `token`, `spender`, `maximum`, `decimals`, `feeLimit`을 확인하고 `--origin`과 `--genesis`도 대조합니다.

`make test P=p01`은 로그인 해시와 별도 철회의 원시 거래를 go-ethereum으로 교차 검증한다. P06 TestUSDC가 빌드되어 있고 `anvil`과 `cast`가 설치된 환경에서는 `python3 products/p01-device-firmware/test/test_revoke_anvil.py products/p01-device-firmware/out/host/test_evm`으로 127.0.0.1에서 새 체인을 시작해 철회, 브라우저 및 WalletConnect 권한 부여가 각각 확정되는지 확인할 수 있다.

### Authenticated firmware TLS dependency

`python3 scripts/fw.py build --release --approtect --security-counter=2`
prepares the official Mbed TLS 4.1.2 release archive in a private temporary
directory. It checks the repository-pinned SHA-256 and applies the Zephyr CMake
integration patch before invoking west. Set `NU54_MBEDTLS_ARCHIVE` to an offline
copy of that same archive. The build never edits the installed NCS modules.
TLS remains certificate-only 1.3 with fresh handshakes, role and ALPN checks,
and no early data, PSK or tickets.

The protected release receipt records the archive, integration patch and
linked source hashes after checking the compile inputs, linker map and
symbols in the ARM ELF. `tls-stack.cdx.json` inventories Mbed TLS and the
Nordic Oberon PSA implementation selected by the board build. The frozen
west manifest still inventories the SDK checkout, whose bundled Mbed TLS
module is superseded for the release application by the pinned override.
The host adapter test uses the archive's bundled TF-PSA-Crypto.

From this product directory, `sh test/test_mbedtls_peer.sh` exercises the
actual board TLS adapter over a local socket pair with temporary certificates.
It verifies owner and terminal authentication, matching exporter bytes, fresh
connection epochs and rejection of a wrong role or pin. The firmware release
command verifies the signed BIN/HEX, recovery layout and interrupted image
prefixes. Board provisioning, radio links, boot and hardware counter readback
require separate physical acceptance.

The [upstream 4.1.2 release](https://github.com/Mbed-TLS/mbedtls/releases/tag/mbedtls-4.1.2)
fixes the listed advisories, including X.509 name verification (CVE-2026-90470).
The [TLS version confusion advisory](https://mbed-tls.readthedocs.io/en/latest/security-advisories/mbedtls-security-advisory-2026-11-tls-version-confusion/)
applies to a mixed-version TLS client configuration. P01's authenticated server
already used TLS 1.3 exclusively. This update covers the Mbed TLS source
linked into P01. Nordic PSA, mobile TLS and host services have their own
dependency and reachability records.

### 사이트의 유한 ERC-20 사용 권한 부여

명령 36은 승인 전용 정책을 `nu54_evm_decode`에 전달해 양수
`approve(spender, amount)`를 해석한다. 정책에는 토큰 계약, 정확한 spender,
양수 256비트 상한이 들어간다. 0은 별도 철회 경로에서만 허용하며, ERC-20
최댓값은 상한과 요청 모두에서 거부한다. `make test`는 세 체인에서 geth
서명 직렬화 및 해시와 대조하고, 다른 대상과 상한 초과를 거절하는지 확인한다.

아래 표는 일반 지갑의 승인 검토와 W27 보호 저널까지 포함한 인수 기준이다.

| 자료 | 확인하는 값 |
|---|---|
| 사이트 정책 | 활성 등록 사이트와 세션, 체인·제네시스, 소유 계정 및 보드 세대, 정확한 토큰 계약·spender, 양수 유한 상한, 수수료 상한, 만료 및 정책 개정 번호 |
| 서명 전 검토 | 별도 권한 유형, 정책 경계, 정확한 EIP-1559 원문과 해시, 원문 nonce, 변경 전 확정 한도와 요청 한도, 전체 토큰·spender 주소, 수수료 및 기한. 독립 화면에서 같은 원문을 검토한 뒤 물리 입력으로 승인 |
| 보호된 미결 | 권한 부여 유형, 원래 요청 ID와 원문 digest, 소유 세대, 체인·제네시스, nonce, 토큰·spender, 요청 수량. 기존 송금과 같은 계정의 nonce·재생 방지 상태를 공유하고 권한 미결은 송금 결제 합계와 분리 |
| 정산 근거 | 같은 원문과 거래 해시의 canonical 최종 영수증, 확정 블록과 해당 블록에서 조회한 동일 owner·token·spender의 allowance. 요청 수량과 온체인 잔여 한도를 따로 보관 |
| 재개와 전환 | 관리 링크·화면 승인 범위가 바뀌면 새 검토로 돌아가고 이미 서명한 원문은 보존. 이전 보호 기록은 기존 권한과 송금의 의미를 보존하며, 새 유형을 해석할 수 없는 설치판은 기존 C1 보호를 유지한 상태에서 작업을 보류 |

성공 영수증은 권한 변경만 증명한다. 이후 제3자의 `transferFrom`이 발생하면
그 별도 거래와 실제 지급 금액을 조회해야 한다. C1 송금 증거로 권한 미결을
해제하거나, 시계 만료·연결 종료만으로 발행된 서명을 폐기하지 않는다.
일반 소유자 지갑은 명령 36을 사용한다. 로컬 Anvil에서 P02의 브라우저·외부 연결에 해당하는 요청 원문을 C 게이트웨이로 서명하고 두 한도 변경을 확인했다. W27의 `CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY` 구성은 보호 저널이 양수 한도 미결을 지원할 때까지 명령 36을 거절한다. [기기 명령 분기](app/src/device_evm.c) · [체인 검증 경로](test/test_revoke_anvil.py)

# P01 · NU-54V-DK device firmware

> **DF-20260925-02 기준 (2026-09-25):** 이번 12주 사이클의 범위와 설계는 [기획](../../docs/content/products/p01/plan.md) · [SRS](../../docs/content/products/p01/srs.md) · [유즈케이스](../../docs/content/products/p01/use-cases.md) · [설계](../../docs/content/products/p01/design.md)가 정한다. 아래 원문은 DF-20260920-01 기준이며 충돌하면 위 문서를 따른다.

## 개발 (DF-20260925-02)

| 경로 | 내용 |
|---|---|
| `core/` | 보드와 무관한 C 코드. Zephyr 헤더를 쓰지 않아 host에서 빌드·시험한다. BLE 메시지 조각 재조립(프로토콜 4절), EIP-712 digest(`nu54_eip712`), 서명자 복원과 low-s·v 계산(`nu54_sig`), keccak-256(`nu54_keccak`), 결정적 CBOR 디코딩·스키마 검사·인코딩(`nu54_cbor`), 결제·셋업 세션(`nu54_session`, 서명과 난수는 플랫폼 콜백, 버튼은 비동기), 버튼 PIN 입력(`nu54_pin_entry`, 4자리), 결제 세션 보안 채널(4.1절: `nu54_secure`의 ECDH, HKDF와 AES-GCM은 플랫폼 콜백으로 보드는 PSA, host 시험은 OpenSSL) |
| `third_party/` | libsecp256k1 v0.8.0(MIT), Keccak compact(CC0). 출처 커밋과 빌드 설정은 [`third_party/README.md`](third_party/README.md) |
| `app/` | Zephyr 앱(C). 부팅 때 저장된 셋업(`src/device_setup.c`: 셋업 기록·PIN HMAC·실패 횟수는 PSA ITS, nonce는 settings)을 읽고 결제 링크(`src/pay_link.c`)를 시작한 뒤, 키가 있으면 자체 검증한다. 셋업이 없으면 기본 빌드는 7주차 고정 셋업으로 키를 만들고, `fw.py build --rental`(`app/rental.conf`)은 UNPROVISIONED로 셋업 세션을 기다린다. 결제 링크가 하는 일은 BLE 광고와 GATT 서비스, 조각 재조립과 envelope digest, `nu54_session`, 응답 조각 notify. 7주차 개발 빌드의 버튼·LED·셋업은 [설계](../../docs/content/products/p01/design.md) 8절 |
| `boards/nucode/nu54v_dk/` | 제조사 보드 패키지(MIT, 출처 커밋은 `VENDORED.md`) |
| `test/` | `core/`의 host 단위 시험(CMake + CTest). `eip712_vectors`, `frame_vectors`, `cbor_vectors`, `session_vectors`는 공용 벡터로 시험하며 적합성 harness가 실행한다 |
| `scripts/fw.py` | NCS 툴체인 안에서 west 빌드와 pyOCD 플래시를 실행한다. 릴리스 빌드, 서명 키 생성, 거부 시험 이미지(아래) |
| `tools/bringup/` | 보드 bring-up 도구: 시리얼 CLI, BLE 스캔·연결, 버튼 시뮬레이션, 레지스터 읽기, 결제 BLE 전송 브리지(`pay_bridge.py`) ([README](tools/bringup/README.md)) |

기기 키(7주차 경로, P01-FR-04의 non-secure 단계): PSA Crypto가 CRACEN으로 secp256k1 키를 한 번 만들고 Zephyr Secure storage(ZMS, `storage` 파티션)에 영속 저장한다. 키 정책은 서명만 허용하고 내보내기를 막는다. 서명은 결정론적 ECDSA(RFC 6979)라서 같은 digest에는 항상 같은 서명이 나오고, `nu54_sig_finish`가 low-s와 v를 맞춘다. 8주차에 TF-M(`/ns` 변형)과 암호화 ITS로 옮긴다. 2026-10-01 보드 확인: 리셋 뒤에도 같은 키를 열고, 기기 로그의 digest와 서명을 host에서 복원하면 기기 주소와 같다.

```bash
make test         # host에서 core/ 시험 (NCS 불필요)
make fw           # nu54v_dk/nrf54l15/cpuapp, NCS v3.4.1
make flash        # 또는 저장소 루트에서 make run P=p01
make reset
```

### 릴리스 빌드 (WBS2-P01-07, P01-FR-15, P01-NFR-02)

릴리스 빌드는 MCUboot와 서명한 앱 두 이미지로 나온다. MCUboot는 저장소 밖 오프라인 키로 서명한 이미지만 부팅하고, 갱신은 유선 serial recovery(UART 위 SMP)로만 받는다. SW1을 누른 채 리셋하면 recovery에 들어가고 LED1이 켜진다. 앱은 렌탈 셋업만 쓰고 콘솔과 로그를 끈다. UART가 recovery 전용이기 때문이다. 이미지 버전은 `app/VERSION`이며 세션의 firmware 값도 같다.

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

폴더 구조와 Zephyr 버전(NCS v3.4.1)은 위 개발 절을 따른다.

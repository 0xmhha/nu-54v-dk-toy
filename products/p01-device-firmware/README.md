# P01 · NU-54V-DK device firmware

> **DF-20260925-02 기준 (2026-09-25):** 이번 12주 사이클의 범위와 설계는 [기획](../../docs/content/products/p01/plan.md) · [SRS](../../docs/content/products/p01/srs.md) · [유즈케이스](../../docs/content/products/p01/use-cases.md) · [설계](../../docs/content/products/p01/design.md)가 정한다. 아래 원문은 DF-20260920-01 기준이며 충돌하면 위 문서를 따른다.

## 개발 (DF-20260925-02)

| 경로 | 내용 |
|---|---|
| `core/` | 보드와 무관한 C 코드. Zephyr 헤더를 쓰지 않아 host에서 빌드·시험한다. BLE 메시지 조각 재조립(프로토콜 4절), EIP-712 digest(`nu54_eip712`), 서명자 복원과 low-s·v 계산(`nu54_sig`), keccak-256(`nu54_keccak`), 결정적 CBOR 디코딩·스키마 검사·인코딩(`nu54_cbor`), 결제·셋업 세션(`nu54_session`, 서명과 난수는 플랫폼 콜백, 버튼은 비동기) |
| `third_party/` | libsecp256k1 v0.8.0(MIT), Keccak compact(CC0). 출처 커밋과 빌드 설정은 [`third_party/README.md`](third_party/README.md) |
| `app/` | Zephyr 앱(C). 부팅 때 저장된 셋업(`src/device_setup.c`: 셋업 기록·PIN HMAC·실패 횟수는 PSA ITS, nonce는 settings)을 읽고 결제 링크(`src/pay_link.c`)를 시작한 뒤, 키가 있으면 자체 검증한다. 셋업이 없으면 기본 빌드는 7주차 고정 셋업으로 키를 만들고, `fw.py build --rental`(`app/rental.conf`)은 UNPROVISIONED로 셋업 세션을 기다린다. 결제 링크가 하는 일은 BLE 광고와 GATT 서비스, 조각 재조립과 envelope digest, `nu54_session`, 응답 조각 notify. 7주차 개발 빌드의 버튼·LED·셋업은 [설계](../../docs/content/products/p01/design.md) 8절 |
| `boards/nucode/nu54v_dk/` | 제조사 보드 패키지(MIT, 출처 커밋은 `VENDORED.md`) |
| `test/` | `core/`의 host 단위 시험(CMake + CTest). `eip712_vectors`, `frame_vectors`, `cbor_vectors`, `session_vectors`는 공용 벡터로 시험하며 적합성 harness가 실행한다 |
| `scripts/fw.py` | NCS 툴체인 안에서 west 빌드와 pyOCD 플래시를 실행한다 |
| `tools/bringup/` | 보드 bring-up 도구: 시리얼 CLI, BLE 스캔·연결, 버튼 시뮬레이션, 레지스터 읽기, 결제 BLE 전송 브리지(`pay_bridge.py`) ([README](tools/bringup/README.md)) |

기기 키(7주차 경로, P01-FR-04의 non-secure 단계): PSA Crypto가 CRACEN으로 secp256k1 키를 한 번 만들고 Zephyr Secure storage(ZMS, `storage` 파티션)에 영속 저장한다. 키 정책은 서명만 허용하고 내보내기를 막는다. 서명은 결정론적 ECDSA(RFC 6979)라서 같은 digest에는 항상 같은 서명이 나오고, `nu54_sig_finish`가 low-s와 v를 맞춘다. 8주차에 TF-M(`/ns` 변형)과 암호화 ITS로 옮긴다. 2026-10-01 보드 확인: 리셋 뒤에도 같은 키를 열고, 기기 로그의 digest와 서명을 host에서 복원하면 기기 주소와 같다.

```bash
make test         # host에서 core/ 시험 (NCS 불필요)
make fw           # nu54v_dk/nrf54l15/cpuapp, NCS v3.4.1
make flash        # 또는 저장소 루트에서 make run P=p01
make reset
```


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

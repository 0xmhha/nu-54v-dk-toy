# 05편 집필 근거와 검토 기록

내부 편집 자료입니다. Medium 본문·레퍼런스에 올리지 않습니다.

- 확인일: 2026-10-19
- 확인한 체크아웃: `ee12b75` 위의 현재 작업 트리. 원고 외 펌웨어·시험·빌드 설정은 수정하지 않았습니다.
- 원고: `05-secure-signing.md`, `05-secure-signing-en.md`
- 상태: 로컬 사전 원고. Medium 초안 생성·공개 발행·Git 커밋·push는 하지 않았습니다.

## 1. 현재 구현과 목표의 구분

- `products/p01-device-firmware/README.md`는 현재 PSA/CRACEN 키 경로를 non-secure 개발 단계로 설명합니다.
- `products/p01-device-firmware/scripts/fw.py`의 보드 대상은 `nu54v_dk/nrf54l15/cpuapp`입니다.
- 보드 패키지·앱 소스·설정을 검색했지만 `/ns` 변형, TF-M 키 서비스, secure GPIO 승인 구현, 실제 거절 시험 결과는 확인하지 못했습니다. 이 결론은 현재 체크아웃에 한정합니다.
- `/ns`가 없는 앱의 CPU 보안 속성을 추측해 단정하지 않습니다. 공개 원고는 **일반 앱과 키 서비스 사이에 TF-M 격리 경계가 구현되지 않은 개발 경로**로 설명합니다.
- `docs/content/products/p01/design.md` 1·2·4·7절은 목표 설계입니다. 완료된 구현으로 인용하지 않습니다. 일반 앱이 만든 digest와 폰 표시 필드 사이의 신뢰 가정도 2절에 명시돼 있습니다.
- 설계에 암호화 ITS·KMU 지원 범위가 적혀 있지만, 현재 구현·실기 증거가 없으므로 공개 원고에서 완료된 보호로 설명하지 않았습니다.
- 키 서비스 인터페이스 `register_digest`와 `sign_digest`는 설계에만 있으며, 원고에서도 미구현으로 표시했습니다.

## 2. 코드 발췌와 주장 대조

아래 경로는 이 문서 기준 프로젝트 루트 상대 경로입니다. 공개 원고에는 파일 이름만 사용합니다. 숫자는 검토 시점의 줄 번호입니다.

| 원고 부분 | 근거 | 확인한 내용 |
|---|---|---|
| 1절 키 정책 | `products/p01-device-firmware/app/src/device_key.c:43` | 영속 키·secp256k1·256비트·SIGN_HASH, EXPORT 권한 없음. 연속 5줄 발췌 |
| 1절 공개키 읽기 | 같은 파일 23줄, `app/prj.conf:14` | 공개키 export 빌드 설정과 키별 사용 정책을 구분 |
| 2절 서명 | 같은 파일 72줄 | 32바이트 digest, PSA 서명, 64바이트 길이 검사. 함수 전체 발췌 |
| 2절 후처리 | `products/p01-device-firmware/core/src/nu54_sig.c` | low-s 정규화, v 27/28, 복원 주소 대조. 개인키로 서명하는 함수가 아님 |
| 3절 버튼 | `products/p01-device-firmware/app/src/main.c:175` | SW1 클릭 승인·SW2 클릭 거절. 연속 4줄 발췌 |
| 3절 부팅 서명 | 같은 파일 42·132줄 | key_self_test가 같은 device_key_sign을 호출. 현 함수 안에 승인 검사 없음 |
| 3절 작업 큐 | `products/p01-device-firmware/app/src/pay_link.c:322`, 346줄 | 버튼을 큐에 넣고 세션 처리. 구조 분리와 하드웨어 보안 격리는 구분 |
| 3절 상태 확인 | `products/p01-device-firmware/core/src/nu54_session.c:189` | pending 종류별 버튼 처리 분기 |
| 5절 nonce 저장 | `products/p01-device-firmware/core/src/nu54_session_pay.c:133` | 다음 nonce 저장 뒤 서명. callback은 조건부이며 보드에서는 연결됨. 연속 6줄 발췌 |
| 5절 보드 저장 연결 | `products/p01-device-firmware/app/src/device_setup.c:50`, 324줄 | settings_save_one에 연결. 모든 전원 차단 시점 검증이라고 주장하지 않음 |
| 5절 중단 정리 | `products/p01-device-firmware/core/src/nu54_session_util.c:118` | setup PIN 대기 중 wipe callback 호출, pending 해제. 함수 전체 발췌 |
| 5절 연결 해제 시험 | `products/p01-device-firmware/test/test_session.c:295` | wipe 호출 횟수·pending·session_open 검사. 실제 저장소 지우기 시험이 아닌 host 대체 구현 |
| 5절 출시 설정 | `products/p01-device-firmware/app/release.conf:3` | 개발 셋업·unpaired anchor 끔, secure session·phone 필요. 연속 4줄 발췌 |

코드 발췌 6곳은 양 언어에서 같고, 줄 앞뒤 들여쓰기만 정규화하여 기존 소스의 연속 부분과 일치함을 확인했습니다. 로그 블록은 실제 출력에서 핵심 줄만 발췌했습니다.

## 3. 실제 변경 이력과 코드에서 읽은 주의점

- `ea5a476` (2026-10-01): PSA 영속 기기 키, CRACEN, Zephyr Secure storage와 서명 자체 시험 도입.
- `0d08eaa` (2026-10-03): 셋업 저장, PIN 검증, 중단 셋업의 새 키 정리와 대여 빌드 도입. nonce 저장 순서·키 정리 의도는 현재 코드로 확인합니다.
- `0e02f93` (2026-10-05): BLE 처리·세션 작업 큐·LED 소유권 분리. 동시 실행 구조를 보안 구역 격리로 표현하지 않습니다.
- `df87a2f` (2026-10-07): 고정 셋업과 개발용 TimeAnchor 예외 분리. 중단 셋업은 응답용 폰 연결을 보존하고, 기존 본딩 정보 정리로 다시 페어링하도록 수정. 커밋 설명과 실제 변경 부분을 함께 확인했습니다.
- nonce 저장과 정리 순서는 **현재 코드에서 확인한 의도·주의점**입니다. 전원 손실이나 키 유출이 실제로 발생했다고 쓰지 않았습니다.
- 공개 원고에는 커밋·PR 번호, 내부 요구사항 ID와 제품 ID, 실제 계정·주소·키 값·트랜잭션 해시를 넣지 않았습니다.

## 4. 2026-10-19 실행한 시험

실행 장소는 로컬 임시 디렉터리이며 공개 원고에서는 내부 경로와 명령 출력을 제외하고 결과만 사용했습니다.

```sh
cmake -S products/p01-device-firmware/test -B /private/tmp/nu54-05-host-tests
cmake --build /private/tmp/nu54-05-host-tests -j 4
ctest --test-dir /private/tmp/nu54-05-host-tests --output-on-failure
/private/tmp/nu54-05-host-tests/test_eip712
/private/tmp/nu54-05-host-tests/test_session
```

결과:

```text
100% tests passed, 0 tests failed out of 8
ok: 9 EIP-712 vectors, 0 failures
pin.entry body: 60 bytes
ok: 38 session scenarios, 0 failures
```

- 8개 시험 이름: `frame`, `button`, `pin_entry`, `links`, `eip712_vectors`, `frame_vectors`, `cbor_vectors`, `session_vectors`.
- 9개 EIP-712 벡터와 38개 세션 시나리오는 위 시험 묶음의 하위 항목입니다. 8+9+38로 합산하지 않았습니다.
- host session 시험은 서명·암호·저장을 시험용 구현으로 대체합니다. 보드 PSA, CRACEN, TF-M, 실제 GPIO 보안 속성, 전원 손실 시험을 대신하지 않습니다.
- 현재 key API의 비내보내기 정책은 코드와 PSA 규격으로 설명합니다. 이번에 실제 보드에서 private-key export를 시도한 결과는 없습니다.
- 보드 플래시·보안 속성 변경·키 재생성·새 체인 거래를 실행하지 않았습니다.

## 5. 외부 기술 자료

2026-10-19 공식 자료를 열어 확인했습니다. SDK 버전별 지원을 새로 확정하거나 현재 펌웨어의 통합 성공을 주장하는 근거가 아닙니다.

1. PSA Crypto API 1.1 Key policies: https://arm-software.github.io/psa-api/crypto/1.1/api/keys/policy.html — usage policy와 공개키 export 구분.
2. Nordic Hardware isolation: https://academy.nordicsemi.com/courses/nrf54l-series-express-course/lessons/lesson-4-security-features/topic/hardware-isolation/ — TrustZone의 메모리·주변장치 접근 경계.
3. PSA Crypto API 1.1 Asymmetric signature: https://arm-software.github.io/psa-api/crypto/1.1/api/ops/sign.html — sign_hash와 sign_message의 차이, ECDSA r||s 표현. 현재 SIGN_ALG의 SHA_256 명칭이 EIP-712 digest를 재해시한다는 설명은 하지 않음. 특정 알고리즘 선택의 규격 적합성을 새로 인증한 것은 아님.
4. TF-M Crypto Service Integration Guide: https://trustedfirmware-m.readthedocs.io/en/latest/integration_guide/services/tfm_crypto_integration_guide.html — 암호 서비스·클라이언트 인터페이스·저장소 연계. 결제 버튼 승인은 프로젝트의 별도 설계.

## 6. 이미지·공개 검토

- `assets/05/build-diagrams.py`로 PNG 6장을 직접 작성했습니다. 이전 편과 같은 색·서체를 사용했습니다.
- 각 PNG는 1600×1100입니다. 두 언어 모두 실제 구현·미구현 목표·시험 한계를 표시하고, 그림을 나란히 열어 잘림·겹침을 확인했습니다.
- 시험 인포그래픽은 시험 묶음 8개를 동일 크기 카드 8개로 표시했습니다. 보안 점수·성능 비율·평균을 만들지 않았습니다.
- 코드의 키 ID 숫자, 실제 key material, 테스트 키·주소, 디버그 출력의 digest·signature, 내부 절대 경로와 프로젝트 저장소 링크를 공개 원고·이미지에 넣지 않았습니다.
- 분량 기준은 코드·캡션·INDEX·레퍼런스를 제외한 한글 본문으로 점검합니다. 코드 설명의 의미와 실제 숫자를 줄여 맞추지 않습니다.
- 공개 전에는 실제 TF-M 구현과 보드 거절 시험 근거를 반영할지 다시 검토합니다. 현재 원고는 구현 전 기준을 명시했으므로, 그대로 발행하더라도 이전 완료 후기로 제목·시제를 바꾸면 안 됩니다.

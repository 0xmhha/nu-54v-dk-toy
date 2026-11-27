# 06편 집필 근거와 검토 메모

이 파일은 로컬 편집·검토 자료입니다. Medium 본문에 올리지 않습니다.

- 작성·코드 확인·재시험: 2026-11-09.
- 대상: `06-unpaired-payment-channel.md`, `06-unpaired-payment-channel-en.md`.
- 상태: 한글·영문 원고와 언어별 도식 3장을 Medium 초안으로 저장한 뒤 2026-11-09 사용자의 요청으로 공개 발행했습니다. Git 커밋·push는 하지 않았습니다. 비공개 링크와 확인 화면은 저장소 밖 로컬 작업 기록으로 분리했습니다.
- 이번 범위는 06편입니다. 07~09편은 기존 주제·순서를 유지하고 실제 근거 확인 후 이어서 집필합니다.

## 1. 실제 시점과 오래된 설계 문구

- `docs/content/acceptance/gate-w7.md` 첫 문단에는 평문으로 통과한다는 초기 계획 문구가 남아 있습니다. 그러나 같은 파일의 구성·판정·waiver 표는 **10/1 개발 실행은 평문, 10/5 Android 실기 결제는 보안 채널 사용**으로 구분합니다.
- `docs/content/specifications/protocol/payment-protocol.md` 4.1절 첫 문단, `docs/content/products/p01/design.md`의 일정 설명에도 이후 주차에 채널을 넣는다는 과거 계획이 있습니다. 원고에서 구현 날짜로 사용하지 않았습니다.
- 채널 규격·시뮬레이터·펌웨어 변경 `93fa51a`, 키오스크 통합 `3dcb7a8`은 모두 2026-10-03입니다. 10/5 실기 기록의 보안 채널 사용과 맞습니다.
- 10/5 결제 1건은 과거 기록입니다. 11/9 이번 작업에서는 보드 플래시·무선 연결·체인 거래를 실행하지 않았습니다.
- 읽기 전용 근거 문서와 펌웨어·앱 코드는 수정하지 않았습니다. 오래된 문구를 교정하는 작업은 이번 집필과 별개입니다.

## 2. 본문 주장과 코드 대조

아래는 프로젝트 루트 기준 상대 경로입니다. 공개 글에는 파일 이름과 기능 이름만 사용합니다.

| 주장 | 근거 | 범위·주의점 |
|---|---|---|
| 폰 본딩과 키오스크 비페어링 구분 | `docs/content/specifications/protocol/payment-protocol.md` 3절, `products/p01-device-firmware/app/src/ble_links.c` | 페어링하지 않는 연결에도 응용 계층 보호를 적용 |
| 셋업 후 passkey LESC 요구 | `ble_links.c`의 `ble_links_bonded`, `auth_passkey`, `pairing_complete`, `drop_old_bonds` | 보안 수준 L4와 SC 플래그 검사. 이름만 보고 bond 저장 여부만 검사한다고 쓰지 않음 |
| 최초 Just Works와 이후 재본딩 | `ble_links.c`의 `auth_just_works`, `products/p01-device-firmware/app/src/pay_link.c`의 `on_renter_event` | 초기 예외를 정상 대여 중 폰 연결과 구분 |
| 운영자→가맹점→임시키의 서명 연결 | `products/p01-device-firmware/core/src/nu54_session_channel.c`의 `ss_operator_attestation`, `ss_kiosk_merchant` | 현재 키오스크 인증 경로. 임시키 생성·지갑 서명 내부 코드는 공개 발췌하지 않음 |
| 곡선 위 점 검사 | `nu54_session_channel.c`의 `ss_kiosk_merchant`, `core/src/nu54_secure.c` | 잘못된 공개키를 거절하는 조건 |
| 세션 가맹점 일치와 증명서 유효 기간 | `core/src/nu54_session_pay.c`의 `ss_identify` | 세션 open에서 기간 검사를 끝낸다고 쓰지 않음 |
| AES-GCM, 128비트 키, 16바이트 태그, AAD 없음 | `products/p01-device-firmware/app/src/device_setup.c`의 `hkdf`, `gcm` | 통신 보호이며 TrustZone 키 경계의 구현 근거가 아님 |
| IV 방향·번호 | `nu54_session_channel.c`의 `make_iv`, `packages/protocol/ts/src/secure.ts` | 규격 1+11바이트, C는 uint64_t로 뒤 8바이트만 채움. 88비트 전체 운용·wrap 종료 검증을 주장하지 않음 |
| 수신 성공 때만 번호 증가 | `nu54_session_channel.c`의 `ss_channel_open` | 함수 일부를 연속 발췌. 태그 실패 뒤 세션 종료는 호출 측 동작 |
| 실패 응답 암호화 후 종료 | `core/src/nu54_session.c`의 `nu54_session_handle`, `ss_channel_end` | 세션이 닫힌 다음 암호문에 대한 평문 BAD_FRAME과 구분 |
| 폰 링크가 키오스크 수신 번호를 건드리지 않음 | `ss_channel_begin`의 foreign 검사, `test/test_session.c`의 별도 링크 삽입 검사 | 실제 무선 다중 연결 재시험의 대체물이 아님 |
| 상대 임시키 누락 때 plaintext fallback 없음 | `products/p04-merchant-kiosk/src/payment/open.ts` | 원래 secure 옵션으로 요청한 경로를 기준으로 설명 |
| 장기 신원의 완전한 상호 인증이라고 표현하지 않음 | 같은 파일과 `core/src/nu54_session.c`의 `session.open.ok` 필드 | 응답 임시 공개키를 장기 기기 키로 서명하는 필드가 없음. 공격 재현이나 정식 보안 감사 결과라고 쓰지 않음 |
| 과거 시각·연결 문제와 수정 | `gate-w7.md` 결함 절, `products/p04-merchant-kiosk/src/payment/session.ts` | 90초 검사에는 chainTime 입력 조건이 있음. 소진·재시도 설명은 10/5 수정 기록을 근거로 사용 |

양 언어 코드 블록 2개는 동일하며, 들여쓰기를 정규화하여 기존 소스의 연속 부분과 대조했습니다. 변경 이력의 내부 ID·커밋·PR 번호·실제 테스트 키와 주소는 공개 본문에 넣지 않았습니다.

## 3. 2026-11-09 실행한 기존 시험

펌웨어 코어:

```sh
cmake -S products/p01-device-firmware/test -B /private/tmp/nu54-06-host-tests
cmake --build /private/tmp/nu54-06-host-tests -j 4
ctest --test-dir /private/tmp/nu54-06-host-tests --output-on-failure
/private/tmp/nu54-06-host-tests/test_session
```

핵심 출력:

```text
100% tests passed, 0 tests failed out of 8
ok: 38 session scenarios, 0 failures
```

시험 이름은 `frame`, `button`, `pin_entry`, `links`, `eip712_vectors`, `frame_vectors`, `cbor_vectors`, `session_vectors`입니다. 38개 시나리오는 마지막 시험에 포함되므로 8개와 더해 세지 않습니다. 시나리오에는 채널 정상 동작·잘못된 서명/곡선점·태그 실패·다른 가맹점·출시 조건의 평문 거절·미설정 기기 거절이 있습니다. 서명·저장·난수는 시험용 플랫폼이며, HKDF와 GCM은 OpenSSL로 계산합니다. 38개를 모두 보안 채널 공격 시험으로 표현하지 않습니다.

통신 라이브러리 디렉터리 `packages/protocol/ts`에서:

```sh
node --experimental-strip-types --test test/secure.test.ts
```

출력: tests 4, pass 4, fail 0. Node.js crypto(OpenSSL)와 ECDH/HKDF/AES-GCM 결과 대조, 방향·번호·변조 거절, 유효하지 않은 키 입력 검사를 포함합니다. 시험 벡터의 결정적 난수는 보드 난수 품질의 근거가 아닙니다.

키오스크 디렉터리 `products/p04-merchant-kiosk`에서:

```sh
pnpm exec jest test/secure.test.ts --runInBand
```

출력: Test Suites 1 passed, Tests 6 passed. 시험은 다음을 검사합니다.

1. 정상 채널: `session.open.ok` 이후 본문이 CBOR로 바로 해독되지 않고 정상 결제로 진행.
2. 다른 서명자의 키오스크 임시키 거절.
3. 기기 임시키 없는 응답 거절, 평문 fallback 없음.
4. require-secure 구성의 평문 결제 거절, 보안 결제 허용.
5. 바깥 프레임 digest를 다시 맞춰도 변조된 응답의 GCM 태그 실패.
6. 한도 변경도 보안 채널로 진행.

4개와 6개는 서로 다른 시험 파일의 수이며, 제품 보안 점수나 공격 방어율로 환산하지 않습니다. 새 시험 코드를 추가하지 않았습니다. 보드 PSA·CRACEN·TF-M·전원 손실·실제 무선 환경·상호 인증의 완전성을 검증한 결과가 아닙니다.

## 4. 외부 자료 확인

2026-11-09 공식 원문을 열어 확인했습니다. 특정 SDK 판본에서 새 통합 성공을 인증한 것은 아닙니다.

- [Zephyr Connection management](https://docs.zephyrproject.org/latest/doxygen/html/group__bt__conn.html): Security levels의 L2/L4 정의.
- [RFC 5869](https://www.rfc-editor.org/rfc/rfc5869): 키 유도 목적과 extract/expand 개념. 원고에는 규격 원문을 길게 옮기지 않음.
- [NIST SP 800-38D](https://csrc.nist.gov/pubs/sp/800/38/d/final), [원문 PDF](https://nvlpubs.nist.gov/nistpubs/Legacy/SP/nistspecialpublication800-38d.pdf): 인증 암호화와 8절 IV 유일성. 이 규격을 사용한다는 것과 제품 인증을 받았다는 것은 다름.
- [RFC 8949](https://www.rfc-editor.org/rfc/rfc8949): CBOR의 형식과 목적.

## 5. 이미지와 공개 검토

- `assets/06/build-diagrams.py`로 한글·영문 각 3장, 총 6장의 1600×1100 PNG를 직접 작성했습니다. AI 생성 사진·가상 앱 화면은 없습니다.
- 이미지 1은 LED 4개·버튼 4개의 화면 없는 기기를 개략도로 표시합니다. 실제 제품 사진으로 설명하지 않습니다.
- 이미지 2는 단계 순서만 표시하며 간격을 소요 시간으로 해석하지 않습니다. 최초 요청·응답의 평문 범위를 표시합니다.
- 이미지 3은 같은 폭의 12개 칸으로 실제 바이트 수를 표시합니다. 방향 1칸, C 구현의 고정 0 3칸, 카운터 8칸입니다. 프로토콜의 11바이트 필드와 C 구현 범위를 함께 표시합니다.
- 6장을 나란히 열어 한글·영문 잘림과 겹침, 수치·순서를 확인했습니다. 임의의 속도·비율·잔액은 넣지 않았습니다.
- 한글 본문은 코드·캡션·INDEX·제목·부제·태그·레퍼런스를 제외해 약 3,800자입니다. 코드와 로그를 분량에 맞추려고 바꾸지 않았습니다.
- 이전 편 URL은 05편이 발행된 뒤 입력합니다. 없는 URL이나 현재 글의 Medium 초안 주소를 만들지 않았습니다.
- 프로젝트 저장소 링크는 마지막 편까지 공개하지 않습니다. 실제 키·주소·해시·계정·내부 절대 경로를 공개 원고와 PNG에서 제외했습니다.

## 6. Medium 초안 저장 확인

- 2026-11-09 사용자의 요청으로 한글·영문 초안을 작성했습니다. 제목·부제와 본문 전체를 넣고, 각 언어의 PNG 3장과 이탤릭 캡션 3개를 삽입했습니다.
- C 코드 2개는 언어를 C로, 실행 출력 2개는 Plain Text로 지정했습니다. 본문의 해시태그와 별개로 주제 Nucode·누코드·Nu54vdk·누코더스·Nucoders 5개를 입력했습니다.
- 두 편집기를 다시 열어 Draft Saved, 이미지 로드, 코드·로그와 모든 문단의 원고 대조, 주제 5개 저장을 확인했습니다. Medium이 바꾼 비분리 공백과 문장 부호는 대조 시 정규화했습니다. 코드·로그·숫자·뜻을 바꿔 게시하지 않았습니다.
- 발행 준비 메모와 이미지 자리 표시는 Medium 본문에 없습니다. 05편 미발행 안내는 유지했습니다.
- Share draft link 메뉴에서 두 검토 링크를 확인했습니다. 공개·예약 발행은 하지 않았습니다.

## 7. 공개 발행 확인

2026-11-09 사용자가 두 판본 발행을 요청했습니다. 태그 Nucode·누코드·Nu54vdk·누코더스·Nucoders를 발행 전과 공개 후 모두 확인했습니다. 두 판본의 발행 성공 안내, 공개 본문, 이미지 3장 로드와 캡션 3개를 확인했습니다. 발행 전 이전 글 URL 추가 메모만 본문에서 지우고 05편 제목은 남겼습니다.

- 한글: https://medium.com/@bc.0x4861/nu-54v-dk-06-%ED%8E%98%EC%96%B4%EB%A7%81-%EC%97%86%EB%8A%94-%EA%B2%B0%EC%A0%9C-%EC%9A%94%EC%B2%AD%EC%9D%84-%EC%96%B4%EB%96%BB%EA%B2%8C-%EB%B3%B4%ED%98%B8%ED%95%A0%EA%B9%8C-47fb51c55155
- 영문: https://medium.com/@bc.0x4861/nu-54v-dk-06-how-do-we-protect-payment-requests-without-pairing-22878f0edd1c

발행 성공 화면과 작업 기록은 저장소 밖 로컬 자료에 보관합니다.

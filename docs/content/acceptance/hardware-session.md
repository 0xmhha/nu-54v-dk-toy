# 실기 시험 절차 (2026-10-03)

보드와 폰이 없는 동안 기기 없이 만들 수 있는 것을 먼저 만들었다. 보안 채널, 폰 링크와 본딩, 버튼 PIN 입력, 결제 모드 메시지, 렌탈 셋업 저장, MCUboot 서명 이미지가 그것이다. 이 문서는 NU-54V-DK 보드, 대여자 폰(P02, Android), 키오스크 태블릿(P04, Android)이 모였을 때 그것들을 어떤 순서로 실기에서 확인하고 무엇을 증거로 남기는지 정한다. 판정 기준은 [DF-20260925-02](../planning/design-freeze-checkpoint-02.md)와 [12주 수용 로그](week12-log.md)를 따르며, 이 문서는 실행 순서만 정한다. 7주차 게이트의 기준과 waiver는 [7주차 게이트 기록](gate-w7.md)에 있다 [N03][N30].

순서는 되돌리기 어려운 단계를 뒤에 두도록 정했다. 개발 빌드로 결제와 폰 링크를 먼저 보고, 렌탈 빌드로 셋업과 반납을 본 뒤, 릴리스 빌드로 서명 부팅을 보고, AP-Protect 잠금은 맨 마지막에 한 번만 한다. AP-Protect를 켜면 디버거로는 erase-all만 할 수 있고, 그러면 기기 키와 셋업이 지워진다.

원본 로그·화면·덤프는 저장소 밖 `evidence/w7/`, `evidence/w12/`에 두고, 문서에는 `0xabcdef…1234`처럼 줄인 값만 쓴다. `evidence/w12/`의 항목은 `w12.py add`로 넣어 sha256을 기록한다([acceptance 도구](../../../products/p10-platform/acceptance/README.md)).

---

## 1. 준비

### 장비와 계정

- NU-54V-DK 보드 1대와 USB 케이블. 콘솔은 VCOM(`/dev/cu.usbmodem*04`, 115200 8N1)이다([bring-up 도구](../../../products/p01-device-firmware/tools/bringup/README.md)).
- Mac 1대. NCS v3.4.1 툴체인, nrfutil, pyOCD, Foundry, Go, pnpm이 있어야 한다.
- 키오스크 태블릿과 대여자 폰. 둘 다 USB 디버깅을 켜고 `adb devices`에 보여야 한다.
- 테스트넷 8283 역할 키는 `~/.nu54/keystores`에 있고 비밀번호는 macOS Keychain에 있다(`opsctl`의 `--keystores`, `keychain:nu54-<role>`).
- 키오스크 가스 잔액이 `kioskMinGasBalance`(13 WKRC)보다 많아야 한다.

### 빌드

```bash
make -C products/p05-operations-backoffice build          # bin/opsctl
O=products/p05-operations-backoffice/bin/opsctl
cd products/p04-merchant-kiosk/android && ./gradlew :app:installDebug && cd -   # 태블릿
cd products/p02-user-app/android && ./gradlew :app:installDebug && cd -         # 폰
```

펌웨어는 단계마다 변형을 바꾸므로 각 절에서 빌드한다. 변형을 바꿀 때는 `--pristine`을 붙이거나 `P01_BUILD_DIR`를 따로 둔다. 키오스크 앱은 결제와 한도 변경을 항상 보안 채널로 열기 때문에, 2026-10-03 이후 main에서 빌드한 펌웨어(PR #42 이후)가 아니면 `NOT_PERMITTED`로 멈춘다. 폰의 결제 모드 버튼은 PR #46 이후 펌웨어와 앱이 있어야 한다.

---

## 2. 개발 빌드: 7주차 게이트

개발 빌드는 셋업이 저장되어 있지 않으면 7주차 고정 셋업(빌드 때 `deployments/8283.json`에서 생성한 운영자, 정산 컨트랙트, chainId)으로 키를 만든다. 이 절의 목표는 결정(2026-10-02)대로 키오스크 Android 앱의 BLE와 화면으로, 사람이 SW1을 눌러 결제 1건을 finalized PaymentSettled까지 끝내는 것이다.

1. 펌웨어를 올리고 부팅 로그를 확인한다.
   ```bash
   cd products/p01-device-firmware
   python3 scripts/fw.py build --pristine && python3 scripts/fw.py flash
   ```
   VCOM에 차례로 `device key created`(또는 `opened`) `(week-7 fixed setup)`, `payment link ready`, `device address 0x…`, `key self-test digest`, `key self-test signature`, `ready: long-press SW4 for payment mode, SW3 for pairing; SW1 approves, SW2 rejects`가 나와야 한다. `key self-test: signature does not recover`가 나오면 여기서 멈춘다.
2. 기기 계정에 입금하고 키오스크를 프로비저닝한다. 주소는 위 로그의 `device address`다.
   ```bash
   $O rental deposit --device <device> --withdraw <withdraw> --amount 10000000
   $O attestation issue --merchant <kiosk> --payout <payout> --name "NU54 Test Cafe" > att.json
   node --experimental-strip-types products/p04-merchant-kiosk/scripts/provision-dev.ts --attestation att.json
   ```
   `wrote provision.json into com.nu54kiosk`가 나오고, 앱을 다시 열면 파일이 사라져야 한다(키는 Android Keystore로 감싸 저장).
3. TimeAnchor는 결제할 때 앱이 Mac의 개발용 anchor 서버에서 새로 받는다(2026-10-05). anchor를 미리 넣고 90초 안에 결제해야 하던 제약이 없어진다([7주차 게이트 기록](gate-w7.md)).
   ```bash
   node --experimental-strip-types products/p04-merchant-kiosk/scripts/anchor-server.ts &   # adb reverse도 설정한다
   node --experimental-strip-types products/p04-merchant-kiosk/scripts/provision-dev.ts --attestation att.json --anchor-url http://127.0.0.1:8095/anchor
   ```
4. SW4를 1초 넘게 눌러 결제 모드를 켜고(`payment mode for 120 s`, 표시등이 2초마다 짧게 깜빡임), 태블릿에서 금액을 넣는다. 표시등이 빠르게 깜빡이고 앱에 10초 카운트다운이 나오면 SW1을 짧게 한 번 누른다. 길게 누르면 `SW1 was held`로 무시된다.
5. 표시등이 느리게 깜빡이다가 2초 켜진 뒤 꺼지고, 태블릿이 approved를, VCOM이 `payment outcome: approved`와 `session over: back to idle`을 보이면 통과다. 결제가 끝나면 결제 모드도 꺼지므로 다음 결제는 SW4부터 다시 한다. tx hash로 증거를 남긴다.
   ```bash
   python3 products/p10-platform/acceptance/w12.py tx W12-03 <txhash>
   ```

**증거:** VCOM 로그, 태블릿 화면, tx hash, 영수증의 PaymentSettled(device, amount, nonce). [7주차 게이트 기록](gate-w7.md)의 기준표를 채우고 판정을 적는다. 보안 채널이 켜진 상태로 통과하므로, waiver 표의 "평문 키오스크 링크" 행은 해당 없음으로 고친다.

---

## 3. 폰 링크와 결제 모드 (W12-01, W12-02)

개발 빌드의 고정 셋업에는 라벨 passkey가 없어 기기가 passkey 0을 쓴다. 그래서 이 절의 라벨은 `passkey=000000`이다. 렌탈 셋업을 마친 기기는 `opsctl rental provision`이 정한 passkey를 쓴다(4절).

1. 보드의 BLE 주소를 확인한다. `tools/bringup`에서 `uv run ble_scan.py`를 실행하거나, 4절의 `opsctl rental provision` 출력에 있는 `bleAddress`를 쓴다.
2. SW3를 1초 넘게 눌러 페어링 모드를 켠다(`pairing mode for 60 s (label passkey)`). 폰 앱에 `nu54://bond?addr=<BLE 주소>&passkey=000000`을 넣고 연결한다. 시스템 창이 뜨면 passkey를 넣는다. VCOM에 `pairing link N complete (bonded)`, `link N security level 4`가 나와야 한다.
3. 페어링 모드가 아닐 때 다른 폰이나 nRF Connect로 페어링을 시도하면 거절되어야 한다(`pairing link N failed`).
4. 폰 앱의 "결제 모드 켜기"를 누른다. VCOM에 `payment mode for 120 s`가 나오고 앱에 "결제 모드: 키오스크가 기기를 찾을 수 있습니다"가 나와야 한다. "결제 모드 끄기"를 누르면 `payment mode ended`가 나와야 한다.
5. 결제 모드를 켠 채 2절처럼 결제한다. 이번에는 폰에 가맹점 이름, 금액, 받는 주소, 주문이 나와야 하고, 결과도 폰에 나와야 한다. 결제 세션이 열려 있는 동안 결제 모드를 다시 눌러도 결제가 끊기지 않아야 한다(보안 채널을 우회하는 다른 링크 메시지, 세션 벡터 SV-32).

**증거:** W12-01은 양쪽 로그(폰 본딩, 키오스크 보안 채널 세션), W12-02는 폰 화면 캡처와 서명 필드 덤프다. 폰 화면의 값이 영수증 PaymentSettled의 값과 같은지 대조한다.

---

## 4. 렌탈 빌드: 셋업, PIN, 한도 변경, 반납 (W12-08, NONCE_REPLAYED)

렌탈 빌드는 셋업이 없으면 `UNPROVISIONED`로 부팅해 운영자 셋업 세션을 기다린다. 셋업 기록, PIN HMAC, PIN 실패 횟수는 PSA ITS에 저장된다.

1. 렌탈 빌드를 올린다. VCOM에 `unprovisioned: waiting for a setup session`이 나와야 한다.
   ```bash
   python3 scripts/fw.py build --pristine --rental && python3 scripts/fw.py flash
   ```
2. SW3를 길게 눌러 페어링 모드를 켜고(`(Just Works)`), Mac에서 셋업을 시작한다.
   ```bash
   $O rental provision --withdraw <withdraw> --amount 10000000
   ```
   `renter: confirm the operator values with the device button`이 나오면 SW1을 누른다. `renter: set the PIN with the device buttons`가 나오면 VCOM의 `enter the PIN`을 확인하고 4자리를 넣는다. SW1을 누른 횟수가 숫자이고, SW3로 자리를 확정하며, SW2는 처음부터 다시 입력한다. 확정한 자리만큼 LED가 켜지고, 시작부터 45초 안에 끝내야 한다. 출력의 `label`(QR 내용)과 `bleAddress`를 적어 둔다.
3. 재부팅해도 `rental setup loaded`가 나와야 한다. 새 라벨로 폰을 다시 본딩한다(3절 2단계, 이번에는 Passkey Entry).
4. 한도 변경을 한다. 폰에 한도가 나오면 PIN을 넣고 SW1으로 승인한다. 같은 서명을 다시 내서 `NONCE_REPLAYED`가 나오는지 확인한다(거절 시연 3.1).
5. PIN 입력을 일부러 45초 넘게 미뤄 `PIN entry timed out`과 TIMEOUT 거절을 확인한다. 이어서 틀린 PIN을 5번 넣어 `PIN_LOCKED`를 확인한다. 재부팅 뒤 `rental setup loaded (PIN locked)`가 나와야 한다.
6. 반납한다. `return: sending device.reset` 뒤 VCOM에 `wipe: done`이 나오고, 재부팅하면 `unprovisioned`, 폰의 본딩도 사라져야 한다.
   ```bash
   $O rental return --device <device>
   ```
7. 셋업 도중(PIN 입력 중) BLE를 끊으면 아무것도 남지 않아야 한다. 재부팅하면 다시 `unprovisioned`가 나와야 한다.

**증거:** W12-08은 셋업 로그와 코드 리뷰(키 가져오기 경로 없음)다. NONCE_REPLAYED는 재제출 로그, PIN 잠금은 재부팅 전후 로그로 남긴다. PIN 숫자는 어느 로그에도 나오면 안 된다.

---

## 5. 반복 결제와 거절 시연 (W12-04, W12-05, W12-07)

1. 렌탈 셋업을 마친 기기로 결제를 20번 연속 한다. 끝나면 시간을 꺼내 판정한다.
   ```bash
   node --experimental-strip-types products/p04-merchant-kiosk/scripts/pull-timings.ts > runs.csv
   python3 products/p10-platform/acceptance/w12.py timings W12-04 runs.csv
   python3 products/p10-platform/acceptance/w12.py add W12-04 runs.csv
   ```
2. 거절 시연 8건은 [12주 수용 로그](week12-log.md) 3.3의 순서(약 25분)로 한다. MERCHANT_FORGED와 UNSUPPORTED_TYPE은 `opsctl refusal host --kind all`로 보내고, 모든 행의 `pass`가 true여야 한다. 보안 채널을 적용한 뒤이므로 MERCHANT_FORGED는 `session.open`에서도 걸린다(세션 벡터 SV-25). 시연에서는 attestation 단계의 거절을 보여 준다.
3. 키오스크 가스를 `kioskMinGasBalance` 아래로 옮겨 두고 결제를 시도하면 화면에 busy가 나와야 한다(W12-07). 시험 뒤 가스를 되돌린다.

---

## 6. 릴리스 빌드: 서명 부팅과 잠금 (W12-09~W12-12)

릴리스 빌드는 MCUboot와 서명한 앱 두 이미지로 나온다. 앱은 콘솔과 로그를 끄므로 VCOM에는 아무것도 나오지 않는다. 확인은 BLE 동작과 LED로 한다. 서명 키는 오프라인에 둔 정식 키를 쓴다(개발용 키 `~/.nu54-keys/dev-signing.pem`이 아니다).

1. 정식 키로 빌드해 올린다. 키오스크 결제와 폰 확인 화면이 4절과 같이 동작해야 한다. 폰 앱 연결을 끊고 결제하면 `NOT_PERMITTED`로 거절되어야 한다(세션 벡터 SV-30).
   ```bash
   export NU54_SIGNING_KEY=<오프라인 키 경로>
   python3 scripts/fw.py build --pristine --release && python3 scripts/fw.py flash
   ```
2. 거부할 이미지를 만들고 serial recovery로 올린다. SW1을 누른 채 리셋하면 recovery에 들어가고 LED1이 켜진다. 두 이미지 모두 부팅되지 않아야 한다. 마지막에 정식 서명 이미지를 다시 올려 복구되는지 확인한다(W12-11).
   ```bash
   python3 scripts/fw.py reject-images out/reject
   ```
3. W12-12의 명령 목록 추출, 이미지 설정 확인(USB CDC 없음), 버튼 없는 서명 요청의 거절 로그를 남긴다.
4. AP-Protect를 켜기 직전의 같은 이미지로 RRAM 전체를 덤프하고, 기기 키와 PIN 패턴을 검색한다(W12-09). 덤프의 `sha256:`과 이미지 해시를 함께 적는다.
5. 마지막으로 `--approtect` 이미지를 올리고, 디버거 연결이 거부되는지 확인한다(W12-10). 이 단계 뒤에는 erase-all 말고는 보드를 열 수 없다. 보드가 1대이므로 12주차 마지막에 한 번만 한다.
   ```bash
   python3 scripts/fw.py build --pristine --release --approtect && python3 scripts/fw.py flash
   ```

---

## 7. 기록과 판정

각 항목은 증거를 넣은 뒤 결과를 적고, 마지막에 전체를 확인한다.

```bash
python3 products/p10-platform/acceptance/w12.py env --write
python3 products/p10-platform/acceptance/w12.py record W12-01 통과 --note "폰 Passkey Entry 본딩, 키오스크 보안 채널"
python3 products/p10-platform/acceptance/w12.py check
```

`check`가 `ok: evidence matches and every row has a result`를 내야 한다. 판정이 실패인 항목은 원인과 재시험 계획을 [12주 수용 로그](week12-log.md)의 비고에 적는다.

이 절차서에서 아직 확인하지 않은 가정은 두 가지다. 하나는 tinygo bluetooth로 만든 opsctl이 macOS에서 실제 보드와 셋업 세션을 끝까지 연다는 것이다. 다른 하나는 Android 폰이 보드의 Passkey Entry 요청에 시스템 passkey 창을 띄운다는 것이다. 둘 다 이 절차의 첫 실행에서 처음 확인된다. 실패하면 그 단계에서 멈추고 로그를 남긴다.

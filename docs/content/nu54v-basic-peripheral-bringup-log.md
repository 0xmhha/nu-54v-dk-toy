# NU-54V-DK 기본 페리페럴 bring-up 기록지

이 문서는 Medium 글에 넣을 실기기 결과를 수집하기 위한 실행 기록지다. 추정값을 완료 결과로 쓰지 않는다. 실물 보드와 제조사 자료로 확인한 값만 기록한다.

## 0. 상태 요약 (2026-09-29)

4주차 증거 게이트용으로 작성했다. 원본 기록은 study 저장소의 bring-up 기록(`projects/nu-54v-dk/04-bringup-log.md`, 커밋 `5c5de23e`, `sha256:7c214605841f7feae2efc6b0b18dcac2ab5d4ea7d3bdafd877b3a20d7292fa6f`)이고, 이 문서는 그 결과를 게이트 기준(BR-01~BR-08)에 맞춰 옮긴 것이다. 2026-09-29에 이 저장소에서 새로 한 시험은 BR-06뿐이다. 원본 로그와 사진은 git에서 제외한 `evidence/w4/`에 두고 여기에는 체크섬만 적는다.

| 판정 | 항목 |
|---|---|
| 충족 | BR-01(전원 인가 쪽은 대체 증거), BR-02, BR-03, BR-04, BR-05, BR-06(HCI 해제 원인 코드는 미확인), BR-07, BR-08, BR-09 |

## 1. 하드웨어 식별

| 항목 | 확인값 | 증거 |
|---|---|---|
| 제품명 | NU-54V-DK, USB 제품 문자열 `NU54DK_v2_Pre-release` | USB 제품 문자열, 디버거 볼륨 이름 `NU54V2PRE` |
| 보드 리비전 | 회로도 Variant NU-54DK-C, 디버거 표기 v2 Pre-release | 제조사 회로도 `NU54_DK_2026-07-13…_SCH.pdf`, USB 문자열 |
| MCU/SoC 표기 | nRF54L15 (FICR PART `0x00054b15`), RAM 256 KB, RRAM 1524 KB | 디버거 `DETAILS.TXT`, pyOCD FICR 읽기 |
| 내장 디버거 | DAPLink 기반 CMSIS-DAP, USB VID/PID `0x0D28/0x0204`, 디버거 칩 nRF52840(HIC ID `6e052840`) | `DETAILS.TXT`, `pyocd list` |
| USB 포트와 케이블 | USB 하나로 디버거, 저장장치 1개, 시리얼 포트 2개. 콘솔은 이름이 `04`로 끝나는 포트, 115200 8N1 | `/dev/cu.usbmodem*` 목록 |
| 기본 LED | LED1 P2.09, LED2 P1.10, LED3 P2.07(SWO와 공유), LED4 P1.14. High에서 켜짐 | 보드 패키지 devicetree, 제조사 보드 문서 |
| 기본 버튼 | SW1 P1.13, SW2 P1.09, SW3 P1.08, SW4 P0.04. 내부 풀업, 누르면 Low | 보드 패키지 devicetree, 제조사 보드 문서 |
| 디버그 잠금 | 풀려 있음 | pyOCD 경고 "NRF54L15 is not in a secure state" |
| 처음 펌웨어 백업 | RRAM 전체 1,560,576바이트, `sha256:cbd6664d3e847a473916446a3c2eaa3004c4a23bdd5039c5be1743c1db445707`, 저장소 밖 보관 | study 기록 4절 |

## 2. 개발 환경

| 항목 | 실제 값 |
|---|---|
| 운영체제 | macOS 26.6.2 (Apple Silicon) |
| SDK/NCS | nRF Connect SDK v3.4.1 (v3.4.0도 설치됨) |
| Zephyr | 4.4.2 (`v4.4.2-33fa6a7aac6a`) |
| 툴체인 | Zephyr SDK 1.0.1 (arm-zephyr-eabi-gcc 14.3.0), west 1.5.0, nRF Util 8.2.1 |
| 보드 타깃 | `nu54v_dk/nrf54l15/cpuapp` (제조사 보드 패키지, 이 저장소에 vendored) |
| 디버그/플래시 도구 | pyOCD 0.42.0 (NCS 툴체인 포함), 타깃 `nrf54l` |
| 직렬/RTT 로그 도구 | 저장소의 bring-up 도구 `serial_cli.py`(VCOM, CR 줄끝), BLE는 bleak(CoreBluetooth)와 Android nRF Connect |

> 보드 타깃은 외형이나 비슷한 개발보드를 보고 추정하지 않는다. NU-54V-DK의 공식 예제 또는 실제 board definition으로 확인한다.

## 3. 빌드·플래시 시도

| 시도 | 날짜 | 예제/커밋 | 설정 | 결과 | 로그·증거 |
|---|---|---|---|---|---|
| 1 | 2026-09-25 | Zephyr `hello_world` | NCS v3.4.0, Nordic DK 타깃 `nrf54l15dk/nrf54l15/cpuapp` | 성공. UART 핀이 같아 로그만 확인, 주변장치 시험에는 쓰지 않음 | study 기록 5절 |
| 2 | 2026-09-26 | 제조사 `led` 예제 | NCS v3.4.1, `nu54v_dk/nrf54l15/cpuapp`, `BOARD_ROOT` 지정 | 성공. FLASH 43,308 B, RAM 7,616 B | study 기록 6절 |
| 3 | 2026-09-26 | 제조사 `button`, `ble_nus` 예제 | 같은 설정 | 성공. `button` FLASH 67 KB·RAM 14 KB, `ble_nus` FLASH 234 KB·RAM 53 KB | study 기록 7·8절 |
| 4 | 2026-09-28 | 제품 펌웨어 골격(LED·버튼 제어) | `make fw`, `make flash` | 성공. FLASH 38,468 B, RAM 7,456 B | 저장소 main `db32039` |

## 4. 기본 시험

| ID | 시험 | 완료 조건 | 결과 | 증거 |
|---|---|---|---|---|
| BR-01 | 부팅 로그 | 전원 인가와 리셋 뒤 동일한 버전·부팅 로그 확인 | 충족(전원 인가 쪽은 대체 증거, 2026-09-29 인정). 리셋 10회 모두 같은 배너(`Booting nRF Connect SDK v3.4.1`, 펌웨어 이름, 보드 타깃). 전원 재연결 뒤에는 리셋 원인 `RESET_BIT_POWER`와 같은 펌웨어 정보(이름 `NU54-DK-BLE-NUS`, 버전 `V260920R1`, 보드 타깃)를 확인했다(2026-09-29 재시험: 포트가 다시 잡힌 뒤 0.03 s 만에 열었을 때 uptime 약 6 s). 디버거의 시리얼 포트가 다시 잡히기 전에 배너가 지나가 배너 문자열 자체는 받지 못함. 전원 투입 리셋 원인과 리셋 때와 같은 펌웨어 이름·버전을 전원 인가 쪽 증거로 인정했다 | study 기록 9절, `evidence/w4/br01-power-on.log`, `sha256:0b02e6f63a73998b4dd83be446e76f191e2f6a463692a7e6a5e0201e23078560` |
| BR-02 | 기본 LED | 보드 정의의 LED를 주기적으로 켜고 끔 | 충족. 제조사 `led` 예제와 제품 펌웨어에서 LED1~4 동작을 확인. 제품 펌웨어는 LED 출력 레지스터로도 확인하고, SW1~SW4를 눌러 LED 네 개가 모두 켜진 사진을 남김(2026-09-29) | study 기록 6절, `evidence/w4/led.jpeg`, `sha256:2a250c0507182b7f5df0b9ec24a9d98f79c210586605fcacf9806f3dfa90000c` |
| BR-03 | 기본 버튼 | 눌림·해제 또는 눌림 이벤트가 로그에 한 번씩 기록됨 | 충족. SW1~SW4 눌림·뗌을 한 번씩 기록, 클릭·길게 누름 판정 확인. 제품 펌웨어에서 손으로 누른 시험과 디버거 흉내 시험 모두 확인 | study 기록 7절 |
| BR-04 | 타이머 | 정한 주기의 이벤트가 허용 오차 안에서 반복됨 | 충족. 120,510 ms 동안 기기 uptime +120,519 ms(+9 ms, 측정 방법의 직렬 지연 수준). GRTC 1 MHz | study 기록 9절 |
| BR-05 | BLE 광고 | Galaxy S25 Ultra에서 이름과 식별값을 검색함 | 충족. S25 Ultra의 nRF Connect 앱과 맥에서 이름 `NU54V-DK`와 NUS UUID로 검색(휴대폰 블루투스 설정 화면에는 BLE 전용 기기가 보이지 않음) | study 기록 8절 |
| BR-06 | BLE 연결 | 연결·해제 이벤트와 원인을 양쪽에서 확인함 | 충족(원인 코드 제외). central이 끊은 경우와 기기가 `ble disconnect`로 끊은 경우 모두, host의 disconnect 콜백과 기기의 `connected : False`·광고 재개를 확인. HCI 해제 원인 코드는 기기 CLI와 macOS CoreBluetooth 모두 보여 주지 않아 확인하지 못함 | `evidence/w4/br06-disconnect.log`, `sha256:92e0eb41dd54d9e703862d5968c4e98152f5d9e3f3af5e5ebf8332be22d0ceb9` (2026-09-29) |
| BR-07 | GATT 쓰기 | 휴대폰의 작은 요청을 기기가 수신하고 로그로 확인함 | 충족. S25 Ultra와 맥에서 RX 쓰기를 기기 rx 카운터와 에코로 확인 | study 기록 8절 |
| BR-08 | GATT 알림 | 기기의 작은 상태 값을 휴대폰이 수신함 | 충족. S25 Ultra에서 프롬프트 알림 수신, 맥에서 명령 결과 251바이트(알림 2개) 수신 | study 기록 8절 |
| BR-09 | 반복 재부팅 | 전원 재연결/리셋을 반복해 같은 동작을 재현함 | 충족. 리셋 10/10 같은 배너, 전원 재연결 1회 뒤 같은 이름·타깃·광고 상태 | study 기록 9절 |

## 5. 실패와 해결 기록

| 날짜 | 증상 | 재현 절차 | 확인한 원인 | 변경 내용 | 재시험 결과 |
|---|---|---|---|---|---|
| 2026-09-25 | `west flash --runner pyocd` 실패 | Nordic DK 타깃으로 빌드 후 flash | DK 보드 정의에 pyOCD 러너가 없음 | `pyocd flash -t nrf54l`로 직접 굽기 | 성공 |
| 2026-09-26 | 보드 패키지를 앱 CMakeLists에 적어도 적용 안 됨 | sysbuild로 빌드 | sysbuild가 앱 CMakeLists보다 먼저 보드를 찾음 | 빌드 명령에 `-DBOARD_ROOT` 전달 | 성공 |
| 2026-09-26 | S25에서 기기가 안 보임 | 휴대폰 블루투스 설정 화면에서 검색 | 설정 화면은 페어링용 기기만 보여 줌 | nRF Connect 앱으로 검색 | 성공 |
| 2026-09-26 | 휴대폰에서 CLI 명령이 실행되지 않음 | nRF Connect TEXT 입력으로 `ble info` 전송 | CLI는 CR(0x0D)을 받아야 실행 | 바이트 배열로 `0D` 포함 전송 | 성공(맥에서 확인) |
| 2026-09-28 | 제품 펌웨어에서 모든 누름이 길게 누름으로 판정 | 디버거를 붙인 채 버튼 흉내 | 디버거 세션이 붙어 있는 동안 커널 타이머가 멈춘 것으로 보임(원인 미확인) | 뗌을 50 ms 폴링으로 확인, 흉내 도구는 핀을 바꿀 때만 디버거 연결 | 성공 |

## 6. 자원 사용량

| 빌드 | Flash | RAM | 부팅 시간 | 비고 |
|---|---:|---:|---:|---|
| 제조사 `led` 예제 | 43,308 B | 7,616 B | 미측정 | NCS v3.4.1 |
| 제조사 `ble_nus` 예제 | 234 KB | 53 KB | 미측정 | BLE와 CLI 포함 |
| 제품 펌웨어 골격 | 38,468 B | 7,456 B | 미측정 | LED·버튼 제어 |

## 7. Medium용 사진과 캡처

- [ ] 보드 앞면 전체와 모델명·리비전이 보이는 사진
- [ ] USB와 디버거 연결 상태가 보이는 사진
- [x] LED가 켜진 상태의 사진 또는 짧은 영상 캡처 (`evidence/w4/led.jpeg`)
- [ ] 빌드·플래시 성공 화면
- [ ] 버튼 이벤트 로그
- [ ] Galaxy S25 Ultra의 BLE 광고 검색 화면
- [ ] BLE 연결과 GATT 쓰기·알림 결과
- [ ] 화면과 로그에 주소, 키, 토큰, 계정 등 민감정보가 없는지 확인

## 8. 게시용 결과 요약

- 사용한 환경: `[작성]`
- 한 번에 성공한 항목: `[작성]`
- 가장 오래 막힌 항목: `[작성]`
- 원인과 해결: `[작성]`
- 반복 검증 결과: `[작성]`
- 다음에 연결할 페리페럴: `[작성]`
- 현재 한계: `[작성]`

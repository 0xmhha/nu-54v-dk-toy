# S25 Ultra 외부 거리 측정 모듈 — 시험 구성

작성: 2026-09-17. 제품 요구사항은 [플랫폼 범위 문서](payment-platform-scope-v2.md)를 따른다. 이 문서는 첫 시험의 부품·연결·검증 기준을 구체화한 작업안이다. 부품 구매, 펌웨어 빌드·플래시, 앱 설치, 실제 거리 시험은 아직 수행하지 않았다.

## 1. 첫 시험 구성

```text
NU-54V-DK 지갑 역할                         S25 Ultra
  CS reflector                              Android 앱
       ⇅ Channel Sounding                       ⇅ USB host
Nordic nRF54L15 DK ── UART → 온보드 J-Link → USB VCOM
  CS initiator + 거리 계산
```

첫 기준 구성은 **NU-54V-DK 1대 + Nordic nRF54L15 DK 1대**로 제안한다. 지갑 역할은 NU 보드에 유지하고 Nordic DK는 측정 모듈의 개발용 기준 장비로 사용한다. Nordic DK는 최종 휴대용 모듈의 크기·전력·가격을 대표하지 않는다.

사용자는 제시한 두 보드를 모두 보유한다고 답했다. 따라서 NU-54V-DK와 Nordic nRF54L15 DK를 사용하는 기준안을 유지하고, 기기를 이용한 검증은 사용자의 지시에 따라 실제 개발 단계에서 진행한다. 하드웨어 검증 결과를 기다리느라 요구사항·소프트웨어 설계를 멈추지 않는다.

이 구성은 폰의 Bluetooth 하드웨어를 교체하지 않는다. 정밀 거리 측정은 두 보드가 수행하고 폰은 USB로 결과를 받는다. S25 자체의 Channel Sounding 지원이 없어도 검증할 수 있는 구조다. USB로 폰에 연결된 모듈과 지갑의 거리이며, 폰과의 물리적 고정 관계가 확인되기 전까지 ‘폰과 지갑의 거리’라고 단정하지 않는다.

## 2. 최소 부품 목록

| 부품 | 기준 수량 | 용도 / 선정 조건 |
|---|---:|---|
| NU-54V-DK | 1 | 실제 제품 지갑 역할. 보드 리비전·제조사 보드 패키지 확인 |
| Nordic nRF54L15 DK | 1 | 공식 예제 기반 initiator와 UART/USB 기준 경로. 보유 NU 2대 구성이 가능하면 대체 검토 |
| 데이터 가능한 USB 케이블 | 2 | 각 보드의 실제 커넥터와 맞는 케이블. 충전 전용 제외 |
| S25 USB-C 호스트 연결용 케이블/어댑터 | 1 | 보드 연결 케이블과 중복 구매하지 않도록 커넥터 확인 |
| NU 보드용 독립 USB 전원 | 1 | PC 또는 기존 USB 전원 활용. 초기 RF 시험에서는 배터리 불필요 |
| 줄자·비금속 고정대 | 각 1 | 안테나 사이 기준 거리와 방향을 반복 재현 |
| 전원 공급 USB 허브 | 조건부 | 폰 직결 전력이 부족하거나 충전과 데이터가 동시에 필요할 때만 검토 |
| USB-UART 어댑터·점퍼선 | 조건부 | NU 2대 구성 또는 USB VCOM 우회. 실제 I/O 전압에 맞는 TTL 어댑터 |

현재 가격·판매처·재고는 비교하지 않았고 구매하지 않았다. 마이크·화면·배터리·클립 케이스는 거리 시험 통과 후 제품 통합 단계에서 추가한다. 일반 Bluetooth 6.0 USB 동글은 이 목록의 측정 모듈을 자동으로 대체하지 않는다.

## 3. 확인된 연결 경로와 미확인 부분

### Nordic 기준 모듈

Nordic 문서는 nRF54L15 DK의 온보드 J-Link와 UART 가상 시리얼 경로를 설명한다. Android 앱은 복합 USB 장치 중 UART에 해당하는 인터페이스/포트를 선택해야 하며, Nordic VCOM의 DTR 조건도 확인한다. PC에서 콘솔이 보였다는 사실만으로 S25 연결 성공을 판단하지 않는다.

Android USB Host API와 CDC/ACM을 다루는 `usb-serial-for-android`를 초기 수신 경로의 후보로 둔다. 해당 라이브러리 문서는 CDC/ACM 인터페이스 탐지와 DTR 제어를 제공하지만, 실제 보드의 descriptor·포트 번호·권한·연결 해제 동작을 실기에서 확인해야 한다. 앱 설치·의존성 도입은 아직 하지 않았다.

USB 브리지는 거리 값을 운반하는 경로다. USB 연결만으로 측정 모듈의 신뢰성이나 근접 로그인 보안성이 보장되는 것은 아니다.

### NU 보드의 USB 경로

Zephyr 지원 PR #117771은 온보드 DAPLink CDC 인터페이스를 설명하지만, PR 작성자가 시험한 사전 배포 펌웨어에서는 UART 콘솔 전달이 동작하지 않았다고 기록한다. 이를 모든 NU 보드의 결함으로 일반화하지 않는다. 보유 보드의 펌웨어·리비전에서 확인할 항목이다.

NU 제조사 기본 핀 정의에는 UART20 TX=P1.4, RX=P1.5가 기재되어 있다. 외부 USB-UART를 쓸 경우 실제 회로와 UART 스위치/솔더브리지 경로를 확인한 뒤 TX→어댑터 RX, RX←어댑터 TX, GND 공통으로 연결한다. I/O 전압은 VDD_MOD 실측에 맞춘다. 5V 신호·RS-232를 직접 연결하지 않고, USB로 전원을 공급하는 보드에 어댑터 전원핀을 중복 연결하지 않는 구성을 우선한다.

초기에는 측정 보드와 폰을 짧은 케이블로 연결해 책상에서 시험한다. 휴대용 USB-C 직결 모듈로 소형화하는 작업은 별도다.

## 4. SDK와 펌웨어 기준

확인한 릴리스 태그 **nRF Connect SDK v3.3.4**를 거리 측정 시험의 기준 후보로 둔다. 해당 태그에 다음 샘플과 Nordic DK target이 존재한다. 이는 NU 보드의 보호 서명·BLE·음성 전체 제품용 SDK를 최종 선택한 결정은 아니다.

| 역할 | 기준 샘플 |
|---|---|
| Initiator / 거리 계산 | `samples/bluetooth/channel_sounding/ras_initiator` |
| Reflector / 응답 | `samples/bluetooth/channel_sounding/ras_reflector` |

Nordic target은 `nrf54l15dk/nrf54l15/cpuapp`이다. 제조사 NU 외부 보드 정의와 미병합 Zephyr PR의 target 이름은 다를 수 있으므로 혼용하지 않는다. NCS가 고정하는 Zephyr 버전과 NU 보드 정의의 호환성을 확인하고 사용 commit을 기록해야 한다.

설정된 NCS v3.3.4 workspace의 `nrf` 디렉터리에서 수행할 **Nordic DK용 빌드 예시**:

```sh
west build --sysbuild -b nrf54l15dk/nrf54l15/cpuapp samples/bluetooth/channel_sounding/ras_initiator -d build/ranging-initiator
```

이 명령은 아직 실행하지 않았다. NU 보드에 Nordic target 펌웨어를 그대로 플래시하지 않는다. NU의 clock·RF·GPIO·board overlay·SoftDevice Controller 호환성을 검증한 뒤 reflector를 포팅한다. 재현성을 위해 SDK tag, manifest revision, toolchain, 보드 정의 commit, 보드/펌웨어 리비전을 모두 기록한다.

## 5. 단계별 시험

### A. 각 보드와 USB 경로

- NU 보드: 제조사 기본 예제로 부팅·광고·버튼 및 디버그 경로 확인. 기존 펌웨어·키 보유 여부를 확인하기 전 erase-all 하지 않는다.
- 기준 모듈: PC에서 UART 출력 수신, USB descriptor와 포트·DTR 조건 기록.
- S25: 같은 UART 출력을 USB 권한 승인 후 수신. 케이블 분리, 앱 재시작, 화면 회전·잠금 시 상태 기록.
- 통과 조건: 지원 포트를 명시적으로 선택하고 연결/수신/해제를 구분할 것. 미수신 값을 0m로 표시하지 않을 것.

### B. 보드 간 거리 측정

- 먼저 한 쌍의 initiator/reflector를 연결하여 capability 교환과 샘플 출력 확인.
- 제안 측정 거리: 0.25m, 0.5m, 1m, 2m, 3m. 기준은 안테나 사이로 기록한다.
- 제안 조건: 정면·90도 회전·인체 가림·탁상 반사 환경. 초기 기준은 비금속 고정대로 고정한다.
- 조건별 100회 측정 시도를 제안한다. 유효값만 골라 성공률을 부풀리지 않고 무효·미수신도 기록한다.
- 산출물: 오차 중앙값/P95, 유효 측정 비율, 갱신 간격, 연결 복구 시간. RTT/IFFT/phase-slope 등 SDK 출력값은 서로 구분한다.
- 정확도와 로그인 거리 임계값은 결과를 보고 정한다. 사전에 ‘10cm 정확도’, ‘1m면 안전’으로 제품 성능을 확정하지 않는다.

### C. 폰 표시

폰 시험 화면은 연결 상태, 측정 거리, 측정 방식, 마지막 수신 시각/경과 시간, 유효/무효/연결 끊김 상태, 기록 내보내기를 표시한다. 폰에 거리 숫자가 보이는 단계는 측정 시연이며 인증 성공이 아니다.

기본 샘플 로그가 여러 줄 또는 일부 조각으로 수신될 수 있으므로 완전한 레코드만 파싱한다. 깨진 데이터·NaN·부정확한 상태를 승인값으로 만들지 않는다. 샘플 갱신 주기는 실제 SDK 설정을 확인하고, 원하는 앱 갱신 주기와 맞지 않으면 수정 후 다시 측정한다.

### D. 근접 승인 시연

자사 시험 서비스에서 등록된 모듈과 지갑, 실제 로그인 클라이언트, 해당 로그인 요청과 측정 결과를 연결한다. 서명·인증·최신성 검증을 하기 전에는 평문 UART의 거리 값을 보안 증명으로 사용하지 않는다.

시험할 실패 조건: 다른 기기 결과, 예전 결과 재사용, 연결 중단, 측정 실패, 승인 대기 중 거리 이탈, 다른 로그인 요청에 결과 재사용, 사용자 거절. 실패 시 승인 대기를 해제하고 재시도를 요구한다. 이미 발급된 세션의 원격 조종이나 세션 탈취는 별도 위협이며 이 시험으로 해결되었다고 표시하지 않는다.

### E. 기존 제품과 통합

음성 전송·지갑 BLE·Channel Sounding을 함께 실행할 때의 유효 거리 비율, 오디오 누락, CPU/RAM, 배터리 소비를 확인한다. 녹음과 결제 승인 입력을 구분한다. 케이스·금속 클립·인체 부착 후 RF와 음질을 재검증한다.

## 6. 측정 기록 파일

[빈 CSV 양식](ranging-measurements-template.csv)을 준비했다. 실제 수치는 아직 없다. 테스트마다 원본 로그와 빌드 정보를 함께 보관한다.

- `run_id`: 시험 묶음. 개인 식별자나 지갑 주소를 쓰지 않는다.
- `ground_truth_m`: 줄자로 측정한 안테나 간 거리.
- `sample_index`: 유효/무효를 포함한 측정 시도 순서.
- `status`: valid, invalid, timeout, disconnected 중 실제 상태.
- `estimated_m`: 선택한 추정 방법의 출력. 실패 시 빈 값이며 0으로 대체하지 않는다.
- `method`: ifft, phase_slope, rtt 등 실제 출력 방법. 같은 시도에 여러 방법을 기록하면 방법별 행으로 나눈다.
- `received_at_utc`: 기록 장치의 수신 시각. 이것만으로 인증용 freshness가 검증되는 것은 아니다.
- `latency_ms`: 실제 측정 시작/끝을 관측할 수 있을 때만 기록. 관측 불가이면 빈 값.
- SDK/보드/폰 버전 및 orientation/condition은 재현 가능한 값으로 기록한다.

측정 성공률 집계 시 sample_index 기준으로 시도를 세고, 여러 method 행을 독립 측정 횟수로 중복 계산하지 않는다. 오차 지표는 유효 표본만 계산하되 전체 시도 수·실패율을 함께 보고한다.

## 7. 현재 환경 확인 결과

2026-09-17 이 Mac에서 읽기 전용으로 확인한 내용:

- Android platform-tools의 ADB가 있다. API 36 SDK platform도 설치되어 있다.
- ADB 목록에는 오프라인 에뮬레이터만 표시되었고 S25 실기기는 확인하지 못했다. USB 디버깅 비활성 또는 연결 방식에 따른 차이를 포함하므로 물리적 부재 자체를 단정하지 않는다.
- `/dev/cu.usb*`에 연결된 USB 시리얼 경로를 찾지 못했다.
- 현재 PATH에서 `west`, `nrfutil`, `ninja`는 발견하지 못했다. 다른 위치에 설치되어 있지 않다는 뜻은 아니다.
- SDK 설치, 소스 checkout, 펌웨어 빌드, 보드 플래시, Android 앱 설치는 수행하지 않았다.

후속 사용자 답변: 두 보드는 보유하고 있으며 기기를 이용한 검증은 실제 개발 단계에서 진행한다. 현재는 소프트웨어 설계로 이동한다. 보드 리비전·폰 OS·실제 연결·빌드 호환성은 개발 시 확인할 검증 항목으로 유지한다.

## 근거

- [Nordic Channel Sounding와 두 DK 시험 경로](https://www.nordicsemi.com/Products/Wireless/Bluetooth-Low-Energy/Channel-Sounding)
- [nRF54L15 DK hardware](https://docs.nordicsemi.com/r/bundle/ug_nrf54l15_dk/page/ug/nrf54l15_dk/intro/intro.html)
- [Nordic 가상 시리얼 포트](https://docs.nordicsemi.com/r/bundle/ug_nrf54l15_dk/page/ug/nrf54l15_dk/program_debug/virtual_serial_ports.html)
- [NCS v3.3.4 릴리스](https://github.com/nrfconnect/sdk-nrf/releases/tag/v3.3.4)
- [고정 태그 initiator README](https://github.com/nrfconnect/sdk-nrf/blob/v3.3.4/samples/bluetooth/channel_sounding/ras_initiator/README.rst)
- [고정 태그 initiator targets](https://github.com/nrfconnect/sdk-nrf/blob/v3.3.4/samples/bluetooth/channel_sounding/ras_initiator/sample.yaml)
- [고정 태그 reflector README](https://github.com/nrfconnect/sdk-nrf/blob/v3.3.4/samples/bluetooth/channel_sounding/ras_reflector/README.rst)
- [NU 제조사 보드 정의](https://github.com/Nucode01/NU54DK_Zephyr_DTS)
- [NU 하드웨어 노트](https://github.com/Nucode01/NU54DK_Zephyr_DTS/blob/main/00_Docs/04_HARDWARE_NOTES.md)
- [NU Zephyr 지원 PR 및 CDC 시험 기록](https://github.com/zephyrproject-rtos/zephyr/pull/117771)
- [Android USB host](https://developer.android.com/develop/connectivity/usb/host)
- [usb-serial-for-android 원본 문서](https://github.com/mik3y/usb-serial-for-android)

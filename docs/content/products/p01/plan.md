# P01 기획 — NU-54V-DK 결제 서명 펌웨어

[DF-20260925-02](../../planning/design-freeze-checkpoint-02.md)에서 P01이 이번 12주 사이클에 무엇을 만들고 무엇을 만들지 않는지 정한다. 요구는 [srs.md](srs.md), 사용 흐름은 [use-cases.md](use-cases.md), 구조는 [design.md](design.md)에 있다. 담당은 role A이고, 보드 bring-up(WBS2-P01-00)만 user가 맡는다 [N15].

## 1. 목표와 범위

P01은 이번 사이클에 실제로 만드는 일곱 제품 가운데 하나다 [N01]. 목표는 하나다. NU-54V-DK(nRF54L15)가 대여 셋업 때 기기 안에서 만든 키로, 운영자 attestation과 온체인 registry가 보증한 가맹점 주문에 대해서만, 주문 내용을 대여자 폰 앱에 보여 준 뒤(기기에는 화면이 없다 [N26]) 물리 버튼을 눌렀을 때 PaymentAuthorization에 서명한다.

이 목표를 위해 P01이 맡는 일은 다음과 같다.

- 대여 셋업: 버튼으로 확인한 운영자 주소·컨트랙트·chainId 기록, TRNG 키 생성, 버튼으로 하는 PIN 설정, 페어링 passkey 기록, TimeAnchor 기록 [N06][N11][N23][N28]
- BLE GATT peripheral: 폰 앱·운영자 도구와 Passkey Entry 본딩, 페어링 없는 키오스크 결제 세션과 보안 채널, 메시지 조각 재조립 [N09][N27]. NFC는 쓰지 않는다 [N29]
- 서명: PaymentAuthorization과 LimitChange 두 EIP-712 타입만, secure 쪽 서명 권한 경계 안에서 [N04][N24]
- 검증: MerchantAttestation, MerchantOrder, TimeAnchor, DeviceReset의 서명과 유효 기간 [N05]
- 확인과 승인: 가맹점 이름, orderId, token, payout, amount를 `confirm.show`로 폰 앱에 보내고 버튼으로 승인, LED로 상태 안내 [N26]
- 반납: 운영자 서명 DeviceReset으로 키와 기록 삭제 [N23]
- 보호: W9부터 외부 secure element, 그 전까지 TF-M waiver [N02][N14]
- 갱신: MCUboot 유선 serial recovery만 [N12]

## 2. 산출물

| 산출물 | 내용 | 완료 증거 |
|---|---|---|
| 펌웨어 이미지 | NCS v3.4.1/Zephyr 4.4.2, 제조사 board package `nu54v_dk/nrf54l15/cpuapp`, USB CDC harness를 뺀 W12 릴리스 빌드 | 재현 빌드 로그와 `sha256:` checksum |
| 결제 GATT 서비스 | [payment-protocol.md](../../specifications/protocol/payment-protocol.md)의 `rx`/`tx`, envelope, 조각 규칙 | 페어링·재조립 로그 |
| 서명 모듈 | EIP-712 hasher, 서명 권한 경계, 거절 코드 | P10 벡터 적합성 로그 |
| 키 수명주기 | 셋업 명령, TRNG 생성, PIN, DeviceReset | 키 수명주기 시험 로그 |
| SE 통합 | TRNG로 만든 키를 외부 SE로 감싼 서명 | W9 게이트 증거 |
| 부트 보호 | MCUboot 서명 이미지, AP-Protect | 서명 안 된 이미지 거부 로그 |

## 3. 일정

작업 행은 [WBS-02](../../planning/product-worklist-and-12week-wbs-02.md)의 CSV와 같다. 동결일(9/25)이 W4 안이므로 모든 작업은 W4부터 시작한다 [N03].

| WBS 행 | 작업 | 주차 | 게이트 |
|---|---|---|---|
| WBS2-P01-00 | 보드 bring-up BR-01..BR-08 (user) | W4 | W4 |
| WBS2-P01-10 | 외부 SE 데이터시트의 secp256k1 확인 | W4 | W4 |
| WBS2-P01-01 | Zephyr 골격, 버튼, LED, 확인 필드 | W4–W5 | W7 |
| WBS2-P01-02 | non-secure PSA 키, 서명 low-s·recovery id, 토큰-digest 바인딩 | W5–W6 | W7 |
| WBS2-P01-03 | BLE 연결 역할, 페어링 모드, Passkey Entry, 재조립 | W6–W7 | W7 |
| WBS2-P01-04 | EIP-712 서명, 검증, TimeAnchor, 개발용 고정 셋업, 거절 코드 | W7 | W7 |
| WBS2-P01-11 | TF-M 키 서비스, `/ns` 보드 변형, secure 버튼, 버튼 PIN | W8 | - |
| WBS2-P01-12 | BLE 셋업 명령, 기기별 passkey 기록 | W8 | - |
| WBS2-P01-05 | 외부 secure element 통합 | W8–W9 | W9 |
| WBS2-P01-14 | 결제 세션 보안 채널 | W9 | - |
| WBS2-P01-13 | 폰 링크 `confirm.show`, 결과 전달 | W10 | - |
| WBS2-P01-07 | MCUboot 서명 이미지, AP-Protect, USB CDC 없는 릴리스 빌드 | W10 | - |
| WBS2-P01-08 | W12 보안 점검과 안정화 (P01-07, P01-04 뒤) | W11–W12 | - |

게이트별로 P01이 내야 하는 증거는 다음과 같다 [N03].

- **W4 증거 게이트(게이트 조건):** BR-01..BR-08 bring-up 결과, 외부 SE 데이터시트의 secp256k1 지원 확인. P01 자체 점검(게이트 조건 아님): 빌드·flash, 버튼 입력.
- **W6 컨트랙트 게이트:** P01 증거는 없다. P06이 소프트웨어 서명으로 finalized PaymentSettled를 낸다.
- **W7 실결제 게이트:** 보드 내장 키로 서명한 결제 1건이 키오스크를 거쳐 finalized PaymentSettled까지 가는 실기 end-to-end. 첫 실결제 게이트이자 컷 트리거다.
- **W9 SE 게이트:** 외부 secure element로 감싼 키로 같은 end-to-end 1건.

## 4. 의존성

| 필요한 것 | 주는 쪽 | 시점 | 없으면 |
|---|---|---|---|
| EIP-712 타입 패키지와 [eip712-vectors.json](../../specifications/protocol/eip712-vectors.json) | P10 (WBS2-P10-01) | W4 | 서명 모듈 시험을 시작할 수 없다 |
| MerchantAttestation 발급, 셋업·TimeAnchor provisioning(`opsctl`) | P05 (WBS2-P05-01, WBS2-P05-02) | W6, W7 | 셋업과 identify 경로를 실기로 돌릴 수 없다 |
| BLE central과 최소 제출 | P04 (WBS2-P04-01, WBS2-P04-02) | W4–W6, W7 | W7 end-to-end를 할 수 없다 |
| 8283 배포 컨트랙트 | P06 (WBS2-P06-02) | W6 | 컨트랙트 층 거절을 확인할 수 없다 |
| 반납 DeviceReset(`opsctl rental return`) | P05 (WBS2-P05-03) | W10 | 반납·재셋업 시험을 할 수 없다 |

## 5. 위험과 컷

- role A는 W4–W7 동안 가용 일수를 모두 쓴다. 서명·검증·셋업 명령(WBS2-P01-04)을 W7 한 주 2.5일에 끝내야 해서 하루만 밀려도 W7 게이트가 위험하다.
- W7 게이트는 non-secure PSA 키, 개발 빌드 전용 고정 셋업, 평문 키오스크 링크로 통과하고 이 사실을 waiver로 적는다. TF-M 키 서비스와 셋업 명령은 W8, 보안 채널은 W9에 넣는다 [N30].
- role A는 P02 폰 앱도 맡아 W8–W12 부하가 가용 일수를 넘는다(W8 7일). 담당자가 감수하기로 했다 [N30].
- W7 게이트가 실패하면 컷 순서는 외부 SE 통합(WBS2-P01-05), P07이다 [N03]. NFC handover는 이미 뺐다 [N29]. SE가 컷되면 키는 W12까지 TF-M secure partition 봉인으로 남고, 수용 로그에 waiver로 적는다 [N14]. WBS2-P01-08은 SE 없이도 P01-07과 P01-04 뒤에 진행한다.
- 보드는 NFC 핀을 I2C로 쓰고 안테나가 없다. BLE scan만 쓴다 [N29].
- 가맹점 정보 확인 화면(폰 앱)은 어떤 컷에서도 빼지 않는다 [N26].
- 전원 손실, watchdog, System OFF 깨어남, serial recovery 뒤 재부팅은 TimeAnchor를 무효로 만들어 결제가 막힌다. 재-anchor는 셋업 세션으로 하므로 시연 전 배터리와 재-anchor 절차를 점검 목록에 넣는다 [N06].

## 6. 범위 밖

다음은 이전 설계에서 없앴거나 이번 사이클에서 만들지 않는다.

- 패스키(CTAP), 녹음, 찾기 [D06][D07]
- BIP-39 니모닉 생성과 import, 키 백업 [D03][RR-DEC-01]
- BLE 전송 MCUmgr SMP와 BLE DFU [D05]. MCUboot serial recovery는 UART 위에서 SMP를 쓰며 이것만 남긴다.
- 원시 트랜잭션, approve, Permit 서명 [N04]
- trusted display(표시·해시를 secure partition에서 만드는 구조) [N24]

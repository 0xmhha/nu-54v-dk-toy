# P01 기획 — NU-54V-DK 결제 서명 펌웨어

[DF-20260925-02](../../planning/design-freeze-checkpoint-02.md)에서 P01이 이번 12주 사이클에 무엇을 만들고 무엇을 만들지 않는지 정한다. 요구는 [srs.md](srs.md), 사용 흐름은 [use-cases.md](use-cases.md), 구조는 [design.md](design.md)에 있다. 담당은 role A이고, 보드 bring-up(WBS2-P01-00)만 user가 맡는다 [N15].

## 1. 목표와 범위

P01은 이번 사이클에 실제로 만드는 여섯 제품 가운데 하나다 [N01]. 목표는 하나다. NU-54V-DK(nRF54L15)가 대여 때 기기 안에서 만든 키로, 운영자 attestation과 온체인 registry가 보증한 가맹점 주문에 대해서만, 주문 내용을 화면에 보여 준 뒤 물리 버튼을 눌렀을 때 PaymentAuthorization에 서명한다.

이 목표를 위해 P01이 맡는 일은 다음과 같다.

- 대여 셋업: TRNG 키 생성, PIN 설정, 운영자 주소와 TimeAnchor 기록 [N06][N11]
- BLE GATT peripheral: LE Secure Connections 페어링, NFC handover, 메시지 조각 재조립 [N09]
- 서명: PaymentAuthorization과 LimitChange 두 EIP-712 타입만 [N04]
- 검증: MerchantAttestation, MerchantOrder, TimeAnchor의 서명과 유효 기간 [N05]
- 표시와 승인: 가맹점 이름, orderId, token, payout, amount 표시와 버튼 승인
- 반납: device.reset으로 키와 기록 삭제
- 보호: W8부터 외부 secure element, 그 전까지 TF-M waiver [N02][N14]
- 갱신: MCUboot 유선 serial recovery만 [N12]

## 2. 산출물

| 산출물 | 내용 | 완료 증거 |
|---|---|---|
| 펌웨어 이미지 | NCS v3.4.0/Zephyr 4.4, project-owned board definition | 재현 빌드 로그와 `sha256:` checksum |
| 결제 GATT 서비스 | [payment-protocol.md](../../specifications/protocol/payment-protocol.md)의 `rx`/`tx`, envelope, 조각 규칙 | 페어링·재조립 로그 |
| 서명 모듈 | EIP-712 hasher와 서명, 거절 코드 | P10 벡터 적합성 로그 |
| 키 수명주기 | TRNG 생성, PIN, device.reset | 키 수명주기 시험 로그 |
| SE 통합 | 외부 SE로 감싼 키 서명 | W8 게이트 증거 |
| 부트 보호 | MCUboot 서명 이미지, AP-Protect | 서명 안 된 이미지 거부 로그 |

## 3. 일정

작업 행은 [WBS-02](../../planning/product-worklist-and-12week-wbs-02.md)의 CSV와 같다.

| WBS 행 | 작업 | 주차 | 게이트 |
|---|---|---|---|
| WBS2-P01-00 | 보드 bring-up BR-01..BR-08 (user) | W1–W4 | W4 |
| WBS2-P01-01 | Zephyr 골격, 버튼, 가맹점 표시 | W1–W2 | W4 |
| WBS2-P01-02 | TRNG 키 생성, TF-M 봉인, PIN | W2–W3 | W4 |
| WBS2-P01-03 | BLE GATT, LESC, 재조립 | W3–W5 | W6 |
| WBS2-P01-04 | EIP-712 서명, attestation·TimeAnchor 검증, 거절 코드 | W5–W6 | W6 |
| WBS2-P01-05 | 외부 secure element 통합 | W7–W9 | W8 |
| WBS2-P01-06 | NFC handover, BLE scan 대체 경로 | W9 | - |
| WBS2-P01-07 | MCUboot 서명 이미지, AP-Protect | W10 | - |
| WBS2-P01-08 | W12 보안 점검과 안정화 | W11–W12 | - |

게이트별로 P01이 내야 하는 증거는 다음과 같다 [N03].

- **W4:** bring-up 결과, 키 생성 로그, 버튼·표시 동작
- **W6:** 보드 내장 키로 서명한 결제 1건이 키오스크를 거쳐 finalized PaymentSettled까지 가는 실기 end-to-end. 첫 실결제 게이트이자 컷 트리거다.
- **W8:** 외부 secure element로 감싼 키로 같은 end-to-end 1건

## 4. 의존성

| 필요한 것 | 주는 쪽 | 시점 | 없으면 |
|---|---|---|---|
| EIP-712 스키마와 [eip712-vectors.json](../../specifications/protocol/eip712-vectors.json) | P10 (WBS2-P10-01) | W2 | 서명 모듈 시험을 시작할 수 없다 |
| MerchantAttestation·TimeAnchor 발급, provisioning 스크립트 | P05 (WBS2-P05-01, WBS2-P05-02) | W3, W5–W7 | 셋업과 identify 경로를 실기로 돌릴 수 없다 |
| BLE central과 제출 | P04 (WBS2-P04-01, WBS2-P04-02) | W5–W6 | W6 end-to-end를 할 수 없다 |
| 8283 배포 컨트랙트 | P06 (WBS2-P06-02) | W4 | 컨트랙트 층 거절을 확인할 수 없다 |

## 5. 위험과 컷

- role A의 부하가 W2와 W5에 가용 일수와 같다. BLE나 키 작업이 하루만 밀려도 W6 게이트가 위험하다.
- W6 게이트가 실패하면 첫 번째 컷이 외부 SE 통합(WBS2-P01-05)이다 [N03]. 이때 키는 W12까지 TF-M secure partition 봉인으로 남고, 수용 로그에 waiver로 적는다 [N14].
- NFC 안테나와 드라이버는 검증되지 않았다. NFC는 게이트 조건이 아니고 BLE scan이 대체 경로다 [N09].
- 가맹점 정보 표시는 어떤 컷에서도 빼지 않는다.
- 전원이 끊기면 TimeAnchor가 무효가 되어 결제가 막힌다. 시연 전 배터리와 셋업 절차를 점검 목록에 넣는다 [N06].

## 6. 범위 밖

다음은 이전 설계에서 없앴거나 이번 사이클에서 만들지 않는다.

- 패스키(CTAP), 녹음, 찾기 [D06][D07]
- BIP-39 니모닉 생성과 import, 키 백업 [D03][RR-DEC-01]
- BLE DFU와 MCUmgr SMP [D05]
- 원시 트랜잭션, approve, Permit 서명 [N04]

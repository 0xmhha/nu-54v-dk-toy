# 12주차 수용 기록지

12주차(11/25) 시연에서 DF-20260925-02의 완료를 판정하는 실행 기록지다 [N18]. 형식은 [bring-up 기록지](../nu54v-basic-peripheral-bringup-log.md)를 따른다. 추정값을 완료 결과로 쓰지 않고, 실물 기기와 StableNet 8283에서 확인한 결과만 적는다.

원본 로그·영상은 git에 올리지 않는 경로에 둔다. 이 문서에는 주소와 tx hash를 줄여(앞 6자리…뒤 4자리) 적고, 원본 파일은 64자리 전체 `sha256:` checksum으로 가리킨다 [N16]. 시험 계정은 시험 전용 EOA만 쓴다.

## 1. 시험 환경

| 항목 | 실제 값 | 증거 |
|---|---|---|
| 기기 펌웨어 | `[버전/커밋]` | `sha256:[이미지 checksum]` |
| 키 보관 | `[SE / TF-M waiver]` | W9 SE 게이트 결과 [N02] |
| 키오스크 빌드 | `[버전/커밋]` | `sha256:[APK checksum]` |
| 정산 컨트랙트 | `[줄인 주소]` | 배포 manifest |
| 체인 | StableNet testnet 8283 | `eth_chainId` 응답 |
| 파라미터 | DF-20260925-02 parameters [N13] | register 커밋 |
| 도구·의존성 버전 | `[NCS, Zephyr, Foundry, RN, 라이브러리 버전]` | 빌드 로그. 의존성 pinning은 waiver라 버전만 기록한다 [N14] |
| 키오스크 시작 가스 잔액 | `[WKRC]` | 잔액 조회. kioskMinGasBalance에 W12 결제 30건분(결제 1건 약 7.14 WKRC)을 더한 약 230 WKRC 이상으로 시작한다 [N10] |

## 2. 기능 항목

| ID | 시험 | 완료 조건 | 결과 | 증거 |
|---|---|---|---|---|
| W12-01 | 연결 | 키오스크가 NFC 또는 BLE scan으로 기기를 찾아 LE Secure Connections로 페어링하고 session.open/session.confirm을 마친다 | 미실행 | 양쪽 로그 `sha256:` |
| W12-02 | 표시 | 기기 화면에 가맹점 이름, orderId, token, payout, amount가 서명 필드와 같게 표시된다 | 미실행 | 화면 사진, 서명 필드 덤프 |
| W12-03 | 정산 | 버튼 승인 뒤 finalized PaymentSettled(merchant, orderId)가 관측되고 키오스크가 approved를 표시한다. 같은 서명을 다시 제출해도 ORDER_ALREADY_PAID 경로로 approved가 유지된다 [N08] | 미실행 | 줄인 tx hash, 이벤트 로그, 재제출 로그 |
| W12-04 | 반복 성공 | 연속 20회 결제가 모두 요청 전달 완료부터 10 s 안에 approved로 끝난다 | 미실행 | 20회 시간 기록표 |
| W12-05 | 거절 시연 8종 | 아래 3절의 합의 5종과 기기 3종이 각각 정해진 층에서 거절되고 체인에 결제가 남지 않는다 [N22] | 미실행 | 코드별 로그 |
| W12-06 | P07 영수증 (conditional: P07이 컷되지 않았을 때) | P07에서 W12-03 결제의 영수증을 조회한다. P07이 컷되면 키오스크가 finalized PaymentSettled 이벤트를 직접 읽은 기록으로 대신한다 | 미실행 | 조회 화면 또는 이벤트 조회 로그 |
| W12-07 | 키오스크 가스 잔액 | 잔액이 kioskMinGasBalance 미만이면 새 결제를 받지 않고 busy를 표시한다 [N10] | 미실행 | 잔액 조회와 화면 사진 |

## 3. 거절 시연

W12-05는 두 묶음으로 판정한다. 3.1은 인터뷰에서 합의한 5종이고, 3.2는 프로토콜 설계에서 생긴 기기 거절 코드 중 보안 약속을 직접 증명하는 3종이다. 늘어난 이유와 비용은 [DF-20260925-02 사이클 중 변경](../planning/design-freeze-checkpoint-02.md)에 적었다 [N18]. TIMEOUT(기기)과 BAD_FRAME(기기와 키오스크)은 시연하지 않고 P01·P04 시험에서 확인한다.

### 3.1 합의 5종

| 코드 | 거절하는 층 | 만드는 방법 | 결과 | 증거 |
|---|---|---|---|---|
| ATTESTATION_EXPIRED | 기기 | validUntil이 지난 attestation으로 payment.identify | 미실행 |  |
| MERCHANT_REVOKED | 컨트랙트 | registry에서 철회한 가맹점으로 결제 | 미실행 |  |
| OVER_CAP | 컨트랙트 | 건당 한도를 넘는 금액으로 결제 | 미실행 |  |
| NONCE_REPLAYED | 컨트랙트 | 시연 중에 LimitChange에 서명해 적용하고, 그 서명의 expiry(authorizationExpiry) 안에 같은 서명을 다시 제출한다. 결제 서명 재제출은 주문 유일성에서 먼저 걸려 approved로 끝나므로 이 코드를 만들지 않는다 | 미실행 |  |
| MERCHANT_FORGED | 기기 | P05 `opsctl refusal-host`가 결제 세션으로 운영자가 서명하지 않은 attestation과 payout을 바꾼 주문을 보낸다. 컨트랙트 층 MerchantForged는 P06 Foundry 시험으로 확인한다 | 미실행 |  |

### 3.2 기기 거절 3종

| 코드 | 거절하는 층 | 만드는 방법 | 결과 | 증거 |
|---|---|---|---|---|
| USER_REJECTED | 기기 | 금액 표시 뒤 대여자가 거절 버튼을 누른다 | 미실행 | 기기 화면 영상, payment.result 로그 |
| UNSUPPORTED_TYPE | 기기 | P05 `opsctl refusal-host`가 따로 페어링해 결제 세션(session.open, session.confirm)을 연 뒤, 스키마에 없는 원시 트랜잭션 서명·Permit 서명 요청을 보낸다 | 미실행 | 두 요청의 error 응답 로그, 등록되지 않은 purpose를 거부하는 secure 쪽 시험 로그 |
| TIME_ANCHOR_MISSING | 기기 | 기기 전원을 뽑았다 꽂은 뒤 결제를 시도한다 | 미실행 | anchorValid=false 로그, 재-anchor 후 결제 성공 로그 |

UNSUPPORTED_TYPE 요청은 키오스크가 아니라 P05 `opsctl refusal-host`로 보낸다. 키오스크 배포 빌드에 거절 유도 기능을 넣지 않기 위해서다.

### 3.3 시연 순서 (runbook)

준비를 한 번에 끝내고 기기 상태를 되돌리는 일이 없도록 순서를 고정한다. 전체 예상 시간은 약 25분이다.

1. **준비.** 만료된 attestation은 시연 시각보다 attestationValidity와 anchorClockSkew를 더한 시간 이상 먼저(시연 이틀 전) P05로 발급해 둔다. 철회용 시험 가맹점과 payout을 바꾼 주문은 시연 전날 만든다. LimitChange는 만료가 짧아 미리 만들지 않는다. W12 컨트랙트가 재배포되었으면 기기를 반납 절차(closeAccount, DeviceReset)로 되돌리고 새 컨트랙트 주소로 다시 셋업한다. 거절을 유도하는 요청(위조 attestation, 변조 주문, LimitChange 재제출)은 모두 P05 도구로 보내고 키오스크 배포 빌드에는 넣지 않는다. 기기는 대여 셋업과 TimeAnchor를 마친 상태로 둔다.
2. **합의 5종(약 10분).** 3.1 표 순서대로 진행한다. NONCE_REPLAYED는 이 자리에서 LimitChange에 서명·적용한 뒤 2분 안에 같은 서명을 다시 제출한다.
3. **USER_REJECTED(약 2분).** 정상 주문을 보내고 거절 버튼을 누른다.
4. **UNSUPPORTED_TYPE(약 3분).** P05 `opsctl refusal-host`로 두 요청을 보낸다. 끝나면 연결을 끊는다.
5. **TIME_ANCHOR_MISSING(약 10분, 마지막).** 전원을 뽑았다 꽂고 결제를 시도해 거절을 확인한다. 이어서 P05 `opsctl rental re-anchor`로 TimeAnchor를 다시 기록하고 결제 1건이 approved로 끝나는 것까지 보인다. 이 단계를 마지막에 두는 이유는 재-anchor 전까지 다른 결제가 모두 거절되기 때문이다.

## 4. 보안 항목

| ID | 시험 | 완료 조건 | 결과 | 증거 |
|---|---|---|---|---|
| W12-08 | TRNG 키 생성 | 키가 대여 셋업에서 TRNG로만 만들어지고 import 경로가 없다 [N11] | 미실행 | 코드 경로 검토, 셋업 로그 |
| W12-09 | 평문 키 없음 | AP-Protect를 켜기 직전의 W12 이미지(같은 image hash)에서 NVM 전체(TF-M ITS 포함)를 덤프해 평문 키나 시드가 없음을 확인한다 | 미실행 | 덤프 `sha256:`, image hash, 검색 결과 |
| W12-10 | 디버그 잠금 | AP-Protect가 켜져 디버거 접근이 거부된다 | 미실행 | 디버거 연결 로그 |
| W12-11 | 서명 안 된 펌웨어 거부 | MCUboot serial recovery로 올린 서명 안 된 이미지를 부팅하지 않는다. 대상 slot과 복구 후 서명된 이미지로 돌아오는 과정을 기록한다 [N12] | 미실행 | 부트 로그 |
| W12-12 | 키 반출 명령 없음 | 모든 전송(BLE, UART)의 명령 목록에 키 반출이 없고, W12 이미지에 USB CDC harness가 없으며, 모든 서명이 기기 버튼 승인 뒤에만 나온다 [N24] | 미실행 | 전송별 명령 목록, 이미지 구성 파일, 버튼 없는 요청의 거절 로그 |

## 5. 판정

W12-01..W12-12가 모두 통과하면 완료다. W12-06은 conditional 항목이어서 P07이 컷된 경우 키오스크의 PaymentSettled 직접 조회 기록으로 통과할 수 있다. 키 보관이 TF-M waiver인 경우 그 사실을 1절에 적는다 [N14].

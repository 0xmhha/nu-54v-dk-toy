# P05 기획 — 운영 스크립트

P05는 이번 사이클에서 웹 백오피스가 아니라 운영자가 직접 실행하는 Foundry 스크립트 묶음이다 [N20]. 가맹점 등록, MerchantAttestation 발급, 대여 셋업과 반납 처리처럼 운영자만 할 수 있는 일을 스크립트 한 번 실행으로 끝내고, 실행 로그를 증거로 남긴다. 제품 범위는 [DF-20260925-02](../../planning/design-freeze-checkpoint-02.md)가 정하며 P05는 만드는 제품 여섯 개 중 하나다 [N01]. 담당은 role B다 [N15].

## 1. 목표와 범위

| 기능 | 하는 일 | 관련 결정 |
|---|---|---|
| 가맹점 등록 | 가맹점 서명 주소와 payout을 P06 registry에 등록한다 | [N05] |
| MerchantAttestation 발급 | 운영자 키로 attestation에 서명하고 attestationValidity가 끝나기 전에 매일 다시 발급한다 | [N05] [N13] |
| 시험 가맹점 주문 서명 | 시험 가맹점 키로 MerchantOrder에 서명하는 보조 도구 | [N04] |
| 대여 셋업 | device.reset 뒤 기기가 만든 주소로 depositFor를 호출하고 TimeAnchor를 기기에 보낸다 | [N06] [N07] [N11] |
| 반납 | closeAccount를 호출하고 기기 초기화를 확인한다 | [N11] |
| 가맹점 철회 | 시연용으로 registry에서 가맹점을 철회한다 | [N05] [N22] |
| payout 변경 | payoutChangeDelay가 지난 뒤 효력이 생기는 payout 변경을 요청한다 | [N13] |

## 2. 산출물

- `script/RegisterMerchant.s.sol` — 가맹점 등록과 payout 설정
- `script/IssueAttestation.s.sol` — MerchantAttestation 서명과 JSON 출력
- `script/SignOrder.s.sol` — 시험 가맹점의 MerchantOrder 서명
- `script/ProvisionRental.s.sol` — depositFor 호출
- `script/IssueTimeAnchor.s.sol` — TimeAnchor 서명
- `script/CloseRental.s.sol` — closeAccount 호출
- `script/RevokeMerchant.s.sol`, `script/ChangePayout.s.sol`
- `tools/setup-client/` — BLE 셋업 세션으로 `setup.timeAnchor`를 보내는 호스트 도구
- 실행 로그 템플릿과 redaction 규칙 문서

## 3. 일정

| WBS | 내용 | 주차 | 게이트 |
|---|---|---|---|
| WBS2-P05-01 | 가맹점 등록·attestation 스크립트 | W1–W3 | W4 증거 게이트에서 소프트웨어 서명 정산에 쓰인다 |
| WBS2-P05-02 | provisioning(depositFor, TimeAnchor)과 반납(closeAccount) | W5–W7 | W6 실결제 게이트의 대여 셋업에 쓰인다 |

일정과 게이트 정의는 [12주 WBS](../../planning/product-worklist-and-12week-wbs-02.md)를 따른다 [N03].

## 4. 의존성

- P06 정산 컨트랙트 ABI와 8283 배포 manifest(WBS2-P06-01, WBS2-P06-02). ABI가 바뀌면 스크립트를 다시 맞춘다.
- P01의 BLE 셋업 세션. `setup.timeAnchor` 메시지는 [결제 프로토콜](../../specifications/protocol/payment-protocol.md) 5절을 따른다.
- P10의 EIP-712 스키마와 `eip712-vectors.json`. 서명 결과를 벡터로 교차 검증한다 [N21].

## 5. 위험

- 운영자 키가 단일 실패 지점이다. 키가 새면 가짜 attestation과 TimeAnchor를 만들 수 있다. testnet에서는 역할별 EOA 하나씩(운영자, registry 관리자, 키오스크)만 두고 multisig/HSM은 보류한다.
- 예치금은 운영자가 대신 넣는 custodial 구조다. 시험 전용 토큰만 쓴다.
- role B가 P04·P07과 함께 맡으므로 W5–W7 부하가 가용 일수에 가깝다.

## 6. 범위 밖

웹 백오피스, 운영자 RBAC와 감사 화면, FOTA 캠페인, 결제 예외 대사 화면은 만들지 않는다. refund 처리도 범위 밖이다 [N17].

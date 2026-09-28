# P05 기획 — 운영 백오피스

P05는 백오피스 전체(HTTP API, React UI, PostgreSQL)를 전제로 설계하고, 이번 사이클에는 그 아래의 Go 운영 코어와 CLI `opsctl`을 구현한다 [N20]. 가맹점 등록, MerchantAttestation 발급, 대여 셋업과 반납 처리처럼 운영자만 할 수 있는 일을 `opsctl` 명령 한 번으로 끝내고, 실행 기록을 감사 로그와 증거로 남긴다. 기술 스택은 [N25]를 따른다. 제품 범위는 [DF-20260925-02](../../planning/design-freeze-checkpoint-02.md)가 정하며 P05는 만드는 제품 여섯 개 중 하나다 [N01]. 담당은 role B다 [N15].

## 1. 목표와 범위

| 기능 | 하는 일 | 관련 결정 |
|---|---|---|
| 가맹점 등록 | registry 관리자 키로 가맹점 서명 주소와 payout을 P06 registry에 등록한다 | [N05] |
| MerchantAttestation 발급 | 운영자 키로 attestation에 서명하고 attestationValidity가 끝나기 전에 매일 다시 발급한다. payout 변경 요청 뒤에는 validUntil을 변경 효력 시각으로 잘라 발급한다 | [N05] [N13] |
| 시험 가맹점 주문 서명 | 시험 가맹점 키로 MerchantOrder에 서명하는 보조 도구 | [N04] |
| 대여 셋업 | 셋업 세션을 열어 `setup.operator`를 보내고(대여자 버튼 확인), 기기가 TRNG 키와 PIN을 만든 뒤 `setup.ack{keygen, device}`로 알린 주소로 TimeAnchor에 서명해 `setup.timeAnchor`를 보내며, 수락 ack를 확인한 뒤에만 depositFor를 호출한다 | [N06] [N07] [N11] [N23] |
| 반납 | closeAccount를 호출하고 finalized 이벤트를 확인한 뒤 운영자가 서명한 DeviceReset을 `device.reset`으로 보낸다 | [N11] [N23] |
| 가맹점 철회 | 시연용으로 registry 관리자 키로 가맹점을 철회한다 | [N05] [N22] |
| payout 변경 | registry 관리자 키로 payoutChangeDelay(attestationValidity 이상)가 지난 뒤 효력이 생기는 payout 변경을 요청하고(필요하면 cancelPayoutChange로 되돌림), 효력 시각에 새 payout attestation을 발급한다 | [N05] [N13] |
| 시험 가맹점 키 전달 | 시험 가맹점 서명 키를 P05 도구로 만들어 키오스크 설정에 secretRef로 전달한다. 키 원문은 저장소에 올리지 않는다 | [N05] |

## 2. 산출물

이번 사이클에 만드는 것:

- Go 모듈 `products/p05-operations-backoffice`
  - `internal/core` — 체인 호출(go-ethereum ethclient), EIP-712 서명(packages/protocol/go 생성 타입), secretRef로 참조하는 암호화 keystore, 감사 기록
  - `internal/ble` — BLE 셋업 클라이언트(tinygo-org/bluetooth). `session.open`(setup), `setup.operator`, `setup.ack`, `setup.timeAnchor`, `device.reset`, 거절 시연 요청을 결제 프로토콜 4절 틀로 보낸다
  - `internal/store` — PostgreSQL 저장소(가맹점, 대여, attestation, 감사)와 migration
  - `cmd/opsctl` — CLI. 명령 묶음은 다음 사이클 API 자원과 같게 나눈다: `merchant`(register, revoke, payout-change, payout-cancel), `attestation issue`, `order sign`(시험 가맹점), `rental provision`, `rental re-anchor`, `rental return`, `withdraw`(request, cancel, execute), `refusal-host`, `merchant-key handover`
- 실행 로그 템플릿과 redaction 규칙 문서

설계만 하고 다음 사이클에 만드는 것:

- `cmd/opsd` — 같은 코어 위의 HTTP API 서버
- `web/` — React·TypeScript 백오피스 UI

## 3. 일정

| WBS | 내용 | 주차 | 게이트 |
|---|---|---|---|
| WBS2-P05-01 | Go 운영 코어와 `opsctl`의 가맹점 등록·attestation·주문 서명(P06-01의 최소 registry 사용) | W6 | W7 실결제 게이트의 가맹점 준비 |
| WBS2-P05-02 | `opsctl rental provision`: BLE 셋업(`setup.operator`, TimeAnchor)과 depositFor | W7 | W7 실결제 게이트의 대여 셋업 |
| WBS2-P05-03 | `opsctl rental return`과 `withdraw`: closeAccount, 서명된 DeviceReset, 출금 요청·취소·실행. P06의 closeAccount 구현(WBS2-P06-04) 뒤에 한다 | W10 | - |
| WBS2-P05-04 | `opsctl merchant revoke/payout-change`와 거절 시연 `refusal-host` | W9–W10 | - |

게이트는 W4 증거, W6 컨트랙트, W7 실결제이며(W9 SE 게이트는 2026-09-29에 없앴다) 일정과 정의는 [12주 WBS](../../planning/product-worklist-and-12week-wbs-02.md)를 따른다 [N03].

## 4. 의존성

- P06 정산 컨트랙트와 최소 registry의 ABI, 8283 배포 manifest(WBS2-P06-01, WBS2-P06-02). payout 변경은 WBS2-P06-03 뒤에, 반납은 WBS2-P06-04 뒤에 쓴다. ABI가 바뀌면 packages/contracts-abi의 Go 바인딩을 다시 생성한다.
- P01의 BLE 셋업 세션과 `device.reset` 처리. 메시지는 [결제 프로토콜](../../specifications/protocol/payment-protocol.md) 5절을 따른다.
- P10의 EIP-712 스키마와 `eip712-vectors.json`. 서명 결과를 벡터로 교차 검증한다 [N21].

## 5. 위험

- 운영자 키가 단일 실패 지점이다. 키가 새면 가짜 attestation과 TimeAnchor를 만들 수 있다. testnet에서는 역할별 EOA 하나씩(운영자, registry 관리자, 키오스크, 시험 가맹점)만 두고 multisig/HSM은 보류한다.
- 예치금은 운영자가 대신 넣는 custodial 구조다. 시험 전용 토큰만 쓴다.
- role B가 P04와 함께 맡으므로 W6, W9에 가용 일수를 모두 쓴다.
- Go BLE 라이브러리(tinygo-org/bluetooth)는 이 환경에서 아직 시험하지 않았다. W7 전에 셋업 세션 한 번으로 확인한다.

## 6. 범위 밖

이번 사이클에는 `opsd` API 서버와 `web/` 백오피스 화면, 운영자 RBAC, FOTA 캠페인, 결제 예외 대사 화면을 만들지 않는다. 설계는 [design.md](design.md) 9절에 둔다. refund 처리도 범위 밖이다 [N17].

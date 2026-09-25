# P05 유즈케이스 — 운영 스크립트

행위자는 운영자(role B가 대행), 기기(P01), 정산 컨트랙트(P06)다. 각 유즈케이스는 [SRS](srs.md)의 요구로 이어진다. 결정은 [DF-20260925-02](../../planning/design-freeze-checkpoint-02.md)를 따른다 [N20].

## 1. UC-P05-01 가맹점 등록

- **사전 조건**: 가맹점 서명 주소와 payout 주소가 준비되어 있고 P06이 8283에 배포되어 있다.
- **기본 흐름**: 1) 운영자가 `RegisterMerchant`를 실행한다. 2) 스크립트가 chainId를 확인한다. 3) registry에 서명 주소와 payout을 등록한다. 4) 등록 이벤트를 finalized 블록에서 확인하고 로그를 남긴다.
- **대안 흐름**: 이미 같은 값이면 트랜잭션 없이 끝난다. payout이 다르면 UC-P05-06으로 안내하고 멈춘다.
- **사후 조건**: registry가 가맹점을 활성으로 보고한다 [N05].
- **요구**: P05-FR-01, P05-NFR-04

## 2. UC-P05-02 attestation 매일 재발급

- **사전 조건**: 가맹점이 registry에 활성이다.
- **기본 흐름**: 1) 운영자가 `IssueAttestation`을 실행한다. 2) 스크립트가 registry payout과 입력 payout이 같은지 본다. 3) validFrom을 현재 finalized 블록 시각으로, 기간을 attestationValidity로 두고 서명한다 [N13]. 4) 서명된 attestation JSON을 키오스크에 전달한다.
- **예외 흐름**: payout 불일치면 서명하지 않는다. 운영자가 재발급을 놓치면 기기가 ATTESTATION_EXPIRED로 거절한다 [N22].
- **요구**: P05-FR-02, P05-FR-03

## 3. UC-P05-03 대여 셋업

- **사전 조건**: 반납 처리된 기기, 대여자 withdrawAddress, 예치 토큰.
- **기본 흐름**:
  1. 기기가 device.reset 상태로 셋업 세션을 연다.
  2. 기기가 TRNG로 새 키를 만들고 주소만 보고한다 [N11].
  3. 대여자가 기기에서 PIN을 정한다.
  4. 운영자 도구가 TimeAnchor에 서명해 `setup.timeAnchor`로 보낸다 [N06].
  5. 기기가 anchor를 받아들였다고 응답한다.
  6. 운영자가 `ProvisionRental`로 depositFor(deviceAddress, amount, withdrawAddress)를 호출한다 [N07].
- **예외 흐름**: 기기가 anchor를 거부하면(서명자 불일치, 이전보다 이른 시각) 예치하지 않는다. depositFor가 실패하면 기기를 다시 reset하지 않고 같은 주소로 재시도한다.
- **사후 조건**: 기기 주소에 예치금이 있고 기기의 anchor가 유효하다.
- **요구**: P05-FR-05, P05-FR-06

## 4. UC-P05-04 반납

- **기본 흐름**: 1) 운영자가 `CloseRental`로 closeAccount를 호출한다. 2) 기기 주소가 즉시 비활성화되고 잔액은 withdrawalDelay 뒤 등록 주소로 지급된다 [N13]. 3) 이벤트 확인 뒤 기기에 device.reset을 지시한다.
- **예외 흐름**: closeAccount가 실패하면 기기를 초기화하지 않는다. 키를 지우면 남은 잔액 처리를 추적할 기기 주소 증거가 사라지기 때문이다.
- **요구**: P05-FR-07

## 5. UC-P05-05 가맹점 철회

- **기본 흐름**: 운영자가 `RevokeMerchant`를 실행하고, 이후 해당 가맹점 결제가 eth_call 단계에서 MERCHANT_REVOKED로 막히는지 확인한다 [N22].
- **요구**: P05-FR-08

## 6. UC-P05-06 payout 변경

- **기본 흐름**: 1) 운영자가 `ChangePayout`으로 변경을 요청한다. 2) payoutChangeDelay 동안 기존 payout이 유지된다. 3) 지연이 끝나면 새 payout으로 UC-P05-02를 실행한다.
- **예외 흐름**: 지연 중 새 payout attestation 발급은 거부한다.
- **요구**: P05-FR-09

## 7. UC-P05-07 전원 차단 뒤 재anchor

- **사전 조건**: 기기가 `anchorValid=false`를 보고하고 결제를 TIME_ANCHOR_MISSING으로 거절한다.
- **기본 흐름**: 운영자가 기기와 셋업 세션을 다시 열고 새 TimeAnchor를 보낸다. 키와 예치금은 그대로다.
- **예외 흐름**: 새 timestamp가 기기의 이전 anchor보다 이르면 도구가 보내지 않는다.
- **요구**: P05-FR-10

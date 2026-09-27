# P05 유즈케이스 — 운영 백오피스

행위자는 운영자(role B가 대행), 기기(P01), 정산 컨트랙트(P06)다. 각 유즈케이스는 [SRS](srs.md)의 요구로 이어진다. 결정은 [DF-20260925-02](../../planning/design-freeze-checkpoint-02.md)를 따른다 [N20].

## 1. UC-P05-01 가맹점 등록

- **사전 조건**: 가맹점 서명 주소와 payout 주소가 준비되어 있고 P06이 8283에 배포되어 있다.
- **기본 흐름**: 1) 운영자가 registry 관리자 키로 `opsctl merchant register`를 실행한다. 2) `opsctl`이 chainId를 확인한다. 3) registry에 서명 주소와 payout을 등록한다. 4) 등록 이벤트를 finalized 블록에서 확인하고 로그를 남긴다.
- **대안 흐름**: 이미 같은 값이면 트랜잭션 없이 끝난다. payout이 다르면 UC-P05-06으로 안내하고 멈춘다.
- **사후 조건**: registry가 가맹점을 활성으로 보고한다 [N05].
- **요구**: P05-FR-01, P05-NFR-04

## 2. UC-P05-02 attestation 매일 재발급

- **사전 조건**: 가맹점이 registry에 활성이다.
- **기본 흐름**: 1) 운영자가 `opsctl attestation issue`를 실행한다. 2) `opsctl`이 registry payout과 입력 payout이 같은지 본다. 3) validFrom을 현재 finalized 블록 시각으로, 기간을 attestationValidity로 두고 서명한다 [N13]. 4) 서명된 attestation JSON을 키오스크에 전달한다.
- **대안 흐름**: payout 변경 대기 중이면 validUntil을 변경 효력 시각으로 잘라 발급한다(UC-P05-06).
- **예외 흐름**: payout 불일치면 서명하지 않는다. 운영자가 재발급을 놓치면 기기가 ATTESTATION_EXPIRED로 거절한다 [N22].
- **요구**: P05-FR-02, P05-FR-03

## 3. UC-P05-03 대여 셋업

- **사전 조건**: 반납 절차로 `UNPROVISIONED` 상태인 기기, 대여자 withdrawAddress, 예치 토큰.
- **기본 흐름**:
  1. 운영자가 `opsctl rental provision`을 실행하면 `session.open`(mode=setup)으로 셋업 세션이 열린다.
  2. `opsctl`이 `setup.operator{operator, contract, chainId}`를 보내고 세 값을 운영자 화면에 보여 준다. 대여자가 기기 버튼으로 확인한다 [N23].
  3. 기기가 TRNG로 새 키를 만들고 대여자가 기기 버튼으로 PIN을 정한다. `opsctl`이 기기별 passkey를 기록하고 라벨 QR을 인쇄한 뒤 기기가 `setup.ack{step: keygen, device}`로 새 주소를 알린다 [N11].
  4. `opsctl`이 그 주소와 최신 finalized 블록 시각으로 TimeAnchor에 서명해 `setup.timeAnchor{device, timestamp, operatorSignature}`로 보낸다 [N06].
  5. 기기가 `setup.ack{step: setup.timeAnchor, accepted, lastAnchor}`로 수락을 알린다.
  6. `opsctl rental provision`이 keygen ack의 device 주소로 depositFor(deviceAddress, amount, withdrawAddress)를 호출한다 [N07].
- **예외 흐름**: 기기가 anchor를 거부하면(서명자 불일치, 이전보다 이른 시각) 예치하지 않는다. depositFor가 실패하면 기기를 다시 reset하지 않고 같은 주소로 재시도한다.
- **사후 조건**: 기기가 `READY`이고 기기 주소에 예치금이 있으며 anchor가 유효하다.
- **요구**: P05-FR-05, P05-FR-06

## 4. UC-P05-04 반납

- **기본 흐름**: 1) 운영자가 `opsctl rental return`으로 closeAccount를 호출한다. 2) 기기 주소가 즉시 비활성화되고 잔액은 withdrawalDelay 뒤 등록 주소로 지급된다 [N13]. 3) finalized 이벤트를 확인한 뒤 DeviceReset `{device, nonce}`에 운영자 키로 서명하고 `device.reset`으로 보낸다 [N23].
- **예외 흐름**: closeAccount가 실패하면 기기를 초기화하지 않는다. 계정을 먼저 비활성화해야 키가 사라진 뒤에도 예치금이 남아 있는 활성 계정이 생기지 않는다.
- **요구**: P05-FR-07

## 5. UC-P05-05 가맹점 철회

- **기본 흐름**: 운영자가 registry 관리자 키로 `opsctl merchant revoke`를 실행하고, 이후 해당 가맹점 결제가 eth_call 단계에서 MERCHANT_REVOKED로 막히는지 확인한다 [N22].
- **요구**: P05-FR-08

## 6. UC-P05-06 payout 변경

- **기본 흐름**: 1) 운영자가 registry 관리자 키로 `opsctl merchant payout-change`를 실행해 변경을 요청한다. 2) payoutChangeDelay(attestationValidity 이상) 동안 기존 payout이 유지되고, 이 기간에 발급하는 옛 payout attestation은 validUntil을 변경 효력 시각으로 자른다 [N05]. 3) 효력 시각에 새 payout으로 UC-P05-02를 실행한다.
- **예외 흐름**: 지연 중 새 payout attestation 발급은 거부한다. 변경을 되돌릴 때는 `opsctl merchant payout-cancel`로 cancelPayoutChange를 호출한다.
- **요구**: P05-FR-09

## 7. UC-P05-07 reset 뒤 재anchor

- **사전 조건**: 전원 손실이나 watchdog처럼 RAM이 초기화되는 reset 뒤 기기가 `PROVISIONED_NO_ANCHOR`이며 `anchorValid=false`를 보고하고 결제를 TIME_ANCHOR_MISSING으로 거절한다.
- **기본 흐름**: `opsctl rental re-anchor`가 `session.open`(mode=setup)으로 셋업 세션을 열고 `session.open.ok`의 lastAnchor를 읽은 뒤 `setup.timeAnchor`만 보낸다 [N06]. 키와 예치금은 그대로다.
- **예외 흐름**: 새 timestamp가 lastAnchor보다 엄격히 늦지 않으면 `opsctl`이 보내지 않는다.
- **요구**: P05-FR-10

## 8. UC-P05-08 거절 시연 준비

- **기본 흐름**: 1) 시연 시각보다 attestationValidity와 anchorClockSkew를 더한 시간 이상 먼저 attestation을 발급해 둔다(소급 발급 없음). 2) 시연 때 `opsctl refusal-host`가 페어링 없이 결제 세션을 열고 스키마 밖 요청을 보낸다 [N22].
- **요구**: P05-FR-11, P05-FR-12

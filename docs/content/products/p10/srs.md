# P10 SRS — 공유 EIP-712 타입·벡터와 수용 기준

## 1. 목적

P10이 제공하는 타입, 벡터, 적합성 harness, 수용 기록지의 요구를 시험으로 판정할 수 있게 적는다. 규칙 본문은 [결제 프로토콜](../../specifications/protocol/payment-protocol.md)에 있다.

## 2. 서명 타입

기기가 서명하는 EIP-712 타입은 PaymentAuthorization과 LimitChange 두 개뿐이다 [N04]. 운영자·가맹점이 서명하는 MerchantAttestation, MerchantOrder, TimeAnchor, DeviceReset은 스키마의 `operatorSignedTypes`에 따로 둔다. 스키마의 `eip712Types`에 다른 타입을 넣으면 validator가 실패한다.

## 3. 기능 요구

| ID | 요구 | 확인 방법 |
|---|---|---|
| P10-FR-01 | 스키마의 `eip712Types`는 PaymentAuthorization `{chainId, contract, merchant, payout, token, amount, orderId, nonce, expiry}`와 LimitChange만 담는다 [N04] | `validate_design_freeze_02.py --check protocol+vectors` |
| P10-FR-02 | 벡터 파일은 기기 서명 타입마다 한 개 이상, 운영자·가맹점 타입(DR-01 DeviceReset 포함)마다 한 개 이상의 벡터를 담고, 각 벡터의 message 키는 스키마 필드와 같다. PaymentAuthorization·LimitChange 벡터의 expiry는 기준 시각부터 authorizationExpiry 안에 있다 [N13] | 같은 검사 |
| P10-FR-03 | 생성기는 digest를 직접 계산하고 `cast wallet sign --data`의 서명이 그 digest로 검증되지 않으면 실패한다 | 생성기 실행 |
| P10-FR-04 | 생성기 `--check`는 커밋된 벡터 파일과 새로 만든 결과가 바이트 단위로 같을 때만 통과한다 | `build_eip712_vectors.py --check` |
| P10-FR-05 | 적합성 harness는 펌웨어(C), 키오스크(TS), 컨트랙트(Solidity) 구현이 각자 계산하는 타입의 벡터 digest를 같게 만들고(펌웨어는 7개 벡터 전부, 키오스크는 PaymentAuthorization·LimitChange·MerchantOrder, 컨트랙트는 PaymentAuthorization·LimitChange), 벡터 서명에서 복원한 signer가 그 벡터의 signerRole(device, operator, merchant)에 해당하는 주소인지 확인한다 | harness 실행 로그 |
| P10-FR-06 | 수용 기록지는 W12-01..W12-12와 register `acceptanceRefusalCodes`의 거절 시연 8종(합의 5종과 기기 3종)을 담고, P07 항목은 conditional로 표시한다 [N18] | `--check acceptance` |
| P10-FR-07 | 증거는 원본을 git 밖에 두고, 문서에는 줄인 주소·tx hash(앞 6자리…뒤 4자리)와 원본 파일의 64자리 전체 `sha256:` checksum만 남긴다 [N16] | `--check redaction` |

## 4. 비기능 요구

| ID | 요구 |
|---|---|
| P10-NFR-01 | 벡터 서명 계정은 Foundry 공개 시험 mnemonic에서 역할마다 다른 index(기기 0, 운영자 1, 가맹점 2)를 쓰며 어떤 네트워크에서도 자금을 넣지 않는다 |
| P10-NFR-02 | 저장소에 개인키 문자열을 두지 않는다. 생성기는 mnemonic 문구로 cast를 호출한다 |
| P10-NFR-03 | 벡터 주소는 반복 바이트 placeholder를 쓴다. 벡터 파일만 redaction 예외다 |

## 5. 제약과 waiver

다음은 이번 사이클에서 설계만 하는 waiver다 [N14].

| 항목 | 내용 | 기록 위치 |
|---|---|---|
| req 10 anti-exfil | 서명 nonce 조작으로 키가 새는 경로 방지는 설계만 한다 | P01 design의 waiver 절 |
| req 12 dependency pinning | 의존성 해시 고정 자동화는 하지 않는다. 사용한 버전은 수용 기록지 1절의 도구·의존성 버전 행에 적는다 | week12-log 1절 |
| req 13 genuine-device attestation | 정품 기기 증명은 설계만 한다 | P01 design의 waiver 절 |
| TF-M 키 봉인 | 외부 SE 도입(W9 SE 게이트) 전까지, 또는 SE가 컷되면 W12까지 TF-M 봉인으로 운영한다 | week12-log 1절 |

## 6. 추적

| 요구 | 결정 | 수용 항목 |
|---|---|---|
| P10-FR-01..05 | [N04][N21] | W12-03, W12-05 |
| P10-FR-06 | [N18] | W12-01..W12-12 |
| P10-FR-07 | [N16] | 모든 항목의 증거 |

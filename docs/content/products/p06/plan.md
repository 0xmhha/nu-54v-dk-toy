# P06 기획 — 정산 컨트랙트

StableNet testnet 8283에 배포하는 결제 정산 컨트랙트와 가맹점 registry의 12주 계획이다. 이 문서는 [DF-20260925-02](../../planning/design-freeze-checkpoint-02.md)를 따르며, 값은 register의 parameters에만 있고 여기서는 이름으로만 부른다 [N13]. 요구는 [srs.md](srs.md), 사용 흐름은 [use-cases.md](use-cases.md), 구조는 [design.md](design.md)에 있다.

## 1. 목표와 범위

DF-20260925-02에서 P06은 이번 사이클에 만드는 일곱 제품 중 하나다 [N01]. 목표는 하나다. 기기가 서명한 PaymentAuthorization을 키오스크가 제출하면, 컨트랙트가 가맹점 권한·한도·nonce·주문 유일성·만료를 검사하고 사전 예치금에서 가맹점 잔액으로 옮긴 뒤 PaymentSettled 이벤트를 남긴다 [N07][N08].

범위에 들어가는 것:

- 정산 컨트랙트 하나: 사전 예치(depositFor), 결제 정산(settle), 한도 변경(LimitChange), 지연 출금, closeAccount, 가맹점 cash-out.
- 가맹점 registry: 등록·철회·payout 변경(지연 적용).
- 배포마다 토큰 하나(dummy USDC 6자리). 여러 토큰을 한 배포에서 섞지 않는다.
- 8283 배포 manifest(주소, codeHash, ABI digest, 배포 블록)와 Foundry 시험.

## 2. 산출물

| 산출물 | 형태 | 완료 증거 |
|---|---|---|
| 정산 컨트랙트와 registry 소스 | Solidity, Foundry 프로젝트(`products/p06-stablenet-contracts/`) | `forge test` 전체 통과 로그 |
| EIP-712 적합성 시험 | P10 [eip712-vectors.json](../../specifications/protocol/eip712-vectors.json)을 읽는 Foundry 시험 | 벡터 digest 일치 로그 |
| 8283 배포 manifest | JSON(줄인 주소, codeHash, 배포 블록) | 배포 로그와 `sha256:` checksum |
| W6 소프트웨어 서명 정산 증거 | 소프트웨어 키로 만든 서명을 제출한 finalized PaymentSettled | 줄인 tx hash와 이벤트 로그 |

## 3. 일정과 게이트

행 데이터는 [WBS-02](../../planning/product-worklist-and-12week-wbs-02.md)에 있다. 담당은 사용자(user)다.

| WBS id | 작업 | 주차 | 관련 게이트 |
|---|---|---|---|
| WBS2-P06-01 | 핵심 로직(depositFor, settle, nonce, 한도, 주문 유일성)과 최소 registry(등록·철회·payout 조회) | W5–W6 | W6 |
| WBS2-P06-02 | 8283 배포와 소프트웨어 서명 PaymentSettled | W6 | W6 |
| WBS2-P06-03 | payout 변경 지연과 LimitChange | W7–W8 | - |
| WBS2-P06-04 | 지연 출금과 closeAccount | W8–W9 | - |
| WBS2-P06-05 | 강화와 finality 관측(1시간 poll) | W9–W10 | - |

게이트는 [N03]을 따른다. 동결일(9/25)이 W4 안이므로 P06 작업은 W5부터 시작한다. W4 증거 게이트(bring-up, SE 데이터시트)에는 P06 산출물이 없다. W6 컨트랙트 게이트에서는 기기 없이 소프트웨어 서명으로 8283에서 finalized PaymentSettled 1건을 보여야 하며, 이때 최소 registry 검사가 켜져 있어야 한다. W7 실결제 게이트는 보드 내장 키로 같은 흐름을 실기로 통과해야 하고, W9 SE 게이트는 외부 SE 키로 같은 흐름을 반복하므로 컨트랙트 변경 없이 통과해야 한다.

W7 게이트가 실패해도 P06 작업은 컷 대상이 아니다. 컷 대상은 P07이며 [N03]을 따른다. 외부 SE와 NFC handover는 이미 뺐다.

## 4. 의존성

- **P10:** EIP-712 타입 패키지(WBS2-P10-01, W4). P06-01의 서명 검증 시험이 이 벡터에 의존한다 [N21].
- **P05:** 가맹점 등록, attestation 발급(WBS2-P05-01, W6), provisioning과 depositFor(WBS2-P05-02, W7), 반납 closeAccount(WBS2-P05-03, W10, P06-04 뒤)를 수행하는 Go 운영 코어와 `opsctl` [N20]. W6에는 settle, PaymentSettled, custom error 이름을 고정해 P05-01이 같은 주에 쓸 수 있게 한다. W7–W9에 더하는 함수(payout 변경 지연, LimitChange, 출금, closeAccount)는 이 고정분을 바꾸지 않는 추가만 하고, W12 배포는 P06-05 뒤의 재배포다 [N20].
- **P04:** 키오스크가 settle을 eth_call로 시뮬레이션하고 custom error 이름을 읽는다. error 이름과 PaymentSettled 시그니처는 W6에 고정하며 이후 추가 함수는 이를 바꾸지 않는다 [N22].

## 5. 위험

| 위험 | 영향 | 대응 |
|---|---|---|
| 감사받지 않은 컨트랙트 | 정산 로직 버그로 예치금이 빠져나갈 수 있다. 조사 문서가 인용한 2026-06 Gnosis Pay 사고도 카드 경로가 아니라 컨트랙트 버그 계열이었다([조사](../../research/payment-hw-wallet-direction/README.md)) | fuzz·invariant 시험, 외부 호출 최소화, testnet 한정 |
| 수탁형 예치 | 운영자가 depositFor로 넣은 예치금을 컨트랙트가 보관한다(testnet custodial PoC) | 출금·closeAccount는 등록된 주소로만 지급 |
| 역할 키 단일 실패점 | registry admin 키가 새면 가짜 가맹점을 등록하거나 payout 변경을 요청할 수 있다. 역할 키는 constructor에서 고정되어 탈취 자체는 막지 못한다 | 역할별 EOA 분리, payoutChangeDelay 동안 공개 이벤트로 감지해 cancelPayoutChange, registry 철회, multisig/HSM은 보류로 기록 |
| 사용자 부하 | 사용자가 W4에 bring-up을, W10–W11에 컨트랙트 강화와 P07을 함께 맡고, W6에는 가용 일수를 모두 쓴다 | W6 컨트랙트 게이트 뒤 W7 실결제 게이트까지 한 주뿐이라 W6 결함 수정을 W7 첫날에 끝낸다 |

## 6. 범위 밖

스마트 계정(ERC-4337/7579), AMM·Perpetual, CafePass·DID·x402는 이번 사이클에서 만들지 않는다 [D11][D12][D16]. refund 기능도 없다 [N17]. 가맹점 간 정산·원화 환산도 다루지 않는다.

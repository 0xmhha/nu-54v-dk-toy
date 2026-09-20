# DeFi·FX·Perpetual 상품별 상태와 처리 규칙

2026-09-19 · **DS-05 상세 설계 후보이며 구현·시장 선택·가격원 선정·실제 거래는 수행하지 않았다.** 15요구·12주 앱 연동·3명·104작업/320세부작업을 유지한다. 문서의 안전 기본 동작과 비교안은 사용자 확정 정책이 아니다.

[구조화 원본](market-product-design.json) · [검증기](validate_market_products.py) · [공통 자산/관측 설계](stablenet-compatibility-design.md) · [기존 재사용 조사](../expanded-scope-reuse-inventory.md)

## 1. 제품 화면에서 구분할 것

U14는 교환과 유동성 관리, U15는 선택된 FX 모델의 견적·결과, U16은 Perpetual 포지션·담보·손익·펀딩·청산을 표시한다. 지갑 잔고, LP 지분, 담보, 미실현 손익, 출금가능액은 서로 다른 수치다. 승인만 완료된 상태를 거래 성공으로 보여주지 않고, 제출/관측/실제 자산·포지션 반영을 구분한다.

기존 코드의 V2 고정 reserve·Perpetual 고정가격·펀딩/청산 미완성은 저장된 조사 결과다. 현재 HEAD나 배포본을 새로 검증하지 않았으며 이를 실제 유동성/가격·완료 기능으로 채택하지 않는다. 이 문서는 투자 추천이나 실자산 상품 운영 승인도 아니다.

## 2. 상품별 카드

### PD-01 · 토큰 교환

- 화면/경로: U14 · API-057, API-058.
- 모델 후보: exact-input 또는 exact-output, 검증 pool/router/route·fee tier 선택.
- 검토에 묶는 내용: assetIn/out·atomic amount·recipient·spender·기한·최소수취/최대지출·quote block/hash.
- 완료 조건: 실제 swap 실행·토큰 증거·수취량·가스 확인; allowance만 성공이면 미실행.
- 미선택 부분: pool 모델/route/quote source/fee·부분체결 지원 미선정.

### PD-02 · 유동성 추가·회수·수수료 수령

- 화면/경로: U14 · API-057, API-058, API-059.
- 모델 후보: fungible LP 또는 범위형 position 선택, add/remove/collect 각각 검토.
- 검토에 묶는 내용: pool/token pair·position owner/id·range/fee tier·지분/유동성·토큰별 minima.
- 완료 조건: 추가/축소된 유동성, 실제 수취/수수료와 잔여 position을 각각 관측.
- 미선택 부분: 회수와 collect가 한 호출인지 별도인지 ABI검증 필요; 같은 shareAmount를 모든 모델에 적용 금지.

### PD-03 · FX

- 화면/경로: U15 · API-057, API-058.
- 모델 후보: USD/KRW 시험자산 현물 교환 후보 또는 별도 파생모델 비교; selection=null.
- 검토에 묶는 내용: base/quote 통화방향·token identity·rateNum/rateDen·단위·spread/fee·가격시점.
- 완료 조건: 선택 모델 실제 체결/수취 또는 포지션을 표시; 은행 환전/법정통화 인출 완료 주장 금지.
- 미선택 부분: FX를 spot로 확정하지 않음; 파생선택 시 별도 profile/증거금·가격·정산 계약 필요.

### PD-04 · Perpetual 포지션

- 화면/경로: U16 · API-060, API-061, API-062.
- 모델 후보: 개설/증액/축소/종료·담보 입출금·펀딩·청산; isolated/cross, linear/inverse 비교.
- 검토에 묶는 내용: 시장·담보·방향/수량·price/mark/index·position revision·수수료·담보한도·risk policy.
- 완료 조건: 실제 position/담보·realized/unrealized PnL·funding·liquidation 상태를 구분.
- 미선택 부분: 모델·레버리지·margin ratio·청산/보험/부채처리·가격원·funding 규칙 미선정.

Uniswap V3 문서는 exact-input과 exact-output의 보호 입력을 구분하고 최소수취·기한·미사용 입력 처리를 설명한다. 여기서는 그 구분을 참고하며 예제 코드의 주소·0 최소수취값을 가져와 운영값으로 쓰지 않는다. [V3 swap 문서](https://developers.uniswap.org/docs/protocols/v3/guides/swapping/single-hop-swapping)

범위형 유동성은 가격 범위를 갖기 때문에 범위/position 정보가 필요하다. LP를 모두 단순 지분 토큰으로 표시하지 않으며 실제 선정 모델에 맞는 schema를 요구한다. [집중 유동성](https://developers.uniswap.org/docs/get-started/concepts/liquidity-providers/concentrated-liquidity)

allowance는 지정 spender가 사용할 수 있는 권한이다. 승인 transaction과 자산 교환은 별도 단계이며, 거래 취소/실패만으로 이미 설정된 allowance가 사라지지 않는다. 철회는 별도 명시적 작업으로 검토한다. [ERC-20](https://eips.ethereum.org/EIPS/eip-20)

## 3. 견적·승인·실행·복구

| ID | 전이 | 경로 | 조건 | 기록/화면 |
|---|---|---|---|---|
| MT-01 | draft → quoted | API-057 또는 API-061 상세 market profile | 등록 market/asset/price/risk policy와 현재 source 확인 | 불변 quoteRef/digest·block/hash·가격유효성·기한. 견적은 서명권 아님 |
| MT-02 | quoted → allowance_pending | allowance 단계 후보 | 선정 spender/code·amount/expiry·token 동작 표시 및 별도 승인 | allowance tx/permit 별도 operation. 무제한승인을 몰래 기본값으로 사용하지 않음 |
| MT-03 | allowance_pending → review_ready | 권한있는 원 결과 조회 | standalone approve는 관측 후 실제 allowance 확인. bundled permit은 선정 profile의 exact token/spender/amount/nonce/deadline와 plan 결합 서명 검증으로 준비만 확인; quote 만료면 새검토 | approve 적용완료와 permit_prepared를 구분; permit 생성은 onchain allowance 적용 아님. 소비/실제 allowance·자산효과는 실행 receipt에서 확인; 로컬취소가 노출 permit을 무효화하지 않음 |
| MT-04 | quoted/review_ready → execution_pending | API-058/API-061 + 미등록 market_action signer adapter | 현 wallet/source·quote/price/position revision·fee/min/max/risk gate 재검사와 물리/MPC 승인 | 실제 calldata와 표시 snapshot 결합; 제출은 원 source로만. adapter 미등록이면 실행 차단 |
| MT-05 | execution_pending → result_observed | 원 tx/UserOp typed reader 후보 | receipt/개별 UserOp와 실제 action 효과·block/hash 검증 | 실패/부분/성공과 내부호출별 효과 기록; receipt 성공만으로 전체 plan 성공 금지 |
| MT-06 | result_observed → completed | 해당 profile의 Indexer/업무 projection | 선정 confirmation·결과 minima/한도·position/잔고 관측, 현재 revision CAS | 실제 수취/지분/포지션 변화 표시; indexer lag은 unknown/stale |
| MT-07 | quoted/review_ready → expired | quote deadline | 새 서명 방지; 기생성/노출 서명 여부 별도 검사 | 만료 후 새 quote는 새 검토. 이미 나온 서명을 무효화했다고 표시하지 않음 |
| MT-08 | allowance_pending/execution_pending/result_observed/completed → unknown | 응답유실/단절/reorg | 원 execution/observation을 재조회하며 exposure 유지 | auto retry는 검증된 동일 payload 재전송만; nonce/new intent 자동생성 금지; 과거 완료 결과/이력은 보존하고 현재 유효성을 재확인, 원 effect identity로 보정 |
| MT-09 | draft/quoted/review_ready → cancelled_local | 사용자 취소 | 외부 서명/permit/order 노출 없음이 확인된 로컬 plan | onchain allowance·주문 취소 아님; 노출 가능하면 unknown/pending 관측 |

여러 단계의 plan은 allowance child와 action execution child를 별도 식별한다. LP remove와 collect는 별도 transaction 또는 실패를 허용하는 호출에서 부분 성공할 수 있다. 원자적 multicall 전체가 revert하면 양쪽 효과가 rollback되므로, 실제 선정 ABI의 원자성과 receipt를 확인해 단계별 결과를 판정한다. unknown 복구는 원 execution/receipt/현재 contract 상태에 근거해 원 단계로 돌아가고, 클라이언트가 phase를 지정하거나 새 intent를 만들어 중복 실행하지 못한다. 정상 allowance가 이미 충분한 경우에는 추가 approve 없이 새 action 검토가 가능하되 현 spender/금액/권한을 확인한다.

permit은 해당 토큰/표준의 명시적 profile이 선택됐을 때만 위 준비 경로를 사용한다. 서명된 permit이 단독 제출되거나 잔여 권한으로 남는 가능성, 실제 nonce/기한/소비·실패 후 잔존 allowance를 추적한다. 아직 permit adapter가 없으면 unsupported이며 일반 approve 경로로 자동 전환해 추가승인을 생략하지 않는다.

현재 market_action은 approval-source-dispatch에서 adapter_pending_execution_blocked다. API057/058/061이 존재해도 전체 payload decoder·source별 승인/서명/제출·typed result reader가 등록되기 전 실행은 미지원이다. 지원하려고 API017 개인송금이나 generic hash sign으로 우회하지 않는다. NFT형 position 권한/승인이나 token permit도 선택 protocol별 별도 adapter다.

## 4. 가격 유효성과 행위별 제한

PriceSnapshot 후보: marketId, sourceId/version, priceRole(index/mark/execution), base/quote asset, value/scale, observedAt, source timestamp/block/hash, validity, reason, policyRevision. 현재 quote와 chain state를 동일 시점이라고 추정하지 않는다. stale/invalid/missing/deviation/unknown을 구분하고 마지막 유효값은 시점과 함께 정보용으로만 표시한다. 독립 가격을 사용할지, 어떤 source 간 편차를 허용할지는 미선택이다.

| ID/행위 | 요구 가격 상태 | 미가용 시 동작 |
|---|---|---|
| PA-01 swap/FX quote·신규실행 | valid | 새 견적/서명 보류, 명시적 새 검증 source 없이는 fallback 가격 금지 |
| PA-02 유동성 추가 | valid | 토큰별 최소치/range·가격 검증 전 보류 |
| PA-03 유동성 회수/collect | profile_specific | 회수가 가격위험과 무관하다고 가정하지 않음; exact outputs/minima/profile 검증 가능 시만 신규승인 |
| PA-04 Perp 신규/증액 | valid | 신규 위험 증가 차단 |
| PA-05 Perp 담보 인출 | valid_and_current_margin | health 계산 불가면 차단; stale cash와 available margin 혼동 금지 |
| PA-06 Perp 담보 추가 | profile_specific | 허용 asset/귀속·새입금 및 protocol 안전성 검증 시만 허용; 자동 안전 보장 없음 |
| PA-07 Perp 축소/종료 | valid_or_explicit_emergency_profile | reduce-only가 가격오류를 없애지 않음; 비상가격/청산 충돌 규칙 미선정이면 보류·사유 표시 |
| PA-08 펀딩 계산/반영 | funding_window_evidence | 유효한 전체 기간/자료 없으면 근거없는 rate0/고정가격으로 덮지 않음; gap/pending 표시 |
| PA-09 청산 실행 | valid_liquidation_evidence | 오래된 가격/position으로 실행금지; 별도 비상 profile 없으면 검토필요 |
| PA-10 상태조회/원거래 관측 | read_authority | 시점·stale/unknown 유지; 신규 위험행위 차단 중에도 원실행 관측 지속 |

이 표의 허용 가능 문구는 선택된 profile·현재 권한·실제 contract의 안전 조건을 모두 만족할 때만 해당한다. 지금 미선택 상태에서는 실행 허용이 아니다. 시장의 normal/reduce_only/paused/recovery mode와 가격상태를 별도 표시하고, reduce-only도 무조건 안전한 청산/출구라고 보장하지 않는다. 이미 가능한 외부 실행·청산을 앱/서비스 hold가 중지시켰다고 표시하지 않는다.

## 5. 금액과 산술 예제

모든 자산 금액은 asset identity와 atomic 정수, 가격은 base/quote와 정수 비율 또는 명시 scale을 사용한다. 서로 다른 자산 수량을 더하거나 float로 환산하지 않는다. fee/slippage/price impact는 각각 표시하며 이미 반영된 spread/fee를 두 번 차감하지 않는다. quote 유효기한과 실제 onchain deadline은 선택 router/engine에서 강제되는지 확인한다.

다음은 **단위/부호 검토용 합성 예제**다. 실제 환율·수익률·레버리지·프로토콜 수식/파라미터의 선택이 아니다. 반올림은 이 예제의 보수적인 제한값 후보이며 실제 calldata/contract 단위와 검증 후 채택한다.

| 예제 | 입력/규칙 | 기대 결과 |
|---|---|---|
| ME-01 exact-input 최소수취 | quote=1001 atomic, 허용편차=50 bps; ceil(quote×(10000−bps)/10000) | 996 atomic |
| ME-02 exact-output 최대지출 | quote=1001 atomic, 허용편차=50 bps; floor(quote×(10000+bps)/10000) | 1006 atomic |
| ME-03 FX 단위 | 2,000,000 atomic(6자리), 가상 rate=1350 quote/base, 출력18자리; floor(amount×rateNum×10^out/(rateDen×10^in)) | 2,700,000,000,000,000,000,000 atomic |
| ME-04 선형 long 부호 예제 | 수량2, 진입100, 평가110; PnL=2×(110−100)=20. 담보100+PnL20−fee2+fundingCashflow(-1) | equity117 (모두 가상 공통 quote 단위) |

ME04는 isolated linear long을 설명하기 위한 단순 산술이고 실제 equity/출금가능액·청산가격 계산기는 아니다. cross/inverse·담보 haircuts·pending fee·funding accrual·유지증거금·보험/부채가 들어가면 다른 계산 계약이 필요하다. fundingCashflow는 사용자 입장에서 수취 +, 지급 −로 표기하자는 UI 후보이며 engine sign convention과 변환을 검증한다. 미정 청산 임계값을 넣어 자동청산을 허용하지 않는다.

tiny amount의 보호 한도가 0이면 자동으로 보호를 끄지 않는다. 지원 token이 fee-on-transfer/rebasing 등 특수동작이면 실제 수취/잔고 변화·오차 정책을 명시한 profile이 필요하다. LP output minima는 각 token별로 검사하며 전체 환산합계 하나로 대체하지 않는다.

## 6. Perpetual 상태·경합

포지션은 market/account/subaccount/direction(또는 netting 방식)/positionId와 revision으로 식별한다. 주문/intent는 별도이며 pending order를 제공하는 engine에서만 접수·부분체결·취소 모델을 사용한다. 로컬 취소는 onchain order 취소가 아니다. close/reduce-only는 실행 당시 남은 position과 비교해 반대 방향 신규개설을 막아야 하며, 초과량 reject 또는 cap 선택은 profile에 둔다.

담보 deposit/withdraw, position increase/reduce, funding accrual/settlement, liquidation의 상태를 한 enum으로 섞지 않는다. 현재상태 snapshot과 원 execution별 결과를 보존한다. close와 liquidation이 경합하면 contract의 현재 position/precondition으로 최종 결과를 검증하고 하나가 성공했다는 이유로 다른 노출을 사라졌다고 보지 않는다.

청산가격·가용담보·미실현 손익은 값과 사용 가격/시각/정책을 함께 보여준다. 가격/펀딩 자료가 없으면 null/unknown이고 0이 아니다. 잔여 부채가 있다면 deficit와 선택되지 않은 처리정책을 표시하며 원장을 0으로 맞추지 않는다. funding 기간/집계 cursor와 position별 반영 cursor를 구분하고 gap을 단순 현재 rate로 소급 채우지 않는다.

## 7. 펀딩·청산 keeper

| ID | 단계 | 규칙 | 한계 |
|---|---|---|---|
| KP-M01 | trigger | market/policy/priceWindow/position snapshot으로 후보 관측 | candidate는 청산/펀딩 발생 사실이 아님 |
| KP-M02 | reserve | purpose+market+position/period+policy의 stable work key, lease/fence 예약 | lease만으로 중복 온체인 실행 방지 보장 금지 |
| KP-M03 | preflight | 현재 price validity·position/risk revision·funding cursor·권한 재검사 | 조건 불일치면 no_action/stale 후보; 현재 데이터로 조용히 다른 행위 서명 금지 |
| KP-M04 | submit | 정확한 payload/hash/nonce와 exposure 영속화 후 제출 | 서버 fence를 외부서명/tx 회수로 오인하지 않음; contract-side period/cursor/precondition 중복방지 필요 |
| KP-M05 | observe | 개별 tx/UserOp·정확한 funding/liquidation event와 새 position 검증 | 모호하면 pending/unknown, keeper 성공로그로 사용자 포지션 변경 금지 |
| KP-M06 | reconcile | canonical 변경/경합 close에 따라 원 effect identity를 보정 | 같은 funding period 재적용/이중청산·허위정산 금지; reorg 후 원증거·현재 engine 상태 대조 |

서버 작업의 멱등 key는 onchain 중복방지와 다르다. old keeper가 서명/전송 가능한 상태에서 lease가 만료될 수 있으므로 contract의 funding period/index·position revision/조건으로 중복효과를 막는지 검증한다. 선택 engine에 그런 경계가 없다면 keeper 병렬 실행을 안전하다고 선언하지 않고 별도 계약 보강을 남긴다. keeper 운영자의 임의가격/forced success가 사용자 지갑 동의나 청산정책을 대신하지 않는다.

## 8. 정책 입력과 계약 보강

| ID | 결정 | 주제 | 필요한 선택 |
|---|---|---|---|
| MD-01 | D12 | 자산쌍·DEX/LP 모델 | pool/router/fee/range·토큰 특수동작·실제유동성 |
| MD-02 | D12 | FX 모델 | 현물 또는 파생·base/quote·가격/환산·수수료 |
| MD-03 | D12 | 가격·슬리피지/체결 | quote source·decimals·기한·최소/최대·부분체결·refund |
| MD-04 | D13 | Perp 계정/담보 | isolated/cross·linear/inverse·담보자산·position/netting |
| MD-05 | D13 | 위험/가격 장애 | index/mark/execution source·유효기준·margin/레버리지·중단/비상정책 |
| MD-06 | D13 | 펀딩 | rate/index·sign convention·기간경계·취득가격·누락/backfill·상한 |
| MD-07 | D13 | 청산/부채 | trigger·부분/전액·수수료·잔여담보·보험/미회수부채·keeper 권한 |
| MD-08 | D19 | 운영/관측 | 확정·worker 재시도·fence/lease·price/Indexing 지연·수치기준 |

위 8개 selection은 모두 null이다. 기존 19개 결정도 open이며 사용자 선택으로 기록하지 않는다.

| 변경 | 기존 API | 주제 | 필요한 보강 |
|---|---|---|---|
| MG-01 | API-057, API-058 | 액션별 quote/intent | swap exact in/out, LP add/remove/collect, FX model별 discriminated schema; quote/amountIn fields만으로 전부 실행금지 |
| MG-02 | API-058, API-061 | market_action 승인·제출·결과 | 현재 source adapter 미등록. wallet/source/hash/allowance-child/execution-child ancestry와 typed reader 필요; API018 EOA fallback 금지 |
| MG-03 | API-059 | LP 조회 | fungible shares vs positionId/range/liquidity/fees owed·collectable/reorg 필드 분리 |
| MG-04 | API-060, API-061, API-062 | Perp 상태 | position·intent/order·collateral·risk snapshot·funding index·liquidation 개별 상태/단위 및 revision |
| MG-05 | API-061 | 주문/취소·reduce-only | engine가 실제 pending order를 지원할 때만 cancel ABI/경합/증거 명세. 로컬 intent 취소와 구분; closing이 반대포지션 개설 금지 |
| MG-06 | API-060, API-062 | 가격·keeper 운영 | price provenance/invalid reason·period cursor·중복방지contract·exposure/재처리. 기존 O03 원천관측과 운영권한 분리 |

API/화면/schema·SQL은 수정하지 않았다. 새 market profile, operation ancestry, price/risk/keeper registry·typed result와 source adapter를 채택해야 한다. 시장 데이터의 공개성은 사적 지갑/포지션 업무권한을 부여하지 않는다. DS04 manifest/version/reorg와 DS02 signer epoch/권한을 그대로 연결한다.

## 9. 개발 단계 수용 사례

아래 32개는 모두 not_run이고 실기·contract·MPC·keeper·거래를 실행하지 않았다. 4개 산술 예제의 로컬 계산 확인은 실제 상품 안전성/체결 검증과 구분한다. 제품별 정상 대조군과 거절/경합/재구성 사례가 모두 필요하다.

| 사례 | 상황 | 기대 결과 |
|---|---|---|
| MK-T01 | 가짜symbol/decimals | manifest 불일치 거절 |
| MK-T02 | 견적중 route/spender 변경 | 새검토 없이는 실행 금지 |
| MK-T03 | allowance만 성공 | 승인완료·상품미실행 표시 |
| MK-T04 | allowance후quote만료 | 잔존승인 표시·새견적 재검토 |
| MK-T05 | 최소수취미달 | 선정 contract 한도거절/실제결과 확인 |
| MK-T06 | tiny amount 최소수취0 | 0보호를 묵시허용하지 않음; 명시unsupported/재입력 |
| MK-T07 | exact-output 잔여입력 | 실제사용/미사용입력 반환과 recipient 증거 확인 |
| MK-T08 | fee-on-transfer/rebase token | 지원profile 없으면 거절; Transfer face amount만 실수취로 쓰지 않음 |
| MK-T09 | 범위LP shareAmount 혼동 | position/range/liquidity별 schema검증 |
| MK-T10 | LP remove 성공 collect 실패 | 별도 tx/실패허용 호출은 partial; 원자적 multicall 전체 revert는 양쪽 효과 rollback. 선택 ABI/receipt로 판정, 중복 remove 금지 |
| MK-T11 | FX 통화방향 역전 | base/quote scale 검증·새견적 |
| MK-T12 | FX spread/fee 중복차감 | quote breakdown과 actual 일치, 이중차감 금지 |
| MK-T13 | 고정 V2 reserve/Perp price | 실제견적/가격으로 채택 금지 |
| MK-T14 | 가격 stale 중 신규증액 | 차단·가격오류표시 |
| MK-T15 | 가격 stale 중 담보인출 | 위험검증 불가면 차단 |
| MK-T16 | 가격 stale 중 reduce-only | 비상profile 없으면 성공보장금지/보류 |
| MK-T17 | 동시close와liquidation | 현재onchain position/precondition으로 판정·이중종료금지 |
| MK-T18 | reduce-only 크기가 남은position 초과 | 반대방향 position 개설금지; 선정 reject/cap profile 필요 |
| MK-T19 | cross vs isolated 혼합 | risk scope/담보범위 불일치 거절 |
| MK-T20 | unrealized PnL 표시 | 가용출금액과 구분·가격시점/부호표시 |
| MK-T21 | funding rate 0과 가격자료누락 | 0과unknown 구분·gap보류 |
| MK-T22 | 동일funding period keeper경쟁 | contract cursor/precondition으로 한번적용 |
| MK-T23 | lease만료후oldkeeper전송 | 기존노출추적·contract중복방지; lease회수로 tx무효주장금지 |
| MK-T24 | funding 반영후reorg | effect보정·cursor정합성 확인, 중복차감금지 |
| MK-T25 | 청산후담보부족 | 미회수부채/정책보류 표시, 0으로 강제정리금지 |
| MK-T26 | 서명후앱종료 | 원tx/UserOp조회, 새서명자동실행금지 |
| MK-T27 | 같은bundle 부분실패 | 자식별실제결과로 partial/failed 판정 |
| MK-T28 | quote→sign 중binding/epoch철회 | 현source/authority재검사 차단 |
| MK-T29 | unknown 결과후새key재시도 | 원노출대사 전 중복position/swap 금지 |
| MK-T30 | 시장중단후원결과관측 | 조회/보정 유지·신규액션정책분리 |
| MK-T31 | 시장adapter미등록 | unsupported; personal_transfer 우회금지 |
| MK-T32 | 정상swap/LP/FX/Perp 흐름 | 각 선택profile의 정상·거절·복구 증거까지 있어야 완료 |

연결 작업: DEX-01, DEX-02, DEX-03, FX-01, FX-02, FX-03, PERP-01, PERP-02, PERP-03, PERP-04, PERP-05, PERP-06. 개인 역할/공수는 배정하지 않았고 기존 초기 EOA/고객 native gas/USDC 더미 시연·후속 AA/후원 순서를 유지한다.

**다음은 DS-06 DID·STO·x402 자격/지급 계약**이다. 발급·검증·철회, 시험 발행물의 자격/권리, 유료자원 결제와 제공 결과를 연결한다. DS05의 상품/가격/위험 규칙과 6개 계약 보강은 채택 전 항목으로 남긴다.

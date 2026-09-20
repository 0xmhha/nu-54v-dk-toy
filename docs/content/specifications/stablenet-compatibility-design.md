# StableNet 자산·스마트계정·Indexer 호환 설계

2026-09-19 · **DS-04 설계 후보. 구현·배포·트랜잭션·실기 시험은 수행하지 않았다.** 로컬 재사용 조사와 표준 원문을 연결했다. RPC/Indexer/저장소 최신 HEAD를 이번에 다시 확인하지 않았으며 이전 관측을 현재 서비스 가용성으로 해석하지 않는다.

[구조화 원본](stablenet-compatibility-design.json) · [검증기](validate_stablenet_compatibility.py) · [네트워크 과거 관측](../stablenet-testnet-baseline.md) · [기존 Indexer 조사](../indexer-integration-and-test-token.md)

## 1. 무엇을 맞춰야 하는가

같은 체인 ID나 토큰 이름만으로 호환을 판정하지 않는다. 환경·배포코드·ABI·서명/제출 버전·decoder·조회 모델을 하나의 검증 가능한 조합으로 연결한다. 초기에는 NU/Cloud EOA와 더미 토큰, 고객 native gas 경로를 연결하고 후속 스마트계정·운영자 후원은 별도 조합으로 검증한다. 사용자 요청의 전체 9개 온체인 영역은 모두 유지한다.

사용자 지정 RPC는 `https://api.test.stablenet.network/`, 탐색기는 `https://explorer.stablenet.network/`다. 8283은 2026-09-17 기록의 관측값이다. 이 문서의 liveVerifiedThisTurn=false와 실제 endpoint/배포 pins=null은 미검증을 뜻하며 null끼리 일치한다고 호환으로 보지 않는다.

## 2. 자산 식별과 표시

| 자산 | 종류 | 현재 상태 | 검증 조건 |
|---|---|---|---|
| native_gas · 사용자가 지정한 네이티브 가스 자산 | native | 주소/decimals 미선정 | 기존 SDK의 WKRC/18 표시는 스냅샷 근거; 현재 네트워크 metadata/기능으로 재검증 필요 |
| dummy_usdc · 시연용 더미 USDC | erc20 | 주소/decimals 미선정 | 6 decimals·MockUSDC 이름은 제안만; 공동시연 mint/burn 권한 검증, 실제 USDC 동등성 주장 금지 |
| target_usdc · 후속 실제 목표 USDC | erc20 | 주소/decimals 미선정 | StableNet 지원/공식 주소/관리 기능/자산운영 조건 미확인. 배포 존재를 가정하지 않음 |
| wrapped_native · WKRC wrapped 후보 | erc20 | 주소/decimals 미선정 | native와 별도 contract/balance; wrap/unwrap 코드·이벤트·가스 조달 동작 검증 |

native 자산의 identity는 environment+native이고 ERC20은 environment+chain+contract다. 주소 없음은 native에만 해당하며 미설정 ERC20 주소를 native로 해석하지 않는다. symbol/decimals는 표시 metadata와 검증 version으로 보관한다. token atomic 금액을 float로 저장하지 않는다. ERC20 name/symbol/decimals는 표준에서 선택 기능이고 Transfer에는 주문ID가 없으므로 metadata/주문 귀속을 별도 검증한다. [ERC-20](https://eips.ethereum.org/EIPS/eip-20)

더미 토큰은 6 decimals 제안만 유지하고 발행/소각 권한을 제한한 시연용으로 설계한다. 실제 USDC의 permit·authorization·관리동작까지 동일하다고 보지 않는다. mint는 native gas 조달을 대신하지 않는다. 실제 USDC의 해당 체인 지원/공식 주소와 운용 조건은 미확인이다. 이를 조회 실패 시 더미 토큰으로 자동 대체하거나 실제 자산처럼 표시하지 않는다.

## 3. Manifest 여덟 경계

아래는 계약 필드 후보이며 기존 compatibility-manifest.schema를 변경하지 않는다. 모든 values=null, compatibility=unverified, evidenceRefs=[]다. 논리 manifest를 작성한 것과 서명된 runtime registry를 구축한 것은 다르다.

| ID | 경계 | 필요한 필드 | 판정 조건 |
|---|---|---|---|
| CM-01 | 네트워크 신원 | environmentId, chainId, genesisHash, checkpointNumber, checkpointHash, rpcIdentity, forkFeatureProfile | chainId만 일치하면 통과하지 않음; RPC/Indexer/탐색기의 같은 높이 canonical hash와 환경을 확인 |
| CM-02 | 자산 | assetKind, contractAddress, codeHash, proxyImplementation, abiDigest, decimals, mintBurnPolicy, symbolLabel | native는 ERC20 주소 없음; token symbol만으로 식별 금지. proxy 구현/권한 변경 관측 필요 |
| CM-03 | 계약 배포 | sourceCommit, compilerVersion, evmTarget, buildSettingsDigest, libraryLinks, dependencyCommits, runtimeCodeHash, deploymentBlockHash, deploymentAddress | 로컬 파일/배포 import 경로·submodule/실행 bytecode 일치. 생성/링크/immutable 차이는 검증된 mapping으로 대조 |
| CM-04 | 스마트계정 | accountModel, accountAddress, factoryAddress, implementationHash, entryPointAddress, entryPointCodeHash, entryPointVersion, validatorVersion, moduleConfigDigest | 지원 version/검증정책/초기화/주소 예측/실제코드 확인. EOA→AA 주소 동일 자동보장 금지 |
| CM-05 | 서명·제출 | signerProfile, reviewDecoderDigest, userOpCodecVersion, hashDomainVersion, sdkCommit, bundlerIdentity, bundlerEntryPoints, gasMode, paymasterEnvelopeVersion | curve/domain/encoding/version 정확히 일치; EOA 서명을 UserOp 승인으로 재사용 금지 |
| CM-06 | Indexer·decoder | indexerCommit, schemaDigest, deploymentId, abiDigest, decoderVersion, effectivePosition, rpcIdentity, backfillRange, completenessPolicy | blockNumber뿐 아니라 tx/log 순서·upgrade 효과시점 필요; 누락 구간은 complete 아님 |
| CM-07 | 조회·소비자 | apiSchemaVersion, projectionGeneration, authorityEpoch, sourceManifestVersion, cursorCodecVersion, ownershipPolicyVersion | 체인관측과 업무/개인정보 권한분리. 과거cursor/권한cache만으로 공개 금지 |
| CM-08 | 호환 증거 | producerBuild, consumerBuild, testCaseRefs, evidenceDigests, observedAt, revocationRevision | 정확한 조합에서 정상/실패·복구 증거 필요. null 동등/서버 응답200은 호환 증거 아님 |

manifest는 원자료 checksum뿐 아니라 변경 권한·승인·서명키·철회·유효범위를 확인해야 한다. 게시된 조합은 immutable revision으로 보관하고 바뀐 배포/ABI는 새 revision으로 연결한다. 허용 목록에 등록했다는 이유로 새로운 정책·서명권을 만들지 않는다. 현재 gate와 source provenance는 매 동작에서 별도 검사한다.

최신 규격 전체를 기존 배포에 소급 적용하지 않는다. 실제 EntryPoint 코드·SDK packing/hash domain·validator·Bundler 지원 버전을 고정해 검증한다. 기존 조사에 있는 Paymaster envelope 차이, @kernel/@account-abstraction 배포 import와 local 구현 차이, Prague EVM target 지원 여부는 그대로 검증 항목이다.

## 4. EOA에서 스마트계정으로

ERC-4337은 UserOperation을 Bundler가 묶어 EntryPoint 호출로 처리하는 구조다. signature의 구체 검증은 계정 구현에 달려 있으며 chain/EntryPoint 결합과 버전 호환을 확인해야 한다. 따라서 한 bundle transaction 성공을 모든 UserOp 성공으로 취급하지 않는다. [ERC-4337](https://eips.ethereum.org/EIPS/eip-4337)

| 비교 경로 | 주소/통제 | 전환 시 요구 | 미선정 결과 |
|---|---|---|---|
| 별도 contract account 생성 | 기존 EOA와 새 account 분리; NU 또는 Cloud signer를 검증정책에 연결 | factory/초기화/validator/주소 검증과 선택 자산 이동, 기존 allowance 유지/철회·새주소 신규승인 및 권한변경 | 기존 Kernel 재사용 후보. 모델 확정 아님 |
| EIP-7702 지원 체인의 위임 | 기존 EOA에 위임 코드 지정 가능; 기존 키의 권한이 사라지는 것은 아님 | 체인 지원·authorization domain/nonce·대상 code/초기화·지속 효과를 검토 | 지원 미확인. 같은 주소 유지 요구를 임의 확정하지 않음 |

EIP-7702는 EOA 코드 위임을 다루며 체인 기능과 별도의 authorization 검증이 필요하다. 위임 변경/제거가 과거 자산 이동이나 외부에 노출된 권한의 모든 효과를 소급 취소하지는 않는다. 이 체인의 지원 여부는 확인하지 않았다. [EIP-7702](https://eips.ethereum.org/EIPS/eip-7702)

일반 ERC20 allowance는 owner/spender 관계이므로 별도주소 account로 자동 이전하지 않는다. 기존 EOA allowance를 유지할지 철회할지와 새 account의 신규 allowance 승인을 각각 검토한다. EIP-7702 동일주소 경로에서는 기존 allowance가 남을 수 있어 별도 검토해야 한다.

가스 지불 방식도 모델 선택과 분리한다. 고객이 자기 native 자금/선정 EntryPoint 자금조달 규칙을 충족하는 AA와 Paymaster 후원은 다르다. 후원 envelope/예치/한도/수수료 토큰·반환규칙이 검증되기 전 무료 또는 토큰가스 지원을 표시하지 않는다. 실패 시 payer나 서명 내용을 자동 교체하지 않는다.

| ID | 전이 | 조건 | 기록·복구 |
|---|---|---|---|
| AA-01 | eoa_active → plan_ready | 선택된 account model, 원 signer/지갑·목표주소/validator/가스·자산/권한 차이 검토 | planDigest와 원 account/binding/source revision 고정, 잔여원거래·nonce 노출 목록 |
| AA-02 | plan_ready → authorized | 기기/Cloud의 모델별 명시 승인; current profile/주소/정책/가스 재검사 | EOA transfer, UserOp, 7702 authorization을 별도 목적/서명형식으로 승인 |
| AA-03 | authorized → execution_pending | 등록된 smart_account adapter와 exact EntryPoint/bundler/validator 조합 | operation/account/authorization/UserOp/tx별 추적. 제출unknown이면 같은결과 조회 |
| AA-04 | execution_pending → account_verified | 예측주소 아닌 실제 코드/초기화/validator·owner 권한 및 실행관측 검증 | 새계정 사용가능성과 자산 이전완료 분리; onchain 변경 전 서버 UI만 owner 바꾸지 않음 |
| AA-05 | account_verified → migration_pending | 선택 자산 이동과 기존 allowance 유지/철회·새주소 allowance 신규승인 및 권한변경을 각각 plan으로 검토/승인; 가스/잔여작업 재검토 | 원주소와 신규주소별 transfer/권한 결과 추적. import unrelated 자산 sweep 금지 |
| AA-06 | account_verified/migration_pending → ready | 선택 model의 정상승인/거절·잔고/권한/가스/미결과 확인. 자산이전 없음도 명시적 plan 결과; 현재 account/binding/config revision과 모든 제한 재검사 | 원 EOA 주소/과거영수증/진행거래 유지; 선택한 계획의 완료만 표시; expected plan/state revision CAS, 늦은 결과가 held 상태를 덮어쓰지 않음 |
| AA-07 | plan_ready/authorized/execution_pending/account_verified/migration_pending → held | 불일치/철회/reorg/만료/응답유실 | 원 단계/노출/commit 관측 후 재개. 원 EOA를 자동삭제하거나 새 plan 자동서명 금지 |

현재 approval-source-dispatch의 smart_account는 adapter_pending_execution_blocked다. 별도 source 생성·전체 검토 payload/권한·UserOp 제출·typed 결과 reader·실패복구 계약을 채택하기 전 personal_transfer/payment/refund EOA adapter로 우회하지 않는다. 이것은 범위 제외가 아니라 기존 SMART 작업의 채택 조건이다.

새 계정이 생겨도 과거 EOA 거래·영수증 소유·늦은 입금·미완료 환불은 원주소에서 계속 추적한다. 주소별 nonce/allowance/자산과 signer 권한을 구분한다. 원 주소 전액이전을 기본값으로 강제하지 않으며 기존 import 지갑의 unrelated 자산을 이동시키지 않는다. HW/Cloud signer 구성은 DS02 미선정 정책을 유지하고 폰/서버에 NU private key를 복제하지 않는다.

## 5. 아홉 영역의 관측·조회

| ID/영역 | 관측 입력 | 조회 모델 | 판정의 한계 |
|---|---|---|---|
| OD-01 USDC/더미 | 허용 token의 Transfer/Approval·receipt 및 token별 권한/동작 | 자산잔고·전송·지급관측 | ERC20 Transfer에 주문ID 없음; 원 intent와 정확한 emitter/from/to/atomic amount/log provenance 확인 |
| OD-02 WKRC/native/wrapped | native 거래/잔고·필요 시 trace, wrapped의 검증된 ABI 이벤트 | native 가스·wrapped 잔고/입출금 | native 비용에 ERC20 Transfer를 요구하지 않음; trace 없는 내부 native 이동은 완전하다고 주장 금지 |
| OD-03 DeFi | 선정 pool/router의 swap·유동성·수수료 이벤트와 시점별 상태 | 시장/견적/포지션·실행이력 | 정확한 상품·ABI 미정. Transfer만으로 swap 가격/성공/포지션을 확정하지 않음 |
| OD-04 스마트계정 | EntryPoint/UserOp·계정/모듈/validator 변경과 실행근거 | 개별 UserOp 결과·계정권한 이력 | bundle receipt와 UserOp 성공 구분; UserOp 안의 지급/내부실패도 별도 확인 |
| OD-05 FX | 선정 통화쌍 spot 또는 파생상품 ABI/가격원 | FX quote/체결·포지션 | 상품 모델·가격/환산규칙은 DS05 미정; DeFi 이벤트를 이름만 FX로 변경 금지 |
| OD-06 Perpetual | 선정 엔진 position/margin/funding/liquidation·가격근거 | 포지션·증거금·펀딩·청산 이력 | 고정가격/TODO 코드를 완료로 인정하지 않음; offchain keeper 시도와 onchain 실행 구분 |
| OD-07 STO | 선정 발행물의 발행/전송/제한·권리변경 | 보유/이전제약·권리 상태 | 토큰 전송만으로 법적 권리·자격 확정 금지; DS06 시험상품 기준 필요 |
| OD-08 DID | 선정 registry/credential status의 발급·철회 관측과 offchain 검증자료 | 자격상태·제시 검증 참조 | 개인 credential 원문 onchain/공개Indexer에 저장 금지; 모든 DID가 chain event를 갖는다고 가정하지 않음 |
| OD-09 x402 | 선정 결제 방식 settlement 증거와 HTTP/facilitator 결과 | 유료자원 지불·제공 상태 | 자원제공 성공은 chain receipt만으로 알 수 없음; protocol version/지원자산·실제 지급 주체 별도 |

관측 후보는 배포된 event signature 목록이 아니다. DS05/06에서 상품/표준·컨트랙트가 정해지면 실제 ABI의 event topic·필드·state reader를 매핑한다. 각 영역에 chain event가 반드시 존재한다고 가정하지 않고 offchain 근거와 현재 검증/철회 권위를 함께 연결한다.

## 6. Indexer 처리와 reorg

| ID | 단계 | 규칙 | 출력·지속성 |
|---|---|---|---|
| IX-01 | 환경/배포 확인 | CM01/03의 신원·checkpoint/code/권한 검증 후 source 등록. 체인 mismatch면 affected ingestion/새승인 hold | 검증된 environment/deployment revision |
| IX-02 | 원시 관측 보존 | block hash/parent·tx receipt/log와 원천 range를 보존. subscription은 알림이며 canonical range 재조회가 기준 | observationId=(environment,blockHash,txHash,logIndex), native/trace는 별도 typed key |
| IX-03 | 누락/재구성 탐지 | cursor의 hash ancestry 대조, 공통조상까지 검증 후 새 branch 읽기. finality 가정 밖 재구성은 격리·전체영향검토 | contiguous complete range; highest block만으로 checkpoint 전진 금지 |
| IX-04 | 버전별 decoding | 배포주소/code/proxy upgrade position에 맞는 ABI/decoder 적용. 같은 block의 upgrade 전후 순서 불명하면 해당범위 hold | immutable decode version + 원 raw digest + decoded/unsupported/invalid 상태 |
| IX-05 | 실행 귀속 | EOA tx와 UserOp를 구분하고 expected payload/실제 log를 결합. 동일 bundle 내 매칭 모호하면 hold | source execution ref와 exact evidence mapping; timestamp/amount만으로 연결 금지 |
| IX-06 | 업무 대사 | 현재 canonicality/confirmation/원 policy와 원 주문·환불 provenance 검사; shared gate와 funding hold 원자변경 | 기존 allocation/effect identity 유지, observation revision 한번 적용; chain 이벤트가 주문승인을 대체하지 않음 |
| IX-07 | 조회 projection | source cut/authorityEpoch/completeness/ownership를 확인해 consumer별 기여분과 generation 반영 | 앱 asOf/freshness/held와 cursor; stale/unknown을 0/failed로 변환 금지 |
| IX-08 | decoder 교체·재처리 | 새 decoder로 shadow build/diff→검토→현재권한/source/generation/fence 재확인→단일consumer atomic publish | 기존 CP-B06 준수; raw 이력/원마감 보존, 재처리로 신규지급/보상/환불 생성 금지 |

### 실행 단위와 관측 단위를 구분

- EOA의 제출 identity는 environment/chain/txHash이며, 개별 token 증거는 허용 emitter와 정확한 receipt log에 연결한다. raw 관측은 blockHash를 포함하므로 reorg branch를 구분한다.
- 같은 tx가 다른 block에 재포함되면 global logIndex가 바뀔 수 있다. receipt 내부 상대 위치·실제 payload/decoded 의미·원 execution provenance를 다시 대조한다. transactionHash+logIndex만으로 새 지급을 만들거나 이전 지급과 자동 동일시하지 않는다. 업무 paymentId/effect identity와 observation revision은 분리한다.
- UserOp identity는 environment/chain/EntryPoint/version/hash에 결합한다. bundle txHash는 컨테이너다. 해당 UserOp event success와 같은 실행에 귀속된 token transfer를 검증하며 같은 tx의 인접 log/같은 금액만으로 연결하지 않는다. 선정 EntryPoint의 실행 경계/trace 또는 검증된 decoder 증거가 없으면 ambiguous/held다. 계정이 내부 실패를 흡수할 수 있으므로 UserOp success만으로 지급 완료를 추정하지 않는다.
- 과거 log의 removed 알림 유무만 신뢰하지 않는다. hash ancestry·정규 block 범위와 누락/페이지 완전성을 확인한다. finality/깊이 정책은 미선정이며 latest 높이를 확정성으로 바꾸지 않는다.

Indexer checkpoint와 business consumer checkpoint는 서로 다르다. 백필 범위/decoder가 바뀌면 새 projection generation으로 계산하되 원천 지급·환불·entitlement identity를 새로 만들지 않는다. 원지급 invalidation과 새로운 환불 승인 차단은 기존 공유 gate/원장 원자 경계를 따른다. consumer가 느리더라도 현재 source deny를 우회하여 혜택을 사용하지 않는다.

기존 탐색기의 일부 전송 응답은 logIndex/blockHash/removed 정보를 생략한다는 조사 기록이 있다. 해당 build를 대사 증거로 쓸 때는 raw receipt/log 경로 또는 검증된 확장 DTO를 사용한다. 새 빌드에서 해결됐는지는 이번에 검증하지 않았다. Next.js 탐색기 UI를 RN 키오스크 구현 완료로 계산하지 않는다.

## 7. 조회·오류·개인정보 계약

읽기 응답 후보는 environmentId, sourceDeploymentRevision, observed block/hash, decoderVersion, projectionGeneration, authorityEpoch, asOf, freshness, completeness, execution/canonicality/confirmation, allowed public reason을 제공한다. cursor는 scope/filter/배포·schema generation에 결합하고 변하면 재조회한다. 응답 필드 이름/GraphQL schema/REST 변환은 기존 indexer-read-contract와 별도 채택 대상이다.

UNSUPPORTED_ABI/CHAIN_MISMATCH/DECODER_AMBIGUOUS/RANGE_INCOMPLETE/SOURCE_REORG/POLICY_UNAVAILABLE은 원인별 logical 상태 후보다. 공개 API code 등록 결과가 아니다. stale 데이터는 시점과 함께 보여주고 unknown을 잔고0/거래실패로 바꾸지 않는다. 온체인 공개 조회권은 개인 주문·대여·정밀위치·후기 소유권을 부여하지 않는다. 탐색기 링크에 사적 식별자·capability·민감 증거를 추가하지 않는다.

## 8. 실기·통합 수용 사례

모든 사례는 not_run이며 정상/실패·복구 대조군을 정확한 manifest tuple과 연결해야 한다. shape/reference 검증은 실행코드·서명·RPC 신원·Indexing correctness를 검증하지 않는다.

| ID | 상황 | 기대 결과 |
|---|---|---|
| SC-T01 | 같은 chainId 다른 genesis/checkpoint | environment mismatch 차단 |
| SC-T02 | 탐색기만 다른 RPC | 관측 불일치 표시·링크 동일성 미검증; 서비스 endpoint 추정 금지 |
| SC-T03 | 동명 토큰 다른 주소 | 다른 asset, 결제 허용목록 거절 |
| SC-T04 | wrapped 잔고만 충분 | native gas 부족 구분 |
| SC-T05 | 토큰 decimals 6→18 차이 | manifest conflict·금액재검토, 과거 atomic 해석 덮어쓰기 금지 |
| SC-T06 | 무권한 mint/burn mock | 공동시연 manifest 검증 실패; 테스트전용 제한 후 별도검증 |
| SC-T07 | proxy 같은 주소 구현 변경 | 새 code/ABI interval 검증, 미지원범위 hold |
| SC-T08 | 스크립트 @kernel과 로컬 Kernel 차이 | 실제 dependency build 검증 전 호환불가 |
| SC-T09 | Prague/7702 지원 불명 | 해당 opcode/계정경로 enable 금지 |
| SC-T10 | bundler EntryPoint 버전 불일치 | UserOp 제출 거절; legacy 자동하향 금지 |
| SC-T11 | paymaster envelope 불일치 | 후원미지원, 사용자 추가승인 없이 fee payer 변경 금지 |
| SC-T12 | 정상 EOA 토큰 지급 | 원 tx+exact log·현재 confirmation·업무정책으로 귀속/수락 |
| SC-T13 | receipt 성공·token Transfer 없음 | 지급미충족/검토; receipt만 paid 금지 |
| SC-T14 | 한 bundle 두 UserOp 한개 실패 | 개별success/증거로 판정, txstatus 전체성공 금지 |
| SC-T15 | 동일 bundle 동일금액 여러전송 | 검증된 UserOp 귀속 없으면 hold |
| SC-T16 | 다른 EntryPoint에서 같은 hash 주장 | domain/source mismatch 거절 |
| SC-T17 | 서명뒤 account module 변경 | 현validator/config 재검사·기존노출추적 |
| SC-T18 | AA 주소만 예측된 상태 | 설치/사용가능 완료로 표시 금지 |
| SC-T19 | 7702 위임 제거 후 과거효과 | 과거자산이동/allowance/서명소급취소 주장 금지 |
| SC-T20 | EOA→새계정 일부이전 실패 | partial/원주소잔고·gas 남김, 전체완료 금지 |
| SC-T21 | 구독 장기단절 | 해시커서/범위겹침 backfill·중복제거, replayLast만으로 완료 금지 |
| SC-T22 | 중복/역순 logs | 같은관측 version 한번반영·빈구간checkpoint 금지 |
| SC-T23 | reorg 후 동일 tx 재포함 | old 관측 invalidated·new관측 재검증, stable 업무source 중복효과 금지 |
| SC-T24 | 재실행된 tx의 logs 변경 | semantic provenance 재검사, 이전 logIndex를 무조건 동일지급으로 취급 금지 |
| SC-T25 | 같은 block 중 upgrade | tx/log 효과순서 불명 decoder hold |
| SC-T26 | 원입금 reorg 환불유효 | funding hold·signed 음수 차이/환불노출 보존 |
| SC-T27 | decoder 재처리후 stamp | 같은entitlement 보정, 새 grant 자동생성 없음 |
| SC-T28 | old backup/cursor 복원 | 최신authorityEpoch/fence/삭제기록 대조 전 공개/쓰기 hold |
| SC-T29 | Indexer lag | unknown/stale 표시, 결제실패/잔고0 추정 금지 |
| SC-T30 | frontend transfer 응답 logIndex 누락 | 결제대사 증거로 단독사용 금지 |
| SC-T31 | DID offchain 자격 | 발급/철회 검증 참조 별도, 없는 chain event 제조 금지 |
| SC-T32 | x402 지급성공 자원실패 | settlement와 delivery 상태 분리 |
| SC-T33 | public explorer 타인 주문정보 | 체인공개자료와 사적 주문/위치 ACL 분리 |
| SC-T34 | 정상 AA 계정+UserOp 조회 | 선택 exact tuple 정상/실패 증거와 source adapter 채택 후만 compatible |

## 9. 남은 채택과 다음 설계

manifest 서명/배포 registry·실제주소/ABI·smart_account source/DTO·decoder 업그레이드 위치·AA event 귀속 profile·확정/가스 정책·기존 Indexer schema 확장은 채택 전 남은 항목이다. 원 저장소 커밋은 재사용 조사 기준이지 현재 서비스 배포 증거가 아니다. 초기 chain/address/wallet balance를 실제 조회하거나 계약을 배포하지 않았다.

연결 작업: BASE-04, INDEX-01, INDEX-02, INDEX-03, INDEX-04, INDEX-05, INDEX-06, SMART-01, SMART-02, SMART-03, TOKEN-01, TOKEN-02, TOKEN-03. 15요구·104작업·320세부작업·19미결정과 개인 역할/공수 보류를 유지한다. 선택8개와 모든 배포 pins/evidence는 미기입이다.

**다음은 DS-05 DeFi·FX·Perpetual 상품별 상태/위험 계약**이다. 공통 자산/관측 경계를 사용해 견적·실행·유동성·증거금·펀딩·청산과 사용자 화면의 의미를 정리한다. 실제 상품 모델·가격원 선택과 구현은 별도다.

## 요구 범위와 변경 영향 구분 — LG-05

주요 선언 범위: [11, 12, 13]. 소속 작업에서 파생되는 영향 범위: [4, 8, 9, 10, 11, 12, 15].

기존 requirementRefs는 주요 선언 범위의 호환용 별칭이다. 전체 영향 분석이나 완료 판정에는 사용하지 않는다. transitiveTaskImpactRequirementRefs는 WBS 원 요구사항의 합집합이며 실제 실행 증거가 아니다.

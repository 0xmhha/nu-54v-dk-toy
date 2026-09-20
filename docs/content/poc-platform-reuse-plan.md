# poc-platform 재사용 검토

> 2026-09-18 범위 갱신: [12주 완료 범위 v3](twelve-week-completion-scope-v3.md)가 최신 기준이다. 아래 기존 검토에서 패스키·녹음·스마트 계정 등을 후보/후속으로 둔 부분은 새 범위를 따른다. 초기 EOA 개발 순서와 고객 가스 부담은 유지하며, 새 Cloud Wallet의 MPC는 별도로 설계한다.

작성: 2026-09-17. 사용자가 제공한 [0xmhha/poc-platform](https://github.com/0xmhha/poc-platform)의 `fcad7900de5ceea5e3ef586774609ad00739b760` 커밋을 읽기 전용으로 검토했다. 설치·빌드·테스트·서비스 실행·체인 배포는 수행하지 않았다. 기존 자산의 존재와 실제 통합 성공을 구분한다.

## 1. 이전 계획에서 달라진 점

**Bundler와 Paymaster 서버, 지갑 SDK 및 웹 화면이 이미 있는 것으로 확인했다.** 앞서 준비가 필요한 인프라로 분류한 항목 중 상당 부분을 기존 코드 기반으로 통합할 수 있다. 신규 개발 범위는 하드웨어 연결, React Native 앱, 카페 업무 모델과 기존 서비스의 통합에 더 집중할 수 있다.

이 저장소를 활용한다는 결정은 스마트 계정·가스비 후원·정기 결제 등 모든 기능을 첫 버전에 넣는다는 뜻이 아니다. 체인은 후속 답변으로 StableNet 8283 공개 테스트 RPC를 선택했다. 수수료는 사용자 결정에 따라 초기 고객 부담, 운영자 후원은 후속으로 구현한다.

## 2. 구성별 재사용 범위

아래 소스 링크는 검토 커밋에 고정한다.

| 기존 구성 | 실제 확인한 내용 | 활용 방향 |
|---|---|---|
| 웹 앱 | 결제 송수신·이력과 구독/가맹점 화면 경로 | 고객 지갑과 사장님 화면의 데이터 흐름·UI 참고. React Native 이식 범위 별도 판단 |
| 지갑 확장 프로그램·SDK | 브라우저 지갑과 UserOperation 구성·조회 코드 | 체인/계정/거래 로직을 선별 재사용. NU 보드 서명 경로는 별도 연결 |
| Bundler | UserOperation 검증·mempool 등록·조회·번들 실행 코드 | ERC-4337 선택 시 기존 서비스 통합 |
| Paymaster Proxy | 후원 요청 처리·서명·정책·예산 예약 및 결과 추적 | 후속 운영자 후원 또는 선택한 고객 토큰 수수료 방식에 통합 |
| contracts/config 패키지 | 체인별 주소·ABI와 배포 출력 변환 | 컨트랙트 배포 결과를 앱·서비스에 일관되게 전달 |
| PG·은행·온램프 시뮬레이터 | 시험용 결제·정산 모델 | 일부 상태/화면 설계 참고. 실제 카페 USDC 결제 완료의 근거로 사용하지 않음 |

구조 근거: [워크스페이스](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/pnpm-workspace.yaml), [웹 결제 화면](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/apps/web/app/payment/page.tsx), [지갑 SDK](https://github.com/0xmhha/poc-platform/tree/fcad7900de5ceea5e3ef586774609ad00739b760/packages/wallet-sdk/src).

## 3. AA 서버는 구현이 있으나 버전 정합성 확인이 우선

후속 [통합 작업 계획](payment-integration-workplan.md)에서 Indexer 조회도 추가 비교했다. SDK의 ERC-20 RPC 배열 요청/배열 응답 기대와 서버의 객체 요청/`{transfers,total}` 응답이 다르며, 거래 GraphQL의 상태·시간 필드도 맞춰야 한다. 기존 client의 존재는 확인했지만 그대로 연결되는 상태는 아니다.

Bundler의 `eth_sendUserOperation`은 실제 체인 ID로 해시를 계산하고 검증한 뒤 mempool에 등록한다. Paymaster Proxy는 요청에 서명하고, 설정된 경우 Bundler 결과를 조회해 후원 예산 예약을 확정/해제한다. 이때의 ‘settlement’는 후원 예산 처리이며 **카페 주문 매출·USDC 수취 대사와 다른 책임**이다.

근거: [Bundler 요청 처리](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/services/bundler/src/rpc/ethHandlers.ts#L132), [후원 결과 추적 연결](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/services/paymaster-proxy/src/app.ts#L102), [예약 처리](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/services/paymaster-proxy/src/settlement/settlementWorker.ts#L133).

**앞서 검토한 stable-poc-contract와 그대로 연결된다고 판단할 수 없는 구체적인 차이가 있다.**

- 이 플랫폼의 Paymaster 서명은 버전·종류·플래그·유효기간·nonce·payload를 포함한 envelope와 도메인 해시를 사용한다. [서명 코드](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/services/paymaster-proxy/src/signer/paymasterSigner.ts#L190), [인코딩](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/packages/sdk-ts/core/src/paymaster/paymasterDataCodec.ts#L47)
- 앞서 검토한 `stable-poc-contract`의 로컬 `VerifyingPaymaster`는 고정 오프셋의 유효기간과 65바이트 서명을 읽고 다른 방식으로 해시를 구성한다. 이 두 구현은 동일한 전송 형식이 아니다. 실제 배포 컨트랙트가 어느 구현인지 확인하고 서버·컨트랙트의 버전을 맞춰야 한다. [컨트랙트 파싱·검증](https://github.com/0xmhha/stable-poc-contract/blob/5d9d6550ef572bd86a036fe92c730e4ff0a1ca94/src/paymaster/VerifyingPaymaster.sol#L102)
- Bundler README는 v0.7을 설명하지만 실제 호출하는 SDK 기본 해시 함수는 v0.9 EIP-712 방식이다. README 이름만으로 EntryPoint 버전을 결정하지 않는다. 실제 배포된 EntryPoint의 해시 결과와 SDK 결과를 동일 입력으로 비교한다. [SDK 해시 함수](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/packages/sdk-ts/core/src/utils/userOperation.ts#L210)
- 주소 생성 스크립트의 기본 입력은 형제 디렉터리 `poc-contract/deployments`이며 환경변수/인자로 변경할 수 있다. 제공받은 저장소 이름이 다르므로 실제 배포 출력 위치와 대응 커밋을 명시해야 한다. [입력 경로](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/packages/contracts/scripts/generate-addresses.ts#L613)

이것은 정적 비교 결과다. 현재 가동 중인 서비스가 잘못되었다는 결론이나 전체 보안 감사가 아니다. 실제 배포 버전·주소는 아직 조회하지 않았다.

## 4. 시뮬레이터와 카페 업무를 구분

`pg-simulator`에는 결제·환불·체크아웃·가맹점별 정산 모델이 있다. 그러나 결제 데이터는 메모리 map에 저장하며 카드 처리 성공 여부 등에 시뮬레이션 로직을 사용한다. 이를 그대로 매장의 영속적 매출 원장이나 실제 USDC 수취 검증으로 사용하지 않는다. [저장 구조](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/services/pg-simulator/internal/service/payment.go#L87), [결제 모델](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/services/pg-simulator/internal/model/payment.go).

`order-router`는 토큰 스왑 경로·견적을 다루는 서비스다. 카페 메뉴 주문 라우터라는 의미가 아니다. [라우팅 모델](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/services/order-router/internal/model/types.go).

가맹점 UI도 이미 있으므로 ‘사장님 화면 전체 신규’라고 단정하지 않는다. 기존 구독 매출·플랜 화면의 재사용 가능성을 평가하되, 가게 ID 로그인·역할 권한·카페 메뉴·주문·환불·대여 상태를 충족하는지는 별도 확인한다.

## 5. 통합 책임 제안

| 영역 | 우선 활용 자산 | 남는 작업 |
|---|---|---|
| 체인 거래 구성·AA 제출 | poc-platform SDK·Bundler | 선택 버전 정합성, 하드웨어 외부 서명, 재시도·복구 연결 |
| 수수료 후원 후보 | poc-platform Proxy + 호환 Paymaster | 후원 주체·예산·정책 결정, 실제 예치·버전 검증 |
| 테스트 자산 | stable-poc-contract의 ERC-20 mock 기반 | 제한된 mint/burn, 배포·초기 발행·환경별 토큰 식별 |
| 거래 관측·운영 조사 | indexer-go / indexer-frontend | 주문·UserOperation·전송 이벤트 연결, 확정·누락 복구 |
| 고객·사장님 앱 | 기존 웹 화면·순수 TS 로직 참고 | React Native 화면/플랫폼 연동, 보드 BLE 승인·설정 |
| 카페 운영·대여 | 기존 데이터 모델·화면 일부 참고 | 영속 저장·매장 권한·메뉴·주문·매출 대사·환불·반납 |

기존 SDK/브라우저 키 저장소에 NU 고객 키를 복제하여 서명하는 것으로 하드웨어 연동을 대체하지 않는다. 가져오기 설정 경로와 일상 결제 서명 경로를 분리하고, 일상 지출 서명은 기존 결정대로 NU 기기가 담당한다.

### 앱·지갑 소스에서 확인한 구체적 접점

- 웹 앱은 Next.js/React, 확장 지갑은 Vite/React/Chrome 환경이다. React Native는 별도 플랫폼 계층이 필요하다. 기존 웹 콘솔과 SDK를 출발점으로 활용한다.
- 확장 지갑의 [keyring](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/apps/wallet-extension/src/background/keyring/index.ts#L122)에 신규 생성·니모닉/개인키 가져오기와 서명 분기가 있다. 현재 브라우저 vault 저장 방식을 폰에 그대로 이식하지 않고 입력 검증·UX와 키 보관 정책을 분리한다.
- [하드웨어 keyring 인터페이스](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/apps/wallet-extension/src/background/keyring/hardwareKeyring.ts#L41)는 메시지·typed data·거래·hash 서명 접점을 제공한다. 실제 기존 연결은 Ledger WebHID이므로 NU BLE transport와 암호화 import·기기 승인 경로는 추가 개발이다. transport 타입의 `bluetooth` 값만으로 구현 완료를 뜻하지 않는다.
- [provider 자동 탐지](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/packages/wallet-sdk/src/provider/detect.ts#L23)는 브라우저 `window`에 의존한다. RN에서는 브라우저 탐지 대신 provider/NU signer 주입을 검토한다. 전체 SDK의 RN 호환성을 시험한 것은 아니다.
- [구독 가맹점 화면](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/apps/web/components/merchant/MerchantDashboard.tsx#L167)은 통계·플랜 생성 기능이 있지만 Webhook/API key 일부는 localStorage를 쓰며 플랜 수정·활성화 전환은 미지원 안내다. 화면 존재를 운영 서버·점포 권한·공유 데이터 저장의 완료로 보지 않는다.
- [거래 이력 훅](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/apps/web/hooks/useTransactionHistory.ts#L52)은 실제 Indexer 조회를 사용한다. 다만 txHash별 전송 한 개로 축약하고 일부 ERC-20 항목을 `confirmed`로 표시하므로, 카페 지급 확정에는 원시 로그·receipt·확정 정책·주문 매칭을 별도로 적용한다.

## 6. 결정 상태

확정: poc-platform을 기존 개발 자산에 추가한다. 더미 토큰과 기존 Indexer 활용 결정을 유지한다.

제안: 신규 AA 서비스를 만들기 전에 기존 SDK·Bundler·Proxy의 호환 조합을 선정하고, 카페 결제에 필요한 최소 흐름만 연결한다.

확정: 초기 일반 지갑(EOA), 후속 스마트 계정 적용. 미정: 후속 계정 전환 방식, 가스 확보·후원 정책, 실제 운용 중인 저장소별 커밋/배포 주소. 체인과 네이티브 가스 지불은 후속 답변으로 확정했다. [네트워크 확인](stablenet-testnet-baseline.md)

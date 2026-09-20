# 12주 확장 범위 — 기존 코드와 추가 작업

2026-09-18 읽기 전용 검토. 기준은 stable-poc-contract `5d9d6550ef572bd86a036fe92c730e4ff0a1ca94`, poc-platform `fcad7900de5ceea5e3ef586774609ad00739b760`이다. 설치·빌드·테스트·배포는 실행하지 않았다. ‘미발견’은 이 커밋의 검사한 소스 경로에 한정하며 다른 저장소/서비스의 부재를 뜻하지 않는다.

[최신 15개 완료 범위](twelve-week-completion-scope-v3.md)를 위한 재사용 판단 자료다. 저장소의 기능 이름과 실제 제품 완료를 구분한다.

| 영역 | 소스에서 확인한 상태 | 새 완료 기준에 반영할 작업 |
|---|---|---|
| NU 펌웨어·FOTA·녹음·찾기 | 두 저장소에서 해당 보드의 구현 미발견 | 별도 펌웨어·하드웨어 작업으로 산정 |
| Google/Apple 로그인 | 검사한 apps/packages/services에서 사용자 소셜 로그인 구현 미발견 | 제공자 설정·계정 연결·세션·앱 콜백·탈퇴 흐름 구현/검증 |
| Cloud Wallet MPC | 사용자용 키 생성·임계 서명·복구 구현 미발견. 기존 mpc 경로는 브리지 서명 수집이며 실패 시 simulated signature 경로 존재 | 별도 MPC 도입/통합. 브리지 키 코드를 고객 Cloud Wallet 완료 증거로 쓰지 않음 |
| Passkey | SDK에 WebAuthn validator 어댑터 존재. signFn은 외부에서 제공 | NU 인증기 펌웨어와 대상 OS/서비스의 실제 등록·인증 구현 |
| DEX V3 | router/quoter 어댑터와 실제 quote RPC 호출 코드 존재 | 배포 pool·유동성·quote·슬리피지·swap 실행·결과 연결 |
| DEX V2 | reserve에 시험용 고정값을 쓰는 경로 존재 | 실제 유동성에 근거한 견적과 구분/수정 |
| 웹 swap | quote API와 UserOperation 실행 경로 있음. 조사한 화면은 필수 routerAddress 연결이 빠져 있음 | 환경 설정·실행 경로 연결. 초기 EOA 지갑 경로도 검토 |
| Perpetual | 엔진·증거금·포지션 소스 존재, 고정 가격·청산/펀딩 미완성 경로 있음 | 오라클·청산·펀딩·상태 변화와 실패 시나리오를 실제로 완성/검증 |
| FX | 전용 상품 구현 미발견 | 통화 쌍 spot swap인지 파생상품인지 지정하고 서비스 구성 |
| STO·DID·x402 | 해당 전용 구현 미발견. KYC·stealth registry 존재는 전체 프로토콜 구현 증거가 아님 | 표준·역할·발행/검증/지불 수명주기 정의 및 구현 |
| 여행자 추천·지도·후기 | 전용 여행 도메인 구현 미발견 | 장소·위치·후기·결제 출처 연결, 추천·챌린지·발도장 |

## 소스 근거

- [브리지 MPC 클라이언트](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/services/bridge-relayer/internal/mpc/signer.go#L42), [시뮬레이션 fallback](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/services/bridge-relayer/internal/executor/bridge_executor.go#L225).
- [WebAuthn validator 어댑터](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/packages/sdk-ts/plugins/webauthn/src/webauthnValidator.ts#L14).
- [DEX 계약 어댑터](https://github.com/0xmhha/stable-poc-contract/blob/5d9d6550ef572bd86a036fe92c730e4ff0a1ca94/src/defi/DEXIntegration.sol#L89), [V3 quote](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/services/order-router/internal/provider/uniswap_v3.go#L251), [V2 시험용 reserve](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/services/order-router/internal/provider/uniswap_v2.go#L84).
- [웹 swap 실행 훅](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/apps/web/hooks/useSwap.ts#L204), [호출 화면](https://github.com/0xmhha/poc-platform/blob/fcad7900de5ceea5e3ef586774609ad00739b760/apps/web/app/defi/swap/page.tsx#L19).
- [Perpetual 고정 가격](https://github.com/0xmhha/stable-poc-contract/blob/5d9d6550ef572bd86a036fe92c730e4ff0a1ca94/src/perpetual/PerpetualEngine.sol#L583), [청산 판정](https://github.com/0xmhha/stable-poc-contract/blob/5d9d6550ef572bd86a036fe92c730e4ff0a1ca94/src/perpetual/core/Liquidator.sol#L133), [청산 실행](https://github.com/0xmhha/stable-poc-contract/blob/5d9d6550ef572bd86a036fe92c730e4ff0a1ca94/src/perpetual/core/Liquidator.sol#L217), [펀딩 정산](https://github.com/0xmhha/stable-poc-contract/blob/5d9d6550ef572bd86a036fe92c730e4ff0a1ca94/src/perpetual/core/FundingRateManager.sol#L358).

이 검토는 전체 보안 감사나 최신 배포 서비스의 장애 판정이 아니다. 기존 구현의 시험 성공 여부·다른 브랜치·실제 배포 버전은 아직 확인하지 않았다.

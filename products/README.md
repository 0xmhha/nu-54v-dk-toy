# Products

이 디렉터리는 10개 제품을 독립된 작업 경계로 나눈다. 제품 README는 구현
진입점이며, 상세 설계 authority와 일정은 `docs/content/`에 유지한다.
2026-09-25 설계 동결 [DF-20260925-02](../docs/content/planning/design-freeze-checkpoint-02.md)에
따라 이번 12주 사이클에는 일곱 제품을 만들고, 나머지 셋은 설계만 둔다. P02 폰 앱은 2026-09-28 개정에서 추가했다.

| ID | 제품 폴더 | 실행 영역 | 이번 사이클 | 핵심 책임 | 설계 문서 |
|---|---|---|---|---|---|
| P01 | [device-firmware](p01-device-firmware/README.md) | Local | 만든다 | Zephyr 펌웨어, 가맹점을 검증하는 결제 서명, BLE, 셋업·TimeAnchor, 외부 SE | [p01](../docs/content/products/p01/plan.md) |
| P02 | [user-app](p02-user-app/README.md) | Local | 만든다 | 대여자 RN 폰 앱: 기기 본딩과 설정, 결제 확인 화면(기기에는 화면이 없다) | [p02](../docs/content/products/p02/plan.md) |
| P03 | [cloud-mpc-wallet](p03-cloud-mpc-wallet/README.md) | Cloud | 설계만 | DKG, threshold 서명, refresh, 복구 | - |
| P04 | [merchant-kiosk](p04-merchant-kiosk/README.md) | Local | 만든다 | RN 태블릿 키오스크, 주문, BLE central, 제출과 finalized 판정 | [p04](../docs/content/products/p04/plan.md) |
| P05 | [operations-backoffice](p05-operations-backoffice/README.md) | Local 도구 | 만든다(축소) | Go 운영 코어와 `opsctl`: 가맹점 등록, attestation, 예치, BLE 셋업·TimeAnchor·passkey, 반납 | [p05](../docs/content/products/p05/plan.md) |
| P06 | [stablenet-contracts](p06-stablenet-contracts/README.md) | Blockchain | 만든다 | 사전 예치 정산 컨트랙트와 가맹점 registry | [p06](../docs/content/products/p06/plan.md) |
| P07 | [indexer](p07-indexer/README.md) | Cloud | 만든다(최소) | PaymentSettled 영수증 조회 indexer | [p07](../docs/content/products/p07/plan.md) |
| P08 | [market-services](p08-market-services/README.md) | Cloud/Blockchain | 설계만 | DEX, TEST FX, perpetual, oracle·keeper | - |
| P09 | [travel-ai](p09-travel-ai/README.md) | Cloud | 설계만 | 장소·후기·발자취, 전사·AI 코스, 챌린지·스탬프 | - |
| P10 | [platform](p10-platform/README.md) | Shared | 만든다 | 공유 EIP-712 타입·시험 벡터, 적합성 harness, 12주 수용 기록 | [p10](../docs/content/products/p10/plan.md) |

폴더 이름은 링크를 깨지 않도록 DF-01 때 이름을 그대로 둔다. 예를 들어
`p05-operations-backoffice`는 백오피스 전체를 설계하되 이번 사이클에는 Go 운영 코어와 `opsctl`만 만든다 [N20].

## 기술 스택

[N25]에 따라 제품마다 다음 스택을 쓴다. 공유 코드는 [`packages/`](../packages/README.md)에 두고,
로컬 검증은 [`sandbox/`](../sandbox/README.md)의 docker compose 환경(anvil chainId 8283,
PostgreSQL)에서 한 뒤 컨테이너 이미지로 AWS 또는 Google Cloud에 배포한다.

| 제품 | 스택 | 폴더 |
|---|---|---|
| P01 | C (Zephyr, NCS v3.4.1), 제조사 보드 패키지, Python 빌드 스크립트 | `p01-device-firmware/core`, `app`, `boards`, `test` |
| P02 | React Native(TypeScript), Kotlin Turbo Module (Android만) | `p02-user-app` (골격 예정) |
| P04 | React Native(TypeScript), Kotlin·Swift Turbo Module | `p04-merchant-kiosk/src`, `android`, `ios` |
| P05 | Go 운영 코어와 `opsctl`, 다음 사이클에 `opsd` API와 React 웹, PostgreSQL | `p05-operations-backoffice` |
| P06 | Solidity, Foundry | `p06-stablenet-contracts` |
| P07 | Go, PostgreSQL | `p07-indexer` |
| P10 | Python 적합성 harness, `packages/protocol` 생성기 | `p10-platform/harness` |

## 제품 폴더 표준

모든 제품과 공유 package는 같은 모양을 가진다. 저장소 루트의 `Makefile`이 이 목표를
모든 폴더에 차례로 실행하고, `P=pNN`을 주면 한 제품만 실행한다.

| 항목 | 내용 |
|---|---|
| `README.md` | 범위, 폴더 설명, 개발 명령 |
| `Makefile` | `setup`, `build`, `test`, `lint`, `run`, `docker` |
| `.env.example` | 로컬 설정 예제. 실제 비밀값은 넣지 않는다 |
| 소스와 시험 폴더 | 언어 관례를 따른다(Go `cmd/`·`internal/`, RN `src/`·`test/`, Foundry `src/`·`test/`, C `core/`·`test/`) |
| `Dockerfile` | 배포하는 제품만(P05 `opsd`, P07) |

```bash
make test            # 모든 제품과 package
make test P=p05      # products/p05-* 하나
make run P=p07
```

## 경계 원칙

- 각 구현물은 주 책임 제품에 한 번만 둔다.
- 제품 간 메시지와 서명 형식은 [결제 프로토콜](../docs/content/specifications/protocol/payment-protocol.md)을 따른다.
- 공통 생성 타입과 라이브러리만 `packages/`로 승격한다.
- 배포 가능한 제품은 자체 빌드, 시험, 환경 예제, 운영 README를 가진다.
- 과거 프로젝트 저장소를 가져올 때는 commit pin과 라이선스를 먼저 기록한다.

일정과 담당은 [12주 WBS (DF-20260925-02)](../docs/content/planning/product-worklist-and-12week-wbs-02.md)를
따른다. 이전 제품 구조는 [한 페이지 제품 아키텍처](../docs/content/planning/product-wbs-overview.md)에
DF-20260920-01 기준으로 남아 있다.

# Products

이 디렉터리는 12주 범위의 10개 제품을 독립된 작업 경계로 나눈다. 제품
README는 구현 진입점이며, 상세 설계 authority와 일정은 `docs/content/`에
유지한다.

| ID | 제품 폴더 | 실행 영역 | 핵심 책임 |
|---|---|---|---|
| P01 | [device-firmware](p01-device-firmware/README.md) | Local | Zephyr 펌웨어, HW 지갑, BLE, FOTA, 패스키, 녹음, 찾기 |
| P02 | [user-app](p02-user-app/README.md) | Local | 사용자 RN 앱, 소셜 로그인, 기기 설정, 두 지갑 UI |
| P03 | [cloud-mpc-wallet](p03-cloud-mpc-wallet/README.md) | Cloud | DKG, threshold 서명, refresh, 복구 |
| P04 | [merchant-kiosk](p04-merchant-kiosk/README.md) | Local | RN 태블릿 키오스크, 주문, 결제, 환불, 매출·정산 |
| P05 | [operations-backoffice](p05-operations-backoffice/README.md) | Cloud | 가맹점, 대여·반납, FOTA 캠페인, 예외·감사 |
| P06 | [stablenet-contracts](p06-stablenet-contracts/README.md) | Blockchain | dummy USDC/WKRC, Smart Account, DID, CafePass, x402 |
| P07 | [indexer](p07-indexer/README.md) | Cloud | canonical ingest, decoder, projection, 조회 프런트엔드 |
| P08 | [market-services](p08-market-services/README.md) | Cloud/Blockchain | DEX, TEST FX, perpetual, oracle·keeper |
| P09 | [travel-ai](p09-travel-ai/README.md) | Cloud | 장소·후기·발자취, 전사·AI 코스, 챌린지·스탬프 |
| P10 | [platform](p10-platform/README.md) | Shared | 공통 계약, 인증, CI, 수용 시험, 관측, 릴리스 |

## 경계 원칙

- 각 구현물은 주 책임 제품에 한 번만 둔다.
- 제품 간 데이터는 문서화된 C1~C9 계약을 통해 교환한다.
- 공통 생성 타입과 라이브러리만 `packages/`로 승격한다.
- 배포 가능한 제품은 자체 빌드, 시험, 환경 예제, 운영 README를 가진다.
- 과거 프로젝트 저장소를 가져올 때는 commit pin과 라이선스를 먼저 기록한다.

전체 의존 관계는 [한 페이지 제품 아키텍처](../docs/content/planning/product-wbs-overview.md)와
[상세 WBS](../docs/content/planning/product-worklist-and-12week-wbs.md)를 따른다.

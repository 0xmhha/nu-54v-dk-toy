# P05 · Operations backoffice

> **DF-20260925-02 기준 (2026-09-25):** 이번 12주 사이클의 범위와 설계는 [기획](../../docs/content/products/p05/plan.md) · [SRS](../../docs/content/products/p05/srs.md) · [유즈케이스](../../docs/content/products/p05/use-cases.md) · [설계](../../docs/content/products/p05/design.md)가 정한다. 아래 원문은 DF-20260920-01 기준이며 충돌하면 위 문서를 따른다.

## 개발 (DF-20260925-02)

설계는 백오피스 전체를 전제로 하고, 이번 사이클에는 Go 운영 코어와 CLI를 만든다 [N20].

| 경로 | 내용 |
|---|---|
| `internal/core/` | 체인 호출, EIP-712 서명(공유 벡터로 검증), keystore, 감사 |
| `internal/ble/` | BLE 셋업 클라이언트 인터페이스(tinygo-org/bluetooth로 구현 예정) |
| `internal/store/migrations/` | PostgreSQL 스키마 |
| `cmd/opsctl/` | 운영 CLI(이번 사이클) |
| `cmd/opsd/` | 백오피스 API 서버(다음 사이클) |
| `web/` | React + TypeScript 백오피스 UI(다음 사이클) |

```bash
make test         # Go 시험과 web 타입 검사
make run          # opsctl
make docker       # opsd 이미지
```

### 개발용 고정 셋업 (7주차, StableNet 테스트넷 8283)

BLE 셋업 세션(`rental provision`)이 생기기 전까지 키오스크와 기기가 테스트넷 결제를 시험할 자료를 `opsctl`로 만든다. 역할 키는 `~/.nu54/keystores/nu54-<role>`의 keystore이고 암호는 macOS Keychain(`nu54-<role>`)에서 메모리로만 읽는다. 컨트랙트 주소는 `products/p06-stablenet-contracts/deployments/8283.json`에서 읽고, 시각은 finalized 블록 시각을 쓴다.

```bash
make build                                  # bin/opsctl
O=products/p05-operations-backoffice/bin/opsctl

# 1. 가맹점 등록: 키오스크의 가맹점 서명 주소와 payout. 같은 값이면 트랜잭션 없이 끝난다 (P05-FR-01)
$O merchant register --merchant <kiosk 가맹점 주소> --payout <payout>

# 2. attestation 발급: registry payout과 같아야 하고, 기간은 register의 attestationValidity(24시간) (P05-FR-02, 03, 09)
$O attestation issue --merchant <kiosk 가맹점 주소> --payout <payout> --name "NU54 Test Cafe" > attestation.json

# 3. TimeAnchor: 기기 주소와 기기가 보고한 lastAnchor (P05-FR-06)
$O anchor sign --device <기기 주소> --last-anchor <lastAnchor> > anchor.json

# 4. 기기 계정 입금: 운영자 잔액이 모자라면 먼저 시험 토큰을 발급한다
$O token mint --to <운영자 주소> --amount 50000000
$O rental deposit --device <기기 주소> --withdraw <대여자 출금 주소> --amount 50000000
```

- attestation은 24시간 뒤 만료되므로 파일로 고정해 두지 않고 필요할 때 다시 발급한다.
- 결과 JSON은 표준 출력과 `evidence/p05/<날짜>-<명령>.log`(git 미추적)에 남는다.
- 로컬 anvil에서는 `--rpc http://127.0.0.1:8545 --deployment <로컬 배포 기록> --read latest`를 붙인다. anvil의 finalized 태그는 최신 블록보다 뒤라서, 방금 배포한 registry가 finalized 시점에는 아직 없다.
- 2026-10-01에 테스트넷에서 1~3단계를 실행했다. 키오스크 역할 주소는 이미 등록되어 있어 트랜잭션이 없었고, attestation과 TimeAnchor 서명은 TypeScript 코어로 복원해 운영자 주소와 같음을 확인했다.


운영자가 가맹점, 대여 기기와 결제 예외를 관리하는 제품이다.

- **소유 범위:** 운영자 RBAC와 감사, 가맹점 승인, 대여·반납, FOTA 캠페인,
  잔액 복구 확인, 늦은 결제·중복·부분·초과 지급 대사
- **주요 경계:** P07 projection, P10 권한·감사, P01/P02 기기 수명주기
- **WBS:** `WBS-P05-01`부터 `WBS-P05-04`

설계 진입점:

- [제품 WBS](../../docs/content/planning/product-wbs-overview.md#p05--백오피스대여운영-신뢰)
- [운영 신뢰 설계](../../docs/content/specifications/trust-operations-design.md)
- [관리 신뢰 수명주기](../../docs/content/specifications/management-trust-lifecycle.md)
- [운영·릴리스 수용](../../docs/content/specifications/operations-release-acceptance-design.md)

구현 시 web UI와 운영 API의 배포 단위, 고위험 작업의 승인 증거와 복구
runbook을 이 폴더에서 관리한다.

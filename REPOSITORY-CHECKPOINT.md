# Repository checkpoint

2026-09-25 · `DF-20260925-02` (`DF-20260920-01` 대체) · 문서 기준선 동결 이후, 제품 구현 이전

이 저장소는 DF-20260925-02로 설계를 동결하고 제품 구현에 들어간 상태다. 일곱 제품(P01, P02, P04, P05, P06, P07, P10) 가운데 여섯 제품의 프로젝트 골격(P02 폰 앱은 2026-09-28에 추가되어 아직 골격이 없다)과 공유 package(`packages/protocol`), 로컬 sandbox가 있고, 보드 bring-up을 마쳤다. 계정 생성, 서버 배포, 테스트넷 컨트랙트 배포는 아직 하지 않았다.

## 현재 기준 읽기 순서

1. [개발 현황과 문서 안내](docs/development-overview.md)
2. [제품별 monorepo 진입점](products/README.md)
3. [설계 동결 DF-20260925-02](docs/content/planning/design-freeze-checkpoint-02.md)
4. [12주 WBS (DF-20260925-02)](docs/content/planning/product-worklist-and-12week-wbs-02.md)
5. [결제 프로토콜](docs/content/specifications/protocol/payment-protocol.md) · [12주 수용 로그](docs/content/acceptance/week12-log.md)
6. [이전 설계 동결 DF-20260920-01](docs/content/planning/design-freeze-checkpoint.md) — DF-20260925-02와 충돌하지 않는 부분만 유효
7. [채택된 구현 진입 기준 계약](docs/content/specifications/design-baseline-contract.md)
8. [외부 환경 준비](docs/content/environment/README.md)
9. [남은 작업 마스터 목록](docs/content/planning/remaining-work-list.md)
10. [이전 12주 WBS 한눈에 보기](docs/content/planning/product-wbs-overview.md) · [이전 제품별 작업 목록·12주 WBS](docs/content/planning/product-worklist-and-12week-wbs.md)
11. [12주 완료 범위](docs/content/twelve-week-completion-scope-v3.md)
12. [104개 작업 인계 카드](docs/content/planning/preimplementation-task-handoffs.md)

충돌 시 위 순서에서 먼저 나온 문서가 현재 상태·선택값·활성화 경계에 관해 우선한다. 세부 API/DTO/상태 필드는 기존 `docs/content/specifications/` inventory를 사용한다.

## 디렉터리 역할

| 경로 | 역할 |
|---|---|
| `products/p01-*` ~ `products/p10-*` | 제품별 구현과 제품 README |
| `packages/` | 여러 제품이 소비하는 공유 생성 코드와 도구 |
| `docs/content/planning/` | 범위, 결정, WBS, 인계, 동결 checkpoint |
| `docs/content/specifications/` | API/BLE/DTO/저장/상태/보안 상세 계약과 검사기 |
| `docs/content/environment/` | 비밀값 없는 toolchain·chain·indexer·provider·trust·fixture manifest |
| `docs/content/analysis/document-logic/` | X-theory 문서 그래프, 추적성, 논리 검토 결과 |
| `docs/content/assets/` | 글과 기획에 쓰는 이미지·미디어 |
| `docs/design-history/` | 과거 승인 전 기준의 보존 archive; 수정 대상이 아님 |

## 효력과 과거 기록

- `design-freeze-checkpoint.json`은 D01~D19와 RR-DEC-01의 현재 선택 authority다.
- `design-baseline-contract.json`은 UA-01~08의 구현 진입용 공동 계약 authority다.
- 이전 문서에 남은 `미선택`, `미채택`, `다음 설계` 문구는 작성 시점의 기록이다. 이를 최신 상태로 오해하지 않는다.
- 기존 7개 reference migration은 수정하지 않는다. 구현 때 후속 migration을 추가한다.
- 계약 채택은 설계 기준 채택이다. runtime, SQL, 기기 profile, 체인 주소는 모두 비활성이다.

## 환경과 비밀

- 실제 secret, mnemonic, private key, MPC share, OAuth secret은 저장소에 넣지 않는다.
- `docs/content/environment/`에는 `secretRef`와 공개 manifest만 둔다.
- `.ouroboros/`와 `.playwright-mcp/`는 로컬 도구 상태·로그이므로 `.gitignore`에서 제외했다.
- StableNet 주소는 code hash·ABI digest·deployment block·admin evidence를 등록하기 전 비활성이다.

## 검증

```bash
python3 scripts/check_markdown_links.py
python3 docs/content/planning/build_design_freeze.py
python3 docs/content/planning/validate_design_freeze.py
python3 docs/content/specifications/validate_specs.py
python3 docs/content/planning/validate_design_freeze_02.py --check all
python3 docs/content/planning/validate_design_freeze_02.py --self-test
```

첫 검사는 checkpoint와 manifest를 재생성한다. 두 번째는 20/10/8/7 개수, ID, source hash, 실행 비활성, chain/secret gate를 검사한다. 세 번째는 기존 API 110개·BLE 34개·권한 60개 등 상세 명세의 구조와 참조를 검사한다. 마지막 두 줄은 DF-20260925-02 register와 그 문서들을 HEAD 기준으로 검사하고, 각 검사가 틀린 입력을 거부하는지 확인한다.

## Git 상태

`main`의 최초 공개 snapshot은 제품별 monorepo 경계, 현재 설계 문서와 보존
이력 전체를 함께 기록한다. 이후 변경은 Conventional Commits와 DCO sign-off를
사용하며, 규칙은 [개발 규칙](docs/development-conventions.md)를 따른다.

## 다음 시작점

1. 사용자가 구현 착수를 명시하면 `docs/content/planning/preimplementation-task-handoffs.md`에서 대상 task를 선택하고 해당 `products/pNN-*` 폴더에서 시작한다.
2. 실제 NU-54V-DK revision·pin·flash/RAM을 측정해 board profile을 활성화한다.
3. 외부 계정과 trust subject를 저장소 밖에서 등록하고 manifest에는 secret reference만 반영한다.
4. StableNet 계약 배포 뒤 주소·ABI/code hash·block을 등록하고 Indexer decoder/backfill을 활성화한다.
5. generated schema와 후속 migration을 만든 뒤 UA-01~08을 runtime activation 후보로 검증한다.

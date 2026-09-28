# Shared packages

여러 제품이 함께 소비하는 코드만 이 경계에 둔다. 한 제품에서만 쓰는 코드는
해당 `products/pNN-*` 폴더에 둔다. 공유 package를 추가할 때는 소비 제품, 버전
호환성, 생성 원본과 검증 명령을 그 package의 README에 기록한다.

## 구성

기술 스택과 작업공간은 [DF-20260925-02](../docs/content/planning/design-freeze-checkpoint-02.md)의
[N25]를 따른다. 체인 연동과 운영 도구는 Go, 키오스크는 React Native와 TypeScript,
스크립트와 생성기는 Python, DB는 PostgreSQL이다. package는 도메인으로 먼저 나누고,
그 아래를 언어별로 나눈다.

| package | 원본 | 언어별 산출물 | 소비 제품 |
|---|---|---|---|
| [`protocol/`](protocol/README.md) | `docs/content/specifications/protocol/payment-protocol.schema.json`, `eip712-vectors.json` | `go/`, `ts/`, `python/`, `c/` (생성) | P01, P04, P05, P06, P07, P10 |
| [`contracts-abi/`](contracts-abi/README.md) | P06 Foundry 빌드 산출물(ABI) | `abi/`(JSON, sha256 manifest), `go/`(abigen 바인딩), `ts/`(`as const` ABI) (생성) | P04, P05, P07 |

## 작업공간

| 언어 | 도구 | 설정 |
|---|---|---|
| Go | `go.work` | 루트 `go.work` |
| TypeScript | pnpm workspaces | 루트 `pnpm-workspace.yaml`, `.npmrc`(React Native Metro 때문에 `node-linker=hoisted`) |
| Python | uv workspace | 루트 `pyproject.toml`(members), package마다 `pyproject.toml` |

로컬 검증은 [`sandbox/`](../sandbox/README.md)의 docker compose 환경에서 한다.

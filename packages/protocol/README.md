# @nu54/protocol

NU-54V-DK 결제 프로토콜의 공유 타입이다 [N21][N25]. 원본은 문서 쪽
[`payment-protocol.schema.json`](../../docs/content/specifications/protocol/payment-protocol.schema.json)과
[`eip712-vectors.json`](../../docs/content/specifications/protocol/eip712-vectors.json)이고,
이 package는 원본에서 언어별 코드를 생성한다. 생성 파일은 직접 고치지 않는다.

| 폴더 | 생성 파일 | 시험 |
|---|---|---|
| `go/` | `protocol/types_gen.go` | `go test ./...` (encodeType이 벡터와 같은지) |
| `ts/` | `src/generated.ts` | `pnpm test`, `pnpm typecheck` |
| `python/` | `nu54_protocol/_generated.py` | `uv run --group dev pytest` |
| `c/` | `include/nu54_protocol.h` | P01 빌드에 포함 |

```bash
python3 schema-gen/generate.py          # 다시 생성
python3 schema-gen/generate.py --check  # 원본과 다르면 실패
```

모든 구현이 같은 벡터를 재현하는지는
[`products/p10-platform/harness/run_conformance.py`](../../products/p10-platform/harness/run_conformance.py)가 한 번에 확인한다.
원본 schema와 벡터는 코드가 안정되면 이 package로 옮긴다. 그때 validator와 문서 링크도 함께 옮긴다.

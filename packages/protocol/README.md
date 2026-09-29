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

`cbor-gen/generate.py`는 결제 프로토콜 4.2절의 CBOR 필드 인코딩 규칙으로 메시지 8개를 인코딩해
[`cbor-vectors.json`](../../docs/content/specifications/protocol/cbor-vectors.json)을 만든다. 벡터마다 JSON 형식 메시지,
CBOR 본문(`cborHex`), envelope(`envelopeHex`: 길이, SHA-256 앞 8바이트, 본문)이 있다. 펌웨어·키오스크·폰 앱·운영 도구의
codec은 이 바이트를 그대로 만들고 읽어야 한다. `--check`는 파일이 최신인지와, 각 벡터를 다시 디코딩·인코딩해 같은 바이트가
나오는지 확인한다. 생성 결과는 독립 구현인 Python `cbor2`의 canonical 인코딩과도 일치하는 것을 확인했다(2026-09-29).

```bash
python3 cbor-gen/generate.py            # 다시 생성
python3 cbor-gen/generate.py --check    # 최신 여부와 왕복 확인
```

모든 구현이 같은 벡터를 재현하는지는
[`products/p10-platform/harness/run_conformance.py`](../../products/p10-platform/harness/run_conformance.py)가 한 번에 확인한다.
원본 schema와 벡터는 코드가 안정되면 이 package로 옮긴다. 그때 validator와 문서 링크도 함께 옮긴다.

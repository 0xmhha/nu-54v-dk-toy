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

`frame-gen/generate.py`는 각 CBOR 벡터의 envelope를 ATT_MTU 23, 185, 247로 나눈 조각과, 받는 쪽이 BAD_FRAME으로
거절해야 하는 조각열 9개를 [`frame-vectors.json`](../../docs/content/specifications/protocol/frame-vectors.json)에 만든다.
`--check`는 기준 수신기로 모든 벡터를 다시 모아 확인한다.

**TypeScript 코어(`ts/src`).** 키오스크와 폰 앱이 그대로 쓰는 구현이다.

| 모듈 | 내용 |
|---|---|
| `cbor.ts` | 4.2절 결정적 CBOR 인코딩·디코딩. 필드 종류는 생성된 `MESSAGE_FIELDS`를 따르고, 규칙 위반은 `ProtocolError(BAD_FRAME)`, 모르는 타입은 `UNSUPPORTED_TYPE` |
| `frame.ts` | envelope, 조각, `FrameWriter`(방향별 sequence), `Reassembler`(4절 수신 규칙) |
| `eip712.ts` | 6개 타입의 digest. domain의 name·version은 생성 상수 `EIP712_DOMAIN` |
| `signature.ts` | 서명자 복원(65바이트, low-s, v 27/28), 결정론적 서명 |
| `bytes.ts` | hex·bigint 변환, React Native에서도 동작하는 UTF-8 |

암호는 `@noble/hashes`(keccak, SHA-256)와 `@noble/curves`(secp256k1)를 버전 고정으로 쓴다. 둘 다 순수 JavaScript라 React Native에서 돈다. 공용 벡터 시험은 `ts/test/vectors.test.ts`다.

```ts
import { encodeMessage, FrameWriter, Reassembler, digest, recoverSigner, hexToBytes } from "@nu54/protocol";

const writer = new FrameWriter(185);                       // 협상된 ATT_MTU
const frags = writer.write(encodeMessage({ v: 1, type: "error", sessionId: "0102030405060708", reason: "BAD_FRAME" }));
const r = new Reassembler();
for (const f of frags) { const res = r.feed(f); if (res.status === "done") console.log(res.body); }
const signer = recoverSigner(digest({ chainId: 8283, verifyingContract: settlement }, "PaymentAuthorization", auth), hexToBytes(sig));
```

모든 구현이 같은 벡터를 재현하는지는
[`products/p10-platform/harness/run_conformance.py`](../../products/p10-platform/harness/run_conformance.py)가 한 번에 확인한다.
원본 schema와 벡터는 코드가 안정되면 이 package로 옮긴다. 그때 validator와 문서 링크도 함께 옮긴다.

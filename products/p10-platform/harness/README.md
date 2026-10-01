# 적합성 harness

컨트랙트, 운영 도구, 키오스크, 펌웨어가 같은 EIP-712 해시와 서명 규칙을 쓰는지 한 명령으로 확인한다(WBS2-P10-02, [N21]). 기준은 [`eip712-vectors.json`](../../../docs/content/specifications/protocol/eip712-vectors.json)이다. 이 파일은 7개 타입의 digest, 서명, 서명자를 담고 있다.

```bash
make conformance                                                    # SKIP은 실패로 치지 않는다
python3 products/p10-platform/harness/run_conformance.py --strict   # SKIP도 실패로 친다(게이트 판정용)
```

결과는 검사마다 `PASS`, `FAIL`, `SKIP` 중 하나다. `SKIP`은 두 경우에 나온다. 검사에 필요한 도구가 설치되어 있지 않거나, 검사할 구현이 아직 없는 경우다. 이유는 괄호 안에 찍힌다. 12주차 판정은 `--strict`로 한다. 펌웨어 C 검사가 7주차에 준비되지 않으면 SKIP으로 기록하고 8주차 첫날 채운다.

## 서명자 역할

벡터는 Foundry 공개 시험 mnemonic(`test test test test test test test test test test test junk`)의 키로 서명되어 있다. 이 키는 공개된 값이므로 시험에서만 쓴다.

| 역할 | mnemonic index | 주소 | 서명하는 타입 |
|---|---|---|---|
| device | 0 | `0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266` | PaymentAuthorization, LimitChange |
| operator | 1 | `0x70997970C51812dc3A010C7d01b50e0d17dc79C8` | MerchantAttestation, TimeAnchor, DeviceReset |
| merchant | 2 | `0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC` | MerchantOrder |

`packages/protocol/go`의 `TestSignersMatchRoles`가 모든 벡터에서 서명자를 복원해 위 역할 주소와 같은지 확인한다. 서명은 65바이트 `r || s || v`, low-s, `v`는 27 또는 28이어야 한다.

## 키오스크와 펌웨어가 연결하는 방법

두 검사는 아래 진입점이 생기면 자동으로 실행된다. 그 전까지는 SKIP이다.

### 키오스크 (TypeScript)

`products/p04-merchant-kiosk/package.json`에 `test:conformance` 스크립트를 추가한다. harness는 그 폴더에서 `pnpm run test:conformance`를 실행한다. 시험은 벡터 파일을 읽어 다음을 확인한다.

1. PaymentAuthorization(PA-01, PA-02), LimitChange(LC-01), MerchantOrder(MO-01)의 digest를 키오스크 코드로 계산해 벡터 값과 같은지.
2. PA-01, PA-02, LC-01의 서명에서 복원한 주소가 device 역할 주소와 같은지. 키오스크는 기기가 돌려준 서명을 제출 전에 이렇게 확인한다.
3. MO-01 digest를 merchant 시험 키(index 2)로 서명한 결과가 벡터 서명과 바이트까지 같은지. 결정론적 서명(RFC 6979)이면 같아야 한다.
4. CBOR 벡터 8개를 같은 바이트로 인코딩하고 디코딩하는지, 조각 벡터를 같은 바이트로 나누고 다시 모으는지, 거절 조각열을 BAD_FRAME으로 거절하는지.

2026-10-01부터 키오스크는 `src/payment/signing.ts`(가맹점 주문 서명, 기기 서명 확인)와 공용 `@nu54/protocol`(CBOR, 조각, EIP-712, 서명)로 이 검사를 통과한다.

### 펌웨어 (C, host 빌드)

`products/p01-device-firmware/test/test_eip712.c`를 추가하고, `test/CMakeLists.txt`에 이름이 `eip712_vectors`인 ctest를 등록한다. harness는 `cmake -S test -B out/conformance`로 빌드한 뒤 `ctest -R eip712_vectors`를 실행한다. 벡터 값은 시험 코드에 넣거나 생성 헤더로 가져와도 된다. 다만 벡터 파일이 바뀌면 시험이 따라 바뀌도록 생성 경로를 README에 적는다. 시험은 다음을 확인한다.

1. 7개 벡터 모두의 digest를 펌웨어 코드로 계산해 벡터 값과 같은지. 기기는 운영자·가맹점 서명도 검증하므로 모든 타입의 digest가 필요하다.
2. MerchantAttestation, TimeAnchor, DeviceReset 서명의 복원 주소가 operator 역할 주소이고, MerchantOrder 서명의 복원 주소가 merchant 역할 주소인지.
3. PA-01과 LC-01 digest를 device 시험 키(index 0)로 서명한 결과가 low-s이고 `v`가 27 또는 28이며, 복원 주소가 device 역할 주소인지. 서명 구현이 무작위 k를 쓰면 바이트 비교는 하지 않는다.
4. 조각 벡터를 reassembler가 같은 본문으로 다시 모으고, 거절 조각열은 reassembler 또는 digest 확인에서 BAD_FRAME이 되는지(ctest `frame_vectors`).
5. CBOR 벡터 8개를 디코딩해 스키마 표로 검사하고 같은 바이트로 다시 인코딩하는지, 거절 규칙을 지키는지(ctest `cbor_vectors`).
6. 세션 벡터의 시나리오 12개를 재생해 메시지마다 같은 바이트로 답하는지(ctest `session_vectors`). 기기 시뮬레이터가 이 벡터를 만들고, 펌웨어는 같은 응답을 내야 한다.

2026-10-01부터 펌웨어는 `core/src/nu54_eip712.c`, `nu54_sig.c`(서명자 복원, secure partition이 준 (r, s)의 low-s 정규화와 v 계산), `nu54_keccak.c`와 `third_party`의 libsecp256k1 v0.8.0(MIT), Keccak compact(CC0)로 이 검사를 통과한다. 벡터는 `test/gen_vectors.py`가 빌드할 때 C 헤더로 만든다.

## 검사 목록

| 검사 | 필요한 도구 | 대상 |
|---|---|---|
| vectors reproducible (cast) | cast | 벡터 파일을 다시 만들면 같은 값이 나오는가 |
| generated bindings match schema | gofmt | 스키마에서 생성한 Go·TS·Python·C 코드가 최신인가 |
| protocol Go encodeType and vector signers by role | go | encodeType 문자열, 서명자 역할 |
| protocol TS encodeType | pnpm | encodeType 문자열 |
| protocol Python encodeType | uv | encodeType 문자열 |
| operations tool Go digests | go | 운영 도구의 7개 digest |
| CBOR message vectors round-trip | 없음 | CBOR 메시지 벡터 |
| BLE fragment vectors reassemble | 없음 | 조각 벡터(기준 수신기) |
| contract Solidity typehashes | forge | 컨트랙트의 typehash |
| contract Solidity EIP-712 digest and signer | forge | 결제·한도 변경 digest와 서명자 |
| kiosk TypeScript EIP-712, CBOR and fragments | pnpm | 위 키오스크 항목 |
| session vectors reproduced by the device simulator | node | 세션 벡터가 시뮬레이터 동작과 같은가 |
| firmware C EIP-712, signatures, CBOR, fragments and sessions (host) | cmake, ctest | 위 펌웨어 항목 |

# P10 설계 — 공유 EIP-712 타입·벡터와 수용 기준

## 1. 구조

P10의 정답은 두 파일이다. 스키마가 타입과 필드를 정하고, [eip712-vectors.json](../../specifications/protocol/eip712-vectors.json)이 그 타입의 digest와 서명 정답을 정한다 [N21]. 나머지 구현은 모두 이 두 파일을 읽어 시험한다.

```
payment-protocol.schema.json ──> build_eip712_vectors.py ──(cast keccak, cast wallet sign)──> eip712-vectors.json
                                                                                                  │
                           ┌──────────────────────────────┬───────────────────────────────────────┤
                    펌웨어 시험(C)                  키오스크 시험(TS)                        컨트랙트 시험(Solidity)
```

## 2. 벡터 생성기

`build_eip712_vectors.py`는 다음 순서로 동작한다.

1. 스키마에서 domain 필드, `eip712Types`, `operatorSignedTypes`를 읽는다.
2. 각 벡터 message에 대해 encodeType → typeHash → hashStruct → `0x1901 | domainSeparator | structHash` 순으로 digest를 계산한다. Keccak-256은 `cast keccak`을 부른다.
3. 같은 typed data를 `cast wallet sign --data`로 서명한다. 서명 키는 벡터의 signerRole에 따라 Foundry 공개 시험 mnemonic의 index 0(기기), 1(운영자), 2(가맹점)를 쓴다. cast가 digest를 따로 계산하므로 두 계산이 교차 검증된다.
4. `cast wallet verify --no-hash`로 서명이 2단계 digest에 대해 그 역할의 signer로 검증되는지 확인한다. 실패하면 파일을 쓰지 않는다.
5. `--check`는 결과를 파일로 쓰지 않고 커밋된 벡터와 비교한다.

벡터 값은 경계 사례를 포함한다. PA-02는 nonce가 2^255보다 큰 unordered nonce이고 expiry는 기준 시각부터 authorizationExpiry 안이다. MA-01은 string 필드 해시를, DR-01은 운영자 서명 DeviceReset을 시험한다. 역할마다 키가 달라 서명자 역할을 뒤섞은 구현은 벡터를 통과하지 못한다.

## 3. 적합성 harness

| 구현 | 읽는 값 | 확인 |
|---|---|---|
| 펌웨어(C) | 각 벡터의 message, signature | 기기 서명 코드가 digest를 같게 계산하고, MA·MO·TA·DR 벡터의 서명자를 운영자·가맹점 주소로 검증 |
| 키오스크(TS) | message, signature | digest가 같고 복원한 signer가 signerRole 주소와 같음 |
| 컨트랙트(Solidity) | message, signature | `settle` 검증 경로의 digest와 `ecrecover` 결과가 기기 signer와 같음 |

harness는 벡터 파일을 복사하지 않고 경로로 읽는다. 코드가 import하기 시작하면 벡터는 `packages/`로 옮긴다.

## 4. 증거와 redaction

[N16]에 따라 증거는 두 층으로 나눈다.

| 층 | 위치 | 내용 |
|---|---|---|
| 원본 | git-ignored 경로 | RPC 응답, 로그, 영상, NVM 덤프 |
| 문서 | [week12-log.md](../../acceptance/week12-log.md) | 결과, 앞 6자리…뒤 4자리로 줄인 주소·tx hash, 원본 파일의 64자리 전체 `sha256:` checksum |

checksum은 줄이지 않고 64자리 전체에 항상 `sha256:` 접두어를 붙인다. 줄이는 대상은 주소와 tx hash뿐이다. `validate_design_freeze_02.py --check redaction`은 baseCommit 이후 바뀐 docs/content 파일에서 전체 길이의 0x 주소·해시와 접두어 없는 64자리 hex를 찾아 실패시킨다. 예외는 register의 `redactionExemptions`(벡터 파일)뿐이다.

## 5. 수용 기록지 관리

기록지는 bring-up 기록지 형식을 따른다. 항목 ID와 거절 시연 8종(register `acceptanceRefusalCodes`의 합의 5종과 기기 3종)은 register가 요구하는 목록과 같아야 하며 `--check acceptance`가 확인한다 [N18]. 1절의 도구·의존성 버전 행은 req 12 waiver의 기록 위치다 [N14]. 결과 칸은 실제 시험 뒤에만 채운다.

## 6. 시험 설계

| 시험 | 기대 |
|---|---|
| 생성기 재실행 `--check` | OK vectors reproducible |
| 벡터 digest 하나를 바꾼 뒤 `--check` | 실패 |
| 스키마에 세 번째 기기 서명 타입 추가 | `--check protocol+vectors` 실패 |
| 문서에 전체 주소 추가 | `--check redaction` 실패 |

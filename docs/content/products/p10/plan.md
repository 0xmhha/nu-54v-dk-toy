# P10 기획 — 공유 EIP-712 타입·벡터와 수용 기준

## 1. 목표와 범위

P10은 [DF-20260925-02](../../planning/design-freeze-checkpoint-02.md)에서 만드는 일곱 제품 중 하나다 [N01]. 이번 사이클의 P10은 넓은 공통 플랫폼이 아니라 세 가지만 맡는다.

1. 기기·키오스크·컨트랙트가 함께 쓰는 EIP-712 타입 정의와 시험 벡터 [N21]
2. 세 구현이 같은 digest를 만드는지 확인하는 적합성 harness
3. 12주차 수용 기록지와 증거 redaction 규칙 [N18][N16]

담당은 사용자(user)다 [N15].

## 2. 산출물

| 산출물 | 위치 |
|---|---|
| 프로토콜 스키마 | [payment-protocol.schema.json](../../specifications/protocol/payment-protocol.schema.json) |
| 시험 벡터 | [eip712-vectors.json](../../specifications/protocol/eip712-vectors.json) |
| 벡터 생성기 | [build_eip712_vectors.py](../../specifications/protocol/build_eip712_vectors.py) |
| 적합성 harness | 펌웨어(C), 키오스크(TS), 컨트랙트(Solidity) 시험이 같은 벡터를 읽는 실행 스크립트 |
| 수용 기록지 | [week12-log.md](../../acceptance/week12-log.md) |

## 3. 일정

| WBS | 작업 | 기간 | 게이트 |
|---|---|---|---|
| WBS2-P10-01 | Shared EIP-712 type package from schema and vectors | W4 | W6 |
| WBS2-P10-02 | Vector conformance harness for firmware kiosk and contract | W7 | W7 |
| WBS2-P10-03 | Week-12 acceptance log and evidence redaction | W11–W12 | - |

스키마와 벡터는 문서 단계에서 이미 만들었고, WBS2-P10-01은 이를 세 구현이 읽는 타입 패키지로 묶는 작업이다. P06 컨트랙트(WBS2-P06-01, W5 시작)와 P01 서명(WBS2-P01-04, W7)의 선행 작업이므로 W4 안에 끝낸다. 게이트는 W4 증거, W6 컨트랙트, W7 실결제다. W9 SE 게이트는 2026-09-29에 없앴다 [N03].

## 4. 수용 기준

12주차는 W12-01..W12-12 열두 항목으로 판정한다 [N18]. 기능 7개(W12-01..W12-07)와 보안 5개(W12-08..W12-12)다. W12-05 거절 시연은 합의 5종과 기기 3종, 모두 8종이다. W12-06(P07 영수증)은 P07이 컷되지 않았을 때만 적용하는 conditional 항목이다. P10은 이 기록지의 틀과 증거 규칙을 관리하고, 각 항목의 시험은 해당 제품 담당이 수행한다.

## 5. 위험

- 사용자는 W4에 bring-up과 타입 패키지를 함께 맡아 가용 일수(2일)를 꽉 채운다. 벡터는 이미 생성되어 있으므로 W4에는 패키징만 한다.
- 벡터 생성은 Foundry `cast`에 의존한다. cast 버전이 바뀌어도 digest는 EIP-712 규격으로 정해지므로 `--check`로 재생성 결과가 같은지 확인한다.

## 6. 범위 밖

- 인증·인가 API, 공통 ID·오류 체계, CI 파이프라인 구축, 관측·백업·릴리스 도구
- 의존성 pinning 자동화(req 12)는 waiver로 남긴다 [N14]

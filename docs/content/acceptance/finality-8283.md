# StableNet 테스트넷 finality 관측 (10/1)

컨트랙트 강화와 finality 관측(WBS2-P06-05)의 완료 판정 중 "3600초 관측 로그(결제 블록 hash 불변)"에 대한 기록이다. 키오스크가 결제를 finalized 블록의 `PaymentSettled`로 판정하는 규칙([결제 프로토콜](../specifications/protocol/payment-protocol.md) 7절)의 근거가 된다.

## 방법

`products/p06-stablenet-contracts/tools/observe_finality.py`가 1시간 동안 1초마다 `latest`와 `finalized` 헤더를 읽고, 블록 번호마다 처음 본 hash를 기록했다. 6주차 게이트 결제 블록(21129166)의 hash는 1분마다 다시 읽었다. 관측이 끝난 뒤에는 본 블록을 모두 다시 읽어 hash가 바뀐 블록이 있는지 확인했다. 표준 라이브러리만 쓰고 공개 RPC(`api.test.stablenet.network`)를 사용했다.

## 결과

| 항목 | 값 |
|---|---|
| 관측 시각 | 2026-09-30T22:49:08Z부터 3,700초 |
| 표본 | 3,518회(성공), 블록 21135848~21139448 중 3,382개 블록을 봄 |
| 블록 간격 | 중앙값 1초, 최대 1초 |
| 결제 블록 hash | 60회 재조회와 종료 시 재조회 모두 게이트 기록 값(`0x80bf5d…c45e`)과 같다. **불변** |
| 종료 후 재조회에서 hash가 바뀐 블록 | 0개 |
| latest − finalized | 0: 3,315회, −1: 148회, +1: 54회, −2: 1회 |
| RPC 오류 | 30회(0.85%), 모두 관측 컴퓨터의 DNS 조회 실패 |

**판정: 충족.** 1시간 동안 재조직(reorg)으로 hash가 바뀐 블록은 없었고, 결제 블록 hash도 그대로였다.

latest와 finalized는 따로 두 번 호출해서 읽었다. 그래서 두 호출 사이에 블록이 생기면 차이가 +1이나 −1로 찍힌다. finalized가 latest보다 앞선 경우(−1, −2)도 나왔으므로, 이 차이는 읽는 시점이 다른 데서 생긴 것으로 본다. finalized가 latest보다 실제로 늦다는 근거는 아니다. 두 태그가 같은 블록을 가리키는지를 한 번의 호출로 확인하지는 않았다.

## 쓰는 쪽이 따를 것

- 결제 판정은 `finalized` 태그로 조회한다. latest와 같다는 가정에 기대지 않는다.
- RPC 호출은 재시도한다. 이번 관측에서 1% 가까이 실패했다(원인은 관측 쪽 네트워크).

## 증거

원본은 저장소에 올리지 않는 `evidence/finality/`에 있다.

| 파일 | sha256 |
|---|---|
| `evidence/finality/finality-20260930T224908Z.jsonl` | `sha256:0b36075b3b615998ca282fef26b59973fcb25586b376b9e236cb3bc6c3b7421c` |
| `evidence/finality/finality-20260930T224908Z.summary.json` | `sha256:b8c7e654bcbdb3693a3046e3ad5e9f1454416a30e8c39fa10f52df6e36230256` |

요약 파일은 스크립트를 고치기 전에 만들어져 latest − finalized의 최댓값(1)만 담고 있다. 위 분포는 로그 파일에서 다시 셌다. 지금 스크립트는 분포를 함께 기록한다.

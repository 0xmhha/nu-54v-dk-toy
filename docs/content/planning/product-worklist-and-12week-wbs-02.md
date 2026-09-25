# 12주 WBS (DF-20260925-02)

[DF-20260925-02](design-freeze-checkpoint-02.md)에 맞춰 다시 만든 12주 작업 계획이다. 행 데이터는 [product-worklist-and-12week-wbs-02.csv](product-worklist-and-12week-wbs-02.csv)에 있고, 이 문서는 그 표를 읽는 방법과 게이트·컷 순서·가용 일수를 적는다. 이전 WBS([product-worklist-and-12week-wbs.md](product-worklist-and-12week-wbs.md))는 DF-20260920-01 기준으로 그대로 남는다.

만드는 제품은 P01, P04, P05, P06, P07, P10 여섯 개뿐이다 [N01]. 담당은 사용자(user)가 P06·P10과 보드 bring-up, role A가 P01 펌웨어, role B가 P04·P05·P07이다 [N15]. 사람 이름은 CSV의 `owner` 열에 팀 모임에서 확정할 때만 역할명 대신 적는다.

## 1. 게이트

| 게이트 | 주차 | 통과 조건 | 실패 시 |
|---|---|---|---|
| W4 증거 게이트 | 4주차(9/30) | BR-01..BR-08 bring-up 결과, 소프트웨어 서명으로 8283에 낸 finalized PaymentSettled, 외부 SE 데이터시트의 secp256k1 지원 확인 [N03] | 빠진 증거를 W5 첫날까지 채우고 W6 계획은 그대로 둔다 |
| W6 실결제 게이트 | 6주차(10/14) | 보드 내장 키로 기기 버튼 승인 → 키오스크 제출 → finalized PaymentSettled까지 실기 end-to-end 1건 [N03][N08] | 아래 cut order를 순서대로 적용한다 |
| W8 SE 게이트 | 8주차(10/28) | 외부 secure element로 감싼 키로 같은 end-to-end 1건 [N02] | TF-M waiver로 W12까지 진행하고 수용 로그에 기록한다 [N14] |

BR-01..BR-08은 보드 bring-up 점검 번호이며 사용자가 W1–W4에 수행한다(WBS2-P01-00).

## 2. cut order

W6 게이트가 실패하면 다음 순서로 범위를 줄인다 [N03]. 기기의 가맹점 정보 표시는 자르지 않는다.

1. 외부 SE 통합(WBS2-P01-05)을 빼고 TF-M waiver로 진행한다.
2. P07 최소 indexer(WBS2-P07-01)를 뺀다. W12의 P07 영수증 항목은 키오스크가 PaymentSettled를 직접 읽은 기록으로 대신한다 [N18].
3. 지연 출금 시연을 뺀다. 컨트랙트 기능(WBS2-P06-04)은 시험으로만 남긴다.

## 3. 주차별 가용 일수

아래 값은 계획 가정이다. 한 사람이 한 주에 쓸 수 있는 작업일을 4일로 두고, 4주차(9/24–9/30)는 Chuseok 연휴(9/24–9/26)로 2일, 6주차는 한글날(10/9)로 3일로 줄였다. 실제 값은 팀 모임에서 확인해 고친다. `validate_design_freeze_02.py --check wbs`는 CSV의 `effortDays`를 시작·끝 주에 고르게 나눈 부하가 이 표를 넘지 않는지 검사한다.

| 주차 | user | role A | role B |
|---|---|---|---|
| W1 | 4 | 4 | 4 |
| W2 | 4 | 4 | 4 |
| W3 | 4 | 4 | 4 |
| W4 | 2 | 2 | 2 |
| W5 | 4 | 4 | 4 |
| W6 | 3 | 3 | 3 |
| W7 | 4 | 4 | 4 |
| W8 | 4 | 4 | 4 |
| W9 | 4 | 4 | 4 |
| W10 | 4 | 4 | 4 |
| W11 | 4 | 4 | 4 |
| W12 | 4 | 4 | 4 |

가장 빡빡한 곳은 role A의 W2와 W5(각 4.0일)이고, 사용자는 W1–W2에 bring-up과 컨트랙트를 함께 맡아 3.75일이다. 이 두 곳에서 일정이 밀리면 W4·W6 게이트가 먼저 흔들린다.

## 4. 제품별 작업

| 제품 | 담당 | 작업(CSV id) | 기간 |
|---|---|---|---|
| P10 공유 EIP-712 | user | 스키마·벡터(P10-01), 교차 적합성 harness(P10-02), W12 수용 로그(P10-03) | W1–W2, W5–W7, W11–W12 |
| P06 정산 컨트랙트 | user | 핵심 로직(P06-01), 8283 배포·소프트웨어 서명 정산(P06-02), registry(P06-03), 지연 출금·closeAccount(P06-04), 강화·finality 관측(P06-05) | W1–W10 |
| P01 펌웨어 | role A | 골격·표시(P01-01), 키(P01-02), BLE(P01-03), 서명·검증(P01-04), SE(P01-05), NFC(P01-06), MCUboot(P01-07), 보안 점검(P01-08) | W1–W12 |
| P05 운영 스크립트 | role B | 가맹점 등록·attestation(P05-01), provisioning·반납(P05-02) | W1–W7 |
| P04 키오스크 | role B | 골격·BLE(P04-01), 제출·timeout(P04-02), W12 리허설(P04-03) | W2–W12 |
| P07 최소 indexer | role B | PaymentSettled 영수증 조회(P07-01) | W9–W10 |

## 5. 위험

- 사용자가 bring-up과 컨트랙트를 W1–W4에 함께 맡는다. W4 증거 게이트가 가장 먼저 영향을 받는다.
- role A의 부하가 W2·W5에서 가용 일수와 같다. 여유가 없어 BLE나 키 작업이 하루만 밀려도 W6 게이트가 위험하다.
- NFC 안테나·드라이버는 검증되지 않았다. NFC는 W6 게이트 조건이 아니고 BLE scan이 대체 경로다 [N09].

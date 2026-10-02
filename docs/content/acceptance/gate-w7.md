# 7주차 실결제 게이트 기록 (초안, 10/2)

[DF-20260925-02](../planning/design-freeze-checkpoint-02.md)의 7주차 실결제 게이트다 [N03]. 조건은 보드 내장 키로 기기 버튼 승인 → 키오스크 제출 → finalized PaymentSettled까지 실기 end-to-end 1건이다. 7주차는 non-secure 키, 개발 빌드 전용 고정 셋업, 평문 키오스크 링크로 통과하며 이 사실을 waiver로 적는다 [N30].

전체 주소와 해시는 저장소 밖 `evidence/w7/`의 원본에 있고, 이 문서는 줄인 값만 쓴다.

## 구성

| 구성 | 이번 실행 |
|---|---|
| 기기 | NU-54V-DK 보드, 펌웨어 0.2.0(main `1a1922b`). 기기 키는 PSA(CRACEN)가 만든 secp256k1 영속 키, 주소 `0xbc3152…f4d8` |
| 기기 계정 | `opsctl rental deposit`로 10 tUSDC 입금(테스트넷) |
| 가맹점 | 키오스크 역할 주소(`0xF92a32…61A4`), payout은 운영자 역할 주소. `opsctl attestation issue`로 발급 |
| TimeAnchor | `opsctl anchor sign`, 실행 직전 finalized 블록 시각 |
| 키오스크 쪽 | 개발 실행: Mac에서 `packages/device-sim/scripts/rehearse.ts --transport ble --submit`. 키오스크의 `src/payment/signing.ts`(기기 서명 확인)와 `src/payment/submit.ts`(제출과 판정)를 그대로 실행하고, BLE는 `tools/bringup/pay_bridge.py`가 맡는다. 게이트 실행: 키오스크 앱(Android)의 BLE 모듈과 결제 화면으로 한다(10/2 결정) |
| 승인 | 보드 SW1(7주차 개발 빌드의 임시 배치) |

## 판정 항목

| 조건 | 개발 실행(10/1, 버튼 시뮬레이터, Mac) | 게이트 실행(사람이 SW1, 키오스크 앱) |
|---|---|---|
| 보드 내장 키로 서명 | 충족: 복원 주소가 보드 주소 | 대기 |
| 기기 버튼 승인 | 버튼 시뮬레이터(디버거로 SW1 핀 제어) | 대기 |
| 키오스크 제출 | 충족: 키오스크 모듈이 type-2 트랜잭션 전송(Mac) | 대기: 키오스크 앱의 BLE 모듈과 화면이 먼저 필요하다 |
| finalized PaymentSettled, device·amount·nonce 일치 | 충족: `0xaf8adb…0b00`, 블록 21181324, 103,820 gas | 대기 |

**판정: 대기.** 게이트 실행에는 두 가지가 남았다.

1. 키오스크 앱(Android)의 BLE 모듈과 결제 화면. 키오스크 실행 환경은 waiver로 두지 않고 앱으로 실행하기로 했다(10/2).
2. 사람이 SW1을 누르는 실행. 10/1 실행은 60초 안에 `payment.result`가 오지 않아 결제가 일어나지 않았다(기기 계정 잔액 변화 없음). 같은 흐름을 버튼 시뮬레이터로 다시 돌리면 통과했으므로, 1초 이상 눌러 길게 누름으로 처리됐거나 대기 시간이 지난 뒤 눌렀을 가능성이 크다. 다음 실행에서 원인이 보이도록 펌웨어가 버튼 동작마다 로그를 남기고, 키오스크 쪽 대기 시간을 120초로 늘렸다. 10/2부터 보드를 쓸 수 없어 재실행은 보드를 다시 쓸 수 있을 때 한다.

## waiver

| 항목 | 내용 | 없어지는 시점 |
|---|---|---|
| non-secure 기기 키 | 키가 Zephyr Secure storage에 있고 secure 쪽 버튼 토큰 경계가 없다 [N30] | 8주차 TF-M(`/ns`) 변형 |
| 개발용 고정 셋업 | 운영자 주소·컨트랙트·chainId를 배포 기록에서 빌드 때 넣는다. TimeAnchor를 페어링하지 않은 링크로 받는다 [N30] | 8주차 BLE 셋업 명령 |
| 평문 키오스크 링크 | 결제 세션 보안 채널 없음 [N30] | 기기 9주차, 키오스크 10–11주차 |
| 폰 확인 화면 없음 | `confirm.show`를 로그로만 남긴다 [N30] | 폰 앱(12주차) |

## 이 과정에서 찾은 결함

- 펌웨어 reassembler가 메시지를 다 모은 뒤 다음 메시지에서 이전 길이를 지우지 않아 두 번째 메시지부터 `BAD_FRAME`이 났다. 고쳤고, 하나의 수신기로 모든 정상 조각 벡터를 연달아 처리하는 host 시험을 더했다(PR #21).
- 사람이 누르는 첫 실행에서 키오스크 쪽이 `device refused: undefined`로 끝났다. 결과가 없을 때와 기기가 거절했을 때를 구분하지 않았기 때문이다. 결과가 없으면 SW1을 짧게 눌렀는지 묻는 오류를 내도록 고쳤다.
- 오래 전에 서명한 TimeAnchor는 기기 시각을 늦춰 `ATTESTATION_EXPIRED`가 된다. anchor는 실행 직전에 발급하고, 키오스크의 만료 여유는 60초로 둔다.

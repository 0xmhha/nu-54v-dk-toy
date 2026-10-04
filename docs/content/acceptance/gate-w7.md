# 7주차 실결제 게이트 기록 (10/5 통과)

[DF-20260925-02](../planning/design-freeze-checkpoint-02.md)의 7주차 실결제 게이트다 [N03]. 조건은 보드 내장 키로 기기 버튼 승인 → 키오스크 제출 → finalized PaymentSettled까지 실기 end-to-end 1건이다. 7주차는 non-secure 키, 개발 빌드 전용 고정 셋업, 평문 키오스크 링크로 통과하며 이 사실을 waiver로 적는다 [N30].

전체 주소와 해시는 저장소 밖 `evidence/w7/`의 원본에 있고, 이 문서는 줄인 값만 쓴다.

## 구성

| 구성 | 이번 실행 |
|---|---|
| 기기 | NU-54V-DK 보드. 개발 실행(10/1)은 펌웨어 0.2.0(main `1a1922b`), 게이트 실행(10/5)은 펌웨어 0.3.0(PR #48 브랜치 `3562dde` 이후, 보안 채널과 LED 표시 포함). 기기 키는 PSA(CRACEN)가 만든 secp256k1 영속 키, 주소 `0xbc3152…f4d8` |
| 기기 계정 | `opsctl rental deposit`로 10 tUSDC 입금(테스트넷) |
| 가맹점 | 키오스크 역할 주소(`0xF92a32…61A4`), payout은 운영자 역할 주소. `opsctl attestation issue`로 발급 |
| TimeAnchor | `opsctl anchor sign`, 실행 직전 finalized 블록 시각 |
| 키오스크 쪽 | 게이트 실행(10/5): Galaxy S25 Ultra(Android 16)의 키오스크 앱(디버그 빌드), 앱의 BLE 모듈(`NusBle`)과 결제 화면, 보안 채널(4.1절) 사용. 개발 실행: Mac에서 `packages/device-sim/scripts/rehearse.ts --transport ble --submit`. 키오스크의 `src/payment/signing.ts`(기기 서명 확인)와 `src/payment/submit.ts`(제출과 판정)를 그대로 실행하고, BLE는 `tools/bringup/pay_bridge.py`가 맡는다. 게이트 실행: 키오스크 앱(Android)의 BLE 모듈과 결제 화면으로 한다(10/2 결정) |
| 승인 | 보드 SW1(7주차 개발 빌드의 임시 배치) |

## 판정 항목

| 조건 | 개발 실행(10/1, 버튼 시뮬레이터, Mac) | 게이트 실행(사람이 SW1, 키오스크 앱) |
|---|---|---|
| 보드 내장 키로 서명 | 충족: 복원 주소가 보드 주소 | 충족: 키오스크가 기기 서명의 서명자를 보드 주소로 확인했고, PaymentSettled의 device가 `0xbc3152…f4d8` |
| 기기 버튼 승인 | 버튼 시뮬레이터(디버거로 SW1 핀 제어) | 충족: 사람이 SW1을 누름(보드 로그 `approved by the button`, 결제 요청 뒤 약 5.5초) |
| 키오스크 제출 | 충족: 키오스크 모듈이 type-2 트랜잭션 전송(Mac) | 충족: 키오스크 앱이 제출, tx `0xfe5d86…b3fd`. 요청부터 결과까지 7.1초 |
| finalized PaymentSettled, device·amount·nonce 일치 | 충족: `0xaf8adb…0b00`, 블록 21181324, 103,820 gas | 충족: 블록 21463022(확인 시 finalized 21463049), merchant `0xf92a32…61a4`, 주문 `0xbada9b…1e71`, 금액 1 tUSDC, nonce가 기기 서명의 nonce와 같음 |

**판정: 통과 (10/5).** 키오스크 Android 앱으로, 사람이 SW1을 눌러, 보드 키 서명 결제 1건이 finalized PaymentSettled까지 끝났다. 원본은 `evidence/w7/2026-10-05-kiosk-app/`(보드 VCOM 로그, 폰 logcat, 영수증, 키오스크 주문 기록, 화면, attestation, anchor, `SHA256SUMS`)에 있다.

## waiver

| 항목 | 내용 | 없어지는 시점 |
|---|---|---|
| non-secure 기기 키 | 키가 Zephyr Secure storage에 있고 secure 쪽 버튼 토큰 경계가 없다 [N30] | 8주차 TF-M(`/ns`) 변형 |
| 개발용 고정 셋업 | 운영자 주소·컨트랙트·chainId를 배포 기록에서 빌드 때 넣는다. TimeAnchor를 페어링하지 않은 링크로 받는다 [N30] | 8주차 BLE 셋업 명령 |
| 평문 키오스크 링크 | 해당 없음(10/5 게이트 실행은 보안 채널 사용). 10/1 개발 실행만 평문 [N30] | 10/5 해소 |
| 폰 확인 화면 없음 | `confirm.show`를 로그로만 남긴다 [N30] | 폰 앱(12주차) |

## 이 과정에서 찾은 결함

- 펌웨어 reassembler가 메시지를 다 모은 뒤 다음 메시지에서 이전 길이를 지우지 않아 두 번째 메시지부터 `BAD_FRAME`이 났다. 고쳤고, 하나의 수신기로 모든 정상 조각 벡터를 연달아 처리하는 host 시험을 더했다(PR #21).
- 사람이 누르는 첫 실행에서 키오스크 쪽이 `device refused: undefined`로 끝났다. 결과가 없을 때와 기기가 거절했을 때를 구분하지 않았기 때문이다. 결과가 없으면 SW1을 짧게 눌렀는지 묻는 오류를 내도록 고쳤다.
- 오래 전에 서명한 TimeAnchor는 기기 시각을 늦춰 `ATTESTATION_EXPIRED`가 된다. anchor는 실행 직전에 발급하고, 키오스크의 만료 여유는 60초로 둔다. 10/5에도 anchor 서명부터 결제까지 약 2분 반이 걸려 같은 거절이 났다. 실행 때는 anchor 서명·전달·결제 요청을 몇 초 안에 이어서 했다. 근본 대책은 별도 과제로 남긴다(기기 시각이 anchor 서명 뒤 흐른 시간만큼 늦는 문제).
- 10/5 첫 실행들은 `TIME_ANCHOR_MISSING`으로 거절됐다. PR #44가 "셋업 세션은 본딩한 링크에서만"을 코어에 넣으면서, 개발 빌드의 예외(키오스크가 페어링 없이 TimeAnchor 전달)를 보드 계층에만 두다 빠뜨렸다. 예외를 코어 규칙으로 옮기고 범위를 좁혔다(PR #48, 세션 벡터 SV-34).
- 키오스크 앱이 실패한 시도에서도 넣어 둔 TimeAnchor를 소진하고, 첫 BLE 연결이 `0x3e`(연결 성립 실패)로 끊길 때 다시 시도하지 않는다. 둘 다 키오스크 앱 결함으로 남긴다.
- 결과 LED가 꺼지지 않아 헷갈렸다. LED 네 개를 하나의 표시등으로 쓰고 단계마다 깜빡임 패턴으로 구분하며, 모든 표시가 꺼짐으로 끝나도록 바꿨다(PR #48).

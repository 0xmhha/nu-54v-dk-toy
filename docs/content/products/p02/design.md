# P02 설계 — 대여자 폰 앱

[DF-20260925-02](../../planning/design-freeze-checkpoint-02.md) 기준 P02 폰 앱의 구조다. 기술 스택은 키오스크와 같이 React Native(TypeScript)와 Kotlin Turbo Module이며 이번 사이클은 Android만 만든다 [N25].

## 1. 구조

| 모듈 | 하는 일 |
|---|---|
| `ble/bond` (Kotlin) | QR의 BLE 주소로 연결, `createBond`와 Passkey Entry에 QR passkey 입력, 본딩 상태 이벤트 [N27] |
| `ble/link` (Kotlin) | 본딩한 링크의 rx write, tx notify 구독, MTU 요청. 본딩하지 않은 링크의 메시지는 버린다 |
| `protocol` (TS, `packages/protocol/ts`) | 조각 재조립, envelope digest, 결정적 CBOR. 키오스크와 같은 코드를 쓴다 |
| `confirm` (TS) | `confirm.show` 검증(스키마), 표시 문자열 생성(EIP-55 주소, 토큰 표 기반 금액), 결과 표시 [N26] |
| `settings` (TS) | 결제 모드, 연결 상태, 한도 표시, PIN 안내 [N28] |
| `qr` (TS) | 라벨 QR 파싱: `nu54://bond?addr=<BLE 주소>&passkey=<6자리>` |

## 2. 확인 화면의 신뢰 규칙

앱은 `confirm.show`의 값만 화면에 그린다 [N26]. 키오스크와 직접 통신하지 않고, 결제 값을 사용자 입력이나 다른 저장소에서 가져오지 않는다. 표시 문자열은 받은 값에서만 만들고, 승인 버튼을 두지 않는다. 이 규칙 때문에 키오스크가 뚫려도 대여자가 보는 내용은 기기가 서명하려는 값과 같다. 이 보장은 기기의 non-secure 코드와 앱이 무결하다는 가정 아래의 주장이다.

금액은 토큰 단위로 소수점 둘째 자리까지 표시하고 셋째 자리 이하는 버린다(예: base unit `5009999`는 `5.00 USDC`) [N31]. 반올림하지 않으므로 화면 금액이 서명 금액보다 커 보이는 일은 없고, 차이는 결제 한 건에 0.01 USDC 미만이다. 이 차이는 컨트랙트 한도 안에서 받아들인다. 버림은 정수 나눗셈(`amount / 10^(decimals-2)`)으로 해서 부동소수점 오차를 피한다. 기호와 소수 자릿수는 앱의 토큰 표(token 주소 → 기호, 소수 자릿수)에서 가져온다. 이번 사이클의 토큰은 시험용 USDC 하나이며, 표에 없는 token은 base unit 정수와 주소를 그대로 보여 준다.

## 3. 상태

```text
NotBonded --QR 읽기와 Passkey Entry 성공--> Bonded
Bonded --연결--> Connected --결제 모드 켜기--> PaymentMode
PaymentMode --confirm.show--> Confirming --결과--> PaymentMode
Connected/PaymentMode --연결 끊김--> Bonded (자동 재연결)
어느 상태든 --본딩 삭제(반납)--> NotBonded
```

## 4. 동시 연결

기기는 폰 앱(본딩)과 키오스크(비본딩)를 동시에 연결한다. 앱은 연결 간격을 요청하지 않고 기기 기본값을 따른다. `confirm.show`는 한 envelope(2048바이트 이하)로 충분하다. 두 연결을 함께 쓰는 결제는 WBS2-P02-04에서 실기로 확인한다.

## 5. 보안

- 앱은 키, 니모닉, PIN을 저장하지 않는다 [N28]. 저장하는 값은 본딩 정보(OS가 관리)와 토큰 표뿐이다.
- 라벨 passkey는 본딩 한 번에만 쓰고 앱에 남기지 않는다 [N27].
- 로그에는 주소와 해시를 줄여 남긴다 [N16].

## 6. 시험 설계

| 대상 | 방법 |
|---|---|
| 표시 문자열 | jest로 PA-01 벡터의 필드에서 만든 문자열을 기대값과 비교 |
| QR 파싱 | jest로 정상·잘못된 QR |
| 본딩과 동시 연결 | 실기: 기기, 폰, 키오스크 태블릿. WBS2-P02-04 |

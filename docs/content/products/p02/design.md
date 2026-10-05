# P02 설계 — 대여자 폰 앱

[DF-20260925-02](../../planning/design-freeze-checkpoint-02.md) 기준 P02 폰 앱의 구조다. 기술 스택은 키오스크와 같이 React Native(TypeScript)와 Kotlin Turbo Module이며 이번 사이클은 Android만 만든다 [N25].

## 1. 구조

| 모듈 | 하는 일 |
|---|---|
| `ble/bond` (Kotlin) | 페어링 모드 기기 스캔(서비스 UUID, LE Limited Discoverable), `createBond`(Just Works 또는 재연결 코드로 Passkey Entry), 본딩 해제, 본딩 상태 이벤트 [N27] |
| `ble/link` (Kotlin) | 본딩한 링크의 rx write, tx notify 구독, MTU 요청. 본딩하지 않은 링크의 메시지는 버린다 |
| `protocol` (TS, `packages/protocol/ts`) | 조각 재조립, envelope digest, 결정적 CBOR(필드 인코딩은 결제 프로토콜 4.2절). 키오스크와 같은 코드를 쓴다 |
| `confirm` (TS) | `confirm.show` 검증(스키마), 표시 문자열 생성(EIP-55 주소, 토큰 표 기반 금액), 결과 표시 [N26] |
| `settings` (TS) | 결제 모드, 연결 상태, 한도 표시, PIN 안내 [N28] |
| `setup` (TS) | 기기·지갑 설정 상태 머신(`flow.ts`), 메시지 대기와 시간 제한(`channel.ts`), 배포 값(`network.ts`) |
| `qr` (TS) | 운영자 도구가 인쇄한 라벨 QR 파싱: `nu54://bond?addr=<BLE 주소>&passkey=<6자리>` |

## 2. 확인 화면의 신뢰 규칙

앱은 `confirm.show`의 값만 화면에 그린다 [N26]. 키오스크와 직접 통신하지 않고, 결제 값을 사용자 입력이나 다른 저장소에서 가져오지 않는다. 표시 문자열은 받은 값에서만 만들고, 승인 버튼을 두지 않는다. 이 규칙 때문에 키오스크가 뚫려도 대여자가 보는 내용은 기기가 서명하려는 값과 같다. 이 보장은 기기의 non-secure 코드와 앱이 무결하다는 가정 아래의 주장이다.

금액은 토큰 단위로 소수점 둘째 자리까지 표시하고 셋째 자리 이하는 버린다(예: base unit `5009999`는 `5.00 USDC`) [N31]. 반올림하지 않으므로 화면 금액이 서명 금액보다 커 보이는 일은 없고, 차이는 결제 한 건에 0.01 USDC 미만이다. 이 차이는 컨트랙트 한도 안에서 받아들인다. 버림은 정수 나눗셈(`amount / 10^(decimals-2)`)으로 해서 부동소수점 오차를 피한다. 기호와 소수 자릿수는 앱의 토큰 표(token 주소 → 기호, 소수 자릿수)에서 가져온다. 이번 사이클의 토큰은 시험용 USDC 하나이며, 표에 없는 token은 base unit 정수와 주소를 그대로 보여 준다.

## 3. 상태

```text
start --등록 기기 없음--> scan --기기 선택--> bonding --> connecting --device.info--+
start --등록 기기 있음--> reconnecting --device.info--------------------------------+
  UNPROVISIONED: setupConfirm(SW1 승인) --> setupPin(기기 버튼 PIN) --> passkey(재연결 코드 표시)
                 --> rebonding(기존 본딩 해제, 코드로 Passkey Entry) --> verify(wallet.check, SW1) --> home
  키 있음, 새 폰: verify --> home
  키 있음, 등록 기기: 지갑 주소가 같으면 home, 다르면 failed(기기 지우기 안내)
home --결제 모드 켜기--> PaymentMode --confirm.show--> Confirming --결과--> home
home --기기 지우기--> scan
어느 단계든 실패 --> failed --처음부터 다시--> start
```

각 단계는 시간 제한이 있다. 기기 버튼 승인은 60초, PIN은 기기가 45초 뒤 `TIMEOUT`을 보내고, 지갑 확인은 기기가 60초 뒤 `TIMEOUT`을 보낸다. 앱은 셋업이 끝나지 않으면 `session.cancel`로 세션을 닫아 기기에 아무것도 남지 않게 한다. 셋업 직후 기기가 새 passkey로 페어링 모드를 스스로 열기 때문에, 대여자는 재연결 코드를 확인한 뒤 버튼을 다시 길게 누르지 않아도 된다.

## 4. 동시 연결

기기는 폰 앱(본딩)과 키오스크(비본딩)를 동시에 연결한다. 앱은 연결 간격을 요청하지 않고 기기 기본값을 따른다. `confirm.show`는 한 envelope(2048바이트 이하)로 충분하다. 두 연결을 함께 쓰는 결제는 WBS2-P02-04에서 실기로 확인한다.

## 5. 보안

- 앱은 키, 니모닉, PIN을 저장하지 않는다 [N28]. 저장하는 값은 본딩 정보(OS가 관리), 등록 기기 목록(BLE 주소, 이름, 지갑 주소), 토큰 표뿐이다.
- passkey(재연결 코드, 라벨 passkey)는 본딩 한 번에만 쓰고 앱에 남기지 않는다 [N27].
- 지갑 확인 서명은 WalletCheck 타입이라 정산 컨트랙트가 받지 않는다. 앱은 서명자를 직접 복원해 keygen 주소와 비교한다 [N04].
- 아직 다루지 않은 위험: 재연결 코드를 잃으면 다른 폰과 본딩할 수 없고(반납 후 reset으로만 회수), 오염된 폰 앱은 `setup.operator`에 다른 운영자 주소를 넣을 수 있다. 분실과 해킹 시나리오는 따로 검토한다.
- 로그에는 주소와 해시를 줄여 남긴다 [N16].

## 6. 시험 설계

| 대상 | 방법 |
|---|---|
| 표시 문자열 | jest로 PA-01 벡터의 필드에서 만든 문자열을 기대값과 비교 |
| QR 파싱 | jest로 정상·잘못된 QR |
| 설정 상태 머신 | jest로 software device와 새 기기 설정, 재시작 재연결, 버튼 거절, PIN 시간 초과, 다른 키 서명 거절, 새 폰 본딩, 기기 지우기, 초기화된 기기 재설정 |
| 본딩과 동시 연결 | 실기: 기기, 폰, 키오스크 태블릿. WBS2-P02-04 |

# P02 · User app

> **DF-20260925-02 기준 (2026-09-28 개정):** 이번 사이클에 대여자 폰 앱으로 만든다. 기기 설정(본딩)과 결제 확인 화면을 맡는다. 범위와 설계는 [기획](../../docs/content/products/p02/plan.md) · [SRS](../../docs/content/products/p02/srs.md) · [유즈케이스](../../docs/content/products/p02/use-cases.md) · [설계](../../docs/content/products/p02/design.md)가 정한다. 아래 원문은 DF-20260920-01 기준이며 충돌하면 위 문서를 따른다.

여행자와 점주가 기기와 지갑 기능을 사용하는 React Native 모바일 앱이다.

- **소유 범위:** Google/Apple 로그인, 계정 수명주기, 기기 등록·설정,
  두 지갑과 자산 선택, 승인·송금·영수증 UI, FOTA 진행 상태
- **주요 경계:** P01과 C1, P03과 C4, P10과 C3, P08/P09와 C8
- **WBS:** `WBS-P02-01`부터 `WBS-P02-04`

설계 진입점:

- [제품 WBS](../../docs/content/planning/product-wbs-overview.md#p02--유저-앱소셜-로그인기기-설정)
- [소셜 로그인과 두 지갑](../../docs/content/specifications/social-wallet-recovery-design.md)
- [화면 흐름](../../docs/content/specifications/screen-flows.md)
- [기기 등록·반납 수용](../../docs/content/specifications/lifecycle-journey-acceptance.md)

## 개발 (DF-20260925-02)

React Native 0.87(TypeScript)과 Kotlin Turbo Module로 만든 Android 앱이다. 키오스크와 같은 구조이며 프로토콜 코드는 `@nu54/protocol`을 같이 쓴다.

| 경로 | 내용 |
|---|---|
| `src/App.tsx` | 라벨로 본딩·연결, 결제 대기, 확인 화면(승인 버튼 없음, "기기 버튼을 눌러 승인하세요"), 결과 화면 |
| `src/confirm/display.ts` | 확인 화면 문자열: EIP-55 주소, 소수점 둘째 자리까지 버린 금액(N31), 토큰 표에 없는 토큰은 base unit과 주소 |
| `src/link/confirmLink.ts` | 본딩한 링크에서만 `confirm.show`와 전달받은 `payment.outcome`을 받아 화면 상태를 바꾼다(P02-FR-02). 결제 모드 켜기·끄기(`device.paymentMode`, 기본 120초, P02-FR-08)를 보내고 기기의 답을 받는다 |
| `src/qr.ts` | 라벨 QR `nu54://bond?addr=<BLE 주소>&passkey=<6자리>` 파싱 |
| `src/specs/NativeRenterBle.ts` | Turbo Module 명세. Kotlin 구현 `android/app/src/main/java/com/nu54renter/ble/RenterBleModule.kt`: 라벨 passkey로 Passkey Entry 본딩, 본딩한 기기에만 연결 |

```bash
make test
cd android && ./gradlew :app:installDebug   # 폰을 USB로 연결
```

아직 없는 것과 시험하지 못한 것은 다음과 같다.
- 라벨 QR은 카메라 대신 텍스트로 넣는다(카메라 라이브러리는 아직 넣지 않았다).
- 결제 모드 켜기·끄기(P02-FR-08)는 프로토콜에 해당 메시지가 없어 만들지 못했다.
- 기기가 폰으로 `confirm.show`와 `payment.outcome`을 보내는 부분은 펌웨어와 소프트웨어 기기 모두 아직 없다(펌웨어는 로그만 남긴다, 7주차 waiver).
- 본딩과 연결은 실기에서 아직 실행하지 않았다.

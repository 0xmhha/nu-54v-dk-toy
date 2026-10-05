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
| `src/App.tsx` | 설정 상태 머신의 화면(기기 찾기, 본딩, 지갑 설정, 재연결 코드, 지갑 확인)과 홈(지갑 주소, 결제 모드, 확인 화면, 결과와 영수증, 등록된 기기) |
| `src/setup/` | `flow.ts` 기기·지갑 설정 상태 머신(P02-FR-09~12): 스캔 → 본딩 → `device.info` → 키 없는 기기면 `setup.operator`·버튼 승인·버튼 PIN → 재연결 코드 → passkey 재본딩 → `wallet.check` 서명 확인 → 홈. 등록 기기가 있으면 다시 연결해 지갑 주소를 확인한다. `channel.ts` 메시지 대기와 시간 제한, `network.ts` 배포 기록의 운영자·컨트랙트·chainId |
| `src/confirm/display.ts` | 확인 화면 문자열: EIP-55 주소, 소수점 둘째 자리까지 버린 금액(N31), 토큰 표에 없는 토큰은 base unit과 주소 |
| `src/link/confirmLink.ts` | 본딩한 링크에서만 `confirm.show`와 전달받은 `payment.outcome`을 받아 화면 상태를 바꾼다(P02-FR-02). 결제 모드 켜기·끄기(`device.paymentMode`, 기본 120초, P02-FR-08)를 보내고 기기의 답을 받는다 |
| `src/App.tsx`의 `Receipt` | 승인된 `payment.outcome`에 실린 디지털 영수증(가맹점, 대표자, 주문 번호, 메뉴·수량·가격, 합계)과 탐색기 링크. 링크는 받은 URL이 아니라 `chainId`와 `txHash`로 앱이 만든다. 형식이 틀린 영수증은 버리고 결과만 보인다 |
| `src/qr.ts` | 운영자 도구가 인쇄한 라벨 QR `nu54://bond?addr=<BLE 주소>&passkey=<6자리>` 파싱 |
| `src/specs/NativeRenterBle.ts` | Turbo Module 명세. Kotlin 구현 `android/app/src/main/java/com/nu54renter/ble/RenterBleModule.kt`: 페어링 모드 기기 스캔, Just Works 또는 passkey 본딩, 본딩 해제, 본딩한 기기에만 연결, SecureRandom, 등록 기기 목록 저장(SharedPreferences) |

```bash
make test
cd android && ./gradlew :app:installDebug   # 폰을 USB로 연결
```

아직 없는 것과 시험하지 못한 것은 다음과 같다.
- 지갑 설정 흐름(스캔부터 지갑 확인까지)은 software device로만 시험했다. 실기는 키 없이 부팅하는 펌웨어 rental 빌드(`python3 scripts/fw.py build --rental`)가 필요하다.
- 재연결 코드 분실과 오염된 폰 앱의 셋업 값은 아직 다루지 않는다(P02 설계 5절).
- 기기가 폰으로 보내는 `confirm.show`, `confirm.limit`, `payment.outcome`과 결제 모드 메시지는 펌웨어·소프트웨어 기기에 들어갔지만, 이 앱과 실기를 함께 돌려 보지는 않았다.
- 본딩과 연결은 실기에서 아직 실행하지 않았다.

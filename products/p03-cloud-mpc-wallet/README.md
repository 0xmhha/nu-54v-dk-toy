# P03 · Cloud MPC wallet

> **DF-20260925-02 (2026-09-25):** 12주 사이클에서 P03은 설계만 다뤘다(out of cycle). 이후 상용 계획 #19의 구현은 별도로 진행한다. [설계 동결 DF-20260925-02](../../docs/content/planning/design-freeze-checkpoint-02.md)

P03은 별도 가입과 별도 동작 승인을 받은 Ethereum Sepolia 시험 지갑을 제공한다. 저장소는 참가자 A, B, C가 각각 관리하며 조정 서버에는 요청, 에포크, 공개 결과 및 확인 증거만 저장한다. 가입·서명·참가자 교체를 각각 다른 요청으로 기록한다. [상용 구현 범위](../../docs/content/commercial/implementation-plan.md) · [제품 요구사항](../../docs/content/commercial/products/p03-cloud-mpc-wallet/prd.md)

| 실행 단위 | 책임 |
| --- | --- |
| `mpc-coordinator` | P03 전용 로그인·인가 검증, PostgreSQL에 요청 예약, A/B/C 결과 대사, 활성 키·세대 및 서명 결과 반환 |
| `mpc-oidc-gateway` | 등록된 외부 OIDC 주체를 P03 전용 로그인과 연계 자격으로 변환 |
| `mpc-participant` | 각 참가자의 독립 서비스. TLS 1.3 상호 인증과 인증서 핀 대조, 생성·서명·재공유 라운드, 암호화한 로컬 share 및 세대 펜스 보관 |
| P02 지갑 선택 | 검증된 시험 프로필로 만든 앱에서 별도 로그인, 요청별 독립 확인, 같은 요청 재조회, 휴대전화 교체, 자격 해제, 지갑 종료 |

## 실행

- `make build`로 P03 실행 단위를 컴파일한다. `make real-test`는 독립 프로세스 6개로 매번 새로운 사전 매개변수와 session nonce를 생성해 DKG, 분산 서명, 같은 공개키를 유지하는 재공유, 교체 뒤 서명을 검증한다. `make db-test`는 임시 PostgreSQL 15 이상에 P03 스키마를 적용해 중복 요청, 일부 ACK, 이전 세대 차단과 마지막 인증수단 해제를 시험한다. 같은 임시 DB와 독립 참가자 9개를 이어서 사용자 API에서 로그인, 가입, 승인, 서명, 같은 주소의 1→2→3 교체 및 각 세대 서명을 실행한다.
- P10 마이그레이션 도구로 P03 스키마를 적용한다. 실행 사용자는 `nu54_p03_service`만 부여한다. 마이그레이션 설정은 `P03_MIGRATION_DATABASE_URL`, 서비스 연결은 `P03_DATABASE_URL`을 사용한다. [저장소 관리](../p10-platform/database/README.md)
- 각 참가자에게 서로 다른 TLS 인증서, 리슨 주소, 본인 전용 `0700` 저장 디렉터리 및 `0600` 32바이트 AES 키 파일을 배정한다. 각 프로세스는 `mpc-participant -config <파일>`로 실행한다. `Certificate`, `CertificateKey`, `ClientCA`, `CoordinatorCertificateSHA256`, `CoordinatorSPKISHA256`, `Vault`, `VaultKey`, `Listen`, `ID`, `Peers`, `CoordinatorKey`, `ApprovalKey`, `RecoveryKey`를 지정한다. 공개키는 Ed25519 원시 32바이트를 base64로 인코딩하고, TLS 핀은 인증서 DER 및 공개키 SPKI DER의 SHA-256 소문자 16진수다.
- `mpc-coordinator -config <파일>`에 API 인증서와 키 경로(`APICertificate`, `APIKey`), 참가자 제어용 인증서·키·CA(`ParticipantCertificate`, `ParticipantKey`, `ParticipantCA`), `Listen`, P03 전용 `IdentityIssuer`, `IdentityKey`, 합성 사용자 한 명의 pairwise UUID `ProfileSubject`, 조정 서버 Ed25519 시드 경로 `SigningSeed`, 각각 독립적인 `ApprovalKey`와 `RecoveryKey`를 지정한다. `First`에는 1세대 A/B/C, `Replacement`에는 교체용으로 구세대와 ID가 겹치지 않는 2세대 A/B/C, `Future`에는 3세대부터 차례로 A/B/C 배열을 추가한다. `Endpoints`에는 ID별 HTTPS `URL`과 `CertificateSHA256`·`SPKISHA256`을 넣는다. 독립 복구 대기는 `RecoveryDelaySeconds`에 60–600초로 지정한다.
- P02에는 서로 다른 HTTPS origin 네 개를 `{"api":"…","identity":"…","approval":"…","recovery":"…"}` 형식으로 지정한다. JSON의 base64를 Android 빌드 속성 `nu54P03Profile` 또는 iOS 빌드 설정 `NU54_P03_PROFILE`에 넣으면 지갑 선택 화면에 시험 프로필이 나타난다. P02는 원문을 재계산하고 서명을 검증한 뒤 서명 거래를 표시한다. 체인 제출은 별도다. [P02 승인·신원 연동 규격](PROTOCOL.md)

## P02 네이티브 참가자 등록

1. `make mobile-android P03_MOBILE_OUT=<절대경로> ANDROID_NDK_CLANG=<절대경로>`로 ARM64 라이브러리를 만든다. P02 Android 빌드 속성 `nu54P03Libs`에는 출력의 `android` 디렉터리를 넣는다.
2. `make mobile-ios-simulator P03_MOBILE_OUT=<절대경로>`로 iOS ARM64 시뮬레이터 아카이브를 만든다. 시험 빌드는 `ARCHS=arm64`, `NU54_P03_NATIVE_LINKED=1` 전처리 정의, `-force_load`로 실제 아카이브를 링크한다. 기기 배포에는 해당 기기용 아카이브를 사용한다.
3. P02의 서명된 앱 빌드 프로필에 `deviceParticipant:true`와 등록 담당기관의 Ed25519 공개키 `rosterAuthority`를 추가한다. 사용자는 로그인 후 **휴대전화 참가자 등록 · 재확인**에서 단말이 만든 공개 CSR을 제출하고 승인 결과를 조회한다. 단말의 TLS 개인키와 share는 단말의 네이티브 암호화 저장소에 남는다.
4. 조정 서버는 `MobileRequests`에 공개 CSR을 예약하고, `MobileApproved`에서 같은 사용자와 CSR에 결합된 서명 명부를 읽는다. 등록 담당자는 별도 CA와 별도 명부 서명키로 `mpc-roster -config <파일>`을 실행하고 같은 인증서 DER 및 SPKI 핀을 해당 위원회와 조정 서버에 등록한다. 승인 파일은 등록 담당자 권한으로 발행하고 조정 서버에 읽기 권한을 부여한다. 명부에는 participant ID, peer와 조정 서버의 인증서 핀, 사용자 UUID와 세대가 들어간다.
5. `mpc-lab-gateway`를 로그인, 동작 승인, 독립 복구용 HTTPS origin에서 별도 키와 별도 자격으로 실행하면 로컬 시험 앱에서 브라우저 확인과 PKCE 결과 조회를 사용할 수 있다. 시험 결과와 승인 경계는 아래 규격을 따른다. [외부 가입·승인 계약](PROTOCOL.md)

연속 교체는 시작 전에 구성한 별도 위원회를 순서대로 사용한다. 직전 위원회는 DB에 활성화할 때 기록한 구성에서 다시 읽고, 이미 활성화에 사용한 참가자 ID는 재사용할 수 없다. 모바일 랩은 직접 접근 가능한 로컬 TLS 경로를 사용한다. 외부망에서 실제 모바일 참가자에 도달하는 네트워크 구성, 실제 소셜 IdP·독립 C 조직·복구 통지, 운영 배포와 하드웨어 보호 인수는 배포 시 별도로 검증한다. [참여자 신뢰 모델](../../docs/content/specifications/wallet-control-recovery-design.md)

### 기기 시험 재현

- 소프트웨어 회귀는 `make db-test`로 실행한다. 임시 PostgreSQL·격리된 참가자·각 권한 게이트웨이의 정상 HTTP 계약과 기존 세대의 펜스를 확인한다.
- Android ARM64 시험은 먼저 공개 `rosterAuthority`가 든 앱을 `nu54LabRunner=com.nu54renter.ble.CloudWalletParticipantLab`, `nu54P03Libs`, `nu54P03Profile` 빌드 속성으로 빌드한다. 앱과 instrumentation APK를 전용 테스트 AVD에 설치한 뒤 `NU54_P03_ANDROID_SERIAL=<AVD>`, `NU54_P03_ANDROID_ROLE=a2`, `NU54_P03_ANDROID_ROSTER_SEED=<별도 권한자의 시험 시드 파일>`을 지정해 `make db-test`를 실행한다. 임시 adb 포워딩으로 공개 CSR과 승인 명부를 교환하고 지분 복구와 재시작 뒤 검증 서명을 확인한다. AVD의 이더리움 네트워크 서명·소셜 계정에는 합성 프로필을 사용한다.
- `make mobile-ios-device P03_MOBILE_OUT=<절대경로>`는 실제 iOS 대상 ARM64 아카이브를 별도로 생성한다. `mobile-ios-simulator`는 iOS Simulator에서만 링크할 아카이브를 만든다.
- 현장 인수는 승인된 실물 Android와 iOS, 독립 장치의 연속 교체, USB 없이 원격 참여자까지 이어지는 라우팅, 소셜 가입 및 W23 부분 인증이 끊긴 뒤의 정상 복귀를 따로 확인한다. 로컬 시험의 역방향 adb 포트는 현장 통신 경로에 포함하지 않는다.

Android 회귀는 에뮬레이터 일련번호별 잠금과 매 실행 별도 시험 subject 저장소를 사용한다. 참가자 종료와 재시작을 확인하고 실제 mTLS 인증 응답이 오기 전에는 절차를 진행하지 않는다. 이전 실행의 앱 데이터를 지울 필요가 없다.

iOS 시뮬레이터 회귀는 `mobile-ios-simulator`로 만든 아카이브와 `NU54_P03_NATIVE_LINKED=1`, `NU54_P03_SIMULATOR_LAB=1`을 사용해 전용 `org.nu54.p03fixture` 번들을 별도 DerivedData에 빌드한다. `NU54_P03_PROFILE`에는 시험 명부의 공개키를 포함한 Base64 인코딩 JSON 앱 프로필을 지정한다. Xcode에서 `CODE_SIGNING_ALLOWED=YES`, `CODE_SIGN_IDENTITY=-`, `CODE_SIGN_STYLE=Automatic`, 빈 `DEVELOPMENT_TEAM`과 `PROVISIONING_PROFILE_SPECIFIER`를 지정해 시뮬레이터의 기본 번들별 Keychain 권한을 포함한다. 이 시험 번들만 지정한 시뮬레이터에 설치한 뒤 `NU54_P03_IOS_SIMULATOR`, `NU54_P03_IOS_ROLE=a2`, `NU54_P03_IOS_BUNDLE=org.nu54.p03fixture`, `NU54_P03_IOS_PROFILE_B64`, `NU54_P03_IOS_ROSTER_SEED`로 `make db-test`를 실행한다. P08 배포를 함께 검증할 때는 P08 fixture가 살아 있는 동안 번들을 `org.nu54.p03marketfixture`로 빌드하고, 일치하는 번들 식별자와 `P03_TEST_MARKET_PROFILE=$P08_PROFILE`를 지정한다. 시뮬레이터 전용 시작 코드는 기기 대상과 일반 앱 빌드에서 제외된다. 실행은 앱의 실제 Keychain·Go 엔진·정지 API를 호출하고 그 전용 앱과 시험 데이터에만 접근한다.

### 계정별 참가자 배치

공용 조정 서버는 `ProfilesDirectory`와 독립 프로필 발급자의 Ed25519 공개키 `ProfilesAuthority`를 한 번 설정한다. 새 고객을 위한 A/B/C 프로세스와 인증된 TLS origin을 준비한 뒤 발급 담당자가 아래 명령으로 해당 고객용 서명 프로필을 발행한다. 조정 서버는 고객의 정상 P02 가입 요청 때 새 파일을 읽고 PostgreSQL에 고정한다. 고객을 추가해도 이미 실행 중인 공용 조정 서버의 설정 수정이나 재기동이 필요하지 않다.

```sh
mpc-roster -profile-spec account-public.json -profile-seed /path/to/separate-authority.seed -profile-out /path/to/approved-profiles
```

공개 계정 프로필에는 `Issuer`, 해당 issuer가 사용한 pairwise UUID `Subject`, 별도 `ApprovalKey`와 `RecoveryKey`, 세대별 `Committees`, `Endpoints`, `DelaySeconds`, 선택적 복구 운영자 UUID `Operator`가 필요하다. 참가자 ID, 정규화한 HTTPS origin, TLS 인증서 및 SPKI 핀은 다른 등록 계정과 공유할 수 없다. 발급 시점에 사용할 교체 위원회를 승인한다. 참가자 저장소는 각 A/B/C 배치에 따로 둔다. 원래 `ProfileSubject` 설정은 기존 지갑의 정족수 및 진행 요청을 계속 지원한다.

`NU54_P03_MULTI=1 make db-test`는 두 issuer 주체가 동일한 요청 번호를 각자 재개하고, 서로 다른 참가자 집합에서 서로 다른 주소와 검증된 서명을 생성하는지 확인한다. 타 계정의 조회·승인·복구 증명·운영자 접근을 거절하고, 첫 번째 계정이 주소를 유지한 채 다음 세대로 교체되는지 검증한다. 이 회귀는 P03의 실제 브라우저 확인 HTTP 계약을 구동한다. 실물 단말에서 두 계정을 이어서 사용하는 현장 인수는 별도다.

정상 P02의 Safari 절차는 전용 iOS 시뮬레이터와 별도 DerivedData에서 재현한다. Xcode, P02 CocoaPods, Go, PostgreSQL 15 이상이 필요하다.

```sh
POSTGRES_BIN=/path/to/postgresql/bin products/p03-cloud-mpc-wallet/test/run_ios_browser.sh <전용-iOS-시뮬레이터-UUID> /tmp/nu54-p03-browser-app
```

시험은 임시 DB와 로그인·승인·복구용 개별 HTTPS origin을 시작한다. 시뮬레이터 전용 신뢰 저장소에 한 시간 동안 유효한 시험 CA를 추가한다. P02 Release의 클라우드 지갑에서 로그인과 요청 승인을 실제 Safari로 열고, 두 계정이 각각 별도 자격으로 가입해 다른 주소를 만들고, Sepolia 시험 전송 원문을 별도로 승인해 앱에서 검증된 서명 영수증을 받는지 확인한다. 첫 계정에 다시 로그인하면 첫 계정의 주소만 표시돼야 한다. 임시 DB에서도 두 계정의 공개키와 서명 완료 상태를 비교한다. 이 시험은 온체인 전송을 발생시키지 않는다. 참가자 A/B/C는 호스트의 독립 프로세스에서 실행한다. 단말 A의 Keychain·재공유 검증은 위의 전용 네이티브 시험으로 수행한다. 이 명령은 다른 번들의 앱과 기존 키체인을 지우지 않는다.

`NU54_P03_BROWSER_RECOVERY=1`은 전용 시뮬레이터에서 새 테스트 번들 ID로 W22·W23의 교체와 종료 절차를 실행한다. Safari와 앱의 왕복을 포함하도록 테스트 실행의 허용 시간을 늘린다. 결과 폴더의 `summary.json`은 아홉 단계의 순서, 단일 XCTest 결과와 PostgreSQL 확인 여부를 기록한다.

### W22 독립 복구 거절과 재심사

W21 서명 거절을 사용하려면 독립 승인 게이트웨이의 `RefusalURL`을 조정 서버의 `/v1/sign/refusals` HTTPS 주소로 지정하고 `RefusalServerCA`, `RefusalClientCertificate`, `RefusalClientKey`를 제공한다. 단일 계정 게이트웨이는 지갑과 결합된 승인 계정의 `Subject`를 설정한다. 조정 서버 `RefusalCA`에 전용 클라이언트 발급 CA를 지정한다. 기존 `RecoveryRefusalCA` 설정도 동일하게 동작한다. 이관 006을 먼저 적용한다. 조정 서버와 P02 앱이 새 상태를 처리할 수 있게 배포한 뒤 승인 게이트웨이에 새 거절 주소를 설정한다.

실제 iOS Safari 회귀는 전용 시뮬레이터에서 `NU54_P03_BROWSER_SIGN_REFUSAL=1 test/run_ios_browser.sh SIMULATOR_UUID APP_DERIVED_DATA_DIRECTORY`로 실행한다. 합성 원문 거절, 중복 접수, 앱 재시작 뒤 원요청 조회, 독립 승인을 받은 별도 요청의 검증된 서명을 확인한다.

독립 복구 게이트웨이는 기존 복구 Ed25519 키와 별도로 발급한 전용 TLS 클라이언트 자격으로 조정 서버에 거절을 제출한다. 게이트웨이에 `RefusalURL`(정확한 `/v1/recovery/refusals` HTTPS URL), `RefusalServerCA`, `RefusalClientCertificate`, `RefusalClientKey`를 설정한다. 조정 서버의 `RecoveryRefusalCA`에는 해당 전용 TLS 클라이언트 발급 CA를 지정한다. 일반 P02 읽기·요청은 기존 로그인 인증을 사용한다.

거절을 커밋하면 이전 세대를 동결 상태로 유지하고 해당 요청 후보의 활성화와 서명을 차단한다. 브라우저와 앱을 끈 뒤에도 같은 UUID를 다시 조회하거나 **복구 거절 기록 조회**에서 영수증을 열 수 있다. 원래 계정의 지정 복구 운영자가 `POST /v1/recovery/reviews`에서 명시적으로 한 번 허가하면 사용자가 새로운 UUID로 연결된 재심사를 시작할 수 있다. 기존 거절은 이력에 남고 신규 독립 승인, 대기 시간, 펜스 및 준비 ACK가 모두 적용된다.

```sh
NU54_P03_BROWSER_RECOVERY=1 NU54_P03_BROWSER_PARTIAL=1 POSTGRES_BIN=/path/to/postgresql/bin \
  products/p03-cloud-mpc-wallet/test/run_ios_browser.sh SIMULATOR_UUID APP_DERIVED_DATA
NU54_P03_BROWSER_RECOVERY=1 NU54_P03_BROWSER_REFUSAL=1 POSTGRES_BIN=/path/to/postgresql/bin \
  products/p03-cloud-mpc-wallet/test/run_ios_browser.sh SIMULATOR_UUID APP_DERIVED_DATA
```

첫 실행은 실제 P02가 일부 ACK(4/5)에서 같은 요청을 재조회하며 서명 동결을 유지하고, 마지막 응답 후 같은 주소를 2세대로 활성화하는지 검증한다. 두 번째 실행은 Safari의 독립 거절, 임시 PostgreSQL 커밋, 앱 재시작과 같은 영수증 조회, 별도의 운영자 허가 및 새 요청까지 확인한다. 이 로컬 러너는 조정 서비스의 HTTP 핸들러와 복구 realm을 다시 만든다. 실제 독립 기관과 기기에서의 검증은 현장 인수 범위다.

로컬 확인 기준은 iOS 27 시뮬레이터, Xcode 27.0 (27A266a), PostgreSQL 15.12, Go 1.26.11, Node.js 22.23.5다. 독립 거절 시나리오는 XCTest 1건과 실제 참여자 연동 1건을 통과했다(각각 210.145초, 241.79초). 동일 영수증 재조회, 구세대 동결, 활성화·서명 완료 0건과 별도 운영자의 신규 심사 1건을 확인했다. 정리 후 남은 숫자 요약의 해시는 `sha256:53b6a10f15cea74592d38b2890d5848ef50da7f0feddf646bbc6b330a0d7150b`다. PostgreSQL의 별도 회귀는 이전 증명 재사용과 정족수 노출 후 재심사, 보류된 재심사에서의 별도 ID 시작을 차단하고 새 독립 증거로만 후보를 활성화했다.

별도 부분 ACK 시나리오는 XCTest 1건을 275.022초, 실제 연동을 312.76초에 통과했다. 첫 탭에 Safari가 열렸다. 같은 요청에서 4/5일 때 서명 버튼과 새 세대가 보이지 않았고, 5/5가 된 뒤 같은 공개키와 수신 주소를 확인했다. DB에는 새 준비 ACK 3개, 구세대 펜스 ACK 2개, 구세대 은퇴와 새 세대 활성화, 성공 서명 0건이 기록됐다. 정리한 요약의 해시는 `sha256:3b488639f9b517050ea041a73c4244ab13efb27d0be2a5a3ffc06e05c2d1e319`다.

### W22·W23 접수 응답 복구

P02는 동결과 종료의 요청 번호를 서비스 호출 전에 지갑·realm·인증 주체 범위의 로컬 저널에 저장한다. 동결 접수의 응답이 없으면 서버 확인 대기를 보여 준다. 재로그인 뒤 같은 번호를 조회해 동결, 참여자 응답 수 또는 서명된 독립 거절을 확인한다. 서버에 기록이 없다는 응답을 받으면 같은 번호만 다시 제출할 수 있다.

종료는 Safari에서 지갑, 세대, 종료 요청 번호를 확인한 뒤 별도 승인을 받는다. 종료 커밋이 서명을 차단하고 종료 영수증을 한 번 저장한다. 참여자가 마지막 세대를 모두 펜스할 때까지 영수증은 처리 대기다. 기존 인증수단으로 재로그인하면 같은 참조와 자산 이동 0건을 조회할 수 있다. 로컬 저널에는 독립 승인 증명이나 세션 토큰이 들어가지 않는다.

```sh
NU54_P03_BROWSER_RECOVERY=1 NU54_P03_BROWSER_RESUME=1 POSTGRES_BIN=/path/to/postgresql/bin \
  products/p03-cloud-mpc-wallet/test/run_ios_browser.sh SIMULATOR_UUID APP_DERIVED_DATA
```

이 모드는 동결·종료 DB 커밋이 성공한 뒤 시험 HTTPS 응답만 실패하도록 만든다. 일반 P02·Safari에서 앱을 재시작하고 재인증한 뒤 같은 요청을 확인한다. 종료 시 한 참가자 응답을 늦추고 처리 대기에서 완료로 이동하는지를 검사한다. 격리 PostgreSQL과 실제 로컬 mTLS 참가자를 사용하며 마지막 공개키를 보존한다. 실제 소셜 제공자, 외부 체인 전송과 실물 기기는 사용하지 않는다.

로컬 시험에서 XCTest 1건과 실제 참가자·DB 연동을 통과했다(각각 451.162초, 493.5초). 별도 등록 자격으로 재로그인한 뒤 같은 동결 요청을 확인했다. 종료 응답 유실 뒤에도 원래 종료 영수증을 다시 열었으며, 마지막 참가자 펜스가 돌아온 뒤에만 완료를 표시했다. PostgreSQL에서 각 동결·종료 감사 사건 1건과 종료 영수증 1건을 확인했다. 자산 이동은 0건이다. 정리한 숫자 요약의 해시는 `sha256:44c8c6af49e991f817103b316a8ffa8696c8c214d27b0adfcfccb75463a0db5f`다.

종료 응답은 요청을 시작한 인증 세션에만 적용한다. 조회나 접수 응답이 늦게 도착해도 이미 만료되거나 교체된 세션의 화면, 다음 사용자의 참가자, 저널을 변경하지 않는다. 종료 접수 저널은 해당 소유자가 다시 인증해 같은 영수증을 조회할 때 사용한다.

### W24 복구 운영자 콘솔

운영자 콘솔은 개인 지갑 앱과 별도의 HTTPS 호스트에서 사용한다. 조정 서비스 설정에 `OperatorConsole`의 `Listen`, `Origin`, `Certificate`, `Key`와 필요한 경우 별도 realm의 신뢰 루트 파일 `RealmCA`를 지정한다. 콘솔과 독립 운영자 realm은 포트뿐 아니라 호스트도 분리한다. 기존 `RecoveryOperatorIssuer`, `RecoveryOperatorKey`와 별도로 `RecoveryOperatorGrants`에 관리자만 갱신할 수 있는 권한 파일을 지정한다. 같은 디렉터리에서 새 파일을 작성하고 원자적으로 교체한다. 읽기 실패, 삭제, 불일치와 만료는 권한을 거절한다.

권한 파일의 객체 키는 `nu54/p03/recovery-ops/v1` UTF-8 바이트, NUL, 현재 운영자 주체 UUID, NUL, 현재 자격 UUID를 이어 붙여 SHA-256으로 계산한 소문자 16진수다. 값은 `subject`, `credential`, `expires`(RFC 3339)다. 서명된 지갑 프로필의 운영자 배정도 현재 값과 대조한다. 일반 지갑 로그인과 승인 자격은 콘솔을 열 수 없다. 인증은 별도 realm의 PKCE 확인을 사용하며 콘솔 재시작 후 다시 인증한다.

참여자 서비스 화면은 현재 활성 세대의 A/B/C에 읽기 전용 mTLS 조회를 보내 응답 확인 여부를 표시한다. 장애 조회는 동결이나 서명을 요청하지 않으며 A+C 우회는 사용할 수 없다. 운영자가 정족수 유출 확인란과 신고 버튼을 직접 선택하면 기존 조정 API가 새 서명을 동결한다. 응답을 잃어도 같은 지갑의 사고 참조·감사 기록을 재조회할 수 있다. 과거 키의 외부 사용 위험은 계속 표시하며 자산은 자동 이동하지 않는다. 감사에는 지갑 범위로 파생한 참조, 시각, 사건 코드와 저장된 증거 digest만 표시한다.

로컬 인수에는 PostgreSQL 15 이상, Node, Playwright 모듈과 Chrome이 필요하다. 합성 자격으로 실제 Chrome을 열고 참가자와 격리 PostgreSQL을 연결한다.

```sh
export POSTGRES_BIN=/path/to/postgresql/bin NODE_PATH=/path/to/pinned/playwright/node_modules
export NU54_P03_BROWSER_LAB="$(mktemp -d)" NU54_P03_OPERATOR_BROWSER=1 NU54_P03_MULTI=1
make -C products/p03-cloud-mpc-wallet db-test
```

격리 재현에서 Chrome 브라우저와 PostgreSQL 15, 실제 로컬 mTLS 참여자를 사용했다. 운영자 인증과 재인증, B 응답 미확인에서 회복, 다른 지갑 거절, 응답 유실 뒤 한 건의 동결, 서명과 자산 이동 0건을 확인했다. 개발자 실행은 79.43초였고 독립 실행은 70.33초였다. 정리한 요약의 해시는 `sha256:f0f35a0058290a1ac1d496a7d2c55074efb0e7a811d4af05943fcdae0c74459b`다.

## W20 등록된 OIDC 로그인

`mpc-oidc-gateway -config <파일>`은 사용자 로그인 origin에 배포한다. 별도 승인·독립 복구·명부에는 기존에 등록한 독립 수단과 키를 사용한다. 클라우드 앱 프로필이 탑재된 경우 지갑 선택에서 W20에 들어가면 네트워크, MPC 참여자, 독립 사용자 증명과 주소 활성 조건을 표시한다. 인증 요청은 제공자 버튼을 누른 뒤 시작한다.

- 앱에 서명해서 배포하는 공개 프로필에 `onboarding`을 추가한다. `network`는 `sepolia-native-test-v1` 또는 `isolated-market-8283-v1`이고, `providers`는 `{"id":"…","name":"…","issuer":"https://…"}` 목록이다. 일반 지갑 선택은 Sepolia 프로필, 격리 시장 지갑 선택은 StableNet 학습 프로필에 해당한다. 선택 후 앱은 조정 서버의 `GET /v1/onboarding`에서 지원 프로필과 로그인 origin을 맞춰 보고, 로그인 서버가 선택한 제공자 정보를 그대로 확인한 경우에만 브라우저를 연다. 입력과 응답에는 로그인 사용자 정보가 없다. 기존 네 origin 시험 프로필은 기존 시험 인증 계약을 사용한다.
- 로그인 게이트웨이 설정에 `Origin`, `Listen`, `Certificate`, `CertificateKey`, `Seed`, `LookupKey`, `Network`, `Providers`, `Bindings`을 지정한다. `Seed`와 `LookupKey`는 각각 다른 32바이트 비공개 파일이다. `Seed`의 공개키 및 `Origin`을 조정 서버의 `IdentityKey`와 `IdentityIssuer`, 각 승인 프로필의 `Issuer`와 맞춘다. 각 공급자 설정에는 같은 공개 `Identity`, 등록된 `ClientID`, `ClientSecretFile`, `ResponseMode`, `PKCE`를 넣는다. 제공자 콘솔에 정확한 공개 HTTPS URI `<Origin>/p03/oidc/callback/<provider-id>`를 등록한다. 이 ID는 앱의 공개 프로필과 게이트웨이 설정의 `Identity.id`를 사용한다.
- 게이트웨이의 비공개 설정 JSON, `ClientSecretFile`, `LookupKey`, `Seed`는 실행 계정 소유의 일반 파일로 배포한다. 쓰기가 필요한 배포 파일은 `0600`, 읽기 전용 시크릿 마운트는 `0400`으로 설정한다. 전환 시 기존 파일의 소유자와 권한을 수정한 뒤 재시작한다. 갱신은 비공개 디렉터리 안에서 원하는 권한으로 새 파일을 만든 뒤 원자적으로 교체한다. 게이트웨이는 위 네 입력을 연 파일에서 일반 파일 여부와 그룹·기타 권한 비트가 모두 0인지 검사한다. 시크릿 마운트의 링크에는 열린 대상 파일의 모드가 적용된다. 파일 소유 UID와 실행 계정의 일치는 배포자가 확인한다. TLS 리스너의 `CertificateKey`는 Go TLS가 별도로 읽으므로 배포자가 해당 키에도 비공개 권한을 설정한다. 앱에 탑재하는 공개 프로필은 기존 배포 방식을 따른다.
- Google 웹 클라이언트는 `https://accounts.google.com`, `ResponseMode: "query"`, `PKCE: true`를 사용한다. Apple 웹 Services ID는 `https://appleid.apple.com`, `ResponseMode: "form_post"`, `PKCE: false`를 사용한다. Apple discovery는 S256을 광고하지 않아 이 경로에서 제공자 쪽 PKCE를 요구하지 않는다. Apple의 `ClientSecretFile`에는 운영 환경에서 발급한 ES256 클라이언트 JWT를 넣고 만료 전에 원자적으로 교체한다. 게이트웨이는 파일을 각 코드 교환 때 읽는다. 이 경로는 Apple 서명키 발급이나 JWT 자동 재발급과 별개다. 두 제공자에 모두 `openid` 범위만 요청한다. [Google OIDC](https://developers.google.com/identity/openid-connect/openid-connect) · [Apple OIDC discovery](https://appleid.apple.com/.well-known/openid-configuration) · [Apple 웹 인증 요청](https://developer.apple.com/documentation/signinwithapplerestapi/request-an-authorization-to-the-sign-in-with-apple-server) · [Apple 토큰 교환](https://developer.apple.com/documentation/SigninwithAppleRESTAPI/Generate-and-validate-tokens)
- 명부 담당자는 사전 등록 절차에서 검증한 upstream 주체를 표준 입력으로 `mpc-oidc-gateway -config <파일> -subject-digest <provider-id>`에 전달한다. `Bindings` 항목에 `Provider`, 출력한 `SubjectDigest`, 승인한 P03 pairwise UUID `Subject`, 수단별 pairwise UUID `Credential`을 넣는다. 이름·이메일·소셜 UID 원문 또는 제공자 토큰은 P03 DB나 P02에 저장하지 않는다. 앱에서 사용자가 명시적으로 계정 연계를 선택해야 등록 API를 호출한다. 생성 승인과 주소 활성은 이후 DKG 흐름을 그대로 따른다.
- Google Console과 Apple Services ID에서 새 제공자별 callback을 먼저 등록한다. 단일 제공자가 공용 callback으로 이미 운영 중이면 설정에 `LegacyCallback: true`를 명시해 기존 등록을 한시적으로 사용한다. 새 URI 등록을 확인한 뒤 해당 플래그를 끄고 배포하고, 각 실제 provider 로그인을 확인한 뒤 구 URI를 provider 콘솔에서 폐기한다. 다중 제공자 구성은 이 플래그가 켜져 있으면 시작할 수 없으며 공용 경로에 fallback하지 않는다. 게이트웨이 재시작 때 브라우저 로그인을 완료하지 못한 사용자는 로그인을 다시 시작한다.
- 콜백은 경로에 연결된 제공자와 시작할 때 선택한 제공자가 같아야 한다. Google과 authorization-response issuer를 광고한 제공자는 토큰 교환 전에 정확히 하나의 `iss`를 대조한다. Apple처럼 이를 광고하지 않는 제공자는 등록된 전용 경로로 구분한다. Google은 GET, Apple은 `form_post` POST만 수락한다. [RFC 9700 §4.4.2](https://www.rfc-editor.org/rfc/rfc9700.html#section-4.4.2) · [RFC 9207 §2.4](https://www.rfc-editor.org/rfc/rfc9207.html#section-2.4)
- OIDC JWKS는 HTTPS discovery에서 조회하고 캐시된 키로 서명을 검증할 수 없을 때 원격 키 집합을 다시 조회한다. 코드 교환 결과에서 RS256 서명, 정확한 issuer와 단일 audience, 발급·만료 시각, 요청의 nonce, 제공자가 포함한 azp 및 at_hash를 검증한다. state와 브라우저 쿠키를 한 번만 소비하며 앱은 별도의 S256 기기 검증값으로만 P03 로그인 결과를 받는다. 게이트웨이 재시작 후 미완료 로그인은 처음부터 다시 시작한다. [OpenID Connect Core](https://openid.net/specs/openid-connect-core-1_0.html#IDTokenValidation)
- `go test ./internal/gateway -run TestRegisteredOIDCLogin -count=1`은 로컬 TLS 제공자로 코드 교환과 P03 로그인 토큰을 실행한다. Google과 Apple의 실제 제공자 계정, 공개 callback 및 운영 자격으로 수행하는 인수 시험은 별도다.

등록된 공급자 경로를 실제 iOS 앱과 Safari에서 재현하려면 전용 시뮬레이터에서 다음을 실행한다. 시험은 로컬 TLS OIDC 공급자를 선택하고 인증 후 P02로 복귀해 계정 연계, 독립 증명 전 활성 주소 0개, DKG 활성 및 앱 재시작 뒤 같은 주소를 확인한다. 임시 PostgreSQL의 완료 DKG 1건과 서명 요청 0건을 대조한다.

```sh
NU54_P03_BROWSER_OIDC=1 POSTGRES_BIN=/path/to/postgresql/bin \
  products/p03-cloud-mpc-wallet/test/run_ios_browser.sh SIMULATOR_UUID APP_DERIVED_DATA
```

전용 iOS 27 시뮬레이터의 Release 앱·Safari 시험은 109.664초, 격리 PostgreSQL·MPC 대사는 185.84초에 통과했다. 신규 W20 진입과 복귀, 로컬 TLS 인증, 독립 증명 전 주소 0개, 활성 주소, 재실행 뒤 동일 주소를 확인했다. 정리한 숫자 요약의 해시는 `sha256:bad099dca46a4744489540040346de873dac56e6794199d2d4203ac45d49a98c`다.

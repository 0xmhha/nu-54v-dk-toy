# Android 전이 의존성 재검증

P02와 P04의 React Native·Fresco HTTP 의존성이 선택하던 OkHttp 4.9.2와 Okio 2.9.0을 확인했다. 두 앱은 같은 Gradle 제약을 사용해 OkHttp/URLConnection 4.12.0과 Okio 모듈·JVM 구현 3.6.0을 선택한다. OkHttp 4.12.0의 Maven POM은 Okio 3.6.0을 선언한다. [Okio 공급자 수정](https://github.com/square/okio/commit/81bce1a30af244550b0324597720e4799281da7b) · [OkHttp POM](https://central.sonatype.com/artifact/com.squareup.okhttp3/okhttp/4.12.0)

[증거 레코드](android-runtime-evidence.json)는 두 앱의 실제 custom APK·AAB SHA-256과 임시 서명 인증서, DEX의 Okio·OkHttp 클래스 확인, custom/release Gradle 해석 결과, 동일 Trivy 데이터베이스 재검사 결과를 묶는다. 이전 86개 모듈의 custom 그래프에서 두 앱 모두 CVE-2023-3635가 검출됐다. 변경한 두 타입은 앱마다 87개 모듈을 해석했고 같은 스캔에서 취약점 결과가 없었다. 검사 범위는 해석한 Gradle runtime 좌표다. 실제 실기 도달성·설치, 공식 인증서, APK 내 다른 생태계의 전수 검사는 별도 증거가 필요하다.

```sh
NU54_ANDROID_EVIDENCE_DIR=/tmp/p02-android-evidence bash scripts/android/test_release.sh p02
NU54_ANDROID_EVIDENCE_DIR=/tmp/p04-android-evidence bash scripts/android/test_release.sh p04
```

검증은 공식 자격이 없는 빌드 거절, custom APK/AAB 서명, 디버그 서명 거절 및 손상 AAB 거절을 포함한다. 산출물 지문과 해석 그래프를 같은 시험 실행에서 기록한다. 원본 스캔은 `trivy fs --skip-db-update --offline-scan --scanners vuln --list-all-pkgs --format json`으로 각 해석 lockfile에 적용한다.

[P02 개인 관리 TLS 추가 이후 기록](p02-owner-runtime-evidence.json)은 별도 검증이다. P02의 custom/release 각 88개 해석 좌표에는 `org.conscrypt:conscrypt-android:2.7.0`이 추가됐다. 같은 Trivy DB로 스캔한 두 그래프에 검출 항목이 없었다. 기록에 포함된 APK와 AAB는 로컬 임시 키로 서명한 custom 빌드다. 독립 TLS 및 Android 에뮬레이터 검증 범위는 [P02 모바일 검증](../../../p02-user-app/MOBILE-V2.md)을 참고한다.

## PSBT 파일 선택기 추가 후 런타임 재검사

11월 21일에 P02·P04의 custom 및 release 네 가지 런타임 그래프를 다시 해석했다. P02는 기존 소유자 지갑 그래프에 `com.google.zxing:core:3.5.4`가 추가돼 89개, P04는 기존 그래프에 Conscrypt 2.7.0이 추가돼 88개였다. Android QR 경로는 ZXing JAR의 SHA-256을 빌드 전에 대조한다. 공급자의 릴리스와 Maven POM에서 버전과 Apache 2.0 라이선스를 확인했다. [빌드 검증](../../../p02-user-app/android/app/build.gradle) · [ZXing 공급자 릴리스](https://github.com/zxing/zxing/releases/tag/zxing-3.5.4) · [Maven Central](https://central.sonatype.com/artifact/com.google.zxing/core/3.5.4)

네 그래프에서 중복을 제거한 89개 공개 좌표를 OSV Maven에 조회했다. 당시 응답에서 보고된 취약점은 0건이었다. 재현 시 각 기존 그래프에 기록된 추가·제거 좌표를 적용하고 정렬해서 새 SHA-256을 대조할 수 있다. 이 결과의 범위는 Maven 런타임 의존성이다. APK 서명 및 iOS·JavaScript 의존성은 해당 배포 후보의 별도 증거로 검증한다. [좌표 차이와 결과 지문](psbt-runtime-scan.json) · [OSV 일괄 질의 명세](https://google.github.io/osv.dev/post-v1-querybatch/)

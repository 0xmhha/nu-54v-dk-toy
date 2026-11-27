# 법적 고지 원고

> 검토받지 않은 초안입니다. 법률 자문이 아닙니다.

두 휴대전화 앱이 보여 주는 문서의 원고다. `user-app/`은 NU Wallet, `kiosk/`는 매장 기기 앱이고, 문서마다 한국어와 영어가 있다. 앱에는 `scripts/legal_notices.py`가 만든 `src/legal/notices.generated.json`이 들어간다.

| 바꿀 것 | 방법 |
|---|---|
| 책임 주체, 연락처, 보관 기간 같은 빈칸 | `publisher.json`의 `null`을 글로 채운다. 언어마다 다르면 `{"ko": "...", "en": "..."}`로 쓴다 |
| 문서의 상태 | 검토가 끝난 문서의 머리글에서 `status: final`, `effective: YYYY-MM-DD`, 새 `edition`을 적는다. 한국어와 영어가 같아야 한다 |
| 문서의 내용 | `## ` 제목 아래 글을 고친다. 내용이 바뀌면 `edition`도 바꾼다. 앱은 새 판에 다시 동의를 받는다 |
| 라이선스 | JavaScript 패키지는 `node_modules`에서 읽는다. Android 모듈과 앱에 넣은 네이티브 코드는 `scripts/legal-licenses/native.json`에 있다 |

고친 뒤 `python3 -B scripts/legal_notices.py`를 실행한다. `--blanks`는 아직 정하지 않은 것을, `--check`는 만든 파일이 최신인지를 알려 준다.

공식 배포 빌드(Android `release`, iOS `official`)는 빈칸이나 초안이 남아 있거나 만든 파일이 오래되면 실패한다. `debug`와 `custom` 빌드, 시뮬레이터 빌드는 막지 않는다.

Android 의존성을 바꾸면 `NU54_SCAN_VARIANT=releaseRuntimeClasspath NU54_SCAN_LOCK=<파일> ./gradlew -I ../../../scripts/android/runtime_graph.init.gradle :app:nu54PrintResolvedRuntime`으로 모듈 목록을 만들고 `python3 -B scripts/legal_notices.py --refresh-android --app <앱> --resolved-runtime <파일>`로 기록한다.

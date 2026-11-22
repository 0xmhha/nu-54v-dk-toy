# 제품 산출물의 출처와 서명 · 03C

[출시·보안 기준](../../../docs/content/commercial/security-and-release.md)

## CI 입력

[CI 잠금 파일](../../../.github/inputs.json)은 GitHub Actions commit SHA, 빌드 도구 버전, Nordic 실행 파일, Gradle wrapper JAR·distribution, bundletool의 SHA-256을 기록한다. [CI](../../../.github/workflows/ci.yml)는 런타임 버전과 다운로드 지문을 빌드 전에 검사한다. 펌웨어 검증은 해석된 west manifest와 도구·산출물 지문을 남긴다. 호스팅된 runner의 OS 패치 전체와 SDK 공급자 전송 전체의 digest는 별도 범위다.

PR은 Android 커스텀 테스트 키 서명, 펌웨어 cross-build 및 임시 커스텀 키로 서명한 이미지, iOS simulator를 검증한다. 후보 산출물의 운영 승격, 공식 키 서명과 Apple 기기 설치는 별도 승인과 증거가 필요하다. [Android runtime 재검사](../patches/android/README.md)

## 독립 소비 SDK 후보

```sh
pnpm install --frozen-lockfile
make -C products/p10-platform release-test
make -C products/p10-platform sdk-candidates SDK_OUT=/tmp/nu54-sdk-candidates
```

출력 경로는 checkout 밖의 새 폴더로 지정한다. 다른 작업과 동시에 빌드할 때는 `python3 products/p10-platform/release/capture_sdk.py /tmp/nu54-sdk-source`로 선택한 자료를 별도 로컬 checkout에 복제한 뒤 그 위치에서 설치와 빌드를 실행한다. 빌드는 프로토콜 스키마, EIP-712, CBOR, 프레임, 계약 ABI를 검증한다. Git 커밋·clean 여부와 실제 사용 도구 및 입력/출력 지문을 기록하고 패키징 뒤 자료를 다시 대조한다. 역사적 ABI 비교용 생성기 `abigen@v1.16.4`와 배포 Go runtime의 패치된 `go-ethereum`은 서로 다른 의존성이다. 생성기 업데이트는 ABI·Go 생성 코드 동등성과 별도 보안 검토가 필요하다. [제품 의존성 재검사](../patches/dependencies/rescan.json)

- `nu54-protocol-0.2.0.tgz`는 독립 npm 프로젝트의 Node ESM import와 TypeScript typecheck를 통과한 배포 파일이다.
- `nu54-protocol-c-0.2.0.tar.gz`는 독립 C11 소비자가 포함하는 v1 프로토콜 헤더, v2 견적 및 전송 코어 소스와 라이선스를 담는다. 호스트 OpenSSL TLS 어댑터 소스는 선택 빌드이며 BLE·하드웨어 드라이버는 별도다.
- `nu54-protocol-go-0.2.0-proxy.tar.gz`는 Go 프록시의 표준 `.zip`, `.mod`, `.info` 및 버전 목록을 담고, 모듈 ZIP 안에 `go.sum`을 포함한다. 독립 Go 프로젝트가 `GOWORK=off`에서 그 프록시로 v0.2.0을 설치한다. 소비자가 작성한 모듈과 go.mod 해시를 대조한다.
- `nu54_protocol-0.2.0-py3-none-any.whl`는 기본 v1 소비와 별도 `[quote-v2]` 암호 백엔드를 지원한다. 독립 venv에서 바퀴 파일과 잠금 해시를 사용해 설치하고 실제 서명 복구를 실행한다.
- `nu54-protocol-vectors-0.2.0.tar.gz`에는 같은 스키마와 벡터가 들어 있어 언어별 런타임 없이 검증할 수 있다.
- 고정 순서와 archive metadata로 payload를 재현한다. CycloneDX SBOM, 소비 시험, 생성기 출처 파일은 각 산출물 해시에 연결된다. 후보 파일 형식은 `nu54-sdk-candidate-v1`이다. 네 SDK 산출물에는 동일한 v2 스키마·C1–C9 레지스트리·견적과 서비스 검증 벡터가 있다.

외부 소비 회귀에서 Go와 TypeScript는 C7 허용·권한 거절을 검증한다. C와 TypeScript는 재연결 뒤 시계 역행을 거절한다. C, Go, Python은 공통 견적 벡터를 실제 서명 복구로 확인한다.

로컬 커스텀 서명과 검증은 고정 취약점 DB를 지정하면 실행한다. 임시 Ed25519 키로 각 payload와 릴리스 목록에 서명하고, 축소된 허용 목록이 다른 산출물을 거절하는지 확인한다.

```sh
python3 products/p10-platform/release/build_sdk.py --smoke --out /tmp/nu54-sdk-proof \
  --scan-cache /path/to/trivy-cache --go-db /path/to/go-vulndb --govulncheck /path/to/govulncheck
```

아카이브를 받은 소비자는 외부에서 전달된 출처와 해시를 먼저 대조한다. 로컬 증거는 실제 registry 게시, 공식 키 승인, 고객 환경 설치를 확인하지 않는다.

## 배포 증거 검증

[신뢰 정책 템플릿](trust-policy.template.json)은 초기 승인 키가 비어 있다. 운영자는 릴리스·설치 공개키와 신뢰 정책을 배포물과 독립 경로에서 제공한다. 정책은 `channels.official`, `channels.custom`, `installers.official`, `installers.custom`마다 DER SPKI SHA-256을 허용된 산출물 ID 목록과 연결한다.

각 산출물에는 `id`, `version`, `target`, `payload` 및 `evidence`가 필요하다. 파일 참조는 검증 폴더 내 `{file, sha256}`이다. `evidence`에는 `provenance`, `sbom`, `rescan`, `regression`, `artifactSigning`의 파일 참조를 쓴다. 각각의 형식은 `nu54-<role>-v1`이며 `artifactSha256`, `evidenceKind`, 원본 `report`의 파일 참조를 연결한다. 출처에는 같은 `sourceCommit`과 `sourceClean`가 필수다. 재검사·소비 회귀·산출물 서명은 통과한 증거에 `passed: true`를 둔다. Android·Apple·MCUboot·SDK 서명은 각 제품 검증 경로에서 실행하고 해당 산출물의 원본 보고서를 연결한다.

승인된 외부 키는 `format: nu54-release-evidence-v1`, `channel`, `sourceCommit`, `sourceClean`, `artifacts`를 담은 JSON 바이트에 detached Ed25519 서명을 한다. 공식 증거는 clean 입력과 실제 관측 보고서가 필요하다.

```sh
python3 products/p10-platform/release/evidence.py \
  --manifest release.json --signature release.sig --public-key release.pub.pem \
  --trust /path/to/external-policy.json --channel official
```

설치 검증은 `--installation receipt.json receipt.sig installer.pub.pem`으로 실행한다. 별도 키로 서명한 `nu54-installation-v1` 영수증이 릴리스 해시, 산출물 ID·해시, 버전, 대상, 채널, 원본 보고서를 연결한다. 산출물 ID·버전·대상별 상태를 출력한다. 해당 산출물의 영수증이 없으면 `installation-unverified`, 합성 영수증이면 `installation-synthetic`이다.

로컬 SDK 증거는 패키지 소비 시험과 고정 DB 재검사, 임시 커스텀 키의 detached 서명을 포함한다. 범위를 줄인 신뢰 정책은 목록 밖 서명을 거절했다. 별도 설치 서명 경로의 테스트 영수증은 합성이다. 실물 설치와 공식 운영 배포 증거는 미측정이다.

## 설치 영수증과 배포 전환

[P10 마이그레이션 004](../database/migrations/004_release_fleet.sql)는 서명한 승인, 배포 코호트, 설치 대상 및 대상별 영수증을 기존 PostgreSQL 배포 원장에 연결한다. `build_release`와 `test_evidence`는 검증한 빌드와 시험을 기록한다. `advisory_case`는 SBOM 변경과 수정 후보를 추적한다. 세 대상의 영수증 수집과 `promotion` 및 배포 포인터 갱신은 한 트랜잭션에서 끝난다.

```sh
python3 -m pip install --require-hashes --no-binary=:all: -r products/p10-platform/release/requirements.txt
P10_MIGRATION_DATABASE_URL=service=p10-migrator make -C products/p10-platform migrate
python3 products/p10-platform/release/fleet.py --database service=fleet-coordinator --policy "$fleet_policy" --help
python3 products/p10-platform/release/fleet_portal.py --database service=fleet-catalog --policy "$fleet_policy"
```

코디네이터 연결에는 `nu54_p10_fleet_service`를, 화면 연결에는 읽기 전용 `nu54_p10_fleet_catalog`를 사용한다. 위임 정책은 승인자별 원 IdP 주체·동일인 별칭·코호트·역할·공개키와, 코호트별 설치자 공개키·채널·타깃 및 합의한 만료 상한을 선언한다. 정책과 설치자의 키는 후보 저장소 외부에서 관리한다.

- `register`가 서명한 후보와 동일 키의 조합 목록을 대조한다. 조합은 제품, OS, 보드, 체인, wire major, SDK, ABI를 포함한다. 같은 산출물에 여러 조합을 연결할 수 있다. 서명, 출처, 실제 검사 파일과 대상 해시가 일치해야 한다.
- `advisory`가 기존/후보 CycloneDX SBOM의 실제 영향 버전을 비교한다. 영향 산출물 누락은 거절한다. 취약 버전이 남거나 후보 파일이 바뀌지 않으면 보류한다.
- 세 승인자가 `vote`를 따로 실행한다. 동일한 요청 계획에 코호트, 현재·새 manifest, 대상별 기대 revision, 만료, 보안 공지 ID와 digest를 결합한다. `begin`으로 계획을 등록한다.
- `install`이 후보 서명을 다시 확인하고 등록된 대상에서 설치·검증한다. SDK 설치의 실제 npm archive integrity와 파일, Android emulator의 API와 설치 APK, iOS simulator의 runtime과 설치 실행 파일, MCUboot 서명과 보안 카운터가 각각 관측 자료가 된다. 설치자는 결과와 계획 해시에 서명한다.
- `collect`에 같은 영수증을 재전송하면 기존 결과를 유지한다. 모든 대상 영수증을 수집한 뒤 `finish`가 CAS로 활동 manifest를 전환한다. 철회는 코호트에 적용되는 별도 서명 계획이다. 복귀는 해당 코호트의 이전 승인 후보, 현 리비전, 새 표적 관측 및 분리 승인으로 진행한다. 철회 키의 후보와 낮은 보드 보안 카운터는 거절한다.

`fleet_portal.py`는 로컬 운영 조회를 제공한다. `/D01?cohort=…`에서 실제 C1–C9 계약을 필터링한다. D02는 조합과 서명 manifest, D03은 빌드 및 설치 영수증, D04는 공지 및 조치, D05는 보드 이미지·키 지문·카운터를 조회한다. 각 경로에 `/api`를 붙이면 JSON이다. [D06·X05 복원 운영 조회](../recovery/README.md)

Node 소비자, emulator, simulator와 호스트 보드 슬롯의 확인은 각각 구분한다. 실기 대상의 공식 증거에는 별도로 승인한 설치자의 관측 영수증이 필요하다. 공식 키와 기기 배포는 운영 등록과 실제 대상 승인이 준비된 뒤 진행한다.


마이그레이션 005부터 공개 여부를 배포 계획의 `public` 필드에 서명한다. 공개 카탈로그 계정은 PostgreSQL 공개 뷰만 읽는다. 기술 지원 조합과 공지에는 승인·설치 영수증이 있는 활성 공개 세대만 나타난다. 원본 SBOM 검토 자료, 설치자 영수증, 담당자, 재검토 예정 시각과 미승격 후보는 검토 계정 범위다.

비공개 D03·D04는 별도 mTLS 운영 리스너에서 제공한다. 정책에 인증서 SHA-256과 승인자, `release-review` 역할, `cohorts` 범위와 Unix 초 단위 `expiresAt`을 등록한다. 정책 변경은 다음 요청부터 반영된다. DB 관리자가 `platform.fleet_review_enrollment`에도 사용 기간과 코호트별 로그인 범위를 설정한다.

```sh
python3 products/p10-platform/release/fleet_portal.py --database service=fleet-review \
  --policy "$fleet_policy" --port 8446 --review \
  --cert "$review_cert" --key "$review_key" --client-ca "$review_ca"
```

D01 계약은 코호트 없이 조회한다. 계약 계열과 역할로 작업을 선택한 뒤 원문 필드, 명령 멱등성과 결과 대사, 오류별 복구 규칙을 펼칠 수 있다. `/api/D01`은 동일 레지스트리의 `errorCodes`, `recovery`, `commandReplay`를 반환한다. 공개 지원표와 공지는 승인 정책에 등록한 공개 코호트에서 조회한다. 390px와 글자 200% 브라우저 검증은 `NU54_FLEET_BROWSER_CLI`로 Playwright CLI를 설정하고 격리 PostgreSQL에서 `python3 products/p10-platform/release/test_fleet.py --existing-postgres`로 실행한다.


실제 SDK 아카이브의 설치·출시 경로는 `rehearse_observed.py --help`로 확인한다. 이전과 갱신 SDK의 원본 출처 보고서, 아카이브, 오프라인 npm·Trivy 캐시를 제공하면 별도 PostgreSQL에서 설치, CycloneDX, 재검사, 독립 승인, 세 설치 영수증, 공개 공지, 철회와 복귀를 실행한다. `--browser`로 Playwright CLI를 지정하면 mTLS 비공개 검토와 공개 호환표·공지 화면도 확인한다. 이전 SDK 버전의 지원 종료를 재현하는 커스텀 리허설이며, SBOM의 관측 버전과 스캐너 결과를 함께 기록한다.

### Mobile SDK runtime peers

`make -C products/p10-platform sdk-mobile-candidate SDK_OUT=/tmp/nu54-mobile-review`
installs the packed mobile SDK in a fresh app with exact React and React Native
peers using `npm ci --strict-peer-deps` and the dedicated transitive consumer
lock. The app, SDK and React Native must share
the same installed peer files. A separate install with an incompatible React
must fail through npm's peer resolver. Set `npm_config_offline=true` and supply
a populated npm cache to run these checks without registry access.

The v2 mobile candidate receipt signs the archive and CycloneDX inventory
hashes and records both installed peers. Verification checks the archive's
declarations against the observed versions and the SDK's direct SBOM dependency
edges. Historical v1 receipts can still be authenticated with their original
signer and carry their original evidence.


### MCUboot 호스트 슬롯 전환 리허설

UTF-8로 생성한 격리 PostgreSQL에서 서명 이미지와 해당 빌드의 manifest, 공개 부트 키, frozen west 목록을 확인한다. 각 보드 조합에는 `bootKey` 지문과 보호 구간 `measurement`가 들어간다. 호스트 슬롯 코호트에는 그 지문에 대응하는 공개키를 별도로 등록한다.

```sh
python3 products/p10-platform/release/rehearse_board.py --existing-postgres \
  --prior-build "$counted_prior" --upgrade-build "$counted_upgrade" \
  --mcuboot-python "$ncs_python" --imgtool "$mcuboot_imgtool" \
  --output "$new_evidence_dir" --browser /path/to/playwright-cli
```

실제 보호 카운터, 해당 부트 서명, 설치 바이트를 대상별 키로 기록한 뒤 세 영수증으로 전환한다. 파일 손상, 등록되지 않은 부트 키, 서명했지만 내용이 일치하지 않는 호스트 슬롯 측정은 보류한다. 이전 버전의 레지스트리 키는 코호트 승인으로 철회한다. 더 낮은 카운터로 복귀하는 계획은 D04·D05에서 담당 역할, 재검토 시각, 사유와 함께 보류하고 현재 버전을 유지한다.

이 리허설은 두 실제 ARM 빌드의 원본 애플리케이션 바이트를 하나의 임시 MCUboot 키로 서명해 호스트 슬롯에 검증한다. 원래 별개 부트 루트로 서명한 이미지도 독립 비교에 등록하지만, 같은 대상의 전환에서는 그 루트를 거절한다. 실물 보드의 부트 루트 교체, 하드웨어 카운터 readback, 실제 부팅과 현장 업데이트에는 별도 증거가 필요하다. 빌드의 frozen west 목록은 해석된 소스 구성 증거이고, 관측 재검사는 이미지·카운터 무결성 범위다.


만료되거나 서명자가 철회된 설치 요청에는 같은 코호트의 새 승인 계획을 내고 `action=resume`과 원 작업 ID인 `supersedes`를 결합한다. 승인자 3명은 새 계획에 다시 서명한다. 만료 요청은 같은 후보만 갱신한다. 철회 요청의 안전한 후속 후보는 중단한 계획과 동일한 대상·리비전을 유지하고, 이미 적용됐을 수 있는 최고 카운터와 기존 부트 루트를 충족한다. 충족하는 후보가 없으면 해당 코호트만 보류한다. 이전 작업과 받았던 영수증은 감사 이력에 남고, 새 후보는 새 계획에 서명된 대상 영수증만 받는다.

보드 리허설에 `--resume-rehearsal`을 추가하면 실제 첫 호스트 슬롯 영수증 뒤 승인 만료, 정상 재승인, 부분 설치 중 키 철회, 재승인된 안전 후보와 카운터 하한을 확인한다. 독립 코호트 배포 상태도 비교한다.

### P02 iOS 시뮬레이터 설치 리허설

서명된 ARM 시뮬레이터 앱 두 빌드를 사용한다. 준비 작업은 새 QA 번들 ID와 리허설 버전을 부여하고 복제본만 ad hoc으로 다시 서명한다. 원본 앱 식별자와 지문은 별도로 기록한다. 독립적인 테스트 키 세 개로 승인하고, 별도의 대상 키 세 개로 실제 설치를 확인한다.

```sh
python3 products/p10-platform/release/rehearse_ios.py --output "$evidence" \
  --prepare "$baseline_simulator_app" "$upgrade_simulator_app"
python3 products/p10-platform/release/rehearse_ios.py --output "$evidence" \
  --existing-postgres --runtime ios-27-0-simulator \
  --device "$qa_simulator1" --device "$qa_simulator2" --device "$qa_simulator3"
```

별도 로컬 PostgreSQL 15의 libpq 소켓에 연결하고, 이미 부팅된 전용 시뮬레이터 세 대를 지정한다. 실행은 새 QA 번들 ID가 세 대상에 없을 때만 시작한다. 설치 전후 번들 버전, 빌드, 코드 서명과 전체 번들 지문을 확인한다. 앱을 실행하지 않으므로 빈 QA 앱 데이터에서만 이전 버전의 안전한 복귀를 검증한다. 만료된 부분 설치의 보류, 재승인 이후 늦은 응답 거절, 업그레이드 게시자 철회와 재설치를 검사한다. 결과는 로컬 custom 채널의 시뮬레이터 설치 증거이며 App Store 서명과 실기 승인 자료는 별도로 관리한다.

기존 연습 기록을 유지하는 iOS 리허설은 별도의 설치 증거가 필요하다. 두 입력 앱은 동일한 전용 QA 번들 ID로 각각 컴파일하고 다른 앱 버전을 사용한다. `--prepare --precompiled-qa-identity`는 원본 서명과 앱 번들을 보존한다. 두 ARM 실행 파일에 포함된 앱 ID 및 Keychain 그룹이 전용 번들 ID에 대응해야 한다. 목적이 다른 운영 데이터를 포함하는 기기 대신 승인된 전용 시뮬레이터를 사용한다. 아래 검증기는 baseline에서 틀린 금액 연습을 보류하고, upgrade에서 원래 결과를 읽어 거절한 뒤 두 번째 연습을 저장한다. 마지막으로 baseline을 다시 설치하고 두 번째 결과를 이어서 완료한다. 각 단계에서 정확한 서명 번들과 설치 버전 및 제품 XCTest 결과를 대조한다.

```sh
python3 products/p10-platform/release/rehearse_ios_profile.py \
  --stage "$evidence" --output "$profile_evidence" --device "$qa_profile_simulator" \
  --key "$profile_verifier_private" --public "$profile_verifier_public"
```

승인 정책의 `iosProfileVerifiers`는 별도로 발급한 키 지문별 공개키 경로 `key`, 허용 번들 ID 목록 `packages`, 런타임 목록 `os`를 사용한다. 기존 프로필을 유지할 대상의 `iosProfile`은 `existing-profile`이며 승인된 후보 조합의 서명 매트릭스에 같은 `profile`과 `profileProof`를 포함한다. 위 전체 리허설은 `--profile-proof "$profile_evidence/proof.json" --profile-key "$profile_verifier_public"`를 지원한다. 실패·누락·만료·다른 앱의 입력은 보류한다. 과거 영수증과 새 설치만 시행한 후보는 공개 지원표에서 ‘새 설치만 관측’으로 표시된다.

배포 시작, 설치 영수증 수집, 전환 및 중단된 작업의 교체는 PostgreSQL 쓰기 뒤 커밋 직전에 기존 프로필 근거를 다시 확인한다. 검증 키 회수는 같은 코호트의 독립 승인 세 개로 서명한 `revoke` 계획을 사용한다. 정책 파일의 허용 목록 변경도 진행 중인 CLI 요청에서 다시 읽는다. 격리 PostgreSQL에서 `python3 products/p10-platform/release/test_ios_profile_race.py --existing-postgres`를 실행하면 DB 잠금 대기 중에 만료와 회수를 유발하고 상태 보존을 확인할 수 있다.

### P02 Android 시뮬레이터 롤백 리허설

두 APK는 별도 QA 전용 패키지 ID와 같은 서명 인증서를 사용한다. 서로 다른 증가 버전의 ARM64 단일 APK를 준비한다. 업데이트 매니페스트는 `android:rollbackDataPolicy="retain"`을 선언한다. 전용 Android API 34 Google APIs 에뮬레이터에서 아래 명령으로 일반 화면을 검증한다. Play Protect가 개입하거나 Android가 가용한 롤백 항목을 제공하지 않으면 보류한다.

```sh
python3 products/p10-platform/release/rehearse_android_profile.py \
  --baseline "$baseline_apk" --upgrade "$retained_apk" --device "$profile_emulator" \
  --aapt "$aapt" --apksigner "$apksigner" --output "$profile_evidence" \
  --key "$profile_verifier_private" --public "$profile_verifier_public"

python3 products/p10-platform/release/rehearse_android.py --existing-postgres \
  --baseline "$baseline_apk" --upgrade "$retained_apk" --unsafe "$no_retain_apk" \
  --profile-proof "$profile_evidence/proof.json" --profile-key "$profile_verifier_public" \
  --aapt "$aapt" --apksigner "$apksigner" --output "$fleet_evidence" \
  --device "$qa_emulator1" --device "$qa_emulator2" --device "$qa_emulator3"
```

독립 검증자가 서명한 연습 기록 범위를 사용한다. 승인 정책의 `androidProfileVerifiers`는 별도로 허용한 공개키, 패키지, API를 지정한다. 코호트 대상 `androidProfile=practice-profile`은 과거 연습을 거절하고 새 영어 연습을 작성한 뒤 OS 롤백한 앱에서 최신 연습을 승인하는 세 관측을 요구한다. 설치 관측기는 실제 기기의 Android PackageManager에 롤백을 요청하고 원본 APK의 해시와 서명 인증서를 검사한다. 코호트는 대상별 ACK를 모두 확인한 뒤 공개되며, 이전 ACK는 재승인에 사용할 수 없다. 철회 시 공개 조합은 제외된다. 이전 영수증은 새 설치 관측 범위로 표시한다.

현재 검증 범위는 API 34의 연습 기록이다. 보호 지갑의 미결 승인·서명·세대 자료 또는 새 저장소 형식의 실제 OS 간 연속성이 확인되지 않은 조합은 보류한다. 별도 저장소 시험에서는 이전 읽기 앱이 새 형식의 기록을 덮어쓰지 않고, 이미 소모된 승인 예약을 되살리지 않는지 확인한다.

별도의 `rehearse_android_protected.py`는 C1 보호 기록에 한정한다. 동일한 QA 인증서로 서명한 두 APK를 준비하고, API 34 전용 에뮬레이터 한 대에서만 실행한다. QA 계측 APK는 실제 P02의 `OwnerKeys.save/load/compareAndSave`를 호출한다. 자체 키나 우회 저장소로 지갑 저장을 대신하지 않는다.

```sh
python3 products/p10-platform/release/rehearse_android_protected.py \
  --baseline "$baseline_apk" --upgrade "$upgrade_apk" --device "$qa_emulator" \
  --aapt "$aapt" --apksigner "$apksigner" --android-jar "$android_jar" --output "$proof_dir" \
  --fixture-keystore "$fixture_keystore" --fixture-pass-file "$fixture_pass_file" \
  --key "$observer_private" --public "$observer_public"

python3 products/p10-platform/release/rehearse_android.py --existing-postgres --protected-local \
  --baseline "$baseline_apk" --upgrade "$upgrade_apk" --unsafe "$incompatible_apk" \
  --profile-proof "$proof_dir/proof.json" --profile-key "$observer_public" \
  --aapt "$aapt" --apksigner "$apksigner" --output "$fleet_dir" --device "$qa_emulator"
```

검증기는 이전 앱에서 미결 승인을 열고, 업데이트 뒤 만료된 미전송 시험 거래와 원본 해시를 확인한다. 롤백 뒤 이전 앱이 최신 AES-GCM 기록과 원래 해시를 읽어야 하며, 오래된 승인과 이전 세대의 쓰기는 거절돼야 한다. 서버나 체인으로 제출하지 않는다. 결과에는 시험 거래의 해시와 범위 요약만 기록한다. 잠금 해제나 만료를 근거로 미전송 기록을 지우는 구버전 조합은 지원에서 제외한다.

개발 코호트의 대상과 서명 매트릭스가 모두 `protected-profile-local`을 사용하고, 별도 `androidProfileVerifiers` 등록에 `contracts: ["c1-retained-local-v1"]`이 있어야 한다. 이 범위는 에뮬레이터에 대한 보호 글쓰기와 복귀 관측이다. 하드웨어 소유자 키를 사용할 수 없었던 에뮬레이터에서도 이 제한된 범위만 표시한다. 실기 또는 일반 보호 지갑 릴리스(`protected-wallet`)는 보류한다.

### D03 프로토콜 소비자 증거

`rehearse_interop.py`는 관측 자료로 서명된 Protocol SDK 묶음과 사전에 준비한 npm 캐시를 사용한다. C 소스 아카이브를 독립 C11 프로젝트에서 빌드·실행하고, TS npm tarball과 Go proxy ZIP, Python wheel을 별도 프로젝트에 설치한다. 각 소비자가 같은 벡터 원문 해시를 확인하고 대상별 서명 영수증을 남긴다. D03의 `view=start`와 `view=partial`에서 C·TS v2 프레임 재접속, Go v1 프레임 분할·순서 오류·재시작, TS·Go 서비스 권한 거절을 구분해 표시한다. Python wheel은 원문과 서명·거절 벡터를 확인한다. 배포된 Python SDK에는 프레임 전송 API가 없다. 실물 링크의 지원은 보류한다.

```sh
python3 products/p10-platform/release/rehearse_interop.py --existing-postgres \
  --source "$signed_sdk_directory" --npm-cache "$offline_npm_cache" \
  --go-proxy "$offline_go_proxy" --python-wheels "$locked_python_wheelhouse" \
  --output "$evidence"
```

### D05 보드 출처와 설치 전 비교

`python3 products/p10-platform/release/board_reference.py ../nu54v-dk`로 고정한 읽기 전용 소스의 회로도와 vendored 핀 제어·장치 트리 지문을 확인한다. D05의 리비전·채널·재빌드 이미지·부트 키 비교는 승인된 설치 영수증에 기록된 값과 제출한 지문을 맞춘다. 불일치하면 설치와 시드 등록을 차단 상태로 표시한다. 참고 회로의 부품 표기는 생산 BOM이나 장착 부품의 공급 이력을 입증하지 않는다. 일치 후에도 제조 BOM·대체 부품·실물 부팅의 증거가 없으면 실물 설치를 보류한다.

### D02 로컬 아카이브 출처 확인

`/D02`에서 공개 릴리스를 고르고 설치 전 SDK 출처 확인을 연다. 선택한 조합의 SDK 파일, 릴리스 manifest, 64바이트 분리 서명, PEM 공개키를 지정한다. Chrome은 등록된 키 지문과 manifest·아카이브의 SHA-256, Ed25519 서명을 브라우저에서 확인한다. 마지막에는 공개 카탈로그의 활성 릴리스 상태를 다시 조회한다. 로컬 파일은 서버로 전송되지 않는다. 불일치한 파일을 올바른 파일로 바꾸고 같은 릴리스에서 다시 확인할 수 있다.

### D04 후보 검토와 보류

마이그레이션 013은 등록 때 확인한 SKU별 SBOM·재검사·회귀·서명 자료의 최소 투영을 보관한다. 검토 화면의 `view=review`에서 해당 코호트의 후보와 아티팩트만 열 수 있다. 기존 후보에서 자료가 비어 있으면 원래 서명된 증거로 같은 후보를 재등록할 수 있다.

보류 쓰기에는 읽기 로그인과 별개의 `--hold-database`를 사용한다. 그 로그인에 `nu54_p10_fleet_hold`를 부여하고, `platform.fleet_hold_enrollment`에 운영자 UUID·코호트·담당 역할·만료를 등록한다. mTLS 정책의 운영자에게 `release-review`와 `release-hold` 권한, 같은 코호트와 `holdRoles`를 지정한다. 보류 화면에서 담당 역할, 사유 및 다음 UTC 검토 시각을 지정한다. 요청 ID와 변경 번호로 같은 제출의 재실행과 다른 내용의 충돌을 구분한다. 공개 카탈로그에는 보류 사유와 검토자의 정보가 표시되지 않는다.

격리 PostgreSQL과 검수용 Chrome에서 기존 관측 SDK 후보 묶음으로 두 기능을 재현한다.

```sh
python3 products/p10-platform/release/test_catalog_review.py --existing-postgres \
  --source "$observed_before_fixed_candidates" --npm-cache "$locked_npm_cache" \
  --browser "$playwright_cli"
```

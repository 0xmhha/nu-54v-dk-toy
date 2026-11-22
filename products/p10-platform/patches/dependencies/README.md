# 03C Go·pnpm 의존성 검증

11월 22일 05:58 KST, 이관 095–097과 대여 정리 지시·영수증, C1 프로토콜 생성 코드를 포함한 소스로 `opsd`, `rental-collect`, `indexer`를 Linux ARM64로 빌드했다. Trivy가 실행 파일에서 식별한 Go 구성요소는 각각 45개, 39개, 42개였다. 11월 21일 18:38 UTC 갱신된 Trivy DB와 공식 Go 취약점 DB를 사용했으며 세 실행 파일 모두 Trivy 취약점과 govulncheck 발견 기록이 0건이었다. 기록에 대상별 구성요소, 소스·바이너리·리포트 지문과 DB 갱신 시각을 남겼다. 이 검사의 범위는 세 실행 파일에 포함된 Go 의존성이다. [검사 기록](wallet-payment-return-services.json)

[기준선 소스](../../baseline/v1.json) · [기준선 패치 목록](../../baseline/patch-ledger.json) · [전 SBOM](before-sbom.json) · [후 SBOM](after-sbom.json) · [재검사 결과](rescan.json)

비교 기준은 v1 고정 소스다. Go는 두 소스 상태를 Go 1.26.11과 동일한 `linux/arm64` 조건에서 확인했다. 명령 바이너리는 `CGO_ENABLED=0 go build -trimpath -buildvcs=false`로 생성했다. pnpm 결과는 같은 Trivy DB로 lockfile의 기본 범위와 개발 의존성 포함 범위를 비교했다. 조회 결과는 스캐너와 DB 버전, advisory ID, 대상, 판정 범위, 원본 리포트 SHA-256을 저장한다.

| 범위 | 변경 전 | 변경 후 |
|---|---:|---:|
| Go 소스 심볼 분석, protocol / ABI / P05 / P07 | 3 / 4 / 4 / 6 | 0 / 0 / 0 / 0 |
| Go 바이너리 심볼, opsd / opsctl / indexer | 0 / 4 / 6 | 0 / 0 / 0 |
| pnpm 전체 / lockfile 기본 범위 | 15 / 8 | 0 / 0 |
| 최종 로컬 컨테이너, opsd / indexer | v1 비교는 명령 바이너리 기준 | 0 / 0 |

Go 심볼 건수는 각 대상에서 심볼까지 연결된 고유 advisory만 집계한다. 모듈·패키지 단계에서만 확인된 별도 항목도 JSON에 남긴다. pnpm 결과의 importer는 제품과 개발/런타임 의존 경로를 나타낸다. 모바일 빌드에 어떤 JS가 포함되는지는 앱 패키지 증거에서 확인한다.

- Go에 geth 1.17.10, x/crypto 0.59.0, gorilla/websocket 1.5.3, OpenTelemetry 1.47.0을 반영했다. Go의 나머지 해석 결과는 모듈·workspace 잠금 자료와 SBOM에 기록했다. [geth 릴리스](https://github.com/ethereum/go-ethereum/releases/tag/v1.17.10) · [x/crypto 모듈](https://pkg.go.dev/golang.org/x/crypto@v0.59.0) · [Go WebSocket 권고](https://pkg.go.dev/vuln/GO-2026-6278)
- P05 web의 Vite 7.3.8과 루트 pnpm override 8개를 고정했다. 패치 전 하위 의존성을 쓰던 경로는 재검사 JSON의 importer 목록에서 확인할 수 있다.
- P05/P07 OCI는 Go 1.26.11 빌더와 Debian 13 distroless nonroot 이미지를 digest로 고정한다. 중간 Debian 12 이미지에서 ca-certificates와 tzdata 권고를 확인했고, P07 이미지에서 OpenTelemetry 권고를 확인해 베이스와 모듈을 함께 갱신했다. 최종 로컬 OCI에는 UID 65532와 앱별 실행 파일 한 개만 추가된다. SBOM의 OCI 항목은 검사한 구성 해시와 연결한다.

검증은 네 Go 모듈의 테스트, JS workspace typecheck와 테스트, P05 web production build, PostgreSQL 15의 P07 재시작·중복 수집 시험과 두 컨테이너 빌드 및 앱 레이어 파일 확인을 포함한다. P05 로컬 컨테이너에서 `/healthz`가 HTTP 200을 반환했다. P02 Jest는 30건, P04 Jest는 80건이 통과했다. 최종 컨테이너는 로컬에서 빌드하고 스캔했다. 출시·설치 버전은 서명된 레지스트리/설치 증거에서 별도로 확정한다.

첫 P05 검증 빌드는 allowlist 적용 전에 공용 로컬 Docker daemon으로 전체 컨텍스트를 전송했다. 해당 daemon의 공유 캐시는 유지했다. 최종 빌드에는 각 Dockerfile의 컨텍스트 allowlist를 적용했고, 컨테이너 앱 레이어에 실행 파일 외 파일이 없는 것을 이미지 저장본에서 확인했다.

## Node 22 CI and packaging update

The CI action pin is unchanged and its Node input now requires 22.23.5.
The verifier checks both the workflow's selected input and the running
executable. Three current CI jobs use this input for framework builds,
SDK packaging and conformance. The build host was verified against the
upstream signed checksum manifest. The observation records the selected Node
and built-in dependency versions and distinguishes local runtime usage from
Go and OS-native mobile TLS. Consumers of the optional Node transport provide
their own maintained Node runtime. [Node evidence](node-22.23.5.json) · [Node.js security release](https://nodejs.org/en/blog/vulnerability/november-2026-security-releases/) · [Release checksums](https://nodejs.org/dist/v22.23.5/SHASUMS256.txt)

The execution-phase P09 addition has its own
[placesd scan receipt](placesd-scan.json). It records the Go source and
actual Linux executable scans, and a separate local OCI scan, without
changing the archived before/after snapshots. The unsigned service candidate
includes a CycloneDX binary SBOM.


### 11월 21일 서버와 운영 명령의 Linux ARM64 후보

가맹점 권한 회수 이관 062를 포함한 소스 2,786개를 고정하고 5개 제품의 Linux ARM64 실행 파일 16개를 빌드했다. Trivy가 각 바이너리에서 Go 패키지를 식별했으며 사용한 DB의 취약점 발견은 모두 0건이었다. govulncheck도 같은 16개 바이너리에서 발견 0건이었다. Go 공식 색인과 사용한 DB의 수정 시각이 일치했다. 기기 펌웨어·모바일 라이브러리·OS 이미지의 결과는 해당 배포별 근거에서 조회한다. [후보별 지문·패키지·검사 원본 지문](service-candidates.json) · [Go 공개 취약점 색인](https://vuln.go.dev/index/db.json)

| 제품 | 검사한 실행 파일 | Trivy가 식별한 Go 구성요소 수 |
|---|---|---|
| P03 | mpc-coordinator, mpc-participant, mpc-roster | 35, 25, 35 |
| P05 | opsd, opsctl, verify-firmware, visit-issuer, visit-benefit, visit-benefit-read | 45, 31, 36, 6, 6, 8 |
| P07 | indexer | 42 |
| P08 | marketd | 39 |
| P09 | erase-worker, erasured, placesd, processor, recordsd | 각 9 |

11월 21일 10시 38분 사본에서는 등록형 인증 게이트웨이를 포함한 실행 파일 17개를 각각 다시 빌드했다. 새 Trivy DB에서 실행 파일마다 Go 구성요소 6~45개를 식별했고, Trivy와 govulncheck가 보고한 의존성 취약점은 각각 0건이었다. 이 사본은 다중 인증 제공자의 콜백 수정을 적용하기 전 소스다. 그 경로의 응용 코드 검수는 지갑 인수에서 별도로 추적한다. [17개 바이너리 지문·구성요소·검사 결과](service-candidates-oidc.json)

제공자별 콜백 수정 후에는 게이트웨이를 Linux ARM64로 다시 빌드해 소스와 바이너리에 govulncheck 심볼 검사를 각각 수행했다. 소스 분석은 모듈 39개, 실행 파일 분석은 38개를 식별했고 두 분석 모두 발견 0건이었다. 사용한 Go 공지 DB와 공식 색인의 수정 시각은 2026년 11월 11일 02:51:11 UTC로 같았다. [콜백 수정본의 별도 지문·심볼 검사](service-candidates-oidc.json) · [Go 공지 DB 색인](https://vuln.go.dev/index/db.json)

배포 설정과 자격 파일을 열린 파일에서 검사하도록 변경한 게이트웨이를 다시 빌드했다. Linux ARM64 실행 파일 지문은 `sha256:3d37fce1385482d26d03cac041dda05cf91aa6bf6483b137b66437d37b117f22`다. 같은 공지 DB를 사용한 govulncheck의 소스·바이너리 심볼 검사에서 모듈은 각각 39개·38개, 발견은 모두 0건이었다. 운영 계정 권한과 TLS 배포는 별도 검증한다. [인증 파일 보호 수정본의 검사 지문](service-candidates-oidc.json)

별도로 17시 42분의 P05 `rental-collect` Linux ARM64 바이너리를 검사했다. Trivy가 BLE 라이브러리 `tinygo.org/x/bluetooth@v0.16.0`을 포함한 Go 구성요소 31개를 식별했다. Trivy의 바이너리 취약점과 govulncheck의 발견이 각각 0건이었다. 실행 파일 지문, 두 공지 DB 시각 및 검사 원본 지문은 수집기 기록에 보존했다. 무선 장치와 호스트 OS는 별도 인수 대상이다. [수집기 바이너리 검사 기록](rental-collector.json)

### nRF54L15 실험 이미지의 SDK 소스 구성

11월 21일의 별도 NCS 3.4.1 한도 보호 실험 빌드에 대해 비보안 앱·TF-M·MCUboot의 컴파일 데이터베이스에서 Nordic SDK 소스를 추출했다. 세 ELF와 서명한 결합 이미지의 지문이 구현자가 별도로 보존한 기록과 일치했다. 공급사 도구 `west ncs-sbom`의 SPDX 2.2 파일 목록에서 각 소스의 체크섬을 재계산하고, 각 SDK 파일의 바이트를 해당 Git 커밋 객체와 대조했다. 세 보고서에 걸쳐 중복을 제외한 SDK 원본은 481개다. [이미지·컴파일 명세·SDK 커밋별 검증](firmware-qualification/inventory.json)

| 구성 | 컴파일 명세에서 확인한 SDK 소스 | SDK 저장소 |
|---|---:|---:|
| [비보안 앱](firmware-qualification/nonsecure-sdk.spdx) | 225 | 4 |
| [TF-M 보안 이미지](firmware-qualification/secure-sdk.spdx) | 217 | 5 |
| [MCUboot](firmware-qualification/bootloader-sdk.spdx) | 255 | 6 |

입력은 각 컴파일 명령이 주 소스로 지정한 SDK 파일이다. 헤더, 미리 빌드한 라이브러리, SDK 밖의 생성물·애플리케이션 코드와 컴파일러는 추가 인벤토리가 필요하다. 파일별 라이선스와 Git 버전을 기록한 결과이며, 해당 코드의 실행 가능 경로와 CVE 판정은 별도 분석한다. 파일 목록의 상대 경로는 세 보고서 모두 `./sdk/`에서 시작한다. 원본과 경로 정리 후 파일의 지문을 함께 보관했다. 제어판과 이미지의 실물 설치, 보안 상태 판독 및 운영 서명은 후속 인수 대상이다.

이 목록은 한도 보호 서비스 V1 실험 이미지에 해당한다. 독립 감사에서 이 버전이 공장 식별자와 저장된 부트 설정 원문을 비보안 영역에 반환한다는 점을 확인했다. V1은 출시 대상에서 제외했다. V2는 원문을 보안 영역에 두고 36바이트 응답만 반환한다. Astra가 실제 보안·비보안 ARM ELF에서 요청과 응답 크기 및 미충족 자격 거절을 확인했다. 세 이미지의 SDK 주 소스는 V1과 경로·내용이 일치했다. 설치 보드의 부트·디버그 잠금·롤백 보호를 남은 인수 기준으로 유지한다. [V2 이미지와 SDK 대조 지문](firmware-qualification/v2-sources.json) · [펌웨어 시험과 실물 인수 범위](../../../p01-device-firmware/W27-POLICY-PROGRESS.md)

11월 21일 OSV에서 SDK 원본 저장소 7개의 정확한 Git 커밋을 조회한 결과, 일치하는 공지는 0건이었다. 이 결과는 포크의 상위 버전, 미등록 공지 또는 링크된 전체 이미지의 취약점 도달성을 판정하지 않는다. Nordic은 11월 4일에 정식 LTS 패치 3.4.2를 공개했다. 기존 3.4.1 빌드 기록을 보존하면서 3.4.2 입력과 대상 보드 재빌드를 검증한다. [커밋별 OSV 조회 결과](firmware-qualification/osv-commits.json) · [Nordic 3.4.2 릴리스](https://github.com/nrfconnect/sdk-nrf/releases/tag/v3.4.2)

Nordic 3.4.2의 변경 내역에는 MCUboot가 이미지 크기를 플래시 파티션 범위와 대조하는 수정이 있다. 별도 MCUboot 공지 CVE-2026-127915는 TLV 반복 처리에 관한 것이다. 프로젝트는 Nordic 포크 3.4.2의 전용 빌드 사본에 유지관리자의 수정 커밋을 적용한다. Astra가 원본에 패치를 다시 적용해 수정 파일을 대조하고, 실험용·커스텀 배포용 두 MCUboot ELF에서 해당 검사 코드의 주소를 확인했다. 실제 단말의 설치 상태는 별도 인수 대상으로 남는다. [Nordic 릴리스 노트](https://raw.githubusercontent.com/nrfconnect/sdk-nrf/v3.4.2/doc/nrf/releases_and_maturity/releases/release-notes-3.4.2.rst) · [MCUboot 유지관리자 공지](https://github.com/mcu-tools/mcuboot/security/advisories/GHSA-4fp3-3qw2-25jv)

3.4.2와 공급자 도구체인 fa36dcc52a로 만든 두 개의 로컬 서명 빌드에서 ELF 다섯 개의 빌드 입력을 추적했다. 중복을 제외한 공급자 파일은 1,518개이며, 헤더 959개와 사전 빌드 라이브러리 19개가 포함된다. SDK 원본은 Git 파일과, 별도 Mbed TLS 4.1.2 소스는 SHA-256을 고정한 배포본과 프로젝트 통합 패치를 새로 풀어 비교했다. 독립 감사에서 CycloneDX 1.6 자료와 두 부트로더의 수정 파일 연결을 재확인했다. 각 ELF에 연결된 자료는 컴파일 입력 의존성이며 세부 심볼의 실행 가능성은 별도 판정한다. 커스텀 CLI가 정리한 일회성 소스 디렉터리는 같은 입력으로 준비한 사본을 사용했고, 원래 컴파일 경로는 빌드 명세에서 확인했다. [프로필과 ELF별 집계](firmware-qualification/ncs-3.4.2-summary.json) · [CycloneDX 파일·관계 목록](firmware-qualification/ncs-3.4.2.cdx.json)

부트로더 ELF를 기본 릴리스 명세에 추가한 뒤 일반 커스텀 보드 빌드를 새 시험 키로 다시 수행했다. 빌드가 기록한 14개 파일을 직접 다시 읽어 모두 일치함을 확인했다. P05가 확인한 이미지 버전은 0.3.0, 보안 카운터는 2였다. 부트로더 ELF의 SHA-256은 `2a39602553ffebac5124a1529876d16c99a05bdbe39e00b822cdfd2089c1b633`이다. 이 커스텀 빌드는 수정 중인 작업 트리를 사용했고 실험용 보드 정책은 비활성 상태였다. 위의 공급자 파일 목록은 앞서 분석한 두 빌드의 입력이다. [실제 빌드에서 생성한 기본 릴리스 명세](firmware-qualification/ncs-3.4.2-custom-primary-manifest.json)

최초 등록의 TLS 1.3 경로를 합친 일반 커스텀 이미지도 별도로 빌드했다. 일회용 등록 발급 공개키의 지문을 명세에 포함했고, Astra가 같은 키의 공개 점을 독립 추출해 애플리케이션 ELF와 대조했다. 관리 세션도 명세의 14개 파일을 다시 해시했다. 애플리케이션 링크에서 플래시 352,416바이트와 RAM 161,932바이트를 사용했다. 호스트의 신규 등록·갱신 시험은 같은 보드 공개키·지급 주소·기기 ID를 보존했다. 이 이미지의 디버그 잠금, 설치 카운터, 실제 부팅은 실물 확인 항목이다. [등록 발급 지문이 포함된 일반 빌드 명세](firmware-qualification/ncs-3.4.2-custom-commissioning-manifest.json) · [등록 도구의 TLS 검증](../../../p01-device-firmware/scripts/enroll_board.py)

이 실행에 사용한 P01 앱·보드·코어·스크립트·벤더 파일과 공통 C 프로토콜의 경로 및 지문 255개도 보존했다. 컴파일 전후의 두 입력 목록과 관리 세션이 다시 읽은 파일 255개의 값이 일치했다. 이 목록은 작업 트리의 로컬 입력을 비교할 때 사용하며 Nordic 모듈·컴파일러와 서명한 출력은 빌드 명세 및 별도 SDK 자료에서 확인한다. [합본 빌드의 로컬 입력 지문](firmware-qualification/ncs-3.4.2-commissioning-sources.json)

### P06·P08 계약 컴파일러

두 제품의 기존 Solidity 0.8.30에는 0.8.39 릴리스의 공급자 버그 목록 기준으로 고유 공지 9개가 연결된다. 그중에는 IR을 사용하는 경우에만 해당하는 항목이 있다. 두 제품은 Prague, 최적화 200회와 기존 코드 생성 방식(viaIR 해제)을 사용한다. 각 컴파일러의 macOS 배포 SHA-256을 확인한 뒤 동일한 13개 소스를 0.8.30과 0.8.39에 입력했다. 계약·라이브러리·인터페이스 정의 14개의 ABI, 메서드 선택자와 저장소 레이아웃이 모두 일치했다. 실제 코드가 들어 있는 정의의 런타임 지문과 크기는 컴파일러별로 기록했다. [컴파일 입력과 정의별 비교](contract-compilers.json) · [Solidity 0.8.39 릴리스](https://github.com/argotorg/solidity/releases/tag/v0.8.39)

Foundry 1.5.1과 고정한 0.8.39로 서로 다른 두 디렉터리에서 빌드한 결과, 계약 여섯 개의 생성 코드와 런타임 템플릿 지문이 각각 일치했다. P06 일반 테스트 110건·고정 가스 비교 102건, P08 Solidity 테스트 10건과 시장 재시작 통합이 통과했다. 독립 검수에서 새 로컬 Anvil에 일반 Go 배포 명령으로 Market과 토큰 두 개를 생성했다. 세 CREATE의 입력 전체와 생성자 인자, 반환 코드, 배포 블록에 기록된 코드를 비교했다. 과거 StableNet 8283 영수증의 계약 세 개는 당시 소스와 0.8.30으로 복원해 원래 런타임 지문을 확인했다. [검증 절차와 계약별 지문](contract-compilers.json)

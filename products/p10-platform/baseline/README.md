# v1 기준선 · 01B/01C

[v1.json](v1.json)은 상용화 이전 소스의 Git 커밋과 제품별 tree, NU-54V-DK 보드 리비전과 vendored 타깃, P02/P04 앱 입력, StableNet 8283 배포 계약·ABI 지문, 도구 버전과 W7 확정 블록을 묶는다. Git tree 지문과 과거 실기 실행의 이미지 지문은 별도 근거다. JSON의 null은 확인할 원본이나 측정값이 없다는 뜻이다.

저장소 루트에서 실행한다. 기준선 검사에는 Python 3, Ruby/Psych와 배포 커밋의 Git 객체가 필요하다. 얕은 checkout에서 객체가 없으면 해당 Git 이력을 먼저 가져온다.

```sh
make -C products/p10-platform baseline
python3 products/p10-platform/baseline/verify.py --require-receipt
make -s -C products/p10-platform baseline-sbom > /tmp/nu54-v1-source-sbom.json
```

`baseline`은 기준 커밋의 소스와 보드·앱·배포/ABI 근거를 대조한다. 원본 영수증이 로컬 archive에 있으면 SHA-256과 성공 블록을 함께 검사한다. `--require-receipt`는 원본이 없는 환경에서 실패한다. 기준 커밋의 Git 분석과 합성 회귀의 현재 checkout은 독립적으로 확인한다. 기준 소스에 대해 회귀를 반복할 때는 manifest의 제품 및 공유 package 입력을 같은 Git 커밋으로 준비한다.

## 출처와 실측 범위

| 항목 | 확인한 근거 | 추적할 항목 |
|---|---|---|
| NU-54V-DK | 보드 Variant NU-54DK-C, v2 Pre-release, nRF54L15, NCS v3.4.1 및 bring-up 도구 버전. [Bring-up](../../../docs/content/nu54v-basic-peripheral-bringup-log.md) | 공식 펌웨어 이미지·부트로더 해시와 대상 보드에 설치된 버전 |
| W7 결제 | 2026-10-05 Android 디버그 키오스크와 사람의 SW1 승인, 기기 서명, StableNet 8283 결제 블록 21463022 및 관측 당시 finalized 21463049. [W7](../../../docs/content/acceptance/gate-w7.md) | W7 빌드의 정확한 firmware commit/이미지 및 APK 해시, P02 폰 확인 포함 실기 호환성 |
| W6 finality | W6 결제 블록 21129166 재조회 및 3,700초 관측. [Finality 기록](../../../docs/content/acceptance/finality-8283.md) | W7 원본 영수증의 성공 블록과 W6 재조회 결과를 각각 인용 |
| 대여/반납 | 2026-10-03 P05 테스트넷 입금 및 계정 종료 확정, 재실행 시 종료 생략, 연결 기기 부재로 초기화 보류. [P05](../../p05-operations-backoffice/README.md) | 기기 초기화·재부팅, 지연 출금 수령, 개설부터 반납까지의 연결된 실기 시험 |
| W12 수용 | 수용 항목·예상 결과·절차. [수용표](../../../docs/content/acceptance/week12-log.md) | 연속 20건, 모든 실기 거절·폰 확인, 최종 설치 패키지와 서명 증빙 |

## 소스 SBOM과 패치 장부

`baseline-sbom`은 기준 커밋에서 11개 산출물의 소스 SBOM을 생성한다. 모바일과 웹은 각 pnpm importer에서 선택 가능한 runtime/development 그래프를 순회하며 peer snapshot과 lockfile integrity를 보존한다. Go 항목은 `go.mod`의 직접·간접 요구와 `go.sum`을 각 실행물에 연결한다. Go workspace의 선택 모듈, 링크된 모듈, OCI OS 패키지는 산출물 검사에서 수집한다. 보드 및 암호 사본은 vendor 출처를 쓰고 Foundry test vendor는 테스트 빌드에 기록한다. 앱의 네이티브 그래프, west 하위 모듈, 바이너리 도달성, 이미지·서명·설치 해시는 각 산출물의 `pendingEvidence`로 추적한다.

[패치 장부](patch-ledger.json)는 공지와 조사 범위를 열린 상태로 기록한다. 실제 사용 버전과 도달성, 지원되는 패치, 담당자와 시한, 수정 빌드와 SBOM, 재검사, 소비자 회귀, 설치 버전으로 갱신한다. [상용 릴리스 기준](../../../docs/content/commercial/security-and-release.md)

## 합성 회귀

Node 22+, pnpm 10.18.0, 저장소의 frozen JS 의존성, Go workspace 지원 도구, CI와 같은 Forge 1.5.1 및 캐시된 solc 0.8.30, CMake/C 컴파일러/OpenSSL이 필요하다. Forge 합성 체인의 chain ID는 8283이다. restart에 필요한 PostgreSQL의 `initdb`와 `pg_ctl`은 PATH에서 찾거나 `POSTGRES_BIN`으로 지정한다.

```sh
pnpm install --frozen-lockfile
make -C products/p10-platform baseline-flow
make -C products/p10-platform baseline-restart
make -C products/p10-platform baseline-regression
```

- **개설·폰 확인**은 P02의 setup/confirm과 P05의 setup 명령 벡터, 합성 PIN 거절·타임아웃을 재실행한다.
- **예치·승인·정산·cashOut·반납**은 P04의 합성 기기 승인/보안 채널·finalized 이벤트 판정, P06의 로컬 서명 및 전체 정산 시험, P05의 fake chain 종료→확정→리셋 순서를 재실행한다. P06의 `test_totalOwed_tracksEveryMove`는 합성 예치 잔액을 시작점으로 서명 결제, 가맹점 지급, 계정 종료, 지연 출금까지 같은 상태에서 검사한다.
- **거절**은 기존 C firmware host session/links 벡터, P05 거절 메시지, P04 기기 거절 및 P06 컨트랙트/테스트 토큰 거절을 재실행한다. 하드웨어, BLE 물리 연결, 외부 RPC 결제의 실행 상태는 위 표에서 추적한다.
- **재시작**은 실행마다 새 PostgreSQL 데이터 디렉터리와 전용 Unix 소켓을 사용한다. 테스트는 합성 finalized 로그를 처리한 뒤 인덱서를 재생성하고, cursor에서 재개해 중복 결과를 확인한다. runner가 직접 만든 접속 정보만 넘기며 `go test -json -count=1`에서 지정 테스트의 실행·통과를 확인한다. 완료 시 서버와 임시 디렉터리를 정리한다.

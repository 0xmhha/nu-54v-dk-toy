# P08 · Market services

> **DF-20260925-02 기준 (2026-09-25):** 이 제품은 이번 12주 사이클에서 설계만 하고 구현하지 않는다(out of cycle). [설계 동결 DF-20260925-02](../../docs/content/planning/design-freeze-checkpoint-02.md) 참조. 아래 원문은 DF-20260920-01 기준이다.

StableNet testnet에서 학습·시연할 시장 상품 제품 경계다.

- **소유 범위:** AMM/LP/swap, TEST FX, perpetual 포지션·펀딩·청산,
  oracle·keeper, 견적과 앱 결과
- **주요 경계:** P06과 C5, P07과 C7, P02와 C8
- **WBS:** `WBS-P08-01`부터 `WBS-P08-07`

설계 진입점:

- [제품 WBS](../../docs/content/planning/product-wbs-overview.md#p08--dexfxperpetual-서비스)
- [시장 상품 설계](../../docs/content/specifications/market-product-design.md)
- [StableNet 호환 설계](../../docs/content/specifications/stablenet-compatibility-design.md)
- [공통 API 계약](../../docs/content/specifications/interface-contracts.md)

구현 시 계약과 off-chain service의 경계를 하위 디렉터리로 분리하고 가격
source, stale 처리, keeper 실패 수용 시험을 함께 둔다.

## 상용 계획 20의 격리 학습 프로필

`p08-isolated-8283-v1`은 loopback Anvil 8283에 따로 배포하는 학습 시장이다. 계약과 서비스가 8283, 제네시스, 배포 블록, venue·두 토큰의 코드 해시를 검사한다. 공개 StableNet 배포 및 D12·D13·D19 상용 정책 선택은 별도다. P05 주문·지급에 P08 서비스나 저장소 의존성이 없다.

| 영역 | 프로필 규칙 |
|---|---|
| 자산 | faucet이 합성하는 LAB 8자리와 LAQ 6자리. 시장 생성 시 두 개의 별도 토큰이 만들어진다. |
| 가격 | 두 개의 서로 다른 발행 주소, 출처별 단조 관측 시각, 120초 유효 기간. 최신 두 가격 간 차이 1% 초과, 미래·이전 관측, 비정상 범위, 운영 중지 시 입력 경로를 닫는다. 두 가격 평균을 LAQ 최소 단위 / LAB로 사용한다. |
| 견적 | 사용자, 동작 및 방향, 두 입력과 두 하한, 가격 버전, nonce, 최대 60초 기한. EIP-712 도메인의 salt는 실행 코드 해시다. 한 견적의 nonce는 해당 사용자와 계약에서 한 번만 소비한다. 새 견적은 새 사용자 승인 필요. |
| 교환과 LP | 상수곱 단일 풀, 교환 30bp, 주문 체결 환율의 오라클 편차 한도 5%. 지급 상한 안에서 두 토큰을 투입하고 지분을 발행한다. 지분 회수 시 두 자산별 수령 하한을 확인한다. 계약 이벤트의 실제 양과 P07 확정 영수증으로 대사한다. |
| TEST FX | 같은 LAB/LAQ 가격의 방향별 최소 단위 환산으로 정수 내림한다. 순수 모의 환전 기록이며 잔액 변경은 발생하지 않는다. 체인 실행에는 실습 가스가 들고 환전 자체 수수료는 없다. |
| 포지션 | 합성 LAQ 격리 증거금, 진입 1배 이하. 가격 손익은 선형으로 계산하고 반환 범위를 최초 증거금의 0–2배로 제한한다. 별도 예약 담보 준비금이 풀 유동성과 분리된다. 현재 평가액이 현재가 기준 명목의 10% 이하면 전액 청산 대상이다. 보상·보험·미회수 채무는 이 정책에 없다. |
| 펀딩 | 300초 기간 경계 후 첫 120초 동안 같은 기간의 두 관측이 필요하다. 현재 명목의 10bp를 매수에서 차감하고 매도에 가산한다. 위치별 기준 revision/기간을 한 번만 적용한다. 누락 기간은 자동 추정·소급하지 않고 종료 이벤트에서 미반영 개수로 표시한다. |
| 계정 | P02의 OS 보관에 생성한 별도 테스트 secp256k1 키. Android 앱별 AndroidKeyStore AES-GCM wrapping 및 iOS ThisDeviceOnly Keychain 저장. JavaScript에는 주소와 승인된 signed tx만 반환한다. iOS Release 시험 빌드와 Android debug 학습 메뉴에서 지원한다. P01 및 P03 승인 경로는 별도 등록된 학습 프로필에서만 선택할 수 있다. |

## 실행과 검증

Anvil, Forge 및 Cast 1.5.1, Go 1.26.11, PostgreSQL 15 이상을 준비한다. 신규 fixture는 [검증된 Solidity 0.8.39 빌드](../p06-stablenet-contracts/README.md#개발-df-20260925-02)를 사용한다. 새 배포의 프로필에서 계약 코드 해시를 다시 얻으며, 견적의 도메인 salt와 서명은 해당 프로필에 묶인다. 작업 구역에서 확인한 포트가 비어 있을 때 루트에서 다음 명령으로 학습용 Anvil과 API를 띄운다. fixture 역할 키는 합성 Anvil 계정만 사용한다. `profile.json`의 `deploymentHash`를 P02 학습 화면에 입력한 뒤 화면의 실습 주소에 합성 토큰과 가스를 발행한다.

```sh
bash products/p08-market-services/test/mobile-fixture.sh "$(mktemp -d)"
```

이 시험은 지정 폴더에 `profile.json`, `anvil.log`, `marketd.log`를 남긴다. 기본 Go 및 Forge 생성물과 `marketd` 실행 파일은 회차 전용 작업공간에서 사용하고 자식 프로세스 종료 직후 삭제한다. 호출자가 지정한 Go 캐시, Forge 출력·캐시 또는 배포 아티팩트 경로는 유지한다.

P02 Android debug에서는 adb reverse로 로컬 8284를 에뮬레이터의 같은 포트에 연결한다. P02 iOS simulator는 8284로 직접 연결한다. 앱이 조건과 예상 수량을 표시하면 학습 키로 토큰별 정확한 허용량과 실행을 각각 승인한다. 방송 직전 로컬 학습 기록에 원래 서명 바이트와 해시를 보관하며, 앱 종료 후 원해시로 상태를 다시 확인할 수 있다.

독립 keeper 데이터베이스에 `make -C products/p08-market-services migrate`를 실행한다. `P08_MIGRATION_DATABASE_URL`은 마이그레이션 권한, `P08_DATABASE_URL`은 `nu54_p08_keeper`를 상속한 실습 작업 권한을 사용한다. `P08_PROFILE`과 `P08_RPC`를 배포와 일치시키고 keeper 전용 `P08_KEEPER_KEY`로 `go run ./cmd/marketd keeper`를 실행한다. `P08_INSPECT_DATABASE_URL`에는 읽기 전용 `nu54_p08_inspector`를 상속한 계정을 사용한다. `go run ./cmd/marketd inspect`는 최근 50개 원작업의 포지션·종류·기간·revision·가격 버전·원해시·대사 상태를 표시한다. 두 발행 계정의 `observe(uint256,uint256)`와 운영 계정의 `setPaused(bool)`는 별도 트랜잭션이다. keeper에는 예치금 인출·거래 승인·가격 발행 권한이 없다.

- `make test P=p08`은 Foundry 및 Go 로컬 시험을 실행한다.
- `make commercial-integration-test`는 실제 8283 Anvil과 제한된 PostgreSQL keeper 역할에서 재시작·분기·경합을 검증한다. P08 fixture 종료 후 P05 등록과 매장 지급의 실제 통합 시험을 실행한다.
- `bash scripts/ios/scene-smoke/run-market.sh SIMULATOR_UUID RENTER_APP OUTPUT_DIRECTORY [entry|lp|fx|orders]`는 iOS에서 선택한 격리 시나리오를 실행한다. `lp`는 네이티브 LP 승인·재시작·P07 잔액 대사, `fx`는 방향별 환산과 잔액 보존, `orders`는 교환·LP 회수·격리 포지션 정상 경로를 검증한다. 각 시나리오는 새 fixture와 별도 비등록 simulator를 사용하며, 기존 등록이 있으면 실행을 중단한다. `RENTER_APP`에는 ad-hoc 서명한 별도 ID의 Release simulator 번들을 사용한다.
- `bash products/p08-market-services/test/mobile-fixture.sh OUTPUT_DIRECTORY bash products/p02-user-app/android/test_market_lab.sh EMULATOR_SERIAL`은 AndroidKeyStore와 JNI signer를 통해 실제 TEST FX 승인·확정 및 위변조 거절을 검증한다.

가격 소스는 수동 실습 발행자다. 공개 피드 및 연속 운영의 지연·장애 예산, 보험 또는 시장간 상계, 실자산, 메인넷은 이 프로필의 인수 범위에 없다.

### L06 운영자 페이지

`marketd ops`는 학습자 API와 별도인 `https://127.0.0.1:8286/`에서 동작한다. `P08_OPS_CA`, `P08_OPS_CERT`, `P08_OPS_KEY`로 전용 TLS 1.3 운영자 인증서를 설정한다. `P08_OPS_GRANTS`는 검증된 leaf 인증서의 SHA-256 소문자 지문을 키로 쓰는 JSON 객체다. 각 grant는 `subjectRef` UUID, `role` 값 `market-operator`, `genesis`, `deploymentHash`, `venue`, Unix 초 `expiresAt`을 가진다. 페이지를 읽기 전과 렌더링 후 전송 직전에 grant와 인증 경로를 다시 확인한다. 해지하려면 해당 grant를 제거한 새 파일을 같은 경로에 원자적으로 교체한다.

운영 링크를 열면 브라우저가 배포 CA 및 클라이언트 인증서로 TLS 연결을 수립한다. 클라이언트 인증서를 제시하지 않거나 인증서가 TLS 검증에 실패하면 브라우저의 연결 오류가 표시된다. TLS 검증을 통과하면 페이지가 시장 운영 권한을 확인한 뒤 운영 현황 또는 접근 거절을 표시한다. 역할·배포가 일치하지 않거나 grant가 만료·철회된 경우 배포 식별자, 가격과 keeper 증거가 없는 동일한 거절 안내가 표시된다. 관리자가 유효한 grant를 배포하면 같은 탭에서 **Check operator access again**으로 확인할 수 있다.

열린 탭은 2초 간격으로 권한을 재검사한다. 권한이 거절되거나 서버 연결이 끊어지면 현재 문서에서 기존 가격과 작업 상세를 지운다. 브라우저에서 탭을 떠나거나 문서를 이동할 때도 상세를 지우며, 돌아오거나 **Refresh observation**을 누르면 권한 확인 후 새 증거를 읽는다. 응답은 브라우저 캐시에 저장하지 않는다.

운영 페이지는 가격 출처별 관측 여부·신선도, 가격 불일치·중지·체인 연결 상실과 마지막 keeper 대사 상태를 제공한다. 중복 수치는 해당 배포의 저장된 확정 작업 키 중 중복 키 개수다. 작업 목록의 현재 포지션 revision은 확인된 체인 상태를 표시하며, 마지막 영속 작업 상태와 함께 대조한다. `P08_INSPECT_DATABASE_URL`은 읽기 전용 `nu54_p08_inspector` 계정을 요구한다. 개인 지갑·개인 식별자·서명 원문과 다른 배포 데이터는 페이지 범위 밖이다.

실제 Chrome 인수는 상용 통합 게이트에 `P08_TEST_OPERATOR_MTLS_BROWSER`를 `test/operator-browser.sh`의 절대 경로로 지정해 실행한다. L06만 검증하려면 PostgreSQL 도구, Foundry, Chrome과 로컬 Playwright CLI가 설치된 환경에서 `P08_PLAYWRIGHT_CLI=/absolute/path/to/playwright-cli bash products/p08-market-services/test/operator-auth-integration.sh [기존_증거_디렉터리]`를 실행한다. 임시 DB와 전용 Chrome 프로필을 사용하고 종료 시 제거한다. 증거 경로를 지정하면 인증 확인, 같은 탭의 접근 거절 및 격리 체인의 운영 현황 스냅샷을 보관한다. 같은 탭에서 grant 철회 후 상세가 사라지고 다시 승인하면 기존 거래 해시가 돌아오는지 검사한다. 인증서 없는 접속, grant 만료·철회·다른 역할·다른 배포, DB 역할 경계도 검사한다.

### 선택형 서명 지갑

- **앱 내 학습 지갑**은 기본 경로다. 별도 모바일 Keychain 또는 AndroidKeyStore 영역에서만 키를 사용한다.
- **P03 격리 학습 지갑**은 기존 P03 앱 등록과 독립 승인 영역을 사용한다. coordinator와 approval gateway에 동일한 `MarketPolicy`를 독립적으로 구성한다. 정책의 `Venue`, `Base`, `Quote`, `Signer`, `CodeHash`, `Genesis`, `Deployment`를 이 시장의 확정 배포와 대조한 뒤 등록한다. 별도 P03 DB와 주체를 사용한다. P03 DB의 지갑 프로필에 배포 해시를 고정하고 승인 화면·두 참여자 결과가 같은 거래 digest에 묶인다. 다른 프로필, 원격 임의 calldata, 네이티브 가치 송금은 해당 시장 지갑에서 거절한다.
- **P01 격리 하드웨어 지갑**은 신뢰된 P01 보드에서 별도 hardened 학습 계정을 파생한다. 기존 보드 등록과 lease, 해당 배포만 허용하는 관리 authority, 독립 표시 장치가 필요하다. 관리 authority의 `market` 프로필에는 `genesis`, `deployment`, `venue`, `base`, `quoteToken`, `quoteSigner`, `codeHash`, `quotePublicKey`, 학습 `sender`, `feeLimit`을 별도 구성한다. 독립 표시 장치는 같은 학습 프로필을 별도 경로에서 설치하고 `confirm_board.py evm --market-profile …`로 거래 의미와 한도를 검토한다. 보드는 독립 표시 서명과 물리 승인, PIN 뒤에만 서명한다.

일반 설치판의 앱 홈에는 테스트 시장 진입점이 있지만, 배포 운영자가 설치 설정을 제공할 때까지 지갑 준비와 네트워크 요청이 차단된다. 운영자는 배포가 고정된 격리 체인의 로컬 RPC 응답과 계약 코드가 검사된 뒤 공급망에서 다음 설정을 생성한다.

```sh
P08_PROFILE=/approved/profile.json P08_RPC=http://127.0.0.1:8285 \
  P08_MOBILE_ORIGIN=https://market.training.example \
  P08_MOBILE_CA_DER=/approved/training-ca.der \
  P08_MOBILE_OPERATOR_ORIGIN=https://market-ops.training.example go run ./cmd/marketd mobile-profile
```

출력 전체를 iOS 빌드 변수 `NU54_MARKET_PROFILE` 또는 Android Gradle 속성 `nu54MarketProfile`에 지정한다. DNS 호스트와 설치에 고정한 HTTPS 포트(기본 443)를 사용한다. 배포 CA의 공개 DER만 앱에 포함하고, 테스트 운영 서버 이름에 맞는 종단 인증서와 해당 개인키는 TLS 게이트웨이가 별도 관리한다. 앱은 네이티브 TLS에서 지정된 CA와 호스트만 신뢰하고 리디렉션을 거절한다. iOS의 ATS도 유지한다. iOS에서 사설 배포 CA를 사용할 경우 배포 운영자가 관리 기기의 신뢰 저장소에 같은 CA를 등록해야 한다. Android에서는 앱에 포함된 지정 CA가 이 시장 연결에만 적용된다. 게이트웨이는 승인된 P08 학습 REST 경로만 로컬 `marketd serve`에 연결한다. RPC, 운영자 mTLS, 관리 키와 PG 접속은 게이트웨이에 연결하지 않는다. 앱은 응답의 환경·체인 8283·genesis·배포 블록·가격 서명자·세 계약 주소와 코드 해시를 설치 지문과 비교한다. QA 셸에서 만든 키와 CA는 폐기용이며 지정된 테스트 앱에만 넣는다.

`P08_MOBILE_OPERATOR_ORIGIN`은 선택 사항이며 같은 설치 배포의 별도 운영 HTTPS origin만 지정한다. 해당 origin을 설치 프로필에 포함하면 학습 홈에 **시장 운영 모니터**가 표시되고 운영용 브라우저로 열린다. 운영 페이지의 **Return to market learning**은 무인자 `nu54://market-learning` 링크로 설치된 앱의 학습 홈을 연다. 브라우저에서 앱을 열 수 없으면 설치된 앱에서 **테스트 시장 학습**을 직접 선택한다. 이 링크는 배포 정보나 운영 증거를 전달하지 않는다.

`loopback`은 로컬 QA 전용 설정이다. iOS simulator에서는 `NU54_MARKET_PROFILE=loopback`으로 별도 번들을 빌드하고 Android debug는 `-Pnu54MarketProfile=loopback`을 사용한다. Android custom/release와 iOS 실제 기기에서 이 값은 준비 필요 화면으로 연결된다. 기본 릴리스 진입 테스트는 Android에서 `bash products/p02-user-app/android/test_market_entry.sh EMULATOR_SERIAL SIGNED_CUSTOM_APK`, iOS에서 `bash scripts/ios/scene-smoke/run-market.sh SIMULATOR_UUID RENTER_APP OUTPUT_DIRECTORY entry`로 실행한다. 테스트판은 다른 앱 데이터를 유지하는 별도 기기에 설치한다. Android 전체 주문 시험은 저장소 루트에서 `bash scripts/android/market/run-orders.sh EMULATOR_SERIAL NEW_EVIDENCE_DIRECTORY`로 실행한다. 로컬 SDK, JDK, Foundry 및 Node가 필요하다. 시험은 고유 앱 ID의 별도 서명 APK와 격리 체인을 만들며 기존 설치는 보존한다. 기본 시험은 주문 사이에 앱을 다시 시작하지 않는다. `NU54_ANDROID_MARKET_REENTER=1`은 화면 전환 원인을 구분하는 진단용으로, 새 주문 전에 앱을 다시 열며 연속 이용 인수와 구분한다. `NU54_ANDROID_MARKET_SCENARIO=fx`는 같은 서명·배포 절차로 가격 변경, 실제 시각 만료와 양방향 환전만 실행한다. LP 양방향, 두 자산의 체인 잔액, 원래 거래 해시로 재시작 대사, 포지션과 펀딩 롤백, 만료된 견적 및 모의 환전을 검사한다. 승인된 QA HTTPS는 `mobile-fixture.sh OUTPUT bash test/mobile-https-fixture.sh RECEIVER`로 실행한다. receiver가 `NU54_MOBILE_QA_PROFILE`로 전용 앱을 빌드하고, iOS 전용 simulator에는 QA fixture의 공개 인증서 `NU54_MOBILE_QA_CA`만 등록한다. Android 전용 emulator에서는 QA용 포트만 `adb reverse tcp:9443 tcp:9443`로 전달한다.

학습 시장 홈만 확인하려면 같은 Android 명령에 `--home-only`를 추가한다. 서명된 앱의 일반 홈과 학습 복귀 링크에서 테스트넷 고지, 주제 선택, 여행으로 이동하고 같은 시장으로 돌아오는 동작을 확인한다. 운영자 원본이 없는 시험판의 운영자 인증은 비활성이다. 이 시험은 학습 지갑을 등록하거나 견적을 실행하지 않는다. APK·로컬 체인과 빌드 캐시는 시험 종료 때 제거하고 입력한 기기는 호출자가 소유한다.

풀 선택만 확인하려면 같은 Android 명령에 환경 변수 `NU54_ANDROID_MARKET_SCENARIO=liquidity-pool`을 지정한다. 등록된 학습 지갑에서 실제 100 LP를 준비한 뒤 승인된 풀 주소와 잔액을 확인한다. 공급과 회수 검토를 각각 취소하고 학습 홈에서 다시 진입할 때 기존 풀, 두 테스트 토큰과 LP 잔액, 지갑 거래 순번이 같은지 검사한다. 실행 기록만 보관하고 로컬 체인과 설치판을 제거한다.


신규 허용 거래는 기존 ERC-20 `approve`의 지출자·수량 뒤에 원 견적의 EIP-712 지문 32바이트를 붙여 서명한다. P08 릴레이는 원 견적의 발행 서명, 지갑, 허용 대상과 수량, 현재 체인의 가격 버전·기한을 대조한 뒤 전송한다. 지문이 없는 구형 허용 거래는 노드에 이미 알려진 동일 해시의 조회에만 사용할 수 있다. 구형 보드로 신규 토큰 허용을 승인하려면 새 형식을 지원하는 펌웨어로 갱신해야 한다. 원래 서명한 해시의 확인은 기존 조회 경로를 사용한다. `LabToken.approve`는 추가 지문이나 가격 기한을 검사하지 않으므로 이 보호의 범위는 승인된 앱과 P08의 전송 경로다. 포지션 실행과 keeper 청산은 시장 계약 자체에서 가격을 다시 검증한다. 저장소 루트에서 `python3 -B products/p08-market-services/test/relay-bound-allowance.py`를 실행하면 격리 체인의 정상 허용과 견적 바꿔치기·오래된 가격·원해시 복구를 C 앱 서명기 및 보드 호스트로 대조한다.

### 설치판 인수 범위

- iOS 27 simulator의 서명된 Release 앱에서 설정 없는 홈 진입과 차단을 확인했다. 전용 QA CA를 기기에 등록한 별도 HTTPS 설치에서 두 토큰 사용 허용, 유동성 공급·회수, 교환, 같은 원해시로 재시작 대사, 격리 포지션·펀딩 이력·수동 종료, 변경된 관측에 따른 기존 견적 거부와 재견적을 확인했다. 실제 iOS 기기는 배포 운영자가 등록한 신뢰 체인으로 별도 인수한다.
- Android 34 emulator의 서명된 custom release에서 설정 없는 홈 진입과 차단, 내장 QA CA로 검증한 HTTPS 배포의 지갑 등록·재시작·실시간 잔액 갱신을 확인했다. Android release 화면에서 두 토큰 사용 허용, 100 LP 공급, 2 LAB → 2.34602 LAQ 교환을 실제 체인에서 확인했다. 직전 영수증과 새 견적이 겹칠 때 Android 승인 입력이 전달되지 않는 결함을 재현했다. 가장 최근의 확정·거절 영수증을 학습 지갑 저널에 저장하고 별도 영수증 화면으로 분리했다. 수정 설치에서 한 화면으로 두 사용 허용, 100 LP 공급, 2 LAB → 2.34602 LAQ 교환 후 단일 승인으로 10 LP를 회수했다. 재시작 후 동일 원해시로 10.2 LAB과 11.765398 LAQ의 수령 및 최종 90 LP를 대사했다. 롱 개설과 수동 종료, 펀딩 이력의 원해시와 reorg 복구, 숏 개설과 별도 keeper 권한에 의한 청산도 같은 실행에서 확인했다. FX 전용 재시험에서는 변경된 시세와 실제 승인 기한을 지난 견적을 거절한 뒤 새 견적의 원해시와 토큰 잔액 보존을 확인했다. 견적 요청 사이에 실제 체인 tip이 전진하면 응답 시점의 새 헤드에서 배포, 가격 버전 및 60초 기한을 다시 검사한다.
- L06은 격리 Anvil과 PostgreSQL에서 실제 mTLS Chrome 조회를 실행한다. 일시 정지, 두 가격의 만료, 새 관측 후 재개 동안 원래 keeper 거래 해시와 revision 3을 유지하고 중복 확정은 0이다. 인증서 없음, 잘못된 역할·배포와 철회에는 원해시를 공개하지 않는다. 테스트 체인은 시간 이동을 거쳐 실제 시각에 다시 도달할 수 있도록 과거 시각에서 시작한다.

P03 실제 threshold와 체인 대사는 fixture 실행 중 `P03_TEST_MARKET_PROFILE=$P08_PROFILE make -C products/p03-cloud-mpc-wallet db-test`로 검증한다.
같은 fixture에서 `NU54_P03_BROWSER_MARKET=1 bash products/p03-cloud-mpc-wallet/test/run_ios_browser.sh SIMULATOR_UUID APP_DERIVED_DATA_DIRECTORY`를 실행하면 일반 P02 시장 화면에서 P03 계정을 선택하고 Safari 독립 승인과 원 거래 대사를 함께 검증한다. 기존 P03 테스트 계정 및 시뮬레이터 학습 기록을 유지하기 위해 처음 실행하는 전용 `org.nu54.p03browsertestmarket.*` 앱과 일회용 데이터베이스를 사용한다. `NU54_P03_BROWSER_EXPIRED=1`을 추가하면 만료된 주문의 서명 거부와 새 견적을 확인한다.
 P01 호스트 단계는 동일한 원견적을 사용해 보드 관리 proof, 독립 표시 challenge, 기기 승인 완료 서명을 EIP-155 서명과 실제 체인 효과에 대조한다. 호스트 인수 환경은 BLE 물리 전송과 보드 펌웨어 설치를 대체하지 않는다.

시장 서버는 새 견적·사용자 거래 릴레이·keeper 송신 전에 헤드 시각을 로컬 운영 시계와 대조한다. 앞선 시각이 10초를 넘어가거나 지연이 30초를 넘어가면 새 전송을 중지한다. 기존 작업의 원래 확정 증거 조회는 계속 제공한다.

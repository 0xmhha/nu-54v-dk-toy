# 프로토콜·라이브러리·저장 방식 선택안

2026-09-18. 19개 기술 선택 카드와 23개 검증 작업을 기존 D01~D19 및 WBS에 연결했다. 전체 15개 기능·앱 연동·12주 완료 전제와 3명 기준을 유지한다. 검증 작업은 기존 세부 작업의 실행/증거를 구체화한 것이며 별도 인력·공수·주차 배정이 아니다.

**작업용 기본 방향은 RN 앱·키오스크, Go 업무 API, PostgreSQL 원장/보안 상태, 보호 객체 저장소, NU용 Zephyr 기반 펌웨어, MCUboot/SMP FOTA, 기존 indexer·계약·viem 계열 재사용이다.** 사용자가 확정한 Zephyr 기반·RN 키오스크·체인 8283·EOA 우선·고객 가스·스마트 계정의 12주 내 지원은 유지한다. Zephyr의 SDK 배포판(NCS 포함 여부)·정확한 revision과 나머지 라이브러리는 검증 전 제안이다.

[선택 원본](technology-selection.json) · [검증 작업 카드](technology-validation-plan.md) · [관찰한 버전](technology-version-baseline.json) · [출처 기록](technology-sources.json) · [기존 API 통합](../specifications/security-api-integration.md)

## 선택 방법과 먼저 확인할 조합

1. **공통 실행 환경:** TECH-01/03/04/06으로 RN native bridge·Go/DB·NU 빌드가 연결되는지 확인한다. 제품별 adapter 경계를 유지하고 새로운 마이크로서비스 19개를 만들지 않는다.
2. **구현 경로를 결정하는 조건:** TECH-07/08/10/12/14/17의 key backend·FOTA·패스키·MPC·UserOperation·x402를 먼저 검증한다. 사용자가 말한 대로 실기 검증은 실제 개발 단계에 수행하며, 이번에는 실험 입력과 판정을 준비한다.
3. **기능 품질과 운영:** 오디오/가격/장소/AI/복구는 실제 표본과 출처를 포함해 검증한다. 참고 구현이 있다는 사실과 앱에서 기능이 동작한다는 증거를 구분한다.

검증 뒤 선택 기록에 후보 revision, 환경, 통과/실패 이유, 대안, 적용 API/작업, 남은 제한을 넣는다. 통과하지 못하면 adapter를 수정하거나 대안을 검증하며 원래 기능을 자동 제외하지 않는다. 제공자 비용·보관 정책·운영 권한 같은 사용자 결정이 필요한 항목은 선택 이유와 영향을 구체화한 뒤 묶어 결정한다.

## 기존 코드에서 확인한 버전과 재사용 경계

| 대상 | 이번에 읽은 선언 | 처리 방침 |
|---|---|---|
| indexer-go | Go 1.24.0, toolchain go1.24.9, chi v5.2.3, Pebble v1.1.5, go-ethereum v1.16.5 | 같은 commit의 동작을 재현한 뒤 지원 버전·취약점·호환성 검토 결과로 pin |
| poc-platform | pnpm 10.30.3, Node >=22, TypeScript ^5.7.2, viem ^2.46.3 | 선언 범위가 실제 resolved 버전이라는 뜻은 아님. lockfile과 RN 적합성 검증 |
| 웹 앱 | Next ^15.5.12, React ^19.0.0 | 웹 백오피스 재사용 후보. RN에 웹 DOM/storage 코드를 그대로 가져오지 않음 |
| stable-poc-contract | solc 0.8.28, EVM prague, optimizer 200 | 체인 fork 기능 및 실제 bytecode/ABI를 확인. compiler 설정만으로 체인 지원 단정 금지 |

위 숫자는 **기존 소스에서 확인한 선언**이지 최신 버전 추천이나 설치 결과가 아니다. 기존 검토 commit은 [버전 기록](technology-version-baseline.json)에 보존했다. Nordic latest 문서에 보이는 개발 버전도 자동 채택하지 않으며 NU board definition과 기능을 함께 검증한 release/commit을 고정한다.

## 호환성 검토에서 드러난 구체 조건

- react-native-ble-plx는 GATT central 작업의 후보이며, README상 bonding/peripheral 기능을 제공하지 않는다. 필요한 본딩 native adapter와 CS peer 연결을 따로 검증한다. [공식 저장소](https://github.com/dotintent/react-native-ble-plx)
- cb-mpc는 암호 primitive 라이브러리이고 공개 지원 환경은 macOS/Linux 중심이다. peer 인증·저장·복구 운영과 RN 모바일 참여자 구현은 별도다. [공식 설명](https://github.com/coinbase/cb-mpc)
- LFDT의 cggmp21 이름을 가진 저장소는 현재 README에서 CGGMP24를 설명하고 key refresh 미지원을 명시한다. 이름만 보고 기존 복구/회전 요구를 충족한다고 선정하지 않는다. [공식 README](https://github.com/LFDT-Lockness/cggmp21)
- x402 exact EVM은 특정 token authorization 경로를 요구한다. 현재 일반 ERC20Mock으로 이를 충족한다고 볼 수 없으며, facilitator의 실제 gas payer가 고객 가스 원칙과 일치하는지도 검증한다. 표준 exact 지원을 주장하려면 해당 규격 시험을 통과해야 한다. custom mechanism은 지원 상대와 규격 차이를 명시한다. [EVM 구현 문서](https://pkg.go.dev/github.com/coinbase/x402/go/mechanisms/evm)
- MCUboot/SMP 지원 자료는 출발점이다. NU의 실제 partition·앱 크기·설정 보존과 모바일 전송을 같은 조합으로 검증해야 한다. [Nordic FOTA](https://nrfconnectdocs.nordicsemi.com/ncs/latest/nrf/app_dev/device_guides/nrf54l/fota_update.html)

Cloud MPC의 2-of-3를 선택한다면 앱 참여자·서버 참여자·독립 복구 참여자의 신뢰/저장 경계를 먼저 정의한다. 서버 운영자 권한 하나로 두 share를 모두 얻을 수 있는 배치는 피한다. 소셜 로그인과 quorum 참여 권한은 분리하며, 기존 HW 단독 승인 기능에 폰 co-sign을 추가로 강제하지 않는다. 라이브러리의 refresh와 participant replacement/recovery는 같은 기능으로 뭉뚱그리지 않고 각각 시험한다.

## 19개 선택 카드

### TECH-01 · RN 앱·키오스크

- 작업용 선택안: TypeScript React Native native-project 기반 앱 2개 + 공통 domain/API 패키지
- 비교 대안: Expo development build + config plugins
- 채택 조건: 키오스크 RN은 사용자 확정. 유저 앱 RN과 monorepo는 제안. BLE/FOTA/MPC native bridge의 Android/iOS 빌드를 먼저 확인
- 연결: D01; APP-01, SHOP-01, BASE-03
- 검증: VAL-01; 현재 미실행
- 상태: 제안. 라이브러리/프로토콜 버전 미선정.
- 근거: [react-native-ble-plx](https://github.com/dotintent/react-native-ble-plx), [Expo BLE development build](https://expo.dev/blog/how-to-build-a-bluetooth-low-energy-powered-expo-app)

### TECH-02 · Google·Apple 인증

- 작업용 선택안: provider/OS별 공식 SDK 또는 시스템 인증 UI adapter + 서버 검증
- 비교 대안: 지원 SDK 경로가 다른 OS는 별도 profile
- 채택 조건: API-103 flow와 API-001/002 providerProof를 결합; 같은 PKCE 필드를 두 제공자에 강제하지 않음
- 연결: D01, D03; AUTH-02, AUTH-03, AUTH-04, BASE-05
- 검증: VAL-02; 현재 미실행
- 상태: 제안. 라이브러리/프로토콜 버전 미선정.
- 근거: 기존 요구·API 설계와 [로컬 소스 선언](technology-version-baseline.json). 새 외부 호환성 검증을 의미하지 않음.

### TECH-03 · 업무 API·백오피스

- 작업용 선택안: Go chi 기반 업무 API + TypeScript 웹 백오피스; 기존 Next 화면 구성 재사용 검토
- 비교 대안: TS 업무 API로 통일하되 Go indexer는 adapter로 유지
- 채택 조건: 기존 Go/TS 코드를 고려한 제안. explorer를 점주/운영 권한 서비스로 그대로 사용하지 않음
- 연결: D02, D08, D19; BASE-04, AUTH-01, OPS-01, SHOP-05
- 검증: VAL-03; 현재 미실행
- 상태: 제안. 라이브러리/프로토콜 버전 미선정.
- 근거: [pgx](https://github.com/jackc/pgx)

### TECH-04 · 업무·보안 상태 저장

- 작업용 선택안: PostgreSQL + pgx; 업무 원장·ReceiptOwnership·pending entitlement·outbox를 같은 DB transaction에 배치
- 비교 대안: Redis는 단기 캐시만 비교; 별도 원장 adapter는 복구 protocol 증거가 있을 때
- 채택 조건: 61개 SQL 참조안은 재사용. 15개 논리 자원은 필요 DDL/권한 mapping을 새로 작성; 암호키는 업무 DB 밖
- 연결: D02, D08, D09, D19; BASE-05, PAY-04, STAMP-01, AUTH-04
- 검증: VAL-04; 현재 미실행
- 상태: 제안. 라이브러리/프로토콜 버전 미선정.
- 근거: [pgx](https://github.com/jackc/pgx), [PostgreSQL locking](https://www.postgresql.org/docs/current/explicit-locking.html)

### TECH-05 · 보호 객체·작업 큐

- 작업용 선택안: private S3-compatible object adapter + 인증 gateway; PostgreSQL outbox polling worker
- 비교 대안: 운영 S3+KMS 또는 선택 자체 object store; broker는 처리량 근거가 생기면 비교
- 채택 조건: 암호화와 소유 ACL 분리. 선택 object store가 SSE/KMS를 동일하게 지원한다고 가정하지 않음
- 연결: D17, D19; BASE-05, REC-03, AI-02, RELEASE-02
- 검증: VAL-05; 현재 미실행
- 상태: 제안. 라이브러리/프로토콜 버전 미선정.
- 근거: [S3 server-side encryption](https://docs.aws.amazon.com/AmazonS3/latest/userguide/serv-side-encryption.html)

### TECH-06 · NU 보드·RTOS

- 작업용 선택안: Zephyr 기반 펌웨어 확정; NU board definition과 검증된 Zephyr revision을 고정
- 비교 대안: SDK 배포 경로: upstream Zephyr 또는 Zephyr 기반 NCS를 기능·보드 지원으로 비교
- 채택 조건: Zephyr 사용은 확정. NCS 채택/버전·NU board target·crypto/controller/FOTA 조합은 별도 검증
- 연결: D05, D07; HW-01, HW-07, OTA-01
- 검증: VAL-06; 현재 미실행
- 상태: Zephyr 기반은 사용자 확정. SDK 배포판/버전·보드 target 미선정.
- 근거: [Nordic nRF54L FOTA](https://nrfconnectdocs.nordicsemi.com/ncs/latest/nrf/app_dev/device_guides/nrf54l/fota_update.html), [Nordic Channel Sounding](https://www.nordicsemi.com/Products/Wireless/Bluetooth-Low-Energy/Channel-Sounding), [Nordic crypto supported operations](https://nrfconnectdocs.nordicsemi.com/ncs/latest/nrf/security/crypto/crypto_supported_features.html), [Zephyr application development](https://docs.zephyrproject.org/latest/develop/application/index.html), [Zephyr sysbuild](https://docs.zephyrproject.org/latest/build/sysbuild/index.html), [Zephyr MCUmgr](https://docs.zephyrproject.org/latest/services/device_mgmt/mcumgr.html)

### TECH-07 · HW 키·암호 backend

- 작업용 선택안: NCS 지원 crypto/보호영역을 adapter로 사용; EOA secp256k1과 passkey curve의 지원을 각각 검증
- 비교 대안: 지원 backend 부족 시 검토된 software crypto 또는 외부 보안 모듈 비교
- 채택 조건: KMU/TrustZone 존재만으로 비추출 secp256k1 서명 완료라고 보지 않음. 수제 키 분할 재조립을 MPC로 부르지 않음
- 연결: D03, D06; HW-02, HW-04, HW-05, KEY-02
- 검증: VAL-07, VAL-20; 현재 미실행
- 상태: 제안. 라이브러리/프로토콜 버전 미선정.
- 근거: [Nordic crypto supported operations](https://nrfconnectdocs.nordicsemi.com/ncs/latest/nrf/security/crypto/crypto_supported_features.html)

### TECH-08 · FOTA·부트 복구

- 작업용 선택안: MCUboot signed image + MCUmgr SMP/BLE + 모바일 native manager bridge
- 비교 대안: NU flash layout상 외부 update bank 필요 여부를 비교
- 채택 조건: legacy Nordic DFU 라이브러리를 SMP 대체로 쓰지 않음. SDK/bootloader/partition/settings 버전을 한 묶음으로 고정
- 연결: D05; OTA-01, OTA-02, OTA-03, OTA-04
- 검증: VAL-08; 현재 미실행
- 상태: 제안. 라이브러리/프로토콜 버전 미선정.
- 근거: [Nordic nRF54L FOTA](https://nrfconnectdocs.nordicsemi.com/ncs/latest/nrf/app_dev/device_guides/nrf54l/fota_update.html), [iOS McuManager](https://github.com/nordicsemi/IOS-nRF-Connect-Device-Manager)

### TECH-09 · BLE 세션·거리·찾기

- 작업용 선택안: react-native-ble-plx GATT central + 필요한 native bonding bridge; NCS CS initiator/reflector를 별도 peer 경로로 연결
- 비교 대안: GATT 앱 라이브러리 교체 또는 직접 native adapter
- 채택 조건: BLE 라이브러리는 CS API와 앱 계층 peer 인증을 대신하지 않음. 외부 거리 peer가 소유한 USB/폰 연결은 별도 profile
- 연결: D02, D07; HW-03, HW-06, FIND-01, FIND-02
- 검증: VAL-09; 현재 미실행
- 상태: 제안. 라이브러리/프로토콜 버전 미선정.
- 근거: [Nordic Channel Sounding](https://www.nordicsemi.com/Products/Wireless/Bluetooth-Low-Energy/Channel-Sounding), [react-native-ble-plx](https://github.com/dotintent/react-native-ble-plx)

### TECH-10 · 표준 패스키

- 작업용 선택안: CTAP2 authenticator와 실제 RP 등록/인증부터 검증; NU가 key/사용자 승인 담당
- 비교 대안: 필요 시 USB HID 지원 companion MCU를 transport bridge로 추가
- 채택 조건: NU USB 단자가 곧 CTAP USB endpoint라는 가정 금지. BLE 앱 challenge signer만으로 표준 passkey 완료라 하지 않음
- 연결: D06, D09; KEY-01, KEY-02, KEY-03, STAMP-04
- 검증: VAL-10, VAL-21; 현재 미실행
- 상태: 제안. 라이브러리/프로토콜 버전 미선정.
- 근거: [CTAP 2.2](https://fidoalliance.org/specs/fido-v2.2-ps-20250714/fido-client-to-authenticator-protocol-v2.2-ps-20250714.html)

### TECH-11 · 녹음 전송·모바일 보관

- 작업용 선택안: sequence/timestamp 포함 custom GATT audio framing; mono PCM으로 측정 후 ADPCM 등 비교
- 비교 대안: 정식 LE Audio는 실제 controller/OS/SDK 경로가 증명되면 별도 비교
- 채택 조건: BLE 6 표기만으로 LE Audio 가능이라 단정하지 않음. 코덱·sample rate·MTU·버퍼를 측정 결과로 선정
- 연결: D07, D17; REC-01, REC-02, REC-03, HW-07
- 검증: VAL-11; 현재 미실행
- 상태: 제안. 라이브러리/프로토콜 버전 미선정.
- 근거: [react-native-ble-plx](https://github.com/dotintent/react-native-ble-plx)

### TECH-12 · Cloud Wallet MPC

- 작업용 선택안: cb-mpc는 backend protocol 평가 후보; 모바일 참여자까지 포함한 후보 비교 후 선정
- 비교 대안: LFDT Lockness 현재 구현을 비교하되 필요한 refresh/복구가 없는 revision은 채택 보류
- 채택 조건: 공통 native profile·독립 참여자·2-of-3 복구 구조는 제안. cb-mpc의 공개 지원 환경만으로 RN 모바일 지원 주장 금지
- 연결: D03, D04; MPC-01, MPC-02, MPC-03, MPC-04, MPC-05
- 검증: VAL-12, VAL-22; 현재 미실행
- 상태: 제안. 라이브러리/프로토콜 버전 미선정.
- 근거: [Coinbase cb-mpc](https://github.com/coinbase/cb-mpc), [LFDT Lockness threshold ECDSA](https://github.com/LFDT-Lockness/cggmp21)

### TECH-13 · EOA·토큰·SDK

- 작업용 선택안: 기존 viem 기반 core adapter + Foundry 계약; 제한 mint 더미 USDC와 native WKRC 구분
- 비교 대안: RN 비호환 SDK 부분만 공통 adapter로 추출
- 채택 조건: 기존 package range는 설치된 최신 버전이 아님. chain 8283/fork/ABI/단위와 signer 입력을 manifest로 고정
- 연결: D08, D10; TOKEN-01, TOKEN-02, TOKEN-03, PAY-01, INDEX-02
- 검증: VAL-13; 현재 미실행
- 상태: 제안. 라이브러리/프로토콜 버전 미선정.
- 근거: 기존 요구·API 설계와 [로컬 소스 선언](technology-version-baseline.json). 새 외부 호환성 검증을 의미하지 않음.

### TECH-14 · 스마트 계정

- 작업용 선택안: 기존 account 계약·SDK·Bundler 중 호환되는 하나의 ERC-4337 bundle 선정
- 비교 대안: Kernel 등 대체 계정은 전체 bundle 호환 증거로 비교
- 채택 조건: EntryPoint 주소/코드/ABI·UserOp packing/hash·nonce·signature·receipt를 함께 고정; EOA 다음이지만 12주 내부 범위
- 연결: D11, D10; SMART-01, SMART-02, SMART-03, BASE-06
- 검증: VAL-14; 현재 미실행
- 상태: 제안. 라이브러리/프로토콜 버전 미선정.
- 근거: 기존 요구·API 설계와 [로컬 소스 선언](technology-version-baseline.json). 새 외부 호환성 검증을 의미하지 않음.

### TECH-15 · DeFi·FX·Perpetual

- 작업용 선택안: 기존 swap/LP 코드 우선 검토; FX 시험 USD/KRW 현물쌍; Perp 가격/펀딩/청산 별도 engine
- 비교 대안: 다른 DEX/perp 구현은 invariants와 ABI/indexer 이식 비용으로 비교
- 채택 조건: 현재 Perp 고정가격/TODO를 오라클/청산 완료로 취급하지 않음. 시장 규칙과 정밀도는 구현 선택 입력
- 연결: D12, D13; DEX-01, DEX-02, FX-01, FX-02, PERP-01, PERP-02, PERP-04, PERP-06
- 검증: VAL-15; 현재 미실행
- 상태: 제안. 라이브러리/프로토콜 버전 미선정.
- 근거: 기존 요구·API 설계와 [로컬 소스 선언](technology-version-baseline.json). 새 외부 호환성 검증을 의미하지 않음.

### TECH-16 · DID·STO 자격

- 작업용 선택안: VC Data Model 2.0 기반 앱 자격 + 등록 issuer/status adapter; STO는 제한 전송 시험 계약
- 비교 대안: DID method/proof suite는 대상 verifier와 실제 발급/철회 호환을 비교
- 채택 조건: 데이터 모델만 선정해 표준 DID/proof 구현 완료라 하지 않음. 개인정보는 체인에 직접 기록하지 않는 제안
- 연결: D14, D15; DID-01, DID-02, DID-03, STO-01, STO-02
- 검증: VAL-16; 현재 미실행
- 상태: 제안. 라이브러리/프로토콜 버전 미선정.
- 근거: [VC Data Model 2.0](https://www.w3.org/TR/vc-data-model-2.0/)

### TECH-17 · x402 유료 자원

- 작업용 선택안: x402 v2의 network eip155:8283 및 token mechanism을 명시한 private verifier/facilitator 평가
- 비교 대안: EIP-3009 호환 시험 토큰 또는 Permit2 경로 비교; 필요 시 명시적 custom customer-paid mechanism
- 채택 조건: 일반 ERC20Mock만으로 exact mechanism 지원을 가정하지 않음. 고객 gas 원칙과 facilitator settlement gas 부담의 일치가 선택 조건
- 연결: D16, D10; X402-01, X402-02, X402-03, TOKEN-02
- 검증: VAL-17, VAL-23; 현재 미실행
- 상태: 제안. 라이브러리/프로토콜 버전 미선정.
- 근거: [x402 v2 specification](https://github.com/x402-foundation/x402/blob/main/specs/x402-specification-v2.md), [x402 EVM implementation](https://pkg.go.dev/github.com/coinbase/x402/go/mechanisms/evm)

### TECH-18 · 장소·전사·여행 AI

- 작업용 선택안: Kakao Local 장소 검색 adapter 우선 평가; 서버 whisper.cpp 전사·llama.cpp 추론 후보와 관리형 API 비교
- 비교 대안: 다른 장소/모델 제공자는 한국 장소 일치·다국어·동의/삭제·지연 기준으로 비교
- 채택 조건: 장소 데이터에 영업시간/경로가 항상 있다고 가정하지 않음. 모델과 cloud vendor는 실행·정책 검증 후 결정
- 연결: D17, D18; TRIP-01, TRIP-02, TRIP-04, AI-01, AI-02, AI-03, REC-03
- 검증: VAL-18; 현재 미실행
- 상태: 제안. 라이브러리/프로토콜 버전 미선정.
- 근거: [Kakao Local API](https://developers.kakao.com/docs/ko/local/dev-guide), [whisper.cpp](https://github.com/ggml-org/whisper.cpp), [llama.cpp](https://github.com/ggml-org/llama.cpp)

### TECH-19 · Indexer·관측·출시

- 작업용 선택안: 기존 Go/Pebble indexer 유지 + 업무 API adapter; 기존 Prometheus 계열 관측과 release manifest
- 비교 대안: 새 indexer/메시지 broker는 기존 복구 시험 실패 원인에 따라 비교
- 채택 조건: Pebble을 업무 원장 DB로 무리하게 통합하지 않음. source commit과 배포 ABI/블록/decoder 버전을 기록
- 연결: D02, D19; INDEX-01, INDEX-03, INDEX-04, INDEX-06, RELEASE-01, RELEASE-02, RELEASE-03
- 검증: VAL-19; 현재 미실행
- 상태: 제안. 라이브러리/프로토콜 버전 미선정.
- 근거: 기존 요구·API 설계와 [로컬 소스 선언](technology-version-baseline.json). 새 외부 호환성 검증을 의미하지 않음.

## 채택 전 남길 산출물

- source/lockfile/toolchain/model digest를 포함한 재현 manifest와 license 검토 기록.
- MCU key handle·MPC signer·BLE/FOTA·API/schema·object/DB의 producer/consumer 호환 행렬.
- 기능별 성공/실패/중단 복구 결과와 민감정보를 제거한 증거.
- 허용 지연·자원 예산·TTL·철회 SLA·보관 수명의 실제 설정값. 미측정 값을 사용자 완료 약속으로 꾸미지 않는다.

이번 검사는 계획 ID·원문 결정·작업·출처·검증 카드 연결의 일관성이다. 라이브러리 설치, provider 등록, firmware 빌드/플래시, 컨트랙트 배포나 실결제는 수행하지 않았다.

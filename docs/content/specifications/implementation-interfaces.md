# Zephyr 기반 구현 인터페이스와 호환 기준

사용자 결정: **“펌웨어는 zephyr 기반으로 할꺼야.”** 펌웨어 RTOS는 Zephyr로 확정했다. Zephyr revision, SDK 배포판(NCS 포함 여부), NU board target, toolchain, crypto/controller 조합은 아직 선정하지 않았다. Zephyr 선택을 NCS 전체 채택이나 특정 버전 확정으로 확대하지 않는다.

[구현 인터페이스 원본](implementation-interfaces.json) · [버전 호환 행렬](compatibility-matrix.md) · [미선정 manifest 템플릿](compatibility-manifest.template.json) · [형태 schema](compatibility-manifest.schema.json)

14개 구성요소 경계와 26개 호환 판정 항목을 기존 API·BLE·기술 검증 작업에 연결했다. 전체 15개 요구·12주 앱 연동 범위는 유지한다. 이 문서는 구현을 나누는 계약이며 실제 C ABI·GATT wire·FOTA 패키지·제품 코드를 생성한 결과는 아니다. 담당자·공수는 정하지 않았다.

## 1. Zephyr 프로젝트의 구성 제안

Zephyr의 application 구성은 CMake, Kconfig 설정, devicetree overlay와 소스를 함께 관리한다. 이에 맞춰 보드 구성과 제품 로직을 분리하는 구조를 제안한다. [Zephyr application 문서](https://docs.zephyrproject.org/latest/develop/application/index.html)

```text
firmware/                      # 다음 구현 단계의 경로 제안, 아직 생성하지 않음
  west.yml                     # 선정한 Zephyr/모듈 commit 고정
  app/
    CMakeLists.txt
    Kconfig
    prj.conf
    VERSION
    boards/                    # 검증된 NU target의 application overlay
    src/
      platform/                # 보드·시간·저장·crypto adapter
      session/                 # 역할/인증·명령 분배
      wallet/                  # 키 handle·intent 검토·서명
      update/                  # 업데이트/부팅 확인
      authenticator/           # CTAP 및 credential 관리
      features/                # 녹음·찾기·스탬프
      arbitration/             # 서명/무선/flash/오디오 자원 중재
    tests/                     # 단위/통합/실기 시험 구성
  board-support/               # 제조사 정의 재사용 또는 필요한 외부 board port
  release/                     # build·partition·key schema·profile manifest
```

디렉터리를 나누는 것 자체가 secure execution 경계는 아니다. key service는 실제 메모리/실행 권한·crypto backend를 검증해 격리하며, 일반 함수 호출로 private key를 반환하지 않는다. Zephyr 설정 이름이나 NU board target은 공급 정의를 확인한 후 고정하고 임의 target 문자열을 빌드 성공값으로 기록하지 않는다.

부트로더와 application 조합은 sysbuild 검토 대상으로 둔다. MCUboot/SMP FOTA는 작업용 제안이며, bootloader/partition/key schema 조합과 복구 증거로 채택한다. [Sysbuild](https://docs.zephyrproject.org/latest/build/sysbuild/index.html), [MCUmgr](https://docs.zephyrproject.org/latest/services/device_mgmt/mcumgr.html)

## 2. 인터페이스 공통 규칙

- 입력은 호출자/세션 권한, requestId, 목적, source/정책 revision, 관련 만료를 포함하는 context에 결합한다. 아래 method 이름과 shape는 내부 논리 계약이며 확정된 함수 signature가 아니다.
- key/session/review handle은 해당 소유·세션·세대 범위에서만 유효하다. handle을 알거나 public metadata를 읽었다는 이유로 키 사용 권한을 주지 않는다.
- 전송 ACK, 작업 접수, 사용자 승인, 서명 완료, 체인 실행, 업무 원장 수락은 각각 다른 상태다. 비동기 호출은 요청/작업 ID로 결과를 재조회한다.
- 타임아웃은 작업 실패와 동일하지 않다. 기기 로컬 deadline은 monotonic 시간 기준, 서버/온체인 만료는 선택 profile의 검증 시간 기준이다. 동기화되지 않은 시계를 동일한 시각으로 취급하지 않는다.
- 취소는 새 실행 방지와 현재 중재 상태를 반환한다. 이미 나온 서명이나 broadcast된 거래를 무효화했다고 표시하지 않는다. 늦게 도착한 결과는 새 세션 결과로 전용하지 않는다.
- 오류에는 `UNSUPPORTED_PROFILE`, `INCOMPATIBLE_VERSION`, `FORBIDDEN`, `BUSY`, `EXPIRED`, `CANCELLED`, `RESULT_UNKNOWN`, `RECOVERY_REQUIRED`의 의미를 구분하는 adapter 규칙을 둔다. 실제 API별 공개 오류는 기존 카탈로그를 따른다.
- 재부팅·앱 재시작 후에는 persisted request/source와 실제 key/firmware 상태를 대조한다. 이전 버튼 승인을 자동 재실행하지 않는다.
- 녹음 buffer·무선 queue·flash 작업에는 상한을 두고 overflow/backpressure를 명시한다. 수치는 실측 검증 후 선정하며 무제한 queue를 사용하지 않는다.

## 3. 호환성을 판정하는 순서

1. public device.info는 탐색용으로만 사용한다. 모델·기능 후보를 찾은 뒤 **선택한 protocol/profile·역할·버전을 인증된 handshake에 결합**한다.
2. 요청 동작이 필요한 key/curve/encoding/transport capability와 맞는지 검사한다. 버전 문자열 하나나 기능 광고만으로 허용하지 않는다.
3. producer/consumer build와 필요한 pin의 정확한 조합에 시험 증거가 있는지 확인한다. semantic version minor 차이는 자동 호환으로 간주하지 않는다. 명시적인 adapter/지원 범위와 시험 증거가 필요하다.
4. 실제 principal·만료·source·동의를 다시 검사하고 작업을 실행한다. 호환성은 권한 검사의 대체물이 아니다.
5. 불일치면 영향받는 새 동작을 거절하고 업데이트/복구 경로를 표시한다. 약한 암호 profile로 자동 낮추거나 일반 송금/단순 서명으로 바꾸지 않는다. 기존 제출 거래의 관측은 계속한다.

### FOTA의 세 방향

- **기존 FW→신규 FW:** partition·image format·서명키·데이터 migration·key handle/주소 보존.
- **부팅 실패→이전 FW 복귀:** 이전 버전이 변경된 저장 schema를 읽을 수 있는지 확인. 복귀 불가능한 migration은 별도 복구 경로/정책 없이는 적용하지 않는다.
- **구버전 앱→신규 FW:** 지원되는 조회/명령과 거절되는 명령을 구분하고 앱 업데이트를 안내한다. 별도 SMP 서비스가 열려 있어도 payment_terminal/ranging_peer가 FOTA로 우회하지 못하게 한다.

### 서명 종류별 조합

EOA transaction, typed data, UserOperation, passkey assertion, DID proof, x402 proof를 별도로 검증한다. 각 행은 curve·hash/domain·encoding·review decoder·signer·검증 주체 버전을 연결한다. 고정 입력에 대한 기기/앱 digest 일치와 실제 verifier 수락, 다른 source/domain/chain의 거절 증거를 남긴다. passkey는 RP/CTAP 경로이고 앱의 일반 wallet.sign 명령과 동일한 기능으로 처리하지 않는다.

## 4. Manifest의 상태와 한계

템플릿은 모든 실제 pin을 null, 모든 판정을 unverified로 둔다. **null끼리 같다고 호환되는 것은 아니다.** exact commit·board revision·profile·producer/consumer build·시험 증거를 채운 뒤 compatible/incompatible로 판정한다. incompatibility도 확인된 결과이므로 기록한다.

`review_candidate`는 모든 행의 판정·참조를 채워 검토할 수 있다는 뜻이다. 모든 제품 기능이 지원되거나 배포 가능한 릴리스라는 의미가 아니다. 실제 요구 경로가 compatible인지, incompatible 경로가 사용자에게 안전하게 거절되는지, evidence가 정확한 tuple에 결합돼 있고 위조/철회되지 않았는지 별도로 확인한다. 전체 기능 범위를 만족하지 못하면 release 완료로 승인하지 않는다.

manifest schema는 필드·필수 pin·상태별 null/evidence 유무만 검사한다. 증거 파일 존재/진위, commit의 실제 내용, 바이너리 서명, 실기 동작을 검사한 것으로 보지 않는다. runtime 허용 목록/릴리스 manifest는 별도로 서명·검증·철회하는 구현이 필요하다.

## 5. 연결된 구현 계약

아래 목록은 JSON 원본에서 생성한다.

<!-- GENERATED_INTERFACES -->

### IF-01 · NU 보드·장치 드라이버

Zephyr board/drivers → firmware services

| 논리 메서드 | 입력 shape | 결과 shape |
|---|---|---|
| initialize | BoardConfig | DeviceCapabilities |
| read_sensor | SensorHandle, deadline | SensorSample |
| set_output | OutputCommand, deadline | OutputResult |

- 경계 조건: 보드 리비전·핀·전원과 capability 일치; Nordic DK 식별자로 NU의 핀 구성을 대체하지 않음
- 연결 작업: HW-01, HW-07
- API: 내부/표준 transport 경로
- BLE: device.info

### IF-02 · 키 저장·암호 서비스

isolated firmware key service → wallet/passkey service

| 논리 메서드 | 입력 shape | 결과 shape |
|---|---|---|
| create | KeyPolicy | KeyHandle, publicMetadata |
| import_secure | AuthenticatedImportSession, encryptedChunk | ImportProgress |
| sign_authorized | KeyHandle, ApprovedPayload | SignatureResult |
| erase | OwnerResetAuthorization | EraseEvidence |

- 경계 조건: 호출자가 handle을 안다는 것만으로 서명 불가; purpose/actor 승인 검사; 키 원문·MPC share 반환 없음
- 연결 작업: HW-02, HW-04, HW-05, KEY-02
- API: 내부/표준 transport 경로
- BLE: wallet.create, wallet.import.begin, wallet.import.chunk, wallet.import.commit, wallet.import.abort, wallet.address

### IF-03 · 인증 세션·BLE 전송

firmware session service → RN native bridge / kiosk / ranging peer

| 논리 메서드 | 입력 shape | 결과 shape |
|---|---|---|
| open | PeerProof, requestedRole, protocolOffer | HandshakeResult |
| dispatch | AuthenticatedFrame | RequestReceipt |
| close | SessionHandle, reason | ClosedState |

- 경계 조건: role·transcript·sequence·scope·capability를 인증된 세션에 결합; unauthenticated feature 광고만으로 허용 금지; approval-baseline schema/ancestry/current action rights are mandatory; draft labels are not tested compatibility
- 연결 작업: HW-03, HW-06, BASE-03
- API: API-033, API-034
- BLE: session.open, session.confirm, session.close, proximity.observe

### IF-04 · 기기 검토·서명 승인

firmware approval service → wallet service / payment peer

| 논리 메서드 | 입력 shape | 결과 shape |
|---|---|---|
| prepare | TypedIntent, session | ReviewHandle |
| approve | ReviewHandle, physicalAction | ApprovalEvidence |
| result | RequestId, currentReadAuthority | SignatureOrState |

- 경계 조건: review의 표시 필드와 실제 payload digest를 결합; 불투명 hash의 blind sign fallback 금지; 취소가 이미 전송된 거래를 취소하지 않음; approval-baseline schema/ancestry/current action rights are mandatory; draft labels are not tested compatibility
- 연결 작업: HW-05, HW-07, PAY-05
- API: API-034, API-018
- BLE: payment.identify, payment.prepare, payment.result, wallet.sign.prepare, wallet.sign.result, request.cancel

### IF-05 · FOTA·부트·설정 migration

Zephyr update service / bootloader → RN update bridge

| 논리 메서드 | 입력 shape | 결과 shape |
|---|---|---|
| offer | SignedUpdateManifest, currentInventory | CompatibilityDecision |
| upload | ImageChunk, transferState | VerifiedProgress |
| apply | ReadyImage, policy | RebootIntent |
| confirm_boot | SelfTestResult, migratedSettings | BootHealth |

- 경계 조건: 논리 fota.*가 SMP wire 자체는 아님; 검증된 MCUmgr image 관리 adapter에 매핑. 전송 완료와 image 검증/부팅 확인을 구분
- 연결 작업: OTA-01, OTA-02, OTA-03, OTA-04
- API: API-048, API-049, API-101
- BLE: fota.begin, fota.chunk, fota.finalize, fota.apply, fota.status

### IF-06 · 표준 패스키 인증기

firmware CTAP authenticator → verified CTAP transport / RP client

| 논리 메서드 | 입력 shape | 결과 shape |
|---|---|---|
| make_credential | ValidatedCtapRequest | AttestedCredentialResult |
| get_assertion | ValidatedCtapRequest | AssertionResult |
| manage_credential | OwnerAuthorization, CredentialId | CredentialState |

- 경계 조건: credentials.list/delete는 앱 관리 명령; RP 인증은 CTAP 경로로 수행. 외부 bridge는 transport이고 private key 보관자가 아님
- 연결 작업: KEY-01, KEY-02, KEY-03, STAMP-04
- API: 내부/표준 transport 경로
- BLE: credentials.list, credentials.delete

### IF-07 · 녹음·찾기·스탬프와 중재

firmware feature services → native audio/storage / user app

| 논리 메서드 | 입력 shape | 결과 shape |
|---|---|---|
| acquire | FeatureRequest, deadline | LeaseOrBusy |
| emit_audio | RecordingHandle, sequence, timestamp, bytes | FrameReceipt |
| stop | FeatureHandle | FinalFeatureState |

- 경계 조건: control/approval/audio/flash 자원 예산과 우선순위 명시; 누락 오디오는 원음으로 채우지 않음; 미귀속 stamp는 사용 가능으로 표시 금지
- 연결 작업: REC-01, REC-02, FIND-01, STAMP-02, HW-07
- API: API-043, API-051
- BLE: recording.start, recording.stop, audio.frame, audio.flow-control, find.start, find.stop, passport.sync, device.settings.update

### IF-08 · RN native 기능 경계

Android/iOS native modules → RN TypeScript app

| 논리 메서드 | 입력 shape | 결과 shape |
|---|---|---|
| connect_device | DeviceSelector, expectedProfile | ConnectionHandle |
| execute | LogicalCommand, requestId | AsyncOperation |
| subscribe | ConnectionHandle, eventType | EventSubscription |
| dispose | OwnerContext | Disposed |

- 경계 조건: JS bridge에 니모닉·key handle의 무제한 서명권을 노출하지 않음; 실제 import 입력 보호 및 lifecycle 별도; 같은 request 재연결 시 자동 재서명 금지
- 연결 작업: APP-01, SHOP-01, REC-02, OTA-03
- API: API-103, API-001
- BLE: 직접 명령 없음

### IF-09 · 계정·정책·저장 서비스

business API / security adapters → user/kiosk/backoffice

| 논리 메서드 | 입력 shape | 결과 shape |
|---|---|---|
| authorize | VerifiedPrincipal, ResourceAction | PolicyDecision |
| commit_command | IdempotentCommand, expectedRevision | DurableResult |
| resolve_claim | AccountProof, HistoricalEligibility | ClaimState |

- 경계 조건: 107 API 계약·57 정책·15 논리 저장 자원에 연결; key material은 일반 업무 저장소로 이동하지 않음; approval-baseline schema/ancestry/current action rights are mandatory; draft labels are not tested compatibility
- 연결 작업: AUTH-01, AUTH-04, BASE-05, OPS-01
- API: API-001, API-006, API-103, API-104, API-105, API-106, API-108, API-109, API-110
- BLE: 직접 명령 없음

### IF-10 · HW/Cloud/스마트 계정 signer 선택

signing orchestrator → app review / chain submitter

| 논리 메서드 | 입력 shape | 결과 shape |
|---|---|---|
| describe_support | WalletBinding | SignerCapabilities |
| prepare | CanonicalIntent | ReviewRequest |
| sign | ApprovedIntent, signerProfile | TypedSignature |
| resume | OperationId, authority | OperationState |

- 경계 조건: EOA tx·typed data·UserOp·credential proof를 구분; 지원하지 않는 형식은 거절. HW 승인에 Cloud MPC quorum을 강제하지 않음; approval-baseline schema/ancestry/current action rights are mandatory; draft labels are not tested compatibility
- 연결 작업: BASE-06, APP-03, MPC-03, SMART-03
- API: API-015, API-017, API-018, API-056, API-108, API-109, API-110
- BLE: 직접 명령 없음

### IF-11 · MPC 참여자·복구

isolated MPC engine → cloud signing coordinator / mobile participant

| 논리 메서드 | 입력 shape | 결과 shape |
|---|---|---|
| begin_round | ProtocolSession, authenticatedParticipants | RoundState |
| advance | BoundRoundMessage | RoundState |
| finish | QuorumTranscript | PublicKeyOrSignature |
| recover | RecoveryPolicyProof | NewParticipantEpoch |

- 경계 조건: 세션/round/participant epoch 재사용 방지; 앱/server/recovery 신뢰 경계; 전체키 재조립을 임계 서명으로 대체하지 않음
- 연결 작업: MPC-01, MPC-02, MPC-03, MPC-04, MPC-05
- API: API-014, API-015, API-016
- BLE: 직접 명령 없음

### IF-12 · 체인·계약·Indexer

chain adapter / contracts / indexer → payment and market services

| 논리 메서드 | 입력 shape | 결과 shape |
|---|---|---|
| encode | ActionProfile, manifest | SignablePayload |
| submit | SignedPayload, expectedChain | TransactionReference |
| observe | SourceRef, cursor | CanonicalObservation |
| project | VerifiedEvent, revision | ProjectionUpdate |

- 경계 조건: chain 8283·ABI/code hash·asset units·EntryPoint/hash一致를 검사; receipt/개별 UserOp/event 귀속 확인 전 성공 처리 금지
- 연결 작업: TOKEN-01, INDEX-02, INDEX-04, INDEX-06, PAY-04, SMART-02, X402-02
- API: API-019, API-058, API-061, API-064, API-071
- BLE: 직접 명령 없음

### IF-13 · 보호 결과·전사·추천

trusted worker / object gateway → app / travel services

| 논리 메서드 | 입력 shape | 결과 shape |
|---|---|---|
| publish | SourceRevision, ObjectVersion, payload | ProtectedResultRef |
| read | CurrentPrincipal, purpose, resultId | PayloadOrStream |
| retract | ConsentOrSourceRevision | BlockedState |

- 경계 조건: source/profile/model/object version과 동의를 보존; gateway stream에서 실제 열람 ACL 재검사; 참고자료 없이 결제 사실을 생성하지 않음
- 연결 작업: REC-03, AI-01, AI-02, TRIP-04, BASE-05
- API: API-053, API-081, API-085, API-107
- BLE: 직접 명령 없음

### IF-14 · 반납·초기화·사용자 데이터

rental coordinator / device reset service → user app / ops safe projection

| 논리 메서드 | 입력 shape | 결과 shape |
|---|---|---|
| check_return | RentalBinding, walletOrigin | VersionedChecklist |
| authorize_reset | CompletedChecks, OwnerApproval | ResetClearance |
| confirm_reset | Clearance, DeviceEvidence | ReturnResult |

- 경계 조건: new travel/import 분기, pending tx·claim·passkey 처리와 reset evidence를 결합; 이전 키/기록을 새 대여자에게 제공하지 않음
- 연결 작업: STAMP-03, STAMP-04, HW-02, KEY-03
- API: API-046, API-047, API-104, API-105
- BLE: device.reset.prepare, device.reset.confirm


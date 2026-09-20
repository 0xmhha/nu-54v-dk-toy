# 반납 프로토콜 요청·응답과 권한 계약 후보

**상세 설계, 기존 계약 병합 전.** [반납 복구 설계](return-recovery-design.md)의 실행 허가·증거 접수·기기 정리·재대여 확인을 논리 메시지 타입으로 구체화한다. 제품 코드·펌웨어·SDK 설치·실기 시험은 포함하지 않는다.

[후보 목록](return-protocol-candidate.json) · [JSON Schema](return-protocol-candidate.schema.json) · [합성 예제](return-protocol-examples.json)

현재 신규 여행 지갑의 복구 정책은 **미결정**이다. 일반적인 “다음 진행”을 백업 기능 추가 또는 복구 없는 초기화 승인으로 해석하지 않는다. 필요한 회수/복구 점검이 unresolved면 신규 commit은 거절한다. 다른 타입과 실패 경로는 이 선택을 기다리는 동안 설계할 수 있다.

## 1. 메시지별 책임

RP 번호는 검토 문서 ID다. API-108처럼 정식 카탈로그 번호를 부여한 것이 아니며 실제 HTTP 경로나 BLE wire format도 아니다. 현재 API 107개·BLE 논리 명령 34개는 유지한다.

| ID | 요청 → 응답/메시지 | 권한과 적용 범위 |
|---|---|---|
| RP-01 | 기기 준비 → PrepareReport | 현재 소유 세션에서 물리적 승인, API-046 점검과 device.reset.prepare 연결 |
| RP-02 | CommitRequest → CommitResult | 현재 대여 소유자. 최신 점검/fence·기기 준비·물리 승인 검증 후 단일 허가 발급 |
| RP-03 | EvidenceSubmit → EvidenceResult | 원job에 한정된 중계 권한. 기존 API-047의 완료 접수 경계 후보 |
| RP-04 | ResetEvidence | 기기 신원 서명이 있는 내부 증거 본문. 평문 API 응답이나 BLE 광고가 아님 |
| RP-05 | CompletionAck | 서버가 반납 완료를 기록했다는 서명 ACK. 기기가 검증 후 상세 복구 기록 정리 |
| RP-06 | SignedCleanupChallenge → CleanupProof | 새 서버 challenge에 결합한 정리 결과 재증명. 이전 지갑 키 불필요 |
| RP-07 | CleanupSubmit → ReuseGateResult | 제한된 중계·holder 증명. 정리 결과 검증 후 재대여 조건 갱신 |
| RP-08 | JobStatus | API-091의 owner/operator_safe projection 후보. 서버가 현재 권한으로 선택 |

기존 API-046/047/091의 전체 HTTP envelope를 복사하지 않고 메시지 body 수준의 후보를 정의했다. 현재 API가 이미 이 타입을 받는다고 가정하지 않는다. RP-01/04의 기기 보고 역시 실제 transport framing·분할 전송·암호 인코딩은 미선정이다.

## 2. 공통 결합 값과 증명 형식

`Binding`은 jobId·deviceId·rentalId·previousBindingId·previousEpoch·checklistRevision·fenceRevision을 묶는다. 서버는 각 ID의 개별 존재뿐 아니라 **같은 대여 사건의 관계**를 확인한다. ID를 아는 것만으로 권한이 생기지 않는다.

`Preparation`에는 prepareId·bootNonce·sessionId·challengeDigest·eraseProfileId·deviceElapsedLimitMs·preparationDigest가 있다. deviceElapsedLimitMs는 기기의 원래 준비 시점에서 측정하는 상대시간이다. 재전달 시점부터 다시 시작하지 않는다. 값은 승인된 profile에서 정하며 클라이언트가 보낸 시간을 무조건 채택하지 않는다.

모든 서명 payload에 protocolVersion·purpose·audience를 둔다. `Proof`는 profileId·keyId·signedDigest·proofBytes를 가진다. 각 verifier는 다음을 함께 확인한다.

- 허용된 profile/신뢰 등록 key와 그 용도·철회 상태
- 목적·대상 verifier/기기와 전체 payload의 정규화된 digest
- 원job·대여·기기 세대·challenge·현재 권한 및 재사용 여부
- 기기 보고인 경우 검증된 장치 신원/부트/저장 profile

서명 알고리즘·정규화 인코딩·암호화 profile을 이 문서에서 임의 확정하지 않는다. profile 문자열과 서명 모양을 검사하는 schema만으로 진위를 입증할 수 없다. epoch는 JSON 정밀도 손실을 피하기 위해 10진 정수 문자열로 표현하며 profile에 따른 범위·증가 규칙을 의미 검사한다.

## 3. 초기화 실행 허가와 재전달

CommitRequest는 requestId/idempotencyKey·Binding·clearanceId·서명된 PrepareReport를 가진다. 서버는 외부 Binding과 PrepareReport 안의 Binding이 완전히 일치하는지 확인하고, 준비 내용·삭제 범위와 물리적 승인 digest를 검증한다. 일반 앱 인증을 기기 물리 승인으로 대체하지 않는다.

검증/쓰기 순서의 제안:

1. 현재 계정과 원대여의 소유 권한을 검사한다.
2. 같은 멱등 의도의 기존 결과가 있으면 현재 조회권을 확인해 기존 결과를 반환한다. 같은 key로 의미가 달라졌으면 충돌이다.
3. 신규 의도에는 최신 checklist/fence, 회수/복구 상태, 준비 challenge, 아직 발급된 grant가 없는지, 서버 기한을 검사한다.
4. job별 유일한 commitId/grantDigest와 grantIssued=true를 내구 저장한 뒤 ExecutionGrant를 반환한다.
5. 같은 job에 다른 key로 온 요청도 추가 grant를 발급하지 않는다. 기존 job 조회로 안내한다.

**불변 필드:** commitId·clearanceId·Binding 전체·prepareId/boot/session/challenge·ownerApprovalDigest·serverRecordedAt·grant 본문·grantDigest. 재인증용 새 challenge나 조회 시각은 이 허가 밖에 둔다.

CommitResult의 issuanceOutcome=existing은 같은 grant를 다시 읽었다는 뜻이다. 기기의 실행 가능 시간이 연장됐다는 뜻이 아니다. deliveryAdvice는 안내일 뿐이며, 기기가 현재 준비·상대시간·journal 상태를 독립 검증한다. 서버가 journal 존재 여부를 모르면 do_not_deliver로 안내하고 증거 조회를 먼저 한다. 이미 journal이 있으면 새 허가 적용 대신 같은 작업을 재개한다.

이전 허가 조회 자체는 기한 뒤에도 허용된 주체가 할 수 있지만, 오래된 boot/session에 허가를 재적용할 수 없다. serverRecordedAt은 서버 발급 기록 시각이며 실제 삭제 시각 증거로 사용하지 않는다.

## 4. 복구 중계 권한과 보호된 증거 제출

RelayAuthority는 ExecutionGrant와 다른 타입이다. purpose=reset_recovery_relay이며 allowedActions는 증거 중계·ACK 전달·정리 증명 요청/제출·제한된 진행 조회 중 서버가 허용한 것만 가진다. 초기화 시작·서명·키 가져오기·일반 owner 기능을 포함할 수 없다.

논리 발급 절차는 **현재 인증된 owner 또는 허용된 복구 운영자 + exact job + holder key 소유 증명**을 입력으로 한다. 클라이언트가 role/actions를 선택해 권한을 올릴 수 없다. 결과 RelayAuthority에는 job/device/epoch·holderKeyThumbprint·authorizationRevision·expiresAt이 결합된다. 이 발급 절차의 HTTP 경로는 후속 카탈로그 병합에서 정한다.

- 초기화 증거 relay 권한은 원grant의 previousEpoch에 결합한다.
- 완료 ACK/cleanup 권한은 검증된 nextEpoch에 결합한다.
- completed job도 정리 확인이 남아 있으면 cleanup 목적만으로 재발급할 수 있다. 현재 소유권·운영 범위·job 상태를 다시 검사한다.
- deviceId나 ciphertext를 가지고 있다는 이유로 발급하지 않는다. 이전 owner BLE bond를 보존하지 않는다.

EvidenceSubmit/CleanupSubmit는 jobId·권한·fresh requestChallenge·holderProof·보호된 본문을 포함한다. holderProof는 요청 목적·authorityId·job·challenge·본문 digest를 결합한다. 중계 증명과 내부 장치 증명을 모두 검사하며 하나로 대체하지 않는다.

`ProtectedEvidence`는 profileId·recipientKeyId·contextDigest·ciphertext다. 정규화된 protocolVersion/purpose/job/authorityId/epoch/challenge/verifier audience를 인증 문맥에 결합한다. 같은 ciphertext를 다른 job/대여/목적에 재사용하면 거절한다. 서명·암호화의 실제 검증과 재생 방지는 서버/기기 adapter의 책임이다.

별도의 미정 업로드 endpoint에 의존하지 않도록 후보에서는 보호된 bytes를 inline으로 운반한다. schema의 문자열 길이 상한은 검토용 안전 한도이며 실제 byte 한도·분할/재조립·메모리 예산·전송률은 BLE profile과 함께 검증해야 한다. 필요한 크기를 만족하지 못하면 framing 설계를 변경하며 무제한 버퍼를 가정하지 않는다.

## 5. 완료 증거의 판정과 늦은 접수

내부 ResetEvidence에는 Binding, commitId/grantDigest/preparationDigest, nextEpoch, deviceIdentityKeyId, firmwareMeasurementRef, eraseProfileId, journalDigest, eraseResult, bootState와 기기 proof가 있다.

acceptance는 다음을 요구한다.

- 서버에 내구 저장된 동일 commit 및 물리 승인 참조와 일치
- 현재 검사 대상이 원대여/기기/세대에 속하며 다른 대여를 해제하지 않음
- 검증된 profile의 다음 epoch와 삭제 범위, complete 결과, unprovisioned 부팅 상태
- 장치 신원·측정/부트·저장 보장의 유효성. 검증 불가능한 profile은 거절/격리

eraseResult=incomplete/unknown 또는 recovery_required를 “반납 완료”로 반환하지 않는다. EvidenceResult는 accepted/already_accepted/verification_pending/quarantined를 구분한다. accepted가 삭제된 키의 외부 사본 부재를 뜻하지는 않는다.

기한 뒤 기존 job의 증거가 도착하면 원grant의 계보를 검증한다. 만료된 clearance만으로 새 실행을 승인하지 않으며, 기존 키가 삭제됐다는 이유로 무조건 최신 점검을 다시 서명시키지도 않는다. 서버 TX-07이 완료됐으면 serverReturnCompleted=true를 반환할 수 있지만 reuseGate는 아직 awaiting_cleanup이다.

이미 정리까지 검증된 job의 재조회는 기존 eligible gate를 보여줄 수 있다. 그러나 **새 completed 전환만으로 gate를 eligible로 만들지는 않는다.** 새 기기 세대/대여가 생겼다면 과거 완료 결과와 현재 재대여 gate를 구분한다.

## 6. ACK 뒤 정리 증명 유실까지 복구

CompletionAck는 deviceId·jobId·commitId·accepted evidenceDigest·nextEpoch·completionHandle·cleanupProfileId·serverCompletionRevision에 결합된 서버 서명이다. 기기는 현재 job·세대·받아들인 증거와 일치하는 ACK만 처리한다.

정리 시 제안 순서:

1. ACK 검증 후 completionHandle/ackDigest/nextEpoch/cleanupProfileId를 포함한 최소 표식을 내구 기록한다.
2. 상세 복구 journal·이전 사용자 귀속 정보를 profile에 따라 제거한다. 최소 표식만 있다고 정리가 끝났다고 추정하지 않는다.
3. 정리 범위 검사 성공을 내구 표식에 기록한 뒤 CleanupProof를 생성한다. 중간 전원 차단 시 정리 검사를 재수행한다.
4. CleanupProof가 유실되면 새 SignedCleanupChallenge를 받아 같은 최소 표식과 실제 정리 상태를 확인해 다시 증명한다.

최소 표식에는 customer key·owner token·rentalId·이전 bindingId·원음·영수증을 넣지 않는다. completionHandle은 서버에만 원job과 매핑되는 불투명 값이며 공개 광고하지 않는다. 새 대여자는 과거 표식/증거를 조회할 수 없다.

서버 challenge는 job·completionHandle·nextEpoch·ackDigest·신선한 nonce/기한을 결합한다. deviceProof는 challenge와 cleanup 결과·markerDigest를 서명한다. proof 유실 때문에 옛 사용자 키나 상세 journal을 복원하지 않는다. 표식까지 없어졌으면 marker_missing으로 보고하고 격리한다.

**eligible 조건:** 서버 원반납 완료 + cleanupResult=complete와 표식 검증 + 현재 deviceEpoch 일치 + 아직 새 binding이 없고 unprovisioned_ready 상태. 서버가 gate revision을 원자 갱신하며, 실제 새 등록 요청에서도 이 조건을 다시 검사한다. ReuseGateResult는 조회 결과이지 누구나 쓸 수 있는 등록 bearer token이 아니다.

예전 epoch의 CleanupProof, 다른 ackDigest/challenge/handle, 만료된 challenge는 현재 gate를 열지 못한다. 같은 성공 증명의 네트워크 재시도는 기존 결과를 반환하고 gate를 다시 증가시키지 않는다. 새 challenge에 대한 재증명 역시 같은 정리 사건에 대한 확인이다.

## 7. 상태 조회와 오류 동작

JobStatus의 owner projection만 rentalId/checklistRevision/lateAssetRecovery를 포함한다. operator_safe에는 허용된 device/job 진행·안전한 사유만 포함한다. 어느 쪽도 grant·proof·ciphertext·사용자 키를 일반 상태 응답으로 받지 않는다. 서버가 projection을 선택하며 요청자 문자열을 권한으로 취급하지 않는다.

| 오류 후보 | 다음 행동 | 금지할 자동 행동 |
|---|---|---|
| RECOVERY_POLICY_UNRESOLVED | 정책 선택/필수 점검 대기 | 초기화 진행 |
| REVISION_CONFLICT / PREPARE_EXPIRED | 원job·점검 조회, 미발급일 때만 재준비 | 새 grant 자동 생성 |
| GRANT_ALREADY_ISSUED | 원commit 조회 | idempotencyKey 변경으로 중복 허가 |
| AUTHORITY_SCOPE_DENIED | 현재 권한으로 제한된 relay 재인증 | owner 권한 자동 부활 |
| JOB_SCOPE_MISMATCH / EPOCH_MISMATCH | 현재 job/기기 상태 대조, 필요 시 격리 | 현재 대여 강제 종료 |
| PROOF_PROFILE_UNSUPPORTED / PROOF_INVALID | 검증 실패 기록, 접근 범위 내 안내 | 문자열 proof를 성공으로 처리 |
| MARKER_MISSING | 기기 격리·검증 경로 검토 | 재대여 가능으로 추정 |
| VERIFICATION_PENDING | 같은 job 관측 | 새 초기화/재대여 |

오류의 safeJobRef는 요청자 조회 권한이 있을 때만 제공하며, 권한 없는 자에게 job 존재/상태를 알려주는 오류 차이를 노출하지 않는다. 실제 HTTP 상태코드와 인증 envelope는 카탈로그 통합 때 기존 공통 오류 규칙과 함께 확정한다.

## 8. 검증 범위와 미결정

합성 예제는 타입·목적/대상 필드·권한 목록·동일 Binding 관계·기기 세대·재대여 조건 등 일부 불변식을 검토한다. proofBytes/ciphertext/digest는 합성값이며 실제 서명/암호문을 검증하지 않는다. 이 결과는 보안 검증·권한 엔진·실기 journal·전원 차단·암호화 성공의 증거가 아니다.

미결정은 다음으로 유지한다: 신규 여행 지갑 복구 정책, NU 신원/부트·저장·epoch 보호 profile, 서명/암호화/정규화 profile, relay 발급·증거 접수·cleanup의 실제 endpoint/명령 및 버전 협상. 기존 반납 계약과 후보 차이는 RR-01~06에 연결하며 자동 병합하지 않는다.

다음 설계에서는 사용자 앱·기기·운영자 화면에서 이 상태와 복구 동작을 어떻게 보여줄지 정리하고, 어떤 미결정이 해당 화면/동작을 막는지 연결한다. 구현 착수로 전환하지 않는다.

후속 [반납 화면 상태·문구·버튼 설계](return-screen-design.md)를 작성했다. 서버 완료 후 정리 실패, 오래된 완료 캐시, 과거 job 조회도 구분한다. 다음 단계는 후보와 기존 명세의 차이 및 미결정 목록을 통합하는 설계 작업이다.

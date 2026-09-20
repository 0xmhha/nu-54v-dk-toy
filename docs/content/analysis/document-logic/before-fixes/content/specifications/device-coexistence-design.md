# 기기 동시동작·FOTA 공존 설계

작성: 2026-09-19. **DS-01 설계 후보이며 구현·실기 검증·기준 명세 병합은 수행하지 않았다.** Zephyr 확정만 유지하고 SDK/보드 target/보안·부트 profile/수치는 선택하지 않는다. 기존 15개 요구·104개 작업·320개 세부 작업·12주 앱 연동 목표와 미결정 정책 19개를 유지한다.

[구조화 원본](device-coexistence-design.json) · [검증기](validate_device_coexistence.py) · [상위 설계 점검](../planning/full-scope-design-review.md) · [기존 인터페이스](implementation-interfaces.md)

## 1. 핵심 동작과 적용 순서

기기는 여러 기능을 갖지만 승인 화면·키 저장·무선·플래시를 무조건 동시에 사용할 수 있다고 가정하지 않는다. **현재 녹음을 임의로 버리거나 이전 버튼 입력으로 결제를 승인하지 않는다.** 녹음 중 결제를 시작하려면 앱이 녹음 종료/부분 저장 상태를 보여주고 사용자가 새 검토를 시작한다. 반대로 이미지 수신은 안전한 chunk 경계에서 멈추고 결제 검토에 자원을 넘길 수 있다. 이 UX는 이번 설계의 제안이며 실기 성능이나 사용자 확정 정책으로 승격하지 않는다.

명령 심사 순서는 ① 인증/역할·현재 권한 ② board/version/profile 호환 ③ reason별 영속 제한과 lifecycle 상태 ④ 요청 멱등성/만료 ⑤ 현재 active set 전체와 자원 예산 ⑥ lease 발급이다. public 조회는 최소 정보만, 보호 상태 조회는 현 read authority로 별도 심사한다. status/cancel/복구는 중재표의 일반 mutable 작업과 별도 통로지만, 제한 우회 권한은 아니다.

두 작업의 표만 통과해도 3개 동시 실행을 허용하지 않는다. 모든 pair 검사에 더해 총 RAM/radio/flash 예산과 exclusive fence를 하나의 coordinator 임계구역에서 검사하고 lease를 발급한다. ISR/callback은 이벤트만 전달하며 직접 권한 상태를 바꾸지 않는다. 제품 coordinator/lock/API는 아직 구현하지 않았다.

## 2. 방향이 있는 공존 표

행은 실행 중인 작업, 열은 새 요청이다. `B`=BUSY, `I`=같은 요청 멱등 조회 또는 BUSY, `Y`=안전 checkpoint에서 기존 작업 정지 후 재심사, `P`=선정 profile의 검증 조합만 공존(미선정은 BUSY). Y는 새 요청 접수/승인 성공이 아니며 자동 재승인을 하지 않는다. 표의 apply는 실제 적용 작업이다. 별도 준비 control 예외 DC-X02와 하위 근접 관측 DC-X01은 아래와 같이 제한한다.

| 진행 중 / 새 요청 | sign | passkey | import | record | find | stamp | ranging | upload | apply | lifecycle |
|---|---|---|---|---|---|---|---|---|---|---|
| sign | I | B | B | B | B | B | B | B | B | B |
| passkey | B | I | B | B | B | B | B | B | B | B |
| import | B | B | I | B | B | B | B | B | B | B |
| record | B | B | B | I | B | B | P | B | B | B |
| find | Y | Y | Y | Y | I | P | B | B | Y | Y |
| stamp | B | B | B | B | P | I | B | B | B | B |
| ranging | B | B | B | P | B | B | I | B | B | B |
| upload | Y | Y | Y | Y | B | B | B | I | B | Y |
| apply | B | B | B | B | B | B | B | B | I | B |
| lifecycle | B | B | B | B | B | B | B | B | B | I |

작업명: sign=거래 검토·서명, passkey=패스키 생성·인증·관리, import=키 생성·가져오기·설정, record=음성 캡처·모바일 전송, find=찾기 출력, stamp=스탬프 동기화, ranging=인증된 근접 관측, upload=FOTA 이미지 전송·검증, apply=FOTA 적용·시험 부팅, lifecycle=등록·반납·취소 영속 전이.

BUSY는 저장되지 않은 대기 요청이며 앱이 명시적 재시도 버튼을 제공한다. Y는 request deadline 안에서만 준비하고 만료 시 새 요청을 폐기한다. 대기열·기한 수치는 profile 미선정 상태다. 기존 작업 lease/암호 연산을 강제로 만료시켜 공유 자원을 빼앗지 않는다. 중단 통지가 와도 실제 안전 종료 ACK 전에는 자원 재할당이 없다. 상태조회/control 서비스에 예약 예산을 두되 무제한 우선순위나 starvation을 허용하지 않는다.

**DC-X01 — 승인 내부 관측:** 근접 조건이 필요한 sign 또는 자체 서비스 passkey 흐름은 같은 operation lease 안에서 종속 ranging을 예약한다. 새 독립 ranging 요청(B)과 구분하며 exact request/session/peer/freshness와 radio 예산을 만족해야 한다. 실패하면 승인을 진행하지 않는다. 외부 RP의 표준 패스키 인증에 거리 조건을 전달하거나 강제할 수 있다고 주장하지 않는다.

**DC-X02 — 적용 준비 control:** 앱의 준비 의사 접수는 final apply와 다르다. 기존 review/import가 있으면 BUSY이고, 녹음 중에는 새 작업을 막는 drain fence만 걸 수 있다. 승인 UI 획득·녹음 자동 중단·부트 target 변경은 없다. 사용자가 녹음을 마치고 모든 mutable lease가 안전 종료한 뒤 독점 검토 UI에서 fresh 기기 적용 확인을 받아야 FT-05를 진행한다. 준비 요청도 현 update 권한을 요구하며 wire 경로는 아직 미등록이다.

## 3. 자원 소유권

| 자원 | 소유 경계 | 규칙 |
|---|---|---|
| review | 인증된 검토 화면/버튼/IMU | 한 review owner만; 목적·payload·session·generation에 입력 귀속. 눌린 채 진입/잔여 제스처는 승인 아님. |
| key_service | 격리된 키 서비스 | 일반 task에 원문키 반환 금지; sign/import/delete 전이는 직렬화; 암호 연산 중간 강제 kill 대신 안전 종료 후 결과 공개 권한 재검사. |
| control | BLE 제어/결과/취소 | bounded 우선 큐와 deadline; 보호된 결과 조회·취소·복구에 예약 예산. transport ACK는 durable ACK가 아님. |
| bulk | 음성/이미지/스탬프 전송 | 각 stream bounded credits/sequence, control과 예산 분리; 동시 허용은 정확한 profile 증거 필요. |
| audio | 마이크/DMA | 동의된 recordingId의 RAM ring만; 중단/넘침/연결 유실 시 gap 기록, 플래시 원음 저장 fallback 없음. |
| ram | 정적/동적 버퍼 | task별 상한과 총 high-water mark; 키 buffer와 오디오 buffer 공유 금지; 해제 전 민감 버퍼 처리 검증. |
| flash | 이미지 쓰기/erase | journal/키 영역과 겹침 금지; erase/write latency 측정 후 중재. 별도 partition만으로 지연 격리가 증명되지는 않음. |
| journal | 서명/등록/반납/업데이트 영속 이력 | 하나의 논리 writer/coordinator; atomic 또는 recoverable commit 증거. monotonic 이력 손실은 격리. |
| output | LED/부저/진동 | 검토 경고와 녹음 표시 우선; find가 승인 표시를 덮지 못함. |
| radio | BLE 연결/근접 측정 | 인증된 별도 peer 역할; 근접 관측은 서명권 아님. disconnect는 이미 생성/제출된 서명 회수 아님. |
| exclusive | 업데이트/수명주기 전역 변경 | reason별 fence와 owner; 자기 제한만 해제. 전체 권한 허용을 local lock 해제로 대체하지 않음. |

lease의 논리 필드는 operationId, requestDigest, principal/role, bindingEpoch, bootGeneration, sessionGeneration, ownerService, acquiredResources, phase, deadline이다. authoritative fenceRevision을 함께 확인한다. lease는 runtime 중재용이며 새 결제/초기화/서명 권한이 아니다. 재부팅 시 runtime lease는 폐기하되 진행 중 영속 작업은 원 job/request로 복원한다. 키 import commit이나 서명 결과가 불명확하면 새 실행 대신 조회/복구한다.

근접 측정이 필요한 승인에서는 측정 결과를 exact session/request·측정 peer·freshness에 묶고 최종 승인 직전 재검사한다. 녹음 때문에 측정을 중단했다면 이전 관측으로 조건을 충족시키지 않는다. 거리 기준·freshness 값·지원 tuple은 미선정이다.

## 4. FOTA 상태와 영속 경계

| ID | 전이 | 조건 | 기록/복구 경계 | 앱 표시 |
|---|---|---|---|
| FT-01 | idle → receiving | 현재 owner/update 권한 + 서명된 manifest 검증 + 정확한 board/slot/schema/profile + 저장 여유 검사 | updateId·manifestDigest·이미지 크기/해시·현재 release/schema·권한 문맥 기록; 수신 offset은 실제 durable bytes까지만 | 전송 시작 |
| FT-02 | receiving → receiving | 동일 updateId/manifest, chunk 범위·hash·세션 재인증 | 범위 검증/쓰기/checkpoint 후 offset ACK; 중복 chunk 일치 검사, 다른 image append 금지 | 전송 중 |
| FT-03 | receiving → verified | 전체 이미지 무결성·서명·버전/철회·호환 검사 | 검증된 정확한 digest와 inventory revision 기록; 아직 boot target 변경 없음 | 이미지 검증 완료 |
| FT-04 | verified → draining | 현재 owner/update 권한 + 앱의 적용 준비 의사 + 적용 전원 profile; 기존 review/import가 있으면 BUSY. 준비 접수는 최종 기기 승인 아님 | 새 mutable 작업 차단 fence 먼저; 진행 작업 안전 종료·결과 보존. 녹음은 사용자 중단/저장확인 필요; 자동 파기 금지 | 업데이트 준비 중 |
| FT-05 | draining → boot_pending | 모든 mutable active lease(서명·패스키·import·audio·stamp·ranging·find·upload) 안전 종료/checkpoint 및 자원 반환 확인; lifecycle 불확정 없음; 모든 required journal 지속성 확인; 독점 검토 UI에서 fresh 기기 적용 확인; boot target 지정 직전 현재 update 권한·inventory revision·manifest 철회/버전 eligibility·전원·모든 fence revision 재검사 | update fence·manifest·schema 세대·보존영역 digest를 journal commit 후 boot target 지정; 전원유실은 두 상태 대조 후 같은 update 복구 | 재시작 준비 완료 |
| FT-06 | boot_pending → trial | 검증된 부트 경로가 exact image 선택; 이전 boot/app 이력 대조 | fresh boot generation; 이전 session/lease/review 무효; migration은 shadow/copy 또는 검증된 reversible 경로 | 새 버전 확인 중 |
| FT-07 | trial → confirmed | 기기 local health + key/주소/credential 참조·모든 lifecycle marker 보존 + backward/forward schema 호환 증거 | boot confirm과 schema commit의 실제 순서/중단 복구를 profile로 고정. 둘 다 확인 전 성공 아님. 완료 journal 후 자기 update fence만 해제 | 업데이트 완료 |
| FT-08 | trial → recovery_hold | self-test 실패 또는 부트 횟수/기한 profile 위반 | 허용된 old image/schema 조합만 복귀; antirollback·epoch·cancel/consent consumed 이력 역행 금지; 증거 없으면 hold | 복구 필요 |
| FT-09 | recovery_hold → confirmed | 선정된 복구/복귀 profile로 현재 키·저널·버전 검증 성공 | 실행중 image와 outcome=applied/reverted 구분 기록. reverted는 새버전 설치 성공으로 표시하지 않음 | 새 버전 적용 또는 이전 버전 복구 |
| FT-10 | receiving/verified/draining → cancelled | boot target commit 이전이며 안전 checkpoint 도달 | 자기 update staging만 정리; 다른 fences·키·반납 증거 보존. boot target 여부 불확실하면 취소 대신 hold | 업데이트 취소 |

업데이트가 공존할 수 없는 기기 lifecycle 상태는 staged/activation unknown, reset grant 발급 또는 여부 불명, erase/evidence 전달/cleanup 미완료, 취소 fence 교환 미완료를 포함한다. 정상 active binding 자체는 차단 사유가 아니다. 미설정 기기라도 trusted provisioning/update authority가 있어야 한다. 전원·부트 장애 복구 업데이트는 해당 제한을 보존하는 별도 검증 profile이 있어야 하며 일반 owner apply로 우회하지 않는다.

draining은 새 작업 차단이지 기존 작업 삭제가 아니다. 취소 또는 drain deadline 경과 시 boot target commit 전임을 확인하고 자기 fence만 해제한다. 이미 다른 서명/초기화 권한이 노출되었을 가능성이 있으면 현재 서버 상태와 기기 증거를 맞추기 전 적용하지 않는다. 서버와의 최종 적용 제한 계약은 후속 wire/권한 설계 대상이며 오프라인에서 안전성을 추정하지 않는다.

## 5. 업데이트로 보존할 데이터

| 영역 | 불변 조건 | 복구 실패 시 |
|---|---|---|
| 지갑 키/주소·패스키 credential | key handle mapping, 주소/credential ID, 보호정책·counter 의미 보존; 새 키 생성으로 대체 금지 | 기능 잠금 및 복구 안내 |
| 등록/세션/동의 | LST-F03 recipient/binding/epoch/activation receipt와 continuation generation·withdrawal 이력 보존 | 재등록 추정 금지 |
| 반납/취소 | LST-F01 reset journal, F02 cleanup marker, F04 tombstone, grant/consumed 이력 보존 | 원 job 한정 복구/격리 |
| 서명/제출 | 원 request/source/digest, 생성·공개 여부 및 복구 handle 보존 | RESULT_UNKNOWN, 재서명 금지 |
| 녹음 | 기기에 영구 원음 저장 없음; 앱 durable 수신 범위·gap/partial 이력과 recordingId 유지 | 부분 녹음 표시 |
| 스탬프/설정 | 출처 receipt·revision 및 설정 schema와 owner 범위 보존 | 서버 재조회, 결제/스탬프 확정 추정 금지 |

antirollback security floor와 data schema generation은 firmware version 문자열과 구분한다. 이미지 복귀가 단조 증가 보안 이력·소유 epoch를 되돌리는 것은 허용하지 않는다. migration 작업 중 실제 flash layout, 최대 live data, 복구 여유, 원자성 증거를 측정해야 한다. 선택하지 않은 MCUboot/SMP 구조를 현재 보드에서 이미 사용 가능한 것으로 표시하지 않는다.

## 6. 앱·기기·서버 관측

| 상황 | 기기/네이티브 계층 | 앱과 서버 처리 |
|---|---|---|
| 녹음 링크 단절 | RAM ring 상한 내 flush 시도 후 stop, 원 sequence와 누락 표시 | durable ACK 이후만 저장됨; 재연결 시 부분 파일 마감 후 새 사용자 시작 |
| 화면 종료/앱 재시작 | OS background 가능 여부를 profile로 검사; 불가하면 명시적 중단 | JS 화면 생존을 녹음 보장으로 쓰지 않음; 자동 몰래 재녹음 없음 |
| FOTA 재연결 | fresh handshake와 exact updateId/manifest/boot generation 조회 | 전송률 100%와 설치 완료 구분; 상태 불명은 복구 중 |
| 소유/세션 철회 | 새 변이 차단; 비가역 작업은 안전 경계와 결과 권한 재검사 | logout으로 chain 거래/키 결과가 회수되었다고 표시 금지 |
| 반납·업데이트 경합 | reason별 fence 유지, 원 작업 복구만 허용 | 서버 승인 gate와 기기 local busy 별도 상태 |
| 구버전 앱 | 인증된 지원 프로토콜의 제한 조회만 제공 | 업데이트 안내; 약한 protocol로 자동 하향 금지 |

후속 상태 DTO 제안: operationId/type/phase, stateRevision, bindingEpoch, bootGeneration, blockedBy(공개 가능한 reason), retryClass, durableProgress, resultRef(현 read 권한으로만), currentImage/targetDigest, userActionRequired. 공통 상태 조회는 관측값이며 서명 허가가 아니다. 새 HTTP/BLE 명령은 등록하지 않았고 IF-05/fota.status, IF-07/recording.stop, IF-08 복구 경계에 매핑할 후보다.

서명·CTAP·import에는 서로 다른 목적별 확인 UI를 둔다. 버튼/IMU 입력은 표시 완료 이후 generation의 release→fresh action만 받는다. 녹음 버튼이 승인으로 넘어가거나 find LED가 승인 화면을 가리는 일을 거절한다. 기록 내용·키 원문·민감 payload를 운영 telemetry에 넣지 않는다.

## 7. 개발 단계 측정 및 수용 계획

CPU/메모리/전력 숫자나 audio codec/샘플링률을 추측해서 확정하지 않았다. JSON의 14개 측정 변수는 모두 null이며 exact board/SDK/boot/crypto/controller/app OS tuple과 증거를 채워야 한다. runtime test는 아래 28개 모두 not_run이다. 문서 검사는 참조/표 완전성만 확인한다.

| 사례 | 상황 | 기대 결과 |
|---|---|---|
| DC-T01 | 서명 중 녹음 시작 | BUSY; 마이크 켜지지 않고 서명 검토 유지 |
| DC-T02 | 녹음 중 서명 요청 | 자동 녹음 중단/자동 서명 없음; 사용자가 녹음을 마친 뒤 새 검토 |
| DC-T03 | 녹음 중 적용 요청 | draining 진입은 새작업 차단; 녹음 사용자 중단과 파일 상태 확인 전 boot target 금지 |
| DC-T04 | 이미지 chunk 중 서명 | durable checkpoint 후 upload pause; 검토는 fresh 입력 필요 |
| DC-T05 | FOTA 수신 재연결 | 동일 manifest의 실제 durable offset만 재개; 승인 재사용 없음 |
| DC-T06 | 다른 manifest chunk 혼입 | 거절; 원 이미지 offset 불변 |
| DC-T07 | 버튼을 누른 채 검토 진입 | release 이후 fresh event 전 승인 없음 |
| DC-T08 | 패스키와 결제 승인 경합 | 한 목적만 표시·승인; wallet.sign으로 CTAP 대체 없음 |
| DC-T09 | 모바일 종료/오디오 링크 단절 | buffer 상한에서 stop; 앱 gap/partial; 숨은 자동 재녹음 없음 |
| DC-T10 | 오디오 프레임 중복/유실 | sequence로 중복 제거/gap 기록; mobile durable ACK와 BLE ACK 구분 |
| DC-T11 | flash erase 중 control cancel | 측정된 control deadline 충족 또는 해당 동시 tuple 불허; 무기한 queue 금지 |
| DC-T12 | 부트 target 쓰기 전후 전원 차단 | journal/boot target 대조; 불명확하면 hold, 잘못된 cancelled 표시 없음 |
| DC-T13 | trial migration 중 전원 차단 | shadow/commit 복구; 키 생성·epoch 초기화 금지 |
| DC-T14 | 구버전이 신 schema를 못 읽음 | 자동 rollback 불허; 선정된 복구 경로 또는 hold |
| DC-T15 | 반납 erase 중 FOTA 적용 | BUSY; 원 reset job/증거 유지 |
| DC-T16 | grant 발급 후 기기 적용 지연 | 새 적용 준비 시 current lifecycle 상태 재검사; 전달 여부 불명확하면 hold |
| DC-T17 | FOTA 후 이전 허가 replay | cancel tombstone/consumed/epoch 보존, replay 거절 |
| DC-T18 | 앱 로그아웃/owner 철회 중 upload | 새 chunk/apply 권한 차단; staging는 승인권 아님 |
| DC-T19 | apply 뒤 모바일 끊김 | 기기 local boothealth 지속; 재연결은 현재 상태 조회, 두번째 apply 금지 |
| DC-T20 | 키 작업 중 권한 철회 | 안전 경계까지 처리 후 현 권한으로 결과 공개 판단; 이미 노출된 서명 회수 주장 없음 |
| DC-T21 | 설정 import 응답 유실 후 업데이트 | import commit 여부 조회/복구 전 apply 불허; 중복 key 생성 없음 |
| DC-T22 | 여러 fence 중 update 완료 | update fence만 해제; 분실/반납 제한 유지 |
| DC-T23 | 알 수 없는 board/profile | deny affected actions; null이 null과 같다는 이유로 허용 금지 |
| DC-T24 | FOTA 권한 없는 kiosk/SMP 우회 | 전송 입구와 적용 입구 모두 거절 |
| DC-T25 | 녹음+근접 측정 공존 | exact tuple 증거 없으면 BUSY; 거리 조건을 만들기 위한 가짜 값 없음 |
| DC-T26 | stamp 동기화 중 전원 단절 | 원 receipt/revision 재조회; stamp를 결제 확정으로 사용하지 않음 |
| DC-T27 | trial 정상인데 앱 구버전 | local confirm 가능 여부는 selected profile; 앱은 지원 조회만, unsupported mutation 거절 |
| DC-T28 | 강제 긴급 업데이트 요청 | 일반 적용 규칙 우회 없음; 별도 복구 profile 없이 key/recording 강제 폐기 금지 |

각 시험에는 정상 대조군과 실패 주입의 실제 evidence가 필요하다. 거절이 예상된 분기의 PASS를 기능 전체 완료로 계산하지 않는다. 실기 power-cut/동시 radio load/키 보존/패스키 RP 검증/앱 background 시험이 있어야 해당 tuple 공존·릴리스를 판정할 수 있다.

## 8. 연결과 다음 설계

작업 참조: BASE-03, FIND-01, FIND-02, HW-01, HW-02, HW-03, HW-04, HW-05, HW-06, HW-07, KEY-01, KEY-02, KEY-03, OPS-02, OTA-01, OTA-02, OTA-03, OTA-04, REC-01, REC-02, STAMP-02, STAMP-03, STAMP-04. 기존 WBS를 추가/삭제하거나 역할·공수를 배정하지 않았다.

D02/03/05/06/07/08/09/19에 걸친 profile·정책 선택은 미확정이다. RR-DEC-01 답변을 추정하지 않으며 FOTA를 이유로 import 지갑의 무관한 자산을 sweep하거나 초기화 조건을 낮추지 않는다. 이번 문서의 안전한 기본 거절 규칙도 기준 채택/실기 검증과 구분한다.

**다음은 DS-02: 소셜 로그인·두 지갑·MPC 복구 연결**이다. DS-01에서 남은 exact DTO/실제 bootloader commit 순서/서버 update fence 계약/수치 profile은 개발 전 채택 항목으로 추적한다. DS-02에서는 계정 전환·로그인 복구가 HW 기기 권한과 Cloud MPC 서명권에 각각 미치는 영향을 연결한다.

# 기기 동시동작·FOTA 공존 설계

LG14~15 설계 보완 반영 · 정책 미선택 · 기준 미병합 · 제품 구현 및 실행 검증 보류

## status

design_proposal_not_merged

## packageRef

DS-01

## implementation

deferred_by_user

## canonicalMerged

False

## runtimeVerified

False

## policySelectionUnchanged

True

## firmwareBase

Zephyr

## selectedSdk

None

## selectedBoardTarget

None

## selectedProfile

None

## requirementRefs

- 1
- 2
- 3
- 4
- 8
- 10
## taskRefs

- BASE-03
- FIND-01
- FIND-02
- HW-01
- HW-02
- HW-03
- HW-04
- HW-05
- HW-06
- HW-07
- KEY-01
- KEY-02
- KEY-03
- OPS-02
- OTA-01
- OTA-02
- OTA-03
- OTA-04
- REC-01
- REC-02
- STAMP-02
- STAMP-03
- STAMP-04
## decisionRefs

- D02
- D03
- D05
- D06
- D07
- D08
- D09
- D19
## operations

### sign

**label**: 거래 검토·서명

**interfaceRef**: IF-04

**resourceRefs**

```json
[
  "review",
  "key_service",
  "control",
  "journal"
]
```

### passkey

**label**: 패스키 생성·인증·관리

**interfaceRef**: IF-06

**resourceRefs**

```json
[
  "review",
  "key_service",
  "control",
  "journal"
]
```

### import

**label**: 키 생성·가져오기·설정

**interfaceRef**: IF-02

**resourceRefs**

```json
[
  "review",
  "key_service",
  "control",
  "journal"
]
```

### record

**label**: 음성 캡처·모바일 전송

**interfaceRef**: IF-07

**resourceRefs**

```json
[
  "audio",
  "bulk",
  "ram"
]
```

### find

**label**: 찾기 출력

**interfaceRef**: IF-07

**resourceRefs**

```json
[
  "output",
  "control"
]
```

### stamp

**label**: 스탬프 동기화

**interfaceRef**: IF-07

**resourceRefs**

```json
[
  "bulk",
  "journal"
]
```

### ranging

**label**: 인증된 근접 관측

**interfaceRef**: IF-03

**resourceRefs**

```json
[
  "radio",
  "control"
]
```

### upload

**label**: FOTA 이미지 전송·검증

**interfaceRef**: IF-05

**resourceRefs**

```json
[
  "bulk",
  "flash",
  "ram"
]
```

### apply

**label**: FOTA 적용·시험 부팅

**interfaceRef**: IF-05

**resourceRefs**

```json
[
  "exclusive",
  "flash",
  "journal"
]
```

### lifecycle

**label**: 등록·반납·취소 영속 전이

**interfaceRef**: IF-02

**resourceRefs**

```json
[
  "exclusive",
  "key_service",
  "journal"
]
```

## admissionMatrix

### admissionMatrix

**active**: sign

**incoming**: sign

**action**: deduplicate_or_busy

**rule**: 같은 principal/context/request의 조회·멱등 복구만 허용; 다른 요청은 BUSY. ID만 같고 payload가 다르면 거절.

### admissionMatrix

**active**: sign

**incoming**: passkey

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: sign

**incoming**: import

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: sign

**incoming**: record

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: sign

**incoming**: find

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: sign

**incoming**: stamp

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: sign

**incoming**: ranging

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: sign

**incoming**: upload

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: sign

**incoming**: apply

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: sign

**incoming**: lifecycle

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: passkey

**incoming**: sign

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: passkey

**incoming**: passkey

**action**: deduplicate_or_busy

**rule**: 같은 principal/context/request의 조회·멱등 복구만 허용; 다른 요청은 BUSY. ID만 같고 payload가 다르면 거절.

### admissionMatrix

**active**: passkey

**incoming**: import

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: passkey

**incoming**: record

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: passkey

**incoming**: find

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: passkey

**incoming**: stamp

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: passkey

**incoming**: ranging

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: passkey

**incoming**: upload

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: passkey

**incoming**: apply

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: passkey

**incoming**: lifecycle

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: import

**incoming**: sign

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: import

**incoming**: passkey

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: import

**incoming**: import

**action**: deduplicate_or_busy

**rule**: 같은 principal/context/request의 조회·멱등 복구만 허용; 다른 요청은 BUSY. ID만 같고 payload가 다르면 거절.

### admissionMatrix

**active**: import

**incoming**: record

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: import

**incoming**: find

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: import

**incoming**: stamp

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: import

**incoming**: ranging

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: import

**incoming**: upload

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: import

**incoming**: apply

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: import

**incoming**: lifecycle

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: record

**incoming**: sign

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: record

**incoming**: passkey

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: record

**incoming**: import

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: record

**incoming**: record

**action**: deduplicate_or_busy

**rule**: 같은 principal/context/request의 조회·멱등 복구만 허용; 다른 요청은 BUSY. ID만 같고 payload가 다르면 거절.

### admissionMatrix

**active**: record

**incoming**: find

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: record

**incoming**: stamp

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: record

**incoming**: ranging

**action**: profile_conditional

**rule**: 선정된 정확한 tuple의 무선/메모리/권한 시험이 이 조합을 허용할 때만 공존. 미선정·null·자원부족은 BUSY.

### admissionMatrix

**active**: record

**incoming**: upload

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: record

**incoming**: apply

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: record

**incoming**: lifecycle

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: find

**incoming**: sign

**action**: yield_at_checkpoint

**rule**: 출력 정지 ACK와 입력 event generation 갱신 뒤 새 요청 심사. 정지 확인 전에는 BUSY.

### admissionMatrix

**active**: find

**incoming**: passkey

**action**: yield_at_checkpoint

**rule**: 출력 정지 ACK와 입력 event generation 갱신 뒤 새 요청 심사. 정지 확인 전에는 BUSY.

### admissionMatrix

**active**: find

**incoming**: import

**action**: yield_at_checkpoint

**rule**: 출력 정지 ACK와 입력 event generation 갱신 뒤 새 요청 심사. 정지 확인 전에는 BUSY.

### admissionMatrix

**active**: find

**incoming**: record

**action**: yield_at_checkpoint

**rule**: 출력 정지 ACK와 입력 event generation 갱신 뒤 새 요청 심사. 정지 확인 전에는 BUSY.

### admissionMatrix

**active**: find

**incoming**: find

**action**: deduplicate_or_busy

**rule**: 같은 principal/context/request의 조회·멱등 복구만 허용; 다른 요청은 BUSY. ID만 같고 payload가 다르면 거절.

### admissionMatrix

**active**: find

**incoming**: stamp

**action**: profile_conditional

**rule**: 선정된 정확한 tuple의 무선/메모리/권한 시험이 이 조합을 허용할 때만 공존. 미선정·null·자원부족은 BUSY.

### admissionMatrix

**active**: find

**incoming**: ranging

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: find

**incoming**: upload

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: find

**incoming**: apply

**action**: yield_at_checkpoint

**rule**: 출력 정지 ACK와 입력 event generation 갱신 뒤 새 요청 심사. 정지 확인 전에는 BUSY.

### admissionMatrix

**active**: find

**incoming**: lifecycle

**action**: yield_at_checkpoint

**rule**: 출력 정지 ACK와 입력 event generation 갱신 뒤 새 요청 심사. 정지 확인 전에는 BUSY.

### admissionMatrix

**active**: stamp

**incoming**: sign

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: stamp

**incoming**: passkey

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: stamp

**incoming**: import

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: stamp

**incoming**: record

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: stamp

**incoming**: find

**action**: profile_conditional

**rule**: 선정된 정확한 tuple의 무선/메모리/권한 시험이 이 조합을 허용할 때만 공존. 미선정·null·자원부족은 BUSY.

### admissionMatrix

**active**: stamp

**incoming**: stamp

**action**: deduplicate_or_busy

**rule**: 같은 principal/context/request의 조회·멱등 복구만 허용; 다른 요청은 BUSY. ID만 같고 payload가 다르면 거절.

### admissionMatrix

**active**: stamp

**incoming**: ranging

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: stamp

**incoming**: upload

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: stamp

**incoming**: apply

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: stamp

**incoming**: lifecycle

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: ranging

**incoming**: sign

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: ranging

**incoming**: passkey

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: ranging

**incoming**: import

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: ranging

**incoming**: record

**action**: profile_conditional

**rule**: 선정된 정확한 tuple의 무선/메모리/권한 시험이 이 조합을 허용할 때만 공존. 미선정·null·자원부족은 BUSY.

### admissionMatrix

**active**: ranging

**incoming**: find

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: ranging

**incoming**: stamp

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: ranging

**incoming**: ranging

**action**: deduplicate_or_busy

**rule**: 같은 principal/context/request의 조회·멱등 복구만 허용; 다른 요청은 BUSY. ID만 같고 payload가 다르면 거절.

### admissionMatrix

**active**: ranging

**incoming**: upload

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: ranging

**incoming**: apply

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: ranging

**incoming**: lifecycle

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: upload

**incoming**: sign

**action**: yield_at_checkpoint

**rule**: 업로드 chunk/검증 단계의 안전 지점과 durable offset 기록 후 자원 반납. 기한 내 불가하면 BUSY. 새 요청은 현 권한을 재검사.

### admissionMatrix

**active**: upload

**incoming**: passkey

**action**: yield_at_checkpoint

**rule**: 업로드 chunk/검증 단계의 안전 지점과 durable offset 기록 후 자원 반납. 기한 내 불가하면 BUSY. 새 요청은 현 권한을 재검사.

### admissionMatrix

**active**: upload

**incoming**: import

**action**: yield_at_checkpoint

**rule**: 업로드 chunk/검증 단계의 안전 지점과 durable offset 기록 후 자원 반납. 기한 내 불가하면 BUSY. 새 요청은 현 권한을 재검사.

### admissionMatrix

**active**: upload

**incoming**: record

**action**: yield_at_checkpoint

**rule**: 업로드 chunk/검증 단계의 안전 지점과 durable offset 기록 후 자원 반납. 기한 내 불가하면 BUSY. 새 요청은 현 권한을 재검사.

### admissionMatrix

**active**: upload

**incoming**: find

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: upload

**incoming**: stamp

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: upload

**incoming**: ranging

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: upload

**incoming**: upload

**action**: deduplicate_or_busy

**rule**: 같은 principal/context/request의 조회·멱등 복구만 허용; 다른 요청은 BUSY. ID만 같고 payload가 다르면 거절.

### admissionMatrix

**active**: upload

**incoming**: apply

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: upload

**incoming**: lifecycle

**action**: yield_at_checkpoint

**rule**: 업로드 chunk/검증 단계의 안전 지점과 durable offset 기록 후 자원 반납. 기한 내 불가하면 BUSY. 새 요청은 현 권한을 재검사.

### admissionMatrix

**active**: apply

**incoming**: sign

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: apply

**incoming**: passkey

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: apply

**incoming**: import

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: apply

**incoming**: record

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: apply

**incoming**: find

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: apply

**incoming**: stamp

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: apply

**incoming**: ranging

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: apply

**incoming**: upload

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: apply

**incoming**: apply

**action**: deduplicate_or_busy

**rule**: 같은 principal/context/request의 조회·멱등 복구만 허용; 다른 요청은 BUSY. ID만 같고 payload가 다르면 거절.

### admissionMatrix

**active**: apply

**incoming**: lifecycle

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: lifecycle

**incoming**: sign

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: lifecycle

**incoming**: passkey

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: lifecycle

**incoming**: import

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: lifecycle

**incoming**: record

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: lifecycle

**incoming**: find

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: lifecycle

**incoming**: stamp

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: lifecycle

**incoming**: ranging

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: lifecycle

**incoming**: upload

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: lifecycle

**incoming**: apply

**action**: busy

**rule**: 새 요청을 실행하지 않는다. 권한/만료를 보존한 자동 승인 대기는 없다.

### admissionMatrix

**active**: lifecycle

**incoming**: lifecycle

**action**: deduplicate_or_busy

**rule**: 같은 principal/context/request의 조회·멱등 복구만 허용; 다른 요청은 BUSY. ID만 같고 payload가 다르면 거절.

## resources

### review

**owner**: 인증된 검토 화면/버튼/IMU

**rule**: 한 review owner만; 목적·payload·session·generation에 입력 귀속. 눌린 채 진입/잔여 제스처는 승인 아님.

### key_service

**owner**: 격리된 키 서비스

**rule**: 일반 task에 원문키 반환 금지; sign/import/delete 전이는 직렬화; 암호 연산 중간 강제 kill 대신 안전 종료 후 결과 공개 권한 재검사.

### control

**owner**: BLE 제어/결과/취소

**rule**: bounded 우선 큐와 deadline; 보호된 결과 조회·취소·복구에 예약 예산. transport ACK는 durable ACK가 아님.

### bulk

**owner**: 음성/이미지/스탬프 전송

**rule**: 각 stream bounded credits/sequence, control과 예산 분리; 동시 허용은 정확한 profile 증거 필요.

### audio

**owner**: 마이크/DMA

**rule**: 동의된 recordingId의 RAM ring만; 중단/넘침/연결 유실 시 gap 기록, 플래시 원음 저장 fallback 없음. 원 capture binding/streamGeneration에 buffer·DMA·queue를 고정한다. owner/대여 경계 중단과 정리 증거 없이 다른 대여자에게 재사용 금지. 폰의 durable 파일 삭제는 별도 scope.

### ram

**owner**: 정적/동적 버퍼

**rule**: task별 상한과 총 high-water mark; 키 buffer와 오디오 buffer 공유 금지; 해제 전 민감 버퍼 처리 검증.

### flash

**owner**: 이미지 쓰기/erase

**rule**: journal/키 영역과 겹침 금지; erase/write latency 측정 후 중재. 별도 partition만으로 지연 격리가 증명되지는 않음.

### journal

**owner**: 서명/등록/반납/업데이트 영속 이력

**rule**: 하나의 논리 writer/coordinator; atomic 또는 recoverable commit 증거. monotonic 이력 손실은 격리.

### output

**owner**: LED/부저/진동

**rule**: 검토 경고와 녹음 표시 우선; find가 승인 표시를 덮지 못함.

### radio

**owner**: BLE 연결/근접 측정

**rule**: 인증된 별도 peer 역할; 근접 관측은 서명권 아님. disconnect는 이미 생성/제출된 서명 회수 아님.

### exclusive

**owner**: 업데이트/수명주기 전역 변경

**rule**: reason별 fence와 owner; 자기 제한만 해제. 전체 권한 허용을 local lock 해제로 대체하지 않음.

## fotaTransitions

### FT-01

**fromState**: idle

**toState**: receiving

**guard**: 현재 owner/update 권한 + 서명된 manifest 검증 + 정확한 board/slot/schema/profile + 저장 여유 검사

**durableBoundary**: updateId·manifestDigest·이미지 크기/해시·현재 release/schema·권한 문맥 기록; 수신 offset은 실제 durable bytes까지만

**uiLabel**: 전송 시작

### FT-02

**fromState**: receiving

**toState**: receiving

**guard**: 동일 updateId/manifest, chunk 범위·hash·세션 재인증

**durableBoundary**: 범위 검증/쓰기/checkpoint 후 offset ACK; 중복 chunk 일치 검사, 다른 image append 금지

**uiLabel**: 전송 중

### FT-03

**fromState**: receiving

**toState**: verified

**guard**: 전체 이미지 무결성·서명·버전/철회·호환 검사

**durableBoundary**: 검증된 정확한 digest와 inventory revision 기록; 아직 boot target 변경 없음

**uiLabel**: 이미지 검증 완료

### FT-04

**fromState**: verified

**toState**: draining

**guard**: 현재 owner/update 권한 + 앱의 적용 준비 의사 + 적용 전원 profile; 기존 review/import가 있으면 BUSY. 준비 접수는 최종 기기 승인 아님

**durableBoundary**: 새 mutable 작업 차단 fence 먼저; 진행 작업 안전 종료·결과 보존. 녹음은 사용자 중단/저장확인 필요; 자동 파기 금지

**uiLabel**: 업데이트 준비 중

### FT-05

**fromState**: draining

**toState**: boot_pending

**guard**: 모든 mutable active lease(서명·패스키·import·audio·stamp·ranging·find·upload) 안전 종료/checkpoint 및 자원 반환 확인; lifecycle 불확정 없음; 모든 required journal 지속성 확인; 독점 검토 UI에서 fresh 기기 적용 확인; boot target 지정 직전 현재 update 권한·inventory revision·manifest 철회/버전 eligibility·전원·모든 fence revision 재검사

**durableBoundary**: update fence·manifest·schema 세대·보존영역 digest를 journal commit 후 boot target 지정; 전원유실은 두 상태 대조 후 같은 update 복구

**uiLabel**: 재시작 준비 완료

### FT-06

**fromState**: boot_pending

**toState**: trial

**guard**: 검증된 부트 경로가 exact image 선택; 이전 boot/app 이력 대조

**durableBoundary**: fresh boot generation; 이전 session/lease/review 무효; migration은 shadow/copy 또는 검증된 reversible 경로

**uiLabel**: 새 버전 확인 중

### FT-07

**fromState**: trial

**toState**: confirmed

**guard**: 기기 local health + key/주소/credential 참조·모든 lifecycle marker 보존 + backward/forward schema 호환 증거

**durableBoundary**: boot confirm과 schema commit의 실제 순서/중단 복구를 profile로 고정. 둘 다 확인 전 성공 아님. 완료 journal 후 자기 update fence만 해제

**uiLabel**: 업데이트 완료

### FT-08

**fromState**: trial

**toState**: recovery_hold

**guard**: self-test 실패 또는 부트 횟수/기한 profile 위반

**durableBoundary**: 허용된 old image/schema 조합만 복귀; antirollback·epoch·cancel/consent consumed 이력 역행 금지; 증거 없으면 hold

**uiLabel**: 복구 필요

### FT-09

**fromState**: recovery_hold

**toState**: confirmed

**guard**: 선정된 복구/복귀 profile로 현재 키·저널·버전 검증 성공

**durableBoundary**: 실행중 image와 outcome=applied/reverted 구분 기록. reverted는 새버전 설치 성공으로 표시하지 않음

**uiLabel**: 새 버전 적용 또는 이전 버전 복구

### FT-10

**fromState**: receiving|verified|draining

**toState**: cancelled

**guard**: boot target commit 이전이며 안전 checkpoint 도달

**durableBoundary**: 자기 update staging만 정리; 다른 fences·키·반납 증거 보존. boot target 여부 불확실하면 취소 대신 hold

**uiLabel**: 업데이트 취소

## measurementVariables

### measurementVariables

**name**: control_deadline

**selectedValue**: None

**evidenceRefs**

```json
[]
```

### measurementVariables

**name**: crypto_nonpreemptible_time

**selectedValue**: None

**evidenceRefs**

```json
[]
```

### measurementVariables

**name**: flash_erase_write_latency

**selectedValue**: None

**evidenceRefs**

```json
[]
```

### measurementVariables

**name**: journal_commit_latency

**selectedValue**: None

**evidenceRefs**

```json
[]
```

### measurementVariables

**name**: ram_peak_per_mode

**selectedValue**: None

**evidenceRefs**

```json
[]
```

### measurementVariables

**name**: audio_ring_bytes

**selectedValue**: None

**evidenceRefs**

```json
[]
```

### measurementVariables

**name**: audio_frame_rate

**selectedValue**: None

**evidenceRefs**

```json
[]
```

### measurementVariables

**name**: mobile_durable_ack_window

**selectedValue**: None

**evidenceRefs**

```json
[]
```

### measurementVariables

**name**: bulk_credit_limit

**selectedValue**: None

**evidenceRefs**

```json
[]
```

### measurementVariables

**name**: radio_schedule_budget

**selectedValue**: None

**evidenceRefs**

```json
[]
```

### measurementVariables

**name**: minimum_apply_power

**selectedValue**: None

**evidenceRefs**

```json
[]
```

### measurementVariables

**name**: image_slot_capacity

**selectedValue**: None

**evidenceRefs**

```json
[]
```

### measurementVariables

**name**: boot_trial_limit

**selectedValue**: None

**evidenceRefs**

```json
[]
```

### measurementVariables

**name**: queue_depth_and_deadline

**selectedValue**: None

**evidenceRefs**

```json
[]
```

## runtimeCases

### DC-T01

**scenario**: 서명 중 녹음 시작

**expected**: BUSY; 마이크 켜지지 않고 서명 검토 유지

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T02

**scenario**: 녹음 중 서명 요청

**expected**: 자동 녹음 중단/자동 서명 없음; 사용자가 녹음을 마친 뒤 새 검토

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T03

**scenario**: 녹음 중 적용 요청

**expected**: draining 진입은 새작업 차단; 녹음 사용자 중단과 파일 상태 확인 전 boot target 금지

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T04

**scenario**: 이미지 chunk 중 서명

**expected**: durable checkpoint 후 upload pause; 검토는 fresh 입력 필요

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T05

**scenario**: FOTA 수신 재연결

**expected**: 동일 manifest의 실제 durable offset만 재개; 승인 재사용 없음

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T06

**scenario**: 다른 manifest chunk 혼입

**expected**: 거절; 원 이미지 offset 불변

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T07

**scenario**: 버튼을 누른 채 검토 진입

**expected**: release 이후 fresh event 전 승인 없음

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T08

**scenario**: 패스키와 결제 승인 경합

**expected**: 한 목적만 표시·승인; wallet.sign으로 CTAP 대체 없음

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T09

**scenario**: 모바일 종료/오디오 링크 단절

**expected**: buffer 상한에서 stop; 앱 gap/partial; 숨은 자동 재녹음 없음

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T10

**scenario**: 오디오 프레임 중복/유실

**expected**: sequence로 중복 제거/gap 기록; mobile durable ACK와 BLE ACK 구분

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T11

**scenario**: flash erase 중 control cancel

**expected**: 측정된 control deadline 충족 또는 해당 동시 tuple 불허; 무기한 queue 금지

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T12

**scenario**: 부트 target 쓰기 전후 전원 차단

**expected**: journal/boot target 대조; 불명확하면 hold, 잘못된 cancelled 표시 없음

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T13

**scenario**: trial migration 중 전원 차단

**expected**: shadow/commit 복구; 키 생성·epoch 초기화 금지

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T14

**scenario**: 구버전이 신 schema를 못 읽음

**expected**: 자동 rollback 불허; 선정된 복구 경로 또는 hold

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T15

**scenario**: 반납 erase 중 FOTA 적용

**expected**: BUSY; 원 reset job/증거 유지

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T16

**scenario**: grant 발급 후 기기 적용 지연

**expected**: 새 적용 준비 시 current lifecycle 상태 재검사; 전달 여부 불명확하면 hold

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T17

**scenario**: FOTA 후 이전 허가 replay

**expected**: cancel tombstone/consumed/epoch 보존, replay 거절

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T18

**scenario**: 앱 로그아웃/owner 철회 중 upload

**expected**: 새 chunk/apply 권한 차단; staging는 승인권 아님

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T19

**scenario**: apply 뒤 모바일 끊김

**expected**: 기기 local boothealth 지속; 재연결은 현재 상태 조회, 두번째 apply 금지

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T20

**scenario**: 키 작업 중 권한 철회

**expected**: 안전 경계까지 처리 후 현 권한으로 결과 공개 판단; 이미 노출된 서명 회수 주장 없음

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T21

**scenario**: 설정 import 응답 유실 후 업데이트

**expected**: import commit 여부 조회/복구 전 apply 불허; 중복 key 생성 없음

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T22

**scenario**: 여러 fence 중 update 완료

**expected**: update fence만 해제; 분실/반납 제한 유지

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T23

**scenario**: 알 수 없는 board/profile

**expected**: deny affected actions; null이 null과 같다는 이유로 허용 금지

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T24

**scenario**: FOTA 권한 없는 kiosk/SMP 우회

**expected**: 전송 입구와 적용 입구 모두 거절

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T25

**scenario**: 녹음+근접 측정 공존

**expected**: exact tuple 증거 없으면 BUSY; 거리 조건을 만들기 위한 가짜 값 없음

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T26

**scenario**: stamp 동기화 중 전원 단절

**expected**: 원 receipt/revision 재조회; stamp를 결제 확정으로 사용하지 않음

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T27

**scenario**: trial 정상인데 앱 구버전

**expected**: local confirm 가능 여부는 selected profile; 앱은 지원 조회만, unsupported mutation 거절

**status**: not_run

**evidenceRefs**

```json
[]
```

### DC-T28

**scenario**: 강제 긴급 업데이트 요청

**expected**: 일반 적용 규칙 우회 없음; 별도 복구 profile 없이 key/recording 강제 폐기 금지

**status**: not_run

**evidenceRefs**

```json
[]
```

## nextDesign

DS-02 소셜 로그인·두 지갑·MPC 복구 연결

## admissionExceptions

### DC-X01

**kind**: approval_child_observation

**rule**: 정책상 근접 조건이 있는 sign 또는 자체 서비스 passkey 흐름에서만 같은 operation lease의 종속 ranging을 예약한다. 일반 incoming ranging 작업이 아니며 exact request/session/peer/freshness에 결합하고 radio 예산 불가 시 승인을 진행하지 않는다. 표준 외부 RP에 거리 조건이 전달/강제됨을 주장하지 않는다.

### DC-X02

**kind**: apply_prepare_control

**rule**: 현재 owner의 적용 준비 요청은 final apply와 다른 제한 control이다. 기존 review/import가 있으면 BUSY; record가 있으면 앱의 준비 의사만 접수하고 drain fence를 건다. 승인 UI를 획득하거나 녹음을 중단하지 않는다. 모든 mutable lease가 안전 종료한 뒤 기기에서 fresh 적용 확인을 받고 FT-05를 수행한다. 별도 wire 명령은 아직 미등록.

## declaredScopeRequirementRefs

- 1
- 2
- 3
- 4
- 8
- 10
## transitiveTaskImpactRequirementRefs

- 1
- 2
- 3
- 4
- 8
- 9
- 10
- 15
## requirementRefsMeaning

legacy alias of declaredScopeRequirementRefs; not exhaustive impact or completion evidence

## recordingBoundaryRef

content/specifications/recording-travel-ai-design.json


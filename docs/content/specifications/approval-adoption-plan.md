# 승인 기준 명세 반영 순서와 버전·프로파일 수용 기준

후속 [승인 기준 통합](approval-baseline.md)에서 이력 보존과 논리 계약 병합을 수행했다. 아래는 병합 전 계획의 이력이며, 이 문서 검증 진입점도 보존본을 확인한다. 실행 수용은 여전히 미검증이다.

2026-09-18 · **설계 계획. 이력 보관·기준 명세 병합·제품 구현·실행 호환 검증은 아직 수행하지 않았다.**

이전 [HTTP·권한·화면·저장 연결 후보](approval-integration-contracts.md)를 기준 명세에 반영하는 순서를 6개 단계로 정리했다. 11개 프로파일 영역과 24개 승인 호환 시나리오를 기존 IF/COMP/VAL 항목에 연결한다. **문서 형식 검증 통과, 실제 조합의 동작 확인, 현재 거래의 권한 허가는 각각 별도 판정**이다.

[원본 계획](approval-adoption-plan.json) · [기존 호환 행렬](compatibility-matrix.md) · [기존 manifest](compatibility-manifest.template.json) · [구조·참조 검증기](render_approval_adoption.py)

## 1. 이번 계획에서 고정한 구분

- `approval-v1-draft`, `approval-ble-v1-draft`는 작성 중인 계약 이름이다. 출시된 API/FW 버전이나 실기 호환 증거가 아니다.
- Zephyr 기반은 사용자 결정이다. 정확한 SDK·Zephyr commit·NU board target·controller·toolchain은 선정되지 않았다. 다른 보드의 시험 결과를 NU-54V-DK 결과로 대체하지 않는다.
- 프로파일 ID가 같은 것만으로 호환을 인정하지 않는다. schema/정규화/domain/verifier 신뢰/권한 정책/저장 형식의 digest와 실제 producer·consumer 빌드를 함께 고정한다.
- 서버에 저장된 원 source·요구 버전·profile을 기준으로 해석한다. 클라이언트가 구버전을 요청해서 증거·권한 요구를 낮출 수 없다.
- 모든 AD-C 행의 실제 pins는 null, 실행 상태는 unverified다. 표의 수용 조건은 앞으로 입증할 조건이지 현재 통과 판정이 아니다.

기준 반영은 문서와 계약의 변경이다. SDK 설치, 앱/펌웨어 작성, SQL 실행, 계약 배포, 기기 flash를 포함하지 않는다. 사용자의 구현 보류 지시는 유지한다.

## 2. 먼저 보존할 이력과 검증 책임

현재 후보 validator는 작성 당시 기준 파일의 hash를 확인한다. 기준 API나 공통 schema를 바꾸고 후보 hash만 최신 값으로 바꾸면 이전 검토가 무엇을 대상으로 했는지 사라진다. 따라서 **AD-S01에서 재현 가능한 checkpoint를 먼저 보존한 뒤 기준 파일을 수정**한다.

보관 범위는 후보 문서/schema/예제/validator, 각 baselineFiles·externalSchemaBindings의 전이적 의존 파일, validator가 상대경로로 읽는 WBS·반납 정책 등이다. 저장 디렉터리에서도 repository-relative 구조를 유지한다. manifest는 정렬된 상대경로와 SHA-256을 기록하고, 보관된 validator가 현재 작업 디렉터리의 변경 가능한 파일을 몰래 참조하지 않는지 확인한다. 새 history 디렉터리 자체를 재귀적으로 포함하지 않는다.

Python/library 환경과 실행 명령·결과도 함께 기록한다. 파일 hash가 맞는 것과 validator가 같은 환경에서 재현되는 것은 구분한다. 보존 환경에서 필요한 의존성이 없으면 이력 검증을 통과했다고 표시하지 않는다. 현재 checkpoint나 환경을 실제로 생성한 것은 아니다.

| 대상 | 기준 변경 후 처리 |
|---|---|
| 개인/source/common/integration 후보 | 원본 bytes와 hash를 보존한 checkpoint에서 기존 validator 실행 |
| commerce·return protocol·return screen 후보 | 승인 기준 변경의 간접 의존도 확인하고 같은 원칙으로 재현 |
| 새 기준 명세 | 새로운 기준 validator가 source별 긍정·부정·projection·권한/저장 참조 검증을 담당 |
| 새 기준으로 통과시킬 구형 기록 | 증거가 실제로 존재하는 변환만 명시 adapter 사례로 검증; 없는 epoch/proof를 합성하지 않음 |

이력 파일과 현재 파일의 역할은 명시적으로 나눈다. “과거 후보 재현 통과”를 “현재 기준 통합 통과”나 “실행 호환 확인”으로 재사용하지 않는다. 원본 파일을 삭제하거나 조용히 재작성해 validator 오류를 없애지 않는다.

## 3. 기준 채택과 실행 수용의 경계

AD-S02~05는 검토 가능한 부분 작업으로 나눌 수 있지만, API만 바뀌고 권한/화면/저장이 뒤따르지 않은 상태를 통합 기준으로 채택하지 않는다. AD-S06은 파일 전체의 정합성을 확인한 **설계 기준 채택**이다. 실행 가능한 릴리스 승인과 다르다.

AC-01~03을 독립 경로로 그대로 등록하는 경우 API는 107개에서 110개가 된다. 실제 반영 시 미사용 API ID를 확인하여 일대일 대응을 기록하며 이번에는 ID를 미리 부여하지 않았다. 렌더러·검증기의 기존 107개/핵심8개/나머지99개 가정도 분류와 함께 갱신해야 한다. 숫자만 바꾸고 권한 매핑을 생략하지 않는다. BLE 명령 34개와 화면 37개는 추가 명령/화면이 없는 한 유지한다.

기존 compatibility manifest는 **정확히 26개 COMP entry**를 요구한다. AD-C 24개는 그 26개에 연결한 승인 세부 수용표이며 기존 entries에 무작정 추가하지 않는다. 향후 실행 증거 manifest에 보조 수용표 참조를 연결하려면 별도의 명시적 schema 변경과 검증이 필요하다. COMP 하나가 compatible이라고 해서 연결된 모든 AD 행이 자동으로 통과하지 않는다.

## 4. 버전 선택·혼합 배포·기존 기록

| 상황 | 새 승인/서명/전송 | 기존 기록·복구 |
|---|---|---|
| 정확한 빌드/profile tuple을 검증하지 않음 | 해당 기능의 실행 지원을 선언하지 않음 | 현재 권한과 지원되는 원 reader로 제한 조회 |
| 서버와 앱의 HTTP 또는 projection 버전 불일치 | 명시적 지원 adapter 없으면 거절 | 원 operation/context를 유지하고 지원 버전 안내 |
| HTTP는 맞지만 BLE/review/device proof 미지원 | 기기 승인 차단 | 자동 blind signing·Cloud 대체·구버전 강등 없음 |
| 구형 기록에 필요한 provenance가 없음 | 새 증거로 승격·재서명/재제출 허용 금지 | 원 관측/기존 허용 조회와 unknown 상태 유지 |
| API/AC/저장 gate가 부분 적용된 서버 | 새 자격 발급·제출 활성화 금지 | 이력과 원 발급 멱등 기록 보존 |
| 원 grant/profile가 철회됨 | 새 보호 결과 전달/permit 차단 | 내부 관측·정산 대사는 계속; 공개 조회도 별도 현재 권한 필요 |
| 지원되는 구형 reader도 없음 | 약한 최신/일반 reader로 fallback 금지 | 상태를 지우지 않고 운영 복구 경로로 넘김 |

미래 구현의 혼합 배포에서는 old/new 조합마다 이 표를 적용한다. 서버가 새 계약을 이해한다고 모든 앱·기기 요청을 새 source로 변환하지 않는다. manifest나 클라이언트의 지원 목록은 권한 증명이 아니며, 인증된 연결/서버 신뢰 정보와 실제 기록을 확인한다. 미지원 조합의 거절 자체도 필요한 부정 시험 결과다.

**관측 보존은 결과 공개 권한 유지와 다르다.** 자격을 철회해도 내부 chain 관측과 환불 원장 대사는 계속할 수 있으나, 기존 사용자나 단말에 signed bytes를 계속 보여준다는 뜻은 아니다. 이미 노출된 EOA 서명의 외부 실행 가능성을 version mismatch나 철회로 없앨 수 있다고 가정하지 않는다.

## 5. 프로파일 수용에 필요한 증거

각 실제 tuple에 대해 다음 증거를 연결해야 한다. 합성 예제나 파일명만 채운 evidenceRef는 충분하지 않다.

1. producer/consumer의 정확한 빌드, schema/정책/profile digest, 실제 board/OS/native bridge/체인 환경 식별.
2. 동일 입력을 생산자와 소비자가 같은 의미로 처리하는 정상 사례. 표시 금액·자산·수취인과 실제 서명/거래 bytes도 대조.
3. source/domain/역할/chain/epoch/대상/nonce/권한을 바꾼 부정 사례와 실패 위치.
4. 응답 유실·세션 끊김·프로세스 재시작·동시 철회/교체·필요한 rollback의 복구 결과.

증거는 원문 비밀을 제거한 artifact와 무결성 정보·실행 환경·판정자/시각을 갖는다. token, private key, mnemonic, MPC share, 무제한 공개 가능한 signed bytes를 검증 기록에 넣지 않는다. 필요한 exact-byte 비교는 제한된 검증 환경에서 수행하고 외부 공유 증거에는 digest/검증 결과를 남긴다.

profile에는 **결정적인 인코딩·domain·기한/시계·검증자 신뢰·철회·오류·크기 제한**이 정의돼야 한다. 이번에 새로운 암호 알고리즘이나 MPC 구성을 선정하지 않았다. 해당 항목이 비어 있으면 동작 지원 선언을 막는다. HTTP Header 운반 형식의 합의만으로 sender proof의 보안을 확인했다고 보지 않는다.

## 6. 기한·세대·rollback의 해석

서명 profile/participant epoch/device epoch 변경은 새 행위의 허가를 다시 판정할 사유다. 이미 만들어진 유효 EOA 서명은 별도 대사 대상으로 남는다. 관측 중인 원 transaction을 새 체인·새 자산·새 payload로 바꾸어 재시도하지 않는다. 환불 reservation을 버전 오류나 TTL만으로 해제하지 않는다.

FW rollback은 이전 바이너리를 되돌리는 것만으로 완료되지 않는다. key store/settings/journal/epoch와 부트 정책의 읽기·보존 가능성을 입증해야 한다. 비가역 migration 뒤 구 FW가 상태를 읽지 못하면 강제 downgrade 대신 검증된 복구 경로를 요구한다. 상세 storage migration과 key 보존 실증은 구현 단계에 수행한다.

거리 측정은 정책별 조건이다. 필수이면 지원되는 peer·인증된 세션·fresh 증거가 필요하다. 비필수이면 서버가 명시적으로 허용하고 snapshot에 고정한 정책이어야 한다. 측정 실패나 null만으로 필수 조건을 해제하지 않는다. 현재 기기 보유 사실과 BLE 버전 표기는 실제 거리 측정 호환 증거가 아니다.

## 7. 남은 정책과 다음 작업

HW/Cloud 주소 관계, MPC 구성, 신규 여행 지갑의 늦은 자산 복구 백업은 이번 기술 수용표로 확정하지 않는다. RR-DEC-01은 답변 대기다. 필수 복구 조건이 충족되지 않은 새 초기화 허가를 추가하지 않는다. passkey/녹음/여행·시장 상품도 전체 범위에 그대로 남으며 승인 EOA 검증으로 해당 기능의 완료를 대신하지 않는다.

다음 작업은 AD-S01의 재현 가능한 이력 보존과 기준 명세 통합을 시작하는 **설계 파일 작업**이다. 단계별 변경 결과·권한·검증 책임을 연결하되, runtime pins/evidence가 없는 상태를 실제 서비스 준비 완료로 표시하지 않는다. 15개 요구·104개 작업·320개 세부 작업·19개 결정과 역할/공수 미배정 상태를 유지한다.

<!-- GENERATED_ADOPTION -->

## 8. 기준 반영 순서 — 모두 미적용

| 단계 | 선행 | 반영 내용 | 완료 확인 |
| --- | --- | --- | --- |
| AD-S01 이력·의존 파일 고정 | 없음 | 검토 시점 schema/카탈로그/validator/예제와 상대경로 의존 파일을 불변 checkpoint로 보존. 기존 validator를 그 checkpoint에서 실행해 결과와 환경 기록 | 스냅샷 hash와 경로 참조 및 기존 후보 재현; 이력 누락이면 기준 변경 착수 금지 |
| AD-S02 통합 타입·HTTP·권한 등록 | AD-S01 | 승인10 API/AC3/공통타입/역할별 API020 projection을 한 병합 단위로 연결. AC는 후속 실제 반영 때 미사용 ID를 확인해 등록 | 신규 경로 권한까지 포함한 schema/ACL 일치; 부모·자식 결과 참조 제한; 다른 source 경로 보존 |
| AD-S03 BLE·화면·기능 인터페이스 연결 | AD-S02 | owner 개인/환불과 guest terminal의 허용 원천·전체 메시지·오류를 연결. 화면17과 비승인 operation 회귀 | 정상/거절/끊김 흐름 연결, 실제 역할·profile 없을 때 차단; BLE 명령34/화면37 유지 여부 확인 |
| AD-S04 저장·권한·원장 제약 연결 | AD-S02 | AI-R01..05와 SS-R01..08을 기존 adapter/DDL 관계에 매핑. lineage CAS/parent ancestry/gate/nonce/protected blob/reference를 검토 | 논리·물리 제약·서비스 책임 구분; migration 설계만 작성, SQL 실행 없음; 환불 예약/reorg 경계 유지 |
| AD-S05 기준 검증 책임·전체 회귀 갱신 | AD-S03, AD-S04 | 기준 validator에 통합 계약 책임 이관하고 old/new mapping과 counts를 함께 반영. 후보 이력 validator는 checkpoint 의존성 유지 | 기준 API/정책/화면/BLE/DDL 참조와 15요구/104작업/320세부작업/19결정 일치; runtime 증거로 승격 금지 |
| AD-S06 설계 기준 채택 판정 | AD-S05 | 파일별 중간 변경이 아닌 S02..05 전체 일관성 검토 후 design_baseline 상태만 채택; 기능별 runtime matrix는 unverified 유지 | 기준 반영 ID/hash와 검토 이력 기록; deploy/기기 flash/SDK 선택/Seed 완료를 뜻하지 않음 |

## 9. 프로파일 영역 — 실행 profile 미선정

| 영역 | 초안 계약 이름 | 기존 호환 참조 | 정의·고정할 내용 |
| --- | --- | --- | --- |
| AD-P01 HTTP 계약·projection | approval-v1-draft | COMP-09 | API/응답 projection/권한 정책의 정확한 schema hash와 서버·앱 빌드 |
| AD-P02 BLE 세션·역할·전송 | approval-ble-v1-draft | COMP-03, COMP-08, COMP-18, COMP-19 | 인증 handshake/제어 codec/role policy/sequence/fragmentation 및 RN native bridge |
| AD-P03 검토·기기 결과 provenance | 미선정 | COMP-02, COMP-04 | reviewProof/device_result의 목적·정규화·검증자 신뢰·device epoch와 키 경계 |
| AD-P04 EOA 거래 인코딩·표시 | 미선정 | COMP-10, COMP-21 | chain/type/nonce/fee/calldata/표시 자산·수취인·금액과 서명 bytes 검증 |
| AD-P05 sender 결합 자격·요청 proof | 미선정 | COMP-09 | opaque grant issuer/audience/binding/replay/권한 revision, header codec와 제한 |
| AD-P06 Cloud MPC·참여자 세대 | 미선정 | COMP-11 | 참여자/프로토콜/backend/복구 정책·transcript·부모 자식 결과 연결 |
| AD-P07 저장·멱등·권한 gate | 미선정 | COMP-09, COMP-13 | lineage CAS/보호 객체/gate/operation ancestry/permit 기록의 schema와 정책 |
| AD-P08 체인·토큰·Indexer | 미선정 | COMP-12 | 8283 외 network identity, token 주소/code hash/decimals, ABI/decoder와 canonicality 정책 |
| AD-P09 근접 증거 | 미선정 | COMP-20 | 서버 선택 proximity policy와 peer/session/기한/관측 freshness; 거리 자체를 신원으로 취급하지 않음 |
| AD-P10 보드·Zephyr·FOTA | 미선정 | COMP-01, COMP-05, COMP-15, COMP-16, COMP-17 | NU board revision/target, Zephyr·SDK·controller·toolchain, key-store migration 및 rollback |
| AD-P11 다른 기능의 독립 profile | 미선정 | COMP-06, COMP-07, COMP-14, COMP-22, COMP-23, COMP-24, COMP-25, COMP-26 | passkey/녹음/반납/typed data/UserOp/DID/x402는 각 기능의 기존 contract·수용 증거를 유지 |

## 10. 승인 수용표 — 24개 모두 실행 미검증

아래 조건을 문서화했을 뿐 실제 조합의 호환을 판정하지 않았다. 각 행의 null pins와 빈 실행 증거 목록은 원본 JSON에 있다.

### AD-C01 · 공통 HTTP 정확한 조합

- 연결: AD-P01 / COMP-09 / VAL-03
- 수용에 필요한 것: HTTP schema/권한/consumer projection 모두 해당 tuple에 검증됨
- 거절할 것: 문자열 버전만 같거나 operationClass 미해석이면 새 승인 차단
- 조회·복구: 원 버전의 인증된 진행 조회만
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C02 · 개인 구후보→새 서버

- 연결: AD-P01, AD-P03 / COMP-09, COMP-10 / VAL-03, VAL-13
- 수용에 필요한 것: 검증 가능한 기존 기록에 한해 명시 adapter와 provenance 보존
- 거절할 것: contextDigest/epoch/proof가 없는 구형 서명은 새 승인으로 승격 금지
- 조회·복구: 구형 원 관측 보존, 자동 새 송금 금지
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C03 · guest 구후보→새 서버

- 연결: AD-P01, AD-P02 / COMP-09, COMP-19 / VAL-03, VAL-09
- 수용에 필요한 것: 원 attempt/session과 버전 고정, 명시 adapter에 동일 신뢰 근거
- 거절할 것: 새 API 타입을 개인 무증거 제출로 우회 금지
- 조회·복구: 유효 원 세션으로 제한 조회; 계정 강제 없음
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C04 · AC 일부만 적용된 서버

- 연결: AD-P01, AD-P05, AD-P07 / COMP-09 / VAL-03, VAL-04
- 수용에 필요한 것: challenge/issue/revoke/보호 API와 저장 gate가 모두 같은 기준 bundle
- 거절할 것: 발급만 되고 검증/철회가 없는 서버 조합은 차단
- 조회·복구: 원 grant/발급 멱등 기록 보존
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C05 · API020 부모·자식·일반 응답

- 연결: AD-P01, AD-P07 / COMP-09, COMP-13 / VAL-03, VAL-05
- 수용에 필요한 것: 서버 ancestry 분류, parent/child resultRef=null, action별 projection 검사
- 거절할 것: approval=null인 자식의 일반 다운로드 위장 거절
- 조회·복구: 부모 조회도 별도 ACL; 일반 녹음/AI 접근 유지
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C06 · 개인 HW owner 승인

- 연결: AD-P02, AD-P03, AD-P04, AD-P10 / COMP-01, COMP-02, COMP-04, COMP-18, COMP-21 / VAL-06, VAL-07, VAL-09, VAL-13
- 수용에 필요한 것: 선택 NU 키/epoch·owner 세션·review/result 증거·거래 표시 검증
- 거절할 것: 미지원 profile 또는 선택과 다른 기기 키 거절
- 조회·복구: signature_unknown이면 원 review 재조회, 자동 Cloud 전환 없음
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C07 · guest NU terminal 승인

- 연결: AD-P02, AD-P03, AD-P04 / COMP-04, COMP-19, COMP-21 / VAL-07, VAL-09, VAL-13
- 수용에 필요한 것: 주소 identify/attempt/session/payer 결합 후 별도 거래 물리 승인
- 거절할 것: 주소 소유 증거를 최종 결제 승인으로 재사용 거절
- 조회·복구: 같은 세션 원 결과만 복구
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C08 · 점주 HW 환불

- 연결: AD-P02, AD-P03, AD-P04, AD-P07 / COMP-04, COMP-18, COMP-21, COMP-09 / VAL-03, VAL-04, VAL-07
- 수용에 필요한 것: 업무 승인/지정 signer 분리, 원 reservation·현재 funding/epoch
- 거절할 것: 일반 매장 조회권으로 서명 결과·제출권 획득 금지
- 조회·복구: 원 환불/예약 보존, 개인 송금으로 대체 금지
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C09 · BLE 역할 교차

- 연결: AD-P02 / COMP-18, COMP-19, COMP-20 / VAL-09
- 수용에 필요한 것: 실제 인증 역할과 명령/source 일치
- 거절할 것: terminal→owner 관리/환불, ranging_peer→signing 거절
- 조회·복구: 연결 종료 후 미확정 서명 존재 가능성 보존
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C10 · review/device_result profile 교차

- 연결: AD-P03 / COMP-02, COMP-04 / VAL-07
- 수용에 필요한 것: domain/context/chain/payload/review/epoch와 verifier를 독립 확인
- 거절할 것: EOA 서명만으로 특정 NU의 물리 승인 추정 금지
- 조회·복구: 안전한 관측만, 이전 약한 verifier로 자동 강등 금지
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C11 · native·더미 ERC20 거래

- 연결: AD-P04, AD-P08 / COMP-10, COMP-12, COMP-21 / VAL-13
- 수용에 필요한 것: 정확한 자산/주소/code/decimals와 실제 bytes/가스 표시
- 거절할 것: 같은 심볼/chainId만으로 토큰 또는 네트워크 호환 인정 금지
- 조회·복구: 원 txHash 관측, 자산 변경한 재서명 금지
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C12 · 개인 Cloud MPC

- 연결: AD-P01, AD-P06 / COMP-09, COMP-11 / VAL-12, VAL-22
- 수용에 필요한 것: 현재 본인 signer/참여자 epoch와 부모·자식 exact result
- 거절할 것: OAuth 성공 또는 임의 자식 succeeded만으로 결과 공개 금지
- 조회·복구: 현재 권한으로 원 MPC 결과 조회
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C13 · 점주 Cloud MPC

- 연결: AD-P01, AD-P06, AD-P07 / COMP-09, COMP-11 / VAL-03, VAL-12
- 수용에 필요한 것: B의 지정 매장 signer 권한과 A의 업무 승인 별도 확인
- 거절할 것: A 또는 일반 매장 멤버에게 B의 서명권 자동 부여 금지
- 조회·복구: 원 환불 snapshot/parent로 B가 독립 재인증
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C14 · MPC/HW 세대 변경

- 연결: AD-P03, AD-P06, AD-P07 / COMP-02, COMP-09, COMP-11 / VAL-07, VAL-12, VAL-22
- 수용에 필요한 것: 현재 epoch/gate·inflight reconciliation 후 허용된 동작만
- 거절할 것: stale epoch의 새 서명/permit, 키 재조합을 MPC라고 표시 금지
- 조회·복구: 기존 외부 실행 가능 서명/예약/관측 유지
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C15 · sender proof·header·replay

- 연결: AD-P05 / COMP-09 / VAL-03, VAL-04
- 수용에 필요한 것: 정확한 token binding/issuer/audience/요청 내용/nonce 및 header codec
- 거절할 것: 다른 sender·body·중복 query·profile크기 초과 거절
- 조회·복구: 새 요청 proof와 동일 논리 멱등키; 토큰 로깅 금지
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C16 · 자격 교체·응답 유실

- 연결: AD-P05, AD-P07 / COMP-09 / VAL-04, VAL-05
- 수용에 필요한 것: issuance_key 복구와 predecessor CAS/단일 successor
- 거절할 것: blob 유실을 미발급으로 해석하거나 revoked lineage 부활 금지
- 조회·복구: 현재 인증·동일 sender로 원 발급 대사
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C17 · 철회·release·permit 경쟁

- 연결: AD-P05, AD-P07 / COMP-09 / VAL-04
- 수용에 필요한 것: 조회 release/전송 permit과 철회 직렬화 및 gate revision 검사
- 거절할 것: 토큰 유효만으로 hold 우회, 이미 노출된 bytes 취소 주장 금지
- 조회·복구: 불명확 네트워크 결과 유지/원 bytes만 대사
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C18 · 근접 필수·증거 없음/낡음

- 연결: AD-P02, AD-P09 / COMP-19, COMP-20 / VAL-09
- 수용에 필요한 것: proximityRequired=true에 맞는 인증 peer/fresh evidence
- 거절할 것: BLE연결/RSSI/이전 관측만으로 필수 근접 검증 대체 금지
- 조회·복구: 현재 세션 재검증 전 새 승인 중단
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C19 · 근접 비필수인 허용 정책

- 연결: AD-P09 / COMP-19, COMP-20 / VAL-09
- 수용에 필요한 것: 서버가 명시적으로 허용하고 snapshot에 고정한 비필수 정책
- 거절할 것: 클라이언트 null/센서 장애를 이유로 필수 정책을 비필수로 변경 금지
- 조회·복구: 현재 정책이 미선정이면 예외 사용 승인 안 됨
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C20 · 구 앱·신규 FW·RN bridge

- 연결: AD-P02, AD-P10 / COMP-03, COMP-08, COMP-17 / VAL-01, VAL-08, VAL-09
- 수용에 필요한 것: 실제 OS/native bridge/controller/NU firmware tuple와 reader 호환
- 거절할 것: BLE6.0 표기나 다른 보드 성공만으로 NU/S25 호환 추정 금지
- 조회·복구: 검증된 업데이트/복구 안내; blind signing 금지
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C21 · FOTA 전진·rollback과 키 저장

- 연결: AD-P03, AD-P10 / COMP-05, COMP-15, COMP-16 / VAL-08, VAL-20
- 수용에 필요한 것: 기존 키/설정/epoch/승인 journal migration 및 rollback readable 증거
- 거절할 것: 새 저장 포맷을 못 읽는 구 FW 강제 rollback 금지
- 조회·복구: 서명/반납 unknown 보존; 비파괴 복구 경로
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C22 · 다른 상품/기능 회귀

- 연결: AD-P11 / COMP-06, COMP-07, COMP-14, COMP-22, COMP-23, COMP-24, COMP-25, COMP-26 / VAL-10, VAL-11, VAL-14, VAL-16, VAL-17, VAL-21, VAL-23
- 수용에 필요한 것: 각 기능별 원 profile과 기존 독립 수용 사례 유지
- 거절할 것: EOA 승인 통과를 passkey/UserOp/DID/x402/반납 완료로 확대 금지
- 조회·복구: 기능별 원 상태/권한/복구 보존; 개별 증거 없이 그룹 compatible 금지
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C23 · Indexer 관측·정규성 변경

- 연결: AD-P08, AD-P07 / COMP-12 / VAL-13, VAL-19
- 수용에 필요한 것: network identity/code hash/ABI/decoder·confirm/reorg 적용 결합
- 거절할 것: RPC 접수·signed·API202를 매출/환불 확정으로 표시 금지
- 조회·복구: 원 관측 철회/confirmed→reserved 환불 보정 유지
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

### AD-C24 · 비승인 operation 보호 결과

- 연결: AD-P01, AD-P07 / COMP-09, COMP-13 / VAL-03, VAL-05, VAL-18
- 수용에 필요한 것: general ancestry와 녹음/AI/FOTA의 기존 보호 객체 ACL
- 거절할 것: 승인 child를 general로 fallback하거나 generic resultRef로 서명 bytes 공개 금지
- 조회·복구: 기능별 재조회·삭제/동의 유지
- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.

## 11. 판정 단계 — 서로 대체하지 않음

| 판정 | 조건 | 의미 |
| --- | --- | --- |
| AD-G01 문서 채택 | S01..S05 완료, 참조·형태·권한·상태·역할 검토 통과 | design_baseline_only |
| AD-G02 실행 환경 식별 | 보드·빌드·OS·profile·schema·체인 tuple 실제 값 고정 | no_execution_without_user_implementation_instruction |
| AD-G03 실행 호환 수용 | 대상 기능 모든 필수 tuple의 정상/부정/복구 실증, 미지원 조합 차단 | runtime_evidence_required |
| AD-G04 행위별 현재 허가 | 현재 actor/source/epoch/hold/TTL/nonce·projection ACL 재검사 | compatibility_not_authorization |
| AD-G05 관측·업무 반영 | 체인 정규성·확정/원 allocation/reservation·이벤트 정합성 | signing_not_payment_completion |

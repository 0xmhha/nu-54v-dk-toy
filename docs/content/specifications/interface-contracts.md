# API·BLE·데이터 연결 명세 v0.1

현재 승인 경로는 [승인 기준 통합](approval-baseline.md)을 따른다. 물리 SQL·실행 검증과 미선택 정책은 별도다.


기준일: 2026-09-18. **15개 기능 전체를 앱 연동까지 12주 안에 완료**하는 계획의 상세 설계다. 역할/공수는 배정하지 않는다. 아래 경로·필드·상태·권한 이름은 제안이며, 구현 또는 실제 기기 검증 결과가 아니다.

[기능 실행 시나리오](../planning/functional-execution-spec.md) → 이 문서 → [API 목록](api-catalog.md) / [BLE 명령 목록](ble-catalog.md). 기존 Indexer의 실제 조회 형식은 [별도 조사 자료](../integration/indexer-read-contract.graphql)를 유지하고, 여기의 `/v1` 제품 API와 혼동하지 않는다.

## 1. 산출물과 명세 수준

펌웨어는 Zephyr 기반으로 확정했다. [구현 인터페이스·버전 호환 기준](implementation-interfaces.md)은 14개 구성요소 경계와 26개 판정 항목을 정의한다. SDK/버전 및 실제 wire/ABI는 검증 전이며, 여기의 API·BLE 논리 계약을 구현 완료로 취급하지 않는다.

| 파일 | 이번에 정한 내용 | 아직 고정하지 않은 내용 |
|---|---|---|
| [api-catalog.json](api-catalog.json) | 110개 API의 요청/응답 필드, 권한 경계, 작업 ID, 개별 오류 | 모든 중첩 타입·필수/선택 필드·상태별 응답을 완성한 OpenAPI는 아님 |
| [ble-catalog.json](ble-catalog.json) | 34개 논리 명령과 허용 세션 역할 | GATT UUID, MTU, 코덱, 프레이밍·암호 suite·인증서 형식 |
| [core.schema.json](core.schema.json) | 금액·자산·지급 연결·오류·이벤트 등 기존 공통 타입과 승인 계약을 포함한 데이터 형태 | 업무 권한·서명 진위·uint256 상한·모든 상태 조건의 런타임 검증 |
| [event-catalog.json](event-catalog.json) | 10개 내부 업무 이벤트의 생산자/소비자/보정 연결 | 실제 브로커·전달 지연·배포 단위 |
| [examples.json](examples.json) | 허용 9건·거절 10건의 합성 예제 | 실제 거래/보드/제공자 실행 증거 |
| [data-model.md](data-model.md) | 저장 책임·고유 제약·상태 갱신·삭제 경계 | DB 엔진·DDL·전체 인덱스와 마이그레이션 구현 |
| [화면별 흐름](screen-flows.md) | 37개 화면의 입력·로딩·결과·실패·재실행과 공통 signer 연결 | UI 디자인·컴포넌트 구현 |
| [핵심 DTO](critical-dtos.md) | 주문·결제·환불·반납 8개 API의 필수 타입·응답·합성 예제 | 실제 인증/증명 검증 및 나머지 endpoint 상세 타입 |
| [확장 DTO](extended-contracts.md) | 나머지 102개 API의 타입·필수/조건부 필드·도메인 모델 | ProfileInput 내부의 선택 표준별 상세 payload |
| [저장 작업](storage-operations.md) | 16개 변경 단위·동시성·이벤트·삭제/복구 순서 | 선택 DB의 DDL·실제 동시성/장애 시험 |
| [데이터베이스 참조안](database/README.md) | 61개 테이블·7개 DDL과 순차 제약 검증 | 운영 엔진 선정·권한·실제 다중 세션 경쟁/앱 시험 |
| [권한·트랜잭션](access-transactions.md) | 110개 API 접근 매핑·60개 정책·34개 허용/거부 기대 시나리오 | 인증 middleware·worker 구현, 보안 저장 adapter·신뢰 registry 선택과 실제 시험 |
| [보안 연결 상세 설계](security-integration-design.md) | 5개 GAP의 입출력·상태·복구·수용 시나리오 30개, 보충 API 5개 필드 계약 | 총 110개 카탈로그/DTO 통합 완료; adapter/profile 선정·런타임 구현/시험 |

상품별 세부 actionInput과 표준 패스키/x402/MPC 전송은 선택한 표준·라이브러리의 명세로 연결한다. 임의의 JSON 요청을 표준 호환 구현으로 취급하지 않는다.

## 2. API 공통 규칙

### 요청·응답

- 모든 인증·결제 API는 보호된 전송을 사용한다. 로그인 proof·서명 payload·복구 proof·동의가 필요한 개인 자료는 일반 access log에 넣지 않는다.
- `requestId`는 HTTP 요청 추적, `operationId`는 지속되는 비동기 업무, `idempotencyKey`는 같은 변경의 재시도를 식별한다. 세 값의 목적을 구분한다.
- 금액은 `assetId`와 `atomicAmount` 정수 문자열로 전달한다. 표시용 기호/소수점은 서버에 등록된 자산 설정에서 읽는다. 클라이언트가 전한 decimals를 신뢰하지 않는다.
- API 목록의 GET 입력은 query, 경로의 `{id}`는 path parameter다. 변경 요청의 필드는 body다. `Idempotency-Key`와 요청 추적 ID는 공통 header로 처리하는 제안이다.
- 응답 envelope 제안: 성공은 `{requestId, data}`, 오류는 `{requestId, error}`. 단순 조회/즉시 처리는 200, 생성은 201, 비동기 작업은 202와 operation 참조를 반환한다.
- 오류 제안: 형태 오류 400, 인증 없음/만료 401, 권한 없음 403, 비공개 자원 존재를 숨길 필요가 있는 경우 404, 중복 키의 다른 내용/상태·revision 충돌 409, 의미 검증 실패 422, 호출 제한 429, 의존 서비스 장애 503.
- 목록은 `limit`, 불투명 `cursor`, `nextCursor`를 사용한다. stable sort와 동시 변경 시 조회 기준을 endpoint 상세화 때 지정한다.
- `expectedRevision`은 오래된 화면의 덮어쓰기를 막는다. 수취 주소·환불·마감·설정·동의 변경은 최신 revision을 검사한다.

### 멱등성과 비동기 복구

변경 요청은 **주체/권한 범위 + endpoint 의미 + idempotencyKey**를 결합해 기록한다. 같은 키·같은 정규화 입력은 이전 결과를 반환하고, 같은 키·다른 입력은 409로 거절한다. 멱등성 기록에는 민감 원문을 보관하지 않는다. 중복 방지 보관 기간과 proof 유효기간은 각각 정한다.

공통 idempotency 규칙이 만료된 제공자 코드·세션·지불 증명을 새 업무에 재사용하도록 허용하는 것은 아니다. 제공자 token 교환·refresh 회전은 별도 재시도/폐기 정책을 갖는다. 이미 끝난 요청 결과를 다시 읽는 경우에도 현재 조회 권한을 검사한다.

제출 응답 유실은 `unknown`으로 보존한다. 거래 hash로 체인을 조회하고 동일 signed payload의 재전송과 새 nonce/수취/금액 거래 생성을 구분한다. 앱 재시작 후 `operationId`/attemptId로 이어서 확인한다. 업무 성공과 HTTP 요청 성공을 동일시하지 않는다.

## 3. 권한 판정

카탈로그의 권한 이름은 서버에서 구현할 **검사 조건**이다. 클라이언트가 role 문자열을 보내면 권한을 얻는 구조가 아니다.

| 권한 계열 | 필요한 검사 |
|---|---|
| account / recent_auth | 실제 제공자 증명에 연결된 서버 세션, 소유 계정, 필요한 추가/최근 인증 |
| store_* | 인증 사용자·해당 store 소속·행위별 권한·관리 모드. 자산 조회와 자금 서명은 별도 |
| wallet_read / wallet_use | 계정 또는 매장에 연결된 지갑의 조회/요청 권한. private key 획득 권한 아님 |
| wallet_sign_authorized | 확인한 intent와 wallet, 허용 signer, 사용자 승인 proof. MPC 참여자 정책도 별도 통과 |
| *_capability | 서버가 검증 가능한 제한 증명, audience·order/attempt/intent·terminal/device·만료·허용 action 결합 |
| device_owner | 등록 앱/계정과 현 대여/소유 binding. 기기 자체에서도 세션 권한 검사 |
| rental_owner_and_clearance | 현 사용자 확인과 완료된 반납 검사/기기 초기화 증거; 운영자 버튼만으로 대체 불가 |
| issuer / verifier / offering_actor | 상품의 발급·검증·관리 정책 및 자격/지갑 통제 증명 |
| ops_* | 운영자별 허용 범위·조치 이유·감사 기록. 고객 자금 서명 권한 제외 |

키오스크 고객 모드는 제한된 단말 세션으로 주문을 만들 수 있다. 서버는 주문 capability에 전체 사용자 프로필·녹음·위치·관리 기능을 노출하지 않는다. 자격/복구 proof의 구체 형식과 신뢰 근거는 각 결정 작업에서 확정한다.

## 4. 폰 공동 서명 없이 NU→키오스크 결제

1. 점주가 인증해 단말을 매장에 연결한다. 고객 모드에는 메뉴/주문/해당 결제만 허용한다.
2. 키오스크가 주문을 생성하고 서버가 메뉴·수취 주소·금액 snapshot을 저장한다.
3. payment-attempt 생성 결과에 quote와 새 pairing challenge를 포함한다. 키오스크/기기는 가게·대상을 확인하고 임시 BLE 결제 세션을 수립한다.
4. 임시 세션의 `payment.identify`로 가게/attempt/challenge를 확인한 사용자에게 지급 지갑 선택·주소 공개 동의를 받는다. 선택 주소와 challenge에 결합한 소유 증명을 얻어 intent를 만든다. 터미널에 일반 wallet.address 조회 권한을 주지 않는다. 고객 폰의 로그인이나 공동 서명을 필수 조건으로 추가하지 않는다.
5. NU는 지원 호출을 파싱해 수취인·토큰·금액·체인·수수료 한도를 표시하고 필요한 유효 근접 관측을 확인한다. 서버가 보낸 표시 문자열만 그대로 믿지 않는다.
6. 물리적 승인으로 정확한 payload를 서명한다. 키오스크는 해당 세션·요청의 결과만 가져올 수 있다.
7. 중계 서비스는 signer·chain·nonce·수취·값/호출을 고정 intent와 비교하고 tx hash를 결합한 뒤 제출한다. 키오스크가 임의 거래를 이 intent의 서명으로 바꾸어 제출할 수 없다.
8. Indexer 관측을 receipt 성공·정규 블록·확정 정책·토큰·수취·금액·지급 주체와 대조한다. 다른 고객의 같은 금액 입금을 먼저 제시해도 거절한다.
9. 지급 acceptance 변경 이벤트가 주문·매출·영수증·혜택·여행 입력에 반영된다. 임시 결제 권한은 종료한다.

EOA ERC20 경로의 `PaymentBinding` schema가 승인 거래/attempt 귀속의 기본 예다. 스마트 계정은 추가로 account 주소, UserOperation 식별자, 개별 실행 결과와 전송 log를 결합한다. 외부 transaction.from을 스마트 계정의 고객 주소로 오인하지 않는다. 이 추가 DTO는 SMART-01/02의 버전 선택 후 고정한다.

`payment.prepare`에는 unsigned transaction 필드, 자산·수취·금액·기한·가게 정보를 가진 intent를 직접 전달한다. 기기가 `intentRef`를 서버에 HTTP 조회할 수 있다고 가정하지 않는다. merchantProofRef/proximityRef는 **이미 해당 기기의 검증된 세션에서 확보한 자료**의 로컬 참조이며, 해당 자료가 없으면 요청을 거절한다. 예제 digest·주소·가스 값은 합성값이고 실제 서명/해시 일치 시험이 아니다.

## 5. BLE 보호·역할·재연결

카탈로그는 **암호화 채널 위에서 처리할 논리 메시지**다. 자체 암호 알고리즘이나 미검증 키 교환 프로토콜을 정의하지 않는다. HW-03에서 검증된 프로토콜/구현을 선택하고 peer 인증, forward secrecy 지원 여부, 재전송 방지, nonce 관리, 키 수명·재키잉을 확인한 후 transport profile로 고정한다.

- `device.info`의 인증 전 응답은 최소 모델·호환 정보만 공개한다. 지갑 주소·소유자·대여 이력·개인 자격은 반환하지 않는다.
- `session.open/confirm`은 선택 프로토콜의 handshake를 호출하는 자리다. JSON challenge/proof가 있다는 사실만으로 상호 인증 완료라고 판단하지 않는다.
- 인증 후 메시지는 `sessionId`, `messageId`, `sequence`, `kind`, `command`, `payload`를 결합한다. 세션 role은 실제 인증 결과로 부여하고 메시지의 role과 대조한다.
- `owner`는 허용된 설정·import·FOTA·찾기·자격 관리 요청을 할 수 있다. 민감 행위는 추가 물리적 승인/잠금 정책을 적용한다.
- `payment_terminal`은 해당 주문의 identify/prepare/result/cancel/close만 허용한다. wallet.import, settings, recording, FOTA, reset은 거절한다.
- `ranging_peer`의 측정은 확인된 peer와 측정 세션에 연결한다. 단순 거리 숫자 입력을 신원 증명으로 사용하지 않는다.
- 요청 digest에 command와 전체 승인 대상·세션을 결합한다. 다른 요청/가게의 result를 조회할 수 없다. 동일 message ID의 다른 내용은 거절한다.
- 재연결은 새 세션/키로 처리하고 필요한 진행 작업을 명시적으로 복원한다. 만료 판단에 장치 시계가 필요하면 신뢰 기준을 정한다. 미설정 기기 RTC를 서버 UTC와 같다고 가정하지 않는다.
- import는 메모리 버퍼·길이 한도·정해진 순서·완료 원자성·중단 정리를 갖춘다. 일반 영속 요청 큐/로그/분석 이벤트에 민감 payload를 저장하지 않는다.
- private key를 반환하는 명령은 정의하지 않는다. 기존 키 import는 사용자가 선택한 소유 앱 경로에서 기기로 들어가는 기능이다.

`BleControlMessage` schema는 구조와 일부 역할/명령 조합만 검사한다. peer proof·AEAD tag·sequence 재사용·session expiration 검증을 대체하지 않는다. 선택 transport profile이 미정인 상태에서 secret import를 실제 전송하는 구현은 시작하지 않는다.

### 표준 패스키와 음성

패스키 등록/인증 자체는 KEY-01에서 선택한 표준 인증기 전송과 요청 처리를 따른다. 카탈로그의 credentials.list/delete는 관리 기능일 뿐이다. `wallet.sign.prepare`를 사용한 임의 challenge 서명이 패스키 인증 성공을 뜻하지 않는다.

음성 `audio.frame`은 control envelope와 별도 데이터 경로다. 프레임에는 recording ID·순서·포맷 프로필·payload가 필요하고 ACK/수신창·누락 표시를 정의한다. 실제 MTU/프레임 크기/코덱은 측정 후 정한다. 큰 payload를 일반 control JSON에 넣는 wire format을 확정하지 않는다. 장치 버튼으로 시작한 녹음도 연결된 소유 앱과 현재 권한/수신 준비 상태를 확인한다.

FOTA의 chunk ACK는 이미지 신뢰 확인과 다르다. 전체 패키지 검증·부트 검증·부팅 확인 후에만 완료다. 키오스크 연결·녹음·서명과의 자원 중재는 HW-07 규칙을 따른다.

## 6. 이벤트 전달과 재구성

업무 이벤트는 `eventId`, aggregate 식별자와 revision, correlation/causation ID를 가진다. 원장 변경과 outbox 기록을 같은 업무 transaction에 넣고 소비자는 inbox/event ID로 중복을 제거하는 구조를 제안한다. 실제 broker 채택과 무관하게 재처리할 수 있어야 한다.

수신 순서가 바뀌면 revision과 원천 상태를 대조하고, 누락 구간은 재조회한다. 체인 관측 철회는 해당 지급의 acceptance를 재평가해 매출·혜택·여행 입력으로 전달한다. 이미 제공한 상품이나 사용한 혜택을 물리적으로 되돌린 것으로 처리하지 않고 예외/보정 기록을 남긴다.

`ChainObservation` schema는 transaction 관측과 erc20_log 관측을 구분한다. transaction은 logIndex=null이며 reverted receipt도 표현한다. erc20_log는 실제 logIndex와 성공 receipt가 필요하다. 네이티브 송금에 가짜 Transfer log를 만들지 않는다. 자산 이동 상세·UserOperation·DID 오프체인 자격은 추가 DTO로 연결한다. 블록 hash는 관측 버전이며, 지급 고유 ID와 구분한다. 일반적인 앱 상태 갱신 수단(조회/SSE/WS)은 나중에 선택하되 재접속 후 원천 조회를 제공한다.

### 반납 초기화 허가와 완료 증거

`return-checks`는 최신 자산/미확정 거래·지갑 출처·외부 접근·권한 정리 상태를 검증한 뒤 조건이 충족되면 `ResetClearance`를 발급한다. clearance는 rental/device/현재 binding/checklist revision/기한/일회성 challenge를 묶는다. 운영자가 검사 결과를 볼 수 있다는 이유로 사용자의 기기 초기화를 승인할 수는 없다.

소유 앱이 clearance를 전달하고 기기는 검증과 물리적 확인 후 초기화한다. 지갑 키 삭제 후 미설정 상태로 부팅한 기기는 이전 binding·clearance challenge·reset counter를 포함한 `DeviceResetEvidence`를 제공한다. `complete-return`은 서버가 저장한 clearance, 소유자 승인과 이 증거를 검증한 뒤 재대여를 허용한다. 전원 중단/증거 유실은 pending 상태로 남겨 같은 clearance의 처리 여부를 조회한다.

여기서 `deviceProof`는 고객 지갑 키를 초기화 뒤 보존한다는 의미가 아니다. 별도 장치 신원/초기화 증명 경로의 선택·프로비저닝·보호 수준은 HW-01/02·STAMP-04의 상세 설계 항목이다. 문자열 proof 또는 앱의 성공 응답만으로 실제 키 삭제를 증명했다고 처리하지 않는다. schema는 이 증거의 형식만 검사한다.

## 7. 반드시 구현할 교차 검증

| 검증 | 수행 위치 | 관련 작업 |
|---|---|---|
| tokenAddress/decimals/chainId를 등록된 자산과 대조 | intent 생성·제출·대사 | TOKEN-01, PAY-01/04 |
| 서명 payload와 기기 표시·승인 대상 일치 | NU signer·제출 서비스 | HW-05, PAY-02 |
| 주문 만료 뒤 입금/중복 입금/다른 고객 지급 구분 | 대사 서비스 | PAY-03/04 |
| 환불 예약 + 완료 금액 ≤ 실제 환불 가능액 | DB 원자 갱신·환불 서비스 | SHOP-04 |
| 제출 불명확 환불의 한도 예약을 성급하게 해제하지 않음 | 환불 추적·운영 재조회 | SHOP-04, OPS-03 |
| import 지갑의 외부 접근 확인과 신규 지갑 회수 분기 | 반납 화면·기기/대여 서비스 | STAMP-03/04 |
| 동의 철회/삭제 뒤 대기 중 AI 작업도 결과 저장 차단 | 작업 실행/완료 단계·데이터 서비스 | REC-03, TRIP-04 |
| scope 외 키오스크 명령과 탈퇴/반납 후 권한 차단 | 서버 capability + 기기 ACL | HW-03, AUTH-04 |

## 8. 다음 상세화의 입력

화면 흐름과 전체 110개 API의 공통 타입은 후속 문서로 연결했다. 다음은 **DB DDL/마이그레이션 → 계약 ABI/배포 manifest → 실제 BLE transport profile**과 미선택 프로토콜 payload 상세화로 이어진다. 상품 선택이 필요한 actionInput은 D11~16의 선택 결과와 함께 확정한다. 맡을 사람과 공수는 여전히 배정하지 않는다.

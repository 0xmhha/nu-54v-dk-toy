# 나머지 기능의 상세 계약과 저장 작업

현재 승인 경로는 [승인 기준 통합](approval-baseline.md)을 따른다. 물리 SQL·실행 검증과 미선택 정책은 별도다.


전체 범위·앱 연동·12주 완료 전제를 유지한다. 이번 문서는 기존 핵심 8개에 이어 **나머지 102개 API의 요청/응답 타입**과 데이터 저장 작업을 구체화한다. 사람별 배정·공수·실제 구현은 아직 수행하지 않는다.

[102개 API 필드 명세](extended-dtos.md) · [타입 schema](extended-dtos.schema.json) · [계약 원본](extended-dtos.json) · [저장 작업](storage-operations.md) · [기존 핵심 8개](critical-dtos.md)

## 1. 계약 적용 순서

1. API 카탈로그의 endpoint·주체/권한과 화면 흐름을 확인한다.
2. 핵심 8개는 critical DTO, 나머지 102개는 extended DTO의 request/response 타입을 적용한다.
3. 인증·자원 소유·멱등성·revision·업무 상태를 검사한다.
4. ProfileInput이 포함되면 해당 profile의 등록·허용 명령·별도 상세 schema·검증기를 확인한다.
5. 필요한 저장 작업과 이벤트를 실행하고 상태/비동기 operation을 반환한다.

schema 통과만으로 3~5단계를 통과한 것으로 보지 않는다. 타입 정의는 논리 요청이다. GET query의 문자열 변환, 인증 header, 커서 인코딩 등은 transport adapter가 처리한다. API별 정상 응답 코드도 작업용 제안이며 실제 라이브러리 동작을 확인한 결과가 아니다.

## 2. 기능별 확정한 설계 경계

| 영역 | 요청/응답에서 구체화한 내용 | 추가 의미 검사 |
|---|---|---|
| Google/Apple·계정 | 제공자 교환·연결·refresh·logout·회원 조회·탈퇴 입력, 세션/소속 결과 | auth flow·redirect/PKCE·provider subject·재사용/회전·탈퇴 후 접근 |
| 기기 등록·지갑 연결 | enrollment challenge·binding·장치 메타데이터·주소 소유 증명 | 서버의 현 binding·대여자·실제 장치 증명·기기 생성/import 주소 일치 |
| Cloud Wallet | 생성·서명·복구의 proof 입력과 operation 결과 | 참여자 profile·threshold 정책·중단 세션·복구/철회 |
| 매장·메뉴·권한 | 매장/소속·단말 세션·가격/옵션/품절·수취 설정 revision | 다른 매장 격리·관리 모드·자금 권한·메뉴 금액 검증 |
| 기록·스탬프·대여 | 영수증·혜택 상태·사용 요청·대여 조회·기기 반납 상태 | 중복 지급/사용·반납 이후 권한·원지급 관측 보정 |
| FOTA | manifest·release·호환성·withdraw·offer 조회 | 서명/모델/버전·실제 부팅 버전·철회와 진행 중 전송 정책 |
| 녹음·AI 처리 | recording format·완전/부분·전사/요약 job·결과 참조 | 파일 소유·업로드 동의·원음/파생물 삭제·완료 시 동의 재검사 |
| 스마트 계정·DEX·FX | 전환 계획·quote·최소 수취·수수료·LP 보유·행위 intent | 계정 버전·signer·allowance·slippage·가격 시점·실제 pool |
| Perpetual | 가격 상태·포지션·담보·signed 손익·펀딩·청산 이력 | 가격/담보 단위·위험 한도·keeper·개별 실행 성공 |
| STO·DID·x402 | 보유/자격·발급/제시/검증/철회·지불/이용권/전달 상태 | issuer/holder·표준 profile·실제 proof·중복 과금·자원 재조회 |
| 장소·후기·여행 | 좌표/시각/출처·평점·수정 revision·여행/발자취 | 위치 동의·실제/테스트 구매 구분·타인 결제 참조 차단 |
| 추천·챌린지 | 시간/이동/예산·코스 stop·검증 결과·진행/증거 revision | 없는 장소·불가능한 경로·중복 증거·증거 철회 시 보정 |
| 운영·감사 | 제한 조회·예외 재처리·서비스/Indexer 시점 | 운영 권한·재조회 멱등성·민감 로그 제외·수동 체인 성공 생성 금지 |

## 3. 선택되지 않은 프로토콜을 다루는 방식

`ProfileInput = {profileId, data}`는 완성된 표준 payload가 아니다. MPC 참여자 증명, BLE 장치 증명, actionInput, DID claims/presentation, x402 payment requirement 같은 내용에 선택한 프로토콜의 타입을 연결하기 위한 자리다. 내부 `data`를 범용 JSON으로 검사하는 것만으로 실행을 허용하지 않는다.

profile registry에 최소한 다음 내용을 등록해야 한다.

- profile ID/버전, 대상 행위·체인·자산·signer, 허용된 API.
- 상세 JSON/binary schema와 길이·중첩·자원 한도.
- 검증 주체·신뢰 근거·challenge·audience·만료·재전송 방지 규칙.
- 화면에 표시하고 사용자가 승인할 필드, 실제 서명 payload와 결합 방법.
- 비밀/개인 필드의 로그·저장·삭제 정책과 정상/거절 예제.

미등록·철회·범위 밖 profile은 `UNSUPPORTED_PROFILE`로 거절한다. 등록된 profile이라도 proof를 실제 검증하지 못하면 `PROOF_INVALID`다. 예상하지 못한 필드를 허용하는 느슨한 검증으로 대체하지 않는다. EOA와 스마트 계정, credential proof와 raw transaction을 같은 bytes로 다룬다고 가정하지 않는다.

`ProfileReference`/`ProofReference`는 이미 안전하게 취득해 검증할 수 있는 자료의 참조다. 기기에 외부 서버 조회 능력을 추가로 가정하지 않는다. 같은 proof가 다른 주문·계정·목적에 사용되지 않도록 binding을 검사한다.

## 4. 입력의 조건과 경계값

- 위치 검색은 region/query/optionalLocation 중 하나 이상 필요하다. 좌표는 범위를 검사하며 관측 시각·정확도가 없을 때 현재 위치의 증거로 승격하지 않는다.
- 후기 수정은 text/rating 중 하나 이상과 expectedRevision이 필요하다. 구매 표식은 별도 서버 판정이다.
- amountIn·담보·수수료는 자산 ID와 기본 단위 정수다. 손익/펀딩은 음수가 가능한 문자열 타입으로 구분한다.
- balance의 unknown은 금액·관측 시각·블록을 null로 표현한다. current/stale에는 실제 관측 금액·시각·블록이 필요하다. 실제 0과 구별하며 조회 실패를 빈 자산 배열의 성공 응답으로 바꾸지 않는다.
- API-010은 enrollmentId를 별도로 반환한다. 다음 등록 확인 경로와 지갑 binding에서 동일 ID를 사용하며 challengeId와 혼동하지 않는다.
- API-057은 swap/liquidity_add/liquidity_remove별 요청 variant다. swap은 단일 입력/출력, LP 추가는 inputs 배열, LP 회수는 positionId/shareAmount를 받는다. LP quote에는 inputs/outputs와 maximumInputs/minimumOutputs·지분 변화가 포함된다. top-level minimumOutputs도 배열이며 swap은 한 항목으로 표현한다.
- 기간 from≤to, 코스 시작≤종료, stop의 도착≤출발, 단계 간 이동 가능성은 추가 의미 검사다.
- latitude/longitude·rating 1~5·목록 limit 1~100·slippage bps 0~10000 같은 값은 현재 schema의 설계 제안이다. 제품별 더 좁은 한도·성능 목표·시장 정책은 별도로 정한다. schema 범위 전체가 허용 거래 정책이라는 뜻은 아니다.
- POST/PUT/PATCH/DELETE에는 멱등 key가 필요하다. 탈퇴·삭제·로그아웃·refresh는 민감 상태 수명주기 규칙을 추가 적용한다. 예전 결과를 반환해 만료/철회된 자격을 되살리지 않는다.

## 5. 조회·저장 책임

조회가 포함됐다는 이유로 같은 데이터의 원장을 여러 서비스에 만들지 않는다.

| 종류 | 기준 데이터 | 앱/조회 모델 |
|---|---|---|
| 개인/점주 계정·기기 소속 | 계정·membership·device binding | 현재 권한의 projection |
| 주문·지급 배정·환불 예약·혜택 | 업무 원장 | 화면 요약과 기간 집계 |
| 잔액·포지션·토큰 전송 | 지정 체인의 실행과 Indexer 관측 | 블록/시점·유효성을 가진 조회 결과 |
| 원음 | 기본 모바일 파일; 동의 시 지정 처리 저장소 | 서버 recording metadata/job |
| MPC 조각 | 선택 참여자의 격리 저장 경계 | 업무 DB에는 signer/participant 참조만 |
| 추천·발자취 | 동의·장소/결제/후기 출처 | 재생성 가능한 코스/진행 projection |

보존·삭제 정책과 실행 순서는 [저장 작업 명세](storage-operations.md)를 따른다. DB 엔진이나 호스팅 제공자를 이번 문서만으로 확정하지 않는다.

## 6. 검증과 다음 구현 입력

102개 request/response schema의 참조·API 연결·필수 필드, 정상/거절 예제와 업무 경계 시나리오를 검사한다. 프로토콜별 data 검증은 별도 profile 선택과 실제 구현 시험이 필요하다.

현재 전체 107개 API가 **공통 envelope와 기능별 타입**을 갖는다. 다만 기존 핵심의 EOA 상세형과 확장 행위의 profile형은 지원 경로가 다르므로 아래가 아직 남는다: profile별 상세 payload, 보안 adapter의 실제 저장 매핑, 실제 ABI/주소 manifest, 하드웨어 전송 profile. 모두 기존 작업/결정에 연결하며 기능을 범위 밖으로 미루지 않는다.

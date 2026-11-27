# 07편 집필 근거와 검토 기록

이 문서는 로컬 편집·검토 자료이며 Medium 본문에 올리지 않습니다.

- 작성·코드 확인·기존 시험 재실행: 2026-11-09.
- 원고: `07-rental-return.md`, `07-rental-return-en.md`.
- 상태: 한글·영문 로컬 사전 원고와 언어별 PNG 3장. Medium 업로드·공개·예약 발행과 Git 커밋·push는 하지 않았습니다.
- 기존 06편의 발행 상태나 내용을 바꾸지 않았습니다. 사용자 선호에 따라 향후 별도 요청이 없는 공개 발행에서는 구독자 메일 알림을 끄도록 발행 메모에 기록했습니다.

## 1. 완료 범위와 아직 없는 결과

- `products/p05-operations-backoffice/README.md`에는 2026-10-03 테스트넷에서 일회용 기기 계정에 1 tUSDC를 입금하고 계정 종료와 finalized 확인을 마친 기록이 있습니다. 기기가 없어 초기화가 보류됐고, 재실행 때 닫기를 건너뛰었습니다. 원본의 블록 번호는 이번 글의 설명에 필요하지 않아 싣지 않았습니다.
- 이 기록은 보드 초기화, 재부팅 확인, 실제 지연 출금 수령까지 마친 기록이 아닙니다. README의 '돌아간다'는 구현 흐름을 관찰 완료 시제로 바꾸지 않았습니다.
- `docs/content/acceptance/hardware-session.md`의 렌탈 빌드·반납 절은 실행 절차입니다. 그곳의 예상 로그를 이번에 실행한 로그로 인용하지 않았습니다.
- `docs/content/acceptance/week12-log.md`는 결과가 미실행으로 남은 수용 기록지입니다. 11/25 예정 시연을 실제 통과 이력으로 쓰지 않았습니다.
- 운영 API·가게 대시보드·AI 경영 지원을 이번 12주에 만든 기능으로 추가하지 않았습니다. 원고에는 실제 Go 운영 코어와 CLI, 컨트랙트만 설명했습니다.

## 2. 코드와 문장 대조

경로는 프로젝트 루트 기준 상대 경로입니다. 공개 본문에는 파일 이름·공개 함수 이름만 사용합니다.

| 원고 부분 | 근거 | 해석 범위 |
|---|---|---|
| 계정 종료 후 정산·한도 변경 거절 | `products/p06-stablenet-contracts/src/PaymentSettlement.sol`의 `closeAccount`, `_activeSigner`, `_openAccount` | 체인에서 종료가 실행된 뒤의 상태. 트랜잭션 전송만으로 중지됐다고 쓰지 않음 |
| 닫힌 계정 추가 입금 거절 | 같은 파일의 `depositFor` | 새 대여에 닫힌 계정을 다시 활성화한다고 설명하지 않음 |
| 아직 열린 계정만 닫기 | `products/p05-operations-backoffice/internal/core/ops/return.go`의 `Return` | 첫 번째 Go 발췌 6줄. 실패하면 reset에 도달하지 않음 |
| 일반 출금은 계정을 열어 둠 | `PaymentSettlement.sol`의 `requestWithdrawal`, `executeWithdrawal` | 요청액은 예약만 하며 잔액을 차감하지 않음. 현재 코드는 요청 뒤 서명인지 구별하지 않으므로 '요청 전에 서명한 결제만 가능'으로 제한하지 않음 |
| 일반 출금 취소·종료 후 취소 불가 | 같은 파일의 `cancelWithdrawal`, `_openAccount`, `test/Extensions.t.sol` | 지연 중 일반 출금 취소와 계정 종료를 혼동하지 않음 |
| 영수증 성공 뒤 확정 이벤트 조회 | `products/p05-operations-backoffice/internal/core/chain.go`의 `send`, `AccountClosedBlock`, `Dial` | `WaitMined`와 Status 확인 후 별도로 finalized 범위의 AccountClosed를 찾음 |
| 이벤트 조회 범위 | 같은 파일 `AccountClosedBlock` | 배포 시작 블록부터 노드의 read head까지, 해당 기기·해당 배포 이벤트를 필터링. 기본 read는 FinalizedBlockNumber |
| 재시도 기본값 | `return.go` | FinalWait 60초, Poll 2초. 체인 확정 지연의 성능 측정치가 아님 |
| 초기화 순서는 운영 도구가 지킴 | `return.go`, `products/p01-device-firmware/core/src/nu54_session_setup.c`의 `ss_device_reset` | 기기는 세션·대상·운영자 서명을 검사하나 AccountClosed를 조회하지 않음 |
| 연결 불가의 부분 결과 | `return.go`의 `ResetPending` 분기 | Reset=false이며 nil error와 부분 결과가 올 수 있음. nil error만으로 전체 반납 성공을 판정하면 안 됨 |
| 잘못된 기기 거절 | 같은 파일 DeviceAddress 검사 | 두 번째 Go 발췌 3줄. 실제 주소를 넣지 않음 |
| 초기화 응답 유실 | `return.go`, `products/p05-operations-backoffice/internal/ble/session.go`의 `SendDeviceReset`, `ack` | ack 실패는 오류로 반환. 응답만 잃은 경우를 자동으로 완료 처리·재조정하는 로직은 없음. 실제 응답 유실을 재현한 결과라고 쓰지 않음 |
| 반환 주소 고정·호출 권한 | `PaymentSettlement.sol`의 `depositFor`, `executeWithdrawal` | 실행은 누구나, 목적지는 최초 등록 주소. 운영자가 임의 주소를 고를 수 있다고 쓰지 않음 |
| 상태 갱신 뒤 토큰 호출 | 같은 파일 `executeWithdrawal`, `_push`, `_call` | Solidity 발췌 6줄. 토큰 호출 실패가 TransferFailed revert로 연결됨 |
| 시험 토큰 수신 보류 가능 | `src/test-token/TestUSDC.sol`의 `wouldHold`, `_send` | 신뢰 송신자와 opt-out 설정에 따라 보류를 건너뛸 수 있음. 모든 지급이 보류된다는 설명은 하지 않음 |
| CLI 출금 명령 미연결 | `products/p05-operations-backoffice/cmd/opsctl/main.go`의 `commands`, `run` map | help에는 withdraw request/cancel/execute가 있으나 실행 handler map에는 없음. 컨트랙트 함수의 존재와 CLI 완료를 구분 |
| 보안 채널 필수 설정과 반납 연결 불일치 | 같은 파일 `rentalReturn`, `openSession`, `internal/ble/session.go`의 `Open`, 펌웨어 `core/src/nu54_session.c`의 `session_open` | Go 경로는 임시 공개키·가맹점 증명서 없이 payment 모드를 엶. 펌웨어 require_secure 조건은 평문 payment를 거절. 코드에서 도출한 연결 한계이며 이번에 실제 무선으로 재현한 시험은 아님 |

코드 블록 3개는 각 원본의 연속 부분과 비교했습니다. Go의 들여쓰기만 정규화하고 코드·오류 문자열·숫자는 바꾸지 않았습니다. 개인키 생성·보관·서명 내부 코드는 공개 발췌하지 않았습니다.

## 3. 수치와 시간축

- 설정 출금 지연은 `docs/content/planning/design-freeze-checkpoint-02.md` parameters, 해당 JSON, `products/p06-stablenet-contracts/deployments/8283.json` parameters의 3,600초입니다.
- `test/Base.t.sol`의 WITHDRAWAL_DELAY도 3600입니다. 시각 이동은 로컬 가상 머신의 `vm.warp`이며 실제로 한 시간 기다린 결과가 아닙니다.
- Go `fakeReturnChain`의 WithdrawAfter는 고정된 시험값이며, 이 시험은 값이 0이 아닌지를 확인합니다. 이 값을 실제 배포의 출금 지연이나 지급 완료 시각으로 사용하지 않았습니다.
- 60초·2초는 운영 도구의 이번 시도 대기와 조회 간격입니다. 3,600초는 별도의 컨트랙트 시각 조건입니다.
- 그림 2의 눈금 0, 900, 1800, 2700, 3600은 설정 지연을 같은 간격으로 나눈 축입니다. 평균이나 실측값을 만들지 않았습니다. 두 행의 0은 각각 해당 함수가 실행된 시각입니다.
- 최초 테스트넷 기록의 1 tUSDC를 이번 로컬 시험의 가상 잔액과 섞지 않았습니다. 시험용 키·전체 주소·nonce 원문·트랜잭션 해시는 싣지 않았습니다.

## 4. 2026-11-09 실행한 기존 시험

### 운영 도구 반납 순서

작업 디렉터리는 `products/p05-operations-backoffice`입니다.

```sh
go test -count=1 -v ./internal/core/ops -run '^TestReturn'
```

기본 환경에서는 의존성 다운로드가 막혀 setup failed였습니다. 임시 Go 작업 파일과 캐시를 사용하고 공식 Go 모듈 저장소의 의존성을 받은 뒤 같은 소스를 실행했습니다. 작업 파일·캐시는 임시 디렉터리에 두었고 저장소의 go.mod, go.sum, go.work는 수정하지 않았습니다.

통과한 시험:

```text
--- PASS: TestReturnClosesThenSendsTheVectorReset (0.00s)
--- PASS: TestReturnAgainSkipsTheClose (0.00s)
--- PASS: TestReturnSendsNoResetWhenTheCloseIsNotDone (0.06s)
--- PASS: TestReturnRefusesAnotherDeviceAndKeepsAnUnreachableOnePending (0.00s)
PASS
```

4개 시험 함수가 여러 조건을 묶어 검사합니다. 가짜 체인과 미리 정한 BLE 메시지 벡터를 사용하며 실제 키·주소·서명 값은 원고에서 제외했습니다. 이 결과는 Go 코드의 순서와 메시지 일치를 확인하며 실제 체인·보드·보안 채널 연동 성공을 뜻하지 않습니다.

### 컨트랙트 출금·계정 종료

작업 디렉터리는 `products/p06-stablenet-contracts`입니다.

```sh
forge test --match-contract ExtensionsTest --match-test 'test_(withdrawal_|closeAccount_|depositFor_closed|execute_permissionlessFixedDestination)' -v
```

선택된 11개가 모두 통과했습니다. 이름은 다음과 같습니다.

```text
test_closeAccount_blocksSettle
test_closeAccount_paysAfterDelay
test_depositFor_closedReverts
test_execute_permissionlessFixedDestination
test_withdrawal_cancel
test_withdrawal_cancelOnlyDuringDelay
test_withdrawal_delayed
test_withdrawal_fixedDestination
test_withdrawal_operatorOnly
test_withdrawal_settleDuringDelay
test_withdrawal_unknownAccount
```

과거 전체 시험 96개를 이번에 모두 재실행했다고 쓰지 않았습니다. 본문에는 새로 확인한 11개 범위만 썼습니다.

### 닫힌 계정 불변식

```sh
forge test --match-test invariant_closedAccountNeverSettles -v
```

핵심 출력:

```text
[PASS] invariant_closedAccountNeverSettles() (runs: 64, calls: 12800, reverts: 0)
```

핸들러는 입금·결제·출금 요청/취소/실행·계정 종료·시간 이동 등을 섞습니다. 함수 내부에서 예상 거절을 catch할 수 있어 출력의 reverts:0은 모든 정산 시도가 성공했다는 뜻이 아닙니다. 12,800을 결제 횟수나 공격 횟수로 표현하지 않았습니다.

Foundry는 시험 통과 후 전역 signature cache 쓰기 권한 경고를 냈습니다. 시험의 exit code는 0이었고, 저장소 소스에 변화가 없습니다. 경고를 숨겨 시험 실패를 성공으로 바꾼 것은 아닙니다.

보드 플래시, 실제 BLE 초기화, 키 삭제, 테스트넷 입출금이나 계정 종료 트랜잭션은 이번 작업에서 실행하지 않았습니다.

## 5. 레퍼런스

2026-11-09 공식 원문을 확인했습니다. 외부 표준은 개념 설명이며 프로젝트의 실기 성공 근거가 아닙니다.

1. [Ethereum JSON-RPC](https://ethereum.org/en/developers/docs/apis/json-rpc/) — finalized block parameter와 로그 조회 범위. Ethereum의 확정 시간이나 합의 조건을 StableNet의 측정값으로 옮기지 않았습니다.
2. [Solidity 0.8.30 Security considerations](https://docs.soliditylang.org/en/v0.8.30/security-considerations.html#use-the-checks-effects-interactions-pattern) — 상태 갱신 뒤 외부 호출 순서. 이 패턴만으로 제품 보안 감사를 통과했다고 하지 않음.
3. [Solidity 0.8.30 Error handling](https://docs.soliditylang.org/en/v0.8.30/control-structures.html#error-handling-assert-require-revert-and-exceptions) — revert가 현재 호출과 하위 호출의 상태 변경을 되돌리는 의미.

프로젝트 저장소는 마지막 편에서 공개하므로 레퍼런스에 넣지 않았습니다.

## 6. 이미지·편집 확인

- `assets/07/build-diagrams.py`가 06편의 서체·색·그리기 도우미를 재사용해 PNG를 만듭니다. 1600×1100, 언어별 3장입니다.
- 직접 작성한 도식이며 AI 사진·가상 앱 화면은 없습니다. 이 글에서 설명할 실제 앱 화면의 신규 결과가 없으므로 모양만 만든 앱 캡처를 넣지 않았습니다.
- 그림 1은 종료 실행 → 확정 확인 → 초기화의 순서와 종료 시각에 시작하는 출금 대기 경로를 나눴습니다. 실기 완주 증거로 표시하지 않습니다.
- 그림 2만 비례 시간축입니다. 그림 1·3의 박스 위치·간격은 개념 배치입니다.
- 6장을 나란히 열어 글자 잘림·겹침·화살표·양 언어의 숫자와 흐름을 확인했습니다.
- 본문은 `~습니다`와 1인칭을 사용하고 사건·수치·코드·미검증 범위를 양 언어에서 맞췄습니다. 기록에 없는 실패 경험이나 감정은 추가하지 않았습니다.
- 내부 개발 ID, 실제 키·전체 주소·거래 해시·계정명·내부 절대 경로를 공개 본문과 이미지에서 제외했습니다.

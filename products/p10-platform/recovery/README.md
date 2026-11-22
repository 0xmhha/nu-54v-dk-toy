# P10 격리 복원과 전환

[D06 복원·승격](../../../docs/content/commercial/screens/specs/D.md#d06--서비스-복원승격)과 [X05 격리 재생](../../../docs/content/commercial/screens/specs/X.md#x05--격리-재생과-조회-전환)은 복원 작업 ID, 격리 상태, 삭제 고수위, 원장·확정 체인 대조 결과, 대상별 revision 및 서명 영수증을 읽는다. [P09 삭제 대장 계약](../../p09-travel-ai/PRIVATE-TRAVEL.md#restore-contract-for-21)과 [플랫폼 복원 승인](../../../docs/content/commercial/architecture/platform.md)을 적용한다.

## 합성 복원 리허설

```sh
POSTGRES_BIN=/path/to/postgresql/bin make commercial-integration-test
```

게이트에서는 새 PostgreSQL에 실제 P05 원장과 P07 인덱스를 구성하고, 격리 Anvil에 서명 거래를 제출해 지급·환불을 확정한다. P05 지급·출고·인쇄 등 실제 흐름 뒤에 `pg_dump`가 사용한 동일한 PostgreSQL 스냅샷에서 원장과 인덱스의 건수 및 행 집합 해시를 얻는다. 서명한 dump를 별도 Unix 소켓의 PostgreSQL로 복원하고 일반 읽기·쓰기와 작업자의 처리를 거절한다. 복원 검증의 RPC는 블록·영수증·로그 조회만 호출한다. 작업을 끝내면 격리 클러스터를 정지한다. `NU54_RECOVERY_REHEARSAL_OUTPUT`에 새 디렉터리를 지정하면 P05/P07 대조 증거와 P07 재시작 실측을 유지한다. P07 재시작 시험은 기존 프로세스가 DB 작업 임대를 보유한 상태에서 강제 종료하고, 같은 임대가 만료될 때까지 대체 프로세스의 쓰기가 없음을 검사한다. 만료 뒤 진행한 revision·높이와 원 지급의 근거·일지·outbox를 기록한다.

P09 리허설은 삭제 전에 별도 백업을 실제로 만든다. 두 번째 클러스터에서 `erasured`가 서명한 삭제 대장을 내보낸다. 격리 작업 클러스터의 `recordsd`는 mTLS로 받은 페이지의 서명·네임스페이스·연속 커서를 확인하고 선택 기록과 종속물을 지운다. 리허설은 첫 삭제 뒤 관련 없는 기록이 남는지, 반복 적용 뒤 같은 결과인지, 새 삭제가 생기면 운영 읽기가 닫히는지, 새 페이지 적용 뒤 삭제한 데이터가 읽히지 않는지 확인한다. 이 경로는 실사용 기록과 외부 공급자 자격 없이 실행한다.

900초 RPO와 3600초 RTO는 리허설 입력값이다. 제품 런타임이나 복원 도구에 운영 기본값으로 들어가지 않는다. 운영 승격에는 합의한 목표의 참조와 측정 증거 및 실제 대상 시스템의 설치·전환 권한 검토가 필요하다.

## 작업자 명령

승인된 소유자 DB에서 스냅샷을 생성한다. P05/P07이 같은 소유 경계의 DB를 함께 쓰면 같은 MVCC 시점을 사용한다. 계정과 암호는 libpq 서비스와 passfile에 보관하고, 새 백업 디렉터리는 암호화된 승인 저장소에 둔다. 같은 스냅샷 안에 P09나 다른 제품이 섞여 있으면 중지한다.

```sh
python3 products/p10-platform/recovery/recover.py backup \
  --scope "$scope" --p05 service=p05-backup --p07 service=p07-backup \
  --archive "$archive" --sign-key "$backup_signer"
python3 products/p10-platform/recovery/recover.py inspect \
  --job "$job" --scope "$scope" --archive "$archive" --backup-key "$approved_backup_public_key" \
  --isolation-root "$private_local_root" --rpc "$approved_chain_rpc" --erasure "$erasure_config" \
  --incident-epoch "$incident_epoch" --goal-ref "$goal_ref" --rpo "$goal_rpo" --rto "$goal_rto" \
  --report "$private_report"
```

검증은 제품 원장과 현재 확정 RPC 양쪽에서 지급·환불을 비교한다. 일반 출고·여행 결제·디지털 구독에 각각 맞는 pickup·outbox·이용 권한 규칙, 잔액 계정, 일지, 지급·환불 타깃, RPC 로그와 금액, 누락과 중복, 미해결 지급·출고 및 인덱서 사고를 검사한다. RPO에는 가장 오래된 데이터베이스 스냅샷부터 사고 시계까지의 구간을 기록한다. RTO는 사고 시계부터 커밋 가능한 입구 전환까지를 별도로 닫으며 검증까지만 걸린 시간을 구분해 기록한다.

삭제 환경 파일에는 `namespace`, 선택적인 `backupCursor`, 그리고 mTLS `journal`과 `workload`가 들어간다. 각 항목의 `origin`, `ca`, `cert`, `key`는 운영자가 독립 설정한다. 마지막 적용 뒤 최신 고수위를 다시 조회한다. P10이 전환할 때는 커밋까지 삭제 대장 쓰기 잠금과 호환되는 읽기 잠금을 유지한다. P09 복원 세대는 출시 후에도 일반 읽기·쓰기와 공개 후기 읽기에 같은 잠금 및 로컬 커서 대조를 강제한다. 커서가 뒤처지면 503으로 응답한다.

## 승인, 전환, 조회

마이그레이션 003의 역할로 운영 계정·타깃 계정·읽기 계정을 구분한다. `platform.recovery_route`는 코호트별 입구를, `platform.recovery_target_route`는 각 제품 소유 DB의 대상 포인터를 설정한다. `cutover.py`의 `--policy`는 데이터베이스 외부에서 관리하는 등록 목록이다.

- `scopes`에는 코호트 ID별 목표 참조와 RPO/RTO, 승인 TTL 상한, 검증 환경, 대상별 Ed25519 공개키 파일을 등록한다. P09가 포함된 경우 승인된 `erasureNamespace`와 읽기 전용 `erasureGuardDatabase`도 등록한다.
- `principals`에는 작업자별 UUID, IdP 원 주체 `issuer`와 `subject`, 동일인을 판별할 내부 `person` 별칭, 승인 역할·범위·공개키 파일을 등록한다. 각 주체와 공개키, 대상별 공개키가 분리되어야 한다.
- 승인할 동일한 계획에 작업·코호트, 이전 활성 manifest, 복원 manifest, 증거 파일 해시, 목표, 발급·만료 시각, 입구와 각 대상의 기대 revision을 지정한다. `vote --role requester|verifier|applier`를 각 보유자가 독립적으로 실행해 투표 서명을 만든다. 서명 꾸러미는 `plan`과 역할별 `votes`를 포함한다.
- `fence`로 승격 입구를 차단한다. 소유 DB에서 `target-apply`를 호출하면 해당 대상이 자기 revision을 CAS하고 서명 ACK를 같은 트랜잭션에 남긴다. 응답이 끊겼으면 같은 작업 ID로 다시 조회한다. 각 ACK를 `collect`하고 `finish`하면 모든 대상의 영수증, 승인 TTL, 목표 시간, 삭제 커서를 확인한다. 부분 응답 중에는 입구를 닫아 둔다. 복귀 계획은 이전 작업 ID와 복귀할 manifest를 서명한다.
- `status --scope "$scope" --job "$job"`에서 현재 계획, 입구 revision, 대상별 서명 영수증, 완료 digest를 조회한다. `recovery_operator.py`는 mTLS에 등록한 운영자에게 `/D06/{scope}/{job}`, `/X05/{scope}/{job}`와 `/v1/recovery/{scope}/{job}`를 제공한다. 검사 보고서는 `--reports` 아래에 내용 해시를 파일명으로 저장한다. 승인 전 검사 상태는 동일한 디렉터리의 `{scope}/{job}.json`에서 조회한다.

관리되는 P05·P07 배포는 각각 `Pxx_RECOVERY_DATABASE_URL`, `Pxx_RECOVERY_SCOPE`, `Pxx_RECOVERY_MANIFEST`를 구성한다. 접속에는 계획·영수증을 볼 수 없는 `nu54_p10_recovery_route_only` 로그인만 사용한다. 같은 대상의 구·신 세대는 서로 다른 앱 DB 연결을 쓴다. P09는 여기에 `P09_LIVE_ERASURE_GUARD_DATABASE_URL`로 `nu54_p09_erasure_guard` 최소 권한 접속을 추가한다. 일반 요청과 작업 루프는 열린 세대인지 확인하는 동안 작업 단위의 공유 입구 잠금을 유지한다. 격리 내부 DB 포트, 기존 운영 로그인, 외부 지급·출고·알림 네트워크와 키에 접근할 수 있는 주체는 별도 운영 구성으로 한정한다.

상용 목표와 담당자는 운영 등록 목록에서 승인한다. 합성 테스트의 키, 목표와 DB 위치는 상용 증거로 제공하지 않는다. 출시 전에는 P05/P07 후보 DB를 소유자가 운영 토폴로지에 배치하고 실제 제품의 재시작·출입구 규칙과 외부 효과 펜스를 확인해야 한다.

D06·X05의 390px·글자 200% 회귀는 테스트 인증서를 사용하는 실제 Chrome을 실행한다. 격리 PostgreSQL 소켓을 설정한 뒤 다음 명령으로 실행한다.

```sh
NU54_RECOVERY_BROWSER_CLI=/path/to/playwright-cli \
  python3 products/p10-platform/recovery/test_cutover.py --existing-postgres
```


P10 마이그레이션 006의 `platform.recovery_operator_enrollment`에 HTTP 상태/전환 서비스 로그인, mTLS 운영 주체, 범위, 권한과 만료를 등록한다. 홈 상태에는 `read`, 전환 시작에는 `requester`, 영수증 수집과 전환 완료에는 `applier`를 부여한다. 운영 정책 파일의 운영 주체에는 Unix 초 단위 `expiresAt`을 지정하고, 철회 인증서의 SHA-256을 `revokedCertificates`에 등록할 수 있다. 인증서 검증은 현재 클라이언트 CA와 중간 인증서의 만료를 반영한다.

조회는 보고서를 보낼 때 권한을 재확인한다. HTTP 전환은 DB 대기 뒤 권한을 검사하고, 같은 잠금 트랜잭션 안에서 작업을 준비한 뒤 커밋 직전에 설정·인증서·승인 키와 DB 시계를 다시 확인한다.

## D06 격리 작업 접수

마이그레이션 011은 전환 기록과 별개로 리허설 상태와 서명 보고서 지문을 보관한다. 운영 화면은 `--rehearsal-config`가 등록된 경우 `/D06/{scope}`에 접수 가능한 프로필과 최근 작업을 표시한다. 프로필은 서버 구성에 등록한 이름이다. 스냅샷 경로와 해시, RPC, 목표와 삭제 대장 설정은 서버에서 고정한다.

```json
{"scopes":{"<scope>":{"p05-p07":{
  "archive":"/private/approved/snapshot","backupKey":"/private/approved/backup.pub",
  "snapshot":"<sha256-of-manifest.json>","rpc":"<approved-read-rpc>","synthetic":true,
  "goalRef":"<goal-uuid>","goals":{"rpoSeconds":900,"rtoSeconds":3600}
}}}}
```

운영 증거 프로필에는 승인된 사고 시각 `incidentEpoch`과 삭제 전파 파일 `erasure`, 해당 파일의 해시 `erasureConfigDigest`도 등록한다. 라이브 공급자와 서비스 입력은 승인된 환경에서 각각 구성한다. 읽기 권한자는 작업만 열 수 있고, 접수와 중단에는 그 범위의 요청자 권한이 필요하다. 같은 작업 ID·범위·요청자·프로필의 재제출은 기존 작업에 연결한다. 중단 후 늦게 끝난 작업자가 보고서를 승격할 수 없다.

```sh
python3 products/p10-platform/recovery/preparation.py --watch \
  --database service=p10-restore-worker --config "$profiles" \
  --isolation-root "$short_private_root" --reports "$report_store" --report-key "$report_signer"
```

작업자 DB 로그인은 `nu54_p10_recovery_worker`를 사용한다. 운영자 화면과 작업자는 입력 설정과 보고서 저장소를 공유한다. 보고서 공개키 `rehearsalReportKey`를 해당 복구 범위의 승인 정책에 등록한다. `requireManagedRehearsal`을 켜면 세 승인자의 별도 서명 전에 완료된 리허설 보고서를 요구한다. 격리 접수와 작업 완료는 운영 입구를 전환하지 않는다.

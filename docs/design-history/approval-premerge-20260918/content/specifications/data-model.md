# 데이터 저장·무결성·수명주기 초안

[연결 명세](interface-contracts.md)의 저장 책임을 구체화한다. 아래는 DB 엔진을 확정한 DDL이 아닌 논리 모델이다. 계정 서버에 고객 HW private key 또는 MPC 전체키를 저장하는 필드는 없다.

후속 산출물: [PostgreSQL 참조 DDL·관계·마이그레이션](database/README.md). 운영 엔진을 확정하지 않고 구체 제약을 검토할 수 있도록 작성했다.

보안 후속 모델은 [논리 저장 계약 15개](security-storage-contracts.json)와 [API 통합 결과](security-api-integration.md)에 연결했다. AuthFlow·ReceiptOwnership·PendingBenefitEntitlement·ProtectedObject 등은 기존 61개 SQL 테이블과 별개의 adapter 설계이며 물리 저장 매핑은 미선정이다.

## 1. 논리 테이블과 소유 서비스

| 테이블/모델 | 핵심 필드·연결 | 고유/동시성 규칙 | 소유 |
|---|---|---|---|
| accounts, auth_identities | account_id, provider, provider_subject | provider+subject 고유; 이메일 자동 병합 금지 | 계정 |
| sessions, auth_flows | account_id, device/terminal, expiry, revoked_at | 세션 회전/철회; auth flow 소비 여부 | 계정 |
| store_memberships | store_id, account_id, role | store+account 고유; 마지막 소유자 제거 보호 | 매장 |
| wallets, wallet_bindings | wallet_id, kind, chain, address, signer_ref, account/store binding | 주소의 합법적 공동 사용과 signer 정책 분리; 한 주소=한 사람 가정 금지 | 지갑 |
| mpc_participants, signing_sessions | wallet_id, participant_ref, session_id, digest, state | 조각 값 대신 격리 저장소 참조; 중단 세션 정책 적용 | MPC 경계 |
| devices, device_bindings | device_id, model, firmware, current_binding, revision | 기기별 현 활성 소유 binding 하나 | 기기 등록 |
| rentals, return_checks | rental_id, device_id, account_id, wallet_origin, check_revision | 활성 대여 중복 금지; 모든 필수 점검 통과 후 cleared | 대여 |
| recipient_configurations | store_id, wallet_ref, proof_ref, revision, validity | 변경 권한/소유 proof; 주문에 주소 snapshot | 매장 자금 설정 |
| menu_items, menu_revisions | store_id, item_id, options, price, availability | revision 충돌 검사; 과거 주문 snapshot 유지 | 매장 |
| orders, order_lines | order_id, store_id, items_snapshot, amount, recipient_snapshot, state | 주문 생성 멱등성; 메뉴/주소 변경 소급 금지 | 주문 |
| payment_attempts, transaction_intents | attempt_id, order_id, quote, expiry, signer_binding, digest | intent의 승인 대상 불변; 변경 시 새 intent | 결제 |
| transaction_submissions | intent_id, chain_id, tx_hash, userop_ref, state | 체인/tx 고유; 하나의 attempt에 여러 교체/제출 기록 가능 | 중계 |
| chain_observations | chain_id, tx_hash, observation_kind, nullable log_index, block_hash, revision, canonicality | ERC20 지급 정체성 chain/tx/log와 관측 이력 분리; 거래 관측은 log_index 없음 | Indexer 어댑터 |
| payment_allocations | payment_evidence_id, order_id, attempt_id, accepted_amount | 한 지급 증거의 중복 주문 배정 금지 | 대사 |
| refunds, refund_reservations | refund_id, payment_id, amount, destination_proof, state | 원자적으로 한도 예약; confirmed+유효 예약 합계 한도 | 환불 |
| settlements, settlement_revisions | store_id, period, observed/expected/refund totals, revision | 기간·자산별 기준; 마감 후 보정 이력 | 정산 |
| benefit_entries, redemptions | account_id, store_id, source_payment, rule_version, delta | 원천 지급+규칙별 중복 발급 금지; 사용 원자성 | 혜택 |
| firmware_releases | model, version, manifest, package_ref, signing_key_ref, state | 파일 변경 시 새 릴리스; 배포 중지와 기기 상태 분리 | 릴리스 |
| recording_metadata, processing_jobs | recording_id, owner, file_ref, completeness, consent_revision, state | 로컬 원음 경로는 앱 저장; 업로드는 동의한 경우만 | 앱/기록 |
| credential_metadata, credential_status | credential_id, issuer, subject_ref, expiry, status | 개인 claims 공개 체인 저장 금지; 공개 상태/개인 내용 분리 | 자격 |
| paid_resource_requests | resource_request_id, price, payment_ref, entitlement, delivery_state | 지급과 전달 상태 별도; 재전송 시 동일 이용권 복구 | x402 연결부 |
| places, store_place_links | provider, provider_place_id, store_id, observed_at | 제공자 ID의 namespace 구분 | 장소 |
| reviews, trips, travel_evidence | owner, place_id, source_type, source_ref, validity, consent | 타인 구매 증거 금지; 테스트/실제/위치/자기보고 분리 | 여행 |
| itineraries, participations, challenge_entries | owner, constraints, evidence_refs, source_revisions | 단계/증거별 중복 완료·보상 차단 | 추천/챌린지 |
| consents, privacy_requests | account_id, purpose, revision, scope, state | 동의 변경/삭제 요청 이력; 작업 큐까지 철회 반영 | 개인 데이터 |
| operations, idempotency_records | principal_scope, endpoint, key, normalized_digest, result_ref | 동일 키·다른 의미 거절; 비밀 원문 미보관 | 업무 공통 |
| outbox, inbox, audit_events | event_id, aggregate_revision, actor, safe_diff | event 고유·재전송; 민감 payload 제외 | 업무 공통 |

온체인 pool/포지션/잔액 조회는 Indexer의 projection이다. 앱 DB 값을 체인의 실제 담보·잔액으로 단독 사용하지 않는다. 기존 Indexer 저장소를 위 표 전체의 업무 DB로 바꾸라는 요구가 아니다.

## 2. 원자적으로 처리할 경계

1. **주문 생성:** 가격·메뉴·수취 설정 검증, snapshot, 멱등 결과를 같은 업무 경계에서 저장한다.
2. **결제 수락:** 검증한 지급 증거의 배정, acceptance revision, outbox를 함께 저장한다. 매출/혜택 소비자는 별도 멱등 반영한다.
3. **환불 요청:** 원지급의 환불 가능액 검사와 한도 예약을 잠금/조건부 갱신으로 묶는다. 동시에 들어온 두 요청이 같은 한도를 소비할 수 없다.
4. **혜택 사용:** 유효 혜택 확인과 소비 기록을 원자적으로 묶는다. 단말이 둘이어도 이중 사용되지 않는다.
5. **반납 완료:** 최신 checklist·초기화 확인·활성 binding 철회·재대여 가능 전환을 함께 검사한다. 앱의 완료 버튼만 신뢰하지 않는다.
6. **개인 자료 삭제:** 접근 차단/처리 취소를 먼저 기록하고 파생 데이터/저장소 삭제를 작업으로 추적한다. 일부 실패를 전체 완료로 표시하지 않는다.

체인 제출은 DB transaction과 원자적일 수 있다고 가정하지 않는다. 제출 의도를 먼저 저장하고 tx hash 기준으로 재조회·복구한다. 관측 보정도 원장 삭제가 아니라 원인과 새 revision을 남긴다.

## 3. 보존·삭제·복구

| 데이터 | 처리 원칙 | 정할 값 |
|---|---|---|
| 니모닉/private key import | 폰 입력과 기기 보안 경로에서만 일시 처리; 서버 DB/로그/일반 큐 제외 | 입력 버퍼·백업 방지·기기 저장 정책 |
| MPC 조각 | 선택한 참여자 경계의 저장소; 업무 DB는 참조만 저장 | 제공 경로·회전·복구·철회 |
| 주문/지급/환불/감사 | 업무 이력과 개인 프로필 분리; 삭제 요구의 적용 범위 설명 | 보관 정책과 접근 역할 |
| 원음/전사/요약 | 소유자 접근; 처리 동의와 파생물 계보 추적 | 기본 로컬/클라우드 설정·보관 기간 |
| 위치/후기/추천 입력 | 목적별 동의와 출처; 삭제/철회 후 추천/챌린지 입력 갱신 | 정밀도·수집 방식·기간 |
| 공개 체인 자료 | 개인 서버 삭제와 체인 기록의 지속성을 구분 | 개인 정보가 체인에 올라가지 않을 필드 설계 |
| 백업 | 삭제/철회 기록을 복원 시 다시 적용해 과거 권한·자료를 부활시키지 않음 | 복구 목표·백업 주기·키 관리 |

보관 기간이나 실제 상거래 운영에 필요한 법적 보존 의무를 이 문서에서 임의 확정하지 않는다. 현재 작업은 테스트넷 제품 설계이며, 법률 적합성 검토 결과를 의미하지 않는다.

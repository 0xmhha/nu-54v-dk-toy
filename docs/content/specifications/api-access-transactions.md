# API별 읽기·쓰기·트랜잭션 연결

[권한 규칙](access-transactions.md) · [논리 adapter 자원](security-storage-contracts.json). SQL 테이블과 adapter 자원을 구분한다. 런타임 권한/SQL GRANT가 아니다.

| API | 요청 | 권한 | SQL 읽기 | SQL 쓰기 | Adapter 읽기 | Adapter 쓰기 | 트랜잭션 |
|---|---|---|---|---|---|---|---|
| API-001 | POST /v1/auth/exchanges | public_auth_flow | accounts, auth_identities, auth_sessions | accounts, auth_identities, auth_sessions | AuthFlow | AuthFlow, RefreshFamily | TX-01 |
| API-002 | POST /v1/auth/links | account_recent_auth | accounts, auth_identities, auth_sessions | auth_identities, auth_sessions | AuthFlow | AuthFlow | TX-01 |
| API-003 | POST /v1/auth/logout | account | auth_sessions | auth_sessions | RefreshFamily | RefreshFamily, RotationOutcome | TX-01 |
| API-004 | POST /v1/account-deletions | account_recent_auth | accounts, auth_sessions, wallet_bindings, device_bindings, rentals | accounts, auth_sessions, privacy_requests, operations | — | — | TX-09 |
| API-005 | GET /v1/me | account | accounts, auth_identities, memberships | — | — | — | READ |
| API-006 | POST /v1/auth/refresh | refresh_session | auth_sessions | auth_sessions | RefreshFamily, RotationOutcome | RefreshFamily, RotationOutcome | TX-01 |
| API-007 | GET /v1/wallets | account | wallets, wallet_bindings | — | ApprovalBinding, AuthorizationGate | — | READ |
| API-008 | GET /v1/stores/{storeId}/wallets | store_asset_read | memberships, stores, wallets, wallet_bindings | — | ApprovalBinding, AuthorizationGate | — | READ |
| API-009 | GET /v1/wallets/{walletId}/balances | wallet_read | wallets, wallet_bindings, assets, chain_projections, memberships, stores | — | ApprovalBinding, AuthorizationGate | — | READ |
| API-010 | POST /v1/device-enrollments | account | devices, rentals, device_bindings, protocol_profiles | enrollments | — | — | TX-02 |
| API-011 | POST /v1/device-enrollments/{enrollmentId}/confirm | account | enrollments, devices, rentals, device_bindings | enrollments, device_bindings, devices | — | — | TX-02 |
| API-012 | DELETE /v1/devices/{deviceId}/binding | device_owner | devices, device_bindings, rentals, return_checks | device_bindings, devices | — | — | TX-02 |
| API-013 | GET /v1/devices/{deviceId} | device_owner | devices, device_bindings, rentals | — | — | — | READ |
| API-014 | POST /v1/cloud-wallets | account_recent_auth | accounts, wallets, wallet_bindings, protocol_profiles | operations | — | — | TX-13 |
| API-015 | POST /v1/wallets/{walletId}/signing-sessions | wallet_sign_authorized | wallets, wallet_bindings, transaction_intents, mpc_participants, protocol_profiles, memberships, stores | signing_sessions, operations | ApprovalBinding, AuthorizationGate | SignatureProvenance | TX-13 |
| API-016 | POST /v1/wallets/{walletId}/recoveries | wallet_recovery_proof | wallets, wallet_bindings, mpc_participants, protocol_profiles | operations | — | — | TX-13 |
| API-017 | POST /v1/transaction-intents | wallet_use | wallets, wallet_bindings, assets, protocol_profiles, memberships, stores | operations, transaction_intents | ApprovalBinding, AuthorizationGate | ApprovalBinding | INTENT_CREATION |
| API-018 | POST /v1/transaction-submissions | intent_submit_capability | transaction_intents, transaction_submissions, protocol_profiles | transaction_submissions, operations | AccessGrantLineage, ApprovalBinding, AuthorizationGate, CapabilityReservation, ScopedCapability, SenderProofReplay | AddressNonceTrack, CapabilityReservation, SenderProofReplay, SignatureProvenance, SignedPayloadObject, SubmissionDispatch | REQUEST_SUBMISSION |
| API-019 | GET /v1/transactions/{transactionRef} | transaction_read | transaction_submissions, transaction_intents, payment_attempts, orders, chain_observations | — | AccessGrantLineage, ApprovalBinding, AuthorizationGate, SenderProofReplay | — | READ |
| API-020 | GET /v1/operations/{operationId} | operation_owner_or_capability | operations | — | AccessGrantLineage, ApprovalBinding, AuthorizationGate, ProtectedObject, ScopedCapability, SenderProofReplay | — | READ |
| API-021 | POST /v1/stores | account | accounts | stores, memberships | — | — | STORE_CONFIGURATION |
| API-022 | GET /v1/stores | account | memberships, stores | — | — | — | READ |
| API-023 | POST /v1/stores/{storeId}/terminal-sessions | store_terminal_manage | memberships, stores | — | TrustPrincipal, TrustGrantRevision | ScopedCapability | SECURITY_STORE |
| API-024 | POST /v1/stores/{storeId}/memberships | store_owner | memberships, accounts, stores | memberships | — | — | STORE_CONFIGURATION |
| API-025 | DELETE /v1/stores/{storeId}/memberships/{membershipId} | store_owner | memberships, stores | memberships | — | — | STORE_CONFIGURATION |
| API-026 | GET /v1/stores/{storeId}/menu | store_customer_session | stores, menu_items | — | — | — | READ |
| API-027 | PUT /v1/stores/{storeId}/menu-items/{itemId} | store_menu_manage | memberships, stores, menu_items | menu_items | — | — | STORE_CONFIGURATION |
| API-028 | POST /v1/stores/{storeId}/recipient-changes | store_funds_manage_recent_auth | memberships, wallets, wallet_bindings, recipient_configurations | recipient_configurations | — | — | STORE_CONFIGURATION |
| API-029 | POST /v1/stores/{storeId}/orders | store_customer_session | stores, menu_items, recipient_configurations | orders, order_lines | ScopedCapability | ScopedCapability | TX-03 |
| API-030 | GET /v1/orders/{orderId} | order_read_or_capability | orders, order_lines, payment_attempts, payment_allocations | — | — | — | READ |
| API-031 | POST /v1/orders/{orderId}/cancel | order_cancel | orders, payment_attempts, transaction_submissions | orders | — | — | TX-03 |
| API-032 | POST /v1/orders/{orderId}/payment-attempts | order_payment_capability | orders, assets, payment_attempts | payment_attempts | ScopedCapability | ScopedCapability | QUOTE_CREATION |
| API-033 | POST /v1/payment-attempts/{attemptId}/device-sessions | terminal_attempt_capability | orders, payment_attempts, devices, protocol_profiles | — | ScopedCapability | ScopedCapability | SECURITY_STORE |
| API-034 | POST /v1/payment-attempts/{attemptId}/intents | attempt_device_capability | orders, payment_attempts, assets, protocol_profiles | operations, payment_attempts, transaction_intents | ApprovalBinding, AuthorizationGate, ScopedCapability | ApprovalBinding, ScopedCapability | INTENT_CREATION |
| API-035 | GET /v1/payment-attempts/{attemptId} | attempt_read_capability | payment_attempts, orders, payment_allocations, chain_observations | — | ScopedCapability | — | READ |
| API-036 | POST /v1/orders/{orderId}/refunds | store_refund_request | orders, payment_allocations, refund_balances, refunds | refund_balances, refunds, refund_reservations | — | — | TX-05 |
| API-037 | POST /v1/refunds/{refundId}/authorize | store_funds_manage_recent_auth | memberships, refunds, refund_balances, refund_reservations, wallet_bindings, protocol_profiles, orders | operations, refunds, transaction_intents | ApprovalBinding, AuthorizationGate | ApprovalBinding | TX-05 |
| API-038 | GET /v1/refunds/{refundId} | refund_read | refunds, refund_balances, orders, transaction_submissions | — | — | — | READ |
| API-039 | GET /v1/stores/{storeId}/sales | store_sales_read | memberships, orders, payment_allocations, refunds, settlements | — | — | — | READ |
| API-040 | POST /v1/stores/{storeId}/settlements | store_settlement_manage | orders, payment_allocations, refunds, settlements | settlements, settlement_revisions | — | — | TX-08 |
| API-041 | POST /v1/settlements/{settlementId}/corrections | store_settlement_manage | settlements, settlement_revisions | settlements, settlement_revisions | — | — | TX-08 |
| API-042 | GET /v1/me/receipts | account | orders, payment_allocations, refunds | — | ReceiptEligibility, ReceiptOwnership | — | READ |
| API-043 | GET /v1/me/passport | account | benefit_entries, benefit_redemptions | — | ReceiptOwnership, PendingBenefitEntitlement | — | READ |
| API-044 | POST /v1/benefit-redemptions | benefit_owner | benefit_entries, benefit_redemptions, orders | benefit_entries, benefit_redemptions | — | — | TX-06 |
| API-045 | POST /v1/rentals | rental_operator | devices, rentals, accounts | rentals, devices | TrustPrincipal, TrustGrantRevision | — | TX-02 |
| API-046 | POST /v1/rentals/{rentalId}/return-checks | rental_owner_or_operator | rentals, device_bindings, wallets, transaction_submissions, return_checks | return_checks, reset_clearances | ReceiptEligibility, ReceiptOwnership | — | TX-07 |
| API-047 | POST /v1/rentals/{rentalId}/complete-return | rental_owner_and_clearance | rentals, device_bindings, return_checks, reset_clearances, devices | rentals, device_bindings, reset_clearances, return_checks, devices | ReceiptEligibility, ReceiptOwnership, ScopedCapability | ScopedCapability | TX-07 |
| API-048 | GET /v1/devices/{deviceId}/firmware-offers | device_owner | devices, device_bindings, firmware_releases, protocol_profiles | — | — | — | READ |
| API-049 | POST /v1/firmware-releases | firmware_release_operator | protocol_profiles | firmware_releases | TrustPrincipal, TrustGrantRevision | — | TX-11 |
| API-050 | POST /v1/firmware-releases/{releaseId}/withdraw | firmware_release_operator | firmware_releases | firmware_releases | TrustPrincipal, TrustGrantRevision | — | TX-11 |
| API-051 | POST /v1/recordings | account | accounts, protocol_profiles | recordings | — | — | TX-09 |
| API-052 | POST /v1/recordings/{recordingId}/processing | recording_owner_with_consent | recordings, consents | operations, processing_jobs | — | — | TX-09 |
| API-053 | GET /v1/recordings/{recordingId}/processing | recording_owner | recordings, processing_jobs | — | ProtectedObject, DeletionTombstone | — | READ |
| API-054 | DELETE /v1/recordings/{recordingId} | recording_owner | recordings, processing_jobs, data_lineage | recordings, processing_jobs, privacy_requests, operations | — | — | TX-09 |
| API-055 | POST /v1/smart-account-plans | wallet_use | wallets, wallet_bindings, protocol_profiles, memberships, stores | operations | ProtectedObject | ProtectedObject, ObjectPayload | TX-13 |
| API-056 | POST /v1/smart-account-intents | wallet_sign_authorized | operations, wallets, wallet_bindings, protocol_profiles, memberships, stores | transaction_intents | — | — | INTENT_CREATION |
| API-057 | POST /v1/market-quotes | account | market_quotes, assets, chain_projections, protocol_profiles | market_quotes | ProtectedObject | ProtectedObject, ObjectPayload | QUOTE_CREATION |
| API-058 | POST /v1/market-intents | wallet_use | market_quotes, wallets, wallet_bindings, protocol_profiles, memberships, stores | transaction_intents | — | — | INTENT_CREATION |
| API-059 | GET /v1/wallets/{walletId}/liquidity-positions | wallet_read | wallets, wallet_bindings, chain_projections, memberships, stores | — | — | — | READ |
| API-060 | GET /v1/perpetual/markets | account | chain_projections, protocol_profiles | — | — | — | READ |
| API-061 | POST /v1/perpetual/intents | wallet_use | wallets, wallet_bindings, chain_projections, protocol_profiles, memberships, stores | transaction_intents | — | — | INTENT_CREATION |
| API-062 | GET /v1/wallets/{walletId}/perpetual-positions | wallet_read | wallets, wallet_bindings, chain_projections, memberships, stores | — | — | — | READ |
| API-063 | GET /v1/offerings | account | chain_projections, protocol_profiles | — | — | — | READ |
| API-064 | POST /v1/offerings/{offeringId}/intents | offering_actor | wallets, wallet_bindings, credential_metadata, chain_projections, protocol_profiles | transaction_intents | — | — | INTENT_CREATION |
| API-065 | POST /v1/credentials | credential_issuer | accounts, protocol_profiles | credential_metadata, credential_status_history | TrustPrincipal, TrustGrantRevision | — | TX-14 |
| API-066 | GET /v1/me/credentials | account | credential_metadata, credential_status_history | — | — | — | READ |
| API-067 | POST /v1/credential-presentations | credential_owner | credential_metadata, credential_status_history, protocol_profiles | operations | TrustPrincipal, TrustGrantRevision | — | TX-14 |
| API-068 | POST /v1/credential-verifications | credential_verifier | credential_metadata, credential_status_history, protocol_profiles | operations | TrustPrincipal, TrustGrantRevision | — | TX-14 |
| API-069 | POST /v1/credentials/{credentialId}/revoke | credential_issuer | credential_metadata, credential_status_history | credential_metadata, credential_status_history | TrustPrincipal, TrustGrantRevision | — | TX-14 |
| API-070 | POST /v1/paid-resource-requests | wallet_use | wallets, wallet_bindings, protocol_profiles, memberships, stores | paid_resource_requests | — | — | TX-15 |
| API-071 | POST /v1/paid-resource-requests/{requestId}/proofs | request_owner | paid_resource_requests, protocol_profiles | operations | — | — | TX-15 |
| API-072 | GET /v1/paid-resource-requests/{requestId}/result | request_owner | paid_resource_requests, entitlements | — | — | — | READ |
| API-073 | GET /v1/places | account | places, store_place_links | — | — | — | READ |
| API-074 | POST /v1/places/{placeId}/reviews | account | places, orders, payment_allocations | reviews | — | — | TX-10 |
| API-075 | PATCH /v1/reviews/{reviewId} | review_owner | reviews | reviews | — | — | TX-10 |
| API-076 | DELETE /v1/reviews/{reviewId} | review_owner | reviews, data_lineage | reviews, privacy_requests, operations | — | — | TX-09 |
| API-077 | POST /v1/trips | account | accounts, consents | trips | — | — | TX-10 |
| API-078 | POST /v1/trips/{tripId}/visits | trip_owner | trips, places, consents | travel_evidence | — | — | TX-10 |
| API-079 | GET /v1/trips/{tripId}/timeline | trip_owner | trips, travel_evidence | — | — | — | READ |
| API-080 | POST /v1/itineraries | account | trips, consents, travel_evidence, places, reviews | operations, processing_jobs | — | — | TX-09 |
| API-081 | PATCH /v1/itineraries/{itineraryId} | itinerary_owner | itineraries, trips, places, consents | itineraries | ProtectedObject, DeletionTombstone | — | TX-10 |
| API-082 | POST /v1/challenges/{challengeId}/participations | account | trips, protocol_profiles | participations | — | — | TX-10 |
| API-083 | POST /v1/participations/{participationId}/evidence | participant | participations, travel_evidence, challenge_entries | challenge_entries, participations | — | — | TX-10 |
| API-084 | PUT /v1/me/consents | account | consents | consents | — | — | TX-09 |
| API-085 | POST /v1/me/data-requests | account_recent_auth | accounts, consents, data_lineage | privacy_requests, operations | ProtectedObject, DeletionTombstone | — | TX-09 |
| API-086 | GET /v1/ops/exceptions | ops_exception_read | payment_allocations, payment_attempts, refunds, operations | — | TrustPrincipal, TrustGrantRevision | — | READ |
| API-087 | POST /v1/ops/reconciliation-jobs | ops_reconcile | payment_allocations, chain_observations, operations | operations | TrustPrincipal, TrustGrantRevision | — | TX-12 |
| API-088 | GET /v1/ops/audit-events | ops_audit_read | audit_events | — | TrustPrincipal, TrustGrantRevision | — | READ |
| API-089 | GET /v1/service-health | service_health_read | operations, chain_projections | — | TrustPrincipal, TrustGrantRevision | — | READ |
| API-090 | GET /v1/devices | device_owner | device_bindings, devices, rentals | — | — | — | READ |
| API-091 | GET /v1/rentals/{rentalId} | rental_owner_or_operator | rentals, return_checks, reset_clearances, device_bindings | — | — | — | READ |
| API-092 | GET /v1/stores/{storeId}/orders | store_order_read | memberships, orders, payment_attempts | — | — | — | READ |
| API-093 | GET /v1/stores/{storeId}/settlements | store_sales_read | memberships, settlements, settlement_revisions | — | — | — | READ |
| API-094 | GET /v1/recordings | account | recordings | — | — | — | READ |
| API-095 | GET /v1/itineraries/{itineraryId} | itinerary_owner | itineraries, trips | — | — | — | READ |
| API-096 | GET /v1/participations/{participationId} | participant | participations, challenge_entries | — | — | — | READ |
| API-097 | GET /v1/trips | account | trips | — | — | — | READ |
| API-098 | POST /v1/wallet-bindings | device_owner | enrollments, device_bindings, wallets, wallet_bindings | wallets, wallet_bindings, device_bindings | — | — | TX-02 |
| API-099 | GET /v1/ops/stores | ops_store_read | stores, memberships | — | TrustPrincipal, TrustGrantRevision | — | READ |
| API-100 | GET /v1/ops/devices | ops_device_read | devices, rentals, device_bindings | — | TrustPrincipal, TrustGrantRevision | — | READ |
| API-101 | GET /v1/firmware-releases/{releaseId} | release_read | firmware_releases, protocol_profiles | — | — | — | READ |
| API-102 | GET /v1/wallets/{walletId}/offering-holdings | wallet_read | wallets, wallet_bindings, chain_projections, credential_metadata, memberships, stores | — | — | — | READ |
| API-103 | POST /v1/auth/flows | auth_flow_bootstrap | accounts, auth_sessions, protocol_profiles | — | — | AuthFlow | SECURITY_STORE |
| API-104 | POST /v1/receipt-claims/challenges | receipt_claim_eligible | accounts, auth_sessions, orders, payment_allocations, wallets, rentals, device_bindings | — | ReceiptEligibility, ReceiptOwnership | ReceiptClaim | SECURITY_STORE |
| API-105 | POST /v1/receipt-claims/{claimId}/confirm | receipt_claim_confirm | accounts, auth_sessions, orders, payment_allocations | outbox | ReceiptEligibility, ReceiptClaim, ReceiptOwnership, PendingBenefitEntitlement | ReceiptClaim, ReceiptOwnership | RECEIPT_CLAIM |
| API-106 | GET /v1/receipt-claims/{claimId} | receipt_claim_owner | accounts, auth_sessions | — | ReceiptClaim, ReceiptOwnership | — | READ |
| API-107 | GET /v1/protected-results/{resultId} | protected_result_read | accounts, auth_sessions, operations, consents, wallet_bindings, memberships, stores | — | ProtectedObject, ObjectPayload, DeletionTombstone | — | READ |
| API-108 | POST /v1/approval-contexts/{approvalContextId}/access-challenges | approval_access_challenge | operations, transaction_intents | — | AccessChallenge, AccessGrantLineage, ApprovalBinding, AuthorizationGate, ProtectedIssuanceResponse, SenderProofReplay | AccessChallenge | SECURITY_STORE |
| API-109 | POST /v1/approval-contexts/{approvalContextId}/access-grants | approval_access_issue | operations, transaction_intents | — | AccessChallenge, AccessGrantLineage, ApprovalBinding, AuthorizationGate, ProtectedIssuanceResponse, SenderProofReplay | AccessChallenge, AccessGrantLineage, ProtectedIssuanceResponse, SenderProofReplay | SECURITY_STORE |
| API-110 | POST /v1/approval-contexts/{approvalContextId}/access-grants/{grantId}/revoke | approval_access_revoke | operations, transaction_intents | — | AccessChallenge, AccessGrantLineage, ApprovalBinding, AuthorizationGate, ProtectedIssuanceResponse, SenderProofReplay | AccessGrantLineage, AuthorizationGate | SECURITY_STORE |

## 반영 조건

- **API-001**: provider exchange before identity commit; never hold DB locks over provider network call
- **API-002**: lock target account and unique provider/subject; recheck recent authentication
- **API-003**: revoke only selected own session; invalidate security cache after commit
- **API-004**: block access and enqueue cleanup; do not delete wallet assets or keys from account row operation
- **API-005**: filter to current account; no provider credentials
- **API-006**: atomic refresh-family rotation/replay policy in protected session store
- **API-007**: Approval baseline: 현재 본인 계정의 지갑 목록. cursor도 계정/context에 결합
- **API-008**: Approval baseline: 해당 store_asset_read; 일반 매장 소속은 부족
- **API-009**: Approval baseline: wallet_read + 명시 context/binding; 서명권과 별개
- **API-010**: new challenge bound to account/device/rental; no ownership grant yet
- **API-011**: consume unexpired challenge and bind proven device owner atomically
- **API-012**: rental return preconditions; unbinding is not proof of physical key erasure
- **API-013**: owner-filtered metadata; reported state not live device proof
- **API-014**: enqueue MPC provisioning; participant worker writes wallets only after completion
- **API-015**: Approval baseline: 현재 개인 또는 매장 Cloud signer + 원 context + approvalProof. AC 자격은 서명권 아님; child operation and original parent/context/signer/epoch association committed before external MPC rounds
- **API-016**: recovery verifier before enrollment; recovery worker applies participant changes
- **API-017**: Approval baseline: wallet_use + 현재 개인 selection. 서버가 personal source 생성; atomically create immutable intent/snapshot + parent operations row + ApprovalBinding ancestry + idempotency linkage; API037 includes refund authorization revisions
- **API-018**: Approval baseline: submit_exact + 원 source/provenance/current epoch + SS-T03/04 gate
- **API-019**: Approval baseline: 기존 transaction_read 또는 원 grant context→dispatch→transactionRef 정확한 관계 + read_progress
- **API-020**: Approval baseline: 기존 operation ACL 보존; approval은 현재 source ACL 또는 exact grant, projection별 검사
- **API-021**: create store and initial owner membership together
- **API-022**: current account memberships only
- **API-023**: mint terminal session only from authorized store management session
- **API-024**: lock store membership changes; role escalation and last owner guard
- **API-025**: serialize owner changes on store; never remove last active owner
- **API-026**: public/customer projection for selected store; hide management-only data
- **API-027**: store-scoped revision check; old order snapshots unchanged
- **API-028**: recent funds authority + address control proof; lock store configuration
- **API-029**: server-side menu/recipient snapshots and order capability after commit
- **API-030**: order owner/store role or exact order capability; guest projection strips account details
- **API-031**: lock order and recheck payment phase; cancellation does not cancel signed tx
- **API-032**: server quote bound to order and recipient; pairing challenge in protected security store
- **API-033**: verify peer then mint attempt+device+terminal scoped temporary session
- **API-034**: Approval baseline: 원 terminal/attempt/session과 검증 identify. guest 계정 없음; atomically create immutable intent/snapshot + parent operations row + ApprovalBinding ancestry + idempotency linkage; API037 includes refund authorization revisions
- **API-035**: exact attempt scope; submit status and chain observation kept separate
- **API-036**: lock refund balance; reserve positive amount in matching order/asset
- **API-037**: Approval baseline: 정확한 매장 자금 승인권/예약/funding revision; 지정 signer와 업무 승인자 분리; atomically create immutable intent/snapshot + parent operations row + ApprovalBinding ancestry + idempotency linkage; API037 includes refund authorization revisions
- **API-038**: store refund role or linked customer receipt; redact merchant signer internals
- **API-039**: store/date/asset filtering; report projection revision
- **API-040**: serialize period close; source revision and unresolved differences checked
- **API-041**: append reasoned correction; preserve original close snapshot
- **API-042**: verified account receipt association; address alone does not reveal private history
- **API-043**: account-owned passport; policy version and revision
- **API-044**: benefit owner and order applicability; single active consumption
- **API-045**: authorized rental operator; lock device and enforce active rental uniqueness
- **API-046**: operator can inspect; owner approval and verified checks required for actionable clearance; expose pre-reset receipt link state to owner only; exported ticket alone is not linked. Do not auto-link from current rental owner.
- **API-047**: owner+clearance+fresh reset proof; atomic revoke/returned/reavailability transition; expose pre-reset receipt link state to owner only; exported ticket alone is not linked. Do not auto-link from current rental owner.
- **API-048**: owner scope and compatible offers only
- **API-049**: release operator permission; manifest verification outside lock then version uniqueness
- **API-050**: recheck revision and publish withdrawal event; no implied device rollback
- **API-051**: register owner metadata; does not upload raw audio
- **API-052**: source ownership+purpose consent; enqueue only, provider call after commit
- **API-053**: owner only; temporary result access grants issued separately
- **API-054**: block access and pending result publication before asynchronous erasure
- **API-055**: persist transition-plan reference in typed operation result; no account onchain transition yet
- **API-056**: plan ownership and version checks; signer approval still separate
- **API-057**: external quote read then persist immutable action-specific snapshot
- **API-058**: validate quote expiry/action/slippage and selected wallet
- **API-059**: wallet owner scope plus chain/block freshness
- **API-060**: market public projection for signed-in account; no other users positions
- **API-061**: market/price/risk validation; no direct position mutation from app claim
- **API-062**: selected wallet account scope; negative pnl/funding preserved
- **API-063**: offering public metadata only, eligibility rules not private claims
- **API-064**: offering action authority and credential validity; owner approval at signer stage
- **API-065**: issuer registry and subject proof; private content protected store, DB reference only
- **API-066**: own credential metadata, no issuer-wide disclosure
- **API-067**: holder consent/challenge; proof adapter after source/visibility checks
- **API-068**: registered verifier, purpose-bound challenge, minimal disclosure
- **API-069**: same issuer authority; append status version and revocation event
- **API-070**: resource+price+wallet snapshot; response is payment requirement, not paid receipt; resolve selected wallet active personal binding or exact-store membership with action permission, and supported payment profile before creating request
- **API-071**: proof validation enqueued; only verified worker can allocate entitlement
- **API-072**: same request owner; payment/delivery status separate
- **API-073**: external place fetch is cache layer; location not implicitly saved as visit
- **API-074**: review owner from auth; purchase ownership/provenance verified, otherwise no verified badge
- **API-075**: same author and expected revision; purchase badge not client-editable
- **API-076**: block visibility and propagate removal to derived recommendations
- **API-077**: own account and current consent profile
- **API-078**: own trip and source verifier; do not equate location with confirmed purchase
- **API-079**: own trip timeline; source-specific redaction
- **API-080**: enqueue AI work with owner/purpose revision; resulting itinerary saved by worker
- **API-081**: same owner, revision and route validation
- **API-082**: own trip and applicable challenge rules; no reward on enrollment alone
- **API-083**: same owner/trip evidence, valid revision, unique reward key
- **API-084**: serialize owner/purpose changes; supersede old version then insert new version
- **API-085**: recent authentication and own data scope; enqueue provider/storage-specific results
- **API-086**: ops exception projection, minimal customer data; no raw keys/proofs
- **API-087**: operator requests source reread only; reconciliation worker makes verified updates
- **API-088**: ops audit entitlement and permitted subject scope
- **API-089**: health projection excludes secrets, user content and internal credentials
- **API-090**: own active bindings only
- **API-091**: rental owner or allowed ops projection; operator does not receive owner reset capability
- **API-092**: store order-read role; cursor scoped to store
- **API-093**: store sales-read role
- **API-094**: owner metadata only; merge local recordings in app without uploading them
- **API-095**: same owner and current visibility; revoked source status visible
- **API-096**: same participant; no other users evidence payloads
- **API-097**: own trips; deleted records excluded
- **API-098**: active owner enrollment+address control; no private key field
- **API-099**: ops store projection with least customer detail
- **API-100**: ops inventory projection; no owner session credentials
- **API-101**: owner compatible release or release operator; package access remains scoped
- **API-102**: own wallet holdings and only own eligibility result
- **API-103**: auth adapter owns bootstrap idempotency and one-time flow; generic DB cache must not retain authorization parameters or tokens
- **API-104**: validate inline eligibilityProof against historical ReceiptEligibility before disclosing challenge; opaque locator/ref alone cannot authorize
- **API-105**: ReceiptOwnership and domain outbox commit atomically in selected ledger adapter; if SQL chosen outbox is same transaction. ReceiptClaim challenge store consumption reconciles by claimId; no distributed atomicity assumed.; emit ownership link event; benefit worker binds existing pending entitlement once, does not issue a second reward
- **API-106**: read only; current principal verified even after idempotent confirmation
- **API-107**: no domain mutation; actual read ACL and digest verified, gateway rechecks before streaming
- **API-108**: Approval baseline: 현재 원 source 접근권 + sender binding; replacement predecessor 독립 확인
- **API-109**: Approval baseline: challenge/proof 검증 후 현재 권한 재검사와 발급/교체 CAS
- **API-110**: Approval baseline: 해당 grant 철회권; 타인의 token/result 공개 권한 아님

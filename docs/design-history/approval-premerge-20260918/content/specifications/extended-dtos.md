# 확장 99개 API 요청·응답 타입

설계 제안. [기본 규칙](extended-contracts.md) · [보안 통합과 binary 전달](security-api-integration.md). `?` 타입은 null 허용, 선택은 필드 생략 허용. path 변수는 body/query와 분리한다. ProfileInput 내부는 선정한 엄격한 profile 검증기가 필요하다.

## API-001 · POST /v1/auth/exchanges

권한: public_auth_flow. 정상 응답: 200. 작업: AUTH-02, AUTH-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| provider | Provider | 필수 |
| authFlowId | Id | 필수 |
| providerProof | ProfileInput | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| session | SessionView |
| account | AccountView |

개별 오류: AUTH_PROOF_INVALID. 인증/권한/형태/멱등성 공통 오류도 적용.

- profileId must match server AuthFlow provider/client/OS/purpose; validate exact registered proof schema, initiator, callback and expiry before consuming flow
- server-selected redirect and provider identity; no arbitrary redirect from exchange input

## API-002 · POST /v1/auth/links

권한: account_recent_auth. 정상 응답: 200. 작업: AUTH-04.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| provider | Provider | 필수 |
| authFlowId | Id | 필수 |
| providerProof | ProfileInput | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| linkedProviders | ProviderLink[] |

개별 오류: ACCOUNT_LINK_CONFLICT. 인증/권한/형태/멱등성 공통 오류도 적용.

- profileId must match server AuthFlow provider/client/OS/purpose; validate exact registered proof schema, initiator, callback and expiry before consuming flow
- server-selected redirect and provider identity; no arbitrary redirect from exchange input

## API-003 · POST /v1/auth/logout

권한: account. 정상 응답: 200. 작업: AUTH-04.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| sessionId | Id | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| revoked | Bool |

개별 오류: AUTH_REQUIRED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-004 · POST /v1/account-deletions

권한: account_recent_auth. 정상 응답: 202. 작업: AUTH-04.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| scope | Text[] | 필수 |
| acknowledgedWalletRecovery | Bool | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| operation | Operation |

개별 오류: RECOVERY_REQUIRED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-005 · GET /v1/me

권한: account. 정상 응답: 200. 작업: AUTH-01.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| account | AccountView |
| memberships | MembershipView[] |
| linkedProviders | ProviderLink[] |

개별 오류: AUTH_REQUIRED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-006 · POST /v1/auth/refresh

권한: refresh_session. 정상 응답: 200. 작업: AUTH-04.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| refreshToken | Text | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| session | SessionView |

개별 오류: SESSION_REVOKED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-007 · GET /v1/wallets

권한: account. 정상 응답: 200. 작업: APP-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| wallets | WalletView[] |

개별 오류: AUTH_REQUIRED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-008 · GET /v1/stores/{storeId}/wallets

권한: store_asset_read. 정상 응답: 200. 작업: APP-02, SHOP-06.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| wallets | WalletView[] |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-009 · GET /v1/wallets/{walletId}/balances

권한: wallet_read. 정상 응답: 200. 작업: APP-02, TOKEN-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| assets | BalanceView[] |
| observedAt | Time |
| blockRef | BlockRef? |

개별 오류: CHAIN_UNAVAILABLE. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-010 · POST /v1/device-enrollments

권한: account. 정상 응답: 201. 작업: HW-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| deviceId | Id | 필수 |
| deviceProof | ProfileInput | 필수 |
| rentalId | Id | 선택 |

| 응답 data 필드 | 타입 |
|---|---|
| enrollmentId | Id |
| enrollmentChallenge | ChallengeView |

개별 오류: DEVICE_ALREADY_ASSIGNED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-011 · POST /v1/device-enrollments/{enrollmentId}/confirm

권한: account. 정상 응답: 201. 작업: HW-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| deviceProof | ProfileInput | 필수 |
| challengeResponse | ProfileInput | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| deviceBinding | DeviceBindingView |

개별 오류: DEVICE_PROOF_INVALID. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-012 · DELETE /v1/devices/{deviceId}/binding

권한: device_owner. 정상 응답: 202. 작업: HW-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| expectedRevision | Revision | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| operation | Operation |

개별 오류: RETURN_CHECK_REQUIRED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-013 · GET /v1/devices/{deviceId}

권한: device_owner. 정상 응답: 200. 작업: HW-01, FIND-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| deviceMetadata | DeviceMetadata |
| lastReportedAt | Time? |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-014 · POST /v1/cloud-wallets

권한: account_recent_auth. 정상 응답: 202. 작업: MPC-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| createRequestId | Id | 필수 |
| participantEnrollmentProof | ProfileInput | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| operation | Operation |

개별 오류: PARTICIPANT_UNAVAILABLE. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-015 · POST /v1/wallets/{walletId}/signing-sessions

권한: wallet_sign_authorized. 정상 응답: 202. 작업: MPC-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| intentId | Id | 필수 |
| approvalProof | ProfileInput | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| operation | Operation |

개별 오류: APPROVAL_REQUIRED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-016 · POST /v1/wallets/{walletId}/recoveries

권한: wallet_recovery_proof. 정상 응답: 202. 작업: MPC-04.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| recoveryProof | ProfileInput | 필수 |
| newParticipantProof | ProfileInput | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| operation | Operation |

개별 오류: RECOVERY_PROOF_INVALID. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-017 · POST /v1/transaction-intents

권한: wallet_use. 정상 응답: 201. 작업: PAY-01, APP-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| walletId | Id | 필수 |
| chainId | ChainId | 필수 |
| action | Text | 필수 |
| actionInput | ProfileInput | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| intent | IntentView |
| review | ReviewView |
| expiry | Time |

개별 오류: UNSUPPORTED_ACTION. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-019 · GET /v1/transactions/{transactionRef}

권한: transaction_read. 정상 응답: 200. 작업: INDEX-05.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| execution | ExecutionState |
| canonicality | BlockCanonicality |
| confirmation | ConfirmationState |
| observedAt | Time |

개별 오류: NOT_FOUND. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-020 · GET /v1/operations/{operationId}

권한: operation_owner_or_capability. 정상 응답: 202. 작업: PAY-02, REC-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| operation | Operation |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-021 · POST /v1/stores

권한: account. 정상 응답: 201. 작업: SHOP-01.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| displayName | Text | 필수 |
| location | Location | 필수 |
| timezone | Text | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| store | StoreView |

개별 오류: VALIDATION_FAILED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-022 · GET /v1/stores

권한: account. 정상 응답: 200. 작업: SHOP-01.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| memberships | MembershipView[] |

개별 오류: AUTH_REQUIRED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-023 · POST /v1/stores/{storeId}/terminal-sessions

권한: store_terminal_manage. 정상 응답: 201. 작업: SHOP-01.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| terminalId | Id | 필수 |
| mode | Mode | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| terminalSession | TerminalSession |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-024 · POST /v1/stores/{storeId}/memberships

권한: store_owner. 정상 응답: 201. 작업: SHOP-01.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| accountId | Id | 필수 |
| role | Role | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| membership | MembershipView |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-025 · DELETE /v1/stores/{storeId}/memberships/{membershipId}

권한: store_owner. 정상 응답: 200. 작업: SHOP-01.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| expectedRevision | Revision | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| revoked | Bool |

개별 오류: LAST_OWNER. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-026 · GET /v1/stores/{storeId}/menu

권한: store_customer_session. 정상 응답: 200. 작업: SHOP-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| menu | MenuItem[] |
| revision | Revision |

개별 오류: STORE_UNAVAILABLE. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-027 · PUT /v1/stores/{storeId}/menu-items/{itemId}

권한: store_menu_manage. 정상 응답: 200. 작업: SHOP-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| name | Text | 필수 |
| options | MenuOption[] | 필수 |
| price | Price | 필수 |
| availability | Availability | 필수 |
| expectedRevision | Revision | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| menuItem | MenuItem |

개별 오류: REVISION_CONFLICT. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-028 · POST /v1/stores/{storeId}/recipient-changes

권한: store_funds_manage_recent_auth. 정상 응답: 201. 작업: SHOP-06.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| walletRef | Id | 필수 |
| ownershipProof | ProfileInput | 필수 |
| expectedRevision | Revision | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| recipientConfiguration | RecipientConfig |

개별 오류: ADDRESS_PROOF_INVALID. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-030 · GET /v1/orders/{orderId}

권한: order_read_or_capability. 정상 응답: 200. 작업: SHOP-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| order | OrderView |
| paymentSummary | PaymentSummary |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-031 · POST /v1/orders/{orderId}/cancel

권한: order_cancel. 정상 응답: 201. 작업: SHOP-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| reason | Text | 필수 |
| expectedRevision | Revision | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| order | OrderView |

개별 오류: PAYMENT_IN_PROGRESS. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-033 · POST /v1/payment-attempts/{attemptId}/device-sessions

권한: terminal_attempt_capability. 정상 응답: 201. 작업: HW-03, SHOP-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| deviceProof | ProfileInput | 필수 |
| challengeResponse | ProfileInput | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| restrictedDeviceSession | RestrictedSession |

개별 오류: DEVICE_PROOF_INVALID. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-035 · GET /v1/payment-attempts/{attemptId}

권한: attempt_read_capability. 정상 응답: 200. 작업: PAY-04.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| attempt | AttemptView |
| observation | ChainObservation? |
| exception | ReconciliationException? |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-038 · GET /v1/refunds/{refundId}

권한: refund_read. 정상 응답: 200. 작업: SHOP-04.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| refund | RefundView |
| transactionRef | Id? |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-039 · GET /v1/stores/{storeId}/sales

권한: store_sales_read. 정상 응답: 200. 작업: SHOP-05.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| period | Period | 필수 |
| assetId | Id | 선택 |

| 응답 data 필드 | 타입 |
|---|---|
| summary | SalesTotals[] |
| exceptions | ReconciliationException[] |
| asOfRevision | Revision |

개별 오류: VALIDATION_FAILED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-040 · POST /v1/stores/{storeId}/settlements

권한: store_settlement_manage. 정상 응답: 201. 작업: SHOP-05.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| period | Period | 필수 |
| expectedRevision | Revision | 필수 |
| note | Text | 선택 |

| 응답 data 필드 | 타입 |
|---|---|
| settlementRecord | SettlementView |

개별 오류: UNRESOLVED_DIFFERENCE. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-041 · POST /v1/settlements/{settlementId}/corrections

권한: store_settlement_manage. 정상 응답: 201. 작업: SHOP-05.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| reason | Text | 필수 |
| expectedRevision | Revision | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| correctedRecord | SettlementView |

개별 오류: REVISION_CONFLICT. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-042 · GET /v1/me/receipts

권한: account. 정상 응답: 200. 작업: APP-04.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| cursor | Text | 선택 |
| limit | Limit | 선택 |

| 응답 data 필드 | 타입 |
|---|---|
| receipts | ReceiptView[] |
| nextCursor | Text? |

개별 오류: AUTH_REQUIRED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-043 · GET /v1/me/passport

권한: account. 정상 응답: 200. 작업: STAMP-01, STAMP-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| stamps | StampView[] |
| benefits | BenefitView[] |
| revision | Revision |
| pendingEntitlements | PendingEntitlementView[] |

개별 오류: AUTH_REQUIRED. 인증/권한/형태/멱등성 공통 오류도 적용.

- only pending entitlements bound to current verified ReceiptOwnership are exposed; pending units are not included in usable stamps/benefits
- same entitlement source cannot appear in pending and materialized view at one consistent ledger revision

## API-044 · POST /v1/benefit-redemptions

권한: benefit_owner. 정상 응답: 201. 작업: STAMP-01.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| benefitId | Id | 필수 |
| orderId | Id | 필수 |
| expectedRevision | Revision | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| redemption | RedemptionView |

개별 오류: BENEFIT_UNAVAILABLE. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-045 · POST /v1/rentals

권한: rental_operator. 정상 응답: 201. 작업: STAMP-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| deviceId | Id | 필수 |
| accountId | Id | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| rental | RentalView |

개별 오류: DEVICE_UNAVAILABLE. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-048 · GET /v1/devices/{deviceId}/firmware-offers

권한: device_owner. 정상 응답: 200. 작업: OTA-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| appVersion | Text | 필수 |
| protocolVersion | Text | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| offer | ReleaseView? |
| compatibility | CompatibilityView |

개별 오류: UPDATE_BLOCKED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-049 · POST /v1/firmware-releases

권한: firmware_release_operator. 정상 응답: 201. 작업: OTA-02, OPS-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| manifest | SignedReleaseManifest | 필수 |
| packageRef | Id | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| release | ReleaseView |

개별 오류: MANIFEST_INVALID. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-050 · POST /v1/firmware-releases/{releaseId}/withdraw

권한: firmware_release_operator. 정상 응답: 201. 작업: OTA-04, OPS-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| reason | Text | 필수 |
| expectedRevision | Revision | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| release | ReleaseView |

개별 오류: REVISION_CONFLICT. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-051 · POST /v1/recordings

권한: account. 정상 응답: 201. 작업: REC-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| recordingId | Id | 필수 |
| format | RecordingFormat | 필수 |
| completeness | Completeness | 필수 |
| duration | Seconds | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| recordingMetadata | RecordingView |

개별 오류: VALIDATION_FAILED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-052 · POST /v1/recordings/{recordingId}/processing

권한: recording_owner_with_consent. 정상 응답: 202. 작업: REC-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| sourceRef | Id | 필수 |
| consentRevision | Revision | 필수 |
| jobType | JobType | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| operation | Operation |

개별 오류: CONSENT_REQUIRED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-053 · GET /v1/recordings/{recordingId}/processing

권한: recording_owner. 정상 응답: 200. 작업: REC-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| jobs | JobView[] |
| transcriptRef | Id? |
| summaryRef | Id? |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-054 · DELETE /v1/recordings/{recordingId}

권한: recording_owner. 정상 응답: 202. 작업: REC-03, TRIP-04.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| expectedRevision | Revision | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| operation | Operation |

개별 오류: REVISION_CONFLICT. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-055 · POST /v1/smart-account-plans

권한: wallet_use. 정상 응답: 201. 작업: SMART-01, SMART-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| walletId | Id | 필수 |
| selectedModel | Id | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| transitionPlan | TransitionPlan |
| review | ReviewView |

개별 오류: UNSUPPORTED_ACCOUNT_MODEL. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-056 · POST /v1/smart-account-intents

권한: wallet_sign_authorized. 정상 응답: 201. 작업: SMART-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| planId | Id | 필수 |
| action | Text | 필수 |
| actionInput | ProfileInput | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| intent | IntentView |
| review | ReviewView |

개별 오류: ACCOUNT_NOT_READY. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-057 · POST /v1/market-quotes

권한: account. 정상 응답: 201. 작업: DEX-02, FX-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| marketId | Id | 필수 |
| action | MarketAction | 필수 |
| amountIn | Money | 선택 |
| assetIn | Id | 선택 |
| assetOut | Id | 선택 |
| inputs | Money[] | 선택 |
| positionId | Id | 선택 |
| shareAmount | UInt | 선택 |

| 응답 data 필드 | 타입 |
|---|---|
| quote | MarketQuote |
| fees | Money[] |
| minimumOutputs | Money[] |
| expiresAt | Time |

개별 오류: INSUFFICIENT_LIQUIDITY. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-058 · POST /v1/market-intents

권한: wallet_use. 정상 응답: 201. 작업: DEX-03, FX-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| walletId | Id | 필수 |
| quoteId | Id | 필수 |
| slippageChoice | SlippageBps | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| intent | IntentView |
| review | ReviewView |

개별 오류: QUOTE_EXPIRED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-059 · GET /v1/wallets/{walletId}/liquidity-positions

권한: wallet_read. 정상 응답: 200. 작업: DEX-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| positions | LiquidityPosition[] |
| asOfBlock | BlockRef? |

개별 오류: CHAIN_UNAVAILABLE. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-060 · GET /v1/perpetual/markets

권한: account. 정상 응답: 200. 작업: PERP-05.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| markets | PerpMarket[] |
| priceValidity | Validity |

개별 오류: PRICE_STALE. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-061 · POST /v1/perpetual/intents

권한: wallet_use. 정상 응답: 201. 작업: PERP-03, PERP-05.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| walletId | Id | 필수 |
| marketId | Id | 필수 |
| action | Text | 필수 |
| actionInput | ProfileInput | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| intent | IntentView |
| review | ReviewView |

개별 오류: RISK_LIMIT_EXCEEDED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-062 · GET /v1/wallets/{walletId}/perpetual-positions

권한: wallet_read. 정상 응답: 200. 작업: PERP-05.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| positions | PerpPosition[] |
| funding | FundingEntry[] |
| liquidations | LiquidationEntry[] |
| asOfBlock | BlockRef? |

개별 오류: CHAIN_UNAVAILABLE. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-063 · GET /v1/offerings

권한: account. 정상 응답: 200. 작업: STO-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| offerings | Offering[] |
| eligibilityRules | ProfileInput[] |

개별 오류: AUTH_REQUIRED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-064 · POST /v1/offerings/{offeringId}/intents

권한: offering_actor. 정상 응답: 201. 작업: STO-02, STO-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| walletId | Id | 필수 |
| action | Text | 필수 |
| actionInput | ProfileInput | 필수 |
| credentialRef | Id | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| intent | IntentView |
| review | ReviewView |

개별 오류: INELIGIBLE. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-065 · POST /v1/credentials

권한: credential_issuer. 정상 응답: 201. 작업: DID-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| subjectProof | ProfileInput | 필수 |
| type | Text | 필수 |
| claims | ProfileInput | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| credential | CredentialView |

개별 오류: ISSUER_UNAUTHORIZED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-066 · GET /v1/me/credentials

권한: account. 정상 응답: 200. 작업: DID-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| credentials | CredentialView[] |
| status | CredentialStatus[] |

개별 오류: AUTH_REQUIRED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-067 · POST /v1/credential-presentations

권한: credential_owner. 정상 응답: 201. 작업: DID-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| requestRef | Id | 필수 |
| credentialId | Id | 필수 |
| disclosureChoice | ProfileInput | 필수 |
| holderProof | ProfileInput | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| presentation | ProfileInput |

개별 오류: CREDENTIAL_INVALID. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-068 · POST /v1/credential-verifications

권한: credential_verifier. 정상 응답: 200. 작업: DID-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| presentation | ProfileInput | 필수 |
| challengeRef | Id | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| verification | VerificationView |

개별 오류: CREDENTIAL_INVALID. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-069 · POST /v1/credentials/{credentialId}/revoke

권한: credential_issuer. 정상 응답: 200. 작업: DID-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| reason | Text | 필수 |
| expectedRevision | Revision | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| revocation | RevocationView |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-070 · POST /v1/paid-resource-requests

권한: wallet_use. 정상 응답: 201. 작업: X402-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| resourceId | Id | 필수 |
| resourceInput | ProfileInput | 필수 |
| walletId | Id | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| resourceRequest | ResourceRequest |
| paymentRequirement | ProfileInput |

개별 오류: UNSUPPORTED_PAYMENT_SCHEME. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-071 · POST /v1/paid-resource-requests/{requestId}/proofs

권한: request_owner. 정상 응답: 202. 작업: X402-02, X402-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| paymentProof | ProfileInput | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| operation | Operation |
| entitlement | Entitlement? |

개별 오류: PAYMENT_PROOF_INVALID. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-072 · GET /v1/paid-resource-requests/{requestId}/result

권한: request_owner. 정상 응답: 200. 작업: X402-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| deliveryState | DeliveryState |
| resourceRef | Id? |

개별 오류: DELIVERY_PENDING. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-073 · GET /v1/places

권한: account. 정상 응답: 200. 작업: TRIP-01.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| region | Region | 선택 |
| query | Text | 선택 |
| optionalLocation | Location | 선택 |
| cursor | Text | 선택 |

| 응답 data 필드 | 타입 |
|---|---|
| places | PlaceView[] |
| source | SourceView |
| observedAt | Time |
| nextCursor | Text? |

개별 오류: LOCATION_UNAVAILABLE. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-074 · POST /v1/places/{placeId}/reviews

권한: account. 정상 응답: 201. 작업: TRIP-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| rating | Rating | 필수 |
| text | Text | 필수 |
| purchaseRef | Id | 선택 |

| 응답 data 필드 | 타입 |
|---|---|
| review | UserReview |
| provenance | PurchaseProvenance |

개별 오류: PURCHASE_NOT_OWNED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-075 · PATCH /v1/reviews/{reviewId}

권한: review_owner. 정상 응답: 200. 작업: TRIP-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| text | Text | 선택 |
| rating | Rating | 선택 |
| expectedRevision | Revision | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| review | UserReview |

개별 오류: REVISION_CONFLICT. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-076 · DELETE /v1/reviews/{reviewId}

권한: review_owner. 정상 응답: 202. 작업: TRIP-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| expectedRevision | Revision | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| operation | Operation |

개별 오류: REVISION_CONFLICT. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-077 · POST /v1/trips

권한: account. 정상 응답: 201. 작업: TRIP-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| region | Region | 필수 |
| locale | Text | 필수 |
| timezone | Text | 필수 |
| consentRevision | Revision | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| trip | TripView |

개별 오류: CONSENT_REQUIRED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-078 · POST /v1/trips/{tripId}/visits

권한: trip_owner. 정상 응답: 201. 작업: TRIP-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| placeId | Id | 필수 |
| evidence | ProfileInput | 필수 |
| sourceType | SourceType | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| visit | VisitView |
| validity | Validity |

개별 오류: EVIDENCE_INVALID. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-079 · GET /v1/trips/{tripId}/timeline

권한: trip_owner. 정상 응답: 200. 작업: TRIP-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| cursor | Text | 선택 |

| 응답 data 필드 | 타입 |
|---|---|
| entries | TimelineEntry[] |
| nextCursor | Text? |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-080 · POST /v1/itineraries

권한: account. 정상 응답: 202. 작업: AI-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| tripId | Id | 필수 |
| constraints | RouteConstraints | 필수 |
| consentRevision | Revision | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| operation | Operation |

개별 오류: INSUFFICIENT_DATA. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-081 · PATCH /v1/itineraries/{itineraryId}

권한: itinerary_owner. 정상 응답: 200. 작업: AI-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| stops | RouteStop[] | 필수 |
| constraints | RouteConstraints | 필수 |
| expectedRevision | Revision | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| itinerary | ItineraryView |
| validation | RouteValidation |

개별 오류: ROUTE_INVALID. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-082 · POST /v1/challenges/{challengeId}/participations

권한: account. 정상 응답: 201. 작업: AI-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| tripId | Id | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| participation | ParticipationView |

개별 오류: ALREADY_PARTICIPATING. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-083 · POST /v1/participations/{participationId}/evidence

권한: participant. 정상 응답: 201. 작업: AI-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| stepId | Id | 필수 |
| evidenceRef | Id | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| progress | ProgressView |
| stamp | StampView? |

개별 오류: EVIDENCE_INVALID. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-084 · PUT /v1/me/consents

권한: account. 정상 응답: 200. 작업: TRIP-04.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| purposes | ConsentPurpose[] | 필수 |
| expectedRevision | Revision | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| consentRevision | Revision |

개별 오류: REVISION_CONFLICT. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-085 · POST /v1/me/data-requests

권한: account_recent_auth. 정상 응답: 202. 작업: TRIP-04.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| type | Text | 필수 |
| scope | Text[] | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| operation | Operation |

개별 오류: AUTH_REQUIRED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-086 · GET /v1/ops/exceptions

권한: ops_exception_read. 정상 응답: 200. 작업: OPS-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| type | Text | 선택 |
| cursor | Text | 선택 |

| 응답 data 필드 | 타입 |
|---|---|
| exceptions | ReconciliationException[] |
| nextCursor | Text? |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-087 · POST /v1/ops/reconciliation-jobs

권한: ops_reconcile. 정상 응답: 202. 작업: OPS-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| subjectRef | Id | 필수 |
| reason | Text | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| operation | Operation |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-088 · GET /v1/ops/audit-events

권한: ops_audit_read. 정상 응답: 200. 작업: OPS-01.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| subjectRef | Id | 선택 |
| cursor | Text | 선택 |

| 응답 data 필드 | 타입 |
|---|---|
| events | AuditView[] |
| nextCursor | Text? |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-089 · GET /v1/service-health

권한: service_health_read. 정상 응답: 200. 작업: INDEX-05, OPS-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| services | ServiceHealth[] |
| indexerLag | IndexerLag? |
| observedAt | Time |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-090 · GET /v1/devices

권한: device_owner. 정상 응답: 200. 작업: HW-03, APP-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| cursor | Text | 선택 |

| 응답 data 필드 | 타입 |
|---|---|
| devices | DeviceMetadata[] |
| nextCursor | Text? |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-091 · GET /v1/rentals/{rentalId}

권한: rental_owner_or_operator. 정상 응답: 200. 작업: STAMP-03, STAMP-04.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| rental | RentalView |
| checklist | ReturnChecklist |
| lastResetState | ResetStateView? |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-092 · GET /v1/stores/{storeId}/orders

권한: store_order_read. 정상 응답: 200. 작업: SHOP-02, SHOP-04.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| cursor | Text | 선택 |
| state | Text | 선택 |

| 응답 data 필드 | 타입 |
|---|---|
| orders | OrderView[] |
| nextCursor | Text? |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-093 · GET /v1/stores/{storeId}/settlements

권한: store_sales_read. 정상 응답: 200. 작업: SHOP-05.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| period | Period | 선택 |
| cursor | Text | 선택 |

| 응답 data 필드 | 타입 |
|---|---|
| settlements | SettlementView[] |
| nextCursor | Text? |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-094 · GET /v1/recordings

권한: account. 정상 응답: 200. 작업: REC-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| cursor | Text | 선택 |

| 응답 data 필드 | 타입 |
|---|---|
| recordingMetadata | RecordingView[] |
| nextCursor | Text? |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-095 · GET /v1/itineraries/{itineraryId}

권한: itinerary_owner. 정상 응답: 200. 작업: AI-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| itinerary | ItineraryView |
| validation | RouteValidation |
| revision | Revision |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-096 · GET /v1/participations/{participationId}

권한: participant. 정상 응답: 200. 작업: AI-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| participation | ParticipationView |
| progress | ProgressView |
| evidenceRevisions | EvidenceRevision[] |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-097 · GET /v1/trips

권한: account. 정상 응답: 200. 작업: TRIP-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| cursor | Text | 선택 |

| 응답 data 필드 | 타입 |
|---|---|
| trips | TripView[] |
| nextCursor | Text? |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-098 · POST /v1/wallet-bindings

권한: device_owner. 정상 응답: 201. 작업: HW-04, APP-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| deviceId | Id | 필수 |
| walletHandle | Id | 필수 |
| address | Address | 필수 |
| ownershipProof | ProfileInput | 필수 |
| enrollmentId | Id | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| wallet | WalletView |
| binding | WalletBinding |

개별 오류: ADDRESS_PROOF_INVALID. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-099 · GET /v1/ops/stores

권한: ops_store_read. 정상 응답: 200. 작업: OPS-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| query | Text | 선택 |
| cursor | Text | 선택 |

| 응답 data 필드 | 타입 |
|---|---|
| stores | StoreView[] |
| nextCursor | Text? |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-100 · GET /v1/ops/devices

권한: ops_device_read. 정상 응답: 200. 작업: OPS-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| query | Text | 선택 |
| rentalState | Text | 선택 |
| cursor | Text | 선택 |

| 응답 data 필드 | 타입 |
|---|---|
| devices | DeviceMetadata[] |
| nextCursor | Text? |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-101 · GET /v1/firmware-releases/{releaseId}

권한: release_read. 정상 응답: 200. 작업: OTA-04, OPS-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| release | ReleaseView |
| compatibility | CompatibilityView |
| status | ReleaseState |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-102 · GET /v1/wallets/{walletId}/offering-holdings

권한: wallet_read. 정상 응답: 200. 작업: STO-03.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| cursor | Text | 선택 |

| 응답 data 필드 | 타입 |
|---|---|
| holdings | Holding[] |
| eligibility | Eligibility |
| nextCursor | Text? |
| asOfBlock | BlockRef? |

개별 오류: FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-103 · POST /v1/auth/flows

권한: auth_flow_bootstrap. 정상 응답: 201. 작업: AUTH-02, AUTH-03, AUTH-04, BASE-05.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| provider | Provider | 필수 |
| purpose | AuthPurpose | 필수 |
| clientProfileId | Id | 필수 |
| redirectId | Id | 필수 |
| challengeParameters | ProfileInput | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| flowId | Id |
| authorizationParameters | ProfileInput |
| expiresAt | Time |

개별 오류: AUTH_FLOW_INVALID, AUTH_REQUIRED, RATE_LIMITED, UNSUPPORTED_PROFILE. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-104 · POST /v1/receipt-claims/challenges

권한: receipt_claim_eligible. 정상 응답: 201. 작업: APP-04, PAY-03, PAY-04, AUTH-01.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| receiptLocator | Id | 필수 |
| eligibilityProof | ProfileInput | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| claimId | Id |
| challenge | ReceiptClaimChallenge |
| expiresAt | Time |

개별 오류: CLAIM_NOT_ELIGIBLE, UNSUPPORTED_PROFILE, RATE_LIMITED. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-105 · POST /v1/receipt-claims/{claimId}/confirm

권한: receipt_claim_confirm. 정상 응답: 200. 작업: APP-04, PAY-03, PAY-04, AUTH-01.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| approvalProof | ProfileInput | 필수 |

| 응답 data 필드 | 타입 |
|---|---|
| claimId | Id |
| state | ClaimLinked |
| receiptId | Id |
| ownershipRevision | Revision |

개별 오류: CLAIM_EXPIRED, PROOF_INVALID, CLAIM_CONFLICT, CLAIM_NOT_ELIGIBLE. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-106 · GET /v1/receipt-claims/{claimId}

권한: receipt_claim_owner. 정상 응답: 200. 작업: APP-04, AUTH-01.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| 없음 | — | — |

| 응답 data 필드 | 타입 |
|---|---|
| claimId | Id |
| state | ReceiptClaimState |
| receiptId | Id? |
| safeErrorCode | Text? |

개별 오류: NOT_FOUND_OR_FORBIDDEN. 인증/권한/형태/멱등성 공통 오류도 적용.

## API-107 · GET /v1/protected-results/{resultId}

권한: protected_result_read. 정상 응답: 200. 작업: BASE-05, APP-03, SMART-01, SMART-03, REC-03, AI-02.

| 입력 필드 | 타입 | 필수 |
|---|---|---|
| purpose | Id | 필수 |
| streamRef | Id | 선택 |

| 응답 data 필드 | 타입 |
|---|---|
| resultId | Id |
| kind | Id |
| version | Id |
| payloadDigest | TaggedDigest |
| delivery | ResultDelivery |
| payload | ProfileInput? |
| streamRef | Id? |
| expiresAt | Time |

개별 오류: NOT_FOUND_OR_FORBIDDEN, RESULT_EXPIRED, RESULT_UNAVAILABLE, CONSENT_REVOKED. 인증/권한/형태/멱등성 공통 오류도 적용.

JSON 외 binary 응답: [전달 계약](security-api-integration.md#binary-delivery).


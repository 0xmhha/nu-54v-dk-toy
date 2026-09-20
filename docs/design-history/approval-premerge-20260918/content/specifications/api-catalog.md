# API 작업 계약 목록

설계 제안. [공통 규칙](interface-contracts.md) · [핵심 8개 DTO](critical-dtos.md) · [확장 DTO](extended-dtos.md) · [보안 API 통합](security-api-integration.md). Path 변수는 상세 request.path를 따른다.

| ID | 요청 | 권한 | 입력 | 결과 | 작업 |
|---|---|---|---|---|---|
| API-001 | POST /v1/auth/exchanges | public_auth_flow | provider, authFlowId, providerProof | session, account | AUTH-02, AUTH-03 |
| API-002 | POST /v1/auth/links | account_recent_auth | provider, authFlowId, providerProof | linkedProviders | AUTH-04 |
| API-003 | POST /v1/auth/logout | account | sessionId | revoked | AUTH-04 |
| API-004 | POST /v1/account-deletions | account_recent_auth | scope, acknowledgedWalletRecovery | operation | AUTH-04 |
| API-005 | GET /v1/me | account | — | account, memberships, linkedProviders | AUTH-01 |
| API-006 | POST /v1/auth/refresh | refresh_session | refreshToken | session | AUTH-04 |
| API-007 | GET /v1/wallets | account | — | wallets | APP-02 |
| API-008 | GET /v1/stores/{storeId}/wallets | store_asset_read | — | wallets | APP-02, SHOP-06 |
| API-009 | GET /v1/wallets/{walletId}/balances | wallet_read | — | assets, observedAt, blockRef | APP-02, TOKEN-03 |
| API-010 | POST /v1/device-enrollments | account | deviceId, deviceProof, rentalId | enrollmentId, enrollmentChallenge | HW-03 |
| API-011 | POST /v1/device-enrollments/{enrollmentId}/confirm | account | deviceProof, challengeResponse | deviceBinding | HW-03 |
| API-012 | DELETE /v1/devices/{deviceId}/binding | device_owner | expectedRevision | operation | HW-03 |
| API-013 | GET /v1/devices/{deviceId} | device_owner | — | deviceMetadata, lastReportedAt | HW-01, FIND-02 |
| API-014 | POST /v1/cloud-wallets | account_recent_auth | createRequestId, participantEnrollmentProof | operation | MPC-02 |
| API-015 | POST /v1/wallets/{walletId}/signing-sessions | wallet_sign_authorized | intentId, approvalProof | operation | MPC-03 |
| API-016 | POST /v1/wallets/{walletId}/recoveries | wallet_recovery_proof | recoveryProof, newParticipantProof | operation | MPC-04 |
| API-017 | POST /v1/transaction-intents | wallet_use | walletId, chainId, action, actionInput | intent, review, expiry | PAY-01, APP-03 |
| API-018 | POST /v1/transaction-submissions | intent_submit_capability | intentId, signedPayload | operation, transactionRef | PAY-02 |
| API-019 | GET /v1/transactions/{transactionRef} | transaction_read | — | execution, canonicality, confirmation, observedAt | INDEX-05 |
| API-020 | GET /v1/operations/{operationId} | operation_owner_or_capability | — | operation | PAY-02, REC-03 |
| API-021 | POST /v1/stores | account | displayName, location, timezone | store | SHOP-01 |
| API-022 | GET /v1/stores | account | — | memberships | SHOP-01 |
| API-023 | POST /v1/stores/{storeId}/terminal-sessions | store_terminal_manage | terminalId, mode | terminalSession | SHOP-01 |
| API-024 | POST /v1/stores/{storeId}/memberships | store_owner | accountId, role | membership | SHOP-01 |
| API-025 | DELETE /v1/stores/{storeId}/memberships/{membershipId} | store_owner | expectedRevision | revoked | SHOP-01 |
| API-026 | GET /v1/stores/{storeId}/menu | store_customer_session | — | menu, revision | SHOP-02 |
| API-027 | PUT /v1/stores/{storeId}/menu-items/{itemId} | store_menu_manage | name, options, price, availability, expectedRevision | menuItem | SHOP-02 |
| API-028 | POST /v1/stores/{storeId}/recipient-changes | store_funds_manage_recent_auth | walletRef, ownershipProof, expectedRevision | recipientConfiguration | SHOP-06 |
| API-029 | POST /v1/stores/{storeId}/orders | store_customer_session | items, menuRevision | order, orderCapability | SHOP-02, PAY-03 |
| API-030 | GET /v1/orders/{orderId} | order_read_or_capability | — | order, paymentSummary | SHOP-03 |
| API-031 | POST /v1/orders/{orderId}/cancel | order_cancel | reason, expectedRevision | order | SHOP-02 |
| API-032 | POST /v1/orders/{orderId}/payment-attempts | order_payment_capability | assetId, accountMode | attempt, quote, pairingChallenge | PAY-01, PAY-03 |
| API-033 | POST /v1/payment-attempts/{attemptId}/device-sessions | terminal_attempt_capability | deviceProof, challengeResponse | restrictedDeviceSession | HW-03, SHOP-03 |
| API-034 | POST /v1/payment-attempts/{attemptId}/intents | attempt_device_capability | signerAddress, ownershipProof, sessionId | intent, review | PAY-01, HW-05 |
| API-035 | GET /v1/payment-attempts/{attemptId} | attempt_read_capability | — | attempt, observation, exception | PAY-04 |
| API-036 | POST /v1/orders/{orderId}/refunds | store_refund_request | amount, destinationProof, reason | refund, reservedAmount | SHOP-04 |
| API-037 | POST /v1/refunds/{refundId}/authorize | store_funds_manage_recent_auth | expectedRevision | refund, signingIntent | SHOP-04 |
| API-038 | GET /v1/refunds/{refundId} | refund_read | — | refund, transactionRef | SHOP-04 |
| API-039 | GET /v1/stores/{storeId}/sales | store_sales_read | period, assetId | summary, exceptions, asOfRevision | SHOP-05 |
| API-040 | POST /v1/stores/{storeId}/settlements | store_settlement_manage | period, expectedRevision, note | settlementRecord | SHOP-05 |
| API-041 | POST /v1/settlements/{settlementId}/corrections | store_settlement_manage | reason, expectedRevision | correctedRecord | SHOP-05 |
| API-042 | GET /v1/me/receipts | account | cursor, limit | receipts, nextCursor | APP-04 |
| API-043 | GET /v1/me/passport | account | — | stamps, benefits, revision, pendingEntitlements | STAMP-01, STAMP-02 |
| API-044 | POST /v1/benefit-redemptions | benefit_owner | benefitId, orderId, expectedRevision | redemption | STAMP-01 |
| API-045 | POST /v1/rentals | rental_operator | deviceId, accountId | rental | STAMP-03 |
| API-046 | POST /v1/rentals/{rentalId}/return-checks | rental_owner_or_operator | walletOrigin, externalAccessProof | checklist, conditionalResetClearance | STAMP-03 |
| API-047 | POST /v1/rentals/{rentalId}/complete-return | rental_owner_and_clearance | checklistRevision, clearanceId, deviceResetEvidence, ownerApprovalRef | rental, bindingRevoked | STAMP-04 |
| API-048 | GET /v1/devices/{deviceId}/firmware-offers | device_owner | appVersion, protocolVersion | offer, compatibility | OTA-02 |
| API-049 | POST /v1/firmware-releases | firmware_release_operator | manifest, packageRef | release | OTA-02, OPS-02 |
| API-050 | POST /v1/firmware-releases/{releaseId}/withdraw | firmware_release_operator | reason, expectedRevision | release | OTA-04, OPS-02 |
| API-051 | POST /v1/recordings | account | recordingId, format, completeness, duration | recordingMetadata | REC-03 |
| API-052 | POST /v1/recordings/{recordingId}/processing | recording_owner_with_consent | sourceRef, consentRevision, jobType | operation | REC-03 |
| API-053 | GET /v1/recordings/{recordingId}/processing | recording_owner | — | jobs, transcriptRef, summaryRef | REC-03 |
| API-054 | DELETE /v1/recordings/{recordingId} | recording_owner | expectedRevision | operation | REC-03, TRIP-04 |
| API-055 | POST /v1/smart-account-plans | wallet_use | walletId, selectedModel | transitionPlan, review | SMART-01, SMART-03 |
| API-056 | POST /v1/smart-account-intents | wallet_sign_authorized | planId, action, actionInput | intent, review | SMART-02 |
| API-057 | POST /v1/market-quotes | account | marketId, action, amountIn, assetIn, assetOut, inputs, positionId, shareAmount | quote, fees, minimumOutputs, expiresAt | DEX-02, FX-02 |
| API-058 | POST /v1/market-intents | wallet_use | walletId, quoteId, slippageChoice | intent, review | DEX-03, FX-03 |
| API-059 | GET /v1/wallets/{walletId}/liquidity-positions | wallet_read | — | positions, asOfBlock | DEX-03 |
| API-060 | GET /v1/perpetual/markets | account | — | markets, priceValidity | PERP-05 |
| API-061 | POST /v1/perpetual/intents | wallet_use | walletId, marketId, action, actionInput | intent, review | PERP-03, PERP-05 |
| API-062 | GET /v1/wallets/{walletId}/perpetual-positions | wallet_read | — | positions, funding, liquidations, asOfBlock | PERP-05 |
| API-063 | GET /v1/offerings | account | — | offerings, eligibilityRules | STO-03 |
| API-064 | POST /v1/offerings/{offeringId}/intents | offering_actor | walletId, action, actionInput, credentialRef | intent, review | STO-02, STO-03 |
| API-065 | POST /v1/credentials | credential_issuer | subjectProof, type, claims | credential | DID-02 |
| API-066 | GET /v1/me/credentials | account | — | credentials, status | DID-03 |
| API-067 | POST /v1/credential-presentations | credential_owner | requestRef, credentialId, disclosureChoice, holderProof | presentation | DID-03 |
| API-068 | POST /v1/credential-verifications | credential_verifier | presentation, challengeRef | verification | DID-02 |
| API-069 | POST /v1/credentials/{credentialId}/revoke | credential_issuer | reason, expectedRevision | revocation | DID-02 |
| API-070 | POST /v1/paid-resource-requests | wallet_use | resourceId, resourceInput, walletId | resourceRequest, paymentRequirement | X402-03 |
| API-071 | POST /v1/paid-resource-requests/{requestId}/proofs | request_owner | paymentProof | operation, entitlement | X402-02, X402-03 |
| API-072 | GET /v1/paid-resource-requests/{requestId}/result | request_owner | — | deliveryState, resourceRef | X402-03 |
| API-073 | GET /v1/places | account | region, query, optionalLocation, cursor | places, source, observedAt, nextCursor | TRIP-01 |
| API-074 | POST /v1/places/{placeId}/reviews | account | rating, text, purchaseRef | review, provenance | TRIP-02 |
| API-075 | PATCH /v1/reviews/{reviewId} | review_owner | text, rating, expectedRevision | review | TRIP-02 |
| API-076 | DELETE /v1/reviews/{reviewId} | review_owner | expectedRevision | operation | TRIP-02 |
| API-077 | POST /v1/trips | account | region, locale, timezone, consentRevision | trip | TRIP-03 |
| API-078 | POST /v1/trips/{tripId}/visits | trip_owner | placeId, evidence, sourceType | visit, validity | TRIP-03 |
| API-079 | GET /v1/trips/{tripId}/timeline | trip_owner | cursor | entries, nextCursor | TRIP-03 |
| API-080 | POST /v1/itineraries | account | tripId, constraints, consentRevision | operation | AI-02 |
| API-081 | PATCH /v1/itineraries/{itineraryId} | itinerary_owner | stops, constraints, expectedRevision | itinerary, validation | AI-02 |
| API-082 | POST /v1/challenges/{challengeId}/participations | account | tripId | participation | AI-03 |
| API-083 | POST /v1/participations/{participationId}/evidence | participant | stepId, evidenceRef | progress, stamp | AI-03 |
| API-084 | PUT /v1/me/consents | account | purposes, expectedRevision | consentRevision | TRIP-04 |
| API-085 | POST /v1/me/data-requests | account_recent_auth | type, scope | operation | TRIP-04 |
| API-086 | GET /v1/ops/exceptions | ops_exception_read | type, cursor | exceptions, nextCursor | OPS-03 |
| API-087 | POST /v1/ops/reconciliation-jobs | ops_reconcile | subjectRef, reason | operation | OPS-03 |
| API-088 | GET /v1/ops/audit-events | ops_audit_read | subjectRef, cursor | events, nextCursor | OPS-01 |
| API-089 | GET /v1/service-health | service_health_read | — | services, indexerLag, observedAt | INDEX-05, OPS-03 |
| API-090 | GET /v1/devices | device_owner | cursor | devices, nextCursor | HW-03, APP-02 |
| API-091 | GET /v1/rentals/{rentalId} | rental_owner_or_operator | — | rental, checklist, lastResetState | STAMP-03, STAMP-04 |
| API-092 | GET /v1/stores/{storeId}/orders | store_order_read | cursor, state | orders, nextCursor | SHOP-02, SHOP-04 |
| API-093 | GET /v1/stores/{storeId}/settlements | store_sales_read | period, cursor | settlements, nextCursor | SHOP-05 |
| API-094 | GET /v1/recordings | account | cursor | recordingMetadata, nextCursor | REC-03 |
| API-095 | GET /v1/itineraries/{itineraryId} | itinerary_owner | — | itinerary, validation, revision | AI-02 |
| API-096 | GET /v1/participations/{participationId} | participant | — | participation, progress, evidenceRevisions | AI-03 |
| API-097 | GET /v1/trips | account | cursor | trips, nextCursor | TRIP-03 |
| API-098 | POST /v1/wallet-bindings | device_owner | deviceId, walletHandle, address, ownershipProof, enrollmentId | wallet, binding | HW-04, APP-02 |
| API-099 | GET /v1/ops/stores | ops_store_read | query, cursor | stores, nextCursor | OPS-02 |
| API-100 | GET /v1/ops/devices | ops_device_read | query, rentalState, cursor | devices, nextCursor | OPS-02 |
| API-101 | GET /v1/firmware-releases/{releaseId} | release_read | — | release, compatibility, status | OTA-04, OPS-02 |
| API-102 | GET /v1/wallets/{walletId}/offering-holdings | wallet_read | cursor | holdings, eligibility, nextCursor, asOfBlock | STO-03 |
| API-103 | POST /v1/auth/flows | auth_flow_bootstrap | provider, purpose, clientProfileId, redirectId, challengeParameters | flowId, authorizationParameters, expiresAt | AUTH-02, AUTH-03, AUTH-04, BASE-05 |
| API-104 | POST /v1/receipt-claims/challenges | receipt_claim_eligible | receiptLocator, eligibilityProof | claimId, challenge, expiresAt | APP-04, PAY-03, PAY-04, AUTH-01 |
| API-105 | POST /v1/receipt-claims/{claimId}/confirm | receipt_claim_confirm | approvalProof | claimId, state, receiptId, ownershipRevision | APP-04, PAY-03, PAY-04, AUTH-01 |
| API-106 | GET /v1/receipt-claims/{claimId} | receipt_claim_owner | — | claimId, state, receiptId, safeErrorCode | APP-04, AUTH-01 |
| API-107 | GET /v1/protected-results/{resultId} | protected_result_read | purpose, streamRef | resultId, kind, version, payloadDigest, delivery, payload, streamRef, expiresAt | BASE-05, APP-03, SMART-01, SMART-03, REC-03, AI-02 |

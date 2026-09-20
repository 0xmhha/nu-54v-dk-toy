# 핵심 8개 API 요청·응답 기준

현재 총 API는 110개이며 핵심 8개와 확장 102개로 구분한다. [승인 기준 통합](approval-baseline.md) · [전체 schema](critical-dtos.schema.json) · [계약/예제](critical-dtos.json) · [확장 타입](extended-dtos.md). 제품 구현·실행 검증 결과가 아니다.

API-034/018/037은 approval-v1-draft 전체 envelope로 반영했다. 그 밖의 주문·환불 생성·반납 계약은 기존 기준이며 후속 commerce/return 후보의 전체 병합은 아직 별도다. 합성 예제는 서로 독립된 사례이며 원래의 주문 예제 하나에 모든 새 snapshot을 연결했다고 가정하지 않는다.

| API | 요청 | 결과 | 계약 상태 |
|---|---|---|---|
| API-029 | items, menuRevision | order, orderCapability | 기존 기준 / 후속 후보 별도 |
| API-032 | assetId, accountMode | attempt, quote, pairingChallenge | 기존 기준 / 후속 후보 별도 |
| API-034 | signerAddress, sessionId, ownershipEvidence | snapshot, operation | approval-v1-draft |
| API-018 | intentId, approvalContextId, signedPayload, approvalEvidence | operation, transactionRef, approvalOperationId | approval-v1-draft |
| API-036 | amount, destinationProof, reason | refund, reservedAmount | 기존 기준 / 후속 후보 별도 |
| API-037 | expectedRevision, expectedFundingRevision, merchantSigner | refund, snapshot, operation | approval-v1-draft |
| API-046 | walletOrigin, externalAccessProof | checklist, conditionalResetClearance | 기존 기준 / 후속 후보 별도 |
| API-047 | checklistRevision, clearanceId, deviceResetEvidence, ownerApprovalRef | rental, bindingRevoked | 기존 기준 / 후속 후보 별도 |

승인 요청은 current account/terminal 인증, 새 제출은 sender-bound submit_exact와 provenance를 요구한다. 요청의 예상 revision과 commit 뒤 검증한 revision은 구분한다. API-037의 업무 승인자는 실제 signer의 권한을 대신하지 않는다.

API-020의 부모/자식 ancestry와 결과 공개 규칙은 승인 기준을 따른다. API-046의 eligible/clearance는 기존 정책별 필수 점검 결과이고, 미응답 RR-DEC-01을 백업 추가나 초기화 허가로 바꾸지 않는다.

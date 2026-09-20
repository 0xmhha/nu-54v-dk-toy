# BLE 논리 명령 기준

34개 명령의 역할을 유지한다. [승인 기준](approval-baseline.md)의 5개 승인 명령은 approval-ble-v1-draft 상세 envelope를 사용하며 draft-1 fallback을 차단한다. 실제 GATT/codec/profile/실기 호환은 미검증이다.

| 명령 | 역할 | 입력 | 결과 | 상세 승인 schema |
|---|---|---|---|---|
| device.info | unbound_public | requestId | model, firmwareVersion, protocolVersions, capabilities | 기존 논리 계약 |
| session.open | handshake | role, challenge, peerProof, requestedScope | handshakeResponse | 기존 논리 계약 |
| session.confirm | handshake | transcriptProof | sessionId, allowedCommands, expiryPolicy | 기존 논리 계약 |
| session.close | owner, payment_terminal | reason | closed | 기존 논리 계약 |
| wallet.create | owner | requestId, creationProfile | walletHandle, address | 기존 논리 계약 |
| wallet.import.begin | owner | requestId, inputFormat, importProfile | importSessionId | 기존 논리 계약 |
| wallet.import.chunk | owner | importSessionId, sequence, sensitivePayload | ack | 기존 논리 계약 |
| wallet.import.commit | owner | importSessionId, transcriptDigest | walletHandle, address | 기존 논리 계약 |
| wallet.import.abort | owner | importSessionId | cleared | 기존 논리 계약 |
| wallet.address | owner | walletHandle, derivationProfile | address | 기존 논리 계약 |
| device.settings.update | owner | expectedRevision, settings | revision | 기존 논리 계약 |
| payment.identify | payment_terminal | attemptId, challenge, merchantProof | typed_result_or_error | AB_BLE_PaymentIdentifyRequest |
| payment.prepare | payment_terminal | attemptId, snapshot, merchantProofRef, proximityRef | typed_result_or_error | AB_BLE_PaymentPrepareRequest |
| payment.result | payment_terminal | reviewId, approvalContextId | typed_result_or_error | AB_BLE_PaymentResultRequest |
| wallet.sign.prepare | owner | snapshot | typed_result_or_error | AB_BLE_WalletSignPrepareRequest |
| wallet.sign.result | owner | reviewId, approvalContextId | typed_result_or_error | AB_BLE_WalletSignResultRequest |
| request.cancel | owner, payment_terminal | reviewId | cancelState | 기존 논리 계약 |
| proximity.observe | ranging_peer | measurementSessionId, peerIds, sequence, result, quality | acceptedObservation | 기존 논리 계약 |
| recording.start | owner | formatProfile | recordingId, state | 기존 논리 계약 |
| recording.stop | owner | recordingId | lastSequence, state | 기존 논리 계약 |
| audio.frame | device_to_owner | recordingId, sequence, formatProfile, payload | streamFrame | 기존 논리 계약 |
| audio.flow-control | owner | recordingId, receiveWindow | streamState | 기존 논리 계약 |
| find.start | owner | pattern, durationLimit | findState | 기존 논리 계약 |
| find.stop | owner | requestId | findState | 기존 논리 계약 |
| passport.sync | owner | accountBinding, revision, verifiedSummary | storedRevision | 기존 논리 계약 |
| fota.begin | owner | releaseManifest, compatibility | transferId, offset | 기존 논리 계약 |
| fota.chunk | owner | transferId, offset, bytes | acceptedOffset | 기존 논리 계약 |
| fota.finalize | owner | transferId | verifiedOrRejected | 기존 논리 계약 |
| fota.apply | owner | transferId | rebooting | 기존 논리 계약 |
| fota.status | owner | transferId | state, runningVersion | 기존 논리 계약 |
| credentials.list | owner | cursor | credentialMetadata, nextCursor | 기존 논리 계약 |
| credentials.delete | owner | credentialHandle | deleted | 기존 논리 계약 |
| device.reset.prepare | owner | rentalId, resetClearance | reviewId | 기존 논리 계약 |
| device.reset.confirm | owner | reviewId | resetState, deviceResetEvidence | 기존 논리 계약 |

# BLE 논리 명령 목록

제안 상태. 기존 서버의 실제 endpoint 또는 실기 검증된 프로토콜이 아니다. 필드 목록은 설계 경계이며 모든 필드가 필수라는 뜻은 아니다. [공통 규칙](interface-contracts.md)을 적용한다.

| 명령 | 허용 역할 | 입력 | 결과 | 작업 |
|---|---|---|---|---|
| device.info | unbound_public | requestId | model, firmwareVersion, protocolVersions, capabilities | HW-01 |
| session.open | handshake | role, challenge, peerProof, requestedScope | handshakeResponse | HW-03 |
| session.confirm | handshake | transcriptProof | sessionId, allowedCommands, expiryPolicy | HW-03 |
| session.close | owner, payment_terminal | reason | closed | HW-03 |
| wallet.create | owner | requestId, creationProfile | walletHandle, address | HW-02 |
| wallet.import.begin | owner | requestId, inputFormat, importProfile | importSessionId | HW-04 |
| wallet.import.chunk | owner | importSessionId, sequence, sensitivePayload | ack | HW-04 |
| wallet.import.commit | owner | importSessionId, transcriptDigest | walletHandle, address | HW-04 |
| wallet.import.abort | owner | importSessionId | cleared | HW-04 |
| wallet.address | owner | walletHandle, derivationProfile | address | HW-02 |
| device.settings.update | owner | expectedRevision, settings | revision | HW-04 |
| payment.identify | payment_terminal | attemptId, challenge, merchantProof | selectedPayerAddress, ownershipProof, attemptBinding | HW-03, HW-05 |
| payment.prepare | payment_terminal | attemptId, intent, merchantProofRef, proximityRef | reviewId, payloadDigest | HW-05 |
| payment.result | payment_terminal | reviewId | state, signatureOrSignedPayload | HW-05 |
| wallet.sign.prepare | owner | intent | reviewId, payloadDigest | HW-05 |
| wallet.sign.result | owner | reviewId | state, signatureOrSignedPayload | HW-05 |
| request.cancel | owner, payment_terminal | reviewId | cancelState | HW-05 |
| proximity.observe | ranging_peer | measurementSessionId, peerIds, sequence, result, quality | acceptedObservation | HW-06 |
| recording.start | owner | formatProfile | recordingId, state | REC-01 |
| recording.stop | owner | recordingId | lastSequence, state | REC-01 |
| audio.frame | device_to_owner | recordingId, sequence, formatProfile, payload | streamFrame | REC-02 |
| audio.flow-control | owner | recordingId, receiveWindow | streamState | REC-02 |
| find.start | owner | pattern, durationLimit | findState | FIND-01 |
| find.stop | owner | requestId | findState | FIND-01 |
| passport.sync | owner | accountBinding, revision, verifiedSummary | storedRevision | STAMP-02 |
| fota.begin | owner | releaseManifest, compatibility | transferId, offset | OTA-03 |
| fota.chunk | owner | transferId, offset, bytes | acceptedOffset | OTA-03 |
| fota.finalize | owner | transferId | verifiedOrRejected | OTA-03 |
| fota.apply | owner | transferId | rebooting | OTA-03 |
| fota.status | owner | transferId | state, runningVersion | OTA-03 |
| credentials.list | owner | cursor | credentialMetadata, nextCursor | KEY-02 |
| credentials.delete | owner | credentialHandle | deleted | KEY-03 |
| device.reset.prepare | owner | rentalId, resetClearance | reviewId | STAMP-04 |
| device.reset.confirm | owner | reviewId | resetState, deviceResetEvidence | STAMP-04 |

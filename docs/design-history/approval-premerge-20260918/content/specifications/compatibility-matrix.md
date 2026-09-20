# 버전 호환 행렬

[해석 규칙](implementation-interfaces.md) · [원본](compatibility-matrix.json) · [manifest 템플릿](compatibility-manifest.template.json). 26개 모두 미검증이다. 버전 번호를 임의 채우지 않았으며 실행 가능한 릴리스 허용 목록이 아니다.

## COMP-01 · IF-01

- 경계: IF-01 — Zephyr board/drivers → firmware services
- 고정할 값: producerBuild, consumerBuild, boardRevision, boardTarget, zephyrCommit, distributionCommit, toolchainVersion, devicetreeDigest, kconfigDigest
- 판정: exact tested tuple; explicit adapter/migration evidence required for any non-identical supported combination
- 불일치: reject affected new operation; show update/recovery path; preserve already submitted transaction observation
- 증거: producer and consumer build identifiers; actual target environment; positive and negative interoperability results; rollback or recovery outcome
- 상태: unverified

## COMP-02 · IF-02

- 경계: IF-02 — isolated firmware key service → wallet/passkey service
- 고정할 값: producerBuild, consumerBuild, cryptoProfile, cryptoBackendVersion, keyStoreSchema, keyPolicyVersion, secureBoundaryDigest
- 판정: exact tested tuple; explicit adapter/migration evidence required for any non-identical supported combination
- 불일치: reject affected new operation; show update/recovery path; preserve already submitted transaction observation
- 증거: producer and consumer build identifiers; actual target environment; positive and negative interoperability results; rollback or recovery outcome
- 상태: unverified

## COMP-03 · IF-03

- 경계: IF-03 — firmware session service → RN native bridge / kiosk / ranging peer
- 고정할 값: producerBuild, consumerBuild, controlProtocol, handshakeProfile, rolePolicyVersion, controllerVersion, transportProfile
- 판정: exact tested tuple; explicit adapter/migration evidence required for any non-identical supported combination
- 불일치: reject affected new operation; show update/recovery path; preserve already submitted transaction observation
- 증거: producer and consumer build identifiers; actual target environment; positive and negative interoperability results; rollback or recovery outcome
- 상태: unverified

## COMP-04 · IF-04

- 경계: IF-04 — firmware approval service → wallet service / payment peer
- 고정할 값: producerBuild, consumerBuild, intentSchema, reviewSchema, signatureProfile, keyPolicyVersion, controlProtocol
- 판정: exact tested tuple; explicit adapter/migration evidence required for any non-identical supported combination
- 불일치: reject affected new operation; show update/recovery path; preserve already submitted transaction observation
- 증거: producer and consumer build identifiers; actual target environment; positive and negative interoperability results; rollback or recovery outcome
- 상태: unverified

## COMP-05 · IF-05

- 경계: IF-05 — Zephyr update service / bootloader → RN update bridge
- 고정할 값: producerBuild, consumerBuild, boardRevision, bootloaderCommit, imageFormat, partitionDigest, updateProfile, keyStoreSchema, migrationVersion
- 판정: exact tested tuple; explicit adapter/migration evidence required for any non-identical supported combination
- 불일치: reject affected new operation; show update/recovery path; preserve already submitted transaction observation
- 증거: producer and consumer build identifiers; actual target environment; positive and negative interoperability results; rollback or recovery outcome
- 상태: unverified

## COMP-06 · IF-06

- 경계: IF-06 — firmware CTAP authenticator → verified CTAP transport / RP client
- 고정할 값: producerBuild, consumerBuild, ctapVersion, transportProfile, rpPolicy, authenticatorAlgorithms, residentStoreSchema
- 판정: exact tested tuple; explicit adapter/migration evidence required for any non-identical supported combination
- 불일치: reject affected new operation; show update/recovery path; preserve already submitted transaction observation
- 증거: producer and consumer build identifiers; actual target environment; positive and negative interoperability results; rollback or recovery outcome
- 상태: unverified

## COMP-07 · IF-07

- 경계: IF-07 — firmware feature services → native audio/storage / user app
- 고정할 값: producerBuild, consumerBuild, audioProfile, frameSchema, sequencePolicy, resourcePolicy, stampProjectionSchema
- 판정: exact tested tuple; explicit adapter/migration evidence required for any non-identical supported combination
- 불일치: reject affected new operation; show update/recovery path; preserve already submitted transaction observation
- 증거: producer and consumer build identifiers; actual target environment; positive and negative interoperability results; rollback or recovery outcome
- 상태: unverified

## COMP-08 · IF-08

- 경계: IF-08 — Android/iOS native modules → RN TypeScript app
- 고정할 값: producerBuild, consumerBuild, rnVersion, nativeBridgeVersion, androidSdk, iosSdk, transportProfile, lifecyclePolicy
- 판정: exact tested tuple; explicit adapter/migration evidence required for any non-identical supported combination
- 불일치: reject affected new operation; show update/recovery path; preserve already submitted transaction observation
- 증거: producer and consumer build identifiers; actual target environment; positive and negative interoperability results; rollback or recovery outcome
- 상태: unverified

## COMP-09 · IF-09

- 경계: IF-09 — business API / security adapters → user/kiosk/backoffice
- 고정할 값: producerBuild, consumerBuild, apiSchema, policyVersion, storageSchema, capabilityProfile, providerProfile
- 판정: exact tested tuple; explicit adapter/migration evidence required for any non-identical supported combination
- 불일치: reject affected new operation; show update/recovery path; preserve already submitted transaction observation
- 증거: producer and consumer build identifiers; actual target environment; positive and negative interoperability results; rollback or recovery outcome
- 상태: unverified

## COMP-10 · IF-10

- 경계: IF-10 — signing orchestrator → app review / chain submitter
- 고정할 값: producerBuild, consumerBuild, signerProfile, intentSchema, reviewSchema, signatureProfile, submissionProfile
- 판정: exact tested tuple; explicit adapter/migration evidence required for any non-identical supported combination
- 불일치: reject affected new operation; show update/recovery path; preserve already submitted transaction observation
- 증거: producer and consumer build identifiers; actual target environment; positive and negative interoperability results; rollback or recovery outcome
- 상태: unverified

## COMP-11 · IF-11

- 경계: IF-11 — isolated MPC engine → cloud signing coordinator / mobile participant
- 고정할 값: producerBuild, consumerBuild, mpcProtocol, libraryCommit, participantEpoch, recoveryProfile, transcriptFormat
- 판정: exact tested tuple; explicit adapter/migration evidence required for any non-identical supported combination
- 불일치: reject affected new operation; show update/recovery path; preserve already submitted transaction observation
- 증거: producer and consumer build identifiers; actual target environment; positive and negative interoperability results; rollback or recovery outcome
- 상태: unverified

## COMP-12 · IF-12

- 경계: IF-12 — chain adapter / contracts / indexer → payment and market services
- 고정할 값: producerBuild, consumerBuild, chainId, chainFork, contractCodeHash, abiDigest, indexerDecoder, entryPointCodeHash, userOpProfile, tokenProfile
- 판정: exact tested tuple; explicit adapter/migration evidence required for any non-identical supported combination
- 불일치: reject affected new operation; show update/recovery path; preserve already submitted transaction observation
- 증거: producer and consumer build identifiers; actual target environment; positive and negative interoperability results; rollback or recovery outcome
- 상태: unverified

## COMP-13 · IF-13

- 경계: IF-13 — trusted worker / object gateway → app / travel services
- 고정할 값: producerBuild, consumerBuild, resultSchema, objectDigestProfile, sourceSchema, modelDigest, consentPolicy, gatewayProfile
- 판정: exact tested tuple; explicit adapter/migration evidence required for any non-identical supported combination
- 불일치: reject affected new operation; show update/recovery path; preserve already submitted transaction observation
- 증거: producer and consumer build identifiers; actual target environment; positive and negative interoperability results; rollback or recovery outcome
- 상태: unverified

## COMP-14 · IF-14

- 경계: IF-14 — rental coordinator / device reset service → user app / ops safe projection
- 고정할 값: producerBuild, consumerBuild, returnPolicy, resetEvidenceProfile, keyStoreSchema, credentialStoreSchema, claimProfile
- 판정: exact tested tuple; explicit adapter/migration evidence required for any non-identical supported combination
- 불일치: reject affected new operation; show update/recovery path; preserve already submitted transaction observation
- 증거: producer and consumer build identifiers; actual target environment; positive and negative interoperability results; rollback or recovery outcome
- 상태: unverified

## COMP-15 · fota_forward

- 경계: IF-05 — Zephyr update service / bootloader → RN update bridge
- 고정할 값: producerBuild, consumerBuild, fromFirmwareBuild, toFirmwareBuild, fromKeyStoreSchema, toKeyStoreSchema, partitionDigest, bootloaderCommit, migrationProfile
- 판정: 기존 FW→신규 FW; 기기 key/settings 보존 후 새 이미지 확인; exact tested tuple, different source/domain/role rejected
- 불일치: unsupported operation rejected; no weak-profile downgrade or blind-signing fallback; preserve safe recovery and existing chain observation
- 증거: fixed valid test vector; mutated source/domain/chain/role negative vector; redacted real device/app/verifier result; recovery outcome
- 상태: unverified

## COMP-16 · fota_rollback

- 경계: IF-05 — Zephyr update service / bootloader → RN update bridge
- 고정할 값: producerBuild, consumerBuild, failedFirmwareBuild, rollbackFirmwareBuild, fromKeyStoreSchema, rollbackReadableSchema, rollbackPolicy, partitionDigest
- 판정: 새 부팅 실패→복귀; 이전 firmware의 migrated store 읽기 가능성 또는 안전 복구; exact tested tuple, different source/domain/role rejected
- 불일치: unsupported operation rejected; no weak-profile downgrade or blind-signing fallback; preserve safe recovery and existing chain observation
- 증거: fixed valid test vector; mutated source/domain/chain/role negative vector; redacted real device/app/verifier result; recovery outcome
- 상태: unverified

## COMP-17 · old_app_new_firmware

- 경계: IF-05 — Zephyr update service / bootloader → RN update bridge
- 고정할 값: producerBuild, consumerBuild, oldAppBuild, newFirmwareBuild, nativeBridgeVersion, controlProfile, smpProfile, firmwareOfferSchema
- 판정: 구버전 앱→신규 FW; 권한 유지한 지원/거절·업데이트 안내; exact tested tuple, different source/domain/role rejected
- 불일치: unsupported operation rejected; no weak-profile downgrade or blind-signing fallback; preserve safe recovery and existing chain observation
- 증거: fixed valid test vector; mutated source/domain/chain/role negative vector; redacted real device/app/verifier result; recovery outcome
- 상태: unverified

## COMP-18 · ble_owner

- 경계: IF-03 — firmware session service → RN native bridge / kiosk / ranging peer
- 고정할 값: producerBuild, consumerBuild, ownerAppBuild, osBuild, nativeBridgeVersion, deviceFirmware, controlProfile, handshakeProfile, ownerScope, smpAccessPolicy
- 판정: owner 세션; 설정/키관리/FOTA의 정확한 허용 행위; exact tested tuple, different source/domain/role rejected
- 불일치: unsupported operation rejected; no weak-profile downgrade or blind-signing fallback; preserve safe recovery and existing chain observation
- 증거: fixed valid test vector; mutated source/domain/chain/role negative vector; redacted real device/app/verifier result; recovery outcome
- 상태: unverified

## COMP-19 · ble_payment_terminal

- 경계: IF-03 — firmware session service → RN native bridge / kiosk / ranging peer
- 고정할 값: producerBuild, consumerBuild, kioskBuild, osBuild, nativeBridgeVersion, deviceFirmware, controlProfile, handshakeProfile, attemptScope, smpAccessPolicy
- 판정: 결제 단말 세션; 서명검토 이외 키관리/FOTA 차단; exact tested tuple, different source/domain/role rejected
- 불일치: unsupported operation rejected; no weak-profile downgrade or blind-signing fallback; preserve safe recovery and existing chain observation
- 증거: fixed valid test vector; mutated source/domain/chain/role negative vector; redacted real device/app/verifier result; recovery outcome
- 상태: unverified

## COMP-20 · ble_ranging_peer

- 경계: IF-03 — firmware session service → RN native bridge / kiosk / ranging peer
- 고정할 값: producerBuild, consumerBuild, peerBoardRevision, peerFirmware, deviceFirmware, rasProfile, handshakeProfile, observationFreshness, roleScope
- 판정: 거리 peer; session-bound observation 외 지갑/업데이트 권한 금지; exact tested tuple, different source/domain/role rejected
- 불일치: unsupported operation rejected; no weak-profile downgrade or blind-signing fallback; preserve safe recovery and existing chain observation
- 증거: fixed valid test vector; mutated source/domain/chain/role negative vector; redacted real device/app/verifier result; recovery outcome
- 상태: unverified

## COMP-21 · signature_eoa_transaction

- 경계: IF-10 — signing orchestrator → app review / chain submitter
- 고정할 값: producerBuild, consumerBuild, curve, hashAlgorithm, transactionEncoding, chainId, reviewDecoder, deviceSignerProfile, validatorVersion
- 판정: EOA transaction 고정 입력의 digest·서명·복원 주소와 거래 bytes 일치; exact tested tuple, different source/domain/role rejected
- 불일치: unsupported operation rejected; no weak-profile downgrade or blind-signing fallback; preserve safe recovery and existing chain observation
- 증거: fixed valid test vector; mutated source/domain/chain/role negative vector; redacted real device/app/verifier result; recovery outcome
- 상태: unverified

## COMP-22 · signature_typed_data

- 경계: IF-10 — signing orchestrator → app review / chain submitter
- 고정할 값: producerBuild, consumerBuild, curve, hashAlgorithm, domainProfile, typedDataEncoding, reviewDecoder, signerProfile, verifierAddress, verifierCodeHash
- 판정: typed-data domain/source/chain binding과 표시 필드 검증; exact tested tuple, different source/domain/role rejected
- 불일치: unsupported operation rejected; no weak-profile downgrade or blind-signing fallback; preserve safe recovery and existing chain observation
- 증거: fixed valid test vector; mutated source/domain/chain/role negative vector; redacted real device/app/verifier result; recovery outcome
- 상태: unverified

## COMP-23 · signature_user_operation

- 경계: IF-10 — signing orchestrator → app review / chain submitter
- 고정할 값: producerBuild, consumerBuild, curve, hashAlgorithm, userOpPacking, userOpHashVersion, chainId, entryPointAddress, entryPointCodeHash, accountCodeHash, reviewDecoder
- 판정: UserOp hash·EntryPoint·계정 validator·개별 실행 상태 일치; exact tested tuple, different source/domain/role rejected
- 불일치: unsupported operation rejected; no weak-profile downgrade or blind-signing fallback; preserve safe recovery and existing chain observation
- 증거: fixed valid test vector; mutated source/domain/chain/role negative vector; redacted real device/app/verifier result; recovery outcome
- 상태: unverified

## COMP-24 · signature_passkey_assertion

- 경계: IF-06 — firmware CTAP authenticator → verified CTAP transport / RP client
- 고정할 값: producerBuild, consumerBuild, rpId, clientDataProfile, authenticatorDataProfile, coseAlgorithm, ctapTransport, uvPolicy, credentialStoreSchema
- 판정: RP/clientData/authenticatorData/서명과 UP/UV 상태 검증; exact tested tuple, different source/domain/role rejected
- 불일치: unsupported operation rejected; no weak-profile downgrade or blind-signing fallback; preserve safe recovery and existing chain observation
- 증거: fixed valid test vector; mutated source/domain/chain/role negative vector; redacted real device/app/verifier result; recovery outcome
- 상태: unverified

## COMP-25 · signature_did_proof

- 경계: IF-10 — signing orchestrator → app review / chain submitter
- 고정할 값: producerBuild, consumerBuild, credentialProfile, didMethod, proofSuite, statusProfile, issuerKeyVersion, verifierVersion, reviewDecoder
- 판정: credential 목적·발급자·proof suite·status/철회와 제시 범위 검증; exact tested tuple, different source/domain/role rejected
- 불일치: unsupported operation rejected; no weak-profile downgrade or blind-signing fallback; preserve safe recovery and existing chain observation
- 증거: fixed valid test vector; mutated source/domain/chain/role negative vector; redacted real device/app/verifier result; recovery outcome
- 상태: unverified

## COMP-26 · signature_x402_proof

- 경계: IF-10 — signing orchestrator → app review / chain submitter
- 고정할 값: producerBuild, consumerBuild, x402Version, mechanism, network, tokenAddress, tokenCodeHash, authorizationMethod, nonceProfile, signerProfile, settlementVerifier, gasPolicy, reviewDecoder
- 판정: payment requirement·토큰 authorization·nonce·settlement·gas payer 검증; exact tested tuple, different source/domain/role rejected
- 불일치: unsupported operation rejected; no weak-profile downgrade or blind-signing fallback; preserve safe recovery and existing chain observation
- 증거: fixed valid test vector; mutated source/domain/chain/role negative vector; redacted real device/app/verifier result; recovery outcome
- 상태: unverified


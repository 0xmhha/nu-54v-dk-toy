# W04–W06 isolated iOS entry smoke

The fixture writes synthetic board and enrollment metadata only to the dedicated `com.nu54renter.w0406qa` Keychain. It contains no key, real account or real BLE device. `OwnerRoutesSmoke` checks W04, W05, W06 phone replacement and board recovery entry screens of the installed P02 app. It does not simulate a board, physical PIN, independent display, backup completion, recovery proof or a real pairing. Host `w04W06OwnerUi.test.tsx` separately exercises synthetic-owner UI transitions on iOS/Android branches; `ownerSession.test.ts` and P01 C `owner` tests separately exercise authenticated-protocol and board-core decisions. Do not treat those as a native end-to-end physical-device result.

2026-11-21: on isolated simulator `DA49352F-9CEE-4455-A394-A357C94AE7FD` (iPhone 17, iOS 27.0), the latest Release app and synthetic fixture produced `/private/tmp/nu54-w0406-entry.xcresult`: Apple summary reports one test passed, none skipped; duration 30.369 s. Four attached screenshots show the owner, backup, phone-replacement and board-recovery entry screens. The fixture's first launch attempt used the product Info.plist and failed before process start; the purpose-built fixture Info.plist in this directory fixed that setup issue. A green fixture screen showed successful Keychain insertion before the candidate app was installed over it. No BLE scan or device mutation occurred.

After the navigation fix, `/private/tmp/nu54-w0406-navigation.xcresult` reports one pass. The W05 and W06 entry screenshots show the instructions and both actions without the global navigation controls; the test also checks that navigation returns on leaving those routes. This does not establish VoiceOver focus on a device or a completed board operation.

Reproduction uses a disposable simulator and a dedicated build. Build P02 Release for `com.nu54renter.w0406qa` with `CODE_SIGNING_ALLOWED=YES CODE_SIGN_IDENTITY=-`; compile `fixture/Fixture.swift` as module `NU54Fixture` for the simulator, embedding the built app's `NU54Renter.app-Simulated.xcent` in `__TEXT,__entitlements`; copy `fixture/Info.plist` and name the executable `NU54Fixture`, then ad-hoc sign and install it. Launch it and check the green status, terminate it, and install the candidate over the same bundle. Run `xcodebuild test -project scripts/ios/w0406-smoke/SceneSmoke.xcodeproj -scheme SceneSmoke -destination 'platform=iOS Simulator,id=<disposable-id>' -derivedDataPath <new-dir> -resultBundlePath <new-result.xcresult> -collect-test-diagnostics never -only-testing:SceneSmoke/OwnerRoutesSmoke/testOwnerBackupAndRecoveryEntrances`. Confirm that the Apple test summary reports one pass.

## W04 owner registration and interrupted result

Run on a booted disposable iOS simulator with Xcode, the P02 pods, CMake, and OpenSSL 3 installed. Pass an unused absolute directory for evidence.

```sh
make -C products/p02-user-app test-ios-owner-registration \
  SIMULATOR_ID="$SIMULATOR_UUID" EVIDENCE_DIR=/private/tmp/nu54-w04-registration
```

The dedicated build uses bundle `com.nu54renter.w04qa`. Its fixture creates a fresh Keychain P-256 key per run, exports its public point to a short-lived test CA, and writes the resulting certificate chain and pinned board fingerprint to the app's protected profile. The fixture leaves the profile without a verified owner or receiving address. The regular iOS registration UI reads that key through Security and uses the product TLS 1.3 trust, exporter, wire2 framing, and challenge signature. The simulator's BLE adapter relays encrypted bytes to a local C process. The host uses the P01 owner and vault core, a file-backed vault, and simulated local confirmation.

The two candidates have the same advertised name and their board certificates share a CA. The app first selects the other candidate and rejects its distinct TLS public key. Its independent vault stays at generation zero. The first interruption on the enrolled candidate occurs after the phone proof verifies and before local confirmation. The second occurs after the host commits the owner generation and before it sends the response. The app restarts after each interruption. The operator then reads the authenticated status and repeats registration after another restart. The runner compares the transcript and a fresh vault read with the original board identity address and the phone's public-key fingerprint. Generation must remain at one and the receive action must stay locked. It saves the XCTest result and the host transcript, removes the installed QA app, and deletes the temporary host private keys. Use a disposable simulator for the temporary phone Keychain key.

The deployment verifier checks the ordinary iOS executable for QA key and relay markers. The QA build guard also rejects physical-device compilation when the QA flag is set. Physical BLE, independent display, board PIN input, and independent backup and recovery remain separate tests.

The first iOS native run reached the TLS 1.3 handshake but rejected its metadata. The older getter returned the legacy `SSLProtocol` value 10, which was compared against 0x0304 from `tls_protocol_version_t`. The iOS adapter now calls the getter for the current enum. Both owner transports bind the wire2 epoch to the negotiated ALPN context. The Android regression uses the production Conscrypt transport against the same P01 host. Run `bash products/p02-user-app/android/test_owner_tls.sh emulator-<serial>` with a disposable API 34 emulator. The pre-fix test established TLS and the C host rejected the first management frame. After the fix, the host returned exactly one STATUS and rejected a corrupted epoch. The phone separately rejected the wrong board pin.

On 2026-11-21, the isolated iPhone 17 simulator on iOS 27.0 passed the full W04 run in 78.517 seconds with zero failures or skips. The evidence is in `/private/tmp/nu54-w04-registration-10` on the runner. The other-board transcript contains only the listener startup and its TLS error log records rejection during certificate validation. The intended board's transcript records generation zero through the first interruption, one commit with its response dropped, and two authenticated generation-one reads. The ordinary P02 Release simulator executable passed the isolation check. The equivalent check rejected the QA executable and the iOS device SDK rejected the QA compilation flag.

## W05 independent backup and the first address

```sh
make -C products/p02-user-app test-ios-owner-backup \
  SIMULATOR_ID="$SIMULATOR_UUID" EVIDENCE_DIR=/private/tmp/nu54-w05
```

The regular phone opens operation 6 after registering against the P01 C host. The test closes one request and opens another. It observes pending status with receive and transfer locked. A separate accessory process uses its own certificate and storage key. Its two test board peers use separate P01 C HD stores, the source certificate pinned by the phone and an independent target certificate. The peers run the same C HD derivation, target and source receipt validation as P01. The accessory proves a target restore only after checking the target's commit digest. The runner exercises user cancellation, the wrong board, the wrong AES-GCM key, a bad commit acknowledgment and a corrupted accessory signature. It compares the address in both stores with the address displayed by the app after restart. Backup seed files are confined to the disposable host adapters and the accessory's encrypted capsule. The phone sees the request number, authenticated status and first address.

The normal iOS flow passed on 2026-11-21. Local evidence is in `/private/tmp/nu54-w05-native-01`. The earlier UI regression failed because completion read the backup flag and never persisted the first address. The app now rechecks the address against the same active owner and generation and saves it in the protected profile. A fresh authenticated status check can resume after an app interruption. The test adapter uses file-backed storage for the C hosts and a PTY for the independent panel. Real UART, protected hardware storage and board button/PIN actions remain field tests.

### W05 authenticated refusal and retry

The `backup-blocked` scenario uses the production P01 source-role and protected-slot eligibility checks to trigger an operation 6 refusal over pinned owner TLS. The host fixture starts without source capability and attaches it for retry. The test selects **다시 확인** and sends a fresh request to the same enrolled original. The runner requires the refusal before either successful request and checks the original vault identity, owner and generation at each event. The independent accessory then verifies the correct restore, and the app retains the original receiving address after restart.

Run `scripts/ios/w0406-smoke/run-backup-owned.sh recovery-chooser` for W06 entry on a fresh phone and from Settings. It opens all three choices in the signed app. The private test uses a pre-authentication P03 configuration with fixture-only origins for the Settings arm. It checks that navigation sends no board commands, leaves the fresh profile empty and the enrolled profile byte-identical, and preserves the existing direct recovery entries after restart. The standalone test removes its simulator and cloned build on exit.

Run `make -C products/p02-user-app test-ios-owner-backup-blocked SIMULATOR_ID="$SIMULATOR_UUID" EVIDENCE_DIR="$OWNED_OUTPUT"` from an isolated project clone. The caller owns the disposable simulator and the output directory. For a self-contained run, use `scripts/ios/w0406-smoke/run-backup-owned.sh backup-blocked` or `scripts/ios/w0406-smoke/run-backup-owned.sh backup`. The script builds from a private clone and removes its simulator, evidence and build caches on exit. Keep its compact console summary.

## W06 lost phone and the retained address

```sh
make -C products/p02-user-app test-ios-owner-replacement \
  SIMULATOR_ID="$SIMULATOR_UUID" EVIDENCE_DIR=/private/tmp/nu54-w06
```

The runner completes W05, keeps that board's C vault and HD slot, and provisions a second disposable Keychain identity and certificate in a fresh protected profile on the simulator. The first replacement receives a signature from an unregistered display key and closes with the old generation still active. The second uses the independently pinned display key and drops the response after the C owner commit. The phone reloads its protected checkpoint, reads the new generation and compares the exact prior address. The runner reopens the old phone identity and checks that it has lost management authority. It also compares the new phone's address attachment with the address independently derived in W05. The simulator uses sequential installations and a separate PTY display process. Field tests cover simultaneous physical phones, BLE bonds and the board's physical recovery mode.

A Jest regression exercised the fresh-phone profile where the previous phone's local metadata is unavailable. The checked address now comes from the protected replacement checkpoint and authenticated board response. Existing address metadata is also checked before replacement.

## W06 replacement board and the original receiving address

```sh
make -C products/p02-user-app test-ios-owner-board-restore \
  SIMULATOR_ID="$SIMULATOR_UUID" EVIDENCE_DIR=/private/tmp/nu54-w06-board-restore
```

After W05, the runner registers a separate C board with a new device identity and TLS key. In the normal recovery form it types all three independently supplied fingerprints, pastes the verifier document through iOS, and uses the visible **키보드 닫기** action before selecting the enrolled board. The verifier document has one screen-reader label. Inputs and otherwise identical scan candidates have separate test identifiers that work regardless of app language.

The accessory first withholds proof, then sends a wrong receipt and finally sends the bound receipt for the original request. The app keeps approval locked until it validates the receipt, confirms the new board's owner and generation, and reads its first address. The runner compares that address against both C vaults and rejects a new restore attempt on the already restored board. The fixture uses the ordinary P02 form and authenticated transport with file-backed C boards and a separate recovery accessory process. Physical board and real funds are outside this simulation.

In the isolated iOS run, W05 took 73.126 seconds and W06 took 104.719 seconds. The C host recorded one opened restore, one refusal, two receipt reads, and one authenticated first-address read. Both vaults retained the same receiving address. The aggregate evidence digest is `sha256:3df01f1a4a12a387caacf23bb2dc1b8f9824ed030813642e80a916d7eb47981e`.

The personal recovery form separates an unauthenticated scan candidate from a verified replacement. Choosing a candidate opens its pinned owner session and reads the authenticated target. The screen shows the enrolled device fingerprint and board key pin before offering a separate restore action. The owner session rechecks the device, owner and generation before opening the restore. Scan order only controls presentation and Bluetooth handles stay in memory.

The board runner reverses the two identically named candidates after enrollment. It selects the wrong board first and requires a failed TLS handshake, an unchanged wrong-board vault and zero application commands on that board. It then checks that only the enrolled replacement can open one restore and retain the original address.

The reversed-discovery iOS run passed on 2026-11-21. W05 took 72.222 seconds and W06 took 118.609 seconds. The wrong candidate received zero application commands and its vault stayed unchanged. The enrolled replacement opened one restore, refused one duplicate, read two receipts and read the authenticated first address once. Both vaults retained the exact original address. The sanitized aggregate digest is `sha256:9e7e735df2e3a836b70a77452183a7bb17cc0b15b9a8817145e7ac9d4e74d237`.

## W12 owner tools

```sh
scripts/ios/w0406-smoke/run-registration.sh "$SIMULATOR_UUID" /private/tmp/nu54-w12-tools tools
```

The registration runner keeps XCTest DerivedData in its own temporary directory and removes it after the test process group stops. Result bundles and logs stay in the selected evidence directory. For the default Release app build, cleanup removes Xcode compiler caches and keeps `build/Build/Products` and its logs. `NU54_W04_DERIVED_DATA` selects a caller-owned Release build directory, which cleanup leaves intact. Run `python3 -B scripts/ios/w0406-smoke/test_registration_workspace.py` to exercise cleanup without Xcode or a simulator.

The simulator first completes the personal-board backup fixture. It then opens Settings, verifies the enrolled board, reads its tools capability and makes explicit find and cancel requests over the normal authenticated OwnerSession. The local C transport checks one capability read, one locate request and one cancel while the original vault remains unchanged. The lamp timing and priority rules run separately against production `status_led.c` with a virtual work queue. The fixture removes the installed QA bundle and its transient build copies and keeps the XCTest results and synthetic command transcript.

For the supported P01 board, management opcode `70` verifies the LED tool and absence of board audio transport, `71` requests the 4.2-second find pattern, and `72` stops only that pattern. These are hexadecimal values. The request uses the pinned owner management channel, an active personal vault or authenticated HD root, and a lifetime of at most ten seconds. Active payment, pairing and PIN feedback take priority. Acknowledgment means the board accepted the request. The user checks for the visible signal on the board itself. The phone's accessible status action speaks short local UI text through handset TTS. It does not read messages, record audio, or perform board audio transfer. An older board that does not reply to capability discovery exposes no find action. No radio, lamps or audio output were measured on physical hardware.

## W27 protected limits

Run against a fresh disposable iOS simulator. The runner uses the existing simulator-only registration TLS adapter and a dedicated ad-hoc signed Release build.

```sh
bash scripts/ios/w0406-smoke/run-policy.sh "$SIMULATOR_UUID" /private/tmp/nu54-w27-policy
```

The runner binds the disposable phone certificate to a separate C owner vault. It initializes an experimental V1 policy through the P01 policy transition and PS adapter. The wallet fixture contains board-bound StableNet test token metadata with six decimals. The C host handles the authenticated owner session using TLS 1.3 and wire2. A separate process presents and checks the transition with the product independent-display validator and signs the exact statement with its own ephemeral P-256 key. A simulated device event supplies SW1 to the P01 local-approval core. The PS adapter uses file-backed storage. The simulator forwards encrypted transport via the existing loopback adapter.

The test uses Settings to read 10/30, reviews and cancels an increase to 12, rejects a corrupted independent signature, and explicitly re-reads after successful approval. The first activation observes the device clock one second before the persisted deadline. The second, after restarting the app, observes no device time. The final activation advances device time directly to the persisted deadline and reads 12/30 from the active revision. The full delay is 24 hours from the approval request's expiry in the experimental profile. The test accelerates device time without changing the phone clock and gives the host a 64-entry replay journal for the compressed run. The host session, simulated PS and simulated SW1 require physical qualification before deployment. The ordinary P01 profile and finality-only experimental mode do not provide V1 mutation.

On 2026-11-21, the signed P02 Release build passed the full W27 iPhone 17 Simulator run on iOS 27.0 in 134.750 seconds with zero failures or skips. The C transcript shows a rejected corrupt display signature, a single accepted change, two refused activations at deadline minus one second and with no trusted time, and one successful activation at the deadline. Readback shows revision 1 at 10/30 with the pending 12/30 candidate, then revision 2 at 12/30 with no pending change, including after another app restart. The runner removed the test app and ephemeral host keys. The disposable simulator and dedicated build caches were also removed. The test result, two screenshots and transcript are in `/private/tmp/nu54-w27-native-05` on the runner.

The updated W27 policy test also verifies the board-reported idle capability, opens W12, explicitly reads the enrolled board status, returns to a fresh read of the pending policy, and opens Settings from the separate applied view. The P02 bridge regression also drops a successful activation response and confirms that a subsequent device read matches the same pending candidate before treating it as applied. Local XCTest evidence is in `/private/tmp/nu54-w27-navigation-03`.

Pass `settings` as the third runner argument to test W27 Settings destinations separately. The installed app opens the existing recovery form, the credentials screen, the six-chain profile screen and travel consent. The default test deployment shows cloud authentication availability from its installed profile. It has no configured cloud account. The local XCTest result is in `/private/tmp/nu54-w27-settings-01`. Both runs removed the test app, temporary keys and dedicated build caches.
Use the two installed-app tests for revision 2 of the policy protocol.

```sh
bash scripts/ios/w0406-smoke/run-policy.sh "$SIMULATOR_UUID" /private/tmp/nu54-w27-recipients recipients
bash scripts/ios/w0406-smoke/run-policy.sh "$SIMULATOR_UUID" /private/tmp/nu54-w27-caps caps
```

The recipient test starts with two active addresses and uses the address book to build eight distinct candidates. It covers cancelling a draft, rejecting a ninth entry, replacing one member, refusing a signature over a different set, and recovering the committed candidate after loss of the proposal reply and an app restart. The cap test starts with eight active recipients and an outstanding reservation. It verifies the current and pending sets, then drops the activation reply and reads back the active revision. A C replay probe checks that the exact original reservation and the debit survived. Both scenario activations test the original deadline and the case with no trusted device time.

The baselines are signed through the same P-256 display validator and C adapter using a historical test clock. The app's requests use normal TLS and wall-clock binding. The independent process receives the private request code and reconstructs the statement from its separate fixture record and the board-generated 32-byte challenge. The test substitutes loopback transport for BLE, files for PSA storage, and a synthetic SW1 event. Use the default `policy` scenario to check the older board's singleton fallback. For repeated tests on identical signed app bits, supply `NU54_W27_DERIVED_DATA` and `NU54_W27_REUSE_BUILD=1` after building it once. The caller owns that external build directory.

## W12 running firmware in update preparation

Run the owned simulator case with `scripts/ios/w0406-smoke/run-backup-owned.sh running-firmware`. It requires MCUboot `imgtool` on PATH, or both `NU54_IMGTOOL_PYTHON` and `NU54_IMGTOOL_SCRIPT` pointing at an installed SDK. The runner generates a disposable test key, signs and verifies a counted MCUboot image, then deletes the key. The actual C owner host parses the protected version and counter from that image and hashes the same measured range. The normal app enters Settings → Device and update → Update preparation and uses pinned owner TLS to read the version and counter. The candidate and power remain unconfirmed, and the phone keeps preparation on hold. The test neither starts a firmware campaign nor applies an update. It uses a signed test image as input to the host adapter and leaves physical board verification separate.

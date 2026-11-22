# Android W10 owner review

Run `python3 -B scripts/android/w10-smoke/isolated.py /tmp/new-receipt.json` with the locked Node, Gradle and JDK, Android SDK, cached dependencies and `POSTGRES_BIN`. The runner builds and verifies an isolated signed Custom APK, boots a fresh API 34 ARM64 emulator, and directs test traffic through its own ADB server. It uses the normal customer registration, Android OwnerWallet and OwnerTls modules. The QA bundle substitutes loopback transports for the nearby and board radios. The native owner key requirement applies throughout.

The Android build and customer registration were exercised on the local API 34 ARM64 image. Its AndroidKeyStore produced a software-backed P-256 key (security level 0), so owner registration stopped under the production hardware policy. The sponsored review requires an environment with a supported hardware-backed owner key.

The test sends real C2 through P04 to P05 terminal mTLS. P05 issues and replays the original merchant-signed quote in its migrated PostgreSQL, using a short-lived fixture sponsorship authority and the production 090 pin. The separate regular opsd test covers a signed deployment and sponsorship authority on local chain 8283. The device checks the proof signatures and displays the item, quantity, amount, recipient and merchant gas responsibility. The C host checks `0x75` and presents the exact signed quote to the test display. The test sends SW2 after observing that review. It requires a durable C refusal, the same order on re-query and zero approvals accepted, payments or relay attempts.

A passing run retains the requested receipt. The APK, source copy, C host, emulator, PostgreSQL and private build caches belong to the run. Physical BLE, physical buttons and actual payment have separate acceptance checks.

## Renter decision fixture

Run `isolated.py --prepare-renter /tmp/new-build-receipt.json` to compile and verify a signed isolated Custom APK with the renter transport overlay. This stage removes its APK and caches after recording the digest and starts no PostgreSQL or emulator. The older owner scenario uses the original overlay and enrollment.

The installed renter scenario uses native customer registration and the existing P05 mTLS route. It uses one ordinary menu sale addressed by a fresh public QR locator for SW1 and a separate original menu sale opened from the same user's W09 list for SW2. Both sales use a currently accepted RENTER handoff, v2 generation and funded escrow from the signed local settlement fixture. The P04 relay runs in the test bundle. Its terminal-to-board route uses the private `renter-board.mts` process and the compiled C test peer over actual C2 TLS. The C peer waits until its W10R is received and verifies the separate N54D display proof of the same quote before the fixture supplies SW1 or SW2. Neither the customer credential nor the payer key is changed by the overlay.

The integration runner must observe the native app's verified W10R display before supplying either switch. It must assert that W10R and SW2 cause zero submissions, payments and relay attempts. For SW1, before tapping submit, it must assert zero submissions and arm loss of a successful originating C3 response. It then waits for the original relay hash, reopens the same sale after restarting the app, mines and finalizes on the isolated chain and checks the same hash and receipt. Exactly one admission, one relay attempt and one settlement are required. The merchant sponsor pays the gas and only the registered renter's generation escrow funds the principal.

Run `isolated.py --renter /tmp/new-installed-receipt.json` for the combined installation. The preparation mode and the host C2-to-P05 regression can run independently. An installed renter receipt requires the combined native app, C board host and chain run.

## Diagnosing a failed installed run

The run workspace is removed on exit. Set `NU54_W10_DIAGNOSTICS=/absolute/dir` to keep, on failure, a screenshot, the full accessibility hierarchy and the emulator logcat in that directory. The runner also prints the visible screen nodes (`SCREEN`), app errors (`SYSLOG`, `RNLOG`) and the fixture relay failures to the installed review log. Set `NU54_W10_EMULATOR_GPU` (for example `host`) to compare the emulator's software renderer with the host GPU when a screen stays blank after the app returns from the clipboard helper.

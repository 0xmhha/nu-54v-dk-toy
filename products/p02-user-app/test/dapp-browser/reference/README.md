# WalletConnect SignClient interoperability

This fixture uses the Apache-2.0 release of `@walletconnect/sign-client@2.21.8`. The package and all transitive archives are pinned in the npm lock. Run through `../wallet_connect_reference.py` to install in a disposable directory and restrict peer and wallet traffic to loopback. See the [mobile spec](../../../MOBILE-V2.md) for the headless acceptance criteria.

The official 2.21.8 release uses relay-auth 1.1.0 and removes the archival graph's StableLib Ed25519 and elliptic packages. The npm findings on SignClient, Core, Utils, relay-auth and query-string were downstream reports of their affected dependencies. [StableLib advisory](https://github.com/StableLib/stablelib/security/advisories/GHSA-x3ff-w252-2g7j) [Elliptic advisory](https://github.com/advisories/GHSA-848j-6mx2-7j84) The exact decoder override fixes CVE-2026-118664 and CVE-2026-45822. The scoped Viem WebSocket override fixes GHSA-58qx-3vcg-4xpx and GHSA-96hv-2xvq-fx4p. The test verifies CommonJS query parsing with the patched ESM decoder. CI audits this independent lock at low severity, including transitive dependencies. A clean npm install and a separate Trivy 0-finding scan covered this graph. WalletConnect releases using the successor Community License require separate authorization for this loopback setup.

## Installed iOS app

Run from the repository root with installed P02 dependencies and Xcode configured for a disposable simulator. Provide `SIM` (a booted disposable iOS simulator UUID), `CERT` (an ephemeral PEM certificate with IP SAN 127.0.0.1), and `KEY` (its matching private key). Use the test bundle identifier on that simulator only. Run the host P01 build before the fixture.

```sh
app=products/p02-user-app
qa="$PWD/$app/test/dapp-browser"
scratch=$(mktemp -d)

xcodebuild -workspace "$app/ios/NU54Renter.xcworkspace" -scheme NU54Renter \
  -configuration Release -sdk iphonesimulator -destination "id=$SIM" \
  -derivedDataPath "$scratch/app" CODE_SIGN_IDENTITY=- \
  PRODUCT_BUNDLE_IDENTIFIER=com.nu54renter.walletconnect.reference \
  BUNDLE_CONFIG="$qa/wallet-native.metro.config.js" build
xcrun simctl keychain "$SIM" add-root-cert "$CERT"
xcrun simctl install "$SIM" "$scratch/app/Build/Products/Release-iphonesimulator/NU54Renter.app"

python3 "$qa/wallet_connect_reference.py" --native "$CERT" "$KEY"
```

In another terminal, wait for `Native reference fixture ready`, then invoke the XCUITest against the installed app.

```sh
qa="$PWD/products/p02-user-app/test/dapp-browser"
xcodebuild test -project "$qa/reference/SceneSmoke.xcodeproj" -scheme SceneSmoke \
  -destination "id=$SIM" -derivedDataPath "$(mktemp -d)" -parallel-testing-enabled NO \
  -only-testing:SceneSmoke/WalletConnectNativeJourney/testInstalledWalletWithPinnedReferencePeer
```

Stop the fixture after the test. The test uses an installed React Native wallet, real iOS protection and reachability APIs, protected storage, HTTPS and WSS. The Metro test alias substitutes the unavailable simulator BLE and physical board interaction. A green run also requires the remote peer to find the same session after the app process was restarted and the app to display the same original hash and finalized RPC evidence. The first peer is intentionally absent from the test authority's registration, so a sanitized peer refusal on the server is expected.

## W15 disconnect and revoke

Use the same loopback QA configuration with the dedicated bundle
`com.nu54renter.wallet.w15qa`. Set `NU54_W15_QA=1` for the Xcode build
with `wallet-native.metro.config.js`. Build the P01 `test_device_evm` host target
and the P06 `TestUSDC.sol` artifact first. With an ephemeral trusted
simulator certificate, run `wallet_connect_reference.py --native-allowance "$CERT" "$KEY"`
and select `SceneSmoke/WalletConnectNativeJourney/testDisconnectAndRevokeExistingAllowance`.
Use a fresh fixture on the isolated W15 QA port 18483. Stop the fixture after XCTest.

The pinned peer verifies zero active sessions and a denied request after the
wallet disconnects. The separate allowance remains 42 with zero owner
transactions during disconnection. The tested app cancels one revocation
review, approves one op14 through the real C gateway with host substitutes,
submits one transaction and restarts. It displays 0 only after looking up the
same hash and reading the finalized allowance. The fixture emits counts and
quantities without printing the ephemeral owner or authority credentials.

## W14 bounded allowance and W15 tracking

Run `bash products/p02-user-app/test/dapp-browser/run_w14_installed.sh /absolute/path/to/evidence`
from the repository root with P02 dependencies and CocoaPods installed, and the
P06 TestUSDC artifact available. The script creates a disposable simulator and
temporarily refreshes the generated React Native iOS bindings. Run it serially
with other iOS builds in the same checkout. Set `W14_SIM_RUNTIME` and
`W14_SIM_DEVICE` when the default simulator runtime is unavailable.
The runner invokes the loopback fixture with `--native-positive-allowance`
and selects `SceneSmoke/WalletConnectNativeJourney/testInstalledWalletGrantsAndRestoresBoundedAllowance`.
The pinned SignClient sends an over-cap request, a valid request the user
declines, a request the user ends by viewing the connection, and a fresh
approved request after the wallet resumes the same session. Viewing the
connection produces zero board approvals and submissions. The app returns the signed transaction hash,
opens tracking, and queries the same hash after restarting. The fixture holds
the original in the Anvil mempool until the first lookup and then mines it.
The second lookup checks the canonical receipt and finalized allowance.
The runner serializes use of loopback port 18483 across checkouts with a
host-wide advisory lease. It refuses to start if another process is listening.
After verifying the disposable simulator and listener were removed, it atomically
publishes an immutable `w14-<run-id>` directory containing only filtered test
results, bundle signatures and source digests. Failed runs leave prior receipts
in place. Run the isolated lease, interruption and receipt tests with
`python3 -m unittest discover -s products/p02-user-app/test/dapp-browser -p test_run_w14_installed.py`
in an environment that allows loopback sockets.

## W14 login review and result

Run `bash products/p02-user-app/test/dapp-browser/run_w14_installed.sh /absolute/path/to/evidence w14-login`.
Use a local Python installation with `cryptography`, `websockets` and
`Crypto`, plus installed Xcode dependencies, CMake, Anvil and Cast. This
mode uses the repository's loopback peer and installs no Python or npm
packages. It builds the test app in a disposable source copy.

The P02 app receives an external WalletConnect request and displays the
origin, account, chain, expiry, nonce and SIWE message before allowing
a board approval. The peer first checks rejection of a mismatched and an
expired request. XCTest declines one live request and approves a distinct
challenge with nonce `8abc0023`. The peer verifies the returned EIP-191
signature against the original owner and exact message, rejects reuse,
and checks the disposable Anvil nonce and balance. The receipt requires
one host P01 login approval and zero submissions or on-chain approvals.

Use `w14-browser-login` for the built-in WebView route. The fixture serves
the browser page and SIWE endpoints on loopback without starting a
WalletConnect relay. The page reads the approved account and chain through
EIP-1193, shows the first rejection, and sends the next signature to the
local verifier. Before the app closes the WebView, the same page rejects a
replay and rechecks its account and chain. This mode keeps the same offline
tool requirements and the same private iOS build path as `w14-login`.

## W13 explore

Use the installed runner with a final `w13-start` argument. It opens the
wallet's explore screen, reviews the HTTPS origin, and visits connection
management, travel and the learning market before returning to the entered
site. It opens the local WebView only after account-sharing consent. It
then enters an independent peer's URI through the WalletConnect form and
waits for explicit scope approval. The pinned peer checks that account
reads work and spending is denied. The fixture leaves the Anvil nonce
unchanged and records zero board calls and submissions.

## W13 expired connection

Use the installed runner with a final `w13` argument. The registered peer
gets a short-lived proof from the loopback authority. XCTest opens the saved
connection from W15 and holds a supported native transfer review through
expiry. It presses the old approval if the system alert is still present.
In the same screen, the wallet requires a fresh scope review and consent
before a different peer can read the account. After restart, only the new
session appears in W15 and it can be resumed for account reads. The receipt
checks zero board calls and submissions and an unchanged Anvil nonce and
balance.

## W15 session management

Use the same runner with a final `w15` argument. It starts the pinned local
relay with two distinct peers, each of which the installed wallet authorizes
for account and chain reads. The test opens W15, verifies both connection
identities and the existing token allowance, and disconnects the first peer.
The second peer answers a new account query after resume. The test cancels
the first disconnect-all prompt, confirms the next one, and verifies that the
remaining DApp is disconnected. Anvil confirms the original allowance and
owner nonce are unchanged, with zero app signing or submission attempts.

## Native receive and ordinary send

Build the P01 `wallet_chain_node_oracle` host target. Use the same disposable certificate and Metro test config with `com.nu54renter.wallet.receiveqa` for the receive app. Start `w07NativeReceive.py "$CERT" "$KEY"` and select `SceneSmoke/WalletReceiveNativeJourney/testVerifyAndDecodeAllSixReceiveProfiles`. The normal owner screen checks C1 before showing the receive QR. The three non-EVM public replies come from the portable P01 HD library using the documented synthetic seed. XCTest independently decodes rendered QR pixels and checks network, the visible address and the XRP zero tag. The test opens the native system share sheet and invokes its local Copy action. Shared text equivalence is covered by the separate receive presentation and share contract tests.

For the ordinary EVM transfer app use `com.nu54renter.wallet.sendqa`, start `w28NativeSend.py CHAIN "$CERT" "$KEY"` with 8283, 11155111 or 84532, and select `SceneSmoke/WalletSendNativeJourney/testOrdinaryEvmDeclineApproveAndResume`. Use a fresh temporary fixture per chain. It binds only loopback, uses synthetic funded accounts and routes C1 op 9 to the real approval gateway with host substitutes for device services. Anvil verifies the signed raw transfer. Read the W28 host scope in the [mobile spec](../../../MOBILE-V2.md).

For the three non-EVM ordinary transfer lanes, use the same installed send QA app. Start a fresh fixture for each lane and run `SceneSmoke/WalletSendNativeJourney/testLocalChainDeclineApproveAndResume` once per fixture.

```sh
qa=products/p02-user-app/test/dapp-browser
cmake --build products/p01-device-firmware/out/host --target wallet_chain_node_oracle
python3 "$qa/w28NativeChains.py" 1 "$CERT" "$KEY" --bitcoind "$BITCOIND"
python3 "$qa/w28NativeChains.py" 2 "$CERT" "$KEY" --validator "$SOLANA_TEST_VALIDATOR"
python3 "$qa/w28NativeChains.py" 3 "$CERT" "$KEY"
```

Run those commands one at a time. Each uses scratch ledger data and binds the app to local TLS. Bitcoin Core 32.1 starts with peer networking disabled. Agave 4.4.1 binds its test validator to loopback. XRPL uses the pinned 3.3.0 standalone container with Docker networking disabled and must already be present locally. The host board substitute verifies that the approved bytes are the exact latest grant and that it can sign only once. Firmware derives the synthetic accounts and signs the approved raw message. The app uses its own platform-protected pending records and only then submits through ordinary production logic. The test reads the original identifier after process restart and validates finality with the respective reference ledger.


For the installed commissioned variant, pass `--commissioned` to those same fixture commands. Use a temporary certificate trusted only by the dedicated simulator, with SANs `DNS:wallet.localhost` and `IP:127.0.0.1`. The fixture signs a one-use N54G network profile for the canonical HTTPS route under `wallet.localhost`. The simulator uses the separately identified send-QA app, its native HTTPS connection and reserved localhost DNS behavior. The shipping transports refuse reserved localhost domains. Build the iOS send-QA bundle with `'GCC_PREPROCESSOR_DEFINITIONS=$(inherited) NU54_WALLET_SEND_QA=1'` (quote the argument to keep the Xcode variable literal). Only this flag plus the send-QA bundle identifier permits the exact test hostname and port. The registered route never contacts a public network. The adapter supplies the pinned public identity evidence while independent local Core, Agave and xrpld validate and confirm the actual raw bytes. The XRPL receiver uses an X-address with an embedded zero tag, and the signer uses the commissioned account namespace. These runs cover the normal app review, single spending attempt and restart in the QA fixture; they do not establish public service availability.

For W16 Signet read-only export, use the same isolated send-QA build and certificate. Start `w16NativeWatch.py "$CERT" "$KEY" --proof "$SCRATCH/public-proof.bin"` on loopback and select `SceneSmoke/WalletSendNativeJourney/testSignetWatchConsentDismissAndRestore`. The fixture supplies a synthetic host board, with no spending RPC. It checks one BIP84 op13 result and two independent branch-address replies. The installed app cancels the system share sheet and reopens its protected record after termination. To independently import the exact public result used in that run:

```sh
node products/p02-user-app/test/receiveQrOracle.mjs --watch --signet --proof "$SCRATCH/public-proof.bin" |
  python3 products/p02-user-app/test/verify_watch_export.py "$BITCOIN_BIN" --signet
```

The W28 pending variant adds `--hold-finality` to the commissioned fixture. BTC and XRP wait for the first restored-ID lookup before growing the chain. Agave uses a longer local tick schedule to let the simulator restart while the transaction can still be observed before finality. None synthesizes the transaction status.

For W17, start `run_bip48_node.py --signet-profile --two-inputs --native-bridge "$SCRATCH"` with the isolated Core binary and compiled wallet host oracle. After `original.psbt` is written, start `w17NativeMultisig.py "$CERT" "$KEY" --bridge "$SCRATCH"` and run `SceneSmoke/WalletSendNativeJourney/testSignetMultisigReviewExportAndRestore`. The native fixture uses the same funded original that independent Core later finalizes. Both partials are required. The test restores the original ID after app process restart and queries the result preserved from the local Core ledger. Use `--proof-json` on the Core runner to produce a synthetic raw vector for repeatable codec tests. The QA transport simulates the board trust boundary and only enrolls a local signed Signet profile.

For the installed document-picker variant, pass `--document NAME` to the same
fixture. Copy the funded original as-is to a `.txt` file for Base64, decode it
into a binary `.psbt` file, or encode its exact Base64 text into one PNG QR.
The fixture rejects any import that differs from the funded bytes. Use a fresh
Core bridge for each approval, since each successful case spends its original
prevouts. On iOS expose the QA app's own Documents folder via
`UIFileSharingEnabled` and `LSSupportsOpeningDocumentsInPlace` in the isolated
bundle, ad hoc re-sign it, and copy the one synthetic file into that folder.
The same XCTest enters through the system Files picker.

For Android, use an assigned emulator and build the separate
`com.nu54renter.wallet.sendqa` custom APK with
`-I ../test/dapp-browser/w17Android.init.gradle :app:assembleCustom` from the
`android` directory. Set `NU54_W17_ANDROID_QA` to an absolute scratch
directory. Put the disposable TLS **certificate only** at
`res/raw/qa_owner_ca.pem`. Add an overlay `AndroidManifest.xml` with
`<application android:networkSecurityConfig="@xml/qa_owner_network"/>`. Define
that resource with cleartext disabled, exact domains `127.0.0.1` and
`wallet.localhost`, and `@raw/qa_owner_ca` as the trust anchor for those
domains. Use separately generated custom signing material. Install only the
isolated package, then grant Bluetooth scan and connect runtime permissions to
that package if the emulator requires it. Run the fixture with `--listen 18451`
and run `w17AndroidProxy.py` on the host. Use device-scoped
`adb reverse tcp:18449 tcp:18451` and `adb reverse tcp:18450 tcp:18452`.
The exact-host proxy carries TLS through unchanged. Push one synthetic document
to Downloads, index only that URI with Android's media scanner if needed, and run `w17AndroidDocuments.py --adb PATH --device SERIAL
--document BASENAME --original "$SCRATCH/original-id"`. After each run remove
only the QA document and reverse rules created for the isolated journey.

The review fixture also exposes expected values for the two prevout scripts,
input and output totals, registered internal path and the conservative sat/vB
lower bound. For the invalid-path case, start `w17NativeMultisig.py` with
`--wrong-change --document NU54W17Invalid.psbt --document-out PATH`. Supply
that generated file to the same native test. The case expects the refusal state
and zero board signing calls. To check the separate review CLI and device host
oracle against an isolated true Signet descriptor engine, run
`python3 products/p01-device-firmware/test/run_bip48_display.py "$BITCOIN_BIN" "$ORACLE"`.

For W26 refusal tests, run the reference driver with
`--native-refusal "$CERT" "$KEY" --refusal-kind nft` and run
`SceneSmoke/WalletConnectNativeJourney/testInstalledWalletRefusesUnsupportedExternalMethod`
on the current reference QA app. Repeat with a fresh session and
`--refusal-kind batch`. The official peer sends the requests through the
same secure channel used by the production app. The two Anvil instances
each use the route's registered chain ID and are barred from submission.
For the Stake Program route, run
`w28NativeChains.py 2 "$CERT" "$KEY" --commissioned --staking-refusal --validator "$SOLANA_VALIDATOR"`
and run `SceneSmoke/WalletSendNativeJourney/testInstalledWalletRefusesUnsupportedStakeProgram`
on the send-QA app. The fixture couples the deliberately unsupported raw
instruction to the same enrolled authority, pinned route, and recent
blockhash used by the normal installed transfer entry.

W25 recovery uses `w25NativeDiscovery.py "$CERT" "$KEY" --bitcoind "$BITCOIND"`
and `SceneSmoke/WalletSendNativeJourney/testHistoricalSignetBranchesAndAccountNameRecovery`.
Use the isolated send-QA bundle with `NU54_W25_QA=1` for its loopback TLS route on port 18525. Build the P01 wallet host oracle first. The disconnected Core watch-only wallet
imports the two public descriptors for the disposable earlier account and
matches their scripts to the host board derivations. It funds a different
index on each branch and serves actual confirmed history. The QA adapter
supplies the Signet identity while the independent ledger stays on regtest.

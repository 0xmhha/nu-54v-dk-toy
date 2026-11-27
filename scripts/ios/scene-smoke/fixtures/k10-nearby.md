# K10 installed app and public reservation hold

Run the real five-minute reservation hold on an isolated migrated local PostgreSQL. The test sends C3 through one enrolled terminal's real mTLS session with 100 customer scopes and eight workers. It reads the same public reservation every 30 seconds. After 150 seconds it restocks one option through the operator API, checks the original rejection, and admits a separate customer. After 301 seconds the test checks the public and private sale counts, the original public expiry, and quote writes.

```sh
NU54_K10_LIVE_HOLD=1 P05_COMMERCE_TEST_DATABASE_URL="$DSN" \
  go test -count=1 -v -run '^TestPostgresOperatorCatalogRealTLS$' \
    ./products/p05-operations-backoffice/internal/commerce
```

For the installed test, build a dedicated P02 simulator Release and a SceneSmoke test bundle in disposable DerivedData. Use the normal workspace Metro configuration with one test-only resolver substitution. Resolve `../specs/NativeNearby.ts` from `CustomerOrders.tsx` to [k10-nearby-adapter.ts](k10-nearby-adapter.ts). The adapter connects two actual C2 frame endpoints in the app process. P02 uses the normal entry, enrollment, Keychain, order screens and order components. P04's relay logic forwards the framed command to a loopback fixture, which forwards C3 to the real mTLS P05 server. This fixture exercises a virtual nearby transport and a native iOS app installation.

Install the signed simulator app and set `NU54_K10_SIM`, `NU54_K10_APP`, and `NU54_K10_XCTESTRUN` to the owned simulator, bundle ID and test configuration respectively. Use a separate synthetic customer registration and disposable PostgreSQL schema.

```sh
NU54_K10_INSTALLED_RUNNER="$PWD/scripts/ios/scene-smoke/run-k10-installed.sh" \
  P05_COMMERCE_TEST_DATABASE_URL="$DSN" go test -count=1 -v \
    -run '^TestPostgresOperatorCatalogRealTLS$/^InstalledK10VirtualNearby$' \
    ./products/p05-operations-backoffice/internal/commerce
```

The installed fixture removes the last option after the app's final catalog read. It discards the first upstream response after the real P05 rejection. XCTest checks the pending and unknown states, terminates the app, restocks the option, and reopens the original rejection from the saved request. It then changes the cart and creates a separate preview reservation. The fixture checks one original denial, one fresh reservation, two keys, and zero quote writes. The caller owns and deletes the app build, simulator and PostgreSQL. The scene runner deletes its private test outputs.

Set `NU54_K10_SIGNED_REVIEW=1` in both the host fixture and the scene runner to exercise W10. Point the Metro resolver substitution to [k10-ios-nearby-adapter.ts](k10-ios-nearby-adapter.ts). It uses the native OwnerWallet storage API to provision the test payer's public address, generation and board fingerprint on the disposable simulator. The seed and test purchase contain no payer signing material.

The W10 fixture enrolls the server's presentation fingerprint independently of its TLS pin. The real P05 signer issues one quote for the selected 7.00 and 8.00 TestUSDC options. The test discards the first signed response, reopens the app, verifies the original order and review, and checks both option quantities and the 15.00 TestUSDC total. PostgreSQL verifies one issue command and outbox event, byte-identical replay and zero forwarding, relay or payment writes.

For the sponsored review and SW2 exercise, also set `NU54_W10_INSTALLED_BOARD=1` and point `NU54_W10_BOARD_HOST` and `NU54_W10_BOARD_VALIDATOR` at the host firmware test targets `owner_registration_host` and `test_install`. Use migration 090 and build for the existing simulator-only owner registration QA bundle `com.nu54renter.w04qa` with `NU54_OWNER_REGISTRATION_QA_FLAGS=-DNU54_OWNER_REGISTRATION_QA=1`. After installing, write a fresh UUID into `Documents/w04-key-scope` in that simulator app container. Create `Documents/w04-reverse-scan` to present the enrolled board first and set `NU54_W10_IOS_HOST_PORT_FILE` to the absolute path of the new `Documents/w04-host-port` in that container. Substitute [w10-owner-nearby.ts](../../../android/w10-smoke/w10-owner-nearby.ts) for NativeNearby from CustomerOrders. The production app screens, native owner identity and mutual TLS remain in the path.

The fixture uses disposable keys and a loopback stream adapter for radio transport. It publishes a fixture sponsorship authority for the exact profile and branch, exercises the production presentation and SQL pin, and routes the board's encrypted requests to the C host. The SW2 control exchanges the independent display challenge only after matching the original signed quote. Required test markers include one passing sponsored XCTest, its Go subtest, the C host's recorded refusal, and PostgreSQL counts of one issue command and zero payment or relay writes.

### Installed SW1 settlement acceptance

Use a separate order, customer, payer and sponsor for the installed approval case. The installed 15.00 TestUSDC refusal and the 2.00 TestUSDC host settlement are separate proofs. The installed approval must recover the original quote after a lost issuance response and verify its exact bytes through opcode 0x75 and the independent display challenge. After SW1, the app must display its board authorization and the original sale must still have zero submissions, payments and relay attempts.

Only the app's explicit submit action sends the approval to P04 and the P05 forwarder. Drop its response after durable admission. In the restarted app, the terminal lookup recovers the original sale and amount. The authenticated customer's payment-status lookup reads its confirmed transaction hash. Confirm one P07-indexed payment, one ledger entry and one fulfillment event, the exact beneficiary credit and customer escrow debit, and the actual gas spend by the enrolled sponsor. Verify the same hash and amount in the authorized finance order view. The installed runner requires the native test, the ledger assertions and the finance browser to pass in the same run.

The [maintained runner](../run-w10-sponsored.py) owns PostgreSQL, Anvil, the P07 indexer, and the C owner host. A [Go fixture producer](../../../../products/p05-operations-backoffice/internal/settlement/w10_installed_fixture_test.go) deploys to loopback 8283, funds a fresh synthetic payer, and writes its signed deployment, grant and scoped logins to a private mode-0600 file. The [installed test](../../../../products/p05-operations-backoffice/internal/commerce/menu_settlement_external_test.go) verifies that input, connects the installed menu to the production forwarder, and advances the isolated chain only for its original sale. Its finance read uses the enrolled finance identity at `/v2/finance/order` and checks P07 observation and confirmation heights.

With the pinned local contract and Go toolchain available, check the fixture producer, the separate 2.00 TestUSDC host finance control, and the [same finalizer used by the installed checkout](../../../../products/p05-operations-backoffice/test/settlement/w10fixture/finalize.go) without a simulator. The finalizer verifies the persisted broadcast for the exact hash before including it in one local block, verifies the receipt, then waits for P07 confirmation on that hash.

```sh
python3 -B scripts/ios/scene-smoke/run-w10-sponsored.py \
  --postgres-bin "$POSTGRES_BIN" --fixture-only
```

For the installed 15.00 TestUSDC SW1 case, use the signed QA app, scoped owner key, discovery order and SceneSmoke test bundle described above. Export `NU54_K10_SIM`, `NU54_K10_APP`, `NU54_K10_XCTESTRUN` and `NU54_W10_IOS_HOST_PORT_FILE` from that disposable simulator and run the same command without `--fixture-only`. The runner selects the SW1 XCTest. The caller owns the signed app build and simulator and removes them after the run. Keep the separately exercised SW2 case on its own fixture.

## W11 nearby pickup receipt

Use the same signed W10 SW1 app, host board, and private simulator inputs with
`run-w10-sponsored.py --w11-pickup`. This selects
`K10NearbySmoke/testSponsoredPickupReceipt`. The default SW1 and fixture-only
runs retain their existing coverage.

The app uses the original approved order and its customer identity. The local
settlement driver uses Anvil and the running indexer, then the real dispatcher
and receiver acknowledge that sale over mutual TLS. A separate operator sends
the regular pickup commands with exact retry keys. The fixture checks a valid
other customer gets 404. It reads the genuine pickup number and both timestamps
from the originating ledger so the test can compare them with the installed
display, return to the payment list, and reopen the same order after restarting
the app. This uses a host board and virtual nearby transport. The staff commands
exercise the authenticated API used by P04.

The receipt keeps chain inclusion separate from the time P05 booked the
finalized payment. A ready timestamp comes from the first authenticated ready
transition and survives collection. Historical transitions without a recorded
timestamp leave it absent. If the recorded payment time precedes canonical
chain inclusion, or the recorded ready time precedes payment confirmation, the
inconsistent additional timestamp is omitted and remains unconfirmed.

## W11 original payment awaiting confirmation

Use the same signed QA setup with `run-w10-sponsored.py --w11-pending`.
This selects `K10NearbySmoke/testSponsoredPendingReceipt`, independently of
the ready-pickup case. The test drops the durable admission reply, reads the
owner's original through C3 with the chain still unconfirmed, and checks the
registered branch, actual amount, and read-only controls. It reopens the same
order from the payment list and after restarting the app before allowing the
real settlement worker to finalize it. The finance browser checks the original
payment with fulfillment still pending.

For a host-only check, set `NU54_W11_PENDING_HOST=1` and select
`^TestLoopbackCustomerOrderReceipt$/^KioskNearbyPaidSeparateOriginal$/^W11NearbyPendingOriginal$`
with the existing isolated PostgreSQL and Anvil settlement fixture. The owner
can read an admitted original repeatedly before a transaction is signed or
broadcast. A second registered customer receives 404, and only one submission
exists. The enclosing case then confirms the same original and assigns its
pickup number.

The app takes the branch name from the validated nearby identity or the
existing authenticated C3 catalog identity. Missing or mismatched labels fall
back to the scoped branch reference. The read-only receipt follows the exact
C3 observation, so an overdue label cannot change another order. Amount and
asset must match the original review before a finalized response can clear the
checkpoint, and a terminal report from the relay requires the owner's C3
read. The JS renderer additionally exercises lookup failure and recovery.

## W11 original quote closed unpaid

Use the same disposable signed app and local board fixture with
`run-w10-sponsored.py --w11-unpaid`. The app submits one existing original
approval and reopens its checking status from the payment list and after
relaunch. The local fixture waits for the signed quote deadline, advances the
isolated Anvil chain to finalized post-deadline coverage, and runs the actual
P07 `prove-absence` command with that deployment's signed profile and exact
quote key. The normal P05 settlement worker applies that evidence. The app
must reopen the same archived original with its original amount and zero
payments after restart. The independently authenticated finance API checks
the same closed original and zero funds and fulfillment effects.

Migration 096 uses the existing private reservation, original signing
customer, and finalized absence record. Historic comparison authorization
alone does not qualify, and the customer still needs a live branch grant.
The projection stores no additional records or attributes. Its visibility
ends with the authorized grant or the original data's retention.

For the separately named host regression, set
`NU54_W11_UNPAID_HOST=1` and select
`^TestLoopbackCustomerOrderReceipt$/^(KioskNearbyPaidSeparateOriginal|W11NearbyClosedOriginal)$`
with the isolated P05/P07 PostgreSQL and Anvil fixture. The first child
retains the paid-order control, and the second independently proves the
terminal-created original without a payment, pickup, or relay.

### W12 maintenance during an original payment approval

Use the same owned signed iOS app and simulator inputs with `run-w10-sponsored.py --w12-busy --postgres-bin "$POSTGRES_BIN"`. This selects `testSponsoredBoardApprovalMaintenanceHold` against a disposable P05 PostgreSQL database. The C owner host accepts the opt-in only for the current enrolled owner, then emits progress on the same authenticated original request while it waits for the separate display decision. The test opens and closes the regular maintenance view without interrupting the approval. It checks:

- The active board notification holds firmware evidence reads and leaves the original order available.
- SW2 goes through the signed independent display fixture. The app then reads fresh owner status from the same board before resuming ordinary maintenance.
- The original C refusal exists and the P05 ledger records zero approvals, payments and relay attempts.

The scenario uses loopback BLE for the signed simulator app, real mTLS and the original C1 request. Actual radio, device buttons and firmware application remain separate device acceptance.

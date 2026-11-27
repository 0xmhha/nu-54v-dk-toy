# W12 credential and original payment fixture

The normal renter endpoint uses actual P05 and P07 services with isolated
PostgreSQL, Anvil and a private TLS authority. The W12 fixture builds a separate
opsd with a temporary test overlay and links the existing P01 session libraries
into a C host. The host uses synthetic protected storage and its host clock. Each request starts
a separate C process and reconstructs the same binding, original issuer grant
and board key, including the requested frozen state. This tests the same
generation scope without claiming one persistent physical vault across phases.
Its credential install, journal freeze, and board receipt use the P01 core.
The independent fixture issuer signs the registered generation term once.
The test connection authenticates with the pinned audience certificate distinct
from the installed renter. The dormant production collector setting stays unset.

Run the host test from the repository root with the pinned Forge/Anvil tools:

```sh
P05_O09_W12_CREDENTIAL=1 \
P05_O09_RENTER_V1_ORIGIN=1 P05_O09_RENTER_ASSESSMENT=mixed \
python3 -B products/p07-indexer/test/generation-v1.py --combined-p05 \
  --pg-bin /path/to/pg/bin --solc /path/to/solc-0.8.39 \
  --forge /path/to/forge-1.5.1 --anvil /path/to/anvil-1.5.1
```

To stage the signed iOS test with private React Native generated bindings
without creating a simulator, run:

```sh
bash scripts/ios/scene-smoke/fixtures/w12/run-owned.sh --prepare
```

Check the installed prerequisites without starting a simulator or database:

```sh
bash scripts/ios/scene-smoke/fixtures/w12/run-owned.sh --preflight
```

Forge and Anvil must be version 1.5.1. The launcher resolves them on
`PATH` unless `W12_FORGE` or `W12_ANVIL` supplies an executable. It checks
the PostgreSQL tools, pinned Solidity version, existing Pods/workspaces and
SceneSmoke sources before staging. It then builds and validates the generated
SceneSmoke `.xctestrun` before the Release app build.

After the iOS test slot is available, repeat staging and run it with:

```sh
bash scripts/ios/scene-smoke/fixtures/w12/run-owned.sh --install
```

The launch builds the ordinary P02 Release workspace with an ad-hoc QA
signature and a unique bundle ID, builds SceneSmoke for the new simulator,
and starts the mixed-origin credential fixture described above. It owns the
P02 source copy, generated bindings, DerivedData, Metro cache, simulator,
and fixture processes until teardown. Installed package dependencies are
read-only inputs. `W12_RUNTIME`, `W12_DEVICE_TYPE`, `W12_PG_BIN`,
`W12_SOLC`, `W12_FORGE` and `W12_ANVIL` may select the corresponding
local tools or simulator runtime. A macOS sandbox must allow the local Xcode
signing and CoreSimulator services, PostgreSQL shared memory and loopback
listeners.

Set `P05_O09_W12_INSTALL_TIMING=1` on the host command to also exercise
the installation schedule without building or launching the app. The fixture
confirms N54G near enrollment, within its 120-second signature admission
window. The preexisting settled payment is signed for the original contract
window, and the still-open payment is reserved within its own 120-second
window later in the run.

The optional `P05_O09_W12_INSTALLED_RUNNER` selects the signed installation test
instead of the host-only client. Use the absolute path to
`scripts/ios/scene-smoke/run-w12-installed.sh` and set `NU54_W12_SIM`,
`NU54_W12_APP` and `NU54_W12_XCTESTRUN` for the caller-owned simulator and
signed builds. The controller generates the enrollment token and supplies the
customer certificate only after receiving the app's public key. The test first
accepts the handover, checks separately rendered credential and evidence
deadlines against the exact UTC pair emitted by the authenticated response,
and checks visibility shortly before and after evidence expiry without a
refresh.
It then freezes the same generation, opens the genuine linked v1 payment,
restarts and verifies denial after revoke. The frozen board host refuses a new
credential receipt. The installed invocation requires its own simulator slot.

The runner keeps XCTest logs and results inside its disposable work directory,
prints aggregate status and removes only the generated adjacent `.xctestrun`.
Run `python3 -B scripts/ios/scene-smoke/fixtures/w12/test_runner.py` to check
the argument, path, log and cleanup behavior without Xcode or a simulator.

For the separate slot-open scenario, run the same command with
`--install-slot` in place of `--install`. The original `--install` keeps the
credential-only provider. Without a simulator, use the combined host command
with `P05_O09_W12_SLOT_OPEN=1` in addition to the credential, timing and mixed
origin flags above. The compiled C host provides the distinct slot receipt
over the test-only loopback bridge. The disposable opsd overlay feeds it into
the same authenticated 091 read and authenticates the exact dates sent in
the app response. Only the four public UTC timestamps reach the test
control endpoint.

The slot XCTest checks the two localized slot timestamps against the verified
UTC values, confirms open shortly before its deadline and unconfirmed without
a tap once it expires. A fresh read confirms the same still-valid credential
term. The original payment is checked after the same binding freezes, and
again following restart, before revocation. The host fixture reconstructs
the same protected state on each subprocess run. It exercises no hardware
persistence or production collector configuration.

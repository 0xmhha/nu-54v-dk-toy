# Private travel acceptance and recovery contract

Public place reads use the app's approved public deployment origin. They
need no customer profile or optional record consent. Commerce stays with
P05. Optional records use a separate P09 certificate grant, subject alias,
connection and private schema. A customer commerce certificate cannot open
private records. Database roles for public and private traffic have no
shared read privileges on private rows. No shared wallet address, payment
receipt, customer account ID or device position appears in P09's browse API.

## Consent and registration

The private grant binds one P09-scoped random subject and expiry to the
enrolled client certificate. Each of location sync, voice sync,
transcription/summary, external recommendation, public review and challenge
evidence has its own current decision and immutable decision receipts.
Each receipt commits the approved policy version, exact recipient, optional
object target, purpose revision, decision time and expiry. Collection and
work must verify the same current revision and its scope. Withdrawal and
dispatch/resolution serialize under that subject and purpose. Immutable
old decisions remain evidence without providing authority. Time limits are
the smaller of the policy limit and the user's declared limit.

Independent enrollment, backups and #21 privacy operations consume only
the contract below. The private app and provider routes use TLS 1.3 with
verified, separately enrolled principals. A public catalog operator cannot
perform disclosure, consent changes or erasure.

## Restore contract for #21

The restore authority uses a separate mTLS role and a physically separate
PostgreSQL erasure journal, outside P09 workload backups. It has no access
to content, keys, full payment receipts or user account identifiers. Its
export includes a restore namespace identifier plus a strictly increasing
high-water mark. Entries are ordered by sequence, and consist only of:

- A purpose fence with P09 scope alias, purpose, through-revision and cutoff.
- An exact object fence with type and immutable opaque UUID.
- Each fence has an expiry no earlier than the longest allowed backup and
  retry horizon. A purge also fences descendant record and job object keys.

The export contract is `GET /v1/restore/fences?after=<cursor>` on the
erasure service. An authenticated ordered response identifies its
namespace, high-water and page range. There is no client-selected trust
root. The restore role applies a complete verified batch using
`POST /v1/restore/apply` on a quarantined workload with namespace,
through-cursor and the corresponding entries. Live intake and provider
workers remain disabled until apply reaches the independently supplied
high-water. Repeated apply is idempotent. Older or reordered exports,
cursor gaps, unknown namespaces and missing expiry proofs fail closed.
Object fences only remove the selected item and dependent rows. Purpose
fences cannot remove independent records from a newer revision. The
server requires durable local and provider/device target evidence before
reporting deletion complete. A provider's cancel ACK alone cannot
substitute for terminal processing and deletion ACK. Backup expiry and
offline device work can keep the overall request pending.

A restored ordinary listener and the public review projection use the
scope's active-manifest lease and a live journal barrier before each
operation. The journal guard login executes only the cursor-match
function. The operation holds a shared journal writer lock until it
returns. A newer journal entry that has not reached the local workload
cursor closes the ordinary listener with HTTP 503. The P10 cutover holds
the same barrier while it commits the ingress generation. See the
[P10 recovery runbook](../p10-platform/recovery/README.md).

## Local acceptance

The first suite creates distinct synthetic subjects, changes policies
and revisions, checks denial by default and attempts stale replay and
cross-scope writes. A later suite races dispatch and withdrawal with a
separate loopback provider and verifies repeatable cleanup through
backup restore. Only synthetic place, receipt, text and audio fixtures
are used. The production provider, regulator and merchant decisions
remain independent release evidence.

## Run the local consent and review gate

Use a new database beginning with `nu54_p09_private` in a disposable
PostgreSQL cluster using a private temporary Unix socket. Export its DSN
as `P09_PRIVATE_TEST_DSN` and run the same owner migrations and process
entry points used by CI.

```sh
P09_MIGRATION_DATABASE_URL="$P09_PRIVATE_TEST_DSN" python3 products/p10-platform/database/migrate.py p09
recordsd="$(mktemp -d)/recordsd"
go build -o "$recordsd" ./products/p09-travel-ai/cmd/recordsd
P09_PRIVATE_TEST_BIN="$recordsd" go test -count=1 -v ./products/p09-travel-ai/internal/records -run '^TestPrivateConsentMutualTLS$'
```

This fixture enrolls rotating disposable certificates and separate DB
capabilities. It attempts a foreign consent read, mismatched recipient,
untargeted consent where a target is required, old revision, forged proof,
cross-role issuance and duplicate claims. A synthetic proof stays test-only.
A second fixture exercises the approved live-mode branch with disposable
proofs, manually authorizes a review to publish and checks that the
unauthenticated public review projection has no subject or proof ID.
A reorganization assertion hides the review and leaves the sealed
original for separate retention or erasure.

Runtime consent and review writes are served by `cmd/recordsd`. It uses
its own user, visit-ingest and restore database connections. Publication
of already-consented reviews is opt-in under `cmd/placesd` using
`P09_PUBLIC_REVIEW_DATABASE_URL` with a read-only review capability and
a separate `P09_REVIEW_KEY`. Asset and payment secrets do not enter
these P09 processes.

The independent P05 issuer is `cmd/visit-issuer`. It requires a private,
operator-approved subject/place/sale enrollment and independent P05
issuer and P07 reconciliation database capabilities. It emits a
domain-separated Ed25519 envelope with seven public-proof fields.
It never transports the source transaction hash, amount, wallet,
merchant keys or P05 subject to P09. The [booking local chain gate](BOOKING-V1.md)
builds and runs this actual executable against a finalized original
deposit, then checks a foreign payment owner is denied.

The signed journal response appends `signature`, padded base64 Ed25519
over `NU54-P09-fences-v1` plus byte zero followed by the canonical JSON
of the ordered page without `signature`. The protected restore applies
the exact unchanged page against an independently installed journal
public key. It rejects reordered or reserialized signed values when the
decoded canonical structure differs. Both cursors and generations in
the wire use decimal strings. The independent publication transaction
can hold `travel_erasure.guard_restore(namespace, high_water)`, which
takes the shared form of the same transaction lock used by appends.

The benefit issuer is exposed to independently enrolled issuer-scoped
mTLS clients at `GET /v1/issuer/benefit/{proof}`. It signs a short-lived,
monotone eligibility classification with a separate P09 signing key.
Enabling it requires `P09_BENEFIT_DATABASE_URL` and
`P09_BENEFIT_SIGNING_KEY`. P05's `cmd/visit-benefit` verifies the signed
envelope under the independently provisioned authority before accepting
it into an approved campaign. The same assertion is idempotent, a newer
source adjustment reverses the previous claim, and synthetic evidence
produces no effect. Benefit events do not modify the payment, refund,
receipt or financial journal. A deployed issuer must publish newer
assertions after reviewing changes to the active P07 source.

## Record processing, deletion and restoration

P02 has a separate device-held P-256 travel identity. Enrollment is
restricted to the approved `/v1/private` server with the supplied CA,
certificate chain and exact TLS leaf pin. It does not reuse a commerce
certificate. Record-only controls remain separate from public catalog
access. The microphone starts only after the user chooses capture and
the OS allows it. The optional short local clip uses app-protected
storage. A separate affirmative action and current voice consent are
required to share it. Transcription requires a current consent for the
exact stored recording and recipient. Production recorder behavior on
real hardware remains a separate acceptance step.

Run `cmd/processor` with its own `P09_PROCESSOR_DATABASE_URL` login, the
service's private content key and the recipient URL, CA, exact server
SPKI and separate P09 TLS client certificate. Register that login in
`travel_private.processing_recipient` under the matching approved
recipient. Worker dispatch checks the latest consent and source
generation transactionally. The recipient receives a single audio sample
and an opaque work handle, without a user, location or payment identifier.
On withdrawal a queued sample is canceled without transmission. For a
dispatched sample, cancellation and terminal acknowledgment precede a
distinct deletion acknowledgment. Failure stays pending and is retried
with the original handle.

Run `cmd/erase-worker` with restricted `P09_ERASURE_WORKLOAD_DATABASE_URL`
and separate `P09_ERASURE_WRITE_DATABASE_URL`, provisioning the independent
journal UUID before use. Selected erasure immediately fences only the
selected object, its dependent records and processing jobs. The worker
records the object fences in the independent journal before erasing the
workload ciphertext. Provider work can still be pending. Local acknowledgment records the level of its evidence, either subject-reported
or installation-reported. The receipt displays the scheduled backup expiry
separately. The restore path
applies the signed journal page before exposing recovered data. Tests
use two independent disposable databases, recover old ciphertext rows,
reject tampered pages and preserve an unrelated recording in the same
consent generation.

```sh
# Two freshly migrated P09 databases at private local Unix sockets.
P09_PRIVATE_TEST_DSN="$WORKLOAD_DSN" P09_ERASURE_TEST_DSN="$JOURNAL_DSN" \
  go test -count=1 -v ./products/p09-travel-ai/internal/records \
  -run 'TestSelectedErasureAndRestore|TestScopedProcessorRevocation|TestItinerarySourcesAndCAS'
```

## Generated itinerary and explicit apply

The private service reads a separately approved signed place snapshot,
including weekly local opening intervals, public sources and optional
planning costs. Configure `P09_APPROVED_CATALOG_DATABASE_URL` and
`P09_APPROVED_CATALOG_TRUST_KEY` with the read-only catalog authority.
`P09_LOCAL_AI_ENDPOINT` only accepts the local
`http://127.0.0.1:<port>/api/generate` API with an explicitly approved
`P09_LOCAL_AI_MODEL`. The ranker receives public place IDs and a coarse
category. It does not receive the person's identity, voice, budget,
chosen date or payment information. It runs without retained model
sessions. Server rules reject unknown or duplicate predicted places and
test the signed costs, opening ranges and travel intervals before
showing a proposed itinerary. Manual review remains available when the
ranker is unavailable. Inference itself has no booking or payment route.

Applying checks the same recommendation revision and the confirmation
for the displayed preview and expected itinerary version. A conflicting
version returns a compare result, and the authenticated user may read
the saved itinerary by its existing opaque ID. Planning cost is a public
estimate, separate from checkout or a quote.

Manual preview and ranked generation both require the current recommendation
decision for the authenticated subject. Generation rechecks consent before
submitting approved public candidates to the local ranker and again before
returning the result. Applying retains the database check for the same
decision, including retries of a previously confirmed request.

P02 keeps the review, comparison and saved-list state within the current
recommendation receipt and revision. Withdrawal, expiry, registration
replacement and leaving the screen clear that state. Returning to the app
and explicitly refreshing saved itineraries re-read the current decision
before revealing saved content. A previous session's response cannot reopen
its review or start a subsequent request. Re-accepting this purpose begins a
new session. These checks use the existing recommendation consent and its
expiry without collecting additional data.

Run the controlled late-response and foreground tests with
`pnpm --dir products/p02-user-app exec jest --runInBand T11Late TravelPrivate`.
The disposable mTLS and database suite exercises withdrawal during provider
processing, manual preview without current consent, and a stale confirmation
after re-acceptance.

On a dedicated iOS simulator, run the two fixture scripts from the
repository root. One serves the approved public catalog over local
TLS with an intentional tile failure, and the other issues the simulator
app's private identity certificate and routes to actual local
`gemma3:12b` when installed.

```sh
POSTGRES_BIN=/path/to/postgresql/bin python3 scripts/ios/scene-smoke/fixtures/p09_public_tls.py
POSTGRES_BIN=/path/to/postgresql/bin python3 scripts/ios/scene-smoke/fixtures/p09_private_tls.py
```

For the signed simulator candidate use the ordinary P02 Release scheme
with `NU54_P09_PUBLIC_ORIGIN=https://127.0.0.1:65140` and a disposable
bundle ID. The test uses `com.nu54renter.travel.itinerary-qa`. Add the
fixture's disposable public CA only to the test simulator, install the
candidate and run `-only-testing:SceneSmoke/PrivateTravelSceneSmoke` from
`scripts/ios/scene-smoke/SceneSmoke.xcodeproj`. The fixture control
enrollment endpoint is accessible only to the test harness. It is not
part of the installed app.

P02's **내 일정과 추천** separates itinerary review from recording, consent
and reviews. The proposal shows places in order, visit times in the
published timezone, opening hours, cost or an explicit unknown, and the
signed catalog source. Applying is a separate action. If the stored
revision has changed, the app opens the latest version for another review.
Saving never invokes checkout.

The existing authenticated records route accepts a distinct
`POST /v1/private/records` body `{"projection":"saved-itineraries"}`.
This projection returns up to 50 small summaries and uses the same current
recommendation consent and restore fences as each itinerary detail. The
legacy `GET /v1/private/records` response has its original record fields.
The projection cannot be combined with a record write.

The iOS microphone refusal check uses `SceneSmoke/TravelMicrophoneSmoke`
after resetting microphone permission for the disposable simulator bundle.
It denies the OS prompt before the recorder starts. Use isolated synthetic
audio for recording acceptance. Local recordings can be reopened from a dated
list. The device keeps the selected deletion target and retry key together so
an acknowledgement always refers to the same original recording, even after
reopening the app.

### Reopening selected deletion receipts

Use **삭제 영수증** in P02 travel records to reopen a completed request. The
authenticated GET on `/v1/private/erasure` shows only the request reference
and request time for up to 50 receipts per page belonging to that enrollment.
The next page uses the last opaque request reference. The
detail route gives journal, processor, device and scheduled backup status.
A scheduled backup date passing is displayed as elapsed time. Receipt
visibility follows the 90-day deletion-evidence limit. A delayed, authenticated
device acknowledgement can still finish its original erasure request.
Receipt history has no separate storage. Recovery fences retain their existing
lifecycle and continue to prevent deleted data from being served.

Run the opt-in native check with a disposable simulator and reusable P02
build directory. The check uses synthetic records, separate workload and
journal databases, and three distinct simulator enrollments.

```sh
bash scripts/ios/scene-smoke/run-travel-erasure.sh SIMULATOR_UDID EXISTING_DERIVED_DATA /tmp/nu54-t12-receipts
```

The disposable iOS test enrolls three app installations with distinct test
Keychain identities. A second installation on the same account reopens the
pending receipt without a local confirmation action. The original app
restarts and confirms its copy. An installation on a separate account sees
no receipt. The report retains XCTest counts and the database counts of the
surviving record, request, independent erasure journal, acknowledgment, and
request commitment.

The T12 runner takes a fresh output directory. It publishes the Xcode test
counts and the two database count checks. Registration uses the app's secure
input and normal native enrollment. The simulator paste item is local, expires
after 15 seconds and is cleared after the paste attempt. The runner keeps raw
Xcode output and the result bundle in a mode-0700 temporary directory and
removes it on exit or a handled interrupt.

### Publishing private service grants

The ordinary and quarantine `recordsd` and the independent journal `erasured`
read the current `-grants` pathname. Publish a complete validated JSON snapshot
by atomic replacement. A missing or malformed snapshot denies authorization.
Each operation binds its starting grant to the currently verified TLS 1.3
leaf and the earlier of the grant and certificate path expiry. An open TLS
connection does not extend a grant. Recheck against the current snapshot
before committing SQL, sending a recommendation to the local ranker, signing
an assertion or fence page, and releasing a private response. A change seen
while SQL is waiting causes rollback. The final authority read is the
authorization point. Revocation cannot recall a response or provider request
whose release point already passed.

### Installation confirmation for T12

Migration 018 links each new selected erasure to one request-scoped secret.
The app writes it to its local deletion journal before the initial request and
reuses it on retries and after restart. The server stores only its SHA-256
commitment in `travel_private.erasure_request.installation_proof_digest` in
the same transaction that creates the request. Deduplication cannot overwrite
an existing commitment or turn a legacy row into an installation-bound row.
A separate installation on the same account can read the permitted receipt.
It needs the original secret to advance installation confirmation.

The iOS journal uses the app's protected Keychain access group,
`WhenUnlockedThisDeviceOnly`, and no synchronization. Uninstalling does not
guarantee that iOS purges that Keychain group. If the original protected
journal survives a reinstall under the same access group, it can still
complete the same pending request. Android encrypts the local journal with
an app-scoped Android Keystore key and excludes app data from Auto Backup.
If the local journal is lost, installation confirmation stays pending.
Server receipt evidence reports what the account or original protected
installation journal acknowledged. Neither inference extends to other copies.
Late confirmation remains possible within the request lifetime without
reopening the 90-day receipt read window.

### Reviewing an itinerary changed on another installation

On a stale apply, P02 leaves the server revision intact and retains the
reviewed candidate in the current screen. Opening the latest version shows
both revisions, stops and budgets together. The user selects which candidate
stops to add. Existing stops and the latest budget stay in the new preview.
Identical stops cannot be selected twice. The server checks the selected
combination again against current opening hours, costs, and consent, then
issues a new confirmation for the latest base revision. The explicit Apply
action is the only write. Closing the comparison or restarting the app drops
the uncommitted selection. The persisted itinerary opens from the saved list.

The simulator replay uses real P09 mTLS for both enrolled clients and requires
one persisted itinerary at revision 3 after the explicit reapply. For a
disposable simulator and three unused consecutive loopback ports, run
`bash scripts/ios/scene-smoke/run-itinerary-conflict.sh SIMULATOR DERIVED RESULTS FIRST_PORT`.
The result folder retains only the XCTest counts and the database row count
and revision.

### T13 local request draft and consent

P02 can edit the date, visit hours, preference and budget in memory before
recommendation consent. Declining the purpose returns to the same draft.
When the traveler chooses to consent and generate, P02 freezes the reviewed
inputs and submits them once under the confirmed decision revision. P09
validates the visit times and budget using approved public sources and
passes only approved public candidate IDs and the coarse preference to the
local ranker. Canceling generation hides the result and leaves the draft
available for an explicit retry. Account changes, withdrawal and leaving
the screen clear the draft and prevent late requests from reopening it.
Applying a verified proposal remains a separate action.

Canceling discards any pending response on the device. If the request has already
reached P09, the server still checks consent before releasing its result. The
draft is not stored on the device or sent in the consent command.

The maintained iOS trial uses an isolated ranker that holds the first
response while the user cancels. It checks that public candidates and the
coarse preference are the only inputs at the ranker, confirms zero saved
rows until explicit apply, and confirms the saved revision after retry.

```bash
pnpm --dir products/p02-user-app exec jest --runInBand T13ConsentDraft T11Late TravelPrivate
bash scripts/ios/scene-smoke/run-itinerary-conflict.sh <sim-udid> <derived> <new-results> <first-loopback-port> --recommendation
```

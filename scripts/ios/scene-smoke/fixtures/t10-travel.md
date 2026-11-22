# T10 · 내 여행

The travel home opens the hub without reading the customer registration, the
private travel profile, or device permissions. Returning to exploration keeps
the current catalog and map selection. The hub displays booking guidance and
the optional nature of travel records.

The booking action resolves the current customer registration and opens the
existing branch- and customer-scoped booking view. It uses the same read
endpoint and checks the installed profile before and after a lookup. Review,
stamps, records and consent, itinerary, and challenge each open their own
existing destination. The general review entry uses the authenticated visit
options, without forcing the last explored place. Destination lookups may
read consent and records. The POST to `/visits` with projection `options` is a
read operation. No destination requests consent, generates recommendations,
records a visit, or starts a payment on entry.

Use the signed [T07 deposit-only runner](t07-travel.md) to verify T10 on the
actual P05 customer path. The test opens the same booking, checks the original
settled payment and receipt against P05, and then exercises T07 separately.
Its private P09 destinations use the unconfigured profile gates.

For the registered P09 path, choose three vacant consecutive loopback ports.
Pass the first port to the maintained runner. On macOS it creates and boots
its own simulator, builds the signed app, and starts a disposable P09
database and private TLS fixture.

```sh
bash scripts/ios/scene-smoke/run-travel-t10-owned.sh 65176
```

The installed app reads the actual empty visit history in review, stamps
and challenge, checks the recording and recommendation consent gates, and
returns to the same published catalog. PostgreSQL verifies zero consent,
record, review, claim, itinerary and erasure writes. The runner prints a
compact JSON summary with test counts, signed artifact digests and database
counts. It reclaims its simulator, builds, database and raw results on
success, failure or interruption. An unconfirmed shutdown or simulator
deletion reports the exact retained resource for recovery.

To supply and retain your own simulator, DerivedData and new result
directory, use the low-level interface:

```sh
bash scripts/ios/scene-smoke/run-itinerary-conflict.sh SIM_UUID DERIVED_DATA RESULT 65176 --my-trip
```

Exercise the cleanup paths without Xcode or a running simulator with
`python3 -B scripts/ios/scene-smoke/fixtures/test_t10_owned_cleanup.py`.

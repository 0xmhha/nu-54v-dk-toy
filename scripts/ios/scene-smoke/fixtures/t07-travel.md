# T07 customer cancellation acceptance

Use a disposable local PostgreSQL with P05 and P07 migrations and the pinned
Foundry tools. The existing settlement runner accepts
`NU54_TRAVEL_T07_HOST=full` to drive the authenticated customer and operations
through the full-paid fixture without a device. With `NU54_TRAVEL_T07_HOST=1`,
it checks deposit-only cancellation. Both run the named
`TestLoopbackSettlementEvidenceAndRefund/TravelBookingPaymentsAndPickup`
scenario. The default 30% deposit fixture still runs without either setting.

The owned signed-iOS entry is
`bash scripts/ios/scene-smoke/run-travel-t07-owned.sh --full ABS_DISPLAY_PYTHON`.
Omit `--full` for deposit-only. The display Python must have the existing
`coincurve` and `Crypto.Hash.keccak` dependencies. Use the pinned Forge, Anvil,
Cast, and local PostgreSQL on PATH. The launcher creates its own device and
disposable result. The separate `run-travel-uc1.sh --t07-full` entry accepts a
caller-owned device and result directory.

The full-paid fixture publishes 100% deposit and balance refunds before the
original cutoff. A fresh authenticated P05 booking read supplies the exact
cutoff, policy revisions, and no-show text to the private test comparison.
The installed customer reviews both original legs and confirms cancellation.
The preview barrier leaves the hold, both original payments and the refund
ledger untouched. Each refund uses the separate existing reserve, approve,
and submit principals. The customer reopens the same booking after the first
refund and after the second; P05 and the chain independently verify the
two original receipts and exactly one refund for each. No simulated click
stand-in is used in the installed path.

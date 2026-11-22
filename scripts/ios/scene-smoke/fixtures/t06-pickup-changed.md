# T06 pickup price and option change

A customer selects an option from the published menu for a specific pickup
window. At the request to create an order, the app reads the current menu
again. A change in price, asset, or selected menu version requires another review.
A stock shortfall also requires review when it affects the selected quantity. A missing option is removed from the selection with
its shortage disclosed. No other option substitutes for it. The next
confirmation rechecks the same selection against the current publication and
may require a further review. An existing admitted order uses its original
terms and recovery reference.

Run the existing pickup driver with a dedicated iOS simulator, privately
owned DerivedData and a new result directory. The changed-price mode builds
and installs only P02; the legacy kitchen DerivedData argument is unused.

```sh
bash scripts/ios/scene-smoke/run-travel-pickup.sh SIM_UUID P02_DERIVED P04_DERIVED RESULT --changed
```

The selector is
`SceneSmoke/TravelPickupSmoke/testChangedPickupRequiresNewConsent`. The signed
app uses the published P09 catalog and a P05 customer certificate. The test
fixture uses an operator grant and the actual P05 catalog publication
function to publish a new option label and price after selection. The app checks
the previous and current amounts. PostgreSQL asserts zero sales, seat
bookings, quotes, payments, or relay attempts before explicit renewed
consent. After consent it asserts one sale at the new amount and no
downstream effect. The existing pickup scenario remains separately runnable
without `--changed`. Remove the test device, build trees and the result
directory after extracting the test summary and signed-image digests.

Both modes build their signed apps from run-owned copies of the native
projects. The inputs from installed dependencies are shared for reads. The
two-app mode enrolls the phone and assigned kitchen under distinct mTLS
credentials and verifies each signed app before installation. Its loopback
fixture supplies synthetic settlement and fulfillment evidence to the real
PostgreSQL entry points. The separate production-chain acceptance scenario
provides the chain verification.

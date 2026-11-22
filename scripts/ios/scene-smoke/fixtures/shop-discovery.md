# K03 signed app discovery smoke

Use a disposable iOS simulator or Android AVD and an isolated signed P02 app with a fresh customer order journal. Enroll that app through the normal registration UI with a certificate for its public enrollment key and the independently supplied server and presentation fingerprints. Use an isolated P05 PostgreSQL with migration 061 and its loopback mTLS endpoint. Publish a stocked menu under the registered branch and set the branch's operating label. The second branch must differ from the enrolled branch. Keep iOS without a nearby enrollment for the unavailable-radio path. Give Android a nearby enrollment and leave its OS Nearby permission undecided, so the runner can deny the actual request.

Both runners read a test-only JSON file with these fields.

| Field | Input |
| --- | --- |
| `bundle` | Installed synthetic P02 app identifier beginning with `com.nu54renter.` |
| `branch` | Enrolled branch UUID |
| `otherBranch` | Different branch UUID |
| `name` | Exact registered storefront label |
| `menuItem` | Full accessible add-button label of a stocked product in the Korean menu |
| `originalSale` | Required offline. UUID of the single checkout created by the online test, read from P05 |
| `resumeURL` | Required offline. Loopback HTTP fixture control which restores the same P05 endpoint and returns that original UUID as plain text |

For the Android app's loopback C3 endpoint, forward its port on the selected AVD with `adb reverse`. Run the online phase while P05 is available.

```sh
scripts/ios/scene-smoke/run-shop-discovery.sh "$SIM" "$SIGNED_APP" online "$FIXTURE" "$RESULTS"
python3 scripts/android/subscription-smoke/shop-discovery.py "$AVD" online "$FIXTURE"
```

Use separate fixture files and isolated customer registrations when running both platforms. Verify that P05 recorded exactly one checkout for the tested customer. Add its sale ID to the corresponding fixture, then make the same mTLS endpoint unavailable while keeping PostgreSQL and the control endpoint running. The offline runner verifies the blocked menu, calls the control to restore P05, and reopens that exact original ID through the app's recovery action.

```sh
scripts/ios/scene-smoke/run-shop-discovery.sh "$SIM" "$SIGNED_APP" offline "$FIXTURE" "$RESULTS"
python3 scripts/android/subscription-smoke/shop-discovery.py "$AVD" offline "$FIXTURE"
```

Check that the same checkout is the only one in the ledger after recovery. The test code and fixture inputs stay outside the signed product. The UI smoke uses the OS entrance deep link or QR address entry. The native nearby transport labs exercise TLS and framed software transport separately.

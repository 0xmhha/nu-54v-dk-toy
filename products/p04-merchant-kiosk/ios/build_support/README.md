# CocoaPods Active Support security update

The CocoaPods build bundle pins Active Support 7.2.4. The locked CocoaPods Core 1.16.2 and the newer 1.17.0 Core both require a version below 8. Rails publishes the fix for GHSA-8474-xrp6-wh69 in 8.0.5.2 and 8.1.4.1. [CocoaPods Core constraints](https://rubygems.org/gems/cocoapods-core/versions/1.17.0/dependencies) · [Rails advisory](https://github.com/rails/rails/security/advisories/GHSA-8474-xrp6-wh69)

The same Trivy database reported five records at 7.2.3 and two at 7.2.4. The upgrade cleared CVE-2026-33176, CVE-2026-33169, and CVE-2026-33170. The remaining version-based records, CVE-2026-126080 and GHSA-8474-xrp6-wh69, identify the same Active Support defect. The scanner still reports both. [CVE mapping](https://github.com/advisories/GHSA-8474-xrp6-wh69)

| Context | VEX disposition | Basis |
|---|---|---|
| Locked CocoaPods process after kiosk Podfile evaluation | Fixed by backport | The Podfile activates the upstream literal-match behavior and the loader verifies the active method and both source fingerprints. |
| CocoaPods before kiosk Podfile evaluation and other Ruby tools | Under investigation | The backport applies during Podfile evaluation. This evidence makes no assertion about earlier or separate Ruby processes. |
| P04 iOS application runtime | Component outside the bundle | Active Support is a Ruby dependency of CocoaPods on the build host. The simulator output was compiled from the locked Pod graph. |

The backport adapts Rails commit `210ac22d446c8e2625b091e79256806b836eee20`. The guarded SHA-256 values appear in every successful locked Podfile evaluation. [Upstream fix](https://github.com/rails/rails/commit/210ac22d446c8e2625b091e79256806b836eee20)

Run with the locked Ruby bundle from the iOS directory.

```sh
bundle exec ruby build_support/test_active_support_backport.rb
bundle exec pod install --deployment
```

From the repository root, a native simulator smoke build is available with `python3 scripts/ios/release.py simulator custom /tmp/nu54-ios-smoke`. Validation covered the patched Podfile path and native simulator build without device installation.

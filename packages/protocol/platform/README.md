# NU54 mobile Protocol SDK 0.3.0

This package ships the reviewed native source for commerce HTTPS, Nearby
GATT and board payment GATT. The mobile library is versioned independently
of the portable v0.2.0 SDK.

Resolve `@nu54/protocol-mobile/package.json` through Node's normal package
resolution. Apply its `android/nu54-mobile.gradle` from the resolved
directory after the Android plugin. The script adds the native Kotlin
sources and pins Conscrypt. On iOS, add `Nu54ProtocolMobile` as a local pod
using the installed package directory, then include its headers from the
consuming app's Objective-C++ wrappers. The wrappers use the consumer's
generated React Native 0.87.1 TurboModule specs. The P02 and P04 wrappers
provide tested examples.

Install the candidate from a locally verified archive or a trusted
registry. Credentials, roots and app-specific provisioning are supplied by
the consuming product.

## Local candidate verification

From a NU54 checkout with the pinned toolchains, build and verify the separate
mobile candidate. The default target checks deterministic packing, installation
from the local archive and detached signatures. The platform flags add two
independent builds from the installed copy.

```sh
out=$(mktemp -d)/mobile
make -C products/p10-platform sdk-mobile-candidate SDK_OUT="$out" MOBILE_SDK_FLAGS="--android --apple"
python3 products/p10-platform/release/build_mobile_sdk.py --out "$out" --verify "$TRUSTED_MOBILE_SIGNER_SHA256"
```

Pin the signer fingerprint independently before verifying the signed receipt.
The receipt binds the SDK archive and the CycloneDX inventory. The verifier
rejects changed archive, manifest or SBOM bytes. The local test signer is
scoped to candidate evidence, and release promotion requires the appropriate
independent trust policy.

## Native virtual lab

The product APK and iOS app builds exercise SDK installation, code generation,
compile and link. The Apple lab uses macOS Network and Security with the same
TLS implementation as iOS. Its encrypted bytes travel between isolated
loopback bridges, using simulated ATT fragment sizes. The Android lab uses
an explicitly selected disposable AVD, the shipping Conscrypt implementation
and distinct keys in Android Keystore. It fragments the TLS stream in software.
Both labs verify two independent visitors, rejection cases and C2 authorization
with the host P01 C wire and signer. Physical GATT, Core Bluetooth runtime on
an iPhone or iPad, Secure Enclave execution, P01 firmware radio and product UI
acceptance require separate device tests.

Run the Apple lab with Node, OpenSSL and Xcode installed.

```sh
python3 packages/protocol/platform/test/test_nearby_apple.py --payment
```

Run the Android lab against a running disposable Android emulator. Set
`ANDROID_HOME` and `JAVA_HOME` to the installed SDK and JDK. Use an OpenSSL
binary with support for issuing a certificate from an external public key.

```sh
(cd products/p02-user-app/android && ./gradlew :app:assembleDebug :app:assembleDebugAndroidTest \
  --offline -Pnu54LabRunner=com.nu54renter.ble.NearbyTlsLab)
python3 packages/protocol/platform/test/test_nearby_android.py emulator-5558 \
  products/p02-user-app/android/app/build/outputs/apk/debug/app-debug.apk \
  products/p02-user-app/android/app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk
```

Use the emulator serial printed by `adb devices`. The lab generates fresh
test-only certificates and removes its temporary host credentials. It leaves
real branch, board and customer credentials untouched.

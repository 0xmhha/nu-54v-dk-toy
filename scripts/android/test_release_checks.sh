#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "$0")/../.." && pwd)"
case "${1:-}" in
  p02) app=p02-user-app; android="$repo/products/$app/android"; package=com.nu54renter; prefix=NU54_P02 ;;
  p04) app=p04-merchant-kiosk; android="$repo/products/$app/android"; package=com.nu54kiosk; prefix=NU54_P04 ;;
  *) echo "Usage: bash scripts/android/test_release.sh p02|p04" >&2; exit 2 ;;
esac
temp="$(mktemp -d)"
trap 'rm -rf "$temp"' EXIT
evidence_dir="${NU54_ANDROID_EVIDENCE_DIR:-$temp/evidence}"
mkdir -p "$evidence_dir"
evidence_dir="$(cd "$evidence_dir" && pwd)"

cd "$android"
unset "${prefix}_OFFICIAL_KEYSTORE" "${prefix}_OFFICIAL_STORE_PASSWORD" \
  "${prefix}_OFFICIAL_KEY_ALIAS" "${prefix}_OFFICIAL_KEY_PASSWORD"
if ./gradlew --no-daemon -Pkotlin.compiler.execution.strategy=in-process :app:assembleRelease > "$temp/official.log" 2>&1; then
  echo "Official Android build unexpectedly succeeded without credentials" >&2
  exit 1
fi
if ! grep -E 'Official Android upload certificate is not enrolled|keystore inputs' "$temp/official.log"; then
  cat "$temp/official.log" >&2
  exit 1
fi

# The Gradle check blocks the release variant exactly while the legal notices are not ready.
legal_ready=true
python3 -B "$repo/scripts/legal_notices.py" --release --app "$app" > "$temp/legal.log" 2>&1 || legal_ready=false
legal_gradle=true
./gradlew --no-daemon -Pkotlin.compiler.execution.strategy=in-process :app:validateReleaseLegalNotices > "$temp/legal-gradle.log" 2>&1 || legal_gradle=false
if [[ "$legal_ready" != "$legal_gradle" ]] || { ! $legal_ready && ! grep 'External distribution is blocked' "$temp/legal-gradle.log"; }; then
  cat "$temp/legal.log" "$temp/legal-gradle.log" >&2
  exit 1
fi
if ./gradlew --no-daemon :app:assembleRelease -x :app:validateReleaseLegalNotices > "$temp/legal-skipped.log" 2>&1 ||
    ! grep 'A release build needs validateReleaseLegalNotices' "$temp/legal-skipped.log"; then
  cat "$temp/legal-skipped.log" >&2
  exit 1
fi

export "${prefix}_CUSTOM_KEYSTORE=$temp/custom.p12"
export "${prefix}_CUSTOM_STORE_PASSWORD=android-test-only"
export "${prefix}_CUSTOM_KEY_ALIAS=signer"
export "${prefix}_CUSTOM_KEY_PASSWORD=android-test-only"
keytool -genkeypair -keyalg RSA -keysize 2048 -validity 2 -storetype PKCS12 \
  -keystore "$temp/custom.p12" -storepass android-test-only -alias signer -dname "CN=NU54 Release Test"
NU54_SCAN_VARIANT=customRuntimeClasspath NU54_SCAN_LOCK="$evidence_dir/custom-gradle.lockfile" \
  ./gradlew --no-daemon -Pkotlin.compiler.execution.strategy=in-process -PreactNativeArchitectures=arm64-v8a -I "$repo/scripts/android/runtime_graph.init.gradle" \
  :app:assembleCustom :app:bundleCustom :app:nu54PrintResolvedRuntime
python3 "$repo/scripts/android/check_runtime_graph.py" "$evidence_dir/custom-gradle.lockfile"
NU54_SCAN_VARIANT=releaseRuntimeClasspath NU54_SCAN_LOCK="$evidence_dir/release-gradle.lockfile" \
  ./gradlew --no-daemon -Pkotlin.compiler.execution.strategy=in-process -I "$repo/scripts/android/runtime_graph.init.gradle" :app:nu54PrintResolvedRuntime
python3 "$repo/scripts/android/check_runtime_graph.py" --variant releaseRuntimeClasspath "$evidence_dir/release-gradle.lockfile"
python3 -B "$repo/scripts/legal_notices.py" --check --app "$app" --resolved-runtime "$evidence_dir/release-gradle.lockfile"

verify=(python3 "$repo/scripts/android/verify_release.py" "$package" custom)
apk=app/build/outputs/apk/custom/app-custom.apk
bundle=app/build/outputs/bundle/custom/app-custom.aab
"${verify[@]}" "$apk" | tee "$evidence_dir/apk-signature.json"
"${verify[@]}" "$bundle" | tee "$evidence_dir/aab-signature.json"

build_tools="${ANDROID_HOME:-${ANDROID_SDK_ROOT:?Set ANDROID_HOME}}/build-tools/37.0.0"
"$build_tools/apksigner" sign --ks app/debug.keystore --ks-key-alias androiddebugkey \
  --ks-pass pass:android --key-pass pass:android --out "$temp/debug-signed.apk" "$apk"
if "${verify[@]}" "$temp/debug-signed.apk" > "$temp/debug-signed.log" 2>&1; then
  echo "Verifier accepted a debug-signed APK" >&2
  exit 1
fi
grep -i 'debug certificate' "$temp/debug-signed.log"

cp "$bundle" "$temp/tampered.aab"
python3 - "$temp/tampered.aab" <<'PY'
import sys
from zipfile import ZipFile
with ZipFile(sys.argv[1], "a") as bundle:
    bundle.writestr("base/assets/unsigned-test.txt", "unsigned")
PY
if "${verify[@]}" "$temp/tampered.aab" > "$temp/tampered.log" 2>&1; then
  echo "Verifier accepted an AAB with an unsigned payload entry" >&2
  exit 1
fi
grep 'Expected one signer for bundle entry' "$temp/tampered.log"

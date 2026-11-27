#!/bin/sh
set -eu
: "${ANDROID_HOME:?Set ANDROID_HOME to the local Android SDK}"
: "${1:?Provide the dedicated Android emulator serial}"
case "$1" in emulator-*) ;; *) echo "Use a dedicated local Android emulator" >&2; exit 1;; esac
here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
tools="$ANDROID_HOME/build-tools/${ANDROID_POS_BUILD_TOOLS:-34.0.0}"
platform="$ANDROID_HOME/platforms/android-34/android.jar"
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
mkdir "$scratch/classes"
javac --release 8 -cp "$platform" -d "$scratch/classes" "$here/Input.java"
"$tools/d8" --lib "$platform" --min-api 26 --output "$scratch" "$scratch/classes/local/poslab/Input.class"
"$tools/aapt" package -f -M "$here/AndroidManifest.xml" -I "$platform" -F "$scratch/unsigned.apk"
(cd "$scratch" && "$tools/aapt" add unsigned.apk classes.dex)
"$tools/zipalign" -f 4 "$scratch/unsigned.apk" "$scratch/aligned.apk"
# Disposable fixture-only signer. The trap discards the generated key.
keytool -genkeypair -keystore "$scratch/signing.p12" -alias android-pos-qa \
  -keyalg RSA -keysize 2048 -validity 1 -dname 'CN=POS Android QA' \
  -storepass android-pos-qa -keypass android-pos-qa >/dev/null 2>&1
"$tools/apksigner" sign --ks "$scratch/signing.p12" --ks-pass pass:android-pos-qa \
  --out "$scratch/input.apk" "$scratch/aligned.apk"
adb="$ANDROID_HOME/platform-tools/adb"
if "$adb" -s "$1" shell pm path local.poslab | grep -q '^package:'; then
  "$adb" -s "$1" uninstall local.poslab
fi
"$adb" -s "$1" install "$scratch/input.apk"

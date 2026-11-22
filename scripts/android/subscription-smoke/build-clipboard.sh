#!/usr/bin/env bash
set -euo pipefail
umask 077
if [[ $# != 2 ]]; then
  echo "usage: build-clipboard.sh ANDROID_BUILD_TOOLS_DIR OUTPUT_APK" >&2
  exit 2
fi
sdk=${ANDROID_SDK_ROOT:-${ANDROID_HOME:?set ANDROID_HOME or ANDROID_SDK_ROOT}}
tools=$(cd -- "$1" && pwd)
out=$2
java=${JAVA_HOME:+$JAVA_HOME/bin/}
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
scratch=$(mktemp -d)
trap 'rm -rf -- "$scratch"' EXIT
android="$sdk/platforms/${ANDROID_PLATFORM:-android-36}/android.jar"
"${java}javac" --release 8 -classpath "$android" -d "$scratch" "$here/Clip.java"
"$tools/d8" --lib "$android" --output "$scratch" "$scratch/org/nu54/qa/subscriptionclip/Clip.class"
"$tools/aapt" package -f -M "$here/AndroidManifest.xml" -I "$android" -F "$scratch/unsigned.apk"
(cd "$scratch" && "$tools/aapt" add unsigned.apk classes.dex)
"$tools/zipalign" -p -f 4 "$scratch/unsigned.apk" "$scratch/aligned.apk"
"${java}keytool" -genkeypair -keystore "$scratch/key.p12" -storepass android -keypass android \
  -alias android -keyalg RSA -keysize 2048 -validity 2 -dname CN=DisposableEmulatorClipboard
"$tools/apksigner" sign --ks "$scratch/key.p12" --ks-pass pass:android --out "$out" "$scratch/aligned.apk"

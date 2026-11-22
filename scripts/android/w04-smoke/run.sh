#!/usr/bin/env bash
set -euo pipefail
umask 077
[[ $# == 1 && "$1" =~ ^emulator-[0-9]+$ ]] || { echo 'usage: run.sh EMULATOR_SERIAL' >&2; exit 2; }
serial=$1
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
root=$(cd -- "$here/../../.." && pwd)
build=$(mktemp -d)
trap 'rm -rf -- "$build"' EXIT
cat > "$build/w04.gradle" <<'GRADLE'
gradle.beforeProject { project ->
    if (project.path != ':app' || project.rootProject.name != 'NU54Renter') return
    project.layout.buildDirectory.set(project.file(System.getenv('NU54_W04_PERMISSION_BUILD_DIR')))
    project.plugins.withId('com.android.application') {
        project.androidComponents.finalizeDsl { app ->
            app.buildTypes.getByName('custom').applicationIdSuffix = '.w04permissionqa'
        }
    }
}
GRADLE
export NU54_W04_PERMISSION_BUILD_DIR="$build/app-output"
export NU54_P02_CUSTOM_KEYSTORE="$build/w04.p12" NU54_P02_CUSTOM_KEY_ALIAS=w04qa
export NU54_P02_CUSTOM_STORE_PASSWORD=$(openssl rand -hex 24)
export NU54_P02_CUSTOM_KEY_PASSWORD="$NU54_P02_CUSTOM_STORE_PASSWORD"
"${JAVA_HOME:?Java is required}/bin/keytool" -genkeypair -alias w04qa -keyalg RSA -keysize 2048 -validity 2 \
  -dname 'CN=NU54 Android W04 permission test' -keystore "$NU54_P02_CUSTOM_KEYSTORE" \
  -storepass:env NU54_P02_CUSTOM_STORE_PASSWORD -keypass:env NU54_P02_CUSTOM_KEY_PASSWORD >/dev/null 2>&1
android="$root/products/p02-user-app/android"
(cd "$android" && ./gradlew --offline -I "$build/w04.gradle" -PreactNativeArchitectures=arm64-v8a :app:assembleCustom >&2)
unset NU54_P02_CUSTOM_STORE_PASSWORD NU54_P02_CUSTOM_KEY_PASSWORD
PYTHONDONTWRITEBYTECODE=1 python3 "$here/permissions.py" "$serial" "$NU54_W04_PERMISSION_BUILD_DIR/outputs/apk/custom/app-custom.apk"

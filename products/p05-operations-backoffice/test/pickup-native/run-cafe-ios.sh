#!/bin/bash
set -euo pipefail
umask 077
here=$(cd "$(dirname "$0")" && pwd)
if [[ ${1:-} == --owned ]]; then
  shift
  exec python3 -B "$here/run-cafe-owned.py" "$@"
fi
if [[ $# != 6 ]]; then
  echo "Usage: run-cafe-ios.sh --owned [--nearby] | prepare|verify LAB STAFF_SIM CUSTOMER_SIM STAFF_APP CUSTOMER_APP" >&2
  exit 2
fi
mode=$1
lab=$2
staff=$3
customer=$4
appStaff=$5
appCustomer=$6
[[ "$staff" != "$customer" ]]
repo=$(cd "$here/../../../.." && pwd)
harness="$repo/scripts/ios/scene-smoke"
[[ $(/usr/libexec/PlistBuddy -c 'Print CFBundleIdentifier' "$appStaff/Info.plist") == com.nu54kiosk.cafepickupqa ]]
[[ $(/usr/libexec/PlistBuddy -c 'Print CFBundleIdentifier' "$appCustomer/Info.plist") == com.nu54renter.cafepickupqa ]]
mkdir -p "$lab/staff" "$lab/customer"
if [[ "$mode" == prepare ]]; then
  for role in staff customer; do
    slot=operator
    sim=$staff
    app=$appStaff
    if [[ "$role" == customer ]]; then slot=customer; sim=$customer; app=$appCustomer; fi
    NU54_SUBSCRIPTION_COMMERCE_SLOT="$slot" bash "$harness/run-subscription-native.sh" "$sim" "$app" "$lab/$role/key" testCommerceEnrollmentPublicKey > "$lab/$role/key.log" 2>&1
    python3 - "$lab/$role" <<'PYKEY'
import base64,json,re,sys
from pathlib import Path
out=Path(sys.argv[1])
key=re.search(r'NU54_SUBSCRIPTION_C3_SPKI=([A-Za-z0-9+/=]+)',(out/'key.log').read_text())[1]
assert len(base64.b64decode(key,validate=True))==91
(out/'public-keys.json').write_text(json.dumps({'ios':key}))
PYKEY
  done
  exit
fi
[[ "$mode" == verify ]]
[[ ! -e "$lab/aborted" ]]
ok=0
trap 'if [[ "$ok" != 1 ]]; then touch "$lab/aborted"; fi' EXIT
for n in {1..100}; do
  test -s "$lab/staff.json" && test -s "$lab/customer.json" && break
  sleep 1
done
test -s "$lab/staff.json"
test -s "$lab/customer.json"
cases=$(python3 -B "$here/cafe_fixture.py" "$lab" "$here/CafePickupSmoke.swift.template" "${NU54_K05_NATIVE_VARIANT:-unknown}")
staffCase=${cases%%$'\n'*}
customerCase=${cases#*$'\n'}
cp -R "$harness/SceneSmoke.xcodeproj" "$lab/"
NU54_SUBSCRIPTION_COMMERCE_SLOT=operator NU54_SUBSCRIPTION_UI_FIXTURE="$(openssl base64 -A -in "$lab/staff.json")"   bash "$harness/run-subscription-native.sh" "$staff" "$appStaff" "$lab/staff/enroll" testNativeKioskStaffEnrollment > "$lab/staff/enroll.log" 2>&1 &
a=$!
NU54_SUBSCRIPTION_COMMERCE_SLOT=customer NU54_SUBSCRIPTION_UI_FIXTURE="$(openssl base64 -A -in "$lab/customer.json")"   bash "$harness/run-subscription-native.sh" "$customer" "$appCustomer" "$lab/customer/enroll" testEnrollNativePaymentReviewFixture > "$lab/customer/enroll.log" 2>&1 &
b=$!
xcodebuild build-for-testing -project "$lab/SceneSmoke.xcodeproj" -scheme SceneSmoke -destination "id=$staff" -derivedDataPath "$lab/UI" -parallel-testing-enabled NO -jobs 1 > "$lab/ui-build.log" 2>&1
wait "$a"
wait "$b"
runner=$(find "$lab/UI/Build/Products" -maxdepth 1 -name '*.xctestrun' -print -quit)
test -s "$runner"
xcodebuild test-without-building -xctestrun "$runner" -destination "id=$staff" -parallel-testing-enabled NO -collect-test-diagnostics never "-only-testing:SceneSmoke/CafePickupInstalledApps/$staffCase" -resultBundlePath "$lab/staff/ui.xcresult" > "$lab/staff/ui.log" 2>&1 &
a=$!
xcodebuild test-without-building -xctestrun "$runner" -destination "id=$customer" -parallel-testing-enabled NO -collect-test-diagnostics never "-only-testing:SceneSmoke/CafePickupInstalledApps/$customerCase" -resultBundlePath "$lab/customer/ui.xcresult" > "$lab/customer/ui.log" 2>&1 &
b=$!
wait "$a"
wait "$b"
ok=1

#!/usr/bin/env bash
# Run hosted production-module XCTest against a private copy of a signed simulator app.
set -euo pipefail
if [[ $# -lt 4 ]]; then
  echo 'usage: run-subscription-native.sh SIMULATOR_UUID SIGNED_APP OUTPUT_DIRECTORY TEST_METHOD [TEST_METHOD ...]' >&2
  exit 2
fi
sim=$1
application=$2
out=$3
shift 3
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
mkdir -p -- "$out"
out=$(cd -- "$out" && pwd)
if [[ -e "$out/Result.xcresult" ]]; then
  echo 'choose a new result directory' >&2
  exit 2
fi
source "$here/private-build.sh"
scene_prepare
mkdir -p "$scene_work/Products"
host="$scene_work/Products/$(basename -- "$application")"
ditto "$application" "$host"
exe=$(/usr/libexec/PlistBuddy -c 'Print CFBundleExecutable' "$host/Info.plist")
bundle=$(/usr/libexec/PlistBuddy -c 'Print CFBundleIdentifier' "$host/Info.plist")
ruby - "$scene_work/NativeAlerts.xcodeproj" "$here/SubscriptionAlertsNative.m" "$host/$exe" <<'RUBY'
require "xcodeproj"
path, source, host = ARGV
project = Xcodeproj::Project.new(path)
target = project.new_target(:unit_test_bundle, "SubscriptionAlertsNative", :ios, "17.0")
target.add_file_references([project.main_group.new_file(source)])
target.build_configurations.each do |config|
  config.build_settings.update({
    "GENERATE_INFOPLIST_FILE" => "YES",
    "PRODUCT_BUNDLE_IDENTIFIER" => "com.nu54.subscription-native-tests",
    "TEST_HOST" => host,
    "BUNDLE_LOADER" => "$(TEST_HOST)",
    "CODE_SIGNING_ALLOWED" => "NO",
    "ARCHS" => "arm64"
  })
end
project.save
RUBY
scene_run xcodebuild build -project "$scene_work/NativeAlerts.xcodeproj" -target SubscriptionAlertsNative \
  -sdk iphonesimulator -configuration Debug \
  OBJROOT="$scene_work/Intermediates" SYMROOT="$scene_work/Build" CONFIGURATION_BUILD_DIR="$scene_work/Products"
codesign --force --sign - "$host/PlugIns/SubscriptionAlertsNative.xctest"
codesign --force --sign - --preserve-metadata=identifier,entitlements "$host"
python3 - "$scene_work" "$(basename -- "$host")" "$bundle" <<'PY'
import os
import pathlib
import plistlib
import sys

directory, app_name, host_bundle = sys.argv[1:]
host = f"__TESTROOT__/Products/{app_name}"
platform = "__PLATFORMS__/iPhoneSimulator.platform/Developer"
fixture_environment = {
    k: os.environ[k]
    for k in ("NU54_SUBSCRIPTION_UI_FIXTURE", "NU54_SUBSCRIPTION_COMMERCE_SLOT", "NU54_SUBSCRIPTION_PREVIOUS_PROFILE", "NU54_SUBSCRIPTION_PRIOR_ORDER", "NU54_SUBSCRIPTION_MANUAL", "NU54_SUBSCRIPTION_DEVICE", "NU54_SUBSCRIPTION_DIRECT_INVOICE", "NU54_TRAVEL_OWNER_SCOPE")
    if k in os.environ
}
target = {
    "BlueprintName": "SubscriptionAlertsNative",
    "ProductModuleName": "SubscriptionAlertsNative",
    "TestHostPath": host,
    "TestHostBundleIdentifier": host_bundle,
    "TestBundlePath": "__TESTHOST__/PlugIns/SubscriptionAlertsNative.xctest",
    "IsAppHostedTestBundle": True,
    "DependentProductPaths": [host],
    "TestingEnvironmentVariables": {
        "DYLD_FRAMEWORK_PATH": f"__TESTROOT__/Products:{platform}/Library/Frameworks",
        "DYLD_LIBRARY_PATH": f"__TESTROOT__/Products:{platform}/usr/lib",
        "DYLD_INSERT_LIBRARIES": f"{platform}/usr/lib/libXCTestBundleInject.dylib",
    },
    "EnvironmentVariables": fixture_environment,
}
run = {
    "__xctestrun_metadata__": {"FormatVersion": 2},
    "TestConfigurations": [{"Name": "Subscription Native", "IsEnabled": True, "TestTargets": [target]}],
}
(pathlib.Path(directory) / "SubscriptionAlerts.xctestrun").write_bytes(plistlib.dumps(run))
PY
tests=()
for method in "$@"; do
  tests+=("-only-testing:SubscriptionAlertsNative/SubscriptionAlertsNative/$method")
done
diagnostics=()
if [[ "${NU54_IOS_SKIP_TEST_DIAGNOSTICS:-}" = 1 ]]; then
  diagnostics=(-collect-test-diagnostics never)
fi
scene_run xcodebuild test-without-building -xctestrun "$scene_work/SubscriptionAlerts.xctestrun"   -destination "platform=iOS Simulator,id=$sim" -parallel-testing-enabled NO   -resultBundlePath "$out/Result.xcresult" "${diagnostics[@]+"${diagnostics[@]}"}" "${tests[@]}"

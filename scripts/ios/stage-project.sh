# Xcode and CocoaPods regenerate bindings beside the project. Copy every
# writable project and Pods path into the caller's owned work directory.
scene_stage_ios_source() {
  local root=$1 stage=$2 input
  mkdir -p "$stage/products"
  for input in node_modules packages package.json; do
    ln -s "$root/$input" "$stage/$input"
  done
  ln -s "$root/products/p01-device-firmware" "$stage/products/p01-device-firmware"
  export NU54_PICKUP_READONLY_ROOT="$root"
}

scene_stage_ios_project() {
  local root=$1 stage=$2 scratch=$3 out=$4 product=$5 spec=$6
  local source_app="$1/products/$5" app="$2/products/$5"
  local file directory generated
  mkdir -p "$app"
  for file in app.json babel.config.js index.js metro.config.js package.json; do
    cp -p "$source_app/$file" "$app/$file"
  done
  for directory in ios src build-tools native vendor scripts; do
    if [[ -d "$source_app/$directory" ]]; then cp -cR "$source_app/$directory" "$app/$directory"; fi
  done
  ln -s "$source_app/node_modules" "$app/node_modules"
  cat >>"$app/metro.config.js" <<'JS'

module.exports.watchFolders.push(process.env.NU54_PICKUP_READONLY_ROOT);
module.exports.resolver.useWatchman = false;
JS
  "${ios_test_env[@]}" node "$root/node_modules/react-native/scripts/generate-codegen-artifacts.js" \
    -p "$app" -o "$scratch/codegen-$spec" -t ios >"$out/$spec-codegen.log" 2>&1
  generated="build/generated/ios/ReactCodegen"
  for file in "$spec/$spec.h" "$spec/$spec-generated.mm" "${spec}JSI.h"; do
    cp -p "$scratch/codegen-$spec/$generated/$file" "$app/ios/$generated/$file"
  done
}

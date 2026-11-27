# Give build subprocesses their own Node/Metro temporary files and XDG cache.
# The caller owns the enclosing directory and removes it when the run ends.
ios_test_prepare_environment() {
  local work="$1/xcode-environment"
  mkdir -m 700 -- "$work" "$work/tmp" "$work/cache"
  ios_test_env=(env "TMPDIR=$work/tmp" "TMP=$work/tmp" "TEMP=$work/tmp" "XDG_CACHE_HOME=$work/cache")
}

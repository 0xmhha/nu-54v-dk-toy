# Private XCTest build lifetime. The registration entry point owns the EXIT trap.
registration_test_work=
registration_test_child=
registration_test_transition=0
registration_test_pending=0

registration_test_interrupt() {
  if [[ $registration_test_transition == 1 ]]; then
    if [[ $registration_test_pending == 0 ]]; then registration_test_pending=$1; fi
    return
  fi
  exit "$1"
}

registration_test_checkpoint() {
  registration_test_transition=0
  if [[ $registration_test_pending != 0 ]]; then exit "$registration_test_pending"; fi
}

registration_test_prepare() {
  local base
  base=$(cd -- "${TMPDIR:-/tmp}" && pwd -P)
  registration_test_transition=1
  registration_test_work=$(mktemp -d "$base/nu54-registration-xctest.XXXXXXXX")
  registration_test_checkpoint
}

registration_test_stop() {
  local pid=$registration_test_child attempt
  if [[ -z "$pid" ]]; then return; fi
  if kill -0 -- "-$pid" 2>/dev/null; then
    kill -TERM -- "-$pid" 2>/dev/null || true
    for ((attempt=0; attempt<50; attempt++)); do
      kill -0 -- "-$pid" 2>/dev/null || break
      sleep 0.1
    done
    kill -KILL -- "-$pid" 2>/dev/null || true
  fi
  wait "$pid" 2>/dev/null || true
  for ((attempt=0; attempt<50; attempt++)); do
    if ! kill -0 -- "-$pid" 2>/dev/null; then
      registration_test_child=
      return
    fi
    sleep 0.1
  done
  echo "Could not stop the owned XCTest process group: $pid" >&2
  return 1
}

registration_test_run() {
  local status=0
  registration_test_transition=1
  set -m
  "${ios_test_env[@]}" "$@" &
  registration_test_child=$!
  set +m
  registration_test_checkpoint
  wait "$registration_test_child" || status=$?
  # An exited build may leave descendants. Reap any remaining owned group.
  registration_test_stop || return
  return "$status"
}

registration_test_cleanup() {
  # Never remove DerivedData while an owned tool may still be using it.
  registration_test_stop || return
  if [[ -n "$registration_test_work" ]]; then
    rm -rf -- "$registration_test_work"
  fi
}

registration_release_clean_cache() {
  local derived=$1
  # The default DerivedData root is freshly created under the evidence
  # directory. Leave Build/Products, logs and all other outputs available.
  if [[ -L "$derived" || -L "$derived/Build" ]]; then
    echo "Retained redirected Release build cache: $derived" >&2
    return 1
  fi
  rm -rf -- "$derived/Build/Intermediates.noindex" "$derived/ModuleCache.noindex" \
    "$derived/SDKStatCaches.noindex" "$derived/SDKExplicitPrecompiledModules" \
    "$derived/CompilationCache.noindex" "$derived/Index.noindex"
}

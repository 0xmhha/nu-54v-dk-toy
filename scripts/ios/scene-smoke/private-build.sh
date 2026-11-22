# Sourced by the simulator scene runners. Own only the generated work directory.
source "${BASH_SOURCE[0]%/*}/../test-environment.sh"

scene_cleanup() {
  local status=$1 owned_spec=${2:-} failed
  trap '' HUP INT TERM
  trap - EXIT
  if [[ -n "$owned_spec" ]]; then
    rm -f -- "$owned_spec" || {
      failed=$?
      echo "could not remove private XCTest spec: $owned_spec" >&2
      if [[ "$status" == 0 ]]; then status=$failed; fi
    }
  fi
  rm -rf -- "$scene_work" || {
    failed=$?
    echo "could not remove private build directory: $scene_work" >&2
    if [[ "$status" == 0 ]]; then status=$failed; fi
  }
  exit "$status"
}

scene_interrupt() {
  trap '' HUP INT TERM
  if [[ -n "$scene_child" ]]; then
    local attempt
    kill -TERM -- "-$scene_child" 2>/dev/null || true
    for ((attempt=0; attempt<50; attempt++)); do
      kill -0 -- "-$scene_child" 2>/dev/null || break
      sleep 0.1
    done
    kill -KILL -- "-$scene_child" 2>/dev/null || true
    wait "$scene_child" 2>/dev/null || true
  fi
  exit "$1"
}

scene_prepare() {
  local temp
  temp=$(cd -- "${TMPDIR:-/tmp}" && pwd -P)
  scene_work=$(mktemp -d "$temp/nu54-scene.XXXXXXXX")
  scene_child=
  trap 'scene_cleanup "$?"' EXIT
  trap 'scene_interrupt 129' HUP
  trap 'scene_interrupt 130' INT
  trap 'scene_interrupt 143' TERM
  ios_test_prepare_environment "$scene_work"
}

scene_remember_spec_interrupt() {
  if [[ -z "$scene_spec_interrupt" ]]; then scene_spec_interrupt=$1; fi
}

scene_allocate_spec() {
  scene_owned_spec=
  scene_spec_interrupt=
  trap 'scene_cleanup "$?" "$scene_owned_spec"' EXIT
  # Defer the first interruption until the sibling's exact path is published.
  trap 'scene_remember_spec_interrupt 129' HUP
  trap 'scene_remember_spec_interrupt 130' INT
  trap 'scene_remember_spec_interrupt 143' TERM
  scene_owned_spec=$(python3 - "$1" "$2" "${3:-}" <<'PY_SPEC'
import os, pathlib, signal, sys, tempfile
for signum in (signal.SIGHUP, signal.SIGINT, signal.SIGTERM):
    signal.signal(signum, signal.SIG_IGN)
prefix, source, follow = sys.argv[1:]
source = pathlib.Path(source)
if follow == "resolved":
    source = source.resolve()
fd, path = tempfile.mkstemp(prefix=prefix + "-", suffix=".xctestrun", dir=source.parent)
os.close(fd)
print(path)
PY_SPEC
)
  trap 'scene_interrupt "${scene_spec_interrupt:-129}"' HUP
  trap 'scene_interrupt "${scene_spec_interrupt:-130}"' INT
  trap 'scene_interrupt "${scene_spec_interrupt:-143}"' TERM
  if [[ -n "$scene_spec_interrupt" ]]; then scene_interrupt "$scene_spec_interrupt"; fi
}

scene_run() {
  local status=0
  # A separate process group lets the signal handler stop the active tool and
  # its children before the EXIT trap removes their working directory.
  set -m
  "${ios_test_env[@]}" "$@" &
  scene_child=$!
  set +m
  wait "$scene_child" || status=$?
  scene_child=
  return "$status"
}

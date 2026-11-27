#!/usr/bin/env bash
# Start a disposable local 8283 venue and quote service. Optionally run one verifier within its lifetime.
set -euo pipefail
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
exec python3 -B "$here/mobile-fixture-workspace.py" "$@"

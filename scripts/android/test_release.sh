#!/usr/bin/env bash
set -euo pipefail
exec python3 -B "$(dirname "$0")/test_release_workspace.py" "$@"

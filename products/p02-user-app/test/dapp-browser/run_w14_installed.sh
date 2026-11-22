#!/usr/bin/env bash
set -euo pipefail
exec "${W14_PYTHON:-python3}" "$(dirname "$0")/run_w14_installed.py" "$@"

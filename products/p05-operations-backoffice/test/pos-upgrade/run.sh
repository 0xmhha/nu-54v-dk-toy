#!/bin/sh
set -eu
here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
exec python3 -B "$here/../../../../scripts/pos_upgrade_fixture.py"

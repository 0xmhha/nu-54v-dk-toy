#!/usr/bin/env bash
# Run the Chrome, TLS, PostgreSQL and Anvil workflow using disposable local data.
set -euo pipefail
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../../.." && pwd)
cd "$root"
if [[ $# -gt 1 ]]; then
  echo "usage: $0 [existing-evidence-directory]" >&2
  exit 2
fi
evidence=
if [[ $# -eq 1 ]]; then
  evidence=$(cd -- "$1" && pwd)
fi
: "${P08_PLAYWRIGHT_CLI:=$(command -v playwright-cli)}"
[[ -x "$P08_PLAYWRIGHT_CLI" ]]
export P08_PLAYWRIGHT_CLI
scratch=$(mktemp -d /tmp/p08-l06.XXXXXX)
export GOPROXY=off GOSUMDB=off FOUNDRY_OFFLINE=true
export PGHOST="$scratch" PGPORT=5432 PGSSLMODE=disable PGUSER="$(id -un)"
export P08_TEST_OPERATOR_MTLS_BROWSER="$root/products/p08-market-services/test/operator-browser.sh"
export P08_OPERATOR_BROWSER_EVIDENCE="$scratch/browser.yml"
cleanup() {
  pg_ctl -D "$scratch/pg" -m immediate stop >/dev/null 2>&1 || true
  rm -rf "$scratch"
}
trap cleanup EXIT
initdb -D "$scratch/pg" -A trust --no-locale --encoding=UTF8 -U "$PGUSER" >/dev/null
if ! pg_ctl -D "$scratch/pg" -o "-h '' -k $scratch" -l "$scratch/pg.log" -w start; then
  cat "$scratch/pg.log" >&2
  exit 1
fi
createdb l06
P08_MIGRATION_DATABASE_URL=l06 python3 products/p10-platform/database/migrate.py p08
psql -X -v ON_ERROR_STOP=1 -d l06 -c \
  'CREATE ROLE marketkeeper LOGIN IN ROLE nu54_p08_keeper; CREATE ROLE marketreader LOGIN IN ROLE nu54_p08_inspector'
export P08_TEST_DATABASE_URL="host=$scratch port=$PGPORT dbname=l06 sslmode=disable user=marketkeeper"
export P08_TEST_INSPECT_DATABASE_URL="host=$scratch port=$PGPORT dbname=l06 sslmode=disable user=marketreader"
bash products/p08-market-services/test/anvil.sh
if [[ -n "$evidence" ]]; then
  cp "$scratch/browser.yml" "$evidence/l06-ready.yml"
  cp "$scratch/browser.yml.auth.yml" "$evidence/l06-auth.yml"
  cp "$scratch/browser.yml.denied.yml" "$evidence/l06-denied.yml"
fi

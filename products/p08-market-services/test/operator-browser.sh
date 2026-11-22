#!/usr/bin/env bash
# Integration-test hook for Chromium or installed Chrome. Credentials come only from the disposable fixture.
set -Eeuo pipefail
trap 'printf "Operator Chrome test failed at line %s (%s).\n" "$LINENO" "$BASH_COMMAND" >&2' ERR
if [[ $# -ne 4 && $# -ne 5 ]]; then
  echo 'expected fixture operation origin, keeper hash, client certificate, client key, and optional market state' >&2
  exit 2
fi
origin=$1
keeper_hash=$2
certificate=$3
key=$4
[[ "$origin" =~ ^https://127\.0\.0\.1:[0-9]+$ && "$keeper_hash" =~ ^0x[0-9a-f]{64}$ ]]
scratch=$(mktemp -d /tmp/p08pw.XXXXXX)
session="p08-operator-${RANDOM}"
mkdir "$scratch/tmp" "$scratch/cache" "$scratch/s"
export TMPDIR="$scratch/tmp" XDG_CACHE_HOME="$scratch/cache" PWTEST_SOCKETS_DIR="$scratch/s"
playwright() {
  if [[ -n "${P08_PLAYWRIGHT_CLI:-}" ]]; then
    "$P08_PLAYWRIGHT_CLI" "$@"
  else
    npx --offline --yes --package @playwright/cli playwright-cli "$@"
  fi
}
assert_browser() {
  local expected=$1
  local output
  if ! output=$(playwright -s="$session" run-code "$2" 2>&1) ||
      ! grep -Fxq "$(printf '"%s"' "$expected")" <<< "$output"; then
    printf 'Browser check %s failed:\n%s\n' "$expected" "$output" >&2
    return 1
  fi
}
cleanup() {
  playwright -s="$session" close >/dev/null 2>&1 || true
  playwright -s="$session" delete-data >/dev/null 2>&1 || true
  rm -rf "$scratch"
}
trap cleanup EXIT
cd "$scratch"
python3 - "$origin" "$certificate" "$key" <<'PY' > browser.json
import json, sys
origin, certificate, key = sys.argv[1:]
print(json.dumps({
    "browser": {
        "browserName": "chromium",
        "launchOptions": {
            "channel": "chrome", "headless": True,
            "ignoreDefaultArgs": ["--disable-back-forward-cache"],
            "args": ["--host-resolver-rules=MAP * ~NOTFOUND, EXCLUDE 127.0.0.1"],
        },
        "contextOptions": {
            "clientCertificates": [{"origin": origin, "certPath": certificate, "keyPath": key}],
        },
    }
}))
PY
playwright -s="$session" open about:blank --browser chrome --config "$scratch/browser.json"
if [[ -n "${P08_OPERATOR_FIXTURE_GRANTS:-}" ]]; then
  # Pause dispatch of the real access request to observe the pending UI before any grant result.
  playwright -s="$session" run-code "async (page) => {
    const pending = page.waitForRequest('$origin/access');
    await page.route('$origin/access', async route => {
      await new Promise(resolve => setTimeout(resolve, 1500));
      await route.continue().catch(() => {});
    }, {times: 1});
    await page.goto('$origin');
    await pending;
    await page.getByRole('heading', {name: 'Checking market operator access'}).waitFor();
  }" >/dev/null
  auth_snapshot="$scratch/auth.yml"
  playwright -s="$session" snapshot --filename "$auth_snapshot" >/dev/null
  grep -q 'Checking market operator access' "$auth_snapshot"
  grep -q 'nu54://market-learning' "$auth_snapshot"
  if grep -Eq "$keeper_hash|Price sources|Keeper reconciliation" "$auth_snapshot"; then
    echo 'Pending authorization displayed privileged data' >&2
    exit 1
  fi
  if [[ -n "${P08_OPERATOR_BROWSER_EVIDENCE:-}" ]]; then
    cp "$auth_snapshot" "${P08_OPERATOR_BROWSER_EVIDENCE}.auth.yml"
  fi
fi
playwright -s="$session" goto "$origin" >/dev/null
playwright -s="$session" run-code 'async (page) => {
  await page.getByRole("heading", {name: "P08 Market Operations"}).waitFor();
}' >/dev/null
snapshot="$scratch/observed.yml"
playwright -s="$session" snapshot --filename "$snapshot" >/dev/null
[[ -f "$snapshot" ]]
grep -q 'P08 Market Operations' "$snapshot"
grep -q "$keeper_hash" "$snapshot"
case "${5:-paused}" in
  paused) grep -q 'paused' "$snapshot" ;;
  price-unavailable) grep -q 'price-unavailable' "$snapshot"; grep -q 'stale' "$snapshot" ;;
  ready) grep -q 'ready' "$snapshot"; grep -q 'current' "$snapshot"; grep -q 'fresh' "$snapshot" ;;
  *) exit 2 ;;
esac
grep -q '3 · closed' "$snapshot"
grep -q 'confirmed' "$snapshot"
grep -q 'Duplicate work keys' "$snapshot"
if [[ -n "${P08_OPERATOR_BROWSER_EVIDENCE:-}" ]]; then
  cp "$snapshot" "$P08_OPERATOR_BROWSER_EVIDENCE"
fi
printf 'Chrome displayed %s with the original keeper hash and revision 3.\n' "${5:-paused}"

if [[ -n "${P08_OPERATOR_FIXTURE_GRANTS:-}" ]]; then
  # The Go integration created this temporary fixture file and explicitly selected it.
  grants=$P08_OPERATOR_FIXTURE_GRANTS
  cp "$grants" "$scratch/grants-original"
  replace_grants() {
    local source=$1
    local next
    next=$(mktemp "${grants}.browser.XXXXXX")
    cat "$source" > "$next"
    mv -f "$next" "$grants"
  }
  printf '{}\n' > "$scratch/grants-revoked"
  replace_grants "$scratch/grants-revoked"
  assert_browser revoked-evidence-cleared 'async (page) => {
    await page.getByRole("heading", {name: "Cannot open market operations"}).waitFor();
    if (await page.locator("#observation").innerHTML() !== "") {
      throw new Error("Revocation left price or keeper nodes in the current document");
    }
    const result = await page.evaluate(async () => {
      const response = await fetch("/observation", {headers: {"X-P08-Console": "observation"}});
      return {status: response.status, body: await response.text()};
    });
    if (result.status !== 403 || result.body !== "") {
      throw new Error("Revoked grant can still retrieve evidence");
    }
    return "revoked-evidence-cleared";
  }'
  assert_browser learning-return-clicked 'async (page) => {
    const link = page.getByRole("link", {name: "Return to market learning"});
    if (await link.getAttribute("href") !== "nu54://market-learning" ||
        await link.getAttribute("rel") !== "noreferrer") {
      throw new Error("The learning return link is not the exact private-data-free route");
    }
    await link.evaluate(anchor => {
      anchor.addEventListener("click", event => {
        event.preventDefault();
        anchor.dataset.testClicked = String(event.isTrusted);
      }, {once: true});
    });
    await link.click();
    if (await link.getAttribute("data-test-clicked") !== "true") {
      throw new Error("Chrome did not dispatch the learning return action");
    }
    return "learning-return-clicked";
  }'
  denied_snapshot="$scratch/denied.yml"
  playwright -s="$session" snapshot --filename "$denied_snapshot" >/dev/null
  grep -q 'Cannot open market operations' "$denied_snapshot"
  if grep -Eq "$keeper_hash|Price sources|Keeper reconciliation" "$denied_snapshot"; then
    echo 'Denied browser snapshot managed to display privileged data' >&2
    exit 1
  fi
  if [[ -n "${P08_OPERATOR_BROWSER_EVIDENCE:-}" ]]; then
    cp "$denied_snapshot" "${P08_OPERATOR_BROWSER_EVIDENCE}.denied.yml"
  fi
  replace_grants "$scratch/grants-original"
  retry_ref=$(sed -nE 's/.*button "Check operator access again".*\[ref=((f[0-9]+)?e[0-9]+)\].*/\1/p' "$denied_snapshot")
  if [[ -z "$retry_ref" ]]; then
    cat "$denied_snapshot" >&2
    exit 1
  fi
  playwright -s="$session" click "$retry_ref"
  assert_browser original-evidence-restored "async (page) => {
    await page.getByRole('heading', {name: 'P08 Market Operations'}).waitFor();
    await page.getByText('$keeper_hash', {exact: true}).first().waitFor();
    return 'original-evidence-restored';
  }"
  playwright -s="$session" goto about:blank
  replace_grants "$scratch/grants-revoked"
  assert_browser history-restoration-checked 'async (page) => {
    await page.goBack();
    if (await page.locator("#observation").innerHTML() !== "") {
      throw new Error("History restoration exposed previous evidence");
    }
    await page.getByRole("heading", {name: "Cannot open market operations"}).waitFor();
    return "history-restoration-checked";
  }'
  replace_grants "$scratch/grants-original"
  printf 'Chrome verified authentication pending, the learning return link, and fresh authorization after history restore.\n'
  printf 'Chrome cleared all evidence in the open tab after grant revocation and restored the original evidence after rechecking a renewed grant.\n'
fi

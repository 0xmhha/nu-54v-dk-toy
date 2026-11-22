#!/bin/sh
set -eu
: "${PLAYWRIGHT_CLI:?path to Playwright CLI entrypoint}"
origin="$1"
branch="$2"
maker_cert="$3"
maker_key="$4"
checker_cert="$5"
checker_key="$6"
scratch=$(mktemp -d)
maker="nu54-payout-maker-$$"
checker="nu54-payout-checker-$$"
cleanup() {
    node "$PLAYWRIGHT_CLI" -s="$maker" close >/dev/null 2>&1 || true
    node "$PLAYWRIGHT_CLI" -s="$checker" close >/dev/null 2>&1 || true
    rm -rf "$scratch"
}
trap cleanup EXIT
cd "$scratch"
profile() {
    python3 - "$origin" "$1" "$2" <<'PY'
import json
import sys
origin, cert, key = sys.argv[1:]
print(json.dumps({"browser": {
    "browserName": "chromium", "launchOptions": {"channel": "chrome"},
    "contextOptions": {"ignoreHTTPSErrors": True, "viewport": {"width": 390, "height": 760},
                       "clientCertificates": [{"origin": origin, "certPath": cert, "keyPath": key}]}
}}))
PY
}
profile "$maker_cert" "$maker_key" > maker.json
profile "$checker_cert" "$checker_key" > checker.json
node "$PLAYWRIGHT_CLI" -s="$maker" open --config="$scratch/maker.json" "$origin/ops/"
node "$PLAYWRIGHT_CLI" -s="$checker" open --config="$scratch/checker.json" "$origin/ops/"
review() {
    result=$(node "$PLAYWRIGHT_CLI" -s="$1" run-code "$(cat <<EOF_JS
async (page) => {
    await page.getByRole("button", {name:"지급 주소", exact:true}).first().click();
    await page.getByLabel("지점 참조").fill("$branch");
    await page.getByRole("button", {name:"같은 변경 상태 조회"}).click();
    await page.getByRole("button", {name:"현재 승인자 검토"}).click();
    await page.getByRole("button", {name:"현재 계정으로 확인"}).click();
    await page.getByRole("status").filter({hasText:"$2"}).waitFor();
    return "nu54-payout-review-ok";
}
EOF_JS
)")
    printf '%s\n' "$result" | grep -q '^"nu54-payout-review-ok"$' || { printf '%s\n' "$result" >&2; return 1; }
}
review "$maker" "지점 또는 지급 주소 변경 권한이 없습니다"
review "$checker" "같은 요청에 검토 결과를 기록했습니다"
result=$(node "$PLAYWRIGHT_CLI" -s="$checker" run-code "$(cat <<'EOF_JS'
async (page) => {
    await page.getByText("1 / 2", {exact:true}).waitFor();
    await page.getByLabel("지점 참조").fill("b7d3377e-7c01-4a53-b9d6-fa5584557fae");
    await page.getByRole("button", {name:"같은 변경 상태 조회"}).click();
    await page.getByRole("status").filter({hasText:"지점 또는 지급 주소 변경 권한이 없습니다"}).waitFor();
    if (await page.getByText("1 / 2", {exact:true}).count()) throw Error("previous scope disclosed");
    return "nu54-payout-scope-ok";
}
EOF_JS
)")
printf '%s\n' "$result" | grep -q '^"nu54-payout-scope-ok"$' || { printf '%s\n' "$result" >&2; exit 1; }

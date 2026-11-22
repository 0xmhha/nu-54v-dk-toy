#!/bin/sh
set -eu
: "${PLAYWRIGHT_CLI:?path to Playwright CLI entrypoint}"
cd "$NU54_ACCESS_DIR"
case "$1" in
submit) actor=maker ;;
verify) actor=checker ;;
*) exit 2 ;;
esac
session="nu54-legacy-$actor-$$"
trap 'node "$PLAYWRIGHT_CLI" -s="$session" close >/dev/null 2>&1 || true' EXIT
python3 - "$NU54_ACCESS_ORIGIN" "$NU54_ACCESS_DIR/$actor" > "$actor.json" <<'PY'
import json, sys
origin, name = sys.argv[1:]
print(json.dumps({"browser": {"browserName": "chromium", "launchOptions": {"channel": "chrome"},
    "contextOptions": {"ignoreHTTPSErrors": True, "viewport": {"width": 390, "height": 760},
    "clientCertificates": [{"origin": origin, "certPath": name+".crt", "keyPath": name+".key"}]}}}))
PY
node "$PLAYWRIGHT_CLI" -s="$session" open --config="$PWD/$actor.json" "$NU54_ACCESS_ORIGIN/ops/"
result=$(node "$PLAYWRIGHT_CLI" -s="$session" run-code "$(cat <<EOF_JS
async page => {
    await page.getByRole("button",{name:"v1 적립 인출",exact:true}).first().click();
    await page.getByLabel("담당 지점").selectOption("$NU54_ACCESS_BRANCH");
    await page.getByRole("button",{name:"기존 v1 계약 조회"}).click();
    if ("$1" === "submit") {
        await page.getByRole("button",{name:"기존 적립 인출 검토"}).click();
        await page.getByText("검토한 금액 0.000003 TestUSDC",{exact:false}).waitFor();
        await page.getByRole("button",{name:"같은 v1 인출 요청"}).click();
        await page.getByText("기존 거래 확정 대기",{exact:false}).waitFor();
    } else {
        await page.getByText("독립 확정 근거 일치",{exact:false}).waitFor();
        if (await page.getByRole("button",{name:"기존 적립 인출 검토"}).count()) throw Error("reviewer can withdraw");
        await page.getByRole("button",{name:"확정 증거 기록"}).click();
        await page.getByText("독립 확정 근거 일치",{exact:false}).waitFor();
    }
    return "nu54-legacy-ok";
}
EOF_JS
)") || { printf '%s\n' "$result" >&2; exit 1; }
printf '%s\n' "$result" | grep -q '^"nu54-legacy-ok"$' || { printf '%s\n' "$result" >&2; exit 1; }

#!/bin/sh
set -eu
: "${PLAYWRIGHT_CLI:?path to Playwright CLI entrypoint}"
origin="$1"
branch="$2"
day="$3"
sale="$4"
cert="$5"
key="$6"
scratch=$(mktemp -d)
session="nu54-finance-$$"
maker="nu54-maker-$$"
cleanup() {
    (cd "$scratch" && node "$PLAYWRIGHT_CLI" -s="$session" close) >/dev/null 2>&1 || true
    (cd "$scratch" && node "$PLAYWRIGHT_CLI" -s="$maker" close) >/dev/null 2>&1 || true
    rm -rf "$scratch"
}
trap cleanup EXIT
cd "$scratch"
python3 - "$origin" "$cert" "$key" > config.json <<'PY'
import json
import sys
origin, cert, key = sys.argv[1:]
print(json.dumps({"browser": {
    "browserName": "chromium", "launchOptions": {"channel": "chrome"},
    "contextOptions": {"ignoreHTTPSErrors": True, "viewport": {"width": 390, "height": 760},
                       "acceptDownloads": True, "clientCertificates": [{
                           "origin": origin, "certPath": cert, "keyPath": key
                       }]}
}}))
PY
node "$PLAYWRIGHT_CLI" -s="$session" open --config="$scratch/config.json" "$origin/ops/"
cat > script.js <<EOF_JS
async (page) => {
    await page.getByRole("button", {name: "정산과 이행"}).click();
    await page.getByLabel("지점 참조").fill("$branch");
    await page.getByLabel("영업일").fill("$day");
    await page.getByRole("button", {name: "정산 조회"}).press("Enter");
    await page.getByText("27.00 TestUSDC", {exact:true}).waitFor();
    const download = page.waitForEvent("download");
    await page.getByRole("button", {name: "이 범위 CSV 내려받기"}).click();
    const file = await download;
    if (file.suggestedFilename() !== "settlement-$day.csv") {
        throw Error("incorrect scoped CSV filename: " + file.suggestedFilename());
    }
    await file.saveAs("$scratch/report.csv");
    await page.getByRole("button", {name: "조사 대기열 조회"}).click();
    await page.getByRole("heading", {name: "조사 대기열"}).waitFor();
    await page.getByLabel("원주문 참조").fill("$sale");
    await page.getByRole("button", {name: "기존 주문 확인"}).click();
    await page.getByText("지급 확정", {exact:false}).waitFor();
    await page.evaluate(() => document.documentElement.style.fontSize = "200%");
    const size = await page.evaluate(() => [innerWidth, document.documentElement.scrollWidth, getComputedStyle(document.documentElement).fontSize]);
    if (size[0] !== size[1] || size[2] !== "32px") throw Error(JSON.stringify(size));
    await page.getByLabel("지점 참조").fill("b7d3377e-7c01-4a53-b9d6-fa5584557fae");
    await page.getByRole("button", {name: "정산 조회"}).click();
    await page.getByRole("status").filter({hasText:"지점 또는 업무 권한을 확인할 수 없습니다"}).waitFor();
    if (await page.getByText("27.00 TestUSDC", {exact:true}).count()) throw Error("original branch totals retained after denial");
    await page.getByRole("button", {name:"v2 지급 증거", exact:true}).click();
    await page.getByLabel("담당 지점").selectOption("$branch");
    await page.getByLabel("지급 증거 원주문").fill("$sale");
    await page.getByRole("button", {name:"같은 원주문 증거 조회"}).click();
    await page.getByRole("heading",{name:"독립 확정 근거 일치"}).waitFor();
    await page.getByText("확정 거래", {exact:false}).waitFor();
    return "nu54-finance-evidence-ok";
}
EOF_JS
result=$(node "$PLAYWRIGHT_CLI" -s="$session" run-code "$(cat script.js)")
printf '%s\n' "$result" | grep -q '^"nu54-finance-evidence-ok"$' || { printf '%s\n' "$result" >&2; exit 1; }
python3 - "$branch" "$day" <<'PY'
import csv
import sys
with open("report.csv") as stream:
    rows = list(csv.DictReader(stream))
assert len(rows) == 1 and rows[0]["branch"] == sys.argv[1] and rows[0]["business_day"] == sys.argv[2]
assert (rows[0]["gross_units"], rows[0]["refund_units"], rows[0]["net_units"]) == ("30000000", "3000000", "27000000")
PY


if [ "$#" -ge 8 ]; then
    python3 - config.json "$7" "$8" > maker.json <<'PY2'
import json
import sys
with open(sys.argv[1]) as src:
    config = json.load(src)
identity = config["browser"]["contextOptions"]["clientCertificates"][0]
identity["certPath"], identity["keyPath"] = sys.argv[2:]
print(json.dumps(config))
PY2
    node "$PLAYWRIGHT_CLI" -s="$maker" open --config="$scratch/maker.json" "$origin/ops/"
    cat > maker.js <<EOF_JS
async (page) => {
    await page.getByRole("button",{name:"정산과 이행"}).click();
    await page.getByLabel("지점 참조").fill("$branch");
    await page.getByLabel("환불 대상 원주문").fill("$sale");
    await page.getByRole("button",{name:"원금과 환불 상태 조회"}).click();
    await page.getByLabel("환불 금액 (TestUSDC)").fill("1.10");
    await page.getByRole("button",{name:"환불 신청 검토"}).click();
    await page.getByRole("button",{name:"같은 환불 요청 기록"}).click();
    await page.getByText("1.10 TestUSDC · 별도 승인 대기",{exact:true}).waitFor();
    if (await page.getByRole("button",{name:"별도 승인 검토"}).count()) throw Error("maker may approve their request");
    const code = await page.evaluate(async ({branch,sale}) => {
        const headers={"Content-Type":"application/json"};
        const base={branchId:branch,saleId:sale};
        const response=await fetch("/v2/finance/refund-status",{method:"POST",headers,body:JSON.stringify(base)});
        const view=await response.json();
        const refundId=view.refunds.find(item=>item.amount==="1100000").id;
        const result=await fetch("/v2/finance/refund-approve",{method:"POST",headers,body:JSON.stringify({...base,refundId})});
        return result.status;
    },{branch:"$branch",sale:"$sale"});
    if(code!==404) throw Error("maker API approval "+code);
    return "nu54-finance-maker-ok";
}
EOF_JS
    result=$(node "$PLAYWRIGHT_CLI" -s="$maker" run-code "$(cat maker.js)")
    printf '%s\n' "$result" | grep -q '^"nu54-finance-maker-ok"$' || { printf '%s\n' "$result" >&2; exit 1; }
    cat > checker.js <<EOF_JS
async (page) => {
    await page.getByRole("button",{name:"정산과 이행"}).click();
    await page.getByLabel("지점 참조").fill("$branch");
    await page.getByLabel("환불 대상 원주문").fill("$sale");
    await page.getByRole("button",{name:"원금과 환불 상태 조회"}).click();
    await page.getByText("1.10 TestUSDC · 별도 승인 대기",{exact:true}).waitFor();
    await page.getByRole("button",{name:"별도 승인 검토"}).click();
    await page.getByRole("heading",{name:"별도 승인 · 1.10 TestUSDC"}).waitFor();
    await page.getByRole("button",{name:"현재 계정으로 결정 기록"}).click();
    await page.getByText("같은 환불의 결재 결과를 확인했습니다.",{exact:true}).waitFor();
    return "nu54-finance-checker-ok";
}
EOF_JS
    result=$(node "$PLAYWRIGHT_CLI" -s="$session" run-code "$(cat checker.js)")
    printf '%s\n' "$result" | grep -q '^"nu54-finance-checker-ok"$' || { printf '%s\n' "$result" >&2; exit 1; }
fi

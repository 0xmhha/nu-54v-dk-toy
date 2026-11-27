#!/bin/sh
set -eu
: "${PLAYWRIGHT_CLI:?path to Playwright CLI entrypoint}"
phase=$1
shift
cd "$NU54_ACCESS_DIR"
trap 'for actor in maker checker; do node "$PLAYWRIGHT_CLI" -s="nu54-access-$actor-$$" close >/dev/null 2>&1 || true; done' EXIT
open() {
    python3 - "$NU54_ACCESS_ORIGIN" "$NU54_ACCESS_DIR/$1" > "$1.json" <<'PY'
import json, sys
origin, name = sys.argv[1:]
print(json.dumps({"browser": {"browserName": "chromium", "launchOptions": {"channel": "chrome"},
    "contextOptions": {"ignoreHTTPSErrors": True, "viewport": {"width": 390, "height": 760},
    "clientCertificates": [{"origin": origin, "certPath": name+".crt", "keyPath": name+".key"}]}}}))
PY
    node "$PLAYWRIGHT_CLI" -s="nu54-access-$1-$$" open --config="$PWD/$1.json" "$NU54_ACCESS_ORIGIN/ops/"
}
run() {
    result=$(node "$PLAYWRIGHT_CLI" -s="nu54-access-$1-$$" run-code "$(cat)") || { printf '%s\n' "$result" >&2; exit 1; }
    printf '%s\n' "$result" | grep -q '^"nu54-access-ok"$' || { printf '%s\n' "$result" >&2; exit 1; }
}
case "$phase" in
change)
    target=$1 role=$2 operation=$3
    open maker
    run maker <<EOF_JS
async page => {
    const picker = page.getByLabel("담당 지점");
    await picker.selectOption("$NU54_ACCESS_OTHER_BRANCH");
    if (await page.getByRole("heading",{name:"직원과 단말 자격"}).count()) throw Error("foreign branch exposes access tools");
    await picker.selectOption("$NU54_ACCESS_BRANCH");
    await page.getByRole("heading",{name:"기존 운영 지점"}).waitFor();
    await page.getByLabel("검증된 대상 참조").fill("$target");
    await page.getByLabel("역할",{exact:true}).selectOption("$role");
    await page.getByLabel("변경",{exact:true}).selectOption("$operation");
    await page.getByRole("button",{name:"자격 변경 검토"}).click();
    await page.getByRole("button",{name:"검토한 요청 기록"}).click();
    const row = page.locator("article").filter({hasText:"$target"}).filter({hasText:"별도 승인 대기"});
    await row.waitFor();
    if (await row.getByRole("button",{name:"독립 승인 검토"}).count()) throw Error("maker can approve own change");
    const codes = await page.evaluate(async ({branch,other,target,role,operation}) => {
        const root = "/v2/operations/access/";
        const view = await (await fetch(root+branch)).json();
        const key = view.find(r => r.target===target && r.role===role && r.operation===operation && !r.appliedAt).requestKey;
        const post = async (path,body) => (await fetch(root+path,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)})).status;
        return [await post(branch+"/approve",{requestKey:key}),await post(other+"/propose",{requestKey:crypto.randomUUID(),target,role,operation})];
    }, {branch:"$NU54_ACCESS_BRANCH",other:"$NU54_ACCESS_OTHER_BRANCH",target:"$target",role:"$role",operation:"$operation"});
    if (codes[0]!==404 || codes[1]!==404) throw Error(JSON.stringify(codes));
    return "nu54-access-ok";
}
EOF_JS
    open checker
    run checker <<EOF_JS
async page => {
    const picker = page.getByLabel("담당 지점");
    await picker.selectOption("$NU54_ACCESS_BRANCH");
    await page.getByRole("heading",{name:"기존 운영 지점"}).waitFor();
    if (await picker.locator('option[value="$NU54_ACCESS_OTHER_BRANCH"]').count()) throw Error("checker sees foreign branch");
    await page.getByRole("button",{name:"같은 요청 상태 조회"}).click();
    const row = page.locator("article").filter({hasText:"$target"}).filter({hasText:"별도 승인 대기"});
    await row.getByRole("button",{name:"독립 승인 검토"}).click();
    await page.getByRole("button",{name:"현재 계정으로 승인"}).click();
    await row.waitFor({state:"detached"});
    await page.locator("article").filter({hasText:"$target"}).filter({hasText:"권한 변경 확인"}).first().waitFor();
    return "nu54-access-ok";
}
EOF_JS
    ;;
ack)
    open maker
    run maker <<EOF_JS
async page => {
    await page.getByLabel("담당 지점").selectOption("$NU54_ACCESS_BRANCH");
    await page.getByRole("button",{name:"같은 요청 상태 조회"}).click();
    await page.locator("article").filter({hasText:"$1"}).getByText("등록 단말 적용 확인",{exact:true}).waitFor();
    return "nu54-access-ok";
}
EOF_JS
    ;;
history)
    date=$1 register=$2 terminalRevoke=$3 staffRevoke=$4
    open checker
    run checker <<EOF_JS
async page => {
    await page.getByLabel("담당 지점").selectOption("$NU54_ACCESS_BRANCH");
    await page.getByLabel("시작일").fill("$date");
    await page.getByLabel("종료일").fill("$date");
    await page.getByRole("button",{name:"이 범위 이력 조회"}).click();
    for (const key of ["$register","$terminalRevoke","$staffRevoke"])
        await page.getByText("참조 "+key,{exact:false}).waitFor();
    return "nu54-access-ok";
}
EOF_JS
    ;;
*) exit 2 ;;
esac

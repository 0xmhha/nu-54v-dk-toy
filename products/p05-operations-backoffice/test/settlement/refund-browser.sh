#!/bin/sh
set -eu
: "${PLAYWRIGHT_CLI:?path to Playwright CLI entrypoint}"
origin="$1"
branch="$2"
sale="$4"
cert="$5"
key="$6"
scratch=$(mktemp -d)
session="nu54-finance-$$"
maker="nu54-maker-$$"
treasury="nu54-treasury-$$"
low_case=0
if [ -n "${NU54_REFUND_LOW_TEST:-}" ]; then low_case=1; fi
cleanup() {
    (cd "$scratch" && node "$PLAYWRIGHT_CLI" -s="$session" close) >/dev/null 2>&1 || true
    (cd "$scratch" && node "$PLAYWRIGHT_CLI" -s="$maker" close) >/dev/null 2>&1 || true
    (cd "$scratch" && node "$PLAYWRIGHT_CLI" -s="$treasury" close) >/dev/null 2>&1 || true
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
test "$#" -ge 10
{
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
    const section=page.getByRole("region",{name:"부분 환불"});
    await section.getByText("원결제 반환 경로",{exact:true}).waitFor();
    await section.getByText("신청 가능액",{exact:true}).waitFor();
    await page.getByLabel("환불 금액 (TestUSDC)").fill("900.00");
    await page.getByRole("button",{name:"환불 신청 검토"}).click();
    await page.getByText("신청 가능액 안에서 금액과 소수 자릿수를 확인하세요.",{exact:true}).waitFor();
    await page.getByLabel("환불 금액 (TestUSDC)").fill("1.10");
    await page.getByRole("button",{name:"환불 신청 검토"}).click();
    await page.getByRole("heading",{name:"환불 1.10 TestUSDC 신청"}).waitFor();
    const currentPath=page.url();
    let accepted=false;
    await page.route("**/v2/finance/refund-reserve",async route=>{
      const reply=await route.fetch();
      accepted=reply.ok();
      await reply.dispose();
      await route.abort("connectionreset");
    },{times:1});
    const dropped=page.waitForEvent("requestfailed",{predicate:r=>r.url().endsWith("/refund-reserve")});
    await page.getByRole("button",{name:"같은 환불 요청 기록"}).click();
    await dropped;
    if(!accepted)throw Error("server did not commit first request");
    await page.goto(currentPath);
    await page.getByRole("button",{name:"정산과 이행"}).click();
    await page.getByLabel("지점 참조").fill("$branch");
    await page.getByLabel("환불 대상 원주문").fill("$sale");
    await page.getByRole("button",{name:"원금과 환불 상태 조회"}).click();
    await page.getByRole("heading",{name:"환불 1.10 TestUSDC 신청"}).waitFor();
    await page.getByRole("button",{name:"같은 환불 요청 기록"}).click();
    await page.getByText("1.10 TestUSDC · 별도 승인 대기",{exact:true}).waitFor();
    if(await page.getByText("1.10 TestUSDC · 별도 승인 대기",{exact:true}).count()!==1)throw Error("duplicate refunds");
    await page.getByLabel("환불 금액 (TestUSDC)").fill("0.40");
    await page.getByRole("button",{name:"환불 신청 검토"}).click();
    await page.getByRole("button",{name:"같은 환불 요청 기록"}).click();
    await page.getByText("0.40 TestUSDC · 별도 승인 대기",{exact:true}).waitFor();
    const rival = await page.context().newPage();
    await rival.goto(currentPath);
    await rival.getByRole("button",{name:"정산과 이행"}).click();
    await rival.getByLabel("지점 참조").fill("$branch");
    await rival.getByLabel("환불 대상 원주문").fill("$sale");
    await rival.getByRole("button",{name:"원금과 환불 상태 조회"}).click();
    await rival.getByLabel("환불 금액 (TestUSDC)").fill("1.00");
    await rival.getByRole("button",{name:"환불 신청 검토"}).click();
    await rival.getByRole("heading",{name:"환불 1.00 TestUSDC 신청"}).waitFor();
    await page.getByLabel("환불 금액 (TestUSDC)").fill("5.00");
    await page.getByRole("button",{name:"환불 신청 검토"}).click();
    await page.getByRole("button",{name:"같은 환불 요청 기록"}).click();
    await page.getByText("5.00 TestUSDC · 별도 승인 대기",{exact:true}).waitFor();
    await rival.getByRole("button",{name:"같은 환불 요청 기록"}).click();
    await rival.getByText(/현재 신청 가능액은 0.50 TestUSDC.*새로 검토하세요/).waitFor();
    if (await rival.getByRole("button",{name:"같은 환불 요청 기록"}).count()) throw Error("losing review could resubmit");
    await rival.close();
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
    const pendingSmall=page.locator("article").filter({hasText:"0.40 TestUSDC · 별도 승인 대기"});
    await pendingSmall.getByRole("button",{name:"거절 검토"}).click();
    await page.getByRole("heading",{name:"환불 요청 거절 · 0.40 TestUSDC"}).waitFor();
    if (await page.getByRole("button",{name:"현재 계정으로 결정 기록"}).isEnabled()) throw Error("missing rejection reason enabled decision");
    await page.getByLabel("반려 사유").selectOption("route_review");
    await page.getByRole("button",{name:"현재 계정으로 결정 기록"}).click();
    await page.getByText("0.40 TestUSDC · 거절 · 예약 원금 해제",{exact:true}).waitFor();
    await page.getByText("반려 사유 · 목적지 검증 자료 필요",{exact:true}).waitFor();
    const large = page.locator("article").filter({hasText:"5.00 TestUSDC · 별도 승인 대기"});
    await large.getByRole("button",{name:"거절 검토"}).click();
    await page.getByLabel("반려 사유").selectOption("amount_review");
    await page.getByRole("button",{name:"현재 계정으로 결정 기록"}).click();
    await page.getByText("5.00 TestUSDC · 거절 · 예약 원금 해제",{exact:true}).waitFor();
    await page.locator("article").filter({hasText:"1.10 TestUSDC · 별도 승인 대기"}).getByRole("button",{name:"별도 승인 검토"}).click();
    await page.getByRole("heading",{name:"별도 승인 · 1.10 TestUSDC"}).waitFor();
    await page.getByRole("button",{name:"현재 계정으로 결정 기록"}).click();
    await page.getByText("같은 환불의 결재 결과를 확인했습니다.",{exact:true}).waitFor();
    if (await page.getByRole("button",{name:"재원과 제출 검토"}).count()) throw Error("checker can submit money");
    return "nu54-finance-checker-ok";
}
EOF_JS
    result=$(node "$PLAYWRIGHT_CLI" -s="$session" run-code "$(cat checker.js)")
    printf '%s\n' "$result" | grep -q '^"nu54-finance-checker-ok"$' || { printf '%s\n' "$result" >&2; exit 1; }
    python3 - config.json "$9" "${10}" > treasury.json <<'PY3'
import json,sys
with open(sys.argv[1]) as file:
    config=json.load(file)
cert=config["browser"]["contextOptions"]["clientCertificates"][0]
cert["certPath"], cert["keyPath"]=sys.argv[2:]
print(json.dumps(config))
PY3
    node "$PLAYWRIGHT_CLI" -s="$treasury" open --config="$scratch/treasury.json" "$origin/ops/"
    cat > treasury.js <<EOF_JS
async (page) => {
    await page.getByRole("button",{name:"정산과 이행"}).click();
    await page.getByLabel("지점 참조").fill("$branch");
    await page.getByLabel("환불 대상 원주문").fill("$sale");
    const lookup=page.getByRole("button",{name:"원금과 환불 상태 조회"});
    await lookup.click();
    await page.getByText("1.10 TestUSDC · 승인 기록 · 재원과 전송 확인 대기",{exact:true}).waitFor();
    if (await page.getByRole("button",{name:"별도 승인 검토"}).count() ||
        await page.getByRole("button",{name:"환불 신청 검토"}).count()) throw Error("treasury inherited maker or checker");
    if ($low_case) {
        const denied = page.getByRole("button",{name:"재원과 제출 검토"});
        await page.getByText(/재원이 부족합니다/).waitFor();
        if (await denied.isEnabled()) throw Error("underfunded treasury enabled");
        if (await page.getByText("0.50 TestUSDC",{exact:true}).count()===0) throw Error("on-chain reserve not shown");
        await lookup.click();
        await page.getByText(/재원이 부족합니다/).waitFor();
        if (await denied.isEnabled()) throw Error("re-read enabled underfunded treasury");
        return "nu54-finance-treasury-ok";
    }
    await page.getByRole("button",{name:"재원과 제출 검토"}).click();
    await page.getByRole("heading",{name:/환불 재원과 제출 대상 확인/}).waitFor();
    await page.getByText(/수수료는 원금과 분리/).waitFor();
    const accepted = page.waitForResponse(r=>r.url().endsWith("/refund-submit") && r.request().method()==="POST");
    await page.getByRole("button",{name:"이 환불만 제출 요청"}).click();
    const reply = await accepted;
    if(!reply.ok())throw Error("treasury submission rejected "+reply.status());
    await page.getByText(/1\.10 TestUSDC · (같은 환불 전송 확인 중|기존 거래의 방송·확정 확인 중)/).waitFor();
    if (await page.getByRole("button",{name:"재원과 제출 검토"}).count()) throw Error("submitted refund could be resubmitted as new");
    for(let i=0;i<22;i++){
        await lookup.click();
        const final=page.locator("article").filter({hasText:"1.10 TestUSDC · 환불 확정"});
        try {
            await final.getByText(/원 매출 영업일.*환불 영업일/).waitFor({timeout:500});
            await final.getByText(/^확정 거래 0x/).waitFor();
            return "nu54-finance-treasury-ok";
        } catch (error) {
            if(error.name!=="TimeoutError") throw error;
        }
    }
    throw Error("same refund did not reach final evidence");
}
EOF_JS
    result=$(node "$PLAYWRIGHT_CLI" -s="$treasury" run-code "$(cat treasury.js)")
    printf '%s\n' "$result" | grep -q '^"nu54-finance-treasury-ok"$' || { printf '%s\n' "$result" >&2; exit 1; }
}

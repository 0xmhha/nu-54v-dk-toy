#!/usr/bin/env node
"use strict";
const { chromium } = require(process.env.NU54_PLAYWRIGHT_MODULE || "playwright");
const assert = require("node:assert/strict");

async function exercise() {
  const [origin, branch, , sale, , , certPath, keyPath] = process.argv.slice(2);
  const browser = await chromium.launch({channel:"chrome", headless:true});
  try {
    const page = await browser.newPage({
      ignoreHTTPSErrors:true, clientCertificates:[{origin,certPath,keyPath}],
    });
    await page.goto(origin + "/ops/");
    await page.getByRole("button",{name:"정산과 이행"}).click();
    await page.getByLabel("지점 참조").fill(branch);
    await page.getByLabel("환불 대상 원주문").fill(sale);
    const lookup=page.waitForResponse(r=>r.url().endsWith("/refund-status"));
    await page.getByRole("button",{name:"원금과 환불 상태 조회"}).click();
    const response=await lookup;
    assert.equal(response.status(),200);
    const model=await response.json();
    if (process.env.NU54_RENTAL_PHASE === "held") {
      assert.equal(model.destination,"rental-held");
      await page.getByText(/종료된 대여의 원결제 반환 경로를 확인할 때까지/).waitFor();
      assert.equal(await page.getByRole("button",{name:"환불 신청 검토"}).count(),0);
    } else {
      assert.equal(model.destination,"rental-verified");
      await page.getByText("원결제 대여 세대의 등록 반환 경로 확인",{exact:true}).waitFor();
      await page.getByLabel("환불 금액 (TestUSDC)").fill("0.10");
      await page.getByRole("button",{name:"환불 신청 검토"}).click();
      await page.getByRole("heading",{name:"환불 0.10 TestUSDC 신청"}).waitFor();
    }
    assert.equal(await page.getByRole("textbox",{name:/환불.*주소/}).count(),0);
  } finally {
    await browser.close();
  }
}
exercise().catch(error=>{ console.error(error.message.replace(/[0-9a-f-]{36}/gi,"[fixture]")); process.exitCode=1; });

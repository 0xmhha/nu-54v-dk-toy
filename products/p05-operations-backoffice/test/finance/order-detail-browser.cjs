"use strict";

const assert = require("node:assert/strict");
const { chromium } = require(process.env.NU54_PLAYWRIGHT_MODULE || "playwright");
const [origin, branch, sale, certPath, keyPath, quote, phase, hash] =
  process.argv.slice(2);

async function exercise() {
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  try {
    const page = await browser.newPage({
      ignoreHTTPSErrors: true,
      clientCertificates: [{ origin, certPath, keyPath }],
      viewport: { width: 1440, height: 900 },
    });
    page.setDefaultTimeout(12000);
    await page.goto(origin + "/ops/");
    await page.getByRole("button", { name: "정산과 이행" }).click();
    await page.getByLabel("지점 참조").fill(branch);
    await page.getByLabel("원주문 참조").fill(sale);
    const detail = page.getByRole("region", { name: "원주문 지급 상세" });
    for (const action of ["기존 주문 확인", "이 주문 다시 조회"]) {
      const response = page.waitForResponse((reply) => reply.url().endsWith("/v2/finance/order"));
      await page.getByRole("button", { name: action, exact: true }).click();
      const reply = await response;
      assert.equal(reply.status(), 200);
      assert.deepEqual(reply.request().postDataJSON(), { branchId: branch, saleId: sale });
      await detail.getByText(sale, { exact: true }).waitFor();
      await detail.getByText(quote, { exact: true }).waitFor();
      if (hash) await detail.getByText(hash, { exact: true }).waitFor();
      else assert.equal(await detail.getByText("거래 해시", { exact: true }).count(), 0);
      if (phase === "signed") {
        assert.equal(await detail.getByText("관측 포함 높이", { exact: true }).count(), 0);
      } else {
        await detail.getByText("관측 포함 높이", { exact: true }).waitFor();
      }
      if (phase === "confirmed" || phase === "confirmed-delivered") {
        await detail.getByText("확정 근거 높이", { exact: true }).waitFor();
        await detail.getByText(/^지급 확정 /).waitFor();
        const delivery = phase === "confirmed-delivered" ? "delivered" : "pending";
        await detail.getByText(new RegExp(`^이행 paid · ${delivery} · `)).waitFor();
      } else {
        await detail.getByText("확정 대기", { exact: true }).waitFor();
        await detail.getByText("새 지급·영수증", { exact: true }).waitFor();
        assert.equal(await detail.getByText("보류", { exact: true }).count(), 2);
        assert.equal(await detail.getByText("확정 근거 높이", { exact: true }).count(), 0);
      }
    }
    console.log("same-order re-fetch, correct evidence and hold");
  } finally {
    await browser.close();
  }
}
exercise().catch((error) => {
  console.error(String(error.message).replace(/0x[0-9a-f]+|[a-f0-9-]{36}/gi, "[fixture]"));
  process.exitCode = 1;
});

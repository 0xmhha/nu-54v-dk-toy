#!/usr/bin/env node
"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const { setTimeout: delay } = require("node:timers/promises");
const { chromium } = require(process.env.NU54_PLAYWRIGHT_MODULE || "playwright");
const [origin, branch, day, , certPath, keyPath, makerCert, makerKey] = process.argv.slice(2);
const publicOrigin = process.env.NU54_NOTIFICATION_PUBLIC_ORIGIN;
const stream = process.env.NU54_NOTIFICATION_PUBLIC_STREAM;
const originalPaid = JSON.parse(process.env.NU54_NOTIFICATION_ORIGINAL_PAID || "{}");
const holdBarrier = process.env.NU54_NOTIFICATION_HOLD_BARRIER;

async function exercise() {
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  try {
    const page = await browser.newPage({
      ignoreHTTPSErrors: true, clientCertificates: [{ origin, certPath, keyPath }],
      viewport: { width: 390, height: 760 },
    });
    page.setDefaultTimeout(12000);
    const replyFor = (operation) => page.waitForResponse((response) => response.url().endsWith("/" + operation));

    async function report() {
      await page.goto(origin + "/ops/");
      await page.getByRole("button", { name: "정산과 이행" }).click();
      await page.getByLabel("지점 참조").fill(branch);
      await page.getByLabel("영업일").fill(day);
      await page.getByRole("button", { name: "정산 조회" }).click();
      await page.getByText("27.00 TestUSDC", { exact: true }).waitFor();
    }
    async function cases() {
      const response = replyFor("reconciliation");
      await page.getByRole("button", { name: "조사 대기열 조회" }).click();
      return (await response).json();
    }

    await report();
    const initial = await cases();
    assert.equal(initial.status, "held");
    assert.equal(initial.events.length, 2);
    assert.equal(initial.chainGross, "30000000");
    assert.equal(initial.chainRefunds, "3000000");
    const review = page.getByRole("region", { name: "통지 재처리 검토" });
    const explorerButton = () => review.getByRole("button", { name: "공개 탐색기에서 원결제 보기", exact: true });
    const rereadButton = () => review.getByRole("button", { name: "같은 통지의 원결제 다시 조회", exact: true });
    async function waitFile(path) {
      for (let attempt = 0; attempt < 60; attempt++) {
        try {
          await fs.access(path);
          return;
        } catch {
          await delay(50);
        }
      }
      throw new Error("live evidence change was not coordinated");
    }
    async function holdOpen(event) {
      await page.route("**/v2/finance/fulfillment-review", async route => {
        await fs.writeFile(holdBarrier + ".requested", "");
        await waitFile(holdBarrier + ".held");
        const reply = await route.fetch();
        assert.equal(reply.status(), 200);
        const current = await reply.json();
        assert.equal(current.eventId, event.eventId);
        assert.ok(!("explorerURL" in current.originalPayment));
        await route.fulfill({response: reply});
        await fs.writeFile(holdBarrier + ".response", "");
      }, {times: 1});
      await explorerButton().click();
      await page.getByRole("status").filter({hasText: /공개 탐색기 근거가 달라졌습니다/}).waitFor();
      assert.equal(new URL(page.url()).origin, origin);
      assert.equal(await explorerButton().count(), 0);
      await waitFile(holdBarrier + ".restored");
      await rereadButton().click();
      await explorerButton().waitFor();
      console.log("O04 P07 live scope lost at click: withheld public link, same event reread");
    }
    async function openExplorer(event) {
      const paid = originalPaid[event.eventId];
      assert.ok(paid);
      const publicRequests = [];
      const capture = request => {
        if (new URL(request.url()).origin === publicOrigin) publicRequests.push(request);
      };
      page.on("request", capture);
      try {
        const receipt = page.waitForResponse(response => response.url().startsWith(publicOrigin + "/v2/receipts?"));
        const opened = page.waitForURL(url => url.origin === publicOrigin);
        await explorerButton().click();
        await opened;
        const destination = new URL(page.url());
        assert.equal(destination.pathname, "/");
        assert.deepEqual(Object.fromEntries(destination.searchParams), {
          stream, merchant: paid.merchant, txHash: paid.txHash, logIndex: String(paid.logIndex),
        });
        assert.ok(!destination.href.includes(event.eventId) && !destination.href.includes(event.saleId) && !destination.href.includes(branch));
        const view = await receipt;
        assert.equal(view.status(), 200);
        const result = await view.json();
        assert.equal(result.deployment.stream, stream);
        assert.equal(result.receipts.length, 1);
        const row = result.receipts[0];
        assert.deepEqual([row.kind, row.merchant, row.txHash, row.logIndex], ["paid", paid.merchant, paid.txHash, paid.logIndex]);
        assert.ok(!JSON.stringify(row).includes(event.saleId));
        await page.getByRole("heading", {name: "확정 가맹점 지급", exact: true}).waitFor();
        await page.getByText(paid.txHash, {exact: true}).waitFor();
        const navigation = publicRequests.find(request => request.isNavigationRequest());
        assert.ok(navigation);
        const headers = await navigation.allHeaders();
        for (const sensitive of ["referer", "authorization", "cookie"]) assert.equal(headers[sensitive], undefined);
        console.log("O04 X02 public receipt verified " + JSON.stringify({stream, ...paid}));
      } finally {
        page.off("request", capture);
      }
    }
    if (publicOrigin) {
      const unauthorized = await browser.newPage({
        ignoreHTTPSErrors: true, clientCertificates: [{origin, certPath: makerCert, keyPath: makerKey}],
      });
      try {
        const scope = await unauthorized.request.get(origin + "/v2/finance/scopes");
        assert.equal(scope.status(), 200);
        const grants = await scope.json();
        assert.ok(Array.isArray(grants));
        assert.ok(grants.every(site => !site.capabilities.includes("reconcile_read")));
        const denied = await unauthorized.request.post(origin + "/v2/finance/fulfillment-review", {
          headers: {Origin: origin}, data: {branchId: branch, businessDay: day, eventId: initial.events[0].eventId},
        });
        assert.equal(denied.status(), 404);
      } finally {
        await unauthorized.close();
      }
    }
    const created = [];
    await page.route("**/v2/finance/fulfillment-retry", async (route) => {
      const request = route.request().postDataJSON();
      const reply = await route.fetch();
      assert.equal(reply.status(), 200);
      const body = await reply.json();
      assert.equal(body.eventId, request.eventId);
      assert.equal(body.requestId, request.idempotencyKey);
      created.push(body);
      if (created.length === 1) {
        await reply.dispose();
        await route.abort("connectionreset");
      } else {
        await route.fulfill({ response: reply });
      }
    });

    for (const [index, event] of initial.events.entries()) {
      const row = page.getByRole("article").filter({ hasText: event.eventId });
      const preview = row.getByRole("button", { name: "통지와 영향 검토" });
      await preview.click();
      await review.getByText(event.saleId, { exact: true }).waitFor();
      await review.getByText("선택한 지점의 이행 시스템", { exact: true }).waitFor();
      await review.getByText("원결제 등록 주소 버전", { exact: true }).waitFor();
      await review.getByText("확정 증거 일치", { exact: true }).waitFor();
      if (publicOrigin) {
        if (index === 0) await holdOpen(event);
        await openExplorer(event);
        await report();
        await cases();
        await preview.click();
        await review.getByText(event.eventId, {exact: true}).waitFor();
        assert.equal(created.length, index);
      }
      if (index === 0) {
        await review.getByRole("button", { name: "취소", exact: true }).click();
        assert.equal(created.length, 0);
        await preview.click();
        await review.getByText(event.eventId, { exact: true }).waitFor();
      }
      const end = index === 0
        ? page.waitForEvent("requestfailed", (request) => request.url().endsWith("/fulfillment-retry"))
        : replyFor("fulfillment-retry");
      await review.getByRole("button", { name: "같은 통지 재확인 요청" }).click();
      await end;
      assert.equal(created.length, index + 1);
      const accepted = created[index];
      await report();
      let model;
      for (let attempt = 0; attempt < 20; attempt++) {
        model = await cases();
        if (!model.events.some((item) => item.eventId === event.eventId)) break;
      }
      assert.ok(!model.events.some((item) => item.eventId === event.eventId));
      const same = model.receipts.find((item) => item.requestId === accepted.requestId);
      assert.deepEqual([same.eventId, same.saleId, same.state], [event.eventId, event.saleId, "delivered"]);
      const receipt = page.locator("p").filter({ hasText: accepted.requestId });
      await receipt.getByRole("button", { name: "같은 조회 ID 확인" }).click();
      await review.getByText(accepted.requestId, { exact: true }).waitFor();
      await review.getByText("수신 영수증", { exact: true }).waitFor();
      assert.equal(await review.getByRole("button", { name: "같은 통지 재확인 요청" }).count(), 0);
      await review.getByRole("button", { name: "조사 대기열로" }).click();
    }
    assert.equal(new Set(created.map((item) => item.requestId)).size, 2);

    await page.getByRole("button", { name: "마감 검토" }).click();
    const close = page.getByRole("region", { name: "영업일 마감" });
    await close.getByText("27.00 TestUSDC", { exact: false }).waitFor();
    const closes = [];
    await page.route("**/v2/finance/report-close", async (route) => {
      const reply = await route.fetch();
      assert.equal(reply.status(), 200);
      const body = await reply.json();
      assert.equal(body.idempotencyKey, route.request().postDataJSON().idempotencyKey);
      closes.push(body);
      if (closes.length === 1) {
        await reply.dispose();
        await route.abort("connectionreset");
      } else {
        await route.fulfill({ response: reply });
      }
    });
    const dropped = page.waitForEvent("requestfailed", (request) => request.url().endsWith("/report-close"));
    await close.getByRole("button", { name: "마감 요청", exact: true }).click();
    await dropped;
    assert.ok(closes[0]?.revision > 0);
    await report();
    await close.getByRole("button", { name: "같은 마감 조회" }).click();
    await page.getByRole("status").filter({ hasText: "검증된 영업일 마감을 기록했습니다." }).waitFor();
    assert.deepEqual(closes, [closes[0], closes[0]]);
    console.log("O04 impact preview and cancel, two same-ID receipts after reload, O01 same verified close after response loss");
  } finally {
    await browser.close();
  }
}
exercise().catch((error) => {
  console.error(String(error.message).replace(/0x[0-9a-f]+|[a-f0-9-]{36}/gi, "[fixture]"));
  process.exitCode = 1;
});

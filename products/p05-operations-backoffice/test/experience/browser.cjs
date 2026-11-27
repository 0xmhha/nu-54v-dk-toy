#!/usr/bin/env node
"use strict";

const assert = require("node:assert/strict");
const { chromium } = require(process.env.NU54_PLAYWRIGHT_MODULE || "playwright");
const [origin, branch, subscription, certPath, keyPath, terminalCert, terminalKey] = process.argv.slice(2);
const stage = process.env.NU54_EXPERIENCE_STAGE || "portfolio";
const bookingRef = process.env.NU54_EXPERIENCE_BOOKING;

async function verifyBookingStage(page, booking) {
  assert.equal(booking.id, bookingRef);
  assert.ok(booking.provider);
  const open = page.getByRole("button", {name:"예약 " + bookingRef, exact:true});
  await open.click();
  const card = page.locator("article").filter({has:open});
  if (stage === "mixed-paid") {
    const deposit = booking.legs.find(l => l.kind === "deposit");
    const balance = booking.legs.find(l => l.kind === "balance");
    assert.notEqual(deposit.order,balance.order);
    assert.deepEqual([deposit.state,deposit.amount,deposit.refundable,balance.state,balance.amount],
      ["paid","10000000","10000000","issued","20000000"]);
    assert.ok(deposit.businessDay && balance.expiresAt);
    assert.match(await card.locator("p").filter({hasText:deposit.order}).innerText(), /10\.00 TestUSDC.*영업일/);
    assert.match(await card.locator("p").filter({hasText:balance.order}).innerText(), /20\.00 TestUSDC.*지급 관측 중.*지급 기한/);
  } else {
    const [deposit] = booking.legs;
    assert.equal(booking.legs.length,1);
    assert.equal(deposit.refundable,"0");
    assert.ok(Date.parse(deposit.expiresAt)<Date.now());
    if (stage === "confirmed-release") {
      assert.equal(deposit.state,"closed");
      assert.match(await card.locator("p").filter({hasText:deposit.order}).innerText(), /확정 미지급/);
    } else {
      assert.ok(["issued","checking"].includes(deposit.state));
    }
  }
  const released = stage === "confirmed-release";
  assert.equal(Boolean(booking.releasedAt),released);
  await card.getByText(released ? "예약 재고 반환 확인" : "기존 지급 증거에 따라 예약 재고를 보류합니다.").waitFor();
  const response = page.waitForResponse(r => r.url().includes("/v2/operations/experience/"));
  await card.getByRole("button",{name:"같은 주문 다시 조회"}).click();
  const same=(await (await response).json()).bookings[0];
  assert.deepEqual(same.legs,booking.legs);
  assert.equal(Boolean(same.releasedAt),released);
  console.log("O07 same original booking: " + stage);
}

async function exercise() {
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  try {
    const context = await browser.newContext({
      ignoreHTTPSErrors: true,
      clientCertificates: [{ origin, certPath, keyPath }],
    });
    const page = await context.newPage();
    await page.goto(origin + "/ops/");
    await page.getByRole("button", { name: "예약과 구독", exact: true }).click();
    await page.getByLabel("지점 참조").fill(branch);
    if (bookingRef) await page.getByLabel("예약 또는 구독 참조").fill(bookingRef);
    const query = page.waitForResponse(r => r.url().includes("/v2/operations/experience/"));
    await page.getByRole("button", { name: "예약과 청구 조회" }).click();
    const response = await query;
    assert.equal(response.status(), 200);
    const portfolio = await response.json();
    if (stage !== "portfolio") {
      assert.equal(portfolio.bookings.length,1);
      await verifyBookingStage(page,portfolio.bookings[0]);
      return;
    }
    const booking = portfolio.bookings.find(b => b.legs.filter(l => l.state === "paid").length === 2);
    assert.ok(booking);
    const open = page.getByRole("button", { name: "예약 " + booking.id, exact: true });
    await open.click();
    const card = page.locator("article").filter({ has: open });
    const provider = card.getByText(new RegExp(booking.provider + " · 이용"));
    await provider.waitFor();
    for (const leg of booking.legs) {
      const line = card.locator("p").filter({ hasText: leg.order });
      await line.waitFor();
      const amount = BigInt(leg.amount);
      const decimal = (amount % 1000000n).toString().padStart(6, "0").replace(/0+$/, "").padEnd(2, "0");
      const shown = await line.innerText();
      assert.ok(shown.includes((amount / 1000000n).toString() + "." + decimal + " TestUSDC"));
      assert.ok(shown.includes("영업일 " + leg.businessDay));
      assert.ok(shown.includes("환불 가능액"));
    }
    await card.getByRole("button", { name: "같은 주문 다시 조회" }).click();
    await card.getByText(/기존 주문 지급 확인/).first().waitFor();

    await page.getByLabel("예약 또는 구독 참조").fill(subscription);
    await page.getByRole("button", { name: "예약과 청구 조회" }).click();
    await page.getByRole("button", { name: "구독 " + subscription, exact: true }).click();
    const invoice = portfolio.subscriptions.find(s => s.id === subscription).invoices[0];
    await page.getByText(invoice.order, { exact: false }).waitFor();
    await page.getByRole("button", { name: "해지 범위 검토" }).click();
    await page.getByText("향후 회차만 종료", { exact: true }).waitFor();
    let firstCommand;
    let firstStatus;
    await page.route("**/v2/operations/experience/*/cancel", async route => {
      firstCommand = route.request().postDataJSON();
      const sent = await route.fetch();
      firstStatus = sent.status();
      await sent.dispose();
      await route.abort("connectionreset");
    }, { times: 1 });
    const interrupted = page.waitForEvent("requestfailed", { predicate: request => request.url().endsWith("/cancel") });
    await page.getByRole("button", { name: "같은 해지 요청 확인" }).click();
    await interrupted;
    assert.equal(firstStatus, 200);
    await page.goto(origin + "/ops/");
    await page.getByRole("button", { name: "예약과 구독", exact: true }).click();
    await page.getByLabel("지점 참조").fill(branch);
    await page.getByLabel("예약 또는 구독 참조").fill(subscription);
    await page.getByRole("button", { name: "예약과 청구 조회" }).click();
    await page.getByRole("button", { name: "구독 " + subscription, exact: true }).click();
    await page.getByText("보낸 해지 요청의 결과를 확인하세요.").waitFor();
    const decision = page.waitForResponse(r => r.url().endsWith("/cancel") && r.request().method() === "POST");
    await page.getByRole("button", { name: "같은 해지 요청 확인" }).click();
    const recorded = await decision;
    assert.equal(recorded.status(), 200);
    assert.deepEqual(recorded.request().postDataJSON(), firstCommand);
    await page.getByText("같은 해지 요청이 기록되었습니다.", { exact: true }).waitFor();
    await page.getByRole("button", { name: "발급 회차 다시 조회" }).click();
    await page.getByText(invoice.order, { exact: false }).filter({ hasText: "지급 관측 중" }).waitFor();
    await page.getByText(/향후 회차 종료/).first().waitFor();
    assert.equal(await page.getByRole("button", { name: "해지 범위 검토" }).count(), 0);
    const replay = await context.request.post(recorded.url(), {
      headers: { Origin: origin, "Content-Type": "application/json" },
      data: recorded.request().postDataJSON(),
    });
    assert.equal(replay.status(), 200);
    const result = (await replay.json()).subscriptions[0];
    assert.equal(result.nextCycle, null);
    assert.equal(result.state, "cancelled");
    assert.equal(result.invoices.length, 1);
    assert.equal(result.invoices[0].order, invoice.order);
    assert.equal(result.invoices[0].state, "issued");
    const terminal = await browser.newContext({
      ignoreHTTPSErrors: true,
      clientCertificates: [{ origin, certPath: terminalCert, keyPath: terminalKey }],
    });
    const denied = await terminal.request.get(origin + "/v2/operations/experience/" + branch);
    assert.equal(denied.status(), 404);
    assert.doesNotMatch(await denied.text(), /bookings|subscriptions/);
    console.log("O07 restricted provider schedule, original paid legs, and original issued invoice after an idempotent cancellation");
  } finally {
    await browser.close();
  }
}

exercise().catch((error) => {
  console.error(error.message.replace(/[a-f0-9-]{36}/gi, "[fixture]"));
  process.exitCode = 1;
});

#!/usr/bin/env node
"use strict";
// Browser check of the escrow settlement and dispute pages against a local mock of
// spending-wallet-wire.md section 7. It needs no database or certificate:
//   VITE_PILOT_ASSETS=0xabababababababababababababababababababab pnpm --filter @nu54/p05-backoffice-web build
//   NU54_PLAYWRIGHT_MODULE=<playwright> node test/finance/escrow-browser.cjs [screenshot dir]

const assert = require("node:assert/strict");
const fs = require("node:fs");
const http = require("node:http");
const { createHash } = require("node:crypto");
const path = require("node:path");
const { chromium } = require(process.env.NU54_PLAYWRIGHT_MODULE || "playwright");
const shots = process.argv[2];
const dist = path.join(__dirname, "../../web/dist");
const branch = "11111111-1111-4111-8111-111111111111";
const viewerBranch = "22222222-2222-4222-8222-222222222222";
const emptyBranch = "33333333-3333-4333-8333-333333333333";
const now = Math.floor(Date.now() / 1000);
const uuid = (kind, n) => `${kind}0000000-0000-4000-8000-${String(n).padStart(12, "0")}`;
const asset = "0x" + "ab".repeat(20);
const hash = "0x" + "cd".repeat(32);

function world(count) {
  const orders = [];
  const add = (n, status, extra = {}) =>
    orders.push({ saleId: uuid("a", n), pickupNo: n, amount: "12500000", asset, status,
      lockedAt: now - 3 * 86400, deliverBy: now + 86400, ...extra });
  for (let n = 1; n <= count; n++) add(n, "claimable", { claimableAt: now - 3600 });
  add(501, "held", { deliveryFailed: true });
  add(502, "waiting", { claimableAt: now + 86400, cancelRefused: true });
  add(601, "disputed", { claimableAt: now - 3600, resolveBy: now + 9 * 86400, cancelRefused: true });
  add(602, "disputed", { claimableAt: now - 3600, resolveBy: now + 5 * 3600,
    items: [{ label: "아메리카노", option: "아이스", quantity: 2 }, { label: "크루아상", option: "standard", quantity: 1 }] });
  add(603, "disputed", { claimableAt: now - 3600, resolveBy: now - 60 });
  add(701, "refunded", { refund: "dispute-expired", transaction: hash });
  delete orders.find((o) => o.status === "held").pickupNo;
  return { orders, batches: new Map(), decisions: new Map(), calls: [], drop: undefined, refuse: undefined };
}
let state = world(3);

function handle(operation, body, actor) {
  const scope = scopes(actor).find((row) => row.id === body.branchId);
  const need = { "escrow-claim": "escrow_claim", "escrow-resolve": "escrow_dispute_request",
    "escrow-resolve-approve": "escrow_dispute_approve", "escrow-resolve-reject": "escrow_dispute_approve" }[operation] ?? "report_read";
  if (!scope || !scope.capabilities.includes(need)) return [404];
  const here = body.branchId === emptyBranch ? [] : state.orders;
  const sum = (status) => here.filter((o) => o.status === status)
    .reduce((all, o) => all + BigInt(o.amount), 0n).toString();
  const decisionView = (d) => d && { ...d, proposer: undefined,
    canApprove: d.state === "proposed" && d.proposer !== actor && scope.capabilities.includes("escrow_dispute_approve") };
  if (operation === "order") {
    const order = here.find((o) => o.saleId === body.saleId);
    return order ? [200, { saleId: order.saleId, amount: order.amount, state: "paid", fundsStatus: "held",
      fulfillmentStatus: "recorded", paidAt: new Date(order.lockedAt * 1000).toISOString(), transaction: hash,
      quoteId: null, observedHeight: 10, confirmedHeight: 11, refunds: "0", events: [],
      ...(state.plain ? {} : { escrow: order }) }] : [404];
  }
  if (operation === "escrow-orders") {
    const rows = here.filter((o) => !body.status || o.status === body.status);
    const from = Number(body.cursor ?? 0);
    return [200, { ...(state.plain ? {} : { receiptScope: createHash("sha256").update(actor + body.branchId).digest("hex") }),
      items: rows.slice(from, from + 50).map((o) => ({ ...o, decision: decisionView(state.decisions.get(o.saleId)) })),
      totals: { claimable: sum("claimable"), waiting: sum("waiting"), disputed: sum("disputed"), held: sum("held") },
      ...(from + 50 < rows.length ? { nextCursor: String(from + 50) } : {}) }];
  }
  if (operation === "escrow-claims") {
    if (body.branchId === emptyBranch) return [200, { items: [], nextCursor: null }];
    // A batch moves one step each time it is read, like the worker would.
    for (const batch of state.batches.values()) {
      if (batch.state === "queued") batch.state = "submitted";
      else if (batch.state === "submitted") {
        const paid = batch.saleIds.filter((id) => !batch.skip.some((row) => row.saleId === id));
        Object.assign(batch, { state: "final", transaction: hash, skipped: batch.skip,
          paid: String(12500000n * BigInt(paid.length)) });
        for (const o of state.orders) if (paid.includes(o.saleId)) Object.assign(o, { status: "paid", payout: "claim" });
      }
    }
    return [200, { items: [...state.batches.values()].map(batchView).reverse() }];
  }
  if (operation === "escrow-claim") {
    assert.ok(body.saleIds.length >= 1 && body.saleIds.length <= 100);
    assert.equal(new Set(body.saleIds).size, body.saleIds.length);
    if (state.refuse) { state.refuse = undefined; return [409]; }
    if (!state.batches.has(body.idempotencyKey)) {
      state.batches.set(body.idempotencyKey, { batchId: uuid("b", state.batches.size + 1), state: "queued",
        createdAt: now + state.batches.size, orders: body.saleIds.length, saleIds: body.saleIds,
        requested: String(12500000n * BigInt(body.saleIds.length)), skipped: [],
        skip: (state.skip ?? []).filter((row) => body.saleIds.includes(row.saleId)) });
      for (const o of state.orders) if (body.saleIds.includes(o.saleId)) o.status = "claiming";
    }
    return [200, batchView(state.batches.get(body.idempotencyKey))];
  }
  if (operation === "escrow-resolve") {
    const order = state.orders.find((o) => o.saleId === body.saleId);
    assert.deepEqual(Object.keys(body).sort(), ["branchId", "idempotencyKey", "outcome", "reason", "saleId"]);
    const old = state.decisions.get(body.saleId);
    if (old?.key === body.idempotencyKey) return [200, decisionView(old)];
    const decision = { decisionId: uuid("d", state.calls.length), saleId: body.saleId, outcome: body.outcome,
      reason: body.reason, state: "proposed", resolveBy: order.resolveBy, proposer: actor, key: body.idempotencyKey };
    state.decisions.set(body.saleId, decision);
    return [200, decisionView(decision)];
  }
  const decision = [...state.decisions.values()].find((d) => d.decisionId === body.decisionId);
  if (decision.proposer === actor) return [404];
  decision.state = operation.endsWith("approve") ? "submitted" : "rejected";
  if (decision.state === "submitted") setTimeout(() => {
    const order = state.orders.find((o) => o.saleId === decision.saleId);
    Object.assign(decision, { state: "final", transaction: hash });
    Object.assign(order, decision.outcome === "pay-merchant"
      ? { status: "paid", payout: "arbiter" } : { status: "refunded", refund: "arbiter" });
  }, 1500);
  return [200, decisionView(decision)];
}
// `plain` answers without the optional fields, like a server that does not send them.
const batchView = ({ saleIds, skip, ...batch }) => state.plain ? batch : { ...batch,
  skipped: batch.skipped.map((row) => ({ ...row, pickupNo: state.orders.find((o) => o.saleId === row.saleId)?.pickupNo })) };
function scopes(actor) {
  const full = ["report_read", "escrow_claim", "escrow_dispute_request", "escrow_dispute_approve"];
  if (actor === "nobody") return [];
  return [
    { id: branch, label: "성수점", timezone: "Asia/Seoul", capabilities: full },
    { id: viewerBranch, label: "조회 전용점", timezone: "Asia/Seoul", capabilities: ["report_read"] },
    { id: emptyBranch, label: "새 매장", timezone: "Asia/Seoul", capabilities: full },
  ];
}

const server = http.createServer(async (req, res) => {
  // Chrome resends a POST by itself when a reused connection dies, which would hide the lost reply.
  res.setHeader("Connection", "close");
  const url = new URL(req.url, "http://localhost");
  const actor = /actor=(\w+)/.exec(req.headers.cookie ?? "")?.[1] ?? "maker";
  const json = (status, body) => {
    res.writeHead(status, { "Content-Type": "application/json" });
    res.end(JSON.stringify(body ?? {}));
  };
  if (url.pathname.endsWith("/scopes"))
    return actor === "broken" ? json(503) : json(200, url.pathname.includes("finance") ? scopes(actor) : []);
  if (url.pathname.startsWith("/v2/finance/")) {
    let raw = "";
    for await (const part of req) raw += part;
    assert.ok(Buffer.byteLength(raw) <= 4096, "opsd reads at most 4096 bytes");
    const operation = url.pathname.split("/").pop();
    const body = JSON.parse(raw);
    state.calls.push({ operation, body, actor });
    // A branch without the claim or resolver setup answers before it writes anything.
    if (state.missing === operation) return json(404);
    const [status, reply] = handle(operation, body, actor);
    // The request is stored, then the connection dies before the reply arrives.
    if (state.drop === operation) {
      state.drop = undefined;
      return req.socket.destroy();
    }
    return json(status, reply);
  }
  const file = path.join(dist, url.pathname.replace(/^\/ops\/?/, "") || "index.html");
  if (!file.startsWith(dist) || !fs.existsSync(file)) return json(404);
  const type = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css" }[path.extname(file)];
  res.writeHead(200, { "Content-Type": type ?? "application/octet-stream" });
  fs.createReadStream(file).pipe(res);
});

async function main() {
  await new Promise((done) => server.listen(0, "127.0.0.1", done));
  const origin = `http://127.0.0.1:${server.address().port}`;
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  const errors = [];
  async function open(view, { actor = "maker", width = 1280, site = branch, scale } = {}) {
    const context = await browser.newContext({ viewport: { width, height: 860 } });
    await context.addCookies([{ name: "actor", value: actor, url: origin }]);
    const page = await context.newPage();
    page.setDefaultTimeout(15000);
    page.on("pageerror", (error) => errors.push(error.message));
    await page.goto(origin + "/ops/");
    if (scale) await page.evaluate((value) => { document.documentElement.style.fontSize = value; }, scale);
    await page.getByRole("button", { name: view, exact: true }).click();
    if (site) await page.getByLabel("담당 지점").selectOption(site);
    return page;
  }
  const button = (page, name) => page.getByRole("button", { name, exact: true });
  const shot = async (page, name) => {
    if (shots) await page.screenshot({ path: path.join(shots, name + ".png"), fullPage: true });
  };
  const sent = (operation) => state.calls.filter((call) => call.operation === operation);
  // No reference or hash may be visible outside an opened disclosure.
  async function plain(page) {
    const text = await page.evaluate(() => document.querySelector("main").innerText);
    assert.doesNotMatch(text, /0x[0-9a-f]{20}|[0-9a-f]{8}-[0-9a-f]{4}-|undefined|NaN|escrow_|not-delivered|goodwill/);
  }
  async function fits(page) {
    const [inner, scroll] = await page.evaluate(() => [innerWidth, document.documentElement.scrollWidth]);
    assert.ok(scroll <= inner, `page is ${scroll}px wide in ${inner}px`);
  }

  try {
    // Normal claim, keyboard only.
    let page = await open("대금 정산");
    await page.getByText("37.50 TestUSDC").first().waitFor();
    await shot(page, "01-claims-ready");
    await plain(page);
    await button(page, "청구 가능 주문 모두 선택").press("Enter");
    await page.getByText("3건 선택 · 37.50 TestUSDC", { exact: true }).waitFor();
    await button(page, "일괄 청구").press("Enter");
    assert.equal(await page.evaluate(() => document.activeElement.textContent), "3건 · 37.50 TestUSDC 청구");
    await shot(page, "02-claims-confirm");
    await button(page, "취소").press("Enter");
    await page.waitForFunction(() => document.activeElement.textContent === "일괄 청구");
    await button(page, "일괄 청구").press("Enter");
    await button(page, "3건 청구하기").press("Enter");
    await page.getByText("3건 · 모두", { exact: false }).or(page.getByText("모두 지급", { exact: false })).first().waitFor();
    assert.equal(sent("escrow-claim").length, 1);
    await page.getByText("지금 청구할 수 있는 주문이 없습니다.", { exact: false }).first().waitFor();
    await shot(page, "03-claims-final");
    await plain(page);
    await page.getByRole("combobox", { name: "상태", exact: true }).selectOption("");
    await page.getByText("매장 청구로 지급됨").first().waitFor();
    await page.getByText("전달 표시가 인정되지 않았습니다.", { exact: false }).waitFor();
    await shot(page, "04-claims-all-states");
    await page.getByRole("button", { name: "주문 502번 상세 보기" }).click();
    const detail = page.getByRole("region", { name: "원주문 지급 상세" });
    await detail.getByText("확인 기간 중", { exact: false }).waitFor();
    assert.equal(await detail.getByText("보류", { exact: true }).count(), 0);
    const refusal = "환불이 처리되지 않았습니다.";
    assert.equal(await page.getByText(refusal).count(), 3); // two rows and the opened detail
    await detail.getByText("구매자에게 연락해", { exact: false }).waitFor();
    await shot(page, "05-order-detail");
    await page.context().close();

    // The finance page reads the same state from the order reply, and still works without it.
    for (const [bare, text] of [[false, "확인 기간 중"], [true, "현재 상태는 대금 정산에서 확인하세요."]]) {
      state.plain = bare;
      page = await open("정산과 이행", { site: "" });
      await page.getByLabel("지점 참조").fill(branch);
      await page.getByLabel("원주문 참조").fill(uuid("a", 502));
      await button(page, "기존 주문 확인").click();
      await page.getByRole("region", { name: "원주문 지급 상세" }).getByText(text, { exact: false }).waitFor();
      await page.context().close();
    }

    // 230 orders: three requests, one refused, one batch with skipped orders.
    state = world(230);
    state.skip = [{ saleId: uuid("a", 4), reason: "disputed" }, { saleId: uuid("a", 7), reason: "window-open" },
      { saleId: uuid("a", 9), reason: "in-another-batch" }];
    page = await open("대금 정산");
    await button(page, "청구 가능 주문 모두 선택").click();
    await page.getByText("230건 선택", { exact: false }).waitFor();
    await button(page, "일괄 청구").click();
    await page.getByText("3개 묶음으로 나눠 보냅니다", { exact: false }).waitFor();
    await button(page, "230건 청구하기").click();
    await page.getByText("묶음 3개의 청구를 접수했습니다", { exact: false }).waitFor();
    assert.deepEqual(sent("escrow-claim").map((call) => call.body.saleIds.length), [100, 100, 30]);
    assert.equal(new Set(sent("escrow-claim").map((call) => call.body.idempotencyKey)).size, 3);
    await page.getByText("3건 건너뜀", { exact: false }).waitFor();
    await page.getByText("건너뜀 1건 · 다른 청구에 이미 포함된 주문 (9번)").waitFor();
    assert.equal(await page.getByText("모두 지급", { exact: false }).count(), 2);
    await page.getByText("건너뜀 1건 · 이의가 접수된 주문 (4번)").first().waitFor();
    await plain(page);
    await shot(page, "06-claims-partial-skip");
    // After a reload only the reply can name the skipped orders.
    await page.reload();
    await button(page, "대금 정산").click();
    await page.getByLabel("담당 지점").selectOption(branch);
    await page.getByText("건너뜀 1건 · 이의가 접수된 주문 (4번)").waitFor();
    state.plain = true;
    await button(page, "다시 조회").first().click();
    await page.getByText("건너뜀 1건 · 이의가 접수된 주문", { exact: true }).waitFor();
    await page.getByText("청구 기록 범위를 확인하지 못해", { exact: false }).waitFor();
    assert.equal(await button(page, "일괄 청구").count(), 0);
    await plain(page);
    await page.context().close();

    state = world(2);
    state.refuse = true;
    page = await open("대금 정산");
    await button(page, "청구 가능 주문 모두 선택").click();
    await button(page, "일괄 청구").click();
    await button(page, "2건 청구하기").click();
    await page.getByText("묶음 1개는 주문이 이미 다른 청구에 들어갔거나", { exact: false }).waitFor();
    assert.equal(await page.getByText("확인이 필요한 청구").count(), 0);

    // Lost reply in the middle of a split claim, then a reload.
    state = world(130);
    await button(page, "다시 조회").first().click();
    await button(page, "청구 가능 주문 모두 선택").click();
    await page.getByText("130건 선택", { exact: false }).waitFor();
    await button(page, "일괄 청구").click();
    state.drop = "escrow-claim";
    await button(page, "130건 청구하기").click();
    await page.getByRole("heading", { name: "확인이 필요한 청구" }).waitFor();
    await page.reload();
    await button(page, "대금 정산").click();
    await page.getByLabel("담당 지점").selectOption(branch);
    await page.getByRole("heading", { name: "확인이 필요한 청구" }).waitFor();
    const stored = () => page.evaluate(() => Object.keys(localStorage).filter((key) => key.startsWith("nu54-escrow-claim/")));
    const scoped = `nu54-escrow-claim/${branch}/${createHash("sha256").update("maker" + branch).digest("hex")}`;
    assert.deepEqual(await stored(), [scoped]);
    // Another account in the same browser is not offered this request.
    await page.context().addCookies([{ name: "actor", value: "checker", url: origin }]);
    await button(page, "이의 처리").click();
    await button(page, "대금 정산").click();
    await page.getByLabel("담당 지점").selectOption(branch);
    await page.getByRole("heading", { name: /청구 · 100건/ }).waitFor();
    assert.equal(await page.getByRole("heading", { name: "확인이 필요한 청구" }).count(), 0);
    await page.context().addCookies([{ name: "actor", value: "maker", url: origin }]);
    await button(page, "이의 처리").click();
    await button(page, "대금 정산").click();
    await page.getByLabel("담당 지점").selectOption(branch);
    await page.getByRole("heading", { name: "확인이 필요한 청구" }).waitFor();
    await shot(page, "07-claims-lost-reply");
    await button(page, "같은 청구 다시 확인").click();
    await page.getByText("묶음 2개의 청구를 접수했습니다", { exact: false }).waitFor();
    const keys = sent("escrow-claim").map((call) => call.body.idempotencyKey);
    assert.equal(keys.length, 3);
    assert.equal(keys[0], keys[1]);
    assert.notEqual(keys[1], keys[2]);
    assert.equal(state.batches.size, 2);
    assert.deepEqual(await stored(), []);
    await page.context().setOffline(true);
    await page.getByText("인터넷 연결이 끊겼습니다.", { exact: false }).waitFor();
    assert.ok(await button(page, "청구 가능 주문 모두 선택").isDisabled());
    await shot(page, "08-claims-offline");
    await page.context().close();

    // Refused before writing: nothing is left to retry.
    state = world(130);
    state.missing = "escrow-claim";
    page = await open("대금 정산");
    await button(page, "청구 가능 주문 모두 선택").click();
    await page.getByText("130건 선택", { exact: false }).waitFor();
    await button(page, "일괄 청구").click();
    await button(page, "130건 청구하기").click();
    await page.getByText("묶음 2개는 청구하지 않았습니다.", { exact: false }).waitFor();
    assert.equal(sent("escrow-claim").length, 1);
    assert.equal(await page.getByRole("heading", { name: "확인이 필요한 청구" }).count(), 0);
    assert.deepEqual(await page.evaluate(() => Object.keys(localStorage)), []);
    await page.context().close();

    // An asset this build does not know is never labelled as the pilot asset.
    state = world(1);
    for (const order of state.orders) order.asset = "0x" + "12".repeat(20);
    page = await open("대금 정산");
    await page.getByText("기본 단위로 표시합니다", { exact: false }).waitFor();
    await button(page, "청구 가능 주문 모두 선택").click();
    await page.getByText("1건 선택 · 12,500,000 기본 단위", { exact: true }).waitFor();
    assert.equal(await page.getByText("TestUSDC", { exact: false }).count(), 0);
    await page.getByRole("button", { name: "주문 1번 상세 보기" }).click();
    await page.getByRole("region", { name: "원주문 지급 상세" }).getByText("12,500,000 기본 단위", { exact: false }).waitFor();
    await shot(page, "18-claims-unknown-asset");
    await button(page, "이의 처리").click();
    await page.getByLabel("담당 지점").selectOption(branch);
    await page.getByRole("heading", { name: "주문 601번 · 12,500,000 기본 단위" }).waitFor();
    assert.equal(await page.getByText("TestUSDC", { exact: false }).count(), 0);
    await page.context().close();

    // Access and empty states.
    page = await open("대금 정산", { site: viewerBranch });
    await page.getByText("조회만 할 수 있습니다.", { exact: false }).waitFor();
    assert.equal(await button(page, "일괄 청구").count(), 0);
    await shot(page, "09-claims-read-only");
    await page.getByLabel("담당 지점").selectOption(emptyBranch);
    await page.getByText("아직 청구한 기록이 없습니다.", { exact: false }).waitFor();
    await shot(page, "10-claims-empty");
    await page.context().close();
    page = await open("대금 정산", { actor: "nobody", site: "" });
    await page.getByText("재무 업무를 맡은 지점이 없습니다", { exact: false }).waitFor();
    await page.context().close();
    page = await open("대금 정산", { actor: "broken", site: "" });
    await page.getByText("담당 지점을 불러오지 못했습니다", { exact: false }).waitFor();
    await page.context().close();

    // Disputes: nearest deadline first, two-person rule, lost reply, final.
    state = world(1);
    const maker = await open("이의 처리");
    await maker.getByRole("heading", { name: /^주문 601번/ }).waitFor();
    assert.deepEqual((await maker.getByRole("list", { name: "이의 목록", exact: true }).getByRole("heading", { level: 3 }).allInnerTexts())
      .map((text) => text.split(" · ")[0]), ["주문 603번", "주문 602번", "주문 601번"]);
    const row = (page, n) => page.getByRole("list", { name: "이의 목록", exact: true }).getByRole("listitem").filter({ hasText: `주문 ${n}번` });
    await row(maker, 603).getByText("자동 환불되며", { exact: false }).waitFor();
    assert.equal(await row(maker, 603).getByLabel("매장 지급").count(), 0);
    await row(maker, 602).getByText("기한 임박", { exact: false }).waitFor();
    assert.deepEqual(await maker.getByRole("list", { name: "주문 602번 주문 내용" }).getByRole("listitem").allInnerTexts(),
      ["아메리카노 · 아이스 · 2개", "크루아상 · 1개"]);
    await row(maker, 601).getByText("구매자에게 연락해", { exact: false }).waitFor();
    assert.equal(await row(maker, 602).getByText("환불이 처리되지 않았습니다.").count(), 0);
    await maker.getByText("판정 기한이 지나 자동 환불됨").waitFor();
    await plain(maker);
    await shot(maker, "11-disputes");
    await row(maker, 602).getByLabel("구매자 환불").check();
    await row(maker, 602).getByLabel("판정 사유").selectOption("not_delivered");
    await row(maker, 602).getByRole("button", { name: "판정 신청 검토" }).click();
    state.drop = "escrow-resolve";
    await row(maker, 602).getByRole("button", { name: "판정 신청", exact: true }).click();
    await row(maker, 602).getByRole("button", { name: "같은 신청 다시 확인" }).waitFor();
    await shot(maker, "12-dispute-lost-reply");
    await row(maker, 602).getByRole("button", { name: "같은 신청 다시 확인" }).click();
    await row(maker, 602).getByText("두 번째 담당자 승인 대기", { exact: true }).waitFor();
    const proposals = sent("escrow-resolve");
    assert.equal(proposals.length, 2);
    assert.equal(proposals[0].body.idempotencyKey, proposals[1].body.idempotencyKey);
    await row(maker, 602).getByText("신청한 계정은 승인도 반려도 할 수 없습니다", { exact: false }).waitFor();
    assert.equal(await row(maker, 602).getByRole("button", { name: /승인 검토|반려 검토/ }).count(), 0);
    await shot(maker, "13-dispute-proposer");
    const checker = await open("이의 처리", { actor: "checker" });
    await row(checker, 602).getByRole("button", { name: "승인 검토" }).click();
    const strong = (name) => row(checker, 602).getByRole("button", { name, exact: true })
      .evaluate((node) => getComputedStyle(node).backgroundColor);
    assert.notEqual(await strong("현재 계정으로 승인"), await strong("취소"));
    await shot(checker, "14-dispute-approve");
    const noAccess = "이 지점의 판정 계정이 설정되지 않았거나 권한이 없습니다.";
    state.missing = "escrow-resolve-approve";
    await row(checker, 602).getByRole("button", { name: "현재 계정으로 승인" }).click();
    await checker.getByText("주문 602번: 승인하지 않았습니다. " + noAccess).waitFor();
    assert.equal(await checker.evaluate(() => Object.keys(localStorage).length), 0);
    state.missing = undefined;
    await row(checker, 602).getByRole("button", { name: "승인 검토" }).click();
    state.drop = "escrow-resolve-approve";
    await row(checker, 602).getByRole("button", { name: "현재 계정으로 승인" }).click();
    await row(checker, 602).getByText("응답을 받지 못했습니다", { exact: false }).waitFor();
    await row(checker, 602).getByRole("button", { name: "현재 계정으로 승인" }).click();
    const approvals = sent("escrow-resolve-approve").slice(1);
    assert.equal(approvals[0].body.idempotencyKey, approvals[1].body.idempotencyKey);
    await checker.getByRole("list", { name: "끝난 이의 목록" }).getByText("이의 판정으로 환불됨", { exact: false }).waitFor();
    await plain(checker);
    await shot(checker, "15-dispute-final");
    await row(maker, 601).getByLabel("매장 지급").check();
    await row(maker, 601).getByLabel("판정 사유").selectOption("goodwill");
    await row(maker, 601).getByRole("button", { name: "판정 신청 검토" }).click();
    state.missing = "escrow-resolve";
    await row(maker, 601).getByRole("button", { name: "판정 신청", exact: true }).click();
    await maker.getByText("주문 601번: 신청하지 않았습니다. " + noAccess).waitFor();
    assert.equal(await row(maker, 601).getByRole("button", { name: "같은 신청 다시 확인" }).count(), 0);
    state.missing = undefined;
    // Rejection lets the proposer ask again.
    await row(maker, 601).getByLabel("매장 지급").check();
    await row(maker, 601).getByLabel("판정 사유").selectOption("delivered_proof");
    await row(maker, 601).getByRole("button", { name: "판정 신청 검토" }).click();
    await row(maker, 601).getByRole("button", { name: "판정 신청", exact: true }).click();
    await row(maker, 601).getByText("두 번째 담당자 승인 대기", { exact: true }).waitFor();
    await button(checker, "다시 조회").click();
    await row(checker, 601).getByRole("button", { name: "반려 검토" }).click();
    await row(checker, 601).getByRole("button", { name: "현재 계정으로 반려" }).click();
    await row(checker, 601).getByText("판정 다시 신청").waitFor();
    await maker.context().close();
    await checker.context().close();

    // Phone width and 200% text.
    state = world(3);
    for (const [name, options] of [["390", { width: 390 }], ["390-200", { width: 390, scale: "200%" }], ["1280-200", { width: 1280, scale: "200%" }]]) {
      page = await open("대금 정산", options);
      await button(page, "청구 가능 주문 모두 선택").click();
      await button(page, "일괄 청구").click();
      await page.getByRole("region", { name: "일괄 청구 확인" }).waitFor();
      await fits(page);
      await shot(page, `16-claims-${name}`);
      await button(page, "취소").click();
      await button(page, "이의 처리").click();
      await page.getByLabel("담당 지점").selectOption(branch);
      await row(page, 602).getByLabel("구매자 환불").check();
      await row(page, 602).getByLabel("판정 사유").selectOption("goodwill");
      await row(page, 602).getByRole("button", { name: "판정 신청 검토" }).click();
      await fits(page);
      const small = await page.evaluate(() => [...document.querySelectorAll("button")]
        .filter((node) => node.offsetParent && node.getBoundingClientRect().height < 43).length);
      assert.equal(small, 0);
      await shot(page, `17-disputes-${name}`);
      await page.context().close();
    }
    assert.deepEqual(errors, []);
    console.log("escrow pages: claim, split, skip, refusal, lost reply, access, two-person rule, deadline, 390px, 200% text");
  } finally {
    await browser.close();
    server.close();
  }
}
main().catch((error) => {
  console.error(error);
  process.exit(1);
});

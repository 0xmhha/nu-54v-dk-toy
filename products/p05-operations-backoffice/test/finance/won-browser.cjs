#!/usr/bin/env node
"use strict";
// Browser check of every finance page and the catalog with a won branch next to a dollar branch, against a
// local mock. It needs no database or certificate:
//   pnpm --filter @nu54/p05-backoffice-web build
//   NU54_PLAYWRIGHT_MODULE=<playwright> node test/finance/won-browser.cjs [screenshot dir]

const assert = require("node:assert/strict");
const fs = require("node:fs");
const http = require("node:http");
const path = require("node:path");
const { chromium } = require(process.env.NU54_PLAYWRIGHT_MODULE || "playwright");
const shots = process.argv[2];
const dist = path.join(__dirname, "../../web/dist");
const wonBranch = "11111111-1111-4111-8111-111111111111";
const usdBranch = "22222222-2222-4222-8222-222222222222";
const refusedBranch = "33333333-3333-4333-8333-333333333333";
const sale = "a0000000-0000-4000-8000-000000000001";
const wonAsset = "0x" + "ab".repeat(20), usdAsset = "0x" + "cd".repeat(20), hash = "0x" + "ef".repeat(32);
const token = { name: "NU54 Test KRW", symbol: "tKRW", label: "tKRW", decimals: 2 };
const none = { name: "", symbol: "", label: "", decimals: 0 };
const now = Math.floor(Date.now() / 1000);

const brands = { [wonBranch]: { token, tokenAsset: wonAsset }, [refusedBranch]: { token: none, tokenAsset: wonAsset }, [usdBranch]: {} };
const assets = { [wonBranch]: wonAsset, [refusedBranch]: wonAsset, [usdBranch]: usdAsset };
const published = [];
function finance(operation, body) {
  const brand = brands[body.branchId], asset = assets[body.branchId];
  const order = (n, status, of = asset) => ({ saleId: sale.slice(0, -1) + n, pickupNo: n, amount: "6500000", asset: of, status, lockedAt: now - 9999, deliverBy: now + 9999, claimableAt: now - 60 });
  if (operation === "report") return { receiptScope: "ab".repeat(32), day: body.businessDay, zone: "Asia/Seoul", asset, gross: "6500000", refunds: "1250000", net: "5250000", activeRefunds: 0, openFulfillment: 0, closeRevision: null, ...brand };
  if (operation === "reconciliation") return { events: [], receipts: [], status: "held", chainGross: "6500000", chainRefunds: "1250000", height: 9, ...brand };
  if (operation === "order") return { saleId: body.saleId, asset: body.saleId.endsWith("2") ? usdAsset : asset, amount: "6500000", state: "paid", paidAt: null, transaction: null, quoteId: null, observedHeight: null, confirmedHeight: null, fundsStatus: "closed", fulfillmentStatus: "closed", refunds: "1250000", events: [], ...brand };
  if (operation === "refund-queue") return { items: [{ refundId: sale, saleId: sale, state: "requested", amount: "1250000", principal: "6500000", asset, next: "read_result" },
    { refundId: sale.slice(0, -1) + "2", saleId: sale, state: "requested", amount: "1250000", principal: "6500000", asset: usdAsset, next: "read_result" }], nextCursor: null, ...brand };
  if (operation === "refund-status") return { receiptScope: "cd".repeat(32), saleId: body.saleId, asset, paid: true, destination: "held", canManageFunds: false, canRequest: false, principal: "6500000", available: "5250000", reserved: "0", inFlight: "0", refunded: "1250000", funds: "unknown", refunds: [], ...brand };
  if (operation === "payment-evidence") return { status: "verified", chain: 8283, contract: usdAsset, asset, amount: "6500000", includedAt: "-", attestedAt: "-", ...brand };
  if (operation === "escrow-orders") return { items: body.branchId === wonBranch ? [order(1, "claimable"), order(2, "claimable", usdAsset)] : [order(3, "claimable")],
    totals: { claimable: "1", waiting: "0", disputed: "0", held: "0" }, nextCursor: null, ...brand };
  if (operation === "escrow-claims") return { batches: [], items: [], nextCursor: null, ...brand };
  return {};
}
const site = (branch) => ({ branch, receiptScope: "ef".repeat(32), asset: { address: assets[branch], symbol: branch === wonBranch ? "KRW" : "TestUSDC", decimals: branch === wonBranch ? 2 : 6, chainId: 8283,
  ...(branch === wonBranch ? { token: { ...token, label: "KRW" } } : {}) },
  catalog: { version: 3, timezone: "Asia/Seoul", expiresAt: new Date((now + 86400) * 1000).toISOString(), capabilities: ["catalog_publish", "catalog_stock"], label: "x",
    rows: [{ sku: "latte", option: "standard", label: "라떼", asset: assets[branch], price: branch === wonBranch ? "650000" : "6500001", available: 5 }] } });

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, "http://localhost");
  const json = (status, body) => { res.writeHead(status, { "Content-Type": "application/json" }); res.end(JSON.stringify(body ?? {})); };
  let raw = "";
  for await (const part of req) raw += part;
  const capabilities = ["report_read", "reconcile_read", "refund_request", "catalog", "escrow_claim", "escrow_dispute_request"];
  const rows = [[wonBranch, "원화점"], [usdBranch, "달러점"], [refusedBranch, "꺼진점"]].map(([id, label]) => ({ id, label, timezone: "Asia/Seoul", capabilities,
    ...(id === wonBranch && url.pathname.includes("finance") ? { asset: wonAsset, token } : {}) }));
  if (url.pathname.endsWith("/scopes")) return json(200, rows);
  if (url.pathname.startsWith("/v2/finance/")) return json(200, finance(url.pathname.split("/").pop(), JSON.parse(raw || "{}")));
  const catalog = /^\/v2\/operations\/catalog\/([0-9a-f-]+)(\/\w+)?$/.exec(url.pathname);
  if (catalog && req.method === "POST") { published.push(JSON.parse(raw)); return json(200, {}); }
  if (catalog) return json(200, site(catalog[1]));
  const file = path.join(dist, url.pathname.replace(/^\/ops\/?/, "") || "index.html");
  if (!file.startsWith(dist) || !fs.existsSync(file) || !fs.statSync(file).isFile()) return json(404);
  res.writeHead(200, { "Content-Type": { ".html": "text/html", ".js": "text/javascript", ".css": "text/css" }[path.extname(file)] ?? "application/octet-stream" });
  fs.createReadStream(file).pipe(res);
});

async function main() {
  await new Promise((done) => server.listen(0, "127.0.0.1", done));
  const origin = `http://127.0.0.1:${server.address().port}`;
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  const errors = [];
  const page = await (await browser.newContext({ viewport: { width: 1280, height: 900 } })).newPage();
  page.setDefaultTimeout(8000);
  page.on("pageerror", (error) => errors.push(error.message));
  const text = () => page.evaluate(() => document.querySelector("main").innerText);
  const view = async (name, branch) => {
    await page.getByRole("button", { name, exact: true }).first().click();
    const pick = page.getByLabel("담당 지점").first();
    if (branch && await pick.count()) await pick.selectOption(branch);
  };
  const shot = async (name) => { if (shots) await page.screenshot({ path: path.join(shots, name + ".png"), fullPage: true }); };
  const shows = async (label, good, bad = [/TestUSDC/]) => {
    const seen = await text();
    for (const part of good) assert.ok(seen.includes(part), `${label}: ${part} not in\n${seen}`);
    for (const part of bad) assert.doesNotMatch(seen, part, label);
  };
  try {
    await page.goto(origin + "/ops/");
    const finance = async (branch) => {
      await page.getByRole("button", { name: "정산과 이행", exact: true }).click();
      await page.getByLabel("지점 참조").fill(branch);
    };
    for (const [branch, gross, refund, bad] of [[wonBranch, "65,000원", "12,500원", [/TestUSDC/]], [usdBranch, "6.50 TestUSDC", "1.25 TestUSDC", [/[0-9]원/]],
      [refusedBranch, "6,500,000 기본 단위", "1,250,000 기본 단위", [/TestUSDC/, /[0-9]원/]]]) {
      await finance(branch);
      await page.getByLabel("영업일", { exact: true }).fill("2026-11-25");
      await page.getByRole("button", { name: "정산 조회", exact: true }).click();
      await page.getByText("원장에 반영된 확정 매출").waitFor();
      await shows("report " + branch, [gross, refund], bad);
      await page.getByRole("button", { name: "조사 대기열 조회", exact: true }).first().click();
      await page.getByText("체인 확정 입금", { exact: false }).waitFor();
      await shows("cases " + branch, ["체인 확정 입금 " + gross], bad);
      await shot("report-" + branch.slice(0, 1));
    }
    // The order and refund pages of the won branch, including an order in an asset the branch used before.
    await finance(wonBranch);
    for (const [id, amount] of [[sale, "65,000원"], [sale.slice(0, -1) + "2", "6,500,000 기본 단위"]]) {
      await page.getByLabel("원주문 참조", { exact: true }).last().fill(id);
      await page.getByRole("button", { name: /기존 주문 확인|이 주문 다시 조회/ }).click();
      const detail = page.getByRole("region", { name: "원주문 지급 상세" });
      await detail.getByText(amount).first().waitFor();
      if (id !== sale) assert.doesNotMatch(await detail.innerText(), /[0-9]원/);
    }
    await page.getByRole("button", { name: "환불 대기열 조회", exact: true }).click();
    await page.getByRole("region", { name: "지점 환불 대기열" }).getByText("12,500원", { exact: false }).first().waitFor();
    await shows("refund queue", ["12,500원", "65,000원", "1,250,000 기본 단위", "6,500,000 기본 단위"]);
    assert.equal(((await page.getByRole("region", { name: "지점 환불 대기열" }).innerText()).match(/12,500원/g) ?? []).length, 1);
    await page.getByRole("region", { name: "부분 환불" }).getByLabel(/원주문/).fill(sale);
    await page.getByRole("button", { name: "원금과 환불 상태 조회", exact: true }).click();
    await page.getByText("52,500원", { exact: false }).first().waitFor();
    await shows("refund review", ["65,000원", "52,500원", "12,500원"]);
    await shot("refunds");
    await page.getByRole("button", { name: "v2 지급 증거", exact: true }).click();
    const place = page.getByLabel(/지점/).first();
    if (await place.evaluate((node) => node.tagName) === "SELECT") await place.selectOption(wonBranch); else await place.fill(wonBranch);
    await page.getByLabel("지급 증거 원주문", { exact: true }).fill(sale);
    await page.getByRole("button", { name: "같은 원주문 증거 조회", exact: true }).click();
    await page.getByText("확정된 지급 금액").waitFor();
    await shows("evidence", ["65,000원"]);
    // Claims: the won order in won, the older dollar order in base units, and the dollar shop unaffected afterwards.
    await view("대금 정산", wonBranch);
    await page.getByText("65,000원").first().waitFor();
    await shows("claims", ["65,000원", "6,500,000 기본 단위"]);
    await shot("claims");
    await view("대금 정산", usdBranch);
    await page.waitForFunction(() => /기본 단위|TestUSDC/.test(document.querySelector("main").innerText) && !document.querySelector("main").innerText.includes("65,000원"));
    await shows("dollar claims after won", [], [/65,000원/]);
    // Catalog: shown by the token's symbol, not its label; input fields keep every digit.
    const menu = async (branch) => {
      await page.getByRole("button", { name: "카탈로그", exact: true }).click();
      await page.getByLabel("담당 지점").first().selectOption(branch);
      await page.getByRole("button", { name: "운영 권한 및 메뉴 조회", exact: true }).click();
      await page.getByText("라떼").first().waitFor();
    };
    await menu(wonBranch);
    await shows("won catalog", ["6,500원"]);
    assert.equal(await page.evaluate(() => [...document.querySelectorAll("main input")].some((node) => node.value === "6500.00")), true, "won input");
    await shot("catalog-won");
    // Half a won is refused on the page, and nothing is sent; a whole price is reviewed in won.
    const publish = async (price) => {
      await page.getByLabel("기본 가격", { exact: true }).first().fill(price);
      await page.getByRole("button", { name: "게시 내용 검토", exact: true }).click();
    };
    await publish("6500.55");
    await page.getByRole("status").filter({ hasText: "원 단위" }).first().waitFor();
    assert.equal(published.length, 0);
    await publish("7000");
    await page.getByRole("region", { name: "변경 확인" }).getByText("7,000원", { exact: false }).waitFor();
    await shot("catalog-review");
    await menu(usdBranch);
    await shows("dollar catalog", ["6.50 TestUSDC"], [/[0-9]원/]);
    assert.equal(await page.evaluate(() => [...document.querySelectorAll("main input")].some((node) => node.value === "6.500001")), true, "every digit stays in the input");
    assert.deepEqual(errors, []);
    console.log("won pages: report, cases, order, refunds, evidence, claims, catalog; refused and dollar branches beside them");
  } finally {
    await browser.close();
    server.close();
  }
}
main().catch((error) => { console.error(error); process.exit(1); });

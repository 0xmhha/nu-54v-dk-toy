#!/usr/bin/env node
"use strict";
// Browser check of the order state section (busy, paused, closed days, sold out, the record) against a local mock:
//   pnpm --filter @nu54/p05-backoffice-web build
//   NU54_PLAYWRIGHT_MODULE=<playwright> node test/catalog/state-browser.cjs [screenshot dir]

const assert = require("node:assert/strict");
const fs = require("node:fs");
const http = require("node:http");
const path = require("node:path");
const { chromium } = require(process.env.NU54_PLAYWRIGHT_MODULE || "playwright");
const shots = process.argv[2];
const dist = path.join(__dirname, "../../web/dist");
const branch = "11111111-1111-4111-8111-111111111111";
const me = "22222222-2222-4222-8222-222222222222", other = "33333333-3333-4333-8333-33333333abcd";
const asset = "0x" + "ab".repeat(20);
const sent = [];
const catalog = {
  version: 1, timezone: "Asia/Seoul", expiresAt: new Date(Date.now() + 86400000).toISOString(),
  rows: [
    { sku: "latte", option: "hot", label: "카페라떼", asset, price: "500000", available: 50 },
    { sku: "latte", option: "ice", label: "카페라떼", asset, price: "500000", available: 50 },
    { sku: "coldbrew", option: "standard", label: "콜드브루", asset, price: "550000", available: 40 },
    { sku: "cake", option: "standard", label: "치즈케이크", asset, price: "650000", available: 5 },
  ],
  breakdown: [], options: [], categories: [], categoryBindings: [],
};
const today = "2026-11-26";
// What opsd keeps; changed the way the server's functions change it.
const state = { mode: "open", busyMinutes: null, resumeAt: null, changedBy: null, changedAt: null, today,
  closedDays: [], soldOut: [{ sku: "coldbrew", by: other, at: "2026-11-26T00:50:00Z" }], log: [] };
const record = (change) => state.log.unshift({ by: me, at: new Date().toISOString(), change });

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, "http://localhost");
  const json = (status, body) => { res.writeHead(status, { "Content-Type": "application/json" }); res.end(JSON.stringify(body ?? {})); };
  let raw = "";
  for await (const part of req) raw += part;
  if (url.pathname.endsWith("/scopes")) return json(200, [{ id: branch, label: "여울 커피", timezone: "Asia/Seoul", capabilities: ["catalog"] }]);
  const changing = /^\/v2\/operations\/state\/[0-9a-f-]+\/([a-z-]+)$/.exec(url.pathname);
  if (changing && req.method === "POST") {
    const body = JSON.parse(raw);
    sent.push([changing[1], body]);
    if (changing[1] === "mode") {
      if (body.mode === "busy" && (body.busyMinutes % 5 || body.busyMinutes < 5 || body.busyMinutes > 60)) return json(400);
      Object.assign(state, { mode: body.mode, busyMinutes: body.busyMinutes ?? null, resumeAt: body.resumeAt ?? null, changedBy: me, changedAt: new Date().toISOString() });
      record(body);
    } else if (changing[1] === "closed-day") {
      if (body.day < today) return json(400);
      state.closedDays = state.closedDays.filter(({ day }) => day !== body.day);
      if (body.closed) state.closedDays.push({ day: body.day, addedBy: me, addedAt: new Date().toISOString() });
      record({ closedDay: body.day, closed: body.closed });
    } else {
      state.soldOut = state.soldOut.filter(({ sku }) => !body.products.includes(sku));
      if (body.soldOut) state.soldOut.push(...body.products.map((sku) => ({ sku, by: me, at: new Date().toISOString() })));
      record({ soldOut: body.soldOut, products: body.products });
    }
    return json(200, { ...state, me });
  }
  if (/^\/v2\/operations\/state\/[0-9a-f-]+$/.test(url.pathname)) return json(200, { ...state, me });
  if (/^\/v2\/operations\/catalog\/[0-9a-f-]+$/.test(url.pathname)) return json(200, { receiptScope: "ef".repeat(32), catalog, photos: [],
    asset: { address: asset, symbol: "tKRW", decimals: 2, chainId: 8283, token: { name: "NU54 Test KRW", symbol: "tKRW", label: "tKRW", decimals: 2 } } });
  const file = path.join(dist, url.pathname.replace(/^\/ops\/?/, "") || "index.html");
  if (!file.startsWith(dist) || !fs.existsSync(file) || !fs.statSync(file).isFile()) return json(404);
  res.writeHead(200, { "Content-Type": { ".html": "text/html", ".js": "text/javascript", ".css": "text/css" }[path.extname(file)] ?? "application/octet-stream" });
  fs.createReadStream(file).pipe(res);
});

async function main() {
  await new Promise((done) => server.listen(0, "127.0.0.1", done));
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  const errors = [];
  const page = await (await browser.newContext({ viewport: { width: 1280, height: 900 }, reducedMotion: "reduce" })).newPage();
  page.setDefaultTimeout(8000);
  page.on("pageerror", (error) => errors.push(error.message));
  const button = (name, scope = page) => scope.getByRole("button", { name, exact: true });
  const shot = async (name) => { if (shots) await page.screenshot({ path: path.join(shots, name + ".png"), fullPage: true }); };
  const fits = async (name) => {
    for (const [width, font] of [[1280, "100%"], [390, "100%"], [390, "200%"]]) {
      await page.setViewportSize({ width, height: 900 });
      await page.evaluate((size) => { document.documentElement.style.fontSize = size; }, font);
      const wide = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      assert.ok(wide <= 0, `${name} at ${width} ${font}: ${wide}px too wide`);
      if (width === 390) await shot(`${name}-390-${font.replace("%", "")}`);
    }
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.evaluate(() => { document.documentElement.style.fontSize = ""; });
    await shot(name);
  };
  const confirm = page.getByRole("alertdialog", { name: "바꾸기 전 확인" });
  const log = page.getByRole("list", { name: "바꾼 기록" });
  try {
    await page.goto(`http://127.0.0.1:${server.address().port}/ops/`);
    await button("카탈로그").click();
    await page.getByLabel("담당 지점").selectOption(branch);
    await button("운영 권한 및 메뉴 조회").click();
    await page.getByText("지금 상태: 받는 중").waitFor();
    assert.equal(await button("지금 상태와 같습니다").isDisabled(), true);
    await log.getByText("아직 바꾼 것이 없습니다.").waitFor();

    // Busy for 25 minutes: picked, asked once more, then recorded as mine.
    await page.getByRole("radio", { name: /바쁨/ }).check();
    for (let i = 0; i < 2; i++) await button("5분 늘리기").click();
    await page.getByText("25분", { exact: true }).waitFor();
    await button("바쁨 · 약 25분(으)로 바꾸기").click();
    await confirm.getByRole("heading", { name: "바쁨 · 약 25분(으)로 바꿀까요?" }).waitFor();
    await fits("state-confirm");
    assert.equal(sent.length, 0, "nothing is sent before it is confirmed");
    await button("바꾸기", confirm).click();
    await page.getByText(/지금 상태: 바쁨 · 약 25분 · .* · 바꾼 사람 나/).waitFor();
    assert.deepEqual(sent.pop(), ["mode", { mode: "busy", busyMinutes: 25 }]);
    await log.getByText("바쁨 · 약 25분").waitFor();

    // Paused with the time it says; cancelling the question sends nothing.
    await page.getByRole("radio", { name: /멈춤/ }).check();
    await page.getByLabel(/^다시 받을 시각/).fill("11:20");
    await button("멈춤 · 11:20쯤 다시 받음으로 알림(으)로 바꾸기").click();
    await button("취소", confirm).click();
    assert.equal(sent.length, 0);
    await button("멈춤 · 11:20쯤 다시 받음으로 알림(으)로 바꾸기").click();
    await button("바꾸기", confirm).click();
    await page.getByText(/지금 상태: 멈춤 · 11:20쯤 다시 받음으로 알림/).waitFor();
    assert.deepEqual(sent.pop(), ["mode", { mode: "paused", resumeAt: "11:20" }]);

    // A closed day: added, shown with who set it, then taken back. A past day cannot be picked.
    assert.equal(await page.getByLabel("날짜", { exact: true }).getAttribute("min"), today);
    await page.getByLabel("날짜", { exact: true }).fill("2026-11-30");
    await button("휴무일 더하기").click();
    await button("바꾸기", confirm).click();
    const days = page.getByRole("list", { name: "정한 휴무일" });
    await days.getByText(/2026년 11월 30일 \(월\)/).waitFor();
    await days.getByText(/정한 사람 나/).waitFor();
    await fits("state-closed-day");
    await days.getByRole("button", { name: /11월 30일 .* 휴무 빼기/ }).click();
    await button("바꾸기", confirm).click();
    await days.getByText("정한 휴무일이 없습니다.").waitFor();

    // Sold out: two products at once; one sold out by someone else shows who, by a short reference.
    const products = page.getByRole("list", { name: "상품" });
    await products.getByText(/품절 · .* · 운영자 abcd/).waitFor();
    await products.getByText("조합 2줄 · 남은 수량 50").waitFor();
    await products.getByLabel(/카페라떼/).check();
    await products.getByLabel(/치즈케이크/).check();
    await button("고른 2개를 품절로").click();
    await button("바꾸기", confirm).click();
    assert.deepEqual(sent.pop(), ["sold-out", { products: ["latte", "cake"], soldOut: true }]);
    await log.getByText("품절로 · 카페라떼, 치즈케이크").waitFor();
    await products.getByLabel(/콜드브루/).check();
    await button("고른 것을 다시 팔기").click();
    await button("바꾸기", confirm).click();
    await log.getByText("다시 팔기 · 콜드브루").waitFor();
    await page.getByRole("searchbox", { name: "찾기" }).fill("케이크");
    assert.equal(await products.getByRole("listitem").count(), 1);
    await fits("state-sold-out");

    // The record lists every change, newest first.
    assert.deepEqual(await log.getByRole("listitem").locator("strong").allTextContents(), [
      "다시 팔기 · 콜드브루", "품절로 · 카페라떼, 치즈케이크", "휴무일 뺌 · 2026년 11월 30일 (월)", "휴무일 더함 · 2026년 11월 30일 (월)",
      "멈춤 · 11:20쯤 다시 받음으로 알림", "바쁨 · 약 25분"]);
    assert.deepEqual(errors, []);
    console.log("order state: busy and paused asked once more, closed days, several sold out at once, the record, 390px and 2x text");
  } finally {
    await browser.close();
    server.close();
  }
}
main().catch((error) => { console.error(error); process.exit(1); });

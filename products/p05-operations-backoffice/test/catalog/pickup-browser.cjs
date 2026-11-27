#!/usr/bin/env node
"use strict";
// Browser check of the order check and its pickup QR scan against a local mock of opsd. The camera and the browser's
// barcode reader are stand-ins, so this checks the page, not a real camera:
//   pnpm --filter @nu54/p05-backoffice-web build
//   NU54_PLAYWRIGHT_MODULE=<playwright> node test/catalog/pickup-browser.cjs [screenshot dir]

const assert = require("node:assert/strict");
const fs = require("node:fs");
const http = require("node:http");
const path = require("node:path");
const { chromium } = require(process.env.NU54_PLAYWRIGHT_MODULE || "playwright");
const shots = process.argv[2];
const dist = path.join(__dirname, "../../web/dist");
const branch = "11111111-1111-4111-8111-111111111111";
const me = "22222222-2222-4222-8222-222222222222";
const asset = "0x" + "ab".repeat(20);
const token = "5e".repeat(32), qr = `nu54://pickup/${token}`;
const catalog = {
  version: 1, timezone: "Asia/Seoul", expiresAt: new Date(Date.now() + 86400000).toISOString(),
  rows: [{ sku: "latte", option: "hot", label: "카페라떼", asset, price: "500000", available: 50 }],
  breakdown: [], options: [], categories: [], categoryBindings: [],
};
const state = { mode: "open", busyMinutes: null, resumeAt: null, changedBy: null, changedAt: null, today: "2026-11-27",
  closedDays: [], soldOut: [], log: [], autoKitchen: false };
const sale17 = "44444444-4444-4444-8444-444444444417";
const orders = {
  17: { sale: sale17, number: 17, stage: "ready", revision: 2, amount: "1250000", asset,
    items: [{ label: "카페라떼", option: "뜨겁게 크게 샷 추가", quantity: 1 }, { label: "치즈케이크", quantity: 1 }] },
  18: { sale: "44444444-4444-4444-8444-444444444418", number: 18, stage: "preparing", revision: 1, amount: "450000", asset, items: [{ label: "아메리카노", quantity: 1 }] },
};
// What the next scan answers: the handover, or one of opsd's refusals.
let answer = "handed";
const scans = [];

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, "http://localhost");
  const json = (status, body) => { res.writeHead(status, { "Content-Type": "application/json" }); res.end(JSON.stringify(body ?? {})); };
  let raw = "";
  for await (const part of req) raw += part;
  if (url.pathname.endsWith("/scopes")) return json(200, [{ id: branch, label: "여울 커피", timezone: "Asia/Seoul", capabilities: ["catalog"] }]);
  const scan = /^\/v2\/operations\/pickup-scan\/([0-9a-f-]+)\/([0-9a-f-]+)$/.exec(url.pathname);
  if (scan && req.method === "POST") {
    scans.push({ branch: scan[1], sale: scan[2], type: req.headers["content-type"], body: JSON.parse(raw) });
    const order = Object.values(orders).find(({ sale }) => sale === scan[2]);
    if (answer === "expired") return json(400, { status: "error", error: "EXPIRED" });
    if (answer === "other") return json(400, { status: "error", error: "MISMATCHED_EVIDENCE" });
    if (answer === "not-ready") return json(409, { status: "error", error: "CHECKOUT_CONFLICT" });
    Object.assign(order, { stage: "collected", revision: order.revision + 1 });
    return json(200, { saleRef: order.sale, revision: order.revision, pickupNo: order.number,
      items: order.items.map(({ label, option, quantity }) => ({ label, option: option ?? "standard", quantity })) });
  }
  if (/^\/v2\/operations\/state\/[0-9a-f-]+\/order$/.test(url.pathname)) {
    const found = orders[url.searchParams.get("number")];
    return found ? json(200, found) : json(404);
  }
  if (/^\/v2\/operations\/state\/[0-9a-f-]+$/.test(url.pathname)) return json(200, { ...state, me });
  if (/^\/v2\/operations\/catalog\/[0-9a-f-]+$/.test(url.pathname)) return json(200, { receiptScope: "ef".repeat(32), catalog, photos: [],
    asset: { address: asset, symbol: "tKRW", decimals: 2, chainId: 8283, token: { name: "NU54 Test KRW", symbol: "tKRW", label: "tKRW", decimals: 2 } } });
  const file = path.join(dist, url.pathname.replace(/^\/ops\/?/, "") || "index.html");
  if (!file.startsWith(dist) || !fs.existsSync(file) || !fs.statSync(file).isFile()) return json(404);
  res.writeHead(200, { "Content-Type": { ".html": "text/html", ".js": "text/javascript", ".css": "text/css" }[path.extname(file)] ?? "application/octet-stream" });
  fs.createReadStream(file).pipe(res);
});

// A camera made of a canvas, and a barcode reader that reads whatever the test puts in window.codes.
const fakeCamera = () => {
  window.tracks = [];
  window.codes = [];
  Object.defineProperty(navigator, "mediaDevices", { value: { getUserMedia: async () => {
    const canvas = document.createElement("canvas");
    canvas.width = 320; canvas.height = 240;
    canvas.getContext("2d").fillRect(0, 0, 20, 20);
    const stream = canvas.captureStream(5);
    window.tracks.push(...stream.getTracks());
    return stream;
  } } });
  window.BarcodeDetector = class {
    static async getSupportedFormats() { return ["qr_code", "ean_13"]; }
    async detect() { const value = window.codes.shift(); return value ? [{ rawValue: value }] : []; }
  };
};

async function main() {
  await new Promise((done) => server.listen(0, "127.0.0.1", done));
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  const errors = [];
  const open = async (camera) => {
    const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, reducedMotion: "reduce" });
    if (camera) await context.addInitScript(fakeCamera);
    const page = await context.newPage();
    page.setDefaultTimeout(8000);
    page.on("pageerror", (error) => errors.push(error.message));
    await page.goto(`http://127.0.0.1:${server.address().port}/ops/`);
    await page.getByRole("button", { name: "카탈로그", exact: true }).click();
    await page.getByLabel("담당 지점").selectOption(branch);
    await page.getByRole("button", { name: "운영 권한 및 메뉴 조회", exact: true }).click();
    return page;
  };
  const fits = async (page, name) => {
    for (const [width, font] of [[1280, "100%"], [390, "100%"], [390, "200%"]]) {
      await page.setViewportSize({ width, height: 900 });
      await page.evaluate((size) => { document.documentElement.style.fontSize = size; }, font);
      const wide = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      assert.ok(wide <= 0, `${name} at ${width} ${font}: ${wide}px too wide`);
      if (shots && width === 390) await page.getByRole("region", { name: /^주문 확인/ }).screenshot({ path: path.join(shots, `${name}-390-${font.replace("%", "")}.png`) });
    }
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.evaluate(() => { document.documentElement.style.fontSize = ""; });
    if (shots) await page.getByRole("region", { name: /^주문 확인/ }).screenshot({ path: path.join(shots, name + ".png") });
  };
  const live = (page) => page.evaluate(() => window.tracks.filter((track) => track.readyState === "live").length);
  try {
    // The camera reads a QR that is not a pickup QR, then the pickup QR of the order that is open.
    let page = await open(true);
    const check = page.getByRole("region", { name: /^주문 확인/ });
    await check.getByLabel("주문번호").fill("18");
    await check.getByRole("button", { name: "열기", exact: true }).click();
    await check.getByText("아직 준비 전인 주문입니다.", { exact: false }).waitFor();
    assert.equal(await check.getByRole("button", { name: "수령 QR 확인" }).count(), 0, "no scan before the order is ready");
    await check.getByLabel("주문번호").fill("17");
    await check.getByRole("button", { name: "열기", exact: true }).click();
    await check.getByRole("heading", { name: "주문 확인 · 17번" }).waitFor();
    await check.getByRole("button", { name: "수령 QR 확인" }).click();
    await check.getByRole("heading", { name: "17번의 수령 QR 확인" }).waitFor();
    await page.waitForFunction(() => window.tracks.length === 1 && window.tracks[0].readyState === "live");
    await fits(page, "pickup-scan");
    await page.evaluate(() => window.codes.push("nu54://order?v=2"));
    await check.getByText("수령 QR이 아닙니다.", { exact: false }).waitFor();
    assert.equal(scans.length, 0, "a QR of another kind is not sent");
    await page.evaluate((text) => window.codes.push(text), qr);
    await check.getByText("17번 · 건네 주세요").waitFor();
    assert.deepEqual(scans.pop(), { branch, sale: sale17, type: "application/json", body: { token } });
    await check.getByText(/결제한 보드로 확인한 손님입니다\. \d\d:\d\d에 전달을 기록했습니다\./).waitFor();
    assert.deepEqual(await check.getByRole("list", { name: "건넬 품목" }).getByRole("listitem").allTextContents(),
      ["카페라떼 뜨겁게 크게 샷 추가 1개", "치즈케이크 1개"]);
    await check.getByText("손님이 받았어요를 누르거나 확인 기간이 끝날 때").waitFor();
    await check.getByText(/\d\d:\d\d · 이 기기/).waitFor();
    assert.equal(await live(page), 0, "the camera stops once the QR is read");
    await fits(page, "pickup-handed");
    await check.getByRole("button", { name: "다음 손님 찍기" }).click();
    assert.equal(await check.getByLabel("주문번호").inputValue(), "");

    // A handheld scanner types into the field and ends with Enter. Each refusal says not to hand over.
    for (const [reply, expect] of [["expired", "이 중 하나"], ["other", "이 경우"], ["not-ready", "이 경우"]]) {
      orders[17].stage = "ready";
      answer = reply;
      await check.getByLabel("주문번호").fill("17");
      await check.getByRole("button", { name: "열기", exact: true }).click();
      await check.getByRole("button", { name: "수령 QR 확인" }).click();
      const typed = check.getByLabel(/^수령 QR 글자/);
      await typed.pressSequentially(qr);
      await typed.press("Enter");
      await check.getByText("건네지 마세요 · 쓸 수 없는 QR입니다").waitFor();
      const marked = await check.getByRole("list", { name: "쓸 수 없는 QR의 경우" }).locator("li.matched strong").allTextContents();
      if (reply === "expired") assert.deepEqual(marked, ["시간이 지난 QR · 이 중 하나", "이미 쓴 QR · 이 중 하나", "다른 매장의 주문 · 이 중 하나"]);
      if (reply === "other") assert.deepEqual(marked, [`다른 주문의 QR · ${expect}`]);
      if (reply === "not-ready") assert.deepEqual(marked, [`아직 준비 전인 주문 · ${expect}`]);
      await check.getByText("지금 연 17번이 아니라 다른 번호의 QR입니다. 그 주문을 열고 다시 찍으세요.").waitFor();
      assert.equal(await live(page), 0);
      if (reply === "expired") await fits(page, "pickup-refused");
      await check.getByRole("button", { name: "다시 찍기" }).click();
      await check.getByRole("heading", { name: "17번의 수령 QR 확인" }).waitFor();
      await page.waitForFunction(() => window.tracks.some((track) => track.readyState === "live"));
      await check.getByRole("button", { name: "닫기" }).click();
      assert.equal(await live(page), 0, "leaving the order stops the camera");
    }
    assert.equal(scans.length, 3);

    // A token used before reads as used once the order shows the handover.
    orders[17].stage = "ready";
    answer = "expired";
    await check.getByLabel("주문번호").fill("17");
    await check.getByRole("button", { name: "열기", exact: true }).click();
    await check.getByRole("button", { name: "수령 QR 확인" }).click();
    orders[17].stage = "collected";
    await check.getByLabel(/^수령 QR 글자/).fill(qr);
    await check.getByRole("button", { name: "확인", exact: true }).click();
    await check.getByText("건네지 마세요 · 쓸 수 없는 QR입니다").waitFor();
    assert.deepEqual(await check.locator("li.matched strong").allTextContents(), ["이미 쓴 QR · 이 경우"]);
    await page.context().close();

    // Without the browser's barcode reader, the field is the way in.
    page = await open(false);
    await page.evaluate(() => { delete window.BarcodeDetector; });
    const plain = page.getByRole("region", { name: /^주문 확인/ });
    orders[17].stage = "ready";
    answer = "handed";
    await plain.getByLabel("주문번호").fill("17");
    await plain.getByRole("button", { name: "열기", exact: true }).click();
    await plain.getByRole("button", { name: "수령 QR 확인" }).click();
    await plain.getByText("이 브라우저에서는 카메라로 QR을 읽지 못합니다.", { exact: false }).waitFor();
    await plain.getByLabel(/^수령 QR 글자/).fill(`  ${qr} `);
    await plain.getByRole("button", { name: "확인", exact: true }).click();
    await plain.getByText("17번 · 건네 주세요").waitFor();
    assert.deepEqual(scans.pop().body, { token });
    await fits(page, "pickup-no-camera");
    assert.deepEqual(errors, []);
    console.log("pickup QR: camera and scanner field, handed, four refusals, camera stopped on result and on leaving, 390px and 2x text");
  } finally {
    await browser.close();
    server.close();
  }
}
main().catch((error) => { console.error(error); process.exit(1); });

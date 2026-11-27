#!/usr/bin/env node
"use strict";
// Browser check of the store page editor against a local mock of opsd:
//   pnpm --filter @nu54/p05-backoffice-web build
//   NU54_PLAYWRIGHT_MODULE=<playwright> node test/catalog/store-page-browser.cjs [screenshot dir]

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
const catalog = {
  version: 1, timezone: "Asia/Seoul", expiresAt: new Date(Date.now() + 86400000).toISOString(),
  rows: [{ sku: "latte", option: "hot", label: "카페라떼", asset, price: "500000", available: 50 }],
  breakdown: [], options: [], categories: [], categoryBindings: [],
};
const state = { mode: "open", busyMinutes: null, resumeAt: null, changedBy: null, changedAt: null, today: "2026-11-27",
  closedDays: [], soldOut: [], log: [], autoKitchen: false };
// What opsd keeps for the page; `refuse` makes the next save a refusal.
let page = { name: "여울 커피", saved: false, published: false, listed: false, kind: "", intro: "", address: "", directions: "", phone: "", amenities: "",
  parking: "", closedNote: "", hours: null, changedBy: null, changedAt: null };
let refuse = false;
const saves = [];

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, "http://localhost");
  const json = (status, body) => { res.writeHead(status, { "Content-Type": "application/json" }); res.end(JSON.stringify(body ?? {})); };
  let raw = "";
  for await (const part of req) raw += part;
  if (url.pathname.endsWith("/scopes")) return json(200, [{ id: branch, label: "여울 커피", timezone: "Asia/Seoul", capabilities: ["catalog"] }]);
  if (/^\/v2\/operations\/store-page\/[0-9a-f-]+$/.test(url.pathname)) {
    if (req.method === "GET") return json(200, page);
    const body = JSON.parse(raw);
    saves.push({ type: req.headers["content-type"], body });
    if (refuse) return json(400, { status: "error", error: "BAD_REQUEST" });
    page = { ...page, ...body, saved: true, changedBy: me, changedAt: "2026-11-27T01:31:00Z" };
    return json(200, page);
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
  try {
    const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, reducedMotion: "reduce" });
    const tab = await context.newPage();
    tab.setDefaultTimeout(8000);
    tab.on("pageerror", (error) => errors.push(error.message));
    await tab.goto(`http://127.0.0.1:${server.address().port}/ops/`);
    await tab.getByRole("button", { name: "카탈로그", exact: true }).click();
    await tab.getByLabel("담당 지점").selectOption(branch);
    await tab.getByRole("button", { name: "운영 권한 및 메뉴 조회", exact: true }).click();
    const editor = tab.getByRole("region", { name: "매장 화면", exact: true });
    const fits = async (name) => {
      for (const [width, font] of [[390, "100%"], [390, "200%"]]) {
        await tab.setViewportSize({ width, height: 900 });
        await tab.evaluate((size) => { document.documentElement.style.fontSize = size; }, font);
        const wide = await tab.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
        assert.ok(wide <= 0, `${name} at ${width} ${font}: ${wide}px too wide`);
        if (shots) await editor.screenshot({ path: path.join(shots, `${name}-390-${font.replace("%", "")}.png`) });
      }
      await tab.setViewportSize({ width: 1280, height: 900 });
      await tab.evaluate(() => { document.documentElement.style.fontSize = ""; });
    };
    const save = editor.getByRole("button", { name: "매장 화면 저장" });

    // Nothing saved yet: the name comes from the branch, and the page cannot be saved without the store's kind.
    await editor.getByText("아직 저장한 매장 화면이 없습니다. 공개하기 전에는 손님 앱에서 이 매장의 화면과 메뉴가 열리지 않습니다.").waitFor();
    assert.equal(await editor.getByLabel("매장 화면 공개").isChecked(), false);
    await editor.getByText("여울 커피", { exact: true }).waitFor();
    await editor.getByText("매장 종류를 적어 주세요. 예: 카페").waitFor();
    assert.equal(await save.isDisabled(), true);

    await editor.getByLabel("매장 종류").fill(" 카페 ");
    await editor.getByLabel("소개").fill("직접 볶은 원두로 내립니다");
    await editor.getByLabel("주소").fill("시험시 시험로 1");
    await editor.getByLabel("전화").fill("전화 주세요");
    await editor.getByText("전화번호는 숫자, 빈칸, 하이픈으로 적어 주세요.").waitFor();
    assert.equal(await save.isDisabled(), true);
    await editor.getByLabel("전화").fill("02-555-0100");
    // Listing without publishing shows nothing to customers, and the editor says so until the page is made public.
    await editor.getByLabel("손님 앱의 매장 목록에 보이기").check();
    await editor.getByText("매장 목록에는 매장 화면을 공개한 동안에만 보입니다.").waitFor();
    await editor.getByLabel("매장 화면 공개").check();
    assert.equal(await editor.getByText("매장 목록에는 매장 화면을 공개한 동안에만 보입니다.").count(), 0);

    // Hours: weekdays and Saturday open, Sunday closed; the lines customers will read are shown before the save.
    await editor.getByLabel("영업시간을 손님에게 보이기").check();
    for (const day of ["월", "화", "수", "목", "금", "토"]) await editor.getByLabel(`${day}요일 영업`).check();
    await editor.getByLabel("토요일 여는 시간").fill("11:00");
    await editor.getByLabel("토요일 닫는 시간").fill("11:00");
    await editor.getByText("토요일의 여는 시간과 닫는 시간이 같습니다.").waitFor();
    assert.equal(await save.isDisabled(), true);
    await editor.getByLabel("토요일 닫는 시간").fill("20:00");
    assert.deepEqual(await editor.getByRole("list", { name: "손님에게 보이는 영업시간" }).getByRole("listitem").allTextContents(),
      ["월~금 10:00~21:00", "토 11:00~20:00", "일 쉼"]);
    await fits("store-page-editor");

    // A refused save keeps what was typed; the next one is stored and read back.
    refuse = true;
    await save.click();
    await editor.getByText("저장할 수 없는 값입니다. 다시 확인해 주세요.").waitFor();
    assert.equal(await editor.getByLabel("소개").inputValue(), "직접 볶은 원두로 내립니다");
    refuse = false;
    await save.click();
    await editor.getByText("매장 화면을 저장했습니다.").waitFor();
    const day = { opens: "10:00", closes: "21:00" };
    assert.deepEqual(saves.pop(), { type: "application/json", body: { published: true, listed: true, kind: "카페", intro: "직접 볶은 원두로 내립니다",
      address: "시험시 시험로 1", directions: "", phone: "02-555-0100", amenities: "", parking: "", closedNote: "",
      hours: [day, day, day, day, day, { opens: "11:00", closes: "20:00" }, null] } });
    await editor.getByText("지금: 공개 · 매장 목록에 보임 · 11월 27일 10:31에 저장").waitFor();
    assert.equal(await editor.getByLabel("매장 종류").inputValue(), "카페");

    // Taking the hours and the listing back sends null hours and an unlisted page.
    await editor.getByLabel("영업시간을 손님에게 보이기").uncheck();
    await editor.getByLabel("손님 앱의 매장 목록에 보이기").uncheck();
    await save.click();
    await editor.getByText("지금: 공개 · 매장 목록에 안 보임 · 매장 바로가기 QR로만 열림", { exact: false }).waitFor();
    const last = saves.pop().body;
    assert.deepEqual([last.published, last.listed, last.hours], [true, false, null]);

    // Taking the page back: nothing of the store opens in the customer app any more.
    await editor.getByLabel("매장 화면 공개").uncheck();
    await save.click();
    await editor.getByText("지금: 비공개 · 손님 앱에서 매장 화면과 메뉴가 열리지 않음", { exact: false }).waitFor();
    assert.equal(saves.pop().body.published, false);
    assert.deepEqual(errors, []);
    console.log("store page editor: required kind, phone and hours checks, hours preview, refused save, published and listed page, unlisted without hours, page taken back, 390px and 2x text");
  } finally {
    await browser.close();
    server.close();
  }
}
main().catch((error) => { console.error(error); process.exit(1); });

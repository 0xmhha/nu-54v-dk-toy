#!/usr/bin/env node
"use strict";
// Browser check of the menu's option groups against a local mock. It needs no database or certificate:
//   pnpm --filter @nu54/p05-backoffice-web build
//   NU54_PLAYWRIGHT_MODULE=<playwright> node test/catalog/options-browser.cjs [screenshot dir]

const assert = require("node:assert/strict");
const fs = require("node:fs");
const http = require("node:http");
const path = require("node:path");
const { chromium } = require(process.env.NU54_PLAYWRIGHT_MODULE || "playwright");
const shots = process.argv[2];
const dist = path.join(__dirname, "../../web/dist");
const branch = "11111111-1111-4111-8111-111111111111";
const asset = "0x" + "ab".repeat(20);
const published = [], stocked = [];
let refuse = false;
let catalog = {
  version: 1, timezone: "Asia/Seoul", expiresAt: new Date(Date.now() + 86400000).toISOString(),
  rows: [
    { sku: "cake", option: "standard", label: "치즈케이크", asset, price: "650000", available: 5 },
    { sku: "beans", option: "whole", label: "원두 200g", asset, price: "1800000", available: 5 }, { sku: "beans", option: "ground", label: "원두 200g", asset, price: "1800000", available: 5 },
  ],
  breakdown: [{ sku: "beans", option: "whole", displayPrice: "1800000", tax: "0", discount: "0", required: true }, { sku: "beans", option: "ground", displayPrice: "1800000", tax: "0", discount: "0", required: true }],
  options: [], categories: [], categoryBindings: [],
};

// What opsd answers after a publication: one row for each combination, and the groups as they were sent.
function accept({ rows, options = [] }) {
  const made = [], breakdown = [];
  for (const row of rows) {
    const option = options.find(({ sku }) => sku === row.sku);
    const groups = option ? option.groups : [];
    const lines = groups.reduce((list, { values }) => list.flatMap((line) => values.map((value) => [...line, value])), [[]]);
    for (const line of lines) {
      const code = line.map(({ key }) => key).join("-") || row.option;
      const price = String((Number(row.displayPrice) + line.reduce((sum, { price }) => sum + Number(price), 0)) * 100);
      made.push({ sku: row.sku, option: code, label: row.label, asset, price, available: 0 });
      breakdown.push({ sku: row.sku, option: code, displayPrice: price, tax: "0", discount: "0", required: row.required,
        metadata: { ...row.metadata, ...(option ? { optionLabels: { ko: line.map(({ labels }) => labels.ko).join(" · ") } } : {}) } });
    }
  }
  const units = (amount) => String(Number(amount) * 100);
  catalog = { ...catalog, version: catalog.version + 1, rows: made, breakdown, options: options.map(({ sku, groups }) => {
    const row = rows.find((item) => item.sku === sku);
    return { sku, base: { label: row.label, displayPrice: units(row.displayPrice), tax: units(row.tax), discount: units(row.discount), metadata: row.metadata },
      groups: groups.map((group) => ({ ...group, values: group.values.map((value) => ({ ...value, price: units(value.price) })) })) };
  }) };
}

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, "http://localhost");
  const json = (status, body) => { res.writeHead(status, { "Content-Type": "application/json" }); res.end(JSON.stringify(body ?? {})); };
  let raw = "";
  for await (const part of req) raw += part;
  if (url.pathname.endsWith("/scopes")) return json(200, [{ id: branch, label: "여울 커피", timezone: "Asia/Seoul", capabilities: ["catalog"] }]);
  const call = /^\/v2\/operations\/catalog\/[0-9a-f-]+(\/[a-z-]+)?$/.exec(url.pathname);
  if (call && req.method === "POST") {
    const body = JSON.parse(raw);
    if (refuse) return json(400);
    if (call[1] === "/publish") { published.push(body); accept(body); } else stocked.push([call[1], body]);
    return json(200, {});
  }
  if (call) return json(200, { receiptScope: "ef".repeat(32), catalog,
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
  const page = await (await browser.newContext({ viewport: { width: 1280, height: 900 } })).newPage();
  page.setDefaultTimeout(8000);
  page.on("pageerror", (error) => errors.push(error.message));
  const button = (name, scope = page) => scope.getByRole("button", { name, exact: true });
  const shot = async (name) => { if (shots) await page.screenshot({ path: path.join(shots, name + ".png"), fullPage: true }); };
  const editor = page.getByRole("form", { name: "메뉴 고치기" });
  const group = (n) => editor.getByRole("group", { name: "묶음 " + n, exact: true });
  const fields = (n, v) => group(n).getByRole("group", { name: "값 " + v, exact: true });
  async function fill(n, name, values) {
    await group(n).getByLabel("묶음 이름", { exact: true }).fill(name);
    for (const [index, [label, price]] of values.entries()) {
      if (index > 1) await group(n).getByRole("button", { name: /^값 추가/ }).click();
      await fields(n, index + 1).getByLabel("값 이름", { exact: true }).fill(label);
      if (price) await fields(n, index + 1).getByLabel("더하는 금액(원)", { exact: true }).fill(price);
    }
  }
  const publish = async () => {
    await button("게시 내용 검토").click();
    await button("변경 적용").click();
    await page.getByRole("status").filter({ hasText: "새 메뉴 버전의 게시를 확인했습니다." }).waitFor();
  };
  // No part of the page may be wider than the window, at 1280 and at phone width with enlarged text.
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
  try {
    await page.goto(`http://127.0.0.1:${server.address().port}/ops/`);
    await button("카탈로그").click();
    await page.getByLabel("담당 지점").selectOption(branch);
    await button("운영 권한 및 메뉴 조회").click();
    await page.getByRole("heading", { name: "메뉴 2개 · 게시 줄 3 / 600", exact: true }).waitFor();
    await page.getByText("옵션 목록 2개").waitFor();
    await button("치즈케이크 고치기").click();
    assert.equal(await page.evaluate(() => document.activeElement.closest("[data-sku]").dataset.sku), "cake");

    // A new product with three groups.
    await button("상품 추가").click();
    await editor.getByLabel("상품 코드", { exact: true }).fill("cake");
    await editor.getByText("이미 쓰는 상품 코드입니다.").waitFor();
    await editor.getByLabel("상품 코드", { exact: true }).fill("latte");
    await editor.getByLabel("상품 이름", { exact: true }).fill("카페라떼");
    await editor.getByLabel(/^가격\(원\)/).fill("5000");
    await fill(1, "온도", [["뜨겁게"], ["차갑게"]]);
    await button("묶음 추가", editor).click();
    await fill(2, "크기", [["보통"], ["크게", "500"]]);
    await button("묶음 추가", editor).click();
    await fill(3, "우유", [["우유"], ["두유", "700"], ["귀리 음료", "700"]]);
    await editor.getByText("조합 12줄 · 5,000원부터 6,200원까지").waitFor();
    await editor.getByText("매장 전체 15 / 600줄").waitFor();

    // Refused beside the group it is about.
    const save = button("이 상품 저장", editor);
    await editor.getByLabel(/^세금/).fill("500");
    await group(2).getByRole("alert").filter({ hasText: "세금이 있는 상품" }).waitFor();
    assert.equal(await group(1).getByRole("alert").count(), 0);
    assert.equal(await save.isDisabled(), true);
    await editor.getByLabel(/^세금/).fill("0");
    await fields(3, 1).getByText(/더해지는 성분/).click();
    await fields(3, 1).getByLabel("우유", { exact: true }).check();
    await group(3).getByRole("alert").filter({ hasText: "알레르기 성분을 먼저" }).waitFor();
    await editor.getByLabel("점주 제공 성분을 게시").check();
    assert.equal(await editor.getByRole("alert").count(), 0);
    await fits("editor");
    await button("우유 지우기", group(3)).click();
    assert.equal(await group(3).getByRole("group", { name: /^값/ }).count(), 2);
    await group(3).getByRole("button", { name: /^값 추가/ }).click();
    await save.click();
    assert.equal(await editor.count(), 1, "a value without a name is not saved");
    await fields(3, 3).getByLabel("값 이름", { exact: true }).fill("우유");
    await fields(3, 3).getByText(/더해지는 성분/).click();
    await fields(3, 3).getByLabel("우유", { exact: true }).check();
    await button("묶음 3 위로", editor).click();
    await button("묶음 2 아래로", editor).click();
    await save.click();
    await page.getByRole("heading", { name: "메뉴 3개 · 게시 줄 15 / 600", exact: true }).waitFor();
    await fits("list");

    // Cancelling keeps what was saved.
    await button("카페라떼 고치기").click();
    await editor.getByLabel(/^가격\(원\)/).fill("9000");
    await button("되돌리기", editor).click();
    await page.getByText("12줄 · 5,000원부터 6,200원까지").waitFor();
    await button("카페라떼 고치기").click();
    await fill(3, "우유", [["두유"], ["귀리 음료"], ["우유"]]);
    assert.equal(await editor.getByLabel("상품 코드", { exact: true }).getAttribute("readonly"), "");
    await button("되돌리기", editor).click();

    // The table before publishing is what is charged.
    await button("게시 내용 검토").click();
    const review = page.getByRole("region", { name: "변경 확인" });
    await review.getByText("카페라떼 · 새로 더함, 12줄").waitFor();
    const table = await review.locator("tbody tr").allInnerTexts();
    assert.equal(table.length, 12);
    const cells = table.map((row) => row.split("\t").map((cell) => cell.trim()));
    for (const [name, base, extra, total, allergens] of cells) {
      const sum = (name.includes("크게") ? 500 : 0) + (/두유|귀리/.test(name) ? 700 : 0);
      assert.deepEqual([base, extra, total], ["5,000원", sum ? `+${sum.toLocaleString("en-US")}원` : "0원", `${(5000 + sum).toLocaleString("en-US")}원`], name);
      assert.equal(allergens, name.endsWith("우유") && !name.includes("두유") ? "우유" : "없음", name);
    }
    await fits("review");
    assert.equal(published.length, 0);
    refuse = true;
    await button("변경 적용").click();
    await page.getByRole("status").filter({ hasText: "앱이 읽을 수 있는 크기" }).waitFor();
    refuse = false;
    await page.getByRole("heading", { name: "메뉴 3개 · 게시 줄 15 / 600", exact: true }).waitFor();
    await publish();
    const [first] = published;
    assert.deepEqual(first.rows.find(({ sku }) => sku === "latte"), { sku: "latte", option: "", label: "카페라떼", asset, displayPrice: "5000", tax: "0", discount: "0", required: true, metadata: { allergens: [] } });
    const [{ groups }] = first.options;
    assert.deepEqual(groups.map(({ labels }) => labels.ko), ["온도", "크기", "우유"]);
    const codes = groups.flatMap(({ key, values }) => [key, ...values.map((value) => value.key)]);
    assert.equal(new Set(codes).size, codes.length);
    for (const code of codes) assert.match(code, /^[a-z0-9]{6}$/);

    // After publishing the codes never show; the names do. Codes stay when the product is edited.
    await page.getByText("뜨겁게 · 보통 · 두유").first().waitFor();
    assert.doesNotMatch(await page.evaluate(() => document.querySelector("main").innerText), new RegExp(groups[0].values[0].key));
    const stock = page.locator("section").filter({ has: page.getByRole("heading", { name: "재고 조정" }) });
    await stock.locator("select").first().selectOption("latte");
    const ice = `${groups[0].values[1].key}-${groups[1].values[1].key}-${groups[2].values[1].key}`;
    await stock.locator("select").nth(1).selectOption({ label: "차갑게 · 크게 · 귀리 음료" });
    await page.getByLabel("증감 수량").fill("0");
    await button("재고 조정 검토").click();
    await page.getByRole("region", { name: "변경 확인" }).getByText("차갑게 · 크게 · 귀리 음료").waitFor();
    await button("변경 적용").click();
    await page.getByRole("status").filter({ hasText: "재고 반영" }).waitFor();
    assert.deepEqual([stocked[0][0], stocked[0][1].option], ["/option-stock", ice]);
    await button("카페라떼 고치기").click();
    await button("귀리 음료 지우기", editor).click();
    await group(3).getByRole("button", { name: /^값 추가/ }).click();
    await fields(3, 3).getByLabel("값 이름", { exact: true }).fill("귀리 음료");
    await save.click();
    await button("게시 내용 검토").click();
    await review.getByText("카페라떼 · 12줄에서 12줄").waitFor();
    await review.getByText("새로 생긴 조합에는 옵션별 재고 한도가 없습니다.", { exact: false }).waitFor();
    await button("취소").click();
    await publish();
    const again = published[1].options[0].groups;
    assert.deepEqual(again[0], groups[0]);
    assert.equal(again[2].values.some(({ key }) => key === groups[2].values[1].key), false, "a removed code is not used again");

    // A single product gets groups, and a product without groups becomes a single one again.
    await button("옵션 묶음 만들기").click();
    await fill(1, "조각", [["한 조각"], ["두 조각", "6000"]]);
    await save.click();
    await button("카페라떼 고치기").click();
    for (let left = 3; left; left--) await button("묶음 1 지우기", editor).click();
    await editor.getByText("묶음이 없으면").waitFor();
    await save.click();
    await publish();
    assert.deepEqual(published[2].options.map(({ sku }) => sku), ["cake"]);
    assert.deepEqual(published[2].rows.map(({ sku, option }) => [sku, option]), [["beans", "whole"], ["beans", "ground"], ["latte", "standard"], ["cake", ""]]);
    await button("치즈케이크 고치기").click();
    await button("이 상품 빼기", editor).click();
    await publish();
    assert.equal(Object.hasOwn(published[3], "options"), false);

    // Six hundred rows for the whole menu.
    await button("상품 추가").click();
    await editor.getByLabel("상품 코드", { exact: true }).fill("wide");
    await editor.getByLabel("상품 이름", { exact: true }).fill("넓은 상품");
    await editor.getByLabel(/^가격\(원\)/).fill("1000");
    const many = (count) => Array.from({ length: count }, (_, index) => ["값" + index]);
    await fill(1, "하나", many(12));
    assert.equal(await group(1).getByRole("button", { name: /^값 추가/ }).isDisabled(), true);
    await button("묶음 추가", editor).click();
    await fill(2, "둘", many(10));
    await button("묶음 추가", editor).click();
    await fill(3, "셋", many(5));
    await editor.getByRole("alert").filter({ hasText: "매장 전체 603 / 600줄" }).waitFor();
    await editor.getByText("3줄을 줄여야 합니다.", { exact: false }).waitFor();
    assert.equal(await save.isDisabled(), true);
    await fits("too-many");
    await button("묶음 추가", editor).click();
    assert.equal(await button("묶음 추가", editor).isDisabled(), true);
    assert.deepEqual(errors, []);
    console.log("option groups: list, editor, refusals, table, publication, names instead of codes, stock by name, codes not reused, 600 rows, 390px and 2x text");
  } finally {
    await browser.close();
    server.close();
  }
}
main().catch((error) => { console.error(error); process.exit(1); });

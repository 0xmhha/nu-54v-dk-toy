#!/usr/bin/env node
"use strict";
// Sets a cafe menu with photos through the operations page of a local run, as an operator would:
//   node cafe-menu.cjs <origin> <branch> <cert> <key> [screenshot dir]
// The photos are this folder's drawings, rendered to PNG and uploaded; the server makes its own JPEGs from them.

const fs = require("node:fs");
const path = require("node:path");
const { chromium } = require(process.env.NU54_PLAYWRIGHT_MODULE || "playwright");
const [origin, branch, certPath, keyPath, shots] = process.argv.slice(2);

const coffee = ["coffee", "커피", "Coffee"], dessert = ["dessert", "디저트", "Dessert"];
const temperature = ["온도", "Temperature", [["뜨겁게", "Hot"], ["차갑게", "Iced"]]];
const size = ["크기", "Size", [["보통", "Regular"], ["크게", "Large", "500"]]];
const menu = [
  { sku: "latte", name: "카페라떼", en: "Cafe latte", price: "5000", art: "latte", category: coffee,
    groups: [temperature, size, ["샷", "Shots", [["기본", "Standard"], ["샷 추가", "Extra shot", "500"]]]] },
  { sku: "vanilla-latte", name: "바닐라 라떼", en: "Vanilla latte", price: "5500", art: "vanilla-latte", category: coffee,
    groups: [temperature, ["우유", "Milk", [["우유", "Milk"], ["두유", "Soy milk", "500"]]]] },
  { sku: "americano", name: "아메리카노", en: "Americano", price: "4500", art: "americano", category: coffee, groups: [temperature, size] },
  { sku: "cream-latte", name: "크림 라떼", en: "Cream latte", price: "6000", art: "cream-latte", category: coffee, groups: [] },
  { sku: "cold-brew", name: "콜드브루", en: "Cold brew", price: "5000", art: "cold-brew", category: coffee, groups: [] },
  // One product without a photo, to see that only its name is shown.
  { sku: "espresso", name: "에스프레소", en: "Espresso", price: "3500", category: coffee, groups: [] },
  { sku: "cheesecake", name: "치즈케이크", en: "Cheesecake", price: "6500", art: "cheesecake", category: dessert, groups: [] },
];

async function main() {
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  try {
    // The drawings as photos a phone would take: PNG files of 1600×1200.
    const painter = await browser.newPage({ viewport: { width: 1600, height: 1200 } });
    const photo = {};
    for (const item of menu.filter(({ art }) => art)) {
      const svg = fs.readFileSync(path.join(__dirname, "cafe", item.art + ".svg"), "utf8");
      await painter.setContent(`<body style="margin:0">${svg.replace("<svg ", '<svg style="width:1600px;height:1200px;display:block" ')}</body>`);
      photo[item.sku] = await painter.screenshot({ type: "png" });
    }
    await painter.close();

    const page = await (await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1280, height: 900 },
      clientCertificates: [{ origin, certPath, keyPath }] })).newPage();
    page.setDefaultTimeout(20000);
    const button = (name, scope = page) => scope.getByRole("button", { name, exact: true });
    await page.goto(origin + "/ops/");
    await button("카탈로그").click();
    await page.getByLabel("담당 지점").selectOption(branch);
    await button("운영 권한 및 메뉴 조회").click();
    await page.getByRole("heading", { name: /^메뉴 \d+개/ }).waitFor();

    // What the test store had before goes away.
    const rows = page.getByRole("form", { name: "옵션 목록 고치기" });
    for (const label of ["Escrow cafe"]) {
      await button(`${label} 고치기`).click();
      while (await button("이 옵션 제거", rows).count()) await button("이 옵션 제거", rows).first().click();
      await button("이 상품 저장", rows).click();
    }
    const editor = page.getByRole("form", { name: "메뉴 고치기" });
    if (await button("Option latte 고치기").count()) {
      await button("Option latte 고치기").click();
      await button("이 상품 빼기", editor).click();
    }

    for (const item of menu) {
      await button("상품 추가").click();
      await editor.getByLabel("상품 코드", { exact: true }).fill(item.sku);
      await editor.getByLabel("상품 이름", { exact: true }).fill(item.name);
      await editor.getByLabel(/^가격\(/).fill(item.price);
      await editor.getByLabel("상품 영어 이름", { exact: true }).fill(item.en);
      const groups = editor.getByRole("group", { name: /^묶음 \d+$/ });
      for (const [index, [name, en, values]] of item.groups.entries()) {
        if (index) await button("묶음 추가", editor).click();
        const group = groups.nth(index);
        await group.getByLabel("묶음 이름", { exact: true }).fill(name);
        await group.getByLabel("영어 이름(선택)", { exact: true }).fill(en);
        for (const [at, [label, english, extra]] of values.entries()) {
          if (at > 1) await group.getByRole("button", { name: /^값 추가/ }).click();
          const value = group.getByRole("group", { name: "값 " + (at + 1), exact: true });
          await value.getByLabel("값 이름", { exact: true }).fill(label);
          await value.getByLabel("영어(선택)", { exact: true }).fill(english);
          if (extra) await value.getByLabel(/^더하는 금액/).fill(extra);
        }
      }
      if (!item.groups.length) await button("묶음 1 지우기", editor).click();
      if (photo[item.sku]) {
        const field = editor.getByRole("group", { name: "메뉴 사진", exact: true });
        await field.getByLabel(/메뉴에 쓸 권리가 매장에 있습니다/).check();
        await field.locator('input[type="file"]').setInputFiles({ name: item.art + ".png", mimeType: "image/png", buffer: photo[item.sku] });
        await field.getByRole("img").waitFor();
      }
      await button("이 상품 저장", editor).click();
      await page.getByRole("heading", { name: /^메뉴 \d+개/ }).waitFor();
    }
    if (shots) await page.screenshot({ path: path.join(shots, "ops-menu.png"), fullPage: true });

    // Categories, and a deadline 40 minutes from now in the store's time.
    for (const [key, ko, en] of [coffee, dessert]) {
      await button("카테고리 추가").click();
      await page.getByLabel("카테고리 코드", { exact: true }).last().fill(key);
      await page.getByLabel("카테고리 한국어 이름", { exact: true }).last().fill(ko);
      await page.getByLabel("카테고리 영어 이름", { exact: true }).last().fill(en);
    }
    for (const item of menu) await page.getByLabel(`${item.sku} 상품 카테고리`, { exact: true }).selectOption(item.category[0]);
    const local = new Intl.DateTimeFormat("sv-SE", { timeZone: "Asia/Seoul", year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" })
      .format(new Date(Date.now() + 40 * 60_000)).replace(" ", "T");
    await page.getByLabel(/^게시 만료/).fill(local);
    await button("게시 내용 검토").click();
    await page.getByRole("list", { name: "사진에서 바뀌는 것" }).waitFor().catch(async (error) => {
      throw new Error(`no review: ${await page.locator('p[role="status"]').first().innerText()} (${error.message.split("\n")[0]})`);
    });
    if (shots) await page.screenshot({ path: path.join(shots, "ops-review.png"), fullPage: true });
    await button("변경 적용").click();
    await page.getByRole("status").filter({ hasText: "새 메뉴 버전의 게시를 확인했습니다." }).waitFor();

    // Fifty of each to sell.
    const stock = page.locator("section").filter({ has: page.getByRole("heading", { name: "재고 조정" }) });
    for (const item of menu) {
      await stock.locator("select").first().selectOption(item.sku);
      await stock.getByLabel("증감 수량").fill("50");
      await button("재고 조정 검토").click();
      await button("변경 적용").click();
      await page.getByRole("status").filter({ hasText: "재고 반영을 확인했습니다." }).waitFor();
    }
    console.log(`cafe menu: ${menu.length} products, ${Object.keys(photo).length} photos, published and stocked`);
    // NU54_CAFE_BUSY=<minutes> marks the store busy, the way an operator would, to see the band on the kiosk.
    if (process.env.NU54_CAFE_BUSY) {
      await page.getByRole("radio", { name: /바쁨/ }).check();
      const minutes = Number(process.env.NU54_CAFE_BUSY);
      for (let at = 15; at < minutes; at += 5) await button("5분 늘리기").click();
      await button(`바쁨 · 약 ${minutes}분(으)로 바꾸기`).click();
      await page.getByRole("alertdialog", { name: "바꾸기 전 확인" }).getByRole("button", { name: "바꾸기", exact: true }).click();
      await page.getByText(new RegExp(`지금 상태: 바쁨 · 약 ${minutes}분`)).waitFor();
      console.log(`cafe store: busy, about ${minutes} minutes`);
    }
  } finally {
    await browser.close();
  }
}
main().catch((error) => { console.error(error); process.exit(1); });

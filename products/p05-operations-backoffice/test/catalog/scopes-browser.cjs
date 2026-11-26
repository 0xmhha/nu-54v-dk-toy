#!/usr/bin/env node
"use strict";

const assert = require("node:assert/strict");
const { chromium } = require(process.env.NU54_PLAYWRIGHT_MODULE || "playwright");
const [origin, branchA, branchB, certPath, keyPath, scenario] = process.argv.slice(2);
const legacyKey = "nu54-operator-catalog-pending-v1";

async function exercise() {
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  try {
    const page = await browser.newPage({
      ignoreHTTPSErrors: true, clientCertificates: [{ origin, certPath, keyPath }],
    });
    page.setDefaultTimeout(8000);
    const button = (name) => page.getByRole("button", { name, exact: true });
    const heading = (name) => page.getByRole("heading", { name, exact: true });
    const published = () => page.getByRole("status").filter({ hasText: "새 메뉴 버전의 게시를 확인했습니다." }).waitFor();
    const publish = (request) => request.url().endsWith("/publish");

    async function open(branch) {
      await page.goto(origin + "/ops/");
      await button("카탈로그").click();
      await page.getByLabel("담당 지점").selectOption(branch);
      const loaded = page.waitForResponse((response) => response.url().endsWith("/" + branch));
      await button("운영 권한 및 메뉴 조회").click();
      await heading("현재 메뉴").waitFor();
      return (await loaded).json();
    }
    // A product's rows are edited on its own page, opened from the menu list.
    const rows = (sku) => page.locator(`button[data-sku="${sku}"]`).click();
    const saveRows = () => button("이 상품 저장").click();
    async function review(price) {
      await page.locator("button[data-sku]").first().click();
      await page.getByLabel("기본 가격", { exact: true }).first().fill(price);
      await saveRows();
      await button("게시 내용 검토").click();
      await heading("메뉴 게시 확인").waitFor();
    }

    if (scenario === "k07") {
      const original = await open(branchA);
      await rows("drink-12");
      const koreanGroup = page.getByLabel("필수 옵션 그룹 한국어 이름", { exact: true });
      const englishGroup = page.getByLabel("필수 옵션 그룹 영어 이름", { exact: true });
      assert.equal(await koreanGroup.first().inputValue(), "");
      await koreanGroup.first().fill("온기");
      await koreanGroup.first().fill("온도");
      await englishGroup.first().fill("Temperature");
      assert.equal(await koreanGroup.nth(1).inputValue(), "온도");
      assert.equal(await englishGroup.nth(1).inputValue(), "Temperature");
      await saveRows();
      await rows("baked-09");
      const candidates = page.getByRole("group", {name: "언어별 메뉴와 점주 알레르기 정보", exact:true});
      const fields = await candidates.count();
      let index = -1;
      for (let i = 0; i < fields; i++) {
        if (await candidates.nth(i).getByLabel("상품 한국어 이름", {exact:true}).inputValue() === "크루아상")
          index = i;
      }
      assert.ok(index >= 0);
      const declaration = candidates.nth(index);
      await declaration.getByLabel("교차접촉 정보", {exact:true}).selectOption("possible");
      await declaration.getByLabel("교차접촉 설비", {exact:true}).selectOption("shared-baking-equipment");
      await declaration.getByLabel("교차접촉 견과류", {exact:true}).check();
      await saveRows();
      await button("게시 내용 검토").click();
      const reviewPanel = page.getByRole("region", {name:"변경 확인"});
      await reviewPanel.getByText(/온도.*Temperature/).first().waitFor();
      await reviewPanel.getByText(/공용 제빵 설비.*교차접촉 신고 성분 견과류/).first().waitFor();
      const response = page.waitForResponse((r) => publish(r.request()));
      await button("변경 적용").click();
      const result = await response;
      assert.equal(result.status(), 200);
      await published();
      const next = await open(branchA);
      assert.equal(next.catalog.version, original.catalog.version + 1);
      console.log("O03 edited Korean/English required labels and owner-declared tree nuts/shared equipment, reviewed, published, and reloaded one new version.");
      return;
    }

    const a = await open(branchA);
    async function addProduct(sku, label) {
      // A new product without groups is saved as one row, in the branch's asset.
      await button("상품 추가").click();
      const editor = page.getByRole("form", {name: "메뉴 고치기"});
      await editor.getByLabel("상품 코드", {exact: true}).fill(sku);
      await editor.getByLabel("상품 이름", {exact: true}).fill(label);
      await editor.getByLabel(/^가격\(/).fill("3.10");
      await editor.getByRole("button", {name: "묶음 1 지우기", exact: true}).click();
      await editor.getByRole("button", {name: "이 상품 저장", exact: true}).click();
    }
    await addProduct("item-22", "Croissant");
    await addProduct("item-23", "House special");
    for (const [key, ko, en] of [["warm", "음료", "Drinks"], ["oven", "베이커리", "Bakery"]]) {
      await button("카테고리 추가").click();
      await page.getByLabel("카테고리 코드", {exact:true}).last().fill(key);
      await page.getByLabel("카테고리 한국어 이름", {exact:true}).last().fill(ko);
      await page.getByLabel("카테고리 영어 이름", {exact:true}).last().fill(en);
    }
    await page.getByLabel("tea 상품 카테고리", {exact:true}).selectOption("warm");
    await page.getByLabel("item-22 상품 카테고리", {exact:true}).selectOption("oven");
    await review("9.25");
    const categories = page.getByRole("list", {name:"게시 카테고리"});
    await categories.getByText(/음료 · Drinks · warm.*tea/).waitFor();
    await categories.getByText(/베이커리 · Bakery · oven.*item-22/).waitFor();
    await categories.getByText(/미분류 · item-23/).waitFor();
    let original;
    await page.route("**/publish", async (route) => {
      original = route.request().postDataJSON();
      const reply = await route.fetch();
      assert.equal(reply.status(), 200);
      await reply.dispose();
      await route.abort("connectionreset");
    }, { times: 1 });
    const dropped = page.waitForEvent("requestfailed", publish);
    await button("변경 적용").click();
    await dropped;
    await heading("응답 미확정").waitFor();
    assert.ok(original);

    // Recover a record written by the previous software version.
    await page.evaluate(({ scope, branch, body, legacy }) => {
      for (const name of Object.keys(localStorage)) {
        if (name.includes(scope)) localStorage.removeItem(name);
      }
      localStorage.setItem(legacy, JSON.stringify({ branch, scope, operation: "publish", body }));
    }, { scope: a.receiptScope, branch: branchA, body: original, legacy: legacyKey });

    const b = await open(branchB);
    assert.deepEqual(b.catalog.categories ?? [], []);
    assert.equal(await page.getByLabel("카테고리 코드", {exact: true}).count(), 0);
    assert.notEqual(a.receiptScope, b.receiptScope);
    assert.equal(await heading("응답 미확정").count(), 0);
    await review("10.25");
    const issued = page.waitForResponse((response) => publish(response.request()));
    await button("변경 적용").click();
    const done = await issued;
    assert.equal(done.status(), 200);
    const branchBRequest = done.request().postDataJSON();
    assert.notEqual(branchBRequest.requestKey, original.requestKey);
    assert.equal(Object.hasOwn(branchBRequest, "categories"), false);
    await published();
    assert.ok(await page.evaluate((legacy) => localStorage.getItem(legacy), legacyKey));

    await open(branchA);
    await heading("응답 미확정").waitFor();
    const recovery = page.waitForRequest(publish);
    await button("같은 요청 재조회").click();
    assert.deepEqual((await recovery).postDataJSON(), original);
    await published();
    assert.equal(await page.evaluate((legacy) => localStorage.getItem(legacy), legacyKey), null);
    const activeA = (await open(branchA)).catalog;
    assert.equal(activeA.version, 3);
    assert.deepEqual(activeA.categories, original.categories);
    assert.deepEqual(activeA.categoryBindings, [
      { sku: "item-22", categoryKey: "oven" }, { sku: "tea", categoryKey: "warm" },
    ]);
    assert.equal(activeA.rows.find((row) => row.sku === "item-23").price, "3100000");
    assert.equal((await open(branchB)).catalog.version, 2);
    console.log("O03 authored two category definitions, bound grouped tea and a second product, and left one SKU unclassified at its published price. A recovered its identical request after response loss; B published independently without category fields.");
  } finally {
    await browser.close();
  }
}
exercise().catch((error) => {
  console.error(String(error.message).replace(/0x[0-9a-f]+|[a-f0-9-]{36}/gi, "[fixture]"));
  process.exitCode = 1;
});

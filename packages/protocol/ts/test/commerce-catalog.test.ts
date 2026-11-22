import assert from "node:assert/strict";
import test from "node:test";
import {
  appendCommerceCatalogPage,
  commerceMenuDisclosure,
  bindServiceRequestV2,
  CommerceClient,
  type CommercePrice,
  type ServiceRequestV2,
} from "../src/index.ts";

const branch = "0fbc54ef-2827-4ab5-bfcd-3a27611c3001";
const actor = "0fbc54ef-2827-4ab5-bfcd-3a27611c3002";
const tenant = "0fbc54ef-2827-4ab5-bfcd-3a27611c3003";
const requestId = "0fbc54ef-2827-4ab5-bfcd-3a27611c3004";
const now = 1_750_000_000;
const profile = {
  subjectRef: actor,
  role: "customer",
  branchRef: branch,
  tenantRef: tenant,
  expiresAt: now + 120,
} as const;

function items(offset: number, size: number): CommercePrice[] {
  return Array.from({ length: size }, (_, index) => ({
    version: 1,
    expiresAt: new Date((now + 60) * 1000).toISOString(),
    sku: `sku-${(index + offset).toString().padStart(3, "0")}`,
    optionKey: "plain",
    label: "Item",
    unitPrice: "7000000",
    asset: "0x" + "5".repeat(40),
    available: 2,
  }));
}

function fixture(read: (page: number) => CommercePrice[], version?: (page: number) => number | undefined) {
  const pages: number[] = [];
  const client = new CommerceClient(
    profile,
    async (raw) => {
      const { command } = JSON.parse(raw) as { command: ServiceRequestV2 };
      const page = Number(command.body.page);
      pages.push(page);
      const binding = bindServiceRequestV2(
        command,
        {
          ...profile,
          grants: [
            {
              domainRef: branch,
              objectRef: branch,
              operations: ["C3/catalog.read"],
            },
          ],
        },
        now
      );
      return JSON.stringify({
        ...binding,
        status: "ok",
        resultRef: branch,
        catalog: read(page),
        catalogVersion: version?.(page),
      });
    },
    async () => requestId,
    () => now
  );
  return { client, pages };
}

for (const size of [0, 49, 50, 150]) {
  test(`published catalog reads exactly ${size} rows`, async () => {
    const wanted = items(0, size);
    const { client, pages } = fixture((page) =>
      wanted.slice(page * 50, (page + 1) * 50)
    );
    assert.deepEqual(await client.publishedCatalog(), wanted);
    assert.deepEqual(
      pages,
      Array.from({ length: Math.floor(size / 50) + 1 }, (_, page) => page)
    );
  });
}

test("published catalog rejects repetition and excess rows within a bounded number of requests", async () => {
  const repeated = fixture(() => items(0, 50));
  await assert.rejects(repeated.client.publishedCatalog(), /BAD_RESPONSE/);
  assert.deepEqual(repeated.pages, [0, 1]);
  const excess = fixture((page) => items(page * 50, 50));
  await assert.rejects(excess.client.publishedCatalog(), /BAD_RESPONSE/);
  assert.deepEqual(excess.pages, [0, 1, 2, 3]);
});

test("catalog merge retains distinct options and rejects duplicate or mixed-version nearby rows", () => {
  const first = items(0, 1)[0];
  const catalog = [first];
  appendCommerceCatalogPage(catalog, [{ ...first, optionKey: "iced" }]);
  assert.equal(catalog.length, 2);
  assert.throws(
    () => appendCommerceCatalogPage(catalog, [first]),
    /BAD_RESPONSE/
  );
  assert.throws(
    () => appendCommerceCatalogPage(catalog, [{ ...first, version: 2 }]),
    /CATALOG_CHANGED/
  );
  assert.equal(catalog.length, 2);
});

test("additive published declarations reach kiosk and consumer catalog readers without inferring missing data", async () => {
  const legacy = items(0, 1)[0];
  const declared: CommercePrice = {
    ...legacy,
    metadata: {
      optionRequired: true,
      optionGroupLabels: {ko: "온도", en: "Temperature"},
      allergens: ["wheat"],
      crossContact: "possible",
      crossContactAllergens: ["tree-nuts"],
      crossContactSource: "shared-baking-equipment",
    },
  };
  assert.deepEqual(await fixture(() => [declared]).client.publishedCatalog(), [declared]);
  assert.equal(commerceMenuDisclosure(declared, "ko"), "점주 신고 성분 밀 · 공용 제빵 설비에서 교차접촉 가능 · 견과류");
  assert.equal(commerceMenuDisclosure(declared, "en"), "Merchant-declared ingredients wheat · Cross-contact possible on shared baking equipment · tree nuts");
  assert.equal(commerceMenuDisclosure(legacy, "en"), "Allergen ingredients not supplied · Cross-contact information not supplied");
  for (const metadata of [
    {...declared.metadata, optionRequired: false},
    {...declared.metadata, crossContact: "unknown"},
    {...declared.metadata, crossContactAllergens: ["tree-nuts", "tree-nuts"]},
    {...declared.metadata, crossContactSource: "unrecognized"},
  ]) {
    const malformed = {...declared, metadata} as CommercePrice;
    await assert.rejects(fixture(() => [malformed]).client.publishedCatalog(), /BAD_RESPONSE/);
  }
});

test("published category projections reject malformed and conflicting versions", async () => {
  const projected = (offset: number, count: number) => items(offset, count).map((row) => ({
    ...row, category: {key: "coffee", sortOrder: 2, labels: {ko: "커피", en: "Coffee"}},
  }));
  const first = projected(0, 50);
  const last = projected(50, 2);
  const valid = fixture((page) => page === 0 ? first : last);
  assert.deepEqual(await valid.client.publishedCatalog(), [...first, ...last]);
  const conflicting = {...last[0], category: {...last[0].category, labels: {ko: "다름", en: "Different"}}};
  await assert.rejects(fixture((page) => page === 0 ? first : [conflicting]).client.publishedCatalog(), /BAD_RESPONSE/);
  const invalid = {...first[0], category: {...first[0].category, sortOrder: -1}};
  await assert.rejects(fixture(() => [invalid]).client.publishedCatalog(), /BAD_RESPONSE/);
});


test("version changes are rejected even on an empty terminating page", async () => {
  const pages = (page: number) => page === 0 ? items(0, 50) : [];
  await assert.rejects(fixture(pages, (page) => page === 0 ? 1 : 2).client.publishedCatalog(), /CATALOG_CHANGED/);
  await assert.rejects(fixture(pages, (page) => page === 0 ? 1 : undefined).client.publishedCatalog(), /CATALOG_CHANGED/);
  assert.equal((await fixture(pages, () => 1).client.publishedCatalog()).length, 50);
});

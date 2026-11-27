import assert from "node:assert/strict";
import test from "node:test";
import {
  assertCommerceCatalogPage,
  assertCommerceChoices,
  bindServiceRequestV2,
  commerceChoiceGroups,
  CommerceClient,
  commerceOptionLabel,
  decodeNearbyRequest,
  utf8Encode,
  type CommercePrice,
  type CommerceSkuOptions,
  type ServiceRequestV2,
} from "../src/index.ts";

const branch = "0fbc54ef-2827-4ab5-bfcd-3a27611c3001";
const now = 1_750_000_000;
const profile = {
  subjectRef: "0fbc54ef-2827-4ab5-bfcd-3a27611c3002", role: "customer", branchRef: branch,
  tenantRef: "0fbc54ef-2827-4ab5-bfcd-3a27611c3003", expiresAt: now + 120,
} as const;

const latte = (): CommerceSkuOptions => ({
  sku: "latte",
  groups: [
    { key: "size", labels: { ko: "크기", en: "Size" }, values: [
      { key: "small", labels: { ko: "작은 컵", en: "Small" } }, { key: "large", labels: { ko: "큰 컵", en: "Large" } }] },
    { key: "temp", labels: { ko: "온도" }, values: [
      { key: "hot", labels: { ko: "뜨거움", en: "Hot" } }, { key: "ice", labels: { ko: "차가움" } }] },
  ],
});
const row = (sku: string, optionKey: string, required = true): CommercePrice => ({
  version: 1, expiresAt: new Date((now + 60) * 1000).toISOString(), sku, optionKey, label: "Item",
  unitPrice: "450000", asset: "0x" + "5".repeat(40), available: 2, ...(required ? { metadata: { optionRequired: true } } : {}),
});
const lattes = () => ["large-hot", "large-ice", "small-hot", "small-ice"].map((key) => row("latte", key));

function client(read: (page: number) => { catalog: CommercePrice[]; catalogOptions?: unknown }) {
  return new CommerceClient(profile, async (raw) => {
    const { command } = JSON.parse(raw) as { command: ServiceRequestV2 };
    const binding = bindServiceRequestV2(command,
      { ...profile, grants: [{ domainRef: branch, objectRef: branch, operations: ["C3/catalog.read"] }] }, now);
    return JSON.stringify({ ...binding, status: "ok", resultRef: branch, ...read(Number(command.body.page)) });
  }, async () => "0fbc54ef-2827-4ab5-bfcd-3a27611c3004", () => now);
}

test("a product's choices are read in the store's order and the other rows stay as they were", async () => {
  const cookie = row("cookie", "standard", false);
  const catalog = await client(() => ({ catalog: [cookie, ...lattes()], catalogOptions: [latte()] })).publishedCatalog();
  assert.deepEqual(catalog[0], cookie);
  const groups = commerceChoiceGroups(catalog.slice(1));
  assert.deepEqual(groups, latte().groups);
  assert.equal(commerceOptionLabel(groups[1].labels, "en"), "온도");
  assert.equal(commerceOptionLabel(groups[0].labels, "en"), "Size");
  assert.deepEqual(commerceChoiceGroups([cookie]), []);
  assert.deepEqual(catalog[1].choices!.map(({ value }) => value.key), ["large", "hot"]);
});

test("a reply without choices reads as before", async () => {
  assert.deepEqual(await client(() => ({ catalog: lattes() })).publishedCatalog(), lattes());
});

test("choices that do not describe the rows are refused", async () => {
  for (const [name, change] of Object.entries<(catalog: CommercePrice[], options: CommerceSkuOptions[]) => unknown>({
    "a missing combination": (catalog) => { catalog.pop(); },
    "another combination": (catalog) => { catalog[0].optionKey = "medium-hot"; },
    "one group too few": (_, options) => { options[0].groups.pop(); },
    "a third part": (catalog) => { catalog[0].optionKey = "large-hot-x"; },
    "an optional row": (catalog) => { delete catalog[0].metadata; },
    "a product that is not listed": (_, options) => { options[0].sku = "mocha"; },
    "the same product twice": (_, options) => { options.push(latte()); },
    "no Korean text": (_, options) => { options[0].groups[0].labels = { en: "Size" } as never; },
    "long text": (_, options) => { options[0].groups[0].values[0].labels.ko = "가".repeat(25); },
    "bidi text": (_, options) => { options[0].groups[0].values[0].labels.ko = "컵‮"; },
    "one value": (_, options) => { options[0].groups[0].values.pop(); },
    "the same value twice": (_, options) => { options[0].groups[0].values[1].key = "small"; },
    "the same group twice": (_, options) => { options[0].groups[1].key = "size"; },
    "no list": (_, options) => { (options[0] as { groups: unknown }).groups = {}; },
  })) {
    const catalog = lattes(), options = [latte()];
    change(catalog, options);
    await assert.rejects(client(() => ({ catalog, catalogOptions: options })).publishedCatalog(), /BAD_RESPONSE/, name);
  }
});

test("a product split over two pages keeps one definition", async () => {
  const many: CommerceSkuOptions = { sku: "many", groups: [10, 6].map((count, g) => ({
    key: `g${g}`, labels: { ko: `묶음${g}` }, values: Array.from({ length: count }, (_, v) => ({ key: `v${v}`, labels: { ko: `값${v}` } })) })) };
  const rows = many.groups[0].values.flatMap((a) => many.groups[1].values.map((b) => row("many", `${a.key}-${b.key}`)));
  const read = (second: CommerceSkuOptions) => client((page) => ({ catalog: rows.slice(page * 50, page * 50 + 50), catalogOptions: [page ? second : many] }));
  assert.equal((await read(many).publishedCatalog()).length, 60);
  const other = structuredClone(many);
  other.groups[0].labels.ko = "다른 이름";
  await assert.rejects(read(other).publishedCatalog(), /BAD_RESPONSE/);
  const reordered = structuredClone(many);
  reordered.groups[1].values.reverse();
  await assert.rejects(read(reordered).publishedCatalog(), /BAD_RESPONSE/);
});

test("600 rows are read and the 601st is refused", async () => {
  const rows = (count: number) => Array.from({ length: count }, (_, index) => row(`sku-${String(index).padStart(3, "0")}`, "plain", false));
  for (const count of [599, 600])
    assert.equal((await client((page) => ({ catalog: rows(count).slice(page * 50, page * 50 + 50) })).publishedCatalog()).length, count);
  await assert.rejects(client((page) => ({ catalog: rows(601).slice(page * 50, page * 50 + 50) })).publishedCatalog(), /BAD_RESPONSE/);
});

test("rows relayed next to the kiosk carry their choices and are checked", async () => {
  const { prices } = await client(() => ({ catalog: lattes(), catalogOptions: [latte()] })).catalogWindow(0);
  assertCommerceCatalogPage(prices, now, 4);
  assertCommerceChoices(prices);
  const broken = structuredClone(prices);
  broken[0].choices![0].value.order = 1;
  broken[0].choices![0].value.labels = { ko: "다른 컵" };
  assertCommerceCatalogPage(broken, now, 4);
  assert.throws(() => assertCommerceChoices(broken), /BAD_RESPONSE/);
  assert.throws(() => assertCommerceChoices([prices[0], { ...prices[1], choices: undefined }]), /BAD_RESPONSE/);
  const oneSize = prices.filter(({ optionKey }) => optionKey.startsWith("large-"));
  assert.throws(() => assertCommerceChoices(oneSize), /BAD_RESPONSE/);
  const sameOrder = structuredClone(prices);
  for (const price of sameOrder) price.choices![0].value.order = 0;
  assert.throws(() => assertCommerceChoices(sameOrder), /BAD_RESPONSE/);
  const bad = structuredClone(prices);
  bad[0].choices!.pop();
  assert.throws(() => assertCommerceCatalogPage(bad, now, 4), /BAD_RESPONSE/);
  for (const offset of [0, 600])
    decodeNearbyRequest(utf8Encode(JSON.stringify({ op: "catalog", branch, offset })), branch);
  assert.throws(() => decodeNearbyRequest(utf8Encode(JSON.stringify({ op: "catalog", branch, offset: 601 })), branch), /NEARBY_COMMAND/);
});

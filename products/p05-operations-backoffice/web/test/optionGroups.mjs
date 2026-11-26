import assert from "node:assert/strict";
import { combinations, emptyProduct, lineCount, menuLines, newKey, priceRange, productProblems, publishedProducts, savedProducts } from "../src/optionGroups.ts";

const won = { symbol: "KRW", decimals: 2, token: { name: "NU54 Test KRW", symbol: "tKRW", label: "KRW", decimals: 2 } };
const coin = { symbol: "KRWC", decimals: 18, token: { name: "WKRC", symbol: "WKRC", label: "KRWC", decimals: 18 } };
const usd = { symbol: "TestUSDC", decimals: 6 };
const value = (key, ko, price = "0", allergens = []) => ({ key, ko, en: "", price, allergens });
const latte = () => ({
  sku: "latte", label: "카페라떼", price: "5000", tax: "0", discount: "", metadata: { allergens: ["soy"] },
  groups: [
    { key: "temp", ko: "온도", en: "Temperature", values: [value("hot", "뜨겁게"), value("ice", "차갑게")] },
    { key: "size", ko: "크기", en: "", values: [value("s", "보통"), value("l", "크게", "500")] },
    { key: "milk", ko: "우유", en: "", values: [value("cow", "우유", "0", ["milk", "soy"]), value("oat", "귀리 음료", "700")] },
  ],
});

// The table before publishing: every combination once, in the store's order, each sum from its own parts.
const rows = combinations(latte(), won);
assert.equal(rows.length, lineCount(latte()));
assert.equal(new Set(rows.map(({ option }) => option)).size, 8);
assert.deepEqual(rows.map(({ option, total }) => [option, total]), [
  ["hot-s-cow", "5,000원"], ["hot-s-oat", "5,700원"], ["hot-l-cow", "5,500원"], ["hot-l-oat", "6,200원"],
  ["ice-s-cow", "5,000원"], ["ice-s-oat", "5,700원"], ["ice-l-cow", "5,500원"], ["ice-l-oat", "6,200원"],
]);
assert.deepEqual(rows[3], { option: "hot-l-oat", label: "뜨겁게 · 크게 · 귀리 음료", base: "5,000원", added: "+1,200원", total: "6,200원", tax: "", allergens: ["soy"] });
// A price that includes 10% shows the tax inside each combination, cut to the whole won.
const taxed = combinations({ ...latte(), metadata: { allergens: ["soy"], tax: "vat10" } }, won);
assert.deepEqual(taxed.map(({ total, tax }) => [total, tax]).slice(0, 4), [["5,000원", "454원"], ["5,700원", "518원"], ["5,500원", "500원"], ["6,200원", "563원"]]);
assert.equal(combinations({ ...latte(), metadata: { tax: "exempt" } }, won)[0].tax, "0원");
assert.deepEqual(rows[0].allergens, ["soy", "milk"]);
assert.equal(rows[0].added, "0원");
assert.equal(priceRange(latte(), won), "5,000원부터 6,200원까지");
assert.equal(priceRange({ ...latte(), discount: "1000" }, won), "4,000원부터 5,200원까지");
assert.equal(priceRange(latte(), coin), "5,000원부터 6,200원까지");
assert.equal(combinations({ ...latte(), price: "5.00", groups: [{ ...latte().groups[1], values: [value("s", "S"), value("l", "L", "0.50")] }] }, usd)[1].total, "5.50 TestUSDC");
assert.deepEqual(combinations({ ...latte(), price: "abc" }, won), []);

// What is sent, and what is read back for the next edit, is the same.
const sent = publishedProducts([latte()], "0xasset");
assert.deepEqual(sent.rows, [{ sku: "latte", option: "", label: "카페라떼", asset: "0xasset", displayPrice: "5000", tax: "0", discount: "", required: true, metadata: { allergens: ["soy"] } }]);
assert.deepEqual(sent.options[0].groups[0].labels, { ko: "온도", en: "Temperature" });
assert.deepEqual(sent.options[0].groups[2].values, [
  { key: "cow", labels: { ko: "우유" }, price: "0", allergens: ["milk", "soy"] }, { key: "oat", labels: { ko: "귀리 음료" }, price: "700" }]);
assert.equal(publishedProducts([{ ...latte(), groups: [{ ...latte().groups[0], values: [value("a", "A", ""), value("b", "B", "00")] }] }], "").options[0].groups[0].values[1].price, "0");
const units = (amount, decimals) => (amount === "0" || amount === "" ? "0" : amount + "0".repeat(decimals));
for (const asset of [won, coin]) {
  const saved = [{ sku: "latte", base: { label: "카페라떼", displayPrice: units("5000", asset.decimals), tax: "0", discount: "0", metadata: { allergens: ["soy"] } },
    groups: sent.options[0].groups.map((group) => ({ ...group, values: group.values.map((v) => ({ ...v, price: units(v.price, asset.decimals) })) })) }];
  assert.deepEqual(savedProducts(saved, asset), [{ ...latte(), discount: "0" }]);
}
assert.equal(savedProducts([{ sku: "a", base: { label: "a", displayPrice: "5500000", tax: "0", discount: "0" }, groups: [] }], usd)[0].price, "5.50");
assert.deepEqual(savedProducts(undefined, won), []);

// Refused, beside the group it is about. The server refuses the same.
const problem = (change, asset = won) => { const product = latte(); change(product); return productProblems(product, asset); };
assert.deepEqual(problem(() => {}), { product: undefined, groups: [undefined, undefined, undefined], incomplete: false });
// A choice may add to a price that includes its tax; only an amount added on top of the price refuses it.
assert.deepEqual(problem((p) => { p.metadata.tax = "vat10"; }).groups, [undefined, undefined, undefined]);
assert.equal(emptyProduct().metadata.tax, "vat10");
const tax = problem((p) => { p.tax = "500"; });
assert.equal(tax.groups[0], undefined);
assert.match(tax.groups[1], /세금이 있는 상품/);
assert.match(tax.groups[2], /세금이 있는 상품/);
assert.match(problem((p) => { delete p.metadata.allergens; }).groups[2], /알레르기 성분을 먼저/);
for (const price of ["0.5", "-500", "1,000", "천"]) assert.match(problem((p) => { p.groups[1].values[1].price = price; }).groups[1], /원 단위/);
assert.match(problem((p) => { p.price = "5.00"; p.groups = [p.groups[1]]; p.groups[0].values[1].price = "0.505"; }, usd).groups[0], /둘째 자리/);
assert.equal(problem((p) => { p.groups[1].values[1].price = ""; }).groups[1], undefined);
for (const ko of ["가".repeat(25), " 크게", "크‮게"]) assert.match(problem((p) => { p.groups[1].values[1].ko = ko; }).groups[1], /24자/);
assert.match(problem((p) => { p.groups[1].values[1].ko = "보통"; }).groups[1], /같은 이름/);
assert.match(problem((p) => { p.groups[1].ko = "온도"; }).product, /같은 이름/);
assert.match(problem((p) => { p.groups.forEach((g) => { g.values[0].key = "k".repeat(12); }); }).product, /길이/);
assert.match(problem((p) => { p.price = "0"; }).product, /0보다/);
for (const change of [(p) => { p.groups[0].ko = ""; }, (p) => { p.groups[0].values[0].ko = ""; }, (p) => { p.groups[0].values.pop(); }, (p) => { p.sku = ""; }, (p) => { p.price = ""; }])
  assert.equal(problem(change).incomplete, true);

// Lines of the whole menu, and codes that are never reused.
const wide = { ...latte(), groups: [12, 12, 5].map((count, g) => ({ key: "g" + g, ko: "묶음" + g, en: "", values: Array.from({ length: count }, (_, v) => value("v" + v, "값" + v)) })) };
assert.equal(menuLines([latte(), wide], 3), 8 + 720 + 3);
assert.deepEqual(combinations(wide, won), []);
const keys = Array.from({ length: 2000 }, newKey);
assert.equal(new Set(keys).size, 2000);
for (const key of keys) assert.match(key, /^[a-z0-9]{6}$/);
const fresh = emptyProduct();
assert.equal(new Set([fresh.groups[0].key, ...fresh.groups[0].values.map(({ key }) => key)]).size, 3);
// Four groups of made-up codes always fit the 32 characters of a combination.
assert.ok([1, 2, 3, 4].map(newKey).join("-").length <= 32);

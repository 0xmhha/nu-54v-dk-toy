import type { CommerceMenuMetadata } from "@nu54/protocol";
import { assetToken, humanAmount, priceProblem, rowTotal, type CatalogAsset, type PriceRow } from "./prices.ts";
import { isWon, tokenMoney } from "./money.ts";
import { includedTax } from "./taxes.ts";

// The server checks the same limits; these explain them before it refuses.
export const rowLimit = 600;
export const groupLimit = 4;
export const valueLimit = 12;
const labelLimit = 24;
const keyLimit = 32;

type Allergens = NonNullable<CommerceMenuMetadata["allergens"]>;
export type OptionValue = { key: string; ko: string; en: string; price: string; allergens: Allergens };
export type OptionGroup = { key: string; ko: string; en: string; values: OptionValue[] };
/** A product whose choices are grouped. Every combination is published as its own row. */
export type Product = PriceRow & { sku: string; label: string; metadata?: CommerceMenuMetadata; groups: OptionGroup[] };

type Labels = { ko: string; en?: string };
export type SavedOptions = {
  sku: string;
  base: { label: string; displayPrice: string; tax: string; discount: string; metadata?: CommerceMenuMetadata };
  groups: { key: string; labels: Labels; values: { key: string; labels: Labels; price?: string; allergens?: Allergens }[] }[];
};

/** A code for a group or a value. Stock limits are kept by it, so a removed one is never given to another value. */
export function newKey(): string {
  const letters = "abcdefghijklmnopqrstuvwxyz0123456789";
  return Array.from(crypto.getRandomValues(new Uint32Array(6)), (n) => letters[n % letters.length]).join("");
}

export const emptyValue = (): OptionValue => ({ key: newKey(), ko: "", en: "", price: "0", allergens: [] });
export const emptyGroup = (): OptionGroup => ({ key: newKey(), ko: "", en: "", values: [emptyValue(), emptyValue()] });
export const emptyProduct = (): Product => ({ sku: "", label: "", price: "", tax: "0", discount: "0", metadata: { tax: "vat10" }, groups: [emptyGroup()] });

export function savedProducts(options: SavedOptions[] | undefined, asset: CatalogAsset): Product[] {
  const amount = (units: string | undefined) => humanAmount(units || "0", asset.decimals).replace(/\.0+$/, "");
  return (options ?? []).map(({ sku, base, groups }) => ({
    sku, label: base.label, price: amount(base.displayPrice), tax: amount(base.tax), discount: amount(base.discount), metadata: base.metadata,
    groups: groups.map(({ key, labels, values }) => ({
      key, ko: labels.ko, en: labels.en ?? "",
      values: values.map((value) => ({ key: value.key, ko: value.labels.ko, en: value.labels.en ?? "", price: amount(value.price), allergens: value.allergens ?? [] })),
    })),
  }));
}

const labels = ({ ko, en }: { ko: string; en: string }): Labels => (en ? { ko, en } : { ko });
export function publishedProducts(products: Product[], asset: string) {
  return {
    rows: products.map(({ sku, label, price, tax, discount, metadata }) => ({ sku, option: "", label, asset, displayPrice: price, tax, discount, required: true, metadata })),
    options: products.map(({ sku, groups }) => ({
      sku,
      groups: groups.map((group) => ({
        key: group.key, labels: labels(group),
        values: group.values.map((value) => ({ key: value.key, labels: labels(value), price: zero(value.price) ? "0" : value.price, ...(value.allergens.length ? { allergens: value.allergens } : {}) })),
      })),
    })),
  };
}

export const lineCount = ({ groups }: Product) => groups.reduce((count, { values }) => count * values.length, 1);

const zero = (amount: string) => /^0*(\.0*)?$/.test(amount);
const added = (value: OptionValue, asset: CatalogAsset) => (zero(value.price) ? 0n : rowTotal({ price: value.price, tax: "", discount: "" }, asset));

export type Combination = { option: string; label: string; base: string; added: string; total: string; tax: string; allergens?: Allergens };
/** The rows the server will make, in the store's order. Empty while an amount is not one. */
export function combinations(product: Product, asset: CatalogAsset): Combination[] {
  const base = rowTotal(product, asset);
  if (base === undefined || lineCount(product) > rowLimit) return [];
  let rows: { keys: string[]; names: string[]; sum: bigint; allergens?: Allergens }[] = [{ keys: [], names: [], sum: 0n, allergens: product.metadata?.allergens }];
  for (const { values } of product.groups) {
    const next: typeof rows = [];
    for (const row of rows)
      for (const value of values) {
        const sum = added(value, asset);
        if (sum === undefined) return [];
        next.push({ keys: [...row.keys, value.key], names: [...row.names, value.ko], sum: row.sum + sum,
          allergens: row.allergens && [...new Set([...row.allergens, ...value.allergens])] });
      }
    rows = next;
  }
  const money = (units: bigint) => tokenMoney(units.toString(), assetToken(asset));
  return rows.map(({ keys, names, sum, allergens }) => ({
    option: keys.join("-"), label: names.join(" · "), base: money(base), added: sum ? `+${money(sum)}` : money(0n), total: money(base + sum),
    tax: product.metadata?.tax ? money(includedTax(base + sum, product.metadata.tax, asset)) : "", allergens,
  }));
}

/** "5,000원부터 6,000원까지" */
export function priceRange(product: Product, asset: CatalogAsset): string {
  const rows = combinations(product, asset);
  const base = rowTotal(product, asset);
  if (!rows.length || base === undefined) return "";
  const sums = product.groups.map(({ values }) => values.map((value) => added(value, asset) ?? 0n));
  const edge = (pick: (a: bigint, b: bigint) => boolean) =>
    sums.reduce((sum, list) => sum + list.reduce((best, n) => (pick(n, best) ? n : best)), base);
  const low = edge((a, b) => a < b), high = edge((a, b) => a > b);
  const money = (units: bigint) => tokenMoney(units.toString(), assetToken(asset));
  return low === high ? money(low) : `${money(low)}부터 ${money(high)}까지`;
}

const labelProblem = (text: string) =>
  text !== text.trim() || [...text].length > labelLimit || /[\p{Cc}‪-‮⁦-⁩]/u.test(text);
const repeated = (names: string[]) => new Set(names).size !== names.length;

export type Problems = { product?: string; groups: (string | undefined)[] };
/** Why this product cannot be published, beside the group it is about. Names still being typed are only `incomplete`. */
export function productProblems(product: Product, asset: CatalogAsset): Problems & { incomplete: boolean } {
  const groups = product.groups.map((group) => {
    const texts = [group.ko, group.en, ...group.values.flatMap(({ ko, en }) => [ko, en])];
    if (texts.some(labelProblem)) return `이름은 앞뒤 공백 없이 ${labelLimit}자까지 적을 수 있습니다.`;
    if (repeated(group.values.map(({ ko }) => ko).filter(Boolean)) || repeated(group.values.map(({ en }) => en).filter(Boolean)))
      return "같은 이름의 값이 있습니다. 손님이 구별할 수 있게 바꿔 주세요.";
    const amounts = group.values.filter(({ price }) => !zero(price));
    if (amounts.length && !zero(product.tax)) return "세금이 있는 상품이라 더하는 금액을 적을 수 없습니다. 더하는 금액을 0으로 바꿔 주세요.";
    if (priceProblem(amounts.map(({ price }) => ({ price, tax: "", discount: "" })), asset))
      return isWon(assetToken(asset)) ? "더하는 금액은 0 이상의 원 단위로 입력하세요." : "더하는 금액은 0 이상으로, 소수점 둘째 자리까지 입력하세요.";
    if (!product.metadata?.allergens && group.values.some(({ allergens }) => allergens.length))
      return "상품의 알레르기 성분을 먼저 적어 주세요. 일부 값에만 적으면 전체 성분으로 잘못 읽힙니다.";
    return undefined;
  });
  const longest = product.groups.reduce((sum, { values }) => sum + 1 + Math.max(0, ...values.map(({ key }) => key.length)), -1);
  const problem = repeated(product.groups.map(({ ko }) => ko).filter(Boolean)) ? "같은 이름의 묶음이 있습니다."
    : longest > keyLimit ? "값의 코드를 합친 길이가 한도를 넘습니다. 값을 지우고 다시 더해 주세요."
    : product.price ? priceProblem([product], asset) : undefined;
  return {
    product: problem, groups,
    incomplete: !product.sku || !product.label.trim() || !product.price || product.groups.some(({ ko, values }) => !ko || values.length < 2 || values.some((value) => !value.ko)),
  };
}

/** The whole menu: the rows of each product plus the rows of products without groups. */
export const menuLines = (products: Product[], rows: number) => products.reduce((sum, product) => sum + lineCount(product), rows);

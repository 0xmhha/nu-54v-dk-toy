import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import test from "node:test";
import { encodeCommerceCart, type CommerceCart, type CommercePrice } from "../src/commerce-v2.ts";
import { reviewCommerceCart, commerceCartTotals, commerceNeedsOptionSupport } from "../src/commerce-cart-review.ts";

const order: CommerceCart = {
  origin: "menu", originRef: "69c9dcda-3020-4cb9-9389-e20391770bd0", menuVersion: 1,
  items: [],
};
const hot = { sku: "tea", option_key: "hot", quantity: 1 };
const iced = { sku: "tea", option_key: "iced", quantity: 2 };

test("distinct options of one SKU are canonical and preserve quantities", () => {
  const canonical = encodeCommerceCart({...order, items: [hot, iced]});
  assert.equal(canonical, encodeCommerceCart({...order, items: [iced, hot]}));
  assert.deepEqual(JSON.parse(canonical).items, [hot, iced]);
  assert.throws(() => encodeCommerceCart({...order, items: [hot, {...hot, quantity: 2}]}), /BAD_REQUEST/);
});

test("an existing single-option request retains its exact digest", () => {
  const canonical = encodeCommerceCart({...order, items: [hot]});
  assert.equal(createHash("sha256").update(canonical).digest("hex"),
    "7e448b1891a4c47b0afdad360d9b719809c8129f30a624d2b786ebe98aa3c086");
});

test("review keeps both option lines and allocates shared SKU stock in pair order", () => {
  const row: CommercePrice = {sku: "tea", optionKey: "hot", label: "Tea", available: 3,
    version: 1, expiresAt: "", unitPrice: "4000000", asset: "test"};
  const cold = {...row, optionKey:"iced", unitPrice:"3500000"};
  const previous = {first:{row:cold,quantity:2}, second:{row,quantity:1}};
  const all = reviewCommerceCart(previous, [cold, row]);
  assert.deepEqual(Object.values(all.selection).map(({row,quantity})=>[row.optionKey,quantity]), [["hot",1],["iced",2]]);
  assert.equal(commerceCartTotals(all.selection)[0].amount, "11000000");
  assert.equal(commerceNeedsOptionSupport(all.selection), true);
  const limited = reviewCommerceCart(previous, [{...cold,skuAvailable:2}, {...row,skuAvailable:2}]);
  assert.deepEqual(Object.values(limited.selection).map(({row,quantity})=>[row.optionKey,quantity]), [["hot",1],["iced",1]]);
  assert.equal(commerceNeedsOptionSupport(limited.selection), false);
  assert.deepEqual(limited.shortages.map(({row,quantity})=>[row.optionKey,quantity]), [["iced",1]]);
});

import assert from "node:assert/strict";
import test from "node:test";
import {
  commerceChoiceDisclosure,
  commerceChoiceState,
  commerceChooseMessage,
  commerceLineKey,
  commerceMenuDisclosure,
  commercePick,
  commercePickChange,
  readPickupKitchen,
  type CommerceMenuMetadata,
  type CommercePrice,
} from "../src/index.ts";

const size = { key: "size", labels: { ko: "크기", en: "Size" } };
const milk = { key: "milk", labels: { ko: "우유" } };
const sizes = [["s", 0], ["l", 500]] as const;
const milks: [string, number, CommerceMenuMetadata["allergens"]][] = [["cow", 0, ["soy", "milk"]], ["oat", 700, ["soy"]]];
const rows = (change: (row: CommercePrice) => void = () => {}): CommercePrice[] =>
  sizes.flatMap(([s, a], i) => milks.map(([m, b, allergens], j) => {
    const row: CommercePrice = {
      version: 1, expiresAt: "2030-01-01T00:00:00Z", sku: "latte", optionKey: `${s}-${m}`, label: "라떼", asset: "0x" + "5".repeat(40),
      unitPrice: String(5000 + a + b), available: 3, skuAvailable: 3, metadata: { optionRequired: true, allergens },
      choices: [{ group: size, value: { key: s, labels: { ko: s }, order: i } }, { group: milk, value: { key: m, labels: { ko: m }, order: j } }],
    };
    change(row);
    return row;
  }));
const state = (list: CommercePrice[], picks = {}, cart = {}) => commerceChoiceState(list, picks, cart)!;
const value = (shown: ReturnType<typeof state>, group: number, index: number) => shown.groups[group].values[index];

test("nothing is picked for the customer, and the amounts follow the rows", () => {
  const shown = state(rows());
  assert.deepEqual(shown.groups.map(({ key, picked }) => [key, picked]), [["size", false], ["milk", false]]);
  assert.equal(shown.row, undefined);
  assert.deepEqual([shown.from, shown.varies], ["5000", true]);
  assert.deepEqual(shown.groups.map(({ values }) => values.map(({ extra }) => extra)), [[0n, 500n], [0n, 700n]]);
  assert.deepEqual(state(rows(), { size: "l" }).from, "5500");
  const picked = state(rows(), { size: "l", milk: "oat" });
  assert.deepEqual([picked.row?.optionKey, picked.from, picked.varies], ["l-oat", "6200", false]);
  for (const { optionKey, unitPrice } of rows()) {
    const [s, m] = optionKey.split("-");
    assert.equal(state(rows(), { size: s, milk: m }).row?.unitPrice, unitPrice);
  }
});

test("a product with one list of options has no groups", () => {
  assert.equal(commerceChoiceState(rows((row) => { delete row.choices; }), {}, {}), undefined);
});

test("sold out alone or only with what is picked, and the way back stays open", () => {
  const list = rows((row) => { if (row.optionKey === "l-cow" || row.optionKey === "s-oat") row.available = 0; });
  assert.deepEqual(state(list).groups.flatMap(({ values }) => values.map(({ soldOut }) => soldOut)), [undefined, undefined, undefined, undefined]);
  let picks = commercePick(commercePick({}, "size", "l"), "milk", "oat");
  const stuck = state(list, picks);
  assert.equal(value(stuck, 0, 0).soldOut, "others");
  assert.equal(value(stuck, 1, 0).soldOut, "others");
  picks = commercePick(picks, "size", "l");
  assert.deepEqual(picks, { milk: "oat" });
  assert.equal(value(state(list, picks), 1, 0).soldOut, undefined);
  const out = rows((row) => { if (row.optionKey.endsWith("oat")) row.available = 0; });
  assert.equal(value(state(out), 1, 1).soldOut, "always");
  // The cheapest combination is sold out, so the price starts at the next one.
  assert.equal(state(rows((row) => { if (row.optionKey === "s-cow") row.available = 0; })).from, "5500");
  assert.equal(state(rows((row) => { row.available = 0; })).from, "5000");
});

test("what is in the basket counts against the product's stock", () => {
  const list = rows();
  const cart = { [commerceLineKey(list[0])]: { row: list[0], quantity: 3 } };
  assert.equal(value(state(list, {}, cart), 0, 1).soldOut, "always");
  assert.equal(value(state(list, {}, cart), 0, 0).soldOut, undefined);
});

test("a combination in the basket is edited, and its quantity does not follow to another one", () => {
  const list = rows();
  const cart = { [commerceLineKey(list[0])]: { row: list[0], quantity: 2 } };
  const edited = commercePickChange(list, { picks: { size: "s" }, quantity: 1 }, "milk", "cow", cart);
  assert.deepEqual(edited, { picks: { size: "s", milk: "cow" }, quantity: 2 });
  assert.equal(commercePickChange(list, edited, "milk", "oat", cart).quantity, 1);
  assert.equal(commercePickChange(list, { picks: { size: "l" }, quantity: 2 }, "milk", "oat", cart).quantity, 2);
});

test("a value or a group the menu no longer has is not picked", () => {
  const shown = state(rows(), { size: "xl", milk: "oat", shots: "two" });
  assert.deepEqual(shown.groups.map(({ picked }) => picked), [false, true]);
  assert.equal(shown.from, "5700");
  const odd = rows((row) => { row.choices![0] = { ...row.choices![0], group: { ...size, key: "constructor" } }; });
  assert.deepEqual(state(odd).groups.map(({ picked }) => picked), [false, false]);
});

test("allergens name what is certain and what depends on the choices", () => {
  assert.equal(commerceChoiceDisclosure(rows(), {}, "ko"), "점주 신고 성분 대두, 옵션에 따라 우유 · 교차접촉 미입력");
  assert.equal(commerceChoiceDisclosure(rows(), {}, "en"), "Merchant-declared ingredients soy, depending on your choices milk · Cross-contact information not supplied");
  for (const [m, text] of [["cow", "대두, 우유"], ["oat", "대두"]])
    assert.equal(commerceChoiceDisclosure(rows(), { milk: m }, "ko"), `점주 신고 성분 ${text} · 교차접촉 미입력`);
  const only = rows((row) => { row.metadata!.allergens = row.optionKey.endsWith("cow") ? ["milk"] : []; });
  assert.equal(commerceChoiceDisclosure(only, {}, "ko"), "점주 신고 성분 옵션에 따라 우유 · 교차접촉 미입력");
  assert.equal(commerceChoiceDisclosure(only, { milk: "oat" }, "ko"), "점주 신고 성분 없음 · 교차접촉 미입력");
  const none = rows((row) => { delete row.metadata!.allergens; });
  assert.equal(commerceChoiceDisclosure(none, {}, "ko"), commerceMenuDisclosure(none[0], "ko"));
  // One row without a declaration must not read as complete.
  const some = rows((row) => { if (row.optionKey === "s-cow") delete row.metadata!.allergens; });
  assert.match(commerceChoiceDisclosure(some, {}, "ko"), /미입력/);
});

test("the message names the groups with the right particle", () => {
  assert.equal(commerceChooseMessage(["온도", "크기", "샷"], "ko"), "온도·크기·샷을 골라 주세요");
  assert.equal(commerceChooseMessage(["크기"], "ko"), "크기를 골라 주세요");
  assert.equal(commerceChooseMessage(["Size"], "ko"), "Size 옵션을 골라 주세요");
  assert.equal(commerceChooseMessage(["Size", "Milk"], "en"), "Choose Size, Milk");
});

test("the kitchen reads the name of a combination and refuses a broken one", () => {
  const order = (item: object) => JSON.stringify({ pickups: [{ saleRef: "0fbc54ef-2827-4ab5-bfcd-3a27611c3001", startsAt: "2026-11-26T01:00:00Z",
    timezone: "Asia/Seoul", state: "accepted", pickupNo: 1, revision: 0, items: [{ label: "라떼", option: "k3x9-p2", quantity: 1, ...item }] }] });
  assert.deepEqual(readPickupKitchen(order({ optionLabel: "차가움 · 귀리" }))[0].items, [{ label: "라떼", option: "k3x9-p2", optionLabel: "차가움 · 귀리", quantity: 1 }]);
  assert.deepEqual(readPickupKitchen(order({}))[0].items, [{ label: "라떼", option: "k3x9-p2", quantity: 1 }]);
  for (const optionLabel of ["", 3, "가".repeat(161)])
    assert.throws(() => readPickupKitchen(order({ optionLabel })), /PICKUP_UNAVAILABLE/);
});

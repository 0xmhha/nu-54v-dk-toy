// The kiosk order: cart lines and total, daily order numbers, the receipt it builds.
import { buildReceipt, cartItems, cartTotal, nextOrderNumber, receiptForPhone } from "../src/kiosk/menu.ts";
import type { KioskConfig } from "../src/kiosk/config.ts";

const menu = [{ name: "Americano", price: "1500000" }, { name: "Cookie", price: "1000000" }, { name: "Water", price: "500000" }];

test("cart lines skip what was not chosen and add up", () => {
  const items = cartItems(menu, { 0: 2, 2: 1, 1: 0 });
  expect(items).toEqual([{ name: "Americano", qty: 2, unitPrice: "1500000" }, { name: "Water", qty: 1, unitPrice: "500000" }]);
  expect(cartTotal(items)).toBe(3_500_000n);
});

test("order numbers count up within a day and start again the next day", async () => {
  const kv = new Map<string, string>();
  const vault = { getSetting: async (n: string) => kv.get(n) ?? null, putSetting: async (n: string, v: string) => { kv.set(n, v); } };
  const day1 = new Date(2026, 9, 5, 10), day2 = new Date(2026, 9, 6, 9);
  expect(await nextOrderNumber(vault, day1)).toBe("A-0001");
  expect(await nextOrderNumber(vault, day1)).toBe("A-0002");
  expect(await nextOrderNumber(vault, day2)).toBe("A-0001");
});

test("the receipt has the attested name, the merchant's details and fits payment.outcome", () => {
  const config = {
    chainId: 8283, tokenSymbol: "tUSDC", tokenDecimals: 6,
    attestation: { name: "NU54 Test Cafe" },
    merchantProfile: { representative: "Test Representative", businessNumber: "000-00-00000", address: "Test address", phone: "000-0000-0000" },
  } as unknown as KioskConfig;
  const items = cartItems(menu, { 0: 1, 1: 1 });
  const r = buildReceipt(config, { orderNumber: "A-0003", orderId: "0x" + "01".repeat(32), items, total: cartTotal(items), payer: "0x" + "bc".repeat(20), time: 1_790_000_000 });
  expect(r).toMatchObject({ store: "NU54 Test Cafe", representative: "Test Representative", businessNumber: "000-00-00000", total: "2500000", token: { symbol: "tUSDC", decimals: 6 } });
  expect(receiptForPhone(r)).toContain('"orderNumber":"A-0003"');
  // A receipt too large for payment.outcome is not sent; the kiosk still shows it.
  const many = Array.from({ length: 80 }, (_, k) => ({ name: `Item ${k} with a long name`, qty: 1, unitPrice: "1" }));
  expect(receiptForPhone({ ...r, items: many, total: "80" })).toBeUndefined();
});

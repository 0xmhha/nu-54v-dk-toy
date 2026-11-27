import assert from "node:assert/strict";
import test from "node:test";
import {
  bindServiceRequestV2,
  CommerceClient,
  CommerceHoldRefusal,
  readHoldRefusal,
  type CommerceCart,
  type ServiceRequestV2,
} from "../src/index.ts";

// The customer entrance says why the cart of a proven board account was not held (opsd customer_funds.go). The client
// hands the reason on, so the app can say a short balance, a balance that could not be read and a limit apart.

const branch = "0fbc54ef-2827-4ab5-bfcd-3a27611c3001";
const actor = "0fbc54ef-2827-4ab5-bfcd-3a27611c3002";
const tenant = "0fbc54ef-2827-4ab5-bfcd-3a27611c3003";
const now = 1_750_000_000;
const profile = { subjectRef: actor, role: "customer", branchRef: branch, tenantRef: tenant, expiresAt: now + 120 } as const;
const cart: CommerceCart = { origin: "menu", originRef: "0fbc54ef-2827-4ab5-bfcd-3a27611c3005", menuVersion: 1, items: [{ sku: "tea", option_key: "standard", quantity: 1 }] };
const asset = "0x" + "2".repeat(40);

/** A client whose server answers `order.create` with one error reply, bound to the request as the server binds it. */
function refusing(reply: Record<string, unknown>, operation = "order.create") {
  return new CommerceClient(profile, async (raw) => {
    const { command } = JSON.parse(raw) as { command: ServiceRequestV2 };
    const { requestId, requestDigest } = bindServiceRequestV2(command,
      { ...profile, grants: [{ domainRef: command.domainRef, objectRef: command.objectRef, operations: [`C3/${operation}`] }] }, now);
    return JSON.stringify({ requestId, requestDigest, status: "error", ...reply });
  }, async () => "0fbc54ef-2827-4ab5-bfcd-3a27611c3004", () => now);
}
const reserve = (reply: Record<string, unknown>) => refusing(reply).reserve(cart, asset, "0fbc54ef-2827-4ab5-bfcd-3a27611c3006");

test("a short balance comes with the cart's amount and the account's balance in base units", async () => {
  await assert.rejects(reserve({ error: "CHECKOUT_CONFLICT", reason: "balance_short", required: "300000", balance: "299999" }), (error) => {
    assert.ok(error instanceof CommerceHoldRefusal);
    assert.deepEqual([error.message, error.reason, error.required, error.balance], ["CHECKOUT_CONFLICT", "balance_short", "300000", "299999"]);
    return true;
  });
  await assert.rejects(reserve({ error: "CHECKOUT_CONFLICT", reason: "balance_short", required: "300000", balance: "0" }),
    (error) => error instanceof CommerceHoldRefusal && error.balance === "0");
});

test("a balance that could not be read and a limit on carts are told apart, with no numbers", async () => {
  for (const reason of ["balance_unavailable", "hold_limit"] as const)
    await assert.rejects(reserve({ error: "BUSY", reason }), (error) => {
      assert.ok(error instanceof CommerceHoldRefusal);
      assert.deepEqual([error.message, error.reason, error.required, error.balance], ["BUSY", reason, undefined, undefined]);
      return true;
    });
});

test("a refusal that does not read is not a refusal the app can explain", async () => {
  for (const reply of [
    { error: "CHECKOUT_CONFLICT", reason: "balance_short", required: "300000", balance: "300000" }, // not short
    { error: "CHECKOUT_CONFLICT", reason: "balance_short", required: "300000", balance: "300001" },
    { error: "CHECKOUT_CONFLICT", reason: "balance_short", required: "300000" },
    { error: "CHECKOUT_CONFLICT", reason: "balance_short", required: 300000, balance: "1" },
    { error: "CHECKOUT_CONFLICT", reason: "balance_short", required: "3e5", balance: "1" },
    { error: "CHECKOUT_CONFLICT", reason: "balance_short", required: "0300000", balance: "1" },
    { error: "CHECKOUT_CONFLICT", reason: "balance_short", required: "300000", balance: "-1" },
    { error: "BUSY", reason: "balance_short", required: "300000", balance: "1" },
    { error: "CHECKOUT_CONFLICT", reason: "balance_unavailable" },
    { error: "CHECKOUT_CONFLICT", reason: "hold_limit" },
    { error: "BUSY", reason: "hold_limit", balance: "1" },
    { error: "BUSY", reason: "later" },
    { error: "BUSY", reason: 7 },
  ])
    await assert.rejects(reserve(reply), (error) => !(error instanceof CommerceHoldRefusal) && (error as Error).message === "BAD_RESPONSE", JSON.stringify(reply));
});

test("an error without a reason stays the plain code, and a reason on another request is not a hold refusal", async () => {
  await assert.rejects(reserve({ error: "CHECKOUT_CONFLICT" }), (error) => !(error instanceof CommerceHoldRefusal) && (error as Error).message === "CHECKOUT_CONFLICT");
  await assert.rejects(reserve({ error: "BUSY" }), (error) => !(error instanceof CommerceHoldRefusal) && (error as Error).message === "BUSY");
  await assert.rejects(refusing({ error: "CHECKOUT_CONFLICT", reason: "balance_short", required: "2", balance: "1" }, "order.status").status(branch),
    (error) => !(error instanceof CommerceHoldRefusal) && (error as Error).message === "CHECKOUT_CONFLICT");
  assert.equal(readHoldRefusal({ error: "BUSY" }), undefined);
});

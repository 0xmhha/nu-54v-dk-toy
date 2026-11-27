import assert from "node:assert/strict";
import test from "node:test";
import {createHash} from "node:crypto";
import {
  bindServiceRequestV2,
  CommerceClient,
  encodeCommerceCart,
  readPickupCustomer,

  type ServiceRequestV2,
} from "../src/index.ts";

const ref = (digit: number) => `0fbc54ef-2827-4ab5-bfcd-3a27611c300${digit}`;
const now = 1_750_000_000;
const profile = {
  subjectRef: ref(1), role: "customer", branchRef: ref(2), tenantRef: ref(3), expiresAt: now + 120,
} as const;
const id = ref(4);
const paid = {
  id, revision: 2, state: "paid", asset: `0x${"a".repeat(40)}`, amount: "100",
  expiresAt: new Date((now + 60) * 1000).toISOString(), receiptRef: ref(5),
};

async function status(order: object, role: "customer" | "terminal" = "customer") {
  const scoped = {...profile, role};
  const client = new CommerceClient(scoped, async (raw) => {
    const {command} = JSON.parse(raw) as {command: ServiceRequestV2};
    const binding = bindServiceRequestV2(command, {...scoped, grants: [{
      domainRef: profile.branchRef, objectRef: id, operations: ["C3/order.status"],
    }]}, now);
    return JSON.stringify({...binding, status: "ok", resultRef: id, order});
  }, async () => ref(6), () => now);
  return client.status(id);
}

test("paid physical and explicitly typed digital and booking receipts remain separate", async () => {
  assert.deepEqual(await status({...paid, pickupNo: 12}), {...paid, pickupNo: 12});
  for (const receiptKind of ["digital", "booking"]) {
    assert.deepEqual(await status({...paid, receiptKind}), {...paid, receiptKind});
    await assert.rejects(status({...paid, receiptKind, receiptRef: undefined}), /BAD_RESPONSE/);
    await assert.rejects(status({...paid, receiptKind, pickupNo: 1}), /BAD_RESPONSE/);
  }
  await assert.rejects(status(paid), /BAD_RESPONSE/);
  await assert.rejects(status({...paid, receiptKind: "other"}), /BAD_RESPONSE/);
  await assert.rejects(status({...paid, pickupNo: 0}), /BAD_RESPONSE/);
});

test("unpaid orders never expose pickup or receipt evidence", async () => {
  const preview = {...paid, state: "preview", receiptRef: undefined};
  assert.equal((await status(preview)).state, "preview");
  for (const evidence of [{pickupNo: 1}, {receiptKind: "digital"}, {receiptRef: ref(5)}]) {
    await assert.rejects(status({...preview, ...evidence}), /BAD_RESPONSE/);
  }
});

test("menu terminal gets optional, state-matched final outcomes", async () => {
  const menuCheckout = {branchLabel: "Fixture branch", payerBound: true};
  const completed = {...paid, pickupNo: 2};
  const closed = {...paid, state: "closed", receiptRef: undefined};
  assert.deepEqual((await status({...completed, menuCheckout}, "terminal")).menuCheckout, menuCheckout);
  const relayPaid = {...menuCheckout, finalOutcome: "relay-paid"};
  const expiredUnpaid = {...menuCheckout, finalOutcome: "expired-unpaid"};
  assert.deepEqual((await status({...completed, menuCheckout: relayPaid}, "terminal")).menuCheckout, relayPaid);
  assert.deepEqual((await status({...closed, menuCheckout: expiredUnpaid}, "terminal")).menuCheckout, expiredUnpaid);
  await assert.rejects(status({...completed, menuCheckout: relayPaid}), /BAD_RESPONSE/);
  await assert.rejects(status({...closed, menuCheckout: relayPaid}, "terminal"), /BAD_RESPONSE/);
  await assert.rejects(status({...completed, menuCheckout: expiredUnpaid}, "terminal"), /BAD_RESPONSE/);
  await assert.rejects(status({...closed, state: "checking", menuCheckout: expiredUnpaid}, "terminal"), /BAD_RESPONSE/);
});

test("subscription reconciliation and proven expiration keep the signed quote reference", async () => {
  const subscription = {
    invoiceRef: ref(7), cycle: "2026-11-20", termsVersion: "v2",
    dueAt: "2026-11-27T00:00:00.000Z", quoteExpiresAt: "2026-11-20T00:02:00.000Z",
    quoteRef: `0x${"a".repeat(64)}`,
  };
  const unpaid = {...paid, receiptRef: undefined};
  assert.equal((await status({...unpaid, state: "checking",
    subscription: {...subscription, stage: "reconcile"}})).subscription?.quoteRef, subscription.quoteRef);
  assert.equal((await status({...unpaid, state: "closed",
    subscription: {...subscription, stage: "expired"}})).subscription?.stage, "expired");
  await assert.rejects(status({...unpaid, state: "closed",
    subscription: {...subscription, stage: "expired", quoteRef: undefined}}), /BAD_RESPONSE/);
  await assert.rejects(status({...unpaid, state: "checking",
    subscription: {...subscription, stage: "expired"}}), /BAD_RESPONSE/);
});

test("pickup cart encodes the same exact scheduling commitment as P05", () => {
  const cart = {
    origin: "menu", originRef: "0fb9757b-d626-4fb8-b02e-68a9fc8930f0", menuVersion: 1,
    items: [{sku: "drip", option_key: "hot", quantity: 1}],
    pickup: {window: "23a6f801-e925-42b9-b4fa-05f5d4f11cee", startsAt: "2027-01-02T09:00:00Z"},
  } as const;
  const canonical = encodeCommerceCart({...cart, items: [...cart.items]});
  assert.equal(createHash("sha256").update(canonical).digest("hex"), "c7735b29a273967cb0b629088b18c01be893c2cd5c9c92be8ce558bb81ae44ec");
  assert.throws(() => encodeCommerceCart({...cart, items:[...cart.items], origin:"pos"}), /BAD_REQUEST/);
});

test("pickup acknowledgment and refund confirmation require the appropriate evidence", () => {
  const base = {saleRef: id, startsAt: paid.expiresAt, timezone:"Asia/Seoul", amount: paid.amount, asset: paid.asset};
  const raw = (order: object) => JSON.stringify({pickups:[{...base, ...order}]});
  assert.equal(readPickupCustomer(raw({state:"awaiting-acceptance", receiptRef: paid.receiptRef}))[0].pickupNo, undefined);
  assert.throws(() => readPickupCustomer(raw({state:"awaiting-acceptance", pickupNo: 1, receiptRef:paid.receiptRef})));
  const failed = {state:"unfulfillable", pickupNo: 1, receiptRef: paid.receiptRef};
  assert.equal(readPickupCustomer(raw({...failed, refund:{state:"confirmed", amount:"100", receiptRef:ref(9)}}))[0].refund?.receiptRef, ref(9));
  assert.throws(() => readPickupCustomer(raw({...failed, refund:{state:"pending", amount:"100", receiptRef:ref(9)}})));
  assert.throws(() => readPickupCustomer(raw({...failed, refund:{state:"confirmed", amount:"100"}})));
});

test("private order pages reject expired claims and nonadvancing cursors", async () => {
  const summary = {id, state: "preview", asset: paid.asset, amount: paid.amount};
  async function page(rows: unknown[], after?: string) {
    const client = new CommerceClient(profile, async raw => {
      const {command} = JSON.parse(raw) as {command: ServiceRequestV2};
      assert.equal(command.objectRef, profile.subjectRef);
      assert.equal(command.body.afterSale, after ?? profile.subjectRef);
      const binding = bindServiceRequestV2(command, {...profile, grants: [{
        domainRef: profile.branchRef, objectRef: profile.subjectRef, operations: ["C3/order.list"],
      }]}, now);
      return JSON.stringify({...binding, status: "ok", resultRef: profile.subjectRef, orders: rows});
    }, async () => ref(6), () => now);
    return client.orders(after);
  }
  assert.deepEqual(await page([summary]), [summary]);
  assert.deepEqual(await page([{...summary, accessEndsAt: paid.expiresAt}]), [summary]);
  await assert.rejects(page([{...summary, accessEndsAt: new Date(now * 1000).toISOString()}]), /BAD_RESPONSE/);
  await assert.rejects(page([summary, summary]), /BAD_RESPONSE/);
  await assert.rejects(page([summary], id), /BAD_RESPONSE/);
});

test("only finalized order evidence can attach the confirmed transaction, time and pickup state", async () => {
  const relayTxHash = `0x${"b".repeat(64)}`;
  const includedAt = paid.expiresAt;
  const pending = {...paid, state: "checking", receiptRef: undefined};
  for (const stage of ["relaySigned", "submitted", "checking"]) {
    assert.equal((await status({...pending, payment: {stage, chainId: 8283, relayTxHash}})).payment?.relayTxHash, relayTxHash);
  }
  assert.equal((await status({...pending, payment: {stage: "received", chainId: 8283}})).payment?.stage, "received");
  assert.equal((await status({...pending, state: "closed", payment: {stage: "unpaid", chainId: 8283, relayTxHash}})).payment?.stage, "unpaid");
  for (const malicious of [{includedAt}, {txHash: relayTxHash}, {pickupStage: "ready"}]) {
    await assert.rejects(status({...pending, payment: {stage: "checking", chainId: 8283, relayTxHash, ...malicious}}), /BAD_RESPONSE/);
  }
  await assert.rejects(status({...pending, payment: {stage: "finalized", chainId: 8283, txHash: relayTxHash, includedAt}}), /BAD_RESPONSE/);
  const originalReceipt = {...paid, pickupNo: 17};
  const finalized = {stage: "finalized", chainId: 8283, txHash: relayTxHash, includedAt};
  assert.deepEqual(await status({...originalReceipt, payment: finalized}), {...originalReceipt, payment: finalized});
  await assert.rejects(status({...originalReceipt, payment: {...finalized, pickupStage: "accepted"}}), /BAD_RESPONSE/);
  assert.equal((await status({...paid, pickupNo: 17,
    payment: {stage: "finalized", chainId: 8283, txHash: relayTxHash, includedAt, pickupStage: "collected"},
  })).payment?.pickupStage, "collected");
  await assert.rejects(status({...paid, receiptKind: "booking",
    payment: {stage: "finalized", chainId: 8283, txHash: relayTxHash, includedAt, pickupStage: "ready"},
  }), /BAD_RESPONSE/);
});

test("kitchen decoding exposes only the pickup fields used by the authenticated kitchen", async () => {
  const {readPickupKitchen} = await import("../src/commerce-pickup.ts");
  const saleRef = "00000000-0000-4000-8000-000000000011";
  const eventRef = "00000000-0000-4000-8000-000000000012";
  const visible = {
    saleRef, eventRef, kind: "cafe", state: "ready", startsAt: "2026-03-01T09:00:00Z",
    timezone: "Asia/Seoul", pickupNo: 2, revision: 1,
    items: [{label: "Coffee", option: "hot", quantity: 1}],
  };
  assert.deepEqual(readPickupKitchen(JSON.stringify({pickups: [{
    ...visible, terminal: "private-terminal", customerRef: "private-customer",
    menuCheckout: {finalOutcome: "relay-paid"}, futureTerminalGrant: "private-grant",
    items: [{...visible.items[0], customerRef: "private-item-field"}],
  }]})), [visible]);
});


test("pickup receipt dates preserve both clocks and enforce ledger readiness order", async () => {
  const hash = `0x${"ab".repeat(32)}`;
  const includedAt = "2026-10-01T00:00:01Z";
  const finalizedAt = "2026-10-01T00:00:03Z";
  const pickupReadyAt = "2026-10-01T00:00:09Z";
  const payment = {stage: "finalized" as const, chainId: 8283 as const, txHash: hash, includedAt,
    finalizedAt, pickupReadyAt, pickupStage: "ready" as const};
  const original = {...paid, pickupNo: 2, payment};
  assert.deepEqual(await status(original), original);
  const futureInclusion = {...original, payment: {...payment, includedAt: "2026-10-01T00:00:20Z"}};
  assert.deepEqual(await status(futureInclusion), futureInclusion);
  for (const bad of [
    {...payment, includedAt: 2026},
    {...payment, finalizedAt: undefined},
    {...payment, finalizedAt: "invalid"},
    {...payment, pickupReadyAt: includedAt},
    {...payment, pickupStage: "preparing"},
    {...payment, pickupReadyAt: 3},
  ]) await assert.rejects(status({...original, payment: bad}), /BAD_RESPONSE/);
});

test("the original nearby unpaid marker is customer-only and requires final unpaid evidence", async () => {
  const closed = {...paid, state: "closed", receiptRef: undefined};
  const legacy = {stage: "unpaid", chainId: 8283};
  assert.equal((await status({...closed, payment: legacy})).payment?.nearbyUnpaid, undefined);
  const authorized = {...legacy, nearbyUnpaid: true};
  assert.equal((await status({...closed, payment: authorized})).payment?.nearbyUnpaid, true);
  await assert.rejects(status({...closed, payment: authorized}, "terminal"), /BAD_RESPONSE/);
  await assert.rejects(status({...closed, payment: {...authorized, nearbyUnpaid: false}}), /BAD_RESPONSE/);
  await assert.rejects(status({...closed, state: "checking",
    payment: {...authorized, stage: "received"}}), /BAD_RESPONSE/);
  await assert.rejects(status({...closed, payment: {...authorized, txHash: `0x${"a".repeat(64)}`}}), /BAD_RESPONSE/);
});

test("a finalized travel pickup retains payment proof before merchant acceptance without exposing a number", async () => {
  const payment = {stage: "finalized", chainId: 8283, txHash: `0x${"1".repeat(64)}`, includedAt: paid.expiresAt, finalizedAt: paid.expiresAt};
  const waiting = {...paid, receiptKind: "pickup-awaiting-acceptance", payment};
  assert.deepEqual(await status(waiting), waiting);
  await assert.rejects(status({...waiting, pickupNo: 12}), /BAD_RESPONSE/);
  await assert.rejects(status({...waiting, payment: {...payment, pickupStage: "ready"}}), /BAD_RESPONSE/);
  await assert.rejects(status({...waiting, state: "checking", receiptRef: undefined, payment: undefined}), /BAD_RESPONSE/);
  await assert.rejects(status(waiting, "terminal"), /BAD_RESPONSE/);
  assert.equal((await status({...paid, pickupNo: 12, payment})).pickupNo, 12);
});

test("ready cafe retains the complete original booking and ready evidence", async () => {
  const {readPickupKitchen} = await import("../src/commerce-pickup.ts");
  const core = {
    saleRef: ref(4), eventRef: ref(5), startsAt: "2030-10-19T10:39:00Z",
    timezone: "Asia/Seoul", state: "ready", kind: "cafe", revision: 2, pickupNo: 16,
  };
  const settlement = {
    amount: "8000000", asset: `0x${"b".repeat(40)}`,
    finalizedAt: "2030-10-19T01:40:00.123456Z", pickupReadyAt: "2030-10-19T01:41:00.123456Z",
  };
  const read = (order: object) => readPickupKitchen(JSON.stringify({pickups: [order]}))[0];
  assert.deepEqual(read({...core, settlement: {...settlement, payer: "hidden"}}).settlement, settlement);
  assert.equal(read(core).settlement, undefined);
  for (const wrong of [null, {...settlement, amount: 8000000}, {...settlement, asset: "unknown"},
    {...settlement, pickupReadyAt: undefined}, {...settlement, pickupReadyAt: "2030-10-19T01:39:00.000000Z"},
    {...settlement, pickupReadyAt: "2030-10-19T01:40:00.123455Z"},
    {...settlement, finalizedAt: "2030-02-31T01:40:00.000000Z"},
    {...settlement, finalizedAt: "2030-10-19T10:40:00+09:00"}]) {
    assert.throws(() => read({...core, settlement: wrong}), /PICKUP_UNAVAILABLE/);
  }
  for (const state of ["accepted", "preparing", "unfulfillable"]) {
    assert.throws(() => read({...core, state, settlement}), /PICKUP_UNAVAILABLE/);
  }
  assert.throws(() => read({...core, kind: "travel", settlement}), /PICKUP_UNAVAILABLE/);
  assert.throws(() => read({...core, state: "checking", revision: 0, eventRef: undefined,
    pickupNo: undefined, settlement}), /PICKUP_UNAVAILABLE/);
  assert.deepEqual(read({...core, state: "collected", settlement}).settlement, settlement);
});

test("pickup readers hold an unsupported timezone before it can reach the renderer", async () => {
  const {readPickupKitchen} = await import("../src/commerce-pickup.ts");
  const booking = {saleRef: ref(4), startsAt: "2030-10-19T01:39:00Z",
    state: "ready", revision: 2, pickupNo: 16};
  const read = (timezone: string) => readPickupKitchen(JSON.stringify({pickups: [{...booking, timezone}]}));
  for (const zone of ["UTC", "Asia/Seoul", "Etc/GMT+9"]) assert.equal(read(zone)[0].timezone, zone);
  for (const zone of ["invalid/zone", "", "+09:00"]) assert.throws(() => read(zone), /PICKUP_UNAVAILABLE/);
});

test("customer forwards and reopens the same cafe payment receipt", async () => {
  const cafeReview = {branchLabel: "Fixture branch", payerBound: true};
  const {receiptRef, ...original} = paid;
  const order = {...original, cafeReview, state: "checking", payment: {chainId: 8283, stage: "received"}};
  const quoteDigest = "a".repeat(64), approvalDigest = "b".repeat(64);
  const signature = "c".repeat(128) + "1b";
  const key = ref(5);
  const forwards: ServiceRequestV2[] = [];
  let nextId = 6;
  const client = new CommerceClient(profile, async (raw) => {
    const {command, approvalSignature} = JSON.parse(raw) as {command: ServiceRequestV2; approvalSignature?: string};
    assert.equal(command.objectRef, id);
    const binding = bindServiceRequestV2(command, {...profile, grants: [{
      domainRef: profile.branchRef, objectRef: id, operations: [`C3/${command.operation}`],
    }]}, now);
    if (command.operation === "payment.forward") {
      assert.equal(approvalSignature, signature);
      assert.deepEqual(command.body, {quoteDigest, approvalDigest});
      forwards.push(command);
      return JSON.stringify({...binding, status: "ok", resultRef: id});
    }
    assert.equal(command.operation, "order.status");
    return JSON.stringify({...binding, status: "ok", resultRef: id, order});
  }, async () => ref(nextId++), () => now);

  const first = await client.forward(id, quoteDigest, approvalDigest, signature, key);
  const replay = await client.forward(id, quoteDigest, approvalDigest, signature, key);
  assert.equal(first, id);
  assert.equal(replay, id);
  assert.deepEqual(forwards.map(request => request.idempotencyKey), [key, key]);
  assert.notEqual(forwards[0].requestId, forwards[1].requestId);
  assert.deepEqual(await client.status(replay), order);
});

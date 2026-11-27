import assert from "node:assert/strict";
import { lineCount, lineName, pickupScanUrl, scanOutcome, scannedToken, sendPickupScan } from "../src/pickupScan.ts";

const token = "ab".repeat(32);
const branch = "11111111-1111-4111-8111-111111111111";
const sale = "44444444-4444-4444-8444-444444444425";
const other = "44444444-4444-4444-8444-444444444426";

// What the camera, a handheld scanner or a paste gives: only the pickup QR form, with the field's outer spaces dropped.
assert.equal(scannedToken(`nu54://pickup/${token}`), token);
assert.equal(scannedToken(`  nu54://pickup/${token}\n`), token);
for (const text of ["", token, `nu54://pickup/${token.toUpperCase()}`, `nu54://pickup/${token}x`, `nu54://pickup/ ${token}`,
  `nu54://order?v=2&branch=${branch}&order=${sale}&expires=1`, `https://example.com/${token}`])
  assert.equal(scannedToken(text), undefined, JSON.stringify(text));

assert.equal(pickupScanUrl(branch, sale), `/v2/operations/pickup-scan/${branch}/${sale}`);

// The handover is told only for the order that was opened.
const reply = { saleRef: sale, revision: 4, pickupNo: 17, items: [{ label: "카페라떼", option: "뜨겁게 크게 샷 추가", quantity: 1 }, { label: "치즈케이크", option: "standard", quantity: 1 }] };
const handed = scanOutcome(200, reply, sale);
assert.deepEqual(handed, { kind: "handed", handed: { saleRef: sale, pickupNo: 17, items: [{ label: "카페라떼", option: "뜨겁게 크게 샷 추가", quantity: 1 }, { label: "치즈케이크", quantity: 1 }] } });
assert.deepEqual([lineName(handed.handed.items[0]), lineCount(handed.handed.items[0]), lineName(handed.handed.items[1])], ["카페라떼 뜨겁게 크게 샷 추가", "1개", "치즈케이크"]);
for (const bad of [{ ...reply, saleRef: other }, { ...reply, pickupNo: 0 }, { ...reply, items: [{ label: "", quantity: 1 }] }, { ...reply, items: [{ label: "x", quantity: 0 }] }, null, "ok"])
  assert.deepEqual(scanOutcome(200, bad, sale), { kind: "refused", reason: "unanswered" });

// The five reasons opsd names, each on its own, and the time of the handover a used QR already recorded.
const at = 1_764_207_060;
assert.deepEqual(scanOutcome(400, { status: "error", error: "EXPIRED", reason: "expired" }, sale), { kind: "refused", reason: "expired" });
assert.deepEqual(scanOutcome(409, { status: "error", error: "CONFLICT", reason: "already_used", recordedAt: at }, sale),
  { kind: "refused", reason: "already-used", recordedAt: new Date(at * 1000) });
assert.deepEqual(scanOutcome(409, { status: "error", error: "CONFLICT", reason: "already_used" }, sale), { kind: "refused", reason: "already-used" });
assert.deepEqual(scanOutcome(400, { status: "error", error: "MISMATCHED_EVIDENCE", reason: "other_store" }, sale), { kind: "refused", reason: "other-store" });
assert.deepEqual(scanOutcome(400, { status: "error", error: "MISMATCHED_EVIDENCE", reason: "other_order" }, sale), { kind: "refused", reason: "other-order" });
assert.deepEqual(scanOutcome(409, { status: "error", error: "CHECKOUT_CONFLICT", reason: "not_ready" }, sale), { kind: "refused", reason: "not-ready" });
// A reason under another status, an unknown reason and a time that is not one are not believed.
assert.deepEqual(scanOutcome(200, { reason: "expired" }, sale), { kind: "refused", reason: "unanswered" });
assert.deepEqual(scanOutcome(409, { error: "EXPIRED", reason: "expired" }, sale), { kind: "refused", reason: "unanswered" });
assert.deepEqual(scanOutcome(400, { error: "BAD_REQUEST", reason: "toString" }, sale), { kind: "refused", reason: "unanswered" });
assert.deepEqual(scanOutcome(400, { error: "EXPIRED", reason: "expired", recordedAt: at }, sale), { kind: "refused", reason: "expired" });
assert.deepEqual(scanOutcome(409, { error: "CONFLICT", reason: "already_used", recordedAt: "10:31" }, sale), { kind: "refused", reason: "already-used" });

// The handover's own time, the option's name over its key, and a repeated scan of a recorded QR.
const again = scanOutcome(200, { ...reply, recordedAt: at, alreadyRecorded: true,
  items: [{ label: "카페라떼", option: "hot-large-shot", optionLabel: "뜨겁게 크게 샷 추가", quantity: 1 }, { label: "치즈케이크", option: "standard", quantity: 1 }] }, sale);
assert.deepEqual(again, { kind: "handed", handed: { saleRef: sale, pickupNo: 17, recordedAt: new Date(at * 1000), alreadyRecorded: true,
  items: [{ label: "카페라떼", option: "뜨겁게 크게 샷 추가", quantity: 1 }, { label: "치즈케이크", quantity: 1 }] } });
assert.equal(scanOutcome(200, { ...reply, recordedAt: at }, sale).handed.alreadyRecorded, undefined);
assert.deepEqual(scanOutcome(200, { ...reply, items: [{ label: "x", optionLabel: 3, quantity: 1 }] }, sale), { kind: "refused", reason: "unanswered" });

// A server that names no reason: timed out, used and other store alike; another order; not ready; no access.
assert.deepEqual(scanOutcome(400, { status: "error", error: "EXPIRED" }, sale), { kind: "refused", reason: "unusable" });
assert.deepEqual(scanOutcome(400, { status: "error", error: "MISMATCHED_EVIDENCE" }, sale), { kind: "refused", reason: "other-order" });
assert.deepEqual(scanOutcome(409, { status: "error", error: "CHECKOUT_CONFLICT" }, sale), { kind: "refused", reason: "not-ready" });
assert.deepEqual(scanOutcome(404, null, sale), { kind: "refused", reason: "denied" });
for (const [status, body] of [[503, { error: "BUSY" }], [400, { error: "BAD_REQUEST" }], [409, { error: "CONFLICT" }], [500, null]])
  assert.deepEqual(scanOutcome(status, body, sale), { kind: "refused", reason: "unanswered" });

// The request carries the opened order's branch and sale in the path and only the token in the body.
const calls = [];
const post = async (url, init) => { calls.push([url, init]); return new Response(JSON.stringify(reply), { status: 200 }); };
assert.equal((await sendPickupScan(branch, sale, token, post)).kind, "handed");
assert.deepEqual(calls, [[`/v2/operations/pickup-scan/${branch}/${sale}`, { method: "POST", credentials: "same-origin",
  headers: { "Content-Type": "application/json" }, body: JSON.stringify({ token }) }]]);
// No answer at all is not a refusal of the QR.
assert.deepEqual(await sendPickupScan(branch, sale, token, async () => { throw new TypeError("offline"); }), { kind: "refused", reason: "unanswered" });
assert.deepEqual(await sendPickupScan(branch, sale, token, async () => new Response("<html>", { status: 502 })), { kind: "refused", reason: "unanswered" });

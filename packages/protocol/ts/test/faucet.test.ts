import assert from "node:assert/strict";
import test from "node:test";
import { CommerceClient } from "../src/commerce-v2.ts";
import { validFaucetStatus, type FaucetStatus } from "../src/faucet.ts";
import { bindServiceRequestV2, type ServiceRequestV2 } from "../src/service-v2.ts";
import { validTerminalDevice, validTerminalDeviceValue, type TerminalDevice } from "../src/terminal-device.ts";

const token = "0x" + "a".repeat(40);
const payer = "0x" + "b".repeat(40);
const id = (n: number) => `00000000-0000-4000-8000-${String(n).padStart(12, "0")}`;
const profile = { tenantRef: id(1), subjectRef: id(2), branchRef: id(3), expiresAt: 2_000_000_000 };
const now = 1_800_000_000;
const status: FaucetStatus = { payer, tokens: [{ token, symbol: "tUSDC", decimals: 6, amount: "5000000", perDay: 2, remaining: 1, state: "ready",
  claim: { amount: "5000000", to: payer, createdAt: now - 60, state: "final", txHash: "0x" + "c".repeat(64) } }] };
const device: TerminalDevice = { revision: 2, language: "en", idleSeconds: 90, readServer: "https://rpc.example/" };

function client(role: "customer" | "operator" | "terminal", answer: Record<string, unknown>, sent: any[] = []) {
  return new CommerceClient({ ...profile, role }, async raw => {
    const outer = JSON.parse(raw) as { command: ServiceRequestV2 };
    sent.push(outer);
    const scope = { ...profile, role, grants: [{ domainRef: outer.command.domainRef, objectRef: outer.command.objectRef, operations: ["C3/" + outer.command.operation] }] };
    const binding = bindServiceRequestV2(outer.command, scope, now);
    return JSON.stringify({ requestId: outer.command.requestId, requestDigest: binding.requestDigest, status: "ok", resultRef: outer.command.objectRef, ...answer });
  }, async () => id(9), () => now);
}

test("a claim names only the token", async () => {
  const sent: any[] = [];
  await client("customer", {}, sent).claimFaucet(token, id(7));
  assert.equal(sent[0].command.operation, "faucet.claim");
  assert.equal(sent[0].command.objectRef, profile.subjectRef);
  assert.deepEqual(sent[0].command.body, { token });
  assert.equal(sent[0].command.idempotencyKey, id(7));
  await assert.rejects(client("customer", {}).claimFaucet("0x" + "A".repeat(40), id(7)), /BAD_REQUEST/);
  await assert.rejects(client("terminal", {}).claimFaucet(token, id(7)), /NOT_AUTHORIZED/);
  await assert.rejects(client("customer", { status: "error", error: "UNSUPPORTED_CAPABILITY", resultRef: undefined }).faucetStatus(), /UNSUPPORTED_CAPABILITY/);
});

test("the faucet state is checked", async () => {
  assert.deepEqual(await client("customer", { faucet: status }).faucetStatus(), status);
  await assert.rejects(client("customer", {}).faucetStatus(), /BAD_RESPONSE/);
  const row = status.tokens[0];
  for (const bad of [{ ...row, remaining: 3 }, { ...row, state: "ready", remaining: 0 }, { ...row, amount: "0" }, { ...row, extra: 1 },
    { ...row, availableAt: now }, { ...row, claim: { ...row.claim!, txHash: undefined } }, { ...row, claim: { ...row.claim!, state: "queued" } }, { ...row, symbol: "<b>" }])
    assert.equal(validFaucetStatus({ payer, tokens: [bad] }), false);
  assert.equal(validFaucetStatus({ payer, tokens: [row, row] }), false);
  assert.equal(validFaucetStatus({ tokens: [{ ...row, state: "limit", remaining: 0, availableAt: now, claim: undefined }] }), true);
  assert.equal(validFaucetStatus({ tokens: [] }), false);
});

test("the network list comes back as one signed text", async () => {
  const raw = await client("customer", { networkList: { payload: "{}", signature: "0x00" } }).networkList();
  assert.equal(raw, '{"payload":"{}","signature":"0x00"}');
  await assert.rejects(client("customer", {}).networkList(), /BAD_RESPONSE/);
});

test("a kiosk setting is bound to its value, and only an operator sends it", async () => {
  const sent: any[] = [];
  assert.deepEqual(await client("operator", { terminalDevice: { ...device, revision: 3 } }, sent).changeTerminalDevice(id(5), "read-server", "https://rpc.example/", 2, id(7)),
    { ...device, revision: 3 });
  assert.equal(sent[0].terminalValue, "https://rpc.example/");
  assert.equal(sent[0].command.body.valueDigest.length, 64);
  await assert.rejects(client("operator", { terminalDevice: device }).changeTerminalDevice(id(5), "language", "en", 2, id(7)), /MISMATCHED_EVIDENCE/);
  await assert.rejects(client("terminal", { terminalDevice: device }).changeTerminalDevice(id(5), "language", "en", 1, id(7)), /NOT_AUTHORIZED/);
  for (const [name, value] of [["read-server", "http://rpc.example"], ["read-server", "https://a@b"], ["language", "fr"], ["idle-seconds", "59"], ["sound", "1"]])
    assert.equal(validTerminalDeviceValue(name, value), false);
  assert.equal(validTerminalDeviceValue("read-server", ""), true);
  assert.deepEqual(await client("terminal", { terminalDevice: device }).terminalDevice(), device);
  assert.equal(validTerminalDevice({ ...device, changes: [] }, "terminal"), false);
  assert.equal(validTerminalDevice({ ...device, changes: [{ name: "language", from: "ko", to: "en", actor: id(2), at: now }],
    payment: { chainId: 8283, asset: token, beneficiary: payer, contract: token, escrow: true, gasSponsor: payer, registrationExpiresAt: now } }, "operator"), true);
  assert.equal(validTerminalDevice({ ...device, readServer: "http://rpc.example" }, "terminal"), false);
});

// Testnet payment rehearsal without a transaction: the kiosk side and the simulated device run
// the payment session over the full wire, and the resulting authorization is checked against
// the deployed settlement contract with eth_call only.
//
// Inputs come from the operations tool, so the operator key never reaches this script:
//   opsctl attestation issue --merchant <kiosk> --payout <payout> --name "..." > attestation.json
//   opsctl anchor sign --device <device> > anchor.json
// Keys: nu54-device (simulated device) and nu54-kiosk (merchant key) keystores, passwords
// from the Keychain.
//
// Usage:
//   node --experimental-strip-types scripts/rehearse.ts --attestation attestation.json --anchor anchor.json [--amount 1000000]

import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { homedir } from "node:os";
import { join, resolve } from "node:path";
import { parseArgs } from "node:util";
import { bytesToHex, type Message } from "@nu54/protocol";
import { connect, SoftwareDevice } from "../src/index.ts";
import { openKeystore } from "../src/keystore.ts";
import { checkDeviceAuthorization, signMerchantOrder } from "../../../products/p04-merchant-kiosk/src/payment/signing.ts";
import { submit } from "../../../products/p04-merchant-kiosk/src/payment/submit.ts";
import { JsonRpcChain, type Hex } from "../../../products/p04-merchant-kiosk/src/chain/rpc.ts";
import { signDigest, addressOfPrivateKey } from "@nu54/protocol";

const { values: a } = parseArgs({
  options: {
    attestation: { type: "string" },
    anchor: { type: "string" },
    amount: { type: "string", default: "1000000" },
    submit: { type: "boolean", default: false },
    rpc: { type: "string", default: "https://api.test.stablenet.network/" },
    deployment: { type: "string", default: resolve(import.meta.dirname, "../../../products/p06-stablenet-contracts/deployments/8283.json") },
    keystores: { type: "string", default: join(homedir(), ".nu54", "keystores") },
  },
});
if (!a.attestation || !a.anchor) throw new Error("--attestation and --anchor are required (opsctl output)");

const dep = JSON.parse(readFileSync(a.deployment!, "utf8"));
const settlement: string = dep.contracts.PaymentSettlement.address;
const token: string = dep.contracts.TestUSDC.address;
const operator: string = dep.roles.operator;
const domain = { chainId: dep.chainId, verifyingContract: settlement };
const att = JSON.parse(readFileSync(a.attestation, "utf8"));
const anc = JSON.parse(readFileSync(a.anchor, "utf8"));

async function rpc(method: string, params: unknown[]) {
  const r = await fetch(a.rpc!, { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ jsonrpc: "2.0", id: 1, method, params }) });
  const j = (await r.json()) as { result?: unknown; error?: unknown };
  if (j.error) throw new Error(JSON.stringify(j.error));
  return j.result;
}
const chainNow = async () => Number(((await rpc("eth_getBlockByNumber", ["finalized", false])) as { timestamp: string }).timestamp);

const deviceKey = openKeystore(join(a.keystores!, "nu54-device"), "keychain:nu54-device");
const merchantKey = openKeystore(join(a.keystores!, "nu54-kiosk"), "keychain:nu54-kiosk");
const start = await chainNow();
const started = Date.now();
const device = new SoftwareDevice({
  key: deviceKey,
  operator,
  contract: settlement,
  chainId: dep.chainId,
  // A fresh 256-block per run so rehearsals never collide with earlier nonces.
  nonceStart: (BigInt(start) << 8n),
  now: () => start + Math.floor((Date.now() - started) / 1000),
});
const link = connect(device, 185);
const SID = bytesToHex(globalThis.crypto.getRandomValues(new Uint8Array(8)), false);

// Setup session: the operator-signed TimeAnchor from opsctl.
link.send({ v: 1, type: "session.open", sessionId: SID, mode: "setup", kioskNonce: bytesToHex(globalThis.crypto.getRandomValues(new Uint8Array(32))) } as Message);
const ack = link.send({ v: 1, type: "setup.timeAnchor", sessionId: SID, ...anc } as Message)[0];
if (!ack?.accepted) throw new Error(`anchor refused: ${JSON.stringify(ack)}`);

// Payment session, kiosk side.
const ok = link.send({ v: 1, type: "session.open", sessionId: SID, mode: "payment", kioskNonce: bytesToHex(globalThis.crypto.getRandomValues(new Uint8Array(32))) } as Message)[0];
link.send({ v: 1, type: "session.confirm", sessionId: SID, deviceNonce: ok.deviceNonce } as Message);
const refused = link.send({ v: 1, type: "payment.identify", sessionId: SID, attestation: att } as Message);
if (refused.length) throw new Error(`identify refused: ${JSON.stringify(refused[0])}`);
const auth = {
  chainId: String(dep.chainId), contract: settlement, merchant: att.merchant, payout: att.payout, token,
  amount: a.amount!, orderId: bytesToHex(globalThis.crypto.getRandomValues(new Uint8Array(32))), expiry: String((await chainNow()) + 90),
};
const merchantSignature = signMerchantOrder(domain, { orderId: auth.orderId, token, amount: auth.amount, payout: auth.payout, expiry: auth.expiry }, merchantKey);
const result = link.send({ v: 1, type: "payment.prepare", sessionId: SID, authorization: auth, merchantSignature } as Message)[0];
const requestedAt = Date.now();
if (result.outcome !== "approved") throw new Error(`device refused: ${JSON.stringify(result)}`);
const check = checkDeviceAuthorization(domain, auth, { signature: String(result.signature), nonce: String(result.nonce) }, String(ok.device));
if (!check.ok) throw new Error(`kiosk check failed: ${JSON.stringify(check)}`);

// eth_call settle from the kiosk address: no transaction is sent.
const tuple = `(${auth.chainId},${auth.contract},${auth.merchant},${auth.payout},${auth.token},${auth.amount},${auth.orderId},${result.nonce},${auth.expiry})`;
let simulation = "success";
try {
  execFileSync("cast", ["call", settlement, "settle((uint256,address,address,address,address,uint256,bytes32,uint256,uint64),bytes)", tuple, String(result.signature), "--from", att.merchant, "--rpc-url", a.rpc!], { stdio: "pipe" });
} catch (e) {
  simulation = String((e as { stderr?: Buffer }).stderr ?? e).trim().split("\n").pop()!;
}
let outcome: unknown = "not submitted (add --submit)";
if (a.submit && simulation === "success") {
  // The kiosk gas key; on the app it lives in the Android Keystore.
  const gasKey = openKeystore(join(a.keystores!, "nu54-kiosk"), "keychain:nu54-kiosk");
  const out = await submit(
    {
      chain: new JsonRpcChain(a.rpc!),
      signer: { address: addressOfPrivateKey(gasKey) as Hex, sign: (d) => signDigest(d, gasKey) },
      settlement: settlement as Hex,
      chainId: BigInt(dep.chainId),
      minGasBalance: 0n,
      fromBlock: BigInt(dep.contracts.PaymentSettlement.block),
    },
    { auth: { ...auth, nonce: String(result.nonce) }, signature: String(result.signature), device: String(ok.device), requestedAt },
  );
  outcome = { ...out, event: "event" in out ? { ...out.event, amount: String(out.event.amount), nonce: String(out.event.nonce), block: String(out.event.block) } : undefined };
  // P04-FR-16: tell the device the final result.
  link.send({ v: 1, type: "payment.outcome", sessionId: SID, orderId: auth.orderId, outcome: out.status === "Checking" ? "Checking" : out.status, ...("reason" in out ? { reason: out.reason } : {}) } as Message);
}
console.log(JSON.stringify({ device: ok.device, merchant: att.merchant, amount: auth.amount, nonce: result.nonce, orderId: auth.orderId, kioskCheck: check, settleSimulation: simulation, outcome, phone: device.phone[0] }, null, 2));
if (simulation !== "success") process.exit(1);

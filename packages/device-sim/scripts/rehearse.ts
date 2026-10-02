// Testnet payment rehearsal: the kiosk side runs a payment session with a device, checks the
// device's signature, simulates settle with eth_call and, with --submit, sends it and judges the
// result as payment protocol section 7 defines.
//
// The device is either the software device (--transport sim, the default) or the board over BLE
// (--transport ble, through products/p01-device-firmware/tools/bringup/pay_bridge.py). On the
// board the renter approves with SW1 (week-7 mapping); --press-sim presses it with the button
// simulator for development runs, the week-7 gate uses a real press.
//
// Inputs come from the operations tool, so the operator key never reaches this script:
//   opsctl attestation issue --merchant <kiosk> --payout <payout> --name "..." > attestation.json
//   opsctl anchor sign --device <device> > anchor.json
// Keys: nu54-kiosk (merchant and gas key) and, for the simulator, nu54-device; passwords from
// the Keychain.
//
// Usage:
//   node --experimental-strip-types scripts/rehearse.ts --attestation att.json --anchor anchor.json
//       [--transport sim|ble] [--press-sim] [--amount 1000000] [--submit]

import { execFile, execFileSync, spawn } from "node:child_process";
import { createInterface } from "node:readline";
import { readFileSync } from "node:fs";
import { homedir } from "node:os";
import { join, resolve } from "node:path";
import { parseArgs } from "node:util";
import { addressOfPrivateKey, bytesToHex, decodeMessage, encodeMessage, hexToBytes, signDigest, type Message } from "@nu54/protocol";
import { connect, SoftwareDevice } from "../src/index.ts";
import { openKeystore } from "../src/keystore.ts";
import { signMerchantOrder } from "../../../products/p04-merchant-kiosk/src/payment/signing.ts";
import type { MessageLink } from "../../../products/p04-merchant-kiosk/src/ble/framing.ts";
import { runPayment } from "../../../products/p04-merchant-kiosk/src/payment/session.ts";
import { submit } from "../../../products/p04-merchant-kiosk/src/payment/submit.ts";
import { JsonRpcChain, type Hex } from "../../../products/p04-merchant-kiosk/src/chain/rpc.ts";

const { values: a } = parseArgs({
  options: {
    attestation: { type: "string" },
    anchor: { type: "string" },
    amount: { type: "string", default: "1000000" },
    submit: { type: "boolean", default: false },
    transport: { type: "string", default: "sim" },
    "press-sim": { type: "boolean", default: false },
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
const BRINGUP = resolve(import.meta.dirname, "../../../products/p01-device-firmware/tools/bringup");

async function rpc(method: string, params: unknown[]) {
  const r = await fetch(a.rpc!, { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ jsonrpc: "2.0", id: 1, method, params }) });
  const j = (await r.json()) as { result?: unknown; error?: unknown };
  if (j.error) throw new Error(JSON.stringify(j.error));
  return j.result;
}
const chainNow = async () => Number(((await rpc("eth_getBlockByNumber", ["finalized", false])) as { timestamp: string }).timestamp);

function simLink(start: number): MessageLink {
  const started = Date.now();
  const device = new SoftwareDevice({
    key: openKeystore(join(a.keystores!, "nu54-device"), "keychain:nu54-device"),
    operator,
    contract: settlement,
    chainId: dep.chainId,
    nonceStart: BigInt(start) << 8n, // a fresh 256-block per run
    now: () => start + Math.floor((Date.now() - started) / 1000),
  });
  const link = connect(device, 185);
  return { send: async (m) => link.send(m), close: async () => {} };
}

async function bleLink(): Promise<MessageLink> {
  const bridge = spawn("uv", ["run", "pay_bridge.py"], { cwd: BRINGUP, stdio: ["pipe", "pipe", "inherit"] });
  const queue: Message[] = [];
  let wake: (() => void) | null = null;
  let connected: (v: unknown) => void;
  const ready = new Promise((r) => (connected = r));
  createInterface({ input: bridge.stdout }).on("line", (line) => {
    const ev = JSON.parse(line);
    if (ev.event === "connected") {
      console.error(`ble: connected to ${ev.name}, ATT MTU ${ev.mtu}`);
      connected(ev);
    } else if (ev.recv) {
      queue.push(decodeMessage(hexToBytes(ev.recv)));
      wake?.();
    } else if (ev.error) {
      console.error(`ble: ${ev.error}`);
    }
  });
  bridge.on("exit", (code) => code && console.error(`ble: bridge exited (${code})`));
  await ready;
  return {
    async send(m, want, waitMs) {
      queue.length = 0;
      bridge.stdin.write(JSON.stringify({ send: bytesToHex(encodeMessage(m), false) }) + "\n");
      const deadline = Date.now() + waitMs;
      while (queue.length < want && Date.now() < deadline) {
        await new Promise<void>((r) => {
          wake = r;
          setTimeout(r, Math.min(200, Math.max(0, deadline - Date.now())));
        });
      }
      return queue.splice(0);
    },
    async close() {
      bridge.stdin.write(JSON.stringify({ close: true }) + "\n");
    },
  };
}

const start = await chainNow();
const link = a.transport === "ble" ? await bleLink() : simLink(start);
const merchantKey = openKeystore(join(a.keystores!, "nu54-kiosk"), "keychain:nu54-kiosk");
if (a.transport === "ble") {
  console.error(a["press-sim"] ? "device: pressing SW1 with the button simulator once LED2 lights"
    : "device: when LED2 lights, press SW1 briefly (under 1 s) to approve, SW2 to reject; waiting up to 120 s");
}
// The kiosk's payment session (products/p04-merchant-kiosk/src/payment/session.ts), with the
// operator-signed TimeAnchor from opsctl (week-7 development fixed setup).
const session = await runPayment(link, {
  domain,
  attestation: att,
  anchor: anc,
  token,
  amount: BigInt(a.amount!),
  expiry: BigInt(await chainNow()) + 60n, // leaves 60 s for an anchor signed a little before
  signOrder: (o) => signMerchantOrder(domain, o, merchantKey),
  random: (n) => globalThis.crypto.getRandomValues(new Uint8Array(n)),
  waitMs: a.transport === "ble" ? 120_000 : 5000, // a human press on the board; the app waits 10 s (N10)
  onStep: (step) => {
    if (step === "waitingDevice" && a["press-sim"]) {
      setTimeout(() => execFile("uv", ["run", "button_sim.py", "--button", "BTN1", "--hold", "0.3"], { cwd: BRINGUP }), 2500);
    }
  },
});
if (session.status === "cancelled") throw new Error("no payment.result from the device within the wait (was SW1 pressed briefly while LED2 was on?)");
if (session.status !== "approved") throw new Error(`device refused: ${JSON.stringify(session)}`);
const { device, auth, signature, requestedAt } = session;
const result = { signature, nonce: auth.nonce };
const check = { ok: true, device }; // runPayment checked the signer

// eth_call settle from the kiosk address.
const tuple = `(${auth.chainId},${auth.contract},${auth.merchant},${auth.payout},${auth.token},${auth.amount},${auth.orderId},${auth.nonce},${auth.expiry})`;
let simulation = "success";
try {
  execFileSync("cast", ["call", settlement, "settle((uint256,address,address,address,address,uint256,bytes32,uint256,uint64),bytes)", tuple, String(result.signature), "--from", att.merchant, "--rpc-url", a.rpc!], { stdio: "pipe" });
} catch (e) {
  simulation = String((e as { stderr?: Buffer }).stderr ?? e).trim().split("\n").pop()!;
}

let outcome: unknown = "not submitted (add --submit)";
if (a.submit && simulation === "success") {
  const gasKey = merchantKey; // the kiosk's gas key; on the app it lives in the Android Keystore
  const out = await submit(
    {
      chain: new JsonRpcChain(a.rpc!),
      signer: { address: addressOfPrivateKey(gasKey) as Hex, sign: (d) => signDigest(d, gasKey) },
      settlement: settlement as Hex,
      chainId: BigInt(dep.chainId),
      minGasBalance: 0n,
      fromBlock: BigInt(dep.contracts.PaymentSettlement.block),
    },
    { auth, signature, device, requestedAt },
  );
  outcome = { ...out, event: "event" in out ? { ...out.event, amount: String(out.event.amount), nonce: String(out.event.nonce), block: String(out.event.block) } : undefined };
  // P04-FR-16: tell the device the final result.
  await link.send({ v: 1, type: "payment.outcome", sessionId: session.sessionId, orderId: auth.orderId, outcome: out.status, ...("reason" in out ? { reason: out.reason } : {}) } as Message, 0, 500);
}
link.close();
console.log(JSON.stringify({ transport: a.transport, device, merchant: att.merchant, amount: auth.amount, nonce: result.nonce, orderId: auth.orderId, kioskCheck: check, settleSimulation: simulation, outcome }, null, 2));
if (simulation !== "success") process.exit(1);

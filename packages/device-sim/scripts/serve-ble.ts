// The software payment device as a real BLE peripheral on this Mac, so a central on a phone
// (the kiosk app) can run whole payment sessions without the board.
//
// peripheral/SimPeripheral.swift owns GATT (advertising, RX writes, TX notifications) and passes
// raw fragments; this script does the rest with the shared protocol code: reassembly, envelope,
// CBOR and the device itself (src/device.ts). The renter's button is this terminal: the
// confirm.show the phone app would display is printed, and y approves, n rejects.
//
// Differences from the board: macOS picks the ATT MTU and gives no control over it, the link is
// unpaired (as the week-7 payment session is), and the device key is the nu54-device test key.
//
// Usage:
//   pnpm -s serve-ble [--approve ask|yes|no] [--name "NU54 Sim"]
// Keys: nu54-device (password from the Keychain).

import { spawn, execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, readFileSync } from "node:fs";
import { homedir, tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { createInterface } from "node:readline";
import { parseArgs } from "node:util";
import { bytesToHex, hexToBytes, type Message } from "@nu54/protocol";
import { DeviceEndpoint, SoftwareDevice } from "../src/index.ts";
import { openKeystore } from "../src/keystore.ts";

const { values: a } = parseArgs({
  options: {
    approve: { type: "string", default: "ask" },
    name: { type: "string", default: "NU54 Sim" },
    deployment: { type: "string", default: resolve(import.meta.dirname, "../../../products/p06-stablenet-contracts/deployments/8283.json") },
    keystores: { type: "string", default: join(homedir(), ".nu54", "keystores") },
  },
});
if (!["ask", "yes", "no"].includes(a.approve!)) throw new Error("--approve is ask, yes or no");

const dep = JSON.parse(readFileSync(a.deployment!, "utf8"));
const schema = JSON.parse(readFileSync(resolve(import.meta.dirname, "../../../docs/content/specifications/protocol/payment-protocol.schema.json"), "utf8"));
const gatt = schema.gatt as { service: string; characteristics: { rx: { uuid: string }; tx: { uuid: string } } };

/** Builds the Swift peripheral once per source version (the binary is cached in the temp dir). */
function peripheralBinary(): string {
  const src = resolve(import.meta.dirname, "../peripheral/SimPeripheral.swift");
  const tag = createHash("sha256").update(readFileSync(src)).digest("hex").slice(0, 12);
  const bin = join(tmpdir(), `nu54-sim-peripheral-${tag}`);
  if (!existsSync(bin)) {
    console.error("building the BLE peripheral (swiftc, once)...");
    execFileSync("swiftc", ["-O", src, "-o", bin], { stdio: "inherit" });
  }
  return bin;
}

// The terminal is the renter's button. Lines typed while nothing waits are ignored.
let waiting: ((ok: boolean) => void) | null = null;
const tty = createInterface({ input: process.stdin });
tty.on("line", (line) => {
  const k = line.trim().toLowerCase();
  if (waiting && (k === "y" || k === "n")) {
    const w = waiting;
    waiting = null;
    w(k === "y");
  }
});

/** Amount as the phone shows it: token units, truncated to two decimals (N31). tUSDC has 6. */
const shown = (amount: string) => {
  const v = BigInt(amount);
  return `${v / 1_000_000n}.${((v % 1_000_000n) / 10_000n).toString().padStart(2, "0")}`;
};

const device = new SoftwareDevice({
  key: openKeystore(join(a.keystores!, "nu54-device"), "keychain:nu54-device"),
  operator: dep.roles.operator,
  contract: dep.contracts.PaymentSettlement.address,
  chainId: dep.chainId,
  // Sequential nonces from a fresh 256-block per start, so runs never reuse a nonce.
  nonceStart: BigInt(Math.floor(Date.now() / 1000)) << 8n,
  approve: (show: Message) => {
    console.error(`\nphone: pay ${shown(String(show.amount))} to "${show.merchantName}" (payout ${show.payout})`);
    if (a.approve !== "ask") {
      console.error(`button: ${a.approve === "yes" ? "approve" : "reject"} (--approve ${a.approve})`);
      return a.approve === "yes";
    }
    console.error("button: type y to approve or n to reject, then Enter");
    return new Promise<boolean>((r) => (waiting = r));
  },
});
console.error(`device ${device.address} on chain ${dep.chainId}, settlement ${dep.contracts.PaymentSettlement.address}`);

const periph = spawn(peripheralBinary(), [a.name!, gatt.service, gatt.characteristics.rx.uuid, gatt.characteristics.tx.uuid], {
  stdio: ["pipe", "pipe", "inherit"],
});
const send = (fragment: Uint8Array) => periph.stdin.write(JSON.stringify({ tx: bytesToHex(fragment, false) }) + "\n");

let endpoint: DeviceEndpoint | null = null;
createInterface({ input: periph.stdout }).on("line", async (line) => {
  const ev = JSON.parse(line);
  if (ev.event === "advertising") {
    console.error(`ble: advertising as "${ev.name}"; connect from the kiosk app`);
  } else if (ev.event === "connected") {
    // A new link starts a new reassembler and sequence; the device keeps its anchor and nonces.
    endpoint = new DeviceEndpoint(device, ev.mtu);
    console.error(`ble: central ${ev.central} connected, ATT MTU ${ev.mtu}`);
  } else if (ev.event === "disconnected") {
    endpoint = null;
    if (waiting) waiting(false);
    waiting = null;
    device.handle({ v: 1, type: "session.cancel", sessionId: "0000000000000000" } as Message);
    console.error(`ble: central ${ev.central} disconnected`);
  } else if (ev.rx !== undefined) {
    if (!endpoint) return;
    const ep = endpoint;
    // receiveAsync reassembles synchronously, so fragment order holds while a press is pending.
    for (const f of await ep.receiveAsync(hexToBytes(ev.rx))) {
      if (ep === endpoint) send(f);
    }
  } else if (ev.error) {
    console.error(`ble: ${ev.error}`);
  }
});
periph.on("exit", (code) => {
  console.error(`ble: peripheral exited (${code})`);
  process.exit(code ?? 1);
});
process.on("SIGINT", () => {
  periph.stdin.write(JSON.stringify({ stop: true }) + "\n");
  setTimeout(() => process.exit(0), 300);
});

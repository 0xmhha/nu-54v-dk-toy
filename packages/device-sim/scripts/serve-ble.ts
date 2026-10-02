// The software payment device as a real BLE peripheral on this Mac, so a central on a phone
// (the kiosk app) can run whole payment sessions without the board.
//
// peripheral/SimPeripheral.swift owns GATT (advertising, RX writes, TX notifications) and passes
// raw fragments; this script does the rest with the shared protocol code: reassembly, envelope,
// CBOR and the device itself (src/device.ts). The renter's button is this terminal: the
// confirm.show the phone app would display is printed (and sent to a listening phone app), and y
// approves, n rejects.
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
import { bytesToHex, encodeMessage, FrameWriter, GATT, hexToBytes, type Message } from "@nu54/protocol";
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

const periph = spawn(peripheralBinary(), [a.name!, GATT.service, GATT.rx, GATT.tx], {
  stdio: ["pipe", "pipe", "inherit"],
});
const send = (central: string, fragment: Uint8Array) =>
  periph.stdin.write(JSON.stringify({ tx: bytesToHex(fragment, false), central }) + "\n");

// Each subscribed central has its own link (reassembler and sequence numbers). A central that
// writes is a session peer (the kiosk); one that only listens is taken as the renter's phone app
// and gets confirm.show and the forwarded payment.outcome. The board tells them apart by bonding;
// macOS gives a peripheral no bonding state, so this simulator goes by behaviour.
type Link = { endpoint: DeviceEndpoint; phone: FrameWriter; wrote: boolean };
const links = new Map<string, Link>();
device.onPhone = (m) => {
  const body = encodeMessage(m);
  for (const [id, link] of links) {
    if (!link.wrote) link.phone.write(body).forEach((f) => send(id, f));
  }
  console.error(`phone: ${m.type} sent to ${[...links.values()].filter((l) => !l.wrote).length} listening central(s)`);
};
createInterface({ input: periph.stdout }).on("line", async (line) => {
  const ev = JSON.parse(line);
  if (ev.event === "advertising") {
    console.error(`ble: advertising as "${ev.name}"; connect from the kiosk app`);
  } else if (ev.event === "connected") {
    // A new link starts a new reassembler and sequence; the device keeps its anchor and nonces.
    links.set(ev.central, { endpoint: new DeviceEndpoint(device, ev.mtu), phone: new FrameWriter(ev.mtu), wrote: false });
    console.error(`ble: central ${ev.central} connected, ATT MTU ${ev.mtu}`);
  } else if (ev.event === "disconnected") {
    const link = links.get(ev.central);
    links.delete(ev.central);
    if (link?.wrote) {
      // The session peer left: a pending press ends without a signature.
      if (waiting) waiting(false);
      waiting = null;
      device.handle({ v: 1, type: "session.cancel", sessionId: "0000000000000000" } as Message);
    }
    console.error(`ble: central ${ev.central} disconnected`);
  } else if (ev.rx !== undefined) {
    const link = links.get(ev.central);
    if (!link) return;
    link.wrote = true;
    // receiveAsync reassembles synchronously, so fragment order holds while a press is pending.
    const early = (fs: Uint8Array[]) => fs.forEach((f) => send(ev.central, f));
    for (const f of await link.endpoint.receiveAsync(hexToBytes(ev.rx), early)) {
      if (links.get(ev.central) === link) send(ev.central, f);
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

// The setup flow against the software device (P02 design 3): scan, Just Works bond, setup with
// the buttons, the reconnect code, the passkey bond, the wallet check, and the registered device
// on the next start. The fake platform models the phone's bond the way the device judges it: an
// UNPROVISIONED device takes any bond, a set-up one only the passkey bond.

import { bytesToHex, decodeMessage, digest, encodeMessage, signDigest, type Message } from "@nu54/protocol";
import { SoftwareDevice, type DeviceConfig } from "@nu54/device-sim";
import { drawPasskey, SetupFlow, type DeviceRecord, type SetupPlatform, type SetupState } from "../src/setup/flow.ts";
import { NETWORK } from "../src/setup/network.ts";
import deployment from "../../p06-stablenet-contracts/deployments/8283.json";

const webCrypto = (globalThis as unknown as { crypto: { getRandomValues<T extends Uint8Array>(a: T): T } }).crypto;

const BLE = "CA:F3:8A:BD:C3:E3";
const FOUND = { address: BLE, name: "NU54-HW-Wallet", rssi: -48 };

class FakePhone implements SetupPlatform {
  bond_: "none" | "justWorks" | "passkey" = "none";
  stored: DeviceRecord[] = [];
  /** The passkey setup.operator recorded on the device (the device keeps it; the app does not). */
  devicePasskey: string | null = null;
  /** Replaces the device's wallet.check signature (a device whose key does not match). */
  forgeCheck = false;
  sent: Message[] = [];

  constructor(public device: SoftwareDevice) {}

  linkBonded = () => this.bond_ !== "none" && (this.device.state === "UNPROVISIONED" || this.bond_ === "passkey");

  async scan(onFound: (d: typeof FOUND) => void) {
    onFound(FOUND);
  }
  async stopScan() {}
  /** What the renter types into the system pairing dialog when the app passes no code. */
  typed: string | null = null;
  async bond(_address: string, passkey: string) {
    const code = passkey || this.typed;
    if (this.device.state === "UNPROVISIONED") this.bond_ = "justWorks";
    else if (this.devicePasskey !== null && code === this.devicePasskey) this.bond_ = "passkey";
    else return false;
    return true;
  }
  async isBonded() {
    return this.bond_ !== "none";
  }
  async removeBond() {
    this.bond_ = "none";
    return true;
  }
  async connect() {
    if (this.bond_ === "none") throw new Error("not bonded");
    let handler: (b: Uint8Array) => void = () => {};
    this.toApp = (m) => handler(encodeMessage(m));
    this.device.onPhone = (m) => {
      if (m.type === "wallet.check.result" && this.forgeCheck && m.signature) {
        const other = new Uint8Array(32).fill(7);
        const d = digest({ chainId: NETWORK.chainId, verifyingContract: NETWORK.contract }, "WalletCheck", { device: this.device.address, challenge: String(this.lastChallenge) });
        m = { ...m, signature: bytesToHex(signDigest(d, other)) } as Message;
      }
      handler(encodeMessage(m));
    };
    return {
      send: async (body: Uint8Array) => {
        const m = decodeMessage(body);
        this.sent.push(m);
        if (m.type === "setup.operator") this.pendingPasskey = String(m.passkey).padStart(6, "0");
        if (m.type === "wallet.check") this.lastChallenge = String(m.challenge);
        const was = this.device.state;
        const replies = await this.device.handleBodyAsync(body, (b) => handler(b));
        if (was === "UNPROVISIONED" && this.device.state !== "UNPROVISIONED") this.devicePasskey = this.pendingPasskey;
        for (const r of replies) handler(r);
      },
      onBody: (h: (b: Uint8Array) => void) => {
        handler = h;
        return () => { handler = () => {}; };
      },
    };
  }
  /** A message from the device to the app outside any reply (pin.entry). */
  toApp: (m: Message) => void = () => {};
  private pendingPasskey: string | null = null;
  private lastChallenge: string | null = null;
  async disconnect() {
    this.device.linkClosed();
  }
  async random(n: number) {
    return webCrypto.getRandomValues(new Uint8Array(n));
  }
  async loadDevices() {
    return this.stored;
  }
  async saveDevices(list: DeviceRecord[]) {
    this.stored = [...list];
  }
  now() {
    return Date.now();
  }
  async sleep() {}
}

function newDevice(over: Partial<DeviceConfig> = {}, phone?: () => FakePhone) {
  return new SoftwareDevice({ enterPin: () => "1234", confirmSetup: () => true, approve: () => true, linkBonded: () => phone!().linkBonded(), ...over });
}

/** The renter writes the code down and types it into the pairing dialog, then starts the wallet check. */
async function noteAndVerify(flow: SetupFlow, phone: FakePhone) {
  const s = flow.current();
  phone.typed = s.kind === "passkey" ? s.passkey : null;
  await flow.passkeyNoted();
  await flow.startVerify();
}

function setup(over: Partial<DeviceConfig> = {}) {
  let phone: FakePhone;
  const device = newDevice(over, () => phone);
  phone = new FakePhone(device);
  const states: SetupState[] = [];
  const flow = new SetupFlow(phone, (s) => states.push(s));
  return { phone, device, flow, states, kinds: () => states.map((s) => s.kind) };
}

test("a new device: scan, setup on the buttons, the reconnect code, the passkey bond, the wallet check, home", async () => {
  const { phone, device, flow, states, kinds } = setup();
  await flow.start();
  expect(flow.current()).toEqual({ kind: "scan", found: [FOUND], scanning: false });
  await flow.choose(FOUND);
  const code = flow.current();
  expect(code.kind).toBe("passkey");
  if (code.kind !== "passkey") return;
  expect(code.passkey).toMatch(/^\d{6}$/);
  expect(code.wallet).toBe(device.address);
  expect(phone.bond_).toBe("justWorks");
  await noteAndVerify(flow, phone);
  expect(flow.current()).toEqual({ kind: "home", record: { address: BLE, name: "NU54-HW-Wallet", wallet: device.address } });
  expect(phone.bond_).toBe("passkey");
  expect(phone.stored).toEqual([{ address: BLE, name: "NU54-HW-Wallet", wallet: device.address }]);
  expect(kinds()).toEqual(["scan", "scan", "scan", "bonding", "connecting", "setupConfirm", "setupPin", "passkey", "rebonding", "verifyReady", "verify", "home"]);
  // The setup session carries the deployment's operator and contract, and is closed when stored.
  const op = phone.sent.find((m) => m.type === "setup.operator")!;
  expect([op.operator, op.contract, op.chainId]).toEqual([NETWORK.operator.toLowerCase(), NETWORK.contract.toLowerCase(), "8283"]); // CBOR bytes
  expect(phone.sent.filter((m) => m.type === "session.cancel")).toHaveLength(1);
  expect(states.some((s) => JSON.stringify(s).includes("1234"))).toBe(false); // the PIN never reaches the app
});

test("the next start reconnects to the registered device and goes home", async () => {
  const first = setup();
  await first.flow.start();
  await first.flow.choose(FOUND);
  await noteAndVerify(first.flow, first.phone);
  first.device.linkClosed();
  const states: SetupState[] = [];
  const again = new SetupFlow(first.phone, (s) => states.push(s));
  await again.start();
  expect(states.map((s) => s.kind)).toEqual(["reconnecting", "home"]);
});

test("the renter refuses the setup values on the device: nothing stored, the device stays without a key", async () => {
  const { device, flow, phone } = setup({ confirmSetup: () => false });
  await flow.start();
  await flow.choose(FOUND);
  expect(flow.current()).toEqual({ kind: "failed", message: "기기에서 거절했습니다" });
  expect(device.state).toBe("UNPROVISIONED");
  expect(phone.stored).toEqual([]);
});

test("the PIN is not entered in time: keygen TIMEOUT, nothing stored", async () => {
  const { device, flow } = setup({ enterPin: () => null });
  await flow.start();
  await flow.choose(FOUND);
  expect(flow.current()).toEqual({ kind: "failed", message: "시간 안에 기기 버튼을 누르지 않았습니다" });
  expect(device.state).toBe("UNPROVISIONED");
});

test("a wallet check signed by another key is refused and the device is not registered", async () => {
  const { flow, phone } = setup();
  await flow.start();
  await flow.choose(FOUND);
  phone.forgeCheck = true;
  await noteAndVerify(flow, phone);
  expect(flow.current()).toEqual({ kind: "failed", message: "기기 서명이 지갑 주소와 맞지 않습니다. 기기를 지우고 다시 설정하세요" });
  expect(phone.stored).toEqual([]);
});

test("the renter rejects the wallet check on the device", async () => {
  let approve = true;
  const { flow, phone } = setup({ approve: () => approve });
  await flow.start();
  await flow.choose(FOUND);
  approve = false;
  await noteAndVerify(flow, phone);
  expect(flow.current()).toEqual({ kind: "failed", message: "지갑을 확인하지 못했습니다: 기기에서 거절했습니다" });
  expect(phone.stored).toEqual([]);
});

test("a device set up before bonds on a new phone with its code, then only the wallet check runs", async () => {
  const { flow, phone, device } = setup();
  await flow.start();
  await flow.choose(FOUND);
  const code = flow.current();
  if (code.kind !== "passkey") throw new Error(code.kind);
  // A new phone: no bond, no record. The system pairing dialog takes the code the renter kept.
  const newPhone = new FakePhone(device);
  newPhone.devicePasskey = phone.devicePasskey;
  newPhone.bond = async (_a: string, _p: string) => {
    newPhone.bond_ = "passkey";
    return true;
  };
  phone.bond_ = "none";
  (device as unknown as { cfg: { linkBonded: () => boolean } }).cfg.linkBonded = () => newPhone.linkBonded();
  const states: SetupState[] = [];
  const other = new SetupFlow(newPhone, (s) => states.push(s));
  await other.start();
  await other.choose(FOUND);
  await other.startVerify();
  expect(states.map((s) => s.kind)).toEqual(["scan", "scan", "scan", "bonding", "connecting", "verifyReady", "verify", "home"]);
  expect(newPhone.stored[0].wallet).toBe(device.address);
  expect(newPhone.sent.some((m) => m.type === "setup.operator")).toBe(false);
});

test("a stale bond the phone still holds is removed before bonding a device picked from the scan", async () => {
  const { flow, phone } = setup();
  phone.bond_ = "passkey"; // left from before the device was wiped
  let removed = 0;
  const remove = phone.removeBond.bind(phone);
  phone.removeBond = async () => { removed++; return remove(); };
  await flow.start();
  await flow.choose(FOUND);
  expect(removed).toBe(1);
  expect(flow.current().kind).toBe("passkey");
});

test("a code mistyped in the pairing dialog brings the code screen back and starts nothing", async () => {
  const { flow, phone } = setup();
  await flow.start();
  await flow.choose(FOUND);
  const code = flow.current();
  if (code.kind !== "passkey") throw new Error(code.kind);
  phone.typed = code.passkey === "000000" ? "111111" : "000000";
  await flow.passkeyNoted();
  expect(flow.current()).toEqual({ ...code, retry: true });
  expect(phone.sent.some((m) => m.type === "wallet.check")).toBe(false);
  phone.typed = code.passkey; // the second try with the right code
  await flow.passkeyNoted();
  expect(flow.current().kind).toBe("verifyReady");
});

test("the wallet check waits for the renter after the passkey bond", async () => {
  const { flow, phone } = setup();
  await flow.start();
  await flow.choose(FOUND);
  const code = flow.current();
  if (code.kind !== "passkey") throw new Error(code.kind);
  phone.typed = code.passkey;
  await flow.passkeyNoted();
  expect(flow.current().kind).toBe("verifyReady");
  expect(phone.bond_).toBe("passkey");
  expect(phone.sent.some((m) => m.type === "wallet.check")).toBe(false);
  await flow.startVerify();
  expect(flow.current().kind).toBe("home");
});

test("the PIN screen follows the device's pin.entry as the renter presses the buttons", async () => {
  let phone: FakePhone;
  const seen: string[] = [];
  const device = newDevice({
    enterPin: () => {
      for (const [digits, position] of [["0000", 0], ["1000", 0], ["1000", 1], ["1900", 1], ["1900", 2]] as const) {
        phone.toApp({ v: 1, type: "pin.entry", sessionId: "0000000000000000", digits, position: String(position) } as Message);
      }
      return "1234";
    },
  }, () => phone);
  phone = new FakePhone(device);
  const flow = new SetupFlow(phone, (s) => { if (s.kind === "setupPin") seen.push(`${s.entry.digits}@${s.entry.position}`); });
  await flow.start();
  await flow.choose(FOUND);
  expect(seen.at(-1)).toBe("1900@2"); // the latest entry, whenever the screen comes up
  expect(flow.current().kind).toBe("passkey");
});

test("forgetting the device removes the bond and the record and goes back to the scan", async () => {
  const { flow, phone } = setup();
  await flow.start();
  await flow.choose(FOUND);
  await noteAndVerify(flow, phone);
  const home = flow.current();
  if (home.kind !== "home") throw new Error(home.kind);
  await flow.forget(home.record);
  expect(phone.bond_).toBe("none");
  expect(phone.stored).toEqual([]);
  expect(flow.current().kind).toBe("scan");
});

test("a registered device that was wiped (returned) is set up again", async () => {
  const { flow, phone } = setup();
  await flow.start();
  await flow.choose(FOUND);
  await noteAndVerify(flow, phone);
  const old = phone.stored[0].wallet;
  // The operator's device.reset wipes it; the phone's bond stays and is enough for an UNPROVISIONED device.
  const wiped = newDevice({}, () => phone);
  phone.device = wiped;
  await flow.start();
  expect(flow.current().kind).toBe("passkey");
  await noteAndVerify(flow, phone);
  const home = flow.current();
  expect(home.kind).toBe("home");
  if (home.kind === "home") expect(home.record.wallet).not.toBe(old);
});

test("the reconnect code is six uniform digits; draws past the last whole million are redrawn", async () => {
  const draws = [[0xff, 0xff, 0xff, 0xff], [0x00, 0x00, 0x00, 0x2a]];
  expect(await drawPasskey(async () => new Uint8Array(draws.shift()!))).toBe("000042");
});

test("the network values are the deployment record's", () => {
  const dep = deployment;
  expect(NETWORK.chainId).toBe(BigInt(dep.chainId));
  expect(NETWORK.operator).toBe(dep.roles.operator);
  expect(NETWORK.contract).toBe(dep.contracts.PaymentSettlement.address);
});

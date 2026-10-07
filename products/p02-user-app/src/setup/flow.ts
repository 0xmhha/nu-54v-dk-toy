// The renter app's device and wallet setup as a state machine (P02 design 3).
//
//   start ─ no device registered ─▶ scan ─ choose ─▶ bonding ─▶ connecting ─▶ device.info
//         └ a device registered ──▶ reconnecting ─▶ device.info
//   device.info: UNPROVISIONED ─▶ setupConfirm ─▶ setupPin ─▶ passkey ─▶ rebonding ─▶ verifyReady ─▶ verify ─▶ home
//                with a key    ─▶ verify (a bond on a new phone) or home (the registered wallet)
//
// The device makes its key with its TRNG and never lets it out (N11); the app keeps only the BLE
// address, the name and the wallet address. Setup sends the same setup session the operator tool
// does (payment-protocol.md 5) with a passkey the app draws and shows once as the reconnect code.
// The bond made in pairing mode before setup is Just Works; after setup the device takes passkey
// bonds only and opens pairing mode by itself, so the app bonds again with the code. The wallet
// check asks the device to sign WalletCheck{device, challenge} after its approve button and
// checks the signer is the wallet address keygen reported.

import { bytesToHex, digest, hexToBytes, recoverSigner, type Message } from "@nu54/protocol";
import { DeviceChannel, isType, type BodyLink } from "./channel.ts";
import { NETWORK } from "./network.ts";

/** A device in pairing mode found by the scan. */
export interface FoundDevice {
  address: string;
  name: string;
  rssi: number;
}

/** A registered device: all the app keeps (no key, PIN or passkey). */
export interface DeviceRecord {
  address: string;
  name: string;
  /** The device's wallet address (keygen ack, checked by wallet.check). */
  wallet: string;
}

export type SetupState =
  | { kind: "loading" }
  | { kind: "scan"; found: FoundDevice[]; scanning: boolean }
  | { kind: "bonding"; device: FoundDevice }
  | { kind: "connecting"; device: FoundDevice }
  | { kind: "reconnecting"; record: DeviceRecord }
  | { kind: "setupConfirm"; device: FoundDevice }
  /**
   * `until`: when the device's PIN entry times out (it starts at the approve button). `entry`:
   * the digits as entered on the buttons and the digit being entered (pin.entry, N28).
   */
  | { kind: "setupPin"; device: FoundDevice; until: number; entry: PinEntry }
  /** `retry`: the bond with the code failed (a mistyped code in the system pairing dialog). */
  | { kind: "passkey"; device: FoundDevice; wallet: string; passkey: string; retry?: boolean }
  /** The system pairing dialog asks the renter for the code; the app does not fill it in. */
  | { kind: "rebonding"; device: FoundDevice; wallet: string; passkey: string }
  /** Bonded with the code; the wallet check starts when the renter is ready to press the button. */
  | { kind: "verifyReady"; device: FoundDevice; wallet: string; passkey?: string }
  | { kind: "verify"; device: FoundDevice; wallet: string; until: number }
  | { kind: "home"; record: DeviceRecord }
  | { kind: "failed"; message: string; record?: DeviceRecord };

/** The PIN entry the device reports while the renter enters it on the buttons. */
export interface PinEntry {
  digits: string;
  /** 0..3: the digit being entered; 4: all four kept. */
  position: number;
}

/** pin.entry as the device sends it; null for anything malformed. */
export function pinEntryOf(m: Message): PinEntry | null {
  const digits = String(m.digits ?? "");
  const position = Number(m.position);
  return /^[0-9]{4}$/.test(digits) && Number.isInteger(position) && position >= 0 && position <= 4 ? { digits, position } : null;
}

/** What the flow needs from the platform (the Turbo Module in the app, the software device in tests). */
export interface SetupPlatform {
  scan(onFound: (d: FoundDevice) => void, seconds: number): Promise<void>;
  stopScan(): Promise<void>;
  bond(address: string, passkey: string): Promise<boolean>;
  isBonded(address: string): Promise<boolean>;
  removeBond(address: string): Promise<boolean>;
  /** Connects to a bonded device; the link carries one body per message. */
  connect(address: string): Promise<BodyLink>;
  disconnect(): Promise<void>;
  random(n: number): Promise<Uint8Array>;
  loadDevices(): Promise<DeviceRecord[]>;
  saveDevices(list: DeviceRecord[]): Promise<void>;
  now(): number;
  sleep(ms: number): Promise<void>;
}

/** How long each step waits; the device's own limits are shorter so its answer comes first. */
export const WAIT = {
  scanSeconds: 20,
  connectMs: 15_000,
  infoMs: 5_000,
  /** setup.operator until the approve button (the device has no limit of its own). */
  setupConfirmMs: 60_000,
  /** The device's PIN entry ends this long after the approve button (NU54_PIN_ENTRY_MS). */
  pinEntryMs: 45_000,
  /** keygen ack: after the device's PIN entry has ended with TIMEOUT at the latest. */
  setupPinMs: 60_000,
  /** wallet.check: the device gives up after 60 s with TIMEOUT. */
  walletCheckMs: 65_000,
  unbondMs: 5_000,
};

const ZERO_SESSION = "0000000000000000";
const REASON_TEXT: Record<string, string> = {
  USER_REJECTED: "기기에서 거절했습니다",
  TIMEOUT: "시간 안에 기기 버튼을 누르지 않았습니다",
  NOT_PERMITTED: "기기가 요청을 받지 않았습니다",
  PIN_LOCKED: "PIN이 잠긴 기기입니다. 반납 절차로만 풀 수 있습니다",
};

const reasonText = (m: Message) => REASON_TEXT[String(m.reason)] ?? String(m.reason ?? "알 수 없는 이유");

/** Six digits, uniform: 32 random bits below the largest multiple of 10^6, then mod 10^6. */
export async function drawPasskey(random: (n: number) => Promise<Uint8Array>): Promise<string> {
  for (;;) {
    const b = await random(4);
    const v = new DataView(b.buffer, b.byteOffset, 4).getUint32(0);
    if (v < 4_294_000_000) return String(v % 1_000_000).padStart(6, "0");
  }
}

export class SetupFlow {
  private state: SetupState = { kind: "loading" };
  private channel: DeviceChannel | null = null;
  private records: DeviceRecord[] = [];
  /** Bumped on every start so a step that finishes after the renter moved on changes nothing. */
  private run = 0;

  constructor(private readonly p: SetupPlatform, private readonly onState: (s: SetupState) => void) {}

  current(): SetupState {
    return this.state;
  }

  /** The registered devices (for the list on the home screen). */
  devices(): DeviceRecord[] {
    return this.records;
  }

  private set(s: SetupState): void {
    this.state = s;
    this.onState(s);
  }

  /** Registered device: reconnect to it; none: scan for one in pairing mode. */
  async start(): Promise<void> {
    const run = ++this.run;
    await this.closeLink();
    this.records = await this.p.loadDevices();
    if (run !== this.run) return;
    const record = this.records[0];
    if (record) await this.reconnect(record, run);
    else await this.scan(run);
  }

  async scan(run = ++this.run): Promise<void> {
    const found: FoundDevice[] = [];
    this.set({ kind: "scan", found: [], scanning: true });
    try {
      await this.p.scan((d) => {
        if (run !== this.run || found.some((f) => f.address === d.address)) return;
        found.push(d);
        this.set({ kind: "scan", found: [...found], scanning: true });
      }, WAIT.scanSeconds);
    } catch (e) {
      if (run === this.run) this.fail(`기기를 찾지 못했습니다: ${errText(e)}`);
      return;
    }
    if (run === this.run && this.state.kind === "scan") this.set({ kind: "scan", found: [...found], scanning: false });
  }

  /** The renter picked a device from the scan: bond, connect, then setup or the wallet check. */
  async choose(device: FoundDevice): Promise<void> {
    const run = ++this.run;
    await this.p.stopScan();
    this.set({ kind: "bonding", device });
    try {
      // A device picked from the scan is in pairing mode for a new bond; an old bond the phone
      // still holds (the device was wiped or bonded elsewhere since) would only fail encryption.
      if (await this.p.isBonded(device.address)) await this.unbond(device.address);
      if (!(await this.p.bond(device.address, ""))) throw new Error("본딩하지 못했습니다. 기기를 페어링 모드로 두고 다시 시도하세요");
      this.set({ kind: "connecting", device });
      const ch = await this.open(device.address);
      const info = await this.info(ch);
      if (run !== this.run) return;
      if (info.state === "UNPROVISIONED") await this.walletSetup(ch, device, run);
      else this.set({ kind: "verifyReady", device, wallet: String(info.device) }); // set up before: this phone checks it
    } catch (e) {
      if (run === this.run) this.fail(errText(e));
    }
  }

  /**
   * The renter wrote the reconnect code down: bond again with it. The renter types the code into
   * the system pairing dialog (the app does not fill it in), which shows it was written down; a
   * wrong code fails the bond and the code screen comes back. The wallet check waits for
   * startVerify, so it never runs while the renter is still on an earlier step.
   */
  async passkeyNoted(): Promise<void> {
    const s = this.state;
    if (s.kind !== "passkey") return;
    const run = ++this.run;
    this.set({ kind: "rebonding", device: s.device, wallet: s.wallet, passkey: s.passkey });
    try {
      await this.closeLink();
      if (await this.p.isBonded(s.device.address)) await this.unbond(s.device.address);
      // The device opened pairing mode with the new passkey when setup finished.
      if (!(await this.p.bond(s.device.address, ""))) {
        if (run === this.run) this.set({ ...s, retry: true });
        return;
      }
      await this.open(s.device.address);
      if (run === this.run) this.set({ kind: "verifyReady", device: s.device, wallet: s.wallet, passkey: s.passkey });
    } catch (e) {
      if (run === this.run) this.fail(errText(e));
    }
  }

  /** The renter is ready to press the approve button: the wallet check starts its 60 seconds. */
  async startVerify(): Promise<void> {
    const s = this.state;
    if (s.kind !== "verifyReady" || !this.channel) return;
    const run = ++this.run;
    try {
      await this.verify(this.channel, s.device, s.wallet, run);
    } catch (e) {
      if (run === this.run) this.fail(errText(e));
    }
  }

  /** Deletes a registered device: the phone's bond and the record. The device keeps its key. */
  async forget(record: DeviceRecord): Promise<void> {
    const run = ++this.run;
    await this.closeLink();
    const removed = await this.p.removeBond(record.address);
    this.records = this.records.filter((r) => r.address !== record.address);
    await this.p.saveDevices(this.records);
    if (run !== this.run) return;
    if (!removed) {
      this.fail("앱 목록에서는 지웠지만 폰의 블루투스 설정에 기기가 남아 있습니다. 설정에서 기기 등록을 해제하세요");
      return;
    }
    await this.scan(run);
  }

  /** Leaves the current step (the home screen takes the link from here). */
  async closeLink(): Promise<void> {
    this.channel?.close();
    this.channel = null;
    await this.p.disconnect();
  }

  /** Stops listening without disconnecting: the home screen's ConfirmLink reads the link next. */
  releaseLink(): void {
    this.channel?.close();
    this.channel = null;
  }

  // ------------------------------------------------------------------ steps

  private async reconnect(record: DeviceRecord, run: number): Promise<void> {
    this.set({ kind: "reconnecting", record });
    try {
      if (!(await this.p.isBonded(record.address))) {
        this.set({ kind: "failed", record, message: "폰에 기기 본딩이 없습니다. 기기를 지우고 다시 연결하세요" });
        return;
      }
      const ch = await this.open(record.address);
      const info = await this.info(ch);
      if (run !== this.run) return;
      if (info.state === "UNPROVISIONED") {
        // The device was returned and wiped: its old wallet is gone, set it up again.
        this.records = this.records.filter((r) => r.address !== record.address);
        await this.p.saveDevices(this.records);
        await this.walletSetup(ch, { address: record.address, name: record.name, rssi: 0 }, run);
        return;
      }
      if (String(info.device).toLowerCase() !== record.wallet.toLowerCase()) {
        this.set({ kind: "failed", record, message: "기기의 지갑 주소가 등록된 주소와 다릅니다. 기기를 지우고 다시 연결하세요" });
        return;
      }
      this.set({ kind: "home", record });
    } catch (e) {
      if (run === this.run) this.set({ kind: "failed", record, message: `기기에 다시 연결하지 못했습니다: ${errText(e)}` });
    }
  }

  private async walletSetup(ch: DeviceChannel, device: FoundDevice, run: number): Promise<void> {
    const sessionId = bytesToHex(await this.p.random(8), false);
    await ch.send({ v: 1, type: "session.open", sessionId, kioskNonce: bytesToHex(await this.p.random(32)), mode: "setup" } as Message);
    const ok = await ch.next((m) => m.type === "session.open.ok" || m.type === "error", WAIT.infoMs, "session.open");
    if (ok.type !== "session.open.ok") throw new Error(`설정을 시작하지 못했습니다 (${reasonText(ok)})`);
    const passkey = await drawPasskey((n) => this.p.random(n));
    // The device sends pin.entry from the approve button on; listen before
    // sending setup.operator, since it can come before the ack is read.
    let entry: PinEntry = { digits: "0000", position: 0 };
    let until = 0;
    const stop = ch.listen(isType("pin.entry"), (m) => {
      entry = pinEntryOf(m) ?? entry;
      if (run === this.run && this.state.kind === "setupPin") this.set({ kind: "setupPin", device, until, entry });
    });
    const cancel = () => ch.send({ v: 1, type: "session.cancel", sessionId } as Message).catch(() => {});
    try {
      this.set({ kind: "setupConfirm", device });
      await ch.send({
        v: 1, type: "setup.operator", sessionId, operator: NETWORK.operator, contract: NETWORK.contract,
        chainId: String(NETWORK.chainId), passkey: String(Number(passkey)),
      } as Message);
      const values = await ch.next(isType("setup.ack", "setup.operator"), WAIT.setupConfirmMs, "기기 버튼 승인");
      if (values.accepted !== true) throw new Error(reasonText(values));
      until = this.p.now() + WAIT.pinEntryMs;
      if (run === this.run) this.set({ kind: "setupPin", device, until, entry });
      const keygen = await ch.next(isType("setup.ack", "keygen"), WAIT.setupPinMs, "PIN 설정");
      if (keygen.accepted !== true) throw new Error(reasonText(keygen));
      await cancel(); // setup is stored; the session has nothing more to do
      if (run === this.run) this.set({ kind: "passkey", device, wallet: String(keygen.device), passkey });
    } catch (e) {
      await cancel(); // a setup that did not finish stores nothing
      throw e;
    } finally {
      stop();
    }
  }

  private async verify(ch: DeviceChannel, device: FoundDevice, wallet: string, run: number): Promise<void> {
    const challenge = bytesToHex(await this.p.random(32));
    this.set({ kind: "verify", device, wallet, until: this.p.now() + WAIT.walletCheckMs });
    await ch.send({ v: 1, type: "wallet.check", sessionId: ZERO_SESSION, challenge } as Message);
    const r = await ch.next(isType("wallet.check.result"), WAIT.walletCheckMs, "지갑 확인 버튼");
    if (r.accepted !== true) throw new Error(`지갑을 확인하지 못했습니다: ${reasonText(r)}`);
    const d = digest({ chainId: NETWORK.chainId, verifyingContract: NETWORK.contract }, "WalletCheck", { device: wallet, challenge });
    let signer = "";
    try {
      signer = recoverSigner(d, hexToBytes(String(r.signature)));
    } catch {
      // a signature that does not recover fails below
    }
    if (signer !== wallet.toLowerCase()) throw new Error("기기 서명이 지갑 주소와 맞지 않습니다. 기기를 지우고 다시 설정하세요");
    if (run !== this.run) return;
    const record: DeviceRecord = { address: device.address, name: device.name, wallet: wallet.toLowerCase() };
    this.records = [record, ...this.records.filter((x) => x.address !== record.address)];
    await this.p.saveDevices(this.records);
    this.set({ kind: "home", record });
  }

  // ------------------------------------------------------------------ helpers

  private async open(address: string): Promise<DeviceChannel> {
    this.channel?.close();
    const link = await withTimeout(this.p.connect(address), WAIT.connectMs, "연결");
    this.channel = new DeviceChannel(link);
    return this.channel;
  }

  private async info(ch: DeviceChannel): Promise<Message> {
    await ch.send({ v: 1, type: "device.info", sessionId: ZERO_SESSION } as Message);
    const m = await ch.next((x) => x.type === "device.info.ack" || x.type === "error", WAIT.infoMs, "device.info");
    if (m.type !== "device.info.ack") throw new Error(`기기가 상태를 알려주지 않았습니다 (${reasonText(m)})`);
    return m;
  }

  /** Removes the phone's bond and waits until the system has done it. */
  private async unbond(address: string): Promise<void> {
    if (!(await this.p.removeBond(address))) throw new Error("폰의 블루투스 설정에서 기기 등록을 해제한 뒤 다시 연결하세요");
    for (let waited = 0; await this.p.isBonded(address); waited += 200) {
      if (waited >= WAIT.unbondMs) throw new Error("폰이 기존 본딩을 지우지 못했습니다");
      await this.p.sleep(200);
    }
  }

  private fail(message: string): void {
    this.set({ kind: "failed", message });
  }
}

function errText(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

async function withTimeout<T>(p: Promise<T>, ms: number, what: string): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    return await Promise.race([p, new Promise<never>((_, reject) => { timer = setTimeout(() => reject(new Error(`${what} 시간 초과`)), ms); })]);
  } finally {
    clearTimeout(timer);
  }
}

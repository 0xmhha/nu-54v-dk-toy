import { readServer } from "./commerce-payment.ts";

/** Settings of one kiosk, kept and recorded by the operations server (`terminal.device`, `terminal.device-change`). */
export type TerminalDeviceName = "read-server" | "language" | "idle-seconds";
export type TerminalDeviceChange = { name: TerminalDeviceName; from: string; to: string; actor: string; at: number };
export type TerminalPaymentView = {
  chainId: 8283;
  asset: string;
  beneficiary: string;
  contract: string;
  /** The contract is OrderEscrow. Otherwise it is SettlementV2. */
  escrow: boolean;
  gasSponsor?: string;
  gasAccount?: string;
  registrationExpiresAt: number;
};
export type TerminalDevice = {
  /** Counts the recorded changes. */
  revision: number;
  language: "ko" | "en";
  idleSeconds: number;
  /** Absent: the kiosk reads the chain from the server in its store registration. */
  readServer?: string;
  /** Newest first. Only for an operator. */
  changes?: TerminalDeviceChange[];
  /** Only for an operator, and only where the server has the store's payment values. */
  payment?: TerminalPaymentView;
};

export const terminalIdleSeconds = [30, 60, 90, 120, 180] as const;
export const defaultTerminalDevice: TerminalDevice = { revision: 0, language: "ko", idleSeconds: 60 };

/** An empty read server means the one in the store registration. */
export function validTerminalDeviceValue(name: string, value: unknown): value is string {
  if (typeof value !== "string") return false;
  if (name === "read-server") return value === "" || readServer(value);
  if (name === "language") return value === "ko" || value === "en";
  return name === "idle-seconds" && terminalIdleSeconds.some(seconds => String(seconds) === value);
}

const reference = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const address = (value: unknown) => typeof value === "string" && /^0x[0-9a-f]{40}$/.test(value) && !/^0x0+$/.test(value);
const time = (value: unknown) => Number.isSafeInteger(value) && (value as number) > 0;
const only = (value: object, names: string[]) => Object.keys(value).every(key => names.includes(key));

export function validTerminalDevice(value: unknown, role: string): value is TerminalDevice {
  const v = value as TerminalDevice;
  if (!v || typeof v !== "object" || !only(v, ["revision", "language", "idleSeconds", "readServer", "changes", "payment"]) ||
      !Number.isSafeInteger(v.revision) || v.revision < 0 || !validTerminalDeviceValue("language", v.language) ||
      !validTerminalDeviceValue("idle-seconds", String(v.idleSeconds)) || typeof v.idleSeconds !== "number" ||
      (v.readServer !== undefined && !readServer(v.readServer)) ||
      (role !== "operator" && (v.changes !== undefined || v.payment !== undefined))) return false;
  const p = v.payment;
  return (v.changes === undefined || (Array.isArray(v.changes) && v.changes.length <= 10 && v.changes.every(c =>
      !!c && typeof c === "object" && only(c, ["name", "from", "to", "actor", "at"]) && validTerminalDeviceValue(c.name, c.from) &&
      validTerminalDeviceValue(c.name, c.to) && reference.test(c.actor) && time(c.at)))) &&
    (p === undefined || (!!p && typeof p === "object" &&
      only(p, ["chainId", "asset", "beneficiary", "contract", "escrow", "gasSponsor", "gasAccount", "registrationExpiresAt"]) &&
      p.chainId === 8283 && [p.asset, p.beneficiary, p.contract].every(address) && typeof p.escrow === "boolean" &&
      [p.gasSponsor, p.gasAccount].every(a => a === undefined || address(a)) && time(p.registrationExpiresAt)));
}

/** Test tokens that the operator's faucet sends to the customer's enrolled account (`faucet.claim`, `faucet.status`). */
export type FaucetClaim = {
  /** Base units. */
  amount: string;
  to: string;
  createdAt: number;
  /** "unknown": the transaction can no longer run and nothing proves whether it did. It is never sent again. */
  state: "queued" | "submitted" | "final" | "failed" | "unknown";
  txHash?: string;
};
export type FaucetToken = {
  token: string;
  symbol: string;
  decimals: number;
  /** Base units of one claim. */
  amount: string;
  /** Claims an account or a customer may make in any 24 hours, and how many are left now. */
  perDay: number;
  remaining: number;
  /** "open": a claim is on its way. "limit": a daily limit is reached. "empty": the faucet has too little to send. */
  state: "ready" | "open" | "limit" | "empty";
  /** For "limit": when the next claim may be possible, in Unix seconds. */
  availableAt?: number;
  /** This customer's latest claim of this token. */
  claim?: FaucetClaim;
};
export type FaucetStatus = {
  /** The account that would receive the tokens. Absent until the customer enrolls one with `payer.enroll`. */
  payer?: string;
  tokens: FaucetToken[];
};

const address = /^0x[0-9a-f]{40}$/;
const units = /^[1-9][0-9]{0,77}$/;
const only = (value: object, names: string[]) => Object.keys(value).every(key => names.includes(key));
const time = (value: unknown) => Number.isSafeInteger(value) && (value as number) > 0;

function validClaim(v: FaucetClaim): boolean {
  return !!v && typeof v === "object" && only(v, ["amount", "to", "createdAt", "state", "txHash"]) &&
    typeof v.amount === "string" && units.test(v.amount) && typeof v.to === "string" && address.test(v.to) && time(v.createdAt) &&
    (v.txHash === undefined ? ["queued", "failed", "unknown"].includes(v.state) :
      typeof v.txHash === "string" && /^0x[0-9a-f]{64}$/.test(v.txHash) && ["submitted", "final", "failed", "unknown"].includes(v.state));
}

export function validFaucetStatus(value: unknown): value is FaucetStatus {
  const v = value as FaucetStatus;
  return !!v && typeof v === "object" && only(v, ["payer", "tokens"]) &&
    (v.payer === undefined || (typeof v.payer === "string" && address.test(v.payer))) &&
    Array.isArray(v.tokens) && v.tokens.length > 0 && v.tokens.length <= 16 &&
    v.tokens.every(t => !!t && typeof t === "object" &&
      only(t, ["token", "symbol", "decimals", "amount", "perDay", "remaining", "state", "availableAt", "claim"]) &&
      typeof t.token === "string" && address.test(t.token) && !/^0x0+$/.test(t.token) &&
      typeof t.symbol === "string" && /^[A-Za-z0-9.-]{1,12}$/.test(t.symbol) &&
      Number.isInteger(t.decimals) && t.decimals >= 0 && t.decimals <= 24 &&
      typeof t.amount === "string" && units.test(t.amount) &&
      Number.isInteger(t.perDay) && t.perDay > 0 && Number.isInteger(t.remaining) && t.remaining >= 0 && t.remaining <= t.perDay &&
      ["ready", "open", "limit", "empty"].includes(t.state) && (t.state !== "ready" || t.remaining > 0) &&
      (t.availableAt === undefined || (t.state === "limit" && time(t.availableAt))) &&
      (t.claim === undefined || validClaim(t.claim))) &&
    new Set(v.tokens.map(t => t.token)).size === v.tokens.length;
}

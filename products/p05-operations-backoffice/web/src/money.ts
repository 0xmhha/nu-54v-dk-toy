/** A branch's payment token, as opsd names it. Absent means the dollar token. */
export type BranchToken = { name: string; symbol: string; label: string; decimals: number };

const dollar: BranchToken = { name: "NU54 Test USD", symbol: "tUSDC", label: "TestUSDC", decimals: 6 };

/** An exact amount: "12,500원" for won, otherwise at least two decimals and the token's label. */
export function tokenMoney(units: string, token: BranchToken = dollar) {
  const value = BigInt(units);
  const absolute = value < 0n ? -value : value;
  const scale = 10n ** BigInt(token.decimals);
  const sign = value < 0n ? "-" : "";
  const fraction = token.decimals
    ? (absolute % scale).toString().padStart(token.decimals, "0").replace(/0+$/, "")
    : "";
  const whole = (absolute / scale).toString();
  if (token.symbol !== "tKRW")
    return `${sign}${whole}.${fraction.padEnd(2, "0")} ${token.label}`;
  const grouped = whole.replace(/\B(?=(\d{3})+$)/g, ",");
  return `${sign}${grouped}${fraction ? `.${fraction.padEnd(2, "0")}` : ""}원`;
}

const named = new Map<string, BranchToken>();
/** Remembers the tokens of the branches this account may read. */
export function rememberTokens(rows: { asset?: string; token?: BranchToken }[]) {
  for (const { asset, token } of rows)
    if (asset && token) named.set(asset.toLowerCase(), token);
}
export const namedToken = (asset?: string) =>
  asset ? named.get(asset.toLowerCase()) : undefined;

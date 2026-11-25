/** A branch's payment token, as opsd names it. Absent means the dollar token. */
export type BranchToken = { name: string; symbol: string; label: string; decimals: number };

const dollar: BranchToken = { name: "NU54 Test USD", symbol: "tUSDC", label: "TestUSDC", decimals: 6 };

/** Dollars and cents. The rest is cut off, never rounded. */
export function dollarText(units: string, decimals: number) {
  const value = BigInt(units);
  const absolute = value < 0n ? -value : value;
  const cents = (absolute * 100n) / 10n ** BigInt(decimals);
  const sign = value < 0n && cents ? "-" : "";
  return `${sign}${cents / 100n}.${(cents % 100n).toString().padStart(2, "0")}`;
}

/** An amount as the pages show it: exact "12,500원" for won, dollars with two decimals for any other token. */
export function tokenMoney(
  units: string,
  token: Pick<BranchToken, "symbol" | "label" | "decimals"> = dollar
) {
  if (token.symbol !== "tKRW")
    return `${dollarText(units, token.decimals)} ${token.label}`;
  const value = BigInt(units);
  const absolute = value < 0n ? -value : value;
  const scale = 10n ** BigInt(token.decimals);
  const sign = value < 0n ? "-" : "";
  const fraction = token.decimals
    ? (absolute % scale).toString().padStart(token.decimals, "0").replace(/0+$/, "")
    : "";
  const grouped = (absolute / scale).toString().replace(/\B(?=(\d{3})+$)/g, ",");
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

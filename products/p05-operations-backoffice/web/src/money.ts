/** A branch's payment token, as opsd names it. Absent means the dollar token. */
export type BranchToken = { name: string; symbol: string; label: string; decimals: number };

/** What opsd says about the amounts of a reply. Absent means a dollar branch. */
export type Branded = { token?: BranchToken; tokenAsset?: string };
/** A token nobody confirmed, or an asset that is not the branch's token: amounts stay in base units. */
export const baseUnits: BranchToken = { name: "", symbol: "", label: "", decimals: 0 };

/** The token of one amount of a reply. `asset` is what the reply says the amount is in, when it says so. */
export const replyToken = (reply: Branded | undefined, asset?: string): BranchToken | undefined =>
  !reply?.token
    ? undefined
    : asset && reply.tokenAsset !== asset.toLowerCase()
      ? baseUnits
      : reply.token;

/** A branch token that is won: the test won, or StableNet's own coin. */
export const isWon = (token: Pick<BranchToken, "symbol">) => token.symbol === "tKRW" || token.symbol === "WKRC";

const dollar: BranchToken = { name: "NU54 Test USD", symbol: "tUSDC", label: "TestUSDC", decimals: 6 };

/** Dollars and cents. The rest is cut off, never rounded. */
export function dollarText(units: string, decimals: number) {
  const value = BigInt(units);
  const absolute = value < 0n ? -value : value;
  const cents = (absolute * 100n) / 10n ** BigInt(decimals);
  const sign = value < 0n && cents ? "-" : "";
  return `${sign}${cents / 100n}.${(cents % 100n).toString().padStart(2, "0")}`;
}

/** An amount as the pages show it: "12,500원" for won, dollars with two decimals for any other token. Never rounded up. */
export function tokenMoney(
  units: string,
  token: Pick<BranchToken, "symbol" | "label" | "decimals"> = dollar
) {
  if (token.decimals === 0)
    return `${BigInt(units).toLocaleString("ko-KR")} 기본 단위`;
  if (!isWon(token))
    return `${dollarText(units, token.decimals)} ${token.label}`;
  const value = BigInt(units);
  const absolute = value < 0n ? -value : value;
  const scale = 10n ** BigInt(token.decimals);
  const sign = value < 0n && absolute * 100n >= scale ? "-" : "";
  const fraction = (absolute % scale).toString().padStart(token.decimals, "0").slice(0, 2).replace(/0+$/, "");
  const grouped = (absolute / scale).toString().replace(/\B(?=(\d{3})+$)/g, ",");
  return `${sign}${grouped}${fraction ? `.${fraction.padEnd(2, "0")}` : ""}원`;
}

const named = new Map<string, BranchToken>();
/** Remembers which asset a confirmed branch token is. Only that asset is ever shown as the token. */
export function rememberTokens(rows: { asset?: string; token?: BranchToken }[]) {
  for (const { asset, token } of rows)
    if (asset && token && token.decimals > 0) named.set(asset.toLowerCase(), token);
}
export const namedToken = (asset?: string) =>
  asset ? named.get(asset.toLowerCase()) : undefined;

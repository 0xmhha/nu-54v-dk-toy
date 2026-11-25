import { isWon, tokenMoney, type BranchToken } from "./money.ts";

/** The branch asset as the catalog names it. `token` is present only for a branch that names its own. */
export type CatalogAsset = { symbol: string; decimals: number; token?: BranchToken };
export type PriceRow = { price: string; tax: string; discount: string };

export const assetToken = (asset: CatalogAsset) =>
  asset.token ?? { symbol: asset.symbol, label: asset.symbol, decimals: asset.decimals };

/** Every digit, for input fields: what is shown here is saved back. */
export function humanAmount(units: string, decimals: number) {
  const padded = units.padStart(decimals + 1, "0");
  if (!decimals) return padded;
  return `${padded.slice(0, -decimals)}.${padded
    .slice(-decimals)
    .replace(/0+$/, "")
    .padEnd(2, "0")}`;
}

const pattern = (decimals: number) =>
  new RegExp(`^(0|[1-9][0-9]{0,77})(\\.[0-9]{1,${decimals}})?$`);
function units(amount: string, decimals: number): bigint {
  const [whole, fraction = ""] = amount.split(".");
  return BigInt(whole + fraction.padEnd(decimals, "0"));
}
/** Base units of a row, or undefined while an amount is not one. */
export function rowTotal(row: PriceRow, asset: CatalogAsset): bigint | undefined {
  const amounts = [row.price, row.tax || "0", row.discount || "0"];
  if (!amounts.every((amount) => pattern(asset.decimals).test(amount))) return undefined;
  const [price, tax, discount] = amounts.map((amount) => units(amount, asset.decimals));
  return price + tax - discount;
}
export const shownTotal = (row: PriceRow, asset: CatalogAsset) => {
  const total = rowTotal(row, asset);
  return total === undefined ? row.price : tokenMoney(total.toString(), assetToken(asset));
};

/** Decimals an operator may type: won is whole, dollars have cents, and the screens show no more. */
const typed = (asset: CatalogAsset) =>
  isWon(assetToken(asset)) ? 0 : Math.min(2, asset.decimals);

/** Why these rows cannot be published. The server checks the same. */
export function priceProblem(rows: PriceRow[], asset: CatalogAsset): string | undefined {
  const won = isWon(assetToken(asset));
  const unit = 10n ** BigInt(asset.decimals - typed(asset));
  const whole = (amount: string) => units(amount, asset.decimals) % unit === 0n;
  for (const row of rows) {
    const total = rowTotal(row, asset);
    if (total !== undefined && ![row.price, row.tax || "0", row.discount || "0"].every(whole))
      return won
        ? "원화 가격과 세금·할인은 원 단위로 입력하세요."
        : "가격과 세금·할인은 소수점 둘째 자리까지 입력하세요.";
    if (total === undefined || total <= 0n)
      return won
        ? "원화 가격과 세금·할인은 원 단위로 입력하고, 최종 가격은 0보다 커야 합니다."
        : "가격과 세금·할인은 소수점 둘째 자리까지 입력하고, 최종 가격은 0보다 커야 합니다.";
  }
  return undefined;
}

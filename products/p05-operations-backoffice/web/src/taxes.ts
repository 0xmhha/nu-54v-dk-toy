import type { CommerceMenuMetadata } from "@nu54/protocol";
import { isWon } from "./money.ts";
import { assetToken, type CatalogAsset } from "./prices.ts";

export type TaxKind = NonNullable<CommerceMenuMetadata["tax"]>;

/** The tax inside an amount of base units: for 10%, what is left once the supply value (10/11) is rounded half up to the whole won or the cent. The apps count the same. */
export function includedTax(total: bigint, kind: TaxKind | undefined, asset: CatalogAsset) {
  if (kind !== "vat10") return 0n;
  const unit = 10n ** BigInt(isWon(assetToken(asset)) ? asset.decimals : Math.max(0, asset.decimals - 2));
  return total - (total * 20n + 11n * unit) / (22n * unit) * unit;
}

import { useId } from "react";
import type { CommerceMenuMetadata } from "@nu54/protocol";
import { tokenMoney } from "./money";
import { assetToken, rowTotal, type CatalogAsset, type PriceRow } from "./prices";
import { includedTax, type TaxKind } from "./taxes";

export type { TaxKind };


const zero = (amount: string) => /^0*(\.0*)?$/.test(amount);
/** An amount typed in the page's own digits, as money. */
const typed = (amount: string, asset: CatalogAsset) => {
  const units = rowTotal({ price: amount, tax: "", discount: "" }, asset);
  return units === undefined ? amount : tokenMoney(units.toString(), assetToken(asset));
};

/** The kind of tax of one product. Choosing one makes the price include it; the old added amount stays only until then. */
export function TaxField({ row, metadata, asset, change }: {
  row: PriceRow;
  metadata?: CommerceMenuMetadata;
  asset: CatalogAsset;
  change: (metadata: CommerceMenuMetadata, tax: string) => void;
}) {
  const name = useId();
  const kind = metadata?.tax;
  const added = !kind && !zero(row.tax) ? row.tax : "";
  const total = rowTotal(row, asset);
  const money = (units: bigint) => tokenMoney(units.toString(), assetToken(asset));
  const pick = (next?: TaxKind) => {
    const { tax: _, taxRefund: __, ...rest } = metadata ?? {};
    change(next ? { ...rest, tax: next, ...(metadata?.taxRefund ? { taxRefund: true } : {}) } : rest, next ? "0" : added || "0");
  };
  return (
    <fieldset>
      <legend>세금</legend>
      <label><input type="radio" name={name} checked={kind === "vat10"} onChange={() => pick("vat10")} />부가세 10% (가격에 포함)</label>
      <label><input type="radio" name={name} checked={kind === "exempt"} onChange={() => pick("exempt")} />면세</label>
      {added && <label><input type="radio" name={name} checked={!kind} onChange={() => pick(undefined)} />세금 {typed(added, asset)}을 가격에 더해 받기 (지금 방식)</label>}
      <label>
        <input type="checkbox" disabled={!kind} checked={!!metadata?.taxRefund}
          onChange={(event) => {
            const { taxRefund: _, ...rest } = metadata ?? {};
            change(event.target.checked ? { ...rest, taxRefund: true } : rest, row.tax);
          }} />
        외국인 세금 환급 대상 상품
      </label>
      <p>포장해 가져가는 물건만 해당할 수 있습니다. 시험 단계에서는 표시만 하고 금액에서 빼지 않습니다.</p>
      {kind && total !== undefined && total > 0n && (
        <dl className="totals" aria-label="손님이 낼 금액">
          <dt>정가(세금 포함)</dt><dd className="amount">{money(total)}</dd>
          <dt>공급가액</dt><dd className="amount">{money(total - includedTax(total, kind, asset))}</dd>
          <dt>{kind === "vat10" ? "세금 10%" : "세금"}</dt><dd className="amount">{money(includedTax(total, kind, asset))}</dd>
        </dl>
      )}
      {kind && <p>세금은 주문의 품목 줄마다 계산합니다. 공급가액의 원 단위 아래는 버리고, 남은 금액이 세금입니다. 옵션으로 더하는 금액에도 같은 세율이 들어 있습니다.</p>}
    </fieldset>
  );
}

type Priced = PriceRow & { sku: string; label: string; metadata?: CommerceMenuMetadata };

/** What publishing changes in the tax of each product, and in what a customer pays. */
export function TaxReview({ before, after, asset }: { before: Priced[]; after: Priced[]; asset: CatalogAsset }) {
  const money = (units?: bigint) => (units === undefined ? "" : tokenMoney(units.toString(), assetToken(asset)));
  const kindName = (row: Priced) => row.metadata?.tax === "vat10" ? "부가세 10% 포함" : row.metadata?.tax === "exempt" ? "면세"
    : zero(row.tax) ? "세금 표시 없음" : `세금 ${typed(row.tax, asset)} 별도`;
  const changes = after.flatMap((row) => {
    const earlier = before.find(({ sku }) => sku === row.sku);
    if (!earlier || (kindName(earlier) === kindName(row) && !!earlier.metadata?.taxRefund === !!row.metadata?.taxRefund)) return [];
    const was = rowTotal(earlier, asset), now = rowTotal(row, asset);
    return [{ row, earlier, was, now }];
  });
  if (!changes.length) return null;
  return (
    <div>
      <h3>세금에서 바뀌는 것</h3>
      <ul aria-label="세금에서 바뀌는 것">
        {changes.map(({ row, earlier, was, now }) => (
          <li key={row.sku}>
            {row.label} · {kindName(earlier)} → {kindName(row)}{row.metadata?.taxRefund ? " · 환급 대상 표시" : ""}
            {was !== now ? ` · 손님이 낼 금액 ${money(was)} → ${money(now)}` : " · 정가 그대로"}
            {was !== undefined && now !== undefined && now < was && <strong className="problem"> 낼 금액이 줄어듭니다. 지금처럼 받으려면 가격을 {money(was)}으로 적어 주세요.</strong>}
          </li>
        ))}
      </ul>
      <p>이미 결제한 주문의 세금은 바뀌지 않습니다.</p>
    </div>
  );
}

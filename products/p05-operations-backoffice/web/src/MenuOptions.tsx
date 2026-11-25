import { useEffect, useRef, useState } from "react";
import { allergenNames, MenuDeclarations } from "./MenuDeclarations";
import { commerceAllergens, type CommerceMenuMetadata } from "@nu54/protocol";
import { assetToken, type CatalogAsset } from "./prices";
import { isWon } from "./money";
import {
  combinations,
  emptyGroup,
  emptyValue,
  groupLimit,
  lineCount,
  priceRange,
  productProblems,
  rowLimit,
  valueLimit,
  type OptionGroup,
  type OptionValue,
  type Product,
} from "./optionGroups";

const unit = (asset: CatalogAsset) => (isWon(assetToken(asset)) ? "원" : asset.symbol);
const allergenText = (codes?: CommerceMenuMetadata["allergens"]) =>
  codes === undefined ? "적지 않음" : codes.map((code) => allergenNames[commerceAllergens.indexOf(code)]).join(", ") || "없음";
const replaced = <T,>(list: T[], index: number, next: T) => list.map((item, i) => (i === index ? next : item));
const without = <T,>(list: T[], index: number) => list.filter((_, i) => i !== index);

export type PlainProduct = { sku: string; label: string; price: string; lines: number };

export function ProductList({ products, plain, asset, lines, edit, add, show }: {
  products: Product[];
  plain: PlainProduct[];
  asset: CatalogAsset;
  lines: number;
  edit: (index: number) => void;
  add: () => void;
  show: (sku: string) => void;
}) {
  return (
    <section aria-label="메뉴">
      <div className="spread">
        <h2>메뉴 {products.length + plain.length}개 · 게시 줄 {lines} / {rowLimit}</h2>
        <button type="button" onClick={add}>상품 추가</button>
      </div>
      <ul className="live">
        {products.map((product, index) => (
          <li key={index} className="spread">
            <span><strong>{product.label}</strong><br />
              {product.groups.map(({ ko, values }) => `${ko} ${values.length}`).join(" · ")}</span>
            <span className="amount">{lineCount(product)}줄 · {priceRange(product, asset)}</span>
            <button type="button" className="quiet" aria-label={`${product.label} 고치기`} onClick={() => edit(index)}>고치기</button>
          </li>
        ))}
        {plain.map(({ sku, label, price, lines }) => (
          <li key={sku} className="spread">
            <span><strong>{label}</strong><br />{lines > 1 ? `옵션 목록 ${lines}개` : "옵션 묶음 없음"}</span>
            <span className="amount">{lines}줄 · {price}</span>
            <button type="button" className="quiet" aria-label={`${label} 고치기`} onClick={() => show(sku)}>고치기</button>
          </li>
        ))}
      </ul>
      <p>고친 것은 게시할 때 한꺼번에 적용됩니다.</p>
    </section>
  );
}

export function ProductEditor({ product, asset, taken, otherLines, fresh, save, remove, cancel }: {
  product: Product;
  asset: CatalogAsset;
  /** Codes of the other products. */
  taken: string[];
  /** Rows of the rest of the menu. */
  otherLines: number;
  fresh: boolean;
  save: (product: Product) => void;
  remove: () => void;
  cancel: () => void;
}) {
  const [draft, setDraft] = useState(product);
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => heading.current?.focus(), []);
  const change = (part: Partial<Product>) => setDraft((old) => ({ ...old, ...part }));
  const setGroup = (index: number, group: OptionGroup) => change({ groups: replaced(draft.groups, index, group) });
  const problems = productProblems(draft, asset);
  const count = lineCount(draft);
  const total = otherLines + count;
  const used = taken.includes(draft.sku);
  const blocked = total > rowLimit || used || !!problems.product || problems.groups.some(Boolean);
  const move = (index: number, by: number) => {
    const groups = [...draft.groups];
    [groups[index], groups[index + by]] = [groups[index + by], groups[index]];
    change({ groups });
  };
  return (
    <form
      aria-label="메뉴 고치기"
      onSubmit={(event) => {
        event.preventDefault();
        if (!blocked && !problems.incomplete) save(draft);
      }}
    >
      <section className="editor">
        <div>
          <h2 ref={heading} tabIndex={-1}>{draft.label || "새 상품"}</h2>
          <label>
            상품 코드
            <input required readOnly={!fresh} pattern="[a-z0-9][a-z0-9_\-]{0,31}" value={draft.sku} onChange={(event) => change({ sku: event.target.value })} />
          </label>
          {used && <p role="alert" className="problem">이미 쓰는 상품 코드입니다.</p>}
          <label>
            상품 이름
            <input required maxLength={120} value={draft.label} onChange={(event) => change({ label: event.target.value })} />
          </label>
          {([["price", "가격"], ["tax", "세금"], ["discount", "할인"]] as const).map(([key, name]) => (
            <label key={key}>
              {name}({unit(asset)}){key === "price" && " · 옵션을 더하기 전"}
              <input required={key === "price"} inputMode="decimal" value={draft[key]} onChange={(event) => change({ [key]: event.target.value })} />
            </label>
          ))}
          {problems.product && <p role="alert" className="problem">{problems.product}</p>}
          <p>세금이 있는 상품에는 더하는 금액을 적을 수 없습니다.</p>
          <p>알레르기 성분을 적지 않은 상품에는 값의 성분도 적을 수 없습니다. 들어 있지 않으면 성분을 하나도 고르지 않고 게시합니다.</p>
          <MenuDeclarations product metadata={draft.metadata} change={(metadata) => change({ metadata })} />
        </div>
        <div>
          <div className="spread">
            <h2>옵션 묶음 {draft.groups.length}/{groupLimit}</h2>
            <button type="button" disabled={draft.groups.length >= groupLimit} onClick={() => change({ groups: [...draft.groups, emptyGroup()] })}>묶음 추가</button>
          </div>
          {!draft.groups.length && <p>묶음이 없으면 옵션 묶음 없는 상품으로 저장합니다.</p>}
          {draft.groups.map((group, index) => (
            <fieldset key={group.key}>
              <legend>묶음 {index + 1}</legend>
              <div>
                <button type="button" className="quiet" aria-label={`묶음 ${index + 1} 위로`} disabled={!index} onClick={() => move(index, -1)}>위로</button>
                <button type="button" className="quiet" aria-label={`묶음 ${index + 1} 아래로`} disabled={index === draft.groups.length - 1} onClick={() => move(index, 1)}>아래로</button>
                <button type="button" className="quiet" aria-label={`묶음 ${index + 1} 지우기`} onClick={() => change({ groups: without(draft.groups, index) })}>지우기</button>
              </div>
              <div className="grid">
                <label>묶음 이름<input required value={group.ko} onChange={(event) => setGroup(index, { ...group, ko: event.target.value })} /></label>
                <label>영어 이름(선택)<input value={group.en} onChange={(event) => setGroup(index, { ...group, en: event.target.value })} /></label>
              </div>
              {group.values.map((value, at) => (
                <ValueFields key={value.key} value={value} number={at + 1} unit={unit(asset)} removable={group.values.length > 2}
                  change={(next) => setGroup(index, { ...group, values: replaced(group.values, at, next) })}
                  remove={() => setGroup(index, { ...group, values: without(group.values, at) })} />
              ))}
              {problems.groups[index] && <p role="alert" className="problem">{problems.groups[index]}</p>}
              <button type="button" className="quiet" disabled={group.values.length >= valueLimit}
                onClick={() => setGroup(index, { ...group, values: [...group.values, emptyValue()] })}>값 추가 ({group.values.length}/{valueLimit})</button>
            </fieldset>
          ))}
        </div>
      </section>
      <section className="spread" aria-label="조합과 저장">
        {total > rowLimit ? (
          <p role="alert" className="problem"><strong>게시할 수 없음 · 매장 전체 {total} / {rowLimit}줄</strong><br />
            {total - rowLimit}줄을 줄여야 합니다. 값이나 묶음을 줄이거나 다른 상품을 빼 주세요.</p>
        ) : (
          <p role="status"><strong>조합 {count}줄{priceRange(draft, asset) && ` · ${priceRange(draft, asset)}`}</strong><br />매장 전체 {total} / {rowLimit}줄</p>
        )}
        <div>
          {!fresh && <button type="button" className="quiet" onClick={remove}>이 상품 빼기</button>}
          <button type="button" className="quiet" onClick={cancel}>되돌리기</button>
          <button disabled={blocked}>이 상품 저장</button>
        </div>
      </section>
    </form>
  );
}

function ValueFields({ value, number, unit, removable, change, remove }: {
  value: OptionValue;
  number: number;
  unit: string;
  removable: boolean;
  change: (value: OptionValue) => void;
  remove: () => void;
}) {
  return (
    <div className="value" role="group" aria-label={`값 ${number}`}>
      <label>값 이름<input required value={value.ko} onChange={(event) => change({ ...value, ko: event.target.value })} /></label>
      <label>영어(선택)<input value={value.en} onChange={(event) => change({ ...value, en: event.target.value })} /></label>
      <label>더하는 금액({unit})<input inputMode="decimal" value={value.price} onChange={(event) => change({ ...value, price: event.target.value })} /></label>
      <details>
        <summary>더해지는 성분: {value.allergens.length ? allergenText(value.allergens) : "없음"}</summary>
        <div className="grid">{commerceAllergens.map((code, index) => (
          <label key={code}>
            <input type="checkbox" checked={value.allergens.includes(code)} onChange={(event) =>
              change({ ...value, allergens: event.target.checked ? [...value.allergens, code] : value.allergens.filter((item) => item !== code) })} />
            {allergenNames[index]}
          </label>
        ))}</div>
      </details>
      <button type="button" className="quiet" disabled={!removable} aria-label={`${value.ko || `값 ${number}`} 지우기`} onClick={remove}>지우기</button>
    </div>
  );
}

/** Every combination the publication will make, for the products it changes. */
export function OptionReview({ before, after, asset, lines }: { before: Product[]; after: Product[]; asset: CatalogAsset; lines: number }) {
  const same = (product: Product) => JSON.stringify(before.find(({ sku }) => sku === product.sku)) === JSON.stringify(product);
  const changed = after.filter((product) => !same(product));
  const removed = before.filter(({ sku }) => !after.some((product) => product.sku === sku));
  return (
    <div>
      <h3>옵션 묶음에서 바뀌는 것</h3>
      <ul aria-label="옵션 묶음에서 바뀌는 것">
        {changed.map((product) => {
          const earlier = before.find(({ sku }) => sku === product.sku);
          return <li key={product.sku}>{product.label} · {earlier ? `${lineCount(earlier)}줄에서 ${lineCount(product)}줄` : `새로 더함, ${lineCount(product)}줄`}</li>;
        })}
        {removed.map(({ sku, label }) => <li key={sku}>{label} · 묶음이 있는 상품에서 빠짐</li>)}
        {after.length > changed.length && <li>묶음이 있는 다른 상품 {after.length - changed.length}개 · 바뀌지 않음</li>}
      </ul>
      {changed.length > 0 && <p>새로 생긴 조합에는 옵션별 재고 한도가 없습니다. 필요하면 게시한 뒤 다시 정해 주세요.</p>}
      <p>매장 전체 {lines} / {rowLimit}줄. 이미 결제한 주문은 바뀌지 않고, 결제 전 장바구니는 다시 확인을 받습니다.</p>
      {changed.map((product) => (
        <details key={product.sku} open={changed.length === 1}>
          <summary>{product.label} · 조합 {lineCount(product)}줄</summary>
          <div className="scroll" tabIndex={0} role="region" aria-label={`${product.label} 조합`}>
            <table>
              <thead><tr><th scope="col">조합</th><th scope="col" className="amount">상품 가격</th><th scope="col" className="amount">더하는 금액</th><th scope="col" className="amount">받는 금액</th><th scope="col">알레르기</th></tr></thead>
              <tbody>
                {combinations(product, asset).map((row) => (
                  <tr key={row.option}><td>{row.label}</td><td className="amount">{row.base}</td><td className="amount">{row.added}</td><td className="amount"><strong>{row.total}</strong></td><td>{allergenText(row.allergens)}</td></tr>
                ))}
              </tbody>
            </table>
          </div>
        </details>
      ))}
    </div>
  );
}

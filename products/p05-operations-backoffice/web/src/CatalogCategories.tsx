import type { CommerceCategory } from "@nu54/protocol";

type Binding = { sku: string; categoryKey: string };
export type CategoryDraft = {
  categories: CommerceCategory[];
  categoryBindings: Binding[];
};

export function CatalogCategories({ value, skus, change }: {
  value: CategoryDraft;
  skus: string[];
  change: (value: CategoryDraft) => void;
}) {
  const { categories, categoryBindings } = value;

  function edit(index: number, category: CommerceCategory) {
    change({
      categories: categories.map((row, i) => i === index ? category : row),
      categoryBindings,
    });
  }

  return <fieldset>
    <legend>메뉴 카테고리</legend>
    {categories.map((category, index) => <fieldset key={index}>
      <legend>카테고리 {index + 1}</legend>
      <label>카테고리 코드
        <input required pattern="[a-z0-9][a-z0-9_\-]{0,31}" value={category.key}
          readOnly={categoryBindings.some((binding) => binding.categoryKey === category.key)}
          onChange={(event) => edit(index, { ...category, key: event.target.value })} />
      </label>
      <label>카테고리 한국어 이름
        <input required maxLength={80} value={category.labels.ko}
          onChange={(event) => edit(index, { ...category, labels: { ...category.labels, ko: event.target.value } })} />
      </label>
      <label>카테고리 영어 이름
        <input required maxLength={80} value={category.labels.en}
          onChange={(event) => edit(index, { ...category, labels: { ...category.labels, en: event.target.value } })} />
      </label>
      <label>카테고리 표시 순서
        <input required type="number" min={0} max={9999} value={Number.isFinite(category.sortOrder) ? category.sortOrder : ""}
          onChange={(event) => edit(index, { ...category, sortOrder: event.target.valueAsNumber })} />
      </label>
      <button type="button" onClick={() => change({
        categories: categories.filter((_, i) => i !== index),
        categoryBindings: categoryBindings.filter((row) => row.categoryKey !== category.key),
      })}>카테고리 {index + 1} 제거</button>
    </fieldset>)}
    <button type="button" disabled={categories.length >= 50} onClick={() => change({
      ...value,
      categories: [...categories, { key: "", sortOrder: categories.length, labels: { ko: "", en: "" } }],
    })}>카테고리 추가</button>
    {!!categories.length && skus.map((sku) => <label key={sku}>
      {sku} 상품 카테고리
      <select aria-label={`${sku} 상품 카테고리`} value={categoryBindings.find((row) => row.sku === sku)?.categoryKey ?? ""}
        onChange={(event) => {
          const categoryKey = event.target.value;
          change({ ...value, categoryBindings: [
            ...categoryBindings.filter((row) => row.sku !== sku),
            ...(categoryKey ? [{ sku, categoryKey }] : []),
          ] });
        }}>
        <option value="">미분류</option>
        {categories.map(({ key, labels }, index) => key &&
          <option key={index} value={key}>{labels.ko || key}</option>)}
      </select>
    </label>)}
  </fieldset>;
}

export function CategoryReview({ value, skus }: { value: CategoryDraft; skus: string[] }) {
  return <ul aria-label="게시 카테고리">
    {value.categories.map(({ key, labels, sortOrder }) => <li key={key}>
      {labels.ko} · {labels.en} · {key} · 순서 {sortOrder} ·{" "}
      {value.categoryBindings.filter((row) => row.categoryKey === key).map((row) => row.sku).join(", ") || "연결 상품 없음"}
    </li>)}
    <li>미분류 · {skus.filter((sku) => !value.categoryBindings.some((row) => row.sku === sku)).join(", ") || "없음"}</li>
  </ul>;
}

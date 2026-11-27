// What a screen shows while a customer picks one value in each group of a product.
import { commerceLineCapacity, commerceLineKey, type CommerceSelection } from "./commerce-cart-review.ts";
import {
  commerceChoiceGroups,
  commerceMenuDisclosure,
  type CommerceChoiceGroup,
  type CommercePrice,
} from "./commerce-v2.ts";

/** Group key to value key. A group that is missing has not been picked. */
export type CommercePicks = Record<string, string>;

export type CommerceChoiceValue = CommerceChoiceGroup["values"][number] & {
  picked: boolean;
  /** Base units on top of the cheapest combination. */
  extra: bigint;
  /** "always": no combination is left. "others": none is left with what is picked in the other groups. */
  soldOut?: "always" | "others";
};

export type CommerceChoiceState = {
  groups: (Omit<CommerceChoiceGroup, "values"> & { picked: boolean; values: CommerceChoiceValue[] })[];
  /** The combination, once every group is picked. */
  row?: CommercePrice;
  /** The lowest price that can still be reached, and whether another one can be reached too. */
  from: string;
  varies: boolean;
};

const lowest = (rows: CommercePrice[]) =>
  rows.reduce((low, { unitPrice }) => (BigInt(unitPrice) < low ? BigInt(unitPrice) : low), BigInt(rows[0].unitPrice));

const matches = (row: CommercePrice, picks: CommercePicks, except?: string) =>
  row.choices!.every(({ group, value }) =>
    group.key === except || picks[group.key] === undefined || picks[group.key] === value.key);

/** Rows of one product whose choices are grouped, or undefined for a product with a single list of options. */
export function commerceChoiceState(
  rows: CommercePrice[],
  picks: CommercePicks,
  selection: CommerceSelection
): CommerceChoiceState | undefined {
  const groups = commerceChoiceGroups(rows);
  if (!groups.length) return undefined;
  // A value the menu no longer has counts as not picked.
  const current = currentPicks(groups, picks);
  const sellable = new Set(rows.filter((row) => commerceLineCapacity(row, selection) > 0));
  const floor = lowest(rows);
  const possible = rows.filter((row) => matches(row, current));
  const reachable = possible.filter((row) => sellable.has(row));
  const priced = reachable.length ? reachable : possible;
  const from = lowest(priced).toString();
  return {
    groups: groups.map((group, index) => ({
      key: group.key,
      labels: group.labels,
      picked: current[group.key] !== undefined,
      values: group.values.map((value) => {
        const has = (row: CommercePrice) => row.choices![index].value.key === value.key;
        const left = rows.filter((row) => sellable.has(row) && has(row));
        const soldOut = !left.length ? "always" as const
          : left.some((row) => matches(row, current, group.key)) ? undefined : "others" as const;
        return {
          ...value,
          picked: current[group.key] === value.key,
          extra: lowest(rows.filter(has)) - floor,
          ...(soldOut && { soldOut }),
        };
      }),
    })),
    ...(possible.length === 1 && groups.every(({ key }) => current[key] !== undefined) && { row: possible[0] }),
    from,
    varies: priced.some(({ unitPrice }) => unitPrice !== from),
  };
}

function currentPicks(groups: CommerceChoiceGroup[], picks: CommercePicks): CommercePicks {
  const out: CommercePicks = Object.create(null); // a group may be called "constructor"
  for (const { key, values } of groups)
    if (values.some((value) => value.key === picks[key])) out[key] = picks[key];
  return out;
}

/** Picks a value, or takes back the pick, so that no choice can lock the customer in. */
export function commercePick(picks: CommercePicks, group: string, value: string): CommercePicks {
  const { [group]: before, ...others } = picks;
  return before === value ? others : { ...others, [group]: value };
}

export type CommercePicking = { picks: CommercePicks; quantity: number };

/** The same, with the quantity: one that is already in the basket is edited, not added to. */
export function commercePickChange(
  rows: CommercePrice[],
  picking: CommercePicking,
  group: string,
  value: string,
  selection: CommerceSelection
): CommercePicking {
  const inBasket = (picks: CommercePicks) => {
    const row = commerceChoiceState(rows, picks, selection)?.row;
    return row && selection[commerceLineKey(row)]?.quantity;
  };
  const picks = commercePick(picking.picks, group, value);
  return { picks, quantity: inBasket(picks) ?? (inBasket(picking.picks) ? 1 : picking.quantity) };
}

/** The merchant's declaration of what can still be picked: "우유, 옵션에 따라 대두". */
export function commerceChoiceDisclosure(rows: CommercePrice[], picks: CommercePicks, locale: "ko" | "en") {
  const current = currentPicks(commerceChoiceGroups(rows), picks);
  const possible = rows.filter((row) => matches(row, current));
  const lists = possible.map(({ metadata }) => metadata?.allergens);
  // A row without a declaration says so; nothing that is declared elsewhere may read as complete.
  const undeclared = possible.find(({ metadata }) => metadata?.allergens === undefined);
  if (undeclared) return commerceMenuDisclosure(undeclared, locale);
  const every = [...new Set(lists.flatMap((list) => list ?? []))];
  const always = every.filter((code) => lists.every((list) => list!.includes(code)));
  const row = possible[0];
  return commerceMenuDisclosure({ ...row, metadata: { ...row.metadata, allergens: always } }, locale,
    every.filter((code) => !always.includes(code)));
}

/** "온도·크기를 골라 주세요" */
export function commerceChooseMessage(names: string[], locale: "ko" | "en") {
  const list = names.join(locale === "ko" ? "·" : ", ");
  if (locale === "en") return `Choose ${list}`;
  const last = list.charCodeAt(list.length - 1) - 0xac00;
  if (last < 0 || last > 11171) return `${list} 옵션을 골라 주세요`;
  return `${list}${last % 28 ? "을" : "를"} 골라 주세요`;
}

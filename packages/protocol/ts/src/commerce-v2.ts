import { validOrderEscrowStatus, validPayerEnrollment, validWalletReturn, validWalletSends, type OrderBuyerAction, type OrderEscrowStatus,
  type PayerEnrollment, type WalletReturn, type WalletSends } from "./order-escrow.ts";
import { validFaucetStatus, type FaucetStatus } from "./faucet.ts";
import { validTerminalDevice, validTerminalDeviceValue, type TerminalDevice, type TerminalDeviceName } from "./terminal-device.ts";
import { networkListLimit } from "./network-list.ts";
import {readShopIdentity, type ShopIdentity} from "./shop-identity.ts";
import { validRentalReviewChallenge, type PrivatePresentation, type RentalReviewChallenge } from "./private-presentation.ts";
import { sha256 } from "@noble/hashes/sha256";
import { bytesToHex, hexToBytes, utf8Encode } from "./bytes.ts";
import { quoteDigest, quoteV2Layout } from "./quote-v2.ts";
import {
  bindServiceRequestV2,
  validateServiceReplyV2,
  type ServiceReplyV2,
  type ServiceRequestV2,
  type ServiceScopeV2,
} from "./service-v2.ts";

export type CommercePickupChoice = { window: string; startsAt: string };
export type CommerceCartLine = { sku: string; option_key: string; quantity: number };
export type CommerceCart = {
  origin: "menu" | "pos";
  originRef: string;
  menuVersion: number;
  items: CommerceCartLine[];
  pickup?: CommercePickupChoice;
  nearby?: { customerRef: string; expiresAt: number };
};

export class CommerceReservationError extends Error {
  readonly unavailable: CommerceCartLine[];

  constructor(code: string, unavailable: CommerceCartLine[]) {
    super(code);
    this.unavailable = unavailable;
  }
}
export const commerceAllergens = [
  "milk",
  "egg",
  "fish",
  "shellfish",
  "tree-nuts",
  "peanut",
  "wheat",
  "soy",
  "sesame",
  "buckwheat",
  "peach",
  "tomato",
  "sulfites",
  "walnut",
  "pine-nuts",
  "pork",
  "beef",
  "chicken",
  "squid",
] as const;
export type CommerceAllergen = (typeof commerceAllergens)[number];
export type CommerceMenuMetadata = {
  optionRequired?: boolean;
  labels?: Partial<Record<"ko" | "en", string>>;
  optionLabels?: Partial<Record<"ko" | "en", string>>;
  optionGroupLabels?: Partial<Record<"ko" | "en", string>>;
  allergens?: CommerceAllergen[];
  crossContact?: "unknown" | "possible" | "declared-none";
  crossContactAllergens?: CommerceAllergen[];
  crossContactSource?: "shared-equipment" | "shared-baking-equipment";
};
export function commerceMenuLabels(row: CommercePrice, locale: "ko" | "en") {
  return {
    product: row.metadata?.labels?.[locale] ?? row.label,
    option: row.metadata?.optionLabels?.[locale] ?? row.optionKey,
  };
}
const koreanAllergens = [
  "우유",
  "달걀",
  "생선",
  "갑각류·패류",
  "견과류",
  "땅콩",
  "밀",
  "대두",
  "참깨",
  "메밀",
  "복숭아",
  "토마토",
  "아황산류",
  "호두",
  "잣",
  "돼지고기",
  "쇠고기",
  "닭고기",
  "오징어",
];

export function commerceMenuDisclosure(
  row: CommercePrice,
  locale: "ko" | "en"
) {
  const { allergens, crossContact, crossContactAllergens, crossContactSource } = row.metadata ?? {};
  const ko = locale === "ko";
  const ingredients =
    allergens === undefined
      ? ko
        ? "알레르기 성분 미입력"
        : "Allergen ingredients not supplied"
      : allergens.length
      ? (ko ? "점주 신고 성분 " : "Merchant-declared ingredients ") +
        allergens
          .map((code) =>
            ko ? koreanAllergens[commerceAllergens.indexOf(code)] : code
          )
          .join(", ")
      : ko
      ? "점주 신고 성분 없음"
      : "Merchant declared no listed ingredients";
  const status = {
    unknown: ko ? "교차접촉 미확인" : "Cross-contact unknown",
    possible: ko ? "교차접촉 가능" : "Cross-contact possible",
    "declared-none": ko
      ? "교차접촉 없음으로 점주 신고"
      : "Merchant declared no cross-contact",
  };
  let disclosure = crossContact
    ? status[crossContact]
    : ko ? "교차접촉 미입력" : "Cross-contact information not supplied";
  if (crossContact === "possible") {
    if (crossContactSource === "shared-equipment")
      disclosure = ko ? "공용 제조 설비에서 교차접촉 가능" : "Cross-contact possible on shared preparation equipment";
    else if (crossContactSource === "shared-baking-equipment")
      disclosure = ko ? "공용 제빵 설비에서 교차접촉 가능" : "Cross-contact possible on shared baking equipment";
    if (crossContactAllergens?.length) {
      const names = crossContactAllergens.map((code) =>
        ko ? koreanAllergens[commerceAllergens.indexOf(code)] : (code === "tree-nuts" ? "tree nuts" : code));
      disclosure += " · " + names.join(", ");
    }
  }
  return ingredients + " · " + disclosure;
}

function validMenuMetadata(value: unknown): boolean {
  if (value === undefined) return true;
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const metadata = value as CommerceMenuMetadata;
  const labelsValid = (labels: unknown) =>
    labels === undefined ||
    (!!labels &&
      typeof labels === "object" &&
      !Array.isArray(labels) &&
      Object.entries(labels).every(
        ([key, label]) =>
          ["ko", "en"].includes(key) &&
          typeof label === "string" &&
          label.length > 0 &&
          label.trim() === label &&
          [...label].length <= 120 &&
          !/\p{Cc}/u.test(label)
      ));
  const validAllergens = (list: unknown) =>
    Array.isArray(list) && list.length <= commerceAllergens.length &&
    new Set(list).size === list.length &&
    list.every(item => commerceAllergens.includes(item));
  return (
    (metadata.optionRequired === undefined || typeof metadata.optionRequired === "boolean") &&
    labelsValid(metadata.labels) &&
    labelsValid(metadata.optionLabels) &&
    (metadata.optionGroupLabels === undefined || (metadata.optionRequired === true &&
      labelsValid(metadata.optionGroupLabels) && Object.values(metadata.optionGroupLabels).length > 0 &&
      Object.values(metadata.optionGroupLabels).every(label => !/[\u202a-\u202e\u2066-\u2069]/u.test(label)))) &&
    (metadata.crossContactSource === undefined ||
      (metadata.crossContact === "possible" && ["shared-equipment","shared-baking-equipment"].includes(metadata.crossContactSource))) &&
    (metadata.crossContactAllergens === undefined || (metadata.crossContact === "possible" &&
      validAllergens(metadata.crossContactAllergens) && metadata.crossContactAllergens.length > 0)) &&
    (metadata.crossContact === undefined ||
      ["unknown", "possible", "declared-none"].includes(
        metadata.crossContact
      )) &&
    (metadata.allergens === undefined || validAllergens(metadata.allergens))
  );
}

export type CommerceCategory = {
  key: string;
  sortOrder: number;
  labels: Record<"ko" | "en", string>;
};

export type CommerceChoice = {
  group: { key: string; labels: CommerceOptionLabels };
  value: { key: string; labels: CommerceOptionLabels; order: number };
};
/** A label without English text shows the Korean one. */
export type CommerceOptionLabels = { ko: string; en?: string };
/** The independent choices of one product, as the server sends them once for each page. */
export type CommerceSkuOptions = {
  sku: string;
  groups: { key: string; labels: CommerceOptionLabels; values: { key: string; labels: CommerceOptionLabels }[] }[];
};

export type CommercePrice = {
  version: number;
  expiresAt: string;
  sku: string;
  optionKey: string;
  label: string;
  asset: string;
  unitPrice: string;
  available: number;
  skuAvailable?: number;
  metadata?: CommerceMenuMetadata;
  category?: CommerceCategory;
  /** One entry for each group, in the store's order. Absent for a product with a single list of options. */
  choices?: CommerceChoice[];
};
export type CommerceOrderPayment = {
  nearbyUnpaid?: true;
  stage: "received" | "relaySigned" | "submitted" | "checking" | "finalized" | "unpaid";
  chainId: 8283;
  relayTxHash?: string;
  txHash?: string;
  includedAt?: string;
  finalizedAt?: string;
  pickupReadyAt?: string;
  pickupStage?: "preparing" | "ready" | "collected" | "unfulfillable";
};
export type CommerceOrder = {
  id: string;
  revision: number;
  state: "preview" | "issued" | "checking" | "paid" | "closed";
  asset: string;
  amount: string;
  expiresAt: string;
  pickupNo?: number;
  receiptRef?: string;
  receiptKind?: "digital" | "booking" | "pickup-awaiting-acceptance";
  quoteDigest?: string;
  orderKey?: string;
  subscription?: SubscriptionInvoiceLifecycle;
  payment?: CommerceOrderPayment;
  /** Present only for an order paid through OrderEscrow. */
  escrow?: OrderEscrowStatus;
  cafeReview?: { branchLabel: string; payerBound: boolean };
  menuCheckout?: {
    branchLabel: string;
    payerBound: boolean;
    customerRef?: string;
    finalOutcome?: "relay-paid" | "expired-unpaid";
  };
};
export type CustomerOrderSummary = Pick<CommerceOrder, "id" | "state" | "asset" | "amount">;
type CustomerOrderRow = CustomerOrderSummary & {accessEndsAt?: string};
export type CheckoutComparison = {
  customerRef?: string;
  code: string;
  expiresAt: number;
};
export type CommerceClaim = { claimDigest: string; expiresAt: number };
export type CommerceQuote = {
  saleRef: string;
  branchRef: string;
  orderKey: string;
  signedQuote: string;
  quoteDigest: string;
  expiresAt: number;
  presentation?: PrivatePresentation;
};
export type StaffPermission = "pickup" | "terminal_settings" | "refund_console" | "print_receipts";
export type StaffAccess = {
  branchRef: string;
  actorRef: string;
  permissions: StaffPermission[];
  console?: string;
};
export type TerminalMode = { mode: "menu" | "pos"; revision: number };
export type TerminalTransition = {
  requestRef: string;
  sourceRevision: number;
  targetRevision: number;
  targetMode: "pos";
  handoff: boolean;
  ack: boolean;
  state: "waiting" | "active";
};
export type TerminalSettings = TerminalMode & {
  orderCount?: number;
  orderRefs?: string[];
  transition?: TerminalTransition;
};
export type SubscriptionOffer = {
  id: string;
  seriesRef: string;
  label: string;
  termsVersion: string;
  termsDigest: string;
  cadence: "weekly" | "monthly";
  timezone: string;
  anchorDay: number;
  dueDays: number;
  asset: string;
  amount: string;
  fulfillment: "physical" | "digital";
};
export type SubscriptionRecord = SubscriptionOffer & {
  offerRef: string;
  state: "active" | "paused" | "cancelled";
  nextAt?: string;
  revision: number;
  needsConsent: boolean;
};
export type SubscriptionInvoice = {
  id: string;
  subscriptionRef: string;
  saleRef: string;
  cycle: string;
  dueAt: string;
  state: CommerceOrder["state"];
  asset: string;
  amount: string;
  fulfillment: "physical" | "digital";
  stage?: "awaitingClosure" | "reconcile" | "expired";
};
export type SubscriptionInvoiceLifecycle = {
  invoiceRef: string;
  cycle: string;
  dueAt: string;
  termsVersion: string;
  stage: "preview" | "issued" | "checking" | "paid" | "closed" | "awaitingClosure" | "reconcile" | "expired";
  quoteRef?: string;
  quoteExpiresAt?: string;
};

function validInvoiceLifecycle(view: SubscriptionInvoiceLifecycle, state: CommerceOrder["state"]): boolean {
  if (!view || !reference.test(view.invoiceRef) ||
      !/^\d{4}-\d{2}-\d{2}$/.test(view.cycle) ||
      !Number.isFinite(Date.parse(view.dueAt)) ||
      typeof view.termsVersion !== "string" || !view.termsVersion.length)
    return false;
  const signed = view.quoteRef !== undefined || view.quoteExpiresAt !== undefined;
  if (signed && (!/^0x[a-f0-9]{64}$/.test(view.quoteRef ?? "") ||
      !Number.isFinite(Date.parse(view.quoteExpiresAt ?? "")))) return false;
  switch (view.stage) {
    case "preview":
    case "awaitingClosure":
      return state === "preview" && !signed;
    case "issued":
    case "checking":
    case "paid":
      return state === view.stage && signed;
    case "closed":
      return state === "closed";
    case "expired":
      return state === "closed" && signed;
    case "reconcile":
      return (state === "issued" || state === "checking") && signed;
    default:
      return false;
  }
}
function validOrderPayment(order: CommerceOrder) {
  const payment = order.payment;
  if (payment === undefined) return true;
  const hash32 = /^0x[0-9a-f]{64}$/;
  if (!payment || payment.chainId !== 8283 ||
      (payment.relayTxHash !== undefined && !hash32.test(payment.relayTxHash))) return false;

  if (payment.nearbyUnpaid !== undefined && (payment.nearbyUnpaid !== true || payment.stage !== "unpaid")) return false;

  if (payment.stage === "finalized") {
    const included = Date.parse(payment.includedAt ?? "");
    const finalized = Date.parse(payment.finalizedAt ?? "");
    const ready = Date.parse(payment.pickupReadyAt ?? "");
    return order.state === "paid" && hash32.test(payment.txHash ?? "") &&
      typeof payment.includedAt === "string" && Number.isFinite(included) &&
      (payment.finalizedAt === undefined || (typeof payment.finalizedAt === "string" && Number.isFinite(finalized) &&
        (order.pickupNo !== undefined || order.receiptKind === "pickup-awaiting-acceptance"))) &&
      (payment.pickupReadyAt === undefined || (typeof payment.pickupReadyAt === "string" && ready >= finalized &&
        (payment.pickupStage === "ready" || payment.pickupStage === "collected") && order.pickupNo !== undefined)) &&
      (payment.pickupStage === undefined || (order.pickupNo !== undefined &&
        ["preparing", "ready", "collected", "unfulfillable"].includes(payment.pickupStage)));
  }
  if (payment.txHash !== undefined || payment.includedAt !== undefined ||
      payment.finalizedAt !== undefined || payment.pickupReadyAt !== undefined || payment.pickupStage !== undefined)
    return false;
  if (payment.stage === "unpaid") return order.state === "closed";
  if (order.state !== "checking") return false;
  if (payment.stage === "received") return payment.relayTxHash === undefined;
  return ["relaySigned", "submitted", "checking"].includes(payment.stage) && payment.relayTxHash !== undefined;
}
function validOrderEscrow(order: CommerceOrder) {
  const escrow = order.escrow;
  if (escrow === undefined) return true;
  if (!validOrderEscrowStatus(escrow)) return false;
  if (order.orderKey !== undefined && order.orderKey !== escrow.orderKey) return false;
  // The store has the order exactly while the lock is final. A refund later keeps the paid record.
  if (escrow.stage === "received" || escrow.stage === "submitted") return order.state === "checking";
  if (escrow.stage === "failed") return order.state === "checking" || order.state === "closed";
  return order.state === "paid";
}
type Response = ServiceReplyV2 & {
  reservationFound?: boolean;
  unavailable?: CommerceCartLine[];
  comparison?: CheckoutComparison;
  comparisons?: CheckoutComparison[];
  terminal?: TerminalSettings;
  staff?: StaffAccess;
  terminalOrders?: string[];
  catalog?: CommercePrice[];
  catalogVersion?: number;
  catalogOptions?: CommerceSkuOptions[];
  shop?: ShopIdentity;
  order?: CommerceOrder;
  orders?: CustomerOrderRow[];
  payer?: PayerEnrollment;
  walletReturn?: WalletReturn;
  walletSends?: WalletSends;
  faucet?: FaucetStatus;
  networkList?: { payload: string; signature: string };
  terminalDevice?: TerminalDevice;
  claim?: CommerceClaim;
  offers?: SubscriptionOffer[];
  subscriptions?: SubscriptionRecord[];
  invoices?: SubscriptionInvoice[];
  quote?: CommerceQuote;
};
export type CommerceProfile = Pick<
  ServiceScopeV2,
  "tenantRef" | "subjectRef" | "expiresAt"
> & { role: "terminal" | "customer" | "operator" | "finance"; branchRef: string };

const code = /^[a-z0-9][a-z0-9_-]{0,31}$/;
const reference =
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const hash = /^[0-9a-f]{64}$/;
const amount = /^[1-9][0-9]{0,77}$/;
const assetRef = /^0x[0-9a-f]{40}$/;

export function encodeCommerceCart(input: CommerceCart): string {
  if (
    !reference.test(input.originRef) ||
    !Number.isSafeInteger(input.menuVersion) ||
    input.menuVersion < 1 ||
    !["menu", "pos"].includes(input.origin) ||
    input.items.length < 1 ||
    input.items.length > 20 ||
    (input.nearby !== undefined && (
      input.origin !== "menu" || input.pickup !== undefined ||
      !reference.test(input.nearby.customerRef) ||
      !Number.isSafeInteger(input.nearby.expiresAt) || input.nearby.expiresAt < 1
    )) ||
    (input.pickup !== undefined && (
      input.origin !== "menu" || !reference.test(input.pickup.window) ||
      !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,6})?Z$/.test(input.pickup.startsAt) ||
      !Number.isFinite(Date.parse(input.pickup.startsAt))
    ))
  )
    throw new Error("BAD_REQUEST");
  const items = input.items.map(({ sku, option_key, quantity }) => {
    if (
      !code.test(sku) ||
      !code.test(option_key) ||
      !Number.isInteger(quantity) ||
      quantity < 1 ||
      quantity > 1000
    ) {
      throw new Error("BAD_REQUEST");
    }
    return { sku, option_key, quantity };
  });
  const lineKey = (item: typeof items[number]) => `${item.sku}/${item.option_key}`;
  if (new Set(items.map(lineKey)).size !== items.length)
    throw new Error("BAD_REQUEST");
  items.sort((a, b) => {
    if (a.sku !== b.sku) return a.sku < b.sku ? -1 : 1;
    return a.option_key === b.option_key ? 0 : a.option_key < b.option_key ? -1 : 1;
  });
  return JSON.stringify({
    origin: input.origin,
    originRef: input.originRef,
    menuVersion: input.menuVersion,
    items,
    ...(input.pickup ? {pickup: {window: input.pickup.window, startsAt: input.pickup.startsAt}} : {}),
    ...(input.nearby ? {nearby: {customerRef: input.nearby.customerRef, expiresAt: input.nearby.expiresAt}} : {}),
  });
}

// Matches the P05 operator publication limit. Both C3 reads carry the same published rows.
export const commerceCatalogLimit = 600;
const commerceCatalogPageSize = 50;

const optionLabels = (labels: unknown): labels is CommerceOptionLabels =>
  !!labels && typeof labels === "object" && !Array.isArray(labels) && "ko" in labels &&
  Object.entries(labels).every(([key, label]) => (key === "ko" || key === "en") && typeof label === "string" &&
    label.length > 0 && label.trim() === label && [...label].length <= 24 &&
    !/[\p{Cc}\u202a-\u202e\u2066-\u2069]/u.test(label));
const optionKey = (key: unknown, length: number): key is string =>
  typeof key === "string" && key.length <= length && /^[a-z0-9]+$/.test(key);

function validChoices(row: CommercePrice): boolean {
  const { choices } = row;
  if (choices === undefined) return true;
  return Array.isArray(choices) && choices.length > 0 && choices.length <= 4 &&
    row.metadata?.optionRequired === true &&
    choices.every((choice) => !!choice?.group && !!choice.value &&
      optionKey(choice.group.key, 16) && optionLabels(choice.group.labels) &&
      optionKey(choice.value.key, 31) && optionLabels(choice.value.labels) &&
      Number.isSafeInteger(choice.value.order) && choice.value.order >= 0 && choice.value.order < 12) &&
    new Set(choices.map(({ group }) => group.key)).size === choices.length &&
    choices.map(({ value }) => value.key).join("-") === row.optionKey;
}

function withChoices(rows: CommercePrice[], options: unknown): CommercePrice[] {
  if (options === undefined) return rows;
  if (!Array.isArray(options) || options.length > rows.length) throw new Error("BAD_RESPONSE");
  const bySKU = new Map<string, CommerceSkuOptions>();
  for (const option of options as CommerceSkuOptions[]) {
    if (!option || !Array.isArray(option.groups) || bySKU.has(option.sku) ||
        option.groups.some((group) => !group || !Array.isArray(group.values) || group.values.length < 2 || group.values.length > 12))
      throw new Error("BAD_RESPONSE");
    bySKU.set(option.sku, option);
  }
  const out = rows.map((row) => {
    const option = bySKU.get(row.sku);
    if (!option) return row;
    const keys = row.optionKey.split("-");
    const choices = option.groups.map(({ key, labels, values }, index): CommerceChoice => {
      const order = values.findIndex((value) => value?.key === keys[index]);
      if (order < 0) throw new Error("BAD_RESPONSE");
      return { group: { key, labels }, value: { key: values[order].key, labels: values[order].labels, order } };
    });
    return { ...row, choices };
  });
  if (options.some(({ sku }) => !rows.some((row) => row.sku === sku)) || !out.every(validChoices))
    throw new Error("BAD_RESPONSE");
  return out;
}

export type CommerceChoiceGroup = {
  key: string;
  labels: CommerceOptionLabels;
  values: { key: string; labels: CommerceOptionLabels }[];
};

/** The groups of one product in the store's order, from all of its rows. Empty when it has a single list of options. */
export function commerceChoiceGroups(rows: CommercePrice[]): CommerceChoiceGroup[] {
  if (!rows.length || rows.some(({ choices }) => !choices)) return [];
  return rows[0].choices!.map(({ group }, index) => {
    const values = new Map(rows.map((row) => [row.choices![index].value.key, row.choices![index].value]));
    return { ...group, values: [...values.values()].sort((a, b) => a.order - b.order).map(({ key, labels }) => ({ key, labels })) };
  });
}

export function commerceOptionLabel(labels: CommerceOptionLabels, locale: "ko" | "en"): string {
  return labels[locale] ?? labels.ko;
}

/** A finished catalog has every combination of a product's choices, described the same way in every row. */
export function assertCommerceChoices(catalog: CommercePrice[]) {
  const products = new Map<string, CommercePrice[]>();
  for (const row of catalog) products.set(row.sku, [...(products.get(row.sku) ?? []), row]);
  for (const rows of products.values()) {
    if (rows.every(({ choices }) => !choices)) continue;
    const groups = commerceChoiceGroups(rows);
    const described = new Map<string, string>();
    const same = (key: string, value: unknown) => {
      const text = JSON.stringify(value);
      if (described.has(key) && described.get(key) !== text) return false;
      described.set(key, text);
      return true;
    };
    if (!groups.length || groups.some(({ values }) => values.length < 2) ||
        groups.reduce((count, { values }) => count * values.length, 1) !== rows.length ||
        rows.some(({ choices }) => choices!.length !== groups.length || choices!.some(({ group, value }, index) =>
          !same(`${index}`, [group.key, group.labels.ko, group.labels.en]) ||
          !same(`${index}/${value.key}`, [value.labels.ko, value.labels.en, value.order]) ||
          !same(`${index}#${value.order}`, value.key))))
      throw new Error("BAD_RESPONSE");
  }
}

function validCategory(value: unknown): boolean {
  if (value === undefined) return true;
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const category = value as CommerceCategory;
  const label = (s: unknown) => typeof s === "string" && s.length > 0 &&
    s.trim() === s && [...s].length <= 80 && !/\p{Cc}/u.test(s);
  return Object.keys(category).length === 3 &&
    typeof category.key === "string" && code.test(category.key) &&
    Number.isSafeInteger(category.sortOrder) && category.sortOrder >= 0 && category.sortOrder <= 9999 &&
    !!category.labels && typeof category.labels === "object" && !Array.isArray(category.labels) &&
    Object.keys(category.labels).length === 2 && label(category.labels.ko) && label(category.labels.en);
}

export function assertCommerceCatalogVersion(version: unknown, rows: CommercePrice[]): asserts version is number | undefined {
  if (version !== undefined &&
      (typeof version !== "number" || !Number.isSafeInteger(version) || version < 1 || rows.some((row) => row.version !== version)))
    throw new Error("CATALOG_CHANGED");
}

export function appendCommerceCatalogPage(catalog: CommercePrice[], rows: CommercePrice[]) {
  if (catalog.length + rows.length > commerceCatalogLimit)
    throw new Error("BAD_RESPONSE");
  const identity = (row: CommercePrice) => row.sku + "/" + row.optionKey;
  const seen = new Set(catalog.map(identity));
  const skuCategories = new Map<string, string | undefined>();
  const categories = new Map<string, CommerceCategory>();
  for (const row of [...catalog, ...rows]) {
    const category = row.category;
    if (skuCategories.has(row.sku) && skuCategories.get(row.sku) !== category?.key)
      throw new Error("BAD_RESPONSE");
    skuCategories.set(row.sku, category?.key);
    if (category) {
      const earlier = categories.get(category.key);
      if (earlier && (earlier.sortOrder !== category.sortOrder ||
          earlier.labels.ko !== category.labels.ko || earlier.labels.en !== category.labels.en))
        throw new Error("BAD_RESPONSE");
      categories.set(category.key, category);
    }
  }
  const version = (catalog[0] ?? rows[0])?.version;
  for (const row of rows) {
    const key = identity(row);
    if (row.version !== version)
      throw new Error("CATALOG_CHANGED");
    if (seen.has(key))
      throw new Error("BAD_RESPONSE");
    seen.add(key);
  }
  catalog.push(...rows);
}

export function assertCommerceCatalogPage(
  prices: unknown,
  at: number,
  limit: number,
): asserts prices is CommercePrice[] {
  if (
    !Array.isArray(prices) ||
    prices.length > limit ||
    prices.some(
      (row) =>
        !row ||
        !Number.isSafeInteger(row.version) ||
        row.version < 1 ||
        !Number.isSafeInteger(row.available) ||
        row.available < 0 ||
        (row.skuAvailable !== undefined &&
          (!Number.isSafeInteger(row.skuAvailable) || row.skuAvailable < row.available)) ||
        typeof row.label !== "string" ||
        row.label.length > 160 ||
        !validMenuMetadata(row.metadata) ||
        !validCategory(row.category) ||
        !validChoices(row) ||
        typeof row.sku !== "string" ||
        !code.test(row.sku) ||
        typeof row.optionKey !== "string" ||
        !code.test(row.optionKey) ||
        typeof row.asset !== "string" ||
        !assetRef.test(row.asset) ||
        typeof row.unitPrice !== "string" ||
        !amount.test(row.unitPrice) ||
        typeof row.expiresAt !== "string" ||
        !(Date.parse(row.expiresAt) > at * 1000)
    )
  ) {
    throw new Error("BAD_RESPONSE");
  }
}

/** Uses enrolled mTLS authority supplied by the platform adapter. IDs, order contents and prices are server-owned. */
export class CommerceClient {
  readonly profile: CommerceProfile;
  private readonly exchange: (request: string) => Promise<string>;
  private readonly newId: () => Promise<string>;
  private readonly clock: () => number;

  constructor(
    profile: CommerceProfile,
    exchange: (request: string) => Promise<string>,
    newId: () => Promise<string>,
    clock = () => Math.floor(Date.now() / 1000)
  ) {
    this.profile = profile;
    this.exchange = exchange;
    this.newId = newId;
    this.clock = clock;
  }

  private async call(
    operation: string,
    objectRef: string,
    body: Record<string, string>,
    extra?: {
      idempotencyKey: string;
      cart?: CommerceCart;
      approvalSignature?: string;
      rentalReview?: RentalReviewChallenge;
      terminalValue?: string;
    }
  ) {
    const requestId = await this.newId();
    const now = this.clock();
    const domainRef = this.profile.branchRef;
    const request: ServiceRequestV2 = {
      version: 2,
      contract: "C3",
      operation,
      requestId,
      domainRef,
      objectRef,
      expiresAt: Math.min(now + 30, this.profile.expiresAt),
      body,
      ...(extra ? { idempotencyKey: extra.idempotencyKey } : {}),
    };
    const binding = bindServiceRequestV2(
      request,
      {
        ...this.profile,
        grants: [{ domainRef, objectRef, operations: [`C3/${operation}`] }],
      },
      now
    );
    const raw = await this.exchange(
      JSON.stringify({
        command: request,
        cart: extra?.cart,
        approvalSignature: extra?.approvalSignature,
        rentalReview: extra?.rentalReview,
        terminalValue: extra?.terminalValue,
      })
    );
    if (raw.length > 32768) throw new Error("BAD_RESPONSE");
    const reply = JSON.parse(raw) as Response;
    validateServiceReplyV2(request, binding, reply, this.clock());
    if (reply.status === "error") {
      if (reply.reservationFound === true &&
        (operation === "order.create" || operation === "order.nearby-result") &&
        ["OUT_OF_STOCK", "CATALOG_CHANGED", "EXPIRED"].includes(reply.error ?? "")) {
        const lines = reply.unavailable ?? [];
        if (!Array.isArray(lines) || lines.length > 20 ||
          (reply.error === "OUT_OF_STOCK" && !lines.length) ||
          (reply.error !== "OUT_OF_STOCK" && lines.length) ||
          lines.some(line => !line || !code.test(line.sku) || !code.test(line.option_key) ||
            !Number.isSafeInteger(line.quantity) || line.quantity < 1 || line.quantity > 1000))
          throw new Error("BAD_RESPONSE");
        throw new CommerceReservationError(reply.error!, lines);
      }
      throw new Error(reply.error);
    }
    if (
      operation !== "order.create" &&
      operation !== "order.nearby-result" &&
      operation !== "subscription.accept" &&
      operation !== "subscription.reconsent" &&
      reply.resultRef !== objectRef
    )
      throw new Error("MISMATCHED_EVIDENCE");
    return reply;
  }

  async subscriptionOffers(): Promise<SubscriptionOffer[]> {
    return (
      (await this.call("subscription.offers", this.profile.subjectRef, {}))
        .offers ?? []
    );
  }

  async subscriptions(): Promise<SubscriptionRecord[]> {
    return (
      (await this.call("subscription.list", this.profile.subjectRef, {}))
        .subscriptions ?? []
    );
  }

  async invoices(): Promise<SubscriptionInvoice[]> {
    const rows = (
      (await this.call("invoice.list", this.profile.subjectRef, {})).invoices ??
      []
    );
    if (!Array.isArray(rows) || rows.some(row => row.stage !== undefined &&
        !(
          (row.stage === "awaitingClosure" && row.state === "preview") ||
          (row.stage === "reconcile" && (row.state === "issued" || row.state === "checking")) ||
          (row.stage === "expired" && row.state === "closed")
        ))) {
      throw new Error("BAD_RESPONSE");
    }
    return rows;
  }

  async skipInvoice(invoice: SubscriptionInvoice, key: string): Promise<void> {
    await this.call("invoice.skip", invoice.id, {}, { idempotencyKey: key });
  }

  async acceptSubscription(
    offer: SubscriptionOffer,
    key: string
  ): Promise<string> {
    return (
      await this.call(
        "subscription.accept",
        offer.id,
        { termsDigest: offer.termsDigest },
        { idempotencyKey: key }
      )
    ).resultRef!;
  }

  async changeSubscription(
    subscription: SubscriptionRecord,
    action: "pause" | "resume" | "cancel",
    key: string
  ): Promise<void> {
    await this.call(
      `subscription.${action}`,
      subscription.id,
      { expectedRevision: String(subscription.revision) },
      { idempotencyKey: key }
    );
  }

  async reconsentSubscription(
    subscription: SubscriptionRecord,
    offer: SubscriptionOffer,
    key: string
  ): Promise<string> {
    return (
      await this.call(
        "subscription.reconsent",
        subscription.id,
        {
          expectedRevision: String(subscription.revision),
          offerRef: offer.id,
          termsDigest: offer.termsDigest,
        },
        { idempotencyKey: key }
      )
    ).resultRef!;
  }

  async staffAccess(): Promise<StaffAccess> {
    const { branchRef, subjectRef, role } = this.profile;
    const { staff } = await this.call("staff.access", subjectRef, {});
    const permitted = role === "operator" ? ["pickup", "terminal_settings", "print_receipts"] :
      role === "terminal" ? ["pickup"] : role === "finance" ? ["refund_console"] : [];
    if (!staff || staff.branchRef !== branchRef || staff.actorRef !== subjectRef ||
        !Array.isArray(staff.permissions) || staff.permissions.length === 0 ||
        staff.permissions.some(permission => !permitted.includes(permission)) ||
        new Set(staff.permissions).size !== staff.permissions.length ||
        (staff.console !== undefined && (role !== "finance" || typeof staff.console !== "string" ||
          !/^https:\/\/(?:[A-Za-z0-9.-]+|\[[0-9A-Fa-f:]+\])(?::[0-9]{1,5})?\/?$/.test(staff.console))))
      throw new Error("MISMATCHED_EVIDENCE");
    return staff;
  }

  async terminalOrders(target: string, settings: TerminalSettings, afterSale: string): Promise<string[]> {
    const { transition, revision } = settings;
    const answer = await this.call("terminal.orders", target, {
      kind: transition ? "transition" : "preview",
      scopeRef: transition?.requestRef ?? target,
      sourceRevision: String(transition?.sourceRevision ?? revision),
      afterSale,
    });
    const orders = answer.terminalOrders ?? [];
    if (!Array.isArray(orders) || orders.length > 20 ||
        orders.some((ref, index) => !reference.test(ref) || ref <= (orders[index - 1] ?? afterSale)))
      throw new Error("MISMATCHED_EVIDENCE");
    return orders;
  }

  async terminalState(target = this.profile.subjectRef): Promise<TerminalSettings> {
    return this.modeFrom(await this.call("terminal.state", target, {}));
  }

  async changeMode(
    target: string,
    mode: TerminalMode["mode"],
    expected: number,
    idempotencyKey: string
  ): Promise<TerminalSettings> {
    const answer = await this.call(
      "terminal.mode",
      target,
      { mode, expectedRevision: String(expected) },
      { idempotencyKey }
    );
    const next = this.modeFrom(answer);
    if (next.transition) {
      this.transitionFrom(next, idempotencyKey, expected + 1);
      if (mode !== "pos") throw new Error("MISMATCHED_EVIDENCE");
    } else if (next.revision !== expected + 1 || next.mode !== mode) {
      throw new Error("MISMATCHED_EVIDENCE");
    }
    return next;
  }

  async confirmModeTransition(
    target: string,
    transition: TerminalTransition,
  ): Promise<TerminalSettings> {
    const next = this.modeFrom(await this.call(
      this.profile.role === "operator" ? "terminal.handoff" : "terminal.ack",
      target,
      { transitionRef: transition.requestRef, targetRevision: String(transition.targetRevision) },
      { idempotencyKey: transition.requestRef }
    ));
    this.transitionFrom(next, transition.requestRef, transition.targetRevision);
    if (this.profile.role === "operator" ? !next.transition?.handoff : !next.transition?.ack)
      throw new Error("MISMATCHED_EVIDENCE");
    return next;
  }

  private transitionFrom(
    mode: TerminalSettings,
    requestRef: string,
    version: number,
  ): TerminalSettings {
    if (mode.transition?.requestRef !== requestRef || mode.transition.targetRevision !== version)
      throw new Error("MISMATCHED_EVIDENCE");
    return mode;
  }

  private modeFrom(answer: Response): TerminalSettings {
    const mode = answer.terminal;
    if (
      !mode ||
      !["menu", "pos"].includes(mode.mode) ||
      !Number.isSafeInteger(mode.revision) ||
      mode.revision < 1 ||
      (mode.orderCount !== undefined && (!Number.isSafeInteger(mode.orderCount) || mode.orderCount < 0)) ||
      (mode.orderRefs !== undefined && (
        !Array.isArray(mode.orderRefs) || mode.orderRefs.length > 20 ||
        mode.orderRefs.some((ref, index) => !reference.test(ref) || (index > 0 && ref <= mode.orderRefs![index - 1])) ||
        (mode.orderCount !== undefined && mode.orderRefs.length > mode.orderCount)
      ))
    ) {
      throw new Error("BAD_RESPONSE");
    }
    const t = mode.transition;
    if (t !== undefined && (
      !t || !reference.test(t.requestRef) || t.targetMode !== "pos" ||
      !Number.isSafeInteger(t.sourceRevision) || t.sourceRevision < 1 ||
      !Number.isSafeInteger(t.targetRevision) || t.targetRevision !== t.sourceRevision + 1 ||
      typeof t.ack !== "boolean" || typeof t.handoff !== "boolean" ||
      (t.state !== "active" && t.state !== "waiting") ||
      (t.state === "waiting" && (mode.mode !== "menu" || mode.revision !== t.sourceRevision || (t.ack && t.handoff))) ||
      (t.state === "active" && (
        !t.ack || !t.handoff || mode.revision < t.targetRevision ||
        (mode.revision === t.targetRevision && mode.mode !== "pos")
      ))
    )) {
      throw new Error("MISMATCHED_EVIDENCE");
    }
    return mode;
  }

  async shopIdentity(): Promise<ShopIdentity> {
    const branch = this.profile.branchRef;
    const reply = await this.call("catalog.read", branch, {page: "0"});
    return readShopIdentity(reply.shop, branch);
  }

  async catalogWindow(page: number) {
    const reply = await this.call("catalog.read", this.profile.branchRef, {
      page: String(page),
    });
    const rows = reply.catalog ?? [];
    assertCommerceCatalogPage(rows, this.clock(), commerceCatalogPageSize);
    const prices = withChoices(rows, reply.catalogOptions);
    assertCommerceCatalogVersion(reply.catalogVersion, prices);
    return {prices, shop: reply.shop, snapshotVersion: reply.catalogVersion};
  }

  async catalog(page = 0): Promise<CommercePrice[]> {
    return (await this.catalogWindow(page)).prices;
  }

  private async readPublishedCatalog(identifyShop: boolean) {
    const catalog: CommercePrice[] = [];
    let shop: ShopIdentity | undefined;
    const lastPage = Math.floor(commerceCatalogLimit / commerceCatalogPageSize);
    let snapshot: number | undefined;
    for (let page = 0; page <= lastPage; page++) {
      const {prices, shop: branch, snapshotVersion} = await this.catalogWindow(page);
      if (page === 0) snapshot = snapshotVersion;
      else if (snapshot !== snapshotVersion) throw new Error("CATALOG_CHANGED");
      if (page === 0 && identifyShop && branch !== undefined) shop = readShopIdentity(branch, this.profile.branchRef);
      appendCommerceCatalogPage(catalog, prices);
      if (prices.length < commerceCatalogPageSize) {
        assertCommerceChoices(catalog);
        return {catalog, shop};
      }
    }
    throw new Error("BAD_RESPONSE");
  }

  async publishedCatalog(): Promise<CommercePrice[]> {
    return (await this.readPublishedCatalog(false)).catalog;
  }

  async publishedMenu(): Promise<{catalog: CommercePrice[]; shop?: ShopIdentity}> {
    return this.readPublishedCatalog(true);
  }

  async reserve(cart: CommerceCart, asset: string, idempotencyKey: string) {
    const canonical = encodeCommerceCart(cart);
    const sorted = JSON.parse(canonical) as CommerceCart;
    const quantity = sorted.items.reduce((sum, line) => sum + line.quantity, 0);
    const reply = await this.call(
      "order.create",
      cart.originRef,
      {
        asset,
        quantity: String(quantity),
        intentDigest: bytesToHex(sha256(utf8Encode(canonical)), false),
      },
      { idempotencyKey, cart: sorted }
    );
    return reply.resultRef!;
  }

  async cancelPublicOrder(sessionRef: string, reserveKey: string) {
    const reply = await this.call("order.cancel-public", sessionRef, {}, {idempotencyKey: reserveKey});
    if (reply.resultRef !== sessionRef) throw new Error("BAD_RESPONSE");
  }

  async nearbyReservation(target: string, customerRef: string, byOrder: boolean) {
    const reply = await this.call("order.nearby-result", target, {customerRef, byOrder: byOrder ? "1" : "0"});
    if (reply.reservationFound === true) {
      if (byOrder && reply.resultRef !== target) throw new Error("BAD_RESPONSE");
      return reply.resultRef;
    }
    if (reply.resultRef !== target) throw new Error("BAD_RESPONSE");
    return undefined;
  }

  async orders(after?: string): Promise<CustomerOrderSummary[]> {
    const subject = this.profile.subjectRef;
    const answer = await this.call("order.list", subject, {afterSale: after ?? subject});
    const rows = answer.orders ?? [];
    let previous = after;
    if (answer.resultRef !== subject || !Array.isArray(rows) || rows.length > 50 ||
        rows.some(row => {
          if (!row || !reference.test(row.id) || (previous !== undefined && row.id <= previous) ||
              !["preview", "issued", "checking", "paid", "closed"].includes(row.state) ||
              !assetRef.test(row.asset) || !amount.test(row.amount) ||
              (row.accessEndsAt !== undefined && !(Date.parse(row.accessEndsAt) > this.clock() * 1000))) return true;
          previous = row.id;
          return false;
        })) throw new Error("BAD_RESPONSE");
    return rows.map(({id, state, asset, amount}) => ({id, state, asset, amount}));
  }

  async status(id: string): Promise<CommerceOrder> {
    const { order } = await this.call("order.status", id, {});
    if (
      !order ||
      order.id !== id ||
      !Number.isSafeInteger(order.revision) ||
      order.revision < 0 ||
      !["preview", "issued", "checking", "paid", "closed"].includes(
        order.state
      ) ||
      !assetRef.test(order.asset) ||
      !amount.test(order.amount) ||
      !Number.isFinite(Date.parse(order.expiresAt)) ||
      (order.subscription !== undefined && !validInvoiceLifecycle(order.subscription, order.state)) ||
      !validOrderPayment(order) ||
      !validOrderEscrow(order) ||
      (order.payment?.nearbyUnpaid === true && this.profile.role !== "customer") ||
      (order.cafeReview !== undefined && (
        this.profile.role !== "customer" ||
        !order.cafeReview ||
        typeof order.cafeReview.branchLabel !== "string" ||
        !order.cafeReview.branchLabel.trim() ||
        Array.from(order.cafeReview.branchLabel).length > 120 ||
        typeof order.cafeReview.payerBound !== "boolean" ||
        order.subscription !== undefined
      )) ||
      (order.menuCheckout !== undefined && (
        this.profile.role !== "terminal" ||
        !order.menuCheckout ||
        typeof order.menuCheckout.branchLabel !== "string" ||
        !order.menuCheckout.branchLabel.trim() ||
        Array.from(order.menuCheckout.branchLabel).length > 120 ||
        typeof order.menuCheckout.payerBound !== "boolean" ||
        (order.menuCheckout.finalOutcome !== undefined &&
          !((order.menuCheckout.finalOutcome === "relay-paid" && order.state === "paid" && order.pickupNo !== undefined) ||
            (order.menuCheckout.finalOutcome === "expired-unpaid" && order.state === "closed"))) ||
        (order.menuCheckout.customerRef !== undefined &&
          (!order.menuCheckout.payerBound || !reference.test(order.menuCheckout.customerRef)))
      )) ||
      (order.state === "paid"
        ? !reference.test(order.receiptRef ?? "") ||
          (order.receiptKind === undefined
            ? !Number.isSafeInteger(order.pickupNo) || (order.pickupNo ?? 0) < 1
            : (!["digital", "booking"].includes(order.receiptKind) &&
              !(order.receiptKind === "pickup-awaiting-acceptance" && this.profile.role === "customer")) ||
              order.pickupNo !== undefined)
        : order.pickupNo !== undefined || order.receiptRef !== undefined || order.receiptKind !== undefined) ||
      (order.state === "issued"
        ? !hash.test(order.quoteDigest ?? "") ||
          !hash.test(order.orderKey ?? "")
        : order.quoteDigest !== undefined || order.orderKey !== undefined)
    )
      throw new Error("BAD_RESPONSE");
    return order;
  }

  async requestComparison(
    id: string,
    key: string
  ): Promise<CheckoutComparison> {
    const { comparison } = await this.call(
      "checkout.request",
      id,
      {},
      { idempotencyKey: key }
    );
    if (
      !comparison ||
      comparison.customerRef !== undefined ||
      !/^[a-f0-9]{8}$/.test(comparison.code) ||
      !Number.isSafeInteger(comparison.expiresAt) ||
      comparison.expiresAt <= this.clock()
    )
      throw new Error("BAD_RESPONSE");
    return comparison;
  }

  async posNearbyCheckout(
    id: string,
    customerRef: string,
    key: string,
    mode: "register" | "resume"
  ): Promise<{ comparison: CheckoutComparison } | { claim: CommerceClaim }> {
    const { comparison, claim } = await this.call(
      "checkout.pos-nearby",
      id,
      { customerRef, mode },
      { idempotencyKey: key }
    );
    if (comparison && !claim && mode === "register" &&
        comparison.customerRef === undefined && /^[a-f0-9]{8}$/.test(comparison.code) &&
        Number.isSafeInteger(comparison.expiresAt) && comparison.expiresAt > this.clock())
      return { comparison };
    if (claim && !comparison && hash.test(claim.claimDigest) &&
        Number.isSafeInteger(claim.expiresAt) && claim.expiresAt > this.clock())
      return { claim };
    throw new Error("BAD_RESPONSE");
  }

  async pendingComparisons(id: string): Promise<CheckoutComparison[]> {
    const result =
      (await this.call("checkout.pending", id, {})).comparisons ?? [];
    if (
      !Array.isArray(result) ||
      result.length > 8 ||
      result.some(
        (item) =>
          !item ||
          !reference.test(item.customerRef ?? "") ||
          !/^[a-f0-9]{8}$/.test(item.code) ||
          !Number.isSafeInteger(item.expiresAt) ||
          item.expiresAt <= this.clock()
      )
    )
      throw new Error("BAD_RESPONSE");
    return result;
  }

  async confirmComparison(
    id: string,
    candidate: CheckoutComparison,
    key: string
  ): Promise<CommerceClaim> {
    const reply = await this.call(
      "checkout.confirm",
      id,
      { customerRef: candidate.customerRef ?? "", code: candidate.code },
      { idempotencyKey: key }
    );
    return this.claimFrom(reply.claim);
  }

  async claim(id: string): Promise<CommerceClaim | undefined> {
    const reply = await this.call("checkout.claim", id, {});
    return reply.claim === undefined ? undefined : this.claimFrom(reply.claim);
  }

  private claimFrom(claim: CommerceClaim | undefined): CommerceClaim {
    if (
      !claim ||
      !hash.test(claim.claimDigest) ||
      !Number.isSafeInteger(claim.expiresAt) ||
      claim.expiresAt <= this.clock()
    ) {
      throw new Error("BAD_RESPONSE");
    }
    return claim;
  }

  async authorize(
    id: string,
    customerRef: string,
    idempotencyKey: string
  ): Promise<CommerceClaim> {
    const { claim } = await this.call(
      "checkout.authorize",
      id,
      { customerRef },
      { idempotencyKey }
    );
    return this.claimFrom(claim);
  }

  async quote(
    id: string,
    claimDigest: string,
    orderRevision: number,
    idempotencyKey: string,
    rentalReview?: RentalReviewChallenge
  ): Promise<CommerceQuote> {
    if (rentalReview !== undefined && (this.profile.role !== "customer" || !validRentalReviewChallenge(rentalReview)))
      throw new Error("BAD_REQUEST");
    const { quote } = await this.call(
      "quote.issue",
      id,
      { claimDigest, orderRevision: String(orderRevision) },
      { idempotencyKey, rentalReview }
    );
    if (
      !quote ||
      quote.saleRef !== id ||
      quote.branchRef !== this.profile.branchRef ||
      !hash.test(quote.orderKey) ||
      !hash.test(quote.quoteDigest) ||
      !Number.isSafeInteger(quote.expiresAt) ||
      quote.expiresAt <= this.clock() ||
      typeof quote.signedQuote !== "string" ||
      quote.signedQuote.length !==
        (quoteV2Layout.unsignedSize + quoteV2Layout.signatureSize) * 2 ||
      !/^[0-9a-f]+$/.test(quote.signedQuote) ||
      bytesToHex(
        quoteDigest(
          hexToBytes(quote.signedQuote).subarray(0, quoteV2Layout.unsignedSize)
        ),
        false
      ) !== quote.quoteDigest
    ) {
      throw new Error("BAD_RESPONSE");
    }
    return quote;
  }

  async forward(
    id: string,
    quoteDigest: string,
    approvalDigest: string,
    approvalSignature: string,
    idempotencyKey: string
  ) {
    if (!/^[0-9a-f]{128}(1b|1c)$/.test(approvalSignature))
      throw new Error("BAD_REQUEST");
    const result = await this.call(
      "payment.forward",
      id,
      { quoteDigest, approvalDigest },
      { idempotencyKey, approvalSignature }
    );
    return result.resultRef!;
  }

  /** Sends the board's order signature; the store's relay pays the gas. */
  async lock(
    id: string,
    quoteDigest: string,
    confirmer: string,
    lockDigest: string,
    payerSignature: string,
    idempotencyKey: string
  ) {
    if (!/^[0-9a-f]{128}(1b|1c)$/.test(payerSignature))
      throw new Error("BAD_REQUEST");
    const result = await this.call(
      "order.lock",
      id,
      { quoteDigest, confirmer, lockDigest },
      { idempotencyKey, approvalSignature: payerSignature }
    );
    return result.resultRef!;
  }

  /** Enrolls the board account this phone pays with. The board signed payerEnrollChallenge for this key. */
  async enrollPayer(payer: string, generation: bigint, accountSignature: string, idempotencyKey: string) {
    if (!/^[0-9a-f]{128}(1b|1c)$/.test(accountSignature)) throw new Error("BAD_REQUEST");
    await this.call("payer.enroll", this.profile.subjectRef, { payer, generation: generation.toString() },
      { idempotencyKey, approvalSignature: accountSignature });
  }

  async enrolledPayer(): Promise<PayerEnrollment | undefined> {
    const { payer } = await this.call("payer.status", this.profile.subjectRef, {});
    if (payer !== undefined && !validPayerEnrollment(payer)) throw new Error("BAD_RESPONSE");
    return payer;
  }

  /** Sends the board's signed transfer home. The relay pays the gas. */
  async returnWallet(
    transfer: { payer: string; to: string; amount: string; validBefore: number; nonce: string },
    boardSignature: string,
    idempotencyKey: string
  ) {
    if (!/^[0-9a-f]{128}(1b|1c)$/.test(boardSignature)) throw new Error("BAD_REQUEST");
    await this.call("wallet.return", this.profile.subjectRef, { ...transfer, validBefore: String(transfer.validBefore) },
      { idempotencyKey, approvalSignature: boardSignature });
  }

  async walletReturn(): Promise<WalletReturn | undefined> {
    const { walletReturn } = await this.call("wallet.return-status", this.profile.subjectRef, {});
    if (walletReturn !== undefined && !validWalletReturn(walletReturn)) throw new Error("BAD_RESPONSE");
    return walletReturn;
  }

  /** Sends the board's signed transfer to any address. The store relays it and pays the gas. Repeat with the same key. */
  async sendWallet(
    transfer: { payer: string; to: string; token: string; amount: string; validBefore: number; nonce: string },
    boardSignature: string,
    idempotencyKey: string
  ) {
    if (!/^[0-9a-f]{128}(1b|1c)$/.test(boardSignature)) throw new Error("BAD_REQUEST");
    await this.call("wallet.send", this.profile.subjectRef, { ...transfer, validBefore: String(transfer.validBefore) },
      { idempotencyKey, approvalSignature: boardSignature });
  }

  async walletSends(): Promise<WalletSends> {
    const { walletSends } = await this.call("wallet.send-status", this.profile.subjectRef, {});
    if (!validWalletSends(walletSends)) throw new Error("BAD_RESPONSE");
    return walletSends;
  }

  /** Asks for one claim of a listed test token. It goes to the enrolled payer, never elsewhere. Repeat with the same key. */
  async claimFaucet(token: string, idempotencyKey: string): Promise<void> {
    await this.call("faucet.claim", this.profile.subjectRef, { token }, { idempotencyKey });
  }

  async faucetStatus(): Promise<FaucetStatus> {
    const { faucet } = await this.call("faucet.status", this.profile.subjectRef, {});
    if (!validFaucetStatus(faucet)) throw new Error("BAD_RESPONSE");
    return faucet;
  }

  /** The operator-signed network list, unread. Pass it to readNetworkList. */
  async networkList(): Promise<string> {
    const { networkList } = await this.call("network.list", this.profile.subjectRef, {});
    if (!networkList || typeof networkList.payload !== "string" || typeof networkList.signature !== "string")
      throw new Error("BAD_RESPONSE");
    const raw = JSON.stringify({ payload: networkList.payload, signature: networkList.signature });
    if (raw.length > networkListLimit) throw new Error("BAD_RESPONSE");
    return raw;
  }

  async terminalDevice(target = this.profile.subjectRef): Promise<TerminalDevice> {
    const { terminalDevice } = await this.call("terminal.device", target, {});
    if (!validTerminalDevice(terminalDevice, this.profile.role)) throw new Error("BAD_RESPONSE");
    return terminalDevice;
  }

  /** Changes one setting of a kiosk. The server records who changed it, when, and both values. */
  async changeTerminalDevice(
    target: string,
    name: TerminalDeviceName,
    value: string,
    expectedRevision: number,
    idempotencyKey: string
  ): Promise<TerminalDevice> {
    if (!validTerminalDeviceValue(name, value)) throw new Error("BAD_REQUEST");
    const valueDigest = bytesToHex(sha256(utf8Encode(value)), false);
    const { terminalDevice } = await this.call("terminal.device-change", target,
      { name, valueDigest, expectedRevision: String(expectedRevision) }, { idempotencyKey, terminalValue: value });
    if (!validTerminalDevice(terminalDevice, this.profile.role) || terminalDevice.revision <= expectedRevision)
      throw new Error("MISMATCHED_EVIDENCE");
    return terminalDevice;
  }

  /** The buyer's answer after delivery, signed by the phone key that the order is locked to. */
  async answer(
    id: string,
    action: OrderBuyerAction,
    claimableAt: number,
    phoneSignature: string,
    idempotencyKey: string
  ) {
    if (!/^[0-9a-f]{128}(1b|1c)$/.test(phoneSignature))
      throw new Error("BAD_REQUEST");
    const result = await this.call(
      "order.answer",
      id,
      { action, claimableAt: String(claimableAt) },
      { idempotencyKey, approvalSignature: phoneSignature }
    );
    return result.resultRef!;
  }
}

export function commerceProfile(
  raw: string,
  expected: CommerceProfile["role"],
  trustedPin?: string
): CommerceProfile {
  if (raw.length > 16384) throw new Error("COMMERCE_ENROLLMENT");
  const { scope, pin } = JSON.parse(raw) as {
    scope?: CommerceProfile;
    pin?: string;
  };
  if (
    !scope ||
    scope.role !== expected ||
    !reference.test(scope.branchRef) ||
    !reference.test(scope.tenantRef) ||
    !reference.test(scope.subjectRef) ||
    !Number.isSafeInteger(scope.expiresAt) ||
    typeof pin !== "string" ||
    !hash.test(pin) ||
    (trustedPin !== undefined && trustedPin !== pin)
  )
    throw new Error("COMMERCE_ENROLLMENT");
  return { ...scope };
}

/** RFC 4122 v4 from the product's OS random source. */
export function commerceRequestId(random: Uint8Array): string {
  if (random.length !== 16) throw new Error("COMMERCE_RANDOM");
  const bytes = random.slice();
  bytes[6] = (bytes[6] & 15) | 0x40;
  bytes[8] = (bytes[8] & 63) | 0x80;
  const id = bytesToHex(bytes, false);
  return [
    id.slice(0, 8),
    id.slice(8, 12),
    id.slice(12, 16),
    id.slice(16, 20),
    id.slice(20),
  ].join("-");
}

/** Issuance advances the order revision once. Reuse the original command intent on reply loss. */
export function quoteOrderRevision(order: CommerceOrder): number {
  if (order.state === "preview") return order.revision;
  if (order.state === "issued" && order.revision > 0) return order.revision - 1;
  throw new Error("ORDER_NOT_ISSUABLE");
}

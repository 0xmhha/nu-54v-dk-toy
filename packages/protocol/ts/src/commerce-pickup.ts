export const pickupKitchenStates = ["accepted", "preparing", "ready", "collected", "unfulfillable"] as const;
export type PickupKitchenState = typeof pickupKitchenStates[number];
export type PickupTarget = Exclude<PickupKitchenState, "accepted">;
export type PickupOrderState = PickupKitchenState | "waiting-payment" | "awaiting-acceptance" | "closed";
export type PickupRefund = {
  state: "awaiting-request" | "pending" | "rejected" | "confirmed";
  amount?: string;
  receiptRef?: string;
};
export type PickupCustomerOrder = {
  saleRef: string;
  startsAt: string;
  timezone: string;
  amount: string;
  asset: string;
  state: PickupOrderState;
  pickupNo?: number;
  receiptRef?: string;
  refund?: PickupRefund;
};
export type PickupKitchenSettlement = {
  amount: string;
  asset: string;
  finalizedAt: string;
  pickupReadyAt: string;
};
export type PickupKitchenOrder = {
  saleRef: string;
  startsAt: string;
  timezone: string;
  state: PickupKitchenState | "checking";
  kind?: "cafe" | "travel";
  eventRef?: string;
  pickupNo?: number;
  settlement?: PickupKitchenSettlement;
  revision: number;
  items?: {label: string; option: string; quantity: number}[];
  /** Present only for an order paid through OrderEscrow. Absent rows keep the earlier refund flow. */
  escrow?: {stage: PickupEscrowStage};
};
const escrowStages = ["locked", "delivered", "disputed", "paid", "refunded"] as const;
export type PickupEscrowStage = typeof escrowStages[number];

const reference = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const units = /^[1-9][0-9]{0,77}$/;
const positiveNumber = (n: number | undefined) => Number.isSafeInteger(n) && (n ?? 0) > 0;
const knownState = (state: string) => (pickupKitchenStates as readonly string[]).includes(state);
function supportedPickupZone(zone: string) {
  if (typeof zone !== "string" || zone.length > 100 || !/^[A-Za-z]/.test(zone)) return false;
  try {
    new Intl.DateTimeFormat("ko-KR", {timeZone: zone});
    return true;
  } catch {
    return false;
  }
}

const queueBase = (order: {saleRef: string; startsAt: string; timezone: string}) => !!order &&
  reference.test(order.saleRef) && Number.isFinite(Date.parse(order.startsAt)) && supportedPickupZone(order.timezone);
const refundValid = (value: PickupRefund | undefined) => !!value && (
  value.state === "confirmed" ? units.test(value.amount ?? "") && reference.test(value.receiptRef ?? "") :
  value.receiptRef === undefined && (value.state === "awaiting-request" ? value.amount === undefined :
    ["pending", "rejected"].includes(value.state) && units.test(value.amount ?? ""))
);

function collection(raw: string): unknown[] {
  const packet = JSON.parse(raw);
  if (!packet || !Array.isArray(packet.pickups) || packet.pickups.length > 32) throw new Error("PICKUP_UNAVAILABLE");
  return packet.pickups;
}

export function readPickupCustomer(raw: string): PickupCustomerOrder[] {
  const list = collection(raw) as PickupCustomerOrder[];
  for (const order of list) {
    if (!queueBase(order) || !units.test(order.amount) || !/^0x[0-9a-f]{40}$/.test(order.asset))
      throw new Error("PICKUP_UNAVAILABLE");
    const accepted = knownState(order.state);
    const awaiting = order.state === "awaiting-acceptance";
    if ((accepted ? !positiveNumber(order.pickupNo) : order.pickupNo !== undefined) ||
        (accepted || awaiting ? !reference.test(order.receiptRef ?? "") :
          order.receiptRef !== undefined || !["waiting-payment", "closed"].includes(order.state)) ||
        (order.state === "unfulfillable" ? !refundValid(order.refund) : order.refund !== undefined))
      throw new Error("PICKUP_UNAVAILABLE");
  }
  return list;
}

function kitchenTimestamp(value: string) {
  if (typeof value !== "string" || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z$/.test(value)) return false;
  const millis = Date.parse(value);
  return Number.isFinite(millis) && new Date(millis).toISOString() === value.slice(0, 23) + "Z";
}

function kitchenSettlement(order: PickupKitchenOrder) {
  const value = order.settlement;
  return value === undefined || (value !== null && order.kind === "cafe" &&
    (order.state === "ready" || order.state === "collected") &&
    typeof value.amount === "string" && units.test(value.amount) &&
    typeof value.asset === "string" && /^0x[0-9a-f]{40}$/.test(value.asset) &&
    kitchenTimestamp(value.finalizedAt) && kitchenTimestamp(value.pickupReadyAt) &&
    value.pickupReadyAt >= value.finalizedAt);
}

export function readPickupKitchen(raw: string): PickupKitchenOrder[] {
  const list = collection(raw) as PickupKitchenOrder[];
  for (const order of list) {
    if (!queueBase(order) || !Number.isSafeInteger(order.revision) || order.revision < 0 ||
        (order.kind !== undefined && order.kind !== "cafe" && order.kind !== "travel") ||
        (order.state === "checking" ? order.kind !== "cafe" || order.revision !== 0 ||
          order.pickupNo !== undefined || order.eventRef !== undefined || order.items !== undefined :
          !knownState(order.state) || !positiveNumber(order.pickupNo) ||
          (order.kind !== undefined ? !reference.test(order.eventRef ?? "") : order.eventRef !== undefined)) ||
        !kitchenSettlement(order) ||
        (order.escrow !== undefined && (!order.escrow || order.state === "checking" || Object.keys(order.escrow).length !== 1 ||
          !escrowStages.includes(order.escrow.stage))) ||
        (order.items !== undefined && (!Array.isArray(order.items) || order.items.length > 20 ||
          order.items.some(item => !item || typeof item.label !== "string" || item.label.length > 120 ||
            typeof item.option !== "string" || item.option.length > 32 || !positiveNumber(item.quantity)))))
      throw new Error("PICKUP_UNAVAILABLE");
  }
  return list.map(({saleRef, startsAt, timezone, state, kind, eventRef, pickupNo, revision, items, settlement, escrow}) => ({
    saleRef, startsAt, timezone, state, revision,
    ...(kind === undefined ? {} : {kind}),
    ...(eventRef === undefined ? {} : {eventRef}),
    ...(pickupNo === undefined ? {} : {pickupNo}),
    ...(settlement === undefined ? {} : {settlement: {
      amount: settlement.amount, asset: settlement.asset,
      finalizedAt: settlement.finalizedAt, pickupReadyAt: settlement.pickupReadyAt,
    }}),
    ...(escrow === undefined ? {} : {escrow: {stage: escrow.stage}}),
    ...(items === undefined ? {} : {items: items.map(({label, option, quantity}) => ({label, option, quantity}))}),
  }));
}

export function formatPickupTime(startsAt: string, timezone: string) {
  return new Intl.DateTimeFormat("ko-KR", {dateStyle: "medium", timeStyle: "short", timeZone: timezone}).format(new Date(startsAt));
}

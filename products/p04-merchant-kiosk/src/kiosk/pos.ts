import { dollarToken, tokenAmount, type CommerceOrder, type PaymentToken } from '@nu54/protocol';

export type POSOrder = {
  branch: string;
  branchLabel: string;
  handoff: string;
  reference: string;
  chainId: 8283;
  payerBound: boolean;
  selectedCode?: string;
  selectionHeld?: boolean;
  order: CommerceOrder;
};

const uuid =
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const externalRef = /^[A-Za-z0-9][A-Za-z0-9._/-]{0,63}$/;
const amount = /^[1-9][0-9]{0,77}$/;
const asset = /^0x[0-9a-f]{40}$/;
const hash = /^[0-9a-f]{64}$/;

export const validPOSHandoff = (value: string) => uuid.test(value);

export async function readPOSOrder(
  exchange: (raw: string) => Promise<string>,
  branch: string,
  handoff: string,
): Promise<POSOrder> {
  if (!validPOSHandoff(handoff)) throw new Error('POS_HANDOFF_INVALID');
  const raw = await exchange(
    JSON.stringify({ pos: { operation: 'lookup', branch, handoff } }),
  );
  if (raw.length > 8192) throw new Error('BAD_RESPONSE');
  const result = JSON.parse(raw) as POSOrder & { error?: string };
  if (result.error === 'POS_ASSET_UNSUPPORTED') throw new Error(result.error);
  const order = result.order;
  if (
    result.branch !== branch ||
    result.handoff !== handoff ||
    result.chainId !== 8283 ||
    typeof result.branchLabel !== 'string' ||
    !result.branchLabel.trim() ||
    Array.from(result.branchLabel).length > 120 ||
    !externalRef.test(result.reference) ||
    typeof result.payerBound !== 'boolean' ||
    (result.selectedCode !== undefined &&
      (!result.payerBound || typeof result.selectedCode !== 'string' || !/^[a-f0-9]{8}$/.test(result.selectedCode))) ||
    !order ||
    order.receiptKind !== undefined ||
    !uuid.test(order.id) ||
    !Number.isSafeInteger(order.revision) ||
    order.revision < 0 ||
    !['preview', 'issued', 'checking', 'paid', 'closed'].includes(
      order.state,
    ) ||
    !amount.test(order.amount) ||
    !asset.test(order.asset) ||
    !Number.isFinite(Date.parse(order.expiresAt)) ||
    (order.state === 'paid'
      ? !uuid.test(order.receiptRef ?? '') ||
        !Number.isSafeInteger(order.pickupNo) ||
        (order.pickupNo ?? 0) < 1
      : order.receiptRef !== undefined || order.pickupNo !== undefined) ||
    (result.selectionHeld !== undefined &&
      (result.selectionHeld !== true || order.state !== 'preview' || !result.payerBound || result.selectedCode !== undefined)) ||
    (result.selectedCode !== undefined && !['preview', 'issued'].includes(order.state)) ||
    (order.state === 'issued'
      ? !hash.test(order.quoteDigest ?? '') || !hash.test(order.orderKey ?? '')
      : order.quoteDigest !== undefined || order.orderKey !== undefined)
  )
    throw new Error('BAD_RESPONSE');
  return result;
}

/** A POS order is always in the store's own asset. */
export function posAmount(raw: string, token: PaymentToken = dollarToken) {
  return tokenAmount(raw, token);
}

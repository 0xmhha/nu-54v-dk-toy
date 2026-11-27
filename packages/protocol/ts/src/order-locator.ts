/** An expiring public pointer. Authentication and order access stay in C3. */
export type OrderLocator = {
  branchRef: string;
  orderRef: string;
  expiresAt: number;
};

const uuid = "[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}";
const form = new RegExp(
  `^nu54://order\\?v=2&branch=(${uuid})&order=(${uuid})&expires=([1-9][0-9]{0,15})$`
);
const absent = "00000000-0000-0000-0000-000000000000";

export function parseOrderLocator(raw: string, now: number): OrderLocator {
  const match = raw.length <= 132 ? form.exec(raw) : null;
  if (
    !match ||
    match[0] !== raw ||
    match[1] === absent ||
    match[2] === absent
  ) {
    throw new Error("ORDER_LOCATOR_INVALID");
  }
  const expiresAt = Number(match[3]);
  if (
    !Number.isSafeInteger(now) ||
    now < 0 ||
    !Number.isSafeInteger(expiresAt) ||
    expiresAt <= now ||
    expiresAt - now > 60
  ) {
    throw new Error("ORDER_LOCATOR_EXPIRED");
  }
  return { branchRef: match[1], orderRef: match[2], expiresAt };
}

export function encodeOrderLocator(locator: OrderLocator, now: number): string {
  const raw = `nu54://order?v=2&branch=${locator.branchRef}&order=${locator.orderRef}&expires=${locator.expiresAt}`;
  parseOrderLocator(raw, now);
  return raw;
}

import type {Approval, Snapshot} from './lab';

const baseScale = 100_000_000n;
const maxUnits = 10n ** 24n;
const maxPrice = 100_000_000n;
const minimumMargin = 1_000_000n;
const fundingInterval = 300;
const freshFor = 120;

function units(value: unknown, max: bigint): bigint | undefined {
  if (typeof value !== 'string' || !/^(?:0|[1-9][0-9]{0,77})$/.test(value)) return;
  const amount = BigInt(value);
  return amount <= max ? amount : undefined;
}

export function verifiedPositionPrice(snapshot: Snapshot, elapsed = 0) {
  if (snapshot.state !== 'ready' || !Number.isSafeInteger(snapshot.time) || snapshot.time <= 0) return;
  const price = units(snapshot.price, maxPrice);
  const version = units(snapshot.priceVersion, 2n ** 256n - 1n);
  if (!price || !version || !Array.isArray(snapshot.observations) || snapshot.observations.length !== 2) return;
  const [a, b] = snapshot.observations.map(item => units(item?.price, maxPrice));
  const [first, second] = snapshot.observations.map(item => Number(units(item?.observedAt, BigInt(Number.MAX_SAFE_INTEGER))));
  const observedAt = Math.min(first, second);
  if (!a || !b || !observedAt || Math.max(first, second) > snapshot.time ||
      snapshot.time + elapsed - observedAt > freshFor) return;
  const [lower, higher] = a < b ? [a,b] : [b,a];
  if ((higher - lower) * 100n > lower || (lower + higher) / 2n !== price) return;
  return {price, observedAt, nextFunding: (Math.floor(snapshot.time / fundingInterval) + 1) * fundingInterval};
}

function lastAtMost(upper: bigint, predicate: (value: bigint) => boolean) {
  let lower = 0n;
  while (lower < upper) {
    const middle = (lower + upper + 1n) / 2n;
    if (predicate(middle)) lower = middle;
    else upper = middle - 1n;
  }
  return lower;
}

function liquidation(long: boolean, size: bigint, initial: bigint, margin: bigint, entry: bigint) {
  const maintenance = (price: bigint) => (size * price / baseScale) / 10n;
  if (!long) {
    const safe = (price: bigint) => {
      const equity = margin + (size * (entry - price)) / baseScale;
      const capped = equity < initial * 2n ? equity : initial * 2n;
      return capped > maintenance(price);
    };
    return {higher: safe(maxPrice) ? undefined : lastAtMost(maxPrice, safe) + 1n};
  }

  // Below entry, the sum of the two truncated notionals differs from
  // the entry notional by at most one unit. Only the last two possible
  // notional buckets can therefore need exact boundary correction.
  const lossBudget = size * entry / baseScale - margin;
  const lower = (price: bigint) => {
    const notional = size * price / baseScale;
    return notional - notional / 10n;
  };
  let downside = lossBudget < 0n ? 0n : lastAtMost(entry, price => lower(price) <= lossBudget);
  while (downside > 0n && lower(downside) === lossBudget) {
    const notional = size * downside / baseScale;
    const qualifyingPrice = entry - ((margin - notional / 10n) * baseScale + size - 1n) / size;
    const exact = qualifyingPrice < downside ? qualifyingPrice : downside;
    if (exact > 0n && size * exact / baseScale === notional) {
      downside = exact;
      break;
    }
    downside = (notional * baseScale + size - 1n) / size - 1n;
  }
  const upside = (initial * 20n * baseScale + size - 1n) / size;
  return {lower: downside > 0n ? downside : undefined, higher: upside <= maxPrice ? upside : undefined};
}

export function reviewPosition(quote: Approval, snapshot: Snapshot) {
  const {action} = quote;
  if (action !== 'open-long' && action !== 'open-short') return;
  const reference = verifiedPositionPrice(snapshot);
  const margin = units(quote.a,maxUnits);
  const size = units(quote.b,maxUnits);
  if (!reference || quote.priceVersion !== snapshot.priceVersion || quote.price !== reference.price.toString() || quote.observedAt !== reference.observedAt ||
      !margin || !size || quote.first !== quote.a || quote.second !== quote.b) return;
  const notional = size * reference.price / baseScale;
  if (notional < minimumMargin || notional > margin) return;
  const long = action === 'open-long';
  const funding = notional / 1_000n;
  const sizeLimit = ((margin + 1n) * baseScale - 1n) / reference.price;
  return {...reference, long, margin, size, funding, sizeLimit: sizeLimit < maxUnits ? sizeLimit : maxUnits,
    beforeFunding: liquidation(long, size, margin, margin, reference.price),
    afterFunding: liquidation(long, size, margin, long ? margin - funding : margin + funding, reference.price)};
}

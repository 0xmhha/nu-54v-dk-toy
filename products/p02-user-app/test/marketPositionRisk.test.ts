import {reviewPosition, verifiedPositionPrice} from '../src/market/positionRisk';
import type {Approval, Snapshot} from '../src/market/lab';

const now = 1777_000_010;
const snapshot = {
  state:'ready', time:now, price:'1200000', priceVersion:'4',
  observations:[{price:'1200000',observedAt:String(now - 2)}, {price:'1200000',observedAt:String(now - 1)}],
} as Snapshot;
const quote = {
  action:'open-short', a:'12000000', b:'100000000', first:'12000000', second:'100000000',
  price:'1200000', priceVersion:'4', observedAt:now - 2,
} as Approval;

test('projects actual registered short terms and truncated funding/maintenance in integer units', () => {
  const risk = reviewPosition(quote,snapshot)!;
  expect(risk).toMatchObject({
    price:1_200_000n, observedAt:now - 2, nextFunding:(Math.floor(now / 300) + 1) * 300,
    long:false, margin:12_000_000n, size:100_000_000n, funding:1_200n, sizeLimit:1_000_000_083n,
    beforeFunding:{higher:12_000_000n}, afterFunding:{higher:12_001_091n},
  });
});

test('distinguishes adverse long thresholds from the high-price equity cap', () => {
  expect(reviewPosition({...quote,action:'open-long'}, snapshot)?.beforeFunding).toEqual({lower:undefined,higher:undefined});
  const maxLeverage = reviewPosition({...quote, action:'open-long', a:'1200000', first:'1200000'},snapshot)!;
  expect(maxLeverage.beforeFunding).toEqual({lower:undefined,higher:24_000_000n});
  expect(maxLeverage.afterFunding).toEqual({lower:1_333n,higher:24_000_000n});
  const dust = reviewPosition({...quote,action:'open-long',a:'1125000',first:'1125000',b:'93750000',second:'93750000'},snapshot)!;
  expect(dust.beforeFunding.lower).toBeUndefined();
});

test('rejects stale, divergent, foreign-version or malformed review data', () => {
  expect(verifiedPositionPrice(snapshot,119)).toBeUndefined();
  expect(verifiedPositionPrice({...snapshot,state:'head-clock-unavailable'})).toBeUndefined();
  expect(verifiedPositionPrice({...snapshot,observations:[snapshot.observations[0],{price:'1800000',observedAt:String(now)}]})).toBeUndefined();
  expect(verifiedPositionPrice({...snapshot, priceVersion:'not numeric'})).toBeUndefined();
  expect(reviewPosition({...quote,priceVersion:'5'},snapshot)).toBeUndefined();
  expect(reviewPosition({...quote,observedAt:now - 1},snapshot)).toBeUndefined();
  expect(reviewPosition({...quote,first:'123'},snapshot)).toBeUndefined();
  expect(reviewPosition({...quote,a:'1100000',first:'1100000'},snapshot)).toBeUndefined();
});

import type { CommerceOrder, OrderEscrowStatus } from '@nu54/protocol';
import { holdOrder, orderFinished, orderProgress, pollDelayMs, screenMs } from '../src/kiosk/orderProgress';

const order = (state: CommerceOrder['state'], more: Partial<CommerceOrder> = {}): CommerceOrder => ({
  id: 'f0551c76-7592-4850-a414-54464d24d004', revision: 1, state, asset: '0x' + '1'.repeat(40), amount: '7',
  expiresAt: new Date(2_000_000).toISOString(), ...more,
});
const escrow = (stage: OrderEscrowStatus['stage']) => ({ stage }) as OrderEscrowStatus;
const at = 1_000;

test('only a paid order with its number is shown as paid', () => {
  expect(orderProgress({ sale: order('paid', { pickupNo: 17 }) }, at)).toBe('paid');
  expect(orderProgress({ sale: order('paid') }, at)).toBe('elsewhere');
  for (const stage of ['received', 'submitted'] as const)
    expect(orderProgress({ sale: order('checking', { escrow: escrow(stage) }), holdUntil: at + 1 }, at)).toBe('submitting');
  for (const stage of ['locked', 'delivered', 'disputed', 'paid', 'refunded'] as const)
    expect(orderProgress({ sale: order('paid', { pickupNo: 1, escrow: escrow(stage) }) }, at)).toBe('paid');
});

test('only proof that nothing was taken is shown as unpaid', () => {
  expect(orderProgress({ sale: order('checking', { escrow: escrow('failed') }) }, at)).toBe('unpaid');
  expect(orderProgress({ sale: order('closed', { escrow: escrow('failed') }) }, at)).toBe('unpaid');
  expect(orderProgress({ sale: order('closed', { menuCheckout: { branchLabel: 'A', payerBound: true, finalOutcome: 'expired-unpaid' } }) }, at)).toBe('unpaid');
  expect(orderProgress({ sale: order('closed') }, at)).toBe('elsewhere');
});

test('an order moves from approving to submitting, and an unresolved one ends as unknown', () => {
  const issued = order('issued');
  expect(orderProgress({}, at)).toBe('open');
  expect(orderProgress({ sale: order('preview') }, at)).toBe('open');
  expect(orderProgress({ sale: issued, holdUntil: at + 1 }, at)).toBe('approving');
  expect(orderProgress({ sale: issued, holdUntil: at }, at)).toBe('late');
  expect(orderProgress({ sale: issued, holdUntil: at + 1, submitting: true }, at)).toBe('submitting');
  expect(orderProgress({ sale: issued, holdUntil: at, submitting: true }, at)).toBe('unknown');
  expect(orderProgress({ sale: order('checking'), holdUntil: at }, at)).toBe('unknown');
  expect(['paid', 'unpaid', 'elsewhere'].every(p => orderFinished(p as 'paid'))).toBe(true);
  expect(['open', 'approving', 'late', 'submitting', 'unknown'].some(p => orderFinished(p as 'open'))).toBe(false);
});

test('the hold ends with the quote, or with the longest possible quote when this terminal does not hold it', () => {
  expect(holdOrder(order('issued'), at, at + 70)).toBe(at + 70);
  expect(holdOrder(order('issued'), at)).toBe(at + 120);
  expect(holdOrder(order('checking'), at + 50, undefined, at + 120)).toBe(at + 120);
  expect(holdOrder(order('issued', { expiresAt: new Date((at + 20) * 1000).toISOString() }), at)).toBe(at + 20);
  expect(screenMs('open', undefined, at * 1000)).toBe(60_000);
  expect(screenMs('approving', at + 90, at * 1000)).toBe(150_000);
  expect(screenMs('unknown', at - 5, at * 1000)).toBe(60_000);
  expect(screenMs('paid', at + 90, at * 1000)).toBe(10_000);
});

test('reading backs off and stays bounded', () => {
  expect([0, 14, 15, 1000].map(reads => pollDelayMs(reads, 0))).toEqual([2_000, 2_000, 5_000, 5_000]);
  expect([1, 2, 3, 30].map(failures => pollDelayMs(0, failures))).toEqual([4_000, 8_000, 15_000, 15_000]);
});

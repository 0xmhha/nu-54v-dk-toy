import {readdirSync, readFileSync, statSync} from 'node:fs';
import {join} from 'node:path';
import React from 'react';
import Renderer from 'react-test-renderer';
import {assertCommerceCatalogPage, commerceAmount, commerceCartAmount, namedTokenAbiSha256, stableNetCoin, tokenAmount, type CommercePrice} from '@nu54/protocol';
import {missingUnits as basketMissing, shownBalance as basketBalance} from '../src/merchant/CustomerOrders';
import {choicePrice} from '../src/merchant/MenuChoices';
import {OrderDetail} from '../src/merchant/OpenOrders';
import type {OpenOrder} from '../src/merchant/orderRecords';
import {BookingPaymentTerms} from '../src/travel/BookingPaymentTerms';
import {assetMoney, cartMoney, missingUnits, orderMoney, orderUnits, receiptLines, shownBalance, shownUnits, storeMoney, storeOf, tokenText} from '../src/wallet/tokens';
import {policy} from './fixtures/order';

jest.mock('react-native-safe-area-context', () => require('react-native-safe-area-context/jest/mock').default);
jest.mock('../src/specs/NativeNearby.ts', () => ({__esModule: true, default: null}));
jest.mock('../src/specs/NativeRenterBle.ts', () => ({__esModule: true, default: {loadDevices: async () => ''}}));
jest.mock('../src/specs/NativeSubscriptionAlerts.ts', () => ({__esModule: true, default: null}));
jest.mock('../src/specs/NativeShopQr.ts', () => ({__esModule: true, default: null}));

// Owner decision 2026-11-27: every won amount on the phone is a whole number of won. What the customer has or gets
// back is cut off, what the customer pays or lacks is rounded up. The operations web and the shared wording keep the parts.
const won = {name: 'NU54 Test KRW', symbol: 'tKRW', label: 'tKRW', decimals: 2};
const coin = {name: 'WKRC', symbol: 'WKRC', label: 'KRWC', decimals: 18, boardDecimals: 2}, wei = 10n ** 18n;
const dollar = {name: 'NU54 Test USD', symbol: 'tUSDC', label: 'TestUSDC', decimals: 6};
const wonPolicy = {...policy, abiSha256: namedTokenAbiSha256, token: won};
const coinPolicy = {...policy, abiSha256: namedTokenAbiSha256, asset: stableNetCoin.asset, token: coin, maxBaseUnits: (300_000n * wei).toString()};
const asset = policy.asset;

test('a balance is cut off at the whole won and never rounded up', () => {
  expect(tokenText('1250099', {symbol: 'tKRW', decimals: 2, won: true})).toBe('12,500원');
  expect(tokenText('1250099', {symbol: 'tKRW', decimals: 2, won: true}, 'en')).toBe('₩12,500');
  expect(storeMoney('1250099', won)).toBe('12,500원');
  expect(storeMoney(1250099n, won, 'en')).toBe('₩12,500');
  expect(assetMoney('1250099', asset, storeOf(wonPolicy))).toBe('12,500원');
  expect(orderMoney('1250099', asset, wonPolicy, 'balance')).toBe('12,500원');
  // Less than one won is no won on the screen. The exact units stay what buttons and requests go by.
  expect(storeMoney('99', won)).toBe('0원');
  expect(shownUnits(99n, won, 'balance')).toBe(0n);
});

test('what is missing is rounded up, and the basket and the wallet count it the same way', () => {
  expect(basketMissing).toBe(missingUnits);
  expect(basketBalance).toBe(shownBalance);
  expect(storeMoney(missingUnits(300_000n, 299_999n, won), won, 'ko', 'shortfall')).toBe('1원');
  expect(storeMoney('1', won, 'ko', 'shortfall')).toBe('1원');
  expect(cartMoney('120001', asset, wonPolicy, 'shortfall')).toBe('1,201원');
  // Held and missing add up to the whole total: 2,999 won and 1 won of 3,000.
  expect(BigInt(shownBalance(299_999n, won)) + BigInt(missingUnits(300_000n, 299_999n, won))).toBe(300_000n);
});

test('a price the store server sends with a part of a won is shown rounded up, never as less than what leaves the wallet', () => {
  // The store server publishes won prices in whole won only (token.go wholePrice). The phone's parser does not ask
  // for that, so the screen has its own answer for a price that came through anyway.
  const rows = [{version: 1, expiresAt: new Date(Date.now() + 120_000).toISOString(), asset, available: 5, sku: 'latte', optionKey: 'standard', label: '카페라떼', unitPrice: '650050'}];
  expect(() => assertCommerceCatalogPage(rows, Math.floor(Date.now() / 1000), 10)).not.toThrow();
  const price = (rows as CommercePrice[])[0];
  expect(orderMoney(price.unitPrice, price.asset, wonPolicy, 'charge')).toBe('6,501원');
  expect(cartMoney(price.unitPrice, price.asset, wonPolicy, 'charge')).toBe('6,501원');
  expect(choicePrice(rows as CommercePrice[], {from: price.unitPrice, varies: true} as never, 'ko', (units, of) => orderMoney(units, of, wonPolicy, 'charge'))).toBe('6,501원부터');
  expect(storeMoney('650001', won, 'ko', 'charge')).toBe('6,501원');
  expect(assetMoney('650001', asset, storeOf(wonPolicy), 'en', 'charge')).toBe('₩6,501');
  // A whole price is shown as it is.
  expect(orderMoney('650000', asset, wonPolicy, 'charge')).toBe('6,500원');
  expect(orderUnits('650050', asset, wonPolicy, 'charge')).toBe(650_100n);
});

test('what comes back to the customer is cut off', () => {
  expect(orderMoney('1666650', asset, wonPolicy, 'refund')).toBe('16,666원'); // half of 33,333 won
  expect(storeMoney('1666650', won, 'ko', 'refund')).toBe('16,666원');
});

test('the coin of 18 decimals is whole won as well', () => {
  expect(tokenText((6_500n * wei - 1n).toString(), {symbol: 'KRWC', decimals: 18, won: true})).toBe('6,499원');
  expect(storeMoney(6_500n * wei - 1n, coin)).toBe('6,499원');
  expect(storeMoney(6_499n * wei + 1n, coin, 'ko', 'charge')).toBe('6,500원');
  expect(storeMoney(6_499n * wei + 1n, coin, 'en', 'shortfall')).toBe('₩6,500');
  expect(orderMoney((6_499n * wei + 1n).toString(), stableNetCoin.asset, coinPolicy, 'charge')).toBe('6,500원');
  expect(orderMoney((wei - 1n).toString(), stableNetCoin.asset, coinPolicy, 'refund')).toBe('0원');
});

test('below zero the rule is the same: what is held is never more than it is, what is paid never less', () => {
  expect(storeMoney(-50n, won)).toBe('-1원');
  expect(storeMoney(-1250050n, won)).toBe('-12,501원');
  expect(storeMoney(-1250050n, won, 'en', 'charge')).toBe('-₩12,500');
  expect(storeMoney(-50n, won, 'ko', 'charge')).toBe('0원');
});

test('dollars keep their rule and an unknown asset is still not worded', () => {
  expect(storeMoney('4499999', dollar, 'ko', 'charge')).toBe('4.49 TestUSDC');
  expect(orderMoney('4499999', asset, policy, 'charge')).toBe('4.49 TestUSDC');
  expect(shownUnits(4_499_999n, dollar, 'charge')).toBe(4_499_999n);
  expect(orderMoney('650050', '0x' + '7'.repeat(40), wonPolicy, 'charge')).toBe('650050 기준 단위');
  expect(orderMoney('-650050', asset, wonPolicy, 'charge')).toBe('-650050 기준 단위');
});

test('the shared wording that the kiosk and the operations side read keeps the parts of a won', () => {
  expect(tokenAmount('1250001', won)).toBe('12,500.01원');
  expect(tokenAmount(-1250050n, won)).toBe('-12,500.50원');
  expect(tokenAmount(6_500n * wei - 1n, coin)).toBe('6,499.99원');
  expect(tokenAmount('1250001', won, 'en')).toBe('₩12,500.01');
  expect(commerceAmount('1250001', asset, wonPolicy)).toBe('12,500.01원');
  expect(commerceCartAmount('1250001', asset, wonPolicy)).toBe('12,500.01원');
});

const sum = (rows: bigint[]) => rows.reduce((total, row) => total + row, 0n);
test('receipt rows are whole won and add up, across each line and down each column', () => {
  // Whole won, as the store server sends them: nothing changes.
  expect(receiptLines([{total: '600000', tax: '54500'}, {total: '650000', tax: '59100'}], won))
    .toEqual([{total: 600_000n, tax: 54_500n, supply: 545_500n}, {total: 650_000n, tax: 59_100n, supply: 590_900n}]);
  // With parts of a won: paid 12,500.75 reads 12,501, the tax 1,136.42 reads 1,136, and the supply value is the rest.
  const lines = receiptLines([{total: '600050', tax: '54550'}, {total: '650025', tax: '59092'}], won);
  for (const line of lines) {
    expect([line.total % 100n, line.tax % 100n, line.supply % 100n]).toEqual([0n, 0n, 0n]);
    expect(line.supply + line.tax).toBe(line.total);
  }
  expect(sum(lines.map(line => line.total))).toBe(1_250_100n);
  expect(sum(lines.map(line => line.tax))).toBe(113_600n);
  expect(sum(lines.map(line => line.supply))).toBe(1_250_100n - 113_600n);
  // A dollar token, or no token, keeps every unit.
  expect(receiptLines([{total: '4500001', tax: '409091'}], dollar)).toEqual([{total: 4_500_001n, tax: 409_091n, supply: 4_090_910n}]);
  expect(receiptLines([{total: '600050'}])).toEqual([{total: 600_050n, tax: 0n, supply: 600_050n}]);
});

const order: OpenOrder = {
  payer: '0x' + '1'.repeat(40), generation: '1', sale: '0fbc54ef-2827-4ab5-bfcd-3a27611c3001', branch: '0fbc54ef-2827-4ab5-bfcd-3a27611c3002',
  customer: '0fbc54ef-2827-4ab5-bfcd-3a27611c3003', confirmer: '0x' + '2'.repeat(40), store: '여울 커피', asset,
  amount: '1250075', escrow: '0x' + '4'.repeat(40), merchantId: 'a'.repeat(64), orderKey: 'b'.repeat(64), sentAt: 1_764_000_000, pickupNo: 17, stage: 'paid',
  items: [{name: '카페라떼', quantity: 1, total: '600050', tax: '54550', taxKind: 'vat10'}, {name: '치즈케이크', quantity: 1, total: '650025', tax: '59092', taxKind: 'vat10'}],
};
test('the receipt on the screen adds up to the amount paid above it', async () => {
  const money = (units: string) => orderMoney(units, asset, wonPolicy, 'charge');
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => {
    tree = Renderer.create(<OrderDetail order={order} amount={money(order.amount)} money={money} token={won} fresh={false} now={1_764_000_100}
      actions={{answer: () => {}, refresh: () => {}, back: () => {}}} />);
  });
  const text = JSON.stringify(tree.toJSON());
  const shown = (label: string) => Number(new RegExp(`"${label}"\\]\\},\\{[^\\]]*\\["([0-9,]+)원"`).exec(text)![1].replace(/,/g, ''));
  expect(text).not.toMatch(/[0-9]\.[0-9]+원/);
  expect([shown('낸 금액'), shown('세금 합계'), shown('공급가액 합계')]).toEqual([12_501, 1_136, 11_365]);
  expect(shown('공급가액 합계') + shown('세금 합계')).toBe(shown('낸 금액'));
  const rows = [...text.matchAll(/"(카페라떼 ×1|치즈케이크 ×1|공급가액|세금)"\]\},\{[^\]]*\["([0-9,]+)원"/g)].map(match => [match[1], Number(match[2].replace(/,/g, ''))] as const);
  const column = (label: string) => rows.filter(row => row[0] === label).reduce((total, row) => total + row[1], 0);
  expect(rows).toHaveLength(6);
  expect(column('카페라떼 ×1') + column('치즈케이크 ×1')).toBe(12_501);
  expect([column('공급가액'), column('세금')]).toEqual([11_365, 1_136]);
  expect(text).toContain('12,501원'); // the amount paid above the receipt
  await Renderer.act(async () => tree.unmount());
});

test('a booking shows what is paid rounded up, and its total is the sum of the two rows', async () => {
  const booking = {id: 'b', slot: 's', partySize: 2, expiresAt: '2026-12-01T00:00:00Z', state: 'held', termsVersion: 1, cancellation: '전액 환불', asset, deposit: '1000050', balance: '2000050'};
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<BookingPaymentTerms payment={{booking, kind: 'deposit', scope: {branch: 'x', customer: 'y', tenant: 'z'}}} policy={wonPolicy} />); });
  const text = JSON.stringify(tree.toJSON());
  for (const part of ['10,001원', '20,001원', '30,002원']) expect(text).toContain(part);
  expect(text).not.toMatch(/[0-9]\.[0-9]+원/);
  await Renderer.act(async () => tree.unmount());
});

test('no screen words a won amount with the shared wording directly', () => {
  // The shared wording keeps the parts of a won. On the phone it is reached through wallet/tokens.ts, which makes the
  // amount whole first; the board pages do the same before they call it.
  const files = (dir: string): string[] => readdirSync(dir).flatMap(name => statSync(join(dir, name)).isDirectory() ? files(join(dir, name)) : /\.tsx?$/.test(name) ? [join(dir, name)] : []);
  const root = join(__dirname, '../src');
  const direct = files(root).filter(file => !file.endsWith(join('wallet', 'tokens.ts'))).flatMap(file =>
    (readFileSync(file, 'utf8').match(/\b(?:tokenAmount|commerceAmount|commerceCartAmount)\(\w*/g) ?? []).map(call => `${file.slice(root.length + 1)}: ${call}`));
  expect(direct).toEqual(['wallet/BoardLife.tsx: tokenAmount(shownUnits']);
});

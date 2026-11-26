import React from 'react';
import Renderer from 'react-test-renderer';
import {OrderDetail} from '../src/merchant/OpenOrders';
import {orderRecords, type OpenOrder} from '../src/merchant/orderRecords';

jest.mock('react-native-safe-area-context', () => require('react-native-safe-area-context/jest/mock').default);

const order: OpenOrder = {
  payer: '0x' + '1'.repeat(40), generation: '1', sale: '0fbc54ef-2827-4ab5-bfcd-3a27611c3001', branch: '0fbc54ef-2827-4ab5-bfcd-3a27611c3002',
  customer: '0fbc54ef-2827-4ab5-bfcd-3a27611c3003', confirmer: '0x' + '2'.repeat(40), store: '여울 커피', asset: '0x' + '3'.repeat(40),
  amount: '1250000', escrow: '0x' + '4'.repeat(40), merchantId: 'a'.repeat(64), orderKey: 'b'.repeat(64), sentAt: 1_764_000_000, pickupNo: 17, stage: 'paid',
  items: [{name: '카페라떼 · 차갑게 · 크게 · 샷 추가', quantity: 1, total: '600000', tax: '54600', taxKind: 'vat10'},
    {name: '치즈케이크', quantity: 1, total: '650000', tax: '59100', taxKind: 'vat10'}],
};
const money = (units: string) => `${(Number(units) / 100).toLocaleString('en-US')}원`;
const text = (tree: Renderer.ReactTestRenderer) => JSON.stringify(tree.toJSON());

test('the receipt shows each line with the supply value and the tax inside it, and the sums', async () => {
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => {
    tree = Renderer.create(<OrderDetail order={order} amount="12,500원" money={money} fresh={false} now={1_764_000_100}
      actions={{answer: () => {}, refresh: () => {}, back: () => {}}} />);
  });
  for (const part of ['영수증', '카페라떼 · 차갑게 · 크게 · 샷 추가 ×1', '6,000원', '5,454원', '546원', '591원', '5,909원',
    '공급가액 합계', '11,363원', '세금 합계', '1,137원', '낸 금액', '12,500원', '뺀 금액'])
    expect(text(tree)).toContain(part);
  // Lines that do not add up to what was paid are not shown as a receipt.
  await Renderer.act(async () => {
    tree.update(<OrderDetail order={{...order, amount: '1250001'}} amount="12,500원" money={money} fresh={false} now={1_764_000_100}
      actions={{answer: () => {}, refresh: () => {}, back: () => {}}} />);
  });
  expect(text(tree)).not.toContain('영수증');
  // An order kept before receipts were kept shows its amount alone.
  await Renderer.act(async () => {
    tree.update(<OrderDetail order={{...order, items: undefined}} amount="12,500원" money={money} fresh={false} now={1_764_000_100}
      actions={{answer: () => {}, refresh: () => {}, back: () => {}}} />);
  });
  expect(text(tree)).not.toContain('영수증');
  await Renderer.act(async () => tree.unmount());
});

test('a kept order with lines reads back, and lines that cannot be true are refused', async () => {
  const journal = new Map<string, string>();
  const storage = {loadJournal: async (name: string) => journal.get(name) ?? '', saveJournal: async (name: string, value: string) => { journal.set(name, value); },
    randomBytes: async () => 'AAAAAAAAAAAAAAAAAAAAAA=='};
  const records = orderRecords(storage);
  await records.keep(order);
  expect((await records.list())[0].items).toEqual(order.items);
  for (const items of [[{...order.items![0], tax: '700000'}], [{...order.items![0], taxKind: undefined}], [{...order.items![0], quantity: 0}], []]) {
    journal.clear();
    await expect(records.keep({...order, items: items as OpenOrder['items']})).rejects.toThrow();
  }
});

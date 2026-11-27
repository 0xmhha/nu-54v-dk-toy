import React from 'react';
import Renderer from 'react-test-renderer';
import {ShopDiscovery} from '../src/merchant/ShopDiscovery.tsx';

// A store QR holds a branch number only. In a build with the customer entrance it is a shortcut to that store's
// page, for any store: nothing compares it with a store this phone is registered at.

const cafe = '11111111-1111-4111-8111-111111111111', bakery = '22222222-2222-4222-8222-222222222222';
type Props = React.ComponentProps<typeof ShopDiscovery>;
let tree: Renderer.ReactTestRenderer;
async function render(overrides: Partial<Props>) {
  await Renderer.act(async () => {
    tree = Renderer.create(<ShopDiscovery find={async () => undefined} select={async () => { throw new Error('unused'); }} readQr={async () => `nu54://shop/${cafe}`}
      online={async () => { throw new Error('SHOP_WRONG_BRANCH'); }} open={async () => {}} reset={() => {}} wallet={() => {}} {...overrides} />);
  });
}
const tap = async (label: string) => {
  const action = tree.root.findAll(node => node.props.title === label && typeof node.props.onPress === 'function')[0];
  expect([label, !!action]).toEqual([label, true]);
  await Renderer.act(async () => action.props.onPress());
};
afterEach(async () => { await Renderer.act(async () => tree.unmount()); });

test('a scanned store QR opens that store\'s page, and no registered store is asked about it', async () => {
  const page = jest.fn(), online = jest.fn(async () => { throw new Error('SHOP_WRONG_BRANCH'); });
  await render({page, online});
  await tap('근처 단말 찾기');
  await tap('카메라로 매장 QR 확인');
  expect(page.mock.calls).toEqual([[cafe]]);
  expect(online).not.toHaveBeenCalled();
  expect(JSON.stringify(tree.toJSON())).not.toContain('등록 지점의 매장 입구 QR을 확인하세요.');
});

test('a store QR that opened the app goes to the page of the store it names, whichever store that is', async () => {
  const page = jest.fn();
  await render({page, initialQr: `nu54://shop/${bakery}`});
  expect(page.mock.calls).toEqual([[bakery]]);
});

test('what is not a store QR opens no page', async () => {
  const page = jest.fn();
  await render({page, readQr: async () => `nu54://order/${cafe}`});
  await tap('근처 단말 찾기');
  await tap('카메라로 매장 QR 확인');
  expect(page).not.toHaveBeenCalled();
});

test('a build without the customer entrance keeps the earlier check', async () => {
  const online = jest.fn(async () => { throw new Error('SHOP_WRONG_BRANCH'); });
  await render({online});
  await tap('근처 단말 찾기');
  await tap('카메라로 매장 QR 확인');
  expect(online).toHaveBeenCalledWith(cafe);
  expect(JSON.stringify(tree.toJSON())).toContain('등록 지점의 매장 입구 QR을 확인하세요.');
});

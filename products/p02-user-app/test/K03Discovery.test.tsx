import React from 'react';
import { TextInput } from 'react-native';
import Renderer from 'react-test-renderer';
import {
  ShopDiscovery,
  readEntranceQr,
  readShopIdentity,
} from '../src/merchant/ShopDiscovery.tsx';

const branch = '11111111-1111-4111-8111-111111111111';
const other = '22222222-2222-4222-8222-222222222222';
const qr = `nu54://shop/${branch}`;
const identity = { branch, branchLabel: '여울 커피 성수점', terminal: other };
const resolved = { identity, route: 'relay' as const };

test('entrance codes are branch locators and cannot carry an original order or endpoint', () => {
  expect(readEntranceQr(qr)).toBe(branch);
  for (const raw of [
    `nu54://order/${branch}`,
    `${qr}/${other}`,
    `${qr}?endpoint=https://example.com`,
  ])
    expect(() => readEntranceQr(raw)).toThrow();
  expect(() =>
    readShopIdentity({ ...identity, branch: other }, branch, other),
  ).toThrow();
  expect(() =>
    readShopIdentity({ ...identity, customerRef: other }, branch, other),
  ).toThrow();
});

test('nearby candidates require authenticated branch and explicit matching signage before K10', async () => {
  const select = jest.fn(async (handle: string) => {
    if (handle === 'wrong') throw new Error('NEARBY_WRONG_TERMINAL');
    return resolved;
  });
  const open = jest.fn();
  const find = jest.fn(async () => [
    { handle: 'wrong' },
    { handle: 'verified' },
  ]);
  const screen = await render({ select, open, find });
  expect(screen.button('이 매장 메뉴 열기')).toBeUndefined();
  await screen.input('다른 지점');
  await screen.tap('근처 단말 찾기');
  await screen.tap('근거리 단말 후보 1 인증');
  expect(screen.text()).toContain('지점 또는 단말 신원을 확인할 수 없습니다');
  expect(open).not.toHaveBeenCalled();
  await screen.tap('근거리 단말 후보 2 인증');
  expect(screen.text()).toContain(identity.branchLabel);
  expect(screen.text()).toContain('단말 BLE 중계');
  expect(screen.text()).not.toContain('RSSI');
  expect(screen.button('이 매장 메뉴 열기').props.disabled).toBe(true);
  await screen.tap('다른 매장 찾기');
  await screen.tap('근처 단말 찾기');
  expect(find).toHaveBeenCalledTimes(2);
  await screen.tap('근거리 단말 후보 2 인증');
  expect(screen.button('이 매장 메뉴 열기').props.disabled).toBeFalsy();
  await screen.tap('이 매장 메뉴 열기');
  expect(open).toHaveBeenCalledWith(resolved);
  await screen.close();
});

test('permission denial offers the camera and online QR, and both offline routes offer original recovery only', async () => {
  const readQr = jest.fn(async () => qr);
  const online = jest
    .fn()
    .mockResolvedValueOnce({
      identity: { branch, branchLabel: identity.branchLabel },
      route: 'direct',
    })
    .mockRejectedValueOnce(new Error('OFFLINE'));
  const recover = jest.fn();
  const open = jest.fn();
  const screen = await render({
    find: async () => undefined,
    readQr,
    online,
    recover,
    open,
  });
  await screen.tap('근처 단말 찾기');
  expect(screen.text()).toContain('매장 입구의 QR을 스캔하세요');
  await screen.tap('카메라로 매장 QR 확인');
  expect(readQr).toHaveBeenCalled();
  expect(online).toHaveBeenCalledWith(branch);
  await screen.tap('다른 매장 찾기');
  await screen.tap('근처 단말 찾기');
  await screen.tap('카메라로 매장 QR 확인');
  expect(screen.text()).toContain('새 주문을 시작할 수 없습니다');
  expect(screen.button('이 매장 메뉴 열기')).toBeUndefined();
  expect(open).not.toHaveBeenCalled();
  await screen.tap('원래 주문 상태 확인');
  expect(recover).toHaveBeenCalledTimes(1);
  await screen.close();
});

type Props = React.ComponentProps<typeof ShopDiscovery>;
async function render(overrides: Partial<Props>) {
  let tree: Renderer.ReactTestRenderer;
  await Renderer.act(async () => {
    tree = Renderer.create(
      <ShopDiscovery
        find={async () => []}
        select={async () => resolved}
        readQr={async () => qr}
        online={async () => ({ identity, route: 'direct' })}
        open={async () => {}}
        reset={() => {}}
        wallet={() => {}}
        {...overrides}
      />,
    );
  });
  const button = (label: string) =>
    tree.root.findAll(
      node =>
        node.props.title === label && typeof node.props.onPress === 'function',
    )[0];
  return {
    button,
    tap: async (label: string) => {
      const action = button(label);
      expect(action).toBeDefined();
      expect(action.props.disabled).toBeFalsy();
      await Renderer.act(async () => action.props.onPress());
    },
    input: async (text: string) => {
      const input = tree.root
        .findAllByType(TextInput)
        .find(node => node.props.accessibilityLabel === '간판의 지점명')!;
      await Renderer.act(async () => input.props.onChangeText(text));
    },
    text: () => JSON.stringify(tree.toJSON()),
    close: () => Renderer.act(async () => tree.unmount()),
  };
}

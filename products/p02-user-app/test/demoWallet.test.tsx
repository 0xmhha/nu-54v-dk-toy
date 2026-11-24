import React from 'react';
import Renderer from 'react-test-renderer';
import {Text} from 'react-native';
import {readFileSync} from 'fs';
import {join} from 'path';
import {demoLife, demoOrders, demoPayments, DemoWalletHome, demoReadState} from '../src/wallet/DemoWalletHome';

jest.mock('react-native-safe-area-context', () => require('react-native-safe-area-context/jest/mock').default);

test('the demo wallet home shows fixture data with a banner and no usable action', async () => {
  let tree!: Renderer.ReactTestRenderer;
  const close = jest.fn();
  await Renderer.act(async () => { tree = Renderer.create(<DemoWalletHome t={text => text} close={close} />); });
  const text = JSON.stringify(tree.toJSON());
  expect(text).toContain('시연용 화면');
  // The balance text is rendered from several children ("42", " ", "TestUSDC").
  const shown = tree.root.findAll(node => node.type === Text).map(node => [node.props.children].flat().join('')).join('|');
  expect(shown).toContain('42 TestUSDC');
  const enabled = tree.root.findAll(node => typeof node.props.onPress === 'function' &&
    ['button', 'tab'].includes(node.props.accessibilityRole) && !node.props.accessibilityState?.disabled)
    .map(node => node.props.accessibilityLabel);
  // Only fixture pages, the fake balance and leaving the demo remain usable.
  expect([...new Set(enabled)].sort()).toEqual(['시연 화면 닫기', '잔액 숨기기', '내 보드 연결됨, 오늘 더 승인할 수 있는 금액 92.00 TestUSDC',
    '그린 카페 주문번호 17 준비 중', ...demoPayments(0).map(([label]) => label), ...demoOrders(0).map(([label]) => label),
    '운영자 허가 없음', ...demoLife(0).map(([label]) => label)].sort());
  await Renderer.act(async () => tree.root.findByProps({accessibilityLabel: '시연 화면 닫기'}).props.onPress());
  expect(close).toHaveBeenCalled();
});

test('the demo payment pages are fixtures without a board, kiosk or server behind them', async () => {
  const source = readFileSync(join(__dirname, '../src/wallet/DemoWalletHome.tsx'), 'utf8');
  expect(source).not.toMatch(/ownerSession|specs\/Native|customerClient|nearbyClient|signDigest/);
  for (const [label] of [...demoPayments(0), ...demoOrders(0), ...demoLife(0)]) {
    let tree!: Renderer.ReactTestRenderer;
    await Renderer.act(async () => { tree = Renderer.create(<DemoWalletHome t={text => text} close={() => {}} />); });
    await Renderer.act(async () => tree.root.findAll(node => node.props.accessibilityLabel === label && typeof node.props.onPress === 'function')[0].props.onPress());
    expect(JSON.stringify(tree.toJSON())).toContain('시연용 화면');
    expect(tree.root.findAll(node => ['order-payment', 'order-detail'].includes(node.props.testID) || String(node.props.testID).startsWith('board-life-')).length).toBeGreaterThan(0);
    await Renderer.act(async () => tree.unmount());
  }
});

test('fixture data is scoped to a fake board, never to a real one', () => {
  const state = demoReadState(0);
  expect(state.observations[0].scope.startsWith('de'.repeat(32))).toBe(true);
  expect(state.observations[0].address).toBe('0x' + 'de'.repeat(20));
});

test('no build in the repository turns the demo wallet on', () => {
  const gradle = readFileSync(join(__dirname, '../android/app/build.gradle'), 'utf8');
  expect(gradle).toContain('buildConfigField "boolean", "NU54_DEMO_WALLET", "false"');
  expect(gradle).not.toMatch(/NU54_DEMO_WALLET", "true"/);
  const signing = readFileSync(join(__dirname, '../../../scripts/android/release-signing.gradle'), 'utf8');
  expect(signing).not.toContain('NU54_DEMO_WALLET');
});

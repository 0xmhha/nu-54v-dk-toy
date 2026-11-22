import React from 'react';
import Renderer from 'react-test-renderer';
import {Text} from 'react-native';
import {readFileSync} from 'fs';
import {join} from 'path';
import {DemoWalletHome, demoReadState} from '../src/wallet/DemoWalletHome';

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
  // Only showing or hiding the fake balance and leaving the demo remain usable.
  expect([...new Set(enabled)].sort()).toEqual(['시연 화면 닫기', '잔액 숨기기'].sort());
  await Renderer.act(async () => tree.root.findByProps({accessibilityLabel: '시연 화면 닫기'}).props.onPress());
  expect(close).toHaveBeenCalled();
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

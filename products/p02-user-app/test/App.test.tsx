/**
 * @format
 */

import React from 'react';
import ReactTestRenderer from 'react-test-renderer';
import {Linking, TextInput} from 'react-native';
import App from '../src/App';
import {importEnrollment, readCommissioningQr} from '../src/wallet/ownerWire';

jest.mock('react-native-safe-area-context', () => require('react-native-safe-area-context/jest/mock').default);
jest.mock('../src/specs/NativeRenterBle.ts', () => ({
  __esModule: true,
  default: {
    loadDevices: async () => '',
    scan: async () => {},
    stopScan: async () => {},
    disconnect: async () => {},
    onScan: () => ({ remove() {} }),
  },
}));
jest.mock('../src/specs/NativeOwnerWallet.ts', () => ({
  __esModule: true,
  default: {loadProfile: async () => ''},
}));

test('with no registered device the app offers guided profiles and keeps approval closed', async () => {
  let tree: ReactTestRenderer.ReactTestRenderer | undefined;
  await ReactTestRenderer.act(async () => {
    tree = ReactTestRenderer.create(<App />);
  });
  const text = JSON.stringify(tree!.toJSON());
  expect(text).toContain('지갑 시작하기');
  expect(text).toContain('결제 연습');
  expect(text).not.toContain('승인하기');
});

test('payments can open from home before any board is enrolled', async () => {
  let tree!: ReactTestRenderer.ReactTestRenderer;
  await ReactTestRenderer.act(async () => {
    tree = ReactTestRenderer.create(<App />);
  });
  const payment = tree.root.findAll(node =>
    node.props.accessibilityLabel === '내 결제' && typeof node.props.onPress === 'function',
  )[0];
  expect(payment).toBeDefined();
  await ReactTestRenderer.act(async () => payment.props.onPress());
  expect(JSON.stringify(tree.toJSON())).toContain('내 결제와 미결 청구');
  await ReactTestRenderer.act(async () => tree.unmount());
});

test('a wired commissioning QR previews pins before populating the separate registration fields', async () => {
  const device = 'ab'.repeat(32);
  const boardSPKI = 'cd'.repeat(32);
  const uri = `nu54://commissioning/v1/${device}/${boardSPKI}`;
  const initial = jest.spyOn(Linking, 'getInitialURL').mockResolvedValue(uri);
  let tree!: ReactTestRenderer.ReactTestRenderer;
  try {
    await ReactTestRenderer.act(async () => {
      tree = ReactTestRenderer.create(<App />);
    });
    const action = (label: string) => tree.root.findAll(node =>
      node.props.accessibilityLabel === label && typeof node.props.onPress === 'function',
    )[0];
    const field = (label: string) => tree.root.findAllByType(TextInput).find(node =>
      node.props.accessibilityLabel === label,
    )!.props.value;
    await ReactTestRenderer.act(async () => action('설치된 보드 연결').props.onPress());
    expect(JSON.stringify(tree.toJSON())).toContain(boardSPKI);
    expect(field('기기 식별값')).toBe('');
    await ReactTestRenderer.act(async () => action('대조한 설치 지문 사용').props.onPress());
    expect(field('기기 식별값')).toBe(device);
    expect(field('발급 문서의 서버 공개키 지문')).toBe(boardSPKI);
    expect(field('발급 등록서')).toBe('');
    const issued = JSON.stringify({version: 2, device, boardSPKI, ca: 'MA==', certificateChain: ['MA==']});
    expect(importEnrollment(issued, device, boardSPKI).device).toBe(device);
    expect(() => importEnrollment(issued, device, '01'.repeat(32))).toThrow('PROFILE_NOT_AUTHENTICATED');
    expect(readCommissioningQr(uri + '?authorization=unused')).toBeUndefined();
    expect(readCommissioningQr(uri.replace('/v1/', '/v2/'))).toBeUndefined();
    expect(readCommissioningQr(uri.replace(device, '0'.repeat(64)))).toBeUndefined();
  } finally {
    await ReactTestRenderer.act(async () => tree?.unmount());
    initial.mockRestore();
  }
});

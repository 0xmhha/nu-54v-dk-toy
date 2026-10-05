/**
 * @format
 */

import React from 'react';
import ReactTestRenderer from 'react-test-renderer';
import App from '../src/App';

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

test('with no registered device the app starts on the scan screen and has no approve button', async () => {
  let tree: ReactTestRenderer.ReactTestRenderer | undefined;
  await ReactTestRenderer.act(async () => {
    tree = ReactTestRenderer.create(<App />);
  });
  const text = JSON.stringify(tree!.toJSON());
  expect(text).toContain('기기 찾기');
  expect(text).not.toContain('승인하기');
});

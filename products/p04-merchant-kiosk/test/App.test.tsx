/**
 * @format
 */

import React from 'react';
import ReactTestRenderer from 'react-test-renderer';
import App from '../src/App';

// Native modules are not in the Jest runtime: an empty vault and an idle BLE central.
jest.mock('../src/specs/NativeKioskVault.ts', () => ({
  __esModule: true,
  default: {
    takeFile: async () => null,
    getSetting: async () => null,
    unwrapKey: async () => null,
    putSetting: async () => {},
    wrapKey: async () => {},
    randomBytes: async () => '',
  },
}));
jest.mock('react-native-safe-area-context', () => require('react-native-safe-area-context/jest/mock').default);
jest.mock('../src/specs/NativeNusBle.ts', () => ({ __esModule: true, default: {} }));

test('an unprovisioned kiosk asks for development provisioning', async () => {
  let tree: ReactTestRenderer.ReactTestRenderer | undefined;
  await ReactTestRenderer.act(async () => {
    tree = ReactTestRenderer.create(<App />);
  });
  expect(JSON.stringify(tree!.toJSON())).toContain('설정 필요');
});

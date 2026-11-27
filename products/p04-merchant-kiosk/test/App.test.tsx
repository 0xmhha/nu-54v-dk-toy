/**
 * @format
 */

import React from 'react';
import ReactTestRenderer from 'react-test-renderer';
import App from '../src/App';
import Http from '../src/specs/NativeCommerceHttp.ts';

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

jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({
  __esModule: true,
  default: {
    loadProfile: jest.fn(async () => ''),
    loadJournal: jest.fn(async () => ''),
    saveJournal: jest.fn(async () => undefined),
  },
}));

test('an unprovisioned kiosk asks for enrolled commerce identity', async () => {
  let tree: ReactTestRenderer.ReactTestRenderer | undefined;
  await ReactTestRenderer.act(async () => {
    tree = ReactTestRenderer.create(<App />);
  });
  expect(JSON.stringify(tree!.toJSON())).toContain('등록 공개키 확인');
  expect(Http?.saveJournal).toHaveBeenCalledTimes(1);
  expect(Http?.saveJournal).toHaveBeenCalledWith('terminal-public-menu-v1', '');
  expect(Http?.loadJournal).not.toHaveBeenCalled();
  await ReactTestRenderer.act(async () => {
    tree!.unmount();
  });
});

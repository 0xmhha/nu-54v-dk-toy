import {beginWallet, decodeWalletStorage, verifyRecovery, allowPersonalApproval} from '../src/wallet/profile.ts';

const legacy = [{address: 'existing-bond', name: 'NU54 Loan', wallet: '0x' + 'a'.repeat(40)}];
const metadata = {boardFingerprint: 'c'.repeat(64), address: legacy[0].wallet, network: '8283' as const, generation: 1};

test('an open v1 rental remains the legacy profile with exactly its original address', () => {
  expect(beginWallet(legacy, null)).toEqual({kind: 'legacy', device: legacy[0]});
});

test('a new install chooses a profile and cannot sign by entering practice mode', () => {
  expect(beginWallet([], null)).toEqual({kind: 'choose'});
  expect(allowPersonalApproval({kind: 'practice'}, {decoded: true, independent: true})).toBe(false);
});

test('a protected personal profile cannot silently downgrade to a rental or authorize after a restart', () => {
  const storage = decodeWalletStorage(JSON.stringify({version: 2, exercised: true, mode: 'personal', verified: metadata}));
  expect(beginWallet(legacy, storage)).toEqual({kind: 'locked', metadata});
  expect(allowPersonalApproval({kind: 'locked', metadata}, {valid: true}, 'intent')).toBe(false);
});

test('caller-supplied recovery booleans, matching addresses and stale generation do not convey authority', () => {
  const previous = {kind: 'recovering' as const, metadata};
  expect(verifyRecovery(previous, {...metadata, generation: 2, independent: true, authenticated: true})).toBe(previous);
  expect(verifyRecovery(previous, metadata)).toBe(previous);
  expect(() => decodeWalletStorage(JSON.stringify({version: 2, exercised: true, seed: 'injected'}))).toThrow('WALLET_STORAGE_CORRUPT');
});
